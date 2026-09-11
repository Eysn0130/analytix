from __future__ import annotations

import json
import time
from typing import Any, Callable, Dict, List, Optional, Sequence

from app.core.db_engine import DuckDBEngine
from app.core.fc_import_norm_rebuild import rebuild_norm_for_case
from app.core.fc_import_privacy_delta import append_privacy_projection_delta_for_case_rows
from app.core.storage import CaseStorage
from app.domain.ordinary_diagnostic_projection import project_cleaning_history_item
from app.repositories.analysis_revision import bump_stats_flow_source_revision
from app.repositories import (
    cleaning_completion,
    cleaning_connection_setup,
    cleaning_duckdb_meta,
    cleaning_failure_status,
    cleaning_legacy_maintenance,
    cleaning_native_runner,
    cleaning_native_runtime,
    cleaning_pipeline,
    cleaning_run_state,
    cleaning_run_setup,
    cleaning_schema_setup,
)
from app.repositories.cleaning_execution_context import CleaningCancelled, CleaningExecutionContext
from app.repositories.cleaning_scope_state import CleaningScopeState
from app.repositories.cleaning_step_catalog import CLEANING_STEP_DEFINITIONS, CleaningStepDefinition, get_cleaning_step_definition
from app.repositories.txn_daily_aggregate import TxnDailyAggregateStore

_STEP_QUERY_RETRYABLE_CONFLICT_MARKERS = (
    "unique file handle conflict",
    "already attached by database",
    "same database file with a different configuration",
    "could not set lock on file",
    "conflicting lock is held",
    "transactioncontext error",
    "conflict on update",
    "cannot open file",
    "being used by another process",
    "another process is using",
    "另一个程序正在使用此文件",
    "进程无法访问",
)


def _is_retryable_step_query_conflict(exc: Exception) -> bool:
    try:
        message = str(exc or "").lower()
    except Exception:
        return False
    return any(marker in message for marker in _STEP_QUERY_RETRYABLE_CONFLICT_MARKERS)


class CleaningStepSourceUnavailableError(RuntimeError):
    def __init__(self) -> None:
        super().__init__("cleaning_step_source_unavailable")


