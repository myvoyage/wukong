# Wukong

> **记忆优先 · 编排驱动 · 安全纵深 AI Agent 平台**
>
> Go 1.26 | tRPC-Agent-Go v1.10.0 | CortexDB v2.25.0 | GNU AGPL-3.0

Wukong 是一个本地优先、框架组装、可深度扩展的 AI Agent 平台。核心理念：**Agent 的真正智能不取决于单次对话的表现，而取决于跨会话的记忆积累、多 Agent 的协同编排、多层纵深的安全防御、以及技能的持续自进化。**

---

## 核心能力矩阵

| 维度 | 能力 | 实现 |
|------|------|------|
| **记忆引擎** | 双引擎闭环 | tRPC Memory (KV 持久化 + 自动提取 + 容量淘汰) × CortexDB Stack (转录回溯 + 向量唤醒 + 知识图谱) |
| **编排模式** | 10 种 | single / chain / parallel / cycle / graph / team_coordinator / team_swarm / claude_code / codex / dify |
| **安全防御** | 5 层纵深 | Guard 权限 → goja JS 沙箱 → OS 级沙箱 → .wukongignore → OS 进程权限 |
| **LLM 后端** | 7 种 | OpenAI / Anthropic / Google / DeepSeek / Ollama / LMStudio / ACP Agent |
| **内置扩展** | 12 个 / 50+ 工具 | developer / memory / cortex / code_mode / browser / visualiser / tutorial / apps / web / agent / topofmind / mcp_broker |
| **协议服务** | 4 种 | A2A (:9090) / ACP (:9091) / AG-UI SSE (:8080) / ACP MCP Bridge (:3400) |
| **部署形态** | 单文件 | wukong.db 承载 session/memory/recall/cortex/todo 全栈存储 |

---

## 5 层安全纵深

```
Layer 5: Guard 权限控制    → auto / smart / manual / chat_only + 命令拦截 + Prompt 注入检测
Layer 4: goja JS 沙箱      → API 白名单 + 内存限制 + 并发控制 + ReDoS 防护
Layer 3: sandbox OS 级隔离  → Landlock(linux) / sandbox-exec(macOS) / Low IL(Windows)
Layer 2: .wukongignore      → gitignore 兼容文件访问黑名单
Layer 1: OS 进程权限         → 非 root 用户运行 + ulimit
```

---

## 快速开始

```bash
# 安装
go install github.com/km269/wukong/cmd/wukong@latest

# 交互式配置向导
wukong configure

# 启动交互会话（自动启用记忆/压缩/安全/图谱/三协议服务）
wukong session

# 指定模型
wukong session --provider deepseek --model deepseek-chat

# 非交互式单次执行
wukong run --prompt "分析当前项目结构"

# 扩展管理
wukong extension install --url wukong://extension?name=memory&version=v1.0.0
wukong extension list
```

启动 `wukong session` 后**自动启用**的全栈能力：

- **对话持久化**：FTS5 全文索引 + HNSW 向量存储
- **记忆闭环**：转录记录 → 语义唤醒 → 事实提取 → 持久化存储
- **知识图谱**：每轮对话自动构建实体/关系 RDF 图
- **上下文压缩**：LLM 智能摘要 + 渐进式截断
- **安全防护**：5 层纵深防御全覆盖

---

## 项目结构

