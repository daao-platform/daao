#Requires -Version 5.1
<#
.SYNOPSIS
    DAAO Satellite Daemon Installer for Windows.

.DESCRIPTION
    Downloads and installs the DAAO satellite daemon, registers with Nexus,
    and optionally sets up a Windows Service or startup task.

    One-liner install:
        irm https://your-nexus/install.ps1 | iex

    Or run locally:
        .\install-satellite.ps1
        .\install-satellite.ps1 -Uninstall
        .\install-satellite.ps1 -NoService

.PARAMETER NexusUrl
    URL of the Cockpit server (required). Set via $env:NEXUS_URL or -NexusUrl.
    The binary is downloaded from $NexusUrl/releases/

.PARAMETER InstallDir
    Override install directory. Default: $env:LOCALAPPDATA\daao

.PARAMETER NoService
    Skip background service/task setup.

.PARAMETER Uninstall
    Remove DAAO satellite daemon.

.PARAMETER Help
    Show usage information.

.EXAMPLE
    .\install-satellite.ps1

.EXAMPLE
    .\install-satellite.ps1 -NexusUrl https://my-nexus.internal:8443

.EXAMPLE
    .\install-satellite.ps1 -Uninstall
#>

[CmdletBinding()]
param(
    [string]$NexusUrl = $env:NEXUS_URL,
    [string]$NexusGrpcAddr = $env:NEXUS_GRPC_ADDR,
    [string]$InstallDir = "",
    [switch]$NoService,
    [switch]$Uninstall,
    [switch]$Help
)

# ─── Defaults ───────────────────────────────────────────────────────────────

if (-not $NexusUrl) {
    Write-Err "NEXUS_URL is not set."
    Write-Host "  Run: `$env:NEXUS_URL = 'http://<your-host>:8081'; irm <cockpit-url>/install.ps1 | iex"
    exit 1
}

# Derive gRPC address from NexusUrl if not explicitly provided
if (-not $NexusGrpcAddr) {
    $uri = [System.Uri]$NexusUrl
    $NexusGrpcAddr = "$($uri.Host):8444"
}

$BinaryName = "daao.exe"
$ServiceName = "DAAOSatellite"
$TaskName = "DAAO Satellite Daemon"
$ConfigDir = Join-Path $env:USERPROFILE ".config\daao"

# ─── Colors ─────────────────────────────────────────────────────────────────

function Write-Info { param([string]$Msg) Write-Host "  i " -ForegroundColor Blue -NoNewline; Write-Host $Msg }
function Write-Success { param([string]$Msg) Write-Host "  √ " -ForegroundColor Green -NoNewline; Write-Host $Msg }
function Write-Warn { param([string]$Msg) Write-Host "  ! " -ForegroundColor Yellow -NoNewline; Write-Host $Msg }
function Write-Err { param([string]$Msg) Write-Host "  x " -ForegroundColor Red -NoNewline; Write-Host $Msg }

function Write-Header {
    param([string]$Title)
    Write-Host ""
    Write-Host "  $Title" -ForegroundColor Cyan
    Write-Host "  $('-' * $Title.Length)" -ForegroundColor DarkCyan
}

function Write-Banner {
    param([string]$Text, [ConsoleColor]$Color = "Cyan")
    Write-Host ""
    Write-Host "    ╔═══════════════════════════════════════╗" -ForegroundColor $Color
    Write-Host "    ║   $($Text.PadRight(36))║" -ForegroundColor $Color
    Write-Host "    ╚═══════════════════════════════════════╝" -ForegroundColor $Color
    Write-Host ""
}

# ─── Help ───────────────────────────────────────────────────────────────────

if ($Help) {
    Write-Host @"

  DAAO Satellite Installer for Windows

  Usage:
    irm https://your-nexus/install.ps1 | iex
    .\install-satellite.ps1 [-NexusUrl <url>] [-NoService] [-Uninstall]

  Parameters:
    -NexusUrl       Cockpit/Nexus URL (required - passed automatically by the one-liner)
    -NexusGrpcAddr  Nexus gRPC address (default: <host>:8444 derived from NexusUrl)
    -InstallDir     Override install directory
    -NoService      Skip startup task setup
    -Uninstall      Remove DAAO satellite

  Environment Variables:
    NEXUS_URL          Cockpit URL (required)
    NEXUS_GRPC_ADDR    Override gRPC address (optional, default: <host>:8444)

"@
    return
}

