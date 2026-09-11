from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime
from typing import Any, List

from app.repositories import (
    cleaning_connection_setup,
    cleaning_duckdb_meta,
    cleaning_import_log_reader,
    cleaning_native_plan,
    cleaning_schema_setup,
    cleaning_scope,
    cleaning_status_store,
)


@dataclass
class CleaningRunSetupProgress:
    con: Any = None
    scope_ids: List[str] = field(default_factory=list)


@dataclass(frozen=True)
class CleaningRunSetup:
    con: Any
    cur: Any
    import_marker: str
    scope_rows: int
    scope_ids: List[str]
    active_steps: List[int]
    native_plan: cleaning_native_plan.NativeCleaningPlan
    active_step_set: set[int]
    step_total: int


def prepare_cleaning_run(executor: Any, *, progress: CleaningRunSetupProgress) -> CleaningRunSetup:
    executor.storage.ensure_funds_tables(executor.case_id)
    con = executor.storage.open_case_engine(executor.case_id)
    progress.con = con
    cleaning_connection_setup.configure_cleaning_connection(executor.storage, executor.case_id, con)
    cur = con.cursor()

    if not cleaning_duckdb_meta.table_exists(cur, "fc_transaction_norm"):
        raise RuntimeError("fc_transaction_norm not found")

    cleaning_schema_setup.ensure_cleaning_schema(cur)
    import_marker = cleaning_import_log_reader.get_last_import_at(cur, executor.case_id)
    cleaning_scope.rebuild_cleaning_scope(executor, cur)

    scope_rows = _read_scope_row_count(cur)
    if scope_rows == 0:
        raise RuntimeError("no transaction rows in cleaning scope")

    scope_ids = list(executor.scope_state.scope_file_ids)
    progress.scope_ids = scope_ids
    started_at = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    cleaning_status_store.update_clean_status(
        executor,
        cur,
        scope_ids,
        "running",
        started_at=started_at,
        error="",
        rows_affected=None,
    )
    con.commit()

    active_steps = list(executor.steps or list(range(1, 11)))
    native_plan = cleaning_native_plan.build_native_cleaning_plan(
        case_id=executor.case_id,
        db_path=executor.storage.case_db(executor.case_id),
        active_steps=active_steps,
        txn_file_ids=executor.scope_state.txn_scope_file_ids,
        acc_file_ids=executor.scope_state.acc_scope_file_ids,
    )
    return CleaningRunSetup(
        con=con,
        cur=cur,
        import_marker=import_marker,
        scope_rows=scope_rows,
        scope_ids=scope_ids,
        active_steps=active_steps,
        native_plan=native_plan,
        active_step_set=native_plan.active_step_set,
        step_total=len(active_steps),
    )


def _read_scope_row_count(cur: Any) -> int:
    try:
        cur.execute("SELECT COUNT(1) FROM txn_scope")
        row = cur.fetchone()
    except Exception:
        raise RuntimeError("cleaning_scope_count_unavailable") from None

    if not isinstance(row, (list, tuple)) or not row:
        raise RuntimeError("cleaning_scope_count_unavailable")
    value = row[0]
    if type(value) is not int or value < 0:
        raise RuntimeError("cleaning_scope_count_unavailable")
    return value


__all__ = [
    "CleaningRunSetup",
    "CleaningRunSetupProgress",
    "prepare_cleaning_run",
]
