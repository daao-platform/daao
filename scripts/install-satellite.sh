#!/usr/bin/env bash
# ============================================================================
# DAAO Satellite Daemon Installer — Linux & macOS
# ============================================================================
# Usage:
#   curl -fsSL https://your-nexus/install | bash
#   curl -fsSL https://your-nexus/install | NEXUS_URL=https://nexus.example.com bash
#
# Options (as environment variables):
#   NEXUS_URL        — Cockpit URL (required; the one-liner sets this automatically)
#   DAAO_VERSION     — Version to install (default: latest)
#   DAAO_INSTALL_DIR — Override install directory
#   DAAO_NO_SERVICE  — Set to 1 to skip service installation
#   DAAO_UNINSTALL   — Set to 1 to uninstall
#
# Or run locally:
#   bash install-satellite.sh
#   bash install-satellite.sh --uninstall
#   bash install-satellite.sh --help
# ============================================================================
set -euo pipefail

# ─── Configuration ──────────────────────────────────────────────────────────

NEXUS_URL="${NEXUS_URL:-}"  # Set via NEXUS_URL env var or the one-liner passes it automatically
NEXUS_GRPC_ADDR="${NEXUS_GRPC_ADDR:-}"  # gRPC address (derived from NEXUS_URL if not set)
DAAO_INSTALL_DIR="${DAAO_INSTALL_DIR:-}"
DAAO_NO_SERVICE="${DAAO_NO_SERVICE:-0}"
DAAO_UNINSTALL="${DAAO_UNINSTALL:-0}"
BINARY_NAME="daao"
SERVICE_NAME="daao-satellite"

# ─── Colors ─────────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# ─── Helpers ────────────────────────────────────────────────────────────────

info()    { echo -e "${BLUE}ℹ${NC} $*"; }
success() { echo -e "${GREEN}✓${NC} $*"; }
warn()    { echo -e "${YELLOW}⚠${NC} $*"; }
error()   { echo -e "${RED}✗${NC} $*" >&2; }
fatal()   { error "$@"; exit 1; }

header() {
    echo ""
    echo -e "${CYAN}${BOLD}$*${NC}"
    echo -e "${CYAN}$(printf '─%.0s' $(seq 1 ${#1}))${NC}"
}

# ─── Derive gRPC address from NEXUS_URL if not explicitly set ───────────────

derive_grpc_addr() {
    local url="$1"
    local host
    host="${url#https://}"
    host="${host#http://}"
    host="${host%%/*}"   # strip path
    host="${host%%:*}"   # strip port
    echo "${host}:8444"
}

if [[ -z "$NEXUS_GRPC_ADDR" && -n "$NEXUS_URL" ]]; then
    NEXUS_GRPC_ADDR="$(derive_grpc_addr "$NEXUS_URL")"
fi

# ─── Argument Parsing ──────────────────────────────────────────────────────

for arg in "$@"; do
    case "$arg" in
        --uninstall) DAAO_UNINSTALL=1 ;;
        --no-service) DAAO_NO_SERVICE=1 ;;
        --help|-h)
            echo "DAAO Satellite Installer"
            echo ""
            echo "Usage:"
            echo "  curl -fsSL https://your-nexus/install | bash"
            echo "  bash install-satellite.sh [--uninstall] [--no-service] [--help]"
            echo ""
            echo "Options:"
            echo "  --uninstall     Remove DAAO satellite daemon"
            echo "  --no-service    Skip systemd/launchd service setup"
            echo "  --help          Show this help"
            echo ""
            echo "Environment Variables:"
            echo "  NEXUS_URL         Cockpit URL (required; the one-liner sets this automatically)"
            echo "  NEXUS_GRPC_ADDR   Override gRPC address (default: <host>:8444 derived from NEXUS_URL)"
            echo "  DAAO_VERSION      Version to install (default: latest)"
            echo "  DAAO_INSTALL_DIR  Override install directory"
            echo "  DAAO_NO_SERVICE   Set to 1 to skip service setup"
            exit 0
            ;;
    esac
done

# ─── Platform Detection ────────────────────────────────────────────────────

