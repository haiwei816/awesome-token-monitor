# 🚀 Awesome Token Monitor (AI Agent 模型与 Token 监控中心)

<p align="center">
  <a href="https://github.com/haiwei816/awesome-token-monitor/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square" alt="License"></a>
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Go-1.21%2B-00ADD8.svg?style=flat-square&logo=go" alt="Go Version"></a>
  <img src="https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-orange.svg?style=flat-square" alt="Platform"></a>
  <img src="https://img.shields.io/badge/Privacy-100%25%20Local%20Only-success.svg?style=flat-square" alt="Privacy"></a>
  <img src="https://img.shields.io/badge/CGO-Zero%20Dependency-brightgreen.svg?style=flat-square" alt="CGO Zero Dependency"></a>
</p>

<p align="center">
  <b>专为本地 AI 编程助手与 Agent 工具打造的跨平台 Token 消耗、模型型号追踪、每日账本流水与会话遥测监控看板。</b>
  <br>
  <sub>纯 Go 原生高性能二进制构建，零外部依赖，毫秒级增量扫描，100% 本地运行，绝不上传任何隐私。</sub>
</p>

<p align="center">
  <a href="#-开源说明">开源说明</a> •
  <a href="#-核心特性">核心特性</a> •
  <a href="#-支持平台与模型生态">支持平台</a> •
  <a href="#-快速上手">快速上手</a> •
  <a href="#-restful-api-接口">API 接口</a> •
  <a href="#-隐私与安全声明">隐私安全</a> •
  <a href="#-开源许可证">许可证</a> •
  <a href="README_EN.md">English Documentation</a>
</p>

---

## 📖 开源说明 (About Awesome Token Monitor)

### 为什么开源这个项目？
随着大语言模型（LLM）与智能编程助手（AI Coding Agents）的爆发，开发者日常工作流中往往同时使用多个不同的 Agent 工具（例如 **Claude Code**、**OpenAI Codex**、**Google Antigravity**、**Pi Agent**、**Qoder**、**WorkBuddy** 等）。

然而，每个工具的遥测数据分布极其分散：
- 存放位置各异（各自分布在 `~/.claude`、`~/.codex`、`~/.gemini`、`~/.cindy` 等隐藏目录下）；
- 缺少统一的 Token 账本与调用流水，无法统计每天、每个模型究竟消耗了多少 Token；
- 部分聚合/套壳工具（如 Cindy）存在会话与底层引擎重复统计或模型映射模糊的问题；
- 缓存命中（Cache Read）、上下文窗口峰值等关键效能指标缺乏透明可视的可视化图表。

**Awesome Token Monitor** 应运而生并决定完全**开源免费**提供给开发者社区。我们希望帮助每一位开发者对自己的本地 AI 算力使用情况拥有**完全透明、精准、实时掌控**的能力。

### 100% 本地隐私安全承诺
- **绝不联网外发**：本项目所有扫描、聚合、数据库读写及前端展示均**严格在本地回环地址（Localhost）**运行，不包含任何外部网络上传逻辑，没有任何遥测跟踪。
- **纯本地数据读取**：仅以只读方式解析本地相关工具的日志与会话缓存，不篡改、不删除用户原始对话内容。

---

## ✨ 核心特性

- 🌐 **多 Agent 平台统一聚合**
  - 自动发现并监控全电脑 7 大主流 AI Agent 平台：`OpenAI Codex`、`Claude Code`、`Pi Agent`、`Google Antigravity`、`Qoder`、`WorkBuddy`、`ZCode`。
- 🔍 **智能套壳解包与底层引擎归属**
  - 深度解析多层 Agent 托管关系（如 Cindy 托管的会话），自动剥离并穿透归属至其底层真实的物理引擎（Codex / Claude / Pi），杜绝虚假统计与重复计数。
- 📅 **每日账本时间轴与分日流水 (Daily Ledger)**
  - 精准天数切片技术（Date-Sliced Ledger）：针对长期跨天会话，精确拆分每日消耗；
  - 智能时区换算：原生支持本地时间转换，彻底解决午夜 00:00~08:00 凌晨会话被 UTC 归入昨天的时差问题；
  - 默认今日视图，支持在 40+ 天历史时间轴上秒级穿梭与切换。
