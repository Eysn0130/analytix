from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from types import SimpleNamespace

import duckdb
import pytest

from app.core.analysis_compute_commands import (
    materialize_txn_daily_args,
    query_chart_dashboard_args,
    query_flow_focus_graph_args,
    query_flow_focus_rows_args,
)
from app.core.analysis_compute_rule_commands import materialize_rule_pattern_index_args
from app.core.fc_import_note import CLEAN_STATS_UNAVAILABLE_NOTE, _append_clean_stats_note
from app.core.fc_import_semantic_filter import build_semantic_row_filter_expr
from app.domain.flow_service import FlowService
from app.repositories import cleaning_sql_counter
from app.repositories.cleaning_run_state import CleaningRunState
from app.repositories.cleaning_summary import build_cleaning_summary
from app.repositories.cleaning_step_catalog import CLEANING_STEP_DEFINITIONS
from app.repositories.flow_repository import _project_node_total_amount, _round2
from app.repositories.txn_daily_aggregate import (
    TxnAmountCoverageIncompleteError,
    TxnDailyAggregateStore,
)


_NONFINITE = (float("nan"), float("inf"), float("-inf"))


def _flag_value(args: list[str], flag: str) -> str:
    return args[args.index(flag) + 1]


def _rule_pattern_command_values() -> dict[str, object]:
    return {
        "case_id": "case-a",
        "db_path": Path("case.duckdb"),
        "param_signature": "rule-pattern-signature-a",
        "round_unit": 10_000.0,
        "round_min_amount": 10_000.0,
        "round_min_count": 3,
        "round_min_total_amount": 50_000.0,
        "small_fast_window_minutes": 60,
        "small_fast_ratio": 0.75,
        "small_fast_min_amount": 1_000.0,
        "small_fast_max_amount": 50_000.0,
        "cash_quick_window_minutes": 60,
        "cash_quick_min_amount": 5_000.0,
        "cash_candidate_window_minutes": 60,
        "cash_candidate_min_amount": 1_000.0,
        "cash_candidate_min_ratio": 0.9,
        "cash_candidate_max_ratio": 1.1,
        "near_threshold_amount": 50_000.0,
        "near_threshold_lower_rate": 0.9,
        "near_threshold_window_minutes": 14_400,
        "near_threshold_min_count": 2,
        "near_threshold_min_total_amount": 90_000.0,
        "repeated_amount_window_minutes": 1_440,
        "repeated_amount_min_amount": 1_000.0,
        "repeated_amount_min_count": 3,
        "repeated_amount_min_total_amount": 10_000.0,
        "threshold_split_window_minutes": 30,
        "threshold_split_amount": 50_000.0,
        "threshold_split_tolerance_rate": 0.05,
        "threshold_split_min_count": 3,
        "high_freq_small_amount_threshold": 5_000.0,
        "high_freq_window_minutes": 30,
        "high_freq_count_threshold": 8,
        "high_freq_min_total_amount": 20_000.0,
        "night_start_hour": 22,
        "night_end_hour": 6,
        "night_min_count": 3,
        "night_min_total_amount": 20_000.0,
        "force": False,
    }


@pytest.mark.parametrize("value", (None, True, "0", -1.0, *_NONFINITE))
def test_rule_pattern_command_rejects_explicit_invalid_numbers_instead_of_defaulting(value: object) -> None:
    values = _rule_pattern_command_values()
    values["round_min_amount"] = value
    with pytest.raises(ValueError, match="^round_min_amount_invalid$"):
        materialize_rule_pattern_index_args(**values)  # type: ignore[arg-type]


def test_rule_pattern_command_preserves_real_zero_and_rejects_inverted_bounds() -> None:
    values = _rule_pattern_command_values()
    values["round_min_amount"] = 0.0
    args = materialize_rule_pattern_index_args(**values)  # type: ignore[arg-type]
    assert _flag_value(args, "--round-min-amount") == "0.0"

    values = _rule_pattern_command_values()
    values["small_fast_min_amount"] = 100.0
    values["small_fast_max_amount"] = 99.0
    with pytest.raises(ValueError, match="^small_fast_max_amount_invalid$"):
        materialize_rule_pattern_index_args(**values)  # type: ignore[arg-type]


