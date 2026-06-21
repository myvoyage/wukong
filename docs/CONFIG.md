# Wukong 配置参考手册

> **配置文件**: `config.yaml` | **加载器**: Viper | **配置段**: 30 | **覆盖**: CLI > ENV > YAML > 默认值
>
> 版本：v0.6.0

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

**5 层安全纵深**：Guard 权限 → goja JS 沙箱 → sandbox OS 隔离 → .wukongignore → OS 权限。

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
  auto_extract: true            # 异步记忆提取
  extract_timeout: "600s"
  extractor_provider: "lmstudio"
  extractor_model: "gemma-4-e4b-it"
```

---

## 7. Todo / Recall / Cortex / MemoryFlow / GraphFlow / ImportFlow

```yaml
todo:
  backend: "sqlite"
  db_path: "wukong.db"
  enable_native_todo: true
  enable_enforcer: true

recall:
  enabled: true
  backend: "sqlite"
  db_path: "wukong.db"
  max_results: 10
  search_mode: "fts5"           # fts5 | hybrid

cortex:
  enabled: true
  db_path: "wukong.db"
  embedding_base_url: "http://localhost:1234"
  embedding_model: "qwen3-embedding-0.6b-graphql"

memoryflow:
  enabled: true
  db_path: "wukong.db"
  planner_model: "gemma-4-e4b-it"
  extractor_model: "gemma-4-e4b-it"

graphflow:
  enabled: true
  db_path: "wukong.db"
  auto_extract: false

importflow:
  enabled: true
  db_path: "wukong.db"
```

---

## 8. Revision — 上下文压缩

```yaml
revision:
  enabled: true
  revision_provider: "lmstudio"
  revision_model: "gemma-4-e4b-it"
  enable_llm_summarize: true
  summary_cooldown: 120s
  summary_timeout: 30s
  max_context_tokens: 64000
  trim_ratio: 0.3
```

---

## 9. Browser / Code Mode / Visualiser / Tutorial

```yaml
browser:
  enabled: true
  headless: true
  search_backend: "duckduckgo"  # duckduckgo | searxng | tavily

code_mode:
  enabled: true
  timeout: "10s"                # JS 执行超时
  max_memory_mb: 128            # 内存限制 (goja + runtime.SetMemoryLimit)

visualiser:
  enabled: true
  output_dir: ".wukong/visuals"
```

**goja JS 沙箱安全措施**：API 白名单（console/JSON/Math）、禁用 eval/Function/RegExp/Date、并发限流(max 5)、1MB 代码大小限制、1MB JSON.parse 限制。

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
  extractor_model: "gemma-4-e4b-it"
memoryflow:
  enabled: true
graphflow:
  enabled: true
importflow:
  enabled: true
revision:
  enabled: true
  revision_provider: "lmstudio"
  revision_model: "gemma-4-e4b-it"
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