class CleaningExecutor:
    """Backend executor for the persisted 10-step cleaning pipeline."""

    def __init__(
        self,
        storage: CaseStorage,
        case_id: str,
        *,
        file_ids: Optional[List[str]] = None,
        steps: Optional[Sequence[int]] = None,
        cancel_check: Optional[Callable[[], bool]] = None,
        progress_cb: Optional[Callable[[int, str, Optional[int]], None]] = None,
        log_cb: Optional[Callable[[str, str], None]] = None,
        allow_legacy_python_cleaning: bool = False,
    ) -> None:
        self.storage = storage
        self.case_id = case_id
        self.file_ids = [f for f in (file_ids or []) if f]
        self.steps = sorted({int(s) for s in (steps or []) if 1 <= int(s) <= 10})
        self.execution_context = CleaningExecutionContext(
            cancel_check=cancel_check,
            progress_cb=progress_cb,
            log_cb=log_cb,
            allow_legacy_python_cleaning=bool(allow_legacy_python_cleaning),
        )
        self.scope_state = CleaningScopeState()

    def run(self) -> Dict[str, Any]:
        con = None
        overall_start = time.perf_counter()
        scope_ids: List[str] = []
        current_step = "init"
        run_state = cleaning_run_state.CleaningRunState()
        setup_progress = cleaning_run_setup.CleaningRunSetupProgress()
        setup_ready = False
        native_runner = None

        try:
            setup = cleaning_run_setup.prepare_cleaning_run(self, progress=setup_progress)
            run_state.set_phase_timing("setup", time.perf_counter() - overall_start)
            setup_ready = True
            con = setup.con
            cur = setup.cur
            import_marker = setup.import_marker
            scope_rows = setup.scope_rows
            scope_ids = setup.scope_ids
            active_steps = setup.active_steps
            native_plan = setup.native_plan
            step_total = setup.step_total
            step_cursor = 0

            def _advance_progress(step_no: int, message: str) -> None:
                nonlocal step_cursor
                step_cursor += 1
                self.execution_context.emit_progress(step_cursor, step_total, message)
                self.execution_context.emit_log(f"step{step_no}", message)

            def _set_current_step(step: str) -> None:
                nonlocal current_step
                current_step = step

            native_runner = cleaning_native_runner.CleaningNativeRunner(
                executor=self,
                con=con,
                cur=cur,
                run_state=run_state,
                advance_progress=_advance_progress,
                set_current_step=_set_current_step,
            )
            pipeline_start = time.perf_counter()
            cleaning_pipeline.execute_cleaning_pipeline(
                executor=self,
                native_plan=native_plan,
                native_runner=native_runner,
            )
            run_state.set_phase_timing("pipeline", time.perf_counter() - pipeline_start)

            return cleaning_completion.complete_cleaning_run(
                self,
                con=native_runner.con,
                cur=native_runner.cur,
                run_state=run_state,
                overall_start=overall_start,
                import_marker=import_marker,
                scope_rows=scope_rows,
                scope_ids=scope_ids,
                active_steps=active_steps,
            )
        except CleaningCancelled:
            status_con = (
                native_runner.con
                if native_runner is not None
                else con if con is not None else (None if setup_ready else setup_progress.con)
            )
            status_scope_ids = scope_ids or ([] if setup_ready else setup_progress.scope_ids)
            cleaning_failure_status.mark_cleaning_cancelled(
                self,
                con=status_con,
                scope_ids=status_scope_ids,
                rows_affected=None,
            )
            raise
        except Exception as exc:
            failure = cleaning_failure_status.build_cleaning_failure(current_step=current_step, exc=exc)
            status_con = (
                native_runner.con
                if native_runner is not None
                else con if con is not None else (None if setup_ready else setup_progress.con)
            )
            status_scope_ids = scope_ids or ([] if setup_ready else setup_progress.scope_ids)
            cleaning_failure_status.mark_cleaning_failed(
                self,
                con=status_con,
                scope_ids=status_scope_ids,
                error=failure.detail,
                rows_affected=None,
            )
            raise RuntimeError(failure.base_message) from exc
        finally:
            close_con = (
                native_runner.con
                if native_runner is not None
                else con if con is not None else (None if setup_ready else setup_progress.con)
            )
            if close_con is not None:
                try:
                    close_con.close()
                except Exception:
                    pass


