from __future__ import annotations

import hashlib
import json
import re
from collections.abc import Callable, Iterator, Mapping
from typing import Any, Protocol

from app.core.failure_boundary import project_cleaning_error, project_import_error
from app.domain.analysis_workbench_boundary import opaque_case_bound_ref
from app.domain.ordinary_diagnostic_projection import (
    project_cleaning_history_item,
    project_cleaning_summary_diagnostic,
    project_diagnostic_timestamp,
    project_operational_event,
    project_query_log_diagnostic,
    project_query_tool_name,
)
from app.repositories.analysis_revision import project_analysis_revision_reason


_MIGRATION_ID = "ordinary_diagnostic_projection_v2"
_JOURNAL_TABLE = "analytix_diagnostic_migration_journal_v2"
_JOURNAL_COLUMNS = (
    "migration_id",
    "schema_version",
    "phase",
    "plan_hash",
    "target_count",
    "migrated_row_count",
    "case_binding_hash",
    "schema_manifest_hash",
    "protected_schema_hash",
    "projection_hash",
)
_TARGET_TABLES = (
    "import_file_log",
    "import_file_log_recycle",
    "cleaning_log",
    "cleaning_log_recycle",
    "analysis_query_log",
    "analysis_audit_event",
    "analysis_run_log_event",
    "analysis_temp_scope",
    "analysis_revision_state",
    "document_assets",
    "document_assets_recycle",
    "privacy_runtime_state",
)
_UNKNOWN_LEGACY_DIAGNOSTIC_TABLES = (
    "cleaning_log_detail",
    "cleaning_log_detail_recycle",
)
_LEGACY_IMPORT_LOG_COLUMNS = frozenset(
    {
        "file_id",
        "case_id",
        "kind",
        "filename",
        "display_path",
        "stored_path",
        "file_type",
        "size",
        "md5",
        "sha256",
        "rows_total",
        "rows_imported",
        "rows_imported_raw",
        "rows_imported_norm",
        "rows_dedup",
        "rows_error",
        "rows_skipped_non_data",
        "status",
        "error",
        "cleaned_status",
        "cleaned_started_at",
        "cleaned_finished_at",
        "cleaned_error",
        "cleaned_rows_affected",
        "created_at",
        "finished_at",
        "recycled_at",
    }
)
_IMPORT_LOG_COLUMNS = _LEGACY_IMPORT_LOG_COLUMNS | {
    "import_counts_version",
    "cleaning_counts_version",
}
_CLEANING_LOG_COLUMNS = frozenset(
    {
        "id",
        "case_id",
        "file_id",
        "import_at",
        "cleaned_at",
        "duration_ms",
        "scope_rows",
        "summary",
        "run_ref",
        "recycled_at",
    }
)
_QUERY_LOG_COLUMNS = frozenset(
    {
        "query_id",
        "case_id",
        "tool_name",
        "params_json",
        "summary_json",
        "row_count",
        "duration_ms",
        "created_at",
    }
)
_AUDIT_EVENT_COLUMNS = frozenset(
    {
        "event_id",
        "case_id",
        "event_type",
        "task_id",
        "run_id",
        "turn_id",
        "artifact_id",
        "approval_id",
        "trace_id",
        "span_id",
        "actor_id",
        "actor_role",
        "tenant_id",
        "request_id",
        "session_id",
        "payload_json",
        "created_at",
    }
)
_RUN_LOG_COLUMNS = frozenset(
    {
        "event_id",
        "case_id",
        "task_id",
        "run_id",
        "turn_id",
        "sequence_no",
        "event_stage",
        "event_type",
        "source",
        "payload_json",
        "created_at",
    }
)
_TEMP_SCOPE_COLUMNS = frozenset(
    {
        "scope_id",
        "case_id",
        "scope_type",
        "source_kind",
        "scope_signature",
        "status",
        "source_revision",
        "source_file_ids_json",
        "document_ids_json",
        "stats_json",
        "audit_json",
        "expires_at",
        "retention_until",
        "last_accessed_at",
        "created_at",
        "updated_at",
    }
)
_REVISION_COLUMNS = frozenset({"revision_key", "revision", "updated_at", "reason"})
_DOCUMENT_ASSET_COLUMNS = frozenset(
    {
        "document_id",
        "case_id",
        "file_id",
        "kind",
        "filename",
        "display_path",
        "stored_path",
        "file_type",
        "size",
        "md5",
        "sha256",
        "duplicate_of_document_id",
        "duplicate_reason",
        "selected_for_llm",
        "llm_ready",
        "parser",
        "extraction_status",
        "ocr_status",
        "vector_status",
        "vector_model",
        "content_path",
        "content_excerpt",
        "content_chars",
        "page_count",
        "chunk_count",
        "token_estimate",
        "last_error",
        "created_at",
        "updated_at",
        "indexed_at",
        "recycled_at",
    }
)
_PRIVACY_STATE_COLUMNS = frozenset(
    {"case_id", "version", "status", "enabled", "progress", "summary", "error", "salt", "updated_at"}
)
_ALLOWED_TARGET_COLUMNS = {
    "import_file_log": _IMPORT_LOG_COLUMNS,
    "import_file_log_recycle": _IMPORT_LOG_COLUMNS,
    "cleaning_log": _CLEANING_LOG_COLUMNS,
    "cleaning_log_recycle": _CLEANING_LOG_COLUMNS,
    "analysis_query_log": _QUERY_LOG_COLUMNS,
    "analysis_audit_event": _AUDIT_EVENT_COLUMNS,
    "analysis_run_log_event": _RUN_LOG_COLUMNS,
    "analysis_temp_scope": _TEMP_SCOPE_COLUMNS,
    "analysis_revision_state": _REVISION_COLUMNS,
    "document_assets": _DOCUMENT_ASSET_COLUMNS,
    "document_assets_recycle": _DOCUMENT_ASSET_COLUMNS,
    "privacy_runtime_state": _PRIVACY_STATE_COLUMNS,
}
_SUPPORTED_TARGET_COLUMN_SETS = {
    "import_file_log": (
        _IMPORT_LOG_COLUMNS - {"recycled_at"},
        _LEGACY_IMPORT_LOG_COLUMNS - {"recycled_at"},
        frozenset(
            {
                "file_id",
                "case_id",
                "filename",
                "display_path",
                "stored_path",
                "status",
                "error",
                "cleaned_status",
                "cleaned_error",
            }
        ),
    ),
    "import_file_log_recycle": (
        _IMPORT_LOG_COLUMNS,
        _LEGACY_IMPORT_LOG_COLUMNS,
        frozenset(
            {
                "file_id",
                "case_id",
                "filename",
                "display_path",
                "stored_path",
                "status",
                "error",
                "cleaned_status",
                "cleaned_error",
                "recycled_at",
            }
        ),
    ),
    "cleaning_log": (_CLEANING_LOG_COLUMNS - {"recycled_at"},),
    "cleaning_log_recycle": (_CLEANING_LOG_COLUMNS,),
    "analysis_query_log": (_QUERY_LOG_COLUMNS,),
    "analysis_audit_event": (_AUDIT_EVENT_COLUMNS,),
    "analysis_run_log_event": (_RUN_LOG_COLUMNS,),
    "analysis_temp_scope": (
        _TEMP_SCOPE_COLUMNS,
        _TEMP_SCOPE_COLUMNS - {"source_revision"},
    ),
    "analysis_revision_state": (_REVISION_COLUMNS,),
    "document_assets": (
        _DOCUMENT_ASSET_COLUMNS - {"recycled_at"},
        frozenset(
            {
                "document_id",
                "case_id",
                "filename",
                "stored_path",
                "content_path",
                "sha256",
                "duplicate_reason",
                "last_error",
            }
        ),
    ),
    "document_assets_recycle": (
        _DOCUMENT_ASSET_COLUMNS,
        frozenset(
            {
                "document_id",
                "case_id",
                "filename",
                "stored_path",
                "content_path",
                "sha256",
                "duplicate_reason",
                "last_error",
                "recycled_at",
            }
        ),
    ),
    "privacy_runtime_state": (
        _PRIVACY_STATE_COLUMNS,
        frozenset({"case_id", "status", "summary", "error", "salt"}),
    ),
}
_TARGET_PRIMARY_KEYS = {
    "import_file_log": "file_id",
    "import_file_log_recycle": "file_id",
    "cleaning_log": "id",
    "cleaning_log_recycle": "id",
    "analysis_query_log": "query_id",
    "analysis_audit_event": "event_id",
    "analysis_run_log_event": "event_id",
    "analysis_temp_scope": "scope_id",
    "analysis_revision_state": "revision_key",
    "document_assets": "document_id",
    "document_assets_recycle": "document_id",
    "privacy_runtime_state": "case_id",
}
_RECYCLE_TABLES_WITHOUT_PRIMARY_KEYS = frozenset(
    {"import_file_log_recycle", "cleaning_log_recycle", "document_assets_recycle"}
)
_BIGINT_TARGET_COLUMNS = frozenset(
    {
        "size",
        "rows_total",
        "rows_imported",
        "rows_imported_raw",
        "rows_imported_norm",
        "rows_dedup",
        "rows_error",
        "rows_skipped_non_data",
        "import_counts_version",
        "cleaned_rows_affected",
        "cleaning_counts_version",
        "id",
        "duration_ms",
        "scope_rows",
        "row_count",
        "sequence_no",
        "source_revision",
        "revision",
        "content_chars",
        "page_count",
        "chunk_count",
        "token_estimate",
    }
)
_BOOLEAN_TARGET_COLUMNS = frozenset({"selected_for_llm", "llm_ready", "enabled"})
_PLAN_HASH = hashlib.sha256(
    (
        "analytix-diagnostic-duckdb-v2-table-spec-v1\x00"
        "projection-contract:ordinary-diagnostic-v2\x00"
        "current-temp-scope-validator:v2\x00"
        "id-contract:case-bound-sha256-v1\x00"
        + "\x00".join(
            (
                f"{table}:pk={_TARGET_PRIMARY_KEYS[table]}:"
                f"variants={'|'.join(','.join(sorted(variant)) for variant in _SUPPORTED_TARGET_COLUMN_SETS[table])}"
            )
            for table in _TARGET_TABLES
        )
        + "\x00bigint:"
        + ",".join(sorted(_BIGINT_TARGET_COLUMNS))
        + "\x00boolean:"
        + ",".join(sorted(_BOOLEAN_TARGET_COLUMNS))
        + "\x00recycle-without-pk:"
        + ",".join(sorted(_RECYCLE_TABLES_WITHOUT_PRIMARY_KEYS))
    ).encode("ascii")
).hexdigest()
_MAX_JSON_BYTES = 4 * 1024 * 1024
_MAX_TARGET_ROWS = 1_000_000
_HASH_PAGE_ROWS = 1_000
_FILE_ID_RE = re.compile(r"^(?:[a-f0-9]{20}|temp_fc_[a-f0-9]{16}|temp_fc_[a-f0-9]{64})$")
_DOCUMENT_ID_RE = re.compile(
    r"^(?:[a-f0-9]{20}|__case_project_doc_(?:brief|direction)__$)"
)
_TEMP_SCOPE_ID_RE = re.compile(r"^temp_scope_v1_[a-f0-9]{64}$")
_TEMP_SCOPE_SIGNATURE_RE = re.compile(r"^temp_scope_v2_[a-f0-9]{16}$")
_TEMP_SOURCE_REF_RE = re.compile(r"^tempsrc_v1_[a-f0-9]{64}$")
_TEMP_DOCUMENT_REF_RE = re.compile(r"^tempdoc_v1_[a-f0-9]{64}$")
_TEMP_SCOPE_SOURCE_KINDS = frozenset(
    {"uploaded_file", "document_selection", "mixed_upload"}
)
_TEMP_SCOPE_FAILURE_CODES = frozenset(
    {
        "source_not_registered",
        "source_path_unavailable",
        "excel_prepare_failed",
        "csv_header_read_failed",
        "source_kind_unsupported",
        "no_transaction_sheet",
        "temp_import_failed",
    }
)
_TEMP_SCOPE_WARNING_CODES = frozenset(
    {
        "TEMP_SCOPE_TTL_EXPIRED",
        "TEMP_SCOPE_QUOTA_TRIMMED",
        "TEMP_SCOPE_AUDIT_PURGED",
    }
)
_JSON_DIAGNOSTIC_COLUMNS = frozenset(
    {
        "summary",
        "params_json",
        "summary_json",
        "payload_json",
    }
)
_TIMESTAMP_DIAGNOSTIC_COLUMNS = frozenset(
    {
        "import_at",
        "cleaned_at",
        "created_at",
        "updated_at",
    }
)
_IMPORT_STATUSES = frozenset(
    {
        "pending",
        "queued",
        "processing",
        "running",
        "succeeded",
        "completed",
        "failed",
        "canceled",
        "cancelled",
        "已登记",
        "导入中",
        "已完成",
        "失败",
        "已取消",
    }
)
_CLEANING_STATUSES = frozenset(
    {
        "pending",
        "queued",
        "running",
        "done",
        "succeeded",
        "completed",
        "failed",
        "canceled",
        "cancelled",
        "已完成",
        "失败",
        "已取消",
    }
)


