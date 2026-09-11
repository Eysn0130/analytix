from typing import Any, Dict, List, Optional

from pydantic import BaseModel, Field

from app.tasks.models import TaskRecord


class RequestMeta(BaseModel):
    request_id: str
    timestamp: str


class HealthChecks(BaseModel):
    config_loaded: bool
    task_service_ready: bool
    ws_sequence_ready: bool


class HealthPayload(BaseModel):
    status: str
    service: str
    version: str
    env: str
    phase: str
    runtime_architecture: str
    codex_aligned: bool
    python_version: str
    codex_runtime_min_python: str
    codex_runtime_ready: bool
    codex_runtime_reason: str
    managed_execution_available: bool
    managed_execution_reason: str
    document_conversion_available: bool
    document_conversion_reason: str
    archive_extraction_available: bool
    archive_extraction_reason: str
    archive_extraction_bin: str
    archive_extraction_bin_source: str
    archive_extraction_supported_exts: List[str]
    uptime_s: float
    checks: HealthChecks


class HealthResponse(BaseModel):
    request_id: str
    timestamp: str
    data: HealthPayload


class TaskListPayload(BaseModel):
    items: List[TaskRecord]
    page: int
    page_size: int
    total: int


class TaskListResponse(BaseModel):
    request_id: str
    timestamp: str
    data: TaskListPayload


class TaskItemResponse(BaseModel):
    request_id: str
    timestamp: str
    data: TaskRecord


class ErrorBody(BaseModel):
    code: str
    message: str
    retryable: bool = False
    details: Dict[str, Any] = Field(default_factory=dict)


class ErrorResponse(BaseModel):
    request_id: str
    timestamp: str
    error: ErrorBody
