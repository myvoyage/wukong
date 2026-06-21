# Wukong 系统架构深度分析

> **版本**: v0.6.0 | **Go**: 1.26 | **总源文件**: 119 `.go` + 34 `_test.go` | **直接依赖**: 30
> **外发包**: `pkg/sandbox/` (10 文件)
>
> 基于 [tRPC-Agent-Go v1.10.0](https://github.com/trpc-group/trpc-agent-go) · [tRPC-MCP-Go v0.0.16](https://github.com/trpc-group/trpc-mcp-go) · [CortexDB v2.25.0](https://github.com/liliang-cn/cortexdb)

---

## 目录

1. [架构哲学](#1-架构哲学)
2. [系统全景](#2-系统全景)
3. [安全纵深模型](#3-安全纵深模型)
4. [Agent 执行引擎](#4-agent-执行引擎)
5. [记忆系统](#5-记忆系统)
6. [多 Agent 编排](#6-多-agent-编排)
7. [扩展与工具系统](#7-扩展与工具系统)
8. [安全架构](#8-安全架构)
9. [上下文管理](#9-上下文管理)
10. [LLM Provider 体系](#10-llm-provider-体系)
11. [服务与协议层](#11-服务与协议层)
12. [存储架构](#12-存储架构)
13. [技能自进化系统](#13-技能自进化系统)
14. [数据流](#14-数据流)
15. [技术选型](#15-技术选型)
16. [关键设计决策 (ADR)](#16-关键设计决策-adr)

---

## 1. 架构哲学

Wukong 的五条核心哲学，每条都有具体的工程体现：

| 哲学 | 核心信念 | 关键工程决策 |
|------|----------|-------------|
| **记忆优先** | Agent 智能来源于知识积累 | 双引擎记忆 + 知识图谱 + HNSW，完整闭环 |
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

Capability Layer:
  Security Guard —— Extension Manager —— Evolution Engine
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

## 3. 安全纵深模型

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
  复刻自 tirdyhouse/sandbox (MIT)，增强集成
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

## 4. Agent 执行引擎

### 4.1 CoreLoop — 中央编排器

`CoreLoop` (`internal/agent/loop.go`, 1300+ 行) 是系统核心。

```go
type CoreLoop struct {
    agent          agent.Agent
    runner         runner.Runner
    sessionService session.Service
    memoryService  memory.Service
    factory        *provider.Factory
    contextMgr     *ContextManager
    security       *security.Guard
    recallStore    *recall.Store
    memoryFlow     *cortex.MemoryFlowService
    closeFn        func() error
    mu             sync.RWMutex
    closed         bool
    bgWg           sync.WaitGroup  // 后台 goroutine 生命周期追踪
}
```

### 4.2 执行循环

```
Run(userID, sessionID, message)
│
├── Phase 1: 对话前准备
│   ├── PrepareContext() → 上下文压缩检查
│   ├── recallStore.StoreMessage() → 回溯索引
│   ├── memoryFlow.IngestTurn(user) → 转录记录
│   ├── memoryFlow.WakeUp() → 向量+FTS5 唤醒上下文注入
│   └── memoryService.ReadMemories() → 持久记忆注入
│
├── Phase 2: Agent 执行
│   └── runner.Run()
│       ├── LLM 推理 (26B 主模型)
│       ├── Tool Calls → 安全管线检查 → 执行
│       ├── AutoExtract (9B 异步记忆提取)
│       └── SummaryJob (9B 异步上下文摘要)
│
├── Phase 3: 对话后收尾
│   ├── memoryFlow.IngestTurn(asst) → 转录记录
│   ├── memoryFlow.PromoteFacts() → 事实提升 (bgWg 追踪)
│   └── contextMgr.AfterRun() → token 更新
│
└── Phase 4: 返回响应
```

### 4.3 关闭序列

```
Close()
├── bgWg.Wait()              ← 等待 PromoteFacts 等后台 goroutine
├── runner.Close()           ← 停止 Agent Runner
├── evolution.Close()        ← 停止进化引擎 worker
├── memory.Close()           ← 停止 AutoExtract worker
├── session.Close()          ← 关闭会话存储
├── telemetry.Shutdown(10s)  ← 刷新 OTLP + Langfuse
└── dbPool.Close()           ← 关闭数据库 (WAL checkpoint)
```

---

## 5. 记忆系统

### 5.1 双引擎架构

```
tRPC Memory (引擎一)              CortexDB Stack (引擎二)
─────────────────────              ─────────────────────
存储: wukong.db                    存储: wukong.db (同库)
模型: 键值 (key-value)             模型: 转录 + 向量 + 图谱

📝 AutoExtract (异步)              📋 MemoryFlow
   9B qwen3.5-9b                     IngestTurn (记录)
   每轮后自动运行                    WakeUp (语义唤醒)
                                    PromoteFacts (桥接)

🔧 6 个工具                        🔗 GraphFlow
   add/search/update/delete          实体 → RDF 知识图谱
   load/clear                        SPARQL 查询

                                    📥 ImportFlow
                                     DDL → KG 映射
                                     CSV 数据导入
```

### 5.2 记忆闭环

```
Before Run                    After Run
┌─────────────┐               ┌──────────────┐
│ WakeUp()    │← 上下文注入    │ IngestTurn() │→ 转录记录
│ ReadMem()   │               │ PromoteFacts │→ 桥接 tRPC
└─────────────┘               └──────────────┘
```

---

## 6. 多 Agent 编排

### 6.1 10 种编排模式

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

### 6.2 HITL 人机回环

Graph 模式支持中断/恢复：`InterruptBefore("dangerous_op")` → 暂停 → 人类审批 → `ResumeInterrupted(checkpoint)`。

---

## 7. 扩展与工具系统

### 7.1 12 个内置扩展

| 扩展 | 工具数 | 安全措施 |
|------|--------|----------|
| **developer** | 6 | Guard 检查 + .wukongignore + sandbox OS 隔离 |
| **computer_controller** | 9 | 权限级别检查 |
| **memory** | 6 | 数据隔离 |
| **auto_visualiser** | 3 | 输出目录限制 |
| **tutorial** | 3 | 无风险 |
| **top_of_mind** | 4 | Guard 检查 |
| **code_mode** | 2 | goja 三级沙箱防护 |
| **apps** | 5 | Guard 检查 |
| **web** | 1 | HTTP 超时 |
| **agent_tools** | 3 | 子 Agent 隔离 |
| **cortex** | 4 | 只读操作 |
| **mcp_broker** | 4 | 转发管控 |

### 7.2 MCP 外部扩展

支持 stdio / SSE / streamable 三种传输方式，通过 Deeplink URL 或 YAML 配置注册。

---

## 8. 安全架构

### 8.1 goja JS 沙箱（Layer 4）

`internal/codemode/` 提供三级递进防护：

| 层级 | 机制 |
|------|------|
| **系统隔离** | 纯 Go 引擎 (无 native code)，无文件/网络/进程 API，每次新 VM |
| **运行时控制** | context.Timeout(10s) + vm.Interrupt()，并发 semaphore(5)，debug.SetMemoryLimit(128MB) |
| **API 白名单** | ✓ console/JSON/Math/__output，✗ eval/Function/setInterval/Date/RegExp |

安全决策：RegExp 禁用 (防 ReDoS)，JSON.parse 1MB 限制，Math.random 确定性返回 0.5，code_execute 在 smart 模式需用户批准。

### 8.2 OS 级文件沙箱（Layer 3）

`pkg/sandbox/` 复刻自 tirdyhouse/sandbox (MIT)，增强集成：

| 平台 | 机制 | 特点 |
|------|------|------|
| Linux | Landlock (5.13+) | self-exec 模式，full FS 只读 |
| macOS | sandbox-exec | 动态 Seatbelt profile，执行后清理 |
| Windows | Low IL | Low Integrity Token + Mandatory Labels |

优化：`os.Executable()` 缓存、空 writableDirs 跳过 marshal、`panic`→`sync.Once`、`CommandContext` 支持、`Start()` 失败 cleanup 保护。

集成位置：`developer.go` 的 `command_execute` 和 `code_search` 工具。启动时 `sandbox.Probe()` 报告能力状态。

---

## 9. 上下文管理

`ContextRevisionEngine` 三层压缩：

| 层级 | 策略 | 触发条件 |
|------|------|----------|
| 1 | LLM 智能摘要 (9B gemma-4-e4b-it) | token 阈值 / 消息数>100 / 时间>5min |
| 2 | 渐进式压缩 (合并现有摘要) | cooldown 120s |
| 3 | 算法截断 (首尾保留) | LLM 不可用时回退 |

tRPC 框架级双通道压缩：Pass1 旧结果→占位符，Pass2 剩余→首尾截断。

---

## 10. LLM Provider 体系

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

## 11. 服务与协议层

| 协议 | 端口 | 用途 | 关闭方式 |
|------|------|------|----------|
| A2A | :9090 | Agent-to-Agent 通信 | Stop(ctx) |
| AG-UI SSE | :8080 | Web UI 实时对话 | Stop(ctx) (优雅关闭) |
| ACP | :9091 | Agent Client Protocol | Stop(ctx) (优雅关闭) |
| ACP MCP Bridge | :3400 | 跨协议工具共享 | Stop() |

所有 HTTP 端点均使用 `*http.Server` + `Shutdown(ctx)` 实现优雅关闭，`json.NewDecoder` 均配置 10MB `io.LimitReader`。

---

## 12. 存储架构

- **单文件 wukong.db**：SQLite WAL 模式，共享 DatabasePool，`MaxOpenConns=4`，`_busy_timeout=5000ms`
- **关闭保证**：`PRAGMA wal_checkpoint(TRUNCATE)` 确保 WAL 刷新
- **跨系统共享**：Session / Memory / Recall / Todo / Cortex 全库合一

---

## 13. 技能自进化系统

`EvolutionEngine` 闭环：

```
执行追踪 → 异步分析队列 → LLM 分析 → 补丁生成 → 版本管理 → 热重载
```

约束：置信度 ≥0.7 / 冷却 ≥30min / 每日上限 10 / 补丁 ≤8KB / 保留 10 个历史版本。

---

## 14. 数据流

### 对话处理流

```
用户输入 → CoreLoop.Run()
  ├── Memory: WakeUp + ReadMemories → message 前缀
  ├── Agent: runner.Run() → LLM + Tools + AutoExtract + SummaryJob
  └── Memory: IngestTurn + PromoteFacts (bgWg) + AfterRun
```

### 关闭流

```
信号 → bgWg.Wait() → runner.Close() → evolution.Close()
     → memory.Close() → session.Close() → telemetry.Shutdown(10s)
     → dbPool.Close() → WAL checkpoint
```

---

## 15. 技术选型

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

## 16. 关键设计决策 (ADR)

| ADR | 决策 | 理由 |
|-----|------|------|
| **ADR-1** | SQLite WAL 共享池，MaxOpenConns=4 | 零配置+并发+FTS5，busy_timeout 处理竞争 |
| **ADR-2** | 双引擎记忆 | tRPC Memory + CortexDB MemoryFlow 互补 |
| **ADR-3** | 辅助模型摘要 | 独立轻量模型节省主模型 token |
| **ADR-4** | CortexDB HNSW 索引 | O(log N) 向量搜索 |
| **ADR-5** | MCP Broker 模式 | 工具不膨胀（50+ → 4 入口） |
| **ADR-6** | 冷启动友好 | 无 embedding 回退 FTS5，无 LLM 回退启发式 |
| **ADR-7** | 单文件数据库 | 跨系统查询、部署简单 |
| **ADR-8** | 记忆 TTL 自动清理 | 30 天旧记忆清除 |
| **ADR-9** | Extractor 回退链 | 专用模型 → 默认模型 → 禁用 |
| **ADR-10** | 非阻塞 Evolution | 后台 goroutine，不影响主循环 |
| **ADR-11** | CoreLoop 依赖注入 | 27 个依赖解耦，可测试可替换 |
| **ADR-12** | 多协议服务器 | A2A + ACP + AG-UI 三端口 |
| **ADR-13** | 服务器优雅关闭 | `*http.Server` + `Shutdown(ctx)` |
| **ADR-14** | HTTP body 大小限制 | `io.LimitReader(10MB)` 防 DoS |
| **ADR-15** | JS 沙箱多层防护 | goja 白名单 + 内存限制 + 并发控制 + ReDoS 防护 |
| **ADR-16** | context 超时覆盖 | 所有 `context.Background()` 加超时 |
| **ADR-17** | Goroutine 生命周期 | CoreLoop.bgWg 追踪，关闭前 Wait |
| **ADR-18** | OS 级文件写入沙箱 | Landlock/Seatbelt/Low IL，内核强制写保护 |
| **ADR-19** | sandbox 包置于 pkg/ | 可被外部项目导入复用，独立测试验证 |
