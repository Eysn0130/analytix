from __future__ import annotations

import json

import pytest

from app.core import diagnostic_duckdb_migration as migration_module
from app.core.db_engine import DuckDBEngine
from app.core.diagnostic_duckdb_migration import (
    DiagnosticDuckDBMigrationError,
    assert_duckdb_diagnostic_migration_settled,
    migrate_legacy_duckdb_diagnostics,
)
from app.domain.analysis_workbench_boundary import opaque_case_bound_ref
from app.domain.ordinary_diagnostic_projection import project_query_log_diagnostic
from app.repositories.analysis_revision import (
    ensure_analysis_revision_table,
    get_stats_flow_source_revision,
)


_SENTINEL = (
    "TRACE_SENTINEL 6222020202020202020 /Users/private/input.csv "
    "SELECT * FROM secret_accounts Traceback reasoning-secret"
)
_TARGET_ONLY_SENTINEL = "ANALYTIX_TARGET_ONLY_DIAGNOSTIC_SECRET_9f4433c7b43c"
_DOCUMENT_ID = "fedcba9876543210fedc"


def _open_legacy_database(tmp_path) -> DuckDBEngine:
    engine = DuckDBEngine(tmp_path / "legacy-diagnostics.duckdb")
    engine.execute(
        """CREATE TABLE import_file_log(
            file_id TEXT PRIMARY KEY,
            case_id TEXT,
            filename TEXT,
            display_path TEXT,
            stored_path TEXT,
            status TEXT,
            error TEXT,
            cleaned_status TEXT,
            cleaned_error TEXT
        )"""
    )
    engine.execute(
        "INSERT INTO import_file_log VALUES(?,?,?,?,?,?,?,?,?)",
        (
            "0123456789abcdef0123",
            "case-alpha",
            "evidence.csv",
            "case-source/evidence.csv",
            "controlled-source/evidence.csv",
            _SENTINEL,
            f"{_TARGET_ONLY_SENTINEL}:{_SENTINEL}",
            _SENTINEL,
            _SENTINEL,
        ),
    )
    engine.execute(
        "CREATE TABLE import_file_log_recycle AS SELECT *, ''::TEXT AS recycled_at FROM import_file_log"
    )
    engine.execute(
        """CREATE TABLE cleaning_log(
            id BIGINT PRIMARY KEY,
            case_id TEXT,
            file_id TEXT,
            import_at TEXT,
            cleaned_at TEXT,
            duration_ms BIGINT,
            scope_rows BIGINT,
            summary TEXT,
            run_ref TEXT
        )"""
    )
    engine.execute(
        "INSERT INTO cleaning_log VALUES(?,?,?,?,?,?,?,?,?)",
        (
            1,
            "case-alpha",
            "0123456789abcdef0123",
            _SENTINEL,
            _SENTINEL,
            9,
            2,
            json.dumps({"traceback": _SENTINEL, "account": "6222020202020202020"}),
            _SENTINEL,
        ),
    )
    engine.execute(
        "CREATE TABLE cleaning_log_recycle AS SELECT *, ''::TEXT AS recycled_at FROM cleaning_log"
    )
    engine.execute(
        """CREATE TABLE analysis_query_log(
            query_id TEXT PRIMARY KEY,
            case_id TEXT,
            tool_name TEXT,
            params_json TEXT,
            summary_json TEXT,
            row_count BIGINT,
            duration_ms BIGINT,
            created_at TEXT
        )"""
    )
    engine.execute(
        "INSERT INTO analysis_query_log VALUES(?,?,?,?,?,?,?,?)",
        (
            "query-legacy",
            "case-alpha",
            _SENTINEL,
            json.dumps({"sql": _SENTINEL, "account": "6222020202020202020"}),
            json.dumps({"error": _SENTINEL, "reasoning": "reasoning-secret"}),
            2645472,
            7,
            _SENTINEL,
        ),
    )
    engine.execute(
        """CREATE TABLE analysis_audit_event(
            event_id TEXT PRIMARY KEY,
            case_id TEXT,
            event_type TEXT,
            task_id TEXT,
            run_id TEXT,
            turn_id TEXT,
            artifact_id TEXT,
            approval_id TEXT,
            trace_id TEXT,
            span_id TEXT,
            actor_id TEXT,
            actor_role TEXT,
            tenant_id TEXT,
            request_id TEXT,
            session_id TEXT,
            payload_json TEXT,
            created_at TEXT
        )"""
    )
    engine.execute(
        "INSERT INTO analysis_audit_event VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
        (
            "audit-legacy",
            "case-alpha",
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            json.dumps({"error": _SENTINEL, "reasoning": "reasoning-secret"}),
            _SENTINEL,
        ),
    )
    engine.execute(
        """CREATE TABLE analysis_run_log_event(
            event_id TEXT PRIMARY KEY,
            case_id TEXT,
            task_id TEXT,
            run_id TEXT,
            turn_id TEXT,
            sequence_no BIGINT,
            event_stage TEXT,
            event_type TEXT,
            source TEXT,
            payload_json TEXT,
            created_at TEXT
        )"""
    )
    engine.execute(
        "INSERT INTO analysis_run_log_event VALUES(?,?,?,?,?,?,?,?,?,?,?)",
        (
            "runlog-legacy",
            "case-alpha",
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            1,
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            json.dumps({"error": _SENTINEL, "reasoning": "reasoning-secret"}),
            _SENTINEL,
        ),
    )
    engine.execute(
        """CREATE TABLE analysis_temp_scope(
            scope_id TEXT PRIMARY KEY,
            case_id TEXT,
            scope_type TEXT,
            source_kind TEXT,
            scope_signature TEXT,
            status TEXT,
            source_file_ids_json TEXT,
            document_ids_json TEXT,
            stats_json TEXT,
            audit_json TEXT,
            expires_at TEXT,
            retention_until TEXT,
            last_accessed_at TEXT,
            created_at TEXT,
            updated_at TEXT
        )"""
    )
    engine.execute(
        "INSERT INTO analysis_temp_scope VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
        (
            "scope-legacy",
            "case-alpha",
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            "active",
            json.dumps([_SENTINEL]),
            json.dumps([_SENTINEL]),
            json.dumps({"path": _SENTINEL, "account": "6222020202020202020"}),
            json.dumps({"sql": _SENTINEL, "reasoning": "reasoning-secret"}),
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
            _SENTINEL,
        ),
    )
    engine.execute(
        """CREATE TABLE analysis_revision_state(
            revision_key TEXT PRIMARY KEY,
            revision BIGINT,
            updated_at TEXT,
            reason TEXT
        )"""
    )
    engine.execute(
        "INSERT INTO analysis_revision_state VALUES('stats_flow_source', 7, '', ?)",
        (_SENTINEL,),
    )
    engine.execute(
        """CREATE TABLE document_assets(
            document_id TEXT PRIMARY KEY,
            case_id TEXT,
            filename TEXT,
            stored_path TEXT,
            content_path TEXT,
            sha256 TEXT,
            duplicate_reason TEXT,
            last_error TEXT
        )"""
    )
    engine.execute(
        "INSERT INTO document_assets VALUES(?,?,?,?,?,?,?,?)",
        (
            _DOCUMENT_ID,
            "case-alpha",
            "evidence.csv",
            "controlled-source",
            "controlled-content",
            "f" * 64,
            _SENTINEL,
            _SENTINEL,
        ),
    )
    engine.execute(
        "CREATE TABLE document_assets_recycle AS SELECT *, ''::TEXT AS recycled_at FROM document_assets"
    )
    engine.execute(
        """CREATE TABLE privacy_runtime_state(
            case_id TEXT PRIMARY KEY,
            status TEXT,
            summary TEXT,
            error TEXT,
            salt TEXT
        )"""
    )
    engine.execute(
        "INSERT INTO privacy_runtime_state VALUES('case-alpha', 'failed', ?, ?, 'opaque-salt')",
        (_SENTINEL, _SENTINEL),
    )
    engine.execute(
        """CREATE TABLE analysis_evidence_ref(
            evidence_id TEXT PRIMARY KEY,
            case_id TEXT,
            payload_json TEXT
        )"""
    )
    engine.execute(
        "INSERT INTO analysis_evidence_ref VALUES(?,?,?)",
        ("evidence-1", "case-alpha", json.dumps({"exact_account": "6222020202020202020"})),
    )
    engine.execute(
        """CREATE TABLE fc_transaction_raw(
            txn_id TEXT PRIMARY KEY,
            case_id TEXT,
            account_no TEXT,
            source_payload TEXT
        )"""
    )
    engine.execute(
        "INSERT INTO fc_transaction_raw VALUES('txn-1', 'case-alpha', '6222020202020202020', ?)",
        (_SENTINEL,),
    )
    engine.execute(
        """CREATE TABLE document_chunks(
            chunk_id TEXT PRIMARY KEY,
            case_id TEXT,
            document_id TEXT,
            text TEXT
        )"""
    )
    engine.execute(
        "INSERT INTO document_chunks VALUES('chunk-1', 'case-alpha', ?, ?)",
        (_DOCUMENT_ID, _SENTINEL),
    )
    return engine