detect_platform() {
    local os arch

    case "$(uname -s)" in
        Linux*)  os="linux" ;;
        Darwin*) os="darwin" ;;
        *)       fatal "Unsupported operating system: $(uname -s). Use install-satellite.ps1 for Windows." ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64)  arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)             fatal "Unsupported architecture: $(uname -m)" ;;
    esac

    OS="$os"
    ARCH="$arch"
}

# ─── Install Directory ─────────────────────────────────────────────────────

determine_install_dir() {
    if [[ -n "$DAAO_INSTALL_DIR" ]]; then
        INSTALL_DIR="$DAAO_INSTALL_DIR"
    elif [[ $EUID -eq 0 ]]; then
        INSTALL_DIR="/usr/local/bin"
    else
        INSTALL_DIR="$HOME/.local/bin"
    fi

    mkdir -p "$INSTALL_DIR"
}

# ─── Prerequisite Check ────────────────────────────────────────────────────

check_prerequisites() {
    local missing=()
    for cmd in curl; do
        if ! command -v "$cmd" &>/dev/null; then
            missing+=("$cmd")
        fi
    done

    if [[ ${#missing[@]} -gt 0 ]]; then
        fatal "Missing required commands: ${missing[*]}"
    fi
}

# ─── Download ──────────────────────────────────────────────────────────────

download_binary() {
    local binary_file="${BINARY_NAME}-${OS}-${ARCH}"
    local download_url="${NEXUS_URL}/releases/${binary_file}"
    local dest="${INSTALL_DIR}/${BINARY_NAME}"

    info "Downloading ${BINARY_NAME} for ${OS}/${ARCH}..."
    info "URL: ${download_url}"

    if curl -fsSL -o "$dest" "$download_url" 2>/dev/null; then
        # Verify we got a real binary, not an HTML error page
        if file "$dest" | grep -qi "html\|text"; then
            rm -f "$dest"
            error "Received HTML instead of binary — release may not be published"
            return 1
        fi
        chmod +x "$dest"
        # macOS: remove quarantine attribute so Gatekeeper doesn't kill unsigned binaries
        if [[ "$(uname -s)" == "Darwin" ]]; then
            xattr -d com.apple.quarantine "$dest" 2>/dev/null || true
        fi
        local size
        size=$(du -h "$dest" | cut -f1)
        success "Downloaded to ${dest} (${size})"
    else
        error "Download failed from ${download_url}"
        warn "You can build the satellite binary manually:"
        echo ""
        echo "    make build-satellite-${OS}-${ARCH}"
        echo ""
        echo "  Then copy bin/${binary_file} to ${dest} on your satellite machine."
        return 1
    fi
}

# ─── Nexus Registration ────────────────────────────────────────────────────

register_satellite() {
    local daao_bin="${INSTALL_DIR}/${BINARY_NAME}"

    if [[ ! -x "$daao_bin" ]]; then
        warn "Binary not found at ${daao_bin} — skipping registration"
        return 0
    fi

    info "Generating keys and registering satellite with Nexus at ${NEXUS_URL}..."

    if NEXUS_URL="$NEXUS_URL" NEXUS_GRPC_ADDR="$NEXUS_GRPC_ADDR" "$daao_bin" login 2>&1; then
        success "Satellite registered (keys generated, record created in Nexus)"
    else
        warn "Registration failed — keys were saved. Run: NEXUS_URL=${NEXUS_URL} daao login"
    fi
}

# ─── systemd Service (Linux) ───────────────────────────────────────────────

install_systemd_service() {
    local service_file="/etc/systemd/system/${SERVICE_NAME}.service"
    local daao_bin="${INSTALL_DIR}/${BINARY_NAME}"
    local run_user

    if [[ $EUID -eq 0 ]]; then
        run_user="${SUDO_USER:-root}"
    else
        run_user="$(whoami)"
        # User-level systemd
        service_file="$HOME/.config/systemd/user/${SERVICE_NAME}.service"
        mkdir -p "$(dirname "$service_file")"
    fi

    info "Installing systemd service..."

    cat > "$service_file" << EOF
[Unit]
Description=DAAO Satellite Daemon
Documentation=https://github.com/daao/daao
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${daao_bin} start
Restart=always
RestartSec=10
Environment=NEXUS_URL=${NEXUS_URL}
Environment=NEXUS_GRPC_ADDR=${NEXUS_GRPC_ADDR}

# Security hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/home/${run_user}/.config/daao

[Install]
WantedBy=multi-user.target
EOF

    if [[ $EUID -eq 0 ]]; then
        systemctl daemon-reload
        systemctl enable "${SERVICE_NAME}" 2>/dev/null || true
        systemctl start "${SERVICE_NAME}" 2>/dev/null || true
        success "systemd service installed and started"
    else
        systemctl --user daemon-reload
        systemctl --user enable "${SERVICE_NAME}" 2>/dev/null || true
        systemctl --user start "${SERVICE_NAME}" 2>/dev/null || true
        success "systemd user service installed and started"
    fi

    echo ""
    info "Manage the service with:"
    if [[ $EUID -eq 0 ]]; then
        echo "    systemctl status ${SERVICE_NAME}"
        echo "    journalctl -u ${SERVICE_NAME} -f"
    else
        echo "    systemctl --user status ${SERVICE_NAME}"
        echo "    journalctl --user -u ${SERVICE_NAME} -f"
    fi
}

# ─── launchd Service (macOS) ───────────────────────────────────────────────

install_launchd_service() {
    local plist_dir="$HOME/Library/LaunchAgents"
    local plist_file="${plist_dir}/io.daao.satellite.plist"
    local daao_bin="${INSTALL_DIR}/${BINARY_NAME}"
    local log_dir="$HOME/Library/Logs/daao"

    mkdir -p "$plist_dir" "$log_dir"

    info "Installing launchd agent..."

    cat > "$plist_file" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>io.daao.satellite</string>
    <key>ProgramArguments</key>
    <array>
        <string>${daao_bin}</string>
        <string>start</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>NEXUS_URL</key>
        <string>${NEXUS_URL}</string>
        <key>NEXUS_GRPC_ADDR</key>
        <string>${NEXUS_GRPC_ADDR}</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>${log_dir}/satellite.log</string>
    <key>StandardErrorPath</key>
    <string>${log_dir}/satellite.err.log</string>
    <key>ThrottleInterval</key>
    <integer>10</integer>
</dict>
</plist>
EOF

    launchctl unload "$plist_file" 2>/dev/null || true
    launchctl load "$plist_file" 2>/dev/null || true

    success "launchd agent installed and loaded"
    echo ""
    info "Manage the service with:"
    echo "    launchctl list | grep daao"
    echo "    tail -f ${log_dir}/satellite.log"
}

# ─── Service Install Router ────────────────────────────────────────────────

install_service() {
    if [[ "$DAAO_NO_SERVICE" == "1" ]]; then
        info "Skipping service installation (--no-service)"
        return 0
    fi

    case "$OS" in
        linux)  install_systemd_service ;;
        darwin) install_launchd_service ;;
    esac
}

# ─── Uninstall ──────────────────────────────────────────────────────────────

uninstall() {
    header "Uninstalling DAAO Satellite"

    # Stop and remove services
    if [[ "$(uname -s)" == "Linux" ]]; then
        if [[ $EUID -eq 0 ]]; then
            systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
            systemctl disable "${SERVICE_NAME}" 2>/dev/null || true
            rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
            systemctl daemon-reload
        else
            systemctl --user stop "${SERVICE_NAME}" 2>/dev/null || true
            systemctl --user disable "${SERVICE_NAME}" 2>/dev/null || true
            rm -f "$HOME/.config/systemd/user/${SERVICE_NAME}.service"
            systemctl --user daemon-reload
        fi
        success "systemd service removed"
    elif [[ "$(uname -s)" == "Darwin" ]]; then
        local plist_file="$HOME/Library/LaunchAgents/io.daao.satellite.plist"
        launchctl unload "$plist_file" 2>/dev/null || true
        rm -f "$plist_file"
        success "launchd agent removed"
    fi

    # Remove binary
    determine_install_dir
    local bin_path="${INSTALL_DIR}/${BINARY_NAME}"
    if [[ -f "$bin_path" ]]; then
        rm -f "$bin_path"
        success "Binary removed: ${bin_path}"
    fi

    # Offer to remove config
    local config_dir="$HOME/.config/daao"
    if [[ -d "$config_dir" ]]; then
        warn "Configuration directory still exists: ${config_dir}"
        echo "  Remove it manually to delete satellite keys:"
        echo "    rm -rf ${config_dir}"
    fi

    echo ""
    success "DAAO satellite uninstalled"
    exit 0
}

# ─── Main ───────────────────────────────────────────────────────────────────

main() {
    echo ""
    echo -e "${BOLD}${CYAN}    ╔═══════════════════════════════════════╗${NC}"
    echo -e "${BOLD}${CYAN}    ║   DAAO Satellite Daemon Installer     ║${NC}"
    echo -e "${BOLD}${CYAN}    ╚═══════════════════════════════════════╝${NC}"
    echo ""

    # Handle uninstall
    if [[ "$DAAO_UNINSTALL" == "1" ]]; then
        uninstall
    fi

    # Require NEXUS_URL
    if [[ -z "$NEXUS_URL" ]]; then
        fatal "NEXUS_URL is not set. Run with: curl -fsSL <cockpit-url>/install | NEXUS_URL=<cockpit-url> bash"
    fi

    # Step 1: Detect platform
    header "1. Detecting Platform"
    detect_platform
    success "Platform: ${OS}/${ARCH}"

    # Step 2: Check prerequisites
    header "2. Checking Prerequisites"
    check_prerequisites
    success "All prerequisites met"

    # Step 3: Determine install directory
    header "3. Preparing Install Directory"
    determine_install_dir
    success "Install directory: ${INSTALL_DIR}"

    # Check if already installed
    if [[ -x "${INSTALL_DIR}/${BINARY_NAME}" ]]; then
        local existing_version
        existing_version=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null || echo "unknown")
        warn "DAAO is already installed (version: ${existing_version})"
        info "Re-installing..."
    fi

    # Step 4: Download binary
    header "4. Downloading DAAO Satellite Binary"
    if ! download_binary; then
        warn "Continuing without binary — complete manual install steps below"
    fi

    # Step 5: Download Nexus CA certificate for TLS verification
    header "5. Downloading Nexus CA Certificate"
    ca_url="${NEXUS_URL}/releases/ca.crt"
    ca_dest="$HOME/.config/daao/nexus-ca.crt"
    mkdir -p "$(dirname "$ca_dest")"
    if curl -fsSL -o "$ca_dest" "$ca_url" 2>/dev/null; then
        success "CA certificate saved to $ca_dest"
    else
        warn "Could not download CA cert from ${ca_url}"
        warn "'daao login' will use TOFU (Trust On First Use) instead"
    fi

    # Step 6: Register with Nexus
    header "6. Registering Satellite"
    register_satellite

    # Step 7: Install service
    header "7. Setting Up Background Service"
    install_service

    # Step 7: Summary
    echo ""
    echo -e "${BOLD}${GREEN}    ╔═══════════════════════════════════════╗${NC}"
    echo -e "${BOLD}${GREEN}    ║   Installation Complete!              ║${NC}"
    echo -e "${BOLD}${GREEN}    ╚═══════════════════════════════════════╝${NC}"
    echo ""
    echo "  Binary:     ${INSTALL_DIR}/${BINARY_NAME}"
    echo "  Config:     ~/.config/daao/"
    echo "  Nexus URL:  ${NEXUS_URL}"
    echo ""
    echo -e "${BOLD}Next Steps:${NC}"
    echo "  1. Verify the daemon is running:"
    if [[ "$OS" == "linux" ]]; then
        if [[ $EUID -eq 0 ]]; then
            echo "       systemctl status ${SERVICE_NAME}"
        else
            echo "       systemctl --user status ${SERVICE_NAME}"
        fi
    elif [[ "$OS" == "darwin" ]]; then
        echo "       launchctl list | grep daao"
    fi
    echo "  2. Open Cockpit to see this satellite:"
    echo "       ${NEXUS_URL}"
    echo "  3. Start an AI agent session:"
    echo "       daao run claude-code"
    echo ""
}

main "$@"
