# Telegram 运维命令速查

> 推荐日常使用 **内联按钮与向导**；下列斜杠命令适合脚本化或高级场景。发送 `/cancel` 可退出当前向导。

## 入门

| 命令 / 按钮 | 说明 |
|-------------|------|
| `/start` `/menu` | 打开主菜单 |
| `初始化向导` | 按步骤完成 Master URL → DNS → 分组 → 节点 → Agent |
| `/status` 或 `当前状态` | 分组 / 节点 / DNS / 风险摘要 |
| `/setup` | 查看初始化进度与缺失项 |
| `帮助` | 命令列表 |

## Master 公网地址

| 命令 / 按钮 | 说明 |
|-------------|------|
| `配置 Master 公网地址` | 向导设置 Agent 可访问的 URL |
| `/config_master_url <url>` | 直接写入公网 API 地址 |

## DNS 服务商

### Cloudflare（默认）

| 命令 / 按钮 | 说明 |
|-------------|------|
| `Cloudflare 配置` | Token、Zone 向导 |
| `/cf <Token> <Zone名> [ZoneID]` | 一次性配置；ZoneID 可省略自动查询 |

### Route53

在 `master.env` 设置 `QDR_DNS_PROVIDER=route53` 与 `QDR_AWS_REGION` 后，Telegram `Cloudflare 配置` 入口会引导选择 **Hosted Zone**（无需 Cloudflare Token）。

## 分组与节点

| 命令 | 说明 |
|------|------|
| `/groups add <分组名>` | 新建分组 |
| `/groups rename <原名> <新名>` | 重命名 |
| `/nodes add <节点名> <公网IP> <分组名> [选项...]` | 新建节点 |

**节点可选参数**：`quota=` `threshold=` `reset_day=` `mode=rx|tx|both` `priority=` `enabled=` `auto_switch=` `iface=`

**IP 类型**：A 记录分组用 **IPv4**；AAAA 分组用 **IPv6**。

## DNS 记录

| 命令 / 按钮 | 说明 |
|-------------|------|
| `DNS 配置` | 向导：选分组 → A/AAAA → 记录名 → 指向节点 |
| `/dns set <分组名> <记录名> [ttl=] [proxied=] [record_id=]` | 命令行绑定记录 |

## 流量策略

| 命令 / 按钮 | 说明 |
|-------------|------|
| `流量策略` | 默认阈值、月流量、重置日、维护窗口等 |
| `/policy set threshold=80 quota=1000GB reset_day=1 mode=both offline=300 auto_switch=true notify_only=false repo=<url>` | 批量设置 |

| 策略项 | 说明 |
|--------|------|
| `maintenance_mode` / **维护窗口** 按钮 | 开启后暂停**自动**切换，手动切换仍可用 |
| `notify_only=true` | 仅通知，不自动改 DNS |
| `repo=` | Agent 安装脚本 URL（默认随版本动态生成） |

## Agent

| 命令 / 按钮 | 说明 |
|-------------|------|
| `Agent 安装` | 选节点生成一次性 join 命令 |
| `/agent install <节点名>` | 直接生成安装命令 |

安装后 Agent env 含 `QDR_AGENT_TRAFFIC_MODE`，须与节点策略一致。

## 切换

| 命令 / 按钮 | 说明 |
|-------------|------|
| `手动切换` | 选分组与目标节点 |
| `/switch <分组名> <节点名>` | 命令行手动切换 |

## 告警与通知速查

在 **当前状态** 面板的「当前风险 / 最近失败」或 Telegram 推送中常见如下类型。

### Telegram 推送（notify_once 去重）

| 类型 | 典型文案 | 含义 | 建议处理 |
|------|----------|------|----------|
| `threshold` | 流量达到阈值 | 当前节点用量超过阈值百分比 | 若 `auto_switch=true` 将尝试切换；`notify_only=true` 仅通知 |
| `dns_unmatched` | DNS 未匹配任何节点 | 记录 IP 与所有节点 `public_ip` 不一致 | 核对 DNS 指向、节点 IP、A/AAAA 类型是否匹配 |
| `no_target` | 无可用切换目标 | 阈值/离线触发但无合格备用节点 | 检查备用节点是否启用、`auto_switch`、优先级、cooldown |
| `offline` | 节点离线 | 超过 `agent_offline_seconds` 未上报 | 检查 Agent systemd、Master 公网 URL、防火墙 |
| `recovered` | 节点恢复 | 曾离线且已重新上报 | 确认流量统计是否跳变；必要时校准本账期用量 |
| `traffic_mode_mismatch` | Agent 统计模式不一致 | 上报 `traffic_mode` ≠ 节点策略 | 修正 `QDR_AGENT_TRAFFIC_MODE` 或节点 `mode` 后 re-join |
| 切换成功 | `DNS 自动切换成功` | Cloudflare/Route53 已更新记录 | 验证 DNS 传播与业务连通 |
| 切换失败 | `DNS 自动切换失败` | API 调用失败 | 查 Token/IAM 权限、Zone、记录 ID、限流 |
| 手动切换 | 成功/失败通知 | 同自动切换，触发原因为 manual | 查看切换历史面板 |

### 状态面板 / `qdr-master status` 风险项

| 错误键 / 描述 | 含义 | 建议处理 |
|---------------|------|----------|
| `cloudflare_zone_lookup` | Zone 查询失败 | 检查 Token、Zone 名/ID、网络 |
| `dns_lookup:<group>` | 分组 DNS 查询失败 | 记录名、Zone、服务商凭证 |
| `dns_update:<group>` | DNS 修改失败 | 权限不足、记录被锁定、API 错误详情 |
| `agent_install_command` | 安装命令生成失败 | 补全 Master URL、分组、节点、DNS 配置 |
| `agent_report_auth` | Agent Token 鉴权失败 | Agent env 与 Master 侧 token 不一致，重新 join |
| `agent_traffic_mode_mismatch` | 统计模式不一致 | 见上表 |
| `notification_delivery` | Telegram 发送失败 | Bot Token、网络、管理员 ID |
| Master 公网地址警告 | URL 为 localhost | `/config_master_url` 改为 Agent 可达地址 |
| 缺少 Cloudflare Token / Route53 Zone | 初始化不完整 | 完成 DNS 服务商向导 |
| 维护窗口已开启 | `maintenance_mode=true` | 自动切换暂停；手动切换仍可用 |

### 按场景排查

**DNS 一直不切**

1. `当前状态` → 分组 cooldown 是否未过
2. 维护窗口是否开启
3. `notify_only` 是否为 true
4. 目标节点 `enabled` / `auto_switch` / 优先级
5. DNS 当前 IP 能否 `ResolveCurrentNode`（无 `dns_unmatched`）

**流量统计不准**

1. `qdr-agent status` 网卡是否正确
2. `traffic_mode` 是否与节点一致（rx / tx / both）
3. 是否需本账期用量校准（Telegram 节点详情）

**Agent 频繁离线**

1. `systemctl status qdr-agent`
2. Master `QDR_MASTER_PUBLIC_API_URL` 从 Agent 主机 `curl` 可达
3. `agent_offline_seconds` / `offline_notify_seconds` 策略是否过短

## CLI 对照（SSH 到 Master）

```bash
qdr-master status
qdr-master config-check
qdr-master backup [--output path]
qdr-master restore --from <backup.db>
```

## 观察者角色

`QDR_TELEGRAM_OBSERVER_IDS` 中的用户可查看状态，**不可**修改配置（按钮与变更类命令被拦截）。
