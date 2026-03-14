# Development Guide

> Local setup, testing, code structure, package reference, and contributing guidelines.

---

## Prerequisites

| Tool | Version | Purpose |
|---|---|---|
| **Go** | 1.25+ | Backend development |
| **Node.js** | 22+ | Cockpit frontend |
| **Docker** | Latest | PostgreSQL and integration tests |
| **protoc** | 3.x | Protobuf compilation (optional) |
| **golangci-lint** | Latest | Go linting (optional) |

---

## Local Setup

### 1. Start PostgreSQL

```bash
# Start just the database
docker compose up -d postgres

# Verify it's healthy
docker compose ps postgres
```

### 2. Run Nexus (Backend)

```bash
# Set environment variables
export DATABASE_URL="postgres://daao:daao_password@localhost:5432/daao?sslmode=disable"
export JWT_SECRET="dev-secret"

# Run Nexus
go run ./cmd/nexus
```

Nexus starts on `:8443` (HTTPS) and `:8444` (gRPC).

### 3. Run Cockpit (Frontend)

```bash
cd cockpit
npm install
npm run dev
```

Cockpit dev server starts on `http://localhost:5173` with hot-reload.

### 4. Run Everything

```bash
# Using Make
make dev          # Starts Nexus + Cockpit dev servers
make dev-nexus    # Nexus only
make dev-cockpit  # Cockpit only
```

---

## Build

```bash
make build            # Build everything
make build-go         # Go binary only → bin/nexus
make build-cockpit    # Cockpit production bundle → cockpit/dist/
```

The Go binary is built with:
```bash
go build -o bin/nexus ./cmd/nexus
```

---

## Testing

### Go Tests

```bash
# Run all tests with race detector
make test-go
# OR
go test -race ./...
```

**Test infrastructure:**
- **testcontainers-go** for PostgreSQL integration tests
- Test files: `internal/session/store_test.go`, `pkg/buffer/ring_test.go`, `pkg/ipc/token_test.go`, `pkg/lifecycle/dms_test.go`, `pkg/pty/pty_unix_test.go`

### Cockpit Tests

```bash
make test-cockpit
# OR
cd cockpit && npm test
```

### Integration & E2E Tests

```
tests/
├── e2e/           # End-to-end tests
└── integration/   # Integration tests
```

---

## Linting

```bash
make lint             # Lint everything
make lint-go          # golangci-lint
make lint-cockpit     # ESLint
```

---

## Code Architecture

### Go Packages

#### `cmd/` — Entry Points

| Package | Files | Description |
|---|---|---|
| `cmd/daao` | `daemon.go`, `login.go`, `run.go` | CLI tool for satellite operations |
| `cmd/nexus` | `main.go` (1603 lines) | API Gateway with REST + gRPC + WebTransport |

#### `internal/` — Server-Specific

| Package | Key Types | Description |
|---|---|---|
| `auth` | `DeviceCode`, `OAuthProvider`, `GitHubProvider`, `GoogleProvider` | OAuth2 Device Code Flow with provider abstraction |
| `database` | `NewPool()` | pgxpool connection pool factory |
| `router` | `StreamRouter`, `SessionResolver`, `ControlMessage`, `OOBUIMessage` | WebTransport ↔ gRPC stream multiplexing |
| `satellite` | `KeyPair`, `SatelliteRegistration`, `NexusClient`, `SatelliteStore` | Ed25519 keys, mTLS certs, registration |
| `session` | `Store`, `Session`, `SessionState`, `EventLog`, `ValidTransitions` | State machine, PostgreSQL persistence |
| `tunnel` | `StreamHandler`, `SatelliteMessage`, `NexusMessage` | gRPC bidirectional stream types |

#### `pkg/` — Reusable Libraries

