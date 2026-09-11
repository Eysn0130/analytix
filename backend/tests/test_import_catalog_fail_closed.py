from __future__ import annotations

from pathlib import Path

import pytest

from app.core.db_engine import DuckDBEngine
from app.core.fc_import_duckdb import (
    duckdb_table_columns,
    duckdb_table_exists,
    ensure_columns,
    supports_json_object,
    table_has_rows,
)
from app.core.fc_import_norm_rebuild import (
    _count_norm_rows,
    _kind_for_file,
    rebuild_norm_for_case,
    rebuild_norm_for_file,
)
from app.core.fc_import_privacy_delta import (
    _single_count_row,
    append_privacy_projection_delta_for_case_rows,
    append_privacy_projection_delta_from_hash_table,
    ensure_privacy_projection_delta_log,
)
from app.core.fc_import_tables import ensure_fc_tables


class _QueryResultEngine:
    def __init__(self, rows: object) -> None:
        self.rows = rows
        self.calls: list[tuple[str, object]] = []

    def query(self, sql: str, params=()):
        self.calls.append((sql, params))
        return self.rows


class _QueryFailureEngine:
    def query(self, _sql: str, _params=()):
        raise OSError("private catalog failure")


def _open_engine(tmp_path: Path, name: str) -> DuckDBEngine:
    return DuckDBEngine(tmp_path / name)


def test_json_object_probe_never_caches_transport_failure_as_unsupported() -> None:
    engine = _QueryFailureEngine()

    with pytest.raises(RuntimeError, match="^duckdb_json_object_probe_failed$"):
        supports_json_object(engine)  # type: ignore[arg-type]

    assert not hasattr(engine, "_analytix_supports_json_object")


@pytest.mark.parametrize("rows", ([], [(None,)], [(1,), (1,)], [(1, 2)]))
def test_json_object_probe_rejects_missing_or_malformed_results(rows: object) -> None:
    with pytest.raises(RuntimeError, match="^duckdb_json_object_probe_invalid$"):
        supports_json_object(_QueryResultEngine(rows))  # type: ignore[arg-type]


def test_json_object_probe_rechecks_legacy_false_cache_and_accepts_verified_result() -> None:
    engine = _QueryResultEngine([('{"a":"1"}',)])
    engine._analytix_supports_json_object = False

    assert supports_json_object(engine) is True  # type: ignore[arg-type]
    assert engine._analytix_supports_json_object is True
    assert len(engine.calls) == 1


def test_catalog_helpers_preserve_authoritative_empty_and_zero(tmp_path: Path) -> None:
    engine = _open_engine(tmp_path, "catalog-zero.duckdb")
    try:
        assert duckdb_table_exists(engine, "missing_table") is False
        engine.execute("CREATE TABLE source_rows(case_id TEXT, row_hash TEXT)")
        assert duckdb_table_exists(engine, "source_rows") is True
        assert duckdb_table_columns(engine, "source_rows") == {"case_id", "row_hash"}
        assert table_has_rows(engine, "source_rows") is False

        engine.execute("INSERT INTO source_rows VALUES ('case-a', 'hash-a')")
        assert table_has_rows(engine, "source_rows") is True
    finally:
        engine.close()


@pytest.mark.parametrize("rows", ([], [(None,)], [(True,)], [("0",)], [(-1,)]))
def test_table_catalog_missing_null_or_invalid_count_is_not_false(rows: object) -> None:
    with pytest.raises(RuntimeError, match="^duckdb_table_catalog_count_unavailable$"):
        duckdb_table_exists(_QueryResultEngine(rows), "source_rows")  # type: ignore[arg-type]


def test_ensure_columns_propagates_unverified_ddl_failure(tmp_path: Path) -> None:
    delegate = _open_engine(tmp_path, "catalog-ddl.duckdb")
    delegate.execute("CREATE TABLE source_rows(case_id TEXT)")

    class _FailingAlterEngine:
        def query(self, sql: str, params=()):
            return delegate.query(sql, params)

        def execute(self, sql: str, params=()) -> None:
            if "ADD COLUMN row_hash" in sql:
                raise RuntimeError("private ddl failure")
            delegate.execute(sql, params)

    try:
        with pytest.raises(RuntimeError, match="^duckdb_column_add_failed$"):
            ensure_columns(
                _FailingAlterEngine(),  # type: ignore[arg-type]
                "source_rows",
                [("row_hash", "TEXT")],
            )
        assert duckdb_table_columns(delegate, "source_rows") == {"case_id"}
    finally:
        delegate.close()


