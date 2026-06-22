# Wukong — 记忆优先、编排驱动、安全纵深的 AI Agent 平台

> **版本**: v0.7.0 | **Go**: 1.26 | **Go 文件**: 138 | **内部包**: 27 | **许可证**: GNU AGPL-3.0
>
> 基于 [tRPC-Agent-Go v1.10.0](https://github.com/trpc-group/trpc-agent-go) · [tRPC-MCP-Go v0.0.16](https://github.com/trpc-group/trpc-mcp-go) · [tRPC-A2A-Go v0.2.5](https://github.com/trpc-group/trpc-a2a-go) · [CortexDB v2.25.0](https://github.com/liliang-cn/cortexdb)

---

## 1. 架构哲学

| 哲学 | 核心信念 | 关键实现 |
|------|----------|----------|
| **记忆优先** | Agent 智能来源于跨会话知识积累 | 双引擎记忆闭环：tRPC Memory + CortexDB Stack |
| **框架组装** | 任何组件都应可替换 | CoreLoop 依赖注入，12 个子系统解耦 |
| **多 Agent 原生** | 编排是第一公民 | 10 种显式编排模式 + HITL |
| **进化智能** | 技能应从失败中学习 | LLM 分析 → 自动补丁 → 版本管理 → 热重载 |
| **纵深防御** | 安全是多层协同 | 5 层防御从 LLM 权限到 OS 内核 |

---

## 2. 系统架构全景

```
┌──────────────────────────────────────────────────────────────────┐
│                     Wukong AI Agent Platform                      │
├──────────────────────────────────────────────────────────────────┤
│ Core Engine: CoreLoop — 中央编排器 (12 子系统)                    │
│   WorkflowBuilder (10 模式) · TeamBuilder · ContextManager        │
│   Security Guard (5 层) · HITL (中断-恢复)                        │
├──────────────────────────────────────────────────────────────────┤
│ Memory Stack: 双引擎三层记忆                                      │
│   短期: MemoryFlow (转录 + WakeUp 上下文)                          │
│   中期: CortexStore (HNSW 向量 + FTS5 语义搜索)                   │
│   长期: tRPC Memory (KV 持久化) + GraphFlow (RDF 知识图谱)         │
├──────────────────────────────────────────────────────────────────┤
│ Recipe System: 14 项功能 (P0-P4)                                  │
│   参数化 · 结构化输出 · 子配方 · 重试 · 继承 · 内联 · 模型覆盖    │
│   超时 · 发现 · 热重载 · 指令模板 · 执行指标                       │
├──────────────────────────────────────────────────────────────────┤
│ Capability: 10 内置扩展 · Evolution Engine · CodeMode (goja JS)   │
│   Browser (Chromedp) · Knowledge (RAG) · Sandbox (OS lock)        │
├──────────────────────────────────────────────────────────────────┤
│ Infrastructure: 7 LLM backends · OpenTelemetry · Langfuse         │
│   DatabasePool (SQLite WAL) · fsnotify · text/template            │
├──────────────────────────────────────────────────────────────────┤
│ Storage: wukong.db — 单文件承载 session/memory/recall/cortex/todo  │
└──────────────────────────────────────────────────────────────────┘
```

---

## 3. 核心特性矩阵

### 3.1 多 Agent 编排 (10 种模式)

| 模式 | 说明 | 典型场景 |
|------|------|----------|
| `single` | 标准单 Agent | 日常对话（默认） |
| `chain` | planner→executor→reviewer 顺序 | 多步流水线 |
| `parallel` | code/doc/test analyzer 并发 | 多角度分析 |
| `cycle` | planner↔executor 迭代 | 自我改进 |
| `graph` | analyze→路由→执行→review | 复杂决策 |
| `team_coordinator` | Coordinator 委托团队成员 | 团队协作 |
| `team_swarm` | Agent 间自主 transfer | 自主委派 |
| `claude_code` | 本地 Claude CLI 封装 | Claude 集成 |
| `codex` | 本地 Codex CLI 封装 | Codex 集成 |
| `dify` | Dify 低代码平台 | 低代码编排 |

### 3.2 Recipe 系统 (14 项功能)

| 阶段 | 功能 | YAML 字段 |
|------|------|----------|
| P0-A | 参数化模板 | `prompt`, `parameters` |
| P0-B | 结构化输出 | `response.json_schema` |
| P1-A | 子配方组合 | `tools: [recipe-xxx]` |
| P1-B | 重试与输出校验 | `retry`, `response.validate_output` |
| P2-A | 内联配方 | `agent.inline_recipes` |
| P2-B | 配方继承 | `extends` |
| P3-A | 模型覆盖 | `model` |
| P3-B | 超时控制 | `timeout` |
| P3-C | 配方发现 | `list_recipes` 工具 |
| P3-D | 热重载 | `reload_recipes` + fsnotify |
| P4-A | 指令模板 | `instruction: "{{.var}}"` |
| P4-B | 执行指标 | 自动 CallCount/Success/Error 收集 |
| P4-C | 统计工具 | `recipe_stats` |

### 3.3 Recipe YAML 示例

```yaml
name: code_reviewer
description: "Parameterized code reviewer"
extends: base_reviewer                    # 继承
instruction: "You are a {{.language}} expert."  # 指令模板
prompt: "Review: {{.code}}"                     # 参数模板
parameters:                                     # 参数定义
  - key: language
    type: select
    options: [go, python, rust]
  - key: code
    type: string
    required: true
tools:                                          # 子配方
  - file_read
  - recipe-sub-reviewer
model: "gpt-4o"                                 # 模型覆盖
timeout: "30s"                                  # 超时
retry:                                          # 重试
  max_attempts: 3
response:                                       # 结构化输出
  json_schema: {type: object, required: [issues, summary]}
  validate_output: true
```

### 3.4 记忆系统

| 层级 | 引擎 | 存储 | 核心机制 |
|------|------|------|----------|
| 短期 | MemoryFlow | CortexDB Episode | 转录 + WakeUp 语义唤醒 |
| 中期 | CortexStore | HNSW + FTS5 | 向量语义搜索 + 全文搜索 |
| 长期 | tRPC Memory | SQLite KV | AutoExtract + SmartCleanup |
| 结构化 | GraphFlow | RDF 知识图谱 | auto_extract 每轮对话 |

---

## 4. 安全纵深模型

```
Layer 5 ── Guard：auto/smart/manual/chat_only + 命令拦截 + Prompt 注入检测
Layer 4 ── goja JS：API 白名单 + 128MB 限制 + 5 并发 + ReDoS 防护
Layer 3 ── sandbox OS：Landlock(linux)/seatbelt(macOS)/Low IL(Windows)
Layer 2 ── .wukongignore：gitignore 兼容文件访问黑名单
Layer 1 ── OS 进程权限：非 root + ulimit
```

---

## 5. 扩展与工具生态

**10 个内置扩展**: developer · memory · cortex · code_mode · browser · visualiser · tutorial · apps · web · topofmind

**MCP Broker** 聚合外部工具为 4 个入口：`mcp_list_servers` / `mcp_list_tools` / `mcp_inspect_tools` / `mcp_call`

**4 个协议端口**: A2A (:9090) · ACP (:9091) · AG-UI SSE (:8080) · ACP MCP (:3400)

---

## 6. 快速开始

```bash
go install github.com/km269/wukong/cmd/wukong@latest
wukong configure                # 交互式配置
wukong session                  # 启动交互会话
wukong session --provider deepseek --model deepseek-chat
wukong run --prompt "分析项目结构"
```

---

## 7. LLM Provider 支持

| Provider | type | 说明 |
|----------|------|------|
| OpenAI | `openai` | GPT-4o, GPT-4, etc. |
| Anthropic | `anthropic` | Claude Sonnet/Opus |
| Google | `google` | Gemini (via OpenAI API) |
| DeepSeek | `deepseek` | DeepSeek-Chat/Reasoner |
| Ollama | `ollama` | 本地开源模型 |
| LMStudio | `lmstudio` | 本地 GPU 加速 |
| ACP | `acp` | 远程 ACP Agent |

---

## 8. 文档索引

| 文档 | 说明 |
|------|------|
| [架构分析](ARCHITECTURE.md) | 完整系统架构、14 个子系统设计、数据流、23 个 ADR |
| [配置手册](CONFIG.md) | 30+ 配置段参考、加载优先级、推荐方案 |

---

## 9. 许可证

GNU AGPL-3.0 — 见 [LICENSE](LICENSE)
