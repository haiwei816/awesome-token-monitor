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

// PiScanner scans Pi coding agent sessions from Cindy pi-agent-home and ~/.pi/sessions
type PiScanner struct {
	piPaths []string
}

func NewPiScanner(homeDir, appData string) *PiScanner {
	if appData == "" {
		appData = filepath.Join(homeDir, "AppData", "Roaming")
	}
	return &PiScanner{
		piPaths: []string{
			filepath.Join(homeDir, ".pi", "agent", "sessions"),
			filepath.Join(homeDir, ".pi", "sessions"),
			filepath.Join(appData, "Cindy", "pi-agent-home", "sessions"),
			filepath.Join(appData, "CindyGlobal", "pi-agent-home", "sessions"),
			filepath.Join(appData, "Cindy", "pi-agent-home"),
			filepath.Join(appData, "CindyGlobal", "pi-agent-home"),
		},
	}
}

type piUsage struct {
	InputTokens          int64 `json:"input_tokens"`
	OutputTokens         int64 `json:"output_tokens"`
	CacheReadInputTokens int64 `json:"cache_read_input_tokens"`
}

type piLine struct {
	Type      string   `json:"type"`
	Timestamp string   `json:"timestamp"`
	Model     string   `json:"model"`
	Usage     *piUsage `json:"usage"`
}

func (s *PiScanner) Scan(scanStates map[string][2]float64) ([]Conversation, []StepRecord, map[string][2]float64, error) {
	newStates := make(map[string][2]float64)
	var convs []Conversation
	var allSteps []StepRecord

	for _, p := range s.piPaths {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			continue
		}

		filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".jsonl") {
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

				var item piLine
				if err := json.Unmarshal(lineBytes, &item); err != nil {
					continue
				}

				if item.Usage == nil {
					continue
				}

				ts := item.Timestamp
				dBucket := ExtractDateBucket(ts)
				mod := item.Model
				if mod == "" {
					mod = "Pi Agent"
				}

				inT := item.Usage.InputTokens
				outT := item.Usage.OutputTokens
				cacheT := item.Usage.CacheReadInputTokens

				if inT > 0 || outT > 0 {
					var uncachedIn int64 = inT
					if cacheT > 0 {
						uncachedIn = inT - cacheT
						if uncachedIn < 0 {
							uncachedIn = 0
						}
					}

					stepIdx := len(dateGroups[dBucket]) + 1
					dateGroups[dBucket] = append(dateGroups[dBucket], StepRecord{
						AgentType:        "pi",
						ConversationID:   cid,
						StepIndex:        stepIdx,
						ModelName:        mod,
						ModelDisplayName: FormatModelName(mod),
						Source:           "assistant",
						StepType:         item.Type,
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
				title := fmt.Sprintf("Pi 会话 #%s", cid)
				if len(cid) > 8 {
					title = fmt.Sprintf("Pi 会话 #%s", cid[:8])
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
						AgentType:            "pi",
						ModelName:            modName,
						ModelDisplayName:     modDisp,
						Title:                title,
						Preview:              "",
						StepCount:            len(steps),
						LastModified:         lastTime,
						ProjectID:            "Pi Agent",
						AgentRole:            "Pi Coding Agent",
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

	return convs, allSteps, newStates, nil
}
