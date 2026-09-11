from __future__ import annotations

import ast
import os
import subprocess
import sys
import types
from pathlib import Path

import pytest

from app.core import analysis_compute_runner
from app.core.external_runtime import HostRuntimeResolution
from app.core.execution_authority_health import ExecutionAuthorityHealth
from app.core.managed_subprocess import ManagedProcessResult
from app.core.process_environment import build_sanitized_subprocess_env
from app.repositories import document_repository
from app.repositories.document_repository import DocumentRepository


_HOST_SENTINELS = {
    "OPENAI_API_KEY": "openai-secret",
    "SOME_REFRESH_TOKEN": "refresh-secret",
    "SOME_PASSWORD_FILE": "/tmp/password-file",
    "AWS_SESSION_TOKEN": "aws-secret",
    "GITHUB_TOKEN": "github-secret",
    "NPM_CONFIG_USERCONFIG": "/tmp/host-npmrc",
    "HTTPS_PROXY": "http://credential@proxy.invalid",
    "NO_PROXY": "localhost",
    "SSH_AUTH_SOCK": "/tmp/agent.sock",
    "DBUS_SESSION_BUS_ADDRESS": "unix:path=/tmp/dbus",
    "XDG_RUNTIME_DIR": "/tmp/host-runtime",
    "UNKNOWN_HOST_CAPABILITY": "ambient-authority",
}


def _install_host_sentinels(monkeypatch: pytest.MonkeyPatch) -> None:
    for name, value in _HOST_SENTINELS.items():
        monkeypatch.setenv(name, value)


def _assert_host_sentinels_absent(environment: dict[str, str]) -> None:
    for name in _HOST_SENTINELS:
        assert name not in environment


def test_subprocess_environment_is_platform_allowlist_with_explicit_safe_increment(tmp_path: Path) -> None:
    source = {
        "PATH": "/attacker-controlled/bin",
        "TMPDIR": "/tmp/analytix",
        "LANG": "C.UTF-8",
        "SystemRoot": r"D:\AttackerWindows",
        "__CF_USER_TEXT_ENCODING": "0x1F5:0x19:0x34",
        "TESSDATA_PREFIX": "/opt/tessdata",
        "OMP_THREAD_LIMIT": "2",
        **_HOST_SENTINELS,
    }

    linux = build_sanitized_subprocess_env(source, platform="linux")
    assert linux == {
        "PATH": "/usr/bin:/bin",
        "LANG": "C.UTF-8",
    }

    safe_temp = tmp_path / "safe-temp"
    safe_temp.mkdir()
    with_safe_temp = build_sanitized_subprocess_env(
        source,
        executable="/opt/analytix/helper",
        safe_temp_directory=safe_temp,
        platform="linux",
    )
    assert with_safe_temp["TEMP"] == str(safe_temp)
    assert with_safe_temp["TMP"] == str(safe_temp)
    assert with_safe_temp["TMPDIR"] == str(safe_temp)

    windows = build_sanitized_subprocess_env(source, platform="win32")
    assert windows["SYSTEMROOT"] == r"C:\Windows"
    assert windows["WINDIR"] == r"C:\Windows"
    assert windows["SYSTEMDRIVE"] == "C:"
    assert windows["PATH"] == r"C:\Windows\System32;C:\Windows"
    assert "__CF_USER_TEXT_ENCODING" not in windows

    darwin = build_sanitized_subprocess_env(source, platform="darwin")
    assert darwin["__CF_USER_TEXT_ENCODING"] == "0x1F5:0x19:0x34"
    assert darwin["PATH"] == "/usr/bin:/bin:/usr/sbin:/sbin"
    assert "SystemRoot" not in darwin

    tesseract = build_sanitized_subprocess_env(
        source,
        executable="/opt/analytix/tesseract",
        additional_allowed_names=("TESSDATA_PREFIX", "OMP_THREAD_LIMIT"),
        platform="linux",
    )
    assert tesseract["TESSDATA_PREFIX"] == "/opt/tessdata"
    assert tesseract["OMP_THREAD_LIMIT"] == "2"
    _assert_host_sentinels_absent(tesseract)

    with pytest.raises(ValueError, match="executable is required"):
        build_sanitized_subprocess_env(
            source,
            additional_allowed_names=("TESSDATA_PREFIX",),
        )
    for forbidden in (
        "OPENAI_API_KEY",
        "SOME_REFRESH_TOKEN",
        "SOME_SECRET",
        "SOME_PASSWORD",
        "SOME_PASSWORD_FILE",
        "AWS_SESSION_TOKEN",
        "GITHUB_TOKEN",
        "NPM_TOKEN",
        "HTTPS_PROXY",
        "SSH_AUTH_SOCK",
    ):
        with pytest.raises(ValueError, match="forbidden subprocess environment name"):
            build_sanitized_subprocess_env(
                source,
                executable="/opt/analytix/helper",
                additional_allowed_names=(forbidden,),
            )


