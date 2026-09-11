from __future__ import annotations

from typing import Annotated, Any, List, Literal, Optional

from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.tasks.failure_boundary import canonical_task_id


class AnalysisRuleParamsDTO(BaseModel):
    case_id: str
    effective_params: dict[str, Any] = Field(default_factory=dict)
    case_params: dict[str, Any] = Field(default_factory=dict)
    system_params: dict[str, Any] = Field(default_factory=dict)


class AnalysisUpdateRuleParamsReq(BaseModel):
    model_config = ConfigDict(extra="forbid")

    case_id: str = Field(min_length=1, max_length=128)
    scope_type: Literal["case", "system"] = "case"
    params: dict[str, Any] = Field(default_factory=dict)


class AnalysisUpdateRuleParamsDTO(BaseModel):
    case_id: str
    scope_type: str
    updated_keys: List[str] = Field(default_factory=list)
    effective_params: dict[str, Any] = Field(default_factory=dict)


class AnalysisTraceRunReq(BaseModel):
    case_id: str = Field(min_length=1, max_length=128)
    seed_type: Literal["txn_id", "account", "entity_id"] = "txn_id"
    seed_value: str = Field(min_length=1, max_length=256)
    depth: int = Field(default=3, ge=1, le=6)
    time_window_sec: int = Field(default=1800, ge=60, le=86400)
    tolerance_rate: float = Field(default=0.03, ge=0, le=1)
    mode: Literal["sync", "async"] = "sync"


class AnalysisTracePublicDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["AnalysisTracePublicV1"] = "AnalysisTracePublicV1"
    trace_ref: str = Field(pattern=r"^atrace_v1_[a-f0-9]{64}$")
    operation_status: Literal["queued", "running", "succeeded", "failed", "cancelled", "interrupted", "unknown"]
    path_refs: List[Annotated[str, Field(pattern=r"^apath_v1_[a-f0-9]{64}$")]] = Field(default_factory=list)
    coverage_status: Literal["unverified"] = "unverified"
    publication_status: Literal["blocked"] = "blocked"
    fact_answer_allowed: Literal[False] = False
    content_access: Literal["controlled_artifact_required"] = "controlled_artifact_required"


class AnalysisEvidencePublicRefDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    evidence_ref: str = Field(pattern=r"^aevidence_v1_[a-f0-9]{64}$")
    verification_status: Literal["unresolved"] = "unresolved"
    content_access: Literal["controlled_artifact_required"] = "controlled_artifact_required"


class AnalysisTracePathPublicDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["AnalysisTracePathPublicV1"] = "AnalysisTracePathPublicV1"
    trace_ref: str = Field(pattern=r"^atrace_v1_[a-f0-9]{64}$")
    path_ref: str = Field(pattern=r"^apath_v1_[a-f0-9]{64}$")
    availability_status: Literal["available"] = "available"
    coverage_status: Literal["unverified"] = "unverified"
    publication_status: Literal["blocked"] = "blocked"
    fact_answer_allowed: Literal[False] = False
    evidence_refs: List[AnalysisEvidencePublicRefDTO] = Field(default_factory=list)
    content_access: Literal["controlled_artifact_required"] = "controlled_artifact_required"


class AnalysisRefreshReq(BaseModel):
    case_id: str = Field(min_length=1, max_length=128)
    force_refresh: bool = False
    mode: Literal["sync", "async"] = "sync"


class AnalysisMaintenancePublicDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["AnalysisMaintenancePublicBoundaryV1"] = "AnalysisMaintenancePublicBoundaryV1"
    case_id: str
    operation: Literal["analysis_refresh", "feature_mart_materialize"]
    operation_status: Literal["blocked", "queued"]
    semantic_status: Literal["blocked"] = "blocked"
    blocker: Literal["host_evidence_receipt_required"] = "host_evidence_receipt_required"
    publication_status: Literal["blocked"] = "blocked"
    fact_answer_allowed: Literal[False] = False
    job_id: str = ""

    @model_validator(mode="after")
    def validate_maintenance_job_state(self):
        if (self.operation_status == "queued") != bool(self.job_id):
            raise ValueError("queued maintenance boundary requires exactly one job id")
        if self.job_id and canonical_task_id(self.job_id) != self.job_id:
            raise ValueError("queued maintenance boundary requires a canonical task id")
        if self.operation == "feature_mart_materialize" and self.operation_status != "blocked":
            raise ValueError("quarantined materialization must remain blocked")
        return self


class AnalysisAccountScopeCoverageDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["AnalysisAccountScopeCoverageV1"] = "AnalysisAccountScopeCoverageV1"
    status: Literal["unverified"] = "unverified"
    completeness: Literal["unknown"] = "unknown"
    checked_scope: Literal["none"] = "none"
    transaction_rows: None = None
    amount_present_rows: None = None
    amount_missing_rows: None = None
    amount_parse_failed_rows: None = None
    direction_covered_rows: None = None


class AnalysisAccountFactBoundaryDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["AnalysisAccountFactBoundaryV1"] = "AnalysisAccountFactBoundaryV1"
    case_id: str
    operation: Literal[
        "account_counterparty_rankings",
        "account_behavior_profile",
        "scope_account_snapshot",
    ]
    semantic_status: Literal["blocked"] = "blocked"
    blocker: Literal["host_evidence_receipt_required"] = "host_evidence_receipt_required"
    publication_status: Literal["blocked"] = "blocked"
    fact_answer_allowed: Literal[False] = False
    raw_details_exposed: Literal[False] = False
    coverage: AnalysisAccountScopeCoverageDTO = Field(default_factory=AnalysisAccountScopeCoverageDTO)
    data: dict[str, Any] = Field(default_factory=dict)
    evidence_receipts: List[dict[str, Any]] = Field(default_factory=list)
    claim_records: List[dict[str, Any]] = Field(default_factory=list)
    summary_text: Literal[""] = ""

    @model_validator(mode="after")
    def validate_account_fact_boundary(self):
        if self.data or self.evidence_receipts or self.claim_records:
            raise ValueError("account facts require Go host evidence authority")
        return self


class AnalysisFeatureMartMaterializeReq(BaseModel):
    model_config = ConfigDict(extra="forbid")

    case_id: str = Field(min_length=1, max_length=128)
    account_keys: List[str] = Field(default_factory=list)
    temp_scope_id: str = ""
    source_file_ids: List[str] = Field(default_factory=list)
    force_refresh: bool = False
    max_accounts: int = Field(default=12, ge=1, le=64)
    mode: Literal["sync", "async", "queue"] = "sync"


class AnalysisEvidencePackPublicDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["AnalysisEvidencePackPublicBoundaryV1"] = "AnalysisEvidencePackPublicBoundaryV1"
    case_id: str
    operation: Literal["evidence_pack_materialize"] = "evidence_pack_materialize"
    operation_status: Literal["blocked"] = "blocked"
    semantic_status: Literal["blocked"] = "blocked"
    blocker: Literal["host_evidence_receipt_required"] = "host_evidence_receipt_required"
    publication_status: Literal["blocked"] = "blocked"
    fact_answer_allowed: Literal[False] = False
    coverage_status: Literal["unverified"] = "unverified"
    coverage_completeness: Literal["unknown"] = "unknown"
    checked_scope: Literal["none"] = "none"
    data: dict[str, Any] = Field(default_factory=dict)
    evidence_receipts: List[dict[str, Any]] = Field(default_factory=list)
    claim_records: List[dict[str, Any]] = Field(default_factory=list)

    @model_validator(mode="after")
    def validate_evidence_pack_boundary(self):
        if self.data or self.evidence_receipts or self.claim_records:
            raise ValueError("evidence pack facts require Go host evidence authority")
        return self


class AnalysisScopeCachePublicDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["AnalysisScopeCachePublicBoundaryV1"] = "AnalysisScopeCachePublicBoundaryV1"
    case_id: str
    operation_status: Literal["blocked"] = "blocked"
    semantic_status: Literal["blocked"] = "blocked"
    blocker: Literal["host_evidence_receipt_required"] = "host_evidence_receipt_required"
    publication_status: Literal["blocked"] = "blocked"
    fact_answer_allowed: Literal[False] = False
    items: List[dict] = Field(default_factory=list)

    @model_validator(mode="after")
    def validate_scope_cache_boundary(self):
        if self.items:
            raise ValueError("scope cache facts require host evidence authority")
        return self


class AnalysisWorkspaceRebuildReq(BaseModel):
    case_id: str = Field(min_length=1, max_length=128)
    files: List[str] = Field(default_factory=list)
    sync_baseline: bool = False
    render_mode: str = "manual_rebuild"
    mode: Literal["sync", "async", "queue"] = "sync"


class AnalysisWorkspaceRebuildDTO(BaseModel):
    case_id: str
    rendered_files: List[dict[str, Any]] = Field(default_factory=list)
    rendered_count: int = 0
    job_id: str = ""


class AnalysisJanitorReq(BaseModel):
    case_id: str = Field(default="", max_length=128)
    mode: Literal["sync", "async", "queue"] = "sync"


