#!/usr/bin/env bash
set -euo pipefail

VERSION="0.1.0-alpha.11"
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
INSTALL_MODE="${QDR_INSTALL_MODE:-binary}"
ALLOW_SOURCE_FALLBACK="${QDR_ALLOW_SOURCE_FALLBACK:-0}"

BINARY_ROOT_MIN_SPACE_MB=80
BINARY_TMP_MIN_SPACE_MB=80
SOURCE_ROOT_MIN_SPACE_MB=800
SOURCE_TMP_MIN_SPACE_MB=500
SOURCE_USR_LOCAL_MIN_SPACE_MB=800

YES=0
DRY_RUN=0
STAGE="初始化"
WORK_DIR=""
SRC_DIR=""
BUILD_DIR=""
GO_TMP_DIR=""
DETECTED_PUBLIC_IP=""
SUGGESTED_PUBLIC_API_URL=""
TG_TOKEN=""
TG_ADMIN_ID=""

usage() {
  cat <<'EOF'
用法：install-master.sh [--yes] [--dry-run] [--help] [--version]

环境变量：
  QDR_TELEGRAM_BOT_TOKEN     Telegram Bot Token，--yes 时必填
  QDR_TELEGRAM_ADMIN_ID      Telegram 管理员 ID，--yes 时必填
  QDR_INSTALL_MODE           安装模式：binary / source / auto，默认 binary
  QDR_ALLOW_SOURCE_FALLBACK  auto + --yes 时允许失败后继续 source，设为 1 开启
  QDR_REPO                   GitHub 仓库，默认 https://github.com/ike-sh/quota-dns-router
  QDR_BRANCH                 Git 分支，默认 main
  QDR_GO_VERSION             官方 Go tarball 版本，默认 1.25.0
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
    *)
      echo "未知参数：$1"
      usage
      exit 1
      ;;
  esac
  shift
done

cleanup() {
  for dir in "$WORK_DIR" "$BUILD_DIR" "$GO_TMP_DIR"; do
    if [ -n "$dir" ] && [ -d "$dir" ] && [ "$DRY_RUN" -ne 1 ]; then
      rm -rf "$dir"
    fi
  done
}

on_error() {
  code=$?
  echo
  echo "安装失败：${STAGE}" >&2
  echo "失败命令：${BASH_COMMAND}" >&2
  echo "建议：先执行 df -h 检查 /、/tmp、/usr/local 空间；服务启动失败时查看 journalctl -u quota-dns-router-master -n 100 --no-pager。" >&2
  exit "$code"
}

trap cleanup EXIT
trap on_error ERR

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
    return 0
  fi
  if [ "$(id -u)" -ne 0 ]; then
    echo "请使用 root 运行安装脚本，例如：sudo bash install-master.sh" >&2
    return 1
  fi
}

require_command() {
  command -v "$1" >/dev/null 2>&1
}

normalize_install_mode() {
  case "${INSTALL_MODE}" in
    binary|source|auto) ;;
    *)
      echo "不支持的 QDR_INSTALL_MODE：${INSTALL_MODE}，可选值：binary / source / auto" >&2
      return 1
      ;;
  esac
}

detect_linux_arch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      echo "暂不支持架构：$arch" >&2
      return 1
      ;;
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
    return 0
  fi
  go version 2>/dev/null | awk '{print $3}' | sed 's/^go//'
}

go_is_ready() {
  local current
  current="$(current_go_version)"
  [ -n "$current" ] && version_ge "$current" "$MIN_GO_VERSION"
}

available_space_mb() {
  df -Pm "$1" | awk 'NR==2 {print $4}'
}

