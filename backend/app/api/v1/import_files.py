from __future__ import annotations

from typing import Dict, Optional

from fastapi import APIRouter, Body, Depends, Query, Request
from fastapi.responses import JSONResponse

from app.api.data_analysis_deps import get_case_service, get_import_service
from app.core.import_count_semantics import project_import_file_log_counts
from app.domain.case_service import CaseDeletedError, CaseNotFoundError, CaseService
from app.domain.case_source_state import CaseSourceUnavailableError
from app.domain.controlled_artifact_gate import CONTROLLED_SOURCE_INGESTION_REQUIRED
from app.domain.import_service import (
    ImportFileBusyError,
    ImportFileNotFoundError,
    ImportService,
)
from app.domain.import_public_projection import (
    project_import_file_log_public,
    project_import_historical_dataset_public,
    resolve_public_import_file_refs,
)
from app.middleware.request_logging import get_request_id
from app.schemas.import_files import (
    ImportBatchActionResultDTO,
    ImportBatchDeleteReq,
    ImportBatchDeleteResultDTO,
    ImportFileLogDTO,
    ImportFileLogView,
    ImportHistoricalDatasetDTO,
)
from app.utils.time import utc_now

router = APIRouter(prefix="/import/files", tags=["import"])


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


def _source_unavailable() -> JSONResponse:
    return _error(
        status_code=503,
        code="SOURCE_UNAVAILABLE",
        message="case-bound source is unavailable",
        retryable=True,
        details={"source_status": "unavailable"},
    )


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
        return _source_unavailable()

    if is_deleted:
        return _error(
            status_code=409,
            code="CASE_DELETED",
            message="case is deleted",
        )
    return None


def _resolve_file_refs(
    *,
    import_service: ImportService,
    case_id: str,
    requested_refs: list[str],
    view: ImportFileLogView,
) -> list[str]:
    source_rows = import_service.list_file_logs(case_id=case_id, view=view)
    return resolve_public_import_file_refs(
        case_id=case_id,
        requested_refs=requested_refs,
        source_rows=source_rows,
    )


