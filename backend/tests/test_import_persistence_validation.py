from __future__ import annotations

import pytest

from app.core.db_engine import DuckDBEngine
from app.core.fc_import_file_log import update_import_progress, upsert_import_file_log
from app.domain.import_service import ImportService
from app.repositories.import_repository import ImportRepository
from app.repositories.import_repository import PreparedImportFile


class _NoEffectEngine:
    def __init__(self) -> None:
        self.calls: list[tuple[str, object]] = []

    def query(self, sql: str, params=()):
        self.calls.append((sql, params))
        return []

    def execute(self, sql: str, params=()) -> None:
        self.calls.append((sql, params))


@pytest.mark.parametrize(("field", "value"), (("size", None), ("size", True), ("source_size", None), ("source_size", True)))
def test_prepared_import_snapshot_rejects_missing_or_invalid_sizes(tmp_path, field: str, value: object) -> None:
    prepared = PreparedImportFile(
        file_id="file-a",
        display_name="transactions.csv",
        display_path="transactions.csv",
        real_path=tmp_path / "transactions.csv",
        file_type="CSV",
        size=1,
        rows_total=0,
        source_size=1,
    )
    setattr(prepared, field, value)

    with pytest.raises(RuntimeError, match=rf"prepared import {field} is unavailable"):
        ImportService._prepared_file_snapshot(prepared)


@pytest.mark.parametrize(
    ("field", "value"),
    (
        ("size", True),
        ("size", -1),
        ("size", 9_007_199_254_740_992),
        ("rows_total", True),
        ("rows_total", -1),
        ("rows_total", 9_007_199_254_740_992),
        ("rows_skipped_non_data", True),
        ("rows_skipped_non_data", -1),
        ("rows_skipped_non_data", 9_007_199_254_740_992),
    ),
)
def test_import_log_never_coerces_missing_or_invalid_counts_to_zero(field: str, value: object) -> None:
    engine = _NoEffectEngine()
    kwargs = {
        "file_id": "file-a",
        "case_id": "case-a",
        "kind": "fc_transaction",
        "filename": "transactions.csv",
        "display_path": "transactions.csv",
        "stored_path": "/controlled/source.csv",
        "file_type": "CSV",
        "size": 1,
        "md5": "",
        "sha256": "a" * 64,
        "rows_total": 1,
        "status": "导入中",
        "rows_skipped_non_data": 0,
    }
    kwargs[field] = value

    with pytest.raises(ValueError, match=rf"^import_log_{field}_invalid$"):
        upsert_import_file_log(engine, **kwargs)  # type: ignore[arg-type]

    assert engine.calls == []


@pytest.mark.parametrize("value", (True, -1))
def test_import_progress_rejects_invalid_required_count_before_write(value: object) -> None:
    engine = _NoEffectEngine()

    with pytest.raises(ValueError, match="^import_log_rows_imported_invalid$"):
        update_import_progress(engine, "file-a", value)  # type: ignore[arg-type]

    assert engine.calls == []


def test_validate_import_persistence_empty_file_ids(tmp_path, monkeypatch) -> None:
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_DIR", str(tmp_path / "app-data"))

    repository = ImportRepository()
    case_id = "case-empty-validation"

    result = repository.validate_import_persistence(case_id, file_ids=[])

    assert result["case_id"] == case_id
    assert result["file_ids"] == []
    assert result["import_file_log_count"] == 0
    assert result["rows_imported_norm"] == 0
    assert result["norm_rows_by_kind"] == {}