def _ordinary_diagnostic_bytes(engine: DuckDBEngine) -> str:
    payload = {
        table: engine.query(f"SELECT * FROM {table} ORDER BY 1")
        for table in (
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
    }
    return json.dumps(payload, ensure_ascii=False, default=str, sort_keys=True)


def _protected_fingerprint(engine: DuckDBEngine) -> str:
    payload = {}
    for table in ("analysis_evidence_ref", "fc_transaction_raw", "document_chunks"):
        columns = engine.query(
            "SELECT column_name, data_type FROM information_schema.columns "
            "WHERE table_schema='main' AND table_name=? ORDER BY ordinal_position",
            (table,),
        )
        rows = engine.query(f"SELECT * FROM {table} ORDER BY 1")
        payload[table] = {"columns": columns, "rows": rows}
    return json.dumps(payload, ensure_ascii=False, default=str, sort_keys=True)


def _assert_closed(value: str) -> None:
    for token in (
        "TRACE_SENTINEL",
        "6222020202020202020",
        "/Users/private/input.csv",
        "SELECT * FROM secret_accounts",
        "Traceback",
        "reasoning-secret",
    ):
        assert token not in value


def _database_generation_contains(tmp_path, token: str) -> bool:
    encoded = token.encode("utf-8")
    return any(encoded in path.read_bytes() for path in tmp_path.iterdir() if path.is_file())


def test_legacy_duckdb_diagnostics_are_physically_migrated_without_touching_evidence(tmp_path) -> None:
    engine = _open_legacy_database(tmp_path)
    engine.execute("CHECKPOINT")
    assert _database_generation_contains(tmp_path, _TARGET_ONLY_SENTINEL)
    protected_before = _protected_fingerprint(engine)

    result = migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")

    assert result["phase"] == "settled"
    assert result["target_count"] == 12
    _assert_closed(_ordinary_diagnostic_bytes(engine))
    assert engine.query(
        "SELECT filename, display_path, stored_path FROM import_file_log"
    ) == [
        (
            "evidence.csv",
            "case-source/evidence.csv",
            "controlled-source/evidence.csv",
        )
    ]
    assert _protected_fingerprint(engine) == protected_before
    journal = engine.query(
        "SELECT migration_id, schema_version, phase, plan_hash, target_count, migrated_row_count "
        "FROM analytix_diagnostic_migration_journal_v2"
    )
    assert len(journal) == 1
    assert journal[0][0:3] == ("ordinary_diagnostic_projection_v2", 2, "settled")
    assert journal[0][4] == 12
    assert _SENTINEL not in json.dumps(journal, ensure_ascii=False)
    assert not _database_generation_contains(tmp_path, _TARGET_ONLY_SENTINEL)
    assert_duckdb_diagnostic_migration_settled(engine, case_id="case-alpha")
    engine.close()


def test_primary_key_migration_pagination_is_stable_and_cannot_settle_raw_rows(
    tmp_path,
    monkeypatch,
) -> None:
    engine = _open_legacy_database(tmp_path)
    for suffix in ("b", "c"):
        engine.execute(
            "INSERT INTO analysis_query_log "
            "SELECT ?, case_id, tool_name, params_json, summary_json, row_count, duration_ms, created_at "
            "FROM analysis_query_log WHERE query_id='query-legacy'",
            (f"query-legacy-{suffix}",),
        )
        engine.execute(
            "INSERT INTO analysis_audit_event "
            "SELECT ?, case_id, event_type, task_id, run_id, turn_id, artifact_id, approval_id, "
            "trace_id, span_id, actor_id, actor_role, tenant_id, request_id, session_id, payload_json, created_at "
            "FROM analysis_audit_event WHERE event_id='audit-legacy'",
            (f"audit-legacy-{suffix}",),
        )
        engine.execute(
            "INSERT INTO analysis_run_log_event "
            "SELECT ?, case_id, task_id, run_id, turn_id, sequence_no, event_stage, event_type, source, "
            "payload_json, created_at FROM analysis_run_log_event WHERE event_id='runlog-legacy'",
            (f"runlog-legacy-{suffix}",),
        )
        engine.execute(
            "INSERT INTO analysis_temp_scope "
            "SELECT ?, case_id, scope_type, source_kind, scope_signature, status, source_file_ids_json, "
            "document_ids_json, stats_json, audit_json, expires_at, retention_until, last_accessed_at, "
            "created_at, updated_at FROM analysis_temp_scope WHERE scope_id='scope-legacy'",
            (f"scope-legacy-{suffix}",),
        )
    monkeypatch.setattr(migration_module, "_HASH_PAGE_ROWS", 2)

    result = migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")

    assert result["phase"] == "settled"
    for table in (
        "analysis_query_log",
        "analysis_audit_event",
        "analysis_run_log_event",
        "analysis_temp_scope",
    ):
        assert engine.query(f"SELECT COUNT(1) FROM {table}") == [(3,)]
    _assert_closed(_ordinary_diagnostic_bytes(engine))
    assert_duckdb_diagnostic_migration_settled(engine, case_id="case-alpha")
    engine.close()


def test_read_gate_rejects_tampered_settled_projection(tmp_path) -> None:
    engine = _open_legacy_database(tmp_path)
    migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")
    engine.execute(
        "UPDATE analysis_query_log SET summary_json=?",
        (json.dumps({"raw": _TARGET_ONLY_SENTINEL}),),
    )

    with pytest.raises(DiagnosticDuckDBMigrationError, match="diagnostic_duckdb_projection_invalid"):
        assert_duckdb_diagnostic_migration_settled(engine, case_id="case-alpha")
    tampered = engine.query("SELECT summary_json FROM analysis_query_log")
    with pytest.raises(DiagnosticDuckDBMigrationError, match="diagnostic_duckdb_projection_invalid"):
        migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")
    assert engine.query("SELECT summary_json FROM analysis_query_log") == tampered

    engine.close()


def test_legitimate_projected_write_does_not_stale_settlement(tmp_path, monkeypatch) -> None:
    engine = _open_legacy_database(tmp_path)
    migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")
    engine.execute(
        "INSERT INTO import_file_log VALUES(?,?,?,?,?,?,?,?,?)",
        (
            "abcdef0123456789abcd",
            "case-alpha",
            "source_name_withheld",
            "case-source/6222020202020202020.csv",
            "controlled-source/6222020202020202020.csv",
            "succeeded",
            "",
            "done",
            "",
        ),
    )
    monkeypatch.setattr(
        engine,
        "replace_with_compacted_generation",
        lambda: pytest.fail("settled projected writes must not rebuild the database generation"),
    )

    assert_duckdb_diagnostic_migration_settled(engine, case_id="case-alpha")
    assert migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")[
        "migrated_row_count"
    ] == 0

    engine.close()


