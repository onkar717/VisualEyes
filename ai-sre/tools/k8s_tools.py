"""
K8s tools for VisualEyes AI-SRE CrewAI agents.
Wraps kubernetes Python client. All tools return JSON strings for LLM consumption.
"""
import json
import logging
from typing import Optional

from crewai.tools import tool
from kubernetes import client, config as kube_config

logger = logging.getLogger("visualeyes.tools.k8s")

_v1: Optional[client.CoreV1Api] = None
_apps_v1: Optional[client.AppsV1Api] = None
_autoscaling_v1: Optional[client.AutoscalingV1Api] = None


def _get_v1() -> client.CoreV1Api:
    global _v1
    if _v1 is None:
        try:
            kube_config.load_incluster_config()
        except kube_config.ConfigException:
            kube_config.load_kube_config()
        _v1 = client.CoreV1Api()
    return _v1


def _get_apps() -> client.AppsV1Api:
    global _apps_v1
    if _apps_v1 is None:
        _get_v1()
        _apps_v1 = client.AppsV1Api()
    return _apps_v1


def _get_autoscaling() -> client.AutoscalingV1Api:
    global _autoscaling_v1
    if _autoscaling_v1 is None:
        _get_v1()
        _autoscaling_v1 = client.AutoscalingV1Api()
    return _autoscaling_v1


@tool
def list_pods_all_namespaces(query: str = "") -> str:
    """
    List all pods across all namespaces with their status, restart counts, and container states.
    Skips healthy kube-system pods to reduce token usage. Use for initial cluster triage.
    Returns JSON array of pod summaries.
    """
    try:
        v1 = _get_v1()
        pods = v1.list_pod_for_all_namespaces()
        result = []
        for p in pods.items:
            if p.metadata.namespace == "kube-system" and p.status.phase == "Running":
                skip = True
                if p.status.container_statuses:
                    for cs in p.status.container_statuses:
                        if cs.restart_count > 5:
                            skip = False
                if skip:
                    continue

            restart_count = 0
            states = []
            if p.status.container_statuses:
                for cs in p.status.container_statuses:
                    restart_count += cs.restart_count
                    state = "running"
                    if cs.state.waiting:
                        state = f"waiting:{cs.state.waiting.reason}"
                    elif cs.state.terminated:
                        state = f"exit:{cs.state.terminated.exit_code}:{cs.state.terminated.reason}"
                    states.append({
                        "name": cs.name,
                        "state": state,
                        "restarts": cs.restart_count,
                        "ready": cs.ready,
                    })

            result.append({
                "ns": p.metadata.namespace,
                "pod": p.metadata.name,
                "phase": p.status.phase,
                "restarts": restart_count,
                "node": p.spec.node_name,
                "states": states,
            })
        return json.dumps(result, default=str)
    except Exception as e:
        logger.error("list_pods_all_namespaces: %s", e)
        return json.dumps({"error": str(e)})


