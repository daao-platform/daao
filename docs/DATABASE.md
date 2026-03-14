# Database Schema

> PostgreSQL schema, migrations, state machine, and event logging specification.

---

## Overview

DAAO uses **PostgreSQL 18** with `golang-migrate/v4` for schema management. The database stores session state, satellite registrations, and an event audit log.

```mermaid
erDiagram
    users ||--o{ sessions : "owns"
    users ||--o{ satellites : "owns"
    satellites ||--o{ sessions : "has"
    sessions ||--o{ event_logs : "generates"

    users {
        uuid id PK
        varchar email
        varchar name
        varchar role
        varchar provider
        varchar provider_id
        text avatar_url
        timestamp last_login_at
        timestamp created_at
        timestamp updated_at
    }

    satellites {
        uuid id PK
        varchar name
        uuid owner_id
        timestamp created_at
        timestamp updated_at
        varchar status
    }

    sessions {
        uuid id PK
        uuid satellite_id FK
        uuid user_id
        varchar state
        text name
        text agent_binary
        text_array agent_args
        integer os_pid
        text pts_name
        smallint cols
        smallint rows
        timestamp last_activity_at
        timestamp started_at
        timestamp detached_at
        timestamp suspended_at
        timestamp terminated_at
        timestamp created_at
        timestamp updated_at
        timestamp last_heartbeat
        jsonb connection_info
    }

    event_logs {
        bigserial id PK
        uuid satellite_id
        uuid session_id
        varchar event_type
        jsonb event_data
        timestamp created_at
    }
```

---

## Tables

### `users`

Platform users with role-based access control.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PRIMARY KEY | Unique user identifier |
| `email` | `VARCHAR(255)` | UNIQUE, NOT NULL | User email address |
| `name` | `VARCHAR(255)` | NOT NULL | Display name |
| `role` | `VARCHAR(50)` | NOT NULL, DEFAULT `'viewer'`, CHECK | Role: `owner`, `admin`, or `viewer` |
| `provider` | `VARCHAR(50)` | nullable | Auth provider (e.g., `oidc`) |
| `provider_id` | `VARCHAR(255)` | nullable | External provider subject ID |
| `avatar_url` | `TEXT` | nullable | Profile picture URL |
| `last_login_at` | `TIMESTAMPTZ` | nullable | Last login timestamp |
| `created_at` | `TIMESTAMPTZ` | DEFAULT `NOW()` | Registration timestamp |
| `updated_at` | `TIMESTAMPTZ` | DEFAULT `NOW()` | Last modification |

**Constraints:**
```sql
CHECK (role IN ('owner', 'admin', 'viewer'))
```

**Indexes:**
- `idx_users_provider_id` → `(provider, provider_id)` UNIQUE WHERE provider IS NOT NULL

---

### `satellites`

Registered remote machines that execute agent processes.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PRIMARY KEY | Unique satellite identifier |
| `name` | `VARCHAR(255)` | NOT NULL | Human-readable name |
| `owner_id` | `UUID` | NOT NULL | Owning user ID |
| `created_at` | `TIMESTAMPTZ` | DEFAULT `NOW()` | Registration timestamp |
| `updated_at` | `TIMESTAMPTZ` | DEFAULT `NOW()` | Last modification |
| `status` | `VARCHAR(50)` | NOT NULL, DEFAULT `'active'` | Current status |

**Indexes:**
- `idx_satellites_owner_id` → `owner_id`
- `idx_satellites_status` → `status`

---

### `sessions`

