# WebTransport Design Notes

> Design discussion from 2026-03-07. Covers architecture decisions, TLS strategy, reverse proxy constraints, and final recommendations for the browser ↔ Nexus WebTransport path.

---

## Starting Point: Terminal Glitches Investigation

While investigating visual glitches (garbled/overlapping text) in the cockpit terminal, we discovered that the client tries WebTransport first and fails over to WebSocket. Three terminal rendering bugs were identified (unrelated to WebTransport):

1. **`convertEol: true` double-conversion** — ConPTY on Windows already emits `\r\n`. xterm.js's `convertEol: true` converts every `\n` → `\r\n`, so each newline becomes `\r\r\n`, causing lines to overwrite each other.
2. **Ring buffer delta race** — `websocket.go` calls `rb.Len()` and `rb.Snapshot()` in separate lock acquisitions; data can be written between them, causing duplicate or missed data.
3. **Initial resize race** — FitAddon fires `onResize` before the transport is connected, so the initial terminal dimensions are silently dropped.

---

## WebTransport Current State

### What's Built

| Layer | Status |
|---|---|
| **Server startup** (`main.go`) | ✅ H3/QUIC listener on `:8446`, port exposed in `docker-compose.yml` |
| **Client negotiation** (`negotiate.ts`) | ✅ Tries WebTransport first, falls back to WebSocket |
| **Client adapter** (`WebTransportTransport.ts`) | ✅ Wraps `WebTransportClient` for the `TransportClient` interface |
| **Server upgrade handler** (`router.go`) | ✅ `/webtransport` endpoint, parses `session_id`, calls `server.Upgrade()` |
| **CSP header** (`nginx.conf`) | ✅ Already allows `connect-src https://*:8446` |

### What's Missing (3 Blockers)

1. **TLS certificates** — Browsers refuse self-signed certs for QUIC. No "click through" option like HTTPS. Needs either CA-signed certs or the `serverCertificateHashes` workaround.
2. **Server-side stream handlers are stubs** — `forwardTerminalInput()`, `forwardTerminalOutput()`, `forwardControlMessages()`, `forwardOOBUI()` in `router.go` are all empty no-ops.
3. **Client stream model is wrong** — `index.ts` calls `createUnidirectionalStream()` for terminal RX (receive), but that creates a *send-only* stream. Needs `transport.incomingUnidirectionalStreams` to receive data from the server.

---

## TLS Strategy Options Discussed

### Option 1: Let's Encrypt (ACME)

- ✅ Right for **production** (Nexus server reachable by domain)
- ❌ Doesn't solve **local dev** (no domain, no public IP)
- Recommended approach: put a reverse proxy (Caddy, NPM, etc.) in front that handles ACME — don't bolt it into the Go binary

### Option 2: `serverCertificateHashes` API (Dev Mode)

- Browser API that trusts a specific self-signed cert by its SHA-256 hash
- Cert must be ≤14 days validity, EC key only
- Nexus would generate a short-lived cert on startup and expose the hash via API
- ✅ Elegant for dev — no manual cert installation
- ❌ Adds complexity for a scenario where WebSocket works fine

### Option 3: Dual-Mode Architecture (Auto-Detection)

- Nexus detects its cert type on startup (self-signed vs CA-signed)
- Exposes `/api/v1/transport/config` telling the client what's available
- Client fetches config before attempting WebTransport
- Graceful fallback to WebSocket always available

---

## Key Constraint: Reverse Proxies Can't Proxy QUIC

WebTransport runs over **QUIC (UDP)**. Every major reverse proxy — NPM, Nginx, Caddy, Traefik, HAProxy — is fundamentally a TCP/HTTP proxy. None of them can forward QUIC/WebTransport connections to an upstream server.

This means the browser must connect **directly** to Nexus on port 8446 for WebTransport, bypassing the reverse proxy entirely. The reverse proxy only handles HTTP/WebSocket traffic.

### Impact on Homelab Setup (NPM on Separate Server)

```
Phone (on bus)
    │
    ├── HTTPS (:443) → UDM-SE → NPM server → Cockpit/Nexus (WebSocket) ✅
    │                   port fwd   reverse proxy
    │
    └── QUIC (:8446) → UDM-SE → Nexus host directly (WebTransport) ✅
                        port fwd   (bypasses NPM)
```

Requires one additional port-forward rule on the UDM-SE: `UDP 8446` → Nexus host.

### Reverse Proxy Compatibility Table

| Proxy | HTTP/3 Support | Can Proxy WebTransport? | ACME Built-in |
|---|---|---|---|
| **NPM** | ❌ | ❌ (expose directly) | ✅ GUI |
| **Nginx** | ✅ (1.25+) | ❌ (expose directly) | ❌ (certbot) |
| **Caddy** | ✅ (built-in) | ❌ (expose directly) | ✅ (auto) |
| **Traefik** | ✅ (v3) | ❌ (expose directly) | ✅ (built-in) |
| **HAProxy** | ⚠️ (experimental) | ❌ (expose directly) | ❌ (external) |

---

## Why NPM Can't Handle UDP (Simple Explanation)

**TCP** (what HTTP and NPM use) is like certified mail — you establish a connection first, every message is guaranteed to arrive in order, lost packets are re-sent automatically.

**UDP** (what QUIC uses) is like shouting across a room — you just start sending, no handshake. Faster, but the application (QUIC) handles reliability itself.

