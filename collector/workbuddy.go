package collector

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// WorkBuddyScanner scans ~/.workbuddy/workbuddy.db and ~/.workbuddy/projects/*/*.jsonl
type WorkBuddyScanner struct {
	dbPath      string
	projectsDir string
}

func NewWorkBuddyScanner(homeDir string) *WorkBuddyScanner {
	return &WorkBuddyScanner{
		dbPath:      filepath.Join(homeDir, ".workbuddy", "workbuddy.db"),
		projectsDir: filepath.Join(homeDir, ".workbuddy", "projects"),
	}
}

type wbSessionMeta struct {
	ID        string
	Title     string
	Model     string
	Status    string
	CreatedAt string
	UpdatedAt string
	ProjectID string
}

type wbUsage struct {
	InputTokens          int64 `json:"input_tokens"`
	OutputTokens         int64 `json:"output_tokens"`
	CacheReadInputTokens int64 `json:"cache_read_input_tokens"`
}

type wbMessage struct {
	Usage     *wbUsage `json:"usage"`
	Timestamp string   `json:"timestamp"`
	Model     string   `json:"model"`
}

type wbLine struct {
	Type      string      `json:"type"`
	Timestamp string      `json:"timestamp"`
	Model     string      `json:"model"`
	Message   *wbMessage  `json:"message"`
	Usage     *wbUsage    `json:"usage"`
}