Agent execution sessions with full lifecycle tracking.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PRIMARY KEY | Unique session identifier |
| `satellite_id` | `UUID` | FK → satellites, NOT NULL | Host satellite |
| `user_id` | `UUID` | NOT NULL | Owning user |
| `state` | `VARCHAR(50)` | NOT NULL, CHECK constraint | Current state machine state |
| `name` | `TEXT` | NOT NULL, DEFAULT `'default'` | Human-readable name |
| `agent_binary` | `TEXT` | NOT NULL, DEFAULT `''` | Agent executable path |
| `agent_args` | `TEXT[]` | NOT NULL, DEFAULT `'{}'` | Command-line arguments |
| `os_pid` | `INTEGER` | nullable | OS process ID (set by satellite) |
| `pts_name` | `TEXT` | nullable | PTY device name |
| `cols` | `SMALLINT` | NOT NULL, DEFAULT `80` | Terminal columns |
| `rows` | `SMALLINT` | NOT NULL, DEFAULT `24` | Terminal rows |
| `last_activity_at` | `TIMESTAMPTZ` | NOT NULL, DEFAULT `NOW()` | Last user interaction |
| `started_at` | `TIMESTAMPTZ` | nullable | When process started |
| `detached_at` | `TIMESTAMPTZ` | nullable | When session was detached |
| `suspended_at` | `TIMESTAMPTZ` | nullable | When process was suspended |
| `terminated_at` | `TIMESTAMPTZ` | nullable | When process terminated |
| `created_at` | `TIMESTAMPTZ` | DEFAULT `NOW()` | Session creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | DEFAULT `NOW()` | Last modification |
| `last_heartbeat` | `TIMESTAMPTZ` | nullable | Last satellite heartbeat |
| `connection_info` | `JSONB` | nullable | Connection metadata |

**State CHECK Constraint:**
```sql
CHECK (state IN ('PROVISIONING', 'RUNNING', 'DETACHED', 'RE_ATTACHING', 'SUSPENDED', 'TERMINATED'))
```

**Indexes:**
- `idx_sessions_satellite_id` → `satellite_id`
- `idx_sessions_user_id` → `user_id`
- `idx_sessions_state` → `state` 
- `idx_sessions_created_at` → `created_at`
- `idx_sessions_active` → `state WHERE terminated_at IS NULL` (partial index)

---

### `event_logs`

Audit trail for all session events. **Range-partitioned** by `created_at` for efficient historical queries.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `BIGSERIAL` | auto-increment | Event identifier |
| `satellite_id` | `UUID` | NOT NULL | Source satellite |
| `session_id` | `UUID` | nullable | Associated session (if any) |
| `event_type` | `VARCHAR(100)` | NOT NULL | Event category |
| `event_data` | `JSONB` | nullable | Event payload |
| `created_at` | `TIMESTAMPTZ` | DEFAULT `NOW()` | Event timestamp |

**Partitioning:**
```sql
CREATE TABLE event_logs (...) PARTITION BY RANGE (created_at);
CREATE TABLE event_logs_default PARTITION OF event_logs DEFAULT;
```

**Event Types:**

| Event Type | Description |
|---|---|
| `STATE_CHANGE` | Session state transition (payload: `{from, to}`) |
| `RESIZE` | Terminal dimensions changed |
| `CLIENT_ATTACH` | Client connected to session |
| `CLIENT_DETACH` | Client disconnected from session |
| `HEARTBEAT_LOSS` | Satellite heartbeat lost |
| `HEARTBEAT_RESTORE` | Satellite heartbeat restored |
| `DMS_TRIGGERED` | Dead Man's Switch triggered suspension |
| `DMS_RESUMED` | Process resumed from DMS suspension |
| `PROCESS_EXIT` | Agent process exited |
| `ERROR` | Error occurred |

**Indexes:**
- `idx_event_logs_satellite_id` → `satellite_id`
- `idx_event_logs_session_id` → `session_id`
- `idx_event_logs_event_type` → `event_type`
- `idx_event_logs_created_at` → `created_at`

---

## Additional Tables

### `agent_definitions`

Agent catalog entries — pre-built and custom agent configurations.

