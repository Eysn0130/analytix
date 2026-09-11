from __future__ import annotations

from math import ceil
from typing import Dict, Optional

from fastapi import APIRouter, Depends, Query, Request
from fastapi.responses import JSONResponse

from app.api.data_analysis_deps import get_case_service, get_flow_service
from app.domain.case_service import CaseDeletedError, CaseNotFoundError, CaseService
from app.domain.flow_service import (
    FlowCaseBindingMismatchError,
    FlowJobNotCancellableError,
    FlowJobNotFoundError,
    FlowJobNotReadyError,
    FlowService,
    FlowViewNotFound,
    project_flow_public_result,
    project_flow_public_view,
)
from app.middleware.request_logging import get_request_id
from app.schemas.cases import PaginationMeta
from app.schemas.flow import (
    FlowAnalysisGraphDataReq,
    FlowBuildJobDTO,
    FlowBuildJobReq,
    FlowEdgeDTO,
    FlowGraphDataMergeReq,
    FlowGraphExpandReq,
    FlowGraphRenderPlanReq,
    FlowGraphReq,
    FlowGraphSearchReq,
    FlowLayoutNodePlanReq,
    FlowLayoutRoleGraphProjectionReq,
    FlowNetworkSectorPlacementReq,
    FlowProjectionLayoutSeedReq,
    FlowProjectionLayoutSyncReq,
    FlowPublicResultDTO,
    FlowPublicViewDTO,
    FlowNodeDTO,
    FlowRuntimeGraphDTO,
    FlowSameNameMergeReq,
    FlowViewReorderReq,
    FlowViewCreateReq,
    FlowViewUpdateReq,
    normalize_flow_runtime_graph,
)
from app.utils.strict_json import StrictJSONError, loads_strict_json
from app.utils.time import utc_now

router = APIRouter(prefix="/analysis/flow", tags=["flow"])
_FLOW_REQUEST_BODY_MAX_BYTES = 4 * 1024 * 1024
_FLOW_REQUEST_BODY_MAX_JSON_NODES = 250_000


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


def _controlled_flow_content_response() -> JSONResponse:
    return _error(
        status_code=403,
        code="FLOW_CONTENT_CONTROLLED_ARTIFACT_REQUIRED",
        message="flow graph content requires a controlled artifact",
    )


async def _strict_json_body(request: Request, model_type):
    media_type = str(request.headers.get("content-type") or "").split(";", 1)[0].strip().lower()
    if media_type != "application/json":
        return None, _error(
            status_code=415,
            code="UNSUPPORTED_MEDIA_TYPE",
            message="request body must use application/json",
        )
    try:
        chunks: list[bytes] = []
        total = 0
        async for chunk in request.stream():
            total += len(chunk)
            if total > _FLOW_REQUEST_BODY_MAX_BYTES:
                raise StrictJSONError("strict_json_too_large")
            chunks.append(chunk)
        payload = loads_strict_json(
            b"".join(chunks),
            max_bytes=_FLOW_REQUEST_BODY_MAX_BYTES,
            max_nodes=_FLOW_REQUEST_BODY_MAX_JSON_NODES,
        )
        return model_type.model_validate(payload), None
    except (StrictJSONError, UnicodeError, ValueError, RuntimeError):
        return None, _error(
            status_code=422,
            code="INVALID_ARGUMENT",
            message="request body does not match the closed schema",
        )


def _request_body_schema(model_type) -> dict:
    return {
        "requestBody": {
            "required": True,
            "content": {"application/json": {"schema": model_type.model_json_schema()}},
        }
    }


def _to_flow_public_result_dto(value: dict) -> dict:
    return FlowPublicResultDTO(**value).model_dump()


def _to_flow_public_view_dto(value: dict) -> dict:
    return FlowPublicViewDTO(**value).model_dump()