| Package | Key Types | Description |
|---|---|---|
| `buffer` | `RingBuffer` | 5MB ANSI-boundary-aware ring buffer for terminal scrollback |
| `ipc` | `Server`, `Conn`, `JSONRPCMessage` | JSON-RPC 2.0 over Unix sockets (POSIX) / Named Pipes (Windows) |
| `lifecycle` | `DeadManSwitch`, `DMSConfig`, `EventLogger` | Automatic process suspension after idle timeout |
| `proc` | `Detach()`, `Suspend()`, `Resume()` | Cross-platform process control (SIGTSTP/SIGCONT, Job Objects) |
| `pty` | `Pty` interface, Unix/Windows implementations | Cross-platform pseudo-terminal (Unix ptmx, Windows ConPTY) |
| `telemetry` | `Scraper`, `ProcessState`, `WaitReason`, `ProcessInfo` | Kernel wait-state detection (Linux `/proc`, macOS `proc_info`, Windows `NtQuery`) |

### Cockpit Architecture

| Directory | Purpose |
|---|---|
| `src/api/client.ts` | Typed REST API client with all CRUD operations |
| `src/components/` | Reusable UI: `AppLayout`, `ErrorBoundary`, `Icons`, `LoadingSpinner`, `NewSessionModal`, `Toast` |
| `src/pages/` | Route pages: `Dashboard`, `Sessions`, `Satellites`, `Agents`, `Settings`, `TerminalView` |
| `src/terminal/` | Terminal renderer (`index.tsx`) + virtual viewport (`viewport.ts`) |
| `src/transport/` | WebTransport client (`index.ts`) + roaming support (`roaming.ts`) |
| `src/oob/` | Out-of-Band UI components (progress, forms, confirmation dialogs) |
| `src/push/` | Web Push notification manager with VAPID support |
| `src/push.ts` | `PushNotificationManager` class — subscribe, handle DMS/INPUT_REQUIRED events |
| `src/hooks.ts` | Custom hooks: `useApi`, `useWebSocket`, `useLocalStorage` |
| `src/index.css` | Global styles (41KB) |

---

## Protobuf

Proto definitions are in `proto/satellite.proto`:

```bash
# Regenerate Go code from .proto
make proto
# OR
go generate ./proto/...
```

Generated files:
- `proto/satellite.pb.go` — Message types
- `proto/satellite_grpc.pb.go` — Service stubs

---

## Platform Support

### Process Management (`pkg/proc`)

| Feature | Unix (Linux/macOS) | Windows |
|---|---|---|
| **Detach** | `syscall.SysProcAttr{Setsid: true}` | `CREATE_NEW_PROCESS_GROUP` |
| **Suspend** | `SIGTSTP` (group signal) | Job Object + `SuspendThread` |
| **Resume** | `SIGCONT` (group signal) | `ResumeThread` |

### PTY (`pkg/pty`)

| Platform | Implementation |
|---|---|
| Unix (Linux/macOS) | `/dev/ptmx` via `posix_openpt` |
| Windows | ConPTY via `CreatePseudoConsole` |

### Telemetry (`pkg/telemetry`)

| Platform | Data Source |
|---|---|
| Linux | `/proc/{pid}/status`, `/proc/{pid}/wchan`, `/proc/{pid}/fd/` |
| macOS | `proc_pidinfo()`, `proc_bsdinfo()` |
| Windows | `NtQueryInformationProcess`, `NtQuerySystemInformation` |
| Fallback | Returns `StateUnknown` |

### IPC (`pkg/ipc`)

| Platform | Transport |
|---|---|
| Unix | Unix domain sockets (`/tmp/daao-sess-{uuid}.sock`) |
| Windows | Named Pipes (`\\.\pipe\daao-sess-{uuid}`) |

---

## Contributing

### Code Style

- **Go**: Follow standard Go conventions. Use `golangci-lint`.
- **TypeScript**: Follow ESLint config in `cockpit/`.
- **Commit messages**: Use conventional commits format.

### Adding a New API Endpoint

1. Add the handler method to `NexusServer` in `cmd/nexus/main.go`
2. Add the route in `handleAPI()`
3. Add corresponding types to `cockpit/src/api/client.ts`
4. Add tests

### Adding a New Session State

1. Add the state constant in `internal/session/store.go`
2. Update `AllSessionStates` and `ValidTransitions`
3. Update the database CHECK constraint via a new migration
4. Update `proto/satellite.proto` `SessionState` enum
5. Run `make proto` to regenerate Go code
6. Update `cockpit/src/api/client.ts` `SESSION_STATES`
