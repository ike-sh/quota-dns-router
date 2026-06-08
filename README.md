# quota-dns-router

quota-dns-router 是一个面向小型 VPS 节点池的 DNS 自动切换工具。它由 Master 和 Agent 两部分组成：Master 通过 Telegram Bot 提供配置入口，维护节点、分组、Cloudflare DNS A 记录和切换策略；Agent 安装在各个 VPS 上，定期上报 RX/TX 流量和在线状态。

当前版本：`0.2.2`

## 项目简介

当一组 VPS 共用同一个业务域名时，你可以用 quota-dns-router 让 DNS A 记录指向当前可用节点。节点流量接近阈值、节点离线、或你手动指定切换目标时，Master 会通过 Cloudflare 更新对应 A 记录，并把结果发送到 Telegram。

项目设计目标是简单、可恢复、易排查：

- 所有运行态数据保存在 Master 本地 SQLite。
- Master 安装后只需要 Telegram Bot Token 和 Telegram 管理员 ID。
- Cloudflare、DNS、分组、节点、策略和 Agent 安装命令都在 Telegram Bot 内完成。
- Agent 默认只读取 Linux `/proc/net/dev`，不上报代理配置、业务端口或用户流量内容。

## 适用场景

- 多台 Linux amd64 VPS 共用一个 Cloudflare DNS A 记录。
- 需要按月流量额度和阈值自动把域名切到备用节点。
- 希望通过 Telegram 完成运维配置，而不是维护 Web 面板。
- 需要清晰知道节点离线、恢复、阈值触发和 DNS 切换结果。

不适合的场景：

- 需要 Web UI 或细粒度 RBAC 权限管理。
- 需要管理代理协议、订阅、用户账号或转发规则。
- 需要多个 DNS 服务商、Docker 编排或任意远程 shell 执行。

## 工作原理

1. Master 运行 Telegram long polling，管理员通过 Bot 配置 Cloudflare、DNS、节点和策略。
2. 每个节点在 Telegram 中生成一次性 Agent 安装命令。
3. Agent 加入 Master 后获得节点 token，并定期上报在线状态、网卡 RX/TX 计数和版本信息。
4. Master 按节点月流量额度、阈值、重置日、在线状态、优先级和 cooldown 判断是否需要切换。
5. 需要切换时，Master 调用 Cloudflare API，把指定分组的 DNS A 记录更新为目标节点 IP。
6. 切换成功、失败、无可用目标、节点离线或恢复时，Master 会发送 Telegram 通知。

## 当前能力

- Master + Agent 双进程。
- Telegram Bot 配置和操作入口。
- Cloudflare DNS A / AAAA 记录查询与更新。
- 分组、节点、默认策略和节点级策略。
- Agent RX/TX 流量统计，支持 `rx`、`tx`、`both` 模式。
- 本账期已用流量校准，适合导入服务商面板已有用量。
- 阈值通知、离线通知、恢复通知、自动切换通知和手动切换。
- SQLite 本地存储和 migration。
- systemd 安装、升级、卸载和 purge。
- GitHub Releases 二进制安装，默认不安装 Go。
- `scripts/smoke.sh` 只读验收检查。
- HTTP 安全加固、`/healthz` / `/readyz` 健康检查、只读 Web 状态面板（`/`、`/api/status`）。
- `qdr-master backup` / `restore` 数据库备份恢复。
- 维护窗口（Telegram 暂停自动切换）。
- 多 Telegram 管理员（`QDR_TELEGRAM_ADMIN_IDS`）。

## 当前限制

- 仅支持 Cloudflare。
- 仅支持 DNS A / AAAA 记录（不支持 CNAME 等）。
- Release 二进制提供 Linux amd64 / arm64。
- 仅支持 Telegram long polling，不支持 webhook。
- 不提供 Web UI。
- 不提供 Docker 管理。
- 不管理代理协议、入站端口、用户账号或订阅。
- 不执行任意远程 shell。
- Agent 需要能访问 Master 公网地址。

其他架构可以使用源码模式安装，但需要 Go、构建依赖和更多磁盘空间。

## 安装 Master

Master 推荐安装在一台稳定的 Linux amd64 服务器上。安装前准备：

- Telegram Bot Token：从 BotFather 获取。
- Telegram 管理员 ID：只允许这个 ID 操作 Bot。

