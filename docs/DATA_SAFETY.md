# Data Safety & LLM Provider Risk

> **DAAO's core promise: Your infrastructure data never leaves your network unless you explicitly configure it to.**

DAAO deploys AI agents to your infrastructure. Those agents need LLM providers to function. This document explains exactly what data flows where, the risks involved, and how to configure DAAO for your organization's risk tolerance.

---

## What Data Leaves Your Network?

When an agent is deployed, the following data is sent to the configured LLM provider:

### ✅ Sent to LLM Provider

| Data | Source | Contains |
|------|--------|----------|
| **System prompt** | Agent definition | Agent instructions, role description |
| **Machine context** (`systeminfo.md`) | Satellite context files | Host role, services, hardware, network topology |
| **Tool call results** | Agent runtime | Command output (e.g., `ps`, `df`, `journalctl`) |
| **Agent prompts** | Conversational turns | Follow-up reasoning by the agent |

### 🔒 Never Sent to LLM Provider

| Data | Where It Stays |
|------|---------------|
| **API keys / secrets** | Injected as env vars, never in prompts |
| **Satellite private keys** | Local filesystem only (`~/.daao/`) |
| **Terminal I/O (PTY sessions)** | Nexus ↔ Satellite via gRPC (encrypted) |
| **Telemetry metrics** (CPU, MEM, disk) | Nexus DB only |
| **Session recordings** | Local filesystem or S3 (your bucket) |
| **Audit logs** | Nexus DB only |
| **User credentials / JWT tokens** | Nexus DB only |

### The Key Insight

**The content of `systeminfo.md` and other context files IS sent to the LLM.** If you write your internal IP addresses, service account names, or database hostnames into `systeminfo.md`, that data goes to whichever LLM provider the agent is configured to use.

> [!IMPORTANT]
> Write context files with the assumption that their content will be processed by a third-party API. Avoid including: actual credentials, internal IP ranges you want to keep private, compliance-sensitive identifiers, or production database hostnames.

---

## Provider Risk Tiers

DAAO supports multiple LLM providers. Each has a different risk profile based on **where your data goes** and **what happens to it after processing**.

| Tier | Risk | Providers | Data Leaves Network? | Provider Retains Data? |
|------|------|-----------|---------------------|----------------------|
| 🟢 **Local** | Minimal | Ollama, vLLM, LocalAI, LM Studio | **No** | N/A — runs on your hardware |
| 🟡 **Zero-Retention API** | Moderate | Anthropic API, OpenAI API (with data opt-out) | Yes | No (per their API data policies) |
| 🟠 **Standard Cloud API** | Higher | OpenAI API (default), Google AI | Yes | May be retained for improvement |

### Provider Data Policies (as of 2026)

| Provider | API Data Used for Training? | Opt-Out Available? | DPA Available? |
|----------|---------------------------|-------------------|---------------|
| **Anthropic** | No (API data not used for training) | N/A — default is no retention | Yes |
| **OpenAI** | No by default for API (since March 2023) | Explicit opt-out via API settings | Yes |
| **Google (Gemini API)** | Varies by product/region | Check current terms | Yes (Workspace) |
| **Ollama** | N/A — runs locally | N/A | N/A |

> [!NOTE]
> Provider policies change. Always verify the current data processing terms for your specific API tier and region before deploying to production infrastructure.

---

## Protection Levels

Choose the level that matches your organization's risk tolerance:

### 🔒 Level 1: Air-Gapped / Maximum Security

**For:** Government, regulated industries, classified environments.

| Setting | Value |
|---------|-------|
| LLM Provider | Ollama or self-hosted vLLM only |
| Network | Air-gapped — Nexus has no internet access |
| Satellites | Internal network only |
| Data egress | **Zero** — all processing stays on your hardware |

**Trade-off:** Limited to models you can run locally. May require GPU hardware.

**Configuration:**
```yaml
# Only configure Ollama in Settings → Integrations
# Do NOT add API keys for cloud providers
# Set agents to use provider: ollama
```

### 🛡️ Level 2: Enterprise Standard

**For:** Enterprise IT teams, MSPs, compliance-conscious organizations.

| Setting | Value |
|---------|-------|
| LLM Provider | Anthropic API or OpenAI API (with DPA) |
| Network | Nexus has internet for API calls only |
| Data policies | Zero-retention API tier, signed DPA |
| Context files | Reviewed — no raw credentials or sensitive identifiers |
| Monitoring | LLM egress audit log enabled (when available) |

**Best practices:**
- Sign a Data Processing Agreement (DPA) with your LLM provider
- Use API tiers that explicitly exclude training on your data
- Review `systeminfo.md` content before enabling agents
- Enable the LLM egress audit log when available
- Use the HITL approval gate for autonomous agents (Enterprise license)

### 🏠 Level 3: Development / Homelab

**For:** Solo sysadmins, homelabs, development environments, evaluation.