def test_current_writer_json_timestamp_and_active_temp_scope_survive_read_gate(
    tmp_path,
    monkeypatch,
) -> None:
    engine = _open_legacy_database(tmp_path)
    migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")
    engine.execute("ALTER TABLE analysis_temp_scope ADD COLUMN source_revision BIGINT")

    query_seed = "current-query"
    params, summary = project_query_log_diagnostic(
        case_id="case-alpha",
        query_id=query_seed,
        params={"sql_digest": "a" * 64},
        summary={
            "execution_status": "diagnosed",
            "diagnostic_mode": "validate",
            "error_class": "none",
        },
    )
    query_id = summary["query_ref"]
    engine.execute(
        "INSERT INTO analysis_query_log VALUES(?,?,?,?,?,?,?,?)",
        (
            query_id,
            "case-alpha",
            "diagnose_case_sql",
            json.dumps(params, ensure_ascii=False),
            json.dumps(summary, ensure_ascii=False),
            None,
            3,
            "2026-07-16 12:00:00",
        ),
    )

    scope_signature = "temp_scope_v2_0123456789abcdef"
    scope_id = opaque_case_bound_ref(
        prefix="temp_scope_v1",
        case_id="case-alpha",
        value=scope_signature,
    )
    source_ref = opaque_case_bound_ref(
        prefix="tempsrc_v1",
        case_id="case-alpha",
        value="temp_fc_source",
    )
    document_ref = opaque_case_bound_ref(
        prefix="tempdoc_v1",
        case_id="case-alpha",
        value="document-source",
    )
    updated_at = "2026-07-16 12:00:00"
    expires_at = "2026-07-17 12:00:00"
    retention_until = "2026-08-15 12:00:00"
    stats = {
        "contract": "TempScopeOperationalStatsV2",
        "requested_source_count": 2,
        "resolved_source_count": 1,
        "failure_count": 1,
        "failures": [
            {
                "source_ref": source_ref,
                "failure_code": "source_path_unavailable",
                "retryable": True,
                "fact_answer_allowed": False,
            }
        ],
        "lifecycle_warning_codes": [],
        "fact_answer_allowed": False,
        "raw_details_exposed": False,
        "lifecycle": {
            "status": "active",
            "ttl_hours": 24,
            "expires_at": expires_at,
            "retention_until": retention_until,
            "active_limit": 16,
        },
    }
    audit = {
        "contract": "TempScopeAuditV2",
        "status": "active",
        "requested_source_count": 2,
        "resolved_source_count": 1,
        "failure_count": 1,
        "cleanup_policy": {
            "ttl_hours": 24,
            "audit_retention_days": 30,
            "active_limit": 16,
        },
        "events": [{"action": "created_or_updated", "at": updated_at}],
        "restricted_details_withheld": True,
    }
    engine.execute(
        """INSERT INTO analysis_temp_scope(
            scope_id, case_id, scope_type, source_kind, scope_signature, status,
            source_file_ids_json, document_ids_json, stats_json, audit_json,
            expires_at, retention_until, last_accessed_at, created_at, updated_at,
            source_revision
        ) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)""",
        (
            scope_id,
            "case-alpha",
            "file_scope",
            "mixed_upload",
            scope_signature,
            "active",
            json.dumps([source_ref], ensure_ascii=False),
            json.dumps([document_ref], ensure_ascii=False),
            json.dumps(stats, ensure_ascii=False),
            json.dumps(audit, ensure_ascii=False),
            expires_at,
            retention_until,
            updated_at,
            updated_at,
            updated_at,
            7,
        ),
    )
    current_before = engine.query(
        "SELECT status, source_kind, stats_json, audit_json FROM analysis_temp_scope WHERE scope_id=?",
        (scope_id,),
    )
    monkeypatch.setattr(
        engine,
        "replace_with_compacted_generation",
        lambda: pytest.fail("current V2 writes must not rebuild the database generation"),
    )

    assert_duckdb_diagnostic_migration_settled(engine, case_id="case-alpha")
    assert migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")[
        "migrated_row_count"
    ] == 0
    assert engine.query(
        "SELECT status, source_kind, stats_json, audit_json FROM analysis_temp_scope WHERE scope_id=?",
        (scope_id,),
    ) == current_before
    engine.close()


