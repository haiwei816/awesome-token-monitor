# 🚀 Awesome Token Monitor (AI Agent 模型与 Token 监控中心)

专为本地主流 AI 编程助手与 Agent 工具打造的**通用型 Token 使用统计、模型型号追踪、全历史天数账本与会话遥测监控大盘**。
本项目采用高性能 **Golang 原生二进制引擎**，支持 Windows/macOS/Linux 无依赖一键极速运行。

---

## 🌟 核心引擎架构与支持平台（全电脑 7 大独立引擎）

根据用户需求，原套壳工具 **Cindy 已彻底拆解并归并至底层真实独立引擎**（Codex、Claude Code 与 Pi Agent），彻底避免重复统计并真实反映底层算力调用：

| Agent 平台 / 真实引擎 | 底层扫描源 / 驱动 | 核心消耗模型型号 | 历史总统计量 |
| :--- | :--- | :--- | :--- |
| **🧠 OpenAI Codex** | `.codex/sessions` + Cindy 托管会话 | GPT-6.1 Sol, GPT-6 Sol, GPT-6-ASTRA, Codex Auto Review | **16.09 亿 Tokens** (147 会话) |
| **⚡ Claude Code** | `.claude/projects` + Cindy CC 任务 | Claude Opus 5.5, Sonnet 5, Fable 5 | **6.43 亿 Tokens** (9 会话) |
| **⚡ Qoder** | `.qoder/projects` (上下文比例与 Token 换算) | 千问 3.8 Flash, qfmodel | **6.15 亿 Tokens** (37 会话) |
| **🛰️ Google Antigravity** | `.gemini/antigravity` (SQLite + 会话流水) | Gemini 3.8 Flash / Pro | **1.81 亿 Tokens** (11 会话) |
| **💼 WorkBuddy** | `.workbuddy/workbuddy.db` + 项目流水 | Tencent Hunyuan 3 / 4, Kimi K3.1, Space Bunny | **1,199 万 Tokens** (5 会话) |
| **🚀 ZCode** | `.zcode/cli/rollout` (模型流式 IO + 子代理) | GLM-5.3-Flash, GLM-4 | **1,019 万 Tokens** (3 会话) |
| **🥧 Pi Coding Agent** | Cindy Pi-Agent 目录与 `~/.pi` | Pi Standalone Agent | **就绪待命中** |
| **全平台全量总计** | -- | -- | **30.71 亿 Tokens (212 独立会话 / 41 天流水)** |

---

## ⚡ Golang 重构技术亮点

1. **零外部依赖 (Pure Go)**：
   - 采用 `modernc.org/sqlite` 纯 Go SQLite 驱动，Windows 下无需安装 gcc 或 CGO 环境，单个二进制 `monitor.exe` (16.9MB) 即可独立运行。
2. **毫秒级极速增量扫描**：
   - 相比原 Python 脚本全盘扫描需要数秒，Go 协程与扫描状态缓存使增量刷新时间降低至 **60 毫秒** 内，性能提升超过 30 倍。
3. **安全完整备份**：
   - 重构前已完整备份 Python 全量代码至：
     - 文件夹：`../antigravity-token-monitor-backup-python`
     - 压缩包：`../antigravity-token-monitor-backup-python.zip` (879 KB)

---

## 🎯 核心功能与交互设计

### 1. “打开时先只显示今天，同时完整统计全部历史天数”
* **默认今日视图 (Today View First)**：
  打开浏览器访问大盘时，系统自动定位并展示今日（2026-10-10）数据，避免海量历史数据造成视觉杂乱。
* **每日流水时间轴 (Daily Ledger Timeline)**：
  大盘上方直观横向陈列全部 41 天的用量卡片。今日卡片带有 `[今日]` 标识并处于高亮选中状态。
* **一键无缝穿梭**：
  - 点击任何历史日期卡片，全屏指标、模型分布与会话列表即刻切换为该日的精确流水；
  - 在顶部日期下拉框选择 `🌐 全部历史天数 (All Time)`，瞬间总览全量 **30.71 亿 Tokens** 的全局大盘；
  - 再次点击卡片或选择今日，随时秒切回今日看板。

### 2. 精准天数切片技术 (Date-Sliced Ledger)
针对跨天活跃的长期会话，引擎将其 Token 消耗精确拆解并打标至各自发生的日期，彻底解决“昨天创建的会话今天聊了很多却算在昨天”的问题。

---

## 🚀 启动与使用

### 访问地址
后台常驻服务已启动在：
👉 **http://localhost:8765**

### Windows 一键脚本
- 启动：双击运行 `start_monitor.bat` 或在 PowerShell 中执行 `.\start_monitor.ps1 -Background`
- 停止：双击运行 `stop_monitor.bat` 或在 PowerShell 中执行 `.\stop_monitor.ps1`
- 手动运行：`.\monitor.exe -port 8765`
