# Wukong

> 记忆优先、编排驱动、安全纵深的 AI Agent 平台 | Go 1.26 | tRPC 生态

Wukong 是一个本地优先、可扩展的 AI Agent 平台。核心哲学：**Agent 的真正智能不取决于一次对话的表现，而取决于跨对话的记忆积累、多 Agent 的协同编排、多层纵深的安全防御、以及技能的持续进化。**

```bash
go install github.com/km269/wukong/cmd/wukong@latest
wukong session     # 交互模式
wukong configure   # 配置向导
```

---

## 安全纵深

```
Layer 5: Guard 权限控制    → auto / smart / manual / chat_only
Layer 4: goja JS 沙箱      → 白名单 + 内存限制 + 超时 + ReDoS 防护
Layer 3: sandbox OS 级隔离  → Landlock / sandbox-exec / Low IL
Layer 2: .wukongignore      → gitignore 语法文件访问黑名单
Layer 1: OS 进程权限         → 非 root 用户运行
```

---

## 文档

| 文档 | 说明 |
|------|------|
| [README](docs/README.md) | 项目概览、架构哲学、核心优势、快速开始 |
| [架构文档](docs/ARCHITECTURE.md) | 系统架构深度分析、子系统设计、数据流、关键设计决策 |
| [配置手册](docs/CONFIG.md) | 30 个配置段完整参考 |

## 许可证

GNU AGPL-3.0
