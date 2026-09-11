from __future__ import annotations

from typing import Any

from app.repositories import cleaning_account_steps
from app.repositories.cleaning_legacy_step_spec import (
    LegacyCleaningStepSpec,
    format_changed_total_message,
    format_elapsed_message,
    map_count_total_result,
)
from app.repositories.cleaning_step_context import CleaningStepContext


def _run_fill_card(context: CleaningStepContext, cur: Any, _total_rows: int) -> tuple[int, int]:
    return cleaning_account_steps.step_fill_card(context, cur)


def _map_fill_card(result: tuple[int, int]) -> dict[str, Any]:
    return map_count_total_result(result, count_key="filled_count", total_key="filled_total")


def _progress_fill_card(result: tuple[int, int], elapsed: float) -> str:
    filled_count, filled_total = result
    return format_changed_total_message("step6 fill card", filled_count, filled_total, elapsed)


def _run_suffix_transaction(context: CleaningStepContext, cur: Any, _total_rows: int) -> tuple[int, int]:
    return cleaning_account_steps.step_suffix_transaction(context, cur)


def _map_suffix_transaction(result: tuple[int, int]) -> dict[str, Any]:
    return map_count_total_result(result, count_key="suffix_txn_count", total_key="suffix_txn_total")


def _progress_suffix_transaction(result: tuple[int, int], elapsed: float) -> str:
    suffix_txn_count, suffix_txn_total = result
    return format_changed_total_message("step7 txn suffix", suffix_txn_count, suffix_txn_total, elapsed)


def _run_account_invalid(context: CleaningStepContext, cur: Any, _total_rows: int) -> tuple[int, int]:
    return cleaning_account_steps.step_account_invalid(context, cur)


def _map_account_invalid(result: tuple[int, int]) -> dict[str, Any]:
    return map_count_total_result(result, count_key="account_invalid_count", total_key="account_invalid_total")


def _progress_account_invalid(result: tuple[int, int], elapsed: float) -> str:
    account_invalid_count, account_invalid_total = result
    return format_elapsed_message(
        f"step8 account invalid changed={account_invalid_count} "
        f"total={account_invalid_total}",
        elapsed,
    )


def _run_suffix_account(context: CleaningStepContext, cur: Any, _total_rows: int) -> tuple[int, int]:
    return cleaning_account_steps.step_suffix_account(context, cur)


def _map_suffix_account(result: tuple[int, int]) -> dict[str, Any]:
    return map_count_total_result(result, count_key="suffix_acc_count", total_key="suffix_acc_total")


def _progress_suffix_account(result: tuple[int, int], elapsed: float) -> str:
    suffix_acc_count, suffix_acc_total = result
    return format_changed_total_message("step9 account suffix", suffix_acc_count, suffix_acc_total, elapsed)


def _run_fill_account_info(context: CleaningStepContext, cur: Any, _total_rows: int) -> tuple[int, int]:
    return cleaning_account_steps.step_fill_account_info(context, cur)


def _map_fill_account_info(result: tuple[int, int]) -> dict[str, Any]:
    return map_count_total_result(result, count_key="account_fill_count", total_key="account_fill_total")


def _progress_fill_account_info(result: tuple[int, int], elapsed: float) -> str:
    account_fill_count, account_fill_total = result
    return format_elapsed_message(
        f"step10 fill account info changed={account_fill_count} "
        f"total={account_fill_total}",
        elapsed,
    )


ACCOUNT_LEGACY_STEP_SPECS = {
    6: LegacyCleaningStepSpec(
        step_no=6,
        run=_run_fill_card,
        map_result=_map_fill_card,
        format_progress=_progress_fill_card,
    ),
    7: LegacyCleaningStepSpec(
        step_no=7,
        run=_run_suffix_transaction,
        map_result=_map_suffix_transaction,
        format_progress=_progress_suffix_transaction,
    ),
    8: LegacyCleaningStepSpec(
        step_no=8,
        run=_run_account_invalid,
        map_result=_map_account_invalid,
        format_progress=_progress_account_invalid,
    ),
    9: LegacyCleaningStepSpec(
        step_no=9,
        run=_run_suffix_account,
        map_result=_map_suffix_account,
        format_progress=_progress_suffix_account,
    ),
    10: LegacyCleaningStepSpec(
        step_no=10,
        run=_run_fill_account_info,
        map_result=_map_fill_account_info,
        format_progress=_progress_fill_account_info,
    ),
}


__all__ = ["ACCOUNT_LEGACY_STEP_SPECS"]
