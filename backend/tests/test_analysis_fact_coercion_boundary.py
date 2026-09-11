from __future__ import annotations

from math import inf, nan
from typing import Any

import pytest

from app.repositories.analysis_repository import (
    RULE_PARAM_DEFAULTS,
    AnalysisRepository,
    TransactionFactSourceUnavailableError,
    _as_float,
    _as_int,
    _first_finite_number_sql,
    _normalize_rule_params,
    _reconcile_count_status,
    _rule_param_scope_overrides,
    _same_fact_txn_key_sql,
    _same_fact_txn_eligibility_sql,
    _same_fact_txn_rejection_reason_sql,
    _summarize_field_quality,
)
from app.core.db_engine import DuckDBEngine
from app.repositories.txn_daily_aggregate import TxnAmountCoverageV1


HOSTILE_FACT_VALUES = (None, "", True, {}, [], nan, inf, -inf)


def _repository() -> AnalysisRepository:
    return object.__new__(AnalysisRepository)


@pytest.mark.parametrize("value", HOSTILE_FACT_VALUES)
def test_global_numeric_coercers_fail_closed_instead_of_returning_zero(value: object) -> None:
    with pytest.raises(ValueError, match="analysis_integer_unknown"):
        _as_int(value)
    with pytest.raises(ValueError, match="analysis_number_unknown"):
        _as_float(value)


def test_global_numeric_coercers_preserve_observed_zero() -> None:
    assert _as_int(0) == 0
    assert _as_float(0) == 0


@pytest.mark.parametrize("value", [nan, inf, -inf])
@pytest.mark.parametrize(
    ("method_name", "base_kwargs", "threshold_field"),
    [
        ("resolve_duplicate_families", {}, "min_amount"),
        ("resolve_owner_scope", {}, "candidate_min_turnover"),
        ("trace_subject_top_outflows", {}, "min_amount"),
        ("trace_subject_top_outflows", {}, "candidate_min_turnover"),
        ("trace_subject_top_outflows", {}, "time_window_hours"),
        ("trace_subject_top_outflows", {}, "amount_tolerance_ratio"),
        ("trace_subject_top_outflows", {}, "amount_tolerance_abs"),
        ("classify_missing_counterparty_business", {}, "min_amount"),
        ("classify_missing_counterparty_business", {}, "candidate_min_turnover"),
        ("get_materialized_next_hop_candidates", {}, "tolerance_rate"),
        (
            "create_trace_run_shell",
            {"seed_type": "account", "seed_value": "acct-a", "depth": 1, "time_window_sec": 60},
            "tolerance_rate",
        ),
        (
            "run_trace",
            {"seed_type": "account", "seed_value": "acct-a", "depth": 1, "time_window_sec": 60},
            "tolerance_rate",
        ),
    ],
)
def test_external_amount_thresholds_reject_nonfinite_values_before_database_access(
    value: float,
    method_name: str,
    base_kwargs: dict[str, Any],
    threshold_field: str,
) -> None:
    repository = _repository()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: None  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("nonfinite threshold reached database access")
    )
    kwargs = {**base_kwargs, threshold_field: value}

    with pytest.raises(ValueError, match="analysis_number_unknown"):
        getattr(repository, method_name)("case-a", **kwargs)


def test_same_fact_key_rejects_missing_and_preserves_observed_zero(tmp_path) -> None:
    engine = DuckDBEngine(tmp_path / "same-fact.duckdb")
    try:
        rows = engine.query(
            f"""
            WITH source(
                txn_id, id, txn_ts, dc_val, amount, balance, cp_key,
                cp_name, cp_raw, summary, remark, txn_type, is_success
            ) AS (
                VALUES
                  ('txn-1', 1, TIMESTAMP '2026-07-20 10:00:00', '进', NULL, NULL, 'cp', '甲', '', '摘要', '备注', '转账', TRUE),
                  ('txn-1', 2, TIMESTAMP '2026-07-20 10:00:00', '进', 0.0, 0.0, 'cp', '甲', '', '摘要', '备注', '转账', TRUE)
            )
            SELECT {_same_fact_txn_key_sql()} AS fact_key
            FROM source
            ORDER BY id
            """
        )
    finally:
        engine.close()

    assert len(rows) == 2
    assert rows[0][0] is None
    assert isinstance(rows[1][0], str)
    assert "|amt:0.0|bal:0.0" in rows[1][0]


def test_same_fact_key_rejects_nonfinite_values(tmp_path) -> None:
    engine = DuckDBEngine(tmp_path / "same-fact-nonfinite.duckdb")
    try:
        rows = engine.query(
            f"""
            WITH source(
                txn_id, id, txn_ts, dc_val, amount, balance, cp_key,
                cp_name, cp_raw, summary, remark, txn_type, is_success
            ) AS (
                VALUES
                  ('txn-1', 1, TIMESTAMP '2026-07-20 10:00:00', '进', 'NaN'::DOUBLE, 1.0, 'cp', '甲', '', '摘要', '备注', '转账', TRUE),
                  ('txn-1', 2, TIMESTAMP '2026-07-20 10:00:00', '进', 'NaN'::DOUBLE, 1.0, 'cp', '甲', '', '摘要', '备注', '转账', TRUE),
                  ('txn-1', 3, TIMESTAMP '2026-07-20 10:00:00', '进', 'Inf'::DOUBLE, 1.0, 'cp', '甲', '', '摘要', '备注', '转账', TRUE)
            )
            SELECT {_same_fact_txn_key_sql()} AS fact_key
            FROM source
            ORDER BY id
            """
        )
    finally:
        engine.close()

    assert [row[0] for row in rows] == [None, None, None]


@pytest.mark.parametrize(
    "overrides",
    [
        {"txn_id": "NULL"},
        {"id": "NULL"},
        {"txn_ts": "NULL"},
        {"dc_val": "'未知'"},
        {"amount": "NULL"},
        {"amount": "'NaN'::DOUBLE"},
        {"amount": "1.001"},
        {"balance": "NULL"},
        {"balance": "'Inf'::DOUBLE"},
        {"balance": "1.001"},
        {"cp_key": "''", "cp_name": "''", "cp_raw": "''"},
        {"summary": "''"},
        {"remark": "''"},
        {"txn_type": "''"},
        {"is_success": "'unknown'"},
    ],
)
def test_same_fact_key_rejects_every_incomplete_required_component(
    tmp_path,
    overrides: dict[str, str],
) -> None:
    values = {
        "txn_id": "'txn-1'",
        "id": "1",
        "txn_ts": "TIMESTAMP '2026-07-20 10:00:00'",
        "dc_val": "'出'",
        "amount": "0.0",
        "balance": "0.0",
        "cp_key": "'cp-1'",
        "cp_name": "'乙方'",
        "cp_raw": "''",
        "summary": "'摘要'",
        "remark": "'备注'",
        "txn_type": "'转账'",
        "is_success": "'true'",
    }
    values.update(overrides)
    projection = ", ".join(f"{value} AS {column}" for column, value in values.items())
    engine = DuckDBEngine(tmp_path / "same-fact-required-component.duckdb")
    try:
        rows = engine.query(
            f"SELECT {_same_fact_txn_key_sql()} AS fact_key, {_same_fact_txn_eligibility_sql()} AS eligible FROM (SELECT {projection}) source"
        )
    finally:
        engine.close()

    assert rows == [(None, False)]


@pytest.mark.parametrize(
    ("override", "reason"),
    [
        ({"txn_id": "'unknown'"}, "transaction_id_missing_or_placeholder"),
        ({"id": "NULL"}, "row_identity_missing_or_placeholder"),
        ({"txn_ts": "NULL"}, "transaction_time_missing_or_invalid"),
        ({"dc_val": "NULL"}, "direction_missing_or_invalid"),
        ({"amount": "'NaN'::DOUBLE"}, "amount_missing_or_invalid"),
        ({"amount": "1.001"}, "amount_precision_unsupported"),
        ({"balance": "NULL"}, "balance_missing_or_invalid"),
        ({"balance": "1.001"}, "balance_precision_unsupported"),
        ({"cp_key": "''", "cp_name": "''", "cp_raw": "''"}, "counterparty_missing_or_placeholder"),
        ({"summary": "''"}, "summary_missing_or_placeholder"),
        ({"remark": "''"}, "remark_missing_or_placeholder"),
        ({"txn_type": "''"}, "transaction_type_missing_or_placeholder"),
        ({"is_success": "'unknown'"}, "success_status_missing_or_unrecognized"),
    ],
)
def test_same_fact_rejection_reason_is_deterministic(
    tmp_path,
    override: dict[str, str],
    reason: str,
) -> None:
    values = {
        "txn_id": "'txn-1'",
        "id": "1",
        "txn_ts": "TIMESTAMP '2026-07-20 10:00:00'",
        "dc_val": "'出'",
        "amount": "0.0",
        "balance": "0.0",
        "cp_key": "'cp-1'",
        "cp_name": "'乙方'",
        "cp_raw": "''",
        "summary": "'摘要'",
        "remark": "'备注'",
        "txn_type": "'转账'",
        "is_success": "'true'",
    }
    values.update(override)
    projection = ", ".join(f"{value} AS {column}" for column, value in values.items())
    engine = DuckDBEngine(tmp_path / "same-fact-rejection-reason.duckdb")
    try:
        rows = engine.query(
            f"SELECT {_same_fact_txn_rejection_reason_sql()} AS rejection_reason FROM (SELECT {projection}) source"
        )
    finally:
        engine.close()

    assert rows == [(reason,)]


class _NoopDailyAggregate:
    def ensure_materialized(self, *_args, **_kwargs) -> bool:
        return True


