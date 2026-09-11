from __future__ import annotations

import asyncio
import json
import logging
import os
from contextlib import asynccontextmanager
from itertools import count

from fastapi import Depends, FastAPI, Query, Request, WebSocket, WebSocketDisconnect
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from app.api.data_analysis_deps import get_settings
from app.api.router import api_router
from app.core.data_engine_client import data_engine_product_error, shutdown_data_engine
from app.core.safe_observability import configure_closed_process_logging, log_closed_diagnostic
from app.core.execution_authority_health import (
    execution_authority_health,
    unadmitted_execution_capability_health,
)
from app.core.paths import ensure_dir, get_app_data_dir
from app.core.process_environment import clear_sensitive_data_analysis_environment
from app.domain.analysis_service import AnalysisService
from app.domain.analysis_worker_service import AnalysisWorkerService
from app.domain.case_service import CaseService
from app.domain.cleaning_service import CleaningService
from app.domain.document_service import DocumentService
from app.domain.export_service import ExportService
from app.domain.flow_service import FlowService
from app.domain.import_service import ImportService
from app.domain.privacy_projection_service import PrivacyProjectionService
from app.domain.stats_job_service import StatsJobService
from app.domain.stats_service import StatsService
from app.domain.workspace_projection_service import WorkspaceProjectionService
from app.infra.config import AppSettings, get_settings as load_settings
from app.infra.logging import configure_logging
from app.middleware.local_backend_auth import (
    DATA_ANALYSIS_AUTH_HEADER,
    DATA_ANALYSIS_LAUNCH_READY_PROTOCOL,
    LocalBackendAuthMiddleware,
    SENSITIVE_RESPONSE_HEADERS,
    build_launch_ready_proof,
)
from app.middleware.request_logging import RequestLoggingMiddleware, get_request_id
from app.repositories.analysis_repository import AnalysisRepository
from app.repositories.case_repository import CaseRepository
from app.repositories.cleaning_repository import CleaningRepository
from app.repositories.document_repository import DocumentRepository
from app.repositories.export_repository import ExportRepository
from app.repositories.flow_repository import FlowRepository
from app.repositories.import_repository import ImportRepository
from app.repositories.privacy_projection_repository import PrivacyProjectionRepository
from app.repositories.stats_repository import StatsRepository
from app.tasks.service import TaskService
from app.utils.time import utc_now
from app.ws.events import build_event
from app.ws.manager import WebSocketManager


def _build_lifecycle_health(request: Request, settings: AppSettings) -> dict:
    started_at = getattr(request.app.state, "started_at", utc_now())
    uptime_s = max((utc_now() - started_at).total_seconds(), 0.0)
    execution_authority = execution_authority_health()
    helper_execution = unadmitted_execution_capability_health(execution_authority)
    checks = {
        "config_loaded": getattr(request.app.state, "settings", None) is not None,
        "task_service_ready": getattr(request.app.state, "task_service", None) is not None,
        "case_service_ready": getattr(request.app.state, "case_service", None) is not None,
        "analysis_service_ready": getattr(request.app.state, "analysis_service", None) is not None,
        "analysis_worker_service_ready": getattr(request.app.state, "analysis_worker_service", None) is not None,
        "workspace_projection_service_ready": getattr(request.app.state, "workspace_projection_service", None) is not None,
        "privacy_projection_service_ready": getattr(request.app.state, "privacy_projection_service", None) is not None,
        "import_service_ready": getattr(request.app.state, "import_service", None) is not None,
        "document_service_ready": getattr(request.app.state, "document_service", None) is not None,
        "cleaning_service_ready": getattr(request.app.state, "cleaning_service", None) is not None,
        "export_service_ready": getattr(request.app.state, "export_service", None) is not None,
        "stats_service_ready": getattr(request.app.state, "stats_service", None) is not None,
        "stats_job_service_ready": getattr(request.app.state, "stats_job_service", None) is not None,
        "flow_service_ready": getattr(request.app.state, "flow_service", None) is not None,
        "ws_manager_ready": getattr(request.app.state, "ws_manager", None) is not None,
        "ws_sequence_ready": getattr(request.app.state, "ws_sequence", None) is not None,
    }
    return {
        "status": "ok" if all(checks.values()) else "degraded",
        "service": settings.app_name,
        "version": settings.app_version,
        "env": settings.app_env,
        "phase": "data-analysis",
        "managed_execution_available": execution_authority.available,
        "managed_execution_reason": execution_authority.reason_code,
        "cleaning_native_available": False,
        "cleaning_native_reason": helper_execution.reason_code,
        "cleaning_native_bin": "",
        "cleaning_native_bin_source": "",
        "legacy_python_cleaning_allowed": False,
        "archive_extraction_available": False,
        "archive_extraction_reason": helper_execution.reason_code,
        "archive_extraction_bin": "",
        "archive_extraction_bin_source": "",
        "archive_extraction_supported_exts": [],
        "analysis_compute_available": False,
        "analysis_compute_reason": helper_execution.reason_code,
        "analysis_compute_bin": "",
        "analysis_compute_required_commands": [],
        "data_engine_available": False,
        "data_engine_reason": helper_execution.reason_code,
        "data_engine_bin": "",
        "data_engine_pid": None,
        "document_conversion_available": False,
        "document_conversion_reason": helper_execution.reason_code,
        "privacy_projection_native_available": False,
        "privacy_projection_native_reason": helper_execution.reason_code,
        "privacy_projection_native_bin": "",
        "privacy_projection_native_bin_source": "",
        "uptime_s": round(uptime_s, 3),
        "checks": checks,
    }


