from __future__ import annotations

import hashlib
import json
import os
import sys
import types
from pathlib import Path

import pytest

from app.core import archive_extraction, external_runtime, managed_subprocess
from app.core.external_runtime import ExternalRuntimeError, HostRuntimeResolution, resolve_host_runtime_binary
from app.core.execution_authority_health import ExecutionAuthorityHealth
from app.core.managed_subprocess import (
    ManagedProcessLaunchError,
    ManagedProcessOutputLimit,
    ManagedProcessResult,
    run_managed_process,
)
from app.repositories import document_repository
from app.repositories.document_repository import DocumentRepository


SENSITIVE_ACCOUNT = "6222020202020202020"
SENSITIVE_PASSWORD = "PASSWORD-6222020202020202020"


def _write_executable(path: Path, source: str) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(source, encoding="utf-8")
    path.chmod(0o700)
    return path


def _write_archive_manifest(runtime_root: Path, binary: Path) -> None:
    digest = hashlib.sha256(binary.read_bytes()).hexdigest()
    (runtime_root / "analytix-archive-extractor-manifest.json").write_text(
        json.dumps(
            {
                "schemaVersion": 1,
                "tool": binary.stem,
                "packageName": "test-runtime",
                "packageVersion": "1.0.0",
                "source": "test-fixture",
                "target": f"resources/runtime/{binary.name}",
                "sha256": digest,
                "generatedAt": "2026-07-15T00:00:00.000Z",
            }
        ),
        encoding="utf-8",
    )


def _write_document_manifest(runtime_root: Path, binaries: list[Path]) -> None:
    (runtime_root / "analytix-document-runtime-manifest.json").write_text(
        json.dumps(
            {
                "schemaVersion": 1,
                "tools": [
                    {
                        "tool": binary.stem,
                        "binary": binary.name,
                        "sha256": _sha256(binary),
                    }
                    for binary in binaries
                ],
            }
        ),
        encoding="utf-8",
    )


def _sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _runtime(path: Path) -> HostRuntimeResolution:
    return HostRuntimeResolution(path=path, reason="", sha256=_sha256(path))


def _private_dir(path: Path) -> Path:
    path.mkdir()
    path.chmod(0o700)
    return path


def test_host_runtime_requires_explicit_canonical_app_resource_binary_and_integrity(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        archive_extraction,
        "_archive_execution_capability_health",
        lambda: ExecutionAuthorityHealth(available=True, reason_code=""),
    )
    resource_root = tmp_path / "resources"
    runtime_root = resource_root / "runtime"
    binary = _write_executable(runtime_root / "7zz", "#!/bin/sh\nexit 0\n")
    _write_archive_manifest(runtime_root, binary)
    monkeypatch.setattr(external_runtime, "trusted_app_resource_root", lambda: resource_root)

    monkeypatch.delenv(archive_extraction.ARCHIVE_EXTRACTOR_ENV, raising=False)
    assert archive_extraction.archive_extractor_health() == {
        "archive_extraction_available": False,
        "archive_extraction_reason": "HOST_RUNTIME_NOT_CONFIGURED",
        "archive_extraction_bin": "",
        "archive_extraction_bin_source": "",
        "archive_extraction_supported_exts": [],
    }

    monkeypatch.setenv(archive_extraction.ARCHIVE_EXTRACTOR_ENV, os.fspath(binary))
    resolution = resolve_host_runtime_binary(
        archive_extraction.ARCHIVE_EXTRACTOR_ENV,
        allowed_names={"7zz"},
        require_archive_manifest=True,
    )
    assert resolution.path == binary
    health = archive_extraction.archive_extractor_health()
    assert health["archive_extraction_available"] is True
    assert ".7z" in health["archive_extraction_supported_exts"]
    assert ".rar" not in health["archive_extraction_supported_exts"]

    binary.write_text("#!/bin/sh\nexit 9\n", encoding="utf-8")
    assert archive_extraction.archive_extractor_health()["archive_extraction_reason"] == "HOST_RUNTIME_INTEGRITY_FAILED"

    outside = _write_executable(tmp_path / "outside" / "7zz", "#!/bin/sh\nexit 0\n")
    monkeypatch.setenv(archive_extraction.ARCHIVE_EXTRACTOR_ENV, os.fspath(outside))
    assert archive_extraction.archive_extractor_health()["archive_extraction_reason"] == "HOST_RUNTIME_UNTRUSTED"

    binary.unlink()
    binary.symlink_to(outside)
    monkeypatch.setenv(archive_extraction.ARCHIVE_EXTRACTOR_ENV, os.fspath(binary))
    assert archive_extraction.archive_extractor_health()["archive_extraction_reason"] == "HOST_RUNTIME_UNTRUSTED"

    monkeypatch.setenv(archive_extraction.ARCHIVE_EXTRACTOR_ENV, "runtime/7zz")
    assert archive_extraction.archive_extractor_health()["archive_extraction_reason"] == "HOST_RUNTIME_UNTRUSTED"


