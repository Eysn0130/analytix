from __future__ import annotations

from typing import Any, Dict, List, Optional

from pydantic import BaseModel, Field


class SkillContractSummaryDTO(BaseModel):
    skill_id: str
    version: str = ""
    title: str = ""
    display_name: str = ""
    action_label: str = ""
    intent: str = ""
    kind: str = ""
    read_only: bool = True
    supports_parallel_tool_calls: bool = False
    parallel_safety: str = ""
    side_effect_class: str = ""
    writes_to: List[str] = Field(default_factory=list)
    privacy_mode: str = ""
    required_permissions: List[str] = Field(default_factory=list)
    cost_class: str = ""
    timeout_class: str = ""
    surface: str = "interactive"
    visibility_tier: str = "user_visible"
    chat_visible: bool = True
    report_internal: bool = False
    report_role: str = "none"
    narrative_allowed: bool = True
    deprecated_at: Optional[str] = None
    runtime_boundary: Dict[str, str] = Field(default_factory=dict)


class SkillContractDTO(SkillContractSummaryDTO):
    executor: dict[str, Any] = Field(default_factory=dict)
    evidence_contract: dict[str, Any] = Field(default_factory=dict)
    when_to_use: List[str] = Field(default_factory=list)
    when_not_to_use: List[str] = Field(default_factory=list)
    input_schema: dict[str, Any] = Field(default_factory=dict)
    output_schema: dict[str, Any] = Field(default_factory=dict)
    policy: dict[str, Any] = Field(default_factory=dict)
    examples: dict[str, Any] = Field(default_factory=dict)
    experience: dict[str, Any] = Field(default_factory=dict)
    playbook: dict[str, Any] = Field(default_factory=dict)


class SkillListDTO(BaseModel):
    items: List[SkillContractSummaryDTO] = Field(default_factory=list)


class SkillExecuteReq(BaseModel):
    input: dict[str, Any] = Field(default_factory=dict)
