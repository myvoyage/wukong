# Wukong 配置参考

> **配置文件**: `config.yaml` | **加载器**: Viper + Cobra | **配置段**: 39 | **结构体**: 40+
>
> Go: 1.26 | 文件: 212 `.go` | CLI: 28 顶层 + 55+ 子命令 | 定义: `internal/config/config.go`

---

## 加载优先级 (7 级)

```
1. CLI 参数 (--provider, --model, --temperature, --max-tokens, --config)
2. 环境变量 (WUKONG_ 前缀)
3. --config CLI 指定文件
4. ./config.yaml (当前目录)
5. ~/.config/wukong/config.yaml
6. /etc/wukong/config.yaml (非 Windows)
7. 内置默认值
```

环境变量: `${ENV_VAR}` 运行时自动展开。

---

## 1. 全局设置

```yaml
log_level: "info"
default_provider: "lmstudio"
lightweight_provider: "lmstudio"
lightweight_model: "gemma-4-e4b-it"
```

---

## 2. Providers (7 种)

```yaml
providers:
  - name: "openai"; type: "openai"; api_key: "${OPENAI_API_KEY}"
    base_url: "https://api.openai.com/v1"; model: "gpt-4o"
  - name: "deepseek"; type: "deepseek"; api_key: "${DEEPSEEK_API_KEY}"
    base_url: "https://api.deepseek.com"; model: "deepseek-chat"
  - name: "anthropic"; type: "anthropic"; api_key: "${ANTHROPIC_API_KEY}"
    base_url: "https://api.anthropic.com"; model: "claude-sonnet-4-20250514"
  - name: "ollama"; type: "ollama"; api_key: "ollama"
    base_url: "http://localhost:11434/v1"; model: "llama3"
  - name: "lmstudio"; type: "lmstudio"; api_key: "lmstudio"
    base_url: "http://localhost:1234/v1"; model: "google/gemma-4-26b-a4b"
  - name: "acp-coder"; type: "acp"
    agent_url: "http://localhost:4000"; model: "acp-default"
```

**CLI**: `wukong provider list | test`

---

## 3. Agent

```yaml
agent:
  max_llm_calls: 50; max_tool_iterations: 30; max_run_duration: "300s"
  parallel_tools: true; streaming: true
  temperature: 0.7; max_tokens: 4096
  tool_retry_enabled: true; tool_retry_max_attempts: 3
  planner: ""; tool_search_enabled: true; context_compaction: true
  todo_tool_enabled: true; todo_enforcer_enabled: true
```

---

## 4. Security

```yaml
security:
  permission_mode: "smart"
  blocked_commands: ["rm -rf /", "dd", "mkfs.", "> /dev/sda"]
  guardrail_enabled: false
  ignore_file_enabled: true
  default_timeout: "30s"; max_timeout: "300s"
```

---

## 5. Apps — HTML 应用 + 网站克隆 + ZIM 打包

```yaml
apps:
  enabled: true
  app_dir: ".wukong/apps"

  # 网站克隆 — 增强引擎 (EnhancedCloner)
  clone:
    max_pages: 0                    # 最大页面数 (0=无限制)
    max_depth: 0                    # 最大链接深度 (0=无限制)
    traversal: "bfs"               # bfs | dfs
    workers: 4
    asset_workers: 4
    timeout: 60                    # 秒
    settle: 1500                   # 网络空闲等待 (毫秒)
    respect_robots: true
    crawl_delay: 0                 # 毫秒 (0=使用robots.txt)
    no_sitemap: false
    dedup_content: true
    mobile_readable: true
    enable_resume: true
    incremental: false
    cache_max_age: 86400           # 秒
    asset_same_domain: true        # 仅下载同注册域资产
    max_asset_bytes: 52428800     # 50MB 上限
    stealth: false                 # 反反爬模式
    chrome_path: ""

  # ZIM 打包
  pack:
    compress: true
    incremental: false
    language: "eng"
    creator: "Wukong"
    format: "html"
```

