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

</div>

# VisualEyes

[![CI](https://github.com/onkar717/VisualEyes/actions/workflows/ci.yaml/badge.svg)](https://github.com/onkar717/VisualEyes/actions/workflows/ci.yaml)
[![Release](https://github.com/onkar717/VisualEyes/actions/workflows/release.yml/badge.svg)](https://github.com/onkar717/VisualEyes/releases)
[![Go Version](https://img.shields.io/badge/go-1.24-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**VisualEyes** is an open-source, cloud-native observability platform for system and Kubernetes monitoring. It combines real-time metrics collection, AI-powered Root Cause Analysis (RCA), alerting, and a live CLI dashboard — giving on-call engineers full visibility from a single tool.

---

## Overview

VisualEyes operates in two modes:

| Mode | What it does |
|------|-------------|
| **veye CLI** | Interactive terminal dashboard — live metrics, alerts, logs, and RCA from your machine |
| **Hub + Agents** | Deployed in-cluster agents push metrics to a central backend with a full React web UI |

---

## Features

- **System metrics** — CPU, memory, disk, network, load average (via `gopsutil`)
- **Kubernetes metrics** — pod-level and node-level stats via kubelet summary API
- **AI-powered RCA** — Claude AI diagnoses incidents and suggests safe remediation commands
- **Alert engine** — configurable rules with dedup, noise filtering, and auto-RCA trigger
- **WebSocket streaming** — live metric push to dashboard, no polling
- **Prometheus `/metrics`** — compatible with any Grafana/Prometheus stack
- **veye CLI** — Bubbletea interactive TUI: `status`, `alerts`, `logs`, `rca`, `watch`
- **PostgreSQL storage** — persistent incident history with MTTR tracking
- **Docker & Kubernetes** — full containerized deployment with manifests and Compose

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                        VisualEyes                            │
│                                                              │
│  ┌─────────────────┐    ┌─────────────────────────────────┐  │
│  │  System Agent   │    │      Kubernetes Agent           │  │
│  │  (gopsutil)     │    │  (kubelet summary API)          │  │
│  │  CPU/Mem/Disk   │    │  Pod & Node metrics             │  │
│  │  Net/Load       │    │  Events + Logs                  │  │
│  └────────┬────────┘    └────────────┬────────────────────┘  │
│           │  POST /api/system-metrics │ POST /api/k8s-metrics  │
│           └──────────────┬───────────┘                       │
│                          ▼                                    │
│           ┌──────────────────────────┐                       │
│           │      Backend Server      │                       │
│           │  Go HTTP · port 8080     │                       │
│           │  Alert Engine            │                       │
│           │  RCA Processor (Claude)  │                       │
│           │  WebSocket Broadcaster   │                       │
│           │  Prometheus Registry     │                       │
│           │  PostgreSQL / MemStore   │                       │
│           └────────┬─────────────────┘                       │
│                    │                                          │
│          ┌─────────┴──────────┐                              │
│          ▼                    ▼                              │
│  ┌───────────────┐   ┌─────────────────────┐                │
│  │  React UI     │   │  veye CLI (TUI)      │               │
│  │  port 5173    │   │  status / alerts     │               │
│  │  Dark/Light   │   │  logs / rca / watch  │               │
│  └───────────────┘   └─────────────────────┘                │
└──────────────────────────────────────────────────────────────┘
```

---

## Install (Pre-built Binaries)

No Go required. Download the latest binary:

```bash
# Install veye CLI
curl -fsSL https://raw.githubusercontent.com/onkar717/VisualEyes/main/install.sh | bash

# Install server binary
curl -fsSL https://raw.githubusercontent.com/onkar717/VisualEyes/main/install.sh | bash -s visual-eyes
```

Or grab a binary directly from [Releases](https://github.com/onkar717/VisualEyes/releases).

---

## Quick Start

### Prerequisites

- Go 1.24+
- Node.js 18+
- Docker & Docker Compose
- PostgreSQL 14+ (or use Docker Compose)
- `kubectl` + a Kubernetes cluster (for K8s mode)

### 1. Clone & Build

```bash
git clone https://github.com/onkar717/visual-eyes.git
cd visual-eyes

# Install Go dependencies
make deps

# Build all components (server, system agent, k8s agent, veye CLI)
make build

# Install UI dependencies
make install-ui
```

### 2. Configure

```bash
cp .env.example .env
# Edit .env — set ANTHROPIC_API_KEY for AI RCA, DATABASE_URL for PostgreSQL
```

### 3. Run Everything Locally

```bash
# Start the backend server (port 8080)
./bin/visual-eyes-server

# Start the system metrics agent (separate terminal)
./bin/visual-eyes-agent

# Start the React UI dev server (port 5173)
make run-ui

# Open the dashboard
open http://localhost:5173
```

### 4. Use the veye CLI

```bash
# Live status
./bin/veye status

# View active alerts
./bin/veye alerts

# Stream logs from a pod
./bin/veye logs --follow

# Show RCA for an incident
./bin/veye rca

# Interactive TUI dashboard
./bin/veye watch
```

---

## Docker Compose (Full Stack)

```bash
docker-compose up --build -d
```

Services started:
- `backend` → port 8080
- `ui` → port 3000
- `postgres` → port 5432
- `system-agent` → pushes host metrics

---

## Kubernetes Deployment

```bash
# Apply RBAC, config, and DaemonSet
kubectl apply -f deployments/kubernetes/rbac.yaml
kubectl apply -f deployments/kubernetes/config.yaml
kubectl apply -f deployments/kubernetes/agent.yaml

# Verify
kubectl get pods -n kube-system -l app=visual-eyes-k8s-agent
```

For detailed Kubernetes setup including minikube and kind, see [INSTALLATION.md](INSTALLATION.md).

---

## Configuration

| Source | Description |
|--------|-------------|
| `configs/config.yaml` | Default config — collection interval, endpoints |
| `.env` | Secrets — `ANTHROPIC_API_KEY`, `DATABASE_URL` |
| `deployments/kubernetes/config.yaml` | In-cluster ConfigMap overrides |
| Environment variables | Override any config key — e.g., `VISUAL_EYES_AGENT_COLLECTION_INTERVAL=5s` |

---

## Development

```bash
make build      # Build all binaries
make test       # Run tests
make fmt        # Format Go code
make lint       # Run golangci-lint
make clean      # Remove build artifacts
make cross      # Cross-compile for all platforms (outputs to dist/)
```

---

## Project Structure

```
visual-eyes/
├── agents/
│   ├── system/          # System metrics agent (CPU, mem, disk, net, load)
│   └── kubernetes/      # Kubernetes metrics agent (kubelet API)
├── backend/
│   ├── alerts/          # Alert engine — rules, dedup, noise filter
│   ├── api/             # HTTP handlers, routes, middleware
│   ├── config/          # Config loading
│   ├── metrics/         # Prometheus registry
│   ├── models/          # Data models
│   ├── rca/             # AI RCA engine (Claude client, context builder)
│   ├── storage/         # Storage interface, PostgreSQL, in-memory
│   └── ws/              # WebSocket broadcaster
├── cli/
│   └── cmd/             # veye CLI commands (status, alerts, logs, rca, watch)
├── configs/             # Default YAML configuration
├── deployments/
│   └── kubernetes/      # RBAC, ConfigMap, DaemonSet manifests
├── docs/
│   └── images/          # Screenshots and architecture diagrams
├── ui/                  # React + MUI + Vite frontend
└── docker-compose.yml   # Full stack local deployment
```

---

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

```bash
git checkout -b feature/your-feature
# make changes
make test && make lint
git commit -m "feat: your feature"
git push origin feature/your-feature
# open a pull request
```

---

## License

MIT — see [LICENSE](LICENSE).
