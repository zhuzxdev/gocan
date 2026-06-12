#!/usr/bin/env bash
# scripts/setup-vcan.sh — 创建用于本地开发 / 集成测试的虚拟 CAN 接口。
# 用法:
#   sudo ./scripts/setup-vcan.sh [up|down] [iface...]
# 示例:
#   sudo ./scripts/setup-vcan.sh up vcan0 vcan1
#   sudo ./scripts/setup-vcan.sh down vcan0 vcan1
#
# 默认: up vcan0 vcan1
# 需要 root（modprobe + ip link）。
set -euo pipefail

ACTION=${1:-up}
shift || true
IFACES=("$@")
if [ ${#IFACES[@]} -eq 0 ]; then
    IFACES=(vcan0 vcan1)
fi

require_root() {
    if [ "$EUID" -ne 0 ]; then
        echo "需要 root 权限：请使用 sudo 运行" >&2
        exit 1
    fi
}

ensure_module() {
    if ! lsmod | grep -q '^vcan'; then
        echo "正在加载 vcan 内核模块..."
        modprobe vcan
    fi
}

up_iface() {
    local iface=$1
    if ip link show "$iface" >/dev/null 2>&1; then
        echo "[skip] $iface 已存在"
    else
        ip link add "$iface" type vcan
        echo "[add ] $iface"
    fi
    ip link set "$iface" up
    echo "[up  ] $iface"
}

down_iface() {
    local iface=$1
    if ip link show "$iface" >/dev/null 2>&1; then
        ip link set "$iface" down
        ip link delete "$iface"
        echo "[del ] $iface"
    else
        echo "[skip] $iface 不存在"
    fi
}

require_root
case "$ACTION" in
    up)
        ensure_module
        for iface in "${IFACES[@]}"; do up_iface "$iface"; done
        ;;
    down)
        for iface in "${IFACES[@]}"; do down_iface "$iface"; done
        ;;
    *)
        echo "未知动作: $ACTION (期望 up 或 down)" >&2
        exit 1
        ;;
esac
