# Wukong 配置参考手册

> **配置文件**: `config.yaml` | **加载器**: Viper | **配置段**: 30 | **覆盖**: CLI > ENV > YAML > 默认值
>
> 版本：v0.6.1

---

## 配置加载机制

```
优先级（从高到低）:
  1. CLI 参数 (--provider, --model, --temperature, --max-tokens, --no-stream)
  2. 环境变量 (WUKONG_ 前缀)
  3. 当前目录 ./config.yaml
  4. ~/.config/wukong/config.yaml
  5. /etc/wukong/config.yaml (仅非 Windows 平台)
  6. 内置默认值
```

---

## 1. 全局设置

```yaml
log_level: "info"              # debug | info | warn | error
default_provider: "lmstudio"   # 默认 LLM Provider
```

### 全局轻量模型（后台任务共享）

```yaml
lightweight_provider: "lmstudio"
lightweight_model: "gemma-4-e4b-it"
```

当各子系统未显式指定模型时统一使用：
- memory.extractor_model（记忆提取）
- memoryflow.planner_model / extractor_model（检索规划/转录提取）
- graphflow.extractor_model（知识图谱构建）
- revision.revision_model（上下文压缩）

---

## 2. Providers

```yaml
providers:
  - name: "openai"           # 唯一标识
    type: "openai"           # openai | anthropic | google | deepseek | ollama | lmstudio | acp
    api_key: "${OPENAI_API_KEY}"
    base_url: "https://api.openai.com/v1"
    model: "gpt-4o"

  - name: "lmstudio"
    type: "lmstudio"
    api_key: "lmstudio"
    base_url: "http://localhost:1234/v1"
    model: "google/gemma-4-26b-a4b"
```

---

## 3. Agent 核心行为

```yaml
agent:
  max_llm_calls: 50             # 最大 LLM 调用次数
  max_tool_iterations: 30       # 最大工具调用次数
  parallel_tools: true          # 并行工具调用
  streaming: true               # 流式输出
  max_run_duration: "300s"      # 最大运行时长
  temperature: 0.7
  max_tokens: 4096

  # 工具重试
  tool_retry_enabled: true
  tool_retry_max_attempts: 3
  tool_retry_backoff_factor: 2.0

  # Planner: builtin (原生 thinking) | react (标签引导) | "" (不启用)
  planner: ""

  # 工具筛选 + 上下文压缩
  tool_search_enabled: true
  context_compaction: true
  session_recall_enabled: false

  # Todo 追踪
  todo_tool_enabled: true
  todo_enforcer_enabled: true

  # 子 Agent / Recipe
  agent_tools_enabled: false
  system_prompt_dir: "~/.config/wukong/prompts/"
```

---

## 4. Security — 安全策略

```yaml
security:
  malware_scan_enabled: true
  default_timeout: "30s"
  max_timeout: "300s"
  block_dangerous_commands: true
  blocked_commands: ["rm -rf /", "dd if=/dev/zero", "mkfs.", "> /dev/sda"]
  permission_mode: "smart"      # auto | smart | manual | chat_only
  allowlist: []
  denylist: []
  guardrail_enabled: false      # Prompt 注入检测 (增加延迟)
  ignore_file_enabled: true     # .wukongignore 文件黑名单
  ignore_file: ".wukongignore"
```

---

## 5. Session — 会话存储

```yaml
session:
  backend: "sqlite"             # sqlite | memory | redis
  db_path: "wukong.db"
  event_limit: 500
  ttl: "0h"
  enable_summary: true
  summary_trigger: 50
```

---

## 6. Memory — 长期记忆

```yaml
memory:
  backend: "sqlite"
  db_path: "wukong.db"
  max_memories: 100
  auto_extract: true            # 异步 LLM 记忆提取
  extract_timeout: "600s"       # 本地模型提取超时
  extractor_provider: "lmstudio" # 独立提取模型 provider
  # extractor_model: ""         # 未指定 → 使用 lightweight_model
  extractor_prompt: |           # 自定义提取 Prompt
    You are a Memory Manager...
```

**SmartCleanup 机制**：
- < 80% 容量：仅删除 TTL 过期记忆（30 天）
- ≥ 80% 容量：评分淘汰（70% recency + 30% content length），降至 60%
- `max_memories: 0` 表示不限制

---

## 7. 记忆系统配置

### Recall — 跨会话对话搜索

```yaml
recall:
  enabled: true
  backend: "sqlite"
  db_path: "wukong.db"
  max_results: 10
  max_messages_per_session: 200
  search_mode: "fts5"           # fts5 | hybrid (需要 embedding)
```

