# Wukong — 记忆优先 · 编排驱动 · 安全纵深 · 双向发现

> **本地优先、框架组装、可深度扩展的开源 AI Agent 平台**
>
> Go 1.26 | 221 `.go` (47 `_test.go`) | 28 内部包 | CLI: 28 + 55+ 子命令 | ~30,000 行 | GNU AGPL-3.0

Wukong 的核心理念：Agent 的真正智能不取决于单次对话的表现，而取决于跨会话的记忆积累、多 Agent 的协同编排、多层纵深的安全防御、以及技能的持续自进化。

---

## 架构哲学

| 哲学 | 核心信念 | 关键工程决策 |
|------|----------|-------------|
| **记忆优先** | Agent 智能源于跨会话知识积累 | 双引擎三层记忆：tRPC Memory + CortexDB Stack (HNSW+FTS5+RDF) |
| **框架组装** | 任何组件都应可替换 | CoreLoop 依赖注入，34 子系统接口隔离 |
| **多 Agent 原生** | 编排是第一公民 | 10 种显式编排模式 + HITL 人机协同 |
| **进化智能** | 技能应从失败中学习 | LLM 分析 → 自动补丁 → 版本管理 → 热重载 |
| **双向发现** | 发现别人，也被人发现 | ARD: 联邦搜索 + RegistryServer 发布 |

---

## 核心能力总览

| 维度 | 方案 |
|------|------|
| **代码规模** | 221 `.go` (47 `_test.go`) / 28 内部包 / ~30,000 行 |
| **编排模式** | 10 种：single / chain / parallel / cycle / graph / team_coordinator / team_swarm / claude_code / codex / dify |
| **Recipe** | 14 项功能：参数化/结构化输出/子配方/重试/继承/内联/模型覆盖/超时/热重载 |
| **LLM 后端** | 7 种：OpenAI / Anthropic / Google / DeepSeek / Ollama / LMStudio / ACP |
| **记忆系统** | 双引擎三层：tRPC Memory × CortexDB (HNSW+FTS5+RDF) |
| **安全防御** | 5 层纵深：Guard → goja JS 沙箱 → OS 沙箱 → `.wukongignore` → OS 权限 |
| **网站克隆** | Headless Chrome 渲染 → 网络空闲等待 → DOM清理 → 单次遍历重写+发现 → 资源过滤 → 内容去重 → 断点续抓 → 多格式打包 |
| **反反爬** | 4 层：静态 Stealth (13 JS + 11 flags) → Antibot 检测 (HTTP/DOM) → 5 级自动升级 → 动态注入 + UA 轮换 |
| **ZIM 打包** | Kiwix 兼容归档 (ZIM v6, zstd 编码 5)：元数据 + 图标 + 计数器 + 增量集群缓存 |
| **扩展体系** | 12 内置扩展 + MCP Broker + ACP MCP Bridge |
| **ARD 双向发现** | 联邦搜索远程 Registry + 发布自身 Registry |
| **多协议支持** | A2A (:9090) / ACP (:9091) / AG-UI SSE (:8080) / ACP MCP (:3400) |
| **CLI 命令** | 28 顶层命令 + 55+ 子命令 |
| **HTML 应用** | 创建/模板/克隆/打包 (ZIM/Binary/App)/导出/版本历史/沙箱预览 |
| **存储** | 单文件 `wukong.db` (SQLite WAL) 承载全栈 |

---

## 快速开始

```bash
go install github.com/km269/wukong/cmd/wukong@latest

# 交互式配置
wukong configure

# 启动交互式会话
wukong session
wukong session --provider deepseek --model deepseek-chat

# 单次执行
wukong run --prompt "分析项目结构"
wukong run -d                         # 多轮对话

# 系统状态
wukong health && wukong stats && wukong system-check

# ── 网站克隆 (JS 全部剥离, 离线镜像) ──
wukong apps clone https://example.com --max-pages 50 --max-depth 2
wukong apps clone example.com --stealth --scroll        # 反反爬 + 懒加载
wukong apps clone example.com --traversal dfs           # 深度优先
wukong apps clone example.com --settle 3000 --workers 8 # 慢速站点
wukong apps clone example.com --cookies cookies.txt     # 登录态克隆
wukong apps clone example.com --incremental             # 增量更新

# ── 打包分发 ──
wukong apps pack example.com --format zim --compress
wukong apps pack example.com --format binary -o ./mirror
wukong apps pack example.com --format app               # macOS .app / Win .exe
```

---

## 网站克隆 — 关键特性

| 特性 | 说明 |
|------|------|
| **Headless Chrome 渲染** | 执行 JS 后捕获最终 DOM，支持懒加载滚动 |
| **网络空闲等待** | 监听 4 种 CDP 网络事件，空闲 1.5s 后快照 (可配置) |
| **反反爬 4 层** | 静态 Stealth → Antibot 检测 → 5 级升级 → 动态注入 |
| **资源过滤** | 跳过视频/音频/文档/归档 38 种扩展名，仅下同域资产 |
| **单次 DOM 遍历** | 发现 + 重写 + 入队合并为一次 walk |
| **SHA-256 内容去重** | 相同内容用硬链接复用，零磁盘冗余 |
| **断点续抓** | `state.json` 原子写入，中断可恢复 |
| **BFS/DFS 遍历** | 广度/深度优先可选 |
| **Cookie 持久化** | Netscape 格式，支持登录态网站克隆 |
| **多格式打包** | 文件夹 / ZIM (Kiwix) / 自包含 Binary / 桌面 App |

---

## 反反爬体系

```
Layer 1: Stealth (静态)    13 JS 注入 + 11 Chrome 反检测标志
Layer 2: Antibot 检测       HTTP 403/429/503 + Cloudflare + DOM 20 关键词
Layer 3: 5 级自动升级      None → Flags → Stealth → Aggressive → Backoff
Layer 4: 动态注入 + UA     pool.EnableStealth() + 9 UA 轮换
```

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
| 可观测 | OpenTelemetry | v1.43.0 | 分布式追踪 |

---

## 文档索引

| 文档 | 说明 |
|------|------|
| [架构分析](docs/ARCHITECTURE.md) | 15 章完整架构、16 ADR |
| [配置手册](docs/CONFIG.md) | 42 结构体、292 字段、5 推荐方案 |
| [变更日志](CHANGELOG.md) | 版本历史 |

---

## 许可证

[GNU AGPL-3.0](docs/LICENSE)
