# veye CLI

Interactive terminal dashboard for VisualEyes. Diagnose incidents, stream logs, and run RCA without opening a browser.

## Commands

| Command | Description |
|---------|-------------|
| `veye status` | Current system and cluster health snapshot |
| `veye alerts` | Active alerts table with severity and timestamps |
| `veye logs [--follow]` | Pod log viewer; `--follow` for live tail |
| `veye rca <incident-id>` | AI Root Cause Analysis detail view |
| `veye watch` | Full Bubbletea interactive TUI dashboard |

## Build

```bash
go build -o bin/veye ./cli
```

Or via Makefile:

```bash
make build-cli
```

## Configuration

The CLI reads the backend address from:

1. `--server` flag: `veye status --server http://my-server:8080`
2. `VISUAL_EYES_SERVER_URL` env variable
3. Default: `http://localhost:8080`

## Usage Examples

```bash
# Check health
./bin/veye status

# Watch all metrics live (TUI)
./bin/veye watch

# Follow logs from all pods
./bin/veye logs --follow

# Show active alerts
./bin/veye alerts

# Get RCA for incident ID abc123
./bin/veye rca abc123

# Point at remote backend
VISUAL_EYES_SERVER_URL=http://prod-backend:8080 ./bin/veye watch
```

## Cross-Platform Binaries

Pre-compiled binaries for all platforms are available on the [Releases](https://github.com/onkar717/visual-eyes/releases) page:

- `veye-linux-amd64`
- `veye-linux-arm64`
- `veye-darwin-amd64` (Intel Mac)
- `veye-darwin-arm64` (Apple Silicon)
- `veye-windows-amd64.exe`
