"""
Runbook tools: load YAML runbooks and execute kubectl commands via the safe allowlist.
"""
import json
import logging
import os
import re
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