def _same_fact_rank_row(
    *,
    row_id: int = 1,
    account_key: str = "acct-a",
    txn_id: str = "txn-1",
    amount: float | None = 100.0,
    balance: float | None = 900.0,
) -> tuple[object, ...]:
    return (
        row_id,
        account_key,
        "cp-1",
        "乙方",
        "乙方原始",
        txn_id,
        "2026-07-20 10:00:00",
        "出",
        amount,
        balance,
        "交易摘要",
        "交易备注",
        "转账",
        "true",
        "甲方",
        "支行",
        "地点",
        "127.0.0.1",
        "00:11:22:33:44:55",
        "0",
        "file-1",
    )


def _same_fact_rank_engine(tmp_path, rows: list[tuple[object, ...]]) -> DuckDBEngine:
    engine = DuckDBEngine(tmp_path / "same-fact-rank.duckdb")
    engine.execute(
        """
        CREATE TABLE analysis_txn_detail_idx (
          id BIGINT,
          acct_key VARCHAR,
          cp_key VARCHAR,
          cp_name VARCHAR,
          cp_raw VARCHAR,
          txn_id VARCHAR,
          txn_ts TIMESTAMP,
          dc_val VARCHAR,
          amount DOUBLE,
          balance DOUBLE,
          summary VARCHAR,
          remark VARCHAR,
          txn_type VARCHAR,
          is_success VARCHAR,
          account_open_name VARCHAR,
          branch_name VARCHAR,
          location VARCHAR,
          ip_addr VARCHAR,
          mac_addr VARCHAR,
          cash_flag VARCHAR,
          file_id VARCHAR
        )
        """
    )
    if rows:
        engine.executemany(
            "INSERT INTO analysis_txn_detail_idx VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
            rows,
        )
    return engine


def _same_fact_rank_repository(engine: DuckDBEngine) -> AnalysisRepository:
    repository = _repository()
    repository.case_exists = lambda case_id: case_id == "case-a"  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: None  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: engine  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._cleanup_temp_scope_lifecycle = lambda *_args, **_kwargs: []  # type: ignore[method-assign]
    repository._daily_agg = _NoopDailyAggregate()
    repository._resolve_rank_scope_accounts = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "scope_type": "holder",
        "scope_requested": True,
        "selected_accounts": ["acct-a", "acct-b"],
        "warnings": [],
    }
    repository._rank_scope_where = lambda *_args, **_kwargs: ("1=1", [])  # type: ignore[method-assign]
    repository._append_query_log = lambda *_args, **_kwargs: "query-1"  # type: ignore[method-assign]
    return repository


def _run_same_fact_rank(repository: AnalysisRepository) -> dict[str, Any]:
    return repository.rank_counterparties(
        "case-a",
        holder_name="甲方",
        metric="outflow",
        dedupe_same_holder_same_fact=True,
        counterparty_group_mode="name",
    )


@pytest.mark.parametrize(
    ("rows", "eligible", "distinct_ids"),
    [
        (
            [
                _same_fact_rank_row(row_id=1, account_key="acct-a", balance=None),
                _same_fact_rank_row(row_id=2, account_key="acct-b", balance=None),
            ],
            0,
            2,
        ),
        (
            [
                _same_fact_rank_row(row_id=1, account_key="acct-a"),
                _same_fact_rank_row(row_id=2, account_key="acct-b", balance=None),
            ],
            1,
            2,
        ),
        (
            [
                _same_fact_rank_row(row_id=1, account_key="acct-a"),
                _same_fact_rank_row(row_id=1, account_key="acct-b"),
            ],
            2,
            1,
        ),
    ],
)
def test_rank_counterparties_dedupe_blocks_incomplete_or_nonunique_key_coverage(
    tmp_path,
    rows: list[tuple[object, ...]],
    eligible: int,
    distinct_ids: int,
) -> None:
    engine = _same_fact_rank_engine(tmp_path, rows)
    result = _run_same_fact_rank(_same_fact_rank_repository(engine))

    assert result["coverage_status"] == "partial"
    assert result["blocker"] == "same_fact_dedupe_key_coverage_incomplete"
    assert result["rankings"] == []
    assert result["query_id"] == ""
    assert result["group_summary"]["dedupe_candidate_duplicate_amount"] is None
    assert result["same_fact_dedupe"] == {
        "contract": "SameFactDedupeCoverageV1",
        "key_version": "analytix.same-fact-dedupe-key/v2",
        "requested": True,
        "scope_authorized": True,
        "applied": False,
        "coverage_status": "partial",
        "requested_txn_count": 2,
        "eligible_txn_count": eligible,
        "ineligible_txn_count": 2 - eligible,
        "distinct_row_id_count": distinct_ids,
        "raw_txn_count": None,
        "effective_txn_count": None,
        "duplicate_row_count": None,
        "candidate_duplicate_amount": None,
        "blocker": "same_fact_dedupe_key_coverage_incomplete",
    }
    assert all(
        warning.get("code") != "RANK_COUNTERPARTIES_SAME_HOLDER_FACT_DEDUPED"
        for warning in result["warnings"]
    )


def test_rank_counterparties_empty_scope_is_not_verified_no_hit(tmp_path) -> None:
    engine = _same_fact_rank_engine(tmp_path, [])
    result = _run_same_fact_rank(_same_fact_rank_repository(engine))

    assert result["coverage_status"] == "empty_unverified"
    assert result["blocker"] == "same_fact_dedupe_scope_empty_unverified"
    assert result["rankings"] == []
    assert result["query_id"] == ""
    assert result["same_fact_dedupe"]["requested_txn_count"] == 0
    assert result["same_fact_dedupe"]["candidate_duplicate_amount"] is None


def test_rank_counterparties_dedupe_never_silently_falls_back_without_holder_scope(tmp_path) -> None:
    engine = _same_fact_rank_engine(tmp_path, [_same_fact_rank_row()])
    repository = _same_fact_rank_repository(engine)
    repository._resolve_rank_scope_accounts = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "scope_type": "case",
        "scope_requested": False,
        "selected_accounts": [],
        "warnings": [],
    }

    result = _run_same_fact_rank(repository)

    assert result["coverage_status"] == "blocked"
    assert result["blocker"] == "same_fact_dedupe_scope_not_authorized"
    assert result["rankings"] == []
    assert result["same_fact_dedupe"]["scope_authorized"] is False
    assert result["same_fact_dedupe"]["applied"] is False


def test_rank_counterparties_malformed_dedupe_preflight_fails_closed(tmp_path) -> None:
    engine = _same_fact_rank_engine(tmp_path, [_same_fact_rank_row()])
    repository = _same_fact_rank_repository(engine)
    repository._same_fact_dedupe_preflight = lambda *_args, **_kwargs: None  # type: ignore[method-assign]

    result = _run_same_fact_rank(repository)

    assert result["coverage_status"] == "unresolved"
    assert result["blocker"] == "same_fact_dedupe_preflight_unavailable"
    assert result["rankings"] == []
    assert result["query_id"] == ""


def test_rank_counterparties_complete_duplicate_preserves_real_zero(tmp_path) -> None:
    engine = _same_fact_rank_engine(
        tmp_path,
        [
            _same_fact_rank_row(row_id=1, account_key="acct-a", amount=0.0, balance=0.0),
            _same_fact_rank_row(row_id=2, account_key="acct-b", amount=0.0, balance=0.0),
        ],
    )
    result = _run_same_fact_rank(_same_fact_rank_repository(engine))

    assert result["coverage_status"] == "complete"
    assert result["query_id"] == "query-1"
    assert len(result["rankings"]) == 1
    assert result["rankings"][0]["txn_count"] == 1
    assert result["rankings"][0]["outflow_total"] == 0.0
    assert result["same_fact_dedupe"]["applied"] is True
    assert result["same_fact_dedupe"]["raw_txn_count"] == 2
    assert result["same_fact_dedupe"]["effective_txn_count"] == 1
    assert result["same_fact_dedupe"]["duplicate_row_count"] == 1
    assert result["same_fact_dedupe"]["candidate_duplicate_amount"] == 0.0


def test_rank_counterparties_complete_nonzero_duplicate_is_folded(tmp_path) -> None:
    engine = _same_fact_rank_engine(
        tmp_path,
        [
            _same_fact_rank_row(row_id=1, account_key="acct-a", amount=100.0, balance=900.0),
            _same_fact_rank_row(row_id=2, account_key="acct-b", amount=100.0, balance=900.0),
        ],
    )
    result = _run_same_fact_rank(_same_fact_rank_repository(engine))

    assert result["coverage_status"] == "complete"
    assert result["rankings"][0]["txn_count"] == 1
    assert result["rankings"][0]["outflow_total"] == 100.0
    assert result["same_fact_dedupe"]["raw_txn_count"] == 2
    assert result["same_fact_dedupe"]["effective_txn_count"] == 1
    assert result["same_fact_dedupe"]["duplicate_row_count"] == 1
    assert result["same_fact_dedupe"]["candidate_duplicate_amount"] == 100.0


def test_rank_counterparties_complete_distinct_facts_report_observed_zero_duplicates(tmp_path) -> None:
    engine = _same_fact_rank_engine(
        tmp_path,
        [
            _same_fact_rank_row(row_id=1, account_key="acct-a", amount=100.0, balance=900.0),
            _same_fact_rank_row(row_id=2, account_key="acct-b", amount=100.0, balance=800.0),
        ],
    )
    result = _run_same_fact_rank(_same_fact_rank_repository(engine))

    assert result["coverage_status"] == "complete"
    assert result["rankings"][0]["txn_count"] == 2
    assert result["rankings"][0]["outflow_total"] == 200.0
    assert result["same_fact_dedupe"]["effective_txn_count"] == 2
    assert result["same_fact_dedupe"]["duplicate_row_count"] == 0
    assert result["same_fact_dedupe"]["candidate_duplicate_amount"] == 0.0


