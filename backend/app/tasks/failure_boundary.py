from __future__ import annotations

import hashlib
import re
from datetime import datetime, timezone
from typing import Any, Mapping, Optional
from uuid import UUID

from app.tasks.models import TaskStatus


CASE_ID_PATTERN = re.compile(r"^[A-Za-z0-9_-]{4,80}$")

_PUBLIC_PHASES = frozenset(
    {
        "queued",
        "starting",
        "running",
        "preparing",
        "reading",
        "processing",
        "importing",
        "cleaning",
        "analyzing",
        "building",
        "exporting",
        "refreshing_analysis",
        "finalizing",
        "completed",
        "failed",
        "canceled",
        "interrupted",
    }
)
_PUBLIC_BOOLEAN_FIELDS = frozenset({"cache_hit", "retryable", "runtime_payload_required"})
_PUBLIC_INTEGER_FIELDS = frozenset(
    {
        "attempt",
        "attempts",
        "max_attempts",
        "build_ms",
    }
)
_PUBLIC_TIMESTAMP_FIELDS = frozenset({"queued_at", "started_at", "finished_at"})
_FAILURE_CODES = frozenset(
    {
        "task_failed",
        "task_canceled",
        "task_interrupted",
        "task_runtime_payload_unavailable",
    }
)
_FAILURE_EVENT_MESSAGES = {
    ("import", "INTERNAL_ERROR"): "import job failed",
    ("import", "JOB_CANCELED"): "import job canceled",
    ("export", "INTERNAL_ERROR"): "export job failed",
    ("export", "JOB_CANCELED"): "export job canceled",
    ("stats_export", "INTERNAL_ERROR"): "stats export job failed",
    ("stats_export", "JOB_CANCELED"): "stats export job canceled",
    ("stats_query", "INTERNAL_ERROR"): "stats query job failed",
    ("stats_query", "JOB_CANCELED"): "stats query job canceled",
    ("flow", "INTERNAL_ERROR"): "flow build job failed",
    ("flow", "JOB_CANCELED"): "flow build job canceled",
}


class InvalidTaskCaseIDError(ValueError):
    pass


def canonical_task_case_id(value: object, *, allow_unbound: bool) -> Optional[str]:
    if value is None:
        if allow_unbound:
            return None
        raise InvalidTaskCaseIDError("task_case_id_required")
    normalized = str(value).strip()
    if not normalized:
        if allow_unbound:
            return None
        raise InvalidTaskCaseIDError("task_case_id_required")
    if normalized != str(value) or CASE_ID_PATTERN.fullmatch(normalized) is None:
        raise InvalidTaskCaseIDError("task_case_id_invalid")
    return normalized


def canonical_task_id(value: object) -> str:
    normalized = str(value or "").strip()
    try:
        parsed = UUID(normalized)
    except (TypeError, ValueError, AttributeError) as exc:
        raise ValueError("task_id_invalid") from exc
    if str(parsed) != normalized:
        raise ValueError("task_id_invalid")
    return normalized


def stable_failure_code(
    status: TaskStatus,
    *,
    interrupted: bool = False,
    payload_unavailable: bool = False,
) -> Optional[str]:
    if payload_unavailable:
        return "task_runtime_payload_unavailable"
    if interrupted:
        return "task_interrupted"
    if status == TaskStatus.FAILED:
        return "task_failed"
    if status == TaskStatus.CANCELED:
        return "task_canceled"
    return None


def stable_failure_phase(status: TaskStatus, *, interrupted: bool = False) -> Optional[str]:
    if interrupted:
        return "interrupted"
    if status == TaskStatus.FAILED:
        return "failed"
    if status == TaskStatus.CANCELED:
        return "canceled"
    return None


