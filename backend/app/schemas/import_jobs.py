from __future__ import annotations

from typing import Annotated, List, Optional

from pydantic import BaseModel, ConfigDict, Field


PublicCount = Annotated[int, Field(ge=0, le=9_007_199_254_740_991, strict=True)]


class ImportArchiveItemSpec(BaseModel):
    model_config = ConfigDict(extra="forbid")

    archive_path: str = Field(min_length=1)
    file_kind: Optional[str] = None
    expected_sha256: str = Field(pattern=r"^[0-9A-Fa-f]{64}$")
    expected_size: int = Field(ge=0)
    field_mapping: Optional[dict[str, str]] = None
    field_mapping_origins: Optional[dict[str, str]] = None


class ImportFileSpec(BaseModel):
    model_config = ConfigDict(extra="forbid")

    file_name: str = Field(min_length=1)
    source_path: str = Field(min_length=1)
    file_kind: Optional[str] = None
    password: Optional[str] = None
    expected_sha256: Optional[str] = None
    expected_size: Optional[int] = Field(default=None, ge=0)
    field_mapping: Optional[dict[str, str]] = None
    field_mapping_origins: Optional[dict[str, str]] = None
    archive_items: Optional[List[ImportArchiveItemSpec]] = None


class ImportJobReq(BaseModel):
    model_config = ConfigDict(extra="forbid")

    case_id: str = Field(min_length=1)
    files: List[ImportFileSpec] = Field(min_length=1)
    auto_cleaning: bool = False


class ImportJobFileDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    file_id: str
    display_name: str
    display_path: str = ""
    file_type: str = ""
    size: Optional[PublicCount] = None
    md5: Optional[str] = None
    sha256: Optional[str] = None
    source_sha256: Optional[str] = None
    source_size: Optional[PublicCount] = None
    kind: str = ""
    status: str = ""
    rows_total: Optional[PublicCount] = None
    rows_seen: Optional[PublicCount] = None
    rows_imported_raw: Optional[PublicCount] = None
    rows_imported_norm: Optional[PublicCount] = None
    rows_dedup: Optional[PublicCount] = None
    rows_error: Optional[PublicCount] = None
    rows_skipped_non_data: Optional[PublicCount] = None
    note: str = ""
    error: str = ""
    attempts: Optional[PublicCount] = None


class ImportJobSummaryDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    total_files: Optional[PublicCount] = None
    rows_total: Optional[PublicCount] = None
    rows_seen: Optional[PublicCount] = None
    rows_imported_raw: Optional[PublicCount] = None
    rows_imported_norm: Optional[PublicCount] = None
    rows_dedup: Optional[PublicCount] = None
    rows_error: Optional[PublicCount] = None
    rows_skipped_non_data: Optional[PublicCount] = None
    retry_count: Optional[PublicCount] = None


class ImportJobDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    job_id: str
    case_id: str
    status: str
    progress: int = Field(ge=0, le=100, strict=True)
    imported_files: Optional[PublicCount] = None
    summary: ImportJobSummaryDTO = Field(default_factory=ImportJobSummaryDTO)
    files: List[ImportJobFileDTO] = Field(default_factory=list)
    current_file: str = ""
    error: Optional[str] = None
    created_at: str
    updated_at: str
