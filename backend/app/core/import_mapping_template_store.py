from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Dict, Optional, Sequence

from app.core.fc_import_projection import normalize_header_for_match
from app.core.fc_import_schema import FC_SCHEMAS


_TEMPLATE_STORE_VERSION = 1


@dataclass(frozen=True)
class ImportMappingTemplate:
    template_id: str
    kind: str
    header_signature: str
    mappings: Dict[str, str]


class ImportMappingTemplateStore:
    """User-confirmed import mapping templates.

    This store is intentionally deterministic. It does not persist model output
    or invent aliases; only manually confirmed mappings from successful imports
    are recorded, and later reuse requires an exact normalized header signature.
    """

    def __init__(self, path: Path) -> None:
        self._path = path

    def find(self, *, kind: str, headers: Sequence[str]) -> Optional[ImportMappingTemplate]:
        normalized_kind = str(kind or "").strip()
        if normalized_kind not in FC_SCHEMAS:
            return None
        signature = header_signature(normalized_kind, headers)
        payload = self._read()
        for item in payload.get("templates", []):
            if not isinstance(item, dict):
                continue
            if str(item.get("kind") or "") != normalized_kind:
                continue
            if str(item.get("header_signature") or "") != signature:
                continue
            mappings = {
                str(key or "").strip(): str(value or "").strip()
                for key, value in (item.get("mappings") or {}).items()
                if str(key or "").strip() and str(value or "").strip()
            }
            if mappings:
                return ImportMappingTemplate(
                    template_id=str(item.get("id") or ""),
                    kind=normalized_kind,
                    header_signature=signature,
                    mappings=mappings,
                )
        return None

    def record_manual_mappings(
        self,
        *,
        kind: str,
        headers: Sequence[str],
        field_mapping: Dict[str, str],
        field_mapping_origins: Dict[str, str],
        file_name: str = "",
    ) -> bool:
        normalized_kind = str(kind or "").strip()
        schema = FC_SCHEMAS.get(normalized_kind)
        if schema is None:
            return False
        header_values = [str(header or "").strip() for header in headers if str(header or "").strip()]
        if not header_values:
            return False
        header_by_normalized = {
            normalize_header_for_match(header): header
            for header in header_values
            if normalize_header_for_match(header)
        }

        manual_mappings: Dict[str, str] = {}
        for target_header, source_header in (field_mapping or {}).items():
            target_header = str(target_header or "").strip()
            source_header = str(source_header or "").strip()
            if not target_header or not source_header:
                continue
            target_key = schema.col_map.get(target_header)
            if not target_key:
                continue
            origin = str(
                (field_mapping_origins or {}).get(target_header)
                or (field_mapping_origins or {}).get(target_key)
                or ""
            ).strip().lower()
            if origin != "manual":
                continue
            actual_source = header_by_normalized.get(normalize_header_for_match(source_header))
            if not actual_source:
                continue
            manual_mappings[target_key] = actual_source

        if not manual_mappings:
            return False

        signature = header_signature(normalized_kind, header_values)
        now = _now()
        template_id = _template_id(normalized_kind, signature, manual_mappings)
        payload = self._read()
        templates = [item for item in payload.get("templates", []) if isinstance(item, dict)]
        updated = False
        for item in templates:
            if str(item.get("kind") or "") == normalized_kind and str(item.get("header_signature") or "") == signature:
                item["id"] = template_id
                item["mappings"] = dict(sorted(manual_mappings.items()))
                item["updated_at"] = now
                item["uses"] = int(item.get("uses") or 0) + 1
                item["last_file_name"] = str(file_name or "")
                updated = True
                break
        if not updated:
            templates.append(
                {
                    "id": template_id,
                    "kind": normalized_kind,
                    "header_signature": signature,
                    "headers": header_values,
                    "mappings": dict(sorted(manual_mappings.items())),
                    "created_at": now,
                    "updated_at": now,
                    "uses": 1,
                    "last_file_name": str(file_name or ""),
                }
            )
        payload = {"version": _TEMPLATE_STORE_VERSION, "templates": templates}
        self._write(payload)
        return True

    def _read(self) -> dict:
        if not self._path.exists():
            return {"version": _TEMPLATE_STORE_VERSION, "templates": []}
        try:
            payload = json.loads(self._path.read_text(encoding="utf-8"))
        except Exception:
            return {"version": _TEMPLATE_STORE_VERSION, "templates": []}
        if not isinstance(payload, dict):
            return {"version": _TEMPLATE_STORE_VERSION, "templates": []}
        if not isinstance(payload.get("templates"), list):
            payload["templates"] = []
        return payload

    def _write(self, payload: dict) -> None:
        self._path.parent.mkdir(parents=True, exist_ok=True)
        temp_path = self._path.with_name(f"{self._path.name}.tmp")
        temp_path.write_text(json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=True), encoding="utf-8")
        temp_path.replace(self._path)


def header_signature(kind: str, headers: Sequence[str]) -> str:
    tokens = sorted(
        {
            normalize_header_for_match(header)
            for header in headers
            if normalize_header_for_match(header)
        }
    )
    payload = json.dumps(
        {"kind": str(kind or "").strip(), "headers": tokens},
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    )
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def _template_id(kind: str, signature: str, mappings: Dict[str, str]) -> str:
    payload = json.dumps(
        {"kind": kind, "signature": signature, "mappings": mappings},
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    )
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()[:16]


def _now() -> str:
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")