@pytest.mark.parametrize("value", (None, True, "0", -1.0, *_NONFINITE))
def test_analysis_compute_thresholds_reject_missing_coercion_and_nonfinite(value: object) -> None:
    with pytest.raises(ValueError, match="^min_amount_invalid$"):
        query_flow_focus_rows_args(
            case_id="case-a",
            db_path=Path("case.duckdb"),
            query_seed_ids=["acct-a"],
            selected_focus_ids=["acct-b"],
            selected_placeholder_kinds=[],
            include_missing_counterparty=False,
            direction="all",
            min_amount=value,  # type: ignore[arg-type]
            date_start="",
            date_end_excl="",
        )

    with pytest.raises(ValueError, match="^large_txn_threshold_invalid$"):
        query_chart_dashboard_args(
            case_id="case-a",
            db_path=Path("case.duckdb"),
            selected_keys=[],
            date_start="",
            date_end="",
            metric_mode="amount",
            direction_mode="all",
            granularity="day",
            success_filter="all",
            cash_filter="all",
            selection_mode="",
            large_txn_threshold=value,  # type: ignore[arg-type]
        )


def test_analysis_compute_thresholds_preserve_real_zero() -> None:
    args = query_flow_focus_rows_args(
        case_id="case-a",
        db_path=Path("case.duckdb"),
        query_seed_ids=["acct-a"],
        selected_focus_ids=["acct-b"],
        selected_placeholder_kinds=[],
        include_missing_counterparty=False,
        direction="all",
        min_amount=0.0,
        date_start="",
        date_end_excl="",
    )
    assert _flag_value(args, "--min-amount") == "0.0"

    graph_args = query_flow_focus_graph_args(
        case_id="case-a",
        db_path=Path("case.duckdb"),
        query_seed_ids=["acct-a"],
        seed_ids=["acct-a"],
        selected_focus_ids=["acct-b"],
        selected_placeholder_kinds=[],
        include_missing_counterparty=False,
        direction="all",
        min_amount=0.0,
        date_start="",
        date_end_excl="",
        depth=1,
        view_mode="relation",
        focus_id_raw="acct-b",
        focus_label="acct-b",
        request_id="request-a",
        source="stats",
        focus_key_type="account",
        expected_total_amount=0.0,
        expected_row_count=0,
    )
    assert _flag_value(graph_args, "--expected-total-amount") == "0.0"
    assert _flag_value(graph_args, "--expected-row-count") == "0"


@pytest.mark.parametrize("value", _NONFINITE)
def test_analysis_compute_expected_total_rejects_nonfinite(value: float) -> None:
    with pytest.raises(ValueError, match="^expected_total_amount_invalid$"):
        query_flow_focus_graph_args(
            case_id="case-a",
            db_path=Path("case.duckdb"),
            query_seed_ids=["acct-a"],
            seed_ids=["acct-a"],
            selected_focus_ids=["acct-b"],
            selected_placeholder_kinds=[],
            include_missing_counterparty=False,
            direction="all",
            min_amount=0.0,
            date_start="",
            date_end_excl="",
            depth=1,
            view_mode="relation",
            focus_id_raw="acct-b",
            focus_label="acct-b",
            request_id="request-a",
            source="stats",
            focus_key_type="account",
            expected_total_amount=value,
            expected_row_count=0,
        )


def test_materialization_source_identity_numbers_are_required_not_defaulted() -> None:
    with pytest.raises(ValueError, match="^source_revision_invalid$"):
        materialize_txn_daily_args(
            case_id="case-a",
            db_path=Path("case.duckdb"),
            source_revision=None,  # type: ignore[arg-type]
            source_snapshot=(0, "", 0),
        )

    args = materialize_txn_daily_args(
        case_id="case-a",
        db_path=Path("case.duckdb"),
        source_revision=1,
        source_snapshot=(0, "", 0),
    )
    assert _flag_value(args, "--source-row-count") == "0"
    assert _flag_value(args, "--source-max-id") == "0"


class _FlowCaptureRepository:
    def __init__(self) -> None:
        self.calls: list[dict] = []

    def build_graph(self, **kwargs):
        self.calls.append(kwargs)
        return {"nodes": [], "edges": []}


@pytest.mark.parametrize("value", (None, True, "0", -1.0, *_NONFINITE))
def test_flow_service_rejects_missing_and_nonfinite_min_amount(value: object) -> None:
    repository = _FlowCaptureRepository()
    service = FlowService(repository=repository)  # type: ignore[arg-type]

    with pytest.raises(ValueError, match="^flow_min_amount_invalid$"):
        service.build_graph(
            case_id="case-a",
            seeds=["acct-a"],
            depth=1,
            direction="both",
            min_amount=value,  # type: ignore[arg-type]
        )

    assert repository.calls == []


