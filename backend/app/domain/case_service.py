from __future__ import annotations

import re
from dataclasses import dataclass
from datetime import datetime
from typing import Any, Dict, List, Optional, Tuple

from app.core.import_count_semantics import (
    known_public_import_count,
    project_import_file_log_counts,
)
from app.domain.case_taxonomy import normalize_case_tags, normalize_case_type
from app.domain.import_public_projection import project_import_overview_row_public
from app.domain.controlled_artifact_gate import (
    require_controlled_artifact_publication,
    require_controlled_source_ingestion,
)
from app.repositories.case_repository import CaseRepository


class CaseNotFoundError(KeyError):
    pass


class CaseDeletedError(RuntimeError):
    pass


class CaseNotInBinError(RuntimeError):
    pass


class CaseArchiveInvalidError(ValueError):
    pass


class CaseEffectAuthorityUnavailableError(RuntimeError):
    """The trusted host binding needed for a case effect was not supplied."""


class CaseEffectBindingRejectedError(RuntimeError):
    """The request binding is incomplete or differs from trusted host state."""


_CASE_ID_PATTERN = re.compile(r"^[A-Za-z0-9_-]{4,80}$")
_SHA256_PATTERN = re.compile(r"^[a-f0-9]{64}$")
_DATASET_SNAPSHOT_ID_PATTERN = re.compile(r"^dsv1_[a-f0-9]{64}$")


@dataclass(frozen=True)
class CaseEffectBindingV1:
    case_id: str
    context_digest: str
    context_epoch: int
    dataset_snapshot_id: str
    execution_grant_id: str


def validate_unwired_case_effect_binding_v1(
    *,
    trusted_expected: Optional[CaseEffectBindingV1],
    observed: Optional[CaseEffectBindingV1],
) -> None:
    """Validate exact case-effect equality without issuing authority.

    This is deliberately an unwired validation primitive. The Python data
    plane cannot mint or discover the Go runtime's authoritative turn context,
    dataset snapshot, or execution grant. A production caller must supply
    ``trusted_expected`` from that host authority before this function can be
    used as an effect gate; caller-controlled values on both sides are not
    authority.
    """

    if trusted_expected is None or not _case_effect_binding_is_canonical(trusted_expected):
        raise CaseEffectAuthorityUnavailableError("case_effect_authority_unavailable")
    if observed is None or not _case_effect_binding_is_canonical(observed):
        raise CaseEffectBindingRejectedError("case_effect_binding_incomplete")
    if observed != trusted_expected:
        raise CaseEffectBindingRejectedError("case_effect_binding_mismatch")


def _case_effect_binding_is_canonical(binding: CaseEffectBindingV1) -> bool:
    return (
        _CASE_ID_PATTERN.fullmatch(binding.case_id) is not None
        and _SHA256_PATTERN.fullmatch(binding.context_digest) is not None
        and isinstance(binding.context_epoch, int)
        and not isinstance(binding.context_epoch, bool)
        and binding.context_epoch > 0
        and _DATASET_SNAPSHOT_ID_PATTERN.fullmatch(binding.dataset_snapshot_id) is not None
        and _SHA256_PATTERN.fullmatch(binding.execution_grant_id) is not None
    )


def _storage_status_to_api(status: str) -> str:
    raw = (status or "").strip()
    if "归档" in raw or "结案" in raw:
        return "archived"
    return "active"


def _api_status_to_storage(status: Optional[str]) -> Optional[str]:
    if status is None:
        return None
    if status == "archived":
        return "已归档"
    return "进行中"


def _to_iso8601(value: str) -> str:
    raw = (value or "").strip()
    if not raw:
        return ""

    try:
        dt = datetime.strptime(raw, "%Y-%m-%d %H:%M:%S")
        return dt.isoformat()
    except Exception:
        pass

    try:
        dt = datetime.fromisoformat(raw)
        return dt.isoformat()
    except Exception:
        return raw


def _to_tags(value: Any) -> List[str]:
    return normalize_case_tags(value)


