from __future__ import annotations

import pytest
from pydantic import ValidationError

from app.core.db_engine import DuckDBEngine
from app.repositories.stats_repository import StatsRepository
from app.repositories.txn_daily_materialization_plan_constants import DETAIL_TABLE
from app.schemas.stats import (
    StatsV2AccountTxnRowsDTO,
    StatsV2CaseOverviewDTO,
    StatsV2ChartDashboardDTO,
    StatsV2ChartDetailRowsDTO,
    StatsV2RowsPublicBoundaryDTO,
    StatsV2TreeDTO,
    StatsV2TxnRowsPublicBoundaryDTO,
)


class _RepoShape:
    def __init__(self, *, tables: dict[str, set[str]]) -> None:
        self.tables = tables

    def _table_exists(self, _con, table: str) -> bool:
        return table in self.tables

    def _table_columns(self, _con, table: str) -> set[str]:
        return set(self.tables.get(table, set()))


class _QueryResult:
    def __init__(self, row: tuple[object, ...]) -> None:
        self.row = row
        self.sql = ""

    def query(self, sql: str):
        self.sql = sql
        return [self.row]

    def close(self) -> None:
        return None


class _DailyAgg:
    def __init__(self, ready: bool) -> None:
        self.ready = ready

    def ensure_materialized(self, _case_id: str) -> bool:
        return self.ready


class _ChartRepository(StatsRepository):
    def __init__(self, *, ready: bool, coverage: dict | None = None) -> None:
        self._daily_agg = _DailyAgg(ready)
        self._coverage = coverage

    def open_case_engine(self, _case_id: str, *, read_only: bool):
        assert read_only is True
        return _QueryResult(tuple())

    def _query_case_overview_transactions(self, _con) -> dict:
        assert self._coverage is not None
        return dict(self._coverage)


def _transaction_overview(*, columns: set[str], row: tuple[object, ...]) -> dict:
    repository = _RepoShape(tables={DETAIL_TABLE: columns})
    connection = _QueryResult(row)
    result = StatsRepository._query_case_overview_transactions(repository, connection)
    assert "COALESCE(TRY_CAST(amount AS DOUBLE), 0)" not in connection.sql
    assert "isfinite(TRY_CAST(amount AS DOUBLE))" in connection.sql
    return result


def _dto_payload(**overrides) -> dict:
    payload = {
        "case_id": "case-status",
        "fact_answer_allowed": False,
        "account_status": "unavailable",
        "account_blocker": "account_source_unavailable",
        "account_count": None,
        "personal_account_count": None,
        "corporate_account_count": None,
        "unknown_account_count": None,
        "transaction_status": "unavailable",
        "transaction_blocker": "host_evidence_receipt_required",
        "transaction_count": None,
        "amount_status": "unavailable",
        "amount_blocker": "host_evidence_receipt_required",
        "inflow_amount": None,
        "outflow_amount": None,
        "amount_total_rows": None,
        "amount_present_rows": None,
        "amount_missing_rows": None,
        "amount_parse_failed_rows": None,
        "direction_covered_rows": None,
        "source_table": DETAIL_TABLE,
        "account_source_table": "",
        "source_revision": 1,
        "generated_at": "2026-07-18T00:00:00.000Z",
    }
    payload.update(overrides)
    return payload


def test_case_overview_public_contract_accepts_only_boundary_without_host_receipt() -> None:
    result = StatsV2CaseOverviewDTO(**_dto_payload())
    assert result.fact_answer_allowed is False
    assert result.transaction_count is None
    assert result.inflow_amount is None


def test_case_overview_missing_transaction_source_is_unavailable_not_zero() -> None:
    repository = _RepoShape(tables={})
    result = StatsRepository._query_case_overview_transactions(repository, _QueryResult(tuple()))

    assert result["transaction_status"] == "unavailable"
    assert result["transaction_count"] is None
    assert result["amount_status"] == "unavailable"
    assert result["inflow_amount"] is None
    assert result["outflow_amount"] is None


def test_case_overview_null_plus_value_is_partial_and_cannot_publish_total() -> None:
    result = _transaction_overview(
        columns={"amount", "amount_parse_failed", "dc_val"},
        row=(2, 1, 0, 2, 1, 0, 100.0, None),
    )

    assert result["transaction_status"] == "verified"
    assert result["transaction_count"] == 2
    assert result["amount_status"] == "partial"
    assert result["amount_present_rows"] == 1
    assert result["amount_missing_rows"] == 1
    assert result["inflow_amount"] is None
    assert result["outflow_amount"] is None


