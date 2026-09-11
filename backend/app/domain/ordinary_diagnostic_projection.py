from __future__ import annotations

import hashlib
import json
import re
from datetime import datetime, timezone
from typing import Any, Mapping, Sequence

from app.domain.analysis_workbench_boundary import opaque_case_bound_ref, project_case_sql_diagnostic

_FILE_ID_RE = re.compile(r"^[a-f0-9]{20}$")
_EVENT_ID_RE = re.compile(
    r"^(?:(?:audit|runlog)_[a-f0-9]{12}|(?:auditref|runlogref)_v1_[a-f0-9]{64})$"
)
_UUID_RE = re.compile(
    r"^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
)
_RUNTIME_ID_RE = re.compile(r"^[a-z][a-z0-9]{1,15}[_-][A-Za-z0-9._:-]{1,96}$")
_HEX_DIGEST_RE = re.compile(r"^[a-f0-9]{64}$")
_SQL_QUERY_REF_RE = re.compile(r"^sqlquery_v1_[a-f0-9]{64}$")
_OPAQUE_RUNTIME_ID_RE = re.compile(r"^(?:jobref|taskref|runref|turnref)_v1_[a-f0-9]{64}$")
_CLEANING_ENGINES = frozenset({"rust_clean_all", "rust_stepwise", "mixed", "python_sql"})
_NATIVE_SEGMENTS = frozenset({"clean-all"})
_LEGACY_REASONS = frozenset({"", "native_unavailable"})
_EVENT_TYPES = frozenset(
    {
        "analysis.runtime_retention.cleaned",
        "llm.turn.completed",
        "llm.turn.failed",
    }
)
_EVENT_STATUSES = frozenset(
    {
        "accepted",
        "blocked",
        "canceled",
        "completed",
        "failed",
        "interrupted",
        "pending",
        "queued",
        "running",
        "started",
        "succeeded",
        "timeout",
    }
)
_EVENT_KINDS = frozenset(
    {
        "query_log",
        "memory_note",
        "repository_artifact",
        "evidence_pack_persistence",
        "report_projection",
        "report_export",
        "skill_cache",
        "workspace_projection",
        "temp_transaction_scope",
        "runtime_retention_janitor",
    }
)
_EVENT_STAGES = frozenset(
    {
        "approval",
        "compact",
        "continuation",
        "final",
        "instruction",
        "maintenance",
        "policy",
        "runtime",
    }
)
_EVENT_SOURCES = frozenset({"live", "recovered", "system"})
_FAILURE_CODES = frozenset(
    {
        "analysis_async_write_failed",
        "analysis_durable_write_failed",
        "analysis_run_failed",
        "source_unavailable",
        "timeout",
        "canceled",
    }
)
_QUERY_STATUSES = frozenset(
    {
        "validated",
        "diagnosed",
        "diagnosed_error",
        "blocked_by_guardrail",
        "explain_succeeded",
        "explain_failed",
        "result_withheld",
    }
)
_QUERY_ERRORS = frozenset(
    {
        "none",
        "sql_parse_error",
        "sql_parser_unavailable",
        "guardrail_blocked",
        "raw_table_blocked",
        "table_unavailable",
        "column_unavailable",
        "performance_risk",
        "empty_result",
        "execution_error",
        "unknown",
    }
)
_QUERY_TOOLS = frozenset(
    {
        "diagnose_case_sql",
        "explain_case_sql",
        "preview_case_rows",
        "profile_case_schema",
        "run_case_sql",
        "stats_skill_query",
    }
)
_QUERY_PARAM_KEYS = frozenset(
    {
        "contract",
        "query_ref",
        "sql_digest",
        "fact_answer_allowed",
        "restricted_details_withheld",
    }
)
_QUERY_SUMMARY_KEYS = frozenset(
    {
        "contract",
        "query_ref",
        "sql_digest",
        "execution_status",
        "diagnostic_mode",
        "error_class",
        "result_check_status",
        "plan",
        "diagnosis",
        "fact_answer_allowed",
        "citation_eligible",
        "evidence_status",
        "evidence_ids",
        "raw_rows_exposed",
        "warnings",
        "restricted_details_withheld",
    }
)
_CLEANING_COUNT_KEYS = frozenset(
    {
        "total_rows",
        "scope_rows",
        "invalid",
        "duplicate",
        "failed",
        "reversal",
        "dc_normalized",
        "dc_inferred",
        "card_filled",
        "account_invalid",
        "account_suffix",
        "account_info_filled",
        "rows_affected_total",
        "duration_ms",
        "amount_updated",
        "balance_updated",
        "amount_failed",
        "balance_failed",
        "rows_affected",
    }
)
_EVENT_COUNT_KEYS = frozenset(
    {
        "attempt",
        "deleted_count",
        "failure_count",
        "job_count",
        "processed_count",
        "record_count",
        "row_count",
        "scanned_count",
        "skipped_count",
        "stale_count",
    }
)
_CLEANING_EVENTS = frozenset(
    {
        "cleaning.job.queued",
        "cleaning.job.progress",
        "cleaning.job.log",
        "cleaning.job.completed",
        "cleaning.job.failed",
    }
)
_CLEANING_LEVELS = frozenset({"debug", "info", "warn", "error", "success", "progress"})
_CLEANING_MESSAGES = {
    "CLEANING_JOB_ACCEPTED": "cleaning job accepted",
    "CLEANING_JOB_PROGRESS": "cleaning job progress",
    "CLEANING_STEP_PROGRESS": "cleaning step progress",
    "CLEANING_OPERATIONAL_LOG": "cleaning operational log",
    "CLEANING_STEP_LOG": "cleaning step log",
    "CLEANING_JOB_COMPLETED": "cleaning completed",
    "CLEANING_JOB_FAILED": "cleaning failed",
}
_CLEANING_COUNTER_KEYS = frozenset(
    {"requested_steps", "cleaned_rows", "invalid", "failed", "reversal", "step"}
)


