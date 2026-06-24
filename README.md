# Wukong — 记忆优先 · 编排驱动 · 安全纵深 · 双向发现

> **本地优先、框架组装、可深度扩展的开源 AI Agent 平台**
>
> Go 1.26 | tRPC-Agent-Go v1.10.0 | CortexDB v2.25.0 | GNU AGPL-3.0

Wukong 的核心理念：Agent 的真正智能不取决于单次对话的表现，而取决于跨会话的记忆积累、多 Agent 的协同编排、多层纵深的安全防御、以及技能的持续自进化。

---

## 架构哲学

| 哲学 | 核心信念 | 关键工程决策 |
|------|----------|-------------|
| **记忆优先** | Agent 智能源于跨会话知识积累 | 双引擎三层记忆：tRPC Memory + CortexDB Stack (HNSW+FTS5+RDF) |
| **框架组装** | 任何组件都应可替换 | CoreLoop 依赖注入，12 子系统接口隔离 |
| **多 Agent 原生** | 编排是第一公民 | 10 种显式编排模式 + HITL 人机协同 |
| **进化智能** | 技能应从失败中学习 | LLM 分析 → 自动补丁 → 版本管理 → 热重载 |
| **双向发现** | 发现别人，也被人发现 | ARD: 联邦搜索 + RegistryServer 发布 |

---

## 核心能力总览

| 维度 | 方案 |
|------|------|
| **代码规模** | 175 `.go` 文件（42 `_test.go`）/ 28 包 |
| **编排模式** | 10 种：single / chain / parallel / cycle / graph / team_coordinator / team_swarm / claude_code / codex / dify |
| **Recipe** | 14 项功能：参数化、结构化输出、子配方、重试、继承、内联、模型覆盖、超时、热重载、指标 |
| **LLM 后端** | 7 种：OpenAI / Anthropic / Google / DeepSeek / Ollama / LMStudio / ACP |
| **记忆系统** | 双引擎三层：tRPC Memory (SQLite KV+SmartCleanup) × CortexDB (HNSW+FTS5+RDF) |
| **安全防御** | 5 层纵深：Guard → goja JS 沙箱 → OS 沙箱 → `.wukongignore` → OS 权限 |
| **扩展体系** | 12 内置扩展 + MCP Broker + ACP MCP Bridge |
| **ARD 双向发现** | 联邦搜索远程 Registry + 发布自身 Registry (:8081) |
| **多协议支持** | A2A(:9090) / ACP(:9091) / AG-UI SSE(:8080) / ACP MCP(:3400) |
| **存储** | 单文件 `wukong.db` (SQLite WAL) 承载全栈 |

---

## 快速开始

```bash
go install github.com/km269/wukong/cmd/wukong@latest

# 交互式配置向导
wukong configure

# 启动交互式会话（自动启用：记忆/知识图谱/安全/压缩/热重载）
wukong session

# 指定 provider 和 model
wukong session --provider deepseek --model deepseek-chat

# 单次执行模式
wukong run --prompt "分析项目结构"

# 扩展管理
wukong extension list
wukong extension install <name> --url <mcp-server-url>

# 项目追踪
wukong project list
wukong project start <name>
```

---

## 项目结构

```
wukong/
├── cmd/wukong/main.go              # 入口点（19 行）
├── config.yaml                     # 主配置文件（32 配置段，39 结构体）
├── ard.yaml                        # ARD 双向发现配置
├── Makefile                        # 构建/测试/发布
├── Taskfile.yaml                   # Task 构建工具
├── .wukong/                        # 运行时数据
│   ├── apps/                       # HTML 应用
│   ├── cache/                      # 浏览器缓存
│   ├── recipes/                    # Recipe 定义（YAML）
│   ├── skills/                     # Agent Skill 定义
│   └── visuals/                    # 可视化输出
├── pkg/                            # 可复用公共库（2 包）
│   ├── sandbox/ (10 文件)          # OS 级文件沙箱（Linux/macOS/Windows）
│   └── zim/ (4 文件)               # ZIM 格式归档库
├── internal/ (160 文件, 28 子目录)  # 核心实现
│   ├── agent/ (21 文件)            # CoreLoop 编排引擎 + WorkflowBuilder + Recipe + HITL
│   ├── cli/ (11 文件)              # Cobra CLI 命令 + Bubbletea TUI
│   ├── config/ (2 文件)            # Viper 配置管理（70KB config.go）
│   ├── provider/ (3 文件)          # LLM 工厂（7 种后端）
│   ├── server/ (3 文件)            # ACP + AG-UI SSE 服务器
│   ├── extension/ (25 文件)        # 扩展管理器 + 12 内置扩展
│   │   └── builtin/ (15 文件)      # developer/memory/cortex/codemode/browser/...
│   ├── cortex/ (12 文件)           # CortexDB 记忆栈（HNSW+FTS5+RDF）
│   ├── memory/ (2 文件)            # tRPC Memory 长期记忆
│   ├── recall/ (4 文件)            # FTS5 全文搜索召回
│   ├── security/ (4 文件)          # 5 层安全纵深防御
│   ├── ard/ (16 文件)              # ARD 双向发现（联邦搜索+注册发布）
│   ├── apps/ (18 文件)             # HTML 应用管理（克隆/打包/清理/服务）
│   ├── evolution/ (6 文件)         # 技能自进化引擎
│   ├── summon/ (6 文件)            # 子代理委派 + A2A
│   ├── browser/ (2 文件)           # Chromedp 浏览器自动化
│   ├── codemode/ (2 文件)          # goja JavaScript 沙箱
│   ├── skill/ (1 文件)             # Agent Skill 仓库
│   ├── knowledge/ (1 文件)         # RAG 知识库
│   ├── session/ (3 文件)           # 会话持久化（SQLite/Redis/内存）
│   ├── todo/ (2 文件)              # 任务管理
│   ├── project/ (2 文件)           # 工作目录追踪
│   ├── topofmind/ (2 文件)         # 持久指令注入
│   ├── telemetry/ (2 文件)          # OpenTelemetry 分布式追踪
│   ├── observability/ (1 文件)     # Langfuse LLM 追踪
│   ├── health/ (2 文件)            # 健康检查
│   ├── artifact/ (1 文件)          # 文件版本化
│   ├── eval/ (1 文件)              # Agent 评估
│   └── util/ (5 文件)              # 数据库池/日志/指针工具
└── docs/                           # 文档
    ├── README.md                    # 架构哲学与特性概述
    ├── ARCHITECTURE.md              # 系统架构深度分析
    ├── CONFIG.md                    # 配置参考手册
    └── LICENSE                      # GNU AGPL-3.0
```

