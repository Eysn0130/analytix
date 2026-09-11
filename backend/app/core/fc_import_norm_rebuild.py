from __future__ import annotations

from typing import Optional

from app.core import fc_import_schema
from app.core.db_engine import DuckDBEngine
from app.core.fc_import_norm_insert import build_norm_insert_sql
from app.core.import_count_semantics import known_public_import_count


def rebuild_norm_for_file(
    engine: DuckDBEngine,
    *,
    case_id: str,
    file_id: str,
    kind: Optional[str] = None,
) -> int:
    """Rebuild norm rows for a single file_id from raw."""
    normalized_case_id = _require_scope_text(case_id, field="case_id")
    normalized_file_id = _require_scope_text(file_id, field="file_id")
    recorded_kind = _kind_for_file(engine, case_id=normalized_case_id, file_id=normalized_file_id)
    if recorded_kind is None:
        raise ValueError("norm_rebuild_file_source_unavailable")
    if kind is not None:
        requested_kind = _require_scope_text(kind, field="kind")
        if requested_kind != recorded_kind:
            raise ValueError("norm_rebuild_file_kind_mismatch")
    resolved_kind = recorded_kind
    if not resolved_kind or resolved_kind not in fc_import_schema.FC_SCHEMAS:
        raise ValueError("norm_rebuild_file_kind_unknown")

    schema = fc_import_schema.FC_SCHEMAS[resolved_kind]
    raw_table = fc_import_schema.raw_table_name(schema.table)
    norm_table = fc_import_schema.norm_table_name(schema.table)
    # Deliberately do not commit here: callers may compose this destructive
    # rebuild inside their own transaction and must be able to roll it back.
    engine.execute(
        f"DELETE FROM {norm_table} WHERE case_id=? AND file_id=?",
        (normalized_case_id, normalized_file_id),
    )
    sql, params = build_norm_insert_sql(
        schema,
        raw_table,
        norm_table,
        case_id=normalized_case_id,
        file_id=normalized_file_id,
    )
    engine.execute(sql, params)
    return _count_norm_rows(
        engine,
        norm_table,
        case_id=normalized_case_id,
        file_id=normalized_file_id,
    )


def rebuild_norm_for_case(
    engine: DuckDBEngine,
    *,
    case_id: str,
    kind: str,
) -> int:
    """Rebuild norm rows for a whole case from raw for a specific funds table kind."""
    normalized_case_id = _require_scope_text(case_id, field="case_id")
    normalized_kind = _require_scope_text(kind, field="kind")
    if normalized_kind not in fc_import_schema.FC_SCHEMAS:
        raise ValueError("norm_rebuild_case_kind_unknown")

    schema = fc_import_schema.FC_SCHEMAS[normalized_kind]
    raw_table = fc_import_schema.raw_table_name(schema.table)
    norm_table = fc_import_schema.norm_table_name(schema.table)
    # This helper participates in the caller's transaction and never commits.
    engine.execute(f"DELETE FROM {norm_table} WHERE case_id=?", (normalized_case_id,))
    sql, params = build_norm_insert_sql(
        schema,
        raw_table,
        norm_table,
        case_id=normalized_case_id,
    )
    engine.execute(sql, params)
    return _count_norm_rows(engine, norm_table, case_id=normalized_case_id)


def _kind_for_file(
    engine: DuckDBEngine,
    *,
    case_id: str,
    file_id: str,
) -> Optional[str]:
    try:
        rows = engine.query(
            "SELECT kind FROM import_file_log WHERE case_id=? AND file_id=? LIMIT 2",
            (case_id, file_id),
        )
    except Exception as exc:
        raise RuntimeError("norm_rebuild_file_catalog_unavailable") from exc
    if type(rows) is not list:
        raise RuntimeError("norm_rebuild_file_catalog_unavailable")
    if not rows:
        return None
    if len(rows) != 1:
        raise RuntimeError("norm_rebuild_file_catalog_ambiguous")
    row = rows[0]
    if not isinstance(row, (list, tuple)) or len(row) != 1:
        raise RuntimeError("norm_rebuild_file_catalog_invalid")
    value = row[0]
    if type(value) is not str or not value or value != value.strip():
        raise RuntimeError("norm_rebuild_file_catalog_invalid")
    return value


def _count_norm_rows(
    engine: DuckDBEngine,
    norm_table: str,
    *,
    case_id: str,
    file_id: Optional[str] = None,
) -> int:
    if file_id is not None:
        rows = engine.query(
            f"SELECT COUNT(1) FROM {norm_table} WHERE case_id=? AND file_id=?",
            (case_id, file_id),
        )
    else:
        rows = engine.query(f"SELECT COUNT(1) FROM {norm_table} WHERE case_id=?", (case_id,))
    if type(rows) is not list or len(rows) != 1:
        raise RuntimeError("norm_rebuild_count_unavailable")
    row = rows[0]
    if not isinstance(row, (list, tuple)) or len(row) != 1:
        raise RuntimeError("norm_rebuild_count_unavailable")
    count = known_public_import_count(row[0])
    if count is None:
        raise RuntimeError("norm_rebuild_count_unavailable")
    return count


def _require_scope_text(value: object, *, field: str) -> str:
    if type(value) is not str or not value or value != value.strip():
        raise ValueError(f"norm_rebuild_{field}_invalid")
    return value
