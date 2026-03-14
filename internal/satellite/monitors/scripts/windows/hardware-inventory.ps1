# hardware-inventory.ps1 — Deterministic hardware inventory collection for Windows
# Outputs structured JSON with CPU, memory, disks, and GPU info.
# Usage: powershell -ExecutionPolicy Bypass -File hardware-inventory.ps1

$ErrorActionPreference = "SilentlyContinue"

try {
    # CPU
    $cpu = Get-CimInstance Win32_Processor | Select-Object -First 1
    $cpuInfo = @{
        model         = $cpu.Name.Trim()
        cores         = $cpu.NumberOfCores
        physical_cpus = (Get-CimInstance Win32_Processor).Count
        threads       = $cpu.NumberOfLogicalProcessors
    }

    # Memory
    $os = Get-CimInstance Win32_OperatingSystem
    $memTotalGB = [math]::Round($os.TotalVisibleMemorySize / 1MB, 2)
    $memAvailGB = [math]::Round($os.FreePhysicalMemory / 1MB, 2)
    
    $pagefile = Get-CimInstance Win32_PageFileUsage -ErrorAction SilentlyContinue | 
        Select-Object -First 1
    $swapGB = if ($pagefile) { [math]::Round($pagefile.AllocatedBaseSize / 1024, 2) } else { 0 }

    $memInfo = @{
        total_gb     = $memTotalGB
        available_gb = $memAvailGB
        swap_gb      = $swapGB
    }

    # Disks
    $disks = Get-CimInstance Win32_DiskDrive | ForEach-Object {
        @{
            name       = $_.DeviceID
            model      = ($_.Model ?? "").Trim()
            size_gb    = [math]::Round($_.Size / 1GB, 1)
            partitions = $_.Partitions
            media_type = $_.MediaType
        }
    }
    if (-not $disks) { $disks = @() }

    # Logical volumes (for mount point info)
    $volumes = Get-CimInstance Win32_LogicalDisk | Where-Object { $_.DriveType -eq 3 } | ForEach-Object {
        @{
            drive_letter = $_.DeviceID
            label        = $_.VolumeName
            size_gb      = [math]::Round($_.Size / 1GB, 1)
            free_gb      = [math]::Round($_.FreeSpace / 1GB, 1)
            filesystem   = $_.FileSystem
        }
    }
    if (-not $volumes) { $volumes = @() }

    # GPUs
    $gpus = Get-CimInstance Win32_VideoController | ForEach-Object {
        @{
            description = $_.Name
            driver      = $_.DriverVersion
            ram_mb      = [math]::Round($_.AdapterRAM / 1MB, 0)
        }
    }
    if (-not $gpus) { $gpus = @() }

    $result = @{
        schema_version = "1.0"
        snapshot_type  = "hardware_inventory"
        os             = "windows"
        collected_at   = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
        status         = "complete"
        data           = @{
            cpu     = $cpuInfo
            memory  = $memInfo
            disks   = @($disks)
            volumes = @($volumes)
            gpus    = @($gpus)
        }
    }

    $result | ConvertTo-Json -Depth 5 -Compress
} catch {
    @{
        schema_version = "1.0"
        snapshot_type  = "hardware_inventory"
        os             = "windows"
        status         = "error"
        data           = @{ error = $_.Exception.Message }
    } | ConvertTo-Json -Depth 3 -Compress
}