# ─── Platform Detection ────────────────────────────────────────────────────

function Get-Arch {
    $arch = $env:PROCESSOR_ARCHITECTURE
    switch ($arch) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { throw "Unsupported architecture: $arch" }
    }
}

# ─── Install Directory ─────────────────────────────────────────────────────

function Get-InstallDir {
    if ($InstallDir) { return $InstallDir }
    return Join-Path $env:LOCALAPPDATA "daao"
}

function Add-ToPath {
    param([string]$Dir)

    $currentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($currentPath -notlike "*$Dir*") {
        [Environment]::SetEnvironmentVariable("PATH", "$currentPath;$Dir", "User")
        $env:PATH = "$env:PATH;$Dir"
        Write-Success "Added $Dir to PATH"
    }
    else {
        Write-Info "$Dir is already in PATH"
    }
}

# ─── Download ──────────────────────────────────────────────────────────────

function Get-SatelliteBinary {
    param(
        [string]$Arch,
        [string]$DestDir
    )

    $binaryFile = "daao-windows-$Arch.exe"
    # Download from Cockpit /releases/ path (same server as install script)
    $url = "$NexusUrl/releases/$binaryFile"

    $dest = Join-Path $DestDir $BinaryName

    Write-Info "Downloading $BinaryName for windows/$Arch..."
    Write-Info "URL: $url"

    try {
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $url -OutFile $dest -UseBasicParsing -ErrorAction Stop

        # Verify we got a real binary, not an HTML error page
        $fileSize = (Get-Item $dest).Length
        if ($fileSize -lt 1024) {
            $content = Get-Content $dest -Raw -ErrorAction SilentlyContinue
            if ($content -match "<html|<!DOCTYPE") {
                Remove-Item $dest -Force
                throw "Received HTML instead of binary — release may not be published"
            }
        }

        Write-Success "Downloaded to $dest ($([math]::Round($fileSize / 1MB, 1)) MB)"
        return $true
    }
    catch {
        Write-Err "Download failed: $_"
        Write-Warn "You can build the satellite binary manually:"
        Write-Host ""
        Write-Host "    make build-satellite-windows-$Arch"
        Write-Host ""
        Write-Host "  Then copy bin\$binaryFile to $dest"
        return $false
    }
}

# ─── Registration ──────────────────────────────────────────────────────────

function Register-Satellite {
    param([string]$DaaoPath)

    if (-not (Test-Path $DaaoPath)) {
        Write-Warn "Binary not found at $DaaoPath — skipping registration"
        return
    }

    Write-Info "Generating keys and registering satellite with Nexus at $NexusUrl..."

    try {
        $env:NEXUS_URL = $NexusUrl
        $env:NEXUS_GRPC_ADDR = $NexusGrpcAddr
        & $DaaoPath login 2>&1 | ForEach-Object { Write-Host "    $_" }
        Write-Success "Satellite registered (keys generated, record created in Nexus)"
    }
    catch {
        Write-Warn "Registration failed — keys were saved. Run: `$env:NEXUS_URL='$NexusUrl'; daao login"
    }
}

# ─── Startup Task ──────────────────────────────────────────────────────────

