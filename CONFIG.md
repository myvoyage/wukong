# Wukong 配置参考手册

> 完整配置文件: `config.yaml` | 配置加载器: `internal/config/config.go`

---

## 目录

1. [配置加载机制](#1-配置加载机制)
2. [配置项完整参考](#2-配置项完整参考)
   - [全局设置](#21-全局设置)
   - [Providers 模型提供商](#22-providers)
   - [Agent 核心行为](#23-agent)
   - [Security 安全策略](#24-security)
   - [Session 会话存储](#25-session)
   - [Memory 长期记忆](#26-memory)
   - [Todo 任务跟踪](#27-todo)
   - [Recall 跨会话搜索](#28-recall)
   - [Revision 上下文管理](#29-revision)
   - [Browser 浏览器自动化](#210-browser)
   - [Knowledge RAG 知识库](#211-knowledge)
   - [Workflow 工作流编排](#212-workflow)
   - [A2A Server](#213-a2a-server)
   - [AG-UI Server](#214-ag-ui-server)
   - [Eval 评估系统](#215-eval)
   - [Extensions MCP 扩展](#216-extensions)
   - [Telemetry 遥测](#217-telemetry)
   - [Observability 可观测性](#218-observability)
   - [Artifact 制品存储](#219-artifact)
3. [环境变量覆盖](#3-环境变量覆盖)
4. [CLI 参数覆盖](#4-cli-参数覆盖)
5. [常见配置场景](#5-常见配置场景)

---

## 1. 配置加载机制

```
优先级 1 (最高): CLI 参数           --provider, --model, --temperature 等
优先级 2:        环境变量            WUKONG_ 前缀（WUKONG_DEFAULT_PROVIDER）
优先级 3:        YAML 配置文件       config.yaml（搜索路径见下）
优先级 4 (最低):  内置默认值          setDefaults() in config.go
```

**配置文件搜索路径**（按优先级）:
1. `--config` CLI 参数指定的路径
2. 当前目录 `./config.yaml`
3. `~/.config/wukong/config.yaml`
4. `/etc/wukong/config.yaml`

**环境变量引用**: 支持 `${ENV_VAR}` 语法，运行时自动展开。

---

## 2. 配置项完整参考

### 2.1 全局设置

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `log_level` | string | `"info"` | 日志级别: debug / info / warn / error |
| `default_provider` | string | - | 默认 LLM 提供商名称，必须匹配 providers[].name |

---

### 2.2 Providers

```yaml
providers:
  - name: "openai"           # 唯一标识名
    type: "openai"            # openai / anthropic / google / deepseek / ollama / lmstudio
    api_key: "${OPENAI_API_KEY}"  # 支持 ${ENV_VAR}
    base_url: "https://api.openai.com/v1"
    model: "gpt-4o"           # 默认模型名
```

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | ✓ | 提供商唯一标识 |
| `type` | string | ✓ | 提供商类型，6选1 |
| `api_key` | string | - | API密钥，`${ENV_VAR}` 语法 |
| `base_url` | string | - | API端点，知名提供商自动填充 |
| `model` | string | ✓ | 默认模型名 |

**支持的 Provider 类型**:

| 类型 | 默认 Base URL | 说明 |
|------|-------------|------|
| `openai` | `https://api.openai.com/v1` | OpenAI 官方 |
| `anthropic` | `https://api.anthropic.com/v1` | Anthropic Claude |
| `google` | `https://generativelanguage.googleapis.com/v1beta/openai` | Google Gemini |
| `deepseek` | `https://api.deepseek.com/v1` | DeepSeek |
| `ollama` | `http://localhost:11434/v1` | 本地 Ollama |
| `lmstudio` | `http://localhost:1234/v1` | 本地 LM Studio |

---

### 2.3 Agent

```yaml
agent:
  max_llm_calls: 50
  max_tool_iterations: 30
  parallel_tools: true
  streaming: true
  max_run_duration: "300s"
  temperature: 0.7
  max_tokens: 4096
  tool_retry_enabled: true
  tool_retry_max_attempts: 3
  tool_retry_initial_wait: "1s"
  tool_retry_backoff_factor: 2.0
  enable_post_tool_prompt: true
  planner: ""                          # builtin / react / 空
  tool_search_enabled: false
  context_compaction: true
  session_recall_enabled: false
  json_repair_enabled: false
  todo_tool_enabled: true
  todo_enforcer_enabled: true
  agent_tools_enabled: true
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_llm_calls` | int | `50` | 每次 Run 最大 LLM API 调用次数 |
| `max_tool_iterations` | int | `30` | 工具调用循环最大迭代数 |
| `parallel_tools` | bool | `true` | 并行执行独立工具调用 |
| `streaming` | bool | `true` | 实时 Token 流式输出 |
| `max_run_duration` | duration | `"300s"` | 单次 Run 墙钟时间限制 |
| `temperature` | float | `0.7` | 采样温度 (0.0-2.0) |
| `max_tokens` | int | `4096` | 每次 API 调用最大输出 Token |
| `tool_retry_enabled` | bool | `true` | 工具调用失败自动重试 |
| `tool_retry_max_attempts` | int | `3` | 最大重试次数 |
| `tool_retry_initial_wait` | duration | `"1s"` | 首次重试前等待时间 |
| `tool_retry_backoff_factor` | float | `2.0` | 指数退避倍数 |
| `enable_post_tool_prompt` | bool | `true` | 工具执行后注入提醒 |
| `planner` | string | `""` | `"builtin"` (原生think模型) / `"react"` (标签引导) / 空(禁用) |
| `reasoning_effort` | string | - | 推理努力程度: low / medium / high (仅 builtin) |
| `tool_search_enabled` | bool | `false` | 自动 TopK 工具筛选 |
| `tool_search_max_tools` | int | `20` | 工具搜索 TopK 数量 |
| `context_compaction` | bool | `true` | 上下文压缩（两遍） |
| `context_compaction_tool_result_max_tokens` | int | `1024` | Pass1 占位符替换阈值 |
| `context_compaction_oversized_max_tokens` | int | `0` | Pass2 截断阈值（0=禁用） |
| `context_compaction_keep_recent` | int | `1` | 保护最近N个请求 |
| `context_compaction_force_clean_tools` | [string] | - | 强制占位符化的工具名列表 |
| `context_compaction_keep_tools` | [string] | - | 始终保留结果的工具名列表 |
| `session_recall_enabled` | bool | `false` | 跨会话上下文预加载 |
| `session_recall_limit` | int | `5` | 召回事件/会话数上限 |
| `json_repair_enabled` | bool | `false` | 修复非标准JSON工具参数 |
| `todo_tool_enabled` | bool | `true` | tRPC 原生 todo_write 工具 |
| `todo_enforcer_enabled` | bool | `true` | 强制完成校验插件 |
| `agent_tools_enabled` | bool | `true` | 子Agent工具（code-reviewer等） |
| `agent_tools_stream` | bool | `false` | 子Agent流式输出 |

---

### 2.4 Security

```yaml
security:
  malware_scan_enabled: true
  default_timeout: "30s"
  max_timeout: "300s"
  block_dangerous_commands: true
  blocked_commands:
    - "rm -rf /"
    - "dd if=/dev/zero"
    - "mkfs."
    - "> /dev/sda"
    - "fork bomb"
  permission_mode: "smart"          # auto / smart / manual / chat_only
  allowlist: []
  denylist: []
  guardrail_enabled: false
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `malware_scan_enabled` | bool | `true` | 扩展恶意软件扫描 |
| `default_timeout` | duration | `"30s"` | 工具执行默认超时 |
| `max_timeout` | duration | `"300s"` | 工具执行硬上限 |
| `block_dangerous_commands` | bool | `true` | 拦截危险命令 |
| `blocked_commands` | [string] | 见上 | 危险命令模式列表 |
| `permission_mode` | string | `"smart"` | auto / smart / manual / chat_only |
| `allowlist` | [string] | `[]` | 白名单（非空时仅允许列表内工具） |
| `denylist` | [string] | `[]` | 黑名单（始终阻止） |
| `guardrail_enabled` | bool | `false` | Prompt 注入检测 |

---

### 2.5 Session

```yaml
session:
  backend: "sqlite"                 # sqlite / memory / redis
  db_path: "wukong.db"              # SQLite 路径
  event_limit: 500                  # 每会话最大事件数
  ttl: "0h"                         # 会话过期时间（0=不过期）
  enable_summary: true              # 自动摘要
  summary_trigger: 50               # 触发摘要的事件数阈值
  redis_url: "redis://localhost:6379/0"  # Redis URL (backend="redis")
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `backend` | string | `"sqlite"` | 存储后端 |
| `db_path` | string | `"wukong.db"` | SQLite 数据库路径 |
| `event_limit` | int | `500` | 每会话最大事件数 |
| `ttl` | duration | `"0h"` | 会话 TTL（0=不自动过期） |
| `enable_summary` | bool | `true` | 启用自动摘要 |
| `summary_trigger` | int | `50` | 触发摘要的事件数阈值 |
| `redis_url` | string | - | Redis 连接 URL（Redis 后端必填） |

---

### 2.6 Memory

```yaml
memory:
  backend: "sqlite"
  db_path: "wukong.db"
  max_memories: 100
  auto_extract: true
  extract_timeout: "120s"
  extractor_provider: ""            # 可选独立提取模型Provider
  extractor_model: ""               # 可选独立提取模型名
  extractor_prompt: |               # 自定义提取Prompt（精简版推荐用于本地小模型）
    You are a Memory Manager...
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `backend` | string | `"sqlite"` | 存储后端 |
| `db_path` | string | `"wukong.db"` | SQLite 路径 |
| `max_memories` | int | `100` | 每用户最大记忆数 |
| `auto_extract` | bool | `true` | 自动记忆提取 |
| `extract_timeout` | duration | `"120s"` | 提取超时 |
| `extractor_provider` | string | - | 独立提取模型 Provider |
| `extractor_model` | string | - | 独立提取模型名 |
| `extractor_prompt` | string | - | 自定义提取 Prompt |

---

### 2.7 Todo

```yaml
todo:
  backend: "sqlite"
  db_path: "wukong.db"
  enable_native_todo: true          # tRPC 原生 todo_write
  enable_enforcer: true             # todoenforcer 插件
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `backend` | string | `"sqlite"` | 存储后端 |
| `db_path` | string | `"wukong.db"` | SQLite 路径 |
| `enable_native_todo` | bool | `true` | tRPC 原生 todo_write 工具 |
| `enable_enforcer` | bool | `true` | 强制完成校验 |

---

### 2.8 Recall

```yaml
recall:
  enabled: true
  backend: "sqlite"
  db_path: "wukong.db"
  max_results: 10
  max_messages_per_session: 200
  search_mode: "fts5"               # fts5 / hybrid
  embedding_provider: ""            # hybrid 模式所需
  embedding_model: ""               # "text-embedding-3-small"
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `true` | 启用跨会话搜索 |
| `max_results` | int | `10` | 最大搜索结果数 |
| `max_messages_per_session` | int | `200` | 每会话存储最大消息数 |
| `search_mode` | string | `"fts5"` | 搜索模式: fts5 / hybrid |
| `embedding_provider` | string | - | Hybrid 模式 Embedding Provider |
| `embedding_model` | string | - | Hybrid 模式 Embedding 模型 |

---

### 2.9 Revision

```yaml
revision:
  enabled: true
  revision_provider: ""
  revision_model: ""
  max_command_output: 8000
  enable_semantic_search: false
  search_strategy: "include_all"
  max_context_tokens: 64000
  trim_ratio: 0.3
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `true` | 启用上下文窗口管理 |
| `revision_provider` | string | - | 摘要模型独立 Provider |
| `revision_model` | string | - | 摘要模型名 |
| `max_command_output` | int | `8000` | 命令输出最大保留字节 |
| `max_context_tokens` | int | `64000` | 上下文 Token 软限制 |
| `trim_ratio` | float | `0.3` | 超过限制时裁剪比例 |

---

### 2.10 Browser

```yaml
browser:
  enabled: true
  browser_type: "chromium"
  headless: true
  cache_dir: ".wukong_cache"
  max_download_size: 104857600      # 100MB
  timeout: "60s"
```

---

### 2.11 Knowledge

```yaml
knowledge:
  enabled: false
  embedder_provider: ""             # Embedding Provider（空=默认LLM Provider）
  embedder_model: "text-embedding-3-small"
  sources:                          # 文档源目录
    - "./docs"
  source_urls: []                   # URL 远程文档
  vector_store: "inmemory"
  max_results: 5
  enable_source_sync: false
  reranker_enabled: false
  search_tool_name: "knowledge_search"
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 启用 RAG 知识库 |
| `embedder_provider` | string | - | Embedding Provider 名称 |
| `embedder_model` | string | `"text-embedding-3-small"` | Embedding 模型 |
| `sources` | [string] | `[]` | 本地文档源目录 |
| `source_urls` | [string] | `[]` | URL 远程文档 |
| `vector_store` | string | `"inmemory"` | 向量存储后端 |
| `max_results` | int | `5` | 每次查询返回文档数 |
| `enable_source_sync` | bool | `false` | 源文件变更自动重新索引 |
| `reranker_enabled` | bool | `false` | 结果重排序 |
| `search_tool_name` | string | `"knowledge_search"` | 注册的搜索工具名 |

---

### 2.12 Workflow

```yaml
workflow:
  mode: "single"                    # single / chain / parallel / cycle / graph
  max_iterations: 10
  cycle_mode: "default"             # 仅 mode="cycle": default / code_review
  sub_agents: []                    # 自定义子Agent
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `mode` | string | `"single"` | 工作流模式（5选1） |
| `max_iterations` | int | `10` | 最大迭代次数 |
| `cycle_mode` | string | `"default"` | Cycle 模式策略 |
| `sub_agents` | []object | `[]` | 自定义子Agent 列表 |

**sub_agents 元素**:
```yaml
- name: "custom-agent"
  instruction: "You are..."
  allowed_tools: ["tool_a", "tool_b"]
  all_tools: false
```

---

### 2.13 A2A Server

```yaml
a2a_server:
  enabled: false
  address: ":9090"
  agent_name: "wukong"
  agent_description: "Wukong AI Agent - A2A service endpoint"
```

**Summon 远程代理**:
```yaml
summon:
  a2a_remotes:
    - name: "code-reviewer"
      description: "Reviews code"
      server_url: "http://localhost:8081"
      auth_type: "api_key"
      api_key: "${A2A_API_KEY}"
```

---

### 2.14 AG-UI Server

```yaml
agui:
  enabled: false
  address: ":8080"
  path: "/agui"
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 启动 AG-UI SSE 服务器 |
| `address` | string | `":8080"` | 监听地址 |
| `path` | string | `"/agui"` | SSE 端点路径 |

---

### 2.15 Eval

```yaml
eval:
  enabled: false
  evalset_path: ".wukong_evals/default.evalset.json"
  results_path: ".wukong_evals/results.json"
  metrics:
    - name: "tool_trajectory_match"
      threshold: 0.8
    - name: "response_not_empty"
      threshold: 1.0
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `evalset_path` | string | `.wukong_evals/...` | 评估用例集 JSON 路径 |
| `results_path` | string | `.wukong_evals/...` | 结果输出路径 |
| `metrics[].name` | string | - | 指标名 |
| `metrics[].threshold` | float | - | 通过阈值 (0.0-1.0) |

**可用指标**: `tool_trajectory_match` / `response_contains_pattern` / `response_min_length` / `response_not_empty`

---

### 2.16 Extensions

```yaml
extensions: []
# 内置扩展:
#   - name: "developer"
#     type: "builtin"
#     enabled: true
#
# 外部 MCP 扩展:
#   - name: "filesystem"
#     type: "external"
#     transport: "stdio"              # stdio / sse / streamable
#     command: "npx"
#     args: ["-y", "@anthropic/mcp-server-filesystem", "/tmp"]
#     enabled: true
#     timeout: "30s"
#     mcp_broker: false              # MCP Broker 按需发现
#     mcp_tool_filter: ["read_*"]    # glob 模式 include
#     mcp_tool_exclude: ["*delete*"] # glob 模式 exclude
#     mcp_session_reconnect: true    # 断线自动重连
#     mcp_session_reconnect_attempts: 3
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `name` | string | ✓ | 扩展唯一标识 |
| `type` | string | ✓ | `"builtin"` / `"external"` |
| `transport` | string | - | 外部扩展: stdio / sse / streamable |
| `command` | string | - | stdio 可执行文件 |
| `args` | [string] | - | stdio 参数列表 |
| `url` | string | - | sse/streamable 服务地址 |
| `enabled` | bool | ✓ | 是否启用 |
| `timeout` | duration | `"30s"` | 超时 |
| `mcp_broker` | bool | `false` | MCP Broker 模式 |
| `mcp_tool_filter` | [string] | - | 工具 include glob |
| `mcp_tool_exclude` | [string] | - | 工具 exclude glob |
| `mcp_session_reconnect` | bool | `false` | 断线重连 |
| `mcp_session_reconnect_attempts` | int | `3` | 最大重连次数 |

---

### 2.17 Telemetry

```yaml
telemetry:
  enabled: false
  exporter_type: "console"          # grpc / http / console
  endpoint: "localhost:4317"        # OTLP Collector 地址
  service_name: "wukong"
  service_version: "1.0.0"
  environment: "development"        # development / staging / production
  sample_rate: 1.0                  # 0.0-1.0
```

---

### 2.18 Observability

```yaml
observability:
  langfuse_enabled: false
  langfuse_host: ""                 # Langfuse Host (不含 http://)
  langfuse_public_key: ""            # Langfuse 公钥
  langfuse_secret_key: ""            # Langfuse 密钥
```

凭证也可以通过环境变量设置: `LANGFUSE_PUBLIC_KEY` / `LANGFUSE_SECRET_KEY` / `LANGFUSE_HOST`

---

### 2.19 Artifact

```yaml
artifact:
  backend: "inmemory"               # inmemory / cos
  cos_bucket_url: ""                # COS Bucket URL
  cos_secret_id: "${COS_SECRETID}"   # Tencent Cloud SecretId
  cos_secret_key: "${COS_SECRETKEY}" # Tencent Cloud SecretKey
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `backend` | string | `"inmemory"` | 存储后端 |
| `cos_bucket_url` | string | - | COS Bucket URL (`backend="cos"`) |
| `cos_secret_id` | string | - | COS SecretId（或 `COS_SECRETID` 环境变量） |
| `cos_secret_key` | string | - | COS SecretKey（或 `COS_SECRETKEY` 环境变量） |

---

## 3. 环境变量覆盖

所有配置项可以通过 `WUKONG_` 前缀的环境变量覆盖，使用 `_` 分隔嵌套层级：

```bash
WUKONG_DEFAULT_PROVIDER=deepseek
WUKONG_AGENT_TEMPERATURE=0.5
WUKONG_AGENT_MAX_TOKENS=8192
WUKONG_SESSION_EVENT_LIMIT=1000
```

---

## 4. CLI 参数覆盖

| CLI 参数 | 覆盖配置项 |
|----------|----------|
| `--provider` / `-p` | 覆盖 `default_provider` |
| `--model` / `-m` | 覆盖 Provider 的 `model` |
| `--temperature` | 覆盖 `agent.temperature` |
| `--max-tokens` | 覆盖 `agent.max_tokens` |
| `--no-stream` | 设置 `agent.streaming = false` |
| `--session-id` / `-s` | 指定会话 ID |
| `--config` / `-c` | 指定配置文件路径 |

---

## 5. 常见配置场景

### 5.1 本地开发 (Ollama / LMStudio)

```yaml
default_provider: "ollama"
providers:
  - name: "ollama"
    type: "ollama"
    base_url: "http://localhost:11434/v1"
    api_key: "ollama"
    model: "llama3"
agent:
  streaming: true
  context_compaction: true
session:
  backend: "sqlite"
```

### 5.2 启用 Web UI (AG-UI)

```yaml
agui:
  enabled: true
  address: ":8080"
```

### 5.3 启用 RAG 知识库

```yaml
knowledge:
  enabled: true
  embedder_model: "text-embedding-3-small"
  sources:
    - "./docs"
    - "./README.md"
```

### 5.4 A2A 分布式部署

```yaml
a2a_server:
  enabled: true
  address: ":9090"
  agent_name: "wukong-worker-1"
```

### 5.5 Redis 生产会话

```yaml
session:
  backend: "redis"
  redis_url: "redis://user:pass@redis-cluster:6379/0"
  event_limit: 1000
  ttl: "24h"
```

### 5.6 COS 云端制品存储

```yaml
artifact:
  backend: "cos"
  cos_bucket_url: "https://my-bucket.cos.ap-guangzhou.myqcloud.com"
  cos_secret_id: "${COS_SECRETID}"
  cos_secret_key: "${COS_SECRETKEY}"
```

### 5.7 Langfuse 全链路追踪

```yaml
observability:
  langfuse_enabled: true
  langfuse_host: "cloud.langfuse.com"
  langfuse_public_key: "${LANGFUSE_PUBLIC_KEY}"
  langfuse_secret_key: "${LANGFUSE_SECRET_KEY}"
```
