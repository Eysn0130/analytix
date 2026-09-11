from __future__ import annotations

from typing import List, Literal, Optional

from pydantic import BaseModel, ConfigDict, Field, model_validator


class CleaningStepSummaryDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    step: int = Field(ge=1, le=10)
    key: str
    title: str
    kind: str
    description: str
    affected_rows: None = None


class CleaningStepSummaryListDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["CleaningStepSummaryPublicBoundaryV1"] = "CleaningStepSummaryPublicBoundaryV1"
    case_id: str
    semantic_status: Literal["blocked"] = "blocked"
    blocker: Literal["host_evidence_receipt_required"] = "host_evidence_receipt_required"
    publication_status: Literal["blocked"] = "blocked"
    fact_answer_allowed: Literal[False] = False
    items: List[CleaningStepSummaryDTO] = Field(default_factory=list)


class CleaningStepDetailRowDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    values: List[str] = Field(default_factory=list)


class CleaningStepDetailDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["CleaningStepDetailPublicBoundaryV1"] = "CleaningStepDetailPublicBoundaryV1"
    case_id: str
    semantic_status: Literal["blocked"] = "blocked"
    blocker: Literal["host_evidence_receipt_required"] = "host_evidence_receipt_required"
    publication_status: Literal["blocked"] = "blocked"
    fact_answer_allowed: Literal[False] = False
    raw_details_exposed: Literal[False] = False
    step: int = Field(ge=1, le=10)
    key: str
    title: str
    kind: str
    description: str
    headers: List[str] = Field(default_factory=list)
    items: List[CleaningStepDetailRowDTO] = Field(default_factory=list)
    page: None = None

    @model_validator(mode="after")
    def validate_public_detail_boundary(self):
        if self.items:
            raise ValueError("cleaning detail facts require host evidence authority")
        return self


class CleaningHistoryItemDTO(BaseModel):
    run_id: str
    cleaned_at: str
    import_at: str
    duration_ms: Optional[int] = Field(default=None, ge=0)
    scope_rows: Optional[int] = Field(default=None, ge=0)
    file_count: Optional[int] = Field(default=None, ge=0)
    summary: dict = Field(default_factory=dict)


class CleaningHistoryListDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["CleaningHistoryPublicBoundaryV1"] = "CleaningHistoryPublicBoundaryV1"
    case_id: str
    semantic_status: Literal["blocked"] = "blocked"
    blocker: Literal["host_evidence_receipt_required"] = "host_evidence_receipt_required"
    fact_answer_allowed: Literal[False] = False
    items: List[CleaningHistoryItemDTO] = Field(default_factory=list)

    @model_validator(mode="after")
    def validate_history_boundary(self):
        if self.items:
            raise ValueError("cleaning history facts require host evidence authority")
        return self


class CleaningLogEventDTO(BaseModel):
    event_id: str
    job_id: str
    event: str
    level: str
    message_code: str
    message: str
    step: Optional[int] = None
    progress: Optional[int] = Field(default=None, ge=0, le=100)
    timestamp: str


class CleaningLogListDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["CleaningLogPublicBoundaryV1"] = "CleaningLogPublicBoundaryV1"
    case_id: str
    semantic_status: Literal["blocked"] = "blocked"
    blocker: Literal["host_evidence_receipt_required"] = "host_evidence_receipt_required"
    fact_answer_allowed: Literal[False] = False
    items: List[CleaningLogEventDTO] = Field(default_factory=list)

    @model_validator(mode="after")
    def validate_log_boundary(self):
        if self.items:
            raise ValueError("cleaning log details require host evidence authority")
        return self
