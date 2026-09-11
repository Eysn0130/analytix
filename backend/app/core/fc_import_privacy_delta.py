from __future__ import annotations

import uuid
from collections.abc import Sequence

from app.core.db_engine import DuckDBEngine
from app.core.fc_import_duckdb import (
    duckdb_table_columns,
    duckdb_table_exists,
    ensure_columns,
    require_duckdb_identifier,
)
from app.core.import_count_semantics import known_public_import_count
from app.core.storage import _now_iso


_DELTA_LOG_COLUMNS = {
    "case_id",
    "op",
    "file_id",
    "row_hash",
    "source",
    "created_at",
    "batch_id",
}


def ensure_privacy_projection_delta_log(engine: DuckDBEngine) -> None:
    engine.execute(
        """CREATE TABLE IF NOT EXISTS privacy_projection_delta_log(
               case_id TEXT,
               op TEXT,
               file_id TEXT,
               row_hash TEXT,
               source TEXT,
               created_at TEXT,
               batch_id TEXT
           )"""
    )
    ensure_columns(engine, "privacy_projection_delta_log", [("batch_id", "TEXT")])
    columns = duckdb_table_columns(engine, "privacy_projection_delta_log")
    if not _DELTA_LOG_COLUMNS.issubset(columns):
        raise RuntimeError("privacy_delta_log_schema_invalid")


def append_privacy_projection_delta_for_case_rows(
    engine: DuckDBEngine,
    *,
    case_id: str,
    op: str,
    table: str,
    where_clause: str,
    params: Sequence[object],
    source: str,
) -> int:
    normalized_op = _normalize_delta_op(op)
    normalized_case_id = _require_nonempty_text(case_id, field="case_id")
    normalized_source = _require_nonempty_text(source, field="source")
    normalized_where = _require_where_clause(where_clause)
    normalized_params = _require_params(params)
    normalized_table = _require_source_table(
        engine,
        table,
        required_columns={"case_id", "file_id", "row_hash"},
    )
    ensure_privacy_projection_delta_log(engine)
    source_predicate = f"({normalized_where}) AND case_id=?"
    source_params = (*normalized_params, normalized_case_id)
    matched_count, invalid_count = _source_identity_metrics(
        engine,
        table=normalized_table,
        predicate=source_predicate,
        params=source_params,
        require_file_id=True,
    )
    if invalid_count:
        raise RuntimeError("privacy_delta_source_identity_invalid")
    expected_count = _distinct_source_identity_count(
        engine,
        table=normalized_table,
        predicate=source_predicate,
        params=source_params,
        include_file_id=True,
    )
    if expected_count > matched_count:
        raise RuntimeError("privacy_delta_source_count_invalid")
    if expected_count == 0:
        return 0

    batch_id = uuid.uuid4().hex
    engine.execute(
        f"""INSERT INTO privacy_projection_delta_log(
                   case_id, op, file_id, row_hash, source, created_at, batch_id
               )
            SELECT DISTINCT ?, ?, file_id, row_hash, ?, ?, ?
              FROM {normalized_table}
             WHERE {source_predicate}
               AND COALESCE(TRIM(CAST(file_id AS VARCHAR)), '')<>''
               AND COALESCE(TRIM(CAST(row_hash AS VARCHAR)), '')<>''""",
        (
            normalized_case_id,
            normalized_op,
            normalized_source,
            _now_iso(),
            batch_id,
            *source_params,
        ),
    )
    actual_count = _delta_batch_count(
        engine,
        batch_id=batch_id,
        case_id=normalized_case_id,
    )
    if actual_count != expected_count:
        raise RuntimeError("privacy_delta_insert_count_mismatch")
    return actual_count


def append_privacy_projection_delta_from_hash_table(
    engine: DuckDBEngine,
    *,
    case_id: str,
    op: str,
    hash_table: str,
    file_id: str,
    source: str,
) -> int:
    normalized_op = _normalize_delta_op(op)
    normalized_case_id = _require_nonempty_text(case_id, field="case_id")
    normalized_file_id = _require_nonempty_text(file_id, field="file_id")
    normalized_source = _require_nonempty_text(source, field="source")
    normalized_hash_table = _require_source_table(
        engine,
        hash_table,
        required_columns={"row_hash"},
    )
    ensure_privacy_projection_delta_log(engine)
    matched_count, invalid_count = _source_identity_metrics(
        engine,
        table=normalized_hash_table,
        predicate="TRUE",
        params=(),
        require_file_id=False,
    )
    if invalid_count:
        raise RuntimeError("privacy_delta_source_identity_invalid")
    expected_count = _distinct_source_identity_count(
        engine,
        table=normalized_hash_table,
        predicate="TRUE",
        params=(),
        include_file_id=False,
    )
    if expected_count > matched_count:
        raise RuntimeError("privacy_delta_source_count_invalid")
    if expected_count == 0:
        return 0

    batch_id = uuid.uuid4().hex
    engine.execute(
        f"""INSERT INTO privacy_projection_delta_log(
                   case_id, op, file_id, row_hash, source, created_at, batch_id
               )
            SELECT DISTINCT ?, ?, ?, row_hash, ?, ?, ?
             FROM {normalized_hash_table}
             WHERE COALESCE(TRIM(CAST(row_hash AS VARCHAR)), '')<>''""",
        (
            normalized_case_id,
            normalized_op,
            normalized_file_id,
            normalized_source,
            _now_iso(),
            batch_id,
        ),
    )
    actual_count = _delta_batch_count(
        engine,
        batch_id=batch_id,
        case_id=normalized_case_id,
    )
    if actual_count != expected_count:
        raise RuntimeError("privacy_delta_insert_count_mismatch")
    return actual_count


