#!/bin/sh
# service-catalog.sh — Deterministic service and container discovery
# Outputs structured JSON with running services, containers, and listening ports.
# Supports: Linux, macOS (Darwin)

set -e

json_escape() {
    printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g; s/	/\\t/g' | tr '\n' ' '
}

collect_services_linux() {
    services="[]"
    if command -v systemctl >/dev/null 2>&1; then
        services=$(systemctl list-units --type=service --state=running --no-pager --plain 2>/dev/null | \
            grep '\.service' | awk '{print $1}' | sed 's/\.service$//' | \
            awk 'BEGIN{printf "["} NR>1{printf ","} {printf "\"%s\"", $0} END{printf "]"}')
    fi

    # Containers
    containers="[]"
    if command -v docker >/dev/null 2>&1; then
        containers=$(docker ps --format '{"name":"{{.Names}}","image":"{{.Image}}","status":"{{.Status}}","ports":"{{.Ports}}"}' 2>/dev/null | \
            awk 'BEGIN{printf "["} NR>1{printf ","} {print} END{printf "]"}' || echo "[]")
    fi

    # Listening ports
    ports="[]"
    if command -v ss >/dev/null 2>&1; then
        ports=$(ss -tlnp 2>/dev/null | tail -n +2 | awk '{
            split($4, addr, ":");
            port = addr[length(addr)];
            proc = $6;
            gsub(/.*users:\(\("/, "", proc);
            gsub(/".*/, "", proc);
            if (port != "" && port+0 == port) printf "%s{\"port\":%s,\"process\":\"%s\"}", (NR>2?",":""), port, proc
        }' | awk 'BEGIN{printf "["} {print} END{printf "]"}' || echo "[]")
    elif command -v netstat >/dev/null 2>&1; then
        ports=$(netstat -tlnp 2>/dev/null | tail -n +3 | awk '{
            split($4, addr, ":");
            port = addr[length(addr)];
            proc = $7;
            if (port != "" && port+0 == port) printf "%s{\"port\":%s,\"process\":\"%s\"}", (NR>3?",":""), port, proc
        }' | awk 'BEGIN{printf "["} {print} END{printf "]"}' || echo "[]")
    fi

    # Docker installed?
    docker_installed="false"
    if command -v docker >/dev/null 2>&1; then
        docker_installed="true"
    fi

    # Podman?
    podman_installed="false"
    if command -v podman >/dev/null 2>&1; then
        podman_installed="true"
    fi

    cat <<EOF
{
  "schema_version": "1.0",
  "snapshot_type": "service_catalog",
  "os": "linux",
  "collected_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "status": "complete",
  "data": {
    "services": $services,
    "containers": $containers,
    "listening_ports": $ports,
    "container_runtimes": {
      "docker": $docker_installed,
      "podman": $podman_installed
    }
  }
}
EOF
}

collect_services_darwin() {
    # launchd services
    services=$(launchctl list 2>/dev/null | tail -n +2 | awk '{
        if ($1 != "-") printf "%s\"%s\"", (NR>2?",":""), $3
    }' | awk 'BEGIN{printf "["} {print} END{printf "]"}' || echo "[]")

    # Docker Desktop
    containers="[]"
    if command -v docker >/dev/null 2>&1; then
        containers=$(docker ps --format '{"name":"{{.Names}}","image":"{{.Image}}","status":"{{.Status}}"}' 2>/dev/null | \
            awk 'BEGIN{printf "["} NR>1{printf ","} {print} END{printf "]"}' || echo "[]")
    fi

    # Listening ports
    ports=$(netstat -an 2>/dev/null | grep LISTEN | awk '{
        split($4, addr, ".");
        port = addr[length(addr)];
        if (port != "" && port+0 == port) printf "%s{\"port\":%s,\"process\":\"\"}", (NR>1?",":""), port
    }' | awk 'BEGIN{printf "["} {print} END{printf "]"}' || echo "[]")

    cat <<EOF
{
  "schema_version": "1.0",
  "snapshot_type": "service_catalog",
  "os": "darwin",
  "collected_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "status": "complete",
  "data": {
    "services": $services,
    "containers": $containers,
    "listening_ports": $ports,
    "container_runtimes": {
      "docker": $(command -v docker >/dev/null 2>&1 && echo "true" || echo "false"),
      "podman": false
    }
  }
}
EOF
}

case "$(uname -s 2>/dev/null)" in
    Linux)  collect_services_linux ;;
    Darwin) collect_services_darwin ;;
    *)
        echo '{"schema_version":"1.0","snapshot_type":"service_catalog","os":"unknown","status":"error","data":{}}'
        exit 1
        ;;
esac
