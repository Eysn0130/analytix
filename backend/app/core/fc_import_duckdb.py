from __future__ import annotations

import re

from app.core.db_engine import DuckDBEngine
from app.core.import_count_semantics import known_public_import_count


_DUCKDB_IDENTIFIER_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")


def require_duckdb_identifier(value: object, *, field: str = "identifier") -> str:
    if type(value) is not str or _DUCKDB_IDENTIFIER_RE.fullmatch(value) is None:
        raise ValueError(f"duckdb_{field}_invalid")
    return value


def _require_column_type(value: object) -> str:
    if type(value) is not str or not value or value != value.strip():
        raise ValueError("duckdb_column_type_invalid")
    if any(marker in value for marker in (";", "--", "/*", "*/", "\x00")):
        raise ValueError("duckdb_column_type_invalid")
    return value


def _single_known_count(rows: object, *, code: str) -> int:
    if type(rows) is not list or len(rows) != 1:
        raise RuntimeError(code)
    row = rows[0]
    if not isinstance(row, (list, tuple)) or len(row) != 1:
        raise RuntimeError(code)
    count = known_public_import_count(row[0])
    if count is None:
        raise RuntimeError(code)
    return count


def supports_json_object(engine: DuckDBEngine) -> bool:
    cached = getattr(engine, "_analytix_supports_json_object", None)
    if cached is True:
        return True
    try:
        rows = engine.query("SELECT json_object('a', '1')")
    except Exception as exc:
        raise RuntimeError("duckdb_json_object_probe_failed") from exc
    if (
        type(rows) is not list
        or len(rows) != 1
        or not isinstance(rows[0], (list, tuple))
        or len(rows[0]) != 1
        or rows[0][0] is None
    ):
        raise RuntimeError("duckdb_json_object_probe_invalid")
    setattr(engine, "_analytix_supports_json_object", True)
    return True


def duckdb_table_columns(engine: DuckDBEngine, table: str) -> set[str]:
    normalized_table = require_duckdb_identifier(table, field="table")
    expected_count = _single_known_count(
        engine.query(
            "SELECT COUNT(1) FROM information_schema.columns "
            "WHERE table_schema='main' AND table_name=?",
            (normalized_table,),
        ),
        code="duckdb_column_catalog_count_unavailable",
    )
    rows = engine.query(
        "SELECT column_name FROM information_schema.columns "
        "WHERE table_schema='main' AND table_name=? ORDER BY ordinal_position",
        (normalized_table,),
    )
    if type(rows) is not list or len(rows) != expected_count:
        raise RuntimeError("duckdb_column_catalog_unavailable")
    columns: list[str] = []
    for row in rows:
        if (
            not isinstance(row, (list, tuple))
            or len(row) != 1
            or type(row[0]) is not str
            or _DUCKDB_IDENTIFIER_RE.fullmatch(row[0]) is None
        ):
            raise RuntimeError("duckdb_column_catalog_invalid")
        columns.append(row[0])
    if len(set(columns)) != len(columns):
        raise RuntimeError("duckdb_column_catalog_invalid")
    return set(columns)


def duckdb_table_exists(engine: DuckDBEngine, table: str) -> bool:
    normalized_table = require_duckdb_identifier(table, field="table")
    count = _single_known_count(
        engine.query(
            "SELECT COUNT(1) FROM information_schema.tables "
            "WHERE table_schema='main' AND table_name=?",
            (normalized_table,),
        ),
        code="duckdb_table_catalog_count_unavailable",
    )
    if count not in {0, 1}:
        raise RuntimeError("duckdb_table_catalog_count_invalid")
    return count == 1


def table_has_rows(engine: DuckDBEngine, table: str) -> bool:
    normalized_table = require_duckdb_identifier(table, field="table")
    try:
        rows = engine.query(
            f"SELECT COUNT(1) FROM (SELECT 1 FROM {normalized_table} LIMIT 1) AS analytix_probe"
        )
    except Exception as exc:
        raise RuntimeError("duckdb_table_row_probe_failed") from exc
    count = _single_known_count(
        rows,
        code="duckdb_table_row_probe_unavailable",
    )
    if count not in {0, 1}:
        raise RuntimeError("duckdb_table_row_probe_invalid")
    return count == 1


def ensure_columns(engine: DuckDBEngine, table: str, columns: list[tuple[str, str]]) -> None:
    normalized_table = require_duckdb_identifier(table, field="table")
    normalized_columns: list[tuple[str, str]] = []
    seen: set[str] = set()
    for item in columns:
        if not isinstance(item, (list, tuple)) or len(item) != 2:
            raise ValueError("duckdb_column_definition_invalid")
        name = require_duckdb_identifier(item[0], field="column")
        col_type = _require_column_type(item[1])
        if name in seen:
            raise ValueError("duckdb_column_definition_duplicate")
        seen.add(name)
        normalized_columns.append((name, col_type))

    existing = duckdb_table_columns(engine, normalized_table)
    for name, col_type in normalized_columns:
        if name in existing:
            continue
        try:
            engine.execute(f"ALTER TABLE {normalized_table} ADD COLUMN {name} {col_type}")
        except Exception as exc:
            # A concurrent, compatible migration may have won the race. It is
            # safe only after a fresh authoritative catalog read proves that
            # the requested column now exists.
            if name not in duckdb_table_columns(engine, normalized_table):
                raise RuntimeError("duckdb_column_add_failed") from exc
        existing.add(name)

    verified = duckdb_table_columns(engine, normalized_table)
    if not seen.issubset(verified):
        raise RuntimeError("duckdb_column_add_unverified")
