# Architecture Deep-Dive

This document covers VisualEyes internals: data flow, component responsibilities, alert lifecycle, RCA pipeline, WebSocket protocol, and storage schema.

---

## Table of Contents

- [Component Overview](#component-overview)
- [Data Flow](#data-flow)
- [Alert Lifecycle](#alert-lifecycle)
- [RCA Pipeline](#rca-pipeline)
- [WebSocket Protocol](#websocket-protocol)
- [Storage Schema](#storage-schema)
- [Configuration Hierarchy](#configuration-hierarchy)

---

## Component Overview

```
agents/system        →  host metrics (gopsutil)
agents/kubernetes    →  cluster metrics (kubelet API)
backend              →  central hub (API, alerts, RCA, WS, Prometheus)
cli                  →  veye TUI (Bubbletea, Cobra)
ui                   →  React dashboard (MUI, Vite, React Query)
```

Each component is an independent Go binary. They communicate only through the backend's HTTP API and WebSocket endpoint. No shared memory, no direct agent-to-UI path.

---

## Data Flow

### System Metrics Path

```
visual-eyes-agent
  └─ gopsutil: collect CPU/mem/disk/net/load every 10s
  └─ HTTP POST /api/system-metrics  →  backend
       └─ storage.Store(SystemMetricRecord)
       └─ alertEngine.Evaluate(metrics)
       └─ ws.Broadcaster.Publish(metrics)  →  connected WebSocket clients
       └─ prometheus.Registry.Update(metrics)
```

### Kubernetes Metrics Path

```
visual-eyes-kube-agent
  └─ kubelet /stats/summary: CPU + mem per pod + node every 15s
  └─ K8s API: pod status, restart counts, events
  └─ HTTP POST /api/kubernetes-metrics  →  backend
       └─ storage.Store(KubernetesMetricRecord)
       └─ alertEngine.Evaluate(k8sMetrics)
       └─ ws.Broadcaster.Publish(k8sMetrics)
```

### veye CLI Read Path

```
veye status / alerts / logs / rca
  └─ HTTP GET /api/metrics/snapshot
  └─ HTTP GET /api/alerts
  └─ HTTP GET /api/logs
  └─ HTTP GET /api/rca
  └─ render in Bubbletea TUI
```

### React UI Live Path

```
React UI  →  WebSocket ws://localhost:8080/ws
  └─ on connect: sends initial state snapshot
  └─ on each agent push: backend broadcasts delta
  └─ React Query polls REST endpoints for alerts/RCA/logs
```

---

## Alert Lifecycle

```
1. Agent pushes metrics to POST /api/*-metrics

2. alertEngine.Evaluate(payload)
   ├─ Load rules from configs/config.yaml
   ├─ Evaluate each rule against current metric values
   ├─ Dedup check: is this rule already firing for this resource?
   │   └─ yes → update timestamp, skip re-fire
   │   └─ no  → create new Alert{ID, Severity, Resource, Reason, FiredAt}
   ├─ Noise filter: suppress if metric recovers within grace period
   └─ Persist via storage.StoreAlert(alert)

3. If alert.Severity <= SEV2 (configurable):
   └─ rcaProcessor.TriggerAsync(alert)  →  RCA pipeline (async goroutine)

4. ws.Broadcaster.Publish(AlertEvent)
   └─ React UI updates Alerts panel immediately
   └─ veye watch updates TUI alerts list
```

**Alert severity levels:**

| Level | Meaning | Auto-RCA |
|-------|---------|---------|
| SEV1 | Critical — service down | ✅ |
| SEV2 | High — degraded | ✅ |
| SEV3 | Medium — warning threshold | configurable |
| SEV4 | Low — informational | ❌ |

---

## RCA Pipeline

The RCA processor runs entirely in `backend/rca/`. Steps:

```
1. context_builder.Build(alert)
   ├─ Pull last N metric snapshots for affected resource
   ├─ Pull recent pod logs for affected pod (if K8s alert)
   ├─ Pull recent events for affected namespace
   └─ Serialize into structured context string

2. claude_client.Analyze(context)
   ├─ System prompt: SRE role, safety constraints, output format
   ├─ User message: structured incident context
   ├─ Claude API call (claude-sonnet-4-6 or configured model)
   └─ Parse response: RootCause, ContributingFactors, RemediationSteps[]

3. executor.Validate(remediationSteps)
   ├─ Each command checked against allowlist (kubectl get/describe/logs/top)
   ├─ Destructive commands (delete, patch, scale 0) flagged is_destructive=true
   └─ Commands not in allowlist → rejected, logged

4. storage.StoreRCAResult(result)
   └─ Persisted with incident_id, root_cause, steps[], confidence, created_at

5. ws.Broadcaster.Publish(RCAEvent)
   └─ React UI opens RCA drawer
   └─ veye watch surfaces RCA in TUI
```

**Allowlisted safe commands:**

```
kubectl get, kubectl describe, kubectl logs, kubectl top
kubectl rollout status, kubectl get events
```

**Flagged (require explicit confirm):**

```
kubectl delete, kubectl patch, kubectl scale
kubectl rollout restart, kubectl set
```

---

## WebSocket Protocol

Backend WebSocket endpoint: `ws://localhost:8080/ws`

### Message Types

All messages are JSON with a `type` discriminator:

```json
// System metrics update
{
  "type": "system_metrics",
  "payload": {
    "cpu_percent": 23.4,
    "mem_used_gb": 6.1,
    "mem_total_gb": 16.0,
    "disk_used_percent": 42.1,
    "load_1m": 0.87,
    "timestamp": "2026-06-07T10:30:00Z"
  }
}

// Alert fired
{
  "type": "alert",
  "payload": {
    "id": "a1b2c3",
    "severity": "SEV1",
    "resource": "payment-service",
    "reason": "CrashLoopBackOff",
    "fired_at": "2026-06-07T10:28:00Z"
  }
}

// RCA result ready
{
  "type": "rca_result",
  "payload": {
    "incident_id": "a1b2c3",
    "root_cause": "...",
    "confidence": 0.94,
    "steps": [...]
  }
}

// Kubernetes metrics update
{
  "type": "kubernetes_metrics",
  "payload": {
    "node_cpu_percent": 18.2,
    "node_mem_used_gb": 3.1,
    "pod_count": 13,
    "timestamp": "2026-06-07T10:30:05Z"
  }
}
```

### Connection Lifecycle

```
Client connect  →  ws.Hub.Register(client)
                →  send last known state snapshot (all message types)
Agent push      →  ws.Hub.Broadcast(message)  →  all registered clients
Client close    →  ws.Hub.Unregister(client)
```

---

## Storage Schema

Both storage backends implement the same `storage.Store` interface (`backend/storage/interface.go`).

### In-Memory

Stores everything in Go slices + maps. Zero config. All data lost on restart.

```go
type MemoryStore struct {
    systemMetrics     []models.MetricRecord
    kubernetesMetrics []models.MetricRecord
    alerts            map[string]models.Alert
    rcaResults        map[string]models.RCAResult
    podLogs           []models.PodLog
    mu                sync.RWMutex
}
```

### PostgreSQL (GORM)

Tables auto-migrated on startup. Set `DATABASE_URL` to activate.

| Table | Model | Key Fields |
|-------|-------|-----------|
| `metric_records` | `MetricRecord` | `id`, `type` (system/k8s), `payload` JSONB, `created_at` |
| `alerts` | `Alert` | `id`, `severity`, `resource`, `reason`, `fired_at`, `resolved_at` |
| `rca_results` | `RCAResult` | `id`, `incident_id`, `root_cause`, `confidence`, `steps` JSONB, `created_at` |
| `pod_logs` | `PodLog` | `id`, `namespace`, `pod`, `container`, `log_line`, `timestamp` |

MTTR is computed as `AVG(resolved_at - fired_at)` across resolved alerts.

---

## Configuration Hierarchy

Resolution order (highest priority wins):

```
1. Environment variables        (VISUAL_EYES_*)
2. .env file                    (loaded at startup)
3. configs/config.yaml          (default config)
4. Compiled-in defaults         (fallback)
```

Config key → env var mapping uses `VISUAL_EYES_` prefix + uppercase key with `_` separating nested keys:

```yaml
# configs/config.yaml           →  env var
server:
  port: 8080                    →  VISUAL_EYES_SERVER_PORT
agent:
  collection_interval: 10s      →  VISUAL_EYES_AGENT_COLLECTION_INTERVAL
  output:
    remote_endpoint: http://... →  VISUAL_EYES_OUTPUT_REMOTE_ENDPOINT
```