class DiagnosticDuckDBMigrationError(RuntimeError):
    """Fixed-code failure for the ordinary-diagnostic migration boundary."""

    def __init__(self, code: str = "diagnostic_duckdb_migration_unresolved") -> None:
        self.code = str(code or "diagnostic_duckdb_migration_unresolved")
        super().__init__(self.code)


class DiagnosticDuckDBEngine(Protocol):
    def execute(self, sql: str, params: tuple[Any, ...] | None = None) -> None: ...

    def query(self, sql: str, params: tuple[Any, ...] | None = None) -> list[tuple[Any, ...]]: ...

    def replace_with_compacted_generation(self) -> None: ...


FaultHook = Callable[[str], None]


def migrate_legacy_duckdb_diagnostics(
    engine: DiagnosticDuckDBEngine,
    *,
    case_id: str,
    fault_hook: FaultHook | None = None,
) -> dict[str, int | str]:
    """Physically replace legacy ordinary diagnostics with closed projections.

    Only the versioned allowlist of operational tables is touched. Accepted case
    evidence, transaction rows, raw-source tables, and evidence registries are
    deliberately outside this migration. The durable journal contains only
    fixed schema values and aggregate counts; it never stores source text,
    paths, SQL, identifiers from a row, PII, or reasoning.
    """

    expected_case_id = _required_key(case_id)
    readiness_error: DiagnosticDuckDBMigrationError | None = None
    try:
        assert_duckdb_diagnostic_migration_settled(engine, case_id=expected_case_id)
    except DiagnosticDuckDBMigrationError as exc:
        readiness_error = exc
    else:
        return {
            "migration_id": _MIGRATION_ID,
            "phase": "settled",
            "target_count": len(_TARGET_TABLES),
            "migrated_row_count": 0,
        }
    if _journal_phase(engine) == "settled":
        _validate_or_create_journal_row(
            engine,
            case_binding_hash=_case_binding_hash(expected_case_id),
        )
        assert readiness_error is not None
        raise readiness_error
    schema_manifest_hash = _target_schema_manifest_hash(engine)
    protected_schema_hash = _protected_schema_hash(engine)
    source_projection_hash = _target_projection_hash(engine)
    case_binding_hash = _case_binding_hash(expected_case_id)
    _ensure_journal(engine)
    _validate_or_create_journal_row(
        engine,
        case_binding_hash=case_binding_hash,
        initial_schema_manifest_hash=schema_manifest_hash,
        initial_protected_schema_hash=protected_schema_hash,
        initial_projection_hash=source_projection_hash,
    )
    journal = _journal_record(engine)
    phase = journal["phase"]
    if phase == "planned":
        if (
            journal["migrated_row_count"] != 0
            or journal["schema_manifest_hash"] != schema_manifest_hash
            or journal["protected_schema_hash"] != protected_schema_hash
            or journal["projection_hash"] != source_projection_hash
        ):
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_recovery_mismatch")
        _fault(fault_hook, "planned_committed")

        migrated_row_count = 0
        engine.execute("BEGIN TRANSACTION")
        try:
            migrated_row_count = _project_target_tables(
                engine,
                expected_case_id=expected_case_id,
                write=True,
            )
            # The write pass is not sufficient settlement evidence. Re-run the
            # closed projection over the same transaction so a skipped page,
            # stale cursor, or partial writer can never advance the journal.
            _project_target_tables(
                engine,
                expected_case_id=expected_case_id,
                write=False,
            )
            applied_schema_manifest_hash = _target_schema_manifest_hash(engine)
            applied_protected_schema_hash = _protected_schema_hash(engine)
            if applied_protected_schema_hash != protected_schema_hash:
                raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_protected_schema_changed")
            projection_hash = _target_projection_hash(engine)
            engine.execute(
                f"UPDATE {_JOURNAL_TABLE} "
                "SET phase='applied', migrated_row_count=?, schema_manifest_hash=?, "
                "protected_schema_hash=?, projection_hash=? WHERE migration_id=?",
                (
                    migrated_row_count,
                    applied_schema_manifest_hash,
                    applied_protected_schema_hash,
                    projection_hash,
                    _MIGRATION_ID,
                ),
            )
            _fault(fault_hook, "before_commit")
            engine.execute("COMMIT")
        except Exception as exc:
            try:
                engine.execute("ROLLBACK")
            except Exception:
                pass
            if isinstance(exc, DiagnosticDuckDBMigrationError):
                raise
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_migration_failed") from None
    elif phase == "applied":
        migrated_row_count = int(journal["migrated_row_count"])
        applied_schema_manifest_hash = str(journal["schema_manifest_hash"])
        applied_protected_schema_hash = str(journal["protected_schema_hash"])
        projection_hash = str(journal["projection_hash"])
        if (
            schema_manifest_hash != applied_schema_manifest_hash
            or protected_schema_hash != applied_protected_schema_hash
            or source_projection_hash != projection_hash
        ):
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_recovery_mismatch")
    else:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_journal_invalid")

    _project_target_tables(
        engine,
        expected_case_id=expected_case_id,
        write=False,
    )
    _fault(fault_hook, "applied_committed")
    try:
        engine.execute("CHECKPOINT")
        _fault(fault_hook, "checkpoint_completed")
        if (
            _target_schema_manifest_hash(engine) != applied_schema_manifest_hash
            or _protected_schema_hash(engine) != applied_protected_schema_hash
            or _target_projection_hash(engine) != projection_hash
        ):
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_settlement_mismatch")
        _project_target_tables(
            engine,
            expected_case_id=expected_case_id,
            write=False,
        )
        engine.replace_with_compacted_generation()
        _fault(fault_hook, "generation_replaced")
        if (
            _target_schema_manifest_hash(engine) != applied_schema_manifest_hash
            or _protected_schema_hash(engine) != applied_protected_schema_hash
            or _target_projection_hash(engine) != projection_hash
        ):
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_settlement_mismatch")
        _project_target_tables(
            engine,
            expected_case_id=expected_case_id,
            write=False,
        )
        engine.execute("BEGIN TRANSACTION")
        try:
            engine.execute(
                f"UPDATE {_JOURNAL_TABLE} SET phase='settled' WHERE migration_id=?",
                (_MIGRATION_ID,),
            )
            _fault(fault_hook, "before_settled_commit")
            engine.execute("COMMIT")
        except Exception:
            try:
                engine.execute("ROLLBACK")
            except Exception:
                pass
            raise
    except Exception as exc:
        if isinstance(exc, DiagnosticDuckDBMigrationError):
            raise
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_settlement_failed") from None
    _fault(fault_hook, "settled")
    return {
        "migration_id": _MIGRATION_ID,
        "phase": "settled",
        "target_count": len(_TARGET_TABLES),
        "migrated_row_count": migrated_row_count,
    }


