from __future__ import annotations

import time
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, MutableMapping

from app.core.db_engine import DuckDBEngine
from app.core.fc_import_csv_reader import duckdb_csv_read_expr, extract_invalid_duckdb_csv_param
from app.core.fc_import_csv_source import ImportCsvSource
from app.core.fc_import_projection import sql_literal
from app.core.fc_import_projection_state import ImportProjectionState
from app.core.fc_import_staging import ImportStagingSql, build_import_staging_sql
from app.core.fc_import_timing import add_phase
from app.core.import_accelerator import prepare_csv
from app.core.paths import ensure_dir


ProjectionStateFactory = Callable[[ImportCsvSource], ImportProjectionState]


@dataclass(frozen=True)
class ImportStagingRequest:
    case_id: str
    file_id: str
    imported_at: str
    staging_table: str
    precleaned_csv: bool


@dataclass(frozen=True)
class ImportStagingResult:
    source: ImportCsvSource
    projection_state: ImportProjectionState
    elapsed_s: float


def create_import_staging(
    *,
    engine: DuckDBEngine,
    request: ImportStagingRequest,
    source: ImportCsvSource,
    projection_state: ImportProjectionState,
    projection_state_factory: ProjectionStateFactory,
    timings: MutableMapping[str, float],
) -> ImportStagingResult:
    started = time.perf_counter()
    active_source = source
    active_projection_state = projection_state
    last_exc: Exception | None = None

    def create_staging(*, minimal: bool = False) -> bool:
        nonlocal last_exc
        for use_all_varchar in (True, False):
            drop_opts: set[str] = set()
            for _ in range(2):
                read_expr = duckdb_csv_read_expr(
                    csv_sql=active_source.csv_sql,
                    encoding_sql=active_source.encoding_sql,
                    use_positional=active_projection_state.header.use_positional,
                    column_map_sql=active_projection_state.header.column_map_sql,
                    force_not_null_sql=active_projection_state.projection.force_not_null_sql,
                    precleaned_csv=request.precleaned_csv,
                    all_varchar=use_all_varchar,
                    minimal=minimal,
                    drop_opts=drop_opts if drop_opts else None,
                )
                try:
                    engine.execute(
                        build_import_staging_sql(
                            ImportStagingSql(
                                staging_table=request.staging_table,
                                case_id_sql=sql_literal(request.case_id),
                                file_id_sql=sql_literal(request.file_id),
                                imported_at_sql=sql_literal(request.imported_at),
                                select_cols=active_projection_state.projection.select_cols,
                                extra_json_expr=active_projection_state.projection.extra_json_expr,
                                raw_json_expr=active_projection_state.projection.raw_json_expr,
                                hash_expr=active_projection_state.projection.hash_expr,
                                read_expr=read_expr,
                                non_empty_filter_expr=active_projection_state.projection.non_empty_filter_expr,
                            )
                        )
                    )
                    return True
                except Exception as exc:
                    last_exc = exc
                    bad_opt = extract_invalid_duckdb_csv_param(str(exc))
                    if bad_opt and bad_opt not in drop_opts:
                        drop_opts.add(bad_opt)
                        continue
                break
        return False

    def prepare_clean_csv(source_csv: Path, clean_path: Path) -> None:
        phase_started = time.perf_counter()
        prepare_csv(source_csv, limit=0, out_path=clean_path)
        add_phase(timings, "preclean_s", time.perf_counter() - phase_started)

    def switch_to_clean_csv(clean_path: Path) -> None:
        nonlocal active_source, active_projection_state
        active_source = ImportCsvSource.from_prepared_clean_path(clean_path)
        active_projection_state = projection_state_factory(active_source)

    def preclean_and_create(*, minimal: bool = False) -> bool:
        clean_path = _clean_csv_path(engine, request.file_id)
        prepare_clean_csv(active_source.path, clean_path)
        switch_to_clean_csv(clean_path)
        return create_staging(minimal=minimal)

    needs_preclean = not active_projection_state.header.raw_headers
    should_preclean_encoding = active_source.requires_encoding_preclean
    created = False

    if should_preclean_encoding or needs_preclean:
        created = preclean_and_create()
        if not created and last_exc is not None and "Invalid named parameter" in str(last_exc):
            created = create_staging(minimal=True)
    else:
        created = create_staging()
        if not created and last_exc is not None:
            message = str(last_exc)
            needs_minimal = "Invalid named parameter" in message
            if "Binder Error" in message or "Referenced column" in message or needs_minimal:
                created = preclean_and_create(minimal=needs_minimal)

    if not created:
        raise RuntimeError(f"failed to create staging table: {last_exc}")

    elapsed = time.perf_counter() - started
    add_phase(timings, "staging_s", elapsed)
    return ImportStagingResult(
        source=active_source,
        projection_state=active_projection_state,
        elapsed_s=elapsed,
    )


def _clean_csv_path(engine: DuckDBEngine, file_id: str) -> Path:
    cache_dir = ensure_dir(engine.path.parent / ".import_cache")
    return cache_dir / f"{file_id}.clean.csv"
