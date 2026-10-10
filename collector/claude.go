package collector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ClaudeScanner scans ~/.claude/projects/**/*.jsonl excluding Cindy managed sessions
type ClaudeScanner struct {
	projectsDir string
}

func NewClaudeScanner(homeDir string) *ClaudeScanner {
	return &ClaudeScanner{
		projectsDir: filepath.Join(homeDir, ".claude", "projects"),
	}
}

type claudeUsage struct {
	InputTokens          int64 `json:"input_tokens"`
	OutputTokens         int64 `json:"output_tokens"`
	CacheReadInputTokens int64 `json:"cache_read_input_tokens"`
}

type claudeMessage struct {
	Model   string      `json:"model"`
	Content interface{} `json:"content"`
	Usage   *claudeUsage `json:"usage"`
}

type claudeLine struct {
	Type      string         `json:"type"`
	Timestamp string         `json:"timestamp"`
	Title     string         `json:"title"`
	Message   *claudeMessage `json:"message"`
}

func (s *ClaudeScanner) Scan(scanStates map[string][2]float64) ([]Conversation, []StepRecord, map[string][2]float64, error) {
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

		folderRaw := filepath.Base(filepath.Dir(path))
		pathLower := strings.ToLower(path)
		folderLower := strings.ToLower(folderRaw)

		// Cindy deduplication
		if strings.Contains(folderLower, "cindy") || strings.Contains(pathLower, "cindy") {
			return nil
		}

		// Check head chunk for Cindy / sdk-ts markers
		fChk, err := os.Open(path)
		if err == nil {
			headBuf := make([]byte, 25000)
			n, _ := io.ReadFull(fChk, headBuf)
			fChk.Close()
			headStr := string(headBuf[:n])
			if strings.Contains(strings.ToLower(headStr), "cindy") || strings.Contains(headStr, "sdk-ts") {
				return nil
			}
		}

		cid := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		projectPath := folderRaw
		if strings.Contains(folderRaw, "--") {
			projectPath = strings.ReplaceAll(folderRaw, "--", "/")
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		dateGroups := make(map[string][]StepRecord)
		title := ""
		firstUserPrompt := ""

		scanner := bufio.NewScanner(f)
		buf := make([]byte, 1024*1024)
		scanner.Buffer(buf, 10*1024*1024)

		for scanner.Scan() {
			lineBytes := scanner.Bytes()
			if len(strings.TrimSpace(string(lineBytes))) == 0 {
				continue
			}

			var item claudeLine
			if err := json.Unmarshal(lineBytes, &item); err != nil {
				continue
			}

			ts := item.Timestamp
			dBucket := ExtractDateBucket(ts)

			if item.Type == "ai-title" && item.Title != "" && title == "" {
				title = item.Title
			}

			if item.Type == "user" && firstUserPrompt == "" && item.Message != nil {
				if strContent, ok := item.Message.Content.(string); ok {
					lines := strings.Split(strContent, "\n")
					for _, l := range lines {
						l = strings.TrimSpace(l)
						if l != "" && !strings.HasPrefix(l, "[UI language]") && !strings.HasPrefix(l, "<system") && !strings.HasPrefix(l, "==") && !strings.HasPrefix(l, "[User]") {
							if len(l) > 60 {
								l = l[:60]
							}
							firstUserPrompt = l
							break
						}
					}
				} else if listContent, ok := item.Message.Content.([]interface{}); ok {
					for _, it := range listContent {
						if m, ok := it.(map[string]interface{}); ok && m["type"] == "text" {
							if t, ok := m["text"].(string); ok {
								t = strings.TrimSpace(t)
								if t != "" && !strings.HasPrefix(t, "[UI language]") && !strings.HasPrefix(t, "<system") && !strings.HasPrefix(t, "[Request interrupted") {
									if len(t) > 60 {
										t = t[:60]
									}
									firstUserPrompt = t
									break
								}
							}
						}
					}
				}
			}

			if item.Message != nil && item.Message.Usage != nil {
				inT := item.Message.Usage.InputTokens
				outT := item.Message.Usage.OutputTokens
				cacheR := item.Message.Usage.CacheReadInputTokens

				if inT+outT+cacheR > 0 {
					mod := item.Message.Model
					if mod == "" || mod == "<synthetic>" {
						mod = "Claude"
					}
					dispModel := FormatModelName(mod)

					stepIdx := len(dateGroups[dBucket]) + 1
					dateGroups[dBucket] = append(dateGroups[dBucket], StepRecord{
						AgentType:        "claude",
						ConversationID:   cid,
						StepIndex:        stepIdx,
						ModelName:        mod,
						ModelDisplayName: dispModel,
						Source:           "assistant",
						StepType:         item.Type,
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
			finalTitle := title
			if finalTitle == "" {
				finalTitle = firstUserPrompt
			}
			if finalTitle == "" {
				cidShort := cid
				if len(cidShort) > 8 {
					cidShort = cidShort[:8]
				}
				finalTitle = fmt.Sprintf("Claude #%s", cidShort)
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
				cID := fmt.Sprintf("%s_%s", cid, dBucket)
				rawModel := steps[len(steps)-1].ModelName
				modDisplay := strings.Join(modelList, ", ")

				convs = append(convs, Conversation{
					ID:                   cID,
					ConversationID:       cid,
					AgentType:            "claude",
					ModelName:            rawModel,
					ModelDisplayName:     modDisplay,
					Title:                finalTitle,
					Preview:              firstUserPrompt,
					StepCount:            len(steps),
					LastModified:         lastTime,
					ProjectID:            projectPath,
					AgentRole:            "Claude Code",
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

	return convs, allSteps, newStates, err
}
