from __future__ import annotations

import inspect
import json
from datetime import datetime

import pytest

from app.core.db_engine import DuckDBEngine
from app.domain.workspace_artifact_renderer import WorkspaceArtifactRenderer
from app.repositories.analysis_repository import (
    AnalysisGraphContractError,
    AnalysisRepository,
    _bucket_overview_text,
    _complete_amount_sum,
    _complete_rank_amount_facts,
    _direction_overview_text,
    _fund_business_category_label,
    _fund_business_category_sql,
    _rule_input_coverage,
    _sort_rank_rows,
    _trace_amount_value,
)
from app.utils.mixed_fund_tracking import (
    build_mixed_fund_conclusion_text,
    enrich_mixed_fund_tracking,
)


def _repository() -> AnalysisRepository:
    return object.__new__(AnalysisRepository)


@pytest.mark.parametrize("value", [None, "", "not-a-number", float("nan"), float("inf"), True])
def test_trace_amount_parser_keeps_missing_and_invalid_unresolved(value: object) -> None:
    assert _trace_amount_value({"amount_val": value}) is None


def test_trace_amount_parser_preserves_explicit_zero() -> None:
    assert _trace_amount_value({"amount_val": 0}) == 0.0
    assert _trace_amount_value({"trace_amount_val": 0, "amount_val": 99}) == 0.0


def test_trace_seed_sort_keeps_unknown_amount_distinct_from_observed_zero() -> None:
    repository = _repository()
    rows = [
        {"account_key": "acct-a", "txn_id": "missing", "amount_val": None},
        {"account_key": "acct-a", "txn_id": "zero", "amount_val": 0},
        {"account_key": "acct-a", "txn_id": "known", "amount_val": 10},
    ]

    ordered = repository._trace_seed_rows(rows, "account", "acct-a")

    assert [row["txn_id"] for row in ordered] == ["known", "zero", "missing"]


def test_transaction_normalization_carries_a_trace_only_nullable_amount() -> None:
    repository = _repository()
    rows = [
        {"account_key": "acct-a", "txn_id": "missing", "amount_val": None, "balance_val": None},
        {"account_key": "acct-a", "txn_id": "invalid", "amount_val": "bad", "balance_val": "bad"},
        {"account_key": "acct-a", "txn_id": "zero", "amount_val": 0, "balance_val": 0},
    ]

    normalized = repository._normalize_transaction_query_rows("case-a", rows)

    assert [item["amount_val"] for item in normalized] == [None, None, 0.0]
    assert [item["trace_amount_val"] for item in normalized] == [None, None, 0.0]
    assert [item["balance_val"] for item in normalized] == [None, None, 0.0]
    assert [item["currency"] for item in normalized] == [None, None, None]


def test_numeric_sql_expression_does_not_substitute_zero_for_a_missing_column() -> None:
    repository = _repository()

    assert repository._first_num_expr(set(), "t", ("amount",)) == "NULL"
    expression = repository._first_num_expr({"amount"}, "t", ("amount",))
    assert 'WHEN NULLIF(TRIM(CAST(t."amount" AS VARCHAR)), \'\') IS NOT NULL' in expression
    assert "isfinite" in expression
    assert ", 0" not in expression


def test_transaction_evidence_ref_is_not_minted_for_an_unresolved_amount() -> None:
    repository = _repository()
    repository._query_dicts = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("unresolved transaction must not query or persist evidence")
    )
    title, snippet, payload = repository._txn_evidence_payload(
        {
            "txn_id": "txn-missing",
            "account_key": "acct-a",
            "amount_val": None,
            "balance_val": "invalid",
        }
    )

    assert "0.00" not in snippet
    assert payload["amount"] is None
    assert payload["balance"] is None
    assert payload["amount_status"] == "unresolved"
    assert repository._upsert_evidence_ref(
        object(),
        case_id="case-a",
        evidence_type="txn",
        ref_table="fc_transaction_norm",
        ref_pk="txn-missing",
        title=title,
        snippet=snippet,
        payload=payload,
    ) == ""


def test_rule_hit_with_unresolved_score_is_not_persisted_or_signed() -> None:
    repository = _repository()
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._rebuild_rule_hit_without_case = lambda *_args, **_kwargs: None  # type: ignore[method-assign]
    repository._rebuild_evidence_ref_without_rule_hit_refs = lambda *_args, **_kwargs: None  # type: ignore[method-assign]
    repository._upsert_evidence_ref = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("unresolved rule score must not mint evidence")
    )
    engine = _NoopEngine()

    assert repository._persist_rule_hits(
        engine,
        "case-a",
        [],
        [{"rule_hit_id": "hit-a", "score": None}],
    ) == []


def test_complete_amount_sum_distinguishes_empty_unknown_and_explicit_zero() -> None:
    assert _complete_amount_sum([]) is None
    assert _complete_amount_sum([None]) is None
    assert _complete_amount_sum(["invalid"]) is None
    assert _complete_amount_sum([float("nan")]) is None
    assert _complete_amount_sum([False]) is None
    assert _complete_amount_sum([0]) == 0.0
    assert _complete_amount_sum([0, 12.5]) == 12.5


@pytest.mark.parametrize("value", [None, "", "invalid", float("nan"), float("inf"), True])
def test_rank_amount_facts_keep_partial_amounts_unresolved(value: object) -> None:
    facts = _complete_rank_amount_facts(
        {
            "txn_count": 2,
            "in_txn_count": 1,
            "out_txn_count": 1,
            "in_amount_present_count": 0,
            "out_amount_present_count": 1,
            "amount_present_count": 1,
            "inflow_total": value,
            "outflow_total": 0,
            "max_single_amount": 0,
        }
    )

    assert facts["inflow_total"] is None
    assert facts["outflow_total"] == 0
    assert facts["turnover_total"] is None
    assert facts["net_flow"] is None
    assert facts["max_single_amount"] is None
    assert facts["amount_coverage_status"] == "partial"
    assert _sort_rank_rows([facts], metric="turnover", tie_field="name") == []


def test_rank_amount_facts_preserve_explicit_zero_and_allow_known_zero_ranking() -> None:
    facts = {
        "name": "显式零值",
        **_complete_rank_amount_facts(
            {
                "txn_count": 2,
                "in_txn_count": 1,
                "out_txn_count": 1,
                "in_amount_present_count": 1,
                "out_amount_present_count": 1,
                "amount_present_count": 2,
                "inflow_total": 0,
                "outflow_total": 0,
                "max_single_amount": 0,
            }
        ),
    }

    assert facts["inflow_total"] == 0
    assert facts["outflow_total"] == 0
    assert facts["turnover_total"] == 0
    assert facts["net_flow"] == 0
    assert facts["max_single_amount"] == 0
    assert facts["amount_coverage_status"] == "complete"
    assert _sort_rank_rows([facts], metric="turnover", tie_field="name")[0]["rank"] == 1


def test_rule_input_coverage_distinguishes_unknown_from_explicit_zero() -> None:
    complete = _rule_input_coverage(
        [
            {
                "txn_id": "txn-zero",
                "source_record_id": "row:1",
                "account_key": "acct-a",
                "direction_norm": "out",
                "amount_val": 0,
                "txn_ts": datetime(2026, 7, 20, 9, 0, 0),
            }
        ]
    )
    partial = _rule_input_coverage(
        [
            {
                "txn_id": "txn-missing",
                "source_record_id": "row:2",
                "account_key": "acct-a",
                "direction_norm": "out",
                "amount_val": None,
                "txn_ts": datetime(2026, 7, 20, 9, 0, 0),
            }
        ]
    )

    assert complete == {
        "status": "complete",
        "txn_count": 1,
        "complete_txn_count": 1,
        "unresolved_txn_count": 0,
        "duplicate_source_record_count": 0,
        "duplicate_txn_id_count": 0,
    }
    assert partial == {
        "status": "partial",
        "txn_count": 1,
        "complete_txn_count": 0,
        "unresolved_txn_count": 1,
        "duplicate_source_record_count": 0,
        "duplicate_txn_id_count": 0,
    }


@pytest.mark.parametrize(
    ("overrides", "duplicate_source_count", "duplicate_txn_count"),
    [
        ({"source_record_id": ""}, 0, 0),
        ({"txn_id": ""}, 0, 0),
        ({"source_record_id": "row:1", "txn_id": "txn-2"}, 1, 0),
        ({"source_record_id": "row:2", "txn_id": "txn-1"}, 0, 1),
    ],
)
def test_rule_input_coverage_rejects_missing_or_duplicated_physical_identity(
    overrides: dict[str, object],
    duplicate_source_count: int,
    duplicate_txn_count: int,
) -> None:
    rows = [
        {
            "txn_id": "txn-1",
            "source_record_id": "row:1",
            "account_key": "acct-a",
            "direction_norm": "out",
            "amount_val": 1,
            "txn_ts": datetime(2026, 7, 20, 9, 0, 0),
        },
        {
            "txn_id": "txn-2",
            "source_record_id": "row:2",
            "account_key": "acct-a",
            "direction_norm": "in",
            "amount_val": 2,
            "txn_ts": datetime(2026, 7, 20, 10, 0, 0),
            **overrides,
        },
    ]

    coverage = _rule_input_coverage(rows)

    assert coverage["status"] == "partial"
    assert coverage["duplicate_source_record_count"] == duplicate_source_count
    assert coverage["duplicate_txn_id_count"] == duplicate_txn_count


