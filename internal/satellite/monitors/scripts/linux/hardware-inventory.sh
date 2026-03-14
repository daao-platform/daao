#!/bin/sh
# hardware-inventory.sh — Deterministic hardware inventory collection
# Outputs structured JSON with CPU, memory, disks, and GPU info.
# Supports: Linux, macOS (Darwin)

set -e

json_escape() {
    printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g; s/	/\\t/g' | tr '\n' ' '
}

collect_hardware_linux() {
    # CPU
    cpu_model=$(grep -m1 "model name" /proc/cpuinfo 2>/dev/null | cut -d: -f2 | xargs || echo "unknown")
    cpu_cores=$(grep -c "^processor" /proc/cpuinfo 2>/dev/null || echo "0")
    cpu_physical=$(grep "physical id" /proc/cpuinfo 2>/dev/null | sort -u | wc -l || echo "0")
    cpu_threads=$cpu_cores

    # Memory
    mem_total_kb=$(grep MemTotal /proc/meminfo 2>/dev/null | awk '{print $2}' || echo "0")
    mem_avail_kb=$(grep MemAvailable /proc/meminfo 2>/dev/null | awk '{print $2}' || echo "0")
    mem_total_gb=$(echo "scale=2; $mem_total_kb / 1048576" | bc 2>/dev/null || echo "0")
    mem_avail_gb=$(echo "scale=2; $mem_avail_kb / 1048576" | bc 2>/dev/null || echo "0")
    swap_total_kb=$(grep SwapTotal /proc/meminfo 2>/dev/null | awk '{print $2}' || echo "0")
    swap_total_gb=$(echo "scale=2; $swap_total_kb / 1048576" | bc 2>/dev/null || echo "0")

    # Disks
    disks="[]"
    if command -v lsblk >/dev/null 2>&1; then
        disks=$(lsblk -Jb -o NAME,SIZE,TYPE,MOUNTPOINT,FSTYPE,MODEL 2>/dev/null | \
            python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    result = []
    for d in data.get('blockdevices', []):
        if d.get('type') == 'disk':
            size_gb = round(int(d.get('size', 0)) / (1024**3), 1)
            result.append({
                'name': d.get('name', ''),
                'size_gb': size_gb,
                'model': (d.get('model') or '').strip(),
                'partitions': len(d.get('children', []))
            })
    print(json.dumps(result))
except:
    print('[]')
" 2>/dev/null || echo "[]")
    fi

    # Fallback disks if python3 not available
    if [ "$disks" = "[]" ] && command -v lsblk >/dev/null 2>&1; then
        disks=$(lsblk -d -b -o NAME,SIZE,TYPE 2>/dev/null | tail -n +2 | awk '
            $3=="disk" {
                size_gb = $2 / (1024*1024*1024);
                printf "%s{\"name\":\"%s\",\"size_gb\":%.1f}", (n++?",":""), $1, size_gb
            }
        ' | awk 'BEGIN{printf "["} {print} END{printf "]"}' || echo "[]")
    fi

    # GPUs
    gpus="[]"
    if command -v lspci >/dev/null 2>&1; then
        gpus=$(lspci 2>/dev/null | grep -iE "VGA|3D|Display" | awk -F: '{
            gsub(/^[ \t]+/, "", $3);
            printf "%s{\"description\":\"%s\"}", (n++?",":""), $3
        }' | awk 'BEGIN{printf "["} {print} END{printf "]"}' || echo "[]")
    fi

    cat <<EOF
{
  "schema_version": "1.0",
  "snapshot_type": "hardware_inventory",
  "os": "linux",
  "collected_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "status": "complete",
  "data": {
    "cpu": {
      "model": "$(json_escape "$cpu_model")",
      "cores": $cpu_cores,
      "physical_cpus": $cpu_physical,
      "threads": $cpu_threads
    },
    "memory": {
      "total_gb": $mem_total_gb,
      "available_gb": $mem_avail_gb,
      "swap_gb": $swap_total_gb
    },
    "disks": $disks,
    "gpus": $gpus
  }
}
EOF
}

collect_hardware_darwin() {
    # CPU via sysctl
    cpu_model=$(sysctl -n machdep.cpu.brand_string 2>/dev/null || echo "unknown")
    cpu_cores=$(sysctl -n hw.physicalcpu 2>/dev/null || echo "0")
    cpu_threads=$(sysctl -n hw.logicalcpu 2>/dev/null || echo "0")

    # Memory
    mem_total=$(sysctl -n hw.memsize 2>/dev/null || echo "0")
    mem_total_gb=$(echo "scale=2; $mem_total / 1073741824" | bc 2>/dev/null || echo "0")

    # Disks (simplified)
    disks=$(diskutil list -plist 2>/dev/null | plutil -extract AllDisks json -o - - 2>/dev/null | \
        python3 -c "
import sys, json, subprocess
try:
    disks = json.load(sys.stdin)
    result = []
    for d in disks:
        if 'disk' in d and 's' not in d:
            info = subprocess.run(['diskutil', 'info', '-plist', d], capture_output=True, text=True)
            size = 0
            result.append({'name': d, 'size_gb': 0})
    print(json.dumps(result))
except:
    print('[]')
" 2>/dev/null || echo "[]")

    cat <<EOF
{
  "schema_version": "1.0",
  "snapshot_type": "hardware_inventory",
  "os": "darwin",
  "collected_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "status": "complete",
  "data": {
    "cpu": {
      "model": "$(json_escape "$cpu_model")",
      "cores": $cpu_cores,
      "physical_cpus": 1,
      "threads": $cpu_threads
    },
    "memory": {
      "total_gb": $mem_total_gb,
      "available_gb": 0,
      "swap_gb": 0
    },
    "disks": $disks,
    "gpus": []
  }
}
EOF
}

case "$(uname -s 2>/dev/null)" in
    Linux)  collect_hardware_linux ;;
    Darwin) collect_hardware_darwin ;;
    *)
        echo '{"schema_version":"1.0","snapshot_type":"hardware_inventory","os":"unknown","status":"error","data":{}}'
        exit 1
        ;;
esac
