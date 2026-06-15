"""
CrewAI pipeline orchestration with stage callbacks to VisualEyes Go SSE hub.
Each agent completion fires POST /internal/rca/stage-event → Go publishes SSE.
"""
import json
import logging
import re
import time
from datetime import datetime
from typing import Any, Dict, Optional

import requests
from crewai import Crew, Task, Process

from .agents import (
    triage_agent, metrics_agent, log_agent,
    infra_agent, runbook_agent, commander_agent,
)
from .config import config

logger = logging.getLogger("visualeyes.pipeline")

STAGE_LABELS = ["Triage", "Metrics", "Logs", "Infra", "Remediation", "Commander"]

MAX_RETRIES = config.max_retries
RETRY_BACKOFF = [15, 30, 60]


def _clean_json(raw: str) -> str:
    return re.sub(r"```(?:json)?|```", "", raw).strip()


def _post_stage_event(go_callback_url: str, alert_id: int, stage: int,
                      status: str, detail: str = "") -> None:
    if not go_callback_url:
        return
    try:
        requests.post(
            go_callback_url,
            json={
                "alert_id": alert_id,
                "stage": stage,
                "label": STAGE_LABELS[stage - 1] if 1 <= stage <= 6 else f"Stage{stage}",
                "status": status,
                "detail": detail,
            },
            timeout=3,
        )
    except Exception as e:
        logger.debug("stage callback failed (non-critical): %s", e)


def _build_alert_context_str(alert_ctx: Dict[str, Any]) -> str:
    """Convert Go-provided AlertContext to a prompt-friendly string."""
    parts = []

    alert = alert_ctx.get("alert", {})
    if alert:
        parts.append(
            f"ALERT: rule={alert.get('rule_name','')} severity={alert.get('severity','')} "
            f"pod={alert.get('resource_id','')} namespace={alert.get('namespace','')} "
            f"value={alert.get('value','')} threshold={alert.get('threshold','')} "
            f"message={alert.get('message','')}"
        )

    anomalies = alert_ctx.get("anomalies", [])
    if anomalies:
        parts.append(f"PRE-DETECTED ANOMALIES: {json.dumps(anomalies)}")

    log_class = alert_ctx.get("log_classification", {})
    if log_class and log_class.get("error_count", 0) > 0:
        parts.append(f"PRE-CLASSIFIED LOG PATTERNS: {json.dumps(log_class)}")

    k8s_events = alert_ctx.get("k8s_events", [])
    if k8s_events:
        parts.append(f"PRE-FETCHED K8S WARNING EVENTS: {json.dumps(k8s_events[:10])}")

    recent_metrics = alert_ctx.get("recent_metrics", [])
    if recent_metrics:
        parts.append(f"RECENT METRIC SAMPLES: {json.dumps(recent_metrics[:5])}")

    return "\n\n".join(parts)