### apps clone CLI

```bash
wukong apps clone <url> [flags]
  -p, --max-pages     int    最大页面数 (0=无限制)
  -d, --max-depth     int    最大链接深度 (0=无限制)
      --traversal      string 遍历策略: bfs (默认) | dfs
  -w, --workers       int    并发数 (默认4)
      --asset-workers  int    资产并发数
      --timeout        int    渲染超时 (秒, 默认60)
      --settle         int    网络空闲等待 (毫秒, 默认1500)
      --subdomains           包含子域名
      --scroll               自动滚动懒加载
      --stealth              反反爬模式
      --asset-same-domain    仅下载同域资产
      --no-sitemap           禁用 Sitemap 发现
      --force                强制删除已有克隆
      --refresh              刷新所有页面
      --incremental          ETag/Last-Modified 增量更新
      --chrome-path   string Chrome 可执行文件路径
```

### apps pack CLI

```bash
wukong apps pack <app-name> [flags]
  -f, --format       string  输出格式: html|zim|binary|app (默认zim)
  -o, --output       string  输出路径
       --compress            启用 zstd 压缩
       --incremental         增量构建(复用集群)
       --language     string  ZIM 语言代码 (默认eng)
       --title        string  ZIM 标题
       --description  string  ZIM 描述
       --date         string  日期 YYYY-MM-DD
       --creator      string  创建者 (默认Wukong)
       --base-binary  string  基础可执行文件(binary/app格式)
       --icon         string  图标路径(app格式)
```

### chrome-path 说明

如果 Chrome 不在系统 PATH 中，使用此参数:

```bash
wukong apps clone example.com --chrome-path "C:\Program Files\Google\Chrome\Application\chrome.exe"
```

启动时日志会显示检测到的 Chrome 路径。

---

## 6. 记忆栈 (Session / Memory / Todo / Recall / Cortex)

| 段 | 关键字段 | CLI |
|----|----------|-----|
| session | backend, db_path, event_limit:500 | session list/delete/info |
| memory | auto_extract:true, max_memories:100 | memory list/search/delete/clear |
| todo | enable_native_todo:true | todo status |
| recall | search_mode:"fts5" | cortex status |
| cortex | HNSW+FTS5; embedding_model | cortex status |
| memoryflow | 转录+唤醒; namespace | cortex status |
| graphflow | RDF; auto_extract:true | cortex status |
| importflow | DDL→KG | cortex status |

---

## 7. 功能工具

| 段 | 配置 | CLI |
|----|------|-----|
| browser | Chromedp; search_backend | — |
| visualiser | output_dir | — |
| tutorial | language | — |
| top_of_mind | instruction_file | init |
| code_mode | goja; timeout:"10s" | — |
| revision | 3层压缩; max_context_tokens | — |
| knowledge | RAG; embedder_model | knowledge status |

---

## 8. 编排与委派

| 段 | 配置 | CLI |
|----|------|-----|
| workflow | 10种模式; mode:"single" | — |
| summon | A2A委派; max_concurrent:5 | — |
| skill | skills_dir; max_skills:20 | skill list/show |
| evolution | 实验性; cooldown:"30m" | evolution status |
| dify | enabled:false | — |
| recipe | recipe_dir | recipe list/show/validate |

---

## 9. 服务端点

```yaml
a2a_server: { enabled:true, address:":9090" }
agui:       { enabled:true, address:":8080", path:"/agui" }
acp_server: { enabled:true, address:":9091", path:"/acp" }
acp_mcp:    { enabled:true, address:":3400", path:"/mcp" }
```

---

## 10. 观测与扩展

```yaml
ard: { enabled:false, publish_enabled:false }
eval: { enabled:false }
extensions: []
telemetry: { enabled:false, exporter_type:"console" }
observability: { langfuse_enabled:false }
artifact: { backend:"inmemory" }
project_dir: "~/.config/wukong/"
```

---

## 11. 推荐配置

### 最小配置

