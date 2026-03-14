# API Reference

> Complete reference for the DAAO REST API, gRPC service, WebTransport protocol, and WebSocket stream.

---

## Base URL

| Environment | URL |
|---|---|
| **Local Development** | `https://localhost:8443` |
| **Docker Compose** | `https://localhost:8443` (Nexus direct) |
| **Via Cockpit proxy** | `http://localhost:8081/api/v1/*` |

---

## Authentication

All API requests require authentication. DAAO supports two authentication mechanisms:

| Method | Used By | Header |
|---|---|---|
| **JWT Bearer Token** | Cockpit (users) | `Authorization: Bearer <token>` |
| **HttpOnly Cookie** | Cockpit (SSE endpoints) | `daao_auth` cookie (automatic) |
| **mTLS Client Certificate** | Satellite (machines) | TLS client cert in handshake |

---

## REST API

### Health Check

```http
GET /health
```

Returns server health status. No authentication required.

**Response:** `200 OK`
```json
{
  "status": "ok"
}
```

---

### Sessions

#### List Sessions

```http
GET /api/v1/sessions
```

Returns all active sessions for the authenticated user.

**Response:** `200 OK`
```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "satellite_id": "660e8400-e29b-41d4-a716-446655440001",
    "user_id": "770e8400-e29b-41d4-a716-446655440002",
    "name": "dev-session-1",
    "agent_binary": "/usr/local/bin/agent",
    "agent_args": ["--model", "gpt-4"],
    "state": "RUNNING",
    "os_pid": 12345,
    "pts_name": "/dev/pts/0",
    "cols": 120,
    "rows": 40,
    "last_activity_at": "2026-03-01T22:00:00Z",
    "started_at": "2026-03-01T20:00:00Z",
    "created_at": "2026-03-01T19:59:00Z"
  }
]
```

---

#### Create Session

```http
POST /api/v1/sessions
```

Creates a new session on the specified satellite.

**Request Body:**
```json
{
  "name": "my-agent-session",
  "satellite_id": "660e8400-e29b-41d4-a716-446655440001",
  "agent_binary": "/usr/local/bin/agent",
  "agent_args": ["--model", "gpt-4"],
  "cols": 120,
  "rows": 40
}
```

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | Yes | — | Human-readable session name |
| `satellite_id` | UUID | Yes | — | Target satellite ID |
| `agent_binary` | string | Yes | — | Path to the agent binary |
| `agent_args` | string[] | No | `[]` | Command-line arguments |
| `cols` | int | No | `80` | Terminal columns |
| `rows` | int | No | `24` | Terminal rows |

