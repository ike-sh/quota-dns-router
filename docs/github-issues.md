# GitHub Issues 路线图

> 由架构审查生成，对应 v0.2.x ~ v1.x 改进计划。已完成项标注 ✅。

## P0 — 安全与可维护性

### ✅ #1 HTTP API 安全加固
- 请求体 64KB 限制
- `/api/agent/join` IP 限流（10次/分钟）
- 结构化访问日志（slog）
- 测试：`http_middleware_test.go`

### ✅ #2 Telegram 模块拆分
- `telegram_wizard_*.go` — 按业务域拆分向导
- `telegram_controller_*.go` — 命令/消息/安装逻辑分离
- `telegram_shared.go` — 共享工具函数

### ✅ #3 修复 ResolveCurrentNode 回退逻辑
- DNS IP 不匹配任何节点时返回 `CurrentNodeUnresolvedError`
- 发送 Telegram 通知并跳过自动切换
- 测试：`TestResolveCurrentNodeReturnsErrorWhenDNSDoesNotMatchNode`

### ✅ #4 修复 Agent 硬编码 rx+tx 上报
- Join 响应携带 `traffic_mode`
- Agent env 新增 `QDR_AGENT_TRAFFIC_MODE`
- 上报使用节点配置的模式

## P1 — 运维增强

### ✅ #5 结构化日志（核心路径）
- DNS 切换成功/失败：`logic.go` slog
- 离线检查/自动切换失败：`runner.go` slog
- HTTP 访问日志：`http_middleware.go` slog

### ✅ #6 健康检查增强
- `/healthz` — 存活探针（纯文本 ok）
- `/readyz` — SQLite Ping 就绪探针（JSON）

### ✅ #7 agent_reports 数据清理
- `Store.PurgeAgentReportsBefore()`
- Master 每 24h 清理，默认保留 30 天（`QDR_AGENT_REPORT_RETENTION_DAYS`）

### ✅ #8 Agent 单元测试
- `RenderAgentEnv` / `PostReport` / `Join` traffic_mode
- `BuildSample` 增量计算

## P2 — 发布与备份

### ✅ #9 备份恢复 CLI
- `qdr-master backup [--output path]`
- `qdr-master restore --from <backup.db>`
- 恢复前自动备份当前库

### ✅ #10 多架构 Release
- CI 构建 `linux/amd64` + `linux/arm64`
- SHA256SUMS 覆盖全部归档

## P3 — 功能扩展

### ✅ #11 AAAA 记录支持
- `DNSProvider` `*WithType` 方法与迁移 `005_record_type.sql`
- Telegram 向导：A / AAAA 类型选择
- DNS 详情 / 摘要面板显示记录类型

### ✅ #12 多 Telegram 管理员（基础）
- `QDR_TELEGRAM_ADMIN_IDS` 逗号分隔，通知广播全部管理员
- 待做：只读观察者角色（细粒度 RBAC）

### ✅ #13 维护窗口
- Policy `maintenance_mode` 开关
- Telegram 策略面板「开启/关闭维护窗口」
- 维护期间暂停自动切换，手动切换仍可用

## P4 — 可选（按需）

### ✅ #14 第二 DNS 服务商（Route53 基础）
- `QDR_DNS_PROVIDER=route53` + `QDR_AWS_REGION`
- AWS 默认凭证链（环境变量 / IAM Role）
- DNSPod 等待贡献

### ✅ #15 只读 Web 状态面板
- `embed.FS` 静态页 `/`、`/status` + `GET /api/status` JSON
- 无需 Telegram 即可查看节点 / DNS / 风险摘要
- 可选 Bearer 只读 Token（`QDR_STATUS_READONLY_TOKEN`）

### ✅ #16 Telegram 只读观察者
- `QDR_TELEGRAM_OBSERVER_IDS`：可查看状态，不可修改配置
