from __future__ import annotations

import math
from pathlib import Path


def materialize_rule_txn_index_args(
    *,
    case_id: str,
    db_path: Path,
    force: bool = False,
) -> list[str]:
    return [
        "materialize-rule-txn-index",
        "--case-id",
        _clean(case_id),
        "--db-path",
        str(db_path),
        "--force",
        "true" if force else "false",
    ]


def materialize_rule_pattern_index_args(
    *,
    case_id: str,
    db_path: Path,
    param_signature: str,
    round_unit: float,
    round_min_amount: float,
    round_min_count: int,
    round_min_total_amount: float,
    small_fast_window_minutes: int,
    small_fast_ratio: float,
    small_fast_min_amount: float,
    small_fast_max_amount: float,
    cash_quick_window_minutes: int,
    cash_quick_min_amount: float,
    cash_candidate_window_minutes: int,
    cash_candidate_min_amount: float,
    cash_candidate_min_ratio: float,
    cash_candidate_max_ratio: float,
    near_threshold_amount: float,
    near_threshold_lower_rate: float,
    near_threshold_window_minutes: int,
    near_threshold_min_count: int,
    near_threshold_min_total_amount: float,
    repeated_amount_window_minutes: int,
    repeated_amount_min_amount: float,
    repeated_amount_min_count: int,
    repeated_amount_min_total_amount: float,
    threshold_split_window_minutes: int,
    threshold_split_amount: float,
    threshold_split_tolerance_rate: float,
    threshold_split_min_count: int,
    high_freq_small_amount_threshold: float,
    high_freq_window_minutes: int,
    high_freq_count_threshold: int,
    high_freq_min_total_amount: float,
    night_start_hour: int,
    night_end_hour: int,
    night_min_count: int,
    night_min_total_amount: float,
    force: bool = False,
) -> list[str]:
    if type(force) is not bool:
        raise ValueError("force_invalid")
    args = [
        "materialize-rule-pattern-index",
        *_string_arg("--case-id", case_id),
        "--db-path",
        str(db_path),
        *_string_arg("--param-signature", param_signature),
        *_round_pattern_args(
            round_unit=round_unit,
            round_min_amount=round_min_amount,
            round_min_count=round_min_count,
            round_min_total_amount=round_min_total_amount,
        ),
        *_small_fast_args(
            small_fast_window_minutes=small_fast_window_minutes,
            small_fast_ratio=small_fast_ratio,
            small_fast_min_amount=small_fast_min_amount,
            small_fast_max_amount=small_fast_max_amount,
        ),
        *_cash_pattern_args(
            cash_quick_window_minutes=cash_quick_window_minutes,
            cash_quick_min_amount=cash_quick_min_amount,
            cash_candidate_window_minutes=cash_candidate_window_minutes,
            cash_candidate_min_amount=cash_candidate_min_amount,
            cash_candidate_min_ratio=cash_candidate_min_ratio,
            cash_candidate_max_ratio=cash_candidate_max_ratio,
        ),
        *_near_threshold_args(
            near_threshold_amount=near_threshold_amount,
            near_threshold_lower_rate=near_threshold_lower_rate,
            near_threshold_window_minutes=near_threshold_window_minutes,
            near_threshold_min_count=near_threshold_min_count,
            near_threshold_min_total_amount=near_threshold_min_total_amount,
        ),
        *_repeated_amount_args(
            repeated_amount_window_minutes=repeated_amount_window_minutes,
            repeated_amount_min_amount=repeated_amount_min_amount,
            repeated_amount_min_count=repeated_amount_min_count,
            repeated_amount_min_total_amount=repeated_amount_min_total_amount,
        ),
        *_threshold_split_args(
            threshold_split_window_minutes=threshold_split_window_minutes,
            threshold_split_amount=threshold_split_amount,
            threshold_split_tolerance_rate=threshold_split_tolerance_rate,
            threshold_split_min_count=threshold_split_min_count,
        ),
        *_high_freq_args(
            high_freq_small_amount_threshold=high_freq_small_amount_threshold,
            high_freq_window_minutes=high_freq_window_minutes,
            high_freq_count_threshold=high_freq_count_threshold,
            high_freq_min_total_amount=high_freq_min_total_amount,
        ),
        *_night_args(
            night_start_hour=night_start_hour,
            night_end_hour=night_end_hour,
            night_min_count=night_min_count,
            night_min_total_amount=night_min_total_amount,
        ),
        "--force",
        "true" if force else "false",
    ]
    return args


