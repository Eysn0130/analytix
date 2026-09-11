from __future__ import annotations

import re
from typing import Any

from app.core.db_engine import DuckDBEngine

_REVISION_TABLE = "analysis_revision_state"
_STATS_FLOW_SOURCE_KEY = "stats_flow_source"
_REVISION_REASONS = frozenset(
    {
        "bootstrap",
        "cleaning:run",
        "import:recycle_files",
        "import:restore_files",
        "source_revision_changed",
        "stats:delete_accounts",
        "stats:update_account_info",
    }
)


def project_analysis_revision_reason(value: Any) -> str:
    candidate = str(value or "").strip()
    if not candidate:
        return ""
    if candidate in _REVISION_REASONS:
        return candidate
    if re.fullmatch(r"(?:cleaning|import)-job:[0-9a-f]{8}-[0-9a-f-]{27}", candidate):
        return candidate
    return "source_revision_changed"


def ensure_analysis_revision_table(con: DuckDBEngine) -> None:
    con.execute(
        f"""
        CREATE TABLE IF NOT EXISTS {_REVISION_TABLE}(
            revision_key TEXT PRIMARY KEY,
            revision BIGINT NOT NULL,
            updated_at TIMESTAMP,
            reason TEXT
        )
        """
    )


def get_stats_flow_source_revision(con: DuckDBEngine) -> int:
    ensure_analysis_revision_table(con)
    rows = con.query(
        f"SELECT revision FROM {_REVISION_TABLE} WHERE revision_key=?",
        (_STATS_FLOW_SOURCE_KEY,),
    )
    if rows:
        try:
            return max(1, int(rows[0][0] or 1))
        except Exception:
            return 1
    con.execute(
        f"INSERT INTO {_REVISION_TABLE}(revision_key, revision, updated_at, reason) VALUES (?,?,NOW(),?)",
        (_STATS_FLOW_SOURCE_KEY, 1, "bootstrap"),
    )
    return 1


def read_stats_flow_source_revision(con: DuckDBEngine) -> int:
    try:
        table_rows = con.query(
            "SELECT COUNT(1) FROM information_schema.tables "
            "WHERE table_schema='main' AND table_name=?",
            (_REVISION_TABLE,),
        )
    except Exception:
        return 0
    if not table_rows or int(table_rows[0][0] or 0) <= 0:
        return 0
    try:
        rows = con.query(
            f"SELECT revision FROM {_REVISION_TABLE} WHERE revision_key=?",
            (_STATS_FLOW_SOURCE_KEY,),
        )
    except Exception:
        return 0
    if not rows:
        return 0
    try:
        return max(1, int(rows[0][0] or 1))
    except Exception:
        return 0


def bump_stats_flow_source_revision(con: DuckDBEngine, *, reason: Any = "") -> int:
    ensure_analysis_revision_table(con)
    current = get_stats_flow_source_revision(con)
    next_revision = max(1, current + 1)
    con.execute(
        f"""
        INSERT INTO {_REVISION_TABLE}(revision_key, revision, updated_at, reason)
        VALUES (?,?,NOW(),?)
        ON CONFLICT(revision_key) DO UPDATE
        SET revision=excluded.revision,
            updated_at=excluded.updated_at,
            reason=excluded.reason
        """,
        (_STATS_FLOW_SOURCE_KEY, next_revision, project_analysis_revision_reason(reason)),
    )
    return next_revision
