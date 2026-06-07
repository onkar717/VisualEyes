# Security Policy

## Supported Versions

| Version | Security Updates |
|---------|----------------|
| 1.1.x | ✅ Active |
| 1.0.x | ✅ Critical fixes only |
| < 1.0 | ❌ Not supported |

---

## Reporting a Vulnerability

**Do not report security vulnerabilities through public GitHub issues.**

Open a [GitHub Security Advisory](https://github.com/onkar717/VisualEyes/security/advisories/new) (private, only visible to maintainers).

Include:

- **Type**   e.g. command injection, SSRF, auth bypass, path traversal
- **Component**   backend / system agent / k8s agent / veye CLI / UI
- **Affected version(s)**   tag, branch, or commit SHA
- **Reproduction steps**   minimal steps to trigger the issue
- **Impact**   what an attacker can achieve
- **Proof-of-concept**   code or curl command if available (will be kept confidential)

You will receive an acknowledgement within **48 hours** and a resolution timeline within **7 days** of confirmation.

---

## Scope

| In Scope | Out of Scope |
|----------|-------------|
| Command injection in RCA executor | Issues in dependencies we don't control |
| SSRF via agent endpoint config | Social engineering attacks |
| Path traversal in log collection | Vulnerabilities in already-patched versions |
| Privilege escalation in K8s RBAC | DoS via resource exhaustion on test systems |
| Secrets leaking in API responses | Scanner findings with no demonstrated impact |

---

## Security Model

### RCA Command Executor

The RCA engine executes `kubectl` commands suggested by Claude AI. Two safeguards prevent misuse:

1. **Allowlist enforcement**   only `kubectl get`, `describe`, `logs`, `top`, `rollout status`, `get events` are permitted without explicit confirmation
2. **Destructive flag**   commands touching state (`delete`, `patch`, `scale`, `set`) are marked `is_destructive: true` and require explicit user confirmation (`y` in CLI) before execution

The Claude API prompt explicitly instructs the model to only suggest safe, read-first diagnostic commands.

### Kubernetes RBAC

The DaemonSet's ClusterRole requests **read-only** access:

```yaml
rules:
- apiGroups: [""]
  resources: ["nodes", "pods", "namespaces"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["nodes/stats", "nodes/proxy"]
  verbs: ["get"]
```

No `create`, `update`, `delete`, or `patch` verbs are requested.

### Secrets Handling

- `ANTHROPIC_API_KEY` and `DATABASE_URL` must be set via environment variables or `.env` file   never hardcoded
- The `.env` file is in `.gitignore`   `.env.example` contains only placeholders
- In Kubernetes, use a `Secret` object and reference it as environment variables; do not embed secrets in ConfigMaps

---

## Hardening Checklist (Production)

### Backend

- [ ] Run behind a reverse proxy (nginx/Caddy) with TLS
- [ ] Add authentication layer (e.g. OAuth2 proxy, API tokens) in front of `/api/*` and `/ws`
- [ ] Bind to `127.0.0.1` locally; use ingress for external access
- [ ] Set `DATABASE_URL` with `sslmode=require`
- [ ] Rotate `ANTHROPIC_API_KEY` periodically

### Docker

- [ ] Use tagged (not `latest`) image versions in production
- [ ] Run containers as non-root (`USER 1000`   already set in provided Dockerfiles)
- [ ] Scan images with `docker scout` or Trivy before deployment
- [ ] Use read-only root filesystem where possible

### Kubernetes

- [ ] Deploy agent in a dedicated namespace, not `kube-system`, in production
- [ ] Apply NetworkPolicy to restrict pod egress to backend only
- [ ] Use `PodSecurityAdmission` with `restricted` profile
- [ ] Store secrets in a secrets manager (Vault, AWS Secrets Manager, Sealed Secrets)
- [ ] Review and restrict RBAC to the minimum required for your cluster

---

## Disclosure Timeline

| Day | Action |
|-----|--------|
| 0 | Report received |
| 1–2 | Acknowledgement sent to reporter |
| 3–7 | Vulnerability confirmed or rejected with explanation |
| 7–21 | Fix developed, tested, and merged to private branch |
| 21–30 | Patched release published, CVE requested if applicable |
| 30 | Public disclosure (coordinated with reporter) |
