# Wukong 系统架构文档

> **版本**: v0.4.0 | **Go**: 1.26 | **源文件**: 64 (.go) + 26 (_test.go) | **~18,000 行**

---

## 目录

1. [分层架构](#1-分层架构)
2. [核心数据流](#2-核心数据流)
3. [工作流引擎](#3-工作流引擎)
4. [服务层详解](#4-服务层详解)
5. [存储层](#5-存储层)
6. [浏览器 & Web 系统](#6-浏览器--web-系统)
7. [安全体系](#7-安全体系)
8. [关闭与恢复](#8-关闭与恢复)
9. [模块依赖](#9-模块依赖)
10. [设计决策 (ADR)](#10-设计决策-adr)

---

## 1. 分层架构

```
┌──────────────────────────────────────────────────────────────────┐
│ CLI | cmd/wukong + cli/ + tui/ | 6 子命令 (session/configure/extension/eval/version/completion)
├──────────────────────────────────────────────────────────────────┤
│ Handler | cli/session.go::bootstrapSession() | ~30 子系统串行初始化
├──────────────────────────────────────────────────────────────────┤
│ 引擎层 | agent/
│  CoreLoop · WorkflowBuilder(10) · TeamBuilder · DifyAgent · HITL · TodoEnforcer
├──────────────────────────────────────────────────────────────────┤
│ 服务层 | internal/*/
│  Provider(6) · Extension(10内置+MCP) · Session(sqlite/redis) · Memory(GracefulShutdown)
│  Knowledge(RAG) · Recall(FTS5) · Artifact(COS) · Browser(HTTP+Chromedp)
│  CodeMode(goja) · Server(AG-UI) · Observability(Langfuse) · Eval
├──────────────────────────────────────────────────────────────────┤
│ 存储层 | wukong.db(WAL,FTS5) + Redis + COS
└──────────────────────────────────────────────────────────────────┘
```

---

## 2. 核心数据流

```
用户输入 → TUI
  → CoreLoop.RunStream(ctx, userID, sessionID, msg)
    ├─ OTel Span
    ├─ ContextManager.PrepareContext()            Token 预算
    ├─ RecallStore.StoreMessage(user)
    ├─ Runner.Run()
    │   ├─ Session 历史 + System Instruction (TopOfMind+Memory+SessionRecall)
    │   ├─ LLM Agent (Planner+GenConfig)
    │   ├─ 工具循环: ToolSearch+Security+Parallel+Retry+Compaction+PostPrompt
    │   ├─ TodoEnforcer + Guardrail
    │   └─ <-chan *event.Event
    ├─ 遍历事件流 → 文本 + 工具统计
    ├─ RecallStore.StoreMessage(assistant)
    └─ ContextManager.AfterRun() → Token更新+摘要触发
```

**关闭链 (5步)**: Runner → Memory(WaitGroup 5s) → Session → Telemetry+Langfuse → DB(WAL checkpoint+Close)

---

## 3. 工作流引擎 (10 种)

| # | 模式 | 实现 | 说明 |
|---|------|------|------|
| 1 | `single` | `llmagent.New()` | 标准单 Agent |
| 2 | `chain` | `chainagent.New()` | 顺序流水线 (plan→exec→review) |
| 3 | `parallel` | `parallelagent.New()` | 并发多专家 |
| 4 | `cycle` | `cycleagent.New()` | 迭代优化 (default/code_review) |
| 5 | `graph` | `graphagent.New()` | 条件路由 DAG |
| 6 | `team_coordinator` | `team.New()` | 协调者+AgentTool 委托 |
| 7 | `team_swarm` | `team.NewSwarm()` | Agent 直接 transfer |
| 8 | `claude_code` | `claudecode.New()` | 本地 Claude Code CLI |
| 9 | `codex` | `codex.New()` | 本地 Codex CLI |
| 10 | `dify` | `DifyAgent`(自研) | Dify Chat API (blocking+SSE) |

**Team 模式**: Coordinator(协调者+AgentTool) / Swarm(transfer,交叉请求,独立成员历史,20次handoff限制)

---

## 4. 服务层详解

### 4.1 Provider (6种, OpenAI兼容API)
`openai` | `anthropic` | `google` | `deepseek` | `ollama` | `lmstudio`

### 4.2 Extensions
```
10 内置: developer | computer_controller | memory | visualiser | tutorial
         web | agent_tools(code-reviewer/summarizer/code-generator)
         top_of_mind | code_mode | apps
外部 MCP: stdio(npx/uvx) | sse | streamable HTTP
  MCP Broker(按需) | Tool Filter(glob include/exclude) | SessionReconnect(3次)
```

### 4.3 Computer Controller (9 工具)
`web_fetch` | `file_cache` | `cache_list` | `cache_clear` | `browser_navigate` | `browser_extract` | `browser_screenshot` | **`browser_click`** | **`browser_fill`**

### 4.4 Memory (Graceful Shutdown)
3 async workers · extract_timeout 120s · `trackingMemoryService`(WaitGroup+isClosing) · Close 时 5s 超时等待

### 4.5 Knowledge RAG
OpenAI embedder(text-embedding-3-small,1536维) → Inmemory VectorStore(余弦) → dir/URL Sources → knowledge_search 工具

---

## 5. 存储层

```
wukong.db (WAL, MaxOpenConns=1)
  Session(tRPC sqlite) · Memory(tRPC sqlite) · Todo(custom) · Recall(FTS5)
Redis (go-redis/v9, session backend)
COS (cos-go-sdk-v5, artifact backend)
```

---

## 6. 浏览器 & Web 系统

```
browser/controller.go
  ├── HTTP 模式: net/http (默认/回退)
  └── Chromedp 模式: CDP协议 · 真实浏览器 · JS渲染 · PNG截图
        allocCancel 保存 → Close 杀进程 (修复泄漏)
  9 个 Agent 工具:
    web_fetch(HTTP) · file_cache · cache_list · cache_clear
    browser_navigate · browser_extract · browser_screenshot
    browser_click(NEW) · browser_fill(NEW)

web.go: DuckDuckGo Instant Answer + search_backend 预留(SearXNG/Tavily)
安全: web_fetch/browser_navigate/screenshot/click/fill 标记高危 (smart 需审批)
```

---

## 7. 安全体系

```
4 层:
  PermissionMode(auto/smart/manual/chat_only)
  Allowlist/Denylist
  Command Pattern 拦截 (rm -rf /, dd, mkfs, fork bomb)
  Guardrail Plugin (Prompt Injection)

8 高危工具: bash/execute_command/run_command/shell/terminal/command/command_execute
           file_write/file_replace/file_delete
           browser_navigate/browser_screenshot/browser_click/browser_fill
           web_fetch
```

---

## 8. 关闭与恢复

| 机制 | 实现 |
|------|------|
| 优雅关闭 | 5步链: Runner→Memory(WaitGroup 5s)→Session→Telemetry+Langfuse→DB(WAL checkpoint+Close) |
| Chromedp 泄漏 | allocCancel 保存, Close 杀浏览器进程 |
| Ctrl+C 流式取消 | context.Cancel → goroutine 退出 → 显示 "[Request cancelled]" |
| 消息上限 | 500 条自动 trim |
| 崩溃恢复 | WAL 下次打开自动 replay |

---

## 9. 模块依赖

```
cmd/main → cli.Execute → session.bootstrapSession
  config · telemetry · provider · session · memory
  extension.Manager(10内置+MCP) · recall · topofmind · codemode · apps
  summon · skill · knowledge · artifact · observability
  → agent.NewCoreLoop(WorkflowBuilder/TeamBuilder,DifyAgent,plugins,contextMgr)
```

---

## 10. 设计决策 (ADR)

| ADR | 决策 | 理由 |
|-----|------|------|
| 1 | tRPC 框架 | Session/Memory/Tool 标准化 |
| 2 | SQLite 共享池 WAL | 零配置+并发+FTS5 |
| 3 | Memory Tools+AutoExtract 分离 | 容错+可控 |
| 4 | 安全默认 smart | 纵深防御 |
| 5 | ContextCompaction 两遍 | 占位符+截断 |
| 6 | Memory GracefulShutdown | WaitGroup+超时 |
| 7 | Ctrl+C 流式取消 | cancelCtx→goroutine退出 |
| 8 | Dify/Codex/ClaudeCode 自研 | v1.10.0 未含对应包 |
| 9 | web_fetch 高危标记 | 防 SSRF |
| 10 | allocCancel 杀进程 | 防 Chrome 僵尸进程 |
