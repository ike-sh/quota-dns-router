# CHANGELOG

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
