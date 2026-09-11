from __future__ import annotations

from typing import Any, List, Literal, Optional, Union

from pydantic import BaseModel, ConfigDict, Field, model_validator


class StatsV2MetaReq(BaseModel):
    case_id: str = Field(min_length=1)


class StatsV2MetaDTO(BaseModel):
    case_id: str
    funds_status: str = "unknown"
    date_min: str = ""
    date_max: str = ""


class StatsV2CaseOverviewReq(BaseModel):
    case_id: str = Field(min_length=1)


class StatsV2CaseOverviewDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    case_id: str
    fact_answer_allowed: Literal[False] = False
    account_status: Literal["verified", "partial", "unavailable"]
    account_blocker: str = ""
    account_count: Optional[int] = Field(default=None, ge=0)
    personal_account_count: Optional[int] = Field(default=None, ge=0)
    corporate_account_count: Optional[int] = Field(default=None, ge=0)
    unknown_account_count: Optional[int] = Field(default=None, ge=0)
    transaction_status: Literal["verified", "partial", "unavailable"]
    transaction_blocker: str = ""
    transaction_count: Optional[int] = Field(default=None, ge=0)
    amount_status: Literal["verified", "partial", "unavailable"]
    amount_blocker: str = ""
    inflow_amount: Optional[float] = None
    outflow_amount: Optional[float] = None
    amount_total_rows: Optional[int] = Field(default=None, ge=0)
    amount_present_rows: Optional[int] = Field(default=None, ge=0)
    amount_missing_rows: Optional[int] = Field(default=None, ge=0)
    amount_parse_failed_rows: Optional[int] = Field(default=None, ge=0)
    direction_covered_rows: Optional[int] = Field(default=None, ge=0)
    source_table: str = ""
    account_source_table: str = ""
    source_revision: int = Field(default=1, ge=1)
    generated_at: str = ""

    @model_validator(mode="after")
    def validate_metric_authority(self):
        if self.account_status == "verified" or self.transaction_status == "verified" or self.amount_status == "verified":
            raise ValueError("case overview facts require a host evidence receipt")
        account_values = (
            self.account_count,
            self.personal_account_count,
            self.corporate_account_count,
            self.unknown_account_count,
        )
        if self.account_status == "verified":
            if any(value is None for value in account_values):
                raise ValueError("verified account overview requires exact counts")
            if self.personal_account_count + self.corporate_account_count + self.unknown_account_count != self.account_count:
                raise ValueError("verified account overview partition is inconsistent")
        elif any(value is not None for value in account_values):
            raise ValueError("unverified account overview cannot publish counts")

        if self.transaction_status == "verified":
            if self.transaction_count is None or self.transaction_count == 0:
                raise ValueError("verified transaction overview requires a non-empty evidenced scope")
        elif self.transaction_count is not None:
            raise ValueError("unverified transaction overview cannot publish a count")

        if self.amount_status == "verified":
            coverage = (
                self.amount_total_rows,
                self.amount_present_rows,
                self.amount_missing_rows,
                self.amount_parse_failed_rows,
                self.direction_covered_rows,
            )
            if self.inflow_amount is None or self.outflow_amount is None or any(value is None for value in coverage):
                raise ValueError("verified amount overview requires exact values and coverage")
            if (
                self.amount_total_rows == 0
                or
                self.amount_present_rows != self.amount_total_rows
                or self.amount_missing_rows != 0
                or self.amount_parse_failed_rows != 0
                or self.direction_covered_rows != self.amount_total_rows
            ):
                raise ValueError("verified amount overview coverage is incomplete")
        elif self.inflow_amount is not None or self.outflow_amount is not None:
            raise ValueError("partial or unavailable amount overview cannot publish case totals")
        return self


class StatsV2TreeReq(BaseModel):
    case_id: str = Field(min_length=1)
    tab: Literal["byName", "byCard"] = "byName"


class StatsV2TreeDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["StatsTreePublicBoundaryV1"] = "StatsTreePublicBoundaryV1"
    semantic_status: Literal["source_unavailable", "blocked"]
    fact_answer_allowed: Literal[False] = False
    blocker: str
    groups: List[dict] = Field(default_factory=list)

    @model_validator(mode="after")
    def validate_tree_public_boundary(self):
        if self.groups:
            raise ValueError("stats tree requires host evidence authority")
        return self


class StatsV2AccountTxnRowsReq(BaseModel):
    case_id: str = Field(min_length=1)
    account_key: str = Field(min_length=1)
    date_start: str = ""
    date_end: str = ""
    start_time: str = ""
    end_time: str = ""
    sort_dir: Literal["asc", "desc"] = "asc"
    limit: int = Field(default=0, ge=0, le=5000)
    cursor: Optional[Any] = None