@pytest.mark.parametrize(
    ("row_id", "txn_id", "expected_source_record_id", "expected_status"),
    [
        (7, "", "row:7", "complete"),
        (None, "txn-a", "txn:txn-a", "complete"),
        (None, "", "", "partial"),
    ],
)
def test_raw_transaction_loader_preserves_physical_source_identity(
    row_id: object,
    txn_id: str,
    expected_source_record_id: str,
    expected_status: str,
) -> None:
    repository = _repository()
    repository._table_exists = lambda *_args: True  # type: ignore[method-assign]
    repository._table_columns = lambda *_args: {  # type: ignore[method-assign]
        "id",
        "acct_no",
        "txn_id",
        "txn_time",
        "amount",
        "dc_flag",
        "clean_invalid",
        "clean_failed",
        "clean_reversal",
    }
    repository._txn_cleaning_authority_ready = lambda *_args: True  # type: ignore[method-assign]
    repository._query_dicts = lambda *_args, **_kwargs: [  # type: ignore[method-assign]
        {
            "account_key": "acct-a",
            "account_name": "甲",
            "txn_id": txn_id,
            "row_id": row_id,
            "txn_time": "2026-07-20 09:00:00",
            "txn_ts_val": datetime(2026, 7, 20, 9, 0, 0),
            "amount_val": 0,
            "direction_raw": "出",
        }
    ]

    rows = repository._load_transaction_rows(_RefreshTransactionEngine(), "case-a")

    assert rows[0]["source_record_id"] == expected_source_record_id
    assert rows[0]["source_txn_id"] == txn_id
    assert _rule_input_coverage(rows)["status"] == expected_status


@pytest.mark.parametrize(
    ("source_count", "expected_status", "expected_blocker"),
    [
        (None, "source_unavailable", "transaction_source_unavailable"),
        (0, "empty_unverified", "transaction_source_empty_unverified"),
    ],
)
def test_refresh_source_boundary_stops_before_every_write(
    source_count: int | None,
    expected_status: str,
    expected_blocker: str,
) -> None:
    repository = _repository()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository._analysis_refresh_txn_count = lambda _case_id: source_count  # type: ignore[method-assign]

    def poison(*_args, **_kwargs):
        raise AssertionError("source boundary reached a write or materializer")

    repository.sync_case_baseline = poison  # type: ignore[method-assign]
    repository.open_case_engine = poison  # type: ignore[method-assign]
    repository._load_rule_params_for_native = poison  # type: ignore[method-assign]
    repository._prepare_rule_txn_index_native = poison  # type: ignore[method-assign]
    repository._prepare_rule_pattern_index_native = poison  # type: ignore[method-assign]
    repository._daily_agg = type("_DailyAgg", (), {"ensure_materialized": poison})()

    result = repository.refresh_case_analysis("case-a")

    assert result["status"] == expected_status
    assert result["blocker"] == expected_blocker
    assert result["refreshed"] == []
    assert set(result["stats"].values()) == {None}


def test_analysis_refresh_source_probe_is_read_only_and_closes_engine() -> None:
    repository = _repository()
    engine = _RefreshTransactionEngine()
    open_calls: list[tuple[str, dict[str, object]]] = []

    def open_engine(case_id: str, **kwargs):
        open_calls.append((case_id, kwargs))
        return engine

    repository.open_case_engine = open_engine  # type: ignore[method-assign]
    repository._analysis_refresh_txn_count_from_engine = lambda *_args: 7  # type: ignore[method-assign]

    assert repository._analysis_refresh_txn_count("case-a") == 7
    assert open_calls == [("case-a", {"read_only": True})]
    assert engine.closed is True


@pytest.mark.parametrize(
    "rows",
    [
        None,
        [],
        [(1,), (1,)],
        [(1, 2)],
        [("1",)],
        [(True,)],
        [(1.0,)],
        [(None,)],
        [(-1,)],
        [((1 << 53),)],
    ],
)
def test_analysis_refresh_source_count_requires_exact_query_shape_and_integer(rows: object) -> None:
    class _ProbeEngine:
        def query(self, _sql: str, _params=()):
            return rows

    repository = _repository()
    repository._table_exists = lambda *_args: True  # type: ignore[method-assign]
    repository._table_columns = lambda *_args: {  # type: ignore[method-assign]
        "clean_invalid",
        "clean_failed",
        "clean_reversal",
    }
    repository._txn_cleaning_authority_ready = lambda *_args: True  # type: ignore[method-assign]

    assert repository._analysis_refresh_txn_count_from_engine(_ProbeEngine(), "case-a") is None


@pytest.mark.parametrize("failed_stage", ("rule_txn", "rule_pattern"))
def test_large_refresh_stops_at_first_failed_materializer(failed_stage: str) -> None:
    repository = _repository()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository._analysis_refresh_txn_count = lambda _case_id: repository.LARGE_REFRESH_TXN_ROW_LIMIT  # type: ignore[method-assign]
    repository._load_rule_params_for_native = lambda _case_id: {}  # type: ignore[method-assign]
    repository._prepare_rule_txn_index_native = lambda *_args, **_kwargs: failed_stage != "rule_txn"  # type: ignore[method-assign]

    def pattern(*_args, **_kwargs):
        if failed_stage == "rule_txn":
            raise AssertionError("pattern materializer ran after rule transaction failure")
        return False

    def daily(*_args, **_kwargs):
        raise AssertionError("daily materializer ran after an earlier dependency failure")

    repository._prepare_rule_pattern_index_native = pattern  # type: ignore[method-assign]
    repository._daily_agg = type("_DailyAgg", (), {"ensure_materialized": daily})()
    repository.open_case_engine = lambda _case_id: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("failed materializer reached summary database")
    )

    result = repository.refresh_case_analysis("case-a")

    assert result["status"] == "dependency_unavailable"
    assert result["blocker"] == "analysis_materialization_dependency_unavailable"
    assert result["refreshed"] == []


class _RefreshTransactionEngine:
    def __init__(
        self,
        persisted_counts: tuple[object, ...] = (0, 1, 1, 1, 0, 0, 1, 1),
        rule_rows: list[tuple[object, ...]] | None = None,
        registry_rows: list[tuple[object, ...]] | None = None,
    ) -> None:
        self.commands: list[str] = []
        self.closed = False
        self.persisted_counts = persisted_counts
        self.rule_rows = list(rule_rows or [])
        self.registry_rows = list(registry_rows or [])

    def query(self, sql: str, _params=()):
        normalized = " ".join(sql.split())
        if normalized.startswith("SELECT (SELECT COUNT(1) FROM analysis_rule_hit"):
            return [self.persisted_counts]
        if normalized.startswith("SELECT rule_hit_id, evidence_ids_json"):
            return self.rule_rows
        if normalized.startswith("SELECT evidence_id, ref_table, ref_pk"):
            return self.registry_rows
        return []

    def execute(self, sql: str, _params=()) -> None:
        self.commands.append(sql.strip())

    def executemany(self, _sql: str, _rows) -> None:
        return None

    def close(self) -> None:
        self.closed = True


def _complete_refresh_row() -> dict[str, object]:
    return {
        "txn_id": "txn-1",
        "source_record_id": "row:1",
        "source_txn_id": "txn-1",
        "account_key": "acct-a",
        "direction_norm": "out",
        "amount_val": 0,
        "txn_ts": datetime(2026, 7, 20, 9, 0, 0),
    }


@pytest.mark.parametrize(
    ("current_count", "rows", "expected_blocker"),
    [
        (2, [_complete_refresh_row()], "transaction_count_mismatch"),
        (1, [{**_complete_refresh_row(), "amount_val": None}], "rule_input_coverage_unresolved"),
        (1, [{**_complete_refresh_row(), "source_record_id": ""}], "rule_input_coverage_unresolved"),
    ],
)
def test_refresh_partial_input_rolls_back_before_schema_or_fact_writes(
    current_count: int,
    rows: list[dict[str, object]],
    expected_blocker: str,
) -> None:
    repository = _repository()
    engine = _RefreshTransactionEngine()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository._analysis_refresh_txn_count = lambda _case_id: 1  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: engine  # type: ignore[method-assign]
    repository._analysis_refresh_txn_count_from_engine = lambda *_args: current_count  # type: ignore[method-assign]
    repository._load_transaction_rows = lambda *_args, **_kwargs: rows  # type: ignore[method-assign]

    def poison(*_args, **_kwargs):
        raise AssertionError("partial refresh reached a factual write")

    repository.ensure_analysis_schema = poison  # type: ignore[method-assign]
    repository._ensure_rule_hits = poison  # type: ignore[method-assign]
    repository._ensure_materialized_graph = poison  # type: ignore[method-assign]
    repository._append_query_log = poison  # type: ignore[method-assign]

    result = repository.refresh_case_analysis("case-a")

    assert result["status"] == "partial"
    assert result["blocker"] == expected_blocker
    assert result["refreshed"] == []
    assert engine.commands == ["BEGIN TRANSACTION", "ROLLBACK"]
    assert engine.closed is True


