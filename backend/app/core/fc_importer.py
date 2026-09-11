from __future__ import annotations

import time
from pathlib import Path
from typing import Dict, Optional, Tuple

from app.core import fc_import_schema
from app.core.db_engine import DuckDBEngine
from app.core.fc_import_file_identity import detect_file_encoding, read_csv_header_map
from app.core.fc_import_note import SingleFileImportNoteContext, build_single_file_import_note
from app.core.fc_import_projection_context import create_import_projection_context
from app.core.fc_import_row_writer import ImportRowWriteRequest, write_import_rows
from app.core.fc_import_single_file_result import (
    SingleFileImportSummary,
    emit_single_file_import_log_and_progress,
    emit_single_file_import_profile,
)
from app.core.fc_import_staging_metrics import ImportStagingMetricsRequest, collect_import_staging_metrics
from app.core.fc_import_staging_runner import ImportStagingRequest, create_import_staging
from app.core.fc_import_temp_tables import build_import_temp_tables, drop_import_temp_tables
from app.core.fc_import_timing import add_phase
from app.core.storage import _now_iso


IMPORT_INSERT_CHUNK_ROWS = 1_000_000


def import_fc_csv_into_db(
    *,
    engine: DuckDBEngine,
    case_id: str,
    file_id: str,
    kind: str,
    path: Path,
    display_name: str,
    field_mapping: Optional[Dict[str, str]] = None,
    progress_cb=None,
    batch_size: int = 5000,
    log_detail: bool = True,
    csv_encoding: Optional[str] = None,
    precleaned_csv: bool = False,
    profile_cb=None,
) -> Tuple[int, int, int, str]:
    """Batch import a single CSV into DuckDB.

    Returns (rows_total_seen, rows_raw_inserted, rows_norm_inserted, import_note).
    """
    schema = fc_import_schema.FC_SCHEMAS[kind]
    raw_table = fc_import_schema.raw_table_name(schema.table)
    norm_table = fc_import_schema.norm_table_name(schema.table)
    alias_map = fc_import_schema.HEADER_ALIASES_BY_TABLE.get(schema.table, {})
    projection_started = time.perf_counter()
    projection_context = create_import_projection_context(
        engine=engine,
        schema=schema,
        path=path,
        csv_encoding=csv_encoding,
        field_mapping=field_mapping,
        alias_map=alias_map,
        precleaned_csv=precleaned_csv,
        display_name=display_name,
        detect_encoding=detect_file_encoding,
        read_header_map=read_csv_header_map,
    )
    source = projection_context.source
    projection_state = projection_context.projection_state

    imported_at = _now_iso()
    temp_tables = build_import_temp_tables(schema_table=schema.table, file_id=file_id)
    staging_table = temp_tables.staging_table
    dedup_table = temp_tables.dedup_table

    timings: Dict[str, float] = {}
    profile_started = time.perf_counter()
    add_phase(timings, "projection_s", time.perf_counter() - projection_started)

    staging_result = create_import_staging(
        engine=engine,
        request=ImportStagingRequest(
            case_id=case_id,
            file_id=file_id,
            imported_at=imported_at,
            staging_table=staging_table,
            precleaned_csv=precleaned_csv,
        ),
        source=source,
        projection_state=projection_state,
        projection_state_factory=projection_context.projection_state_factory,
        timings=timings,
    )
    projection_state = staging_result.projection_state
    staging_elapsed_s = staging_result.elapsed_s

    inserted_rows = 0
    dup_in_file = 0
    dup_existing = 0
    staging_metrics = collect_import_staging_metrics(
        engine=engine,
        request=ImportStagingMetricsRequest(
            schema=schema,
            staging_table=staging_table,
        ),
        timings=timings,
    )
    total_rows = staging_metrics.total_rows
    key_missing = staging_metrics.key_missing
    dedup_rows = 0

    projection = projection_state.projection
    row_write_result = write_import_rows(
        engine=engine,
        request=ImportRowWriteRequest(
            schema=schema,
            raw_table=raw_table,
            norm_table=norm_table,
            staging_table=staging_table,
            dedup_table=dedup_table,
            case_id=case_id,
            file_id=file_id,
            total_rows=total_rows,
            mapping_missing=projection.mapping_missing,
            chunk_rows=IMPORT_INSERT_CHUNK_ROWS,
            precleaned_csv=precleaned_csv,
        ),
        timings=timings,
    )
    inserted_rows = row_write_result.inserted_rows
    norm_inserted = row_write_result.norm_inserted
    chunk_count = row_write_result.chunk_count
    dedup_rows = row_write_result.dedup_rows
    dup_in_file = row_write_result.dup_in_file
    dup_existing = row_write_result.dup_existing

    drop_import_temp_tables(
        engine=engine,
        tables=(staging_table, dedup_table),
        timings=timings,
    )
    import_summary = SingleFileImportSummary(
        display_name=display_name,
        total_rows=int(total_rows),
        inserted_rows=int(inserted_rows),
        norm_inserted=int(norm_inserted),
        dedup_rows=int(dedup_rows),
        chunk_count=int(chunk_count),
        staging_elapsed_s=staging_elapsed_s,
        row_write_elapsed_s=row_write_result.elapsed_s,
    )
    emit_single_file_import_log_and_progress(
        summary=import_summary,
        log_detail=log_detail,
        progress_cb=progress_cb,
    )

    mapping_note = build_single_file_import_note(
        engine,
        SingleFileImportNoteContext(
            schema=schema,
            norm_table=norm_table,
            case_id=case_id,
            file_id=file_id,
            total_rows=int(total_rows),
            inserted_rows=int(inserted_rows),
            norm_inserted=int(norm_inserted),
            dedup_rows=int(dedup_rows),
            dup_in_file=int(dup_in_file),
            dup_existing=int(dup_existing),
            key_missing=key_missing,
            projection=projection,
            alias_map=alias_map,
        ),
    )
    emit_single_file_import_profile(
        summary=import_summary,
        timings=timings,
        profile_started=profile_started,
        profile_cb=profile_cb,
    )

    return int(total_rows), int(inserted_rows), int(norm_inserted), mapping_note

