#!/bin/bash
set -e

# satellite-entrypoint.sh — Generates keys, registers with Nexus, and starts the daemon
#
# Environment Variables:
#   NEXUS_URL          - Cockpit URL for registration (e.g., http://cockpit)
#   NEXUS_GRPC_ADDR    - gRPC address to connect to (e.g., nexus:8444 or nexus-2:8444)
#   SATELLITE_NAME     - Human-readable name for this satellite

SATELLITE_NAME="${SATELLITE_NAME:-satellite-$(hostname)}"
CONFIG_DIR="/root/.config/daao"
mkdir -p "$CONFIG_DIR"

echo "=== DAAO Satellite Container ==="
echo "  Name:      $SATELLITE_NAME"
echo "  gRPC:      ${NEXUS_GRPC_ADDR:-nexus:8444}"
echo "  Nexus URL: ${NEXUS_URL:-http://cockpit}"

# Check if already registered
if [ -f "$CONFIG_DIR/registration.json" ]; then
    echo "  Already registered: $(cat $CONFIG_DIR/registration.json | head -1)"
else
    echo "  Registering with Nexus..."
    # Generate keys (the login command does this)
    NEXUS_URL="${NEXUS_URL}" daao login 2>&1 || true
    
    if [ -f "$CONFIG_DIR/registration.json" ]; then
        echo "  Registered successfully"
    else
        echo "  Registration failed — will retry on connect"
    fi
fi

echo "  Starting satellite daemon..."
exec daao start
