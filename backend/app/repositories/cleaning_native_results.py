from __future__ import annotations

from typing import Any, Dict, Mapping, Tuple


class CleaningNativeResultError(RuntimeError):
    pass


AMOUNT_BALANCE_KEYS: Tuple[str, ...] = (
    "amount_updated",
    "balance_updated",
    "amount_failed",
    "balance_failed",
    "amount_fixed_total",
    "amount_failed_total",
    "balance_fixed_total",
    "balance_failed_total",
    "rows_affected",
)

CLEAN_ALL_FACT_KEYS: Tuple[str, ...] = (
    *AMOUNT_BALANCE_KEYS,
    "invalid_changed",
    "invalid_total",
    "duplicate_changed",
    "duplicate_total",
    "failed_changed",
    "reversal_changed",
    "failed_total",
    "reversal_total",
    "normalized",
    "inferred",
    "normalized_total",
    "inferred_total",
    "filled_count",
    "filled_total",
    "suffix_txn_count",
    "suffix_txn_total",
    "account_invalid_count",
    "account_invalid_total",
    "suffix_acc_count",
    "suffix_acc_total",
    "account_fill_count",
    "account_fill_total",
    "rows_affected_total",
    "native_completion_applied",
)

CLEAN_ALL_TELEMETRY_KEYS: Tuple[str, ...] = (
    "native_subprocess_ms",
    "native_subprocess_spawn_ms",
    "native_subprocess_communicate_ms",
    "native_subprocess_json_parse_ms",
    "native_subprocess_unattributed_ms",
    "native_worker_request_ms",
    "native_worker_spawn_ms",
    "native_worker_roundtrip_ms",
    "native_worker_reused",
    "clean_all_open_db_ms",
    "clean_all_configure_ms",
    "clean_all_ensure_txn_table_ms",
    "clean_all_txn_scope_ms",
    "clean_all_acct_state_ms",
    "clean_all_acc_scope_ms",
    "clean_all_begin_ms",
    "clean_all_steps_ms",
    "clean_all_temp_cleanup_ms",
    "clean_all_rust_total_ms",
    "clean_all_connection_drop_ms",
    "amount_balance_ms",
    "amount_balance_desired_ms",
    "amount_balance_counts_ms",
    "amount_balance_update_ms",
    "amount_balance_totals_ms",
    "quality_flags_ms",
    "dc_flag_ms",
    "account_keys_ms",
    "account_info_ms",
    "clean_all_commit_ms",
    "account_info_pending_ms",
    "account_info_key_maps_ms",
    "account_info_primary_match_ms",
    "account_info_projection_ms",
    "account_info_txn_unique_map_ms",
    "account_info_txn_unique_match_ms",
    "account_info_counterparty_map_ms",
    "account_info_counterparty_match_ms",
    "account_info_alpha_card_match_ms",
    "account_info_acct_to_card_map_ms",
    "account_info_acct_to_card_match_ms",
    "account_info_fill_total_ms",
    "native_completion_mark_txn_ms",
    "native_completion_privacy_projection_ms",
    "native_completion_mark_account_ms",
    "native_completion_account_state_ms",
    "native_completion_total_ms",
)

CLEAN_ALL_KEYS: Tuple[str, ...] = (*CLEAN_ALL_FACT_KEYS, *CLEAN_ALL_TELEMETRY_KEYS)

QUALITY_FLAG_KEYS: Tuple[str, ...] = (
    "invalid_changed",
    "invalid_total",
    "duplicate_changed",
    "duplicate_total",
    "failed_changed",
    "reversal_changed",
    "failed_total",
    "reversal_total",
)

DC_FLAG_KEYS: Tuple[str, ...] = (
    "normalized",
    "inferred",
    "normalized_total",
    "inferred_total",
)

ACCOUNT_KEYS_KEYS: Tuple[str, ...] = (
    "filled_count",
    "filled_total",
    "suffix_txn_count",
    "suffix_txn_total",
    "account_invalid_count",
    "account_invalid_total",
    "suffix_acc_count",
    "suffix_acc_total",
)

