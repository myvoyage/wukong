# Wukong 配置参考手册

> **配置文件**: `config.yaml` | **加载器**: Viper + Cobra | **配置段**: 32 | **结构体**: 39
>
> 版本: v0.1.13 | Go: 1.26 | 定义: `internal/config/config.go` (70KB)

---

## 加载优先级（7 级）

```
1. CLI 命令行参数        (--provider, --model, --temperature, --max-tokens, --config)
2. 环境变量              (WUKONG_ 前缀, 如 WUKONG_DEFAULT_PROVIDER)
3. --config CLI 指定文件
4. ./config.yaml        (当前目录)
5. ~/.config/wukong/config.yaml
6. /etc/wukong/config.yaml (非 Windows)
7. 内置默认值
```

环境变量引用语法: `${ENV_VAR}` 运行时自动展开，如 `api_key: "${OPENAI_API_KEY}"`

---

## 1. 全局设置

```yaml
log_level: "info"                    # 日志级别: debug | info | warn | error
default_provider: "lmstudio"         # 默认 LLM Provider 名称

# 轻量模型 — 后台任务共享的廉价模型
# 回退链: 子系统.extractor_model → lightweight_model → default_provider
lightweight_provider: "lmstudio"
lightweight_model: "gemma-4-e4b-it"
```

### 模型回退链

```
子系统.extractor_model (优先级最高)
  → lightweight_model (全局轻量模型)
  → default_provider (全局默认，最终回退)
```

---

## 2. Providers — AI 模型提供商

支持 7 种 Provider 类型: `openai | anthropic | google | deepseek | ollama | lmstudio | acp`

```yaml
providers:
  - name: "openai"                  # Provider 唯一名称（需与 default_provider 对应）
    type: "openai"                  # 提供商类型
    api_key: "${OPENAI_API_KEY}"    # API 密钥（支持环境变量）
    base_url: "https://api.openai.com/v1"
    model: "gpt-4o"                # 默认模型

  - name: "anthropic"
    type: "anthropic"
    api_key: "${ANTHROPIC_API_KEY}"
    base_url: "https://api.anthropic.com"
    model: "claude-sonnet-4-20250514"

  - name: "google"
    type: "google"
    api_key: "${GOOGLE_API_KEY}"
    base_url: "https://generativelanguage.googleapis.com"
    model: "gemini-2.5-pro"

  - name: "deepseek"
    type: "deepseek"
    api_key: "${DEEPSEEK_API_KEY}"
    base_url: "https://api.deepseek.com"
    model: "deepseek-chat"

  - name: "ollama"
    type: "ollama"
    api_key: "ollama"
    base_url: "http://localhost:11434/v1"
    model: "llama3"

  - name: "lmstudio"
    type: "lmstudio"
    api_key: "lmstudio"
    base_url: "http://localhost:1234/v1"
    model: "google/gemma-4-26b-a4b"

  - name: "acp-coder"
    type: "acp"
    agent_url: "http://localhost:4000"
    model: "acp-default"
```

### Provider 类型对比

| 类型 | SDK | 特点 | 适用场景 |
|------|-----|------|----------|
| `openai` | openai-go | GPT 系列 | 通用最佳 |
| `anthropic` | openai-go (兼容) | Claude 系列 | 代码/分析 |
| `google` | openai-go (兼容) | Gemini 系列 | 多模态 |
| `deepseek` | openai-go (兼容) | 国产高性价比 | 成本敏感 |
| `ollama` | openai-go (兼容) | 本地开源 | 隐私优先 |
| `lmstudio` | openai-go (兼容) | 本地 GUI 服务 | 开发测试 |
| `acp` | HTTP Client | Agent 代理 | 远程 Agent |

---

## 3. Agent — 核心行为与执行策略

