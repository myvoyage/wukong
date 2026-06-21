# Wukong 配置参考手册

> **配置文件**: `config.yaml` | **加载器**: Viper + Cobra | **配置段**: 30+ | **配置结构体**: 38
>
> 版本：v0.6.1 | 结构体定义文件：`internal/config/config.go` (~1534 行)

---

## 配置加载机制

### 优先级链（从高到低）

```
1. CLI 参数
   ├── --provider, --model        → AI 提供商/模型覆盖
   ├── --temperature, --max-tokens → 生成参数覆盖
   ├── --no-stream                → 禁用流式输出
   ├── --config <path>            → 指定配置文件路径
   └── --debug / --quiet          → 日志级别覆盖

2. 环境变量 (WUKONG_ 前缀)
   ├── WUKONG_DEFAULT_PROVIDER     → 覆盖 default_provider
   ├── WUKONG_LOG_LEVEL            → 覆盖 log_level
   └── ...                         → 所有配置项支持

3. 配置文件（按搜索顺序）
   ├── --config 指定的路径
   ├── ./config.yaml               → 当前工作目录
   ├── ~/.config/wukong/config.yaml → 用户配置目录
   └── /etc/wukong/config.yaml     → 系统配置 (非 Windows)
```

**环境变量引用语法**: 配置文件中使用 `${ENV_VAR}`，运行时自动展开。
```yaml
api_key: "${OPENAI_API_KEY}"  # → 自动从环境变量读取
```

---

## 1. 全局设置

```yaml
# 日志级别: debug | info | warn | error
# CLI --debug 覆盖为 debug, --quiet 覆盖为 error
log_level: "info"

# 默认 AI 提供商（必须与 providers 列表中某 name 匹配）
default_provider: "lmstudio"
```

### 全局轻量模型

后台任务（记忆提取、上下文压缩、知识图谱构建、检索规划）共享的轻量模型配置：

```yaml
lightweight_provider: "lmstudio"
lightweight_model: "gemma-4-e4b-it"
```

**回退链**: 各子系统 `extractor_model` / `planner_model` / `revision_model` 未指定时 → 使用此全局轻量模型。

### 系统路径

```yaml
# 项目工作目录追踪文件
project_dir: "~/.config/wukong/"
```

---

## 2. Providers — AI 模型提供商

支持的 provider 类型：`openai` | `anthropic` | `google` | `deepseek` | `ollama` | `lmstudio` | `acp`

```yaml
providers:
  - name: "openai"                    # 唯一标识（必填）
    type: "openai"                    # 提供商类型（必填）
    api_key: "${OPENAI_API_KEY}"      # API 密钥（必填，支持环境变量）
    base_url: "https://api.openai.com/v1" # API 基础 URL
    model: "gpt-4o"                   # 默认模型名

  - name: "anthropic"
    type: "anthropic"
    api_key: "${ANTHROPIC_API_KEY}"
    base_url: "https://api.anthropic.com"
    model: "claude-sonnet-4-20250514"

  - name: "google"
    type: "google"
    api_key: "${GOOGLE_API_KEY}"
    model: "gemini-2.5-flash"

  - name: "deepseek"
    type: "deepseek"
    api_key: "${DEEPSEEK_API_KEY}"
    base_url: "https://api.deepseek.com"
    model: "deepseek-chat"

  - name: "ollama"
    type: "ollama"
    api_key: "ollama"                 # 本地可任意填
    base_url: "http://localhost:11434/v1"
    model: "llama3"

  - name: "lmstudio"
    type: "lmstudio"
    api_key: "lmstudio"
    base_url: "http://localhost:1234/v1"
    model: "google/gemma-4-26b-a4b"

  - name: "acp-coder"                 # ACP 代理客户端协议
    type: "acp"
    agent_url: "http://localhost:4000" # ACP Agent 端点
    model: "acp-default"
```

---

## 3. Agent — 核心行为配置

