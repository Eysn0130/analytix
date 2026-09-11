from __future__ import annotations

import time
from dataclasses import dataclass
from typing import MutableMapping, Sequence

from app.core.db_engine import DuckDBEngine
from app.core.fc_import_insert_plan import (
    DedupTableSql,
    build_dedup_table_sql,
    build_import_insert_plan,
    build_privacy_delta_insert_sql,
    build_raw_insert_sql,
    iter_insert_chunks,
)
from app.core.fc_import_norm_insert import build_norm_insert_sql
from app.core.fc_import_privacy_delta import ensure_privacy_projection_delta_log
from app.core.fc_import_projection import FcProjectionSchema
from app.core.fc_import_temp_tables import drop_table_if_exists
from app.core.fc_import_timing import add_phase
from app.core.import_count_semantics import known_public_import_count
from app.core.storage import _now_iso


@dataclass(frozen=True)
class ImportRowWriteRequest:
    schema: FcProjectionSchema
    raw_table: str
    norm_table: str
    staging_table: str
    dedup_table: str
    case_id: str
    file_id: str
    total_rows: int
    mapping_missing: Sequence[str]
    chunk_rows: int
    precleaned_csv: bool


@dataclass(frozen=True)
class ImportRowWriteResult:
    inserted_rows: int
    norm_inserted: int
    chunk_count: int
    dedup_rows: int
    dup_in_file: int
    dup_existing: int
    elapsed_s: float


def write_import_rows(
    *,
    engine: DuckDBEngine,
    request: ImportRowWriteRequest,
    timings: MutableMapping[str, float],
) -> ImportRowWriteResult:
    started_total = time.perf_counter()
    total_rows = _require_count(request.total_rows, field="row_write_total_rows")
    insert_plan = build_import_insert_plan(
        schema=request.schema,
        mapping_missing=request.mapping_missing,
        chunk_rows=request.chunk_rows,
    )
    inserted_rows = 0
    norm_inserted = 0
    chunk_count = 0
    privacy_delta_ready = False
    try:
        for chunk_start, chunk_end in iter_insert_chunks(total_rows, insert_plan.chunk_size):
            chunk_count += 1
            drop_table_if_exists(engine, request.dedup_table)

            started = time.perf_counter()
            engine.execute(
                build_dedup_table_sql(
                    DedupTableSql(
                        dedup_table=request.dedup_table,
                        staging_table=request.staging_table,
                        raw_table=request.raw_table,
                        case_id=request.case_id,
                        insert_cols=insert_plan.insert_cols,
                        chunk_start=chunk_start,
                        chunk_end=chunk_end,
                    )
                )
            )
            add_phase(timings, "dedup_build_s", time.perf_counter() - started)

            started = time.perf_counter()
            rows = engine.query(f"SELECT COUNT(1) FROM {request.dedup_table}")
            chunk_inserted = _require_single_count(rows, field="dedup_chunk_rows")
            chunk_width = chunk_end - chunk_start + 1
            if chunk_inserted > chunk_width:
                raise ValueError("dedup_chunk_rows_exceed_source_chunk")
            add_phase(timings, "dedup_count_s", time.perf_counter() - started)
            if chunk_inserted == 0:
                continue

            inserted_rows += chunk_inserted
            if inserted_rows > total_rows:
                raise ValueError("inserted_rows_exceed_staging_total")
            started = time.perf_counter()
            engine.execute(
                build_raw_insert_sql(
                    raw_table=request.raw_table,
                    dedup_table=request.dedup_table,
                    raw_insert_cols=insert_plan.raw_insert_cols,
                )
            )
            add_phase(timings, "raw_insert_s", time.perf_counter() - started)

            norm_sql, norm_params = build_norm_insert_sql(
                request.schema,
                request.dedup_table,
                request.norm_table,
                case_id=request.case_id,
                file_id=request.file_id,
                precleaned_source=request.precleaned_csv,
                skip_existing_check=True,
                input_values_cleaned=True,
                missing_headers=insert_plan.missing_headers,
            )
            started = time.perf_counter()
            engine.execute(norm_sql, norm_params)
            add_phase(timings, "norm_insert_s", time.perf_counter() - started)
            norm_inserted += chunk_inserted

            if request.schema.table == "fc_transaction":
                started = time.perf_counter()
                if not privacy_delta_ready:
                    ensure_privacy_projection_delta_log(engine)
                    privacy_delta_ready = True
                engine.execute(
                    build_privacy_delta_insert_sql(dedup_table=request.dedup_table),
                    (request.case_id, request.file_id, _now_iso()),
                )
                add_phase(timings, "privacy_delta_s", time.perf_counter() - started)

        dedup_rows = total_rows - inserted_rows
        dup_in_file = 0
        dup_existing = 0
        if dedup_rows > 0 and total_rows > 0:
            started = time.perf_counter()
            rows = engine.query(f"SELECT COUNT(DISTINCT row_hash) FROM {request.staging_table}")
            distinct_hashes = _require_single_count(rows, field="staging_distinct_hashes")
            if distinct_hashes > total_rows:
                raise ValueError("staging_distinct_hashes_exceed_total")
            if inserted_rows > distinct_hashes:
                raise ValueError("inserted_rows_exceed_staging_distinct_hashes")
            dup_in_file = total_rows - distinct_hashes
            dup_existing = distinct_hashes - inserted_rows
            add_phase(timings, "dedup_distinct_s", time.perf_counter() - started)
    except Exception as exc:
        raise RuntimeError(f"failed to import deduplicated rows: {exc}") from exc

    return ImportRowWriteResult(
        inserted_rows=int(inserted_rows),
        norm_inserted=int(norm_inserted),
        chunk_count=int(chunk_count),
        dedup_rows=int(dedup_rows),
        dup_in_file=int(dup_in_file),
        dup_existing=int(dup_existing),
        elapsed_s=time.perf_counter() - started_total,
    )


def _require_count(value: object, *, field: str) -> int:
    normalized = known_public_import_count(value)
    if normalized is None:
        raise ValueError(f"{field}_count_invalid")
    return normalized


def _require_single_count(rows: object, *, field: str) -> int:
    if not isinstance(rows, list) or len(rows) != 1:
        raise ValueError(f"{field}_row_unavailable")
    row = rows[0]
    if not isinstance(row, (list, tuple)) or len(row) != 1:
        raise ValueError(f"{field}_row_invalid")
    return _require_count(row[0], field=field)
