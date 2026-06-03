#!/usr/bin/env bash
set -euo pipefail

VERSION="0.1.0-alpha.5"
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
GO_TARBALL_MIN_SPACE_MB=800
JOIN_CODE=""
MASTER_URL="${QDR_MASTER_API_URL:-}"
YES=0
DRY_RUN=0
STAGE="初始化"
WORK_DIR=""
SRC_DIR=""
BUILD_DIR=""

usage() {
  cat <<'EOF'
用法：install-agent.sh --join <code> [--master <url>] [--yes] [--dry-run] [--help] [--version]

说明：
  --join / --code          Telegram 生成的加入码，必填
  --master                 Master 公网地址；未提供时读取 QDR_MASTER_API_URL
  --yes                    兼容参数，Agent 安装默认无交互
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
  echo "建议：确认 Master 公网地址可访问，必要时先执行 df -h 检查磁盘空间；systemd 失败时请查看 journalctl -u quota-dns-router-agent -n 100 --no-pager。" >&2
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
  go version 2>/dev/null | awk '{print $3}' | sed 's/^go//'
}

go_is_ready() {
  current="$(current_go_version)"
  [ -n "$current" ] && version_ge "$current" "$MIN_GO_VERSION"
}

check_disk_space() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] df -Pm /usr/local /tmp"
    return
  fi
  df -Pm /usr/local /tmp
}

available_space_mb() {
  df -Pm "$1" | awk 'NR==2 {print $4}'
}

ensure_space_for_go_fallback() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 检查 /usr/local 和 /tmp 可用空间是否 >= ${GO_TARBALL_MIN_SPACE_MB}MB"
    return
  fi
  local usr_local_avail tmp_avail
  usr_local_avail="$(available_space_mb /usr/local)"
  tmp_avail="$(available_space_mb /tmp)"
  if [ -z "$usr_local_avail" ] || [ -z "$tmp_avail" ]; then
    echo "无法检查 /usr/local 或 /tmp 的可用空间。"
    exit 1
  fi
  if [ "$usr_local_avail" -lt "$GO_TARBALL_MIN_SPACE_MB" ] || [ "$tmp_avail" -lt "$GO_TARBALL_MIN_SPACE_MB" ]; then
    echo "可用磁盘空间不足，无法安全使用官方 Go tarball fallback。"
    echo "请先执行 df -h，释放 /usr/local 和 /tmp 空间后重试。"
    exit 1
  fi
}

print_go_tar_failure() {
  local log_file="$1"
  echo "Go 工具链解压失败。"
  echo "可能是磁盘空间不足、下载不完整、权限问题。"
  echo "建议执行 df -h 后重新安装。"
  if [ -f "$log_file" ]; then
    tail -n 50 "$log_file" || true
  fi
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
  ensure_space_for_go_fallback
  go_arch="$(detect_go_arch)"
  url="https://go.dev/dl/go${GO_VERSION}.linux-${go_arch}.tar.gz"
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] GO_TMP=\"\$(mktemp -d)\""
    echo "[dry-run] curl -fL --retry 3 --connect-timeout 10 -o \"\$GO_TMP/go.tgz\" ${url}"
    echo "[dry-run] mkdir -p \"\$GO_TMP/extract\""
    echo "[dry-run] tar -C \"\$GO_TMP/extract\" -xzf \"\$GO_TMP/go.tgz\""
    echo "[dry-run] test -x \"\$GO_TMP/extract/go/bin/go\""
    echo "[dry-run] rm -rf /usr/local/go"
    echo "[dry-run] mv \"\$GO_TMP/extract/go\" /usr/local/go"
    return
  fi
  GO_TMP="$(mktemp -d)"
  curl -fL --retry 3 --connect-timeout 10 -o "${GO_TMP}/go.tgz" "$url"
  mkdir -p "${GO_TMP}/extract"
  tar_log="${GO_TMP}/go-tar.log"
  if ! tar -C "${GO_TMP}/extract" -xzf "${GO_TMP}/go.tgz" 2>"${tar_log}"; then
    print_go_tar_failure "${tar_log}"
    exit 1
  fi
  if [ ! -x "${GO_TMP}/extract/go/bin/go" ]; then
    echo "Go 工具链解压后未找到 go/bin/go。"
    print_go_tar_failure "${tar_log}"
    exit 1
  fi
  rm -rf /usr/local/go
  mv "${GO_TMP}/extract/go" /usr/local/go
  export PATH="/usr/local/go/bin:${PATH}"
  if ! go version >/dev/null 2>&1 || ! go_is_ready; then
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
    echo "[dry-run] ${BUILD_DIR:-/tmp/qdr-agent-build}/${BIN_NAME} version"
    echo "[dry-run] install -m 0755 qdr-agent ${PREFIX}/${BIN_NAME}"
    return
  fi
  BUILD_DIR="$(mktemp -d)"
  (cd "$SRC_DIR" && go build -o "${BUILD_DIR}/${BIN_NAME}" ./cmd/qdr-agent)
  built_version="$("${BUILD_DIR}/${BIN_NAME}" version | tr -d '\r')"
  expected_version="quota-dns-router agent ${VERSION}"
  if [ "$built_version" != "$expected_version" ]; then
    echo "qdr-agent version 校验失败：期望 ${expected_version}，实际 ${built_version}"
    exit 1
  fi
  install -m 0755 "${BUILD_DIR}/${BIN_NAME}" "${PREFIX}/${BIN_NAME}"
}

