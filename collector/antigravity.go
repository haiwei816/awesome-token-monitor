package collector

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

var (
	regexpModelEnum = regexp.MustCompile(`model_enum\x12.([A-Za-z0-9_]+)`)
	regexpLastStep  = regexp.MustCompile(`last_step_index\x12.([0-9]+)`)
)

type AntigravityScanner struct {
	BaseDir string
}

type stepModelRange struct {
	lastStepIndex int
	modelName     string
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
					if strings.Contains(lowerRaw, "m417") || strings.Contains(lowerRaw, "m408") || strings.Contains(lowerRaw, "argon") || strings.Contains(lowerRaw, "gemini-4") {
						defaultModel = "gemini-4-argon"
					} else if strings.Contains(lowerRaw, "m401") || strings.Contains(lowerRaw, "opus-5") {
						defaultModel = "claude-opus-5-5"
					} else if strings.Contains(lowerRaw, "m26") || strings.Contains(lowerRaw, "opus-4") {
						defaultModel = "claude-opus-4-6"
					} else if strings.Contains(lowerRaw, "m35") || strings.Contains(lowerRaw, "m36") || strings.Contains(lowerRaw, "sonnet") {
						defaultModel = "claude-sonnet-4-6"
					} else if strings.Contains(lowerRaw, "m318") || strings.Contains(lowerRaw, "m322") || strings.Contains(lowerRaw, "placeholder") {
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

		// Load step-level model ranges for this conversation
		ranges := s.loadStepModelRanges(meta.cid, meta.agentName, defaultModel)

		f, err := os.Open(tFile)
		if err != nil {
			continue
		}

		groupMap := make(map[string][]StepRecord)
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

			stepIdx := 0
			if v, ok := item["step_index"].(float64); ok {
				stepIdx = int(v)
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

			stepModel := getModelForStep(ranges, stepIdx, defaultModel)
			modDisp := FormatModelName(stepModel)

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
				ModelName:        stepModel,
				ModelDisplayName: modDisp,
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

			groupKey := dBucket + "___" + stepModel
			groupMap[groupKey] = append(groupMap[groupKey], step)
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

		for groupKey, steps := range groupMap {
			if len(steps) == 0 {
				continue
			}
			allSteps = append(allSteps, steps...)

			parts := strings.Split(groupKey, "___")
			dBucket := parts[0]
			modelName := parts[1]
			modDisp := FormatModelName(modelName)

			var cIn, cOut, cCache int64
			for _, s := range steps {
				cIn += s.InputTokens
				cOut += s.OutputTokens
				cCache += s.CacheReadTokens
			}

			firstT := steps[0].CreatedAt
			lastT := steps[len(steps)-1].CreatedAt
			cID := fmt.Sprintf("%s_%s_%s", meta.cid, dBucket, modelName)

			conv := Conversation{
				ID:                   cID,
				ConversationID:       meta.cid,
				AgentType:            "antigravity",
				ModelName:            modelName,
				ModelDisplayName:     modDisp,
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

// loadStepModelRanges reads conversations/<cid>.db (gen_metadata) to get step ranges and exact model enums
func (s *AntigravityScanner) loadStepModelRanges(cid, agentName, defaultModel string) []stepModelRange {
	if agentName == "image-generator" {
		return []stepModelRange{{lastStepIndex: 999999999, modelName: "gemini-3.1-flash-image"}}
	}

	convDBPath := filepath.Join(s.BaseDir, "conversations", cid+".db")
	if _, err := os.Stat(convDBPath); os.IsNotExist(err) {
		return []stepModelRange{{lastStepIndex: 999999999, modelName: defaultModel}}
	}

	db, err := sql.Open("sqlite", convDBPath+"?mode=ro")
	if err != nil {
		return []stepModelRange{{lastStepIndex: 999999999, modelName: defaultModel}}
	}
	defer db.Close()

	rows, err := db.Query("SELECT idx, data FROM gen_metadata WHERE data IS NOT NULL ORDER BY idx ASC")
	if err != nil {
		return []stepModelRange{{lastStepIndex: 999999999, modelName: defaultModel}}
	}
	defer rows.Close()

	var ranges []stepModelRange
	for rows.Next() {
		var rowIdx int
		var blob []byte
		if err := rows.Scan(&rowIdx, &blob); err != nil || len(blob) == 0 {
			continue
		}

		lastIdx := rowIdx
		if m := regexpLastStep.FindSubmatch(blob); len(m) > 1 {
			if val, err := strconv.Atoi(string(m[1])); err == nil {
				lastIdx = val
			}
		}

		mName := defaultModel
		if m := regexpModelEnum.FindSubmatch(blob); len(m) > 1 {
			enumVal := string(m[1])
			switch enumVal {
			case "MODEL_PLACEHOLDER_M318", "MODEL_PLACEHOLDER_M319", "MODEL_PLACEHOLDER_M322", "MODEL_PLACEHOLDER_M320", "MODEL_PLACEHOLDER_M299":
				mName = "gemini-3.8-flash"
			case "MODEL_PLACEHOLDER_M417", "MODEL_PLACEHOLDER_M408":
				mName = "gemini-4-argon"
			case "MODEL_PLACEHOLDER_M401":
				mName = "claude-opus-5-5"
			case "MODEL_PLACEHOLDER_M26":
				mName = "claude-opus-4-6"
			case "MODEL_PLACEHOLDER_M35", "MODEL_PLACEHOLDER_M36":
				mName = "claude-sonnet-4-6"
			default:
				if strings.Contains(enumVal, "GPT_OSS") {
					mName = "gpt-oss-120b"
				} else {
					mName = enumVal
				}
			}
		}

		ranges = append(ranges, stepModelRange{
			lastStepIndex: lastIdx,
			modelName:     mName,
		})
	}

	if len(ranges) == 0 {
		return []stepModelRange{{lastStepIndex: 999999999, modelName: defaultModel}}
	}

	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].lastStepIndex < ranges[j].lastStepIndex
	})

	return ranges
}

func getModelForStep(ranges []stepModelRange, stepIdx int, fallback string) string {
	for _, r := range ranges {
		if stepIdx <= r.lastStepIndex {
			return r.modelName
		}
	}
	if len(ranges) > 0 {
		return ranges[len(ranges)-1].modelName
	}
	return fallback
}
