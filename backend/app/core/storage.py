from __future__ import annotations

import json
import hashlib
import logging
import os
import shutil
import sqlite3
import threading
import time
import uuid
from contextlib import contextmanager
from dataclasses import dataclass, field
from datetime import datetime
from getpass import getuser
from pathlib import Path
from typing import Optional, List, Dict, Any, Literal

import pandas as pd

from app.core.case_project_doc import (
    case_project_doc_path,
    parse_case_project_doc_metadata,
    sync_case_project_doc as write_case_project_doc,
)
from app.core.db_engine import DuckDBEngine, get_case_db_path, validate_case_storage_id
from app.core.diagnostic_file_transaction import transform_diagnostic_file_transactionally
from app.core.failure_boundary import project_cleaning_error, project_import_error
from app.core.import_count_semantics import (
    MAX_PUBLIC_IMPORT_COUNT,
    first_known_public_import_count,
    project_persisted_import_file_log_counts,
)
from app.core.paths import get_app_data_dir, ensure_dir
from app.core.safe_observability import log_closed_diagnostic
from app.domain.case_audit_projection import project_case_audit_record
from app.domain.case_source_state import CaseSourceUnavailableError
from app.utils.fs import (
    atomic_write_private_text,
    case_bound_storage_name,
    legacy_case_bound_storage_name_v1,
    private_exclusive_file_lock,
    read_private_text,
    remove_path_no_follow_under,
    safe_fs_name,
)


REGISTRY_FILE = "case_registry.json"
_CASE_AUDIT_LOG_LOCK = threading.RLock()
_CASE_AUDIT_MAX_BYTES = 128 * 1024 * 1024
_CASE_REGISTRY_MAX_BYTES = 32 * 1024 * 1024
_CASE_LIFECYCLE_GENERATION_FIELD = "lifecycle_generation"

_DUCKDB_TRANSIENT_OPEN_MARKERS = (
    "could not set lock on file",
    "conflicting lock is held",
    "cannot open file",
    "being used by another process",
    "another process is using",
    "另一个程序正在使用此文件",
    "进程无法访问",
)


def _is_retryable_duckdb_open_conflict(exc: Exception) -> bool:
    try:
        message = str(exc or "").lower()
    except Exception:
        return False
    return any(marker in message for marker in _DUCKDB_TRANSIENT_OPEN_MARKERS)


@dataclass
class CaseInfo:
    case_id: str
    name: str
    created_at: str
    updated_at: str
    case_no: str = ""
    case_type: str = ""
    owner: str = ""
    org: str = ""
    status: str = "进行中"
    summary: str = ""
    tags: List[str] = field(default_factory=list)
    deleted_at: str = ""
    last_opened_at: str = ""
    last_opened_by: str = ""
    created_by: str = ""
    updated_by: str = ""


@dataclass
class CaseStats:
    case_id: str
    accounts: int
    persons: int
    persons_fc: int
    persons_profile: int
    transactions: int
    tasks: int
    datasets: int
    size_bytes: int
    last_imported_at: str
    count_status: Dict[str, str] = field(default_factory=dict)


@dataclass
class PersonInfo:
    person_id: str
    name: str
    id_no: str
    role: str
    phone: str
    address: str
    source_filename: str
    source_file: str
    status: str
    photo_path: str
    created_at: str
    updated_at: str


@dataclass
class DatasetInfo:
    dataset_id: str
    filename: str
    kind: str
    rows: Optional[int]
    cols: Optional[int]
    imported_at: str
    stored_path: str


def _historical_dataset_count(value: object, *, field_name: str) -> Optional[int]:
    if value is None:
        return None
    if type(value) is not int or value < 0 or value > MAX_PUBLIC_IMPORT_COUNT:
        raise RuntimeError(f"historical_dataset_{field_name}_invalid")
    return value


def _now_iso() -> str:
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")


def _user_name() -> str:
    try:
        return getuser()
    except Exception:
        return "unknown"


def _split_tags(tags: Any) -> List[str]:
    if tags is None:
        return []
    if isinstance(tags, list):
        return [str(t).strip() for t in tags if str(t).strip()]
    if isinstance(tags, str):
        raw = tags.replace("；", ",").replace("，", ",")
        return [t.strip() for t in raw.split(",") if t.strip()]
    return [str(tags).strip()]


def _normalize_project_doc_status(status: Any) -> str:
    raw = str(status or "").strip().lower()
    if raw in {"archived", "archive", "已归档", "归档", "结案"} or "归档" in raw or "结案" in raw:
        return "已归档"
    return "进行中"


def _table_exists(engine: DuckDBEngine, name: str) -> bool:
    rows = engine.query(
        "SELECT 1 FROM information_schema.tables WHERE table_schema='main' AND table_name=? LIMIT 1",
        (name,),
    )
    return bool(rows)


def _table_columns(engine: DuckDBEngine, table: str) -> List[str]:
    rows = engine.query(
        "SELECT column_name FROM information_schema.columns WHERE table_schema='main' AND table_name=?",
        (table,),
    )
    return [r[0] for r in rows]


def _sqlite_table_exists(con: sqlite3.Connection, name: str) -> bool:
    cur = con.cursor()
    cur.execute("SELECT name FROM sqlite_master WHERE type='table' AND name=?", (name,))
    return cur.fetchone() is not None


def clean_cell(x):
    # 清理 \t 和前后空格
    if isinstance(x, str):
        return x.replace("\t", "").strip()
    return x


def clean_columns(cols):
    out = []
    for c in cols:
        if c is None:
            out.append("")
            continue
        s = str(c).replace("\t", "").strip()
        out.append(s)
    return out


def _project_case_audit_file(source: bytes | None) -> bytes:
    if source is None or source == b"":
        return b""
    try:
        text = source.decode("utf-8")
    except UnicodeError:
        raise ValueError("case_audit_encoding_invalid") from None
    projected_lines: list[str] = []
    for line in text.splitlines():
        raw = line.strip()
        if not raw:
            continue
        try:
            payload = json.loads(raw, object_pairs_hook=_strict_case_audit_object)
        except (json.JSONDecodeError, ValueError):
            payload = {}
        if not isinstance(payload, dict):
            payload = {}
        projected = project_case_audit_record(payload)
        projected_lines.append(
            json.dumps(
                projected,
                ensure_ascii=False,
                separators=(",", ":"),
                sort_keys=True,
            )
        )
    if not projected_lines:
        return b""
    return ("\n".join(projected_lines) + "\n").encode("utf-8")


def _strict_case_audit_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate case-audit key")
        result[key] = value
    return result


def detect_kind(filename: str, df: pd.DataFrame) -> str:
    name = filename.lower()
    cols = {str(c).strip() for c in df.columns}
    # 交易明细：常见字段
    if ("交易时间" in cols or "交易日期" in cols) and ("交易金额" in cols or "发生额" in cols):
        return "交易明细"
    if "账户信息" in name or ("开户机构" in cols and "账号" in cols):
        return "账户信息"
    if "人员信息" in name or ("姓名" in cols and ("证件号码" in cols or "身份证号" in cols)):
        return "人员信息"
    if "联系方式" in name or ("联系方式" in cols or "电话号码" in cols):
        return "联系方式"
    if "住址" in name or ("住址" in cols or "地址" in cols):
        return "住址信息"
    if "任务" in name or ("任务流水号" in cols or "请求单号" in cols):
        return "任务信息"
    return "未知"