@tool
def describe_pod(pod_name: str, namespace: str = "default") -> str:
    """
    Get full details for a specific pod: events, conditions, container states,
    init containers, resource limits, and previous termination reasons.
    Essential for diagnosing CrashLoopBackOff and OOMKilled pods.
    Format: describe_pod(pod_name="my-pod", namespace="default")
    """
    try:
        v1 = _get_v1()
        pod = v1.read_namespaced_pod(pod_name, namespace)

        events = v1.list_namespaced_event(namespace)
        pod_events = [
            {
                "time": str(e.last_timestamp),
                "type": e.type,
                "reason": e.reason,
                "message": (e.message or "")[:200],
                "count": e.count,
            }
            for e in events.items
            if e.involved_object.name == pod_name
        ]

        init_states = []
        if pod.status.init_container_statuses:
            for ics in pod.status.init_container_statuses:
                state = "unknown"
                if ics.state.running:
                    state = "running"
                elif ics.state.waiting:
                    state = f"waiting:{ics.state.waiting.reason}"
                elif ics.state.terminated:
                    state = f"terminated:{ics.state.terminated.reason}:{ics.state.terminated.exit_code}"
                init_states.append({"name": ics.name, "state": state, "restarts": ics.restart_count})

        container_detail = []
        if pod.status.container_statuses:
            for cs in pod.status.container_statuses:
                detail: dict = {
                    "name": cs.name,
                    "ready": cs.ready,
                    "restart_count": cs.restart_count,
                    "image": cs.image,
                }
                if cs.state.waiting:
                    detail["waiting_reason"] = cs.state.waiting.reason
                    detail["waiting_message"] = cs.state.waiting.message
                if cs.state.terminated:
                    t = cs.state.terminated
                    detail["exit_code"] = t.exit_code
                    detail["exit_reason"] = t.reason
                if cs.last_state.terminated:
                    t = cs.last_state.terminated
                    detail["last_exit_code"] = t.exit_code
                    detail["last_reason"] = t.reason
                # Resource limits
                if pod.spec.containers:
                    for c in pod.spec.containers:
                        if c.name == cs.name and c.resources:
                            detail["limits"] = {k: str(v) for k, v in (c.resources.limits or {}).items()}
                            detail["requests"] = {k: str(v) for k, v in (c.resources.requests or {}).items()}
                container_detail.append(detail)

        return json.dumps({
            "name": pod_name,
            "namespace": namespace,
            "phase": pod.status.phase,
            "node": pod.spec.node_name,
            "start_time": str(pod.status.start_time),
            "conditions": [
                {"type": c.type, "status": c.status, "reason": c.reason, "message": c.message}
                for c in (pod.status.conditions or [])
            ],
            "init_containers": init_states,
            "containers": container_detail,
            "events": pod_events[-15:],
        }, default=str)
    except Exception as e:
        return json.dumps({"error": str(e)})


@tool
def get_pod_logs(pod_name: str, namespace: str = "default",
                 container: str = "", tail_lines: int = 100,
                 previous: bool = False) -> str:
    """
    Fetch recent logs from a pod container. Set previous=True to get logs from
    the last crashed container (critical for CrashLoopBackOff diagnosis).
    Format: get_pod_logs(pod_name="my-pod", namespace="default", tail_lines=100, previous=False)
    """
    try:
        v1 = _get_v1()
        kwargs: dict = dict(
            name=pod_name, namespace=namespace,
            tail_lines=tail_lines, timestamps=True, previous=previous,
        )
        if container:
            kwargs["container"] = container
        logs = v1.read_namespaced_pod_log(**kwargs)
        return logs or "No logs available."
    except Exception as e:
        return f"Error getting logs for {pod_name}: {e}"


@tool
def get_cluster_events(namespace: str = "", warning_only: bool = True, limit: int = 25) -> str:
    """
    Get recent Kubernetes events. Set warning_only=True (default) to focus on problems.
    Set namespace="" to search all namespaces. Returns deduped, sorted events.
    Format: get_cluster_events(namespace="production", warning_only=True, limit=25)
    """
    try:
        v1 = _get_v1()
        if namespace:
            events = v1.list_namespaced_event(namespace, limit=200)
        else:
            events = v1.list_event_for_all_namespaces(limit=200)

        items = events.items
        if warning_only:
            items = [e for e in items if e.type == "Warning"]

        items.sort(key=lambda e: e.last_timestamp or e.event_time or "", reverse=True)

        seen: set = set()
        deduped = []
        for e in items:
            key = (e.reason, e.involved_object.name, e.involved_object.namespace)
            if key not in seen:
                seen.add(key)
                deduped.append(e)

        return json.dumps([
            {
                "reason": e.reason,
                "object": f"{e.involved_object.kind}/{e.involved_object.name}",
                "namespace": e.involved_object.namespace,
                "msg": (e.message or "")[:200],
                "count": e.count,
                "type": e.type,
            }
            for e in deduped[:limit]
        ], default=str)
    except Exception as e:
        return json.dumps({"error": str(e)})


