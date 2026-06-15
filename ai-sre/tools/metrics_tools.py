"""
Prometheus metrics tools with z-score anomaly detection.
Falls back gracefully when Prometheus is unavailable.
"""
import json
import logging
import math
import time
from typing import List, Tuple

import requests
from crewai.tools import tool

from ..config import config

logger = logging.getLogger("visualeyes.tools.metrics")

PROM_URL = config.prometheus_url
WINDOW = config.metrics_window


def _prom_query(query: str) -> dict:
    try:
        resp = requests.get(
            f"{PROM_URL}/api/v1/query",
            params={"query": query},
            timeout=10,
        )
        resp.raise_for_status()
        return resp.json()
    except requests.exceptions.ConnectionError:
        return {"status": "error", "error": f"Prometheus unreachable at {PROM_URL}. Use K8s data instead."}
    except Exception as e:
        return {"status": "error", "error": str(e)}


def _prom_range(query: str, start: float, end: float, step: str = "60s") -> dict:
    try:
        resp = requests.get(
            f"{PROM_URL}/api/v1/query_range",
            params={"query": query, "start": start, "end": end, "step": step},
            timeout=15,
        )
        resp.raise_for_status()
        return resp.json()
    except Exception as e:
        return {"status": "error", "error": str(e)}


def _results(resp: dict) -> list:
    if resp.get("status") != "success":
        return []
    return resp.get("data", {}).get("result", [])


def _zscore(values: List[float], threshold: float = 2.5) -> Tuple[bool, float]:
    """Return (is_anomaly, z_score) using z-score over the sample window."""
    if len(values) < 5:
        return False, 0.0
    mean = sum(values) / len(values)
    std = math.sqrt(sum((v - mean) ** 2 for v in values) / len(values))
    if std == 0:
        return False, 0.0
    z = (values[-1] - mean) / std
    return abs(z) > threshold, round(z, 2)


@tool
def get_cpu_usage_per_pod(namespace: str = "") -> str:
    """
    Get CPU usage (millicores) per pod via Prometheus.
    Returns top 30 pods by CPU, sorted descending.
    Use to identify CPU-hungry or throttled pods.
    Format: get_cpu_usage_per_pod(namespace="production") or "" for all.
    """
    ns_filter = f', namespace="{namespace}"' if namespace else ""
    q = (
        f"sort_desc(sum by(pod,namespace)"
        f"(rate(container_cpu_usage_seconds_total{{container!=\"\"{ns_filter}}}[{WINDOW}])))"
    )
    resp = _prom_query(q)
    if resp.get("status") == "error":
        return json.dumps(resp)
    items = []
    for r in _results(resp)[:30]:
        val = float(r["value"][1]) if r.get("value") else 0.0
        items.append({
            "pod": r["metric"].get("pod", "?"),
            "namespace": r["metric"].get("namespace", "?"),
            "cpu_millicores": round(val * 1000, 1),
        })
    return json.dumps({"status": "ok", "window": WINDOW, "pods": items})


@tool
def get_memory_usage_per_pod(namespace: str = "") -> str:
    """
    Get memory usage (MB) per pod via Prometheus working set bytes.
    Returns top 30 pods. Use to identify memory-heavy or leaking pods.
    Format: get_memory_usage_per_pod(namespace="production")
    """
    ns_filter = f', namespace="{namespace}"' if namespace else ""
    q = (
        f"sort_desc(sum by(pod,namespace)"
        f"(container_memory_working_set_bytes{{container!=\"\"{ns_filter}}}))"
    )
    resp = _prom_query(q)
    if resp.get("status") == "error":
        return json.dumps(resp)
    items = []
    for r in _results(resp)[:30]:
        val = float(r["value"][1]) if r.get("value") else 0.0
        items.append({
            "pod": r["metric"].get("pod", "?"),
            "namespace": r["metric"].get("namespace", "?"),
            "memory_mb": round(val / (1024 * 1024), 1),
        })
    return json.dumps({"status": "ok", "pods": items})


@tool
def get_pod_restart_rate(namespace: str = "") -> str:
    """
    Get pod restart counts in last 30 minutes.
    High restart rates indicate CrashLoopBackOff or OOM kills.
    Returns only pods with at least 1 restart.
    Format: get_pod_restart_rate(namespace="default")
    """
    ns_filter = f', namespace="{namespace}"' if namespace else ""
    q = (
        f"sort_desc(increase(kube_pod_container_status_restarts_total"
        f"{{container!=\"\"{ns_filter}}}[30m]))"
    )
    resp = _prom_query(q)
    if resp.get("status") == "error":
        return json.dumps(resp)
    items = []
    for r in _results(resp):
        val = float(r["value"][1]) if r.get("value") else 0.0
        if val > 0:
            items.append({
                "pod": r["metric"].get("pod", "?"),
                "container": r["metric"].get("container", "?"),
                "namespace": r["metric"].get("namespace", "?"),
                "restarts_30m": round(val, 1),
            })
    return json.dumps({"status": "ok", "pods": items[:20]})


