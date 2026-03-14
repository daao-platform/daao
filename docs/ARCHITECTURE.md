# Architecture

> System architecture, component interaction diagrams, data flow, and state machine specifications.

---

## System Overview

DAAO follows a **hub-and-spoke architecture** where Nexus acts as the central API gateway, Satellites are the remote execution agents, and Cockpit is the web-based control plane.

```mermaid
graph TB
    subgraph Clients
        CLI["daao CLI"]
        CK["Cockpit<br/>(React SPA)"]
        Mobile["Mobile Browser"]
    end

    subgraph "Nexus (API Gateway)"
        REST["REST API<br/>:8443"]
        GRPC["gRPC Server<br/>:8444"]
        WT["WebSocket<br/>(terminal stream)"]
        WS["WebSocket<br/>(session stream)"]
        Auth["Auth Middleware<br/>(JWT + mTLS)"]
        Router["Stream Router"]
        SR["Stream Registry"]
        RL["Rate Limiter"]
    end

    subgraph "Data Layer"
        PG["PostgreSQL<br/>(sessions, satellites, event_logs)"]
        Migrate["golang-migrate"]
    end

    subgraph "Satellite (Remote Machine)"
        Daemon["Satellite Daemon"]
        PTY["PTY Manager<br/>(Unix/ConPTY)"]
        DMS["Dead Man's Switch"]
        IPC["IPC Server<br/>(JSON-RPC)"]
        Ring["Ring Buffer<br/>(5MB, ANSI-aware)"]
        Telem["Telemetry<br/>(system metrics + wait-state)"]
        Agent["AI Agent Process"]
    end

    CLI -->|"JWT Auth"| Auth
    CLI -->|"run / login"| REST
    CK -->|"REST + WS"| REST
    CK -->|"WebSocket"| WT
    Mobile -->|"Web Push"| CK

    REST --> Auth
    Auth --> Router
    Router --> SR
    SR -->|"chan← NexusMessage"| GRPC

    GRPC <-->|"bidi stream<br/>SatelliteGateway.Connect"| Daemon

    REST --> PG
    Migrate --> PG

    Daemon --> PTY
    PTY --> Agent
    Daemon --> DMS
    Daemon --> IPC
    Daemon --> Ring
    Daemon --> Telem
    Telem --> Agent
```

---

## Component Deep Dive

### Nexus — API Gateway

Nexus is the central server that orchestrates all communication. It runs three server protocols simultaneously:

| Protocol | Port | Purpose |
|---|---|---|
| **HTTPS (REST)** | `:8443` | Session CRUD, satellite listing, health checks |
| **gRPC** | `:8444` | Bidirectional streaming with Satellites |
| **WebSocket** | `:8443` | Real-time terminal streaming to Cockpit |
| **WebTransport** | `:8443` | Low-latency terminal streaming (HTTP/3, secondary) |

```mermaid
graph LR
    subgraph "Nexus Server"
        direction TB
        HTTP["HTTP Server<br/>(mTLS)"]
        GRPC["gRPC Server<br/>(mTLS)"]
        WS["WebSocket<br/>(gorilla/websocket)"]
        HTTP --> AM["Auth Middleware"]
        AM --> HA["handleAPI()"]
        HA --> HS["handleListSessions"]
        HA --> HC["handleCreateSession"]
        HA --> HG["handleGetSession"]
        HA --> HAT["handleAttachSession"]
        HA --> HD["handleDetachSession"]
        HA --> HSU["handleSuspendSession"]
        HA --> HR["handleResumeSession"]
        HA --> HK["handleKillSession"]
        HA --> HSA["handleListSatellites"]
        HA --> HSS["handleSessionStream (WS)"]

        GRPC --> Connect["Connect() bidi stream"]

        WS --> WTH["WebSocket Handler"]
    end
```

**Key Internal Structs:**

| Struct | Responsibility |
|---|---|
| `NexusServer` | Main server; embeds HTTP, gRPC, WebTransport servers |
| `StreamRegistry` | Maps sessionID → gRPC channel for message routing |
| `JWTTokenValidator` | Validates Cockpit user JWT tokens |
| `SatelliteCertValidator` | Validates Satellite mTLS client certificates |
| `RateLimiter` | Token-bucket rate limiting per satellite and per user |