@pytest.mark.parametrize("value", (None, *_NONFINITE))
def test_flow_amount_rounding_never_fabricates_zero(value: object) -> None:
    with pytest.raises(
        TxnAmountCoverageIncompleteError,
        match="^transaction_amount_coverage_incomplete$",
    ):
        _round2(value)

    assert _round2(0.0) == 0.0


def test_missing_node_amount_remains_unresolved_while_real_zero_is_preserved() -> None:
    assert _project_node_total_amount({}, "acct-missing") is None
    assert _project_node_total_amount({"acct-zero": 0.0}, "acct-zero") == 0.0


@pytest.mark.parametrize("value", (None, True, "0", -1.0, *_NONFINITE))
def test_txn_daily_query_threshold_fails_closed_before_source_access(value: object) -> None:
    store = object.__new__(TxnDailyAggregateStore)
    with pytest.raises(
        TxnAmountCoverageIncompleteError,
        match="^transaction_amount_coverage_incomplete$",
    ):
        store.query_flow_focus_rows(
            case_id="case-a",
            query_seed_ids=["acct-a"],
            selected_focus_ids=["acct-b"],
            selected_placeholder_kinds=[],
            include_missing_counterparty=False,
            direction="all",
            min_amount=value,  # type: ignore[arg-type]
            date_start=None,
            date_end_excl=None,
        )


@dataclass
class _SemanticSchema:
    table: str
    headers: list[str]
    col_map: dict[str, str]


def test_import_semantic_filter_treats_zero_as_present_and_nonfinite_as_invalid() -> None:
    schema = _SemanticSchema(
        table="fc_transaction",
        headers=["acct", "time", "amount", "feedback"],
        col_map={
            "acct": "acct_no",
            "time": "txn_time",
            "amount": "amount",
            "feedback": "query_feedback_reason",
        },
    )
    expression = build_semantic_row_filter_expr(schema)
    con = duckdb.connect(":memory:")
    try:
        con.execute(
            """
            CREATE TABLE rows(
                acct_no_raw VARCHAR,
                txn_time_raw VARCHAR,
                amount_raw VARCHAR,
                query_feedback_reason_raw VARCHAR
            )
            """
        )
        con.executemany(
            "INSERT INTO rows VALUES (?,?,?,?)",
            [
                ("acct-a", "2026-01-01", "0", "feedback"),
                ("acct-a", "2026-01-01", None, "feedback"),
                ("acct-a", "2026-01-01", "NaN", "feedback"),
                ("acct-a", "2026-01-01", "Infinity", "feedback"),
            ],
        )
        kept = con.execute(f"SELECT amount_raw FROM rows base WHERE {expression}").fetchall()
    finally:
        con.close()

    assert kept == [("0",)]


def test_account_semantic_filter_does_not_collapse_missing_or_nonfinite_balance_to_zero() -> None:
    schema = _SemanticSchema(
        table="fc_account",
        headers=["acct", "balance"],
        col_map={"acct": "acct_no", "balance": "balance"},
    )
    expression = build_semantic_row_filter_expr(schema)
    con = duckdb.connect(":memory:")
    try:
        con.execute("CREATE TABLE rows(acct_no_raw VARCHAR, balance_raw VARCHAR)")
        con.executemany(
            "INSERT INTO rows VALUES (?,?)",
            [
                ("acct-missing", None),
                ("acct-zero", "0"),
                ("acct-nan", "NaN"),
                ("acct-inf", "Infinity"),
                ("acct-real", "1.25"),
            ],
        )
        kept = con.execute(f"SELECT acct_no_raw FROM rows base WHERE {expression}").fetchall()
    finally:
        con.close()

    assert kept == [("acct-real",)]