def test_norm_rebuild_resolves_file_kind_with_case_binding(tmp_path: Path) -> None:
    engine = _open_engine(tmp_path, "norm-case-binding.duckdb")
    try:
        ensure_fc_tables(engine, include_secondary_indexes=False)
        engine.execute(
            "INSERT INTO import_file_log(file_id, case_id, kind) VALUES (?, ?, ?)",
            ("file-a", "case-a", "fc_person"),
        )
        engine.execute(
            "INSERT INTO fc_person_raw(case_id, file_id, row_no, imported_at, row_hash) "
            "VALUES (?, ?, ?, ?, ?)",
            ("case-a", "file-a", 1, "2026-07-22 00:00:00", "hash-a"),
        )

        assert rebuild_norm_for_file(engine, case_id="case-a", file_id="file-a") == 1
        assert rebuild_norm_for_case(engine, case_id="case-b", kind="fc_person") == 0
        with pytest.raises(ValueError, match="^norm_rebuild_file_source_unavailable$"):
            rebuild_norm_for_file(engine, case_id="case-b", file_id="file-a")
        with pytest.raises(ValueError, match="^norm_rebuild_file_kind_mismatch$"):
            rebuild_norm_for_file(
                engine,
                case_id="case-a",
                file_id="file-a",
                kind="fc_account",
            )

        assert engine.query(
            "SELECT COUNT(1) FROM fc_person_norm WHERE case_id='case-a'"
        ) == [(1,)]
        assert engine.query(
            "SELECT COUNT(1) FROM fc_person_norm WHERE case_id='case-b'"
        ) == [(0,)]
    finally:
        engine.close()


def test_norm_file_catalog_query_always_binds_case_and_file() -> None:
    engine = _QueryResultEngine([("fc_person",)])

    assert _kind_for_file(  # type: ignore[arg-type]
        engine,
        case_id="case-a",
        file_id="file-a",
    ) == "fc_person"
    assert "WHERE case_id=? AND file_id=?" in engine.calls[0][0]
    assert engine.calls[0][1] == ("case-a", "file-a")


@pytest.mark.parametrize(
    "rows",
    ([], [(None,)], [(True,)], [("0",)], [(-1,)], [(0,), (0,)], [(0, 1)]),
)
def test_norm_missing_null_or_invalid_count_is_not_zero(rows: object) -> None:
    with pytest.raises(RuntimeError, match="^norm_rebuild_count_unavailable$"):
        _count_norm_rows(  # type: ignore[arg-type]
            _QueryResultEngine(rows),
            "fc_person_norm",
            case_id="case-a",
        )


def test_norm_authoritative_zero_is_preserved() -> None:
    assert _count_norm_rows(  # type: ignore[arg-type]
        _QueryResultEngine([(0,)]),
        "fc_person_norm",
        case_id="case-a",
    ) == 0


def test_privacy_delta_is_case_bound_exactly_counted_and_transaction_friendly(
    tmp_path: Path,
) -> None:
    engine = _open_engine(tmp_path, "privacy-delta-case.duckdb")
    try:
        engine.execute(
            "CREATE TABLE source_rows(case_id TEXT, file_id TEXT, row_hash TEXT)"
        )
        engine.executemany(
            "INSERT INTO source_rows VALUES (?, ?, ?)",
            (
                ("case-a", "file-a", "hash-a"),
                ("case-a", "file-a", "hash-a"),
                ("case-b", "file-b", "hash-b"),
            ),
        )
        ensure_privacy_projection_delta_log(engine)
        assert "batch_id" in duckdb_table_columns(
            engine,
            "privacy_projection_delta_log",
        )

        engine.execute("BEGIN TRANSACTION")
        assert append_privacy_projection_delta_for_case_rows(
            engine,
            case_id="case-a",
            op="insert",
            table="source_rows",
            where_clause="TRUE",
            params=(),
            source="test:case-bound",
        ) == 1
        rows = engine.query(
            "SELECT case_id, file_id, row_hash, batch_id "
            "FROM privacy_projection_delta_log"
        )
        assert len(rows) == 1
        assert rows[0][:3] == ("case-a", "file-a", "hash-a")
        assert isinstance(rows[0][3], str) and rows[0][3]
        engine.execute("ROLLBACK")

        assert engine.query("SELECT COUNT(1) FROM privacy_projection_delta_log") == [(0,)]
        assert append_privacy_projection_delta_for_case_rows(
            engine,
            case_id="case-a",
            op="insert",
            table="source_rows",
            where_clause="file_id=?",
            params=("missing-file",),
            source="test:empty",
        ) == 0
        assert engine.query("SELECT COUNT(1) FROM privacy_projection_delta_log") == [(0,)]
    finally:
        engine.close()