class CleaningRepository:
    def __init__(self) -> None:
        self._storage = CaseStorage()
        self._daily_agg = TxnDailyAggregateStore(self._storage)

    @property
    def storage(self) -> CaseStorage:
        return self._storage

    def case_exists(self, case_id: str) -> bool:
        return self._storage.get_case(case_id) is not None

    def get_cleaning_runtime_health(self) -> Dict[str, Any]:
        return cleaning_native_runtime.native_cleaning_binary_health()

    def run_cleaning(
        self,
        *,
        case_id: str,
        file_ids: Optional[List[str]],
        steps: Optional[Sequence[int]],
        cancel_check: Optional[Callable[[], bool]] = None,
        progress_cb: Optional[Callable[[int, str, Optional[int]], None]] = None,
        log_cb: Optional[Callable[[str, str], None]] = None,
    ) -> Dict[str, Any]:
        worker = CleaningExecutor(
            self._storage,
            case_id,
            file_ids=file_ids,
            steps=steps,
            cancel_check=cancel_check,
            progress_cb=progress_cb,
            log_cb=log_cb,
        )
        return worker.run()

    def run_legacy_python_cleaning_for_maintenance(
        self,
        *,
        case_id: str,
        file_ids: Optional[List[str]],
        steps: Optional[Sequence[int]],
        cancel_check: Optional[Callable[[], bool]] = None,
        progress_cb: Optional[Callable[[int, str, Optional[int]], None]] = None,
        log_cb: Optional[Callable[[str, str], None]] = None,
    ) -> Dict[str, Any]:
        worker = CleaningExecutor(
            self._storage,
            case_id,
            file_ids=file_ids,
            steps=steps,
            cancel_check=cancel_check,
            progress_cb=progress_cb,
            log_cb=log_cb,
            allow_legacy_python_cleaning=True,
        )
        return cleaning_legacy_maintenance.run_legacy_python_cleaning(worker)

    def refresh_analysis_outputs_after_cleaning(self, case_id: str, *, reason: str) -> Dict[str, Any]:
        if not case_id:
            raise ValueError("case_id required")

        con = self._storage.open_case_engine(case_id)
        try:
            bump_stats_flow_source_revision(con, reason=reason)
            self._storage.compute_case_stats(case_id, engine=con)
        finally:
            try:
                con.close()
            except Exception:
                pass

        analysis_timing: Dict[str, Any] = {}
        materialized = self._daily_agg.ensure_materialized(
            case_id,
            force=True,
            profile_cb=lambda profile: analysis_timing.update(profile or {}),
        )
        if "ok" not in analysis_timing:
            analysis_timing["ok"] = bool(materialized)
        return analysis_timing

    def reset_clean_state(self, case_id: str) -> Dict[str, Any]:
        if not case_id:
            raise ValueError("case_id required")

        self._storage.ensure_funds_tables(case_id)
        con = self._storage.open_case_engine(case_id)
        executor = CleaningExecutor(self._storage, case_id)
        transaction_started = False
        try:
            cleaning_connection_setup.configure_cleaning_connection(self._storage, case_id, con)
            cur = con.cursor()
            if not cleaning_duckdb_meta.table_exists(cur, "fc_transaction_norm"):
                raise RuntimeError("fc_transaction_norm not found")

            con.execute("BEGIN TRANSACTION")
            transaction_started = True
            cleaning_schema_setup.ensure_cleaning_schema(cur)

            if not cleaning_duckdb_meta.table_exists(cur, "import_file_log"):
                raise RuntimeError("cleaning_reset_rebuild_source_unavailable")
            cur.execute(
                "SELECT file_id, kind FROM import_file_log "
                "WHERE case_id=? AND kind IN ('fc_transaction','fc_account')",
                (case_id,),
            )
            raw_rows = cur.fetchall()
            if not isinstance(raw_rows, list):
                raise RuntimeError("cleaning_reset_rebuild_source_unavailable")
            rows: List[tuple[str, str]] = []
            for row in raw_rows:
                if not isinstance(row, (list, tuple)) or len(row) < 2:
                    raise RuntimeError("cleaning_reset_rebuild_source_unavailable")
                file_id = row[0]
                kind = row[1]
                if type(file_id) is not str or not file_id.strip():
                    raise RuntimeError("cleaning_reset_rebuild_source_unavailable")
                if kind not in {"fc_transaction", "fc_account"}:
                    raise RuntimeError("cleaning_reset_rebuild_source_unavailable")
                rows.append((file_id, kind))
            if not rows:
                raise RuntimeError("cleaning_reset_rebuild_source_unavailable")

            rebuild_kinds = sorted({kind for _, kind in rows})
            for kind in rebuild_kinds:
                rebuilt_rows = rebuild_norm_for_case(con, case_id=case_id, kind=kind)
                self._require_non_negative_count(
                    rebuilt_rows,
                    code="cleaning_reset_rebuild_count_unavailable",
                )
            rebuilt_files = len(rows)

            if cleaning_duckdb_meta.table_exists(cur, "cleaning_log"):
                cur.execute("DELETE FROM cleaning_log WHERE case_id=?", (case_id,))
            if cleaning_duckdb_meta.table_exists(cur, "cleaning_log_detail"):
                cur.execute("DELETE FROM cleaning_log_detail WHERE case_id=?", (case_id,))

            if cleaning_duckdb_meta.table_exists(cur, "import_file_log"):
                fields: List[str] = []
                if cleaning_duckdb_meta.column_exists(cur, "import_file_log", "cleaned_status"):
                    fields.append("cleaned_status=NULL")
                if cleaning_duckdb_meta.column_exists(cur, "import_file_log", "cleaned_started_at"):
                    fields.append("cleaned_started_at=NULL")
                if cleaning_duckdb_meta.column_exists(cur, "import_file_log", "cleaned_finished_at"):
                    fields.append("cleaned_finished_at=NULL")
                if cleaning_duckdb_meta.column_exists(cur, "import_file_log", "cleaned_error"):
                    fields.append("cleaned_error=NULL")
                if cleaning_duckdb_meta.column_exists(cur, "import_file_log", "cleaned_rows_affected"):
                    fields.append("cleaned_rows_affected=NULL")
                if cleaning_duckdb_meta.column_exists(cur, "import_file_log", "cleaning_counts_version"):
                    fields.append("cleaning_counts_version=NULL")
                if fields:
                    cur.execute(
                        f"UPDATE import_file_log SET {', '.join(fields)} "
                        "WHERE case_id=? AND kind LIKE 'fc_%'",
                        (case_id,),
                    )

            privacy_delta_rows = append_privacy_projection_delta_for_case_rows(
                con,
                case_id=case_id,
                op="upsert",
                table="fc_transaction_norm",
                where_clause="case_id=?",
                params=[case_id],
                source="cleaning:reset",
            )
            self._require_non_negative_count(
                privacy_delta_rows,
                code="cleaning_reset_privacy_delta_count_unavailable",
            )
            con.execute("COMMIT")
            transaction_started = False
            return {
                "case_id": case_id,
                "rebuilt": True,
                "rebuilt_files": rebuilt_files,
            }
        except BaseException:
            if transaction_started:
                try:
                    con.execute("ROLLBACK")
                except Exception:
                    pass
            raise
        finally:
            try:
                con.close()
            except Exception:
                pass

    @staticmethod
    def _require_non_negative_count(value: Any, *, code: str) -> int:
        if type(value) is not int or value < 0:
            raise RuntimeError(code)
        return value

    def list_step_summaries(self, case_id: str) -> List[Dict[str, Any]]:
        if not case_id:
            raise ValueError("case_id required")

        try:
            con = self._open_step_query_connection(case_id)
        except Exception:
            raise CleaningStepSourceUnavailableError() from None
        try:
            cur = con.cursor()
            items: List[Dict[str, Any]] = []
            for definition in CLEANING_STEP_DEFINITIONS:
                total = self._fetch_step_count(cur, case_id, definition)
                items.append(
                    {
                        "step": definition.step,
                        "key": definition.key,
                        "title": definition.title,
                        "kind": definition.kind,
                        "description": definition.description,
                        "affected_rows": total,
                    }
                )
            return items
        except CleaningStepSourceUnavailableError:
            raise
        except Exception:
            raise CleaningStepSourceUnavailableError() from None
        finally:
            try:
                con.close()
            except Exception:
                pass

    def list_cleaning_history(self, case_id: str, *, limit: int = 50) -> List[Dict[str, Any]]:
        if not case_id:
            raise ValueError("case_id required")

        con = self._open_step_query_connection(case_id)
        try:
            cur = con.cursor()
            executor = CleaningExecutor(self._storage, case_id)
            if not cleaning_duckdb_meta.table_exists(cur, "cleaning_log"):
                return []

            cur.execute(
                """
                SELECT MIN(id) AS first_id,
                       MAX(id) AS last_id,
                       MAX(COALESCE(run_ref, '')) AS run_ref,
                       cleaned_at,
                       MIN(import_at) AS import_at,
                       MAX(duration_ms) AS duration_ms,
                       MAX(scope_rows) AS scope_rows,
                       COUNT(DISTINCT COALESCE(NULLIF(file_id, ''), '__all__')) AS file_count,
                       summary
                FROM cleaning_log
                WHERE case_id=?
                GROUP BY COALESCE(NULLIF(run_ref, ''), ''), cleaned_at, summary
                ORDER BY cleaned_at DESC
                LIMIT ?
                """,
                (case_id, max(1, int(limit or 50))),
            )
            rows = cur.fetchall()
            items: List[Dict[str, Any]] = []
            for row in rows:
                items.append(
                    project_cleaning_history_item(
                        case_id=case_id,
                        row={
                            "first_id": row[0],
                            "last_id": row[1],
                            "run_ref": row[2],
                            "cleaned_at": row[3],
                            "import_at": row[4],
                            "duration_ms": row[5],
                            "scope_rows": row[6],
                            "file_count": row[7],
                            "summary": row[8],
                        },
                    )
                )
            return items
        finally:
            try:
                con.close()
            except Exception:
                pass

    def get_step_detail(
        self,
        case_id: str,
        step: int,
        *,
        page: int,
        page_size: int,
        offset: Optional[int] = None,
    ) -> Dict[str, Any]:
        if not case_id:
            raise ValueError("case_id required")
        if page < 1:
            raise ValueError("page must be >= 1")
        if page_size < 1:
            raise ValueError("page_size must be >= 1")
        if offset is not None and offset < 0:
            raise ValueError("offset must be >= 0")

        definition = get_cleaning_step_definition(step)
        query_offset = int(offset) if offset is not None else (page - 1) * page_size
        try:
            con = self._open_step_query_connection(case_id)
        except Exception:
            raise CleaningStepSourceUnavailableError() from None
        try:
            cur = con.cursor()
            total = self._fetch_step_count(cur, case_id, definition)
            cur.execute(definition.sql, (case_id, page_size, query_offset))
            raw_rows = cur.fetchall()

            rows: List[Dict[str, List[str]]] = []
            for raw_row in raw_rows:
                values = [self._normalize_step_value(value, definition.empty_placeholder) for value in raw_row]
                rows.append({"values": values})

            return {
                "step": definition.step,
                "key": definition.key,
                "title": definition.title,
                "kind": definition.kind,
                "description": definition.description,
                "headers": list(definition.headers),
                "items": rows,
                "total": total,
            }
        except CleaningStepSourceUnavailableError:
            raise
        except Exception:
            raise CleaningStepSourceUnavailableError() from None
        finally:
            try:
                con.close()
            except Exception:
                pass

    def _open_step_query_connection(self, case_id: str) -> DuckDBEngine:
        last_error: Exception | None = None
        for attempt in range(5):
            con: DuckDBEngine | None = None
            try:
                con = self._storage.open_case_engine(case_id)
                cleaning_connection_setup.configure_cleaning_connection(self._storage, case_id, con)
                return con
            except Exception as exc:
                if con is not None:
                    try:
                        con.close()
                    except Exception:
                        pass
                if not _is_retryable_step_query_conflict(exc) or attempt >= 4:
                    raise
                last_error = exc
                time.sleep(0.05 * (attempt + 1))
        if last_error is not None:
            raise last_error
        raise RuntimeError("failed to open cleaning step query connection")

    def _fetch_step_count(self, cur: Any, case_id: str, definition: CleaningStepDefinition) -> int:
        try:
            cur.execute(definition.count_sql, (case_id,))
            row = cur.fetchone()
            if row is None or not isinstance(row, (list, tuple)) or not row or row[0] is None:
                raise CleaningStepSourceUnavailableError()
            total = int(row[0])
            if total < 0:
                raise CleaningStepSourceUnavailableError()
            return total
        except CleaningStepSourceUnavailableError:
            raise
        except Exception:
            raise CleaningStepSourceUnavailableError() from None

    @staticmethod
    def _normalize_step_value(value: Any, empty_placeholder: Optional[str]) -> str:
        if value is None:
            return empty_placeholder or ""
        text = str(value)
        if not text and empty_placeholder is not None:
            return empty_placeholder
        return text
