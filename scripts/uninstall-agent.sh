#!/usr/bin/env bash
set -euo pipefail

YES=0
PURGE=0
DRY_RUN=0

while [ $# -gt 0 ]; do
  case "$1" in
    --yes) YES=1 ;;
    --purge) PURGE=1 ;;
    --dry-run) DRY_RUN=1 ;;
    --help)
      echo "用法：uninstall-agent.sh [--yes] [--purge] [--dry-run]"
      exit 0
      ;;
    *) echo "未知参数：$1"; exit 1 ;;
  esac
  shift
done

run() {
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] $*"
  else
    "$@" || true
  fi
}

if [ "$(id -u)" -ne 0 ]; then
  echo "请使用 root 运行卸载脚本。"
  exit 1
fi

if [ "$YES" -ne 1 ]; then
  read -r -p "确认卸载 Agent? [y/N] " ans
  case "$ans" in y|Y|yes|YES) ;; *) echo "已取消"; exit 0 ;; esac
fi

run systemctl stop quota-dns-router-agent.service
run systemctl disable quota-dns-router-agent.service
run rm -f /etc/systemd/system/quota-dns-router-agent.service
run rm -f /usr/local/bin/qdr-agent
run rm -f /etc/quota-dns-router/agent.env
run rm -f /var/lib/quota-dns-router/agent-state.json
run systemctl daemon-reload

if [ "$PURGE" -eq 1 ]; then
  run rm -rf /var/lib/quota-dns-router
  run rm -rf /var/log/quota-dns-router
fi

echo "Agent 卸载完成。"
