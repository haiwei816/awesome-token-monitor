package collector

import (
	"strconv"
	"strings"
	"time"
)

// FormatModelName formats raw model identifiers into human-friendly display names.
func FormatModelName(rawName string) string {
	m := strings.TrimSpace(rawName)
	if m == "" {
		return "Unknown Model"
	}
	// Strip billing/metadata tags like #billing=subscription
	if idx := strings.Index(m, "#"); idx != -1 {
		m = m[:idx]
	}
	lower := strings.ToLower(m)

	if strings.Contains(lower, "codex-auto-review") || strings.Contains(lower, "codex_review") {
		return "Codex Auto Review"
	}
	if strings.Contains(lower, "gpt-6-astra") {
		return "GPT-6-ASTRA"
	}
	if strings.Contains(lower, "gpt-6.1-sol") || strings.Contains(lower, "gpt-6-1-sol") {
		return "GPT-6.1 Sol"
	}
	if strings.Contains(lower, "gpt-6-sol") {
		return "GPT-6 Sol"
	}
	if strings.Contains(lower, "gpt-6-luna") {
		return "GPT-6 Luna"
	}
	if strings.Contains(lower, "gpt-5.6-luna") {
		return "GPT-5.6 Luna"
	}
	if strings.Contains(lower, "gpt-5.6-terra") {
		return "GPT-5.6 Terra"
	}
	if strings.Contains(lower, "deepseek-v4.1-flash") || strings.Contains(lower, "deepseek-v4-1-flash") {
		return "DeepSeek V4.1 Flash"
	}
	if strings.Contains(lower, "deepseek-v4-flash") || strings.Contains(lower, "deepseek_v4_flash") {
		return "DeepSeek V4 Flash"
	}
	if strings.Contains(lower, "deepseek-flash") {
		return "DeepSeek Flash"
	}
	if strings.Contains(lower, "gemini-4") || strings.Contains(lower, "argon") ||
		strings.Contains(lower, "m408") || strings.Contains(lower, "m409") || strings.Contains(lower, "m410") ||
		strings.Contains(lower, "m411") || strings.Contains(lower, "m412") || strings.Contains(lower, "m413") ||
		strings.Contains(lower, "m417") || strings.Contains(lower, "m418") || strings.Contains(lower, "m419") {
		return "Gemini 4 Argon"
	}
	if strings.Contains(lower, "claude-opus-5-5") || strings.Contains(lower, "claude-opus-5.5") || strings.Contains(lower, "opus-5-5") || strings.Contains(lower, "opus-5.5") ||
		strings.Contains(lower, "m400") || strings.Contains(lower, "m401") || strings.Contains(lower, "m402") || strings.Contains(lower, "m403") || strings.Contains(lower, "m404") || strings.Contains(lower, "m405") {
		return "Claude Opus 5.5"
	}
	if strings.Contains(lower, "claude-opus-4-6") || strings.Contains(lower, "opus-4-6") || strings.Contains(lower, "m26") {
		return "Claude Opus 4.6"
	}
	if strings.Contains(lower, "claude-sonnet-4-6") || strings.Contains(lower, "m35") || strings.Contains(lower, "m36") || strings.Contains(lower, "m37") || strings.Contains(lower, "m16") {
		return "Claude Sonnet 4.6"
	}
	if strings.Contains(lower, "claude-sonnet-5") {
		return "Claude Sonnet 5.5"
	}
	if strings.Contains(lower, "gemini-2.5-pro") || strings.Contains(lower, "m18") || strings.Contains(lower, "m50") || strings.Contains(lower, "m148") {
		return "Gemini 2.5 Pro"
	}
	if strings.Contains(lower, "gemini-2.5-flash") || strings.Contains(lower, "m198") {
		return "Gemini 2.5 Flash"
	}
	if strings.Contains(lower, "gemini-3.1-flash-image") {
		return "Gemini 3.1 Flash Image"
	}
	if strings.Contains(lower, "gpt-oss") || strings.Contains(lower, "120b") {
		return "GPT-OSS 120B"
	}
	if strings.Contains(lower, "gemini-3.8-flash") || strings.Contains(lower, "gemini-flash") ||
		strings.Contains(lower, "m318") || strings.Contains(lower, "m322") || strings.Contains(lower, "m319") ||
		strings.Contains(lower, "m320") || strings.Contains(lower, "m299") || strings.Contains(lower, "m298") ||
		strings.Contains(lower, "m300") || strings.Contains(lower, "m124") || strings.Contains(lower, "m78") ||
		strings.Contains(lower, "placeholder") || strings.Contains(lower, "gemini") {
		return "Gemini 3.8 Flash"
	}
	if strings.Contains(lower, "qfmodel") || strings.Contains(lower, "qwen") || strings.Contains(lower, "千问") {
		return "千问 3.8 Flash"
	}
	if strings.Contains(lower, "glm-5.3-flash") || strings.Contains(lower, "glm-5") {
		return "GLM 5.3 Flash"
	}
	if strings.Contains(lower, "kimi-k3") || strings.Contains(lower, "kimi") {
		return "Kimi K3.1"
	}
	if strings.Contains(lower, "claude-haiku-5-5") || strings.Contains(lower, "claude-haiku-5.5") || strings.Contains(lower, "haiku") {
		return "Claude Haiku 5.5"
	}
	if strings.Contains(lower, "step-5") || strings.Contains(lower, "step_5") {
		return "Step 5 Preview"
	}
	if strings.Contains(lower, "claude-opus-5-5") || strings.Contains(lower, "claude-opus-5.5") {
		return "Claude Opus 5.5"
	}
	if strings.Contains(lower, "claude-opus-5") {
		return "Claude Opus 5"
	}
	if strings.Contains(lower, "claude-sonnet-5") {
		return "Claude Sonnet 5"
	}
	if strings.Contains(lower, "claude-fable-5") {
		return "Claude Fable 5"
	}
	if strings.Contains(lower, "hy4") || strings.Contains(lower, "hunyuan-4") {
		return "Tencent Hunyuan 4 Preview"
	}
	if strings.Contains(lower, "hy3") || strings.Contains(lower, "hunyuan-3") || strings.Contains(lower, "hunyuan") {
		return "Tencent Hunyuan 3"
	}
	if strings.Contains(lower, "gpt-image-2") {
		return "GPT-IMAGE-2"
	}
	if strings.Contains(lower, "gpt-5.6-sol") || strings.Contains(lower, "gpt-5-6-sol") {
		return "GPT-5.6 Sol"
	}
	if strings.Contains(lower, "seed-2") || strings.Contains(lower, "bytedance-seed") {
		return "Seed 2.1 Pro"
	}
	if strings.Contains(lower, "sensenova") {
		return "SenseNova 6.8 Flash Lite"
	}
	if strings.Contains(lower, "space-bunny") {
		return "Space Bunny"
	}

	return m
}