print_disk_space_error() {
  local component="$1"
  local root_avail="$2"
  local tmp_avail="$3"
  local need_root="$4"
  local need_tmp="$5"
  local need_usr_local="${6:-0}"
  local usr_local_avail="${7:-0}"
  local mode_label="${8:-二进制安装}"

  echo "错误：磁盘空间不足，无法安装 ${component}。"
  echo
  echo "当前空间："
  printf "/      可用 %sMB\n" "$root_avail"
  printf "/tmp   可用 %sMB\n" "$tmp_avail"
  if [ "$need_usr_local" -gt 0 ]; then
    printf "/usr/local 可用 %sMB\n" "$usr_local_avail"
  fi
  echo
  echo "${mode_label}至少需要："
  printf "/      %sMB\n" "$need_root"
  printf "/tmp   %sMB\n" "$need_tmp"
  if [ "$need_usr_local" -gt 0 ]; then
    printf "/usr/local %sMB\n" "$need_usr_local"
  fi
  echo
  echo "请先清理磁盘后重试："
  echo "apt clean"
  echo "journalctl --vacuum-size=100M"
  echo "docker system prune -af"
  echo "df -h"
}

ensure_binary_disk_space() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 检查 / 和 /tmp 可用空间是否分别 >= ${BINARY_ROOT_MIN_SPACE_MB}MB / ${BINARY_TMP_MIN_SPACE_MB}MB"
    return 0
  fi
  local root_avail tmp_avail
  root_avail="$(available_space_mb /)"
  tmp_avail="$(available_space_mb /tmp)"
  if [ -z "$root_avail" ] || [ -z "$tmp_avail" ]; then
    echo "无法检查 / 或 /tmp 的可用空间。" >&2
    return 1
  fi
  if [ "$root_avail" -lt "$BINARY_ROOT_MIN_SPACE_MB" ] || [ "$tmp_avail" -lt "$BINARY_TMP_MIN_SPACE_MB" ]; then
    print_disk_space_error "Master" "$root_avail" "$tmp_avail" "$BINARY_ROOT_MIN_SPACE_MB" "$BINARY_TMP_MIN_SPACE_MB"
    return 1
  fi
}

ensure_source_disk_space() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 检查 / 和 /tmp 可用空间是否分别 >= ${SOURCE_ROOT_MIN_SPACE_MB}MB / ${SOURCE_TMP_MIN_SPACE_MB}MB"
    return 0
  fi
  local root_avail tmp_avail
  root_avail="$(available_space_mb /)"
  tmp_avail="$(available_space_mb /tmp)"
  if [ -z "$root_avail" ] || [ -z "$tmp_avail" ]; then
    echo "无法检查 / 或 /tmp 的可用空间。" >&2
    return 1
  fi
  if [ "$root_avail" -lt "$SOURCE_ROOT_MIN_SPACE_MB" ] || [ "$tmp_avail" -lt "$SOURCE_TMP_MIN_SPACE_MB" ]; then
    print_disk_space_error "Master" "$root_avail" "$tmp_avail" "$SOURCE_ROOT_MIN_SPACE_MB" "$SOURCE_TMP_MIN_SPACE_MB" 0 0 "源码构建"
    return 1
  fi
}

ensure_space_for_go_fallback() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 检查 /usr/local 和 /tmp 可用空间是否分别 >= ${SOURCE_USR_LOCAL_MIN_SPACE_MB}MB / ${SOURCE_TMP_MIN_SPACE_MB}MB"
    return 0
  fi
  local root_avail tmp_avail usr_local_avail
  root_avail="$(available_space_mb /)"
  tmp_avail="$(available_space_mb /tmp)"
  usr_local_avail="$(available_space_mb /usr/local)"
  if [ -z "$root_avail" ] || [ -z "$tmp_avail" ] || [ -z "$usr_local_avail" ]; then
    echo "无法检查 /、/tmp 或 /usr/local 的可用空间。" >&2
    return 1
  fi
  if [ "$usr_local_avail" -lt "$SOURCE_USR_LOCAL_MIN_SPACE_MB" ] || [ "$tmp_avail" -lt "$SOURCE_TMP_MIN_SPACE_MB" ] || [ "$root_avail" -lt "$SOURCE_ROOT_MIN_SPACE_MB" ]; then
    print_disk_space_error "Master" "$root_avail" "$tmp_avail" "$SOURCE_ROOT_MIN_SPACE_MB" "$SOURCE_TMP_MIN_SPACE_MB" "$SOURCE_USR_LOCAL_MIN_SPACE_MB" "$usr_local_avail" "源码构建（Go tarball fallback）"
    return 1
  fi
}

