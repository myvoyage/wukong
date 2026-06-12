# Wukong 🐵

> **本地优先、可扩展的 AI Agent 平台** | Go 语言 | tRPC 生态系统

Wukong 是一个本地优先、可扩展的 AI Agent 平台，基于 [tRPC-Agent-Go](https://github.com/trpc-group/trpc-agent-go)、[tRPC-MCP-Go](https://github.com/trpc-group/trpc-mcp-go) 和 [tRPC-A2A-Go](https://github.com/trpc-group/trpc-a2a-go) 三大框架构建。

---

## 功能特性

### 核心能力

| 特性 | 说明 |
|------|------|
| **交互式 Agent 循环** | LLM 推理 → 工具调用 → 结果反馈 → 响应输出 |
| **5 种工作流模式** | Single / Chain / Parallel / Cycle / Graph |
| **多模型支持** | OpenAI / Anthropic / Google / DeepSeek / Ollama / LMStudio |
| **MCP 工具生态** | 10+ 内置扩展 + 外部 MCP 服务器（STDIO/SSE/HTTP） |
| **长期记忆** | SQLite 持久化 + 自动提取 + 手动管理 |
| **RAG 知识检索** | Embedding 向量化 + 语义搜索 + 文档加载 |
| **会话管理** | SQLite/Redis/memory 后端 + 异步摘要 + 上下文压缩 |

### 智能增强

| 特性 | 说明 |
|------|------|
| **Todo 任务跟踪** | tRPC 原生 todo_write + TodoEnforcer 强制完成校验 |
| **Agent 工具委托** | Code Reviewer / Summarizer / Code Generator 子代理 |
| **评估系统** | EvalSet 用例集 + 4 种指标 + CLI 回归测试 |
| **上下文优化** | 两遍压缩 + Token 预算管理 + 摘要触发 |
| **跨会话召回** | FTS5 全文搜索 + Hybrid 混合语义搜索 |

### 分布式与可观测

| 特性 | 说明 |
|------|------|
| **A2A 协议** | Agent-to-Agent 分布式通信（Server + Client） |
| **AG-UI 协议** | SSE 实时对话，兼容 CopilotKit / TDesign Chat |
| **Langfuse 追踪** | LLM 全链路可观测 + 可视化调试 |
| **OpenTelemetry** | 分布式 Tracing + Metrics + Logging |
| **Artifact COS** | 腾讯云 COS 云端制品存储（版本化二进制数据） |

### 安全与治理

| 特性 | 说明 |
|------|------|
| **4 级权限模式** | auto / smart / manual / chat_only |
| **命令拦截** | 危险命令模式拦截（rm -rf /, dd, mkfs 等） |
| **Prompt 注入检测** | tRPC Guardrail 插件 |
| **工具过滤** | Allowlist / Denylist + 权限策略 |
| **HITL 中断** | Graph 工作流人工审批暂停/恢复 |

---

## 快速开始

### 前置条件

- Go 1.26+
- LLM API Key（或本地 Ollama / LMStudio）

### 安装

```bash
git clone https://github.com/km269/wukong.git
cd wukong
go build -o wukong ./cmd/wukong/
```

### 最小配置

```yaml
# config.yaml
default_provider: "openai"

providers:
  - name: "openai"
    type: "openai"
    base_url: "https://api.openai.com/v1"
    api_key: "${OPENAI_API_KEY}"
    model: "gpt-4o"
```

### 使用

```bash
# 启动交互式会话
./wukong session

# 指定 Provider
./wukong session --provider deepseek

# 恢复会话
./wukong session --session-id <id>

# 运行评估
./wukong eval

# 管理扩展
./wukong extension list
./wukong extension install <deeplink-url>

# 生成配置
./wukong configure
```

### TUI 操作

| 操作 | 说明 |
|------|------|
| 输入消息 + `Ctrl+D` | 发送消息 |
| `Ctrl+C` | 退出 |
| `/new` | 新会话 |
| `/clear` | 清屏 |
| `/model` | 查看/切换模型 |
| `/exts` | 查看扩展 |
| `/help` | 帮助 |

---

## 项目结构

```
wukong/
├── cmd/wukong/main.go              # 程序入口
├── internal/
│   ├── agent/                      # 核心引擎：循环、工作流、上下文、HITL
│   ├── apps/                       # HTML 应用管理
│   ├── artifact/                   # 制品服务工厂（inmemory/cos）
│   ├── browser/                    # 浏览器自动化（Chromedp）
│   ├── cli/                        # CLI 命令 + TUI + 配置向导
│   │   └── tui/                    # Bubbletea 终端界面
│   ├── codemode/                   # JS 沙箱（goja）
│   ├── config/                     # Viper 配置加载 + 全部 Config 结构体
│   ├── eval/                       # 评估框架（EvalSet/Metric/Evaluator）
│   ├── extension/                  # MCP 扩展系统
│   │   └── builtin/                # 10 个内置扩展
│   ├── knowledge/                  # RAG 知识检索（Embedding+索引+搜索）
│   ├── memory/                     # 长期记忆管理
│   ├── observability/              # Langfuse LLM 追踪
│   ├── provider/                   # LLM 模型工厂（6 种 Provider）
│   ├── recall/                     # 跨会话 FTS5 搜索
│   ├── security/                   # 安全守卫
│   ├── server/                     # AG-UI SSE 服务器
│   ├── session/                    # 会话存储（sqlite/redis）
│   ├── skill/                      # Agent Skill 仓库
│   ├── summon/                     # 子代理委派 + A2A 凭证
│   ├── telemetry/                  # OpenTelemetry
│   ├── todo/                       # 任务跟踪
│   ├── topofmind/                  # 持久化指令注入
│   └── util/                       # 工具库（DB 连接池、日志）
├── config.yaml                     # 默认配置
└── docs-related files...
```

---

## 技术栈

| 组件 | 技术 |
|------|------|
| **Agent 框架** | tRPC-Agent-Go v1.10.0 |
| **MCP 协议** | tRPC-MCP-Go v0.0.16 |
| **A2A 协议** | tRPC-A2A-Go v0.2.5 |
| **CLI/TUI** | Cobra + Bubbletea + Lipgloss |
| **存储** | SQLite (WAL) + Redis (go-redis/v9) |
| **LLM** | OpenAI 兼容 API |
| **浏览器** | Chromedp |
| **JS 沙箱** | goja |
| **可观测** | OpenTelemetry + Langfuse |
| **制品存储** | 腾讯云 COS (cos-go-sdk-v5) |
| **配置** | Viper (YAML + 环境变量) |

---

## 文档

| 文档 | 说明 |
|------|------|
| [README.md](README.md) | 本文档 |
| [ARCHITECTURE.md](ARCHITECTURE.md) | 系统架构设计文档 |
| [CONFIG.md](CONFIG.md) | 完整配置参考手册 |

---

## 开发

```bash
make build          # 构建
make test           # 测试
make lint           # 代码检查
make run            # 运行
```

## 致谢

- [tRPC-Agent-Go](https://github.com/trpc-group/trpc-agent-go) — Agent 核心框架
- [tRPC-MCP-Go](https://github.com/trpc-group/trpc-mcp-go) — MCP 协议实现
- [tRPC-A2A-Go](https://github.com/trpc-group/trpc-a2a-go) — A2A 协议实现