def assert_duckdb_diagnostic_migration_settled(
    engine: DiagnosticDuckDBEngine,
    *,
    case_id: str,
) -> None:
    """Fail closed before any read-only case DB is exposed."""

    expected_case_id = _required_key(case_id)
    if tuple(_table_columns(engine, _JOURNAL_TABLE)) != _JOURNAL_COLUMNS:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_migration_required")
    rows = engine.query(
        f"SELECT schema_version, phase, plan_hash, target_count, case_binding_hash, "
        "schema_manifest_hash, protected_schema_hash, projection_hash "
        f"FROM {_JOURNAL_TABLE} WHERE migration_id=?",
        (_MIGRATION_ID,),
    )
    if len(rows) != 1:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_migration_required")
    (
        schema_version,
        phase,
        plan_hash,
        target_count,
        case_binding_hash,
        schema_manifest_hash,
        protected_schema_hash,
        projection_hash,
    ) = rows[0]
    if (
        schema_version != 2
        or phase != "settled"
        or plan_hash != _PLAN_HASH
        or target_count != len(_TARGET_TABLES)
        or case_binding_hash != _case_binding_hash(expected_case_id)
        or type(schema_manifest_hash) is not str
        or re.fullmatch(r"[a-f0-9]{64}", schema_manifest_hash) is None
        or type(protected_schema_hash) is not str
        or re.fullmatch(r"[a-f0-9]{64}", protected_schema_hash) is None
        or type(projection_hash) is not str
        or re.fullmatch(r"[a-f0-9]{64}", projection_hash) is None
    ):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_migration_required")
    _reject_populated_unknown_legacy_tables(engine)
    _project_target_tables(engine, expected_case_id=expected_case_id, write=False)


def _project_target_tables(
    engine: DiagnosticDuckDBEngine,
    *,
    expected_case_id: str,
    write: bool,
) -> int:
    _reject_populated_unknown_legacy_tables(engine)
    return sum(
        (
            _migrate_import_file_log(
                engine,
                table="import_file_log",
                expected_case_id=expected_case_id,
                write=write,
            ),
            _migrate_import_file_log(
                engine,
                table="import_file_log_recycle",
                expected_case_id=expected_case_id,
                write=write,
            ),
            _migrate_cleaning_log(
                engine,
                table="cleaning_log",
                expected_case_id=expected_case_id,
                write=write,
            ),
            _migrate_cleaning_log(
                engine,
                table="cleaning_log_recycle",
                expected_case_id=expected_case_id,
                write=write,
            ),
            _migrate_query_log(
                engine,
                expected_case_id=expected_case_id,
                write=write,
            ),
            _migrate_operational_events(
                engine,
                table="analysis_audit_event",
                run_log=False,
                expected_case_id=expected_case_id,
                write=write,
            ),
            _migrate_operational_events(
                engine,
                table="analysis_run_log_event",
                run_log=True,
                expected_case_id=expected_case_id,
                write=write,
            ),
            _migrate_temp_scopes(
                engine,
                expected_case_id=expected_case_id,
                write=write,
            ),
            _migrate_revision_state(engine, write=write),
            _migrate_document_diagnostics(
                engine,
                table="document_assets",
                expected_case_id=expected_case_id,
                write=write,
            ),
            _migrate_document_diagnostics(
                engine,
                table="document_assets_recycle",
                expected_case_id=expected_case_id,
                write=write,
            ),
            _migrate_privacy_runtime_state(
                engine,
                expected_case_id=expected_case_id,
                write=write,
            ),
        )
    )


def _ensure_journal(engine: DiagnosticDuckDBEngine) -> None:
    engine.execute(
        f"""CREATE TABLE IF NOT EXISTS {_JOURNAL_TABLE}(
            migration_id TEXT PRIMARY KEY,
            schema_version INTEGER NOT NULL,
            phase TEXT NOT NULL,
            plan_hash TEXT NOT NULL,
            target_count INTEGER NOT NULL,
            migrated_row_count BIGINT NOT NULL,
            case_binding_hash TEXT NOT NULL,
            schema_manifest_hash TEXT NOT NULL,
            protected_schema_hash TEXT NOT NULL,
            projection_hash TEXT NOT NULL
        )"""
    )
    columns = _table_columns(engine, _JOURNAL_TABLE)
    if tuple(columns) != _JOURNAL_COLUMNS:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_journal_schema_invalid")


def _journal_phase(engine: DiagnosticDuckDBEngine) -> str:
    if tuple(_table_columns(engine, _JOURNAL_TABLE)) != _JOURNAL_COLUMNS:
        return ""
    rows = engine.query(
        f"SELECT phase FROM {_JOURNAL_TABLE} WHERE migration_id=?",
        (_MIGRATION_ID,),
    )
    if len(rows) != 1 or len(rows[0]) != 1 or type(rows[0][0]) is not str:
        return ""
    return rows[0][0]


def _validate_or_create_journal_row(
    engine: DiagnosticDuckDBEngine,
    *,
    case_binding_hash: str,
    initial_schema_manifest_hash: str = "",
    initial_protected_schema_hash: str = "",
    initial_projection_hash: str = "",
) -> None:
    rows = engine.query(
        f"SELECT schema_version, phase, plan_hash, target_count, migrated_row_count, "
        "case_binding_hash, schema_manifest_hash, protected_schema_hash, projection_hash "
        f"FROM {_JOURNAL_TABLE} WHERE migration_id=?",
        (_MIGRATION_ID,),
    )
    if not rows:
        if not all(
            _is_sha256(value)
            for value in (
                initial_schema_manifest_hash,
                initial_protected_schema_hash,
                initial_projection_hash,
            )
        ):
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_journal_invalid")
        engine.execute(
            f"INSERT INTO {_JOURNAL_TABLE} VALUES(?,?,?,?,?,?,?,?,?,?)",
            (
                _MIGRATION_ID,
                2,
                "planned",
                _PLAN_HASH,
                len(_TARGET_TABLES),
                0,
                case_binding_hash,
                initial_schema_manifest_hash,
                initial_protected_schema_hash,
                initial_projection_hash,
            ),
        )
        return
    if len(rows) != 1:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_journal_invalid")
    (
        schema_version,
        phase,
        plan_hash,
        target_count,
        migrated_row_count,
        stored_case_binding_hash,
        schema_manifest_hash,
        protected_schema_hash,
        projection_hash,
    ) = rows[0]
    if (
        type(schema_version) is not int
        or schema_version != 2
        or phase not in {"planned", "applied", "settled"}
        or plan_hash != _PLAN_HASH
        or type(target_count) is not int
        or target_count != len(_TARGET_TABLES)
        or type(migrated_row_count) is not int
        or migrated_row_count < 0
        or stored_case_binding_hash != case_binding_hash
        or not _is_sha256(schema_manifest_hash)
        or not _is_sha256(protected_schema_hash)
        or not _is_sha256(projection_hash)
    ):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_journal_invalid")


