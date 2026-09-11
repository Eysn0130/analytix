from __future__ import annotations

from pathlib import Path

import pytest

from app.core import diagnostic_duckdb_migration as diagnostic_migration_module
from app.core import storage as storage_module
from app.domain.case_source_state import CaseSourceUnavailableError


class _EngineCapture:
    calls: list[tuple[Path, bool]] = []

    def __init__(self, path: Path, *, read_only: bool = False) -> None:
        self.calls.append((Path(path), read_only))

    def close(self) -> None:
        return None


def _storage(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> storage_module.CaseStorage:
    monkeypatch.setattr(storage_module, "get_app_data_dir", lambda: tmp_path)
    instance = storage_module.CaseStorage()
    instance._registry = {
        case_id: {
            "status": "进行中",
            "deleted_at": "",
            "lifecycle_generation": 1,
        }
        for case_id in ("case-read", "case-write", "case-missing", "case-link")
    }
    monkeypatch.setattr(storage_module, "DuckDBEngine", _EngineCapture)
    monkeypatch.setattr(
        diagnostic_migration_module,
        "assert_duckdb_diagnostic_migration_settled",
        lambda _engine, **_kwargs: None,
    )
    return instance


def test_open_case_engine_preserves_exact_read_only_authority(tmp_path, monkeypatch) -> None:
    instance = _storage(tmp_path, monkeypatch)
    db_path = tmp_path / "cases" / "case-read" / "case.duckdb"
    db_path.parent.mkdir(parents=True, exist_ok=True)
    db_path.touch()
    monkeypatch.setattr(instance, "case_db", lambda _case_id: db_path)
    monkeypatch.setattr(instance, "_ensure_case_db", lambda _case_id: pytest.fail("read-only open mutated the database"))
    validated: list[object] = []
    monkeypatch.setattr(
        diagnostic_migration_module,
        "assert_duckdb_diagnostic_migration_settled",
        lambda engine, **_kwargs: validated.append(engine),
    )
    _EngineCapture.calls.clear()

    opened = instance.open_case_engine("case-read", read_only=True)

    assert _EngineCapture.calls == [(db_path, True)]
    assert validated == [opened]


def test_read_only_open_never_initializes_or_migrates_missing_database(tmp_path, monkeypatch) -> None:
    instance = _storage(tmp_path, monkeypatch)
    db_path = tmp_path / "cases" / "case-missing" / "case.duckdb"
    monkeypatch.setattr(instance, "case_db", lambda _case_id: db_path)
    monkeypatch.setattr(instance, "_ensure_case_db", lambda _case_id: pytest.fail("read-only open initialized the database"))
    _EngineCapture.calls.clear()

    with pytest.raises(FileNotFoundError, match="unavailable for read-only access"):
        instance.open_case_engine("case-missing", read_only=True)

    assert _EngineCapture.calls == []


def test_public_case_reads_never_initialize_missing_database_or_publish_empty_source(
    tmp_path,
    monkeypatch,
) -> None:
    instance = _storage(tmp_path, monkeypatch)
    db_path = tmp_path / "cases" / "case-missing" / "case.duckdb"
    monkeypatch.setattr(instance, "case_db", lambda _case_id: db_path)
    monkeypatch.setattr(
        instance,
        "_ensure_case_db",
        lambda _case_id: pytest.fail("public read initialized the database"),
    )

    def tree_snapshot() -> dict[str, bytes]:
        return {
            str(path.relative_to(tmp_path)): path.read_bytes()
            for path in tmp_path.rglob("*")
            if path.is_file()
        }

    before = tree_snapshot()
    for read in (
        lambda: instance.list_import_files("case-missing"),
        lambda: instance.get_case_import_overview("case-missing"),
        lambda: instance.get_case_stats("case-missing"),
        lambda: instance.list_datasets("case-missing"),
    ):
        with pytest.raises(CaseSourceUnavailableError):
            read()
        assert tree_snapshot() == before

    assert not db_path.exists()


def test_import_file_read_rejects_foreign_case_rows_before_projection(tmp_path, monkeypatch) -> None:
    instance = _storage(tmp_path, monkeypatch)

    class _ForeignRowEngine:
        closed = False

        def query(self, sql: str, _params=()):
            if "information_schema.columns" in sql:
                return [("file_id",), ("case_id",)]
            if "COUNT(1)" in sql:
                return [(1,)]
            pytest.fail("foreign rows reached ordinary projection")

        def close(self) -> None:
            self.closed = True

    engine = _ForeignRowEngine()
    monkeypatch.setattr(instance, "open_case_engine", lambda *_args, **_kwargs: engine)
    monkeypatch.setattr(storage_module, "_table_exists", lambda *_args: True)

    with pytest.raises(CaseSourceUnavailableError, match="case_import_binding_mismatch"):
        instance.list_import_files("case-read")

    assert engine.closed is True


def test_write_open_retains_explicit_write_authority(tmp_path, monkeypatch) -> None:
    instance = _storage(tmp_path, monkeypatch)
    db_path = tmp_path / "cases" / "case-write" / "case.duckdb"
    monkeypatch.setattr(instance, "case_db", lambda _case_id: db_path)
    initialized: list[str] = []

    def ensure(case_id: str) -> None:
        initialized.append(case_id)
        db_path.parent.mkdir(parents=True, exist_ok=True)
        db_path.touch()

    monkeypatch.setattr(instance, "_ensure_case_db", ensure)
    migrated: list[object] = []
    monkeypatch.setattr(
        diagnostic_migration_module,
        "migrate_legacy_duckdb_diagnostics",
        lambda engine, **_kwargs: migrated.append(engine),
    )
    _EngineCapture.calls.clear()

    opened = instance.open_case_engine("case-write", read_only=False)

    assert initialized == ["case-write"]
    assert _EngineCapture.calls == [(db_path, False)]
    assert migrated == [opened]


def test_case_storage_rejects_traversal_and_symlink_aliases(tmp_path, monkeypatch) -> None:
    instance = _storage(tmp_path, monkeypatch)
    outside = tmp_path / "outside"

    with pytest.raises(KeyError, match="case_registry_binding_invalid"):
        instance.open_case_engine("../outside", read_only=False)

    assert not outside.exists()
    outside.mkdir()
    (tmp_path / "cases" / "case-link").symlink_to(outside, target_is_directory=True)
    with pytest.raises(RuntimeError, match="case_storage_path_invalid"):
        instance.case_dir("case-link")
