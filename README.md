# Wukong  🐵

> **本地优先、可扩展的 AI Agent 平台** | Go | tRPC 生态 | 10 工作流 | 64 源文件 | ~18,000 行

Wukong 是一个本地优先、可扩展的 AI Agent 平台，基于 tRPC-Agent-Go v1.10.0、tRPC-MCP-Go 和 tRPC-A2A-Go 构建。

---

## 功能矩阵

| 领域 | 特性 |
|------|------|
| **Agent 引擎** | 交互式工具调用循环 · 10 种工作流 · 两遍上下文压缩 · Token 预算 · 消息上限 500 |
| **多 Agent** | Chain/Parallel/Cycle/Graph · Team(Coordinator/Swarm) · AgentTool · A2A |
| **外部 Agent** | Claude Code · Codex · Dify 平台 · 远程 A2A |
| **LLM Provider** | OpenAI · Anthropic · Google · DeepSeek · Ollama · LMStudio (6 种) |
| **MCP 工具** | 10 内置扩展 · 外部 STDIO/SSE/HTTP · MCP Broker · Tool Filter(glob) · SessionReconnect |
| **浏览器自动化** | Chromedp(CDP) · 9 个工具(navigate/extract/screenshot/click/fill/web_fetch) · 文件缓存 · 2 浏览模式 |
| **Web 搜索** | DuckDuckGo + 预留 SearXNG/Tavily · search_backend 配置 |
| **RAG 知识库** | OpenAI Embedding · Inmemory Vector · dir/URL 源 · knowledge_search |
| **长期记忆** | SQLite 持久化 · 异步提取(3w) · 6 手动工具 · WaitGroup 优雅停止 · 5s 超时 |
| **会话管理** | SQLite/Redis/Inmemory · 异步摘要 · TTL · 事件分页 |
| **安全防护** | 4 级权限(smart) · Allowlist/Denylist · 命令拦截 · Prompt注入检测 · web_fetch 高危标记 |
| **上下文优化** | 两遍压缩(占位符+截断) · 修订摘要 · Per-Tool控制 · Token裁剪 |
| **可观测性** | OpenTelemetry Tracing · Langfuse LLM追踪 · 结构化日志 · 健康检查 |
| **制品存储** | Inmemory(默认) · COS(云端) |
| **分布式** | A2A Server · AG-UI SSE · Redis Session |
| **评估** | EvalSet · 4 指标 · CLI 回归 · JSON 结果 |
| **HITL** | Graph 中断/恢复 · 静态/动态/外部 3 种 |
| **TUI** | Bubbletea + Lipgloss · Ctrl+C 流式取消 · 消息上限 · 友好错误(401/429/500) |

---

## 快速开始

```bash
git clone https://github.com/km269/wukong.git && cd wukong
go build -o wukong ./cmd/wukong/

# 最小 config.yaml
default_provider: "openai"
providers:
  - name: "openai"
    type: "openai"
    api_key: "${OPENAI_API_KEY}"
    model: "gpt-4o"

./wukong session                     # 交互会话
./wukong session --provider deepseek # 指定 Provider
./wukong eval                        # 回归测试
./wukong extension list              # 扩展管理
./wukong configure                   # 配置向导
```

---

## 项目结构 (64 源文件)

```
wukong/
├── cmd/wukong/main.go          入口
├── internal/
│   ├── agent/       CoreLoop · WorkflowBuilder(10模式) · TeamBuilder · DifyAgent · HITL · TodoEnforcer
│   ├── apps/        HTML 应用文件管理
│   ├── artifact/    inmemory / COS 制品工厂
│   ├── browser/     HTTP + Chromedp(CDP) 双模引擎 · Click/Fill 修复泄漏
│   ├── cli/+tui/    6子命令 · Bubbletea TUI · Ctrl+C 流式取消
│   ├── codemode/    goja JS 沙箱
│   ├── config/      Viper 配置 · 28 段 · 453行
│   ├── eval/        EvalSet/Metric/Evaluator
│   ├── extension/   MCP 管理器 + 10内置 + MCP Client(stdio/sse/http)
│   ├── knowledge/   RAG(Embedding+Vector+Source)
│   ├── memory/      GracefulShutdown(WaitGroup+timeout+isClosing)
│   ├── observability/ Langfuse OTLP
│   ├── provider/    6种 LLM 工厂
│   ├── recall/      FTS5 跨会话搜索
│   ├── security/    4级权限 + 8高危工具(web_fetch新增)
│   ├── server/      AG-UI SSE
│   ├── session/     sqlite/redis 会话
│   ├── skill/       Agent Skill 仓库
│   ├── summon/      A2A + 凭证轮换
│   ├── telemetry/   OTel
│   ├── todo/        任务跟踪
│   └── util/        DB池(WAL checkpoint) · 日志
├── config.yaml      28段 · 453行
└── docs: README / ARCHITECTURE / CONFIG
```

---

## 技术栈

tRPC-Agent-Go v1.10.0 · tRPC-MCP-Go v0.0.16 · Bubbletea+lipgloss · SQLite(WAL) · go-redis/v9 · COS SDK · Chromedp · goja · OTel · Langfuse · Cobra+Viper

---

## 文档

[README](README.md) · [ARCHITECTURE](ARCHITECTURE.md) · [CONFIG](CONFIG.md)