def project_cleaning_summary_diagnostic(value: object) -> dict[str, Any]:
    source = value if isinstance(value, Mapping) else {}
    file_ids = _safe_file_ids(source.get("file_ids"))
    existing_binding = _text(source.get("scope_binding_hash")).lower()
    if re.fullmatch(r"[a-f0-9]{64}", existing_binding) is None:
        existing_binding = cleaning_scope_binding_hash(file_ids)
    projected: dict[str, Any] = {
        "contract": "CleaningOperationalSummaryV2",
        "scope_binding_hash": existing_binding,
        "scope_file_count": (
            len(file_ids) if file_ids else _optional_non_negative_int(source.get("scope_file_count"))
        ),
        "steps_executed": _safe_steps(source.get("steps_executed")),
        "cleaning_engine": _enum(source.get("cleaning_engine"), _CLEANING_ENGINES, ""),
        "native_segments": [
            item for item in _safe_text_list(source.get("native_segments")) if item in _NATIVE_SEGMENTS
        ],
        "python_fallback_steps": _safe_steps(source.get("python_fallback_steps")),
        "legacy_python_cleaning": _optional_bool(source, "legacy_python_cleaning"),
        "legacy_python_cleaning_steps": _safe_steps(source.get("legacy_python_cleaning_steps")),
        "legacy_python_cleaning_reason": _enum(
            source.get("legacy_python_cleaning_reason"),
            _LEGACY_REASONS,
            "native_unavailable" if source.get("legacy_python_cleaning") else "",
        ),
        "native_clean_all_attempted": _optional_bool(source, "native_clean_all_attempted"),
        "native_clean_all_failed": _optional_bool(source, "native_clean_all_failed"),
        "fact_answer_allowed": False,
        "citation_eligible": False,
        "restricted_details_withheld": bool(source.get("restricted_details_withheld"))
        or any(str(key) not in _cleaning_allowed_keys() for key in source),
    }
    for key in _CLEANING_COUNT_KEYS:
        projected[key] = _optional_non_negative_int(source.get(key))
    return projected


