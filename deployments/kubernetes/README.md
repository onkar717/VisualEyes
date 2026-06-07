# Kubernetes Deployment

Manifests for deploying the VisualEyes Kubernetes agent as a DaemonSet in your cluster.

## Files

| File | Kind | Description |
|------|------|-------------|
| `rbac.yaml` | ServiceAccount, ClusterRole, ClusterRoleBinding | Least-privilege access to kubelet and K8s APIs |
| `config.yaml` | ConfigMap | Agent configuration — backend endpoint, collection interval |
| `agent.yaml` | DaemonSet | One agent pod per node; runs in `kube-system` namespace |

## Apply Order

Always apply in this order — DaemonSet needs the ServiceAccount and ConfigMap to exist first:

```bash
kubectl apply -f deployments/kubernetes/rbac.yaml
kubectl apply -f deployments/kubernetes/config.yaml
kubectl apply -f deployments/kubernetes/agent.yaml
```

Remove:

```bash
kubectl delete -f deployments/kubernetes/
```

Or via Makefile:

```bash
make deploy-k8s
make undeploy-k8s
make status-k8s
```

## Configuration

Edit `config.yaml` before applying. Key values:

| Key | Description |
|-----|-------------|
| `VISUAL_EYES_OUTPUT_REMOTE_ENDPOINT` | URL where agent pushes metrics — must be reachable from inside the cluster |
| `VISUAL_EYES_AGENT_COLLECTION_INTERVAL` | How often to collect and push (e.g. `15s`) |

## Reaching the Backend from Inside the Cluster

The backend typically runs outside the cluster during development. Get the host IP:

**minikube:**

```bash
minikube ssh -- ip route | grep default | awk '{print $3}'
```

**kind:**

```bash
docker inspect kind-control-plane | grep '"Gateway"'
```

Set that IP in `config.yaml`:

```yaml
data:
  VISUAL_EYES_OUTPUT_REMOTE_ENDPOINT: "http://<host-ip>:8080/api/kubernetes-metrics"
```

## RBAC Permissions

The ClusterRole grants read-only access to:

- `nodes`, `pods`, `namespaces` — for listing cluster resources
- `nodes/stats`, `nodes/proxy` — for kubelet summary API access (CPU/memory per pod and node)

No write permissions are requested.

## Verify

```bash
# Check pod status
kubectl get pods -n kube-system -l app=visual-eyes-k8s-agent

# Check logs
kubectl logs -n kube-system -l app=visual-eyes-k8s-agent -f

# Check config
kubectl get configmap -n kube-system visual-eyes-agent-config -o yaml
```
