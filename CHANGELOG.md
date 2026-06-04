# CHANGELOG

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
- 版本升级到 `0.1.0-alpha.12`，同步更新安装脚本、README 和测试。

## 0.1.0-alpha.11

- Telegram Agent 安装页新增 `copy_text` 复制安装/卸载命令按钮，旧客户端继续保留“显示纯安装命令 / 显示纯卸载命令”fallback；复制和显示纯命令都不会刷新 join code，只有“重新生成命令”刷新。
- 补齐流量阈值、节点离线、节点恢复、DNS 自动切换成功/失败、无可用切换目标 Telegram 通知，并基于 `notifications` 表去重，避免同一周期或同一离线状态反复刷屏。
- `/policy` 增加通知设置状态展示和通知开关占位入口，当前细分通知默认启用。
- 继续确认主菜单“手动切换”进入按钮向导并写入 `dns_switch_history trigger_type=manual`，`/dns set` help 保持包含 `<A记录>`。
- 版本升级到 `0.1.0-alpha.11`，同步更新安装脚本、README 和测试。

## 0.1.0-alpha.10

- Telegram 的 pending prompt 统一改为可追踪的编辑/删除流程，Cloudflare Token、Zone Name、DNS TTL、DNS A 记录、分组、节点名、节点 IP 和策略输入在成功或 `/cancel` 后都会清理提示消息。
- Agent 安装命令改为按节点缓存 join code，纯命令复制不会每次点击都生成新码，只有点“重新生成命令”才会刷新。
- DNS help 与向导文案继续对齐到 `<A记录>` 和 `example.com` 占位值，避免帮助文本和真实参数不一致。
- 版本升级到 `0.1.0-alpha.10`，同步更新安装脚本、README 和测试。

## 0.1.0-alpha.9

- Telegram 状态页在按钮入口下改为带导航的状态面板，支持“刷新状态 / DNS 配置 / 节点管理 / 返回主菜单”，并继续优先使用 `editMessageText` 更新当前消息。
- 分组管理新增分组详情与改名流程，`/groups rename <old> <new>` 继续保留兼容命令；分组详情可直接跳转到该组 DNS 或节点列表。
- DNS 管理新增详情页，支持修改域名、TTL、proxied，以及把记录改为指向某个节点；TTL 默认值改为 `60`，并支持 `1/auto` 自动 TTL。
- Agent 命令页改为“说明预览 + 纯安装命令 / 纯卸载命令”双层交互，便于在 Telegram 客户端里直接长按复制。
- 仓库内示例域名和示例 IP 全部统一为 `example.com` / `hk.example.com` 等占位值与 RFC 保留网段，并新增扫描测试防止旧示例残留。
- 统计模式、TTL 等选择页补齐回退入口，减少流程中断后只能依赖 `/cancel` 退出的情况。
- 版本升级到 `0.1.0-alpha.9`，同步更新 Master / Agent CLI、安装脚本、README、CHANGELOG 和测试。

## 0.1.0-alpha.7

- 修复 `.github/workflows/release.yml` 中 `Build release archives` 步骤的 shell 语法错误，移除 YAML `run` 块里的 heredoc，改用 `printf` 生成 `README.txt`。
- Release 工作流暂时收敛为仅构建 `linux/amd64`，发布 `qdr-master_linux_amd64.tar.gz`、`qdr-agent_linux_amd64.tar.gz` 和 `SHA256SUMS`。
- 安装脚本默认二进制下载 tag 升级到 `v0.1.0-alpha.7`，并固定下载 `linux_amd64` release 包。
- 版本升级到 `0.1.0-alpha.7`，同步更新 Master / Agent CLI、安装脚本、README 和测试。

## 0.1.0-alpha.6

- 安装器默认切换到 `QDR_INSTALL_MODE=binary`，优先从 GitHub Releases 下载预编译 `qdr-master` / `qdr-agent`，安装后强制做版本校验。
- 新增 `.github/workflows/release.yml`，在 tag 推送时构建 Linux amd64 / arm64 release 包，并发布 `SHA256SUMS`。
- 安装脚本新增 `binary / source / auto` 三种模式、`QDR_ALLOW_SOURCE_FALLBACK` 控制、二进制安装最小依赖和更明确的安装模式输出。
- 安装脚本按模式区分磁盘要求：二进制模式优先面向小磁盘 VPS，源码模式继续保留系统 Go / 包管理器 / 官方 tarball 的稳健 fallback。
- Telegram 节点创建流程默认简化为“分组 -> 节点名 -> 公网 IP -> 确认创建”，默认策略统一集中到 `/policy`，并补充节点详情页、节点策略修改、节点启停、自动切换开关和安装排查入口。
- DNS 向导在“记录存在但 IP 未匹配任何节点”时，新增一键改为指向已配置节点的修正分支。
- 版本升级到 `0.1.0-alpha.6`，同步更新 Master / Agent CLI、安装脚本、README 和测试。

