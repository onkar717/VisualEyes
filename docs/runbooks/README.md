# Runbooks

YAML runbooks for common Kubernetes and system failure modes. VisualEyes AI RCA references these during incident diagnosis to provide structured remediation steps.

## Available Runbooks

| Runbook | Severity | Description |
|---------|----------|-------------|
| [crashloopbackoff.yaml](crashloopbackoff.yaml) | SEV1 | Pod repeatedly crashes — startup failure, missing config, bad image |
| [oomkilled.yaml](oomkilled.yaml) | SEV2 | Container exceeded memory limit |
| [high-cpu.yaml](high-cpu.yaml) | SEV3 | CPU throttling — limit too low or hot-path inefficiency |
| [imagepullbackoff.yaml](imagepullbackoff.yaml) | SEV2 | Image pull failure — bad tag, missing secret, registry down |
| [disk-pressure.yaml](disk-pressure.yaml) | SEV1 | Node disk usage critical — kubelet may evict pods |
| [node-not-ready.yaml](node-not-ready.yaml) | SEV1 | Node not in Ready state — kubelet crash, OOM, network partition |
| [pending-pods.yaml](pending-pods.yaml) | SEV2 | Pods stuck in Pending — insufficient resources, taint, affinity |

## Runbook Format

Each runbook is a YAML file with this structure:

```yaml
name: string               # Human-readable name
description: string        # One-line description
severity: SEV1|SEV2|SEV3|SEV4
tags: [list, of, tags]

symptoms:                  # What you observe
  - string

triage_steps:
  - step: int
    description: string    # What this step checks
    command: string        # kubectl / shell command (use {placeholders})
    safe: bool             # true = read-only, false = may modify state

common_causes:
  - cause: string
    signal: string         # What you see in logs/events that indicates this cause
    fix: string            # How to resolve it

remediation:
  - scenario: string
    command: string
    destructive: bool      # true = deletes/patches/scales something
    confirm_required: bool

monitoring:                # Optional — metric thresholds for proactive alerting
  - metric: string
    threshold: string
    action: string
```

## Adding a Custom Runbook

1. Create a new YAML file in `docs/runbooks/` following the format above
2. Add it to the table in this README
3. The AI RCA engine picks up all runbooks in this directory automatically

**Placeholder convention:** Use `{namespace}`, `{pod}`, `{deployment}`, `{node}`, `{container}` in commands — the RCA engine substitutes real values from the incident context before displaying to the user.

## Contributing Runbooks

Runbook contributions are welcome. Good runbooks have:
- Concrete `signal` descriptions quoting exact error messages or kubectl output patterns
- `safe: true` on all read-only triage steps
- `destructive: true` + `confirm_required: true` on anything that changes state
- A `monitoring` section with a proactive threshold where applicable