```yaml
agent:
  # === 执行限制 ===
  max_llm_calls: 50             # 单次运行最大 LLM 调用次数
  max_tool_iterations: 30       # 单次运行最大工具调用迭代次数
  max_run_duration: "300s"      # 单次运行最大时长 (支持 s/m/h)
  parallel_tools: true          # 是否允许并行工具调用
  streaming: true               # 是否启用流式输出

  # === 生成参数 ===
  temperature: 0.7              # 生成温度 [0.0, 2.0]
  max_tokens: 4096              # 最大生成 token 数
  # reasoning_effort: "medium"  # 仅 builtin planner: low / medium / high

  # === 工具重试 ===
  tool_retry_enabled: true
  tool_retry_max_attempts: 3
  tool_retry_initial_wait: "1s"
  tool_retry_backoff_factor: 2.0  # 指数退避系数

  # === Planner (规划器) ===
  # builtin: 适合支持原生 thinking 的模型 (Claude/Gemini/OpenAI o-series)
  # react:   适合不支持原生 thinking 的模型 (通过标签引导思考)
  # "":      不启用 planner
  planner: ""
  enable_post_tool_prompt: true  # 工具调用后注入规划提示

  # === Tool Search (工具筛选) ===
  tool_search_enabled: true       # 自动筛选相关工具，减少 token 消耗
  # tool_search_max_tools: 20     # 每次最多提供给 LLM 的工具数

  # === Context Compaction (上下文压缩) ===
  context_compaction: true        # 自动截断过长工具结果
  # context_compaction_tool_result_max_tokens: 1024  # Pass1 阈值
  # context_compaction_oversized_max_tokens: 8192    # Pass2 阈值
  # context_compaction_keep_recent: 1                # 保留最近 N 个请求
  # context_compaction_force_clean_tools             # 强制占位符化的工具
  #   - "shell"
  #   - "grep"
  # context_compaction_keep_tools                   # 始终保留结果的工具
  #   - "memory_search"
  #   - "session_search"

  # === Session Recall (跨会话上下文预加载) ===
  session_recall_enabled: false

  # === JSON Repair ===
  json_repair_enabled: true       # 自动修复 LLM 输出的格式错误 JSON

  # === Todo ===
  todo_tool_enabled: true         # 启用 todo 工具集
  todo_enforcer_enabled: true     # 启用 todo 强制执行器 (确保完成)

  # === Agent Tools (子 Agent 工具) ===
  agent_tools_enabled: false      # 是否暴露子 Agent 作为工具

  # === 提示模板 ===
  system_prompt_dir: "~/.config/wukong/prompts/" # 系统提示模板目录

  # === Recipe 系统 ===
  recipe_enabled: true             # 启用 recipe 子 Agent
  # recipe_dir: ".wukong/recipes/" # 配方文件目录
  # inline_recipes:                # P2-A: 内联配方（直接在配置中定义）
  #   - name: quick-helper
  #     description: "Quick helper"
  #     instruction: "You are a quick helper."
  #     tools: [file_read]
  #     temperature: 0.3
```

---

## 4. Security — 安全策略

```yaml
security:
  # === 权限模式 ===
  # auto:       自动批准所有工具 (适合受信任环境)
  # smart:      根据 allowlist/denylist 智能决策
  # manual:     所有工具调用需要人工确认
  # chat_only:  完全禁止工具调用
  permission_mode: "smart"

  # === 安全控制 ===
  malware_scan_enabled: true       # 文件内容威胁扫描
  block_dangerous_commands: true   # 阻止危险命令执行
  guardrail_enabled: false         # Prompt 注入检测 (启用增加延迟)

  # === 超时 ===
  default_timeout: "30s"           # 工具执行默认超时
  max_timeout: "300s"              # 工具执行最大超时

  # === 黑白名单 ===
  allowlist: []                    # 工具白名单 (支持通配符，如 "file_*")
  denylist: []                     # 工具黑名单 (支持通配符)
  blocked_commands:                # 危险命令模式匹配列表
    - "rm -rf /"
    - "dd if=/dev/zero"
    - "mkfs."
    - "> /dev/sda"

  # === 文件黑名单 ===
  ignore_file_enabled: true        # 是否启用 .wukongignore
  ignore_file: ".wukongignore"     # 忽略文件名
```

### 权限模式详解

| 模式 | 行为 |
|------|------|
| `auto` | 所有工具自动批准，无需用户确认 |
| `smart` | 根据 allowlist/denylist + threat 扫描结果智能决策 |
| `manual` | 每次工具调用弹出确认提示，用户手动批准/拒绝 |
| `chat_only` | 禁止所有工具调用，Agent 只能进行纯文本对话 |

---

## 5. Session — 会话存储

```yaml
session:
  backend: "sqlite"               # sqlite | memory | redis
  db_path: "wukong.db"            # SQLite 路径 (仅 sqlite 后端)
  event_limit: 500                # 单个会话最大事件数
  ttl: "0h"                       # 会话过期时间 (0 = 不过期)
  enable_summary: true            # 是否启用会话摘要
  summary_trigger: 50             # 触发摘要的事件数阈值
```

