from __future__ import annotations

import logging
import time
from dataclasses import dataclass
from typing import Any, Callable, MutableMapping

from app.core.fc_import_timing import ImportTimingProfile
from app.core.safe_observability import log_closed_diagnostic


@dataclass(frozen=True)
class SingleFileImportSummary:
    display_name: str
    total_rows: int
    inserted_rows: int
    norm_inserted: int
    dedup_rows: int
    chunk_count: int
    staging_elapsed_s: float
    row_write_elapsed_s: float


ProgressCallback = Callable[[int, int], None]
ProfileCallback = Callable[[dict[str, Any]], None]


def format_single_file_import_log(summary: SingleFileImportSummary) -> str:
    return (
        f"topic=application code=completed read_csv={summary.staging_elapsed_s:.2f}s "
        f"staging=0.00s hash=0.00s "
        f"dedup_insert={summary.row_write_elapsed_s:.2f}s rows={summary.total_rows} "
        f"inserted={summary.inserted_rows} dedup={summary.dedup_rows}"
    )


def build_single_file_import_profile(
    *,
    summary: SingleFileImportSummary,
    timings: MutableMapping[str, float],
    wall_s: float,
) -> dict[str, Any]:
    return ImportTimingProfile(
        phases=dict(timings),
        rows_seen=int(summary.total_rows),
        rows_imported=int(summary.inserted_rows),
        chunks=int(summary.chunk_count),
        files=1,
        wall_s=wall_s,
    ).as_dict()


def emit_single_file_import_log_and_progress(
    *,
    summary: SingleFileImportSummary,
    log_detail: bool,
    progress_cb: ProgressCallback | None,
) -> None:
    if log_detail:
        log_closed_diagnostic(
            logging.getLogger("analytix.data_analysis.import"),
            logging.INFO,
            topic="application",
            code="completed",
        )

    if progress_cb:
        progress_cb(int(summary.inserted_rows), int(summary.total_rows))


def emit_single_file_import_profile(
    *,
    summary: SingleFileImportSummary,
    timings: MutableMapping[str, float],
    profile_started: float,
    profile_cb: ProfileCallback | None,
) -> None:
    if profile_cb:
        profile_cb(
            build_single_file_import_profile(
                summary=summary,
                timings=timings,
                wall_s=time.perf_counter() - profile_started,
            )
        )