@tool
def get_http_error_rate(namespace: str = "") -> str:
    """
    Get HTTP 5xx error rate per service (as %).
    Values above 1% indicate service problems; above 5% is critical.
    Format: get_http_error_rate(namespace="production")
    """
    ns_filter = f', namespace="{namespace}"' if namespace else ""
    err_q = f'sum by(service,namespace)(rate(http_requests_total{{code=~"5.."{ns_filter}}}[{WINDOW}]))'
    ok_q  = f'sum by(service,namespace)(rate(http_requests_total{{code!~"5.."{ns_filter}}}[{WINDOW}]))'

    errors = {
        (r["metric"].get("service",""), r["metric"].get("namespace","")): float(r["value"][1])
        for r in _results(_prom_query(err_q))
    }
    totals = {
        (r["metric"].get("service",""), r["metric"].get("namespace","")): float(r["value"][1])
        for r in _results(_prom_query(ok_q))
    }

    result = []
    for key, err in errors.items():
        tot = totals.get(key, 0)
        pct = err / (tot + err) * 100 if (tot + err) > 0 else 0
        result.append({
            "service": key[0] or "?",
            "namespace": key[1] or "?",
            "error_rate_pct": round(pct, 2),
            "error_rps": round(err, 4),
        })
    result.sort(key=lambda x: x["error_rate_pct"], reverse=True)
    return json.dumps({"status": "ok", "window": WINDOW, "services": result[:20]})


@tool
def get_request_latency_p99(namespace: str = "") -> str:
    """
    Get P99 request latency (ms) per service via Prometheus histogram_quantile.
    High P99 means slow tail responses. Returns top 20 services.
    Format: get_request_latency_p99(namespace="production")
    """
    q = (
        "sort_desc(histogram_quantile(0.99, sum by(service,namespace,le)"
        f"(rate(http_request_duration_seconds_bucket[{WINDOW}]))))"
    )
    items = []
    for r in _results(_prom_query(q))[:20]:
        val = float(r["value"][1]) if r.get("value") else 0.0
        if not (math.isnan(val) or math.isinf(val)):
            items.append({
                "service": r["metric"].get("service", "?"),
                "namespace": r["metric"].get("namespace", "?"),
                "p99_latency_ms": round(val * 1000, 1),
            })
    return json.dumps({"status": "ok", "window": WINDOW, "services": items})


@tool
def get_node_resource_pressure(query: str = "") -> str:
    """
    Get CPU and memory utilization % per node from node_exporter.
    Identifies overloaded nodes. cpu_critical or mem_critical=true means >90% usage.
    """
    cpu_q = '100 - (avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)'
    mem_q = '100 * (1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes))'

    nodes: dict = {}
    for r in _results(_prom_query(cpu_q)):
        inst = r["metric"].get("instance", r["metric"].get("node", "?"))
        nodes.setdefault(inst, {})["cpu_pct"] = round(float(r["value"][1]), 1)
    for r in _results(_prom_query(mem_q)):
        inst = r["metric"].get("instance", r["metric"].get("node", "?"))
        nodes.setdefault(inst, {})["mem_pct"] = round(float(r["value"][1]), 1)

    result = [{
        "node": k,
        "cpu_pct": v.get("cpu_pct", "n/a"),
        "mem_pct": v.get("mem_pct", "n/a"),
        "cpu_critical": isinstance(v.get("cpu_pct"), float) and v["cpu_pct"] > config.cpu_critical_pct,
        "mem_critical": isinstance(v.get("mem_pct"), float) and v["mem_pct"] > config.memory_critical_pct,
    } for k, v in nodes.items()]
    return json.dumps({"status": "ok", "nodes": result})


@tool
def get_oom_kill_events(query: str = "") -> str:
    """
    Detect OOM killed containers via kube_pod_container_status_last_terminated_reason.
    OOMKilled pods need higher memory limits or a memory leak fix.
    Returns all affected pods with namespace and container name.
    """
    q = 'kube_pod_container_status_last_terminated_reason{reason="OOMKilled"} == 1'
    pods = [{
        "pod": r["metric"].get("pod", "?"),
        "namespace": r["metric"].get("namespace", "?"),
        "container": r["metric"].get("container", "?"),
    } for r in _results(_prom_query(q))]
    return json.dumps({"status": "ok", "oom_count": len(pods), "pods": pods})


@tool
def detect_metric_anomaly(pod_name: str, namespace: str = "default",
                           metric: str = "cpu") -> str:
    """
    Run z-score anomaly detection on a pod's CPU or memory usage over the last hour.
    z_score > 2.5 means statistically anomalous vs the pod's own baseline.
    metric: "cpu" or "memory"
    Format: detect_metric_anomaly(pod_name="my-pod", namespace="default", metric="cpu")
    """
    end = time.time()
    start = end - 3600

    if metric == "memory":
        q = f'container_memory_working_set_bytes{{pod="{pod_name}",namespace="{namespace}",container!=""}}'
        unit = "memory_mb"
    else:
        q = f'rate(container_cpu_usage_seconds_total{{pod="{pod_name}",namespace="{namespace}",container!=""}}[2m])'
        unit = "cpu_millicores"

    data = _prom_range(q, start, end, step="120s")
    items = _results(data)
    if not items:
        return json.dumps({"status": "no_data", "pod": pod_name, "metric": metric})

    raw_values = [float(v[1]) for v in items[0].get("values", []) if v[1] not in ("NaN", "+Inf")]
    if not raw_values:
        return json.dumps({"status": "no_data", "pod": pod_name, "metric": metric})

    is_anom, z = _zscore(raw_values)
    if metric == "memory":
        current = round(raw_values[-1] / (1024 * 1024), 1)
        avg = round(sum(raw_values) / len(raw_values) / (1024 * 1024), 1)
    else:
        current = round(raw_values[-1] * 1000, 1)
        avg = round(sum(raw_values) / len(raw_values) * 1000, 1)

    return json.dumps({
        "pod": pod_name,
        "namespace": namespace,
        "metric": metric,
        "is_anomaly": is_anom,
        "z_score": z,
        f"current_{unit}": current,
        f"avg_{unit}": avg,
        "sample_count": len(raw_values),
    })
