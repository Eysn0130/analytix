from __future__ import annotations

from typing import Any, Dict, Mapping, Optional


MAX_PUBLIC_IMPORT_COUNT = 9_007_199_254_740_991
IMPORT_COUNTS_VERSION = 1
CLEANING_COUNTS_VERSION = 1

_IMPORT_ROW_COUNT_FIELDS = (
    "rows_total",
    "rows_imported",
    "rows_imported_raw",
    "rows_imported_norm",
    "rows_dedup",
    "rows_error",
    "rows_skipped_non_data",
)
_IMPORT_SUCCESS_STATUSES = frozenset({"completed", "done", "succeeded", "success", "已完成"})
_CLEANING_SUCCESS_STATUSES = frozenset({"completed", "done", "succeeded", "success", "已完成"})


def known_public_import_count(value: object) -> Optional[int]:
    if type(value) is not int or value < 0 or value > MAX_PUBLIC_IMPORT_COUNT:
        return None
    return value


def first_known_public_import_count(*values: object) -> Optional[int]:
    for value in values:
        normalized = known_public_import_count(value)
        if normalized is not None:
            return normalized
    return None


def _completed(status: object, finished_at: object, *, allowed: frozenset[str]) -> bool:
    if type(status) is not str or status.strip().lower() not in allowed:
        return False
    return type(finished_at) is str and bool(finished_at.strip())


def is_import_success_status(status: object) -> bool:
    return type(status) is str and status.strip().lower() in _IMPORT_SUCCESS_STATUSES


def is_cleaning_success_status(status: object) -> bool:
    return type(status) is str and status.strip().lower() in _CLEANING_SUCCESS_STATUSES


def project_import_file_log_counts(row: Mapping[str, Any]) -> Dict[str, Any]:
    """Project import counters without upgrading placeholders to facts."""

    projected = dict(row)
    projected["size"] = known_public_import_count(row.get("size"))

    import_complete = _completed(
        row.get("status"),
        row.get("finished_at"),
        allowed=_IMPORT_SUCCESS_STATUSES,
    )
    for field in _IMPORT_ROW_COUNT_FIELDS:
        projected[field] = known_public_import_count(row.get(field)) if import_complete else None

    cleaning_complete = _completed(
        row.get("cleaned_status"),
        row.get("cleaned_finished_at"),
        allowed=_CLEANING_SUCCESS_STATUSES,
    )
    projected["cleaned_rows_affected"] = (
        known_public_import_count(row.get("cleaned_rows_affected"))
        if cleaning_complete
        else None
    )
    return projected


def project_persisted_import_file_log_counts(row: Mapping[str, Any]) -> Dict[str, Any]:
    """Require current writers' verification markers before publishing stored counts."""

    projected = dict(row)
    if type(row.get("import_counts_version")) is not int or row.get(
        "import_counts_version"
    ) != IMPORT_COUNTS_VERSION:
        for field in _IMPORT_ROW_COUNT_FIELDS:
            projected[field] = None
    if type(row.get("cleaning_counts_version")) is not int or row.get(
        "cleaning_counts_version"
    ) != CLEANING_COUNTS_VERSION:
        projected["cleaned_rows_affected"] = None
    projected.pop("import_counts_version", None)
    projected.pop("cleaning_counts_version", None)
    return project_import_file_log_counts(projected)


__all__ = [
    "MAX_PUBLIC_IMPORT_COUNT",
    "CLEANING_COUNTS_VERSION",
    "IMPORT_COUNTS_VERSION",
    "first_known_public_import_count",
    "is_cleaning_success_status",
    "is_import_success_status",
    "known_public_import_count",
    "project_import_file_log_counts",
    "project_persisted_import_file_log_counts",
]
