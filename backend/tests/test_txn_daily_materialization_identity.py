from __future__ import annotations

import re

import pytest

from app.core.db_engine import DuckDBEngine
from app.repositories import txn_daily_aggregate as txn_daily_aggregate_module
from app.repositories.txn_daily_aggregate import TxnDailyAggregateStore
from app.repositories.txn_daily_materialization_identity import (
    TxnDailyMaterializationIdentity,
    build_txn_daily_materialization_identity,
)


def _fixture_engine(tmp_path, *, case_id: str = "case-a", revision: int = 7, sha256: str | None = None):
    engine = DuckDBEngine(tmp_path / "case.duckdb")
    engine.execute("CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT)")
    engine.execute(
        "INSERT INTO analysis_revision_state VALUES ('stats_flow_source', ?)",
        (revision,),
    )
    engine.execute(
        """
        CREATE TABLE fc_transaction_norm(
            id BIGINT,
            case_id TEXT,
            txn_ts TIMESTAMP,
            clean_invalid INTEGER,
            clean_failed INTEGER,
            clean_reversal INTEGER
        )
        """
    )
    engine.execute(
        "INSERT INTO fc_transaction_norm VALUES (1, ?, NULL, 0, 0, 0), (2, ?, NULL, 1, 0, 0)",
        (case_id, case_id),
    )
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
        "INSERT INTO import_file_log VALUES ('file-1', ?, 'fc_transaction', ?, 2, '已完成', 'done')",
        (case_id, sha256 or "a" * 64),
    )
    return engine


def test_materialization_identity_is_deterministic_and_case_revision_content_bound(tmp_path) -> None:
    engine = _fixture_engine(tmp_path)
    try:
        baseline = build_txn_daily_materialization_identity(
            engine,
            case_id="case-a",
            source_revision=7,
            source_snapshot=(2, "", 2),
        )
        repeated = build_txn_daily_materialization_identity(
            engine,
            case_id="case-a",
            source_revision=7,
            source_snapshot=(2, "", 2),
        )
        assert repeated == baseline
        assert baseline.source_signature == "a5c945c07a0f0d47f008deb3ddbd139eb2bc4757ed5247ea6e9d268595533284"
        assert re.fullmatch(r"txn_daily_snapshot:v12:[a-f0-9]{64}", baseline.value)

        engine.execute(
            "UPDATE fc_transaction_norm SET clean_invalid=CASE id WHEN 1 THEN 1 ELSE 0 END"
        )
        changed_clean_binding = build_txn_daily_materialization_identity(
            engine,
            case_id="case-a",
            source_revision=7,
            source_snapshot=(2, "", 2),
        )
        assert changed_clean_binding != baseline
        engine.execute(
            "UPDATE fc_transaction_norm SET clean_invalid=CASE id WHEN 1 THEN 0 ELSE 1 END"
        )

        engine.execute("UPDATE import_file_log SET sha256=?", ("b" * 64,))
        changed_content = build_txn_daily_materialization_identity(
            engine,
            case_id="case-a",
            source_revision=7,
            source_snapshot=(2, "", 2),
        )
        assert changed_content != baseline

        engine.execute("UPDATE import_file_log SET sha256=?", ("a" * 64,))
        engine.execute("UPDATE analysis_revision_state SET revision=8")
        changed_revision = build_txn_daily_materialization_identity(
            engine,
            case_id="case-a",
            source_revision=8,
            source_snapshot=(2, "", 2),
        )
        assert changed_revision != baseline
    finally:
        engine.close()


