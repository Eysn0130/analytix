from __future__ import annotations

import pytest

from app.core.db_engine import DuckDBEngine
from app.repositories.txn_daily_aggregate import (
    TxnAmountCoverageIncompleteError,
    TxnDailyAggregateStore,
)
from app.repositories.txn_daily_agg_sql import build_daily_agg_sql
from app.repositories.txn_daily_detail_sql import build_detail_idx_sql
from app.repositories.txn_daily_materialization_identity import (
    build_txn_daily_materialization_identity,
)
from app.repositories.txn_daily_projection_sql import build_txn_projection_exprs


def test_python_detail_materialization_preserves_missing_parse_failed_and_real_zero(tmp_path) -> None:
    engine = DuckDBEngine(tmp_path / "amount-semantics.duckdb")
    try:
        engine.execute(
            """
            CREATE TABLE fc_transaction_norm(
                id BIGINT,
                case_id TEXT,
                acct_no TEXT,
                amount TEXT,
                clean_invalid INTEGER,
                clean_failed INTEGER,
                clean_reversal INTEGER
            )
            """
        )
        engine.execute(
            """
            INSERT INTO fc_transaction_norm VALUES
                (1, 'case-a', 'acct-a', '12.34', 0, 0, 0),
                (2, 'case-a', 'acct-a', NULL, 0, 0, 0),
                (3, 'case-a', 'acct-a', 'not-a-number', 0, 0, 0),
                (4, 'case-a', 'acct-a', '0', 0, 0, 0),
                (5, 'case-a', 'acct-a', '', 0, 0, 0),
                (6, 'case-a', 'acct-a', 'NaN', 0, 0, 0),
                (7, 'case-a', 'acct-a', 'Inf', 0, 0, 0),
                (8, 'case-a', 'acct-a', '-Inf', 0, 0, 0),
                (9, 'case-a', 'acct-a', '1e309', 0, 0, 0)
            """
        )
        columns = TxnDailyAggregateStore._table_columns(engine, "fc_transaction_norm")
        engine.execute(build_detail_idx_sql(build_txn_projection_exprs(columns)), ("case-a",))

        assert engine.query(
            """
            SELECT amount, amount_source_present, amount_parse_failed
              FROM analysis_txn_detail_idx__staging
             ORDER BY id
            """
        ) == [
            (12.34, 1, 0),
            (None, 0, 0),
            (None, 1, 1),
            (0.0, 1, 0),
            (None, 0, 0),
            (None, 1, 1),
            (None, 1, 1),
            (None, 1, 1),
            (None, 1, 1),
        ]
        engine.execute(build_daily_agg_sql())
        assert engine.query(
            """
            SELECT txn_count, amt_sum, amount_source_present_count, amount_valid_count,
                   amount_missing_count, amount_parse_failed_count
              FROM analysis_txn_daily_agg__staging
            """
        ) == [(9, None, 7, 2, 2, 5)]
    finally:
        engine.close()


def test_current_materialization_requires_amount_lineage_columns(tmp_path) -> None:
    engine = DuckDBEngine(tmp_path / "stale-amount-schema.duckdb")
    store = object.__new__(TxnDailyAggregateStore)
    try:
        for table_name in (
            "analysis_txn_daily_agg",
            "analysis_txn_keyword_idx",
            "analysis_account_dim",
        ):
            engine.execute(f"CREATE TABLE {table_name}(id BIGINT)")
        engine.execute("CREATE TABLE analysis_txn_detail_idx(amount DOUBLE)")

        assert store._is_current(engine, "case-a", 1, (0, "", 0)) is False
    finally:
        engine.close()


def test_higher_priority_invalid_numeric_never_falls_back_to_lower_priority_value(tmp_path) -> None:
    engine = DuckDBEngine(tmp_path / "numeric-priority.duckdb")
    try:
        engine.execute(
            """
            CREATE TABLE fc_transaction_norm(
                id BIGINT,
                case_id TEXT,
                acct_no TEXT,
                clean_amount TEXT,
                amount TEXT,
                clean_balance TEXT,
                balance TEXT,
                clean_invalid INTEGER,
                clean_failed INTEGER,
                clean_reversal INTEGER
            )
            """
        )
        engine.execute(
            "INSERT INTO fc_transaction_norm VALUES (1, 'case-a', 'acct-a', 'NaN', '123.45', 'Inf', '456.78', 0, 0, 0)"
        )
        columns = TxnDailyAggregateStore._table_columns(engine, "fc_transaction_norm")
        engine.execute(build_detail_idx_sql(build_txn_projection_exprs(columns)), ("case-a",))

        assert engine.query(
            "SELECT amount, balance, amount_source_present, amount_parse_failed FROM analysis_txn_detail_idx__staging"
        ) == [(None, None, 1, 1)]
    finally:
        engine.close()


