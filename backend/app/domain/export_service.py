from __future__ import annotations

import threading
from typing import Dict, List, Optional, Tuple

from app.domain.controlled_artifact_gate import require_controlled_artifact_publication
from app.repositories.export_repository import ExportCancelled, ExportRepository
from app.tasks.failure_boundary import closed_task_failure_event_payload
from app.tasks.models import TaskRecord, TaskStatus, TaskType
from app.tasks.service import TaskNotFoundError, TaskService
from app.tasks.state_machine import InvalidTaskTransitionError
from app.utils.time import utc_now
from app.ws.events import build_event
from app.ws.manager import WebSocketManager


class ExportJobNotFoundError(KeyError):
    pass


class ExportJobNotCancellableError(RuntimeError):
    pass


class ExportJobAlreadyActiveError(RuntimeError):
    def __init__(self, *, case_id: str, job_id: str, mode: str) -> None:
        super().__init__("export job already active")
        self.case_id = case_id
        self.job_id = job_id
        self.mode = mode


class ExportDatasetNotReadyError(RuntimeError):
    pass


_EXPORT_PUBLIC_PROGRESS_STAGES = frozenset(
    {"queued", "running", "preparing", "processing", "exporting", "finalizing", "completed"}
)


class ExportService:
    def __init__(
        self,
        *,
        task_service: TaskService,
        repository: ExportRepository,
        ws_manager: WebSocketManager,
        ws_sequence,
        ws_version: str = "v1",
    ) -> None:
        self._task_service = task_service
        self._repository = repository
        self._ws_manager = ws_manager
        self._ws_sequence = ws_sequence
        self._ws_version = ws_version

        self._lock = threading.RLock()
        self._cancel_flags: Dict[str, threading.Event] = {}
        self._threads: Dict[str, threading.Thread] = {}
        self._job_event_seq: Dict[str, int] = {}
        self._event_dedup: Dict[str, set[str]] = {}

    def create_job(
        self,
        *,
        case_id: str,
        mode: str,
        export_format: str,
        output_name: str,
        target_dir: Optional[str],
        filters: Optional[dict],
    ) -> TaskRecord:
        require_controlled_artifact_publication()
        norm_mode = (mode or "").strip().lower()
        if norm_mode not in {"raw", "cleaned"}:
            raise ValueError("mode must be raw or cleaned")
        norm_format = (export_format or "").strip().lower()
        if norm_format not in {"csv", "xlsx"}:
            raise ValueError("export_format must be csv or xlsx")
        if norm_mode == "cleaned":
            try:
                self._repository.ensure_cleaned_export_ready(case_id)
            except ValueError as exc:
                raise ExportDatasetNotReadyError(str(exc)) from exc

        metadata = {
            "mode": norm_mode,
            "export_format": norm_format,
            "output_name": (output_name or "").strip(),
            "target_dir": (target_dir or "").strip(),
            "filters": filters or {},
            "output_path": "",
            "summary": {},
        }
        with self._lock:
            active_job = self._find_active_job(case_id=case_id)
            if active_job is not None:
                raise ExportJobAlreadyActiveError(case_id=case_id, job_id=active_job.task_id, mode=norm_mode)
            task = self._task_service.create_task(
                task_type=TaskType.EXPORT,
                case_id=case_id,
                metadata=metadata,
            )
            self._cancel_flags[task.task_id] = threading.Event()
            thread = threading.Thread(
                target=self._run_job,
                args=(task.task_id, case_id),
                daemon=True,
                name=f"export-job-{task.task_id[:8]}",
            )
            self._threads[task.task_id] = thread

        self._emit_event(
            job_id=task.task_id,
            case_id=case_id,
            event="export.job.queued",
            event_type="info",
            dedupe_key="queued",
            payload={
                "progress": 0,
                "stage": "queued",
                "message": "export job accepted",
                "counters": {
                    "mode": 1 if norm_mode == "cleaned" else 0,
                    "format": 1 if norm_format == "xlsx" else 0,
                },
            },
        )
        thread.start()
        return task

    def get_job(self, job_id: str, *, case_id: str) -> TaskRecord:
        try:
            task = self._task_service.get_public_task(job_id, case_id=case_id)
        except TaskNotFoundError as exc:
            raise ExportJobNotFoundError(job_id) from exc
        if task.task_type != TaskType.EXPORT:
            raise ExportJobNotFoundError(job_id)
        return task

    def list_jobs(
        self,
        *,
        case_id: str,
        page: int,
        page_size: int,
        status: Optional[TaskStatus] = None,
    ) -> Tuple[List[TaskRecord], int]:
        return self._task_service.list_tasks(
            page=page,
            page_size=page_size,
            status=status,
            task_type=TaskType.EXPORT,
            case_id=case_id,
        )

    def cancel_job(self, job_id: str, *, case_id: str) -> TaskRecord:
        task = self.get_job(job_id, case_id=case_id)
        if task.status in {TaskStatus.SUCCEEDED, TaskStatus.FAILED, TaskStatus.CANCELED}:
            raise ExportJobNotCancellableError(job_id)
        with self._lock:
            self._cancel_flags.setdefault(job_id, threading.Event()).set()
        return self.get_job(job_id, case_id=case_id)

    def _find_active_job(self, *, case_id: str) -> Optional[TaskRecord]:
        active_status = (TaskStatus.QUEUED, TaskStatus.RUNNING)
        for status in active_status:
            tasks, _ = self._task_service.list_tasks(
                page=1,
                page_size=2000,
                status=status,
                task_type=TaskType.EXPORT,
                case_id=case_id,
            )
            if tasks:
                tasks.sort(key=lambda item: (item.created_at, item.task_id), reverse=True)
                return tasks[0]
        return None

    def _run_job(self, job_id: str, case_id: str) -> None:
        task = self.get_job(job_id, case_id=case_id)
        runtime_payload = self._task_service.read_runtime_payload(job_id, case_id=task.case_id)
        mode = str(runtime_payload.get("mode") or "raw")
        export_format = str(runtime_payload.get("export_format") or "xlsx")
        output_name = str(runtime_payload.get("output_name") or "")
        target_dir = str(runtime_payload.get("target_dir") or "")
        filters = runtime_payload.get("filters") if isinstance(runtime_payload, dict) else {}

        if self._is_cancel_requested(job_id):
            self._finalize_canceled(task, "export job canceled before running")
            return

        started = self._safe_transition(
            job_id,
            case_id=case_id,
            to_status=TaskStatus.RUNNING,
            progress=1,
            metadata={"started_at": utc_now().isoformat()},
        )
        if started is None:
            return

        self._emit_event(
            job_id=job_id,
            case_id=case_id,
            event="export.job.progress",
            event_type="progress",
            dedupe_key="progress:running",
            payload={
                "progress": 1,
                "stage": "running",
                "message": "export job started",
                "counters": {"mode": 1 if mode == "cleaned" else 0},
            },
        )

        try:
            summary = self._repository.run_export(
                case_id=case_id,
                mode=mode,
                export_format=export_format,
                output_name=output_name,
                target_dir=target_dir,
                filters=filters if isinstance(filters, dict) else {},
                cancel_check=lambda: self._is_cancel_requested(job_id),
                progress_cb=lambda progress, message, stage, counters: self._on_progress(
                    job_id=job_id,
                    case_id=case_id,
                    progress=progress,
                    message=message,
                    stage=stage,
                    counters=counters,
                ),
            )
            output_path = str(summary.get("output_path") or "")
            updated = self._safe_transition(
                job_id,
                case_id=case_id,
                to_status=TaskStatus.SUCCEEDED,
                progress=100,
                metadata={
                    "output_path": output_path,
                    "summary": summary,
                    "finished_at": utc_now().isoformat(),
                },
            )
            if updated is not None:
                self._emit_event(
                    job_id=job_id,
                    case_id=case_id,
                    event="export.job.completed",
                    event_type="success",
                    dedupe_key="completed",
                    payload={
                        "progress": 100,
                        "stage": "completed",
                        "message": "export completed",
                        "artifact_access": "controlled_artifact_required",
                        "publication_status": "blocked",
                    },
                )
        except ExportCancelled:
            self._finalize_canceled(
                self.get_job(job_id, case_id=case_id),
                "export canceled by user",
            )
        except Exception:
            updated = self._safe_transition(
                job_id,
                case_id=case_id,
                to_status=TaskStatus.FAILED,
                progress=100,
                error="task_failed",
            )
            if updated is not None:
                self._emit_event(
                    job_id=job_id,
                    case_id=case_id,
                    event="export.job.failed",
                    event_type="error",
                    dedupe_key="failed",
                    payload=closed_task_failure_event_payload(
                        service="export",
                        code="INTERNAL_ERROR",
                        retryable=False,
                        correlation_id=updated.metadata.get("failure_correlation_id"),
                    ),
                )
        finally:
            self._cleanup_runtime(job_id)

    def _on_progress(
        self,
        *,
        job_id: str,
        case_id: str,
        progress: int,
        message: str,
        stage: Optional[str],
        counters: Optional[dict],
    ) -> None:
        pct = max(0, min(100, int(progress)))
        self._safe_transition(
            job_id,
            case_id=case_id,
            to_status=TaskStatus.RUNNING,
            progress=pct,
        )
        requested_stage = (stage or "running").strip()
        stage_label = requested_stage if requested_stage in _EXPORT_PUBLIC_PROGRESS_STAGES else "running"
        dedupe_key = f"progress:{stage_label}:{pct}"
        self._emit_event(
            job_id=job_id,
            case_id=case_id,
            event="export.job.progress",
            event_type="progress",
            dedupe_key=dedupe_key,
            payload={
                "progress": pct,
                "stage": stage_label,
                "message": f"export job {stage_label}",
            },
        )

    def _finalize_canceled(self, task: TaskRecord, message: str) -> None:
        updated = self._safe_transition(
            task.task_id,
            case_id=task.case_id or "",
            to_status=TaskStatus.CANCELED,
            progress=max(task.progress, 1),
            error=message,
        )
        if updated is None:
            return
        self._emit_event(
            job_id=task.task_id,
            case_id=task.case_id,
            event="export.job.failed",
            event_type="error",
            dedupe_key="failed:canceled",
            payload=closed_task_failure_event_payload(
                service="export",
                code="JOB_CANCELED",
                retryable=False,
                correlation_id=updated.metadata.get("failure_correlation_id"),
            ),
        )

    def _safe_transition(
        self,
        job_id: str,
        *,
        case_id: str,
        to_status: TaskStatus,
        progress: Optional[int] = None,
        error: Optional[str] = None,
        metadata: Optional[dict] = None,
    ) -> Optional[TaskRecord]:
        try:
            self.get_job(job_id, case_id=case_id)
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
        with self._lock:
            self._threads.pop(job_id, None)
            self._cancel_flags.pop(job_id, None)
            self._job_event_seq.pop(job_id, None)
            self._event_dedup.pop(job_id, None)

    def _emit_event(
        self,
        *,
        job_id: str,
        case_id: Optional[str],
        event: str,
        event_type: str,
        payload: dict,
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
            channel="export",
            sequence=next(self._ws_sequence),
            version=self._ws_version,
            job_id=job_id,
            case_id=case_id,
            payload=payload_obj,
        )
        self._ws_manager.publish_threadsafe(envelope)
