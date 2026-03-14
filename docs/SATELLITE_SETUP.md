# Satellite Setup Guide

This guide covers how to install the DAAO satellite daemon on any machine — Linux, macOS, or Windows.

## Quick Install

### Linux / macOS

```bash
curl -fsSL https://your-nexus.example.com/install | bash
```

With a custom Nexus URL:

```bash
curl -fsSL https://your-nexus.example.com/install | NEXUS_URL=https://nexus.internal:8443 bash
```

### Windows (PowerShell)

```powershell
irm https://your-nexus.example.com/install.ps1 | iex
```

With a custom Nexus URL:

```powershell
$env:NEXUS_URL = "https://nexus.internal:8443"; irm https://your-nexus.example.com/install.ps1 | iex
```

---

## What the Installer Does

1. **Detects your OS and architecture** (linux/darwin/windows × amd64/arm64)
2. **Downloads the `daao` binary** from the release server
3. **Installs it** to the appropriate location:
   - Linux (root): `/usr/local/bin/daao`
   - Linux (non-root): `~/.local/bin/daao`
   - macOS: `~/.local/bin/daao`
   - Windows: `%LOCALAPPDATA%\daao\daao.exe`
4. **Runs `daao login`** to generate Ed25519 keys and register with Nexus
5. **Sets up a background service**:
   - Linux: systemd unit (`daao-satellite.service`)
   - macOS: launchd agent (`io.daao.satellite.plist`)
   - Windows: Windows Service or Task Scheduler task

---

## Manual Install

If you prefer to install manually or the release binaries aren't published yet:

### 1. Build the binary

From the DAAO project root:

```bash
# Build for your current OS:
make build-satellite

# Or cross-compile for a specific target:
make build-satellite-linux-amd64
make build-satellite-linux-arm64
make build-satellite-darwin-amd64
make build-satellite-darwin-arm64
make build-satellite-windows-amd64
make build-satellite-windows-arm64

# Or build all at once:
make build-satellite-all
```

Output binaries appear in `bin/`:
```
bin/daao-linux-amd64
bin/daao-linux-arm64
bin/daao-darwin-amd64
bin/daao-darwin-arm64
bin/daao-windows-amd64.exe
bin/daao-windows-arm64.exe
```

### 2. Copy to satellite machine

```bash
scp bin/daao-linux-amd64 satellite-host:/usr/local/bin/daao
ssh satellite-host chmod +x /usr/local/bin/daao
```

### 3. Register with Nexus

```bash
NEXUS_URL=https://your-nexus:8443 daao login
```

This generates an Ed25519 key pair in `~/.config/daao/` and registers the satellite.

### 4. Start the daemon

```bash
daao start
```

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `NEXUS_URL` | `https://nexus.daao.io` | Nexus server URL |
| `DAAO_DMS_TTL` | `60` | Dead Man's Switch TTL in minutes |
| `DAAO_DOWNLOAD_URL` | GitHub releases | Override binary download base URL |

Configuration files are stored in:
- Linux/macOS: `~/.config/daao/`
- Windows: `%USERPROFILE%\.config\daao\`

Key files:
- `satellite.pub` — Ed25519 public key
- `satellite.key` — Ed25519 private key (keep secure!)
- `satellite.crt` — mTLS certificate (after registration)
- `registration.json` — Local registration cache

---

## Service Management

### Linux (systemd)

```bash
# Status
systemctl status daao-satellite          # root install
systemctl --user status daao-satellite   # user install

# Logs
journalctl -u daao-satellite -f          # root
journalctl --user -u daao-satellite -f   # user

# Restart
systemctl restart daao-satellite
```

### macOS (launchd)

```bash
# Status
launchctl list | grep daao

# Logs
tail -f ~/Library/Logs/daao/satellite.log

# Restart
launchctl stop io.daao.satellite
launchctl start io.daao.satellite
```

### Windows

```powershell
# If installed as Windows Service:
sc.exe query DAAOSatellite
sc.exe stop DAAOSatellite
sc.exe start DAAOSatellite

# If installed as Task Scheduler task:
Get-ScheduledTask -TaskName "DAAO Satellite Daemon"
Start-ScheduledTask -TaskName "DAAO Satellite Daemon"
Stop-ScheduledTask -TaskName "DAAO Satellite Daemon"
```

---

## Troubleshooting

### Satellite not showing in Cockpit

1. Check the daemon is running (see Service Management above)
2. Verify Nexus URL: `echo $NEXUS_URL` / `$env:NEXUS_URL`
3. Check network connectivity: `curl -k https://your-nexus:8443/health`
4. Re-register: `daao login`

### Keys not found

```bash
ls -la ~/.config/daao/
# Should contain satellite.pub and satellite.key
```

If missing, re-run `daao login` to generate new keys.

### Permission denied (Linux)

If installed as non-root, ensure `~/.local/bin` is in your PATH:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Add this to your `~/.bashrc` or `~/.zshrc`.

---

## Unraid

Unraid uses Slackware (no systemd) and boots from USB (RAM-based root filesystem), so the standard Linux installer won't work. Use the dedicated Unraid installer instead.

### Quick Install

SSH into your Unraid box and run:

```bash
curl -fsSL https://your-nexus.example.com/install-unraid | NEXUS_URL=https://your-nexus:8443 bash
```

The script will pull the correct binary from Nexus automatically.

### Manual Install (if releases aren't published yet)

1. Build the satellite binary on your dev machine:
   ```bash
   make build-satellite-linux-amd64
   ```

2. Copy the binary and installer to your Unraid box:
   ```bash
   scp bin/daao-linux-amd64 scripts/install-satellite-unraid.sh root@<unraid-ip>:/tmp/
   ```

3. SSH in and run the installer:
   ```bash
   ssh root@<unraid-ip>
   cd /tmp
   DAAO_BINARY_PATH=./daao-linux-amd64 NEXUS_URL=https://your-nexus:8443 bash install-satellite-unraid.sh
   ```

The installer will:
- Verify it's running on Unraid
- Store the binary and config on the flash drive (`/boot/config/plugins/daao/`) for persistence across reboots
- Detect the **User Scripts** plugin or `go` boot script for auto-start
- Register the satellite with Nexus
- Start the daemon immediately

### Persistence

Unraid's root filesystem is in RAM — files in `/usr/local/bin` are lost on reboot. The Unraid installer stores everything on the USB flash drive and restores symlinks on boot.

| What | Location | Persists? |
|------|----------|-----------|
| Binary | `/boot/config/plugins/daao/bin/daao` | ✓ Flash |
| Config/keys | `/boot/config/plugins/daao/config/` | ✓ Flash |
| Logs | `/var/log/daao/` | ✗ RAM (reset on reboot) |

### Service Management

```bash
# Check if running
pgrep -fa 'daao start'

# View logs
tail -f /var/log/daao/satellite.log

# Stop
pkill -f 'daao start'

# Start manually
nohup daao start >> /var/log/daao/satellite.log 2>&1 &
```

### Uninstall

```bash
bash install-satellite-unraid.sh --uninstall
```

---

## Uninstall

### Linux / macOS

```bash
bash install-satellite.sh --uninstall
```

Or with curl:
```bash
curl -fsSL https://your-nexus/install | DAAO_UNINSTALL=1 bash
```

### Windows

```powershell
.\install-satellite.ps1 -Uninstall
```

### Manual cleanup

Remove the configuration directory to delete satellite keys:

```bash
rm -rf ~/.config/daao    # Linux/macOS
```

```powershell
Remove-Item -Recurse -Force "$env:USERPROFILE\.config\daao"  # Windows
```