def test_refresh_complete_small_case_commits_once_and_failure_rolls_back() -> None:
    def configured(graph_failure: bool = False):
        repository = _repository()
        engine = _RefreshTransactionEngine()
        repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
        repository._analysis_refresh_txn_count = lambda _case_id: 1  # type: ignore[method-assign]
        repository.open_case_engine = lambda _case_id: engine  # type: ignore[method-assign]
        repository._analysis_refresh_txn_count_from_engine = lambda *_args: 1  # type: ignore[method-assign]
        repository._load_transaction_rows = lambda *_args, **_kwargs: [_complete_refresh_row()]  # type: ignore[method-assign]
        repository._table_exists = lambda *_args, **_kwargs: True  # type: ignore[method-assign]
        repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
        repository._ensure_rule_hits = lambda *_args, **_kwargs: []  # type: ignore[method-assign]
        if graph_failure:
            repository._ensure_materialized_graph = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
                RuntimeError("graph failed")
            )
        else:
            repository._ensure_materialized_graph = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
                "node_count": 1,
                "edge_count": 1,
                "txn_count": 1,
                "account_node_count": 1,
                "unresolved_amount_edge_count": 0,
            }
        repository._append_query_log = lambda *_args, **_kwargs: "query-a"  # type: ignore[method-assign]
        return repository, engine

    repository, engine = configured()
    result = repository.refresh_case_analysis("case-a")
    assert result["status"] == "succeeded"
    assert engine.commands == ["BEGIN TRANSACTION", "COMMIT"]

    repository, engine = configured(graph_failure=True)
    with pytest.raises(RuntimeError, match="graph failed"):
        repository.refresh_case_analysis("case-a")
    assert engine.commands == ["BEGIN TRANSACTION", "ROLLBACK"]


def test_well_shaped_graph_stats_without_persisted_rows_cannot_commit() -> None:
    repository = _repository()
    engine = _RefreshTransactionEngine(persisted_counts=(0, 0, 0, 0, 0, 0, 0, 0))
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository._analysis_refresh_txn_count = lambda _case_id: 1  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: engine  # type: ignore[method-assign]
    repository._analysis_refresh_txn_count_from_engine = lambda *_args: 1  # type: ignore[method-assign]
    repository._load_transaction_rows = lambda *_args, **_kwargs: [_complete_refresh_row()]  # type: ignore[method-assign]
    repository._table_exists = lambda *_args, **_kwargs: True  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._ensure_rule_hits = lambda *_args, **_kwargs: []  # type: ignore[method-assign]
    repository._ensure_materialized_graph = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "node_count": 1,
        "edge_count": 1,
        "txn_count": 1,
        "account_node_count": 1,
        "unresolved_amount_edge_count": 0,
    }
    repository._append_query_log = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("unpersisted graph reached query-log publication")
    )

    result = repository.refresh_case_analysis("case-a")

    assert result["status"] == "partial"
    assert result["blocker"] == "analysis_graph_contract_invalid"
    assert result["refreshed"] == []
    assert set(result["stats"].values()) == {None}
    assert engine.commands == ["BEGIN TRANSACTION", "ROLLBACK"]


@pytest.mark.parametrize(
    ("evidence_ids_json", "registry_rows", "expected_status"),
    [
        ("[]", [], "partial"),
        ('["ev-missing"]', [], "partial"),
        ('["ev-txn"]', [("ev-txn", "fc_transaction_norm", "txn-1")], "partial"),
        ('["ev-rule"]', [("ev-rule", "analysis_rule_hit", "hit-1")], "succeeded"),
    ],
)
def test_small_refresh_requires_host_registry_rule_evidence_before_commit(
    evidence_ids_json: str,
    registry_rows: list[tuple[object, ...]],
    expected_status: str,
) -> None:
    repository = _repository()
    engine = _RefreshTransactionEngine(
        persisted_counts=(1, 1, 1, 1, 0, 0, 1, 1),
        rule_rows=[("hit-1", evidence_ids_json)],
        registry_rows=registry_rows,
    )
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository._analysis_refresh_txn_count = lambda _case_id: 1  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: engine  # type: ignore[method-assign]
    repository._analysis_refresh_txn_count_from_engine = lambda *_args: 1  # type: ignore[method-assign]
    repository._load_transaction_rows = lambda *_args, **_kwargs: [_complete_refresh_row()]  # type: ignore[method-assign]
    repository._table_exists = lambda *_args, **_kwargs: True  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._ensure_rule_hits = lambda *_args, **_kwargs: [{"rule_hit_id": "hit-1"}]  # type: ignore[method-assign]
    repository._ensure_materialized_graph = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "node_count": 1,
        "edge_count": 1,
        "txn_count": 1,
        "account_node_count": 1,
        "unresolved_amount_edge_count": 0,
    }
    log_calls: list[object] = []
    repository._append_query_log = lambda *_args, **_kwargs: log_calls.append(object()) or "query-1"  # type: ignore[method-assign]

    result = repository.refresh_case_analysis("case-a")

    assert result["status"] == expected_status
    if expected_status == "succeeded":
        assert engine.commands == ["BEGIN TRANSACTION", "COMMIT"]
        assert len(log_calls) == 1
    else:
        assert result["blocker"] == "analysis_graph_contract_invalid"
        assert engine.commands == ["BEGIN TRANSACTION", "ROLLBACK"]
        assert log_calls == []


@pytest.mark.parametrize(
    "graph_stats",
    [
        {"node_count": 1, "edge_count": 0},
        {
            "node_count": 1,
            "edge_count": 0,
            "txn_count": 1,
            "account_node_count": 1,
            "unresolved_amount_edge_count": 0,
            "unexpected": 0,
        },
        {
            "node_count": True,
            "edge_count": 0,
            "txn_count": 1,
            "account_node_count": 1,
            "unresolved_amount_edge_count": 0,
        },
        {
            "node_count": 1,
            "edge_count": 0,
            "txn_count": "1",
            "account_node_count": 1,
            "unresolved_amount_edge_count": 0,
        },
        {
            "node_count": 1,
            "edge_count": -1,
            "txn_count": 1,
            "account_node_count": 1,
            "unresolved_amount_edge_count": 0,
        },
        {
            "node_count": 1,
            "edge_count": 0,
            "txn_count": 2,
            "account_node_count": 1,
            "unresolved_amount_edge_count": 0,
        },
        {
            "node_count": 1,
            "edge_count": 0,
            "txn_count": 1,
            "account_node_count": 0,
            "unresolved_amount_edge_count": 0,
        },
        {
            "node_count": 1,
            "edge_count": 0,
            "txn_count": 1,
            "account_node_count": 2,
            "unresolved_amount_edge_count": 0,
        },
        {
            "node_count": 1,
            "edge_count": 0,
            "txn_count": 1,
            "account_node_count": 1,
            "unresolved_amount_edge_count": 1,
        },
    ],
)
def test_refresh_malformed_graph_stats_roll_back_without_query_log(
    graph_stats: dict[str, object],
) -> None:
    repository = _repository()
    engine = _RefreshTransactionEngine()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository._analysis_refresh_txn_count = lambda _case_id: 1  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: engine  # type: ignore[method-assign]
    repository._analysis_refresh_txn_count_from_engine = lambda *_args: 1  # type: ignore[method-assign]
    repository._load_transaction_rows = lambda *_args, **_kwargs: [_complete_refresh_row()]  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._ensure_rule_hits = lambda *_args, **_kwargs: []  # type: ignore[method-assign]
    repository._ensure_materialized_graph = lambda *_args, **_kwargs: graph_stats  # type: ignore[method-assign]
    repository._append_query_log = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("invalid graph stats reached query-log publication")
    )

    result = repository.refresh_case_analysis("case-a")

    assert result["status"] == "partial"
    assert result["blocker"] == "analysis_graph_contract_invalid"
    assert result["refreshed"] == []
    assert set(result["stats"].values()) == {None}
    assert engine.commands == ["BEGIN TRANSACTION", "ROLLBACK"]


def test_refresh_rollback_failure_never_returns_boundary_or_success() -> None:
    class _RollbackFailureEngine(_RefreshTransactionEngine):
        def execute(self, sql: str, _params=()) -> None:
            super().execute(sql, _params)
            if sql.strip().upper() == "ROLLBACK":
                raise RuntimeError("rollback unavailable")

    repository = _repository()
    engine = _RollbackFailureEngine()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository._analysis_refresh_txn_count = lambda _case_id: 1  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: engine  # type: ignore[method-assign]
    repository._analysis_refresh_txn_count_from_engine = lambda *_args: 2  # type: ignore[method-assign]

    with pytest.raises(RuntimeError, match="^analysis_refresh_rollback_failed$"):
        repository.refresh_case_analysis("case-a")

    assert engine.commands == ["BEGIN TRANSACTION", "ROLLBACK"]
    assert engine.closed is True