def test_document_runtime_rejects_legacy_single_binary_manifest(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        document_repository,
        "_document_execution_capability_health",
        lambda: ExecutionAuthorityHealth(available=True, reason_code=""),
    )
    resource_root = tmp_path / "resources"
    runtime_root = resource_root / "runtime"
    document_root = runtime_root / "document-runtime"
    binary = _write_executable(document_root / "antiword", "#!/bin/sh\nexit 0\n")
    _write_document_manifest(document_root, [binary])
    monkeypatch.setattr(external_runtime, "trusted_app_resource_root", lambda: resource_root)
    monkeypatch.setenv(document_repository._DOCUMENT_ANTIWORD_ENV, os.fspath(binary))

    resolution = document_repository._resolved_document_runtime(
        document_repository._DOCUMENT_ANTIWORD_ENV,
        {"antiword"},
    )
    assert resolution == HostRuntimeResolution(None, "HOST_RUNTIME_INTEGRITY_FAILED")


class _TestExecutionAuthority:
    def __init__(
        self,
        execute,
        *,
        guarantees: frozenset[str] | None = None,
    ) -> None:
        self._execute = execute
        self._guarantees = (
            managed_subprocess._required_authority_guarantees(sys.platform)
            if guarantees is None
            else guarantees
        )

    def status(self) -> managed_subprocess.ManagedExecutionAuthorityStatus:
        return managed_subprocess.ManagedExecutionAuthorityStatus(
            available=True,
            protocol_version=managed_subprocess._AUTHORITY_PROTOCOL_VERSION,
            platform=sys.platform,
            reason_code="",
            guarantees=self._guarantees,
        )

    def execute(
        self,
        request: managed_subprocess.ManagedExecutionRequest,
    ) -> ManagedProcessResult:
        result = self._execute(request)
        if (
            isinstance(result, ManagedProcessResult)
            and result.execution_receipt is None
        ):
            return ManagedProcessResult(
                returncode=result.returncode,
                stdout=result.stdout,
                execution_receipt=managed_subprocess.ManagedExecutionReceipt(
                    protocol_version=request.protocol_version,
                    launch_nonce=request.launch_nonce,
                    cwd_device=request.cwd_device,
                    cwd_inode=request.cwd_inode,
                    executable_sha256=request.expected_executable_sha256,
                    environment_sha256=request.environment_sha256,
                ),
            )
        return result


def test_production_managed_execution_authority_is_explicitly_unavailable() -> None:
    status = managed_subprocess.managed_execution_authority_status()

    assert status == managed_subprocess.ManagedExecutionAuthorityStatus(
        available=False,
        protocol_version=managed_subprocess._AUTHORITY_PROTOCOL_VERSION,
        platform=sys.platform,
        reason_code="managed_process_execution_authority_unavailable",
        guarantees=frozenset(),
    )