- 🎯 **真实模型型号自动识别与排行榜**
  - 自动归一化解析并呈现底层模型：`Gemini 3.8 Flash/Pro`、`GPT-6.1 Sol`、`GPT-6-ASTRA`、`Claude Opus 5.5 / Sonnet 5`、`DeepSeek V4.1 Flash`、`千问 3.8 Flash`、`GLM-5.3` 等；
  - 支持按模型型号一键下钻筛选与全局排行榜展示。
- ⚡ **深度轮次交互检查器 (Step Inspector)**
  - 抽屉式侧边栏逐轮下钻：输入 Tokens、输出 Tokens、缓存命中 Tokens、思考链（Thinking / CoT）字符长度、调用的 Tool / Function Calls 名称列表等。
- 📊 **多维效能指标与可视化看板**
  - 输入/输出占比、缓存命中率（Cache Hit Ratio）、峰值上下文窗口（Peak Context Window）、轮次均值（Avg Tokens/Step）；
  - 基于 Chart.js 的日/小时双维度趋势图。
- 🚀 **极速纯 Go 架构 (Zero-CGO)**
  - 采用纯 Go 驱动编译，单二进制文件仅 16MB，无任何外部运行环境依赖（无需 Python、Node.js 或 C 编译器）；
  - 毫秒级增量文件缓存更新，增量扫描耗时低于 60ms。
- 📥 **专业报表一键导出**
  - 支持一键导出兼容 Excel 的 CSV 统计报表以及结构化 JSON 全量数据。

---

## 🤖 支持平台与模型生态

| Agent 平台 | 扫描来源 | 识别模型型号示例 | 架构特性 |
| :--- | :--- | :--- | :--- |
| **🧠 OpenAI Codex** | `.codex/sessions` + 托管会话 | GPT-6.1 Sol, GPT-6 Sol, GPT-6-ASTRA, Codex Auto Review | 轮次级 Token 自动核算 |
| **🟣 Claude Code** | `.claude/projects/**/*.jsonl` | Claude Opus 5.5, Claude Sonnet 5, Claude Fable 5 | 缓存命中率与上下文追踪 |
| **🥧 Pi Agent** | Cindy Pi 运行时 + `~/.pi` | Pi Standalone Agent, DeepSeek V4.1 Flash | 纯净独立归档，支持子代理追踪 |
| **⚡ Qoder** | `.qoder/projects` | 千问 3.8 Flash, qfmodel | 上下文比例与 Token 精准换算 |
| **🛰️ Antigravity** | `.gemini/antigravity` | Gemini 3.8 Flash, Gemini 3.8 Pro | 会话流水树、工具调用提取 |
| **💼 WorkBuddy** | `.workbuddy/workbuddy.db` | Tencent Hunyuan 3/4, Kimi K3.1, Space Bunny | SQLite 任务流水解析 |
| **🔷 ZCode** | `.zcode/cli/rollout` | GLM-5.3-Flash, GLM-4 | 交互流式 IO 遥测 |

---

## 🚀 快速上手

### 方式一：下载预编译可执行文件（推荐）

