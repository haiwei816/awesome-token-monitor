package db

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"antigravity-token-monitor/collector"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
	mu   sync.RWMutex
}

// NewDB opens or creates the SQLite database and sets up schema.
func NewDB(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// Performance Pragmas
	conn.Exec("PRAGMA journal_mode = WAL;")
	conn.Exec("PRAGMA synchronous = NORMAL;")
	conn.Exec("PRAGMA busy_timeout = 5000;")

	d := &DB{conn: conn}
	if err := d.initSchema(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) initSchema() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			conversation_id TEXT,
			agent_type TEXT,
			model_name TEXT,
			model_display_name TEXT,
			title TEXT,
			preview TEXT,
			step_count INTEGER,
			last_modified TEXT,
			project_id TEXT,
			agent_role TEXT,
			parent_conversation_id TEXT,
			nesting_depth INTEGER,
			status TEXT,
			total_input_tokens INTEGER,
			total_output_tokens INTEGER,
			total_cache_tokens INTEGER,
			total_tokens INTEGER,
			step_records_count INTEGER,
			date_bucket TEXT,
			first_step_time TEXT,
			last_step_time TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_conv_agent ON conversations(agent_type);`,
		`CREATE INDEX IF NOT EXISTS idx_conv_date ON conversations(date_bucket);`,
		`CREATE INDEX IF NOT EXISTS idx_conv_model ON conversations(model_display_name);`,
		`CREATE TABLE IF NOT EXISTS step_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_type TEXT,
			conversation_id TEXT,
			step_index INTEGER,
			model_name TEXT,
			model_display_name TEXT,
			source TEXT,
			step_type TEXT,
			status TEXT,
			created_at TEXT,
			date_bucket TEXT,
			input_tokens INTEGER,
			output_tokens INTEGER,
			cache_read_tokens INTEGER,
			tool_calls_count INTEGER,
			tool_names TEXT,
			thinking_length INTEGER,
			UNIQUE(agent_type, conversation_id, step_index, date_bucket, model_name)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_step_agent_date ON step_records(agent_type, date_bucket);`,
		`CREATE INDEX IF NOT EXISTS idx_step_conv ON step_records(conversation_id);`,
		`CREATE TABLE IF NOT EXISTS file_scan_state (
			file_path TEXT PRIMARY KEY,
			agent_type TEXT,
			mtime REAL,
			file_size INTEGER,
			scanned_at TEXT
		);`,
	}

	for _, q := range queries {
		if _, err := d.conn.Exec(q); err != nil {
			return fmt.Errorf("failed executing schema: %w", err)
		}
	}
	return nil
}

// GetSummary returns aggregated KPIs and breakdowns.
func (d *DB) GetSummary(agentFilter, modelFilter, dateFilter string) (*collector.SummaryResponse, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var where []string
	var args []interface{}

	if agentFilter != "" && agentFilter != "all" {
		where = append(where, "agent_type = ?")
		args = append(args, agentFilter)
	}
	if modelFilter != "" && modelFilter != "all" {
		where = append(where, "model_display_name = ?")
		args = append(args, modelFilter)
	}
	if dateFilter != "" && dateFilter != "all" {
		where = append(where, "date_bucket = ?")
		args = append(args, dateFilter)
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	// Overall KPIs from conversations
	query := fmt.Sprintf(`
		SELECT 
			COALESCE(SUM(total_input_tokens), 0),
			COALESCE(SUM(total_output_tokens), 0),
			COALESCE(SUM(total_cache_tokens), 0),
			COALESCE(SUM(total_tokens), 0),
			COUNT(DISTINCT conversation_id),
			COALESCE(SUM(step_records_count), 0)
		FROM conversations %s;
	`, whereSQL)

	res := &collector.SummaryResponse{}
	err := d.conn.QueryRow(query, args...).Scan(
		&res.TotalInput,
		&res.TotalOutput,
		&res.TotalCache,
		&res.TotalTokens,
		&res.TotalConvs,
		&res.TotalSteps,
	)
	if err != nil {
		return nil, err
	}

	inAndCache := res.TotalInput + res.TotalCache
	if inAndCache > 0 {
		res.CacheHitRatio = math.Round((float64(res.TotalCache)/float64(inAndCache)*100)*10) / 10
	}

	// Today stats from step_records
	todayStr := time.Now().UTC().Format("2006-01-02")
	stepWhere := []string{"date_bucket = ?"}
	stepArgs := []interface{}{todayStr}
	if agentFilter != "" && agentFilter != "all" {
		stepWhere = append(stepWhere, "agent_type = ?")
		stepArgs = append(stepArgs, agentFilter)
	}
	if modelFilter != "" && modelFilter != "all" {
		stepWhere = append(stepWhere, "model_display_name = ?")
		stepArgs = append(stepArgs, modelFilter)
	}

	todayQuery := fmt.Sprintf(`
		SELECT 
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cache_read_tokens), 0),
			COUNT(id)
		FROM step_records WHERE %s;
	`, strings.Join(stepWhere, " AND "))

	_ = d.conn.QueryRow(todayQuery, stepArgs...).Scan(
		&res.TodayInput,
		&res.TodayOutput,
		&res.TodayCache,
		&res.TodaySteps,
	)
	res.TodayTotal = res.TodayInput + res.TodayOutput

	// Peak context window
	peakQuery := fmt.Sprintf(`SELECT COALESCE(MAX(input_tokens), 0) FROM step_records %s;`, whereSQL)
	_ = d.conn.QueryRow(peakQuery, args...).Scan(&res.PeakContextWindow)

	if res.TotalSteps > 0 {
		res.AvgTokensPerStep = res.TotalTokens / int64(res.TotalSteps)
	}

	// Agents breakdown
	agentBreakdownQuery := fmt.Sprintf(`
		SELECT 
			agent_type,
			COUNT(DISTINCT conversation_id),
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(total_input_tokens), 0),
			COALESCE(SUM(total_output_tokens), 0),
			COALESCE(SUM(total_cache_tokens), 0)
		FROM conversations %s
		GROUP BY agent_type
		ORDER BY total_tokens DESC;
	`, whereSQL)

	aRows, err := d.conn.Query(agentBreakdownQuery, args...)
	if err == nil {
		defer aRows.Close()
		for aRows.Next() {
			var ab collector.AgentBreakdown
			if err := aRows.Scan(&ab.AgentType, &ab.ConvCount, &ab.TotalTokens, &ab.InputTokens, &ab.OutputTokens, &ab.CacheTokens); err == nil {
				res.AgentsBreakdown = append(res.AgentsBreakdown, ab)
			}
		}
	}

	// Models breakdown
	modelBreakdownQuery := fmt.Sprintf(`
		SELECT 
			model_display_name,
			COUNT(DISTINCT conversation_id),
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(total_input_tokens), 0),
			COALESCE(SUM(total_output_tokens), 0),
			COALESCE(SUM(total_cache_tokens), 0)
		FROM conversations %s
		GROUP BY model_display_name
		ORDER BY total_tokens DESC;
	`, whereSQL)

	mRows, err := d.conn.Query(modelBreakdownQuery, args...)
	if err == nil {
		defer mRows.Close()
		for mRows.Next() {
			var mb collector.ModelBreakdown
			if err := mRows.Scan(&mb.ModelDisplayName, &mb.ConvCount, &mb.TotalTokens, &mb.InputTokens, &mb.OutputTokens, &mb.CacheTokens); err == nil {
				res.ModelsBreakdown = append(res.ModelsBreakdown, mb)
			}
		}
	}

	return res, nil
}

// GetTrends returns chronological trend aggregations.
func (d *DB) GetTrends(agentFilter, granularity, modelFilter, dateFilter string) ([]collector.TrendPoint, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var where []string
	var args []interface{}

	if agentFilter != "" && agentFilter != "all" {
		where = append(where, "agent_type = ?")
		args = append(args, agentFilter)
	}
	if modelFilter != "" && modelFilter != "all" {
		where = append(where, "model_display_name = ?")
		args = append(args, modelFilter)
	}
	if dateFilter != "" && dateFilter != "all" {
		where = append(where, "date_bucket = ?")
		args = append(args, dateFilter)
	}

	timeExpr := "date_bucket"
	if granularity == "hour" {
		timeExpr = "strftime('%Y-%m-%d %H:00', created_at)"
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT 
			%s as time_bucket,
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cache_read_tokens), 0),
			COALESCE(SUM(input_tokens + output_tokens + cache_read_tokens), 0),
			COUNT(id)
		FROM step_records
		%s
		GROUP BY time_bucket
		ORDER BY time_bucket ASC;
	`, timeExpr, whereSQL)

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []collector.TrendPoint
	for rows.Next() {
		var pt collector.TrendPoint
		if err := rows.Scan(&pt.TimeBucket, &pt.InputTokens, &pt.OutputTokens, &pt.CacheTokens, &pt.TotalTokens, &pt.StepCount); err == nil {
			points = append(points, pt)
		}
	}
	return points, nil
}

