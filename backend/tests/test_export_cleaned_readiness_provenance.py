from __future__ import annotations

import re
from pathlib import Path

import pytest

from app.core.import_count_semantics import CLEANING_COUNTS_VERSION, IMPORT_COUNTS_VERSION
from app.repositories import export_repository as export_repository_module
from app.repositories.export_repository import ExportRepository


_REQUIRED_COLUMNS = {
    "kind",
    "rows_imported_norm",
    "import_counts_version",
    "status",
    "finished_at",
    "cleaned_status",
    "cleaned_finished_at",
    "cleaned_rows_affected",
    "cleaning_counts_version",
}


class _Storage:
    def __init__(self) -> None:
        self.ensure_calls: list[str] = []

    def ensure_funds_tables(self, case_id: str) -> None:
        self.ensure_calls.append(case_id)

    @staticmethod
    def case_db(_case_id: str) -> Path:
        return Path("unused.duckdb")


class _Engine:
    def __init__(
        self,
        rows: list[tuple],
        *,
        columns: set[str] | None = None,
        table_exists: bool = True,
    ) -> None:
        self.rows = rows
        self.columns = _REQUIRED_COLUMNS if columns is None else columns
        self.table_exists = table_exists
        self.closed = False

    def query(self, sql: str, params=None) -> list[tuple]:
        if "information_schema.tables" in sql:
            return [(1,)] if self.table_exists else []
        if "information_schema.columns" in sql:
            return [(column,) for column in sorted(self.columns)]
        if "FROM import_file_log" in sql:
            assert tuple(params or ()) == ("case-alpha",)
            return self.rows
        raise AssertionError(f"unexpected query: {sql}")

    def close(self) -> None:
        self.closed = True


def _row(
    *,
    kind: str = "fc_transaction",
    rows_imported_norm: object = 1,
    import_version: object = IMPORT_COUNTS_VERSION,
    status: object = "已完成",
    finished_at: object = "2026-07-21T00:00:00Z",
    cleaned_status: object = "done",
    cleaned_finished_at: object = "2026-07-21T00:01:00Z",
    cleaned_rows_affected: object = 0,
    cleaning_version: object = CLEANING_COUNTS_VERSION,
) -> tuple:
    return (
        kind,
        rows_imported_norm,
        import_version,
        status,
        finished_at,
        cleaned_status,
        cleaned_finished_at,
        cleaned_rows_affected,
        cleaning_version,
    )


def _repository(monkeypatch: pytest.MonkeyPatch, engine: _Engine) -> tuple[ExportRepository, _Storage]:
    storage = _Storage()
    monkeypatch.setattr(export_repository_module, "DuckDBEngine", lambda _path: engine)
    return ExportRepository(storage=storage), storage  # type: ignore[arg-type]


@pytest.mark.parametrize(
    ("rows", "expected"),
    (
        ([_row(rows_imported_norm=0, import_version=None)], "import count provenance unavailable"),
        ([_row(rows_imported_norm=None)], "import count provenance unavailable"),
        ([_row(status="running", finished_at=None)], "import scope file(s) not completed"),
        ([_row(status="已完成", finished_at=None)], "import scope file(s) not completed"),
        ([_row(rows_imported_norm=0)], "no transaction data in cleaning scope"),
        ([_row(cleaned_status="running")], "1 cleaning scope file(s) not completed"),
        ([_row(cleaned_finished_at=None)], "1 cleaning scope file(s) not completed"),
        ([_row(cleaning_version=None)], "cleaning count provenance unavailable"),
        ([_row(cleaned_rows_affected=None)], "cleaning count provenance unavailable"),
    ),
)
def test_cleaned_export_unknown_or_incomplete_counts_fail_closed(
    monkeypatch: pytest.MonkeyPatch,
    rows: list[tuple],
    expected: str,
) -> None:
    engine = _Engine(rows)
    repository, storage = _repository(monkeypatch, engine)

    with pytest.raises(
        ValueError,
        match=rf"^cleaned dataset not ready: {re.escape(expected)}$",
    ):
        repository.ensure_cleaned_export_ready("case-alpha")

    assert storage.ensure_calls == ["case-alpha"]
    assert engine.closed is True


def test_cleaned_export_requires_import_manifest_schema(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    engine = _Engine([], columns=_REQUIRED_COLUMNS - {"import_counts_version"})
    repository, _storage = _repository(monkeypatch, engine)

    with pytest.raises(
        ValueError,
        match="^cleaned dataset not ready: import count provenance unavailable$",
    ):
        repository.ensure_cleaned_export_ready("case-alpha")


def test_cleaned_export_accepts_only_current_completed_import_and_cleaning_provenance(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    engine = _Engine(
        [
            _row(),
            _row(kind="fc_account", rows_imported_norm=2, cleaned_rows_affected=1),
        ]
    )
    repository, storage = _repository(monkeypatch, engine)

    repository.ensure_cleaned_export_ready("case-alpha")

    assert storage.ensure_calls == ["case-alpha"]
    assert engine.closed is True