class StatsV2AccountTxnRowsDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["StatsAccountTxnRowsPublicBoundaryV1"] = "StatsAccountTxnRowsPublicBoundaryV1"
    semantic_status: Literal["source_unavailable", "needs_input", "blocked"]
    fact_answer_allowed: Literal[False] = False
    raw_details_exposed: Literal[False] = False
    blocker: str = ""
    rows: List[dict] = Field(default_factory=list)
    done: Optional[bool] = None
    next_cursor: Optional[dict] = None

    @model_validator(mode="after")
    def validate_account_rows_public_boundary(self):
        if self.rows or self.done is True or self.next_cursor is not None:
            raise ValueError("account transaction rows require host evidence authority")
        return self


class StatsV2TxnRowsReq(BaseModel):
    case_id: str = Field(min_length=1)
    selected: List[str] = Field(default_factory=list)
    date_start: str = ""
    date_end: str = ""
    key_type: Literal["account", "name"] = "account"
    key_value: str = ""
    key_values: List[str] = Field(default_factory=list)
    filter: Literal["all", "in", "out"] = "all"
    sort_col: Literal["txn_time", "amount"] = "txn_time"
    sort_dir: Literal["asc", "desc"] = "asc"
    limit: int = Field(default=200, ge=0, le=5000)
    cursor: Optional[Any] = None
    row_format: Literal["object", "array"] = "object"
    fields: List[str] = Field(default_factory=list)


class StatsV2RowsReq(BaseModel):
    case_id: str = Field(min_length=1)
    mode: Literal["inAccount", "outAccount", "inName", "outName"] = "inAccount"
    selected: List[str] = Field(default_factory=list)
    date_start: str = ""
    date_end: str = ""
    search_text: str = ""
    row_sort_col: str = ""
    row_sort_dir: Literal["asc", "desc"] = "desc"
    row_offset: int = Field(default=0, ge=0, le=1000000)
    row_limit: int = Field(default=0, ge=0, le=5000)
    row_format: Literal["object", "array"] = "object"
    fields: List[str] = Field(default_factory=list)


class StatsV2RowsPublicBoundaryDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["StatsRowsPublicBoundaryV1"] = "StatsRowsPublicBoundaryV1"
    semantic_status: Literal["blocked"] = "blocked"
    blocker: Literal["host_evidence_receipt_required"] = "host_evidence_receipt_required"
    fact_answer_allowed: Literal[False] = False
    raw_details_exposed: Literal[False] = False
    rows: List[Any] = Field(default_factory=list)
    row_fields: List[str] = Field(default_factory=list)
    status: Literal["controlled_projection_required"] = "controlled_projection_required"
    total: Optional[int] = None
    row_summary: dict = Field(default_factory=dict)

    @model_validator(mode="after")
    def validate_rows_public_boundary(self):
        if self.rows or self.row_fields or self.total is not None or self.row_summary:
            raise ValueError("stats rows require host evidence authority")
        return self


class StatsV2TxnRowsPublicBoundaryDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["StatsTxnRowsPublicBoundaryV1"] = "StatsTxnRowsPublicBoundaryV1"
    semantic_status: Literal["blocked"] = "blocked"
    blocker: Literal["host_evidence_receipt_required"] = "host_evidence_receipt_required"
    fact_answer_allowed: Literal[False] = False
    raw_details_exposed: Literal[False] = False
    rows: List[Any] = Field(default_factory=list)
    row_fields: List[str] = Field(default_factory=list)
    done: Literal[False] = False
    next_cursor: Optional[dict] = None

    @model_validator(mode="after")
    def validate_txn_rows_public_boundary(self):
        if self.rows or self.row_fields or self.done or self.next_cursor is not None:
            raise ValueError("stats transaction rows require host evidence authority")
        return self


class StatsV2ChartFilterTokenDTO(BaseModel):
    source_panel_id: str = ""
    dimension: str = ""
    value: str = ""
    label: str = ""
    payload: dict[str, Any] = Field(default_factory=dict)


class StatsV2ChartPanelViewStateDTO(BaseModel):
    panel_id: str = ""
    view: str = ""
    metric_basis: str = ""
    dimension: str = ""
    extra: dict[str, Any] = Field(default_factory=dict)


class StatsV2ChartDashboardReq(BaseModel):
    case_id: str = Field(min_length=1)
    selected: List[str] = Field(default_factory=list)
    date_start: str = ""
    date_end: str = ""
    metric_mode: Literal["amount", "count", "counterparty"] = "amount"
    direction_mode: Literal["all", "in", "out", "net"] = "all"
    granularity: Literal["day", "week", "month", "hour"] = "day"
    success_filter: Literal["all", "success", "failed"] = "all"
    cash_filter: Literal["all", "cash", "non-cash"] = "all"
    chart_filters: List[StatsV2ChartFilterTokenDTO] = Field(default_factory=list)
    panel_views: List[StatsV2ChartPanelViewStateDTO] = Field(default_factory=list)


