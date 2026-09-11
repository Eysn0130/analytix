from __future__ import annotations

import hashlib
import re
from datetime import datetime, timezone
from typing import Any, Mapping, Sequence

from app.core.failure_boundary import (
    project_cleaning_error,
    project_import_error,
)


_IMPORT_FILE_REF_RE = re.compile(r"^importfile_v1_[a-f0-9]{64}$")
_IMPORT_DATASET_REF_RE = re.compile(r"^importdataset_v1_[a-f0-9]{64}$")
_SAFE_TOKEN_RE = re.compile(r"^[a-z][a-z0-9_]{0,63}$")
_SAFE_FILE_TYPE_RE = re.compile(r"^[a-z0-9]{1,16}$")
_LOWER_MD5_RE = re.compile(r"^[a-f0-9]{32}$")
_LOWER_SHA256_RE = re.compile(r"^[a-f0-9]{64}$")
_MAX_SAFE_INTEGER = 9_007_199_254_740_991
_PUBLIC_SOURCE_NAME = "Imported source"

_IMPORT_STATUS = {
    "queued": "queued",
    "waiting": "queued",
    "等待中": "queued",
    "running": "running",
    "导入中": "running",
    "succeeded": "succeeded",
    "done": "succeeded",
    "completed": "succeeded",
    "已完成": "succeeded",
    "failed": "failed",
    "失败": "failed",
    "canceled": "canceled",
    "cancelled": "canceled",
    "已取消": "canceled",
}
_CLEANING_STATUS = {
    "queued": "queued",
    "running": "running",
    "succeeded": "succeeded",
    "done": "succeeded",
    "completed": "succeeded",
    "failed": "failed",
    "canceled": "canceled",
    "cancelled": "canceled",
}


class ImportPublicProjectionError(ValueError):
    pass


def _exact_text(value: Any, *, max_bytes: int = 512) -> str:
    if type(value) is not str or not value or value != value.strip():
        raise ImportPublicProjectionError("import_public_text_invalid")
    if len(value.encode("utf-8", errors="strict")) > max_bytes:
        raise ImportPublicProjectionError("import_public_text_invalid")
    return value


def _case_id(value: Any) -> str:
    case_id = _exact_text(value, max_bytes=80)
    if re.fullmatch(r"[A-Za-z0-9_-]{4,80}", case_id) is None:
        raise ImportPublicProjectionError("import_public_case_invalid")
    return case_id


def _opaque_ref(*, domain: bytes, prefix: str, case_id: Any, source_id: Any) -> str:
    bound_case_id = _case_id(case_id)
    source = _exact_text(source_id, max_bytes=512)
    digest = hashlib.sha256()
    digest.update(domain)
    for value in (bound_case_id, source):
        encoded = value.encode("utf-8", errors="strict")
        digest.update(len(encoded).to_bytes(8, byteorder="big", signed=False))
        digest.update(encoded)
    return f"{prefix}_{digest.hexdigest()}"


def public_import_file_ref(*, case_id: Any, source_file_id: Any) -> str:
    return _opaque_ref(
        domain=b"AnalytixImportFilePublicRefV1\x00",
        prefix="importfile_v1",
        case_id=case_id,
        source_id=source_file_id,
    )


def public_import_dataset_ref(*, case_id: Any, source_dataset_id: Any) -> str:
    return _opaque_ref(
        domain=b"AnalytixImportDatasetPublicRefV1\x00",
        prefix="importdataset_v1",
        case_id=case_id,
        source_id=source_dataset_id,
    )


def is_public_import_file_ref(value: Any) -> bool:
    return type(value) is str and _IMPORT_FILE_REF_RE.fullmatch(value) is not None


def is_public_import_dataset_ref(value: Any) -> bool:
    return type(value) is str and _IMPORT_DATASET_REF_RE.fullmatch(value) is not None


def _optional_count(value: Any) -> int | None:
    if value is None:
        return None
    if type(value) is not int or value < 0 or value > _MAX_SAFE_INTEGER:
        return None
    return value


def _strict_optional_count(value: Any, *, field: str) -> int | None:
    if value is None:
        return None
    projected = _optional_count(value)
    if projected is None:
        raise ImportPublicProjectionError(f"import_public_{field}_invalid")
    return projected


def _safe_token(value: Any) -> str:
    if type(value) is not str or value != value.strip():
        return "unknown"
    return value if _SAFE_TOKEN_RE.fullmatch(value) is not None else "unknown"


def _safe_file_type(value: Any) -> str:
    if type(value) is not str or value != value.strip():
        return "unknown"
    normalized = value.lower()
    return normalized if _SAFE_FILE_TYPE_RE.fullmatch(normalized) is not None else "unknown"


def _safe_hash(value: Any, pattern: re.Pattern[str]) -> str | None:
    if value in (None, ""):
        return None
    return value if type(value) is str and pattern.fullmatch(value) is not None else None


def _safe_timestamp(value: Any) -> str | None:
    if value in (None, ""):
        return None
    if type(value) is not str or value != value.strip() or len(value) > 64:
        return None
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc).isoformat()


def _status(value: Any, vocabulary: Mapping[str, str]) -> str:
    if type(value) is not str or value != value.strip():
        return "unknown"
    return vocabulary.get(value.lower(), vocabulary.get(value, "unknown"))