def _to_flow_job_dto(task, *, result_available: bool = False) -> dict:
    summary: dict = {}
    if isinstance(task.metadata, dict):
        source_summary = task.metadata.get("summary")
        if isinstance(source_summary, dict):
            build_ms = source_summary.get("build_ms")
            if isinstance(build_ms, int) and not isinstance(build_ms, bool) and build_ms >= 0:
                summary["build_ms"] = build_ms

    error = None
    if task.status.value == "failed":
        error = "flow_build_failed"
    elif task.status.value == "canceled":
        error = "flow_build_canceled"

    dto = FlowBuildJobDTO(
        job_id=task.task_id,
        case_id=task.case_id or "",
        status=task.status.value,
        progress=task.progress,
        summary=summary,
        error=error,
        result_available=task.status.value == "succeeded" and bool(result_available),
        created_at=task.created_at.isoformat(),
        updated_at=task.updated_at.isoformat(),
    )
    return dto.model_dump()


def _to_runtime_graph_dto(source: dict | None) -> dict:
    payload = normalize_flow_runtime_graph(source or {})
    return FlowRuntimeGraphDTO(**payload).model_dump(exclude_none=True)


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
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="flow operation failed",
            retryable=True,
        )

    if not isinstance(case, dict):
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="flow operation failed",
            retryable=True,
        )
    if case.get("is_deleted"):
        return _error(
            status_code=409,
            code="CASE_DELETED",
            message="case is deleted",
        )
    return None


@router.post("/graph", openapi_extra=_request_body_schema(FlowGraphReq))
async def build_flow_graph(
    request: Request,
    case_service: CaseService = Depends(get_case_service),
    flow_service: FlowService = Depends(get_flow_service),
):
    payload, body_error = await _strict_json_body(request, FlowGraphReq)
    if body_error is not None:
        return body_error
    assert payload is not None
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result

    try:
        payload_data = payload.model_dump()
        request_context = {
            key: value
            for key, value in payload_data.items()
            if key not in {"case_id", "seeds", "depth", "direction", "min_amount"}
        }
        graph = flow_service.build_graph(
            case_id=payload.case_id,
            seeds=payload.seeds,
            depth=payload.depth,
            direction=payload.direction,
            min_amount=payload.min_amount,
            request_context=request_context,
        )
        snapshot_ref = graph.get("result_snapshot_ref") if isinstance(graph, dict) else None
        if not isinstance(snapshot_ref, dict):
            snapshot_ref = flow_service.persist_result_snapshot(case_id=payload.case_id, result=graph)
        public_result = project_flow_public_result(
            case_id=payload.case_id,
            result={
                "stats": graph.get("stats") if isinstance(graph, dict) else {},
                "result_snapshot_ref": snapshot_ref,
            },
        )
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="flow operation failed",
            retryable=True,
        )
    return _ok(_to_flow_public_result_dto(public_result))