---

### Satellite Daemon

The Satellite daemon runs on the remote execution machine. It manages PTY sessions, maintains a persistent gRPC connection to Nexus, and handles process lifecycle.

```mermaid
graph TB
    subgraph "Satellite Daemon"
        D["Daemon"]
        D --> CS["CreateSession()"]
        D --> AS["AttachSession()"]
        D --> TS["TransitionSessionState()"]

        CS --> SP["spawnProcessWithPTY()"]
        SP --> PTY["PTY (unix/conpty)"]
        PTY --> Agent["Agent Process"]

        D --> GL["runGrpcLoop()"]
        GL --> CN["connectToNexus()"]
        CN --> RM["receiveMessages()"]
        RM --> HNM["handleNexusMessage()"]
        HNM --> HTI["handleTerminalInput()"]
        HNM --> HRC["handleResizeCommand()"]
        HNM --> HSC["handleSuspendCommand()"]
        HNM --> HReC["handleResumeCommand()"]
        HNM --> HKC["handleKillCommand()"]

        D --> HL["heartbeatLoop()"]
        D --> FPO["forwardPtyOutput()"]
        FPO --> RB["Ring Buffer (5MB)"]
        FPO --> GL

        CS --> IPC["IPC Server"]
        CS --> DMS["Dead Man's Switch"]
    end
```

**Session Struct:**

```go
type Session struct {
    ID         string
    Pty        *os.File          // PTY file descriptor
    IPCServer  *ipc.Server       // JSON-RPC IPC endpoint
    RingBuffer *buffer.RingBuffer // ANSI-aware scrollback
    DMS        *lifecycle.DeadManSwitch
    Process    *os.Process
    State      session.SessionState
}
```

---

### Cockpit — Web Dashboard

The Cockpit frontend is a **React 19 + Vite + TypeScript** single-page application served by Nginx.

```mermaid
graph TB
    subgraph "Cockpit SPA"
        Main["main.tsx<br/>(BrowserRouter)"]

        Main --> AL["AppLayout"]
        AL --> Dash["Dashboard"]
        AL --> Sess["Sessions"]
        AL --> Sats["Satellites"]
        AL --> Agents["Agents"]
        AL --> Set["Settings"]
        AL --> TV["TerminalView"]

        TV --> Term["Terminal Renderer"]
        Term --> VP["Virtual Viewport"]
        TV --> WS["WebSocket Client"]
        WT --> Roam["Roaming Client"]

        Main --> EB["ErrorBoundary"]
        Main --> TP["ToastProvider"]

        Sess --> NSM["NewSessionModal"]
        Sess --> API["API Client"]

        Set --> Push["Push Notification<br/>Manager"]
    end
```

**Pages:**

| Page | Route | Description |
|---|---|---|
| Dashboard | `/` | Overview with session stats, recent activity |
| Sessions | `/sessions` | Session list with state management actions |
| Satellites | `/satellites` | Registered satellite machines |
| Agents | `/agents` | AI agent configurations |
| Settings | `/settings` | User preferences, push notification config |
| TerminalView | `/session/:sessionId` | Live terminal with WebSocket streaming |

---

## Data Flow

### Session Create → Terminal Stream

```mermaid
sequenceDiagram
    participant User as Cockpit User
    participant CK as Cockpit
    participant NX as Nexus
    participant PG as PostgreSQL
    participant SAT as Satellite

    User->>CK: Click "New Session"
    CK->>NX: POST /api/v1/sessions
    NX->>PG: INSERT session (PROVISIONING)
    NX->>SAT: gRPC → RegisterRequest

    SAT->>SAT: spawnProcessWithPTY()
    SAT->>SAT: Start Ring Buffer + DMS + IPC
    SAT->>NX: gRPC ← SessionStateUpdate(RUNNING)
    NX->>PG: UPDATE session → RUNNING

    loop Terminal Stream
        SAT->>SAT: PTY output → Ring Buffer
        SAT->>NX: gRPC ← TerminalData
        NX->>NX: StreamRegistry.SendToSession()
        NX->>CK: WebSocket → terminal bytes
        CK->>CK: Terminal Renderer
    end

    User->>CK: Type input
    CK->>NX: WebSocket → input bytes
    NX->>NX: StreamRegistry → gRPC channel
    NX->>SAT: gRPC → TerminalInput
    SAT->>SAT: PTY.Write(input)
```

