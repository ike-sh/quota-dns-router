# quota-dns-router

`quota-dns-router` 是一个面向 Linux 服务器的流量额度 DNS 切换工具。Master 通过 Telegram Bot 管理 Cloudflare、分组、节点和策略；Agent 安装在每台服务器上，统计 RX/TX 流量并上报。当当前 DNS 指向的节点达到阈值或不可用时，Master 自动把 Cloudflare DNS A 记录切换到同组下一台可用节点。

当前版本：`0.1.0-alpha.6`

本项目只实现核心能力：Telegram Bot long polling、SQLite、HTTP Agent API、Cloudflare A 记录管理、systemd 安装和卸载。不包含 Web UI、Webhook、Docker 管理、多 DNS 服务商或代理协议管理。

## 架构说明

- Master：运行 `qdr-master run`，启动 HTTP API、Telegram Bot polling、自动检查任务。
- Agent：运行 `qdr-agent run`，从 `/proc/net/dev` 读取网卡计数，持久化上次计数并上报 Master。
- SQLite：Master 使用 `/var/lib/quota-dns-router/master.db`，启动时自动执行 migration。
- Cloudflare：使用 API Token 管理 DNS A 记录。
- Telegram：只有配置的管理员 ID 可以操作。

## Master 一键安装

推荐使用 raw 一行安装：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/main/scripts/install-master.sh)
```

安装脚本首次只询问两个内容：

1. Telegram Bot Token
2. Telegram 管理员 ID

非交互安装：

```bash
QDR_TELEGRAM_BOT_TOKEN="你的BotToken" QDR_TELEGRAM_ADMIN_ID="你的管理员ID" bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/main/scripts/install-master.sh) --yes
```

安装脚本默认从 GitHub Releases 下载预编译二进制，校验 SHA256 后安装 `qdr-master` 并写入 systemd。Master 安装阶段只询问 Telegram Token 和管理员 ID；Cloudflare、DNS、节点、阈值都在 Telegram 中继续配置。

### 默认安装方式

- 默认 `QDR_INSTALL_MODE=binary`
- 默认不会安装 Go
- 默认不会在目标机器上 `git clone` + `go build`
- 默认来源：GitHub Releases 预编译包

### 小磁盘机器要求

- Master 二进制安装建议至少：
  - `/` 可用空间 >= 80MB
  - `/tmp` 可用空间 >= 80MB
- Agent 二进制安装建议至少：
  - `/` 可用空间 >= 50MB
  - `/tmp` 可用空间 >= 50MB

如果磁盘空间不足，安装器会在执行 `apt` / `curl` / `tar` / `systemd` 之前直接失败，并提示先清理磁盘。

### 源码构建模式

只有显式指定 `QDR_INSTALL_MODE=source` 时，脚本才会准备 Go、下载源码并现场构建：

```bash
QDR_INSTALL_MODE=source bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/main/scripts/install-agent.sh) --join xxx --master http://x.x.x.x:8080
```

源码模式需要更多磁盘空间，且仅在该模式下才会进入 Go 工具链准备与源码编译流程。

更新前如需完全清理旧版本，推荐先执行：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/main/scripts/uninstall-master.sh) --yes --purge
```

安装后会创建：

- `/etc/quota-dns-router/master.env`
- `/var/lib/quota-dns-router/`
- `/var/log/quota-dns-router/`
- `quota-dns-router-master.service`

启动后向 Bot 发送 `/start` 继续初始化。安装后第一步是在 Telegram 配置 Master 公网地址，不需要手动编辑 `master.env`：

```text
/config_master_url
```

支持：

```text
http://1.2.3.4:8080
https://domain.example.com
```

## 配置 Master 公网地址

推荐方式：

```text
点击 Telegram 里的“配置 Master 公网地址”
然后点击“使用当前公网地址”
```

也可以手动发送：

```text
/config_master_url http://服务器公网IP:8080
```

如果直接输入服务器公网 IP：

```text
服务器公网IP
```

系统会自动补全为：

```text
http://服务器公网IP:8080
```

如果 Master 前面有反代或 HTTPS 域名，应配置 `https://domain.example.com`。如果服务器安全组没有放行 8080，Agent 无法连接。`127.0.0.1` 只适合本机调试，不适合 Agent 部署。

## Telegram 初始化流程

推荐顺序改为按钮 / 向导优先：

```text
1. /start
2. 点击 “1. 配置 Master 公网地址”
3. 点击 “使用当前公网地址”
4. 点击 “2. Cloudflare 配置”
5. 粘贴 Cloudflare Token
6. 选择 Zone
7. 点击 “3. 分组管理” 创建分组
8. 点击 “4. 节点管理” 添加节点
9. 点击 “5. DNS 配置” 绑定 A 记录
10. 点击 “6. Agent 安装” 生成安装命令
11. 点击 “7. 当前状态” 观察 Agent 是否上线
```