def test_privacy_delta_log_migrates_legacy_schema_without_minting_counts(
    tmp_path: Path,
) -> None:
    engine = _open_engine(tmp_path, "privacy-delta-legacy.duckdb")
    try:
        engine.execute(
            """CREATE TABLE privacy_projection_delta_log(
                   case_id TEXT,
                   op TEXT,
                   file_id TEXT,
                   row_hash TEXT,
                   source TEXT,
                   created_at TEXT
               )"""
        )
        engine.execute(
            "INSERT INTO privacy_projection_delta_log VALUES (?, ?, ?, ?, ?, ?)",
            (
                "case-a",
                "insert",
                "file-a",
                "hash-a",
                "legacy",
                "2026-07-22 00:00:00",
            ),
        )

        ensure_privacy_projection_delta_log(engine)

        assert "batch_id" in duckdb_table_columns(
            engine,
            "privacy_projection_delta_log",
        )
        assert engine.query(
            "SELECT case_id, file_id, row_hash, batch_id "
            "FROM privacy_projection_delta_log"
        ) == [("case-a", "file-a", "hash-a", None)]
    finally:
        engine.close()


def test_privacy_delta_rejects_invalid_identity_and_missing_table(tmp_path: Path) -> None:
    engine = _open_engine(tmp_path, "privacy-delta-invalid.duckdb")
    try:
        engine.execute(
            "CREATE TABLE source_rows(case_id TEXT, file_id TEXT, row_hash TEXT)"
        )
        engine.execute("INSERT INTO source_rows VALUES ('case-a', 'file-a', NULL)")

        with pytest.raises(RuntimeError, match="^privacy_delta_source_identity_invalid$"):
            append_privacy_projection_delta_for_case_rows(
                engine,
                case_id="case-a",
                op="upsert",
                table="source_rows",
                where_clause="TRUE",
                params=(),
                source="test:invalid",
            )
        assert engine.query("SELECT COUNT(1) FROM privacy_projection_delta_log") == [(0,)]

        with pytest.raises(RuntimeError, match="^privacy_delta_source_table_unavailable$"):
            append_privacy_projection_delta_for_case_rows(
                engine,
                case_id="case-a",
                op="delete",
                table="missing_rows",
                where_clause="TRUE",
                params=(),
                source="test:missing",
            )
    finally:
        engine.close()


def test_hash_table_privacy_delta_preserves_zero_and_deduplicates(tmp_path: Path) -> None:
    engine = _open_engine(tmp_path, "privacy-delta-hashes.duckdb")
    try:
        engine.execute("CREATE TABLE hashes(row_hash TEXT)")
        engine.executemany(
            "INSERT INTO hashes VALUES (?)",
            (("hash-a",), ("hash-a",)),
        )
        assert append_privacy_projection_delta_from_hash_table(
            engine,
            case_id="case-a",
            op="delete",
            hash_table="hashes",
            file_id="file-a",
            source="test:hashes",
        ) == 1
        assert engine.query(
            "SELECT case_id, file_id, row_hash FROM privacy_projection_delta_log"
        ) == [("case-a", "file-a", "hash-a")]

        engine.execute("CREATE TABLE empty_hashes(row_hash TEXT)")
        assert append_privacy_projection_delta_from_hash_table(
            engine,
            case_id="case-a",
            op="delete",
            hash_table="empty_hashes",
            file_id="file-a",
            source="test:empty-hashes",
        ) == 0
    finally:
        engine.close()


@pytest.mark.parametrize("rows", ([], [(None,)], [(True,)], [("0",)], [(-1,)]))
def test_privacy_delta_missing_null_or_invalid_count_is_not_zero(rows: object) -> None:
    with pytest.raises(RuntimeError, match="^privacy_delta_count_unavailable$"):
        _single_count_row(rows, code="privacy_delta_count_unavailable")


def test_privacy_delta_authoritative_zero_is_preserved() -> None:
    assert _single_count_row(
        [(0,)],
        code="privacy_delta_count_unavailable",
    ) == 0