def _build_crew(alert_ctx: Dict[str, Any], alert_id: int,
                go_callback_url: str) -> tuple:
    """
    Build the 6-task sequential crew. Injects pre-built Go AlertContext into
    the triage task description so agents start with rich context, not blind.
    """
    ctx_str = _build_alert_context_str(alert_ctx)
    namespaces_str = ", ".join(config.namespaces)
    alert = alert_ctx.get("alert", {})
    pod_name = alert.get("resource_id", "")
    namespace = alert.get("namespace", namespaces_str)

    focus = f"Focus on pod={pod_name} in namespace={namespace}." if pod_name else \
        f"Scan namespaces: {namespaces_str}."

    # ── Task 1: Triage ───────────────────────────────────────────────────────
    task_triage = Task(
        description=(
            f"{focus}\n\n"
            "PRE-BUILT CONTEXT (from VisualEyes Go agent use as starting knowledge):\n"
            f"{ctx_str}\n\n"
            "Now verify and expand:\n"
            "1. list_pods_all_namespaces confirm which pods are non-Running\n"
            "2. get_cluster_events recent Warning events in scope namespace\n"
            "3. get_node_health node conditions and taints\n"
            "4. get_namespace_summary pod counts by phase\n"
            "5. Classify severity SEV1–SEV4 based on all evidence.\n"
            "Output: structured triage summary with severity, affected pods, and scope."
        ),
        expected_output=(
            "Triage report: severity classification, list of affected pods/namespaces, "
            "top Warning events, node health status."
        ),
        agent=triage_agent,
        callback=lambda output: _post_stage_event(go_callback_url, alert_id, 1, "done",
                                                   str(output)[:120]) if output else None,
    )

    # ── Task 2: Metrics ──────────────────────────────────────────────────────
    task_metrics = Task(
        description=(
            f"Analyse metrics for the pods identified in triage. {focus}\n"
            f"CPU critical: {config.cpu_critical_pct}%  "
            f"Memory critical: {config.memory_critical_pct}%  "
            f"Restart critical: {config.restart_critical}/30min\n\n"
            "1. get_cpu_usage_per_pod top CPU consumers\n"
            "2. get_memory_usage_per_pod top memory consumers\n"
            "3. get_pod_restart_rate recent restarts\n"
            "4. get_oom_kill_events OOM-killed containers\n"
            "5. If specific pod suspect: detect_metric_anomaly(pod_name, namespace)\n"
            "6. get_http_error_rate and get_request_latency_p99 if service-level impact\n"
            "Report exact numbers with context. State if Prometheus unavailable."
        ),
        expected_output=(
            "Metrics report: CPU/memory per pod, restart rates, OOM events, "
            "anomaly detection results, and whether metrics confirm or refute triage."
        ),
        agent=metrics_agent,
        context=[task_triage],
        callback=lambda output: _post_stage_event(go_callback_url, alert_id, 2, "done",
                                                   str(output)[:120]) if output else None,
    )

    # ── Task 3: Logs ─────────────────────────────────────────────────────────
    task_logs = Task(
        description=(
            f"Analyse logs from problematic pods identified in triage and metrics. {focus}\n\n"
            "1. analyze_pod_logs for the top affected pods\n"
            "2. For crashlooping pods: set previous=True to get pre-crash logs\n"
            "3. search_logs_pattern for specific error signatures\n"
            "4. loki_query if Loki is available (LOKI_ENABLED)\n"
            "5. Identify the FIRST occurrence of errors (failure onset time)\n"
            "6. Extract exact stack traces and error messages\n"
            "Quote exact error strings never paraphrase log content."
        ),
        expected_output=(
            "Log analysis: error categories and frequencies, exact stack traces, "
            "first error timestamps, key error lines that reveal root cause."
        ),
        agent=log_agent,
        context=[task_triage, task_metrics],
        callback=lambda output: _post_stage_event(go_callback_url, alert_id, 3, "done",
                                                   str(output)[:120]) if output else None,
    )

    # ── Task 4: Infra ─────────────────────────────────────────────────────────
    task_infra = Task(
        description=(
            f"Investigate K8s infrastructure constraints. {focus}\n\n"
            "1. describe_pod for the primary failing pod check events and conditions\n"
            "2. get_resource_quotas is namespace at/near quota limits?\n"
            "3. get_pvc_status any unbound PVCs blocking scheduling?\n"
            "4. get_hpa_status any HPA at max replicas?\n"
            "5. get_node_health node taints, unschedulable, pressure conditions\n"
            "Determine: is root cause INFRA (quotas/storage/scheduling) or "
            "APPLICATION (code bug/config error/resource limits)?"
        ),
        expected_output=(
            "Infra diagnosis: quota status, PVC health, HPA analysis, node conditions, "
            "conclusion on whether root cause is infra or application."
        ),
        agent=infra_agent,
        context=[task_triage, task_metrics, task_logs],
        callback=lambda output: _post_stage_event(go_callback_url, alert_id, 4, "done",
                                                   str(output)[:120]) if output else None,
    )

    # ── Task 5: Runbook ───────────────────────────────────────────────────────
    task_runbook = Task(
        description=(
            "Select the best runbook and produce the remediation plan.\n\n"
            "1. list_available_runbooks see what's available\n"
            "2. load_runbook for the best match\n"
            "3. Adapt steps to the specific pods/deployments affected\n"
            "4. Produce numbered plan with exact commands\n"
            "5. Mark each step: is_auto_safe (allowlist-safe), is_destructive\n"
            "6. Order: rolling restart > force delete > scale > cordon\n"
            f"DRY_RUN: {'ENABLED do not execute' if config.dry_run else 'DISABLED can execute safe commands'}"
        ),
        expected_output=(
            "Numbered remediation plan with exact kubectl commands, "
            "auto-safe/destructive flags, and matched runbook filename."
        ),
        agent=runbook_agent,
        context=[task_triage, task_metrics, task_logs, task_infra],
        callback=lambda output: _post_stage_event(go_callback_url, alert_id, 5, "done",
                                                   str(output)[:120]) if output else None,
    )

    # ── Task 6: Commander ─────────────────────────────────────────────────────
    task_command = Task(
        description=(
            "Synthesize ALL findings into a final incident report.\n"
            "Output EXACTLY this JSON no markdown, no prose, just the object:\n\n"
            "{\n"
            '  "has_issue": true/false,\n'
            '  "severity": "SEV1|SEV2|SEV3|SEV4",\n'
            '  "category": "crashloop|oom|high_cpu|high_memory|image_pull|pending|'
            'node_not_ready|disk_pressure|error_rate|latency|healthy",\n'
            '  "title": "concise incident title",\n'
            '  "affected_namespaces": ["ns1"],\n'
            '  "affected_services": [{"service_name": "x", "namespace": "y", "impact_level": "down|degraded|at_risk"}],\n'
            '  "root_cause": "3-5 sentence root cause explanation",\n'
            '  "explanation": "1-2 sentence operator-facing summary",\n'
            '  "contributing_factors": ["factor1", "factor2"],\n'
            '  "confidence": 0-100,\n'
            '  "commands": [\n'
            '    {"command": "kubectl ...", "description": "...", "is_auto_safe": true, "risk": "low", "step": 1}\n'
            '  ],\n'
            '  "runbook_used": "filename.yaml"\n'
            "}\n\n"
            "RULES:\n"
            "- has_issue=false only if at least 2 data sources confirm cluster is healthy\n"
            "- Only include commands that are kubectl-based and actionable\n"
            "- confidence reflects how strongly the evidence supports the conclusion\n"
            "- Never hallucinate pods, metrics, or errors not present in agent outputs"
        ),
        expected_output="Valid JSON incident report. No markdown. No explanation. Just the JSON object.",
        agent=commander_agent,
        context=[task_triage, task_metrics, task_logs, task_infra, task_runbook],
        callback=lambda output: _post_stage_event(go_callback_url, alert_id, 6, "done",
                                                   str(output)[:120]) if output else None,
    )

    crew = Crew(
        agents=[triage_agent, metrics_agent, log_agent, infra_agent, runbook_agent, commander_agent],
        tasks=[task_triage, task_metrics, task_logs, task_infra, task_runbook, task_command],
        process=Process.sequential,
        verbose=True,
        memory=False,
        max_rpm=30,
    )
    tasks = [task_triage, task_metrics, task_logs, task_infra, task_runbook, task_command]
    return crew, tasks


