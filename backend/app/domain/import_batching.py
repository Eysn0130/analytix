from __future__ import annotations

from typing import Callable, Optional, Sequence, TypeVar

T = TypeVar("T")

ImportGroupKeyResolver = Callable[[T], Optional[tuple]]

DEFAULT_IMPORT_TRANSACTION_BATCH_FILES = 200
DEFAULT_IMPORT_TRANSACTION_BATCH_ROWS = 1_000_000


def _require_nonnegative_scheduling_count(value: object, *, field: str) -> int:
    if type(value) is not int or value < 0:
        raise ValueError(f"import_batching_{field}_invalid")
    return value


def plan_next_import_unit(
    prepared: Sequence[T],
    start_index: int,
    *,
    group_key: ImportGroupKeyResolver[T],
    file_limit: int,
    row_limit: int,
) -> list[T]:
    first = prepared[start_index]
    key = group_key(first)
    if key is None:
        return [first]

    normalized_file_limit = _require_nonnegative_scheduling_count(
        file_limit,
        field="file_limit",
    )
    normalized_row_limit = _require_nonnegative_scheduling_count(
        row_limit,
        field="row_limit",
    )
    unit = [first]
    rows_in_unit = _rows_total(first)
    cursor = start_index + 1
    while cursor < len(prepared):
        if normalized_file_limit > 0 and len(unit) >= normalized_file_limit:
            break
        candidate = prepared[cursor]
        if group_key(candidate) != key:
            break
        candidate_rows = _rows_total(candidate)
        if (
            normalized_row_limit > 0
            and rows_in_unit > 0
            and candidate_rows > 0
            and rows_in_unit + candidate_rows > normalized_row_limit
        ):
            break
        unit.append(candidate)
        rows_in_unit += candidate_rows
        cursor += 1

    return unit if len(unit) > 1 else [first]


def import_result_row_weight(result) -> int:
    return max(
        _require_nonnegative_scheduling_count(
            getattr(result, "rows_seen", None),
            field="rows_seen",
        ),
        _require_nonnegative_scheduling_count(
            getattr(result, "rows_imported_raw", None),
            field="rows_imported_raw",
        ),
        _require_nonnegative_scheduling_count(
            getattr(result, "rows_imported_norm", None),
            field="rows_imported_norm",
        ),
    )


def should_commit_import_transaction(
    *,
    files_since_begin: int,
    rows_since_begin: int,
    file_limit: int = DEFAULT_IMPORT_TRANSACTION_BATCH_FILES,
    row_limit: int = DEFAULT_IMPORT_TRANSACTION_BATCH_ROWS,
) -> bool:
    normalized_file_limit = _require_nonnegative_scheduling_count(
        file_limit,
        field="file_limit",
    )
    normalized_row_limit = _require_nonnegative_scheduling_count(
        row_limit,
        field="row_limit",
    )
    normalized_files_since_begin = _require_nonnegative_scheduling_count(
        files_since_begin,
        field="files_since_begin",
    )
    normalized_rows_since_begin = _require_nonnegative_scheduling_count(
        rows_since_begin,
        field="rows_since_begin",
    )
    return (
        normalized_file_limit > 0 and normalized_files_since_begin >= normalized_file_limit
    ) or (
        normalized_row_limit > 0 and normalized_rows_since_begin >= normalized_row_limit
    )


def _rows_total(item) -> int:
    return _require_nonnegative_scheduling_count(
        getattr(item, "rows_total", None),
        field="rows_total",
    )
