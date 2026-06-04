#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-}"
PREFIX="${PREFIX:-/usr/local/bin}"
ETC_DIR="${ETC_DIR:-/etc/quota-dns-router}"
DATA_DIR="${DATA_DIR:-/var/lib/quota-dns-router}"

fail=0

usage() {
  echo "用法：smoke.sh master|agent"
}

check() {
  local title="$1"
  shift
  if "$@"; then
    echo "✅ ${title}"
  else
    echo "❌ ${title}"
    fail=1
  fi
}

check_file_exists() {
  [ -f "$1" ]
}

check_dir_exists() {
  [ -d "$1" ]
}

check_env_permissions() {
  local path="$1"
  [ -f "$path" ] || return 1
  local mode
  mode="$(stat -c '%a' "$path" 2>/dev/null || true)"
  case "$mode" in
    600|640) return 0 ;;
    *) echo "当前权限：${path} ${mode:-unknown}" >&2; return 1 ;;
  esac
}

check_data_permissions() {
  local path="$1"
  [ -d "$path" ] || return 1
  local mode
  mode="$(stat -c '%a' "$path" 2>/dev/null || true)"
  case "$mode" in
    700|750|755) return 0 ;;
    *) echo "当前权限：${path} ${mode:-unknown}" >&2; return 1 ;;
  esac
}

check_systemd() {
  local unit="$1"
  systemctl is-active --quiet "$unit"
}

smoke_master() {
  local bin="${PREFIX}/qdr-master"
  echo "Master 验收检查"
  check "Master 二进制存在" check_file_exists "$bin"
  check "Master 版本输出" "$bin" version
  check "Master systemd 正在运行" check_systemd quota-dns-router-master.service
  check "master.env 存在且权限安全" check_env_permissions "${ETC_DIR}/master.env"
  check "数据目录存在且权限合理" check_data_permissions "$DATA_DIR"
  check "qdr-master status" "$bin" status
  check "qdr-master config-check" "$bin" config-check
  check "qdr-master telegram-status" "$bin" telegram-status
}

smoke_agent() {
  local bin="${PREFIX}/qdr-agent"
  echo "Agent 验收检查"
  check "Agent 二进制存在" check_file_exists "$bin"
  check "Agent 版本输出" "$bin" version
  check "Agent systemd 正在运行" check_systemd quota-dns-router-agent.service
  check "agent.env 存在且权限安全" check_env_permissions "${ETC_DIR}/agent.env"
  check "数据目录存在且权限合理" check_data_permissions "$DATA_DIR"
  check "qdr-agent status" "$bin" status
  check "qdr-agent config-check" "$bin" config-check
}

case "$TARGET" in
  master) smoke_master ;;
  agent) smoke_agent ;;
  *) usage; exit 2 ;;
esac

if [ "$fail" -ne 0 ]; then
  echo "验收失败，请根据上面的 ❌ 项排查。"
  exit 1
fi

echo "验收通过。"
