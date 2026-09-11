from __future__ import annotations

import hashlib
import json
from typing import Any, Mapping, Optional, Sequence
from uuid import uuid4

from app.domain.ordinary_diagnostic_projection import project_cleaning_summary_diagnostic
from app.repositories.cleaning_sql_counter import require_non_negative_count
from app.utils.time import utc_now


def record_cleaning(
    executor: Any,
    cur: Any,
    import_at: str,
    summary: Mapping[str, Any],
    *,
    file_ids: Optional[Sequence[str]] = None,
    duration_ms: int,
    scope_rows: int,
) -> None:
    cleaned_at = utc_now().isoformat()
    run_ref = "cleanrun_v2_" + hashlib.sha256(
        f"{executor.case_id}\x00{uuid4()}".encode("utf-8")
    ).hexdigest()
    payload = json.dumps(
        project_cleaning_summary_diagnostic(summary),
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    scope_file_ids = [str(file_id or "") for file_id in (file_ids or [""])]
    values_sql = ",".join(["(?)"] * len(scope_file_ids))
    cur.execute(
        "INSERT INTO cleaning_log(case_id, file_id, import_at, cleaned_at, duration_ms, scope_rows, summary, run_ref) "
        "SELECT ?, v.file_id, ?, ?, ?, ?, ?, ? "
        f"FROM (VALUES {values_sql}) AS v(file_id)",
        (
            executor.case_id,
            import_at or "",
            cleaned_at,
            require_non_negative_count(
                duration_ms,
                code="cleaning_duration_unavailable",
            ),
            require_non_negative_count(
                scope_rows,
                code="cleaning_scope_count_unavailable",
            ),
            payload,
            run_ref,
            *scope_file_ids,
        ),
    )


__all__ = ["record_cleaning"]
