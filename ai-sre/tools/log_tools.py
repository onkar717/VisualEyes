"""
Log analysis tools: pattern mining, stack trace extraction, Loki LogQL support.
Falls back to kubectl logs if Loki unavailable.
"""
import concurrent.futures
import json
import logging
import re
from typing import Dict, List, Optional

import requests
from crewai.tools import tool
from kubernetes import client, config as kube_config

from ..config import config as sre_config

logger = logging.getLogger("visualeyes.tools.logs")

ERROR_PATTERNS = [
    (re.compile(r"(FATAL|CRITICAL|PANIC)", re.I), "fatal"),
    (re.compile(r"\b(ERROR|Exception|Traceback|panic:)", re.I), "error"),
    (re.compile(r"(OOMKilled|OutOfMemory|out of memory|Cannot allocate memory)", re.I), "oom"),
    (re.compile(r"(connection refused|timeout|ECONNREFUSED|ETIMEDOUT|dial tcp)", re.I), "connectivity"),
    (re.compile(r"(permission denied|unauthorized|forbidden|401|403)", re.I), "auth"),
    (re.compile(r"(CrashLoopBackOff|BackOff|back-off)", re.I), "crashloop"),
    (re.compile(r"(disk full|no space left|ENOSPC|quota exceeded)", re.I), "disk"),
    (re.compile(r"(segfault|signal 11|SIGSEGV|core dumped)", re.I), "segfault"),
    (re.compile(r"(ImagePullBackOff|ErrImagePull|pull access denied)", re.I), "image_pull"),
    (re.compile(r"(FailedScheduling|Insufficient|Unschedulable)", re.I), "scheduling"),
]

STACK_START = re.compile(
    r"(Traceback \(most recent call last\)|goroutine \d+ \[|java\.lang\.|panic:|at .*\(.*\.java:\d+\))",
    re.I,
)

_v1: Optional[client.CoreV1Api] = None


def _get_v1() -> client.CoreV1Api:
    global _v1
    if _v1 is None:
        try:
            kube_config.load_incluster_config()
        except kube_config.ConfigException:
            kube_config.load_kube_config()
        _v1 = client.CoreV1Api()
    return _v1


def _fetch_pod_logs(pod: str, ns: str, tail: int = 150, previous: bool = False) -> str:
    try:
        v1 = _get_v1()
        return v1.read_namespaced_pod_log(
            name=pod, namespace=ns, tail_lines=tail, timestamps=True, previous=previous,
        ) or ""
    except Exception as e:
        return f"[log error: {e}]"


def _classify_line(line: str) -> Optional[str]:
    for pattern, category in ERROR_PATTERNS:
        if pattern.search(line):
            return category
    return None


def _extract_stack_traces(lines: List[str], max_frames: int = 15) -> List[str]:
    traces = []
    in_trace = False
    current: List[str] = []
    for line in lines:
        if STACK_START.search(line):
            in_trace = True
            current = [line]
        elif in_trace:
            if line.strip():
                current.append(line)
                if len(current) >= max_frames:
                    traces.append("\n".join(current))
                    in_trace = False
                    current = []
            else:
                if len(current) > 2:
                    traces.append("\n".join(current))
                in_trace = False
                current = []
    if current and len(current) > 2:
        traces.append("\n".join(current))
    return traces[:3]


def _mine_patterns(raw_logs: str) -> dict:
    lines = raw_logs.splitlines()
    categories: dict = {}
    error_lines: List[str] = []
    for line in lines:
        cat = _classify_line(line)
        if cat:
            categories[cat] = categories.get(cat, 0) + 1
            if len(error_lines) < 20:
                error_lines.append(line.strip()[:200])
    traces = _extract_stack_traces(lines)
    return {
        "total_lines": len(lines),
        "error_categories": categories,
        "top_error_lines": error_lines[:10],
        "stack_traces": traces,
        "has_errors": bool(categories),
    }


