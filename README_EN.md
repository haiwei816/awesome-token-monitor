# 🚀 Awesome Token Monitor (AI Agent Model & Token Analytics Hub)

<p align="center">
  <a href="https://github.com/haiwei816/awesome-token-monitor/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square" alt="License"></a>
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Go-1.21%2B-00ADD8.svg?style=flat-square&logo=go" alt="Go Version"></a>
  <img src="https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-orange.svg?style=flat-square" alt="Platform"></a>
  <img src="https://img.shields.io/badge/Privacy-100%25%20Local%20Only-success.svg?style=flat-square" alt="Privacy"></a>
  <img src="https://img.shields.io/badge/CGO-Zero%20Dependency-brightgreen.svg?style=flat-square" alt="CGO Zero Dependency"></a>
</p>

<p align="center">
  <b>A unified, cross-platform token consumption monitor, model tracker, daily ledger, and session telemetry dashboard designed for local AI coding assistants and agents.</b>
  <br>
  <sub>Built with native Golang for high performance, zero external dependencies, millisecond-level incremental scanning, and 100% local privacy guarantee.</sub>
</p>

<p align="center">
  <a href="#-about-awesome-token-monitor">About</a> •
  <a href="#-key-features">Key Features</a> •
  <a href="#-supported-agents--models">Supported Agents</a> •
  <a href="#-quick-start">Quick Start</a> •
  <a href="#-restful-api-specification">API Reference</a> •
  <a href="#-privacy--security">Privacy & Security</a> •
  <a href="#-license">License</a> •
  <a href="README.md">简体中文文档</a>
</p>

---

## 📖 About Awesome Token Monitor

### Why did we create and open-source this project?
With the rapid evolution of Large Language Models (LLMs) and AI Coding Agents, developers frequently juggle multiple local agent tools in parallel (such as **Claude Code**, **OpenAI Codex**, **Google Antigravity**, **Pi Agent**, **Qoder**, **WorkBuddy**, and more).

However, token telemetry is fragmented and obscure:
- Telemetry logs are scattered across various hidden directories (`~/.claude`, `~/.codex`, `~/.gemini`, `~/.cindy`, etc.);
- There is no unified ledger or chronological timeline to see how many tokens and which models were consumed each day;
- Proxy or aggregator tools (e.g., Cindy) often obscure the underlying physical engines, causing double-counting or ambiguous model mapping;
- Crucial efficiency indicators like cache hit rates (Cache Read) and peak context window sizes lack clear visualization.

**Awesome Token Monitor** was built to solve these exact pain points. We decided to make it completely **open-source and free** for the developer community, giving every engineer complete, transparent, and real-time control over their local AI computational usage.

### 100% Local Privacy Guarantee
- **Zero External Network Requests**: All log scanning, metric aggregation, database caching, and web dashboard rendering run strictly on `localhost`. There is zero external telemetry or cloud upload.
- **Read-Only Non-Destructive Parsing**: Telemetry data is parsed in read-only mode. It will never modify, overwrite, or delete your original chat transcripts.

---

## ✨ Key Features

- 🌐 **Multi-Agent Unified Telemetry**
  - Automatically discovers and monitors 7 local AI agent platforms: `OpenAI Codex`, `Claude Code`, `Pi Agent`, `Google Antigravity`, `Qoder`, `WorkBuddy`, and `ZCode`.
- 🔍 **Wrapper Unwrapping & Underlying Engine Attribution**
  - Automatically unrolls nested agent layers (such as sessions managed by Cindy) and maps them cleanly to their underlying physical engines (Codex / Claude / Pi), eliminating double-counting.
- 📅 **Daily Ledger Timeline & Precise Date-Slicing**
  - **Date-Sliced Ledger**: Accurately splits token usage for long-running multi-day sessions so that tokens consumed today are attributed to today, not yesterday.
  - **Local Timezone Normalization**: Automatically converts ISO and Unix timestamps to system local time (e.g. CST / UTC+8), preventing midnight sessions from lagging behind.
  - **Today-View First**: Defaults to today's active ledger while allowing instant one-click travel across 40+ days of history.
- 🎯 **Accurate Model Classification & Top Models Leaderboard**
  - Standardizes and identifies models including `Gemini 3.8 Flash/Pro`, `GPT-6.1 Sol`, `GPT-6-ASTRA`, `Claude Opus 5.5 / Sonnet 5`, `DeepSeek V4.1 Flash`, `Qwen 3.8 Flash`, and `GLM-5.3`.
  - Filter by model with a single click.
- ⚡ **Deep Step Inspector**
  - Side drawer inspector breakdown per interaction turn: input tokens, output tokens, cache read tokens, reasoning/CoT character count, and tool/function call names.
- 📊 **Comprehensive Analytics & Visual Charts**
  - Input/Output ratio, Cache Hit Ratio, Peak Context Window, Average tokens per step.
  - Interactive trend charts with day and hour granularities powered by Chart.js.
