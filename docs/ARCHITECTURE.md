# Wukong 系统架构深度分析

> **版本**: v0.6.1 | **Go**: 1.26 | **总源文件**: 119 `.go` + 34 `_test.go` | **直接依赖**: 30
> **外发包**: `pkg/sandbox/` (10 文件)
>
> 基于 [tRPC-Agent-Go v1.10.0](https://github.com/trpc-group/trpc-agent-go) · [tRPC-MCP-Go v0.0.16](https://github.com/trpc-group/trpc-mcp-go) · [tRPC-A2A-Go v0.2.5](https://github.com/trpc-group/trpc-a2a-go) · [CortexDB v2.25.0](https://github.com/liliang-cn/cortexdb)

---

## 目录

1. [架构哲学](#1-架构哲学)
2. [系统全景](#2-系统全景)
3. [Agent 执行引擎 (CoreLoop)](#3-agent-执行引擎-coreloop)
4. [双引擎记忆系统](#4-双引擎记忆系统)
5. [多 Agent 编排](#5-多-agent-编排)
6. [扩展与工具系统](#6-扩展与工具系统)
7. [安全纵深防御](#7-安全纵深防御)
8. [上下文管理](#8-上下文管理)
9. [LLM Provider 体系](#9-llm-provider-体系)
10. [配置系统](#10-配置系统)
11. [服务与协议层](#11-服务与协议层)
12. [存储架构](#12-存储架构)
13. [技能自进化系统](#13-技能自进化系统)
14. [完整数据流](#14-完整数据流)
15. [技术选型](#15-技术选型)
16. [关键设计决策 (ADR)](#16-关键设计决策-adr)

---

## 1. 架构哲学

Wukong 的五大核心哲学决定了所有工程决策：

| 哲学 | 核心信念 | 关键工程决策 |
|------|----------|-------------|
| **记忆优先** | Agent 智能来源于知识积累 | 双引擎记忆闭环 + 知识图谱 + HNSW 向量 |
| **框架组装** | 组件应可替换 | CoreLoop 依赖注入，27 个子系统解耦 |
| **多 Agent 原生** | 编排是第一公民 | 10 种显式模式，非事后插件 |
| **进化智能** | 技能应自我改进 | LLM 分析 → 自动补丁 → 版本管理 → 热重载 |
| **纵深防御** | 安全是多层协同 | 5 层防御从 LLM 权限到 OS 内核 |

---

## 2. 系统全景

```
┌──────────────────────────────────────────────────────────────────────┐
│                        Wukong AI Agent Platform                      │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│ Entry Points:                                                        │
│   CLI (cobra + BubbleTea TUI)   │ API (A2A:9090/ACP:9091/AG-UI:8080)│
│                                                                      │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│ Core Engine: CoreLoop ─── 中央编排器，协调 27 个子系统                 │
│   ├── WorkflowBuilder (10 种模式) + TeamBuilder (多 Agent)            │
│   ├── ContextRevisionEngine (3 层压缩)                                │
│   └── Security Guard (5 层防御)                                       │
│                                                                      │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│ Agent Framework: tRPC-Agent-Go Runner                                │
│   LLMAgent / ChainAgent / ParallelAgent / CycleAgent / GraphAgent     │
│   Session Service / Memory Service / Artifact Service                │
│   Planner / ToolSearch / ContextCompaction / Skill                   │
│                                                                      │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│ Memory Stack (双引擎):                                                │
│ ┌─ tRPC Memory (SQLite KV) ────────────────────────────────────┐    │
│ │  AutoExtract → 异步记忆提取 (9B 模型) → ReadMemories          │    │
│ │  6 tools: add/search/update/delete/load/clear                 │    │
│ │  SmartCleanup: 容量感知评分淘汰 (80%→60%)                     │    │
│ ├──────────────────────────────────────────────────────────────┤    │
│ │  CortexDB Stack (HNSW + FTS5 + RDF)                          │    │
│ │  ┌── MemoryFlow: IngestTurn → WakeUp → PromoteFacts           │    │
│ │  ├── GraphFlow: 实体提取 → RDF KG (auto_extract per turn)    │    │
│ │  ├── ImportFlow: DDL/CSV → KG 导入                            │    │
│ │  └── CortexStore: HNSW 向量 + FTS5 全文 (含工具消息索引)     │    │
│ └──────────────────────────────────────────────────────────────┘    │
│                                                                      │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│ Capability Layer:                                                    │
│   Security Guard · Extension Manager · Evolution Engine              │
│   Knowledge (RAG) · Browser (Chromedp) · CodeMode (goja JS)         │
│   Skill System · Summon (A2A) · pkg/sandbox (OS lock)               │
│   Health Check · Todo System · TopOfMind · Project Manager          │
│                                                                      │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│ Infrastructure:                                                      │
│   Provider Factory (7 LLM backends) · Viper Config Loader            │
│   DatabasePool (shared SQLite WAL) · OpenTelemetry + Langfuse       │
│                                                                      │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│ Storage: wukong.db (单文件 SQLite WAL, shared DatabasePool)          │
│   sessions / memories / recall(FTS5) / todos / projects              │
│   cortex: transcripts(episodes) / entities / relations / HNSW vecs  │
│   evolution: skill_versions / evolution_records                     │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 3. Agent 执行引擎 (CoreLoop)

### 3.1 CoreLoop 结构定义

`CoreLoop` (`internal/agent/loop.go`, ~1560 行) 是整个系统的中央编排器。通过依赖注入持有 27 个子系统的引用：

```go
type CoreLoop struct {
    agent          agent.Agent          // tRPC-Agent-Go Agent 实例
    runner         runner.Runner        // tRPC-Agent-Go Runner
    sessionService session.Service      // 会话管理
    memoryService  memory.Service       // tRPC Memory Service
    factory        *provider.Factory    // LLM Provider 工厂
    cfg            *config.WukongConfig // 完整配置
    contextMgr     *ContextManager      // 上下文压缩引擎
    security       *security.Guard      // 安全守卫
    recallStore    *recall.Store        // FTS5 对话搜索
    cortexStore    *cortex.CortexStore  // HNSW 向量存储
    memoryFlow     *cortex.MemoryFlowService // 转录回溯
    graphFlow      *cortex.GraphFlowService // 知识图谱
    evolution      *evolution.Engine    // 技能进化引擎
    // ... 更多子系统通过 BootstrapState 注入
    closeFn        func() error         // 组合关闭函数
    mu     sync.RWMutex                 // 状态保护锁
    closed bool
    bgWg   sync.WaitGroup               // 后台 goroutine 追踪
}
```

### 3.2 BootstrapState — 子系统组装

会话启动 (`bootstrapSession`) 时的完整引导序列，包含所有子系统的创建和初始化：

```
bootstrapSession() ── 会话引导函数 (internal/cli/session.go)
├── CreateLoader()                    ── 配置加载器 (Viper)
├── SetupLogging()                    ── 日志系统 (slog)
├── StartTelemetry()                  ── OpenTelemetry (gRPC/HTTP/Console)
├── CreateModelFactory()              ── LLM Provider 工厂
├── CreateSandboxProbe()              ── OS 沙箱能力探测
├── CreateExtensionManager()          ── MCP 扩展管理器 (注册 12 个内置扩展)
├── CreateMemoryService()             ── tRPC Memory Service (SQLite KV)
├── CreateSessionService()            ── Session Service (SQLite/Redis/InMemory)
├── CreateRecallStore()               ── FTS5 对话搜索 Store
├── CreateCortexStore()               ── HNSW 向量 Store
├── CreateMemoryFlow()                ── MemoryFlow 转录回溯
├── CreateGraphFlow()                 ── GraphFlow 知识图谱
├── CreateImportFlow()                ── ImportFlow 数据导入
├── CreateTodoManager()               ── Todo 任务系统
├── CreateRecallManager()             ── 跨系统搜索管理器
├── CreateSecurityGuard()             ── 安全守卫
├── CreateKnowledgeManager()          ── RAG 知识库
├── CreateBrowserController()         ── Chromedp 浏览器
├── CreateCodeExecutor()              ── goja JS 沙箱
├── CreateSummonManager()             ── 子代理委托管理
├── CreateSkillManager()              ── Skill 系统 (FSRepository)
├── CreateEvolutionEngine()           ── 技能进化引擎
├── CreateA2AServer()                 ── A2A 服务器 (端口 9090)
├── CreateAGUIServer()                ── AG-UI SSE 服务器 (端口 8080)
├── CreateACPServer()                 ── ACP 服务器 (端口 9091)
├── CreateACPMCPBridge()              ── ACP MCP Bridge (端口 3400)
├── CreateCoreLoop()                  ── 创建中央编排器
└── CoreLoop.Run()                    ── 启动 Agent 交互
```

### 3.3 执行循环 (Run 方法)

对话处理的完整四个阶段：

```
CoreLoop.Run(userID, sessionID, message)
│
├── Phase 1: 对话前准备 (PrepareContext)
│   ├── recallStore.StoreMessage(user)       → [FTS5 + HNSW 索引]
│   ├── cortexStore.StoreMessage(user)       → [HNSW 向量同步]
│   ├── memoryFlow.IngestTurn(user)          → [CortexDB Episode 转录]
│   ├── memoryFlow.WakeUp()                  → [向量+FTS5 唤醒上下文注入]
│   ├── memoryService.ReadMemories()          → [持久记忆注入 + 去重]
│   └── graphFlow.BuildContext()             → [KG 增强上下文]
│
├── Phase 2: Agent 执行 (runner.Run)
│   └── tRPC Runner.Run()
│       ├── LLM 推理 (主模型, 26B)
│       ├── Tool Calls → Security Guard 管线 → 执行
│       │   └── Guard.Check(tool, params)
│       │       ├── Permission Mode 检查 (auto/smart/manual/chat_only)
│       │       ├── Allowlist/Denylist 匹配 → Threat 扫描 → 超时控制
│       │       └── 用户审批 (manual 模式)
│       ├── AutoExtract (异步, 9B 模型, Memory Service)
│       │   └── ExtractFacts → AddMemory (后台 goroutine)
│       └── SummaryJob (异步, 上下文摘要, Revision Engine)
│           └── Summarize → UpdateContext (后台 goroutine)
│
├── Phase 3: 对话后收尾
│   ├── recallStore.StoreMessage(assistant)   → [FTS5 + HNSW]
│   ├── cortexStore.StoreMessage(assistant)   → [HNSW]
│   ├── recallStore.StoreMessage(tool_calls)   → [FTS5 + HNSW]
│   ├── recallStore.StoreMessage(tool_responses)→ [FTS5 + HNSW]
│   ├── cortexStore.StoreMessage(tool_*)      → [HNSW]
│   ├── memoryFlow.IngestTurn(assistant)      → [CortexDB Episode]
│   ├── memoryFlow.PromoteFacts()             → [GetTranscript → Extract → AddMemory]
│   └── graphFlow auto-extract                → [BuildTranscript → Extract → BuildGraph]
│
└── Phase 4: 返回响应
    └── contextMgr.AfterRun() → token 统计更新
```

### 3.4 关闭序列

```
Close() ── 优雅关闭所有子系统
├── bgWg.Wait()                ← 等待所有后台 goroutine
├── runner.Close()             ← 停止 Agent Runner
├── evolution.Close()          ← 停止进化引擎 worker
├── memory.Close()             ← 停止 AutoExtract worker (5s timeout)
├── session.Close()            ← 关闭会话存储
├── graphFlow.Close()          ← 关闭知识图谱引擎
├── telemetry.Shutdown(10s)    ← 刷新 OTLP + Langfuse traces
└── dbPool.Close()             ← 关闭数据库 (PRAGMA wal_checkpoint(TRUNCATE))
```

---

## 4. 双引擎记忆系统

### 4.1 引擎一：tRPC Memory (SQLite KV)

**实现**: `internal/memory/store.go`

**核心功能**:
- `AutoExtract`: 每轮对话后异步触发 LLM (9B 模型) 提取事实
- `SmartCleanup`: 容量超过 80% 时触发评分淘汰，降至 60%。评分公式：70% recency + 30% content length
- `MemoryBridge`: 接收 MemoryFlow.PromoteFacts 写入的事实

**6 个工具**:
| 工具 | 功能 |
|------|------|
| `memory_add` | 添加记忆条目 |
| `memory_search` | 语义搜索记忆 |
| `memory_update` | 更新记忆 |
| `memory_delete` | 删除记忆 |
| `memory_load` | 加载全部记忆 |
| `memory_clear` | 清空记忆 |

### 4.2 引擎二：CortexDB Stack

CortexDB 是一个集成 HNSW 向量索引、FTS5 全文搜索和 RDF 知识图谱的智能记忆引擎。

#### CortexStore (`internal/cortex/store.go`)

基于 CortexDB 的 HNSW 向量搜索 + FTS5 全文搜索：
- 存储所有消息 (user/assistant/tool_call/tool_response)
- `SearchWithMemory()` → 跨系统搜索：CortexDB HNSW + tRPC Memory SearchMemories

#### MemoryFlow (`internal/cortex/memoryflow.go`)

对话转录与语义唤醒：
- `IngestTurn(content)` → 将对话轮次存储到 CortexDB Episode
- `WakeUp(query)` → 基于当前查询进行向量 + FTS5 语义召回
- `PromoteFacts(sessionID, userID)` → 事实提取与桥接

#### GraphFlow (`internal/cortex/graphflow.go`)

知识图谱构建：
- `auto_extract`: 每轮对话后自动执行
- 流程：`BuildTranscript → ExtractEntitiesFromTranscript → BuildGraph(RDF)`
- 支持 SPARQL 查询

#### ImportFlow (`internal/cortex/import_flow.go`)

结构化数据导入：
- DDL 解析 → KG 映射计划 → 实体/关系构建
- CSV 数据导入 → RAG + KG 双通道

#### Extractor (`internal/cortex/extractor.go`)

LLM + 启发式事实提取：
- LLM Extract: 结构化 Prompt → JSON 解析 → 事实列表
- Heuristic: 中英文关键词匹配 (偏好、"我喜欢"、决策、"决定"、笔记、"记住")
- 回退链：专用模型 → 默认模型 → 禁用 (回退到启发式)

#### Planner (`internal/cortex/planner.go`)

LLM 驱动的检索策略规划：
- 输入：用户查询 + 可用检索能力 (lexical/vector/hybrid)
- 输出：最优检索策略建议

#### RecallManager (`internal/cortex/recall_manager.go`)

跨系统搜索管理器：
- `recall_search`: 同时查询 CortexDB (对话历史) + tRPC Memory (持久记忆)
- `recall_sessions`: 列出相关会话

### 4.3 记忆闭环完整数据流

```
Before Run (上下文注入)                          After Run (记录与提取)
┌─────────────────────────────────┐         ┌───────────────────────────────────┐
│ 1. WakeUp()                     │         │ 1. StoreMessage(user/assistant/   │
│    → CortexDB 向量+FTS5 召回    │         │    tool_call/tool_response)       │
│    历史对话上下文                │         │    → FTS5 + HNSW 索引            │
│                                 │         │                                   │
│ 2. ReadMemories()               │         │ 2. IngestTurn(assistant)         │
│    → tRPC SQLite 持久记忆       │         │    → CortexDB Episode 存储       │
│    → 与 WakeUp 去重             │         │                                   │
│    [isMemoryDuplicated]         │         │ 3. PromoteFacts()                 │
│                                 │         │    → GetTranscript() 完整对话     │
│ 3. BuildContext()               │         │    → LLM/Heuristic 事实提取       │
│    → KG 增强上下文              │         │    → AddMemory() tRPC 持久化      │
│                                 │         │                                   │
│                                 │         │ 4. GraphFlow auto-extract        │
│                                 │         │    → ExtractFromTranscript()     │
│                                 │         │    → BuildGraph() RDF 图谱       │
└─────────────────────────────────┘         └───────────────────────────────────┘

Cross-System Search:
  recall_search → CortexStore.SearchWithMemory()
    ├── CortexDB HNSW vector search (conversation history + tool messages)
    └── tRPC SearchMemories() (persistent memories)
    → Merge results
```

### 4.4 PromoteFacts 关键路径

```
MemoryFlowService.PromoteFacts(sessionID, userID)
  │
  ├── flow.GetTranscript()
  │     └── 从 CortexDB Episode 加载完整对话轮次
  │         Transcript{Turns: [user, assistant, user, ...]}
  │
  ├── extractor.Extract(transcript)
  │     ├── LLM Extract: 结构化 Prompt → JSON parsing
  │     │     └── 回退链: 专用模型 → 默认模型 → 禁用/启发式
  │     └── Heuristic: 中英文多关键词匹配
  │           (偏好/我喜欢/决策/决定/笔记/记住/todo/计划)
  │
  └── memoryService.AddMemory()
        └── 写入 tRPC SQLite 持久化
              topics: [kind, collection] → SmartCleanup 管理
```

---

## 5. 多 Agent 编排

### 5.1 WorkflowBuilder (workflow.go)

`WorkflowBuilder` 根据 `config.WorkflowConfig.Mode` 构建不同拓扑：

| 模式 | 拓扑 | 实现方式 | 适用场景 |
|------|------|----------|----------|
| `single` | 单 Agent | LLMAgent 直接执行 | 简单对话（默认） |
| `chain` | 顺序管道 | ChainAgent: planner → executor → reviewer | 多步流水线 |
| `parallel` | 并发执行 | ParallelAgent: 多 Agent 并发，结果聚合 | 多角度分析 |
| `cycle` | 迭代循环 | CycleAgent: planner↔executor 或 generator↔reviewer | 自我改进 |
| `graph` | 条件路由 | GraphAgent: StateGraph + 条件边 + HITL | 复杂决策 |
| `dify` | Dify 平台 | DifyAgent: Dify Chat API 包装 | 低代码编排 |

### 5.2 TeamBuilder (team.go)

两种多 Agent 协作拓扑：

| 模式 | 协调方式 | 说明 |
|------|----------|------|
| `team_coordinator` | Leader 通过 AgentTool 委托 | Leader 将成员包装为工具，主动调度 |
| `team_swarm` | 蜂群自主 transfer_to_agent | 依赖 Agent 自身判断进行任务转移 |

### 5.3 Recipe System (recipe.go, recipe_tool.go, recipe_compose.go)

基于 YAML 的结构化子 Agent 定义。每个 recipe 加载后注册为可被主 Agent 调用的工具，工具名前缀 `recipe-<name>`。

**基础字段**（向后兼容，无新字段的 recipe 行为不变）：

```yaml
# .wukong/recipes/code_reviewer.yaml
name: code_reviewer
description: "Professional code reviewer"
instruction: "You are a senior code reviewer..."
model: "gpt-4o"                    # 可选，覆盖默认模型
tools: [file_read, code_search]    # 授权工具名列表
temperature: 0.2                   # 默认 0.3
max_tokens: 2048                   # 默认 1024
max_iterations: 5                  # 默认 3
skip_summarization: false          # 可选
```

**参数系统（P0）**：通过 `parameters` + `prompt` 让 recipe 可参数化复用。`prompt` 是 Go `text/template` 模板，渲染后作为子 Agent 的用户消息：

```yaml
name: code_reviewer
description: "Parameterized code reviewer"
instruction: "You are an expert code reviewer."
prompt: |
  Review the following {{.language}} code focusing on {{.focus}}:
  {{.code}}
parameters:
  - key: language
    description: "Programming language"
    type: select              # string|number|boolean|select
    required: true
    options: [go, python, rust]
  - key: focus
    description: "Review focus area"
    type: string
    required: false
    default: "all issues"
  - key: code
    description: "Code to review"
    type: string
    required: true
```

主 Agent 调用 `recipe-code_reviewer` 工具时传入 JSON 参数（如 `{"language":"go","focus":"security","code":"..."}`），`recipeTool` 校验参数、填充默认值、渲染 prompt 模板后传给子 Agent。

> **条件渲染注意**：Go template 对非空字符串视为真。boolean 参数值为 `"false"` 时在 `{{if .flag}}` 中仍为真。需用 `{{if eq .flag "true"}}` 显式比较，或用空字符串表示假。

**结构化输出（P0）**：通过 `response` 配置 JSON Schema，子 Agent 最终输出强制符合 schema：

```yaml
response:
  json_schema:
    type: object
    properties:
      issues:
        type: array
        items:
          type: object
          properties:
            severity: {type: string}
            message: {type: string}
            line: {type: integer}
      summary: {type: string}
    required: [issues, summary]
  strict: true                    # 严格模式（禁止额外字段）
  validate_output: true           # P1-B: 校验返回的 JSON
  description: "Structured review report"
```

底层利用 tRPC-Agent-Go 的 `llmagent.WithStructuredOutputJSONSchema`，通过模型原生的 response_format 机制约束输出（provider 支持时生效）。

**重试与校验（P1-B）**：通过 `retry` 配置指数退避重试。配合 `response.validate_output: true`，输出校验失败时也会触发重试：

```yaml
retry:
  max_attempts: 3                 # 总尝试次数（含首次）
  initial_wait: "1s"              # 首次重试前等待
  backoff_factor: 2.0             # 退避系数
  max_wait: "30s"                 # 最大等待上限
```

重试逻辑由 `retryTool` 包装器实现（`recipe_compose.go`），包装在任何 `tool.CallableTool` 外层。重试条件：执行错误或输出校验失败。context 取消时立即终止。最终尝试仍失败时返回聚合错误。

**子配方组合（P1-A）**：recipe 的 `tools` 列表可引用其他 recipe，实现层级编排。被引用的 recipe 工具会授权给引用方的子 Agent：

```yaml
# .wukong/recipes/orchestrator.yaml
name: orchestrator
description: "Delegates to sub-recipes"
instruction: "You orchestrate code reviews."
tools:
  - file_read
  - recipe-code_reviewer     # 引用其他 recipe（前缀形式）
  - summarizer               # 或裸名形式
```

加载时通过拓扑排序确定构建顺序（被依赖的 recipe 先构建），循环依赖在加载时检测并拒绝。`mergeToolSets` 将基础工具和已构建的 recipe 工具合并，授予给当前 recipe 的子 Agent。

**内联扩展（P2-A）**：recipe 可直接在 `config.yaml` 中定义，无需独立 YAML 文件：

```yaml
# config.yaml
agent:
  recipe_enabled: true
  inline_recipes:
    - name: quick-helper
      description: "Quick helper"
      instruction: "You are a quick helper."
      tools: [file_read]
      temperature: 0.3
```

内联 recipe 与文件 recipe 合并加载，同名时内联优先。通过 YAML marshal/unmarshal 往返转换 `map[string]any` → `RecipeConfig`，避免 config 与 agent 包间的循环依赖。

**配方继承（P2-B）**：recipe 可通过 `extends` 继承另一个 recipe 的字段，子 recipe 非零字段覆盖父 recipe：

```yaml
# .wukong/recipes/security_reviewer.yaml
name: security_reviewer
extends: base_reviewer          # 继承 base_reviewer 的所有字段
instruction: "You are a security-focused reviewer."  # 覆盖
tools: [file_read, code_search, recipe-vuln_scanner]  # 覆盖
temperature: 0.1                # 覆盖
```

继承链递归解析，循环 extends 在加载时检测并拒绝。`mergeRecipes` 执行深合并：标量字段子覆盖父，切片字段子非空时整体替换。

**加载流水线**（`NewRecipeToolSet`，五阶段）：

```
Phase 1: 加载所有 recipe 配置（文件 + 内联）→ map[name]*RecipeConfig
Phase 2: 解析 extends 继承链 → resolveAllExtends
Phase 3: 拓扑排序子配方依赖 → topoSortRecipes
Phase 4: 按序构建 recipe 工具（agenttool + recipeTool + retryTool 包装）
Phase 5: 注册到工具集，返回给主 Agent
```

### 5.4 HITL 人机协同 (hitl.go)

GraphAgent 模式支持中断-恢复：
```
graph.AddInterruptBefore("dangerous_op")
  → 执行到危险节点前暂停
  → 发送审批请求给用户
  → user.ResumeInterrupted(checkpointID)
  → 从检查点恢复执行
```

### 5.5 Dify 集成 (dify.go)

将 Dify AI 平台的 Chat API 包装为 tRPC-Agent-Go 兼容的 Agent：
- 支持 Dify Chat 端点
- 自定义 Dify 工作流变量
- 工作流执行结果解析

---

## 6. 扩展与工具系统

### 6.1 ExtensionManager (extension/manager.go)

`Manager` 负责 MCP 扩展的完整生命周期：

**注册方式**: Deeplink URL 或 YAML 配置文件

**传输协议**: stdio / SSE / streamable HTTP

**工具权限**: `ToolPermission` 结构体控制：
```go
type ToolPermission struct {
    Allowlist []string // 允许的工具名称（支持通配符）
    Denylist  []string // 禁止的工具名称（支持通配符）
}
```

**MCP Broker**: 将所有外部 MCP Server 的工具聚合为 4 个入口工具：
- `list_extensions`: 列出所有已注册扩展
- `enable_extension`: 启用扩展
- `disable_extension`: 禁用扩展
- `install_extension`: 安装新扩展

### 6.2 ACP MCP Bridge (extension/acp_mcp.go)

`ACPMCPBridge` 将 Wukong 内置扩展暴露为 MCP Server：
- 自动将 Extension 工具转换为 MCP Tool 定义
- 通过 Streamable HTTP 暴露在 `:3400/mcp`
- 供 ACP 兼容的编码代理发现和调用

### 6.3 MCP Client (extension/mcp_client.go)

原生 MCP 客户端，支持三种传输：
- **stdio**: 通过子进程标准输入输出通信
- **sse**: Server-Sent Events 流式通信
- **streamable**: HTTP Streamable 传输

### 6.4 Deeplink 解析 (extension/deeplink.go)

支持 `wukong://extension?name=xxx&transport=stdio&command=python&args=server.py` 格式的 Deeplink URL 解析，实现一键安装 MCP 扩展。

### 6.5 内置扩展注册 (extension/builtin/registry.go)

```go
func RegisterBuiltins(manager *Manager, cfg *config.WukongConfig, state *BootstrapState) {
    manager.Register("developer", NewDeveloperToolSet(cfg))
    manager.Register("memory", NewMemoryToolSet(cfg, state.MemoryService))
    manager.Register("cortex", NewCortexToolSet(cfg, state.RecallManager))
    manager.Register("code_mode", NewCodeModeToolSet(cfg, state.CodeExecutor))
    manager.Register("computer_controller", NewComputerControllerToolSet(cfg, state.Browser))
    manager.Register("auto_visualiser", NewVisualiserToolSet(cfg))
    manager.Register("tutorial", NewTutorialToolSet(cfg))
    manager.Register("top_of_mind", NewTopOfMindToolSet(cfg, state.TopOfMind))
    manager.Register("apps", NewAppsToolSet(cfg, state.AppsManager))
    manager.Register("web", NewWebToolSet(cfg))
    manager.Register("agent_tools", NewAgentToolSet(cfg))
}
```

---

## 7. 安全纵深防御

### 7.1 Layer 5: Guard 权限控制 (security/guard.go)

`Guard` 实现多模式权限控制和工具执行管线：

```
Guard.Check(toolName, params)
  ├── PermissionMode 检查:
  │     auto:      自动批准 (无需用户确认)
  │     smart:     根据 allowlist/denylist 智能决策
  │     manual:    所有工具需用户确认
  │     chat_only: 禁止所有工具调用
  ├── allowlist/denylist 匹配 (支持通配符)
  │     e.g. "file_*" → 匹配 file_read, file_write, file_replace
  ├── block_dangerous_commands: 模式匹配
  │     e.g. "rm -rf /", "dd if=/dev/zero", "mkfs.", "> /dev/sda"
  ├── malware_scan: 文件内容威胁扫描
  ├── timeout: default=30s, max=300s (context.WithTimeout)
  └── guardrail: Prompt 注入检测 (可选, 独立轻量 Runner)
```

### 7.2 Layer 4: goja JS 沙箱 (codemode/executor.go)

`Executor` 创建沙箱化的 JavaScript 执行环境：

**安全措施**:
| 措施 | 实现 |
|------|------|
| API 白名单 | console / JSON / Math / __output |
| 完全禁用 | eval / Function / setInterval / Date / RegExp |
| 内存限制 | `debug.SetMemoryLimit(128MB)` |
| 超时控制 | `context.WithTimeout(10s)` |
| 并发控制 | channel semaphore (max 5) |
| JSON 保护 | `JSON.parse` 1MB 输入限制 |
| ReDoS 防护 | `regexp` 完全禁用 |

**工具发现**: 通过 `__tools` 全局变量注入可用工具列表。

### 7.3 Layer 3: sandbox OS 级隔离 (pkg/sandbox/)

`pkg/sandbox/` 是独立可复用的 OS 级文件沙箱包：

```
sandbox.Command / sandbox.CommandContext
  ├── Linux (kernel 5.13+): Landlock LSM
  │     ├── self-exec 模式 (landlock_helper 子进程)
  │     ├── 全文件系统 read-only
  │     └── 仅 WritableDirs 可写
  ├── macOS: sandbox-exec + Seatbelt
  │     ├── 动态 profile 生成
  │     └── 仅 work_dir + .wukong 可写
  ├── Windows: Low Integrity Level + Mandatory Labels
  │     └── 仅 writableDirs 可写
  └── Other: 非沙箱运行 + WARN 日志
```

启动时 `sandbox.Probe()` 检测平台能力并输出日志。

### 7.4 Layer 2: .wukongignore (security/ignore.go)

`IgnoreFile` 实现 gitignore 兼容的文件访问黑名单：

- **语法**: gitignore 兼容（支持 negate 规则 `!`）
- **匹配工具**: `file_read` / `file_write` / `file_replace` / `file_delete`
- **搜索优先级**: CWD > HOME > CWD/.wukong/
- **检查方法**: `IsIgnored(path)` → 所有文件操作前验证

### 7.5 Layer 1: OS 进程权限

- 非 root 用户运行
- ulimit 资源限制

---

## 8. 上下文管理

### 8.1 ContextRevisionEngine (agent/context.go)

`ContextManager` 实现三层上下文压缩策略：

| 层级 | 策略 | 触发条件 | 实现 |
|------|------|----------|------|
| 1 | LLM 智能摘要 | token 超阈值 / 消息数 > 100 / 时间 > 5min | 独立 revision_model (默认 9B gemma-4-e4b-it) |
| 2 | 渐进式压缩 | cooldown 120s 后 | 合并已有摘要 + 新内容摘要 |
| 3 | 算法截断 | LLM 不可用时回退 | 首部保留 + 尾部保留 |

**tRPC 框架级双通道压缩**：
- **Pass 1**: 旧的大尺寸工具结果 → 替换为占位符 `[Truncated: tool result too large]`
- **Pass 2**: 剩余超大结果 → 首部 + 尾部截断

**配置参数**:
```yaml
revision:
  enabled: true
  enable_llm_summarize: true
  summary_cooldown: 120s
  summary_timeout: 30s
  max_command_output: 8000
  max_context_tokens: 64000
  trim_ratio: 0.3
```

### 8.2 TodoEnforcer (agent/todo_enforcer.go)

轻量级 todo 执行器插件：
- Agent 给出最终答案前，检查所有 pending todo 是否完成
- 如存在未完成 todo，强制 Agent 先完成任务
- 阻止 Agent 在 todo 未完成时给出不完整的回答

### 8.3 PromptTemplate (agent/prompt_template.go)

从配置目录加载 `.md` 提示模板文件：
- 存储位置：`~/.config/wukong/prompts/`
- 支持 Go `text/template` 变量替换
- 模板变量：`{{.Task}}`, `{{.Context}}`, `{{.Memory}}` 等

---

## 9. LLM Provider 体系

### 9.1 Provider Factory (provider/factory.go)

`Factory` 基于统一接口创建 LLM 模型实例：

```go
type Factory struct {
    config   *config.WukongConfig
    defaults map[string]*config.ProviderConfig
}
```

**支持的 Provider 类型**:

| Provider | type 值 | 基础 URL | 认证方式 | SDK 实现 |
|----------|---------|----------|----------|----------|
| OpenAI | `openai` | `https://api.openai.com/v1` | API Key (Bearer) | openai-go SDK |
| Anthropic | `anthropic` | `https://api.anthropic.com` | API Key (x-api-key) | anthropic-go SDK |
| Google | `google` | 自动 | API Key | google-genai SDK |
| DeepSeek | `deepseek` | `https://api.deepseek.com` | API Key (Bearer) | openai-go SDK |
| Ollama | `ollama` | `http://localhost:11434/v1` | 本地无认证 | openai-go SDK |
| LMStudio | `lmstudio` | `http://localhost:1234/v1` | 本地无认证 | openai-go SDK |
| ACP | `acp` | agent_url | 自定义 | HTTP client |

### 9.2 ACP Provider (provider/acp.go)

`ACPProvider` 实现 `model.Model` 接口，将 ACP 兼容编码代理包装为 LLM Provider：
- 通过 HTTP 向 ACP `message/send` 端点发送请求
- 支持流式响应 (SSE)
- 模型分工策略：主对话 26B / 记忆提取 9B / 上下文摘要 4B

### 9.3 模型分工策略

| 用途 | 典型模型 | 配置项 |
|------|----------|--------|
| 主对话 | 26B (Gemma-4-26b) | `default_provider` + CLI `--provider` / `--model` |
| 记忆提取 | 9B (gemma-4-e4b-it) | `lightweight_model` → `memory.extractor_model` |
| 上下文压缩 | 独立模型 | `revision.revision_model` → 回退到 `lightweight_model` |
| 知识图谱提取 | 独立模型 | `graphflow.extractor_model` → 回退到 `lightweight_model` |
| 检索规划 | 独立模型 | `memoryflow.planner_model` → 回退到 `lightweight_model` |

---

## 10. 配置系统

### 10.1 配置结构 (config/config.go, ~1534 行)

完整配置包含 38 个结构体，分为以下几个大类：

| 类别 | 结构体 |
|------|--------|
| 根配置 | `WukongConfig` |
| Provider | `ProviderConfig` |
| Extension | `ExtensionConfig`, `ToolPermission` |
| Agent | `AgentConfig` (19 个字段) |
| Security | `SecurityConfig` (11 个字段) |
| Storage | `SessionConfig`, `MemoryConfig`, `TodoConfig`, `RecallConfig` |
| CortexDB | `CortexConfig`, `MemoryFlowConfig`, `GraphFlowConfig`, `ImportFlowConfig` |
| Context | `RevisionConfig` (11 个字段) |
| Features | `BrowserConfig`, `VisualiserConfig`, `TutorialConfig`, `TopOfMindConfig`, `CodeModeConfig`, `AppsConfig` |
| Summon | `SummonConfig`, `A2ARemoteConfig`, `SkillConfig`, `EvolutionConfig` |
| Knowledge | `KnowledgeConfig` |
| Workflow | `DifyConfig`, `WorkflowConfig`, `WorkflowSubAgentConfig`, `TeamMemberConfig` |
| Services | `A2AServerConfig`, `AGUIConfig`, `ACPServerConfig`, `ACPMCPConfig` |
| Eval | `EvalConfig`, `EvalMetricConfig` |
| Artifact | `ArtifactConfig` |
| Observability | `ObservabilityConfig`, `TelemetryConfig` |

### 10.2 配置加载机制

```
优先级（从高到低）:
  1. CLI 参数 (--provider, --model, --temperature, --max-tokens, --no-stream)
  2. 环境变量 (WUKONG_ 前缀, 如 WUKONG_DEFAULT_PROVIDER)
  3. --config CLI 参数指定的文件
  4. 当前目录 ./config.yaml
  5. ~/.config/wukong/config.yaml
  6. /etc/wukong/config.yaml (仅非 Windows)
  7. 内置默认值
```

**环境变量展开**: `${ENV_VAR}` 语法自动展开，支持 `api_key: "${OPENAI_API_KEY}"`。

### 10.3 配置查询辅助函数

| 函数 | 功能 |
|------|------|
| `FindProvider(name)` | 根据名称查找 Provider 配置 |
| `DefaultProviderConfig()` | 获取默认 Provider 配置 |
| `EffectiveLightweightModel()` | 获取有效的轻量模型名 (考虑回退链) |
| `EffectiveLightweightProvider()` | 获取有效的轻量模型 Provider |
| `EnabledExtensions()` | 获取所有启用的扩展列表 |
| `FindExtension(name)` | 查找特定扩展配置 |

---

## 11. 服务与协议层

### 11.1 协议端点

| 协议 | 端口 | 路径 | 用途 | 实现文件 |
|------|------|------|------|----------|
| A2A (:9090) | 9090 | / | Agent-to-Agent 标准通信 | internal/summon/a2a.go |
| ACP (:9091) | 9091 | /acp | Agent Client Protocol | internal/server/acp.go |
| AG-UI SSE (:8080) | 8080 | /agui | Web UI 实时对话 (SSE 流) | internal/server/agui.go |
| ACP MCP (:3400) | 3400 | /mcp | 跨协议工具桥接 | internal/extension/acp_mcp.go |

### 11.2 优雅关闭

所有 HTTP 端点使用 `*http.Server` + `Shutdown(ctx)`:
1. 停止接收新连接
2. 等待活跃请求完成
3. 超时强制终止

**关闭序列**:
```
Signal Received
  → bgWg.Wait()              (等待所有后台 goroutine 完成)
  → runner.Close()            (停止 Agent Runner)
  → evolution.Close()         (停止进化引擎)
  → memory.Close(5s timeout)  (停止 AutoExtract)
  → session.Close()           (关闭会话存储)
  → graphFlow.Close()         (关闭知识图谱)
  → a2aServer.Stop(ctx)       (停止 A2A 服务)
  → aguiServer.Stop(ctx)      (停止 AG-UI 服务)
  → acpServer.Stop(ctx)       (停止 ACP 服务)
  → acpMCP.Stop()             (停止 MCP Bridge)
  → telemetry.Shutdown(10s)   (刷新 OTLP + Langfuse traces)
  → dbPool.Close()            (WAL checkpoint + close)
```

### 11.3 安全规范

- 所有 HTTP 请求 body 大小限制 10MB (`io.LimitReader(10MB)`)
- 所有 `context.Background()` 调用附加超时
- Goroutine 生命周期由 `CoreLoop.bgWg` 统一追踪

---

## 12. 存储架构

### 12.1 单文件数据库

Wukong 使用单文件 SQLite WAL 数据库 (`wukong.db`) 承载所有子系统存储：

```
wukong.db (SQLite WAL)
├── sessions            ← Session Service
├── memories            ← tRPC Memory Service
├── recall              ← FTS5 全文搜索 Store
├── todos               ← Todo Store
├── projects            ← Project Manager
├── cortex_*            ← CortexDB (episodes/entities/relations/HNSW vectors)
├── evolution_*         ← Evolution Engine (skill_versions/evolution_records)
└── extension_*         ← Extension Manager registry
```

### 12.2 DatabasePool (util/database.go)

共享数据库连接池：
```go
type DatabasePool struct {
    db *sql.DB
    mu sync.RWMutex
    path string
    lazy bool
}
```

**配置**:
- `MaxOpenConns = 4`
- `_busy_timeout = 5000ms` (处理并发竞争)
- `_journal_mode = WAL`
- `_foreign_keys = ON`

**关闭**: `PRAGMA wal_checkpoint(TRUNCATE)` 确保 WAL 刷新到主文件。

### 12.3 CortexDB 实例共享

关键设计决策：CortexStore 和 MemoryFlow 共享同一个 `*cortexdb.DB` 实例，避免双连接 WAL 冲突。

---

## 13. 技能自进化系统

### 13.1 EvolutionEngine (evolution/engine.go)

闭环流程：
```
Agent 执行 → ExecutionTrace 收集
  → 异步分析队列 (后台 worker goroutine)
    → LLM Analysis (evolution/analyzer.go)
      → 识别问题：工具选择错误 / 参数错误 / 效率低下
      → 生成 PatchSuggestion (evolution/types.go)
    → Patch Application (evolution/patcher.go)
      → 备份当前 SKILL.md (版本管理)
      → 应用 LLM 生成补丁
      → 保存版本记录 (evolution/store.go)
    → 热重载 (skill.Manager.Refresh)
```

### 13.2 约束机制

| 约束 | 值 | 说明 |
|------|-----|------|
| 最小置信度 | 0.7 | PatchSuggestion.Confidence ≥ 0.7 才应用 |
| 冷却时间 | 30min | 同一技能两次进化间隔 |
| 每日上限 | 10 | 全局每日最多 10 个补丁 |
| 最大补丁 | 8KB | 单个补丁内容大小限制 |
| 版本保留 | 10 | 保留最近 10 个历史版本 |

### 13.3 核心类型 (evolution/types.go)

```go
type ExecutionTrace struct {
    SkillName   string
    SessionID   string
    ToolCalls   []ToolCallRecord
    Success     bool
    Error       string
}

type ToolCallRecord struct {
    ToolName   string
    Parameters map[string]interface{}
    Result     string
    Duration   time.Duration
    Error      string
}

type PatchSuggestion struct {
    SkillName   string
    Description string
    Patch       string   // 补丁内容 (SKILL.md diff)
    Confidence  float64  // 0-1 置信度
    Reason      string   // 生成此补丁的原因
}

type EvolutionRecord struct {
    ID          string
    SkillName   string
    Patch       string
    BeforeHash  string
    AfterHash   string
    Confidence  float64
    AppliedAt   time.Time
    Reverted    bool
}

type SkillVersion struct {
    Version int
    Content string
    Hash    string
    Created time.Time
}
```

### 13.4 生命周期管理

- **Evolution Hook**: `SkillManager` 执行后回调 `EvolutionEngine.Analyze()`
- **非阻塞**: 所有分析和补丁生成在后台 goroutine 中进行
- **版本回滚**: `EvolutionPatcher.Revert()` 支持回滚到上一版本

---

## 14. 完整数据流

### 14.1 对话处理流

```
User Input → CoreLoop.Run(userID, sessionID, message)
  │
  ├── Phase 1: 对话前准备
  │   ├── PrepareContext()                    → 上下文压缩
  │   ├── recallStore.StoreMessage(user)      → [FTS5 + HNSW 索引]
  │   ├── cortexStore.StoreMessage(user)      → [HNSW 向量同步]
  │   ├── memoryFlow.IngestTurn(user)         → [CortexDB Episode]
  │   ├── memoryFlow.WakeUp()                 → [向量+FTS5 唤醒上下文]
  │   └── memoryService.ReadMemories()         → [持久记忆 + 去重]
  │
  ├── Phase 2: Agent 执行
  │   └── runner.Run()
  │       ├── LLM Inference                   → [主模型 26B]
  │       ├── Tool Calls
  │       │   ├── Guard.Check(tool, params)   → [5 层安全检查]
  │       │   └── Execute Tool                → [沙箱/浏览器/JS/文件]
  │       ├── AutoExtract (async)             → [记忆提取 9B]
  │       └── SummaryJob (async)              → [上下文压缩]
  │
  ├── Phase 3: 对话后
  │   ├── recallStore.StoreMessage(assistant)  → [FTS5+HNSW]
  │   ├── recallStore.StoreMessage(tool_*)     → [FTS5+HNSW]
  │   ├── cortexStore.StoreMessage(*)          → [HNSW]
  │   ├── memoryFlow.IngestTurn(assistant)     → [CortexDB Episode]
  │   ├── memoryFlow.PromoteFacts()            → [Transcript→Extract→AddMemory]
  │   │     └── 写入 tRPC SQLite 持久化
  │   ├── graphFlow auto-extract              → [BuildTranscript→Extract→BuildGraph]
  │   │     └── RDF 知识图谱更新
  │   └── evolution.AddTrace() (if skill)     → [技能执行追踪]
  │
  └── Phase 4: 返回
      └── contextMgr.AfterRun()               → [token 统计更新]
```

### 14.2 关闭流

```
Signal/Exit → Close()
  ├── bgWg.Wait()                             ← 后台 goroutine (PromoteFacts, GraphFlow, AutoExtract)
  ├── runner.Close()                           ← Agent Runner
  ├── evolution.Close()                        ← 进化引擎 worker
  ├── memory.Close(5s timeout)                 ← AutoExtract (5s 超时)
  ├── session.Close()                          ← 会话存储
  ├── graphFlow.Close()                        ← 知识图谱
  ├── a2aServer.Stop(ctx)                      ← A2A 服务 (:9090)
  ├── aguiServer.Stop(ctx)                     ← AG-UI 服务 (:8080)
  ├── acpServer.Stop(ctx)                      ← ACP 服务 (:9091)
  ├── acpMCP.Stop()                            ← MCP Bridge (:3400)
  ├── telemetry.Shutdown(10s timeout)          ← OTLP + Langfuse
  └── dbPool.Close()                           ← PRAGMA wal_checkpoint(TRUNCATE) → sql.DB.Close()
```

### 14.3 记忆闭环流

```
┌──────────────────────────────────────────────────────────────┐
│                        Per-Turn Memory Cycle                  │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  Context Injection (Before)          Record & Extract (After)│
│  ┌────────────────────────┐         ┌──────────────────────┐ │
│  │ WakeUp (CortexDB)      │         │ StoreMessage (Recall) │ │
│  │  → vector + FTS5 recap │         │  → FTS5 + HNSW index  │ │
│  │                        │         │ StoreMessage (Cortex) │ │
│  │ ReadMemories (tRPC)    │         │  → HNSW vector sync   │ │
│  │  → persistent memory   │         │                       │ │
│  │  → dedup w/ WakeUp     │         │ IngestTurn (CortexDB) │ │
│  │                        │         │  → episode transcript  │ │
│  │ BuildContext (GraphFlow)│        │                       │ │
│  │  → KG-enhanced context │         │ PromoteFacts→tRPC     │ │
│  └────────────────────────┘         │  → extract → persist  │ │
│                                     │                       │ │
│                                     │ GraphFlow auto-extract│ │
│                                     │  → entity/relation RDF│ │
│                                     └──────────────────────┘ │
│                                                              │
│  Cross-system Search:                                        │
│    recall_search → SearchWithMemory()                        │
│      ├── CortexDB HNSW (conversations)                       │
│      └── tRPC SearchMemories (persistent)                    │
└──────────────────────────────────────────────────────────────┘
```

---

## 15. 技术选型

| 类别 | 选择 | 版本 | 理由 |
|------|------|------|------|
| **Agent 框架** | tRPC-Agent-Go | v1.10.0 | 多 Agent 编排、完整 Session/Memory/Planner/Skill 抽象 |
| **MCP 协议** | tRPC-MCP-Go | v0.0.16 | stdio/sse/streamable 三传输支持 |
| **A2A 协议** | tRPC-A2A-Go | v0.2.5 | Agent 间标准通信协议 |
| **智能记忆** | CortexDB | v2.25.0 | HNSW + FTS5 + RDF/SPARQL 集成 |
| **JS 引擎** | goja | latest | 纯 Go 零 CGO、跨平台、沙箱友好 |
| **OS 沙箱** | pkg/sandbox | 自维护 | Landlock/Seatbelt/Low IL，无外部依赖 |
| **数据库** | SQLite WAL | via mattn | 单文件部署、FTS5 全文搜索、零配置 |
| **前端** | BubbleTea + LipGloss | latest | 终端 TUI，纯 Go |
| **浏览器** | Chromedp | latest | Chrome DevTools Protocol |
| **配置** | Viper + Cobra | latest | CLI > ENV > YAML 多级覆盖 |
| **可观测** | OpenTelemetry + Langfuse | latest | 全链路追踪 + LLM 调用分析 |
| **语言** | Go | 1.26 | 高性能、跨平台交叉编译、goroutine 并发 |

---

## 16. 关键设计决策 (ADR)

本节记录了 23 个架构关键决策 (Architecture Decision Records)，每个决策包含选择内容和背后的理由。

| ADR | 决策 | 理由 | 影响范围 |
|-----|------|------|----------|
| **ADR-1** | SQLite WAL 共享池，MaxOpenConns=4 | 单文件零配置部署 + 并发支持 + FTS5 索引，`_busy_timeout=5000ms` 处理竞争 | util/database.go |
| **ADR-2** | 双引擎记忆 (tRPC + CortexDB) | tRPC Memory 处理 KV 持久化 + 自动提取，CortexDB 处理转录/向量/图谱结构化存储 | memory/, cortex/ |
| **ADR-3** | 辅助轻量模型分工 | 独立轻量模型 (9B) 处理记忆提取/上下文压缩/图谱构建，节省主模型 token 和延迟 | config, agent/context.go |
| **ADR-4** | CortexDB 实例共享 | CortexStore 与 MemoryFlow/GraphFlow 共用同一 `*cortexdb.DB`，避免双连接 WAL 冲突 | cortex/store.go |
| **ADR-5** | MemoryFlow → tRPC Bridge | PromoteFacts 从转录提取事实后通过 MemoryBridge 写入 tRPC 持久存储 | cortex/memoryflow.go |
| **ADR-6** | MCP Broker 4 入口模式 | 将多个外部 MCP Server 的工具聚合为 4 个管理入口，防止 50+ 工具直接暴露导致 token 消耗过大 | extension/manager.go |
| **ADR-7** | 冷启动友好降级 | 无 Embedding 服务时回退 FTS5 全文搜索，无 LLM 时回退启发式规则提取 | cortex/planner.go, cortex/extractor.go |
| **ADR-8** | 单文件数据库 | Session + Memory + Recall + Todo + Cortex 全库合一，跨系统查询零成本，部署简单 | util/database.go |
| **ADR-9** | SmartCleanup 容量淘汰 | 80% 阈值触发评分（70% recency + 30% length），淘汰至 60%，平衡容量与质量 | memory/store.go |
| **ADR-10** | 记忆去重 (WakeUp ↔ ReadMemories) | WakeUp 返回的记忆与 ReadMemories 进行滑动窗口去重，防止重复注入 | agent/loop.go |
| **ADR-11** | Tool 消息完整索引 | user/assistant/tool_call/tool_response 全部存入 FTS5 + HNSW，支持工具调用历史的语义搜索 | recall/store.go |
| **ADR-12** | GraphFlow auto_extract | 每轮对话后自动触发的实体/关系提取，构建 RDF 知识图谱，支持 SPARQL 查询 | cortex/graphflow.go |
| **ADR-13** | Extractor 三层回退链 | 专用模型 → 默认模型 → 禁用/启发式规则，确保任何条件下都能提取事实 | cortex/extractor.go |
| **ADR-14** | 非阻塞 Evolution | 所有技能分析和补丁生成在后台 goroutine 进行，不影响主 Agent 循环 | evolution/engine.go |
| **ADR-15** | CoreLoop 依赖注入 | 27 个子系统通过 BootstrapState 结构体注入，接口隔离、可测试、可替换 | agent/loop.go, cli/session.go |
| **ADR-16** | 三协议服务器 (A2A + ACP + AG-UI) | 三个端口分别提供 Agent 间通信、客户端协议和 Web UI 支持 | server/, summon/a2a.go |
| **ADR-17** | 服务器优雅关闭 | 全部使用 `*http.Server` + `Shutdown(ctx)`，确保活跃请求处理完毕 | server/ |
| **ADR-18** | HTTP body 大小限制 | `io.LimitReader(10MB)` 防止 DoS 攻击 | server/ |
| **ADR-19** | goja JS 沙箱多层防护 | API 白名单 + 内存限制 128MB + 并发控制 max 5 + ReDoS 防护（禁用 RegExp）| codemode/executor.go |
| **ADR-20** | Context 超时覆盖 | 所有 `context.Background()` 调用加显式超时，防止 goroutine 泄漏 | 全局 |
| **ADR-21** | Goroutine 生命周期统一管理 | `CoreLoop.bgWg` 追踪所有后台 goroutine，关闭前 `Wait()` | agent/loop.go |
| **ADR-22** | OS 级文件写入沙箱 | Landlock(linux)/Seatbelt(macOS)/Low IL(Windows) 内核级强制写保护 | pkg/sandbox/ |
| **ADR-23** | sandbox 置于 pkg/ 包 | 可被外部项目独立导入复用，独立测试验证，与业务逻辑解耦 | pkg/sandbox/ |

---

## 附录

### A. 关键文件清单

| 文件 | 行数 | 模块 | 说明 |
|------|------|------|------|
| `internal/agent/loop.go` | ~1560 | Core Engine | 中央编排器，协调所有子系统 |
| `internal/config/config.go` | ~1534 | Config | 完整配置结构定义 (38 个结构体) |
| `internal/cli/session.go` | ~1300 | CLI | 会话引导，27 个子系统初始化 |
| `internal/agent/workflow.go` | ~637 | Workflow | 10 种编排模式构建器 |
| `internal/agent/context.go` | ~559 | Context | 上下文压缩引擎 |
| `internal/security/guard.go` | ~484 | Security | 5 层安全守卫实现 |
| `internal/memory/store.go` | ~548 | Memory | tRPC Memory 管理器 |
| `internal/recall/store.go` | ~615 | Recall | FTS5 对话搜索 Store |
| `internal/cortex/lexical.go` | ~587 | Cortex | 词法回退存储 (SQLite + FTS5) |
| `internal/cortex/extractor.go` | ~439 | Cortex | LLM 事实提取器 |
| `internal/extension/manager.go` | ~428 | Extension | MCP 扩展生命周期管理 |
| `internal/evolution/engine.go` | ~400 | Evolution | 技能自进化引擎 |
| `pkg/sandbox/sandbox.go` | ~200 | Sandbox | OS 级文件沙箱 API |

### B. 术语表

| 术语 | 全称 | 说明 |
|------|------|------|
| CoreLoop | Core Orchestration Loop | 中央编排循环 |
| MemoryFlow | Memory Flow | CortexDB 的记忆流管道 |
| GraphFlow | Graph Flow | CortexDB 的知识图谱管道 |
| ImportFlow | Import Flow | CortexDB 的数据导入管道 |
| PromoteFacts | Promote Facts | 从转录中提取并持久化事实 |
| WakeUp | Memory Wake-Up | 基于当前查询的上下文唤醒 |
| AutoExtract | Automatic Memory Extraction | 异步 LLM 记忆自动提取 |
| SmartCleanup | Smart Memory Cleanup | 容量感知记忆淘汰 |
| MCP | Model Context Protocol | 模型上下文协议 |
| A2A | Agent-to-Agent | Agent 间通信协议 |
| ACP | Agent Client Protocol | 客户端-Agent 通信协议 |
| HITL | Human-in-the-Loop | 人机协同 |
| HNSW | Hierarchical Navigable Small World | 层级导航小世界向量索引 |
| RDF | Resource Description Framework | 资源描述框架 (知识图谱) |
| FTS5 | Full-Text Search 5 | SQLite 全文搜索引擎 |
| WAL | Write-Ahead Log | 预写日志 |
| ADR | Architecture Decision Record | 架构决策记录 |