def _normalize_delta_op(op: str) -> str:
    if type(op) is not str:
        raise ValueError("privacy_delta_op_invalid")
    normalized_op = op.strip().lower()
    if normalized_op == "update":
        normalized_op = "upsert"
    if normalized_op not in {"insert", "delete", "upsert"}:
        raise ValueError("privacy_delta_op_invalid")
    return normalized_op


def _require_nonempty_text(value: object, *, field: str) -> str:
    if type(value) is not str or not value or value != value.strip():
        raise ValueError(f"privacy_delta_{field}_invalid")
    return value


def _require_where_clause(value: object) -> str:
    if type(value) is not str or not value.strip():
        raise ValueError("privacy_delta_where_clause_invalid")
    if any(marker in value for marker in (";", "--", "/*", "*/", "\x00")):
        raise ValueError("privacy_delta_where_clause_invalid")
    return value.strip()


def _require_params(params: object) -> tuple[object, ...]:
    if isinstance(params, (str, bytes, bytearray)) or not isinstance(params, Sequence):
        raise ValueError("privacy_delta_params_invalid")
    return tuple(params)


def _require_source_table(
    engine: DuckDBEngine,
    table: object,
    *,
    required_columns: set[str],
) -> str:
    normalized_table = require_duckdb_identifier(table, field="table")
    if not duckdb_table_exists(engine, normalized_table):
        raise RuntimeError("privacy_delta_source_table_unavailable")
    columns = duckdb_table_columns(engine, normalized_table)
    if not required_columns.issubset(columns):
        raise RuntimeError("privacy_delta_source_schema_invalid")
    return normalized_table


def _source_identity_metrics(
    engine: DuckDBEngine,
    *,
    table: str,
    predicate: str,
    params: Sequence[object],
    require_file_id: bool,
) -> tuple[int, int]:
    invalid_predicates = ["COALESCE(TRIM(CAST(row_hash AS VARCHAR)), '')=''"]
    if require_file_id:
        invalid_predicates.append("COALESCE(TRIM(CAST(file_id AS VARCHAR)), '')=''")
    rows = engine.query(
        f"""SELECT COUNT(1),
                   COUNT(1) FILTER (WHERE {' OR '.join(invalid_predicates)})
              FROM {table}
             WHERE {predicate}""",
        params,
    )
    if type(rows) is not list or len(rows) != 1:
        raise RuntimeError("privacy_delta_source_count_unavailable")
    row = rows[0]
    if not isinstance(row, (list, tuple)) or len(row) != 2:
        raise RuntimeError("privacy_delta_source_count_unavailable")
    matched_count = _known_count(row[0], code="privacy_delta_source_count_unavailable")
    invalid_count = _known_count(row[1], code="privacy_delta_source_count_unavailable")
    if invalid_count > matched_count:
        raise RuntimeError("privacy_delta_source_count_invalid")
    return matched_count, invalid_count


def _distinct_source_identity_count(
    engine: DuckDBEngine,
    *,
    table: str,
    predicate: str,
    params: Sequence[object],
    include_file_id: bool,
) -> int:
    identity_columns = "file_id, row_hash" if include_file_id else "row_hash"
    valid_predicates = ["COALESCE(TRIM(CAST(row_hash AS VARCHAR)), '')<>''"]
    if include_file_id:
        valid_predicates.append("COALESCE(TRIM(CAST(file_id AS VARCHAR)), '')<>''")
    rows = engine.query(
        f"""SELECT COUNT(1)
              FROM (
                    SELECT DISTINCT {identity_columns}
                      FROM {table}
                     WHERE {predicate}
                       AND {' AND '.join(valid_predicates)}
                   ) AS analytix_privacy_delta_source""",
        params,
    )
    return _single_count_row(rows, code="privacy_delta_source_count_unavailable")


def _delta_batch_count(
    engine: DuckDBEngine,
    *,
    batch_id: str,
    case_id: str,
) -> int:
    rows = engine.query(
        "SELECT COUNT(1) FROM privacy_projection_delta_log WHERE batch_id=? AND case_id=?",
        (batch_id, case_id),
    )
    return _single_count_row(rows, code="privacy_delta_insert_count_unavailable")


def _single_count_row(rows: object, *, code: str) -> int:
    if type(rows) is not list or len(rows) != 1:
        raise RuntimeError(code)
    row = rows[0]
    if not isinstance(row, (list, tuple)) or len(row) != 1:
        raise RuntimeError(code)
    return _known_count(row[0], code=code)


def _known_count(value: object, *, code: str) -> int:
    count = known_public_import_count(value)
    if count is None:
        raise RuntimeError(code)
    return count