交互安装：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/v0.2.2/scripts/install-master.sh)
```

非交互安装：

```bash
QDR_TELEGRAM_BOT_TOKEN="你的BotToken" QDR_TELEGRAM_ADMIN_ID="你的管理员ID" bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/v0.2.2/scripts/install-master.sh) --yes
```

安装器默认使用 `QDR_INSTALL_MODE=binary`：

- 从 GitHub Releases 下载 `v0.2.2` 二进制包。
- 不安装 Go。
- 不执行 `git clone`。
- 不执行源码构建。
- 下载 `SHA256SUMS` 并校验归档。
- 安装后执行版本校验。

源码模式：

```bash
QDR_INSTALL_MODE=source bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/v0.2.2/scripts/install-master.sh)
```

源码模式适合非 amd64 或需要自行构建的环境，需要 Go 和较多磁盘空间。
源码模式默认构建 `v0.2.2`，需要指定其他引用时可以设置 `QDR_BRANCH`。

常用路径：

- Master env：`/etc/quota-dns-router/master.env`
- 数据目录：`/var/lib/quota-dns-router`
- 日志目录：`/var/log/quota-dns-router`
- systemd unit：`quota-dns-router-master.service`
- 二进制：`/usr/local/bin/qdr-master`

## Telegram 初始化流程

安装 Master 后，在 Telegram 中打开你的 Bot，发送：

```text
/start
```

推荐顺序：

1. 配置 Master 公网地址。
2. 配置 Cloudflare Token 和 Zone。
3. 配置 DNS A 记录。
4. 创建分组。
5. 添加节点。
6. 生成 Agent 安装命令。
7. 查看状态。

所有面板都提供返回路径；支持复制命令的 Telegram 客户端会优先显示 `copy_text` 按钮，旧客户端仍可显示纯命令。

## 配置 Cloudflare Token

在 Telegram 中进入 Cloudflare 配置，发送 Cloudflare API Token。Token 建议只授予目标 Zone 的 DNS 编辑权限。

Token 保存后，Master 会尝试查询 Zone 列表。你可以从按钮选择 Zone，也可以手动输入 Zone Name 和 Zone ID。

示例：

```text
Zone Name: example.com
Token: cf_********abcd
```

Master 不会在 Telegram 消息里明文回显 Token。

## 配置 DNS A 记录

每个分组对应一个 Cloudflare DNS A 记录。推荐使用清晰的业务记录名，例如：

```text
hk.example.com
sg.example.com
node.example.com
```

当记录不存在时，向导可以创建记录；当记录存在但 IP 没有匹配节点时，向导会提示你把记录指向已配置节点。

当前只支持 A 记录，不支持 AAAA。

## 添加节点

节点是安装 Agent 的 VPS。每个节点至少需要：

- 节点名：例如 `hk-01`、`sg-01`、`us-01`
- 公网 IPv4：例如 `203.0.113.10`、`198.51.100.10`、`192.0.2.10`
- 所属分组：例如 `hk`、`sg`、`us`

节点策略默认从全局策略继承，也可以在节点详情里单独修改：

- 月流量额度。
- 阈值百分比。
- 账期重置日。
- 统计模式：`rx`、`tx`、`both`。
- 优先级。
- 是否启用。
- 是否允许自动切换。
- 统计网卡。

## 安装 Agent

Agent 安装命令由 Telegram 为具体节点生成。不要手写 join code，也不要复用过期命令。

流程：

1. Telegram 主菜单进入 `Agent 安装`。
2. 选择节点。
3. 复制 Bot 生成的完整安装命令。
4. 在对应 VPS 上执行。
5. 回到 Telegram 查看节点是否上线。

Agent 默认安装路径：

- Agent env：`/etc/quota-dns-router/agent.env`
- 数据目录：`/var/lib/quota-dns-router`
- 日志目录：`/var/log/quota-dns-router`
- systemd unit：`quota-dns-router-agent.service`
- 二进制：`/usr/local/bin/qdr-agent`

如果服务器有多张网卡，可以在 Telegram 节点策略中设置统计网卡，或在安装命令里显式带上 `--iface eth0`。

## 流量统计口径

Agent 默认读取 Linux `/proc/net/dev` 中的网卡计数器。

默认口径：

- Agent 首次上报后记录基线。
- 后续按 RX/TX 计数器增量计算本账期用量。
- 计数器回绕或系统重启后，Agent 会尽量按新的计数器继续累计。
- 新账期开始后，校准值会自动清零。

统计模式：

- `rx`：只统计接收流量。
- `tx`：只统计发送流量。
- `both`：统计接收 + 发送。

如果统计网卡和默认路由网卡不同，`qdr-agent config-check` 会给出提示。

## 校准已用流量

如果 VPS 在安装 Agent 前，本月已经产生了流量，需要在 Telegram 节点详情里执行“校准已用流量”。

例如服务商面板显示本月已用 `350.5GB`：

```text
350.5GB
```

之后 Master 会用：

```text
初始已用流量 + Agent 增量
```

作为本账期总用量，并据此判断阈值和自动切换。

## 自动切换规则

Master 会综合以下条件选择目标节点：

- 节点已启用。
- 节点在线。
- 节点未超过阈值。
- 节点允许自动切换。
- 分组 cooldown 已结束。
- 优先级更高的节点优先。

当当前节点达到阈值且存在可用目标时，Master 会更新 Cloudflare A 记录。没有可用目标时只发送通知，不会盲目改 DNS。

## 手动切换

你可以在 Telegram 主菜单进入“手动切换”，选择分组和目标节点。手动切换会写入切换历史，触发 Cloudflare A 记录更新，并发送结果通知。

手动切换适合维护窗口、临时迁移或主动恢复主节点。

## 通知说明

当前通知包括：

- 阈值通知。
- 节点离线通知。
- 节点恢复通知。
- DNS 自动切换成功通知。
- DNS 自动切换失败通知。
- 无可用切换目标通知。
- 手动切换结果通知。

通知会基于数据库记录去重，减少同一状态反复刷屏。

## 升级

重新执行安装脚本即可升级或修复安装。

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/v0.2.2/scripts/install-master.sh)
```

