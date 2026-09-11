from __future__ import annotations

from pathlib import Path
import shutil

import pytest

from app.core import data_engine_client, db_engine


def _active_case_path(root: Path, case_id: str = "case-1") -> Path:
    path = root / "cases" / case_id / "case.duckdb"
    path.parent.mkdir(parents=True, exist_ok=True)
    return path


def test_active_case_db_path_detection_uses_app_cases_dir(tmp_path, monkeypatch) -> None:
    monkeypatch.setattr(db_engine, "get_app_data_dir", lambda: tmp_path)

    path = _active_case_path(tmp_path, "case-1")

    assert db_engine.is_active_case_db_path(path, case_id="case-1") is True
    assert db_engine.is_active_case_db_path(tmp_path / "snapshots" / "case.duckdb") is False
    assert db_engine.is_active_case_db_path(tmp_path / "cases" / "case-1" / "snapshot.duckdb") is False


def test_case_db_path_rejects_traversal_whitespace_and_symlink_aliases(tmp_path, monkeypatch) -> None:
    monkeypatch.setattr(db_engine, "get_app_data_dir", lambda: tmp_path)
    for case_id in ("../outside", " case-1", "case-1 ", "case/other", ".", ".."):
        with pytest.raises(ValueError, match="case_storage_id_invalid"):
            db_engine.get_case_db_path(case_id)

    outside = tmp_path / "outside"
    outside.mkdir()
    cases = tmp_path / "cases"
    cases.mkdir()
    (cases / "case-link").symlink_to(outside, target_is_directory=True)
    with pytest.raises(ValueError, match="case_storage_path_invalid"):
        db_engine.get_case_db_path("case-link")


def test_direct_read_only_engine_creates_no_temp_directory(tmp_path) -> None:
    db_path = tmp_path / "snapshot" / "snapshot.duckdb"
    db_path.parent.mkdir()
    writable = db_engine.DuckDBEngine(db_path)
    writable.execute("CREATE TABLE values_table(value INTEGER)")
    writable.close()
    shutil.rmtree(db_path.parent / "duckdb_tmp")

    read_only = db_engine.DuckDBEngine(db_path, read_only=True)
    try:
        assert read_only.query("SELECT COUNT(1) FROM values_table") == [(0,)]
        assert not (db_path.parent / "duckdb_tmp").exists()
    finally:
        read_only.close()


@pytest.mark.parametrize("configured", [None, "", "0", "false", "off", "1", "true"])
def test_active_direct_duckdb_gate_cannot_be_enabled_by_environment(
    tmp_path,
    monkeypatch,
    configured: str | None,
) -> None:
    for name in ("ANALYTIX_DISABLE_DIRECT_PYTHON_DUCKDB", "ANALYTIX_DISABLE_DIRECT_DUCKDB_HELPERS"):
        if configured is None:
            monkeypatch.delenv(name, raising=False)
        else:
            monkeypatch.setenv(name, configured)
    monkeypatch.setattr(db_engine, "get_app_data_dir", lambda: tmp_path)
    active_path = _active_case_path(tmp_path)

    assert db_engine.direct_python_duckdb_disabled() is True
    for maintenance in ({}, {"snapshot": True}, {"legacy": True}):
        with pytest.raises(db_engine.DirectActiveDuckDBAccessDisabledError):
            db_engine.assert_python_duckdb_allowed(active_path, reason="environment must not authorize", **maintenance)