def test_managed_execution_is_unavailable_before_case_data_reaches_adapter(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    adapter_marker = tmp_path / f"adapter-received-{SENSITIVE_ACCOUNT}"
    input_marker = tmp_path / f"input-inspected-{SENSITIVE_ACCOUNT}"

    class PoisonCommand:
        def __iter__(self):
            input_marker.write_text("unsafe", encoding="utf-8")
            raise AssertionError("argv was inspected before authority admission")

    class PoisonCwd:
        def __fspath__(self) -> str:
            input_marker.write_text("unsafe", encoding="utf-8")
            raise AssertionError("cwd was inspected before authority admission")

    class UnavailableAuthority:
        def status(self) -> managed_subprocess.ManagedExecutionAuthorityStatus:
            return managed_subprocess.ManagedExecutionAuthorityStatus(
                available=False,
                protocol_version=1,
                platform=sys.platform,
                reason_code="managed_process_execution_authority_unavailable",
                guarantees=frozenset(),
            )

        def execute(self, _request) -> ManagedProcessResult:
            adapter_marker.write_text("unsafe", encoding="utf-8")
            raise AssertionError("unavailable adapter received case data")

    monkeypatch.setattr(
        managed_subprocess,
        "_production_execution_authority",
        lambda: UnavailableAuthority(),
    )
    with pytest.raises(
        ManagedProcessLaunchError,
        match="^managed_process_execution_authority_unavailable$",
    ) as raised:
        run_managed_process(
            PoisonCommand(),  # type: ignore[arg-type]
            cwd=PoisonCwd(),  # type: ignore[arg-type]
            input_bytes=SENSITIVE_ACCOUNT.encode("utf-8"),
            timeout_seconds=1,
            expected_executable_sha256="a" * 64,
        )

    assert not adapter_marker.exists()
    assert not input_marker.exists()
    assert SENSITIVE_ACCOUNT not in str(raised.value)


def test_managed_execution_rejects_incomplete_authority_claim_before_execute(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    executed = False

    def unsafe_execute(_request) -> ManagedProcessResult:
        nonlocal executed
        executed = True
        return ManagedProcessResult(returncode=0, stdout=b"")

    authority = _TestExecutionAuthority(unsafe_execute, guarantees=frozenset({"path_hash"}))
    monkeypatch.setattr(managed_subprocess, "_production_execution_authority", lambda: authority)

    with pytest.raises(
        ManagedProcessLaunchError,
        match="^managed_process_execution_authority_unavailable$",
    ):
        run_managed_process(
            ["/runtime/helper"],
            cwd=tmp_path,
            input_bytes=None,
            timeout_seconds=1,
            expected_executable_sha256="a" * 64,
        )
    assert executed is False


def test_managed_facade_validates_request_and_nonce_at_test_authority_port(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    received: list[managed_subprocess.ManagedExecutionRequest] = []
    monkeypatch.setenv("OPENAI_API_KEY", SENSITIVE_PASSWORD)
    monkeypatch.setenv("HTTPS_PROXY", f"http://{SENSITIVE_PASSWORD}@proxy.invalid")
    monkeypatch.setenv("SSH_AUTH_SOCK", f"/tmp/{SENSITIVE_ACCOUNT}.sock")

    def execute(request: managed_subprocess.ManagedExecutionRequest) -> ManagedProcessResult:
        received.append(request)
        return ManagedProcessResult(returncode=0, stdout=b"bounded")

    authority = _TestExecutionAuthority(execute)
    monkeypatch.setattr(managed_subprocess, "_production_execution_authority", lambda: authority)
    result = run_managed_process(
        ["/runtime/helper", "opaque-input"],
        cwd=tmp_path,
        input_bytes=b"opaque-bytes",
        timeout_seconds=2,
        expected_executable_sha256="b" * 64,
        capture_stdout=True,
    )

    assert result == ManagedProcessResult(returncode=0, stdout=b"bounded")
    assert len(received) == 1
    request = received[0]
    assert request.protocol_version == managed_subprocess._AUTHORITY_PROTOCOL_VERSION
    assert len(request.launch_nonce) == 64
    assert int(request.launch_nonce, 16) >= 0
    assert request.argv == ("/runtime/helper", "opaque-input")
    assert request.input_bytes == b"opaque-bytes"
    assert dict(request.environment) == managed_subprocess.build_sanitized_subprocess_env(
        {},
        executable="/runtime/helper",
        platform=sys.platform,
    )
    assert request.environment_sha256 == managed_subprocess._execution_environment_sha256(
        request.environment
    )
    assert not (
        {"OPENAI_API_KEY", "HTTPS_PROXY", "SSH_AUTH_SOCK"}
        & dict(request.environment).keys()
    )


def test_managed_facade_hands_authority_a_pinned_cwd_descriptor(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    original = _private_dir(tmp_path / "original")
    decoy = _private_dir(tmp_path / "decoy")
    displaced = tmp_path / "displaced"
    caller_fd = os.open(original, os.O_RDONLY | os.O_DIRECTORY)
    expected = os.fstat(caller_fd)
    observed_request_fd = -1

    def execute(request: managed_subprocess.ManagedExecutionRequest) -> ManagedProcessResult:
        nonlocal observed_request_fd
        observed_request_fd = request.cwd_fd
        original.rename(displaced)
        decoy.rename(original)
        opened = os.fstat(request.cwd_fd)
        assert (int(opened.st_dev), int(opened.st_ino)) == (
            int(expected.st_dev),
            int(expected.st_ino),
        )
        marker_fd = os.open(
            "pinned.txt",
            os.O_WRONLY | os.O_CREAT | os.O_EXCL,
            0o600,
            dir_fd=request.cwd_fd,
        )
        os.close(marker_fd)
        return ManagedProcessResult(returncode=0, stdout=b"")

    authority = _TestExecutionAuthority(execute)
    monkeypatch.setattr(managed_subprocess, "_production_execution_authority", lambda: authority)
    try:
        result = run_managed_process(
            ["/runtime/helper"],
            cwd=original,
            cwd_fd=caller_fd,
            expected_cwd_device=int(expected.st_dev),
            expected_cwd_inode=int(expected.st_ino),
            input_bytes=None,
            timeout_seconds=1,
            expected_executable_sha256="e" * 64,
        )
    finally:
        os.close(caller_fd)

    assert result == ManagedProcessResult(returncode=0, stdout=b"")
    assert (displaced / "pinned.txt").is_file()
    assert not (original / "pinned.txt").exists()
    with pytest.raises(OSError):
        os.fstat(observed_request_fd)


def test_managed_facade_rejects_mismatched_cwd_descriptor_before_execute(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    expected_dir = _private_dir(tmp_path / "expected")
    wrong_dir = _private_dir(tmp_path / "wrong")
    wrong_fd = os.open(wrong_dir, os.O_RDONLY | os.O_DIRECTORY)
    executed = False

    def execute(_request: managed_subprocess.ManagedExecutionRequest) -> ManagedProcessResult:
        nonlocal executed
        executed = True
        return ManagedProcessResult(returncode=0, stdout=b"")

    authority = _TestExecutionAuthority(execute)
    monkeypatch.setattr(managed_subprocess, "_production_execution_authority", lambda: authority)
    try:
        with pytest.raises(
            ManagedProcessLaunchError,
            match="^managed_process_workdir_identity_invalid$",
        ):
            run_managed_process(
                ["/runtime/helper"],
                cwd=expected_dir,
                cwd_fd=wrong_fd,
                input_bytes=None,
                timeout_seconds=1,
                expected_executable_sha256="f" * 64,
            )
    finally:
        os.close(wrong_fd)
    assert executed is False


def test_managed_facade_rejects_oversized_authority_output(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    authority = _TestExecutionAuthority(
        lambda _request: ManagedProcessResult(returncode=0, stdout=b"x" * 129)
    )
    monkeypatch.setattr(managed_subprocess, "_production_execution_authority", lambda: authority)
    monkeypatch.setattr(managed_subprocess, "_MAX_CAPTURED_STDOUT_BYTES", 128)

    with pytest.raises(ManagedProcessOutputLimit, match="^managed_process_stdout_limit$"):
        run_managed_process(
            ["/runtime/helper"],
            cwd=tmp_path,
            input_bytes=None,
            timeout_seconds=1,
            expected_executable_sha256="c" * 64,
            capture_stdout=True,
        )


def test_managed_facade_rejects_mismatched_execution_receipt(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def execute(
        request: managed_subprocess.ManagedExecutionRequest,
    ) -> ManagedProcessResult:
        return ManagedProcessResult(
            returncode=0,
            stdout=b"",
            execution_receipt=managed_subprocess.ManagedExecutionReceipt(
                protocol_version=request.protocol_version,
                launch_nonce=request.launch_nonce,
                cwd_device=request.cwd_device,
                cwd_inode=request.cwd_inode + 1,
                executable_sha256=request.expected_executable_sha256,
                environment_sha256=request.environment_sha256,
            ),
        )

    authority = _TestExecutionAuthority(execute)
    monkeypatch.setattr(managed_subprocess, "_production_execution_authority", lambda: authority)
    with pytest.raises(
        ManagedProcessLaunchError,
        match="^managed_process_execution_receipt_invalid$",
    ):
        run_managed_process(
            ["/runtime/helper"],
            cwd=tmp_path,
            input_bytes=None,
            timeout_seconds=1,
            expected_executable_sha256="9" * 64,
        )


def test_managed_facade_rejects_unbound_execution_environment_receipt(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def execute(
        request: managed_subprocess.ManagedExecutionRequest,
    ) -> ManagedProcessResult:
        return ManagedProcessResult(
            returncode=0,
            stdout=b"",
            execution_receipt=managed_subprocess.ManagedExecutionReceipt(
                protocol_version=request.protocol_version,
                launch_nonce=request.launch_nonce,
                cwd_device=request.cwd_device,
                cwd_inode=request.cwd_inode,
                executable_sha256=request.expected_executable_sha256,
                environment_sha256="0" * 64,
            ),
        )

    monkeypatch.setenv("OPENAI_API_KEY", SENSITIVE_PASSWORD)
    authority = _TestExecutionAuthority(execute)
    monkeypatch.setattr(managed_subprocess, "_production_execution_authority", lambda: authority)
    with pytest.raises(
        ManagedProcessLaunchError,
        match="^managed_process_execution_receipt_invalid$",
    ):
        run_managed_process(
            ["/runtime/helper"],
            cwd=tmp_path,
            input_bytes=None,
            timeout_seconds=1,
            expected_executable_sha256="8" * 64,
        )


def test_managed_facade_does_not_swallow_cancellation_from_authority(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def cancel(_request: managed_subprocess.ManagedExecutionRequest) -> ManagedProcessResult:
        raise KeyboardInterrupt

    authority = _TestExecutionAuthority(cancel)
    monkeypatch.setattr(managed_subprocess, "_production_execution_authority", lambda: authority)

    with pytest.raises(KeyboardInterrupt):
        run_managed_process(
            ["/runtime/helper"],
            cwd=tmp_path,
            input_bytes=None,
            timeout_seconds=1,
            expected_executable_sha256="d" * 64,
        )


def test_darwin_swap_back_attacker_marker_never_executes_without_authority(
    tmp_path: Path,
) -> None:
    marker = tmp_path / f"swap-back-marker-{SENSITIVE_ACCOUNT}"
    helper = _write_executable(tmp_path / "helper", "#!/bin/sh\nexit 0\n")
    expected_sha256 = _sha256(helper)
    helper.write_text(f"#!/bin/sh\necho unsafe > {marker!s}\n", encoding="utf-8")
    helper.chmod(0o700)

    with pytest.raises(
        ManagedProcessLaunchError,
        match="^managed_process_execution_authority_unavailable$",
    ):
        run_managed_process(
            [os.fspath(helper)],
            cwd=_private_dir(tmp_path / "runtime-work"),
            input_bytes=SENSITIVE_ACCOUNT.encode("utf-8"),
            timeout_seconds=1,
            expected_executable_sha256=expected_sha256,
        )
    assert not marker.exists()


def test_setsid_descendant_marker_never_executes_without_authority(tmp_path: Path) -> None:
    marker = tmp_path / f"setsid-marker-{SENSITIVE_ACCOUNT}"
    helper = _write_executable(
        tmp_path / "setsid-helper",
        f"#!/bin/sh\nsetsid /bin/sh -c 'echo unsafe > {marker!s}' &\n",
    )

    with pytest.raises(
        ManagedProcessLaunchError,
        match="^managed_process_execution_authority_unavailable$",
    ):
        run_managed_process(
            [os.fspath(helper)],
            cwd=_private_dir(tmp_path / "runtime-work"),
            input_bytes=None,
            timeout_seconds=1,
            expected_executable_sha256=_sha256(helper),
        )
    assert not marker.exists()


def test_archive_unavailable_execution_authority_is_fixed_boundary_and_writes_nothing(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / f"案件-{SENSITIVE_ACCOUNT}.7z"
    archive_path.write_bytes(b"opaque archive bytes")
    output_dir = tmp_path / f"案件输出-{SENSITIVE_ACCOUNT}"
    argv_audit = tmp_path / "argv.json"
    helper = _write_executable(
        tmp_path / "7zz",
        "\n".join(
            [
                f"#!{sys.executable}",
                "import json, pathlib, subprocess, sys, time",
                f"pathlib.Path({str(argv_audit)!r}).write_text(json.dumps(sys.argv), encoding='utf-8')",
                "_ = sys.stdin.buffer.read()",
                "subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(60)'])",
                "time.sleep(60)",
                "",
            ]
        ),
    )
    monkeypatch.setattr(
        archive_extraction,
        "archive_extractor_runtime",
        lambda: pytest.fail("runtime manifest must not be read before capability admission"),
    )
    with pytest.raises(
        ExternalRuntimeError,
        match="^managed_process_execution_authority_unavailable$",
    ) as raised:
        archive_extraction.extract_archive(
            archive_path,
            output_dir,
            password=SENSITIVE_PASSWORD,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )

    assert not argv_audit.exists()
    assert str(raised.value) == "managed_process_execution_authority_unavailable"
    assert SENSITIVE_ACCOUNT not in str(raised.value)
    assert SENSITIVE_PASSWORD not in str(raised.value)
    assert not output_dir.exists()


def test_document_unavailable_execution_authority_is_fixed_boundary_and_does_not_launch(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    source = tmp_path / f"银行账号-{SENSITIVE_ACCOUNT}.doc"
    source.write_bytes(b"legacy doc bytes")
    argv_audit = tmp_path / "doc-argv.json"
    helper = _write_executable(
        tmp_path / "antiword",
        "\n".join(
            [
                f"#!{sys.executable}",
                "import json, pathlib, sys",
                f"pathlib.Path({str(argv_audit)!r}).write_text(json.dumps(sys.argv), encoding='utf-8')",
                "sys.stderr.write('BANK-' + '6222020202020202020')",
                "raise SystemExit(17)",
                "",
            ]
        ),
    )
    monkeypatch.setattr(
        document_repository,
        "_resolved_document_runtime",
        lambda *_args, **_kwargs: pytest.fail(
            "document runtime manifest must not be read before capability admission"
        ),
    )

    with pytest.raises(
        ExternalRuntimeError,
        match="^managed_process_execution_authority_unavailable$",
    ) as raised:
        DocumentRepository()._extract_doc(case_id="case-a", path=source)

    assert not argv_audit.exists()
    assert str(raised.value) == "managed_process_execution_authority_unavailable"
    assert raised.value.__cause__ is None
    assert document_repository._safe_document_error_code(raised.value) == (
        "managed_process_execution_authority_unavailable"
    )
    assert document_repository._safe_document_error_code(RuntimeError(SENSITIVE_ACCOUNT)) == "document_extract_failed"


@pytest.mark.parametrize("converter", ["soffice", "textutil"])
def test_document_converter_argv_uses_only_opaque_relative_paths(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    converter: str,
) -> None:
    monkeypatch.setattr(
        document_repository,
        "_document_execution_capability_health",
        lambda: ExecutionAuthorityHealth(available=True, reason_code=""),
    )
    source = tmp_path / f"案件银行账号-{SENSITIVE_ACCOUNT}.doc"
    source.write_bytes(b"legacy document")
    repository = DocumentRepository()
    case_root = tmp_path / f"case-{SENSITIVE_ACCOUNT}"
    case_root.mkdir(mode=0o700)
    monkeypatch.setattr(repository._storage, "case_dir", lambda _case_id: case_root)
    captured: dict[str, object] = {}

    def fake_run(command, **options):
        captured["command"] = list(command)
        captured["cwd"] = options["cwd"]
        cwd = Path(options["cwd"])
        if converter == "soffice":
            (cwd / "out" / "input.docx").write_bytes(b"converted-docx")
        else:
            (cwd / "output.txt").write_text("converted text", encoding="utf-8")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(document_repository, "run_managed_process", fake_run)
    if converter == "soffice":
        converted = repository._convert_doc_with_soffice(
            case_id="case-a",
            office_binary="/trusted/runtime/soffice",
            expected_executable_sha256="a" * 64,
            path=source,
        )
    else:
        converted = repository._convert_doc_with_textutil(
            case_id="case-a",
            textutil_binary="/trusted/runtime/textutil",
            expected_executable_sha256="b" * 64,
            path=source,
        )

    argv = json.dumps(captured["command"], ensure_ascii=False)
    assert SENSITIVE_ACCOUNT not in argv
    assert os.fspath(source) not in argv
    assert os.fspath(converted) not in argv
    assert "input.doc" in argv
    assert converted.is_file()
    assert len(converted.parent.name) == 64


@pytest.mark.skipif(os.name == "nt", reason="POSIX dir-fd publication assertion")
@pytest.mark.parametrize("attack", ["directory", "file"])
def test_document_publish_rejects_target_symlink_without_touching_victim(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    attack: str,
) -> None:
    repository = DocumentRepository()
    case_root = tmp_path / "case-a"
    monkeypatch.setattr(repository._storage, "case_dir", lambda _case_id: case_root)
    staged = tmp_path / "staged.txt"
    staged.write_text("converted evidence", encoding="utf-8")
    source_sha256 = "d" * 64
    convert_root = case_root / "knowledge" / "_docconvert"
    convert_root.mkdir(parents=True)
    target_dir = convert_root / source_sha256
    victim = tmp_path / "victim"
    victim.mkdir()
    victim_file = victim / "do-not-touch.txt"
    victim_file.write_text("original", encoding="utf-8")
    if attack == "directory":
        target_dir.symlink_to(victim, target_is_directory=True)
    else:
        target_dir.mkdir()
        (target_dir / "document.txt").symlink_to(victim_file)

    with pytest.raises(
        ExternalRuntimeError,
        match="^(document_publish_root_untrusted|document_publish_failed)$",
    ):
        repository._publish_converted_document(
            case_id="case-a",
            source_sha256=source_sha256,
            staged_path=staged,
            suffix=".txt",
        )

    assert victim_file.read_text(encoding="utf-8") == "original"


def test_ocr_exception_is_fixed_code_without_sensitive_child_context(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        document_repository,
        "_document_execution_capability_health",
        lambda: ExecutionAuthorityHealth(available=True, reason_code=""),
    )
    class FakeImage:
        def save(self, output, *, format: str) -> None:
            assert format == "PNG"
            output.write(SENSITIVE_ACCOUNT.encode())

    class FakePage:
        def render(self, *, scale: float):
            assert scale == 2.0
            return types.SimpleNamespace(to_pil=lambda: FakeImage())

        def close(self) -> None:
            pass

    class FakeDocument:
        def get_page(self, index: int) -> FakePage:
            assert index == 0
            return FakePage()

        def close(self) -> None:
            pass

    monkeypatch.setitem(sys.modules, "pypdfium2", types.SimpleNamespace(PdfDocument=lambda _path: FakeDocument()))
    monkeypatch.setattr(
        document_repository,
        "_resolved_document_runtime",
        lambda *_args, **_kwargs: HostRuntimeResolution(Path("/runtime/tesseract"), "", "c" * 64),
    )

    def fail_with_sensitive_context(*_args, **_kwargs):
        raise RuntimeError(SENSITIVE_ACCOUNT)

    monkeypatch.setattr(document_repository, "run_managed_process", fail_with_sensitive_context)
    segments, status, error = DocumentRepository()._extract_pdf_with_ocr(
        tmp_path / f"scan-{SENSITIVE_ACCOUNT}.pdf",
        [1],
    )

    assert segments == []
    assert status == "failed"
    assert error == "ocr_failed"
    assert SENSITIVE_ACCOUNT not in error