ensure_user() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 确保系统用户 quota-dns-router 存在"
    return
  fi
  if ! id quota-dns-router >/dev/null 2>&1; then
    useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin quota-dns-router
  fi
}

prepare_existing_service() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] systemctl stop quota-dns-router-agent.service 2>/dev/null || true"
    return
  fi
  systemctl stop quota-dns-router-agent.service 2>/dev/null || true
}

prepare_dirs() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] install -d -m 750 -o root -g quota-dns-router ${ETC_DIR}"
    echo "[dry-run] install -d -m 750 -o quota-dns-router -g quota-dns-router ${DATA_DIR} ${LOG_DIR}"
    return
  fi
  install -d -m 750 -o root -g quota-dns-router "$ETC_DIR"
  install -d -m 750 -o quota-dns-router -g quota-dns-router "$DATA_DIR" "$LOG_DIR"
  chown -R quota-dns-router:quota-dns-router "$DATA_DIR" "$LOG_DIR"
  chmod 750 "$DATA_DIR" "$LOG_DIR"
  if [ -f "${DATA_DIR}/agent-state.json" ]; then
    chown quota-dns-router:quota-dns-router "${DATA_DIR}/agent-state.json"
    chmod 600 "${DATA_DIR}/agent-state.json"
  fi
}

join_master() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] ${PREFIX}/${BIN_NAME} join --code <已隐藏> --master ${MASTER_URL} --env ${ETC_DIR}/agent.env"
    return
  fi
  "${PREFIX}/${BIN_NAME}" join --code "$JOIN_CODE" --master "$MASTER_URL" --env "${ETC_DIR}/agent.env"
  chown root:quota-dns-router "${ETC_DIR}/agent.env"
  chmod 0640 "${ETC_DIR}/agent.env"
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
User=quota-dns-router
Group=quota-dns-router
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
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] systemctl reset-failed quota-dns-router-agent.service 2>/dev/null || true"
  else
    systemctl reset-failed quota-dns-router-agent.service 2>/dev/null || true
  fi
  run systemctl enable --now quota-dns-router-agent.service
}

check_service() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] systemctl is-active --quiet quota-dns-router-agent.service"
    echo "[dry-run] 如失败，打印 systemctl status 和 journalctl 排查命令"
    return
  fi
  if ! systemctl is-active --quiet quota-dns-router-agent.service; then
    echo "Agent 服务未成功启动，请查看："
    echo "systemctl status quota-dns-router-agent --no-pager -l"
    echo "journalctl -u quota-dns-router-agent -n 100 --no-pager"
    systemctl status quota-dns-router-agent --no-pager -l || true
    journalctl -u quota-dns-router-agent -n 100 --no-pager || true
    exit 1
  fi
}

if [ -z "$JOIN_CODE" ]; then
  echo "缺少 --join <code>。请先在 Telegram 执行 /agent install <节点名> 生成安装命令。"
  exit 1
fi

if [ -z "$MASTER_URL" ]; then
  echo "缺少 Master 地址。请使用 --master <url>，或直接使用 Telegram 生成的完整命令。"
  exit 1
fi

step "[1/8] 检查系统环境"
require_root
detect_go_arch >/dev/null
check_disk_space

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
ensure_user
prepare_existing_service
prepare_dirs
join_master

step "[7/8] 写入 systemd 并启动 Agent"
write_service
start_service

step "[8/8] 安装完成"
check_service
echo "Agent 已安装并启动。"
echo "可执行：qdr-agent status"
echo "也可查看：systemctl status quota-dns-router-agent --no-pager -l"
echo "查看日志：journalctl -u quota-dns-router-agent -n 100 --no-pager"