### Redis 后端配置

```yaml
session:
  backend: "redis"
  redis_url: "redis://localhost:6379/0"
  redis_password: ""
  redis_db: 0
```

---

## 6. Memory — 长期记忆 (tRPC Memory)

```yaml
memory:
  backend: "sqlite"               # sqlite | memory
  db_path: "wukong.db"
  max_memories: 100               # 最大记忆数 (0=不限制)

  # === Auto Extract (自动提取) ===
  auto_extract: true              # 每轮对话后异步 LLM 提取事实
  extract_timeout: "600s"         # 提取超时时间
  extractor_provider: "lmstudio"  # 独立提取模型 provider
  # extractor_model: ""           # 未指定 → 使用 lightweight_model

  # === 自定义提取 Prompt ===
  extractor_prompt: |
    You are a Memory Manager responsible for managing information about the user.
    Extract key facts, preferences, decisions, and personal notes from the conversation.
    Return the extracted facts in JSON format.

  # === Smart Cleanup ===
  enable_smart_cleanup: true      # 启用容量感知淘汰
  cleanup_trigger_threshold: 0.8  # 触发阈值 (80%)
  cleanup_target_threshold: 0.6   # 淘汰目标 (60%)
  memory_ttl: "720h"              # 记忆 TTL (30 天)
```

**SmartCleanup 机制**:
- `< 80%` 容量: 仅删除 TTL 过期记忆
- `≥ 80%` 容量: 评分淘汰 (70% recency + 30% content length)，降至 60%
- `max_memories: 0` 表示不限制容量

---

## 7. 记忆子系统配置

### 7.1 Recall — 跨会话对话搜索

```yaml
recall:
  enabled: true                     # 是否启用
  backend: "sqlite"                 # sqlite
  db_path: "wukong.db"
  max_results: 10                   # 单次搜索最大结果
  max_messages_per_session: 200     # 单会话最大存储消息数
  search_mode: "fts5"               # fts5 | hybrid (hybrid 需要 Embedding)
```

### 7.2 CortexDB — 智能回溯与知识存储

```yaml
cortex:
  enabled: true
  db_path: "wukong.db"
  max_results: 10
  max_messages_per_session: 200
  # Embedding 配置 (hybrid 模式需要)
  embedding_base_url: "http://localhost:1234"
  embedding_api_key: "lmstudio"
  embedding_model: "qwen3-embedding-0.6b-graphql"
```

### 7.3 MemoryFlow — 转录记录与语义唤醒

```yaml
memoryflow:
  enabled: true
  db_path: "wukong.db"
  namespace: "assistant"            # CortexDB 命名空间
  embedding_dimensions: 0           # 0 = 自动检测
  # planner_model: ""               # 检索规划模型 → 回退到 lightweight_model
  # extractor_model: ""             # 事实提取模型 → 回退到 lightweight_model
```

### 7.4 GraphFlow — 知识图谱构建

```yaml
graphflow:
  enabled: true
  db_path: "wukong.db"
  max_chars_per_doc: 8000           # 单文档最大字符数
  auto_extract: true                # 每轮对话后自动提取实体/关系
  # extractor_model: ""             # 图谱提取模型 → 回退到 lightweight_model
```

### 7.5 ImportFlow — 结构化数据导入

```yaml
importflow:
  enabled: true
  db_path: "wukong.db"
```

### 7.6 Todo — 任务管理

```yaml
todo:
  backend: "sqlite"
  db_path: "wukong.db"
  enable_native_todo: true         # 启用 todo 工具
  enable_enforcer: true            # 启用 todo 强制执行器
```

---

## 8. Revision — 上下文压缩

```yaml
revision:
  enabled: true
  revision_provider: "lmstudio"     # 压缩模型 provider
  # revision_model: ""             # 未指定 → 使用 lightweight_model
  enable_llm_summarize: true        # 启用 LLM 智能摘要
  summary_cooldown: 120s            # 两次摘要间的最小间隔
  summary_timeout: 30s              # 摘要生成超时
  max_command_output: 8000          # 命令输出最大保留字符
  enable_semantic_search: false     # 启用语义搜索辅助
  search_strategy: "include_all"    # 搜索策略
  max_context_tokens: 64000         # 上下文最大 token 数
  trim_ratio: 0.3                   # 算法截断比例
```

---

## 9. Browser — 浏览器自动化