```yaml
agent:
  # 执行限制
  max_llm_calls: 50                  # 单轮最大 LLM 调用次数
  max_tool_iterations: 30            # 单轮最大工具迭代次数
  max_run_duration: "300s"           # 单次运行最大时长
  parallel_tools: true               # 并行工具调用
  streaming: true                    # 流式输出

  # 生成参数
  temperature: 0.7                   # 温度参数 (0.0-2.0)
  max_tokens: 4096                   # 最大生成 token 数
  # reasoning_effort: "medium"       # 推理深度 (仅支持的模型)

  # 工具重试（指数退避）
  tool_retry_enabled: true           # 启用工具调用失败重试
  tool_retry_max_attempts: 3         # 最大重试次数
  tool_retry_initial_wait: "1s"      # 初始等待时间
  tool_retry_backoff_factor: 2.0     # 退避因子 (1s→2s→4s)
  enable_post_tool_prompt: true      # 工具调用后注入提示

  # Planner 规划器
  planner: ""                        # builtin | react | ""

  # Tool Search 自动工具筛选
  tool_search_enabled: true          # 大量工具时自动筛选相关工具

  # Context Compaction 上下文压缩
  context_compaction: true           # 上下文窗口管理

  # Session Recall 跨会话预加载
  session_recall_enabled: false

  # JSON Repair
  json_repair_enabled: false         # 自动修复 LLM 输出的 JSON 错误

  # Todo 任务管理
  todo_tool_enabled: true            # 启用 todo_write 工具
  todo_enforcer_enabled: true        # 强制 Agent 维护 TODO 列表

  # Agent Tools 子 Agent 工具
  agent_tools_enabled: false         # 启用 Recipe 子 Agent 工具

  # 提示模板
  system_prompt_dir: "~/.config/wukong/prompts/"
```

### 工具重试退避公式

```
等待时间 = tool_retry_initial_wait × backoff_factor^(attempt-1)
例: 1s × 2.0^0=1s → 1s×2.0^1=2s → 1s×2.0^2=4s
```

---

## 4. Security — 安全策略

```yaml
security:
  permission_mode: "smart"           # auto | smart | manual | chat_only
  require_approval: false
  malware_scan_enabled: true
  block_dangerous_commands: true
  blocked_commands:
    - "rm -rf /"
    - "dd if=/dev/zero"
    - "mkfs."
    - "> /dev/sda"
    - "fork bomb"
  default_timeout: "30s"            # 默认工具执行超时
  max_timeout: "300s"               # 最大工具执行超时
  allowlist: []                     # 工具 allowlist (空=全部允许)
  denylist: []                      # 工具 denylist
  guardrail_enabled: false          # Prompt 注入检测
  ignore_file_enabled: true         # .wukongignore 文件黑名单
  ignore_file: ".wukongignore"      # 忽略文件路径
```

### 权限模式行为

| 模式 | 读文件 | 写工作目录 | 修改外部文件 | 执行命令 | 安装软件 |
|------|--------|-----------|-------------|----------|----------|
| `auto` | ✓ 自动 | ✓ 自动 | ✓ 自动 | 需确认 | 需确认 |
| `smart` | ✓ 自动 | ✓ 自动 | 需确认 | 需确认 | 需确认 |
| `manual` | 需确认 | 需确认 | 需确认 | 需确认 | 需确认 |
| `chat_only` | ✗ 禁止 | ✗ 禁止 | ✗ 禁止 | ✗ 禁止 | ✗ 禁止 |

---

## 5. Session — 会话持久化

```yaml
session:
  backend: "sqlite"                  # sqlite | memory | redis
  db_path: "wukong.db"
  event_limit: 500                   # 单会话最大事件数
  ttl: "0h"                          # 会话 TTL (0h=永不过期)
  enable_summary: true               # 自动生成会话摘要
  summary_trigger: 50                # 触发摘要的事件数阈值
```

### Session 后端对比

| 后端 | 持久化 | 分布式 | 适用场景 |
|------|--------|--------|----------|
| `sqlite` | ✓ | ✗ | 本地部署（默认） |
| `memory` | ✗ | ✗ | 测试/临时 |
| `redis` | ✓ | ✓ | 多实例/生产 |

---

