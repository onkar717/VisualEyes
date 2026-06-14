import os
from dataclasses import dataclass, field
from typing import List, Optional
from dotenv import load_dotenv

load_dotenv(dotenv_path=os.path.expanduser("~/.visualeyes/.env"))
load_dotenv()


@dataclass
class AISREConfig:
    # ─── LLM ───────────────────────────────────────────────────────────────────
    llm_provider: str      = os.getenv("LLM_PROVIDER", "groq")
    llm_model: str         = os.getenv("LLM_MODEL", "groq/llama-3.3-70b-versatile")
    llm_temperature: float = float(os.getenv("LLM_TEMPERATURE", "0.1"))
    llm_max_tokens: int    = int(os.getenv("LLM_MAX_TOKENS", "2048"))

    # ─── Kubernetes ────────────────────────────────────────────────────────────
    namespaces: List[str] = field(
        default_factory=lambda: os.getenv("K8S_NAMESPACES", "default").split(",")
    )
    scan_all_namespaces: bool = os.getenv("K8S_SCAN_ALL", "false").lower() == "true"

    # ─── Prometheus ────────────────────────────────────────────────────────────
    prometheus_url: str     = os.getenv("PROMETHEUS_URL", "http://localhost:9090")
    prometheus_enabled: bool = os.getenv("PROMETHEUS_ENABLED", "true").lower() == "true"
    metrics_window: str     = os.getenv("METRICS_WINDOW", "5m")

    # ─── Loki ──────────────────────────────────────────────────────────────────
    loki_url: str     = os.getenv("LOKI_URL", "http://localhost:3100")
    loki_enabled: bool = os.getenv("LOKI_ENABLED", "false").lower() == "true"

    # ─── Thresholds ────────────────────────────────────────────────────────────
    cpu_critical_pct: float    = float(os.getenv("CPU_CRITICAL_PCT", "90.0"))
    memory_critical_pct: float = float(os.getenv("MEMORY_CRITICAL_PCT", "90.0"))
    restart_critical: int      = int(os.getenv("RESTART_CRITICAL_COUNT", "10"))
    error_rate_critical: float = float(os.getenv("ERROR_RATE_CRITICAL_PCT", "5.0"))

    # ─── Execution ─────────────────────────────────────────────────────────────
    dry_run: bool         = os.getenv("DRY_RUN", "true").lower() == "true"
    auto_remediate: bool  = os.getenv("AUTO_REMEDIATE", "false").lower() == "true"
    pod_log_tail: int     = int(os.getenv("POD_LOG_TAIL_LINES", "150"))
    agent_timeout_s: int  = int(os.getenv("AGENT_TIMEOUT_SECONDS", "120"))
    max_retries: int      = int(os.getenv("MAX_RETRIES", "3"))

    # ─── Runbooks ──────────────────────────────────────────────────────────────
    runbooks_dir: str = os.getenv(
        "RUNBOOKS_DIR",
        os.path.join(os.path.dirname(__file__), "..", "server", "rca", "runbooks"),
    )

    # ─── Server ────────────────────────────────────────────────────────────────
    host: str = os.getenv("AI_SRE_HOST", "0.0.0.0")
    port: int = int(os.getenv("AI_SRE_PORT", "8001"))

    # ─── API Keys ──────────────────────────────────────────────────────────────
    groq_api_key: Optional[str]      = os.getenv("GROQ_API_KEY")
    mistral_api_key: Optional[str]   = os.getenv("MISTRAL_API_KEY")
    openai_api_key: Optional[str]    = os.getenv("OPENAI_API_KEY")
    anthropic_api_key: Optional[str] = os.getenv("ANTHROPIC_API_KEY")
    gemini_api_key: Optional[str]    = os.getenv("GEMINI_API_KEY")

    def validate(self) -> None:
        key_map = {
            "groq": self.groq_api_key,
            "mistral": self.mistral_api_key,
            "openai": self.openai_api_key,
            "anthropic": self.anthropic_api_key,
            "gemini": self.gemini_api_key,
        }
        provider = self.llm_provider.lower()
        if provider in key_map and not key_map[provider]:
            raise ValueError(
                f"{provider.upper()}_API_KEY required when LLM_PROVIDER={provider}"
            )
        # Export for LiteLLM / CrewAI
        for env_key, val in [
            ("GROQ_API_KEY", self.groq_api_key),
            ("MISTRAL_API_KEY", self.mistral_api_key),
            ("OPENAI_API_KEY", self.openai_api_key),
            ("ANTHROPIC_API_KEY", self.anthropic_api_key),
            ("GEMINI_API_KEY", self.gemini_api_key),
        ]:
            if val:
                os.environ[env_key] = val

        os.environ["CREWAI_TRACING_ENABLED"] = "false"
        os.environ["CREWAI_DISABLE_TRACING"] = "true"
        os.environ.setdefault("OTEL_SDK_DISABLED", "true")


config = AISREConfig()