def _journal_record(engine: DiagnosticDuckDBEngine) -> dict[str, int | str]:
    rows = engine.query(
        f"SELECT phase, migrated_row_count, schema_manifest_hash, "
        f"protected_schema_hash, projection_hash FROM {_JOURNAL_TABLE} WHERE migration_id=?",
        (_MIGRATION_ID,),
    )
    if len(rows) != 1 or len(rows[0]) != 5:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_journal_invalid")
    phase, migrated_row_count, schema_manifest_hash, protected_schema_hash, projection_hash = rows[0]
    return {
        "phase": str(phase or ""),
        "migrated_row_count": int(migrated_row_count or 0),
        "schema_manifest_hash": str(schema_manifest_hash or ""),
        "protected_schema_hash": str(protected_schema_hash or ""),
        "projection_hash": str(projection_hash or ""),
    }


def _migrate_import_file_log(
    engine: DiagnosticDuckDBEngine,
    *,
    table: str,
    expected_case_id: str,
    write: bool,
) -> int:
    columns = _validated_target_columns(engine, table)
    if not columns:
        return 0
    required = {"file_id", "case_id"}
    if not required.issubset(columns):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    selected = [
        column
        for column in (
            "file_id",
            "case_id",
            "status",
            "error",
            "cleaned_status",
            "cleaned_error",
        )
        if column in columns
    ]
    rows = _query_rows(
        engine,
        table,
        selected,
        order_by="file_id",
        stable_row_identity=True,
    )
    updated = 0
    for row in rows:
        file_id = _required_file_id(row.get("file_id"))
        _require_case_binding(row.get("case_id"), expected_case_id)
        assignments: dict[str, Any] = {}
        if "status" in columns:
            assignments["status"] = _closed_status(row.get("status"), allowed=_IMPORT_STATUSES)
        if "error" in columns:
            assignments["error"] = project_import_error(row.get("error"), status=row.get("status"))
        if "cleaned_status" in columns:
            assignments["cleaned_status"] = _closed_status(
                row.get("cleaned_status"),
                allowed=_CLEANING_STATUSES,
            )
        if "cleaned_error" in columns:
            assignments["cleaned_error"] = project_cleaning_error(
                row.get("cleaned_error"),
                status=row.get("cleaned_status"),
            )
        updated += _update_by_key(
            engine,
            table=table,
            key_column="file_id",
            key=file_id,
            current=row,
            assignments=assignments,
            write=write,
        )
    return updated


def _migrate_cleaning_log(
    engine: DiagnosticDuckDBEngine,
    *,
    table: str,
    expected_case_id: str,
    write: bool,
) -> int:
    columns = _validated_target_columns(engine, table)
    if not columns:
        return 0
    if not {"id", "case_id"}.issubset(columns):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    selected = [
        column
        for column in (
            "id",
            "case_id",
            "file_id",
            "import_at",
            "cleaned_at",
            "duration_ms",
            "scope_rows",
            "summary",
            "run_ref",
        )
        if column in columns
    ]
    rows = _query_rows(
        engine,
        table,
        selected,
        order_by="id",
        stable_row_identity=True,
    )
    updated = 0
    for row in rows:
        row_id = _required_integer_key(row.get("id"))
        _require_case_binding(row.get("case_id"), expected_case_id)
        file_id = _optional_file_id(row.get("file_id"))
        summary, summary_was_restricted = _json_mapping(row.get("summary"))
        if summary_was_restricted:
            summary = {**summary, "restricted_details_withheld": True}
        projected = project_cleaning_history_item(
            case_id=str(row.get("case_id") or ""),
            row={**row, "summary": summary},
        )
        projected_summary = project_cleaning_summary_diagnostic(projected["summary"])
        assignments: dict[str, Any] = {}
        if "summary" in columns:
            assignments["summary"] = _canonical_json(projected_summary)
        if "run_ref" in columns:
            assignments["run_ref"] = _closed_cleaning_run_ref(
                row_id=row_id,
                case_id=str(row.get("case_id") or ""),
                current=row.get("run_ref"),
            )
        if "import_at" in columns:
            assignments["import_at"] = projected["import_at"]
        if "cleaned_at" in columns:
            assignments["cleaned_at"] = projected["cleaned_at"]
        if "file_id" in columns:
            assignments["file_id"] = file_id
        updated += _update_by_key(
            engine,
            table=table,
            key_column="id",
            key=row_id,
            current=row,
            assignments=assignments,
            write=write,
        )
    return updated


def _migrate_query_log(
    engine: DiagnosticDuckDBEngine,
    *,
    expected_case_id: str,
    write: bool,
) -> int:
    columns = _validated_target_columns(engine, "analysis_query_log")
    if not columns:
        return 0
    if not {"query_id", "case_id"}.issubset(columns):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    selected = [
        column
        for column in (
            "query_id",
            "case_id",
            "tool_name",
            "params_json",
            "summary_json",
            "row_count",
            "created_at",
        )
        if column in columns
    ]
    rows = _query_rows(
        engine,
        "analysis_query_log",
        selected,
        order_by="query_id",
        stable_row_identity=True,
    )
    updated = 0
    for row in rows:
        query_id = _required_key(row.get("query_id"))
        case_id = _required_key(row.get("case_id"))
        _require_case_binding(case_id, expected_case_id)
        params, params_restricted = _json_mapping(row.get("params_json"))
        summary, summary_restricted = _json_mapping(row.get("summary_json"))
        if params_restricted:
            params = {**params, "restricted_details_withheld": True}
        if summary_restricted:
            summary = {**summary, "restricted_details_withheld": True}
        public_params, public_summary = project_query_log_diagnostic(
            case_id=case_id,
            query_id=query_id,
            params=params,
            summary=summary,
        )
        assignments: dict[str, Any] = {}
        assignments["query_id"] = public_summary["query_ref"]
        if "tool_name" in columns:
            assignments["tool_name"] = project_query_tool_name(row.get("tool_name"))
        if "params_json" in columns:
            assignments["params_json"] = _canonical_json(public_params)
        if "summary_json" in columns:
            assignments["summary_json"] = _canonical_json(public_summary)
        if "row_count" in columns:
            assignments["row_count"] = None
        if "created_at" in columns:
            assignments["created_at"] = project_diagnostic_timestamp(row.get("created_at"))
        updated += _update_by_key(
            engine,
            table="analysis_query_log",
            key_column="query_id",
            key=query_id,
            current=row,
            assignments=assignments,
            write=write,
        )
    return updated


def _migrate_operational_events(
    engine: DiagnosticDuckDBEngine,
    *,
    table: str,
    run_log: bool,
    expected_case_id: str,
    write: bool,
) -> int:
    columns = _validated_target_columns(engine, table)
    if not columns:
        return 0
    if not {"event_id", "case_id"}.issubset(columns):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    candidate_columns = (
        "event_id",
        "case_id",
        "task_id",
        "run_id",
        "turn_id",
        "sequence_no",
        "event_stage",
        "event_type",
        "source",
        "artifact_id",
        "approval_id",
        "trace_id",
        "span_id",
        "actor_id",
        "actor_role",
        "tenant_id",
        "request_id",
        "session_id",
        "payload_json",
        "created_at",
    )
    selected = [column for column in candidate_columns if column in columns]
    rows = _query_rows(
        engine,
        table,
        selected,
        order_by="event_id",
        stable_row_identity=True,
    )
    updated = 0
    for row in rows:
        event_id = _required_key(row.get("event_id"))
        case_id = _required_key(row.get("case_id"))
        _require_case_binding(case_id, expected_case_id)
        payload, payload_restricted = _json_mapping(row.get("payload_json"))
        if payload_restricted:
            payload = {**payload, "restricted_details_withheld": True}
        projected = project_operational_event(
            event={
                **row,
                "timestamp": row.get("created_at"),
                "payload": payload,
            },
            run_log=run_log,
            case_id=case_id,
        )
        assignments: dict[str, Any] = {}
        assignments["event_id"] = projected["event_id"] or opaque_case_bound_ref(
            prefix="runlogref_v1" if run_log else "auditref_v1",
            case_id=case_id,
            value=event_id,
        )
        for column in ("task_id", "run_id", "turn_id", "event_type"):
            if column in columns:
                assignments[column] = projected[column]
        if "payload_json" in columns:
            assignments["payload_json"] = _canonical_json(projected["payload"])
        if "created_at" in columns:
            assignments["created_at"] = projected["timestamp"]
        if run_log:
            if "event_stage" in columns:
                assignments["event_stage"] = projected["event_stage"]
            if "source" in columns:
                assignments["source"] = projected["source"]
        else:
            for column in (
                "artifact_id",
                "approval_id",
                "trace_id",
                "span_id",
                "request_id",
                "session_id",
            ):
                if column in columns:
                    assignments[column] = ""
            if "actor_id" in columns:
                assignments["actor_id"] = projected["actor_id"]
            if "actor_role" in columns:
                assignments["actor_role"] = projected["actor_role"]
            if "tenant_id" in columns:
                assignments["tenant_id"] = projected["tenant_id"]
        updated += _update_by_key(
            engine,
            table=table,
            key_column="event_id",
            key=event_id,
            current=row,
            assignments=assignments,
            write=write,
        )
        _ = case_id
    return updated