@router.post("/layout-role-graph")
async def project_flow_layout_role_graph(
    payload: FlowLayoutRoleGraphProjectionReq,
    case_service: CaseService = Depends(get_case_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result
    return _controlled_flow_content_response()


@router.post("/layout-node-plan")
async def compute_flow_layout_node_plan(
    payload: FlowLayoutNodePlanReq,
    case_service: CaseService = Depends(get_case_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result
    return _controlled_flow_content_response()


@router.post("/graph-render-plan")
async def project_flow_graph_render_plan(
    payload: FlowGraphRenderPlanReq,
    case_service: CaseService = Depends(get_case_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result
    return _controlled_flow_content_response()


@router.post("/layout-network-sector-placement")
async def project_flow_layout_network_sector_placement(
    payload: FlowNetworkSectorPlacementReq,
    case_service: CaseService = Depends(get_case_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result
    return _controlled_flow_content_response()


@router.post("/graph-data-merge")
async def project_flow_graph_data_merge(
    payload: FlowGraphDataMergeReq,
    case_service: CaseService = Depends(get_case_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result
    return _controlled_flow_content_response()


@router.post("/analysis-graph-data")
async def project_flow_analysis_graph_data(
    payload: FlowAnalysisGraphDataReq,
    case_service: CaseService = Depends(get_case_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result
    return _controlled_flow_content_response()


@router.post("/graph-search")
async def project_flow_graph_search(
    payload: FlowGraphSearchReq,
    case_service: CaseService = Depends(get_case_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result
    return _controlled_flow_content_response()


@router.post("/projection-layout-seed")
async def project_flow_projection_layout_seed(
    payload: FlowProjectionLayoutSeedReq,
    case_service: CaseService = Depends(get_case_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result
    return _controlled_flow_content_response()


@router.post("/same-name-merge")
async def merge_flow_same_name_graph(
    payload: FlowSameNameMergeReq,
    case_service: CaseService = Depends(get_case_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result
    return _controlled_flow_content_response()


@router.post("/jobs", openapi_extra=_request_body_schema(FlowBuildJobReq))
async def create_flow_build_job(
    request: Request,
    case_service: CaseService = Depends(get_case_service),
    flow_service: FlowService = Depends(get_flow_service),
):
    payload, body_error = await _strict_json_body(request, FlowBuildJobReq)
    if body_error is not None:
        return body_error
    assert payload is not None
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result

    if not payload.seeds:
        return _error(
            status_code=400,
            code="INVALID_ARGUMENT",
            message="seeds must not be empty",
        )

    try:
        payload_data = payload.model_dump()
        request_context = {
            key: value
            for key, value in payload_data.items()
            if key not in {"case_id", "seeds", "depth", "direction", "min_amount"}
        }
        job = flow_service.create_build_job(
            case_id=payload.case_id,
            seeds=payload.seeds,
            depth=payload.depth,
            direction=payload.direction,
            min_amount=payload.min_amount,
            request_context=request_context,
        )
    except ValueError:
        return _error(
            status_code=400,
            code="INVALID_ARGUMENT",
            message="flow operation failed",
        )
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="flow operation failed",
            retryable=True,
        )

    return _ok(_to_flow_job_dto(job), status_code=202)


@router.get("/jobs/{job_id}")
async def get_flow_build_job(
    job_id: str,
    case_id: str = Query(..., min_length=1),
    case_service: CaseService = Depends(get_case_service),
    flow_service: FlowService = Depends(get_flow_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result
    try:
        job = flow_service.get_job(job_id, case_id=case_id)
    except FlowJobNotFoundError:
        return _error(
            status_code=404,
            code="JOB_NOT_FOUND",
            message="flow build job not found",
        )
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="flow operation failed",
            retryable=True,
        )

    try:
        result_available = flow_service.job_result_available(job_id, case_id=case_id)
    except Exception:
        result_available = False
    return _ok(_to_flow_job_dto(job, result_available=result_available))


@router.get("/jobs/{job_id}/result")
async def get_flow_build_result(
    job_id: str,
    case_id: str = Query(..., min_length=1),
    case_service: CaseService = Depends(get_case_service),
    flow_service: FlowService = Depends(get_flow_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result
    try:
        result = flow_service.get_job_result(job_id, case_id=case_id)
    except FlowJobNotFoundError:
        return _error(
            status_code=404,
            code="JOB_NOT_FOUND",
            message="flow build job not found",
        )
    except FlowJobNotReadyError:
        return _error(
            status_code=409,
            code="JOB_NOT_READY",
            message="flow build job result is not ready",
        )
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="flow operation failed",
            retryable=True,
        )

    return _ok(_to_flow_public_result_dto(result))


@router.post("/jobs/{job_id}/cancel")
async def cancel_flow_build_job(
    job_id: str,
    case_id: str = Query(..., min_length=1),
    case_service: CaseService = Depends(get_case_service),
    flow_service: FlowService = Depends(get_flow_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result
    try:
        _ = flow_service.cancel_job(job_id, case_id=case_id)
    except FlowJobNotFoundError:
        return _error(
            status_code=404,
            code="JOB_NOT_FOUND",
            message="flow build job not found",
        )
    except FlowJobNotCancellableError:
        return _error(
            status_code=409,
            code="JOB_NOT_CANCELLABLE",
            message="flow build job is already terminal",
        )
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="flow operation failed",
            retryable=True,
        )

    return _ok({"ok": True})


@router.post("/views", openapi_extra=_request_body_schema(FlowViewCreateReq))
async def create_flow_view(
    request: Request,
    case_service: CaseService = Depends(get_case_service),
    flow_service: FlowService = Depends(get_flow_service),
):
    payload, body_error = await _strict_json_body(request, FlowViewCreateReq)
    if body_error is not None:
        return body_error
    assert payload is not None
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result

    try:
        item = flow_service.create_view(case_id=payload.case_id)
        projected_item = _to_flow_public_view_dto(
            project_flow_public_view(case_id=payload.case_id, view=item)
        )
    except FlowCaseBindingMismatchError:
        return _error(
            status_code=409,
            code="FLOW_CASE_BINDING_MISMATCH",
            message="flow case binding mismatch",
        )
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="flow operation failed",
            retryable=True,
        )

    return _ok(projected_item, status_code=201)


@router.get("/views")
async def list_flow_views(
    case_id: str = Query(..., min_length=1),
    page: int = Query(default=1, ge=1),
    page_size: int = Query(default=50, ge=1, le=500),
    case_service: CaseService = Depends(get_case_service),
    flow_service: FlowService = Depends(get_flow_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result

    try:
        items, total = flow_service.list_views(case_id=case_id, page=page, page_size=page_size)
        projected_items = [
            _to_flow_public_view_dto(project_flow_public_view(case_id=case_id, view=item))
            for item in items
        ]
    except FlowCaseBindingMismatchError:
        return _error(
            status_code=409,
            code="FLOW_CASE_BINDING_MISMATCH",
            message="flow case binding mismatch",
        )
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="flow operation failed",
            retryable=True,
        )

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


@router.get("/result-snapshots/{snapshot_id}")
async def get_flow_result_snapshot(
    snapshot_id: str,
    case_id: str = Query(..., min_length=1),
    case_service: CaseService = Depends(get_case_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result
    return _controlled_flow_content_response()


@router.get("/result-snapshots/{snapshot_id}/patch")
async def get_flow_result_snapshot_patch(
    snapshot_id: str,
    case_id: str,
    base_snapshot_id: str,
    case_service: CaseService = Depends(get_case_service),
):
    normalized_case_id = str(case_id or "").strip()
    normalized_base_snapshot_id = str(base_snapshot_id or "").strip()
    if not normalized_case_id:
        return _error(status_code=400, code="INVALID_ARGUMENT", message="case_id must not be empty")
    if not normalized_base_snapshot_id:
        return _error(status_code=400, code="INVALID_ARGUMENT", message="base_snapshot_id must not be empty")
    case_result = _ensure_case_ready(case_service, normalized_case_id)
    if case_result is not None:
        return case_result
    return _controlled_flow_content_response()


@router.post("/result-snapshots/{snapshot_id}/expand")
async def expand_flow_result_snapshot(
    snapshot_id: str,
    payload: FlowGraphExpandReq,
    case_service: CaseService = Depends(get_case_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result
    return _controlled_flow_content_response()


@router.post("/result-snapshots/{snapshot_id}/projection-layout")
async def sync_flow_result_snapshot_projection_layout(
    snapshot_id: str,
    payload: FlowProjectionLayoutSyncReq,
    case_service: CaseService = Depends(get_case_service),
):
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result
    return _controlled_flow_content_response()


@router.get("/result-snapshot-metrics")
async def get_flow_result_snapshot_metrics(
    case_id: str = Query(..., min_length=1),
    case_service: CaseService = Depends(get_case_service),
    flow_service: FlowService = Depends(get_flow_service),
):
    normalized_case_id = str(case_id or "").strip()
    case_result = _ensure_case_ready(case_service, normalized_case_id)
    if case_result is not None:
        return case_result
    try:
        result = flow_service.get_result_snapshot_metrics(case_id=normalized_case_id)
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="flow operation failed",
            retryable=True,
        )
    return _ok(result)


@router.post("/result-snapshot-gc")
async def run_flow_result_snapshot_gc(
    case_id: str = Query(..., min_length=1),
    case_service: CaseService = Depends(get_case_service),
    flow_service: FlowService = Depends(get_flow_service),
):
    normalized_case_id = str(case_id or "").strip()
    case_result = _ensure_case_ready(case_service, normalized_case_id)
    if case_result is not None:
        return case_result
    try:
        result = flow_service.run_result_snapshot_gc(case_id=normalized_case_id)
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="flow operation failed",
            retryable=True,
        )
    return _ok(result)


@router.get("/views/{view_id}")
async def get_flow_view(
    view_id: str,
    case_id: str = Query(..., min_length=1),
    case_service: CaseService = Depends(get_case_service),
    flow_service: FlowService = Depends(get_flow_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result
    try:
        item = flow_service.get_view(view_id, case_id=case_id)
        projected_item = _to_flow_public_view_dto(
            project_flow_public_view(case_id=case_id, view=item)
        )
    except FlowViewNotFound:
        return _error(
            status_code=404,
            code="NOT_FOUND",
            message="flow view not found",
        )
    except FlowCaseBindingMismatchError:
        return _error(
            status_code=409,
            code="FLOW_CASE_BINDING_MISMATCH",
            message="flow case binding mismatch",
        )
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="flow operation failed",
            retryable=True,
        )

    return _ok(projected_item)


@router.patch("/views/{view_id}", openapi_extra=_request_body_schema(FlowViewUpdateReq))
async def update_flow_view(
    view_id: str,
    request: Request,
    case_id: str = Query(..., min_length=1),
    case_service: CaseService = Depends(get_case_service),
    flow_service: FlowService = Depends(get_flow_service),
):
    _, body_error = await _strict_json_body(request, FlowViewUpdateReq)
    if body_error is not None:
        return body_error
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result
    try:
        item = flow_service.update_view(case_id=case_id, view_id=view_id)
        projected_item = _to_flow_public_view_dto(
            project_flow_public_view(case_id=case_id, view=item)
        )
    except FlowCaseBindingMismatchError:
        return _error(
            status_code=409,
            code="FLOW_CASE_BINDING_MISMATCH",
            message="flow case binding mismatch",
        )
    except FlowViewNotFound:
        return _error(
            status_code=404,
            code="NOT_FOUND",
            message="flow view not found",
        )
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="flow operation failed",
            retryable=True,
        )

    return _ok(projected_item)


@router.post("/views/reorder", openapi_extra=_request_body_schema(FlowViewReorderReq))
async def reorder_flow_views(
    request: Request,
    case_service: CaseService = Depends(get_case_service),
    flow_service: FlowService = Depends(get_flow_service),
):
    payload, body_error = await _strict_json_body(request, FlowViewReorderReq)
    if body_error is not None:
        return body_error
    assert payload is not None
    case_result = _ensure_case_ready(case_service, payload.case_id)
    if case_result is not None:
        return case_result

    try:
        result = flow_service.reorder_views(case_id=payload.case_id, order=payload.order)
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="flow operation failed",
            retryable=True,
        )

    return _ok(result if isinstance(result, dict) else {"ok": True})


@router.delete("/views/{view_id}")
async def delete_flow_view(
    view_id: str,
    case_id: str = Query(..., min_length=1),
    case_service: CaseService = Depends(get_case_service),
    flow_service: FlowService = Depends(get_flow_service),
):
    case_result = _ensure_case_ready(case_service, case_id)
    if case_result is not None:
        return case_result
    try:
        flow_service.delete_view(view_id, case_id=case_id)
    except FlowViewNotFound:
        return _error(
            status_code=404,
            code="NOT_FOUND",
            message="flow view not found",
        )
    except Exception:
        return _error(
            status_code=500,
            code="INTERNAL_ERROR",
            message="flow operation failed",
            retryable=True,
        )

    return _ok({"ok": True})
