<div align="center">

```
 ██╗   ██╗██╗███████╗██╗   ██╗ █████╗ ██╗     ███████╗██╗   ██╗███████╗███████╗
 ██║   ██║██║██╔════╝██║   ██║██╔══██╗██║     ██╔════╝╚██╗ ██╔╝██╔════╝██╔════╝
 ██║   ██║██║███████╗██║   ██║███████║██║     █████╗   ╚████╔╝ █████╗  ███████╗
 ╚██╗ ██╔╝██║╚════██║██║   ██║██╔══██║██║     ██╔══╝    ╚██╔╝  ██╔══╝  ╚════██║
  ╚████╔╝ ██║███████║╚██████╔╝██║  ██║███████╗███████╗   ██║   ███████╗███████║
   ╚═══╝  ╚═╝╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚══════╝╚══════╝   ╚═╝   ╚══════╝╚══════╝
```

**Cloud-Native Observability · AI-Powered RCA · Real-Time Monitoring**

[![CI](https://github.com/onkar717/VisualEyes/actions/workflows/ci.yaml/badge.svg)](https://github.com/onkar717/VisualEyes/actions/workflows/ci.yaml)
[![Release](https://img.shields.io/github/v/release/onkar717/VisualEyes?color=blue&label=Release)](https://github.com/onkar717/VisualEyes/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/onkar717/VisualEyes?label=Go)](https://golang.org)
[![Go Report Card](https://goreportcard.com/badge/github.com/onkar717/visual-eyes)](https://goreportcard.com/report/github.com/onkar717/visual-eyes)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/onkar717/VisualEyes/pulls)

### Open-source observability platform for system and Kubernetes monitoring with AI-powered incident response

</div>

---

**VisualEyes** combines real-time metrics collection, a Bubbletea TUI dashboard, AI Root Cause Analysis, and a full React web UI — giving on-call engineers complete visibility from a single tool.

---

## 🌟 Two Ways to Use VisualEyes

### 1. veye CLI (Terminal-First)
A Bubbletea-powered interactive TUI. Run `veye watch` on your machine and get live metrics, active alerts, pod logs, and AI RCA without opening a browser. Perfect for on-call engineers.

### 2. Hub + Agents (Continuous Monitoring)
Deploy agents to your systems and Kubernetes clusters. Agents push metrics to a central backend with a React dashboard, WebSocket streaming, and PostgreSQL-backed incident history.

---

## ✨ Features

- 📊 **System metrics** — CPU, memory, disk, network, load average via `gopsutil`
- ☸️ **Kubernetes metrics** — pod-level and node-level stats via kubelet summary API
- 🧠 **AI-powered RCA** — Claude AI diagnoses incidents and proposes safe remediation commands
- 🚨 **Alert engine** — configurable rules with dedup, noise filtering, auto-RCA trigger
- ⚡ **WebSocket streaming** — live metric push to React dashboard, no polling
- 📈 **Prometheus `/metrics`** — plug into any Grafana/Prometheus stack
- 🖥️ **veye CLI** — Bubbletea TUI: `status`, `alerts`, `logs`, `rca`, `watch`
- 🗄️ **PostgreSQL storage** — persistent incident history with MTTR tracking
- 🔒 **Safety-first RCA** — remediation commands validated before execution
- 🐳 **Docker & Kubernetes** — full containerized deployment, Compose + manifests

---

## 🏗️ Architecture

```
┌--------------------------------------------------------------┐
│                        VisualEyes                            │
│                                                              │
│  ┌-----------------┐    ┌---------------------------------┐  │
│  │  System Agent   │    │      Kubernetes Agent           │  │
│  │  (gopsutil)     │    │  (kubelet summary API)          │  │
│  │  CPU/Mem/Disk   │    │  Pod & Node metrics             │  │
│  │  Net/Load       │    │  Events + Logs                  │  │
│  └--------┬--------┘    └------------┬--------------------┘  │
│           │  POST /api/system-metrics │ POST /api/k8s-metrics  │
│           └--------------┬-----------┘                       │
│                          ▼                                    │
│           ┌--------------------------┐                       │
│           │      Backend Server      │                       │
│           │  Go HTTP · port 8080     │                       │
│           │  Alert Engine            │                       │
│           │  RCA Processor (Claude)  │                       │
│           │  WebSocket Broadcaster   │                       │
│           │  Prometheus /metrics     │                       │
│           │  PostgreSQL / MemStore   │                       │
│           └--------┬-----------------┘                       │
│                    │                                          │
│          ┌---------┴----------┐                              │
│          ▼                    ▼                              │
│  ┌---------------┐   ┌---------------------┐                │
│  │  React UI     │   │  veye CLI (TUI)      │               │
│  │  port 5173    │   │  status / alerts     │               │
│  │  Dark/Light   │   │  logs / rca / watch  │               │
│  └---------------┘   └---------------------┘                │
└--------------------------------------------------------------┘
```

---

## 🚀 Install (Pre-built Binaries)

No Go required. One command:

```bash
# Install veye CLI
curl -fsSL https://raw.githubusercontent.com/onkar717/VisualEyes/main/install.sh | bash

# Install server binary
curl -fsSL https://raw.githubusercontent.com/onkar717/VisualEyes/main/install.sh | bash -s visual-eyes
```

Or download directly from [Releases](https://github.com/onkar717/VisualEyes/releases).

---

## 💻 Sample Output

### veye watch — Interactive TUI Dashboard

```
┌------------------------ VisualEyes -------------------------┐
│  System        CPU: 23.4%  Mem: 6.1/16GB  Load: 0.87        │
│  Kubernetes    Nodes: 1    Pods: 13/13     Alerts: 2         │
├-------------------- Active Alerts --------------------------┤
│  [SEV1] payment-service  CrashLoopBackOff  2m ago           │
│  [SEV2] db-worker        OOMKilled         8m ago           │
├------------------------ RCA Result -------------------------┤
│  ROOT CAUSE (94% confidence):                               │
│  payment-service cannot connect to Redis at                 │
│  redis.prod.svc:6379 — connection refused.                  │
│                                                              │
│  PROPOSED FIX:                                              │
│  kubectl get svc redis -n prod                              │
│  Execute? [y / dry / n]: _                                  │
└--------------------------------------------------------------┘
```

### veye alerts

```
$ veye alerts

ID        SEVERITY  COMPONENT          REASON              AGE
-------   --------  -----------------  ------------------  ----
a1b2c3    SEV1      payment-service    CrashLoopBackOff    2m
d4e5f6    SEV2      db-worker          OOMKilled           8m
g7h8i9    SEV3      frontend-deploy    HighCPUThrottle     14m

3 active alerts. Run: veye rca <id> for root cause analysis.
```

### veye rca

```
$ veye rca a1b2c3

[VisualEyes RCA] Incident a1b2c3 — payment-service (SEV1)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

ROOT CAUSE:
  Container 'payment-service' is CrashLoopBackOff.
  Exit code 1 — unable to connect to Redis cache at
  redis.prod.svc.cluster.local:6379. TCP connection refused.

CONTRIBUTING FACTORS:
  • Redis service has 0 ready endpoints (selector mismatch)
  • Pod redis-0 is Pending — PVC not bound

REMEDIATION PLAN:
  Step 1: Check Redis pod status
  $ kubectl get pods -n prod -l app=redis
  Execute? [y/dry/n]: y
  ✓ Output: redis-0 is Pending (PVC unbound)

  Step 2: Check PersistentVolumeClaim
  $ kubectl describe pvc redis-data -n prod
  Execute? [y/dry/n]:
```

---

## 🛠️ Getting Started (Build from Source)

### Prerequisites

- Go 1.24+
- Node.js 18+
- Docker & Docker Compose
- PostgreSQL 14+ (or use Docker Compose — no local install needed)
- `kubectl` + cluster (for Kubernetes mode)

### 1. Clone & Build

```bash
git clone https://github.com/onkar717/VisualEyes.git
cd VisualEyes

make deps        # Download Go dependencies
make build       # Build server, agents, and veye CLI → bin/
make install-ui  # Install frontend dependencies
```

### 2. Configure

```bash
cp .env.example .env
# Set ANTHROPIC_API_KEY for AI RCA (optional — RCA disabled if not set)
# Set DATABASE_URL for PostgreSQL (optional — falls back to in-memory)
```

### 3. Run Locally

```bash
./bin/visual-eyes-server   # Backend API on :8080
./bin/visual-eyes-agent    # System metrics push (separate terminal)
make run-ui                # React UI on :5173
open http://localhost:5173
```

### 4. Or — Full Stack with Docker Compose

```bash
docker-compose up --build -d
# Backend :8080 · UI :3000 · PostgreSQL :5432 · system-agent
```

---

## ☸️ Kubernetes Deployment

```bash
kubectl apply -f deployments/kubernetes/rbac.yaml
kubectl apply -f deployments/kubernetes/config.yaml
kubectl apply -f deployments/kubernetes/agent.yaml

# Verify
kubectl get pods -n kube-system -l app=visual-eyes-k8s-agent
```

For minikube, kind, and production setup see [INSTALLATION.md](INSTALLATION.md).

---

## 📋 Alert Categories

VisualEyes detects and automatically triggers RCA for:

- **Pod Lifecycle** — `CrashLoopBackOff`, `OOMKilled`, `ImagePullBackOff`, `Pending`, `CreateContainerConfigError`
- **Resource Pressure** — CPU throttling, memory saturation, disk pressure, node not-ready
- **Kubernetes Health** — pod restarts exceeding threshold, deployment replica mismatch
- **Network** — service endpoint unavailability, DNS resolution failures
- **Storage** — unbound PVCs, volume mount failures
- **Custom Rules** — define your own threshold-based alert rules in `configs/config.yaml`

---

## ⚖️ Comparison

| Feature | VisualEyes | Prometheus + Grafana | Datadog | Robusta |
|---------|-----------|---------------------|---------|---------|
| System + K8s metrics | ✅ | ✅ | ✅ | ✅ K8s only |
| AI-powered RCA | ✅ Claude | ❌ | ❌ | Partial |
| Interactive TUI CLI | ✅ veye | ❌ | ❌ | ❌ |
| WebSocket live stream | ✅ | ❌ | ✅ | ❌ |
| Prometheus compatible | ✅ | ✅ | ✅ | ✅ |
| Self-hosted | ✅ | ✅ | ❌ SaaS | ✅ |
| Safe remediation | ✅ | ❌ | ❌ | Partial |
| Cost | Free / OSS | Free / OSS | $$$$ | Free/Paid |

---

## ⚙️ Configuration

| Source | Description |
|--------|-------------|
| `configs/config.yaml` | Default config — collection interval, endpoints, alert rules |
| `.env` | Secrets — `ANTHROPIC_API_KEY`, `DATABASE_URL` |
| `deployments/kubernetes/config.yaml` | In-cluster ConfigMap overrides |
| Environment variables | Override any key — e.g., `VISUAL_EYES_AGENT_COLLECTION_INTERVAL=5s` |

---

## 🧑‍💻 Development

```bash
make build      # Build all binaries (server, agents, veye)
make test       # Run Go tests
make fmt        # Format Go code
make lint       # Run golangci-lint
make clean      # Remove build artifacts
make cross      # Cross-compile all binaries → dist/ (5 platforms)
```

---

## 📁 Project Structure

```
VisualEyes/
├-- agents/
│   ├-- system/          # Host metrics agent — CPU, mem, disk, net, load
│   └-- kubernetes/      # K8s agent — kubelet API, pod/node metrics, events
├-- backend/
│   ├-- alerts/          # Alert engine: rule eval, dedup, noise filter
│   ├-- api/             # HTTP handlers, routes, middleware
│   ├-- rca/             # AI RCA: Claude client, context builder, executor
│   ├-- storage/         # Interface, PostgreSQL (GORM), in-memory
│   └-- ws/              # WebSocket broadcaster
├-- cli/
│   └-- cmd/             # veye commands: status, alerts, logs, rca, watch
├-- configs/             # Default YAML config
├-- deployments/
│   └-- kubernetes/      # RBAC, ConfigMap, DaemonSet
├-- docs/images/         # Screenshots, architecture diagrams
├-- ui/                  # React 19 + MUI 7 + Vite dashboard
└-- docker-compose.yml
```

---

## 📍 Roadmap

- [x] System metrics agent (CPU, mem, disk, net, load)
- [x] Kubernetes DaemonSet agent (kubelet API, pod/node metrics)
- [x] Backend with alert engine, RCA processor, WebSocket, Prometheus
- [x] PostgreSQL persistent storage
- [x] React dashboard with dark/light theme, live updates
- [x] AI-powered RCA with Claude — safe remediation command execution
- [x] veye CLI — interactive Bubbletea TUI (status, alerts, logs, rca, watch)
- [x] GitHub Actions CI/CD + cross-platform release pipeline
- [ ] Distributed tracing integration (OpenTelemetry)
- [ ] Slack / PagerDuty alert routing
- [ ] Multi-cluster support in the web dashboard
- [ ] Custom runbook library (YAML-based, embeddable)
- [ ] eBPF network flow visibility

---

## 🤝 Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

```bash
git checkout -b feature/your-feature
make test && make lint
git commit -m "feat: describe your change"
git push origin feature/your-feature
# open a pull request — template will guide you
```

---

## 📄 License

MIT — see [LICENSE](LICENSE).