| Setting | Value |
|---------|-------|
| LLM Provider | Any — based on preference and cost |
| Context files | General descriptions (no production secrets) |
| Monitoring | Standard agent run history |

**Guidance:**
- Be aware that infrastructure details in context files are sent to the LLM provider
- Use Ollama for sensitive machines, cloud providers for general-purpose agents
- Don't put production credentials in context files (use `secrets-policy.md` for references only)

---

## Best Practices for Context Files

Context files (`systeminfo.md`, `runbooks.md`, etc.) are the primary source of infrastructure data that reaches LLM providers. Write them defensively:

### ✅ Do

```markdown
## Services
- Web application server (port 443)
- PostgreSQL database (port 5432)
- Redis cache (port 6379)

## Network
- Management VLAN: 10.0.x.0/24
- Role: Primary application server
```

### ❌ Don't

```markdown
## Services
- Web app at https://internal-app.corp.acme.com:443
- PostgreSQL: postgres://admin:P@ssw0rd123@db-prod-01.acme.local:5432/production
- Redis: redis://:secret-token@cache-01:6379

## Network
- IP: 10.0.14.37
- Domain admin: ACME\svc_daao_admin
```

### General Rules

1. **Use generic descriptions** instead of specific hostnames and IPs
2. **Never include credentials** — use `secrets-policy.md` to reference what exists, not the values
3. **Describe roles and services** rather than exact endpoints
4. **Review before enabling agents** — treat context files as semi-public data
5. **Use Ollama for sensitive machines** where you can't abstract the context

---

## Data Flow Diagram

```
┌──────────────────────────────────────────────────────────────────────┐
│                        YOUR NETWORK                                   │
│                                                                       │
│  ┌─────────────┐    mTLS/gRPC     ┌──────────────┐                   │
│  │  Satellite   │ ◄─────────────► │    Nexus      │                   │
│  │  (your host) │                  │  (your server)│                   │
│  │              │                  │               │                   │
│  │ Context files│                  │ DB: runs,     │                   │
│  │ Terminal I/O │                  │  events, audit│                   │
│  │ Telemetry    │                  │  secrets (enc)│                   │
│  └─────────────┘                  └───────┬───────┘                   │
│                                           │                           │
│  ┌─────────────┐                          │ REST API                  │
│  │  Cockpit    │ ◄────────────────────────┘                           │
│  │  (browser)  │                                                      │
│  └─────────────┘                                                      │
│                                                                       │
└───────────────────────────────────┬───────────────────────────────────┘
                                    │
                         ┌──────────▼──────────┐
                         │  NETWORK BOUNDARY    │
                         └──────────┬──────────┘
                                    │
                    Only if using cloud LLM provider:
                                    │
                    ┌───────────────▼───────────────┐
                    │   LLM Provider API             │
                    │   (Anthropic / OpenAI / Google) │
                    │                                │
                    │   Receives:                     │
                    │   • System prompt (with context)│
                    │   • Agent conversation turns    │
                    │   • Tool call results           │
                    │                                │
                    │   Does NOT receive:             │
                    │   • API keys or secrets         │
                    │   • Terminal I/O                │
                    │   • Telemetry metrics           │
                    │   • Recordings                  │
                    │   • Audit logs                  │
                    └────────────────────────────────┘
```

> [!TIP]
> **Using Ollama or another local model eliminates the network boundary entirely.** All processing stays on your infrastructure. This is the recommended configuration for sensitive environments.

---

## Future: LLM Egress Audit

> [!NOTE]
> The features below are planned but not yet implemented. They are tracked in `docs/PROGRESS.md`.

### Planned Capabilities

- **LLM Egress Audit Log** — Every outbound LLM API call is logged with: provider, model, prompt size, which context files were included, whether the provider is local or cloud, and a hash of the prompt content (for deduplication, not content recovery)
- **Cockpit Data Flow Page** — Visual dashboard showing all data that left your network, filterable by time range, provider, and satellite
- **Provider Risk Badges** — Settings → Integrations will display risk tier badges (🟢🟡🟠) next to each configured provider
- **Data Redaction Layer** — Pre-LLM filter that automatically replaces sensitive patterns (IPs, hostnames, credential-like strings) with safe placeholders before the prompt reaches the provider

---

## DAAO's Data Safety Philosophy

1. **Your data, your control.** DAAO never phones home. Nexus runs on your infrastructure. Satellites connect to your Nexus. You choose the LLM provider.

2. **Local-first is always an option.** Ollama is a first-class supported provider. You can run DAAO with zero external API calls.

3. **Transparency over convenience.** We document exactly what data goes where. No hidden telemetry, no usage analytics, no "anonymized" data collection.

4. **Defense in depth.** Transport security (mTLS, TLS 1.3), agent guardrails (path jailing, tool restrictions), HITL approval gates, and audit trails work together — no single layer is trusted alone.

5. **The operator decides.** DAAO doesn't make risk decisions for you. We provide the tools, the documentation, and the visibility. You configure the level of protection that fits your environment.
