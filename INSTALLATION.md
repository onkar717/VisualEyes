# Installation Guide

This guide covers all deployment modes for VisualEyes.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Option A: Local Development](#option-a-local-development)
- [Option B: Docker Compose](#option-b-docker-compose)
- [Option C: Kubernetes (minikube)](#option-c-kubernetes-minikube)
- [Option D: Kubernetes (kind)](#option-d-kubernetes-kind)
- [Option E: Production Kubernetes](#option-e-production-kubernetes)
- [Configuration Reference](#configuration-reference)
- [Verifying the Installation](#verifying-the-installation)

---

## Prerequisites

| Tool | Minimum Version | Required For |
|------|----------------|-------------|
| Go | 1.24 | Building from source |
| Node.js | 18 | UI development |
| Docker | 24 | Container deployment |
| Docker Compose | v2 | Full stack local |
| kubectl | 1.26 | Kubernetes deployment |
| PostgreSQL | 14 | Persistent storage (optional   falls back to in-memory) |

---

## Option A: Local Development

### 1. Clone and Build

```bash
git clone https://github.com/onkar717/visual-eyes.git
cd visual-eyes
make deps
make build
make install-ui
```

### 2. Configure Environment

```bash
cp .env.example .env
```

Edit `.env`:

```env
# Required for AI RCA
ANTHROPIC_API_KEY=sk-ant-...

# Optional   omit to use in-memory storage
DATABASE_URL=postgres://user:pass@localhost:5432/visualeyes?sslmode=disable

# Backend server address (for agents and CLI)
VISUAL_EYES_SERVER_URL=http://localhost:8080
```

### 3. Start Components

Open three terminals:

**Terminal 1   Backend server:**

```bash
./bin/visual-eyes-server
# Listening on :8080
```

**Terminal 2   System agent:**

```bash
./bin/visual-eyes-agent
# Pushing metrics every 10s
```

**Terminal 3   UI:**

```bash
make run-ui
# http://localhost:5173
```

### 4. Optional: Kubernetes agent (local cluster)

```bash
./bin/visual-eyes-kube-agent
```

---

## Option B: Docker Compose

The fastest way to run the full stack.

```bash
git clone https://github.com/onkar717/visual-eyes.git
cd visual-eyes
cp .env.example .env
# Set ANTHROPIC_API_KEY in .env

docker-compose up --build -d
```

Services:

| Service | Port | Description |
|---------|------|-------------|
| backend | 8080 | API + WebSocket |
| ui | 3000 | React dashboard |
| postgres | 5432 | Persistent storage |
| system-agent |   | Host metrics push |

Check logs:

```bash
docker-compose logs -f backend
docker-compose logs -f system-agent
```

Stop:

```bash
docker-compose down
```

---

## Option C: Kubernetes (minikube)

### 1. Start minikube

```bash
minikube start --driver=docker
```

### 2. Get Host IP

The agents inside the cluster need to reach the backend running on your host.

```bash
minikube ssh -- ip route | grep default | awk '{print $3}'
# Example output: 192.168.49.1
```

### 3. Run Backend on Host

```bash
./bin/visual-eyes-server
```

### 4. Deploy Agent to Kubernetes

Edit `deployments/kubernetes/config.yaml` and set the endpoint:

```yaml
data:
  VISUAL_EYES_OUTPUT_REMOTE_ENDPOINT: "http://192.168.49.1:8080/api/kubernetes-metrics"
```

Apply manifests in order:

```bash
kubectl apply -f deployments/kubernetes/rbac.yaml
kubectl apply -f deployments/kubernetes/config.yaml
kubectl apply -f deployments/kubernetes/agent.yaml
```

Verify:

```bash
kubectl get pods -n kube-system -l app=visual-eyes-k8s-agent
kubectl logs -n kube-system -l app=visual-eyes-k8s-agent -f
```

---

## Option D: Kubernetes (kind)

```bash
kind create cluster --name visual-eyes
kubectl cluster-info --context kind-visual-eyes

# Get host IP accessible from inside kind
docker inspect kind-control-plane | grep '"Gateway"'
# Use that IP in deployments/kubernetes/config.yaml
```

Then follow the same apply steps as Option C.

---

## Option E: Production Kubernetes

### 1. Push Images to Registry

```bash
make build-docker-server
make push-docker-server

make build-docker-system-agent
make push-docker-system-agent
```

### 2. Update Image References

Edit `deployments/kubernetes/agent.yaml` to reference your registry images.

### 3. Create Namespace & Secrets

```bash
kubectl create namespace visual-eyes
kubectl create secret generic visual-eyes-secrets \
  --from-literal=ANTHROPIC_API_KEY=sk-ant-... \
  --from-literal=DATABASE_URL=postgres://... \
  -n visual-eyes
```

### 4. Apply Manifests

```bash
kubectl apply -f deployments/kubernetes/ -n visual-eyes
```

---

## Configuration Reference

All settings can be set via `configs/config.yaml` or environment variables. Environment variables take precedence.

| Config Key | Env Variable | Default | Description |
|------------|-------------|---------|-------------|
| `server.port` | `VISUAL_EYES_SERVER_PORT` | `8080` | Backend HTTP port |
| `agent.collection_interval` | `VISUAL_EYES_AGENT_COLLECTION_INTERVAL` | `10s` | Metrics push interval |
| `agent.output.remote_endpoint` | `VISUAL_EYES_OUTPUT_REMOTE_ENDPOINT` | `http://localhost:8080/api/system-metrics` | Backend push URL |
| `agent.disable_kube_metrics` | `VISUAL_EYES_AGENT_DISABLE_KUBE_METRICS` | `false` | Skip K8s metric collection |
| `agent.disable_host_metrics` | `VISUAL_EYES_AGENT_DISABLE_HOST_METRICS` | `false` | Skip system metric collection |
| `database.url` | `DATABASE_URL` |   | PostgreSQL connection string |
| `rca.anthropic_api_key` | `ANTHROPIC_API_KEY` |   | Claude API key for RCA |

---

## Verifying the Installation

### Check API health

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

### Check metrics snapshot

```bash
curl http://localhost:8080/api/metrics/snapshot | jq .
```

### Check Prometheus endpoint

```bash
curl http://localhost:8080/metrics | grep visual_eyes
```

### Check active alerts

```bash
./bin/veye alerts
```

### Run interactive dashboard

```bash
./bin/veye watch
```