class AnalysisJanitorDTO(BaseModel):
    result_status: Literal["pending", "completed"]
    job_kind: str = ""
    case_count: Optional[int] = Field(default=None, ge=0)
    affected_case_count: Optional[int] = Field(default=None, ge=0)
    affected_scope_count: Optional[int] = Field(default=None, ge=0)
    affected_item_count: Optional[int] = Field(default=None, ge=0)
    scanned_count: Optional[int] = Field(default=None, ge=0)
    stale_count: Optional[int] = Field(default=None, ge=0)
    deleted_count: Optional[int] = Field(default=None, ge=0)
    skipped_count: Optional[int] = Field(default=None, ge=0)
    items: List[dict[str, Any]] = Field(default_factory=list)
    job_id: str = ""

    @model_validator(mode="after")
    def validate_janitor_result_state(self):
        counts = (
            self.case_count,
            self.affected_case_count,
            self.affected_scope_count,
            self.affected_item_count,
            self.scanned_count,
            self.stale_count,
            self.deleted_count,
            self.skipped_count,
        )
        if self.result_status == "pending":
            if canonical_task_id(self.job_id) != self.job_id or any(value is not None for value in counts):
                raise ValueError("pending janitor result cannot publish counts")
            if self.items:
                raise ValueError("pending janitor result cannot publish items")
        elif self.job_id:
            raise ValueError("completed janitor result cannot retain a pending job id")
        return self


class AnalysisReportTemplateConfigDTO(BaseModel):
    template_id: str = "standard"
    template_label: str = ""
    organization_name: str = ""
    header_title: str = ""
    header_subtitle: str = ""
    footer_note: str = ""
    page_number_style: Literal["cn_simple", "compact", "arabic"] = "cn_simple"
    signature_label: str = "签发"
    signature_name: str = ""
    signature_title: str = ""
    signature_date: str = ""
    show_signature_block: bool = True


class AnalysisReportDraftReq(BaseModel):
    case_id: str = Field(min_length=1, max_length=128)
    scope: str = Field(default="stage_report", min_length=1, max_length=80)
    section_plan: List[str] = Field(default_factory=list, max_length=16)
    template_config: AnalysisReportTemplateConfigDTO = Field(default_factory=AnalysisReportTemplateConfigDTO)


class AnalysisReportTemplatePresetDTO(BaseModel):
    template_id: str
    template_label: str
    description: str = ""
    default_section_plan: List[str] = Field(default_factory=list)
    template_config: AnalysisReportTemplateConfigDTO = Field(default_factory=AnalysisReportTemplateConfigDTO)


class AnalysisReportDraftDTO(BaseModel):
    report_id: str
    scope: str
    title: str
    outline: List[str] = Field(default_factory=list)
    content_md: str = ""
    evidence_ids: List[str] = Field(default_factory=list)
    query_ids: List[str] = Field(default_factory=list)
    version: int = 1
    status: str = "draft"
    publication_status: str = "unpublished"
    review_history: List[dict[str, Any]] = Field(default_factory=list)
    template_config: AnalysisReportTemplateConfigDTO = Field(default_factory=AnalysisReportTemplateConfigDTO)
    frozen_citations: dict[str, List[str]] = Field(default_factory=dict)
    approval_note_ids: List[str] = Field(default_factory=list)


class AnalysisReportExportFileDTO(BaseModel):
    file_name: str
    file_path: str
    mime_type: str = ""


class AnalysisReportPublishReq(BaseModel):
    case_id: str = Field(min_length=1, max_length=128)
    report_id: str = Field(default="", max_length=128)
    scope: str = Field(default="stage_report", min_length=1, max_length=80)
    write_workspace: bool = True
    export_report: bool = False
    export_format: Literal["html", "md", "pdf", "docx", "both", "all"] = "all"
    template_config: AnalysisReportTemplateConfigDTO = Field(default_factory=AnalysisReportTemplateConfigDTO)


class AnalysisReportWorkflowReq(BaseModel):
    case_id: str = Field(min_length=1, max_length=128)
    report_id: str = Field(min_length=1, max_length=128)
    action: Literal["submit", "confirm", "reopen"] = "submit"
    actor: str = Field(default="", max_length=80)
    comment: str = Field(default="", max_length=500)


class AnalysisReportDiffSectionDTO(BaseModel):
    section: str
    changed: bool = False
    base_preview: str = ""
    target_preview: str = ""
    diff_lines: List[str] = Field(default_factory=list)


class AnalysisReportDiffDTO(BaseModel):
    report_id: str
    baseline_report_id: str = ""
    report_status: str = ""
    baseline_status: str = ""
    changed_sections: List[str] = Field(default_factory=list)
    citation_delta: dict[str, List[str]] = Field(default_factory=dict)
    section_diffs: List[AnalysisReportDiffSectionDTO] = Field(default_factory=list)


class AnalysisReportWorkflowDTO(AnalysisReportDraftDTO):
    comparison: Optional[AnalysisReportDiffDTO] = None


class AnalysisReportTemplateListDTO(BaseModel):
    items: List[AnalysisReportTemplatePresetDTO] = Field(default_factory=list)


class AnalysisWorkspaceFileDTO(BaseModel):
    file_name: str
    version: int
    updated_at: str


class AnalysisWorkspaceRenderedFileDTO(AnalysisWorkspaceFileDTO):
    content_md: str


class AnalysisReportPublishDTO(AnalysisReportDraftDTO):
    workspace_file: Optional[AnalysisWorkspaceRenderedFileDTO] = None
    export_files: List[AnalysisReportExportFileDTO] = Field(default_factory=list)