def test_legacy_missing_temp_scope_bindings_stay_unknown_not_zero(tmp_path) -> None:
    engine = _open_legacy_database(tmp_path)
    engine.execute(
        "UPDATE analysis_temp_scope SET source_file_ids_json=NULL, document_ids_json=NULL"
    )

    migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")

    stats_json = engine.query("SELECT stats_json FROM analysis_temp_scope")[0][0]
    assert json.loads(stats_json)["requested_source_count"] is None
    assert_duckdb_diagnostic_migration_settled(engine, case_id="case-alpha")
    engine.close()


def test_current_timestamp_revision_row_survives_migration_and_read_gate(tmp_path) -> None:
    engine = DuckDBEngine(tmp_path / "current-revision.duckdb")
    ensure_analysis_revision_table(engine)
    assert get_stats_flow_source_revision(engine) == 1
    before = engine.query(
        "SELECT revision_key, revision, updated_at, reason FROM analysis_revision_state"
    )
    assert before[0][2] is not None

    migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")

    assert engine.query(
        "SELECT revision_key, revision, updated_at, reason FROM analysis_revision_state"
    ) == before
    assert_duckdb_diagnostic_migration_settled(engine, case_id="case-alpha")
    engine.close()


def test_migration_rolls_back_data_transaction_then_resumes_from_planned_journal(tmp_path) -> None:
    engine = _open_legacy_database(tmp_path)
    before = _ordinary_diagnostic_bytes(engine)
    protected_before = _protected_fingerprint(engine)

    def crash(phase: str) -> None:
        if phase == "before_commit":
            raise RuntimeError("simulated crash")

    with pytest.raises(DiagnosticDuckDBMigrationError, match="diagnostic_duckdb_migration_failed"):
        migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha", fault_hook=crash)

    assert _ordinary_diagnostic_bytes(engine) == before
    assert engine.query("SELECT phase FROM analytix_diagnostic_migration_journal_v2") == [("planned",)]
    assert _protected_fingerprint(engine) == protected_before

    migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")
    _assert_closed(_ordinary_diagnostic_bytes(engine))
    assert engine.query("SELECT phase FROM analytix_diagnostic_migration_journal_v2") == [("settled",)]
    assert _protected_fingerprint(engine) == protected_before
    engine.close()


