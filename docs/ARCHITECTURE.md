# Wukong 系统架构深度分析

> **版本**: v0.6.1 | **Go**: 1.26 | **总源文件**: 119 `.go` + 34 `_test.go` | **直接依赖**: 30
> **外发包**: `pkg/sandbox/` (10 文件)
>
> 基于 [tRPC-Agent-Go v1.10.0](https://github.com/trpc-group/trpc-agent-go) · [tRPC-MCP-Go v0.0.16](https://github.com/trpc-group/trpc-mcp-go) · [CortexDB v2.25.0](https://github.com/liliang-cn/cortexdb)

---

## 目录

1. [架构哲学](#1-架构哲学)
2. [系统全景](#2-系统全景)
3. [Agent 执行引擎](#3-agent-执行引擎)
4. [记忆系统 —— 双引擎架构](#4-记忆系统--双引擎架构)
5. [多 Agent 编排](#5-多-agent-编排)
6. [扩展与工具系统](#6-扩展与工具系统)
7. [安全纵深防御](#7-安全纵深防御)
8. [上下文管理](#8-上下文管理)
9. [LLM Provider 体系](#9-llm-provider-体系)
10. [服务与协议层](#10-服务与协议层)
11. [存储架构](#11-存储架构)
12. [技能自进化系统](#12-技能自进化系统)
13. [完整数据流](#13-完整数据流)
14. [技术选型](#14-技术选型)
15. [关键设计决策 (ADR)](#15-关键设计决策-adr)

---

## 1. 架构哲学

Wukong 的五条核心哲学：

| 哲学 | 核心信念 | 关键工程决策 |
|------|----------|-------------|
| **记忆优先** | Agent 智能来源于知识积累 | 双引擎记忆闭环 + 知识图谱 + HNSW 向量 |
| **框架组装** | 组件应可替换 | CoreLoop 依赖注入，27 个子系统解耦 |
| **多 Agent 原生** | 编排是第一公民 | 10 种显式模式，非事后插件 |
| **进化智能** | 技能应自我改进 | LLM 分析 → 自动补丁 → 热重载 |
| **纵深防御** | 安全是多层协同 | 5 层防御从 LLM 权限到 OS 内核 |

---

## 2. 系统全景

```
                        Wukong AI Agent Platform

Entry Points:
  CLI (cobra + BubbleTea) │ API (A2A :9090 / ACP :9091 / AG-UI :8080 / MCP :3400)

Core Engine:
  CoreLoop ─── 中央编排器，协调 27 个子系统
    ├── WorkflowBuilder (10 种模式) + TeamBuilder (多 Agent)
    ├── ContextRevisionEngine (3 层压缩)
    └── Security Guard (5 层防御)

Agent Framework:
  tRPC-Agent-Go Runner
    ├── LLMAgent / ChainAgent / ParallelAgent / CycleAgent / GraphAgent
    ├── Session Service / Memory Service / Artifact Service
    └── Planner / Tool / Plugin / ToolSearch

Memory Stack (双引擎):
  ┌─ tRPC Memory (SQLite KV) ──────────────────────────┐
  │  AutoExtract → 异步记忆提取 → ReadMemories         │
  │  6 工具: add/search/update/delete/load/clear       │
  │  SmartCleanup: 容量感知淘汰 (80%→60%)              │
  ├─────────────────────────────────────────────────────┤
  │  CortexDB Stack (HNSW + FTS5 + RDF)                │
  │  ┌── MemoryFlow: IngestTurn → WakeUp → PromoteFacts│
  │  ├── GraphFlow: 实体提取 → RDF 知识图谱 (auto_extr)│
  │  └── ImportFlow: DDL/CSV → KG 导入                │
  └─────────────────────────────────────────────────────┘

Capability Layer:
  Security Guard · Extension Manager · Evolution Engine
  Memory + Recall + CortexDB Stack
  Knowledge (RAG) / Browser (Chromedp) / CodeMode (goja)
  Summon + Skill / pkg/sandbox (OS lock)

Infrastructure:
  Provider Factory (7 LLM backends) / Viper Config Loader
  DatabasePool (shared SQLite WAL) / Telemetry (OTel + Langfuse)

Storage:
  wukong.db (单文件 SQLite WAL)
    sessions / memories / recall (FTS5) / todos
    transcripts / entities (RDF) / relations / HNSW vectors
```

---

## 3. Agent 执行引擎

### 3.1 CoreLoop — 中央编排器

`CoreLoop` (`internal/agent/loop.go`, ~1500 行) 是系统核心。

```go
type CoreLoop struct {
    agent          agent.Agent
    runner         runner.Runner
    sessionService session.Service
    memoryService  memory.Service
    factory        *provider.Factory
    cfg            *config.WukongConfig
    contextMgr     *ContextManager
    security       *security.Guard
    recallStore    *recall.Store
    cortexStore    *cortex.CortexStore
    memoryFlow     *cortex.MemoryFlowService
    graphFlow      *cortex.GraphFlowService
    closeFn        func() error

    mu     sync.RWMutex
    closed bool
    bgWg   sync.WaitGroup
}
```

### 3.2 执行循环

```
Run(userID, sessionID, message)
│
├── Phase 1: 对话前准备
│   ├── PrepareContext() → 上下文压缩
│   ├── recallStore.StoreMessage(user) → FTS5+HNSW 索引
│   ├── cortexStore.StoreMessage(user) → HNSW 向量同步
│   ├── memoryFlow.IngestTurn(user) → CortexDB 转录
│   ├── memoryFlow.WakeUp() → 向量+FTS5 唤醒上下文注入
│   └── memoryService.ReadMemories() → 持久记忆注入 [去重]
│
├── Phase 2: Agent 执行
│   └── runner.Run()
│       ├── LLM 推理 (26B 主模型)
│       ├── Tool Calls → 安全管线检查 → 执行
│       ├── AutoExtract (9B 异步记忆提取)
│       └── SummaryJob (9B 异步上下文摘要)
│
├── Phase 3: 对话后收尾
│   ├── recallStore.StoreMessage(assistant) → FTS5+HNSW
│   ├── cortexStore.StoreMessage(assistant) → HNSW
│   ├── recallStore.StoreMessage(tool_calls) → FTS5+HNSW
│   ├── recallStore.StoreMessage(tool_responses) → FTS5+HNSW
│   ├── cortexStore.StoreMessage(tool_*) → HNSW
│   ├── memoryFlow.IngestTurn(assistant) → CortexDB 转录
│   ├── memoryFlow.PromoteFacts() → [GetTranscript → Extract → AddMemory]
│   ├── graphFlow auto-extract → [BuildTranscript → ExtractFromTranscript → BuildGraph]
│   └── contextMgr.AfterRun() → token 更新
│
└── Phase 4: 返回响应
```

### 3.3 关闭序列

```
Close()
├── bgWg.Wait()              ← 等待后台 goroutine (PromoteFacts/GraphFlow)
├── runner.Close()           ← 停止 Agent Runner
├── evolution.Close()        ← 停止进化引擎 worker
├── memory.Close()           ← 停止 AutoExtract worker (5s timeout)
├── session.Close()          ← 关闭会话存储
├── graphFlow.Close()        ← 关闭知识图谱引擎
├── telemetry.Shutdown(10s)  ← 刷新 OTLP + Langfuse
└── dbPool.Close()           ← 关闭数据库 (WAL checkpoint)
```

---

## 4. 记忆系统 —— 双引擎架构

### 4.1 架构总览

```
tRPC Memory (引擎一)              CortexDB Stack (引擎二)
─────────────────────              ─────────────────────
存储: wukong.db                    存储: wukong.db (同库，共享 CortexDB 实例)
模型: 键值 (key-value)             模型: 转录 + 向量 + 图谱

📝 AutoExtract (异步)              📋 MemoryFlow
   9B 轻量模型                        IngestTurn (对话转录记录)
   每轮后自动运行                    WakeUp (向量+全文语义唤醒)
                                     PromoteFacts (桥接 tRPC Memory)

🔧 6 个工具                        🔗 GraphFlow
   add/search/update/delete           实体提取 → RDF 知识图谱
   load/clear                         SPARQL 查询
                                      BuildContext (KG 增强上下文)
SmartCleanup (80%→60% 淘汰)
                                     📥 ImportFlow
                                      DDL → KG 映射
                                      CSV 数据导入

📂 Recall / CortexStore
   FTS5 全文搜索 + HNSW 向量搜索
   recall_search (含 tool_call/response)
   recall_sessions (会话列表)
```

### 4.2 记忆闭环

```
Before Run (上下文注入)                              After Run (记录与提取)
┌─────────────────────────────────┐     ┌───────────────────────────────────┐
│ 1. WakeUp()                     │     │ 1. StoreMessage(user/assistant/   │
│    → CortexDB 向量+FTS5 召回    │     │    tool_call/tool_response)       │
│    历史对话上下文                │     │    → FTS5 + HNSW 索引            │
│                                 │     │                                   │
│ 2. ReadMemories()               │     │ 2. IngestTurn(assistant)         │
│    → tRPC SQLite 持久记忆       │     │    → CortexDB Episode 存储       │
│    → 与 WakeUp 去重             │     │                                   │
│    [isMemoryDuplicated]         │     │ 3. PromoteFacts()                 │
│                                 │     │    → GetTranscript() 完整对话     │
│                                 │     │    → LLM/Heuristic 事实提取       │
│                                 │     │    → AddMemory() tRPC 持久化      │
│                                 │     │                                   │
│                                 │     │ 4. GraphFlow auto-extract        │
│                                 │     │    → ExtractFromTranscript()     │
│                                 │     │    → BuildGraph() RDF 图谱       │
└─────────────────────────────────┘     └───────────────────────────────────┘

Recall Search (跨系统搜索):
  recall_search → CortexStore.SearchWithMemory()
    ├── CortexDB HNSW 向量搜索 (对话历史)
    └── tRPC SearchMemories() (持久记忆)
    → 合并返回
```

### 4.3 PromoteFacts 数据流（关键路径）

```
对话后触发:
  MemoryFlowService.PromoteFacts(sessionID, userID)
    │
    ├── flow.GetTranscript()          ← 从 CortexDB Episode 加载完整对话轮次
    │     └── 返回 Transcript{Turns: [user, assistant, user, ...]}
    │
    ├── extractor.Extract(transcript) ← LLM/Heuristic 提取事实
    │     ├── LLM Extract: 结构化 Prompt → JSON parsing
    │     └── Heuristic: 中英文关键词匹配 (偏好/决策/笔记)
    │
    └── AddMemory()                   ← 写入 tRPC SQLite 持久化
          └── topics: [kind, collection]
```

---

## 5. 多 Agent 编排

### 5.1 10 种编排模式

| 模式 | 拓扑 | 适用场景 |
|------|------|----------|
| `single` | 单 Agent | 简单对话（默认） |
| `chain` | 顺序管道（planner→executor→reviewer） | 多步任务 |
| `parallel` | 并发执行 | 多角度分析 |
| `cycle` | 迭代循环（planner↔executor / generator↔reviewer） | 自我改进 |
| `graph` | 条件路由（StateGraph + HITL） | 复杂决策 |
| `team_coordinator` | 中央协调（成员作为 AgentTool） | 任务分派 |
| `team_swarm` | 蜂群自主（transfer_to_agent） | 自主协作 |
| `claude_code` | 委托 Claude Code CLI | 外部编码代理 |
| `codex` | 委托 Codex CLI (workspace-write 沙箱) | 外部编码代理 |
| `dify` | Dify AI 平台可视化工作流 | 低代码编排 |

### 5.2 HITL 人机回环

Graph 模式支持中断/恢复：`InterruptBefore("dangerous_op")` → 暂停 → 人类审批 → `ResumeInterrupted(checkpoint)`。

---

## 6. 扩展与工具系统

### 6.1 12 个内置扩展

| 扩展 | 工具数 | 安全措施 |
|------|--------|----------|
| **developer** | 6 | Guard + .wukongignore + sandbox OS 隔离 |
| **computer_controller** | 9 | 权限级别检查 |
| **memory** | 6 | tRPC Service 注入 |
| **cortex** | 5 | recall_search/search+sessions + KG query/analyze |
| **auto_visualiser** | 3 | 输出目录限制 |
| **tutorial** | 3 | 无风险 |
| **top_of_mind** | 4 | Guard 检查 |
| **code_mode** | 2 | goja 三级沙箱防护 |
| **apps** | 5 | Guard 检查 |
| **web** | 1 | HTTP 超时 |
| **agent_tools** | 3 | 子 Agent 隔离 |
| **mcp_broker** | 4 | 转发管控 |

### 6.2 MCP 外部扩展

支持 stdio / SSE / streamable 三种传输方式，通过 Deeplink URL 或 YAML 配置注册。

---

## 7. 安全纵深防御

Wukong 实现了 5 层递进的安全防御：

```
Layer 5 ─ Guard 权限控制
  4 种模式: auto / smart / manual / chat_only
  allowlist / denylist (支持通配符)
  blocked_commands 模式匹配
  prompt injection 审查 (独立轻量 Runner)

Layer 4 ─ goja JS 沙箱 (code_execute / code_discover_tools)
  API 白名单: console / JSON / Math / __output
  禁用: eval / Function / setInterval / Date / RegExp
  运行时: debug.SetMemoryLimit(128MB) + context.Timeout(10s)
  并发: semaphore (max 5) + goroutine 泄漏防护
  JSON.parse: 1MB 输入限制

Layer 3 ─ sandbox OS 级隔离 (command_execute / code_search)
  Linux:    Landlock LSM (内核 5.13+)，self-exec 模式
  macOS:    sandbox-exec + Seatbelt 动态 profile
  Windows:  Low Integrity Level + Mandatory Labels
  保护: 仅工作目录 + .wukong 可写，其余只读
  启动报告: sandbox.Probe() → INFO/WARN 日志

Layer 2 ─ .wukongignore 文件黑名单
  gitignore 兼容语法，支持 negate 规则 (!)
  file_read / write / replace / delete 路径验证
  搜索优先级: CWD > HOME > CWD/.wukong/

Layer 1 ─ OS 进程权限
  非 root 用户运行
  ulimit 资源限制
```

---

## 8. 上下文管理

`ContextRevisionEngine` 三层压缩：

| 层级 | 策略 | 触发条件 |
|------|------|----------|
| 1 | LLM 智能摘要 (9B gemma-4-e4b-it) | token 阈值 / 消息数>100 / 时间>5min |
| 2 | 渐进式压缩 (合并现有摘要) | cooldown 120s |
| 3 | 算法截断 (首尾保留) | LLM 不可用时回退 |

tRPC 框架级双通道压缩：Pass1 旧结果→占位符，Pass2 剩余→首尾截断。

---

## 9. LLM Provider 体系

| Provider | 类型 | 典型模型 |
|----------|------|----------|
| OpenAI | openai | GPT-4o |
| Anthropic | anthropic | Claude Sonnet 4 |
| Google | google | Gemini |
| DeepSeek | deepseek | DeepSeek-Chat |
| Ollama | ollama | Llama3 |
| LMStudio | lmstudio | Gemma-4-26b |
| ACP | acp | 远程代理 |

模型分工策略：主对话 26B / 记忆提取 9B / 上下文摘要 4B。

---

## 10. 服务与协议层

| 协议 | 端口 | 用途 | 关闭方式 |
|------|------|------|----------|
| A2A | :9090 | Agent-to-Agent 通信 | Stop(ctx) |
| AG-UI SSE | :8080 | Web UI 实时对话 | Stop(ctx) (优雅关闭) |
| ACP | :9091 | Agent Client Protocol | Stop(ctx) (优雅关闭) |
| ACP MCP Bridge | :3400 | 跨协议工具共享 | Stop() |

所有 HTTP 端点均使用 `*http.Server` + `Shutdown(ctx)` 实现优雅关闭，`json.NewDecoder` 均配置 10MB `io.LimitReader`。

---

## 11. 存储架构

- **单文件 wukong.db**：SQLite WAL 模式，共享 DatabasePool，`MaxOpenConns=4`，`_busy_timeout=5000ms`
- **关闭保证**：`PRAGMA wal_checkpoint(TRUNCATE)` 确保 WAL 刷新
- **跨系统共享**：Session / Memory / Recall / Todo / Cortex 全库合一
- **CortexDB 实例共享**：CortexStore ↔ MemoryFlow 共享同一 `*cortexdb.DB` 实例，避免双连接冲突

---

## 12. 技能自进化系统

`EvolutionEngine` 闭环：

```
执行追踪 → 异步分析队列 → LLM 分析 → 补丁生成 → 版本管理 → 热重载
```

约束：置信度 ≥0.7 / 冷却 ≥30min / 每日上限 10 / 补丁 ≤8KB / 保留 10 个历史版本。

---

## 13. 完整数据流

### 对话处理流

```
用户输入 → CoreLoop.Run()
  │
  ├── Phase 1: 对话前
  │   ├── recallStore.StoreMessage(user)   → [FTS5 + HNSW]
  │   ├── cortexStore.StoreMessage(user)   → [HNSW]
  │   ├── memoryFlow.IngestTurn(user)      → [CortexDB Episode]
  │   ├── memoryFlow.WakeUp()              → [唤醒上下文注入]
  │   └── memoryService.ReadMemories()     → [持久记忆注入 + 去重]
  │
  ├── Phase 2: 执行
  │   └── runner.Run() → LLM + Tools + AutoExtract + SummaryJob
  │
  ├── Phase 3: 对话后
  │   ├── StoreMessage(assistant)          → [FTS5 + HNSW]
  │   ├── StoreMessage(tool_calls)         → [FTS5 + HNSW]
  │   ├── StoreMessage(tool_responses)     → [FTS5 + HNSW]
  │   ├── memoryFlow.IngestTurn(assistant) → [CortexDB Episode]
  │   ├── PromoteFacts()                   → [GetTranscript → Extract → AddMemory]
  │   │     └── 写入 tRPC SQLite 持久化
  │   └── GraphFlow auto-extract          → [BuildTranscript → Extract → BuildGraph]
  │         └── RDF 知识图谱更新
  │
  └── Phase 4: 返回
      └── contextMgr.AfterRun()
```

### 关闭流

```
信号 → bgWg.Wait()
     → runner.Close()
     → evolution.Close()
     → memory.Close()
     → session.Close()
     → graphFlow.Close()
     → telemetry.Shutdown(10s)
     → dbPool.Close() → WAL checkpoint
```

---

## 14. 技术选型

| 类别 | 选择 | 理由 |
|------|------|------|
| Agent 框架 | tRPC-Agent-Go v1.10.0 | 多 Agent 编排、完整的 Session/Memory/Planner 抽象 |
| MCP 协议 | tRPC-MCP-Go v0.0.16 | stdio/sse/streamable 三传输 |
| A2A 协议 | tRPC-A2A-Go v0.2.5 | Agent 间标准通信 |
| 智能记忆 | CortexDB v2.25.0 | HNSW + FTS5 + RDF/SPARQL |
| JS 引擎 | goja (纯 Go) | 零 CGO、跨平台、沙箱友好 |
| OS 沙箱 | pkg/sandbox (自维护) | Landlock/Seatbelt/Low IL，零外部依赖 |
| 数据库 | SQLite WAL | 单文件部署、FTS5 全文搜索 |
| 前端 | BubbleTea + LipGloss | 终端 TUI，纯 Go |
| 浏览器 | Chromedp | Chrome DevTools 协议 |
| 配置 | Viper + Cobra | CLI > ENV > YAML 多级覆盖 |
| 可观测 | OpenTelemetry + Langfuse | 全链路追踪 + LLM 分析 |

---

## 15. 关键设计决策 (ADR)

| ADR | 决策 | 理由 |
|-----|------|------|
| **ADR-1** | SQLite WAL 共享池，MaxOpenConns=4 | 零配置+并发+FTS5，busy_timeout 处理竞争 |
| **ADR-2** | 双引擎记忆 + 完整闭环 | tRPC Memory (KV) + CortexDB Stack (转录/向量/图谱) |
| **ADR-3** | 辅助模型摘要 | 独立轻量模型节省主模型 token |
| **ADR-4** | CortexDB 实例共享 | CortexStore ↔ MemoryFlow 共用 `*cortexdb.DB` |
| **ADR-5** | MemoryFlow → tRPC Bridge | PromoteFacts 提取事实后写入持久化存储 |
| **ADR-6** | MCP Broker 模式 | 工具不膨胀（50+ → 4 入口） |
| **ADR-7** | 冷启动友好 | 无 embedding 回退 FTS5，无 LLM 回退启发式 |
| **ADR-8** | 单文件数据库 | 跨系统查询、部署简单 |
| **ADR-9** | SmartCleanup 容量淘汰 | 80% 阈值→智能评分→淘汰至 60% |
| **ADR-10** | 记忆去重 | WakeUp 上下文与 ReadMemories 滑动窗口去重 |
| **ADR-11** | Tool 消息完整记录 | user/assistant/tool_call/tool_response 全部索引 |
| **ADR-12** | GraphFlow auto_extract | 每轮对话后自动提取实体/关系 |
| **ADR-13** | Extractor 回退链 | 专用模型 → 默认模型 → 禁用/启发式 |
| **ADR-14** | 非阻塞 Evolution | 后台 goroutine，不影响主循环 |
| **ADR-15** | CoreLoop 依赖注入 | 27 个依赖解耦，可测试可替换 |
| **ADR-16** | 多协议服务器 | A2A + ACP + AG-UI 三端口 |
| **ADR-17** | 服务器优雅关闭 | `*http.Server` + `Shutdown(ctx)` |
| **ADR-18** | HTTP body 大小限制 | `io.LimitReader(10MB)` 防 DoS |
| **ADR-19** | JS 沙箱多层防护 | goja 白名单 + 内存限制 + 并发控制 + ReDoS 防护 |
| **ADR-20** | context 超时覆盖 | 所有 `context.Background()` 加超时 |
| **ADR-21** | Goroutine 生命周期 | CoreLoop.bgWg 追踪，关闭前 Wait |
| **ADR-22** | OS 级文件写入沙箱 | Landlock/Seatbelt/Low IL，内核强制写保护 |
| **ADR-23** | sandbox 包置于 pkg/ | 可被外部项目导入复用，独立测试验证 |
