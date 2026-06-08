# CHANGELOG

## 0.2.2

- Route53 DNS adapter（`QDR_DNS_PROVIDER=route53`，凭证走 AWS 默认链）。
- Telegram Route53 面板：跳过 Token 配置，直接选择 Hosted Zone。
- `/api/agent/report` 限流（120 次/分钟 / token 或 IP）。
- join 限流 map 在条目过多时全局 sweep 过期 key。

## 0.2.1

- 只读 Web 状态面板：`GET /`、`/status` 与 `GET /api/status` JSON API。
- 可选 `QDR_STATUS_READONLY_TOKEN` Bearer 鉴权。
- HTTP `clientIP` 仅在可信反代（loopback/私网）时信任 `X-Forwarded-For` / `X-Real-IP`。
- join 限流 map 自动清理过期 IP 条目。
- Telegram 只读观察者：`QDR_TELEGRAM_OBSERVER_IDS` 可查看状态，不可修改配置。
- DNS 服务商工厂：`QDR_DNS_PROVIDER=cloudflare`（预留第二服务商扩展）。

## 0.2.0

- HTTP API 安全加固：请求体大小限制、join 限流、访问日志。
- Telegram 模块按业务域拆分，提升可维护性。
- 修复 DNS 当前节点解析：IP 不匹配任何节点时不再错误回退。
- Agent 上报使用节点配置的 `traffic_mode`，Join 响应写入 env。
- 健康检查：`/healthz` 存活探针、`/readyz` SQLite 就绪探针。
- 结构化日志覆盖 DNS 切换、离线检查、上报清理等关键路径。
- `agent_reports` 自动清理，默认保留 30 天（`QDR_AGENT_REPORT_RETENTION_DAYS`）。
- `qdr-master backup` / `restore` 数据库备份恢复 CLI。
- Release 与安装脚本支持 `linux/arm64`。
- 维护窗口：Telegram 可暂停自动切换，手动切换仍可用。
- 多 Telegram 管理员：`QDR_TELEGRAM_ADMIN_IDS` 逗号分隔。
- DNS 记录类型基础支持：配置可保存 A / AAAA，切换按记录类型更新 Cloudflare。

## 0.1.0

- 首个正式版本，稳定提供 Master + Agent、Telegram Bot 配置、Cloudflare DNS A 记录切换和 SQLite 本地存储。
- 支持 Agent RX/TX 流量统计、本账期已用流量校准、阈值通知、离线/恢复通知、自动切换通知和手动切换。
- 安装器默认从 GitHub Releases 下载 Linux amd64 二进制，校验 `SHA256SUMS`，并支持重复执行升级/修复安装。
- 卸载脚本默认保留数据，`--purge` 可完全清理配置、数据、日志、unit 和二进制。
- 保留 smoke 验收脚本，用于 Master / Agent 版本、systemd、权限和 CLI 诊断检查。
- README 已按最终用户视角重写，示例域名、IP、Token 和安装命令统一为正式占位值与 `v0.1.0` 路径。

## 0.1.0-rc.1

- 版本升级到 `0.1.0-rc.1`，同步 Master / Agent CLI、安装脚本、README 和 release 下载路径。
- 安装器补齐升级/修复安装路径：检测到已有安装时停止旧服务、备份现有 env、保留配置和数据、替换二进制并重启服务。
- 卸载脚本修复 `--purge` 文案，区分保留数据与完全清理，并保持重复卸载幂等。
- migration 增强旧库兼容，列已存在但 migration 未记录时不会因重复 `ALTER TABLE` 失败。
- 增加 smoke 验收脚本，用于 Master / Agent 版本、systemd、权限和 CLI 诊断检查。

## 0.1.0-alpha.12

- 节点新增本周期 `traffic_offset_bytes` 校准能力，可在 Telegram 节点详情中设置/清零“初始已用流量”，用于导入服务商面板已有的本月用量。
- 节点列表、节点详情、阈值判断、自动切换和通知统一使用“初始已用 + Agent 增量”的合计流量；新账期开始后校准值自动清零。
- `qdr-agent status` / `config-check` 增强网卡诊断，显示统计网卡、默认路由网卡、RX/TX 和 `/proc/net/dev` 可读性，并提示统计网卡与默认路由不一致。
- Agent 安装/加入支持 `--iface eth0` 显式配置统计网卡；`auto` 继续优先识别默认路由网卡。
- 版本升级到 `0.1.0-alpha.12`，同步更新安装脚本、README 和验证。

## 0.1.0-alpha.11

- Telegram Agent 安装页新增 `copy_text` 复制安装/卸载命令按钮，旧客户端继续保留“显示纯安装命令 / 显示纯卸载命令”fallback；复制和显示纯命令都不会刷新 join code，只有“重新生成命令”刷新。
- 补齐流量阈值、节点离线、节点恢复、DNS 自动切换成功/失败、无可用切换目标 Telegram 通知，并基于 `notifications` 表去重，避免同一周期或同一离线状态反复刷屏。
- `/policy` 增加通知设置状态展示和通知开关占位入口，当前细分通知默认启用。
- 继续确认主菜单“手动切换”进入按钮向导并写入 `dns_switch_history trigger_type=manual`，`/dns set` help 保持包含 `<A记录>`。
- 版本升级到 `0.1.0-alpha.11`，同步更新安装脚本、README 和验证。

## 0.1.0-alpha.10

- Telegram 的 pending prompt 统一改为可追踪的编辑/删除流程，Cloudflare Token、Zone Name、DNS TTL、DNS A 记录、分组、节点名、节点 IP 和策略输入在成功或 `/cancel` 后都会清理提示消息。
- Agent 安装命令改为按节点缓存 join code，纯命令复制不会每次点击都生成新码，只有点“重新生成命令”才会刷新。
- DNS help 与向导文案继续对齐到 `<A记录>` 和 `example.com` 占位值，避免帮助文本和真实参数不一致。
