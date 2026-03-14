# DAAO Scaling & Concurrent Session Limits

This document describes the per-session resource footprint, practical limits, and tuning guidance for DAAO satellites and Nexus.

## Per-Session Resource Footprint

### Satellite (daemon)

| Resource | Per Session | Notes |
|---|---|---|
| PTY | 1 master + 1 slave FD | Linux: `/dev/ptmx`; Windows: ConPTY (conhost.exe instance) |
| Goroutines | 2 | `forwardPtyOutput` reader + process wait |
| Ring buffer | 64 KB (default) | Configurable; holds latest terminal output for reattach |
| IPC server | 1 named pipe | Session-specific IPC for local agent communication |
| Memory total | ~100–200 KB | PTY buffers + ring buffer + goroutine stacks |

### Nexus (server)

| Resource | Per Session | Notes |
|---|---|---|
| Ring buffer | 64 KB | Mirrors satellite ring buffer for WebSocket replay |
| gRPC handler | Shared per satellite | All sessions from one satellite share a single bidirectional stream |
| WebSocket viewer | 1 goroutine per viewer | Polls the ring buffer and forwards to the browser |
| DB row | 1 | `sessions` table; updated on state transitions |

## Practical Limits

| Scale | Expected Behavior |
|---|---|
| **1–8 sessions/satellite** | Comfortable. No tuning needed. |
| **8–20 sessions/satellite** | Works fine; monitor memory and FD usage. |
| **20–50 sessions/satellite** | Requires profiling. Watch for PTY FD exhaustion on Linux (`/proc/sys/kernel/pty/max`). On Windows, each ConPTY spawns a `conhost.exe` (~5 MB RSS each). |
| **50+ sessions/satellite** | Not recommended without load testing. Nexus gRPC stream may become a bottleneck (all sessions share one stream per satellite). |

## OS-Specific Limits

### Linux
- `/proc/sys/kernel/pty/max` — max PTY pairs system-wide (default: 4096)
- `/proc/sys/kernel/pty/nr` — currently allocated PTY pairs
- File descriptor limit: `ulimit -n` (default: 1024, raise to 65535 for heavy use)

### Windows (ConPTY)
- Each ConPTY session spawns a `conhost.exe` process (~5 MB RAM)
- No hard system limit, but 50+ concurrent `conhost.exe` instances will consume significant memory
- Windows Defender real-time scanning can slow PTY I/O — consider excluding the daemon directory

### macOS (darwin)
- PTY limit governed by `/dev/ptmx` — typically 256–512 pairs
- `sysctl kern.tty.ptmx_max` shows the limit

## Configurable Session Cap

The satellite daemon supports a per-satellite session limit via the `DAAO_MAX_SESSIONS` environment variable:

```bash
# Limit this satellite to 10 concurrent sessions
export DAAO_MAX_SESSIONS=10
```

| Value | Behavior |
|---|---|
| `0` (default) | No limit — sessions bounded only by OS resources |
| `N > 0` | Daemon rejects `StartSessionCommand` when `N` sessions are already active, returning an error to Nexus |

When the limit is reached, the Nexus UI shows the error: **"satellite session limit reached (max: N)"**.

## Tuning Recommendations

1. **Start with defaults** — for 1–8 sessions, no tuning is needed
2. **Monitor `conhost.exe` on Windows** — use Task Manager or `tasklist | findstr conhost` to track instances
3. **Increase FD limits on Linux** — if running 20+ sessions, set `ulimit -n 65535`
4. **Profile before scaling** — use `go tool pprof` on the daemon for memory profiling:
   ```bash
   # Add to daemon startup for profiling
   export DAAO_PPROF=1
   # Then: go tool pprof http://localhost:6060/debug/pprof/heap
   ```
5. **Consider multiple satellites** — for 50+ total sessions, distribute across multiple satellite machines

---

## Cluster Scaling (Enterprise)

> Requires enterprise license with `FeatureHA` enabled. Community edition runs a single Nexus instance.

### Architecture: Multi-Instance Nexus

Enterprise deployments use multiple Nexus instances behind a load balancer, with NATS JetStream for cross-instance communication.

```
  Satellites ──gRPC──► HAProxy ──► Nexus-1 ──┐
                                   Nexus-2 ──┼── NATS JetStream
  Cockpit ──HTTPS──►  HAProxy ──► Nexus-3 ──┘
                                     │
                                 PostgreSQL
```