def test_rank_counterparties_rejects_non_boolean_dedupe_before_database_access() -> None:
    repository = _repository()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("invalid dedupe flag reached case synchronization")
    )

    with pytest.raises(ValueError, match="dedupe_same_holder_same_fact must be boolean"):
        repository.rank_counterparties("case-a", dedupe_same_holder_same_fact="false")  # type: ignore[arg-type]


def test_first_finite_number_sql_never_falls_through_present_invalid_source(tmp_path) -> None:
    expression = _first_finite_number_sql(
        {"amount_val", "amount"},
        ["amount_val", "amount"],
    )
    engine = DuckDBEngine(tmp_path / "first-finite-number.duckdb")
    try:
        rows = engine.query(
            f"""
            WITH source(amount_val, amount) AS (
              VALUES
                (NULL, NULL),
                (NULL, '0'),
                ('bad', '0'),
                ('NaN', '0'),
                ('Inf', '0'),
                ('0', '9')
            )
            SELECT {expression} AS amount_value
            FROM source
            """
        )
    finally:
        engine.close()

    assert [row[0] for row in rows] == [None, 0.0, None, None, None, 0.0]


class _Engine:
    def __init__(self) -> None:
        self.sql: list[str] = []

    def query(self, sql: str, _params=()):
        self.sql.append(sql)
        return []

    def execute(self, sql: str, _params=()) -> None:
        self.sql.append(sql)

    def close(self) -> None:
        return None


@pytest.mark.parametrize("value", HOSTILE_FACT_VALUES)
def test_field_quality_does_not_promote_missing_or_invalid_coverage_to_zero(value: object) -> None:
    summary = _summarize_field_quality(
        {
            "counterparty": value,
            "summary": value,
            "remark": value,
            "branch_name": value,
            "location": value,
            "success_status": value,
            "ip_addr": value,
            "mac_addr": value,
            "cash_flag": value,
        }
    )

    assert summary["label"] == "unknown"
    assert summary["label_display"] == "未知"
    assert summary["score"] is None
    assert summary["field_coverage"]["counterparty"] is None
    assert summary["missing_rates"]["counterparty"] is None
    assert "counterparty" not in summary["low_fields"]


def test_field_quality_preserves_observed_zero_without_calling_it_unknown() -> None:
    summary = _summarize_field_quality(
        {
            "counterparty": 0,
            "summary": 0,
            "remark": 0,
            "branch_name": 0,
            "location": 0,
            "success_status": 0,
            "ip_addr": 0,
            "mac_addr": 0,
            "cash_flag": 0,
        }
    )

    assert summary["score"] == 0
    assert summary["label"] == "low"
    assert summary["field_coverage"]["counterparty"] == 0
    assert summary["missing_rates"]["counterparty"] == 1


@pytest.mark.parametrize("value", HOSTILE_FACT_VALUES)
def test_reconciliation_does_not_match_invalid_counts_as_zero(value: object) -> None:
    assert _reconcile_count_status(value, 0) == "unavailable"
    assert _reconcile_count_status(0, value) == "unavailable"


def test_reconciliation_preserves_real_zero_count_observations() -> None:
    assert _reconcile_count_status(0, 0) == "matched"
    assert _reconcile_count_status(0, 1) == "mismatch"


@pytest.mark.parametrize("value", HOSTILE_FACT_VALUES)
def test_rule_param_normalization_rejects_hostile_numeric_overrides(value: object) -> None:
    with pytest.raises(ValueError, match="^rule_parameter_invalid:fast_in_out_window_minutes$"):
        _normalize_rule_params({"fast_in_out_window_minutes": value})


def test_rule_param_normalization_uses_defaults_only_for_absent_keys_and_preserves_valid_zero() -> None:
    normalized = _normalize_rule_params(
        {
            "fast_in_out_min_amount": 0,
        }
    )

    assert normalized["fast_in_out_window_minutes"] == RULE_PARAM_DEFAULTS["fast_in_out_window_minutes"]
    assert normalized["fast_in_out_min_amount"] == 0


def test_rule_param_normalization_rejects_unknown_out_of_range_and_inverted_bounds() -> None:
    with pytest.raises(ValueError, match="^rule_parameter_unknown:unknown_param$"):
        _normalize_rule_params({"unknown_param": 0})
    with pytest.raises(ValueError, match="^rule_parameter_invalid:fast_in_out_window_minutes$"):
        _normalize_rule_params({"fast_in_out_window_minutes": 0})
    with pytest.raises(ValueError, match="^rule_parameter_invalid:night_activity_start_hour$"):
        _normalize_rule_params({"night_activity_start_hour": 24})
    with pytest.raises(ValueError, match="^rule_parameter_invalid:small_fast_in_out_max_amount$"):
        _normalize_rule_params(
            {
                "small_fast_in_out_min_amount": 100.0,
                "small_fast_in_out_max_amount": 99.0,
            }
        )


def test_persisted_rule_param_loader_rejects_duplicate_invalid_and_wrong_scope_rows() -> None:
    base = {
        "scope_type": "case",
        "scope_key": "case-a",
        "param_key": "fast_in_out_window_minutes",
        "param_value_json": "30",
    }
    with pytest.raises(
        ValueError,
        match="^rule_parameter_duplicate:case:fast_in_out_window_minutes$",
    ):
        _rule_param_scope_overrides([base, dict(base)], case_id="case-a")

    invalid = {**base, "param_value_json": "null"}
    with pytest.raises(ValueError, match="^rule_parameter_invalid:fast_in_out_window_minutes$"):
        _rule_param_scope_overrides([invalid], case_id="case-a")

    wrong_scope = {**base, "scope_key": "case-b"}
    with pytest.raises(ValueError, match="^rule_parameter_scope_invalid$"):
        _rule_param_scope_overrides([wrong_scope], case_id="case-a")


@pytest.mark.parametrize("value", HOSTILE_FACT_VALUES)
def test_corrupt_persisted_rule_score_is_excluded_instead_of_hydrated_as_zero(value: object) -> None:
    rows = _repository()._hydrate_rule_hit_rows(
        [
            {
                "rule_hit_id": "hit-a",
                "rule_code": "FAST_IN_FAST_OUT",
                "score": value,
                "evidence_ids_json": '["fake-evidence"]',
            }
        ]
    )

    assert rows == []


def test_persisted_rule_score_preserves_observed_zero() -> None:
    rows = _repository()._hydrate_rule_hit_rows(
        [{"rule_hit_id": "hit-zero", "rule_code": "FAST_IN_FAST_OUT", "score": 0}]
    )

    assert rows[0]["score"] == 0


def _configure_account_aggregate_repository(rows: list[dict[str, Any]]) -> tuple[AnalysisRepository, _Engine]:
    repository = _repository()
    engine = _Engine()
    repository._table_exists = lambda *_args, **_kwargs: True  # type: ignore[method-assign]
    repository._table_columns = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "clean_acct_no",
        "account_open_name",
        "clean_amount",
        "txn_ts",
        "dc_final",
        "opener_id_no",
    }

    def _query_dicts(_engine, sql: str, _params=()):
        engine.sql.append(sql)
        return [dict(row) for row in rows]

    repository._query_dicts = _query_dicts  # type: ignore[method-assign]
    return repository, engine


def test_top_accounts_keeps_hostile_aggregate_facts_unresolved() -> None:
    repository, engine = _configure_account_aggregate_repository(
        [
            {
                "account_key": "acct-a",
                "display_name": "甲",
                "txn_count": True,
                "amount_present_count": {},
                "in_txn_count": "",
                "out_txn_count": [],
                "in_amount_present_count": nan,
                "out_amount_present_count": inf,
                "turnover_amount": None,
                "in_amount": "invalid",
                "out_amount": False,
            }
        ]
    )

    item = repository._top_accounts(engine, "case-a")[0]

    assert item["txn_count"] is None
    assert item["turnover_amount"] is None
    assert item["in_amount"] is None
    assert item["out_amount"] is None
    assert "amount_present_count" not in item
    assert all("ELSE 0" not in sql for sql in engine.sql)


def test_top_accounts_preserves_zero_only_when_the_amount_row_is_present() -> None:
    repository, engine = _configure_account_aggregate_repository(
        [
            {
                "account_key": "acct-a",
                "display_name": "甲",
                "txn_count": 1,
                "amount_present_count": 1,
                "in_txn_count": 0,
                "out_txn_count": 1,
                "in_amount_present_count": 0,
                "out_amount_present_count": 1,
                "turnover_amount": 0,
                "in_amount": None,
                "out_amount": 0,
            }
        ]
    )

    item = repository._top_accounts(engine, "case-a")[0]

    assert item["txn_count"] == 1
    assert item["turnover_amount"] == 0
    assert item["in_amount"] is None
    assert item["out_amount"] == 0


def test_account_set_summary_propagates_unknown_facts_across_grouping() -> None:
    repository, engine = _configure_account_aggregate_repository(
        [
            {
                "account_key": "acct-a",
                "holder_name": "甲",
                "holder_id_no": "id-a",
                "txn_count": 1,
                "amount_present_count": 1,
                "in_txn_count": 1,
                "out_txn_count": 0,
                "in_amount_present_count": 1,
                "out_amount_present_count": 0,
                "turnover_amount": 10,
                "in_amount": 10,
                "out_amount": None,
            },
            {
                "account_key": "acct-b",
                "holder_name": "甲",
                "holder_id_no": "id-a",
                "txn_count": 1,
                "amount_present_count": 0,
                "in_txn_count": 0,
                "out_txn_count": 1,
                "in_amount_present_count": 0,
                "out_amount_present_count": 0,
                "turnover_amount": None,
                "in_amount": None,
                "out_amount": None,
            },
        ]
    )

    item = repository._build_account_set_summaries(engine, "case-a")["items"][0]

    assert item["account_count"] == 2
    assert item["txn_count"] == 2
    assert item["turnover_amount"] is None
    assert item["in_amount"] is None
    assert item["out_amount"] is None