def test_metadata_migration_removes_legacy_alias_and_never_mutates_identity(tmp_path) -> None:
    engine = _fixture_engine(tmp_path)
    store = object.__new__(TxnDailyAggregateStore)
    try:
        store._ensure_meta_table(engine)
        engine.execute(
            """CREATE TABLE analysis_txn_daily_agg(
                   acct_key TEXT, txn_day DATE, cp_key TEXT, dc_val TEXT,
                   txn_count BIGINT, amt_sum DOUBLE,
                   amount_source_present_count BIGINT, amount_valid_count BIGINT,
                   amount_missing_count BIGINT, amount_parse_failed_count BIGINT
               )"""
        )
        engine.execute(
            "INSERT INTO analysis_txn_daily_agg VALUES ('acct-a', DATE '2026-01-01', 'cp-a', '进', 1, 10, 1, 1, 0, 0)"
        )
        engine.execute(
            """CREATE TABLE analysis_txn_detail_idx(
                   id BIGINT, amount DOUBLE, amount_source_present BIGINT,
                   amount_parse_failed BIGINT
               )"""
        )
        engine.execute(
            "CREATE TABLE analysis_txn_keyword_idx(txn_row_id BIGINT, kind TEXT, token TEXT, token_order BIGINT)"
        )
        engine.execute("CREATE TABLE analysis_account_dim(account_key TEXT)")
        engine.execute(
            "INSERT INTO analysis_materialization_meta(agg_name, agg_version, source_revision) "
            "VALUES ('txn_daily_' || 'active:v8', 8, 7)"
        )
        identity = build_txn_daily_materialization_identity(
            engine,
            case_id="case-a",
            source_revision=7,
            source_snapshot=(2, "", 2),
        )

        store._replace_materialization_metadata(
            engine,
            case_id="case-a",
            identity=identity,
            source_revision=7,
            source_row_count=2,
            source_max_txn_ts="",
            source_max_id=2,
            row_count=1,
        )
        rows = engine.query(
            "SELECT agg_name, source_signature FROM analysis_materialization_meta "
            "WHERE starts_with(agg_name, 'txn_daily_')"
        )
        assert rows == [(identity.value, identity.source_signature)]
        assert store._is_current(engine, "case-a", 7, (2, "", 2)) is False

        engine.execute("UPDATE analysis_txn_daily_agg SET amt_sum=11")
        assert store._is_current(engine, "case-a", 7, (2, "", 2)) is False
        engine.execute("UPDATE analysis_txn_daily_agg SET amt_sum=10")

        with pytest.raises(RuntimeError, match="result row count is inconsistent"):
            store._replace_materialization_metadata(
                engine,
                case_id="case-a",
                identity=identity,
                source_revision=7,
                source_row_count=2,
                source_max_txn_ts="",
                source_max_id=2,
                row_count=2,
            )
    finally:
        engine.close()


def test_invalid_manifest_and_stale_snapshot_fail_closed(tmp_path) -> None:
    engine = _fixture_engine(tmp_path)
    try:
        with pytest.raises(RuntimeError, match="source content changed"):
            build_txn_daily_materialization_identity(
                engine,
                case_id="case-a",
                source_revision=7,
                source_snapshot=(3, "", 2),
            )
        engine.execute("UPDATE import_file_log SET sha256='not-a-complete-sha256'")
        with pytest.raises(RuntimeError, match="manifest is incomplete"):
            build_txn_daily_materialization_identity(
                engine,
                case_id="case-a",
                source_revision=7,
                source_snapshot=(2, "", 2),
            )
    finally:
        engine.close()


def test_result_signature_matches_rust_typed_row_golden(tmp_path) -> None:
    engine = DuckDBEngine(tmp_path / "result-signature-golden.duckdb")
    store = object.__new__(TxnDailyAggregateStore)
    try:
        engine.execute(
            "CREATE TABLE analysis_txn_daily_agg(acct_key TEXT, txn_day DATE, cp_key TEXT, dc_val TEXT)"
        )
        engine.execute(
            """INSERT INTO analysis_txn_daily_agg VALUES
                   ('账户一', DATE '2026-01-02', '对手甲', '进'),
                   ('账户一', DATE '2026-01-01', NULL, '出')"""
        )
        engine.execute(
            """CREATE TABLE analysis_txn_detail_idx(
                   id BIGINT, amount DOUBLE, note TEXT, exact_amount DECIMAL(20,4),
                   huge_value HUGEINT, observed_at TIMESTAMPTZ
               )"""
        )
        engine.execute(
            """INSERT INTO analysis_txn_detail_idx VALUES
                   (2, NULL, '中文', 12.3400, 170141183460469231731687303715884105,
                    TIMESTAMPTZ '2026-01-01 10:00:00+08:00'),
                   (1, 0, 'zero', 0.0000, 0, NULL)"""
        )
        engine.execute(
            """CREATE TABLE analysis_txn_keyword_idx(
                   txn_row_id BIGINT, kind TEXT, token TEXT, token_order BIGINT
               )"""
        )
        engine.execute(
            "INSERT INTO analysis_txn_keyword_idx VALUES (2, 'summary', '关键词', 0), (1, 'remark', 'zero', 0)"
        )
        engine.execute("CREATE TABLE analysis_account_dim(account_key TEXT, label TEXT)")
        engine.execute("INSERT INTO analysis_account_dim VALUES ('账户一', '测试账户')")

        assert (
            store._materialization_result_signature(engine, "case-跨语言")
            == "affbe8e3b71ba16deea7ed1254a5b1505956f70ddfb7881c64556da6758c07cf"
        )
    finally:
        engine.close()


def test_materialization_identity_rejects_missing_manifest_row_count(tmp_path) -> None:
    engine = _fixture_engine(tmp_path)
    try:
        engine.execute("UPDATE import_file_log SET rows_imported_norm=NULL")
        with pytest.raises(RuntimeError, match="manifest is incomplete"):
            build_txn_daily_materialization_identity(
                engine,
                case_id="case-a",
                source_revision=7,
                source_snapshot=(2, "", 2),
            )
    finally:
        engine.close()