```yaml
default_provider: "lmstudio"
providers:
  - name: "lmstudio"; type: "lmstudio"; api_key: "lmstudio"
    base_url: "http://localhost:1234/v1"; model: "google/gemma-4-26b-a4b"
```

### 云端模型

```yaml
default_provider: "deepseek"
providers:
  - name: "deepseek"; type: "deepseek"; api_key: "${DEEPSEEK_API_KEY}"
    base_url: "https://api.deepseek.com"; model: "deepseek-chat"
memory: { auto_extract: true }
agent: { todo_enforcer_enabled: true, context_compaction: true }
```

### 网站克隆

在以上基础上:

```yaml
apps:
  enabled: true
  clone:
    workers: 4
    respect_robots: true
    dedup_content: true
    mobile_readable: true
    enable_resume: true
```

### 完整记忆

```yaml
cortex: { enabled: true, embedding_model: "qwen3-embedding-0.6b" }
memoryflow: { enabled: true }
graphflow: { enabled: true, auto_extract: true }
importflow: { enabled: true }
recall: { enabled: true }
revision: { enabled: true, enable_llm_summarize: true }
```

### 快速诊断

```bash
wukong config validate    # 配置校验
wukong system-check       # 系统就绪诊断
wukong health             # 运行健康检查
wukong stats              # 统计面板
wukong bench              # 模型性能基准
```

---

## 配置项完整索引

| 配置段 | 结构体 | 字段数 | CLI 入口 |
|--------|--------|--------|----------|
| `log_level` | — | 1 | — |
| `default_provider` | — | 1 | provider list |
| `lightweight_*` | — | 2 | config show |
| `providers` | `ProviderConfig[]` | 多 | provider list/test |
| `agent` | `AgentConfig` | 35+ | config show |
| `security` | `SecurityConfig` | 12 | system-check |
| `session` | `SessionConfig` | 6 | session list/delete/info |
| `memory` | `MemoryConfig` | 10 | memory list/search/delete/clear |
| `todo` | `TodoConfig` | 4 | todo status |
| `recall` | `RecallConfig` | 5 | cortex status |
| `cortex` | `CortexConfig` | 6 | cortex status |
| `memoryflow` | `MemoryFlowConfig` | 4 | cortex status |
| `graphflow` | `GraphFlowConfig` | 3 | cortex status |
| `importflow` | `ImportFlowConfig` | 2 | cortex status |
| `revision` | `RevisionConfig` | 11 | config show |
| `browser` | `BrowserConfig` (含 Stealth) | 11 | — |
| `visualiser` | `VisualiserConfig` | 4 | — |
| `tutorial` | `TutorialConfig` | 2 | — |
| `top_of_mind` | `TopOfMindConfig` | 3 | init |
| `code_mode` | `CodeModeConfig` | 3 | — |
| `apps` | `AppsConfig` + `CloneDefaults` | 15 | apps clone/pack/list |
| `ard` | `ARDConfig` | 5 | ard status/catalog |
| `summon` | `SummonConfig` | 4 | — |
| `skill` | `SkillConfig` | 4 | skill list/show |
| `evolution` | `EvolutionConfig` | 9 | evolution status |
| `knowledge` | `KnowledgeConfig` | 8 | knowledge status |
| `workflow` | `WorkflowConfig` | 7 | — |
| `dify` | `DifyConfig` | 4 | — |
| `a2a_server` | `A2AServerConfig` | 4 | server |
| `agui` | `AGUIConfig` | 3 | server |
| `acp_server` | `ACPServerConfig` | 4 | server |
| `acp_mcp` | `ACPMCPConfig` | 3 | server |
| `eval` | `EvalConfig` | 3 | eval |
| `extensions` | `ExtensionConfig[]` | 多 | extension list/install |
| `telemetry` | `TelemetryConfig` | 多 | health |
| `observability` | `ObservabilityConfig` | 多 | health |
| `artifact` | `ArtifactConfig` | 多 | config show |
| `project_dir` | — | 1 | project list |
