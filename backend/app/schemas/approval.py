from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, Field


class ApprovalRecordDTO(BaseModel):
    approval_id: str = ""
    case_id: str = ""
    run_id: str = ""
    turn_id: str = ""
    session_id: str = ""
    skill_id: str = ""
    risk_type: str = ""
    risk_level: str = ""
    status: Literal["requested", "approved", "continued", "rejected"] = "requested"
    batch_id: str = ""
    batch_index: int = 0
    blocked: bool = False
    blocked_code: str = ""
    blocked_reason: str = ""
    retryable: bool = False
    retry_class: str = ""
    duplicate_batch: bool = False
    grant_root: str = ""
    network_target: str = ""
    network_host: str = ""
    network_protocol: str = ""
    network_port: int = 0
    available_decisions: list[dict[str, Any]] = Field(default_factory=list)
    policy_amendment_options: list[dict[str, Any]] = Field(default_factory=list)
    policy_amendment: dict[str, Any] = Field(default_factory=dict)
    denied_source: str = ""
    timed_out: bool = False
    review_reasoning: str = ""
    request_payload: dict[str, Any] = Field(default_factory=dict)
    decision_payload: dict[str, Any] = Field(default_factory=dict)
    created_at: str = ""
    updated_at: str = ""


class ApprovalDecisionReq(BaseModel):
    decision: Literal["approve", "approve_for_session", "apply_policy_amendment", "reject", "cancel", "timed_out"] = "approve"
    note: str = Field(default="", max_length=1000)
    policy_amendment: dict[str, Any] = Field(default_factory=dict)
    denied_source: str = ""