install_binary_dependencies() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 安装最小依赖 ca-certificates curl tar"
    return 0
  fi
  if require_command apt-get; then
    run apt-get update
    run apt-get install -y ca-certificates curl tar
    return 0
  fi
  if require_command dnf; then
    run dnf install -y ca-certificates curl tar
    return 0
  fi
  if require_command yum; then
    run yum install -y ca-certificates curl tar
    return 0
  fi

  local missing=()
  require_command curl || missing+=("curl")
  require_command tar || missing+=("tar")
  if [ "${#missing[@]}" -gt 0 ]; then
    echo "系统没有可用包管理器，且缺少命令：${missing[*]}。" >&2
    return 1
  fi
}

install_source_dependencies() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 安装 ca-certificates curl tar git build-essential"
    return 0
  fi
  if require_command apt-get; then
    run apt-get update
    run apt-get install -y ca-certificates curl tar git build-essential
    return 0
  fi
  if require_command dnf; then
    run dnf install -y ca-certificates curl tar git gcc gcc-c++ make
    return 0
  fi
  if require_command yum; then
    run yum install -y ca-certificates curl tar git gcc gcc-c++ make
    return 0
  fi

  local missing=()
  require_command curl || missing+=("curl")
  require_command tar || missing+=("tar")
  if [ "${#missing[@]}" -gt 0 ]; then
    echo "系统没有可用包管理器，且缺少命令：${missing[*]}。" >&2
    return 1
  fi
}

try_install_distro_go() {
  if go_is_ready; then
    return 0
  fi
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 尝试通过系统包管理器安装 Go"
    return 0
  fi
  if require_command apt-get; then
    run apt-get install -y golang-go || true
  elif require_command dnf; then
    run dnf install -y golang || true
  elif require_command yum; then
    run yum install -y golang || true
  fi
}

print_go_tar_failure() {
  local log_file="$1"
  echo "Go 工具链解压失败。"
  echo "可能是磁盘空间不足、下载不完整或权限问题。"
  echo "建议先执行 df -h 再重试。"
  if [ -f "$log_file" ]; then
    tail -n 50 "$log_file" || true
  fi
}

install_official_go() {
  local go_arch url tar_log

  if go_is_ready; then
    return 0
  fi

  ensure_space_for_go_fallback
  go_arch="$(detect_linux_arch)"
  url="https://go.dev/dl/go${GO_VERSION}.linux-${go_arch}.tar.gz"

  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] GO_TMP=\"\$(mktemp -d)\""
    echo "[dry-run] curl -fL --retry 3 --connect-timeout 10 -o \"\$GO_TMP/go.tgz\" ${url}"
    echo "[dry-run] mkdir -p \"\$GO_TMP/extract\""
    echo "[dry-run] tar -C \"\$GO_TMP/extract\" -xzf \"\$GO_TMP/go.tgz\""
    echo "[dry-run] test -x \"\$GO_TMP/extract/go/bin/go\""
    echo "[dry-run] rm -rf /usr/local/go"
    echo "[dry-run] mv \"\$GO_TMP/extract/go\" /usr/local/go"
    return 0
  fi

  GO_TMP_DIR="$(mktemp -d)"
  curl -fL --retry 3 --connect-timeout 10 -o "${GO_TMP_DIR}/go.tgz" "$url"
  mkdir -p "${GO_TMP_DIR}/extract"
  tar_log="${GO_TMP_DIR}/go-tar.log"
  if ! tar -C "${GO_TMP_DIR}/extract" -xzf "${GO_TMP_DIR}/go.tgz" 2>"${tar_log}"; then
    print_go_tar_failure "${tar_log}"
    return 1
  fi
  if [ ! -x "${GO_TMP_DIR}/extract/go/bin/go" ]; then
    echo "Go 工具链解压后未找到 go/bin/go。" >&2
    print_go_tar_failure "${tar_log}"
    return 1
  fi
  rm -rf /usr/local/go
  mv "${GO_TMP_DIR}/extract/go" /usr/local/go
  export PATH="/usr/local/go/bin:${PATH}"
  if ! go version >/dev/null 2>&1 || ! go_is_ready; then
    echo "Go 工具链安装后仍不可用，请检查 /usr/local/go/bin/go。" >&2
    return 1
  fi
}