---

## 技术选型

| 类别 | 选择 | 版本 | 用途 |
|------|------|------|------|
| Agent 框架 | tRPC-Agent-Go | v1.10.0 | Agent 编排、Runner、Planner |
| MCP 协议 | tRPC-MCP-Go | v0.0.16 | 模型上下文协议 |
| A2A 协议 | tRPC-A2A-Go | v0.2.5 | Agent 间通信 |
| 智能记忆 | CortexDB | v2.25.0 | HNSW 向量 + FTS5 全文 + RDF 图谱 |
| CLI 框架 | Cobra + Viper | latest | 命令行参数与配置 |
| TUI 界面 | Bubbletea + LipGloss | latest | 三区布局交互界面 |
| 浏览器 | Chromedp | v0.15.1 | 无头浏览器自动化 |
| JS 引擎 | goja | latest | JavaScript 沙箱执行 |
| 数据库 | modernc.org/sqlite | v1.38.2 | 纯 Go SQLite WAL 模式 |
| 缓存 | go-redis | v9.12.1 | Redis 会话后端 |
| 文件监控 | fsnotify | v1.8.0 | 技能/配置热重载 |
| 可观测性 | OpenTelemetry | v1.43.0 | 分布式追踪 |
| LLM 追踪 | Langfuse | - | LLM 调用追踪 |
| 语言 | Go | 1.26 | 编译型、并发、跨平台 |

---

## ARD 双向发现

Wukong 支持 Agentic Resource Discovery —— 既能发现其他 Agent/Server，也能被其他 Agent 发现：

```yaml
# ard.yaml
ard:
  enabled: true
  registry_url: "https://remote.registry.example.com"  # 发现别人
  catalog_path: ".wukong/ard/catalog.json"
  publish_enabled: true                                  # 让自己可被发现
  publish_port: 8081                                     # 发布端口
```

- **Outbound**: 通过 `ard_search` / `ard_discover` 工具搜索远程 Registry
- **Inbound**: 在 `:8081` 启动 RegistryServer，通过 `/.well-known/ai-catalog.json` 暴露自身
- **Auto**: MCP 连接和 A2A Remote 自动注册到 ARD 目录

---

## 安全纵深防御

Wukong 实施 5 层纵深防御：

```
Layer 5: Guard        — auto/smart/manual/chat_only 权限模式 + 命令拦截 + Prompt注入检测
Layer 4: goja JS      — API 白名单 + 128MB 限制 + 5 并发 + ReDoS 防护 + 1MB 代码限制
Layer 3: OS 沙箱       — Landlock(Linux) / Seatbelt(macOS) / Low IL(Windows)
Layer 2: .wukongignore — gitignore 兼容的文件黑名单
Layer 1: OS 权限       — 非 root 运行 + ulimit 限制
```

---

## 文档索引

| 文档 | 说明 |
|------|------|
| [项目概述](docs/README.md) | 架构哲学、核心特性、快速开始 |
| [架构分析](docs/ARCHITECTURE.md) | 14 章完整架构、28 ADR、模块依赖、数据流 |
| [配置手册](docs/CONFIG.md) | 32 配置段、39 结构体、4 种推荐方案 |

---

## 许可证

[GNU AGPL-3.0](docs/LICENSE) — 自由使用、修改和分发，但衍生作品必须同样以 AGPL-3.0 开源。
