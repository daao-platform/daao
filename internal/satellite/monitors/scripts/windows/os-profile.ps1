# os-profile.ps1 — Deterministic OS profile collection for Windows
# Outputs structured JSON with OS, kernel, architecture, hostname, and uptime.
# Usage: powershell -ExecutionPolicy Bypass -File os-profile.ps1

$ErrorActionPreference = "SilentlyContinue"

try {
    $os = Get-CimInstance Win32_OperatingSystem
    $cs = Get-CimInstance Win32_ComputerSystem
    
    $uptime = (New-TimeSpan -Start $os.LastBootUpTime -End (Get-Date)).TotalSeconds
    $bootTime = $os.LastBootUpTime.ToString("yyyy-MM-ddTHH:mm:ssZ")
    
    # Detect virtualization
    $virt = "physical"
    $model = $cs.Model
    if ($model -match "Virtual|VMware|KVM|QEMU|Hyper-V|VirtualBox|Xen") {
        if ($model -match "VMware") { $virt = "vmware" }
        elseif ($model -match "Virtual") { $virt = "hyperv" }
        elseif ($model -match "KVM|QEMU") { $virt = "kvm" }
        elseif ($model -match "VirtualBox") { $virt = "virtualbox" }
        elseif ($model -match "Xen") { $virt = "xen" }
        else { $virt = "virtual" }
    }
    
    # Detect if running in container
    if (Test-Path "/.dockerenv" -ErrorAction SilentlyContinue) { $virt = "container" }

    $result = @{
        schema_version = "1.0"
        snapshot_type  = "os_profile"
        os             = "windows"
        collected_at   = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
        status         = "complete"
        data           = @{
            os_family       = "windows"
            os_id           = "windows"
            os_name         = $os.Caption
            os_version      = $os.Version
            os_pretty       = "$($os.Caption) ($($os.BuildNumber))"
            kernel          = $os.Version
            arch            = $env:PROCESSOR_ARCHITECTURE.ToLower()
            hostname        = $env:COMPUTERNAME
            uptime_seconds  = [math]::Floor($uptime)
            boot_time       = $bootTime
            timezone        = (Get-TimeZone).Id
            virtualization  = $virt
        }
    }

    $result | ConvertTo-Json -Depth 5 -Compress
} catch {
    @{
        schema_version = "1.0"
        snapshot_type  = "os_profile"
        os             = "windows"
        status         = "error"
        data           = @{ error = $_.Exception.Message }
    } | ConvertTo-Json -Depth 3 -Compress
}
