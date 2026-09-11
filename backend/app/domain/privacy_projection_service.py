from __future__ import annotations

import logging
import threading
from typing import Any, NoReturn

from app.core.execution_authority_health import (
    ExecutionAuthorityHealth,
    execution_authority_health,
    unadmitted_execution_capability_health,
)
from app.core.safe_observability import log_closed_diagnostic
from app.repositories.privacy_projection_repository import PrivacyProjectionRepository


_PRIVACY_PROJECTION_CAPABILITY_UNAVAILABLE = "managed_process_capability_admission_unavailable"


class PrivacyProjectionAlreadyRunningError(RuntimeError):
    pass


class PrivacyProjectionUnavailableError(RuntimeError):
    pass


class PrivacyProjectionService:
    def __init__(self, *, repository: PrivacyProjectionRepository | None = None) -> None:
        self._repository = repository or PrivacyProjectionRepository()
        self._lock = threading.RLock()
        self._threads: dict[str, threading.Thread] = {}
        self._running_status: dict[str, dict[str, Any]] = {}
        self._logger = logging.getLogger("analytix.privacy_projection")

    def get_status(self, case_id: str) -> dict[str, Any]:
        normalized_case_id = str(case_id or "").strip()
        with self._lock:
            running = dict(self._running_status.get(normalized_case_id) or {})
            thread = self._threads.get(normalized_case_id)
            thread_alive = bool(thread is not None and thread.is_alive())
        try:
            status = self._repository.get_status(normalized_case_id)
        except KeyError:
            raise
        except Exception as exc:
            if running:
                return running
            raise RuntimeError(str(exc)) from exc
        if running and (thread_alive or str(status.get("status") or "") not in {"completed", "failed", "disabled", "stale"}):
            status["status"] = "running"
            status["enabled"] = True
            status["progress"] = max(int(status.get("progress") or 0), int(running.get("progress") or 1))
            status["label"] = f"{min(99, int(status.get('progress') or 1))}%"
        return status

    def toggle(self, case_id: str) -> dict[str, Any]:
        self._require_execution_capability()
        normalized_case_id = str(case_id or "").strip()
        if not normalized_case_id:
            raise KeyError("case_id")
        current = self.get_status(normalized_case_id)
        current_status = str(current.get("status") or "").strip()
        if current_status == "running":
            return current
        if bool(current.get("enabled")) and current_status == "completed" and not bool(current.get("requires_refresh")):
            self._run_cli(normalized_case_id, "disable")
            return self.get_status(normalized_case_id)
        return self.start_build(normalized_case_id)

    def get_display_map(self, case_id: str) -> dict[str, str]:
        return self._repository.get_display_map(case_id)

    def get_runtime_health(self) -> dict[str, Any]:
        capability = self._execution_capability_health()
        return {
            "privacy_projection_native_available": False,
            "privacy_projection_native_reason": capability.reason_code,
            "privacy_projection_native_bin": "",
            "privacy_projection_native_bin_source": "",
        }

    def start_build(self, case_id: str) -> dict[str, Any]:
        self._require_execution_capability()
        normalized_case_id = str(case_id or "").strip()
        if not normalized_case_id:
            raise KeyError("case_id")
        self._repository.get_db_path(normalized_case_id)
        with self._lock:
            thread = self._threads.get(normalized_case_id)
            if thread is not None and thread.is_alive():
                return dict(self._running_status.get(normalized_case_id) or self._repository.get_status(normalized_case_id))
            self._running_status[normalized_case_id] = {
                "case_id": normalized_case_id,
                "version": "privacy_projection_v1",
                "status": "running",
                "enabled": True,
                "progress": 1,
                "label": "1%",
                "summary": "准备隐私投影",
                "error": "",
                "updated_at": "",
                "requires_refresh": False,
            }
            thread = threading.Thread(
                target=self._run_build_thread,
                args=(normalized_case_id,),
                daemon=True,
                name="analytix-privacy-projection",
            )
            self._threads[normalized_case_id] = thread
            thread.start()
            return dict(self._running_status[normalized_case_id])

    def _run_build_thread(self, case_id: str) -> None:
        try:
            result = self._run_cli(case_id, "build")
            progress = int(result.get("progress") or 100) if isinstance(result, dict) else 100
            with self._lock:
                self._running_status[case_id] = {
                    **dict(self._running_status.get(case_id) or {}),
                    "case_id": case_id,
                    "status": "completed",
                    "enabled": True,
                    "progress": min(100, max(0, progress)),
                    "label": "已脱敏",
                    "summary": str(result.get("summary") or "") if isinstance(result, dict) else "",
                    "error": "",
                }
        except Exception:
            log_closed_diagnostic(
                self._logger,
                logging.ERROR,
                topic="privacy_projection",
                code="projection_failed",
            )
            self._repository.mark_failed(case_id, "privacy_projection_failed")
            with self._lock:
                self._running_status[case_id] = {
                    **dict(self._running_status.get(case_id) or {}),
                    "case_id": case_id,
                    "status": "failed",
                    "enabled": False,
                    "progress": 0,
                    "label": "脱敏",
                    "summary": "",
                    "error": "privacy_projection_failed",
                }
        finally:
            with self._lock:
                self._threads.pop(case_id, None)

    def _run_cli(self, case_id: str, command: str) -> dict[str, Any]:
        del case_id, command
        self._require_execution_capability()

    @staticmethod
    def _require_execution_capability() -> NoReturn:
        # The former environment/path launcher could receive case ids, database
        # paths, and changed-file ids without a host-issued execution admission.
        # Keep the optional native projection mechanically unavailable until a
        # versioned per-capability receipt is enforced by the shared authority.
        PrivacyProjectionService._execution_capability_health()
        raise PrivacyProjectionUnavailableError(_PRIVACY_PROJECTION_CAPABILITY_UNAVAILABLE)

    @staticmethod
    def _execution_capability_health() -> ExecutionAuthorityHealth:
        return unadmitted_execution_capability_health(execution_authority_health())
