from __future__ import annotations

import threading
from concurrent.futures import Future, ThreadPoolExecutor
from typing import Any, Dict, List, Optional

from app.domain.controlled_artifact_gate import require_controlled_artifact_publication
from app.domain.stats_export_request_store import (
    StatsExportRequestStore,
    StatsExportRequestUnavailableError,
)
from app.domain.stats_export_source import StatsExportSourceResolver
from app.domain.stats_export_writer import StatsExportCanceled, StatsExportWriter
from app.domain.stats_service import StatsService
from app.tasks.failure_boundary import closed_task_failure_event_payload
from app.tasks.models import TaskRecord, TaskStatus, TaskType
from app.tasks.service import TaskNotFoundError, TaskService
from app.tasks.state_machine import InvalidTaskTransitionError
from app.utils.time import utc_now
from app.ws.events import build_event
from app.ws.manager import WebSocketManager


class StatsExportJobNotFoundError(KeyError):
    pass


class StatsExportJobNotCancellableError(RuntimeError):
    pass


class StatsQueryJobNotFoundError(KeyError):
    pass


class StatsQueryJobNotCancellableError(RuntimeError):
    pass


class StatsQueryJobNotReadyError(RuntimeError):
    pass


class StatsQueryJobQuarantinedError(RuntimeError):
    pass


