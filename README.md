# Wukong

> **记忆优先 · 编排驱动 · 安全纵深 AI Agent 平台**
>
> Go 1.26 | tRPC-Agent-Go v1.10.0 | CortexDB v2.25.0 | GNU AGPL-3.0

Wukong 是一个本地优先、框架组装、可深度扩展的 AI Agent 平台。核心理念：**Agent 的真正智能不取决于单次对话的表现，而取决于跨会话的记忆积累、多 Agent 的协同编排、多层纵深的安全防御、以及技能的持续自进化。**

---

## 核心能力矩阵

| 维度 | 数量/方案 | 实现 |
|------|----------|------|
| **文件规模** | 138 `.go` / 27 包 | `cmd/` (1) + `pkg/` (10) + `internal/` (127) |
| **编排模式** | 10 种 | single / chain / parallel / cycle / graph / team_coordinator / team_swarm / claude_code / codex / dify |
| **Recipe 功能** | 14 项  | 参数化/结构化输出/子配方/重试/继承/内联/模型覆盖/超时/发现/热重载/指令模板/指标 |
| **LLM 后端** | 7 种 | OpenAI / Anthropic / Google / DeepSeek / Ollama / LMStudio / ACP |
| **记忆引擎** | 双引擎闭环 | tRPC Memory (KV + AutoExtract + SmartCleanup) × CortexDB (HNSW + FTS5 + RDF) |
| **安全防御** | 5 层纵深 | Guard → goja JS 沙箱 → OS 沙箱 → .wukongignore → OS 权限 |
| **内置扩展** | 10 个 | developer / memory / cortex / code_mode / browser / visualiser / tutorial / apps / web / topofmind |
| **协议服务** | 4 种 | A2A (:9090) / ACP (:9091) / AG-UI SSE (:8080) / ACP MCP (:3400) |
| **部署形态** | 单文件 | wukong.db 承载全栈：session/memory/recall/cortex/todo/evolution |

---

## 5 层安全纵深

```
Layer 5: Guard 权限控制    → auto / smart / manual / chat_only + 命令拦截 + Prompt 注入检测
Layer 4: goja JS 沙箱      → API 白名单 + 128MB 限制 + 5 并发 + ReDoS 防护
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

# 启动交互会话
wukong session

# 指定模型
wukong session --provider deepseek --model deepseek-chat

# 非交互式单次执行
wukong run --prompt "分析当前项目结构"

# 扩展管理
wukong extension install --url wukong://extension?name=memory&version=v1.0.0
wukong extension list
```

启动 `wukong session` 后**自动启用**：

- **对话持久化**：FTS5 全文索引 + HNSW 向量存储
- **记忆闭环**：转录 → 语义唤醒 → 事实提取 → 持久化
- **知识图谱**：每轮对话自动构建实体/关系 RDF 图
- **上下文压缩**：LLM 智能摘要 + 渐进式截断
- **安全防护**：5 层纵深防御全覆盖
- **Recipe 系统**：14 项功能的 YAML 配方子 Agent
- **热重载**：fsnotify 监听 recipe 文件自动重建
- **可观测性**：OpenTelemetry + Langfuse 全链路追踪

---

## Recipe 系统

基于 YAML 的结构化子 Agent 定义，14 项功能覆盖：

```yaml
# .wukong/recipes/code_reviewer.yaml
name: code_reviewer
description: "Parameterized code reviewer"
instruction: "You are a {{.language}} expert."    # 指令模板
prompt: "Review: {{.code}}"                        # 参数模板
extends: base_reviewer                            # 继承
parameters:                                       #  参数
  - key: language
    type: select
    options: [go, python, rust]
tools:                                            #  子配方
  - file_read
  - recipe-sub-reviewer
model: "gpt-4o"                                   # 模型覆盖
timeout: "30s"                                    # 超时
retry:                                            # 重试
  max_attempts: 3
response:                                         # 结构化输出
  json_schema: {type: object, required: [issues]}
  validate_output: true
```

**辅组工具**: `list_recipes` (发现) / `reload_recipes` (热重载) / `recipe_stats` (指标)

---

## 项目结构