安装器会检测已有安装：

- 停止旧服务。
- 备份现有 env。
- 保留配置和数据。
- 替换二进制。
- 执行版本校验。
- 重启 systemd 服务。

Agent 升级同样使用 Telegram 中生成的安装命令。已有 `agent.env` 时，安装器会保留配置并跳过 join。

## 卸载

默认卸载会保留数据目录，方便之后恢复。

卸载 Master：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/v0.2.2/scripts/uninstall-master.sh) --yes
```

卸载 Agent：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/v0.2.2/scripts/uninstall-agent.sh) --yes
```

完全清理：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/v0.2.2/scripts/uninstall-master.sh) --yes --purge
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/v0.2.2/scripts/uninstall-agent.sh) --yes --purge
```

`--purge` 会清理配置、数据、日志、unit 和二进制。

## Smoke 验收

`scripts/smoke.sh` 只做只读检查，不修改配置。

Master：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/v0.2.2/scripts/smoke.sh) master
```

Agent：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/v0.2.2/scripts/smoke.sh) agent
```

检查项包括二进制、版本输出、systemd 状态、env 权限、数据目录权限和 CLI 诊断命令。

## 故障排查

Master 常用命令：

```bash
qdr-master status
qdr-master config-check
qdr-master telegram-status
journalctl -u quota-dns-router-master -n 100 --no-pager
```

Agent 常用命令：

```bash
qdr-agent status
qdr-agent config-check
journalctl -u quota-dns-router-agent -n 100 --no-pager
```

常见问题：

- Bot 无响应：检查 Telegram Token、管理员 ID、网络连通性和 `qdr-master telegram-status`。
- Agent 不上线：确认 Master 公网地址可访问，Agent 安装命令来自 Telegram，且 join code 未过期。
- 流量不符合预期：检查统计模式、统计网卡和是否需要校准已用流量。
- DNS 未切换：检查 Cloudflare Token 权限、Zone、A 记录、节点在线状态和分组 cooldown。
- 非 amd64 机器无法下载二进制：使用 `QDR_INSTALL_MODE=source`。

## 安全注意事项

- Telegram Bot Token、Cloudflare Token、Agent token 和 join code 都应视为敏感数据。
- Cloudflare Token 建议仅授予目标 Zone 的 DNS 编辑权限。
- 只把 Telegram 管理员 ID 设置为可信账号。
- 不要把 Telegram 生成的 Agent 安装命令公开发布。
- Master 公网地址建议放在防火墙、反向代理或安全组策略之后，只开放必要端口。
- 定期备份 `/var/lib/quota-dns-router/master.db`。

## 常见问题

### Master 安装时还需要 Cloudflare Token 吗？

不需要。Master 安装阶段只需要 Telegram Bot Token 和 Telegram 管理员 ID。Cloudflare Token 在 Telegram Bot 中配置。

### Agent 安装命令可以自己拼吗？

不建议。Agent 命令包含一次性 join code，应从 Telegram 复制完整命令。

### 已有 VPS 本月流量怎么办？

在 Telegram 节点详情中使用“校准已用流量”，填入服务商面板显示的本账期已用量。

### 二进制安装失败怎么办？

确认服务器是 Linux amd64，并检查 GitHub Releases 访问。其他架构使用源码模式：

```bash
QDR_INSTALL_MODE=source bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/v0.2.2/scripts/install-master.sh)
```

### 卸载会删除数据库吗？

默认不会。只有传入 `--purge` 才会清理配置、数据和日志。

## Release 与架构支持

`v0.2.2` 发布资产：

- `qdr-master_linux_amd64.tar.gz`
- `qdr-agent_linux_amd64.tar.gz`
- `SHA256SUMS`

归档内包含对应二进制和 `README.txt`。当前 Release 只提供 Linux amd64；其他架构请使用源码模式构建。