| Column | Type | Description |
|---|---|---|
| `id` | `UUID` PK | Unique agent identifier |
| `name` | `TEXT` | Display name |
| `description` | `TEXT` | Human-readable description |
| `agent_type` | `TEXT` | `pi_rpc` or `pty` |
| `provider` | `TEXT` | LLM provider (e.g., `anthropic`) |
| `model` | `TEXT` | Model ID (e.g., `claude-opus-4-5`) |
| `system_prompt` | `TEXT` | Agent system prompt |
| `tools` | `JSONB` | Tool allow/deny configuration |
| `extensions` | `TEXT[]` | Pi extension IDs to load |
| `timeout_seconds` | `INTEGER` | Max run duration |
| `is_builtin` | `BOOLEAN` | Whether this is a pre-built agent |
| `created_by` | `UUID` | Creating user ID |
| `created_at` / `updated_at` | `TIMESTAMPTZ` | Timestamps |

---

### `agent_runs`

Individual agent execution records — run-level summary.

| Column | Type | Description |
|---|---|---|
| `id` | `UUID` PK | Unique run identifier |
| `agent_id` | `UUID` FK → agent_definitions | Which agent was deployed |
| `satellite_id` | `UUID` FK → satellites | Where it ran |
| `session_id` | `UUID` FK → sessions | Associated PTY session (optional) |
| `status` | `TEXT` | `running`, `completed`, `failed`, `timeout`, `killed` |
| `started_at` | `TIMESTAMPTZ` | Run start time |
| `ended_at` | `TIMESTAMPTZ` | Run end time (null if in progress) |
| `total_tokens` | `INTEGER` | Total LLM tokens consumed |
| `estimated_cost` | `DECIMAL` | Estimated API cost |
| `tool_call_count` | `INTEGER` | Number of tool calls made |
| `result` | `TEXT` | Final agent output |
| `error` | `TEXT` | Error message if failed |
| `metadata` | `JSONB` | Additional run metadata |

---

### `agent_run_events`

Individual Pi RPC events within a run. Powers live streaming and history replay.

| Column | Type | Description |
|---|---|---|
| `id` | `UUID` PK | Unique event identifier |
| `run_id` | `UUID` FK → agent_runs ON DELETE CASCADE | Parent run |
| `event_type` | `TEXT` | `agent_start`, `turn_start`, `message_update`, `tool_execution_start`, `tool_execution_end`, `turn_end`, `agent_end` |
| `payload` | `JSONB` | Event-specific data |
| `sequence` | `INTEGER` | Ordering within run |
| `created_at` | `TIMESTAMPTZ` | Event timestamp |

Index: `(run_id, sequence)` for fast ordered replay.

Writes to this table are batched at 100ms intervals via `database.BatchEventWriter` for improved throughput under high event volume.

---

### `satellite_telemetry_hourly` (Continuous Aggregate View)

TimescaleDB continuous aggregate view for efficient hourly telemetry queries. Automatically materializes aggregations from the `satellite_telemetry` hypertable.

| Column | Type | Description |
|---|---|---|
| `satellite_id` | `UUID` | Satellite identifier |
| `bucket` | `TIMESTAMPTZ` | Hour-aligned time bucket |
| `avg_cpu_percent` | `DOUBLE PRECISION` | Average CPU usage |
| `avg_memory_percent` | `DOUBLE PRECISION` | Average memory usage |
| `max_cpu_percent` | `DOUBLE PRECISION` | Peak CPU usage |
| `max_memory_percent` | `DOUBLE PRECISION` | Peak memory usage |
| `sample_count` | `INTEGER` | Number of samples in bucket |

Created via `migration 032` when `TIMESCALEDB_ENABLED=true`. This view enables fast dashboard queries without scanning raw telemetry rows.

---

### `satellite_context_files`

Context markdown files synced between satellite filesystem and Nexus.

