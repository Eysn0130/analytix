from __future__ import annotations

from math import ceil
from typing import Dict, Optional

from fastapi import APIRouter, BackgroundTasks, Depends, Query, Request
from fastapi.responses import JSONResponse

from app.api.data_analysis_deps import get_analysis_service, get_case_service
from app.domain.analysis_service import AnalysisService
from app.domain.controlled_artifact_gate import (
    CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED,
    CONTROLLED_SOURCE_INGESTION_REQUIRED,
)
from app.domain.case_service import (
    CaseDeletedError,
    CaseNotFoundError,
    CaseNotInBinError,
    CaseService,
)
from app.middleware.request_logging import get_request_id
from app.schemas.cases import (
    CaseAuditItemDTO,
    CaseArchiveExportDTO,
    CaseArchiveExportReq,
    CaseCreateReq,
    CaseDetailDTO,
    CaseListItemDTO,
    CaseUpdateReq,
    PaginationMeta,
)
from app.utils.time import utc_now

router = APIRouter(prefix="/cases", tags=["cases"])


def _meta() -> Dict[str, str]:
    return {
        "request_id": get_request_id(),
        "timestamp": utc_now().isoformat(),
    }


def _error(
    *,
    status_code: int,
    code: str,
    message: str,
    retryable: bool = False,
    details: Optional[dict] = None,
) -> JSONResponse:
    return JSONResponse(
        status_code=status_code,
        content={
            **_meta(),
            "error": {
                "code": code,
                "message": message,
                "retryable": retryable,
                "details": details or {},
            },
        },
    )


def _ok(data: dict, *, status_code: int = 200) -> JSONResponse:
    return JSONResponse(status_code=status_code, content={**_meta(), "data": data})


def _queue_case_bootstrap(
    background_tasks: BackgroundTasks,
    analysis_service: AnalysisService,
    case_id: str,
) -> None:
    def _bootstrap() -> None:
        try:
            analysis_service.ensure_case_bootstrap(case_id)
        except Exception:
            return

    background_tasks.add_task(_bootstrap)


@router.get("")
def list_cases(
    page: int = Query(default=1, ge=1),
    page_size: int = Query(default=50, ge=1, le=500),
    keyword: str = Query(default="", max_length=128),
    include_deleted: bool = Query(default=False),
    case_service: CaseService = Depends(get_case_service),
):
    items, total = case_service.list_cases(
        page=page,
        page_size=page_size,
        keyword=keyword,
        include_deleted=include_deleted,
    )

    total_pages = ceil(total / page_size) if total > 0 else 0
    page_info = PaginationMeta(
        page=page,
        page_size=page_size,
        total=total,
        total_pages=total_pages,
        has_next=page < total_pages,
    )

    payload = {
        "items": [CaseListItemDTO(**item).model_dump() for item in items],
        "page": page_info.model_dump(),
    }
    return _ok(payload)


@router.post("")
def create_case(
    payload: CaseCreateReq,
    background_tasks: BackgroundTasks,
    case_service: CaseService = Depends(get_case_service),
    analysis_service: AnalysisService = Depends(get_analysis_service),
):
    created = case_service.create_case(
        case_name=payload.case_name,
        case_number=payload.case_number,
        owner=payload.owner,
        note=payload.note,
        case_type=payload.case_type,
        tags=payload.tags,
    )
    _queue_case_bootstrap(background_tasks, analysis_service, created["case_id"])
    return _ok(CaseListItemDTO(**created).model_dump(), status_code=201)


@router.get("/active")
def get_explicit_case_selection(
    case_id: str = Query(min_length=1, max_length=80),
    case_service: CaseService = Depends(get_case_service),
):
    try:
        case = case_service.get_explicit_case_selection(case_id)
    except CaseNotFoundError:
        return _error(
            status_code=404,
            code="CASE_NOT_FOUND",
            message="case not found",
            details={"case_id": case_id},
        )
    except CaseDeletedError:
        return _error(
            status_code=409,
            code="CASE_DELETED",
            message="case is deleted",
            details={"case_id": case_id},
        )

    return _ok(CaseListItemDTO(**case).model_dump())


@router.get("/{case_id}/audit")
def list_case_audit(
    case_id: str,
    limit: int = Query(default=50, ge=1, le=500),
    case_service: CaseService = Depends(get_case_service),
):
    try:
        items = case_service.list_case_audit(case_id, limit=limit)
    except CaseNotFoundError:
        return _error(status_code=404, code="CASE_NOT_FOUND", message="case not found", details={"case_id": case_id})

    payload = {"items": [CaseAuditItemDTO(**item).model_dump() for item in items]}
    return _ok(payload)


