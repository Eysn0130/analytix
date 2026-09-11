from __future__ import annotations

import time
from typing import AbstractSet, Any, Callable, Dict, Optional, Sequence

from app.repositories import cleaning_legacy_dispatch
from app.repositories.cleaning_run_state import CleaningRunState
from app.repositories.cleaning_sql_counter import require_non_negative_count


AdvanceProgress = Callable[[int, str], None]
SetCurrentStep = Callable[[str], None]

LEGACY_CLEANING_DISABLED_MESSAGE = (
    "legacy python cleaning is disabled on the API/runtime cleaning path; "
    "run the explicit maintenance/test path to compare legacy Python SQL semantics"
)


class CleaningLegacyRunner:
    def __init__(
        self,
        *,
        executor: Any,
        native_runner: Any,
        run_state: CleaningRunState,
        scope_rows: int,
        active_steps: AbstractSet[int],
        advance_progress: AdvanceProgress,
        set_current_step: SetCurrentStep,
    ) -> None:
        self.executor = executor
        self.native_runner = native_runner
        self.run_state = run_state
        self.scope_rows = require_non_negative_count(
            scope_rows,
            code="legacy_cleaning_scope_count_unavailable",
        )
        self.active_steps = active_steps
        self._advance_progress = advance_progress
        self._set_current_step = set_current_step

    def dispatch_if_needed(
        self,
        *,
        step_no: int,
        native_clean_all: object,
        native_segment_result: Optional[Dict[str, int]],
        native_command: Sequence[str],
    ) -> None:
        if cleaning_legacy_dispatch.should_dispatch_legacy_step(
            native_clean_all=native_clean_all,
            native_segment_result=native_segment_result,
            active_steps=self.active_steps,
            step_no=step_no,
            native_command=native_command,
        ):
            self.dispatch(step_no, native_command)

    def dispatch(self, step_no: int, native_command: Sequence[str]) -> None:
        self._enter_legacy_python_cleaning_step(step_no, native_command)
        self._set_current_step(cleaning_legacy_dispatch.legacy_step_label(step_no))
        t0 = time.perf_counter()
        result = cleaning_legacy_dispatch.dispatch_legacy_python_step(
            self.executor,
            cur=self.native_runner.cur,
            con=self.native_runner.con,
            step_no=step_no,
            total_rows=self.scope_rows,
            native_command=native_command,
            advance_progress=self._advance_progress,
        )
        self.run_state.add_phase_timing("legacy", time.perf_counter() - t0)
        self.run_state.apply_legacy_result(step_no, result)

    def _enter_legacy_python_cleaning_step(self, step_no: int, native_command: Sequence[str]) -> None:
        if native_command:
            raise RuntimeError(
                f"legacy python cleaning step{step_no} blocked because native command is available"
            )
        if not self.executor.execution_context.legacy_python_cleaning_allowed():
            raise RuntimeError(f"{LEGACY_CLEANING_DISABLED_MESSAGE}; step={step_no}")
        self.run_state.enter_legacy_step(step_no)


__all__ = [
    "CleaningLegacyRunner",
    "LEGACY_CLEANING_DISABLED_MESSAGE",
]
