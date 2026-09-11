from __future__ import annotations

import json
import shutil
import zipfile
from datetime import datetime
from pathlib import Path
from typing import Any, List, Optional

from app.core.storage import CaseStorage
from app.domain.case_audit_projection import project_case_audit_record
from app.domain.controlled_artifact_gate import (
    require_controlled_artifact_publication,
    require_controlled_source_ingestion,
)
from app.utils.fs import safe_fs_name


class CaseRepository:
    """Repository adapter over CaseStorage."""

    def __init__(self) -> None:
        self._storage = CaseStorage()

    def list_cases(self, *, include_deleted: bool = False) -> List[Any]:
        return self._storage.list_cases(include_deleted=include_deleted)

    def get_last_opened_case(self) -> Optional[Any]:
        candidates = [
            case
            for case in self._storage.list_cases(include_deleted=False)
            if str(getattr(case, "last_opened_at", "") or "").strip()
        ]
        if not candidates:
            return None
        candidates.sort(
            key=lambda case: str(getattr(case, "last_opened_at", "") or "").strip(),
            reverse=True,
        )
        return candidates[0]

    def get_case(self, case_id: str) -> Optional[Any]:
        return self._storage.get_case(case_id)

    def create_case(
        self,
        *,
        case_name: str,
        case_number: str,
        owner: str,
        note: str,
        status: str,
        case_type: str,
        tags: List[str],
    ) -> Any:
        return self._storage.create_case(
            name=case_name,
            case_no=case_number,
            owner=owner,
            summary=note,
            status=status,
            case_type=case_type,
            tags=tags,
        )

    def update_case(self, case_id: str, **fields: Any) -> Any:
        return self._storage.update_case_meta(case_id, **fields)

    def soft_delete_case(self, case_id: str) -> None:
        self._storage.delete_case(case_id)

    def restore_case(self, case_id: str) -> None:
        self._storage.restore_case(case_id)

    def purge_case(self, case_id: str) -> None:
        self._storage.delete_case(case_id, permanent=True)

    def record_case_opened(self, case_id: str) -> None:
        self._storage.record_case_opened(case_id)

    def get_case_stats(self, case_id: str) -> Any:
        return self._storage.get_case_stats(case_id)

    def get_case_import_overview(self, case_id: str, *, limit: int = 5) -> dict:
        return self._storage.get_case_import_overview(case_id, limit=limit)

    def case_size(self, case_id: str) -> int:
        return self._storage.case_size(case_id)

    def list_case_audit(self, case_id: str, *, limit: int = 50) -> List[dict]:
        log_path = self._storage.app_dir / "case_audit.log"
        if not log_path.exists():
            return []

        items: List[dict] = []
        try:
            with log_path.open("r", encoding="utf-8") as handle:
                for line in reversed(handle.readlines()):
                    raw = line.strip()
                    if not raw:
                        continue
                    try:
                        payload = json.loads(raw)
                    except Exception:
                        continue
                    if str(payload.get("case_id") or "") != case_id:
                        continue
                    projected = project_case_audit_record(payload)
                    if projected.get("case_id") != case_id:
                        continue
                    items.append(projected)
                    if len(items) >= max(1, int(limit)):
                        break
        except Exception:
            return []
        return items

    def export_case_archive(self, case_id: str, *, target_dir: str = "") -> str:
        require_controlled_artifact_publication()
        case = self.get_case(case_id)
        if case is None:
            raise KeyError(case_id)

        case_dir = self._storage.case_dir(case_id)
        if not case_dir.exists():
            raise FileNotFoundError(case_id)

        output_root = (
            Path(str(target_dir).strip()).expanduser()
            if str(target_dir).strip()
            else (self._storage.app_dir / "backups")
        )
        output_root.mkdir(parents=True, exist_ok=True)

        stamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        base_name = safe_fs_name(
            f"{getattr(case, 'name', '') or case_id}_{stamp}",
            f"{case_id}_{stamp}",
        )
        archive_path = output_root / f"{base_name}.zip"
        suffix = 1
        while archive_path.exists():
            archive_path = output_root / f"{base_name}_{suffix}.zip"
            suffix += 1

        tags_raw = getattr(case, "tags", [])
        if isinstance(tags_raw, list):
            tags = [str(item).strip() for item in tags_raw if str(item).strip()]
        elif isinstance(tags_raw, str):
            tags = [chunk.strip() for chunk in tags_raw.replace("，", ",").split(",") if chunk.strip()]
        else:
            tags = []

        storage_status = str(getattr(case, "status", "") or "").strip()
        status_api = "archived" if ("归档" in storage_status or "结案" in storage_status) else "active"
        manifest = {
            "schema": "analytix.case-archive.v1",
            "exported_at": datetime.now().isoformat(timespec="seconds"),
            "source_case_id": case_id,
            "case_name": str(getattr(case, "name", "") or case_id),
            "case_number": str(getattr(case, "case_no", "") or ""),
            "owner": str(getattr(case, "owner", "") or ""),
            "note": str(getattr(case, "summary", "") or ""),
            "case_type": str(getattr(case, "case_type", "") or ""),
            "tags": tags,
            "status_api": status_api,
        }

        with zipfile.ZipFile(archive_path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
            archive.writestr(
                "manifest.json",
                json.dumps(manifest, ensure_ascii=False, indent=2),
            )
            for file_path in sorted(case_dir.rglob("*")):
                if not file_path.is_file():
                    continue
                rel = file_path.relative_to(case_dir).as_posix()
                archive.write(file_path, arcname=f"case_data/{rel}")

        return str(archive_path.resolve())

    def import_case_archive(self, archive_path: str) -> Any:
        require_controlled_source_ingestion()
        source_path = Path(str(archive_path or "").strip()).expanduser()
        if not source_path.exists() or not source_path.is_file():
            raise FileNotFoundError(str(source_path))
        if source_path.suffix.lower() != ".zip":
            raise ValueError("archive must be a .zip file")

        with zipfile.ZipFile(source_path, "r") as archive:
            names = archive.namelist()
            if "manifest.json" not in names:
                raise ValueError("archive manifest.json is missing")

            try:
                manifest_obj = json.loads(archive.read("manifest.json").decode("utf-8"))
            except Exception as exc:
                raise ValueError("archive manifest.json is invalid") from exc

            if not isinstance(manifest_obj, dict):
                raise ValueError("archive manifest.json must be an object")

            data_entries = [
                info
                for info in archive.infolist()
                if info.filename.startswith("case_data/") and not info.is_dir()
            ]
            if not data_entries:
                raise ValueError("archive has no case_data payload")

            default_case_name = source_path.stem or "导入案件"
            case_name = str(manifest_obj.get("case_name") or default_case_name).strip()[:128] or default_case_name
            case_number = str(
                manifest_obj.get("case_number")
                or manifest_obj.get("source_case_id")
                or source_path.stem
                or datetime.now().strftime("%Y%m%d%H%M%S")
            ).strip()[:64]
            if not case_number:
                case_number = datetime.now().strftime("%Y%m%d%H%M%S")
            owner = str(manifest_obj.get("owner") or "").strip()[:64]
            note = str(manifest_obj.get("note") or "").strip()[:500]
            case_type = str(manifest_obj.get("case_type") or "").strip()[:64]
            tags_raw = manifest_obj.get("tags")
            if isinstance(tags_raw, list):
                tags = [str(item).strip() for item in tags_raw if str(item).strip()][:32]
            elif isinstance(tags_raw, str):
                tags = [chunk.strip() for chunk in tags_raw.replace("，", ",").split(",") if chunk.strip()][:32]
            else:
                tags = []
            status_api = str(manifest_obj.get("status_api") or "").strip().lower()
            status = "已归档" if status_api == "archived" else "进行中"

            created = self._storage.create_case(
                name=case_name,
                case_no=case_number,
                owner=owner,
                summary=note,
                status=status,
                case_type=case_type,
                tags=tags,
            )
            case_dir = self._storage.case_dir(created.case_id).resolve()

            for info in data_entries:
                rel_name = info.filename[len("case_data/") :]
                if not rel_name:
                    continue
                rel_path = Path(rel_name)
                if any(part in ("", ".", "..") for part in rel_path.parts):
                    continue
                target_path = (case_dir / rel_path).resolve()
                if case_dir not in target_path.parents and target_path != case_dir:
                    continue
                target_path.parent.mkdir(parents=True, exist_ok=True)
                with archive.open(info, "r") as src, target_path.open("wb") as dst:
                    shutil.copyfileobj(src, dst)

        self._storage.sync_case_project_doc(created.case_id)
        imported = self.get_case(created.case_id)
        if imported is None:
            raise RuntimeError("imported case not found")
        return imported