ACCOUNT_INFO_KEYS: Tuple[str, ...] = (
    "account_fill_count",
    "account_fill_total",
    "account_info_pending_ms",
    "account_info_key_maps_ms",
    "account_info_primary_match_ms",
    "account_info_projection_ms",
    "account_info_txn_unique_map_ms",
    "account_info_txn_unique_match_ms",
    "account_info_counterparty_map_ms",
    "account_info_counterparty_match_ms",
    "account_info_alpha_card_match_ms",
    "account_info_acct_to_card_map_ms",
    "account_info_acct_to_card_match_ms",
    "account_info_fill_total_ms",
)

ACCOUNT_INFO_FACT_KEYS: Tuple[str, ...] = (
    "account_fill_count",
    "account_fill_total",
)

ACCOUNT_INFO_TELEMETRY_KEYS: Tuple[str, ...] = tuple(
    key for key in ACCOUNT_INFO_KEYS if key not in ACCOUNT_INFO_FACT_KEYS
)


def _non_negative_result_int(payload: Mapping[str, Any], key: str) -> int:
    value = payload[key]
    if type(value) is not int or value < 0 or value > 9_223_372_036_854_775_807:
        raise CleaningNativeResultError("cleaning_native_result_invalid")
    return value


def _int_result(
    payload: Mapping[str, Any],
    required_keys: Tuple[str, ...],
    *,
    optional_keys: Tuple[str, ...] = (),
    expected_case_id: str,
) -> Dict[str, int]:
    normalized_case_id = str(expected_case_id or "").strip()
    allowed_keys = {"ok", "case_id", *required_keys, *optional_keys}
    if (
        not normalized_case_id
        or normalized_case_id == "active"
        or payload.get("ok") is not True
        or type(payload.get("case_id")) is not str
        or payload.get("case_id") != normalized_case_id
        or any(key not in allowed_keys for key in payload)
        or any(key not in payload for key in required_keys)
    ):
        raise CleaningNativeResultError("cleaning_native_result_invalid")
    result = {key: _non_negative_result_int(payload, key) for key in required_keys}
    for key in optional_keys:
        if key in payload:
            result[key] = _non_negative_result_int(payload, key)
    return result


def amount_balance_result(payload: Mapping[str, Any], *, expected_case_id: str) -> Dict[str, int]:
    return _int_result(payload, AMOUNT_BALANCE_KEYS, expected_case_id=expected_case_id)


def clean_all_result(payload: Mapping[str, Any], *, expected_case_id: str) -> Dict[str, int]:
    result = _int_result(
        payload,
        CLEAN_ALL_FACT_KEYS,
        optional_keys=CLEAN_ALL_TELEMETRY_KEYS,
        expected_case_id=expected_case_id,
    )
    if result["native_completion_applied"] != 1:
        raise CleaningNativeResultError("cleaning_native_result_incomplete")
    return result


def quality_flags_result(payload: Mapping[str, Any], *, expected_case_id: str) -> Dict[str, int]:
    return _int_result(payload, QUALITY_FLAG_KEYS, expected_case_id=expected_case_id)


def dc_flag_result(payload: Mapping[str, Any], *, expected_case_id: str) -> Dict[str, int]:
    return _int_result(payload, DC_FLAG_KEYS, expected_case_id=expected_case_id)


def account_keys_result(payload: Mapping[str, Any], *, expected_case_id: str) -> Dict[str, int]:
    return _int_result(payload, ACCOUNT_KEYS_KEYS, expected_case_id=expected_case_id)


def account_info_result(payload: Mapping[str, Any], *, expected_case_id: str) -> Dict[str, int]:
    return _int_result(
        payload,
        ACCOUNT_INFO_FACT_KEYS,
        optional_keys=ACCOUNT_INFO_TELEMETRY_KEYS,
        expected_case_id=expected_case_id,
    )


__all__ = [
    "ACCOUNT_INFO_KEYS",
    "ACCOUNT_INFO_FACT_KEYS",
    "ACCOUNT_INFO_TELEMETRY_KEYS",
    "ACCOUNT_KEYS_KEYS",
    "AMOUNT_BALANCE_KEYS",
    "CLEAN_ALL_KEYS",
    "CLEAN_ALL_FACT_KEYS",
    "CLEAN_ALL_TELEMETRY_KEYS",
    "DC_FLAG_KEYS",
    "QUALITY_FLAG_KEYS",
    "CleaningNativeResultError",
    "account_info_result",
    "account_keys_result",
    "amount_balance_result",
    "clean_all_result",
    "dc_flag_result",
    "quality_flags_result",
]
