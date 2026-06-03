#!/usr/bin/env bash
set -euo pipefail

VERSION="0.1.0-alpha.1"
PREFIX="/usr/local/bin"
ETC_DIR="/etc/quota-dns-router"
DATA_DIR="/var/lib/quota-dns-router"
LOG_DIR="/var/log/quota-dns-router"
UNIT="/etc/systemd/system/quota-dns-router-master.service"
BIN_NAME="qdr-master"
REPO="${QDR_REPO:-https://github.com/ike-sh/quota-dns-router}"
BRANCH="${QDR_BRANCH:-main}"
GO_VERSION="${QDR_GO_VERSION:-1.25.0}"
MIN_GO_VERSION="${QDR_MIN_GO_VERSION:-1.25.0}"
YES=0
DRY_RUN=0
STAGE="初始化"
WORK_DIR=""
SRC_DIR=""
BUILD_DIR=""

usage() {
  cat <<'EOF'
用法：install-master.sh [--yes] [--dry-run] [--help] [--version]

环境变量：
  QDR_TELEGRAM_BOT_TOKEN   Telegram Bot Token，--yes 时必填
  QDR_TELEGRAM_ADMIN_ID    Telegram 管理员 ID，--yes 时必填
  QDR_REPO                 GitHub 仓库，默认 https://github.com/ike-sh/quota-dns-router
  QDR_BRANCH               Git 分支，默认 main
  QDR_GO_VERSION           官方 Go tarball 版本，默认 1.25.0
EOF
}

version() {
  echo "quota-dns-router install-master ${VERSION}"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --yes|-y) YES=1 ;;
    --dry-run) DRY_RUN=1 ;;
    --help|-h) usage; exit 0 ;;
    --version) version; exit 0 ;;
    *) echo "未知参数：$1"; usage; exit 1 ;;
  esac
  shift
done

on_error() {
  code=$?
  echo
  echo "安装失败：${STAGE}" >&2
  echo "失败命令：${BASH_COMMAND}" >&2
  echo "建议：确认网络可访问 GitHub/raw.githubusercontent.com，或使用 --dry-run 检查参数；systemd 失败时请查看 journalctl -u quota-dns-router-master -n 100 --no-pager。" >&2
  exit "$code"
}

cleanup() {
  if [ -n "${WORK_DIR}" ] && [ -d "${WORK_DIR}" ] && [ "$DRY_RUN" -ne 1 ]; then
    rm -rf "${WORK_DIR}"
  fi
}

trap on_error ERR
trap cleanup EXIT

step() {
  STAGE="$1"
  echo "$1"
}

run() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] $*"
  else
    "$@"
  fi
}

require_root() {
  if [ "$DRY_RUN" -eq 1 ]; then
    return
  fi
  if [ "$(id -u)" -ne 0 ]; then
    echo "请使用 root 运行安装脚本，例如：sudo bash install-master.sh"
    exit 1
  fi
}

require_command() {
  command -v "$1" >/dev/null 2>&1
}

detect_go_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) echo "暂不支持架构：$arch" >&2; exit 1 ;;
  esac
}

version_ge() {
  local left="$1" right="$2"
  local la lb lc ra rb rc
  IFS=. read -r la lb lc <<< "$left"
  IFS=. read -r ra rb rc <<< "$right"
  la="${la:-0}"; lb="${lb:-0}"; lc="${lc:-0}"
  ra="${ra:-0}"; rb="${rb:-0}"; rc="${rc:-0}"
  [ "$la" -gt "$ra" ] && return 0
  [ "$la" -lt "$ra" ] && return 1
  [ "$lb" -gt "$rb" ] && return 0
  [ "$lb" -lt "$rb" ] && return 1
  [ "$lc" -ge "$rc" ]
}

current_go_version() {
  if ! require_command go; then
    echo ""
    return
  fi
  go env GOVERSION 2>/dev/null | sed 's/^go//'
}

go_is_ready() {
  current="$(current_go_version)"
  [ -n "$current" ] && version_ge "$current" "$MIN_GO_VERSION"
}

install_dependencies() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 安装 ca-certificates curl tar git build-essential"
    return
  fi
  if require_command apt-get; then
    run apt-get update
    run apt-get install -y ca-certificates curl tar git build-essential
  elif require_command dnf; then
    run dnf install -y ca-certificates curl tar git gcc gcc-c++ make
  elif require_command yum; then
    run yum install -y ca-certificates curl tar git gcc gcc-c++ make
  else
    echo "未识别包管理器，请先安装 ca-certificates、curl、tar、git、构建工具和 Go。"
    exit 1
  fi
}

try_install_distro_go() {
  if go_is_ready; then
    return
  fi
  if require_command apt-get; then
    run apt-get install -y golang-go || true
  elif require_command dnf; then
    run dnf install -y golang || true
  elif require_command yum; then
    run yum install -y golang || true
  fi
}

