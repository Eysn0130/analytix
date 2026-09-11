from __future__ import annotations

from typing import AbstractSet, Any, Callable, Dict, Sequence

from app.repositories import cleaning_legacy_sql


AdvanceProgress = Callable[[int, str], None]

FRONT_LEGACY_STEPS = frozenset({1, 2, 3, 4, 5})
ACCOUNT_LEGACY_STEPS = frozenset({6, 7, 8, 9, 10})
LEGACY_STEP_LABELS = {
    1: "step1 amount_balance",
    2: "step2 invalid",
    3: "step3 duplicate",
    4: "step4 failed_reversal",
    5: "step5 dc_flag",
    6: "step6 fill_card",
    7: "step7 suffix_transaction",
    8: "step8 account_invalid",
    9: "step9 suffix_account",
    10: "step10 fill_account_info",
}


def legacy_step_kind(step_no: int) -> str:
    step = int(step_no)
    if step in FRONT_LEGACY_STEPS:
        return "front"
    if step in ACCOUNT_LEGACY_STEPS:
        return "account"
    raise RuntimeError(f"legacy python cleaning does not own step{step}")


def legacy_step_label(step_no: int) -> str:
    step = int(step_no)
    if step in LEGACY_STEP_LABELS:
        return LEGACY_STEP_LABELS[step]
    raise RuntimeError(f"legacy python cleaning does not own step{step}")


def should_dispatch_legacy_step(
    *,
    native_clean_all: object,
    native_segment_result: object,
    active_steps: AbstractSet[int],
    step_no: int,
    native_command: Sequence[str],
) -> bool:
    return (
        native_clean_all is None
        and native_segment_result is None
        and int(step_no) in active_steps
        and not native_command
    )


def dispatch_legacy_python_step(
    executor: Any,
    *,
    cur: Any,
    con: Any,
    step_no: int,
    total_rows: int,
    native_command: Sequence[str],
    advance_progress: AdvanceProgress,
) -> Dict[str, Any]:
    kind = legacy_step_kind(step_no)
    if kind == "front":
        return cleaning_legacy_sql.run_legacy_python_front_step(
            executor,
            cur=cur,
            con=con,
            step_no=step_no,
            total_rows=total_rows,
            native_command=native_command,
            advance_progress=advance_progress,
        )
    return cleaning_legacy_sql.run_legacy_python_account_step(
        executor,
        cur=cur,
        con=con,
        step_no=step_no,
        native_command=native_command,
        advance_progress=advance_progress,
    )


__all__ = [
    "ACCOUNT_LEGACY_STEPS",
    "FRONT_LEGACY_STEPS",
    "LEGACY_STEP_LABELS",
    "dispatch_legacy_python_step",
    "legacy_step_kind",
    "legacy_step_label",
    "should_dispatch_legacy_step",
]
