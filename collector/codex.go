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

// CodexScanner scans ~/.codex/sessions/**/rollout-*.jsonl
type CodexScanner struct {
	sessionsDir string
}

func NewCodexScanner(homeDir string) *CodexScanner {
	return &CodexScanner{
		sessionsDir: filepath.Join(homeDir, ".codex", "sessions"),
	}
}

type codexPayload struct {
	Model   string                 `json:"model"`
	Cwd     string                 `json:"cwd"`
	Content string                 `json:"content"`
	Info    map[string]interface{} `json:"info"`
}

type codexLine struct {
	Type      string       `json:"type"`
	Payload   codexPayload `json:"payload"`
	Timestamp string       `json:"timestamp"`
}

func (s *CodexScanner) Scan(scanStates map[string][2]float64) ([]Conversation, []StepRecord, map[string][2]float64, error) {
	newStates := make(map[string][2]float64)
	if _, err := os.Stat(s.sessionsDir); os.IsNotExist(err) {
		return nil, nil, newStates, nil
	}

	var convs []Conversation
	var allSteps []StepRecord

	err := filepath.WalkDir(s.sessionsDir, func(path string, d fs.DirEntry, err error) error {
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

		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		cid := strings.TrimPrefix(base, "rollout-")
		sessionModel := "gpt-6.1-sol"
		title := ""

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

			var item codexLine
			if err := json.Unmarshal(lineBytes, &item); err != nil {
				continue
			}

			ts := item.Timestamp
			dBucket := ExtractDateBucket(ts)
			if item.Payload.Model != "" {
				sessionModel = item.Payload.Model
			}

			if item.Type == "session_meta" && title == "" {
				if item.Payload.Cwd != "" {
					title = filepath.Base(item.Payload.Cwd)
				}
			}

			if item.Type == "response_item" && title == "" {
				cnt := strings.TrimSpace(item.Payload.Content)
				if len(cnt) > 1 {
					if len(cnt) > 40 {
						cnt = cnt[:40]
					}
					title = cnt
				}
			}

			if item.Type == "event_msg" && item.Payload.Info != nil {
				if m, ok := item.Payload.Info["model"].(string); ok && m != "" {
					sessionModel = m
				}

				if lastU, ok := item.Payload.Info["last_token_usage"].(map[string]interface{}); ok {
					var inp, out, cache int64
					if v, ok := lastU["input_tokens"].(float64); ok {
						inp = int64(v)
					}
					if v, ok := lastU["output_tokens"].(float64); ok {
						out = int64(v)
					}
					if v, ok := lastU["cached_input_tokens"].(float64); ok {
						cache = int64(v)
					}

					if inp > 0 || out > 0 {
						var uncachedIn int64 = inp
						if cache > 0 {
							uncachedIn = inp - cache
							if uncachedIn < 0 {
								uncachedIn = 0
							}
						}

						stepIdx := len(dateGroups[dBucket]) + 1
						dispModel := FormatModelName(sessionModel)

						dateGroups[dBucket] = append(dateGroups[dBucket], StepRecord{
							AgentType:        "codex",
							ConversationID:   cid,
							StepIndex:        stepIdx,
							ModelName:        sessionModel,
							ModelDisplayName: dispModel,
							Source:           "assistant",
							StepType:         item.Type,
							Status:           "DONE",
							CreatedAt:        ts,
							DateBucket:       dBucket,
							InputTokens:      uncachedIn,
							OutputTokens:     out,
							CacheReadTokens:  cache,
							ToolCallsCount:   0,
							ToolNames:        "",
							ThinkingLength:   0,
						})
					}
				}
			}
		}

		if len(dateGroups) > 0 {
			finalTitle := title
			if finalTitle == "" {
				cidShort := cid
				if len(cidShort) > 8 {
					cidShort = cidShort[:8]
				}
				finalTitle = fmt.Sprintf("Codex 任务 #%s", cidShort)
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
				dispModel := FormatModelName(sessionModel)

				convs = append(convs, Conversation{
					ID:                   cID,
					ConversationID:       cid,
					AgentType:            "codex",
					ModelName:            sessionModel,
					ModelDisplayName:     dispModel,
					Title:                finalTitle,
					Preview:              "",
					StepCount:            len(steps),
					LastModified:         lastTime,
					ProjectID:            "Codex Workspace",
					AgentRole:            "Codex",
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
