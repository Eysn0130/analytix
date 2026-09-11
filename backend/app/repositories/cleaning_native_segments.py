from __future__ import annotations

from dataclasses import dataclass
from typing import Dict, Mapping, Optional, Tuple

from app.repositories import cleaning_native_results


StepProgress = Tuple[int, str]


@dataclass(frozen=True)
class NativeSegmentUpdate:
    segment: str
    progress: Tuple[StepProgress, ...]
    rows_affected_delta: int = 0
    rows_affected_total: Optional[int] = None
    step1: Optional[Dict[str, int]] = None
    invalid_total: Optional[int] = None
    duplicate_total: Optional[int] = None
    failed_total: Optional[int] = None
    reversal_total: Optional[int] = None
    normalized_count: Optional[int] = None
    inferred_count: Optional[int] = None
    filled_count: Optional[int] = None
    account_invalid_total: Optional[int] = None
    suffix_acc_total: Optional[int] = None
    account_fill_total: Optional[int] = None
    timings_ms: Optional[Dict[str, int]] = None
    phase_timings_ms: Optional[Dict[str, int]] = None
    native_completion_state_applied: bool = False


def _amount_step1(result: Mapping[str, int]) -> Dict[str, int]:
    return {key: result[key] for key in cleaning_native_results.AMOUNT_BALANCE_KEYS}


def clean_all_update(result: Mapping[str, int], *, elapsed: float) -> NativeSegmentUpdate:
    invalid_changed = result["invalid_changed"]
    invalid_total = result["invalid_total"]
    duplicate_changed = result["duplicate_changed"]
    duplicate_total = result["duplicate_total"]
    failed_changed = result["failed_changed"]
    reversal_changed = result["reversal_changed"]
    failed_total = result["failed_total"]
    reversal_total = result["reversal_total"]
    normalized_count = result["normalized"]
    inferred_count = result["inferred"]
    norm_total = result["normalized_total"]
    infer_total = result["inferred_total"]
    filled_count = result["filled_count"]
    filled_total = result["filled_total"]
    suffix_txn_count = result["suffix_txn_count"]
    suffix_txn_total = result["suffix_txn_total"]
    account_invalid_count = result["account_invalid_count"]
    account_invalid_total = result["account_invalid_total"]
    suffix_acc_count = result["suffix_acc_count"]
    suffix_acc_total = result["suffix_acc_total"]
    account_fill_count = result["account_fill_count"]
    account_fill_total = result["account_fill_total"]
    amount_elapsed_text = _native_elapsed_text(result, "amount_balance_ms", elapsed)
    quality_elapsed_text = _native_elapsed_text(result, "quality_flags_ms", elapsed)
    dc_elapsed_text = _native_elapsed_text(result, "dc_flag_ms", elapsed)
    account_keys_elapsed_text = _native_elapsed_text(result, "account_keys_ms", elapsed)
    account_info_elapsed_text = _native_elapsed_text(result, "account_info_ms", elapsed)
    timings_ms = _native_timings_ms(result)
    phase_timings_ms = _native_completion_phase_timings_ms(result)
    return NativeSegmentUpdate(
        segment="clean-all",
        step1=_amount_step1(result),
        invalid_total=invalid_total,
        duplicate_total=duplicate_total,
        failed_total=failed_total,
        reversal_total=reversal_total,
        normalized_count=normalized_count,
        inferred_count=inferred_count,
        filled_count=filled_count,
        account_invalid_total=account_invalid_total,
        suffix_acc_total=suffix_acc_total,
        account_fill_total=account_fill_total,
        timings_ms=timings_ms or None,
        phase_timings_ms=phase_timings_ms or None,
        native_completion_state_applied=result["native_completion_applied"] == 1,
        rows_affected_total=result["rows_affected_total"],
        progress=(
            (1, f"step1 amount/balance normalization (native clean-all {amount_elapsed_text})"),
            (
                2,
                (
                    f"step2 invalid mark changed={invalid_changed} total={invalid_total} "
                    f"(native clean-all {quality_elapsed_text})"
                ),
            ),
            (
                3,
                (
                    f"step3 duplicate mark(clear) changed={duplicate_changed} total={duplicate_total} "
                    f"(native clean-all {quality_elapsed_text})"
                ),
            ),
            (
                4,
                (
                    f"step4 failed/reversal changed_failed={failed_changed} "
                    f"changed_reversal={reversal_changed} total_failed={failed_total} "
                    f"total_reversal={reversal_total} (native clean-all {quality_elapsed_text})"
                ),
            ),
            (
                5,
                (
                    f"step5 dc normalize={normalized_count} infer={inferred_count} "
                    f"total_normalized={norm_total} total_inferred={infer_total} "
                    f"(native clean-all {dc_elapsed_text})"
                ),
            ),
            (
                6,
                (
                    f"step6 fill card changed={filled_count} total={filled_total} "
                    f"(native clean-all {account_keys_elapsed_text})"
                ),
            ),
            (
                7,
                (
                    f"step7 txn suffix changed={suffix_txn_count} total={suffix_txn_total} "
                    f"(native clean-all {account_keys_elapsed_text})"
                ),
            ),
            (
                8,
                (
                    f"step8 account invalid changed={account_invalid_count} "
                    f"total={account_invalid_total} (native clean-all {account_keys_elapsed_text})"
                ),
            ),
            (
                9,
                (
                    f"step9 account suffix changed={suffix_acc_count} total={suffix_acc_total} "
                    f"(native clean-all {account_keys_elapsed_text})"
                ),
            ),
            (
                10,
                (
                    f"step10 fill account info changed={account_fill_count} "
                    f"total={account_fill_total} (native clean-all {account_info_elapsed_text})"
                ),
            ),
        ),
    )