def test_cleaning_aggregate_preserves_real_zero_and_blocks_incomplete_amounts() -> None:
    step = next(item for item in CLEANING_STEP_DEFINITIONS if item.step == 10)
    con = duckdb.connect(":memory:")
    try:
        con.execute(
            """
            CREATE TABLE fc_transaction_norm(
                case_id VARCHAR,
                clean_card_no VARCHAR,
                card_no VARCHAR,
                clean_acct_no VARCHAR,
                acct_no VARCHAR,
                account_open_name VARCHAR,
                opener_id_no VARCHAR,
                clean_dc_flag VARCHAR,
                dc_flag VARCHAR,
                clean_amount VARCHAR,
                amount VARCHAR,
                txn_ts TIMESTAMP,
                txn_time VARCHAR,
                clean_invalid INTEGER,
                clean_failed INTEGER,
                clean_reversal INTEGER
            )
            """
        )
        con.executemany(
            "INSERT INTO fc_transaction_norm VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
            [
                ("case-a", None, "card-zero", None, "acct-zero", None, None, "进", None, "0", None, None, None, 0, 0, 0),
                ("case-a", None, "card-missing", None, "acct-missing", None, None, "进", None, None, None, None, None, 0, 0, 0),
                ("case-a", None, "card-nan", None, "acct-nan", None, None, "进", None, "NaN", None, None, None, 0, 0, 0),
                ("case-a", None, "card-partial", None, "acct-partial", None, None, "进", None, "5", None, None, None, 0, 0, 0),
                ("case-a", None, "card-partial", None, "acct-partial", None, None, "进", None, None, None, None, None, 0, 0, 0),
                ("case-a", None, "card-unknown", None, "acct-unknown", None, None, "进", None, "8", None, None, None, None, 0, 0),
            ],
        )
        rows = con.execute(step.sql, ("case-a", 100, 0)).fetchall()
    finally:
        con.close()

    by_account = {str(row[1]): row for row in rows}
    assert by_account["acct-zero"][4] == 0.0
    assert by_account["acct-missing"][4] is None
    assert by_account["acct-nan"][4] is None
    assert by_account["acct-partial"][4] is None
    assert "acct-unknown" not in by_account


def test_cleaning_numeric_step_marks_nonfinite_high_priority_values_as_failed() -> None:
    step = next(item for item in CLEANING_STEP_DEFINITIONS if item.step == 1)
    con = duckdb.connect(":memory:")
    try:
        con.execute(
            """
            CREATE TABLE fc_transaction_norm(
                case_id VARCHAR,
                file_id VARCHAR,
                row_no BIGINT,
                id BIGINT,
                card_no VARCHAR,
                acct_no VARCHAR,
                txn_time VARCHAR,
                orig_amount VARCHAR,
                amount VARCHAR,
                clean_amount VARCHAR,
                clean_amt_fixed INTEGER,
                clean_amt_failed INTEGER,
                orig_balance VARCHAR,
                balance VARCHAR,
                clean_balance VARCHAR,
                clean_bal_fixed INTEGER,
                clean_bal_failed INTEGER,
                clean_dc_flag VARCHAR,
                dc_flag VARCHAR,
                counterparty_acct VARCHAR,
                counterparty_name VARCHAR
            )
            """
        )
        con.execute(
            """
            CREATE TABLE fc_transaction_raw(
                case_id VARCHAR,
                file_id VARCHAR,
                row_no BIGINT,
                amount_raw VARCHAR,
                balance_raw VARCHAR
            )
            """
        )
        con.executemany(
            "INSERT INTO fc_transaction_norm VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
            [
                ("case-a", "f", 1, 1, "card-a", "acct-a", "", None, "1", "0", 1, 0, None, "2", "0", 1, 0, "进", None, None, None),
                ("case-a", "f", 2, 2, "card-b", "acct-b", "", None, "1", "NaN", 1, 0, None, "2", "Infinity", 1, 0, "进", None, None, None),
            ],
        )
        rows = con.execute(step.sql, ("case-a", 100, 0)).fetchall()
    finally:
        con.close()

    by_account = {str(row[1]): row for row in rows}
    assert by_account["acct-a"][4] == "__GREEN__0"
    assert by_account["acct-a"][6] == "__GREEN__0"
    assert by_account["acct-b"][4] == "__RED__转换失败"
    assert by_account["acct-b"][6] == "__RED__转换失败"


class _CleanStatsEngine:
    def __init__(self, response: object) -> None:
        self.response = response

    def query(self, _sql: str, _params=()):
        if isinstance(self.response, BaseException):
            raise self.response
        return self.response


def _clean_stats_context() -> SimpleNamespace:
    return SimpleNamespace(
        norm_table="fc_transaction_norm",
        case_id="case-a",
        file_id="file-a",
    )


