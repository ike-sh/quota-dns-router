# 生产部署检查清单

> quota-dns-router v0.2.3+ 上线前逐项确认。

## 1. Master 基础

- [ ] 使用 `qdr-master run`（**不要**在生产环境使用 `telegram-run`）
- [ ] `QDR_MASTER_PUBLIC_API_URL` 为 Agent 可达的公网 HTTPS/HTTP 地址（非 `127.0.0.1` / `localhost`）
- [ ] `QDR_MASTER_LISTEN_ADDR` 与防火墙规则一致（默认 `:8080`）
- [ ] SQLite 数据目录 `/var/lib/quota-dns-router` 可写且已备份策略
- [ ] `go test ./...` 或 smoke 脚本在目标环境通过

## 2. 安全

- [ ] `QDR_STATUS_READONLY_TOKEN` 已设置（公网暴露时 **必须**；未设置时仅 localhost 可访问状态页）
- [ ] Telegram Bot Token 与管理员 ID 仅保存在 `master.env`（权限 `600`）
- [ ] 反代位于可信网络；`X-Forwarded-For` 仅在 loopback/私网反代后生效
- [ ] Agent join 码一次性使用，安装后立即失效

## 3. DNS 服务商

### Cloudflare（默认）

- [ ] `QDR_DNS_PROVIDER=cloudflare`（或留空）
- [ ] API Token 具备目标 Zone 的 DNS 编辑权限
- [ ] 各分组 DNS 记录类型（A / AAAA）与节点公网 IP 类型一致

### Route53

- [ ] `QDR_DNS_PROVIDER=route53` + `QDR_AWS_REGION`
- [ ] Master 主机 IAM Role 或环境变量具备 `route53:ChangeResourceRecordSets` 等权限
- [ ] Hosted Zone 已在 Telegram 向导中配置

## 4. Agent

- [ ] 每节点独立 join；`QDR_AGENT_TRAFFIC_MODE` 与 Master 节点策略一致
- [ ] `qdr-agent status` 显示的统计模式与节点配置匹配
- [ ] 节点公网 IP：A 记录分组用 IPv4，AAAA 分组用 IPv6
- [ ] systemd 服务 `qdr-agent` 已 enable 且 `Restart=always`

## 5. 运维与监控

- [ ] `GET /healthz` 存活探针、`GET /readyz` 就绪探针已接入监控
- [ ] 定期 `qdr-master backup`（或文件系统快照）
- [ ] Telegram「当前状态」/ Web 面板无 `agent_traffic_mode_mismatch` 告警
- [ ] 维护窗口：重大变更前在策略面板开启 `maintenance_mode`

## 6. 发布后验证

```bash
qdr-master version          # 期望 v0.2.3+
qdr-master config-check     # DNS 服务商与 Zone 状态正常
qdr-agent config-check      # 各 Agent 主机
curl -fsS http://127.0.0.1:8080/healthz
```

## 7. 回滚

- 恢复上一版本二进制 + `qdr-master restore --from <backup.db>`
- DNS 记录可手动切回；切换历史见 Telegram / `/api/status`