def public_task_metadata(
    metadata: object,
    *,
    status: TaskStatus,
    failure_code: Optional[str] = None,
    failure_phase: Optional[str] = None,
    failure_correlation_id: Optional[str] = None,
    runtime_payload_discarded: bool = False,
    runtime_payload_required: bool = False,
) -> dict[str, Any]:
    source = metadata if isinstance(metadata, Mapping) else {}
    projected: dict[str, Any] = {}

    raw_stage = source.get("stage")
    if isinstance(raw_stage, str) and raw_stage in _PUBLIC_PHASES:
        projected["stage"] = raw_stage

    for key in _PUBLIC_BOOLEAN_FIELDS:
        value = source.get(key)
        if isinstance(value, bool):
            projected[key] = value

    for key in _PUBLIC_INTEGER_FIELDS:
        value = source.get(key)
        if isinstance(value, int) and not isinstance(value, bool) and 0 <= value <= 1_000_000_000:
            projected[key] = value

    for key in _PUBLIC_TIMESTAMP_FIELDS:
        value = _canonical_timestamp(source.get(key))
        if value is not None:
            projected[key] = value

    normalized_code = failure_code if failure_code in _FAILURE_CODES else None
    normalized_phase = failure_phase if failure_phase in _PUBLIC_PHASES else None
    normalized_correlation = _canonical_correlation_id(failure_correlation_id)
    if normalized_code is not None:
        projected["failure_code"] = normalized_code
    if normalized_phase is not None:
        projected["failure_phase"] = normalized_phase
    if normalized_correlation is not None:
        projected["failure_correlation_id"] = normalized_correlation

    if status != TaskStatus.SUCCEEDED and runtime_payload_required:
        projected["runtime_payload_required"] = True
    else:
        projected.pop("runtime_payload_required", None)
    if status in {TaskStatus.SUCCEEDED, TaskStatus.FAILED, TaskStatus.CANCELED} and runtime_payload_discarded:
        projected["runtime_payload_discarded"] = True
    return projected


def existing_failure_correlation_id(metadata: object) -> Optional[str]:
    if not isinstance(metadata, Mapping):
        return None
    return _canonical_correlation_id(metadata.get("failure_correlation_id"))


def migrated_failure_correlation_id(*, task_id: str, failure_code: str) -> str:
    """Return a deterministic UUIDv4-shaped correlation id for legacy rows.

    Runtime failures still receive random correlation ids. Migration cannot do
    so: a crash/restart must project the same bytes and converge on one durable
    generation without persisting the rejected diagnostic text as seed data.
    """

    digest = bytearray(
        hashlib.sha256(
            b"analytix-task-failure-migration-v1\x00"
            + str(task_id or "").encode("ascii", errors="strict")
            + b"\x00"
            + str(failure_code or "").encode("ascii", errors="strict")
        ).digest()[:16]
    )
    digest[6] = (digest[6] & 0x0F) | 0x40
    digest[8] = (digest[8] & 0x3F) | 0x80
    return str(UUID(bytes=bytes(digest)))


def closed_task_failure_event_payload(
    *,
    service: str,
    code: str,
    retryable: bool,
    correlation_id: object,
    details: object = None,
) -> dict[str, Any]:
    normalized_code = code if code in {"INTERNAL_ERROR", "JOB_CANCELED"} else "INTERNAL_ERROR"
    message = _FAILURE_EVENT_MESSAGES.get((service, normalized_code))
    if message is None:
        raise ValueError("task_failure_event_service_invalid")

    public_details: dict[str, Any] = {}
    source_details = details if isinstance(details, Mapping) else {}
    if service == "import":
        for key in ("imported_files", "failed_files"):
            value = source_details.get(key)
            if isinstance(value, int) and not isinstance(value, bool) and 0 <= value <= 1_000_000_000:
                public_details[key] = value
    elif service == "stats_query":
        query = source_details.get("query")
        if query in {"rows", "txn-rows"}:
            public_details["query"] = query

    payload: dict[str, Any] = {
        "code": normalized_code,
        "message": message,
        "retryable": bool(retryable) if normalized_code == "INTERNAL_ERROR" else False,
        "details": public_details,
    }
    normalized_correlation = _canonical_correlation_id(correlation_id)
    if normalized_correlation is not None:
        payload["correlation_id"] = normalized_correlation
    return payload


def _canonical_correlation_id(value: object) -> Optional[str]:
    if not isinstance(value, str):
        return None
    try:
        parsed = UUID(value)
    except (TypeError, ValueError, AttributeError):
        return None
    if parsed.version != 4 or str(parsed) != value:
        return None
    return value


def _canonical_timestamp(value: object) -> Optional[str]:
    if not isinstance(value, str) or len(value) > 40:
        return None
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc).isoformat()