def _parse_commander_output(raw: str, start_time: float) -> dict:
    cleaned = _clean_json(raw)
    match = re.search(r"\{.*\}", cleaned, re.DOTALL)
    if match:
        cleaned = match.group(0)
    try:
        data = json.loads(cleaned)
    except json.JSONDecodeError as e:
        logger.warning("commander JSON parse failed: %s | raw: %s", e, raw[:300])
        data = {
            "has_issue": True,
            "severity": "SEV3",
            "category": "unknown",
            "title": "Analysis parse error manual review needed",
            "root_cause": f"Agent output could not be parsed: {raw[:500]}",
            "explanation": "AI pipeline completed but output was malformed.",
            "confidence": 10,
        }

    data.setdefault("has_issue", True)
    data.setdefault("severity", "SEV3")
    data.setdefault("category", "unknown")
    data.setdefault("title", "Unnamed Incident")
    data.setdefault("root_cause", "")
    data.setdefault("explanation", data.get("root_cause", "")[:200])
    data.setdefault("contributing_factors", [])
    data.setdefault("confidence", 50)
    data.setdefault("commands", [])
    data.setdefault("affected_namespaces", [])
    data.setdefault("affected_services", [])
    data.setdefault("runbook_used", None)
    data["scan_duration_seconds"] = round(time.time() - start_time, 1)
    data["llm_model"] = config.llm_model
    data["raw_output"] = raw[:2000]
    return data


def run_pipeline(
    alert_ctx: Dict[str, Any],
    alert_id: int,
    go_callback_url: str = "",
) -> dict:
    """
    Run the full 6-agent CrewAI pipeline for one alert.
    Returns structured incident report dict (matches Go RCAResponse shape).
    """
    start_time = time.time()
    logger.info("AI-SRE pipeline starting for alert_id=%d model=%s", alert_id, config.llm_model)

    # Emit stage-start events before each agent so CLI shows live progress.
    # CrewAI task callbacks fire on completion; we emit start events pre-run.
    for stage, label in enumerate(STAGE_LABELS, 1):
        _post_stage_event(go_callback_url, alert_id, stage, "start")

    last_error: Optional[Exception] = None
    for attempt in range(MAX_RETRIES):
        try:
            crew, _ = _build_crew(alert_ctx, alert_id, go_callback_url)
            result = crew.kickoff()
            raw = result.raw if hasattr(result, "raw") else str(result)
            report = _parse_commander_output(raw, start_time)
            logger.info(
                "AI-SRE pipeline complete: alert_id=%d severity=%s confidence=%d duration=%.1fs",
                alert_id, report.get("severity"), report.get("confidence", 0),
                report.get("scan_duration_seconds", 0),
            )
            return report
        except Exception as e:
            last_error = e
            err_str = str(e).lower()
            is_rate_limit = any(x in err_str for x in ["rate limit", "429", "ratelimit", "quota"])
            if is_rate_limit and attempt < MAX_RETRIES - 1:
                wait = RETRY_BACKOFF[attempt]
                logger.warning("rate limit hit attempt=%d, retrying in %ds", attempt + 1, wait)
                time.sleep(wait)
                continue
            logger.error("pipeline failed attempt=%d: %s", attempt + 1, e)
            break

    # All retries exhausted emit failed events and return minimal error report.
    for stage in range(1, 7):
        _post_stage_event(go_callback_url, alert_id, stage, "failed")

    return {
        "has_issue": True,
        "severity": "SEV3",
        "category": "unknown",
        "title": "AI-SRE Pipeline Error",
        "root_cause": f"Pipeline failed after {MAX_RETRIES} attempts: {last_error}",
        "explanation": "AI analysis failed manual investigation required.",
        "contributing_factors": [],
        "confidence": 0,
        "commands": [],
        "affected_namespaces": [],
        "affected_services": [],
        "runbook_used": None,
        "scan_duration_seconds": round(time.time() - start_time, 1),
        "llm_model": config.llm_model,
        "error": str(last_error),
    }
