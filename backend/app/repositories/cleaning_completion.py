from __future__ import annotations

import time
from datetime import datetime
from typing import Any, Dict, Sequence

from app.core.fc_import_privacy_delta import append_privacy_projection_delta_for_case_rows
from app.core.import_count_semantics import known_public_import_count
from app.repositories import (
    cleaning_account_state,
    cleaning_duckdb_meta,
    cleaning_log_store,
    cleaning_status_store,
)
from app.repositories.analysis_revision import bump_stats_flow_source_revision
from app.repositories.cleaning_run_state import CleaningRunState


def complete_cleaning_run(
    executor: Any,
    *,
    con: Any,
    cur: Any,
    run_state: CleaningRunState,
    overall_start: float,
    import_marker: str,
    scope_rows: int,
    scope_ids: Sequence[str],
    active_steps: Sequence[int],
) -> Dict[str, Any]:
    completion_start = time.perf_counter()
    if getattr(run_state, "native_completion_state_applied", False):
        _ensure_native_completion_timings(run_state)
    else:
        cleaned_at = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        txn_where = executor.scope_state.txn_where_base or executor.scope_state.txn_where
        txn_params = list(executor.scope_state.txn_where_base_params or executor.scope_state.txn_params)
        t0 = time.perf_counter()
        cur.execute(
            f"UPDATE fc_transaction_norm SET cleaned_at=? WHERE {txn_where}",
            (cleaned_at, *txn_params),
        )
        run_state.set_phase_timing("completion_mark_txn", time.perf_counter() - t0)
        t0 = time.perf_counter()
        append_privacy_projection_delta_for_case_rows(
            con,
            case_id=executor.case_id,
            op="upsert",
            table="fc_transaction_norm",
            where_clause=txn_where,
            params=txn_params,
            source="cleaning:run",
        )
        run_state.set_phase_timing("completion_privacy_projection", time.perf_counter() - t0)
        t0 = time.perf_counter()
        if cleaning_duckdb_meta.table_exists(cur, "fc_account_norm"):
            cur.execute(
                f"UPDATE fc_account_norm SET cleaned_at=? WHERE {executor.scope_state.acc_where}",
                (cleaned_at, *executor.scope_state.acc_params),
            )
        run_state.set_phase_timing("completion_mark_account", time.perf_counter() - t0)
        t0 = time.perf_counter()
        cleaning_account_state.update_acct_state(executor, cur, cleaned_at)
        run_state.set_phase_timing("completion_account_state", time.perf_counter() - t0)

    duration_ms = int((time.perf_counter() - overall_start) * 1000)
    summary = run_state.build_summary(
        scope_rows=scope_rows,
        scope_ids=list(scope_ids),
        active_steps=list(active_steps),
        duration_ms=duration_ms,
    )
    t0 = time.perf_counter()
    cleaning_log_store.record_cleaning(
        executor,
        cur,
        import_marker,
        summary,
        file_ids=list(scope_ids),
        duration_ms=duration_ms,
        scope_rows=scope_rows,
    )
    run_state.set_phase_timing("completion_log", time.perf_counter() - t0)
    finished_at = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    rows_affected_total = known_public_import_count(run_state.rows_affected_total)
    if rows_affected_total is None:
        raise RuntimeError("cleaning_rows_affected_unknown")
    t0 = time.perf_counter()
    cleaning_status_store.update_clean_status(
        executor,
        cur,
        list(scope_ids),
        "done",
        finished_at=finished_at,
        error="",
        rows_affected=rows_affected_total,
    )
    run_state.set_phase_timing("completion_status", time.perf_counter() - t0)
    if rows_affected_total > 0:
        t0 = time.perf_counter()
        try:
            bump_stats_flow_source_revision(con, reason="cleaning:run")
        except Exception:
            pass
        run_state.set_phase_timing("completion_revision", time.perf_counter() - t0)
    t0 = time.perf_counter()
    con.commit()
    run_state.set_phase_timing("completion_commit", time.perf_counter() - t0)
    run_state.set_phase_timing("completion", time.perf_counter() - completion_start)
    summary["phase_timings_ms"] = dict(run_state.phase_timings_ms)

    executor.execution_context.emit_final_progress("cleaning completed")
    executor.execution_context.emit_log("summary", f"cleaning completed in {duration_ms}ms")
    return summary


def _ensure_native_completion_timings(run_state: CleaningRunState) -> None:
    for key in (
        "completion_mark_txn",
        "completion_privacy_projection",
        "completion_mark_account",
        "completion_account_state",
    ):
        if key not in run_state.phase_timings_ms:
            run_state.set_phase_timing(key, 0.0)


__all__ = ["complete_cleaning_run"]