def _migrate_temp_scopes(
    engine: DiagnosticDuckDBEngine,
    *,
    expected_case_id: str,
    write: bool,
) -> int:
    table = "analysis_temp_scope"
    columns = _validated_target_columns(engine, table)
    if not columns:
        return 0
    if not {"scope_id", "case_id"}.issubset(columns):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    selected = [
        column
        for column in (
            "scope_id",
            "case_id",
            "scope_type",
            "source_kind",
            "scope_signature",
            "status",
            "source_revision",
            "source_file_ids_json",
            "document_ids_json",
            "stats_json",
            "audit_json",
            "expires_at",
            "retention_until",
            "last_accessed_at",
            "created_at",
            "updated_at",
        )
        if column in columns
    ]
    rows = _query_rows(
        engine,
        table,
        selected,
        order_by="scope_id",
        stable_row_identity=True,
    )
    updated = 0
    for row in rows:
        scope_id = _required_key(row.get("scope_id"))
        case_id = _required_key(row.get("case_id"))
        _require_case_binding(case_id, expected_case_id)
        if _has_migrated_temp_scope_marker(row):
            if not _is_migrated_temp_scope_row(row):
                raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid")
            continue
        if _has_current_temp_scope_marker(row) and not _is_current_temp_scope_row(row):
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid")
        if _is_current_temp_scope_row(row):
            continue
        source_refs, source_input_valid = _closed_ref_list(
            row.get("source_file_ids_json"),
            case_id=case_id,
            prefix="tempsrc_v1",
        )
        document_refs, document_input_valid = _closed_ref_list(
            row.get("document_ids_json"),
            case_id=case_id,
            prefix="tempdoc_v1",
        )
        requested_source_count: int | None = (
            len(source_refs) + len(document_refs)
            if source_input_valid and document_input_valid
            else None
        )
        migrated_at = project_diagnostic_timestamp(
            row.get("updated_at") or row.get("created_at")
        )
        stats = {
            "contract": "TempScopeOperationalStatsV2",
            "requested_source_count": requested_source_count,
            "resolved_source_count": None,
            "failure_count": None,
            "failures": [],
            "lifecycle_warning_codes": ["TEMP_SCOPE_LEGACY_DIAGNOSTIC_MIGRATED"],
            "fact_answer_allowed": False,
            "raw_details_exposed": False,
            "restricted_details_withheld": True,
            "lifecycle": {
                "status": "expired",
                "expires_at": project_diagnostic_timestamp(row.get("expires_at")),
                "retention_until": project_diagnostic_timestamp(row.get("retention_until")),
            },
        }
        audit = {
            "contract": "TempScopeAuditV2",
            "status": "expired",
            "requested_source_count": requested_source_count,
            "resolved_source_count": None,
            "failure_count": None,
            "cleanup_reason": "legacy_diagnostic_migrated",
            "events": [
                {
                    "action": "expired",
                    "reason": "legacy_diagnostic_migrated",
                    "at": migrated_at,
                }
            ],
            "restricted_details_withheld": True,
        }
        signature = str(row.get("scope_signature") or "").strip()
        if re.fullmatch(r"temp_scope_migration_v1_[a-f0-9]{64}", signature) is None:
            signature = opaque_case_bound_ref(
                prefix="temp_scope_migration_v1",
                case_id=case_id,
                value=scope_id,
            )
        assignments: dict[str, Any] = {}
        assignments["scope_id"] = (
            scope_id
            if _TEMP_SCOPE_ID_RE.fullmatch(scope_id)
            else opaque_case_bound_ref(
                prefix="temp_scope_v1",
                case_id=case_id,
                value=scope_id,
            )
        )
        for column, value in (
            ("scope_type", "file_scope"),
            ("source_kind", "legacy_withheld"),
            ("scope_signature", signature),
            ("status", "expired"),
            ("source_file_ids_json", _canonical_json_list(source_refs)),
            ("document_ids_json", _canonical_json_list(document_refs)),
            ("stats_json", _canonical_json(stats)),
            ("audit_json", _canonical_json(audit)),
            ("expires_at", project_diagnostic_timestamp(row.get("expires_at"))),
            ("retention_until", project_diagnostic_timestamp(row.get("retention_until"))),
            ("last_accessed_at", project_diagnostic_timestamp(row.get("last_accessed_at"))),
            ("created_at", project_diagnostic_timestamp(row.get("created_at"))),
            ("updated_at", migrated_at),
        ):
            if column in columns:
                assignments[column] = value
        updated += _update_by_key(
            engine,
            table=table,
            key_column="scope_id",
            key=scope_id,
            current=row,
            assignments=assignments,
            write=write,
        )
    return updated


def _is_current_temp_scope_row(row: Mapping[str, Any]) -> bool:
    scope_id = row.get("scope_id")
    signature = row.get("scope_signature")
    status = row.get("status")
    source_revision = row.get("source_revision")
    if (
        type(scope_id) is not str
        or _TEMP_SCOPE_ID_RE.fullmatch(scope_id) is None
        or row.get("scope_type") != "file_scope"
        or row.get("source_kind") not in _TEMP_SCOPE_SOURCE_KINDS
        or type(signature) is not str
        or _TEMP_SCOPE_SIGNATURE_RE.fullmatch(signature) is None
        or scope_id
        != opaque_case_bound_ref(
            prefix="temp_scope_v1",
            case_id=str(row.get("case_id") or ""),
            value=signature,
        )
        or status not in {"active", "expired"}
        or type(source_revision) is not int
        or source_revision < 0
    ):
        return False
    source_refs = _strict_opaque_ref_list(
        row.get("source_file_ids_json"),
        expected=_TEMP_SOURCE_REF_RE,
    )
    document_refs = _strict_opaque_ref_list(
        row.get("document_ids_json"),
        expected=_TEMP_DOCUMENT_REF_RE,
    )
    if source_refs is None or document_refs is None:
        return False
    if not all(
        _valid_diagnostic_timestamp(row.get(column), required=True)
        for column in (
            "expires_at",
            "retention_until",
            "last_accessed_at",
            "created_at",
            "updated_at",
        )
    ):
        return False
    stats, stats_invalid = _json_mapping(row.get("stats_json"))
    audit, audit_invalid = _json_mapping(row.get("audit_json"))
    if stats_invalid or audit_invalid:
        return False
    stats_valid = _valid_current_temp_scope_stats(
        stats,
        status=status,
        source_refs=source_refs,
        document_refs=document_refs,
        expires_at=row.get("expires_at"),
        retention_until=row.get("retention_until"),
    )
    return stats_valid and _valid_current_temp_scope_audit(
        audit,
        status=status,
        stats=stats,
        updated_at=row.get("updated_at"),
    )


def _has_current_temp_scope_marker(row: Mapping[str, Any]) -> bool:
    stats, stats_invalid = _json_mapping(row.get("stats_json"))
    audit, audit_invalid = _json_mapping(row.get("audit_json"))
    return bool(
        (
            type(row.get("scope_signature")) is str
            and str(row.get("scope_signature")).startswith("temp_scope_v2_")
        )
        or (not stats_invalid and stats.get("contract") == "TempScopeOperationalStatsV2")
        or (not audit_invalid and audit.get("contract") == "TempScopeAuditV2")
    )


def _has_migrated_temp_scope_marker(row: Mapping[str, Any]) -> bool:
    stats, stats_invalid = _json_mapping(row.get("stats_json"))
    warnings = stats.get("lifecycle_warning_codes") if not stats_invalid else None
    return bool(
        row.get("source_kind") == "legacy_withheld"
        or (
            type(row.get("scope_signature")) is str
            and str(row.get("scope_signature")).startswith("temp_scope_migration_v1_")
        )
        or (
            isinstance(warnings, list)
            and "TEMP_SCOPE_LEGACY_DIAGNOSTIC_MIGRATED" in warnings
        )
    )


