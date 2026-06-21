# Wukong — 记忆优先、编排驱动、安全纵深的 AI Agent 平台

> **版本**: v0.6.1 | **Go**: 1.26 | **源文件**: 119 `.go` + 34 `_test.go` | **直接依赖**: 30 | **许可证**: GNU AGPL-3.0
>
> 基于 [tRPC-Agent-Go v1.10.0](https://github.com/trpc-group/trpc-agent-go) · [tRPC-MCP-Go v0.0.16](https://github.com/trpc-group/trpc-mcp-go) · [tRPC-A2A-Go v0.2.5](https://github.com/trpc-group/trpc-a2a-go) · [CortexDB v2.25.0](https://github.com/liliang-cn/cortexdb)

---

## 目录

1. [架构哲学](#1-架构哲学)
2. [系统架构全景](#2-系统架构全景)
3. [核心特性矩阵](#3-核心特性矩阵)
4. [安全纵深模型](#4-安全纵深模型)
5. [双引擎记忆系统](#5-双引擎记忆系统)
6. [多 Agent 编排](#6-多-agent-编排)
7. [扩展与工具生态](#7-扩展与工具生态)
8. [服务与协议](#8-服务与协议)
9. [快速开始](#9-快速开始)
10. [项目结构](#10-项目结构)
11. [文档索引](#11-文档索引)

---

## 1. 架构哲学

Wukong 的五大核心哲学决定了其所有工程决策：

| 哲学 | 核心信念 | 关键实现 |
|------|----------|----------|
| **记忆优先** | Agent 智能来源于跨会话知识积累 | 双引擎记忆闭环：tRPC Memory + CortexDB Stack |
| **框架组装** | 任何组件都应可替换 | CoreLoop 依赖注入，27 个子系统通过配置解耦 |
| **多 Agent 原生** | 编排是一等公民，非事后插件 | 10 种显式编排模式 + HITL 人机协同 |
| **进化智能** | 技能应从失败中学习改进 | LLM 分析 → 自动补丁 → 版本管理 → 热重载 |
| **纵深防御** | 安全是多层协同，非单点检查 | 5 层防御从 LLM 权限到 OS 内核全覆盖 |

### 1.1 记忆优先 (Memory-First)

> Agent 的真正智能来源于跨会话的知识积累，而非单次对话的上下文窗口。

Wukong 的记忆系统由两个互补引擎构成：

- **tRPC Memory（引擎一）**: SQLite 键值持久化存储，提供 `add/search/update/delete/load/clear` 六大操作。核心机制包括 `AutoExtract`（异步 LLM 记忆提取，9B 轻量模型）、`SmartCleanup`（容量感知评分淘汰：80% → 60%）、`MemoryBridge`（接收 MemoryFlow 的事实写入）。

- **CortexDB Stack（引擎二）**: 基于 HNSW 向量索引 + FTS5 全文搜索 + RDF 知识图谱的三合一智能记忆栈。核心流程包括 `MemoryFlow`（转录记录 → 向量语义唤醒 → 事实桥接 tRPC Memory）、`GraphFlow`（每轮对话自动实体/关系提取 → RDF 知识图谱构建 → SPARQL 查询）、`ImportFlow`（DDL → KG 映射 + CSV 批量导入）。

### 1.2 框架组装 (Framework-Assembled)

> 框架是能力的来源，不是限制的边界。

- `CoreLoop` 通过配置结构体注入 27 个子系统，每个子系统有清晰接口边界
- `Security Guard`、`ContextRevisionEngine`、`Extension Manager` 各自独立
- 7 种 LLM Provider 通过统一 `provider.Factory` 接入
- Session 后端支持 SQLite / Redis / InMemory 三种实现

### 1.3 多 Agent 原生 (Multi-Agent by Default)

> 复杂任务不应由单个 Agent 完成。多 Agent 编排是一等公民。

提供 **10 种显式编排模式**：

| 模式 | 拓扑 | 说明 |
|------|------|------|
| `single` | 单 Agent | 简单对话（默认） |
| `chain` | 顺序管道 | planner → executor → reviewer 线性流水线 |
| `parallel` | 并发执行 | 多 Agent 并行处理、结果汇聚 |
| `cycle` | 迭代循环 | planner ↔ executor / generator ↔ reviewer 自改进 |
| `graph` | 条件路由 | StateGraph + 条件边 + HITL 中断/恢复 |
| `team_coordinator` | 中央协调 | Leader 将成员包装为 AgentTool 进行任务分派 |
| `team_swarm` | 蜂群自主 | 通过 `transfer_to_agent` 自主协作 |
| `claude_code` | 委托 Claude Code | 外部 CLI 编码代理 |
| `codex` | 委托 Codex CLI | workspace-write 沙箱隔离的外部编码代理 |
| `dify` | Dify 平台 | 可视化工作流编排 |

### 1.4 进化智能 (Evolving Intelligence)

> 技能应从失败中学习改进，而非等待人类修复。

`EvolutionEngine` 闭环：执行追踪 → 异步分析队列 → LLM 分析 → 补丁生成 → 版本管理 → 热重载。四重约束：置信度 ≥0.7 / 冷却时间 ≥30min / 每日上限 10 个 / 补丁 ≤8KB / 保留 10 个历史版本。

### 1.5 纵深防御 (Defense in Depth)

> 安全不是单点检查，而是多层协同纵深。

见 [§4 安全纵深模型](#4-安全纵深模型)。

---

## 2. 系统架构全景

```
┌─────────────────────────────────────────────────────────────────────┐
│                       Wukong AI Agent Platform                      │
├─────────────────────────────────────────────────────────────────────┤
│ Entry Points                                                        │
│   CLI (cobra + BubbleTea TUI)    │ API (A2A:9090/ACP:9091/AGUI:8080)│
├─────────────────────────────────────────────────────────────────────┤
│ Core Engine ─── CoreLoop (27 子系统依赖注入协调)                      │
│   ├── WorkflowBuilder (10 种编排模式) + TeamBuilder (多 Agent)        │
│   ├── ContextRevisionEngine (3 层 LLM 压缩)                          │
│   └── Security Guard (5 层纵深防御)                                  │
├─────────────────────────────────────────────────────────────────────┤
│ Agent Runtime ─── tRPC-Agent-Go Runner                              │
│   LLMAgent / ChainAgent / ParallelAgent / CycleAgent / GraphAgent    │
│   Session Service / Memory Service / Artifact Service                │
│   Planner / ToolSearch / ContextCompaction                           │
├─────────────────────────────────────────────────────────────────────┤
│ Memory Stack (双引擎)                                                │
│ ┌─ tRPC Memory (SQLite KV) ───────────────────────────────────┐     │
│ │  AutoExtract → 异步记忆提取 → ReadMemories                  │     │
│ │  6 tools: add/search/update/delete/load/clear                │     │
│ │  SmartCleanup: 容量感知淘汰 (80%→60%)                       │     │
│ ├────────────────────────────────────────────────────────────┤     │
│ │  CortexDB Stack (HNSW + FTS5 + RDF)                         │     │
│ │  ┌── MemoryFlow: IngestTurn → WakeUp → PromoteFacts          │     │
│ │  ├── GraphFlow: 实体提取 → RDF KG (auto_extract)             │     │
│ │  └── ImportFlow: DDL/CSV → KG 导入                           │     │
│ └────────────────────────────────────────────────────────────┘     │
├─────────────────────────────────────────────────────────────────────┤
│ Capability Layer                                                    │
│   Security Guard / Extension Manager / Evolution Engine             │
│   Knowledge (RAG) / Browser (Chromedp) / CodeMode (goja JS)         │
│   Skill System / Summon (A2A) / pkg/sandbox (OS lock)              │
├─────────────────────────────────────────────────────────────────────┤
│ Infrastructure                                                      │
│   Provider Factory (7 LLMs) / Viper Config / DatabasePool (SQLite)  │
│   Telemetry (OTel) / Observability (Langfuse) / Health Check        │
├─────────────────────────────────────────────────────────────────────┤
│ Storage: wukong.db (单文件 SQLite WAL)                              │
│   sessions / memories / todos / recall(FTS5) / cortex(HNSW+RDF)     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 3. 核心特性矩阵

### 3.1 LLM Provider (7 种)

| Provider | 类型 | 典型模型 | 配置要求 |
|----------|------|----------|----------|
| OpenAI | openai | GPT-4o | API Key |
| Anthropic | anthropic | Claude Sonnet 4 | API Key |
| Google | google | Gemini | API Key |
| DeepSeek | deepseek | DeepSeek-Chat | API Key |
| Ollama | ollama | Llama3 | 本地服务 |
| LMStudio | lmstudio | Gemma-4-26b | 本地服务 |
| ACP Agent | acp | 远程代理 | Agent URL |

模型分工策略：主对话使用 26B 模型 / 记忆提取使用 9B 轻量模型 / 上下文摘要使用独立模型。

### 3.2 内置扩展 (12 个，50+ 工具)

| 扩展 | 工具 | 安全措施 |
|------|------|----------|
| **developer** (6) | file_read, file_write, file_replace, command_execute, code_search, list_directory | Guard + .wukongignore + sandbox OS 隔离 |
| **computer_controller** (9) | navigate, extract_text, screenshot, click, fill, mouse_move, keyboard_type, clipboard, scroll | 权限级别检查 |
| **memory** (6) | memory_add, memory_search, memory_update, memory_delete, memory_load, memory_clear | tRPC Service 安全边界 |
| **cortex** (5) | recall_search, recall_sessions, kg_query, kg_analyze, import_data | 底层 CortexDB 安全边界 |
| **auto_visualiser** (3) | generate_chart, generate_diagram, generate_table | 输出目录限制 |
| **tutorial** (3) | tutorial_start, tutorial_step, tutorial_info | 无风险操作 |
| **top_of_mind** (4) | get_instructions, set_instructions, append_instructions, clear_instructions | Guard 检查 |
| **code_mode** (2) | code_execute, code_discover_tools | goja 三级沙箱防护 |
| **apps** (5) | create_app, get_app, update_app, list_apps, delete_app | Guard 检查 |
| **web** (1) | web_search | HTTP 超时控制 |
| **agent_tools** (3) | code_reviewer, summarizer, code_generator | 子 Agent 进程隔离 |
| **mcp_broker** (4) | list_extensions, enable_extension, disable_extension, install_extension | 转发权限管控 |

### 3.3 附加能力

| 能力 | 实现 | 说明 |
|------|------|------|
| **RAG 知识检索** | tRPC-Knowledge | OpenAI 兼容 Embedding + 内存向量存储 + 目录/URL 源 |
| **浏览器自动化** | Chromedp | 双后端 (HTTP/Headless Chrome)，支持导航/截屏/点击/填表 |
| **JS 沙箱** | goja | 纯 Go 引擎，API 白名单 + 内存限制 + 并发控制 |
| **技能系统** | tRPC FSRepository | SKILL.md 驱动的 Agent Skill，支持 Evolution Hook |
| **Dify 集成** | Dify Chat API | 将 Dify 工作流包装为 Agent |
| **可观测性** | OpenTelemetry + Langfuse | gRPC/HTTP/Console 导出器 + LLM 追踪 |
| **制品存储** | InMemory / COS | cos 后端支持腾讯云对象存储 |
| **健康检查** | Health Registry | DB/Model/Extension/A2A 四维检查 + Liveness/Readiness 端点 |

---

## 4. 安全纵深模型

Wukong 实现了 5 层递进安全防御，层层递进、纵深协同：

```
Layer 5 ─ Guard 权限控制 (internal/security/guard.go)
  ├── 4 种模式: auto / smart / manual / chat_only
  ├── allowlist / denylist (支持通配符匹配)
  ├── blocked_commands 模式匹配 (rm -rf /, dd if=, mkfs., > /dev/sda)
  ├── prompt injection 审查 (独立轻量 Runner + 独立 LLM 调用)
  └── 工具执行超时控制 (default: 30s, max: 300s)

Layer 4 ─ goja JS 沙箱 (internal/codemode/executor.go)
  ├── API 白名单: console / JSON / Math / __output
  ├── 完全禁用: eval / Function / setInterval / Date / RegExp
  ├── 运行时限制: debug.SetMemoryLimit(128MB) + context.Timeout(10s)
  ├── 并发控制: semaphore 信号量 (max 5)
  └── JSON.parse: 1MB 输入限制

Layer 3 ─ sandbox OS 级隔离 (pkg/sandbox/)
  ├── Linux:    Landlock LSM (内核 5.13+, self-exec 模式, 全文件系统只读)
  ├── macOS:    sandbox-exec + Seatbelt (动态 profile 生成)
  ├── Windows:  Low Integrity Level + Mandatory Labels
  └── 仅工作目录 + .wukong 可写，其余路径只读

Layer 2 ─ .wukongignore 文件黑名单 (internal/security/ignore.go)
  ├── gitignore 兼容语法 (支持 negate 规则 !)
  ├── file_read / write / replace / delete 路径验证
  └── 搜索优先级: CWD > HOME > CWD/.wukong/

Layer 1 ─ OS 进程权限
  ├── 非 root 用户运行
  └── ulimit 资源限制
```

---

## 5. 双引擎记忆系统

### 5.1 双引擎架构

```
tRPC Memory (引擎一)                CortexDB Stack (引擎二)
─────────────────────                ─────────────────────
存储: wukong.db (SQLite KV)         存储: wukong.db (同库共享实例)
模型: 键值存储                        模型: 转录 + 向量 + 图谱

AutoExtract (异步 9B 模型)           MemoryFlow
  ├── 每轮后自动运行                  ├── IngestTurn (对话转录记录)
  ├── add/search/update/delete       ├── WakeUp (向量+FTS5 语义唤醒)
  ├── load/clear                     └── PromoteFacts (桥接 tRPC Memory)
  └── SmartCleanup (80%→60%)

                                    GraphFlow
Recall / CortexStore                  ├── 实体提取 → RDF 知识图谱
  ├── FTS5 全文搜索                  ├── SPARQL 查询
  ├── HNSW 向量向量                 ├── BuildContext (KG 增强上下文)
  ├── tool_call/response 索引        └── auto_extract (每轮触发)
  └── SearchWithMemory (跨系统)
                                    ImportFlow
                                      ├── DDL → KG 映射
                                      └── CSV 数据导入
```

### 5.2 记忆闭环数据流

```
对话前注入                              对话后记录与提取
┌──────────────────────┐              ┌───────────────────────────┐
│ WakeUp()             │              │ StoreMessage()            │
│  → 向量+FTS5 历史召回│              │  → FTS5+HNSW 索引         │
│ ReadMemories()       │              │    (user/assistant/       │
│  → tRPC 持久记忆注入 │              │     tool_call/response)   │
│  → WakeUp 结果去重   │              │ IngestTurn()              │
│                      │              │  → CortexDB Episode       │
│                      │              │ PromoteFacts()            │
│                      │              │  → GetTranscript()        │
│                      │              │  → Extract (LLM/启发式)   │
│                      │              │  → AddMemory() tRPC       │
│                      │              │ GraphFlow auto-extract    │
│                      │              │  → ExtractFromTranscript()│
│                      │              │  → BuildGraph() RDF       │
└──────────────────────┘              └───────────────────────────┘
```

### 5.3 PromoteFacts 关键路径

```
对话后触发:
  MemoryFlowService.PromoteFacts(sessionID, userID)
    │
    ├── flow.GetTranscript()
    │     └── 从 CortexDB Episode 加载完整对话轮次
    │         Transcript{Turns: [user, assistant, ...]}
    │
    ├── extractor.Extract(transcript)
    │     ├── LLM Extract: 结构化 Prompt → JSON 解析
    │     │     └── 回退链: 专用模型 → 默认模型 → 禁用
    │     └── Heuristic: 中英文关键词匹配 (偏好/决策/笔记)
    │
    └── memoryService.AddMemory()
          └── 写入 tRPC SQLite 持久化
                topics: [kind, collection]
```

### 5.4 跨系统搜索

```
recall_search(query)
  └── CortexStore.SearchWithMemory()
        ├── CortexDB HNSW 向量搜索 (对话历史含工具消息)
        └── tRPC Memory.SearchMemories() (持久记忆)
        → 合并去重返回
```

---

## 6. 多 Agent 编排

### 6.1 工作流模式 (workflow.go)

`WorkflowBuilder` 根据 `config.WorkflowConfig.Mode` 构建不同拓扑的 Agent：

| 模式 | 拓扑结构 | 适用场景 |
|------|----------|----------|
| `single` | 单个 LLMAgent | 简单对话 |
| `chain` | planner → executor → reviewer 顺序执行 | 多步流水线任务 |
| `parallel` | 多 Agent 并发执行，结果聚合 | 多角度分析 |
| `cycle` | planner ↔ executor 或 generator ↔ reviewer | 自我改进循环 |
| `graph` | StateGraph + 条件路由边 + HITL 中断 | 复杂条件决策 |

### 6.2 团队模式 (team.go)

`TeamBuilder` 提供两种多 Agent 协作拓扑：

| 模式 | 协调方式 | 说明 |
|------|----------|------|
| `team_coordinator` | Leader → AgentTool 委托 | Leader 将成员包装为工具，主动调度 |
| `team_swarm` | transfer_to_agent | 蜂群自主转移，依赖 Agent 自身判断 |

### 6.3 HITL 人机协同 (hitl.go)

Graph 模式支持中断/恢复机制：
- `InterruptBefore("dangerous_op")` → 暂停工作流执行
- 等待人工审批 → `ResumeInterrupted(checkpointID)` → 从检查点恢复

### 6.4 配方系统 (recipe.go)

基于 YAML 的结构化子 Agent 定义：
- 存储位置：`.wukong/recipes/*.yaml`
- 加载流程：`RecipeManager.LoadRecipes(dir)` 扫描 → 解析 YAML → 验证
- 定义了子 Agent 的名称、描述、系统提示、工具集、模型等参数

---

## 7. 扩展与工具生态

### 7.1 MCP 扩展管理 (extension/manager.go)

`Manager` 负责 MCP 扩展的完整生命周期：

- **注册机制**: Deeplink URL (`wukong://extension?...`) 或 YAML 配置文件
- **传输方式**: stdio / SSE / streamable 三种 MCP 传输协议
- **工具权限**: 细粒度 `ToolPermission` 控制（允许/拒绝特定工具）
- **动态管理**: 运行时启用/禁用扩展，无需重启
- **MCP Broker**: 将多个外部 MCP Server 的工具聚合为统一入口（4 个工具入口，避免工具爆炸）

### 7.2 ACP MCP 桥接 (extension/acp_mcp.go)

`ACPMCPBridge` 将 Wukong 的内置扩展暴露为 MCP Server：
- 供 ACP 兼容的编码代理发现和调用
- 自动将 Wukong Extension 的工具转换为 MCP Tool
- 监听 `:3400/mcp`，通过 Streamable HTTP 传输

### 7.3 内置扩展详情

#### developer (6 工具)
核心开发工具集，所有文件操作经过 sandbox OS 级隔离：
- `file_read` / `file_write` / `file_replace` → `.wukongignore` 路径验证
- `command_execute` → Landlock / Seatbelt / Low IL 沙箱保护
- `code_search` → ripgrep 文本搜索
- `list_directory` → 目录浏览

#### computer_controller (9 工具)
基于 Chromedp 的浏览器自动化：
- `navigate` / `extract_text` / `screenshot` → 页面操作
- `click` / `fill_form` → 交互操作
- `mouse_move` / `keyboard_type` / `clipboard` / `scroll` → 输入控制

#### auto_visualiser (3 工具)
自动生成可视化内容：
- `generate_chart` → SVG 图表（柱状图、折线图、饼图等）
- `generate_diagram` → Mermaid 流程图
- `generate_table` → HTML 表格

---

## 8. 服务与协议

### 8.1 四协议服务

| 协议 | 端口 | 用途 | 实现 |
|------|------|------|------|
| A2A (:9090) | Agent-to-Agent | 跨 Agent 标准通信协议 | internal/summon/a2a.go |
| ACP (:9091) | Agent Client Protocol | 客户端-Agent 通信标准 | internal/server/acp.go |
| AG-UI SSE (:8080) | Web UI 实时对话 | SSE 流式事件推送 | internal/server/agui.go |
| ACP MCP (:3400) | 跨协议工具桥接 | MCP 协议的 ACP 适配器 | internal/extension/acp_mcp.go |

### 8.2 优雅关闭

所有 HTTP 端点使用 `*http.Server` + `Shutdown(ctx)` 实现优雅关闭：
1. 停止接收新连接
2. 等待活跃请求完成
3. 超时强制终止

关闭序列：
```
Signal → bgWg.Wait() (后台 goroutine)
     → runner.Close()
     → evolution.Close()
     → memory.Close() (5s timeout)
     → session.Close()
     → graphFlow.Close()
     → telemetry.Shutdown(10s) (OTLP + Langfuse 刷新)
     → dbPool.Close() → WAL checkpoint
```

### 8.3 安全规范

- 所有 `json.NewDecoder` 配置 `io.LimitReader(10MB)` 防 DoS
- 所有 `context.Background()` 调用附加超时
- Goroutine 生命周期由 `CoreLoop.bgWg` 追踪

---

## 9. 快速开始

```bash
# 安装
go install github.com/km269/wukong/cmd/wukong@latest

# 交互式配置
wukong configure

# 最小配置 (LMStudio 本地模型)
# config.yaml:
#   default_provider: "lmstudio"
#   lightweight_provider: "lmstudio"
#   lightweight_model: "gemma-4-e4b-it"
#   providers:
#     - name: "lmstudio"
#       type: "lmstudio"
#       base_url: "http://localhost:1234/v1"
#       api_key: "lmstudio"
#       model: "google/gemma-4-26b-a4b"

# 交互式会话 (自动启用全栈能力)
wukong session

# 指定 Provider/模型
wukong session --provider deepseek --model deepseek-chat

# 非交互式执行
wukong run --prompt "分析这个项目"
wukong run --prompt "修复测试" --temperature 0.3

# 对话模式
wukong run --prompt "开始项目规划" --dialogue

# 扩展管理
wukong extension list
wukong extension install --url "wukong://extension?name=my_tool&transport=stdio&command=python&args=server.py"
wukong extension enable my_tool
```

启动 `wukong session` 后**自动启用**：
- ✅ 对话持久化 (FTS5 + HNSW 索引)
- ✅ 记忆回溯唤醒 (CortexDB WakeUp)
- ✅ 事实提取与持久化 (MemoryFlow PromoteFacts)
- ✅ 知识图谱构建 (GraphFlow auto_extract)
- ✅ 上下文智能压缩 (ContextRevisionEngine)
- ✅ 5 层安全纵深防御
- ✅ A2A / ACP / AG-UI / MCP 四协议服务端点

---

## 10. 项目结构

```
wukong/
├── cmd/wukong/main.go              # 入口点 (调用 cli.Execute())
├── config.yaml                     # 完整配置 (30+ 配置段, ~590 行)
├── pkg/sandbox/                    # OS 级文件沙箱 (可单独引用)
│   ├── sandbox.go                  #   Command / CommandContext / Start / Run / Probe
│   ├── sandbox_linux.go            #   Landlock 后端 (内核 5.13+, self-exec)
│   ├── sandbox_darwin.go           #   macOS sandbox-exec + Seatbelt
│   ├── sandbox_windows.go          #   Windows Low Integrity Level
│   ├── sandbox_other.go            #   不支持的平台 (unsandboxed + warning)
│   └── sandbox_stubs.go            #   构建标签存根
├── internal/
│   ├── agent/                      # 核心引擎 (13 文件) ★
│   │   ├── loop.go                 #   CoreLoop 中央编排器 (~1560 行)
│   │   ├── context.go              #   上下文压缩引擎 (3 层策略)
│   │   ├── workflow.go             #   10 种编排模式 (WorkflowBuilder)
│   │   ├── team.go                 #   多 Agent 团队编排 (TeamBuilder)
│   │   ├── recipe.go               #   YAML 配方定义 (RecipeManager)
│   │   ├── prompt_template.go      #   .md 提示模板系统
│   │   ├── dify.go                 #   Dify AI 平台集成
│   │   ├── hitl.go                 #   人机协同中断/恢复
│   │   └── todo_enforcer.go        #   Todo 完成强制执行器
│   ├── cortex/                     # CortexDB 智能记忆栈 (12 文件) ★
│   │   ├── store.go                #   CortexStore (HNSW 向量 + FTS5)
│   │   ├── memoryflow.go           #   MemoryFlow (转录记录/唤醒/事实桥接)
│   │   ├── graphflow.go            #   GraphFlow (知识图谱构建)
│   │   ├── extractor.go            #   LLM + 启发式事实提取器
│   │   ├── planner.go              #   检索策略规划器
│   │   ├── recall_manager.go       #   跨系统搜索管理器
│   │   ├── import_flow.go          #   DDL/CSV 结构化导入
│   │   ├── embedder.go             #   OpenAI 兼容嵌入客户端
│   │   ├── json_generator.go       #   LLM 实体提取 JSON 生成器
│   │   ├── lexical.go              #   SQLite + FTS5 词法回退存储
│   │   ├── kg_tools.go             #   KG 查询/分析工具
│   │   └── import_tools.go         #   DDL 解析+映射工具
│   ├── extension/                  # 扩展系统 (24 文件)
│   │   ├── manager.go              #   MCP 扩展生命周期管理
│   │   ├── mcp_client.go           #   原生 MCP 客户端 (stdio/sse/streamable)
│   │   ├── acp_mcp.go              #   ACP MCP 桥接器
│   │   ├── deeplink.go             #   Deeplink URL 解析器
│   │   └── builtin/                #   12 个内置扩展 (14 文件)
│   ├── memory/                     # tRPC Memory 管理器 → AutoExtract + SmartCleanup
│   ├── recall/                     # FTS5 跨会话对话搜索 Store
│   ├── session/                    # 会话存储 (SQLite/Redis/InMemory)
│   ├── security/                   # 安全守卫 (Guard + .wukongignore)
│   ├── config/config.go            # Viper 配置系统 (38 结构体, ~1534 行)
│   ├── cli/                        # CLI + Bubbletea TUI
│   │   ├── root.go                 #   Cobra 根命令 + 7 个子命令
│   │   ├── session.go              #   交互会话引导 (~1300 行)
│   │   ├── run.go                  #   非交互式执行
│   │   ├── configure.go            #   交互式配置向导
│   │   ├── extension.go            #   扩展管理命令
│   │   └── tui/                    #   Bubbletea TUI (model/update/view)
│   ├── provider/                   # LLM Provider Factory (7 种后端)
│   ├── browser/                    # Chromedp 浏览器自动化 (HTTP + Chrome)
│   ├── codemode/                   # goja JS 沙箱执行器
│   ├── knowledge/                  # RAG 知识库管理 (tRPC Knowledge)
│   ├── summon/                     # 子代理委托 (A2A + 凭证轮换)
│   ├── skill/                      # Agent Skill 系统 (FSRepository)
│   ├── evolution/                  # 技能自进化引擎
│   ├── server/                     # ACP Server + AG-UI SSE Server
│   ├── health/                     # 健康检查 (DB/Model/Extension/A2A)
│   ├── telemetry/                  # OpenTelemetry (gRPC/HTTP/Console)
│   ├── observability/              # Langfuse LLM 追踪
│   ├── todo/                       # 任务追踪系统 (SQLite CRUD)
│   ├── topofmind/                  # 持久化指令注入
│   ├── project/                    # 工作目录追踪与恢复
│   ├── apps/                       # HTML 应用管理
│   ├── artifact/                   # 制品存储工厂 (InMemory/COS)
│   ├── eval/                       # Agent 评估/回归测试框架
│   └── util/                       # 共享工具 (数据库连接池/日志)
└── docs/
    ├── README.md                   # 本文档 (项目详细概览)
    ├── ARCHITECTURE.md             # 系统架构深度分析 (23 个 ADR)
    └── CONFIG.md                   # 配置参考手册 (30 个配置段)
```

★ 标记为核心记忆子系统。

---

## 11. 文档索引

| 文档 | 行数 | 内容 |
|------|------|------|
| [架构分析](ARCHITECTURE.md) | ~490 行 | 系统架构深度分析、记忆闭环数据流、23 个关键设计决策 (ADR) |
| [配置手册](CONFIG.md) | ~456 行 | 30 个配置段完整参考、加载优先级、4 种推荐配置方案 |

---

## 技术栈

| 类别 | 选择 | 版本 |
|------|------|------|
| Agent 框架 | tRPC-Agent-Go | v1.10.0 |
| MCP 协议 | tRPC-MCP-Go | v0.0.16 |
| A2A 协议 | tRPC-A2A-Go | v0.2.5 |
| 智能记忆 | CortexDB | v2.25.0 |
| JS 引擎 | goja (纯 Go, 零 CGO) | - |
| 数据库 | SQLite WAL (单文件) | - |
| OS 沙箱 | pkg/sandbox (自维护) | - |
| 前端 | BubbleTea + LipGloss | - |
| 浏览器 | Chromedp (Chrome DevTools) | - |
| 配置 | Viper + Cobra | - |
| 可观测 | OpenTelemetry + Langfuse | - |
| 语言 | Go | 1.26 |
