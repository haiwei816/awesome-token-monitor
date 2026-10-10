package collector

import (
	"database/sql"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type CindyScanner struct {
	GlobalDir string
	CNDir     string
}

func hashString(s string) int {
	h := fnv.New32a()
	h.Write([]byte(s))
	return int(h.Sum32() % 10000000)
}

func (s *CindyScanner) loadPiSessionIDs() map[string]bool {
	piSet := make(map[string]bool)
	dirs := []string{
		filepath.Join(s.CNDir, "pi-agent-home", "runtime", "pi-subagent-deleted"),
		filepath.Join(s.CNDir, "pi-agent-home", "runtime", "pi-subagent-runs"),
		filepath.Join(s.GlobalDir, "pi-agent-home", "runtime", "pi-subagent-deleted"),
		filepath.Join(s.GlobalDir, "pi-agent-home", "runtime", "pi-subagent-runs"),
	}
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if len(name) > 0 {
				piSet[name] = true
			}
		}
	}
	return piSet
}

func (s *CindyScanner) Scan(scanStates map[string][2]float64) ([]Conversation, []StepRecord, map[string][2]float64, error) {
	var allConvs []Conversation
	var allSteps []StepRecord
	updatedStates := make(map[string][2]float64)

	dirs := []struct {
		path       string
		defaultPrj string
	}{
		{s.GlobalDir, "Cindy-Global"},
		{filepath.Join(s.GlobalDir, "databases"), "Cindy-Global"},
		{s.CNDir, "Cindy-CN"},
		{filepath.Join(s.CNDir, "databases"), "Cindy-CN"},
	}

	for _, dInfo := range dirs {
		if _, err := os.Stat(dInfo.path); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(dInfo.path)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasPrefix(e.Name(), "cindy-") || !strings.HasSuffix(e.Name(), ".db") {
				continue
			}
			if strings.Contains(e.Name(), "lock") || strings.Contains(e.Name(), "bak") || strings.Contains(e.Name(), "migration") {
				continue
			}

			dbPath := filepath.Join(dInfo.path, e.Name())
			st, err := os.Stat(dbPath)
			if err != nil {
				continue
			}

			mtime := float64(st.ModTime().UnixNano()) / 1e9
			size := st.Size()
			lastState, exists := scanStates[dbPath]
			if exists && lastState[0] == mtime && int64(lastState[1]) == size {
				continue
			}

			cList, sList, _, err := s.scanSingleDB(dbPath, dInfo.defaultPrj)
			if err != nil {
				continue
			}

			allConvs = append(allConvs, cList...)
			allSteps = append(allSteps, sList...)
			updatedStates[dbPath] = [2]float64{mtime, float64(size)}
		}
	}

	return allConvs, allSteps, updatedStates, nil
}

type cindySession struct {
	id          string
	title       string
	workingDir  string
	model       string
	status      string
	totalTokens int64
	createdAt   int64
	updatedAt   int64
	agentKind   string
}

type modelStat struct {
	raw   string
	clean string
	in    int64
	out   int64
	cache int64
	tot   int64
	rem   int64
}