def test_case_overview_empty_snapshot_is_not_zero_without_host_no_hit_receipt() -> None:
    result = _transaction_overview(
        columns={"amount", "amount_parse_failed", "dc_val"},
        row=(0, 0, 0, 0, 0, 0, None, None),
    )

    assert result["transaction_status"] == "unavailable"
    assert result["transaction_blocker"] == "empty_scope_requires_host_receipt"
    assert result["transaction_count"] is None
    assert result["amount_status"] == "unavailable"
    assert result["amount_total_rows"] == 0
    assert result["inflow_amount"] is None
    assert result["outflow_amount"] is None


def test_case_overview_preserves_missing_zero_and_nonfinite_amount_states(tmp_path) -> None:
    engine = DuckDBEngine(tmp_path / "stats-overview-amounts.duckdb")
    repository = _RepoShape(
        tables={DETAIL_TABLE: {"amount", "amount_parse_failed", "dc_val"}}
    )
    try:
        engine.execute(
            f"""
            CREATE TABLE {DETAIL_TABLE}(
                amount DOUBLE,
                amount_parse_failed BIGINT,
                dc_val TEXT
            )
            """
        )
        engine.executemany(
            f"INSERT INTO {DETAIL_TABLE} VALUES (?,?,?)",
            [
                (None, 0, "进"),
                (0.0, 0, "出"),
                (float("nan"), 0, "进"),
                (float("inf"), 0, "出"),
            ],
        )

        result = StatsRepository._query_case_overview_transactions(repository, engine)

        assert result["amount_status"] == "partial"
        assert result["amount_total_rows"] == 4
        assert result["amount_present_rows"] == 1
        assert result["amount_missing_rows"] == 3
        assert result["amount_parse_failed_rows"] == 2
        assert result["inflow_amount"] is None
        assert result["outflow_amount"] is None
    finally:
        engine.close()


def test_case_overview_complete_real_zero_and_absent_direction_are_verified_zero(tmp_path) -> None:
    engine = DuckDBEngine(tmp_path / "stats-overview-real-zero.duckdb")
    repository = _RepoShape(
        tables={DETAIL_TABLE: {"amount", "amount_parse_failed", "dc_val"}}
    )
    try:
        engine.execute(
            f"""
            CREATE TABLE {DETAIL_TABLE}(
                amount DOUBLE,
                amount_parse_failed BIGINT,
                dc_val TEXT
            )
            """
        )
        engine.executemany(
            f"INSERT INTO {DETAIL_TABLE} VALUES (?,?,?)",
            [(0.0, 0, "进"), (0.0, 0, "进")],
        )

        result = StatsRepository._query_case_overview_transactions(repository, engine)

        assert result["amount_status"] == "verified"
        assert result["amount_present_rows"] == 2
        assert result["amount_missing_rows"] == 0
        assert result["amount_parse_failed_rows"] == 0
        assert result["inflow_amount"] == 0.0
        assert result["outflow_amount"] == 0.0
    finally:
        engine.close()


@pytest.mark.parametrize("invalid_sum", [None, float("nan"), float("inf")])
def test_case_overview_direction_with_rows_rejects_invalid_sum(invalid_sum: float | None) -> None:
    result = _transaction_overview(
        columns={"amount", "amount_parse_failed", "dc_val"},
        row=(1, 1, 0, 1, 1, 0, invalid_sum, None),
    )

    assert result["amount_status"] == "partial"
    assert result["amount_blocker"] == "amount_aggregate_invalid"
    assert result["inflow_amount"] is None
    assert result["outflow_amount"] is None


@pytest.mark.parametrize("invalid_sum", [0.0, 1.0, float("nan"), float("inf")])
def test_case_overview_absent_direction_requires_null_sum_before_zero_projection(
    invalid_sum: float,
) -> None:
    result = _transaction_overview(
        columns={"amount", "amount_parse_failed", "dc_val"},
        row=(1, 1, 0, 1, 1, 0, 0.0, invalid_sum),
    )

    assert result["amount_status"] == "partial"
    assert result["amount_blocker"] == "amount_aggregate_invalid"
    assert result["inflow_amount"] is None
    assert result["outflow_amount"] is None


