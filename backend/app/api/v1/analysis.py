from __future__ import annotations

from typing import Dict, Optional

from fastapi import APIRouter, Depends, Query
from fastapi.responses import JSONResponse

from app.api.data_analysis_deps import (
    get_analysis_service,
    get_analysis_worker_service,
    get_case_service,
    get_task_service,
)
from app.domain.analysis_service import (
    AnalysisCaseNotFoundError,
    AnalysisService,
    AnalysisTraceNotFoundError,
)
from app.domain.analysis_public_projection import (
    AnalysisPublicProjectionError,
    project_analysis_trace_run_public,
)
from app.domain.analysis_maintenance_public_projection import (
    project_analysis_maintenance_public,
    project_analysis_scope_cache_public,
)
from app.domain.analysis_worker_service import AnalysisWorkerService
from app.domain.case_service import CaseDeletedError, CaseNotFoundError, CaseService
from app.middleware.request_logging import get_request_id
from app.schemas.analysis import (
    AnalysisJanitorDTO,
    AnalysisJanitorReq,
    AnalysisMaintenancePublicDTO,
    AnalysisRefreshReq,
    AnalysisRuleParamsDTO,
    AnalysisScopeCachePublicDTO,
    AnalysisTracePathPublicDTO,
    AnalysisTracePublicDTO,
    AnalysisTraceRunReq,
    AnalysisUpdateRuleParamsDTO,
    AnalysisUpdateRuleParamsReq,
)
from app.tasks.service import TaskService
from app.utils.time import utc_now

router = APIRouter(prefix="/analysis", tags=["analysis"])


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
        case = case_service.get_case(case_id)
    except CaseNotFoundError:
        return _error(status_code=404, code="CASE_NOT_FOUND", message="case not found")
    except CaseDeletedError:
        return _error(status_code=409, code="CASE_DELETED", message="case is deleted")

    if case.get("is_deleted"):
        return _error(status_code=409, code="CASE_DELETED", message="case is deleted")
    return None


