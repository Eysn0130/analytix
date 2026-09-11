from __future__ import annotations

import logging
import threading
from time import monotonic
from typing import Any, Dict, List, Optional, Sequence, Tuple

from app.core.data_engine_client import data_engine_product_error
from app.core.safe_observability import log_closed_diagnostic
from app.domain.ordinary_diagnostic_projection import project_cleaning_log_event
from app.repositories.cleaning_repository import CleaningCancelled, CleaningRepository
from app.tasks.models import TaskRecord, TaskStatus, TaskType
from app.tasks.service import TaskNotFoundError, TaskRuntimePayloadUnavailableError, TaskService
from app.tasks.state_machine import InvalidTaskTransitionError
from app.utils.time import utc_now
from app.ws.events import build_event
from app.ws.manager import WebSocketManager

_LOGGER = logging.getLogger("analytix.data_analysis.cleaning")


class CleaningJobNotFoundError(KeyError):
    pass


class CleaningJobNotCancellableError(RuntimeError):
    pass


class CleaningJobAlreadyActiveError(RuntimeError):
    def __init__(self, *, case_id: str, job_id: str) -> None:
        super().__init__("cleaning job already active")
        self.case_id = case_id
        self.job_id = job_id


class CleaningNativeUnavailableError(RuntimeError):
    def __init__(self, *, health: dict[str, Any]) -> None:
        message = str(health.get("message") or "Rust cleaning unavailable")
        super().__init__(message)
        self.health = dict(health)