def amount_balance_update(result: Mapping[str, int], *, elapsed: float) -> NativeSegmentUpdate:
    step1 = _amount_step1(result)
    return NativeSegmentUpdate(
        segment="amount-balance",
        step1=step1,
        rows_affected_delta=step1["rows_affected"],
        progress=((1, f"step1 amount/balance normalization (native {elapsed:.2f}s)"),),
    )


def quality_flags_update(result: Mapping[str, int], *, elapsed: float) -> NativeSegmentUpdate:
    invalid_changed = result["invalid_changed"]
    invalid_total = result["invalid_total"]
    duplicate_changed = result["duplicate_changed"]
    duplicate_total = result["duplicate_total"]
    failed_changed = result["failed_changed"]
    reversal_changed = result["reversal_changed"]
    failed_total = result["failed_total"]
    reversal_total = result["reversal_total"]
    return NativeSegmentUpdate(
        segment="quality-flags",
        invalid_total=invalid_total,
        duplicate_total=duplicate_total,
        failed_total=failed_total,
        reversal_total=reversal_total,
        rows_affected_delta=invalid_changed + duplicate_changed + failed_changed + reversal_changed,
        progress=(
            (2, f"step2 invalid mark changed={invalid_changed} total={invalid_total} (native {elapsed:.2f}s)"),
            (3, f"step3 duplicate mark(clear) changed={duplicate_changed} total={duplicate_total} (native {elapsed:.2f}s)"),
            (
                4,
                (
                    f"step4 failed/reversal changed_failed={failed_changed} changed_reversal={reversal_changed} "
                    f"total_failed={failed_total} total_reversal={reversal_total} (native {elapsed:.2f}s)"
                ),
            ),
        ),
    )