```yaml
browser:
  enabled: true
  browser_type: "chromium"          # 浏览器类型
  headless: true                    # 无头模式
  search_backend: "duckduckgo"      # 搜索后端: duckduckgo | searxng | tavily
  timeout: "60s"                    # 浏览器操作超时
```

---

## 10. CodeMode — JS 沙箱执行

```yaml
code_mode:
  enabled: true
  timeout: "10s"                    # JS 执行超时 (context.Timeout)
  max_memory_mb: 128                # Go 运行时内存限制 (debug.SetMemoryLimit)
```

**goja JS 沙箱安全措施** (由 `codemode/executor.go` 自动执行):
| 措施 | 实现 |
|------|------|
| API 白名单 | console / JSON / Math / __output |
| 完全禁用 | eval / Function / setInterval / Date / RegExp |
| 内存限制 | `debug.SetMemoryLimit(128MB)` |
| 并发控制 | channel semaphore (max 5) |
| JSON 保护 | `JSON.parse` 1MB 输入限制 |

---

## 11. Visualiser / Tutorial / TopOfMind / Apps

```yaml
auto_visualiser:
  enabled: true
  output_dir: ".wukong/visualisations"

tutorial:
  enabled: true
  # 内置教程: git, docker, go 等

top_of_mind:
  enabled: true
  max_chars: 4096                   # 指令最大字符数
  storage_path: ".wukong/top_of_mind.txt"

apps:
  enabled: true
  apps_dir: ".wukong/apps"
```

---

## 12. Summon / Skill / Evolution

```yaml
summon:
  enabled: true
  skills_dir: ".wukong/skills"      # Skill 文件目录
  max_concurrent: 5                 # 最大并发子代理
  a2a_remotes: []                   # 远程 A2A 代理列表
  # - name: "remote-coder"
  #   url: "http://other-host:9090"
  #   transport: "http"

skill:
  enabled: true
  skills_dir: ".wukong/skills"
  auto_load: true                   # 启动时自动加载所有 Skill
  max_skills: 20                    # 最大 Skill 数量

evolution:
  enabled: false                    # 是否启用技能自进化 (实验性)
  auto_patch: false                 # 是否自动应用补丁 (false=仅生成建议)
  min_confidence: 0.7               # 最低置信度阈值 [0.0, 1.0]
  cooldown_period: "30m"            # 同一技能两次进化间冷却时间
  max_patches_per_day: 10           # 全局每日最大补丁数
  max_patch_size: 8192              # 单个补丁最大字节数 (8KB)
  max_versions_kept: 10             # 保留的历史版本数
```

---

## 13. Knowledge — RAG 知识库

```yaml
knowledge:
  enabled: false
  embedding_model: "qwen3-embedding-0.6b-graphql"
  embedding_base_url: "http://localhost:1234"
  embedding_api_key: "lmstudio"
  sources: []                       # 知识源列表
  # - type: "directory"
  #   path: "./docs/"
  # - type: "url"
  #   url: "https://example.com/docs"
```

---

## 14. Workflow / Dify / Team

```yaml
workflow:
  # 10 种模式:
  #   single / chain / parallel / cycle / graph
  #   team_coordinator / team_swarm
  #   claude_code / codex
  #   dify
  mode: "single"
  max_iterations: 10                # cycle 模式最大迭代次数

  # Chain 模式配置
  chain_agents: []                  # 链式 Agent 名称列表
  # parallel_agents: []             # 并行 Agent 名称列表

  # Team 模式配置
  team_members: []
  # - name: "coder"
  #   provider: "lmstudio"
  #   model: "google/gemma-4-26b-a4b"
  #   system_prompt: "You are an expert Go developer."

dify:
  enabled: false
  base_url: "https://api.dify.ai/v1"
  api_key: "${DIFY_API_KEY}"

  # Dify 工作流子代理
  sub_agents: []
  # - name: "analysis-workflow"
  #   app_id: "your-app-id"
  #   api_key: "${DIFY_APP_KEY}"
```

---

## 15. 服务端点配置

### 15.1 A2A Server — Agent-to-Agent 通信 (端口 9090)

```yaml
a2a_server:
  enabled: true
  address: ":9090"
  # 可选端点: /a2a/message, /a2a/agent-card
  # 实现: internal/summon/a2a.go
```

### 15.2 AG-UI — Web UI 实时对话 (端口 8080)

```yaml
agui:
  enabled: true
  address: ":8080"
  path: "/agui"
  # 使用 SSE (Server-Sent Events) 推送流式响应
  # 实现: internal/server/agui.go
```