## 6. Memory — 长期记忆（tRPC Memory）

```yaml
memory:
  backend: "sqlite"
  db_path: "wukong.db"
  max_memories: 100                  # 最大记忆数量

  # AutoExtract 异步 LLM 提取事实
  auto_extract: true                 # 自动从对话提取记忆
  extract_timeout: "600s"            # 提取超时
  extractor_provider: "lmstudio"     # 提取用 Provider
  # extractor_model: ""              # 提取用模型 (空=使用 lightweight_model)

  # SmartCleanup 容量淘汰
  enable_smart_cleanup: true         # 启用智能清理
  cleanup_trigger_threshold: 0.8    # 80% 触发
  cleanup_target_threshold: 0.6     # 60% 目标
  # 排序: 70% 新鲜度 + 30% 长度

  memory_ttl: "720h"                # 记忆过期时间 (720h=30天)
  extractor_prompt: |
    You are a Memory Manager. Extract concise memories from the conversation.
    Today's date is {current_date}.

    <rules>
    1. Atomicity: One fact per memory.
    2. No subject prefix: Never start with "User", "The user", etc.
    3. Deduplication: Check existing memories before adding.
    4. Specificity: Include names, dates, places, quantities.
    5. Episode vs Fact: Episode = events with time, Fact = stable attributes.
    6. All speakers: Extract info about EVERY person.
    7. Skip transient greetings or trivial queries.
    </rules>
```

### SmartCleanup 工作流

```
容量 > 80% → 触发清理
  → 按权重排序: freshness_score × 0.7 + length_score × 0.3
  → 淘汰到 60%，保留排序最高的记忆
```

---

## 7. Todo — 任务追踪

```yaml
todo:
  backend: "sqlite"
  db_path: "wukong.db"
  enable_native_todo: true           # 启用原生 TODO 工具
  enable_enforcer: true              # 启用 TODO 强制执行器
```

---

## 8-12. CortexDB 记忆栈

### 8. Recall — FTS5 全文搜索

```yaml
recall:
  enabled: true
  backend: "sqlite"
  db_path: "wukong.db"
  max_results: 10                    # 最大返回结果数
  max_messages_per_session: 200      # 每会话最大存储消息数
  search_mode: "fts5"               # fts5 | hybrid
```

### 9. CortexDB — HNSW 向量 + 全文搜索

```yaml
cortex:
  enabled: true
  db_path: "wukong.db"
  max_results: 10
  max_messages_per_session: 200
  embedding_base_url: "http://localhost:1234"       # Embedding API 地址
  embedding_api_key: "lmstudio"
  embedding_model: "qwen3-embedding-0.6b-graphql"   # Embedding 模型
```

### 10. MemoryFlow — 转录记录 + 语义唤醒

```yaml
memoryflow:
  enabled: true
  db_path: "wukong.db"
  namespace: "assistant"             # 命名空间
  embedding_dimensions: 0            # 0=自动检测
```

**3 层唤醒上下文**:
1. 身份层 — Agent 持久身份定义
2. 回忆层 — 从历史中检索的相关信息
3. 当前会话层 — 最近的对话上下文

### 11. GraphFlow — 知识图谱

```yaml
graphflow:
  enabled: true
  db_path: "wukong.db"
  max_chars_per_doc: 8000            # 单个文档最大字符数
  auto_extract: true                 # 每轮对话自动提取实体/关系
```

### 12. ImportFlow — 结构化数据导入

```yaml
importflow:
  enabled: true
  db_path: "wukong.db"
```

---

## 13. Revision — 上下文窗口管理（3 层压缩）

```yaml
revision:
  enabled: true
  revision_provider: "lmstudio"      # 压缩用 Provider
  enable_llm_summarize: true         # LLM 摘要
  summary_cooldown: 120s             # 摘要冷却时间
  summary_timeout: 30s               # 摘要超时
  max_command_output: 8000           # 命令输出最大字符数
  enable_semantic_search: false      # 语义搜索补充
  search_strategy: "include_all"    # 搜索策略
  max_context_tokens: 64000          # 上下文窗口最大 token
  trim_ratio: 0.3                    # 裁剪比例 (30%)
```