@pytest.mark.parametrize(
    "overrides",
    [
        {"transaction_status": "unavailable", "transaction_count": 0},
        {"amount_status": "partial", "inflow_amount": 0.0, "outflow_amount": 0.0},
        {"fact_answer_allowed": True},
        {
            "transaction_status": "verified",
            "transaction_count": 1,
            "amount_status": "verified",
            "inflow_amount": 100.0,
            "outflow_amount": 0.0,
            "amount_total_rows": 1,
            "amount_present_rows": 1,
            "amount_missing_rows": 0,
            "amount_parse_failed_rows": 0,
            "direction_covered_rows": 1,
        },
    ],
)
def test_case_overview_contract_rejects_unverified_or_incomplete_zero_facts(overrides: dict) -> None:
    with pytest.raises(ValidationError):
        StatsV2CaseOverviewDTO(**_dto_payload(**overrides))


def _chart_query(repository: StatsRepository, *, selected: list[str]) -> dict:
    return repository.query_v2_chart_dashboard(
        case_id="case-chart",
        selected_keys=selected,
        date_start="",
        date_end="",
        metric_mode="amount",
        direction_mode="all",
        granularity="day",
        success_filter="all",
        cash_filter="all",
        chart_filters=[],
        panel_views=[],
    )


def test_chart_dashboard_source_unavailable_never_becomes_empty_zero_dashboard() -> None:
    result = _chart_query(_ChartRepository(ready=False), selected=["acct-1"])

    assert result["evidence_status"] == "source_unavailable"
    assert result["fact_answer_allowed"] is False
    assert result["object_summary"]["selected_card_count"] == 1
    assert result["summary"] == {}
    assert all(result[name] == {} for name in ("trend", "counterparties", "structure", "heatmap", "distribution", "flow", "anomaly"))
    StatsV2ChartDashboardDTO(**result)


def test_chart_dashboard_partial_amount_coverage_cannot_publish_subset_total() -> None:
    coverage = {
        "transaction_status": "verified",
        "transaction_blocker": "",
        "amount_status": "partial",
        "amount_blocker": "amount_or_direction_coverage_partial",
        "amount_total_rows": 2,
        "amount_present_rows": 1,
        "amount_missing_rows": 1,
        "amount_parse_failed_rows": 0,
        "direction_covered_rows": 2,
    }
    result = _chart_query(_ChartRepository(ready=True, coverage=coverage), selected=["acct-1"])

    assert result["evidence_status"] == "partial"
    assert result["fact_answer_allowed"] is False
    assert result["coverage"]["amount_missing_rows"] == 1
    assert result["summary"] == {}
    StatsV2ChartDashboardDTO(**result)


def test_chart_dashboard_complete_local_data_still_requires_host_receipt() -> None:
    coverage = {
        "transaction_status": "verified",
        "transaction_blocker": "",
        "amount_status": "verified",
        "amount_blocker": "",
        "amount_total_rows": 2,
        "amount_present_rows": 2,
        "amount_missing_rows": 0,
        "amount_parse_failed_rows": 0,
        "direction_covered_rows": 2,
    }
    result = _chart_query(_ChartRepository(ready=True, coverage=coverage), selected=["acct-1"])

    assert result["evidence_status"] == "blocked"
    assert result["blocker"] == "host_evidence_receipt_required"
    assert result["fact_answer_allowed"] is False
    assert result["summary"] == {}
    StatsV2ChartDashboardDTO(**result)


def test_chart_dashboard_contract_rejects_facts_for_non_verified_status() -> None:
    result = _chart_query(_ChartRepository(ready=False), selected=["acct-1"])
    result["summary"] = {"total_in_amount": 0.0, "txn_total_count": 0}
    with pytest.raises(ValidationError):
        StatsV2ChartDashboardDTO(**result)


def _chart_detail_query(repository: StatsRepository, *, selected: list[str]) -> dict:
    return repository.query_v2_chart_detail_rows(
        case_id="case-chart",
        selected_keys=selected,
        date_start="",
        date_end="",
        metric_mode="amount",
        direction_mode="all",
        granularity="day",
        success_filter="all",
        cash_filter="all",
        chart_filters=[],
        panel_views=[],
        sort_col="txn_time",
        sort_dir="desc",
        page=1,
        limit=200,
        visible_columns=[],
    )


