# DAAO Roadmap

> This is the public-facing roadmap for DAAO. For detailed status on completed work, see [PROGRESS.md](./PROGRESS.md).

---

## Current Focus: Open-Source Launch

DAAO is preparing for its first public release as a source-available project under the [Business Source License 1.1](../LICENSE).

### What's Shipping in v1.0

- ✅ Cross-platform satellite daemon (Windows, Linux, macOS)
- ✅ Nexus control plane with gRPC + REST API
- ✅ Web Cockpit with live terminal streaming (React + xterm.js)
- ✅ Multi-session dashboard (1-6 concurrent panes)
- ✅ Session lifecycle (create, attach, detach, suspend, resume, kill)
- ✅ Session recordings with playback
- ✅ Real-time satellite telemetry (CPU, MEM, DISK)
- ✅ In-app notifications with SSE push
- ✅ Security hardening (CSP, HSTS, input validation, request limits)
- ✅ Docker deployment (docker-compose)
- ✅ Satellite auto-update system
- ✅ Enterprise license system (Ed25519-signed JWT keys, feature gating, three-layer protection)

---

## Next: Agent Forge

**Agent Forge** is DAAO's agent creation and deployment engine. It transforms the platform from a session manager into a full agentic operations platform.

### Community (Free)

| Feature | Status | Description |
|---------|--------|-------------|
| **Agent Catalog** | ✅ Shipped | Browse, deploy, and manage pre-built specialist agents |
| **6 Pre-Built Agents** | ✅ Shipped | Log Analyzer, Security Scanner, System Monitor, Deployment Assistant, Agent Builder (enterprise), Infrastructure Discovery (enterprise) |
| **Pi RPC Agent Runtime** | ✅ Shipped | Pi process bridge in satellite daemon — structured event streaming |
| **Satellite Context** | ✅ Shipped | 8 standard context files (systeminfo, runbooks, alerts, topology, secrets-policy, history, monitoring, dependencies) with bidirectional sync |
| **PTY Agent Sessions** | ✅ Shipped | Use Claude Code, Gemini CLI, or any CLI-based agent |
| **Secrets Management** | ✅ Shipped | Local encrypted secrets backend for API keys |
| **Agent Event Live View** | ✅ Shipped | Real-time Pi RPC event streaming in Cockpit via SSE (per-run endpoint + DB persistence + HttpOnly cookie auth) |

### Team (In Development — Not Yet Available)

| Feature | Description |
|---------|-------------|
| **Agent Builder** | No-code configurator: system prompts, tool allow/deny, guardrails |
| **Unlimited Agents** | Full pre-built agent library + custom agent definitions |
| **Custom Contexts** | Per-satellite context files for agent awareness |
| **Retention Policies** | Configurable recording and telemetry retention |
| **Multi-user auth + RBAC** | Team-based access control |

### Enterprise (In Development — Not Yet Available)

| Feature | Description |
|---------|-------------|
| **Agent Builder** | No-code configurator: system prompts, tool allow/deny, guardrails |
| **Unlimited Agents** | Full pre-built agent library + custom agent definitions |
| **HITL Guardrails** | Human-in-the-loop approval gates on agent tool calls |
| **Scheduled Agents** | Cron-based automated agent runs |
| **Event-Triggered Agents** | Telemetry threshold triggers (e.g., disk > 90% → cleanup agent) |
| **Agent Chaining** | Pipeline workflows: specialist → specialist → specialist |
| **Autonomous Discovery** | AI-driven `systeminfo.md` generation and CMDB population |
| **Vault Integrations** | Azure Key Vault, HashiCorp Vault, OpenBao, Infisical |
| **Dynamic Secret Leases** | Time-limited credentials with auto-renewal and revocation |
| **Agent Analytics** | Run history, token usage, cost tracking |
| **High Availability** | Multi-instance Nexus cluster with NATS JetStream, load balancer, automatic failover |
| **Object Storage Recordings** | S3/MinIO-backed recording storage for cluster deployments |
| **Distributed Rate Limiting** | Redis-backed rate limiter shared across Nexus instances |

---

## Upcoming: Enterprise Features

These features are planned for DAAO Enterprise (in development, not yet available):

| Feature | Description |
|---------|-------------|
| **Multi-user & RBAC** | OIDC/SSO integration, role-based access, team management |
| **HITL Guardrails** | Human-in-the-loop command approval for AI-driven remediation |
| **Autonomous Discovery** | Time-boxed audit mode for CMDB auto-population |
| **SIEM Integration** | Structured audit log streaming to Splunk, Sentinel, Datadog |
| **Advanced Telemetry** | GPU metrics, historical trends, capacity planning |

---

## Security Hardening

Security improvements planned across all tiers:

| Feature | Tier | Description |
|---------|------|-------------|
| **WebSocket Origin Validation** | All | `DAAO_ALLOWED_ORIGINS` env var to prevent Cross-Site WebSocket Hijacking |
| **JWT Auth Migration** | All | Move WebSocket auth from query params to first-message or subprotocol header |
| **Default Secrets Enforcement** | All | Refuse to start in production mode with known-default JWT secrets |
| **Community Agent Guardrails** | Community+ | Basic tool allow/deny, read-only mode, and timeouts for all agent users |
| **Satellite Key Protection** | All | OS-level credential storage (DPAPI, Keychain, libsecret) for satellite private keys |
| **Admin Audit Log** | All (SIEM: Enterprise) | Immutable log of all state-changing admin actions |
| **Certificate Rotation** | Enterprise | Automated mTLS certificate rotation for satellites |
| **Automated Security Scanning** | All | Pre-launch: `gosec`, `govulncheck`, OWASP ZAP, `npm audit` (free) |
| **Continuous Vuln Scanning** | All | Post-launch: Pentest-Tools.com or Intruder.io (~$100/mo) |
| **Pre-Launch Pen Test** | Internal | XBOW (AI, ~$4K) or Blaze Info Security (~$5K) before Enterprise GA |

> See [SECURITY.md](./SECURITY.md) for the full production hardening checklist.

---

## Future Vision

- Cloud-hosted managed offering
- Agent Marketplace (community & paid agents with safety scanning)
- Agent routing (GPU/capability-based scheduling)
- High availability (multi-instance Nexus, NATS JetStream, automatic failover)
- Session memory (persistent context via ARK integration)
- Mobile PWA

---

## Contributing

We welcome contributions! See [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.

If you're interested in a feature, check the [Issues](../../issues) page or open a discussion.
