from __future__ import annotations

import time
from types import SimpleNamespace
from typing import Any, Dict, List

from app.repositories import (
    cleaning_completion,
    cleaning_failure_status,
    cleaning_legacy_runner,
    cleaning_run_setup,
    cleaning_run_state,
)
from app.repositories.cleaning_execution_context import CleaningCancelled


def run_legacy_python_cleaning(executor: Any) -> Dict[str, Any]:
    con = None
    overall_start = time.perf_counter()
    scope_ids: List[str] = []
    current_step = "init"
    run_state = cleaning_run_state.CleaningRunState()
    setup_progress = cleaning_run_setup.CleaningRunSetupProgress()
    setup_ready = False
    legacy_context = None

    try:
        setup = cleaning_run_setup.prepare_cleaning_run(executor, progress=setup_progress)
        run_state.set_phase_timing("setup", time.perf_counter() - overall_start)
        setup_ready = True
        con = setup.con
        cur = setup.cur
        import_marker = setup.import_marker
        scope_rows = setup.scope_rows
        scope_ids = setup.scope_ids
        active_steps = setup.active_steps
        active_step_set = setup.active_step_set
        step_total = setup.step_total
        step_cursor = 0
        run_state.legacy_python_cleaning_reason = "maintenance_python_sql"

        def _advance_progress(step_no: int, message: str) -> None:
            nonlocal step_cursor
            step_cursor += 1
            executor.execution_context.emit_progress(step_cursor, step_total, message)
            executor.execution_context.emit_log(f"step{step_no}", message)

        def _set_current_step(step: str) -> None:
            nonlocal current_step
            current_step = step

        legacy_context = SimpleNamespace(con=con, cur=cur)
        legacy_runner = cleaning_legacy_runner.CleaningLegacyRunner(
            executor=executor,
            native_runner=legacy_context,
            run_state=run_state,
            scope_rows=scope_rows,
            active_steps=active_step_set,
            advance_progress=_advance_progress,
            set_current_step=_set_current_step,
        )

        pipeline_start = time.perf_counter()
        for step_no in active_steps:
            legacy_runner.dispatch(int(step_no), ())
        run_state.set_phase_timing("pipeline", time.perf_counter() - pipeline_start)

        return cleaning_completion.complete_cleaning_run(
            executor,
            con=legacy_context.con,
            cur=legacy_context.cur,
            run_state=run_state,
            overall_start=overall_start,
            import_marker=import_marker,
            scope_rows=scope_rows,
            scope_ids=scope_ids,
            active_steps=active_steps,
        )
    except CleaningCancelled:
        status_con = (
            legacy_context.con
            if legacy_context is not None
            else con if con is not None else (None if setup_ready else setup_progress.con)
        )
        status_scope_ids = scope_ids or ([] if setup_ready else setup_progress.scope_ids)
        cleaning_failure_status.mark_cleaning_cancelled(
            executor,
            con=status_con,
            scope_ids=status_scope_ids,
            rows_affected=None,
        )
        raise
    except Exception as exc:
        failure = cleaning_failure_status.build_cleaning_failure(current_step=current_step, exc=exc)
        status_con = (
            legacy_context.con
            if legacy_context is not None
            else con if con is not None else (None if setup_ready else setup_progress.con)
        )
        status_scope_ids = scope_ids or ([] if setup_ready else setup_progress.scope_ids)
        cleaning_failure_status.mark_cleaning_failed(
            executor,
            con=status_con,
            scope_ids=status_scope_ids,
            error=failure.detail,
            rows_affected=None,
        )
        raise RuntimeError(failure.base_message) from exc
    finally:
        close_con = (
            legacy_context.con
            if legacy_context is not None
            else con if con is not None else (None if setup_ready else setup_progress.con)
        )
        if close_con is not None:
            try:
                close_con.close()
            except Exception:
                pass


__all__ = ["run_legacy_python_cleaning"]