def dc_flag_update(result: Mapping[str, int], *, elapsed: float) -> NativeSegmentUpdate:
    normalized_count = result["normalized"]
    inferred_count = result["inferred"]
    norm_total = result["normalized_total"]
    infer_total = result["inferred_total"]
    return NativeSegmentUpdate(
        segment="dc-flag",
        normalized_count=normalized_count,
        inferred_count=inferred_count,
        rows_affected_delta=normalized_count + inferred_count,
        progress=(
            (
                5,
                (
                    f"step5 dc normalize={normalized_count} infer={inferred_count} "
                    f"total_normalized={norm_total} total_inferred={infer_total} "
                    f"(native {elapsed:.2f}s)"
                ),
            ),
        ),
    )


def account_keys_update(result: Mapping[str, int], *, elapsed: float) -> NativeSegmentUpdate:
    filled_count = result["filled_count"]
    filled_total = result["filled_total"]
    suffix_txn_count = result["suffix_txn_count"]
    suffix_txn_total = result["suffix_txn_total"]
    account_invalid_count = result["account_invalid_count"]
    account_invalid_total = result["account_invalid_total"]
    suffix_acc_count = result["suffix_acc_count"]
    suffix_acc_total = result["suffix_acc_total"]
    return NativeSegmentUpdate(
        segment="account-keys",
        filled_count=filled_count,
        account_invalid_total=account_invalid_total,
        suffix_acc_total=suffix_acc_total,
        rows_affected_delta=filled_count + suffix_txn_count + account_invalid_count + suffix_acc_count,
        progress=(
            (6, f"step6 fill card changed={filled_count} total={filled_total} (native {elapsed:.2f}s)"),
            (7, f"step7 txn suffix changed={suffix_txn_count} total={suffix_txn_total} (native {elapsed:.2f}s)"),
            (8, f"step8 account invalid changed={account_invalid_count} total={account_invalid_total} (native {elapsed:.2f}s)"),
            (9, f"step9 account suffix changed={suffix_acc_count} total={suffix_acc_total} (native {elapsed:.2f}s)"),
        ),
    )


def account_info_update(result: Mapping[str, int], *, elapsed: float) -> NativeSegmentUpdate:
    account_fill_count = result["account_fill_count"]
    account_fill_total = result["account_fill_total"]
    timings_ms = _native_account_info_timings_ms(result)
    return NativeSegmentUpdate(
        segment="account-info",
        account_fill_total=account_fill_total,
        rows_affected_delta=account_fill_count,
        timings_ms=timings_ms or None,
        progress=(
            (
                10,
                (
                    f"step10 fill account info changed={account_fill_count} "
                    f"total={account_fill_total} (native {elapsed:.2f}s)"
                ),
            ),
        ),
    )


def _native_elapsed_text(result: Mapping[str, int], key: str, fallback_elapsed: float) -> str:
    elapsed_ms = _optional_metric(result, key)
    if elapsed_ms is not None:
        return f"{elapsed_ms / 1000:.2f}s"
    return f"{fallback_elapsed:.2f}s"


def _optional_metric(result: Mapping[str, int], key: str) -> int | None:
    if key not in result:
        return None
    value = result[key]
    if type(value) is not int or value < 0:
        raise cleaning_native_results.CleaningNativeResultError(
            "cleaning_native_result_invalid"
        )
    return value


def _metric_map(result: Mapping[str, int], keys: Mapping[str, str]) -> Dict[str, int]:
    projected: Dict[str, int] = {}
    for public_key, source_key in keys.items():
        value = _optional_metric(result, source_key)
        if value is not None:
            projected[public_key] = value
    return projected


def _native_timings_ms(result: Mapping[str, int]) -> Dict[str, int]:
    timings = _metric_map(
        result,
        {
            "amount_balance": "amount_balance_ms",
            "quality_flags": "quality_flags_ms",
            "dc_flag": "dc_flag_ms",
            "account_keys": "account_keys_ms",
            "account_info": "account_info_ms",
        },
    )
    timings.update(_native_amount_balance_timings_ms(result))
    timings.update(_native_clean_all_timings_ms(result))
    timings.update(_native_account_info_timings_ms(result))
    return timings