def test_refresh_failed_after_fact_write_is_atomic(tmp_path) -> None:
    db_path = tmp_path / "refresh-atomic.duckdb"
    setup_engine = DuckDBEngine(db_path)
    try:
        setup_engine.execute("CREATE TABLE refresh_marker(value TEXT)")
    finally:
        setup_engine.close()

    repository = _repository()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository._analysis_refresh_txn_count = lambda _case_id: 1  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: DuckDBEngine(db_path)  # type: ignore[method-assign]
    repository._analysis_refresh_txn_count_from_engine = lambda *_args: 1  # type: ignore[method-assign]
    repository._load_transaction_rows = lambda *_args, **_kwargs: [_complete_refresh_row()]  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]

    def write_then_return(engine: DuckDBEngine, *_args, **_kwargs):
        engine.execute("INSERT INTO refresh_marker VALUES ('must-rollback')")
        return []

    repository._ensure_rule_hits = write_then_return  # type: ignore[method-assign]
    repository._ensure_materialized_graph = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        RuntimeError("graph failed after write")
    )

    with pytest.raises(RuntimeError, match="graph failed after write"):
        repository.refresh_case_analysis("case-a")

    verify_engine = DuckDBEngine(db_path, read_only=True)
    try:
        assert verify_engine.query("SELECT COUNT(1) FROM refresh_marker") == [(0,)]
    finally:
        verify_engine.close()


def test_materialized_graph_explicit_empty_never_reloads_transactions() -> None:
    repository = _repository()
    repository._load_transaction_rows = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("explicit empty rows were treated as absent")
    )
    repository._reset_entity_graph_tables = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("invalid graph input reached a factual write")
    )

    with pytest.raises(AnalysisGraphContractError, match="analysis_graph_contract_invalid"):
        repository._ensure_materialized_graph(
            _RefreshTransactionEngine(),
            "case-a",
            [],
        )


class _ScopeStatsEngine:
    def __init__(self, rows: list[tuple[object, ...]]) -> None:
        self.rows = rows
        self.sql = ""

    def query(self, sql: str, _params=()):
        self.sql = sql
        return self.rows


def test_materialized_scope_stats_missing_query_row_is_unresolved_not_zero() -> None:
    engine = _ScopeStatsEngine([])

    result = _repository()._scope_stats_from_materialized(engine, where_sql="1=1", params=[])

    assert result["txn_count"] is None
    assert result["account_count"] is None
    assert result["in_amount"] is None
    assert result["out_amount"] is None
    assert result["turnover_total"] is None
    assert result["field_coverage"]["cash_flag"] is None
    assert result["coverage_status"] == "unresolved"
    assert "COALESCE(amount, 0)" not in engine.sql


def test_materialized_scope_stats_rejects_partial_amount_sum_but_preserves_explicit_zero() -> None:
    engine = _ScopeStatsEngine(
        [
            (
                2,
                1,
                1,
                "甲",
                "2026-07-20 09:00:00",
                "2026-07-20 10:00:00",
                None,
                0,
                1,
                1,
                2,
                0,
                None,
                0,
                0,
                0,
                0,
                0,
                0,
                0,
                0,
                0,
                0,
                1,
            )
        ]
    )

    result = _repository()._scope_stats_from_materialized(engine, where_sql="1=1", params=[])

    assert result["in_amount"] is None
    assert result["out_amount"] == 0
    assert result["turnover_total"] is None
    assert result["in_amount_present_count"] == 0
    assert result["out_amount_present_count"] == 1
    assert result["field_coverage"]["counterparty"] is None
    assert result["field_coverage"]["branch_name"] == 0
    assert result["coverage_status"] == "partial"


@pytest.mark.parametrize("value", [None, "", "invalid", float("nan"), float("inf"), False])
def test_session_data_binding_keeps_unknown_factual_stats_unresolved(value: object) -> None:
    binding = _repository()._normalize_session_data_binding(
        {
            "bound_source_revision": value,
            "scope_stats": {
                "txn_count": value,
                "account_count": value,
                "in_amount": value,
                "out_amount": value,
            },
        }
    )

    assert binding["bound_source_revision"] is None
    assert binding["scope_stats"]["txn_count"] is None
    assert binding["scope_stats"]["account_count"] is None
    assert binding["scope_stats"]["in_amount"] is None
    assert binding["scope_stats"]["out_amount"] is None


def test_session_data_binding_preserves_explicit_zero_stats() -> None:
    binding = _repository()._normalize_session_data_binding(
        {
            "bound_source_revision": 0,
            "scope_stats": {
                "txn_count": 0,
                "account_count": 0,
                "in_amount": 0,
                "out_amount": 0,
            },
        }
    )

    assert binding["bound_source_revision"] == 0
    assert binding["scope_stats"]["txn_count"] == 0
    assert binding["scope_stats"]["account_count"] == 0
    assert binding["scope_stats"]["in_amount"] == 0
    assert binding["scope_stats"]["out_amount"] == 0


@pytest.mark.parametrize(("value", "expected"), [(None, True), ("invalid", True), (False, False), (True, True)])
def test_memory_note_revalidation_invalid_state_fails_safe(
    value: object,
    expected: bool,
) -> None:
    note = _repository()._normalize_llm_memory_note(
        {
            "memory_type": "project_signal",
            "required_revalidation": value,
        }
    )

    assert note["required_revalidation"] is expected


class _SnapshotEngine:
    def __init__(self) -> None:
        self.sql: list[str] = []

    def query(self, sql: str, _params=()):
        self.sql.append(sql)
        return [(2,)]

    def execute(self, _sql: str, _params=()) -> None:
        return None

    def close(self) -> None:
        return None


def test_account_scope_snapshot_does_not_rank_or_sum_partial_amount_coverage() -> None:
    repository = _repository()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.open_case_engine = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("unverified account snapshot reached the database")
    )

    result = repository.build_scope_account_snapshot("case-a", account_keys=["acct-a"])

    assert result["contract"] == "AnalysisAccountFactBoundaryV1"
    assert result["semantic_status"] == "blocked"
    assert result["blocker"] == "host_evidence_receipt_required"
    assert result["fact_answer_allowed"] is False
    assert result["data"] == {}
    assert result["coverage"]["transaction_rows"] is None


class _RankEngine:
    def __init__(self) -> None:
        self.sql: list[str] = []

    def execute(self, _sql: str, _params=()) -> None:
        return None

    def close(self) -> None:
        return None


def _configure_rank_repository(
    engine: _RankEngine,
    query_rows,
) -> AnalysisRepository:
    repository = _repository()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: None  # type: ignore[method-assign]
    repository._resolve_rank_scope_accounts = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "scope_type": "case",
        "scope_requested": False,
        "selected_accounts": [],
        "warnings": [],
    }
    repository.open_case_engine = lambda _case_id: engine  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._cleanup_temp_scope_lifecycle = lambda *_args, **_kwargs: []  # type: ignore[method-assign]
    repository._daily_agg = type("_DailyAgg", (), {"ensure_materialized": lambda *_args, **_kwargs: True})()
    repository._rank_scope_where = lambda *_args, **_kwargs: ("1=1", [])  # type: ignore[method-assign]
    repository._scope_stats_from_materialized = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "txn_count": 2,
        "account_count": 1,
        "counterparty_count": 1,
        "directed_txn_count": 2,
        "undirected_txn_count": 0,
        "in_amount": None,
        "out_amount": 0,
        "turnover_total": None,
        "coverage_status": "partial",
    }
    repository._query_dicts = query_rows  # type: ignore[method-assign]
    repository._append_query_log = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("partial ranking must not mint query evidence")
    )
    return repository


def test_rank_accounts_excludes_partial_amount_rows_and_does_not_sign_query_evidence() -> None:
    repository = _repository()
    engine = _RankEngine()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: None  # type: ignore[method-assign]
    repository._resolve_rank_scope_accounts = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "scope_type": "case",
        "scope_requested": False,
        "selected_accounts": [],
        "warnings": [],
    }
    repository.open_case_engine = lambda _case_id: engine  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._cleanup_temp_scope_lifecycle = lambda *_args, **_kwargs: []  # type: ignore[method-assign]
    repository._daily_agg = type("_DailyAgg", (), {"ensure_materialized": lambda *_args, **_kwargs: True})()
    repository._rank_scope_where = lambda *_args, **_kwargs: ("1=1", [])  # type: ignore[method-assign]
    repository._scope_stats_from_materialized = lambda *_args, **_kwargs: {  # type: ignore[method-assign]
        "txn_count": 2,
        "account_count": 1,
        "counterparty_count": 1,
        "in_amount": None,
        "out_amount": 0,
        "turnover_total": None,
        "coverage_status": "partial",
    }

    def _query_dicts(_engine, sql: str, _params=()):
        engine.sql.append(sql)
        return [
            {
                "account_key": "acct-a",
                "txn_count": 2,
                "in_txn_count": 1,
                "out_txn_count": 1,
                "in_amount_present_count": 0,
                "out_amount_present_count": 1,
                "amount_present_count": 1,
                "inflow_total": None,
                "outflow_total": 0,
                "max_single_amount": 0,
            }
        ]

    repository._query_dicts = _query_dicts  # type: ignore[method-assign]
    repository._append_query_log = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("partial ranking must not mint query evidence")
    )

    result = repository.rank_accounts("case-a")

    assert result["rankings"] == []
    assert result["query_id"] == ""
    assert result["coverage_status"] == "partial"
    assert result["fact_answer_allowed"] is False
    assert result["group_summary"]["inflow_total"] is None
    assert result["group_summary"]["outflow_total"] == 0
    assert result["group_summary"]["turnover_total"] is None
    assert result["group_summary"]["net_flow"] is None
    assert all("COALESCE(t.amount, 0)" not in sql for sql in engine.sql)


