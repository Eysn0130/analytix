from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Callable, Dict, Mapping, Optional, Sequence

from app.repositories import cleaning_native_results, cleaning_native_runtime
from app.repositories.cleaning_pipeline_models import NativeCleaningAction


ResultMapper = Callable[..., Dict[str, int]]


@dataclass(frozen=True)
class NativeInvocationSpec:
    command: str
    result_mapper: ResultMapper
    include_account_scope: bool = False


NATIVE_INVOCATION_SPECS: Dict[NativeCleaningAction, NativeInvocationSpec] = {
    NativeCleaningAction.CLEAN_ALL: NativeInvocationSpec(
        command="clean-all",
        result_mapper=cleaning_native_results.clean_all_result,
        include_account_scope=True,
    ),
    NativeCleaningAction.AMOUNT_BALANCE: NativeInvocationSpec(
        command="amount-balance",
        result_mapper=cleaning_native_results.amount_balance_result,
    ),
    NativeCleaningAction.QUALITY_FLAGS: NativeInvocationSpec(
        command="quality-flags",
        result_mapper=cleaning_native_results.quality_flags_result,
    ),
    NativeCleaningAction.DC_FLAG: NativeInvocationSpec(
        command="dc-flag",
        result_mapper=cleaning_native_results.dc_flag_result,
    ),
    NativeCleaningAction.ACCOUNT_KEYS: NativeInvocationSpec(
        command="account-keys",
        result_mapper=cleaning_native_results.account_keys_result,
        include_account_scope=True,
    ),
    NativeCleaningAction.ACCOUNT_INFO: NativeInvocationSpec(
        command="account-info",
        result_mapper=cleaning_native_results.account_info_result,
        include_account_scope=True,
    ),
}


@dataclass(frozen=True)
class CleaningNativeInvoker:
    executor: Any

    def run(
        self,
        action: NativeCleaningAction,
        argv: Sequence[str],
    ) -> Optional[Dict[str, int]]:
        try:
            spec = NATIVE_INVOCATION_SPECS[action]
        except KeyError as exc:
            raise ValueError(f"unsupported native cleaning action: {action}") from exc

        payload = cleaning_native_runtime.run_native_cleaning_command(
            case_id=self.executor.case_id,
            db_path=self.executor.storage.case_db(self.executor.case_id),
            command=spec.command,
            txn_file_ids=self.executor.scope_state.txn_scope_file_ids,
            acc_file_ids=self.executor.scope_state.acc_scope_file_ids if spec.include_account_scope else (),
            argv=argv,
        )
        if payload is None:
            return None
        return spec.result_mapper(payload, expected_case_id=self.executor.case_id)


__all__ = [
    "CleaningNativeInvoker",
    "NATIVE_INVOCATION_SPECS",
    "NativeInvocationSpec",
]