def project_cleaning_history_item(*, case_id: str, row: Mapping[str, Any]) -> dict[str, Any]:
    summary_raw = row.get("summary")
    if isinstance(summary_raw, Mapping):
        summary = dict(summary_raw)
    elif type(summary_raw) is str and summary_raw.strip():
        try:
            decoded = json.loads(summary_raw)
        except (json.JSONDecodeError, ValueError, TypeError):
            decoded = {}
        summary = decoded if isinstance(decoded, dict) else {}
    else:
        summary = {}
    cleaned_at = _safe_timestamp(row.get("cleaned_at"))
    import_at = _safe_timestamp(row.get("import_at"))
    identity = "\x00".join(
        (
            _text(case_id),
            _text(row.get("first_id")),
            _text(row.get("last_id")),
            _text(row.get("cleaned_at")),
        )
    )
    stored_run_ref = _text(row.get("run_ref"))
    run_ref = (
        stored_run_ref
        if re.fullmatch(r"cleanrun_v2_[a-f0-9]{64}", stored_run_ref)
        else "cleanrun_v1_" + hashlib.sha256(identity.encode("utf-8")).hexdigest()
    )
    return {
        "run_id": run_ref,
        "cleaned_at": cleaned_at,
        "import_at": import_at,
        "duration_ms": _optional_non_negative_int(row.get("duration_ms")),
        "scope_rows": _optional_non_negative_int(row.get("scope_rows")),
        "file_count": _optional_non_negative_int(row.get("file_count")),
        "summary": project_cleaning_summary_diagnostic(summary),
    }


def project_cleaning_log_event(
    *,
    job_id: str,
    event: object,
    timestamp: object = "",
    case_id: str = "",
) -> dict[str, Any]:
    source = event if isinstance(event, Mapping) else {}
    event_name = _enum(source.get("event"), _CLEANING_EVENTS, "cleaning.job.log")
    step = _bounded_optional_int(source.get("step"), lower=1, upper=10)
    progress = _bounded_optional_int(source.get("progress"), lower=0, upper=100)
    if event_name == "cleaning.job.queued":
        message_code = "CLEANING_JOB_ACCEPTED"
    elif event_name == "cleaning.job.progress":
        message_code = "CLEANING_STEP_PROGRESS" if step is not None else "CLEANING_JOB_PROGRESS"
    elif event_name == "cleaning.job.completed":
        message_code = "CLEANING_JOB_COMPLETED"
    elif event_name == "cleaning.job.failed":
        message_code = "CLEANING_JOB_FAILED"
    else:
        message_code = "CLEANING_STEP_LOG" if step is not None else "CLEANING_OPERATIONAL_LOG"
    raw_event_id = _text(source.get("event_id"))
    event_identity = "\x00".join((_text(job_id), raw_event_id, event_name, str(step), str(progress)))
    event_binding_hash = hashlib.sha256(f"cleaning-job\x00{_text(job_id)}".encode("utf-8")).hexdigest()
    projected_event_id = (
        raw_event_id
        if source.get("contract") == "CleaningLogEventV1"
        and source.get("event_binding_hash") == event_binding_hash
        and re.fullmatch(r"cleanlog_v1_[a-f0-9]{64}", raw_event_id)
        else "cleanlog_v1_" + hashlib.sha256(event_identity.encode("utf-8")).hexdigest()
    )
    raw_counters = source.get("counters") if isinstance(source.get("counters"), Mapping) else {}
    counters = {
        key: parsed
        for key in _CLEANING_COUNTER_KEYS
        if (parsed := _optional_non_negative_int(raw_counters.get(key))) is not None
    }
    stage = (
        "queued"
        if event_name == "cleaning.job.queued"
        else "completed"
        if event_name == "cleaning.job.completed"
        else "failed"
        if event_name == "cleaning.job.failed"
        else f"step_{step}"
        if step is not None
        else "running"
        if event_name == "cleaning.job.progress"
        else "runtime"
    )
    return {
        "contract": "CleaningLogEventV1",
        "event_binding_hash": event_binding_hash,
        "event_id": projected_event_id,
        "job_id": _safe_runtime_id(job_id, case_id=case_id, prefix="jobref_v1"),
        "event": event_name,
        "level": _enum(source.get("level"), _CLEANING_LEVELS, "info"),
        "message_code": message_code,
        "message": _CLEANING_MESSAGES[message_code],
        "stage": stage,
        "counters": counters,
        "step": step,
        "progress": progress,
        "timestamp": _safe_timestamp(timestamp or source.get("timestamp")),
    }