function Install-StartupTask {
    param([string]$DaaoPath)

    # Don't install service if binary doesn't exist
    if (-not (Test-Path $DaaoPath)) {
        Write-Warn "Binary not found at $DaaoPath — skipping service setup"
        Write-Warn "Install the binary first, then re-run to set up the service"
        return "none"
    }

    Write-Info "Setting up Windows startup task..."

    # Try Windows Service first (requires admin), fall back to Task Scheduler
    $isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

    if ($isAdmin) {
        try {
            & sc.exe stop $ServiceName 2>&1 | Out-Null
            & sc.exe delete $ServiceName 2>&1 | Out-Null
            & sc.exe create $ServiceName binPath= "`"$DaaoPath`" start" start= auto DisplayName= "DAAO Satellite Daemon" 2>&1 | Out-Null
            & sc.exe description $ServiceName "DAAO Satellite Daemon - connects to Nexus for AI agent session management" 2>&1 | Out-Null
            & sc.exe start $ServiceName 2>&1 | Out-Null
            Write-Success "Windows Service '$ServiceName' installed and started"
            Write-Host ""
            Write-Info "Manage the service with:"
            Write-Host "    sc.exe query $ServiceName"
            Write-Host "    sc.exe stop $ServiceName"
            Write-Host "    sc.exe start $ServiceName"
            return "service"
        }
        catch {
            Write-Warn "Windows Service installation failed, falling back to Task Scheduler"
        }
    }

    # Task Scheduler fallback
    try {
        Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue

        $action = New-ScheduledTaskAction -Execute $DaaoPath -Argument "start"
        $trigger = New-ScheduledTaskTrigger -AtLogOn
        $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)

        Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Settings $settings -Description "DAAO Satellite Daemon" | Out-Null
        Start-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue

        Write-Success "Startup task '$TaskName' created"
        Write-Host ""
        Write-Info "Manage the task with:"
        Write-Host "    Get-ScheduledTask -TaskName '$TaskName'"
        Write-Host "    Start-ScheduledTask -TaskName '$TaskName'"
        Write-Host "    Stop-ScheduledTask -TaskName '$TaskName'"
        return "task"
    }
    catch {
        Write-Warn "Task Scheduler setup failed: $_"
        Write-Warn "You can start the daemon manually: daao start"
        return "none"
    }
}

# ─── Uninstall ──────────────────────────────────────────────────────────────

function Invoke-Uninstall {
    Write-Banner "Uninstalling DAAO Satellite" -Color Yellow

    $isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    if ($isAdmin) {
        & sc.exe stop $ServiceName 2>&1 | Out-Null
        & sc.exe delete $ServiceName 2>&1 | Out-Null
        Write-Success "Windows Service removed"
    }

    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
    Write-Success "Startup task removed"

    $dir = Get-InstallDir
    $bin = Join-Path $dir $BinaryName
    if (Test-Path $bin) {
        Remove-Item $bin -Force
        Write-Success "Binary removed: $bin"
    }

    $currentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($currentPath -like "*$dir*") {
        $newPath = ($currentPath -split ";" | Where-Object { $_ -ne $dir }) -join ";"
        [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
        Write-Success "Removed $dir from PATH"
    }

    if (Test-Path $ConfigDir) {
        Write-Warn "Configuration directory still exists: $ConfigDir"
        Write-Host "  Remove it manually to delete satellite keys:"
        Write-Host "    Remove-Item -Recurse -Force '$ConfigDir'"
    }

    Write-Host ""
    Write-Success "DAAO satellite uninstalled"
    return
}

# ─── Main ───────────────────────────────────────────────────────────────────

function Main {
    Write-Banner "DAAO Satellite Daemon Installer"

    if ($Uninstall) {
        Invoke-Uninstall
        return
    }

    # Step 1: Detect platform
    Write-Header "1. Detecting Platform"
    $arch = Get-Arch
    Write-Success "Platform: windows/$arch"

    # Step 2: Prepare install directory
    Write-Header "2. Preparing Install Directory"
    $dir = Get-InstallDir
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    Write-Success "Install directory: $dir"

    $binPath = Join-Path $dir $BinaryName
    if (Test-Path $binPath) {
        try {
            $ver = & $binPath --version 2>&1
            Write-Warn "DAAO is already installed (version: $ver)"
        }
        catch {
            Write-Warn "DAAO is already installed"
        }
        Write-Info "Re-installing..."
    }

    # Step 3: Download binary
    Write-Header "3. Downloading DAAO Satellite Binary"
    $downloaded = Get-SatelliteBinary -Arch $arch -DestDir $dir

    # Step 4: Add to PATH
    Write-Header "4. Updating PATH"
    Add-ToPath -Dir $dir

    # Step 5: Download Nexus CA certificate for TLS verification
    Write-Header "5. Downloading Nexus CA Certificate"
    $caUrl = "$NexusUrl/releases/ca.crt"
    $caDest = Join-Path $ConfigDir "nexus-ca.crt"
    New-Item -ItemType Directory -Path $ConfigDir -Force | Out-Null
    try {
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $caUrl -OutFile $caDest -UseBasicParsing -ErrorAction Stop
        Write-Success "CA certificate saved to $caDest"
    } catch {
        Write-Warn "Could not download CA cert from $caUrl"
        Write-Warn "'daao login' will use TOFU (Trust On First Use) instead"
    }

    # Step 6: Register with Nexus
    Write-Header "6. Registering Satellite"
    Register-Satellite -DaaoPath $binPath

    # Step 5b: Write daemon.env for Windows Service
    $envFile = Join-Path $dir "daemon.env"

    # Detect Pi binary path — prefer pi.cmd over pi.ps1
    # PowerShell's Get-Command returns .ps1 first, but the Go daemon needs .cmd
    # because resolvePiCommand only knows how to unwrap .cmd/.bat wrappers.
    $piPath = ""
    $piCmd = Get-Command pi -ErrorAction SilentlyContinue
    if ($piCmd) {
        $piDir = Split-Path $piCmd.Source
        $piCmdPath = Join-Path $piDir "pi.cmd"
        if (Test-Path $piCmdPath) {
            $piPath = $piCmdPath
        }
        else {
            $piPath = $piCmd.Source
        }
        Write-Success "Found Pi at $piPath"
    }
    else {
        Write-Warn "pi binary not found in PATH — DAAO_PI_PATH will be empty. Install from pi.dev and re-run."
    }

    # Create extensions directory next to the binary
    $extensionsDir = Join-Path $dir "extensions"
    if (-not (Test-Path $extensionsDir)) {
        New-Item -ItemType Directory -Path $extensionsDir -Force | Out-Null
    }
    Write-Success "Extensions directory: $extensionsDir"

    $envContent = @"
# DAAO Satellite Daemon Configuration
# Written by installer — used by Windows Service (LocalSystem has no user env)
NEXUS_URL=$NexusUrl
NEXUS_GRPC_ADDR=$NexusGrpcAddr
HOME=$env:USERPROFILE
USERPROFILE=$env:USERPROFILE
DAAO_PI_PATH=$piPath
DAAO_EXTENSIONS_PATH=$extensionsDir
"@
    Set-Content -Path $envFile -Value $envContent -Force
    Write-Success "Configuration written to $envFile"

    # Step 6: Set up service
    $serviceMethod = "none"
    if (-not $NoService) {
        Write-Header "6. Setting Up Background Service"
        $serviceMethod = Install-StartupTask -DaaoPath $binPath
    }
    else {
        Write-Info "Skipping service setup (-NoService)"
    }

    # Summary
    Write-Banner "Installation Complete!" -Color Green

    Write-Host "  Binary:     $binPath"
    Write-Host "  Config:     $ConfigDir"
    Write-Host "  Nexus URL:  $NexusUrl"
    Write-Host ""

    $hasBinary = Test-Path $binPath
    Write-Host "  Next Steps:" -ForegroundColor White

    if (-not $hasBinary) {
        Write-Host "    1. Build or download the daao binary:"
        Write-Host "         make build-satellite-windows-$arch"
        Write-Host "       Then copy bin\daao-windows-$arch.exe to $binPath"
        Write-Host "    2. Register with Nexus:  daao login"
        Write-Host "    3. Re-run this installer to set up the background service"
    }
    else {
        Write-Host "    1. Verify the daemon is running:"
        if ($serviceMethod -eq "service") {
            Write-Host "         sc.exe query $ServiceName"
        }
        elseif ($serviceMethod -eq "task") {
            Write-Host "         Get-ScheduledTask -TaskName '$TaskName'"
        }
        else {
            Write-Host "         daao start"
        }
        Write-Host "    2. Open Cockpit to see this satellite"
        Write-Host "    3. Start an AI agent session:"
        Write-Host "         daao run claude-code"
    }
    Write-Host ""
}

Main