// GetConversations returns paginated conversations matching search/filters.
func (d *DB) GetConversations(agentFilter, modelFilter, dateFilter, sortCol, order, search string, limit, offset int) (map[string]interface{}, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var where []string
	var args []interface{}

	if agentFilter != "" && agentFilter != "all" {
		where = append(where, "agent_type = ?")
		args = append(args, agentFilter)
	}
	if modelFilter != "" && modelFilter != "all" {
		where = append(where, "model_display_name = ?")
		args = append(args, modelFilter)
	}
	if dateFilter != "" && dateFilter != "all" {
		where = append(where, "date_bucket = ?")
		args = append(args, dateFilter)
	}
	if search != "" {
		where = append(where, "(title LIKE ? OR preview LIKE ? OR project_id LIKE ? OR conversation_id LIKE ?)")
		pat := "%" + search + "%"
		args = append(args, pat, pat, pat, pat)
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM conversations %s;", whereSQL)
	var total int
	_ = d.conn.QueryRow(countQuery, args...).Scan(&total)

	// Sort
	allowedCols := map[string]string{
		"total_tokens":  "total_tokens",
		"last_modified": "last_modified",
		"step_count":    "step_count",
	}
	col, ok := allowedCols[sortCol]
	if !ok {
		col = "total_tokens"
	}
	dir := "DESC"
	if strings.ToLower(order) == "asc" {
		dir = "ASC"
	}
	orderBy := fmt.Sprintf("%s %s", col, dir)

	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	listQuery := fmt.Sprintf(`
		SELECT 
			id, conversation_id, agent_type, model_name, model_display_name,
			title, preview, step_count, last_modified, project_id, agent_role,
			parent_conversation_id, nesting_depth, status, total_input_tokens,
			total_output_tokens, total_cache_tokens, total_tokens, step_records_count,
			date_bucket, first_step_time, last_step_time
		FROM conversations
		%s
		ORDER BY %s
		LIMIT %d OFFSET %d;
	`, whereSQL, orderBy, limit, offset)

	rows, err := d.conn.Query(listQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []collector.Conversation{}
	for rows.Next() {
		var c collector.Conversation
		if err := rows.Scan(
			&c.ID, &c.ConversationID, &c.AgentType, &c.ModelName, &c.ModelDisplayName,
			&c.Title, &c.Preview, &c.StepCount, &c.LastModified, &c.ProjectID, &c.AgentRole,
			&c.ParentConversationID, &c.NestingDepth, &c.Status, &c.TotalInputTokens,
			&c.TotalOutputTokens, &c.TotalCacheTokens, &c.TotalTokens, &c.StepRecordsCount,
			&c.DateBucket, &c.FirstStepTime, &c.LastStepTime,
		); err == nil {
			items = append(items, c)
		}
	}

	return map[string]interface{}{
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"items":  items,
	}, nil
}

// GetConversationDetail returns complete conversation details, steps and any subagents.
func (d *DB) GetConversationDetail(conversationID string) (*collector.ConversationDetail, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var conv collector.Conversation
	err := d.conn.QueryRow(`
		SELECT id, conversation_id, agent_type, model_name, model_display_name, title, preview,
		       step_count, last_modified, project_id, agent_role, parent_conversation_id,
		       nesting_depth, status, total_input_tokens, total_output_tokens, total_cache_tokens,
		       total_tokens, step_records_count, date_bucket, first_step_time, last_step_time
		FROM conversations
		WHERE conversation_id = ?
		ORDER BY last_modified DESC LIMIT 1
	`, conversationID).Scan(
		&conv.ID, &conv.ConversationID, &conv.AgentType, &conv.ModelName, &conv.ModelDisplayName,
		&conv.Title, &conv.Preview, &conv.StepCount, &conv.LastModified, &conv.ProjectID,
		&conv.AgentRole, &conv.ParentConversationID, &conv.NestingDepth, &conv.Status,
		&conv.TotalInputTokens, &conv.TotalOutputTokens, &conv.TotalCacheTokens, &conv.TotalTokens,
		&conv.StepRecordsCount, &conv.DateBucket, &conv.FirstStepTime, &conv.LastStepTime,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// Aggregate over multi-day buckets for this conversation
	var aggIn, aggOut, aggCache, aggTot sql.NullInt64
	var aggSteps sql.NullInt64
	var aggModels sql.NullString
	_ = d.conn.QueryRow(`
		SELECT 
			SUM(total_input_tokens),
			SUM(total_output_tokens),
			SUM(total_cache_tokens),
			SUM(total_tokens),
			SUM(step_count),
			GROUP_CONCAT(DISTINCT model_display_name)
		FROM conversations
		WHERE conversation_id = ?
	`, conversationID).Scan(&aggIn, &aggOut, &aggCache, &aggTot, &aggSteps, &aggModels)

	if aggIn.Valid {
		conv.TotalInputTokens = aggIn.Int64
	}
	if aggOut.Valid {
		conv.TotalOutputTokens = aggOut.Int64
	}
	if aggCache.Valid {
		conv.TotalCacheTokens = aggCache.Int64
	}
	if aggTot.Valid {
		conv.TotalTokens = aggTot.Int64
	}
	if aggSteps.Valid {
		conv.StepCount = int(aggSteps.Int64)
	}
	if aggModels.Valid && aggModels.String != "" {
		conv.ModelDisplayName = aggModels.String
	}

	detail := &collector.ConversationDetail{
		Conversation: conv,
		Steps:        []collector.StepRecord{},
		Subagents:    []collector.SubagentInfo{},
	}

	// Fetch steps
	stepRows, err := d.conn.Query(`
		SELECT agent_type, conversation_id, step_index, model_name, model_display_name,
		       source, step_type, status, created_at, date_bucket,
		       input_tokens, output_tokens, cache_read_tokens,
		       tool_calls_count, tool_names, thinking_length
		FROM step_records
		WHERE conversation_id = ?
		ORDER BY created_at ASC, step_index ASC
	`, conversationID)
	if err == nil {
		defer stepRows.Close()
		for stepRows.Next() {
			var st collector.StepRecord
			if err := stepRows.Scan(
				&st.AgentType, &st.ConversationID, &st.StepIndex, &st.ModelName, &st.ModelDisplayName,
				&st.Source, &st.StepType, &st.Status, &st.CreatedAt, &st.DateBucket,
				&st.InputTokens, &st.OutputTokens, &st.CacheReadTokens,
				&st.ToolCallsCount, &st.ToolNames, &st.ThinkingLength,
			); err == nil {
				detail.Steps = append(detail.Steps, st)
			}
		}
	}

	// Fetch subagents
	subRows, err := d.conn.Query(`
		SELECT conversation_id, title, agent_role, model_display_name,
		       total_tokens, total_input_tokens, total_output_tokens, step_records_count
		FROM conversations
		WHERE parent_conversation_id = ?
	`, conversationID)
	if err == nil {
		defer subRows.Close()
		for subRows.Next() {
			var sa collector.SubagentInfo
			if err := subRows.Scan(
				&sa.ConversationID, &sa.Title, &sa.AgentRole, &sa.ModelDisplayName,
				&sa.TotalTokens, &sa.TotalInputTokens, &sa.TotalOutputTokens, &sa.StepRecordsCount,
			); err == nil {
				detail.Subagents = append(detail.Subagents, sa)
			}
		}
	}

	return detail, nil
}

// GetModelsList returns all distinct models with their token statistics.
func (d *DB) GetModelsList() ([]collector.ModelListItem, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.conn.Query(`
		SELECT 
			model_display_name,
			COUNT(DISTINCT conversation_id) as conv_count,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(total_input_tokens), 0) as input_tokens,
			COALESCE(SUM(total_output_tokens), 0) as output_tokens,
			COALESCE(SUM(total_cache_tokens), 0) as cache_tokens
		FROM conversations
		WHERE model_display_name != ''
		GROUP BY model_display_name
		ORDER BY total_tokens DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []collector.ModelListItem{}
	for rows.Next() {
		var m collector.ModelListItem
		if err := rows.Scan(
			&m.ModelDisplayName, &m.ConvCount, &m.TotalTokens,
			&m.InputTokens, &m.OutputTokens, &m.CacheTokens,
		); err == nil {
			list = append(list, m)
		}
	}
	return list, nil
}

// GetConversationSteps returns individual steps for a conversation.
func (d *DB) GetConversationSteps(conversationID, dateBucket string) ([]collector.StepRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var where []string
	var args []interface{}

	where = append(where, "conversation_id = ?")
	args = append(args, conversationID)

	if dateBucket != "" && dateBucket != "all" {
		where = append(where, "date_bucket = ?")
		args = append(args, dateBucket)
	}

	query := fmt.Sprintf(`
		SELECT 
			agent_type, conversation_id, step_index, model_name, model_display_name,
			source, step_type, status, created_at, date_bucket, input_tokens,
			output_tokens, cache_read_tokens, tool_calls_count, tool_names, thinking_length
		FROM step_records
		WHERE %s
		ORDER BY step_index ASC;
	`, strings.Join(where, " AND "))

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []collector.StepRecord
	for rows.Next() {
		var s collector.StepRecord
		if err := rows.Scan(
			&s.AgentType, &s.ConversationID, &s.StepIndex, &s.ModelName, &s.ModelDisplayName,
			&s.Source, &s.StepType, &s.Status, &s.CreatedAt, &s.DateBucket, &s.InputTokens,
			&s.OutputTokens, &s.CacheReadTokens, &s.ToolCallsCount, &s.ToolNames, &s.ThinkingLength,
		); err == nil {
			steps = append(steps, s)
		}
	}
	return steps, nil
}

// GetDaily returns daily aggregations.
func (d *DB) GetDaily(agentFilter, modelFilter string, limit int) ([]collector.DailyRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var where []string
	var args []interface{}
	where = append(where, "date_bucket != ''")

	if agentFilter != "" && agentFilter != "all" {
		where = append(where, "agent_type = ?")
		args = append(args, agentFilter)
	}
	if modelFilter != "" && modelFilter != "all" {
		where = append(where, "model_display_name = ?")
		args = append(args, modelFilter)
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")
	if limit <= 0 {
		limit = 60
	}

	query := fmt.Sprintf(`
		SELECT 
			date_bucket,
			COUNT(DISTINCT conversation_id) as conv_count,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(total_input_tokens), 0) as input_tokens,
			COALESCE(SUM(total_output_tokens), 0) as output_tokens,
			COALESCE(SUM(total_cache_tokens), 0) as cache_tokens,
			COALESCE(SUM(step_records_count), 0) as total_steps,
			COALESCE(GROUP_CONCAT(DISTINCT agent_type), '') as agents,
			COALESCE(GROUP_CONCAT(DISTINCT model_display_name), '') as models
		FROM conversations
		%s
		GROUP BY date_bucket
		ORDER BY date_bucket DESC
		LIMIT %d;
	`, whereSQL, limit)

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	todayStr := time.Now().UTC().Format("2006-01-02")
	list := []collector.DailyRecord{}
	for rows.Next() {
		var r collector.DailyRecord
		if err := rows.Scan(
			&r.DateBucket, &r.ConvCount, &r.TotalTokens, &r.InputTokens,
			&r.OutputTokens, &r.CacheTokens, &r.TotalSteps, &r.Agents, &r.Models,
		); err == nil {
			r.IsToday = (r.DateBucket == todayStr)
			for _, a := range strings.Split(r.Agents, ",") {
				a = strings.TrimSpace(a)
				if a != "" {
					r.AgentsList = append(r.AgentsList, a)
				}
			}
			for _, m := range strings.Split(r.Models, ",") {
				m = strings.TrimSpace(m)
				if m != "" {
					r.ModelsList = append(r.ModelsList, m)
				}
			}
			list = append(list, r)
		}
	}
	return list, nil
}

// SaveBatch persists conversations and steps in a transaction.
func (d *DB) SaveBatch(convs []collector.Conversation, steps []collector.StepRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if len(steps) > 0 {
		stmt, err := tx.Prepare(`
			INSERT OR REPLACE INTO step_records
			(agent_type, conversation_id, step_index, model_name, model_display_name, source, step_type, status, created_at, date_bucket,
			 input_tokens, output_tokens, cache_read_tokens, tool_calls_count, tool_names, thinking_length)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, s := range steps {
			_, err = stmt.Exec(
				s.AgentType, s.ConversationID, s.StepIndex, s.ModelName, s.ModelDisplayName,
				s.Source, s.StepType, s.Status, s.CreatedAt, s.DateBucket,
				s.InputTokens, s.OutputTokens, s.CacheReadTokens, s.ToolCallsCount, s.ToolNames, s.ThinkingLength,
			)
			if err != nil {
				return err
			}
		}
	}

	if len(convs) > 0 {
		stmt, err := tx.Prepare(`
			INSERT OR REPLACE INTO conversations
			(id, conversation_id, agent_type, model_name, model_display_name, title, preview, step_count, last_modified,
			 project_id, agent_role, parent_conversation_id, nesting_depth, status,
			 total_input_tokens, total_output_tokens, total_cache_tokens, total_tokens,
			 step_records_count, date_bucket, first_step_time, last_step_time)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, c := range convs {
			_, err = stmt.Exec(
				c.ID, c.ConversationID, c.AgentType, c.ModelName, c.ModelDisplayName,
				c.Title, c.Preview, c.StepCount, c.LastModified,
				c.ProjectID, c.AgentRole, c.ParentConversationID, c.NestingDepth, c.Status,
				c.TotalInputTokens, c.TotalOutputTokens, c.TotalCacheTokens, c.TotalTokens,
				c.StepRecordsCount, c.DateBucket, c.FirstStepTime, c.LastStepTime,
			)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// GetScanStates loads file scan states for an agent.
func (d *DB) GetScanStates(agentType string) (map[string][2]float64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.conn.Query("SELECT file_path, mtime, file_size FROM file_scan_state WHERE agent_type = ?;", agentType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make(map[string][2]float64)
	for rows.Next() {
		var path string
		var mtime, size float64
		if err := rows.Scan(&path, &mtime, &size); err == nil {
			res[path] = [2]float64{mtime, size}
		}
	}
	return res, nil
}

// UpdateScanState updates scan cache for a file.
func (d *DB) UpdateScanState(filePath, agentType string, mtime float64, fileSize int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.conn.Exec(`
		INSERT OR REPLACE INTO file_scan_state (file_path, agent_type, mtime, file_size, scanned_at)
		VALUES (?, ?, ?, ?, ?);
	`, filePath, agentType, mtime, fileSize, now)
	return err
}
