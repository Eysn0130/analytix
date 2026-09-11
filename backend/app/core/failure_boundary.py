from __future__ import annotations

import errno
from typing import Any


IMPORT_FILE_PROCESSING_FAILED = "import_file_processing_failed"
IMPORT_FILE_CANCELLED = "import_file_cancelled"
IMPORT_FILE_WARNING = "import_file_warning"
IMPORT_FILE_REGISTERED = "import_file_registered"
IMPORT_ARCHIVE_PREVIEW_FAILED = "import_archive_preview_failed"
IMPORT_JOB_FAILED = "import_job_failed"
IMPORT_JOB_CANCELLED = "import_job_cancelled"
CLEANING_FAILED = "cleaning_failed"
CLEANING_CANCELLED = "cleaning_cancelled"
_TASK_FAILURE_CODES = frozenset(
    {
        "task_failed",
        "task_canceled",
        "task_interrupted",
        "task_runtime_payload_unavailable",
    }
)


def project_import_error(value: Any, *, status: Any = "") -> str:
    """Project untrusted import diagnostics to a stable persistence/public code."""
    if value is None or (type(value) is str and value == ""):
        return ""
    if type(value) is str:
        if value == IMPORT_FILE_PROCESSING_FAILED:
            return IMPORT_FILE_PROCESSING_FAILED
        if value == IMPORT_FILE_CANCELLED:
            return IMPORT_FILE_CANCELLED
        if value == IMPORT_FILE_WARNING:
            return IMPORT_FILE_WARNING
    normalized = status.strip().lower() if type(status) is str else ""
    if normalized in {"canceled", "cancelled", "已取消"}:
        return IMPORT_FILE_CANCELLED
    if normalized in {"succeeded", "done", "completed", "已完成"}:
        return IMPORT_FILE_WARNING
    return IMPORT_FILE_PROCESSING_FAILED


def project_import_note(value: Any) -> str:
    if value is None or (type(value) is str and value == ""):
        return ""
    if type(value) is str and value == IMPORT_FILE_REGISTERED:
        return IMPORT_FILE_REGISTERED
    return IMPORT_FILE_WARNING


def project_import_job_error(value: Any, *, status: Any = "") -> str:
    if value is None or (type(value) is str and value == ""):
        return ""
    if type(value) is str:
        if value in _TASK_FAILURE_CODES:
            return value
        if value == IMPORT_JOB_CANCELLED:
            return IMPORT_JOB_CANCELLED
        if value == IMPORT_JOB_FAILED:
            return IMPORT_JOB_FAILED
    normalized = status.strip().lower() if type(status) is str else ""
    if normalized in {"canceled", "cancelled"}:
        return IMPORT_JOB_CANCELLED
    return IMPORT_JOB_FAILED


def project_cleaning_error(value: Any, *, status: Any = "") -> str:
    """Project legacy or current cleaning diagnostics without parsing their text."""
    if value is None or (type(value) is str and value == ""):
        return ""
    if type(value) is str:
        if value == CLEANING_CANCELLED:
            return CLEANING_CANCELLED
        if value == CLEANING_FAILED:
            return CLEANING_FAILED
    normalized = status.strip().lower() if type(status) is str else ""
    if normalized in {"canceled", "cancelled", "已取消"}:
        return CLEANING_CANCELLED
    return CLEANING_FAILED


def private_exception_is_retryable(exc: BaseException) -> bool:
    """Classify retryability in memory; callers must never persist exception text."""
    if isinstance(exc, (TimeoutError, ConnectionError)):
        return True
    if isinstance(exc, OSError):
        try:
            error_number = exc.errno
        except Exception:
            error_number = None
        if error_number in {
            errno.EAGAIN,
            errno.EBUSY,
            errno.ECONNABORTED,
            errno.ECONNRESET,
            errno.ETIMEDOUT,
        }:
            return True
    try:
        message = str(exc).lower()
    except Exception:
        return False
    return any(
        marker in message
        for marker in (
            "timeout",
            "temporarily unavailable",
            "database is locked",
            "resource busy",
            "connection reset",
            "connection aborted",
        )
    )


__all__ = [
    "CLEANING_CANCELLED",
    "CLEANING_FAILED",
    "IMPORT_ARCHIVE_PREVIEW_FAILED",
    "IMPORT_FILE_CANCELLED",
    "IMPORT_FILE_PROCESSING_FAILED",
    "IMPORT_FILE_REGISTERED",
    "IMPORT_FILE_WARNING",
    "IMPORT_JOB_CANCELLED",
    "IMPORT_JOB_FAILED",
    "private_exception_is_retryable",
    "project_cleaning_error",
    "project_import_error",
    "project_import_job_error",
    "project_import_note",
]
