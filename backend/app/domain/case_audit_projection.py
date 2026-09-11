from __future__ import annotations

from datetime import datetime
from typing import Any, Mapping


CASE_AUDIT_PUBLIC_VERSION = "case_audit_public_v1"
CASE_AUDIT_PUBLIC_ACTIONS = frozenset(
    {
        "analysis.bootstrap",
        "backup_case",
        "create_case",
        "delete_case",
        "open_case",
        "purge_case",
        "purge_import_files",
        "recycle_import_files",
        "restore_case",
        "restore_import_files",
        "stats.skill_query",
        "sync_case_project_doc_metadata",
        "update_case",
    }
)
CASE_AUDIT_CHANGED_FIELDS = frozenset(
    {
        "case_no",
        "case_type",
        "name",
        "org",
        "owner",
        "status",
        "summary",
        "tags",
    }
)

_CASE_AUDIT_SYSTEM_ACTIONS = frozenset(
    {
        "analysis.bootstrap",
        "stats.skill_query",
        "sync_case_project_doc_metadata",
    }
)
_CASE_AUDIT_IDENTIFIER_CHARS = frozenset(
    "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_."
)


def _safe_string(value: Any) -> str:
    try:
        return str(value or "").strip()
    except Exception:
        return ""


def _safe_identifier(value: Any) -> str:
    candidate = _safe_string(value)
    if not candidate or len(candidate) > 128:
        return ""
    if any(char not in _CASE_AUDIT_IDENTIFIER_CHARS for char in candidate):
        return ""
    return candidate


def _safe_time(value: Any) -> str:
    candidate = _safe_string(value)
    if not candidate or len(candidate) > 32:
        return ""
    for pattern in ("%Y-%m-%d %H:%M:%S", "%Y-%m-%dT%H:%M:%S"):
        try:
            return datetime.strptime(candidate, pattern).strftime("%Y-%m-%d %H:%M:%S")
        except ValueError:
            continue
    return ""


def _safe_count(value: Any) -> int | None:
    if isinstance(value, bool):
        return None
    try:
        parsed = int(value)
    except (TypeError, ValueError, OverflowError):
        return None
    if parsed < 0 or parsed > 1_000_000_000:
        return None
    return parsed


def _project_changed_fields(value: Any) -> list[str]:
    if not isinstance(value, (list, tuple, set, frozenset)):
        return []
    projected: set[str] = set()
    for item in value:
        candidate = _safe_string(item)
        if candidate in CASE_AUDIT_CHANGED_FIELDS:
            projected.add(candidate)
    return sorted(projected)


def project_case_audit_record(record: Mapping[str, Any]) -> dict[str, Any]:
    """Return the closed, non-authoritative public form of a case audit row.

    Audit rows are operational metadata. They never carry case facts, source
    paths, file names, evidence identifiers, query parameters, or user-supplied
    error text. Historical rows with wider payloads are deterministically
    reduced to this projection before they are read or rewritten.
    """

    raw_action = _safe_string(record.get("action"))
    action = raw_action if raw_action in CASE_AUDIT_PUBLIC_ACTIONS else "unclassified_event"
    extra = record.get("details")
    if not isinstance(extra, Mapping):
        extra = record.get("extra")
    if not isinstance(extra, Mapping):
        extra = {}

    changed_fields = _project_changed_fields(extra.get("changed_fields", extra.get("fields")))
    affected_count = _safe_count(extra.get("affected_count"))
    is_public_projection = record.get("event_version") == CASE_AUDIT_PUBLIC_VERSION
    if is_public_projection:
        accepted_detail_keys = {"affected_count", "changed_fields", "restricted_details_withheld"}
        restricted_details_withheld = bool(extra.get("restricted_details_withheld"))
    else:
        accepted_detail_keys: set[str] = set()
        if changed_fields:
            accepted_detail_keys.update({"changed_fields", "fields"})
        if affected_count is not None:
            accepted_detail_keys.add("affected_count")
        restricted_details_withheld = bool(
            action == "unclassified_event"
            or record.get("user")
            or record.get("extra")
        )
    restricted_details_withheld = bool(
        restricted_details_withheld
        or any(_safe_string(key) not in accepted_detail_keys for key in extra)
    )

    return {
        "event_version": CASE_AUDIT_PUBLIC_VERSION,
        "time": _safe_time(record.get("time")),
        "actor": "system" if action in _CASE_AUDIT_SYSTEM_ACTIONS else "local_operator",
        "action": action,
        "case_id": _safe_identifier(record.get("case_id")),
        "status": "recorded",
        "details": {
            "affected_count": affected_count,
            "changed_fields": changed_fields,
            "restricted_details_withheld": restricted_details_withheld,
        },
    }
