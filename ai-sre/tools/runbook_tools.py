"""
Runbook tools: load YAML runbooks and execute kubectl commands via the safe allowlist.
"""
import json
import logging
import os
import shlex
import subprocess
from typing import List

import yaml
from crewai.tools import tool

from ..config import config

logger = logging.getLogger("visualeyes.tools.runbooks")

# Mirrors the Go executor's isSafe() allowlist exactly.
SAFE_PREFIXES: List[str] = [
    "kubectl get ",
    "kubectl describe ",
    "kubectl logs ",
    "kubectl top ",
    "kubectl rollout status ",
    "kubectl delete pod ",
    "kubectl rollout restart ",
    "kubectl scale deployment/",
    "kubectl cordon ",
    "kubectl uncordon ",
    "kubectl set image ",
    "kubectl annotate ",
    "kubectl label ",
]

SHELL_METACHARS = ["&&", "||", ";", "|", ">", "<", "`", "$("]


def _is_safe(cmd: str) -> bool:
    trimmed = cmd.strip()
    for meta in SHELL_METACHARS:
        if meta in trimmed:
            return False
    for prefix in SAFE_PREFIXES:
        if trimmed.startswith(prefix):
            return True
    return False


def _load_all_runbooks() -> list:
    runbooks = []
    rb_dir = config.runbooks_dir
    if not os.path.isdir(rb_dir):
        logger.warning("runbooks dir not found: %s", rb_dir)
        return runbooks
    for fname in os.listdir(rb_dir):
        if fname.endswith((".yaml", ".yml")):
            path = os.path.join(rb_dir, fname)
            try:
                with open(path) as f:
                    data = yaml.safe_load(f)
                    data["_filename"] = fname
                    runbooks.append(data)
            except Exception as e:
                logger.warning("failed to load runbook %s: %s", fname, e)
    return runbooks


@tool
def list_available_runbooks(query: str = "") -> str:
    """
    List all available runbooks with their names, descriptions, and trigger categories.
    Use this FIRST to discover what runbooks exist before loading one.
    """
    runbooks = _load_all_runbooks()
    result = [{
        "filename": rb.get("_filename", "?"),
        "name": rb.get("name", "?"),
        "description": rb.get("description", ""),
        "categories": rb.get("categories", []),
        "triggers": rb.get("triggers", []),
    } for rb in runbooks]
    return json.dumps({"runbooks": result, "count": len(result)})


@tool
def load_runbook(filename: str) -> str:
    """
    Load a specific runbook by filename (e.g. "crashloop.yaml").
    Returns full runbook content including diagnosis steps and remediation plan.
    Format: load_runbook(filename="crashloop.yaml")
    """
    rb_dir = config.runbooks_dir
    path = os.path.join(rb_dir, filename)

    # Security: prevent path traversal
    resolved = os.path.realpath(path)
    if not resolved.startswith(os.path.realpath(rb_dir)):
        return json.dumps({"error": "Invalid filename path traversal not allowed"})

    if not os.path.exists(resolved):
        # Try without extension
        for ext in (".yaml", ".yml"):
            alt = os.path.join(rb_dir, filename + ext)
            if os.path.exists(alt):
                resolved = alt
                break
        else:
            return json.dumps({"error": f"Runbook not found: {filename}"})

    try:
        with open(resolved) as f:
            data = yaml.safe_load(f)
        return json.dumps(data, default=str)
    except Exception as e:
        return json.dumps({"error": str(e)})


@tool
def restart_deployment(deployment_name: str, namespace: str = "default") -> str:
    """Perform a rolling restart of a Kubernetes deployment.
    Safe — triggers new rollout without downtime.
    Use for CrashLoopBackOff pods or after config changes.
    Format: restart_deployment(deployment_name="web", namespace="default")"""
    cmd = f"kubectl rollout restart deployment/{deployment_name} -n {namespace}"
    if config.dry_run:
        return json.dumps({"status": "dry_run", "command": cmd, "action": "restart_deployment",
                           "target": f"{namespace}/{deployment_name}"})
    try:
        result = subprocess.run(shlex.split(cmd), capture_output=True, text=True, timeout=30)
        return json.dumps({
            "status": "executed" if result.returncode == 0 else "error",
            "action": "restart_deployment", "target": f"{namespace}/{deployment_name}",
            "command": cmd, "stdout": result.stdout[:500], "stderr": result.stderr[:300],
            "success": result.returncode == 0,
        })
    except Exception as e:
        return json.dumps({"status": "error", "reason": str(e), "command": cmd})


@tool
def scale_deployment(deployment_name: str, replicas: int, namespace: str = "default") -> str:
    """Scale a Kubernetes deployment to the specified replica count.
    Use to scale UP during high load or DOWN to free resources. Max 50 replicas.
    Format: scale_deployment(deployment_name="web", replicas=3, namespace="default")"""
    replicas = max(0, min(replicas, 50))
    cmd = f"kubectl scale deployment/{deployment_name} --replicas={replicas} -n {namespace}"
    if config.dry_run:
        return json.dumps({"status": "dry_run", "command": cmd, "action": "scale_deployment",
                           "target": f"{namespace}/{deployment_name}", "replicas": replicas})
    try:
        result = subprocess.run(shlex.split(cmd), capture_output=True, text=True, timeout=30)
        return json.dumps({
            "status": "executed" if result.returncode == 0 else "error",
            "action": "scale_deployment", "target": f"{namespace}/{deployment_name}",
            "replicas": replicas, "command": cmd,
            "stdout": result.stdout[:500], "success": result.returncode == 0,
        })
    except Exception as e:
        return json.dumps({"status": "error", "reason": str(e), "command": cmd})


