package collector

// StepRecord represents an individual LLM step/turn record.
type StepRecord struct {
	AgentType        string `json:"agent_type"`
	ConversationID   string `json:"conversation_id"`
	StepIndex        int    `json:"step_index"`
	ModelName        string `json:"model_name"`
	ModelDisplayName string `json:"model_display_name"`
	Source           string `json:"source"`
	StepType         string `json:"step_type"`
	Status           string `json:"status"`
	CreatedAt        string `json:"created_at"`
	DateBucket       string `json:"date_bucket"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	ToolCallsCount   int    `json:"tool_calls_count"`
	ToolNames        string `json:"tool_names"`
	ThinkingLength   int    `json:"thinking_length"`
}

// Conversation represents an aggregated session conversation.
type Conversation struct {
	ID                   string `json:"id"`
	ConversationID       string `json:"conversation_id"`
	AgentType            string `json:"agent_type"`
	ModelName            string `json:"model_name"`
	ModelDisplayName     string `json:"model_display_name"`
	Title                string `json:"title"`
	Preview              string `json:"preview"`
	StepCount            int    `json:"step_count"`
	LastModified         string `json:"last_modified"`
	ProjectID            string `json:"project_id"`
	AgentRole            string `json:"agent_role"`
	ParentConversationID string `json:"parent_conversation_id"`
	NestingDepth         int    `json:"nesting_depth"`
	Status               string `json:"status"`
	TotalInputTokens     int64  `json:"total_input_tokens"`
	TotalOutputTokens    int64  `json:"total_output_tokens"`
	TotalCacheTokens     int64  `json:"total_cache_tokens"`
	TotalTokens          int64  `json:"total_tokens"`
	StepRecordsCount     int    `json:"step_records_count"`
	DateBucket           string `json:"date_bucket"`
	FirstStepTime        string `json:"first_step_time"`
	LastStepTime         string `json:"last_step_time"`
}

// SummaryResponse represents the response structure for /api/summary
type SummaryResponse struct {
	TotalInput        int64            `json:"total_input"`
	TotalOutput       int64            `json:"total_output"`
	TotalCache        int64            `json:"total_cache"`
	TotalTokens       int64            `json:"total_tokens"`
	TotalConvs        int              `json:"total_convs"`
	TotalSteps        int              `json:"total_steps"`
	CacheHitRatio     float64          `json:"cache_hit_ratio"`
	TodayInput        int64            `json:"today_input"`
	TodayOutput       int64            `json:"today_output"`
	TodayCache        int64            `json:"today_cache"`
	TodayTotal        int64            `json:"today_total"`
	TodaySteps        int              `json:"today_steps"`
	PeakContextWindow int64            `json:"peak_context_window"`
	AvgTokensPerStep  int64            `json:"avg_tokens_per_step"`
	AgentsBreakdown   []AgentBreakdown `json:"agents_breakdown"`
	ModelsBreakdown   []ModelBreakdown `json:"models_breakdown"`
}

// AgentBreakdown represents summary per agent.
type AgentBreakdown struct {
	AgentType   string `json:"agent_type"`
	ConvCount   int    `json:"conv_count"`
	TotalTokens int64  `json:"total_tokens"`
	InputTokens int64  `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	CacheTokens int64  `json:"cache_tokens"`
}

// ModelBreakdown represents summary per model.
type ModelBreakdown struct {
	ModelDisplayName string `json:"model_display_name"`
	ConvCount        int    `json:"conv_count"`
	TotalTokens      int64  `json:"total_tokens"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheTokens      int64  `json:"cache_tokens"`
}

// DailyRecord represents daily usage.
type DailyRecord struct {
	DateBucket   string   `json:"date_bucket"`
	ConvCount    int      `json:"conv_count"`
	TotalTokens  int64    `json:"total_tokens"`
	InputTokens  int64    `json:"input_tokens"`
	OutputTokens int64    `json:"output_tokens"`
	CacheTokens  int64    `json:"cache_tokens"`
	TotalSteps   int      `json:"total_steps"`
	Agents       string   `json:"agents"`
	Models       string   `json:"models"`
	IsToday      bool     `json:"is_today"`
	AgentsList   []string `json:"agents_list"`
	ModelsList   []string `json:"models_list"`
}

// TrendPoint represents a point on trend chart.
type TrendPoint struct {
	TimeBucket   string `json:"time_bucket"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheTokens  int64  `json:"cache_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
	StepCount    int    `json:"step_count"`
}

// SubagentInfo represents nested subagent information.
type SubagentInfo struct {
	ConversationID    string `json:"conversation_id"`
	Title             string `json:"title"`
	AgentRole         string `json:"agent_role"`
	ModelDisplayName  string `json:"model_display_name"`
	TotalTokens       int64  `json:"total_tokens"`
	TotalInputTokens  int64  `json:"total_input_tokens"`
	TotalOutputTokens int64  `json:"total_output_tokens"`
	StepRecordsCount  int    `json:"step_records_count"`
}

// ConversationDetail represents conversation full detail with its steps and subagents.
type ConversationDetail struct {
	Conversation
	Steps     []StepRecord   `json:"steps"`
	Subagents []SubagentInfo `json:"subagents"`
}

// ModelListItem represents model usage item for /api/models.
type ModelListItem struct {
	ModelDisplayName string `json:"model_display_name"`
	ConvCount        int    `json:"conv_count"`
	TotalTokens      int64  `json:"total_tokens"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheTokens      int64  `json:"cache_tokens"`
}

// SyncResult represents the result of a sync run.
type SyncResult struct {
	Status          string         `json:"status"`
	DurationSeconds float64        `json:"duration_seconds"`
	Counts          map[string]int `json:"counts"`
	Timestamp       string         `json:"timestamp"`
}

