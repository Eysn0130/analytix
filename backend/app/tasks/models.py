from datetime import datetime
from enum import Enum
from typing import Any, Dict, Optional

from pydantic import BaseModel, Field


class TaskType(str, Enum):
    SYSTEM = "system"
    IMPORT = "import"
    CLEANING = "cleaning"
    EXPORT = "export"
    FLOW_BUILD = "flow_build"
    ANALYSIS_TRACE = "analysis_trace"
    ANALYSIS_WORKER = "analysis_worker"
    ANALYSIS_REFRESH = "analysis_refresh"
    ANALYSIS_MATERIALIZE = "analysis_materialize"
    ANALYSIS_MAINTENANCE = "analysis_maintenance"
    WORKSPACE_REBUILD = "workspace_rebuild"
    MEMORY_PROMOTION = "memory_promotion"
    SCHEDULER_EXECUTION = "scheduler_execution"
    STATS_EXPORT = "stats_export"
    STATS_QUERY = "stats_query"


class TaskStatus(str, Enum):
    QUEUED = "queued"
    RUNNING = "running"
    SUCCEEDED = "succeeded"
    FAILED = "failed"
    CANCELED = "canceled"


class TaskRecord(BaseModel):
    task_id: str
    task_type: TaskType
    status: TaskStatus
    progress: int = Field(default=0, ge=0, le=100)
    case_id: Optional[str] = None
    metadata: Dict[str, Any] = Field(default_factory=dict)
    error: Optional[str] = None
    created_at: datetime
    updated_at: datetime


class TaskCreateRequest(BaseModel):
    task_type: TaskType
    case_id: Optional[str] = None
    metadata: Dict[str, Any] = Field(default_factory=dict)


class TaskTransitionRequest(BaseModel):
    to_status: TaskStatus
    progress: Optional[int] = Field(default=None, ge=0, le=100)
    error: Optional[str] = None
    metadata: Dict[str, Any] = Field(default_factory=dict)