def _verified_case_stat(stats: Any, field_name: str) -> Optional[int]:
    count_status = getattr(stats, "count_status", None)
    if not isinstance(count_status, dict) or count_status.get(field_name) != "verified":
        return None
    value = getattr(stats, field_name, None)
    if type(value) is not int or value < 0:
        return None
    return value


def _import_health(status: str) -> str:
    raw = (status or "").strip().lower()
    if not raw:
        return "unknown"
    if "fail" in raw or "失败" in raw or "error" in raw:
        return "fail"
    if "warn" in raw or "警告" in raw or "异常" in raw:
        return "warn"
    if "ok" in raw or "成功" in raw or "完成" in raw or raw in {"succeeded", "completed"}:
        return "ok"
    return "unknown"


def _size_label(size_bytes: int) -> str:
    if size_bytes <= 0:
        return "0 MB"
    size_mb = round(size_bytes / (1024 * 1024))
    if size_mb == 0:
        size_kb = max(1, round(size_bytes / 1024))
        return f"{size_kb} KB"
    return f"{size_mb} MB"


class CaseService:
    """Case domain service with explicit case identity for every read/write."""

    def __init__(self, repository: CaseRepository) -> None:
        self._repository = repository

    def _require_case(self, case_id: str) -> Any:
        case = self._repository.get_case(case_id)
        if case is None:
            raise CaseNotFoundError(case_id)
        return case

    def is_case_deleted(self, case_id: str) -> bool:
        case = self._require_case(case_id)
        return bool(case.deleted_at)

    def _read_import_overview(
        self,
        case_id: str,
        *,
        limit: int,
    ) -> Tuple[dict, str]:
        try:
            overview = self._repository.get_case_import_overview(case_id, limit=max(1, limit))
        except Exception:
            return {}, "unavailable"
        if not isinstance(overview, dict) or overview.get("source_status") != "available":
            return {}, "unavailable"
        return overview, "available"

    def _to_base_dto(
        self,
        case: Any,
        *,
        import_limit: int = 1,
        overview_state: Optional[Tuple[dict, str]] = None,
    ) -> Dict[str, Any]:
        size_bytes: Optional[int] = None
        size_status = "unavailable"
        try:
            observed_size = self._repository.case_size(case.case_id)
            if type(observed_size) is not int or observed_size < 0:
                raise ValueError("case_size_invalid")
            size_bytes = observed_size
            size_status = "available"
        except Exception:
            size_bytes = None

        overview, import_source_status = overview_state or self._read_import_overview(
            case.case_id,
            limit=import_limit,
        )
        recent = overview.get("recent") if isinstance(overview, dict) else []
        recent = recent if isinstance(recent, list) else []
        latest_status = ""
        if recent:
            latest_status = str((recent[0] or {}).get("status") or "")

        return {
            "case_id": case.case_id,
            "case_name": case.name or case.case_id,
            "case_number": case.case_no or "",
            "owner": case.owner or "",
            "note": case.summary or "",
            "case_type": normalize_case_type(case.case_type),
            "tags": normalize_case_tags(case.tags),
            "status": _storage_status_to_api(case.status),
            "is_deleted": bool(case.deleted_at),
            "size_label": _size_label(size_bytes) if size_bytes is not None else "unknown",
            "size_bytes": size_bytes,
            "size_status": size_status,
            "import_health": _import_health(latest_status),
            "import_source_status": import_source_status,
            "created_at": _to_iso8601(case.created_at),
            "updated_at": _to_iso8601(case.updated_at),
        }

    def _to_import_logs(
        self,
        case_id: str,
        *,
        limit: int = 5,
        overview_state: Optional[Tuple[dict, str]] = None,
    ) -> List[Dict[str, Any]]:
        overview, source_status = overview_state or self._read_import_overview(case_id, limit=limit)
        if source_status != "available":
            return []
        recent = overview.get("recent") if isinstance(overview, dict) else []
        if not isinstance(recent, list):
            return []

        items: List[Dict[str, Any]] = []
        for row in recent:
            payload = project_import_file_log_counts(row if isinstance(row, dict) else {})
            projected = project_import_overview_row_public(row=payload)
            rows_total = known_public_import_count(projected.get("rows_total"))
            rows_imported = known_public_import_count(projected.get("rows_imported"))
            rows_dedup = known_public_import_count(projected.get("rows_dedup"))
            rows_error = known_public_import_count(projected.get("rows_error"))
            rows_skipped_non_data = known_public_import_count(
                projected.get("rows_skipped_non_data")
            )
            msg = "导入数量未知" if rows_imported is None else f"导入 {rows_imported} 条"
            if rows_dedup is not None and rows_dedup > 0:
                msg += f" · 去重 {rows_dedup} 条"
            if rows_error is not None and rows_error > 0:
                msg += f" · 错误 {rows_error} 条"
            items.append(
                {
                    "title": projected["title"],
                    "time": projected["time"],
                    "status": _import_health(projected["status"]),
                    "msg": msg,
                    "rows_total": rows_total,
                    "rows_imported": rows_imported,
                    "rows_dedup": rows_dedup,
                    "rows_error": rows_error,
                    "rows_skipped_non_data": rows_skipped_non_data,
                    "error": projected["error"],
                }
            )
        return items

    def _to_stats(self, case_id: str) -> Dict[str, Optional[int]]:
        return self._read_stats(case_id)[0]

    def _read_stats(self, case_id: str) -> Tuple[Dict[str, Optional[int]], str]:
        try:
            stats = self._repository.get_case_stats(case_id)
        except Exception:
            return ({
                "tasks": None,
                "accounts": None,
                "persons": None,
                "tx": None,
                "sub": None,
            }, "unavailable")

        projected = {
            "tasks": _verified_case_stat(stats, "tasks"),
            "accounts": _verified_case_stat(stats, "accounts"),
            "persons": _verified_case_stat(stats, "persons"),
            "tx": _verified_case_stat(stats, "transactions"),
            "sub": None,
        }
        source_status = (
            "available"
            if any(projected[field] is not None for field in ("tasks", "accounts", "persons", "tx"))
            else "unavailable"
        )
        return projected, source_status

    def list_cases(
        self,
        *,
        page: int,
        page_size: int,
        keyword: str,
        include_deleted: bool,
    ) -> Tuple[List[Dict[str, Any]], int]:
        cases = self._repository.list_cases(include_deleted=True)

        if not include_deleted:
            cases = [case for case in cases if not case.deleted_at]

        keyword_norm = (keyword or "").strip().lower()
        if keyword_norm:

            def _hit(case: Any) -> bool:
                tags = normalize_case_tags(getattr(case, "tags", []))
                raw_case_type = str(getattr(case, "case_type", "") or "").strip()
                normalized_case_type = normalize_case_type(raw_case_type)
                haystack = [
                    case.case_id,
                    case.name,
                    case.case_no,
                    case.owner,
                    case.summary,
                    raw_case_type,
                    normalized_case_type,
                    " ".join(tags),
                ]
                return any(keyword_norm in (str(item or "").lower()) for item in haystack)

            cases = [case for case in cases if _hit(case)]

        total = len(cases)
        start = (page - 1) * page_size
        end = start + page_size
        page_items = cases[start:end]

        return [self._to_base_dto(case, import_limit=1) for case in page_items], total

    def get_case(self, case_id: str) -> Dict[str, Any]:
        case = self._require_case(case_id)
        overview_state = self._read_import_overview(case_id, limit=5)
        stats, stats_source_status = self._read_stats(case_id)
        payload = self._to_base_dto(case, import_limit=5, overview_state=overview_state)
        payload["stats"] = stats
        payload["stats_source_status"] = stats_source_status
        payload["imports"] = self._to_import_logs(
            case_id,
            limit=5,
            overview_state=overview_state,
        )
        payload["imports_source_status"] = overview_state[1]
        return payload

    def list_case_audit(self, case_id: str, *, limit: int = 50) -> List[Dict[str, Any]]:
        _ = self._require_case(case_id)
        rows = self._repository.list_case_audit(case_id, limit=limit)
        items: List[Dict[str, Any]] = []
        for row in rows:
            payload = row if isinstance(row, dict) else {}
            items.append(
                {
                    "event_version": str(payload.get("event_version") or ""),
                    "time": str(payload.get("time") or ""),
                    "actor": str(payload.get("actor") or ""),
                    "action": str(payload.get("action") or ""),
                    "case_id": str(payload.get("case_id") or case_id),
                    "status": str(payload.get("status") or ""),
                    "details": payload.get("details") if isinstance(payload.get("details"), dict) else {},
                }
            )
        return items

    def create_case(
        self,
        *,
        case_name: str,
        case_number: str,
        owner: str,
        note: str,
        case_type: str,
        tags: List[str],
    ) -> Dict[str, Any]:
        normalized_case_type = normalize_case_type(case_type)
        normalized_tags = normalize_case_tags(tags)
        created = self._repository.create_case(
            case_name=case_name,
            case_number=case_number,
            owner=owner,
            note=note,
            status=_api_status_to_storage("active") or "进行中",
            case_type=normalized_case_type,
            tags=normalized_tags,
        )
        return self._to_base_dto(created)

    def update_case(self, case_id: str, updates: Dict[str, Any]) -> Dict[str, Any]:
        current = self._require_case(case_id)

        mapped: Dict[str, Any] = {}
        if "case_name" in updates and updates["case_name"] is not None:
            mapped["name"] = updates["case_name"]
        if "case_number" in updates and updates["case_number"] is not None:
            mapped["case_no"] = updates["case_number"]
        if "owner" in updates and updates["owner"] is not None:
            mapped["owner"] = updates["owner"]
        if "note" in updates and updates["note"] is not None:
            mapped["summary"] = updates["note"]
        if "case_type" in updates and updates["case_type"] is not None:
            mapped["case_type"] = normalize_case_type(updates["case_type"])
        if "tags" in updates and updates["tags"] is not None:
            mapped["tags"] = normalize_case_tags(updates["tags"])
        if "status" in updates and updates["status"] is not None:
            mapped["status"] = _api_status_to_storage(updates["status"]) or current.status

        if mapped:
            updated = self._repository.update_case(case_id, **mapped)
        else:
            updated = current

        return self._to_base_dto(updated)

    def delete_case(self, case_id: str) -> None:
        _ = self._require_case(case_id)
        self._repository.soft_delete_case(case_id)

    def purge_case(self, case_id: str) -> None:
        case = self._require_case(case_id)
        if not case.deleted_at:
            raise CaseNotInBinError(case_id)
        self._repository.purge_case(case_id)

    def restore_case(self, case_id: str) -> Dict[str, Any]:
        _ = self._require_case(case_id)
        self._repository.restore_case(case_id)
        restored = self._require_case(case_id)
        return self._to_base_dto(restored)

    def export_case_archive(self, case_id: str, *, target_dir: str = "") -> Dict[str, Any]:
        require_controlled_artifact_publication()
        case = self._require_case(case_id)
        archive_path = self._repository.export_case_archive(case_id, target_dir=target_dir)
        return {
            "case_id": case.case_id,
            "case_name": case.name or case.case_id,
            "archive_path": archive_path,
        }

    def import_case_archive(self, archive_path: str) -> Dict[str, Any]:
        require_controlled_source_ingestion()
        try:
            imported = self._repository.import_case_archive(archive_path)
        except ValueError as exc:
            raise CaseArchiveInvalidError(str(exc)) from exc
        return self._to_base_dto(imported)

    def activate_case(self, case_id: str) -> Dict[str, Any]:
        """Record UI recency without creating process-global fact authority."""

        case = self._require_case(case_id)
        if case.deleted_at:
            raise CaseDeletedError(case_id)

        self._repository.record_case_opened(case_id)

        reopened = self._require_case(case_id)
        return self._to_base_dto(reopened)

    def get_explicit_case_selection(self, case_id: str) -> Dict[str, Any]:
        """Resolve only the caller's explicit case selection."""

        explicit_case_id = str(case_id or "").strip()
        if not explicit_case_id:
            raise CaseNotFoundError("explicit_case_id")
        case = self._require_case(explicit_case_id)
        if case.deleted_at:
            raise CaseDeletedError(explicit_case_id)

        return self._to_base_dto(case)
