from __future__ import annotations

import threading
import time
from dataclasses import asdict
from typing import Dict, List, Literal, Optional, Tuple

from app.domain.controlled_artifact_gate import require_controlled_source_ingestion
from app.core.failure_boundary import (
    IMPORT_FILE_CANCELLED,
    IMPORT_FILE_PROCESSING_FAILED,
    IMPORT_JOB_CANCELLED,
    IMPORT_JOB_FAILED,
    private_exception_is_retryable,
    project_cleaning_error,
    project_import_error,
    project_import_note,
)
from app.core.import_count_semantics import known_public_import_count, project_import_file_log_counts
from app.tasks.failure_boundary import closed_task_failure_event_payload
from app.tasks.models import TaskRecord, TaskStatus, TaskType
from app.tasks.service import TaskNotFoundError, TaskService
from app.tasks.state_machine import InvalidTaskTransitionError
from app.utils.time import utc_now
from app.ws.events import build_event
from app.ws.manager import WebSocketManager
from app.core.fc_import_timing import add_phase, merge_timing_profiles
from app.repositories.cleaning_execution_context import CleaningCancelled
from app.repositories import cleaning_native_runtime
from app.repositories.cleaning_repository import CleaningRepository
from app.domain.import_batching import (
    DEFAULT_IMPORT_TRANSACTION_BATCH_FILES,
    DEFAULT_IMPORT_TRANSACTION_BATCH_ROWS,
    plan_next_import_unit,
)
from app.repositories.import_repository import (
    ImportArchiveItemInput,
    ImportExecutionResult,
    ImportFileInput,
    ImportRepository,
    PreparedImportFile,
)


class ImportJobNotFoundError(KeyError):
    pass


class ImportJobNotCancellableError(RuntimeError):
    pass


class ImportFileNotFoundError(KeyError):
    def __init__(self, file_ids: str | List[str]) -> None:
        normalized = (
            [str(file_id).strip() for file_id in file_ids if str(file_id).strip()]
            if isinstance(file_ids, list)
            else [str(file_ids).strip()]
        )
        self.file_ids = normalized
        super().__init__(normalized[0] if len(normalized) == 1 else ",".join(normalized))


class ImportFileBusyError(RuntimeError):
    def __init__(self, file_ids: str | List[str]) -> None:
        normalized = (
            [str(file_id).strip() for file_id in file_ids if str(file_id).strip()]
            if isinstance(file_ids, list)
            else [str(file_ids).strip()]
        )
        self.file_ids = normalized
        super().__init__(normalized[0] if len(normalized) == 1 else ",".join(normalized))