### CortexDB — 智能回溯与知识存储

```yaml
cortex:
  enabled: true
  db_path: "wukong.db"
  max_results: 10
  max_messages_per_session: 200
  embedding_base_url: "http://localhost:1234"
  embedding_api_key: "lmstudio"
  embedding_model: "qwen3-embedding-0.6b-graphql"
```

### MemoryFlow — 转录记录与唤醒

```yaml
memoryflow:
  enabled: true
  db_path: "wukong.db"
  namespace: "assistant"
  embedding_dimensions: 0       # 0=自动检测
  # planner_model: ""           # 留空使用 lightweight_model
  # extractor_model: ""         # 留空使用 lightweight_model
```

### GraphFlow — 知识图谱构建

```yaml
graphflow:
  enabled: true
  db_path: "wukong.db"
  max_chars_per_doc: 8000
  auto_extract: true            # 每轮对话后自动提取实体/关系
  # extractor_model: ""         # 留空使用 lightweight_model
```

### ImportFlow — 结构化数据导入

```yaml
importflow:
  enabled: true
  db_path: "wukong.db"
```

### Todo — 任务管理

```yaml
todo:
  backend: "sqlite"
  db_path: "wukong.db"
  enable_native_todo: true
  enable_enforcer: true
```

---

## 8. Revision — 上下文压缩

```yaml
revision:
  enabled: true
  revision_provider: "lmstudio"
  # revision_model: ""          # 未指定 → lightweight_model
  enable_llm_summarize: true
  summary_cooldown: 120s
  summary_timeout: 30s
  max_command_output: 8000
  enable_semantic_search: false
  search_strategy: "include_all"
  max_context_tokens: 64000
  trim_ratio: 0.3
```

---

## 9. Browser / Code Mode

```yaml
browser:
  enabled: true
  browser_type: "chromium"
  headless: true
  search_backend: "duckduckgo"  # duckduckgo | searxng | tavily
  timeout: "60s"

code_mode:
  enabled: true
  timeout: "10s"                # JS 执行超时
  max_memory_mb: 128            # 内存限制 (goja + runtime.SetMemoryLimit)
```

**goja JS 沙箱安全措施**：API 白名单（console/JSON/Math）、禁用 eval/Function/RegExp/Date、并发限流(max 5)。

---

## 10. Summon / Skill / Evolution

```yaml
summon:
  enabled: true
  skills_dir: ".wukong/skills"
  max_concurrent: 5
  a2a_remotes: []               # 远程 A2A 代理

skill:
  enabled: true
  skills_dir: ".wukong/skills"
  auto_load: true
  max_skills: 20

evolution:
  enabled: false                # 技能自进化
  auto_patch: false             # 自动应用补丁
  min_confidence: 0.7
  cooldown_period: "30m"
  max_patches_per_day: 10
  max_versions_kept: 10
```

---

## 11. Workflow / Dify

```yaml
workflow:
  mode: "single"                # 10 种模式之一
  max_iterations: 10

dify:
  enabled: false
```

---

## 12. 服务端点

```yaml
a2a_server:
  enabled: true
  address: ":9090"

agui:
  enabled: true
  address: ":8080"
  path: "/agui"

acp_server:
  enabled: true
  address: ":9091"
  path: "/acp"

acp_mcp:
  enabled: true
  address: ":3400"
  path: "/mcp"
```

所有 HTTP 端点均实现优雅关闭，body 大小限制 10MB。

---

## 13. Telemetry / Observability / Artifact / Project

```yaml
telemetry:
  enabled: false
observability:
  langfuse_enabled: false
artifact:
  backend: "inmemory"           # inmemory | cos
project_dir: "~/.config/wukong/"
```

---

## 推荐配置

### 最小配置（开箱即用）

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

### 完整记忆配置

```yaml
memory:
  auto_extract: true
  extractor_provider: "lmstudio"
  max_memories: 100
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
```

### 安全增强配置

```yaml
security:
  permission_mode: "smart"
  block_dangerous_commands: true
  ignore_file_enabled: true
agent:
  todo_enforcer_enabled: true
code_mode:
  enabled: true
  timeout: "10s"
  max_memory_mb: 128
```

### 全功能配置（推荐）

启用双引擎记忆闭环 + 知识图谱 + 智能压缩的所有功能：

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

agent:
  todo_tool_enabled: true
  todo_enforcer_enabled: true

security:
  permission_mode: "smart"

code_mode:
  enabled: true
```
