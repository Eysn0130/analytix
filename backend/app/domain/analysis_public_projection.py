from __future__ import annotations

import hashlib
from collections.abc import Mapping
from typing import Any


class AnalysisPublicProjectionError(RuntimeError):
    pass


_TRACE_REF_PREFIX = "atrace_v1_"
_PATH_REF_PREFIX = "apath_v1_"
_EVIDENCE_REF_PREFIX = "aevidence_v1_"

_TRACE_REF_DOMAIN = b"AnalytixAnalysisTracePublicRefV1\x00"
_PATH_REF_DOMAIN = b"AnalytixAnalysisPathPublicRefV1\x00"
_EVIDENCE_REF_DOMAIN = b"AnalytixAnalysisEvidencePublicRefV1\x00"

_PUBLIC_TRACE_STATUSES = {
    "queued",
    "running",
    "succeeded",
    "failed",
    "cancelled",
    "interrupted",
}


def _text(value: object) -> str:
    return value.strip() if isinstance(value, str) else ""


def _case_bound_ref(*, prefix: str, domain: bytes, case_id: str, values: tuple[str, ...]) -> str:
    normalized_case_id = _text(case_id)
    normalized_values = tuple(_text(value) for value in values)
    if not normalized_case_id or any(not value for value in normalized_values):
        raise AnalysisPublicProjectionError("analysis_public_projection_rejected")

    digest = hashlib.sha256()
    digest.update(domain)
    for value in (normalized_case_id, *normalized_values):
        encoded = value.encode("utf-8", errors="strict")
        digest.update(len(encoded).to_bytes(8, byteorder="big", signed=False))
        digest.update(encoded)
    # Public refs are case-bound routing projections only. They are neither
    # EvidenceReceipts nor authorization tokens for controlled artifacts.
    return f"{prefix}{digest.hexdigest()}"


def build_analysis_trace_public_ref(*, case_id: str, trace_id: str) -> str:
    return _case_bound_ref(
        prefix=_TRACE_REF_PREFIX,
        domain=_TRACE_REF_DOMAIN,
        case_id=case_id,
        values=(trace_id,),
    )


def build_analysis_path_public_ref(*, case_id: str, trace_id: str, path_id: str) -> str:
    return _case_bound_ref(
        prefix=_PATH_REF_PREFIX,
        domain=_PATH_REF_DOMAIN,
        case_id=case_id,
        values=(trace_id, path_id),
    )


def _build_analysis_evidence_public_ref(*, case_id: str, evidence_id: str) -> str:
    return _case_bound_ref(
        prefix=_EVIDENCE_REF_PREFIX,
        domain=_EVIDENCE_REF_DOMAIN,
        case_id=case_id,
        values=(evidence_id,),
    )


def _public_status(value: object) -> str:
    status = _text(value).lower()
    return status if status in _PUBLIC_TRACE_STATUSES else "unknown"


def _require_mapping(value: object) -> Mapping[str, Any]:
    if not isinstance(value, Mapping):
        raise AnalysisPublicProjectionError("analysis_public_projection_rejected")
    return value


def project_analysis_trace_public(*, case_id: str, trace: object) -> dict[str, Any]:
    source = _require_mapping(trace)
    normalized_case_id = _text(case_id)
    source_case_id = _text(source.get("case_id"))
    if not normalized_case_id or source_case_id != normalized_case_id:
        raise AnalysisPublicProjectionError("analysis_trace_case_binding_mismatch")

    trace_id = _text(source.get("trace_id"))
    trace_ref = build_analysis_trace_public_ref(case_id=normalized_case_id, trace_id=trace_id)
    raw_top_paths = source.get("top_paths")
    if raw_top_paths is None:
        raw_top_paths = []
    if not isinstance(raw_top_paths, list):
        raise AnalysisPublicProjectionError("analysis_public_projection_rejected")

    path_refs: list[str] = []
    seen_path_refs: set[str] = set()
    for item in raw_top_paths:
        path = _require_mapping(item)
        path_id = _text(path.get("path_id"))
        path_ref = build_analysis_path_public_ref(
            case_id=normalized_case_id,
            trace_id=trace_id,
            path_id=path_id,
        )
        if path_ref not in seen_path_refs:
            seen_path_refs.add(path_ref)
            path_refs.append(path_ref)

    return {
        "contract": "AnalysisTracePublicV1",
        "trace_ref": trace_ref,
        "operation_status": _public_status(source.get("status")),
        "path_refs": path_refs,
        "coverage_status": "unverified",
        "publication_status": "blocked",
        "fact_answer_allowed": False,
        "content_access": "controlled_artifact_required",
    }


