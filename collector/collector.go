package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Database interface {
	GetScanStates(agentType string) (map[string][2]float64, error)
	UpdateScanState(filePath, agentType string, mtime float64, fileSize int64) error
	SaveBatch(convs []Conversation, steps []StepRecord) error
}

type Orchestrator struct {
	db          Database
	homeDir     string
	mu          sync.Mutex
	antigravity *AntigravityScanner
	cindy       *CindyScanner
	claude      *ClaudeScanner
	pi          *PiScanner
	codex       *CodexScanner
	zcode       *ZCodeScanner
	workbuddy   *WorkBuddyScanner
	qoder       *QoderScanner
}

func NewOrchestrator(db Database) (*Orchestrator, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("unable to determine user home directory: %w", err)
	}

	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = filepath.Join(homeDir, "AppData", "Roaming")
	}

	return &Orchestrator{
		db:          db,
		homeDir:     homeDir,
		antigravity: &AntigravityScanner{BaseDir: filepath.Join(homeDir, ".gemini", "antigravity")},
		cindy: &CindyScanner{
			GlobalDir: filepath.Join(appData, "CindyGlobal"),
			CNDir:     filepath.Join(appData, "Cindy"),
		},
		claude:    NewClaudeScanner(homeDir),
		pi:        NewPiScanner(homeDir, appData),
		codex:     NewCodexScanner(homeDir),
		zcode:     NewZCodeScanner(homeDir),
		workbuddy: NewWorkBuddyScanner(homeDir),
		qoder:     NewQoderScanner(homeDir),
	}, nil
}

// Sync runs a scan cycle across all agent sources.
func (o *Orchestrator) Sync() (*SyncResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	startTime := time.Now()
	counts := make(map[string]int)

	scanners := []struct {
		name    string
		scanFn  func(map[string][2]float64) ([]Conversation, []StepRecord, map[string][2]float64, error)
	}{
		{"antigravity", o.antigravity.Scan},
		{"cindy", o.cindy.Scan}, // Disaggregates into codex / claude / pi
		{"claude", o.claude.Scan},
		{"pi", o.pi.Scan},
		{"codex", o.codex.Scan},
		{"zcode", o.zcode.Scan},
		{"workbuddy", o.workbuddy.Scan},
		{"qoder", o.qoder.Scan},
	}

	for _, sc := range scanners {
		states, err := o.db.GetScanStates(sc.name)
		if err != nil {
			states = make(map[string][2]float64)
		}

		convs, steps, newStates, err := sc.scanFn(states)
		if err != nil {
			fmt.Printf("[%s Scanner Error]: %v\n", sc.name, err)
			continue
		}

		if len(convs) > 0 || len(steps) > 0 {
			if err := o.db.SaveBatch(convs, steps); err != nil {
				fmt.Printf("[%s SaveBatch Error]: %v\n", sc.name, err)
			}
		}

		for p, st := range newStates {
			_ = o.db.UpdateScanState(p, sc.name, st[0], int64(st[1]))
		}

		// Count conversations by actual agent_type produced
		for _, cv := range convs {
			counts[cv.AgentType]++
		}
	}

	duration := time.Since(startTime).Seconds()
	return &SyncResult{
		Status:          "ok",
		DurationSeconds: duration,
		Counts:          counts,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	}, nil
}
