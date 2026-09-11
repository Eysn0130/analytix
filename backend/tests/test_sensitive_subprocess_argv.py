from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path

import pytest

import app.core.archive_extraction as archive_extraction
import app.core.local_field_mapping_model as field_mapping_model
from app.core.external_runtime import HostRuntimeResolution
from app.core.execution_authority_health import ExecutionAuthorityHealth
from app.core.managed_subprocess import (
    ManagedProcessLaunchError,
    ManagedProcessResult,
    ManagedProcessTimeout,
)
from app.repositories.privacy_projection_repository import PrivacyProjectionRepository


CASE_FILE = "CASE-FILE-ACCOUNT-6222020202020202020.csv"
ACCOUNT_HEADER = "ACCOUNT-6222020202020202020"
CARD_SAMPLE = "CARD-6217000011112222333"
COUNTERPARTY_SAMPLE = "COUNTERPARTY-SENSITIVE-NAME"
ARCHIVE_PASSWORD = "ARCHIVE-PASSWORD-SENSITIVE-2026"


@pytest.fixture(autouse=True)
def _admit_archive_execution_capability(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(
        archive_extraction,
        "_archive_execution_capability_health",
        lambda: ExecutionAuthorityHealth(available=True, reason_code=""),
    )


def _mapping_request() -> field_mapping_model.LocalFieldMappingRequest:
    return field_mapping_model.LocalFieldMappingRequest(
        file_name=CASE_FILE,
        kind="bank_transaction",
        headers=[ACCOUNT_HEADER, "counterparty_name", "amount"],
        target_keys=["acct_no", "counterparty_name", "amount"],
        target_labels={
            "acct_no": "账号",
            "counterparty_name": "对手名称",
            "amount": "金额",
        },
        samples_by_header={
            ACCOUNT_HEADER: [CARD_SAMPLE],
            "counterparty_name": [COUNTERPARTY_SAMPLE],
            "amount": ["100.00"],
        },
    )


def _configured_mapping_model(tmp_path: Path) -> field_mapping_model.LocalFieldMappingModel:
    binary = tmp_path / "llama-cli"
    model = tmp_path / "mapping.gguf"
    binary.write_bytes(b"binary-placeholder")
    model.write_bytes(b"model-placeholder")
    return field_mapping_model.LocalFieldMappingModel(
        binary_path=binary,
        model_path=model,
        timeout_seconds=1,
    )


def _argv_text(command: list[object]) -> str:
    return json.dumps([str(value) for value in command], ensure_ascii=False)


def _sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _archive_runtime(path: Path) -> HostRuntimeResolution:
    digest = _sha256(path) if path.is_file() else "a" * 64
    return HostRuntimeResolution(path, "", digest)


def test_field_mapping_configured_runtime_is_blocked_before_case_prompt(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    prompt_built = False

    def must_not_build_prompt(_request) -> str:
        nonlocal prompt_built
        prompt_built = True
        raise AssertionError("case prompt must not be built before execution authority admission")

    monkeypatch.setattr(field_mapping_model, "_build_prompt", must_not_build_prompt)

    response = _configured_mapping_model(tmp_path).suggest(_mapping_request())

    assert response.available is False
    assert response.mappings == {}
    assert response.message == "local field mapping execution authority is unavailable"
    assert CARD_SAMPLE not in response.message
    assert COUNTERPARTY_SAMPLE not in response.message
    assert prompt_built is False


def test_field_mapping_capability_gate_does_not_probe_runtime_or_model_paths() -> None:
    inspected: list[str] = []

    class PoisonPath:
        def exists(self) -> bool:
            inspected.append("exists")
            raise AssertionError("configured path was probed before capability admission")

        def __fspath__(self) -> str:
            inspected.append("fspath")
            raise AssertionError("configured path was serialized before capability admission")

    response = field_mapping_model.LocalFieldMappingModel(
        binary_path=PoisonPath(),  # type: ignore[arg-type]
        model_path=PoisonPath(),  # type: ignore[arg-type]
    ).suggest(_mapping_request())

    assert response.available is False
    assert response.message == "local field mapping execution authority is unavailable"
    assert inspected == []


def test_archive_password_uses_closed_stdin_pipe_and_never_argv(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "case.7z"
    archive_path.write_bytes(b"archive-placeholder")
    output_dir = tmp_path / "out"
    extractor = tmp_path / "7zz"
    captured: dict[str, object] = {}

    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _archive_runtime(extractor))

    def fake_run(command, **options):
        captured["command"] = list(command)
        captured["options"] = options
        (Path(options["cwd"]) / "out" / "inside.csv").write_text("account,amount\n1,2\n", encoding="utf-8")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)

    archive_extraction.extract_archive(
        archive_path,
        output_dir,
        password=ARCHIVE_PASSWORD,
        expected_sha256=_sha256(archive_path),
        reuse=False,
    )

    command = captured["command"]
    options = captured["options"]
    assert isinstance(command, list)
    assert isinstance(options, dict)
    assert ARCHIVE_PASSWORD not in _argv_text(command)
    assert command.count("-p") == 1
    assert "-sccUTF-8" in command
    assert options["input_bytes"] == (ARCHIVE_PASSWORD + "\n").encode("utf-8")
    assert options["timeout_seconds"] == 300
    argv = _argv_text(command)
    assert str(archive_path) not in argv
    assert str(output_dir) not in argv


@pytest.mark.parametrize("mode", ["nonzero", "timeout", "exception"])
def test_archive_password_failure_never_reflects_secret(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    mode: str,
) -> None:
    archive_path = tmp_path / "case.7z"
    archive_path.write_bytes(b"archive-placeholder")
    extractor = tmp_path / "7zz"
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _archive_runtime(extractor))

    def fake_run(command, **options):
        assert ARCHIVE_PASSWORD not in _argv_text(command)
        assert options["input_bytes"] == (ARCHIVE_PASSWORD + "\n").encode("utf-8")
        if mode == "timeout":
            raise ManagedProcessTimeout(ARCHIVE_PASSWORD)
        if mode == "exception":
            raise ManagedProcessLaunchError(ARCHIVE_PASSWORD)
        return ManagedProcessResult(returncode=2, stdout=ARCHIVE_PASSWORD.encode())

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)

    with pytest.raises(ValueError) as raised:
        archive_extraction.extract_archive(
            archive_path,
            tmp_path / "out",
            password=ARCHIVE_PASSWORD,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )

    assert ARCHIVE_PASSWORD not in str(raised.value)
    if mode == "timeout":
        assert str(raised.value) == "archive_extract_timeout"
        assert raised.value.__cause__ is None
        assert raised.value.__context__ is None
    elif mode == "exception":
        assert str(raised.value) == "archive_runtime_launch_failed"
    else:
        assert str(raised.value) == "archive_password_or_format_invalid"


