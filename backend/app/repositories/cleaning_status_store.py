from __future__ import annotations

from typing import Any, List, Optional, Sequence

from app.core.failure_boundary import project_cleaning_error
from app.core.import_count_semantics import (
    CLEANING_COUNTS_VERSION,
    is_cleaning_success_status,
    known_public_import_count,
)
from app.repositories import cleaning_duckdb_meta


def update_clean_status(
    executor: Any,
    cur: Any,
    file_ids: Sequence[str],
    status: str,
    *,
    error: Optional[str] = None,
    started_at: Optional[str] = None,
    finished_at: Optional[str] = None,
    rows_affected: Optional[int] = None,
) -> None:
    if not file_ids:
        return
    if not cleaning_duckdb_meta.table_exists(cur, "import_file_log"):
        return
    columns = cleaning_duckdb_meta.column_names(cur, "import_file_log")
    if "cleaned_status" not in columns:
        return

    fields = ["cleaned_status=?"]
    params: List[Any] = [status]
    if started_at is not None and "cleaned_started_at" in columns:
        fields.append("cleaned_started_at=?")
        params.append(started_at)
    if finished_at is not None and "cleaned_finished_at" in columns:
        fields.append("cleaned_finished_at=?")
        params.append(finished_at)
    if error is not None and "cleaned_error" in columns:
        fields.append("cleaned_error=?")
        params.append(project_cleaning_error(error, status=status))
    cleaning_succeeded = is_cleaning_success_status(status)
    normalized_rows_affected = None
    if rows_affected is not None:
        normalized_rows_affected = known_public_import_count(rows_affected)
        if normalized_rows_affected is None:
            raise ValueError("cleaned_rows_affected_invalid")
    if "cleaned_rows_affected" in columns:
        if normalized_rows_affected is not None and cleaning_succeeded:
            fields.append("cleaned_rows_affected=?")
            params.append(normalized_rows_affected)
        else:
            fields.append("cleaned_rows_affected=NULL")
    if "cleaning_counts_version" in columns:
        if normalized_rows_affected is not None and cleaning_succeeded:
            fields.append("cleaning_counts_version=?")
            params.append(CLEANING_COUNTS_VERSION)
        else:
            fields.append("cleaning_counts_version=NULL")

    placeholders = ",".join(["?"] * len(file_ids))
    params.extend(file_ids)
    cur.execute(
        f"UPDATE import_file_log SET {', '.join(fields)} WHERE file_id IN ({placeholders})",
        tuple(params),
    )


__all__ = ["update_clean_status"]