### 3 层压缩流程

```
1. Trim (裁剪)       — 删除旧消息，保留最近 N 轮
2. LLM Summarize     — 用轻量模型生成对话摘要（cooldown 120s）
3. Semantic Search   — (可选) 语义检索补充关键信息
```

---

## 14-19. 功能工具

### 14. Browser — 浏览器自动化

```yaml
browser:
  enabled: true
  browser_type: "chromium"           # 浏览器类型
  headless: true                     # 无头模式
  cache_dir: ".wukong/cache"
  max_download_size: 104857600       # 最大下载大小 (100MB)
  timeout: "60s"                     # 操作超时
  viewport_width: 1280               # 视口宽度
  viewport_height: 720               # 视口高度
  search_backend: "duckduckgo"       # duckduckgo | searxng | tavily
```

### 15. Visualiser — 图表生成

```yaml
visualiser:
  enabled: true
  output_dir: ".wukong/visuals"
  max_width: 1200
  max_height: 800
```

### 16. Tutorial — 交互式教程

```yaml
tutorial:
  enabled: true
  language: "zh"                     # zh | en
```

### 17. Top of Mind — 持久化指令注入

```yaml
top_of_mind:
  enabled: true
  instruction_file: ".wukong/instructions.md"
  max_length: 2000                   # 最大指令长度
```

### 18. Code Mode — goja JavaScript 沙箱

```yaml
code_mode:
  enabled: true
  timeout: "10s"                     # 执行超时
  max_memory_mb: 128                # 最大内存 (MB)
```

**5 层安全限制**:
1. API 白名单（仅安全函数可用）
2. 128MB 内存限制
3. 5 并发 goroutine 限制
4. ReDoS 正则攻击防护
5. 1MB 源代码最大限制

### 19. Apps — HTML 应用管理

```yaml
apps:
  enabled: true
  app_dir: ".wukong/apps"
```

---

## 20. ARD — Agentic Resource Discovery（双向发现）

```yaml
ard:
  enabled: false                     # 默认关闭，按需启用
  registry_url: ""                   # 远程 Registry URL（发现别人）
  catalog_path: ".wukong/ard/catalog.json"
  publish_enabled: false             # 发布自身为 Registry（被人发现）
  # publish_port: 8081               # Registry HTTP 端口
```

### ARD 工作模式

| 方向 | 机制 | 配置 |
|------|------|------|
| **Outbound** | 搜索远程 Registry | `registry_url` 指定远程地址 |
| **Inbound** | 启动 RegistryServer | `publish_enabled: true`, 默认 :8081 |
| **Auto** | 自动注册资源 | MCP 连接 → 注册；A2A Remote → 注册 |

---

## 21-26. 编排与委派

### 21. Summon — 子代理委派 + A2A

```yaml
summon:
  enabled: true
  skills_dir: ".wukong/skills"
  max_concurrent: 5                  # 最大并发子代理数
  a2a_remotes: []                    # A2A 远程代理配置
```

### 22. Skill — Agent Skill 仓库

```yaml
skill:
  enabled: true
  skills_dir: ".wukong/skills"
  auto_load: true                    # 自动加载
  max_skills: 20                     # 最大 Skill 数
```

### 23. Evolution — 技能自进化（实验性）

```yaml
evolution:
  enabled: false                     # 默认关闭（实验性功能）
  auto_patch: false                  # 自动应用补丁
  analysis_provider: ""              # 分析用 Provider
  analysis_model: ""                 # 分析用模型
  min_confidence: 0.7                # 最小可信度
  cooldown_period: "30m"             # 冷却时间
  max_patches_per_day: 10            # 每日最大补丁数
  max_versions_kept: 10              # 保留版本数
  max_patch_size: 8192              # 最大补丁大小 (字符)
  analysis_timeout: "60s"            # 分析超时
```

### 24. Knowledge — RAG 知识库