def _seed_amount_coverage_fixture(engine: DuckDBEngine, rows: list[tuple[object, ...]]) -> None:
    store = object.__new__(TxnDailyAggregateStore)
    engine.execute("CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT)")
    engine.execute("INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 1)")
    engine.execute(
        """
        CREATE TABLE fc_transaction_norm(
            id BIGINT,
            case_id TEXT,
            txn_ts TIMESTAMP,
            acct_no TEXT,
            amount TEXT,
            dc_flag TEXT,
            clean_invalid INTEGER,
            clean_failed INTEGER,
            clean_reversal INTEGER
        )
        """
    )
    source_rows = [
        (
            index,
            "case-a",
            None,
            "acct-a",
            None if amount is None else str(amount),
            dc_val,
            0,
            0,
            0,
        )
        for index, (amount, _source_present, _parse_failed, dc_val) in enumerate(rows, start=1)
    ]
    if source_rows:
        engine.executemany("INSERT INTO fc_transaction_norm VALUES (?,?,?,?,?,?,?,?,?)", source_rows)
    engine.execute(
        """
        CREATE TABLE import_file_log(
            file_id TEXT,
            case_id TEXT,
            kind TEXT,
            sha256 TEXT,
            rows_imported_norm BIGINT,
            status TEXT,
            cleaned_status TEXT
        )
        """
    )
    engine.execute(
        "INSERT INTO import_file_log VALUES ('file-a', 'case-a', 'fc_transaction', ?, ?, '已完成', 'done')",
        ("a" * 64, len(rows)),
    )
    engine.execute(
        """
        CREATE TABLE analysis_txn_detail_idx(
            id BIGINT,
            amount DOUBLE,
            amount_source_present BIGINT,
            amount_parse_failed BIGINT,
            dc_val TEXT
        )
        """
    )
    if rows:
        engine.executemany(
            "INSERT INTO analysis_txn_detail_idx VALUES (?,?,?,?,?)",
            [
                (index, amount, source_present, parse_failed, dc_val)
                for index, (amount, source_present, parse_failed, dc_val) in enumerate(rows, start=1)
            ],
        )
    engine.execute(
        """
        CREATE TABLE analysis_txn_daily_agg(
            acct_key TEXT,
            txn_day DATE,
            cp_key TEXT,
            txn_count BIGINT,
            amount_source_present_count BIGINT,
            amount_valid_count BIGINT,
            amount_missing_count BIGINT,
            amount_parse_failed_count BIGINT,
            amt_sum DOUBLE,
            dc_val TEXT
        )
        """
    )
    aggregate_rows = []
    for amount, source_present, parse_failed, dc_val in rows:
        valid = int(amount is not None and source_present == 1 and parse_failed == 0)
        missing = int(amount is None and source_present == 0 and parse_failed == 0)
        failed = int(amount is None and source_present == 1 and parse_failed == 1)
        aggregate_rows.append(
            (
                "acct-a",
                None,
                f"cp-{len(aggregate_rows) + 1}",
                1,
                source_present,
                valid,
                missing,
                failed,
                amount if valid else None,
                dc_val,
            )
        )
    if aggregate_rows:
        engine.executemany(
            "INSERT INTO analysis_txn_daily_agg VALUES (?,?,?,?,?,?,?,?,?,?)",
            aggregate_rows,
        )
    engine.execute(
        "CREATE TABLE analysis_txn_keyword_idx(txn_row_id BIGINT, kind TEXT, token TEXT, token_order BIGINT)"
    )
    engine.execute("CREATE TABLE analysis_account_dim(account_key TEXT)")
    engine.execute("INSERT INTO analysis_account_dim VALUES ('acct-a')")
    store._ensure_meta_table(engine)
    source_snapshot = (len(rows), "", len(rows))
    identity = build_txn_daily_materialization_identity(
        engine,
        case_id="case-a",
        source_revision=1,
        source_snapshot=source_snapshot,
    )
    store._replace_materialization_metadata(
        engine,
        case_id="case-a",
        identity=identity,
        source_revision=1,
        source_row_count=source_snapshot[0],
        source_max_txn_ts=source_snapshot[1],
        source_max_id=source_snapshot[2],
        row_count=len(aggregate_rows),
    )


