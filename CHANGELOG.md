# Changelog

All notable changes to Wukong after v0.1.14 baseline.

---

## [Unreleased] — 2026-06-26

### Clone Engine — 对标优化
- **Settle 网络空闲等待**: 替代 `Sleep(2s)`, 监听 `EventLoadingFinished/DataReceived/RequestWillBeSent/ResponseReceived`
- **SkipAssetExts + AssetSameDomain**: 38 种文件扩展名过滤, 同域资产限制, 50MB 单资产上限
- **自包含 ZIM 二进制**: `packBinary()` 先建 ZIM 再嵌入可执行文件
- **单次 DOM sink 回调**: `rewriteAndDiscover()` 合并 extract+rewrite 为一次遍历
- **外部 robots.txt**: `github.com/temoto/robotstxt` v1.1.2 替代自定义解析器
- **BFS/DFS 遍历**: `TraversalBFS`/`TraversalDFS` + `traversalDispatcher` + LIFO stack

### Stealth + Antibot 反反爬体系
- **共享 Stealth**: `internal/browser/stealth/` — 13 JS + 11 Chrome flags
- **Antibot 检测**: HTTP 403/429/503 + Cloudflare Headers + 20 DOM 关键词
- **5 级自动升级**: None → Flags → Stealth → Aggressive → Backoff
- **动态注入**: `pool.EnableStealth()` / `controller.EnableStealth()` 运行时 CDP 注入
- **UA 轮换**: Level 3+ 自动切换 9 个真实浏览器 UA

### Settle/Cookie/Session
- **共享 Settle**: `internal/browser/settle/` 消除 ~100 行重复
- **Cookie 持久化**: Netscape 格式, `--cookies <file>`, `CloneSession` 自动保存/加载

### ZIM 修复
- **集群缓存 key 匹配**: 统一为未压缩 hash, 修复增量构建永久 Miss

### 安全修复
- **路径遍历**: `server.go` `hasPrefix` → `strings.HasPrefix` + `filepath.Clean`

### Bug 修复
- **DFS dispatcher goroutine 泄漏**: 新增 `dispatcherStop` channel
- **Stealth flags 格式**: `chromedp.Flag("--key=val", true)` → `chromedp.Flag("key", "val")`

### 测试
- 44 测试: stealth (3) + antibot (18) + session (3) + clone (20, existing)

### 文档
- `config.yaml` 完全重写: 342 行, 28 配置段
- `README.md` 更新: 221 `.go`, 克隆特性表, 反反爬体系
- `docs/ARCHITECTURE.md`: 更新统计
- `docs/CONFIG.md`: 新增 apps.clone 段, Stealth 字段
- `CHANGELOG.md` (新建)

### 统计
- 37 文件变更, 3 新包, 23 新功能, ~250 行删除重复代码
- 1 新依赖: `temoto/robotstxt`
