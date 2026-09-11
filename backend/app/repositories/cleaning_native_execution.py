from __future__ import annotations

from dataclasses import dataclass
from typing import Callable, Mapping

from app.repositories import cleaning_native_segments
from app.repositories.cleaning_native_segments import NativeSegmentUpdate


UpdateBuilder = Callable[..., NativeSegmentUpdate]


@dataclass(frozen=True)
class NativeExecutionSpec:
    current_step: str
    error_label: str
    update_builder: UpdateBuilder
    commit_before_progress: bool = True
    tracks_clean_all_status: bool = False
    prepare_cleaning_after_native: bool = True

    def build_update(self, result: Mapping[str, int], *, elapsed: float) -> NativeSegmentUpdate:
        return self.update_builder(result, elapsed=elapsed)


CLEAN_ALL = NativeExecutionSpec(
    current_step="native clean-all",
    error_label="clean-all",
    update_builder=cleaning_native_segments.clean_all_update,
    tracks_clean_all_status=True,
    prepare_cleaning_after_native=False,
)

AMOUNT_BALANCE = NativeExecutionSpec(
    current_step="native step1 amount_balance",
    error_label="amount-balance",
    update_builder=cleaning_native_segments.amount_balance_update,
)

QUALITY_FLAGS = NativeExecutionSpec(
    current_step="native step2-4 quality_flags",
    error_label="quality-flags",
    update_builder=cleaning_native_segments.quality_flags_update,
    commit_before_progress=False,
)

DC_FLAG = NativeExecutionSpec(
    current_step="native step5 dc_flag",
    error_label="dc-flag",
    update_builder=cleaning_native_segments.dc_flag_update,
)

ACCOUNT_KEYS = NativeExecutionSpec(
    current_step="native step6-9 account_keys",
    error_label="account-keys",
    update_builder=cleaning_native_segments.account_keys_update,
)

ACCOUNT_INFO = NativeExecutionSpec(
    current_step="native step10 fill_account_info",
    error_label="account-info",
    update_builder=cleaning_native_segments.account_info_update,
)


__all__ = [
    "ACCOUNT_INFO",
    "ACCOUNT_KEYS",
    "AMOUNT_BALANCE",
    "CLEAN_ALL",
    "DC_FLAG",
    "NativeExecutionSpec",
    "QUALITY_FLAGS",
]