Nginx (underlying NPM) is fundamentally built to:
1. Accept **TCP** connections
2. Read **HTTP** request headers
3. Forward to a backend over **TCP**

It literally doesn't have the code to listen on UDP sockets, understand QUIC packets (encrypted from the first byte), or relay UDP datagrams.

---

## Security Analysis of Exposing Port 8446/UDP

### What's Protected

- **TLS 1.3 mandatory** — QUIC encrypts from the very first packet. No plaintext phase. Port scanners can't fingerprint it.
- **Session-scoped** — `/webtransport?session_id=...` requires a valid session ID; invalid connections are rejected immediately after upgrade.
- **JWT authenticated** — In production, requires a valid token (same auth as WebSocket).

### Risk Comparison

| Port | Protocol | Risk Profile |
|---|---|---|
| `:443` (NPM) | TCP/TLS | Full HTTP surface — API, WebSocket, static files |
| `:8443` (Nexus) | TCP/TLS | REST API, WebSocket terminal streams |
| `:8444` (Nexus) | TCP/TLS | gRPC — satellite mTLS connections |
| **`:8446` (new)** | **UDP/QUIC/TLS** | **WebTransport only — session-scoped, authenticated** |

Port 8446 has the **smallest** attack surface because it only speaks one protocol and only does one thing.

### Genuine Risks

1. **UDP amplification** — Mitigated by QUIC's address validation tokens (built into `quic-go`)
2. **Bugs in `quic-go`** — Well-maintained (used by Cloudflare, Caddy) but still a dependency to keep updated
3. **Bypass of NPM access controls** — IP allowlists/geo-blocking on NPM won't apply to direct QUIC connections; need equivalent host-level firewall rules

### UDM-SE Specific

UniFi's IDS/IPS inspects traffic on all forwarded ports, including UDP. It'll flag anomalous patterns on 8446 just like any other forwarded port.

---

## Alternative Considered: gRPC-over-QUIC for Satellites

We explored putting QUIC on the **satellite ↔ Nexus** link instead of the **browser ↔ Nexus** link. QUIC's connection migration would let satellite laptops survive WiFi→cellular network changes without dropping the gRPC stream.

### Pros
- ✅ No extra ports needed (satellite initiates outbound)
- ✅ Connection migration = sessions survive network changes
- ✅ 0-RTT reconnect = faster recovery
- ✅ Marketing differentiator

### Cons
- ❌ Nexus still needs a UDP port for satellites to connect to
- ❌ The existing gRPC reconnection with exponential backoff already works
- ❌ Ring buffer replay handles the transition gracefully
- ❌ Building for a problem no customer has reported yet

### Conclusion

The satellite QUIC idea is valid but premature. The existing TCP/gRPC path with exponential-backoff reconnect and ring buffer replay provides a good enough experience. This can be revisited when customers report satellite connectivity issues on unreliable networks.

---

## What Products Like DAAO Actually Do

| Product | Browser → Server | Server → Agent |
|---|---|---|
| **GitHub Codespaces** | WebSocket | Internal (Azure fabric) |
| **Cloudflare Tunnel** | HTTPS/WebSocket | QUIC (cloudflared) |
| **Tailscale** | HTTPS (web console) | WireGuard (UDP) |
| **GitPod** | WebSocket | Internal gRPC |
| **Teleport** | WebSocket | SSH / gRPC |

Pattern: QUIC/UDP for machine-to-machine, WebSocket for browser. But DAAO's four-stream WebTransport architecture (Terminal RX/TX, Control, OOB UI) enables **multiplexed agent UI channels** — a differentiator for Agent Forge that competitors don't have.

---

## Final Recommendation

### Priority 1: Ship v1.0 with WebSocket (Now)
- Fix the three terminal rendering bugs (`convertEol`, ring buffer race, resize race)
- WebSocket through any reverse proxy just works
- No deployment friction

### Priority 2: WebTransport as Opt-In Upgrade (Post-Launch)
- Keep existing scaffolding (`router.go`, `negotiate.ts`, `WebTransportTransport.ts`)
- Complete the server-side stream handlers to bridge ring buffer + gRPC
- Fix the client stream direction model
- Auto-detect via `/api/v1/transport/config`
- Graceful fallback to WebSocket when QUIC port isn't available
- Document deployment: "For WebTransport, forward UDP 8446 to your Nexus host"
- Market as premium: *"Low-latency terminal streaming with multiplexed agent UI channels"*

### Priority 3: gRPC-over-QUIC for Satellites (Future, If Needed)
- Only build when customers report satellite connectivity issues
- Don't engineer for a problem nobody has complained about

### Deployment Guides Needed (Phase 3)
- `docs/deployment/npm.md` — NPM on separate server, UDP port forward
- `docs/deployment/nginx.md` — Nginx manual config
- `docs/deployment/caddy.md` — Caddy with auto-ACME
- `docs/deployment/traefik.md` — Traefik v3 with ACME resolver
- `docs/deployment/haproxy.md` — HAProxy TCP mode + certbot

---

## Summary

The original WebTransport design (four multiplexed streams for Terminal, Control, OOB UI) is architecturally correct and forward-looking. The practical constraint is that no reverse proxy can forward QUIC, so WebTransport requires direct access to Nexus on a UDP port. The graceful WebSocket fallback in `negotiate.ts` handles this perfectly — users behind a proxy get WebSocket, users with direct access get the premium WebTransport experience. Ship with WebSocket for v1.0, complete WebTransport post-launch.
