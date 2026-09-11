from __future__ import annotations

from typing import List, Literal, Optional

from pydantic import BaseModel, ConfigDict, Field, model_validator


class CleaningJobReq(BaseModel):
    case_id: str = Field(min_length=1)
    steps: List[int] = Field(default_factory=list)
    force_rebuild: bool = False


class CleaningResetReq(BaseModel):
    reason: str = Field(default="", max_length=200)


class CleaningJobDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    job_id: str
    case_id: str
    status: str
    progress: int = Field(ge=0, le=100)
    result_semantic_status: Literal["blocked"] = "blocked"
    result_blocker: Literal["host_evidence_receipt_required"] = "host_evidence_receipt_required"
    fact_answer_allowed: Literal[False] = False
    cleaned_rows: Optional[int] = Field(default=None, ge=0)
    summary: dict = Field(default_factory=dict)
    error: Optional[str] = None
    created_at: str
    updated_at: str

    @model_validator(mode="after")
    def validate_public_job_boundary(self):
        if self.cleaned_rows is not None or self.summary:
            raise ValueError("cleaning job facts require host evidence authority")
        return self