def test_duckdb_engine_routes_active_db_through_data_engine_by_default(tmp_path, monkeypatch) -> None:
    monkeypatch.delenv("ANALYTIX_DISABLE_DIRECT_PYTHON_DUCKDB", raising=False)
    monkeypatch.delenv("ANALYTIX_DISABLE_DIRECT_DUCKDB_HELPERS", raising=False)
    monkeypatch.setattr(db_engine, "get_app_data_dir", lambda: tmp_path)
    path = _active_case_path(tmp_path)
    calls: list[dict] = []

    def fake_run(command, *, case_id="", db_path=None, payload=None, timeout=1800):
        calls.append({
            "command": command,
            "case_id": case_id,
            "db_path": db_path,
            "payload": payload,
            "timeout": timeout,
        })
        if command == "duckdb.open_session":
            return {"session_id": "session-1"}
        if command == "duckdb.query":
            return {"columns": ["value"], "rows": [[42]], "row_count": 1}
        return {}

    def fail_connect(*args, **kwargs):
        raise AssertionError("python duckdb.connect must not open active case.duckdb")

    monkeypatch.setattr(data_engine_client, "run_data_engine_command", fake_run)
    monkeypatch.setattr(db_engine.duckdb, "connect", fail_connect)

    engine = db_engine.DuckDBEngine(path)
    try:
        assert engine.query("SELECT 42") == [(42,)]
    finally:
        engine.close()

    assert calls[0]["command"] == "duckdb.open_session"
    assert calls[0]["case_id"] == "case-1"
    assert calls[0]["payload"]["read_only"] is False
    assert any(call["command"] == "duckdb.query" for call in calls)
    assert calls[-1]["command"] == "duckdb.close_session"


@pytest.mark.parametrize("configured", [None, "0", "false"])
def test_duckdb_engine_does_not_fallback_to_direct_connect_when_engine_unavailable(
    tmp_path,
    monkeypatch,
    configured: str | None,
) -> None:
    monkeypatch.delenv("ANALYTIX_DISABLE_DIRECT_PYTHON_DUCKDB", raising=False)
    monkeypatch.delenv("ANALYTIX_DISABLE_DIRECT_DUCKDB_HELPERS", raising=False)
    if configured is not None:
        monkeypatch.setenv("ANALYTIX_DISABLE_DIRECT_PYTHON_DUCKDB", configured)
    monkeypatch.setattr(db_engine, "get_app_data_dir", lambda: tmp_path)
    path = _active_case_path(tmp_path)

    def fail_engine(*args, **kwargs):
        raise data_engine_client.DataEngineUnavailableError("engine down", code="data_engine_unavailable")

    def fail_connect(*args, **kwargs):
        raise AssertionError("python duckdb.connect fallback must not run")

    monkeypatch.setattr(data_engine_client, "run_data_engine_command", fail_engine)
    monkeypatch.setattr(db_engine.duckdb, "connect", fail_connect)

    with pytest.raises(data_engine_client.DataEngineUnavailableError, match="engine down"):
        db_engine.DuckDBEngine(path)


def test_active_db_uses_fixed_unavailable_admission_without_direct_fallback(tmp_path, monkeypatch) -> None:
    monkeypatch.delenv("ANALYTIX_DISABLE_DIRECT_PYTHON_DUCKDB", raising=False)
    monkeypatch.delenv("ANALYTIX_DISABLE_DIRECT_DUCKDB_HELPERS", raising=False)
    monkeypatch.setattr(db_engine, "get_app_data_dir", lambda: tmp_path)
    path = _active_case_path(tmp_path, "case-account-6222020200000000000")

    def fail_connect(*args, **kwargs):
        raise AssertionError("fixed unavailable admission must never fall back to duckdb.connect")

    monkeypatch.setattr(db_engine.duckdb, "connect", fail_connect)

    with pytest.raises(data_engine_client.DataEngineUnavailableError) as raised:
        db_engine.DuckDBEngine(path)

    assert raised.value.code == "managed_process_capability_admission_unavailable"
    assert str(path) not in str(raised.value)
    assert "6222020200000000000" not in str(raised.value)


