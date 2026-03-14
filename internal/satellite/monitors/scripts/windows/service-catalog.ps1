# service-catalog.ps1 — Deterministic service and container discovery for Windows
# Outputs structured JSON with running services, containers, and listening ports.
# Usage: powershell -ExecutionPolicy Bypass -File service-catalog.ps1

$ErrorActionPreference = "SilentlyContinue"

try {
    # Running services
    $services = Get-Service | Where-Object { $_.Status -eq 'Running' } | ForEach-Object {
        $_.Name
    }
    if (-not $services) { $services = @() }

    # Docker containers
    $containers = @()
    $dockerInstalled = $false
    if (Get-Command docker -ErrorAction SilentlyContinue) {
        $dockerInstalled = $true
        $dockerOutput = docker ps --format '{{json .}}' 2>$null
        if ($dockerOutput) {
            $containers = $dockerOutput | ForEach-Object {
                $c = $_ | ConvertFrom-Json
                @{
                    name   = $c.Names
                    image  = $c.Image
                    status = $c.Status
                    ports  = $c.Ports
                }
            }
        }
    }

    # Listening ports
    $ports = Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | ForEach-Object {
        $procName = ""
        try {
            $proc = Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue
            $procName = $proc.ProcessName
        } catch {}
        @{
            port    = $_.LocalPort
            process = $procName
            address = $_.LocalAddress
        }
    } | Sort-Object { $_.port } -Unique
    if (-not $ports) { $ports = @() }

    $result = @{
        schema_version = "1.0"
        snapshot_type  = "service_catalog"
        os             = "windows"
        collected_at   = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
        status         = "complete"
        data           = @{
            services           = @($services)
            containers         = @($containers)
            listening_ports    = @($ports)
            container_runtimes = @{
                docker = $dockerInstalled
                podman = [bool](Get-Command podman -ErrorAction SilentlyContinue)
            }
        }
    }

    $result | ConvertTo-Json -Depth 5 -Compress
} catch {
    @{
        schema_version = "1.0"
        snapshot_type  = "service_catalog"
        os             = "windows"
        status         = "error"
        data           = @{ error = $_.Exception.Message }
    } | ConvertTo-Json -Depth 3 -Compress
}