`/setup` 或 `/start` 会给出同样的初始化向导，并显示当前还缺少哪些配置。`/cf <token> <zone>`、`/groups add <name>`、`/nodes add ...`、`/dns set ...` 这类命令仍可用，但更适合作为高级用法或批量操作。

常用命令：

```text
/start
/menu
/status
/setup
/config_master_url
/nodes
/groups
/dns
/policy
/help
```

## 首屏观察说明

Telegram `/status` 用来看总览，适合真实部署时先扫一眼当前是否能自动切换、最近是否切过、失败是否还在发生。

- `/status`：查看 Master URL、Cloudflare、DNS、分组、节点、在线 Agent、自动切换、最近切换、最近失败和当前风险。
- `/cf`：查看 Cloudflare Token / Zone 配置和最近一次 Zone 查询结果。
- `/dns`：查看 DNS A 记录、当前 IP、匹配节点和最近一次 DNS 查询/修改结果。
- `/groups`：查看切换组、当前指向、可用切换目标和 cooldown。
- `/nodes`：查看 Agent 在线状态、流量、阈值和节点是否可作为切换目标。
- `qdr-master config-check`：在 CLI 侧查看同类诊断，适合配合 systemd 日志排障。

`/status` 中的三个摘要含义：

- 最近切换：最近一次 DNS 切换行为，包括分组、DNS Record、旧 IP、新 IP、旧节点、新节点、切换时间和结果。
- 最近失败：最近一次关键失败，例如 Cloudflare Zone 查询失败、DNS 查询失败、DNS 修改失败、Agent install/join 生成失败或 Agent 上报鉴权失败。
- 当前风险：初始化未完成时会优先显示“待完成”事项，例如配置 DNS A 记录、安装 Agent 到节点；只有节点曾经上线后又失联，才会显示“Agent 离线”。

## Cloudflare Token 权限

建议创建最小权限 API Token：

- Zone.Zone Read
- Zone.DNS Edit

Token 只通过 Telegram `/cf` 配置，展示时会脱敏，不会写入按钮 callback_data。
请只在和 Bot 的私聊里配置 Cloudflare Token，不要在群聊里发送。

排障时 Token 只会显示脱敏摘要，不会明文输出。

## Agent 通过 Telegram 安装

在 Telegram 中执行：

```text
/agent install <节点名>
```

Bot 会生成类似命令：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/main/scripts/install-agent.sh) --join xxxxx --master https://master.example.com
```

建议直接使用 Telegram 生成的完整命令。脚本也支持通过环境变量提供 Master 地址，或显式传入：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/main/scripts/install-agent.sh) --join <Telegram生成的join_code> --master https://master.example.com
```

如果当前分组还没有 DNS A 记录，Bot 仍可生成 Agent 命令，但会明确提示“可以先安装，自动切换暂时不会生效”，避免把初始化流程卡死在最后一步。

Agent 会使用加入码向 Master 拉取 Agent ID、Agent Token、节点名、分组、上报周期和网卡配置，并写入 `/etc/quota-dns-router/agent.env`。安装完成后 `qdr-agent run` 会立即执行一次上报，Telegram `/nodes` 会从“未安装 / 未上线”变成“在线”。

### Agent 安装失败排查

```bash
df -h
qdr-agent version
qdr-agent status
qdr-agent config-check
systemctl status quota-dns-router-agent --no-pager -l
journalctl -u quota-dns-router-agent -n 100 --no-pager
```

重点说明：

- 如果 `df -h` 显示 `/` 已经 100%，请先清理磁盘再重试：

```bash
apt clean
journalctl --vacuum-size=100M
docker system prune -af
```

- 默认是二进制安装，不需要 Go；只有 `QDR_INSTALL_MODE=source` 才会准备 Go 并源码构建。
- 如果系统已经有可用的 Go，源码模式会优先复用系统 Go。
- 官方 Go tarball fallback 会先解压到临时目录，确认完整后再移动到 `/usr/local/go`。

## 节点流量策略

节点支持三种统计模式：

- `rx`：只统计下行 RX
- `tx`：只统计上行 TX
- `both` 或 `rx+tx`：统计 RX+TX

`/policy` 现在是默认策略中心，新建节点时默认只需要选择分组、填写节点名称和公网 IP，其他值直接继承默认策略；如果需要单独修改，再在确认页点击“修改流量策略”，或在节点详情中单独调整。

可通过 `/policy set` 设置默认值，也可在 `/nodes add` 时覆盖：

```text
/policy set threshold=80 quota=1000GB reset_day=1 mode=both offline=300 auto_switch=true notify_only=false
```

## 自动切换说明

触发条件包括：

- 当前节点流量达到阈值
- 当前节点离线超过策略时间
- 当前节点被禁用
- 当前节点不参与自动切换

目标节点规则：

- 同组
- enabled=true
- auto_switch=true
- 未离线
- 未达到阈值
- priority 小的优先
- priority 相同选择使用率最低

每个分组有切换冷却时间，默认 10 分钟，避免频繁切换。

自动切换更可靠的前提：

