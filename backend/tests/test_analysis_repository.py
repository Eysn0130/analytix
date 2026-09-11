from __future__ import annotations

import pytest

import app.repositories.analysis_repository as analysis_repository_module

from app.core.db_engine import DuckDBEngine
from app.repositories.analysis_repository import (
    AnalysisRepository,
    RULE_PATTERN_RESULT_SIGNATURE_DOMAIN,
    RULE_TXN_RESULT_SIGNATURE_DOMAIN,
    RULE_TXN_SOURCE_SIGNATURE_DOMAIN,
)


def test_rule_txn_prepare_requires_exact_same_case_materializer_receipt(tmp_path, monkeypatch) -> None:
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_DIR", str(tmp_path / "app-data"))
    repo = AnalysisRepository()
    monkeypatch.setattr(repo._storage, "case_db", lambda _case_id: tmp_path / "case.duckdb")

    def complete_result() -> dict:
        return {
            "ok": True,
            "case_id": "case-a",
            "row_count": 3,
            "rebuilt": True,
            "agg_name": "rule_txn_index:v2",
            "agg_version": 2,
        }

    monkeypatch.setattr(
        analysis_repository_module,
        "try_materialize_rule_txn_index",
        lambda **_kwargs: complete_result(),
    )
    monkeypatch.setattr(repo, "_verify_rule_txn_index_clean_binding", lambda case_id: case_id == "case-a")
    assert repo._prepare_rule_txn_index_native("case-a") is True

    monkeypatch.setattr(repo, "_verify_rule_txn_index_clean_binding", lambda _case_id: False)
    assert repo._prepare_rule_txn_index_native("case-a") is False

    def verifier_must_not_run(_case_id: str) -> bool:
        raise AssertionError("invalid materializer receipt reached clean-binding verification")

    monkeypatch.setattr(repo, "_verify_rule_txn_index_clean_binding", verifier_must_not_run)
    for field, value in (
        ("ok", False),
        ("case_id", "case-b"),
        ("row_count", None),
        ("rebuilt", 1),
        ("agg_name", "rule_txn_index:v1"),
        ("agg_version", 1),
    ):
        result = complete_result()
        result[field] = value
        monkeypatch.setattr(
            analysis_repository_module,
            "try_materialize_rule_txn_index",
            lambda result=result, **_kwargs: result,
        )
        assert repo._prepare_rule_txn_index_native("case-a") is False, (field, value)

    result = complete_result()
    result["unexpected"] = 1
    monkeypatch.setattr(
        analysis_repository_module,
        "try_materialize_rule_txn_index",
        lambda **_kwargs: result,
    )
    assert repo._prepare_rule_txn_index_native("case-a") is False