prepare_source() {
  local repo_no_git

  if [ -f "./go.mod" ] && [ -d "./cmd/qdr-master" ]; then
    SRC_DIR="$(pwd)"
    echo "使用当前源码目录：${SRC_DIR}"
    return 0
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    SRC_DIR="/tmp/quota-dns-router-src"
    echo "[dry-run] 下载 ${REPO} (${BRANCH}) 到临时目录"
    return 0
  fi

  WORK_DIR="$(mktemp -d)"
  if require_command git; then
    git clone --depth 1 --branch "$BRANCH" "$REPO" "${WORK_DIR}/src"
    SRC_DIR="${WORK_DIR}/src"
    return 0
  fi

  repo_no_git="${REPO%.git}"
  mkdir -p "${WORK_DIR}/src"
  curl -fsSL "${repo_no_git}/archive/refs/heads/${BRANCH}.tar.gz" | tar -xz -C "${WORK_DIR}/src" --strip-components=1
  SRC_DIR="${WORK_DIR}/src"
}

verify_binary_version() {
  local binary_path="$1"
  local expected="$2"
  local actual

  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] ${binary_path} version"
    return 0
  fi

  actual="$("${binary_path}" version | tr -d '\r')"
  if [ "$actual" != "$expected" ]; then
    echo "${BIN_NAME} version 校验失败：期望 ${expected}，实际 ${actual}" >&2
    return 1
  fi
}

build_master_from_source() {
  local expected

  expected="quota-dns-router master ${VERSION}"
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] cd ${SRC_DIR} && CGO_ENABLED=0 go build -trimpath -ldflags=\"-s -w\" -o qdr-master ./cmd/qdr-master"
    echo "[dry-run] install -m 0755 qdr-master ${PREFIX}/${BIN_NAME}"
    echo "[dry-run] ${PREFIX}/${BIN_NAME} version"
    return 0
  fi

  BUILD_DIR="$(mktemp -d)"
  (
    cd "$SRC_DIR"
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "${BUILD_DIR}/${BIN_NAME}" ./cmd/qdr-master
  )
  install -m 0755 "${BUILD_DIR}/${BIN_NAME}" "${PREFIX}/${BIN_NAME}"
  verify_binary_version "${PREFIX}/${BIN_NAME}" "$expected"
}

verify_sha256() {
  local package_path="$1"
  local sums_path="$2"
  local package_name="$3"
  local expected actual

  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 校验 ${package_name} 的 SHA256"
    return 0
  fi

  expected="$(grep "  ${package_name}\$" "$sums_path" | awk '{print $1}' | head -n 1 || true)"
  if [ -z "$expected" ]; then
    echo "SHA256SUMS 中未找到 ${package_name}。" >&2
    return 1
  fi

  if require_command sha256sum; then
    (
      cd "$(dirname "$package_path")"
      grep "  ${package_name}\$" "$sums_path" | sha256sum -c -
    )
    return 0
  fi
  if require_command shasum; then
    actual="$(shasum -a 256 "$package_path" | awk '{print $1}')"
  elif require_command openssl; then
    actual="$(openssl dgst -sha256 "$package_path" | awk '{print $NF}')"
  else
    echo "警告：未找到 sha256sum/shasum/openssl，跳过 SHA256 校验。"
    return 0
  fi

  if [ "$actual" != "$expected" ]; then
    echo "SHA256 校验失败：${package_name}" >&2
    return 1
  fi
}

