from __future__ import annotations

from typing import Annotated, Dict, List, Literal, Optional

from pydantic import BaseModel, ConfigDict, Field


ImportFileLogView = Literal["active", "recycle"]
ImportBatchAction = Literal["recycle", "restore", "purge"]
PublicImportCount = Annotated[int, Field(ge=0, le=9_007_199_254_740_991, strict=True)]


class ImportFileLogDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    file_id: str
    kind: str = ""
    filename: str = ""
    display_path: str = ""
    stored_path: str = ""
    file_type: str = ""
    size: Optional[PublicImportCount] = None
    md5: Optional[str] = None
    sha256: Optional[str] = None
    rows_total: Optional[PublicImportCount] = None
    rows_imported: Optional[PublicImportCount] = None
    rows_imported_raw: Optional[PublicImportCount] = None
    rows_imported_norm: Optional[PublicImportCount] = None
    rows_dedup: Optional[PublicImportCount] = None
    rows_error: Optional[PublicImportCount] = None
    rows_skipped_non_data: Optional[PublicImportCount] = None
    status: str = ""
    error: Optional[str] = ""
    cleaned_status: Optional[str] = ""
    cleaned_started_at: Optional[str] = ""
    cleaned_finished_at: Optional[str] = ""
    cleaned_error: Optional[str] = ""
    cleaned_rows_affected: Optional[PublicImportCount] = None
    created_at: Optional[str] = ""
    finished_at: Optional[str] = ""
    recycled_at: Optional[str] = ""


class ImportHistoricalDatasetDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    dataset_id: str
    filename: str = ""
    kind: str = ""
    rows: Optional[PublicImportCount] = None
    cols: Optional[PublicImportCount] = None
    imported_at: Optional[str] = ""
    stored_path: str = ""


class ImportPreviewFileSpec(BaseModel):
    file_name: str = Field(min_length=1)
    source_path: str = Field(min_length=1)
    file_kind: Optional[str] = None
    password: Optional[str] = None


class ImportPreviewReq(BaseModel):
    case_id: str = Field(min_length=1)
    files: List[ImportPreviewFileSpec] = Field(min_length=1)


class ImportBatchDeleteReq(BaseModel):
    case_id: str = Field(min_length=1)
    file_ids: List[str] = Field(min_length=1)


class ImportBatchActionResultDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    ok: bool = Field(default=True)
    action: ImportBatchAction
    affected_count: PublicImportCount = 0
    file_ids: List[str] = Field(default_factory=list)


class ImportBatchDeleteResultDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    ok: bool = Field(default=True)
    deleted_count: PublicImportCount = 0
    file_ids: List[str] = Field(default_factory=list)


class ImportPreviewArchiveChildDTO(BaseModel):
    file_name: str
    archive_path: str = ""
    file_type: str = ""
    size: int = 0
    sha256: str = ""
    rows_total: int = 0
    columns_total: int = 0
    header_preview: List[str] = Field(default_factory=list)
    sample_rows: List[List[str]] = Field(default_factory=list)
    domain_category: Literal["structured", "entity", "support"] = "support"
    suggested_kind: str = ""
    suggested_kind_label: str = ""
    status: Literal["ready", "review", "unsupported"] = "ready"
    issue: str = ""
    detected_by: str = ""
    field_mapping: Dict[str, str] = Field(default_factory=dict)
    field_mapping_origins: Dict[str, str] = Field(default_factory=dict)
    mapping_status: str = ""
    mapping_method: str = ""
    mapping_message: str = ""
    mapping_required_missing: List[str] = Field(default_factory=list)


class ImportPreviewFileDTO(BaseModel):
    file_name: str
    source_path: str
    file_type: str = ""
    size: int = 0
    sha256: str = ""
    rows_total: int = 0
    columns_total: int = 0
    header_preview: List[str] = Field(default_factory=list)
    sample_rows: List[List[str]] = Field(default_factory=list)
    domain_category: Literal["structured", "entity", "support"] = "support"
    suggested_kind: str = ""
    suggested_kind_label: str = ""
    status: Literal["ready", "review", "unsupported"] = "ready"
    issue: str = ""
    accepts_password: bool = False
    requires_password: bool = False
    detected_by: str = ""
    archive_children: List[ImportPreviewArchiveChildDTO] = Field(default_factory=list)
    field_mapping: Dict[str, str] = Field(default_factory=dict)
    field_mapping_origins: Dict[str, str] = Field(default_factory=dict)
    mapping_status: str = ""
    mapping_method: str = ""
    mapping_message: str = ""
    mapping_required_missing: List[str] = Field(default_factory=list)


class AckData(BaseModel):
    ok: bool = Field(default=True)