@pytest.mark.parametrize(
    "coverage_rows",
    (
        [(None, None)],
        [(True, True)],
        [("0", "0")],
        [(0.0, 0.0)],
        [(-1, -1)],
        [(0,)],
        [(0, 0, 0)],
        [],
    ),
)
def test_rule_txn_clean_binding_rejects_unknown_coverage_counts(
    tmp_path,
    monkeypatch,
    coverage_rows,
) -> None:
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_DIR", str(tmp_path / "app-data"))
    repo = AnalysisRepository()

    class FakeEngine:
        def __init__(self) -> None:
            self.query_count = 0
            self.closed = False

        def query(self, _sql, _params):
            self.query_count += 1
            return [(0,)] if self.query_count == 1 else coverage_rows

        def close(self) -> None:
            self.closed = True

    engine = FakeEngine()
    monkeypatch.setattr(repo, "open_case_engine", lambda _case_id: engine)
    monkeypatch.setattr(repo, "_table_exists", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(repo, "_table_columns", lambda *_args, **_kwargs: ["id", "clean_acct_no"])
    monkeypatch.setattr(repo, "_txn_cleaning_authority_ready", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(repo, "_rule_txn_index_meta_ready", lambda *_args, **_kwargs: True)

    assert repo._verify_rule_txn_index_clean_binding("case-a") is False
    assert engine.closed is True


def test_rule_txn_clean_binding_accepts_exact_zero_coverage(tmp_path, monkeypatch) -> None:
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_DIR", str(tmp_path / "app-data"))
    repo = AnalysisRepository()

    class FakeEngine:
        def __init__(self) -> None:
            self.query_count = 0

        def query(self, _sql, _params):
            self.query_count += 1
            return [(0,)] if self.query_count == 1 else [(0, 0)]

        def close(self) -> None:
            return None

    engine = FakeEngine()
    monkeypatch.setattr(repo, "open_case_engine", lambda _case_id: engine)
    monkeypatch.setattr(repo, "_table_exists", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(repo, "_table_columns", lambda *_args, **_kwargs: ["id", "clean_acct_no"])
    monkeypatch.setattr(repo, "_txn_cleaning_authority_ready", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(repo, "_rule_txn_index_meta_ready", lambda *_args, **_kwargs: True)

    assert repo._verify_rule_txn_index_clean_binding("case-a") is True


def test_rule_pattern_prepare_requires_complete_current_run_input_coverage(tmp_path, monkeypatch) -> None:
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_DIR", str(tmp_path / "app-data"))
    repo = AnalysisRepository()
    monkeypatch.setattr(repo._storage, "case_db", lambda _case_id: tmp_path / "case.duckdb")
    complete_coverage = {
        "total_rows": 3,
        "accepted_rows": 3,
        "rejected_rows": 0,
        "account_key_covered_rows": 3,
        "transaction_id_covered_rows": 3,
        "transaction_time_covered_rows": 3,
        "amount_covered_rows": 3,
        "direction_covered_rows": 3,
        "cash_covered_rows": 3,
        "cash_unknown_rows": 0,
        "cash_conflict_rows": 0,
    }
    complete_readiness = {
        "cash_dependent_rules": {
            "status": "complete",
            "requested_rows": 3,
            "eligible_rows": 3,
            "unknown_cash_rows": 0,
            "conflict_cash_rows": 0,
            "blocker": None,
        },
        "cash_independent_rules": {
            "status": "complete",
            "requested_rows": 3,
            "eligible_rows": 3,
            "blocker": None,
        },
    }

    def complete_result() -> dict:
        return {
            "ok": True,
            "case_id": "case-a",
            "row_count": 1,
            "rebuilt": True,
            "input_coverage": dict(complete_coverage),
            "feature_readiness": {
                "cash_dependent_rules": dict(complete_readiness["cash_dependent_rules"]),
                "cash_independent_rules": dict(complete_readiness["cash_independent_rules"]),
            },
            "agg_name": "rule_pattern_index:v9",
            "agg_version": 9,
        }

    monkeypatch.setattr(
        analysis_repository_module,
        "try_materialize_rule_pattern_index",
        lambda **_kwargs: complete_result(),
    )
    assert repo._prepare_rule_pattern_index_native("case-a", params={}) is True

    for location, field, value in [
        ("result", "case_id", "case-b"),
        ("result", "row_count", None),
        ("result", "rebuilt", 1),
        ("result", "agg_name", "rule_pattern_index:v8"),
        ("result", "agg_version", 8),
        ("result", "input_coverage", None),
        ("coverage", "total_rows", 0),
        ("coverage", "accepted_rows", 2),
        ("coverage", "rejected_rows", 1),
        ("coverage", "amount_covered_rows", 2),
        ("coverage", "direction_covered_rows", None),
        ("coverage", "cash_covered_rows", 2),
        ("coverage", "cash_unknown_rows", 1),
        ("coverage", "cash_conflict_rows", 1),
        ("result", "feature_readiness", None),
        ("cash_dependent", "status", "partial"),
        ("cash_dependent", "eligible_rows", 2),
        ("cash_dependent", "unknown_cash_rows", 1),
        ("cash_dependent", "blocker", "cash_classification_coverage_incomplete"),
        ("cash_independent", "eligible_rows", 2),
    ]:
        result = complete_result()
        payload = result["input_coverage"]
        readiness = result["feature_readiness"]
        if location == "result":
            result[field] = value
        elif location == "coverage":
            payload[field] = value
        elif location == "cash_dependent":
            readiness["cash_dependent_rules"][field] = value
        else:
            readiness["cash_independent_rules"][field] = value
        monkeypatch.setattr(
            analysis_repository_module,
            "try_materialize_rule_pattern_index",
            lambda result=result, **_kwargs: result,
        )
        assert repo._prepare_rule_pattern_index_native("case-a", params={}) is False, (
            location,
            field,
            value,
        )

    for extra_location in ("result", "coverage", "readiness", "cash_dependent", "cash_independent"):
        result = complete_result()
        payload = result["input_coverage"]
        readiness = result["feature_readiness"]
        target = {
            "result": result,
            "coverage": payload,
            "readiness": readiness,
            "cash_dependent": readiness["cash_dependent_rules"],
            "cash_independent": readiness["cash_independent_rules"],
        }[extra_location]
        target["unexpected"] = 1
        monkeypatch.setattr(
            analysis_repository_module,
            "try_materialize_rule_pattern_index",
            lambda result=result, **_kwargs: result,
        )
        assert repo._prepare_rule_pattern_index_native("case-a", params={}) is False


def test_rule_parameter_updates_validate_complete_batch_before_atomic_write(tmp_path, monkeypatch) -> None:
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_DIR", str(tmp_path / "app-data"))
    repo = AnalysisRepository()
    db_path = tmp_path / "rule-config.duckdb"
    engine = DuckDBEngine(db_path)
    repo.ensure_analysis_schema(engine)
    engine.close()
    monkeypatch.setattr(repo, "case_exists", lambda _case_id: True)
    monkeypatch.setattr(repo, "open_case_engine", lambda _case_id: DuckDBEngine(db_path))

    with pytest.raises(ValueError, match="^rule_parameter_invalid:night_activity_start_hour$"):
        repo.update_rule_params(
            "case-a",
            scope_type="case",
            params={"round_amount_min_amount": 0.0, "night_activity_start_hour": 24},
        )
    engine = DuckDBEngine(db_path)
    assert engine.query("SELECT COUNT(1) FROM analysis_rule_config")[0][0] == 0
    engine.close()

    result = repo.update_rule_params(
        "case-a",
        scope_type="case",
        params={"round_amount_min_amount": 0.0},
    )
    assert result["effective_params"]["round_amount_min_amount"] == 0.0
    engine = DuckDBEngine(db_path)
    assert engine.query(
        "SELECT param_value_json FROM analysis_rule_config WHERE param_key='round_amount_min_amount'"
    ) == [("0.0",)]
    engine.close()


def test_rule_txn_index_loader_excludes_clean_flags(tmp_path, monkeypatch) -> None:
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_DIR", str(tmp_path / "app-data"))
    repo = AnalysisRepository()
    engine = DuckDBEngine(tmp_path / "case.duckdb")
    try:
        engine.execute(
            """CREATE TABLE fc_transaction_norm(
                   id BIGINT,
                   case_id VARCHAR,
                   acct_no VARCHAR,
                   clean_invalid INTEGER DEFAULT 0,
                   clean_failed INTEGER DEFAULT 0,
                   clean_reversal INTEGER DEFAULT 0
               )"""
        )
        engine.execute(
            "CREATE TABLE analysis_revision_state(revision_key VARCHAR, revision BIGINT)"
        )
        engine.execute(
            "INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 1)"
        )
        engine.execute(
            """CREATE TABLE analysis_rule_txn_idx(
                   case_id VARCHAR,
                   account_key VARCHAR,
                   account_name VARCHAR,
                   txn_id VARCHAR,
                   row_id VARCHAR,
                   txn_time VARCHAR,
                   txn_ts_val TIMESTAMP,
                   amount_val DOUBLE,
                   balance_val DOUBLE,
                   direction_raw VARCHAR,
                   success_raw VARCHAR,
                   reason_raw VARCHAR,
                   opener_id_no VARCHAR,
                   ip_addr VARCHAR,
                   mac_addr VARCHAR,
                   counterparty_acct VARCHAR,
                   counterparty_name VARCHAR,
                   counterparty_id_no VARCHAR,
                   counterparty_bank VARCHAR,
                   summary VARCHAR,
                   currency VARCHAR,
                   branch_name VARCHAR,
                   location VARCHAR,
                   cash_raw VARCHAR,
                   voucher_no VARCHAR,
                   receipt_no VARCHAR,
                   log_no VARCHAR,
                   voucher_type VARCHAR,
                   voucher_id VARCHAR,
                   teller_no VARCHAR,
                   remark VARCHAR,
                   txn_type VARCHAR,
                   file_id VARCHAR
               )"""
        )
        engine.executemany(
            "INSERT INTO fc_transaction_norm(id, case_id, acct_no, clean_reversal) VALUES (?, ?, ?, ?)",
            [(1, "case-1", "acct-1", 0), (2, "case-1", "acct-1", 1)],
        )
        engine.executemany(
            """INSERT INTO analysis_rule_txn_idx(
                   case_id, account_key, account_name, txn_id, row_id, txn_time, txn_ts_val,
                   amount_val, balance_val, direction_raw, success_raw, reason_raw,
                   opener_id_no, ip_addr, mac_addr, counterparty_acct, counterparty_name,
                   counterparty_id_no, counterparty_bank, summary, currency, branch_name,
                   location, cash_raw, voucher_no, receipt_no, log_no, voucher_type,
                   voucher_id, teller_no, remark, txn_type, file_id
               ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            [
                (
                    "case-1",
                    "acct-1",
                    "张三",
                    "txn-1",
                    "1",
                    "2026-01-01 10:00:00",
                    "2026-01-01 10:00:00",
                    100.0,
                    1000.0,
                    "进",
                    "",
                    "",
                    "",
                    "",
                    "",
                    "cp-1",
                    "李四",
                    "",
                    "",
                    "转账",
                    "CNY",
                    "",
                    "",
                    "",
                    "",
                    "",
                    "",
                    "",
                    "",
                    "",
                    "",
                    "转账",
                    "file-1",
                ),
            ],
        )
        engine.execute(
            """CREATE TABLE analysis_materialization_meta(
                   agg_name VARCHAR PRIMARY KEY,
                   agg_version INTEGER,
                   case_id VARCHAR,
                   source_revision BIGINT,
                   source_row_count BIGINT,
                   source_signature VARCHAR,
                   result_signature VARCHAR,
                   row_count BIGINT
               )"""
        )
        source_signature = repo._canonical_case_table_signature(
            engine,
            table_name="fc_transaction_norm",
            case_id="case-1",
            domain=RULE_TXN_SOURCE_SIGNATURE_DOMAIN,
            order_by="TRY_CAST(r.id AS BIGINT)",
        )
        result_signature = repo._canonical_case_table_signature(
            engine,
            table_name="analysis_rule_txn_idx",
            case_id="case-1",
            domain=RULE_TXN_RESULT_SIGNATURE_DOMAIN,
            order_by="TRY_CAST(r.row_id AS BIGINT), r.row_id, r.account_key, r.txn_id",
        )
        engine.execute(
            """INSERT INTO analysis_materialization_meta(
                   agg_name, agg_version, case_id, source_revision, source_row_count,
                   source_signature, result_signature, row_count
               ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)""",
            (
                "rule_txn_index:v2",
                2,
                "case-1",
                1,
                2,
                source_signature,
                result_signature,
                1,
            ),
        )

        rows = repo._load_transaction_rows_from_rule_index(engine, "case-1")

        assert [row["txn_id"] for row in rows or []] == ["txn-1"]

        engine.execute(
            "UPDATE fc_transaction_norm SET clean_reversal=0 WHERE case_id='case-1' AND id=2"
        )
        changed_source_signature = repo._canonical_case_table_signature(
            engine,
            table_name="fc_transaction_norm",
            case_id="case-1",
            domain=RULE_TXN_SOURCE_SIGNATURE_DOMAIN,
            order_by="TRY_CAST(r.id AS BIGINT)",
        )
        engine.execute(
            "UPDATE analysis_materialization_meta SET source_signature=? WHERE agg_name='rule_txn_index:v2'",
            (changed_source_signature,),
        )
        assert repo._load_transaction_rows_from_rule_index(engine, "case-1") is None
    finally:
        engine.close()


def test_rule_pattern_loader_rejects_same_count_result_tampering(tmp_path, monkeypatch) -> None:
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_DIR", str(tmp_path / "app-data"))
    repo = AnalysisRepository()
    monkeypatch.setattr(repo, "_rule_txn_index_meta_ready", lambda *_args: True)
    engine = DuckDBEngine(tmp_path / "pattern.duckdb")
    try:
        signature = repo._native_rule_pattern_param_signature({})
        engine.execute(
            """CREATE TABLE analysis_rule_pattern_idx(
                   case_id TEXT,
                   param_signature TEXT,
                   feature_code TEXT,
                   account_key TEXT,
                   direction TEXT,
                   first_time TEXT,
                   last_time TEXT,
                   total_amount DOUBLE
               )"""
        )
        engine.execute(
            "INSERT INTO analysis_rule_pattern_idx VALUES (?, ?, 'SMALL_FAST_EVENT', 'acct-a', '进', '2026-01-01', '2026-01-02', 10)",
            ("case-1", signature),
        )
        result_signature = repo._canonical_case_table_signature(
            engine,
            table_name="analysis_rule_pattern_idx",
            case_id="case-1",
            domain=RULE_PATTERN_RESULT_SIGNATURE_DOMAIN,
            order_by="r.param_signature, r.feature_code, r.account_key, r.direction, r.first_time, r.last_time",
        )
        engine.execute(
            """CREATE TABLE analysis_materialization_meta(
                   agg_name TEXT,
                   agg_version INTEGER,
                   case_id TEXT,
                   source_signature TEXT,
                   source_parameter_signature TEXT,
                   result_signature TEXT,
                   row_count BIGINT
               )"""
        )
        engine.executemany(
            "INSERT INTO analysis_materialization_meta VALUES (?,?,?,?,?,?,?)",
            [
                ("rule_txn_index:v2", 2, "case-1", "b" * 64, "", "a" * 64, 1),
                ("rule_pattern_index:v9", 9, "case-1", "a" * 64, signature, result_signature, 1),
            ],
        )

        loaded = repo._load_native_rule_pattern_features(engine, "case-1", params={})
        assert loaded is not None and loaded["ready"] is True

        engine.execute("UPDATE analysis_rule_pattern_idx SET param_signature='stale-signature'")
        stale_signature = repo._canonical_case_table_signature(
            engine,
            table_name="analysis_rule_pattern_idx",
            case_id="case-1",
            domain=RULE_PATTERN_RESULT_SIGNATURE_DOMAIN,
            order_by="r.param_signature, r.feature_code, r.account_key, r.direction, r.first_time, r.last_time",
        )
        engine.execute(
            "UPDATE analysis_materialization_meta SET result_signature=? WHERE agg_name='rule_pattern_index:v9'",
            (stale_signature,),
        )
        assert repo._load_native_rule_pattern_features(engine, "case-1", params={}) is None

        engine.execute("UPDATE analysis_rule_pattern_idx SET param_signature=?", (signature,))
        restored_signature = repo._canonical_case_table_signature(
            engine,
            table_name="analysis_rule_pattern_idx",
            case_id="case-1",
            domain=RULE_PATTERN_RESULT_SIGNATURE_DOMAIN,
            order_by="r.param_signature, r.feature_code, r.account_key, r.direction, r.first_time, r.last_time",
        )
        engine.execute(
            "UPDATE analysis_materialization_meta SET result_signature=? WHERE agg_name='rule_pattern_index:v9'",
            (restored_signature,),
        )
        engine.execute("UPDATE analysis_rule_pattern_idx SET total_amount=11")
        assert repo._load_native_rule_pattern_features(engine, "case-1", params={}) is None
    finally:
        engine.close()


def test_persist_rule_hits_rebuilds_current_case_without_deleting_other_cases(tmp_path, monkeypatch) -> None:
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_DIR", str(tmp_path / "app-data"))
    repo = AnalysisRepository()
    engine = DuckDBEngine(tmp_path / "case.duckdb")
    try:
        repo.ensure_analysis_schema(engine)
        engine.executemany(
            """
            INSERT INTO analysis_rule_hit(
                rule_hit_id, case_id, rule_code, risk_type, severity, score,
                entity_ids_json, txn_ids_json, evidence_ids_json, summary, detail_json, created_at
            ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
            """,
            [
                ("old-case-1", "case-1", "OLD", "old", "low", 1.0, "[]", "[]", "[]", "old", "{}", "2026-01-01"),
                ("keep-case-2", "case-2", "KEEP", "keep", "high", 9.0, "[]", "[]", "[]", "keep", "{}", "2026-01-01"),
            ],
        )
        engine.executemany(
            """
            INSERT INTO analysis_evidence_ref(
                evidence_id, case_id, evidence_type, ref_table, ref_pk, title, snippet, payload_json, created_at
            ) VALUES (?,?,?,?,?,?,?,?,?)
            """,
            [
                ("ev-old-case-1", "case-1", "rule_hit", "analysis_rule_hit", "old-case-1", "old", "old", "{}", "2026-01-01"),
                ("ev-keep-case-2", "case-2", "rule_hit", "analysis_rule_hit", "keep-case-2", "keep", "keep", "{}", "2026-01-01"),
                ("ev-txn-case-1", "case-1", "txn", "fc_transaction_norm", "txn-1", "txn", "txn", "{}", "2026-01-01"),
            ],
        )

        persisted = repo._persist_rule_hits(
            engine,
            "case-1",
            [],
            [
                {
                    "rule_hit_id": "new-case-1",
                    "rule_code": "FAST_IN_FAST_OUT",
                    "risk_type": "flow",
                    "severity": "high",
                    "score": 88.0,
                    "entity_ids": ["entity-1"],
                    "txn_ids": [],
                    "summary": "new hit",
                    "detail_json": {"reason": "refresh"},
                    "title": "new title",
                }
            ],
        )

        case_1_hits = repo._query_dicts(
            engine,
            "SELECT rule_hit_id FROM analysis_rule_hit WHERE case_id=? ORDER BY rule_hit_id",
            ("case-1",),
        )
        case_2_hits = repo._query_dicts(
            engine,
            "SELECT rule_hit_id FROM analysis_rule_hit WHERE case_id=? ORDER BY rule_hit_id",
            ("case-2",),
        )
        evidence_refs = repo._query_dicts(
            engine,
            """
            SELECT evidence_id, case_id, ref_table, ref_pk
              FROM analysis_evidence_ref
             ORDER BY evidence_id
            """,
        )

        assert [row["rule_hit_id"] for row in case_1_hits] == ["new-case-1"]
        assert [row["rule_hit_id"] for row in case_2_hits] == ["keep-case-2"]
        assert persisted[0]["rule_hit_id"] == "new-case-1"
        assert ("ev-old-case-1", "case-1", "analysis_rule_hit", "old-case-1") not in [
            (row["evidence_id"], row["case_id"], row["ref_table"], row["ref_pk"]) for row in evidence_refs
        ]
        assert ("ev-keep-case-2", "case-2", "analysis_rule_hit", "keep-case-2") in [
            (row["evidence_id"], row["case_id"], row["ref_table"], row["ref_pk"]) for row in evidence_refs
        ]
        assert ("ev-txn-case-1", "case-1", "fc_transaction_norm", "txn-1") in [
            (row["evidence_id"], row["case_id"], row["ref_table"], row["ref_pk"]) for row in evidence_refs
        ]
        assert any(row["case_id"] == "case-1" and row["ref_pk"] == "new-case-1" for row in evidence_refs)
    finally:
        engine.close()


def test_refresh_state_legacy_writer_is_removed() -> None:
    assert not hasattr(AnalysisRepository, "_upsert_refresh_state")
    assert not hasattr(AnalysisRepository, "_refresh_state_row")