```yaml
knowledge:
  enabled: false
  embedder_provider: "lmstudio"
  embedder_model: "qwen3-embedding-4b"
  vector_store: "inmemory"           # inmemory | cos
  max_results: 5
  enable_source_sync: false
  reranker_enabled: false
  search_tool_name: "knowledge_search"
```

### 25. Workflow — 多模式 Agent 编排

```yaml
workflow:
  mode: "single"                     # single|chain|parallel|cycle|graph|team_coordinator|team_swarm|claude_code|codex|dify
  max_iterations: 10
  cycle_mode: "default"
  stream_mode: "none"
  cache_enabled: false
  engine: "bsp"                      # bsp (Bulk Synchronous Parallel)
  sub_agents: []                     # 子 Agent 配置
```

### 26. Dify — Dify AI 平台集成

```yaml
dify:
  enabled: false
  agent_name: "dify"
  enable_streaming: false
  timeout: "120s"
```

---

## 27-28. 服务端点

### 27. 四协议服务

```yaml
# A2A 服务器 (:9090)
a2a_server:
  enabled: true
  address: ":9090"
  agent_name: "wukong"
  agent_description: "Wukong AI Agent - A2A service endpoint"

# AG-UI SSE (:8080)
agui:
  enabled: true
  address: ":8080"
  path: "/agui"

# ACP 服务器 (:9091)
acp_server:
  enabled: true
  address: ":9091"
  path: "/acp"
  enable_streaming: true

# ACP MCP Bridge (:3400)
acp_mcp:
  enabled: true
  address: ":3400"
  path: "/mcp"
```

### 协议端口映射

| 协议 | 端口 | 路径 | 用途 |
|------|------|------|------|
| A2A | 9090 | — | Agent-to-Agent 通信 |
| AG-UI | 8080 | `/agui` | Web UI 实时对话 |
| ACP | 9091 | `/acp` | Agent Client Protocol |
| ACP MCP | 3400 | `/mcp` | 跨协议工具桥接 |

---

## 28-32. 观测/扩展/存储

### 28. Eval — Agent 评估

```yaml
eval:
  enabled: false
  evalset_path: ".wukong/evals/default.evalset.json"
  results_path: ".wukong/evals/results.json"
```

### 29. Extensions — 外部 MCP 扩展

```yaml
extensions: []                       # 外部 MCP 扩展配置
# 示例:
# extensions:
#   - name: "custom-mcp"
#     type: "external"
#     url: "http://localhost:9000"
#     mcp_broker: true
```

### 30. Telemetry — OpenTelemetry 分布式追踪

```yaml
telemetry:
  enabled: false
  exporter_type: "console"           # console | otlp-grpc | otlp-http
  endpoint: "localhost:4317"         # OTLP 导出端点
  service_name: "wukong"
  service_version: "1.0.0"
  environment: "development"
  sample_rate: 1.0                   # 采样率 (0.0-1.0)
```

### 31. Observability — Langfuse LLM 追踪

```yaml
observability:
  langfuse_enabled: false            # 启用 Langfuse LLM 调用追踪
```

### 32. Artifact — 文件版本化

```yaml
artifact:
  backend: "inmemory"                # inmemory | cos
```

### 33. Project — 工作目录

```yaml
project_dir: "~/.config/wukong/"     # 项目配置和缓存目录
```

---

## 配置项完整索引