def test_rank_holders_excludes_partial_amount_rows() -> None:
    engine = _RankEngine()

    def _query_rows(_engine, sql: str, _params=()):
        engine.sql.append(sql)
        return [
            {
                "holder_name": "甲",
                "holder_group_key": "甲|无证件号",
                "account_count": 1,
                "account_keys_text": "acct-a",
                "txn_count": 2,
                "in_txn_count": 1,
                "out_txn_count": 1,
                "in_amount_present_count": 0,
                "out_amount_present_count": 1,
                "amount_present_count": 1,
                "inflow_total": None,
                "outflow_total": 0,
                "max_single_amount": 0,
            }
        ]

    result = _configure_rank_repository(engine, _query_rows).rank_holders("case-a")

    assert result["rankings"] == []
    assert result["coverage_status"] == "partial"
    assert result["fact_answer_allowed"] is False
    assert result["query_id"] == ""
    assert all("COALESCE(t.amount, 0)" not in sql for sql in engine.sql)


def test_rank_counterparties_excludes_partial_amount_rows() -> None:
    engine = _RankEngine()

    def _query_rows(_engine, sql: str, _params=()):
        engine.sql.append(sql)
        if "ranked_clusters" in sql:
            return []
        return [
            {
                "ranking_key": "cp-a",
                "representative_counterparty_key": "cp-a",
                "counterparty_keys_text": "cp-a",
                "source_accounts_text": "acct-a",
                "alias_names_text": "乙",
                "account_count": 1,
                "counterparty_account_count": 1,
                "row_count": 2,
                "txn_count": 2,
                "in_txn_count": 1,
                "out_txn_count": 1,
                "in_amount_present_count": 0,
                "out_amount_present_count": 1,
                "amount_present_count": 1,
                "inflow_total": None,
                "outflow_total": 0,
                "max_single_amount": 0,
            }
        ]

    result = _configure_rank_repository(engine, _query_rows).rank_counterparties("case-a")

    assert result["rankings"] == []
    assert result["coverage_status"] == "partial"
    assert result["fact_answer_allowed"] is False
    assert result["query_id"] == ""
    assert all("COALESCE(amount, 0)" not in sql for sql in engine.sql)


def test_compute_rule_hits_short_circuits_when_any_required_fact_is_unknown() -> None:
    repository = _repository()
    repository._build_rule_hit = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("partial rule input must not produce a hit")
    )
    txn_rows = [
        {
            "txn_id": "txn-in",
            "account_key": "acct-a",
            "account_entity_id": "entity-a",
            "direction_norm": "in",
            "amount_val": 100000,
            "txn_ts": datetime(2026, 7, 20, 9, 0, 0),
        },
        {
            "txn_id": "txn-out",
            "account_key": "acct-a",
            "account_entity_id": "entity-a",
            "direction_norm": "out",
            "amount_val": 100000,
            "txn_ts": datetime(2026, 7, 20, 9, 5, 0),
        },
        {
            "txn_id": "txn-unknown",
            "account_key": "acct-a",
            "account_entity_id": "entity-a",
            "direction_norm": "out",
            "amount_val": None,
            "txn_ts": datetime(2026, 7, 20, 9, 6, 0),
        },
    ]

    assert repository._compute_rule_hits("case-a", txn_rows) == []


def test_get_rule_hits_returns_boundary_before_rule_persistence_for_partial_inputs() -> None:
    repository = _repository()
    engine = _RankEngine()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: None  # type: ignore[method-assign]
    repository._load_rule_params_for_native = lambda *_args, **_kwargs: {}  # type: ignore[method-assign]
    repository._prepare_rule_txn_index_native = lambda *_args, **_kwargs: False  # type: ignore[method-assign]
    repository.open_case_engine = lambda *_args, **_kwargs: engine  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._load_transaction_rows = lambda *_args, **_kwargs: [  # type: ignore[method-assign]
        {
            "txn_id": "txn-missing",
            "account_key": "acct-a",
            "direction_norm": "out",
            "amount_val": None,
            "txn_ts": datetime(2026, 7, 20, 9, 0, 0),
        }
    ]
    repository._load_rule_params = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("partial inputs must stop before rule evaluation")
    )
    repository._ensure_rule_hits = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("partial inputs must stop before rule persistence")
    )
    repository._append_query_log = lambda *_args, **_kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        AssertionError("partial inputs must not mint query evidence")
    )

    result = repository.get_rule_hits("case-a")

    assert result["semantic_status"] == "unresolved"
    assert result["blocker"] == "rule_input_coverage_unresolved"
    assert result["fact_answer_allowed"] is False
    assert result["items"] == []
    assert result["evidence_ids"] == []
    assert result["query_id"] == ""


def test_transaction_amount_filter_rejects_invalid_input_and_never_matches_unknown_as_zero() -> None:
    repository = _repository()
    rows = [
        {"txn_id": "unknown", "amount_val": None},
        {"txn_id": "zero", "amount_val": 0},
    ]

    with pytest.raises(ValueError, match="^transaction_min_amount_invalid$"):
        repository._apply_txn_filters(rows, {"min_amount": "invalid"})

    assert [row["txn_id"] for row in repository._apply_txn_filters(rows, {"min_amount": 0})] == ["zero"]


def test_transaction_slice_preserves_unknown_facts_and_empty_direction_sum() -> None:
    repository = _repository()
    engine = _NoopEngine()
    rows = [
        {
            "txn_id": "unknown",
            "account_key": "acct-a",
            "txn_time": "2026-07-20 09:00:00",
            "txn_ts": datetime(2026, 7, 20, 9, 0, 0),
            "direction_norm": "in",
            "amount_val": None,
            "balance_val": None,
            "cash_flag_norm": None,
            "success_flag_norm": None,
        },
        {
            "txn_id": "zero",
            "account_key": "acct-a",
            "txn_time": "2026-07-20 09:05:00",
            "txn_ts": datetime(2026, 7, 20, 9, 5, 0),
            "direction_norm": "out",
            "amount_val": 0,
            "balance_val": 0,
            "cash_flag_norm": False,
            "success_flag_norm": False,
        },
    ]
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: None  # type: ignore[method-assign]
    repository.open_case_engine = lambda *_args, **_kwargs: engine  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._load_transaction_rows = lambda *_args, **_kwargs: rows  # type: ignore[method-assign]
    repository._upsert_evidence_ref = (  # type: ignore[method-assign]
        lambda *_args, **kwargs: "evidence-zero" if kwargs["payload"]["amount"] == 0 else ""
    )
    repository._append_query_log = lambda *_args, **_kwargs: ""  # type: ignore[method-assign]

    result = repository.get_txn_slice("case-a")
    item_by_id = {item["txn_id"]: item for item in result["items"]}

    assert item_by_id["unknown"]["amount"] is None
    assert item_by_id["unknown"]["balance"] is None
    assert item_by_id["unknown"]["is_cash"] is None
    assert item_by_id["unknown"]["is_success"] is None
    assert item_by_id["unknown"]["evidence_id"] == ""
    assert item_by_id["zero"]["amount"] == 0
    assert item_by_id["zero"]["balance"] == 0
    assert item_by_id["zero"]["is_cash"] is False
    assert item_by_id["zero"]["is_success"] is False
    assert result["stats"]["sum_in_amount"] is None
    assert result["stats"]["sum_out_amount"] == 0
    assert result["evidence_ids"] == ["evidence-zero"]


def test_fact_overview_text_never_renders_unknown_amount_or_count_as_zero() -> None:
    direction = _direction_overview_text([{"display_name": "甲", "total_amount": None}], fallback="unused")
    bucket = _bucket_overview_text(
        [{"label": "测试", "txn_count": None, "total_amount": "invalid"}],
        fallback="unused",
    )

    assert direction == "甲（金额未核验）"
    assert bucket == "测试（笔数未核验，金额未核验）"
    assert "0" not in direction
    assert "0" not in bucket


