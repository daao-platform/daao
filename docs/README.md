<p align="center">
  <h1 align="center">🛰️ DAAO</h1>
  <p align="center"><strong>Distributed AI Agent Orchestration</strong></p>
  <p align="center">
    A platform for running distributed AI agents with persistent sessions,<br/>
    real-time terminal streaming, and automatic lifecycle management.
  </p>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white" alt="Go 1.26"/>
  <img src="https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black" alt="React"/>
  <img src="https://img.shields.io/badge/gRPC-Streaming-244c5a?logo=grpc&logoColor=white" alt="gRPC"/>
  <img src="https://img.shields.io/badge/PostgreSQL-336791?logo=postgresql&logoColor=white" alt="PostgreSQL"/>
  <img src="https://img.shields.io/badge/WebSocket-Streaming-blueviolet" alt="WebSocket"/>
  <img src="https://img.shields.io/badge/License-BSL_1.1-blue" alt="BSL 1.1"/>
</p>

---

## What is DAAO?

DAAO lets you launch AI agent sessions on **any machine** (called a *Satellite*), detach from them, and resume later — from any device. Think of it as `tmux` for AI agents, with a web dashboard, real-time streaming, and automatic state management.

### Key Capabilities

| Feature | Description |
|---|---|
| **Persistent Sessions** | Sessions survive disconnects. Detach, close your laptop, re-attach from your phone. |
| **Live Terminal Streaming** | Real-time terminal output via WebSocket (primary) and WebTransport HTTP/3 (secondary). |
| **Dead Man's Switch (DMS)** | Automatic process suspension after idle timeout. Resume instantly. |
| **Session State Machine** | 6-state lifecycle: `PROVISIONING → RUNNING → DETACHED → RE_ATTACHING → SUSPENDED → TERMINATED` |
| **Push Notifications** | Web Push API alerts for DMS triggers, INPUT_REQUIRED, and session events. |
| **Multi-Platform PTY** | Cross-platform pseudo-terminal support (Linux, macOS, Windows ConPTY). |
| **Kernel Telemetry** | System metrics (CPU, memory, disk, GPU) and kernel wait-state detection across Linux, macOS, and Windows. |
| **Authentication** | JWT + mTLS today; OAuth2 Device Flow (GitHub, Google) planned. |

---

## Architecture at a Glance

```
┌─────────────┐      gRPC         ┌──────────────┐       HTTP / WS       ┌──────────────┐
│  Satellite   │◀──── bidi ──────▶│    Nexus      │◀────────────────────▶│   Cockpit    │
│  (Daemon)    │    streaming     │  (API Gateway) │     REST + WS       │  (React SPA) │
│              │                  │                │    + WebSocket      │              │
│  ┌─────────┐ │                  │  ┌──────────┐  │                     │  ┌─────────┐ │
│  │  PTY    │ │                  │  │  Router  │  │                     │  │Terminal │ │
│  │  + DMS  │ │                  │  │  + Auth  │  │                     │  │  View   │ │
│  │  + IPC  │ │                  │  │  + Store │  │                     │  │  + OOB  │ │
│  └─────────┘ │                  │  └──────────┘  │                     │  └─────────┘ │
└─────────────┘                   └───────┬────────┘                     └──────────────┘
                                          │
                                   ┌──────┴──────┐
                                   │  PostgreSQL  │
                                   │  (sessions,  │
                                   │  satellites,  │
                                   │  event_logs)  │
                                   └─────────────┘
```

> See [ARCHITECTURE.md](./ARCHITECTURE.md) for the full deep dive with Mermaid diagrams.

---

## Quick Start

### Prerequisites

| Requirement | Version |
|---|---|
| Docker & Docker Compose | Latest |
| Go | 1.25+ |
| Node.js | 22+ |
| PostgreSQL | Latest stable (included in Docker Compose) |

### 1. Start All Services

```bash
# Clone the repository
git clone https://github.com/daao-platform/daao.git
cd daao

# Start the full stack (Postgres + Nexus + Cockpit)
docker compose up -d

# Verify all services are healthy
docker compose ps
```

### 2. Authenticate

```bash
# Authenticate with Nexus
./daao.exe login
```

### 3. Run an Agent Session

```bash
# Launch a new session
./daao.exe run -- bash

# List active sessions
./daao.exe run --list
```

### 4. Open the Dashboard

Navigate to **[http://localhost:8081](http://localhost:8081)** to access the Cockpit web UI.

---

## Project Structure

```
daao/
├── cmd/
│   ├── daao/            # Satellite daemon CLI (cross-platform)
│   └── nexus/           # API Gateway server (REST + gRPC + WebSocket + WebTransport)
├── internal/
│   ├── api/             # HTTP handlers, middleware, pagination
│   ├── auth/            # JWT validation, mTLS, rate limiting
│   ├── database/        # PostgreSQL connection pool
│   ├── enterprise/      # Enterprise-only features (proprietary license)
│   ├── grpc/            # gRPC gateway (SatelliteGateway bidi streaming)
│   ├── license/         # Ed25519 license key validation + feature gates
│   ├── notification/    # SSE push notifications, event bus, dispatchers
│   ├── recording/       # asciicast v2 session recording + playback
│   ├── router/          # WebSocket stream routing
│   ├── satellite/       # Satellite registration & mTLS certificates
│   ├── session/         # Session store with state machine + ring buffers
│   ├── stream/          # Stream registry (session → gRPC channel mapping)
│   └── transport/       # WebSocket + WebTransport terminal handlers
├── pkg/
│   ├── buffer/          # ANSI-boundary-aware ring buffer (5MB)
│   ├── ipc/             # JSON-RPC over Unix sockets / Named Pipes
│   ├── lifecycle/       # Dead Man's Switch (DMS) with event logging
│   ├── proc/            # Process detach/suspend (Unix & Windows)
│   ├── pty/             # Cross-platform PTY (Unix pty, Windows ConPTY)
│   ├── sysmetrics/      # System metrics collection (CPU, MEM, DISK)
│   └── telemetry/       # Kernel wait-state scraping (Linux, macOS, Windows)
├── proto/               # Protobuf definitions (SatelliteGateway service)
├── cockpit/             # React 19 + Vite + TypeScript frontend
├── db/migrations/       # SQL migration files (001–014)
├── deploy/              # Nginx reverse proxy config
├── docker-compose.yml   # Full stack orchestration
├── Dockerfile.nexus     # Multi-stage Go build
├── Dockerfile.cockpit   # Multi-stage Node + Nginx build
└── Makefile             # Build, test, lint, dev commands
```

---

## Documentation

| Document | Description |
|---|---|
| [Architecture](./ARCHITECTURE.md) | System architecture, component diagrams, data flow, and state machines |
| [API Reference](./API_REFERENCE.md) | REST endpoints, gRPC service, WebTransport protocol, and WebSocket API |
| [Database](./DATABASE.md) | Schema, migrations, state machine, and event logging |
| [Security](./SECURITY.md) | mTLS, JWT, OAuth2 Device Code Flow, rate limiting |
| [Deployment](./DEPLOYMENT.md) | Docker Compose, production config, Nginx, monitoring |
| [Development](./DEVELOPMENT.md) | Local setup, testing, code structure, contributing |

---

## Development

```bash
# Build everything
make build

# Run tests with race detector
make test

# Start development servers (Nexus + Cockpit)
make dev

# Lint
make lint

# Regenerate protobuf
make proto

# Clean build artifacts
make clean
```

---

## License

[Business Source License 1.1](../LICENSE) — free to self-host, modify, and use internally. Converts to Apache 2.0 on 2030-03-06.