## 0.1.0-alpha.5

- 调整 Telegram 推荐初始化顺序为 Master 公网地址 -> Cloudflare -> 分组 -> 节点 -> DNS -> Agent -> 状态，并把节点创建后的主推荐按钮改为 DNS 优先。
- Agent 安装命令页面增加节点、分组、Master URL、join code 到期时间，以及“无 DNS 记录”或“DNS IP 未匹配节点”的显式 warning。
- `/dns` 成功保存或创建后，直接提示匹配节点和下一步 Agent 安装；`/status` 与 `/nodes` 区分“未安装/未上线”和“离线”。
- `install-agent.sh` 新增 `--yes` 兼容参数、磁盘空间检查、系统 Go 优先复用、apt 优先安装、官方 Go tarball 临时目录解压与最后 50 行错误输出。
- `install-agent.sh` 构建后强制执行 `qdr-agent version` 校验；`install-master.sh` 同步增加磁盘空间检查和安全 Go fallback。
- 版本升级到 `0.1.0-alpha.5`，同步更新 Master / Agent CLI、安装脚本、README 和测试。

## 0.1.0-alpha.4

- Telegram 初始化流程改为“按钮 + 向导”优先，覆盖 Cloudflare、DNS、分组、节点、策略和 Agent 安装。
- `/cf` 新增 Token 输入、Zone 选择、手动输入 Zone Name、自动查询 Zone 列表和下一步按钮。
- `/dns` 新增分组选择、记录名逐步输入、记录不存在时的节点选择创建流程。
- `/groups`、`/nodes`、`/policy`、`/agent` 新增面板和向导式交互，保留命令行直传兼容。
- 抽象统一 pending 状态处理，错误输入不清状态，`/cancel` 可取消，切换向导会提示已切换。
- Cloudflare Token 回复和诊断保持脱敏，尝试删除 Token 输入消息但不因删除失败中断流程。
- Agent 安装命令改为显式 `--master` 参数，join code 有效期缩短为 30 分钟。
- 版本升级到 `0.1.0-alpha.4`，同步更新 Master / Agent CLI 与安装脚本版本输出。

## 0.1.0-alpha.3

- 修复 Telegram `/config_master_url` 等待输入状态在错误输入后丢失的问题。
- `/config_master_url <url>` 支持命令参数直传，错误后保持 pending 状态并允许重试。
- Master 公网地址支持公网 IP 自动补全为 `http://IP:8080`，并默认拒绝本机地址。
- Master 安装脚本尝试检测公网 IPv4，并写入 `QDR_SUGGESTED_PUBLIC_API_URL`。
- Telegram 配置 Master 公网地址时支持“一键使用当前公网地址”按钮。

## 0.1.0-alpha.2

- 修复 Master / Agent systemd 运行用户读取 env 文件和写入数据目录的权限问题。
- `qdr-master run` / `qdr-agent run` 默认从当前环境读取配置，避免 systemd 已注入 EnvironmentFile 后再次强制打开默认 env 文件。
- 增加 `qdr-master telegram-status` 轻量诊断命令。
- 安装脚本增加旧服务 stop/reset-failed、启动自检和 status/journal 排查提示。
- 卸载脚本增加 reset-failed，purge 路径保持幂等。

## 0.1.0-alpha.1

- 初始版本：Master / Agent CLI。
- SQLite migration 与核心数据表。
- Telegram Bot long polling 配置入口。
- Agent join code、Bearer Token 鉴权与流量上报。
- `/proc/net/dev` RX/TX 统计与计数器重置处理。
- Cloudflare DNS A 记录查询与更新客户端。
- 自动切换逻辑、阈值判断、节点选择和 cooldown。
- systemd 安装和卸载脚本。
- 中文 README 与基础测试。
- GitHub raw 一行安装脚本：Master / Agent 可自动下载源码并构建。
