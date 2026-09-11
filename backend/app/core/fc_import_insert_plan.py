from __future__ import annotations

from dataclasses import dataclass
from typing import Iterable, List, Sequence, Set, Tuple

from app.core.fc_import_projection import EXTRA_JSON_TABLES, FcProjectionSchema, raw_col, sql_literal
from app.core.import_count_semantics import known_public_import_count


@dataclass(frozen=True)
class ImportInsertPlan:
    insert_cols: List[str]
    raw_insert_cols: List[str]
    missing_headers: Set[str]
    chunk_size: int


@dataclass(frozen=True)
class DedupTableSql:
    dedup_table: str
    staging_table: str
    raw_table: str
    case_id: str
    insert_cols: Sequence[str]
    chunk_start: int
    chunk_end: int


def build_import_insert_plan(
    *,
    schema: FcProjectionSchema,
    mapping_missing: Sequence[str],
    chunk_rows: int,
) -> ImportInsertPlan:
    normalized_chunk_rows = _require_positive_count(chunk_rows, field="insert_chunk_rows")
    missing_headers = set(mapping_missing)
    insert_cols = ["case_id", "file_id", "row_no", "imported_at", "row_hash"]
    insert_cols.extend([raw_col(schema.col_map[header]) for header in schema.headers])
    if schema.table in EXTRA_JSON_TABLES:
        insert_cols.append("extra_json")
    if schema.store_raw_json:
        insert_cols.append("raw_json")

    raw_insert_cols = ["case_id", "file_id", "row_no", "imported_at", "row_hash"]
    raw_insert_cols.extend(
        raw_col(schema.col_map[header])
        for header in schema.headers
        if header not in missing_headers
    )
    if schema.table in EXTRA_JSON_TABLES:
        raw_insert_cols.append("extra_json")
    if schema.store_raw_json:
        raw_insert_cols.append("raw_json")

    return ImportInsertPlan(
        insert_cols=insert_cols,
        raw_insert_cols=raw_insert_cols,
        missing_headers=missing_headers,
        chunk_size=normalized_chunk_rows,
    )


def iter_insert_chunks(total_rows: int, chunk_size: int) -> Iterable[Tuple[int, int]]:
    normalized_total = _require_count(total_rows, field="insert_total_rows")
    normalized_chunk = _require_positive_count(chunk_size, field="insert_chunk_size")
    for chunk_start in range(1, normalized_total + 1, normalized_chunk):
        yield chunk_start, min(chunk_start + normalized_chunk - 1, normalized_total)


def _require_count(value: object, *, field: str) -> int:
    normalized = known_public_import_count(value)
    if normalized is None:
        raise ValueError(f"{field}_count_invalid")
    return normalized


def _require_positive_count(value: object, *, field: str) -> int:
    normalized = _require_count(value, field=field)
    if normalized == 0:
        raise ValueError(f"{field}_count_invalid")
    return normalized


def build_dedup_table_sql(config: DedupTableSql) -> str:
    if not config.insert_cols:
        raise ValueError("dedup table requires insert columns")
    qualified_insert_cols = ", ".join(f"s.{column}" for column in config.insert_cols)
    return f"""CREATE TEMP TABLE {config.dedup_table} AS
                    WITH ordered_rows AS (
                        SELECT s.*, row_number() OVER (ORDER BY s.row_no) AS import_seq
                        FROM {config.staging_table} s
                    ),
                    first_rows AS (
                        SELECT s.row_hash, MIN(s.row_no) AS row_no
                        FROM ordered_rows s
                        WHERE s.import_seq BETWEEN {int(config.chunk_start)} AND {int(config.chunk_end)}
                        GROUP BY s.row_hash
                    )
                    SELECT {qualified_insert_cols}
                    FROM ordered_rows s
                    JOIN first_rows f
                      ON f.row_hash=s.row_hash AND f.row_no=s.row_no
                    WHERE NOT EXISTS (
                        SELECT 1 FROM {config.raw_table} t
                        WHERE t.case_id={sql_literal(config.case_id)} AND t.row_hash=s.row_hash
                      )
                """


def build_raw_insert_sql(*, raw_table: str, dedup_table: str, raw_insert_cols: Sequence[str]) -> str:
    if not raw_insert_cols:
        raise ValueError("raw insert requires columns")
    columns_sql = ", ".join(raw_insert_cols)
    return f"""INSERT INTO {raw_table}({columns_sql})
                    SELECT {columns_sql}
                    FROM {dedup_table}
                """


def build_privacy_delta_insert_sql(*, dedup_table: str) -> str:
    return f"""INSERT INTO privacy_projection_delta_log(case_id, op, file_id, row_hash, source, created_at)
                        SELECT ?, 'insert', ?, row_hash, 'import', ?
                        FROM {dedup_table}"""
