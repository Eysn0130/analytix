from __future__ import annotations

from typing import List, Literal

from pydantic import BaseModel, Field


class SubjectContextDTO(BaseModel):
    subject_id: str
    case_id: str
    display_name: str = ""
    canonical_name: str = ""
    id_no: str = ""
    id_type: str = ""
    identity_bucket: Literal["has_id", "missing_id"] = "missing_id"
    identity_label: str = ""
    merge_status: Literal["identified", "candidate"] = "candidate"
    merge_label: str = ""
    selected_for_llm: bool = False
    llm_ready: bool = False
    source_kinds: List[str] = Field(default_factory=list)
    source_labels: List[str] = Field(default_factory=list)
    source_file_ids: List[str] = Field(default_factory=list)
    source_count: int = 0
    source_file_count: int = 0
    source_row_count: int = 0
    contact_count: int = 0
    address_count: int = 0
    email_count: int = 0
    base_count: int = 0
    detail_hint: str = ""
    summary_text: str = ""
    token_estimate: int = 0
    updated_at: str = ""


class SubjectContextSourceFileDTO(BaseModel):
    file_id: str
    kind: str = ""
    filename: str = ""
    display_path: str = ""
    status: str = ""
    created_at: str = ""


class SubjectContextSourceRowDTO(BaseModel):
    source_row_key: str
    source_kind: str = ""
    source_label: str = ""
    file_id: str = ""
    file_name: str = ""
    row_no: int = 0
    content_text: str = ""


class SubjectContextDetailDTO(BaseModel):
    subject: SubjectContextDTO
    resolved_subject_id: str
    text_preview: str = ""
    source_files: List[SubjectContextSourceFileDTO] = Field(default_factory=list)
    source_rows: List[SubjectContextSourceRowDTO] = Field(default_factory=list)


class SubjectContextSelectionReq(BaseModel):
    case_id: str = Field(min_length=1)
    subject_ids: List[str] = Field(min_length=1)
    selected_for_llm: bool


class SubjectContextSelectionAck(BaseModel):
    ok: bool = True
    updated_count: int = 0


class SubjectContextDeleteAck(BaseModel):
    ok: bool = True
    deleted_count: int = 0
    file_ids: List[str] = Field(default_factory=list)