def test_materialization_identity_rejects_duplicate_source_row_lineage(tmp_path) -> None:
    engine = _fixture_engine(tmp_path)
    try:
        engine.execute("UPDATE fc_transaction_norm SET id=1 WHERE id=2")
        with pytest.raises(RuntimeError, match="row lineage is ambiguous"):
            build_txn_daily_materialization_identity(
                engine,
                case_id="case-a",
                source_revision=7,
                source_snapshot=(2, "", 2),
            )
    finally:
        engine.close()


def test_caller_owned_engine_never_bypasses_native_materialization_authority(monkeypatch) -> None:
    class FakeEngine:
        pass

    store = object.__new__(TxnDailyAggregateStore)
    checked = []

    monkeypatch.setattr(
        store,
        "_is_current",
        lambda _con, _case_id, _revision, _snapshot: checked.append(True) or True,
    )

    assert store.ensure_materialized("case-a", engine=FakeEngine()) is False
    assert checked == []


def test_native_unavailable_never_invokes_python_materialization_fallback(
    tmp_path, monkeypatch
) -> None:
    class FakeEngine:
        path = tmp_path / "case.duckdb"

        def close(self) -> None:
            return None

    class FakeStorage:
        def __init__(self) -> None:
            self.open_calls: list[tuple[str, bool]] = []

        def open_case_engine(self, case_id: str, *, read_only: bool):
            self.open_calls.append((case_id, read_only))
            return FakeEngine()

    store = object.__new__(TxnDailyAggregateStore)
    storage = FakeStorage()
    store._storage = storage
    native_calls = []

    monkeypatch.setattr(store, "_table_exists", lambda _con, _table: True)
    monkeypatch.setattr(store, "_compute_source_snapshot", lambda _con, _case_id: (2, "", 2))
    monkeypatch.setattr(txn_daily_aggregate_module, "get_stats_flow_source_revision", lambda _con: 7)
    monkeypatch.setattr(txn_daily_aggregate_module, "_shutdown_stats_worker_before_duckdb_write", lambda: None)
    monkeypatch.setattr(
        txn_daily_aggregate_module,
        "try_materialize_txn_daily",
        lambda **kwargs: native_calls.append(kwargs) or None,
    )
    monkeypatch.setattr(
        store,
        "_rebuild_materialized",
        lambda *_args, **_kwargs: pytest.fail("Python materialization fallback executed"),
    )

    assert store.ensure_materialized("case-a") is False
    assert storage.open_calls == [("case-a", True)]
    assert native_calls == [
        {
            "case_id": "case-a",
            "db_path": tmp_path / "case.duckdb",
            "source_revision": 7,
            "source_snapshot": (2, "", 2),
        }
    ]


def test_native_materialization_result_is_not_current_without_native_verifier(
    tmp_path, monkeypatch
) -> None:
    class FakeEngine:
        path = tmp_path / "case.duckdb"

        def close(self) -> None:
            return None

    class FakeStorage:
        def __init__(self) -> None:
            self.open_calls: list[tuple[str, bool]] = []

        def open_case_engine(self, case_id: str, *, read_only: bool):
            self.open_calls.append((case_id, read_only))
            return FakeEngine()

    store = object.__new__(TxnDailyAggregateStore)
    storage = FakeStorage()
    store._storage = storage

    monkeypatch.setattr(store, "_table_exists", lambda _con, _table: True)
    monkeypatch.setattr(store, "_compute_source_snapshot", lambda _con, _case_id: (2, "", 2))
    monkeypatch.setattr(txn_daily_aggregate_module, "get_stats_flow_source_revision", lambda _con: 7)
    monkeypatch.setattr(txn_daily_aggregate_module, "_shutdown_stats_worker_before_duckdb_write", lambda: None)
    monkeypatch.setattr(
        txn_daily_aggregate_module,
        "try_materialize_txn_daily",
        lambda **_kwargs: {"ok": True},
    )

    assert store.ensure_materialized("case-a") is False
    assert storage.open_calls == [("case-a", True), ("case-a", True)]


class _CurrentIdentityEngine:
    def __init__(
        self,
        *,
        metadata_rows: list[tuple[object, ...]] | None = None,
        row_count_rows: list[tuple[object, ...]] | None = None,
    ) -> None:
        self.metadata_rows = metadata_rows if metadata_rows is not None else [
            (
                "txn_daily_snapshot:v12:" + "a" * 64,
                12,
                "case-a",
                2,
                7,
                2,
                "",
                2,
                "b" * 64,
                "c" * 64,
                1,
            )
        ]
        self.row_count_rows = row_count_rows if row_count_rows is not None else [(1,)]

    def query(self, sql: str, _params=()):
        normalized = " ".join(sql.split())
        if normalized.startswith("SELECT agg_name, agg_version"):
            return self.metadata_rows
        if normalized == "SELECT COUNT(1) FROM analysis_txn_daily_agg":
            return self.row_count_rows
        raise AssertionError(f"unexpected SQL: {normalized}")