def _round_pattern_args(
    *,
    round_unit: float,
    round_min_amount: float,
    round_min_count: int,
    round_min_total_amount: float,
) -> list[str]:
    return [
        *_float_arg("--round-unit", round_unit, minimum=1.0),
        *_float_arg("--round-min-amount", round_min_amount, minimum=0.0),
        *_int_arg("--round-min-count", round_min_count, minimum=2),
        *_float_arg("--round-min-total-amount", round_min_total_amount, minimum=0.0),
    ]


def _small_fast_args(
    *,
    small_fast_window_minutes: int,
    small_fast_ratio: float,
    small_fast_min_amount: float,
    small_fast_max_amount: float,
) -> list[str]:
    _require_ordered(
        "small_fast_max_amount",
        small_fast_max_amount,
        "small_fast_min_amount",
        small_fast_min_amount,
    )
    return [
        *_int_arg("--small-fast-window-minutes", small_fast_window_minutes, minimum=1),
        *_float_arg("--small-fast-ratio", small_fast_ratio, minimum=0.1),
        *_float_arg("--small-fast-min-amount", small_fast_min_amount, minimum=0.0),
        *_float_arg("--small-fast-max-amount", small_fast_max_amount, minimum=0.0),
    ]


def _cash_pattern_args(
    *,
    cash_quick_window_minutes: int,
    cash_quick_min_amount: float,
    cash_candidate_window_minutes: int,
    cash_candidate_min_amount: float,
    cash_candidate_min_ratio: float,
    cash_candidate_max_ratio: float,
) -> list[str]:
    _require_ordered(
        "cash_candidate_max_ratio",
        cash_candidate_max_ratio,
        "cash_candidate_min_ratio",
        cash_candidate_min_ratio,
    )
    return [
        *_int_arg("--cash-quick-window-minutes", cash_quick_window_minutes, minimum=1),
        *_float_arg("--cash-quick-min-amount", cash_quick_min_amount, minimum=0.0),
        *_int_arg(
            "--cash-candidate-window-minutes",
            cash_candidate_window_minutes,
            minimum=1,
        ),
        *_float_arg("--cash-candidate-min-amount", cash_candidate_min_amount, minimum=0.0),
        *_float_arg("--cash-candidate-min-ratio", cash_candidate_min_ratio, minimum=0.1),
        *_float_arg("--cash-candidate-max-ratio", cash_candidate_max_ratio, minimum=0.1),
    ]


def _near_threshold_args(
    *,
    near_threshold_amount: float,
    near_threshold_lower_rate: float,
    near_threshold_window_minutes: int,
    near_threshold_min_count: int,
    near_threshold_min_total_amount: float,
) -> list[str]:
    return [
        *_float_arg("--near-threshold-amount", near_threshold_amount, minimum=0.0),
        *_float_arg("--near-threshold-lower-rate", near_threshold_lower_rate, minimum=0.0, maximum=0.99),
        *_int_arg(
            "--near-threshold-window-minutes",
            near_threshold_window_minutes,
            minimum=1,
        ),
        *_int_arg("--near-threshold-min-count", near_threshold_min_count, minimum=2),
        *_float_arg("--near-threshold-min-total-amount", near_threshold_min_total_amount, minimum=0.0),
    ]