### What Moves from In-Memory to Distributed

| Component | Community (single) | Enterprise (cluster) |
|---|---|---|
| StreamRegistry | Go channels (in-memory) | NATS pub/sub (`daao.session.<id>`) |
| RunEventHub | In-memory subscriber map | NATS pub/sub (`daao.run.<id>`) |
| SSEHub | Process-local | NATS cross-instance fan-out |
| RecordingPool | Local filesystem | S3/MinIO with pre-signed URLs |
| RateLimiter | In-memory token buckets | Redis `INCR + EXPIRE` |
| Scheduler | In-memory cron | PG advisory lock leader election |
| RingBufferPool | In-memory (stays local) | Local + NATS request-reply for cross-instance replay |

### Scale Targets

| Milestone | Satellites | Concurrent Sessions | DB Scaling Needed? |
|---|---|---|---|
| **Phase 1** | 100 | 500 | No — PG single-primary handles it |
| **Phase 2** | 500 | 2,000 | Yes — read replicas, PgBouncer. Complete. |
| **Phase 3** | 1,000+ | 5,000+ | ✓ TimescaleDB for telemetry, batched writes |

### Connection Math (100 satellites)

- Heartbeats: 100 × 1 per 15s = ~400/min (trivial for PG)
- Telemetry: 100 × 1 per 30s = ~200/min
- Terminal data: NATS pub/sub, never hits PG
- Agent events: ~100 writes/s peak (batched at 100ms)
- Connection pool: `pgxpool` default 4 × 3 Nexus = 12 connections (PG default max: 100)

### Enterprise Docker Compose

```bash
# Scale to 3 Nexus instances (includes PgBouncer for connection pooling)
docker compose -f docker-compose.yml -f docker-compose.enterprise.yml up --scale nexus=3
```

If NATS is unreachable at startup, Nexus logs a warning and falls back to in-memory mode — the cluster still operates single-instance.

If S3 is unreachable, Nexus falls back to local disk storage (warning logged). If Redis is unreachable, rate limiting fails open (requests allowed, warning logged). If scheduler leader election fails, the instance runs without scheduled sessions.

### Environment Variables

| Variable | Description |
|---|---|
| `NATS_URL` | NATS server URL (e.g. `nats://nats:4222`). Empty = community single-instance mode. Set automatically by docker-compose.enterprise.yml. |
| `S3_ENDPOINT` | S3/MinIO endpoint (e.g. `http://minio:9000`). Empty = local disk. Set automatically by docker-compose.enterprise.yml. |
| `S3_BUCKET` | S3 bucket name for recordings (e.g. `daao-recordings`). |
| `S3_ACCESS_KEY_ID` | S3 access key. |
| `S3_SECRET_ACCESS_KEY` | S3 secret key. |
| `S3_REGION` | S3 region (default `us-east-1`; MinIO accepts any value). |
| `REDIS_URL` | Redis URL (e.g. `redis://redis:6379`). Empty = in-memory rate limiting. Set automatically by docker-compose.enterprise.yml. |
| `DATABASE_URL` | PostgreSQL connection string. When using PgBouncer in transaction pooling mode, point to `pgbouncer:5432` (e.g. `postgresql://user:pass@pgbouncer:5432/daao`). Set automatically by docker-compose.enterprise.yml. |
| `TIMESCALEDB_ENABLED` | Set to `true` to activate TimescaleDB hypertable mode for telemetry data. When enabled, `satellite_telemetry` table is converted to a hypertable for better performance at scale. |
| `DATABASE_READ_URL` | Optional read replica PostgreSQL URL. When set, list handlers route read queries to the replica for improved read throughput. |

If TimescaleDB is not installed, migration 032 logs a warning and skips hypertable creation. Standard PostgreSQL performance applies.

### Infrastructure Requirements

| Component | Image | Idle RAM | Purpose |
|---|---|---|---|
| NATS JetStream | `nats:alpine` (~15MB image) | ~15MB | Pub/sub + message persistence |
| HAProxy | `haproxy:alpine` | ~5MB | L4/L7 load balancing (gRPC-aware) |
| PgBouncer | `pgbouncer:latest` | ~5MB | Connection multiplexing |
| S3/MinIO (optional) | `minio/minio` | ~100MB | Recording storage for cluster |
| Redis (optional) | `redis:alpine` | ~30MB | Distributed rate limiting + cache |
