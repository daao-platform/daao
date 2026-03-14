#!/bin/sh
# os-profile.sh — Deterministic OS profile collection
# Outputs structured JSON with OS, kernel, architecture, hostname, and uptime.
# Supports: Linux, macOS (Darwin)
# Usage: sh os-profile.sh

set -e

json_escape() {
    printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g; s/	/\\t/g' | tr '\n' ' '
}

collect_linux() {
    # OS release info
    os_name=""
    os_version=""
    os_id=""
    os_pretty=""
    if [ -f /etc/os-release ]; then
        os_name=$(. /etc/os-release && echo "$NAME")
        os_version=$(. /etc/os-release && echo "$VERSION_ID")
        os_id=$(. /etc/os-release && echo "$ID")
        os_pretty=$(. /etc/os-release && echo "$PRETTY_NAME")
    fi

    kernel=$(uname -r 2>/dev/null || echo "unknown")
    arch=$(uname -m 2>/dev/null || echo "unknown")
    hostname=$(hostname 2>/dev/null || cat /etc/hostname 2>/dev/null || echo "unknown")
    
    # Uptime in seconds
    uptime_seconds=""
    if [ -f /proc/uptime ]; then
        uptime_seconds=$(cut -d' ' -f1 /proc/uptime | cut -d'.' -f1)
    fi

    # Boot time
    boot_time=""
    if command -v who >/dev/null 2>&1; then
        boot_time=$(who -b 2>/dev/null | awk '{print $3, $4}' || echo "")
    fi

    # Timezone
    tz=$(cat /etc/timezone 2>/dev/null || readlink /etc/localtime 2>/dev/null | sed 's|.*/zoneinfo/||' || echo "unknown")

    # Virtualization
    virt="physical"
    if [ -f /sys/class/dmi/id/product_name ]; then
        product=$(cat /sys/class/dmi/id/product_name 2>/dev/null || echo "")
        case "$product" in
            *VirtualBox*) virt="virtualbox" ;;
            *VMware*) virt="vmware" ;;
            *KVM*|*QEMU*) virt="kvm" ;;
            *Hyper-V*) virt="hyperv" ;;
        esac
    fi
    if [ -f /.dockerenv ] || grep -q 'docker\|lxc\|containerd' /proc/1/cgroup 2>/dev/null; then
        virt="container"
    fi

    cat <<EOF
{
  "schema_version": "1.0",
  "snapshot_type": "os_profile",
  "os": "linux",
  "collected_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "status": "complete",
  "data": {
    "os_family": "linux",
    "os_id": "$(json_escape "$os_id")",
    "os_name": "$(json_escape "$os_name")",
    "os_version": "$(json_escape "$os_version")",
    "os_pretty": "$(json_escape "$os_pretty")",
    "kernel": "$(json_escape "$kernel")",
    "arch": "$(json_escape "$arch")",
    "hostname": "$(json_escape "$hostname")",
    "uptime_seconds": ${uptime_seconds:-0},
    "boot_time": "$(json_escape "$boot_time")",
    "timezone": "$(json_escape "$tz")",
    "virtualization": "$(json_escape "$virt")"
  }
}
EOF
}

collect_darwin() {
    os_name="macOS"
    os_version=$(sw_vers -productVersion 2>/dev/null || echo "unknown")
    build=$(sw_vers -buildVersion 2>/dev/null || echo "unknown")
    kernel=$(uname -r 2>/dev/null || echo "unknown")
    arch=$(uname -m 2>/dev/null || echo "unknown")
    hostname=$(hostname 2>/dev/null || echo "unknown")
    uptime_seconds=$(sysctl -n kern.boottime 2>/dev/null | awk '{print $4}' | tr -d ',' | xargs -I{} sh -c 'echo $(( $(date +%s) - {} ))' 2>/dev/null || echo "0")

    cat <<EOF
{
  "schema_version": "1.0",
  "snapshot_type": "os_profile",
  "os": "darwin",
  "collected_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "status": "complete",
  "data": {
    "os_family": "darwin",
    "os_id": "macos",
    "os_name": "$(json_escape "$os_name")",
    "os_version": "$(json_escape "$os_version")",
    "os_pretty": "macOS $(json_escape "$os_version") ($(json_escape "$build"))",
    "kernel": "$(json_escape "$kernel")",
    "arch": "$(json_escape "$arch")",
    "hostname": "$(json_escape "$hostname")",
    "uptime_seconds": ${uptime_seconds:-0},
    "boot_time": "",
    "timezone": "$(date +%Z)",
    "virtualization": "physical"
  }
}
EOF
}

# Main: detect OS and dispatch
case "$(uname -s 2>/dev/null)" in
    Linux)  collect_linux ;;
    Darwin) collect_darwin ;;
    *)
        echo '{"schema_version":"1.0","snapshot_type":"os_profile","os":"unknown","status":"error","data":{}}'
        exit 1
        ;;
esac