def test_field_presence_missing_query_row_is_unresolved_not_zero() -> None:
    repository = _repository()
    repository._query_dicts = lambda *_args, **_kwargs: []  # type: ignore[method-assign]

    result = repository._field_presence_rates(
        _Engine(),
        table_name="facts",
        columns=["amount"],
        available_columns={"amount"},
    )

    assert result["total"] is None
    assert result["amount"]["present"] is None
    assert result["amount"]["rate"] is None


def test_field_presence_real_empty_count_is_observed_but_not_a_coverage_rate() -> None:
    repository = _repository()
    repository._query_dicts = lambda *_args, **_kwargs: [{"total": 0, "amount_present": 0}]  # type: ignore[method-assign]

    result = repository._field_presence_rates(
        _Engine(),
        table_name="facts",
        columns=["amount"],
        available_columns={"amount"},
    )

    assert result["total"] == 0
    assert result["amount"]["present"] == 0
    assert result["amount"]["rate"] is None


def _configure_continuation_repository() -> AnalysisRepository:
    repository = _repository()
    engine = _Engine()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: None  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: engine  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._daily_agg = type("_DailyAgg", (), {"ensure_materialized": lambda *_args, **_kwargs: True})()
    repository._append_query_log = lambda *_args, **_kwargs: ""  # type: ignore[method-assign]
    return repository


def test_empty_result_is_not_zero_continuation_list_semantics() -> None:
    result = _configure_continuation_repository().validate_continuation_list(
        "case-a",
        rows=[],
        strict_db_match=False,
    )

    assert result["validation_status"] == "warning"
    assert result["amount_total_yuan"] is None
    assert result["expected_total_yuan"] is None
    assert any(issue["code"] == "CONTINUATION_LIST_EMPTY" for issue in result["issues"])


@pytest.mark.parametrize("value", HOSTILE_FACT_VALUES)
def test_continuation_list_hostile_amount_is_missing_not_zero(value: object) -> None:
    result = _configure_continuation_repository().validate_continuation_list(
        "case-a",
        rows=[
            {
                "txn_id": "txn-a",
                "account_key": "acct-a",
                "txn_time": "2026-07-20 09:00:00",
                "amount": value,
                "direction": "out",
            }
        ],
        strict_db_match=False,
    )

    assert result["amount_total_yuan"] is None
    assert result["validation_status"] == "failed"
    assert result["blank_field_counts"]["amount"] == 1


def test_continuation_list_preserves_observed_zero_without_verified_no_hit_upgrade() -> None:
    result = _configure_continuation_repository().validate_continuation_list(
        "case-a",
        rows=[
            {
                "txn_id": "txn-zero",
                "account_key": "acct-a",
                "txn_time": "2026-07-20 09:00:00",
                "amount": 0,
                "direction": "out",
            }
        ],
        strict_db_match=False,
    )

    assert result["amount_total_yuan"] == 0
    assert result["blank_field_counts"].get("amount", 0) == 0
    assert result["expected_total_yuan"] is None
    assert result["validation_status"] != "verified_no_hit"


@pytest.mark.parametrize("value", HOSTILE_FACT_VALUES)
def test_continuation_expected_total_hostile_value_stays_unresolved(value: object) -> None:
    result = _configure_continuation_repository().validate_continuation_list(
        "case-a",
        rows=[],
        expected_total_amount=value,  # type: ignore[arg-type]
        strict_db_match=False,
    )

    assert result["expected_total_yuan"] is None


def _configure_public_repository(engine: _Engine) -> AnalysisRepository:
    repository = _repository()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: None  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: engine  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._daily_agg = type("_DailyAgg", (), {"ensure_materialized": lambda *_args, **_kwargs: True})()
    repository._append_query_log = lambda *_args, **_kwargs: ""  # type: ignore[method-assign]
    return repository


def _configure_large_refresh_authority(
    repository: AnalysisRepository,
    *,
    txn_count: int = 1000,
    coverage: object | None = None,
) -> None:
    repository._analysis_refresh_txn_count_from_engine = lambda *_args: txn_count  # type: ignore[method-assign]
    repository._rule_txn_index_meta_ready = lambda *_args: True  # type: ignore[method-assign]
    repository._native_rule_pattern_param_signature = lambda _params: "pattern-signature-a"  # type: ignore[method-assign]
    repository._load_native_rule_pattern_features = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "ready": True,
        "param_signature": "pattern-signature-a",
        "by_account": {},
    }
    effective_coverage = coverage or TxnAmountCoverageV1(
        case_id="case-a",
        materialization_identity=f"txn_daily_snapshot:v12:{'a' * 64}",
        total_rows=txn_count,
        amount_valid_rows=txn_count,
        amount_missing_rows=0,
        amount_parse_failed_rows=0,
        direction_covered_rows=txn_count,
    )
    repository._daily_agg = type(
        "_DailyAgg",
        (),
        {
            "require_complete_amount_coverage": lambda *_args, **_kwargs: effective_coverage
        },
    )()


def test_owner_scope_missing_coverage_and_hostile_account_aggregates_stay_unresolved() -> None:
    engine = _Engine()
    repository = _configure_public_repository(engine)
    repository._query_dicts = lambda *_args, **_kwargs: [  # type: ignore[method-assign]
        {
            "account_key": "acct-unresolved",
            "txn_count": True,
            "amount_present_count": {},
            "in_txn_count": "",
            "out_txn_count": [],
            "in_amount_present_count": nan,
            "out_amount_present_count": inf,
            "inflow_total": None,
            "outflow_total": False,
            "max_single_amount": "invalid",
        }
    ]

    result = repository.resolve_owner_scope("case-a")
    coverage = result["expanded_coverage"]
    unresolved = result["unresolved_high_value_accounts"][0]

    assert coverage["txn_total"] is None
    assert coverage["txn_analyzed"] is None
    assert coverage["inflow_total"] is None
    assert coverage["outflow_total"] is None
    assert coverage["coverage_status"] == "unresolved"
    assert unresolved["txn_count"] is None
    assert unresolved["inflow_total"] is None
    assert unresolved["outflow_total"] is None
    assert unresolved["turnover_total"] is None
    assert unresolved["max_single_amount"] is None


def test_compare_analysis_scopes_does_not_fabricate_zero_from_missing_query_rows() -> None:
    engine = _Engine()
    repository = _configure_public_repository(engine)

    def _query_dicts(_engine, sql: str, _params=()):
        engine.sql.append(sql)
        return []

    repository._query_dicts = _query_dicts  # type: ignore[method-assign]

    result = repository.compare_analysis_scopes("case-a")
    scopes = {item["scope_id"]: item for item in result["scopes"]}

    assert scopes["detail_index_all_rows"]["txn_count"] is None
    assert scopes["detail_index_all_rows"]["account_count"] is None
    assert scopes["detail_index_all_rows"]["inflow_total"] is None
    assert scopes["detail_index_all_rows"]["outflow_total"] is None
    assert scopes["by_name_holder_aggregate"]["unresolved_txn_count"] is None
    assert scopes["by_name_holder_aggregate"]["unresolved_turnover_total"] is None
    assert scopes["report_dedupe_candidate"]["txn_count"] is None
    assert all("COALESCE(amount, 0)" not in sql for sql in engine.sql)


def _configure_trace_repository(seed_amount: object) -> AnalysisRepository:
    engine = _Engine()
    repository = _configure_public_repository(engine)
    repository._materialized_filtered_detail_scope = lambda *_args, **_kwargs: ("1=1", [])  # type: ignore[method-assign]
    repository._scope_stats_from_materialized = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "txn_count": 1,
        "in_amount": None,
        "out_amount": None,
        "coverage_status": "partial",
    }

    def _query_dicts(_engine, sql: str, _params=()):
        engine.sql.append(sql)
        if "AS business_category" in sql and "ORDER BY" in sql:
            return [
                {
                    "txn_id": "seed-a",
                    "txn_time": "2026-07-20 09:00:00",
                    "acct_key": "acct-a",
                    "amount": seed_amount,
                    "dc_val": "出",
                    "cp_key": "",
                    "cp_name": "",
                    "business_category": "transfer_unclassified",
                }
            ]
        return []

    repository._query_dicts = _query_dicts  # type: ignore[method-assign]
    return repository


@pytest.mark.parametrize("value", HOSTILE_FACT_VALUES)
def test_trace_top_outflows_skips_unresolved_seed_amounts(value: object) -> None:
    result = _configure_trace_repository(value).trace_subject_top_outflows(
        "case-a",
        downstream_limit_per_seed=0,
    )

    assert result["top_outflows"] == []
    assert result["terminal_summary"] == []
    assert result["business_category_summary"] == []
    assert any(warning["code"] == "TRACE_AMOUNT_COVERAGE_PARTIAL" for warning in result["warnings"])


def test_trace_top_outflows_preserves_an_observed_zero_seed() -> None:
    result = _configure_trace_repository(0).trace_subject_top_outflows(
        "case-a",
        downstream_limit_per_seed=0,
    )

    assert result["top_outflows"][0]["seed_txn"]["amount"] == 0
    assert result["terminal_summary"][0]["amount_total"] == 0