@router.get("")
def list_import_files(
    case_id: str = Query(..., min_length=1),
    view: ImportFileLogView = Query("active"),
    case_service: CaseService = Depends(get_case_service),
    import_service: ImportService = Depends(get_import_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result

    try:
        rows = import_service.list_file_logs(case_id=case_id, view=view)
        items = []
        for item in rows:
            counted = project_import_file_log_counts(item if isinstance(item, dict) else {})
            projected = project_import_file_log_public(case_id=case_id, row=counted)
            items.append(ImportFileLogDTO(**projected).model_dump())
    except CaseSourceUnavailableError:
        return _source_unavailable()
    except Exception:
        return _internal_error()

    return _ok({"items": items})


def _handle_batch_action_error(*, payload: ImportBatchDeleteReq, exc: Exception) -> JSONResponse:
    if isinstance(exc, ValueError):
        return _error(
            status_code=400,
            code="INVALID_ARGUMENT",
            message="invalid import request",
        )
    if isinstance(exc, ImportFileNotFoundError):
        missing_file_ids = getattr(exc, "file_ids", []) or payload.file_ids
        return _error(
            status_code=404,
            code="IMPORT_FILE_NOT_FOUND",
            message="import file not found",
            details={"missing_count": len(missing_file_ids)},
        )
    if isinstance(exc, ImportFileBusyError):
        busy_file_ids = getattr(exc, "file_ids", []) or payload.file_ids
        return _error(
            status_code=409,
            code="IMPORT_FILE_BUSY",
            message="one or more import files are in running job",
            details={"busy_count": len(busy_file_ids)},
        )
    return _internal_error()


@router.get("/historical-datasets")
def list_import_historical_datasets(
    case_id: str = Query(..., min_length=1),
    case_service: CaseService = Depends(get_case_service),
    import_service: ImportService = Depends(get_import_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result

    try:
        rows = import_service.list_historical_datasets(case_id=case_id)
        items = [
            ImportHistoricalDatasetDTO(
                **project_import_historical_dataset_public(
                    case_id=case_id,
                    row=item if isinstance(item, dict) else {},
                )
            ).model_dump()
            for item in rows
        ]
    except CaseSourceUnavailableError:
        return _source_unavailable()
    except Exception:
        return _internal_error()

    return _ok({"items": items})


@router.post("/preview")
def preview_import_files(
    _request: Request,
):
    return _error(
        status_code=409,
        code="CONTROLLED_SOURCE_INGESTION_REQUIRED",
        message=CONTROLLED_SOURCE_INGESTION_REQUIRED,
        details={"phase": "p0_quarantine", "replacement": "p2_host_staged_ingestion"},
    )

@router.get("/runtime-paths")
def get_import_runtime_paths(
    _request: Request,
):
    return _error(
        status_code=409,
        code="CONTROLLED_SOURCE_INGESTION_REQUIRED",
        message=CONTROLLED_SOURCE_INGESTION_REQUIRED,
        details={"phase": "p0_quarantine", "replacement": "p2_host_staged_ingestion"},
    )


@router.post("/delete-batch")
def delete_import_files_batch(
    payload: ImportBatchDeleteReq = Body(...),
    case_service: CaseService = Depends(get_case_service),
    import_service: ImportService = Depends(get_import_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result

    try:
        source_ids = _resolve_file_refs(
            import_service=import_service,
            case_id=payload.case_id,
            requested_refs=payload.file_ids,
            view="active",
        )
        deleted = import_service.delete_files(case_id=payload.case_id, file_ids=source_ids)
    except Exception as exc:
        return _handle_batch_action_error(payload=payload, exc=exc)

    return _ok(
        ImportBatchDeleteResultDTO(
            ok=True,
            deleted_count=len(deleted),
            file_ids=list(payload.file_ids),
        ).model_dump()
    )


@router.post("/recycle-batch")
def recycle_import_files_batch(
    payload: ImportBatchDeleteReq = Body(...),
    case_service: CaseService = Depends(get_case_service),
    import_service: ImportService = Depends(get_import_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result

    try:
        source_ids = _resolve_file_refs(
            import_service=import_service,
            case_id=payload.case_id,
            requested_refs=payload.file_ids,
            view="active",
        )
        affected = import_service.recycle_files(case_id=payload.case_id, file_ids=source_ids)
    except Exception as exc:
        return _handle_batch_action_error(payload=payload, exc=exc)

    return _ok(
        ImportBatchActionResultDTO(
            ok=True,
            action="recycle",
            affected_count=len(affected),
            file_ids=list(payload.file_ids),
        ).model_dump()
    )


@router.post("/restore-batch")
def restore_import_files_batch(
    payload: ImportBatchDeleteReq = Body(...),
    case_service: CaseService = Depends(get_case_service),
    import_service: ImportService = Depends(get_import_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result

    try:
        source_ids = _resolve_file_refs(
            import_service=import_service,
            case_id=payload.case_id,
            requested_refs=payload.file_ids,
            view="recycle",
        )
        affected = import_service.restore_files(case_id=payload.case_id, file_ids=source_ids)
    except Exception as exc:
        return _handle_batch_action_error(payload=payload, exc=exc)

    return _ok(
        ImportBatchActionResultDTO(
            ok=True,
            action="restore",
            affected_count=len(affected),
            file_ids=list(payload.file_ids),
        ).model_dump()
    )


@router.post("/purge-batch")
def purge_import_files_batch(
    payload: ImportBatchDeleteReq = Body(...),
    case_service: CaseService = Depends(get_case_service),
    import_service: ImportService = Depends(get_import_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result

    try:
        source_ids = _resolve_file_refs(
            import_service=import_service,
            case_id=payload.case_id,
            requested_refs=payload.file_ids,
            view="recycle",
        )
        affected = import_service.purge_files(case_id=payload.case_id, file_ids=source_ids)
    except Exception as exc:
        return _handle_batch_action_error(payload=payload, exc=exc)

    return _ok(
        ImportBatchActionResultDTO(
            ok=True,
            action="purge",
            affected_count=len(affected),
            file_ids=list(payload.file_ids),
        ).model_dump()
    )


@router.delete("/{file_id}")
def delete_import_file(
    file_id: str,
    case_id: str = Query(..., min_length=1),
    case_service: CaseService = Depends(get_case_service),
    import_service: ImportService = Depends(get_import_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result

    try:
        source_id = _resolve_file_refs(
            import_service=import_service,
            case_id=case_id,
            requested_refs=[file_id],
            view="active",
        )[0]
        import_service.delete_file(case_id=case_id, file_id=source_id)
    except ImportFileNotFoundError as exc:
        missing_file_ids = getattr(exc, "file_ids", [file_id])
        return _error(
            status_code=404,
            code="IMPORT_FILE_NOT_FOUND",
            message="import file not found",
            details={"missing_count": len(missing_file_ids)},
        )
    except ImportFileBusyError as exc:
        busy_file_ids = getattr(exc, "file_ids", [file_id])
        return _error(
            status_code=409,
            code="IMPORT_FILE_BUSY",
            message="import file is in running job",
            details={"busy_count": len(busy_file_ids)},
        )
    except Exception:
        return _internal_error()

    return _ok({"ok": True})
