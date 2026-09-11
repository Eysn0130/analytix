from __future__ import annotations

from typing import Any

from app.repositories import cleaning_duckdb_meta


def get_last_import_at(cur: Any, case_id: str) -> str:
    if not cleaning_duckdb_meta.table_exists(cur, "import_file_log"):
        return ""
    cur.execute(
        "SELECT COALESCE(finished_at, created_at) "
        "FROM import_file_log "
        "WHERE case_id=? AND kind LIKE 'fc_%' "
        "ORDER BY COALESCE(finished_at, created_at) DESC LIMIT 1",
        (case_id,),
    )
    row = cur.fetchone()
    return row[0] if row and row[0] else ""


__all__ = ["get_last_import_at"]