def test_missing_counterparty_classification_keeps_partial_amount_aggregates_unresolved() -> None:
    engine = _Engine()
    repository = _configure_public_repository(engine)
    repository._materialized_filtered_detail_scope = lambda *_args, **_kwargs: ("1=1", [])  # type: ignore[method-assign]
    repository._scope_stats_from_materialized = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "txn_count": 1,
        "coverage_status": "partial",
    }

    def _query_dicts(_engine, sql: str, _params=()):
        engine.sql.append(sql)
        if "WITH flagged AS" in sql:
            return [
                {
                    "detected_missing_kind": "counterparty",
                    "business_category": "transfer_unclassified",
                    "direction": "出",
                    "txn_count": 1,
                    "account_count": 1,
                    "counterparty_count": 0,
                    "amount_present_count": 0,
                    "amount_total": None,
                    "max_single_amount": None,
                }
            ]
        return [
            {
                "txn_id": "txn-a",
                "txn_time": "2026-07-20 09:00:00",
                "acct_key": "acct-a",
                "amount": None,
                "dc_val": "出",
                "detected_missing_kind": "counterparty",
                "business_category": "transfer_unclassified",
            }
        ]

    repository._query_dicts = _query_dicts  # type: ignore[method-assign]

    result = repository.classify_missing_counterparty_business("case-a")

    assert result["category_summary"][0]["amount_total"] is None
    assert result["category_summary"][0]["max_single_amount"] is None
    assert result["top_rows"][0]["amount"] is None
    assert any(warning["code"] == "CLASSIFY_AMOUNT_COVERAGE_PARTIAL" for warning in result["warnings"])
    assert all("COALESCE(amount, 0)" not in sql for sql in engine.sql)


def _configure_link_projection_repository() -> tuple[AnalysisRepository, _Engine]:
    engine = _Engine()
    repository = _repository()
    repository._detail_index_distinct_values = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "account_keys": [],
        "opener_ids": [],
        "ip_addrs": [],
        "mac_addrs": [],
    }
    repository._detail_index_sample_rows = lambda *_args, **_kwargs: []  # type: ignore[method-assign]
    repository._append_query_log = lambda *_args, **_kwargs: ""  # type: ignore[method-assign]
    return repository, engine


def test_device_link_projection_does_not_turn_hostile_counts_into_zero() -> None:
    repository, engine = _configure_link_projection_repository()

    def _query_dicts(_engine, sql: str, _params=()):
        if "AS matched_links" in sql:
            return [{"matched_links": True}]
        if "AS txn_count" in sql:
            return [{"device_value": "aa:bb", "device_label": "MAC", "txn_count": {}}]
        return []

    repository._query_dicts = _query_dicts  # type: ignore[method-assign]

    result = repository._get_device_links_from_detail_idx(
        engine,
        "case-a",
        ip="",
        mac="",
        person_id="",
        cursor="",
        limit=20,
        started=0,
    )

    assert result["items"][0]["txn_count"] is None
    assert result["stats"]["matched_links"] is None
    assert result["has_more"] is False


def test_branch_link_projection_does_not_turn_hostile_counts_into_zero() -> None:
    repository, engine = _configure_link_projection_repository()

    def _query_dicts(_engine, sql: str, _params=()):
        if "AS matched_links" in sql:
            return [{"matched_links": []}]
        if "AS txn_count" in sql:
            return [
                {
                    "branch_name": "网点甲",
                    "voucher_type": "凭证",
                    "voucher_no": "v-a",
                    "teller_no": "t-a",
                    "log_no": "l-a",
                    "txn_count": inf,
                }
            ]
        return []

    repository._query_dicts = _query_dicts  # type: ignore[method-assign]

    result = repository._get_branch_voucher_links_from_detail_idx(
        engine,
        "case-a",
        voucher_no="",
        teller_no="",
        log_no="",
        cursor="",
        limit=20,
        started=0,
    )

    assert result["items"][0]["txn_count"] is None
    assert result["stats"]["matched_links"] is None
    assert result["has_more"] is False


def test_large_refresh_summary_keeps_missing_database_counts_unresolved() -> None:
    engine = _Engine()
    repository = _configure_public_repository(engine)
    repository._table_exists = lambda *_args, **_kwargs: True  # type: ignore[method-assign]
    _configure_large_refresh_authority(repository)

    result = repository._refresh_large_case_analysis_summary(
        "case-a",
        txn_count=1000,
        force_refresh=False,
        native_rule_params={},
        native_rule_txn_index_ready=True,
        native_rule_pattern_index_ready=True,
        daily_agg_ready=True,
        started=0,
    )

    assert result["status"] == "dependency_unavailable"
    assert result["blocker"] == "analysis_materialization_count_unavailable"
    assert result["refreshed"] == []
    assert result["stats"]["txn_count"] is None
    assert result["stats"]["account_count"] is None
    assert result["stats"]["counterparty_count"] is None
    assert result["stats"]["node_count"] is None
    assert result["stats"]["edge_count"] is None
    assert result["stats"]["rule_index_count"] is None
    assert result["stats"]["rule_pattern_count"] is None
    assert engine.sql[0].strip().upper() == "BEGIN TRANSACTION"
    assert engine.sql[-1].strip().upper() == "ROLLBACK"
    assert not any(sql.strip().upper() == "COMMIT" for sql in engine.sql)


@pytest.mark.parametrize(
    "coverage",
    [
        object(),
        TxnAmountCoverageV1(
            case_id="case-b",
            materialization_identity=f"txn_daily_snapshot:v12:{'a' * 64}",
            total_rows=1000,
            amount_valid_rows=1000,
            amount_missing_rows=0,
            amount_parse_failed_rows=0,
            direction_covered_rows=1000,
        ),
        TxnAmountCoverageV1(
            case_id="case-a",
            materialization_identity="unbound",
            total_rows=1000,
            amount_valid_rows=1000,
            amount_missing_rows=0,
            amount_parse_failed_rows=0,
            direction_covered_rows=1000,
        ),
        TxnAmountCoverageV1(
            case_id="case-a",
            materialization_identity=f"txn_daily_snapshot:v12:{'a' * 64}",
            total_rows=1000,
            amount_valid_rows=999,
            amount_missing_rows=1,
            amount_parse_failed_rows=0,
            direction_covered_rows=1000,
        ),
        TxnAmountCoverageV1(
            case_id="case-a",
            materialization_identity=f"txn_daily_snapshot:v12:{'a' * 64}",
            total_rows=999,
            amount_valid_rows=999,
            amount_missing_rows=0,
            amount_parse_failed_rows=0,
            direction_covered_rows=999,
        ),
        TxnAmountCoverageV1(
            case_id="case-a",
            materialization_identity=f"txn_daily_snapshot:v12:{'a' * 64}",
            total_rows="1000",  # type: ignore[arg-type]
            amount_valid_rows="1000",  # type: ignore[arg-type]
            amount_missing_rows=0,
            amount_parse_failed_rows=0,
            direction_covered_rows=1000,
        ),
    ],
)
def test_large_refresh_rejects_cross_case_or_malformed_daily_coverage(coverage: object) -> None:
    engine = _Engine()
    repository = _configure_public_repository(engine)
    repository._table_exists = lambda *_args, **_kwargs: True  # type: ignore[method-assign]
    _configure_large_refresh_authority(repository, coverage=coverage)
    repository._append_query_log = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("invalid coverage reached query-log publication")
    )

    result = repository._refresh_large_case_analysis_summary(
        "case-a",
        txn_count=1000,
        force_refresh=False,
        native_rule_params={},
        native_rule_txn_index_ready=True,
        native_rule_pattern_index_ready=True,
        daily_agg_ready=True,
        started=0,
    )

    assert result["status"] == "partial"
    assert result["blocker"] == "transaction_amount_coverage_incomplete"
    assert result["refreshed"] == []
    assert set(result["stats"].values()) == {None}
    assert engine.sql[0].strip().upper() == "BEGIN TRANSACTION"
    assert engine.sql[-1].strip().upper() == "ROLLBACK"
    assert not any(sql.strip().upper() == "COMMIT" for sql in engine.sql)


def test_large_refresh_revalidates_every_authority_inside_one_transaction() -> None:
    class _CompleteLargeEngine(_Engine):
        def query(self, sql: str, _params=()):
            self.sql.append(sql)
            if "analysis_txn_detail_idx" in sql:
                return [(1000,)]
            if "analysis_account_dim" in sql:
                return [(2,)]
            if "analysis_txn_daily_agg" in sql:
                return [(2, 3, 4, 1000)]
            if "analysis_rule_txn_idx" in sql:
                return [(1000,)]
            if "analysis_rule_pattern_idx" in sql:
                return [(7,)]
            return []

    engine = _CompleteLargeEngine()
    repository = _configure_public_repository(engine)
    repository._table_exists = lambda *_args, **_kwargs: True  # type: ignore[method-assign]
    calls: list[str] = []

    def assert_snapshot(name: str, observed_engine: object) -> None:
        assert observed_engine is engine
        assert engine.sql[0].strip().upper() == "BEGIN TRANSACTION"
        assert not any(sql.strip().upper() in {"COMMIT", "ROLLBACK"} for sql in engine.sql)
        calls.append(name)

    repository._analysis_refresh_txn_count_from_engine = lambda observed_engine, _case_id: (  # type: ignore[method-assign]
        assert_snapshot("source_count", observed_engine) or 1000
    )
    repository._rule_txn_index_meta_ready = lambda observed_engine, _case_id: (  # type: ignore[method-assign]
        assert_snapshot("rule_meta", observed_engine) or True
    )
    repository._native_rule_pattern_param_signature = lambda _params: "pattern-signature-a"  # type: ignore[method-assign]

    def load_pattern(observed_engine, _case_id, **_kwargs):
        assert_snapshot("pattern_meta", observed_engine)
        return {"ready": True, "param_signature": "pattern-signature-a", "by_account": {}}

    repository._load_native_rule_pattern_features = load_pattern  # type: ignore[method-assign]

    class _DailyCoverage:
        def require_complete_amount_coverage(self, *, case_id, engine: object, ensure_ready: bool):
            assert case_id == "case-a"
            assert ensure_ready is False
            assert_snapshot("daily_coverage", engine)
            return TxnAmountCoverageV1(
                case_id="case-a",
                materialization_identity=f"txn_daily_snapshot:v12:{'a' * 64}",
                total_rows=1000,
                amount_valid_rows=1000,
                amount_missing_rows=0,
                amount_parse_failed_rows=0,
                direction_covered_rows=1000,
            )

    repository._daily_agg = _DailyCoverage()

    def append_log(observed_engine, *_args, **_kwargs):
        assert_snapshot("query_log", observed_engine)
        return "query-a"

    repository._append_query_log = append_log  # type: ignore[method-assign]

    result = repository._refresh_large_case_analysis_summary(
        "case-a",
        txn_count=1000,
        force_refresh=False,
        native_rule_params={},
        native_rule_txn_index_ready=True,
        native_rule_pattern_index_ready=True,
        daily_agg_ready=True,
        started=0,
    )

    assert result["status"] == "succeeded"
    assert calls == ["source_count", "rule_meta", "pattern_meta", "daily_coverage", "query_log"]
    assert engine.sql[0].strip().upper() == "BEGIN TRANSACTION"
    assert engine.sql[-1].strip().upper() == "COMMIT"