class StatsJobService:
    def __init__(
        self,
        *,
        task_service: TaskService,
        stats_service: StatsService,
        ws_manager: WebSocketManager,
        ws_sequence,
        ws_version: str = "v1",
        max_workers: int = 4,
    ) -> None:
        self._task_service = task_service
        self._stats_service = stats_service
        self._ws_manager = ws_manager
        self._ws_sequence = ws_sequence
        self._ws_version = ws_version
        self._max_workers = max(1, int(max_workers))
        self._executor = ThreadPoolExecutor(max_workers=self._max_workers, thread_name_prefix="stats-job")

        self._lock = threading.RLock()
        self._cancel_flags: Dict[str, threading.Event] = {}
        self._futures: Dict[str, Future[None]] = {}
        self._job_event_seq: Dict[str, int] = {}
        self._event_dedup: Dict[str, set[str]] = {}
        self._export_request_store = StatsExportRequestStore(
            request_dir=self._stats_service.storage.app_dir / "stats_export_requests"
        )
        self._export_writer = StatsExportWriter(storage=self._stats_service.storage)
        # Raw stats-query rows are not an evidence authority. Do not create,
        # scan, GC, or read the legacy result directory during service startup.
        self._export_sources = StatsExportSourceResolver()

    def create_export_job(
        self,
        *,
        case_id: str,
        case_name: str,
        date: str,
        label: str,
        output_path: str,
        sheets: List[dict],
    ) -> TaskRecord:
        require_controlled_artifact_publication()
        if not sheets:
            raise ValueError("sheets is empty")

        request_payload = {
            "case_id": str(case_id or "").strip(),
            "case_name": str(case_name or "").strip(),
            "date": str(date or "").strip(),
            "label": str(label or "").strip(),
            "output_path": str(output_path or "").strip(),
            "sheets": sheets,
        }
        metadata = {
            "request": {
                "case_id": request_payload["case_id"],
                "case_name": request_payload["case_name"],
                "date": request_payload["date"],
                "label": request_payload["label"],
                "output_path": request_payload["output_path"],
                "sheet_count": len(sheets),
            },
            "result": {},
        }
        task = self._task_service.create_task(
            task_type=TaskType.STATS_EXPORT,
            case_id=case_id,
            metadata=metadata,
        )
        request_persisted = False
        try:
            request_ref = self._export_request_store.persist(job_id=task.task_id, request=request_payload)
            request_persisted = True
            metadata["request"]["request_ref"] = request_ref
            task = self._task_service.update_task_metadata(
                task.task_id,
                metadata={"request": metadata["request"]},
            )
        except Exception as exc:
            if request_persisted:
                try:
                    self._export_request_store.delete_for_job(job_id=task.task_id)
                except StatsExportRequestUnavailableError:
                    pass
            self._safe_transition(
                task.task_id,
                to_status=TaskStatus.FAILED,
                progress=100,
                error=str(exc),
            )
            raise
        with self._lock:
            self._cancel_flags[task.task_id] = threading.Event()

        self._emit_event(
            job_id=task.task_id,
            case_id=case_id,
            event="analysis.stats.export.progress",
            event_type="progress",
            dedupe_key="progress:queued",
            payload={
                "progress": 0,
                "stage": "queued",
                "message": "stats export job accepted",
            },
        )

        with self._lock:
            future = self._executor.submit(self._run_export_job, task.task_id)
            self._futures[task.task_id] = future
            future.add_done_callback(lambda completed: self._forget_future(task.task_id, completed))
        return task

    def create_query_job(
        self,
        *,
        case_id: str,
        query: str,
        request_id: str,
        mode: str,
        selected: List[str],
        date_start: str,
        date_end: str,
        key_type: str,
        key_value: str,
        direction: str,
        sort_col: str,
        sort_dir: str,
        limit: int,
        cursor: Optional[dict],
        search_text: str,
        row_sort_col: str,
        row_sort_dir: str,
        row_offset: int,
        row_limit: int,
    ) -> TaskRecord:
        del (
            case_id,
            query,
            request_id,
            mode,
            selected,
            date_start,
            date_end,
            key_type,
            key_value,
            direction,
            sort_col,
            sort_dir,
            limit,
            cursor,
            search_text,
            row_sort_col,
            row_sort_dir,
            row_offset,
            row_limit,
        )
        raise StatsQueryJobQuarantinedError("host_evidence_receipt_required")

    def _get_job_by_type(self, job_id: str, *, task_type: TaskType, err_cls):
        try:
            task = self._task_service.get_task(job_id)
        except TaskNotFoundError as exc:
            raise err_cls(job_id) from exc
        if task.task_type != task_type:
            raise err_cls(job_id)
        return task

    def get_export_job(self, job_id: str) -> TaskRecord:
        return self._get_job_by_type(job_id, task_type=TaskType.STATS_EXPORT, err_cls=StatsExportJobNotFoundError)

    def get_query_job(self, job_id: str) -> TaskRecord:
        del job_id
        raise StatsQueryJobQuarantinedError("host_evidence_receipt_required")

    def get_query_job_result(self, job_id: str) -> dict:
        del job_id
        raise StatsQueryJobQuarantinedError("host_evidence_receipt_required")

    def cancel_export_job(self, job_id: str) -> TaskRecord:
        task = self.get_export_job(job_id)
        if task.status in {TaskStatus.SUCCEEDED, TaskStatus.FAILED, TaskStatus.CANCELED}:
            raise StatsExportJobNotCancellableError(job_id)
        with self._lock:
            self._cancel_flags.setdefault(job_id, threading.Event()).set()
        return self.get_export_job(job_id)

    def cancel_query_job(self, job_id: str) -> TaskRecord:
        del job_id
        raise StatsQueryJobQuarantinedError("host_evidence_receipt_required")

    def _run_export_job(self, job_id: str) -> None:
        case_id = ""
        try:
            task = self.get_export_job(job_id)
            case_id = task.case_id or ""
            request_ref = self._resolve_export_request_ref(job_id=job_id, task=task)
            if self._is_cancel_requested(job_id):
                self._finalize_canceled(task, "stats export job canceled before running")
                return

            started = self._safe_transition(
                job_id,
                to_status=TaskStatus.RUNNING,
                progress=1,
                metadata={"started_at": utc_now().isoformat()},
            )
            if started is None:
                return

            self._emit_event(
                job_id=job_id,
                case_id=case_id,
                event="analysis.stats.export.progress",
                event_type="progress",
                dedupe_key="progress:running",
                payload={
                    "progress": 1,
                    "stage": "running",
                    "message": "stats export started",
                },
            )

            request_obj = self._resolve_export_request(job_id=job_id, request_ref=request_ref)
            sheets = self._export_sources.resolve_sheets(job_id=job_id, request=request_obj)
            if not sheets:
                raise ValueError("sheets is empty")
            out_path = self._export_writer.resolve_output_path(request_obj)

            def on_sheet_written(sheet_index: int, sheet_total: int, progress: int) -> None:
                self._safe_transition(job_id, to_status=TaskStatus.RUNNING, progress=progress)
                self._emit_event(
                    job_id=job_id,
                    case_id=case_id,
                    event="analysis.stats.export.progress",
                    event_type="progress",
                    dedupe_key=f"progress:sheet:{sheet_index}:{progress}",
                    payload={
                        "progress": progress,
                        "stage": "writing",
                        "message": f"writing sheet {sheet_index}/{sheet_total}",
                        "counters": {"sheet_index": sheet_index, "sheet_total": sheet_total},
                    },
                )

            written = self._export_writer.write_workbook(
                output_path=out_path,
                sheets=sheets,
                should_cancel=lambda: self._is_cancel_requested(job_id),
                on_sheet_written=on_sheet_written,
            )

            updated = self._safe_transition(
                job_id,
                to_status=TaskStatus.SUCCEEDED,
                progress=100,
                metadata={
                    "result": {
                        "path": str(written.path),
                        "sheet_count": written.sheet_count,
                    },
                    "finished_at": utc_now().isoformat(),
                },
            )
            if updated is not None:
                self._emit_event(
                    job_id=job_id,
                    case_id=case_id,
                    event="analysis.stats.export.completed",
                    event_type="success",
                    dedupe_key="completed",
                    payload={
                        "progress": 100,
                        "path": str(written.path),
                        "sheet_count": written.sheet_count,
                    },
                )
        except StatsExportCanceled:
            self._finalize_canceled(self.get_export_job(job_id), "stats export job canceled")
        except Exception as exc:
            updated = self._safe_transition(
                job_id,
                to_status=TaskStatus.FAILED,
                progress=100,
                error=str(exc),
            )
            if updated is not None:
                self._emit_event(
                    job_id=job_id,
                    case_id=case_id,
                    event="analysis.stats.export.failed",
                    event_type="error",
                    dedupe_key="failed",
                    payload=closed_task_failure_event_payload(
                        service="stats_export",
                        code="INTERNAL_ERROR",
                        retryable=False,
                        correlation_id=updated.metadata.get("failure_correlation_id"),
                    ),
                )
        finally:
            self._cleanup_runtime(job_id)

    def _run_query_job(self, job_id: str) -> None:
        del job_id
        raise StatsQueryJobQuarantinedError("host_evidence_receipt_required")

    def _resolve_export_request_ref(self, *, job_id: str, task: TaskRecord) -> object:
        runtime_payload = self._task_service.read_runtime_payload(job_id, case_id=task.case_id)
        request = runtime_payload.get("request") if isinstance(runtime_payload, dict) else None
        if isinstance(request, dict):
            request_ref = request.get("request_ref")
            if request_ref is not None:
                return request_ref
        raise ValueError("stats export request payload not found")

    def _resolve_export_request(self, *, job_id: str, request_ref: object) -> dict:
        try:
            return self._export_request_store.load(job_id=job_id, request_ref=request_ref)
        except StatsExportRequestUnavailableError as exc:
            raise ValueError("stats export request payload not found") from exc

    def _finalize_canceled(self, task: TaskRecord, message: str) -> None:
        updated = self._safe_transition(
            task.task_id,
            to_status=TaskStatus.CANCELED,
            progress=max(task.progress, 1),
            error=message,
        )
        if updated is None:
            return
        self._emit_event(
            job_id=task.task_id,
            case_id=task.case_id,
            event="analysis.stats.export.failed",
            event_type="error",
            dedupe_key="failed:canceled",
            payload=closed_task_failure_event_payload(
                service="stats_export",
                code="JOB_CANCELED",
                retryable=False,
                correlation_id=updated.metadata.get("failure_correlation_id"),
            ),
        )

    def _safe_transition(
        self,
        job_id: str,
        *,
        to_status: TaskStatus,
        progress: Optional[int] = None,
        error: Optional[str] = None,
        metadata: Optional[dict] = None,
    ) -> Optional[TaskRecord]:
        try:
            return self._task_service.transition_task(
                task_id=job_id,
                to_status=to_status,
                progress=progress,
                error=error,
                metadata=metadata,
            )
        except (TaskNotFoundError, InvalidTaskTransitionError):
            return None

    def _is_cancel_requested(self, job_id: str) -> bool:
        with self._lock:
            flag = self._cancel_flags.get(job_id)
        return bool(flag and flag.is_set())

    def _cleanup_runtime(self, job_id: str) -> None:
        try:
            self._export_request_store.delete_for_job(job_id=job_id)
        except StatsExportRequestUnavailableError:
            pass
        with self._lock:
            self._futures.pop(job_id, None)
            self._cancel_flags.pop(job_id, None)
            self._job_event_seq.pop(job_id, None)
            self._event_dedup.pop(job_id, None)

    def _forget_future(self, job_id: str, future: Future[None]) -> None:
        with self._lock:
            current = self._futures.get(job_id)
            if current is future:
                self._futures.pop(job_id, None)

    def shutdown(self) -> None:
        self._executor.shutdown(wait=False, cancel_futures=False)

    def _emit_event(
        self,
        *,
        job_id: str,
        case_id: Optional[str],
        event: str,
        event_type: str,
        payload: dict[str, Any],
        dedupe_key: Optional[str],
    ) -> None:
        with self._lock:
            if dedupe_key:
                emitted = self._event_dedup.setdefault(job_id, set())
                if dedupe_key in emitted:
                    return
                emitted.add(dedupe_key)
            local_seq = self._job_event_seq.get(job_id, 0) + 1
            self._job_event_seq[job_id] = local_seq

        payload_obj = dict(payload or {})
        payload_obj.setdefault("event_id", f"{job_id}:{local_seq}")

        envelope = build_event(
            event=event,
            event_type=event_type,
            channel="analysis",
            sequence=next(self._ws_sequence),
            version=self._ws_version,
            job_id=job_id,
            case_id=case_id,
            payload=payload_obj,
        )
        self._ws_manager.publish_threadsafe(envelope)

    def _coerce_non_negative_int(self, value: Any) -> int:
        try:
            out = int(value or 0)
        except Exception:
            return 0
        return max(0, out)