1. 前往 GitHub [Releases](https://github.com/haiwei816/awesome-token-monitor/releases) 页面下载适合你系统的最新压缩包；
2. 解压至任意本地目录；
3. **Windows 用户**：双击运行 `start_monitor.bat` 即可一键启动后台服务，并在浏览器自动打开仪表盘：
   - 默认访问地址：`http://localhost:8765`
   - 需要关闭服务时，双击运行 `stop_monitor.bat`。

---

### 方式二：从源码构建 (Go 1.21+)

如果你本地已安装 Go 环境：

```bash
# 1. 克隆代码仓库
git clone https://github.com/haiwei816/awesome-token-monitor.git
cd awesome-token-monitor

# 2. 编译生成二进制执行文件
go build -o monitor.exe .

# 3. 运行服务
./monitor.exe -port 8765
```

控制台输出如下提示即代表启动成功：
```text
================================================================
  Awesome Token Monitor (Go Edition)
  Web Dashboard: http://localhost:8765
  Supported Agents: Antigravity, Cindy (Codex/Claude/Pi), Codex, WorkBuddy, Qoder, ZCode
================================================================
```

---

## ⚙️ 命令行启动参数说明

`monitor` 支持以下灵活参数配置：

| 参数项 | 默认值 | 说明 |
| :--- | :--- | :--- |
| `-port` | `8765` | Web 服务监听端口（若端口被占用会自动尝试 `port+1`） |
| `-db` | `token_monitor_cache.db` | 本地 SQLite 缓存数据库文件路径 |
| `-web` | `web` | 前端静态页面文件目录路径 |

示例：
```bash
# 指定端口为 9000
./monitor.exe -port 9000 -db my_cache.db
```

---

## 🔌 RESTful API 接口规范

服务端内置了开箱即用的轻量 RESTful API，方便第三方脚本或自动化工具调用：

| 接口地址 | 请求方法 | 参数说明 | 接口功能描述 |
| :--- | :---: | :--- | :--- |
| `/api/summary` | `GET` | `agent`, `model`, `date` | 获取全局或筛选条件下的核心 KPI、Agent 分布与模型排行榜 |
| `/api/daily` | `GET` | `agent`, `model`, `limit` | 获取按天聚合的日账本流水（包含今日标识、各 Agent 与模型标签） |
| `/api/trends` | `GET` | `agent`, `granularity` (`day`/`hour`), `model`, `date` | 获取 Token 消耗与缓存命中的时间走势数据点 |
| `/api/conversations` | `GET` | `agent`, `model`, `date`, `sort`, `order`, `search`, `limit`, `offset` | 分页查询多 Agent 会话明细列表 |
| `/api/conversation/{id}` | `GET` | -- | 查询指定会话的完整元数据、子代理（Subagents）与交互步骤 |
| `/api/models` | `GET` | -- | 获取全部去重后的模型型号列表及用量汇总 |
| `/api/scan` | `POST` | -- | 手动触发全盘 Agent 日志增量同步刷新 |
| `/api/export` | `GET` | `format` (`csv`/`json`), `agent`, `model`, `date` | 导出过滤视图下的 Excel CSV 报表或 JSON 原始文件 |

---

## 🔒 隐私与数据安全说明

1. **零外部网络传输**：本程序没有集成任何遥测上报 SDK（如 Google Analytics、Sentry 等），不向任何外网服务器发送请求。
2. **只读保护**：扫描器只以只读方式（Read-Only）读取本地已有的日志文件和数据库，不会篡改任何 AI 软件的配置与对话文件。
3. **数据主权**：生成的所有索引与聚合缓存均以纯 SQLite 文件（`token_monitor_cache.db`）保存在本地项目目录下，随时可删除重建。

---

## 🗺️ 项目技术栈

- **后端核心**：Golang (Go 1.21+)
- **本地存储**：SQLite (`modernc.org/sqlite` 纯 Go 驱动，免 CGO)
- **前端架构**：Vanilla JavaScript (ES6+), Modern CSS3 Glassmorphism UI
- **数据图表**：Chart.js
- **打包部署**：单一静态可执行文件 + 静态资源热加载

---

## 🤝 参与贡献 (Contributing)

欢迎任何形式的贡献！
1. 如果你发现了 Bug 或有时区/模型识别偏差，请提交 [Issue](https://github.com/haiwei816/awesome-token-monitor/issues)；
2. 如果你希望支持更多本地 AI 工具（如 Cursor、Aider、Continue 等），欢迎提交 Pull Request；
3. 欢迎为本项目点亮一粒 ⭐ **Star** 支持开源创作！

---

## 📄 开源许可证 (License)

本项目采用 [MIT 许可证](LICENSE) 开源。允许自由使用、修改、分发及用于商业或个人用途。