### Detach → DMS → Suspend → Resume

```mermaid
sequenceDiagram
    participant User
    participant CK as Cockpit
    participant NX as Nexus
    participant SAT as Satellite
    participant DMS as Dead Man's Switch

    User->>CK: Close browser tab
    CK--xNX: WebTransport disconnects
    NX->>NX: Detect client loss
    NX->>SAT: gRPC → (no more input)
    SAT->>SAT: TransitionState → DETACHED
    SAT->>NX: gRPC ← StateUpdate(DETACHED)

    SAT->>DMS: Start idle monitoring
    DMS->>DMS: Wait TTL (configurable, default 60min)

    alt User returns within TTL
        User->>CK: Open session
        CK->>NX: POST /sessions/{id}/attach
        NX->>SAT: gRPC → ResumeCommand
        SAT->>SAT: TransitionState → RE_ATTACHING → RUNNING
        DMS->>DMS: RecordActivity() → reset timer
    else TTL expires
        DMS->>SAT: TriggerSuspend()
        SAT->>SAT: SIGTSTP → process suspended
        SAT->>NX: gRPC ← StateUpdate(SUSPENDED)
        NX->>User: Push Notification: "Session suspended"
    end

    Note over User,SAT: Later...
    User->>CK: Click "Resume"
    CK->>NX: POST /sessions/{id}/resume
    NX->>SAT: gRPC → ResumeCommand
    SAT->>SAT: SIGCONT → process resumed
    SAT->>SAT: TransitionState → RUNNING
    SAT->>NX: gRPC ← StateUpdate(RUNNING)
    SAT->>CK: Ring Buffer snapshot (hydration)
```

---

## Session State Machine

Sessions follow a strict finite state machine with validated transitions:

```mermaid
stateDiagram-v2
    [*] --> PROVISIONING

    PROVISIONING --> RUNNING: PTY acquired, process started
    PROVISIONING --> TERMINATED: Provisioning failed

    RUNNING --> DETACHED: All clients disconnect
    RUNNING --> SUSPENDED: Manual suspend
    RUNNING --> TERMINATED: Process exit / kill

    DETACHED --> RE_ATTACHING: Client reconnects
    DETACHED --> SUSPENDED: DMS TTL expires
    DETACHED --> TERMINATED: Admin kill

    RE_ATTACHING --> RUNNING: Buffer hydrated, stream active
    RE_ATTACHING --> DETACHED: Re-attach failed
    RE_ATTACHING --> TERMINATED: Process died

    SUSPENDED --> RUNNING: Resume (SIGCONT)
    SUSPENDED --> TERMINATED: Admin kill

    TERMINATED --> [*]
```

**Transition Validation:**

```go
var ValidTransitions = map[SessionState][]SessionState{
    StateProvisioning: {StateRunning, StateTerminated},
    StateRunning:      {StateDetached, StateSuspended, StateTerminated},
    StateDetached:     {StateReAttaching, StateSuspended, StateTerminated},
    StateReAttaching:  {StateRunning, StateDetached, StateTerminated},
    StateSuspended:    {StateRunning, StateTerminated},
    StateTerminated:   {}, // Terminal — no transitions out
}
```

---

## gRPC Protocol

The `SatelliteGateway` service uses a single **bidirectional streaming RPC**. The satellite initiates the outbound-only connection (Zero-Trust — no inbound ports required on the satellite machine).

```protobuf
service SatelliteGateway {
  rpc Connect(stream SatelliteMessage) returns (stream NexusMessage);
}
```

### Satellite → Nexus Message Types