def _repeated_amount_args(
    *,
    repeated_amount_window_minutes: int,
    repeated_amount_min_amount: float,
    repeated_amount_min_count: int,
    repeated_amount_min_total_amount: float,
) -> list[str]:
    return [
        *_int_arg(
            "--repeated-amount-window-minutes",
            repeated_amount_window_minutes,
            minimum=1,
        ),
        *_float_arg("--repeated-amount-min-amount", repeated_amount_min_amount, minimum=0.0),
        *_int_arg("--repeated-amount-min-count", repeated_amount_min_count, minimum=2),
        *_float_arg(
            "--repeated-amount-min-total-amount",
            repeated_amount_min_total_amount,
            minimum=0.0,
        ),
    ]


def _threshold_split_args(
    *,
    threshold_split_window_minutes: int,
    threshold_split_amount: float,
    threshold_split_tolerance_rate: float,
    threshold_split_min_count: int,
) -> list[str]:
    return [
        *_int_arg(
            "--threshold-split-window-minutes",
            threshold_split_window_minutes,
            minimum=1,
        ),
        *_float_arg("--threshold-split-amount", threshold_split_amount, minimum=0.0),
        *_float_arg("--threshold-split-tolerance-rate", threshold_split_tolerance_rate, minimum=0.0),
        *_int_arg("--threshold-split-min-count", threshold_split_min_count, minimum=2),
    ]


def _high_freq_args(
    *,
    high_freq_small_amount_threshold: float,
    high_freq_window_minutes: int,
    high_freq_count_threshold: int,
    high_freq_min_total_amount: float,
) -> list[str]:
    return [
        *_float_arg(
            "--high-freq-small-amount-threshold",
            high_freq_small_amount_threshold,
            minimum=0.0,
        ),
        *_int_arg("--high-freq-window-minutes", high_freq_window_minutes, minimum=1),
        *_int_arg("--high-freq-count-threshold", high_freq_count_threshold, minimum=2),
        *_float_arg("--high-freq-min-total-amount", high_freq_min_total_amount, minimum=0.0),
    ]


def _night_args(
    *,
    night_start_hour: int,
    night_end_hour: int,
    night_min_count: int,
    night_min_total_amount: float,
) -> list[str]:
    return [
        *_int_arg("--night-start-hour", night_start_hour, minimum=0, maximum=23),
        *_int_arg("--night-end-hour", night_end_hour, minimum=0, maximum=23),
        *_int_arg("--night-min-count", night_min_count, minimum=2),
        *_float_arg("--night-min-total-amount", night_min_total_amount, minimum=0.0),
    ]


def _string_arg(flag: str, value: object) -> list[str]:
    if not isinstance(value, str) or not value.strip():
        raise ValueError(_invalid_code(flag))
    return [flag, value.strip()]


def _float_arg(
    flag: str,
    value: object,
    *,
    minimum: float | None = None,
    maximum: float | None = None,
) -> list[str]:
    if value is None or isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError(_invalid_code(flag))
    number = float(value)
    if (
        not math.isfinite(number)
        or (minimum is not None and number < minimum)
        or (maximum is not None and number > maximum)
    ):
        raise ValueError(_invalid_code(flag))
    return [flag, str(number)]


def _int_arg(
    flag: str,
    value: object,
    *,
    minimum: int | None = None,
    maximum: int | None = None,
) -> list[str]:
    if type(value) is not int:
        raise ValueError(_invalid_code(flag))
    if (minimum is not None and value < minimum) or (maximum is not None and value > maximum):
        raise ValueError(_invalid_code(flag))
    return [flag, str(value)]


def _require_ordered(
    upper_name: str,
    upper_value: object,
    lower_name: str,
    lower_value: object,
) -> None:
    upper = _validated_float(upper_name, upper_value)
    lower = _validated_float(lower_name, lower_value)
    if upper < lower:
        raise ValueError(f"{upper_name}_invalid")


def _validated_float(name: str, value: object) -> float:
    if value is None or isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError(f"{name}_invalid")
    number = float(value)
    if not math.isfinite(number):
        raise ValueError(f"{name}_invalid")
    return number


def _invalid_code(flag: str) -> str:
    return f"{flag.removeprefix('--').replace('-', '_')}_invalid"


def _clean(value: object) -> str:
    return str(value or "").strip()
