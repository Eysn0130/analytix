from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any
from uuid import UUID

from app.domain.controlled_artifact_gate import require_controlled_artifact_publication
from app.utils.fs import atomic_create_private_text, read_private_text


_REQUEST_VERSION = 1
_REQUEST_TYPE = "stats_export_request"
_REQUEST_MAX_BYTES = 64 * 1024 * 1024
_REQUEST_DOMAIN = b"AnalytixStatsExportRequestV1\x00"


class StatsExportRequestUnavailableError(RuntimeError):
    pass


class StatsExportRequestStore:
    def __init__(self, *, request_dir: Path) -> None:
        self._request_dir = Path(request_dir)

    def persist(self, *, job_id: str, request: dict) -> dict:
        require_controlled_artifact_publication()
        normalized_job_id = self._canonical_job_id(job_id)
        data = request if isinstance(request, dict) else {}
        case_id = str(data.get("case_id") or "").strip()
        if not case_id or case_id == "active":
            raise StatsExportRequestUnavailableError("stats_export_request_case_invalid")
        body = {
            "version": _REQUEST_VERSION,
            "type": _REQUEST_TYPE,
            "job_id": normalized_job_id,
            "case_id": case_id,
            "request": data,
        }
        body_text = json.dumps(body, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
        request_sha256 = hashlib.sha256(_REQUEST_DOMAIN + body_text.encode("utf-8")).hexdigest()
        text = json.dumps(
            {**body, "request_sha256": request_sha256},
            ensure_ascii=False,
            sort_keys=True,
            separators=(",", ":"),
        )
        if len(text.encode("utf-8")) > _REQUEST_MAX_BYTES:
            raise StatsExportRequestUnavailableError("stats_export_request_too_large")
        out_path = self._request_file(normalized_job_id)
        try:
            created = atomic_create_private_text(out_path, text)
            observed = read_private_text(out_path, max_bytes=_REQUEST_MAX_BYTES)
        except OSError:
            raise StatsExportRequestUnavailableError("stats_export_request_unavailable") from None
        if observed != text:
            code = "stats_export_request_conflict" if not created else "stats_export_request_invalid"
            raise StatsExportRequestUnavailableError(code)
        return {
            "version": _REQUEST_VERSION,
            "type": _REQUEST_TYPE,
            "job_id": normalized_job_id,
            "case_id": case_id,
            "request_sha256": request_sha256,
        }

    def load(self, *, job_id: str, request_ref: Any) -> dict:
        normalized_job_id = self._canonical_job_id(job_id)
        ref = request_ref if isinstance(request_ref, dict) else {}
        if set(ref) != {"version", "type", "job_id", "case_id", "request_sha256"}:
            raise StatsExportRequestUnavailableError(normalized_job_id)
        case_id = str(ref.get("case_id") or "").strip()
        request_sha256 = str(ref.get("request_sha256") or "").strip()
        if (
            ref.get("version") != _REQUEST_VERSION
            or ref.get("type") != _REQUEST_TYPE
            or str(ref.get("job_id") or "") != normalized_job_id
            or not case_id
            or case_id == "active"
            or len(request_sha256) != 64
            or any(character not in "0123456789abcdef" for character in request_sha256)
        ):
            raise StatsExportRequestUnavailableError(normalized_job_id)
        try:
            text = read_private_text(self._request_file(normalized_job_id), max_bytes=_REQUEST_MAX_BYTES)
            payload = json.loads(text)
        except (OSError, json.JSONDecodeError):
            raise StatsExportRequestUnavailableError(normalized_job_id) from None
        if not isinstance(payload, dict) or set(payload) != {
            "version",
            "type",
            "job_id",
            "case_id",
            "request",
            "request_sha256",
        }:
            raise StatsExportRequestUnavailableError(normalized_job_id)
        request = payload.get("request") if isinstance(payload.get("request"), dict) else None
        if (
            request is None
            or payload.get("version") != _REQUEST_VERSION
            or payload.get("type") != _REQUEST_TYPE
            or str(payload.get("job_id") or "") != normalized_job_id
            or str(payload.get("case_id") or "") != case_id
            or str(payload.get("request_sha256") or "") != request_sha256
            or str(request.get("case_id") or "").strip() != case_id
        ):
            raise StatsExportRequestUnavailableError(normalized_job_id)
        body = {
            "version": _REQUEST_VERSION,
            "type": _REQUEST_TYPE,
            "job_id": normalized_job_id,
            "case_id": case_id,
            "request": request,
        }
        body_text = json.dumps(body, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
        expected_sha256 = hashlib.sha256(_REQUEST_DOMAIN + body_text.encode("utf-8")).hexdigest()
        expected_text = json.dumps(
            {**body, "request_sha256": expected_sha256},
            ensure_ascii=False,
            sort_keys=True,
            separators=(",", ":"),
        )
        if request_sha256 != expected_sha256 or text != expected_text:
            raise StatsExportRequestUnavailableError(normalized_job_id)
        return json.loads(json.dumps(request, ensure_ascii=False))

    def delete_for_job(self, *, job_id: str) -> None:
        normalized_job_id = self._canonical_job_id(job_id)
        root = self._request_dir
        if not root.exists():
            return
        try:
            if root.is_symlink() or root.resolve(strict=True) != root.absolute():
                raise OSError
            self._request_file(normalized_job_id).unlink(missing_ok=True)
        except OSError:
            raise StatsExportRequestUnavailableError(normalized_job_id) from None

    def _request_file(self, job_id: str) -> Path:
        return self._request_dir / f"{self._canonical_job_id(job_id)}.json"

    @staticmethod
    def _canonical_job_id(job_id: str) -> str:
        value = str(job_id or "").strip()
        try:
            if str(UUID(value)) != value:
                raise ValueError
        except (AttributeError, TypeError, ValueError):
            raise StatsExportRequestUnavailableError("stats_export_request_job_invalid") from None
        return value