- 当前 Cloudflare DNS A 记录 IP 最好能匹配某个已配置节点
- `proxied=true` 时，看到的可能是 Cloudflare 代理链路表现，不一定等于真实源站 IP
- Agent 离线节点不会作为切换目标

## 卸载

Master：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/main/scripts/uninstall-master.sh) --yes
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/main/scripts/uninstall-master.sh) --yes --purge
```

Agent：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/main/scripts/uninstall-agent.sh) --yes
bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/main/scripts/uninstall-agent.sh) --yes --purge
```

卸载脚本支持 `--dry-run` 和 `--help`，重复执行不会因为文件不存在而致命失败。

## CLI

Master：

```bash
qdr-master run
qdr-master telegram-run
qdr-master status
qdr-master config-check
qdr-master telegram-status
qdr-master version
qdr-master migrate
```

Agent：

```bash
qdr-agent run
qdr-agent once
qdr-agent status
qdr-agent join --code <code> --master <url>
qdr-agent config-check
qdr-agent version
```

## 常见问题

**Agent 第一次上报 delta 为 0 正常吗？**

正常。第一次运行会保存当前计数，下一次开始计算增量。

**网卡计数器重置怎么办？**

Agent 发现当前计数小于上次计数时，会按重置处理，使用当前计数作为本次 delta，避免负数。

**Cloudflare Zone ID / Record ID 必须手动填吗？**

不是必须。程序会在切换时查询 Zone 和 A 记录；提供 ID 可以减少 API 查询。

**Master 安装后 Agent 命令里的地址是 127.0.0.1 怎么办？**

这是未完成初始化的状态。请在 Telegram 执行 `/config_master_url`，配置 Agent 可访问的公网地址。未配置前，Bot 会拒绝生成 Agent 安装命令，并提示“当前 Master 地址仍是本机地址，Agent 无法访问。请先配置 Master 公网地址。”首次安装脚本仍只询问 Telegram Token 和管理员 ID。

**服务启动时报 master.env permission denied 怎么办？**

先看日志：

```bash
journalctl -u quota-dns-router-master -n 100 --no-pager
```

如果看到：

```text
open /etc/quota-dns-router/master.env: permission denied
```

说明配置文件或目录权限错误，应检查：

```bash
ls -l /etc/quota-dns-router/master.env
ls -ld /etc/quota-dns-router /var/lib/quota-dns-router /var/log/quota-dns-router
```

推荐权限是 `/etc/quota-dns-router` 为 `root:quota-dns-router 750`，`master.env` 为 `root:quota-dns-router 640`，数据和日志目录为 `quota-dns-router:quota-dns-router 750`。

**Token 泄露怎么办？**

不要把 Telegram Bot Token 发到公开聊天或日志里。如果泄露，请立即到 @BotFather 重新生成 token，并更新 `/etc/quota-dns-router/master.env` 后重启服务。

## 首次部署验收清单

1. 确认 Master 服务正常：

```bash
systemctl status quota-dns-router-master
```

2. 在服务器上运行：

```bash
qdr-master config-check
```

3. 在 Telegram 中执行：

```text
/status
```

确认没有关键缺项，至少已完成：

- Master 公网地址
- Cloudflare Token / Zone
- DNS A 记录
- 分组
- 节点

4. 先在 Telegram 中完成 DNS A 记录配置，再生成 Agent 安装命令：

```text
/dns
/agent install <节点名>
```

5. 在节点服务器执行安装命令，等待 Agent 首次上线。

6. 回到 Telegram 检查：

```text
/nodes
```

确认节点 `online=true`。

7. 进行切换验收：

- 可以先做 dry-run 式人工验证：检查当前 DNS 配置、节点优先级、阈值和自动切换开关。
- 再通过降低阈值或构造测试流量，模拟阈值触发，观察 Telegram 通知和 Cloudflare A 记录切换结果。

## 排障查看顺序

推荐按下面顺序排查：

1. Telegram `/status`
2. Telegram `/cf`
3. Telegram `/dns`
4. Telegram `/groups`
5. Telegram `/nodes`
6. `qdr-master config-check`
7. `journalctl -u quota-dns-router-master -n 100 --no-pager`
8. `journalctl -u quota-dns-router-agent -n 100 --no-pager`

重点说明：

- Token 只显示脱敏内容
- `proxied=true` 可能影响你看到的实际回源行为
- DNS 当前 IP 最好匹配某个节点，自动切换判断会更稳定
- Agent 离线节点不会被视为可用切换目标

## 安全注意事项

- Telegram 只允许管理员 ID 操作。
- Telegram Token、Cloudflare Token、Agent Token 不应进入日志或错误消息。
- Agent API 必须使用 Bearer Token。
- callback_data 不包含密钥。
- 不提供远程执行 shell 命令能力。

## 测试

```bash
gofmt -w cmd internal migrations scripts
go test ./...
go vet ./...
bash -n scripts/install-master.sh
bash -n scripts/install-agent.sh
bash -n scripts/uninstall-master.sh
bash -n scripts/uninstall-agent.sh
```