class StatsV2ChartDashboardDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    evidence_status: Literal["verified", "verified_no_hit", "partial", "source_unavailable", "needs_selection", "blocked"]
    answer_card_complete: bool
    fact_answer_allowed: bool
    blocker: str = ""
    coverage: "StatsV2ChartCoverageDTO"
    selection_mode: Literal["single-card", "multi-card"] = "multi-card"
    object_summary: dict[str, Any] = Field(default_factory=dict)
    summary: dict[str, Any] = Field(default_factory=dict)
    trend: dict[str, Any] = Field(default_factory=dict)
    counterparties: dict[str, Any] = Field(default_factory=dict)
    structure: dict[str, Any] = Field(default_factory=dict)
    heatmap: dict[str, Any] = Field(default_factory=dict)
    distribution: dict[str, Any] = Field(default_factory=dict)
    flow: dict[str, Any] = Field(default_factory=dict)
    anomaly: dict[str, Any] = Field(default_factory=dict)

    @model_validator(mode="after")
    def validate_dashboard_publication_state(self):
        factual_sections = (
            self.summary,
            self.trend,
            self.counterparties,
            self.structure,
            self.heatmap,
            self.distribution,
            self.flow,
            self.anomaly,
        )
        if self.fact_answer_allowed != (self.evidence_status == "verified"):
            raise ValueError("dashboard fact authority does not match evidence status")
        if self.evidence_status != "verified" and any(section for section in factual_sections):
            raise ValueError("unverified dashboard cannot publish factual sections")
        if self.evidence_status == "verified" and not self.summary:
            raise ValueError("verified dashboard requires a summary")
        return self


class StatsV2ChartCoverageDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    total_rows: Optional[int] = Field(default=None, ge=0)
    amount_present_rows: Optional[int] = Field(default=None, ge=0)
    amount_missing_rows: Optional[int] = Field(default=None, ge=0)
    amount_parse_failed_rows: Optional[int] = Field(default=None, ge=0)
    direction_covered_rows: Optional[int] = Field(default=None, ge=0)


class StatsV2ChartDetailRowsReq(BaseModel):
    case_id: str = Field(min_length=1)
    selected: List[str] = Field(default_factory=list)
    date_start: str = ""
    date_end: str = ""
    metric_mode: Literal["amount", "count", "counterparty"] = "amount"
    direction_mode: Literal["all", "in", "out", "net"] = "all"
    granularity: Literal["day", "week", "month", "hour"] = "day"
    success_filter: Literal["all", "success", "failed"] = "all"
    cash_filter: Literal["all", "cash", "non-cash"] = "all"
    chart_filters: List[StatsV2ChartFilterTokenDTO] = Field(default_factory=list)
    panel_views: List[StatsV2ChartPanelViewStateDTO] = Field(default_factory=list)
    sort_col: str = "txn_time"
    sort_dir: Literal["asc", "desc"] = "desc"
    page: int = Field(default=1, ge=1, le=1000000)
    limit: int = Field(default=200, ge=1, le=5000)
    visible_columns: List[str] = Field(default_factory=list)


class StatsV2ChartDetailRowsDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    evidence_status: Literal["verified", "verified_no_hit", "partial", "source_unavailable", "needs_selection", "blocked"]
    answer_card_complete: bool
    fact_answer_allowed: bool
    blocker: str = ""
    selection_mode: Literal["single-card", "multi-card"] = "multi-card"
    rows: List[dict] = Field(default_factory=list)
    total: Optional[int] = Field(default=None, ge=0)

    @model_validator(mode="after")
    def validate_detail_publication_state(self):
        if self.fact_answer_allowed != (self.evidence_status == "verified"):
            raise ValueError("chart detail fact authority does not match evidence status")
        if self.evidence_status != "verified" and (self.rows or self.total is not None):
            raise ValueError("unverified chart detail cannot publish rows or totals")
        if self.evidence_status == "verified" and self.total is None:
            raise ValueError("verified chart detail requires an exact total")
        return self