def test_fake_data_engine_session_never_enables_direct_fallback(tmp_path, monkeypatch) -> None:
    monkeypatch.setattr(db_engine, "get_app_data_dir", lambda: tmp_path)
    path = _active_case_path(tmp_path)

    def fake_engine(*args, **kwargs):
        return {"session_id": ""}

    def fail_connect(*args, **kwargs):
        raise AssertionError("an invalid data-engine session must not fall back to duckdb.connect")

    monkeypatch.setattr(data_engine_client, "run_data_engine_command", fake_engine)
    monkeypatch.setattr(db_engine.duckdb, "connect", fail_connect)

    with pytest.raises(RuntimeError, match="did not return duckdb session_id"):
        db_engine.DuckDBEngine(path)


def test_direct_python_duckdb_guard_blocks_active_db_and_allows_snapshot(tmp_path, monkeypatch) -> None:
    monkeypatch.setenv("ANALYTIX_DISABLE_DIRECT_PYTHON_DUCKDB", "false")
    monkeypatch.setattr(db_engine, "get_app_data_dir", lambda: tmp_path)
    active_path = _active_case_path(tmp_path)
    snapshot_path = tmp_path / "snapshots" / "case.duckdb"

    with pytest.raises(db_engine.DirectActiveDuckDBAccessDisabledError) as raised:
        db_engine.assert_python_duckdb_allowed(
            active_path,
            reason="account 6222020200000000000 must not enter public errors",
        )

    assert raised.value.code == "direct_active_duckdb_access_disabled"
    assert str(active_path) not in str(raised.value)
    assert "6222020200000000000" not in str(raised.value)
    assert "admitted Analytix data engine" in str(raised.value)
    with pytest.raises(db_engine.DirectActiveDuckDBAccessDisabledError):
        db_engine.assert_python_duckdb_allowed(active_path, reason="snapshot bypass", snapshot=True)
    with pytest.raises(db_engine.DirectActiveDuckDBAccessDisabledError):
        db_engine.assert_python_duckdb_allowed(active_path, reason="legacy bypass", legacy=True)
    db_engine.assert_python_duckdb_allowed(snapshot_path, reason="snapshot", snapshot=True)
    db_engine.assert_python_duckdb_allowed(tmp_path / "legacy" / "legacy.duckdb", reason="legacy", legacy=True)


def test_snapshot_database_outside_active_case_remains_an_explicit_direct_path(tmp_path, monkeypatch) -> None:
    monkeypatch.setattr(db_engine, "get_app_data_dir", lambda: tmp_path)
    snapshot_path = tmp_path / "snapshots" / "case.duckdb"
    snapshot_path.parent.mkdir(parents=True, exist_ok=True)
    real_connect = db_engine.duckdb.connect
    opened: list[Path] = []

    def track_connect(path, *args, **kwargs):
        opened.append(Path(path).resolve())
        return real_connect(path, *args, **kwargs)

    monkeypatch.setattr(db_engine.duckdb, "connect", track_connect)

    engine = db_engine.DuckDBEngine(snapshot_path)
    try:
        assert engine.query("SELECT 42") == [(42,)]
    finally:
        engine.close()

    assert opened == [snapshot_path.resolve()]


def test_remote_cursor_supports_fetchone_fetchmany_and_description(tmp_path, monkeypatch) -> None:
    monkeypatch.delenv("ANALYTIX_DISABLE_DIRECT_DUCKDB_HELPERS", raising=False)
    monkeypatch.setattr(db_engine, "get_app_data_dir", lambda: tmp_path)
    path = _active_case_path(tmp_path)

    def fake_run(command, *, case_id="", db_path=None, payload=None, timeout=1800):
        if command == "duckdb.open_session":
            return {"session_id": "session-1"}
        if command == "duckdb.query":
            return {"columns": ["name"], "rows": [["alpha"], ["beta"]], "row_count": 2}
        return {}

    monkeypatch.setattr(data_engine_client, "run_data_engine_command", fake_run)

    engine = db_engine.DuckDBEngine(path)
    try:
        cursor = engine.cursor().execute("SELECT name FROM fc_file")
        assert [item[0] for item in cursor.description] == ["name"]
        assert cursor.fetchone() == ("alpha",)
        assert cursor.fetchmany(10) == [("beta",)]
        assert cursor.fetchone() is None
    finally:
        engine.close()