@tool
def get_node_health(query: str = "") -> str:
    """
    Get health status of all cluster nodes: Ready/NotReady, pressure conditions,
    taints, and allocatable vs capacity resources. Essential for infra diagnosis.
    """
    try:
        v1 = _get_v1()
        nodes = v1.list_node()
        result = []
        for n in nodes.items:
            conditions = {
                c.type: {"status": c.status, "reason": c.reason, "message": c.message}
                for c in n.status.conditions
            }
            taints = [
                {"key": t.key, "effect": t.effect, "value": t.value}
                for t in (n.spec.taints or [])
            ]
            cap = n.status.capacity
            alloc = n.status.allocatable
            result.append({
                "name": n.metadata.name,
                "ready": conditions.get("Ready", {}).get("status") == "True",
                "unschedulable": bool(n.spec.unschedulable),
                "conditions": conditions,
                "taints": taints,
                "capacity": {
                    "cpu": str(cap.get("cpu", "?")),
                    "memory_mb": _ki_to_mb(str(cap.get("memory", "0"))),
                    "pods": str(cap.get("pods", "?")),
                },
                "allocatable": {
                    "cpu": str(alloc.get("cpu", "?")),
                    "memory_mb": _ki_to_mb(str(alloc.get("memory", "0"))),
                },
                "os": n.status.node_info.os_image,
                "kernel": n.status.node_info.kernel_version,
            })
        return json.dumps(result, default=str)
    except Exception as e:
        return json.dumps({"error": str(e)})


def _ki_to_mb(val: str) -> int:
    try:
        if "Ki" in val:
            return int(val.replace("Ki", "")) // 1024
        if "Mi" in val:
            return int(val.replace("Mi", ""))
        if "Gi" in val:
            return int(val.replace("Gi", "")) * 1024
    except Exception:
        pass
    return 0


@tool
def get_deployments_status(namespace: str = "") -> str:
    """
    List all deployments: desired vs ready vs available replicas.
    Identifies replica mismatches indicating rollout failures or pod crashes.
    Format: get_deployments_status(namespace="production") or "" for all namespaces.
    """
    try:
        apps = _get_apps()
        deploys = apps.list_namespaced_deployment(namespace) if namespace \
            else apps.list_deployment_for_all_namespaces()

        result = []
        for d in deploys.items:
            desired = d.spec.replicas or 0
            ready = d.status.ready_replicas or 0
            available = d.status.available_replicas or 0
            result.append({
                "namespace": d.metadata.namespace,
                "name": d.metadata.name,
                "desired": desired,
                "ready": ready,
                "available": available,
                "is_healthy": ready == desired and desired > 0,
                "image": d.spec.template.spec.containers[0].image
                    if d.spec.template.spec.containers else "?",
                "conditions": [
                    {"type": c.type, "status": c.status, "reason": c.reason, "message": c.message}
                    for c in (d.status.conditions or [])
                ],
            })
        return json.dumps(result, default=str)
    except Exception as e:
        return json.dumps({"error": str(e)})


@tool
def get_resource_quotas(namespace: str = "default") -> str:
    """
    Get resource quotas and limit ranges for a namespace.
    Essential for diagnosing Pending pods due to quota exhaustion.
    Format: get_resource_quotas(namespace="production")
    """
    try:
        v1 = _get_v1()
        quotas = v1.list_namespaced_resource_quota(namespace)
        limit_ranges = v1.list_namespaced_limit_range(namespace)
        result: dict = {"namespace": namespace, "quotas": [], "limit_ranges": []}
        for q in quotas.items:
            result["quotas"].append({
                "name": q.metadata.name,
                "hard": dict(q.status.hard or {}),
                "used": dict(q.status.used or {}),
            })
        for lr in limit_ranges.items:
            for lim in (lr.spec.limits or []):
                result["limit_ranges"].append({
                    "type": lim.type,
                    "default": dict(lim.default or {}),
                    "default_request": dict(lim.default_request or {}),
                    "max": dict(lim.max or {}),
                })
        return json.dumps(result, default=str)
    except Exception as e:
        return json.dumps({"error": str(e)})


