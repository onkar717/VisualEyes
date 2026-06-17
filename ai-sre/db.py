"""
db.py — SQLite persistence for VisualEyes AI-SRE standalone CLI.
Stores incident reports and cluster snapshots locally for:
  - Historical browsing (veye-ai incidents)
  - Single-incident detail (veye-ai show <id>)
  - Report export (veye-ai report <id>)
  - MTTR tracking
"""

import json
import logging
import sqlite3
from contextlib import contextmanager
from datetime import datetime, timedelta
from pathlib import Path
from typing import Dict, List, Optional

logger = logging.getLogger("visualeyes.db")

DB_PATH = Path.home() / ".visualeyes" / "incidents.db"


@contextmanager
def get_conn():
    DB_PATH.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(str(DB_PATH))
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA journal_mode=WAL")
    conn.execute("PRAGMA foreign_keys=ON")
    try:
        yield conn
        conn.commit()
    except Exception:
        conn.rollback()
        raise
    finally:
        conn.close()


def init_db() -> None:
    with get_conn() as conn:
        conn.executescript("""
            CREATE TABLE IF NOT EXISTS incidents (
                id                    TEXT PRIMARY KEY,
                created_at            TEXT NOT NULL,
                updated_at            TEXT NOT NULL,
                severity              TEXT NOT NULL,
                title                 TEXT NOT NULL,
                category              TEXT DEFAULT 'unknown',
                status                TEXT NOT NULL DEFAULT 'OPEN',
                affected_namespaces   TEXT DEFAULT '[]',
                root_cause            TEXT DEFAULT '',
                contributing_factors  TEXT DEFAULT '[]',
                confidence_score      INTEGER DEFAULT 0,
                runbook_used          TEXT,
                detected_at           TEXT,
                mitigated_at          TEXT,
                resolved_at           TEXT,
                mttr_seconds          INTEGER,
                llm_model             TEXT,
                scan_duration_seconds REAL,
                raw_json              TEXT NOT NULL
            );

            CREATE TABLE IF NOT EXISTS cluster_snapshots (
                id               INTEGER PRIMARY KEY AUTOINCREMENT,
                timestamp        TEXT NOT NULL,
                total_nodes      INTEGER,
                ready_nodes      INTEGER,
                total_pods       INTEGER,
                running_pods     INTEGER,
                pending_pods     INTEGER,
                failed_pods      INTEGER,
                crashloop_pods   INTEGER,
                open_incidents   INTEGER,
                health_score     REAL
            );

            CREATE INDEX IF NOT EXISTS idx_inc_severity  ON incidents(severity);
            CREATE INDEX IF NOT EXISTS idx_inc_status    ON incidents(status);
            CREATE INDEX IF NOT EXISTS idx_inc_created   ON incidents(created_at);
            CREATE INDEX IF NOT EXISTS idx_snap_ts       ON cluster_snapshots(timestamp);
        """)
    logger.debug("DB initialised at %s", DB_PATH)


def save_incident(report: dict) -> None:
    now = datetime.utcnow().isoformat()
    with get_conn() as conn:
        conn.execute("""
            INSERT OR REPLACE INTO incidents
            (id, created_at, updated_at, severity, title, category, status,
             affected_namespaces, root_cause, contributing_factors,
             confidence_score, runbook_used, detected_at, mitigated_at,
             resolved_at, mttr_seconds, llm_model, scan_duration_seconds, raw_json)
            VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
        """, (
            report.get("id", f"INC-{now[:10].replace('-','')}"),
            report.get("created_at", now),
            now,
            report.get("severity", "SEV4"),
            report.get("title", "Unknown Incident"),
            report.get("category", "unknown"),
            report.get("status", "OPEN"),
            json.dumps(report.get("affected_namespaces", [])),
            report.get("root_cause", ""),
            json.dumps(report.get("contributing_factors", [])),
            report.get("confidence", report.get("confidence_score", 0)),
            report.get("runbook_used"),
            report.get("detected_at", now),
            report.get("mitigated_at"),
            report.get("resolved_at"),
            report.get("mttr_seconds"),
            report.get("llm_model", ""),
            report.get("scan_duration_seconds"),
            json.dumps(report),
        ))


def get_incidents(
    severity: Optional[str] = None,
    status: Optional[str] = None,
    hours: int = 0,
    limit: int = 50,
) -> List[dict]:
    clauses, params = [], []
    if severity:
        clauses.append("severity = ?")
        params.append(severity.upper())
    if status:
        clauses.append("status = ?")
        params.append(status.upper())
    if hours > 0:
        since = (datetime.utcnow() - timedelta(hours=hours)).isoformat()
        clauses.append("created_at >= ?")
        params.append(since)
    where = ("WHERE " + " AND ".join(clauses)) if clauses else ""
    params.append(limit)
    with get_conn() as conn:
        rows = conn.execute(
            f"SELECT raw_json FROM incidents {where} ORDER BY created_at DESC LIMIT ?",
            params,
        ).fetchall()
    result = []
    for row in rows:
        try:
            result.append(json.loads(row["raw_json"]))
        except Exception as e:
            logger.warning("deserialize failed: %s", e)
    return result


def get_incident_by_id(incident_id: str) -> Optional[dict]:
    with get_conn() as conn:
        row = conn.execute(
            "SELECT raw_json FROM incidents WHERE id = ?", (incident_id,)
        ).fetchone()
    if not row:
        return None
    return json.loads(row["raw_json"])


def update_status(incident_id: str, status: str) -> None:
    now = datetime.utcnow().isoformat()
    extra_col = None
    if status == "MITIGATED":
        extra_col = "mitigated_at"
    elif status == "RESOLVED":
        extra_col = "resolved_at"
    with get_conn() as conn:
        conn.execute(
            "UPDATE incidents SET status = ?, updated_at = ? WHERE id = ?",
            (status.upper(), now, incident_id),
        )
        if extra_col:
            conn.execute(
                f"UPDATE incidents SET {extra_col} = ? WHERE id = ?",
                (now, incident_id),
            )


def get_mttr_stats() -> Dict[str, dict]:
    with get_conn() as conn:
        rows = conn.execute("""
            SELECT severity, AVG(mttr_seconds) as avg, COUNT(*) as cnt
            FROM incidents
            WHERE mttr_seconds IS NOT NULL
            GROUP BY severity
        """).fetchall()
    return {
        row["severity"]: {"avg_mttr_seconds": int(row["avg"] or 0), "count": row["cnt"]}
        for row in rows
    }


def open_incident_count() -> int:
    with get_conn() as conn:
        row = conn.execute(
            "SELECT COUNT(*) as c FROM incidents WHERE status IN ('OPEN','INVESTIGATING')"
        ).fetchone()
    return row["c"] if row else 0
