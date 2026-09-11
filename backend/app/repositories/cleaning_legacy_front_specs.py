from __future__ import annotations

from typing import Any

from app.repositories import cleaning_front_steps
from app.repositories.cleaning_legacy_step_spec import (
    LegacyCleaningStepSpec,
    format_changed_total_message,
    format_elapsed_message,
    map_total_result,
    rows_affected_delta,
)
from app.repositories.cleaning_step_context import CleaningStepContext


def _run_amount_balance(context: CleaningStepContext, cur: Any, total_rows: int) -> dict[str, int]:
    return cleaning_front_steps.step_amount_balance(context, cur, total_rows)


def _map_amount_balance(result: dict[str, int]) -> dict[str, Any]:
    return {"step1": result, "rows_affected_delta": rows_affected_delta(result.get("rows_affected"))}


def _progress_amount_balance(_result: dict[str, int], elapsed: float) -> str:
    return format_elapsed_message("step1 amount/balance normalization", elapsed)


def _run_invalid(context: CleaningStepContext, cur: Any, _total_rows: int) -> tuple[int, int]:
    return cleaning_front_steps.step_invalid(context, cur)


def _map_invalid(result: tuple[int, int]) -> dict[str, Any]:
    return map_total_result(result, total_key="invalid_total")


def _progress_invalid(result: tuple[int, int], elapsed: float) -> str:
    invalid_changed, invalid_total = result
    return format_changed_total_message("step2 invalid mark", invalid_changed, invalid_total, elapsed)


def _run_duplicate(context: CleaningStepContext, cur: Any, _total_rows: int) -> tuple[int, int]:
    return cleaning_front_steps.step_duplicate(context, cur)


def _map_duplicate(result: tuple[int, int]) -> dict[str, Any]:
    return map_total_result(result, total_key="duplicate_total")


def _progress_duplicate(result: tuple[int, int], elapsed: float) -> str:
    duplicate_changed, duplicate_total = result
    return format_changed_total_message("step3 duplicate mark(clear)", duplicate_changed, duplicate_total, elapsed)


def _run_failed_reversal(
    context: CleaningStepContext,
    cur: Any,
    _total_rows: int,
) -> tuple[int, int, int, int]:
    return cleaning_front_steps.step_failed_reversal(context, cur)


def _map_failed_reversal(result: tuple[int, int, int, int]) -> dict[str, Any]:
    failed_changed, reversal_changed, failed_total, reversal_total = result
    return {
        "failed_total": failed_total,
        "reversal_total": reversal_total,
        "rows_affected_delta": rows_affected_delta(failed_changed, reversal_changed),
    }


def _progress_failed_reversal(result: tuple[int, int, int, int], elapsed: float) -> str:
    failed_changed, reversal_changed, failed_total, reversal_total = result
    return format_elapsed_message(
        f"step4 failed/reversal changed_failed={failed_changed} changed_reversal={reversal_changed} "
        f"total_failed={failed_total} total_reversal={reversal_total}",
        elapsed,
    )


def _run_dc_flag(context: CleaningStepContext, cur: Any, _total_rows: int) -> tuple[int, int, int, int]:
    return cleaning_front_steps.step_dc_flag(context, cur)


def _map_dc_flag(result: tuple[int, int, int, int]) -> dict[str, Any]:
    normalized_count, inferred_count, norm_total, infer_total = result
    return {
        "normalized_count": normalized_count,
        "inferred_count": inferred_count,
        "norm_total": norm_total,
        "infer_total": infer_total,
        "rows_affected_delta": rows_affected_delta(normalized_count, inferred_count),
    }


def _progress_dc_flag(result: tuple[int, int, int, int], elapsed: float) -> str:
    normalized_count, inferred_count, norm_total, infer_total = result
    return format_elapsed_message(
        f"step5 dc normalize={normalized_count} infer={inferred_count} "
        f"total_normalized={norm_total} total_inferred={infer_total}",
        elapsed,
    )


FRONT_LEGACY_STEP_SPECS = {
    1: LegacyCleaningStepSpec(
        step_no=1,
        run=_run_amount_balance,
        map_result=_map_amount_balance,
        format_progress=_progress_amount_balance,
    ),
    2: LegacyCleaningStepSpec(
        step_no=2,
        run=_run_invalid,
        map_result=_map_invalid,
        format_progress=_progress_invalid,
    ),
    3: LegacyCleaningStepSpec(
        step_no=3,
        run=_run_duplicate,
        map_result=_map_duplicate,
        format_progress=_progress_duplicate,
    ),
    4: LegacyCleaningStepSpec(
        step_no=4,
        run=_run_failed_reversal,
        map_result=_map_failed_reversal,
        format_progress=_progress_failed_reversal,
    ),
    5: LegacyCleaningStepSpec(
        step_no=5,
        run=_run_dc_flag,
        map_result=_map_dc_flag,
        format_progress=_progress_dc_flag,
    ),
}


__all__ = ["FRONT_LEGACY_STEP_SPECS"]