```
wukong/
├── cmd/wukong/main.go              # 入口
├── config.yaml                     # 完整配置 (30+ 配置段)
├── pkg/sandbox/                    # OS 级文件沙箱 (10 文件)
│   ├── sandbox.go                  #   Command / CommandContext
│   ├── sandbox_linux.go            #   Landlock 后端 (内核 5.13+)
│   ├── sandbox_darwin.go           #   sandbox-exec + Seatbelt
│   └── sandbox_windows.go          #   Low Integrity Level
├── internal/                       # 27 个包，127 个 .go 文件
│   ├── agent/                      # 核心引擎 (21 文件)
│   │   ├── loop.go                 #   CoreLoop 中央编排器
│   │   ├── context.go              #   上下文压缩引擎 (3 层)
│   │   ├── workflow.go             #   10 种编排模式
│   │   ├── team.go                 #   多 Agent 团队 (coordinator/swarm)
│   │   ├── recipe.go               #   YAML Recipe 定义 + 加载流水线
│   │   ├── recipe_tool.go          #   参数化模板 + 指标收集
│   │   ├── recipe_compose.go       #   子配方组合/继承/重试包装器
│   │   ├── recipe_advance.go       #   模型覆盖/超时/发现/热重载
│   │   ├── recipe_metrics.go       #   执行指标 + recipe_stats 工具
│   │   ├── prompt_template.go      #   提示模板系统
│   │   ├── dify.go                 #   Dify 平台集成
│   │   ├── hitl.go                 #   HITL 人机协同
│   │   └── todo_enforcer.go        #   Todo 执行器
│   ├── config/config.go            # Viper 配置 (38 结构体)
│   ├── cli/                        # CLI + Bubbletea TUI (10 文件)
│   │   ├── session.go              #   会话引导 (~1300 行, 28 子系统初始化)
│   │   ├── run.go / configure.go   #   运行/配置命令
│   │   └── tui/                    #   TUI 界面
│   ├── provider/                   # LLM Provider 工厂 (3 文件, 7 backend)
│   ├── cortex/                     # CortexDB 智能记忆栈 (12 文件)
│   │   ├── store.go                #   HNSW + FTS5 混合搜索
│   │   ├── memoryflow.go           #   转录/唤醒/PromoteFacts 桥接
│   │   ├── graphflow.go            #   RDF 知识图谱 auto_extract
│   │   ├── extractor.go            #   LLM + 启发式事实提取
│   │   └── recall_manager.go       #   跨系统搜索管理器
│   ├── extension/                  # 扩展系统 (23 文件)
│   │   ├── manager.go              #   MCP 扩展生命周期管理
│   │   ├── mcp_client.go           #   原生 MCP 客户端 (3 传输)
│   │   ├── acp_mcp.go              #   ACP MCP 桥接器
│   │   └── builtin/                #   10 个内置扩展
│   ├── memory/                     # tRPC Memory 管理器 (AutoExtract)
│   ├── recall/                     # FTS5 对话搜索 Store
│   ├── security/                   # 安全守卫 (Guard + .wukongignore)
│   ├── evolution/                  # 技能自进化引擎 (6 文件)
│   ├── summon/                     # 子代理委派 + A2A (6 文件)
│   ├── server/                     # ACP + AG-UI SSE 服务 (3 文件)
│   ├── codemode/                   # goja JS 沙箱
│   ├── browser/                    # Chromedp 浏览器自动化
│   ├── knowledge/                  # RAG 知识库管理
│   ├── skill/                      # Agent Skill 系统
│   ├── session/                    # 会话存储 (SQLite/内存/Redis)
│   ├── todo/                       # 任务追踪系统
│   ├── project/                    # 工作目录追踪
│   ├── apps/                       # HTML 应用管理
│   ├── topofmind/                  # 持久化指令注入
│   ├── health/                     # 健康检查
│   ├── telemetry/                  # OpenTelemetry 分布式追踪
│   ├── observability/              # Langfuse LLM 追踪
│   ├── artifact/                   # 制品存储后端工厂
│   ├── eval/                       # 评估/回归测试框架
│   └── util/                       # 共享工具 (DatabasePool/Logger)
├── .wukong/                        # 运行时数据
│   ├── recipes/                    #   7 个 YAML 配方文件
│   ├── apps/ / cache/ / skills/    #   应用/缓存/技能目录
│   └── visuals/                    #   可视化输出目录
└── docs/                           # 文档
    ├── README.md                   #   项目概览
    ├── ARCHITECTURE.md             #   系统架构深度分析 (23 ADR)
    └── CONFIG.md                   #   配置参考手册
```

---

## 文档

| 文档 | 说明 |
|------|------|
| [项目概述](docs/README.md) | 架构哲学、核心优势、特性矩阵 |
| [架构分析](docs/ARCHITECTURE.md) | 子系统设计、数据流、23 个 ADR |
| [配置手册](docs/CONFIG.md) | 30+ 配置段完整参考、加载优先级 |

---

## 技术选型

| 类别 | 选择 | 版本 |
|------|------|------|
| Agent 框架 | tRPC-Agent-Go | v1.10.0 |
| MCP 协议 | tRPC-MCP-Go | v0.0.16 |
| A2A 协议 | tRPC-A2A-Go | v0.2.5 |
| 智能记忆 | CortexDB | v2.25.0 |
| JS 引擎 | goja | latest |
| OS 沙箱 | pkg/sandbox | 自维护 |
| 数据库 | SQLite WAL | single-file |
| 前端 | BubbleTea + LipGloss | TUI |
| 浏览器 | Chromedp | Chrome DevTools |
| 配置 | Viper + Cobra | CLI > ENV > YAML |
| 可观测 | OpenTelemetry + Langfuse | 全链路 |
| 文件监听 | fsnotify | v1.8.0 |
| 模板引擎 | text/template | stdlib |

## 许可证

GNU AGPL-3.0 — 见 [LICENSE](docs/LICENSE)
