from __future__ import annotations

from typing import Literal, Optional

from pydantic import BaseModel, ConfigDict, Field


class ExportReq(BaseModel):
    case_id: str = Field(min_length=1)
    export_format: str = Field(default="xlsx", pattern="^(csv|xlsx)$")
    filters: dict = Field(default_factory=dict)
    output_name: str = Field(default="", max_length=180)
    target_dir: Optional[str] = Field(default=None, max_length=4096)


class ExportJobDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    job_id: str
    case_id: str
    status: Literal["queued", "running", "succeeded", "failed", "canceled"]
    progress: int = Field(ge=0, le=100)
    output_path: Literal[None] = None
    error: Optional[
        Literal["task_failed", "task_canceled", "task_interrupted", "task_runtime_payload_unavailable"]
    ] = None
    artifact_access: Literal["controlled_artifact_required"] = "controlled_artifact_required"
    publication_status: Literal["blocked"] = "blocked"
    created_at: str
    updated_at: str