@pytest.mark.parametrize("amount", [None, "", "bad", float("nan")])
def test_trace_path_with_unknown_required_amount_is_not_scored(amount: object) -> None:
    repository = _repository()
    hops = [
        {
            "txn_id": "txn-a",
            "account_key": "acct-a",
            "amount_val": amount,
            "txn_ts": datetime(2026, 7, 20, 9, 0, 0),
        }
    ]

    assert repository._trace_path_features(
        hops,
        txn_rows=hops,
        seed_account="acct-a",
        time_window_sec=1800,
        tolerance_rate=0.03,
        effective_depth=3,
    ) is None


def test_trace_path_explicit_zero_is_distinct_but_not_scored_as_fund_flow() -> None:
    repository = _repository()
    hops = [
        {
            "txn_id": "txn-zero",
            "account_key": "acct-a",
            "amount_val": 0,
            "txn_ts": datetime(2026, 7, 20, 9, 0, 0),
        }
    ]

    features = repository._trace_path_features(
        hops,
        txn_rows=hops,
        seed_account="acct-a",
        time_window_sec=1800,
        tolerance_rate=0.03,
        effective_depth=3,
    )

    assert features == {
        "amount_integrity": "known_zero",
        "root_amount": 0.0,
        "sink_amount": 0.0,
        "path_score": None,
    }


def test_trace_breakdown_and_risk_do_not_default_unknowns() -> None:
    repository = _repository()

    assert repository._trace_merge_support_breakdown(
        [{"txn_id": "support-a", "amount_val": None}],
        target_amount=100,
    ) == []
    assert repository._trace_cycle_risk_level(
        cycle_risk_score=None,
        cycle_detected=False,
        round_trip_detected=False,
        merge_detected=False,
    ) == "unresolved"
    assert repository._trace_cycle_risk_level(
        cycle_risk_score=0,
        cycle_detected=False,
        round_trip_detected=False,
        merge_detected=False,
    ) == "low"

    unresolved = repository._trace_allocation_explanation(
        root_amount=None,
        sink_amount=0,
        amount_gap=0,
        conservation_score=0,
        merge_support_breakdown=[],
        target_amount=0,
    )
    explicit_zero = repository._trace_allocation_explanation(
        root_amount=0,
        sink_amount=0,
        amount_gap=0,
        conservation_score=0,
        merge_support_breakdown=[],
        target_amount=0,
    )
    assert "0.00" not in unresolved
    assert "未核验" in unresolved
    assert "0.00" in explicit_zero


class _TraceEngine:
    def __init__(self) -> None:
        self.executed: list[str] = []

    def execute(self, sql: str, _params=()) -> None:
        self.executed.append(sql)

    def close(self) -> None:
        return None


@pytest.mark.parametrize(
    ("amount", "expected_unknown", "expected_zero"),
    [(None, 1, 0), ("invalid", 1, 0), (0, 0, 1)],
)
def test_run_trace_never_persists_or_signs_unscoreable_amount_paths(
    amount: object,
    expected_unknown: int,
    expected_zero: int,
) -> None:
    repository = _repository()
    engine = _TraceEngine()
    evidence_calls: list[dict] = []
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: None  # type: ignore[method-assign]
    repository.open_case_engine = lambda *_args, **_kwargs: engine  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._query_dicts = lambda *_args, **_kwargs: []  # type: ignore[method-assign]
    repository._load_transaction_rows = lambda *_args, **_kwargs: [  # type: ignore[method-assign]
        {
            "txn_id": "txn-a",
            "account_key": "acct-a",
            "account_entity_id": "entity-a",
            "counterparty_entity_id": "entity-b",
            "counterparty_acct": "acct-b",
            "direction_norm": "in",
            "trace_amount_val": amount,
            "amount_val": 0,
            "txn_time": "2026-07-20 09:00:00",
            "txn_ts": datetime(2026, 7, 20, 9, 0, 0),
        }
    ]
    repository._upsert_evidence_ref = lambda *_args, **kwargs: evidence_calls.append(kwargs) or "evidence"  # type: ignore[method-assign]
    repository._append_query_log = lambda *_args, **_kwargs: ""  # type: ignore[method-assign]

    result = repository.run_trace(
        "case-a",
        seed_type="account",
        seed_value="acct-a",
        depth=3,
        time_window_sec=1800,
        tolerance_rate=0.03,
    )

    assert result["status"] == "unresolved"
    assert result["summary"]["path_count"] == 0
    assert result["summary"]["top_score"] is None
    assert result["stats"]["unresolved_amount_path_count"] == expected_unknown
    assert result["stats"]["explicit_zero_path_count"] == expected_zero
    assert evidence_calls == []
    assert not any("INSERT INTO analysis_trace_path(" in sql for sql in engine.executed)
    assert not any("INSERT INTO analysis_trace_path_hop(" in sql for sql in engine.executed)


class _ReadbackEngine:
    def close(self) -> None:
        return None


def _readback_repository(*, explicit_zero: bool) -> AnalysisRepository:
    repository = _repository()
    engine = _ReadbackEngine()
    detail = {
        "path_rank": 1,
        "compared_path_count": 1,
        "path_mode": "seed_only",
        "conservation_score": 0 if explicit_zero else None,
        "cycle_detected": False if explicit_zero else None,
        "round_trip_detected": False if explicit_zero else None,
        "merge_detected": False if explicit_zero else None,
        "split_detected": False if explicit_zero else None,
        "cycle_risk_score": 0 if explicit_zero else None,
        "cycle_risk_level": "low" if explicit_zero else "",
        "merge_support_ratio": 0 if explicit_zero else None,
        "amount_gap": 0 if explicit_zero else None,
        "root_amount": 0 if explicit_zero else None,
        "sink_amount": 0 if explicit_zero else None,
        "support_txn_ids": ["txn-a"],
        "allocation_explanation": "显式金额 0.00 元。",
        "replay_steps": [
            {
                "amount": 0 if explicit_zero else None,
                "allocated_amount": 0 if explicit_zero else None,
                "amount_share_rate": 0 if explicit_zero else None,
            }
        ],
        "merge_support_breakdown": [],
    }
    path_row = {
        "path_id": "path-a",
        "path_index": 1,
        "path_score": 0 if explicit_zero else None,
        "amount_match_rate": 0 if explicit_zero else "invalid",
        "time_span_sec": 0,
        "summary": "显式金额 0.00 元。" if explicit_zero else "错误默认金额 0.00 元。",
        "evidence_ids_json": json.dumps(["evidence-a"]),
        "detail_json": json.dumps(detail, ensure_ascii=False),
    }
    hop_row = {
        "hop_index": 1,
        "txn_id": "txn-a",
        "src_entity_id": "entity-a",
        "dst_entity_id": "entity-b",
        "txn_time": "2026-07-20 09:00:00",
        "amount": 0 if explicit_zero else None,
        "direction": "in",
    }

    def _query(_engine, sql: str, _params=()):
        if "FROM analysis_trace_run" in sql:
            return [
                {
                    "status": "succeeded",
                    "summary_json": json.dumps(
                        {"summary": {"top_score": 0}, "stats": {"path_score_spread": 0}}
                    ),
                }
            ]
        if "FROM analysis_trace_path_hop" in sql:
            return [hop_row]
        if "FROM analysis_trace_path" in sql:
            return [path_row]
        return []

    repository.open_case_engine = lambda _case_id: engine  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._query_dicts = _query  # type: ignore[method-assign]
    repository._evidence_ref_rows_for_ids = lambda *_args, **_kwargs: []  # type: ignore[method-assign]
    repository._trace_path_mixed_fund_tracking = lambda *_args, **_kwargs: {}  # type: ignore[method-assign]
    return repository


def test_trace_readback_keeps_unknown_fields_unresolved_without_zero_narrative() -> None:
    repository = _readback_repository(explicit_zero=False)

    trace = repository.get_trace("trace-a", case_id="case-a")
    path = repository.get_trace_path("trace-a", "path-a", case_id="case-a")

    assert trace["summary"] == {"coverage_status": "unresolved", "verified_path_count": 0}
    assert trace["top_paths"][0]["path_score"] is None
    assert trace["top_paths"][0]["cycle_detected"] is None
    assert trace["top_paths"][0]["cycle_risk_level"] == "unresolved"
    assert "0.00" not in trace["top_paths"][0]["summary"]
    assert trace["top_paths"][0]["evidence_ids"] == []
    assert path["hops"][0]["amount"] is None
    assert path["cycle_detected"] is None
    assert path["cycle_risk_level"] == "unresolved"
    assert path["merge_support_breakdown"] == []
    assert path["replay_steps"] == []
    assert path["evidence_ids"] == []
    assert "0.00" not in path["summary"]
    assert "0.00" not in path["allocation_explanation"]


def test_trace_readback_preserves_explicit_zero_when_integrity_is_complete() -> None:
    path = _readback_repository(explicit_zero=True).get_trace_path(
        "trace-a",
        "path-a",
        case_id="case-a",
    )

    assert path["path_score"] == 0
    assert path["cycle_risk_score"] == 0
    assert path["cycle_detected"] is False
    assert path["cycle_risk_level"] == "low"
    assert path["hops"][0]["amount"] == 0
    assert path["summary"] == "显式金额 0.00 元。"


