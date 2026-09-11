from __future__ import annotations

import csv
import hashlib
import io
import json
import os
import secrets
import shutil
import time
import zipfile
from contextlib import ExitStack
from dataclasses import dataclass, field, replace
from pathlib import Path
from threading import RLock
from typing import Callable, Dict, Iterable, List, Optional, Sequence
from pypdf import PdfReader, PdfWriter

from msoffcrypto import OfficeFile, exceptions as msoffcrypto_exceptions

from app.core.archive_extraction import (
    SUPPORTED_ARCHIVE_EXTS as _SUPPORTED_ARCHIVE_EXTS,
    ArchiveMemberReceiptV1,
    ArchiveExtractionLeaseV1,
    archive_cache_dir_name,
    archive_cache_output_name,
    archive_entry_leaf_name,
    archive_path_identity,
    combine_archive_path,
    open_archive_extraction_v1,
)
from app.core.db_engine import DuckDBEngine
from app.core.failure_boundary import (
    IMPORT_ARCHIVE_PREVIEW_FAILED,
    IMPORT_FILE_CANCELLED,
    IMPORT_FILE_PROCESSING_FAILED,
    IMPORT_FILE_REGISTERED,
    IMPORT_FILE_WARNING,
    private_exception_is_retryable,
    project_import_note,
)
from app.core.fc_group_importer import FcGroupImportSource, import_fc_csv_group_into_db
from app.core.import_accelerator import (
    CsvColumnProfiles,
    ImportAcceleratorUnavailableError,
    PrepareCsvBatchItem,
    PrepareCsvWithProfileBatchItem,
    PreparedCsv,
    PreparedCsvWithProfile,
    excel_to_csv,
    prepare_csv,
    prepare_csv_batch,
    prepare_csv_with_profile,
    prepare_csv_with_profile_batch,
    split_excel_account_sections,
)
from app.core.immutable_generation import (
    ImmutableGeneration,
    ImmutableGenerationError,
    RegularSourceReceipt,
    ensure_private_generation_root,
    inspect_authorized_generation,
    inspect_private_generation_root,
    inspect_published_generation,
    inspect_regular_source,
    open_verified_source,
    open_verified_text,
    private_work_directory,
    publish_generated,
    publish_json,
    publish_source,
    read_immutable_bytes,
    read_published_generation_bytes,
)
from app.core.import_preview_profile import ImportPreviewProfiler
from app.core.field_mapping_transform_rules import field_mapping_transform_messages
from app.core.field_mapping_suggester import (
    required_missing_for_mapping,
    suggest_field_mapping,
    validate_field_mapping_candidate,
)
from app.core.fc_import_bank_name import derive_bank_name_from_file_name, derived_bank_headers_for_kind
from app.core.fc_import_file_log import update_import_progress, upsert_import_file_log
from app.core.fc_import_file_identity import (
    detect_fc_kind,
    detect_file_encoding,
    file_hash,
    read_csv_headers,
)
from app.core.fc_import_privacy_delta import append_privacy_projection_delta_for_case_rows
from app.core.fc_import_projection import normalize_header_for_match
from app.core.fc_import_schema import FC_SCHEMAS, HEADER_ALIASES_BY_TABLE, norm_table_name, raw_table_name
from app.core.fc_import_tables import ensure_fc_tables
from app.core.fc_importer import import_fc_csv_into_db
from app.core.import_count_semantics import IMPORT_COUNTS_VERSION, MAX_PUBLIC_IMPORT_COUNT
from app.core.import_mapping_template_store import ImportMappingTemplateStore
from app.core.storage import CaseStorage
from app.domain.controlled_artifact_gate import require_controlled_source_ingestion
from app.repositories.analysis_revision import bump_stats_flow_source_revision
from app.repositories.document_repository import DocumentRepository
from app.repositories.txn_daily_aggregate import TxnDailyAggregateStore


_ZIP_GARBAGE_CHARS = set("╔╧║╞╓╢½╖╒╣╥°╨▌╙▐╦╛╜╗╫├≈┼")
_SUPPORTED_DATA_EXTS = {".csv", ".xlsx", ".xls"}
_SUPPORTED_DOCUMENT_EXTS = {".pdf", ".doc", ".docx", ".txt", ".md"}
_SUPPORTED_IMPORT_EXTS = _SUPPORTED_DATA_EXTS | _SUPPORTED_DOCUMENT_EXTS | _SUPPORTED_ARCHIVE_EXTS
_SUPPORT_FILE_KIND = "support_file"
_MAX_NESTED_ZIP_DEPTH = 4
_MAX_NESTED_ARCHIVE_DEPTH = _MAX_NESTED_ZIP_DEPTH
_MAX_NESTED_ZIP_BYTES = 64 * 1024 * 1024
_MAX_ZIP_MEMBER_BYTES = 512 * 1024 * 1024
_EXCEL_IMPORT_HEADER_SCAN_ROWS = 25
_EXCEL_IMPORT_HEADER_MIN_MATCHED = 3
_EXCEL_IMPORT_HEADER_MIN_SCORE = 30


def _require_import_count(value: object, *, field: str) -> int:
    if type(value) is not int or value < 0 or value > MAX_PUBLIC_IMPORT_COUNT:
        raise ValueError(f"import_{field}_invalid")
    return value

_IMPORT_REQUIRED_HEADERS_BY_KIND = {
    "fc_transaction": {"交易账号", "交易时间", "交易金额"},
    "fc_account": {"交易账号"},
    "fc_person": {"证照号码"},
    "fc_sub_account": {"银行名称", "开户账号", "子账户账号"},
    "fc_person_address": {"开户名称", "证照号码", "住宅地址"},
    "fc_person_contact": {"开户名称", "联系电话"},
    "fc_coercive_measure": {"银行名称", "账号", "冻结措施类型"},
}

_KIND_CN = {
    "fc_account": "账户信息",
    "fc_person": "人员信息",
    "fc_coercive_measure": "强制措施",
    "fc_transaction": "交易明细",
    "fc_sub_account": "关联子账户",
    "fc_person_address": "人员住址",
    "fc_person_contact": "人员联系方式",
    "fc_task_success": "任务信息（成功）",
    "fc_task_fail": "任务信息（失败）",
    _SUPPORT_FILE_KIND: "研判文件",
}


@dataclass
class ImportFileInput:
    file_name: str
    source_path: str
    file_kind: Optional[str] = None
    password: Optional[str] = None
    expected_sha256: Optional[str] = None
    expected_size: Optional[int] = None
    field_mapping: Optional[Dict[str, str]] = None
    field_mapping_origins: Optional[Dict[str, str]] = None
    archive_items: Optional[List["ImportArchiveItemInput"]] = None


@dataclass
class ImportArchiveItemInput:
    archive_path: str
    file_kind: Optional[str] = None
    expected_sha256: Optional[str] = None
    expected_size: Optional[int] = None
    field_mapping: Optional[Dict[str, str]] = None
    field_mapping_origins: Optional[Dict[str, str]] = None


def _archive_override_map(
    archive_items: Optional[List[ImportArchiveItemInput]],
) -> Dict[str, ImportArchiveItemInput]:
    overrides: Dict[str, ImportArchiveItemInput] = {}
    for item in archive_items or []:
        identity = archive_path_identity(item.archive_path)
        if identity in overrides:
            raise ValueError(f"duplicate archive entry override: {identity}")
        digest = str(item.expected_sha256 or "").strip().upper()
        if len(digest) != 64 or any(character not in "0123456789ABCDEF" for character in digest):
            raise ValueError(f"archive entry receipt invalid: {identity}")
        if isinstance(item.expected_size, bool) or item.expected_size is None or int(item.expected_size) < 0:
            raise ValueError(f"archive entry receipt invalid: {identity}")
        item.expected_sha256 = digest
        item.expected_size = int(item.expected_size)
        overrides[identity] = item
    return overrides


@dataclass
class PreparedImportFile:
    file_id: str
    display_name: str
    display_path: str
    real_path: Path
    file_type: str
    size: int
    rows_total: int
    kind_hint: Optional[str] = None
    field_mapping: Optional[Dict[str, str]] = None
    field_mapping_origins: Optional[Dict[str, str]] = None
    source_sha256: str = ""
    source_size: int = 0
    # sha256 always identifies the exact consumed immutable generation. The
    # original source identity is retained separately above for lineage.
    sha256: str = ""
    csv_encoding: str = ""
    duckdb_csv_path: Optional[Path] = None
    duckdb_csv_encoding: str = ""
    csv_headers: List[str] = field(default_factory=list)
    ingestion_receipt: Dict[str, object] = field(default_factory=dict)
    duckdb_csv_receipt: Dict[str, object] = field(default_factory=dict)


@dataclass
class ImportExecutionResult:
    file_id: str
    display_name: str
    display_path: str
    file_type: str
    size: int
    md5: str
    sha256: str
    kind: str
    status: str
    rows_total: Optional[int]
    rows_seen: Optional[int]
    rows_imported_raw: Optional[int]
    rows_imported_norm: Optional[int]
    rows_dedup: Optional[int]
    rows_error: Optional[int]
    note: str
    error: str
    attempts: int
    timings: Dict[str, object] = field(default_factory=dict)
    rows_skipped_non_data: Optional[int] = None
    retryable: bool = False


@dataclass
class ImportFilePreview:
    file_name: str
    source_path: str
    file_type: str
    size: int
    rows_total: int
    columns_total: int
    header_preview: List[str]
    sample_rows: List[List[str]]
    domain_category: str
    suggested_kind: str
    suggested_kind_label: str
    status: str
    issue: str
    accepts_password: bool
    requires_password: bool
    detected_by: str
    archive_children: List["ImportArchivePreview"]
    sha256: str = ""
    field_mapping: Dict[str, str] = field(default_factory=dict)
    field_mapping_origins: Dict[str, str] = field(default_factory=dict)
    mapping_status: str = ""
    mapping_method: str = ""
    mapping_message: str = ""
    mapping_required_missing: List[str] = field(default_factory=list)


@dataclass
class ImportArchivePreview:
    file_name: str
    archive_path: str
    file_type: str
    size: int
    rows_total: int
    columns_total: int
    header_preview: List[str]
    sample_rows: List[List[str]]
    domain_category: str
    suggested_kind: str
    suggested_kind_label: str
    status: str
    issue: str
    detected_by: str
    sha256: str = ""
    field_mapping: Dict[str, str] = field(default_factory=dict)
    field_mapping_origins: Dict[str, str] = field(default_factory=dict)
    mapping_status: str = ""
    mapping_method: str = ""
    mapping_message: str = ""
    mapping_required_missing: List[str] = field(default_factory=list)


@dataclass(frozen=True)
class _CsvTabularPreviewRequest:
    path: Path
    label: str
    limit: int
    encoding: Optional[str] = None


@dataclass(frozen=True)
class _PreparedCsvProfilePreview:
    value: PreparedCsvWithProfile
    elapsed_s: float


@dataclass(frozen=True)
class _CsvTabularPreviewResult:
    rows_total: int
    columns_total: int
    headers: List[str]
    sample_rows: List[List[str]]
    profiles: Optional[CsvColumnProfiles]


@dataclass(frozen=True)
class _ExcelImportHeaderCandidate:
    row_index: int
    kind: str
    headers: List[str]
    matched_targets: int
    required_targets: int
    score: int


@dataclass(frozen=True)
class _ArchiveCsvPreviewBatchItem:
    preview_path: Path
    archive_path: str
    source_archive_path: Path
    kind_hint: str
    sha256: str
    size: int


@dataclass(frozen=True)
class _VerifiedArchiveMemberGeneration:
    logical_path: str
    member: ArchiveMemberReceiptV1
    generation: ImmutableGeneration
    source_archive_path: Path