@router.get("/{case_id}")
def get_case(case_id: str, case_service: CaseService = Depends(get_case_service)):
    try:
        case = case_service.get_case(case_id)
    except CaseNotFoundError:
        return _error(status_code=404, code="CASE_NOT_FOUND", message="case not found", details={"case_id": case_id})

    return _ok(CaseDetailDTO(**case).model_dump())


@router.patch("/{case_id}")
def update_case(
    case_id: str,
    payload: CaseUpdateReq,
    background_tasks: BackgroundTasks,
    case_service: CaseService = Depends(get_case_service),
    analysis_service: AnalysisService = Depends(get_analysis_service),
):
    if payload.status is not None and payload.status not in {"active", "archived"}:
        return _error(
            status_code=400,
            code="INVALID_ARGUMENT",
            message="status must be active or archived",
            details={"status": payload.status},
        )

    try:
        updated = case_service.update_case(case_id, payload.model_dump(exclude_unset=True))
    except CaseNotFoundError:
        return _error(status_code=404, code="CASE_NOT_FOUND", message="case not found", details={"case_id": case_id})
    _queue_case_bootstrap(background_tasks, analysis_service, case_id)

    return _ok(CaseListItemDTO(**updated).model_dump())


@router.delete("/{case_id}")
def delete_case(case_id: str, case_service: CaseService = Depends(get_case_service)):
    try:
        case_service.delete_case(case_id)
    except CaseNotFoundError:
        return _error(status_code=404, code="CASE_NOT_FOUND", message="case not found", details={"case_id": case_id})

    return _ok({"ok": True})


@router.delete("/{case_id}/purge")
def purge_case(case_id: str, case_service: CaseService = Depends(get_case_service)):
    try:
        case_service.purge_case(case_id)
    except CaseNotFoundError:
        return _error(status_code=404, code="CASE_NOT_FOUND", message="case not found", details={"case_id": case_id})
    except CaseNotInBinError:
        return _error(
            status_code=409,
            code="CASE_NOT_IN_BIN",
            message="case is not in recycle bin",
            details={"case_id": case_id},
        )

    return _ok({"ok": True})


@router.post("/{case_id}/restore")
def restore_case(case_id: str, case_service: CaseService = Depends(get_case_service)):
    try:
        restored = case_service.restore_case(case_id)
    except CaseNotFoundError:
        return _error(status_code=404, code="CASE_NOT_FOUND", message="case not found", details={"case_id": case_id})

    return _ok(CaseListItemDTO(**restored).model_dump())


@router.post("/{case_id}/archive/export")
def export_case_archive(
    case_id: str,
    payload: CaseArchiveExportReq,
    case_service: CaseService = Depends(get_case_service),
):
    return _error(
        status_code=409,
        code="CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED",
        message=CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED,
        details={"phase": "p0_quarantine", "replacement": "p2_staging_host_publish"},
    )

    # P2 replaces this quarantine with backend-owned staging and host publication.
    try:
        exported = case_service.export_case_archive(case_id, target_dir=payload.target_dir)
    except CaseNotFoundError:
        return _error(status_code=404, code="CASE_NOT_FOUND", message="case not found", details={"case_id": case_id})
    except FileNotFoundError:
        return _error(
            status_code=404,
            code="CASE_ARCHIVE_SOURCE_NOT_FOUND",
            message="case source data not found",
            details={"case_id": case_id},
        )
    except Exception as exc:
        return _error(
            status_code=500,
            code="CASE_ARCHIVE_EXPORT_FAILED",
            message=str(exc) or "export failed",
            retryable=False,
        )

    return _ok(CaseArchiveExportDTO(**exported).model_dump())


@router.post("/archive/import")
def import_case_archive(_request: Request):
    return _error(
        status_code=409,
        code="CONTROLLED_SOURCE_INGESTION_REQUIRED",
        message=CONTROLLED_SOURCE_INGESTION_REQUIRED,
        details={"phase": "p0_quarantine", "replacement": "p2_host_staged_ingestion"},
    )


@router.post("/{case_id}/activate")
def activate_case(case_id: str, case_service: CaseService = Depends(get_case_service)):
    try:
        active = case_service.activate_case(case_id)
    except CaseNotFoundError:
        return _error(status_code=404, code="CASE_NOT_FOUND", message="case not found", details={"case_id": case_id})
    except CaseDeletedError:
        return _error(status_code=409, code="CASE_DELETED", message="case is in recycle bin", details={"case_id": case_id})

    return _ok(CaseListItemDTO(**active).model_dump())
