# Wukong 配置参考手册

> **配置文件**: `config.yaml` (453行) | **加载器**: Viper | **配置段**: 28 个 | **CLI+ENV 覆盖**

---

## 配置加载机制

```
优先级: CLI参数 > 环境变量(WUKONG_) > config.yaml > 内置默认值
搜索: --config → ./config.yaml → ~/.config/wukong/config.yaml → /etc/wukong/config.yaml
```

---

## 1. 全局 + Provider

```yaml
log_level: "info"              # debug|info|warn|error
default_provider: "openai"

providers:                     # 6 种: openai/anthropic/google/deepseek/ollama/lmstudio
  - name: "openai"
    type: "openai"
    api_key: "${OPENAI_API_KEY}"
    base_url: "https://api.openai.com/v1"
    model: "gpt-4o"
```

---

## 2. Agent

| 参数 | 默认 | 说明 |
|------|------|------|
| `max_llm_calls` | 50 | API 调用/run |
| `max_tool_iterations` | 30 | 工具迭代上限 |
| `parallel_tools` | true | 并行工具 |
| `streaming` | true | 流式输出 |
| `temperature` / `max_tokens` | 0.7 / 4096 | 生成参数 |
| `tool_retry_*` | enabled/3attempts/1s/2.0backoff | 工具重试 |
| `planner` | "" | builtin/react/空 |
| `tool_search_*` | false/20 | TopK工具筛选 |
| `context_compaction` | true | 两遍压缩 |
| `context_compaction_*` | 1024/0/1 | Pass1阈值/Pass2阈值/保护最近请求 |
| `session_recall_*` | false/5 | 跨会话预加载 |
| `json_repair_enabled` | false | JSON修复 |
| `todo_*` | true/true | todo_write+enforcer |
| `agent_tools_*` | true/false | 子Agent工具+流式 |

---

## 3. Security

```yaml
permission_mode: "smart"       # auto/smart/manual/chat_only
block_dangerous_commands: true
blocked_commands: ["rm -rf /","dd if=/dev/zero","mkfs.","> /dev/sda","fork bomb"]
guardrail_enabled: false       # Prompt 注入检测
```

8 高危工具 (smart 需审批): bash/command* · file_write/replace/delete · browser_* · web_fetch

---

## 4. Session + Memory

```yaml
session:
  backend: sqlite              # sqlite/redis/memory
  db_path: wukong.db
  event_limit: 500 / ttl: 0h / enable_summary: true / summary_trigger: 50
  redis_url: ""               # backend=redis 时

memory:
  backend: sqlite / max_memories: 100 / auto_extract: true / extract_timeout: 120s
  extractor_provider: "" / extractor_model: "" / extractor_prompt: | ...
```

---

## 5. Knowledge (RAG)

```yaml
knowledge:
  enabled: false
  embedder_model: text-embedding-3-small  # 1536维
  sources: ["./docs"] / source_urls: []
  vector_store: inmemory / max_results: 5
  enable_source_sync: false / reranker_enabled: false
```

---

## 6. Workflow (10 模式)

```yaml
workflow:
  mode: single                 # single|chain|parallel|cycle|graph
                               # team_coordinator|team_swarm|claude_code|codex|dify
  max_iterations: 10 / cycle_mode: default / stream_mode: none
  cache_enabled: false / engine: bsp
  claude_code_bin: claude / codex_bin: codex
  team_members: []             # Team 模式成员
  sub_agents: []               # 自定义子Agent
```

---

## 7. Dify

```yaml
dify:
  enabled: false
  base_url: "https://api.dify.ai/v1"
  api_secret: "${DIFY_API_SECRET}"
  enable_streaming: false / timeout: 120s
```

---

## 8. Browser (9 工具)

```yaml
browser:
  enabled: true
  browser_type: chromium       # chromium(CDP)
  headless: true
  cache_dir: .wukong_cache / max_download_size: 104857600 / timeout: 60s
  browser_path: ""             # 自定义Chrome路径
  viewport_width: 1280 / viewport_height: 720
  search_backend: duckduckgo   # duckduckgo/searxng/tavily
  search_backend_url: ""       # SearXNG URL
  search_api_key: ""           # Tavily API Key
```

---

## 9. A2A + AG-UI + Eval + Telemetry + Observability + Artifact

```yaml
a2a_server: { enabled: false, address: ":9090" }
agui: { enabled: false, address: ":8080", path: "/agui" }
eval: { enabled: false, evalset_path: ".wukong_evals/...", results_path: "..." }

telemetry: { enabled: false, exporter_type: console, endpoint: "localhost:4317" }
observability: { langfuse_enabled: false }  # 或 LANGFUSE_* 环境变量
artifact: { backend: inmemory }             # cos 需 cos_bucket_url + COS_SECRETID/KEY
```

---

## 10. Extensions (MCP)

```yaml
extensions:
  - name: filesystem / type: external / transport: stdio
    command: npx / args: ["-y","@anthropic/mcp-server-filesystem","/tmp"]
    mcp_broker: false          # 按需发现
    mcp_tool_filter: []        # include glob
    mcp_tool_exclude: []       # exclude glob
    mcp_session_reconnect: true / mcp_session_reconnect_attempts: 3
  - name: developer / type: builtin / enabled: true
```

---

## 11. 环境变量 + CLI 覆盖

```bash
WUKONG_DEFAULT_PROVIDER=deepseek
WUKONG_AGENT_TEMPERATURE=0.5
```

| CLI 参数 | 覆盖 |
|----------|------|
| `-p/--provider` | default_provider |
| `-m/--model` | provider.model |
| `--temperature` | agent.temperature |
| `--max-tokens` | agent.max_tokens |
| `--no-stream` | agent.streaming=false |
| `-s/--session-id` | sessionID |
| `-c/--config` | 配置文件路径 |
