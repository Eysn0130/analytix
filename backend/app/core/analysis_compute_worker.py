from __future__ import annotations

from pathlib import Path
from typing import NoReturn, Optional, Sequence

from app.core.analysis_compute_runner import AnalysisComputeUnavailableError


_ANALYSIS_COMPUTE_CAPABILITY_UNAVAILABLE = "managed_process_capability_admission_unavailable"


def _raise_analysis_compute_capability_unavailable() -> NoReturn:
    """Reject before reading case identifiers, paths, filters, or cursors."""

    raise AnalysisComputeUnavailableError(_ANALYSIS_COMPUTE_CAPABILITY_UNAVAILABLE)


def query_stats_date_range_worker(
    *,
    case_id: str,
    db_path: Path,
    include_diagnostics: bool = False,
) -> Optional[dict]:
    _raise_analysis_compute_capability_unavailable()


def query_stats_rows_worker(
    *,
    case_id: str,
    db_path: Path,
    mode: str,
    selected_keys: Sequence[str],
    date_start: str,
    date_end: str,
    search_text: str,
    row_sort_col: str,
    row_sort_dir: str,
    row_offset: int,
    row_limit: int,
    row_format: str = "object",
    fields: Sequence[str] = (),
    include_diagnostics: bool = False,
) -> Optional[dict]:
    _raise_analysis_compute_capability_unavailable()


def query_stats_tree_worker(
    *,
    case_id: str,
    db_path: Path,
    tab: str,
    include_diagnostics: bool = False,
) -> Optional[dict]:
    _raise_analysis_compute_capability_unavailable()


def query_stats_txn_rows_worker(
    *,
    case_id: str,
    db_path: Path,
    key_type: str,
    key_value: str,
    selected_keys: Sequence[str],
    date_start: str,
    date_end: str,
    direction: str,
    sort_col: str,
    sort_dir: str,
    limit: int,
    cursor: Optional[dict],
    key_values: Sequence[str] = (),
    start_time: str = "",
    end_time: str = "",
    row_format: str = "object",
    fields: Sequence[str] = (),
    include_diagnostics: bool = False,
) -> Optional[dict]:
    _raise_analysis_compute_capability_unavailable()


def shutdown_stats_query_worker() -> None:
    """Compatibility no-op: no direct worker process can be created."""
    return None
