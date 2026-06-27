# Changelog

All changes after v0.1.14 baseline.

---

## [Unreleased] — 2026-06-27

### Clone Engine — Chrome 渲染快照（对标优化）

- **Settle 网络空闲等待**: 替代 Sleep(2s), 监听 4 CDP 网络事件
- **autoScroll 动态高度**: 每步重算 scrollHeight, 懒加载图片不再遗漏
- **ErrNotHTML 路由**: 非 HTML 资源 (PDF/图片) → AssetDownloader 自动下载
- **AssetWorkers 4→8**: IO 密集型下载并发翻倍
- **RenderTimeout 独立**: 30s 渲染硬超时, 与 HTTP Timeout 60s 分离
- **BFS/DFS 遍历**: traversalDispatcher + LIFO stack
- **external robots.txt**: temoto/robotstxt v1.1.2

### Clone Engine — JS/追踪全清除（安全性）

- **sanitize.CleanHTMLWithOptions**: Script / Noscript / MetaRefresh / DeadLink / 条件注释 全移除
- **unsafe elements 移除**: Iframe, Embed, Object, Applet, Base — 确保零网络请求
- **on* handlers 剥离**: 所有事件处理属性 (onclick, onload 等)
- **javascript: URL 中性化**: href→"#", src/action 等直接删除属性
- **charset 保证**: 自动注入 <meta charset="utf-8">

### Clone Engine — 资源本地化 + 链接重写

- **单次 DOM sink 回调**: rewriteAndDiscover() 合并重写+发现+入队
- **CSS 全量重写**: url() 三种引号 + @import 两种引号
- **资产过滤**: 42 种扩展名跳过 + AssetSameDomain + 50MB 上限
- **SHA-256 内容去重**: 硬链接复用, 零磁盘冗余
- **断点续抓**: frontier state.json 原子写入
- **apps view 命令**: HTTP 服务 + 自动打开浏览器预览

### 反反爬体系 — 5 层深度防御

| 层 | 技术 | 默认 |
|----|------|------|
| 1 | Stealth (13 JS + 11 Chrome flags) | ✅ on |
| 2 | Preflight CF 检测 (HEAD → cf-* headers) | ✅ on |
| 3 | 5 级自动升级 (None→Flags→Stealth→Aggressive→Backoff) | ✅ on |
| 4 | cf_clearance 提取+复用 (Chrome cookie → HTTP) | ✅ auto |
| 5 | 161 UA 轮换池 (8 浏览器 × 5 平台) | ✅ L3+ |
| + | sec-ch-ua / Referer / Accept-Language header 伪装 | ✅ auto |

### Cookie/Session/Profile

- **Cookie 持久化**: Netscape 格式, --cookies <file>
- **Chrome Profile**: ./wukong_chrome_profile (默认), 复用 cookies/storage
- **cf_clearance**: Chrome 提取 → 注入 preflight + asset downloader

### ZIM 修复

- **集群缓存 key 匹配**: 统一为未压缩 hash, 修复增量构建永久 Miss

### 安全修复

- **路径遍历**: server.go hasPrefix → strings.HasPrefix + filepath.Clean
- **unsafe elements**: enhanced.go 补齐 Iframe/Embed/Object/Applet/Base 移除

### Bug 修复

- **DFS dispatcher goroutine 泄漏**: dispatcherStop channel
- **Stealth flags 格式**: chromedp.Flag("--key=val", true) → chromedp.Flag("key", "val")
- **Int63n(0) panic**: Cooldown=0 安全保护
- **autoScroll height freeze**: 每步动态重算 scrollHeight

### CI/CD

- .goreleaser.yaml: 6 平台 + Homebrew + Scoop + Docker multi-arch
- .github/workflows/ci.yml: Lint + Test(race) + Cross-build
- .github/workflows/release.yml: Tag → GPG + Cosign + Docker push
- Dockerfile: Alpine + Chromium 多阶段构建

### 测试

- 51 _test.go: clone 6 + antibot 18 + stealth 3 + session 3 + 其他 21
- 26 antibot tests (HTTP 7 + DOM 11 + Turnstile 4 + Headers 2 + Markers 2)

### 文档

- config.yaml: 完全重写, 43 结构体, 精确对齐代码默认值
- README.md: 221 .go, 反爬默认开启表, 技术选型
- docs/ARCHITECTURE.md: 更新文件/结构体统计
- docs/CONFIG.md: 更新字段/结构体统计
- CHANGELOG.md (本文件)
