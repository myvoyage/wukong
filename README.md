# Wukong — 记忆优先 · 编排驱动 · 安全纵深 · 双向发现

> 本地优先、框架组装、可深度扩展的开源 AI Agent 平台
>
> Go 1.26 | 221 `.go` (51 `_test.go`) | 28 内部包 | 43 配置结构体
> CLI: 28 + 55+ 子命令 | 依赖: 29 direct + 105 indirect

---

## 架构哲学

| 哲学 | 核心信念 | 关键工程决策 |
|------|----------|-------------|
| **记忆优先** | Agent 智能源于跨会话知识积累 | 双引擎三层记忆：tRPC Memory + CortexDB Stack |
| **框架组装** | 任何组件都应可替换 | CoreLoop 依赖注入，34 子系统接口隔离 |
| **多 Agent 原生** | 编排是第一公民 | 10 种显式编排模式 + HITL 人机协同 |
| **进化智能** | 技能应从失败中学习 | LLM 分析 → 自动补丁 → 版本管理 → 热重载 |
| **双向发现** | 发现别人，也被人发现 | ARD: 联邦搜索 + RegistryServer 发布 |

---

## 核心能力

| 维度 | 方案 |
|------|------|
| **代码规模** | 221 `.go` (51 `_test.go`) / 28 内部包 |
| **编排模式** | 10 种：single / chain / parallel / cycle / graph / team_* / claude_code / codex / dify |
| **LLM 后端** | 7 种：OpenAI / Anthropic / Google / DeepSeek / Ollama / LMStudio / ACP |
| **记忆系统** | 双引擎三层：tRPC Memory × CortexDB (HNSW+FTS5+RDF) |
| **安全防御** | 5 层纵深：Guard → goja JS 沙箱 → OS 沙箱 → `.wukongignore` → OS 权限 |
| **网站克隆** | Chrome 渲染 → Settle 等待 → DOM 清理 → 单次遍历重写+发现 → 资源过滤 → 去重 → 断点续抓 → 多格式打包 |
| **反反爬** | 5 层：Stealth (默认) → preflight (HEAD) → Antibot 5级升级 → cf_clearance 提取 → 161 UA 池 |
| **ZIM 打包** | Kiwix 兼容 (ZIM v6, zstd 编码 5)：元数据 + 图标 + 计数器 + 增量集群缓存 |
| **扩展体系** | 12 内置扩展 + MCP Broker + ACP MCP Bridge |
| **多协议** | A2A (:9090) / ACP (:9091) / AG-UI SSE (:8080) / ACP MCP (:3400) |
| **存储** | 单文件 `wukong.db` (SQLite WAL) |

---

## 快速开始

```bash
go install github.com/km269/wukong/cmd/wukong@latest

# 交互式配置
wukong configure

# 交互式会话
wukong session
wukong session --provider deepseek --model deepseek-chat

# 单次执行
wukong run --prompt "分析项目结构"
wukong run -d

# 网站克隆 — 反爬全开，开箱即用
wukong apps clone https://example.com --max-pages 50 --max-depth 2
wukong apps clone example.com --scroll --traversal dfs
wukong apps clone example.com --incremental

# 关闭默认反爬功能
wukong apps clone example.com --no-stealth --no-chrome-profile --no-antibot

# Turnstile 站点手动模式
wukong apps clone example.com --no-headless

# 预览 & 打包
wukong apps view example.com
wukong apps pack example.com --format zim --compress
wukong apps pack example.com --format binary -o ./mirror

# Docker
docker build -t wukong .
docker run --rm -v ./out:/out wukong apps clone https://example.com
```

---

## 网站克隆 — 默认开启的全栈反爬

| # | 层 | 默认 | 关闭 CLI |
|---|-----|------|---------|
| 1 | **Stealth** (13 JS + 11 Chrome flags) | ✅ | `--no-stealth` |
| 2 | **Chrome Profile** (`./wukong_chrome_profile`) | ✅ | `--no-chrome-profile` |
| 3 | **Preflight CF 检测** (HEAD 探测 cf-* headers) | ✅ | `--no-antibot` |
| 4 | **5 级自动升级** (None→Flags→Stealth→Aggressive→Backoff) | ✅ | `--no-antibot-auto` |
| 5 | **cf_clearance 提取+复用** (Chrome cookie → HTTP 注入) | ✅ | 自动 |
| 6 | **161 UA 轮换池** (8 浏览器 × 5 平台) | ✅ | Level 3+ 自动 |
| 7 | **sec-ch-ua 头** (Cloudflare L3 检测绕过) | ✅ | 自动 |
| 8 | **Referer + Accept-Language** (HTTP 伪装) | ✅ | 自动 |
| 9 | **ErrNotHTML 路由** (PDF/图片 → asset 下载) | ✅ | 自动 |
| 10 | **Settle 网络空闲等待** (4 CDP 事件监控) | ✅ | `--settle` |

---

## 技术选型

| 类别 | 选择 | 版本 | 用途 |
|------|------|------|------|
| Agent 框架 | tRPC-Agent-Go | v1.10.0 | Agent 编排 |
| MCP 协议 | tRPC-MCP-Go | v0.0.16 | 模型上下文协议 |
| A2A 协议 | tRPC-A2A-Go | v0.2.5 | Agent 间通信 |
| 记忆引擎 | CortexDB | v2.25.0 | HNSW+FTS5+RDF |
| CLI | Cobra + Viper | v1.9.1 / v1.20.1 | 命令行 |
| 浏览器 | Chromedp | v0.15.1 | 无头 Chrome |
| robots.txt | temoto/robotstxt | v1.1.2 | 精确解析 |
| JS 沙箱 | goja | latest | 安全沙箱 |
| 数据库 | modernc.org/sqlite | v1.38.2 | 纯 Go SQLite |

---

## CI/CD & 发布

| 能力 | 说明 |
|------|------|
| **多平台构建** | Linux / macOS / Windows × amd64 + arm64 (GoReleaser) |
| **包管理器** | Homebrew (`km269/tap/wukong`) + Scoop |
| **Docker** | Multi-arch (`ghcr.io/km269/wukong:latest`, Alpine + Chromium) |
| **CI** | Lint + Test(race) + 6-platform Cross-build (GitHub Actions) |

---

## 文档索引

| 文档 | 说明 |
|------|------|
| [架构分析](docs/ARCHITECTURE.md) | 系统架构 · 数据流 · 设计决策 |
| [配置手册](docs/CONFIG.md) | 43 结构体 · 全字段 · 推荐方案 |
| [变更日志](CHANGELOG.md) | 版本历史 |

---

## 许可证

[GNU AGPL-3.0](docs/LICENSE)