| Message | Channel | Purpose |
|---|---|---|
| `RegisterRequest` | Priority | Satellite identity (ID, fingerprint, version, OS, arch) |
| `HeartbeatPing` | Priority | Keep-alive with timestamps |
| `SessionStateUpdate` | Priority | Session state machine transitions |
| `TelemetryReport` | Priority | CPU/MEM/DISK/GPU metrics |
| `TerminalData` | Bulk | PTY output bytes with sequence numbers |
| `BufferReplay` | Bulk | Ring buffer snapshot on reconnect (session hydration) |
| `IpcEvent` | Bulk | IPC events between processes |
| `AgentEvent` | Agent (isolated) | Pi RPC events: agent_start, message_update, tool_execution_start/end, agent_end |
| `ContextFileUpdate` | Bulk | fsnotify-triggered context file sync from satellite filesystem |

### Nexus → Satellite Message Types

| Message | Purpose |
|---|---|
| `TerminalInput` | User keystrokes from Cockpit |
| `ResizeCommand` | Terminal dimension changes |
| `SuspendCommand` | Suspend a session |
| `ResumeCommand` | Resume a suspended session |
| `KillCommand` | Terminate a session |
| `StartSessionCommand` | Create and start a new PTY session |
| `UpdateAvailable` | Notify satellite a new binary version is available |
| `SessionReconciliation` | List of active session IDs — satellite prunes orphans |
| `DeployAgentCommand` | Deploy a Pi RPC agent: agent definition + secrets |
| `ContextFilePush` | Push an updated context file from Cockpit to satellite disk |

### Three-Channel Priority Architecture

The satellite daemon uses three separate outbound channels drained by a single `streamWriter` goroutine. This prevents high-volume PTY output from starving control messages, and isolates agent event bursts from the terminal stream:

```
sendPriorityCh  (64 slots)  — heartbeats, state updates, telemetry  → never dropped
agentEventCh    (512 slots) — Pi RPC agent events                   → dropped gracefully under pressure
sendCh          (256 slots) — PTY output, buffer replays            → bulk
```

`streamWriter` drains `sendPriorityCh` first, then round-robins between `agentEventCh` and `sendCh`. When `agentEventCh` fills (agent flooding events faster than gRPC can send), events are dropped — acceptable because the DB is the source of truth and a dropped live event only means slightly stale display, not data loss.



---

## Pi RPC Bridge

The satellite daemon can spawn Pi processes in RPC mode and bridge their JSON stdin/stdout event stream to Nexus.

```
Nexus ──DeployAgentCommand──▶ daemon.go
                                 │
                                 ▼
                          PiBridge.Start()
                                 │
                    ┌────────────┴────────────┐
                    │   Pi process (Node.js)  │
                    │   stdin ◀── JSON cmds   │
                    │   stdout ──▶ JSON events│
                    └────────────┬────────────┘
                                 │
                          bridge.Events()
                                 │
                                 ▼
                    sendToNexus(AgentEvent) ──▶ Nexus gateway
```

**Key files:**
- `internal/satellite/pi_bridge.go` — PiBridge struct, Start(), Events(), Stop()
- `internal/satellite/runtime.go` — runtime config (enabled/disabled per satellite)
- `cmd/daao/daemon.go` — handleDeployAgentCommand(), bridges map, runContextWatcher()
- `extensions/` — DAAO Pi extension pack (TypeScript): daao-telemetry, daao-guardrails, daao-hitl-gate, daao-output-router, daao-context-loader, daao-sandbox

**Active bridges** are tracked in `Daemon.bridges map[string]*satellite.PiBridge`, keyed by session ID. On daemon shutdown, all bridges are stopped before the gRPC connection closes.

---

## Context System

