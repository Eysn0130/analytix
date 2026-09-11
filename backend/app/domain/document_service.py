from __future__ import annotations

import hashlib
import hmac
import time
from typing import List, Optional

from app.core.case_project_doc import CASE_DIRECTION_DOC_DOCUMENT_ID, CASE_PROJECT_DOC_DOCUMENT_ID
from app.repositories.document_repository import (
    DocumentPublicMetadataUnavailableError,
    DocumentRepository,
)


class DocumentNotFoundError(KeyError):
    pass


class DocumentPublicProjectionError(RuntimeError):
    pass


class DocumentPublicProjectionUnavailableError(DocumentPublicProjectionError):
    pass


_DOCUMENT_PUBLIC_REF_PREFIX = "docref_v1_"
_DOCUMENT_PUBLIC_REF_DOMAIN = b"AnalytixDocumentPublicRefV1\x00"


def build_document_public_ref(*, case_id: str, document_id: str) -> str:
    normalized_case_id = str(case_id or "").strip()
    normalized_document_id = str(document_id or "").strip()
    if not normalized_case_id or not normalized_document_id:
        raise DocumentPublicProjectionError("document_public_projection_rejected")
    digest = hashlib.sha256()
    digest.update(_DOCUMENT_PUBLIC_REF_DOMAIN)
    for value in (normalized_case_id, normalized_document_id):
        encoded = value.encode("utf-8", errors="strict")
        digest.update(len(encoded).to_bytes(8, byteorder="big", signed=False))
        digest.update(encoded)
    # This case-bound opaque reference is a public routing projection, not an
    # authorization token. Repository access still requires exact case binding.
    return f"{_DOCUMENT_PUBLIC_REF_PREFIX}{digest.hexdigest()}"


def _project_document_asset(*, case_id: str, asset: object) -> dict:
    if not isinstance(asset, dict) or str(asset.get("case_id") or "").strip() != case_id:
        raise DocumentPublicProjectionError("document_case_binding_mismatch")
    document_id = str(asset.get("document_id") or "").strip()
    document_ref = build_document_public_ref(case_id=case_id, document_id=document_id)
    kind = str(asset.get("kind") or "").strip()
    if kind == "case_project_doc":
        document_type = "case_project_doc"
    elif kind == "support_file":
        document_type = "support_file"
    else:
        document_type = "other"
    selected_for_llm = asset.get("selected_for_llm")
    llm_ready = asset.get("llm_ready")
    if (
        (selected_for_llm is not None and type(selected_for_llm) is not bool)
        or (llm_ready is not None and type(llm_ready) is not bool)
    ):
        raise DocumentPublicProjectionError("document_public_projection_rejected")
    return {
        "contract": "DocumentPublicAssetV1",
        "document_ref": document_ref,
        "document_type": document_type,
        "selected_for_llm": selected_for_llm,
        "llm_ready": llm_ready,
        "content_access": "controlled_artifact_required",
    }


class DocumentService:
    def __init__(self, *, repository: DocumentRepository) -> None:
        self._repository = repository

    @staticmethod
    def _is_transient_case_db_conflict(exc: Exception) -> bool:
        message = str(exc or "").lower()
        return (
            "unique file handle conflict" in message
            or "already attached by database" in message
            or "same database file with a different configuration" in message
            or "could not set lock on file" in message
            or "conflicting lock is held" in message
            or "transactioncontext error" in message
            or "conflict on update" in message
        )

    def _with_case_engine_retry(self, operation):
        last_error: Exception | None = None
        for attempt in range(3):
            try:
                return operation()
            except Exception as exc:
                if not self._is_transient_case_db_conflict(exc) or attempt >= 2:
                    raise
                last_error = exc
                time.sleep(0.05 * (attempt + 1))
        if last_error is not None:
            raise last_error

    def _list_public_metadata(self, case_id: str, **kwargs) -> List[dict]:
        try:
            return self._with_case_engine_retry(
                lambda: self._repository.list_document_public_metadata(case_id, **kwargs)
            )
        except DocumentPublicMetadataUnavailableError as exc:
            raise DocumentPublicProjectionUnavailableError(
                "document_public_projection_unavailable"
            ) from exc

    def list_documents(
        self,
        *,
        case_id: str,
        selected_for_llm: Optional[bool] = None,
        llm_ready: Optional[bool] = None,
        include_system: bool = False,
    ) -> List[dict]:
        rows = self._list_public_metadata(
            case_id,
            selected_for_llm=selected_for_llm,
            llm_ready=llm_ready,
            include_system=include_system,
        )
        return [_project_document_asset(case_id=case_id, asset=row) for row in rows]

    def get_document(
        self,
        *,
        case_id: str,
        document_ref: str,
    ) -> dict:
        raw_document_id = self._resolve_document_id(
            case_id=case_id,
            document_ref=document_ref,
        )
        rows = self._list_public_metadata(case_id, include_system=True)
        asset = None
        available_document_ids: set[str] = set()
        for row in rows:
            if not isinstance(row, dict) or str(row.get("case_id") or "").strip() != case_id:
                raise DocumentPublicProjectionError("document_case_binding_mismatch")
            row_document_id = str(row.get("document_id") or "").strip()
            if not row_document_id:
                raise DocumentPublicProjectionError("document_public_projection_rejected")
            available_document_ids.add(row_document_id)
            if row_document_id == raw_document_id:
                asset = row
        if asset is None:
            raise DocumentNotFoundError(document_ref)
        projected_asset = _project_document_asset(case_id=case_id, asset=asset)
        resolved_document_id = str(asset.get("duplicate_of_document_id") or raw_document_id).strip()
        if resolved_document_id not in available_document_ids:
            raise DocumentPublicProjectionError("document_resolved_reference_unavailable")
        return {
            "contract": "DocumentPublicDetailV1",
            "document": projected_asset,
            "resolved_document_ref": build_document_public_ref(
                case_id=case_id,
                document_id=resolved_document_id,
            ),
            "content_access": "controlled_artifact_required",
        }

    def _resolve_document_id(self, *, case_id: str, document_ref: str) -> str:
        normalized_ref = str(document_ref or "").strip()
        if not normalized_ref.startswith(_DOCUMENT_PUBLIC_REF_PREFIX) or len(normalized_ref) != 74:
            raise DocumentNotFoundError(document_ref)

        system_document_ids = (CASE_PROJECT_DOC_DOCUMENT_ID, CASE_DIRECTION_DOC_DOCUMENT_ID)
        for document_id in system_document_ids:
            candidate_ref = build_document_public_ref(case_id=case_id, document_id=document_id)
            if hmac.compare_digest(candidate_ref, normalized_ref):
                return document_id

        rows = self._list_public_metadata(case_id, include_system=False)
        matched_document_id = ""
        for row in rows:
            if not isinstance(row, dict) or str(row.get("case_id") or "").strip() != case_id:
                raise DocumentPublicProjectionError("document_case_binding_mismatch")
            document_id = str(row.get("document_id") or "").strip()
            candidate_ref = build_document_public_ref(case_id=case_id, document_id=document_id)
            if hmac.compare_digest(candidate_ref, normalized_ref):
                if matched_document_id and matched_document_id != document_id:
                    raise DocumentPublicProjectionError("document_public_reference_ambiguous")
                matched_document_id = document_id
        if not matched_document_id:
            raise DocumentNotFoundError(document_ref)
        return matched_document_id