def _native_amount_balance_timings_ms(result: Mapping[str, int]) -> Dict[str, int]:
    return _metric_map(
        result,
        {
            "amount_balance_desired": "amount_balance_desired_ms",
            "amount_balance_counts": "amount_balance_counts_ms",
            "amount_balance_update": "amount_balance_update_ms",
            "amount_balance_totals": "amount_balance_totals_ms",
        },
    )


def _native_clean_all_timings_ms(result: Mapping[str, int]) -> Dict[str, int]:
    return _metric_map(
        result,
        {
            "native_subprocess": "native_subprocess_ms",
            "native_subprocess_spawn": "native_subprocess_spawn_ms",
            "native_subprocess_communicate": "native_subprocess_communicate_ms",
            "native_subprocess_json_parse": "native_subprocess_json_parse_ms",
            "native_subprocess_unattributed": "native_subprocess_unattributed_ms",
            "native_worker_request": "native_worker_request_ms",
            "native_worker_spawn": "native_worker_spawn_ms",
            "native_worker_roundtrip": "native_worker_roundtrip_ms",
            "native_worker_reused": "native_worker_reused",
            "clean_all_open_db": "clean_all_open_db_ms",
            "clean_all_configure": "clean_all_configure_ms",
            "clean_all_ensure_txn_table": "clean_all_ensure_txn_table_ms",
            "clean_all_txn_scope": "clean_all_txn_scope_ms",
            "clean_all_acct_state": "clean_all_acct_state_ms",
            "clean_all_acc_scope": "clean_all_acc_scope_ms",
            "clean_all_begin": "clean_all_begin_ms",
            "clean_all_steps": "clean_all_steps_ms",
            "clean_all_temp_cleanup": "clean_all_temp_cleanup_ms",
            "clean_all_commit": "clean_all_commit_ms",
            "clean_all_connection_drop": "clean_all_connection_drop_ms",
            "clean_all_rust_total": "clean_all_rust_total_ms",
            "native_completion_total": "native_completion_total_ms",
            "native_completion_mark_txn": "native_completion_mark_txn_ms",
            "native_completion_privacy_projection": "native_completion_privacy_projection_ms",
            "native_completion_mark_account": "native_completion_mark_account_ms",
            "native_completion_account_state": "native_completion_account_state_ms",
        },
    )


def _native_account_info_timings_ms(result: Mapping[str, int]) -> Dict[str, int]:
    return _metric_map(
        result,
        {
            "account_info_pending": "account_info_pending_ms",
            "account_info_key_maps": "account_info_key_maps_ms",
            "account_info_primary_match": "account_info_primary_match_ms",
            "account_info_projection": "account_info_projection_ms",
            "account_info_txn_unique_map": "account_info_txn_unique_map_ms",
            "account_info_txn_unique_match": "account_info_txn_unique_match_ms",
            "account_info_counterparty_map": "account_info_counterparty_map_ms",
            "account_info_counterparty_match": "account_info_counterparty_match_ms",
            "account_info_alpha_card_match": "account_info_alpha_card_match_ms",
            "account_info_acct_to_card_map": "account_info_acct_to_card_map_ms",
            "account_info_acct_to_card_match": "account_info_acct_to_card_match_ms",
            "account_info_fill_total": "account_info_fill_total_ms",
        },
    )


def _native_completion_phase_timings_ms(result: Mapping[str, int]) -> Dict[str, int]:
    return _metric_map(
        result,
        {
            "completion_mark_txn": "native_completion_mark_txn_ms",
            "completion_privacy_projection": "native_completion_privacy_projection_ms",
            "completion_mark_account": "native_completion_mark_account_ms",
            "completion_account_state": "native_completion_account_state_ms",
        },
    )


__all__ = [
    "NativeSegmentUpdate",
    "StepProgress",
    "account_info_update",
    "account_keys_update",
    "amount_balance_update",
    "clean_all_update",
    "dc_flag_update",
    "quality_flags_update",
]
