from __future__ import annotations

from typing import Dict, Optional

from fastapi import APIRouter, Depends, Query
from fastapi.responses import JSONResponse

from app.api.data_analysis_deps import get_case_service, get_cleaning_service
from app.domain.case_service import CaseDeletedError, CaseNotFoundError, CaseService
from app.domain.cleaning_service import CleaningService
from app.middleware.request_logging import get_request_id
from app.repositories.cleaning_step_catalog import CLEANING_STEP_DEFINITIONS, get_cleaning_step_definition
from app.schemas.cleaning_insights import (
    CleaningHistoryListDTO,
    CleaningLogListDTO,
    CleaningStepDetailDTO,
    CleaningStepSummaryListDTO,
    CleaningStepSummaryDTO,
)
from app.utils.time import utc_now

router = APIRouter(prefix="/cleaning", tags=["cleaning"])


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


@router.get("/steps")
def list_cleaning_step_summaries(
    case_id: str = Query(..., min_length=1),
    case_service: CaseService = Depends(get_case_service),
    cleaning_service: CleaningService = Depends(get_cleaning_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result

    del cleaning_service
    dto_items = [
        CleaningStepSummaryDTO(
            step=definition.step,
            key=definition.key,
            title=definition.title,
            kind=definition.kind,
            description=definition.description,
            affected_rows=None,
        )
        for definition in CLEANING_STEP_DEFINITIONS
    ]
    boundary = CleaningStepSummaryListDTO(case_id=case_id, items=dto_items)
    return _ok(boundary.model_dump())


@router.get("/history")
def list_cleaning_history(
    case_id: str = Query(..., min_length=1),
    limit: int = Query(default=50, ge=1, le=500),
    case_service: CaseService = Depends(get_case_service),
    cleaning_service: CleaningService = Depends(get_cleaning_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result

    del limit, cleaning_service
    return _ok(CleaningHistoryListDTO(case_id=case_id, items=[]).model_dump())


@router.get("/logs")
def list_cleaning_logs(
    case_id: str = Query(..., min_length=1),
    limit: int = Query(default=200, ge=1, le=1000),
    case_service: CaseService = Depends(get_case_service),
    cleaning_service: CleaningService = Depends(get_cleaning_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result

    del limit, cleaning_service
    return _ok(CleaningLogListDTO(case_id=case_id, items=[]).model_dump())


@router.get("/steps/{step}")
def get_cleaning_step_detail(
    step: int,
    case_id: str = Query(..., min_length=1),
    page: int = Query(default=1, ge=1),
    page_size: int = Query(default=20, ge=1, le=5000),
    offset: Optional[int] = Query(default=None, ge=0),
    case_service: CaseService = Depends(get_case_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result

    try:
        definition = get_cleaning_step_definition(step)
    except KeyError:
        return _error(
            status_code=404,
            code="STEP_NOT_FOUND",
            message="cleaning step not found",
            details={"step": step},
        )
    del page, page_size, offset
    dto = CleaningStepDetailDTO(
        case_id=case_id,
        step=definition.step,
        key=definition.key,
        title=definition.title,
        kind=definition.kind,
        description=definition.description,
        headers=list(definition.headers),
        items=[],
        page=None,
    )
    return _ok(dto.model_dump())