def test_trace_path_rank_and_compared_count_are_required_for_complete_readback() -> None:
    row = {"path_index": None, "path_score": 0, "amount_match_rate": 0}
    detail = {
        "path_rank": None,
        "compared_path_count": None,
        "conservation_score": 0,
        "cycle_detected": False,
        "round_trip_detected": False,
        "merge_detected": False,
        "split_detected": False,
        "cycle_risk_score": 0,
        "cycle_risk_level": "low",
        "merge_support_ratio": 0,
        "amount_gap": 0,
        "root_amount": 0,
        "sink_amount": 0,
        "merge_support_breakdown": [],
        "replay_steps": [],
    }

    assert AnalysisRepository._trace_path_detail_is_complete(row, detail) is False


def test_split_trace_summary_does_not_invent_a_fan_out_count() -> None:
    repository = _repository()
    hops = [
        {
            "txn_id": "txn-a",
            "account_key": "acct-a",
            "amount_val": 100,
            "txn_ts": datetime(2026, 7, 20, 9, 0, 0),
        },
        {
            "txn_id": "txn-b",
            "account_key": "acct-a",
            "counterparty_acct": "acct-b",
            "amount_val": 100,
            "txn_ts": datetime(2026, 7, 20, 9, 5, 0),
        },
    ]

    summary = repository._build_trace_path_summary(
        hops,
        {"path_mode": "split_trace", "fan_out_count": None},
    )

    assert summary == repository._trace_private_unresolved_summary()
    assert "2 个" not in summary


def test_project_theme_probe_is_quarantined_before_database_or_query_side_effects() -> None:
    repository = _repository()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: (_ for _ in ()).throw(AssertionError("unexpected baseline"))  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: (_ for _ in ()).throw(AssertionError("unexpected database"))  # type: ignore[method-assign]

    result = repository.fund_analysis_probe(
        "case-a",
        probe_type="project_litigation_asset",
        keywords=["工程", "投标", "利益输送"],
    )

    serialized = json.dumps(result, ensure_ascii=False, sort_keys=True)
    assert result["capability_status"] == "unresolved"
    assert result["findings"] == []
    assert result["query_id"] == ""
    assert result["warnings"][0]["code"] == "TYPED_SOURCE_CAPABILITY_REQUIRED"
    for prohibited in ("工程项目/租赁/保证金", "project_business", "利益输送"):
        assert prohibited not in serialized

    category_sql = _fund_business_category_sql(("summary", "remark"))
    for prohibited in ("project_business", "工程", "项目", "投标", "保证金", "租赁", "劳务", "材料"):
        assert prohibited not in category_sql
    assert _fund_business_category_label("project_business") == "普通转账/未分类"


@pytest.mark.parametrize(
    "probe_type",
    [
        "discovery",
        "cash_breakpoints",
        "financial_product_flows",
        "investigative_patterns",
        "old_thread_regression",
        "report_claim_review",
    ],
)
def test_every_fund_probe_requires_host_evidence_authority_before_database_access(probe_type: str) -> None:
    repository = _repository()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: (_ for _ in ()).throw(AssertionError("unexpected baseline"))  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: (_ for _ in ()).throw(AssertionError("unexpected database"))  # type: ignore[method-assign]

    result = repository.fund_analysis_probe(
        "case-a",
        probe_type=probe_type,
        keywords=["100000.00", "账号6222000000000000", "利益输送"],
    )

    serialized = json.dumps(result, ensure_ascii=False, sort_keys=True)
    assert result["capability_status"] == "unresolved"
    assert result["semantic_status"] == "unresolved"
    assert result["blocker"] == "host_evidence_receipt_required"
    assert result["fact_answer_allowed"] is False
    assert result["findings"] == []
    assert result["evidence_ids"] == []
    assert result["query_id"] == ""
    for prohibited in ("100000.00", "6222000000000000", "利益输送"):
        assert prohibited not in serialized


def test_quarantined_account_fact_entrypoints_contain_no_dead_sql_implementation() -> None:
    probe_source = inspect.getsource(AnalysisRepository.fund_analysis_probe)
    rankings_source = inspect.getsource(AnalysisRepository.build_account_counterparty_rankings)
    profile_source = inspect.getsource(AnalysisRepository.build_account_behavior_profile)

    assert "host_evidence_receipt_required" in probe_source
    assert "project_account_fact_boundary" in rankings_source
    assert "project_account_fact_boundary" in profile_source
    for source in (probe_source, rankings_source, profile_source):
        assert "open_case_engine" not in source
        assert "analysis_txn_detail_idx" not in source
        assert "COALESCE(amount" not in source
    assert not hasattr(AnalysisRepository, "_compute_key_node_feature_rows")
    assert not hasattr(AnalysisRepository, "_compute_signal_feature_rows")


def test_report_workspace_attachment_requires_publication_receipt_before_database_access() -> None:
    repository = _repository()
    repository.case_exists = lambda _case_id: (_ for _ in ()).throw(AssertionError("unexpected case read"))  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: (_ for _ in ()).throw(AssertionError("unexpected database"))  # type: ignore[method-assign]

    with pytest.raises(RuntimeError, match="^report_publication_receipt_required$"):
        repository.attach_report_workspace_projection(
            "case-a",
            report_id="report-a",
            workspace_file={"file_name": "reports/a.md", "version": None},
        )


class _NoopEngine:
    def execute(self, _sql: str, _params=()) -> None:
        return None

    def close(self) -> None:
        return None


class _GraphCaptureEngine(_NoopEngine):
    def __init__(self) -> None:
        self.batches: list[tuple[str, list[tuple[object, ...]]]] = []

    def executemany(self, sql: str, rows) -> None:
        self.batches.append((sql, list(rows)))


@pytest.mark.parametrize(
    ("amount", "expected_amount", "expects_relation_evidence"),
    [(None, None, False), ("invalid", None, False), (0, 0.0, True)],
)
def test_entity_graph_never_materializes_unknown_amount_as_zero_or_evidence(
    amount: object,
    expected_amount: float | None,
    expects_relation_evidence: bool,
) -> None:
    repository = _repository()
    engine = _GraphCaptureEngine()
    repository._reset_entity_graph_tables = lambda *_args, **_kwargs: None  # type: ignore[method-assign]

    txn_row = {
        "txn_id": "txn-a",
        "source_record_id": "row:1",
        "txn_time": "2026-07-20 09:00:00",
        "txn_ts": datetime(2026, 7, 20, 9, 0, 0),
        "account_key": "acct-a",
        "account_name": "甲",
        "account_entity_id": "account-a",
        "device_entity_id": "device-a",
        "device_key": "device-key-a",
        "mac_addr": "00:11:22:33:44:55",
        "direction_norm": "out",
        "amount_val": amount,
    }
    if not expects_relation_evidence:
        with pytest.raises(AnalysisGraphContractError, match="analysis_graph_contract_invalid"):
            repository._ensure_materialized_graph(
                engine,
                "case-a",
                [txn_row],
                force_refresh=True,
            )
        assert engine.batches == []
        return

    stats = repository._ensure_materialized_graph(
        engine,
        "case-a",
        [txn_row],
        force_refresh=True,
    )

    edge_batch = next(rows for sql, rows in engine.batches if "INSERT INTO analysis_relation_edge" in sql)
    assert len(edge_batch) == 1
    edge = edge_batch[0]
    assert edge[6] == 1
    assert edge[7] == expected_amount
    assert (edge[5] is not None) is expects_relation_evidence
    coverage = json.loads(str(edge[10]))["amount_coverage"]
    assert coverage["status"] == ("complete" if expects_relation_evidence else "unresolved")
    evidence_batch = next(rows for sql, rows in engine.batches if "INSERT INTO analysis_evidence_ref" in sql)
    relation_evidence = [row for row in evidence_batch if row[3] == "analysis_relation_edge"]
    assert bool(relation_evidence) is expects_relation_evidence
    assert stats["unresolved_amount_edge_count"] == (0 if expects_relation_evidence else 1)


def test_entity_graph_structural_owner_edge_has_not_applicable_amount_coverage() -> None:
    repository = _repository()
    engine = _GraphCaptureEngine()
    repository._reset_entity_graph_tables = lambda *_args, **_kwargs: None  # type: ignore[method-assign]

    stats = repository._ensure_materialized_graph(
        engine,
        "case-a",
        [
            {
                "txn_id": "txn-a",
                "source_record_id": "row:1",
                "txn_time": "2026-07-20 09:00:00",
                "txn_ts": datetime(2026, 7, 20, 9, 0, 0),
                "account_key": "acct-a",
                "account_name": "甲",
                "account_entity_id": "account-a",
                "opener_id_no": "person-a",
                "person_entity_id": "person-a",
                "counterparty_entity_id": "counterparty-a",
                "direction_norm": "out",
                "amount_val": 0,
            }
        ],
    )

    edge_batch = next(rows for sql, rows in engine.batches if "INSERT INTO analysis_relation_edge" in sql)
    owner_edge = next(row for row in edge_batch if row[4] == "account_to_person_owner")
    assert owner_edge[6] == 0
    assert owner_edge[7] is None
    assert json.loads(str(owner_edge[10]))["amount_coverage"]["status"] == "not_applicable"
    assert stats["unresolved_amount_edge_count"] == 0


