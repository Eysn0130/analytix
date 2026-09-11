from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Any, Sequence

from app.core.failure_boundary import (
    CLEANING_CANCELLED,
    CLEANING_FAILED,
    project_cleaning_error,
)
from app.repositories import cleaning_status_store


@dataclass(frozen=True)
class CleaningFailureMessage:
    base_message: str
    detail: str


def build_cleaning_failure(*, current_step: str, exc: BaseException) -> CleaningFailureMessage:
    # Do not retain or stringify the exception. The caller still owns the
    # caught value for local control flow, while public and persisted status
    # fields receive only the fixed failure code.
    del current_step
    del exc
    return CleaningFailureMessage(
        base_message=CLEANING_FAILED,
        detail=CLEANING_FAILED,
    )


def mark_cleaning_cancelled(
    executor: Any,
    *,
    con: Any,
    scope_ids: Sequence[str],
    rows_affected: int,
) -> None:
    _mark_failed_status(
        executor,
        con=con,
        scope_ids=scope_ids,
        error=CLEANING_CANCELLED,
        rows_affected=rows_affected,
    )


def mark_cleaning_failed(
    executor: Any,
    *,
    con: Any,
    scope_ids: Sequence[str],
    error: str,
    rows_affected: int,
) -> None:
    _mark_failed_status(
        executor,
        con=con,
        scope_ids=scope_ids,
        error=error,
        rows_affected=rows_affected,
    )


def _mark_failed_status(
    executor: Any,
    *,
    con: Any,
    scope_ids: Sequence[str],
    error: str,
    rows_affected: int,
) -> None:
    if con is None:
        return
    public_error = project_cleaning_error(
        error,
        status="cancelled" if type(error) is str and error == CLEANING_CANCELLED else "failed",
    )
    finished_at = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    try:
        cleaning_status_store.update_clean_status(
            executor,
            con.cursor(),
            list(scope_ids),
            "failed",
            finished_at=finished_at,
            error=public_error,
            rows_affected=rows_affected,
        )
        con.commit()
    except Exception:
        pass


__all__ = [
    "CleaningFailureMessage",
    "build_cleaning_failure",
    "mark_cleaning_cancelled",
    "mark_cleaning_failed",
]