func (s *CindyScanner) scanSingleDB(dbPath, defaultProject string) ([]Conversation, []StepRecord, int, error) {
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, nil, 0, err
	}
	defer db.Close()

	piSessions := s.loadPiSessionIDs()

	// 1. Read sessions
	sessionsMap := make(map[string]cindySession)
	hasAgentKind := false
	var pragmaCols []string
	pRows, err := db.Query("PRAGMA table_info(sessions);")
	if err == nil {
		for pRows.Next() {
			var cid, notnull, pk int
			var name, ctype string
			var dfltValue sql.NullString
			if err := pRows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
				pragmaCols = append(pragmaCols, name)
				if name == "agent_kind" {
					hasAgentKind = true
				}
			}
		}
		pRows.Close()
	}

	sessQuery := "SELECT id, title, working_dir, model, status, total_token_usage, created_at, updated_at FROM sessions;"
	if hasAgentKind {
		sessQuery = "SELECT id, title, working_dir, model, status, total_token_usage, created_at, updated_at, agent_kind FROM sessions;"
	}

	rows, err := db.Query(sessQuery)
	if err != nil {
		return nil, nil, 0, err
	}

	for rows.Next() {
		var cs cindySession
		var t, wd, m, st, ak sql.NullString
		var tok, ca, ua sql.NullInt64
		if hasAgentKind {
			_ = rows.Scan(&cs.id, &t, &wd, &m, &st, &tok, &ca, &ua, &ak)
			cs.agentKind = ak.String
		} else {
			_ = rows.Scan(&cs.id, &t, &wd, &m, &st, &tok, &ca, &ua)
		}
		cs.title = t.String
		cs.workingDir = wd.String
		cs.model = m.String
		cs.status = st.String
		cs.totalTokens = tok.Int64
		cs.createdAt = ca.Int64
		cs.updatedAt = ua.Int64
		sessionsMap[cs.id] = cs
	}
	rows.Close()

	// 2. Precompute daily message counts
	dailyMsgCounts := make(map[string]int) // key: session_id + "_" + day
	totalMsgCounts := make(map[string]int) // key: session_id
	mRows, err := db.Query(`
		SELECT 
			session_id, 
			strftime('%Y-%m-%d', datetime(created_at / 1000, 'unixepoch')) as day, 
			count(*) 
		FROM messages 
		GROUP BY session_id, day;
	`)
	if err == nil {
		for mRows.Next() {
			var sid, day string
			var cnt int
			if err := mRows.Scan(&sid, &day, &cnt); err == nil {
				dailyMsgCounts[sid+"_"+day] = cnt
				totalMsgCounts[sid] += cnt
			}
		}
		mRows.Close()
	}

	// 3. Read daily_model_usage
	dailyModelUsage := make(map[string]*modelStat) // key: day + "_" + model
	overallModelUsage := make(map[string]*modelStat)
	var globalIn, globalOut, globalCache int64

	dmuRows, err := db.Query("SELECT day, model, input_tokens, output_tokens, cache_read_tokens FROM daily_model_usage;")
	if err == nil {
		for dmuRows.Next() {
			var d, rawM string
			var inV, outV, cacheV sql.NullInt64
			if err := dmuRows.Scan(&d, &rawM, &inV, &outV, &cacheV); err == nil {
				cleanM := strings.Split(rawM, "#")[0]
				stat := &modelStat{
					raw:   rawM,
					clean: cleanM,
					in:    inV.Int64,
					out:   outV.Int64,
					cache: cacheV.Int64,
					tot:   inV.Int64 + outV.Int64 + cacheV.Int64,
				}
				stat.rem = stat.tot
				dailyModelUsage[d+"_"+cleanM] = stat

				if overall, exists := overallModelUsage[cleanM]; exists {
					overall.in += stat.in
					overall.out += stat.out
					overall.cache += stat.cache
					overall.tot += stat.tot
				} else {
					overallModelUsage[cleanM] = &modelStat{
						raw: rawM, clean: cleanM, in: stat.in, out: stat.out, cache: stat.cache, tot: stat.tot,
					}
				}
				globalIn += stat.in
				globalOut += stat.out
				globalCache += stat.cache
			}
		}
		dmuRows.Close()
	}

	// 4. Read daily_session_usage
	type sessDayRow struct {
		sid    string
		tokens int64
		updAt  int64
	}
	dailyByDay := make(map[string][]sessDayRow)
	dsuRows, err := db.Query("SELECT day, session_id, tokens, updated_at FROM daily_session_usage;")
	if err == nil {
		for dsuRows.Next() {
			var day, sid string
			var tokens, updAt sql.NullInt64
			if err := dsuRows.Scan(&day, &sid, &tokens, &updAt); err == nil {
				dailyByDay[day] = append(dailyByDay[day], sessDayRow{
					sid:    sid,
					tokens: tokens.Int64,
					updAt:  updAt.Int64,
				})
			}
		}
		dsuRows.Close()
	}

	var convs []Conversation
	var steps []StepRecord
	coveredSessions := make(map[string]bool)
	syncedCount := 0

	for day, sList := range dailyByDay {
		// Available models on this day
		var availMods []*modelStat
		for k, v := range dailyModelUsage {
			if strings.HasPrefix(k, day+"_") {
				availMods = append(availMods, &modelStat{
					raw:   v.raw,
					clean: v.clean,
					in:    v.in,
					out:   v.out,
					cache: v.cache,
					tot:   v.tot,
					rem:   v.tot,
				})
			}
		}

		matchedMap := make(map[string]*modelStat)
		for _, sItem := range sList {
			coveredSessions[sItem.sid] = true
			var bestM *modelStat
			for _, am := range availMods {
				diff := math.Abs(float64(am.rem - sItem.tokens))
				maxTol := math.Max(10, float64(sItem.tokens)*0.001)
				if diff <= maxTol {
					bestM = am
					break
				}
			}
			if bestM != nil {
				bestM.rem -= sItem.tokens
				matchedMap[sItem.sid] = bestM
			}
		}

		for _, sItem := range sList {
			if _, matched := matchedMap[sItem.sid]; matched {
				continue
			}
			meta := sessionsMap[sItem.sid]
			sMod := strings.Split(meta.model, "#")[0]
			if sMod == "" {
				sMod = "gpt-6.1-sol"
			}
			var bestM *modelStat
			for _, am := range availMods {
				if am.rem > 0 && am.clean == sMod {
					bestM = am
					break
				}
			}
			if bestM != nil {
				bestM.rem -= sItem.tokens
				matchedMap[sItem.sid] = bestM
			} else {
				for _, am := range availMods {
					if am.rem > 0 {
						bestM = am
						break
					}
				}
				matchedMap[sItem.sid] = bestM
			}
		}

		for _, sItem := range sList {
			meta := sessionsMap[sItem.sid]
			am := matchedMap[sItem.sid]
			rawModel := "gpt-6.1-sol"
			if am != nil {
				rawModel = am.clean
			} else if meta.model != "" {
				rawModel = strings.Split(meta.model, "#")[0]
			}
			modDisp := FormatModelName(rawModel)

			targetEngine := ResolveCindyEngine(meta.agentKind, rawModel)
			if piSessions[sItem.sid] || strings.Contains(strings.ToLower(meta.agentKind), "pi") {
				targetEngine = "pi"
			}
			targetRole := GetEngineRole(targetEngine)

			var inT, outT, cacheT int64
			var mStats *modelStat
			if am != nil {
				mStats = am
			} else if ms, ok := dailyModelUsage[day+"_"+rawModel]; ok {
				mStats = ms
			} else if ms, ok := overallModelUsage[rawModel]; ok {
				mStats = ms
			}

			if mStats != nil && mStats.tot > 0 {
				inT = int64(float64(sItem.tokens) * (float64(mStats.in) / float64(mStats.tot)))
				outT = int64(float64(sItem.tokens) * (float64(mStats.out) / float64(mStats.tot)))
				cacheT = sItem.tokens - inT - outT
				if cacheT < 0 {
					cacheT = 0
				}
			} else {
				inT = int64(float64(sItem.tokens) * 0.05)
				outT = int64(float64(sItem.tokens) * 0.01)
				cacheT = sItem.tokens - inT - outT
				if cacheT < 0 {
					cacheT = 0
				}
			}

			updatedISO := day + "T12:00:00Z"
			if sItem.updAt > 0 {
				updatedISO = time.Unix(sItem.updAt/1000, 0).UTC().Format(time.RFC3339)
			}
			createdISO := updatedISO
			if meta.createdAt > 0 {
				createdISO = time.Unix(meta.createdAt/1000, 0).UTC().Format(time.RFC3339)
			}

			daySteps := dailyMsgCounts[sItem.sid+"_"+day]
			if daySteps <= 0 {
				daySteps = totalMsgCounts[sItem.sid]
			}
			if daySteps <= 0 {
				daySteps = 1
			}

			stepIdx := hashString(fmt.Sprintf("%s_%s_%s", sItem.sid, day, rawModel)) + 1
			step := StepRecord{
				AgentType:        targetEngine,
				ConversationID:   sItem.sid,
				StepIndex:        stepIdx,
				ModelName:        rawModel,
				ModelDisplayName: modDisp,
				Source:           "assistant",
				StepType:         "DAILY_USAGE",
				Status:           meta.status,
				CreatedAt:        updatedISO,
				DateBucket:       day,
				InputTokens:      inT,
				OutputTokens:     outT,
				CacheReadTokens:  cacheT,
				ToolCallsCount:   0,
				ToolNames:        "",
				ThinkingLength:   0,
			}
			steps = append(steps, step)

			title := strings.TrimSpace(meta.title)
			if title == "" {
				if targetEngine == "pi" {
					title = fmt.Sprintf("Pi 任务 #%s", sItem.sid[:8])
				} else if targetEngine == "claude" {
					title = fmt.Sprintf("Claude 任务 #%s", sItem.sid[:8])
				} else {
					title = fmt.Sprintf("Codex 任务 #%s", sItem.sid[:8])
				}
			}

			prj := meta.workingDir
			if prj == "" {
				prj = defaultProject
			}

			status := meta.status
			if status == "" {
				status = "completed"
			}

			cID := fmt.Sprintf("%s_%s_%s", sItem.sid, day, rawModel)
			conv := Conversation{
				ID:                   cID,
				ConversationID:       sItem.sid,
				AgentType:            targetEngine,
				ModelName:            rawModel,
				ModelDisplayName:     modDisp,
				Title:                title,
				Preview:              "",
				StepCount:            daySteps,
				LastModified:         updatedISO,
				ProjectID:            prj,
				AgentRole:            targetRole,
				ParentConversationID: "",
				NestingDepth:         0,
				Status:               status,
				TotalInputTokens:     inT,
				TotalOutputTokens:    outT,
				TotalCacheTokens:     cacheT,
				TotalTokens:          sItem.tokens,
				StepRecordsCount:     daySteps,
				DateBucket:           day,
				FirstStepTime:        createdISO,
				LastStepTime:         updatedISO,
			}
			convs = append(convs, conv)
			syncedCount++
		}
	}

	// 5. Fallback for sessions not covered in daily_session_usage
	for sid, meta := range sessionsMap {
		if coveredSessions[sid] || meta.totalTokens <= 0 {
			continue
		}

		sessTokens := meta.totalTokens
		staticRaw := strings.Split(meta.model, "#")[0]
		if staticRaw == "" {
			staticRaw = "gpt-6.1-sol"
		}

		type msgDay struct {
			day string
			cnt int
		}
		var sessMsgDays []msgDay
		mRows, err := db.Query(`
			SELECT strftime('%Y-%m-%d', datetime(created_at / 1000, 'unixepoch')) as day, count(*)
			FROM messages
			WHERE session_id = ?
			GROUP BY day;
		`, sid)
		if err == nil {
			for mRows.Next() {
				var md msgDay
				if err := mRows.Scan(&md.day, &md.cnt); err == nil {
					sessMsgDays = append(sessMsgDays, md)
				}
			}
			mRows.Close()
		}

		totSessMsgs := 0
		for _, md := range sessMsgDays {
			totSessMsgs += md.cnt
		}
		if len(sessMsgDays) == 0 {
			dBucket := ExtractDateBucket(fmt.Sprintf("%d", meta.createdAt))
			sessMsgDays = append(sessMsgDays, msgDay{day: dBucket, cnt: 1})
			totSessMsgs = 1
		}

		for _, md := range sessMsgDays {
			dayShare := float64(md.cnt) / float64(totSessMsgs)
			curTok := int64(float64(sessTokens) * dayShare)
			if curTok <= 0 {
				continue
			}

			rawModel := staticRaw
			modDisp := FormatModelName(rawModel)
			targetEngine := ResolveCindyEngine(meta.agentKind, rawModel)
			if piSessions[sid] || strings.Contains(strings.ToLower(meta.agentKind), "pi") {
				targetEngine = "pi"
			}
			targetRole := GetEngineRole(targetEngine)

			var inT, outT, cacheT int64
			mStats := dailyModelUsage[md.day+"_"+rawModel]
			if mStats == nil {
				mStats = overallModelUsage[rawModel]
			}
			if mStats != nil && mStats.tot > 0 {
				inT = int64(float64(curTok) * (float64(mStats.in) / float64(mStats.tot)))
				outT = int64(float64(curTok) * (float64(mStats.out) / float64(mStats.tot)))
				cacheT = curTok - inT - outT
				if cacheT < 0 {
					cacheT = 0
				}
			} else {
				inT = int64(float64(curTok) * 0.05)
				outT = int64(float64(curTok) * 0.01)
				cacheT = curTok - inT - outT
				if cacheT < 0 {
					cacheT = 0
				}
			}

			dISO := md.day + "T12:00:00Z"
			stepIdx := hashString(fmt.Sprintf("%s_%s_%s", sid, md.day, rawModel)) + 1
			step := StepRecord{
				AgentType:        targetEngine,
				ConversationID:   sid,
				StepIndex:        stepIdx,
				ModelName:        rawModel,
				ModelDisplayName: modDisp,
				Source:           "assistant",
				StepType:         "SESSION_SUMMARY",
				Status:           meta.status,
				CreatedAt:        dISO,
				DateBucket:       md.day,
				InputTokens:      inT,
				OutputTokens:     outT,
				CacheReadTokens:  cacheT,
				ToolCallsCount:   0,
				ToolNames:        "",
				ThinkingLength:   0,
			}
			steps = append(steps, step)

			title := strings.TrimSpace(meta.title)
			if title == "" {
				if targetEngine == "pi" {
					title = fmt.Sprintf("Pi 任务 #%s", sid[:8])
				} else if targetEngine == "claude" {
					title = fmt.Sprintf("Claude 任务 #%s", sid[:8])
				} else {
					title = fmt.Sprintf("Codex 任务 #%s", sid[:8])
				}
			}

			prj := meta.workingDir
			if prj == "" {
				prj = defaultProject
			}
			status := meta.status
			if status == "" {
				status = "completed"
			}

			cID := fmt.Sprintf("%s_%s_%s", sid, md.day, rawModel)
			conv := Conversation{
				ID:                   cID,
				ConversationID:       sid,
				AgentType:            targetEngine,
				ModelName:            rawModel,
				ModelDisplayName:     modDisp,
				Title:                title,
				Preview:              "",
				StepCount:            md.cnt,
				LastModified:         dISO,
				ProjectID:            prj,
				AgentRole:            targetRole,
				ParentConversationID: "",
				NestingDepth:         0,
				Status:               status,
				TotalInputTokens:     inT,
				TotalOutputTokens:    outT,
				TotalCacheTokens:     cacheT,
				TotalTokens:          curTok,
				StepRecordsCount:     md.cnt,
				DateBucket:           md.day,
				FirstStepTime:        dISO,
				LastStepTime:         dISO,
			}
			convs = append(convs, conv)
			syncedCount++
		}
	}

	return convs, steps, syncedCount, nil
}
