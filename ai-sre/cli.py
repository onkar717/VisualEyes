#!/usr/bin/env python3
"""
VisualEyes AI-SRE standalone CLI.
Runs CrewAI pipeline directly no Go server required.

Usage:
  python -m ai_sre.cli scan               Run one AI-SRE scan
  python -m ai_sre.cli scan --apply       Scan and interactively apply remediation
  python -m ai_sre.cli scan --dry-run     Preview what would be scanned
  python -m ai_sre.cli watch              Continuous monitoring loop
  python -m ai_sre.cli status             Cluster health snapshot (no LLM)
  python -m ai_sre.cli config             Show active configuration
"""
import json
import logging
import os
import signal
import sys
import time
from datetime import datetime

import click
from rich.console import Console
from rich.panel import Panel
from rich.table import Table
from rich.text import Text
from rich import box

from .config import config
from .pipeline import run_pipeline
from .tools.k8s_tools import (
    list_pods_all_namespaces,
    get_cluster_events,
    get_namespace_summary,
)

console = Console()
logging.basicConfig(level=logging.WARNING)
logger = logging.getLogger("visualeyes.cli")

BANNER = """[bold cyan]
 ██╗   ██╗██╗███████╗██╗   ██╗ █████╗ ██╗     ███████╗██╗   ██╗███████╗███████╗
 ██║   ██║██║██╔════╝██║   ██║██╔══██╗██║     ██╔════╝╚██╗ ██╔╝██╔════╝██╔════╝
 ██║   ██║██║███████╗██║   ██║███████║██║     █████╗   ╚████╔╝ █████╗  ███████╗
 ╚██╗ ██╔╝██║╚════██║██║   ██║██╔══██║██║     ██╔══╝    ╚██╔╝  ██╔══╝  ╚════██║
  ╚████╔╝ ██║███████║╚██████╔╝██║  ██║███████╗███████╗   ██║   ███████╗███████║
   ╚═══╝  ╚═╝╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚══════╝╚══════╝   ╚═╝   ╚══════╝╚══════╝
[/bold cyan][bright_black]  AI-SRE · CrewAI-powered Kubernetes Root Cause Analysis[/bright_black]
"""

SEV_STYLES = {
    "SEV1": "bold red",
    "SEV2": "bold yellow",
    "SEV3": "bold cyan",
    "SEV4": "bold green",
}


def print_banner():
    console.print(BANNER)


def _sev_style(sev: str) -> str:
    return SEV_STYLES.get(sev.upper(), "white")


def _render_report(report: dict) -> None:
    sev = report.get("severity", "SEV3")
    category = report.get("category", "unknown")
    has_issue = report.get("has_issue", True)
    confidence = report.get("confidence", 0)

    if not has_issue:
        console.print(Panel(
            "[bold green]✓ Cluster is healthy no issues detected[/bold green]",
            border_style="green",
            title="[bold green]VisualEyes RCA Result[/bold green]",
        ))
        return

    sev_style = _sev_style(sev)
    title_text = Text()
    title_text.append(f"[{sev}] ", style=sev_style)
    title_text.append(report.get("title", "Unnamed Incident"))

    body = Text()
    body.append("Root Cause:    ", style="bold bright_white")
    body.append(report.get("root_cause", "") + "\n\n", style="white")
    body.append("Explanation:   ", style="bold bright_white")
    body.append(report.get("explanation", "") + "\n\n", style="white")

    factors = report.get("contributing_factors", [])
    if factors:
        body.append("Contributing:  ", style="bold bright_white")
        body.append(", ".join(factors) + "\n\n")

    namespaces = report.get("affected_namespaces", [])
    if namespaces:
        body.append("Namespaces:    ", style="bold bright_white")
        body.append(", ".join(namespaces) + "\n")

    body.append("Category:      ", style="bold bright_white")
    body.append(f"{category}   ")
    body.append("Confidence:    ", style="bold bright_white")
    body.append(f"{confidence}%   ")
    body.append("Model:         ", style="bold bright_white")
    body.append(report.get("llm_model", config.llm_model) + "\n")

    runbook = report.get("runbook_used")
    if runbook:
        body.append("Runbook:       ", style="bold bright_white")
        body.append(runbook + "\n")

    console.print(Panel(body, title=title_text, border_style=sev_style))

    commands = report.get("commands", [])
    if commands:
        console.print("\n[bold bright_white]Remediation Plan:[/bold bright_white]")
        table = Table(box=box.ROUNDED, show_header=True, header_style="bold bright_black")
        table.add_column("Step", width=4)
        table.add_column("Command", style="cyan")
        table.add_column("Safe", width=6)
        table.add_column("Risk", width=8)
        table.add_column("Description")
        for cmd in commands:
            safe_icon = "[green]✓[/green]" if cmd.get("is_auto_safe") else "[red]✗[/red]"
            risk = cmd.get("risk", "medium")
            risk_style = "green" if risk == "low" else ("yellow" if risk == "medium" else "red")
            table.add_row(
                str(cmd.get("step", "?")),
                cmd.get("command", ""),
                safe_icon,
                f"[{risk_style}]{risk}[/{risk_style}]",
                cmd.get("description", ""),
            )
        console.print(table)