| 配置段 | 结构体 | mapstructure 标签 | 字段数 |
|--------|--------|-------------------|--------|
| — | `WukongConfig` | — | 2 + 嵌套 |
| `log_level` | — | `log_level` | — |
| `default_provider` | — | `default_provider` | — |
| `lightweight_provider` | — | `lightweight_provider` | — |
| `lightweight_model` | — | `lightweight_model` | — |
| `providers` | `ProviderConfig[]` | `providers` | 7 字段/个 |
| `agent` | `AgentConfig` | `agent` | 35+ |
| `security` | `SecurityConfig` | `security` | 12 |
| `session` | `SessionConfig` | `session` | 6 |
| `memory` | `MemoryConfig` | `memory` | 10 |
| `todo` | `TodoConfig` | `todo` | 4 |
| `recall` | `RecallConfig` | `recall` | 5 |
| `cortex` | `CortexConfig` | `cortex` | 6 |
| `memoryflow` | `MemoryFlowConfig` | `memoryflow` | 4 |
| `graphflow` | `GraphFlowConfig` | `graphflow` | 3 |
| `importflow` | `ImportFlowConfig` | `importflow` | 2 |
| `revision` | `RevisionConfig` | `revision` | 11 |
| `browser` | `BrowserConfig` | `browser` | 10 |
| `visualiser` | `VisualiserConfig` | `visualiser` | 4 |
| `tutorial` | `TutorialConfig` | `tutorial` | 2 |
| `top_of_mind` | `TopOfMindConfig` | `top_of_mind` | 3 |
| `code_mode` | `CodeModeConfig` | `code_mode` | 3 |
| `apps` | `AppsConfig` | `apps` | 2 |
| `ard` | `ARDConfig` | `ard` | 5 |
| `summon` | `SummonConfig` | `summon` | 4 |
| `skill` | `SkillConfig` | `skill` | 4 |
| `evolution` | `EvolutionConfig` | `evolution` | 9 |
| `knowledge` | `KnowledgeConfig` | `knowledge` | 8 |
| `workflow` | `WorkflowConfig` | `workflow` | 7 |
| `dify` | `DifyConfig` | `dify` | 4 |
| `a2a_server` | `A2AServerConfig` | `a2a_server` | 4 |
| `agui` | `AGUIConfig` | `agui` | 3 |
| `acp_server` | `ACPServerConfig` | `acp_server` | 4 |
| `acp_mcp` | `ACPMCPConfig` | `acp_mcp` | 3 |
| `eval` | `EvalConfig` | `eval` | 3 |
| `extensions` | `ExtensionConfig[]` | `extensions` | 多字段 |
| `telemetry` | `TelemetryConfig` | `telemetry` | 5 |
| `observability` | `ObservabilityConfig` | `observability` | 1 |
| `artifact` | `ArtifactConfig` | `artifact` | 1 |
| — | — | `project_dir` | — |

---

## 推荐配置方案

### 方案一：最小配置（仅本地模型）

```yaml
default_provider: "lmstudio"
providers:
  - name: "lmstudio"
    type: "lmstudio"
    api_key: "lmstudio"
    base_url: "http://localhost:1234/v1"
    model: "google/gemma-4-26b-a4b"
```

### 方案二：云端模型 + 基础功能

```yaml
default_provider: "deepseek"
providers:
  - name: "deepseek"
    type: "deepseek"
    api_key: "${DEEPSEEK_API_KEY}"
    base_url: "https://api.deepseek.com"
    model: "deepseek-chat"
agent:
  todo_enforcer_enabled: true
  context_compaction: true
memory:
  auto_extract: true
```

### 方案三：完整记忆配置

在方案二基础上额外启用:

```yaml
lightweight_provider: "deepseek"
lightweight_model: "deepseek-chat"
cortex:
  enabled: true
  embedding_base_url: "http://localhost:1234"
  embedding_model: "qwen3-embedding-0.6b"
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
  enable_llm_summarize: true
```

### 方案四：安全增强配置

```yaml
security:
  permission_mode: "smart"
  block_dangerous_commands: true
  ignore_file_enabled: true
  guardrail_enabled: true
agent:
  todo_enforcer_enabled: true
  json_repair_enabled: true
code_mode:
  enabled: true
  timeout: "10s"
  max_memory_mb: 128
```

### 方案五：全功能配置

以上所有 + 以下额外启用:

```yaml
browser:
  enabled: true
a2a_server:
  enabled: true
agui:
  enabled: true
acp_server:
  enabled: true
acp_mcp:
  enabled: true
ard:
  enabled: true
  publish_enabled: true
  publish_port: 8081
```