def cleaning_scope_binding_hash(file_ids: object) -> str:
    normalized = sorted(set(_safe_file_ids(file_ids)))
    if not normalized:
        return ""
    payload = b"analytix-cleaning-scope-v1\x00" + b"\x00".join(
        item.encode("ascii") for item in normalized
    )
    return hashlib.sha256(payload).hexdigest()


def project_query_log_diagnostic(
    *,
    case_id: str,
    query_id: str,
    params: object,
    summary: object,
) -> tuple[dict[str, Any], dict[str, Any]]:
    raw_params = params if isinstance(params, Mapping) else {}
    raw_summary = summary if isinstance(summary, Mapping) else {}
    digest = _text(raw_params.get("sql_digest")).lower()
    if _HEX_DIGEST_RE.fullmatch(digest) is None:
        digest = ""
    execution_status = _enum(raw_summary.get("execution_status"), _QUERY_STATUSES, "result_withheld")
    error_class = _enum(raw_summary.get("error_class"), _QUERY_ERRORS, "unknown")
    raw_plan = raw_summary.get("plan") if isinstance(raw_summary.get("plan"), Mapping) else {}
    envelope = project_case_sql_diagnostic(
        case_id=case_id,
        query_id=query_id,
        sql_digest=digest,
        execution_status=execution_status,
        diagnostic_mode=_text(raw_summary.get("diagnostic_mode")) or _text(raw_params.get("diagnostic_mode")),
        error_class=error_class,
        result_check_status=(
            _text(raw_summary.get("result_check_status"))
            or _text(raw_summary.get("row_count_status"))
            or "not_checked"
        ),
        plan_summary={
            "full_table_scan_possible": raw_plan.get("full_scan_possible"),
            "estimated_row_upper_bound": raw_plan.get("estimated_row_upper_bound"),
        },
    )
    existing_ref = _text(raw_params.get("query_ref"))
    if (
        raw_params.get("contract") == "AnalysisQueryDiagnosticParamsV1"
        and raw_summary.get("contract") == "CaseSqlDiagnosticBoundaryV2"
        and existing_ref == query_id
        and _text(raw_summary.get("query_ref")) == query_id
        and _SQL_QUERY_REF_RE.fullmatch(query_id)
    ):
        envelope["query_ref"] = query_id
    params_restricted = bool(raw_params.get("restricted_details_withheld")) or any(
        key not in _QUERY_PARAM_KEYS for key in raw_params
    )
    summary_restricted = bool(raw_summary.get("restricted_details_withheld")) or any(
        key not in _QUERY_SUMMARY_KEYS for key in raw_summary
    )
    return (
        {
            "contract": "AnalysisQueryDiagnosticParamsV1",
            "query_ref": envelope["query_ref"],
            "sql_digest": envelope["sql_digest"],
            "fact_answer_allowed": False,
            "restricted_details_withheld": params_restricted,
        },
        {**envelope, "restricted_details_withheld": summary_restricted},
    )


