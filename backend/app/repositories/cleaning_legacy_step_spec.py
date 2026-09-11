from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Callable, Dict

from app.repositories.cleaning_step_context import CleaningStepContext
from app.repositories.cleaning_sql_counter import require_non_negative_count


LegacyStepRunner = Callable[[CleaningStepContext, Any, int], Any]
LegacyResultMapper = Callable[[Any], Dict[str, Any]]
LegacyProgressFormatter = Callable[[Any, float], str]


@dataclass(frozen=True)
class LegacyCleaningStepSpec:
    step_no: int
    run: LegacyStepRunner
    map_result: LegacyResultMapper
    format_progress: LegacyProgressFormatter


def rows_affected_delta(*values: Any) -> int:
    return sum(
        require_non_negative_count(value, code="legacy_cleaning_count_unavailable")
        for value in values
    )


def map_count_total_result(result: tuple[Any, Any], *, count_key: str, total_key: str) -> Dict[str, Any]:
    count, total = result
    return {
        count_key: count,
        total_key: total,
        "rows_affected_delta": rows_affected_delta(count),
    }


def map_total_result(result: tuple[Any, Any], *, total_key: str) -> Dict[str, Any]:
    count, total = result
    return {
        total_key: total,
        "rows_affected_delta": rows_affected_delta(count),
    }


def format_elapsed_message(message: str, elapsed: float) -> str:
    return f"{message} ({elapsed:.2f}s)"


def format_changed_total_message(prefix: str, changed: Any, total: Any, elapsed: float) -> str:
    return format_elapsed_message(f"{prefix} changed={changed} total={total}", elapsed)


__all__ = [
    "format_changed_total_message",
    "format_elapsed_message",
    "LegacyCleaningStepSpec",
    "LegacyProgressFormatter",
    "LegacyResultMapper",
    "LegacyStepRunner",
    "map_count_total_result",
    "map_total_result",
    "rows_affected_delta",
]
