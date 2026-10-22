package collector

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
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
	pRows, err := db.Query("PRAGMA table_info(sessions);")
	if err == nil {
		for pRows.Next() {
			var cid, notnull, pk int
			var name, ctype string
			var dfltValue sql.NullString
			if err := pRows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
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

	// 2. Read messages for accurate turn-level tokens and models
	type msgGroup struct {
		sid       string
		day       string
		model     string
		inT       int64
		outT      int64
		cacheT    int64
		tot       int64
		steps     int
		firstTime int64
		lastTime  int64
	}

	msgGroups := make(map[string]*msgGroup)
	coveredSessions := make(map[string]bool)

	mRows, err := db.Query(`
		SELECT 
			session_id, 
			strftime('%Y-%m-%d', datetime(created_at / 1000, 'unixepoch', 'localtime')) as day, 
			role,
			agent_meta,
			created_at
		FROM messages 
		ORDER BY created_at ASC;
	`)
	if err == nil {
		type metaPayload struct {
			Model            string `json:"model"`
			Usage            *struct {
				Input       int64 `json:"input"`
				Output      int64 `json:"output"`
				CacheRead   int64 `json:"cacheRead"`
				TotalTokens int64 `json:"totalTokens"`
			} `json:"usage"`
			TurnUsageDetails *struct {
				Model           string `json:"model"`
				InputTokens     int64  `json:"inputTokens"`
				OutputTokens    int64  `json:"outputTokens"`
				CacheReadTokens int64  `json:"cacheReadTokens"`
				TotalTokens     int64  `json:"totalTokens"`
			} `json:"turnUsageDetails"`
		}

		for mRows.Next() {
			var sid, day, role string
			var metaStr sql.NullString
			var createdAt int64
			if err := mRows.Scan(&sid, &day, &role, &metaStr, &createdAt); err != nil {
				continue
			}

			if role != "assistant" || !metaStr.Valid || len(metaStr.String) == 0 {
				continue
			}

			var mp metaPayload
			if err := json.Unmarshal([]byte(metaStr.String), &mp); err != nil {
				continue
			}

			rawM := ""
			var inT, outT, cacheT int64
			if mp.TurnUsageDetails != nil && mp.TurnUsageDetails.Model != "" {
				rawM = mp.TurnUsageDetails.Model
				inT = mp.TurnUsageDetails.InputTokens
				outT = mp.TurnUsageDetails.OutputTokens
				cacheT = mp.TurnUsageDetails.CacheReadTokens
			} else if mp.Model != "" && mp.Usage != nil {
				rawM = mp.Model
				inT = mp.Usage.Input
				outT = mp.Usage.Output
				cacheT = mp.Usage.CacheRead
			}

			if rawM != "" && (inT+outT+cacheT > 0) {
				cleanM := strings.Split(rawM, "#")[0]
				k := fmt.Sprintf("%s_%s_%s", sid, day, cleanM)
				g, exists := msgGroups[k]
				if !exists {
					g = &msgGroup{
						sid:       sid,
						day:       day,
						model:     cleanM,
						firstTime: createdAt,
						lastTime:  createdAt,
					}
					msgGroups[k] = g
				}
				g.inT += inT
				g.outT += outT
				g.cacheT += cacheT
				g.tot += (inT + outT + cacheT)
				g.steps++
				if createdAt > g.lastTime {
					g.lastTime = createdAt
				}
				if createdAt < g.firstTime && createdAt > 0 {
					g.firstTime = createdAt
				}
				coveredSessions[sid] = true
			}
		}
		mRows.Close()
	}

	var convs []Conversation
	var steps []StepRecord
	syncedCount := 0

	// 3. Emit accurate records from message groups
	for _, g := range msgGroups {
		meta, hasMeta := sessionsMap[g.sid]
		rawModel := g.model
		modDisp := FormatModelName(rawModel)

		agentKind := ""
		if hasMeta {
			agentKind = meta.agentKind
		}
		targetEngine := ResolveCindyEngine(agentKind, rawModel)
		if piSessions[g.sid] || strings.Contains(strings.ToLower(agentKind), "pi") {
			targetEngine = "pi"
		}
		targetRole := GetEngineRole(targetEngine)

		updatedISO := g.day + "T12:00:00Z"
		if g.lastTime > 0 {
			updatedISO = time.Unix(g.lastTime/1000, 0).UTC().Format(time.RFC3339)
		}
		createdISO := updatedISO
		if g.firstTime > 0 {
			createdISO = time.Unix(g.firstTime/1000, 0).UTC().Format(time.RFC3339)
		} else if hasMeta && meta.createdAt > 0 {
			createdISO = time.Unix(meta.createdAt/1000, 0).UTC().Format(time.RFC3339)
		}

		daySteps := g.steps
		if daySteps <= 0 {
			daySteps = 1
		}

		stepIdx := hashString(fmt.Sprintf("%s_%s_%s", g.sid, g.day, rawModel)) + 1
		step := StepRecord{
			AgentType:        targetEngine,
			ConversationID:   g.sid,
			StepIndex:        stepIdx,
			ModelName:        rawModel,
			ModelDisplayName: modDisp,
			Source:           "assistant",
			StepType:         "DAILY_USAGE",
			Status:           meta.status,
			CreatedAt:        updatedISO,
			DateBucket:       g.day,
			InputTokens:      g.inT,
			OutputTokens:     g.outT,
			CacheReadTokens:  g.cacheT,
			ToolCallsCount:   0,
			ToolNames:        "",
			ThinkingLength:   0,
		}
		steps = append(steps, step)

		title := ""
		if hasMeta {
			title = strings.TrimSpace(meta.title)
		}
		if title == "" {
			if targetEngine == "pi" {
				title = fmt.Sprintf("Pi 任务 #%s", g.sid[:8])
			} else if targetEngine == "claude" {
				title = fmt.Sprintf("Claude 任务 #%s", g.sid[:8])
			} else {
				title = fmt.Sprintf("Codex 任务 #%s", g.sid[:8])
			}
		}

		prj := defaultProject
		if hasMeta && meta.workingDir != "" {
			prj = meta.workingDir
		}

		status := "completed"
		if hasMeta && meta.status != "" {
			status = meta.status
		}

		cID := fmt.Sprintf("%s_%s_%s", g.sid, g.day, rawModel)
		conv := Conversation{
			ID:                   cID,
			ConversationID:       g.sid,
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
			TotalInputTokens:     g.inT,
			TotalOutputTokens:    g.outT,
			TotalCacheTokens:     g.cacheT,
			TotalTokens:          g.tot,
			StepRecordsCount:     daySteps,
			DateBucket:           g.day,
			FirstStepTime:        createdISO,
			LastStepTime:         updatedISO,
		}
		convs = append(convs, conv)
		syncedCount++
	}

	// 4. Fallback for sessions not covered by messages with usage
	type dsuItem struct {
		day    string
		tokens int64
		updAt  int64
	}
	dsuBySession := make(map[string][]dsuItem)
	dsuRows, err := db.Query("SELECT session_id, day, tokens, updated_at FROM daily_session_usage;")
	if err == nil {
		for dsuRows.Next() {
			var sid, day string
			var tokens, updAt sql.NullInt64
			if err := dsuRows.Scan(&sid, &day, &tokens, &updAt); err == nil && tokens.Int64 > 0 {
				dsuBySession[sid] = append(dsuBySession[sid], dsuItem{
					day:    day,
					tokens: tokens.Int64,
					updAt:  updAt.Int64,
				})
			}
		}
		dsuRows.Close()
	}

	for sid, meta := range sessionsMap {
		if coveredSessions[sid] || meta.totalTokens <= 0 {
			continue
		}

		rawModel := strings.Split(meta.model, "#")[0]
		if rawModel == "" {
			rawModel = "gpt-6.1-sol"
		}
		modDisp := FormatModelName(rawModel)
		targetEngine := ResolveCindyEngine(meta.agentKind, rawModel)
		if piSessions[sid] || strings.Contains(strings.ToLower(meta.agentKind), "pi") {
			targetEngine = "pi"
		}
		targetRole := GetEngineRole(targetEngine)

		title := strings.TrimSpace(meta.title)
		if title == "" {
			title = fmt.Sprintf("任务 #%s", sid[:8])
		}
		prj := meta.workingDir
		if prj == "" {
			prj = defaultProject
		}
		status := meta.status
		if status == "" {
			status = "completed"
		}

		dsuList, hasDSU := dsuBySession[sid]
		if hasDSU && len(dsuList) > 0 {
			for _, item := range dsuList {
				inT := int64(float64(item.tokens) * 0.05)
				outT := int64(float64(item.tokens) * 0.01)
				cacheT := item.tokens - inT - outT
				if cacheT < 0 {
					cacheT = 0
				}
				dISO := item.day + "T12:00:00Z"
				if item.updAt > 0 {
					dISO = time.Unix(item.updAt/1000, 0).UTC().Format(time.RFC3339)
				}
				stepIdx := hashString(fmt.Sprintf("%s_%s_%s", sid, item.day, rawModel)) + 1
				step := StepRecord{
					AgentType:        targetEngine,
					ConversationID:   sid,
					StepIndex:        stepIdx,
					ModelName:        rawModel,
					ModelDisplayName: modDisp,
					Source:           "assistant",
					StepType:         "SESSION_FALLBACK",
					Status:           status,
					CreatedAt:        dISO,
					DateBucket:       item.day,
					InputTokens:      inT,
					OutputTokens:     outT,
					CacheReadTokens:  cacheT,
				}
				steps = append(steps, step)

				cID := fmt.Sprintf("%s_%s_%s", sid, item.day, rawModel)
				conv := Conversation{
					ID:                   cID,
					ConversationID:       sid,
					AgentType:            targetEngine,
					ModelName:            rawModel,
					ModelDisplayName:     modDisp,
					Title:                title,
					StepCount:            1,
					LastModified:         dISO,
					ProjectID:            prj,
					AgentRole:            targetRole,
					Status:               status,
					TotalInputTokens:     inT,
					TotalOutputTokens:    outT,
					TotalCacheTokens:     cacheT,
					TotalTokens:          item.tokens,
					StepRecordsCount:     1,
					DateBucket:           item.day,
					FirstStepTime:        dISO,
					LastStepTime:         dISO,
				}
				convs = append(convs, conv)
				syncedCount++
			}
		} else {
			dBucket := ExtractDateBucket(fmt.Sprintf("%d", meta.createdAt))
			inT := int64(float64(meta.totalTokens) * 0.05)
			outT := int64(float64(meta.totalTokens) * 0.01)
			cacheT := meta.totalTokens - inT - outT
			if cacheT < 0 {
				cacheT = 0
			}
			dISO := dBucket + "T12:00:00Z"
			if meta.updatedAt > 0 {
				dISO = time.Unix(meta.updatedAt/1000, 0).UTC().Format(time.RFC3339)
			}
			createdISO := dISO
			if meta.createdAt > 0 {
				createdISO = time.Unix(meta.createdAt/1000, 0).UTC().Format(time.RFC3339)
			}

			stepIdx := hashString(fmt.Sprintf("%s_%s_%s", sid, dBucket, rawModel)) + 1
			step := StepRecord{
				AgentType:        targetEngine,
				ConversationID:   sid,
				StepIndex:        stepIdx,
				ModelName:        rawModel,
				ModelDisplayName: modDisp,
				Source:           "assistant",
				StepType:         "SESSION_FALLBACK",
				Status:           status,
				CreatedAt:        dISO,
				DateBucket:       dBucket,
				InputTokens:      inT,
				OutputTokens:     outT,
				CacheReadTokens:  cacheT,
			}
			steps = append(steps, step)

			cID := fmt.Sprintf("%s_%s_%s", sid, dBucket, rawModel)
			conv := Conversation{
				ID:                   cID,
				ConversationID:       sid,
				AgentType:            targetEngine,
				ModelName:            rawModel,
				ModelDisplayName:     modDisp,
				Title:                title,
				StepCount:            1,
				LastModified:         dISO,
				ProjectID:            prj,
				AgentRole:            targetRole,
				Status:               status,
				TotalInputTokens:     inT,
				TotalOutputTokens:    outT,
				TotalCacheTokens:     cacheT,
				TotalTokens:          meta.totalTokens,
				StepRecordsCount:     1,
				DateBucket:           dBucket,
				FirstStepTime:        createdISO,
				LastStepTime:         dISO,
			}
			convs = append(convs, conv)
			syncedCount++
		}
	}

	return convs, steps, syncedCount, nil
}