def project_operational_event(
    *,
    event: object,
    run_log: bool,
    case_id: str = "",
) -> dict[str, Any]:
    source = event if isinstance(event, Mapping) else {}
    normalized_case_id = _text(case_id) or _text(source.get("case_id"))
    payload = source.get("payload") if isinstance(source.get("payload"), Mapping) else {}
    raw_type = _text(source.get("event_type"))
    event_type = _safe_event_type(raw_type)
    inferred_status = (
        "failed"
        if raw_type.endswith(".failed")
        else "completed"
        if raw_type.endswith((".completed", ".cleaned"))
        else "started"
        if raw_type.endswith(".started")
        else "pending"
    )
    status = _enum(payload.get("status"), _EVENT_STATUSES, inferred_status)
    failure_code = _enum(payload.get("error"), _FAILURE_CODES, "")
    if not failure_code:
        failure_code = _enum(payload.get("failure_code"), _FAILURE_CODES, "")
    public_payload: dict[str, Any] = {
        "contract": "OperationalDiagnosticEventV1",
        "status": status,
        "kind": _enum(payload.get("kind"), _EVENT_KINDS, ""),
        "failure_code": failure_code,
        "terminal_reason_present": bool(
            payload.get("terminal_reason_present")
            or payload.get("stop_reason")
            or payload.get("stop_code")
            or payload.get("blocked_reason")
        ),
        "fact_answer_allowed": False,
        "citation_eligible": False,
        "restricted_details_withheld": bool(payload.get("restricted_details_withheld"))
        or any(key not in _operational_payload_allowed_keys() for key in payload),
    }
    for key in _EVENT_COUNT_KEYS:
        if key in payload:
            public_payload[key] = _optional_non_negative_int(payload.get(key))
    record: dict[str, Any] = {
        "event_id": _safe_event_id(source.get("event_id"), run_log=run_log),
        "event_type": event_type,
        "task_id": _safe_runtime_id(
            source.get("task_id"),
            case_id=normalized_case_id,
            prefix="taskref_v1",
        ),
        "run_id": _safe_runtime_id(
            source.get("run_id"),
            case_id=normalized_case_id,
            prefix="runref_v1",
        ),
        "turn_id": _safe_runtime_id(
            source.get("turn_id"),
            case_id=normalized_case_id,
            prefix="turnref_v1",
        ),
        "actor_id": "system" if source.get("actor_id") == "system" else "local_operator",
        "actor_role": "system" if source.get("actor_role") == "system" else "operator",
        "tenant_id": "default",
        "timestamp": _safe_timestamp(source.get("timestamp")),
        "payload": public_payload,
    }
    if run_log:
        record.update(
            {
                "event_stage": _enum(source.get("event_stage"), _EVENT_STAGES, "runtime"),
                "source": _enum(source.get("source"), _EVENT_SOURCES, "live"),
            }
        )
    else:
        record.update(
            {
                "artifact_id": "",
                "approval_id": "",
                "trace_id": "",
                "span_id": "",
                "request_id": "",
                "session_id": "",
            }
        )
    return record


def project_query_tool_name(value: object) -> str:
    return _enum(value, _QUERY_TOOLS, "case_query_diagnostic")


def project_diagnostic_timestamp(value: object) -> str:
    return _safe_timestamp(value)


def _safe_event_type(value: str) -> str:
    if value in _EVENT_TYPES:
        return value
    for prefix in ("analysis.async_write.", "analysis.durable_write."):
        if value.startswith(prefix) and value.removeprefix(prefix) in _EVENT_STATUSES:
            return value
    return "analysis.diagnostic.withheld"


def _safe_event_id(value: object, *, run_log: bool) -> str:
    candidate = _text(value).lower()
    expected_prefixes = ("runlog_", "runlogref_v1_") if run_log else ("audit_", "auditref_v1_")
    if _EVENT_ID_RE.fullmatch(candidate) and candidate.startswith(expected_prefixes):
        return candidate
    return ""