def test_analysis_compute_has_no_direct_process_even_with_ambient_host_authority(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
) -> None:
    _install_host_sentinels(monkeypatch)
    binary = tmp_path / "analytix-analysis-compute"
    marker = tmp_path / "must-not-exist"
    binary.write_text(f"marker={marker}", encoding="utf-8")
    monkeypatch.setenv("ANALYTIX_ANALYSIS_COMPUTE_BIN", str(binary))

    with pytest.raises(
        analysis_compute_runner.AnalysisComputeUnavailableError,
        match="^managed_process_capability_admission_unavailable$",
    ):
        analysis_compute_runner.run_analysis_compute(["project-flow-layout-network-plan"])

    assert not marker.exists()


def test_document_ocr_direct_process_drops_ambient_authority_and_never_uses_pytesseract(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
) -> None:
    monkeypatch.setattr(
        document_repository,
        "_document_execution_capability_health",
        lambda: ExecutionAuthorityHealth(available=True, reason_code=""),
    )
    _install_host_sentinels(monkeypatch)
    monkeypatch.setenv("TESSDATA_PREFIX", "/opt/tessdata")
    monkeypatch.setenv("OMP_THREAD_LIMIT", "3")
    captured: dict[str, object] = {}

    class FakeImage:
        def save(self, output, *, format: str) -> None:
            assert format == "PNG"
            output.write(b"safe-png-bytes")

    class FakeBitmap:
        def to_pil(self) -> FakeImage:
            return FakeImage()

    class FakePage:
        closed = False

        def render(self, *, scale: float) -> FakeBitmap:
            assert scale == 2.0
            return FakeBitmap()

        def close(self) -> None:
            self.closed = True

    class FakeDocument:
        closed = False

        def get_page(self, index: int) -> FakePage:
            assert index == 0
            return FakePage()

        def close(self) -> None:
            self.closed = True

    fake_document = FakeDocument()

    def fake_run(command: list[str], **kwargs: object) -> ManagedProcessResult:
        captured["command"] = command
        captured["input"] = kwargs.get("input_bytes")
        captured["cwd"] = kwargs.get("cwd")
        captured["expected_executable_sha256"] = kwargs.get("expected_executable_sha256")
        return ManagedProcessResult(returncode=0, stdout=b"recognized text")

    monkeypatch.setitem(
        sys.modules,
        "pypdfium2",
        types.SimpleNamespace(PdfDocument=lambda _path: fake_document),
    )
    monkeypatch.setattr(
        document_repository,
        "_resolved_document_runtime",
        lambda env_name, allowed_names: HostRuntimeResolution(
            Path("/opt/tesseract"),
            "",
            "a" * 64,
        ),
    )
    monkeypatch.setattr(document_repository, "run_managed_process", fake_run)

    segments, status, error = DocumentRepository()._extract_pdf_with_ocr(tmp_path / "input.pdf", [1])

    assert status == "completed"
    assert error == ""
    assert [segment.text for segment in segments] == ["recognized text"]
    assert captured["command"] == ["/opt/tesseract", "stdin", "stdout", "-l", "chi_sim+eng"]
    assert captured["input"] == b"safe-png-bytes"
    assert captured["expected_executable_sha256"] == "a" * 64
    assert isinstance(captured["cwd"], Path)
    assert fake_document.closed is True