def _apply_remediation(report: dict, dry_run: bool = True) -> None:
    from .tools.runbook_tools import execute_safe_command
    commands = report.get("commands", [])
    if not commands:
        console.print("[yellow]No remediation commands available.[/yellow]")
        return

    applied, skipped, failed = 0, 0, 0
    for cmd in commands:
        command = cmd.get("command", "")
        cmd.get("is_auto_safe", False)
        console.print(f"\n  [bold]Step {cmd.get('step','?')}:[/bold] {cmd.get('description','')}")
        console.print(f"  Command: [cyan]{command}[/cyan]")

        choice = console.input("  Execute? [bold][[green]y[/green]/[red]N[/red]/[yellow]dry[/yellow]][/bold]: ").strip().lower()
        if choice in ("dry", "d"):
            result_json = execute_safe_command.func(command=command, dry_run=True)
            result = json.loads(result_json)
            console.print(f"  [yellow]~[/yellow] [dim][DRY RUN] {command}[/dim]")
        elif choice in ("y", "yes"):
            result_json = execute_safe_command.func(command=command, dry_run=dry_run)
            result = json.loads(result_json)
            if result.get("status") == "executed" and result.get("success"):
                console.print("  [green]✓[/green] Done")
                applied += 1
            elif result.get("status") == "blocked":
                console.print(f"  [red]✗[/red] Blocked: {result.get('reason')}")
                skipped += 1
            elif result.get("status") == "dry_run":
                console.print("  [yellow]~[/yellow] Dry run (DRY_RUN=true in config)")
                skipped += 1
            else:
                console.print(f"  [red]✗[/red] Failed: {result.get('stderr') or result.get('reason')}")
                failed += 1
        else:
            console.print("  [dim]-[/dim] Skipped.")
            skipped += 1

    console.print(
        f"\n  Applied: [green]{applied}[/green]  "
        f"Skipped: [dim]{skipped}[/dim]  "
        f"Failed: [red]{failed}[/red]"
    )


def _quick_cluster_status() -> dict:
    """Get a fast cluster overview without LLM."""
    import json as _json
    result = {"namespaces": {}, "problem_pods": [], "events": []}
    try:
        ns_raw = get_namespace_summary.func(query="")
        result["namespaces"] = _json.loads(ns_raw)
    except Exception as e:
        result["namespaces_error"] = str(e)

    try:
        events_raw = get_cluster_events.func(namespace="", warning_only=True, limit=10)
        result["events"] = _json.loads(events_raw)
    except Exception as e:
        result["events_error"] = str(e)

    try:
        pods_raw = list_pods_all_namespaces.func(query="")
        pods = _json.loads(pods_raw)
        result["problem_pods"] = [
            p for p in pods
            if p.get("phase") not in ("Running", "Succeeded") or p.get("restarts", 0) > 5
        ]
    except Exception as e:
        result["pods_error"] = str(e)

    return result


# ── CLI ───────────────────────────────────────────────────────────────────────

@click.group()
@click.option("--debug", is_flag=True, help="Enable debug logging")
@click.option("--model", default="", envvar="LLM_MODEL",
              help="Override LLM model (e.g. groq/llama-3.3-70b-versatile)")
