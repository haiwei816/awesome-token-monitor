package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"antigravity-token-monitor/collector"
	"antigravity-token-monitor/db"
)

type Server struct {
	port         int
	webDir       string
	db           *db.DB
	orchestrator *collector.Orchestrator
}

func NewServer(port int, webDir string, database *db.DB, orchestrator *collector.Orchestrator) *Server {
	return &Server{
		port:         port,
		webDir:       webDir,
		db:           database,
		orchestrator: orchestrator,
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Static web assets
	fileServer := http.FileServer(http.Dir(s.webDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.handleAPI(w, r)
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "" {
			http.ServeFile(w, r, filepath.Join(s.webDir, "index.html"))
			return
		}
		// Check if file exists, else fallback to index.html
		filePath := filepath.Join(s.webDir, filepath.Clean(r.URL.Path))
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			http.ServeFile(w, r, filepath.Join(s.webDir, "index.html"))
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	addr := fmt.Sprintf(":%d", s.port)
	fmt.Printf("================================================================\n")
	fmt.Printf("  Universal AI Agent Token & Model Monitor (Go Edition)\n")
	fmt.Printf("  Web Dashboard: http://localhost:%d\n", s.port)
	fmt.Printf("  Supported Agents: Antigravity, Cindy (Codex/Claude/Pi), Codex, WorkBuddy, Qoder, ZCode\n")
	fmt.Printf("================================================================\n")

	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	path := r.URL.Path
	query := r.URL.Query()

	if r.Method == http.MethodPost && path == "/api/scan" {
		res, err := s.orchestrator.Sync()
		if err != nil {
			s.sendJSON(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
			return
		}
		s.sendJSON(w, res, http.StatusOK)
		return
	}

	if r.Method != http.MethodGet {
		s.sendJSON(w, map[string]string{"error": "Method not allowed"}, http.StatusMethodNotAllowed)
		return
	}

	agent := query.Get("agent")
	model := query.Get("model")
	date := query.Get("date")

	switch {
	case path == "/api/summary":
		data, err := s.db.GetSummary(agent, model, date)
		if err != nil {
			s.sendJSON(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
			return
		}
		s.sendJSON(w, data, http.StatusOK)

	case path == "/api/daily":
		limit, _ := strconv.Atoi(query.Get("limit"))
		if limit <= 0 {
			limit = 60
		}
		data, err := s.db.GetDaily(agent, model, limit)
		if err != nil {
			s.sendJSON(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
			return
		}
		s.sendJSON(w, data, http.StatusOK)

	case path == "/api/trends":
		granularity := query.Get("granularity")
		if granularity == "" {
			granularity = "day"
		}
		data, err := s.db.GetTrends(agent, granularity, model, date)
		if err != nil {
			s.sendJSON(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
			return
		}
		s.sendJSON(w, data, http.StatusOK)

	case path == "/api/conversations":
		sortCol := query.Get("sort")
		if sortCol == "" {
			sortCol = "total_tokens"
		}
		order := query.Get("order")
		if order == "" {
			order = "desc"
		}
		search := query.Get("search")
		limit, _ := strconv.Atoi(query.Get("limit"))
		if limit <= 0 {
			limit = 50
		}
		offset, _ := strconv.Atoi(query.Get("offset"))
		if offset < 0 {
			offset = 0
		}

		data, err := s.db.GetConversations(agent, model, date, sortCol, order, search, limit, offset)
		if err != nil {
			s.sendJSON(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
			return
		}
		s.sendJSON(w, data, http.StatusOK)

	case strings.HasPrefix(path, "/api/conversation/"):
		cid := strings.TrimPrefix(path, "/api/conversation/")
		data, err := s.db.GetConversationDetail(cid)
		if err != nil {
			s.sendJSON(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
			return
		}
		if data == nil {
			s.sendJSON(w, map[string]string{"error": "Conversation not found"}, http.StatusNotFound)
			return
		}
		s.sendJSON(w, data, http.StatusOK)

	case path == "/api/models":
		data, err := s.db.GetModelsList()
		if err != nil {
			s.sendJSON(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
			return
		}
		s.sendJSON(w, data, http.StatusOK)

	case path == "/api/export":
		format := query.Get("format")
		if format == "csv" {
			s.handleExportCSV(w, agent, model, date)
		} else {
			s.handleExportJSON(w, agent, model, date)
		}

	default:
		s.sendJSON(w, map[string]string{"error": "API route not found"}, http.StatusNotFound)
	}
}

func (s *Server) handleExportCSV(w http.ResponseWriter, agent, model, date string) {
	convMap, err := s.db.GetConversations(agent, model, date, "total_tokens", "desc", "", 10000, 0)
	if err != nil {
		s.sendJSON(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
		return
	}

	convs, ok := convMap["items"].([]collector.Conversation)
	if !ok {
		convs = []collector.Conversation{}
	}

	lines := []string{
		"Date,Agent,Model,Conversation ID,Title,Project,Steps,Input Tokens,Output Tokens,Cache Tokens,Total Tokens,Last Modified",
	}

	for _, c := range convs {
		titleEscaped := `"` + strings.ReplaceAll(c.Title, `"`, `""`) + `"`
		lines = append(lines, fmt.Sprintf("%s,%s,%s,%s,%s,%s,%d,%d,%d,%d,%d,%s",
			c.DateBucket, c.AgentType, c.ModelDisplayName, c.ConversationID,
			titleEscaped, c.ProjectID, c.StepRecordsCount,
			c.TotalInputTokens, c.TotalOutputTokens, c.TotalCacheTokens, c.TotalTokens,
			c.LastModified,
		))
	}

	csvData := "\ufeff" + strings.Join(lines, "\n")
	content := []byte(csvData)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="universal_ai_token_report.csv"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (s *Server) handleExportJSON(w http.ResponseWriter, agent, model, date string) {
	summary, _ := s.db.GetSummary(agent, model, date)
	daily, _ := s.db.GetDaily(agent, model, 60)
	trends, _ := s.db.GetTrends(agent, "day", model, date)
	convMap, _ := s.db.GetConversations(agent, model, date, "total_tokens", "desc", "", 10000, 0)

	convs, _ := convMap["items"].([]collector.Conversation)
	if convs == nil {
		convs = []collector.Conversation{}
	}

	exportObj := map[string]interface{}{
		"exported_at": time.Now().Format("2006-01-02 15:04:05"),
		"filter": map[string]string{
			"agent": agent,
			"model": model,
			"date":  date,
		},
		"summary":       summary,
		"daily_records": daily,
		"trends":        trends,
		"conversations": convs,
	}

	content, err := json.MarshalIndent(exportObj, "", "  ")
	if err != nil {
		s.sendJSON(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="universal_ai_token_data.json"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (s *Server) sendJSON(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
