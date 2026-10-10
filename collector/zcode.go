package collector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ZCodeScanner scans ~/.zcode/cli/rollout/model-io-*.jsonl
type ZCodeScanner struct {
	rolloutDir string
}

func NewZCodeScanner(homeDir string) *ZCodeScanner {
	return &ZCodeScanner{
		rolloutDir: filepath.Join(homeDir, ".zcode", "cli", "rollout"),
	}
}

type zcodeModelInfo struct {
	ModelID string `json:"modelId"`
}

type zcodeToolCall struct {
	Name     string `json:"name"`
	ToolName string `json:"toolName"`
}

type zcodeUsage struct {
	InputTokens     int64 `json:"inputTokens"`
	OutputTokens    int64 `json:"outputTokens"`
	CacheReadTokens int64 `json:"cacheReadTokens"`
}

type zcodeResponse struct {
	Usage         zcodeUsage      `json:"usage"`
	ToolCalls     []zcodeToolCall `json:"toolCalls"`
	ReasoningText string          `json:"reasoningText"`
}

type zcodeMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type zcodeRequest struct {
	Messages []zcodeMessage `json:"messages"`
}

type zcodeLine struct {
	SessionID   string          `json:"sessionId"`
	StartedAt   string          `json:"startedAt"`
	CompletedAt string          `json:"completedAt"`
	Model       interface{}     `json:"model"`
	Response    zcodeResponse   `json:"response"`
	Request     zcodeRequest    `json:"request"`
}

func (s *ZCodeScanner) Scan(scanStates map[string][2]float64) ([]Conversation, []StepRecord, map[string][2]float64, error) {
	newStates := make(map[string][2]float64)
	if _, err := os.Stat(s.rolloutDir); os.IsNotExist(err) {
		return nil, nil, newStates, nil
	}

	var convs []Conversation
	var allSteps []StepRecord

	err := filepath.WalkDir(s.rolloutDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, "model-io-") || !strings.HasSuffix(strings.ToLower(name), ".jsonl") {
			return nil
		}

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

		base := strings.TrimSuffix(name, filepath.Ext(name))
		sidFromFile := strings.TrimPrefix(base, "model-io-")

		isSubagent := strings.Contains(sidFromFile, "subagent")
		parentID := ""
		if isSubagent {
			parentID = "sess_f9aece1d-db68-43aa-97e3-1cd280d71ad9"
		}
		agentRole := "ZCode"
		if isSubagent {
			agentRole = "ZCode Subagent"
		}
		nestingDepth := 0
		if isSubagent {
			nestingDepth = 1
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		dateGroups := make(map[string][]StepRecord)
		firstUserPrompt := ""

		scanner := bufio.NewScanner(f)
		buf := make([]byte, 1024*1024)
		scanner.Buffer(buf, 10*1024*1024)

		for scanner.Scan() {
			lineBytes := scanner.Bytes()
			if len(strings.TrimSpace(string(lineBytes))) == 0 {
				continue
			}

			var item zcodeLine
			if err := json.Unmarshal(lineBytes, &item); err != nil {
				continue
			}

			sid := item.SessionID
			if sid == "" {
				sid = sidFromFile
			}

			ts := item.StartedAt
			if ts == "" {
				ts = item.CompletedAt
			}
			dBucket := ExtractDateBucket(ts)

			rawModel := "GLM-5.3-Flash"
			if mStr, ok := item.Model.(string); ok && mStr != "" {
				rawModel = mStr
			} else if mMap, ok := item.Model.(map[string]interface{}); ok {
				if id, ok := mMap["modelId"].(string); ok && id != "" {
					rawModel = id
				}
			}
			dispModel := FormatModelName(rawModel)

			toolCount := len(item.Response.ToolCalls)
			var toolNames []string
			for idx, tc := range item.Response.ToolCalls {
				if idx >= 5 {
					break
				}
				n := tc.Name
				if n == "" {
					n = tc.ToolName
				}
				if n != "" {
					toolNames = append(toolNames, n)
				}
			}
			toolNamesStr := strings.Join(toolNames, ",")
			thinkingLen := len(item.Response.ReasoningText)

			if firstUserPrompt == "" {
				for _, msg := range item.Request.Messages {
					if msg.Role == "user" {
						cText := fmt.Sprintf("%v", msg.Content)
						if strings.Contains(cText, "</system-reminder>") {
							parts := strings.SplitN(cText, "</system-reminder>", 2)
							cText = parts[1]
						}
						cText = strings.TrimSpace(cText)
						if strings.HasPrefix(cText, "http") {
							parts := strings.SplitN(cText, " ", 2)
							if len(parts) > 1 {
								cText = strings.TrimSpace(parts[1])
							}
						}
						if cText != "" {
							if len(cText) > 60 {
								cText = cText[:60]
							}
							firstUserPrompt = cText
							break
						}
					}
				}
			}

			stepIdx := len(dateGroups[dBucket]) + 1
			dateGroups[dBucket] = append(dateGroups[dBucket], StepRecord{
				AgentType:        "zcode",
				ConversationID:   sid,
				StepIndex:        stepIdx,
				ModelName:        rawModel,
				ModelDisplayName: dispModel,
				Source:           "assistant",
				StepType:         "model_io",
				Status:           "DONE",
				CreatedAt:        ts,
				DateBucket:       dBucket,
				InputTokens:      item.Response.Usage.InputTokens,
				OutputTokens:     item.Response.Usage.OutputTokens,
				CacheReadTokens:  item.Response.Usage.CacheReadTokens,
				ToolCallsCount:   toolCount,
				ToolNames:        toolNamesStr,
				ThinkingLength:   thinkingLen,
			})
		}

		if len(dateGroups) > 0 {
			finalTitle := firstUserPrompt
			if finalTitle == "" {
				if isSubagent {
					finalTitle = "ZCode 简历审查与生成"
				} else {
					finalTitle = "ZCode 会话"
				}
			}

			for dBucket, steps := range dateGroups {
				if len(steps) == 0 {
					continue
				}

				var cIn, cOut, cCache int64
				distinctModels := make(map[string]bool)
				var modelList []string

				for _, st := range steps {
					cIn += st.InputTokens
					cOut += st.OutputTokens
					cCache += st.CacheReadTokens
					allSteps = append(allSteps, st)

					if !distinctModels[st.ModelDisplayName] {
						distinctModels[st.ModelDisplayName] = true
						modelList = append(modelList, st.ModelDisplayName)
					}
				}

				firstTime := steps[0].CreatedAt
				lastTime := steps[len(steps)-1].CreatedAt
				cID := fmt.Sprintf("%s_%s", sidFromFile, dBucket)
				rawModel := steps[len(steps)-1].ModelName
				modDisplay := strings.Join(modelList, ", ")

				convs = append(convs, Conversation{
					ID:                   cID,
					ConversationID:       sidFromFile,
					AgentType:            "zcode",
					ModelName:            rawModel,
					ModelDisplayName:     modDisplay,
					Title:                finalTitle,
					Preview:              firstUserPrompt,
					StepCount:            len(steps),
					LastModified:         lastTime,
					ProjectID:            "ZCode-Default",
					AgentRole:            agentRole,
					ParentConversationID: parentID,
					NestingDepth:         nestingDepth,
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

	return convs, allSteps, newStates, err
}