// ExtractDateBucket extracts YYYY-MM-DD from an ISO string, timestamp or number in local timezone.
func ExtractDateBucket(tsStr string) string {
	ts := strings.TrimSpace(tsStr)
	if ts == "" {
		return time.Now().Local().Format("2006-01-02")
	}

	// 1. Try parsing ISO8601/RFC3339 timestamps with time/timezone
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05.999999999+00:00",
		"2006-01-02 15:04:05.999999+00:00",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, ts); err == nil {
			return t.Local().Format("2006-01-02")
		}
	}

	// 2. Try parsing unix epoch timestamps (milliseconds or seconds)
	if val, err := strconv.ParseFloat(ts, 64); err == nil {
		sec := int64(val)
		nsec := int64(0)
		if val > 1e11 { // ms
			sec = int64(val / 1000.0)
			nsec = int64((val - float64(sec*1000)) * 1e6)
		}
		return time.Unix(sec, nsec).Local().Format("2006-01-02")
	}

	// 3. Fallback: if it's already YYYY-MM-DD
	if len(ts) >= 10 && ts[4] == '-' && ts[7] == '-' {
		return ts[:10]
	}

	return time.Now().Local().Format("2006-01-02")
}

// ResolveCindyEngine maps Cindy's internal agent_kind and model name to the true underlying engine.
func ResolveCindyEngine(agentKind, rawModel string) string {
	k := strings.ToLower(strings.TrimSpace(agentKind))
	if strings.Contains(k, "claude") || k == "cc" {
		return "claude"
	}
	if strings.Contains(k, "pi") {
		return "pi"
	}
	if strings.Contains(k, "codex") {
		return "codex"
	}
	m := strings.ToLower(rawModel)
	if strings.Contains(m, "claude") || strings.Contains(m, "deepseek") || strings.Contains(m, "glm") {
		return "claude"
	}
	return "codex"
}

// GetEngineRole returns the display role for an engine.
func GetEngineRole(engine string) string {
	switch engine {
	case "claude":
		return "Claude Code Engine"
	case "codex":
		return "Codex Engine"
	case "pi":
		return "Pi Coding Agent"
	default:
		return "AI Engine"
	}
}
