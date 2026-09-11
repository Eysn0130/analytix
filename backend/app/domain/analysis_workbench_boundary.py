from __future__ import annotations

import hashlib
import re
from typing import Any, Mapping, Sequence


_SQL_ERROR_CLASSES = frozenset(
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
_SQL_EXECUTION_STATUSES = frozenset(
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
_SQL_DIAGNOSTIC_MODES = frozenset({"validate", "empty_result", "performance", "error", "explain"})
_SQL_RESULT_CHECK_STATUSES = frozenset({"not_checked", "rows_observed", "no_rows_observed"})
_SQL_REMEDIATION: dict[str, tuple[str, ...]] = {
    "sql_parse_error": ("use_single_read_only_select", "remove_incomplete_sql_fragments"),
    "sql_parser_unavailable": ("restore_structured_sql_parser",),
    "guardrail_blocked": ("use_current_case_cleaned_or_analysis_tables",),
    "raw_table_blocked": ("use_cleaned_or_analysis_tables",),
    "table_unavailable": ("inspect_current_case_schema", "rebuild_analysis_index_if_authorized"),
    "column_unavailable": ("inspect_current_case_schema", "use_canonical_sql_identifiers"),
    "performance_risk": ("reduce_query_scope", "run_bounded_explain"),
    "empty_result": ("review_query_scope", "treat_empty_as_no_hit_in_checked_scope_only"),
    "execution_error": ("retry_bounded_read_only_diagnostic",),
    "unknown": ("inspect_current_case_schema",),
    "none": ("continue_with_host_evidence_verification",),
}
_TEMP_FAILURE_CODES = frozenset(
    {
        "source_not_registered",
        "source_path_unavailable",
        "excel_prepare_failed",
        "csv_header_read_failed",
        "source_kind_unsupported",
        "no_transaction_sheet",
        "temp_import_failed",
    }
)
_TEMP_SCOPE_ID_RE = re.compile(r"^temp_scope_v1_[a-f0-9]{64}$")
_SQL_QUERY_REF_RE = re.compile(r"^sqlquery_v1_[a-f0-9]{64}$")
_SQL_CAPABILITIES = frozenset(
    {"schema_inventory", "schema_profile", "unindexed_source_audit", "notebook_artifact"}
)


def _text(value: Any) -> str:
    return value.strip() if type(value) is str else ""


def _optional_non_negative_int(value: Any) -> int | None:
    return value if type(value) is int and value >= 0 else None


def _optional_bool(value: Any) -> bool | None:
    return value if type(value) is bool else None


def opaque_case_bound_ref(*, prefix: str, case_id: str, value: Any) -> str:
    safe_prefix = _text(prefix)
    if not safe_prefix or not re.fullmatch(r"[a-z][a-z0-9_]{1,31}", safe_prefix):
        raise ValueError("opaque_ref_prefix_invalid")
    digest = hashlib.sha256(f"{_text(case_id)}\x00{_text(value)}".encode("utf-8")).hexdigest()
    return f"{safe_prefix}_{digest}"


def normalize_case_sql_query_ref(*, case_id: str, query_id: Any) -> str:
    candidate = _text(query_id)
    return opaque_case_bound_ref(prefix="sqlquery_v1", case_id=case_id, value=candidate)


def project_case_sql_diagnostic(
    *,
    case_id: str,
    query_id: str,
    sql_digest: str,
    execution_status: str,
    diagnostic_mode: str,
    error_class: str,
    result_check_status: str = "not_checked",
    plan_summary: Mapping[str, Any] | None = None,
) -> dict[str, Any]:
    normalized_error = _text(error_class)
    if normalized_error not in _SQL_ERROR_CLASSES:
        normalized_error = "execution_error"
    normalized_status = _text(execution_status)
    if normalized_status not in _SQL_EXECUTION_STATUSES:
        normalized_status = "diagnosed_error"
    normalized_mode = _text(diagnostic_mode)
    if normalized_mode not in _SQL_DIAGNOSTIC_MODES:
        normalized_mode = "validate"
    normalized_result_status = _text(result_check_status)
    if normalized_result_status not in _SQL_RESULT_CHECK_STATUSES:
        normalized_result_status = "not_checked"
    normalized_digest = _text(sql_digest).lower()
    if not re.fullmatch(r"[a-f0-9]{64}", normalized_digest):
        normalized_digest = ""
    plan = dict(plan_summary or {})
    return {
        "contract": "CaseSqlDiagnosticBoundaryV2",
        "query_ref": normalize_case_sql_query_ref(case_id=case_id, query_id=query_id),
        "sql_digest": normalized_digest,
        "execution_status": normalized_status,
        "diagnostic_mode": normalized_mode,
        "error_class": normalized_error,
        "result_check_status": normalized_result_status,
        "plan": {
            "full_scan_possible": _optional_bool(plan.get("full_table_scan_possible")),
            "estimated_row_upper_bound": _optional_non_negative_int(
                plan.get("estimated_row_upper_bound")
            ),
        },
        "diagnosis": {
            "classification": normalized_error,
            "corrective_actions": list(_SQL_REMEDIATION[normalized_error]),
        },
        "fact_answer_allowed": False,
        "citation_eligible": False,
        "evidence_status": "unsupported",
        "evidence_ids": [],
        "raw_rows_exposed": False,
        "warnings": [
            {
                "code": "CASE_SQL_DIAGNOSTIC_FACT_INELIGIBLE",
                "severity": "warning",
                "remediation": "obtain_host_verified_evidence_receipt",
            }
        ],
    }


def project_case_sql_capability_boundary(
    *,
    case_id: str,
    capability: str,
    request_token: Any,
) -> dict[str, Any]:
    normalized_capability = _text(capability)
    if normalized_capability not in _SQL_CAPABILITIES:
        normalized_capability = "schema_inventory"
    return {
        "contract": "CaseSqlCapabilityBoundaryV2",
        "capability": normalized_capability,
        "request_ref": opaque_case_bound_ref(
            prefix="sqlrequest_v1",
            case_id=case_id,
            value=request_token,
        ),
        "execution_status": "controlled_artifact_required",
        "result_access": "controlled_artifact_required",
        "schema_status": "not_inspected",
        "tables": [],
        "columns": [],
        "records": [],
        "row_count": None,
        "row_count_status": "not_checked",
        "raw_rows_exposed": False,
        "fact_answer_allowed": False,
        "citation_eligible": False,
        "evidence_status": "unsupported",
        "evidence_ids": [],
        "warnings": [
            {
                "code": "CASE_SQL_CONTROLLED_ARTIFACT_GRANT_REQUIRED",
                "severity": "warning",
                "remediation": "request_authorized_case_bound_artifact",
            }
        ],
    }


def project_workbench_history_item(
    *,
    case_id: str,
    row: Mapping[str, Any],
    host_registry_member: bool = False,
) -> dict[str, Any]:
    tool_name = _text(row.get("tool_name"))
    tool_category = {
        "run_case_sql": "bounded_query",
        "diagnose_case_sql": "diagnostic",
        "explain_case_sql": "diagnostic",
        "preview_case_rows": "bounded_preview",
        "profile_case_schema": "schema_profile",
    }.get(tool_name, "other")
    params = row.get("params_json") if isinstance(row.get("params_json"), Mapping) else {}
    summary = row.get("summary_json") if isinstance(row.get("summary_json"), Mapping) else {}
    error_class = _text(summary.get("error_class"))
    if error_class not in _SQL_ERROR_CLASSES:
        error_class = "unknown"
    execution_status = _text(summary.get("execution_status"))
    if execution_status not in _SQL_EXECUTION_STATUSES:
        execution_status = "result_withheld"
    result_check_status = _text(summary.get("result_check_status"))
    if result_check_status not in _SQL_RESULT_CHECK_STATUSES:
        result_check_status = "not_checked"
    digest = _text(params.get("sql_digest")).lower()
    if not re.fullmatch(r"[a-f0-9]{64}", digest):
        digest = ""
    return {
        "query_ref": (
            _text(row.get("query_id"))
            if host_registry_member and _SQL_QUERY_REF_RE.fullmatch(_text(row.get("query_id")))
            else normalize_case_sql_query_ref(case_id=case_id, query_id=row.get("query_id"))
        ),
        "tool_category": tool_category,
        "execution_status": execution_status,
        "error_class": error_class,
        "result_check_status": result_check_status,
        "sql_digest": digest,
        "fact_answer_allowed": False,
        "citation_eligible": False,
    }


def project_temp_scope_failure(*, case_id: str, source_token: Any, failure_code: str) -> dict[str, Any]:
    normalized_code = _text(failure_code)
    if normalized_code not in _TEMP_FAILURE_CODES:
        normalized_code = "temp_import_failed"
    return {
        "source_ref": opaque_case_bound_ref(prefix="tempsrc_v1", case_id=case_id, value=source_token),
        "failure_code": normalized_code,
        "retryable": normalized_code in {"source_path_unavailable", "excel_prepare_failed", "csv_header_read_failed"},
        "fact_answer_allowed": False,
    }


def project_temp_scope_public(
    *,
    case_id: str,
    scope_id: Any,
    status: Any,
    requested_source_count: Any = None,
    resolved_source_count: Any = None,
    failure_items: Sequence[Mapping[str, Any]] = (),
) -> dict[str, Any]:
    normalized_scope_id = _text(scope_id)
    if not _TEMP_SCOPE_ID_RE.fullmatch(normalized_scope_id):
        normalized_scope_id = opaque_case_bound_ref(
            prefix="temp_scope_v1",
            case_id=case_id,
            value=normalized_scope_id,
        )
    normalized_status = _text(status).lower()
    if normalized_status not in {"active", "expired", "failed"}:
        normalized_status = "failed"
    requested_count = _optional_non_negative_int(requested_source_count)
    resolved_count = _optional_non_negative_int(resolved_source_count)
    safe_failures: list[dict[str, Any]] = []
    for item in list(failure_items or []):
        if not isinstance(item, Mapping):
            continue
        source_ref = _text(item.get("source_ref"))
        failure_code = _text(item.get("failure_code"))
        if not re.fullmatch(r"tempsrc_v1_[a-f0-9]{64}", source_ref):
            continue
        if failure_code not in _TEMP_FAILURE_CODES:
            failure_code = "temp_import_failed"
        safe_failures.append(
            {
                "source_ref": source_ref,
                "failure_code": failure_code,
                "retryable": failure_code
                in {"source_path_unavailable", "excel_prepare_failed", "csv_header_read_failed"},
                "fact_answer_allowed": False,
            }
        )
    operational_status = "ready"
    if normalized_status != "active" or resolved_count in {None, 0}:
        operational_status = "unavailable"
    elif safe_failures:
        operational_status = "partial"
    return {
        "contract": "TempTransactionScopePublicV2",
        "scope_id": normalized_scope_id,
        "status": normalized_status,
        "operational_status": operational_status,
        "requested_source_count": requested_count,
        "resolved_source_count": resolved_count,
        "failure_count": len(safe_failures),
        "failures": safe_failures,
        "fact_answer_allowed": False,
        "citation_eligible": False,
        "evidence_ids": [],
        "content_access": "controlled_artifact_required",
        "raw_details_exposed": False,
    }


__all__ = [
    "opaque_case_bound_ref",
    "normalize_case_sql_query_ref",
    "project_case_sql_capability_boundary",
    "project_case_sql_diagnostic",
    "project_temp_scope_failure",
    "project_temp_scope_public",
    "project_workbench_history_item",
]
