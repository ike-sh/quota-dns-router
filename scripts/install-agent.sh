#!/usr/bin/env bash
set -euo pipefail

PREFIX="/usr/local/bin"
ETC_DIR="/etc/quota-dns-router"
DATA_DIR="/var/lib/quota-dns-router"
LOG_DIR="/var/log/quota-dns-router"
UNIT="/etc/systemd/system/quota-dns-router-agent.service"
BIN_NAME="qdr-agent"
JOIN_CODE=""
MASTER_URL=""

while [ $# -gt 0 ]; do
  case "$1" in
    --join|--code) JOIN_CODE="${2:-}"; shift 2 ;;
    --master) MASTER_URL="${2:-}"; shift 2 ;;
    --help) echo "用法：install-agent.sh --join <code> --master <url>"; exit 0 ;;
    *) echo "未知参数：$1"; exit 1 ;;
  esac
done

if [ "$(id -u)" -ne 0 ]; then
  echo "请使用 root 运行安装脚本。"
  exit 1
fi
if [ -z "$JOIN_CODE" ] || [ -z "$MASTER_URL" ]; then
  echo "缺少 --join <code> 或 --master <url>。请使用 Telegram Bot 生成完整命令。"
  exit 1
fi

mkdir -p "$ETC_DIR" "$DATA_DIR" "$LOG_DIR"

if [ -x "./${BIN_NAME}" ]; then
  install -m 0755 "./${BIN_NAME}" "${PREFIX}/${BIN_NAME}"
elif command -v go >/dev/null 2>&1 && [ -f "go.mod" ]; then
  go build -o "${PREFIX}/${BIN_NAME}" ./cmd/qdr-agent
else
  echo "未找到 ./${BIN_NAME}，且当前目录无法 go build。请先放置二进制或在源码目录运行。"
  exit 1
fi

"${PREFIX}/${BIN_NAME}" join --code "$JOIN_CODE" --master "$MASTER_URL" --env "${ETC_DIR}/agent.env"

cat > "$UNIT" <<'EOF'
[Unit]
Description=Quota DNS Router Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/quota-dns-router/agent.env
ExecStart=/usr/local/bin/qdr-agent run
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
ReadWritePaths=/var/lib/quota-dns-router /var/log/quota-dns-router

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now quota-dns-router-agent.service
echo "Agent 已安装并启动。"
