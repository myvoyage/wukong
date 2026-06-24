# Wukong 系统架构深度分析

> **版本**: v0.1.13 | **Go**: 1.26 | **文件**: 175 `.go` (42 `_test.go`) | **包**: 28 | **依赖**: 27 直接 + 106 间接
>
> 基于 [tRPC-Agent-Go v1.10.0](https://github.com/trpc-group/trpc-agent-go) · [tRPC-MCP-Go v0.0.16](https://github.com/trpc-group/trpc-mcp-go) · [tRPC-A2A-Go v0.2.5](https://github.com/trpc-group/trpc-a2a-go) · [CortexDB v2.25.0](https://github.com/liliang-cn/cortexdb)

---

## 1. 架构哲学

Wukong 遵循五大核心哲学，决定所有工程决策：

| 哲学 | 核心信念 | 关键工程决策 |
|------|----------|-------------|
| **记忆优先** | Agent 智能源于跨会话知识积累 | 双引擎三层记忆：tRPC Memory + CortexDB Stack (HNSW+FTS5+RDF) |
| **框架组装** | 任何组件都应可替换 | CoreLoop 依赖注入，12 子系统接口隔离 |
| **多 Agent 原生** | 编排是第一公民 | 10 种显式编排模式 + HITL 人机协同 |
| **进化智能** | 技能应从失败中学习 | LLM 分析 → 自动补丁 → 版本管理 → 热重载 |
| **双向发现** | 发现别人，也被人发现 | ARD: 联邦搜索 + RegistryServer 发布 |

---

## 2. 系统全景图

```
┌──────────────────────────────────────────────────────────────────────┐
│                       Wukong AI Agent Platform                         │
├──────────────────────────────────────────────────────────────────────┤
│ Entry Points: CLI (cobra+TUI) │ A2A :9090 │ ACP :9091 │ AG-UI :8080  │
├──────────────────────────────────────────────────────────────────────┤
│ Core Engine: CoreLoop (1560行) — 中央编排器, 12 子系统                  │
│   WorkflowBuilder(10模式) · TeamBuilder · ContextManager(3层压缩)     │
│   Security Guard(5层防御) · HITL · TodoEnforcer                         │
├──────────────────────────────────────────────────────────────────────┤
│ Agent Framework: tRPC-Agent-Go v1.10.0                                │
│   LLMAgent / ChainAgent / ParallelAgent / CycleAgent / GraphAgent      │
│   Planner / ToolSearch / ContextCompaction / Skill / Recipe            │
│   6 Callbacks + Session/Memory/Artifact Service                        │
├──────────────────────────────────────────────────────────────────────┤
│ Memory Stack (双引擎三层):                                              │
│   短期: MemoryFlow — IngestTurn → WakeUp(3层) → PromoteFacts           │
│   中期: CortexStore — HNSW向量 + FTS5全文 + 余弦相似度                   │
│   长期: tRPC Memory — AutoExtract + SmartCleanup(70%+30%)              │
│   结构化: GraphFlow — auto_extract → RDF → SPARQL                      │
├──────────────────────────────────────────────────────────────────────┤
│ Capability Layer:                                                       │
│   Recipe(14功能) · 12内置扩展 · ARD(双向发现+7工具)                      │
│   Evolution · Summon(A2A委派) · CodeMode(goja JS)                     │
│   Browser(Chromedp) · Knowledge(RAG) · Apps(5子目录)                   │
│   pkg/sandbox(跨平台OS隔离) · pkg/zim(ZIM格式)                           │
├──────────────────────────────────────────────────────────────────────┤
│ Infrastructure:                                                         │
│   7 LLM backends · OpenTelemetry · Langfuse                            │
│   MultiPool(SQLite WAL) · fsnotify · text/template                     │
├──────────────────────────────────────────────────────────────────────┤
│ Storage: wukong.db (单文件, shared MultiPool)                           │
│   sessions / memories / recall(FTS5) / todos / projects                │
│   cortex_* / apps_history / evolution_* / FTS5 / HNSW / vectors        │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 3. CoreLoop 中央编排引擎

`internal/agent/loop.go` (1560 行, 13 源文件 + 8 测试文件)

CoreLoop 是 Wukong 的中央编排引擎，类似 Goose 的交互式工具调用循环，建立在 tRPC-Agent-Go 的 Runner 和 LLMAgent 之上。

### 3.1 结构体定义

```go
type CoreLoop struct {
    agent          agent.Agent          // tRPC Agent 实例
    runner         runner.Runner        // tRPC Runner 执行器
    sessionService session.Service      // 会话持久化
    memoryService  memory.Service       // 长期记忆服务
    factory        *provider.Factory    // LLM Provider 工厂
    cfg            *config.WukongConfig // 全局配置
    contextMgr     *ContextManager      // 上下文管理（3层压缩）
    security       *security.Guard      // 安全守卫（5层防御）
    recallStore    *recall.Store        // FTS5 全文搜索召回
    cortexStore    *cortex.CortexStore  // HNSW 向量检索（可选）
    memoryFlow     *cortex.MemoryFlowService  // 转录+唤醒
    graphFlow      *cortex.GraphFlowService  // KG 自动提取
    closeFn        func() error         // 关闭回调链
    mu     sync.RWMutex
    closed bool
    bgWg   sync.WaitGroup               // 后台 goroutine 追踪
}
```

### 3.2 依赖注入配置

```go
type CoreLoopConfig struct {
    Config               *config.WukongConfig
    Factory              *provider.Factory
    SessionService       session.Service
    MemoryService        memory.Service
    ArtifactService      artifact.Service
    ToolSets             []tool.ToolSet
    FunctionTools        []tool.Tool
    SecurityGuard        *security.Guard
    RecallStore          *recall.Store
    CortexStore          *cortex.CortexStore
    RevisionModel        RevisionModel
    MemoryFlowService    *cortex.MemoryFlowService
    GraphFlowService     *cortex.GraphFlowService
    TopOfMindInstructions string
    TelemetryShutdown    func(context.Context) error
    MemoryClose          func() error
}
```

### 3.3 初始化序列（8 步）

```
1. 收集 FunctionTools
   → 2. 加载 Recipe YAML 文件
   → 3. 添加 Todo 工具（如启用）
   → 4. 选择编排模式（single→LLMAgent / 其他→WorkflowBuilder）
   → 5. 创建 Runner（含 session/memory/artifact service）
   → 6. 配置插件（ToolSearch/TodoEnforcer/ContextCompaction）
   → 7. 初始化 ContextManager + RevisionEngine
   → 8. 组装 closeFn 关闭回调链
```

### 3.4 执行循环（4 阶段）

```
Phase 1: Prepare (准备阶段)
  ├── ContextManager.Prepare()        — 上下文压缩/裁剪
  ├── Recall/Cortex 存储检索          — 相关历史消息
  ├── MemoryFlow.WakeUp()             — 3层唤醒（身份/回忆/当前会话）
  ├── tRPC Memory.ReadMemories()     — 长期记忆去重
  └── GraphFlow KG 增强               — 知识图谱补充

Phase 2: Execute (执行阶段)
  ├── runner.Run()                    — LLM 推理
  ├── Tool Calls 执行                 — 工具调用
  └── Guard.Check()                   — 安全检查（权限/命令/注入）

Phase 3: Finalize (收尾阶段)
  ├── StoreMessage()                  — 保存本轮消息
  ├── MemoryFlow.IngestTurn()        — 转录本轮对话
  ├── tRPC Memory.PromoteFacts()     — 提取长期记忆
  └── GraphFlow.auto_extract()       — 自动 KG 提取

Phase 4: Return (返回阶段)
  └── contextMgr.AfterRun()           — Token 统计与清理
```

### 3.5 优雅关闭序列（6 步）

```
bgWg.Wait() → runner.Close()
  → evolution.Close()
  → memory.Close(5s 超时)
  → session.Close() + graphFlow.Close()
  → telemetry.Shutdown(10s 超时)
  → dbPool.Close()
```

---

## 4. 多 Agent 编排系统

`internal/agent/workflow.go` (18.37 KB) + `internal/agent/team.go` (8.90 KB)

### 4.1 WorkflowMode 定义

```go
const (
    WorkflowSingle          WorkflowMode = "single"
    WorkflowChain           WorkflowMode = "chain"
    WorkflowParallel        WorkflowMode = "parallel"
    WorkflowCycle           WorkflowMode = "cycle"
    WorkflowGraph           WorkflowMode = "graph"
    WorkflowTeamCoordinator WorkflowMode = "team_coordinator"
    WorkflowTeamSwarm       WorkflowMode = "team_swarm"
    WorkflowClaudeCode      WorkflowMode = "claude_code"
    WorkflowCodex           WorkflowMode = "codex"
    WorkflowDify            WorkflowMode = "dify"
)
```

### 4.2 模式详解

| 模式 | 拓扑 | 底层实现 | 适用场景 |
|------|------|----------|----------|
| `single` | 单体 Agent | LLMAgent | 日常对话（默认） |
| `chain` | planner→executor→reviewer | ChainAgent | 流水线处理 |
| `parallel` | 3 视角并发 | ParallelAgent | 多角度分析 |
| `cycle` | planner↔executor | CycleAgent | 自我改进迭代 |
| `graph` | 条件路由 DAG | GraphAgent | 复杂决策流程 |
| `team_coordinator` | Leader 委派 | TeamAgent | 团队协作 |
| `team_swarm` | 自动 transfer | TeamAgent(swarm) | 自主委派 |
| `claude_code` | 外部 CLI 进程 | exec.Cmd | 本地 Claude CLI |
| `codex` | 外部 CLI 进程 | exec.Cmd | 本地 Codex CLI |
| `dify` | HTTP API | HTTP Client | Dify 低代码平台 |

### 4.3 WorkflowBuilder

`WorkflowBuilder` 负责根据 `OrchestrationConfig` 构建对应的 tRPC Agent：

```go
type WorkflowBuilder struct {
    factory   *provider.Factory
    cfg       *config.WukongConfig
    model     model.Model
    genConfig model.GenerationConfig
    tools     []tool.Tool
    toolSets  []tool.ToolSet
}
```

### 4.4 TeamBuilder

`internal/agent/team.go` 实现团队模式：
- `team_coordinator`: Leader Agent 负责任务分解和委派
- `team_swarm`: 子 Agent 之间通过 `transfer` 自动切换

---

## 5. Recipe 子 Agent 系统

`internal/agent/recipe*.go` (5 文件, 1878 行)

### 5.1 架构概览

Recipe 是一个轻量级可复用的子 Agent 定义系统，通过 YAML 文件描述 Agent 的行为、工具和提示。

**5 文件协同**:
| 文件 | 行数 | 功能 |
|------|------|------|
| `recipe.go` | ~650 | 核心定义、加载、模板 |
| `recipe_compose.go` | ~450 | 组合、继承、子配方 |
| `recipe_tool.go` | ~400 | 工具包装器 |
| `recipe_advance.go` | ~280 | 高级特性（超时/重试/模型覆盖） |
| `recipe_metrics.go` | ~140 | 执行统计与监控 |

### 5.2 14 项功能

| # | 功能 | 说明 |
|---|------|------|
| 1 | 参数化 | `${param}` 模板变量替换 |
| 2 | 结构化输出 | JSON Schema 定义输出格式 |
| 3 | 子配方 | 配方间组合与嵌套 |
| 4 | 重试 | 指数退避自动重试（可配置次数/初始等待/退避因子） |
| 5 | 继承 | 父配方属性继承 |
| 6 | 内联 | 配方内容直接嵌入 |
| 7 | 模型覆盖 | 指定特定 LLM provider/model |
| 8 | 超时 | 执行时间限制 |
| 9 | 热重载 | `fsnotify` 文件变更自动重载 |
| 10 | 指标 | 调用次数/成功率/平均延迟 |
| 11 | Agent Tool 包装 | 包装为 tRPC Agent Tool |
| 12 | Function Tool | 直接注册为 FunctionTool |
| 13 | Retry Tool | 带退避重试的 Tool 包装 |
| 14 | Timeout Tool | 带超时控制的 Tool 包装 |

### 5.3 工具包装器链

```
agenttool.NewTool(recipeTool)
  → recipeTool(参数注入+模板渲染)
  → retryTool(指数退避重试)
  → timeoutTool(超时控制)
```

### 5.4 加载阶段（5 阶段）

```
1. 文件发现    — 扫描 recipe_dir 下的 YAML 文件
2. 解析验证    — YAML → Recipe 结构体 + Schema 验证
3. 模板编译    — text/template 编译提示模板
4. 依赖解析    — 继承链 + 子配方引用展开
5. 工具注册    — 生成工具并注册到 Agent
```

---

## 6. 双引擎记忆系统

### 6.1 架构总览

```
┌─────────────────────────────────────────────────────────────┐
│                     Wukong Memory Stack                      │
├─────────────┬────────────────┬───────────┬──────────────────┤
│   短期记忆   │   中期记忆       │  长期记忆  │  结构化记忆       │
│ MemoryFlow   │ CortexStore     │ tRPC Memory│ GraphFlow        │
├─────────────┼────────────────┼───────────┼──────────────────┤
│ IngestTurn   │ HNSW Vector     │ AutoExtract│ auto_extract     │
│ (转录记录)    │ (向量索引)       │ (异步LLM)  │ (每轮对话)        │
│ WakeUp       │ FTS5 Full-Text  │ SmartCleanup│ RDF Triple       │
│ (3层唤醒)     │ (全文搜索)       │ (容量淘汰)  │ (实体关系)        │
│ PromoteFacts  │ Cosine Similarity│ ReadMemories│ SPARQL           │
│ (事实提升)     │ (余弦相似度)      │ (去重查询)  │ (图谱查询)        │
└─────────────┴────────────────┴───────────┴──────────────────┘
```

### 6.2 短期记忆：MemoryFlow

**功能**: 转录本轮对话 → 生成唤醒上下文 → 将重要事实提升为长期记忆

**3 层唤醒上下文**:
1. **身份层** — Agent 的持久身份和角色定义
2. **回忆层** — 从历史记忆中检索的相关信息
3. **当前会话层** — 最近的对话上下文

### 6.3 中期记忆：CortexStore

`internal/cortex/store.go`

**双索引架构**:
- **HNSW 向量索引**: 当 embedding 配置时，使用 CortexDB 的 HNSW 进行语义搜索
- **FTS5 全文索引**: 始终可用，通过共享 SQLite 连接避免事务冲突

```go
type CortexStore struct {
    cfg      *config.CortexConfig
    embedder *Embedder
    db       *cortexdb.DB      // 真实 CortexDB (HNSW + FTS5)
    lexical  *lexicalStore     // FTS5 词法存储
}
```

**降级策略**: 无 embedder → 回退到 FTS5 全文搜索

### 6.4 长期记忆：tRPC Memory

`internal/memory/store.go`

**核心机制**:
- **AutoExtract**: 异步 LLM 提取关键事实（使用 `lightweight_model`）
- **SmartCleanup**: 容量达到 80% 触发 → 清理至 60% → 按 70% 新鲜度 + 30% 长度排序淘汰
- **ReadMemories**: 每轮对话自动检索去重后的相关记忆

### 6.5 结构化记忆：GraphFlow

`internal/cortex/graphflow.go`

**核心机制**:
- `auto_extract`: 每轮对话结束自动提取实体和关系
- RDF 三元组存储
- SPARQL 查询支持
- `max_chars_per_doc: 8000` 限制文档长度

---

## 7. 扩展与工具系统

### 7.1 Extension Manager

`internal/extension/manager.go` (13.28 KB)

```go
type Manager struct {
    mu       sync.RWMutex
    toolSets map[string]tool.ToolSet
    status   map[string]ExtensionInfo
    cfg      *config.WukongConfig
    ardTS    *ard.ToolSet  // 可选 ARD 自动发现集成
}
```

**核心功能**:
- 动态启用/禁用扩展
- 细粒度工具权限控制（allowlist/denylist）
- MCP Broker 集成（批量管理外部 MCP Server）
- ARD 自动注册（MCP 连接 → 自动注册到 ARD 目录）
- DeepLink 机制（扩展间引用）

### 7.2 12 内置扩展

| 扩展名 | 源文件 | 工具数 | 启用条件 | 功能描述 |
|--------|--------|--------|----------|----------|
| `developer` | `developer.go` | 多 | 始终 | 文件读写、命令执行、Shell 交互 |
| `computer_controller` | `computer_controller.go` | 多 | `browser.enabled` | Chromedp 浏览器导航/截图/点击/输入 |
| `memory` | `memory.go` | 6 | 始终 | search/add/update/delete/list/clear 记忆 |
| `auto_visualiser` | `auto_visualiser.go` | 多 | `visualiser.enabled` | 自动图表/流程图/思维导图生成 |
| `tutorial` | `tutorial.go` | 多 | `tutorial.enabled` | 交互式教程，支持中英文 |
| `top_of_mind` | `topofmind.go` | 0 | `top_of_mind.enabled` | 持久指令注入系统提示 |
| `code_mode` | `codemode.go` | 多 | `code_mode.enabled` | goja JavaScript 沙箱执行 |
| `apps` | `apps.go` | 多 | `apps.enabled` | HTML 应用克隆/打包/清理/服务 |
| `web` | `web.go` | 多 | 始终 | Web 搜索和抓取 |
| `agent_tools` | `agent.go` | 多 | 始终 | Recipe 子 Agent 工具包装 |
| `ard` | `ard.go` | 7 | `ard.enabled` | ARD 资源发现工具 |
| `cortex` | `cortex.go` | 多 | `cortex.enabled` | CortexDB 知识图谱操作 |

### 7.3 ARD 7 工具

`internal/ard/tools.go`

| 工具 | 功能 | 方向 |
|------|------|------|
| `ard_search` | 语义搜索远程资源 | Outbound |
| `ard_discover` | 联邦发现多个 Registry | Outbound |
| `ard_list` | 列出本地目录资源 | 本地 |
| `ard_get` | 获取资源详情 | 本地 |
| `ard_register` | 注册新资源 | Inbound/本地 |
| `ard_unregister` | 注销资源 | Inbound/本地 |
| `ard_export` | 导出目录 | Outbound |

### 7.4 ARD 双向发现架构

```
Outbound (发现别人):
  ToolSet.Search() → HTTP Client → 远程 Registry → FederatedSearch
  → 语义搜索 → URN 解析 → Agent/Server/MCP Server

Inbound (被人发现):
  RegistryServer(:8081) → /.well-known/ai-catalog.json
  → 其他 ARD Agent 可发现 Wukong

Auto (自动注册):
  MCP 连接建立 → buildARDEntry → ardTS.Register
  A2A Remote 配置 → RegisterA2AAgent → ardTS.Register
```

---

## 8. 安全纵深防御

`internal/security/guard.go` (12.83 KB) + `ignore.go` (7.42 KB)

### 8.1 五层架构

```
Layer 5: Guard 执行防护
  • 4 种模式: auto（自动批准）/ smart（智能判断）/ manual（手动确认）/ chat_only（仅对话）
  • 危险命令拦截: rm -rf /, dd if=/dev/zero, mkfs, fork bomb
  • Prompt 注入检测（tRPC guardrail 插件）
  • 细粒度工具 allowlist/denylist
  • 执行超时控制（default 30s / max 300s）

Layer 4: goja JavaScript 沙箱
  • API 白名单（仅允许安全内置函数）
  • 128MB 内存限制
  • 5 并发 goroutine 限制
  • ReDoS 正则表达式攻击防护
  • 源代码最大 1MB 限制

Layer 3: OS 级文件沙箱（pkg/sandbox）
  • Linux: Landlock（内核 5.13+ 内建）
  • macOS: sandbox-exec(1)（系统内建）
  • Windows: Low Integrity Level + Mandatory Labels
  • 仅允许写入指定目录，其余只读
  • 无 Docker、无守护进程、无额外安装

Layer 2: .wukongignore 文件黑名单
  • gitignore 兼容语法
  • 阻止 Agent 访问 .env / .git / secrets 等敏感文件

Layer 1: OS 级别权限
  • 建议非 root 用户运行
  • ulimit 资源限制
```

### 8.2 Guard 结构体

```go
type Guard struct {
    mu              sync.RWMutex
    cfg             *config.SecurityConfig
    approvedCommands map[string]bool  // 用户已批准的临时白名单
    blockedCount    int
    ignoreMatcher   *IgnoreMatcher    // .wukongignore 匹配器
}
```

### 8.3 权限模式比较

| 模式 | 自动执行 | 需确认操作 | 禁止操作 |
|------|----------|-----------|----------|
| `auto` | 读文件/搜索 | 危险命令 | blocked_commands |
| `smart` | 读文件/搜索/工作目录写入 | 修改外部文件/安装软件 | blocked_commands |
| `manual` | 无 | 所有工具调用 | blocked_commands |
| `chat_only` | 无 | 无（无工具执行） | 所有命令 |

---

## 9. LLM Provider 体系

`internal/provider/factory.go` (8.54 KB) + `acp.go` (6.52 KB)

### 9.1 Provider 注册表

| Provider | 配置类型 | SDK | 特点 |
|----------|----------|-----|------|
| OpenAI | `openai` | openai-go | GPT-4o/GPT-4 系列 |
| Anthropic | `anthropic` | openai-go (兼容) | Claude Sonnet 4 系列 |
| Google | `google` | openai-go (兼容) | Gemini 系列 |
| DeepSeek | `deepseek` | openai-go (兼容) | DeepSeek-Chat 系列 |
| Ollama | `ollama` | openai-go (兼容) | 本地开源模型 |
| LMStudio | `lmstudio` | openai-go (兼容) | 本地模型服务 |
| ACP | `acp` | HTTP Client | 远程 ACP Agent 代理 |

### 9.2 Factory 模式

```go
type Factory struct {
    providers map[string]ProviderInfo
    default_  string
}

// 创建 LLM Model 实例
factory.CreateModel(ctx, modelName) (model.Model, error)

// 创建 Embedding Model 实例
factory.CreateEmbeddingModel(ctx, modelName) (model.Model, error)
```

### 9.3 模型分工策略

```
主对话模型: default_provider + default_model
    ↓ (用于: 用户对话、工具调用)

后台任务模型: lightweight_provider + lightweight_model
    ↓ (用于: 记忆提取、上下文压缩、KG提取、检索规划)

回退链: 子系统.extractor_model → lightweight_model → default_provider
```

---

## 10. 服务与协议

`internal/server/acp.go` (14.57 KB) + `agui.go` (6.64 KB)

### 10.1 四协议概览

| 协议 | 端口 | 路径 | 用途 | 实现 |
|------|------|------|------|------|
| A2A | 9090 | — | Agent-to-Agent 标准通信 | tRPC-A2A-Go v0.2.5 |
| ACP | 9091 | `/acp` | Agent Client Protocol | 自实现 HTTP Server |
| AG-UI SSE | 8080 | `/agui` | Web UI 实时对话流 | SSE (Server-Sent Events) |
| ACP MCP | 3400 | `/mcp` | 跨协议工具桥接 | HTTP → MCP 翻译 |

### 10.2 Bootstrap 启动序列（28 步）

`internal/cli/session.go` 中的 `bootstrapSession()` 函数按严格顺序初始化所有子系统：

```
 1. 加载配置 (Viper)
 2. 设置日志级别
 3. 验证配置
 4. 初始化 OpenTelemetry
 5. 注册 12 个内置扩展
 6. 应用 CLI 参数覆盖
 7. 创建 Model Provider Factory
 8. 创建 MultiPool 数据库池
 9. 创建 Session Service (SQLite)
10. 创建 Memory Service (SQLite)
11. 创建 Todo Service
12. 创建 Recall Store (FTS5)
13. 创建 Cortex Store (HNSW)
14. 创建 Embedder
15. 创建 MemoryFlow Service
16. 创建 GraphFlow Service
17. 创建 Security Guard
18. 创建 Context Manager (Revision)
19. 加载 TopOfMind 持久指令
20. 创建 CoreLoop
21. 启动 A2A Server
22. 启动 ACP Server
23. 启动 AG-UI Server
24. 启动 ACP MCP Bridge
25. 启动 Evolution Engine
26. 启动 Skill Manager (热重载)
27. 启动 Project Manager
28. 启动 TUI 交互界面
```

### 10.3 ACP Server

ACP (Agent Client Protocol) 提供 HTTP 接口供外部客户端连接：

- **Session 管理**: 创建/获取/删除会话
- **消息处理**: 发送用户消息，接收 Agent 响应
- **流式输出**: SSE 方式流式返回 Agent 响应
- **工具调用**: 通过 ACP MCP Bridge 调用 MCP 工具

### 10.4 AG-UI Server

AG-UI 通过 SSE 提供实时对话流：

- 事件类型: `text` / `tool_call` / `tool_result` / `error` / `done`
- 支持 `Last-Event-ID` 断线重连

---

## 11. CortexDB 记忆栈详解

`internal/cortex/` (12 源文件)

### 11.1 组件关系图

```
┌──────────────────────────────────────────────────────┐
│                   CortexDB Stack                       │
├─────────────┬──────────────┬───────────┬──────────────┤
│ CortexStore  │ MemoryFlow   │ GraphFlow  │ ImportFlow   │
│ (核心存储)    │ (转录+唤醒)    │ (知识图谱)  │ (结构导入)    │
├─────────────┼──────────────┼───────────┼──────────────┤
│ embedder.go │ extractor.go │ planner.go │ lexical.go   │
│ (向量嵌入)    │ (实体提取)     │ (检索计划)  │ (词汇搜索)     │
├─────────────┴──────────────┴───────────┴──────────────┤
│  recall_manager.go   kg_tools.go   json_generator.go   │
│  (统一召回管理)        (KG工具)       (JSON输出)         │
└──────────────────────────────────────────────────────┘
```

### 11.2 核心组件

| 组件 | 文件 | 功能 | 技术 |
|------|------|------|------|
| CortexStore | `store.go` | HNSW 向量 + FTS5 全文混合检索 | CortexDB |
| LexicalStore | `lexical.go` | 纯 FTS5 词法搜索 | SQLite FTS5 |
| Embedder | `embedder.go` | 文本 → 向量嵌入 | OpenAI 兼容 API |
| MemoryFlow | `memoryflow.go` | 转录记录 + 3 层唤醒上下文 | CortexDB |
| GraphFlow | `graphflow.go` | 实体/关系提取 + RDF 图谱 | CortexDB RDF |
| Extractor | `extractor.go` | LLM 驱动实体和关系提取 | LLM (lightweight) |
| Planner | `planner.go` | 查询计划生成 | LLM (lightweight) |
| RecallManager | `recall_manager.go` | 统一召回（FTS5 + HNSW + KG） | 多源融合 |
| ImportFlow | `import_flow.go` | DDL/结构化数据 → KG 映射 | CortexDB |
| ImportTools | `import_tools.go` | 导入工具集 | — |
| KGTools | `kg_tools.go` | 知识图谱 CRUD 工具 | SPARQL |
| JSONGenerator | `json_generator.go` | JSON 格式输出 | — |

---

## 12. 关键子系统

### 12.1 上下文压缩 (Revision)

`internal/agent/context.go`

3 层压缩策略:
1. **Trim**: 裁剪旧消息，保留最近 N 轮
2. **LLM Summarize**: 用轻量模型生成对话摘要
3. **Semantic Search**: 语义检索补充相关信息

配置:
```yaml
revision:
  enabled: true
  enable_llm_summarize: true
  summary_cooldown: 120s
  max_context_tokens: 64000
  trim_ratio: 0.3
```

### 12.2 HITL 人机协同

`internal/agent/hitl.go` (5.49 KB)

在关键决策点暂停执行，等待用户确认：
- 危险操作拦截
- 需确认的工具调用
- 多步计划的中间检查点

### 12.3 TodoEnforcer

`internal/agent/todo_enforcer.go` (2.93 KB)

强制 Agent 在执行多步任务时维护 TODO 列表：
- 自动解析 Agent 的 TODO 更新
- 阻止在未完成前置步骤时跳过任务
- 提供 `todo_write` 工具

### 12.4 提示模板系统

`internal/agent/prompt_template.go` (4.44 KB)

- 基于 `text/template` 的提示模板引擎
- 支持条件/循环/变量
- 从 `system_prompt_dir` 加载模板文件
- 运行时变量注入（日期、用户信息、持久指令等）

### 12.5 会话持久化

`internal/session/` (3 文件)

| 后端 | 文件 | 特点 |
|------|------|------|
| SQLite | `store.go` | 默认，本地持久化，单文件 |
| Redis | `redis.go` | 分布式，多实例共享 |
| 内存 | `store.go` | 测试/临时使用 |

### 12.6 Evolution 引擎

`internal/evolution/` (6 文件)

技能自进化流程：
```
1. Monitor    — 监控 Recipe 执行失败
2. Analyze    — LLM 分析失败原因
3. Patch      — 自动生成修复补丁
4. Version    — 版本管理存储
5. Reload     — fsnotify 热重载
```

### 12.7 Summon 子代理委派

`internal/summon/` (6 文件)

- **delegate.go**: 本地子代理创建和管理
- **a2a.go**: A2A 远程代理通信
- **auth.go**: A2A 身份认证（JWT）

### 12.8 CodeMode JS 沙箱

`internal/codemode/executor.go` (15.45 KB)

goja JavaScript 执行器：
- 5 层安全限制
- API 白名单（console.log / JSON / Math / Date）
- 128MB 内存 / 5 并发 / 1MB 代码 / 10s 超时
- ReDoS 正则防护

### 12.9 Browser 浏览器自动化

`internal/browser/controller.go` (16.41 KB)

基于 Chromedp 的浏览器自动化：
- 导航/截图/点击/输入/滚动
- 搜索引擎后端: DuckDuckGo / SearXNG / Tavily
- Headless 模式 / 自定义视口 / 下载大小限制

### 12.10 Apps 应用管理

`internal/apps/` (18 文件, 5 子目录)

| 子目录 | 功能 |
|--------|------|
| `clone/` | Git 克隆 + 缓存 |
| `pack/` | 应用打包（ZIM 格式） |
| `sanitize/` | HTML/CSS/JS 清理 |
| `server/` | 本地 HTTP 服务 |
| `mcpapps/` | MCP 应用桥接 |
| — | `manager.go` — 生命周期管理 |
| — | `history.go` — 版本历史 |

### 12.11 Project 项目追踪

`internal/project/manager.go` (6.46 KB)

- 工作目录与会话关联
- 会话恢复（从上次中断点继续）
- 项目状态持久化

### 12.12 TopOfMind 持久指令

`internal/topofmind/mind.go` (3.60 KB)

- 从 `.wukong/instructions.md` 加载持久指令
- 注入到系统提示中
- 支持 `fsnotify` 热更新

---

## 13. 底层基础设施

### 13.1 数据库 MultiPool

`internal/util/database.go` (6.21 KB)

共享 SQLite 连接池：
```go
// WAL 模式, MaxOpenConns=4, synchronous=NORMAL, busy_timeout=5000ms
pool := util.NewDatabasePool("wukong.db", maxOpenConns)
```

所有子系统共享同一个 `*sql.DB`，避免 SQLite 事务冲突。

### 13.2 日志系统

`internal/util/logger.go` (1.38 KB)

- 基于 `log/slog` 结构化日志
- 支持 debug/info/warn/error 级别
- JSON 和 Text 两种格式

### 13.3 可观测性

#### OpenTelemetry

`internal/telemetry/telemetry.go` (5.84 KB)

- 分布式追踪（Traces）
- 支持 gRPC/HTTP 导出器
- Console 本地导出（开发模式）

#### Langfuse

`internal/observability/langfuse.go` (2.04 KB)

- LLM 调用追踪
- Token 用量统计
- 延迟分析

### 13.4 OS 沙箱 (pkg/sandbox)

`pkg/sandbox/` (10 文件)

跨平台文件系统隔离：

| 平台 | 后端 | 机制 |
|------|------|------|
| Linux | Landlock | 内核 5.13+ 内建 |
| macOS | seatbelt | sandbox-exec(1) |
| Windows | Low IL | 强制完整性标签 |
| 其他 | none | 未沙箱运行（警告） |

特点：
- `os/exec` 兼容 API
- 自动探测当前平台能力
- 不支持时优雅降级
- 无外部依赖

### 13.5 ZIM 格式库 (pkg/zim)

`pkg/zim/` (4 文件)

- ZIM 格式读写
- 用于 Apps 打包
- 支持压缩索引

---

## 14. 关键设计决策 (ADRs)

| # | 决策 | 理由 |
|---|------|------|
| 1 | SQLite WAL 共享 MultiPool | 避免多连接事务冲突，单文件部署 |
| 2 | 双引擎记忆（tRPC + CortexDB） | tRPC 负责长期关键事实，CortexDB 负责语义搜索和知识图谱 |
| 3 | 轻量模型分工 | 主模型处理对话，轻量模型处理后台提取任务 |
| 4 | CoreLoop 依赖注入 | 所有子系统通过 Config 注入，支持替换和测试 |
| 5 | MemoryFlow → tRPC Bridge | 短期事实经评估后提升为长期记忆 |
| 6 | 文件式 Recipe 定义 | YAML 文件热重载，无需重新编译 |
| 7 | 冷启动友好降级 | 无 embedder → 回退 FTS5；无 LLM → 仅结构操作 |
| 8 | HITL 融入编排循环 | 非附加式，在决策点原生暂停 |
| 9 | SmartCleanup 容量淘汰 | 70% 新鲜度 + 30% 长度，80% 触发 → 60% 目标 |
| 10 | Extension Manager 统一管理 | 内置和外部 MCP 采用统一接口 |
| 11 | ACP + AG-UI 双 UI 协议 | ACP 面向客户端，AG-UI SSE 面向浏览器 |
| 12 | ACP MCP Bridge | 跨协议工具调用，ACP 客户端可操作 MCP 工具 |
| 13 | MCP Broker 批量管理 | 外部 MCP Server 通过 Broker 统一暴露 |
| 14 | 提示模板分离 | text/template 渲染，运行时变量注入 |
| 15 | goja JS 多层沙箱 | API白名单+内存+并发+ReDoS+代码长度 5 层限制 |
| 16 | OS 级沙箱跨平台 | Landlock/Seatbelt/LowIL 三种后端 |
| 17 | ARD 双向发现 | 联邦搜索 + RegistryServer 发布 |
| 18 | ARD 联邦搜索 | 多 Registry 并行搜索 + 结果去重 |
| 19 | Evolution 版本管理 | 每个补丁保留版本，支持回滚 |
| 20 | TopOfMind 持久指令 | fsnotify 监控文件变更，自动重载 |
| 21 | 单文件 wukong.db 全栈存储 | 简化部署，SQLite WAL 提供足够并发 |
| 22 | OpenTelemetry 分布式追踪 | 标准化可观测性，支持多种导出器 |
| 23 | Chromedp 浏览器自动化 | 纯 Go 实现，无外部依赖 |
| 24 | Recipe 重试指数退避 | `initial_wait * backoff_factor^(attempt-1)` |
| 25 | 4 协议服务器共存 | 满足不同客户端需求 |
| 26 | goroutine 生命周期管理 | `bgWg` + context 确保优雅关闭 |
| 27 | ToolSearch 自动工具筛选 | 大量工具时减少提示词 token 消耗 |
| 28 | ContextCompaction 两遍压缩 | Trim + LLM Summarize 双层保障 |

---

## 15. 技术栈全景

| 类别 | 技术 | 版本 | 用途 |
|------|------|------|------|
| Agent 框架 | tRPC-Agent-Go | v1.10.0 | Agent 编排、Runner、Planner、Guardrail |
| MCP 协议 | tRPC-MCP-Go | v0.0.16 | 模型上下文协议（MCP Broker + Client） |
| A2A 协议 | tRPC-A2A-Go | v0.2.5 | Agent-to-Agent 通信 |
| 智能记忆 | CortexDB | v2.25.0 | HNSW 向量 + FTS5 全文 + RDF 图谱 |
| CLI 框架 | Cobra | v1.9.1 | 命令行解析 |
| 配置管理 | Viper | v1.20.1 | 7 级配置加载 |
| TUI 界面 | Bubbletea + Bubbles + LipGloss | v1.3.10 / v0.21.0 / v1.1.0 | 三区交互终端 |
| 浏览器 | Chromedp | v0.15.1 | 无头浏览器自动化 |
| JS 引擎 | goja | latest | JavaScript 沙箱执行 |
| LLM SDK | openai-go | v1.12.0 | OpenAI 兼容 API |
| 数据库 | modernc.org/sqlite | v1.38.2 | 纯 Go SQLite (CGO 禁用) |
| 缓存 | go-redis | v9.12.1 | Redis 会话后端 |
| 文件监控 | fsnotify | v1.8.0 | 技能/配置热重载 |
| 可观测性 | OpenTelemetry | v1.43.0 | 分布式追踪 |
| UUID | google/uuid | v1.6.0 | 唯一标识生成 |
| JSON | goccy/go-json | v0.10.6 | 高性能 JSON 库 |
| JWT | lestrrat-go/jwx | v2.1.4 | A2A 身份认证 |
| 文件 | doublestar | v4.9.1 | glob 模式匹配 |
| 并发池 | ants | v2.10.0 | goroutine 池管理 |
