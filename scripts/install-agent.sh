#!/usr/bin/env bash
set -euo pipefail

VERSION="0.1.0-alpha.1"
PREFIX="/usr/local/bin"
ETC_DIR="/etc/quota-dns-router"
DATA_DIR="/var/lib/quota-dns-router"
LOG_DIR="/var/log/quota-dns-router"
UNIT="/etc/systemd/system/quota-dns-router-agent.service"
BIN_NAME="qdr-agent"
REPO="${QDR_REPO:-https://github.com/ike-sh/quota-dns-router}"
BRANCH="${QDR_BRANCH:-main}"
GO_VERSION="${QDR_GO_VERSION:-1.25.0}"
MIN_GO_VERSION="${QDR_MIN_GO_VERSION:-1.25.0}"
JOIN_CODE=""
MASTER_URL="${QDR_MASTER_API_URL:-}"
DRY_RUN=0
STAGE="初始化"
WORK_DIR=""
SRC_DIR=""
BUILD_DIR=""

usage() {
  cat <<'EOF'
用法：install-agent.sh --join <code> [--master <url>] [--dry-run] [--help] [--version]

说明：
  --join / --code          Telegram 生成的加入码，必填
  --master                 Master 公网地址；未提供时读取 QDR_MASTER_API_URL
  QDR_REPO                 GitHub 仓库，默认 https://github.com/ike-sh/quota-dns-router
  QDR_BRANCH               Git 分支，默认 main
EOF
}

version() {
  echo "quota-dns-router install-agent ${VERSION}"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --join|--code)
      if [ $# -lt 2 ]; then
        echo "缺少 --join <code> 的参数值。"
        exit 1
      fi
      JOIN_CODE="${2:-}"
      shift 2
      continue
      ;;
    --master)
      if [ $# -lt 2 ]; then
        echo "缺少 --master <url> 的参数值。"
        exit 1
      fi
      MASTER_URL="${2:-}"
      shift 2
      continue
      ;;
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
  echo "建议：确认 Master 公网地址可访问，或先运行 qdr-master /agent install 重新生成安装命令；systemd 失败时请查看 journalctl -u quota-dns-router-agent -n 100 --no-pager。" >&2
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
    echo "请使用 root 运行安装脚本，例如：sudo bash install-agent.sh --join <code> --master <url>"
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
  if [ -f "./go.mod" ] && [ -d "./cmd/qdr-agent" ]; then
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

build_agent() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] cd ${SRC_DIR} && go build -o qdr-agent ./cmd/qdr-agent"
    echo "[dry-run] install -m 0755 qdr-agent ${PREFIX}/${BIN_NAME}"
    return
  fi
  BUILD_DIR="$(mktemp -d)"
  (cd "$SRC_DIR" && go build -o "${BUILD_DIR}/${BIN_NAME}" ./cmd/qdr-agent)
  install -m 0755 "${BUILD_DIR}/${BIN_NAME}" "${PREFIX}/${BIN_NAME}"
}

join_master() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] ${PREFIX}/${BIN_NAME} join --code <已隐藏> --master ${MASTER_URL} --env ${ETC_DIR}/agent.env"
    return
  fi
  mkdir -p "$ETC_DIR" "$DATA_DIR" "$LOG_DIR"
  "${PREFIX}/${BIN_NAME}" join --code "$JOIN_CODE" --master "$MASTER_URL" --env "${ETC_DIR}/agent.env"
}

write_service() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 写入 ${UNIT}"
    return
  fi
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
}

start_service() {
  run systemctl daemon-reload
  run systemctl enable --now quota-dns-router-agent.service
}

if [ -z "$JOIN_CODE" ]; then
  echo "缺少 --join <code>。请先在 Telegram 执行 /agent install <节点名> 生成安装命令。"
  exit 1
fi

if [ -z "$MASTER_URL" ]; then
  echo "缺少 Master 公网地址。请使用 --master <url>，或设置 QDR_MASTER_API_URL。"
  exit 1
fi

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

step "[5/8] 构建 qdr-agent"
build_agent

step "[6/8] 加入 Master 并写入配置"
join_master

step "[7/8] 写入 systemd 并启动 Agent"
write_service
start_service

step "[8/8] 安装完成"
echo "Agent 已安装并启动。"
echo "可执行：qdr-agent status"
echo "也可查看：systemctl status quota-dns-router-agent --no-pager"