def test_archive_password_control_characters_fail_before_launch(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "case.7z"
    archive_path.write_bytes(b"archive-placeholder")

    def must_not_launch(*_args, **_kwargs):
        raise AssertionError("subprocess must not launch")

    monkeypatch.setattr(archive_extraction, "run_managed_process", must_not_launch)

    with pytest.raises(ValueError, match="^archive_password_invalid$"):
        archive_extraction.extract_archive(
            archive_path,
            tmp_path / "out",
            password="line-one\nline-two",
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )


def test_archive_password_cancellation_is_not_swallowed(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "case.7z"
    archive_path.write_bytes(b"archive-placeholder")
    extractor = tmp_path / "7zz"
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _archive_runtime(extractor))

    def cancel(_command, **_options):
        raise KeyboardInterrupt

    monkeypatch.setattr(archive_extraction, "run_managed_process", cancel)

    with pytest.raises(KeyboardInterrupt):
        archive_extraction.extract_archive(
            archive_path,
            tmp_path / "out",
            password=ARCHIVE_PASSWORD,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )


@pytest.mark.skipif(os.name == "nt", reason="uses a POSIX executable sentinel")
def test_field_mapping_configured_child_is_never_executed(tmp_path: Path) -> None:
    marker = tmp_path / "must-not-exist"
    binary = tmp_path / "llama-cli"
    binary.write_text(
        "\n".join(
            [
                "#!/bin/sh",
                f"touch {str(marker)!r}",
                "",
            ]
        ),
        encoding="utf-8",
    )
    binary.chmod(0o700)
    model = tmp_path / "mapping.gguf"
    model.write_bytes(b"model-placeholder")

    response = field_mapping_model.LocalFieldMappingModel(
        binary_path=binary,
        model_path=model,
        timeout_seconds=2,
    ).suggest(_mapping_request())

    assert response.available is False
    assert response.mappings == {}
    assert response.message == "local field mapping execution authority is unavailable"
    assert not marker.exists()


@pytest.mark.skipif(os.name == "nt", reason="uses a POSIX executable sentinel")
def test_ordinary_privacy_projection_never_executes_configured_filter(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    marker = tmp_path / "must-not-exist"
    binary = tmp_path / "privacy-filter"
    binary.write_text("#!/bin/sh\ntouch " + repr(str(marker)) + "\n", encoding="utf-8")
    binary.chmod(0o700)
    monkeypatch.setenv("ANALYTIX_PRIVACY_FILTER_BIN", str(binary))

    account = "6217000011112222333"
    projected = PrivacyProjectionRepository()._redact_text(f"银行卡号：{account}")

    assert projected == "银行卡号：[ACCOUNT]"
    assert account not in projected
    assert not marker.exists()


def test_archive_password_never_reaches_unavailable_execution_authority(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "case.7z"
    archive_path.write_bytes(b"archive-placeholder")
    output_dir = tmp_path / "out"
    extractor = tmp_path / "7zz"
    extractor.write_text(
        "\n".join(
            [
                "#!/usr/bin/env python3",
                "import pathlib, sys",
                "password_line = sys.stdin.buffer.read()",
                "assert password_line.endswith(b'\\n') and len(password_line) > 1",
                "assert '-p' in sys.argv",
                "assert all(arg == '-p' or not arg.startswith('-p') for arg in sys.argv[1:])",
                f"assert {str(archive_path)!r} not in ' '.join(sys.argv)",
                f"assert {str(output_dir)!r} not in ' '.join(sys.argv)",
                "output_arg = next(arg for arg in sys.argv if arg.startswith('-o'))",
                "output_dir = pathlib.Path(output_arg[2:])",
                "output_dir.mkdir(parents=True, exist_ok=True)",
                "(output_dir / 'inside.csv').write_text('account,amount\\n1,2\\n', encoding='utf-8')",
                "",
            ]
        ),
        encoding="utf-8",
    )
    extractor.chmod(0o700)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _archive_runtime(extractor))

    with pytest.raises(ValueError, match="^archive_runtime_launch_failed$") as raised:
        archive_extraction.extract_archive(
            archive_path,
            output_dir,
            password=ARCHIVE_PASSWORD,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )

    assert ARCHIVE_PASSWORD not in str(raised.value)
    assert not output_dir.exists()