class _CoverageQueryEngine:
    def __init__(
        self,
        *,
        revision_rows: list[tuple[object, ...]] | None = None,
        detail_rows: list[tuple[object, ...]] | None = None,
        aggregate_rows: list[tuple[object, ...]] | None = None,
        identity_rows: list[tuple[object, ...]] | None = None,
    ) -> None:
        self.revision_rows = revision_rows if revision_rows is not None else [(1,)]
        self.detail_rows = detail_rows if detail_rows is not None else [(1, 1, 0, 0, 1)]
        self.aggregate_rows = (
            aggregate_rows if aggregate_rows is not None else [(1, 1, 1, 1, 0, 0, 1, 0)]
        )
        self.identity_rows = identity_rows if identity_rows is not None else [
            ("txn_daily_snapshot:v12:" + "a" * 64, 12, "case-a", 1)
        ]
        self.sql: list[str] = []

    def query(self, sql: str, _params=()):
        normalized = " ".join(sql.split())
        self.sql.append(normalized)
        if normalized.startswith("SELECT revision FROM analysis_revision_state"):
            return self.revision_rows
        if "FROM analysis_txn_detail_idx" in normalized:
            return self.detail_rows
        if "FROM analysis_txn_daily_agg" in normalized:
            return self.aggregate_rows
        if "FROM analysis_materialization_meta" in normalized:
            return self.identity_rows
        raise AssertionError(f"unexpected SQL: {normalized}")

    def execute(self, sql: str, _params=()):
        raise AssertionError(f"coverage verification attempted a write: {sql}")


def _coverage_store(monkeypatch) -> TxnDailyAggregateStore:
    store = object.__new__(TxnDailyAggregateStore)
    monkeypatch.setattr(store, "_table_exists", lambda _con, _table: True)
    monkeypatch.setattr(
        store,
        "_table_columns",
        lambda _con, table: (
            {"amount", "amount_source_present", "amount_parse_failed", "dc_val"}
            if table == "analysis_txn_detail_idx"
            else {
                "txn_count",
                "amount_source_present_count",
                "amount_valid_count",
                "amount_missing_count",
                "amount_parse_failed_count",
                "amt_sum",
                "dc_val",
                "agg_name",
                "agg_version",
                "case_id",
                "source_signature",
                "result_signature",
                "row_count",
            }
        ),
    )
    monkeypatch.setattr(store, "_compute_source_snapshot", lambda _con, _case: (1, "", 1))
    monkeypatch.setattr(store, "_is_current", lambda *_args: True)
    return store


def test_amount_coverage_accepts_real_zero_after_current_native_verification(
    tmp_path, monkeypatch
) -> None:
    engine = DuckDBEngine(tmp_path / "amount-coverage-complete.duckdb")
    store = object.__new__(TxnDailyAggregateStore)
    monkeypatch.setattr(store, "_is_current", lambda *_args: True)
    try:
        _seed_amount_coverage_fixture(engine, [(0.0, 1, 0, "进"), (12.34, 1, 0, "出")])

        first = store.require_complete_amount_coverage(
            case_id="case-a",
            engine=engine,
            ensure_ready=False,
        )
        second = store.require_complete_amount_coverage(
            case_id="case-a",
            engine=engine,
            ensure_ready=False,
        )

        assert first.complete is True
        assert first.total_rows == 2
        assert first.amount_valid_rows == 2
        assert first.digest == second.digest
        assert first.digest.startswith("txn_amount_coverage_v1_")
    finally:
        engine.close()