@tool
def get_pvc_status(namespace: str = "") -> str:
    """
    Get PersistentVolumeClaim status. Identifies unbound PVCs that block pod
    scheduling or full volumes causing disk pressure.
    """
    try:
        v1 = _get_v1()
        pvcs = v1.list_namespaced_persistent_volume_claim(namespace) if namespace \
            else v1.list_persistent_volume_claim_for_all_namespaces()
        result = [{
            "namespace": p.metadata.namespace,
            "name": p.metadata.name,
            "phase": p.status.phase,
            "storage_class": p.spec.storage_class_name,
            "capacity": dict(p.status.capacity or {}),
            "is_bound": p.status.phase == "Bound",
        } for p in pvcs.items]
        return json.dumps(result, default=str)
    except Exception as e:
        return json.dumps({"error": str(e)})


@tool
def get_hpa_status(namespace: str = "") -> str:
    """
    Get HorizontalPodAutoscaler status: current vs desired replicas, min/max bounds.
    An HPA at max replicas means scaling is blocked often a root cause.
    """
    try:
        auto = _get_autoscaling()
        hpas = auto.list_namespaced_horizontal_pod_autoscaler(namespace) if namespace \
            else auto.list_horizontal_pod_autoscaler_for_all_namespaces()
        result = [{
            "namespace": h.metadata.namespace,
            "name": h.metadata.name,
            "target": h.spec.scale_target_ref.name,
            "min_replicas": h.spec.min_replicas,
            "max_replicas": h.spec.max_replicas,
            "current_replicas": h.status.current_replicas,
            "desired_replicas": h.status.desired_replicas,
            "current_cpu_pct": h.status.current_cpu_utilization_percentage,
            "target_cpu_pct": h.spec.target_cpu_utilization_percentage,
            "at_max": h.status.current_replicas >= h.spec.max_replicas,
        } for h in hpas.items]
        return json.dumps(result, default=str)
    except Exception as e:
        return json.dumps({"error": str(e)})


@tool
def get_namespace_summary(query: str = "") -> str:
    """
    Get pod counts by phase per namespace: running, pending, failed, crashloop.
    Good overview for multi-tenant cluster analysis and scoping the blast radius.
    """
    try:
        v1 = _get_v1()
        namespaces = v1.list_namespace()
        pods_all = v1.list_pod_for_all_namespaces()

        ns_pods: dict = {}
        for p in pods_all.items:
            ns = p.metadata.namespace
            if ns not in ns_pods:
                ns_pods[ns] = {"running": 0, "pending": 0, "failed": 0, "total": 0, "crashloop": 0}
            ns_pods[ns]["total"] += 1
            phase = p.status.phase or ""
            if phase == "Running":
                ns_pods[ns]["running"] += 1
            elif phase == "Pending":
                ns_pods[ns]["pending"] += 1
            elif phase in ("Failed", "Unknown"):
                ns_pods[ns]["failed"] += 1
            if p.status.container_statuses:
                for cs in p.status.container_statuses:
                    if cs.state.waiting and cs.state.waiting.reason == "CrashLoopBackOff":
                        ns_pods[ns]["crashloop"] += 1

        result = []
        for ns in namespaces.items:
            name = ns.metadata.name
            result.append({
                "namespace": name,
                "status": ns.status.phase,
                "pods": ns_pods.get(name, {"running": 0, "pending": 0, "failed": 0, "total": 0, "crashloop": 0}),
            })
        return json.dumps(result, default=str)
    except Exception as e:
        return json.dumps({"error": str(e)})
