# Wukong 系统架构深度分析

> **版本**: v0.7.0 | **Go**: 1.26 | **Go 文件**: 138 | **内部包**: 27 | **直接依赖**: 30+
>
> 基于 [tRPC-Agent-Go v1.10.0](https://github.com/trpc-group/trpc-agent-go) · [tRPC-MCP-Go v0.0.16](https://github.com/trpc-group/trpc-mcp-go) · [tRPC-A2A-Go v0.2.5](https://github.com/trpc-group/trpc-a2a-go) · [CortexDB v2.25.0](https://github.com/liliang-cn/cortexdb)

---

## 目录

1. [架构哲学](#1-架构哲学)
2. [系统全景](#2-系统全景)
3. [CoreLoop 执行引擎](#3-coreloop-执行引擎)
4. [多 Agent 编排](#4-多-agent-编排)
5. [Recipe 子系统](#5-recipe-子系统)
6. [双引擎记忆系统](#6-双引擎记忆系统)
7. [扩展与工具系统](#7-扩展与工具系统)
8. [安全纵深防御](#8-安全纵深防御)
9. [LLM Provider 体系](#9-llm-provider-体系)
10. [技能自进化系统](#10-技能自进化系统)
11. [配置系统](#11-配置系统)
12. [服务与协议层](#12-服务与协议层)
13. [存储架构](#13-存储架构)
14. [关键设计决策 (ADR)](#14-关键设计决策-adr)
15. [技术选型](#15-技术选型)

---

## 1. 架构哲学

Wukong 的五大核心哲学决定所有工程决策：

| 哲学 | 核心信念 | 关键工程决策 |
|------|----------|-------------|
| **记忆优先** | Agent 智能源于跨会话知识积累 | 双引擎记忆闭环 + 知识图谱 + HNSW 向量 |
| **框架组装** | 任何组件都应可替换 | CoreLoop 依赖注入，12 个子系统解耦 |
| **多 Agent 原生** | 编排是第一公民 | 10 种显式编排模式 + HITL |
| **进化智能** | 技能应自我改进 | LLM 分析 → 自动补丁 → 版本管理 → 热重载 |
| **纵深防御** | 安全是多层协同 | 5 层防御从 LLM 权限到 OS 内核 |

---

## 2. 系统全景

```
┌──────────────────────────────────────────────────────────────────┐
│                     Wukong AI Agent Platform                      │
├──────────────────────────────────────────────────────────────────┤
│ Entry Points:                                                     │
│   CLI (cobra + BubbleTea TUI)  │  API (A2A:9090/ACP:9091/AG-UI:8080) │
├──────────────────────────────────────────────────────────────────┤
│ Core Engine: CoreLoop — 中央编排器，协调 12 个子系统               │
│   ├── WorkflowBuilder (10 种模式) + TeamBuilder (多 Agent)        │
│   ├── ContextManager (3 层压缩)                                    │
│   └── Security Guard (5 层防御)                                    │
├──────────────────────────────────────────────────────────────────┤
│ Agent Framework: tRPC-Agent-Go Runner                             │
│   LLMAgent / ChainAgent / ParallelAgent / CycleAgent / GraphAgent │
│   Session / Memory / Artifact Service                             │
│   Planner / ToolSearch / ContextCompaction / Skill                │
├──────────────────────────────────────────────────────────────────┤
│ Memory Stack (双引擎):                                            │
│ ┌─ tRPC Memory (SQLite KV) ──────────────────────────────────┐   │
│ │  AutoExtract → 异步记忆提取 (轻量模型) → AddMemory         │   │
│ │  6 tools: add/search/update/delete/load/clear              │   │
│ │  SmartCleanup: 容量感知评分淘汰 (80%→60%)                  │   │
│ ├──────────────────────────────────────────────────────────┤   │
│ │  CortexDB Stack (HNSW + FTS5 + RDF)                       │   │
│ │  ├── MemoryFlow: IngestTurn → WakeUp → PromoteFacts        │   │
│ │  ├── GraphFlow: 实体提取 → RDF KG (auto_extract)           │   │
│ │  ├── ImportFlow: DDL/CSV → KG 导入                         │   │
│ │  └── CortexStore: HNSW 向量 + FTS5 全文                    │   │
│ └──────────────────────────────────────────────────────────┘   │
├──────────────────────────────────────────────────────────────────┤
│ Capability Layer:                                                 │
│   Recipe System (P0-P4) · Extension Manager (10 built-ins)       │
│   Evolution Engine · Summon (A2A) · CodeMode (goja JS)           │
│   Browser (Chromedp) · Knowledge (RAG) · pkg/sandbox (OS lock)   │
│   TopOfMind · Health Check · Todo System · Project Manager       │
├──────────────────────────────────────────────────────────────────┤
│ Infrastructure:                                                   │
│   Provider Factory (7 LLM backends) · Viper Config Loader        │
│   DatabasePool (shared SQLite WAL) · OTel + Langfuse             │
│   fsnotify hot-reload · text/template rendering                  │
├──────────────────────────────────────────────────────────────────┤
│ Storage: wukong.db (单文件 SQLite WAL, shared DatabasePool)       │
│   sessions / memories / recall(FTS5) / todos / projects          │
│   cortex: transcripts(episodes) / entities / relations / HNSW    │
│   evolution: skill_versions / evolution_records                  │
└──────────────────────────────────────────────────────────────────┘
```

---

## 3. CoreLoop 执行引擎

`CoreLoop` (`internal/agent/loop.go`) 是系统的中央编排器，通过依赖注入持有 12 个子系统：

### 3.1 结构定义

```go
type CoreLoop struct {
    agent          agent.Agent           // tRPC-Agent-Go Agent 实例
    runner         runner.Runner         // tRPC-Agent-Go Runner
    sessionService session.Service       // 会话管理
    memoryService  memory.Service        // 持久记忆服务
    factory        *provider.Factory     // LLM Provider 工厂
    cfg            *config.WukongConfig  // 完整配置
    contextMgr     *ContextManager       // 上下文压缩引擎
    security       *security.Guard       // 安全守卫
    recallStore    *recall.Store         // FTS5 对话搜索
    cortexStore    *cortex.CortexStore   // HNSW 向量存储
    memoryFlow     *cortex.MemoryFlowService  // 转录回溯
    graphFlow      *cortex.GraphFlowService   // 知识图谱
    closeFn        func() error          // 组合关闭函数
    mu             sync.RWMutex
    closed         bool
    bgWg           sync.WaitGroup
}
```

### 3.2 执行循环

```
CoreLoop.Run(userID, sessionID, message)
│
├── Phase 1: 对话前准备 (上下文注入)
│   ├── ContextManager.PrepareContext()     → 上下文压缩
│   ├── recallStore.StoreMessage(user)      → [FTS5 + HNSW]
│   ├── cortexStore.StoreMessage(user)      → [HNSW 向量]
│   ├── memoryFlow.IngestTurn(user)         → [CortexDB Episode]
│   ├── memoryFlow.WakeUp()                 → [向量+FTS5 唤醒上下文]
│   ├── memoryService.ReadMemories()         → [持久记忆 + 去重]
│   └── graphFlow.BuildContext()            → [KG 增强上下文]
│
├── Phase 2: Agent 执行
│   └── runner.Run()
│       ├── LLM 推理 (主模型)
│       ├── Tool Calls → Guard.Check() → 执行
│       │   ├── Permission Mode 检查
│       │   ├── Allowlist/Denylist 匹配
│       │   ├── Threat 扫描 → 超时控制
│       │   └── 用户审批 (smart/manual 模式)
│       ├── AutoExtract (异步, 轻量模型)
│       └── SummaryJob (异步, 上下文摘要)
│
├── Phase 3: 对话后收尾
│   ├── recallStore.StoreMessage(assistant/tool_*) → [FTS5+HNSW]
│   ├── cortexStore.StoreMessage(*)                 → [HNSW]
│   ├── memoryFlow.IngestTurn(assistant)            → [Episode]
│   ├── memoryFlow.PromoteFacts()                   → [事实→持久化]
│   └── graphFlow auto-extract                      → [实体/关系→KG]
│
└── Phase 4: 返回响应
    └── contextMgr.AfterRun() → token 统计更新
```

### 3.3 关闭序列

```
Close() — 6 步严格顺序:
├── 1. runner.Close()              → 停止 Agent Runner
├── 2. evolution.Close()           → 停止进化引擎 worker
├── 3. memory.Close()              → 停止 AutoExtract (5s timeout)
├── 4. session.Close() + graphFlow.Close()  → 关闭持久化层
├── 5. telemetry.Shutdown(10s)     → 刷新 OTLP + Langfuse traces
└── 6. dbPool.Close()              → WAL checkpoint → close
```

---

## 4. 多 Agent 编排

### 4.1 WorkflowBuilder (workflow.go)

根据 `WorkflowConfig.Mode` 构建 10 种拓扑：

| 模式 | 拓扑 | Agent 类型 | 适用场景 |
|------|------|-----------|----------|
| `single` | 单 Agent | LLMAgent | 简单对话（默认） |
| `chain` | 顺序管道 (planner→executor→reviewer) | ChainAgent | 多步流水线 |
| `parallel` | 并发执行 (code/doc/test analyzer) | ParallelAgent | 多角度分析 |
| `cycle` | 迭代循环 (planner↔executor) | CycleAgent | 自我改进 |
| `graph` | 条件路由 (analyze→code/search/answer→review) | GraphAgent | 复杂决策 |
| `team_coordinator` | Leader 委托成员 | Team | 团队协作 |
| `team_swarm` | 蜂群自主 transfer | Team (Swarm) | 自主委派 |
| `claude_code` | Claude CLI 封装 | claudecode Agent | 本地 Claude |
| `codex` | Codex CLI 封装 | codex Agent | 本地 Codex |
| `dify` | Dify 平台 | DifyAgent | 低代码编排 |

### 4.2 TeamBuilder (team.go)

| 模式 | 协调方式 | 关键实现 |
|------|----------|----------|
| `team_coordinator` | Coordinator 通过 AgentTool 调度 | `team.New(coordinator, members)` + `WithEnableParallelTools(true)` |
| `team_swarm` | Agent 间 `transfer_to_agent` | `team.NewSwarm(entry, members)` + `WithCrossRequestTransfer(true)` |

默认团队成员：researcher（研究）、coder（编程）、reviewer（审查）。

### 4.3 HITL 人机协同 (hitl.go)

GraphAgent 模式支持中断-恢复：
```
graph.AddInterruptBefore("dangerous_op")
  → 执行到危险节点前暂停 → 用户审批 → ResumeInterrupted → 继续执行
```

---

## 5. Recipe 子系统

Recipe 是基于 YAML 的结构化子 Agent 定义系统（`recipe.go`, `recipe_tool.go`, `recipe_compose.go`, `recipe_advance.go`, `recipe_metrics.go`）。

### 5.1 五阶段加载流水线

```
Phase 1: 加载配置（文件 + 内联）→ map[name]*RecipeConfig
Phase 2: 解析 extends 继承链 → resolveAllExtends
Phase 3: 拓扑排序子配方依赖 → topoSortRecipes
Phase 4: 按序构建工具 (agenttool → recipeTool → retryTool → timeoutTool)
Phase 5: 注册发现/热重载/统计工具，返回主 Agent
```

### 5.2 功能矩阵

| 阶段 | 功能 | YAML 字段 | 实现 |
|------|------|----------|------|
| P0-A | 参数化模板 | `prompt`, `parameters` | Go text/template 渲染 |
| P0-B | 结构化输出 | `response.json_schema` | `WithStructuredOutputJSONSchema` |
| P1-A | 子配方组合 | `tools: [recipe-xxx]` | 拓扑排序 + DAG 构建 |
| P1-B | 重试与校验 | `retry`, `response.validate_output` | 指数退避 + JSON 校验 |
| P2-A | 内联配方 | `agent.inline_recipes` | YAML round-trip 转换 |
| P2-B | 配方继承 | `extends` | 递归继承合并 |
| P3-A | 模型覆盖 | `model` | `CreateModelWithName` |
| P3-B | 超时控制 | `timeout` | context.WithTimeout |
| P3-C | 配方发现 | `list_recipes` 工具 | JSON 返回配方列表 |
| P3-D | 热重载 | `reload_recipes` + fsnotify | 500ms 防抖自动重建 |
| P4-A | 指令模板 | `instruction: "{{.var}}"` | 与 prompt 共同渲染 |
| P4-B/C | 执行指标 | `recipe_stats` 工具 | CallCount/Success/Error/Duration |

### 5.3 工具包装器链

Recipe 工具构建时按以下顺序应用包装器（最内层到最外层）：

```
agenttool.NewTool (框架层子 Agent 工具)
  → recipeTool (参数校验 + 模板渲染 + 指令模板 + 指标记录)
    → retryTool (指数退避重试 + 输出校验)
      → timeoutTool (context.WithTimeout 超时控制)
```

### 5.4 Recipe YAML 示例

```yaml
name: multi_lang_reviewer
description: "Parameterized code reviewer"
extends: base_reviewer                  # P2-B 继承
instruction: "You are a {{.language}} expert."  # P4-A 指令模板
prompt: "Review the {{.language}} code:\n{{.code}}"
parameters:                            # P0-A 参数
  - key: language
    type: select
    options: [go, python, rust]
  - key: code
    type: string
    required: true
tools:                                 # P1-A 子配方
  - file_read
  - recipe-sub-reviewer
model: "gpt-4o"                        # P3-A 模型
timeout: "30s"                         # P3-B 超时
retry:                                 # P1-B 重试
  max_attempts: 3
  initial_wait: "1s"
response:                              # P0-B 结构化输出
  json_schema: {type: object, required: [issues, summary]}
  validate_output: true
```

### 5.5 辅组工具

| 工具 | 功能 |
|------|------|
| `list_recipes` | 列出所有 recipe 的名称、描述、参数 schema (P3-C) |
| `reload_recipes` | 从磁盘重新加载所有 recipe (P3-D) |
| `recipe_stats` | 查询 recipe 执行统计：调用次数、成功率、耗时 (P4-C) |

---

## 6. 双引擎记忆系统

### 6.1 引擎一：tRPC Memory (SQLite KV)

**实现**: `internal/memory/store.go`

| 功能 | 说明 |
|------|------|
| `AutoExtract` | 每轮对话后异步 LLM (轻量模型) 提取事实 |
| `SmartCleanup` | 80% 容量阈值触发评分淘汰，降至 60%。70% recency + 30% length |
| `MemoryBridge` | 接收 MemoryFlow.PromoteFacts 写入的事实 |

**6 个工具**：`memory_add`, `memory_search`, `memory_update`, `memory_delete`, `memory_load`, `memory_clear`

### 6.2 引擎二：CortexDB Stack

#### CortexStore (store.go)
基于 CortexDB 的 HNSW 向量搜索 + FTS5 全文搜索。双写策略：FTS5 写入 → HNSW 向量索引（有 embedder 时）。搜索时优先 HNSW 语义搜索，失败回退 FTS5。

#### MemoryFlow (memoryflow.go)
对话转录 + 语义唤醒：
- `IngestTurn()` → 存储到 CortexDB Episode
- `WakeUp()` → 三层上下文构建（身份/回忆/会话）
- `PromoteFacts()` → 事实提取 → 桥接 tRPC Memory

#### GraphFlow (graphflow.go)
知识图谱构建：
- `auto_extract`：每轮对话后自动执行
- 流程：BuildTranscript → ExtractEntities → BuildGraph(RDF)
- 支持 SPARQL 查询

#### Extractor (extractor.go)
LLM + 启发式双重事实提取，回退链：专用模型 → 默认模型 → 禁用/启发式。

### 6.3 记忆闭环数据流

```
Before Run (上下文注入)                    After Run (记录与提取)
┌───────────────────────────┐         ┌─────────────────────────────┐
│ WakeUp (CortexDB)         │         │ StoreMessage (FTS5 + HNSW)   │
│ ReadMemories (tRPC SQLite)│         │ IngestTurn (Episode)         │
│ BuildContext (KG增强)     │         │ PromoteFacts → tRPC 持久化  │
│ 去重合并                   │         │ GraphFlow auto_extract → KG │
└───────────────────────────┘         └─────────────────────────────┘
```

---

## 7. 扩展与工具系统

### 7.1 ExtensionManager (extension/manager.go)

管理 MCP 扩展的完整生命周期。支持 Deeplink URL 或 YAML 配置注册。

**MCP Broker**：4 个入口工具聚合外部 MCP Server：
- `mcp_list_servers` — 列出已注册 MCP 服务器
- `mcp_list_tools` — 列出某服务器工具
- `mcp_inspect_tools` — 检查工具参数详情
- `mcp_call` — 调用 MCP 工具

**10 个内置扩展** (`builtin/registry.go`)：

| 扩展名 | 功能 |
|--------|------|
| `developer` | 文件读写、命令执行 |
| `computer_controller` | Chromedp 浏览器自动化 |
| `memory` | 记忆管理 (6 tools) |
| `auto_visualiser` | 自动可视化 |
| `tutorial` | 教程系统 |
| `top_of_mind` | 持久指令注入 |
| `code_mode` | goja JS 沙箱 |
| `apps` | HTML 应用管理 |
| `web` | Web 工具 |
| `agent_tools` | Agent 工具 |

### 7.2 ACP MCP Bridge (extension/acp_mcp.go)

将 Wukong 内置扩展暴露为 MCP Server，通过 Streamable HTTP 在 `:3400/mcp` 提供服务。

---

## 8. 安全纵深防御

### 8.1 5 层防御模型

```
Layer 5: Guard 权限控制    → auto/smart/manual/chat_only + 命令拦截
Layer 4: goja JS 沙箱      → API 白名单 + 128MB 内存限制 + 5 并发 + ReDoS 防护
Layer 3: sandbox OS 级隔离  → Landlock(linux) / sandbox-exec(macOS) / Low IL(Windows)
Layer 2: .wukongignore      → gitignore 兼容文件访问黑名单
Layer 1: OS 进程权限         → 非 root + ulimit
```

### 8.2 Guard 详解

```go
type Guard struct {
    cfg              *config.SecurityConfig
    approvedCommands map[string]bool  // 用户批准的运行时命令
    ignoreMatcher    *IgnoreMatcher   // .wukongignore 匹配器
}
```

4 种 PermissionMode：`auto` (全自动)、`smart` (智能审批，默认)、`manual` (全部审批)、`chat_only` (纯文本)。

### 8.3 CodeMode goja JS 沙箱

| 安全措施 | 实现 |
|----------|------|
| API 白名单 | console / JSON / Math / __output |
| 显式禁用 | eval / Function / setInterval / Date / RegExp |
| 内存限制 | `debug.SetMemoryLimit(128MB)` |
| 超时控制 | `context.WithTimeout(10s)` |
| 并发控制 | channel semaphore (max 5) |
| JSON 保护 | 1MB 输入限制 |

### 8.4 OS 级沙箱 (pkg/sandbox/)

| 平台 | 技术 |
|------|------|
| Linux (5.13+) | Landlock LSM，全文件系统只读，仅 WritableDirs 可写 |
| macOS | sandbox-exec + Seatbelt 动态 profile |
| Windows | Low Integrity Level + Mandatory Labels |
| 其他 | 非沙箱运行 + WARN 日志 |

---

## 9. LLM Provider 体系

### 9.1 Factory (provider/factory.go)

```go
type Factory struct {
    cfg     *config.WukongConfig
    mcpAddr string  // ACP MCP bridge address
}
```

| Provider | type | 基础 URL | SDK |
|----------|------|----------|-----|
| OpenAI | `openai` | `https://api.openai.com/v1` | openai-go |
| Anthropic | `anthropic` | `https://api.anthropic.com` | openai-go (兼容) |
| Google | `google` | 自动 | openai-go (Gemini via OpenAI API) |
| DeepSeek | `deepseek` | `https://api.deepseek.com` | openai-go |
| Ollama | `ollama` | `http://localhost:11434/v1` | openai-go |
| LMStudio | `lmstudio` | `http://localhost:1234/v1` | openai-go |
| ACP | `acp` | agent_url | HTTP client |

### 9.2 模型分工

| 用途 | 典型模型 | 配置 |
|------|---------|------|
| 主对话 | 默认 provider | CLI `--provider` / `--model` |
| 记忆提取 | 轻量模型 | `memory.extractor_model` |
| 上下文压缩 | 独立模型 | `revision.revision_model` |
| 知识图谱提取 | 独立模型 | `graphflow.extractor_model` |
| Recipe 模型 | 可覆盖 | `recipe.Model` → `CreateModelWithName` |

### 9.3 关键方法

| 方法 | 说明 |
|------|------|
| `CreateModel(name)` | 按 provider 名称创建模型 |
| `CreateModelWithName(provider, model)` | 覆盖模型名称创建模型 (P3-A) |
| `CreateDefaultModel()` | 使用默认 provider 创建 |
| `CreateRevisionModel()` | 创建上下文压缩专用模型 |

---

## 10. 技能自进化系统

### 10.1 EvolutionEngine (evolution/engine.go)

```go
type EvolutionEngine struct {
    analyzer   *EvolutionAnalyzer   // LLM 分析器
    patcher    *EvolutionPatcher    // 补丁应用器
    store      *VersionStore        // 版本存储
    refresher  SkillRefresher       // 热重载触发器
    analysisCh chan *ExecutionTrace // 异步分析通道
}
```

闭环流程：
```
Agent 执行 → ExecutionTrace 收集
  → 异步分析队列 (后台 goroutine)
    → LLM Analysis (置信度 ≥ 0.7)
      → 生成 PatchSuggestion
    → Patch Application
      → 备份 SKILL.md (版本管理)
      → 应用补丁
    → 热重载 (SkillManager.Refresh)
```

### 10.2 约束机制

| 约束 | 值 | 说明 |
|------|-----|------|
| 最小置信度 | 0.7 | 低于此阈值的建议被拒绝 |
| 冷却时间 | 30min | 同一技能两次进化间隔 |
| 每日上限 | 10 | 全局每日最大补丁数 |
| 最大补丁 | 8KB | 单个补丁内容大小限制 |
| 版本保留 | 10 | 保留最近历史版本数 |

### 10.3 Summon 子代理委托 (summon/)

将子代理（skills）包装为可调用工具。使用 `llmagent` 构建，注入模型/指令/工具。温度 0.3，最大 LLM 调用 10 次。

---

## 11. 配置系统

### 11.1 配置加载优先级

```
1. CLI 参数 (--provider, --model, --temperature, --max-tokens)
2. 环境变量 (WUKONG_ 前缀)
3. --config CLI 指定的文件
4. 当前目录 ./config.yaml
5. ~/.config/wukong/config.yaml
6. /etc/wukong/config.yaml (非 Windows)
7. 内置默认值
```

### 11.2 配置分类

| 类别 | 关键结构 |
|------|----------|
| 根 | `WukongConfig` |
| Provider | `ProviderConfig` (7 种 backend) |
| Extension | `ExtensionConfig`, `ToolPermission` |
| Agent | `AgentConfig` (20+ 字段含 recipe/inline_recipes/agent_tools) |
| Security | `SecurityConfig` (11 字段) |
| Storage | `SessionConfig`, `MemoryConfig`, `TodoConfig` |
| CortexDB | `CortexConfig`, `MemoryFlowConfig`, `GraphFlowConfig` |
| Context | `RevisionConfig` (11 字段) |
| Features | `BrowserConfig`, `CodeModeConfig`, `AppsConfig` 等 |
| Services | `A2AServerConfig`, `AGUIConfig`, `ACPServerConfig` |
| Evolution | `EvolutionConfig`, `SkillConfig` |

---

## 12. 服务与协议层

### 12.1 协议端点

| 协议 | 端口 | 路径 | 用途 |
|------|------|------|------|
| A2A | 9090 | / | Agent-to-Agent 标准通信 |
| ACP | 9091 | /acp | Agent Client Protocol |
| AG-UI SSE | 8080 | /agui | Web UI 实时对话 (SSE 流) |
| ACP MCP | 3400 | /mcp | 跨协议工具桥接 |

### 12.2 优雅关闭

所有 HTTP 端点使用 `*http.Server` + `Shutdown(ctx)`：
1. 停止接收新连接
2. 等待活跃请求完成
3. 超时强制终止

---

## 13. 存储架构

### 13.1 单文件数据库

```
wukong.db (SQLite WAL)
├── sessions              ← Session Service
├── memories              ← tRPC Memory Service
├── recall                ← FTS5 全文搜索 Store
├── todos                 ← Todo Store
├── projects              ← Project Manager
├── cortex_*              ← CortexDB (episodes/entities/relations/HNSW)
├── evolution_*           ← Evolution Engine (skill_versions/evolution_records)
└── extension_*           ← Extension Manager registry
```

### 13.2 DatabasePool 配置

| 参数 | 值 |
|------|-----|
| `_journal_mode` | WAL |
| `_synchronous` | NORMAL |
| `_foreign_keys` | ON |
| `_busy_timeout` | 5000ms |
| `MaxOpenConns` | 4 |
| `MaxIdleConns` | 2 |

关闭时 `PRAGMA wal_checkpoint(TRUNCATE)` 确保 WAL 刷新。

---

## 14. 关键设计决策 (ADR)

| ADR | 决策 | 影响 |
|-----|------|------|
| ADR-1 | SQLite WAL 共享池，MaxOpenConns=4 | 单文件部署 + FTS5 + 并发支持 |
| ADR-2 | 双引擎记忆 (tRPC + CortexDB) | KV 持久化 + 向量/图谱结构化 |
| ADR-3 | 轻量模型分工 | 节省主模型 token |
| ADR-4 | CortexDB 实例共享 | 避免 WAL 双连接冲突 |
| ADR-5 | MemoryFlow → tRPC Bridge | 转录事实自动提升为持久记忆 |
| ADR-6 | MCP Broker 4 入口模式 | 防止工具泛滥 |
| ADR-7 | 冷启动友好降级 | 无 Embedding 回退 FTS5 |
| ADR-8 | 单文件数据库 | 跨系统查询零成本 |
| ADR-9 | SmartCleanup 容量淘汰 | 70% recency + 30% length |
| ADR-10 | 记忆去重 (WakeUp ↔ ReadMemories) | 滑动窗口防重复 |
| ADR-11 | Tool 消息完整索引 | FTS5 + HNSW 全覆盖 |
| ADR-12 | GraphFlow auto_extract | 每轮对话自动 KG |
| ADR-13 | Extractor 三层回退链 | 专用模型 → 默认 → 启发式 |
| ADR-14 | 非阻塞 Evolution | 后台 goroutine 不阻塞主循环 |
| ADR-15 | CoreLoop 依赖注入 | 12 子系统接口隔离 |
| ADR-16 | 三协议服务器 | A2A + ACP + AG-UI |
| ADR-17 | HTTP body 10MB 限制 | DoS 防护 |
| ADR-18 | goja JS 多层沙箱 | API 白名单 + 内存 + 并发控制 |
| ADR-19 | Context 超时覆盖 | 防止 goroutine 泄漏 |
| ADR-20 | Goroutine bgWg 管理 | 关闭前 Wait() |
| ADR-21 | OS 级沙箱跨平台 | Landlock/Seatbelt/Low IL |
| ADR-22 | sandbox 独立 pkg/ 包 | 可被外部项目导入复用 |
| ADR-23 | Recipe 拓扑排序构建 | DAG 依赖顺序 + 循环检测 |

---

## 15. 技术选型

| 类别 | 选择 | 版本 | 理由 |
|------|------|------|------|
| Agent 框架 | tRPC-Agent-Go | v1.10.0 | 多 Agent 编排、Session/Memory/Planner/Skill |
| MCP 协议 | tRPC-MCP-Go | v0.0.16 | stdio/sse/streamable 三传输 |
| A2A 协议 | tRPC-A2A-Go | v0.2.5 | Agent 间标准通信 |
| 智能记忆 | CortexDB | v2.25.0 | HNSW + FTS5 + RDF/SPARQL |
| JS 引擎 | goja | latest | 纯 Go 零 CGO、沙箱友好 |
| OS 沙箱 | pkg/sandbox | 自维护 | Landlock/Seatbelt/Low IL |
| 数据库 | SQLite WAL | via mattn | 单文件零配置 |
| 前端 | BubbleTea + LipGloss | latest | 纯 Go TUI |
| 浏览器 | Chromedp | latest | Chrome DevTools Protocol |
| 配置 | Viper + Cobra | latest | CLI > ENV > YAML |
| 可观测 | OpenTelemetry + Langfuse | latest | 全链路追踪 |
| 文件监听 | fsnotify | v1.8.0 | Recipe 热重载 |
| 模板引擎 | text/template | stdlib | Recipe prompt/instruction 渲染 |
| 语言 | Go | 1.26 | 跨平台交叉编译 |
