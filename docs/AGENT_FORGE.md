# Agent Forge

> **Status**: Production-ready (hardened)  
> **License Gate**: Analytics, Scheduler, and Agent Builder require an Enterprise license. The agent registry, pre-built agents, and deployment flow work in Community.

Agent Forge is DAAO's agent orchestration engine. It lets you define, deploy, schedule, and monitor AI agents across your satellite infrastructure. Agents run on remote machines via Pi (`pi.dev`), using the satellite daemon as a runtime bridge.

---

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Core Concepts](#core-concepts)
- [Agent Registry](#agent-registry)
- [Deployment Flow](#deployment-flow)
- [Secrets & Provider Configuration](#secrets--provider-configuration)
- [Scheduling & Triggers (Enterprise)](#scheduling--triggers-enterprise)
- [Analytics (Enterprise)](#analytics-enterprise)
- [Pi Extension Pack](#pi-extension-pack)
- [Context System](#context-system)
- [Database Schema](#database-schema)
- [API Reference](#api-reference)
- [Community vs Enterprise Features](#community-vs-enterprise-features)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     COCKPIT (React)                          │
│   useAgents hook · ContextEditor · Agent Builder (Ent.)      │
└────────────────────────────┬────────────────────────────────┘
                             │ REST API (/api/v1/agents/...)
┌────────────────────────────▼────────────────────────────────┐
│                     NEXUS GATEWAY (Go)                       │
│                                                              │
│  ┌──────────────┐  ┌────────────────┐  ┌─────────────────┐  │
│  │ AgentHandler │  │ ProviderConfig │  │ Secrets Broker   │  │
│  │  CRUD + Deploy│  │  Masked Keys   │  │  AES-256-GCM    │  │
│  └──────┬───────┘  └────────────────┘  └─────────────────┘  │
│         │                                                    │
│  ┌──────▼───────┐  ┌────────────────┐  ┌─────────────────┐  │
│  │ agent_defs   │  │ agent_runs     │  │ encrypted_secrets│  │
│  │ (PostgreSQL) │  │ (PostgreSQL)   │  │ (PostgreSQL)     │  │
│  └──────────────┘  └────────────────┘  └─────────────────┘  │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐     │
│  │          Enterprise Forge (license-gated)            │     │
│  │   Scheduler (cron) · Triggers (telemetry) · Analytics│     │
│  └─────────────────────────────────────────────────────┘     │
└──────────────────────────┬──────────────────────────────────┘
                           │ gRPC: DeployAgent
┌──────────────────────────▼──────────────────────────────────┐
│                  SATELLITE DAEMON (Go)                        │
│                                                              │
│  1. Receives agent definition from Nexus                     │
│  2. Writes AGENTS.md + systeminfo.md context                 │
│  3. Spawns: pi --mode rpc --provider X --model Y             │
│  4. Bridges Pi RPC events ↔ Nexus WebSocket stream           │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐    │
│  │ Pi RPC Process (Node.js)                              │    │
│  │                                                       │    │
│  │ Extensions loaded:                                    │    │
│  │  ├─ daao-telemetry.ts    (token/cost tracking)        │    │
│  │  ├─ daao-guardrails.ts   (safety limits)              │    │
│  │  ├─ daao-hitl-gate.ts    (human approval gates)       │    │
│  │  ├─ daao-context-loader.ts (systeminfo.md injection)  │    │
│  │  ├─ daao-output-router.ts  (structured output)        │    │
│  │  └─ daao-sandbox.ts       (command sandboxing)        │    │
│  └──────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────┘
```

---

## Core Concepts

### Agent Types

| Type | Description | Example |
|------|-------------|---------|
| **Autonomous** | Full-scope agents that operate independently, use tools, make decisions, and run for extended periods | Virtual Sysadmin, Incident Responder |
| **Specialist** | Narrowly-scoped, task-focused agents with constrained tools and clear input/output contracts | Log Analyzer, Security Scanner |

### Pre-Built Core Pack

DAAO ships 3 built-in agents (seeded on first startup via `SeedBuiltinAgents`):

| Agent | Type | Category | Description |
|-------|------|----------|-------------|
| **Log Analyzer** | Specialist | Operations | Parses and analyzes system logs for error patterns, security incidents, and performance issues |
| **Security Scanner** | Specialist | Security | Scans systems and configurations for vulnerabilities and compliance issues |
| **Virtual Sysadmin** | Autonomous | Operations | End-to-end infrastructure management — server management, monitoring, automation |

All built-in agents use `gpt-4o` by default. Users can change the provider/model after creation.

---

## Agent Registry

The registry stores agent definitions in the `agent_definitions` table. Each agent has:

| Field | Type | Description |
|-------|------|-------------|
| `name` | TEXT (unique) | Machine identifier (e.g., `log-analyzer`) |
| `display_name` | TEXT | Human-readable name |
| `type` | TEXT | `specialist` or `autonomous` |
| `category` | TEXT | Grouping: `operations`, `security`, etc. |
| `provider` | TEXT | AI provider: `openai`, `anthropic`, `google`, `ollama` |
| `model` | TEXT | Model identifier (e.g., `gpt-4o`, `claude-sonnet-4-20250514`) |
| `system_prompt` | TEXT | The agent's system prompt (supports Go `text/template` variables: `{{.GOOS}}`, `{{.GOARCH}}`, `{{.CONTEXT_DIR}}`, `{{.TEMP_DIR}}`) |
| `tools_config` | JSONB | Tool permissions: allow/deny lists |
| `guardrails` | JSONB | Safety configuration: HITL, read-only, timeouts |
| `schedule` | JSONB | Cron schedule (Enterprise) |
| `output_config` | JSONB | Structured output routing configuration |
| `is_builtin` | BOOLEAN | Whether shipped by DAAO |
| `is_enterprise` | BOOLEAN | Whether requires Enterprise license |

### Filtering

The API supports filtering agents by `type`, `category`, `is_builtin`, and `is_enterprise` via query parameters.

---

## Deployment Flow

```
User → Cockpit → POST /api/v1/agents/:id/deploy
                  body: { satellite_id, session_id?, config?, secrets? }
                       │
                       ▼
              Nexus validates agent + satellite
              Inserts new row in agent_runs (status: 'running')
              Sends gRPC DeployAgent to satellite daemon
                       │
                       ▼
               Satellite daemon receives definition
               Expands system prompt template ({{.GOOS}}, {{.GOARCH}}, etc.)
               Sets cmd.Dir = context directory (path sandbox)
               Injects DAAO_CONTEXT_DIR + DAAO_ALLOWED_DIRS env vars
               Writes context files (systeminfo.md, AGENTS.md)
               Spawns Pi RPC process with:
                 - Provider/model from definition
                 - Expanded system prompt + context
                 - DAAO extension pack
                        |
                        v
               Pi runs agent, streams events back
               Nexus updates agent_runs on completion
               (status: 'completed' | 'failed' | 'timeout' | 'killed')
```

### Run Tracking

Every deployment creates an `agent_runs` row:

| Field | Description |
|-------|-------------|
| `agent_id` | References `agent_definitions.id` |
| `satellite_id` | References `satellites.id` |
| `session_id` | Optional reference to a PTY session |
| `status` | `running` → `completed` / `failed` / `timeout` / `killed` |
| `total_tokens` | Token usage (updated via telemetry extension) |
| `estimated_cost` | Dollar cost estimate |
| `tool_call_count` | Number of tool calls executed |
| `started_at` | Run start timestamp |
| `ended_at` | Run completion timestamp |

---

## Secrets & Provider Configuration

Agent Forge uses a **server-side encrypted secrets model** — API keys never reach the browser in plaintext.

### Architecture

```
Browser                          Nexus                          PostgreSQL
  │                                │                                │
  │  PUT /config/providers         │                                │
  │  { "anthropic": "sk-ant-..." } │                                │
  │ ─────────────────────────────► │                                │
  │                                │  AES-256-GCM encrypt           │
  │                                │  UPSERT encrypted_secrets      │
  │                                │ ─────────────────────────────► │
  │                                │                                │
  │  GET /config/providers         │                                │
  │ ─────────────────────────────► │  Fetch + decrypt               │
  │  { masked_key: "sk-...t-..." } │  Return masked only            │
  │ ◄───────────────────────────── │                                │
```

### Encryption Details

- **Algorithm**: AES-256-GCM (authenticated encryption)
- **Master Key**: Derived from `DAAO_SECRET_KEY` env var via SHA-256. If not set, a random key is generated (secrets won't persist across restarts)
- **Storage**: `encrypted_secrets` table with `key_hash` (SHA-256), `cipher_text`, and unique `nonce` per entry
- **Key Masking**: API responses return `sk-...abc1` format — prefix (3 chars) + `...` + suffix (4 chars)

### Secret Scoping (Broker Pattern)

The `Broker` supports per-satellite secrets via the `secret_scopes` table:

1. **Satellite-specific scope**: Look for `provider = X AND satellite_id = Y`
2. **Global fallback**: If not found, look for `provider = X AND satellite_id IS NULL`

This allows different API keys per satellite (e.g., different OpenAI keys for production vs. development satellites).

---

## Scheduling & Triggers (Enterprise)

The Forge Scheduler enables automated agent execution. Both features require an Enterprise license.

### Cron Scheduling

Register periodic agent runs with standard cron expressions:

```go
scheduler.RegisterSchedule(
    agentID,                  // Which agent to run
    "0 */6 * * *",            // Every 6 hours
    satelliteID,              // Target satellite
)
```

The scheduler uses `github.com/robfig/cron/v3` under the hood. On failure, it supports:
- **Retry logic**: Up to `max_retries` attempts
- **Notification**: Via the `FailureHandler.Notify()` interface
- **Escalation**: Critical failures escalate via `FailureHandler.Escalate()`

### Event Triggers

Register agents that fire when telemetry thresholds are breached:

```go
scheduler.RegisterTrigger(
    agentID,
    TriggerCondition{
        Metric:    "cpu_usage",
        Threshold: 90.0,
        Operator:  "gt",       // gt, lt, eq, gte, lte
    },
    satelliteID,
    5 * time.Minute,           // Cooldown between triggers
)
```

### Telemetry Processing

When a satellite sends a `TelemetryReport` (metrics map like `{"cpu_usage": 95.0, "memory_usage": 72.3}`), the scheduler:

1. Acquires a write lock (`sync.Mutex` — not `RWMutex`, to prevent data races)
2. Iterates all registered triggers
3. Filters by matching `satellite_id`
4. Evaluates each condition against the reported metric
5. Checks the cooldown window (`lastFired + cooldown > now` → skip)
6. Fires the `AgentRunner.RunAgent()` if the condition is met

### Supported Operators

| Operator | Description |
|----------|-------------|
| `gt` | Greater than (default for unknown operators) |
| `lt` | Less than |
| `eq` | Equal to |
| `gte` | Greater than or equal |
| `lte` | Less than or equal |

---

## Analytics (Enterprise)

The Analytics module queries the `agent_runs` table and returns computed statistics. All methods require an Enterprise license.

### Available Queries

| Method | Description | Output |
|--------|-------------|--------|
| `GetAggregateStats` | Global stats across all runs | `total_runs`, `success_rate`, `avg_duration_ms`, `total_tokens`, `total_cost` |
| `GetAgentStats(agentID)` | Stats scoped to one agent | Same fields + `agent_id` |
| `GetSatelliteStats(satelliteID)` | Stats for one satellite + top-10 most active agents | Includes `most_active_agents[]` |
| `GetTimeSeries(interval, start, end)` | Time-bucketed run data | `timestamp`, `total_runs`, `successes`, `failures`, `tokens`, `cost` |

### Query Optimization

All stats queries compute success rates inline (single `SELECT` with `CASE WHEN status = 'completed'`), avoiding N+1 patterns. The `GetSatelliteStats` query fetches the top-10 agents with their success rates in a single grouped query.

---

## Pi Extension Pack

Agent Forge agents run via Pi (`pi.dev`) with a custom extension pack that integrates with the DAAO telemetry and safety infrastructure.

### Extensions

| Extension | Purpose |
|-----------|---------|
| `daao-telemetry.ts` | Reports token usage, cost tracking, and cumulative metrics back to Nexus |
| `daao-guardrails.ts` | Enforces safety limits: max turns, token budgets, time limits, write path restrictions via `DAAO_ALLOWED_DIRS` |
| `daao-hitl-gate.ts` | Human-in-the-Loop approval gates — pauses agent execution for commands above a risk threshold |
| `daao-context-loader.ts` | Injects `systeminfo.md` and other satellite context files into the agent's system prompt |
| `daao-output-router.ts` | Routes agent output to structured destinations (logs, notifications, files) |
| `daao-sandbox.ts` | Sandboxes command execution with allow/deny lists |

### Event Flow

```
Pi Process ──▶ stdout (JSON events)
              ├─ tool_call events     → daao-guardrails checks limits
              ├─ token_usage events   → daao-telemetry reports to Nexus
              ├─ text events          → daao-output-router dispatches
              └─ high-risk commands   → daao-hitl-gate pauses for approval

Nexus ──▶ stdin (JSON commands)
          ├─ prompt
          ├─ abort
          └─ hitl-approve / hitl-deny
```

---

## Context System

Each satellite can have context files managed via the Cockpit UI's `ContextEditor`. Files are versioned with full history tracking.

### Machine Identity (`systeminfo.md`)

The foundational concept: when an agent is deployed, the satellite's `systeminfo.md` is injected into the system prompt. This gives the agent awareness of the host's role, services, storage, and network topology.

### Context File API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/satellites/:id/context` | `GET` | List all context files |
| `/api/v1/satellites/:id/context` | `POST` | Create a new context file |
| `/api/v1/satellites/:id/context/:fileId` | `GET` | Get a specific file |
| `/api/v1/satellites/:id/context/:fileId` | `PUT` | Update content (creates history entry) |
| `/api/v1/satellites/:id/context/:fileId` | `DELETE` | Remove a file |
| `/api/v1/satellites/:id/context/:fileId/history` | `GET` | Version history with diffs |

---

## Database Schema

### Migrations

| Migration | Table | Purpose |
|-----------|-------|---------|
| 015 | `satellites` | Adds `available_agents TEXT[]` column |
| 016 | `agent_definitions` | Agent registry — 19 columns including JSONB configs |
| 017 | `agent_runs` | Run tracking — links agents to satellites/sessions |
| 020 | `encrypted_secrets` | AES-256-GCM encrypted secret storage |

### Key Relationships

```
agent_definitions.id ◄──── agent_runs.agent_id
satellites.id        ◄──── agent_runs.satellite_id
sessions.id          ◄──── agent_runs.session_id (optional)
```

---

## API Reference

### Agent CRUD

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/agents` | `GET` | List agents (paginated, filterable) |
| `/api/v1/agents` | `POST` | Create a new agent definition |
| `/api/v1/agents/:id` | `GET` | Get a single agent |
| `/api/v1/agents/:id` | `PUT` | Update an agent definition |
| `/api/v1/agents/:id` | `DELETE` | Delete an agent |
| `/api/v1/agents/:id/deploy` | `POST` | Deploy agent to a satellite |
| `/api/v1/agents/:id/runs` | `GET` | List runs for an agent |

### Provider Configuration

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/config/providers` | `GET` | List providers with masked keys |
| `/api/v1/config/providers` | `PUT` | Save/update provider API keys |

### Supported Providers

| ID | Provider |
|----|----------|
| `openai` | OpenAI (GPT-4o, etc.) |
| `anthropic` | Anthropic (Claude) |
| `google` | Google (Gemini) |
| `ollama` | Ollama (local models) |

---

## Community vs Enterprise Features

| Feature | Community | Enterprise |
|---------|:---------:|:----------:|
| Agent Catalog & Registry | ✅ | ✅ |
| 3 Pre-built Core Agents | ✅ | ✅ |
| Agent CRUD API | ✅ | ✅ |
| Deploy to Satellites | ✅ | ✅ |
| Encrypted Secret Storage | ✅ | ✅ |
| Provider Config API | ✅ | ✅ |
| Context Files (systeminfo.md) | ✅ | ✅ |
| HITL Guardrails | ❌ | ✅ |
| Cron Scheduling | ❌ | ✅ |
| Event Triggers (Telemetry) | ❌ | ✅ |
| Analytics Dashboard | ❌ | ✅ |
| Agent Builder UI | ❌ | ✅ |
| Agent Chaining / Pipelines | ❌ | ✅ |

---

## File Map

```
internal/
├── api/
│   ├── agents.go              # Agent CRUD + deploy handlers
│   ├── helpers.go             # writeJSON / writeJSONError
│   ├── provider_config.go     # Provider API key management
│   └── context.go             # Context file CRUD handlers
├── database/
│   ├── agents.go              # AgentDefinition struct + queries
│   └── agent_runs.go          # AgentRun struct + queries
├── enterprise/
│   └── forge/
│       ├── scheduler.go       # Cron schedules + event triggers
│       └── analytics.go       # Run statistics + time series
├── secrets/
│   ├── broker.go              # Secret scoping + pull-on-demand
│   └── local.go               # AES-256-GCM LocalBackend
cockpit/
└── src/
    ├── api/client.ts          # Shared apiRequest (canonical)
    ├── hooks/useAgents.ts     # React hooks for agent registry
    └── components/
        └── ContextEditor.tsx  # Context file editor UI
extensions/
├── daao-telemetry.ts          # Token/cost tracking
├── daao-guardrails.ts         # Safety limits
├── daao-hitl-gate.ts          # Human approval gates
├── daao-context-loader.ts     # systeminfo.md injection
├── daao-output-router.ts      # Structured output dispatch
└── daao-sandbox.ts            # Command sandboxing
db/migrations/
├── 016_agent_definitions.up.sql
├── 017_agent_runs.up.sql
└── 020_encrypted_secrets.up.sql
```