@click.pass_context
def cli(ctx: click.Context, debug: bool, model: str):
    """VisualEyes AI-SRE CrewAI-powered Kubernetes RCA engine."""
    ctx.ensure_object(dict)
    if debug:
        logging.getLogger("visualeyes").setLevel(logging.DEBUG)
        logging.getLogger().addHandler(logging.StreamHandler())
    if model:
        config.llm_model = model
        os.environ["LLM_MODEL"] = model
    try:
        config.validate()
    except ValueError as e:
        console.print(f"[red bold]Config error:[/red bold] {e}")
        console.print("[dim]Set the required API key environment variable and retry.[/dim]")
        sys.exit(1)
    ctx.obj["dry_run"] = config.dry_run


@cli.command()
@click.option("--apply", is_flag=True, help="Interactively apply remediation after scan")
@click.option("--dry-run/--no-dry-run", default=True, show_default=True,
              help="Dry run preview commands without executing")
@click.option("--namespace", "-n", default="", help="Target namespace (default: all from config)")
def scan(apply: bool, dry_run: bool, namespace: str):
    """Run a single 6-agent AI-SRE scan across the cluster."""
    print_banner()

    if namespace:
        config.namespaces = [namespace]

    console.print(Panel(
        Text.assemble(
            ("LLM Model: ", "bright_black"), (config.llm_model, "cyan"), ("   ", ""),
            ("Namespaces: ", "bright_black"), (", ".join(config.namespaces), "cyan"), ("   ", ""),
            ("Dry Run: ", "bright_black"),
            (str(dry_run), "yellow"),
        ),
        border_style="bright_black",
    ))
    console.print()
    console.print("[cyan bold]Starting AI-SRE scan 6 agents deploying...[/cyan bold]")
    console.print("[bright_black]Triage → Metrics → Logs → Infra → Remediation → Commander[/bright_black]")
    console.print()

    start = time.time()
    try:
        report = run_pipeline(
            alert_ctx={"alert": {}, "anomalies": [], "k8s_events": [], "recent_metrics": []},
            alert_id=0,
            go_callback_url="",
        )
    except KeyboardInterrupt:
        console.print("\n[yellow]Scan interrupted.[/yellow]")
        return
    except Exception as e:
        console.print(f"\n[red bold]Scan failed:[/red bold] {e}")
        logger.exception("scan failed")
        sys.exit(1)

    duration = time.time() - start
    console.print(f"\n[bright_black]Scan completed in {duration:.1f}s[/bright_black]\n")
    _render_report(report)

    if apply and report.get("has_issue") and report.get("commands"):
        console.print()
        _apply_remediation(report, dry_run=dry_run)


@cli.command()
@click.option("--interval", default=config.scan_interval_seconds, show_default=True,
              help="Scan interval in seconds")
@click.option("--apply", is_flag=True, help="Prompt to apply remediation for SEV1/2 findings")
@click.option("--namespace", "-n", default="", help="Target namespace")
def watch(interval: int, apply: bool, namespace: str):
    """Continuous monitoring loop with live incident display."""
    print_banner()

    if namespace:
        config.namespaces = [namespace]

    console.print(
        f"[cyan bold]Continuous watcher starting[/cyan bold] "
        f"[bright_black](interval={interval}s, Ctrl+C to stop)[/bright_black]\n"
    )

    scan_count = 0

    def graceful_exit(sig, frame):
        console.print("\n[yellow]Watcher stopped.[/yellow]")
        sys.exit(0)

    signal.signal(signal.SIGINT, graceful_exit)
    signal.signal(signal.SIGTERM, graceful_exit)

    while True:
        scan_count += 1
        ts = datetime.utcnow().strftime("%Y-%m-%d %H:%M:%S UTC")
        console.rule(f"[cyan]Scan #{scan_count}  ·  {ts}[/cyan]")

        try:
            report = run_pipeline(
                alert_ctx={"alert": {}, "anomalies": [], "k8s_events": [], "recent_metrics": []},
                alert_id=scan_count,
                go_callback_url="",
            )
            _render_report(report)

            sev = report.get("severity", "SEV4")
            if report.get("has_issue") and sev in ("SEV1", "SEV2"):
                if apply:
                    _apply_remediation(report, dry_run=config.dry_run)
                else:
                    console.print("\n[yellow]Use --apply to interactively remediate.[/yellow]")

        except Exception as e:
            console.print(f"[red]Scan #{scan_count} error: {e}[/red]")

        console.print(f"\n[dim]Next scan in {interval}s press Ctrl+C to stop[/dim]\n")
        time.sleep(interval)


