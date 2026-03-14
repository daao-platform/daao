# Deployment Guide

> Docker Compose setup, production configuration, Nginx reverse proxy, and monitoring.

---

## Docker Compose (Development / Staging)

The default `docker-compose.yml` brings up the complete stack:

```bash
docker compose up -d
```

### Services

```mermaid
graph TB
    subgraph "Docker Compose"
        PG["daao-postgres<br/>postgres:alpine<br/>:5432"]
        NX["daao-nexus<br/>Go multi-stage<br/>:8443 / :8444"]
        CK["daao-cockpit<br/>Nginx + React<br/>:8081 → :80"]
    end

    NX -->|"depends_on<br/>service_healthy"| PG
    CK -->|"depends_on<br/>service_healthy"| NX
```

| Service | Image | Ports | Health Check |
|---|---|---|---|
| `postgres` | `postgres:alpine` | `5432:5432` | `pg_isready -U daao -d daao` |
| `nexus` | Multi-stage Go build | `8443:8443`, `8444:8444` | `nc -z localhost 8443` |
| `cockpit` | Multi-stage Node + Nginx | `8081:80` | `wget --spider http://127.0.0.1/health` |

### Environment Variables

#### Nexus

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | `postgres://daao:daao_password@postgres:5432/daao?sslmode=disable` | PostgreSQL connection string |
| `JWT_SECRET` | `change-me-in-production` | HMAC-SHA256 signing key |
| `NEXUS_HTTP_ADDR` | `:8443` | HTTPS listen address |
| `NEXUS_GRPC_ADDR` | `:8444` | gRPC listen address |
| `SERVER_CERT` | `/certs/server.crt` | TLS server certificate |
| `SERVER_KEY` | `/certs/key.pem` | TLS server private key |
| `CLIENT_CAS` | `/certs/ca.crt` | CA for client certificate verification |

#### PostgreSQL

| Variable | Default | Description |
|---|---|---|
| `POSTGRES_USER` | `daao` | Database user |
| `POSTGRES_PASSWORD` | `daao_password` | Database password |
| `POSTGRES_DB` | `daao` | Database name |

---

## Docker Images

### Nexus (`Dockerfile.nexus`)

Multi-stage build:

| Stage | Base Image | Purpose |
|---|---|---|
| **builder** | `golang:1.25-alpine` | Compile Go binary with `-ldflags="-s -w"` |
| **production** | `alpine:3.21` | Minimal runtime with `ca-certificates` and `netcat-openbsd` |

Build features:
- `CGO_ENABLED=0` for static binary
- Runs as non-root user (`nonroot`)
- Includes database migrations at `/db`
- Includes TLS certificates at `/certs`

### Cockpit (`Dockerfile.cockpit`)

Multi-stage build:

| Stage | Base Image | Purpose |
|---|---|---|
| **builder** | `node:22-alpine` | Build Vite/React app with `npm run build` |
| **production** | `nginx:alpine` | Serve static assets |

---

## Nginx Configuration

The Cockpit container uses Nginx as a reverse proxy:

```nginx
# deploy/nginx.conf
server {
    listen 80;

    # Cockpit static assets
    location / {
        root /usr/share/nginx/html;
        try_files $uri /index.html;  # SPA fallback
    }

    # Proxy API requests to Nexus
    location /api/ {
        proxy_pass https://nexus:8443;
        proxy_ssl_verify off;  # Internal network
    }

    # WebSocket proxy
    location /api/v1/sessions/stream {
        proxy_pass https://nexus:8443;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    # Health check
    location /health {
        return 200 'ok';
    }
}
```

---

## Production Deployment

### Security Hardening

1. **Replace JWT secret:**
   ```bash
   export JWT_SECRET=$(openssl rand -base64 32)
   ```

2. **Use CA-signed TLS certificates:**
   ```bash
   # Mount real certificates
   volumes:
     - /etc/letsencrypt/live/yourdomain.com:/certs:ro
   ```

3. **Enable database SSL:**
   ```
   DATABASE_URL=postgres://daao:STRONG_PASSWORD@db:5432/daao?sslmode=require
   ```

4. **Restrict CORS origins** in Nexus WebSocket upgrader and Nginx config.

### Volumes and Data Persistence

| Volume | Mount | Description |
|---|---|---|
| `postgres_data` | `/var/lib/postgresql/data` | PostgreSQL data directory |

### Network

All services communicate over the `daao-network` bridge network. Only external ports are exposed:

| Port | Service | Purpose |
|---|---|---|
| `5432` | PostgreSQL | Database (remove in production) |
| `8081` | Cockpit | Web dashboard |
| `8443` | Nexus | REST API / WebSocket |
| `8444` | Nexus | gRPC (Satellite connections) |

---

## Satellite Deployment

Satellites are standalone binaries deployed on remote machines:

```bash
# 1. Build the satellite binary
make build-go

# 2. Copy to the remote machine
scp bin/nexus remote-machine:~/daao

# 3. On the remote machine: generate keys and register
./daao login
```

### Satellite Configuration

| Environment Variable | Default | Description |
|---|---|---|
| `DAAO_DMS_TTL` | `60` | DMS idle timeout in minutes |
| `NEXUS_ADDR` | — | Nexus gRPC address |

---

## Monitoring

### Health Checks

| Service | Endpoint | Method |
|---|---|---|
| Nexus | `GET /health` | HTTP 200 `{"status":"ok"}` |
| Cockpit | `GET /health` | HTTP 200 `ok` |
| PostgreSQL | `pg_isready` CLI | Exit code 0 |

### Docker Compose Health

```bash
# View service status
docker compose ps

# View logs
docker compose logs -f nexus
docker compose logs -f cockpit

# Restart a service
docker compose restart nexus
```

### Key Metrics to Monitor

| Metric | Source | Description |
|---|---|---|
| Active sessions | `sessions` table WHERE `terminated_at IS NULL` | Current session count |
| Session state distribution | `sessions` table GROUP BY `state` | Health indicator |
| Event rate | `event_logs` table | Operations per second |
| gRPC stream count | `StreamRegistry` | Active Satellite connections |
| Heartbeat loss | Event logs `HEARTBEAT_LOSS` | Satellite connectivity |
| DMS triggers | Event logs `DMS_TRIGGERED` | Idle session management |