def test_crash_after_applied_commit_is_idempotently_recovered(tmp_path) -> None:
    engine = _open_legacy_database(tmp_path)
    protected_before = _protected_fingerprint(engine)

    def crash(phase: str) -> None:
        if phase == "applied_committed":
            raise RuntimeError("simulated process loss after commit")

    with pytest.raises(RuntimeError, match="simulated process loss after commit"):
        migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha", fault_hook=crash)

    first_generation = _ordinary_diagnostic_bytes(engine)
    _assert_closed(first_generation)
    assert engine.query("SELECT phase FROM analytix_diagnostic_migration_journal_v2") == [("applied",)]
    applied_count = engine.query(
        "SELECT migrated_row_count FROM analytix_diagnostic_migration_journal_v2"
    )[0][0]

    second = migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")
    assert applied_count > 0
    assert second["migrated_row_count"] == applied_count
    assert _ordinary_diagnostic_bytes(engine) == first_generation
    assert _protected_fingerprint(engine) == protected_before
    engine.close()


def test_planned_recovery_never_rebaselines_changed_source_rows(tmp_path) -> None:
    engine = _open_legacy_database(tmp_path)

    def crash(phase: str) -> None:
        if phase == "before_commit":
            raise RuntimeError("simulated crash")

    with pytest.raises(DiagnosticDuckDBMigrationError, match="diagnostic_duckdb_migration_failed"):
        migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha", fault_hook=crash)

    journal_before = engine.query("SELECT * FROM analytix_diagnostic_migration_journal_v2")
    engine.execute(
        "UPDATE analysis_query_log SET summary_json=?",
        (json.dumps({"changed_after_plan": True}),),
    )
    changed_source = engine.query("SELECT summary_json FROM analysis_query_log")

    with pytest.raises(DiagnosticDuckDBMigrationError, match="diagnostic_duckdb_recovery_mismatch"):
        migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")

    assert engine.query("SELECT * FROM analytix_diagnostic_migration_journal_v2") == journal_before
    assert engine.query("SELECT summary_json FROM analysis_query_log") == changed_source
    engine.close()