def test_remote_cursor_routes_write_sql_to_execute(tmp_path, monkeypatch) -> None:
    monkeypatch.delenv("ANALYTIX_DISABLE_DIRECT_PYTHON_DUCKDB", raising=False)
    monkeypatch.setattr(db_engine, "get_app_data_dir", lambda: tmp_path)
    path = _active_case_path(tmp_path)
    commands: list[str] = []

    def fake_run(command, *, case_id="", db_path=None, payload=None, timeout=1800):
        commands.append(command)
        if command == "duckdb.open_session":
            return {"session_id": "session-1"}
        return {}

    monkeypatch.setattr(data_engine_client, "run_data_engine_command", fake_run)

    engine = db_engine.DuckDBEngine(path)
    try:
        cursor = engine.cursor().execute("CREATE TABLE items(value INTEGER)")
        assert cursor.fetchall() == []
    finally:
        engine.close()

    assert "duckdb.execute" in commands
    assert "duckdb.query" not in commands


def test_remote_commit_is_noop_without_active_transaction(tmp_path, monkeypatch) -> None:
    monkeypatch.delenv("ANALYTIX_DISABLE_DIRECT_PYTHON_DUCKDB", raising=False)
    monkeypatch.setattr(db_engine, "get_app_data_dir", lambda: tmp_path)
    path = _active_case_path(tmp_path)
    execute_sql: list[str] = []

    def fake_run(command, *, case_id="", db_path=None, payload=None, timeout=1800):
        if command == "duckdb.open_session":
            return {"session_id": "session-1"}
        if command == "duckdb.execute":
            execute_sql.append(str((payload or {}).get("sql") or ""))
        return {}

    monkeypatch.setattr(data_engine_client, "run_data_engine_command", fake_run)

    engine = db_engine.DuckDBEngine(path)
    try:
        engine.commit()
        engine.execute("BEGIN TRANSACTION")
        engine.commit()
        engine.commit()
    finally:
        engine.close()

    transaction_sql = [sql for sql in execute_sql if sql in {"BEGIN TRANSACTION", "COMMIT"}]
    assert transaction_sql == ["BEGIN TRANSACTION", "COMMIT"]


def test_data_engine_product_error_maps_direct_python_guard() -> None:
    sensitive_path = "/cases/case-account-6222020200000000000/case.duckdb"
    error = db_engine.DirectActiveDuckDBAccessDisabledError(
        sensitive_path,
        reason="account 6222020200000000000",
    )

    mapped = data_engine_client.data_engine_product_error(error)

    assert mapped is not None
    assert mapped["status_code"] == 409
    assert mapped["code"] == "DIRECT_ACTIVE_DUCKDB_ACCESS_DISABLED"
    assert "数据引擎统一接管" in mapped["message"]
    assert sensitive_path not in str(error)
    assert sensitive_path not in str(mapped)
    assert "6222020200000000000" not in str(error)
    assert "6222020200000000000" not in str(mapped)


def test_product_source_scan_requires_direct_python_duckdb_gate() -> None:
    repo_root = Path(__file__).resolve().parents[2]
    app_root = repo_root / "backend/app"
    db_engine_source = (app_root / "core/db_engine.py").read_text(encoding="utf-8")
    duckdb_connect_allowlist = {
        Path("core/db_engine.py"): "central DuckDBEngine implementation either proxies active DB through data engine or guards direct opens",
    }
    offenders: list[str] = []

    for path in app_root.rglob("*.py"):
        rel = path.relative_to(app_root)
        text = path.read_text(encoding="utf-8")
        if "duckdb.connect(" in text and rel not in duckdb_connect_allowlist:
            offenders.append(str(rel))

    assert "ANALYTIX_DISABLE_DIRECT_PYTHON_DUCKDB" not in db_engine_source
    assert "ANALYTIX_DISABLE_DIRECT_DUCKDB_HELPERS" not in db_engine_source
    assert offenders == []