@asynccontextmanager
async def lifespan(app: FastAPI):
    settings = load_settings()
    configure_logging(settings)
    configure_closed_process_logging()
    logger = logging.getLogger("analytix.data_analysis.lifecycle")

    app_data_dir = ensure_dir(get_app_data_dir())
    task_persist_path = app_data_dir / "data_analysis_task_store.json"

    app.state.settings = settings
    app.state.started_at = utc_now()
    app.state.task_service = TaskService(
        persist_path=task_persist_path,
        max_records=settings.task_store_max_records,
        terminal_ttl_days=settings.task_store_terminal_ttl_days,
    )
    app.state.case_service = CaseService(CaseRepository())

    privacy_projection_repository = PrivacyProjectionRepository()
    analysis_repository = AnalysisRepository(
        privacy_projection_repository=privacy_projection_repository,
    )
    app.state.privacy_projection_service = PrivacyProjectionService(repository=privacy_projection_repository)
    app.state.workspace_projection_service = WorkspaceProjectionService(analysis_repository)
    app.state.analysis_service = AnalysisService(
        analysis_repository,
        task_service=app.state.task_service,
        workspace_projection_service=app.state.workspace_projection_service,
        privacy_projection_repository=privacy_projection_repository,
    )
    app.state.analysis_worker_service = AnalysisWorkerService(
        analysis_service=app.state.analysis_service,
        task_service=app.state.task_service,
        max_workers=max(2, int(getattr(settings, "stats_job_workers", 2) or 2)),
    )
    app.state.analysis_worker_service.start_background_maintenance(
        interval_s=settings.analysis_temp_scope_gc_interval_s,
        startup_delay_s=settings.analysis_temp_scope_gc_startup_delay_s,
    )

    app.state.ws_sequence = count(start=1)
    app.state.ws_manager = WebSocketManager()
    app.state.ws_manager.bind_loop(asyncio.get_running_loop())

    document_repository = DocumentRepository(
        chunk_size=settings.document_chunk_size,
        chunk_overlap=settings.document_chunk_overlap,
        ocr_enabled=settings.document_ocr_enabled,
        ocr_language=settings.document_ocr_language,
        vector_mode=settings.document_vector_mode,
    )
    app.state.document_service = DocumentService(repository=document_repository)
    app.state.stats_service = StatsService(repository=StatsRepository())
    app.state.flow_service = FlowService(
        repository=FlowRepository(),
        task_service=app.state.task_service,
        ws_manager=app.state.ws_manager,
        ws_sequence=app.state.ws_sequence,
        ws_version=settings.ws_contract_version,
    )
    app.state.flow_service.start_background_maintenance(
        interval_s=settings.flow_snapshot_gc_interval_s,
        startup_delay_s=settings.flow_snapshot_gc_startup_delay_s,
    )

    cleaning_repository = CleaningRepository()
    app.state.import_service = ImportService(
        task_service=app.state.task_service,
        repository=ImportRepository(document_repository=document_repository),
        ws_manager=app.state.ws_manager,
        ws_sequence=app.state.ws_sequence,
        cleaning_repository=cleaning_repository,
        ws_version=settings.ws_contract_version,
    )
    app.state.cleaning_service = CleaningService(
        task_service=app.state.task_service,
        repository=cleaning_repository,
        ws_manager=app.state.ws_manager,
        ws_sequence=app.state.ws_sequence,
        ws_version=settings.ws_contract_version,
    )
    app.state.export_service = ExportService(
        task_service=app.state.task_service,
        repository=ExportRepository(),
        ws_manager=app.state.ws_manager,
        ws_sequence=app.state.ws_sequence,
        ws_version=settings.ws_contract_version,
    )
    app.state.stats_job_service = StatsJobService(
        task_service=app.state.task_service,
        stats_service=app.state.stats_service,
        ws_manager=app.state.ws_manager,
        ws_sequence=app.state.ws_sequence,
        ws_version=settings.ws_contract_version,
        max_workers=settings.stats_job_workers,
    )

    log_closed_diagnostic(
        logger,
        logging.INFO,
        topic="application",
        code="completed",
    )

    try:
        yield
    finally:
        for name in ("stats_job_service", "analysis_worker_service", "analysis_service", "flow_service"):
            service = getattr(app.state, name, None)
            if service is not None and hasattr(service, "shutdown"):
                service.shutdown()
        shutdown_data_engine()
        uptime_s = max((utc_now() - app.state.started_at).total_seconds(), 0.0)
        log_closed_diagnostic(
            logger,
            logging.INFO,
            topic="application",
            code="completed",
            numeric={"duration_ms": round(uptime_s * 1000, 3)},
        )