**Response:** `201 Created`
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "state": "PROVISIONING",
  "satellite_id": "660e8400-e29b-41d4-a716-446655440001",
  "name": "my-agent-session",
  "created_at": "2026-03-01T22:00:00Z"
}
```

---

#### Get Session

```http
GET /api/v1/sessions/{sessionId}
```

Returns session details including recent event logs.

**Response:** `200 OK`
```json
{
  "session": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "state": "RUNNING",
    "...": "..."
  },
  "events": [
    {
      "id": 1,
      "session_id": "550e8400-e29b-41d4-a716-446655440000",
      "event_type": "STATE_CHANGE",
      "payload": {
        "from": "PROVISIONING",
        "to": "RUNNING"
      },
      "created_at": "2026-03-01T22:00:01Z"
    }
  ]
}
```

---

#### Session Actions

All session actions use `POST` and return an `SessionActionResponse`:

```json
{
  "status": "ok",
  "state": "RUNNING",
  "session": "550e8400-e29b-41d4-a716-446655440000"
}
```

| Action | Endpoint | Description |
|---|---|---|
| **Attach** | `POST /api/v1/sessions/{id}/attach` | Re-attach to a DETACHED session → RE_ATTACHING → RUNNING |
| **Detach** | `POST /api/v1/sessions/{id}/detach` | Detach from a RUNNING session → DETACHED (starts DMS) |
| **Suspend** | `POST /api/v1/sessions/{id}/suspend` | Suspend a session → SUSPENDED (SIGTSTP) |
| **Resume** | `POST /api/v1/sessions/{id}/resume` | Resume a SUSPENDED session → RUNNING (SIGCONT) |
| **Kill** | `POST /api/v1/sessions/{id}/kill` | Terminate a session → TERMINATED |

---

### Satellites

#### List Satellites

```http
GET /api/v1/satellites
```

Returns all registered satellites.

**Response:** `200 OK`
```json
[
  {
    "id": "660e8400-e29b-41d4-a716-446655440001",
    "name": "dev-machine-1",
    "owner_id": "770e8400-e29b-41d4-a716-446655440002",
    "status": "active",
    "created_at": "2026-02-28T10:00:00Z",
    "updated_at": "2026-03-01T22:00:00Z"
  }
]
```

---

### Agents

#### List Agents

```http
GET /api/v1/agents
```

Returns all agent definitions, with optional filtering.

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `category` | string | Filter by category (`infrastructure`, `development`, `security`, `operations`) |
| `type` | string | Filter by type (`specialist`, `autonomous`) |
| `builtin` | bool | Filter by built-in status |

**Response:** `200 OK`
```json
{
  "agents": [
    {
      "id": "b6a5622d-61d7-465d-ba8f-e6726e0051b9",
      "name": "log-inspector-01",
      "display_name": "log-inspector-01",
      "description": null,
      "version": "1.0.0",
      "type": "specialist",
      "category": "operations",
      "icon": "🔍",
      "provider": "ollama",
      "model": "minimax-m2.5",
      "system_prompt": "You are a Docker Log Reviewer...",
      "tools_config": "{\"allow\":[\"exec\",\"read\"],\"deny\":[\"write\"]}",
      "guardrails": "{\"hitl_enabled\":false,\"read_only\":true,\"max_tool_calls\":100}",
      "schedule": null,
      "output_config": null,
      "is_builtin": false,
      "is_enterprise": false,
      "created_at": "2026-03-08T00:05:50Z",
      "updated_at": "2026-03-08T00:05:50Z"
    }
  ]
}
```

---

#### Get Agent

```http
GET /api/v1/agents/{agentId}
```

Returns a single agent definition by ID.

**Response:** `200 OK`
```json
{
  "agent": {
    "id": "b6a5622d-...",
    "name": "log-inspector-01",
    "...": "..."
  }
}
```

---

#### Create Agent

```http
POST /api/v1/agents
```

Creates a new agent definition.

**Request Body:**
```json
{
  "name": "my-agent",
  "display_name": "My Agent",
  "description": "Analyzes logs for errors",
  "type": "specialist",
  "category": "operations",
  "icon": "🔍",
  "provider": "openai",
  "model": "gpt-4o",
  "system_prompt": "You are a helpful agent.",
  "tools_config": {"allow": ["exec", "read"], "deny": ["write"]},
  "guardrails": {"hitl_enabled": false, "read_only": true, "max_tool_calls": 100},
  "schedule": null
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Machine-readable name (lowercase, hyphens, `^[a-z][a-z0-9-]*$`) |
| `display_name` | string | Yes | Human-readable display name |
| `description` | string | No | Agent description |
| `type` | string | Yes | `specialist` or `autonomous` |
| `category` | string | Yes | `infrastructure`, `development`, `security`, or `operations` |
| `icon` | string | No | Emoji icon |
| `provider` | string | Yes | AI provider (`openai`, `anthropic`, `google`, `ollama`) |
| `model` | string | Yes | Model identifier (e.g., `gpt-4o`) |
| `system_prompt` | string | Yes | System prompt text |
| `tools_config` | object | No | `{allow: string[], deny: string[]}` |
| `guardrails` | object | No | `{hitl_enabled, read_only, max_tool_calls, max_turns, timeout_minutes}` |
| `schedule` | string | No | Cron expression for scheduled execution |

**Response:** `201 Created` — returns the created agent object.

---

#### Update Agent

```http
PUT /api/v1/agents/{agentId}
```

Updates an existing agent definition. All fields are optional — only provided fields are updated.

**Request Body:**
```json
{
  "display_name": "Updated Name",
  "description": "Updated description",
  "provider": "anthropic",
  "model": "claude-3.5-sonnet"
}
```

**Response:** `200 OK` — returns the updated agent object wrapped in `{"agent": {...}}`.

---

#### Delete Agent

```http
DELETE /api/v1/agents/{agentId}
```

Deletes an agent definition. Built-in agents cannot be deleted.

**Response:** `200 OK`
```json
{
  "status": "deleted"
}
```

---

#### Deploy Agent

```http
POST /api/v1/agents/{agentId}/deploy
```

Deploys an agent to a satellite, creating a new agent run and session.

**Request Body:**
```json
{
  "satellite_id": "660e8400-e29b-41d4-a716-446655440001",
  "session_id": null,
  "config": {},
  "secrets": {}
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `satellite_id` | UUID | Yes | Target satellite |
| `session_id` | UUID | No | Existing session to attach to |
| `config` | object | No | Runtime configuration overrides |
| `secrets` | object | No | Secret values for the agent |

**Response:** `200 OK`
```json
{
  "run_id": "...",
  "agent_id": "...",
  "satellite_id": "...",
  "session_id": "...",
  "status": "running"
}
```

---

#### List Agent Runs

```http
GET /api/v1/agents/{agentId}/runs
```

Returns execution history for an agent.

**Response:** `200 OK`
```json
{
  "runs": [
    {
      "id": "...",
      "agent_id": "...",
      "status": "completed",
      "started_at": "2026-03-08T00:10:00Z",
      "ended_at": "2026-03-08T00:15:00Z",
      "total_tokens": 1234,
      "estimated_cost": 0.05,
      "tool_call_count": 12
    }
  ]
}
```

---

#### Agent Run Event Stream (SSE)

```http
GET /api/v1/runs/{runId}/stream
```

Server-Sent Events stream for real-time agent events. Replays full history on connect, then streams live events. Uses `EventSource` API. Authentication via `daao_auth` HttpOnly cookie (set by `POST /api/v1/auth/cookie`).

**Event Types:**

| Event | Data | Description |
|-------|------|-------------|
| `connected` | — | Stream established |
| `history` | `AgentStreamEvent` JSON | Replayed historical event |
| `live_start` | — | All history replayed, live events follow |
| `agent_event` | `AgentStreamEvent` JSON | Live event from running agent |

**AgentStreamEvent format:**
```json
{
  "id": "uuid",
  "run_id": "uuid",
  "event_type": "message_update",
  "payload": {"delta": "Hello "},
  "sequence": 5,
  "created_at": "2026-03-08T22:00:00Z"
}
```

---

### Auth Cookie

#### Set Auth Cookie

```http
POST /api/v1/auth/cookie
Authorization: Bearer <jwt>
```

Validates a JWT token and sets it as an `HttpOnly; Secure; SameSite=Strict` cookie (`daao_auth`) for SSE endpoint authentication. Called by the frontend after OIDC token exchange.

**Response:** `204 No Content` (cookie set in response headers)

**Errors:**
- `401` — Missing or invalid Bearer token

---

#### Clear Auth Cookie

```http
DELETE /api/v1/auth/cookie
```

Clears the `daao_auth` cookie. Called on logout.

**Response:** `204 No Content`

---

### Users

> Requires RBAC — endpoints are gated by role. See [SECURITY.md](SECURITY.md) for the role model.

#### List Users

```http
GET /api/v1/users
```

Returns all users. **Requires `admin` role.**

**Response:** `200 OK`
```json
[
  {
    "id": "770e8400-e29b-41d4-a716-446655440002",
    "email": "alice@example.com",
    "name": "Alice",
    "role": "admin",
    "avatar_url": "https://...",
    "last_login_at": "2026-03-07T22:00:00Z",
    "created_at": "2026-03-01T10:00:00Z"
  }
]
```

---

#### Get Current User

```http
GET /api/v1/users/me
```

Returns the authenticated user's profile. **Requires `viewer` role (any authenticated user).**

**Response:** `200 OK` — same shape as a single user object.

**Errors:**
- `401` — No user in context (OIDC not configured or no token)

---

#### Get User by ID

```http
GET /api/v1/users/{userId}
```

Returns a user by UUID. **Requires `admin` role.**

**Response:** `200 OK` — single user object.

---

#### Invite User

```http
POST /api/v1/users/invite
```

Pre-registers a user with an email and role. **Requires `owner` role.**

**Request Body:**
```json
{
  "email": "bob@example.com",
  "role": "admin"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `email` | string | Yes | User's email address |
| `role` | string | Yes | Role: `viewer`, `admin`, or `owner` |

**Response:** `201 Created` — returns the created user object.

**Errors:**
- `409` — Email already exists

---

#### Change User Role

```http
PATCH /api/v1/users/{userId}/role
```

Changes a user's role. **Requires `owner` role.**

**Request Body:**
```json
{
  "role": "admin"
}
```

**Response:** `200 OK`
```json
{
  "ok": true
}
```

**Errors:**
- `400` — Invalid role value

---

#### Delete User

```http
DELETE /api/v1/users/{userId}
```

Removes a user. **Requires `owner` role.** Cannot delete self.

**Response:** `204 No Content`

**Errors:**
- `400` — Cannot delete self

---

#### Request Access Upgrade

```http
POST /api/v1/users/{userId}/request-access
```

Requests a role upgrade. Creates a notification for all owners. **Requires `viewer` role.**

**Request Body:**
```json
{
  "requested_role": "admin",
  "reason": "Need to manage satellites for my team"
}
```

**Response:** `200 OK`
```json
{
  "ok": true
}
```

---

### Push Notifications

#### Subscribe

```http
POST /api/v1/push/subscribe
```

Register a Web Push subscription.

**Request Body:**
```json
{
  "endpoint": "https://fcm.googleapis.com/fcm/send/...",
  "keys": {
    "p256dh": "BNcRdreALRFX...",
    "auth": "tBHItJI5svbpC..."
  },
  "expirationTime": null,
  "vapidPublicKey": "BEl62iUYgU..."
}
```

#### Unsubscribe

```http
POST /api/v1/push/unsubscribe
```

```json
{
  "endpoint": "https://fcm.googleapis.com/fcm/send/..."
}
```

---

### Session Stream (WebSocket)

```http
GET /api/v1/sessions/stream
```

Upgrades to a WebSocket connection that receives real-time session list updates.

**Message Format:**
```json
[
  {
    "id": "550e8400-...",
    "state": "RUNNING",
    "name": "my-session",
    "last_activity_at": "2026-03-01T22:05:00Z"
  }
]
```

Messages are pushed whenever session state changes occur.

---

## gRPC Service

### SatelliteGateway

```protobuf
service SatelliteGateway {
  // Bidirectional streaming between Satellite and Nexus
  rpc Connect(stream SatelliteMessage) returns (stream NexusMessage);
}
```

**Service Address:** `:8444` (mTLS required)

### Satellite → Nexus Messages

```protobuf
message SatelliteMessage {
  oneof payload {
    RegisterRequest register_request = 1;
    TerminalData terminal_data = 2;
    SessionStateUpdate session_state_update = 3;
    IpcEvent ipc_event = 4;
    HeartbeatPing heartbeat_ping = 5;
  }
}
```

| Message | Fields | Description |
|---|---|---|
| `RegisterRequest` | `satellite_id`, `fingerprint`, `public_key`, `timestamp` | Sent on connect |
| `TerminalData` | `session_id`, `data`, `sequence_number`, `timestamp`, `is_stdout` | PTY output bytes |
| `SessionStateUpdate` | `session_id`, `state`, `timestamp`, `error_message` | State transitions |
| `IpcEvent` | `session_id`, `event_type`, `payload`, `timestamp` | IPC events |
| `HeartbeatPing` | `timestamp`, `sequence_number` | Keep-alive (30s interval) |

### Nexus → Satellite Messages

```protobuf
message NexusMessage {
  oneof payload {
    TerminalInput terminal_input = 1;
    ResizeCommand resize_command = 2;
    SuspendCommand suspend_command = 3;
    ResumeCommand resume_command = 4;
    KillCommand kill_command = 5;
  }
}
```

| Message | Fields | Description |
|---|---|---|
| `TerminalInput` | `session_id`, `data`, `sequence_number` | User keystrokes |
| `ResizeCommand` | `session_id`, `width`, `height`, `pixel_width`, `pixel_height` | Terminal resize |
| `SuspendCommand` | `session_id` | Trigger process suspension |
| `ResumeCommand` | `session_id` | Resume suspended process |
| `KillCommand` | `session_id`, `exit_code` | Terminate process |

### Session States (protobuf enum)

```protobuf
enum SessionState {
  SESSION_STATE_UNSPECIFIED = 0;
  SESSION_STATE_STARTING = 1;
  SESSION_STATE_RUNNING = 2;
  SESSION_STATE_SUSPENDED = 3;
  SESSION_STATE_TERMINATED = 4;
  SESSION_STATE_ERROR = 5;
  SESSION_STATE_DETACHED = 6;
  SESSION_STATE_RE_ATTACHING = 7;
}
```

---

## WebTransport Protocol

WebTransport is used for low-latency terminal streaming between Cockpit and Nexus.

**Endpoint:** `https://localhost:8443/api/v1/sessions/{sessionId}/transport`

### Stream Types

| Type ID | Direction | Purpose |
|---|---|---|
| `0` (TerminalRX) | Server → Client | Terminal output from satellite |
| `1` (TerminalTX) | Client → Server | Terminal input from user |
| `2` (Control) | Bidirectional | PING/PONG, state updates, session info |
| `3` (OOBUI) | Server → Client | Out-of-band UI (progress bars, forms) |

### Control Messages

| Type | Payload | Description |
|---|---|---|
| `PING` | `{timestamp, sequenceNumber}` | Latency measurement |
| `PONG` | `{timestamp, sequenceNumber}` | Ping response |
| `STATE_UPDATE` | `{sessionId, state, timestamp, errorMessage}` | State change notification |
| `SESSION_INFO` | `{sessionId, satelliteId, userId, cols, rows, createdAt}` | Session metadata |
| `ERROR` | `{message}` | Error notification |

### OOB UI Messages

| Type | Description |
|---|---|
| `Progress` | Progress bar for long-running operations |
| `Form` | Interactive form overlay |
| `Confirmation` | Yes/No dialog |
| `Info` | Informational notification |
| `Error` | Error notification overlay |

---

## Error Responses

All error responses follow a consistent format:

```json
{
  "error": "descriptive error message"
}
```

| HTTP Status | Meaning |
|---|---|
| `400` | Bad request (invalid JSON, missing fields) |
| `401` | Authentication required |
| `403` | Forbidden (insufficient permissions) |
| `404` | Resource not found |
| `409` | State conflict (invalid state transition) |
| `429` | Rate limited |
| `500` | Internal server error |