@pytest.mark.parametrize("target", ["detail", "account", "aggregate", "rule", "pattern"])
def test_large_refresh_count_contract_is_exact_for_every_query(target: str) -> None:
    class _HostileCountEngine(_Engine):
        def query(self, sql: str, _params=()):
            self.sql.append(sql)
            if "analysis_txn_detail_idx" in sql:
                return [(True,)] if target == "detail" else [(1000,)]
            if "analysis_account_dim" in sql:
                return [("2",)] if target == "account" else [(2,)]
            if "analysis_txn_daily_agg" in sql:
                return [(2, 3, 4, 1000.0)] if target == "aggregate" else [(2, 3, 4, 1000)]
            if "analysis_rule_txn_idx" in sql:
                return [(1000,), (1000,)] if target == "rule" else [(1000,)]
            if "analysis_rule_pattern_idx" in sql:
                return [(7, 0)] if target == "pattern" else [(7,)]
            return []

    engine = _HostileCountEngine()
    repository = _configure_public_repository(engine)
    repository._table_exists = lambda *_args, **_kwargs: True  # type: ignore[method-assign]
    _configure_large_refresh_authority(repository)
    repository._append_query_log = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("hostile count reached query-log publication")
    )

    result = repository._refresh_large_case_analysis_summary(
        "case-a",
        txn_count=1000,
        force_refresh=False,
        native_rule_params={},
        native_rule_txn_index_ready=True,
        native_rule_pattern_index_ready=True,
        daily_agg_ready=True,
        started=0,
    )

    assert result["status"] == "dependency_unavailable"
    assert result["blocker"] == "analysis_materialization_count_unavailable"
    assert result["refreshed"] == []
    assert set(result["stats"].values()) == {None}
    assert engine.sql[-1].strip().upper() == "ROLLBACK"
    assert not any(sql.strip().upper() == "COMMIT" for sql in engine.sql)


@pytest.mark.parametrize(
    ("rule_ready", "pattern_ready", "daily_ready"),
    [
        (False, True, True),
        (True, False, True),
        (True, True, False),
    ],
)
def test_large_refresh_requires_every_materializer_before_database_access(
    rule_ready: bool,
    pattern_ready: bool,
    daily_ready: bool,
) -> None:
    repository = _repository()
    repository.open_case_engine = lambda _case_id: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("incomplete large refresh reached the summary database")
    )

    result = repository._refresh_large_case_analysis_summary(
        "case-a",
        txn_count=1000,
        force_refresh=False,
        native_rule_params={},
        native_rule_txn_index_ready=rule_ready,
        native_rule_pattern_index_ready=pattern_ready,
        daily_agg_ready=daily_ready,
        started=0,
    )

    assert result["status"] == "dependency_unavailable"
    assert result["blocker"] == "analysis_materialization_dependency_unavailable"
    assert result["refreshed"] == []
    assert set(result["stats"].values()) == {None}


@pytest.mark.parametrize(
    ("daily_txn_count", "expected_status", "expected_blocker"),
    [
        (1000, "succeeded", ""),
        (999, "partial", "analysis_materialization_count_mismatch"),
        ("1000", "dependency_unavailable", "analysis_materialization_count_unavailable"),
        (True, "dependency_unavailable", "analysis_materialization_count_unavailable"),
        (1000.0, "dependency_unavailable", "analysis_materialization_count_unavailable"),
    ],
)
def test_large_refresh_requires_exact_materialized_counts(
    daily_txn_count: object,
    expected_status: str,
    expected_blocker: str,
) -> None:
    class _LargeRefreshEngine(_Engine):
        def query(self, sql: str, _params=()):
            self.sql.append(sql)
            if "analysis_txn_detail_idx" in sql:
                return [(1000,)]
            if "analysis_account_dim" in sql:
                return [(2,)]
            if "analysis_txn_daily_agg" in sql:
                return [(2, 3, 4, daily_txn_count)]
            if "analysis_rule_txn_idx" in sql:
                return [(1000,)]
            if "analysis_rule_pattern_idx" in sql:
                return [(7,)]
            return []

    engine = _LargeRefreshEngine()
    repository = _configure_public_repository(engine)
    repository._table_exists = lambda *_args, **_kwargs: True  # type: ignore[method-assign]
    _configure_large_refresh_authority(repository)
    log_calls: list[dict[str, object]] = []
    repository._append_query_log = lambda *_args, **kwargs: log_calls.append(kwargs) or "query-a"  # type: ignore[method-assign]

    result = repository._refresh_large_case_analysis_summary(
        "case-a",
        txn_count=1000,
        force_refresh=False,
        native_rule_params={},
        native_rule_txn_index_ready=True,
        native_rule_pattern_index_ready=True,
        daily_agg_ready=True,
        started=0,
    )

    assert result["status"] == expected_status
    if expected_status == "succeeded":
        assert result["stats"]["txn_count"] == 1000
        assert result["stats"]["node_count"] == 5
        assert len(log_calls) == 1
        assert engine.sql[0].strip().upper() == "BEGIN TRANSACTION"
        assert engine.sql[-1].strip().upper() == "COMMIT"
        assert not any(sql.strip().upper() == "ROLLBACK" for sql in engine.sql)
    else:
        assert result["blocker"] == expected_blocker
        assert result["refreshed"] == []
        assert set(result["stats"].values()) == {None}
        assert log_calls == []
        assert engine.sql[0].strip().upper() == "BEGIN TRANSACTION"
        assert engine.sql[-1].strip().upper() == "ROLLBACK"
        assert not any(sql.strip().upper() == "COMMIT" for sql in engine.sql)


def test_rule_hit_sort_unknown_metrics_never_become_facts_or_raise() -> None:
    repository = _repository()

    key = repository._rule_hit_significance_sort_key(
        {
            "rule_code": "FAST_IN_FAST_OUT",
            "severity": "high",
            "score": None,
            "detail_json": {
                "txn_count": None,
                "out_amount": None,
                "in_amount": None,
                "out_ratio": None,
            },
        }
    )

    assert isinstance(key, tuple)
    assert key[2:6] == (0.0, 0.0, 0, 0.0)


def test_rule_param_update_does_not_call_removed_unbound_refresh_cache() -> None:
    engine = _Engine()
    repository = _repository()
    repository.case_exists = lambda case_id: case_id == "case-a"  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: engine  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._query_dicts = lambda *_args, **_kwargs: []  # type: ignore[method-assign]
    repository._load_rule_params = lambda *_args, **_kwargs: dict(RULE_PARAM_DEFAULTS)  # type: ignore[method-assign]

    result = repository.update_rule_params(
        "case-a",
        scope_type="case",
        params={"fast_in_out_min_amount": 0},
    )

    assert result["updated_keys"] == ["fast_in_out_min_amount"]
    assert all("analysis_refresh_state" not in sql for sql in engine.sql)


def test_case_reconciliation_missing_rows_never_match_as_zero() -> None:
    engine = _Engine()
    repository = _configure_public_repository(engine)
    repository._table_exists = lambda *_args, **_kwargs: True  # type: ignore[method-assign]
    repository._table_columns = lambda *_args, **_kwargs: {"txn_count"}  # type: ignore[method-assign]
    repository._scope_stats_from_materialized = lambda *_args, **_kwargs: {"field_coverage": {}}  # type: ignore[method-assign]
    repository._query_dicts = lambda *_args, **_kwargs: []  # type: ignore[method-assign]

    result = repository.build_case_reconciliation("case-a")

    assert all(value is None for value in result["counts"].values())
    assert all(value == "unavailable" for value in result["reconciliation"].values())
    assert result["all_transaction_indexes_matched"] is False


def test_duplicate_family_projection_keeps_missing_summary_and_family_facts_unresolved() -> None:
    engine = _Engine()
    repository = _configure_public_repository(engine)
    repository._resolve_rank_scope_accounts = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "scope_type": "holder",
        "scope_requested": True,
        "selected_accounts": ["acct-zero-a", "acct-zero-b"],
        "warnings": [],
    }
    repository._rank_scope_where = lambda *_args, **_kwargs: ("1=1", [])  # type: ignore[method-assign]

    def _query_dicts(_engine, sql: str, _params=()):
        engine.sql.append(sql)
        if "requested_row_count" in sql:
            return [
                {
                    "requested_row_count": True,
                    "fact_eligible_row_count": {},
                    "family_eligible_row_count": [],
                    "rejected_row_count": nan,
                }
            ]
        if "SELECT rejection_reason" in sql:
            return []
        raise AssertionError("unresolved eligibility must suppress family query")

    repository._query_dicts = _query_dicts  # type: ignore[method-assign]

    result = repository.resolve_duplicate_families("case-a")

    assert result["coverage_status"] == "unresolved"
    assert result["blocker"] == "same_fact_eligibility_unavailable"
    assert result["same_fact_eligibility"]["rejection_reason_histogram"] is None
    assert result["summary"]["scoped_txn_count"] is None
    assert result["summary"]["same_holder_same_fact"]["extra_rows"] is None
    assert result["summary"]["same_holder_same_fact"]["candidate_duplicate_amount"] is None
    assert result["families"] == []
    assert all("COALESCE(t.amount, 0)" not in sql for sql in engine.sql)
    assert all("COALESCE(t.balance, 0)" not in sql for sql in engine.sql)