def _is_migrated_temp_scope_row(row: Mapping[str, Any]) -> bool:
    scope_id = row.get("scope_id")
    signature = row.get("scope_signature")
    if (
        type(scope_id) is not str
        or _TEMP_SCOPE_ID_RE.fullmatch(scope_id) is None
        or row.get("scope_type") != "file_scope"
        or row.get("source_kind") != "legacy_withheld"
        or type(signature) is not str
        or re.fullmatch(r"temp_scope_migration_v1_[a-f0-9]{64}", signature) is None
        or scope_id.removeprefix("temp_scope_v1_")
        != signature.removeprefix("temp_scope_migration_v1_")
        or row.get("status") != "expired"
        or row.get("source_revision") not in {None, 0}
    ):
        return False
    source_refs = _strict_opaque_ref_list(
        row.get("source_file_ids_json"),
        expected=_TEMP_SOURCE_REF_RE,
    )
    document_refs = _strict_opaque_ref_list(
        row.get("document_ids_json"),
        expected=_TEMP_DOCUMENT_REF_RE,
    )
    if source_refs is None or document_refs is None:
        return False
    if not all(
        _valid_diagnostic_timestamp(row.get(column), required=False)
        for column in (
            "expires_at",
            "retention_until",
            "last_accessed_at",
            "created_at",
            "updated_at",
        )
    ):
        return False
    stats, stats_invalid = _json_mapping(row.get("stats_json"))
    audit, audit_invalid = _json_mapping(row.get("audit_json"))
    requested_count = stats.get("requested_source_count")
    if requested_count not in {None, len(source_refs) + len(document_refs)}:
        return False
    expected_stats = {
        "contract": "TempScopeOperationalStatsV2",
        "requested_source_count": requested_count,
        "resolved_source_count": None,
        "failure_count": None,
        "failures": [],
        "lifecycle_warning_codes": ["TEMP_SCOPE_LEGACY_DIAGNOSTIC_MIGRATED"],
        "fact_answer_allowed": False,
        "raw_details_exposed": False,
        "restricted_details_withheld": True,
        "lifecycle": {
            "status": "expired",
            "expires_at": row.get("expires_at") or "",
            "retention_until": row.get("retention_until") or "",
        },
    }
    expected_audit = {
        "contract": "TempScopeAuditV2",
        "status": "expired",
        "requested_source_count": requested_count,
        "resolved_source_count": None,
        "failure_count": None,
        "cleanup_reason": "legacy_diagnostic_migrated",
        "events": [
            {
                "action": "expired",
                "reason": "legacy_diagnostic_migrated",
                "at": row.get("updated_at") or "",
            }
        ],
        "restricted_details_withheld": True,
    }
    return not stats_invalid and not audit_invalid and stats == expected_stats and audit == expected_audit


def _valid_current_temp_scope_stats(
    value: Mapping[str, Any],
    *,
    status: object,
    source_refs: list[str],
    document_refs: list[str],
    expires_at: object,
    retention_until: object,
) -> bool:
    lifecycle_statuses = {status}
    # The prior writer marked the row expired but left its closed operational
    # lifecycle at active. Accept that one exact historical drift while new
    # writes persist the matching expired lifecycle; arbitrary status upgrades
    # remain invalid.
    if status == "expired":
        lifecycle_statuses.add("active")
    expected_keys = {
        "contract",
        "requested_source_count",
        "resolved_source_count",
        "failure_count",
        "failures",
        "lifecycle_warning_codes",
        "fact_answer_allowed",
        "raw_details_exposed",
        "lifecycle",
    }
    if set(value) != expected_keys or value.get("contract") != "TempScopeOperationalStatsV2":
        return False
    requested = _strict_non_negative_int(value.get("requested_source_count"))
    resolved = _strict_non_negative_int(value.get("resolved_source_count"))
    failure_count = _strict_non_negative_int(value.get("failure_count"))
    if requested is None or resolved is None or failure_count is None or resolved > requested:
        return False
    if requested < len(source_refs) + len(document_refs):
        return False
    failures = value.get("failures")
    if not isinstance(failures, list) or failure_count != len(failures):
        return False
    for failure in failures:
        if not isinstance(failure, Mapping) or set(failure) != {
            "source_ref",
            "failure_code",
            "retryable",
            "fact_answer_allowed",
        }:
            return False
        if (
            type(failure.get("source_ref")) is not str
            or _TEMP_SOURCE_REF_RE.fullmatch(failure["source_ref"]) is None
            or failure.get("failure_code") not in _TEMP_SCOPE_FAILURE_CODES
            or type(failure.get("retryable")) is not bool
            or failure.get("fact_answer_allowed") is not False
        ):
            return False
    warnings = value.get("lifecycle_warning_codes")
    if (
        not isinstance(warnings, list)
        or len(warnings) != len(set(warnings))
        or any(type(item) is not str or item not in _TEMP_SCOPE_WARNING_CODES for item in warnings)
        or value.get("fact_answer_allowed") is not False
        or value.get("raw_details_exposed") is not False
    ):
        return False
    lifecycle = value.get("lifecycle")
    return bool(
        isinstance(lifecycle, Mapping)
        and set(lifecycle)
        == {"status", "ttl_hours", "expires_at", "retention_until", "active_limit"}
        and lifecycle.get("status") in lifecycle_statuses
        and lifecycle.get("ttl_hours") == 24
        and lifecycle.get("expires_at") == expires_at
        and lifecycle.get("retention_until") == retention_until
        and lifecycle.get("active_limit") == 16
    )


def _valid_current_temp_scope_audit(
    value: Mapping[str, Any],
    *,
    status: object,
    stats: Mapping[str, Any],
    updated_at: object,
) -> bool:
    if value.get("contract") != "TempScopeAuditV2" or value.get("status") != status:
        return False
    events = value.get("events")
    if not isinstance(events, list) or len(events) != 1 or not isinstance(events[0], Mapping):
        return False
    event = events[0]
    if status == "active":
        if set(value) != {
            "contract",
            "status",
            "requested_source_count",
            "resolved_source_count",
            "failure_count",
            "cleanup_policy",
            "events",
            "restricted_details_withheld",
        }:
            return False
        counts = tuple(
            _strict_non_negative_int(value.get(key))
            for key in ("requested_source_count", "resolved_source_count", "failure_count")
        )
        cleanup_policy = value.get("cleanup_policy")
        return bool(
            all(item is not None for item in counts)
            and counts[1] <= counts[0]
            and counts
            == (
                stats.get("requested_source_count"),
                stats.get("resolved_source_count"),
                stats.get("failure_count"),
            )
            and isinstance(cleanup_policy, Mapping)
            and dict(cleanup_policy)
            == {"ttl_hours": 24, "audit_retention_days": 30, "active_limit": 16}
            and set(event) == {"action", "at"}
            and event.get("action") == "created_or_updated"
            and event.get("at") == updated_at
            and _valid_diagnostic_timestamp(event.get("at"), required=True)
            and value.get("restricted_details_withheld") is True
        )
    if set(value) != {
        "contract",
        "status",
        "cleanup_reason",
        "cleanup_at",
        "events",
        "restricted_details_withheld",
    }:
        return False
    reason = value.get("cleanup_reason")
    return bool(
        reason in {"ttl_expired", "quota_trimmed", "lifecycle_cleanup"}
        and value.get("cleanup_at") == updated_at
        and _valid_diagnostic_timestamp(value.get("cleanup_at"), required=True)
        and set(event) == {"action", "reason", "at"}
        and event.get("action") == "expired"
        and event.get("reason") == reason
        and event.get("at") == updated_at
        and _valid_diagnostic_timestamp(event.get("at"), required=True)
        and value.get("restricted_details_withheld") is True
    )


def _migrate_revision_state(engine: DiagnosticDuckDBEngine, *, write: bool) -> int:
    table = "analysis_revision_state"
    columns = _validated_target_columns(engine, table)
    if not columns:
        return 0
    if "revision_key" not in columns:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    selected = [column for column in ("revision_key", "updated_at", "reason") if column in columns]
    updated_at_type = _target_column_type(engine, table=table, column="updated_at")
    rows = _query_rows(
        engine,
        table,
        selected,
        order_by="revision_key",
        stable_row_identity=True,
    )
    updated = 0
    for row in rows:
        revision_key = _required_key(row.get("revision_key"))
        if revision_key != "stats_flow_source":
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_binding_invalid")
        assignments = (
            {"reason": project_analysis_revision_reason(row.get("reason"))}
            if "reason" in columns
            else {}
        )
        if "updated_at" in columns and updated_at_type == "VARCHAR":
            assignments["updated_at"] = project_diagnostic_timestamp(row.get("updated_at"))
        updated += _update_by_key(
            engine,
            table=table,
            key_column="revision_key",
            key=revision_key,
            current=row,
            assignments=assignments,
            write=write,
        )
    return updated


def _migrate_document_diagnostics(
    engine: DiagnosticDuckDBEngine,
    *,
    table: str,
    expected_case_id: str,
    write: bool,
) -> int:
    columns = _validated_target_columns(engine, table)
    if not columns:
        return 0
    if not {"document_id", "case_id"}.issubset(columns):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    selected = [
        column
        for column in ("document_id", "case_id", "last_error", "duplicate_reason")
        if column in columns
    ]
    rows = _query_rows(
        engine,
        table,
        selected,
        order_by="document_id",
        stable_row_identity=True,
    )
    updated = 0
    for row in rows:
        document_id = _required_document_id(row.get("document_id"))
        _require_case_binding(row.get("case_id"), expected_case_id)
        assignments: dict[str, Any] = {}
        if "last_error" in columns:
            assignments["last_error"] = (
                "document_extract_failed" if str(row.get("last_error") or "") else ""
            )
        if "duplicate_reason" in columns:
            assignments["duplicate_reason"] = (
                "same_sha256" if row.get("duplicate_reason") == "same_sha256" else ""
            )
        updated += _update_by_key(
            engine,
            table=table,
            key_column="document_id",
            key=document_id,
            current=row,
            assignments=assignments,
            write=write,
        )
    return updated


