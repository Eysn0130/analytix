from __future__ import annotations

from typing import Dict, Optional

from fastapi import APIRouter, Depends, Query
from fastapi.responses import JSONResponse

from app.api.data_analysis_deps import get_case_service, get_document_service
from app.domain.case_service import CaseDeletedError, CaseNotFoundError, CaseService
from app.domain.controlled_artifact_gate import CONTROLLED_SOURCE_INGESTION_REQUIRED
from app.domain.document_service import (
    DocumentNotFoundError,
    DocumentPublicProjectionError,
    DocumentPublicProjectionUnavailableError,
    DocumentService,
)
from app.middleware.request_logging import get_request_id
from app.schemas.documents import (
    DocumentAssetDTO,
    DocumentDetailDTO,
)
from app.utils.time import utc_now

router = APIRouter(prefix="/documents", tags=["documents"])


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


def _ensure_case_ready(case_service: CaseService, case_id: str) -> Optional[JSONResponse]:
    try:
        is_deleted = case_service.is_case_deleted(case_id)
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
    except Exception:
        return _error(
            status_code=503,
            code="SOURCE_UNAVAILABLE",
            message="case state is unavailable",
            retryable=True,
        )

    if is_deleted:
        return _error(
            status_code=409,
            code="CASE_DELETED",
            message="case is deleted",
        )
    return None


@router.get("")
def list_documents(
    case_id: str = Query(..., min_length=4, max_length=80, pattern=r"^[A-Za-z0-9_-]+$"),
    selected_for_llm: Optional[bool] = Query(default=None),
    llm_ready: Optional[bool] = Query(default=None),
    include_system: bool = Query(default=False),
    case_service: CaseService = Depends(get_case_service),
    document_service: DocumentService = Depends(get_document_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result

    try:
        rows = document_service.list_documents(
            case_id=case_id,
            selected_for_llm=selected_for_llm,
            llm_ready=llm_ready,
            include_system=include_system,
        )
    except DocumentPublicProjectionUnavailableError:
        return _error(
            status_code=409,
            code="DOCUMENT_PUBLIC_PROJECTION_UNAVAILABLE",
            message="document metadata is unavailable",
        )
    except DocumentPublicProjectionError:
        return _error(
            status_code=409,
            code="DOCUMENT_PUBLIC_PROJECTION_REJECTED",
            message="document metadata is unavailable",
        )
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="document list unavailable",
            retryable=True,
        )

    items = [DocumentAssetDTO(**(item if isinstance(item, dict) else {})).model_dump() for item in rows]
    return _ok({"items": items})


@router.get("/{document_ref}")
def get_document_detail(
    document_ref: str,
    case_id: str = Query(..., min_length=4, max_length=80, pattern=r"^[A-Za-z0-9_-]+$"),
    include_chunks: bool = Query(default=False),
    case_service: CaseService = Depends(get_case_service),
    document_service: DocumentService = Depends(get_document_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result

    if include_chunks:
        return _error(
            status_code=403,
            code="DOCUMENT_CONTENT_CONTROLLED_ARTIFACT_REQUIRED",
            message="document content requires a host-issued controlled artifact authorization",
        )

    try:
        payload = document_service.get_document(
            case_id=case_id,
            document_ref=document_ref,
        )
    except DocumentPublicProjectionUnavailableError:
        return _error(
            status_code=409,
            code="DOCUMENT_PUBLIC_PROJECTION_UNAVAILABLE",
            message="document metadata is unavailable",
        )
    except (DocumentNotFoundError, DocumentPublicProjectionError):
        return _error(
            status_code=404,
            code="DOCUMENT_NOT_FOUND",
            message="document not found",
        )
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="document detail unavailable",
            retryable=True,
        )

    dto = DocumentDetailDTO(**payload).model_dump()
    return _ok(dto)


@router.post("/selection")
def update_document_selection():
    return _error(
        status_code=409,
        code="CONTROLLED_SOURCE_INGESTION_REQUIRED",
        message=CONTROLLED_SOURCE_INGESTION_REQUIRED,
        details={"phase": "p0_quarantine", "replacement": "p2_host_source_handle"},
    )
