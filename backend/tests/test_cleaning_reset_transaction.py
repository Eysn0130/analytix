from __future__ import annotations

from pathlib import Path
from typing import Any

import pytest

from app.core.db_engine import DuckDBEngine
from app.repositories import cleaning_repository, cleaning_run_setup


class _Cursor:
    def __init__(self, *, import_rows: list[tuple[str, str]] | None = None) -> None:
        self.import_rows = list(import_rows or [("file-txn", "fc_transaction")])
        self.operations: list[str] = []
        self._rows: list[tuple[Any, ...]] = []

    def execute(self, sql: str, params: Any = None) -> "_Cursor":
        statement = " ".join(str(sql).split())
        self.operations.append(statement)
        if statement.startswith("SELECT file_id, kind FROM import_file_log"):
            self._rows = list(self.import_rows)
        return self

    def fetchall(self) -> list[tuple[Any, ...]]:
        return list(self._rows)


class _Connection:
    def __init__(self, cursor: _Cursor) -> None:
        self._cursor = cursor
        self.closed = False

    def cursor(self) -> _Cursor:
        return self._cursor

    def execute(self, sql: str, params: Any = None) -> None:
        self._cursor.execute(sql, params)

    def close(self) -> None:
        self.closed = True


class _Storage:
    def __init__(self, connection: _Connection) -> None:
        self.connection = connection

    def ensure_funds_tables(self, _case_id: str) -> None:
        return None

    def open_case_engine(self, _case_id: str) -> _Connection:
        return self.connection


def _repository(monkeypatch: pytest.MonkeyPatch, *, import_log_exists: bool = True) -> tuple[Any, _Connection]:
    cursor = _Cursor()
    connection = _Connection(cursor)
    repository = cleaning_repository.CleaningRepository()
    repository._storage = _Storage(connection)

    monkeypatch.setattr(
        cleaning_repository.cleaning_connection_setup,
        "configure_cleaning_connection",
        lambda *_args: None,
    )
    monkeypatch.setattr(
        cleaning_repository.cleaning_schema_setup,
        "ensure_cleaning_schema",
        lambda _cur: None,
    )
    monkeypatch.setattr(
        cleaning_repository.cleaning_duckdb_meta,
        "table_exists",
        lambda _cur, table: table != "import_file_log" or import_log_exists,
    )
    monkeypatch.setattr(
        cleaning_repository.cleaning_duckdb_meta,
        "column_exists",
        lambda *_args: False,
    )
    return repository, connection


