# Kubernetes Agent

Collects pod-level and node-level metrics from the Kubernetes kubelet summary API and pushes them to the VisualEyes backend.

## Metrics Collected

| Metric | Source |
|--------|--------|
| Node CPU / memory / network | kubelet `/stats/summary` |
| Pod CPU / memory per container | kubelet `/stats/summary` |
| Pod status and restart counts | Kubernetes API |
| Kubernetes events | `kubectl get events` equivalent |
| Pod logs (last N lines) | kubelet log API |

## Deployment

The agent runs as a **DaemonSet** in `kube-system`, one pod per node.

```bash
kubectl apply -f deployments/kubernetes/rbac.yaml
kubectl apply -f deployments/kubernetes/config.yaml
kubectl apply -f deployments/kubernetes/agent.yaml
```

## Configuration

Set in `deployments/kubernetes/config.yaml` (ConfigMap) or as environment variables:

| Env Variable | Default | Description |
|-------------|---------|-------------|
| `VISUAL_EYES_OUTPUT_REMOTE_ENDPOINT` | `http://localhost:8080/api/kubernetes-metrics` | Backend push URL |
| `VISUAL_EYES_AGENT_COLLECTION_INTERVAL` | `15s` | Metrics push interval |
| `VISUAL_EYES_AGENT_DISABLE_KUBE_METRICS` | `false` | Disable K8s collection |

## Host Access (minikube / kind)

The agent pod needs to reach the backend on the host machine. Get the host IP:

```bash
# minikube
minikube ssh -- ip route | grep default | awk '{print $3}'

# kind
docker inspect kind-control-plane | grep '"Gateway"'
```

Set `VISUAL_EYES_OUTPUT_REMOTE_ENDPOINT=http://<host-ip>:8080/api/kubernetes-metrics` in the ConfigMap.

## Verify

```bash
kubectl get pods -n kube-system -l app=visual-eyes-k8s-agent
kubectl logs -n kube-system -l app=visual-eyes-k8s-agent -f
```
