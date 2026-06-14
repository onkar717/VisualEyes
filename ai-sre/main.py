"""
VisualEyes AI-SRE FastAPI service.
Wraps CrewAI pipeline with a REST interface consumed by the Go server.
"""
import logging
import os
import sys
from contextlib import asynccontextmanager
from typing import Any, Dict, List, Optional

import uvicorn
from fastapi import FastAPI, HTTPException, BackgroundTasks
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field

from .config import config
from .pipeline import run_pipeline

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
    handlers=[logging.StreamHandler(sys.stdout)],
)
logger = logging.getLogger("visualeyes.ai-sre")


@asynccontextmanager
async def lifespan(app: FastAPI):
    try:
        config.validate()
        logger.info("AI-SRE service starting: model=%s port=%d", config.llm_model, config.port)
    except ValueError as e:
        logger.error("Config validation failed: %s", e)
        sys.exit(1)
    yield
    logger.info("AI-SRE service shutting down")


app = FastAPI(
    title="VisualEyes AI-SRE",
    description="CrewAI-powered 6-agent RCA engine for VisualEyes",
    version="1.0.0",
    lifespan=lifespan,
)


# ── Request / Response schemas ─────────────────────────────────────────────────

class AlertInfo(BaseModel):
    rule_name: str = ""
    severity: str = ""
    message: str = ""
    resource_id: str = ""
    namespace: str = "default"
    value: float = 0.0
    threshold: float = 0.0


class PrebuiltContext(BaseModel):
    recent_metrics: List[Dict[str, Any]] = Field(default_factory=list)
    related_metrics: List[Dict[str, Any]] = Field(default_factory=list)
    pod_logs: List[Dict[str, Any]] = Field(default_factory=list)
    prev_logs: List[Dict[str, Any]] = Field(default_factory=list)
    k8s_events: List[Dict[str, Any]] = Field(default_factory=list)
    log_classification: Dict[str, Any] = Field(default_factory=dict)
    anomalies: List[Dict[str, Any]] = Field(default_factory=list)


class RCARequest(BaseModel):
    alert_id: int
    alert: AlertInfo = Field(default_factory=AlertInfo)
    context: PrebuiltContext = Field(default_factory=PrebuiltContext)
    go_callback_url: str = ""  # Go /internal/rca/stage-event endpoint
    dry_run: bool = False


class CommandItem(BaseModel):
    command: str
    description: str = ""
    is_auto_safe: bool = False
    risk: str = "medium"
    step: int = 1


class ServiceImpact(BaseModel):
    service_name: str
    namespace: str
    impact_level: str = "degraded"


class RCAResponse(BaseModel):
    alert_id: int
    has_issue: bool
    severity: str
    category: str
    title: str
    root_cause: str
    explanation: str
    contributing_factors: List[str] = Field(default_factory=list)
    confidence: int
    commands: List[CommandItem] = Field(default_factory=list)
    affected_namespaces: List[str] = Field(default_factory=list)
    affected_services: List[ServiceImpact] = Field(default_factory=list)
    runbook_used: Optional[str] = None
    scan_duration_seconds: float = 0.0
    llm_model: str = ""
    error: Optional[str] = None


# ── Routes ─────────────────────────────────────────────────────────────────────

@app.get("/health")
async def health():
    return {
        "status": "healthy",
        "model": config.llm_model,
        "provider": config.llm_provider,
        "dry_run": config.dry_run,
    }


@app.get("/config")
async def get_config():
    return {
        "llm_model": config.llm_model,
        "llm_provider": config.llm_provider,
        "prometheus_enabled": config.prometheus_enabled,
        "loki_enabled": config.loki_enabled,
        "namespaces": config.namespaces,
        "dry_run": config.dry_run,
        "auto_remediate": config.auto_remediate,
    }


@app.post("/run-rca", response_model=RCAResponse)
async def run_rca(req: RCARequest):
    """
    Run the 6-agent CrewAI RCA pipeline for one alert.
    The Go server calls this endpoint when an alert enters the RCA worker pool.
    Stage callbacks are fired to req.go_callback_url as each agent completes.
    """
    logger.info("run-rca request: alert_id=%d pod=%s ns=%s",
                req.alert_id, req.alert.resource_id, req.alert.namespace)

    alert_ctx = {
        "alert": req.alert.model_dump(),
        "recent_metrics": req.context.recent_metrics,
        "related_metrics": req.context.related_metrics,
        "pod_logs": req.context.pod_logs,
        "prev_logs": req.context.prev_logs,
        "k8s_events": req.context.k8s_events,
        "log_classification": req.context.log_classification,
        "anomalies": req.context.anomalies,
    }

    # Override dry_run if request specifies it
    if req.dry_run:
        config.dry_run = True

    try:
        report = run_pipeline(
            alert_ctx=alert_ctx,
            alert_id=req.alert_id,
            go_callback_url=req.go_callback_url,
        )
    except Exception as e:
        logger.exception("run-rca failed: alert_id=%d", req.alert_id)
        raise HTTPException(status_code=500, detail=str(e))

    # Normalise commands field — may be list of dicts or already CommandItems
    raw_commands = report.get("commands", [])
    commands = []
    for i, cmd in enumerate(raw_commands, 1):
        if isinstance(cmd, dict):
            commands.append(CommandItem(
                command=cmd.get("command", ""),
                description=cmd.get("description", ""),
                is_auto_safe=cmd.get("is_auto_safe", False),
                risk=cmd.get("risk", "medium"),
                step=cmd.get("step", i),
            ))

    # Normalise affected_services
    raw_services = report.get("affected_services", [])
    services = []
    for svc in raw_services:
        if isinstance(svc, dict):
            services.append(ServiceImpact(
                service_name=svc.get("service_name", ""),
                namespace=svc.get("namespace", ""),
                impact_level=svc.get("impact_level", "degraded"),
            ))

    return RCAResponse(
        alert_id=req.alert_id,
        has_issue=report.get("has_issue", True),
        severity=report.get("severity", "SEV3"),
        category=report.get("category", "unknown"),
        title=report.get("title", ""),
        root_cause=report.get("root_cause", ""),
        explanation=report.get("explanation", ""),
        contributing_factors=report.get("contributing_factors", []),
        confidence=int(report.get("confidence", 50)),
        commands=commands,
        affected_namespaces=report.get("affected_namespaces", []),
        affected_services=services,
        runbook_used=report.get("runbook_used"),
        scan_duration_seconds=report.get("scan_duration_seconds", 0.0),
        llm_model=report.get("llm_model", config.llm_model),
        error=report.get("error"),
    )


if __name__ == "__main__":
    uvicorn.run(
        "ai_sre.main:app",
        host=config.host,
        port=config.port,
        reload=os.getenv("AI_SRE_RELOAD", "false").lower() == "true",
        log_level="info",
    )