def _migrate_privacy_runtime_state(
    engine: DiagnosticDuckDBEngine,
    *,
    expected_case_id: str,
    write: bool,
) -> int:
    table = "privacy_runtime_state"
    columns = _validated_target_columns(engine, table)
    if not columns:
        return 0
    if "case_id" not in columns:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    selected = [column for column in ("case_id", "summary", "error") if column in columns]
    rows = _query_rows(
        engine,
        table,
        selected,
        order_by="case_id",
        stable_row_identity=True,
    )
    updated = 0
    for row in rows:
        case_id = _required_key(row.get("case_id"))
        _require_case_binding(case_id, expected_case_id)
        assignments: dict[str, Any] = {}
        if "summary" in columns:
            assignments["summary"] = ""
        if "error" in columns:
            assignments["error"] = (
                "privacy_projection_failed" if str(row.get("error") or "") else ""
            )
        updated += _update_by_key(
            engine,
            table=table,
            key_column="case_id",
            key=case_id,
            current=row,
            assignments=assignments,
            write=write,
        )
    return updated


def _reject_populated_unknown_legacy_tables(engine: DiagnosticDuckDBEngine) -> None:
    for table in _UNKNOWN_LEGACY_DIAGNOSTIC_TABLES:
        if not _table_columns(engine, table):
            continue
        rows = engine.query(f"SELECT COUNT(1) FROM {table}")
        if not rows or len(rows[0]) != 1:
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_unknown_schema")
        try:
            row_count = int(rows[0][0] or 0)
        except (TypeError, ValueError, OverflowError):
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_unknown_schema") from None
        if row_count != 0:
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_unknown_schema")


def _query_rows(
    engine: DiagnosticDuckDBEngine,
    table: str,
    columns: list[str],
    *,
    order_by: str,
    stable_row_identity: bool,
) -> Iterator[dict[str, Any]]:
    if (
        table not in _TARGET_TABLES
        or order_by != _TARGET_PRIMARY_KEYS.get(table)
        or not columns
        or any(column not in _ALLOWED_TARGET_COLUMNS[table] for column in columns)
    ):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    count_rows = engine.query(f"SELECT COUNT(1) FROM {table}")
    if len(count_rows) != 1 or len(count_rows[0]) != 1:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid")
    try:
        row_count = int(count_rows[0][0] or 0)
    except (TypeError, ValueError, OverflowError):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid") from None
    if row_count < 0 or row_count > _MAX_TARGET_ROWS:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_limit")
    plan_table = f"analytix_diagnostic_plan_{table}"
    if stable_row_identity:
        try:
            engine.execute(
                f"CREATE TEMP TABLE {plan_table} AS "
                f"SELECT row_number() OVER (ORDER BY {order_by})::BIGINT AS ordinal, "
                f"{order_by} AS source_key FROM {table} ORDER BY {order_by}"
            )
            plan_count = engine.query(f"SELECT COUNT(1) FROM {plan_table}")
        except Exception:
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid") from None
        if plan_count != [(row_count,)]:
            try:
                engine.execute(f"DROP TABLE {plan_table}")
            except Exception:
                pass
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid")
    last_key: object | None = None
    emitted = 0
    completed = False
    try:
        while emitted < row_count:
            selected_columns = ", ".join(f"target.{column}" for column in columns)
            if stable_row_identity:
                cursor = int(last_key or 0)
                rows = engine.query(
                    f"SELECT plan.ordinal, {selected_columns} FROM {plan_table} AS plan "
                    f"JOIN {table} AS target ON target.{order_by}=plan.source_key "
                    f"WHERE plan.ordinal>? ORDER BY plan.ordinal LIMIT {_HASH_PAGE_ROWS}",
                    (cursor,),
                )
            elif last_key is None:
                rows = engine.query(
                    f"SELECT {', '.join(columns)} FROM {table} "
                    f"ORDER BY {order_by} LIMIT {_HASH_PAGE_ROWS}"
                )
            else:
                rows = engine.query(
                    f"SELECT {', '.join(columns)} FROM {table} WHERE {order_by}>? "
                    f"ORDER BY {order_by} LIMIT {_HASH_PAGE_ROWS}",
                    (last_key,),
                )
            if not rows:
                raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid")
            for row in rows:
                expected_width = len(columns) + (1 if stable_row_identity else 0)
                if len(row) != expected_width:
                    raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid")
                if stable_row_identity:
                    key = row[0]
                    projected = dict(zip(columns, row[1:], strict=True))
                    if type(key) is not int or key <= 0:
                        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid")
                else:
                    projected = dict(zip(columns, row, strict=True))
                    key = projected.get(order_by)
                if key is None or (last_key is not None and key <= last_key):
                    raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid")
                last_key = key
                emitted += 1
                yield projected
            if len(rows) < _HASH_PAGE_ROWS and emitted != row_count:
                raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid")
        if emitted != row_count:
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid")
        completed = True
    finally:
        if stable_row_identity:
            try:
                engine.execute(f"DROP TABLE IF EXISTS {plan_table}")
            except Exception:
                if completed:
                    raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid") from None


def _update_by_key(
    engine: DiagnosticDuckDBEngine,
    *,
    table: str,
    key_column: str,
    key: Any,
    current: Mapping[str, Any],
    assignments: Mapping[str, Any],
    write: bool,
) -> int:
    changed = {
        name: value
        for name, value in assignments.items()
        if not _diagnostic_values_equivalent(
            column=name,
            current=current.get(name),
            projected=value,
        )
    }
    if not changed:
        return 0
    if not write:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid")
    fields = ", ".join(f"{name}=?" for name in changed)
    engine.execute(
        f"UPDATE {table} SET {fields} WHERE {key_column}=?",
        (*changed.values(), key),
    )
    return 1


def _diagnostic_values_equivalent(
    *,
    column: str,
    current: object,
    projected: object,
) -> bool:
    if current == projected:
        return True
    if column in _JSON_DIAGNOSTIC_COLUMNS:
        current_mapping, current_invalid = _json_mapping(current)
        projected_mapping, projected_invalid = _json_mapping(projected)
        return not current_invalid and not projected_invalid and current_mapping == projected_mapping
    if column in _TIMESTAMP_DIAGNOSTIC_COLUMNS:
        current_timestamp = project_diagnostic_timestamp(current)
        projected_timestamp = project_diagnostic_timestamp(projected)
        return bool(current_timestamp) and current_timestamp == projected_timestamp
    return False


def _table_columns(engine: DiagnosticDuckDBEngine, table: str) -> list[str]:
    rows = engine.query(
        "SELECT column_name FROM information_schema.columns "
        "WHERE table_schema='main' AND table_name=? ORDER BY ordinal_position",
        (table,),
    )
    return [str(row[0] or "") for row in rows if row and row[0]]


def _target_column_type(
    engine: DiagnosticDuckDBEngine,
    *,
    table: str,
    column: str,
) -> str:
    rows = engine.query(
        "SELECT data_type FROM information_schema.columns "
        "WHERE table_schema='main' AND table_name=? AND column_name=?",
        (table, column),
    )
    if len(rows) != 1 or len(rows[0]) != 1 or type(rows[0][0]) is not str:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    return rows[0][0].upper()


def _validated_target_columns(engine: DiagnosticDuckDBEngine, table: str) -> set[str]:
    columns = set(_table_columns(engine, table))
    if not columns:
        return set()
    allowed = _ALLOWED_TARGET_COLUMNS.get(table)
    supported_sets = _SUPPORTED_TARGET_COLUMN_SETS.get(table, ())
    if (
        allowed is None
        or not columns.issubset(allowed)
        or frozenset(columns) not in supported_sets
    ):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    metadata = engine.query(
        "SELECT column_name, data_type FROM information_schema.columns "
        "WHERE table_schema='main' AND table_name=? ORDER BY ordinal_position",
        (table,),
    )
    if len(metadata) != len(columns):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    for row in metadata:
        if len(row) != 2 or type(row[0]) is not str or type(row[1]) is not str:
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
        if row[1].upper() not in _expected_target_column_types(table, row[0]):
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    _validate_target_key_constraint(engine, table=table, columns=columns)
    return columns


def _expected_target_column_types(table: str, column: str) -> frozenset[str]:
    if column in _BIGINT_TARGET_COLUMNS:
        return frozenset({"BIGINT"})
    if column in _BOOLEAN_TARGET_COLUMNS:
        return frozenset({"BOOLEAN"})
    if column == "progress":
        return frozenset({"INTEGER"})
    if table == "analysis_revision_state" and column == "updated_at":
        return frozenset({"TIMESTAMP", "VARCHAR"})
    return frozenset({"VARCHAR"})


