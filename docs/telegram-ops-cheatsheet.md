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

## 常见告警含义

| 状态 / 告警 | 处理 |
|-------------|------|
| `agent_traffic_mode_mismatch` | 核对 Agent env 与节点 `mode`，必要时重新 join |
| `dns_unmatched` | 当前 DNS IP 无匹配节点，检查记录与节点 IP |
| `offline` / `recovered` | 节点离线或恢复，关注 Agent systemd |
| `threshold` | 流量达阈值，将自动切换或仅通知（视策略） |

## CLI 对照（SSH 到 Master）

```bash
qdr-master status
qdr-master config-check
qdr-master backup [--output path]
qdr-master restore --from <backup.db>
```

## 观察者角色

`QDR_TELEGRAM_OBSERVER_IDS` 中的用户可查看状态，**不可**修改配置（按钮与变更类命令被拦截）。