@tool
def analyze_pod_logs(pod_name: str, namespace: str = "default",
                     container: str = "", tail_lines: int = 150) -> str:
    """
    Fetch and analyze logs from a pod. Extracts error patterns, classifies error types,
    and pulls stack traces. Automatically checks PREVIOUS container logs for crashlooping pods.
    Format: analyze_pod_logs(pod_name="web-abc", namespace="production", tail_lines=150)
    """
    current_raw = _fetch_pod_logs(pod_name, namespace, tail_lines, previous=False)
    prev_raw = _fetch_pod_logs(pod_name, namespace, tail_lines // 2, previous=True)

    current_analysis = _mine_patterns(current_raw)
    prev_analysis = _mine_patterns(prev_raw) if not prev_raw.startswith("[log error") else None

    result: dict = {
        "pod": pod_name,
        "namespace": namespace,
        "current_logs": current_analysis,
        "previous_logs": prev_analysis,
        "recommendation": "",
    }

    # Derive recommendation from patterns
    cats = current_analysis.get("error_categories", {})
    if "oom" in cats:
        result["recommendation"] = "OOM detected increase memory limits or fix memory leak"
    elif "crashloop" in cats or (prev_analysis and "error" in prev_analysis.get("error_categories", {})):
        result["recommendation"] = "Crash loop evidence check previous logs for root error"
    elif "connectivity" in cats:
        result["recommendation"] = "Connectivity failures check network policies and service endpoints"
    elif "auth" in cats:
        result["recommendation"] = "Auth failures check RBAC, secrets, and token expiry"
    elif "disk" in cats:
        result["recommendation"] = "Disk pressure check PVC usage and node disk"
    elif "image_pull" in cats:
        result["recommendation"] = "Image pull failure verify image name, tag, and registry credentials"
    elif "scheduling" in cats:
        result["recommendation"] = "Scheduling failure check resource quotas and node capacity"

    return json.dumps(result)


@tool
def search_logs_pattern(pod_name: str, namespace: str = "default",
                        pattern: str = "ERROR", tail_lines: int = 200) -> str:
    """
    Search pod logs for a specific pattern (regex supported).
    Returns matching lines with line numbers and timestamps.
    Format: search_logs_pattern(pod_name="my-pod", namespace="default", pattern="OOMKilled")
    """
    raw = _fetch_pod_logs(pod_name, namespace, tail_lines)
    if raw.startswith("[log error"):
        return json.dumps({"error": raw})

    try:
        regex = re.compile(pattern, re.I)
    except re.error as e:
        return json.dumps({"error": f"Invalid regex: {e}"})

    matches = []
    for i, line in enumerate(raw.splitlines(), 1):
        if regex.search(line):
            matches.append({"line": i, "text": line.strip()[:300]})

    return json.dumps({
        "pod": pod_name,
        "pattern": pattern,
        "match_count": len(matches),
        "matches": matches[:30],
    })


@tool
def loki_query(logql: str = "", namespace: str = "", pod_name: str = "",
               limit: int = 50, minutes_back: int = 30) -> str:
    """
    Query Loki for logs using LogQL. Falls back to kubectl logs if Loki unavailable.
    If logql is empty, builds a query from namespace and pod_name.
    Format: loki_query(logql='{namespace="production",pod="web-abc"}|="ERROR"', limit=50)
    """
    if not sre_config.loki_enabled:
        # Fallback: use kubectl logs
        if pod_name and namespace:
            raw = _fetch_pod_logs(pod_name, namespace, limit)
            return json.dumps({
                "source": "kubectl_logs_fallback",
                "pod": pod_name,
                "namespace": namespace,
                "log": raw[:3000],
            })
        return json.dumps({"error": "Loki disabled. Set LOKI_ENABLED=true or provide pod_name+namespace."})

    if not logql:
        parts = []
        if namespace:
            parts.append(f'namespace="{namespace}"')
        if pod_name:
            parts.append(f'pod="{pod_name}"')
        logql = "{" + ",".join(parts) + "}" if parts else '{job="kubernetes"}'

    import time as _time
    end_ns = int(_time.time() * 1e9)
    start_ns = end_ns - (minutes_back * 60 * int(1e9))

    try:
        resp = requests.get(
            f"{sre_config.loki_url}/loki/api/v1/query_range",
            params={
                "query": logql,
                "start": start_ns,
                "end": end_ns,
                "limit": limit,
                "direction": "backward",
            },
            timeout=15,
        )
        resp.raise_for_status()
        data = resp.json()
        streams = data.get("data", {}).get("result", [])
        lines = []
        for stream in streams:
            for entry in stream.get("values", []):
                lines.append({"ts": entry[0], "msg": entry[1][:300]})
        return json.dumps({
            "source": "loki",
            "logql": logql,
            "lines": lines[:limit],
            "count": len(lines),
        })
    except requests.exceptions.ConnectionError:
        # Fallback to kubectl logs
        if pod_name and namespace:
            raw = _fetch_pod_logs(pod_name, namespace, limit)
            return json.dumps({
                "source": "kubectl_logs_fallback",
                "loki_error": f"Loki unreachable at {sre_config.loki_url}",
                "log": raw[:3000],
            })
        return json.dumps({"error": f"Loki unreachable at {sre_config.loki_url}"})
    except Exception as e:
        return json.dumps({"error": str(e)})


@tool
def analyze_logs_for_namespace(namespace: str = "default", max_pods: int = 10) -> str:
    """Analyze logs from all running pods in a namespace in parallel.
    Returns aggregated error counts, top error categories, and worst offenders.
    Use when you need a namespace-wide log health picture quickly without targeting a specific pod.
    Format: analyze_logs_for_namespace(namespace="production", max_pods=10)"""
    try:
        v1 = _get_v1()
        pods = v1.list_namespaced_pod(namespace)
        pod_names = [
            p.metadata.name for p in pods.items
            if p.status and p.status.phase == "Running"
        ][:max_pods]

        if not pod_names:
            return json.dumps({"namespace": namespace, "pods_analyzed": 0,
                               "message": "No running pods found"})

        def analyze_one(name: str) -> dict:
            raw = _fetch_pod_logs(name, namespace, tail=100)
            analysis = _mine_patterns(raw)
            errors = analysis.get("errors", [])
            cat_freq: Dict[str, int] = {}
            for e in errors:
                cat = e.get("category", "other")
                cat_freq[cat] = cat_freq.get(cat, 0) + 1
            return {
                "pod": name,
                "error_count": len(errors),
                "categories": cat_freq,
                "top_error": errors[0]["text"][:200] if errors else None,
            }

        results = []
        with concurrent.futures.ThreadPoolExecutor(max_workers=5) as ex:
            futures = {ex.submit(analyze_one, name): name for name in pod_names}
            for fut in concurrent.futures.as_completed(futures):
                try:
                    results.append(fut.result(timeout=15))
                except Exception as e:
                    results.append({"pod": futures[fut], "error": str(e)})

        results.sort(key=lambda r: r.get("error_count", 0), reverse=True)

        total_errors = sum(r.get("error_count", 0) for r in results)
        agg_cats: Dict[str, int] = {}
        for r in results:
            for cat, cnt in r.get("categories", {}).items():
                agg_cats[cat] = agg_cats.get(cat, 0) + cnt

        return json.dumps({
            "namespace": namespace,
            "pods_analyzed": len(results),
            "total_errors": total_errors,
            "error_categories": agg_cats,
            "pod_breakdown": results,
            "worst_offender": results[0]["pod"] if results and results[0].get("error_count", 0) > 0 else None,
        })
    except Exception as e:
        return json.dumps({"error": str(e), "namespace": namespace})
