# Backend Server

Go HTTP server. Central hub for metrics ingestion, alert processing, AI RCA, WebSocket broadcasting, and Prometheus metrics exposure.

## API Reference

### System Metrics

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/system-metrics` | Ingest system metrics from system agent |
| `GET` | `/api/metrics/snapshot` | Latest system metrics snapshot |

### Kubernetes Metrics

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/kubernetes-metrics` | Ingest K8s metrics from kubernetes agent |
| `GET` | `/api/kubernetes/metrics` | Latest Kubernetes metrics snapshot |

### Alerts

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/alerts` | List all active alerts |
| `GET` | `/api/alerts/:id` | Get single alert by ID |

### RCA

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/rca` | List all RCA results |
| `GET` | `/api/rca/:id` | Get RCA result for incident |

### Logs

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/logs` | List collected pod logs |

### Health & Observability

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check — `{"status":"ok"}` |
| `GET` | `/metrics` | Prometheus metrics endpoint |
| `GET` | `/ws` | WebSocket — live metric stream |

## Architecture

```
backend/
├-- alerts/          # Alert engine: rule evaluation, dedup, noise filter
├-- api/
│   ├-- handler.go   # All HTTP handler functions
│   ├-- routes.go    # Route registration
│   └-- middleware/  # CORS, logging, rate limit, recovery
├-- config/          # Config loading from file + env vars
├-- http/            # Outbound HTTP client (used by agents)
├-- metrics/         # Prometheus registry and metric definitions
├-- models/          # Shared data models (Alert, Metric, RCAResult, PodLog)
├-- rca/
│   ├-- claude_client.go    # Anthropic API client
│   ├-- context_builder.go  # Builds incident context for Claude
│   ├-- executor.go         # Safe command execution
│   └-- processor.go        # RCA pipeline orchestration
├-- storage/
│   ├-- interface.go  # Storage interface
│   ├-- memory.go     # In-memory store (dev/no-DB mode)
│   └-- postgres.go   # PostgreSQL via GORM
├-- ws/
│   └-- broadcaster.go  # WebSocket hub, client management, broadcast loop
└-- main.go
```

## Run

```bash
# Build
go build -o bin/visual-eyes-server ./backend

# Run (in-memory storage, no DB required)
./bin/visual-eyes-server

# Run with PostgreSQL
DATABASE_URL=postgres://user:pass@localhost:5432/visualeyes ./bin/visual-eyes-server

# Run with AI RCA enabled
ANTHROPIC_API_KEY=sk-ant-... ./bin/visual-eyes-server
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `VISUAL_EYES_SERVER_PORT` | `8080` | HTTP listen port |
| `DATABASE_URL` | — | PostgreSQL DSN; falls back to in-memory if unset |
| `ANTHROPIC_API_KEY` | — | Claude API key for RCA; RCA disabled if unset |
| `VISUAL_EYES_LOG_LEVEL` | `info` | Log level: debug/info/warn/error |

## Storage Modes

**In-memory** (default, no config needed):
- All data lost on restart
- Good for local dev and demos

**PostgreSQL** (set `DATABASE_URL`):
- Persistent incident history, alerts, RCA results, pod logs
- Auto-migrates schema on startup via GORM
