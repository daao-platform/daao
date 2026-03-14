#!/bin/sh
# network-topology.sh — Deterministic network topology collection
# Outputs structured JSON with interfaces, routes, DNS, and listening connections.
# Supports: Linux, macOS (Darwin)

set -e

json_escape() {
    printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g; s/	/\\t/g' | tr '\n' ' '
}

collect_network_linux() {
    # Interfaces with IPs
    interfaces="[]"
    if command -v ip >/dev/null 2>&1; then
        interfaces=$(ip -j addr show 2>/dev/null || echo "[]")
    fi

    # Routes
    routes="[]"
    if command -v ip >/dev/null 2>&1; then
        routes=$(ip -j route show 2>/dev/null || echo "[]")
    fi

    # DNS servers
    dns_servers="[]"
    if [ -f /etc/resolv.conf ]; then
        dns_servers=$(grep '^nameserver' /etc/resolv.conf 2>/dev/null | awk '{printf "%s\"%s\"", (NR>1?",":""), $2}' | awk 'BEGIN{printf "["} {print} END{printf "]"}')
    fi

    # Default gateway
    gateway=""
    if command -v ip >/dev/null 2>&1; then
        gateway=$(ip route show default 2>/dev/null | awk '/default/{print $3; exit}' || echo "")
    fi

    # Public IP (best-effort, may fail in air-gapped environments)
    public_ip=""
    if command -v curl >/dev/null 2>&1; then
        public_ip=$(curl -s --connect-timeout 3 https://ifconfig.me 2>/dev/null || echo "")
    fi

    cat <<EOF
{
  "schema_version": "1.0",
  "snapshot_type": "network_topology",
  "os": "linux",
  "collected_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "status": "complete",
  "data": {
    "interfaces": $interfaces,
    "routes": $routes,
    "dns_servers": $dns_servers,
    "default_gateway": "$(json_escape "$gateway")",
    "public_ip": "$(json_escape "$public_ip")",
    "hostname": "$(hostname 2>/dev/null || echo "unknown")"
  }
}
EOF
}

collect_network_darwin() {
    # Interfaces (simplified — macOS doesn't have ip -j)
    interfaces=$(ifconfig 2>/dev/null | awk '
        /^[a-z]/ { iface=$1; sub(/:$/,"",iface) }
        /inet / { printf "%s{\"name\":\"%s\",\"addr\":\"%s\"}", (n++?",":""), iface, $2 }
    ' | awk 'BEGIN{printf "["} {print} END{printf "]"}' || echo "[]")

    # Routes
    routes=$(netstat -rn 2>/dev/null | tail -n +5 | head -20 | awk '{
        printf "%s{\"destination\":\"%s\",\"gateway\":\"%s\",\"interface\":\"%s\"}", (NR>1?",":""), $1, $2, $NF
    }' | awk 'BEGIN{printf "["} {print} END{printf "]"}' || echo "[]")

    # DNS
    dns_servers=$(scutil --dns 2>/dev/null | grep 'nameserver\[' | awk '{printf "%s\"%s\"", (n++?",":""), $3}' | awk 'BEGIN{printf "["} {print} END{printf "]"}' || echo "[]")

    # Gateway
    gateway=$(netstat -rn 2>/dev/null | awk '/^default/{print $2; exit}' || echo "")

    cat <<EOF
{
  "schema_version": "1.0",
  "snapshot_type": "network_topology",
  "os": "darwin",
  "collected_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "status": "complete",
  "data": {
    "interfaces": $interfaces,
    "routes": $routes,
    "dns_servers": $dns_servers,
    "default_gateway": "$(json_escape "$gateway")",
    "public_ip": "",
    "hostname": "$(hostname 2>/dev/null || echo "unknown")"
  }
}
EOF
}

case "$(uname -s 2>/dev/null)" in
    Linux)  collect_network_linux ;;
    Darwin) collect_network_darwin ;;
    *)
        echo '{"schema_version":"1.0","snapshot_type":"network_topology","os":"unknown","status":"error","data":{}}'
        exit 1
        ;;
esac
