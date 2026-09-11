from __future__ import annotations

from datetime import datetime
from math import ceil
from typing import Dict, Optional

from fastapi import APIRouter, Depends, Query, Request
from fastapi.responses import JSONResponse

from app.api.data_analysis_deps import get_import_service
from app.core.failure_boundary import project_import_job_error
from app.domain.controlled_artifact_gate import CONTROLLED_SOURCE_INGESTION_REQUIRED
from app.domain.import_service import (
    ImportJobNotCancellableError,
    ImportJobNotFoundError,
    ImportService,
)
from app.middleware.request_logging import get_request_id
from app.schemas.cases import PaginationMeta
from app.schemas.import_jobs import ImportJobDTO
from app.tasks.failure_boundary import canonical_task_case_id, canonical_task_id
from app.tasks.models import TaskStatus, TaskType
from app.utils.time import utc_now

router = APIRouter(prefix="/import/jobs", tags=["import"])

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


def _to_job_dto(task, *, expected_case_id: str) -> dict:
    bound_case_id = canonical_task_case_id(expected_case_id, allow_unbound=False)
    if task.task_type != TaskType.IMPORT or task.case_id != bound_case_id:
        raise ValueError("import_job_case_binding_invalid")
    job_id = canonical_task_id(task.task_id)
    if not isinstance(task.status, TaskStatus):
        raise ValueError("import_job_status_invalid")
    if type(task.progress) is not int or not 0 <= task.progress <= 100:
        raise ValueError("import_job_progress_invalid")
    if not isinstance(task.created_at, datetime) or task.created_at.tzinfo is None:
        raise ValueError("import_job_created_at_invalid")
    if not isinstance(task.updated_at, datetime) or task.updated_at.tzinfo is None:
        raise ValueError("import_job_updated_at_invalid")

    dto = ImportJobDTO(
        job_id=job_id,
        case_id=bound_case_id,
        status=task.status.value,
        progress=task.progress,
        # TaskRecord.metadata is operational state, not a count receipt. The
        # public TaskService currently strips these fields, and this final
        # projection must remain fail-closed if a future caller bypasses that
        # boundary. A future controlled ingestion path must supply a separate,
        # case-bound authoritative projection before job counts can be shown.
        imported_files=None,
        summary={},
        files=[],
        current_file="",
        error=project_import_job_error(task.error, status=task.status.value),
        created_at=task.created_at.isoformat(),
        updated_at=task.updated_at.isoformat(),
    )
    payload = dto.model_dump()
    return payload


@router.post("")
def create_import_job(
    _request: Request,
):
    return _error(
        status_code=409,
        code="CONTROLLED_SOURCE_INGESTION_REQUIRED",
        message=CONTROLLED_SOURCE_INGESTION_REQUIRED,
        details={"phase": "p0_quarantine", "replacement": "p2_host_staged_ingestion"},
    )

@router.get("")
def list_import_jobs(
    case_id: str = Query(..., min_length=1),
    page: int = Query(default=1, ge=1),
    page_size: int = Query(default=50, ge=1, le=500),
    status: Optional[TaskStatus] = Query(default=None),
    import_service: ImportService = Depends(get_import_service),
):
    try:
        items, total = import_service.list_jobs(
            case_id=case_id,
            page=page,
            page_size=page_size,
            status=status,
        )
        projected_items = [_to_job_dto(item, expected_case_id=case_id) for item in items]
    except Exception:
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
            "items": projected_items,
            "page": page_info.model_dump(),
        }
    )


@router.get("/{job_id}")
def get_import_job(
    job_id: str,
    case_id: str = Query(..., min_length=4, max_length=80, pattern=r"^[A-Za-z0-9_-]+$"),
    import_service: ImportService = Depends(get_import_service),
):
    try:
        job = import_service.get_job(job_id, case_id=case_id)
    except ImportJobNotFoundError:
        return _error(
            status_code=404,
            code="JOB_NOT_FOUND",
            message="import job not found",
        )

    try:
        return _ok(_to_job_dto(job, expected_case_id=case_id))
    except Exception:
        return _internal_error()


@router.post("/{job_id}/cancel")
def cancel_import_job(
    job_id: str,
    case_id: str = Query(..., min_length=4, max_length=80, pattern=r"^[A-Za-z0-9_-]+$"),
    import_service: ImportService = Depends(get_import_service),
):
    try:
        _ = import_service.cancel_job(job_id, case_id=case_id)
    except ImportJobNotFoundError:
        return _error(
            status_code=404,
            code="JOB_NOT_FOUND",
            message="import job not found",
        )
    except ImportJobNotCancellableError:
        return _error(
            status_code=409,
            code="JOB_NOT_CANCELLABLE",
            message="import job is already terminal",
        )

    return _ok({"ok": True})
