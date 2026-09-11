from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, Field


class RunLogEventDTO(BaseModel):
    event_id: str = ""
    case_id: str = ""
    task_id: str = ""
    run_id: str = ""
    turn_id: str = ""
    sequence_no: int = 0
    event_stage: str = ""
    event_type: str = ""
    source: str = "live"
    payload: dict[str, Any] = Field(default_factory=dict)
    created_at: str = ""


class RunLogTimelineEntryDTO(BaseModel):
    sequence_no: int = 0
    stage: str = ""
    event_type: str = ""
    source: str = "live"
    created_at: str = ""
    payload_keys: list[str] = Field(default_factory=list)
    terminal: bool = False
    status_hint: str = ""


class RunLogStageSummaryDTO(BaseModel):
    stage: str = ""
    event_count: int = 0
    first_sequence_no: int = 0
    last_sequence_no: int = 0
    event_types: list[str] = Field(default_factory=list)
    source_counts: dict[str, int] = Field(default_factory=dict)


class RunLogSummaryDTO(BaseModel):
    case_id: str = ""
    run_id: str = ""
    turn_id: str = ""
    event_count: int = 0
    first_sequence_no: int = 0
    last_sequence_no: int = 0
    first_created_at: str = ""
    last_created_at: str = ""
    stages: list[str] = Field(default_factory=list)
    event_types: list[str] = Field(default_factory=list)
    source_counts: dict[str, int] = Field(default_factory=dict)
    final_status: Literal["empty", "running", "done", "failed"] = "empty"
    terminal_event_type: str = ""
    replay_ready: bool = False


class RunLogReplayDTO(BaseModel):
    case_id: str = ""
    run_id: str = ""
    turn_id: str = ""
    summary: RunLogSummaryDTO
    stage_summary: list[RunLogStageSummaryDTO] = Field(default_factory=list)
    timeline: list[RunLogTimelineEntryDTO] = Field(default_factory=list)
    events: list[RunLogEventDTO] = Field(default_factory=list)
