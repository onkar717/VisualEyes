# System Agent

Collects host-level metrics and pushes them to the VisualEyes backend.

## Metrics Collected

| Metric | Details |
|--------|---------|
| CPU | Per-core usage %, system/user/idle split |
| Memory | Used/free/total, swap usage |
| Disk | Usage % per mount, read/write I/O |
| Network | Bytes sent/received per interface |
| Load Average | 1m, 5m, 15m |

## Configuration

| Env Variable | Default | Description |
|-------------|---------|-------------|
| `VISUAL_EYES_OUTPUT_REMOTE_ENDPOINT` | `http://localhost:8080/api/system-metrics` | Backend push URL |
| `VISUAL_EYES_AGENT_COLLECTION_INTERVAL` | `10s` | How often to collect and push |
| `VISUAL_EYES_AGENT_DISABLE_HOST_METRICS` | `false` | Disable host collection |

## Run

```bash
# Build
go build -o bin/visual-eyes-agent ./agents/system

# Run (defaults)
./bin/visual-eyes-agent

# Run pointing at remote backend
VISUAL_EYES_OUTPUT_REMOTE_ENDPOINT=http://192.168.1.100:8080/api/system-metrics \
  ./bin/visual-eyes-agent
```

## Docker

```bash
docker build -f agents/system/Dockerfile -t visual-eyes-system-agent:latest .
docker run --rm --network host visual-eyes-system-agent:latest
```
