from __future__ import annotations

from typing import Optional

from app.core.db_engine import DuckDBEngine
from app.core.failure_boundary import project_import_error
from app.core.import_count_semantics import (
    IMPORT_COUNTS_VERSION,
    MAX_PUBLIC_IMPORT_COUNT,
    is_import_success_status,
)
from app.core.storage import _now_iso


def _require_nonnegative_int(value: object, *, field: str) -> int:
    if (
        isinstance(value, bool)
        or not isinstance(value, int)
        or value < 0
        or value > MAX_PUBLIC_IMPORT_COUNT
    ):
        raise ValueError(f"import_log_{field}_invalid")
    return value


def _optional_nonnegative_int(value: object, *, field: str) -> Optional[int]:
    if value is None:
        return None
    return _require_nonnegative_int(value, field=field)


def upsert_import_file_log(
    engine: DuckDBEngine,
    *,
    file_id: str,
    case_id: str,
    kind: str,
    filename: str,
    display_path: str,
    stored_path: str,
    file_type: str,
    size: Optional[int],
    md5: str,
    sha256: str,
    rows_total: Optional[int],
    status: str,
    error: str = "",
    rows_skipped_non_data: Optional[int] = None,
) -> None:
    normalized_size = _optional_nonnegative_int(size, field="size")
    normalized_rows_total = _optional_nonnegative_int(rows_total, field="rows_total")
    normalized_rows_skipped = _optional_nonnegative_int(
        rows_skipped_non_data,
        field="rows_skipped_non_data",
    )
    now = _now_iso()
    public_error = project_import_error(error, status=status)
    exists = bool(engine.query("SELECT 1 FROM import_file_log WHERE file_id=? LIMIT 1", (file_id,)))
    if exists:
        reset_progress = str(status or "").strip().lower() in {"导入中", "running"}
        progress_reset_sql = (
            ", rows_imported=NULL, rows_imported_raw=NULL, rows_imported_norm=NULL, "
            "rows_dedup=NULL, rows_error=NULL, cleaned_status='pending', "
            "cleaned_started_at=NULL, cleaned_finished_at=NULL, cleaned_error=NULL, "
            "cleaned_rows_affected=NULL, cleaning_counts_version=NULL"
            if reset_progress
            else ""
        )
        engine.execute(
            f"""UPDATE import_file_log
                   SET kind=?, filename=?, display_path=?, stored_path=?, file_type=?,
                   size=?, md5=?, sha256=?, rows_total=?, status=?, error=?,
                   rows_skipped_non_data=?, import_counts_version=NULL{progress_reset_sql}
               WHERE file_id=?""",
            (
                kind,
                filename,
                display_path,
                stored_path,
                file_type,
                normalized_size,
                md5,
                sha256,
                normalized_rows_total,
                status,
                public_error,
                normalized_rows_skipped,
                file_id,
            ),
        )
        return

    engine.execute(
        """INSERT INTO import_file_log(
               file_id, case_id, kind, filename, display_path, stored_path, file_type,
               size, md5, sha256, rows_total, rows_imported, rows_imported_raw, rows_imported_norm,
               rows_dedup, rows_error, rows_skipped_non_data, import_counts_version, status, error,
               cleaned_status, cleaned_started_at, cleaned_finished_at, cleaned_error,
               cleaned_rows_affected, cleaning_counts_version, created_at, finished_at
           ) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)""",
        (
            file_id,
            case_id,
            kind,
            filename,
            display_path,
            stored_path,
            file_type,
            normalized_size,
            md5,
            sha256,
            normalized_rows_total,
            None,
            None,
            None,
            None,
            None,
            normalized_rows_skipped,
            None,
            status,
            public_error,
            "pending",
            None,
            None,
            None,
            None,
            None,
            now,
            None,
        ),
    )


def update_import_progress(
    engine: DuckDBEngine,
    file_id: str,
    rows_imported: Optional[int],
    status: Optional[str] = None,
    error: str = "",
    rows_total: Optional[int] = None,
    rows_dedup: Optional[int] = None,
    rows_error: Optional[int] = None,
    rows_imported_raw: Optional[int] = None,
    rows_imported_norm: Optional[int] = None,
    rows_skipped_non_data: Optional[int] = None,
) -> None:
    now = _now_iso()
    fields = []
    params = []
    count_values = {
        "rows_total": rows_total,
        "rows_imported": rows_imported,
        "rows_imported_raw": rows_imported_raw,
        "rows_imported_norm": rows_imported_norm,
        "rows_dedup": rows_dedup,
        "rows_error": rows_error,
        "rows_skipped_non_data": rows_skipped_non_data,
    }
    if rows_imported is not None:
        fields.append("rows_imported=?")
        params.append(_require_nonnegative_int(rows_imported, field="rows_imported"))
    if rows_total is not None:
        fields.append("rows_total=?")
        params.append(_require_nonnegative_int(rows_total, field="rows_total"))
    if rows_dedup is not None:
        fields.append("rows_dedup=?")
        params.append(_require_nonnegative_int(rows_dedup, field="rows_dedup"))
    if rows_error is not None:
        fields.append("rows_error=?")
        params.append(_require_nonnegative_int(rows_error, field="rows_error"))
    if rows_imported_raw is not None:
        fields.append("rows_imported_raw=?")
        params.append(_require_nonnegative_int(rows_imported_raw, field="rows_imported_raw"))
    if rows_imported_norm is not None:
        fields.append("rows_imported_norm=?")
        params.append(_require_nonnegative_int(rows_imported_norm, field="rows_imported_norm"))
    if rows_skipped_non_data is not None:
        fields.append("rows_skipped_non_data=?")
        params.append(_require_nonnegative_int(rows_skipped_non_data, field="rows_skipped_non_data"))
    if status:
        normalized_status = status.strip().lower()
        finished_at = (
            now
            if normalized_status
            in {
                "已完成",
                "失败",
                "已取消",
                "completed",
                "done",
                "succeeded",
                "success",
                "failed",
                "cancelled",
                "canceled",
            }
            else None
        )
        fields.append("status=?")
        params.append(status)
        fields.append("error=?")
        params.append(project_import_error(error, status=status))
        fields.append("finished_at=COALESCE(?, finished_at)")
        params.append(finished_at)
        if is_import_success_status(status):
            for field, value in count_values.items():
                if value is None:
                    fields.append(f"{field}=NULL")
            fields.append("import_counts_version=?")
            params.append(IMPORT_COUNTS_VERSION)
        else:
            fields.append("import_counts_version=NULL")

    if not fields:
        return
    sql = f"UPDATE import_file_log SET {', '.join(fields)} WHERE file_id=?"
    params.append(file_id)
    engine.execute(sql, tuple(params))
