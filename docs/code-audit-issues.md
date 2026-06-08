# 代码审查 Issues（v0.2.3 审计）

> 2026-06 全量代码审查发现项，按优先级排列。已修复项标注 ✅。

## P0 — 功能缺陷

### ✅ #A1 AAAA 记录与 IPv6 节点不闭环
- **问题**：DNS 向导支持 AAAA，但节点校验仅接受 IPv4
- **修复**：`ValidatePublicIP` + `GroupDNSRecordType`，按分组 DNS 类型校验 IPv4/IPv6
- **文件**：`internal/master/validation.go`、`telegram_wizard_nodes.go`

### ✅ #A2 CLI status/config-check 硬编码 Cloudflare
- **问题**：Route53 部署时 `qdr-master status` 诊断不准
- **修复**：使用 `master.NewDNSProvider(cfg.DNSProvider, cfg.AWSRegion)`
- **文件**：`cmd/qdr-master/main.go`

## P1 — 行为错误 / 安全

### ✅ #A3 Agent status 硬编码 RX+TX
- **问题**：`qdr-agent status` 未读取 `QDR_AGENT_TRAFFIC_MODE`
- **修复**：显示真实统计模式标签
- **文件**：`cmd/qdr-agent/main.go`

### ✅ #A4 安装脚本 URL 仍为 v0.1.0
- **问题**：Policy 默认值、Telegram 安装/卸载命令指向旧版本
- **修复**：`version.DefaultInstallAgentURL()` / `DefaultUninstallAgentURL()` 动态版本
- **文件**：`internal/version/version.go`、`db/store.go`、`telegram_controller_*.go`

### ✅ #A5 只读 Web 面板默认无鉴权
- **问题**：未配置 Token 时 `/api/status` 对公网开放
- **修复**：无 Token 时仅允许 localhost 访问；远程需配置 `QDR_STATUS_READONLY_TOKEN`
- **文件**：`internal/master/web_status.go`

## P2 — 设计 / 待规划

### ✅ #A6 Route53 复用 cloudflare_defaults 表（语义清理）
- **修复**：`dns_provider` 持久化到 settings；`DNSCredentialLabel` / `DNSCredentialConfigured` 区分 Route53 与 Cloudflare
- **文件**：`dns_defaults.go`、`setup_status.go`、`runner.go`

### ✅ #A7 telegram-run 子命令废弃警告
- **修复**：`telegram-run` 与 `TelegramRun` 启动时输出 stderr 警告
- **文件**：`cmd/qdr-master/main.go`、`runner.go`

### ✅ #A8 Agent 上报 traffic_mode 一致性校验
- **修复**：上报时比对节点配置，不一致写入 `agent_traffic_mode_mismatch` 告警
- **文件**：`http.go`、`diagnostics.go`、`status_overview.go`

### ✅ #A9 Run() 优雅关闭
- **修复**：首个 worker 错误触发 `stop()`，等待其余 worker 最多 10s
- **文件**：`runner.go`

## 孤立代码清理

### ✅ #A10 internal/system/systemd.go
- **问题**：Go 代码零引用，unit 内容由安装脚本内联
- **修复**：已删除

### #A11 NewBot / NewBotForAdmins 兼容包装
- **状态**：保留，生产路径使用 `NewBotForRoles`

## 验证

```bash
go vet ./...
go test ./...
```