def _validate_target_key_constraint(
    engine: DiagnosticDuckDBEngine,
    *,
    table: str,
    columns: set[str],
) -> None:
    primary_key = _TARGET_PRIMARY_KEYS[table]
    if primary_key not in columns:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    count_rows = engine.query(
        f"SELECT COUNT(1), COUNT({primary_key}), COUNT(DISTINCT {primary_key}) FROM {table}"
    )
    if len(count_rows) != 1 or len(count_rows[0]) != 3:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    try:
        total, non_null, distinct = (int(value or 0) for value in count_rows[0])
    except (TypeError, ValueError, OverflowError):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid") from None
    if total != non_null or total != distinct:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    if table in _RECYCLE_TABLES_WITHOUT_PRIMARY_KEYS:
        return
    constraints = engine.query(
        "SELECT constraint_column_names FROM duckdb_constraints() "
        "WHERE schema_name='main' AND table_name=? AND constraint_type='PRIMARY KEY'",
        (table,),
    )
    if len(constraints) != 1 or len(constraints[0]) != 1:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
    constrained_columns = constraints[0][0]
    if not isinstance(constrained_columns, (list, tuple)) or tuple(constrained_columns) != (
        primary_key,
    ):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")


def _case_binding_hash(case_id: str) -> str:
    return hashlib.sha256(f"analytix-case-binding-v1\x00{case_id}".encode("utf-8")).hexdigest()


def _target_schema_manifest_hash(engine: DiagnosticDuckDBEngine) -> str:
    digest = hashlib.sha256(b"analytix-diagnostic-schema-v2\x00")
    for table in _TARGET_TABLES:
        columns = _validated_target_columns(engine, table)
        _hash_field(digest, table)
        if not columns:
            _hash_field(digest, "absent")
            continue
        rows = engine.query(
            "SELECT column_name, data_type, is_nullable FROM information_schema.columns "
            "WHERE table_schema='main' AND table_name=? ORDER BY ordinal_position",
            (table,),
        )
        for row in rows:
            _hash_field(digest, json.dumps(row, ensure_ascii=False, default=str))
    return digest.hexdigest()


def _protected_schema_hash(engine: DiagnosticDuckDBEngine) -> str:
    excluded = {*_TARGET_TABLES, _JOURNAL_TABLE, "analytix_diagnostic_migration_journal"}
    rows = engine.query(
        "SELECT table_name, column_name, data_type, is_nullable "
        "FROM information_schema.columns WHERE table_schema='main' "
        "ORDER BY table_name, ordinal_position"
    )
    digest = hashlib.sha256(b"analytix-protected-schema-v1\x00")
    for row in rows:
        if not row or str(row[0] or "") in excluded:
            continue
        _hash_field(digest, json.dumps(row, ensure_ascii=False, default=str))
    return digest.hexdigest()


def _target_projection_hash(engine: DiagnosticDuckDBEngine) -> str:
    digest = hashlib.sha256(b"analytix-diagnostic-projection-v2\x00")
    for table in _TARGET_TABLES:
        ordered_columns = _table_columns(engine, table)
        if not ordered_columns:
            _hash_field(digest, f"{table}:absent")
            continue
        _validated_target_columns(engine, table)
        primary_key = _TARGET_PRIMARY_KEYS[table]
        if primary_key not in ordered_columns:
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_schema_invalid")
        count_rows = engine.query(f"SELECT COUNT(1) FROM {table}")
        if not count_rows or len(count_rows[0]) != 1:
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid")
        try:
            row_count = int(count_rows[0][0] or 0)
        except (TypeError, ValueError, OverflowError):
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_invalid") from None
        if row_count < 0 or row_count > _MAX_TARGET_ROWS:
            raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_limit")
        _hash_field(digest, table)
        _hash_field(digest, ",".join(ordered_columns))
        _hash_field(digest, str(row_count))
        for row in _query_rows(
            engine,
            table,
            ordered_columns,
            order_by=primary_key,
            stable_row_identity=False,
        ):
            for column in ordered_columns:
                encoded = json.dumps(
                    row[column],
                    ensure_ascii=False,
                    separators=(",", ":"),
                    sort_keys=True,
                    default=str,
                ).encode("utf-8")
                if len(encoded) > _MAX_JSON_BYTES:
                    raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_projection_limit")
                _hash_bytes(digest, encoded)
    return digest.hexdigest()


def _hash_field(digest: Any, value: str) -> None:
    _hash_bytes(digest, str(value).encode("utf-8"))


def _hash_bytes(digest: Any, value: bytes) -> None:
    digest.update(len(value).to_bytes(8, "big", signed=False))
    digest.update(value)


def _is_sha256(value: object) -> bool:
    return type(value) is str and re.fullmatch(r"[a-f0-9]{64}", value) is not None


def _json_mapping(value: object) -> tuple[dict[str, Any], bool]:
    if isinstance(value, Mapping):
        return dict(value), False
    if type(value) is not str or not value:
        return {}, bool(value)
    if len(value.encode("utf-8", errors="ignore")) > _MAX_JSON_BYTES:
        return {}, True
    try:
        decoded = json.loads(value, object_pairs_hook=_strict_json_object)
    except (TypeError, ValueError, json.JSONDecodeError):
        return {}, True
    if not isinstance(decoded, dict):
        return {}, True
    return decoded, False


def _strict_json_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    output: dict[str, Any] = {}
    for key, value in pairs:
        if key in output:
            raise ValueError("duplicate JSON key")
        output[key] = value
    return output


def _canonical_json(value: Mapping[str, Any]) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True)


def _closed_status(value: object, *, allowed: frozenset[str]) -> str:
    candidate = str(value or "").strip()
    return candidate if candidate in allowed else "unknown"


def _canonical_json_list(value: list[str]) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"))


def _closed_ref_list(
    value: object,
    *,
    case_id: str,
    prefix: str,
) -> tuple[list[str], bool]:
    if type(value) is not str:
        return [], False
    if len(value.encode("utf-8", errors="ignore")) > _MAX_JSON_BYTES:
        return [], False
    try:
        decoded = json.loads(value)
    except (TypeError, ValueError, json.JSONDecodeError):
        return [], False
    if not isinstance(decoded, list):
        return [], False
    expected = re.compile(rf"{re.escape(prefix)}_[a-f0-9]{{64}}")
    refs: list[str] = []
    seen: set[str] = set()
    for item in decoded:
        if type(item) is not str or not item:
            return [], False
        candidate = item if expected.fullmatch(item) else opaque_case_bound_ref(
            prefix=prefix,
            case_id=case_id,
            value=item,
        )
        if candidate not in seen:
            seen.add(candidate)
            refs.append(candidate)
    refs.sort()
    return refs, True


def _strict_opaque_ref_list(
    value: object,
    *,
    expected: re.Pattern[str],
) -> list[str] | None:
    if type(value) is not str or len(value.encode("utf-8", errors="ignore")) > _MAX_JSON_BYTES:
        return None
    try:
        decoded = json.loads(value)
    except (TypeError, ValueError, json.JSONDecodeError):
        return None
    if not isinstance(decoded, list):
        return None
    output: list[str] = []
    seen: set[str] = set()
    for item in decoded:
        if type(item) is not str or expected.fullmatch(item) is None or item in seen:
            return None
        seen.add(item)
        output.append(item)
    return output


def _strict_non_negative_int(value: object) -> int | None:
    if type(value) is not int or value < 0:
        return None
    return value


def _valid_diagnostic_timestamp(value: object, *, required: bool) -> bool:
    if value in (None, ""):
        return not required
    return bool(project_diagnostic_timestamp(value))


def _closed_cleaning_run_ref(*, row_id: int, case_id: str, current: object) -> str:
    candidate = str(current or "").strip()
    if re.fullmatch(r"cleanrun_v[12]_[a-f0-9]{64}", candidate):
        return candidate
    digest = hashlib.sha256(f"{case_id}\x00{row_id}".encode("utf-8")).hexdigest()
    return f"cleanrun_v1_{digest}"


def _required_key(value: object) -> str:
    if type(value) is not str or not value or value != value.strip() or len(value) > 512:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_binding_invalid")
    return value


def _required_file_id(value: object) -> str:
    candidate = _required_key(value)
    if _FILE_ID_RE.fullmatch(candidate) is None:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_binding_invalid")
    return candidate


def _optional_file_id(value: object) -> str:
    candidate = str(value or "").strip()
    if not candidate:
        return ""
    return _required_file_id(candidate)


def _required_document_id(value: object) -> str:
    candidate = _required_key(value)
    if _DOCUMENT_ID_RE.fullmatch(candidate) is None:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_binding_invalid")
    return candidate


def _required_integer_key(value: object) -> int:
    if isinstance(value, bool):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_binding_invalid")
    try:
        candidate = int(value)
    except (TypeError, ValueError, OverflowError):
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_binding_invalid") from None
    if candidate < 0:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_binding_invalid")
    return candidate


def _require_case_binding(value: object, expected_case_id: str) -> None:
    if _required_key(value) != expected_case_id:
        raise DiagnosticDuckDBMigrationError("diagnostic_duckdb_case_binding_mismatch")


def _fault(hook: FaultHook | None, phase: str) -> None:
    if hook is not None:
        hook(phase)


__all__ = [
    "DiagnosticDuckDBMigrationError",
    "assert_duckdb_diagnostic_migration_settled",
    "migrate_legacy_duckdb_diagnostics",
]
