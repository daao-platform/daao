# network-topology.ps1 — Deterministic network topology collection for Windows
# Outputs structured JSON with interfaces, routes, DNS, and connections.
# Usage: powershell -ExecutionPolicy Bypass -File network-topology.ps1

$ErrorActionPreference = "SilentlyContinue"

try {
    # Network interfaces with IPs
    $interfaces = Get-NetIPAddress -ErrorAction SilentlyContinue | Where-Object {
        $_.AddressFamily -eq 'IPv4' -and $_.IPAddress -ne '127.0.0.1'
    } | ForEach-Object {
        $adapter = Get-NetAdapter -InterfaceIndex $_.InterfaceIndex -ErrorAction SilentlyContinue
        @{
            name          = $adapter.Name
            interface_idx = $_.InterfaceIndex
            ip_address    = $_.IPAddress
            prefix_length = $_.PrefixLength
            status        = $adapter.Status
            mac_address   = $adapter.MacAddress
            link_speed    = $adapter.LinkSpeed
        }
    }
    if (-not $interfaces) { $interfaces = @() }

    # Routes
    $routes = Get-NetRoute -ErrorAction SilentlyContinue | Where-Object {
        $_.AddressFamily -eq 'IPv4'
    } | Select-Object -First 50 | ForEach-Object {
        @{
            destination    = $_.DestinationPrefix
            next_hop       = $_.NextHop
            interface_idx  = $_.InterfaceIndex
            metric         = $_.RouteMetric
        }
    }
    if (-not $routes) { $routes = @() }

    # DNS servers
    $dnsServers = Get-DnsClientServerAddress -ErrorAction SilentlyContinue | Where-Object {
        $_.AddressFamily -eq 2  # IPv4
    } | ForEach-Object { $_.ServerAddresses } | Sort-Object -Unique
    if (-not $dnsServers) { $dnsServers = @() }

    # Default gateway
    $gateway = (Get-NetRoute -DestinationPrefix '0.0.0.0/0' -ErrorAction SilentlyContinue | 
        Select-Object -First 1).NextHop
    if (-not $gateway) { $gateway = "" }

    $result = @{
        schema_version = "1.0"
        snapshot_type  = "network_topology"
        os             = "windows"
        collected_at   = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
        status         = "complete"
        data           = @{
            interfaces      = @($interfaces)
            routes          = @($routes)
            dns_servers     = @($dnsServers)
            default_gateway = $gateway
            public_ip       = ""
            hostname        = $env:COMPUTERNAME
        }
    }

    $result | ConvertTo-Json -Depth 5 -Compress
} catch {
    @{
        schema_version = "1.0"
        snapshot_type  = "network_topology"
        os             = "windows"
        status         = "error"
        data           = @{ error = $_.Exception.Message }
    } | ConvertTo-Json -Depth 3 -Compress
}