### 15.3 ACP Server — Agent Client Protocol (端口 9091)

```yaml
acp_server:
  enabled: true
  address: ":9091"
  path: "/acp"
  # 实现: internal/server/acp.go
```

### 15.4 ACP MCP Bridge — 跨协议工具桥接 (端口 3400)

```yaml
acp_mcp:
  enabled: true
  address: ":3400"
  path: "/mcp"
  # 将 Wukong 内置扩展暴露为 MCP Server
  # 实现: internal/extension/acp_mcp.go
```

---

## 16. Telemetry / Observability / Artifact / Project

```yaml
telemetry:
  enabled: false                    # 启用 OpenTelemetry 分布式追踪
  exporter: "console"               # console | grpc | http
  endpoint: ""                      # gRPC/HTTP 导出端点
  sample_rate: 1.0                  # 采样率 [0.0, 1.0]
  # 实现: internal/telemetry/telemetry.go

observability:
  langfuse_enabled: false           # 启用 Langfuse LLM 追踪
  langfuse_host: ""
  langfuse_public_key: ""
  langfuse_secret_key: ""
  # 实现: internal/observability/langfuse.go

artifact:
  backend: "inmemory"               # inmemory | cos
  # cos_endpoint: ""                # 腾讯云 COS 端点
  # cos_secret_id: "${COS_SECRET_ID}"
  # cos_secret_key: "${COS_SECRET_KEY}"
  # cos_bucket: ""
  # 实现: internal/artifact/factory.go

project:
  enabled: true
  storage_path: "~/.config/wukong/projects.json"
  max_projects: 100                 # 最大追踪项目数
  # 实现: internal/project/manager.go
```

---

## 17. Extension — 扩展系统

```yaml
extensions:
  enabled: []                       # 启用的扩展列表 (空=全部启用)
  disabled: []                      # 禁用的扩展列表
  tool_permissions:                 # 细粒度工具权限
    - tool_name: "command_execute"
      allow: true
    - tool_name: "file_delete"
      allow: false
  # 实现: internal/extension/manager.go
```

---

## 18. Health — 健康检查

```yaml
health:
  enabled: true
  # 检查项: DB / Model / Extension / A2A Server
  # 端点: /health (综合), /health/live (存活), /health/ready (就绪)
  # 实现: internal/health/health.go
```

---

## 19. Eval — Agent 评估

```yaml
eval:
  evalset_path: ""                  # 评测集 JSON 文件路径
  metrics:                          # 启用的评估指标
    - tool_trajectory_match
    - response_contains_pattern
    - response_min_length
    - response_not_empty
  # 实现: internal/eval/eval.go
  # CLI: wukong eval --evalset ./eval_set.json
```

---

## 推荐配置方案

### 方案一：最小配置（开箱即用）

适用于首次体验，仅需 LMStudio 本地模型：

```yaml
default_provider: "lmstudio"
lightweight_provider: "lmstudio"
lightweight_model: "gemma-4-e4b-it"
providers:
  - name: "lmstudio"
    type: "lmstudio"
    base_url: "http://localhost:1234/v1"
    api_key: "lmstudio"
    model: "google/gemma-4-26b-a4b"
```

### 方案二：完整记忆配置

启用双引擎记忆闭环的所有功能：

```yaml
default_provider: "lmstudio"
lightweight_provider: "lmstudio"
lightweight_model: "gemma-4-e4b-it"
providers:
  - name: "lmstudio"
    type: "lmstudio"
    base_url: "http://localhost:1234/v1"
    api_key: "lmstudio"
    model: "google/gemma-4-26b-a4b"

memory:
  auto_extract: true
  max_memories: 100
  extractor_provider: "lmstudio"

cortex:
  enabled: true
  embedding_base_url: "http://localhost:1234"
  embedding_model: "qwen3-embedding-0.6b-graphql"

memoryflow:
  enabled: true

graphflow:
  enabled: true
  auto_extract: true

importflow:
  enabled: true

recall:
  enabled: true
  search_mode: "fts5"           # 无 embedding 时使用

revision:
  enabled: true
  revision_provider: "lmstudio"
  enable_llm_summarize: true
```

### 方案三：安全增强配置

推荐在对外暴露时使用：

