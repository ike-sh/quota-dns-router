#!/usr/bin/env bash
set -euo pipefail

YES=0
PURGE=0
KEEP_DATA=0
DRY_RUN=0

while [ $# -gt 0 ]; do
  case "$1" in
    --yes) YES=1 ;;
    --purge) PURGE=1 ;;
    --keep-data) KEEP_DATA=1 ;;
    --dry-run) DRY_RUN=1 ;;
    --help)
      echo "用法：uninstall-master.sh [--yes] [--purge] [--keep-data] [--dry-run]"
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
  read -r -p "确认卸载 Master? [y/N] " ans
  case "$ans" in y|Y|yes|YES) ;; *) echo "已取消"; exit 0 ;; esac
fi

run systemctl stop quota-dns-router-master.service
run systemctl disable quota-dns-router-master.service
run rm -f /etc/systemd/system/quota-dns-router-master.service
run rm -f /usr/local/bin/qdr-master
run systemctl daemon-reload

if [ "$PURGE" -eq 1 ]; then
  run rm -rf /etc/quota-dns-router
  if [ "$KEEP_DATA" -ne 1 ]; then
    run rm -rf /var/lib/quota-dns-router
  fi
  run rm -rf /var/log/quota-dns-router
  if id quota-dns-router >/dev/null 2>&1; then
    run userdel quota-dns-router
  fi
fi

echo "Master 卸载完成。默认保留 /var/lib/quota-dns-router；使用 --purge 删除更多数据。"
