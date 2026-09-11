from __future__ import annotations

import logging
import time
from typing import Any, Callable, Dict, Optional, Sequence

from app.core.safe_observability import log_closed_diagnostic

from app.repositories import (
    cleaning_connection_setup,
    cleaning_native_execution,
    cleaning_schema_setup,
    cleaning_scope,
)
from app.repositories.cleaning_native_segments import NativeSegmentUpdate
from app.repositories.cleaning_run_state import CleaningRunState


AdvanceProgress = Callable[[int, str], None]
SetCurrentStep = Callable[[str], None]
NativeCommandRunner = Callable[[Sequence[str]], Optional[Dict[str, int]]]


class CleaningNativeRunner:
    def __init__(
        self,
        *,
        executor: Any,
        con: Any,
        cur: Any,
        run_state: CleaningRunState,
        advance_progress: AdvanceProgress,
        set_current_step: SetCurrentStep,
    ) -> None:
        self.executor = executor
        self.con = con
        self.cur = cur
        self.run_state = run_state
        self._advance_progress = advance_progress
        self._set_current_step = set_current_step

    def execute(
        self,
        spec: cleaning_native_execution.NativeExecutionSpec,
        command: Sequence[str],
        runner: NativeCommandRunner,
    ) -> Optional[Dict[str, int]]:
        if not command:
            return None

        self._set_current_step(spec.current_step)
        if spec.tracks_clean_all_status:
            self.run_state.native_clean_all_attempted = True
        t0 = time.perf_counter()
        try:
            result = self._run_native_with_reopened_connection(
                lambda: runner(command),
                rebuild_scope=not spec.tracks_clean_all_status,
                prepare_cleaning=spec.prepare_cleaning_after_native,
            )
        except Exception as exc:
            log_closed_diagnostic(
                logging.getLogger("analytix.cleaning"),
                logging.ERROR,
                topic="cleaning_job",
                code="failed",
            )
            if spec.tracks_clean_all_status:
                self.run_state.native_clean_all_failed = True
            raise RuntimeError(f"native {spec.error_label} failed") from exc
        elapsed = time.perf_counter() - t0
        self.run_state.add_phase_timing("native", elapsed)

        if result is not None:
            update = spec.build_update(result, elapsed=elapsed)
            if spec.commit_before_progress:
                self.con.commit()
                self._apply_native_segment_update(update)
            else:
                self._apply_native_segment_update(update)
                self.con.commit()
        return result

    def _run_native_with_reopened_connection(
        self,
        runner: Callable[[], Optional[Dict[str, int]]],
        *,
        rebuild_scope: bool = True,
        prepare_cleaning: bool = True,
    ) -> Optional[Dict[str, int]]:
        close_started = time.perf_counter()
        self.con.commit()
        self.con.close()
        self.con = None
        self.run_state.add_phase_timing(
            "native_connection_close",
            time.perf_counter() - close_started,
        )
        try:
            return runner()
        finally:
            reopen_started = time.perf_counter()
            self._reopen_after_native_run(
                rebuild_scope=rebuild_scope,
                prepare_cleaning=prepare_cleaning,
            )
            self.run_state.add_phase_timing(
                "native_reopen",
                time.perf_counter() - reopen_started,
            )

    def _reopen_after_native_run(
        self,
        *,
        rebuild_scope: bool = True,
        prepare_cleaning: bool = True,
    ) -> None:
        if self.con is not None:
            return
        self.con = self.executor.storage.open_case_engine(self.executor.case_id)
        if prepare_cleaning:
            cleaning_connection_setup.configure_cleaning_connection(self.executor.storage, self.executor.case_id, self.con)
        self.cur = self.con.cursor()
        if prepare_cleaning:
            cleaning_schema_setup.ensure_cleaning_schema(self.cur)
        if prepare_cleaning and rebuild_scope:
            cleaning_scope.rebuild_cleaning_scope(self.executor, self.cur)

    def _apply_native_segment_update(
        self,
        update: NativeSegmentUpdate,
    ) -> None:
        self.run_state.apply_native_segment(update)
        for step_no, message in update.progress:
            self._advance_progress(step_no, message)


__all__ = ["CleaningNativeRunner"]