_SUBPROCESS_METHODS = {
    "Popen",
    "call",
    "check_call",
    "check_output",
    "getoutput",
    "getstatusoutput",
    "run",
}
_ASYNCIO_METHODS = {"create_subprocess_exec", "create_subprocess_shell"}
_OS_PROCESS_METHODS = {
    "execv",
    "execve",
    "execvp",
    "execvpe",
    "popen",
    "posix_spawn",
    "posix_spawnp",
    "spawnl",
    "spawnle",
    "spawnlp",
    "spawnlpe",
    "spawnv",
    "spawnve",
    "spawnvp",
    "spawnvpe",
    "system",
}


def _is_environment_builder(node: ast.AST | None) -> bool:
    if not isinstance(node, ast.Call):
        return False
    if isinstance(node.func, ast.Name):
        return node.func.id == "build_sanitized_subprocess_env"
    return isinstance(node.func, ast.Attribute) and node.func.attr == "build_sanitized_subprocess_env"


def _process_environment_violations(source: str, *, filename: str) -> list[str]:
    tree = ast.parse(source, filename=filename)
    module_aliases: dict[str, str] = {}
    call_aliases: dict[str, tuple[str, str]] = {}
    violations: list[str] = []

    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                if alias.name in {"asyncio", "os", "pytesseract", "subprocess"}:
                    module_aliases[alias.asname or alias.name] = alias.name
        elif isinstance(node, ast.ImportFrom) and node.module in {"asyncio", "os", "pytesseract", "subprocess"}:
            for alias in node.names:
                call_aliases[alias.asname or alias.name] = (node.module, alias.name)

    for node in ast.walk(tree):
        if not isinstance(node, ast.Call):
            continue
        module = ""
        method = ""
        if isinstance(node.func, ast.Attribute) and isinstance(node.func.value, ast.Name):
            module = module_aliases.get(node.func.value.id, "")
            method = node.func.attr
        elif isinstance(node.func, ast.Name) and node.func.id in call_aliases:
            module, method = call_aliases[node.func.id]

        if module == "pytesseract":
            violations.append(f"{filename}:{node.lineno}:indirect-pytesseract-process")
            continue
        process_call = (
            (module == "subprocess" and method in _SUBPROCESS_METHODS)
            or (module == "asyncio" and method in _ASYNCIO_METHODS)
        )
        if process_call:
            if filename != "core/managed_subprocess.py":
                violations.append(f"{filename}:{node.lineno}:managed-process-authority-bypass")
            env_value = next((keyword.value for keyword in node.keywords if keyword.arg == "env"), None)
            shell_value = next((keyword.value for keyword in node.keywords if keyword.arg == "shell"), None)
            if not _is_environment_builder(env_value):
                violations.append(f"{filename}:{node.lineno}:missing-closed-environment")
            if isinstance(shell_value, ast.Constant) and shell_value.value is True:
                violations.append(f"{filename}:{node.lineno}:shell-execution-forbidden")
        elif module == "os" and method in _OS_PROCESS_METHODS:
            if filename != "core/managed_subprocess.py":
                violations.append(f"{filename}:{node.lineno}:managed-process-authority-bypass")

    return violations


def test_process_call_auditor_covers_alias_asyncio_and_indirect_ocr() -> None:
    source = """
import asyncio as aio
import pytesseract as ocr
from subprocess import Popen as launch

launch([\"helper\"])
aio.create_subprocess_exec(\"helper\")
ocr.image_to_string(object())
"""

    assert _process_environment_violations(source, filename="hostile.py") == [
        "hostile.py:6:managed-process-authority-bypass",
        "hostile.py:6:missing-closed-environment",
        "hostile.py:7:managed-process-authority-bypass",
        "hostile.py:7:missing-closed-environment",
        "hostile.py:8:indirect-pytesseract-process",
    ]


def test_every_backend_process_entry_uses_the_single_managed_authority() -> None:
    app_root = Path(__file__).resolve().parents[1] / "app"
    violations: list[str] = []
    for source_path in sorted(app_root.rglob("*.py")):
        source = source_path.read_text(encoding="utf-8")
        relative = str(source_path.relative_to(app_root))
        violations.extend(_process_environment_violations(source, filename=relative))
        if "pytesseract" in source:
            violations.append(f"{relative}:indirect-pytesseract-import")

    assert violations == []