```yaml
security:
  permission_mode: "smart"
  block_dangerous_commands: true
  malware_scan_enabled: true
  ignore_file_enabled: true
  allowlist:
    - "file_read"
    - "code_search"
    - "list_directory"
    - "memory_*"
    - "recall_*"

agent:
  todo_enforcer_enabled: true
  json_repair_enabled: true

code_mode:
  enabled: true
  timeout: "10s"
  max_memory_mb: 128
```

### 方案四：全功能配置（生产环境推荐）

```yaml
log_level: "info"
default_provider: "lmstudio"
lightweight_provider: "lmstudio"
lightweight_model: "gemma-4-e4b-it"

providers:
  - name: "lmstudio"
    type: "lmstudio"
    base_url: "http://localhost:1234/v1"
    api_key: "lmstudio"
    model: "google/gemma-4-26b-a4b"

agent:
  max_llm_calls: 50
  max_tool_iterations: 30
  max_run_duration: "300s"
  parallel_tools: true
  streaming: true
  temperature: 0.7
  max_tokens: 4096
  tool_retry_enabled: true
  tool_search_enabled: true
  context_compaction: true
  todo_tool_enabled: true
  todo_enforcer_enabled: true
  json_repair_enabled: true

security:
  permission_mode: "smart"
  malware_scan_enabled: true
  block_dangerous_commands: true
  ignore_file_enabled: true

memory:
  auto_extract: true
  max_memories: 100
  extractor_provider: "lmstudio"

cortex:
  enabled: true
  embedding_base_url: "http://localhost:1234"
  embedding_model: "qwen3-embedding-0.6b-graphql"

memoryflow:
  enabled: true

graphflow:
  enabled: true
  auto_extract: true

importflow:
  enabled: true

recall:
  enabled: true

revision:
  enabled: true
  revision_provider: "lmstudio"
  enable_llm_summarize: true

code_mode:
  enabled: true
  timeout: "10s"
  max_memory_mb: 128

browser:
  enabled: true
  search_backend: "duckduckgo"

a2a_server:
  enabled: true

agui:
  enabled: true

acp_server:
  enabled: true

acp_mcp:
  enabled: true
```

---

## 配置项索引

| 配置段 | 配置结构体 | 核心文件 |
|--------|-----------|----------|
| 全局 | `WukongConfig` | config/config.go |
| Providers | `ProviderConfig` | provider/factory.go |
| Agent | `AgentConfig` | agent/loop.go |
| Security | `SecurityConfig` | security/guard.go |
| Session | `SessionConfig` | session/ |
| Memory | `MemoryConfig` | memory/store.go |
| Recall | `RecallConfig` | recall/store.go |
| CortexDB | `CortexConfig` | cortex/store.go |
| MemoryFlow | `MemoryFlowConfig` | cortex/memoryflow.go |
| GraphFlow | `GraphFlowConfig` | cortex/graphflow.go |
| ImportFlow | `ImportFlowConfig` | cortex/import_flow.go |
| Todo | `TodoConfig` | todo/tool.go |
| Revision | `RevisionConfig` | agent/context.go |
| Browser | `BrowserConfig` | browser/controller.go |
| CodeMode | `CodeModeConfig` | codemode/executor.go |
| Visualiser | `VisualiserConfig` | extension/builtin/auto_visualiser.go |
| Tutorial | `TutorialConfig` | extension/builtin/tutorial.go |
| TopOfMind | `TopOfMindConfig` | topofmind/mind.go |
| Apps | `AppsConfig` | apps/manager.go |
| Summon | `SummonConfig` | summon/delegate.go |
| Skill | `SkillConfig` | skill/manager.go |
| Evolution | `EvolutionConfig` | evolution/engine.go |
| Knowledge | `KnowledgeConfig` | knowledge/manager.go |
| Workflow | `WorkflowConfig` | agent/workflow.go |
| Dify | `DifyConfig` | agent/dify.go |
| Team | `TeamMemberConfig` | agent/team.go |
| A2A Server | `A2AServerConfig` | summon/a2a.go |
| AG-UI | `AGUIConfig` | server/agui.go |
| ACP Server | `ACPServerConfig` | server/acp.go |
| ACP MCP | `ACPMCPConfig` | extension/acp_mcp.go |
| Eval | `EvalConfig` | eval/eval.go |
| Artifact | `ArtifactConfig` | artifact/factory.go |
| Telemetry | `TelemetryConfig` | telemetry/telemetry.go |
| Observability | `ObservabilityConfig` | observability/langfuse.go |
| Extension | `ExtensionConfig` | extension/manager.go |
| Project | (project_dir) | project/manager.go |
