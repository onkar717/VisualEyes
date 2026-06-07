# API Reference

Full reference for the VisualEyes backend REST API and WebSocket endpoint.

**Base URL:** `http://localhost:8080`

---

## Table of Contents

- [Authentication](#authentication)
- [System Metrics](#system-metrics)
- [Kubernetes Metrics](#kubernetes-metrics)
- [Alerts](#alerts)
- [RCA](#rca)
- [Logs](#logs)
- [Health & Observability](#health--observability)
- [WebSocket](#websocket)

---

## Authentication

No authentication is required by default. For production deployments, place the backend behind a reverse proxy (nginx, Caddy) with your preferred auth mechanism.

---

## System Metrics

### POST /api/system-metrics

Ingest a system metrics payload from the system agent.

**Request body:**

```json
{
  "cpu_percent": 23.4,
  "cpu_per_core": [18.2, 29.1, 22.0, 24.4],
  "mem_used_bytes": 6553600000,
  "mem_total_bytes": 17179869184,
  "mem_percent": 38.1,
  "swap_used_bytes": 0,
  "swap_total_bytes": 2147483648,
  "disk_used_bytes": 45000000000,
  "disk_total_bytes": 107374182400,
  "disk_percent": 41.9,
  "net_bytes_sent": 1048576,
  "net_bytes_recv": 2097152,
  "load_1m": 0.87,
  "load_5m": 0.72,
  "load_15m": 0.65,
  "timestamp": "2026-06-07T10:30:00Z"
}
```

**Response:** `200 OK`

```json
{ "status": "accepted" }
```

---

### GET /api/metrics/snapshot

Get the latest system metrics snapshot.

**Response:** `200 OK`

```json
{
  "cpu_percent": 23.4,
  "mem_used_gb": 6.1,
  "mem_total_gb": 16.0,
  "disk_percent": 41.9,
  "load_1m": 0.87,
  "timestamp": "2026-06-07T10:30:00Z"
}
```

---

## Kubernetes Metrics

### POST /api/kubernetes-metrics

Ingest a Kubernetes metrics payload from the Kubernetes agent.

**Request body:**

```json
{
  "node_name": "minikube",
  "node_cpu_percent": 18.2,
  "node_mem_used_bytes": 3221225472,
  "node_mem_total_bytes": 8589934592,
  "pods": [
    {
      "name": "payment-service-84f9b8c-x2z9",
      "namespace": "prod",
      "cpu_usage_nano": 125000000,
      "mem_usage_bytes": 52428800,
      "status": "CrashLoopBackOff",
      "restart_count": 14,
      "containers": [...]
    }
  ],
  "timestamp": "2026-06-07T10:30:05Z"
}
```

**Response:** `200 OK`

```json
{ "status": "accepted" }
```

---

### GET /api/kubernetes/metrics

Get the latest Kubernetes metrics snapshot.

**Response:** `200 OK`

```json
{
  "node_name": "minikube",
  "node_cpu_percent": 18.2,
  "node_mem_used_gb": 3.1,
  "pod_count": 13,
  "pods": [...],
  "timestamp": "2026-06-07T10:30:05Z"
}
```

---

## Alerts

### GET /api/alerts

List all active alerts.

**Query parameters:**

| Param | Type | Description |
|-------|------|-------------|
| `severity` | string | Filter by severity: `SEV1`, `SEV2`, `SEV3`, `SEV4` |
| `resolved` | bool | `true` to include resolved alerts |
| `limit` | int | Max results (default: 100) |

**Response:** `200 OK`

```json
[
  {
    "id": "a1b2c3",
    "severity": "SEV1",
    "resource": "payment-service",
    "namespace": "prod",
    "reason": "CrashLoopBackOff",
    "message": "Pod has restarted 14 times in the last 10 minutes",
    "fired_at": "2026-06-07T10:28:00Z",
    "resolved_at": null,
    "rca_id": "r9x8y7"
  }
]
```

---

### GET /api/alerts/:id

Get a single alert by ID.

**Response:** `200 OK`   same shape as single element above.

**Error:** `404 Not Found`

```json
{ "error": "alert not found" }
```

---

## RCA

### GET /api/rca

List all RCA results.

**Query parameters:**

| Param | Type | Description |
|-------|------|-------------|
| `incident_id` | string | Filter by alert/incident ID |
| `limit` | int | Max results (default: 50) |

**Response:** `200 OK`

```json
[
  {
    "id": "r9x8y7",
    "incident_id": "a1b2c3",
    "root_cause": "payment-service cannot connect to Redis at redis.prod.svc:6379   TCP connection refused.",
    "contributing_factors": [
      "Redis service has 0 ready endpoints (selector mismatch)",
      "redis-0 pod is Pending   PVC not bound"
    ],
    "confidence": 0.94,
    "steps": [
      {
        "step_number": 1,
        "description": "Check Redis pod status",
        "command": "kubectl get pods -n prod -l app=redis",
        "is_destructive": false
      },
      {
        "step_number": 2,
        "description": "Describe unbound PVC",
        "command": "kubectl describe pvc redis-data -n prod",
        "is_destructive": false
      }
    ],
    "created_at": "2026-06-07T10:28:30Z"
  }
]
```

---

### GET /api/rca/:id

Get a single RCA result by ID.

**Response:** `200 OK`   same shape as single element above.

---

## Logs

### GET /api/logs

List collected pod logs.

**Query parameters:**

| Param | Type | Description |
|-------|------|-------------|
| `namespace` | string | Filter by namespace |
| `pod` | string | Filter by pod name |
| `container` | string | Filter by container name |
| `limit` | int | Max log lines (default: 200) |

**Response:** `200 OK`

```json
[
  {
    "id": "l1m2n3",
    "namespace": "prod",
    "pod": "payment-service-84f9b8c-x2z9",
    "container": "payment-service",
    "log_line": "Error: ECONNREFUSED   Redis connection failed",
    "timestamp": "2026-06-07T10:27:58Z"
  }
]
```

---

## Health & Observability

### GET /health

Liveness check. Returns `200 OK` when the server is up.

```json
{ "status": "ok", "version": "v1.1.0" }
```

### GET /metrics

Prometheus metrics endpoint. Exposes all registered VisualEyes metrics in Prometheus text format.

```
# HELP visual_eyes_cpu_percent Current CPU usage percentage
# TYPE visual_eyes_cpu_percent gauge
visual_eyes_cpu_percent 23.4

# HELP visual_eyes_alerts_total Total alerts fired since startup
# TYPE visual_eyes_alerts_total counter
visual_eyes_alerts_total{severity="SEV1"} 3
visual_eyes_alerts_total{severity="SEV2"} 7
```

---

## WebSocket

### GET /ws (Upgrade)

WebSocket endpoint for live metric streaming. Clients receive all metric, alert, and RCA events as they happen.

**Connect:**

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');
ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  console.log(msg.type, msg.payload);
};
```

**Message types:** See [docs/ARCHITECTURE.md   WebSocket Protocol](ARCHITECTURE.md#websocket-protocol) for full message schemas.

| `type` | Trigger |
|--------|---------|
| `system_metrics` | System agent pushes metrics |
| `kubernetes_metrics` | K8s agent pushes metrics |
| `alert` | Alert engine fires a new alert |
| `alert_resolved` | Alert condition clears |
| `rca_result` | RCA pipeline completes for an incident |
| `snapshot` | Sent on connect   current state of all metrics/alerts |

**Reconnection:** Clients should implement exponential backoff reconnect. The backend does not persist connection state between reconnects   a new `snapshot` message is sent on each fresh connection.