install_master_from_release() {
  local arch repo_no_git release_base package_name package_url sums_url expected

  arch="$(detect_linux_arch)"
  if [ "$arch" != "amd64" ]; then
    echo "binary release currently supports linux/amd64 only; detected ${arch}. Please set QDR_INSTALL_MODE=source." >&2
    return 1
  fi
  repo_no_git="${REPO%.git}"
  release_base="${repo_no_git}/releases/download/v${VERSION}"
  package_name="${BIN_NAME}_linux_amd64.tar.gz"
  package_url="${release_base}/${package_name}"
  sums_url="${release_base}/SHA256SUMS"
  expected="quota-dns-router master ${VERSION}"

  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] TMP=\"\$(mktemp -d)\""
    echo "[dry-run] curl -fL --retry 3 --connect-timeout 10 -o \"\$TMP/${package_name}\" ${package_url}"
    echo "[dry-run] curl -fL --retry 3 --connect-timeout 10 -o \"\$TMP/SHA256SUMS\" ${sums_url}"
    echo "[dry-run] 校验 ${package_name} 的 SHA256"
    echo "[dry-run] mkdir -p \"\$TMP/extract\""
    echo "[dry-run] tar -C \"\$TMP/extract\" -xzf \"\$TMP/${package_name}\""
    echo "[dry-run] test -x \"\$TMP/extract/${BIN_NAME}\""
    echo "[dry-run] install -m 0755 \"\$TMP/extract/${BIN_NAME}\" ${PREFIX}/${BIN_NAME}"
    echo "[dry-run] ${PREFIX}/${BIN_NAME} version"
    return 0
  fi

  WORK_DIR="$(mktemp -d)"
  curl -fL --retry 3 --connect-timeout 10 -o "${WORK_DIR}/${package_name}" "${package_url}"
  curl -fL --retry 3 --connect-timeout 10 -o "${WORK_DIR}/SHA256SUMS" "${sums_url}"
  verify_sha256 "${WORK_DIR}/${package_name}" "${WORK_DIR}/SHA256SUMS" "${package_name}"
  mkdir -p "${WORK_DIR}/extract"
  tar -C "${WORK_DIR}/extract" -xzf "${WORK_DIR}/${package_name}"
  if [ ! -x "${WORK_DIR}/extract/${BIN_NAME}" ]; then
    echo "解压后的 release 包中未找到可执行文件 ${BIN_NAME}。" >&2
    return 1
  fi
  install -m 0755 "${WORK_DIR}/extract/${BIN_NAME}" "${PREFIX}/${BIN_NAME}"
  verify_binary_version "${PREFIX}/${BIN_NAME}" "${expected}"
}

detect_public_ip() {
  local ip endpoint

  if [ "$DRY_RUN" -eq 1 ]; then
    DETECTED_PUBLIC_IP="${QDR_DETECTED_PUBLIC_IP:-203.0.113.10}"
    SUGGESTED_PUBLIC_API_URL="http://${DETECTED_PUBLIC_IP}:8080"
    echo "[dry-run] 检测公网 IPv4：${DETECTED_PUBLIC_IP}"
    return 0
  fi

  for endpoint in \
    "https://api.ipify.org" \
    "https://ifconfig.me/ip" \
    "https://icanhazip.com"; do
    ip="$(curl -4fsS --max-time 3 "$endpoint" 2>/dev/null | tr -d '\r\n[:space:]' || true)"
    if printf '%s' "$ip" | grep -Eq '^[0-9]{1,3}(\.[0-9]{1,3}){3}$'; then
      DETECTED_PUBLIC_IP="$ip"
      SUGGESTED_PUBLIC_API_URL="http://${ip}:8080"
      return 0
    fi
  done
}

ensure_user() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 确保系统用户 quota-dns-router 存在"
    return 0
  fi
  if ! id quota-dns-router >/dev/null 2>&1; then
    useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin quota-dns-router
  fi
}

prepare_existing_service() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] systemctl stop quota-dns-router-master.service 2>/dev/null || true"
    return 0
  fi
  systemctl stop quota-dns-router-master.service 2>/dev/null || true
}