Each satellite maintains a context directory (`/etc/daao/context/` on Linux/macOS, `C:\ProgramData\daao\context\` on Windows) containing markdown files that give AI agents situational awareness about the host.

**Standard files seeded on first start:**

| File | Purpose |
|------|---------|
| `systeminfo.md` | Role, services, hardware, network |
| `runbooks.md` | SOPs and operational procedures |
| `alerts.md` | Known alert conditions + resolution steps |
| `topology.md` | Network relationships and dependencies |
| `secrets-policy.md` | Credential references (no actual values) |
| `history.md` | Recent changes, deployments, incidents |
| `monitoring.md` | Metrics, thresholds, dashboards |
| `dependencies.md` | Upstream/downstream service dependencies |

**Bidirectional sync:**

```
Satellite filesystem  ←──fsnotify──▶  context_watcher.go
                                           │
                              gRPC ContextFileUpdate
                                           │
                                    Nexus gateway
                                           │
                               DB upsert (ON CONFLICT)
                                           │
                          ◀── gRPC ContextFilePush ──  Cockpit PUT /context/:id
```

- **Satellite → Nexus:** `fsnotify` with 300ms debounce (collapses Windows write+rename events). On reconnect, all current files are sent as a bulk sync.
- **Nexus → Satellite:** `ContextFilePush` dispatched from `ContextHandler.handleUpdateContextFile()` via `StreamRegistry.SendToSatellite()` after every DB write.
- **Path traversal protection:** `handleContextFilePush` validates that the file path is a plain filename (no `../` etc.) before writing to disk.

**Key files:**
- `internal/satellite/context_watcher.go` — ContextWatcher, SeedStandardFiles, ReadAllContextFiles
- `internal/api/context.go` — REST CRUD handlers + ContextFilePush dispatch
- `cockpit/src/components/ContextEditor.tsx` — editor with standard file quick-picks + version history

---

## Agent Event Streaming

Real-time streaming of Pi RPC agent events from satellite to Cockpit browser.

**Flow:**
```
Pi events ──agentEventCh──▶ gateway ──▶ {async DB write + in-memory pub/sub}
                                                        │
                                          GET /api/v1/runs/:id/stream (SSE)
                                                        │
                                              Cockpit AgentRunView
```

Agent event writes are batched at 100ms intervals using `database.BatchEventWriter` to reduce database round-trips under high event throughput.

The SSE endpoint replays full event history from `agent_run_events` on subscribe (late joiners, tab reload, reconnects all work), then streams live events as they arrive.

**SSE Authentication:** `EventSource` cannot send `Authorization` headers. Authentication uses an `HttpOnly; Secure; SameSite=Strict` cookie (`daao_auth`) set by `POST /api/v1/auth/cookie` after OIDC token exchange. The auth middleware falls back to this cookie when no `Authorization` header is present.

---

## Package Architecture

### `pkg/` — Reusable Libraries

```mermaid
graph TB
    subgraph "pkg/"
        buffer["buffer<br/>ANSI-aware ring buffer<br/>5MB capacity"]
        ipc["ipc<br/>JSON-RPC server<br/>Unix sockets / Named Pipes"]
        lifecycle["lifecycle<br/>Dead Man's Switch<br/>idle monitoring + auto-suspend"]
        proc["proc<br/>Process detach/suspend<br/>Unix: SIGTSTP/SIGCONT<br/>Windows: Job Objects"]
        pty["pty<br/>Cross-platform PTY<br/>Unix: /dev/ptmx<br/>Windows: ConPTY"]
        telemetry["telemetry<br/>Kernel wait-state<br/>Linux: /proc<br/>macOS: proc_info<br/>Windows: NtQueryInformation"]
    end

    lifecycle --> proc
    lifecycle --> buffer
```

### `internal/` — Server-Specific Code

```mermaid
graph TB
    subgraph "internal/"
        auth["auth<br/>JWT + mTLS<br/>OAuth2 Device Flow (planned)"]
        database["database<br/>pgxpool connection"]
        router["router<br/>Stream multiplexing<br/>WebSocket ↔ gRPC"]
        satellite["satellite<br/>Registration, Ed25519<br/>mTLS certificates"]
        session["session<br/>State machine + Store<br/>PostgreSQL CRUD"]
    end

    router --> session
    satellite --> auth
```

---

## Deployment Architecture

```mermaid
graph TB
    subgraph "Docker Compose Stack"
        direction TB
        PG["postgres:alpine<br/>:5432<br/>Volume: postgres_data"]
        NX["nexus<br/>:8443 (HTTPS)<br/>:8444 (gRPC)<br/>Multi-stage Alpine build"]
        CK["cockpit<br/>:8081 (HTTP → 80)<br/>Nginx + static assets"]
    end

    NX -->|"depends_on: healthy"| PG
    CK -->|"depends_on: healthy"| NX

    subgraph "External"
        SAT1["Satellite 1<br/>(any machine)"]
        SAT2["Satellite 2<br/>(any machine)"]
        Browser["Web Browser"]
    end

    SAT1 -->|"gRPC :8444"| NX
    SAT2 -->|"gRPC :8444"| NX
    Browser -->|"HTTP :8081"| CK
    CK -.->|"proxy /api/*"| NX
```

---

## Technology Stack

| Layer | Technology | Version |
|---|---|---|
| **Language** | Go | 1.26 |
| **Frontend** | React + TypeScript | 19 |
| **Build Tool** | Vite | Latest |
| **Database** | PostgreSQL + TimescaleDB (optional) | Latest stable |

When `TIMESCALEDB_ENABLED=true`, the `satellite_telemetry` table is converted to a TimescaleDB hypertable for efficient time-series queries at scale.

| **Migrations** | golang-migrate | v4 |
| **RPC** | gRPC + Protobuf | 1.78 |
| **Transport** | WebSocket (primary) + WebTransport (HTTP/3) | gorilla/websocket 1.5, quic-go |
| **Auth** | JWT + mTLS (OAuth2 Device Flow planned) | |
| **Crypto** | Ed25519 keys, x509 certificates | |
| **Container** | Docker + Docker Compose | |
| **Reverse Proxy** | Nginx | Alpine |

| **Testing** | testcontainers-go (Postgres) | v0.35 |

---

## HA & Scaling (Enterprise)

> Requires enterprise license with `FeatureHA`. See [SCALING.md](./SCALING.md) for detailed limits and connection math.

Community DAAO runs a single Nexus instance. Enterprise enables multi-instance clustering:

```mermaid
graph TB
    subgraph "Load Balancer"
        LB["HAProxy<br/>gRPC + HTTPS"]
    end

    subgraph "Nexus Cluster"
        N1["Nexus-1"]
        N2["Nexus-2"]
        N3["Nexus-3"]
    end

    subgraph "Messaging"
        NATS["NATS JetStream"]
    end

    subgraph "Data"
        PG["PostgreSQL"]
        S3["S3/MinIO<br/>(recordings)"]
    end

    SAT["Satellites"] --> LB
    CK["Cockpit"] --> LB
    LB --> N1 & N2 & N3
    N1 & N2 & N3 <--> NATS
    N1 & N2 & N3 --> PG
    N1 & N2 & N3 --> S3
```

### Read Replica Support

For improved read throughput at scale, Nexus supports an optional read replica PostgreSQL instance. When `DATABASE_READ_URL` is set, list handlers route read queries to the replica while writes continue to the primary database.

### Interface-Based Injection

All stateful components implement interfaces. Enterprise implementations are injected at startup based on license:

| Interface | Community | Enterprise |
|---|---|---|
| `StreamRegistryInterface` | In-memory Go channels | NATSStreamRegistry (`internal/enterprise/ha/`) |
| `AgentRunHubInterface` | In-memory subscriber map | NATSRunEventHub (`internal/enterprise/ha/`) |
| `RecordingPoolInterface` | Local filesystem | S3RecordingPool (`internal/enterprise/ha/`) |
| Rate limiter interface | In-memory token buckets | RedisRateLimiter (`internal/enterprise/ha/`) |
| Scheduler | In-memory cron (all instances) | LeaderSchedulerGuard + PG advisory lock (`internal/enterprise/ha/`) |

Phase 2 complete. Set `S3_ENDPOINT`/`S3_BUCKET` for S3 recordings, `REDIS_URL` for distributed rate limiting (both require enterprise license with `FeatureHA`).

Code lives in `internal/enterprise/ha/`. No API changes required.