def test_amount_coverage_rejects_self_consistent_zero_transaction_aggregate_row(tmp_path) -> None:
    engine = DuckDBEngine(tmp_path / "amount-coverage-zero-aggregate.duckdb")
    store = object.__new__(TxnDailyAggregateStore)
    try:
        _seed_amount_coverage_fixture(engine, [(0.0, 1, 0, "进"), (12.34, 1, 0, "出")])
        engine.execute(
            """
            INSERT INTO analysis_txn_daily_agg VALUES
              ('acct-zero', NULL, 'cp-zero', 0, 0, 0, 0, 0, 0.0, '进')
            """
        )
        result_signature = store._materialization_result_signature(engine, "case-a")
        engine.execute(
            """
            UPDATE analysis_materialization_meta
               SET result_signature=?, row_count=3
             WHERE starts_with(agg_name, 'txn_daily_')
            """,
            (result_signature,),
        )

        with pytest.raises(TxnAmountCoverageIncompleteError, match="^transaction_amount_coverage_incomplete$"):
            store.require_complete_amount_coverage(
                case_id="case-a",
                engine=engine,
                ensure_ready=False,
            )
    finally:
        engine.close()


def test_amount_coverage_read_only_verification_does_not_bootstrap_or_mutate(monkeypatch) -> None:
    store = _coverage_store(monkeypatch)
    engine = _CoverageQueryEngine()
    monkeypatch.setattr(
        store,
        "ensure_materialized",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(AssertionError("materializer ran")),
    )

    coverage = store.require_complete_amount_coverage(
        case_id="case-a",
        engine=engine,  # type: ignore[arg-type]
        ensure_ready=False,
    )

    assert coverage.complete is True
    assert engine.sql
    assert all(sql.startswith("SELECT ") for sql in engine.sql)
    assert not any(
        token in f" {sql.upper()} "
        for sql in engine.sql
        for token in (" CREATE ", " INSERT ", " UPDATE ", " DELETE ", " ALTER ")
    )


@pytest.mark.parametrize(
    ("field", "invalid_value"),
    [
        ("detail", "1"),
        ("aggregate", True),
        ("identity_version", 12.0),
        ("identity_row_count", None),
        ("revision", "1"),
        ("revision", True),
        ("revision", 1.0),
        ("revision", None),
        ("revision", 1 << 53),
        ("detail", 1 << 53),
        ("identity_name", b"txn_daily_snapshot:v12:" + b"a" * 64),
        ("identity_case", " case-a"),
        ("identity_case", 1),
    ],
)
def test_amount_coverage_rejects_coercible_or_unknown_counts_and_versions(
    monkeypatch,
    field: str,
    invalid_value: object,
) -> None:
    store = _coverage_store(monkeypatch)
    engine = _CoverageQueryEngine()
    if field == "detail":
        engine.detail_rows = [(invalid_value, 1, 0, 0, 1)]
    elif field == "aggregate":
        engine.aggregate_rows = [(1, invalid_value, 1, 1, 0, 0, 1, 0)]
    elif field == "identity_version":
        engine.identity_rows = [("txn_daily_snapshot:v12:" + "a" * 64, invalid_value, "case-a", 1)]
    elif field == "identity_row_count":
        engine.identity_rows = [("txn_daily_snapshot:v12:" + "a" * 64, 12, "case-a", invalid_value)]
    elif field == "identity_name":
        engine.identity_rows = [(invalid_value, 12, "case-a", 1)]
    elif field == "identity_case":
        engine.identity_rows = [("txn_daily_snapshot:v12:" + "a" * 64, 12, invalid_value, 1)]
    else:
        engine.revision_rows = [(invalid_value,)]

    with pytest.raises(TxnAmountCoverageIncompleteError, match="^transaction_amount_coverage_incomplete$"):
        store.require_complete_amount_coverage(
            case_id="case-a",
            engine=engine,  # type: ignore[arg-type]
            ensure_ready=False,
        )


@pytest.mark.parametrize(
    ("row_kind", "rows"),
    [
        ("detail", [(1, 1, 0, 0, 1), (1, 1, 0, 0, 1)]),
        ("aggregate", [(1, 1, 1, 1, 0, 0, 1, 0), (1, 1, 1, 1, 0, 0, 1, 0)]),
        (
            "identity",
            [
                ("txn_daily_snapshot:v12:" + "a" * 64, 12, "case-a", 1),
                ("txn_daily_snapshot:v12:" + "b" * 64, 12, "case-a", 1),
            ],
        ),
        ("revision", [(1,), (1,)]),
    ],
)
def test_amount_coverage_rejects_ambiguous_extra_rows(monkeypatch, row_kind: str, rows) -> None:
    store = _coverage_store(monkeypatch)
    engine = _CoverageQueryEngine()
    setattr(engine, f"{row_kind}_rows", rows)

    with pytest.raises(TxnAmountCoverageIncompleteError, match="^transaction_amount_coverage_incomplete$"):
        store.require_complete_amount_coverage(
            case_id="case-a",
            engine=engine,  # type: ignore[arg-type]
            ensure_ready=False,
        )