def test_validate_import_persistence_counts_import_log_and_norm_rows(tmp_path, monkeypatch) -> None:
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_DIR", str(tmp_path / "app-data"))

    repository = ImportRepository()
    case_id = "case-persistence-validation"
    # This is a repository-query unit test, not an active case-engine transport
    # test. Keep its local fixture outside cases/<case>/case.duckdb so it cannot
    # weaken or bypass the production data-engine admission invariant.
    fixture_db = tmp_path / "maintenance-fixtures" / "import-validation.duckdb"
    fixture_db.parent.mkdir(parents=True, exist_ok=True)
    engine = DuckDBEngine(fixture_db)
    try:
        repository.ensure_tables(engine, include_secondary_indexes=False)
        upsert_import_file_log(
            engine,
            file_id="txn-file-1",
            case_id=case_id,
            kind="fc_transaction",
            filename="transactions.csv",
            display_path="transactions.csv",
            stored_path="/tmp/transactions.csv",
            file_type="CSV",
            size=100,
            md5="",
            sha256="",
            rows_total=2,
            status="导入中",
        )
        update_import_progress(
            engine,
            "txn-file-1",
            2,
            status="已完成",
            rows_imported_raw=2,
            rows_imported_norm=2,
            rows_dedup=0,
            rows_error=0,
            rows_skipped_non_data=0,
        )
        engine.execute(
            """INSERT INTO fc_transaction_norm(case_id, file_id, row_no, imported_at, row_hash)
               VALUES (?, ?, 1, '2026-07-08 00:00:00', 'hash-1'),
                      (?, ?, 2, '2026-07-08 00:00:00', 'hash-2')""",
            (case_id, "txn-file-1", case_id, "txn-file-1"),
        )
    finally:
        engine.close()

    def open_fixture_case(requested_case_id: str) -> DuckDBEngine:
        assert requested_case_id == case_id
        return DuckDBEngine(fixture_db)

    monkeypatch.setattr(repository.storage, "open_case_engine", open_fixture_case)

    result = repository.validate_import_persistence(case_id, file_ids=["txn-file-1"])

    assert result["import_file_log_count"] == 1
    assert result["rows_imported_norm"] == 2
    assert result["norm_rows_by_kind"] == {"fc_transaction": 2}


@pytest.mark.parametrize(
    "manifest_row",
    (
        ("txn-file-1", "fc_transaction", None, 1, "已完成", "done"),
        ("txn-file-1", "fc_transaction", 2, 1, "导入中", "done"),
        ("txn-file-1", "fc_transaction", 2, 1, "已完成", None),
        ("txn-file-1", "", 2, 1, "已完成", "done"),
        ("txn-file-1", "fc_transaction", 2, None, "已完成", "done"),
    ),
)
def test_validate_import_persistence_rejects_incomplete_manifest_before_zero_coercion(
    tmp_path,
    monkeypatch,
    manifest_row,
) -> None:
    class _ManifestEngine:
        def query(self, sql: str, _params=()):
            if "FROM import_file_log" in sql:
                return [manifest_row]
            pytest.fail(f"normalized row query ran after invalid manifest: {sql}")

        def close(self) -> None:
            return None

    repository = ImportRepository()
    monkeypatch.setattr(repository.storage, "open_case_engine", lambda _case_id: _ManifestEngine())
    monkeypatch.setattr(repository, "_table_exists", lambda _engine, _table: True)

    with pytest.raises(RuntimeError, match="manifest is incomplete"):
        repository.validate_import_persistence("case-a", file_ids=["txn-file-1"])


@pytest.mark.parametrize(
    ("files_summary", "persistence"),
    (
        (
            [{"status": "succeeded", "file_id": "file-a", "kind": "fc_transaction"}],
            {},
        ),
        (
            [{"status": "succeeded", "file_id": "file-a", "kind": "fc_transaction", "rows_imported_norm": 2}],
            {"import_file_log_count": 1, "rows_imported_norm": None, "norm_rows_by_kind": {"fc_transaction": 2}},
        ),
        (
            [{"status": "succeeded", "file_id": "file-a", "kind": "fc_transaction", "rows_imported_norm": 2}],
            {"import_file_log_count": 1, "rows_imported_norm": 2, "norm_rows_by_kind": {}},
        ),
    ),
)
def test_import_service_persistence_consumer_rejects_missing_counts(files_summary, persistence) -> None:
    class _Repository:
        def validate_import_persistence(self, _case_id: str, *, file_ids, engine=None):
            assert file_ids == ["file-a"]
            assert engine is None
            return persistence

    service = object.__new__(ImportService)
    service._repository = _Repository()

    with pytest.raises(RuntimeError, match="import persistence"):
        service._validate_import_persistence(case_id="case-a", summary={}, files_summary=files_summary)
