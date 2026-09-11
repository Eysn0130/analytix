from __future__ import annotations

import inspect
import json
import os
import subprocess
import sys
import threading
from pathlib import Path

import pytest

from app.core import (
    data_engine_client,
    managed_subprocess,
    native_component_paths,
    process_environment,
)


_BLOCKER = "managed_process_capability_admission_unavailable"
_SENSITIVE_ACCOUNT = "6222020202020202020"


class _PoisonArgument:
    def __init__(self, marker: Path) -> None:
        self._marker = marker

    def _inspected(self) -> None:
        self._marker.write_text("argument inspected", encoding="utf-8")
        raise AssertionError("data-engine argument was inspected before admission")

    def __bool__(self) -> bool:
        self._inspected()

    def __bytes__(self) -> bytes:
        self._inspected()

    def __float__(self) -> float:
        self._inspected()

    def __fspath__(self) -> str:
        self._inspected()

    def __iter__(self):
        self._inspected()

    def __str__(self) -> str:
        self._inspected()

    def exists(self) -> bool:
        self._inspected()

    def stat(self):
        self._inspected()


def _assert_fixed_blocker(error: data_engine_client.DataEngineUnavailableError) -> None:
    assert error.code == _BLOCKER
    assert error.message == _BLOCKER
    assert str(error) == _BLOCKER
    assert error.diagnostics == {}
    assert error.raw_error is None
    assert _SENSITIVE_ACCOUNT not in str(error)
    assert error.__cause__ is None
    assert error.__context__ is None


def test_data_engine_rejects_before_case_arguments_are_inspected(tmp_path: Path) -> None:
    marker = tmp_path / "argument-inspected"
    poison = _PoisonArgument(marker)

    with pytest.raises(data_engine_client.DataEngineUnavailableError) as raised:
        data_engine_client.run_data_engine_command(
            poison,
            case_id=poison,
            db_path=poison,
            payload=poison,
            timeout=poison,
        )

    _assert_fixed_blocker(raised.value)
    assert not marker.exists()


def test_data_engine_env_binary_and_global_authority_cannot_enable_consumer(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    execution_marker = tmp_path / "binary-executed"
    binary = tmp_path / "analytix-data-engine"
    binary.write_text(
        "\n".join(
            (
                f"#!{sys.executable}",
                "from pathlib import Path",
                f"Path({os.fspath(execution_marker)!r}).write_text('unsafe')",
                "",
            )
        ),
        encoding="utf-8",
    )
    binary.chmod(0o700)
    monkeypatch.setenv("ANALYTIX_DATA_ENGINE_BIN", os.fspath(binary))
    monkeypatch.setenv("ANALYTIX_DATA_ENGINE_DISABLED", "0")

    class GloballyAvailableAuthority:
        def status(self) -> managed_subprocess.ManagedExecutionAuthorityStatus:
            return managed_subprocess.ManagedExecutionAuthorityStatus(
                available=True,
                protocol_version=managed_subprocess._AUTHORITY_PROTOCOL_VERSION,
                platform=sys.platform,
                reason_code="",
                guarantees=managed_subprocess._required_authority_guarantees(sys.platform),
            )

        def execute(self, _request) -> managed_subprocess.ManagedProcessResult:
            execution_marker.write_text("unsafe", encoding="utf-8")
            return managed_subprocess.ManagedProcessResult(returncode=0)

    monkeypatch.setattr(
        managed_subprocess,
        "_production_execution_authority",
        lambda: GloballyAvailableAuthority(),
    )

    def must_not_discover(*_args, **_kwargs):
        execution_marker.write_text("unsafe", encoding="utf-8")
        raise AssertionError("path or environment discovery ran before admission")

    monkeypatch.setattr(
        native_component_paths,
        "resolve_canonical_native_component",
        must_not_discover,
    )
    monkeypatch.setattr(
        native_component_paths,
        "resolve_explicit_native_binary",
        must_not_discover,
    )
    monkeypatch.setattr(
        process_environment,
        "build_sanitized_subprocess_env",
        must_not_discover,
    )
    monkeypatch.setattr(subprocess, "Popen", must_not_discover)
    monkeypatch.setattr(subprocess, "run", must_not_discover)
    monkeypatch.setattr(threading, "Thread", must_not_discover)
    data_engine_client.data_engine_binary.cache_clear()

    assert data_engine_client.data_engine_binary() is None
    assert data_engine_client.data_engine_binary_health() == {
        "data_engine_available": False,
        "data_engine_reason": _BLOCKER,
        "data_engine_bin": "",
        "data_engine_pid": None,
    }
    with pytest.raises(data_engine_client.DataEngineUnavailableError) as raised:
        data_engine_client.run_data_engine_command(
            f"case-{_SENSITIVE_ACCOUNT}",
            case_id=f"case-{_SENSITIVE_ACCOUNT}",
            db_path=tmp_path / f"case-{_SENSITIVE_ACCOUNT}.duckdb",
            payload={"account": _SENSITIVE_ACCOUNT},
        )

    _assert_fixed_blocker(raised.value)
    assert not execution_marker.exists()
    assert not any(
        worker.name == "analytix-data-engine-io" for worker in threading.enumerate()
    )


def test_data_engine_private_launch_entry_is_closed_without_reading_binary(
    tmp_path: Path,
) -> None:
    marker = tmp_path / "binary-inspected"
    poison = _PoisonArgument(marker)
    client = data_engine_client._DataEngineClient(poison)

    with pytest.raises(data_engine_client.DataEngineUnavailableError) as raised:
        client._ensure_process()

    _assert_fixed_blocker(raised.value)
    assert client.pid is None
    assert not marker.exists()


@pytest.mark.parametrize(
    "configured",
    (None, "", "0", "false", "off", "1", "true", "on"),
)
def test_direct_duckdb_helper_gate_cannot_be_disabled_by_environment(
    monkeypatch: pytest.MonkeyPatch,
    configured: str | None,
) -> None:
    if configured is None:
        monkeypatch.delenv("ANALYTIX_DISABLE_DIRECT_DUCKDB_HELPERS", raising=False)
    else:
        monkeypatch.setenv("ANALYTIX_DISABLE_DIRECT_DUCKDB_HELPERS", configured)

    assert data_engine_client.direct_duckdb_helpers_disabled() is True


def test_data_engine_health_and_source_have_no_sensitive_or_process_projection(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("ANALYTIX_DATA_ENGINE_BIN", f"/private/{_SENSITIVE_ACCOUNT}")

    health = data_engine_client.data_engine_binary_health()
    source = inspect.getsource(data_engine_client)

    assert _SENSITIVE_ACCOUNT not in json.dumps(health, ensure_ascii=False)
    assert health["data_engine_pid"] is None
    assert "subprocess" not in source
    assert "Popen" not in source
    assert "_probe_data_engine" not in source
    assert "ANALYTIX_DATA_ENGINE_BIN" not in source
