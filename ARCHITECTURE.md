# Wukong 系统架构文档

> **版本**: v0.2.0 | **语言**: Go 1.26 | **框架**: tRPC-Agent-Go v1.10.0

---

## 目录

1. [分层架构](#1-分层架构)
2. [核心数据流](#2-核心数据流)
3. [核心引擎层](#3-核心引擎层)
4. [服务层详解](#4-服务层详解)
5. [存储层](#5-存储层)
6. [扩展系统](#6-扩展系统)
7. [安全体系](#7-安全体系)
8. [模块依赖关系](#8-模块依赖关系)
9. [设计决策与ADRs](#9-设计决策与adrs)

---

## 1. 分层架构

```
┌──────────────────────────────────────────────────────────────┐
│                    CLI 层 (cmd/wukong + internal/cli)         │
│  main.go → cli.Execute() → 6 子命令 → TUI 界面                │
├──────────────────────────────────────────────────────────────┤
│                    Handler 层 (internal/cli/session.go)       │
│  bootstrapSession() → 按序初始化全部子系统 → 创建 CoreLoop     │
├──────────────────────────────────────────────────────────────┤
│                    核心引擎层 (internal/agent/)               │
│  ┌───────────────┐ ┌────────────────┐ ┌────────────────────┐ │
│  │ CoreLoop      │ │ WorkflowBuilder│ │ ContextRevEngine   │ │
│  │ Run/RunStream │ │ 5种编排模式    │ │ Token预算+压缩      │ │
│  └───────────────┘ └────────────────┘ └────────────────────┘ │
│  ┌───────────────┐ ┌────────────────┐                        │
│  │ TodoEnforcer  │ │ HITL 中断/恢复 │                        │
│  └───────────────┘ └────────────────┘                        │
├──────────────────────────────────────────────────────────────┤
│                     服务层 (internal/*/)                      │
│  Provider │ Extension │ Session │ Memory │ Summon │ Security │
│  Factory  │  Manager  │ Service │ Mgr    │ Mgr    │  Guard   │
│ ──────────┼───────────┼─────────┼────────┼────────┼──────────│
│ Knowledge │  Recall   │ Artifact│ Code   │ Server │ Observ.  │
│  RAG      │   Store   │ Factory │ Mode   │ AG-UI  │ Langfuse │
├──────────────────────────────────────────────────────────────┤
│                   存储层 (共享 SQLite 连接池)                  │
│  Session(events) │ Memory(facts) │ Todo(tasks) │ Recall(FTS5) │
│  ────────────────┼───────────────┼─────────────┼──────────────│
│            wukong.db (WAL, FTS5, 共享连接池)                  │
└──────────────────────────────────────────────────────────────┘
```

### 层间通信规则

- **向下依赖**：上层可依赖下层，下层不可依赖上层
- **水平隔离**：同层模块通过接口通信，无直接依赖
- **存储共享**：SQLite 模块共享 `util.DatabasePool`，Redis 模块独立连接

---

## 2. 核心数据流

### 2.1 单次对话完整生命周期

```
用户输入 (TUI)
  │
  ▼
CoreLoop.RunStream(ctx, userID, sessionID, message)
  │
  ├─[1] OpenTelemetry Span 创建
  ├─[2] ContextManager.PrepareContext()           // Token预算检查
  ├─[3] RecallStore.StoreMessage(user)            // 存档用户消息
  ├─[4] Runner.Run(ctx, userID, sessionID, msg)   // tRPC Runner
  │     │
  │     ├── Session 加载历史事件
  │     ├── System Instruction 构建
  │     │     ├── Top of Mind 持久化指令
  │     │     ├── Memory 预加载 (PreloadMemory)
  │     │     └── Session Recall (PreloadSessionRecall)
  │     ├── LLM Agent 推理 (GenerationConfig)
  │     ├── 工具调用循环
  │     │     ├── ToolSearch Plugin (TopK 过滤)
  │     │     ├── Security Guard (权限检查)
  │     │     ├── 并行工具执行 (ParallelTools)
  │     │     ├── Tool Retry (指数退避+抖动)
  │     │     ├── Context Compaction (两遍压缩)
  │     │     └── Post Tool Prompt (工具提醒)
  │     ├── TodoEnforcer Plugin (完成校验)
  │     ├── Guardrail Plugin (注入检测)
  │     └── 返回事件流 (<-chan *event.Event)
  │
  ├─[5] 遍历事件流，提取响应文本 + 工具调用统计
  ├─[6] RecallStore.StoreMessage(assistant)       // 存档助手回复
  ├─[7] ContextManager.AfterRun()                 // Token更新+摘要触发
  └─[8] 返回最终响应文本
```

### 2.2 事件流处理

```
runner.Run() 返回事件流
  │
  ├── chat.completion.chunk    → Delta.Content → 流式文本
  ├── tool_call                → ToolCalls → 工具执行
  ├── tool_result              → 工具结果 → 反馈给模型
  ├── runner.completion        → StateDelta["last_response"] → 最终结果
  └── error                    → Error.Message → 错误处理
```

---

## 3. 核心引擎层

### 3.1 CoreLoop (`internal/agent/loop.go`)

**结构体定义**:

```go
type CoreLoop struct {
    agent          agent.Agent      // LLM Agent 实例
    runner         runner.Runner    // tRPC Runner
    sessionService session.Service  // 会话服务
    factory        *provider.Factory // 模型工厂
    cfg            *config.WukongConfig
    contextMgr     *ContextManager  // 上下文管理器
    security       *security.Guard  // 安全守卫
    recallStore    *recall.Store    // 召回存储
    closeFn        func() error     // 资源清理链
}
```

**关键方法**:

| 方法 | 功能 |
|------|------|
| `NewCoreLoop(cfg)` | 收集工具 → 创建Agent → 创建Runner → 配置插件 → 创建ContextManager |
| `Run(ctx, uid, sid, msg)` | 执行单条消息，返回原始事件流 |
| `RunStream(ctx, uid, sid, msg, onEvent)` | 流式处理，提取最终文本，存储召回，触发上下文优化 |
| `RunUserMessage(ctx, uid, sid, content)` | 便捷方法，构建 UserMessage 并返回文本 |
| `Close()` | 顺序清理：Memory → Runner → Session → Telemetry → Database |

**Agent 创建时配置的 tRPC 选项**:

| 选项 | 说明 |
|------|------|
| `WithPreloadMemory(10)` | 预加载最新10条记忆到 System Prompt |
| `WithEnableContextCompaction(true)` | 启用两遍工具结果压缩 |
| `WithEnableParallelTools(true)` | 并行执行独立工具调用 |
| `WithToolCallRetryPolicy(...)` | 指数退避重试（3次） |
| `WithEnablePostToolPrompt(true)` | 工具执行后提醒模型 |
| `WithPreloadSessionRecall(limit)` | 跨会话上下文注入 |
| `WithPlanner(builtin.New())` | BuiltinPlanner / ReActPlanner |

### 3.2 WorkflowBuilder (`internal/agent/workflow.go`)

支持 5 种 Agent 编排模式：

| 模式 | tRPC Agent | 适用场景 | 实现 |
|------|-----------|---------|------|
| **Single** | `llmagent.New()` | 标准单 Agent | 直接创建 LLMAgent |
| **Chain** | `chainagent.New()` | 顺序流水线（Plan→Exec→Review） | `buildChainAgent()` |
| **Parallel** | `parallelagent.New()` | 并发多维度分析 | `buildParallelAgent()` |
| **Cycle** | `cycleagent.New()` | 迭代优化循环 | `buildCycleAgent()` + EscalationFunc |
| **Graph** | `graphagent.New()` | 条件路由 DAG | `buildGraphAgent()` + StateGraph |

**Cycle 模式内置策略**:

| 策略 | 子Agent | 退出条件 |
|------|---------|---------|
| `default` | planner → executor | `TASK_COMPLETE` |
| `code_review` | generator → reviewer | `CODE_APPROVED` |

**Graph 模式默认路由**:
```
analyze → [code_task → code] / [search_task → search] / [question → answer] → review
```

### 3.3 ContextManager (`internal/agent/context.go`)

Token 预算管理体系：

| 机制 | 触发条件 | 行为 |
|------|---------|------|
| **PrepareContext** | 每次 Run 前 | 检查 Token 是否超阈值，触发异步摘要 |
| **AfterRun** | 每次 Run 后 | 更新 Token 估算，缓存最近输出 |
| **Context Compaction** | 每次模型调用前 | Pass1: 旧工具结果占位符化 / Pass2: 超大结果截断 |
| **Revision Summarize** | Token 超 `max*trim_ratio` 阈值 | 用 RevisionModel 生成摘要 |

### 3.4 HITL 中断 (`internal/agent/hitl.go`)

支持三种中断模式：

| 模式 | API | 说明 |
|------|-----|------|
| **静态中断** | `graph.WithInterruptBefore/After()` | 声明节点时配置 |
| **动态中断** | `graph.Interrupt(ctx, state, key, meta)` | 运行时条件触发 |
| **外部中断** | `graph.WithGraphInterrupt(ctx)` | 外部 goroutine 暂停 |

---

## 4. 服务层详解

### 4.1 Provider Factory (`internal/provider/`)

```go
// 支持 6 种 Provider
openai / anthropic / google / deepseek / ollama / lmstudio
// 全部走 OpenAI 兼容 API
```

| 方法 | 功能 |
|------|------|
| `CreateModel(name)` | 按名称创建模型实例 |
| `CreateDefaultModel()` | 使用默认 Provider |
| `CreateRevisionModel()` | 创建摘要用模型（可选独立 Provider） |
| `GetDefaultGenerationConfig()` | 从 AgentConfig 提取参数 |

### 4.2 Extension Manager (`internal/extension/`)

管理 MCP 扩展的完整生命周期：

```
Extension Manager
├── 内置扩展 (builtin/)
│   ├── developer          → file_read/write/replace, command_execute
│   ├── computer_controller → web_fetch, browser_navigate/extract/screenshot
│   ├── memory             → memory_add/search/update/delete/load/clear
│   ├── auto_visualiser    → visualiser_chart/diagram/table
│   ├── tutorial           → tutorial_start/list/step
│   ├── top_of_mind        → 持久化指令工具
│   ├── code_mode          → JS 沙箱执行
│   ├── apps               → HTML 应用管理
│   ├── agent_tools        → code-reviewer/summarizer/code-generator
│   └── web                → web_search
│
└── 外部 MCP (mcp_client.go)
    ├── stdio              → 子进程通信 (npx/uvx)
    ├── sse                → HTTP SSE
    └── streamable         → HTTP 流式
    └── MCP Broker         → 按需工具发现 (mcp_list_servers/call)
```

**扩展管理工具**: `extension_list` / `extension_enable` / `extension_disable` / `extension_add_deeplink`

### 4.3 Session Service (`internal/session/`)

| 后端 | 适用场景 | 实现 |
|------|---------|------|
| **sqlite** | 本地持久化（默认） | `tRPC session/sqlite` |
| **memory** | 开发测试 | `tRPC session/inmemory` |
| **redis** | 生产分布式 | `Wukong 原生实现` (go-redis/v9) |

### 4.4 Knowledge RAG (`internal/knowledge/`)

```
Knowledge Manager
├── Embedder (OpenAI compatible)
│   └── text-embedding-3-small (1536维)
├── VectorStore (inmemory)
│   └── 余弦相似度搜索
├── Sources
│   ├── dir.New()   → 目录递归扫描
│   └── url.New()   → URL 远程文档
└── Search Tool
    └── knowledge_search → 语义检索
```

### 4.5 Memory Manager (`internal/memory/`)

```
MemoryManager
├── Auto Extract (auto_extract: true)
│   ├── Runner 完成后触发异步提取
│   ├── Extractor 模型分析对话
│   └── Worker 池 (3 workers)
└── Manual Tools
    ├── memory_add/search/update/delete
    ├── memory_load/clear
    └── 始终对 Agent 可见
```

### 4.6 Summon + Skill (`internal/summon/` + `internal/skill/`)

双通道子代理系统：

| 系统 | 来源 | 格式 | 并发 |
|------|------|------|------|
| **Summon** | `.wukong_skills/*.md` | 单个 MD 文件 | max_concurrent: 5 |
| **Skill** | `.wukong_agent_skills/<name>/SKILL.md` | 目录+SKILL.md | 不限 |

### 4.7 新增服务

| 服务 | 目录 | 说明 |
|------|------|------|
| **AG-UI Server** | `internal/server/` | SSE 实时对话，兼容 CopilotKit |
| **Evaluation** | `internal/eval/` | EvalSet + 4 种 Metric + CLI |
| **Observability** | `internal/observability/` | Langfuse OTLP LLM 追踪 |
| **Artifact** | `internal/artifact/` | inmemory/COS 制品存储工厂 |

---

## 5. 存储层

### 5.1 共享 SQLite 连接池

```
util.DatabasePool (wukong.db)
  ├── WAL 模式 + 外键约束
  ├── 所有模块共享同一个 *sql.DB
  ├── noCloseDBWrapper 防止单模块关闭
  └── 关闭顺序: Memory → Session → DB Pool
```

### 5.2 各模块存储

| 模块 | 存储 | 表/机制 |
|------|------|---------|
| Session | tRPC session/sqlite | events, summaries |
| Memory | tRPC memory/sqlite | memories |
| Todo | custom SQLite | tasks (id,title,desc,status) |
| Recall | custom SQLite | FTS5 全文索引 |

---

## 6. 扩展系统

### 6.1 MCP 协议传输

| 传输 | 实现 | 适用场景 |
|------|------|---------|
| **stdio** | 子进程 STDIN/STDOUT | 本地进程 (npx, uvx) |
| **sse** | HTTP Server-Sent Events | 远程服务 |
| **streamable** | HTTP 流式 | 兼容性最佳 |

### 6.2 MCP 高级特性

| 特性 | 说明 |
|------|------|
| **MCP Broker** | 聚合多个 MCP 服务器，按需发现工具 |
| **Tool Filter** | glob 模式 include/exclude |
| **Session Reconnect** | SSE/Streamable 断线自动重连 (3次) |
| **Deeplink 安装** | `wukong://extension?name=...` 一键安装 |

---

## 7. 安全体系

### 7.1 四层防御

```
Layer 1: PermissionMode (auto/smart/manual/chat_only)
Layer 2: Allowlist/Denylist 细粒度控制
Layer 3: Command Pattern 拦截 (rm -rf /, dd, mkfs, fork bomb)
Layer 4: Guardrail Plugin (Prompt Injection 检测)
```

### 7.2 工具执行安全检查

```
BeforeTool Callback
  → SecurityGuard.CheckToolPermission(toolName)
  → Denylist 检查 → Allowlist 检查 → PermissionMode
  → Command Validation (shell 工具)
  → Execute / Block / Request Approval
```

---

## 8. 模块依赖关系

```
cmd/wukong/main.go → cli.Execute()
  └── cli/session.go (bootstrapSession)
      ├── config.Loader
      ├── telemetry.Manager
      ├── provider.Factory
      ├── session.NewSessionService     ← SQLite/Redis/memory
      ├── memory.NewMemoryManager       ← SQLite
      ├── extension.Manager
      │   ├── builtin (10 extensions)
      │   └── mcp_client (MCP ToolSets)
      ├── recall.Store                  ← FTS5
      ├── topofmind.Manager
      ├── codemode.Executor
      ├── apps.Manager
      ├── summon.Manager
      │   ├── a2a (A2A Server + Agent)
      │   └── auth (CredentialRotator)
      ├── skill.Manager
      ├── knowledge.Manager             ← RAG
      ├── artifact.NewService           ← COS/inmemory
      ├── observability.StartLangfuse   ← OTLP
      ├── todo.TodoManager
      └── agent.NewCoreLoop
          ├── runner.NewRunner
          ├── llmagent.New (or WorkflowBuilder)
          ├── todoenforcer (plugin)
          ├── guardrail (plugin)
          ├── toolsearch (plugin)
          └── contextMgr (token budget)
```

**关键依赖方向**:
- `agent/` 不依赖 `cli/` 或 `tui/`（可独立测试）
- `extension/` 不依赖 `agent/`（可独立测试）
- `security/` 纯业务逻辑，无外部依赖
- `provider/` 纯模型工厂，无业务依赖
- `util/` 所有模块的底层工具库

---

## 9. 设计决策与 ADRs

### ADR-1: 为什么选 tRPC 框架而非自建 Agent 循环？

- **Session 管理**：内置事件持久化、摘要、TTL
- **Memory 系统**：auto-extract 模式 + 异步提取
- **Tool 系统**：标准化注册/调用/回调机制
- **Runner 抽象**：统一事件流，支持多 Agent 类型
- **生态整合**：与 tRPC-MCP-Go / tRPC-A2A-Go 无缝协作

### ADR-2: 为什么用 SQLite 共享连接池？

- **零配置**：无需安装数据库服务器
- **本地优先**：数据文件随项目移动
- **WAL模式**：支持并发读写
- **FTS5**：内置全文搜索（Recall 系统）
- **连接池共享**：避免多模块竞争锁

### ADR-3: 为什么分离 Memory Tools 和 Auto Extract？

- **容错性**：Extractor 模型不可用时，Agent 仍可手动管理
- **可控性**：用户可精确添加/删除特定记忆
- **灵活性**：Auto extract 自动捕获隐含信息，Manual tools 精确管理

### ADR-4: 安全设计原则

- **默认安全**：permission_mode 默认为 smart
- **纵深防御**：Allowlist + Denylist + PermissionMode + 命令拦截
- **最小权限**：扩展可通过 Permissions 限制特定工具
- **超时保护**：所有工具默认 30s 超时

### ADR-5: 上下文压缩两遍策略

- **Pass 1**：旧工具结果占位符化（保留 ToolID/ToolName）
- **Pass 2**：超大结果首尾截断（保留头尾+中间标记）
- **Per-Tool 配置**：ForceClean 强制清理高噪声工具，Keep 保护关键工具
