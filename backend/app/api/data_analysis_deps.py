from __future__ import annotations

from fastapi import HTTPException, Request

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
from app.infra.config import AppSettings
from app.tasks.service import TaskService


def _state_service(request: Request, name: str):
    service = getattr(request.app.state, name, None)
    if service is None:
        raise HTTPException(status_code=503, detail=f"{name}_not_ready")
    return service


def get_settings(request: Request) -> AppSettings:
    return _state_service(request, "settings")


def get_task_service(request: Request) -> TaskService:
    return _state_service(request, "task_service")


def get_ws_sequence(request: Request):
    return _state_service(request, "ws_sequence")


def get_case_service(request: Request) -> CaseService:
    return _state_service(request, "case_service")


def get_analysis_service(request: Request) -> AnalysisService:
    return _state_service(request, "analysis_service")


def get_privacy_projection_service(request: Request) -> PrivacyProjectionService:
    return _state_service(request, "privacy_projection_service")


def get_analysis_worker_service(request: Request) -> AnalysisWorkerService:
    return _state_service(request, "analysis_worker_service")


def get_workspace_projection_service(request: Request) -> WorkspaceProjectionService:
    return _state_service(request, "workspace_projection_service")


def get_import_service(request: Request) -> ImportService:
    return _state_service(request, "import_service")


def get_cleaning_service(request: Request) -> CleaningService:
    return _state_service(request, "cleaning_service")


def get_document_service(request: Request) -> DocumentService:
    return _state_service(request, "document_service")


def get_export_service(request: Request) -> ExportService:
    return _state_service(request, "export_service")


def get_stats_service(request: Request) -> StatsService:
    return _state_service(request, "stats_service")


def get_stats_job_service(request: Request) -> StatsJobService:
    return _state_service(request, "stats_job_service")


def get_flow_service(request: Request) -> FlowService:
    return _state_service(request, "flow_service")