class CleaningService:
    _EVENT_LOG_MAX_ENTRIES = 500
    _EVENT_LOG_FLUSH_INTERVAL_SECONDS = 0.75
    _EVENT_LOG_FLUSH_EVERY_EVENTS = 12

    def __init__(
        self,
        *,
        task_service: TaskService,
        repository: CleaningRepository,
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
        self._event_log_state: Dict[str, dict[str, Any]] = {}

    def get_runtime_health(self) -> dict[str, Any]:
        return self._repository.get_cleaning_runtime_health()

    def create_job(
        self,
        *,
        case_id: str,
        steps: Optional[Sequence[int]],
        force_rebuild: bool,
        reset_reason: Optional[str] = None,
        source_job_id: Optional[str] = None,
    ) -> TaskRecord:
        norm_steps = sorted({int(s) for s in (steps or []) if 1 <= int(s) <= 10})
        metadata = {
            "requested_steps": norm_steps,
            "force_rebuild": bool(force_rebuild),
            "reset_reason": reset_reason or "",
            "source_job_id": source_job_id or "",
            "reset_result": {},
            "event_log": [],
        }
        with self._lock:
            active_job = self._find_active_job(case_id=case_id)
            if active_job is not None:
                raise CleaningJobAlreadyActiveError(case_id=case_id, job_id=active_job.task_id)
            runtime_health = self._repository.get_cleaning_runtime_health()
            if not runtime_health.get("native_cleaning_available"):
                raise CleaningNativeUnavailableError(health=runtime_health)
            metadata["native_cleaning_preflight"] = runtime_health
            task = self._task_service.create_task(
                task_type=TaskType.CLEANING,
                case_id=case_id,
                metadata=metadata,
            )
            self._cancel_flags[task.task_id] = threading.Event()
            self._event_log_state[task.task_id] = {
                "events": [],
                "event_ids": set(),
                "pending_events": 0,
                "dirty": False,
                "last_flush_monotonic": monotonic(),
            }
            thread = threading.Thread(
                target=self._run_job,
                args=(task.task_id,),
                daemon=True,
                name=f"cleaning-job-{task.task_id[:8]}",
            )
            self._threads[task.task_id] = thread

        self._emit_event(
            job_id=task.task_id,
            case_id=case_id,
            event="cleaning.job.queued",
            event_type="info",
            dedupe_key="queued",
            persist_immediately=True,
            payload={
                "progress": 0,
                "stage": "queued",
                "message": "cleaning job accepted",
                "counters": {
                    "requested_steps": len(norm_steps) if norm_steps else 10,
                },
            },
        )
        thread.start()
        return task

    def reset_job(self, job_id: str, *, case_id: str, reason: Optional[str]) -> TaskRecord:
        old = self.get_job(job_id, case_id=case_id)
        try:
            runtime_payload = self._task_service.read_runtime_payload(job_id, case_id=old.case_id)
        except TaskRuntimePayloadUnavailableError:
            runtime_payload = {}
        steps = runtime_payload.get("requested_steps") if isinstance(runtime_payload, dict) else []
        return self.create_job(
            case_id=old.case_id or "",
            steps=steps if isinstance(steps, list) else [],
            force_rebuild=True,
            reset_reason=reason or "",
            source_job_id=job_id,
        )

    def get_job(self, job_id: str, *, case_id: Optional[str] = None) -> TaskRecord:
        try:
            task = self._task_service.get_task(job_id)
        except TaskNotFoundError as exc:
            raise CleaningJobNotFoundError(job_id) from exc
        if task.task_type != TaskType.CLEANING:
            raise CleaningJobNotFoundError(job_id)
        if case_id is not None and task.case_id != case_id:
            raise CleaningJobNotFoundError(job_id)
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
            task_type=TaskType.CLEANING,
            case_id=case_id,
        )

    def cancel_job(self, job_id: str, *, case_id: str) -> TaskRecord:
        task = self.get_job(job_id, case_id=case_id)
        if task.status in {TaskStatus.SUCCEEDED, TaskStatus.FAILED, TaskStatus.CANCELED}:
            raise CleaningJobNotCancellableError(job_id)
        with self._lock:
            self._cancel_flags.setdefault(job_id, threading.Event()).set()
        return self.get_job(job_id)

    def _find_active_job(self, *, case_id: str) -> Optional[TaskRecord]:
        active_status = (TaskStatus.QUEUED, TaskStatus.RUNNING)
        for status in active_status:
            tasks, _ = self._task_service.list_tasks(
                page=1,
                page_size=2000,
                status=status,
                task_type=TaskType.CLEANING,
                case_id=case_id,
            )
            if tasks:
                tasks.sort(key=lambda item: (item.created_at, item.task_id), reverse=True)
                return tasks[0]
        return None

    def list_step_summaries(self, *, case_id: str) -> List[dict]:
        return self._repository.list_step_summaries(case_id)

    def get_step_detail(
        self,
        *,
        case_id: str,
        step: int,
        page: int,
        page_size: int,
        offset: Optional[int] = None,
    ) -> dict:
        return self._repository.get_step_detail(
            case_id,
            step,
            page=page,
            page_size=page_size,
            offset=offset,
        )

    def list_history(self, *, case_id: str, limit: int = 50) -> List[dict]:
        return self._repository.list_cleaning_history(case_id, limit=limit)

    def list_log_events(self, *, case_id: str, limit: int = 200) -> List[dict]:
        items, _ = self._task_service.list_tasks(
            page=1,
            page_size=5000,
            task_type=TaskType.CLEANING,
            case_id=case_id,
        )
        events: List[dict] = []
        seen: set[str] = set()
        for task in items:
            raw_items = self._snapshot_task_event_log(task)
            if not isinstance(raw_items, list):
                continue
            for item in raw_items:
                if not isinstance(item, dict):
                    continue
                projected = project_cleaning_log_event(
                    job_id=task.task_id,
                    event=item,
                    timestamp=item.get("timestamp"),
                    case_id=case_id,
                )
                event_id = projected["event_id"]
                if event_id and event_id in seen:
                    continue
                if event_id:
                    seen.add(event_id)
                events.append(projected)
        events.sort(key=lambda item: item["timestamp"], reverse=True)
        return events[: max(1, int(limit or 200))]

    def _load_persisted_task_events(self, job_id: str) -> List[dict]:
        try:
            task = self._task_service.get_task(job_id)
            metadata = self._task_service.read_runtime_payload(job_id, case_id=task.case_id)
        except (TaskNotFoundError, TaskRuntimePayloadUnavailableError):
            return []
        raw_log = metadata.get("event_log")
        if not isinstance(raw_log, list):
            return []
        return [
            project_cleaning_log_event(
                job_id=job_id,
                event=item,
                timestamp=item.get("timestamp"),
                case_id=task.case_id,
            )
            for item in raw_log
            if isinstance(item, dict)
        ]

    def _ensure_event_log_state(self, job_id: str) -> dict[str, Any]:
        with self._lock:
            state = self._event_log_state.get(job_id)
        if state is not None:
            return state

        persisted_events = self._load_persisted_task_events(job_id)
        restored_state: dict[str, Any] = {
            "events": persisted_events[-self._EVENT_LOG_MAX_ENTRIES :],
            "event_ids": {
                str(item.get("event_id") or "").strip()
                for item in persisted_events
                if str(item.get("event_id") or "").strip()
            },
            "pending_events": 0,
            "dirty": False,
            "last_flush_monotonic": monotonic(),
        }
        with self._lock:
            current = self._event_log_state.get(job_id)
            if current is not None:
                return current
            self._event_log_state[job_id] = restored_state
            return restored_state

    def _snapshot_task_event_log(self, task: TaskRecord) -> List[dict]:
        with self._lock:
            runtime = self._event_log_state.get(task.task_id)
            if runtime is not None:
                return [dict(item) for item in list(runtime.get("events") or []) if isinstance(item, dict)]
        try:
            metadata = self._task_service.read_runtime_payload(task.task_id, case_id=task.case_id)
        except TaskRuntimePayloadUnavailableError:
            return []
        raw_items = metadata.get("event_log")
        if not isinstance(raw_items, list):
            return []
        return [dict(item) for item in raw_items if isinstance(item, dict)]

    def _flush_task_events(self, job_id: str, *, force: bool = False) -> None:
        state = self._ensure_event_log_state(job_id)
        with self._lock:
            dirty = bool(state.get("dirty"))
            pending_events = int(state.get("pending_events") or 0)
            last_flush = float(state.get("last_flush_monotonic") or 0.0)
            now = monotonic()
            should_flush = force or (
                dirty
                and (
                    pending_events >= self._EVENT_LOG_FLUSH_EVERY_EVENTS
                    or (now - last_flush) >= self._EVENT_LOG_FLUSH_INTERVAL_SECONDS
                )
            )
            if not should_flush:
                return
            event_log = [dict(item) for item in list(state.get("events") or []) if isinstance(item, dict)]

        try:
            self._task_service.update_task_metadata(job_id, {"event_log": event_log})
        except TaskNotFoundError:
            return

        with self._lock:
            current = self._event_log_state.get(job_id)
            if current is not None:
                current["pending_events"] = 0
                current["dirty"] = False
                current["last_flush_monotonic"] = now

    def _run_job(self, job_id: str) -> None:
        try:
            self._run_job_inner(job_id)
        except Exception:
            self._safe_transition(
                job_id,
                to_status=TaskStatus.FAILED,
                progress=100,
                error="cleaning_failed",
            )
        finally:
            self._cleanup_runtime(job_id)

    def _run_job_inner(self, job_id: str) -> None:
        task = self.get_job(job_id)
        case_id = task.case_id or ""

        if self._is_cancel_requested(job_id):
            self._finalize_canceled(task, "cleaning job canceled before running")
            return

        started = self._safe_transition(
            job_id,
            to_status=TaskStatus.RUNNING,
            progress=1,
            metadata={"started_at": utc_now().isoformat()},
        )
        if started is None:
            return
        runtime_payload = self._task_service.read_runtime_payload(job_id, case_id=task.case_id)
        requested_steps = runtime_payload.get("requested_steps") if isinstance(runtime_payload, dict) else []
        force_rebuild = bool(runtime_payload.get("force_rebuild")) if isinstance(runtime_payload, dict) else False
        reset_reason = runtime_payload.get("reset_reason") if isinstance(runtime_payload, dict) else ""

        self._emit_event(
            job_id=job_id,
            case_id=case_id,
            event="cleaning.job.progress",
            event_type="progress",
            dedupe_key="progress:running",
            payload={
                "progress": 1,
                "stage": "running",
                "message": "cleaning job started",
                "counters": {"requested_steps": len(requested_steps) if requested_steps else 10},
            },
        )

        try:
            if force_rebuild:
                self._emit_log_event(
                    job_id=job_id,
                    case_id=case_id,
                    step_id="reset",
                    message=f"reset clean state requested: {reset_reason or 'manual reset'}",
                    level="info",
                )
                reset_result = self._repository.reset_clean_state(case_id)
                self._safe_transition(
                    job_id,
                    to_status=TaskStatus.RUNNING,
                    progress=3,
                    metadata={"reset_result": reset_result},
                )

            summary = self._repository.run_cleaning(
                case_id=case_id,
                file_ids=None,
                steps=requested_steps,
                cancel_check=lambda: self._is_cancel_requested(job_id),
                progress_cb=lambda progress, message, step: self._on_progress(
                    job_id=job_id,
                    case_id=case_id,
                    progress=progress,
                    message=message,
                    step=step,
                ),
                log_cb=lambda step_id, message: self._emit_log_event(
                    job_id=job_id,
                    case_id=case_id,
                    step_id=step_id,
                    message=message,
                    level="info",
                ),
            )

            analysis_timing = self._repository.refresh_analysis_outputs_after_cleaning(
                case_id,
                reason=f"cleaning-job:{job_id}",
            )
            summary["analysis_timing"] = analysis_timing
            updated = self._safe_transition(
                job_id,
                to_status=TaskStatus.SUCCEEDED,
                progress=100,
                metadata={
                    "finished_at": utc_now().isoformat(),
                },
            )
            if updated is not None:
                self._emit_event(
                    job_id=job_id,
                    case_id=case_id,
                    event="cleaning.job.completed",
                    event_type="success",
                    dedupe_key="completed",
                    persist_immediately=True,
                    payload={
                        "progress": 100,
                        "stage": "completed",
                        "message": "cleaning completed",
                    },
                )
        except CleaningCancelled:
            self._finalize_canceled(self.get_job(job_id), "cleaning canceled by user")
        except Exception as exc:
            error_payload = self._job_error_payload(exc)
            if error_payload["code"] != "INTERNAL_ERROR":
                log_closed_diagnostic(
                    _LOGGER,
                    logging.WARNING,
                    topic="cleaning_job",
                    code="data_engine_failed",
                    flags={"retryable": bool(error_payload["retryable"])},
                )
            else:
                log_closed_diagnostic(
                    _LOGGER,
                    logging.ERROR,
                    topic="cleaning_job",
                    code="internal_error",
                    flags={"retryable": False},
                )
            updated = self._safe_transition(
                job_id,
                to_status=TaskStatus.FAILED,
                progress=100,
                error=error_payload["message"],
            )
            if updated is not None:
                self._emit_event(
                    job_id=job_id,
                    case_id=case_id,
                    event="cleaning.job.failed",
                    event_type="error",
                    dedupe_key="failed",
                    persist_immediately=True,
                    payload={
                        "code": error_payload["code"],
                        "message": error_payload["message"],
                        "retryable": error_payload["retryable"],
                        "details": error_payload["details"],
                    },
                )
    @staticmethod
    def _job_error_payload(exc: Exception) -> dict[str, Any]:
        data_engine_error = data_engine_product_error(exc)
        if data_engine_error is not None:
            return data_engine_error
        return {
            "status_code": 500,
            "code": "INTERNAL_ERROR",
            "message": "cleaning failed",
            "retryable": False,
            "details": {},
        }

    def _on_progress(
        self,
        *,
        job_id: str,
        case_id: str,
        progress: int,
        message: str,
        step: Optional[int],
    ) -> None:
        pct = max(0, min(100, int(progress)))
        self._safe_transition(job_id, to_status=TaskStatus.RUNNING, progress=pct)
        step_label = f"step_{step}" if step else "running"
        self._emit_event(
            job_id=job_id,
            case_id=case_id,
            event="cleaning.job.progress",
            event_type="progress",
            dedupe_key=f"progress:{step_label}:{pct}:{message}",
            payload={
                "progress": pct,
                "stage": step_label,
                "message": message,
                "counters": {"step": int(step or 0)},
            },
        )

    def _emit_log_event(
        self,
        *,
        job_id: str,
        case_id: str,
        step_id: str,
        message: str,
        level: str,
    ) -> None:
        step_num: Optional[int] = None
        if step_id.startswith("step"):
            try:
                step_num = int(step_id.replace("step", "").strip())
            except Exception:
                step_num = None
        self._emit_event(
            job_id=job_id,
            case_id=case_id,
            event="cleaning.job.log",
            event_type="log",
            payload={
                "level": level if level in {"debug", "info", "warn", "error"} else "info",
                "message": message,
                "step": step_num,
            },
            dedupe_key=None,
        )

    def _append_task_event(
        self,
        *,
        job_id: str,
        case_id: str,
        event: str,
        payload: dict,
        persist_immediately: bool = False,
    ) -> None:
        state = self._ensure_event_log_state(job_id)
        with self._lock:
            event_log = list(state.get("events") or [])
            event_ids = set(state.get("event_ids") or set())
        event_id = str(payload.get("event_id") or "").strip()
        if event_id and event_id in event_ids:
            return
        counters = payload.get("counters") if isinstance(payload.get("counters"), dict) else {}
        step_value = payload.get("step")
        if step_value is None:
            step_value = counters.get("step")
        progress_value = payload.get("progress")
        if event == "cleaning.job.log":
            level = str(payload.get("level") or "info")
        elif event.endswith("failed"):
            level = "error"
        elif event.endswith("completed"):
            level = "success"
        elif event.endswith("progress"):
            level = "progress"
        else:
            level = "info"
        entry = {
            "contract": str(payload.get("contract") or ""),
            "event_binding_hash": str(payload.get("event_binding_hash") or ""),
            "event_id": event_id or f"{job_id}:{len(event_log) + 1}",
            "job_id": job_id,
            "case_id": case_id,
            "event": event,
            "level": level,
            "message_code": str(payload.get("message_code") or ""),
            "message": str(payload.get("message") or ""),
            "step": int(step_value) if step_value is not None and str(step_value).strip() else None,
            "progress": int(progress_value) if progress_value is not None and str(progress_value).strip() else None,
            "timestamp": utc_now().isoformat(),
        }
        with self._lock:
            current = self._event_log_state.get(job_id)
            if current is None:
                current = state
                self._event_log_state[job_id] = current
            events = current.setdefault("events", [])
            ids = current.setdefault("event_ids", set())
            if entry["event_id"] in ids:
                return
            events.append(entry)
            ids.add(entry["event_id"])
            if len(events) > self._EVENT_LOG_MAX_ENTRIES:
                del events[: len(events) - self._EVENT_LOG_MAX_ENTRIES]
                current["event_ids"] = {
                    str(item.get("event_id") or "").strip()
                    for item in events
                    if isinstance(item, dict) and str(item.get("event_id") or "").strip()
                }
            current["pending_events"] = int(current.get("pending_events") or 0) + 1
            current["dirty"] = True
        self._flush_task_events(job_id, force=persist_immediately)

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
            event="cleaning.job.failed",
            event_type="error",
            dedupe_key="failed:canceled",
            persist_immediately=True,
            payload={
                "code": "JOB_CANCELED",
                "message": message,
                "retryable": False,
                "details": {},
            },
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
        self._flush_task_events(job_id, force=True)
        with self._lock:
            self._threads.pop(job_id, None)
            self._cancel_flags.pop(job_id, None)
            self._job_event_seq.pop(job_id, None)
            self._event_dedup.pop(job_id, None)
            self._event_log_state.pop(job_id, None)

    def _emit_event(
        self,
        *,
        job_id: str,
        case_id: Optional[str],
        event: str,
        event_type: str,
        payload: dict,
        dedupe_key: Optional[str],
        persist_immediately: bool = False,
    ) -> None:
        with self._lock:
            if dedupe_key:
                emitted = self._event_dedup.setdefault(job_id, set())
                if dedupe_key in emitted:
                    return
                emitted.add(dedupe_key)
            local_seq = self._job_event_seq.get(job_id, 0) + 1
            self._job_event_seq[job_id] = local_seq

        raw_payload = dict(payload or {})
        raw_payload.setdefault("event_id", f"{job_id}:{local_seq}")
        projected = project_cleaning_log_event(
            job_id=job_id,
            event={"event": event, **raw_payload},
            timestamp=utc_now().isoformat(),
            case_id=str(case_id or ""),
        )
        payload_obj = {
            "contract": projected["contract"],
            "event_binding_hash": projected["event_binding_hash"],
            "event_id": projected["event_id"],
            "level": projected["level"],
            "message_code": projected["message_code"],
            "message": projected["message"],
            "stage": projected["stage"],
            "counters": projected["counters"],
            "step": projected["step"],
            "progress": projected["progress"],
        }
        self._append_task_event(
            job_id=job_id,
            case_id=case_id or "",
            event=event,
            payload=payload_obj,
            persist_immediately=persist_immediately,
        )

        envelope = build_event(
            event=event,
            event_type=event_type,
            channel="cleaning",
            sequence=next(self._ws_sequence),
            version=self._ws_version,
            job_id=job_id,
            case_id=case_id,
            payload=payload_obj,
        )
        self._ws_manager.publish_threadsafe(envelope)