class CaseStorage:
    """案件注册 + 单案数据落地"""

    def __init__(self):
        self.app_dir = ensure_dir(get_app_data_dir())
        self.cases_dir = ensure_dir(self.app_dir / "cases")
        self.registry_path = self.app_dir / REGISTRY_FILE
        self._registry: Dict[str, Dict[str, Any]] = self._load_registry()
        self._case_engine_open_locks_guard = threading.RLock()
        self._case_engine_open_locks: Dict[str, threading.RLock] = {}
        self._migrate_case_audit_log()

    def _registry_lock_path(self) -> Path:
        return self.app_dir / "case_registry.lock"

    def _flow_inventory_lock_path(self) -> Path:
        return self.app_dir / "flow_result_snapshot_inventory.lock"

    def _read_registry_file(self) -> Dict[str, Dict[str, Any]]:
        if self.registry_path.exists():
            try:
                payload = json.loads(
                    read_private_text(
                        self.registry_path,
                        max_bytes=_CASE_REGISTRY_MAX_BYTES,
                    )
                )
            except (OSError, UnicodeError, json.JSONDecodeError):
                raise RuntimeError("case_registry_invalid") from None
            if not isinstance(payload, dict):
                raise RuntimeError("case_registry_invalid")
            registry: Dict[str, Dict[str, Any]] = {}
            for case_id, meta in payload.items():
                try:
                    normalized_case_id = validate_case_storage_id(case_id)
                except ValueError:
                    raise RuntimeError("case_registry_invalid") from None
                if normalized_case_id != case_id or not isinstance(meta, dict):
                    raise RuntimeError("case_registry_invalid")
                normalized_meta = dict(meta)
                raw_status = normalized_meta.get("status")
                raw_deleted_at = normalized_meta.get("deleted_at")
                raw_generation = normalized_meta.get(_CASE_LIFECYCLE_GENERATION_FIELD)
                if raw_status not in {"进行中", "已归档"}:
                    raise RuntimeError("case_registry_invalid")
                if not isinstance(raw_deleted_at, str):
                    raise RuntimeError("case_registry_invalid")
                if type(raw_generation) is not int or raw_generation < 1:
                    raise RuntimeError("case_registry_invalid")
                normalized_meta[_CASE_LIFECYCLE_GENERATION_FIELD] = raw_generation
                registry[normalized_case_id] = normalized_meta
            return registry
        return {}

    def _write_registry_file(self) -> None:
        atomic_write_private_text(
            self.registry_path,
            json.dumps(self._registry, ensure_ascii=False, indent=2),
        )

    def _load_registry(self) -> Dict[str, Dict[str, Any]]:
        with private_exclusive_file_lock(self._registry_lock_path()):
            return self._read_registry_file()

    def _save_registry(self):
        with private_exclusive_file_lock(self._registry_lock_path()):
            self._write_registry_file()

    @contextmanager
    def _registry_write_transaction(self):
        with private_exclusive_file_lock(self._registry_lock_path()):
            self._registry = self._read_registry_file()
            yield
            self._write_registry_file()

    def _refresh_registry(self) -> None:
        self._registry = self._load_registry()

    def _case_info_from_meta(self, case_id: str, meta: Dict[str, Any]) -> CaseInfo:
        tags = _split_tags(meta.get("tags", []))
        return CaseInfo(
            case_id=case_id,
            name=meta.get("name", case_id),
            created_at=meta.get("created_at", ""),
            updated_at=meta.get("updated_at", ""),
            case_no=meta.get("case_no", ""),
            case_type=meta.get("case_type", ""),
            owner=meta.get("owner", ""),
            org=meta.get("org", ""),
            status=meta["status"],
            summary=meta.get("summary", ""),
            tags=tags,
            deleted_at=meta["deleted_at"],
            last_opened_at=meta.get("last_opened_at", ""),
            last_opened_by=meta.get("last_opened_by", ""),
            created_by=meta.get("created_by", ""),
            updated_by=meta.get("updated_by", ""),
        )

    def _sync_case_project_doc_metadata_from_file(self, case_id: str) -> list[str]:
        meta = self._registry.get(case_id)
        if meta is None:
            return []

        path = case_project_doc_path(self.case_dir(case_id))
        if not path.exists():
            return []
        try:
            parsed = parse_case_project_doc_metadata(path.read_text(encoding="utf-8", errors="ignore"))
        except OSError:
            return []
        if not parsed:
            return []

        parsed_case_id = str(parsed.get("case_id") or "").strip()
        if parsed_case_id and parsed_case_id != case_id:
            return []

        candidate_fields: dict[str, Any] = {}
        if "name" in parsed:
            name = str(parsed.get("name") or "").strip()
            if name:
                candidate_fields["name"] = name
        if "case_no" in parsed:
            candidate_fields["case_no"] = str(parsed.get("case_no") or "").strip()
        if "case_type" in parsed:
            candidate_fields["case_type"] = str(parsed.get("case_type") or "").strip()
        if "tags" in parsed:
            candidate_fields["tags"] = _split_tags(parsed.get("tags", []))
        if "status" in parsed:
            candidate_fields["status"] = _normalize_project_doc_status(parsed.get("status"))
        if "summary" in parsed:
            candidate_fields["summary"] = str(parsed.get("summary") or "").strip()

        changed: list[str] = []
        persisted_meta: Dict[str, Any] = {}
        with self._registry_write_transaction():
            meta = self._registry.get(case_id)
            if meta is None:
                return []
            for key, value in candidate_fields.items():
                current = _split_tags(meta.get(key, [])) if key == "tags" else str(meta.get(key) or "").strip()
                if current == value:
                    continue
                meta[key] = value
                changed.append(key)
            if changed:
                meta["updated_at"] = _now_iso()
                meta["updated_by"] = _user_name()
            persisted_meta = dict(meta)

        if changed:
            self._audit("sync_case_project_doc_metadata", case_id, {"fields": sorted(changed)})
            try:
                write_case_project_doc(
                    self.case_dir(case_id),
                    self._case_info_from_meta(case_id, persisted_meta),
                )
            except Exception:
                pass
        return changed

    def _migrate_case_audit_log(self) -> None:
        log_path = self.app_dir / "case_audit.log"
        with _CASE_AUDIT_LOG_LOCK:
            transform_diagnostic_file_transactionally(
                log_path,
                _project_case_audit_file,
                surface="case_audit",
                max_bytes=_CASE_AUDIT_MAX_BYTES,
            )

    def _append_case_audit_payload(self, payload: Dict[str, Any]) -> None:
        encoded = json.dumps(payload, ensure_ascii=False, separators=(",", ":"), sort_keys=True).encode("utf-8")

        def append(source: bytes | None) -> bytes:
            current = source or b""
            if current and not current.endswith(b"\n"):
                raise ValueError("case_audit_generation_invalid")
            if len(current) + len(encoded) + 1 > _CASE_AUDIT_MAX_BYTES:
                raise ValueError("case_audit_generation_limit")
            return current + encoded + b"\n"

        with _CASE_AUDIT_LOG_LOCK:
            transform_diagnostic_file_transactionally(
                self.app_dir / "case_audit.log",
                append,
                surface="case_audit",
                max_bytes=_CASE_AUDIT_MAX_BYTES,
            )

    def _audit(self, action: str, case_id: str, extra: Optional[Dict[str, Any]] = None):
        payload = project_case_audit_record({
            "time": _now_iso(),
            "action": action,
            "case_id": case_id,
            "details": extra or {},
        })
        try:
            self._append_case_audit_payload(payload)
        except Exception:
            pass

    def list_cases(self, *, include_deleted: bool = False) -> List[CaseInfo]:
        self._refresh_registry()
        out = []
        for cid, v in self._registry.items():
            deleted_at = v.get("deleted_at", "")
            if deleted_at and not include_deleted:
                continue
            out.append(self._case_info_from_meta(cid, v))
        # 最近更新优先
        out.sort(key=lambda x: x.updated_at or "", reverse=True)
        return out

    def get_case(self, case_id: str) -> Optional[CaseInfo]:
        self._refresh_registry()
        if case_id not in self._registry:
            return None
        return self._case_info_from_meta(case_id, self._registry[case_id])

    def get_case_lifecycle_generation(self, case_id: str) -> Optional[int]:
        state = self.get_case_binding_state(case_id)
        if state is None:
            return None
        return int(state["lifecycle_generation"])

    def get_case_binding_state(self, case_id: str) -> Optional[Dict[str, Any]]:
        try:
            normalized_case_id = validate_case_storage_id(case_id)
        except ValueError:
            return None
        with private_exclusive_file_lock(self._registry_lock_path()):
            self._registry = self._read_registry_file()
            meta = self._registry.get(normalized_case_id)
            if not isinstance(meta, dict):
                return None
            generation = meta.get(_CASE_LIFECYCLE_GENERATION_FIELD)
            deleted_at = meta.get("deleted_at")
            if type(generation) is not int or generation < 1 or not isinstance(deleted_at, str):
                return None
            return {
                "case_id": normalized_case_id,
                "deleted_at": deleted_at.strip(),
                "lifecycle_generation": generation,
            }

    def update_case_meta(self, case_id: str, **fields) -> CaseInfo:
        if {"deleted_at", _CASE_LIFECYCLE_GENERATION_FIELD}.intersection(fields):
            raise ValueError("case_lifecycle_field_reserved")
        with self._registry_write_transaction():
            if case_id not in self._registry:
                raise KeyError(case_id)
            meta = self._registry[case_id]
            for k, v in fields.items():
                if k == "tags":
                    meta[k] = _split_tags(v)
                elif k == "status":
                    meta[k] = v or "进行中"
                else:
                    meta[k] = v
            meta["updated_at"] = _now_iso()
            meta["updated_by"] = _user_name()
        self.sync_case_project_doc(case_id)
        self._audit("update_case", case_id, {"fields": sorted(list(fields.keys()))})
        return self.get_case(case_id) or CaseInfo(case_id=case_id, name=case_id, created_at="", updated_at="")

    def record_case_opened(self, case_id: str):
        with self._registry_write_transaction():
            if case_id not in self._registry:
                return
            meta = self._registry[case_id]
            meta["last_opened_at"] = _now_iso()
            meta["last_opened_by"] = _user_name()
        self._audit("open_case", case_id)

    def backup_case(self, case_id: str) -> Optional[Path]:
        if case_id not in self._registry:
            return None
        case_dir = self.case_dir(case_id)
        if not case_dir.exists():
            return None
        backup_dir = ensure_dir(self.app_dir / "backups")
        stamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        base = backup_dir / f"{case_id}_{stamp}"
        try:
            archive = shutil.make_archive(str(base), "zip", root_dir=str(case_dir))
            self._audit("backup_case", case_id, {"archive": archive})
            return Path(archive)
        except Exception:
            return None

    def soft_delete_case(self, case_id: str):
        try:
            normalized_case_id = validate_case_storage_id(case_id)
        except ValueError:
            return
        case_storage_name = case_bound_storage_name(normalized_case_id)
        lock_path = self.app_dir / "flow_result_snapshot_locks" / f"{case_storage_name}.lock"
        with private_exclusive_file_lock(lock_path):
            with self._registry_write_transaction():
                if normalized_case_id not in self._registry:
                    return
                meta = self._registry[normalized_case_id]
                if not str(meta.get("deleted_at") or "").strip():
                    meta[_CASE_LIFECYCLE_GENERATION_FIELD] = int(
                        meta.get(_CASE_LIFECYCLE_GENERATION_FIELD) or 1
                    ) + 1
                meta["deleted_at"] = _now_iso()
                meta["updated_at"] = _now_iso()
                meta["updated_by"] = _user_name()
        self._audit("delete_case", case_id)

    def restore_case(self, case_id: str):
        try:
            normalized_case_id = validate_case_storage_id(case_id)
        except ValueError:
            return
        case_storage_name = case_bound_storage_name(normalized_case_id)
        lock_path = self.app_dir / "flow_result_snapshot_locks" / f"{case_storage_name}.lock"
        with private_exclusive_file_lock(lock_path):
            with self._registry_write_transaction():
                if normalized_case_id not in self._registry:
                    return
                meta = self._registry[normalized_case_id]
                if str(meta.get("deleted_at") or "").strip():
                    meta[_CASE_LIFECYCLE_GENERATION_FIELD] = int(
                        meta.get(_CASE_LIFECYCLE_GENERATION_FIELD) or 1
                    ) + 1
                meta["deleted_at"] = ""
                meta["updated_at"] = _now_iso()
                meta["updated_by"] = _user_name()
        self._audit("restore_case", case_id)

    def purge_case(self, case_id: str):
        try:
            normalized_case_id = validate_case_storage_id(case_id)
        except ValueError:
            raise KeyError("case_registry_binding_invalid") from None
        case_storage_name = case_bound_storage_name(normalized_case_id)
        lock_path = self.app_dir / "flow_result_snapshot_locks" / f"{case_storage_name}.lock"
        with private_exclusive_file_lock(self._flow_inventory_lock_path()):
            with private_exclusive_file_lock(lock_path):
                with self._registry_write_transaction():
                    normalized_case_id = self._require_registered_case_id(normalized_case_id)
                    case_dir = self.case_dir(normalized_case_id)
                    safe_case_id = safe_fs_name(normalized_case_id, "case")
                    legacy_case_storage_name = legacy_case_bound_storage_name_v1(normalized_case_id)
                    for owned_path in (
                        case_dir,
                        self.app_dir / "flow_view_metadata_v2" / f"{case_storage_name}.json",
                        self.app_dir / "flow_view_metadata_v2" / f"{legacy_case_storage_name}.json",
                        self.app_dir / "flow_result_snapshots" / case_storage_name,
                        self.app_dir / "flow_result_snapshots" / legacy_case_storage_name,
                        # Pre-v2 factual aliases are invalidated during the same purge.
                        self.app_dir / "flow_result_snapshots" / safe_case_id,
                        self.app_dir / "flow_job_results" / case_storage_name,
                        self.app_dir / "flow_job_results" / legacy_case_storage_name,
                        self.app_dir / "flow_job_results" / safe_case_id,
                    ):
                        remove_path_no_follow_under(self.app_dir, owned_path)
                    del self._registry[normalized_case_id]
        self._audit("purge_case", normalized_case_id)

    def create_case(
        self,
        name: str,
        *,
        case_no: str = "",
        case_type: str = "",
        owner: str = "",
        org: str = "",
        status: str = "进行中",
        summary: str = "",
        tags: Optional[List[str]] = None,
    ) -> CaseInfo:
        created = _now_iso()
        user = _user_name()
        with private_exclusive_file_lock(self._flow_inventory_lock_path()):
            with self._registry_write_transaction():
                while True:
                    cid = uuid.uuid4().hex[:12]
                    if cid not in self._registry:
                        break
                self._registry[cid] = {
                    "name": name,
                    "case_no": case_no,
                    "case_type": case_type,
                    "owner": owner,
                    "org": org,
                    "status": status or "进行中",
                    "summary": summary,
                    "tags": _split_tags(tags or []),
                    "created_at": created,
                    "updated_at": created,
                    "created_by": user,
                    "updated_by": user,
                    "last_opened_at": "",
                    "last_opened_by": "",
                    "deleted_at": "",
                    _CASE_LIFECYCLE_GENERATION_FIELD: 1,
                }

        case_dir = ensure_dir(self.cases_dir / cid)
        ensure_dir(case_dir / "raw")
        ensure_dir(case_dir / "datasets")
        self._init_case_db(cid)
        self.sync_case_project_doc(cid)
        self._audit("create_case", cid, {"name": name})
        return CaseInfo(
            case_id=cid,
            name=name,
            created_at=created,
            updated_at=created,
            case_no=case_no,
            case_type=case_type,
            owner=owner,
            org=org,
            status=status or "进行中",
            summary=summary,
            tags=_split_tags(tags or []),
            deleted_at="",
            last_opened_at="",
            last_opened_by="",
            created_by=user,
            updated_by=user,
        )

    def delete_case(self, case_id: str, *, permanent: bool = False):
        if permanent:
            self.purge_case(case_id)
        else:
            self.soft_delete_case(case_id)

    def rename_case(self, case_id: str, new_name: str):
        self.update_case_meta(case_id, name=new_name)

    def touch_case(self, case_id: str):
        with self._registry_write_transaction():
            if case_id in self._registry:
                self._registry[case_id]["updated_at"] = _now_iso()
                self._registry[case_id]["updated_by"] = _user_name()

    def case_dir(self, case_id: str) -> Path:
        normalized_case_id = self._require_registered_case_id(case_id)
        cases_root = self.cases_dir.resolve()
        candidate = cases_root / normalized_case_id
        if candidate.is_symlink() or candidate.resolve(strict=False).parent != cases_root:
            raise RuntimeError("case_storage_path_invalid")
        return candidate

    def sync_case_project_doc(self, case_id: str, *, source: Literal["registry", "document"] = "registry") -> Path:
        self._refresh_registry()
        if case_id not in self._registry:
            raise KeyError(case_id)
        if source == "document":
            self._sync_case_project_doc_metadata_from_file(case_id)
        case = self._case_info_from_meta(case_id, self._registry[case_id])
        return write_case_project_doc(self.case_dir(case_id), case)

    def case_db(self, case_id: str) -> Path:
        normalized_case_id = self._require_registered_case_id(case_id)
        db_path = get_case_db_path(normalized_case_id)
        expected = self.case_dir(normalized_case_id) / "case.duckdb"
        if db_path != expected or db_path.is_symlink():
            raise RuntimeError("case_storage_path_invalid")
        return db_path

    def open_case_engine(self, case_id: str, *, read_only: bool = False) -> DuckDBEngine:
        db_path = self.case_db(case_id)
        with self._case_engine_open_lock(case_id):
            if read_only:
                if not db_path.is_file():
                    raise FileNotFoundError("case DuckDB database is unavailable for read-only access")
            else:
                self._ensure_case_db(case_id)
            try:
                from app.core.analysis_compute_worker import shutdown_stats_query_worker

                shutdown_stats_query_worker()
            except Exception:
                pass
            # A caller that requests read-only access must receive an engine whose
            # connection/session is read-only. Mixed-mode conflicts fail closed;
            # they must never be resolved by silently upgrading a query to write
            # authority or by initializing/migrating the case database on a read.
            engine = DuckDBEngine(db_path, read_only=read_only)
            try:
                from app.core.diagnostic_duckdb_migration import (
                    assert_duckdb_diagnostic_migration_settled,
                    migrate_legacy_duckdb_diagnostics,
                )

                if read_only:
                    assert_duckdb_diagnostic_migration_settled(engine, case_id=case_id)
                else:
                    migrate_legacy_duckdb_diagnostics(engine, case_id=case_id)
            except BaseException:
                engine.close()
                raise
            return engine

    def _case_engine_open_lock(self, case_id: str) -> threading.RLock:
        normalized_case_id = self._require_registered_case_id(case_id)
        with self._case_engine_open_locks_guard:
            lock = self._case_engine_open_locks.get(normalized_case_id)
            if lock is None:
                lock = threading.RLock()
                self._case_engine_open_locks[normalized_case_id] = lock
            return lock

    def _require_registered_case_id(self, case_id: object) -> str:
        try:
            normalized_case_id = validate_case_storage_id(case_id)
        except ValueError:
            raise KeyError("case_registry_binding_invalid") from None
        if normalized_case_id not in self._registry:
            raise KeyError("case_registry_binding_invalid")
        return normalized_case_id

    def _ensure_case_db(self, case_id: str):
        ensure_dir(self.case_dir(case_id))
        db_path = self.case_db(case_id)
        if db_path.exists():
            return
        sqlite_meta_path = self.case_dir(case_id) / "meta.sqlite"
        if sqlite_meta_path.exists():
            self._migrate_case_sqlite_to_duckdb(case_id, sqlite_meta_path, db_path)
            return
        self._init_case_db(case_id)

    def _migrate_case_sqlite_to_duckdb(self, case_id: str, sqlite_path: Path, duckdb_path: Path):
        start = time.perf_counter()

        engine = DuckDBEngine(duckdb_path)
        try:
            self._init_case_db(case_id, engine=engine)
            from app.core.fc_import_tables import ensure_fc_tables
            ensure_fc_tables(engine)

            src = sqlite3.connect(sqlite_path)
            try:
                src_cur = src.cursor()
                src_cur.execute("SELECT name FROM sqlite_master WHERE type='table'")
                tables = [r[0] for r in src_cur.fetchall() if r and r[0] and not r[0].startswith("sqlite_")]

                for table in tables:
                    if not _sqlite_table_exists(src, table):
                        continue
                    src_cur = src.cursor()
                    src_cur.execute(f"PRAGMA table_info({table})")
                    src_cols = [r[1] for r in src_cur.fetchall()]
                    if not src_cols:
                        continue

                    if not _table_exists(engine, table):
                        cols_def = ", ".join([f"{c} TEXT" for c in src_cols])
                        engine.execute(f"CREATE TABLE IF NOT EXISTS {table}({cols_def})")

                    dst_cols = set(_table_columns(engine, table))
                    for col in src_cols:
                        if col not in dst_cols:
                            engine.execute(f"ALTER TABLE {table} ADD COLUMN {col} TEXT")
                            dst_cols.add(col)

                    cols = [c for c in src_cols if c in dst_cols]
                    if not cols:
                        continue
                    placeholders = ",".join(["?"] * len(cols))
                    insert_sql = f"INSERT INTO {table}({','.join(cols)}) VALUES({placeholders})"

                    total = 0
                    try:
                        src_cur.execute(f"SELECT COUNT(1) FROM {table}")
                        row = src_cur.fetchone()
                        total = int(row[0] or 0) if row else 0
                    except Exception:
                        total = 0

                    t0 = time.perf_counter()
                    src_cur.execute(f"SELECT {', '.join(cols)} FROM {table}")
                    batch = 5000
                    copied = 0
                    while True:
                        rows = src_cur.fetchmany(batch)
                        if not rows:
                            break
                        engine.executemany(insert_sql, rows)
                        copied += len(rows)
                    t1 = time.perf_counter()
                    log_closed_diagnostic(
                        logging.getLogger("analytix.data_analysis.migration"),
                        logging.INFO,
                        topic="application",
                        code="completed",
                        numeric={
                            "affected_scope_count": copied,
                            "duration_ms": max(0, int((t1 - t0) * 1000)),
                        },
                    )
            finally:
                src.close()
        finally:
            engine.close()

        backup = sqlite_path.with_suffix(sqlite_path.suffix + ".bak")
        if backup.exists():
            stamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            backup = sqlite_path.with_suffix(sqlite_path.suffix + f".bak.{stamp}")
        try:
            sqlite_path.rename(backup)
        except Exception:
            pass

        elapsed = time.perf_counter() - start
        log_closed_diagnostic(
            logging.getLogger("analytix.data_analysis.migration"),
            logging.INFO,
            topic="application",
            code="completed",
            numeric={"duration_ms": max(0, int(elapsed * 1000))},
        )

    def case_size(self, case_id: str) -> int:
        case_dir = self.case_dir(case_id)
        if not case_dir.exists():
            raise CaseSourceUnavailableError("case_storage_source_unavailable")
        total = 0
        for p in case_dir.rglob("*"):
            try:
                if p.is_file():
                    total += p.stat().st_size
            except Exception:
                continue
        return total

    def _init_case_db(self, case_id: str, *, engine: Optional[DuckDBEngine] = None):
        close_engine = False
        if engine is None:
            engine = DuckDBEngine(self.case_db(case_id))
            close_engine = True

        engine.execute(
            """CREATE TABLE IF NOT EXISTS datasets(
                dataset_id TEXT PRIMARY KEY,
                filename TEXT,
                kind TEXT,
                rows INTEGER,
                cols INTEGER,
                imported_at TEXT,
                stored_path TEXT
            )"""
        )

        engine.execute(
            """CREATE TABLE IF NOT EXISTS persons(
                person_id TEXT PRIMARY KEY,
                name TEXT,
                id_no TEXT,
                role TEXT,
                phone TEXT,
                address TEXT,
                source_filename TEXT,
                source_file TEXT,
                status TEXT,
                source_dataset_id TEXT,
                created_at TEXT,
                updated_at TEXT
            )"""
        )
        engine.execute("CREATE SEQUENCE IF NOT EXISTS seq_person_photos")
        engine.execute(
            """CREATE TABLE IF NOT EXISTS person_photos(
                id BIGINT PRIMARY KEY DEFAULT nextval('seq_person_photos'),
                person_id TEXT,
                photo_path TEXT,
                note TEXT,
                created_at TEXT
            )"""
        )

        try:
            engine.execute("DROP TABLE IF EXISTS case_stats_cache")
        except Exception as exc:
            raise RuntimeError("legacy_case_stats_cache_invalidation_failed") from exc

        # 兼容历史 persons 表字段缺失
        try:
            cols = set(_table_columns(engine, "persons"))
            alters = []
            if "role" not in cols:
                alters.append("ALTER TABLE persons ADD COLUMN role TEXT")
            if "source_file" not in cols:
                alters.append("ALTER TABLE persons ADD COLUMN source_file TEXT")
            if "status" not in cols:
                alters.append("ALTER TABLE persons ADD COLUMN status TEXT")
            if "created_at" not in cols:
                alters.append("ALTER TABLE persons ADD COLUMN created_at TEXT")
            if "updated_at" not in cols:
                alters.append("ALTER TABLE persons ADD COLUMN updated_at TEXT")
            for sql in alters:
                try:
                    engine.execute(sql)
                except Exception:
                    pass
        except Exception:
            pass

        if close_engine:
            engine.close()

    def record_case_audit(self, case_id: str, action: str, *, extra: Optional[Dict[str, Any]] = None) -> None:
        self._audit(action, case_id, extra)

    def list_datasets(self, case_id: str) -> List[DatasetInfo]:
        del case_id
        # The legacy datasets table has no row-level case binding or immutable
        # source lineage. Reading it as a same-case catalog would manufacture
        # authority from the containing path. P2 replaces it with a case-bound
        # immutable dataset registry; until then this surface is quarantined.
        raise CaseSourceUnavailableError("historical_dataset_case_binding_unavailable")

    def get_case_stats(
        self,
        case_id: str,
        *,
        force_refresh: bool = False,
        engine: Optional[DuckDBEngine] = None,
    ) -> CaseStats:
        size_bytes = self.case_size(case_id)
        close_engine = False
        if engine is None:
            try:
                engine = self.open_case_engine(case_id, read_only=True)
            except Exception as exc:
                raise CaseSourceUnavailableError("case_stats_source_unavailable") from exc
            close_engine = True
        del force_refresh
        # P0: compute from the currently opened case database. Legacy factual
        # cache rows are never an authority and are deleted by schema setup.
        stats = self.compute_case_stats(case_id, engine=engine)
        if close_engine:
            engine.close()
        return CaseStats(
            case_id=case_id,
            accounts=stats.accounts,
            persons=stats.persons,
            persons_fc=stats.persons_fc,
            persons_profile=stats.persons_profile,
            transactions=stats.transactions,
            tasks=stats.tasks,
            datasets=stats.datasets,
            size_bytes=size_bytes,
            last_imported_at=stats.last_imported_at,
            count_status=dict(stats.count_status),
        )

    def compute_case_stats(
        self,
        case_id: str,
        *,
        engine: Optional[DuckDBEngine] = None,
    ) -> CaseStats:
        close_engine = False
        if engine is None:
            self._ensure_case_db(case_id)
            engine = DuckDBEngine(self.case_db(case_id))
            close_engine = True

        def _table_state(table: str) -> Optional[bool]:
            try:
                return _table_exists(engine, table)
            except Exception:
                return None

        def _count_existing(table: str) -> tuple[int, str]:
            try:
                t0 = time.perf_counter()
                rows = engine.query(f"SELECT COUNT(1) FROM {table}")
                row = rows[0] if rows else None
                if row is None or len(row) != 1 or type(row[0]) is not int or row[0] < 0:
                    raise ValueError("case_stats_count_invalid")
                count = row[0]
                elapsed = time.perf_counter() - t0
                log_closed_diagnostic(
                    logging.getLogger("analytix.data_analysis.stats"),
                    logging.INFO,
                    topic="stats_request",
                    code="completed",
                    numeric={
                        "affected_scope_count": count,
                        "duration_ms": max(0, int(elapsed * 1000)),
                    },
                )
                return count, "verified"
            except Exception:
                return 0, "unavailable"

        def _count_first_available(*tables: str) -> tuple[int, str]:
            for table in tables:
                available = _table_state(table)
                if available is None:
                    return 0, "unavailable"
                if available:
                    return _count_existing(table)
            return 0, "unavailable"

        accounts, accounts_status = _count_first_available("fc_account_norm", "fc_account")
        persons_fc, persons_fc_status = _count_first_available("fc_person_norm", "fc_person")
        persons_profile, persons_profile_status = _count_first_available("persons")
        transactions, transactions_status = _count_first_available("fc_transaction_norm", "fc_transaction")
        tasks, tasks_status = _count_first_available("import_file_log")
        datasets, datasets_status = _count_first_available("datasets")

        last_imported_at = ""
        if _table_state("import_file_log") is True:
            try:
                rows = engine.query(
                    "SELECT COALESCE(finished_at, created_at) FROM import_file_log "
                    "ORDER BY COALESCE(finished_at, created_at) DESC LIMIT 1"
                )
                row = rows[0] if rows else None
                if row and row[0]:
                    last_imported_at = row[0]
            except Exception:
                pass
        if not last_imported_at and _table_state("datasets") is True:
            try:
                rows = engine.query("SELECT imported_at FROM datasets ORDER BY imported_at DESC LIMIT 1")
                row = rows[0] if rows else None
                if row and row[0]:
                    last_imported_at = row[0]
            except Exception:
                pass

        size_bytes = self.case_size(case_id)
        if persons_fc_status == "verified" and persons_fc > 0:
            persons = persons_fc
            persons_status = "verified"
        elif persons_fc_status == "verified" and persons_profile_status == "verified":
            persons = persons_profile
            persons_status = "verified"
        else:
            persons = 0
            persons_status = "unavailable"
        count_status = {
            "accounts": accounts_status,
            "persons": persons_status,
            "persons_fc": persons_fc_status,
            "persons_profile": persons_profile_status,
            "transactions": transactions_status,
            "tasks": tasks_status,
            "datasets": datasets_status,
        }
        if close_engine:
            engine.close()
        return CaseStats(
            case_id=case_id,
            accounts=accounts,
            persons=persons,
            persons_fc=persons_fc,
            persons_profile=persons_profile,
            transactions=transactions,
            tasks=tasks,
            datasets=datasets,
            size_bytes=size_bytes,
            last_imported_at=last_imported_at,
            count_status=count_status,
        )

    def get_funds_cleaning_state(self, case_id: str) -> tuple[str, str]:
        if not case_id:
            return ("", "")
        engine = self.open_case_engine(case_id)
        last_import_at = ""
        last_clean_at = ""
        try:
            if _table_exists(engine, "import_file_log"):
                rows = engine.query(
                    "SELECT COALESCE(finished_at, created_at) "
                    "FROM import_file_log "
                    "WHERE case_id=? AND kind LIKE 'fc_%' "
                    "ORDER BY COALESCE(finished_at, created_at) DESC LIMIT 1",
                    (case_id,),
                )
                row = rows[0] if rows else None
                if row and row[0]:
                    last_import_at = row[0]
            if _table_exists(engine, "cleaning_log"):
                rows = engine.query(
                    "SELECT cleaned_at FROM cleaning_log WHERE case_id=? ORDER BY cleaned_at DESC LIMIT 1",
                    (case_id,),
                )
                row = rows[0] if rows else None
                if row and row[0]:
                    last_clean_at = row[0]
        finally:
            engine.close()
        return (last_import_at, last_clean_at)

    def is_funds_clean_ready(self, case_id: str) -> bool:
        last_import_at, last_clean_at = self.get_funds_cleaning_state(case_id)
        if not last_import_at or not last_clean_at:
            return False
        return last_clean_at >= last_import_at

    def get_case_import_overview(self, case_id: str, limit: int = 5) -> Dict[str, Any]:
        try:
            engine = self.open_case_engine(case_id, read_only=True)
        except Exception as exc:
            raise CaseSourceUnavailableError("case_import_source_unavailable") from exc
        try:
            if not _table_exists(engine, "import_file_log"):
                raise CaseSourceUnavailableError("case_import_source_unavailable")
            cols = set(_table_columns(engine, "import_file_log"))
            if not {"case_id", "file_id"}.issubset(cols):
                raise CaseSourceUnavailableError("case_import_binding_unavailable")
            foreign_rows = engine.query(
                "SELECT COUNT(1) FROM import_file_log WHERE case_id IS NULL OR case_id<>?",
                (case_id,),
            )
            if (
                len(foreign_rows) != 1
                or len(foreign_rows[0]) != 1
                or type(foreign_rows[0][0]) is not int
                or foreign_rows[0][0] != 0
            ):
                raise CaseSourceUnavailableError("case_import_binding_mismatch")
            base_cols = [
                "case_id", "filename", "status", "rows_total", "rows_imported", "rows_imported_raw", "rows_imported_norm",
                "rows_dedup", "rows_error", "rows_skipped_non_data", "import_counts_version", "error",
                "created_at", "finished_at",
            ]
            select_cols = [c for c in base_cols if c in cols]
            order_by = "created_at" if "created_at" in cols else "file_id"
            rows = engine.query(
                f"SELECT {', '.join(select_cols)} FROM import_file_log WHERE case_id=? "
                f"ORDER BY {order_by} DESC LIMIT ?",
                (case_id, limit),
            )
        finally:
            engine.close()

        recent = []
        status_counts: Dict[str, int] = {}
        risk_flags = set()
        last_imported_at = ""
        for r in rows:
            row_data = dict(zip(select_cols, r))
            filename = row_data.get("filename")
            status = row_data.get("status")
            projected_counts = project_persisted_import_file_log_counts(row_data)
            rows_imported = first_known_public_import_count(
                projected_counts.get("rows_imported_norm"),
                projected_counts.get("rows_imported"),
            )
            rows_dedup = projected_counts.get("rows_dedup")
            rows_error = projected_counts.get("rows_error")
            rows_skipped_non_data = projected_counts.get("rows_skipped_non_data")
            error = row_data.get("error")
            created_at = row_data.get("created_at")
            finished_at = row_data.get("finished_at")
            status = status or ""
            status_counts[status] = status_counts.get(status, 0) + 1
            if status == "失败":
                risk_flags.add("导入失败")
            if rows_dedup is not None and rows_dedup > 0:
                risk_flags.add("疑似重复导入")
            if rows_error is not None and rows_error > 0:
                risk_flags.add("存在错误行")
            if error:
                risk_flags.add("字段异常")
            if not last_imported_at:
                last_imported_at = finished_at or created_at or ""
            recent.append({
                "filename": filename,
                "status": status,
                "rows_total": projected_counts.get("rows_total"),
                "rows_imported": rows_imported,
                "rows_dedup": rows_dedup,
                "rows_error": rows_error,
                "rows_skipped_non_data": rows_skipped_non_data,
                "error": project_import_error(error, status=status),
                "created_at": created_at or "",
                "finished_at": finished_at or "",
            })

        status_summary = " ".join([f"{k}{v}" for k, v in status_counts.items() if k])
        return {
            "source_status": "available",
            "recent": recent,
            "status_summary": status_summary,
            "risk_flags": sorted(list(risk_flags)),
            "last_imported_at": last_imported_at,
        }

    def build_case_summary(self, case_id: str) -> Dict[str, Any]:
        info = self.get_case(case_id)
        stats = self.get_case_stats(case_id)
        import_overview = self.get_case_import_overview(case_id, limit=5)
        return {
            "case": info.__dict__ if info else {},
            "stats": stats.__dict__,
            "import_overview": import_overview,
        }

    def add_dataset(self, case_id: str, filename: str, df: pd.DataFrame, kind: Optional[str] = None) -> DatasetInfo:
        case_dir = self.case_dir(case_id)
        raw_dir = ensure_dir(case_dir / "raw")
        ds_dir = ensure_dir(case_dir / "datasets")

        # 清洗
        df = df.copy()
        df.columns = clean_columns(df.columns)
        # 对字符串列做 \t 清理
        for c in df.columns:
            if df[c].dtype == "object":
                df[c] = df[c].map(clean_cell)

        if kind is None:
            kind = detect_kind(filename, df)

        dataset_id = uuid.uuid4().hex[:16]
        stored_path = ds_dir / f"{dataset_id}.csv.gz"
        df.to_csv(stored_path, index=False, compression="gzip")

        info = DatasetInfo(
            dataset_id=dataset_id,
            filename=filename,
            kind=kind,
            rows=int(df.shape[0]),
            cols=int(df.shape[1]),
            imported_at=_now_iso(),
            stored_path=str(stored_path),
        )

        # 写入索引
        self._ensure_case_db(case_id)
        engine = DuckDBEngine(self.case_db(case_id))
        engine.execute(
            "INSERT INTO datasets(dataset_id, filename, kind, rows, cols, imported_at, stored_path) VALUES (?,?,?,?,?,?,?)",
            (info.dataset_id, info.filename, info.kind, info.rows, info.cols, info.imported_at, info.stored_path),
        )

        # Ensure missing persons columns exist when historical datasets are imported.
        try:
            cols = set(_table_columns(engine, "persons"))
            alters = []
            if "role" not in cols:
                alters.append("ALTER TABLE persons ADD COLUMN role TEXT")
            if "source_file" not in cols:
                alters.append("ALTER TABLE persons ADD COLUMN source_file TEXT")
            if "status" not in cols:
                alters.append("ALTER TABLE persons ADD COLUMN status TEXT")
            if "created_at" not in cols:
                alters.append("ALTER TABLE persons ADD COLUMN created_at TEXT")
            if "updated_at" not in cols:
                alters.append("ALTER TABLE persons ADD COLUMN updated_at TEXT")
            for sql in alters:
                try:
                    engine.execute(sql)
                except Exception:
                    pass
        except Exception:
            pass

        engine.close()

        # 更新案件更新时间
        self.touch_case(case_id)
        return info

    def load_dataset_df(self, case_id: str, dataset_id: str) -> pd.DataFrame:
        ds = None
        for d in self.list_datasets(case_id):
            if d.dataset_id == dataset_id:
                ds = d
                break
        if ds is None:
            raise KeyError(dataset_id)
        p = Path(ds.stored_path)
        return pd.read_csv(p, low_memory=False)

    def find_latest_by_kind(self, case_id: str, kind: str) -> Optional[DatasetInfo]:
        for d in self.list_datasets(case_id):
            if d.kind == kind:
                return d
        return None



    # =========================
    # 人员库 & 照片（v5+）
    # =========================

    def upsert_person(
        self,
        case_id: str,
        *,
        name: str = "",
        id_no: str = "",
        role: str = "",
        phone: str = "",
        address: str = "",
        source_filename: str = "",
        source_file: str = "",
        status: str = "已导入",
        source_dataset_id: str = ""
    ) -> str:
        """插入/更新人员，返回 person_id（优先身份证号）。"""
        self._ensure_case_db(case_id)
        person_id = (id_no.strip() if id_no else "").upper()
        if not person_id:
            person_id = uuid.uuid4().hex[:16]
        now = _now_iso()

        engine = DuckDBEngine(self.case_db(case_id))
        exists = bool(engine.query("SELECT 1 FROM persons WHERE person_id=? LIMIT 1", (person_id,)))

        if exists:
            engine.execute(
                """UPDATE persons
                   SET name=?, id_no=?, role=?, phone=?, address=?,
                       source_filename=?, source_file=?, status=?,
                       source_dataset_id=?, updated_at=?
                   WHERE person_id=?""",
                (name, id_no, role, phone, address, source_filename, source_file, status, source_dataset_id, now, person_id),
            )
        else:
            engine.execute(
                """INSERT INTO persons(
                       person_id, name, id_no, role, phone, address,
                       source_filename, source_file, status, source_dataset_id,
                       created_at, updated_at
                   ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)""",
                (person_id, name, id_no, role, phone, address, source_filename, source_file, status, source_dataset_id, now, now),
            )

        engine.close()
        self.touch_case(case_id)
        return person_id

    def set_person_photo(self, case_id: str, person_id: str, photo_path: str, note: str = ""):
        self._ensure_case_db(case_id)
        now = _now_iso()
        engine = DuckDBEngine(self.case_db(case_id))

        engine.execute("DELETE FROM person_photos WHERE person_id=?", (person_id,))
        engine.execute(
            "INSERT INTO person_photos(person_id, photo_path, note, created_at) VALUES (?,?,?,?)",
            (person_id, photo_path, note, now),
        )
        engine.execute("UPDATE persons SET updated_at=? WHERE person_id=?", (now, person_id))
        engine.close()
        self.touch_case(case_id)

    def list_persons(self, case_id: str) -> List[PersonInfo]:
        """列出人员库（附带照片路径）。"""
        self._ensure_case_db(case_id)
        engine = DuckDBEngine(self.case_db(case_id))
        rows = engine.query(
            """SELECT p.person_id,
                      COALESCE(p.name,''),
                      COALESCE(p.id_no,''),
                      COALESCE(p.role,''),
                      COALESCE(p.phone,''),
                      COALESCE(p.address,''),
                      COALESCE(p.source_filename,''),
                      COALESCE(p.source_file,''),
                      COALESCE(p.status,''),
                      COALESCE(pp.photo_path,''),
                      COALESCE(p.created_at,''),
                      COALESCE(p.updated_at,'')
               FROM persons p
               LEFT JOIN person_photos pp ON p.person_id = pp.person_id
               ORDER BY p.updated_at DESC"""
        )
        engine.close()
        return [PersonInfo(*r) for r in rows]

    def get_person(self, case_id: str, person_id: str) -> Optional[PersonInfo]:
        self._ensure_case_db(case_id)
        engine = DuckDBEngine(self.case_db(case_id))
        rows = engine.query(
            """SELECT p.person_id,
                      COALESCE(p.name,''),
                      COALESCE(p.id_no,''),
                      COALESCE(p.role,''),
                      COALESCE(p.phone,''),
                      COALESCE(p.address,''),
                      COALESCE(p.source_filename,''),
                      COALESCE(p.source_file,''),
                      COALESCE(p.status,''),
                      COALESCE(pp.photo_path,''),
                      COALESCE(p.created_at,''),
                      COALESCE(p.updated_at,'')
               FROM persons p
               LEFT JOIN person_photos pp ON p.person_id = pp.person_id
               WHERE p.person_id=?""",
            (person_id,),
        )
        row = rows[0] if rows else None
        engine.close()
        return PersonInfo(*row) if row else None


    def update_person_role(self, case_id: str, person_id: str, role: str):
        self._ensure_case_db(case_id)
        now = _now_iso()
        engine = DuckDBEngine(self.case_db(case_id))
        engine.execute("UPDATE persons SET role=?, updated_at=? WHERE person_id=?", (role, now, person_id))
        engine.close()
        self.touch_case(case_id)

    def link_photos(self, case_id: str, photo_dir: Path) -> Dict[str, int]:
        """按身份证号/姓名匹配照片文件并绑定。"""
        photo_dir = Path(photo_dir)
        if not photo_dir.exists():
            raise FileNotFoundError(str(photo_dir))

        exts = {".jpg", ".jpeg", ".png", ".bmp", ".webp"}
        idx: Dict[str, str] = {}
        total_photos = 0
        for p in photo_dir.rglob("*"):
            if p.is_file() and p.suffix.lower() in exts:
                total_photos += 1
                stem = p.stem.strip().upper()
                idx.setdefault(stem, str(p))

        persons = self.list_persons(case_id)
        matched = 0
        for person in persons:
            key_id = (person.id_no or person.person_id or "").strip().upper()
            key_name = (person.name or "").strip().upper()
            ppath = None
            if key_id and key_id in idx:
                ppath = idx[key_id]
            elif key_name and key_name in idx:
                ppath = idx[key_name]
            if ppath:
                self.set_person_photo(case_id, person.person_id, ppath, note="auto-match")
                matched += 1

        return {"total_photos": total_photos, "persons": len(persons), "matched": matched}


    # ---------------------------
    # Funds-control DB (fc_*) API
    # ---------------------------
    def ensure_funds_tables(self, case_id: str):
        """Create fc_* tables in case.duckdb if missing."""
        self._ensure_case_db(case_id)
        from app.core.fc_import_tables import ensure_fc_tables
        engine = DuckDBEngine(self.case_db(case_id))
        ensure_fc_tables(engine)
        engine.close()

    def list_import_files(self, case_id: str, view: Literal["active", "recycle"] = "active"):
        """Return import_file_log rows for UI."""
        # Keep this UI polling path read-shaped: import/cleaning execution owns
        # fc schema creation, while missing databases/tables are unavailable.
        # Use the shared case-engine opener so active Rust stats workers are
        # released before a foreground import/cleaning read opens DuckDB.
        last_exc: Exception | None = None
        for attempt in range(3):
            engine = None
            try:
                engine = self.open_case_engine(case_id, read_only=True)
                table_name = "import_file_log_recycle" if view == "recycle" else "import_file_log"
                if not _table_exists(engine, table_name):
                    raise CaseSourceUnavailableError("case_import_source_unavailable")
                cols = {
                    r[0]
                    for r in engine.query(
                        "SELECT column_name FROM information_schema.columns "
                        "WHERE table_schema='main' AND table_name=?",
                        (table_name,),
                    )
                }
                if not {"case_id", "file_id"}.issubset(cols):
                    raise CaseSourceUnavailableError("case_import_binding_unavailable")
                foreign_rows = engine.query(
                    f"SELECT COUNT(1) FROM {table_name} WHERE case_id IS NULL OR case_id<>?",
                    (case_id,),
                )
                if (
                    len(foreign_rows) != 1
                    or len(foreign_rows[0]) != 1
                    or type(foreign_rows[0][0]) is not int
                    or foreign_rows[0][0] != 0
                ):
                    raise CaseSourceUnavailableError("case_import_binding_mismatch")
                select_cols = [
                    ("file_id", "file_id"),
                    ("case_id", "case_id"),
                    ("kind", "kind"),
                    ("filename", "filename"),
                    ("display_path", "display_path"),
                    ("stored_path", "stored_path"),
                    ("file_type", "file_type"),
                    ("size", "size"),
                    ("md5", "md5"),
                    ("sha256", "sha256"),
                    ("rows_total", "rows_total"),
                    ("rows_imported", "rows_imported"),
                    ("rows_imported_raw", "rows_imported_raw"),
                    ("rows_imported_norm", "rows_imported_norm"),
                    ("rows_dedup", "rows_dedup"),
                    ("rows_error", "rows_error"),
                    ("rows_skipped_non_data", "rows_skipped_non_data"),
                    ("import_counts_version", "import_counts_version"),
                    ("status", "status"),
                    ("error", "error"),
                    ("cleaned_status", "cleaned_status"),
                    ("cleaned_started_at", "cleaned_started_at"),
                    ("cleaned_finished_at", "cleaned_finished_at"),
                    ("cleaned_error", "cleaned_error"),
                    ("cleaned_rows_affected", "cleaned_rows_affected"),
                    ("cleaning_counts_version", "cleaning_counts_version"),
                    ("created_at", "created_at"),
                    ("finished_at", "finished_at"),
                    ("recycled_at", "recycled_at"),
                ]
                select_exprs = [
                    name if name in cols else f"NULL AS {alias}" for name, alias in select_cols
                ]
                order_by = "recycled_at DESC, created_at DESC" if view == "recycle" else "created_at DESC"
                rows = engine.query(
                    f"""SELECT {', '.join(select_exprs)}
                        FROM {table_name}
                        WHERE case_id=?
                        ORDER BY {order_by}""",
                    (case_id,),
                )
                break
            except Exception as exc:
                last_exc = exc
                from app.core.diagnostic_duckdb_migration import DiagnosticDuckDBMigrationError

                if isinstance(exc, (FileNotFoundError, DiagnosticDuckDBMigrationError)):
                    raise CaseSourceUnavailableError("case_import_source_unavailable") from exc
                if attempt >= 2 or not _is_retryable_duckdb_open_conflict(exc):
                    raise
                time.sleep(0.12 * (attempt + 1))
            finally:
                if engine is not None:
                    engine.close()
        else:
            raise last_exc if last_exc is not None else RuntimeError("list_import_files failed")
        out = []
        for r in rows:
            row_data = dict(zip((alias for _, alias in select_cols), r))
            row_data["error"] = project_import_error(
                row_data.get("error"),
                status=row_data.get("status"),
            )
            row_data["cleaned_error"] = project_cleaning_error(
                row_data.get("cleaned_error"),
                status=row_data.get("cleaned_status"),
            )
            out.append(project_persisted_import_file_log_counts(row_data))
        return out