def _safe_runtime_id(value: object, *, case_id: str, prefix: str) -> str:
    candidate = _text(value)
    if not candidate:
        return ""
    lowered = candidate.lower()
    if _OPAQUE_RUNTIME_ID_RE.fullmatch(lowered) and lowered.startswith(f"{prefix}_"):
        return lowered
    if _UUID_RE.fullmatch(lowered) or _RUNTIME_ID_RE.fullmatch(candidate):
        return opaque_case_bound_ref(prefix=prefix, case_id=case_id, value=candidate)
    return ""


def _safe_file_ids(value: object) -> list[str]:
    return [item for item in _safe_text_list(value) if _FILE_ID_RE.fullmatch(item)]


def _safe_steps(value: object) -> list[int]:
    if not isinstance(value, Sequence) or isinstance(value, (str, bytes, bytearray)):
        return []
    result: set[int] = set()
    for item in value:
        if type(item) is int and 1 <= item <= 32:
            result.add(item)
    return sorted(result)


def _safe_text_list(value: object) -> list[str]:
    if not isinstance(value, Sequence) or isinstance(value, (str, bytes, bytearray)):
        return []
    result: list[str] = []
    for item in value:
        candidate = _text(item)
        if candidate and len(candidate) <= 64:
            result.append(candidate)
    return result


def _cleaning_allowed_keys() -> frozenset[str]:
    return _CLEANING_COUNT_KEYS | frozenset(
        {
            "contract",
            "file_ids",
            "scope_binding_hash",
            "scope_file_count",
            "steps_executed",
            "cleaning_engine",
            "native_segments",
            "python_fallback_steps",
            "legacy_python_cleaning",
            "legacy_python_cleaning_steps",
            "legacy_python_cleaning_reason",
            "native_clean_all_attempted",
            "native_clean_all_failed",
            "fact_answer_allowed",
            "citation_eligible",
            "restricted_details_withheld",
        }
    )


def _enum(value: object, allowed: frozenset[str], fallback: str) -> str:
    candidate = _text(value)
    return candidate if candidate in allowed else fallback


def _text(value: object) -> str:
    return value.strip() if type(value) is str else ""


def _non_negative_int(value: object) -> int:
    if type(value) is bool:
        return 0
    try:
        parsed = int(value)
    except (TypeError, ValueError, OverflowError):
        return 0
    return min(max(parsed, 0), 1_000_000_000_000)


def _optional_non_negative_int(value: object) -> int | None:
    if type(value) is not int or value < 0 or value > 1_000_000_000_000:
        return None
    return value


def _bounded_optional_int(value: object, *, lower: int, upper: int) -> int | None:
    parsed = _optional_non_negative_int(value)
    if parsed is None or parsed < lower or parsed > upper:
        return None
    return parsed


def _optional_bool(source: Mapping[str, Any], key: str) -> bool | None:
    return bool(source.get(key)) if key in source and type(source.get(key)) is bool else None


def _operational_payload_allowed_keys() -> frozenset[str]:
    return _EVENT_COUNT_KEYS | frozenset(
        {
            "contract",
            "status",
            "kind",
            "failure_code",
            "terminal_reason_present",
            "fact_answer_allowed",
            "citation_eligible",
            "restricted_details_withheld",
        }
    )


def _safe_timestamp(value: object) -> str:
    candidate = _text(value)
    if not candidate or len(candidate) > 40:
        return ""
    try:
        parsed = datetime.fromisoformat(candidate.replace("Z", "+00:00"))
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=timezone.utc)
        return parsed.astimezone(timezone.utc).isoformat()
    except (ValueError, OverflowError, OSError):
        return ""


__all__ = [
    "cleaning_scope_binding_hash",
    "project_cleaning_history_item",
    "project_cleaning_log_event",
    "project_diagnostic_timestamp",
    "project_cleaning_summary_diagnostic",
    "project_operational_event",
    "project_query_log_diagnostic",
    "project_query_tool_name",
]