write_config() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] install -d -m 750 -o root -g quota-dns-router ${ETC_DIR}"
    echo "[dry-run] install -d -m 750 -o quota-dns-router -g quota-dns-router ${DATA_DIR} ${LOG_DIR}"
    echo "[dry-run] 写入 ${ETC_DIR}/master.env（Token 已隐藏），权限 root:quota-dns-router 640"
    echo "[dry-run] QDR_DETECTED_PUBLIC_IP=${DETECTED_PUBLIC_IP}"
    echo "[dry-run] QDR_SUGGESTED_PUBLIC_API_URL=${SUGGESTED_PUBLIC_API_URL}"
    return 0
  fi

  install -d -m 750 -o root -g quota-dns-router "$ETC_DIR"
  install -d -m 750 -o quota-dns-router -g quota-dns-router "$DATA_DIR" "$LOG_DIR"
  chown -R quota-dns-router:quota-dns-router "$DATA_DIR" "$LOG_DIR"
  chmod 750 "$DATA_DIR" "$LOG_DIR"
  if [ -f "${DATA_DIR}/master.db" ]; then
    chown quota-dns-router:quota-dns-router "${DATA_DIR}/master.db"
    chmod 600 "${DATA_DIR}/master.db"
  fi

  cat > "${ETC_DIR}/master.env" <<EOF
QDR_TELEGRAM_TOKEN=${TG_TOKEN}
QDR_TELEGRAM_ADMIN_ID=${TG_ADMIN_ID}
QDR_MASTER_LISTEN_ADDR=:8080
QDR_MASTER_PUBLIC_API_URL=http://127.0.0.1:8080
QDR_MASTER_DB_PATH=${DATA_DIR}/master.db
QDR_MASTER_DATA_DIR=${DATA_DIR}
QDR_MASTER_LOG_DIR=${LOG_DIR}
QDR_DETECTED_PUBLIC_IP=${DETECTED_PUBLIC_IP}
QDR_SUGGESTED_PUBLIC_API_URL=${SUGGESTED_PUBLIC_API_URL}
QDR_TELEGRAM_POLL_TIMEOUT=20s
QDR_CHECK_INTERVAL=60s
QDR_AGENT_OFFLINE_AFTER=300s
QDR_OFFLINE_NOTIFY_AFTER=600s
EOF
  chown root:quota-dns-router "${ETC_DIR}/master.env"
  chmod 0640 "${ETC_DIR}/master.env"

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
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] systemctl reset-failed quota-dns-router-master.service 2>/dev/null || true"
  else
    systemctl reset-failed quota-dns-router-master.service 2>/dev/null || true
  fi
  run systemctl enable --now quota-dns-router-master.service
}

check_service() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] systemctl is-active --quiet quota-dns-router-master.service"
    echo "[dry-run] 如失败，打印 systemctl status 和 journalctl 排查命令"
    return 0
  fi
  if ! systemctl is-active --quiet quota-dns-router-master.service; then
    echo "Master 服务未成功启动，请查看："
    echo "systemctl status quota-dns-router-master --no-pager -l"
    echo "journalctl -u quota-dns-router-master -n 100 --no-pager"
    systemctl status quota-dns-router-master --no-pager -l || true
    journalctl -u quota-dns-router-master -n 100 --no-pager || true
    return 1
  fi
}

collect_config() {
  TG_TOKEN="${QDR_TELEGRAM_BOT_TOKEN:-${QDR_TELEGRAM_TOKEN:-}}"
  TG_ADMIN_ID="${QDR_TELEGRAM_ADMIN_ID:-}"

  if [ "$YES" -eq 1 ]; then
    if [ -z "$TG_TOKEN" ] || [ -z "$TG_ADMIN_ID" ]; then
      echo "--yes 模式需要提前设置 QDR_TELEGRAM_BOT_TOKEN 和 QDR_TELEGRAM_ADMIN_ID。" >&2
      return 1
    fi
    return 0
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    TG_TOKEN="dry-run-token"
    TG_ADMIN_ID="0"
    return 0
  fi

  read -r -p "Telegram Bot Token: " TG_TOKEN
  read -r -p "Telegram 管理员 ID: " TG_ADMIN_ID
}

print_binary_banner() {
  local arch
  arch="$(detect_linux_arch)"
  version
  echo "安装模式：binary"
  echo "来源：GitHub Releases"
  echo "架构：linux/${arch}"
}

print_source_banner() {
  version
  echo "安装模式：source"
  echo "来源：GitHub source ${BRANCH}"
}