func (s *WorkBuddyScanner) Scan(scanStates map[string][2]float64) ([]Conversation, []StepRecord, map[string][2]float64, error) {
	newStates := make(map[string][2]float64)
	if _, err := os.Stat(s.dbPath); os.IsNotExist(err) {
		return nil, nil, newStates, nil
	}

	sessMap := make(map[string]wbSessionMeta)
	dbConn, err := sql.Open("sqlite", s.dbPath+"?mode=ro")
	if err == nil {
		rows, err := dbConn.Query("SELECT id, title, model, status, created_at, updated_at, project_id FROM sessions")
		if err == nil {
			for rows.Next() {
				var sm wbSessionMeta
				var title, model, status, ca, ua, pid sql.NullString
				if err := rows.Scan(&sm.ID, &title, &model, &status, &ca, &ua, &pid); err == nil {
					sm.Title = title.String
					sm.Model = model.String
					sm.Status = status.String
					sm.CreatedAt = ca.String
					sm.UpdatedAt = ua.String
					sm.ProjectID = pid.String
					sessMap[sm.ID] = sm
				}
			}
			rows.Close()
		}
		dbConn.Close()
	}

	var convs []Conversation
	var allSteps []StepRecord
	coveredSessions := make(map[string]bool)

	if _, err := os.Stat(s.projectsDir); err == nil {
		filepath.WalkDir(s.projectsDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".jsonl") {
				return nil
			}

			cid := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
			coveredSessions[cid] = true

			info, err := d.Info()
			if err != nil {
				return nil
			}

			mtime := float64(info.ModTime().UnixNano()) / 1e9
			size := info.Size()

			if prev, ok := scanStates[path]; ok {
				if prev[0] == mtime && int64(prev[1]) == size {
					return nil
				}
			}

			metaInfo := sessMap[cid]
			fileModel := metaInfo.Model

			metaPath := strings.TrimSuffix(path, ".jsonl") + ".meta.json"
			if fileModel == "" {
				if mData, err := os.ReadFile(metaPath); err == nil {
					var mObj map[string]interface{}
					if err := json.Unmarshal(mData, &mObj); err == nil {
						if rMid, ok := mObj["responseModelId"].(string); ok && rMid != "" {
							fileModel = rMid
						} else if mid, ok := mObj["model"].(string); ok && mid != "" {
							fileModel = mid
						}
					}
				}
			}

			sessionModel := fileModel
			if sessionModel == "" {
				sessionModel = "hy3"
			}
			title := metaInfo.Title
			if title == "" {
				cidShort := cid
				if len(cidShort) > 8 {
					cidShort = cidShort[:8]
				}
				title = fmt.Sprintf("WorkBuddy 会话 #%s", cidShort)
			}

			f, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer f.Close()

			dateGroups := make(map[string][]StepRecord)
			scanner := bufio.NewScanner(f)
			buf := make([]byte, 1024*1024)
			scanner.Buffer(buf, 10*1024*1024)

			for scanner.Scan() {
				lineBytes := scanner.Bytes()
				if len(strings.TrimSpace(string(lineBytes))) == 0 {
					continue
				}

				var item wbLine
				if err := json.Unmarshal(lineBytes, &item); err != nil {
					continue
				}

				var usage *wbUsage
				if item.Message != nil && item.Message.Usage != nil {
					usage = item.Message.Usage
				} else if item.Usage != nil {
					usage = item.Usage
				}
				if usage == nil {
					continue
				}

				ts := item.Timestamp
				if ts == "" && item.Message != nil {
					ts = item.Message.Timestamp
				}
				dBucket := ExtractDateBucket(ts)

				mod := fileModel
				if mod == "" && item.Model != "" {
					mod = item.Model
				} else if mod == "" && item.Message != nil && item.Message.Model != "" {
					mod = item.Message.Model
				}
				if mod == "" {
					mod = sessionModel
				}

				inT := usage.InputTokens
				outT := usage.OutputTokens
				cacheT := usage.CacheReadInputTokens

				if inT > 0 || outT > 0 {
					var uncachedIn int64 = inT
					if cacheT > 0 {
						uncachedIn = inT - cacheT
						if uncachedIn < 0 {
							uncachedIn = 0
						}
					}

					stepType := item.Type
					if stepType == "" {
						stepType = "step"
					}

					stepIdx := len(dateGroups[dBucket]) + 1
					dateGroups[dBucket] = append(dateGroups[dBucket], StepRecord{
						AgentType:        "workbuddy",
						ConversationID:   cid,
						StepIndex:        stepIdx,
						ModelName:        mod,
						ModelDisplayName: FormatModelName(mod),
						Source:           "assistant",
						StepType:         stepType,
						Status:           "completed",
						CreatedAt:        ts,
						DateBucket:       dBucket,
						InputTokens:      uncachedIn,
						OutputTokens:     outT,
						CacheReadTokens:  cacheT,
						ToolCallsCount:   0,
						ToolNames:        "",
						ThinkingLength:   0,
					})
				}
			}

			if len(dateGroups) > 0 {
				for dBucket, steps := range dateGroups {
					if len(steps) == 0 {
						continue
					}

					var cIn, cOut, cCache int64
					for _, st := range steps {
						cIn += st.InputTokens
						cOut += st.OutputTokens
						cCache += st.CacheReadTokens
						allSteps = append(allSteps, st)
					}

					firstTime := steps[0].CreatedAt
					lastTime := steps[len(steps)-1].CreatedAt
					cID := fmt.Sprintf("%s_%s", cid, dBucket)
					modName := steps[0].ModelName
					modDisp := steps[0].ModelDisplayName

					convs = append(convs, Conversation{
						ID:                   cID,
						ConversationID:       cid,
						AgentType:            "workbuddy",
						ModelName:            modName,
						ModelDisplayName:     modDisp,
						Title:                title,
						Preview:              "",
						StepCount:            len(steps),
						LastModified:         lastTime,
						ProjectID:            "WorkBuddy",
						AgentRole:            "WorkBuddy",
						ParentConversationID: "",
						NestingDepth:         0,
						Status:               "completed",
						TotalInputTokens:     cIn,
						TotalOutputTokens:    cOut,
						TotalCacheTokens:     cCache,
						TotalTokens:          cIn + cCache + cOut,
						StepRecordsCount:     len(steps),
						DateBucket:           dBucket,
						FirstStepTime:        firstTime,
						LastStepTime:         lastTime,
					})
				}
			}

			newStates[path] = [2]float64{mtime, float64(size)}
			return nil
		})
	}

	// Fallback for sessions not covered by jsonl
	for sid, meta := range sessMap {
		if !coveredSessions[sid] {
			rawModel := meta.Model
			if rawModel == "" {
				rawModel = "hy3"
			}
			tStr := meta.UpdatedAt
			if tStr == "" {
				tStr = meta.CreatedAt
			}
			dBucket := ExtractDateBucket(tStr)
			cID := fmt.Sprintf("%s_%s", sid, dBucket)

			var usedTokens int64
			if dbConn, err := sql.Open("sqlite", s.dbPath+"?mode=ro"); err == nil {
				var used sql.NullInt64
				_ = dbConn.QueryRow("SELECT used FROM session_usage WHERE id = ?", sid).Scan(&used)
				if used.Valid {
					usedTokens = used.Int64
				}
				dbConn.Close()
			}

			title := meta.Title
			if title == "" {
				sidShort := sid
				if len(sidShort) > 8 {
					sidShort = sidShort[:8]
				}
				title = fmt.Sprintf("WorkBuddy 会话 #%s", sidShort)
			}
			status := meta.Status
			if status == "" {
				status = "completed"
			}

			convs = append(convs, Conversation{
				ID:                   cID,
				ConversationID:       sid,
				AgentType:            "workbuddy",
				ModelName:            rawModel,
				ModelDisplayName:     FormatModelName(rawModel),
				Title:                title,
				Preview:              "",
				StepCount:            1,
				LastModified:         tStr,
				ProjectID:            "WorkBuddy",
				AgentRole:            "WorkBuddy",
				ParentConversationID: "",
				NestingDepth:         0,
				Status:               status,
				TotalInputTokens:     usedTokens,
				TotalOutputTokens:    0,
				TotalCacheTokens:     0,
				TotalTokens:          usedTokens,
				StepRecordsCount:     1,
				DateBucket:           dBucket,
				FirstStepTime:        tStr,
				LastStepTime:         tStr,
			})
		}
	}

	return convs, allSteps, newStates, nil
}