class ImportRepository:
    """Repository adapter over persisted import storage and document state."""

    _prepared_generation_registry_bootstrap_lock = RLock()

    def __init__(self, *, document_repository: Optional[DocumentRepository] = None) -> None:
        self._storage = CaseStorage()
        self._document_repository = document_repository or DocumentRepository()
        self._daily_agg = TxnDailyAggregateStore(self._storage)
        self._preview_manifest_generation_registry: Dict[str, Dict[str, object]] = {}
        self._preview_manifest_generation_registry_lock = RLock()
        self._prepared_generation_registry: Dict[str, str] = {}
        self._prepared_generation_registry_lock = RLock()

    @property
    def storage(self) -> CaseStorage:
        return self._storage

    def case_exists(self, case_id: str) -> bool:
        return self._storage.get_case(case_id) is not None

    def open_case_engine(self, case_id: str) -> DuckDBEngine:
        return self._storage.open_case_engine(case_id)

    def list_import_files(self, case_id: str, view: str = "active") -> List[dict]:
        return self._storage.list_import_files(case_id, view=view if view == "recycle" else "active")

    def list_historical_datasets(self, case_id: str) -> List[dict]:
        return [item.__dict__.copy() for item in self._storage.list_datasets(case_id)]

    def get_case_paths(self, case_id: str) -> dict:
        require_controlled_source_ingestion()
        db_path = self._storage.case_db(case_id)
        case_dir = self._storage.case_dir(case_id)
        raw_dir = case_dir / "raw"
        return {
            "case_id": case_id,
            "case_dir": str(case_dir),
            "db_path": str(db_path),
            "db_dir": str(db_path.parent),
            "raw_dir": str(raw_dir),
        }

    def ensure_tables(self, engine: DuckDBEngine, *, include_secondary_indexes: bool = True) -> None:
        ensure_fc_tables(engine, include_secondary_indexes=include_secondary_indexes)

    def refresh_case_stats(self, case_id: str, *, engine: Optional[DuckDBEngine] = None) -> None:
        self._storage.compute_case_stats(case_id, engine=engine)

    def validate_import_persistence(
        self,
        case_id: str,
        *,
        file_ids: Sequence[str],
        engine: Optional[DuckDBEngine] = None,
    ) -> dict:
        normalized_ids = list(
            dict.fromkeys([str(file_id or "").strip() for file_id in file_ids if str(file_id or "").strip()])
        )
        if not case_id:
            raise ValueError("case_id required")
        if not normalized_ids:
            return {
                "case_id": case_id,
                "file_ids": [],
                "import_file_log_count": 0,
                "rows_imported_norm": 0,
                "norm_rows_by_kind": {},
            }

        owns_engine = engine is None
        active_engine = engine if engine is not None else self._storage.open_case_engine(case_id)
        try:
            placeholders = self._sql_placeholders(len(normalized_ids))
            params: List[object] = [case_id, *normalized_ids]
            import_file_log_count = 0
            rows_imported_norm = 0
            kinds_by_file: dict[str, str] = {}

            if not self._table_exists(active_engine, "import_file_log"):
                raise RuntimeError("import persistence manifest is unavailable")
            rows = active_engine.query(
                f"""SELECT file_id, kind, rows_imported_norm, import_counts_version, status, cleaned_status
                    FROM import_file_log
                    WHERE case_id=? AND file_id IN ({placeholders})""",
                params,
            )
            import_file_log_count = len(rows)
            seen_file_ids: set[str] = set()
            for file_id, kind, rows_norm, import_counts_version, status, cleaned_status in rows:
                normalized_file_id = str(file_id or "").strip()
                normalized_kind = str(kind or "").strip()
                normalized_status = str(status or "").strip()
                normalized_cleaned_status = str(cleaned_status or "").strip().lower()
                if (
                    not normalized_file_id
                    or normalized_file_id not in normalized_ids
                    or normalized_file_id in seen_file_ids
                    or normalized_kind not in {*FC_SCHEMAS.keys(), _SUPPORT_FILE_KIND}
                    or type(import_counts_version) is not int
                    or import_counts_version != IMPORT_COUNTS_VERSION
                    or normalized_status != "已完成"
                    or normalized_cleaned_status not in {"pending", "done"}
                    or isinstance(rows_norm, bool)
                    or not isinstance(rows_norm, int)
                    or rows_norm < 0
                ):
                    raise RuntimeError("import persistence manifest is incomplete")
                seen_file_ids.add(normalized_file_id)
                kinds_by_file[normalized_file_id] = normalized_kind
                rows_imported_norm += rows_norm
            if seen_file_ids != set(normalized_ids):
                raise RuntimeError("import persistence manifest is incomplete")

            norm_rows_by_kind: dict[str, int] = {}
            for kind, schema in FC_SCHEMAS.items():
                candidate_ids = [file_id for file_id in normalized_ids if kinds_by_file.get(file_id) == kind]
                if not candidate_ids:
                    continue
                norm_table = norm_table_name(schema.table)
                if not self._table_exists(active_engine, norm_table):
                    raise RuntimeError("import persistence normalized source is unavailable")
                kind_placeholders = self._sql_placeholders(len(candidate_ids))
                count_rows = active_engine.query(
                    f"SELECT COUNT(1) FROM {norm_table} WHERE case_id=? AND file_id IN ({kind_placeholders})",
                    [case_id, *candidate_ids],
                )
                if (
                    len(count_rows) != 1
                    or isinstance(count_rows[0][0], bool)
                    or not isinstance(count_rows[0][0], int)
                    or count_rows[0][0] < 0
                ):
                    raise RuntimeError("import persistence normalized count is unavailable")
                norm_rows_by_kind[kind] = count_rows[0][0]

            return {
                "case_id": case_id,
                "file_ids": normalized_ids,
                "import_file_log_count": import_file_log_count,
                "rows_imported_norm": rows_imported_norm,
                "norm_rows_by_kind": norm_rows_by_kind,
            }
        finally:
            if owns_engine:
                try:
                    active_engine.close()
                except Exception:
                    pass

    def mark_analysis_dirty(self, *, engine: DuckDBEngine, reason: str) -> None:
        try:
            bump_stats_flow_source_revision(engine, reason=reason)
        except Exception:
            pass

    def refresh_analysis_aggregates(
        self,
        case_id: str,
        *,
        engine: Optional[DuckDBEngine] = None,
        profile_cb: Optional[Callable[[dict], None]] = None,
    ) -> None:
        materialized = self._daily_agg.ensure_materialized(case_id, force=True, engine=engine, profile_cb=profile_cb)
        if not materialized:
            raise RuntimeError("analysis materialization refresh failed")

    @staticmethod
    def _table_columns(engine: DuckDBEngine, table: str) -> set[str]:
        try:
            return {
                str(row[0] or "")
                for row in engine.query(
                    "SELECT column_name FROM information_schema.columns "
                    "WHERE table_schema='main' AND table_name=?",
                    (table,),
                )
                if row and row[0]
            }
        except Exception:
            return set()

    @classmethod
    def _table_exists(cls, engine: DuckDBEngine, table: str) -> bool:
        return bool(cls._table_columns(engine, table))

    @staticmethod
    def _sql_placeholders(size: int) -> str:
        return ", ".join(["?"] * max(0, int(size)))

    @staticmethod
    def _recycle_table_name(table: str) -> str:
        return f"{table}_recycle"

    @classmethod
    def _table_columns_with_types(cls, engine: DuckDBEngine, table: str) -> List[tuple[str, str]]:
        if not cls._table_exists(engine, table):
            return []
        rows = engine.query(
            "SELECT column_name, data_type "
            "FROM information_schema.columns "
            "WHERE table_schema='main' AND table_name=? "
            "ORDER BY ordinal_position",
            (table,),
        )
        return [(str(row[0] or ""), str(row[1] or "TEXT")) for row in rows if row and row[0]]

    @staticmethod
    def _now_text() -> str:
        return time.strftime("%Y-%m-%d %H:%M:%S")

    @staticmethod
    def _is_within_root(path: Path, root: Path) -> bool:
        try:
            path.resolve().relative_to(root.resolve())
            return True
        except Exception:
            return False

    def _ensure_shadow_table(
        self,
        engine: DuckDBEngine,
        *,
        source_table: str,
        shadow_table: str,
        extra_columns: Sequence[tuple[str, str]] = (("recycled_at", "TEXT"),),
    ) -> bool:
        source_columns = self._table_columns_with_types(engine, source_table)
        if not source_columns:
            return self._table_exists(engine, shadow_table)

        if not self._table_exists(engine, shadow_table):
            select_exprs = [name for name, _ in source_columns]
            for extra_name, extra_type in extra_columns:
                select_exprs.append(f"CAST(NULL AS {extra_type}) AS {extra_name}")
            engine.execute(
                f"CREATE TABLE IF NOT EXISTS {shadow_table} AS "
                f"SELECT {', '.join(select_exprs)} FROM {source_table} WHERE 1=0"
            )

        shadow_columns = self._table_columns(engine, shadow_table)
        for name, column_type in source_columns:
            if name not in shadow_columns:
                engine.execute(f"ALTER TABLE {shadow_table} ADD COLUMN {name} {column_type}")
        shadow_columns = self._table_columns(engine, shadow_table)
        for extra_name, extra_type in extra_columns:
            if extra_name not in shadow_columns:
                engine.execute(f"ALTER TABLE {shadow_table} ADD COLUMN {extra_name} {extra_type}")
        return True

    def _ensure_table_from_shadow(
        self,
        engine: DuckDBEngine,
        *,
        target_table: str,
        shadow_table: str,
        exclude_columns: Sequence[str] = ("recycled_at",),
    ) -> bool:
        if self._table_exists(engine, target_table):
            return True
        shadow_columns = [
            (name, column_type)
            for name, column_type in self._table_columns_with_types(engine, shadow_table)
            if name not in set(exclude_columns)
        ]
        if not shadow_columns:
            return False
        defs = ", ".join(f"{name} {column_type}" for name, column_type in shadow_columns)
        engine.execute(f"CREATE TABLE IF NOT EXISTS {target_table}({defs})")
        return True

    @staticmethod
    def _count_matching_rows(engine: DuckDBEngine, *, table: str, where_clause: str, params: Sequence[object]) -> int:
        rows = engine.query(f"SELECT COUNT(*) FROM {table} WHERE {where_clause}", params)
        if len(rows) != 1 or len(rows[0]) != 1:
            raise RuntimeError("import_count_query_invalid")
        return _require_import_count(rows[0][0], field="matching_rows")

    def _move_rows_to_shadow(
        self,
        engine: DuckDBEngine,
        *,
        source_table: str,
        shadow_table: str,
        where_clause: str,
        params: Sequence[object],
        recycled_at: str,
    ) -> int:
        if not self._table_exists(engine, source_table):
            return 0
        self._ensure_shadow_table(engine, source_table=source_table, shadow_table=shadow_table)
        count = self._count_matching_rows(engine, table=source_table, where_clause=where_clause, params=params)
        if count <= 0:
            return 0

        source_columns = [name for name, _ in self._table_columns_with_types(engine, source_table)]
        shadow_columns = set(self._table_columns(engine, shadow_table))
        common_columns = [name for name in source_columns if name in shadow_columns]
        if not common_columns:
            return 0
        insert_columns = list(common_columns)
        select_exprs = list(common_columns)
        insert_params: List[object] = list(params)
        if "recycled_at" in shadow_columns:
            insert_columns.append("recycled_at")
            select_exprs.append("?")
            insert_params = [recycled_at, *insert_params]

        engine.execute(
            f"INSERT INTO {shadow_table} ({', '.join(insert_columns)}) "
            f"SELECT {', '.join(select_exprs)} FROM {source_table} WHERE {where_clause}",
            insert_params,
        )
        engine.execute(f"DELETE FROM {source_table} WHERE {where_clause}", params)
        return count

    def _restore_rows_from_shadow(
        self,
        engine: DuckDBEngine,
        *,
        target_table: str,
        shadow_table: str,
        where_clause: str,
        params: Sequence[object],
    ) -> int:
        if not self._table_exists(engine, shadow_table):
            return 0
        self._ensure_table_from_shadow(engine, target_table=target_table, shadow_table=shadow_table)
        if not self._table_exists(engine, target_table):
            return 0
        count = self._count_matching_rows(engine, table=shadow_table, where_clause=where_clause, params=params)
        if count <= 0:
            return 0
        target_columns = [name for name, _ in self._table_columns_with_types(engine, target_table)]
        shadow_columns = set(self._table_columns(engine, shadow_table))
        common_columns = [name for name in target_columns if name in shadow_columns and name != "recycled_at"]
        if not common_columns:
            return 0
        engine.execute(
            f"INSERT INTO {target_table} ({', '.join(common_columns)}) "
            f"SELECT {', '.join(common_columns)} FROM {shadow_table} WHERE {where_clause}",
            params,
        )
        engine.execute(f"DELETE FROM {shadow_table} WHERE {where_clause}", params)
        return count

    def _delete_rows_from_table(
        self,
        engine: DuckDBEngine,
        *,
        table: str,
        where_clause: str,
        params: Sequence[object],
    ) -> int:
        if not self._table_exists(engine, table):
            return 0
        count = self._count_matching_rows(engine, table=table, where_clause=where_clause, params=params)
        if count <= 0:
            return 0
        engine.execute(f"DELETE FROM {table} WHERE {where_clause}", params)
        return count

    def _append_privacy_projection_delta_for_files(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        table: str,
        file_ids: Sequence[str],
        op: str,
        source: str,
    ) -> int:
        normalized_ids = [str(file_id or "").strip() for file_id in file_ids if str(file_id or "").strip()]
        if not normalized_ids or not self._table_exists(engine, table):
            return 0
        placeholders = self._sql_placeholders(len(normalized_ids))
        return append_privacy_projection_delta_for_case_rows(
            engine,
            case_id=case_id,
            op=op,
            table=table,
            where_clause=f"case_id=? AND file_id IN ({placeholders})",
            params=[case_id, *normalized_ids],
            source=source,
        )

    def _resolve_dataset_ids_for_delete(
        self,
        engine: DuckDBEngine,
        *,
        targets: Sequence[tuple[str, str, str, str, str]],
        table: str = "datasets",
    ) -> List[str]:
        if not targets or not self._table_exists(engine, table):
            return []

        dataset_rows = engine.query(f"SELECT dataset_id, filename, kind, stored_path FROM {table}")
        ids_by_dataset_id: Dict[str, str] = {}
        ids_by_path: Dict[str, List[str]] = {}
        ids_by_signature: Dict[tuple[str, str], List[str]] = {}

        for dataset_id, filename, kind, stored_path in dataset_rows:
            normalized_dataset_id = str(dataset_id or "").strip()
            if not normalized_dataset_id:
                continue
            normalized_filename = str(filename or "").strip()
            normalized_kind = str(kind or "").strip()
            normalized_path = str(stored_path or "").strip()
            ids_by_dataset_id[normalized_dataset_id] = normalized_dataset_id
            if normalized_path:
                ids_by_path.setdefault(normalized_path, []).append(normalized_dataset_id)
            ids_by_signature.setdefault((normalized_filename, normalized_kind), []).append(normalized_dataset_id)

        resolved: List[str] = []
        for file_id, filename, kind, display_path, stored_path in targets:
            normalized_file_id = str(file_id or "").strip()
            if normalized_file_id and normalized_file_id in ids_by_dataset_id:
                resolved.append(normalized_file_id)
                continue

            path_candidates = {
                str(display_path or "").strip(),
                str(stored_path or "").strip(),
            }
            path_matches = {
                candidate_id
                for path in path_candidates
                if path
                for candidate_id in ids_by_path.get(path, [])
            }
            if len(path_matches) == 1:
                resolved.extend(path_matches)
                continue

            signature_matches = ids_by_signature.get((str(filename or "").strip(), str(kind or "").strip()), [])
            if len(signature_matches) == 1:
                resolved.append(signature_matches[0])

        return list(dict.fromkeys([dataset_id for dataset_id in resolved if dataset_id]))

    def _resolve_cleaning_log_ids(
        self,
        engine: DuckDBEngine,
        *,
        table: str,
        case_id: str,
        file_ids: Sequence[str],
    ) -> List[str]:
        if not file_ids or not self._table_exists(engine, table):
            return []
        columns = self._table_columns(engine, table)
        if "id" not in columns or "case_id" not in columns or "file_id" not in columns:
            return []
        placeholders = self._sql_placeholders(len(file_ids))
        rows = engine.query(
            f"SELECT id FROM {table} WHERE case_id=? AND file_id IN ({placeholders})",
            [case_id, *file_ids],
        )
        return [str(row[0] or "") for row in rows if row and str(row[0] or "").strip()]

    def _ensure_recycle_tables(self, engine: DuckDBEngine) -> None:
        ensure_fc_tables(engine)
        self._document_repository.ensure_tables(engine)
        self._ensure_shadow_table(
            engine,
            source_table="import_file_log",
            shadow_table=self._recycle_table_name("import_file_log"),
        )
        for schema in FC_SCHEMAS.values():
            for table in (raw_table_name(schema.table), norm_table_name(schema.table)):
                self._ensure_shadow_table(
                    engine,
                    source_table=table,
                    shadow_table=self._recycle_table_name(table),
                )
        for table in ("datasets", "cleaning_log", "cleaning_log_detail", "document_assets", "document_chunks"):
            if self._table_exists(engine, table):
                self._ensure_shadow_table(
                    engine,
                    source_table=table,
                    shadow_table=self._recycle_table_name(table),
                )

    def _list_content_paths_for_file_ids(self, engine: DuckDBEngine, *, case_id: str, table: str, file_ids: Sequence[str]) -> List[str]:
        if not file_ids or not self._table_exists(engine, table):
            return []
        columns = set(self._table_columns(engine, table))
        if "content_path" not in columns or "file_id" not in columns:
            return []
        placeholders = self._sql_placeholders(len(file_ids))
        params: List[object] = [*file_ids]
        where = f"file_id IN ({placeholders})"
        if "case_id" in columns:
            where = f"case_id=? AND {where}"
            params = [case_id, *params]
        rows = engine.query(
            f"SELECT DISTINCT content_path FROM {table} WHERE {where}",
            params,
        )
        return [str(row[0] or "").strip() for row in rows if row and str(row[0] or "").strip()]

    def _list_dataset_paths(self, engine: DuckDBEngine, *, table: str, dataset_ids: Sequence[str]) -> List[str]:
        if not dataset_ids or not self._table_exists(engine, table):
            return []
        columns = set(self._table_columns(engine, table))
        if "stored_path" not in columns or "dataset_id" not in columns:
            return []
        placeholders = self._sql_placeholders(len(dataset_ids))
        rows = engine.query(
            f"SELECT DISTINCT stored_path FROM {table} WHERE dataset_id IN ({placeholders})",
            dataset_ids,
        )
        return [str(row[0] or "").strip() for row in rows if row and str(row[0] or "").strip()]

    def _table_has_path_reference(self, engine: DuckDBEngine, *, table: str, column: str, path_value: str) -> bool:
        if not path_value or not self._table_exists(engine, table):
            return False
        if column not in set(self._table_columns(engine, table)):
            return False
        return bool(engine.query(f"SELECT 1 FROM {table} WHERE {column}=? LIMIT 1", (path_value,)))

    def _cleanup_paths_if_unreferenced(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        paths: Sequence[str],
        allowed_root: Path,
        references: Sequence[tuple[str, str]],
        delete_callback: Optional[Callable[[str], None]] = None,
    ) -> None:
        unique_paths = list(dict.fromkeys([str(path or "").strip() for path in paths if str(path or "").strip()]))
        for path_value in unique_paths:
            if any(self._table_has_path_reference(engine, table=table, column=column, path_value=path_value) for table, column in references):
                continue
            target = Path(path_value).expanduser()
            if not self._is_within_root(target, allowed_root):
                continue
            try:
                if delete_callback is not None:
                    delete_callback(path_value)
                else:
                    target.unlink(missing_ok=True)
            except Exception:
                pass

    def _delete_file_cleaning_logs(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        file_ids: Sequence[str],
    ) -> None:
        if not file_ids:
            return

        placeholders = self._sql_placeholders(len(file_ids))
        params = [case_id, *file_ids]

        if self._table_exists(engine, "cleaning_log"):
            cleaning_log_columns = self._table_columns(engine, "cleaning_log")
            if "case_id" in cleaning_log_columns and "file_id" in cleaning_log_columns:
                engine.execute(
                    f"DELETE FROM cleaning_log WHERE case_id=? AND file_id IN ({placeholders})",
                    params,
                )

        if not self._table_exists(engine, "cleaning_log_detail"):
            return

        detail_columns = self._table_columns(engine, "cleaning_log_detail")
        if "case_id" in detail_columns and "file_id" in detail_columns:
            engine.execute(
                f"DELETE FROM cleaning_log_detail WHERE case_id=? AND file_id IN ({placeholders})",
                params,
            )
            return

        if "case_id" in detail_columns and "log_id" in detail_columns and self._table_exists(engine, "cleaning_log"):
            engine.execute(
                "DELETE FROM cleaning_log_detail "
                "WHERE case_id=? AND log_id NOT IN (SELECT id FROM cleaning_log WHERE case_id=?)",
                (case_id, case_id),
            )

    def preview_files(
        self,
        case_id: str,
        files: Iterable[ImportFileInput],
        profile_cb: Optional[Callable[[dict[str, object]], None]] = None,
    ) -> List[ImportFilePreview]:
        require_controlled_source_ingestion()
        verified_items: List[tuple[ImportFileInput, Path, RegularSourceReceipt]] = []
        for item in list(files):
            src_path = Path(item.source_path).expanduser()
            if not src_path.exists():
                raise FileNotFoundError(str(src_path))
            if not src_path.is_file():
                raise ValueError(f"not a file: {src_path}")
            verified_items.append((item, src_path, inspect_regular_source(src_path)))
        if not verified_items:
            return []

        preview_source_root = self._storage.case_dir(case_id) / "raw" / "_preview_sources"
        ensure_private_generation_root(preview_source_root)
        previews: List[ImportFilePreview] = []
        profiler = ImportPreviewProfiler() if profile_cb else None
        for item, src_path, source_receipt in verified_items:
            preview_generation = publish_source(
                src_path,
                preview_source_root,
                expected=source_receipt,
                expected_sha256=source_receipt.sha256,
                expected_size=source_receipt.size,
                suffix=src_path.suffix,
                lineage={
                    "kind": "case_bound_import_preview_source",
                    "case_id": str(case_id),
                    "derived": False,
                    "source_sha256": source_receipt.sha256,
                    "encrypted_source": "unknown",
                },
            )
            if profiler:
                profiler.count("source_files")
                with profiler.time_phase("source_file_s"):
                    preview = self._preview_file(
                        case_id,
                        preview_generation.path,
                        display_name=item.file_name or src_path.name,
                        kind_hint=item.file_kind,
                        password=item.password,
                        profiler=profiler,
                    )
            else:
                preview = self._preview_file(
                    case_id,
                    preview_generation.path,
                    display_name=item.file_name or src_path.name,
                    kind_hint=item.file_kind,
                    password=item.password,
                )
            # The UI retains the user-selected path, while all preview bytes,
            # hashes and samples came from the immutable case-bound generation.
            preview.source_path = str(src_path)
            preview.sha256 = source_receipt.sha256
            preview.size = source_receipt.size
            previews.append(preview)
        if profile_cb and profiler:
            profile_cb(profiler.as_dict())
        return previews

    def prepare_files(self, case_id: str, files: Iterable[ImportFileInput]) -> List[PreparedImportFile]:
        require_controlled_source_ingestion()
        verified_items: List[tuple[ImportFileInput, Path, RegularSourceReceipt]] = []
        for item in list(files):
            src_path = Path(item.source_path).expanduser()
            if not str(item.expected_sha256 or "").strip() or item.expected_size is None:
                raise ValueError("controlled source receipt with SHA-256 and size is required")
            try:
                source_receipt = inspect_regular_source(
                    src_path,
                    expected_sha256=item.expected_sha256,
                    expected_size=item.expected_size,
                )
            except ImmutableGenerationError as exc:
                label = item.file_name or src_path.name or "source"
                raise ValueError(f"{label} 在预检后已发生变更，请重新执行 SHA-256 检验后再导入。") from exc
            verified_items.append((item, src_path, source_receipt))
        if not verified_items:
            raise ValueError("no importable files prepared")

        raw_dir = self._storage.case_dir(case_id) / "raw"
        ensure_private_generation_root(raw_dir)

        prepared: List[PreparedImportFile] = []
        for item, src_path, source_receipt in verified_items:
            source_sha256 = source_receipt.sha256

            ext = src_path.suffix.lower()
            if ext in _SUPPORTED_ARCHIVE_EXTS:
                if (item.file_kind or "").strip() == _SUPPORT_FILE_KIND:
                    prepared.extend(
                        self._prepare_from_support_file(
                            case_id,
                            src_path,
                            display_name=item.file_name or src_path.name,
                            source_label=item.source_path,
                            kind_hint=item.file_kind,
                            password=item.password,
                            field_mapping=item.field_mapping,
                            field_mapping_origins=item.field_mapping_origins,
                            verified_sha256=source_sha256,
                            verified_source=source_receipt,
                        )
                    )
                else:
                    if ext == ".zip":
                        prepared.extend(
                            self._prepare_from_zip(
                                case_id,
                                src_path,
                                file_name=item.file_name,
                                kind_hint=item.file_kind,
                                password=item.password,
                                field_mapping=item.field_mapping,
                                field_mapping_origins=item.field_mapping_origins,
                                archive_items=item.archive_items,
                                verified_archive_sha256=source_sha256,
                                verified_source=source_receipt,
                            )
                        )
                    else:
                        prepared.extend(
                            self._prepare_from_archive(
                                case_id,
                                src_path,
                                file_name=item.file_name,
                                kind_hint=item.file_kind,
                                password=item.password,
                                field_mapping=item.field_mapping,
                                field_mapping_origins=item.field_mapping_origins,
                                archive_items=item.archive_items,
                                verified_archive_sha256=source_sha256,
                                verified_source=source_receipt,
                            )
                        )
                continue
            if ext in _SUPPORTED_DOCUMENT_EXTS:
                prepared.extend(
                    self._prepare_from_support_file(
                        case_id,
                        src_path,
                        display_name=item.file_name or src_path.name,
                        source_label=item.source_path,
                        kind_hint=item.file_kind,
                        password=item.password,
                        field_mapping=item.field_mapping,
                        field_mapping_origins=item.field_mapping_origins,
                        verified_sha256=source_sha256,
                        verified_source=source_receipt,
                    )
                )
                continue
            if ext not in _SUPPORTED_DATA_EXTS:
                raise ValueError(f"unsupported file type: {src_path.suffix}")

            prepared.extend(
                self._prepare_from_data_file(
                    case_id,
                    src_path,
                    display_name=item.file_name or src_path.name,
                    source_label=item.source_path,
                    kind_hint=item.file_kind,
                    password=item.password,
                    field_mapping=item.field_mapping,
                    field_mapping_origins=item.field_mapping_origins,
                    verified_sha256=source_sha256,
                    verified_source=source_receipt,
                )
            )

        if not prepared:
            raise ValueError("no importable files prepared")
        self._normalize_prepared_item_identities(prepared)
        self._bind_prepared_items_to_case(case_id=case_id, authority_root=raw_dir, items=prepared)
        return prepared

    @staticmethod
    def _domain_category_for_kind(kind: str) -> str:
        if kind in {"fc_person", "fc_person_address", "fc_person_contact"}:
            return "entity"
        if kind == _SUPPORT_FILE_KIND:
            return "support"
        if kind.startswith("fc_"):
            return "structured"
        return "support"

    def _suggest_preview_field_mapping(
        self,
        *,
        file_name: str,
        kind: str,
        headers: Sequence[str],
        profiles: Optional[CsvColumnProfiles],
        profiler: Optional[ImportPreviewProfiler] = None,
    ) -> dict:
        normalized_kind = str(kind or "").strip()
        if normalized_kind not in FC_SCHEMAS or not headers:
            return {}
        if profiler:
            profiler.count("mapping_suggestions")
        started = time.perf_counter()
        try:
            suggestion = suggest_field_mapping(
                file_name=file_name,
                kind=normalized_kind,
                headers=headers,
                profiles=profiles,
            )
            mappings = dict(suggestion.mappings or {})
            field_mapping_origins = {
                candidate.target_key: "ai" if candidate.method == "ai_validated" else "auto"
                for candidate in suggestion.candidates
                if candidate.target_key in mappings
            }
            template_applied = self._apply_confirmed_mapping_template(
                kind=normalized_kind,
                headers=headers,
                profiles=profiles,
                mappings=mappings,
                origins=field_mapping_origins,
            )
            derived_bank_name = derive_bank_name_from_file_name(file_name, kind=normalized_kind)
            derived_bank_headers = set(derived_bank_headers_for_kind(normalized_kind)) if derived_bank_name else set()
            required_missing = (
                required_missing_for_mapping(normalized_kind, mappings)
                if template_applied
                else list(suggestion.required_missing or [])
            )
            derived_missing = [header for header in required_missing if header in derived_bank_headers]
            if derived_missing:
                required_missing = [header for header in required_missing if header not in derived_bank_headers]
            transform_messages = field_mapping_transform_messages(
                kind=normalized_kind,
                headers=headers,
                mappings=mappings,
                profiles=profiles,
            )
        finally:
            if profiler:
                profiler.add_phase("field_mapping_s", time.perf_counter() - started)
        mapping_message = suggestion.message
        mapping_status = suggestion.status
        mapping_method = suggestion.method
        if template_applied:
            mapping_method = "template"
            if required_missing:
                mapping_status = "review"
                mapping_message = f"已套用用户确认模板 {template_applied} 项字段映射；仍缺少关键字段：{', '.join(required_missing)}。"
            else:
                mapping_status = "ready"
                mapping_message = f"已套用用户确认模板并自动确认 {len(mappings)} 项字段映射。"
        if derived_missing:
            derived_note = f"{', '.join(derived_missing)}可从文件名自动补全：{derived_bank_name}"
            if required_missing:
                mapping_message = f"规则无法确认关键字段：{', '.join(required_missing)}；{derived_note}。"
            else:
                mapping_status = "ready"
                if mapping_method in {"", "none", "ai_unavailable"}:
                    mapping_method = "rule"
                mapping_message = f"已自动确认 {len(suggestion.mappings)} 项字段映射；{derived_note}。"
        if transform_messages:
            mapping_message = f"{mapping_message} 规则转换：" + "；".join(transform_messages)
        return {
            "field_mapping": mappings,
            "field_mapping_origins": field_mapping_origins,
            "mapping_status": mapping_status,
            "mapping_method": mapping_method,
            "mapping_message": mapping_message,
            "mapping_required_missing": required_missing,
        }

    def _apply_confirmed_mapping_template(
        self,
        *,
        kind: str,
        headers: Sequence[str],
        profiles: Optional[CsvColumnProfiles],
        mappings: Dict[str, str],
        origins: Dict[str, str],
    ) -> int:
        template = self._mapping_template_store().find(kind=kind, headers=headers)
        if template is None:
            return 0
        header_by_normalized = {
            normalize_header_for_match(header): str(header or "").strip()
            for header in headers
            if normalize_header_for_match(header)
        }
        applied = 0
        for target_key, stored_source_header in template.mappings.items():
            source_header = header_by_normalized.get(normalize_header_for_match(stored_source_header))
            if not source_header:
                continue
            if not validate_field_mapping_candidate(
                kind=kind,
                target_key=target_key,
                source_header=source_header,
                headers=headers,
                profiles=profiles,
            ):
                continue
            for existing_key, existing_source in list(mappings.items()):
                if existing_key != target_key and normalize_header_for_match(existing_source) == normalize_header_for_match(source_header):
                    mappings.pop(existing_key, None)
                    origins.pop(existing_key, None)
            if mappings.get(target_key) != source_header:
                applied += 1
            mappings[target_key] = source_header
            origins[target_key] = "auto"
        return applied

    def _record_confirmed_mapping_template(
        self,
        *,
        kind: str,
        headers: Sequence[str],
        field_mapping: Optional[Dict[str, str]],
        field_mapping_origins: Optional[Dict[str, str]] = None,
        file_name: str,
        rows_imported_norm: int,
    ) -> None:
        if rows_imported_norm <= 0 or not field_mapping or not field_mapping_origins:
            return
        try:
            self._mapping_template_store().record_manual_mappings(
                kind=kind,
                headers=headers,
                field_mapping=field_mapping,
                field_mapping_origins=field_mapping_origins,
                file_name=file_name,
            )
        except Exception:
            return

    def _mapping_template_store(self) -> ImportMappingTemplateStore:
        return ImportMappingTemplateStore(Path(self._storage.app_dir) / "import_mapping_templates.v1.json")

    def _preview_file(
        self,
        case_id: str,
        src_path: Path,
        *,
        display_name: str,
        kind_hint: Optional[str],
        password: Optional[str],
        profiler: Optional[ImportPreviewProfiler] = None,
    ) -> ImportFilePreview:
        ext = src_path.suffix.lower()
        file_type = self._file_type(src_path)
        size = _require_import_count(src_path.stat().st_size, field="source_size")
        display_name = display_name or src_path.name
        manual_kind = str(kind_hint or "").strip()
        if profiler:
            with profiler.time_phase("source_hash_s"):
                source_sha256 = file_hash(src_path, "sha256")
        else:
            source_sha256 = file_hash(src_path, "sha256")

        if ext not in _SUPPORTED_IMPORT_EXTS:
            if profiler:
                profiler.count("unsupported_files")
            return ImportFilePreview(
                file_name=display_name,
                source_path=str(src_path),
                file_type=file_type,
                size=size,
                rows_total=0,
                columns_total=0,
                header_preview=[],
                sample_rows=[],
                domain_category="support",
                suggested_kind=manual_kind,
                suggested_kind_label=_KIND_CN.get(manual_kind, manual_kind or "—"),
                status="unsupported",
                issue="当前格式暂不支持导入，请改用数据表、压缩包或研判文件。",
                accepts_password=False,
                requires_password=False,
                detected_by="extension",
                archive_children=[],
                sha256=source_sha256,
            )

        if ext in _SUPPORTED_DOCUMENT_EXTS:
            if profiler:
                profiler.count("document_files")
            encrypted = self._is_file_password_protected(src_path)
            support_kind = manual_kind or _SUPPORT_FILE_KIND
            issue = ""
            status = "ready"
            if encrypted and not password:
                status = "review"
                issue = "文件已加密，请先验证密码后继续。"
            elif encrypted:
                try:
                    self._maybe_decrypt_file(src_path, self._preview_cache_dir(src_path), password=password)
                except ValueError:
                    status = "review"
                    issue = "密码验证失败，请重新输入正确密码。"
            return ImportFilePreview(
                file_name=display_name,
                source_path=str(src_path),
                file_type=file_type,
                size=size,
                rows_total=0,
                columns_total=0,
                header_preview=[],
                sample_rows=[],
                domain_category="support",
                suggested_kind=support_kind,
                suggested_kind_label=_KIND_CN.get(support_kind, "研判文件"),
                status=status,
                issue=issue,
                accepts_password=encrypted,
                requires_password=encrypted,
                detected_by="extension",
                archive_children=[],
                sha256=source_sha256,
            )

        if ext in _SUPPORTED_ARCHIVE_EXTS:
            if profiler:
                profiler.count("archive_files")
            if ext == ".zip":
                return self._preview_zip_file(
                    src_path,
                    display_name=display_name,
                    kind_hint=manual_kind,
                    password=password,
                    source_sha256=source_sha256,
                    profiler=profiler,
                )
            return self._preview_archive_file(
                case_id,
                src_path,
                display_name=display_name,
                kind_hint=manual_kind,
                password=password,
                source_sha256=source_sha256,
                profiler=profiler,
            )

        if profiler:
            profiler.count("data_files")
        return self._preview_data_file(
            src_path,
            display_name=display_name,
            kind_hint=manual_kind,
            password=password,
            source_sha256=source_sha256,
            profiler=profiler,
        )

    def _preview_data_file(
        self,
        src_path: Path,
        *,
        display_name: str,
        kind_hint: str,
        password: Optional[str],
        source_sha256: str,
        profiler: Optional[ImportPreviewProfiler] = None,
    ) -> ImportFilePreview:
        detected_by = "manual" if kind_hint else "headers"
        encrypted = self._is_file_password_protected(src_path)
        preview_path = src_path
        if encrypted and not password:
            suggested_kind = kind_hint or detect_fc_kind(display_name, []) or detect_fc_kind(src_path.name, []) or ""
            return ImportFilePreview(
                file_name=display_name,
                source_path=str(src_path),
                file_type=self._file_type(src_path),
                size=_require_import_count(src_path.stat().st_size, field="source_size"),
                rows_total=0,
                columns_total=0,
                header_preview=[],
                sample_rows=[],
                domain_category=self._domain_category_for_kind(suggested_kind or "fc_transaction"),
                suggested_kind=suggested_kind,
                suggested_kind_label=_KIND_CN.get(suggested_kind, "待映射"),
                status="review",
                issue="文件已加密，请先验证密码后继续。",
                accepts_password=True,
                requires_password=True,
                detected_by=detected_by,
                archive_children=[],
                sha256=source_sha256,
            )
        if encrypted:
            try:
                preview_path = self._maybe_decrypt_file(src_path, self._preview_cache_dir(src_path), password=password)
            except ValueError:
                suggested_kind = kind_hint or detect_fc_kind(display_name, []) or detect_fc_kind(src_path.name, []) or ""
                return ImportFilePreview(
                    file_name=display_name,
                    source_path=str(src_path),
                    file_type=self._file_type(src_path),
                    size=_require_import_count(src_path.stat().st_size, field="source_size"),
                    rows_total=0,
                    columns_total=0,
                    header_preview=[],
                    sample_rows=[],
                    domain_category=self._domain_category_for_kind(suggested_kind or "fc_transaction"),
                    suggested_kind=suggested_kind,
                    suggested_kind_label=_KIND_CN.get(suggested_kind, "待映射"),
                    status="review",
                    issue="密码验证失败，请重新输入正确密码。",
                    accepts_password=True,
                    requires_password=True,
                    detected_by=detected_by,
                    archive_children=[],
                    sha256=source_sha256,
                )
        rows_total, columns_total, preview_headers, sample_rows, profiles = self._read_tabular_preview(
            preview_path,
            limit=5,
            preview_source_path=src_path if encrypted else None,
            preview_source_key="data" if encrypted else "",
            preview_label=display_name,
            profiler=profiler,
        )

        suggested_kind = kind_hint or detect_fc_kind(display_name, preview_headers) or detect_fc_kind(src_path.name, preview_headers) or ""
        status = "ready"
        issue = ""
        if not suggested_kind:
            status = "review"
            issue = "未能自动识别映射类型，请在下一步手动指定到资金数据或主体信息。"
        mapping_payload = self._suggest_preview_field_mapping(
            file_name=display_name,
            kind=suggested_kind,
            headers=preview_headers,
            profiles=profiles,
            profiler=profiler,
        )

        return ImportFilePreview(
            file_name=display_name,
            source_path=str(src_path),
            file_type=self._file_type(src_path),
            size=_require_import_count(src_path.stat().st_size, field="source_size"),
            rows_total=rows_total,
            columns_total=columns_total,
            header_preview=preview_headers,
            sample_rows=sample_rows,
            domain_category=self._domain_category_for_kind(suggested_kind or "fc_transaction"),
            suggested_kind=suggested_kind,
            suggested_kind_label=_KIND_CN.get(suggested_kind, "待映射"),
            status=status,
            issue=issue,
            accepts_password=False,
            requires_password=False,
            detected_by=detected_by,
            archive_children=[],
            sha256=source_sha256,
            **mapping_payload,
        )

    def _preview_zip_file(
        self,
        src_path: Path,
        *,
        display_name: str,
        kind_hint: str,
        password: Optional[str],
        source_sha256: str,
        profiler: Optional[ImportPreviewProfiler] = None,
    ) -> ImportFilePreview:
        encrypted = self._zip_requires_password(src_path)
        invalid_password = False
        if encrypted and password:
            try:
                self._validate_zip_password(src_path, password)
            except ValueError:
                invalid_password = True
        locked_archive = encrypted and (not password or invalid_password)
        if kind_hint == _SUPPORT_FILE_KIND:
            return ImportFilePreview(
                file_name=display_name,
                source_path=str(src_path),
                file_type=self._file_type(src_path),
                size=_require_import_count(src_path.stat().st_size, field="source_size"),
                rows_total=0,
                columns_total=0,
                header_preview=[],
                sample_rows=[],
                domain_category="support",
                suggested_kind=_SUPPORT_FILE_KIND,
                suggested_kind_label="研判文件",
                status="review" if locked_archive else "ready",
                issue="压缩包已加密，请先验证密码后继续。" if encrypted and not password else "压缩包密码验证失败，请重新输入正确密码。" if invalid_password else "",
                accepts_password=encrypted,
                requires_password=encrypted,
                detected_by="manual",
                archive_children=[],
                sha256=source_sha256,
            )

        data_entries = 0
        doc_entries = 0
        archive_kind_candidates: List[str] = []
        suggested_kind = kind_hint
        archive_children: List[ImportArchivePreview] = []
        zip_csv_batch_items: List[_ArchiveCsvPreviewBatchItem] = []
        preview_cache_key = f"{src_path.stem}_{file_hash(src_path, 'md5')[:10]}"

        with zipfile.ZipFile(src_path, "r") as archive:
            if profiler:
                with profiler.time_phase("zip_scan_s"):
                    zip_infos = archive.infolist()
            else:
                zip_infos = archive.infolist()
            for info in zip_infos:
                if info.is_dir():
                    continue
                fixed_name = self._fix_zip_name(info.filename, getattr(info, "flag_bits", 0))
                suffix = Path(fixed_name).suffix.lower()
                if suffix in _SUPPORTED_DATA_EXTS:
                    if profiler:
                        profiler.count("archive_children")
                    data_entries += 1
                    child_suggested_kind = kind_hint or detect_fc_kind(Path(fixed_name).name, []) or ""
                    if locked_archive:
                        child_preview = self._preview_zip_child(
                            fixed_name=fixed_name,
                            file_size=_require_import_count(
                                getattr(info, "file_size", None),
                                field="archive_member_size",
                            ),
                            kind_hint=child_suggested_kind,
                            encrypted=True,
                        )
                    elif suffix == ".csv":
                        zip_csv_batch_items.append(
                            self._preview_zip_csv_batch_item(
                                zip_path=src_path,
                                archive=archive,
                                info=info,
                                fixed_name=fixed_name,
                                password=password,
                                kind_hint=child_suggested_kind,
                                preview_cache_key=preview_cache_key,
                                profiler=profiler,
                            )
                        )
                        continue
                    else:
                        child_preview = self._preview_zip_child_resolved(
                            zip_path=src_path,
                            archive=archive,
                            info=info,
                            fixed_name=fixed_name,
                            password=password,
                            kind_hint=child_suggested_kind,
                            preview_cache_key=preview_cache_key,
                            profiler=profiler,
                        )
                    if child_preview.suggested_kind:
                        archive_kind_candidates.append(child_preview.suggested_kind)
                    archive_children.append(child_preview)
                elif suffix == ".zip" and not locked_archive:
                    nested_children = self._preview_nested_zip_children(
                        zip_path=src_path,
                        archive=archive,
                        info=info,
                        fixed_name=fixed_name,
                        password=password,
                        kind_hint=kind_hint,
                        preview_cache_key=preview_cache_key,
                        profiler=profiler,
                    )
                    for child_preview in nested_children:
                        child_suffix = Path(child_preview.file_name).suffix.lower()
                        if child_suffix in _SUPPORTED_DATA_EXTS:
                            data_entries += 1
                            if child_preview.suggested_kind:
                                archive_kind_candidates.append(child_preview.suggested_kind)
                        elif child_suffix in _SUPPORTED_DOCUMENT_EXTS:
                            doc_entries += 1
                        archive_children.append(child_preview)
                elif suffix in _SUPPORTED_DOCUMENT_EXTS:
                    if profiler:
                        profiler.count("archive_children")
                    doc_entries += 1
                    child_preview = (
                        self._preview_zip_child(
                            fixed_name=fixed_name,
                            file_size=_require_import_count(
                                getattr(info, "file_size", None),
                                field="archive_member_size",
                            ),
                            kind_hint=_SUPPORT_FILE_KIND,
                            encrypted=True,
                        )
                        if locked_archive
                        else self._preview_zip_child_resolved(
                            zip_path=src_path,
                            archive=archive,
                            info=info,
                            fixed_name=fixed_name,
                            password=password,
                            kind_hint=_SUPPORT_FILE_KIND,
                            preview_cache_key=preview_cache_key,
                            profiler=profiler,
                        )
                    )
                    archive_children.append(child_preview)

        if zip_csv_batch_items:
            for child_preview in self._preview_archive_csv_batch_from_paths(
                zip_csv_batch_items,
                profiler=profiler,
            ):
                if child_preview.suggested_kind:
                    archive_kind_candidates.append(child_preview.suggested_kind)
                archive_children.append(child_preview)

        if data_entries == 0 and doc_entries == 0:
            return ImportFilePreview(
                file_name=display_name,
                source_path=str(src_path),
                file_type=self._file_type(src_path),
                size=_require_import_count(src_path.stat().st_size, field="source_size"),
                rows_total=0,
                columns_total=0,
                header_preview=[],
                sample_rows=[],
                domain_category="support",
                suggested_kind="",
                suggested_kind_label="待检查",
                status="unsupported",
                issue="压缩包内未发现可导入的数据文件或研判文件。",
                accepts_password=encrypted,
                requires_password=encrypted,
                detected_by="archive",
                archive_children=[],
                sha256=source_sha256,
            )

        unique_kinds = sorted({kind for kind in archive_kind_candidates if kind})
        if not suggested_kind:
            if len(unique_kinds) == 1:
                suggested_kind = unique_kinds[0]
            elif len(unique_kinds) > 1:
                suggested_kind = ""

        display_kind = suggested_kind or (unique_kinds[0] if unique_kinds else "fc_transaction")
        domain_category = "support" if data_entries == 0 else self._domain_category_for_kind(display_kind)
        status = "ready" if data_entries == 0 or bool(suggested_kind) or len(unique_kinds) > 1 else "review"
        issue = ""
        if encrypted and not password:
            status = "review"
            issue = "压缩包已加密，请先验证密码后继续。"
        elif invalid_password:
            status = "review"
            issue = "压缩包密码验证失败，请重新输入正确密码。"
        elif len(unique_kinds) > 1:
            issue = f"压缩包内包含 {len(unique_kinds)} 类结构化数据，请按子项分别确认字段与类型。"
        elif data_entries > 0 and not suggested_kind:
            issue = "压缩包内包含结构化数据，但未能自动识别映射类型，请在下一步确认。"
        elif data_entries > 0 and doc_entries > 0:
            issue = f"压缩包内检测到 {data_entries} 个数据文件与 {doc_entries} 个研判文件。"
        elif doc_entries > 0 and data_entries == 0:
            suggested_kind = _SUPPORT_FILE_KIND

        return ImportFilePreview(
            file_name=display_name,
            source_path=str(src_path),
            file_type=self._file_type(src_path),
            size=_require_import_count(src_path.stat().st_size, field="source_size"),
            rows_total=0,
            columns_total=0,
            header_preview=[],
            sample_rows=[],
            domain_category=domain_category,
            suggested_kind=suggested_kind or "",
            suggested_kind_label=_KIND_CN.get(suggested_kind, "待映射" if data_entries > 0 else "研判文件"),
            status=status,
            issue=issue,
            accepts_password=encrypted,
            requires_password=encrypted,
            detected_by="archive",
            archive_children=archive_children,
            sha256=source_sha256,
        )

    def _preview_archive_file(
        self,
        case_id: str,
        src_path: Path,
        *,
        display_name: str,
        kind_hint: str,
        password: Optional[str],
        source_sha256: str,
        profiler: Optional[ImportPreviewProfiler] = None,
    ) -> ImportFilePreview:
        if kind_hint == _SUPPORT_FILE_KIND:
            return ImportFilePreview(
                file_name=display_name,
                source_path=str(src_path),
                file_type=self._file_type(src_path),
                size=_require_import_count(src_path.stat().st_size, field="source_size"),
                rows_total=0,
                columns_total=0,
                header_preview=[],
                sample_rows=[],
                domain_category="support",
                suggested_kind=_SUPPORT_FILE_KIND,
                suggested_kind_label="研判文件",
                status="ready",
                issue="",
                accepts_password=False,
                requires_password=False,
                detected_by="manual",
                archive_children=[],
                sha256=source_sha256,
            )

        extract_root = self._archive_extract_root(case_id, src_path, source_sha256)
        nested_cache_dir = extract_root.parent / "_nested_archives"
        member_generation_root = extract_root.parent / "_verified_archive_members"
        try:
            if profiler:
                with profiler.time_phase("archive_extract_s"):
                    archive_children, data_entries, doc_entries, archive_kind_candidates = self._preview_extracted_archive_children(
                        extract_root=extract_root,
                        nested_cache_dir=nested_cache_dir,
                        member_generation_root=member_generation_root,
                        source_archive_path=src_path,
                        source_archive_sha256=source_sha256,
                        kind_hint=kind_hint,
                        password=password,
                        profiler=profiler,
                    )
            else:
                archive_children, data_entries, doc_entries, archive_kind_candidates = self._preview_extracted_archive_children(
                    extract_root=extract_root,
                    nested_cache_dir=nested_cache_dir,
                    member_generation_root=member_generation_root,
                    source_archive_path=src_path,
                    source_archive_sha256=source_sha256,
                    kind_hint=kind_hint,
                    password=password,
                    profiler=profiler,
                )
        except ValueError:
            return ImportFilePreview(
                file_name=display_name,
                source_path=str(src_path),
                file_type=self._file_type(src_path),
                size=_require_import_count(src_path.stat().st_size, field="source_size"),
                rows_total=0,
                columns_total=0,
                header_preview=[],
                sample_rows=[],
                domain_category="support",
                suggested_kind="",
                suggested_kind_label="待检查",
                status="unsupported",
                issue=IMPORT_ARCHIVE_PREVIEW_FAILED,
                accepts_password=False,
                requires_password=False,
                detected_by="archive",
                archive_children=[],
                sha256=source_sha256,
            )

        if data_entries == 0 and doc_entries == 0:
            return ImportFilePreview(
                file_name=display_name,
                source_path=str(src_path),
                file_type=self._file_type(src_path),
                size=_require_import_count(src_path.stat().st_size, field="source_size"),
                rows_total=0,
                columns_total=0,
                header_preview=[],
                sample_rows=[],
                domain_category="support",
                suggested_kind="",
                suggested_kind_label="待检查",
                status="unsupported",
                issue="压缩包内未发现可导入的数据文件或研判文件。",
                accepts_password=False,
                requires_password=False,
                detected_by="archive",
                archive_children=[],
                sha256=source_sha256,
            )

        suggested_kind = kind_hint
        unique_kinds = sorted({kind for kind in archive_kind_candidates if kind})
        if not suggested_kind:
            if len(unique_kinds) == 1:
                suggested_kind = unique_kinds[0]
            elif len(unique_kinds) > 1:
                suggested_kind = ""

        display_kind = suggested_kind or (unique_kinds[0] if unique_kinds else "fc_transaction")
        domain_category = "support" if data_entries == 0 else self._domain_category_for_kind(display_kind)
        status = "ready" if data_entries == 0 or bool(suggested_kind) or len(unique_kinds) > 1 else "review"
        issue = ""
        if len(unique_kinds) > 1:
            issue = f"压缩包内包含 {len(unique_kinds)} 类结构化数据，请按子项分别确认字段与类型。"
        elif data_entries > 0 and not suggested_kind:
            issue = "压缩包内包含结构化数据，但未能自动识别映射类型，请在下一步确认。"
        elif data_entries > 0 and doc_entries > 0:
            issue = f"压缩包内检测到 {data_entries} 个数据文件与 {doc_entries} 个研判文件。"
        elif doc_entries > 0 and data_entries == 0:
            suggested_kind = _SUPPORT_FILE_KIND

        return ImportFilePreview(
            file_name=display_name,
            source_path=str(src_path),
            file_type=self._file_type(src_path),
            size=_require_import_count(src_path.stat().st_size, field="source_size"),
            rows_total=0,
            columns_total=0,
            header_preview=[],
            sample_rows=[],
            domain_category=domain_category,
            suggested_kind=suggested_kind or "",
            suggested_kind_label=_KIND_CN.get(suggested_kind, "待映射" if data_entries > 0 else "研判文件"),
            status=status,
            issue=issue,
            accepts_password=False,
            requires_password=False,
            detected_by="archive",
            archive_children=archive_children,
            sha256=source_sha256,
        )

    def _preview_extracted_archive_children(
        self,
        *,
        extract_root: Path,
        nested_cache_dir: Path,
        member_generation_root: Path,
        source_archive_path: Path,
        source_archive_sha256: str,
        kind_hint: str,
        password: Optional[str],
        parent_path: str = "",
        depth: int = 0,
        profiler: Optional[ImportPreviewProfiler] = None,
    ) -> tuple[List[ImportArchivePreview], int, int, List[str]]:
        children: List[ImportArchivePreview] = []
        data_entries = 0
        doc_entries = 0
        archive_kind_candidates: List[str] = []
        ordered_entries: List[ImportArchivePreview | List[ImportArchivePreview] | _ArchiveCsvPreviewBatchItem] = []
        csv_batch_items: List[_ArchiveCsvPreviewBatchItem] = []
        materialized: List[_VerifiedArchiveMemberGeneration] = []
        with open_archive_extraction_v1(
            source_archive_path,
            extract_root,
            password=password,
            expected_sha256=source_archive_sha256,
            reuse=True,
        ) as lease:
            for member in lease.file_members():
                suffix = Path(member.relative_path).suffix.lower()
                if suffix not in _SUPPORTED_ARCHIVE_EXTS and suffix not in _SUPPORTED_DATA_EXTS and suffix not in _SUPPORTED_DOCUMENT_EXTS:
                    continue
                generation = lease.publish_member(
                    member,
                    member_generation_root,
                    lineage={
                        "kind": "archive_preview_member",
                        "logical_parent_path": parent_path,
                    },
                )
                materialized.append(
                    _VerifiedArchiveMemberGeneration(
                        logical_path=member.relative_path,
                        member=member,
                        generation=generation,
                        source_archive_path=source_archive_path,
                    )
                )

        for verified in materialized:
            child_path = verified.generation.path
            member_name = verified.logical_path
            archive_path = combine_archive_path(parent_path, member_name)
            suffix = Path(member_name).suffix.lower()
            if suffix in _SUPPORTED_ARCHIVE_EXTS:
                if depth >= _MAX_NESTED_ARCHIVE_DEPTH:
                    continue
                nested_root = nested_cache_dir / archive_cache_output_name(archive_path)
                try:
                    if profiler:
                        profiler.count("nested_archives")
                    nested_children, nested_data_entries, nested_doc_entries, _ = self._preview_extracted_archive_children(
                        extract_root=nested_root,
                        nested_cache_dir=nested_cache_dir,
                        member_generation_root=member_generation_root,
                        source_archive_path=child_path,
                        source_archive_sha256=verified.generation.sha256,
                        kind_hint=kind_hint,
                        password=password,
                        parent_path=archive_path,
                        depth=depth + 1,
                        profiler=profiler,
                    )
                except ValueError:
                    continue
                ordered_entries.append(nested_children)
                data_entries += nested_data_entries
                doc_entries += nested_doc_entries
                continue
            if suffix not in _SUPPORTED_DATA_EXTS and suffix not in _SUPPORTED_DOCUMENT_EXTS:
                continue
            consumed_path = self._maybe_decrypt_file(
                child_path,
                member_generation_root / "_decryptcache",
                password=password,
            )
            consumed_receipt = inspect_regular_source(consumed_path)
            if suffix in _SUPPORTED_DATA_EXTS:
                if profiler:
                    profiler.count("archive_children")
                data_entries += 1
                child_kind = kind_hint or detect_fc_kind(Path(member_name).name, []) or ""
            else:
                if profiler:
                    profiler.count("archive_children")
                doc_entries += 1
                child_kind = _SUPPORT_FILE_KIND
            if suffix == ".csv":
                if profiler:
                    profiler.count("data_files")
                    with profiler.time_phase("child_hash_s"):
                        child_sha256 = consumed_receipt.sha256.lower()
                else:
                    child_sha256 = consumed_receipt.sha256.lower()
                batch_item = _ArchiveCsvPreviewBatchItem(
                    preview_path=consumed_path,
                    archive_path=archive_path,
                    source_archive_path=source_archive_path,
                    kind_hint=child_kind,
                    sha256=child_sha256,
                    size=consumed_receipt.size,
                )
                csv_batch_items.append(batch_item)
                ordered_entries.append(batch_item)
                continue
            child_preview = self._preview_archive_child_resolved_from_path(
                preview_path=consumed_path,
                archive_path=archive_path,
                source_archive_path=source_archive_path,
                kind_hint=child_kind,
                profiler=profiler,
            )
            ordered_entries.append(child_preview)

        csv_batch_previews: Dict[_ArchiveCsvPreviewBatchItem, ImportArchivePreview] = {}
        if csv_batch_items:
            csv_batch_previews = dict(
                zip(
                    csv_batch_items,
                    self._preview_archive_csv_batch_from_paths(
                        csv_batch_items,
                        profiler=profiler,
                    ),
                )
            )

        for entry in ordered_entries:
            if isinstance(entry, list):
                children.extend(entry)
                archive_kind_candidates.extend(child.suggested_kind for child in entry if child.suggested_kind)
                continue
            child_preview = csv_batch_previews.get(entry) if isinstance(entry, _ArchiveCsvPreviewBatchItem) else entry
            if not child_preview:
                continue
            if child_preview.suggested_kind:
                archive_kind_candidates.append(child_preview.suggested_kind)
            children.append(child_preview)

        return children, data_entries, doc_entries, archive_kind_candidates

    def _preview_archive_csv_batch_from_paths(
        self,
        items: Sequence[_ArchiveCsvPreviewBatchItem],
        *,
        profiler: Optional[ImportPreviewProfiler] = None,
    ) -> List[ImportArchivePreview]:
        preview_results = self._read_csv_tabular_previews_batch(
            [
                _CsvTabularPreviewRequest(
                    path=item.preview_path,
                    label=item.archive_path if "::" in item.archive_path else archive_entry_leaf_name(item.archive_path),
                    limit=5,
                )
                for item in items
            ],
            profiler=profiler,
        )
        previews: List[ImportArchivePreview] = []
        for item, result in zip(items, preview_results):
            leaf_name = archive_entry_leaf_name(item.archive_path)
            suggested_kind = item.kind_hint or detect_fc_kind(leaf_name, result.headers) or ""
            issue = ""
            status = "ready"
            if not suggested_kind:
                status = "review"
                issue = "未能自动识别映射类型，请确认该子项的入库类型。"
            mapping_payload = self._suggest_preview_field_mapping(
                file_name=leaf_name,
                kind=suggested_kind,
                headers=result.headers,
                profiles=result.profiles,
                profiler=profiler,
            )
            previews.append(
                ImportArchivePreview(
                    file_name=leaf_name,
                    archive_path=item.archive_path,
                    file_type=self._file_type(item.preview_path),
                    size=item.size,
                    rows_total=result.rows_total,
                    columns_total=result.columns_total,
                    header_preview=result.headers,
                    sample_rows=result.sample_rows,
                    domain_category=self._domain_category_for_kind(suggested_kind or "fc_transaction"),
                    suggested_kind=suggested_kind,
                    suggested_kind_label=_KIND_CN.get(suggested_kind, "待映射"),
                    status=status,
                    issue=issue,
                    detected_by="archive-entry",
                    sha256=item.sha256,
                    **mapping_payload,
                )
            )
        return previews

    def _preview_archive_child_resolved_from_path(
        self,
        *,
        preview_path: Path,
        archive_path: str,
        source_archive_path: Path,
        kind_hint: str,
        profiler: Optional[ImportPreviewProfiler] = None,
    ) -> ImportArchivePreview:
        suffix = preview_path.suffix.lower()
        leaf_name = archive_entry_leaf_name(archive_path)
        if suffix in _SUPPORTED_DOCUMENT_EXTS:
            suggested_kind = kind_hint or _SUPPORT_FILE_KIND
            if profiler:
                profiler.count("document_files")
                with profiler.time_phase("child_hash_s"):
                    child_sha256 = file_hash(preview_path, "sha256")
            else:
                child_sha256 = file_hash(preview_path, "sha256")
            return ImportArchivePreview(
                file_name=leaf_name,
                archive_path=archive_path,
                file_type=self._file_type(preview_path),
                size=_require_import_count(preview_path.stat().st_size, field="source_size"),
                rows_total=0,
                columns_total=0,
                header_preview=[],
                sample_rows=[],
                domain_category="support",
                suggested_kind=suggested_kind,
                suggested_kind_label=_KIND_CN.get(suggested_kind, "研判文件"),
                status="ready",
                issue="",
                detected_by="archive-entry",
                sha256=child_sha256,
            )

        if profiler:
            profiler.count("data_files")
            with profiler.time_phase("child_hash_s"):
                child_sha256 = file_hash(preview_path, "sha256")
        else:
            child_sha256 = file_hash(preview_path, "sha256")
        rows_total, columns_total, preview_headers, sample_rows, profiles = self._read_tabular_preview(
            preview_path,
            limit=5,
            preview_source_path=source_archive_path,
            preview_source_key=archive_path,
            preview_excel_sha256=child_sha256,
            preview_label=leaf_name,
            profiler=profiler,
        )
        suggested_kind = kind_hint or detect_fc_kind(leaf_name, preview_headers) or ""
        status = "ready"
        issue = ""
        if not suggested_kind:
            status = "review"
            issue = "未能自动识别映射类型，请确认该子项的入库类型。"
        mapping_payload = self._suggest_preview_field_mapping(
            file_name=leaf_name,
            kind=suggested_kind,
            headers=preview_headers,
            profiles=profiles,
            profiler=profiler,
        )
        return ImportArchivePreview(
            file_name=leaf_name,
            archive_path=archive_path,
            file_type=self._file_type(preview_path),
            size=_require_import_count(preview_path.stat().st_size, field="source_size"),
            rows_total=rows_total,
            columns_total=columns_total,
            header_preview=preview_headers,
            sample_rows=sample_rows,
            domain_category=self._domain_category_for_kind(suggested_kind or "fc_transaction"),
            suggested_kind=suggested_kind,
            suggested_kind_label=_KIND_CN.get(suggested_kind, "待映射"),
            status=status,
            issue=issue,
            detected_by="archive-entry",
            sha256=child_sha256,
            **mapping_payload,
        )

    def _preview_zip_child(
        self,
        *,
        fixed_name: str,
        file_size: int,
        kind_hint: str,
        encrypted: bool,
    ) -> ImportArchivePreview:
        child_name = Path(fixed_name).name
        file_type = self._file_type(Path(child_name))
        suffix = Path(child_name).suffix.lower()
        suggested_kind = kind_hint or (_SUPPORT_FILE_KIND if suffix in _SUPPORTED_DOCUMENT_EXTS else "")
        if suffix in _SUPPORTED_DOCUMENT_EXTS:
            domain_category = "support"
            status = "review" if encrypted else "ready"
            issue = "压缩包已加密，导入前需要输入密码。" if encrypted else ""
        else:
            domain_category = self._domain_category_for_kind(suggested_kind or "fc_transaction")
            status = "review" if encrypted or not suggested_kind else "ready"
            issue = "压缩包已加密，导入前需要输入密码。" if encrypted else ""
            if not encrypted and not suggested_kind:
                issue = "未能自动识别映射类型，请在下一步确认。"

        return ImportArchivePreview(
            file_name=child_name,
            archive_path=fixed_name,
            file_type=file_type,
            size=_require_import_count(file_size, field="archive_member_size"),
            rows_total=0,
            columns_total=0,
            header_preview=[],
            sample_rows=[],
            domain_category=domain_category,
            suggested_kind=suggested_kind,
            suggested_kind_label=_KIND_CN.get(suggested_kind, "待映射" if suffix in _SUPPORTED_DATA_EXTS else "研判文件"),
            status=status,
            issue=issue,
            detected_by="archive-entry",
            sha256="",
        )

    def _preview_zip_csv_batch_item(
        self,
        *,
        zip_path: Path,
        archive: zipfile.ZipFile,
        info: zipfile.ZipInfo,
        fixed_name: str,
        password: Optional[str],
        kind_hint: str,
        preview_cache_key: str,
        profiler: Optional[ImportPreviewProfiler] = None,
    ) -> _ArchiveCsvPreviewBatchItem:
        preview_path = self._extract_zip_entry_for_preview(
            zip_path=zip_path,
            archive=archive,
            info=info,
            fixed_name=fixed_name,
            password=password,
            preview_cache_key=preview_cache_key,
            profiler=profiler,
        )
        if profiler:
            profiler.count("data_files")
            with profiler.time_phase("child_hash_s"):
                child_sha256 = file_hash(preview_path, "sha256")
        else:
            child_sha256 = file_hash(preview_path, "sha256")
        return _ArchiveCsvPreviewBatchItem(
            preview_path=preview_path,
            archive_path=fixed_name,
            source_archive_path=zip_path,
            kind_hint=kind_hint,
            sha256=child_sha256,
            size=int(preview_path.stat().st_size),
        )

    def _preview_zip_child_resolved(
        self,
        *,
        zip_path: Path,
        archive: zipfile.ZipFile,
        info: zipfile.ZipInfo,
        fixed_name: str,
        password: Optional[str],
        kind_hint: str,
        preview_cache_key: str,
        profiler: Optional[ImportPreviewProfiler] = None,
    ) -> ImportArchivePreview:
        preview_path = self._extract_zip_entry_for_preview(
            zip_path=zip_path,
            archive=archive,
            info=info,
            fixed_name=fixed_name,
            password=password,
            preview_cache_key=preview_cache_key,
            profiler=profiler,
        )
        suffix = preview_path.suffix.lower()
        if suffix in _SUPPORTED_DOCUMENT_EXTS:
            suggested_kind = kind_hint or _SUPPORT_FILE_KIND
            if profiler:
                profiler.count("document_files")
                with profiler.time_phase("child_hash_s"):
                    child_sha256 = file_hash(preview_path, "sha256")
            else:
                child_sha256 = file_hash(preview_path, "sha256")
            return ImportArchivePreview(
                file_name=Path(fixed_name).name,
                archive_path=fixed_name,
                file_type=self._file_type(preview_path),
                size=int(preview_path.stat().st_size),
                rows_total=0,
                columns_total=0,
                header_preview=[],
                sample_rows=[],
                domain_category="support",
                suggested_kind=suggested_kind,
                suggested_kind_label=_KIND_CN.get(suggested_kind, "研判文件"),
                status="ready",
                issue="",
                detected_by="archive-entry",
                sha256=child_sha256,
            )

        if profiler:
            profiler.count("data_files")
            with profiler.time_phase("child_hash_s"):
                child_sha256 = file_hash(preview_path, "sha256")
        else:
            child_sha256 = file_hash(preview_path, "sha256")
        rows_total, columns_total, preview_headers, sample_rows, profiles = self._read_tabular_preview(
            preview_path,
            limit=5,
            preview_source_path=zip_path,
            preview_source_key=fixed_name,
            preview_excel_sha256=child_sha256,
            preview_label=Path(fixed_name).name,
            profiler=profiler,
        )
        suggested_kind = kind_hint or detect_fc_kind(Path(fixed_name).name, preview_headers) or ""
        issue = ""
        status = "ready"
        if not suggested_kind:
            status = "review"
            issue = "未能自动识别映射类型，请确认该子项的入库类型。"
        mapping_payload = self._suggest_preview_field_mapping(
            file_name=Path(fixed_name).name,
            kind=suggested_kind,
            headers=preview_headers,
            profiles=profiles,
            profiler=profiler,
        )
        return ImportArchivePreview(
            file_name=Path(fixed_name).name,
            archive_path=fixed_name,
            file_type=self._file_type(preview_path),
            size=int(preview_path.stat().st_size),
            rows_total=rows_total,
            columns_total=columns_total,
            header_preview=preview_headers,
            sample_rows=sample_rows,
            domain_category=self._domain_category_for_kind(suggested_kind or "fc_transaction"),
            suggested_kind=suggested_kind,
            suggested_kind_label=_KIND_CN.get(suggested_kind, "待映射"),
            status=status,
            issue=issue,
            detected_by="archive-entry",
            sha256=child_sha256,
            **mapping_payload,
        )

    def _preview_nested_zip_children(
        self,
        *,
        zip_path: Path,
        archive: zipfile.ZipFile,
        info: zipfile.ZipInfo,
        fixed_name: str,
        password: Optional[str],
        kind_hint: str,
        preview_cache_key: str,
        depth: int = 0,
        profiler: Optional[ImportPreviewProfiler] = None,
    ) -> List[ImportArchivePreview]:
        if depth >= _MAX_NESTED_ZIP_DEPTH:
            return []

        children: List[ImportArchivePreview] = []
        zip_csv_batch_items: List[_ArchiveCsvPreviewBatchItem] = []
        try:
            nested_path = self._extract_zip_entry_for_preview(
                zip_path=zip_path,
                archive=archive,
                info=info,
                fixed_name=fixed_name,
                password=password,
                preview_cache_key=preview_cache_key,
                profiler=profiler,
            )
            with zipfile.ZipFile(nested_path, "r") as nested_archive:
                if profiler:
                    with profiler.time_phase("zip_scan_s"):
                        nested_infos = nested_archive.infolist()
                else:
                    nested_infos = nested_archive.infolist()
                for nested_info in nested_infos:
                    if nested_info.is_dir():
                        continue
                    nested_name = self._fix_zip_name(nested_info.filename, getattr(nested_info, "flag_bits", 0))
                    combined_name = combine_archive_path(fixed_name, nested_name)
                    nested_suffix = Path(nested_name).suffix.lower()
                    if nested_suffix == ".zip":
                        children.extend(
                            self._preview_nested_zip_children(
                                zip_path=nested_path,
                                archive=nested_archive,
                                info=nested_info,
                                fixed_name=combined_name,
                                password=password,
                                kind_hint=kind_hint,
                                preview_cache_key=preview_cache_key,
                                depth=depth + 1,
                                profiler=profiler,
                            )
                        )
                        continue
                    if nested_suffix not in _SUPPORTED_DATA_EXTS and nested_suffix not in _SUPPORTED_DOCUMENT_EXTS:
                        continue
                    if nested_suffix in _SUPPORTED_DATA_EXTS:
                        if profiler:
                            profiler.count("archive_children")
                        child_kind = kind_hint or detect_fc_kind(Path(nested_name).name, []) or ""
                        if nested_suffix == ".csv":
                            zip_csv_batch_items.append(
                                self._preview_zip_csv_batch_item(
                                    zip_path=nested_path,
                                    archive=nested_archive,
                                    info=nested_info,
                                    fixed_name=combined_name,
                                    password=password,
                                    kind_hint=child_kind,
                                    preview_cache_key=preview_cache_key,
                                    profiler=profiler,
                                )
                            )
                            continue
                    else:
                        if profiler:
                            profiler.count("archive_children")
                        child_kind = _SUPPORT_FILE_KIND
                    child_preview = self._preview_zip_child_resolved(
                        zip_path=nested_path,
                        archive=nested_archive,
                        info=nested_info,
                        fixed_name=combined_name,
                        password=password,
                        kind_hint=child_kind,
                        preview_cache_key=preview_cache_key,
                        profiler=profiler,
                    )
                    child_preview.file_name = Path(nested_name).name
                    children.append(child_preview)
                if zip_csv_batch_items:
                    children.extend(
                        self._preview_archive_csv_batch_from_paths(
                            zip_csv_batch_items,
                            profiler=profiler,
                        )
                    )
        except (RuntimeError, zipfile.BadZipFile):
            return []
        return children

    @staticmethod
    def _preview_cache_dir(path: Path) -> Path:
        if path.parent.parent.name == ".import_preview_cache":
            return path.parent.parent
        return path.parent / ".import_preview_cache"

    def _archive_extract_root(self, case_id: str, archive_path: Path, archive_sha256: str) -> Path:
        archive_cache_root = (
            self._storage.case_dir(case_id)
            / "raw"
            / "_archivecache"
            / archive_cache_dir_name(archive_path, archive_sha256)
        )
        return archive_cache_root / "root"

    @staticmethod
    def _preview_cache_output_name(fixed_name: str) -> str:
        return archive_cache_output_name(fixed_name)

    def _extract_zip_entry_for_preview(
        self,
        *,
        zip_path: Path,
        archive: zipfile.ZipFile,
        info: zipfile.ZipInfo,
        fixed_name: str,
        password: Optional[str],
        preview_cache_key: str,
        profiler: Optional[ImportPreviewProfiler] = None,
    ) -> Path:
        preview_cache_root = self._preview_cache_dir(zip_path)
        ensure_private_generation_root(preview_cache_root)
        preview_root = preview_cache_root / preview_cache_key
        ensure_private_generation_root(preview_root)
        archive_pwd = password.encode("utf-8") if password else None

        def publish_entry() -> Path:
            generation = publish_generated(
                preview_root,
                suffix=Path(fixed_name).suffix,
                producer=lambda output: self._copy_zip_entry(archive, info, archive_pwd, output),
                lineage={
                    "kind": "archive_entry",
                    "derived": True,
                    "archive_sha256": inspect_regular_source(zip_path).sha256,
                    "archive_entry": fixed_name,
                    "encrypted_source": bool(info.flag_bits & 0x1),
                },
            )
            expected_size = _require_import_count(
                getattr(info, "file_size", None),
                field="archive_member_size",
            )
            if generation.size != expected_size:
                raise ValueError("archive entry size mismatch")
            return generation.path

        if profiler:
            with profiler.time_phase("zip_extract_entry_s"):
                output_path = publish_entry()
        else:
            output_path = publish_entry()
        if profiler:
            with profiler.time_phase("decrypt_file_s"):
                return self._maybe_decrypt_file(output_path, preview_root / "_decryptcache", password=password)
        return self._maybe_decrypt_file(output_path, preview_root / "_decryptcache", password=password)

    @staticmethod
    def _copy_zip_entry(
        archive: zipfile.ZipFile,
        info: zipfile.ZipInfo,
        password: Optional[bytes],
        output,
    ) -> None:
        with archive.open(info, "r", pwd=password) as source:
            shutil.copyfileobj(source, output)

    @staticmethod
    def _validate_zip_password(path: Path, password: str) -> None:
        pwd = password.encode("utf-8")
        with zipfile.ZipFile(path, "r") as archive:
            encrypted_entries = [info for info in archive.infolist() if not info.is_dir() and (info.flag_bits & 0x1)]
            if not encrypted_entries:
                return
            probe = encrypted_entries[0]
            try:
                with archive.open(probe, "r", pwd=pwd) as handle:
                    handle.read(1)
            except RuntimeError as exc:
                raise ValueError(f"invalid password for encrypted archive: {path.name}") from exc

    def run_file_import(
        self,
        *,
        engine: DuckDBEngine,
        case_id: str,
        item: PreparedImportFile,
        progress_cb: Optional[Callable[[dict], None]] = None,
        attempts: int = 1,
    ) -> ImportExecutionResult:
        self._validate_prepared_item(case_id, item)
        rows_seen = 0
        rows_imported_raw = 0
        rows_imported_norm = 0
        rows_dedup = 0
        rows_error = 0
        rows_skipped_non_data = 0
        note = ""
        final_error = ""
        kind = item.kind_hint or ""
        md5 = ""
        sha256 = item.sha256 or ""

        try:
            if not sha256:
                source_receipt = item.ingestion_receipt.get("source")
                sha256 = str(source_receipt.get("sha256") or "") if isinstance(source_receipt, dict) else ""
                if not sha256:
                    sha256 = str(item.ingestion_receipt.get("sha256") or "")

            import_path = item.duckdb_csv_path or item.real_path
            csv_encoding = str(item.csv_encoding or "").strip()
            duckdb_csv_encoding = str(item.duckdb_csv_encoding or "").strip()
            if item.kind_hint == _SUPPORT_FILE_KIND or import_path.suffix.lower() in _SUPPORTED_DOCUMENT_EXTS:
                kind = item.kind_hint or _SUPPORT_FILE_KIND
                note = IMPORT_FILE_REGISTERED

                upsert_import_file_log(
                    engine,
                    file_id=item.file_id,
                    case_id=case_id,
                    kind=kind,
                    filename=item.display_name,
                    display_path=item.display_path,
                    stored_path=str(item.real_path),
                    file_type=item.file_type,
                    size=item.size,
                    md5=md5,
                    sha256=sha256,
                    rows_total=0,
                    status="导入中",
                    error="",
                )
                update_import_progress(
                    engine,
                    item.file_id,
                    0,
                    status="已完成",
                    error="",
                    rows_total=0,
                    rows_dedup=0,
                    rows_error=0,
                    rows_imported_raw=0,
                    rows_imported_norm=0,
                    rows_skipped_non_data=0,
                )
                try:
                    knowledge_result = self._document_repository.ingest_imported_file(
                        engine=engine,
                        case_id=case_id,
                        file_id=item.file_id,
                        filename=item.display_name,
                        display_path=item.display_path,
                        stored_path=str(item.real_path),
                        file_type=item.file_type,
                        size=item.size,
                        md5=md5,
                        sha256=sha256,
                        kind=kind,
                    )
                    if knowledge_result.note:
                        note = IMPORT_FILE_WARNING
                except Exception:
                    note = IMPORT_FILE_WARNING

                return ImportExecutionResult(
                    file_id=item.file_id,
                    display_name=item.display_name,
                    display_path=item.display_path,
                    file_type=item.file_type,
                    size=item.size,
                    md5=md5,
                    sha256=sha256,
                    kind=kind,
                    status="succeeded",
                    rows_total=0,
                    rows_seen=0,
                    rows_imported_raw=0,
                    rows_imported_norm=0,
                    rows_dedup=0,
                    rows_error=0,
                    note=note,
                    error="",
                    attempts=attempts,
                )

            if import_path.suffix.lower() in {".xlsx", ".xls"}:
                cache_dir = self._storage.case_dir(case_id) / "raw" / "_excelcache"
                import_path = self._excel_to_import_csv(import_path, cache_dir)
                if not csv_encoding:
                    csv_encoding = "utf-8"

            if import_path.suffix.lower() != ".csv":
                raise ValueError("当前格式暂不支持导入，请改用数据表、压缩包或研判文件。")

            rows_total_hint = _require_import_count(item.rows_total, field="rows_total")
            if rows_total_hint <= 0:
                rows_total_hint = self._count_rows_fast(import_path)
            rows_total_hint = _require_import_count(rows_total_hint, field="rows_total_hint")

            encoding = duckdb_csv_encoding or csv_encoding or detect_file_encoding(import_path)
            headers = list(item.csv_headers or [])
            if not headers:
                headers = read_csv_headers(import_path, encoding)
            if not kind:
                kind = detect_fc_kind(item.display_name, headers) or detect_fc_kind(import_path.name, headers) or ""
            if kind not in FC_SCHEMAS:
                raise ValueError("failed to detect funds-control file kind from headers")

            upsert_import_file_log(
                engine,
                file_id=item.file_id,
                case_id=case_id,
                kind=kind,
                filename=item.display_name,
                display_path=item.display_path,
                stored_path=str(item.real_path),
                file_type=item.file_type,
                size=item.size,
                md5=md5,
                sha256=sha256,
                rows_total=rows_total_hint,
                status="导入中",
                error="",
            )

            def _on_progress(imported_rows: int, total_seen: int) -> None:
                nonlocal rows_seen, rows_imported_raw, rows_imported_norm, rows_dedup, rows_skipped_non_data
                rows_seen = _require_import_count(total_seen, field="rows_seen")
                rows_imported_raw = _require_import_count(
                    imported_rows,
                    field="rows_imported_raw",
                )
                rows_imported_norm = rows_imported_raw
                rows_dedup = max(rows_seen - rows_imported_raw, 0)
                rows_skipped_non_data = max(rows_total_hint - rows_seen, 0)

                if progress_cb:
                    progress_cb(
                        {
                            "kind": kind,
                            "file_id": item.file_id,
                            "display_name": item.display_name,
                            "rows_seen": rows_seen,
                            "rows_total": max(rows_total_hint, rows_seen),
                            "rows_imported_raw": rows_imported_raw,
                            "rows_imported_norm": rows_imported_norm,
                            "rows_dedup": rows_dedup,
                            "rows_error": rows_error,
                            "rows_skipped_non_data": rows_skipped_non_data,
                        }
                    )

            timing_profile: Dict[str, object] = {}
            total_seen, raw_inserted, norm_inserted, import_note = import_fc_csv_into_db(
                engine=engine,
                case_id=case_id,
                file_id=item.file_id,
                kind=kind,
                path=import_path,
                display_name=item.display_name,
                field_mapping=item.field_mapping,
                progress_cb=_on_progress if progress_cb else None,
                batch_size=5000,
                log_detail=False,
                csv_encoding=encoding,
                precleaned_csv=item.duckdb_csv_path is not None,
                profile_cb=lambda profile: timing_profile.update(profile or {}),
            )

            rows_seen = _require_import_count(total_seen, field="rows_seen")
            rows_imported_raw = _require_import_count(
                raw_inserted,
                field="rows_imported_raw",
            )
            rows_imported_norm = _require_import_count(
                norm_inserted,
                field="rows_imported_norm",
            )
            rows_dedup = max(rows_seen - rows_imported_raw, 0)
            rows_skipped_non_data = max(rows_total_hint - rows_seen, 0)
            rows_total = max(rows_total_hint, rows_seen + rows_skipped_non_data, rows_seen)
            note = project_import_note(import_note)

            update_import_progress(
                engine,
                item.file_id,
                rows_imported_norm,
                status="已完成",
                error=note[:10000] if note else "",
                rows_total=rows_total,
                rows_dedup=rows_dedup,
                rows_error=rows_error,
                rows_imported_raw=rows_imported_raw,
                rows_imported_norm=rows_imported_norm,
                rows_skipped_non_data=rows_skipped_non_data,
            )
            self._record_confirmed_mapping_template(
                kind=kind,
                headers=headers,
                field_mapping=item.field_mapping,
                field_mapping_origins=item.field_mapping_origins,
                file_name=item.display_name,
                rows_imported_norm=rows_imported_norm,
            )

            return ImportExecutionResult(
                file_id=item.file_id,
                display_name=item.display_name,
                display_path=item.display_path,
                file_type=item.file_type,
                size=item.size,
                md5=md5,
                sha256=sha256,
                kind=kind,
                status="succeeded",
                rows_total=rows_total,
                rows_seen=rows_seen,
                rows_imported_raw=rows_imported_raw,
                rows_imported_norm=rows_imported_norm,
                rows_dedup=rows_dedup,
                rows_error=rows_error,
                note=note,
                error="",
                attempts=attempts,
                timings=timing_profile,
                rows_skipped_non_data=rows_skipped_non_data,
            )
        except Exception as exc:
            final_error = IMPORT_FILE_PROCESSING_FAILED
            retryable = private_exception_is_retryable(exc)
            try:
                update_import_progress(
                    engine,
                    item.file_id,
                    None,
                    status="失败",
                    error=final_error,
                )
            except Exception:
                pass

            return ImportExecutionResult(
                file_id=item.file_id,
                display_name=item.display_name,
                display_path=item.display_path,
                file_type=item.file_type,
                size=item.size,
                md5=md5,
                sha256=sha256,
                kind=kind,
                status="failed",
                rows_total=None,
                rows_seen=None,
                rows_imported_raw=None,
                rows_imported_norm=None,
                rows_dedup=None,
                rows_error=None,
                note="",
                error=final_error,
                attempts=attempts,
                rows_skipped_non_data=None,
                retryable=retryable,
            )

    def prepared_import_group_key(self, item: PreparedImportFile) -> Optional[tuple]:
        if item.duckdb_csv_path is None or item.duckdb_csv_path.suffix.lower() != ".csv":
            return None
        headers = [str(header or "") for header in (item.csv_headers or [])]
        if not headers:
            return None
        if any("\t" in header for header in headers):
            return None
        if any(header.strip().lower() == "filename" for header in headers):
            return None
        kind = item.kind_hint or detect_fc_kind(item.display_name, headers) or detect_fc_kind(item.real_path.name, headers) or ""
        if kind not in FC_SCHEMAS:
            return None
        mapping_key = tuple(
            sorted(
                (str(key or "").strip(), str(value or "").strip())
                for key, value in (item.field_mapping or {}).items()
                if str(key or "").strip() and str(value or "").strip()
            )
        )
        return kind, tuple(headers), mapping_key

    def run_file_import_group(
        self,
        *,
        engine: DuckDBEngine,
        case_id: str,
        items: Sequence[PreparedImportFile],
        attempts: int = 1,
    ) -> List[ImportExecutionResult]:
        group_items = list(items)
        if len(group_items) < 2:
            raise ValueError("group import requires at least two files")
        for item in group_items:
            self._validate_prepared_item(case_id, item)
            if item.duckdb_csv_path is None:
                raise ImmutableGenerationError("prepared_import_group_csv_generation_required")
        key = self.prepared_import_group_key(group_items[0])
        if key is None:
            raise ValueError("first file is not eligible for grouped import")
        kind = str(key[0])
        headers = list(key[1])
        mapping = group_items[0].field_mapping

        try:
            for item in group_items:
                item_key = self.prepared_import_group_key(item)
                if item_key != key:
                    raise ValueError("grouped import files must share kind, headers, and field mapping")
                source_receipt = item.ingestion_receipt.get("source")
                source_sha256 = (
                    str(source_receipt.get("sha256") or "") if isinstance(source_receipt, dict) else ""
                )
                sha256 = item.sha256 or source_sha256 or str(item.ingestion_receipt.get("sha256") or "")
                upsert_import_file_log(
                    engine,
                    file_id=item.file_id,
                    case_id=case_id,
                    kind=kind,
                    filename=item.display_name,
                    display_path=item.display_path,
                    stored_path=str(item.real_path),
                    file_type=item.file_type,
                    size=item.size,
                    md5="",
                    sha256=sha256,
                    rows_total=item.rows_total,
                    status="导入中",
                    error="",
                )

            group_profile: Dict[str, object] = {}
            group_results = import_fc_csv_group_into_db(
                engine=engine,
                case_id=case_id,
                kind=kind,
                sources=[
                    FcGroupImportSource(
                        file_id=item.file_id,
                        display_name=item.display_name,
                        path=item.duckdb_csv_path,
                        rows_total=item.rows_total,
                    )
                    for item in group_items
                    if item.duckdb_csv_path is not None
                ],
                headers=headers,
                field_mapping=mapping,
                profile_cb=lambda profile: group_profile.update(profile or {}),
            )
            by_file_id = {result.file_id: result for result in group_results}
            out: List[ImportExecutionResult] = []
            for item in group_items:
                result = by_file_id[item.file_id]
                result_counts = {
                    field: _require_import_count(getattr(result, field), field=field)
                    for field in (
                        "rows_total",
                        "rows_seen",
                        "rows_imported_raw",
                        "rows_imported_norm",
                        "rows_dedup",
                        "rows_error",
                        "rows_skipped_non_data",
                    )
                }
                update_import_progress(
                    engine,
                    item.file_id,
                    result_counts["rows_imported_norm"],
                    status="已完成",
                    error=project_import_note(result.note),
                    rows_total=result_counts["rows_total"],
                    rows_dedup=result_counts["rows_dedup"],
                    rows_error=result_counts["rows_error"],
                    rows_imported_raw=result_counts["rows_imported_raw"],
                    rows_imported_norm=result_counts["rows_imported_norm"],
                    rows_skipped_non_data=result_counts["rows_skipped_non_data"],
                )
                self._record_confirmed_mapping_template(
                    kind=kind,
                    headers=headers,
                    field_mapping=item.field_mapping,
                    field_mapping_origins=item.field_mapping_origins,
                    file_name=item.display_name,
                    rows_imported_norm=result_counts["rows_imported_norm"],
                )
                out.append(
                    ImportExecutionResult(
                        file_id=item.file_id,
                        display_name=item.display_name,
                        display_path=item.display_path,
                        file_type=item.file_type,
                        size=item.size,
                        md5="",
                        sha256=item.sha256 or "",
                        kind=kind,
                        status="succeeded",
                        rows_total=result_counts["rows_total"],
                        rows_seen=result_counts["rows_seen"],
                        rows_imported_raw=result_counts["rows_imported_raw"],
                        rows_imported_norm=result_counts["rows_imported_norm"],
                        rows_dedup=result_counts["rows_dedup"],
                        rows_error=result_counts["rows_error"],
                        note=project_import_note(result.note),
                        error="",
                        attempts=attempts,
                        timings=group_profile if not out else {},
                        rows_skipped_non_data=result_counts["rows_skipped_non_data"],
                    )
                )
            return out
        except Exception as exc:
            final_error = IMPORT_FILE_PROCESSING_FAILED
            retryable = private_exception_is_retryable(exc)
            out = []
            for item in group_items:
                try:
                    update_import_progress(
                        engine,
                        item.file_id,
                        None,
                        status="失败",
                        error=final_error,
                    )
                except Exception:
                    pass
                out.append(
                    ImportExecutionResult(
                        file_id=item.file_id,
                        display_name=item.display_name,
                        display_path=item.display_path,
                        file_type=item.file_type,
                        size=item.size,
                        md5="",
                        sha256=item.sha256 or "",
                        kind=kind,
                        status="failed",
                        rows_total=None,
                        rows_seen=None,
                        rows_imported_raw=None,
                        rows_imported_norm=None,
                        rows_dedup=None,
                        rows_error=None,
                        note="",
                        error=final_error,
                        attempts=attempts,
                        rows_skipped_non_data=None,
                        retryable=retryable,
                    )
                )
            return out

    def mark_file_canceled(
        self,
        engine: DuckDBEngine,
        file_id: str,
        *,
        rows_imported: Optional[int] = None,
    ) -> None:
        try:
            update_import_progress(
                engine,
                file_id=file_id,
                rows_imported=rows_imported,
                status="已取消",
                error=IMPORT_FILE_CANCELLED,
            )
        except Exception:
            pass

    def delete_import_file(self, case_id: str, file_id: str) -> bool:
        return bool(self.recycle_import_files(case_id, [file_id]))

    def delete_import_files(self, case_id: str, file_ids: Sequence[str]) -> List[str]:
        return self.recycle_import_files(case_id, file_ids)

    def recycle_import_files(self, case_id: str, file_ids: Sequence[str]) -> List[str]:
        normalized_ids = list(dict.fromkeys([str(file_id or "").strip() for file_id in file_ids if str(file_id or "").strip()]))
        if not normalized_ids:
            return []

        engine = self._storage.open_case_engine(case_id)
        try:
            ensure_fc_tables(engine)
            self._ensure_recycle_tables(engine)
            placeholders = self._sql_placeholders(len(normalized_ids))
            target_rows = [
                (
                    str(row[0] or ""),
                    str(row[1] or ""),
                    str(row[2] or ""),
                    str(row[3] or ""),
                    str(row[4] or ""),
                )
                for row in engine.query(
                    f"SELECT file_id, filename, kind, display_path, stored_path "
                    f"FROM import_file_log WHERE case_id=? AND file_id IN ({placeholders})",
                    [case_id, *normalized_ids],
                )
            ]
            if not target_rows:
                return []

            target_ids = [row[0] for row in target_rows if row[0]]
            target_where = f"case_id=? AND file_id IN ({self._sql_placeholders(len(target_ids))})"
            target_params: List[object] = [case_id, *target_ids]
            dataset_ids = self._resolve_dataset_ids_for_delete(engine, targets=target_rows, table="datasets")
            cleaning_log_ids = self._resolve_cleaning_log_ids(
                engine,
                table="cleaning_log",
                case_id=case_id,
                file_ids=target_ids,
            )
            recycled_at = self._now_text()

            engine.execute("BEGIN TRANSACTION")
            try:
                self._append_privacy_projection_delta_for_files(
                    engine,
                    case_id=case_id,
                    table="fc_transaction_norm",
                    file_ids=target_ids,
                    op="delete",
                    source="import:recycle",
                )
                detail_columns = set(self._table_columns(engine, "cleaning_log_detail"))
                if detail_columns:
                    if {"case_id", "file_id"}.issubset(detail_columns):
                        self._move_rows_to_shadow(
                            engine,
                            source_table="cleaning_log_detail",
                            shadow_table=self._recycle_table_name("cleaning_log_detail"),
                            where_clause=target_where,
                            params=target_params,
                            recycled_at=recycled_at,
                        )
                    elif {"case_id", "log_id"}.issubset(detail_columns) and cleaning_log_ids:
                        log_where = f"case_id=? AND log_id IN ({self._sql_placeholders(len(cleaning_log_ids))})"
                        self._move_rows_to_shadow(
                            engine,
                            source_table="cleaning_log_detail",
                            shadow_table=self._recycle_table_name("cleaning_log_detail"),
                            where_clause=log_where,
                            params=[case_id, *cleaning_log_ids],
                            recycled_at=recycled_at,
                        )

                self._move_rows_to_shadow(
                    engine,
                    source_table="cleaning_log",
                    shadow_table=self._recycle_table_name("cleaning_log"),
                    where_clause=target_where,
                    params=target_params,
                    recycled_at=recycled_at,
                )
                self._move_rows_to_shadow(
                    engine,
                    source_table="document_chunks",
                    shadow_table=self._recycle_table_name("document_chunks"),
                    where_clause=target_where,
                    params=target_params,
                    recycled_at=recycled_at,
                )
                self._move_rows_to_shadow(
                    engine,
                    source_table="document_assets",
                    shadow_table=self._recycle_table_name("document_assets"),
                    where_clause=target_where,
                    params=target_params,
                    recycled_at=recycled_at,
                )

                if dataset_ids:
                    dataset_where = f"dataset_id IN ({self._sql_placeholders(len(dataset_ids))})"
                    self._move_rows_to_shadow(
                        engine,
                        source_table="datasets",
                        shadow_table=self._recycle_table_name("datasets"),
                        where_clause=dataset_where,
                        params=dataset_ids,
                        recycled_at=recycled_at,
                    )

                for schema in FC_SCHEMAS.values():
                    for table in (raw_table_name(schema.table), norm_table_name(schema.table)):
                        self._move_rows_to_shadow(
                            engine,
                            source_table=table,
                            shadow_table=self._recycle_table_name(table),
                            where_clause=target_where,
                            params=target_params,
                            recycled_at=recycled_at,
                        )

                self._move_rows_to_shadow(
                    engine,
                    source_table="import_file_log",
                    shadow_table=self._recycle_table_name("import_file_log"),
                    where_clause=target_where,
                    params=target_params,
                    recycled_at=recycled_at,
                )
                engine.execute("COMMIT")
            except Exception:
                try:
                    engine.execute("ROLLBACK")
                except Exception:
                    pass
                raise

            try:
                bump_stats_flow_source_revision(engine, reason="import:recycle_files")
            except Exception:
                pass
            self.refresh_case_stats(case_id, engine=engine)
            self.refresh_analysis_aggregates(case_id, engine=engine)
            try:
                self._storage.record_case_audit(
                    case_id,
                    "recycle_import_files",
                    extra={
                        "mode": "recycle",
                        "affected_count": len(target_ids),
                        "file_ids": target_ids,
                        "recycled_at": recycled_at,
                        "files": [
                            {
                                "file_id": file_id,
                                "filename": filename,
                                "kind": kind,
                                "display_path": display_path,
                                "stored_path": stored_path,
                            }
                            for file_id, filename, kind, display_path, stored_path in target_rows
                        ],
                    },
                )
            except Exception:
                pass
            return target_ids
        finally:
            try:
                engine.close()
            except Exception:
                pass

    def restore_import_files(self, case_id: str, file_ids: Sequence[str]) -> List[str]:
        normalized_ids = list(dict.fromkeys([str(file_id or "").strip() for file_id in file_ids if str(file_id or "").strip()]))
        if not normalized_ids:
            return []

        engine = self._storage.open_case_engine(case_id)
        try:
            ensure_fc_tables(engine)
            self._ensure_recycle_tables(engine)
            placeholders = self._sql_placeholders(len(normalized_ids))
            recycle_table = self._recycle_table_name("import_file_log")
            target_rows = [
                (
                    str(row[0] or ""),
                    str(row[1] or ""),
                    str(row[2] or ""),
                    str(row[3] or ""),
                    str(row[4] or ""),
                )
                for row in engine.query(
                    f"SELECT file_id, filename, kind, display_path, stored_path "
                    f"FROM {recycle_table} WHERE case_id=? AND file_id IN ({placeholders})",
                    [case_id, *normalized_ids],
                )
            ]
            if not target_rows:
                return []

            target_ids = [row[0] for row in target_rows if row[0]]
            target_where = f"case_id=? AND file_id IN ({self._sql_placeholders(len(target_ids))})"
            target_params: List[object] = [case_id, *target_ids]
            dataset_ids = self._resolve_dataset_ids_for_delete(
                engine,
                targets=target_rows,
                table=self._recycle_table_name("datasets"),
            )
            cleaning_log_ids = self._resolve_cleaning_log_ids(
                engine,
                table=self._recycle_table_name("cleaning_log"),
                case_id=case_id,
                file_ids=target_ids,
            )

            engine.execute("BEGIN TRANSACTION")
            try:
                self._append_privacy_projection_delta_for_files(
                    engine,
                    case_id=case_id,
                    table=self._recycle_table_name("fc_transaction_norm"),
                    file_ids=target_ids,
                    op="upsert",
                    source="import:restore",
                )
                self._restore_rows_from_shadow(
                    engine,
                    target_table="import_file_log",
                    shadow_table=self._recycle_table_name("import_file_log"),
                    where_clause=target_where,
                    params=target_params,
                )

                for schema in FC_SCHEMAS.values():
                    for table in (raw_table_name(schema.table), norm_table_name(schema.table)):
                        self._restore_rows_from_shadow(
                            engine,
                            target_table=table,
                            shadow_table=self._recycle_table_name(table),
                            where_clause=target_where,
                            params=target_params,
                        )

                if dataset_ids:
                    dataset_where = f"dataset_id IN ({self._sql_placeholders(len(dataset_ids))})"
                    self._restore_rows_from_shadow(
                        engine,
                        target_table="datasets",
                        shadow_table=self._recycle_table_name("datasets"),
                        where_clause=dataset_where,
                        params=dataset_ids,
                    )

                self._restore_rows_from_shadow(
                    engine,
                    target_table="document_assets",
                    shadow_table=self._recycle_table_name("document_assets"),
                    where_clause=target_where,
                    params=target_params,
                )
                self._restore_rows_from_shadow(
                    engine,
                    target_table="document_chunks",
                    shadow_table=self._recycle_table_name("document_chunks"),
                    where_clause=target_where,
                    params=target_params,
                )
                self._restore_rows_from_shadow(
                    engine,
                    target_table="cleaning_log",
                    shadow_table=self._recycle_table_name("cleaning_log"),
                    where_clause=target_where,
                    params=target_params,
                )

                detail_table = self._recycle_table_name("cleaning_log_detail")
                detail_columns = set(self._table_columns(engine, detail_table))
                if detail_columns:
                    if {"case_id", "file_id"}.issubset(detail_columns):
                        self._restore_rows_from_shadow(
                            engine,
                            target_table="cleaning_log_detail",
                            shadow_table=detail_table,
                            where_clause=target_where,
                            params=target_params,
                        )
                    elif {"case_id", "log_id"}.issubset(detail_columns) and cleaning_log_ids:
                        log_where = f"case_id=? AND log_id IN ({self._sql_placeholders(len(cleaning_log_ids))})"
                        self._restore_rows_from_shadow(
                            engine,
                            target_table="cleaning_log_detail",
                            shadow_table=detail_table,
                            where_clause=log_where,
                            params=[case_id, *cleaning_log_ids],
                        )

                engine.execute("COMMIT")
            except Exception:
                try:
                    engine.execute("ROLLBACK")
                except Exception:
                    pass
                raise

            try:
                bump_stats_flow_source_revision(engine, reason="import:restore_files")
            except Exception:
                pass
            self.refresh_case_stats(case_id, engine=engine)
            self.refresh_analysis_aggregates(case_id, engine=engine)
            try:
                self._storage.record_case_audit(
                    case_id,
                    "restore_import_files",
                    extra={
                        "mode": "restore",
                        "affected_count": len(target_ids),
                        "file_ids": target_ids,
                        "files": [
                            {
                                "file_id": file_id,
                                "filename": filename,
                                "kind": kind,
                                "display_path": display_path,
                                "stored_path": stored_path,
                            }
                            for file_id, filename, kind, display_path, stored_path in target_rows
                        ],
                    },
                )
            except Exception:
                pass
            return target_ids
        finally:
            try:
                engine.close()
            except Exception:
                pass

    def purge_import_files(self, case_id: str, file_ids: Sequence[str]) -> List[str]:
        normalized_ids = list(dict.fromkeys([str(file_id or "").strip() for file_id in file_ids if str(file_id or "").strip()]))
        if not normalized_ids:
            return []

        engine = self._storage.open_case_engine(case_id)
        try:
            ensure_fc_tables(engine)
            self._ensure_recycle_tables(engine)
            placeholders = self._sql_placeholders(len(normalized_ids))
            recycle_table = self._recycle_table_name("import_file_log")
            target_rows = [
                (
                    str(row[0] or ""),
                    str(row[1] or ""),
                    str(row[2] or ""),
                    str(row[3] or ""),
                    str(row[4] or ""),
                )
                for row in engine.query(
                    f"SELECT file_id, filename, kind, display_path, stored_path "
                    f"FROM {recycle_table} WHERE case_id=? AND file_id IN ({placeholders})",
                    [case_id, *normalized_ids],
                )
            ]
            if not target_rows:
                return []

            target_ids = [row[0] for row in target_rows if row[0]]
            target_where = f"case_id=? AND file_id IN ({self._sql_placeholders(len(target_ids))})"
            target_params: List[object] = [case_id, *target_ids]
            dataset_ids = self._resolve_dataset_ids_for_delete(
                engine,
                targets=target_rows,
                table=self._recycle_table_name("datasets"),
            )
            cleaning_log_ids = self._resolve_cleaning_log_ids(
                engine,
                table=self._recycle_table_name("cleaning_log"),
                case_id=case_id,
                file_ids=target_ids,
            )
            content_paths = self._list_content_paths_for_file_ids(
                engine,
                case_id=case_id,
                table=self._recycle_table_name("document_assets"),
                file_ids=target_ids,
            )
            dataset_paths = self._list_dataset_paths(
                engine,
                table=self._recycle_table_name("datasets"),
                dataset_ids=dataset_ids,
            )
            raw_paths = [stored_path for _, _, _, _, stored_path in target_rows if stored_path]

            engine.execute("BEGIN TRANSACTION")
            try:
                self._append_privacy_projection_delta_for_files(
                    engine,
                    case_id=case_id,
                    table=self._recycle_table_name("fc_transaction_norm"),
                    file_ids=target_ids,
                    op="delete",
                    source="import:purge",
                )
                detail_table = self._recycle_table_name("cleaning_log_detail")
                detail_columns = set(self._table_columns(engine, detail_table))
                if detail_columns:
                    if {"case_id", "file_id"}.issubset(detail_columns):
                        self._delete_rows_from_table(
                            engine,
                            table=detail_table,
                            where_clause=target_where,
                            params=target_params,
                        )
                    elif {"case_id", "log_id"}.issubset(detail_columns) and cleaning_log_ids:
                        log_where = f"case_id=? AND log_id IN ({self._sql_placeholders(len(cleaning_log_ids))})"
                        self._delete_rows_from_table(
                            engine,
                            table=detail_table,
                            where_clause=log_where,
                            params=[case_id, *cleaning_log_ids],
                        )

                self._delete_rows_from_table(
                    engine,
                    table=self._recycle_table_name("cleaning_log"),
                    where_clause=target_where,
                    params=target_params,
                )
                self._delete_rows_from_table(
                    engine,
                    table=self._recycle_table_name("document_chunks"),
                    where_clause=target_where,
                    params=target_params,
                )
                self._delete_rows_from_table(
                    engine,
                    table=self._recycle_table_name("document_assets"),
                    where_clause=target_where,
                    params=target_params,
                )

                if dataset_ids:
                    dataset_where = f"dataset_id IN ({self._sql_placeholders(len(dataset_ids))})"
                    self._delete_rows_from_table(
                        engine,
                        table=self._recycle_table_name("datasets"),
                        where_clause=dataset_where,
                        params=dataset_ids,
                    )

                for schema in FC_SCHEMAS.values():
                    for table in (raw_table_name(schema.table), norm_table_name(schema.table)):
                        self._delete_rows_from_table(
                            engine,
                            table=self._recycle_table_name(table),
                            where_clause=target_where,
                            params=target_params,
                        )

                self._delete_rows_from_table(
                    engine,
                    table=self._recycle_table_name("import_file_log"),
                    where_clause=target_where,
                    params=target_params,
                )
                engine.execute("COMMIT")
            except Exception:
                try:
                    engine.execute("ROLLBACK")
                except Exception:
                    pass
                raise

            case_dir = self._storage.case_dir(case_id)
            self._cleanup_paths_if_unreferenced(
                engine,
                case_id=case_id,
                paths=raw_paths,
                allowed_root=case_dir / "raw",
                references=[
                    ("import_file_log", "stored_path"),
                    (self._recycle_table_name("import_file_log"), "stored_path"),
                ],
            )
            self._cleanup_paths_if_unreferenced(
                engine,
                case_id=case_id,
                paths=dataset_paths,
                allowed_root=case_dir / "datasets",
                references=[
                    ("datasets", "stored_path"),
                    (self._recycle_table_name("datasets"), "stored_path"),
                ],
            )
            self._cleanup_paths_if_unreferenced(
                engine,
                case_id=case_id,
                paths=content_paths,
                allowed_root=case_dir / "knowledge" / "texts",
                references=[
                    ("document_assets", "content_path"),
                    (self._recycle_table_name("document_assets"), "content_path"),
                ],
                delete_callback=lambda path: self._document_repository.delete_managed_text_path(
                    case_id=case_id,
                    content_path=path,
                ),
            )
            try:
                self._storage.record_case_audit(
                    case_id,
                    "purge_import_files",
                    extra={
                        "mode": "purge",
                        "affected_count": len(target_ids),
                        "file_ids": target_ids,
                        "files": [
                            {
                                "file_id": file_id,
                                "filename": filename,
                                "kind": kind,
                                "display_path": display_path,
                                "stored_path": stored_path,
                            }
                            for file_id, filename, kind, display_path, stored_path in target_rows
                        ],
                    },
                )
            except Exception:
                pass
            return target_ids
        finally:
            try:
                engine.close()
            except Exception:
                pass

    def _prepare_from_data_file(
        self,
        case_id: str,
        src_path: Path,
        *,
        display_name: str,
        source_label: str,
        kind_hint: Optional[str],
        password: Optional[str],
        field_mapping: Optional[Dict[str, str]],
        field_mapping_origins: Optional[Dict[str, str]] = None,
        verified_sha256: str,
        verified_source: RegularSourceReceipt,
    ) -> List[PreparedImportFile]:
        raw_dir = self._storage.case_dir(case_id) / "raw"
        raw_generation = self._copy_to_raw(
            src_path,
            raw_dir,
            verified_source=verified_source,
            encrypted=self._is_file_password_protected(src_path),
        )
        copied_path = raw_generation.path
        dst = self._maybe_decrypt_file(copied_path, raw_dir / "_decryptcache", password=password)
        ingestion_receipt = self._prepared_generation_receipt(
            dst,
            raw_generation=raw_generation,
            encrypted=dst != copied_path,
        )
        if dst.suffix.lower() in {".xlsx", ".xls"}:
            self._try_promote_excel_csv_cache(
                source_path=src_path,
                prepared_path=dst,
                target_cache_dir=raw_dir / "_excelcache",
                source_key="data",
                expected_excel_sha256=inspect_regular_source(dst).sha256 if dst != copied_path else None,
            )

        if not kind_hint and dst.suffix.lower() in {".xlsx", ".xls"}:
            split_files = self._split_excel_account_sections(dst, raw_dir / "_excelcache")
            if split_files:
                out: List[PreparedImportFile] = []
                for csv_path, split_kind in split_files:
                    kind_cn = _KIND_CN.get(split_kind, split_kind)
                    out.append(
                        PreparedImportFile(
                            file_id=self._new_file_id(f"{time.time()}|{csv_path}"),
                            display_name=f"{Path(display_name).stem}【{kind_cn}】.csv",
                            display_path=f"{source_label}::{kind_cn}",
                            real_path=csv_path,
                            file_type=self._file_type(csv_path),
                            size=csv_path.stat().st_size,
                            rows_total=0,
                            kind_hint=split_kind,
                            field_mapping=field_mapping,
                            field_mapping_origins=field_mapping_origins,
                            sha256=verified_sha256,
                            ingestion_receipt=self._derived_generation_receipt(
                                csv_path,
                                source_generation=raw_generation,
                                root_source=verified_source,
                                derivation="excel_account_section_split",
                                encrypted_source=bool(dst != copied_path),
                            ),
                        )
                    )
                self._prepare_import_csv_metadata_batch(out, raw_dir / "_duckdb_csv")
                return out

        out = [
            PreparedImportFile(
                file_id=self._new_file_id(f"{time.time()}|{dst}"),
                display_name=display_name or dst.name,
                display_path=source_label,
                real_path=dst,
                file_type=self._file_type(dst),
                size=dst.stat().st_size,
                rows_total=0,
                kind_hint=kind_hint,
                field_mapping=field_mapping,
                field_mapping_origins=field_mapping_origins,
                sha256=verified_sha256,
                csv_encoding="utf-8" if dst.suffix.lower() in {".xlsx", ".xls"} else "",
                ingestion_receipt=ingestion_receipt,
            )
        ]
        self._prepare_import_csv_metadata_batch(out, raw_dir / "_duckdb_csv")
        return out

    def _prepare_from_support_file(
        self,
        case_id: str,
        src_path: Path,
        *,
        display_name: str,
        source_label: str,
        kind_hint: Optional[str],
        password: Optional[str],
        field_mapping: Optional[Dict[str, str]],
        field_mapping_origins: Optional[Dict[str, str]] = None,
        verified_sha256: str,
        verified_source: RegularSourceReceipt,
    ) -> List[PreparedImportFile]:
        raw_dir = self._storage.case_dir(case_id) / "raw"
        raw_generation = self._copy_to_raw(
            src_path,
            raw_dir,
            verified_source=verified_source,
            encrypted=self._is_file_password_protected(src_path),
        )
        dst = raw_generation.path
        dst = self._maybe_decrypt_file(dst, raw_dir / "_decryptcache", password=password)
        return [
            PreparedImportFile(
                file_id=self._new_file_id(f"{time.time()}|{dst}|support"),
                display_name=display_name or dst.name,
                display_path=source_label,
                real_path=dst,
                file_type=self._file_type(dst),
                size=dst.stat().st_size,
                rows_total=0,
                kind_hint=kind_hint or _SUPPORT_FILE_KIND,
                field_mapping=field_mapping,
                field_mapping_origins=field_mapping_origins,
                sha256=verified_sha256,
                ingestion_receipt=self._prepared_generation_receipt(
                    dst,
                    raw_generation=raw_generation,
                    encrypted=dst != raw_generation.path,
                ),
            )
        ]

    def _prepare_from_zip(
        self,
        case_id: str,
        zip_path: Path,
        *,
        file_name: str,
        kind_hint: Optional[str],
        password: Optional[str],
        field_mapping: Optional[Dict[str, str]],
        field_mapping_origins: Optional[Dict[str, str]] = None,
        archive_items: Optional[List[ImportArchiveItemInput]],
        verified_archive_sha256: str,
        verified_source: RegularSourceReceipt,
    ) -> List[PreparedImportFile]:
        raw_dir = self._storage.case_dir(case_id) / "raw"
        override_by_path = _archive_override_map(archive_items)
        archive_pwd = password.encode("utf-8") if password else None
        planned_paths: set[str] = set()
        seen_preflight_paths: set[str] = set()

        def preflight_zip_inventory(
            archive: zipfile.ZipFile,
            *,
            parent_path: str = "",
            depth: int = 0,
        ) -> None:
            for info in archive.infolist():
                if info.is_dir():
                    continue
                member_name = self._fix_zip_name(
                    info.filename,
                    getattr(info, "flag_bits", 0),
                )
                archive_path = combine_archive_path(parent_path, member_name)
                if archive_path in seen_preflight_paths:
                    raise ValueError(f"duplicate archive entry identity: {archive_path}")
                seen_preflight_paths.add(archive_path)
                member_size = _require_import_count(
                    getattr(info, "file_size", None),
                    field="archive_member_size",
                )
                if member_size > _MAX_ZIP_MEMBER_BYTES:
                    raise ValueError(f"archive entry size invalid: {archive_path}")
                suffix = Path(member_name).suffix.lower()
                if suffix == ".zip":
                    if depth >= _MAX_NESTED_ZIP_DEPTH:
                        raise ValueError(f"nested archive depth exceeded: {archive_path}")
                    if member_size > _MAX_NESTED_ZIP_BYTES:
                        raise ValueError(f"nested archive size exceeded: {archive_path}")
                    try:
                        with archive.open(info, "r", pwd=archive_pwd) as nested_source:
                            payload = nested_source.read(_MAX_NESTED_ZIP_BYTES + 1)
                    except RuntimeError as exc:
                        raise ValueError(f"archive password invalid: {archive_path}") from exc
                    if len(payload) != member_size or len(payload) > _MAX_NESTED_ZIP_BYTES:
                        raise ValueError(f"nested archive size mismatch: {archive_path}")
                    try:
                        with zipfile.ZipFile(io.BytesIO(payload), "r") as nested_archive:
                            preflight_zip_inventory(
                                nested_archive,
                                parent_path=archive_path,
                                depth=depth + 1,
                            )
                    except zipfile.BadZipFile as exc:
                        raise ValueError(f"invalid nested zip archive: {archive_path}") from exc
                    continue
                if suffix in _SUPPORTED_DATA_EXTS or suffix in _SUPPORTED_DOCUMENT_EXTS:
                    planned_paths.add(archive_path)

        with open_verified_source(zip_path, verified_source) as zip_source:
            try:
                with zipfile.ZipFile(zip_source, "r") as archive:
                    preflight_zip_inventory(archive)
            except zipfile.BadZipFile as exc:
                raise ValueError(f"{file_name or zip_path.name} 不是有效 ZIP 压缩包") from exc
        if not planned_paths:
            raise ValueError("no csv/xlsx/xls/pdf/doc/docx/txt files found in zip archive")
        if planned_paths != set(override_by_path):
            raise ValueError("archive entry override does not match extracted inventory")

        archive_generation = self._copy_to_raw(
            zip_path,
            raw_dir,
            verified_source=verified_source,
            encrypted=self._zip_requires_password(zip_path),
        )
        zip_path = archive_generation.path
        cache_dir = raw_dir / "_zipcache"
        ensure_private_generation_root(cache_dir)

        unzip_dir = cache_dir / verified_archive_sha256.lower()
        ensure_private_generation_root(unzip_dir)

        prepared: List[PreparedImportFile] = []
        matched_override_paths: set[str] = set()
        seen_archive_paths: set[str] = set()

        def extract_member(
            archive: zipfile.ZipFile,
            info: zipfile.ZipInfo,
            *,
            archive_path: str,
            member_name: str,
            target_dir: Path,
            capture_csv_rows: bool,
            current_archive_path: Path,
        ) -> tuple[Path, int]:
            try:
                generation = publish_generated(
                    target_dir,
                    suffix=Path(member_name).suffix,
                    producer=lambda output: self._copy_zip_entry(archive, info, archive_pwd, output),
                    lineage={
                        "kind": "archive_entry",
                        "derived": True,
                        "archive_sha256": inspect_regular_source(current_archive_path).sha256,
                        "root_archive_sha256": verified_archive_sha256,
                        "archive_entry": archive_path,
                        "encrypted_source": bool(info.flag_bits & 0x1),
                    },
                )
            except ImmutableGenerationError:
                raise
            except RuntimeError as exc:
                if info.flag_bits & 0x1:
                    if not password:
                        raise ValueError(f"password required for encrypted archive: {zip_path.name}") from exc
                    raise ValueError(f"invalid password for encrypted archive: {zip_path.name}") from exc
                raise
            expected_size = _require_import_count(
                getattr(info, "file_size", None),
                field="archive_member_size",
            )
            if generation.size != expected_size:
                raise ValueError(f"archive entry size mismatch: {archive_path}")
            extracted_rows_total = self._count_csv_rows_fast(generation.path) if capture_csv_rows else 0
            return generation.path, extracted_rows_total

        def prepare_archive_entries(
            archive: zipfile.ZipFile,
            *,
            current_zip_path: Path,
            parent_path: str = "",
            depth: int = 0,
        ) -> None:
            for info in archive.infolist():
                if info.is_dir():
                    continue

                member_name = self._fix_zip_name(info.filename, getattr(info, "flag_bits", 0))
                archive_path = combine_archive_path(parent_path, member_name)
                if archive_path in seen_archive_paths:
                    raise ValueError(f"duplicate archive entry identity: {archive_path}")
                seen_archive_paths.add(archive_path)
                inner = Path(member_name)
                inner_suffix = inner.suffix.lower()
                if inner_suffix == ".zip":
                    if depth >= _MAX_NESTED_ZIP_DEPTH:
                        raise ValueError(f"nested archive depth exceeded: {archive_path}")
                    nested_dir = unzip_dir / "_nested_zips"
                    ensure_private_generation_root(nested_dir)
                    nested_zip_path, _ = extract_member(
                        archive,
                        info,
                        archive_path=archive_path,
                        member_name=member_name,
                        target_dir=nested_dir,
                        capture_csv_rows=False,
                        current_archive_path=current_zip_path,
                    )
                    try:
                        nested_receipt = inspect_regular_source(nested_zip_path)
                        with open_verified_source(nested_zip_path, nested_receipt) as nested_source:
                            with zipfile.ZipFile(nested_source, "r") as nested_archive:
                                prepare_archive_entries(
                                    nested_archive,
                                    current_zip_path=nested_zip_path,
                                    parent_path=archive_path,
                                    depth=depth + 1,
                                )
                    except zipfile.BadZipFile as exc:
                        raise ValueError(f"{file_name or zip_path.name}::{archive_path} 不是有效 ZIP 压缩包") from exc
                    continue

                if inner_suffix not in _SUPPORTED_DATA_EXTS and inner_suffix not in _SUPPORTED_DOCUMENT_EXTS:
                    continue

                out_path, _ = extract_member(
                    archive,
                    info,
                    archive_path=archive_path,
                    member_name=member_name,
                    target_dir=unzip_dir,
                    capture_csv_rows=inner_suffix == ".csv",
                    current_archive_path=current_zip_path,
                )
                out_path = self._maybe_decrypt_file(out_path, raw_dir / "_decryptcache", password=password)
                override = override_by_path.get(archive_path)
                if override is None:
                    raise ValueError(f"archive entry receipt missing: {archive_path}")
                matched_override_paths.add(archive_path)
                effective_kind_hint = override.file_kind if override and override.file_kind else kind_hint
                effective_field_mapping = override.field_mapping if override and override.field_mapping else field_mapping
                effective_field_mapping_origins = (
                    override.field_mapping_origins
                    if override and override.field_mapping_origins
                    else field_mapping_origins
                )
                if (
                    not str(override.expected_sha256 or "").strip()
                    or override.expected_size is None
                ):
                    raise ValueError(f"archive entry receipt missing: {archive_path}")
                effective_sha256 = self._resolve_verified_sha256(
                    out_path,
                    expected_sha256=override.expected_sha256,
                    source_label=f"{file_name or zip_path.name}::{archive_path}",
                )
                try:
                    inspect_regular_source(
                        out_path,
                        expected_sha256=effective_sha256,
                        expected_size=override.expected_size,
                    )
                except ImmutableGenerationError as exc:
                    raise ValueError(f"archive entry receipt mismatch: {archive_path}") from exc
                if out_path.suffix.lower() in {".xlsx", ".xls"}:
                    self._try_promote_excel_csv_cache(
                        source_path=current_zip_path,
                        prepared_path=out_path,
                        target_cache_dir=raw_dir / "_excelcache",
                        source_key=archive_path,
                        expected_excel_sha256=effective_sha256,
                    )

                source_label = f"{file_name or str(zip_path)}::{archive_path}"
                if inner_suffix in _SUPPORTED_DOCUMENT_EXTS:
                    prepared.append(
                        PreparedImportFile(
                            file_id=self._new_file_id(f"{time.time()}|{zip_path}|{archive_path}|support"),
                            display_name=inner.name,
                            display_path=source_label,
                            real_path=out_path,
                            file_type=self._file_type(out_path),
                            size=out_path.stat().st_size,
                            rows_total=0,
                            kind_hint=effective_kind_hint or _SUPPORT_FILE_KIND,
                            field_mapping=effective_field_mapping,
                            field_mapping_origins=effective_field_mapping_origins,
                            sha256=effective_sha256,
                            ingestion_receipt=self._derived_generation_receipt(
                                out_path,
                                source_generation=archive_generation,
                                root_source=verified_source,
                                derivation="zip_archive_entry",
                                encrypted_source=bool(info.flag_bits & 0x1),
                                source_entry=archive_path,
                            ),
                        )
                    )
                    continue

                if not effective_kind_hint and out_path.suffix.lower() in {".xlsx", ".xls"}:
                    split_files = self._split_excel_account_sections(out_path, raw_dir / "_excelcache")
                    if split_files:
                        for csv_path, split_kind in split_files:
                            kind_cn = _KIND_CN.get(split_kind, split_kind)
                            prepared.append(
                                PreparedImportFile(
                                    file_id=self._new_file_id(
                                        f"{time.time()}|{zip_path}|{archive_path}|{csv_path}"
                                    ),
                                    display_name=f"{inner.stem}【{kind_cn}】.csv",
                                    display_path=f"{source_label}::{kind_cn}",
                                    real_path=csv_path,
                                    file_type=self._file_type(csv_path),
                                    size=csv_path.stat().st_size,
                                    rows_total=0,
                                    kind_hint=split_kind,
                                    field_mapping=effective_field_mapping,
                                    field_mapping_origins=effective_field_mapping_origins,
                                    sha256=effective_sha256,
                                    ingestion_receipt=self._derived_generation_receipt(
                                        csv_path,
                                        source_generation=archive_generation,
                                        root_source=verified_source,
                                        derivation="zip_excel_account_section_split",
                                        encrypted_source=bool(info.flag_bits & 0x1),
                                        source_entry=archive_path,
                                    ),
                                )
                            )
                        continue

                prepared.append(
                    PreparedImportFile(
                        file_id=self._new_file_id(f"{time.time()}|{zip_path}|{archive_path}"),
                        display_name=inner.name,
                        display_path=source_label,
                        real_path=out_path,
                        file_type=self._file_type(out_path),
                        size=out_path.stat().st_size,
                        rows_total=0,
                        kind_hint=effective_kind_hint,
                        field_mapping=effective_field_mapping,
                        field_mapping_origins=effective_field_mapping_origins,
                        sha256=effective_sha256,
                        csv_encoding="utf-8" if out_path.suffix.lower() in {".xlsx", ".xls"} else "",
                        ingestion_receipt=self._derived_generation_receipt(
                            out_path,
                            source_generation=archive_generation,
                            root_source=verified_source,
                            derivation="zip_archive_entry",
                            encrypted_source=bool(info.flag_bits & 0x1),
                            source_entry=archive_path,
                        ),
                    )
                )

        self._validate_prepared_generation(zip_path, archive_generation.receipt())
        zip_receipt = inspect_regular_source(zip_path, expected_sha256=verified_archive_sha256)
        with open_verified_source(zip_path, zip_receipt) as zip_source:
            with zipfile.ZipFile(zip_source, "r") as archive:
                prepare_archive_entries(archive, current_zip_path=zip_path)

        if matched_override_paths != set(override_by_path):
            raise ValueError("archive entry override does not match extracted inventory")
        if not prepared:
            raise ValueError("no csv/xlsx/xls/pdf/doc/docx/txt files found in zip archive")
        self._prepare_import_csv_metadata_batch(prepared, raw_dir / "_duckdb_csv")
        return prepared

    def _prepare_from_archive(
        self,
        case_id: str,
        archive_path: Path,
        *,
        file_name: str,
        kind_hint: Optional[str],
        password: Optional[str],
        field_mapping: Optional[Dict[str, str]],
        field_mapping_origins: Optional[Dict[str, str]] = None,
        archive_items: Optional[List[ImportArchiveItemInput]],
        verified_archive_sha256: str,
        verified_source: RegularSourceReceipt,
    ) -> List[PreparedImportFile]:
        raw_dir = self._storage.case_dir(case_id) / "raw"
        override_by_path = _archive_override_map(archive_items)
        archive_generation = self._copy_to_raw(
            archive_path,
            raw_dir,
            verified_source=verified_source,
            encrypted="unknown",
        )
        archive_path = archive_generation.path
        extract_dir = self._archive_extract_root(case_id, archive_path, verified_archive_sha256)
        archive_cache_root = extract_dir.parent
        archive_member_root = archive_cache_root / "_inventory_quarantine"
        self._validate_prepared_generation(archive_path, archive_generation.receipt())

        prepared: List[PreparedImportFile] = []
        matched_override_paths: set[str] = set()
        planned_leaf_members: List[_VerifiedArchiveMemberGeneration] = []
        seen_entry_paths: set[str] = set()

        def append_prepared_file(
            *,
            out_path: Path,
            entry_path: str,
            source_archive_path: Path,
            source_generation: ImmutableGeneration,
        ) -> None:
            logical_name = archive_entry_leaf_name(entry_path)
            inner_suffix = Path(logical_name).suffix.lower()
            override = override_by_path.get(entry_path)
            if override is None:
                raise ValueError(f"archive entry receipt missing: {entry_path}")
            matched_override_paths.add(entry_path)
            effective_kind_hint = override.file_kind if override and override.file_kind else kind_hint
            effective_field_mapping = override.field_mapping if override and override.field_mapping else field_mapping
            effective_field_mapping_origins = (
                override.field_mapping_origins
                if override and override.field_mapping_origins
                else field_mapping_origins
            )
            if (
                not str(override.expected_sha256 or "").strip()
                or override.expected_size is None
            ):
                raise ValueError(f"archive entry receipt missing: {entry_path}")
            effective_sha256 = self._resolve_verified_sha256(
                out_path,
                expected_sha256=override.expected_sha256,
                source_label=f"{file_name or archive_path.name}::{entry_path}",
            )
            try:
                inspect_regular_source(
                    out_path,
                    expected_sha256=effective_sha256,
                    expected_size=override.expected_size,
                )
            except ImmutableGenerationError as exc:
                raise ValueError(f"archive entry receipt mismatch: {entry_path}") from exc
            if inner_suffix in {".xlsx", ".xls"}:
                self._try_promote_excel_csv_cache(
                    source_path=source_archive_path,
                    prepared_path=out_path,
                    target_cache_dir=raw_dir / "_excelcache",
                    source_key=entry_path,
                    expected_excel_sha256=effective_sha256,
                )

            source_label = f"{file_name or str(archive_path)}::{entry_path}"
            if inner_suffix in _SUPPORTED_DOCUMENT_EXTS:
                prepared.append(
                    PreparedImportFile(
                        file_id=self._new_file_id(f"{time.time()}|{archive_path}|{entry_path}|support"),
                        display_name=logical_name,
                        display_path=source_label,
                        real_path=out_path,
                        file_type=inner_suffix.replace(".", "").upper(),
                        size=out_path.stat().st_size,
                        rows_total=0,
                        kind_hint=effective_kind_hint or _SUPPORT_FILE_KIND,
                        field_mapping=effective_field_mapping,
                        field_mapping_origins=effective_field_mapping_origins,
                        sha256=effective_sha256,
                        ingestion_receipt=self._derived_generation_receipt(
                            out_path,
                            source_generation=source_generation,
                            root_source=verified_source,
                            derivation="archive_entry",
                            encrypted_source="unknown",
                            source_entry=entry_path,
                        ),
                    )
                )
                return

            if not effective_kind_hint and inner_suffix in {".xlsx", ".xls"}:
                split_files = self._split_excel_account_sections(out_path, raw_dir / "_excelcache")
                if split_files:
                    for csv_path, split_kind in split_files:
                        kind_cn = _KIND_CN.get(split_kind, split_kind)
                        prepared.append(
                            PreparedImportFile(
                                file_id=self._new_file_id(f"{time.time()}|{archive_path}|{entry_path}|{csv_path}"),
                                display_name=f"{Path(logical_name).stem}【{kind_cn}】.csv",
                                display_path=f"{source_label}::{kind_cn}",
                                real_path=csv_path,
                                file_type=self._file_type(csv_path),
                                size=csv_path.stat().st_size,
                                rows_total=0,
                                kind_hint=split_kind,
                                field_mapping=effective_field_mapping,
                                field_mapping_origins=effective_field_mapping_origins,
                                sha256=effective_sha256,
                                ingestion_receipt=self._derived_generation_receipt(
                                    csv_path,
                                    source_generation=source_generation,
                                    root_source=verified_source,
                                    derivation="archive_excel_account_section_split",
                                    encrypted_source="unknown",
                                    source_entry=entry_path,
                                ),
                            )
                        )
                    return

            prepared.append(
                PreparedImportFile(
                    file_id=self._new_file_id(f"{time.time()}|{archive_path}|{entry_path}"),
                    display_name=logical_name,
                    display_path=source_label,
                    real_path=out_path,
                    file_type=inner_suffix.replace(".", "").upper(),
                    size=out_path.stat().st_size,
                    rows_total=0,
                    kind_hint=effective_kind_hint,
                    field_mapping=effective_field_mapping,
                    field_mapping_origins=effective_field_mapping_origins,
                    sha256=effective_sha256,
                    csv_encoding="utf-8" if inner_suffix in {".xlsx", ".xls"} else "",
                    ingestion_receipt=self._derived_generation_receipt(
                        out_path,
                        source_generation=source_generation,
                        root_source=verified_source,
                        derivation="archive_entry",
                        encrypted_source="unknown",
                        source_entry=entry_path,
                    ),
                )
            )

        pending_leaf_members: List[
            tuple[
                ArchiveExtractionLeaseV1,
                ArchiveMemberReceiptV1,
                str,
                Path,
            ]
        ] = []
        with ExitStack() as extraction_leases:
            def inventory_extracted_entries(
                *,
                root: Path,
                source_archive_path: Path,
                source_archive_sha256: str,
                parent_path: str = "",
                depth: int = 0,
            ) -> None:
                lease = extraction_leases.enter_context(
                    open_archive_extraction_v1(
                        source_archive_path,
                        root,
                        password=password,
                        expected_sha256=source_archive_sha256,
                        reuse=True,
                    )
                )
                for member in lease.file_members():
                    suffix = Path(member.relative_path).suffix.lower()
                    if suffix not in _SUPPORTED_ARCHIVE_EXTS and suffix not in _SUPPORTED_DATA_EXTS and suffix not in _SUPPORTED_DOCUMENT_EXTS:
                        continue
                    entry_path = combine_archive_path(
                        parent_path,
                        member.relative_path,
                    )
                    if entry_path in seen_entry_paths:
                        raise ValueError(
                            f"duplicate archive entry identity: {entry_path}"
                        )
                    seen_entry_paths.add(entry_path)
                    if suffix in _SUPPORTED_ARCHIVE_EXTS:
                        if depth >= _MAX_NESTED_ARCHIVE_DEPTH:
                            raise ValueError(
                                f"nested archive depth exceeded: {entry_path}"
                            )
                        generation = lease.publish_member(
                            member,
                            archive_member_root,
                            lineage={
                                "kind": "case_archive_inventory_quarantine",
                                "case_id": case_id,
                                "logical_parent_path": parent_path,
                                "logical_entry_path": entry_path,
                                "authoritative": False,
                            },
                        )
                        nested_root = (
                            archive_cache_root
                            / "_nested_archives"
                            / archive_cache_output_name(entry_path)
                        )
                        inventory_extracted_entries(
                            root=nested_root,
                            source_archive_path=generation.path,
                            source_archive_sha256=generation.sha256,
                            parent_path=entry_path,
                            depth=depth + 1,
                        )
                        continue
                    pending_leaf_members.append(
                        (lease, member, entry_path, source_archive_path)
                    )

            inventory_extracted_entries(
                root=extract_dir,
                source_archive_path=archive_path,
                source_archive_sha256=verified_archive_sha256,
            )

            planned_paths = {
                entry_path
                for _lease, _member, entry_path, _source_path
                in pending_leaf_members
            }
            if planned_paths != set(override_by_path):
                raise ValueError(
                    "archive entry override does not match extracted inventory"
                )
            for lease, member, entry_path, source_archive_path in pending_leaf_members:
                generation = lease.publish_member(
                    member,
                    archive_member_root,
                    lineage={
                        "kind": "case_archive_inventory_quarantine",
                        "case_id": case_id,
                        "logical_entry_path": entry_path,
                        "authoritative": False,
                    },
                )
                planned_leaf_members.append(
                    _VerifiedArchiveMemberGeneration(
                        logical_path=entry_path,
                        member=member,
                        generation=generation,
                        source_archive_path=source_archive_path,
                    )
                )
        verified_consumed_members: List[
            tuple[_VerifiedArchiveMemberGeneration, Path]
        ] = []
        for verified in planned_leaf_members:
            entry_path = verified.logical_path
            override = override_by_path[entry_path]
            decrypted_path = self._maybe_decrypt_file(
                verified.generation.path,
                raw_dir / "_decryptcache",
                password=password,
            )
            try:
                inspect_regular_source(
                    decrypted_path,
                    expected_sha256=override.expected_sha256,
                    expected_size=override.expected_size,
                )
            except ImmutableGenerationError as exc:
                raise ValueError(
                    f"archive entry receipt mismatch: {entry_path}"
                ) from exc
            verified_consumed_members.append((verified, decrypted_path))

        for verified, decrypted_path in verified_consumed_members:
            append_prepared_file(
                out_path=decrypted_path,
                entry_path=verified.logical_path,
                source_archive_path=verified.source_archive_path,
                source_generation=verified.generation,
            )

        if matched_override_paths != planned_paths:
            raise ValueError("archive entry override does not match extracted inventory")
        if not prepared:
            raise ValueError("no csv/xlsx/xls/pdf/doc/docx/txt files found in archive")
        self._prepare_import_csv_metadata_batch(prepared, raw_dir / "_duckdb_csv")
        return prepared

    @staticmethod
    def _new_file_id(seed: str) -> str:
        return hashlib.md5(seed.encode("utf-8")).hexdigest()[:20]

    @staticmethod
    def _count_csv_rows_fast(path: Path) -> int:
        try:
            receipt = inspect_regular_source(path)
            with open_verified_source(path, receipt) as handle:
                line_count = sum(chunk.count(b"\n") for chunk in iter(lambda: handle.read(1024 * 1024), b""))
        except Exception:
            return 0
        return max(line_count - 1, 0)

    @staticmethod
    def _resolve_verified_sha256(path: Path, *, expected_sha256: Optional[str], source_label: str) -> str:
        try:
            receipt = inspect_regular_source(path, expected_sha256=expected_sha256)
        except ImmutableGenerationError as exc:
            label = source_label or path.name
            raise ValueError(f"{label} 在预检后已发生变更，请重新执行 SHA-256 检验后再导入。") from exc
        return receipt.sha256

    @staticmethod
    def _zip_requires_password(path: Path) -> bool:
        try:
            with zipfile.ZipFile(path, "r") as archive:
                return any((info.flag_bits & 0x1) for info in archive.infolist() if not info.is_dir())
        except Exception:
            return False

    @staticmethod
    def _file_type(path: Path) -> str:
        return path.suffix.lower().replace(".", "").upper()

    def _is_file_password_protected(self, path: Path) -> bool:
        suffix = path.suffix.lower()
        if suffix == ".pdf":
            return self._is_pdf_password_protected(path)
        if suffix in {".doc", ".docx", ".xls", ".xlsx"}:
            return self._is_office_password_protected(path)
        if suffix == ".zip":
            return self._zip_requires_password(path)
        return False

    @staticmethod
    def _is_pdf_password_protected(path: Path) -> bool:
        try:
            receipt = inspect_regular_source(path)
            with open_verified_source(path, receipt) as source:
                reader = PdfReader(source)
                return bool(reader.is_encrypted)
        except Exception:
            return False

    @staticmethod
    def _is_office_password_protected(path: Path) -> bool:
        try:
            receipt = inspect_regular_source(path)
            with open_verified_source(path, receipt) as handle:
                office = OfficeFile(handle)
                return bool(office.is_encrypted())
        except msoffcrypto_exceptions.FileFormatError:
            return False
        except Exception:
            return False

    def _maybe_decrypt_file(self, path: Path, cache_dir: Path, *, password: Optional[str]) -> Path:
        suffix = path.suffix.lower()
        if suffix == ".pdf" and self._is_pdf_password_protected(path):
            if not password:
                raise ValueError(f"password required for encrypted file: {path.name}")
            return self._decrypt_pdf_file(path, cache_dir, password)
        if suffix in {".doc", ".docx", ".xls", ".xlsx"} and self._is_office_password_protected(path):
            if not password:
                raise ValueError(f"password required for encrypted file: {path.name}")
            return self._decrypt_office_file(path, cache_dir, password)
        return path

    def _decrypt_pdf_file(self, path: Path, cache_dir: Path, password: str) -> Path:
        source_receipt = inspect_regular_source(path)
        with open_verified_source(path, source_receipt) as source:
            reader = PdfReader(source)
            if not reader.is_encrypted:
                return path
            result = reader.decrypt(password)
            if not result:
                raise ValueError(f"invalid password for encrypted file: {path.name}")
            writer = PdfWriter()
            writer.clone_document_from_reader(reader)
            generation = publish_generated(
                cache_dir,
                suffix=path.suffix,
                producer=lambda output: writer.write(output),
                lineage={
                    "kind": "decrypted_derivative",
                    "encrypted_source": True,
                    "derived": True,
                    "derived_from_sha256": source_receipt.sha256,
                    "format": "pdf",
                },
            )
        return generation.path

    def _decrypt_office_file(self, path: Path, cache_dir: Path, password: str) -> Path:
        source_receipt = inspect_regular_source(path)
        with open_verified_source(path, source_receipt) as handle:
            office = OfficeFile(handle)
            if not office.is_encrypted():
                return path
            try:
                office.load_key(password=password, verify_password=True)
            except msoffcrypto_exceptions.InvalidKeyError as exc:
                raise ValueError(f"invalid password for encrypted file: {path.name}") from exc
            generation = publish_generated(
                cache_dir,
                suffix=path.suffix,
                producer=lambda output: office.decrypt(output),
                lineage={
                    "kind": "decrypted_derivative",
                    "encrypted_source": True,
                    "derived": True,
                    "derived_from_sha256": source_receipt.sha256,
                    "format": "office",
                },
            )
        return generation.path

    def _prepare_csv_with_profile_for_preview(
        self,
        path: Path,
        *,
        limit: int,
        encoding: Optional[str],
        profiler: Optional[ImportPreviewProfiler],
        phase_name: str,
    ) -> _PreparedCsvProfilePreview:
        if profiler:
            profiler.count("profile_scans")
        scan_started = time.perf_counter()
        try:
            prepared_with_profile = prepare_csv_with_profile(path, limit=limit, encoding=encoding)
        except ImportAcceleratorUnavailableError:
            prepared_with_profile = self._prepare_csv_with_profile_python(path, limit=limit, encoding=encoding)
        scan_elapsed_s = time.perf_counter() - scan_started
        if profiler:
            profiler.add_phase(phase_name, scan_elapsed_s)
        return _PreparedCsvProfilePreview(
            value=prepared_with_profile,
            elapsed_s=scan_elapsed_s,
        )

    @staticmethod
    def _detect_csv_encoding_python(path: Path, preferred: Optional[str] = None) -> str:
        candidates = [
            str(preferred or "").strip().lower(),
            "utf-8-sig",
            "utf-8",
            "gb18030",
            "gbk",
        ]
        receipt = inspect_regular_source(path)
        with open_verified_source(path, receipt) as source:
            sample = source.read(65536)
        for encoding in [item for item in candidates if item]:
            try:
                sample.decode(encoding)
                return encoding
            except Exception:
                continue
        return "gb18030"

    def _prepare_csv_with_profile_python(
        self,
        path: Path,
        *,
        limit: int,
        encoding: Optional[str],
    ) -> PreparedCsvWithProfile:
        resolved_encoding = self._detect_csv_encoding_python(path, encoding)
        rows_total = 0
        headers: List[str] = []
        sample_rows: List[List[str]] = []
        sample_limit = _require_import_count(limit, field="preview_sample_limit")
        source_receipt = inspect_regular_source(path)
        with open_verified_text(
            path,
            source_receipt,
            encoding=resolved_encoding,
            errors="replace",
            newline="",
        ) as handle:
            reader = csv.reader(handle, skipinitialspace=True)
            headers = [str(item or "") for item in next(reader, [])]
            for row in reader:
                rows_total += 1
                if len(sample_rows) < sample_limit:
                    sample_rows.append([str(item or "") for item in row])
        return PreparedCsvWithProfile(
            prepared=PreparedCsv(
                encoding=resolved_encoding,
                rows_total=rows_total,
                columns_total=len(headers),
                header_preview=headers,
                sample_rows=sample_rows,
            ),
            profiles=None,
            elapsed_s=None,
        )

    @staticmethod
    def _copy_stream_to_path(src, dst, *, capture_csv_rows: bool = False) -> int:
        newline_count = 0
        bytes_written = 0
        ends_with_newline = True
        while True:
            chunk = src.read(1024 * 1024)
            if not chunk:
                break
            dst.write(chunk)
            if capture_csv_rows:
                newline_count += chunk.count(b"\n")
                bytes_written += len(chunk)
                ends_with_newline = chunk.endswith(b"\n")
        if not capture_csv_rows or bytes_written <= 0:
            return 0
        physical_lines = newline_count + (0 if ends_with_newline else 1)
        return max(physical_lines - 1, 0)

    def _count_rows_fast(self, path: Path) -> int:
        if path.suffix.lower() != ".csv":
            return 0
        try:
            return _require_import_count(
                prepare_csv(path, limit=0).rows_total,
                field="estimated_rows_total",
            )
        except Exception:
            return 0

    def _prepare_import_csv_metadata_batch(self, items: Sequence[PreparedImportFile], cache_dir: Path) -> None:
        csv_items = [item for item in items if item.real_path.suffix.lower() == ".csv"]
        if not csv_items:
            return
        ensure_private_generation_root(cache_dir)
        accelerator_available = True
        promoted_receipts: Dict[str, Dict[str, object]] = {}
        try:
            work_root = cache_dir / "_work"
            ensure_private_generation_root(work_root)
            with private_work_directory(work_root) as work_dir:
                clean_paths = [self._duckdb_csv_cache_path(item.real_path, work_dir) for item in csv_items]
                opaque_inputs = []
                for item in csv_items:
                    source_receipt = inspect_regular_source(item.real_path)
                    opaque_inputs.append(
                        publish_source(
                            item.real_path,
                            work_dir,
                            expected=source_receipt,
                            suffix=item.real_path.suffix,
                            lineage={
                                "kind": "csv_accelerator_input",
                                "derived": False,
                                "source_sha256": source_receipt.sha256,
                                "encrypted_source": False,
                            },
                        ).path
                    )
                raw_prepared_items = prepare_csv_batch(
                    [
                        PrepareCsvBatchItem(
                            path=opaque_input,
                            out_path=clean_path,
                            limit=0,
                            clean_if_needed=True,
                        )
                        for opaque_input, clean_path in zip(opaque_inputs, clean_paths)
                    ]
                )
                if len(raw_prepared_items) != len(csv_items):
                    raise ImportAcceleratorUnavailableError("import accelerator returned mismatched batch")
                prepared_items = []
                for item, prepared in zip(csv_items, raw_prepared_items):
                    if prepared.output is None:
                        prepared_items.append(prepared)
                        continue
                    generated = inspect_regular_source(prepared.output)
                    source = inspect_regular_source(item.real_path)
                    promoted = publish_source(
                        prepared.output,
                        cache_dir,
                        expected=generated,
                        suffix=".csv",
                        lineage={
                            "kind": "duckdb_csv_derivative",
                            "derived": True,
                            "derived_from_sha256": source.sha256,
                            "transform": "prepare_csv_clean_v1",
                            "encrypted_source": False,
                        },
                    )
                    promoted_receipts[str(promoted.path)] = promoted.receipt()
                    prepared_items.append(replace(prepared, output=promoted.path))
                if len(prepared_items) != len(csv_items):
                    raise ImportAcceleratorUnavailableError("import accelerator returned incomplete batch")
        except ImportAcceleratorUnavailableError:
            accelerator_available = False
            prepared_items = [
                self._prepare_csv_with_profile_python(item.real_path, limit=0, encoding=None).prepared
                for item in csv_items
            ]
        for item, prepared in zip(csv_items, prepared_items):
            duckdb_csv_path = (prepared.output or item.real_path) if accelerator_available else None
            item.rows_total = prepared.rows_total
            item.csv_encoding = prepared.encoding
            item.csv_headers = list(prepared.header_preview or [])
            item.duckdb_csv_path = duckdb_csv_path
            item.duckdb_csv_encoding = "utf-8" if prepared.output is not None else ""
            if duckdb_csv_path is not None:
                item.duckdb_csv_receipt = (
                    promoted_receipts.get(str(duckdb_csv_path), {})
                    if duckdb_csv_path != item.real_path
                    else dict(item.ingestion_receipt)
                )

    @staticmethod
    def _duckdb_csv_cache_path(path: Path, cache_dir: Path) -> Path:
        file_tag = hashlib.md5(str(path).encode("utf-8")).hexdigest()[:10]
        return cache_dir / f"{path.stem}_{file_tag}.duckdb.csv"

    @staticmethod
    def _copy_to_raw(
        src_path: Path,
        raw_dir: Path,
        *,
        verified_source: RegularSourceReceipt,
        encrypted: object,
    ) -> ImmutableGeneration:
        return publish_source(
            src_path,
            raw_dir,
            expected=verified_source,
            expected_sha256=verified_source.sha256,
            expected_size=verified_source.size,
            suffix=src_path.suffix,
            lineage={
                "kind": "raw_source",
                "encrypted_source": encrypted if isinstance(encrypted, bool) else str(encrypted or "unknown"),
                "derived": False,
                "source_sha256": verified_source.sha256,
            },
        )

    @staticmethod
    def _prepared_generation_receipt(
        path: Path,
        *,
        raw_generation: ImmutableGeneration,
        encrypted: bool,
    ) -> Dict[str, object]:
        if raw_generation.source is None:
            raise ImmutableGenerationError("prepared_import_source_receipt_required")
        if path == raw_generation.path:
            return raw_generation.receipt()
        prepared = inspect_published_generation(path)
        return {
            "schema_version": 1,
            "publication": "immutable_no_overwrite_generation",
            "sha256": prepared.sha256,
            "size": prepared.size,
            "device": prepared.device,
            "inode": prepared.inode,
            "root_device": prepared.root_device,
            "root_inode": prepared.root_inode,
            "source": raw_generation.source.as_dict(),
            "lineage": {
                "kind": "decrypted_derivative" if encrypted else "raw_source",
                "encrypted_source": bool(dict(raw_generation.lineage).get("encrypted_source")),
                "derived": bool(encrypted),
                "derived_from_sha256": raw_generation.sha256 if encrypted else "",
                "source_sha256": raw_generation.source.sha256,
            },
        }

    @staticmethod
    def _derived_generation_receipt(
        path: Path,
        *,
        source_generation: ImmutableGeneration,
        root_source: RegularSourceReceipt,
        derivation: str,
        encrypted_source: object,
        source_entry: str = "",
    ) -> Dict[str, object]:
        derived = inspect_published_generation(path)
        return {
            "schema_version": 1,
            "publication": "immutable_no_overwrite_generation",
            "sha256": derived.sha256,
            "size": derived.size,
            "device": derived.device,
            "inode": derived.inode,
            "root_device": derived.root_device,
            "root_inode": derived.root_inode,
            "source": root_source.as_dict(),
            "lineage": {
                "kind": "derived_import_artifact",
                "encrypted_source": (
                    encrypted_source
                    if isinstance(encrypted_source, bool)
                    else str(encrypted_source or "unknown")
                ),
                "derived": True,
                "derivation": str(derivation),
                "derived_from_sha256": source_generation.sha256,
                "immediate_parent_sha256": source_generation.sha256,
                "root_source_sha256": root_source.sha256,
                "source_entry": str(source_entry or ""),
                "source_generation_lineage": dict(source_generation.lineage),
            },
        }

    @staticmethod
    def _prepared_receipt_registry_digest(receipt: Dict[str, object]) -> str:
        try:
            payload = json.dumps(
                receipt,
                ensure_ascii=False,
                sort_keys=True,
                separators=(",", ":"),
            ).encode("utf-8")
        except (TypeError, ValueError) as exc:
            raise ImmutableGenerationError("prepared_import_generation_receipt_invalid") from exc
        return hashlib.sha256(payload).hexdigest().upper()

    def _ensure_prepared_generation_registry(self) -> None:
        if hasattr(self, "_prepared_generation_registry") and hasattr(
            self,
            "_prepared_generation_registry_lock",
        ):
            return
        with self._prepared_generation_registry_bootstrap_lock:
            if not hasattr(self, "_prepared_generation_registry"):
                self._prepared_generation_registry = {}
            if not hasattr(self, "_prepared_generation_registry_lock"):
                self._prepared_generation_registry_lock = RLock()

    def _register_case_bound_generation(
        self,
        *,
        case_id: str,
        authority_root: Path,
        path: Path,
        receipt: Dict[str, object],
    ) -> Dict[str, object]:
        bound, digest = self._build_case_bound_generation_receipt(
            case_id=case_id,
            authority_root=authority_root,
            path=path,
            receipt=receipt,
        )
        self._ensure_prepared_generation_registry()
        with self._prepared_generation_registry_lock:
            receipt_id = str(bound["receipt_id"])
            if receipt_id in self._prepared_generation_registry:
                raise ImmutableGenerationError(
                    "prepared_import_generation_receipt_collision"
                )
            self._prepared_generation_registry[receipt_id] = digest
        return bound

    def _build_case_bound_generation_receipt(
        self,
        *,
        case_id: str,
        authority_root: Path,
        path: Path,
        receipt: Dict[str, object],
    ) -> tuple[Dict[str, object], str]:
        self._validate_prepared_generation(path, receipt)
        root = inspect_private_generation_root(authority_root)
        try:
            relative_path = path.relative_to(authority_root)
        except ValueError as exc:
            raise ImmutableGenerationError("prepared_import_generation_outside_case_root") from exc
        if not relative_path.parts or any(part in {"", ".", ".."} for part in relative_path.parts):
            raise ImmutableGenerationError("prepared_import_generation_outside_case_root")

        bound = dict(receipt)
        bound.update(
            {
                "schema_version": 2,
                "receipt_id": secrets.token_hex(32),
                "case_id": str(case_id),
                "authority_root_device": root.device,
                "authority_root_inode": root.inode,
                "authority_relative_path": relative_path.as_posix(),
            }
        )
        digest = self._prepared_receipt_registry_digest(bound)
        return bound, digest

    def _bind_prepared_items_to_case(
        self,
        *,
        case_id: str,
        authority_root: Path,
        items: Sequence[PreparedImportFile],
    ) -> None:
        admissions: list[
            tuple[PreparedImportFile, str, Dict[str, object], str]
        ] = []
        for item in items:
            bound, digest = self._build_case_bound_generation_receipt(
                case_id=case_id,
                authority_root=authority_root,
                path=item.real_path,
                receipt=item.ingestion_receipt,
            )
            admissions.append((item, "ingestion_receipt", bound, digest))
            if item.duckdb_csv_path is None:
                continue
            if item.duckdb_csv_path == item.real_path:
                admissions.append(
                    (item, "duckdb_csv_receipt", bound, digest)
                )
                continue
            derivative_bound, derivative_digest = (
                self._build_case_bound_generation_receipt(
                    case_id=case_id,
                    authority_root=authority_root,
                    path=item.duckdb_csv_path,
                    receipt=item.duckdb_csv_receipt,
                )
            )
            admissions.append(
                (
                    item,
                    "duckdb_csv_receipt",
                    derivative_bound,
                    derivative_digest,
                )
            )

        unique_admissions: dict[str, str] = {}
        for _item, _attribute, bound, digest in admissions:
            receipt_id = str(bound["receipt_id"])
            prior = unique_admissions.get(receipt_id)
            if prior is not None and prior != digest:
                raise ImmutableGenerationError(
                    "prepared_import_generation_receipt_collision"
                )
            unique_admissions[receipt_id] = digest
        self._ensure_prepared_generation_registry()
        with self._prepared_generation_registry_lock:
            if any(
                receipt_id in self._prepared_generation_registry
                for receipt_id in unique_admissions
            ):
                raise ImmutableGenerationError(
                    "prepared_import_generation_receipt_collision"
                )
            self._prepared_generation_registry.update(unique_admissions)

        for item, attribute, bound, _digest in admissions:
            setattr(item, attribute, dict(bound))

    @staticmethod
    def _normalize_prepared_item_identities(items: Sequence[PreparedImportFile]) -> None:
        for item in items:
            receipt = item.ingestion_receipt
            try:
                consumed_sha256 = str(receipt["sha256"] or "").strip().upper()
                consumed_size = int(receipt["size"])
            except (KeyError, TypeError, ValueError) as exc:
                raise ImmutableGenerationError("prepared_import_generation_receipt_invalid") from exc
            if (
                len(consumed_sha256) != 64
                or any(char not in "0123456789ABCDEF" for char in consumed_sha256)
                or int(item.size) != consumed_size
            ):
                raise ImmutableGenerationError("prepared_import_generation_identity_mismatch")
            source_sha256, source_size = ImportRepository._validated_source_receipt(
                receipt.get("source")
            )
            item.source_sha256 = source_sha256
            item.source_size = source_size
            item.sha256 = consumed_sha256

    @staticmethod
    def _validate_prepared_generation(path: Path, receipt: Dict[str, object]) -> None:
        if not isinstance(receipt, dict) or receipt.get("schema_version") not in {1, 2}:
            raise ImmutableGenerationError("prepared_import_generation_receipt_required")
        if receipt.get("publication") != "immutable_no_overwrite_generation":
            raise ImmutableGenerationError("prepared_import_generation_receipt_invalid")
        required = (
            "sha256",
            "size",
            "device",
            "inode",
            "root_device",
            "root_inode",
        )
        if any(key not in receipt for key in required):
            raise ImmutableGenerationError("prepared_import_generation_receipt_incomplete")
        ImportRepository._validated_source_receipt(receipt.get("source"))
        expected_sha256 = str(receipt["sha256"] or "").strip().upper()
        if len(expected_sha256) != 64 or any(char not in "0123456789ABCDEF" for char in expected_sha256):
            raise ImmutableGenerationError("prepared_import_generation_receipt_invalid")
        try:
            expected_size = int(receipt["size"])
            expected_device = int(receipt["device"])
            expected_inode = int(receipt["inode"])
            expected_root_device = int(receipt["root_device"])
            expected_root_inode = int(receipt["root_inode"])
        except (TypeError, ValueError) as exc:
            raise ImmutableGenerationError("prepared_import_generation_receipt_invalid") from exc
        inspect_published_generation(
            path,
            expected_sha256=expected_sha256,
            expected_size=expected_size,
            expected_device=expected_device,
            expected_inode=expected_inode,
            expected_root_device=expected_root_device,
            expected_root_inode=expected_root_inode,
        )

    def _validate_case_bound_generation(
        self,
        *,
        case_id: str,
        authority_root: Path,
        path: Path,
        receipt: Dict[str, object],
    ) -> None:
        self._validate_prepared_generation(path, receipt)
        if receipt.get("schema_version") != 2:
            raise ImmutableGenerationError("prepared_import_generation_case_binding_required")
        receipt_id = str(receipt.get("receipt_id") or "").strip()
        if not receipt_id or str(receipt.get("case_id") or "") != str(case_id):
            raise ImmutableGenerationError("prepared_import_generation_case_binding_mismatch")
        relative_path = str(receipt.get("authority_relative_path") or "")
        relative = Path(relative_path)
        if (
            not relative.parts
            or relative.is_absolute()
            or any(part in {"", ".", ".."} for part in relative.parts)
            or authority_root.joinpath(*relative.parts) != path
        ):
            raise ImmutableGenerationError("prepared_import_generation_case_binding_mismatch")
        try:
            authority_device = int(receipt["authority_root_device"])
            authority_inode = int(receipt["authority_root_inode"])
            expected_size = int(receipt["size"])
            expected_device = int(receipt["device"])
            expected_inode = int(receipt["inode"])
            expected_root_device = int(receipt["root_device"])
            expected_root_inode = int(receipt["root_inode"])
        except (KeyError, TypeError, ValueError) as exc:
            raise ImmutableGenerationError("prepared_import_generation_receipt_invalid") from exc

        self._ensure_prepared_generation_registry()
        digest = self._prepared_receipt_registry_digest(receipt)
        with self._prepared_generation_registry_lock:
            registered_digest = self._prepared_generation_registry.get(receipt_id)
        if registered_digest != digest:
            raise ImmutableGenerationError("prepared_import_generation_registry_membership_required")

        inspect_authorized_generation(
            authority_root,
            relative_path,
            expected_authority_device=authority_device,
            expected_authority_inode=authority_inode,
            expected_sha256=str(receipt.get("sha256") or ""),
            expected_size=expected_size,
            expected_device=expected_device,
            expected_inode=expected_inode,
            expected_root_device=expected_root_device,
            expected_root_inode=expected_root_inode,
        )

    def _validate_prepared_item(self, case_id: str, item: PreparedImportFile) -> None:
        self._validate_prepared_generation(item.real_path, item.ingestion_receipt)
        authority_root = self._storage.case_dir(case_id) / "raw"
        self._validate_case_bound_generation(
            case_id=case_id,
            authority_root=authority_root,
            path=item.real_path,
            receipt=item.ingestion_receipt,
        )
        try:
            item_size = _require_import_count(item.size, field="source_size")
            receipt_size = _require_import_count(
                item.ingestion_receipt.get("size"),
                field="receipt_source_size",
            )
            _require_import_count(item.rows_total, field="rows_total")
            item_source_size = _require_import_count(
                item.source_size,
                field="root_source_size",
            )
        except ValueError as exc:
            raise ImmutableGenerationError("prepared_import_generation_size_invalid") from exc
        if item_size != receipt_size:
            raise ImmutableGenerationError("prepared_import_generation_size_mismatch")
        item_sha256 = str(item.sha256 or "").strip().upper()
        receipt_sha256 = str(item.ingestion_receipt.get("sha256") or "").strip().upper()
        if item_sha256 != receipt_sha256:
            raise ImmutableGenerationError("prepared_import_generation_sha256_mismatch")
        receipt_source_sha256, receipt_source_size = self._validated_source_receipt(
            item.ingestion_receipt.get("source")
        )
        if (
            item_source_size != receipt_source_size
            or str(item.source_sha256 or "").strip().upper()
            != receipt_source_sha256
        ):
            raise ImmutableGenerationError("prepared_import_source_identity_mismatch")
        if item.duckdb_csv_path is not None and item.duckdb_csv_path != item.real_path:
            self._validate_case_bound_generation(
                case_id=case_id,
                authority_root=authority_root,
                path=item.duckdb_csv_path,
                receipt=item.duckdb_csv_receipt,
            )
            lineage = item.duckdb_csv_receipt.get("lineage")
            if (
                not isinstance(lineage, dict)
                or lineage.get("derived") is not True
                or str(lineage.get("kind") or "") != "duckdb_csv_derivative"
                or str(lineage.get("transform") or "") != "prepare_csv_clean_v1"
                or str(lineage.get("derived_from_sha256") or "").upper()
                != str(item.ingestion_receipt.get("sha256") or "").upper()
            ):
                raise ImmutableGenerationError("prepared_import_derivative_lineage_mismatch")

    @staticmethod
    def _validated_source_receipt(source: object) -> tuple[str, int]:
        required = {
            "device",
            "inode",
            "uid",
            "mode",
            "links",
            "size",
            "mtime_ns",
            "ctime_ns",
            "generation",
            "sha256",
        }
        if not isinstance(source, dict) or set(source) != required:
            raise ImmutableGenerationError("prepared_import_source_receipt_invalid")
        if any(
            isinstance(source[key], bool)
            for key in required - {"sha256"}
        ):
            raise ImmutableGenerationError("prepared_import_source_receipt_invalid")
        try:
            values = {
                key: int(source[key])
                for key in required - {"sha256"}
            }
            sha256 = str(source["sha256"] or "").strip().upper()
        except (KeyError, TypeError, ValueError) as exc:
            raise ImmutableGenerationError("prepared_import_source_receipt_invalid") from exc
        if (
            len(sha256) != 64
            or any(character not in "0123456789ABCDEF" for character in sha256)
            or values["inode"] <= 0
            or values["links"] != 1
            or values["size"] < 0
            or values["mode"] < 0
            or values["mode"] > 0o7777
            or any(
                values[key] < 0
                for key in ("device", "uid", "mtime_ns", "ctime_ns", "generation")
            )
        ):
            raise ImmutableGenerationError("prepared_import_source_receipt_invalid")
        return sha256, values["size"]

    @staticmethod
    def _fix_zip_name(name: str, flag_bits: int) -> str:
        if flag_bits & 0x800:
            return name
        if any(ch in _ZIP_GARBAGE_CHARS for ch in name):
            try:
                raw = name.encode("cp437", errors="replace")
                for enc in ("gb18030", "gbk"):
                    try:
                        fixed = raw.decode(enc, errors="strict")
                        if "�" not in fixed:
                            return fixed
                    except Exception:
                        pass
            except Exception:
                pass
        return name

    def _excel_to_csv(self, path: Path, cache_dir: Path) -> Path:
        ensure_private_generation_root(cache_dir)
        source = inspect_regular_source(path)
        work_root = cache_dir / "_work"
        ensure_private_generation_root(work_root)
        with private_work_directory(work_root) as work_dir:
            opaque_source = publish_source(
                path,
                work_dir,
                expected=source,
                suffix=path.suffix,
                lineage={
                    "kind": "excel_converter_input",
                    "derived": False,
                    "source_sha256": source.sha256,
                    "encrypted_source": False,
                },
            )
            generated_path = work_dir / "generated.csv"
            excel_to_csv(opaque_source.path, out_path=generated_path)
            generated = inspect_regular_source(generated_path)
            generation = publish_source(
                generated_path,
                cache_dir,
                expected=generated,
                suffix=".csv",
                lineage={
                    "kind": "excel_csv_derivative",
                    "derived": True,
                    "derived_from_sha256": source.sha256,
                    "encrypted_source": False,
                    "transform": "excel_to_csv_v1",
                },
            )
        return generation.path

    def _excel_to_import_csv(self, path: Path, cache_dir: Path) -> Path:
        if cache_dir.parent.name == ".import_preview_cache":
            ensure_private_generation_root(cache_dir.parent)
        ensure_private_generation_root(cache_dir)
        csv_path = self._excel_import_csv_cache_path(path, cache_dir)
        raw_csv = self._excel_to_csv(path, cache_dir / "_raw")
        return self._normalize_excel_import_csv(raw_csv=raw_csv, output=csv_path)

    @staticmethod
    def _excel_csv_cache_path(path: Path, cache_dir: Path) -> Path:
        file_tag = hashlib.md5(str(path).encode("utf-8")).hexdigest()[:10]
        return cache_dir / f"{path.stem}_{file_tag}.csv"

    @staticmethod
    def _excel_import_csv_cache_path(path: Path, cache_dir: Path) -> Path:
        file_tag = hashlib.md5(str(path).encode("utf-8")).hexdigest()[:10]
        return cache_dir / f"{path.stem}_{file_tag}.import.csv"

    def _normalize_excel_import_csv(self, *, raw_csv: Path, output: Path) -> Path:
        raw_receipt = inspect_regular_source(raw_csv)
        leading_rows: List[List[str]] = []
        with open_verified_text(raw_csv, raw_receipt, encoding="utf-8", newline="") as handle:
            reader = csv.reader(handle)
            for _ in range(_EXCEL_IMPORT_HEADER_SCAN_ROWS):
                try:
                    leading_rows.append([str(cell or "") for cell in next(reader)])
                except StopIteration:
                    break

        candidate = self._detect_excel_import_header_candidate(leading_rows)
        if candidate is None:
            return publish_source(
                raw_csv,
                output.parent,
                expected=raw_receipt,
                suffix=".csv",
                lineage={
                    "kind": "normalized_excel_csv",
                    "derived": True,
                    "derived_from_sha256": raw_receipt.sha256,
                    "transform": "excel_import_header_normalize_v1",
                    "header_row": 0,
                },
            ).path

        def produce(target_binary) -> None:
            with open_verified_text(raw_csv, raw_receipt, encoding="utf-8", newline="") as source:
                duplicate = os.fdopen(os.dup(target_binary.fileno()), "wb", closefd=True)
                target = io.TextIOWrapper(duplicate, encoding="utf-8", newline="", write_through=True)
                reader = csv.reader(source)
                writer = csv.writer(target)
                try:
                    for row_index, row in enumerate(reader):
                        if row_index < candidate.row_index:
                            continue
                        if row_index == candidate.row_index:
                            writer.writerow(candidate.headers)
                            continue
                        writer.writerow(row)
                    target.flush()
                finally:
                    target.close()

        return publish_generated(
            output.parent,
            suffix=".csv",
            producer=produce,
            lineage={
                "kind": "normalized_excel_csv",
                "derived": True,
                "derived_from_sha256": raw_receipt.sha256,
                "transform": "excel_import_header_normalize_v1",
                "header_row": candidate.row_index,
            },
        ).path

    @classmethod
    def _detect_excel_import_header_candidate(
        cls,
        rows: Sequence[Sequence[str]],
    ) -> Optional[_ExcelImportHeaderCandidate]:
        best: Optional[_ExcelImportHeaderCandidate] = None
        for row_index, row in enumerate(rows):
            if not any(str(cell or "").strip() for cell in row):
                continue
            for kind, schema in FC_SCHEMAS.items():
                score_headers = cls._canonicalize_excel_import_headers(row, kind=kind)
                matched = {header for header in score_headers if header in schema.headers}
                if not matched:
                    continue
                required = _IMPORT_REQUIRED_HEADERS_BY_KIND.get(kind, set())
                required_count = len(matched & required)
                score = len(matched) * 10 + required_count * 8 - row_index
                if len(matched) < _EXCEL_IMPORT_HEADER_MIN_MATCHED and required_count < 2:
                    continue
                if score < _EXCEL_IMPORT_HEADER_MIN_SCORE:
                    continue
                candidate = _ExcelImportHeaderCandidate(
                    row_index=row_index,
                    kind=kind,
                    headers=cls._clean_excel_import_headers(row),
                    matched_targets=len(matched),
                    required_targets=required_count,
                    score=score,
                )
                if best is None or cls._excel_import_header_candidate_sort_key(candidate) > cls._excel_import_header_candidate_sort_key(best):
                    best = candidate
        return best

    @staticmethod
    def _excel_import_header_candidate_sort_key(candidate: _ExcelImportHeaderCandidate) -> tuple[int, int, int, int]:
        return (
            candidate.score,
            candidate.required_targets,
            candidate.matched_targets,
            -candidate.row_index,
        )

    @classmethod
    def _canonicalize_excel_import_headers(cls, row: Sequence[str], *, kind: str) -> List[str]:
        lookup = cls._canonical_header_lookup(kind)
        headers = []
        for value in row:
            label = cls._clean_excel_import_header_label(value)
            normalized = normalize_header_for_match(label)
            headers.append(lookup.get(normalized, label))
        return cls._dedupe_csv_headers(headers)

    @classmethod
    def _clean_excel_import_headers(cls, row: Sequence[str]) -> List[str]:
        headers = [cls._clean_excel_import_header_label(value) for value in row]
        return cls._dedupe_csv_headers(headers)

    @staticmethod
    def _clean_excel_import_header_label(value: object) -> str:
        text = str(value or "").replace("\ufeff", "").strip()
        while text[:1] in {"*", "＊"}:
            text = text[1:].strip()
        return text

    @staticmethod
    def _dedupe_csv_headers(headers: Sequence[str]) -> List[str]:
        seen: set[str] = set()
        out: List[str] = []
        for index, header in enumerate(headers):
            base = str(header or "").strip() or f"Unnamed: {index}"
            candidate = base
            suffix = 1
            while candidate in seen:
                candidate = f"{base}.{suffix}"
                suffix += 1
            seen.add(candidate)
            out.append(candidate)
        return out

    @staticmethod
    def _canonical_header_lookup(kind: str) -> Dict[str, str]:
        schema = FC_SCHEMAS.get(kind)
        if schema is None:
            return {}
        alias_map = HEADER_ALIASES_BY_TABLE.get(kind, {})
        lookup: Dict[str, str] = {}
        for header in schema.headers:
            for candidate in (header, *alias_map.get(header, [])):
                normalized = normalize_header_for_match(candidate)
                if normalized and normalized not in lookup:
                    lookup[normalized] = header
        return lookup

    def _try_promote_excel_csv_cache(
        self,
        *,
        source_path: Path,
        prepared_path: Path,
        target_cache_dir: Path,
        source_key: str = "",
        expected_excel_sha256: Optional[str] = None,
    ) -> Optional[Path]:
        if (
            source_path.suffix.lower() not in {".xlsx", ".xls"}
            and source_path.suffix.lower() not in _SUPPORTED_ARCHIVE_EXTS
        ):
            return None
        if prepared_path.suffix.lower() not in {".xlsx", ".xls"}:
            return None
        target_csv = self._excel_import_csv_cache_path(prepared_path, target_cache_dir)
        entry = self._load_excel_csv_preview_manifest_entry(source_path, source_key=source_key)
        if not entry:
            return None
        entry_sha256 = str(entry.get("excel_sha256") or "").strip().upper()
        expected_sha256 = str(expected_excel_sha256 or "").strip().upper()
        if entry_sha256:
            if not expected_sha256:
                expected_sha256 = inspect_regular_source(prepared_path).sha256
            if expected_sha256 != entry_sha256:
                return None
        csv_path = Path(str(entry.get("csv_path") or ""))
        promoted = self._copy_promoted_excel_csv_cache(
            preview_csv=csv_path,
            prepared_path=prepared_path,
            target_csv=target_csv,
        )
        return promoted

    @staticmethod
    def _copy_promoted_excel_csv_cache(*, preview_csv: Path, prepared_path: Path, target_csv: Path) -> Optional[Path]:
        try:
            preview_receipt = inspect_regular_source(preview_csv)
            prepared_receipt = inspect_regular_source(prepared_path)
        except ImmutableGenerationError:
            return None
        return publish_source(
            preview_csv,
            target_csv.parent,
            expected=preview_receipt,
            suffix=".csv",
            lineage={
                "kind": "promoted_excel_csv_cache",
                "derived": True,
                "derived_from_sha256": prepared_receipt.sha256,
                "preview_csv_sha256": preview_receipt.sha256,
                "transform": "excel_preview_promotion_v1",
                "encrypted_source": False,
            },
        ).path

    def _read_tabular_preview(
        self,
        path: Path,
        *,
        limit: int,
        preview_source_path: Optional[Path] = None,
        preview_source_key: str = "",
        preview_excel_sha256: Optional[str] = None,
        preview_label: str = "",
        profiler: Optional[ImportPreviewProfiler] = None,
    ) -> tuple[int, int, List[str], List[List[str]], Optional[CsvColumnProfiles]]:
        if profiler:
            profiler.count("tabular_previews")
        if path.suffix.lower() == ".csv":
            csv_path = path
            csv_encoding = None
        else:
            if profiler:
                with profiler.time_phase("excel_to_csv_s"):
                    csv_path = self._excel_to_import_csv(path, self._preview_cache_dir(path) / "_excelcsv")
            else:
                csv_path = self._excel_to_import_csv(path, self._preview_cache_dir(path) / "_excelcsv")
            csv_encoding = "utf-8"
        prepared_profile = self._prepare_csv_with_profile_for_preview(
            csv_path,
            limit=limit,
            encoding=csv_encoding,
            profiler=profiler,
            phase_name="prepare_csv_with_profile_s",
        )
        result = self._csv_tabular_preview_result_from_prepared(
            _CsvTabularPreviewRequest(
                path=csv_path,
                label=preview_label or path.name,
                limit=limit,
                encoding=csv_encoding,
            ),
            prepared_profile.value,
            profiler=profiler,
            elapsed_s=prepared_profile.elapsed_s,
        )
        if path.suffix.lower() in {".xlsx", ".xls"} and preview_source_path is not None:
            self._record_excel_csv_preview_manifest(
                source_path=preview_source_path or path,
                source_key=preview_source_key,
                excel_path=path,
                csv_path=csv_path,
                rows_total=result.rows_total,
                excel_sha256=preview_excel_sha256,
            )
        return result.rows_total, result.columns_total, result.headers, result.sample_rows, result.profiles

    def _csv_tabular_preview_result_from_prepared(
        self,
        request: _CsvTabularPreviewRequest,
        prepared_with_profile: PreparedCsvWithProfile,
        *,
        profiler: Optional[ImportPreviewProfiler],
        elapsed_s: float,
    ) -> _CsvTabularPreviewResult:
        prepared_csv = prepared_with_profile.prepared
        profiles = prepared_with_profile.profiles
        rows_total = prepared_csv.rows_total
        preview_headers, sample_rows = self._trim_trailing_blank_preview_columns(
            prepared_csv.header_preview,
            prepared_csv.sample_rows,
        )
        columns_total = len(preview_headers)
        if profiler:
            profiler.record_tabular_scan(
                label=request.label or request.path.name,
                elapsed_s=elapsed_s,
                rows_total=rows_total,
                columns_total=columns_total,
                profile_columns_total=profiles.columns_total if profiles else 0,
                profile_ready=profiles is not None,
            )
        return _CsvTabularPreviewResult(
            rows_total=rows_total,
            columns_total=columns_total,
            headers=preview_headers,
            sample_rows=sample_rows,
            profiles=profiles,
        )

    def _read_csv_tabular_previews_batch(
        self,
        requests: Sequence[_CsvTabularPreviewRequest],
        *,
        profiler: Optional[ImportPreviewProfiler] = None,
    ) -> List[_CsvTabularPreviewResult]:
        if not requests:
            return []
        if profiler:
            profiler.count("tabular_previews", len(requests))

        if profiler:
            profiler.count("profile_scans", len(requests))
            scan_started = time.perf_counter()
        try:
            prepared_items = prepare_csv_with_profile_batch(
                [
                    PrepareCsvWithProfileBatchItem(
                        path=request.path,
                        limit=request.limit,
                        encoding=request.encoding,
                    )
                    for request in requests
                ]
            )
        except ImportAcceleratorUnavailableError:
            prepared_items = [
                self._prepare_csv_with_profile_python(
                    request.path,
                    limit=request.limit,
                    encoding=request.encoding,
                )
                for request in requests
            ]
        if profiler:
            profiler.add_phase("prepare_csv_with_profile_batch_s", time.perf_counter() - scan_started)
        if len(prepared_items) != len(requests):
            raise ImportAcceleratorUnavailableError("Rust import accelerator returned mismatched batch profile previews")

        results = [
            self._csv_tabular_preview_result_from_prepared(
                request,
                prepared_with_profile,
                profiler=profiler,
                elapsed_s=(
                    0.0
                    if prepared_with_profile.elapsed_s is None
                    else float(prepared_with_profile.elapsed_s)
                ),
            )
            for request, prepared_with_profile in zip(requests, prepared_items)
        ]
        return results

    @staticmethod
    def _trim_trailing_blank_preview_columns(
        headers: Sequence[str],
        sample_rows: Sequence[Sequence[str]],
    ) -> tuple[List[str], List[List[str]]]:
        trimmed_headers = [str(header or "") for header in headers]
        trimmed_rows = [[str(cell or "") for cell in row] for row in sample_rows]

        def is_generated_blank_header(header: str) -> bool:
            text = str(header or "").strip()
            if not text.startswith("Unnamed:"):
                return False
            suffix = text.split(":", 1)[1].strip()
            return suffix.isdigit()

        end = len(trimmed_headers)
        while end > 0 and is_generated_blank_header(trimmed_headers[end - 1]):
            has_sample_value = any(
                end - 1 < len(row) and str(row[end - 1] or "").strip()
                for row in trimmed_rows
            )
            if has_sample_value:
                break
            end -= 1
        if end == len(trimmed_headers):
            return trimmed_headers, trimmed_rows
        return trimmed_headers[:end], [row[:end] for row in trimmed_rows]

    def _record_excel_csv_preview_manifest(
        self,
        *,
        source_path: Path,
        source_key: str,
        excel_path: Path,
        csv_path: Path,
        rows_total: int,
        excel_sha256: Optional[str] = None,
    ) -> None:
        try:
            source_receipt = inspect_regular_source(source_path)
            excel_receipt = inspect_regular_source(excel_path)
            csv_receipt = inspect_regular_source(csv_path)
            manifest_path = self._excel_csv_preview_manifest_path(
                source_path,
                source_key=source_key,
                source_receipt=source_receipt,
            )
            ensure_private_generation_root(self._preview_cache_dir(source_path))
            encrypted_source = self._is_file_password_protected(source_path)
            if inspect_regular_source(source_path) != source_receipt:
                raise ImmutableGenerationError("excel_preview_source_identity_mismatch")
            manifest = {
                "schema_version": 2,
                "source": {
                    "sha256": source_receipt.sha256,
                    "size": source_receipt.size,
                },
                "source_key": self._excel_csv_preview_manifest_key(source_path, source_key),
                "artifact": {
                    "excel_path": str(excel_path),
                    "excel_sha256": str(excel_sha256 or excel_receipt.sha256).upper(),
                    "excel_size": excel_receipt.size,
                    "csv_path": str(csv_path),
                    "csv_sha256": csv_receipt.sha256,
                    "csv_size": csv_receipt.size,
                    "rows_total": _require_import_count(
                        rows_total,
                        field="preview_manifest_rows_total",
                    ),
                },
                "lineage": {
                    "kind": "excel_preview_manifest",
                    "derived": True,
                    "derived_from_sha256": source_receipt.sha256,
                    "encrypted_source": encrypted_source,
                    "transform": "excel_import_header_normalize_v1",
                },
            }
            generation = publish_json(
                manifest_path.parent,
                manifest,
                target_name=manifest_path.name,
                lineage=dict(manifest["lineage"]),
            )
            generation_receipt = generation.receipt()
            generation_receipt["source"] = source_receipt.as_dict()
            generation_receipt["lineage"] = {
                **dict(generation_receipt.get("lineage") or {}),
                "immediate_parent_sha256": source_receipt.sha256,
                "root_source_sha256": source_receipt.sha256,
            }
            self._remember_preview_manifest_generation(
                manifest_path,
                generation_receipt,
            )
        except Exception:
            return

    def _load_excel_csv_preview_manifest_entry(self, source_path: Path, *, source_key: str) -> Optional[Dict[str, object]]:
        try:
            source_receipt = inspect_regular_source(source_path)
            manifest_path = self._excel_csv_preview_manifest_path(
                source_path,
                source_key=source_key,
                source_receipt=source_receipt,
            )
            generation_receipt = self._get_preview_manifest_generation(manifest_path)
            if not generation_receipt:
                return None
            self._validate_prepared_generation(manifest_path, generation_receipt)
            payload = read_published_generation_bytes(
                manifest_path,
                expected_sha256=str(generation_receipt.get("sha256") or ""),
                expected_size=int(generation_receipt.get("size") or -1),
                expected_device=int(generation_receipt.get("device") or -1),
                expected_inode=int(generation_receipt.get("inode") or -1),
                expected_root_device=int(generation_receipt.get("root_device") or -1),
                expected_root_inode=int(generation_receipt.get("root_inode") or -1),
                max_bytes=1024 * 1024,
            )

            def reject_duplicate_keys(pairs):
                value: Dict[str, object] = {}
                for key, item in pairs:
                    if key in value:
                        raise ValueError("duplicate preview manifest key")
                    value[key] = item
                return value

            manifest = json.loads(payload.decode("utf-8"), object_pairs_hook=reject_duplicate_keys)
            if (
                not isinstance(manifest, dict)
                or set(manifest) != {"schema_version", "source", "source_key", "artifact", "lineage"}
                or manifest.get("schema_version") != 2
            ):
                return None
            source = manifest.get("source")
            if not isinstance(source, dict) or set(source) != {"sha256", "size"}:
                return None
            if str(source.get("sha256") or "").upper() != source_receipt.sha256:
                return None
            if int(source.get("size") or -1) != source_receipt.size:
                return None
            if manifest.get("source_key") != self._excel_csv_preview_manifest_key(source_path, source_key):
                return None
            entry = manifest.get("artifact")
            if not isinstance(entry, dict) or set(entry) != {
                "excel_path",
                "excel_sha256",
                "excel_size",
                "csv_path",
                "csv_sha256",
                "csv_size",
                "rows_total",
            }:
                return None
            lineage = manifest.get("lineage")
            if (
                not isinstance(lineage, dict)
                or set(lineage)
                != {"kind", "derived", "derived_from_sha256", "encrypted_source", "transform"}
                or lineage.get("kind") != "excel_preview_manifest"
                or lineage.get("derived") is not True
                or str(lineage.get("derived_from_sha256") or "").upper() != source_receipt.sha256
                or lineage.get("transform") != "excel_import_header_normalize_v1"
            ):
                return None
            csv_path = Path(str(entry.get("csv_path") or ""))
            expected_csv_sha256 = str(entry.get("csv_sha256") or "").strip().upper()
            preview_root = self._preview_cache_dir(source_path).absolute()
            try:
                csv_path.absolute().relative_to(preview_root)
            except ValueError:
                return None
            if csv_path.name != f"{expected_csv_sha256.lower()}.csv":
                return None
            csv_generation = inspect_published_generation(
                csv_path,
                expected_sha256=expected_csv_sha256,
                expected_size=int(entry.get("csv_size") or -1),
            )
            csv_receipt = inspect_regular_source(csv_generation.path, expected_sha256=expected_csv_sha256)
            if int(entry.get("csv_size") or -1) != csv_receipt.size:
                return None
            return entry
        except Exception:
            return None

    def _remember_preview_manifest_generation(self, path: Path, receipt: Dict[str, object]) -> None:
        lock = getattr(self, "_preview_manifest_generation_registry_lock", None)
        if lock is None:
            lock = RLock()
            self._preview_manifest_generation_registry_lock = lock
        with lock:
            registry = getattr(self, "_preview_manifest_generation_registry", None)
            if not isinstance(registry, dict):
                registry = {}
                self._preview_manifest_generation_registry = registry
            registry[str(path)] = dict(receipt)
            if len(registry) > 2048:
                self._preview_manifest_generation_registry = dict(list(registry.items())[-1024:])

    def _get_preview_manifest_generation(self, path: Path) -> Dict[str, object]:
        lock = getattr(self, "_preview_manifest_generation_registry_lock", None)
        if lock is None:
            return {}
        with lock:
            registry = getattr(self, "_preview_manifest_generation_registry", None)
            if not isinstance(registry, dict):
                return {}
            receipt = registry.get(str(path))
            return dict(receipt) if isinstance(receipt, dict) else {}

    def _excel_csv_preview_manifest_path(
        self,
        source_path: Path,
        *,
        source_key: str,
        source_receipt: Optional[RegularSourceReceipt] = None,
    ) -> Path:
        receipt = source_receipt or inspect_regular_source(source_path)
        key = self._excel_csv_preview_manifest_key(source_path, source_key)
        key_hash = hashlib.sha256(key.encode("utf-8")).hexdigest()[:24]
        return (
            self._preview_cache_dir(source_path)
            / "_manifests_v2"
            / f"{receipt.sha256.lower()}-{key_hash}-excel-preview-v2.json"
        )

    @staticmethod
    def _excel_csv_preview_manifest_key(source_path: Path, source_key: str) -> str:
        normalized_key = str(source_key or "").strip()
        if normalized_key:
            return normalized_key
        return str(source_path)

    def _split_excel_account_sections(self, path: Path, cache_dir: Path) -> List[tuple[Path, str]]:
        ensure_private_generation_root(cache_dir)
        source = inspect_regular_source(path)
        work_root = cache_dir / "_work"
        ensure_private_generation_root(work_root)
        with private_work_directory(work_root) as work_dir:
            opaque_source = publish_source(
                path,
                work_dir,
                expected=source,
                suffix=path.suffix,
                lineage={
                    "kind": "excel_splitter_input",
                    "derived": False,
                    "source_sha256": source.sha256,
                    "encrypted_source": False,
                },
            )
            generated = split_excel_account_sections(opaque_source.path, output_dir=work_dir)
            promoted: List[tuple[Path, str]] = []
            for generated_path, kind in generated:
                generated_receipt = inspect_regular_source(generated_path)
                generation = publish_source(
                    generated_path,
                    cache_dir,
                    expected=generated_receipt,
                    suffix=".csv",
                    lineage={
                        "kind": "excel_account_section_derivative",
                        "derived": True,
                        "derived_from_sha256": source.sha256,
                        "transform": "split_excel_account_sections_v1",
                        "section_kind": str(kind),
                        "encrypted_source": False,
                    },
                )
                promoted.append((generation.path, kind))
        return promoted
