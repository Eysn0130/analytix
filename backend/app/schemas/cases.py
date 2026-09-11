from __future__ import annotations

from typing import Annotated, List, Literal, Optional

from pydantic import BaseModel, ConfigDict, Field


PublicCaseCount = Annotated[int, Field(ge=0, le=9_007_199_254_740_991, strict=True)]


class CaseCreateReq(BaseModel):
    case_name: str = Field(min_length=1, max_length=128)
    case_number: str = Field(min_length=1, max_length=64)
    owner: str = Field(default="", max_length=64)
    note: str = Field(default="", max_length=500)
    case_type: str = Field(default="", max_length=64)
    tags: List[str] = Field(default_factory=list, max_length=32)


class CaseUpdateReq(BaseModel):
    case_name: Optional[str] = Field(default=None, min_length=1, max_length=128)
    case_number: Optional[str] = Field(default=None, min_length=1, max_length=64)
    owner: Optional[str] = Field(default=None, max_length=64)
    note: Optional[str] = Field(default=None, max_length=500)
    case_type: Optional[str] = Field(default=None, max_length=64)
    tags: Optional[List[str]] = Field(default=None, max_length=32)
    status: Optional[str] = None


class CaseArchiveExportReq(BaseModel):
    target_dir: str = Field(default="", max_length=1024)


class CaseArchiveImportReq(BaseModel):
    archive_path: str = Field(min_length=1, max_length=1024)


class CaseArchiveExportDTO(BaseModel):
    case_id: str
    case_name: str
    archive_path: str


class PaginationMeta(BaseModel):
    page: int
    page_size: int
    total: int
    total_pages: int
    has_next: bool


class CaseListItemDTO(BaseModel):
    case_id: str
    case_name: str
    case_number: str
    owner: str
    note: str
    case_type: str
    tags: List[str]
    status: str
    is_deleted: bool
    size_label: str
    size_bytes: Optional[int]
    size_status: Literal["available", "unavailable"]
    import_health: Literal["ok", "warn", "fail", "unknown"]
    import_source_status: Literal["available", "unavailable"]
    created_at: str
    updated_at: str


class CaseStatsDTO(BaseModel):
    tasks: Optional[int] = None
    accounts: Optional[int] = None
    persons: Optional[int] = None
    tx: Optional[int] = None
    sub: Optional[int] = None


class CaseImportLogDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    title: str
    time: str
    status: Literal["ok", "warn", "fail", "unknown"]
    msg: str
    rows_total: Optional[PublicCaseCount] = None
    rows_imported: Optional[PublicCaseCount] = None
    rows_dedup: Optional[PublicCaseCount] = None
    rows_error: Optional[PublicCaseCount] = None
    rows_skipped_non_data: Optional[PublicCaseCount] = None
    error: str = ""


class CaseDetailDTO(CaseListItemDTO):
    stats: CaseStatsDTO
    stats_source_status: Literal["available", "unavailable"]
    imports: List[CaseImportLogDTO]
    imports_source_status: Literal["available", "unavailable"]


class CaseAuditDetailsDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    affected_count: Optional[int] = Field(default=None, ge=0, le=1_000_000_000)
    changed_fields: List[
        Literal["case_no", "case_type", "name", "org", "owner", "status", "summary", "tags"]
    ] = Field(default_factory=list)
    restricted_details_withheld: bool = False


class CaseAuditItemDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    event_version: Literal["case_audit_public_v1"]
    time: str
    actor: Literal["local_operator", "system"]
    action: Literal[
        "analysis.bootstrap",
        "backup_case",
        "create_case",
        "delete_case",
        "open_case",
        "purge_case",
        "purge_import_files",
        "recycle_import_files",
        "restore_case",
        "restore_import_files",
        "stats.skill_query",
        "sync_case_project_doc_metadata",
        "unclassified_event",
        "update_case",
    ]
    case_id: str
    status: Literal["recorded"]
    details: CaseAuditDetailsDTO = Field(default_factory=CaseAuditDetailsDTO)


class CaseItemData(BaseModel):
    data: CaseListItemDTO


class CaseListData(BaseModel):
    items: List[CaseListItemDTO]
    page: PaginationMeta


class CaseDTO(CaseListItemDTO):
    """Backward compatible alias for pre-P15 imports."""


class AckData(BaseModel):
    ok: bool


class ApiErrorBody(BaseModel):
    code: str
    message: str
    retryable: bool
    details: dict