class StatsV2QueryJobReq(BaseModel):
    model_config = ConfigDict(extra="forbid")

    case_id: str = Field(min_length=1, max_length=128)
    query: Literal["rows", "txn-rows"] = "rows"
    request_id: str = ""
    mode: Literal["inAccount", "outAccount", "inName", "outName"] = "inAccount"
    selected: List[str] = Field(default_factory=list)
    date_start: str = ""
    date_end: str = ""
    key_type: Literal["account", "name"] = "account"
    key_value: str = ""
    filter: Literal["all", "in", "out"] = "all"
    sort_col: Literal["txn_time", "amount"] = "txn_time"
    sort_dir: str = "asc"
    limit: int = Field(default=200, ge=0, le=5000)
    cursor: Optional[Any] = None
    search_text: str = ""
    row_sort_col: str = ""
    row_sort_dir: Literal["asc", "desc"] = "desc"
    row_offset: int = Field(default=0, ge=0, le=1000000)
    row_limit: int = Field(default=0, ge=0, le=5000)


class StatsV2QueryJobDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    job_id: str
    case_id: str
    query: Literal["rows", "txn-rows"]
    request_id: str = ""
    status: Literal["queued", "running", "succeeded", "failed", "canceled"]
    progress: int = Field(ge=0, le=100)
    result_ref: None = None
    error: Optional[str] = None
    created_at: str
    updated_at: str


class StatsV2RowsResultPayload(BaseModel):
    model_config = ConfigDict(extra="forbid")

    rows: List[Any] = Field(default_factory=list)
    row_fields: List[str] = Field(default_factory=list)
    status: str = ""
    total: Optional[int] = Field(default=None, ge=0)
    row_summary: dict[str, Any] = Field(default_factory=dict)


class StatsV2TxnRowsResultPayload(BaseModel):
    model_config = ConfigDict(extra="forbid")

    rows: List[Any] = Field(default_factory=list)
    row_fields: List[str] = Field(default_factory=list)
    done: bool = False
    next_cursor: Optional[dict] = None


class StatsV2QueryResultDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    query: Literal["rows", "txn-rows"]
    request_id: str = ""
    result: Union[StatsV2RowsResultPayload, StatsV2TxnRowsResultPayload]

    @model_validator(mode="after")
    def _ensure_result_matches_query(self):
        if self.query == "rows" and not isinstance(self.result, StatsV2RowsResultPayload):
            raise ValueError("rows query result payload shape mismatch")
        if self.query == "txn-rows" and not isinstance(self.result, StatsV2TxnRowsResultPayload):
            raise ValueError("txn-rows query result payload shape mismatch")
        return self


class StatsV2AccountKeysReq(BaseModel):
    case_id: str = Field(min_length=1)
    account_keys: List[str] = Field(default_factory=list)


class StatsV2AccountDeleteInfoDTO(BaseModel):
    txn_count: int = Field(ge=0)


class StatsV2UpdateAccountInfoReq(BaseModel):
    case_id: str = Field(min_length=1)
    account_keys: List[str] = Field(default_factory=list)
    account_open_name: str = ""
    opener_id_no: str = ""


class StatsV2UpdateAccountInfoDTO(BaseModel):
    txn_count: int = Field(ge=0)
    account_count: int = Field(ge=0)


class StatsV2SetDocPendingReq(BaseModel):
    case_id: str = Field(min_length=1)
    key_type: Literal["account", "name"] = "account"
    key_value: str = Field(min_length=1)
    pending: bool = True


class StatsV2TableWidthsReq(BaseModel):
    tab: str = ""
    mode: str = ""


class StatsV2SetTableWidthsReq(BaseModel):
    tab: str = ""
    mode: str = ""
    version: str = ""
    widths: Any = Field(default_factory=list)


class StatsV2LogConfigDTO(BaseModel):
    enabled: bool = False
    leftSelection: bool = False
    tableSelection: bool = False
    transfer: bool = False
    sampleLimit: int = Field(default=8, ge=3, le=50)


class StatsV2LogDebugReq(BaseModel):
    payload: Any = Field(default_factory=dict)


class StatsV2ExportDefaultPathReq(BaseModel):
    case_id: str = Field(min_length=1)
    date: str = ""
    label: str = "统计情况"


class StatsV2ExportDefaultPathDTO(BaseModel):
    path: str


class StatsV2ExportSheet(BaseModel):
    name: str = Field(min_length=1, max_length=64)
    headers: List[str] = Field(default_factory=list)
    rows: List[List[Any]] = Field(default_factory=list)
    row_keys: List[str] = Field(default_factory=list)
    source_result_ref: Optional[dict[str, Any]] = None


class StatsV2ExportJobReq(BaseModel):
    case_id: str = Field(min_length=1)
    date: str = ""
    label: str = "统计情况"
    output_path: str = ""
    sheets: List[StatsV2ExportSheet] = Field(default_factory=list)


class StatsV2ExportJobDTO(BaseModel):
    job_id: str
    case_id: str
    status: Literal["queued", "running", "succeeded", "failed", "canceled"]
    progress: int = Field(ge=0, le=100)
    output_path: Optional[str] = None
    error: Optional[str] = None
    created_at: str
    updated_at: str
