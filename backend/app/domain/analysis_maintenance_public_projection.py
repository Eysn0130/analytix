from __future__ import annotations

from typing import Literal

from app.tasks.failure_boundary import canonical_task_id
from app.tasks.models import TaskType
from app.tasks.service import TaskService


AnalysisMaintenanceOperation = Literal["analysis_refresh", "feature_mart_materialize"]

_EVIDENCE_PACK_SKILL_IDS = frozenset({"get_evidence_pack"})
_EVIDENCE_PACK_SCOPE_KINDS = frozenset({"evidence_pack"})


def evidence_pack_skill_requires_host_authority(value: object) -> bool:
    return str(value or "").strip() in _EVIDENCE_PACK_SKILL_IDS


def evidence_pack_scope_requires_host_authority(value: object) -> bool:
    return str(value or "").strip() in _EVIDENCE_PACK_SCOPE_KINDS


def evidence_pack_deferred_write_requires_host_authority(
    *,
    kind: object,
    payload: object,
) -> bool:
    normalized_kind = str(kind or "").strip()
    payload_dict = payload if isinstance(payload, dict) else {}
    if normalized_kind == "skill_cache":
        return evidence_pack_skill_requires_host_authority(payload_dict.get("skill_id"))
    if normalized_kind == "query_log":
        return evidence_pack_skill_requires_host_authority(payload_dict.get("tool_name"))
    return False


def project_analysis_maintenance_public(
    *,
    case_id: str,
    operation: AnalysisMaintenanceOperation,
    job_id: str = "",
    task_service: TaskService | None = None,
    allow_queued_job: bool = False,
) -> dict:
    normalized_job_id = (
        resolve_analysis_refresh_job_id(
            case_id=case_id,
            job_id=job_id,
            task_service=task_service,
        )
        if operation == "analysis_refresh" and allow_queued_job
        else ""
    )
    operation_status = "queued" if normalized_job_id else "blocked"
    return {
        "contract": "AnalysisMaintenancePublicBoundaryV1",
        "case_id": str(case_id or "").strip(),
        "operation": operation,
        "operation_status": operation_status,
        "semantic_status": "blocked",
        "blocker": "host_evidence_receipt_required",
        "publication_status": "blocked",
        "fact_answer_allowed": False,
        "job_id": normalized_job_id if operation_status == "queued" else "",
    }


def resolve_analysis_refresh_job_id(
    *,
    case_id: str,
    job_id: object,
    task_service: TaskService | None,
) -> str:
    if task_service is None:
        return ""
    try:
        normalized_job_id = canonical_task_id(job_id)
        task = task_service.get_public_task(normalized_job_id, case_id=str(case_id or "").strip())
    except Exception:
        # Public maintenance projection is fail-closed. Registry/persistence
        # failures and hostile identifiers must not become public job handles.
        return ""
    if (
        task.task_id != normalized_job_id
        or task.task_type != TaskType.ANALYSIS_REFRESH
        or task.case_id != str(case_id or "").strip()
    ):
        return ""
    return normalized_job_id


def project_analysis_scope_cache_public(*, case_id: str) -> dict:
    return {
        "contract": "AnalysisScopeCachePublicBoundaryV1",
        "case_id": str(case_id or "").strip(),
        "operation_status": "blocked",
        "semantic_status": "blocked",
        "blocker": "host_evidence_receipt_required",
        "publication_status": "blocked",
        "fact_answer_allowed": False,
        "items": [],
    }


def project_analysis_evidence_pack_public(*, case_id: str) -> dict:
    return {
        "contract": "AnalysisEvidencePackPublicBoundaryV1",
        "case_id": str(case_id or "").strip(),
        "operation": "evidence_pack_materialize",
        "operation_status": "blocked",
        "semantic_status": "blocked",
        "blocker": "host_evidence_receipt_required",
        "publication_status": "blocked",
        "fact_answer_allowed": False,
        "coverage_status": "unverified",
        "coverage_completeness": "unknown",
        "checked_scope": "none",
        "data": {},
        "evidence_receipts": [],
        "claim_records": [],
    }


__all__ = [
    "evidence_pack_deferred_write_requires_host_authority",
    "evidence_pack_scope_requires_host_authority",
    "evidence_pack_skill_requires_host_authority",
    "project_analysis_evidence_pack_public",
    "project_analysis_maintenance_public",
    "project_analysis_scope_cache_public",
    "resolve_analysis_refresh_job_id",
]