def test_duplicate_family_sql_preserves_observed_zero_when_every_row_is_eligible(tmp_path) -> None:
    engine = DuckDBEngine(tmp_path / "duplicate-family-finite-boundary.duckdb")
    engine.execute(
        """
        CREATE TABLE analysis_account_dim (
          account_key VARCHAR,
          open_name VARCHAR,
          id_no VARCHAR
        )
        """
    )
    engine.execute(
        """
        CREATE TABLE analysis_txn_detail_idx (
          id BIGINT,
          account_open_name VARCHAR,
          opener_id_no VARCHAR,
          acct_key VARCHAR,
          txn_ts TIMESTAMP,
          txn_time VARCHAR,
          amount DOUBLE,
          balance DOUBLE,
          dc_val VARCHAR,
          cp_key VARCHAR,
          cp_raw VARCHAR,
          counterparty_acct VARCHAR,
          cp_name VARCHAR,
          counterparty_name VARCHAR,
          summary VARCHAR,
          remark VARCHAR,
          txn_type VARCHAR,
          is_success VARCHAR,
          txn_id VARCHAR,
          file_id VARCHAR
        )
        """
    )
    engine.execute(
        """
        INSERT INTO analysis_account_dim VALUES
          ('acct-zero-a', '甲', 'id-a'), ('acct-zero-b', '甲', 'id-a')
        """
    )
    engine.execute(
        """
        INSERT INTO analysis_txn_detail_idx VALUES
          (1, '甲', 'id-a', 'acct-zero-a', TIMESTAMP '2026-07-20 10:00:00', '', 0, 0, '进', 'cp', '', '', '乙', '', '摘要', '备注', '转账', 'true', 'txn-zero', 'file-a'),
          (2, '甲', 'id-a', 'acct-zero-b', TIMESTAMP '2026-07-20 10:00:00', '', 0, 0, '进', 'cp', '', '', '乙', '', '摘要', '备注', '转账', 'true', 'txn-zero', 'file-b')
        """
    )
    repository = _configure_public_repository(engine)  # type: ignore[arg-type]
    repository._resolve_rank_scope_accounts = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "scope_type": "holder",
        "scope_requested": True,
        "selected_accounts": ["acct-zero-a", "acct-zero-b"],
        "warnings": [],
    }
    repository._rank_scope_where = lambda *_args, **_kwargs: ("TRUE", [])  # type: ignore[method-assign]

    result = repository.resolve_duplicate_families("case-a", min_amount=0)

    assert result["coverage_status"] == "complete"
    assert result["same_fact_eligibility"] == {
        "contract": "SameFactEligibilityV1",
        "key_version": "analytix.same-fact-dedupe-key/v2",
        "coverage_status": "complete",
        "requested_row_count": 2,
        "fact_eligible_row_count": 2,
        "family_eligible_row_count": 2,
        "rejected_row_count": 0,
        "rejection_reason_histogram": {},
        "blocker": "",
    }
    assert result["summary"]["scoped_txn_count"] == 2
    assert result["summary"]["same_holder_same_fact"] == {
        "group_count": 1,
        "extra_rows": 1,
        "candidate_duplicate_amount": 0.0,
    }
    assert len(result["families"]) == 1
    assert result["families"][0]["amount"] == "0.00"
    assert result["families"][0]["balance"] == "0.00"
    assert result["families"][0]["candidate_duplicate_amount"] == 0.0


def _duplicate_family_row(
    *,
    row_id: int,
    account_key: str,
    amount: float = 100.0,
    balance: float = 900.0,
    txn_id: str = "txn-family",
) -> tuple[object, ...]:
    return (
        row_id,
        "甲",
        "id-a",
        account_key,
        "2026-07-20 10:00:00",
        "",
        amount,
        balance,
        "进",
        "cp",
        "",
        "",
        "乙",
        "",
        "摘要",
        "备注",
        "转账",
        "true",
        txn_id,
        "file-a",
    )


def _duplicate_family_test_repository(tmp_path, rows: list[tuple[object, ...]]) -> AnalysisRepository:
    engine = DuckDBEngine(tmp_path / "duplicate-family-eligibility.duckdb")
    engine.execute(
        "CREATE TABLE analysis_account_dim (account_key VARCHAR, open_name VARCHAR, id_no VARCHAR)"
    )
    engine.execute(
        """
        CREATE TABLE analysis_txn_detail_idx (
          id BIGINT, account_open_name VARCHAR, opener_id_no VARCHAR, acct_key VARCHAR,
          txn_ts TIMESTAMP, txn_time VARCHAR, amount DOUBLE, balance DOUBLE, dc_val VARCHAR,
          cp_key VARCHAR, cp_raw VARCHAR, counterparty_acct VARCHAR, cp_name VARCHAR,
          counterparty_name VARCHAR, summary VARCHAR, remark VARCHAR, txn_type VARCHAR,
          is_success VARCHAR, txn_id VARCHAR, file_id VARCHAR
        )
        """
    )
    account_keys = sorted({str(row[3]) for row in rows})
    if account_keys:
        engine.executemany(
            "INSERT INTO analysis_account_dim VALUES (?, '甲', 'id-a')",
            [(account_key,) for account_key in account_keys],
        )
    if rows:
        engine.executemany(
            "INSERT INTO analysis_txn_detail_idx VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
            rows,
        )
    repository = _configure_public_repository(engine)  # type: ignore[arg-type]
    repository._resolve_rank_scope_accounts = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "scope_type": "holder",
        "scope_requested": True,
        "selected_accounts": account_keys or ["acct-empty"],
        "warnings": [],
    }
    repository._rank_scope_where = lambda *_args, **_kwargs: ("TRUE", [])  # type: ignore[method-assign]
    return repository


def test_duplicate_family_partial_eligibility_never_publishes_subset_zero_or_family(tmp_path) -> None:
    repository = _duplicate_family_test_repository(
        tmp_path,
        [
            _duplicate_family_row(row_id=1, account_key="acct-a"),
            _duplicate_family_row(row_id=2, account_key="acct-b"),
            _duplicate_family_row(row_id=3, account_key="acct-c", amount=nan),
        ],
    )

    result = repository.resolve_duplicate_families("case-a")

    assert result["coverage_status"] == "partial"
    assert result["blocker"] == "same_fact_eligibility_incomplete"
    assert result["same_fact_eligibility"]["requested_row_count"] == 3
    assert result["same_fact_eligibility"]["fact_eligible_row_count"] == 2
    assert result["same_fact_eligibility"]["family_eligible_row_count"] == 2
    assert result["same_fact_eligibility"]["rejection_reason_histogram"] == {
        "amount_missing_or_invalid": 1
    }
    assert result["summary"]["same_holder_same_fact"] == {
        "group_count": None,
        "extra_rows": None,
        "candidate_duplicate_amount": None,
    }
    assert result["families"] == []


def test_duplicate_family_duplicate_row_identity_fails_closed(tmp_path) -> None:
    repository = _duplicate_family_test_repository(
        tmp_path,
        [
            _duplicate_family_row(row_id=1, account_key="acct-a"),
            _duplicate_family_row(row_id=1, account_key="acct-b"),
        ],
    )

    result = repository.resolve_duplicate_families("case-a")

    assert result["coverage_status"] == "partial"
    assert result["same_fact_eligibility"]["fact_eligible_row_count"] == 2
    assert result["same_fact_eligibility"]["family_eligible_row_count"] == 0
    assert result["same_fact_eligibility"]["rejection_reason_histogram"] == {
        "duplicate_row_identity": 2
    }
    assert result["families"] == []


def test_duplicate_family_empty_scope_is_not_verified_no_hit(tmp_path) -> None:
    repository = _duplicate_family_test_repository(tmp_path, [])

    result = repository.resolve_duplicate_families("case-a")

    assert result["coverage_status"] == "empty_unverified"
    assert result["same_fact_eligibility"]["requested_row_count"] == 0
    assert result["same_fact_eligibility"]["rejection_reason_histogram"] == {}
    assert result["summary"]["scoped_txn_count"] is None
    assert result["summary"]["same_holder_same_fact"]["group_count"] is None
    assert result["families"] == []


@pytest.mark.parametrize(
    ("kwargs", "message"),
    [
        ({"scope_mode": "unknown"}, "scope_mode must be"),
        ({"min_amount": -1}, "min_amount must be nonnegative"),
    ],
)
def test_duplicate_family_rejects_invalid_external_scope_before_database_access(
    kwargs: dict[str, object],
    message: str,
) -> None:
    repository = _repository()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: None  # type: ignore[method-assign]
    repository._resolve_rank_scope_accounts = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "scope_type": "holder",
        "scope_requested": True,
        "selected_accounts": ["acct-a"],
        "warnings": [],
    }
    repository.open_case_engine = lambda _case_id: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("invalid scope reached database access")
    )

    with pytest.raises(ValueError, match=message):
        repository.resolve_duplicate_families("case-a", **kwargs)  # type: ignore[arg-type]


