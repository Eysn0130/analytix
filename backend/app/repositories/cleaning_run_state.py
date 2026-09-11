from __future__ import annotations

import math
from dataclasses import dataclass, field
from typing import Any, Dict, List, Mapping, Sequence

from app.repositories import cleaning_native_results, cleaning_summary
from app.repositories.cleaning_native_segments import NativeSegmentUpdate
from app.repositories.cleaning_sql_counter import require_non_negative_count


def _default_step1() -> Dict[str, int]:
    return {}


def _elapsed_ms(value: object) -> int:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise RuntimeError("cleaning_timing_unavailable")
    numeric = float(value)
    if not math.isfinite(numeric) or numeric < 0:
        raise RuntimeError("cleaning_timing_unavailable")
    return int(numeric * 1000)


def _required_result_count(result: Mapping[str, Any], key: str) -> int:
    if key not in result:
        raise RuntimeError("legacy_cleaning_result_incomplete")
    return require_non_negative_count(
        result[key],
        code="legacy_cleaning_result_incomplete",
    )


def _required_step1(value: object) -> Dict[str, int]:
    if not isinstance(value, Mapping):
        raise RuntimeError("legacy_cleaning_result_incomplete")
    if any(key not in value for key in cleaning_native_results.AMOUNT_BALANCE_KEYS):
        raise RuntimeError("legacy_cleaning_result_incomplete")
    return {
        key: require_non_negative_count(
            value[key],
            code="legacy_cleaning_result_incomplete",
        )
        for key in cleaning_native_results.AMOUNT_BALANCE_KEYS
    }


def _validated_metric_map(values: Mapping[str, Any]) -> Dict[str, int]:
    result: Dict[str, int] = {}
    for key, value in values.items():
        name = str(key).strip()
        if not name:
            raise RuntimeError("cleaning_timing_unavailable")
        result[name] = require_non_negative_count(
            value,
            code="cleaning_timing_unavailable",
        )
    return result