@pytest.mark.parametrize(
    "response",
    (
        [],
        [(None, None, None, None, None, None)],
        RuntimeError("stats unavailable"),
    ),
)
def test_single_file_import_note_marks_missing_clean_stats_unavailable(response: object) -> None:
    note_lines: list[str] = []

    _append_clean_stats_note(
        _CleanStatsEngine(response),  # type: ignore[arg-type]
        note_lines,
        _clean_stats_context(),  # type: ignore[arg-type]
    )

    assert note_lines == [CLEAN_STATS_UNAVAILABLE_NOTE]
    assert " 0 " not in note_lines[0]


def test_single_file_import_note_preserves_verified_zero_without_unavailable_warning() -> None:
    note_lines: list[str] = []

    _append_clean_stats_note(
        _CleanStatsEngine([(0, 0, 0, 0, 0, 0)]),  # type: ignore[arg-type]
        note_lines,
        _clean_stats_context(),  # type: ignore[arg-type]
    )

    assert note_lines == []


class _AmbiguousUpdateCursor:
    def __init__(self) -> None:
        self.executions: list[str] = []

    def execute(self, sql: str, _params=()) -> None:
        self.executions.append(sql)
        if sql.startswith("SELECT COUNT(1) FROM ("):
            raise RuntimeError("unsupported wrapper")

    def fetchall(self):
        raise RuntimeError("result count unavailable")


def test_cleaning_update_count_fails_closed_without_reexecuting_ambiguous_update() -> None:
    cursor = _AmbiguousUpdateCursor()

    with pytest.raises(
        cleaning_sql_counter.CleaningCountUnavailableError,
        match="^cleaning_update_count_unavailable$",
    ):
        cleaning_sql_counter.update_count(
            cursor,
            "UPDATE rows SET value=1 WHERE id=1",
            (),
        )

    update_executions = [sql for sql in cursor.executions if sql.startswith("UPDATE rows")]
    assert update_executions == ["UPDATE rows SET value=1 WHERE id=1 RETURNING 1"]


@pytest.mark.parametrize("row", (None, (), (None,), (True,), (-1,)))
def test_cleaning_changes_never_converts_missing_or_invalid_count_to_zero(row: object) -> None:
    class _Cursor:
        def execute(self, _sql: str) -> None:
            return None

        def fetchone(self):
            return row

    with pytest.raises(
        cleaning_sql_counter.CleaningCountUnavailableError,
        match="^cleaning_changes_count_unavailable$",
    ):
        cleaning_sql_counter.changes(_Cursor())


def test_cleaning_update_count_preserves_real_zero_and_positive_counts_in_duckdb() -> None:
    con = duckdb.connect(":memory:")
    try:
        con.execute("CREATE TABLE rows(id INTEGER, value INTEGER)")
        con.executemany("INSERT INTO rows VALUES (?,?)", [(1, 0), (2, 0)])

        assert cleaning_sql_counter.update_count(
            con,
            "UPDATE rows SET value=1 WHERE id>10",
            (),
        ) == 0
        assert cleaning_sql_counter.update_count(
            con,
            "UPDATE rows SET value=1 WHERE id<=2",
            (),
        ) == 2
    finally:
        con.close()


def test_legacy_cleaning_result_missing_count_fails_without_state_mutation() -> None:
    state = CleaningRunState()

    with pytest.raises(RuntimeError, match="^legacy_cleaning_result_incomplete$"):
        state.apply_legacy_result(2, {"rows_affected_delta": 7})

    assert state.rows_affected_total == 0
    assert state.invalid_total is None


def test_cleaning_summary_omits_unexecuted_counts_and_preserves_verified_zero() -> None:
    summary = build_cleaning_summary(
        scope_rows=1,
        scope_ids=["file-a"],
        active_steps=[2],
        native_segments=[],
        python_fallback_steps=[2],
        legacy_python_cleaning_steps=[2],
        native_clean_all_attempted=False,
        native_clean_all_failed=False,
        invalid_total=0,
        duplicate_total=None,
        failed_total=None,
        reversal_total=None,
        normalized_count=None,
        inferred_count=None,
        filled_count=None,
        account_invalid_total=None,
        suffix_acc_total=None,
        account_fill_total=None,
        rows_affected_total=0,
        duration_ms=0,
        step1={},
    )

    assert summary["invalid"] == 0
    for absent in (
        "duplicate",
        "failed",
        "reversal",
        "dc_normalized",
        "dc_inferred",
        "card_filled",
        "account_invalid",
        "account_suffix",
        "account_info_filled",
    ):
        assert absent not in summary
