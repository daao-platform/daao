# ============================================================================
# setup.ps1 — Interactive first-time setup for DAAO (Windows PowerShell)
# ============================================================================
#
# Generates secrets, TLS certificates, and .env — everything needed to
# run `docker-compose up` for the first time.
#
# Usage:
#   .\scripts\setup.ps1
#
# ============================================================================

$ErrorActionPreference = "Stop"

function Write-Step($num, $total, $msg) { Write-Host "`n  [$num/$total] $msg" -ForegroundColor White }
function Write-Ok($msg)   { Write-Host "  ✓ $msg" -ForegroundColor Green }
function Write-Info($msg)  { Write-Host "  → $msg" -ForegroundColor Cyan }
function Write-Warn($msg)  { Write-Host "  ! $msg" -ForegroundColor Yellow }

# ---------- Detect repo root ------------------------------------------------

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot  = Split-Path -Parent $ScriptDir
Set-Location $RepoRoot

$TotalSteps = 4

Write-Host ""
Write-Host "  🔧  DAAO Setup" -ForegroundColor White
Write-Host "  ─────────────────────────────────────────" -ForegroundColor DarkGray

# ---------- Pre-flight -------------------------------------------------------

if (Test-Path ".env") {
    Write-Host ""
    Write-Warn ".env already exists."
    $overwrite = Read-Host "  Overwrite? [y/N]"
    if ($overwrite -ne "y" -and $overwrite -ne "Y") {
        Write-Info "Setup cancelled. Your existing .env was not modified."
        exit 0
    }
}

# ---------- Step 1: Generate secrets ----------------------------------------

Write-Step 1 $TotalSteps "Generating secrets"

function New-Secret {
    $bytes = New-Object byte[] 33
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    return [Convert]::ToBase64String($bytes).Substring(0, 44)
}

$JwtSecret = New-Secret
$PostgresPassword = New-Secret

Write-Ok "JWT_SECRET generated"
Write-Ok "POSTGRES_PASSWORD generated"

# ---------- Step 2: Generate TLS certificates --------------------------------

Write-Step 2 $TotalSteps "Generating TLS certificates"

$CertDir = Join-Path $RepoRoot "certs"
New-Item -ItemType Directory -Path $CertDir -Force | Out-Null

if ((Test-Path "$CertDir\ca.crt") -and (Test-Path "$CertDir\server.crt") -and (Test-Path "$CertDir\key.pem")) {
    Write-Ok "Certificates already exist in certs/ — skipping"
} else {
    # Check for openssl
    $opensslPath = Get-Command openssl -ErrorAction SilentlyContinue
    if (-not $opensslPath) {
        # Try common Windows locations
        $gitOpenssl = "C:\Program Files\Git\usr\bin\openssl.exe"
        if (Test-Path $gitOpenssl) {
            $env:PATH = "C:\Program Files\Git\usr\bin;$env:PATH"
        } else {
            Write-Host ""
            Write-Host "  ✗ openssl not found." -ForegroundColor Red
            Write-Host "    Install Git for Windows (includes openssl) or add openssl to PATH." -ForegroundColor Red
            exit 1
        }
    }

    # Generate CA
    openssl genrsa -out "$CertDir\ca.key" 2048 2>$null
    openssl req -new -x509 -key "$CertDir\ca.key" `
        -out "$CertDir\ca.crt" `
        -days 3650 `
        -subj "/CN=DAAO Local CA" 2>$null
    Write-Ok "CA certificate created (valid 10 years)"

    # Generate server key
    openssl genrsa -out "$CertDir\key.pem" 2048 2>$null

    # Create SAN config
    $sanConfig = @"
[req]
default_bits = 2048
prompt = no
distinguished_name = dn
req_extensions = v3_req

[dn]
CN = localhost

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = nexus
DNS.3 = *.daao.local
IP.1 = 127.0.0.1
"@
    $sanConfig | Out-File -FilePath "$CertDir\server.cnf" -Encoding ASCII

    openssl req -new -key "$CertDir\key.pem" `
        -out "$CertDir\server.csr" `
        -config "$CertDir\server.cnf" 2>$null

    openssl x509 -req -in "$CertDir\server.csr" `
        -CA "$CertDir\ca.crt" -CAkey "$CertDir\ca.key" `
        -CAcreateserial `
        -out "$CertDir\server.crt" `
        -days 365 `
        -extensions v3_req `
        -extfile "$CertDir\server.cnf" 2>$null

    # Clean up
    Remove-Item "$CertDir\server.csr", "$CertDir\server.cnf", "$CertDir\ca.srl" -ErrorAction SilentlyContinue

    Write-Ok "Server certificate created (valid 1 year)"
    Write-Ok "SAN: localhost, nexus, *.daao.local, 127.0.0.1"
}

# ---------- Step 3: Owner account -------------------------------------------

Write-Step 3 $TotalSteps "Owner account (optional)"

Write-Host "  The owner is the first admin user. You can also create" -ForegroundColor DarkGray
Write-Host "  users later via the API. Press Enter to skip." -ForegroundColor DarkGray
Write-Host ""

$OwnerEmail = Read-Host "  Email for admin user"

if ($OwnerEmail) {
    Write-Ok "Owner email set: $OwnerEmail"
    Write-Info "A random password will be printed to the Nexus log on first boot"
} else {
    Write-Info "Skipped — no owner account will be created"
}

# ---------- Step 4: Write .env -----------------------------------------------

Write-Step 4 $TotalSteps "Writing .env"

$timestamp = (Get-Date).ToUniversalTime().ToString("yyyy-MM-dd HH:mm:ss UTC")

$envContent = @"
# DAAO Environment — generated by scripts/setup.ps1
# $timestamp

# Security
JWT_SECRET=$JwtSecret

# Database
POSTGRES_PASSWORD=$PostgresPassword

# Owner account
DAAO_OWNER_EMAIL=$OwnerEmail

# Cockpit port
COCKPIT_PORT=8081
"@

$envContent | Out-File -FilePath ".env" -Encoding ASCII
Write-Ok ".env created"

# ---------- Done -------------------------------------------------------------

Write-Host ""
Write-Host "  ─────────────────────────────────────────" -ForegroundColor DarkGray
Write-Host "  ✅  Setup complete!" -ForegroundColor White
Write-Host ""
Write-Host "  To start DAAO:" -ForegroundColor White
Write-Host "    docker-compose up --build" -ForegroundColor Cyan
Write-Host ""
Write-Host "  Then open:" -ForegroundColor White
Write-Host "    http://localhost:8081  — Cockpit dashboard" -ForegroundColor Cyan
Write-Host ""
if ($OwnerEmail) {
    Write-Host "  Your admin password will appear in the Nexus container log." -ForegroundColor DarkGray
    Write-Host ""
}