def _required_persistence_count(value: object, *, field: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        raise RuntimeError(f"import persistence {field} is unavailable")
    return value


class ImportService:
    """Orchestrates import job lifecycle on top of TaskService and the import repository."""

    _IMPORT_TRANSACTION_BATCH_FILES = DEFAULT_IMPORT_TRANSACTION_BATCH_FILES
    _IMPORT_TRANSACTION_BATCH_ROWS = DEFAULT_IMPORT_TRANSACTION_BATCH_ROWS

    def __init__(
        self,
        *,
        task_service: TaskService,
        repository: ImportRepository,
        ws_manager: WebSocketManager,
        ws_sequence,
        cleaning_repository: Optional[CleaningRepository] = None,
        ws_version: str = "v1",
        max_retries_per_file: int = 1,
    ) -> None:
        self._task_service = task_service
        self._repository = repository
        self._cleaning_repository = cleaning_repository or CleaningRepository()
        self._ws_manager = ws_manager
        self._ws_sequence = ws_sequence
        self._ws_version = ws_version
        self._max_retries_per_file = max(0, int(max_retries_per_file))

        self._lock = threading.RLock()
        self._cancel_flags: Dict[str, threading.Event] = {}
        self._threads: Dict[str, threading.Thread] = {}
        self._job_event_seq: Dict[str, int] = {}
        self._event_dedup: Dict[str, set[str]] = {}
        self._job_secrets: Dict[str, Dict[str, str]] = {}

    def create_job(self, *, case_id: str, files: List[dict], auto_cleaning: bool) -> TaskRecord:
        require_controlled_source_ingestion()
        sanitized_files: List[dict] = []
        secrets: Dict[str, str] = {}
        for item in files:
            source_path = str(item.get("source_path") or "")
            password = str(item.get("password") or "")
            sanitized = dict(item)
            sanitized.pop("password", None)
            sanitized_files.append(sanitized)
            if source_path and password:
                secrets[source_path] = password

        metadata = {
            "requested_files": sanitized_files,
            "auto_cleaning": bool(auto_cleaning),
            "imported_files": 0,
            "summary": {},
            "files": [],
            "retry_policy": {
                "max_retries_per_file": self._max_retries_per_file,
                "mode": "transient-only",
            },
        }
        task = self._task_service.create_task(
            task_type=TaskType.IMPORT,
            case_id=case_id,
            metadata=metadata,
        )

        with self._lock:
            self._cancel_flags[task.task_id] = threading.Event()
            if secrets:
                self._job_secrets[task.task_id] = secrets

        self._emit_event(
            job_id=task.task_id,
            case_id=case_id,
            event="import.job.queued",
            event_type="info",
            dedupe_key="queued",
            payload={
                "progress": 0,
                "stage": "queued",
                "message": "import job accepted",
                "counters": {
                    "requested_files": len(files),
                    "imported_files": 0,
                },
            },
        )

        thread = threading.Thread(
            target=self._run_job,
            args=(task.task_id,),
            daemon=True,
            name=f"import-job-{task.task_id[:8]}",
        )
        with self._lock:
            self._threads[task.task_id] = thread
        thread.start()
        return task

    def _get_job(self, job_id: str) -> TaskRecord:
        try:
            task = self._task_service.get_task(job_id)
        except TaskNotFoundError as exc:
            raise ImportJobNotFoundError(job_id) from exc
        if task.task_type != TaskType.IMPORT:
            raise ImportJobNotFoundError(job_id)
        return task

    def get_job(self, job_id: str, *, case_id: str) -> TaskRecord:
        try:
            task = self._task_service.get_public_task(job_id, case_id=case_id)
        except (TaskNotFoundError, ValueError) as exc:
            raise ImportJobNotFoundError(job_id) from exc
        if task.task_type != TaskType.IMPORT:
            raise ImportJobNotFoundError(job_id)
        return task

    def list_jobs(
        self,
        *,
        case_id: str,
        page: int,
        page_size: int,
        status: Optional[TaskStatus] = None,
    ) -> Tuple[List[TaskRecord], int]:
        return self._task_service.list_public_tasks(
            page=page,
            page_size=page_size,
            status=status,
            task_type=TaskType.IMPORT,
            case_id=case_id,
        )

    def cancel_job(self, job_id: str, *, case_id: str) -> TaskRecord:
        task = self.get_job(job_id, case_id=case_id)
        if task.status in {TaskStatus.SUCCEEDED, TaskStatus.FAILED, TaskStatus.CANCELED}:
            raise ImportJobNotCancellableError(job_id)

        with self._lock:
            flag = self._cancel_flags.setdefault(job_id, threading.Event())
            flag.set()

        if task.status == TaskStatus.QUEUED:
            updated = self._safe_transition(
                job_id,
                to_status=TaskStatus.CANCELED,
                progress=task.progress,
                error=IMPORT_JOB_CANCELLED,
            )
            if updated is not None:
                self._emit_failed_event(
                    job_id=updated.task_id,
                    case_id=updated.case_id,
                    code="JOB_CANCELED",
                    message="import job canceled before execution",
                    retryable=False,
                    details={},
                    dedupe_key="failed:canceled",
                )
                self._cleanup_runtime(updated.task_id)
                return updated

        return self.get_job(job_id, case_id=case_id)

    def list_file_logs(self, *, case_id: str, view: Literal["active", "recycle"] = "active") -> List[dict]:
        rows = self._repository.list_import_files(case_id, view=view)
        projected: List[dict] = []
        for row in rows:
            if not isinstance(row, dict):
                continue
            item = dict(row)
            item["error"] = project_import_error(item.get("error"), status=item.get("status"))
            item["cleaned_error"] = project_cleaning_error(
                item.get("cleaned_error"),
                status=item.get("cleaned_status"),
            )
            projected.append(project_import_file_log_counts(item))
        return projected

    def list_historical_datasets(self, *, case_id: str) -> List[dict]:
        return self._repository.list_historical_datasets(case_id)

    def preview_files(self, *, case_id: str, files: List[dict]) -> List[dict]:
        require_controlled_source_ingestion()
        inputs = [
            self._import_file_input_from_metadata(
                item,
                password=(item.get("password") or None),
            )
            for item in files
        ]
        return [asdict(item) for item in self._repository.preview_files(case_id, inputs)]

    def get_case_paths(self, *, case_id: str) -> dict:
        require_controlled_source_ingestion()
        return self._repository.get_case_paths(case_id)

    def delete_file(self, *, case_id: str, file_id: str) -> None:
        self.recycle_files(case_id=case_id, file_ids=[file_id])

    def delete_files(self, *, case_id: str, file_ids: List[str]) -> List[str]:
        return self.recycle_files(case_id=case_id, file_ids=file_ids)

    def recycle_file(self, *, case_id: str, file_id: str) -> None:
        self.recycle_files(case_id=case_id, file_ids=[file_id])

    def recycle_files(self, *, case_id: str, file_ids: List[str]) -> List[str]:
        normalized = list(dict.fromkeys([str(file_id or "").strip() for file_id in file_ids if str(file_id or "").strip()]))
        if not normalized:
            return []

        existing_ids = {
            str(item.get("file_id") or "").strip()
            for item in self._repository.list_import_files(case_id, view="active")
            if str(item.get("file_id") or "").strip()
        }
        missing = [file_id for file_id in normalized if file_id not in existing_ids]
        if missing:
            raise ImportFileNotFoundError(missing)

        busy = [file_id for file_id in normalized if self._is_file_in_active_job(case_id=case_id, file_id=file_id)]
        if busy:
            raise ImportFileBusyError(busy)

        deleted = self._repository.recycle_import_files(case_id, normalized)
        deleted_ids = {str(file_id or "").strip() for file_id in deleted if str(file_id or "").strip()}
        if len(deleted_ids) != len(normalized):
            raise ImportFileNotFoundError([file_id for file_id in normalized if file_id not in deleted_ids])
        return [file_id for file_id in normalized if file_id in deleted_ids]

    def restore_files(self, *, case_id: str, file_ids: List[str]) -> List[str]:
        normalized = list(dict.fromkeys([str(file_id or "").strip() for file_id in file_ids if str(file_id or "").strip()]))
        if not normalized:
            return []

        existing_ids = {
            str(item.get("file_id") or "").strip()
            for item in self._repository.list_import_files(case_id, view="recycle")
            if str(item.get("file_id") or "").strip()
        }
        missing = [file_id for file_id in normalized if file_id not in existing_ids]
        if missing:
            raise ImportFileNotFoundError(missing)

        restored = self._repository.restore_import_files(case_id, normalized)
        restored_ids = {str(file_id or "").strip() for file_id in restored if str(file_id or "").strip()}
        if len(restored_ids) != len(normalized):
            raise ImportFileNotFoundError([file_id for file_id in normalized if file_id not in restored_ids])
        return [file_id for file_id in normalized if file_id in restored_ids]

    def purge_files(self, *, case_id: str, file_ids: List[str]) -> List[str]:
        normalized = list(dict.fromkeys([str(file_id or "").strip() for file_id in file_ids if str(file_id or "").strip()]))
        if not normalized:
            return []

        existing_ids = {
            str(item.get("file_id") or "").strip()
            for item in self._repository.list_import_files(case_id, view="recycle")
            if str(item.get("file_id") or "").strip()
        }
        missing = [file_id for file_id in normalized if file_id not in existing_ids]
        if missing:
            raise ImportFileNotFoundError(missing)

        purged = self._repository.purge_import_files(case_id, normalized)
        purged_ids = {str(file_id or "").strip() for file_id in purged if str(file_id or "").strip()}
        if len(purged_ids) != len(normalized):
            raise ImportFileNotFoundError([file_id for file_id in normalized if file_id not in purged_ids])
        return [file_id for file_id in normalized if file_id in purged_ids]

    def _run_job(self, job_id: str) -> None:
        # Persisted or directly invoked legacy jobs are untrusted entrypoints too.
        # P2 will replace this quarantine with a host-issued immutable source handle.
        require_controlled_source_ingestion()
        job_started = time.perf_counter()
        job_timing_phases: dict[str, float] = {}
        summary = None

        def record_job_elapsed(phase: str, elapsed_s: float) -> None:
            add_phase(job_timing_phases, phase, elapsed_s)
            if isinstance(summary, dict):
                summary["job_timing"] = self._job_timing_profile(
                    job_timing_phases,
                    wall_s=time.perf_counter() - job_started,
                )

        def record_job_phase(phase: str, phase_started: float) -> None:
            record_job_elapsed(phase, time.perf_counter() - phase_started)

        task = self._get_job(job_id)
        case_id = task.case_id or ""

        if self._is_cancel_requested(job_id):
            self._finalize_canceled(task, message="import job canceled before running")
            return

        started = self._safe_transition(
            job_id,
            to_status=TaskStatus.RUNNING,
            progress=1,
            metadata={"started_at": utc_now().isoformat()},
        )
        if started is None:
            return
        runtime_payload = self._task_service.read_runtime_payload(
            job_id,
            case_id=started.case_id,
        )

        self._emit_event(
            job_id=job_id,
            case_id=case_id,
            event="import.job.progress",
            event_type="progress",
            dedupe_key="progress:running",
            payload={
                "progress": 1,
                "stage": "running",
                "message": "preparing import files",
                "counters": {
                    "requested_files": len(runtime_payload.get("requested_files", [])),
                    "imported_files": 0,
                },
            },
        )

        engine = None
        try:
            phase_started = time.perf_counter()
            with self._lock:
                password_map = dict(self._job_secrets.get(job_id, {}))
            inputs = [
                self._import_file_input_from_metadata(
                    item,
                    password=password_map.get(str(item.get("source_path") or ""), None),
                )
                for item in runtime_payload.get("requested_files", [])
            ]
            if not inputs:
                raise ValueError("no files in import job request")
            record_job_phase("build_inputs", phase_started)

            phase_started = time.perf_counter()
            prepared = self._repository.prepare_files(case_id, inputs)
            record_job_phase("prepare_files", phase_started)
            total_files = len(prepared)
            auto_cleaning = bool(runtime_payload.get("auto_cleaning"))
            summary = self._new_summary(total_files=total_files)
            summary["job_timing"] = self._job_timing_profile(
                job_timing_phases,
                wall_s=time.perf_counter() - job_started,
            )

            files_summary: List[dict] = [self._prepared_file_snapshot(item) for item in prepared]
            persistence_units: List[dict] = []
            imported_files = 0
            failed_files = 0

            phase_started = time.perf_counter()
            self._safe_transition(
                job_id,
                to_status=TaskStatus.RUNNING,
                progress=2,
                metadata={
                    "prepared_files": total_files,
                    "files": files_summary,
                    "summary": summary,
                    "imported_files": imported_files,
                    "failed_files": failed_files,
                    "current_file": "",
                },
            )
            record_job_phase("metadata_prepared", phase_started)

            phase_started = time.perf_counter()
            engine = self._repository.open_case_engine(case_id)
            self._repository.ensure_tables(engine, include_secondary_indexes=False)
            record_job_phase("open_engine_prepare_tables", phase_started)

            index = 1
            while index <= total_files:
                unit = plan_next_import_unit(
                    prepared,
                    index - 1,
                    group_key=self._repository.prepared_import_group_key,
                    file_limit=self._IMPORT_TRANSACTION_BATCH_FILES,
                    row_limit=self._IMPORT_TRANSACTION_BATCH_ROWS,
                )
                item = unit[0]
                if self._is_cancel_requested(job_id):
                    self._set_file_status(
                        files_summary,
                        item.file_id,
                        status="canceled",
                        error="canceled by user",
                    )
                    summary["persistence"] = self._combine_persistence_units(
                        case_id=case_id,
                        files_summary=files_summary,
                        persistence_units=persistence_units,
                    )
                    self._safe_transition(
                        job_id,
                        to_status=TaskStatus.RUNNING,
                        progress=max(
                            1,
                            self._calc_import_progress(
                                index=index,
                                total=total_files,
                                sub_progress=0,
                                auto_cleaning=auto_cleaning,
                            ),
                        ),
                        metadata={
                            "files": files_summary,
                            "summary": summary,
                            "imported_files": imported_files,
                            "failed_files": failed_files,
                            "current_file": item.display_name,
                        },
                    )
                    self._finalize_canceled(
                        self._get_job(job_id),
                        message="import job canceled by user",
                        summary=summary,
                    )
                    return

                phase_started = time.perf_counter()
                start_progress = self._calc_import_progress(
                    index=index,
                    total=total_files,
                    sub_progress=0,
                    auto_cleaning=auto_cleaning,
                )
                for unit_item in unit:
                    self._set_file_status(files_summary, unit_item.file_id, status="running")
                self._safe_transition(
                    job_id,
                    to_status=TaskStatus.RUNNING,
                    progress=start_progress,
                    metadata={
                        "files": files_summary,
                        "current_file": item.display_name,
                    },
                )
                unit_end_index = index + len(unit) - 1
                start_message = (
                    f"importing files {index}-{unit_end_index}/{total_files}"
                    if len(unit) > 1
                    else f"importing file {index}/{total_files}"
                )
                self._emit_event(
                    job_id=job_id,
                    case_id=case_id,
                    event="import.job.progress",
                    event_type="progress",
                    payload={
                        "progress": start_progress,
                        "stage": "file_start",
                        "message": start_message,
                        "counters": {
                            "file_index": index,
                            "file_end_index": unit_end_index,
                            "total_files": total_files,
                            "imported_files": imported_files,
                        },
                    },
                    dedupe_key=f"progress:file_start:{index}",
                )
                record_job_phase("metadata_file_start", phase_started)

                phase_started = time.perf_counter()
                if len(unit) > 1:
                    results, unit_persistence = self._run_file_group_with_retry(
                        engine=engine,
                        case_id=case_id,
                        job_id=job_id,
                        items=unit,
                        index=index,
                        total_files=total_files,
                        auto_cleaning=auto_cleaning,
                    )
                else:
                    result, unit_persistence = self._run_file_with_retry(
                        engine=engine,
                        case_id=case_id,
                        job_id=job_id,
                        item=item,
                        index=index,
                        total_files=total_files,
                        auto_cleaning=auto_cleaning,
                    )
                    results = [result]
                record_job_phase("import_units", phase_started)

                if len(results) != len(unit):
                    raise RuntimeError("grouped import returned mismatched result count")
                if unit_persistence is not None:
                    persistence_units.append(unit_persistence)

                if any(result.status == "canceled" for result in results):
                    for result in results:
                        self._merge_file_result(files_summary, result)
                    summary["persistence"] = self._combine_persistence_units(
                        case_id=case_id,
                        files_summary=files_summary,
                        persistence_units=persistence_units,
                    )
                    self._safe_transition(
                        job_id,
                        to_status=TaskStatus.RUNNING,
                        progress=max(
                            1,
                            self._calc_import_progress(
                                index=index,
                                total=total_files,
                                sub_progress=0,
                                auto_cleaning=auto_cleaning,
                            ),
                        ),
                        metadata={
                            "files": files_summary,
                            "summary": summary,
                            "imported_files": imported_files,
                            "failed_files": failed_files,
                            "current_file": item.display_name,
                        },
                    )
                    self._finalize_canceled(
                        self._get_job(job_id),
                        message="import job canceled by user",
                        summary=summary,
                    )
                    return

                phase_started = time.perf_counter()
                for offset, result in enumerate(results):
                    current_index = index + offset
                    current_item = unit[offset]
                    self._merge_file_result(files_summary, result)
                    self._accumulate(summary, result)
                    if result.status == "succeeded":
                        imported_files += 1
                    elif result.status == "failed":
                        failed_files += 1

                    done_progress = self._calc_import_progress(
                        index=current_index,
                        total=total_files,
                        sub_progress=100,
                        auto_cleaning=auto_cleaning,
                    )
                    self._safe_transition(
                        job_id,
                        to_status=TaskStatus.RUNNING,
                        progress=done_progress,
                        metadata={
                            "files": files_summary,
                            "summary": summary,
                            "imported_files": imported_files,
                            "failed_files": failed_files,
                            "current_file": current_item.display_name,
                        },
                    )

                    self._emit_event(
                        job_id=job_id,
                        case_id=case_id,
                        event="import.job.progress",
                        event_type="progress",
                        payload={
                            "progress": done_progress,
                            "stage": "file_done",
                            "message": f"file {current_index}/{total_files} finished",
                            "counters": {
                                "file_index": current_index,
                                "total_files": total_files,
                                "imported_files": imported_files,
                                "failed_files": failed_files,
                                "rows_imported_norm": summary["rows_imported_norm"],
                                "rows_dedup": summary["rows_dedup"],
                            },
                        },
                        dedupe_key=f"progress:file_done:{current_index}",
                    )
                record_job_phase("metadata_file_done", phase_started)

                index += len(unit)

            phase_started = time.perf_counter()
            summary["persistence"] = self._combine_persistence_units(
                case_id=case_id,
                files_summary=files_summary,
                persistence_units=persistence_units,
            )
            if self._summary_has_imported_rows(summary):
                self._repository.mark_analysis_dirty(engine=engine, reason=f"import-job:{job_id}")
            record_job_phase("aggregate_import_persistence", phase_started)

            phase_started = time.perf_counter()
            if self._summary_has_imported_rows(summary):
                self._repository.ensure_tables(engine, include_secondary_indexes=True)
            record_job_phase("secondary_indexes", phase_started)

            if auto_cleaning and imported_files > 0:
                phase_started = time.perf_counter()
                if engine is not None:
                    engine.close()
                    engine = None
                cleaning_summary = self._run_auto_cleaning(job_id=job_id, case_id=case_id)
                summary["auto_cleaning"] = {
                    "enabled": True,
                    "status": "succeeded",
                    "cleaned_rows": self._verified_auto_cleaning_count(cleaning_summary),
                    "summary": cleaning_summary,
                }
                record_job_phase("auto_cleaning", phase_started)
                analysis_timing, refresh_phases = self._refresh_case_stats_and_analysis(
                    case_id,
                    engine=None,
                    refresh_analysis=True,
                )
            else:
                summary["auto_cleaning"] = {
                    "enabled": bool(auto_cleaning),
                    "status": "skipped",
                    "cleaned_rows": 0,
                    "summary": {},
                }
                if engine is not None:
                    phase_started = time.perf_counter()
                    try:
                        engine.close()
                    finally:
                        engine = None
                    record_job_phase("close_engine_before_refresh", phase_started)
                analysis_timing, refresh_phases = self._refresh_case_stats_and_analysis(
                    case_id,
                    engine=None,
                    refresh_analysis=False,
                )
            if analysis_timing:
                summary["analysis_timing"] = analysis_timing
            for phase, elapsed_s in refresh_phases.items():
                record_job_elapsed(phase, elapsed_s)
            if engine is not None:
                phase_started = time.perf_counter()
                try:
                    engine.close()
                except Exception:
                    pass
                engine = None
                record_job_phase("close_engine_before_finalize", phase_started)
            self._finalize_done(
                job_id=job_id,
                case_id=case_id,
                imported_files=imported_files,
                failed_files=failed_files,
                summary=summary,
                files_summary=files_summary,
            )
        except CleaningCancelled:
            self._finalize_canceled(
                self._get_job(job_id),
                message="import job canceled during auto cleaning",
                summary=summary if isinstance(summary, dict) else None,
            )
        except Exception as exc:
            retryable = private_exception_is_retryable(exc)
            updated = self._safe_transition(
                job_id,
                to_status=TaskStatus.FAILED,
                progress=100,
                error=IMPORT_JOB_FAILED,
            )
            if updated is not None:
                self._emit_failed_event(
                    job_id=job_id,
                    case_id=case_id,
                    code="INTERNAL_ERROR",
                    message="import job failed",
                    retryable=retryable,
                    details={},
                    dedupe_key="failed:internal",
                )
        finally:
            if engine is not None:
                try:
                    engine.close()
                except Exception:
                    pass
            self._cleanup_runtime(job_id)

    @staticmethod
    def _import_file_input_from_metadata(item: dict, *, password: Optional[str]) -> ImportFileInput:
        return ImportFileInput(
            file_name=str(item.get("file_name") or ""),
            source_path=str(item.get("source_path") or ""),
            file_kind=(item.get("file_kind") or None),
            password=password,
            expected_sha256=(item.get("expected_sha256") or None),
            expected_size=(item.get("expected_size") if item.get("expected_size") is not None else None),
            field_mapping=item.get("field_mapping") if isinstance(item.get("field_mapping"), dict) else None,
            field_mapping_origins=(
                item.get("field_mapping_origins")
                if isinstance(item.get("field_mapping_origins"), dict)
                else None
            ),
            archive_items=[
                ImportArchiveItemInput(
                    archive_path=str(child.get("archive_path") or ""),
                    file_kind=(child.get("file_kind") or None),
                    expected_sha256=(child.get("expected_sha256") or None),
                    expected_size=(
                        child.get("expected_size")
                        if child.get("expected_size") is not None
                        else None
                    ),
                    field_mapping=(child.get("field_mapping") if isinstance(child.get("field_mapping"), dict) else None),
                    field_mapping_origins=(
                        child.get("field_mapping_origins")
                        if isinstance(child.get("field_mapping_origins"), dict)
                        else None
                    ),
                )
                for child in (item.get("archive_items") or [])
                if isinstance(child, dict) and str(child.get("archive_path") or "").strip()
            ]
            or None,
        )

    def _is_file_in_active_job(self, *, case_id: str, file_id: str) -> bool:
        active_status = (TaskStatus.QUEUED, TaskStatus.RUNNING)
        for status in active_status:
            tasks, _ = self._task_service.list_tasks(
                page=1,
                page_size=2000,
                status=status,
                task_type=TaskType.IMPORT,
                case_id=case_id,
            )
            for task in tasks:
                runtime_payload = self._task_service.read_runtime_payload(
                    task.task_id,
                    case_id=task.case_id,
                )
                files = runtime_payload.get("files")
                if isinstance(files, list):
                    for item in files:
                        if isinstance(item, dict) and str(item.get("file_id") or "") == file_id:
                            return True
                requested = runtime_payload.get("requested_files")
                if isinstance(requested, list):
                    # requested_files does not carry file_id until prepare_files, so keep source-path keyed secrets until then.
                    continue
        return False

    def _run_file_with_retry(
        self,
        *,
        engine,
        case_id: str,
        job_id: str,
        item,
        index: int,
        total_files: int,
        auto_cleaning: bool,
    ) -> tuple[ImportExecutionResult, Optional[dict]]:
        results, persistence = self._run_import_unit_with_retry(
            engine=engine,
            case_id=case_id,
            job_id=job_id,
            items=[item],
            index=index,
            total_files=total_files,
            auto_cleaning=auto_cleaning,
        )
        if len(results) != 1:
            raise RuntimeError("single import returned mismatched result count")
        return results[0], persistence

    def _run_file_group_with_retry(
        self,
        *,
        engine,
        case_id: str,
        job_id: str,
        items: List[PreparedImportFile],
        index: int,
        total_files: int,
        auto_cleaning: bool,
    ) -> tuple[List[ImportExecutionResult], Optional[dict]]:
        return self._run_import_unit_with_retry(
            engine=engine,
            case_id=case_id,
            job_id=job_id,
            items=items,
            index=index,
            total_files=total_files,
            auto_cleaning=auto_cleaning,
        )

    def _run_import_unit_with_retry(
        self,
        *,
        engine,
        case_id: str,
        job_id: str,
        items: List[PreparedImportFile],
        index: int,
        total_files: int,
        auto_cleaning: bool,
    ) -> tuple[List[ImportExecutionResult], Optional[dict]]:
        if not items:
            raise ValueError("import unit requires at least one file")
        attempts = 0
        max_attempts = self._max_retries_per_file + 1

        while attempts < max_attempts:
            attempts += 1
            if self._is_cancel_requested(job_id):
                return self._closed_unit_results(
                    items,
                    status="canceled",
                    error=IMPORT_FILE_CANCELLED,
                    attempts=attempts,
                    retryable=False,
                ), None

            transaction_open = False
            retryable = False
            failed_results: List[ImportExecutionResult]
            try:
                engine.execute("BEGIN TRANSACTION")
                transaction_open = True
                if len(items) == 1:
                    results = [
                        self._repository.run_file_import(
                            engine=engine,
                            case_id=case_id,
                            item=items[0],
                            attempts=attempts,
                            progress_cb=lambda snapshot: self._on_file_progress(
                                job_id=job_id,
                                case_id=case_id,
                                index=index,
                                total_files=total_files,
                                auto_cleaning=auto_cleaning,
                                snapshot=snapshot,
                            ),
                        )
                    ]
                else:
                    results = self._repository.run_file_import_group(
                        engine=engine,
                        case_id=case_id,
                        items=items,
                        attempts=attempts,
                    )
                if len(results) != len(items):
                    raise RuntimeError("import unit returned mismatched result count")

                if self._is_cancel_requested(job_id):
                    self._rollback_import_transaction(engine)
                    transaction_open = False
                    return self._closed_unit_results(
                        items,
                        status="canceled",
                        error=IMPORT_FILE_CANCELLED,
                        attempts=attempts,
                        retryable=False,
                    ), None

                if not all(result.status == "succeeded" for result in results):
                    retryable = any(bool(result.retryable) for result in results)
                    self._rollback_import_transaction(engine)
                    transaction_open = False
                    failed_results = self._closed_unit_results(
                        items,
                        status="failed",
                        error=IMPORT_FILE_PROCESSING_FAILED,
                        attempts=attempts,
                        retryable=retryable,
                    )
                else:
                    validation_summary: dict = {}
                    persistence = self._validate_import_persistence(
                        case_id=case_id,
                        summary=validation_summary,
                        files_summary=[asdict(result) for result in results],
                        engine=engine,
                    )
                    if not self._commit_import_transaction_if_not_canceled(
                        engine,
                        job_id=job_id,
                    ):
                        self._rollback_import_transaction(engine)
                        transaction_open = False
                        return self._closed_unit_results(
                            items,
                            status="canceled",
                            error=IMPORT_FILE_CANCELLED,
                            attempts=attempts,
                            retryable=False,
                        ), None
                    transaction_open = False
                    return results, persistence
            except Exception as exc:
                if transaction_open:
                    self._rollback_import_transaction(engine)
                    transaction_open = False
                retryable = private_exception_is_retryable(exc)
                failed_results = self._closed_unit_results(
                    items,
                    status="failed",
                    error=IMPORT_FILE_PROCESSING_FAILED,
                    attempts=attempts,
                    retryable=retryable,
                )

            if not retryable or attempts >= max_attempts:
                return failed_results, None

            self._emit_event(
                job_id=job_id,
                case_id=case_id,
                event="import.job.progress",
                event_type="progress",
                payload={
                    "progress": self._calc_import_progress(
                        index=index,
                        total=total_files,
                        sub_progress=95,
                        auto_cleaning=auto_cleaning,
                    ),
                    "stage": "retrying",
                    "message": (
                        f"retrying file {items[0].display_name}"
                        if len(items) == 1
                        else f"retrying grouped import {index}-{index + len(items) - 1}"
                    ),
                    "counters": {
                        "file_index": index,
                        "file_end_index": index + len(items) - 1,
                        "total_files": total_files,
                        "attempt": attempts + 1,
                    },
                },
                dedupe_key=f"progress:retry:unit:{items[0].file_id}:{attempts}",
            )
            time.sleep(min(0.5 * attempts, 2.0))

        raise RuntimeError("import retry loop did not produce a result")

    @staticmethod
    def _rollback_import_transaction(engine) -> None:
        try:
            engine.execute("ROLLBACK")
        except Exception as exc:
            raise RuntimeError("import transaction rollback failed") from exc

    def _commit_import_transaction_if_not_canceled(self, engine, *, job_id: str) -> bool:
        with self._lock:
            if self._is_cancel_requested(job_id):
                return False
            engine.execute("COMMIT")
            return True

    @staticmethod
    def _closed_unit_results(
        items: List[PreparedImportFile],
        *,
        status: Literal["failed", "canceled"],
        error: str,
        attempts: int,
        retryable: bool,
    ) -> List[ImportExecutionResult]:
        return [
            ImportExecutionResult(
                file_id=item.file_id,
                display_name=item.display_name,
                display_path=item.display_path,
                file_type=item.file_type,
                size=item.size,
                md5="",
                sha256=item.sha256 or "",
                kind=item.kind_hint or "",
                status=status,
                rows_total=None,
                rows_seen=None,
                rows_imported_raw=None,
                rows_imported_norm=None,
                rows_dedup=None,
                rows_error=None,
                note="",
                error=error,
                attempts=attempts,
                rows_skipped_non_data=None,
                retryable=retryable,
            )
            for item in items
        ]

    @staticmethod
    def _prepared_file_snapshot(item: PreparedImportFile) -> dict:
        size = known_public_import_count(item.size)
        source_size = known_public_import_count(item.source_size)
        if size is None:
            raise RuntimeError("prepared import size is unavailable")
        if source_size is None:
            raise RuntimeError("prepared import source_size is unavailable")
        return {
            "file_id": item.file_id,
            "display_name": item.display_name,
            "display_path": item.display_path,
            "file_type": item.file_type,
            "size": size,
            "md5": "",
            "sha256": item.sha256 or "",
            "source_sha256": item.source_sha256 or "",
            "source_size": source_size,
            "kind": item.kind_hint or "",
            "status": "queued",
            "rows_total": None,
            "rows_seen": None,
            "rows_imported_raw": None,
            "rows_imported_norm": None,
            "rows_dedup": None,
            "rows_error": None,
            "rows_skipped_non_data": None,
            "note": "",
            "error": "",
            "attempts": 0,
        }

    @staticmethod
    def _merge_file_result(files_summary: List[dict], result: ImportExecutionResult) -> None:
        payload = asdict(result)
        payload["error"] = project_import_error(payload.get("error"), status=payload.get("status"))
        payload["note"] = project_import_note(payload.get("note"))
        for index, item in enumerate(files_summary):
            if str(item.get("file_id") or "") == result.file_id:
                merged = dict(item)
                merged.update(payload)
                files_summary[index] = merged
                return
        files_summary.append(payload)

    @staticmethod
    def _set_file_status(files_summary: List[dict], file_id: str, *, status: str, error: str = "") -> None:
        for index, item in enumerate(files_summary):
            if str(item.get("file_id") or "") != file_id:
                continue
            next_item = dict(item)
            next_item["status"] = status
            if error:
                next_item["error"] = project_import_error(error, status=status)
            files_summary[index] = next_item
            return

    def _on_file_progress(
        self,
        *,
        job_id: str,
        case_id: str,
        index: int,
        total_files: int,
        auto_cleaning: bool,
        snapshot: dict,
    ) -> None:
        rows_seen = self._optional_verified_count(snapshot.get("rows_seen"), field="rows_seen")
        rows_total = self._optional_verified_count(snapshot.get("rows_total"), field="rows_total")
        local_progress = 0
        if rows_seen is not None and rows_total is not None and rows_total > 0:
            local_progress = max(0, min(100, int(rows_seen * 100 / rows_total)))
        progress = self._calc_import_progress(
            index=index,
            total=total_files,
            sub_progress=local_progress,
            auto_cleaning=auto_cleaning,
        )

        self._safe_transition(
            job_id,
            to_status=TaskStatus.RUNNING,
            progress=progress,
        )

        self._emit_event(
            job_id=job_id,
            case_id=case_id,
            event="import.job.progress",
            event_type="progress",
            payload={
                "progress": progress,
                "stage": "importing",
                "message": f"processing {snapshot.get('display_name') or ''}".strip(),
                "counters": {
                    "file_index": index,
                    "total_files": total_files,
                    "rows_seen": rows_seen,
                    "rows_imported_raw": self._optional_verified_count(
                        snapshot.get("rows_imported_raw"),
                        field="rows_imported_raw",
                    ),
                    "rows_imported_norm": self._optional_verified_count(
                        snapshot.get("rows_imported_norm"),
                        field="rows_imported_norm",
                    ),
                    "rows_dedup": self._optional_verified_count(
                        snapshot.get("rows_dedup"),
                        field="rows_dedup",
                    ),
                    "rows_skipped_non_data": self._optional_verified_count(
                        snapshot.get("rows_skipped_non_data"),
                        field="rows_skipped_non_data",
                    ),
                },
            },
            dedupe_key=(
                f"progress:importing:{snapshot.get('file_id')}:{rows_seen}:"
                f"{self._optional_verified_count(snapshot.get('rows_imported_norm'), field='rows_imported_norm')}"
            ),
        )

    @staticmethod
    def _optional_verified_count(value: object, *, field: str) -> Optional[int]:
        if value is None:
            return None
        normalized = known_public_import_count(value)
        if normalized is None:
            raise ValueError(f"import_progress_{field}_invalid")
        return normalized

    @staticmethod
    def _verified_auto_cleaning_count(summary: object) -> int:
        if not isinstance(summary, dict):
            raise RuntimeError("auto cleaning row count is unavailable")
        try:
            total_rows = _required_persistence_count(
                summary.get("total_rows"),
                field="auto_cleaning.total_rows",
            )
            scope_rows = _required_persistence_count(
                summary.get("scope_rows"),
                field="auto_cleaning.scope_rows",
            )
        except RuntimeError as exc:
            raise RuntimeError("auto cleaning row count is unavailable") from exc
        if total_rows != scope_rows:
            raise RuntimeError("auto cleaning row count is inconsistent")
        return total_rows

    def _run_auto_cleaning(self, *, job_id: str, case_id: str) -> dict:
        self._safe_transition(
            job_id,
            to_status=TaskStatus.RUNNING,
            progress=91,
            metadata={"auto_cleaning_status": "running"},
        )
        self._emit_event(
            job_id=job_id,
            case_id=case_id,
            event="import.job.progress",
            event_type="progress",
            payload={
                "progress": 91,
                "stage": "auto_cleaning",
                "message": "running auto cleaning",
                "counters": {},
            },
            dedupe_key="progress:auto_cleaning:start",
        )

        started = time.perf_counter()
        summary = self._cleaning_repository.run_cleaning(
            case_id=case_id,
            file_ids=None,
            steps=None,
            cancel_check=lambda: self._is_cancel_requested(job_id),
            progress_cb=lambda progress, message, step: self._on_auto_cleaning_progress(
                job_id=job_id,
                case_id=case_id,
                progress=progress,
                message=message,
                step=step,
            ),
        )
        payload = dict(summary or {})
        payload["duration_s"] = round(time.perf_counter() - started, 6)
        self._safe_transition(
            job_id,
            to_status=TaskStatus.RUNNING,
            progress=99,
            metadata={"auto_cleaning_status": "succeeded", "auto_cleaning_summary": payload},
        )
        return payload

    def _on_auto_cleaning_progress(
        self,
        *,
        job_id: str,
        case_id: str,
        progress: int,
        message: str,
        step: Optional[int],
    ) -> None:
        local_progress = max(0, min(100, int(progress or 0)))
        overall_progress = max(91, min(99, 90 + int(local_progress * 9 / 100)))
        step_label = f"auto_cleaning_step_{step}" if step else "auto_cleaning"
        self._safe_transition(job_id, to_status=TaskStatus.RUNNING, progress=overall_progress)
        self._emit_event(
            job_id=job_id,
            case_id=case_id,
            event="import.job.progress",
            event_type="progress",
            payload={
                "progress": overall_progress,
                "stage": step_label,
                "message": message,
                "counters": {
                    "step": int(step or 0),
                    "cleaning_progress": local_progress,
                },
            },
            dedupe_key=f"progress:auto_cleaning:{step_label}:{overall_progress}:{message}",
        )

    def _finalize_done(
        self,
        *,
        job_id: str,
        case_id: str,
        imported_files: int,
        failed_files: int,
        summary: dict,
        files_summary: List[dict],
    ) -> None:
        message = "import job completed"
        status = TaskStatus.SUCCEEDED
        error = None
        if imported_files == 0 and failed_files > 0:
            status = TaskStatus.FAILED
            message = "all files failed to import"
            error = IMPORT_JOB_FAILED

        updated = self._safe_transition(
            job_id,
            to_status=status,
            progress=100,
            error=error,
            metadata={
                "imported_files": imported_files,
                "failed_files": failed_files,
                "summary": summary,
                "files": files_summary,
                "finished_at": utc_now().isoformat(),
            },
        )
        if updated is None:
            return

        if status == TaskStatus.SUCCEEDED:
            counters = {
                "imported_files": imported_files,
                "failed_files": failed_files,
                "rows_total": summary["rows_total"],
                "rows_imported_raw": summary["rows_imported_raw"],
                "rows_imported_norm": summary["rows_imported_norm"],
                "rows_dedup": summary["rows_dedup"],
                "rows_error": summary["rows_error"],
                "rows_skipped_non_data": summary["rows_skipped_non_data"],
            }
            auto_summary = summary.get("auto_cleaning") if isinstance(summary, dict) else None
            if isinstance(auto_summary, dict) and auto_summary.get("status") == "succeeded":
                counters["cleaned_rows"] = _required_persistence_count(
                    auto_summary.get("cleaned_rows"),
                    field="auto_cleaning.cleaned_rows",
                )
            self._emit_event(
                job_id=job_id,
                case_id=case_id,
                event="import.job.completed",
                event_type="success",
                dedupe_key="completed",
                payload={
                    "progress": 100,
                    "stage": "completed",
                    "message": message,
                    "counters": counters,
                },
            )
            if self._should_warm_native_cleaning_worker(summary):
                self._warm_native_cleaning_worker_async()
        else:
            self._emit_failed_event(
                job_id=job_id,
                case_id=case_id,
                code="INTERNAL_ERROR",
                message=message,
                retryable=False,
                details={
                    "imported_files": imported_files,
                    "failed_files": failed_files,
                },
                dedupe_key="failed:all-files",
            )

    @staticmethod
    def _should_warm_native_cleaning_worker(summary: dict) -> bool:
        auto_summary = summary.get("auto_cleaning") if isinstance(summary, dict) else None
        if isinstance(auto_summary, dict) and auto_summary.get("status") == "succeeded":
            return False
        return int((summary or {}).get("rows_imported_norm") or 0) > 0

    def _warm_native_cleaning_worker_async(self) -> None:
        thread = threading.Thread(
            target=self._warm_native_cleaning_worker,
            daemon=True,
            name="cleaning-worker-warmup",
        )
        thread.start()

    @staticmethod
    def _warm_native_cleaning_worker() -> None:
        try:
            cleaning_native_runtime.warm_native_cleaning_worker()
        except Exception:
            pass

    def _finalize_canceled(
        self,
        task: TaskRecord,
        *,
        message: str,
        summary: Optional[dict] = None,
    ) -> None:
        runtime_payload = self._task_service.read_runtime_payload(
            task.task_id,
            case_id=task.case_id,
        )
        updated = self._safe_transition(
            task.task_id,
            to_status=TaskStatus.CANCELED,
            progress=max(task.progress, 1),
            error=IMPORT_JOB_CANCELLED,
            metadata={"summary": summary or runtime_payload.get("summary", {})},
        )
        if updated is None:
            return

        self._emit_failed_event(
            job_id=task.task_id,
            case_id=task.case_id,
            code="JOB_CANCELED",
            message=message,
            retryable=False,
            details={},
            dedupe_key="failed:canceled",
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
        with self._lock:
            self._threads.pop(job_id, None)
            self._cancel_flags.pop(job_id, None)
            self._job_event_seq.pop(job_id, None)
            self._event_dedup.pop(job_id, None)
            self._job_secrets.pop(job_id, None)

    def _refresh_case_stats_and_analysis(
        self,
        case_id: str,
        *,
        engine,
        refresh_analysis: bool,
    ) -> tuple[dict, dict[str, float]]:
        phases: dict[str, float] = {}
        phase_started = time.perf_counter()
        self._repository.refresh_case_stats(case_id, engine=engine)
        phases["refresh_case_stats"] = time.perf_counter() - phase_started

        if not refresh_analysis:
            return {}, phases

        phase_started = time.perf_counter()
        analysis_timing: dict = {}
        self._repository.refresh_analysis_aggregates(
            case_id,
            engine=engine,
            profile_cb=lambda profile: analysis_timing.update(profile or {}),
        )
        phases["refresh_analysis_aggregates"] = time.perf_counter() - phase_started
        return analysis_timing, phases

    @staticmethod
    def _calc_import_progress(*, index: int, total: int, sub_progress: int, auto_cleaning: bool) -> int:
        progress = ImportService._calc_progress(index=index, total=total, sub_progress=sub_progress)
        if not auto_cleaning:
            return progress
        return max(0, min(90, int(progress * 0.9)))

    @staticmethod
    def _calc_progress(*, index: int, total: int, sub_progress: int) -> int:
        if total <= 0:
            return max(0, min(100, sub_progress))
        base = (max(index, 1) - 1) / total
        pct = base * 100.0 + (max(0, min(100, sub_progress)) / total)
        return max(0, min(100, int(pct)))

    @staticmethod
    def _new_summary(*, total_files: int) -> dict:
        return {
            "total_files": int(total_files),
            "rows_total": None,
            "rows_seen": None,
            "rows_imported_raw": None,
            "rows_imported_norm": None,
            "rows_dedup": None,
            "rows_error": None,
            "rows_skipped_non_data": None,
            "retry_count": 0,
            "import_timing": {},
            "job_timing": {},
            "analysis_timing": {},
        }

    @staticmethod
    def _accumulate(summary: dict, result: ImportExecutionResult) -> None:
        attempts = known_public_import_count(result.attempts)
        if attempts is None or attempts == 0:
            raise ValueError("import_result_attempts_invalid")
        if result.status != "succeeded":
            summary["retry_count"] += attempts - 1
            return
        fields = (
            "rows_total",
            "rows_seen",
            "rows_imported_raw",
            "rows_imported_norm",
            "rows_dedup",
            "rows_error",
            "rows_skipped_non_data",
        )
        verified_counts = {
            field: _required_persistence_count(getattr(result, field), field=field)
            for field in fields
        }
        current_counts = {
            field: (
                None
                if summary.get(field) is None
                else _required_persistence_count(summary.get(field), field=field)
            )
            for field in fields
        }
        summary["retry_count"] += attempts - 1
        for field in fields:
            value = verified_counts[field]
            current = current_counts[field]
            if current is None:
                summary[field] = value
            else:
                summary[field] = current + value
        summary["import_timing"] = merge_timing_profiles(
            [
                summary.get("import_timing") or {},
                result.timings or {},
            ]
        )

    @staticmethod
    def _summary_has_imported_rows(summary: dict) -> bool:
        for field in ("rows_total", "rows_imported_raw"):
            raw_value = summary.get(field)
            if raw_value is None:
                continue
            value = known_public_import_count(raw_value)
            if value is None:
                raise ValueError(f"import_summary_{field}_invalid")
            if value > 0:
                return True
        return False

    @staticmethod
    def _job_timing_profile(phases: dict[str, float], *, wall_s: float | None = None) -> dict:
        rounded_phases = {key: round(float(value or 0), 6) for key, value in sorted(dict(phases or {}).items())}
        out = {
            "phases_s": rounded_phases,
            "total_phase_s": round(sum(rounded_phases.values()), 6),
        }
        if wall_s is not None:
            out["wall_s"] = round(max(0.0, float(wall_s or 0)), 6)
        return out

    def _emit_event(
        self,
        *,
        job_id: str,
        case_id: Optional[str],
        event: str,
        event_type: str,
        payload: dict,
        dedupe_key: Optional[str] = None,
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
            channel="import",
            sequence=next(self._ws_sequence),
            version=self._ws_version,
            job_id=job_id,
            case_id=case_id,
            payload=payload_obj,
        )
        self._ws_manager.publish_threadsafe(envelope)

    def _emit_failed_event(
        self,
        *,
        job_id: str,
        case_id: Optional[str],
        code: str,
        message: str,
        retryable: bool,
        details: dict,
        dedupe_key: str,
    ) -> None:
        correlation_id = ""
        try:
            task = self._task_service.get_task(job_id)
            correlation_id = str(task.metadata.get("failure_correlation_id") or "")
        except TaskNotFoundError:
            pass
        self._emit_event(
            job_id=job_id,
            case_id=case_id,
            event="import.job.failed",
            event_type="error",
            dedupe_key=dedupe_key,
            payload=closed_task_failure_event_payload(
                service="import",
                code=code,
                retryable=retryable,
                correlation_id=correlation_id,
                details=details,
            ),
        )

    def _validate_import_persistence(
        self,
        *,
        case_id: str,
        summary: dict,
        files_summary: List[dict],
        engine=None,
    ) -> dict:
        successful_files = [
            item
            for item in files_summary
            if str(item.get("status") or "").strip().lower() == "succeeded"
            and str(item.get("file_id") or "").strip()
        ]
        if not successful_files:
            return {
                "case_id": case_id,
                "file_ids": [],
                "import_file_log_count": 0,
                "rows_imported_norm": 0,
                "norm_rows_by_kind": {},
            }

        file_ids = [str(item.get("file_id") or "").strip() for item in successful_files]
        expected_log_count = len(file_ids)
        expected_rows_norm = sum(
            _required_persistence_count(item.get("rows_imported_norm"), field="rows_imported_norm")
            for item in successful_files
        )
        expected_norm_by_kind: dict[str, int] = {}
        normalized_source_kinds = {
            "fc_account",
            "fc_coercive_measure",
            "fc_person",
            "fc_person_address",
            "fc_person_contact",
            "fc_sub_account",
            "fc_task_fail",
            "fc_task_success",
            "fc_transaction",
        }
        for item in successful_files:
            kind = str(item.get("kind") or "").strip()
            if kind not in normalized_source_kinds and kind != "support_file":
                raise RuntimeError("import persistence source kind is unavailable")
            if kind in normalized_source_kinds:
                expected_norm_by_kind[kind] = expected_norm_by_kind.get(kind, 0) + _required_persistence_count(
                    item.get("rows_imported_norm"), field="rows_imported_norm"
                )

        persistence = self._repository.validate_import_persistence(
            case_id,
            file_ids=file_ids,
            engine=engine,
        )
        if not isinstance(persistence, dict):
            raise RuntimeError("import persistence validation result is unavailable")
        summary["persistence"] = persistence

        mismatches: list[str] = []
        actual_log_count = _required_persistence_count(
            persistence.get("import_file_log_count"), field="import_file_log_count"
        )
        if actual_log_count != expected_log_count:
            mismatches.append(f"import_file_log_count {actual_log_count} != expected {expected_log_count}")

        actual_rows_norm = _required_persistence_count(
            persistence.get("rows_imported_norm"), field="rows_imported_norm"
        )
        if actual_rows_norm != expected_rows_norm:
            mismatches.append(f"rows_imported_norm {actual_rows_norm} != expected {expected_rows_norm}")

        actual_norm_by_kind = persistence.get("norm_rows_by_kind")
        if not isinstance(actual_norm_by_kind, dict):
            raise RuntimeError("import persistence norm_rows_by_kind is unavailable")
        for kind, expected_rows in sorted(expected_norm_by_kind.items()):
            actual_rows = _required_persistence_count(
                actual_norm_by_kind.get(kind), field=f"{kind}_norm_rows"
            )
            if actual_rows != expected_rows:
                mismatches.append(f"{kind}_norm_rows {actual_rows} != expected {expected_rows}")

        if mismatches:
            raise RuntimeError("import persistence validation failed: " + "; ".join(mismatches))
        return persistence

    @staticmethod
    def _combine_persistence_units(
        *,
        case_id: str,
        files_summary: List[dict],
        persistence_units: List[dict],
    ) -> dict:
        expected_files = {
            str(item.get("file_id") or "").strip(): item
            for item in files_summary
            if str(item.get("status") or "").strip().lower() == "succeeded"
            and str(item.get("file_id") or "").strip()
        }
        seen: set[str] = set()
        rows_imported_norm = 0
        norm_rows_by_kind: dict[str, int] = {}
        for persistence in persistence_units:
            if not isinstance(persistence, dict) or persistence.get("case_id") != case_id:
                raise RuntimeError("import persistence unit binding is unavailable")
            file_ids = persistence.get("file_ids")
            if not isinstance(file_ids, list) or any(
                type(file_id) is not str or not file_id.strip() for file_id in file_ids
            ):
                raise RuntimeError("import persistence unit file ids are unavailable")
            normalized_ids = [file_id.strip() for file_id in file_ids]
            if len(normalized_ids) != len(set(normalized_ids)) or seen.intersection(normalized_ids):
                raise RuntimeError("import persistence unit file ids overlap")
            unit_log_count = _required_persistence_count(
                persistence.get("import_file_log_count"),
                field="import_file_log_count",
            )
            if unit_log_count != len(normalized_ids):
                raise RuntimeError("import persistence unit manifest count mismatch")
            seen.update(normalized_ids)
            rows_imported_norm += _required_persistence_count(
                persistence.get("rows_imported_norm"),
                field="rows_imported_norm",
            )
            unit_kinds = persistence.get("norm_rows_by_kind")
            if not isinstance(unit_kinds, dict):
                raise RuntimeError("import persistence unit kinds are unavailable")
            for kind, count in unit_kinds.items():
                if type(kind) is not str or not kind:
                    raise RuntimeError("import persistence unit kind is unavailable")
                norm_rows_by_kind[kind] = norm_rows_by_kind.get(kind, 0) + _required_persistence_count(
                    count,
                    field=f"{kind}_norm_rows",
                )

        if seen != set(expected_files):
            raise RuntimeError("import persistence unit coverage mismatch")
        expected_rows_norm = sum(
            _required_persistence_count(item.get("rows_imported_norm"), field="rows_imported_norm")
            for item in expected_files.values()
        )
        if rows_imported_norm != expected_rows_norm:
            raise RuntimeError("import persistence unit normalized count mismatch")
        return {
            "case_id": case_id,
            "file_ids": sorted(seen),
            "import_file_log_count": len(seen),
            "rows_imported_norm": rows_imported_norm,
            "norm_rows_by_kind": norm_rows_by_kind,
        }