def project_import_file_log_public(*, case_id: Any, row: Mapping[str, Any]) -> dict[str, Any]:
    bound_case_id = _case_id(case_id)
    return {
        "file_id": public_import_file_ref(case_id=bound_case_id, source_file_id=row.get("file_id")),
        "kind": _safe_token(row.get("kind")),
        "filename": _PUBLIC_SOURCE_NAME,
        "display_path": "",
        "stored_path": "",
        "file_type": _safe_file_type(row.get("file_type")),
        "size": _optional_count(row.get("size")),
        "md5": _safe_hash(row.get("md5"), _LOWER_MD5_RE),
        "sha256": _safe_hash(row.get("sha256"), _LOWER_SHA256_RE),
        "rows_total": _optional_count(row.get("rows_total")),
        "rows_imported": _optional_count(row.get("rows_imported")),
        "rows_imported_raw": _optional_count(row.get("rows_imported_raw")),
        "rows_imported_norm": _optional_count(row.get("rows_imported_norm")),
        "rows_dedup": _optional_count(row.get("rows_dedup")),
        "rows_error": _optional_count(row.get("rows_error")),
        "rows_skipped_non_data": _optional_count(row.get("rows_skipped_non_data")),
        "status": _status(row.get("status"), _IMPORT_STATUS),
        "error": project_import_error(row.get("error"), status=row.get("status")),
        "cleaned_status": _status(row.get("cleaned_status"), _CLEANING_STATUS),
        "cleaned_started_at": _safe_timestamp(row.get("cleaned_started_at")),
        "cleaned_finished_at": _safe_timestamp(row.get("cleaned_finished_at")),
        "cleaned_error": project_cleaning_error(
            row.get("cleaned_error"),
            status=row.get("cleaned_status"),
        ),
        "cleaned_rows_affected": _optional_count(row.get("cleaned_rows_affected")),
        "created_at": _safe_timestamp(row.get("created_at")),
        "finished_at": _safe_timestamp(row.get("finished_at")),
        "recycled_at": _safe_timestamp(row.get("recycled_at")),
    }


def project_import_historical_dataset_public(*, case_id: Any, row: Mapping[str, Any]) -> dict[str, Any]:
    bound_case_id = _case_id(case_id)
    return {
        "dataset_id": public_import_dataset_ref(
            case_id=bound_case_id,
            source_dataset_id=row.get("dataset_id"),
        ),
        "filename": _PUBLIC_SOURCE_NAME,
        "kind": _safe_token(row.get("kind")),
        "rows": _strict_optional_count(row.get("rows"), field="rows"),
        "cols": _strict_optional_count(row.get("cols"), field="cols"),
        "imported_at": _safe_timestamp(row.get("imported_at")),
        "stored_path": "",
    }


def project_import_overview_row_public(*, row: Mapping[str, Any]) -> dict[str, Any]:
    return {
        "title": _PUBLIC_SOURCE_NAME,
        "time": _safe_timestamp(row.get("finished_at") or row.get("created_at")) or "",
        "status": _status(row.get("status"), _IMPORT_STATUS),
        "rows_total": _optional_count(row.get("rows_total")),
        "rows_imported": _optional_count(row.get("rows_imported")),
        "rows_dedup": _optional_count(row.get("rows_dedup")),
        "rows_error": _optional_count(row.get("rows_error")),
        "rows_skipped_non_data": _optional_count(row.get("rows_skipped_non_data")),
        "error": project_import_error(row.get("error"), status=row.get("status")),
    }


def resolve_public_import_file_refs(
    *,
    case_id: Any,
    requested_refs: Sequence[Any],
    source_rows: Sequence[Mapping[str, Any]],
) -> list[str]:
    bound_case_id = _case_id(case_id)
    refs = list(requested_refs)
    if not refs or any(not is_public_import_file_ref(value) for value in refs):
        raise ImportPublicProjectionError("import_public_file_ref_invalid")
    if len(set(refs)) != len(refs):
        raise ImportPublicProjectionError("import_public_file_ref_invalid")
    source_by_ref: dict[str, str] = {}
    for row in source_rows:
        if not isinstance(row, Mapping):
            continue
        source_id = row.get("file_id")
        try:
            public_ref = public_import_file_ref(case_id=bound_case_id, source_file_id=source_id)
            exact_source_id = _exact_text(source_id, max_bytes=512)
        except ImportPublicProjectionError:
            continue
        if public_ref in source_by_ref and source_by_ref[public_ref] != exact_source_id:
            raise ImportPublicProjectionError("import_public_file_ref_collision")
        source_by_ref[public_ref] = exact_source_id
    if any(value not in source_by_ref for value in refs):
        raise ImportPublicProjectionError("import_public_file_ref_unknown")
    return [source_by_ref[value] for value in refs]


__all__ = [
    "ImportPublicProjectionError",
    "is_public_import_dataset_ref",
    "is_public_import_file_ref",
    "project_import_file_log_public",
    "project_import_historical_dataset_public",
    "project_import_overview_row_public",
    "public_import_dataset_ref",
    "public_import_file_ref",
    "resolve_public_import_file_refs",
]
