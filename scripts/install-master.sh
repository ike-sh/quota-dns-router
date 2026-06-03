#!/usr/bin/env bash
set -euo pipefail

PREFIX="/usr/local/bin"
ETC_DIR="/etc/quota-dns-router"
DATA_DIR="/var/lib/quota-dns-router"
LOG_DIR="/var/log/quota-dns-router"
UNIT="/etc/systemd/system/quota-dns-router-master.service"
BIN_NAME="qdr-master"

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    echo "请使用 root 运行安装脚本。"
    exit 1
  fi
}

detect_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) echo "amd64" ;;
    *) echo "暂不支持架构：$arch" >&2; exit 1 ;;
  esac
}

install_binary() {
  if [ -x "./${BIN_NAME}" ]; then
    install -m 0755 "./${BIN_NAME}" "${PREFIX}/${BIN_NAME}"
    return
  fi
  if command -v go >/dev/null 2>&1 && [ -f "go.mod" ]; then
    go build -o "${PREFIX}/${BIN_NAME}" ./cmd/qdr-master
    return
  fi
  echo "未找到 ./${BIN_NAME}，且当前目录无法 go build。请先放置二进制或在源码目录运行。"
  exit 1
}

require_root
detect_arch >/dev/null

read -r -p "Telegram Bot Token: " TG_TOKEN
read -r -p "Telegram 管理员 ID: " TG_ADMIN_ID
PUBLIC_API="http://127.0.0.1:8080"

mkdir -p "$ETC_DIR" "$DATA_DIR" "$LOG_DIR"
if ! id quota-dns-router >/dev/null 2>&1; then
  useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin quota-dns-router
fi
chown -R quota-dns-router:quota-dns-router "$DATA_DIR" "$LOG_DIR"

cat > "${ETC_DIR}/master.env" <<EOF
QDR_TELEGRAM_TOKEN=${TG_TOKEN}
QDR_TELEGRAM_ADMIN_ID=${TG_ADMIN_ID}
QDR_MASTER_LISTEN_ADDR=:8080
QDR_MASTER_PUBLIC_API_URL=${PUBLIC_API}
QDR_MASTER_DB_PATH=${DATA_DIR}/master.db
QDR_MASTER_DATA_DIR=${DATA_DIR}
QDR_MASTER_LOG_DIR=${LOG_DIR}
QDR_TELEGRAM_POLL_TIMEOUT=20s
QDR_CHECK_INTERVAL=60s
QDR_AGENT_OFFLINE_AFTER=300s
QDR_OFFLINE_NOTIFY_AFTER=600s
EOF
chmod 0600 "${ETC_DIR}/master.env"
chown root:quota-dns-router "${ETC_DIR}/master.env"

install_binary

cat > "$UNIT" <<'EOF'
[Unit]
Description=Quota DNS Router Master
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=quota-dns-router
Group=quota-dns-router
EnvironmentFile=/etc/quota-dns-router/master.env
ExecStart=/usr/local/bin/qdr-master run
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
systemctl enable --now quota-dns-router-master.service

echo "Master 已安装并启动。请在 Telegram 向 Bot 发送 /start 继续初始化。"