```
wukong/
├── cmd/wukong/main.go              # 入口
├── config.yaml                     # 完整配置 (30+ 配置段)
├── pkg/sandbox/                    # OS 级文件沙箱 (可单独引用)
│   ├── sandbox.go                  #   Command / CommandContext / Run / Start
│   ├── sandbox_linux.go            #   Landlock 后端 (内核 5.13+)
│   ├── sandbox_darwin.go           #   sandbox-exec + Seatbelt
│   └── sandbox_windows.go          #   Low Integrity Level
├── internal/
│   ├── agent/                      # 核心引擎 (13 文件)
│   │   ├── loop.go                 #   CoreLoop 中央编排器
│   │   ├── context.go              #   上下文压缩引擎
│   │   ├── workflow.go             #   10 种编排模式
│   │   ├── team.go                 #   多 Agent 团队编排
│   │   ├── recipe.go               #   YAML 配方定义
│   │   ├── prompt_template.go      #   提示模板系统
│   │   ├── dify.go                 #   Dify 平台集成
│   │   ├── hitl.go                 #   人机协同中断
│   │   └── todo_enforcer.go        #   Todo 执行器
│   ├── cortex/                     # CortexDB 智能记忆栈 (12 文件)
│   │   ├── store.go                #   CortexStore (HNSW + FTS5)
│   │   ├── memoryflow.go           #   MemoryFlow (转录/唤醒/桥接)
│   │   ├── graphflow.go            #   GraphFlow (RDF 知识图谱)
│   │   ├── extractor.go            #   LLM 事实提取器
│   │   ├── planner.go              #   检索策略规划器
│   │   ├── recall_manager.go       #   跨系统搜索管理器
│   │   ├── import_flow.go          #   结构化数据导入
│   │   ├── embedder.go             #   OpenAI 兼容嵌入客户端
│   │   ├── json_generator.go       #   LLM 实体提取 JSON
│   │   ├── lexical.go              #   词法回退存储
│   │   ├── kg_tools.go             #   知识图谱工具
│   │   └── import_tools.go         #   数据导入工具
│   ├── extension/                  # 扩展系统 (24 文件)
│   │   ├── manager.go              #   MCP 扩展生命周期管理
│   │   ├── mcp_client.go           #   原生 MCP 客户端
│   │   ├── acp_mcp.go              #   ACP MCP 桥接器
│   │   ├── deeplink.go             #   Deeplink 解析
│   │   └── builtin/                #   12 个内置扩展
│   ├── config/config.go            # Viper 配置 (38 个结构体, ~1500 行)
│   ├── cli/                        # CLI + Bubbletea TUI
│   │   ├── root.go                 #   Cobra 根命令
│   │   ├── session.go              #   交互会话引导 (~1300 行)
│   │   ├── run.go                  #   非交互式执行
│   │   ├── configure.go            #   交互式配置向导
│   │   ├── extension.go            #   扩展管理命令
│   │   └── tui/                    #   TUI 界面
│   ├── provider/                   # LLM Provider 工厂 (3 文件)
│   ├── security/                   # 安全守卫 (4 文件)
│   ├── server/                     # ACP + AG-UI SSE 服务 (3 文件)
│   ├── session/                    # 会话存储 (SQLite/内存/Redis)
│   ├── memory/                     # tRPC Memory 管理器
│   ├── recall/                     # 跨会话对话搜索
│   ├── browser/                    # Chromedp 浏览器自动化
│   ├── codemode/                   # goja JS 沙箱
│   ├── knowledge/                  # RAG 知识库管理
│   ├── summon/                     # 子代理委托 + A2A
│   ├── skill/                      # Agent Skill 系统
│   ├── evolution/                  # 技能自进化引擎
│   ├── health/                     # 健康检查
│   ├── observability/              # Langfuse LLM 追踪
│   ├── telemetry/                  # OpenTelemetry 分布式追踪
│   ├── todo/                       # 任务追踪系统
│   ├── topofmind/                  # 持久化指令注入
│   ├── project/                    # 工作目录追踪与恢复
│   ├── apps/                       # HTML 应用管理
│   ├── artifact/                   # 制品存储后端工厂
│   ├── eval/                       # 评估/回归测试框架
│   └── util/                       # 共享工具 (数据库连接池/日志)
└── docs/
    ├── README.md                   # 项目详细概览
    ├── ARCHITECTURE.md             # 系统架构深度分析 (23 个 ADR)
    └── CONFIG.md                   # 配置参考手册 (30 个配置段)
```

---

## 文档

| 文档 | 说明 |
|------|------|
| [项目概述](docs/README.md) | 架构哲学、核心优势、特性矩阵、快速开始 |
| [架构分析](docs/ARCHITECTURE.md) | 系统架构深度分析、子系统设计、数据流、23 个设计决策 |
| [配置手册](docs/CONFIG.md) | 30 个配置段完整参考、加载优先级、推荐配置方案 |

## 技术选型

| 类别 | 底层实现 |
|------|----------|
| Agent 框架 | tRPC-Agent-Go v1.10.0 |
| MCP 协议 | tRPC-MCP-Go v0.0.16 |
| A2A 协议 | tRPC-A2A-Go v0.2.5 |
| 智能记忆 | CortexDB v2.25.0 |
| JS 引擎 | goja (纯 Go) |
| 数据库 | SQLite WAL (单文件) |
| 前端 | BubbleTea + LipGloss (TUI) |
| 配置 | Viper + Cobra |
| 可观测 | OpenTelemetry + Langfuse |

## 许可证

GNU AGPL-3.0 — 见 [LICENSE](docs/LICENSE)