| Column | Type | Description |
|---|---|---|
| `id` | `UUID` PK | Unique file identifier |
| `satellite_id` | `UUID` FK → satellites | Host satellite |
| `file_path` | `TEXT` | Filename (e.g., `systeminfo.md`) |
| `content` | `TEXT` | Full file content |
| `version` | `INTEGER` | Monotonic version counter |
| `last_modified_by` | `TEXT` | Attribution: `user@cockpit` or `local:<satellite_id>` |
| `created_at` / `updated_at` | `TIMESTAMPTZ` | Timestamps |

Unique constraint: `(satellite_id, file_path)` — one record per file per satellite.

---

### `context_file_history`

Full version history of context file edits with diffs.

| Column | Type | Description |
|---|---|---|
| `id` | `UUID` PK | History entry identifier |
| `context_file_id` | `UUID` FK → satellite_context_files | Parent file |
| `version` | `INTEGER` | Version number |
| `content` | `TEXT` | Full content at this version |
| `modified_by` | `TEXT` | Attribution |
| `modified_at` | `TIMESTAMPTZ` | Edit timestamp |

---

### `notifications`

Persistent notification records for the in-app bell/panel and SSE push.

| Column | Type | Description |
|---|---|---|
| `id` | `UUID` PK | Notification identifier |
| `user_id` | `UUID` FK → users | Target user |
| `event_type` | `TEXT` | `SESSION_TERMINATED`, `SESSION_SUSPENDED`, `SESSION_ERROR`, `SATELLITE_OFFLINE` |
| `title` | `TEXT` | Short notification title |
| `message` | `TEXT` | Full notification message |
| `priority` | `TEXT` | `LOW`, `MEDIUM`, `HIGH`, `CRITICAL` |
| `read` | `BOOLEAN` | Whether user has seen it |
| `satellite_id` / `session_id` | `UUID` | Related entities |
| `created_at` | `TIMESTAMPTZ` | When generated |

---

### `encrypted_secrets`

Server-side encrypted storage for API provider keys (Anthropic, OpenAI, etc.).

| Column | Type | Description |
|---|---|---|
| `id` | `UUID` PK | Secret identifier |
| `provider` | `TEXT` | Provider name (e.g., `anthropic`) |
| `encrypted_value` | `BYTEA` | AES-256-GCM encrypted key |
| `created_at` / `updated_at` | `TIMESTAMPTZ` | Timestamps |

Keys are encrypted at rest using `DAAO_SECRET_KEY` env var (falls back to random key in dev with warning).

---

### `secret_scopes`

Per-satellite secret access grants — which secrets a satellite can request from the Nexus broker.

| Column | Type | Description |
|---|---|---|
| `id` | `UUID` PK | Scope identifier |
| `satellite_id` | `UUID` FK → satellites | Satellite being granted access |
| `provider` | `TEXT` | Provider name |
| `backend` | `TEXT` | Backend type (`local`, `vault`, etc.) |
| `created_at` | `TIMESTAMPTZ` | Grant timestamp |

---

### `admin_audit_log`

Immutable audit trail of state-changing admin actions.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PRIMARY KEY, DEFAULT `gen_random_uuid()` | Unique entry identifier |
| `actor_id` | `UUID` | FK → `users(id)` ON DELETE SET NULL | User who performed the action |
| `actor_email` | `TEXT` | NOT NULL, DEFAULT `''` | Actor email at time of action |
| `action` | `TEXT` | NOT NULL | Action identifier (e.g. `session.create`, `agent.deploy`) |
| `resource_type` | `TEXT` | NOT NULL | Resource category (`session`, `agent`, `satellite`, `provider_config`, `user`) |
| `resource_id` | `TEXT` | nullable | Resource UUID or identifier |
| `details` | `JSONB` | nullable | Action-specific context (never contains secrets) |
| `ip_address` | `TEXT` | nullable | Client IP (from X-Forwarded-For, X-Real-IP, or RemoteAddr) |
| `created_at` | `TIMESTAMPTZ` | NOT NULL, DEFAULT `NOW()` | When the action occurred |

