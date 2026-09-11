from datetime import datetime, timezone
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException, Query, Request

from app.api.data_analysis_deps import (
    get_settings,
    get_task_service,
)
from app.core.execution_authority_health import (
    execution_authority_health,
    unadmitted_execution_capability_health,
)
from app.infra.config import AppSettings
from app.middleware.request_logging import get_request_id
from app.schemas.system import HealthChecks, HealthPayload, HealthResponse
from app.tasks.failure_boundary import CASE_ID_PATTERN, InvalidTaskCaseIDError
from app.tasks.models import TaskStatus, TaskType
from app.tasks.service import TaskNotFoundError, TaskService
from app.tasks.state_machine import TaskStateMachine
from app.utils.time import utc_now

router = APIRouter(prefix="/system", tags=["system"])


def _request_meta() -> dict:
    return {
        "request_id": get_request_id(),
        "timestamp": utc_now().isoformat(),
    }


def _build_health_payload(request: Request, settings: AppSettings) -> HealthPayload:
    started_at = getattr(request.app.state, "started_at", utc_now())
    uptime_s = max((utc_now() - started_at).total_seconds(), 0.0)
    llm_service = getattr(request.app.state, "llm_service", None)
    query_engine = getattr(request.app.state, "query_engine", None)
    runtime_architecture = getattr(llm_service, "runtime_architecture", None)
    execution_authority = execution_authority_health()
    helper_execution = unadmitted_execution_capability_health(execution_authority)
    codex_runtime = (
        query_engine.get_codex_runtime_health()
        if helper_execution.available
        and query_engine is not None
        and hasattr(query_engine, "get_codex_runtime_health")
        else {
            "python_version": "",
            "min_python_version": "3.10",
            "codex_runtime_ready": False,
            "codex_runtime_reason": helper_execution.reason_code,
        }
    )
    checks = HealthChecks(
        config_loaded=getattr(request.app.state, "settings", None) is not None,
        task_service_ready=getattr(request.app.state, "task_service", None) is not None,
        ws_sequence_ready=getattr(request.app.state, "ws_sequence", None) is not None,
    )

    status_value = "ok" if all(checks.model_dump().values()) else "degraded"
    return HealthPayload(
        status=status_value,
        service=settings.app_name,
        version=settings.app_version,
        env=settings.app_env,
        phase="P08",
        runtime_architecture=str(getattr(runtime_architecture, "mode", "") or "").strip() or "codex-aligned",
        codex_aligned=bool(getattr(runtime_architecture, "is_codex_aligned", True)),
        python_version=str(codex_runtime.get("python_version") or "").strip(),
        codex_runtime_min_python=str(codex_runtime.get("min_python_version") or "3.10").strip(),
        codex_runtime_ready=bool(codex_runtime.get("codex_runtime_ready", False)),
        codex_runtime_reason=str(codex_runtime.get("codex_runtime_reason") or "").strip(),
        managed_execution_available=execution_authority.available,
        managed_execution_reason=execution_authority.reason_code,
        document_conversion_available=False,
        document_conversion_reason=helper_execution.reason_code,
        archive_extraction_available=False,
        archive_extraction_reason=helper_execution.reason_code,
        archive_extraction_bin="",
        archive_extraction_bin_source="",
        archive_extraction_supported_exts=[],
        uptime_s=round(uptime_s, 3),
        checks=checks,
    )


@router.get("/meta")
async def system_meta(settings: AppSettings = Depends(get_settings)) -> dict:
    return {
        **_request_meta(),
        "data": {
            "service": settings.app_name,
            "version": settings.app_version,
            "env": settings.app_env,
            "api_prefix": settings.api_prefix,
            "ws_contract_version": settings.ws_contract_version,
            "time": datetime.now(timezone.utc).isoformat(),
        },
    }


@router.get("/health", response_model=HealthResponse)
async def system_health(
    request: Request,
    settings: AppSettings = Depends(get_settings),
) -> dict:
    return {
        **_request_meta(),
        "data": _build_health_payload(request, settings),
    }


@router.get("/task-state-map")
async def task_state_map() -> dict:
    return {
        **_request_meta(),
        "data": {
            "transitions": TaskStateMachine.transition_map(),
        },
    }


@router.get("/tasks")
async def list_tasks(
    page: int = Query(default=1, ge=1),
    page_size: int = Query(default=50, ge=1, le=500),
    status_filter: Optional[TaskStatus] = Query(default=None, alias="status"),
    task_type: Optional[TaskType] = Query(default=None),
    case_id: str = Query(
        ...,
        min_length=4,
        max_length=80,
        pattern=CASE_ID_PATTERN.pattern,
    ),
    task_service: TaskService = Depends(get_task_service),
) -> dict:
    try:
        items, total = task_service.list_public_tasks(
            page=page,
            page_size=page_size,
            status=status_filter,
            task_type=task_type,
            case_id=case_id,
        )
    except InvalidTaskCaseIDError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc

    return {
        **_request_meta(),
        "data": {
            "items": [item.model_dump() for item in items],
            "page": page,
            "page_size": page_size,
            "total": total,
        },
    }


@router.get("/tasks/{task_id}")
async def get_task(
    task_id: str,
    case_id: str = Query(
        ...,
        min_length=4,
        max_length=80,
        pattern=CASE_ID_PATTERN.pattern,
    ),
    task_service: TaskService = Depends(get_task_service),
) -> dict:
    try:
        task = task_service.get_public_task(task_id, case_id=case_id)
    except InvalidTaskCaseIDError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc
    except TaskNotFoundError:
        raise HTTPException(status_code=404, detail="task_not_found")

    return {
        **_request_meta(),
        "data": task.model_dump(),
    }
