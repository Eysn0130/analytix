from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Sequence, Tuple

from app.repositories import cleaning_native_runtime


FULL_CLEANING_STEPS: Tuple[int, ...] = tuple(range(1, 11))


@dataclass(frozen=True)
class NativeCleaningPlan:
    active_steps: Tuple[int, ...]
    clean_all: Tuple[str, ...] = ()
    amount_balance: Tuple[str, ...] = ()
    quality_flags: Tuple[str, ...] = ()
    dc_flag: Tuple[str, ...] = ()
    account_keys: Tuple[str, ...] = ()
    account_info: Tuple[str, ...] = ()

    @property
    def active_step_set(self) -> set[int]:
        return set(self.active_steps)


def _build_command(
    *,
    case_id: str,
    db_path: Path,
    command: str,
    txn_file_ids: Sequence[str],
    acc_file_ids: Sequence[str] = (),
) -> Tuple[str, ...]:
    return tuple(
        cleaning_native_runtime.build_native_cleaning_command(
            case_id=case_id,
            db_path=db_path,
            command=command,
            txn_file_ids=txn_file_ids,
            acc_file_ids=acc_file_ids,
        )
    )


def build_native_cleaning_plan(
    *,
    case_id: str,
    db_path: Path,
    active_steps: Sequence[int],
    txn_file_ids: Sequence[str],
    acc_file_ids: Sequence[str],
) -> NativeCleaningPlan:
    steps = tuple(int(step) for step in active_steps)
    active_step_set = set(steps)
    clean_all = (
        _build_command(
            case_id=case_id,
            db_path=db_path,
            command="clean-all",
            txn_file_ids=txn_file_ids,
            acc_file_ids=acc_file_ids,
        )
        if steps == FULL_CLEANING_STEPS
        else ()
    )
    if clean_all:
        return NativeCleaningPlan(active_steps=steps, clean_all=clean_all)

    return NativeCleaningPlan(
        active_steps=steps,
        amount_balance=(
            _build_command(
                case_id=case_id,
                db_path=db_path,
                command="amount-balance",
                txn_file_ids=txn_file_ids,
            )
            if 1 in active_step_set
            else ()
        ),
        quality_flags=(
            _build_command(
                case_id=case_id,
                db_path=db_path,
                command="quality-flags",
                txn_file_ids=txn_file_ids,
            )
            if {2, 3, 4}.issubset(active_step_set)
            else ()
        ),
        dc_flag=(
            _build_command(
                case_id=case_id,
                db_path=db_path,
                command="dc-flag",
                txn_file_ids=txn_file_ids,
            )
            if 5 in active_step_set
            else ()
        ),
        account_keys=(
            _build_command(
                case_id=case_id,
                db_path=db_path,
                command="account-keys",
                txn_file_ids=txn_file_ids,
                acc_file_ids=acc_file_ids,
            )
            if {6, 7, 8, 9}.issubset(active_step_set)
            else ()
        ),
        account_info=(
            _build_command(
                case_id=case_id,
                db_path=db_path,
                command="account-info",
                txn_file_ids=txn_file_ids,
                acc_file_ids=acc_file_ids,
            )
            if 10 in active_step_set
            else ()
        ),
    )


__all__ = [
    "FULL_CLEANING_STEPS",
    "NativeCleaningPlan",
    "build_native_cleaning_plan",
]
