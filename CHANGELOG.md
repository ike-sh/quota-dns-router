# CHANGELOG

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