install_official_go() {
  if go_is_ready; then
    return
  fi
  go_arch="$(detect_go_arch)"
  url="https://go.dev/dl/go${GO_VERSION}.linux-${go_arch}.tar.gz"
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] curl -fsSL ${url} -o /tmp/go.tgz"
    echo "[dry-run] 安装 Go 到 /usr/local/go"
    return
  fi
  go_tmp="$(mktemp -d)"
  curl -fsSL "$url" -o "${go_tmp}/go.tgz"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "${go_tmp}/go.tgz"
  export PATH="/usr/local/go/bin:${PATH}"
  if ! go_is_ready; then
    echo "Go 工具链安装后仍不可用，请检查 /usr/local/go/bin/go。"
    exit 1
  fi
}

prepare_source() {
  if [ -f "./go.mod" ] && [ -d "./cmd/qdr-master" ]; then
    SRC_DIR="$(pwd)"
    echo "使用当前源码目录：${SRC_DIR}"
    return
  fi
  if [ "$DRY_RUN" -eq 1 ]; then
    SRC_DIR="/tmp/quota-dns-router-src"
    echo "[dry-run] 下载 ${REPO} (${BRANCH}) 到临时目录"
    return
  fi
  WORK_DIR="$(mktemp -d)"
  SRC_DIR="${WORK_DIR}/src"
  if require_command git; then
    git clone --depth 1 --branch "$BRANCH" "$REPO" "$SRC_DIR"
  else
    mkdir -p "$SRC_DIR"
    repo_no_git="${REPO%.git}"
    curl -fsSL "${repo_no_git}/archive/refs/heads/${BRANCH}.tar.gz" | tar -xz -C "$SRC_DIR" --strip-components=1
  fi
}

build_master() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] cd ${SRC_DIR} && go build -o qdr-master ./cmd/qdr-master"
    echo "[dry-run] install -m 0755 qdr-master ${PREFIX}/${BIN_NAME}"
    return
  fi
  BUILD_DIR="$(mktemp -d)"
  (cd "$SRC_DIR" && go build -o "${BUILD_DIR}/${BIN_NAME}" ./cmd/qdr-master)
  install -m 0755 "${BUILD_DIR}/${BIN_NAME}" "${PREFIX}/${BIN_NAME}"
}

write_config() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 创建 ${ETC_DIR} ${DATA_DIR} ${LOG_DIR}"
    echo "[dry-run] 写入 ${ETC_DIR}/master.env（Token 已隐藏）"
    return
  fi
  mkdir -p "$ETC_DIR" "$DATA_DIR" "$LOG_DIR"
  if ! id quota-dns-router >/dev/null 2>&1; then
    useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin quota-dns-router
  fi
  chown -R quota-dns-router:quota-dns-router "$DATA_DIR" "$LOG_DIR"
  cat > "${ETC_DIR}/master.env" <<EOF
QDR_TELEGRAM_TOKEN=${TG_TOKEN}
QDR_TELEGRAM_ADMIN_ID=${TG_ADMIN_ID}
QDR_MASTER_LISTEN_ADDR=:8080
QDR_MASTER_PUBLIC_API_URL=http://127.0.0.1:8080
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
}

start_service() {
  run systemctl daemon-reload
  run systemctl enable --now quota-dns-router-master.service
}

collect_config() {
  TG_TOKEN="${QDR_TELEGRAM_BOT_TOKEN:-${QDR_TELEGRAM_TOKEN:-}}"
  TG_ADMIN_ID="${QDR_TELEGRAM_ADMIN_ID:-}"
  if [ "$YES" -eq 1 ]; then
    if [ -z "$TG_TOKEN" ] || [ -z "$TG_ADMIN_ID" ]; then
      echo "--yes 模式需要提前设置 QDR_TELEGRAM_BOT_TOKEN 和 QDR_TELEGRAM_ADMIN_ID。"
      exit 1
    fi
    return
  fi
  if [ "$DRY_RUN" -eq 1 ]; then
    TG_TOKEN="dry-run-token"
    TG_ADMIN_ID="0"
    return
  fi
  read -r -p "Telegram Bot Token: " TG_TOKEN
  read -r -p "Telegram 管理员 ID: " TG_ADMIN_ID
}

collect_config

step "[1/8] 检查系统环境"
require_root
detect_go_arch >/dev/null

step "[2/8] 安装依赖"
install_dependencies

step "[3/8] 准备 Go 工具链"
try_install_distro_go
install_official_go

step "[4/8] 下载 quota-dns-router 源码"
prepare_source

step "[5/8] 构建 qdr-master"
build_master

step "[6/8] 写入配置和 systemd"
write_config

step "[7/8] 启动 Master 服务"
start_service

step "[8/8] 安装完成"
echo "Master 已安装并启动。"
echo "下一步：在 Telegram 向 Bot 发送 /start，然后配置 Master 公网地址、Cloudflare、DNS、节点和 Agent。"