@cli.command()
def status():
    """Show cluster health snapshot (no LLM instant, from K8s API directly)."""
    print_banner()
    console.print("[cyan]Collecting cluster status (no LLM direct K8s API)...[/cyan]\n")

    data = _quick_cluster_status()

    # Namespace table
    namespaces = data.get("namespaces", [])
    if isinstance(namespaces, list) and namespaces:
        table = Table(title="Namespace Summary", box=box.ROUNDED, show_header=True)
        table.add_column("Namespace", style="cyan")
        table.add_column("Total", justify="right")
        table.add_column("Running", justify="right", style="green")
        table.add_column("Pending", justify="right", style="yellow")
        table.add_column("Failed", justify="right", style="red")
        table.add_column("CrashLoop", justify="right", style="bold red")
        for ns in namespaces:
            pods = ns.get("pods", {})
            cl = pods.get("crashloop", 0)
            cl_style = f"[bold red]{cl}[/bold red]" if cl > 0 else str(cl)
            table.add_row(
                ns["namespace"], str(pods.get("total", 0)),
                str(pods.get("running", 0)), str(pods.get("pending", 0)),
                str(pods.get("failed", 0)), cl_style,
            )
        console.print(table)

    # Problem pods
    problem_pods = data.get("problem_pods", [])
    if problem_pods:
        console.print()
        pt = Table(title="Problem Pods", box=box.ROUNDED, show_header=True)
        pt.add_column("Pod", style="yellow")
        pt.add_column("Namespace", style="cyan")
        pt.add_column("Phase")
        pt.add_column("Restarts", justify="right")
        for pod in problem_pods[:15]:
            pt.add_row(
                pod.get("pod", "?"), pod.get("ns", "?"),
                pod.get("phase", "?"), str(pod.get("restarts", 0)),
            )
        console.print(pt)
    else:
        console.print("\n[green bold]✓ No problem pods detected.[/green bold]")

    # Recent warning events
    events = data.get("events", [])
    if events:
        console.print()
        et = Table(title="Recent Warning Events", box=box.ROUNDED, show_header=True)
        et.add_column("Object", style="yellow")
        et.add_column("Reason", style="cyan")
        et.add_column("Count", justify="right")
        et.add_column("Message")
        for ev in events[:10]:
            et.add_row(
                ev.get("object", "?"), ev.get("reason", "?"),
                str(ev.get("count", 0)), ev.get("msg", "")[:80],
            )
        console.print(et)
    else:
        console.print("\n[green bold]✓ No warning events.[/green bold]")


@cli.command("config")
def show_config():
    """Show active AI-SRE configuration."""
    print_banner()
    table = Table(title="Active Configuration", box=box.ROUNDED, show_header=False)
    table.add_column("Key", style="bold bright_white")
    table.add_column("Value", style="cyan")

    rows = [
        ("LLM Model",       config.llm_model),
        ("LLM Provider",    config.llm_provider),
        ("Temperature",     str(config.llm_temperature)),
        ("Max Tokens",      str(config.llm_max_tokens)),
        ("Namespaces",      ", ".join(config.namespaces)),
        ("Prometheus URL",  config.prometheus_url),
        ("Prometheus",      "enabled" if config.prometheus_enabled else "disabled"),
        ("Loki URL",        config.loki_url),
        ("Loki",            "enabled" if config.loki_enabled else "disabled"),
        ("Dry Run",         str(config.dry_run)),
        ("Auto Remediate",  str(config.auto_remediate)),
        ("Runbooks Dir",    config.runbooks_dir),
        ("Agent Timeout",   f"{config.agent_timeout_s}s"),
        ("Max Retries",     str(config.max_retries)),
        ("Log Tail Lines",  str(config.pod_log_tail)),
    ]
    for key, val in rows:
        table.add_row(key, val)
    console.print(table)


if __name__ == "__main__":
    cli()