def test_reset_clean_state_uses_one_explicit_transaction_without_savepoint_or_fallback(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, connection = _repository(monkeypatch)
    monkeypatch.setattr(cleaning_repository, "rebuild_norm_for_case", lambda *_args, **_kwargs: 3)
    monkeypatch.setattr(
        cleaning_repository,
        "append_privacy_projection_delta_for_case_rows",
        lambda *_args, **_kwargs: 1,
    )

    assert repository.reset_clean_state("case-alpha") == {
        "case_id": "case-alpha",
        "rebuilt": True,
        "rebuilt_files": 1,
    }

    operations = connection._cursor.operations
    assert operations.count("BEGIN TRANSACTION") == 1
    assert operations.count("COMMIT") == 1
    assert "ROLLBACK" not in operations
    assert not any("SAVEPOINT" in statement for statement in operations)
    assert not any("SET orig_amount=NULL" in statement for statement in operations)
    assert operations.index("BEGIN TRANSACTION") < operations.index("DELETE FROM cleaning_log WHERE case_id=?")
    assert connection.closed is True


@pytest.mark.parametrize("failure", ["rebuild", "rebuild_count", "privacy", "privacy_count"])
def test_reset_clean_state_rolls_back_every_rebuild_count_or_privacy_failure(
    monkeypatch: pytest.MonkeyPatch,
    failure: str,
) -> None:
    repository, connection = _repository(monkeypatch)

    def rebuild(*_args: Any, **_kwargs: Any) -> int | None:
        if failure == "rebuild":
            raise RuntimeError("rebuild failed")
        if failure == "rebuild_count":
            return None
        return 3

    def privacy(*_args: Any, **_kwargs: Any) -> int | None:
        if failure == "privacy":
            raise RuntimeError("privacy failed")
        if failure == "privacy_count":
            return None
        return 1

    monkeypatch.setattr(cleaning_repository, "rebuild_norm_for_case", rebuild)
    monkeypatch.setattr(cleaning_repository, "append_privacy_projection_delta_for_case_rows", privacy)

    with pytest.raises(RuntimeError):
        repository.reset_clean_state("case-alpha")

    operations = connection._cursor.operations
    assert operations.count("BEGIN TRANSACTION") == 1
    assert operations.count("ROLLBACK") == 1
    assert "COMMIT" not in operations
    assert not any("SAVEPOINT" in statement for statement in operations)
    assert not any("SET orig_amount=NULL" in statement for statement in operations)
    assert connection.closed is True


def test_reset_clean_state_rejects_missing_rebuild_source_without_destructive_fallback(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, connection = _repository(monkeypatch, import_log_exists=False)
    monkeypatch.setattr(
        cleaning_repository,
        "append_privacy_projection_delta_for_case_rows",
        lambda *_args, **_kwargs: 1,
    )

    with pytest.raises(RuntimeError, match="cleaning_reset_rebuild_source_unavailable"):
        repository.reset_clean_state("case-alpha")

    operations = connection._cursor.operations
    assert operations.count("ROLLBACK") == 1
    assert not any("SET orig_amount=NULL" in statement for statement in operations)


def test_reset_clean_state_rolls_back_real_duckdb_mutation_when_privacy_delta_fails(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
) -> None:
    db_path = tmp_path / "cleaning-reset-transaction.duckdb"
    engine = DuckDBEngine(db_path)
    engine.execute("CREATE TABLE reset_probe(value INTEGER)")
    engine.execute("INSERT INTO reset_probe VALUES (1)")
    engine.execute("CREATE TABLE fc_transaction_norm(case_id TEXT)")
    engine.execute("CREATE TABLE import_file_log(case_id TEXT, file_id TEXT, kind TEXT)")
    engine.execute("INSERT INTO import_file_log VALUES ('case-alpha', 'file-txn', 'fc_transaction')")

    repository = cleaning_repository.CleaningRepository()
    repository._storage = _Storage(engine)
    monkeypatch.setattr(
        cleaning_repository.cleaning_connection_setup,
        "configure_cleaning_connection",
        lambda *_args: None,
    )
    monkeypatch.setattr(
        cleaning_repository.cleaning_schema_setup,
        "ensure_cleaning_schema",
        lambda _cur: None,
    )
    monkeypatch.setattr(
        cleaning_repository.cleaning_duckdb_meta,
        "table_exists",
        lambda _cur, table: table in {"fc_transaction_norm", "import_file_log"},
    )

    def rebuild(con: DuckDBEngine, **_kwargs: Any) -> int:
        con.execute("UPDATE reset_probe SET value=99")
        return 1

    monkeypatch.setattr(cleaning_repository, "rebuild_norm_for_case", rebuild)
    monkeypatch.setattr(
        cleaning_repository,
        "append_privacy_projection_delta_for_case_rows",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("privacy failed")),
    )

    with pytest.raises(RuntimeError, match="privacy failed"):
        repository.reset_clean_state("case-alpha")

    reopened = DuckDBEngine(db_path)
    try:
        assert reopened.query("SELECT value FROM reset_probe") == [(1,)]
    finally:
        reopened.close()


class _ScopeCountCursor:
    def __init__(self, row: Any = (1,), *, error: Exception | None = None) -> None:
        self.row = row
        self.error = error

    def execute(self, _sql: str) -> None:
        if self.error is not None:
            raise self.error

    def fetchone(self) -> Any:
        return self.row


@pytest.mark.parametrize("row", [None, (), (None,), (False,), ("0",), (-1,)])
def test_cleaning_scope_count_missing_null_or_noncanonical_is_unavailable(row: Any) -> None:
    with pytest.raises(RuntimeError, match="cleaning_scope_count_unavailable"):
        cleaning_run_setup._read_scope_row_count(_ScopeCountCursor(row))


def test_cleaning_scope_count_query_error_is_unavailable() -> None:
    with pytest.raises(RuntimeError, match="cleaning_scope_count_unavailable"):
        cleaning_run_setup._read_scope_row_count(_ScopeCountCursor(error=RuntimeError("query failed")))


def test_cleaning_scope_count_preserves_explicit_zero_and_positive_values() -> None:
    assert cleaning_run_setup._read_scope_row_count(_ScopeCountCursor((0,))) == 0
    assert cleaning_run_setup._read_scope_row_count(_ScopeCountCursor((7,))) == 7