def _current_store(monkeypatch) -> TxnDailyAggregateStore:
    store = object.__new__(TxnDailyAggregateStore)
    identity = TxnDailyMaterializationIdentity(
        value="txn_daily_snapshot:v12:" + "a" * 64,
        source_signature="b" * 64,
    )
    monkeypatch.setattr(store, "_table_exists", lambda _con, _table: True)
    monkeypatch.setattr(
        store,
        "_table_columns",
        lambda _con, table: (
            {"amount", "amount_source_present", "amount_parse_failed"}
            if table == "analysis_txn_detail_idx"
            else {
                "txn_count",
                "amt_sum",
                "amount_source_present_count",
                "amount_valid_count",
                "amount_missing_count",
                "amount_parse_failed_count",
            }
        ),
    )
    monkeypatch.setattr(
        txn_daily_aggregate_module,
        "build_txn_daily_materialization_identity",
        lambda *_args, **_kwargs: identity,
    )
    monkeypatch.setattr(store, "_materialization_result_signature", lambda *_args: "c" * 64)
    return store


@pytest.mark.parametrize(
    "row_count_rows",
    [
        [("1",)],
        [(True,)],
        [(1.0,)],
        [(None,)],
        [(1,), (1,)],
    ],
)
def test_current_materialization_rejects_coercible_unknown_or_ambiguous_sql_count(
    monkeypatch,
    row_count_rows,
) -> None:
    store = _current_store(monkeypatch)
    engine = _CurrentIdentityEngine(row_count_rows=row_count_rows)

    assert store._is_current(engine, "case-a", 7, (2, "", 2)) is False  # type: ignore[arg-type]


@pytest.mark.parametrize(
    ("field_index", "invalid_value"),
    [
        (1, "12"),
        (3, True),
        (4, 7.0),
        (5, None),
        (10, "1"),
    ],
)
def test_current_materialization_rejects_non_exact_numeric_metadata(
    monkeypatch,
    field_index: int,
    invalid_value: object,
) -> None:
    store = _current_store(monkeypatch)
    baseline = list(_CurrentIdentityEngine().metadata_rows[0])
    baseline[field_index] = invalid_value
    engine = _CurrentIdentityEngine(metadata_rows=[tuple(baseline)])

    assert store._is_current(engine, "case-a", 7, (2, "", 2)) is False  # type: ignore[arg-type]


@pytest.mark.parametrize(
    "rows",
    (
        None,
        [],
        [()],
        [(0,)],
        [(0, None, None), (0, None, None)],
        [(True, None, None)],
        [("0", None, None)],
        [(-1, None, None)],
        [((1 << 53), None, None)],
        [(0, "", 0)],
        [(1, None, None)],
        [(1, None, True)],
        [(1, None, (1 << 53))],
    ),
)
def test_source_snapshot_rejects_missing_malformed_and_defaulted_aggregate_rows(rows) -> None:
    class FakeEngine:
        def query(self, _sql, _params=()):
            return rows

    store = object.__new__(TxnDailyAggregateStore)

    with pytest.raises(RuntimeError, match="^materialization source snapshot is unavailable$"):
        store._compute_source_snapshot(FakeEngine(), "case-a")  # type: ignore[arg-type]


def test_source_snapshot_preserves_database_observed_empty_result(tmp_path) -> None:
    engine = DuckDBEngine(tmp_path / "empty-source-snapshot.duckdb")
    store = object.__new__(TxnDailyAggregateStore)
    try:
        engine.execute("CREATE TABLE fc_transaction_norm(case_id TEXT, txn_ts TIMESTAMP, id BIGINT)")
        assert store._compute_source_snapshot(engine, "case-a") == (0, "", 0)
    finally:
        engine.close()


@pytest.mark.parametrize(
    ("rows", "expected"),
    [
        ([(1,)], True),
        ([(0,)], False),
        ([("1",)], False),
        ([(True,)], False),
        ([(1.0,)], False),
        ([(None,)], False),
        ([(1,), (1,)], False),
    ],
)
def test_table_exists_requires_one_exact_sql_count(rows, expected: bool) -> None:
    class FakeEngine:
        def query(self, _sql, _params=()):
            return rows

    assert TxnDailyAggregateStore._table_exists(FakeEngine(), "table-a") is expected  # type: ignore[arg-type]
