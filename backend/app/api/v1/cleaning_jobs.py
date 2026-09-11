from __future__ import annotations

from math import ceil
from typing import Dict, Optional

from fastapi import APIRouter, Depends, Query
from fastapi.responses import JSONResponse

from app.api.data_analysis_deps import get_case_service, get_cleaning_service
from app.core.data_engine_client import data_engine_product_error
from app.domain.case_service import CaseDeletedError, CaseNotFoundError, CaseService
from app.domain.cleaning_service import (
    CleaningJobAlreadyActiveError,
    CleaningJobNotCancellableError,
    CleaningJobNotFoundError,
    CleaningNativeUnavailableError,
    CleaningService,
)
from app.middleware.request_logging import get_request_id
from app.schemas.cases import PaginationMeta
from app.schemas.cleaning_jobs import CleaningJobDTO, CleaningJobReq, CleaningResetReq
from app.tasks.models import TaskStatus
from app.utils.time import utc_now

router = APIRouter(prefix="/cleaning/jobs", tags=["cleaning"])


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


def _internal_error() -> JSONResponse:
    return _error(
        status_code=500,
        code="INTERNAL_ERROR",
        message="unexpected server error",
        retryable=True,
    )


def _known_runtime_error(exc: Exception) -> Optional[JSONResponse]:
    try:
        data_engine_error = data_engine_product_error(exc)
    except Exception:
        return None
    if data_engine_error is None:
        return None
    return _error(**data_engine_error)


def _to_job_dto(task) -> dict:
    dto = CleaningJobDTO(
        job_id=task.task_id,
        case_id=task.case_id or "",
        status=task.status.value,
        progress=task.progress,
        cleaned_rows=None,
        summary={},
        error=task.error,
        created_at=task.created_at.isoformat(),
        updated_at=task.updated_at.isoformat(),
    )
    return dto.model_dump()


@router.post("")
def create_cleaning_job(
    payload: CleaningJobReq,
    case_service: CaseService = Depends(get_case_service),
    cleaning_service: CleaningService = Depends(get_cleaning_service),
):
    try:
        case = case_service.get_case(payload.case_id)
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

    try:
        job = cleaning_service.create_job(
            case_id=payload.case_id,
            steps=payload.steps,
            force_rebuild=payload.force_rebuild,
        )
    except ValueError:
        return _error(status_code=400, code="INVALID_ARGUMENT", message="invalid cleaning request")
    except CleaningJobAlreadyActiveError:
        return _error(
            status_code=409,
            code="CLEANING_JOB_ACTIVE",
            message="cleaning job already active",
        )
    except CleaningNativeUnavailableError:
        return _error(
            status_code=503,
            code="NATIVE_CLEANING_UNAVAILABLE",
            message="native cleaning unavailable",
            retryable=True,
        )
    except Exception as exc:
        known = _known_runtime_error(exc)
        if known is not None:
            return known
        return _internal_error()

    return _ok(_to_job_dto(job), status_code=202)


@router.get("")
def list_cleaning_jobs(
    case_id: str = Query(..., min_length=1),
    page: int = Query(default=1, ge=1),
    page_size: int = Query(default=50, ge=1, le=500),
    status: Optional[TaskStatus] = Query(default=None),
    cleaning_service: CleaningService = Depends(get_cleaning_service),
):
    try:
        items, total = cleaning_service.list_jobs(
            case_id=case_id,
            page=page,
            page_size=page_size,
            status=status,
        )
    except Exception as exc:
        known = _known_runtime_error(exc)
        if known is not None:
            return known
        return _internal_error()

    total_pages = ceil(total / page_size) if total > 0 else 0
    page_info = PaginationMeta(
        page=page,
        page_size=page_size,
        total=total,
        total_pages=total_pages,
        has_next=page < total_pages,
    )
    return _ok(
        {
            "items": [_to_job_dto(item) for item in items],
            "page": page_info.model_dump(),
        }
    )


@router.get("/{job_id}")
def get_cleaning_job(
    job_id: str,
    case_id: str = Query(..., min_length=1),
    cleaning_service: CleaningService = Depends(get_cleaning_service),
):
    try:
        job = cleaning_service.get_job(job_id, case_id=case_id)
    except CleaningJobNotFoundError:
        return _error(
            status_code=404,
            code="JOB_NOT_FOUND",
            message="cleaning job not found",
        )
    return _ok(_to_job_dto(job))


@router.post("/{job_id}/reset")
def reset_cleaning_job(
    job_id: str,
    case_id: str = Query(..., min_length=1),
    payload: Optional[CleaningResetReq] = None,
    cleaning_service: CleaningService = Depends(get_cleaning_service),
):
    try:
        job = cleaning_service.reset_job(job_id, case_id=case_id, reason=(payload.reason if payload else ""))
    except CleaningJobNotFoundError:
        return _error(
            status_code=404,
            code="JOB_NOT_FOUND",
            message="cleaning job not found",
        )
    except Exception as exc:
        known = _known_runtime_error(exc)
        if known is not None:
            return known
        return _internal_error()

    return _ok(_to_job_dto(job), status_code=202)


@router.post("/{job_id}/cancel")
def cancel_cleaning_job(
    job_id: str,
    case_id: str = Query(..., min_length=1),
    cleaning_service: CleaningService = Depends(get_cleaning_service),
):
    try:
        _ = cleaning_service.cancel_job(job_id, case_id=case_id)
    except CleaningJobNotFoundError:
        return _error(
            status_code=404,
            code="JOB_NOT_FOUND",
            message="cleaning job not found",
        )
    except CleaningJobNotCancellableError:
        return _error(
            status_code=409,
            code="JOB_NOT_CANCELLABLE",
            message="cleaning job is already terminal",
        )
    return _ok({"ok": True})
