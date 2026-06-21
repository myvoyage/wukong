# Wukong — 记忆优先、编排驱动、安全纵深的 AI Agent 平台

> **版本**: v0.6.0 | **Go**: 1.26 | **源文件**: 119 `.go` + 34 `_test.go` | **依赖**: 30 direct | **许可证**: GNU AGPL-3.0
>
> 基于 [tRPC-Agent-Go v1.10.0](https://github.com/trpc-group/trpc-agent-go) · [tRPC-MCP-Go v0.0.16](https://github.com/trpc-group/trpc-mcp-go) · [CortexDB v2.25.0](https://github.com/liliang-cn/cortexdb)

---

## 目录

1. [架构哲学](#1-架构哲学)
2. [核心优势](#2-核心优势)
3. [安全纵深模型](#3-安全纵深模型)
4. [系统全景](#4-系统全景)
5. [核心特性](#5-核心特性)
6. [快速开始](#6-快速开始)
7. [项目结构](#7-项目结构)
8. [文档索引](#8-文档索引)

---

## 1. 架构哲学

### 1.1 记忆优先（Memory-First）

> Agent 的真正智能来源于跨会话的知识积累，而非单次对话的上下文窗口。

- **双引擎记忆**：tRPC Memory（键值） + CortexDB MemoryFlow（转录+唤醒）
- **知识图谱**：RDF/SPARQL + 实体关系自动提取
- **HNSW 向量索引**：O(log N) 语义搜索
- **完整闭环**：记录 → 提取 → 召回 → 注入

### 1.2 框架组装（Framework-Assembled）

> 框架是能力的来源，不是限制的边界。任何组件都应可替换。

- `CoreLoop` 依赖注入模型，27 个子系统通过配置结构体注入
- 清晰接口边界：Security Guard、Context Engine、Extension Manager 各自独立
- 7 种 LLM Provider 通过统一工厂接入
- Session 后端可选 SQLite / Redis / InMemory

### 1.3 多 Agent 原生（Multi-Agent by Default）

> 复杂任务不应由单个 Agent 完成。多 Agent 编排是一等公民。

提供 **10 种显式编排模式**：single / chain / parallel / cycle / graph / team_coordinator / team_swarm / claude_code / codex / dify

### 1.4 进化智能（Evolving Intelligence）

> 技能应从失败中学习改进，而非等待人类修复。

- 执行追踪 → 异步 LLM 分析 → 补丁生成 → 版本管理 → 热重载
- 置信度阈值、冷却时间、每日上限、最大补丁大小四重约束

### 1.5 纵深防御（Defense in Depth）

> 安全不是单点检查，而是多层纵深。

见 [§3 安全纵深模型](#3-安全纵深模型)。

---

## 2. 核心优势

### 2.1 技术差异化

| 能力 | Wukong | 优势 |
|------|--------|------|
| **长期记忆** | 双引擎 + 知识图谱 + HNSW | 完整闭环，自动提取 |
| **多 Agent 编排** | 10 种显式模式 | 图工作流、循环迭代、蜂群协作 |
| **安全纵深** | 5 层防御 | 从 LLM 权限到 OS 内核全覆盖 |
| **JS 沙箱** | goja 三级防护 | 白名单 + 内存限制 + 并发控制 + ReDoS 防护 |
| **OS 沙箱** | Landlock / Seatbelt / Low IL | 内核级文件写入保护 |
| **上下文压缩** | 3 层策略 + 独立摘要模型 | LLM 智能摘要 + 渐进式压缩 |
| **技能进化** | 自动分析 → 补丁 → 热重载 | 闭环自改进 |
| **数据库** | 单文件 wukong.db | 零配置部署 |

### 2.2 记忆系统全景

```
tRPC Memory (键值)        CortexDB MemoryFlow (转录+唤醒)
    │                           │
    ├── AutoExtract (9B 异步)   ├── IngestTurn (记录)
    ├── 6 个工具                ├── WakeUp (向量+全文召回)
    └── 结构化事实              ├── PromoteFacts (桥接)
                                ├── GraphFlow (RDF 知识图谱)
                                └── ImportFlow (DDL/CSV → KG)
```

---

## 3. 安全纵深模型

```
Layer 5 ─ Guard 权限控制
  auto / smart / manual / chat_only
  allowlist / denylist / blocked_commands
  prompt injection 审查

Layer 4 ─ goja JS 沙箱 (code_mode)
  白名单 API: console、JSON、Math
  禁用: eval、Function、RegExp、Date
  debug.SetMemoryLimit(128MB)
  并发限流 semaphore (max 5)
  JSON.parse 1MB 输入限制

Layer 3 ─ sandbox OS 级隔离 (command_execute / code_search)
  Linux:    Landlock (内核 5.13+，全文件系统只读)
  macOS:    sandbox-exec + Seatbelt
  Windows:  Low Integrity Level + Mandatory Labels
  保护: 工作目录 + .wukong 可写，其余只读

Layer 2 ─ .wukongignore 文件黑名单
  gitignore 兼容语法
  file_read / write / replace / delete 路径验证

Layer 1 ─ OS 进程权限
  非 root 运行
  ulimit 资源限制
```

---

## 4. 系统全景

```
CLI (cobra + BubbleTea) │ API (A2A :9090 / ACP :9091 / AG-UI :8080)
  └── CoreLoop (中央编排器)
      ├── WorkflowBuilder (10 种模式) + TeamBuilder (多 Agent)
      ├── ContextRevisionEngine (3 层压缩)
      ├── Security Guard (5 层防御)
      └── tRPC-Agent-Go Runner
          ├── LLMAgent / ChainAgent / ParallelAgent / CycleAgent / GraphAgent
          ├── Session / Memory / Artifact / Tool / Planner / Plugin
          └── Extensions (12 内置 + MCP 外部)

Storage: wukong.db (SQLite WAL 单文件)
  sessions / memories / recall (FTS5) / todos / transcripts
  knowledge graph (RDF) / HNSV vectors / evolution records

Protocols: MCP (stdio/sse/streamable) | A2A | ACP | AG-UI SSE
```

---

## 5. 核心特性

### 5.1 LLM Provider（7 种）

OpenAI · Anthropic · Google · DeepSeek · Ollama · LMStudio · ACP 代理

### 5.2 内置扩展（12 个，50+ 工具）

| 扩展 | 工具数 | 核心能力 |
|------|--------|----------|
| developer | 6 | 文件读写、命令执行（sandbox 保护）、代码搜索 |
| memory | 6 | 长期记忆 CRUD |
| cortex | 4 | SPARQL 知识图谱查询 |
| code_mode | 2 | goja JS 沙箱执行 + 工具发现 |
| computer_controller | 9 | 鼠标/键盘/截图/剪贴板 |
| auto_visualiser | 3 | 图表/流程图生成 |
| tutorial | 3 | 交互式教程 |
| top_of_mind | 4 | 持久指令 |
| apps | 5 | HTML 应用 CRUD |
| web | 1 | Web 搜索 |
| agent_tools | 3 | 子 Agent 工具 |
| mcp_broker | 4 | 外部 MCP 扩展聚合 |

### 5.3 附加能力

RAG 知识检索 · Chromedp 浏览器自动化 · JS 沙箱 (goja) · 技能系统 + 自进化 · Dify 集成 · OpenTelemetry + Langfuse · 制品存储 (InMemory/COS)

---

## 6. 快速开始

```bash
# 安装
go install github.com/km269/wukong/cmd/wukong@latest

# 配置（交互式或直接编辑 config.yaml）
wukong configure

# 最小配置 (LMStudio 本地模型)
# config.yaml:
# default_provider: "lmstudio"
# providers:
#   - name: "lmstudio"
#     type: "lmstudio"
#     base_url: "http://localhost:1234/v1"
#     model: "google/gemma-4-26b-a4b"

# 交互式会话
wukong session

# 指定 Provider/模型
wukong session --provider deepseek --model deepseek-chat
```

启动后**自动启用**：对话持久化、记忆转录唤醒、知识图谱构建、上下文智能压缩、5 层安全防护、YAML Recipe 发现、三协议服务端点。

---

## 7. 项目结构

```
wukong/
├── cmd/wukong/main.go              # 入口点
├── config.yaml                     # 完整配置 (577 行)
├── pkg/sandbox/                    # OS 级文件沙箱 (10 文件) ★
│   ├── sandbox.go                  # 核心 API (Command / CommandContext / Probe)
│   ├── sandbox_linux.go            # Landlock 后端 (内核 5.13+)
│   ├── sandbox_darwin.go           # macOS sandbox-exec 后端
│   ├── sandbox_windows.go          # Windows Low IL 后端
│   ├── helper_linux.go             # self-exec 辅助进程
│   └── ...
├── internal/
│   ├── agent/                      # 核心引擎 (14 文件)
│   │   ├── loop.go                 # 中央编排器 (1300+ 行) ★
│   │   ├── context.go              # 上下文管理 (3 层压缩)
│   │   ├── workflow.go             # 10 种编排模式
│   │   ├── team.go                 # 多 Agent 团队
│   │   ├── hitl.go                 # 人机回环
│   │   └── ...
│   ├── codemode/                   # goja JS 沙箱 (2 文件) ★
│   ├── security/                   # 安全守卫 (4 文件) ★
│   ├── extension/                  # 扩展系统 (19 文件)
│   │   └── builtin/                # 12 个内置扩展
│   ├── cortex/                     # 智能记忆 (12 文件) ★
│   ├── server/                     # ACP + AG-UI 服务器
│   ├── cli/                        # CLI + TUI 界面
│   ├── config/                     # Viper 配置系统 (30 段)
│   ├── provider/                   # LLM Provider 工厂 (7 种)
│   ├── memory/ recall/ session/    # 存储层
│   ├── evolution/ summon/ skill/   # 进化 + 调度 + 技能
│   ├── browser/ knowledge/         # 浏览器 + RAG
│   └── util/                       # 数据库池 + 日志
├── docs/
│   ├── README.md                   # 本文档
│   ├── ARCHITECTURE.md             # 系统架构深度分析
│   └── CONFIG.md                   # 配置参考手册
└── go.mod / go.sum / Makefile
```

★ 标记为核心子系统。

---

## 8. 文档索引

| 文档 | 说明 |
|------|------|
| [架构文档](ARCHITECTURE.md) | 系统架构深度分析、子系统设计、数据流、ADR 决策 |
| [配置手册](CONFIG.md) | 30 个配置段完整参考、加载优先级、推荐配置 |