def project_analysis_trace_run_public(*, case_id: str, result: object) -> dict[str, Any]:
    source = _require_mapping(result)
    raw_top_path_ids = source.get("top_path_ids")
    if raw_top_path_ids is None:
        raw_top_path_ids = []
    if not isinstance(raw_top_path_ids, list):
        raise AnalysisPublicProjectionError("analysis_public_projection_rejected")
    return project_analysis_trace_public(
        case_id=case_id,
        trace={
            "trace_id": source.get("trace_id"),
            "case_id": case_id,
            "status": source.get("status"),
            "top_paths": [{"path_id": path_id} for path_id in raw_top_path_ids],
        },
    )


def _candidate_evidence_ids(source: Mapping[str, Any]) -> list[str]:
    raw_evidence_ids = source.get("evidence_ids")
    raw_evidence_refs = source.get("evidence_refs")
    if raw_evidence_ids is None:
        raw_evidence_ids = []
    if raw_evidence_refs is None:
        raw_evidence_refs = []
    if not isinstance(raw_evidence_ids, list) or not isinstance(raw_evidence_refs, list):
        raise AnalysisPublicProjectionError("analysis_public_projection_rejected")

    candidates: list[str] = []
    for item in raw_evidence_ids:
        evidence_id = _text(item)
        if evidence_id:
            candidates.append(evidence_id)
    for item in raw_evidence_refs:
        evidence = _require_mapping(item)
        evidence_id = _text(evidence.get("evidence_id"))
        if evidence_id:
            candidates.append(evidence_id)
    return candidates


def project_analysis_trace_path_public(
    *,
    case_id: str,
    trace_id: str,
    path_id: str,
    path: object,
) -> dict[str, Any]:
    source = _require_mapping(path)
    normalized_case_id = _text(case_id)
    normalized_trace_id = _text(trace_id)
    normalized_path_id = _text(path_id)
    source_case_id = _text(source.get("case_id"))
    if source_case_id and source_case_id != normalized_case_id:
        raise AnalysisPublicProjectionError("analysis_trace_case_binding_mismatch")
    source_path_id = _text(source.get("path_id"))
    if source_path_id != normalized_path_id:
        raise AnalysisPublicProjectionError("analysis_path_binding_mismatch")

    trace_ref = build_analysis_trace_public_ref(
        case_id=normalized_case_id,
        trace_id=normalized_trace_id,
    )
    path_ref = build_analysis_path_public_ref(
        case_id=normalized_case_id,
        trace_id=normalized_trace_id,
        path_id=normalized_path_id,
    )

    evidence_refs: list[dict[str, str]] = []
    seen_evidence_refs: set[str] = set()
    for evidence_id in _candidate_evidence_ids(source):
        evidence_ref = _build_analysis_evidence_public_ref(
            case_id=normalized_case_id,
            evidence_id=evidence_id,
        )
        if evidence_ref in seen_evidence_refs:
            continue
        seen_evidence_refs.add(evidence_ref)
        evidence_refs.append(
            {
                "evidence_ref": evidence_ref,
                "verification_status": "unresolved",
                "content_access": "controlled_artifact_required",
            }
        )

    return {
        "contract": "AnalysisTracePathPublicV1",
        "trace_ref": trace_ref,
        "path_ref": path_ref,
        "availability_status": "available",
        "coverage_status": "unverified",
        "publication_status": "blocked",
        "fact_answer_allowed": False,
        "evidence_refs": evidence_refs,
        "content_access": "controlled_artifact_required",
    }
