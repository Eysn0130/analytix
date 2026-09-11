from __future__ import annotations

from typing import Dict, Optional

from fastapi import APIRouter, Depends, Query
from fastapi.responses import JSONResponse

from app.api.data_analysis_deps import get_case_service, get_export_service
from app.domain.case_service import CaseDeletedError, CaseNotFoundError, CaseService
from app.domain.controlled_artifact_gate import CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED
from app.domain.export_service import (
    ExportJobNotCancellableError,
    ExportJobNotFoundError,
    ExportService,
)
from app.middleware.request_logging import get_request_id
from app.schemas.export_jobs import ExportJobDTO, ExportReq
from app.utils.time import utc_now

router = APIRouter(prefix="/export", tags=["export"])


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


def _to_job_dto(task) -> dict:
    dto = ExportJobDTO(
        job_id=task.task_id,
        case_id=task.case_id or "",
        status=task.status.value,
        progress=task.progress,
        output_path=None,
        error=task.error,
        artifact_access="controlled_artifact_required",
        publication_status="blocked",
        created_at=task.created_at.isoformat(),
        updated_at=task.updated_at.isoformat(),
    )
    return dto.model_dump()


def _ensure_case_ready(case_service: CaseService, case_id: str) -> Optional[JSONResponse]:
    try:
        case = case_service.get_case(case_id)
    except CaseNotFoundError:
        return _error(
            status_code=404,
            code="CASE_NOT_FOUND",
            message="case not found",
        )
    except CaseDeletedError:
        return _error(
            status_code=409,
            code="CASE_DELETED",
            message="case is deleted",
        )
    if case.get("is_deleted"):
        return _error(
            status_code=409,
            code="CASE_DELETED",
            message="case is deleted",
        )
    return None


def _create_job_response() -> JSONResponse:
    return _error(
        status_code=409,
        code="CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED",
        message=CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED,
        details={"phase": "p0_quarantine", "replacement": "p2_staging_host_publish"},
    )

@router.post("/raw")
async def create_raw_export_job(
    payload: ExportReq,
):
    return _create_job_response()


@router.post("/cleaned")
async def create_cleaned_export_job(
    payload: ExportReq,
):
    return _create_job_response()


@router.get("/jobs/{job_id}")
async def get_export_job(
    job_id: str,
    case_id: str = Query(min_length=1, max_length=80),
    case_service: CaseService = Depends(get_case_service),
    export_service: ExportService = Depends(get_export_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result
    try:
        job = export_service.get_job(job_id, case_id=case_id)
    except ExportJobNotFoundError:
        return _error(
            status_code=404,
            code="JOB_NOT_FOUND",
            message="export job not found",
        )
    return _ok(_to_job_dto(job))


@router.post("/jobs/{job_id}/cancel")
async def cancel_export_job(
    job_id: str,
    case_id: str = Query(min_length=1, max_length=80),
    case_service: CaseService = Depends(get_case_service),
    export_service: ExportService = Depends(get_export_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result
    try:
        _ = export_service.cancel_job(job_id, case_id=case_id)
    except ExportJobNotFoundError:
        return _error(
            status_code=404,
            code="JOB_NOT_FOUND",
            message="export job not found",
        )
    except ExportJobNotCancellableError:
        return _error(
            status_code=409,
            code="JOB_NOT_CANCELLABLE",
            message="export job is already terminal",
        )
    return _ok({"ok": True})