@router.get("/config/rule-params")
async def get_rule_params(
    case_id: str = Query(min_length=1, max_length=128),
    case_service: CaseService = Depends(get_case_service),
    analysis_service: AnalysisService = Depends(get_analysis_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result
    try:
        return _ok(AnalysisRuleParamsDTO(**analysis_service.get_rule_params(case_id)).model_dump())
    except AnalysisCaseNotFoundError:
        return _error(
            status_code=404,
            code="CASE_NOT_FOUND",
            message="case not found",
            details={"case_id": case_id},
        )
    except ValueError as exc:
        return _error(
            status_code=409,
            code="RULE_PARAMS_INVALID",
            message="persisted rule parameters are invalid",
            details={"reason": str(exc)},
        )


@router.post("/config/rule-params")
async def update_rule_params(
    payload: AnalysisUpdateRuleParamsReq,
    case_service: CaseService = Depends(get_case_service),
    analysis_service: AnalysisService = Depends(get_analysis_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result
    try:
        data = analysis_service.update_rule_params(
            payload.case_id,
            scope_type=payload.scope_type,
            params=payload.params,
        )
        return _ok(AnalysisUpdateRuleParamsDTO(**data).model_dump())
    except AnalysisCaseNotFoundError:
        return _error(
            status_code=404,
            code="CASE_NOT_FOUND",
            message="case not found",
            details={"case_id": payload.case_id},
        )
    except ValueError as exc:
        return _error(
            status_code=422,
            code="RULE_PARAMS_INVALID",
            message="rule parameters are invalid",
            details={"reason": str(exc)},
        )


@router.post("/refresh")
async def refresh_case_analysis(
    payload: AnalysisRefreshReq,
    case_service: CaseService = Depends(get_case_service),
    analysis_service: AnalysisService = Depends(get_analysis_service),
    task_service: TaskService = Depends(get_task_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result
    try:
        data = analysis_service.refresh_case_analysis(
            payload.case_id,
            force_refresh=payload.force_refresh,
            mode=payload.mode,
        )
        candidate_job_id = data.get("job_id") if type(data) is dict else ""
        job_id = candidate_job_id if type(candidate_job_id) is str else ""
        projected = project_analysis_maintenance_public(
            case_id=payload.case_id,
            operation="analysis_refresh",
            job_id=job_id,
            task_service=task_service,
            allow_queued_job=payload.mode == "async",
        )
        return _ok(AnalysisMaintenancePublicDTO(**projected).model_dump())
    except AnalysisCaseNotFoundError:
        return _error(status_code=404, code="CASE_NOT_FOUND", message="case not found", details={"case_id": payload.case_id})


@router.post("/feature-mart/materialize")
async def materialize_feature_mart():
    projected = project_analysis_maintenance_public(
        case_id="",
        operation="feature_mart_materialize",
    )
    return _ok(AnalysisMaintenancePublicDTO(**projected).model_dump())


@router.get("/feature-mart/scopes")
async def list_feature_mart_scopes(
    case_id: str = Query(min_length=1, max_length=128),
    scope_kind: str = Query(default="", max_length=80),
    limit: int = Query(default=50, ge=1, le=200),
    case_service: CaseService = Depends(get_case_service),
    analysis_service: AnalysisService = Depends(get_analysis_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result
    del scope_kind, limit, analysis_service
    projected = project_analysis_scope_cache_public(case_id=case_id)
    return _ok(AnalysisScopeCachePublicDTO(**projected).model_dump())


@router.post("/maintenance/temp-scope-janitor")
async def run_temp_scope_janitor(
    payload: AnalysisJanitorReq,
    analysis_service: AnalysisService = Depends(get_analysis_service),
    analysis_worker_service: AnalysisWorkerService = Depends(get_analysis_worker_service),
):
    if payload.mode == "sync":
        return _ok(
            AnalysisJanitorDTO(
                result_status="completed",
                **analysis_service.run_temp_scope_janitor(),
            ).model_dump()
        )
    task = analysis_worker_service.create_temp_scope_janitor_job(
        case_id=payload.case_id,
        dispatch_mode="queue" if payload.mode == "queue" else "thread",
    )
    return _ok(
        AnalysisJanitorDTO(
            result_status="pending",
            job_kind="temp_scope_janitor",
            job_id=task.task_id,
        ).model_dump(),
        status_code=202,
    )


@router.post("/maintenance/runtime-retention-janitor")
async def run_runtime_retention_janitor(
    payload: AnalysisJanitorReq,
    analysis_service: AnalysisService = Depends(get_analysis_service),
    analysis_worker_service: AnalysisWorkerService = Depends(get_analysis_worker_service),
):
    if payload.mode == "sync":
        try:
            return _ok(
                AnalysisJanitorDTO(
                    result_status="completed",
                    **analysis_service.run_runtime_retention_janitor(case_id=payload.case_id),
                ).model_dump()
            )
        except AnalysisCaseNotFoundError:
            return _error(
                status_code=404,
                code="CASE_NOT_FOUND",
                message="case not found",
                details={"case_id": payload.case_id},
            )
    task = analysis_worker_service.create_runtime_retention_janitor_job(
        case_id=payload.case_id,
        dispatch_mode="queue" if payload.mode == "queue" else "thread",
    )
    return _ok(
        AnalysisJanitorDTO(
            result_status="pending",
            job_kind="runtime_retention_janitor",
            job_id=task.task_id,
        ).model_dump(),
        status_code=202,
    )


@router.post("/trace/run")
async def run_trace(
    payload: AnalysisTraceRunReq,
    case_service: CaseService = Depends(get_case_service),
    analysis_service: AnalysisService = Depends(get_analysis_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result
    try:
        data = analysis_service.run_trace(
            payload.case_id,
            seed_type=payload.seed_type,
            seed_value=payload.seed_value,
            depth=payload.depth,
            time_window_sec=payload.time_window_sec,
            tolerance_rate=payload.tolerance_rate,
            mode=payload.mode,
        )
        return _ok(
            AnalysisTracePublicDTO(
                **project_analysis_trace_run_public(case_id=payload.case_id, result=data)
            ).model_dump()
        )
    except AnalysisCaseNotFoundError:
        return _error(status_code=404, code="CASE_NOT_FOUND", message="case not found")
    except AnalysisPublicProjectionError:
        return _error(
            status_code=409,
            code="ANALYSIS_TRACE_PUBLIC_PROJECTION_REJECTED",
            message="analysis trace public projection rejected",
        )


@router.get("/trace/{trace_id}")
async def get_trace(
    trace_id: str,
    case_id: str = Query(min_length=1, max_length=128),
    case_service: CaseService = Depends(get_case_service),
    analysis_service: AnalysisService = Depends(get_analysis_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result
    try:
        data = analysis_service.get_trace(trace_id, case_id=case_id)
        dto = AnalysisTracePublicDTO(**data)
        return _ok(dto.model_dump())
    except AnalysisTraceNotFoundError:
        return _error(status_code=404, code="TRACE_NOT_FOUND", message="trace not found")
    except AnalysisPublicProjectionError:
        return _error(
            status_code=409,
            code="ANALYSIS_TRACE_PUBLIC_PROJECTION_REJECTED",
            message="analysis trace public projection rejected",
        )


@router.get("/trace/{trace_id}/paths/{path_id}")
async def get_trace_path(
    trace_id: str,
    path_id: str,
    case_id: str = Query(min_length=1, max_length=128),
    case_service: CaseService = Depends(get_case_service),
    analysis_service: AnalysisService = Depends(get_analysis_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result
    try:
        data = analysis_service.get_trace_path(trace_id, path_id, case_id=case_id)
        dto = AnalysisTracePathPublicDTO(**data)
        return _ok(dto.model_dump())
    except AnalysisTraceNotFoundError:
        return _error(status_code=404, code="TRACE_PATH_NOT_FOUND", message="trace path not found")
    except AnalysisPublicProjectionError:
        return _error(
            status_code=409,
            code="ANALYSIS_TRACE_PUBLIC_PROJECTION_REJECTED",
            message="analysis trace public projection rejected",
        )