@pytest.mark.parametrize(("value", "expected"), [(None, None), ("invalid", None), (False, False), (True, True)])
def test_scratchpad_formalization_status_never_defaults_unknown_to_false(
    value: object,
    expected: bool | None,
) -> None:
    repository = _repository()
    engine = _NoopEngine()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: engine  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._query_dicts = lambda *_args, **_kwargs: []  # type: ignore[method-assign]

    result = repository.write_scratchpad_artifact(
        "case-a",
        artifact={"workspace_id": "workspace-a", "is_formalized": value},
    )

    assert result["is_formalized"] is expected


def _mixed_fund_repository(rows: list[dict]) -> AnalysisRepository:
    repository = _repository()
    engine = _NoopEngine()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: None  # type: ignore[method-assign]
    repository.open_case_engine = lambda *_args, **_kwargs: engine  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._load_transaction_rows = lambda *_args, **_kwargs: rows  # type: ignore[method-assign]
    return repository


@pytest.mark.parametrize(
    ("amount", "balance", "blocker"),
    [
        (None, 100, "seed_amount_or_balance_unresolved"),
        ("invalid", 100, "seed_amount_or_balance_unresolved"),
        (100, float("nan"), "seed_amount_or_balance_unresolved"),
        (0, 100, "seed_amount_not_positive"),
    ],
)
def test_mixed_fund_tracking_never_computes_or_narrates_from_unknown_or_zero_seed(
    amount: object,
    balance: object,
    blocker: str,
) -> None:
    repository = _mixed_fund_repository(
        [
            {
                "txn_id": "seed",
                "account_key": "acct-a",
                "direction_norm": "in",
                "amount_val": amount,
                "balance_val": balance,
            }
        ]
    )

    result = repository.get_seed_mixed_fund_tracking(
        "case-a",
        seed_txn_id="seed",
        candidate_txn_ids=["out"],
    )

    assert result["semantic_status"] == "unresolved"
    assert result["blocker"] == blocker
    assert result["fact_answer_allowed"] is False
    assert result["seed_amount"] is None
    assert result["traceable_residual_amount"] is None
    assert result["evidence_ids"] == []
    assert result["conclusion_text"] == ""


def test_mixed_fund_tracking_preserves_verified_explicit_zero_without_absence_fallback() -> None:
    repository = _mixed_fund_repository(
        [
            {
                "txn_id": "seed",
                "account_key": "acct-a",
                "direction_norm": "in",
                "amount_val": 100,
                "balance_val": 100,
                "txn_time": "2026-07-20 09:00:00",
                "txn_ts": datetime(2026, 7, 20, 9, 0, 0),
            },
            {
                "txn_id": "out",
                "account_key": "acct-a",
                "direction_norm": "out",
                "amount_val": 0,
                "balance_val": 100,
                "txn_time": "2026-07-20 09:05:00",
                "txn_ts": datetime(2026, 7, 20, 9, 5, 0),
            },
        ]
    )

    result = repository.get_seed_mixed_fund_tracking(
        "case-a",
        seed_txn_id="seed",
        candidate_txn_ids=["out"],
    )

    assert result["direct_outflow_amount"] == 0
    assert result["replenishment_amount"] is None
    assert result["latest_balance"] == 100


def test_mixed_fund_helper_cannot_publish_internal_or_caller_supplied_case_conclusion() -> None:
    tracking = {
        "scope_label": "当前案件库可见后续流水口径",
        "cutoff_time": "2026-07-20 09:05:00",
        "traceable_residual_amount": 100,
        "direct_outflow_amount": 0,
        "mixed_residual_amount": 100,
        "consumed_amount": 0,
        "lowest_balance": 100,
        "conclusion_text": "未经宿主证据门验证的案件结论 0.00 元",
    }

    assert build_mixed_fund_conclusion_text(tracking) == ""
    assert enrich_mixed_fund_tracking(tracking)["conclusion_text"] == ""


def test_case_top_mixed_fund_tracking_is_boundary_only_before_database_access() -> None:
    repository = _repository()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: (_ for _ in ()).throw(AssertionError("unexpected baseline"))  # type: ignore[method-assign]
    repository.open_case_engine = lambda _case_id: (_ for _ in ()).throw(AssertionError("unexpected database"))  # type: ignore[method-assign]

    result = repository.get_case_top_mixed_fund_tracking("case-a")

    assert result["semantic_status"] == "unresolved"
    assert result["blocker"] == "host_evidence_receipt_required"
    assert result["traceable_residual_amount"] is None
    assert result["conclusion_text"] == ""


class _NextHopEngine:
    def close(self) -> None:
        return None


def test_next_hop_unknown_seed_amount_returns_boundary_without_cache_or_fact_candidates() -> None:
    repository = _repository()
    engine = _NextHopEngine()
    repository.case_exists = lambda _case_id: True  # type: ignore[method-assign]
    repository.sync_case_baseline = lambda _case_id: None  # type: ignore[method-assign]
    repository.open_case_engine = lambda *_args, **_kwargs: engine  # type: ignore[method-assign]
    repository.ensure_analysis_schema = lambda _engine: None  # type: ignore[method-assign]
    repository._daily_agg = type("_DailyAgg", (), {"ensure_materialized": lambda *_args, **_kwargs: True})()
    repository._table_exists = lambda *_args, **_kwargs: True  # type: ignore[method-assign]

    def _query(_engine, sql: str, _params=()):
        if "WHERE txn_id=?" in sql:
            return [
                {
                    "txn_id": "seed",
                    "acct_key": "acct-a",
                    "cp_key": "acct-b",
                    "dc_val": "出",
                    "txn_ts": datetime(2026, 7, 20, 9, 0, 0),
                    "amount": None,
                }
            ]
        if "WHERE acct_key=?" in sql:
            return []
        raise AssertionError(sql)

    repository._query_dicts = _query  # type: ignore[method-assign]
    result = repository.get_materialized_next_hop_candidates(
        "case-a",
        seed_txn_id="seed",
    )

    assert result["semantic_status"] == "unresolved"
    assert result["blocker"] == "seed_amount_unresolved"
    assert result["fact_answer_allowed"] is False
    assert result["candidates"] == []
    assert result["supporting_txn_ids"] == []


@pytest.mark.parametrize("value", [None, "", "invalid", float("nan"), float("inf"), False])
def test_phase_judgment_confidence_keeps_unknown_values_unresolved(value: object) -> None:
    normalized = _repository()._normalize_llm_phase_judgment(
        {"status": "working_hypothesis", "confidence": {"main_line": value}}
    )

    assert normalized["confidence"]["main_line"] is None


def test_phase_judgment_confidence_preserves_explicit_zero() -> None:
    normalized = _repository()._normalize_llm_phase_judgment(
        {"status": "working_hypothesis", "confidence": {"main_line": 0}}
    )

    assert normalized["confidence"]["main_line"] == 0


@pytest.mark.parametrize(
    ("confidence", "expected"),
    [
        (None, "未核验"),
        ("", "未核验"),
        ("invalid", "未核验"),
        (float("nan"), "未核验"),
        (False, "未核验"),
        (0, "0"),
        (0.456, "0.46"),
    ],
)
def test_hypothesis_confidence_distinguishes_unknown_from_explicit_zero(
    confidence: object,
    expected: str,
) -> None:
    rendered = WorkspaceArtifactRenderer().render_hypotheses(
        [{"title": "测试假设", "confidence": confidence}]
    )

    assert f"- 置信度：{expected}" in rendered
    if expected == "未核验":
        assert "- 置信度：0" not in rendered


@pytest.mark.parametrize(
    ("required_revalidation", "expected"),
    [
        (None, "复核状态未核验；不得作为案件事实"),
        ("invalid", "复核状态未核验；不得作为案件事实"),
        (False, "已明确标记为无需复核的工作指引"),
        (True, "需结合当前证据复核"),
    ],
)
def test_workspace_memory_revalidation_does_not_default_missing_to_false(
    required_revalidation: object,
    expected: str,
) -> None:
    rendered = WorkspaceArtifactRenderer().render_workspace_memory(
        [{"title": "测试记忆", "required_revalidation": required_revalidation}]
    )

    assert f"- 使用要求：{expected}" in rendered
    if required_revalidation is None:
        assert "可作为稳定工作指引" not in rendered


def test_hypothesis_status_does_not_default_missing_to_open() -> None:
    rendered = WorkspaceArtifactRenderer().render_hypotheses([{"title": "测试假设"}])

    assert "- 状态：未核验" in rendered
    assert "- 状态：open" not in rendered