def test_corrupt_or_drifted_journal_fails_closed_without_changing_diagnostics(tmp_path) -> None:
    engine = _open_legacy_database(tmp_path)
    migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")
    closed_before = _ordinary_diagnostic_bytes(engine)
    engine.execute(
        "UPDATE analytix_diagnostic_migration_journal_v2 SET plan_hash='unexpected-plan'"
    )

    with pytest.raises(DiagnosticDuckDBMigrationError, match="diagnostic_duckdb_journal_invalid"):
        migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")

    assert _ordinary_diagnostic_bytes(engine) == closed_before
    engine.close()


def test_unrecognized_target_column_fails_closed_without_partial_scrub(tmp_path) -> None:
    engine = _open_legacy_database(tmp_path)
    engine.execute("ALTER TABLE import_file_log ADD COLUMN raw_traceback TEXT")
    engine.execute("UPDATE import_file_log SET raw_traceback=?", (_SENTINEL,))
    before = _ordinary_diagnostic_bytes(engine)
    protected_before = _protected_fingerprint(engine)

    with pytest.raises(DiagnosticDuckDBMigrationError, match="diagnostic_duckdb_schema_invalid"):
        migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")

    assert _ordinary_diagnostic_bytes(engine) == before
    assert _protected_fingerprint(engine) == protected_before
    assert engine.query(
        "SELECT COUNT(1) FROM information_schema.tables "
        "WHERE table_schema='main' AND table_name='analytix_diagnostic_migration_journal_v2'"
    ) == [(0,)]
    engine.close()