print_auto_banner() {
  local arch
  arch="$(detect_linux_arch)"
  version
  echo "安装模式：auto"
  echo "先尝试：GitHub Releases"
  echo "架构：linux/${arch}"
}

install_master_binary_mode() {
  print_binary_banner
  step "[1/7] 检查系统环境"
  require_root
  detect_linux_arch >/dev/null
  ensure_binary_disk_space

  step "[2/7] 安装最小依赖"
  install_binary_dependencies

  step "[3/7] 下载 release 二进制"
  install_master_from_release

  step "[4/7] 检测公网地址"
  detect_public_ip

  step "[5/7] 写入配置和 systemd"
  ensure_user
  prepare_existing_service
  write_config

  step "[6/7] 启动 Master 服务"
  start_service

  step "[7/7] 安装完成"
  check_service
}

install_master_source_mode() {
  print_source_banner
  step "[1/8] 检查系统环境"
  require_root
  detect_linux_arch >/dev/null
  ensure_source_disk_space

  step "[2/8] 安装源码构建依赖"
  install_source_dependencies

  step "[3/8] 准备 Go 工具链"
  try_install_distro_go
  install_official_go

  step "[4/8] 下载 quota-dns-router 源码"
  prepare_source

  step "[5/8] 构建 qdr-master"
  build_master_from_source

  step "[6/8] 检测公网地址"
  detect_public_ip

  step "[7/8] 写入配置和 systemd"
  ensure_user
  prepare_existing_service
  write_config

  step "[8/8] 启动并检查 Master 服务"
  start_service
  check_service
}

prompt_source_fallback() {
  if [ "$ALLOW_SOURCE_FALLBACK" = "1" ]; then
    echo "已启用 QDR_ALLOW_SOURCE_FALLBACK=1，继续尝试源码构建。"
    return 0
  fi
  if [ "$YES" -eq 1 ]; then
    echo "二进制安装失败。当前是非交互 --yes 模式，默认不会自动切换到源码构建。" >&2
    echo "如需允许 fallback，请设置 QDR_ALLOW_SOURCE_FALLBACK=1 后重试。" >&2
    return 1
  fi
  if [ "$DRY_RUN" -eq 1 ]; then
    return 1
  fi

  printf "二进制安装失败，是否改用 source 模式继续安装？[y/N]: "
  read -r answer
  case "${answer}" in
    y|Y|yes|YES) return 0 ;;
    *) return 1 ;;
  esac
}

finish_message() {
  echo "Master 已安装并启动。"
  echo "下一步：在 Telegram 向 Bot 发送 /start，然后按按钮依次完成 Master 公网地址、Cloudflare、DNS、节点和 Agent 安装。没有分组时，DNS 向导会自动创建 default 分组。"
  if [ -n "$SUGGESTED_PUBLIC_API_URL" ]; then
    echo "检测到公网地址建议："
    echo "$SUGGESTED_PUBLIC_API_URL"
    echo "请在 Telegram 点击“配置 Master 公网地址”，然后选择“使用当前公网地址”。"
  else
    echo "未能自动检测公网 IP，请在 Telegram 手动配置："
    echo "/config_master_url http://你的服务器公网IP:8080"
  fi
  echo "建议检查：systemctl status quota-dns-router-master --no-pager -l"
  echo "查看日志：journalctl -u quota-dns-router-master -n 100 --no-pager"
  echo "CLI 诊断：qdr-master status"
  echo "配置检查：qdr-master config-check"
  echo "卸载 Master："
  echo "bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/main/scripts/uninstall-master.sh) --yes"
  echo "完全清理 Master："
  echo "bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/main/scripts/uninstall-master.sh) --yes --purge"
}

main() {
  normalize_install_mode
  collect_config

  case "$INSTALL_MODE" in
    binary)
      install_master_binary_mode
      ;;
    source)
      install_master_source_mode
      ;;
    auto)
      print_auto_banner
      if install_master_binary_mode; then
        :
      else
        echo "⚠️ 二进制安装失败。"
        if ! prompt_source_fallback; then
          return 1
        fi
        install_master_source_mode
      fi
      ;;
  esac

  finish_message
}

main "$@"
