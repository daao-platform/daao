# Testing Strategy & Roadmap

## Current Test Coverage

| Package | Tests | What's Covered |
|---------|-------|----------------|
| `internal/session` | `store_test.go` | Exhaustive state machine (all 36 transitions), timestamp side-effects |
| `internal/grpc` | `gateway_test.go`, `gateway_routing_test.go` | Heartbeat batching, message routing (register, terminal data, heartbeat, unknown) |
| `internal/auth` | `jwt_test.go` | JWT validation (valid, expired, wrong issuer/secret, malformed, missing claims) |
| `internal/transport` | `websocket_integration_test.go` | WebSocket terminal stream, ring buffer flush, state polling, ping/pong, resize |
| `internal/api` | `handlers_test.go`, `pagination_test.go` | API handlers, cursor pagination |
| `internal/satellite` | `pi_bridge_test.go`, `bootstrap_test.go`, `context_parser_test.go`, `agent_bench_test.go` | Pi arg building, system prompt expansion, write path validation, bootstrap configs, Node.js URL generation, platform detection |
| `pkg/buffer` | `ring_test.go` | Ring buffer read/write |
| `pkg/pty` | `pty_test.go` | PTY read/write, close, deadline (Linux only) |
| `pkg/ipc` | `token_test.go` | IPC token handling |
| `pkg/lifecycle` | `dms_test.go` | Dead Man's Switch |
| `tests/e2e` | `e2e_test.go`, `terminal_pipeline_test.go` | End-to-end session lifecycle |
| `tests/integration` | `api_integration_test.go`, `satellite_test.go` | API + satellite integration with real DB |

## CI Tiers

- **🔴 Must-Pass Gate:** `go build ./...` + `go vet ./...`
- **🟡 Informational:** `go test -race ./...` + `npm run build && npm test`
- **🔵 Nightly:** Docker Compose, integration tests, smoke tests

## Runtime Provisioning Test Framework

Cross-platform provisioning verification across all satellite types.

**Script:** `scripts/test-provisioning.sh`

```bash
# All targets
./scripts/test-provisioning.sh

# Specific platform
./scripts/test-provisioning.sh --target docker     # Docker/Linux satellites
./scripts/test-provisioning.sh --target macos       # Mac Mini @ 192.168.20.165
./scripts/test-provisioning.sh --target windows     # Windows native

# Skip bootstrap download (offline mode)
./scripts/test-provisioning.sh --skip-bootstrap
```

### Test Phases

| Phase | What's Tested | Targets |
|-------|---------------|---------|
| 1. Unit Tests | Bootstrap config, URL generation, platform detection (12 tests) | Dev machine |
| 2. Build Verification | Cross-compile for linux/amd64, linux/arm64, darwin/arm64, windows/amd64 | Dev machine |
| 3. Docker/Linux | Container prereqs (curl/tar/xz), extensions, runtime dir, agent detection, download URL | sat-alpha/bravo/charlie |
| 4. macOS | SSH connectivity, daao binary, runtime dir, Node.js + Pi, extensions, download URL | Mac Mini (sat-delta/echo) |
| 5. Windows | daao binary, pi CLI, Node.js, extensions dir, daemon.env, service status | Local Windows |
| 6. Proto/API | ProvisionRuntimeCommand in proto, generated Go code, API available_agents field | Dev machine + Nexus |
| 7. Bootstrap Integration | Trigger lazy bootstrap on Docker satellite, verify Node.js + Pi installed | Docker satellite |

### Cross-Platform Verification Matrix

| Check | Linux (Docker) | macOS (Mac Mini) | Windows (Native) |
|-------|:-:|:-:|:-:|
| `resolveBinary("bash")` finds binary | ✅ `/bin/bash` | ✅ `/bin/bash` | ✅ `System32` |
| Extensions at `ExtensionsDir()` | ✅ `/opt/daao/extensions/` | ✅ `/opt/daao/extensions/` | ✅ `%LOCALAPPDATA%\daao\extensions\` |
| Bootstrap download URL | `.tar.xz` | `.tar.gz` | `.zip` |
| Node.js binary path | `/opt/daao/runtime/node/bin/node` | `/opt/daao/runtime/node/bin/node` | `%ProgramFiles%\daao\runtime\node\node.exe` |
| Pi binary path | `/opt/daao/runtime/node/bin/pi` | `/opt/daao/runtime/node/bin/pi` | `%ProgramFiles%\daao\runtime\node\pi.cmd` |
| `available_agents` in API | `["bash"]` → `["bash","node","pi"]` | `["bash","zsh"]` → `+node,pi` | `["cmd","powershell","bash"]` → `+node,pi` |
| `ProvisionRuntimeCommand` gRPC | Works | Works | Works |

## Next Steps (when API stabilizes)

- [ ] **REST handler tests** — Use `httptest.NewServer` + JSON assertions against each endpoint
- [ ] **Cockpit Vitest setup** — Add `vitest` to cockpit, test key React components
- [ ] **Rate limiter tests** — Verify `internal/auth/ratelimit.go` behavior under load
- [ ] **Cert validation tests** — Test mTLS cert validator with self-signed certs

## Later (when architecture settles)

- [ ] **E2E satellite flow** — Automated satellite registration → session create → terminal stream
- [ ] **Docker Compose integration** — Health check all services in CI (currently nightly)
- [ ] **Session recording tests** — When session recording is implemented
- [ ] **Multi-satellite tests** — Routing correctness with multiple satellites
- [x] **Runtime provisioning** — Cross-platform bootstrap verification (see above)