**Indexes:** `actor_id`, `action`, `resource_type`, `created_at`

**API:** `GET /api/v1/audit-log` (admin+ only, paginated with filters), `GET /api/v1/audit-log/export` (CSV export)

---

## Migrations

Migrations are stored in `db/migrations/` and executed by `db/migrate.go` using `golang-migrate/v4`.

| Migration | File | Description |
|---|---|---|
| **001** | `001_core_tables.up.sql` | Creates `satellites` and `sessions` tables with indexes |
| **002** | `002_event_logs.up.sql` | Creates range-partitioned `event_logs` table |
| **003** | `003_sessions_full_schema.up.sql` | Adds session metadata columns (name, agent, terminal, lifecycle timestamps) |
| **005** | `005_add_users_table.up.sql` | Creates `users` table (id, email, name, timestamps) |
| **006** | `006_satellite_fingerprint_index.up.sql` | Unique partial index on `satellites.fingerprint` |
| **007** | `007_recordings.up.sql` | Session recording metadata table |
| **008** | `008_notifications.up.sql` | Notifications table |
| **009** | `009_satellite_telemetry.up.sql` | Telemetry columns on satellites |
| **013–015** | `01x_agent_forge.up.sql` | Agent definitions, runs, available agents tables |
| **016** | `016_agent_definitions.up.sql` | Agent catalog with full definition schema |
| **017** | `017_agent_runs.up.sql` | Agent run records |
| **018** | `018_context_files.up.sql` | `satellite_context_files` and `context_file_history` |
| **019** | `019_secret_scopes.up.sql` | Secret access grants |
| **020** | `020_encrypted_secrets.up.sql` | Encrypted provider key storage |
| **021** | `021_context_files_cascade.up.sql` | ON DELETE CASCADE for context_file_history |
| **022** | `022_auth_rbac.up.sql` | Adds `role`, `provider`, `provider_id`, `last_login_at`, `avatar_url` to `users`. RBAC role CHECK constraint. |
| **023** | `023_agent_run_events.up.sql` | Individual Pi RPC event records for live streaming and history replay |
| **024** | `024_admin_audit_log.up.sql` | Admin audit trail table with indexes on actor_id, action, resource_type, created_at |

### Running Migrations

Migrations are automatically applied when Nexus starts. They can also be run manually:

```go
import "github.com/daao/nexus/db"

// Run all pending migrations
err := db.RunMigrations(databaseURL)
```

---

## Session Store Interface

The `session.Store` interface defines all database operations:

```go
type Store interface {
    // Session CRUD
    CreateSession(ctx context.Context, session *Session) error
    GetSession(ctx context.Context, id uuid.UUID) (*Session, error)
    UpdateSession(ctx context.Context, session *Session) error
    DeleteSession(ctx context.Context, id uuid.UUID) error

    // Queries
    ListSessionsBySatellite(ctx context.Context, satelliteID uuid.UUID) ([]*Session, error)
    ListSessionsByUser(ctx context.Context, userID uuid.UUID) ([]*Session, error)
    ListActiveSessions(ctx context.Context) ([]*Session, error)

    // State Machine
    TransitionSession(ctx context.Context, id uuid.UUID, newState SessionState) (*Session, error)

    // Event Logs
    WriteEventLog(ctx context.Context, event *EventLog) error
    GetEventLogs(ctx context.Context, sessionID uuid.UUID, limit int) ([]*EventLog, error)
}
```

The `TransitionSession` method enforces the state machine — invalid transitions return a `TransitionError`.

---

## Connection Pool

The database connection pool is managed by `pgx/v5`:

```go
// internal/database/pool.go
func NewPool(databaseURL string) (*pgxpool.Pool, error)
```

Configuration is via the `DATABASE_URL` environment variable:
```
postgres://daao:daao_password@postgres:5432/daao?sslmode=disable
```
