from __future__ import annotations

import logging
import queue
import threading
import time
import uuid
from typing import Any, Mapping, Optional, Sequence

from app.core.safe_observability import log_closed_diagnostic
from app.domain.analysis_public_projection import (
    project_analysis_trace_path_public,
    project_analysis_trace_public,
)
from app.domain.analysis_account_fact_boundary import (
    account_fact_skill_requires_host_authority,
    project_account_fact_boundary,
)
from app.domain.analysis_maintenance_public_projection import (
    evidence_pack_deferred_write_requires_host_authority,
    evidence_pack_skill_requires_host_authority,
    project_analysis_evidence_pack_public,
    project_analysis_maintenance_public,
    resolve_analysis_refresh_job_id,
)
from app.repositories.analysis_repository import AnalysisRepository
from app.repositories.privacy_projection_repository import PrivacyProjectionRepository
from app.tasks.models import TaskStatus, TaskType
from app.tasks.service import TaskService
from app.tasks.state_machine import InvalidTaskTransitionError
from app.domain.durable_write_activity import build_durable_write_event
from app.domain.privacy_semantic_payload import (
    enrich_provider_visible_privacy_semantics,
)
from app.utils.memory_secret_guard import (
    scan_memory_note_for_secrets,
    scan_session_memory_for_secrets,
)
from app.utils.time import utc_now
from app.domain.workspace_projection_service import WorkspaceProjectionService


class AnalysisCaseNotFoundError(KeyError):
    pass


class AnalysisValidationError(ValueError):
    pass


class AnalysisTraceNotFoundError(KeyError):
    pass


class AnalysisReportNotFoundError(KeyError):
    pass


class AnalysisPrivacyProjectionError(RuntimeError):
    pass


class AnalysisApprovalNotFoundError(KeyError):
    pass


_ASYNC_WRITE_RETRYABLE_CONFLICT_MARKERS = (
    "unique file handle conflict",
    "already attached by database",
    "same database file with a different configuration",
    "could not set lock on file",
    "conflicting lock is held",
    "transactioncontext error",
    "conflict on update",
)

_ASYNC_WRITE_DROPPABLE_MARKERS = (
    "no such file or directory",
    "unable to open database file",
)

_ANALYSIS_TRACE_FAILED = "analysis_trace_failed"
_ANALYSIS_REFRESH_FAILED = "analysis_refresh_failed"


def _async_write_conflict_message(exc: Exception) -> str:
    return str(exc or "").lower()


def _is_retryable_async_write_conflict(exc: Exception) -> bool:
    message = _async_write_conflict_message(exc)
    return any(marker in message for marker in _ASYNC_WRITE_RETRYABLE_CONFLICT_MARKERS)


def _is_droppable_async_write_error(exc: Exception) -> bool:
    message = _async_write_conflict_message(exc)
    return any(marker in message for marker in _ASYNC_WRITE_DROPPABLE_MARKERS)