def test_settled_table_type_drift_fails_closed_without_reprojection(tmp_path) -> None:
    engine = _open_legacy_database(tmp_path)
    migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")
    engine.execute("ALTER TABLE analysis_query_log ALTER COLUMN row_count TYPE VARCHAR")
    before = engine.query("SELECT * FROM analysis_query_log")

    with pytest.raises(DiagnosticDuckDBMigrationError, match="diagnostic_duckdb_schema_invalid"):
        assert_duckdb_diagnostic_migration_settled(engine, case_id="case-alpha")
    with pytest.raises(DiagnosticDuckDBMigrationError, match="diagnostic_duckdb_schema_invalid"):
        migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")

    assert engine.query("SELECT * FROM analysis_query_log") == before
    engine.close()


@pytest.mark.parametrize(
    "create_sql",
    [
        "CREATE TABLE analysis_query_log(query_id TEXT, case_id TEXT)",
        """CREATE TABLE analysis_query_log(
            query_id TEXT,
            case_id TEXT,
            tool_name TEXT,
            params_json TEXT,
            summary_json TEXT,
            row_count BIGINT,
            duration_ms BIGINT,
            created_at TEXT
        )""",
    ],
    ids=["partial_schema", "missing_primary_key"],
)
def test_unsupported_or_keyless_target_schema_never_settles(tmp_path, create_sql) -> None:
    engine = DuckDBEngine(tmp_path / "unsupported-target-schema.duckdb")
    engine.execute(create_sql)

    with pytest.raises(DiagnosticDuckDBMigrationError, match="diagnostic_duckdb_schema_invalid"):
        migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")

    assert engine.query(
        "SELECT COUNT(1) FROM information_schema.tables "
        "WHERE table_schema='main' AND table_name='analytix_diagnostic_migration_journal_v2'"
    ) == [(0,)]
    engine.close()