- 🚀 **Blazing Fast Pure Go Architecture (Zero CGO)**
  - Compiles into a single standalone binary (~16MB) using pure Go SQLite driver (`modernc.org/sqlite`). No Python, Node.js, or GCC/CGO compiler required.
  - Incremental file scanning takes under 60 milliseconds.
- 📥 **One-Click Export**
  - Export current view data to Excel-compatible CSV reports or raw JSON.

---

## 🤖 Supported Agents & Models

| Agent Platform | Scan Location | Recognized Models | Architectural Notes |
| :--- | :--- | :--- | :--- |
| **🧠 OpenAI Codex** | `.codex/sessions` + Managed sessions | GPT-6.1 Sol, GPT-6 Sol, GPT-6-ASTRA, Codex Auto Review | Turn-level token usage tracking |
| **🟣 Claude Code** | `.claude/projects/**/*.jsonl` | Claude Opus 5.5, Claude Sonnet 5, Claude Fable 5 | Cache read & context window breakdown |
| **🥧 Pi Agent** | Cindy Pi runtime + `~/.pi` | Pi Standalone Agent, DeepSeek V4.1 Flash | Isolated ledger with subagent tracking |
| **⚡ Qoder** | `.qoder/projects` | Qwen 3.8 Flash, qfmodel | Exact context ratio calculation |
| **🛰️ Antigravity** | `.gemini/antigravity` | Gemini 3.8 Flash, Gemini 3.8 Pro | Session tree & tool call extraction |
| **💼 WorkBuddy** | `.workbuddy/workbuddy.db` | Tencent Hunyuan 3/4, Kimi K3.1, Space Bunny | SQLite task ledger analysis |
| **🔷 ZCode** | `.zcode/cli/rollout` | GLM-5.3-Flash, GLM-4 | Streaming IO telemetry |

---

## 🚀 Quick Start

### Method 1: Pre-built Binary (Recommended)

1. Download the latest release archive from GitHub [Releases](https://github.com/haiwei816/awesome-token-monitor/releases);
2. Extract to any local directory;
3. **Windows**: Double-click `start_monitor.bat` to launch the background service and open the dashboard in your default browser:
   - Dashboard URL: `http://localhost:8765`
   - To stop the service: Double-click `stop_monitor.bat`.

---

### Method 2: Build from Source (Go 1.21+)

```bash
# 1. Clone the repository
git clone https://github.com/haiwei816/awesome-token-monitor.git
cd awesome-token-monitor

# 2. Build the binary
go build -o monitor.exe .

# 3. Start the server
./monitor.exe -port 8765
```

---

## ⚙️ CLI Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-port` | `8765` | HTTP server port (automatically tries `port+1` if in use) |
| `-db` | `token_monitor_cache.db` | Local SQLite cache database file path |
| `-web` | `web` | Directory containing dashboard static web files |

---

## 🔌 RESTful API Specification

| Endpoint | Method | Parameters | Description |
| :--- | :---: | :--- | :--- |
| `/api/summary` | `GET` | `agent`, `model`, `date` | Core KPIs, agent breakdown, and model rankings |
| `/api/daily` | `GET` | `agent`, `model`, `limit` | Daily ledger timeline with today flag and agent/model tags |
| `/api/trends` | `GET` | `agent`, `granularity` (`day`/`hour`), `model`, `date` | Chronological trend data points |
| `/api/conversations` | `GET` | `agent`, `model`, `date`, `sort`, `order`, `search`, `limit`, `offset` | Paginated conversation records |
| `/api/conversation/{id}` | `GET` | -- | Conversation metadata, subagents, and interaction steps |
| `/api/models` | `GET` | -- | Distinct models list with total usage counts |
| `/api/scan` | `POST` | -- | Triggers an immediate incremental scan of local telemetry files |
| `/api/export` | `GET` | `format` (`csv`/`json`), `agent`, `model`, `date` | Exports filtered view data as CSV or JSON |

---

## 🔒 Privacy & Security

1. **Zero External Communication**: No telemetry SDKs (Google Analytics, Sentry, etc.) are included. Zero outbound network traffic.
2. **Read-Only Protection**: The scanner strictly reads existing logs and SQLite files without modifying or deleting user files.
3. **Data Ownership**: All parsed metadata is kept in local SQLite files (`token_monitor_cache.db`), which can be deleted or rebuilt at any time.

---

## 🤝 Contributing

Contributions are welcome!
- Found a bug or time zone parsing discrepancy? Open an [Issue](https://github.com/haiwei816/awesome-token-monitor/issues).
- Want to add support for another local AI coding tool (Cursor, Aider, Continue)? Submit a Pull Request.
- Don't forget to star ⭐ the repository if you find it helpful!

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).