class AnalysisService:
    def __init__(
        self,
        repository: AnalysisRepository,
        *,
        task_service: Optional[TaskService] = None,
        workspace_projection_service: Optional[WorkspaceProjectionService] = None,
        privacy_projection_repository: Optional[PrivacyProjectionRepository] = None,
    ) -> None:
        self._repository = repository
        self._task_service = task_service
        self._workspace_projection_service = workspace_projection_service
        self._privacy_projection_repository = privacy_projection_repository
        self._lock = threading.RLock()
        self._threads: dict[str, threading.Thread] = {}
        self._maintenance_stop = threading.Event()
        self._maintenance_thread: Optional[threading.Thread] = None
        self._maintenance_interval_s = 0
        self._maintenance_startup_delay_s = 0
        self._logger = logging.getLogger("analytix.analysis.maintenance")
        self._async_write_queue: queue.Queue[tuple[str, str, dict[str, Any], int]] = queue.Queue()
        self._hot_case_counts: dict[str, int] = {}
        self._hot_case_lock = threading.RLock()
        self._async_write_thread = threading.Thread(
            target=self._async_write_worker,
            name="analytix-analysis-async-write",
            daemon=True,
        )
        self._async_write_thread.start()

    @property
    def task_service(self) -> Optional[TaskService]:
        return self._task_service

    def ensure_case_bootstrap(self, case_id: str) -> None:
        try:
            self._repository.sync_case_baseline(case_id)
            if self._workspace_projection_service is not None:
                self._workspace_projection_service.ensure_case_projection(
                    case_id,
                    sync_baseline=False,
                    render_mode="bootstrap",
                )
            else:
                self._repository.render_workspace_files(
                    case_id,
                    [],
                    sync_baseline=False,
                    render_mode="bootstrap",
                )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def is_privacy_projection_enabled(self, case_id: str) -> bool:
        if self._privacy_projection_repository is None:
            return False
        return bool(self._privacy_projection_repository.is_enabled(case_id))

    def project_privacy_model_payload(self, case_id: str, payload: Any) -> Any:
        try:
            semantic_payload = enrich_provider_visible_privacy_semantics(payload)
            repository = self._privacy_projection_repository or PrivacyProjectionRepository()
            return repository.project_model_payload(case_id, semantic_payload)
        except Exception:
            log_closed_diagnostic(
                self._logger,
                logging.ERROR,
                topic="privacy_projection",
                code="projection_failed",
            )
            raise AnalysisPrivacyProjectionError("privacy_projection_failed") from None

    def project_privacy_model_text(self, case_id: str, text: str) -> str:
        try:
            repository = self._privacy_projection_repository or PrivacyProjectionRepository()
            return repository.project_model_text(case_id, text)
        except Exception:
            log_closed_diagnostic(
                self._logger,
                logging.ERROR,
                topic="privacy_projection",
                code="projection_failed",
            )
            raise AnalysisPrivacyProjectionError("privacy_projection_failed") from None

    def assert_provider_visible_privacy_payload_safe(self, case_id: str, payload: Any) -> None:
        repository = self._privacy_projection_repository or PrivacyProjectionRepository()
        repository.assert_provider_visible_payload_safe(case_id, payload)

    def get_case_profile_semantic_view(self, case_id: str) -> dict:
        try:
            return self._repository.get_case_profile_semantic_view(case_id)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def list_llm_sessions(
        self,
        case_id: str,
        *,
        keyword: str = "",
        limit: int = 200,
    ) -> list[dict]:
        try:
            return self._repository.list_llm_sessions(case_id, keyword=keyword, limit=limit)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def get_llm_session(self, case_id: str, session_id: str, *, include_domain_trace: bool = False) -> dict:
        try:
            return self._repository.get_llm_session(case_id, session_id, include_domain_trace=include_domain_trace)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def delete_llm_session(self, case_id: str, *, session_id: str) -> dict:
        try:
            return self._repository.delete_llm_session(case_id, session_id=session_id)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def _raise_on_blocked_memory_secret_scan(self, secret_scan: dict[str, Any], *, label: str) -> None:
        if not bool(secret_scan.get("blocked")):
            return
        secret_types = [
            str(item or "").strip()
            for item in list(secret_scan.get("secret_types") or [])
            if str(item or "").strip()
        ]
        detail = "、".join(secret_types[:3]) or "secret_like_content"
        raise AnalysisValidationError(f"{label}疑似包含敏感信息（{detail}），请脱敏后再保存。")

    def sync_llm_sessions(self, case_id: str, *, sessions: Sequence[dict], replace_missing: bool = True) -> dict:
        try:
            return self._repository.sync_llm_sessions(case_id, sessions, replace_missing=replace_missing)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def save_llm_session_memory(
        self,
        case_id: str,
        *,
        session_id: str,
        memory: dict,
        covered_until_turn_id: str = "",
        covered_message_count: int = 0,
        last_prompt: str = "",
        title: str = "",
        base_session: Optional[dict] = None,
    ) -> dict:
        self._raise_on_blocked_memory_secret_scan(
            scan_session_memory_for_secrets(
                memory,
                title=title,
                last_prompt=last_prompt,
            ),
            label="会话记忆",
        )
        try:
            return self._repository.save_llm_session_memory(
                case_id,
                session_id=session_id,
                memory=memory,
                covered_until_turn_id=covered_until_turn_id,
                covered_message_count=covered_message_count,
                last_prompt=last_prompt,
                title=title,
                base_session=base_session,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def list_llm_memory_notes(
        self,
        case_id: str,
        *,
        scope: str = "",
        actor_id: str = "",
        limit: int = 100,
        include_archived: bool = False,
    ) -> list[dict]:
        try:
            return self._repository.list_llm_memory_notes(
                case_id,
                scope=scope,
                actor_id=actor_id,
                limit=limit,
                include_archived=include_archived,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def save_llm_memory_note(
        self,
        case_id: str,
        *,
        note: dict[str, Any],
        actor_id: str = "",
        defer_projection: bool = False,
    ) -> dict[str, Any]:
        self._raise_on_blocked_memory_secret_scan(
            scan_memory_note_for_secrets(note),
            label="记忆内容",
        )
        started_payload = {
            "artifact_id": "memory_note",
            "source_query_ids": list(note.get("source_query_ids") or []),
            "source_evidence_ids": list(note.get("source_evidence_ids") or []),
            "projection_kind": "workflow_note",
            "render_mode": "memory_note_update",
        }
        self._record_durable_write_activity_event(
            case_id=case_id,
            kind="memory_note",
            payloads=[started_payload],
            status="started",
        )
        try:
            record = self._repository.save_llm_memory_note(
                case_id,
                note=note,
                actor_id=actor_id,
            )
        except KeyError as exc:
            self._record_durable_write_activity_event(
                case_id=case_id,
                kind="memory_note",
                payloads=[started_payload],
                status="failed",
                error=str(exc),
            )
            raise AnalysisCaseNotFoundError(case_id) from exc
        try:
            if (
                str(record.get("scope") or "").strip() == "workspace"
            ):
                projection_files = ["WORKSPACE_MEMORY.md"]
                if defer_projection or self.is_case_hot(case_id):
                    async_job_id = self.refresh_workspace_projection_async(
                        case_id,
                        files=projection_files,
                        sync_baseline=False,
                        render_mode="memory_note_update",
                    )
                    record["workspace_projection_queued"] = True
                    record["workspace_projection_result_ref"] = self._workspace_projection_result_ref(
                        files=projection_files,
                        render_mode="memory_note_update",
                        status="queued",
                        async_job_id=async_job_id,
                    )
                else:
                    if self._workspace_projection_service is not None:
                        rendered = self._workspace_projection_service.refresh_projection_files(
                            case_id,
                            files=projection_files,
                            sync_baseline=False,
                            render_mode="memory_note_update",
                        )
                    else:
                        rendered = self._repository.render_workspace_files(
                            case_id,
                            projection_files,
                            sync_baseline=False,
                            render_mode="memory_note_update",
                        )
                    record["workspace_files"] = list(rendered or [])
                    record["workspace_projection_result_ref"] = self._workspace_projection_result_ref(
                        files=projection_files,
                        render_mode="memory_note_update",
                        status="completed",
                        rendered_files=list(rendered or []),
                    )
            completed_payload = {
                **started_payload,
                "note_id": str(record.get("note_id") or "").strip(),
                "workspace_files": list(dict(record.get("workspace_projection_result_ref") or {}).get("workspace_files") or []),
            }
            record["memory_write_result_ref"] = self._record_durable_write_activity_event(
                case_id=case_id,
                kind="memory_note",
                payloads=[completed_payload],
                status="completed",
            )
        except Exception as exc:
            self._record_durable_write_activity_event(
                case_id=case_id,
                kind="memory_note",
                payloads=[started_payload],
                status="failed",
                error=str(exc),
            )
            raise
        return record

    def review_llm_memory_hygiene(
        self,
        case_id: str,
        *,
        actor_id: str = "",
        limit: int = 100,
    ) -> dict[str, Any]:
        from app.memory.memory_hygiene import review_memory_hygiene

        notes = self.list_llm_memory_notes(
            case_id,
            actor_id=actor_id,
            limit=limit,
            include_archived=False,
        )
        return review_memory_hygiene(
            case_id=case_id,
            actor_id=actor_id,
            notes=notes,
        )

    def append_audit_event(self, case_id: str, *, event: dict[str, Any]) -> dict[str, Any]:
        try:
            return self._repository.append_audit_event(case_id, event=event)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def append_audit_events(self, case_id: str, *, events: Sequence[dict[str, Any]]) -> list[dict[str, Any]]:
        try:
            return self._repository.append_audit_events(case_id, events=events)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def append_audit_event_async(self, case_id: str, *, event: dict[str, Any]) -> None:
        self._async_write_queue.put(("audit", str(case_id or "").strip(), dict(event or {}), 0))

    def append_run_log_event(self, case_id: str, *, event: dict[str, Any]) -> dict[str, Any]:
        try:
            return self._repository.append_run_log_event(case_id, event=event)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def append_run_log_events(self, case_id: str, *, events: Sequence[dict[str, Any]]) -> list[dict[str, Any]]:
        try:
            return self._repository.append_run_log_events(case_id, events=events)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def append_run_log_event_async(self, case_id: str, *, event: dict[str, Any]) -> None:
        self._async_write_queue.put(("run_log", str(case_id or "").strip(), dict(event or {}), 0))

    def mark_case_hot(self, case_id: str, hot: bool) -> None:
        normalized_case_id = str(case_id or "").strip()
        if not normalized_case_id:
            return
        with self._hot_case_lock:
            if hot:
                self._hot_case_counts[normalized_case_id] = self._hot_case_counts.get(normalized_case_id, 0) + 1
                return
            current_count = self._hot_case_counts.get(normalized_case_id, 0)
            if current_count <= 1:
                self._hot_case_counts.pop(normalized_case_id, None)
            else:
                self._hot_case_counts[normalized_case_id] = current_count - 1

    def is_case_hot(self, case_id: str) -> bool:
        normalized_case_id = str(case_id or "").strip()
        if not normalized_case_id:
            return False
        with self._hot_case_lock:
            return self._hot_case_counts.get(normalized_case_id, 0) > 0

    def _next_async_write_job_id(self, kind: str) -> str:
        normalized_kind = str(kind or "write").strip() or "write"
        return f"async-write:{normalized_kind}:{uuid.uuid4().hex[:12]}"

    def _async_write_result_ref_from_payloads(
        self,
        *,
        kind: str,
        payloads: Sequence[dict[str, Any]],
        status: str,
        error: str = "",
    ) -> dict[str, Any]:
        job_ids = [
            str(payload.get("async_job_id") or "").strip()
            for payload in payloads
            if str(payload.get("async_job_id") or "").strip()
        ]
        report_ids = [
            str(payload.get("report_id") or "").strip()
            for payload in payloads
            if str(payload.get("report_id") or "").strip()
        ]
        result_ref: dict[str, Any] = {
            "async_write_kind": str(kind or "").strip(),
            "async_write_status": str(status or "").strip(),
            "async_write_job_ids": job_ids,
            "report_ids": report_ids,
            "query_ids": [
                str(payload.get("query_id") or "").strip()
                for payload in payloads
                if str(payload.get("query_id") or "").strip()
            ],
            "workspace_files": [
                str(file_name or "").strip()
                for payload in payloads
                for file_name in list(payload.get("files") or [])
                if str(file_name or "").strip()
            ],
            "projection_kinds": [
                str(payload.get("projection_kind") or "").strip()
                for payload in payloads
                if str(payload.get("projection_kind") or "").strip()
            ],
            "render_modes": [
                str(payload.get("render_mode") or "").strip()
                for payload in payloads
                if str(payload.get("render_mode") or "").strip()
            ],
        }
        if error:
            result_ref["error"] = str(error)
        return {key: value for key, value in result_ref.items() if value not in ("", None, [], {})}

    def _workspace_projection_result_ref(
        self,
        *,
        files: Sequence[str],
        render_mode: str,
        status: str,
        projection_kind: str = "files",
        async_job_id: str = "",
        rendered_files: Sequence[dict[str, Any]] = (),
    ) -> dict[str, Any]:
        workspace_files = [
            str(file_name or "").strip()
            for file_name in list(files or [])
            if str(file_name or "").strip()
        ]
        if not workspace_files:
            workspace_files = [
                str(item.get("file_name") or item.get("path") or "").strip()
                for item in list(rendered_files or [])
                if isinstance(item, dict) and str(item.get("file_name") or item.get("path") or "").strip()
            ]
        payload: dict[str, Any] = {
            "files": workspace_files,
            "render_mode": str(render_mode or "manual").strip() or "manual",
            "projection_kind": str(projection_kind or "files").strip() or "files",
        }
        if str(async_job_id or "").strip():
            payload["async_job_id"] = str(async_job_id or "").strip()
        return self._async_write_result_ref_from_payloads(
            kind="workspace_projection",
            payloads=[payload],
            status=status,
        )

    def _record_durable_write_activity_event(
        self,
        *,
        case_id: str,
        kind: str,
        payloads: Sequence[dict[str, Any]],
        status: str,
        error: str = "",
    ) -> dict[str, Any]:
        normalized_case_id = str(case_id or "").strip()
        safe_error = "analysis_durable_write_failed" if error else ""
        result_ref, event = build_durable_write_event(
            kind=kind,
            payloads=payloads,
            status=status,
            error=safe_error,
        )
        if not normalized_case_id:
            return result_ref
        try:
            if hasattr(self._repository, "append_run_log_event"):
                self.append_run_log_event(normalized_case_id, event=event)
            if hasattr(self._repository, "append_audit_event"):
                self.append_audit_event(normalized_case_id, event=event)
        except Exception:
            log_closed_diagnostic(
                getattr(self, "_logger", logging.getLogger("analytix.analysis.maintenance")),
                logging.ERROR,
                topic="analysis_async_write",
                code="audit_failed",
            )
        return result_ref

    def _record_async_write_result_event(
        self,
        *,
        case_id: str,
        kind: str,
        payloads: Sequence[dict[str, Any]],
        status: str,
        error: str = "",
    ) -> None:
        if str(kind or "").strip() not in {"query_log", "workspace_projection"}:
            return
        normalized_case_id = str(case_id or "").strip()
        if not normalized_case_id:
            return
        normalized_status = str(status or "").strip() or "completed"
        payload_list = [dict(payload or {}) for payload in list(payloads or [])]
        safe_error = "analysis_async_write_failed" if error else ""
        result_ref = self._async_write_result_ref_from_payloads(
            kind=kind,
            payloads=payload_list,
            status=normalized_status,
            error=safe_error,
        )
        event = {
            "event_type": f"analysis.async_write.{normalized_status}",
            "actor_id": "system",
            "actor_role": "system",
            "payload": {
                "kind": str(kind or "").strip(),
                "status": normalized_status,
                "result_ref": result_ref,
                "jobs": [
                    {
                        "async_job_id": str(payload.get("async_job_id") or "").strip(),
                        "report_id": str(payload.get("report_id") or "").strip(),
                        "scope_present": bool(str(payload.get("scope") or "").strip()),
                    }
                    for payload in payload_list
                    if str(payload.get("async_job_id") or payload.get("report_id") or "").strip()
                ],
                **({"error": safe_error} if safe_error else {}),
            },
        }
        try:
            self.append_run_log_event(normalized_case_id, event=event)
            self.append_audit_event(normalized_case_id, event=event)
        except Exception:
            log_closed_diagnostic(
                self._logger,
                logging.ERROR,
                topic="analysis_async_write",
                code="audit_failed",
            )

    def flush_async_events(self, *, timeout_s: float = 10.0) -> None:
        deadline = time.monotonic() + max(0.0, float(timeout_s or 0.0))
        while self._async_write_queue.unfinished_tasks:
            if timeout_s > 0 and time.monotonic() >= deadline:
                return
            time.sleep(0.02)

    def ensure_scratchpad_workspace(self, case_id: str, *, workspace: dict[str, Any]) -> dict[str, Any]:
        try:
            return self._repository.ensure_scratchpad_workspace(case_id, workspace=workspace)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def list_scratchpad_workspaces(
        self,
        case_id: str,
        *,
        run_id: str = "",
        turn_id: str = "",
    ) -> list[dict[str, Any]]:
        try:
            return self._repository.list_scratchpad_workspaces(case_id, run_id=run_id, turn_id=turn_id)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def write_scratchpad_artifact(self, case_id: str, *, artifact: dict[str, Any]) -> dict[str, Any]:
        started_payload = {
            "artifact_id": str(artifact.get("artifact_id") or artifact.get("run_id") or "scratchpad").strip(),
            "source_query_ids": list(artifact.get("query_ids") or artifact.get("source_query_ids") or []),
            "source_evidence_ids": list(artifact.get("evidence_ids") or artifact.get("source_evidence_ids") or []),
            "source_path_ids": list(artifact.get("path_ids") or artifact.get("source_path_ids") or []),
        }
        self._record_durable_write_activity_event(
            case_id=case_id,
            kind="repository_artifact",
            payloads=[started_payload],
            status="started",
        )
        try:
            result = self._repository.write_scratchpad_artifact(case_id, artifact=artifact)
            completed_payload = {
                **started_payload,
                "artifact_id": str(
                    result.get("artifact_id") or result.get("run_id") or started_payload["artifact_id"]
                ).strip(),
            }
            result["result_ref"] = self._record_durable_write_activity_event(
                case_id=case_id,
                kind="repository_artifact",
                payloads=[completed_payload],
                status="completed",
            )
            return result
        except KeyError as exc:
            self._record_durable_write_activity_event(
                case_id=case_id,
                kind="repository_artifact",
                payloads=[started_payload],
                status="failed",
                error=str(exc),
            )
            raise AnalysisCaseNotFoundError(case_id) from exc
        except Exception as exc:
            self._record_durable_write_activity_event(
                case_id=case_id,
                kind="repository_artifact",
                payloads=[started_payload],
                status="failed",
                error=str(exc),
            )
            raise

    def list_scratchpad_artifacts(
        self,
        case_id: str,
        *,
        run_id: str = "",
        workspace_id: str = "",
    ) -> list[dict[str, Any]]:
        try:
            return self._repository.list_scratchpad_artifacts(case_id, run_id=run_id, workspace_id=workspace_id)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def _async_write_worker(self) -> None:
        while True:
            first_kind, first_case_id, first_payload, first_attempt = self._async_write_queue.get()
            batch: list[tuple[str, str, dict[str, Any], int]] = [
                (first_kind, first_case_id, first_payload, first_attempt)
            ]
            try:
                while len(batch) < 64:
                    batch.append(self._async_write_queue.get_nowait())
            except queue.Empty:
                pass
            try:
                grouped: dict[tuple[str, str, str, str], list[tuple[dict[str, Any], int]]] = {}
                for kind, case_id, payload, attempt in batch:
                    payload_dict = dict(payload or {})
                    grouped.setdefault(
                        (
                            kind,
                            case_id,
                            str(payload_dict.get("run_id") or "").strip(),
                            str(payload_dict.get("turn_id") or "").strip(),
                        ),
                        [],
                    ).append((payload_dict, max(0, int(attempt or 0))))
                for (kind, case_id, _run_id, _turn_id), payload_entries in grouped.items():
                    if kind not in {"audit", "run_log", "query_log", "workspace_projection"}:
                        continue
                    payload_entries = [
                        (payload, attempt)
                        for payload, attempt in payload_entries
                        if not evidence_pack_deferred_write_requires_host_authority(
                            kind=kind,
                            payload=payload,
                        )
                    ]
                    if not payload_entries:
                        continue
                    payloads = [dict(payload or {}) for payload, _attempt in payload_entries]
                    if self.is_case_hot(case_id):
                        for payload, attempt in payload_entries:
                            self._async_write_queue.put((kind, case_id, payload, attempt))
                        time.sleep(0.02)
                        continue
                    if not self._repository.case_exists(case_id):
                        self._record_async_write_result_event(
                            case_id=case_id,
                            kind=kind,
                            payloads=payloads,
                            status="failed",
                            error="case_not_found",
                        )
                        continue
                    case_db_path = self._repository.storage.case_db(case_id)
                    if not case_db_path.exists():
                        self._record_async_write_result_event(
                            case_id=case_id,
                            kind=kind,
                            payloads=payloads,
                            status="failed",
                            error="case_database_missing",
                        )
                        continue
                    self._record_async_write_result_event(
                        case_id=case_id,
                        kind=kind,
                        payloads=payloads,
                        status="started",
                    )
                    try:
                        if kind == "audit":
                            self.append_audit_events(case_id, events=payloads)
                        elif kind == "run_log":
                            self.append_run_log_events(case_id, events=payloads)
                        elif kind == "query_log":
                            for payload in payloads:
                                query_id = str(payload.get("query_id") or "").strip()
                                tool_name = str(payload.get("tool_name") or "").strip()
                                if not query_id or not tool_name:
                                    continue
                                self.append_query_log_entry(
                                    case_id,
                                    query_id=query_id,
                                    tool_name=tool_name,
                                    params=dict(payload.get("params") or {}),
                                    summary=dict(payload.get("summary") or {}),
                                    row_count=int(payload.get("row_count") or 0),
                                    duration_ms=int(payload.get("duration_ms") or 0),
                                )
                            self._record_async_write_result_event(
                                case_id=case_id,
                                kind=kind,
                                payloads=payloads,
                                status="completed",
                            )
                        elif kind == "workspace_projection":
                            for payload in payloads:
                                projection_kind = str(payload.get("projection_kind") or "files").strip() or "files"
                                render_mode = str(payload.get("render_mode") or "manual").strip() or "manual"
                                files = [
                                    str(item or "").strip()
                                    for item in list(payload.get("files") or [])
                                    if str(item or "").strip()
                                ]
                                if self._workspace_projection_service is not None:
                                    if projection_kind == "evidence_index":
                                        self._workspace_projection_service.refresh_evidence_index_projection(
                                            case_id,
                                            render_mode=render_mode,
                                        )
                                    elif projection_kind == "workflow_note":
                                        self._workspace_projection_service.refresh_workflow_note_projection(
                                            case_id,
                                            day_text=str(payload.get("day_text") or "").strip(),
                                            render_mode=render_mode,
                                        )
                                    elif files:
                                        self._workspace_projection_service.refresh_projection_files(
                                            case_id,
                                            files=files,
                                            sync_baseline=bool(payload.get("sync_baseline")),
                                            render_mode=render_mode,
                                        )
                                elif files:
                                    self._repository.render_workspace_files(
                                        case_id,
                                        files,
                                        sync_baseline=bool(payload.get("sync_baseline")),
                                        render_mode=render_mode,
                                    )
                            self._record_async_write_result_event(
                                case_id=case_id,
                                kind=kind,
                                payloads=payloads,
                                status="completed",
                            )
                    except Exception as exc:
                        if _is_droppable_async_write_error(exc):
                            self._record_async_write_result_event(
                                case_id=case_id,
                                kind=kind,
                                payloads=payloads,
                                status="failed",
                                error="analysis_async_write_failed",
                            )
                            continue
                        if _is_retryable_async_write_conflict(exc):
                            max_attempt = max((attempt for _payload, attempt in payload_entries), default=0)
                            if max_attempt < 8:
                                delay_s = min(0.5, 0.02 * (2 ** max_attempt))
                                for payload, attempt in payload_entries:
                                    self._async_write_queue.put((kind, case_id, payload, attempt + 1))
                                time.sleep(delay_s)
                                continue
                        self._record_async_write_result_event(
                            case_id=case_id,
                            kind=kind,
                            payloads=payloads,
                            status="failed",
                            error="analysis_async_write_failed",
                        )
                        raise
            except Exception:
                log_closed_diagnostic(
                    self._logger,
                    logging.ERROR,
                    topic="analysis_async_write",
                    code="write_failed",
                    numeric={"batch_size": len(batch)},
                )
            finally:
                for _ in batch:
                    self._async_write_queue.task_done()

    def mark_scratchpad_retention(
        self,
        case_id: str,
        *,
        run_id: str,
        status: str,
        retention_state: str,
    ) -> dict[str, Any]:
        try:
            return self._repository.mark_scratchpad_retention(
                case_id,
                run_id=run_id,
                status=status,
                retention_state=retention_state,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def list_audit_events(
        self,
        case_id: str,
        *,
        run_id: str = "",
        turn_id: str = "",
        limit: int = 200,
    ) -> list[dict]:
        try:
            return self._repository.list_audit_events(case_id, run_id=run_id, turn_id=turn_id, limit=limit)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def list_run_log_events(
        self,
        case_id: str,
        *,
        run_id: str = "",
        turn_id: str = "",
        limit: int = 500,
    ) -> list[dict[str, Any]]:
        try:
            return self._repository.list_run_log_events(case_id, run_id=run_id, turn_id=turn_id, limit=limit)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def build_llm_session_memory_rollups(
        self,
        case_id: str,
        *,
        session_id: str = "",
        limit: int = 4,
    ) -> dict:
        try:
            return self._repository.build_llm_session_memory_rollups(
                case_id,
                session_id=session_id,
                limit=limit,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def get_latest_llm_phase_judgment(self, case_id: str, *, session_id: str = "") -> dict:
        try:
            return self._repository.get_latest_llm_phase_judgment(case_id, session_id=session_id)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def list_llm_phase_judgments(
        self,
        case_id: str,
        *,
        session_id: str = "",
        limit: int = 20,
    ) -> list[dict]:
        try:
            return self._repository.list_llm_phase_judgments(case_id, session_id=session_id, limit=limit)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def save_llm_phase_judgment(self, case_id: str, *, judgment: dict) -> dict:
        try:
            return self._repository.save_llm_phase_judgment(case_id, judgment)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def materialize_feature_mart(
        self,
        case_id: str,
        *,
        account_keys: Sequence[str] = (),
        temp_scope_id: str = "",
        source_file_ids: Sequence[str] = (),
        force_refresh: bool = False,
        max_accounts: int = 12,
    ) -> dict:
        del account_keys, temp_scope_id, source_file_ids, force_refresh, max_accounts
        return project_analysis_maintenance_public(
            case_id=case_id,
            operation="feature_mart_materialize",
        )

    def get_llm_reference_preview(self, case_id: str, *, ref_type: str, ref_id: str) -> dict:
        if not self._repository.case_exists(case_id):
            raise AnalysisCaseNotFoundError(case_id)
        return self._repository.get_llm_reference_preview(case_id, ref_type, ref_id)

    def get_rule_hits(
        self,
        case_id: str,
        *,
        entity_id: str = "",
        risk_type: str = "",
        severity: str = "",
        cursor: str = "",
        limit: int = 20,
        force_refresh: bool = False,
    ) -> dict:
        try:
            result = self._repository.get_rule_hits(
                case_id,
                entity_id=entity_id,
                risk_type=risk_type,
                severity=severity,
                cursor=cursor,
                limit=limit,
                force_refresh=force_refresh,
            )
            if self._workspace_projection_service is not None and list(result.get("items") or []):
                if self.is_case_hot(case_id):
                    self.refresh_evidence_index_projection_async(
                        case_id,
                        render_mode="system_refresh",
                    )
                else:
                    self._workspace_projection_service.refresh_evidence_index_projection(
                        case_id,
                        render_mode="system_refresh",
                    )
            return result
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def get_rule_params(self, case_id: str) -> dict:
        try:
            return self._repository.get_rule_params(case_id)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def write_workspace_projection_file(
        self,
        case_id: str,
        *,
        file_name: str,
        content_md: str,
        render_mode: str,
    ) -> dict:
        try:
            return self._repository.write_workspace_projection_file(
                case_id,
                file_name=file_name,
                content_md=content_md,
                render_mode=render_mode,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def update_rule_params(self, case_id: str, *, scope_type: str, params: dict) -> dict:
        try:
            return self._repository.update_rule_params(case_id, scope_type=scope_type, params=params)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def get_txn_slice(
        self,
        case_id: str,
        *,
        filters: Optional[dict] = None,
        visible_columns: Optional[Sequence[str]] = None,
        sort: Optional[dict] = None,
        cursor: str = "",
        limit: int = 50,
    ) -> dict:
        try:
            return self._repository.get_txn_slice(
                case_id,
                filters=filters,
                visible_columns=visible_columns,
                sort=sort,
                cursor=cursor,
                limit=limit,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def upsert_temp_transaction_scope(
        self,
        case_id: str,
        *,
        source_file_ids: Sequence[str],
        document_ids: Optional[Sequence[str]] = None,
        source_kind: str = "uploaded_file",
    ) -> dict:
        started_payload = {
            "artifact_id": "temp_scope_operation",
            "projection_kind": "temp_transaction_scope",
            "render_mode": "temp_scope_ingest",
        }
        self._record_durable_write_activity_event(
            case_id=case_id,
            kind="temp_transaction_scope",
            payloads=[started_payload],
            status="started",
        )
        try:
            result = self._repository.upsert_temp_transaction_scope(
                case_id,
                source_file_ids=source_file_ids,
                document_ids=document_ids,
                source_kind=source_kind,
            )
            completed_payload = {
                **started_payload,
                "artifact_id": str(result.get("scope_id") or started_payload["artifact_id"]).strip(),
                "projection_kind": str(result.get("scope_type") or "temp_transaction_scope").strip()
                or "temp_transaction_scope",
            }
            result["result_ref"] = self._record_durable_write_activity_event(
                case_id=case_id,
                kind="temp_transaction_scope",
                payloads=[completed_payload],
                status="completed",
            )
            return result
        except KeyError as exc:
            self._record_durable_write_activity_event(
                case_id=case_id,
                kind="temp_transaction_scope",
                payloads=[started_payload],
                status="failed",
                error="temp_scope_ingest_failed",
            )
            raise AnalysisCaseNotFoundError(case_id) from exc
        except Exception as exc:
            self._record_durable_write_activity_event(
                case_id=case_id,
                kind="temp_transaction_scope",
                payloads=[started_payload],
                status="failed",
                error="temp_scope_ingest_failed",
            )
            raise

    def get_temp_transaction_scope(self, case_id: str, *, scope_id: str) -> dict:
        try:
            return self._repository.get_temp_transaction_scope(case_id, scope_id)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def run_temp_scope_janitor(self) -> dict[str, Any]:
        return self._repository.run_temp_scope_janitor()

    def run_runtime_retention_janitor(self, *, case_id: str = "") -> dict[str, Any]:
        try:
            result = self._repository.run_runtime_retention_janitor(case_id=case_id)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc
        for item in list(result.get("items") or []):
            current_case_id = str(item.get("case_id") or "").strip()
            if not current_case_id:
                continue
            if int(item.get("stale_count") or 0) <= 0 and int(item.get("deleted_count") or 0) <= 0:
                continue
            self.append_audit_event(
                current_case_id,
                event={
                    "event_type": "analysis.runtime_retention.cleaned",
                    "actor_id": "system",
                    "actor_role": "system",
                    "payload": {
                        "job_kind": "runtime_retention_janitor",
                        "summary": dict(item),
                    },
                },
            )
        return result

    def append_query_log_entry(
        self,
        case_id: str,
        *,
        query_id: str,
        tool_name: str,
        params: dict[str, Any],
        summary: dict[str, Any],
        row_count: int,
        duration_ms: int,
    ) -> str:
        if evidence_pack_skill_requires_host_authority(tool_name):
            return ""
        try:
            return self._repository.append_query_log_entry(
                case_id,
                query_id=query_id,
                tool_name=tool_name,
                params=params,
                summary=summary,
                row_count=row_count,
                duration_ms=duration_ms,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def append_query_log_entry_async(
        self,
        case_id: str,
        *,
        query_id: str,
        tool_name: str,
        params: dict[str, Any],
        summary: dict[str, Any],
        row_count: int,
        duration_ms: int,
    ) -> str:
        if evidence_pack_skill_requires_host_authority(tool_name):
            return ""
        async_job_id = self._next_async_write_job_id("query_log")
        self._async_write_queue.put(
            (
                "query_log",
                str(case_id or "").strip(),
                {
                    "async_job_id": async_job_id,
                    "query_id": str(query_id or "").strip(),
                    "tool_name": str(tool_name or "").strip(),
                    "params": dict(params or {}),
                    "summary": dict(summary or {}),
                    "row_count": max(0, int(row_count or 0)),
                    "duration_ms": max(0, int(duration_ms or 0)),
                },
                0,
            )
        )
        return async_job_id

    def refresh_workspace_projection_async(
        self,
        case_id: str,
        *,
        files: Sequence[str],
        sync_baseline: bool = False,
        render_mode: str = "manual",
        projection_kind: str = "files",
    ) -> str:
        requested_files = [str(item or "").strip() for item in list(files or []) if str(item or "").strip()]
        normalized_files = self._repository.validate_ordinary_workspace_files(requested_files)
        async_job_id = self._next_async_write_job_id("workspace_projection")
        self._async_write_queue.put(
            (
                "workspace_projection",
                str(case_id or "").strip(),
                {
                    "async_job_id": async_job_id,
                    "files": normalized_files,
                    "sync_baseline": bool(sync_baseline),
                    "render_mode": str(render_mode or "manual").strip() or "manual",
                    "projection_kind": str(projection_kind or "files").strip() or "files",
                },
                0,
            )
        )
        return async_job_id

    def refresh_evidence_index_projection_async(
        self,
        case_id: str,
        *,
        render_mode: str = "system_refresh",
    ) -> str:
        return self.refresh_workspace_projection_async(
            case_id,
            files=["EVIDENCE_INDEX.md"],
            sync_baseline=False,
            render_mode=render_mode,
            projection_kind="evidence_index",
        )

    def build_evidence_pack(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        txn_ids: Optional[Sequence[str]] = None,
        source_file_ids: Optional[Sequence[str]] = None,
        temp_scope_id: str = "",
        scope_source: str = "db",
        budget: Optional[dict[str, Any]] = None,
    ) -> dict:
        del (
            account_keys,
            txn_ids,
            source_file_ids,
            temp_scope_id,
            scope_source,
            budget,
        )
        return project_analysis_evidence_pack_public(case_id=case_id)

    def build_scope_account_snapshot(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        source_file_ids: Optional[Sequence[str]] = None,
        temp_scope_id: str = "",
        scope_source: str = "db",
        date_start: str = "",
        date_end: str = "",
        direction_mode: str = "both",
        success_filter: str = "all",
        cash_filter: str = "all",
        metric_mode: str = "full",
        group_by: str = "none",
    ) -> dict:
        del (
            account_keys,
            source_file_ids,
            temp_scope_id,
            scope_source,
            date_start,
            date_end,
            direction_mode,
            success_filter,
            cash_filter,
            metric_mode,
            group_by,
        )
        return project_account_fact_boundary(case_id, "scope_account_snapshot")

    def build_case_reconciliation(self, case_id: str) -> dict:
        try:
            return self._repository.build_case_reconciliation(case_id)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def audit_case_data_quality(self, case_id: str, *, example_limit: int = 10) -> dict:
        try:
            return self._repository.audit_case_data_quality(case_id, example_limit=example_limit)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def resolve_duplicate_families(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        account_key: str = "",
        card_no: str = "",
        acct_no: str = "",
        holder_name: str = "",
        id_no: str = "",
        match_mode: str = "exact",
        date_start: str = "",
        date_end: str = "",
        direction_mode: str = "both",
        min_amount: float = 0.0,
        scope_mode: str = "same_holder_accounts",
        limit: int = 20,
    ) -> dict:
        try:
            return self._repository.resolve_duplicate_families(
                case_id,
                account_keys=account_keys,
                account_key=account_key,
                card_no=card_no,
                acct_no=acct_no,
                holder_name=holder_name,
                id_no=id_no,
                match_mode=match_mode,
                date_start=date_start,
                date_end=date_end,
                direction_mode=direction_mode,
                min_amount=min_amount,
                scope_mode=scope_mode,
                limit=limit,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def inspect_case_schema(
        self,
        case_id: str,
        *,
        table_limit: int = 200,
        column_limit: int = 80,
        include_columns: bool = True,
    ) -> dict:
        try:
            return self._repository.inspect_case_schema(
                case_id,
                table_limit=table_limit,
                column_limit=column_limit,
                include_columns=include_columns,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def run_case_sql(
        self,
        case_id: str,
        *,
        purpose: str,
        sql: str = "",
        query_request: str = "",
        parameters: Optional[Mapping[str, Any]] = None,
        date_start: str = "",
        date_end: str = "",
        row_limit: int = 100,
        result_mode: str = "preview",
        allowed_view_policy: str = "cleaned_and_analysis_only",
        include_notebook_cell: bool = False,
        include_debug: bool = False,
    ) -> dict:
        try:
            return self._repository.run_case_sql(
                case_id,
                purpose=purpose,
                sql=sql,
                query_request=query_request,
                parameters=parameters,
                date_start=date_start,
                date_end=date_end,
                row_limit=row_limit,
                result_mode=result_mode,
                allowed_view_policy=allowed_view_policy,
                include_notebook_cell=include_notebook_cell,
                include_debug=include_debug,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc
        except ValueError as exc:
            raise AnalysisValidationError("case_sql_request_rejected") from exc

    def explain_case_sql(
        self,
        case_id: str,
        *,
        purpose: str,
        sql: str,
        row_limit: int = 100,
        include_analyze: bool = False,
    ) -> dict:
        try:
            return self._repository.explain_case_sql(
                case_id,
                purpose=purpose,
                sql=sql,
                row_limit=row_limit,
                include_analyze=include_analyze,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc
        except ValueError as exc:
            raise AnalysisValidationError("case_sql_request_rejected") from exc

    def diagnose_case_sql(
        self,
        case_id: str,
        *,
        purpose: str,
        sql: str = "",
        error_message: str = "",
        diagnostic_mode: str = "validate",
        row_limit: int = 100,
    ) -> dict:
        try:
            return self._repository.diagnose_case_sql(
                case_id,
                purpose=purpose,
                sql=sql,
                error_message=error_message,
                diagnostic_mode=diagnostic_mode,
                row_limit=row_limit,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc
        except ValueError as exc:
            raise AnalysisValidationError("case_sql_request_rejected") from exc

    def profile_case_schema(
        self,
        case_id: str,
        *,
        tables: Optional[Sequence[str]] = None,
        table_limit: int = 8,
        column_limit: int = 24,
        enum_limit: int = 8,
    ) -> dict:
        try:
            return self._repository.profile_case_schema(
                case_id,
                tables=tables,
                table_limit=table_limit,
                column_limit=column_limit,
                enum_limit=enum_limit,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc
        except ValueError as exc:
            raise AnalysisValidationError("case_sql_request_rejected") from exc

    def preview_case_rows(
        self,
        case_id: str,
        *,
        purpose: str,
        table_name: str,
        columns: Optional[Sequence[str]] = None,
        where_sql: str = "",
        row_limit: int = 20,
    ) -> dict:
        try:
            return self._repository.preview_case_rows(
                case_id,
                purpose=purpose,
                table_name=table_name,
                columns=columns,
                where_sql=where_sql,
                row_limit=row_limit,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc
        except ValueError as exc:
            raise AnalysisValidationError("case_sql_request_rejected") from exc

    def inspect_workbench_history(self, case_id: str, *, limit: int = 20) -> dict:
        try:
            return self._repository.inspect_workbench_history(case_id, limit=limit)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def case_sql_recipes(self, case_id: str, *, category: str = "", recipe_id: str = "", limit: int = 50) -> dict:
        try:
            return self._repository.case_sql_recipes(case_id, category=category, recipe_id=recipe_id, limit=limit)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def create_case_notebook(
        self,
        case_id: str,
        *,
        title: str = "",
        analysis_goal: str,
        cells: Optional[Sequence[Mapping[str, Any]]] = None,
        max_rows_per_query: int = 100,
        allowed_view_policy: str = "cleaned_and_analysis_only",
        delivery_mode: str = "notebook_artifact",
        include_debug: bool = False,
    ) -> dict:
        try:
            return self._repository.create_case_notebook(
                case_id,
                title=title,
                analysis_goal=analysis_goal,
                cells=cells,
                max_rows_per_query=max_rows_per_query,
                allowed_view_policy=allowed_view_policy,
                delivery_mode=delivery_mode,
                include_debug=include_debug,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc
        except ValueError as exc:
            raise AnalysisValidationError("case_sql_request_rejected") from exc

    def audit_unindexed_sources(
        self,
        case_id: str,
        *,
        table_limit: int = 200,
        column_limit: int = 80,
    ) -> dict:
        try:
            return self._repository.audit_unindexed_sources(
                case_id,
                table_limit=table_limit,
                column_limit=column_limit,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def build_scope_coverage(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        source_file_ids: Optional[Sequence[str]] = None,
        date_start: str = "",
        date_end: str = "",
        direction_mode: str = "both",
        success_filter: str = "all",
        cash_filter: str = "all",
    ) -> dict:
        try:
            return self._repository.build_scope_coverage(
                case_id,
                account_keys=account_keys,
                source_file_ids=source_file_ids,
                date_start=date_start,
                date_end=date_end,
                direction_mode=direction_mode,
                success_filter=success_filter,
                cash_filter=cash_filter,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def resolve_account_scope(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        account_key: str = "",
        card_no: str = "",
        acct_no: str = "",
    ) -> dict:
        try:
            return self._repository.resolve_account_scope(
                case_id,
                account_keys=account_keys,
                account_key=account_key,
                card_no=card_no,
                acct_no=acct_no,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def resolve_holder_scope(
        self,
        case_id: str,
        *,
        holder_name: str = "",
        id_no: str = "",
        match_mode: str = "exact",
    ) -> dict:
        try:
            return self._repository.resolve_holder_scope(
                case_id,
                holder_name=holder_name,
                id_no=id_no,
                match_mode=match_mode,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def build_account_counterparty_rankings(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        source_file_ids: Optional[Sequence[str]] = None,
        temp_scope_id: str = "",
        scope_source: str = "db",
        date_start: str = "",
        date_end: str = "",
        success_filter: str = "all",
        cash_filter: str = "all",
        limit: int = 10,
    ) -> dict:
        try:
            return self._repository.build_account_counterparty_rankings(
                case_id,
                account_keys=account_keys,
                source_file_ids=source_file_ids,
                temp_scope_id=temp_scope_id,
                scope_source=scope_source,
                date_start=date_start,
                date_end=date_end,
                success_filter=success_filter,
                cash_filter=cash_filter,
                limit=limit,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def rank_accounts(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        account_key: str = "",
        card_no: str = "",
        acct_no: str = "",
        holder_name: str = "",
        id_no: str = "",
        match_mode: str = "exact",
        source_file_ids: Optional[Sequence[str]] = None,
        date_start: str = "",
        date_end: str = "",
        direction_mode: str = "both",
        success_filter: str = "all",
        cash_filter: str = "all",
        metric: str = "turnover",
        limit: int = 20,
    ) -> dict:
        try:
            return self._repository.rank_accounts(
                case_id,
                account_keys=account_keys,
                account_key=account_key,
                card_no=card_no,
                acct_no=acct_no,
                holder_name=holder_name,
                id_no=id_no,
                match_mode=match_mode,
                source_file_ids=source_file_ids,
                date_start=date_start,
                date_end=date_end,
                direction_mode=direction_mode,
                success_filter=success_filter,
                cash_filter=cash_filter,
                metric=metric,
                limit=limit,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def rank_holders(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        account_key: str = "",
        card_no: str = "",
        acct_no: str = "",
        holder_name: str = "",
        id_no: str = "",
        match_mode: str = "exact",
        source_file_ids: Optional[Sequence[str]] = None,
        date_start: str = "",
        date_end: str = "",
        direction_mode: str = "both",
        success_filter: str = "all",
        cash_filter: str = "all",
        metric: str = "turnover",
        limit: int = 20,
    ) -> dict:
        try:
            return self._repository.rank_holders(
                case_id,
                account_keys=account_keys,
                account_key=account_key,
                card_no=card_no,
                acct_no=acct_no,
                holder_name=holder_name,
                id_no=id_no,
                match_mode=match_mode,
                source_file_ids=source_file_ids,
                date_start=date_start,
                date_end=date_end,
                direction_mode=direction_mode,
                success_filter=success_filter,
                cash_filter=cash_filter,
                metric=metric,
                limit=limit,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def rank_counterparties(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        account_key: str = "",
        card_no: str = "",
        acct_no: str = "",
        holder_name: str = "",
        id_no: str = "",
        match_mode: str = "exact",
        source_file_ids: Optional[Sequence[str]] = None,
        date_start: str = "",
        date_end: str = "",
        direction_mode: str = "both",
        success_filter: str = "all",
        cash_filter: str = "all",
        metric: str = "turnover",
        limit: int = 20,
        dedupe_same_holder_same_fact: bool = False,
        counterparty_group_mode: str = "account",
    ) -> dict:
        try:
            return self._repository.rank_counterparties(
                case_id,
                account_keys=account_keys,
                account_key=account_key,
                card_no=card_no,
                acct_no=acct_no,
                holder_name=holder_name,
                id_no=id_no,
                match_mode=match_mode,
                source_file_ids=source_file_ids,
                date_start=date_start,
                date_end=date_end,
                direction_mode=direction_mode,
                success_filter=success_filter,
                cash_filter=cash_filter,
                metric=metric,
                limit=limit,
                dedupe_same_holder_same_fact=dedupe_same_holder_same_fact,
                counterparty_group_mode=counterparty_group_mode,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def resolve_owner_scope(
        self,
        case_id: str,
        *,
        holder_name: str = "",
        id_no: str = "",
        account_keys: Optional[Sequence[str]] = None,
        include_candidate_accounts: bool = True,
        candidate_min_turnover: float = 100000.0,
        limit: int = 50,
    ) -> dict:
        try:
            return self._repository.resolve_owner_scope(
                case_id,
                holder_name=holder_name,
                id_no=id_no,
                account_keys=account_keys,
                include_candidate_accounts=include_candidate_accounts,
                candidate_min_turnover=candidate_min_turnover,
                limit=limit,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def compare_analysis_scopes(self, case_id: str) -> dict:
        try:
            return self._repository.compare_analysis_scopes(case_id)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def fund_analysis_probe(
        self,
        case_id: str,
        *,
        probe_type: str = "discovery",
        holder_name: str = "",
        id_no: str = "",
        account_keys: Optional[Sequence[str]] = None,
        include_candidate_accounts: bool = False,
        candidate_min_turnover: float = 100000.0,
        date_start: str = "",
        date_end: str = "",
        keywords: Optional[Sequence[str]] = None,
        min_amount: float = 0.0,
        limit: int = 20,
    ) -> dict:
        try:
            return self._repository.fund_analysis_probe(
                case_id,
                probe_type=probe_type,
                holder_name=holder_name,
                id_no=id_no,
                account_keys=account_keys,
                include_candidate_accounts=include_candidate_accounts,
                candidate_min_turnover=candidate_min_turnover,
                date_start=date_start,
                date_end=date_end,
                keywords=keywords,
                min_amount=min_amount,
                limit=limit,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def trace_subject_top_outflows(
        self,
        case_id: str,
        *,
        holder_name: str = "",
        id_no: str = "",
        account_keys: Optional[Sequence[str]] = None,
        include_candidate_accounts: bool = False,
        candidate_min_turnover: float = 100000.0,
        date_start: str = "",
        date_end: str = "",
        top_n: int = 20,
        min_amount: float = 0.0,
        dedupe_seed_txn_id: bool = True,
        trace_depth: int = 1,
        time_window_hours: float = 72.0,
        amount_tolerance_ratio: float = 0.05,
        amount_tolerance_abs: float = 100.0,
        downstream_limit_per_seed: int = 5,
    ) -> dict:
        try:
            return self._repository.trace_subject_top_outflows(
                case_id,
                holder_name=holder_name,
                id_no=id_no,
                account_keys=account_keys,
                include_candidate_accounts=include_candidate_accounts,
                candidate_min_turnover=candidate_min_turnover,
                date_start=date_start,
                date_end=date_end,
                top_n=top_n,
                min_amount=min_amount,
                dedupe_seed_txn_id=dedupe_seed_txn_id,
                trace_depth=trace_depth,
                time_window_hours=time_window_hours,
                amount_tolerance_ratio=amount_tolerance_ratio,
                amount_tolerance_abs=amount_tolerance_abs,
                downstream_limit_per_seed=downstream_limit_per_seed,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def classify_missing_counterparty_business(
        self,
        case_id: str,
        *,
        holder_name: str = "",
        id_no: str = "",
        account_keys: Optional[Sequence[str]] = None,
        include_candidate_accounts: bool = False,
        candidate_min_turnover: float = 100000.0,
        date_start: str = "",
        date_end: str = "",
        missing_kind: str = "both",
        min_amount: float = 0.0,
        limit: int = 50,
    ) -> dict:
        try:
            return self._repository.classify_missing_counterparty_business(
                case_id,
                holder_name=holder_name,
                id_no=id_no,
                account_keys=account_keys,
                include_candidate_accounts=include_candidate_accounts,
                candidate_min_turnover=candidate_min_turnover,
                date_start=date_start,
                date_end=date_end,
                missing_kind=missing_kind,
                min_amount=min_amount,
                limit=limit,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def validate_continuation_list(
        self,
        case_id: str,
        *,
        rows: Optional[Sequence[Mapping[str, Any]]] = None,
        required_fields: Optional[Sequence[str]] = None,
        amount_unit: str = "yuan",
        expected_total_amount: float = 0.0,
        expected_txn_count: int = 0,
        amount_tolerance: float = 0.01,
        strict_db_match: bool = True,
    ) -> dict:
        try:
            return self._repository.validate_continuation_list(
                case_id,
                rows=rows,
                required_fields=required_fields,
                amount_unit=amount_unit,
                expected_total_amount=expected_total_amount,
                expected_txn_count=expected_txn_count,
                amount_tolerance=amount_tolerance,
                strict_db_match=strict_db_match,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def build_account_behavior_profile(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        source_file_ids: Optional[Sequence[str]] = None,
        temp_scope_id: str = "",
        scope_source: str = "db",
        date_start: str = "",
        date_end: str = "",
        success_filter: str = "all",
        cash_filter: str = "all",
        large_amount_threshold: float = 20000.0,
    ) -> dict:
        try:
            return self._repository.build_account_behavior_profile(
                case_id,
                account_keys=account_keys,
                source_file_ids=source_file_ids,
                temp_scope_id=temp_scope_id,
                scope_source=scope_source,
                date_start=date_start,
                date_end=date_end,
                success_filter=success_filter,
                cash_filter=cash_filter,
                large_amount_threshold=large_amount_threshold,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def get_materialized_next_hop_candidates(
        self,
        case_id: str,
        *,
        seed_txn_id: str = "",
        seed_account_id: str = "",
        file_ids: Optional[Sequence[str]] = None,
        time_window_sec: int = 7200,
        tolerance_rate: float = 0.03,
        max_candidates: int = 12,
    ) -> dict:
        try:
            return self._repository.get_materialized_next_hop_candidates(
                case_id,
                seed_txn_id=seed_txn_id,
                seed_account_id=seed_account_id,
                file_ids=file_ids,
                time_window_sec=time_window_sec,
                tolerance_rate=tolerance_rate,
                max_candidates=max_candidates,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def get_seed_mixed_fund_tracking(
        self,
        case_id: str,
        *,
        seed_txn_id: str,
        candidate_txn_ids: Optional[Sequence[str]] = None,
        file_ids: Optional[Sequence[str]] = None,
    ) -> dict:
        try:
            return self._repository.get_seed_mixed_fund_tracking(
                case_id,
                seed_txn_id=seed_txn_id,
                candidate_txn_ids=candidate_txn_ids,
                file_ids=file_ids,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def get_case_top_mixed_fund_tracking(self, case_id: str) -> dict:
        try:
            return self._repository.get_case_top_mixed_fund_tracking(case_id)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def get_entity_graph(
        self,
        case_id: str,
        *,
        entity_id: str,
        hops: int = 2,
        edge_types: Optional[Sequence[str]] = None,
        force_refresh: bool = False,
    ) -> dict:
        try:
            return self._repository.get_entity_graph(
                case_id,
                entity_id=entity_id,
                hops=hops,
                edge_types=edge_types,
                force_refresh=force_refresh,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def get_device_links(
        self,
        case_id: str,
        *,
        ip: str = "",
        mac: str = "",
        person_id: str = "",
        cursor: str = "",
        limit: int = 20,
        force_refresh: bool = False,
    ) -> dict:
        try:
            return self._repository.get_device_links(
                case_id,
                ip=ip,
                mac=mac,
                person_id=person_id,
                cursor=cursor,
                limit=limit,
                force_refresh=force_refresh,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def get_branch_voucher_links(
        self,
        case_id: str,
        *,
        voucher_no: str = "",
        teller_no: str = "",
        log_no: str = "",
        cursor: str = "",
        limit: int = 20,
        force_refresh: bool = False,
    ) -> dict:
        try:
            return self._repository.get_branch_voucher_links(
                case_id,
                voucher_no=voucher_no,
                teller_no=teller_no,
                log_no=log_no,
                cursor=cursor,
                limit=limit,
                force_refresh=force_refresh,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def _run_trace_job(self, task_id: str) -> None:
        if self._task_service is None:
            return
        trace_id = ""
        case_id = ""
        try:
            task = self._task_service.get_task(task_id)
            self._task_service.transition_task(
                task_id,
                to_status=TaskStatus.RUNNING,
                progress=1,
                metadata={"started_at": utc_now().isoformat()},
            )
            metadata = self._task_service.read_runtime_payload(task_id, case_id=task.case_id)
            request = metadata.get("request") or {}
            trace_id = str(metadata.get("trace_id") or "").strip()
            case_id = str(task.case_id or request.get("case_id") or "").strip()
            self._repository.run_trace(
                case_id,
                seed_type=str(request.get("seed_type") or ""),
                seed_value=str(request.get("seed_value") or ""),
                depth=int(request.get("depth") or 3),
                time_window_sec=int(request.get("time_window_sec") or 1800),
                tolerance_rate=float(request.get("tolerance_rate") or 0.03),
                file_ids=list(request.get("file_ids") or []),
                trace_id=trace_id,
            )
            self._task_service.transition_task(
                task_id,
                to_status=TaskStatus.SUCCEEDED,
                progress=100,
                metadata={"stage": "completed"},
            )
        except Exception:
            try:
                if case_id and trace_id:
                    self._repository.update_trace_run_status(
                        case_id,
                        trace_id=trace_id,
                        status="failed",
                        summary={"error": _ANALYSIS_TRACE_FAILED},
                    )
            except Exception:
                pass
            try:
                self._task_service.transition_task(
                    task_id,
                    to_status=TaskStatus.FAILED,
                    progress=100,
                    error=_ANALYSIS_TRACE_FAILED,
                )
            except InvalidTaskTransitionError:
                pass
        finally:
            with self._lock:
                self._threads.pop(task_id, None)

    def run_trace(
        self,
        case_id: str,
        *,
        seed_type: str,
        seed_value: str,
        depth: int,
        time_window_sec: int,
        tolerance_rate: float,
        file_ids: Optional[Sequence[str]] = None,
        mode: str = "sync",
    ) -> dict:
        try:
            if _is_async_mode(mode) and self._task_service is not None:
                shell = self._repository.create_trace_run_shell(
                    case_id,
                    seed_type=seed_type,
                    seed_value=seed_value,
                    depth=depth,
                    time_window_sec=time_window_sec,
                    tolerance_rate=tolerance_rate,
                    status="queued",
                )
                task = self._task_service.create_task(
                    task_type=TaskType.ANALYSIS_TRACE,
                    case_id=case_id,
                    metadata={
                        "trace_id": shell["trace_id"],
                        "request": {
                            "case_id": case_id,
                            "seed_type": seed_type,
                        "seed_value": seed_value,
                        "depth": depth,
                        "time_window_sec": time_window_sec,
                        "tolerance_rate": tolerance_rate,
                            "file_ids": list(file_ids or []),
                        },
                    },
                )
                thread = threading.Thread(
                    target=self._run_trace_job,
                    args=(task.task_id,),
                    daemon=True,
                    name=f"analysis-trace-job-{task.task_id[:8]}",
                )
                with self._lock:
                    self._threads[task.task_id] = thread
                thread.start()
                return {
                    "trace_id": shell["trace_id"],
                    "job_id": task.task_id,
                    "status": "queued",
                    "summary": {},
                    "top_path_ids": [],
                    "stats": {},
                }
            return self._repository.run_trace(
                case_id,
                seed_type=seed_type,
                seed_value=seed_value,
                depth=depth,
                time_window_sec=time_window_sec,
                tolerance_rate=tolerance_rate,
                file_ids=file_ids,
            )
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def get_trace(self, trace_id: str, *, case_id: str) -> dict:
        try:
            trace = self._repository.get_trace(trace_id, case_id=case_id)
        except KeyError as exc:
            raise AnalysisTraceNotFoundError(trace_id) from exc
        return project_analysis_trace_public(case_id=case_id, trace=trace)

    def get_trace_path(self, trace_id: str, path_id: str, *, case_id: str) -> dict:
        try:
            path = self._repository.get_trace_path(trace_id, path_id, case_id=case_id)
        except KeyError as exc:
            raise AnalysisTraceNotFoundError(path_id) from exc
        return project_analysis_trace_path_public(
            case_id=case_id,
            trace_id=trace_id,
            path_id=path_id,
            path=path,
        )

    def _run_refresh_job(self, task_id: str, expected_case_id: str) -> None:
        if self._task_service is None:
            return
        canonical_job_id = ""
        try:
            canonical_job_id = resolve_analysis_refresh_job_id(
                case_id=expected_case_id,
                job_id=task_id,
                task_service=self._task_service,
            )
            if not canonical_job_id:
                raise RuntimeError(_ANALYSIS_REFRESH_FAILED)
            self._task_service.transition_task(
                canonical_job_id,
                to_status=TaskStatus.RUNNING,
                progress=1,
                metadata={"started_at": utc_now().isoformat(), "stage": "starting"},
            )
            metadata = self._task_service.read_runtime_payload(canonical_job_id, case_id=expected_case_id)
            request = metadata.get("request") or {}
            case_id = str(expected_case_id or "").strip()
            if (
                type(request) is not dict
                or request.get("case_id") != case_id
                or type(request.get("force_refresh")) is not bool
            ):
                raise RuntimeError(_ANALYSIS_REFRESH_FAILED)
            self._task_service.transition_task(
                canonical_job_id,
                to_status=TaskStatus.RUNNING,
                progress=10,
                metadata={"stage": "refreshing_analysis"},
            )
            result = self._repository.refresh_case_analysis(
                case_id,
                force_refresh=request["force_refresh"],
            )
            if (
                type(result) is not dict
                or str(result.get("case_id") or "").strip() != case_id
                or result.get("status") != "succeeded"
            ):
                raise RuntimeError(_ANALYSIS_REFRESH_FAILED)
            self._task_service.transition_task(
                canonical_job_id,
                to_status=TaskStatus.RUNNING,
                progress=90,
                metadata={"stage": "finalizing"},
            )
            self._task_service.transition_task(
                canonical_job_id,
                to_status=TaskStatus.SUCCEEDED,
                progress=100,
                metadata={"stage": "completed"},
            )
        except Exception:
            if canonical_job_id:
                try:
                    self._task_service.transition_task(
                        canonical_job_id,
                        to_status=TaskStatus.FAILED,
                        progress=100,
                        error=_ANALYSIS_REFRESH_FAILED,
                    )
                except InvalidTaskTransitionError:
                    pass
        finally:
            with self._lock:
                self._threads.pop(task_id, None)

    def refresh_case_analysis(self, case_id: str, *, force_refresh: bool = False, mode: str = "sync") -> dict:
        try:
            if _is_async_mode(mode) and self._task_service is not None:
                task = self._task_service.create_task(
                    task_type=TaskType.ANALYSIS_REFRESH,
                    case_id=case_id,
                    metadata={"request": {"case_id": case_id, "force_refresh": bool(force_refresh)}},
                )
                job_id = resolve_analysis_refresh_job_id(
                    case_id=case_id,
                    job_id=task.task_id,
                    task_service=self._task_service,
                )
                if not job_id:
                    return {"job_id": ""}
                thread = threading.Thread(
                    target=self._run_refresh_job,
                    args=(job_id, case_id),
                    daemon=True,
                    name=f"analysis-refresh-job-{job_id[:8]}",
                )
                with self._lock:
                    self._threads[job_id] = thread
                thread.start()
                return {"job_id": job_id}
            return self._repository.refresh_case_analysis(case_id, force_refresh=force_refresh)
        except KeyError as exc:
            raise AnalysisCaseNotFoundError(case_id) from exc

    def _maintenance_loop(self) -> None:
        if self._maintenance_startup_delay_s > 0 and self._maintenance_stop.wait(self._maintenance_startup_delay_s):
            return
        while not self._maintenance_stop.is_set():
            try:
                result = self.run_temp_scope_janitor()
                total_cleaned = int(result.get("affected_scope_count") or 0)
                if total_cleaned > 0:
                    log_closed_diagnostic(
                        self._logger,
                        logging.INFO,
                        topic="analysis_maintenance",
                        code="completed",
                        numeric={
                            "case_count": int(result.get("case_count") or 0),
                            "affected_scope_count": total_cleaned,
                        },
                    )
            except Exception:
                log_closed_diagnostic(
                    self._logger,
                    logging.WARNING,
                    topic="analysis_maintenance",
                    code="janitor_failed",
                )
            try:
                runtime_result = self.run_runtime_retention_janitor()
                total_deleted = int(runtime_result.get("deleted_count") or 0)
                total_stale = int(runtime_result.get("stale_count") or 0)
                if total_deleted > 0 or total_stale > 0:
                    log_closed_diagnostic(
                        self._logger,
                        logging.INFO,
                        topic="analysis_maintenance",
                        code="completed",
                        numeric={
                            "case_count": int(runtime_result.get("case_count") or 0),
                            "stale_count": total_stale,
                            "deleted_count": total_deleted,
                        },
                    )
            except Exception:
                log_closed_diagnostic(
                    self._logger,
                    logging.WARNING,
                    topic="analysis_maintenance",
                    code="retention_failed",
                )
            interval = max(1, int(self._maintenance_interval_s or 0))
            if self._maintenance_stop.wait(interval):
                break

    def start_background_maintenance(self, *, interval_s: int, startup_delay_s: int = 0) -> None:
        normalized_interval = max(0, int(interval_s or 0))
        self._maintenance_interval_s = normalized_interval
        self._maintenance_startup_delay_s = max(0, int(startup_delay_s or 0))
        if normalized_interval <= 0:
            return
        with self._lock:
            thread = self._maintenance_thread
            if thread is not None and thread.is_alive():
                return
            self._maintenance_stop.clear()
            self._maintenance_thread = threading.Thread(
                target=self._maintenance_loop,
                daemon=True,
                name="analysis-temp-scope-janitor",
            )
            self._maintenance_thread.start()

    def shutdown(self) -> None:
        self._maintenance_stop.set()
        with self._lock:
            thread = self._maintenance_thread
        if thread is not None and thread.is_alive():
            thread.join(timeout=2.0)


def _is_async_mode(mode: str) -> bool:
    return str(mode or "").strip().lower() == "async"