@pytest.mark.parametrize(
    ("ready", "status", "blocker"),
    [
        (False, "source_unavailable", "materialization_unavailable"),
        (True, "blocked", "host_evidence_receipt_required"),
    ],
)
def test_chart_detail_never_turns_unavailable_or_unreceipted_scope_into_empty_zero(
    ready: bool,
    status: str,
    blocker: str,
) -> None:
    result = _chart_detail_query(_ChartRepository(ready=ready), selected=["acct-1"])
    assert result["evidence_status"] == status
    assert result["blocker"] == blocker
    assert result["fact_answer_allowed"] is False
    assert result["rows"] == []
    assert result["total"] is None
    StatsV2ChartDetailRowsDTO(**result)


def test_chart_detail_contract_rejects_unverified_rows_and_zero_total() -> None:
    result = _chart_detail_query(_ChartRepository(ready=False), selected=["acct-1"])
    result["rows"] = [{"amount": 0}]
    result["total"] = 0
    with pytest.raises(ValidationError):
        StatsV2ChartDetailRowsDTO(**result)


@pytest.mark.parametrize(
    ("ready", "status", "blocker"),
    [
        (False, "source_unavailable", "materialization_unavailable"),
        (True, "blocked", "host_evidence_receipt_required"),
    ],
)
def test_account_transaction_rows_are_boundary_only_without_host_receipt(
    ready: bool,
    status: str,
    blocker: str,
) -> None:
    result = _ChartRepository(ready=ready).query_v2_account_txn_rows(
        case_id="case-chart",
        account_key="acct-1",
        date_start="",
        date_end="",
        start_time="",
        end_time="",
        sort_dir="asc",
        limit=200,
    )
    assert result["semantic_status"] == status
    assert result["blocker"] == blocker
    assert result["fact_answer_allowed"] is False
    assert result["rows"] == []
    assert result["done"] is False
    StatsV2AccountTxnRowsDTO(**result)


def test_account_transaction_rows_contract_rejects_unreceipted_zero_row() -> None:
    result = _ChartRepository(ready=False).query_v2_account_txn_rows(
        case_id="case-chart",
        account_key="acct-1",
        date_start="",
        date_end="",
        start_time="",
        end_time="",
        sort_dir="asc",
        limit=200,
    )
    result["rows"] = [{"amount": 0, "dc_flag": "进"}]
    with pytest.raises(ValidationError):
        StatsV2AccountTxnRowsDTO(**result)


@pytest.mark.parametrize(
    ("ready", "status", "blocker"),
    [
        (False, "source_unavailable", "materialization_unavailable"),
        (True, "blocked", "host_evidence_receipt_required"),
    ],
)
def test_tree_unavailable_is_not_no_objects_without_host_receipt(
    ready: bool,
    status: str,
    blocker: str,
) -> None:
    result = _ChartRepository(ready=ready).query_v2_tree(case_id="case-chart", tab="byName")
    assert result["semantic_status"] == status
    assert result["blocker"] == blocker
    assert result["fact_answer_allowed"] is False
    assert result["groups"] == []
    StatsV2TreeDTO(**result)


def test_tree_contract_rejects_unreceipted_account_identity() -> None:
    result = _ChartRepository(ready=True).query_v2_tree(case_id="case-chart", tab="byCard")
    result["groups"] = [{"id": "acct-1", "title": "account", "items": []}]
    with pytest.raises(ValidationError):
        StatsV2TreeDTO(**result)


def test_direct_rows_contract_rejects_self_reported_fact_payload() -> None:
    payload = {
        "contract": "StatsRowsPublicBoundaryV1",
        "semantic_status": "blocked",
        "blocker": "host_evidence_receipt_required",
        "fact_answer_allowed": False,
        "raw_details_exposed": False,
        "rows": [{"account_no": "6222020000000000", "amount": 100}],
        "row_fields": [],
        "status": "controlled_projection_required",
        "total": None,
        "row_summary": {},
    }
    with pytest.raises(ValidationError):
        StatsV2RowsPublicBoundaryDTO(**payload)


def test_direct_txn_contract_rejects_self_reported_fact_payload() -> None:
    payload = {
        "contract": "StatsTxnRowsPublicBoundaryV1",
        "semantic_status": "blocked",
        "blocker": "host_evidence_receipt_required",
        "fact_answer_allowed": False,
        "raw_details_exposed": False,
        "rows": [{"account_no": "6222020000000000", "mac_addr": "AA:BB:CC:DD:EE:FF"}],
        "row_fields": [],
        "done": False,
        "next_cursor": None,
    }
    with pytest.raises(ValidationError):
        StatsV2TxnRowsPublicBoundaryDTO(**payload)