@tool
def delete_stuck_pod(pod_name: str, namespace: str = "default", force: bool = False) -> str:
    """Delete a stuck or failed pod so its controller recreates it.
    Use for Evicted, Failed, or stuck Terminating pods.
    Set force=True ONLY for pods stuck in Terminating state (adds --grace-period=0).
    Format: delete_stuck_pod(pod_name="web-abc-123", namespace="default", force=False)"""
    force_flags = " --force --grace-period=0" if force else ""
    cmd = f"kubectl delete pod {pod_name} -n {namespace}{force_flags}"
    if config.dry_run:
        return json.dumps({"status": "dry_run", "command": cmd, "action": "delete_pod",
                           "target": f"{namespace}/{pod_name}"})
    try:
        result = subprocess.run(shlex.split(cmd), capture_output=True, text=True, timeout=30)
        return json.dumps({
            "status": "executed" if result.returncode == 0 else "error",
            "action": "delete_pod", "target": f"{namespace}/{pod_name}",
            "command": cmd, "stdout": result.stdout[:500], "success": result.returncode == 0,
        })
    except Exception as e:
        return json.dumps({"status": "error", "reason": str(e), "command": cmd})


@tool
def cordon_node(node_name: str, uncordon: bool = False) -> str:
    """Cordon or uncordon a Kubernetes node to control pod scheduling.
    Cordon prevents NEW pods from being scheduled on the node (existing pods unaffected).
    Use when node shows disk/memory pressure or needs maintenance.
    Set uncordon=True to re-enable scheduling after the node recovers.
    Format: cordon_node(node_name="node-1", uncordon=False)"""
    action = "uncordon" if uncordon else "cordon"
    cmd = f"kubectl {action} {node_name}"
    if config.dry_run:
        return json.dumps({"status": "dry_run", "command": cmd, "action": action, "node": node_name})
    try:
        result = subprocess.run(shlex.split(cmd), capture_output=True, text=True, timeout=30)
        return json.dumps({
            "status": "executed" if result.returncode == 0 else "error",
            "action": action, "node": node_name,
            "command": cmd, "stdout": result.stdout[:500], "success": result.returncode == 0,
        })
    except Exception as e:
        return json.dumps({"status": "error", "reason": str(e), "command": cmd})


@tool
def describe_cluster_resource(resource_type: str, resource_name: str, namespace: str = "default") -> str:
    """Run kubectl describe on any cluster resource for deep diagnosis.
    Returns full status, events, conditions, and resource limits.
    resource_type must be one of: pod, deployment, node, service, pvc, replicaset, statefulset, daemonset, job.
    Format: describe_cluster_resource(resource_type="pod", resource_name="web-abc-123", namespace="default")"""
    SAFE_TYPES = {"pod", "deployment", "node", "service", "pvc", "replicaset", "statefulset", "daemonset", "job"}
    if resource_type.lower() not in SAFE_TYPES:
        return json.dumps({"error": f"resource_type '{resource_type}' not allowed. Use: {sorted(SAFE_TYPES)}"})
    ns_flag = f" -n {namespace}" if resource_type.lower() != "node" else ""
    cmd = f"kubectl describe {resource_type} {resource_name}{ns_flag}"
    try:
        result = subprocess.run(shlex.split(cmd), capture_output=True, text=True, timeout=20)
        return json.dumps({
            "command": cmd,
            "output": result.stdout[:4000],
            "error": result.stderr[:500] if result.returncode != 0 else None,
        })
    except Exception as e:
        return json.dumps({"error": str(e), "command": cmd})


@tool
def execute_safe_command(command: str, dry_run: bool = True) -> str:
    """
    Execute a kubectl command against the cluster. ONLY commands in the safe allowlist
    are permitted (kubectl get/describe/logs/delete pod/rollout restart/scale deployment/).
    Set dry_run=True to preview the command without executing.
    Format: execute_safe_command(command="kubectl delete pod web-abc -n default", dry_run=False)
    """
    if not _is_safe(command):
        return json.dumps({
            "status": "blocked",
            "reason": f"Command not in safe allowlist: {command!r}",
            "safe_prefixes": SAFE_PREFIXES,
        })

    if dry_run or config.dry_run:
        return json.dumps({
            "status": "dry_run",
            "command": command,
            "would_execute": True,
        })

    try:
        args = shlex.split(command)
        result = subprocess.run(
            args,
            capture_output=True,
            text=True,
            timeout=30,
        )
        return json.dumps({
            "status": "executed",
            "command": command,
            "returncode": result.returncode,
            "stdout": result.stdout[:1000],
            "stderr": result.stderr[:500],
            "success": result.returncode == 0,
        })
    except subprocess.TimeoutExpired:
        return json.dumps({"status": "error", "reason": "Command timed out after 30s"})
    except FileNotFoundError:
        return json.dumps({"status": "error", "reason": "kubectl not found in PATH"})
    except Exception as e:
        return json.dumps({"status": "error", "reason": str(e)})