@dataclass
class CleaningRunState:
    rows_affected_total: int = 0
    step1: Dict[str, int] = field(default_factory=_default_step1)
    invalid_total: int | None = None
    duplicate_total: int | None = None
    failed_total: int | None = None
    reversal_total: int | None = None
    normalized_count: int | None = None
    inferred_count: int | None = None
    filled_count: int | None = None
    account_invalid_total: int | None = None
    suffix_acc_total: int | None = None
    account_fill_total: int | None = None
    native_segments: List[str] = field(default_factory=list)
    python_fallback_steps: List[int] = field(default_factory=list)
    legacy_python_cleaning_steps: List[int] = field(default_factory=list)
    legacy_python_cleaning_reason: str = ""
    native_clean_all_attempted: bool = False
    native_clean_all_failed: bool = False
    native_completion_state_applied: bool = False
    native_timings_ms: Dict[str, int] = field(default_factory=dict)
    phase_timings_ms: Dict[str, int] = field(default_factory=dict)

    def set_phase_timing(self, key: str, elapsed_seconds: float) -> None:
        name = str(key or "").strip()
        if name:
            self.phase_timings_ms[name] = _elapsed_ms(elapsed_seconds)

    def add_phase_timing(self, key: str, elapsed_seconds: float) -> None:
        name = str(key or "").strip()
        if name:
            self.phase_timings_ms[name] = int(self.phase_timings_ms.get(name, 0)) + max(
                0,
                _elapsed_ms(elapsed_seconds),
            )

    def enter_legacy_step(self, step_no: int) -> None:
        self.python_fallback_steps.append(int(step_no))
        self.legacy_python_cleaning_steps.append(int(step_no))
        if not self.legacy_python_cleaning_reason:
            self.legacy_python_cleaning_reason = "native_unavailable"

    def apply_native_segment(self, update: NativeSegmentUpdate) -> None:
        self.native_segments.append(update.segment)
        if update.step1 is not None:
            self.step1 = _required_step1(update.step1)
        if update.rows_affected_total is not None:
            self.rows_affected_total = require_non_negative_count(
                update.rows_affected_total,
                code="cleaning_native_count_unavailable",
            )
        else:
            self.rows_affected_total += require_non_negative_count(
                update.rows_affected_delta,
                code="cleaning_native_count_unavailable",
            )
        if update.invalid_total is not None:
            self.invalid_total = require_non_negative_count(
                update.invalid_total,
                code="cleaning_native_count_unavailable",
            )
        if update.duplicate_total is not None:
            self.duplicate_total = require_non_negative_count(
                update.duplicate_total,
                code="cleaning_native_count_unavailable",
            )
        if update.failed_total is not None:
            self.failed_total = require_non_negative_count(
                update.failed_total,
                code="cleaning_native_count_unavailable",
            )
        if update.reversal_total is not None:
            self.reversal_total = require_non_negative_count(
                update.reversal_total,
                code="cleaning_native_count_unavailable",
            )
        if update.normalized_count is not None:
            self.normalized_count = require_non_negative_count(
                update.normalized_count,
                code="cleaning_native_count_unavailable",
            )
        if update.inferred_count is not None:
            self.inferred_count = require_non_negative_count(
                update.inferred_count,
                code="cleaning_native_count_unavailable",
            )
        if update.filled_count is not None:
            self.filled_count = require_non_negative_count(
                update.filled_count,
                code="cleaning_native_count_unavailable",
            )
        if update.account_invalid_total is not None:
            self.account_invalid_total = require_non_negative_count(
                update.account_invalid_total,
                code="cleaning_native_count_unavailable",
            )
        if update.suffix_acc_total is not None:
            self.suffix_acc_total = require_non_negative_count(
                update.suffix_acc_total,
                code="cleaning_native_count_unavailable",
            )
        if update.account_fill_total is not None:
            self.account_fill_total = require_non_negative_count(
                update.account_fill_total,
                code="cleaning_native_count_unavailable",
            )
        if update.timings_ms is not None:
            self.native_timings_ms.update(_validated_metric_map(update.timings_ms))
        if update.phase_timings_ms is not None:
            self.phase_timings_ms.update(_validated_metric_map(update.phase_timings_ms))
        if update.native_completion_state_applied:
            self.native_completion_state_applied = True

    def apply_legacy_front_result(self, step_no: int, result: Mapping[str, Any]) -> None:
        delta = _required_result_count(result, "rows_affected_delta")
        if step_no == 1:
            if "step1" not in result:
                raise RuntimeError("legacy_cleaning_result_incomplete")
            step1 = _required_step1(result["step1"])
            self.rows_affected_total += delta
            self.step1 = step1
            return
        if step_no == 2:
            total = _required_result_count(result, "invalid_total")
            self.rows_affected_total += delta
            self.invalid_total = total
            return
        if step_no == 3:
            total = _required_result_count(result, "duplicate_total")
            self.rows_affected_total += delta
            self.duplicate_total = total
            return
        if step_no == 4:
            failed_total = _required_result_count(result, "failed_total")
            reversal_total = _required_result_count(result, "reversal_total")
            self.rows_affected_total += delta
            self.failed_total = failed_total
            self.reversal_total = reversal_total
            return
        if step_no == 5:
            normalized_count = _required_result_count(result, "normalized_count")
            inferred_count = _required_result_count(result, "inferred_count")
            self.rows_affected_total += delta
            self.normalized_count = normalized_count
            self.inferred_count = inferred_count
            return
        raise RuntimeError(f"legacy python front cleaning does not own step{step_no}")

    def apply_legacy_account_result(self, step_no: int, result: Mapping[str, Any]) -> None:
        delta = _required_result_count(result, "rows_affected_delta")
        if step_no == 6:
            total = _required_result_count(result, "filled_count")
            self.rows_affected_total += delta
            self.filled_count = total
            return
        if step_no == 7:
            self.rows_affected_total += delta
            return
        if step_no == 8:
            total = _required_result_count(result, "account_invalid_total")
            self.rows_affected_total += delta
            self.account_invalid_total = total
            return
        if step_no == 9:
            total = _required_result_count(result, "suffix_acc_total")
            self.rows_affected_total += delta
            self.suffix_acc_total = total
            return
        if step_no == 10:
            total = _required_result_count(result, "account_fill_total")
            self.rows_affected_total += delta
            self.account_fill_total = total
            return
        raise RuntimeError(f"legacy python account cleaning does not own step{step_no}")

    def apply_legacy_result(self, step_no: int, result: Mapping[str, Any]) -> None:
        step = int(step_no)
        if 1 <= step <= 5:
            self.apply_legacy_front_result(step, result)
            return
        if 6 <= step <= 10:
            self.apply_legacy_account_result(step, result)
            return
        raise RuntimeError(f"legacy python cleaning does not own step{step}")

    def build_summary(
        self,
        *,
        scope_rows: int,
        scope_ids: Sequence[str],
        active_steps: Sequence[int],
        duration_ms: int,
    ) -> Dict[str, Any]:
        return cleaning_summary.build_cleaning_summary(
            scope_rows=scope_rows,
            scope_ids=scope_ids,
            active_steps=active_steps,
            native_segments=self.native_segments,
            python_fallback_steps=self.python_fallback_steps,
            legacy_python_cleaning_steps=self.legacy_python_cleaning_steps,
            legacy_python_cleaning_reason=self.legacy_python_cleaning_reason,
            native_clean_all_attempted=self.native_clean_all_attempted,
            native_clean_all_failed=self.native_clean_all_failed,
            invalid_total=self.invalid_total,
            duplicate_total=self.duplicate_total,
            failed_total=self.failed_total,
            reversal_total=self.reversal_total,
            normalized_count=self.normalized_count,
            inferred_count=self.inferred_count,
            filled_count=self.filled_count,
            account_invalid_total=self.account_invalid_total,
            suffix_acc_total=self.suffix_acc_total,
            account_fill_total=self.account_fill_total,
            rows_affected_total=self.rows_affected_total,
            duration_ms=duration_ms,
            step1=self.step1,
            native_timings_ms=self.native_timings_ms,
            phase_timings_ms=self.phase_timings_ms,
        )


__all__ = ["CleaningRunState"]