def create_app() -> FastAPI:
    boot_settings = load_settings()
    clear_sensitive_data_analysis_environment()
    app = FastAPI(
        title="analytix data analysis service",
        version=boot_settings.app_version,
        description="Local data import, cleaning, statistics, and visualization API for analytix.",
        lifespan=lifespan,
    )
    app.state.boot_settings = boot_settings
    app.add_middleware(
        LocalBackendAuthMiddleware,
        expected_token=boot_settings.data_analysis_auth_token.get_secret_value(),
    )
    app.add_middleware(
        CORSMiddleware,
        allow_origins=boot_settings.cors_allow_origins,
        allow_credentials=True,
        allow_methods=["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"],
        allow_headers=["Accept", "Content-Type", "X-Request-Id", DATA_ANALYSIS_AUTH_HEADER],
        expose_headers=[
            "Content-Encoding",
            "Content-Length",
            "Server-Timing",
            "X-Analytix-Content-Encoding",
            "X-Analytix-Json-Bytes",
            "X-Analytix-Request-Duration-Ms",
        ],
    )
    app.add_middleware(RequestLoggingMiddleware)

    @app.get("/")
    async def root():
        return JSONResponse(content={"status": "alive"}, headers=SENSITIVE_RESPONSE_HEADERS)

    @app.get("/health")
    async def health():
        return JSONResponse(content={"status": "alive"}, headers=SENSITIVE_RESPONSE_HEADERS)

    @app.get("/health/live")
    async def health_live():
        return JSONResponse(content={"status": "alive"}, headers=SENSITIVE_RESPONSE_HEADERS)

    @app.get("/health/launch-ready")
    async def health_launch_ready(
        request: Request,
        challenge: str = Query(min_length=43, max_length=43, pattern=r"^[A-Za-z0-9_-]{43}$"),
        settings: AppSettings = Depends(get_settings),
    ):
        lifecycle = _build_lifecycle_health(request, settings)
        if lifecycle.get("status") != "ok":
            return JSONResponse(
                status_code=503,
                content={"status": "degraded"},
                headers=SENSITIVE_RESPONSE_HEADERS,
            )
        service = str(lifecycle.get("service") or "")
        pid = os.getpid()
        try:
            proof = build_launch_ready_proof(
                token=settings.data_analysis_auth_token.get_secret_value(),
                launch_id=settings.data_analysis_launch_id,
                challenge=challenge,
                service=service,
                status="ok",
                pid=pid,
            )
        except ValueError:
            return JSONResponse(
                status_code=503,
                content={"status": "unavailable"},
                headers=SENSITIVE_RESPONSE_HEADERS,
            )
        return JSONResponse(
            content={
                "protocol": DATA_ANALYSIS_LAUNCH_READY_PROTOCOL,
                "service": service,
                "status": "ok",
                "launchId": settings.data_analysis_launch_id,
                "challenge": challenge,
                "pid": pid,
                "proof": proof,
            },
            headers=SENSITIVE_RESPONSE_HEADERS,
        )

    @app.get("/health/ready")
    async def health_ready(request: Request, settings: AppSettings = Depends(get_settings)):
        payload = _build_lifecycle_health(request, settings)
        return JSONResponse(status_code=200 if payload["status"] == "ok" else 503, content=payload)

    app.include_router(api_router, prefix="/api/v1")

    @app.websocket("/ws/events")
    async def ws_events(websocket: WebSocket) -> None:
        settings_obj = getattr(websocket.app.state, "settings", None)
        if settings_obj is None:
            await websocket.close(code=1011, reason="settings_not_ready")
            return

        case_id = str(websocket.query_params.get("case_id") or "").strip()
        if not case_id:
            await websocket.close(code=4400, reason="case_id_required")
            return

        sequence = getattr(websocket.app.state, "ws_sequence", count(start=1))
        ws_manager: WebSocketManager = getattr(websocket.app.state, "ws_manager", None)
        if ws_manager is None:
            await websocket.close(code=1011, reason="websocket_manager_not_ready")
            return
        try:
            await ws_manager.connect(websocket, case_id)
        except ValueError:
            await websocket.close(code=4400, reason="case_id_invalid")
            return

        try:
            await websocket.accept()
            await websocket.send_json(
                build_event(
                    event="system.ready",
                    event_type="info",
                    channel="system",
                    sequence=next(sequence),
                    version=settings_obj.ws_contract_version,
                    case_id=case_id,
                    payload={"service": "analytix-data-analysis", "phase": "data-analysis"},
                )
            )
            while True:
                raw_message = await websocket.receive_text()
                try:
                    client_message = json.loads(raw_message or "{}")
                except json.JSONDecodeError:
                    client_message = {}
                if not isinstance(client_message, dict):
                    continue
        except WebSocketDisconnect:
            pass
        finally:
            await ws_manager.disconnect(websocket)

    @app.exception_handler(Exception)
    async def unhandled_exception_handler(_: Request, exc: Exception):
        logger = logging.getLogger("analytix.data_analysis.exception")
        try:
            data_engine_error = data_engine_product_error(exc)
        except Exception:
            data_engine_error = None
        log_closed_diagnostic(
            logger,
            logging.ERROR,
            topic="application",
            code="data_engine_failed" if data_engine_error is not None else "internal_error",
            flags={"retryable": bool(data_engine_error.get("retryable", True))}
            if data_engine_error is not None
            else {"retryable": True},
        )
        if data_engine_error is not None:
            return JSONResponse(
                status_code=int(data_engine_error.get("status_code") or 500),
                content={
                    "request_id": get_request_id(),
                    "timestamp": utc_now().isoformat(),
                    "error": {
                        "code": data_engine_error.get("code"),
                        "message": data_engine_error.get("message"),
                        "retryable": bool(data_engine_error.get("retryable", True)),
                        "details": data_engine_error.get("details") or {},
                    },
                },
            )
        return JSONResponse(
            status_code=500,
            content={
                "request_id": get_request_id(),
                "timestamp": utc_now().isoformat(),
                "error": {
                    "code": "INTERNAL_ERROR",
                    "message": "unexpected server error",
                    "retryable": True,
                    "details": {},
                },
            },
        )

    return app


app = create_app()
