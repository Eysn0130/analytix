from __future__ import annotations

import threading
from concurrent.futures import Future, ThreadPoolExecutor
from typing import Any, Optional, Sequence

from app.domain.analysis_service import AnalysisService
from app.tasks.models import TaskRecord, TaskStatus, TaskType
from app.tasks.service import TaskNotFoundError, TaskService
from app.tasks.state_machine import InvalidTaskTransitionError


class AnalysisMaterializationQuarantinedError(RuntimeError):
    pass


class AnalysisWorkerService:
    """Owns background worker execution for analysis maintenance/materialization."""

    def __init__(
        self,
        *,
        analysis_service: AnalysisService,
        task_service: TaskService,
        max_workers: int = 2,
    ) -> None:
        self._analysis_service = analysis_service
        self._task_service = task_service
        self._executor = ThreadPoolExecutor(max_workers=max(1, int(max_workers or 1)), thread_name_prefix="analysis-worker")
        self._lock = threading.RLock()
        self._futures: dict[str, Future[None]] = {}
        self._maintenance_stop = threading.Event()
        self._maintenance_thread: Optional[threading.Thread] = None
        self._maintenance_interval_s = 0
        self._maintenance_startup_delay_s = 0

    def start_background_maintenance(self, *, interval_s: int = 1800, startup_delay_s: int = 120) -> None:
        normalized_interval = max(0, int(interval_s or 0))
        if normalized_interval <= 0:
            return
        with self._lock:
            if self._maintenance_thread is not None and self._maintenance_thread.is_alive():
                return
            self._maintenance_stop.clear()
            self._maintenance_interval_s = normalized_interval
            self._maintenance_startup_delay_s = max(0, int(startup_delay_s or 0))
            self._maintenance_thread = threading.Thread(
                target=self._run_maintenance_loop,
                name="analytix-analysis-worker-maintenance",
                daemon=True,
            )
            self._maintenance_thread.start()

    def shutdown(self) -> None:
        self._maintenance_stop.set()
        with self._lock:
            thread = self._maintenance_thread
            self._maintenance_thread = None
        if thread is not None and thread.is_alive():
            thread.join(timeout=2.0)
        with self._lock:
            pending = list(self._futures.items())
        for task_id, future in pending:
            if not future.cancel():
                continue
            try:
                self._task_service.transition_task(
                    task_id,
                    to_status=TaskStatus.CANCELED,
                    progress=100,
                )
            except (InvalidTaskTransitionError, TaskNotFoundError):
                pass
            finally:
                self._forget_future(task_id, future)
        self._executor.shutdown(wait=False, cancel_futures=True)

    def create_feature_mart_materialize_job(
        self,
        *,
        case_id: str,
        account_keys: Sequence[str] = (),
        temp_scope_id: str = "",
        source_file_ids: Sequence[str] = (),
        force_refresh: bool = False,
        max_accounts: int = 12,
        dispatch_mode: str = "thread",
    ) -> TaskRecord:
        del (
            case_id,
            account_keys,
            temp_scope_id,
            source_file_ids,
            force_refresh,
            max_accounts,
            dispatch_mode,
        )
        raise AnalysisMaterializationQuarantinedError("host_evidence_receipt_required")

    def create_temp_scope_janitor_job(self, *, case_id: str = "", dispatch_mode: str = "thread") -> TaskRecord:
        task = self._task_service.create_task(
            task_type=TaskType.ANALYSIS_MAINTENANCE,
            case_id=case_id or None,
            metadata={
                "request": {"case_id": case_id or "", "job_kind": "temp_scope_janitor"},
            },
        )
        if str(dispatch_mode or "thread").strip().lower() != "queue":
            self._submit(task.task_id, self._run_temp_scope_janitor_job)
        return task

    def create_runtime_retention_janitor_job(self, *, case_id: str = "", dispatch_mode: str = "thread") -> TaskRecord:
        task = self._task_service.create_task(
            task_type=TaskType.ANALYSIS_MAINTENANCE,
            case_id=case_id or None,
            metadata={
                "request": {"case_id": case_id or "", "job_kind": "runtime_retention_janitor"},
            },
        )
        if str(dispatch_mode or "thread").strip().lower() != "queue":
            self._submit(task.task_id, self._run_runtime_retention_janitor_job)
        return task

    def run_next_queued_job(self, *, case_id: str = "") -> Optional[TaskRecord]:
        task = self._task_service.claim_next_task(
            task_types=[
                TaskType.ANALYSIS_MAINTENANCE,
            ],
            case_id=(case_id or None),
        )
        if task is None:
            return None
        completed_task: Optional[TaskRecord] = None
        if task.task_type == TaskType.ANALYSIS_MAINTENANCE:
            try:
                runtime_payload = self._task_service.read_runtime_payload(
                    task.task_id,
                    case_id=task.case_id,
                )
                request = dict(runtime_payload.get("request") or {})
                if str(request.get("job_kind") or "").strip() == "runtime_retention_janitor":
                    completed_task = self._run_runtime_retention_janitor_job(task.task_id)
                else:
                    completed_task = self._run_temp_scope_janitor_job(task.task_id)
            except Exception:
                completed_task = self._fail_task_or_return_terminal(
                    task.task_id,
                    error="analysis_maintenance_failed",
                )
        try:
            return self._task_service.get_task(task.task_id)
        except TaskNotFoundError:
            return completed_task or task

    def _submit(self, task_id: str, runner) -> None:
        future = self._executor.submit(runner, task_id)
        with self._lock:
            self._futures[task_id] = future
        future.add_done_callback(lambda completed: self._forget_future(task_id, completed))

    def _forget_future(self, task_id: str, future: Future[None] | None = None) -> None:
        with self._lock:
            current = self._futures.get(task_id)
            if future is None or current is future:
                self._futures.pop(task_id, None)

    def _fail_task_or_return_terminal(self, task_id: str, *, error: str) -> TaskRecord:
        try:
            return self._task_service.transition_task(
                task_id,
                to_status=TaskStatus.FAILED,
                progress=100,
                error=error,
            )
        except InvalidTaskTransitionError:
            return self._task_service.get_task(task_id)

    def _run_feature_mart_materialize_job(self, task_id: str) -> TaskRecord:
        del task_id
        raise AnalysisMaterializationQuarantinedError("host_evidence_receipt_required")

    def _run_temp_scope_janitor_job(self, task_id: str) -> TaskRecord:
        try:
            self._task_service.transition_task(task_id, to_status=TaskStatus.RUNNING, progress=1)
            self._analysis_service.run_temp_scope_janitor()
            return self._task_service.transition_task(
                task_id,
                to_status=TaskStatus.SUCCEEDED,
                progress=100,
                metadata={"stage": "completed"},
            )
        except Exception:
            return self._fail_task_or_return_terminal(
                task_id,
                error="analysis_maintenance_failed",
            )
        finally:
            self._forget_future(task_id)

    def _run_runtime_retention_janitor_job(self, task_id: str) -> TaskRecord:
        try:
            task = self._task_service.get_task(task_id)
            self._task_service.transition_task(task_id, to_status=TaskStatus.RUNNING, progress=1)
            runtime_payload = self._task_service.read_runtime_payload(task_id, case_id=task.case_id)
            request = dict(runtime_payload.get("request") or {})
            self._analysis_service.run_runtime_retention_janitor(case_id=str(request.get("case_id") or ""))
            return self._task_service.transition_task(
                task_id,
                to_status=TaskStatus.SUCCEEDED,
                progress=100,
                metadata={"stage": "completed"},
            )
        except Exception:
            return self._fail_task_or_return_terminal(
                task_id,
                error="analysis_maintenance_failed",
            )
        finally:
            self._forget_future(task_id)

    def _run_maintenance_loop(self) -> None:
        if self._maintenance_startup_delay_s > 0 and self._maintenance_stop.wait(self._maintenance_startup_delay_s):
            return
        while not self._maintenance_stop.is_set():
            try:
                self.create_temp_scope_janitor_job()
                self.create_runtime_retention_janitor_job()
            except Exception:
                pass
            if self._maintenance_stop.wait(max(60, int(self._maintenance_interval_s or 1800))):
                return
