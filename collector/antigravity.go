package collector

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type AntigravityScanner struct {
	BaseDir string
}

func (s *AntigravityScanner) Scan(scanStates map[string][2]float64) ([]Conversation, []StepRecord, map[string][2]float64, error) {
	sumDB := filepath.Join(s.BaseDir, "conversation_summaries.db")
	brainDir := filepath.Join(s.BaseDir, "brain")
	if _, err := os.Stat(sumDB); os.IsNotExist(err) {
		return nil, nil, make(map[string][2]float64), nil
	}

	defaultModel := "gemini-3.8-flash"
	pbtxtPath := filepath.Join(s.BaseDir, "antigravity_state.pbtxt")
	if b, err := os.ReadFile(pbtxtPath); err == nil {
		lines := strings.Split(string(b), "\n")
		for _, l := range lines {
			if strings.Contains(l, "last_selected_agent_model") {
				parts := strings.Split(l, ":")
				if len(parts) > 1 {
					raw := strings.Trim(strings.TrimSpace(parts[1]), `"`)
					lowerRaw := strings.ToLower(raw)
					if strings.Contains(lowerRaw, "m318") || strings.Contains(lowerRaw, "placeholder") {
						defaultModel = "gemini-3.8-flash"
					} else if raw != "" {
						defaultModel = raw
					}
				}
			}
		}
	}

	// Connect to conversation_summaries.db
	db, err := sql.Open("sqlite", sumDB+"?mode=ro")
	if err != nil {
		return nil, nil, nil, err
	}
	defer db.Close()

	rows, err := db.Query("SELECT conversation_id, title, preview, step_count, last_modified_time, project_id, agent_name, parent_conversation_id, nesting_depth, status FROM conversation_summaries")
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()

	type convMeta struct {
		cid          string
		title        string
		preview      string
		stepCount    int
		lastModified string
		projectID    string
		agentName    string
		parentCID    string
		nestingDepth int
		status       string
	}

	var summaries []convMeta
	for rows.Next() {
		var m convMeta
		var t, p, lm, pid, an, pcid, st sql.NullString
		var sc, nd sql.NullInt64
		if err := rows.Scan(&m.cid, &t, &p, &sc, &lm, &pid, &an, &pcid, &nd, &st); err == nil {
			m.title = t.String
			m.preview = p.String
			m.stepCount = int(sc.Int64)
			m.lastModified = lm.String
			m.projectID = pid.String
			m.agentName = an.String
			m.parentCID = pcid.String
			m.nestingDepth = int(nd.Int64)
			m.status = st.String
			summaries = append(summaries, m)
		}
	}

	var allConvs []Conversation
	var allSteps []StepRecord
	updatedScanStates := make(map[string][2]float64)

	for _, meta := range summaries {
		tFile := filepath.Join(brainDir, meta.cid, ".system_generated", "logs", "transcript.jsonl")
		st, err := os.Stat(tFile)
		if err != nil {
			continue
		}

		mtime := float64(st.ModTime().UnixNano()) / 1e9
		size := st.Size()
		lastState, exists := scanStates[tFile]
		if exists && lastState[0] == mtime && int64(lastState[1]) == size {
			continue
		}

		f, err := os.Open(tFile)
		if err != nil {
			continue
		}

		dateGroups := make(map[string][]StepRecord)
		scanner := bufio.NewScanner(f)
		buf := make([]byte, 0, 1024*1024)
		scanner.Buffer(buf, 10*1024*1024)

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var item map[string]interface{}
			if err := json.Unmarshal([]byte(line), &item); err != nil {
				continue
			}

			inpRaw, hasInp := item["input_tokens"]
			if !hasInp || inpRaw == nil {
				continue
			}

			created, _ := item["created_at"].(string)
			dBucket := ExtractDateBucket(created)

			var inTokens, outTokens, cacheTokens int64
			if v, ok := inpRaw.(float64); ok {
				inTokens = int64(v)
			}
			if v, ok := item["output_tokens"].(float64); ok {
				outTokens = int64(v)
			}
			if v, ok := item["cache_read_tokens"].(float64); ok {
				cacheTokens = int64(v)
			}

			stepIdx := 0
			if v, ok := item["step_index"].(float64); ok {
				stepIdx = int(v)
			}

			source, _ := item["source"].(string)
			stType, _ := item["type"].(string)
			status, _ := item["status"].(string)
			thinking, _ := item["thinking"].(string)

			toolCalls, _ := item["tool_calls"].([]interface{})
			toolCount := len(toolCalls)
			var toolNamesList []string
			for _, tc := range toolCalls {
				if tcm, ok := tc.(map[string]interface{}); ok {
					if name, ok := tcm["name"].(string); ok && name != "" {
						toolNamesList = append(toolNamesList, name)
					} else if fn, ok := tcm["function"].(map[string]interface{}); ok {
						if fnName, ok := fn["name"].(string); ok && fnName != "" {
							toolNamesList = append(toolNamesList, fnName)
						}
					}
				}
				if len(toolNamesList) >= 5 {
					break
				}
			}

			step := StepRecord{
				AgentType:        "antigravity",
				ConversationID:   meta.cid,
				StepIndex:        stepIdx,
				ModelName:        defaultModel,
				ModelDisplayName: FormatModelName(defaultModel),
				Source:           source,
				StepType:         stType,
				Status:           status,
				CreatedAt:        created,
				DateBucket:       dBucket,
				InputTokens:      inTokens,
				OutputTokens:     outTokens,
				CacheReadTokens:  cacheTokens,
				ToolCallsCount:   toolCount,
				ToolNames:        strings.Join(toolNamesList, ","),
				ThinkingLength:   len(thinking),
			}
			dateGroups[dBucket] = append(dateGroups[dBucket], step)
		}
		f.Close()

		title := meta.title
		if title == "" {
			title = meta.preview
		}
		if title == "" {
			title = "Antigravity 对话"
		}

		projID := meta.projectID
		if projID == "" {
			projID = "outside-of-project"
		}

		for dBucket, steps := range dateGroups {
			if len(steps) == 0 {
				continue
			}
			allSteps = append(allSteps, steps...)

			var cIn, cOut, cCache int64
			for _, s := range steps {
				cIn += s.InputTokens
				cOut += s.OutputTokens
				cCache += s.CacheReadTokens
			}

			firstT := steps[0].CreatedAt
			lastT := steps[len(steps)-1].CreatedAt
			cID := meta.cid + "_" + dBucket

			conv := Conversation{
				ID:                   cID,
				ConversationID:       meta.cid,
				AgentType:            "antigravity",
				ModelName:            defaultModel,
				ModelDisplayName:     FormatModelName(defaultModel),
				Title:                title,
				Preview:              meta.preview,
				StepCount:            len(steps),
				LastModified:         lastT,
				ProjectID:            projID,
				AgentRole:            meta.agentName,
				ParentConversationID: meta.parentCID,
				NestingDepth:         meta.nestingDepth,
				Status:               meta.status,
				TotalInputTokens:     cIn,
				TotalOutputTokens:    cOut,
				TotalCacheTokens:     cCache,
				TotalTokens:          cIn + cOut + cCache,
				StepRecordsCount:     len(steps),
				DateBucket:           dBucket,
				FirstStepTime:        firstT,
				LastStepTime:         lastT,
			}
			allConvs = append(allConvs, conv)
		}

		updatedScanStates[tFile] = [2]float64{mtime, float64(size)}
	}

	return allConvs, allSteps, updatedScanStates, nil
}