def test_duplicate_family_rejects_stale_materialization_before_query() -> None:
    engine = _Engine()
    repository = _configure_public_repository(engine)
    repository._daily_agg = type(
        "_UnavailableDailyAgg",
        (),
        {"ensure_materialized": lambda *_args, **_kwargs: False},
    )()
    repository._resolve_rank_scope_accounts = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "scope_type": "holder",
        "scope_requested": True,
        "selected_accounts": ["acct-a"],
        "warnings": [],
    }

    with pytest.raises(TransactionFactSourceUnavailableError):
        repository.resolve_duplicate_families("case-a")

    assert engine.sql == []


def test_data_quality_audit_uses_same_fact_eligibility_for_verified_zero_amount(tmp_path) -> None:
    repository = _duplicate_family_test_repository(
        tmp_path,
        [
            _duplicate_family_row(row_id=1, account_key="acct-a", amount=0.0, balance=0.0),
            _duplicate_family_row(row_id=2, account_key="acct-b", amount=0.0, balance=0.0),
        ],
    )

    result = repository.audit_case_data_quality("case-a")
    cleaning = result["cleaning_quality"]

    assert cleaning["same_fact_eligibility"] == {
        "contract": "SameFactEligibilityV1",
        "key_version": "analytix.same-fact-dedupe-key/v2",
        "coverage_status": "complete",
        "requested_row_count": 2,
        "fact_eligible_row_count": 2,
        "family_eligible_row_count": 2,
        "rejected_row_count": 0,
        "rejection_reason_histogram": {},
        "blocker": "",
    }
    assert cleaning["duplicate_summary"]["same_fact_cross_account_groups"] == 1
    assert cleaning["duplicate_summary"]["same_fact_cross_account_extra_rows"] == 1
    assert cleaning["duplicate_summary"]["same_holder_same_fact_candidate_duplicate_amount"] == 0.0
    assert len(cleaning["duplicate_examples"]["same_fact_cross_account"]) == 1
    assert cleaning["duplicate_examples"]["same_fact_cross_account"][0]["max_amount"] == 0.0


def test_data_quality_audit_partial_same_fact_coverage_publishes_no_subset_fact(tmp_path) -> None:
    repository = _duplicate_family_test_repository(
        tmp_path,
        [
            _duplicate_family_row(row_id=1, account_key="acct-a"),
            _duplicate_family_row(row_id=2, account_key="acct-b"),
            _duplicate_family_row(row_id=3, account_key="acct-c", amount=nan),
        ],
    )

    result = repository.audit_case_data_quality("case-a")
    cleaning = result["cleaning_quality"]

    assert cleaning["same_fact_eligibility"]["coverage_status"] == "partial"
    assert cleaning["same_fact_eligibility"]["rejection_reason_histogram"] == {
        "amount_missing_or_invalid": 1
    }
    assert cleaning["duplicate_summary"]["same_fact_cross_account_groups"] is None
    assert cleaning["duplicate_summary"]["same_fact_cross_account_extra_rows"] is None
    assert cleaning["duplicate_summary"]["same_holder_same_fact_candidate_duplicate_amount"] is None
    assert cleaning["duplicate_examples"]["same_fact_cross_account"] == []


def test_data_quality_audit_empty_same_fact_scope_is_not_zero(tmp_path) -> None:
    repository = _duplicate_family_test_repository(tmp_path, [])

    result = repository.audit_case_data_quality("case-a")
    cleaning = result["cleaning_quality"]

    assert cleaning["same_fact_eligibility"]["coverage_status"] == "empty_unverified"
    assert cleaning["same_fact_eligibility"]["requested_row_count"] == 0
    assert cleaning["duplicate_summary"]["same_fact_cross_account_groups"] is None
    assert cleaning["duplicate_summary"]["same_holder_same_fact_groups"] is None


def test_data_quality_audit_stale_materialization_keeps_same_fact_unresolved() -> None:
    engine = _Engine()
    repository = _configure_public_repository(engine)
    repository._daily_agg = type(
        "_UnavailableDailyAgg",
        (),
        {"ensure_materialized": lambda *_args, **_kwargs: False},
    )()
    repository._table_exists = lambda *_args, **_kwargs: False  # type: ignore[method-assign]

    result = repository.audit_case_data_quality("case-a")

    assert result["cleaning_quality"]["same_fact_eligibility"]["coverage_status"] == "unresolved"
    assert result["cleaning_quality"]["duplicate_summary"]["same_fact_cross_account_groups"] is None
    assert any(
        warning.get("code") == "SAME_FACT_MATERIALIZATION_UNAVAILABLE"
        for warning in result["warnings"]
    )


def test_data_quality_audit_never_fabricates_row_number_zero_when_column_is_absent() -> None:
    engine = _Engine()
    repository = _configure_public_repository(engine)
    repository._daily_agg = type(
        "_UnavailableDailyAgg",
        (),
        {"ensure_materialized": lambda *_args, **_kwargs: False},
    )()
    repository._table_exists = lambda _engine, table: table == "fc_transaction_norm"  # type: ignore[method-assign]
    repository._table_columns = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "row_hash",
        "card_no_norm",
        "account_open_name",
        "txn_ts",
        "amount_val",
        "dc_final",
        "counterparty_acct_norm",
        "counterparty_name",
        "txn_id",
    }
    repository._query_dicts = lambda _engine, sql, _params=(): engine.sql.append(sql) or []  # type: ignore[method-assign]

    repository.audit_case_data_quality("case-a")

    combined_sql = "\n".join(engine.sql)
    assert "0 AS row_numbers" not in combined_sql
    assert "[] AS row_numbers" in combined_sql


def test_data_quality_audit_keeps_missing_and_hostile_fact_counts_unresolved() -> None:
    engine = _Engine()
    repository = _configure_public_repository(engine)
    repository._table_exists = lambda *_args, **_kwargs: True  # type: ignore[method-assign]
    repository._table_columns = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "rows_total",
        "rows_imported_raw",
        "rows_imported_norm",
        "rows_dedup",
        "rows_error",
        "rows_skipped_non_data",
        "account_open_name",
        "card_no_norm",
        "acct_no_norm",
        "txn_ts",
        "amount",
        "amount_val",
        "balance",
        "balance_val",
        "dc_final",
        "counterparty_name",
        "counterparty_acct_norm",
        "row_hash",
        "txn_id",
        "row_no",
        "source_signature",
    }

    def _hostile_counts(*keys: str) -> dict[str, object]:
        values = iter(HOSTILE_FACT_VALUES)
        return {key: next(values, nan) for key in keys}

    def _query_dicts(_engine, sql: str, _params=()):
        engine.sql.append(sql)
        if "COUNT(1) AS file_log_count" in sql and "GROUP BY" not in sql:
            return [
                _hostile_counts(
                    "file_log_count",
                    "rows_total",
                    "rows_imported_raw",
                    "rows_imported_norm",
                    "rows_dedup",
                    "rows_error",
                    "rows_skipped_non_data",
                )
            ]
        if "COUNT(1) AS txn_total" in sql and "FROM fc_transaction_norm" in sql:
            return [
                _hostile_counts(
                    "txn_total",
                    "clean_duplicate",
                    "clean_failed",
                    "clean_reversal",
                    "txn_blank_holder_account_count",
                    "txn_blank_holder_rows",
                    "blank_counterparty_name_rows",
                    "blank_counterparty_account_rows",
                )
            ]
        if "abnormal_early_txn_time_rows" in sql:
            return [_hostile_counts("abnormal_early_txn_time_rows", "missing_txn_time_rows")]
        if "WITH dim AS" in sql:
            return [
                _hostile_counts(
                    "account_count",
                    "registered_account_count",
                    "unregistered_account_count",
                    "unregistered_txn_count",
                    "unregistered_amount_present_count",
                    "unregistered_turnover_total",
                )
            ]
        if "SELECT\n                      d.account_key" in sql:
            return [
                {
                    "account_key": "acct-a",
                    "txn_count": True,
                    "amount_present_count": {},
                    "turnover_total": nan,
                }
            ]
        if "txn_blank_holder_resolved_account_count" in sql:
            return [
                _hostile_counts(
                    "txn_blank_holder_account_count",
                    "txn_blank_holder_resolved_account_count",
                    "txn_blank_holder_unregistered_account_count",
                )
            ]
        if "row_hash_duplicate_groups" in sql:
            return [
                _hostile_counts(
                    "row_hash_duplicate_groups",
                    "row_hash_extra_rows",
                    "same_txn_id_cross_account_groups",
                    "same_txn_id_cross_account_extra_rows",
                    "same_fact_cross_account_groups",
                    "same_fact_cross_account_extra_rows",
                    "natural_duplicate_within_account_groups",
                    "natural_duplicate_within_account_extra_rows",
                )
            ]
        if "same_holder_same_fact_groups" in sql:
            return [
                _hostile_counts(
                    "same_holder_same_fact_groups",
                    "same_holder_same_fact_extra_rows",
                    "same_holder_same_fact_candidate_duplicate_amount",
                    "full_case_same_fact_cross_holder_groups",
                )
            ]
        return []

    repository._query_dicts = _query_dicts  # type: ignore[method-assign]

    result = repository.audit_case_data_quality("case-a")
    identity = result["account_identity_quality"]
    duplicate = result["cleaning_quality"]["duplicate_summary"]

    assert all(value is None for value in result["table_counts"].values())
    assert result["import_lineage"]["summary"]["rows_total"] is None
    assert result["cleaning_quality"]["flags"]["clean_duplicate"] is None
    assert result["cleaning_quality"]["transaction_time_quality"]["missing_txn_time_rows"] is None
    assert identity["account_count"] is None
    assert identity["unregistered_txn_count"] is None
    assert identity["unregistered_turnover_total"] is None
    assert identity["unregistered_accounts"][0]["txn_count"] is None
    assert identity["unregistered_accounts"][0]["turnover_total"] is None
    assert duplicate["row_hash_duplicate_groups"] is None
    assert duplicate["same_holder_same_fact_candidate_duplicate_amount"] is None