def test_malformed_current_temp_scope_marker_is_not_laundered_as_legacy(tmp_path) -> None:
    engine = _open_legacy_database(tmp_path)
    engine.execute(
        "UPDATE analysis_temp_scope SET scope_signature='temp_scope_v2_0123456789abcdef'"
    )
    before = engine.query("SELECT * FROM analysis_temp_scope")

    with pytest.raises(DiagnosticDuckDBMigrationError, match="diagnostic_duckdb_projection_invalid"):
        migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")

    assert engine.query("SELECT * FROM analysis_temp_scope") == before
    engine.close()


def test_populated_unknown_cleaning_detail_schema_blocks_readiness(tmp_path) -> None:
    engine = _open_legacy_database(tmp_path)
    engine.execute("CREATE TABLE cleaning_log_detail(id BIGINT, raw_message TEXT)")
    engine.execute("INSERT INTO cleaning_log_detail VALUES(1, ?)", (_SENTINEL,))
    before = _ordinary_diagnostic_bytes(engine)
    protected_before = _protected_fingerprint(engine)

    with pytest.raises(DiagnosticDuckDBMigrationError, match="diagnostic_duckdb_unknown_schema"):
        migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")

    assert _ordinary_diagnostic_bytes(engine) == before
    assert engine.query("SELECT * FROM cleaning_log_detail") == [(1, _SENTINEL)]
    assert _protected_fingerprint(engine) == protected_before
    engine.close()


def test_row_limit_is_checked_before_target_rows_are_loaded(tmp_path, monkeypatch) -> None:
    engine = _open_legacy_database(tmp_path)
    engine.execute(
        "INSERT INTO analysis_query_log VALUES(?,?,?,?,?,?,?,?)",
        (
            "query-legacy-2",
            "case-alpha",
            _SENTINEL,
            json.dumps({"sql": _SENTINEL}),
            json.dumps({"error": _SENTINEL}),
            1,
            1,
            _SENTINEL,
        ),
    )
    before = _ordinary_diagnostic_bytes(engine)
    monkeypatch.setattr(migration_module, "_MAX_TARGET_ROWS", 1)

    with pytest.raises(DiagnosticDuckDBMigrationError, match="diagnostic_duckdb_projection_limit"):
        migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")

    assert _ordinary_diagnostic_bytes(engine) == before
    assert engine.query(
        "SELECT COUNT(1) FROM information_schema.tables "
        "WHERE table_schema='main' AND table_name='analytix_diagnostic_migration_journal_v2'"
    ) == [(0,)]
    engine.close()


def test_cross_case_ordinary_row_blocks_migration_without_touching_evidence(tmp_path) -> None:
    engine = _open_legacy_database(tmp_path)
    engine.execute("UPDATE analysis_query_log SET case_id='case-bravo'")
    before = _ordinary_diagnostic_bytes(engine)
    protected_before = _protected_fingerprint(engine)

    with pytest.raises(
        DiagnosticDuckDBMigrationError,
        match="diagnostic_duckdb_case_binding_mismatch",
    ):
        migrate_legacy_duckdb_diagnostics(engine, case_id="case-alpha")

    assert _ordinary_diagnostic_bytes(engine) == before
    assert _protected_fingerprint(engine) == protected_before
    engine.close()
