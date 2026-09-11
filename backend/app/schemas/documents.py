from __future__ import annotations

from typing import Literal, Optional

from pydantic import BaseModel, ConfigDict, Field


class DocumentAssetDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["DocumentPublicAssetV1"] = "DocumentPublicAssetV1"
    document_ref: str = Field(pattern=r"^docref_v1_[a-f0-9]{64}$")
    document_type: Literal["case_project_doc", "support_file", "other"] = "other"
    selected_for_llm: Optional[bool]
    llm_ready: Optional[bool]
    content_access: Literal["controlled_artifact_required"] = "controlled_artifact_required"


class DocumentDetailDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["DocumentPublicDetailV1"] = "DocumentPublicDetailV1"
    document: DocumentAssetDTO
    resolved_document_ref: str = Field(pattern=r"^docref_v1_[a-f0-9]{64}$")
    content_access: Literal["controlled_artifact_required"] = "controlled_artifact_required"