def test_amount_coverage_preserves_explicit_zero_but_empty_case_is_not_complete(monkeypatch) -> None:
    store = _coverage_store(monkeypatch)
    engine = _CoverageQueryEngine(
        detail_rows=[(0, 0, 0, 0, 0)],
        aggregate_rows=[(0, 0, 0, 0, 0, 0, 0, 0)],
        identity_rows=[("txn_daily_snapshot:v12:" + "a" * 64, 12, "case-a", 0)],
    )

    with pytest.raises(TxnAmountCoverageIncompleteError, match="^transaction_amount_coverage_incomplete$"):
        store.require_complete_amount_coverage(
            case_id="case-a",
            engine=engine,  # type: ignore[arg-type]
            ensure_ready=False,
        )


@pytest.mark.parametrize("case_id", [None, 1, True, " case-a", "case-a ", ""])
def test_amount_coverage_requires_exact_nonempty_case_identity(monkeypatch, case_id: object) -> None:
    store = _coverage_store(monkeypatch)
    engine = _CoverageQueryEngine()

    with pytest.raises(TxnAmountCoverageIncompleteError, match="^transaction_amount_coverage_incomplete$"):
        store.require_complete_amount_coverage(
            case_id=case_id,  # type: ignore[arg-type]
            engine=engine,  # type: ignore[arg-type]
            ensure_ready=False,
        )
    assert engine.sql == []


@pytest.mark.parametrize(
    "rows",
    [
        [],
        [(100.0, 1, 0, "进"), (None, 0, 0, "出")],
        [(100.0, 1, 0, "进"), (None, 1, 1, "出")],
        [(100.0, 1, 0, "未知")],
        [(None, 0, 0, "进"), (None, 1, 1, "出")],
        [(float("inf"), 1, 0, "进")],
    ],
)
def test_amount_coverage_incomplete_never_becomes_zero_or_partial_sum(tmp_path, rows) -> None:
    engine = DuckDBEngine(tmp_path / "amount-coverage-incomplete.duckdb")
    store = object.__new__(TxnDailyAggregateStore)
    try:
        _seed_amount_coverage_fixture(engine, rows)

        with pytest.raises(TxnAmountCoverageIncompleteError, match="^transaction_amount_coverage_incomplete$"):
            store.require_complete_amount_coverage(
                case_id="case-a",
                engine=engine,
                ensure_ready=False,
            )
    finally:
        engine.close()


def test_amount_coverage_rejects_aggregate_or_case_binding_mismatch(tmp_path) -> None:
    engine = DuckDBEngine(tmp_path / "amount-coverage-mismatch.duckdb")
    store = object.__new__(TxnDailyAggregateStore)
    try:
        _seed_amount_coverage_fixture(engine, [(10.0, 1, 0, "进")])
        engine.execute(
            """
            UPDATE analysis_txn_daily_agg
               SET amount_source_present_count=0,
                   amount_valid_count=0,
                   amount_missing_count=1,
                   amt_sum=NULL
            """
        )
        with pytest.raises(TxnAmountCoverageIncompleteError, match="^transaction_amount_coverage_incomplete$"):
            store.require_complete_amount_coverage(case_id="case-a", engine=engine, ensure_ready=False)

        engine.execute(
            """
            UPDATE analysis_txn_daily_agg
               SET amount_source_present_count=1,
                   amount_valid_count=1,
                   amount_missing_count=0,
                   amt_sum=10.0
            """
        )
        engine.execute("UPDATE analysis_materialization_meta SET case_id='case-b'")
        with pytest.raises(TxnAmountCoverageIncompleteError, match="^transaction_amount_coverage_incomplete$"):
            store.require_complete_amount_coverage(case_id="case-a", engine=engine, ensure_ready=False)
    finally:
        engine.close()
