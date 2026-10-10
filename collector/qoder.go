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

// QoderScanner scans ~/.qoder/projects/*/*.jsonl
type QoderScanner struct {
	projectsDir string
}

func NewQoderScanner(homeDir string) *QoderScanner {
	return &QoderScanner{
		projectsDir: filepath.Join(homeDir, ".qoder", "projects"),
	}
}

type qoderUsage struct {
	InputTokens          int64   `json:"input_tokens"`
	OutputTokens         int64   `json:"output_tokens"`
	CacheReadInputTokens int64   `json:"cache_read_input_tokens"`
	ContextUsageRatio    float64 `json:"context_usage_ratio"`
	Credits              float64 `json:"credits"`
}

type qoderMessage struct {
	Model   string      `json:"model"`
	Content interface{} `json:"content"`
	Usage   *qoderUsage `json:"usage"`
}

type qoderLine struct {
	Type      string        `json:"type"`
	Timestamp string        `json:"timestamp"`
	Model     string        `json:"model"`
	Message   *qoderMessage `json:"message"`
}

func (s *QoderScanner) Scan(scanStates map[string][2]float64) ([]Conversation, []StepRecord, map[string][2]float64, error) {
	newStates := make(map[string][2]float64)
	if _, err := os.Stat(s.projectsDir); os.IsNotExist(err) {
		return nil, nil, newStates, nil
	}

	var convs []Conversation
	var allSteps []StepRecord

	err := filepath.WalkDir(s.projectsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".jsonl") {
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

		cid := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		projectDir := filepath.Base(filepath.Dir(path))

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		dateGroups := make(map[string][]StepRecord)
		firstUser := ""
		sessionModel := "qfmodel"
		var prevCtx int64

		scanner := bufio.NewScanner(f)
		buf := make([]byte, 1024*1024)
		scanner.Buffer(buf, 10*1024*1024)

		for scanner.Scan() {
			lineBytes := scanner.Bytes()
			if len(strings.TrimSpace(string(lineBytes))) == 0 {
				continue
			}

			var item qoderLine
			if err := json.Unmarshal(lineBytes, &item); err != nil {
				continue
			}

			ts := item.Timestamp
			dBucket := ExtractDateBucket(ts)
			if item.Model != "" {
				sessionModel = item.Model
			}

			if item.Type == "user" && firstUser == "" && item.Message != nil {
				if sContent, ok := item.Message.Content.(string); ok {
					sContent = strings.TrimSpace(sContent)
					if len(sContent) > 60 {
						sContent = sContent[:60]
					}
					firstUser = sContent
				} else if lContent, ok := item.Message.Content.([]interface{}); ok && len(lContent) > 0 {
					if m, ok := lContent[0].(map[string]interface{}); ok {
						if t, ok := m["text"].(string); ok {
							t = strings.TrimSpace(t)
							if len(t) > 60 {
								t = t[:60]
							}
							firstUser = t
						}
					}
				}
			}

			if item.Type == "assistant" && item.Message != nil {
				rawMod := item.Message.Model
				if rawMod == "" {
					rawMod = item.Model
				}
				if rawMod == "" {
					rawMod = sessionModel
				}
				if rawMod == "auto" || rawMod == "<synthetic>" {
					rawMod = "qfmodel"
				}
				sessionModel = rawMod

				var inT, outT, cacheR int64
				var ratio, credits float64

				if item.Message.Usage != nil {
					inT = item.Message.Usage.InputTokens
					outT = item.Message.Usage.OutputTokens
					cacheR = item.Message.Usage.CacheReadInputTokens
					ratio = item.Message.Usage.ContextUsageRatio
					credits = item.Message.Usage.Credits
				}

				textLen := 0
				if lContent, ok := item.Message.Content.([]interface{}); ok {
					for _, c := range lContent {
						if cm, ok := c.(map[string]interface{}); ok {
							if th, ok := cm["thinking"].(string); ok {
								textLen += len(th)
							}
							if tx, ok := cm["text"].(string); ok {
								textLen += len(tx)
							}
						}
					}
				} else if sContent, ok := item.Message.Content.(string); ok {
					textLen = len(sContent)
				}

				if inT == 0 && outT == 0 {
					if ratio > 0 {
						ctx := int64(ratio * 200000)
						outCalc := int64(float64(textLen) * 0.75)
						if outCalc < 100 {
							outCalc = 100
						}
						outT = outCalc

						var unitCredit float64
						if ctx > 0 {
							unitCredit = (credits / float64(ctx)) * 1000000
						}

						if prevCtx > 0 && (unitCredit < 6.0 || credits < 0.15) {
							if prevCtx < ctx {
								cacheR = prevCtx
							} else {
								cacheR = ctx
							}
							rem := ctx - cacheR
							if rem < 100 {
								rem = 100
							}
							inT = rem
						} else {
							cacheR = 0
							inT = ctx
						}
						prevCtx = ctx
					} else if textLen > 0 {
						outT = int64(float64(textLen) * 0.75)
						inT = outT * 4
					}
				} else {
					if inT > 0 {
						prevCtx = inT + cacheR
					}
				}

				if inT > 0 || outT > 0 {
					stepType := item.Type
					if stepType == "" {
						stepType = "step"
					}
					stepIdx := len(dateGroups[dBucket]) + 1
					dateGroups[dBucket] = append(dateGroups[dBucket], StepRecord{
						AgentType:        "qoder",
						ConversationID:   cid,
						StepIndex:        stepIdx,
						ModelName:        rawMod,
						ModelDisplayName: FormatModelName(rawMod),
						Source:           "assistant",
						StepType:         stepType,
						Status:           "DONE",
						CreatedAt:        ts,
						DateBucket:       dBucket,
						InputTokens:      inT,
						OutputTokens:     outT,
						CacheReadTokens:  cacheR,
						ToolCallsCount:   0,
						ToolNames:        "",
						ThinkingLength:   0,
					})
				}
			}
		}

		if len(dateGroups) > 0 {
			finalTitle := firstUser
			if finalTitle == "" {
				cidShort := cid
				if len(cidShort) > 8 {
					cidShort = cidShort[:8]
				}
				finalTitle = fmt.Sprintf("Qoder 任务 #%s", cidShort)
			}

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
					AgentType:            "qoder",
					ModelName:            modName,
					ModelDisplayName:     modDisp,
					Title:                finalTitle,
					Preview:              firstUser,
					StepCount:            len(steps),
					LastModified:         lastTime,
					ProjectID:            projectDir,
					AgentRole:            "Qoder Agent",
					ParentConversationID: "",
					NestingDepth:         0,
					Status:               "completed",
					TotalInputTokens:     cIn,
					TotalOutputTokens:    cOut,
					TotalCacheTokens:     cCache,
					TotalTokens:          cIn + cOut + cCache,
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
