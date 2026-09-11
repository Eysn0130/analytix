from __future__ import annotations

from typing import Any, Dict, Mapping, Sequence

from app.repositories.cleaning_sql_counter import require_non_negative_count


def _optional_count(value: object, *, code: str) -> int | None:
    if value is None:
        return None
    return require_non_negative_count(value, code=code)


def _required_count(value: object, *, code: str) -> int:
    return require_non_negative_count(value, code=code)


def resolve_cleaning_engine(*, native_segments: Sequence[str], python_fallback_steps: Sequence[int]) -> str:
    if "clean-all" in native_segments:
        return "rust_clean_all"
    if native_segments and python_fallback_steps:
        return "mixed"
    if native_segments:
        return "rust_stepwise"
    return "python_sql"


def build_cleaning_summary(
    *,
    scope_rows: int,
    scope_ids: Sequence[str],
    active_steps: Sequence[int],
    native_segments: Sequence[str],
    python_fallback_steps: Sequence[int],
    legacy_python_cleaning_steps: Sequence[int],
    native_clean_all_attempted: bool,
    native_clean_all_failed: bool,
    invalid_total: int | None,
    duplicate_total: int | None,
    failed_total: int | None,
    reversal_total: int | None,
    normalized_count: int | None,
    inferred_count: int | None,
    filled_count: int | None,
    account_invalid_total: int | None,
    suffix_acc_total: int | None,
    account_fill_total: int | None,
    rows_affected_total: int,
    duration_ms: int,
    step1: Dict[str, Any],
    native_timings_ms: Mapping[str, Any] | None = None,
    phase_timings_ms: Mapping[str, Any] | None = None,
    legacy_python_cleaning_reason: str | None = None,
) -> Dict[str, Any]:
    legacy_reason = str(legacy_python_cleaning_reason or "").strip()
    if not legacy_python_cleaning_steps:
        legacy_reason = ""
    elif not legacy_reason:
        legacy_reason = "native_unavailable"

    projected_step1 = {
        str(key): _required_count(value, code="cleaning_step1_count_unavailable")
        for key, value in step1.items()
        if str(key).strip()
    }

    summary = {
        "total_rows": _required_count(scope_rows, code="cleaning_scope_count_unavailable"),
        "scope_rows": _required_count(scope_rows, code="cleaning_scope_count_unavailable"),
        "file_ids": list(scope_ids),
        "steps_executed": list(active_steps),
        "cleaning_engine": resolve_cleaning_engine(
            native_segments=native_segments,
            python_fallback_steps=python_fallback_steps,
        ),
        "native_segments": list(native_segments),
        "python_fallback_steps": list(python_fallback_steps),
        "legacy_python_cleaning": bool(legacy_python_cleaning_steps),
        "legacy_python_cleaning_steps": list(legacy_python_cleaning_steps),
        "legacy_python_cleaning_reason": legacy_reason,
        "native_clean_all_attempted": bool(native_clean_all_attempted),
        "native_clean_all_failed": bool(native_clean_all_failed),
        "rows_affected_total": _required_count(
            rows_affected_total,
            code="cleaning_rows_affected_count_unavailable",
        ),
        "duration_ms": _required_count(duration_ms, code="cleaning_duration_unavailable"),
        **projected_step1,
    }
    for key, value in (
        ("invalid", invalid_total),
        ("duplicate", duplicate_total),
        ("failed", failed_total),
        ("reversal", reversal_total),
        ("dc_normalized", normalized_count),
        ("dc_inferred", inferred_count),
        ("card_filled", filled_count),
        ("account_invalid", account_invalid_total),
        ("account_suffix", suffix_acc_total),
        ("account_info_filled", account_fill_total),
    ):
        count = _optional_count(value, code=f"{key}_count_unavailable")
        if count is not None:
            summary[key] = count
    timings = {
        str(key): _required_count(value, code="cleaning_timing_unavailable")
        for key, value in dict(native_timings_ms or {}).items()
        if str(key).strip()
    }
    if timings:
        summary["native_timings_ms"] = timings
    phase_timings = {
        str(key): _required_count(value, code="cleaning_timing_unavailable")
        for key, value in dict(phase_timings_ms or {}).items()
        if str(key).strip()
    }
    if phase_timings:
        summary["phase_timings_ms"] = phase_timings
    return summary


__all__ = [
    "build_cleaning_summary",
    "resolve_cleaning_engine",
]
