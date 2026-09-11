from __future__ import annotations

import os
import sys
import tempfile
from pathlib import Path

import pytest

from app.core import import_accelerator, managed_subprocess


_BLOCKER = "managed_process_capability_admission_unavailable"
_SENSITIVE_ACCOUNT = "6222020202020202020"


class _PoisonCaseArgument:
    def __init__(self, marker: Path) -> None:
        self._marker = marker

    def _inspected(self) -> None:
        self._marker.write_text("case argument inspected", encoding="utf-8")
        raise AssertionError("case argument was inspected before capability admission")

    def __bool__(self) -> bool:
        self._inspected()

    def __fspath__(self) -> str:
        self._inspected()

    def __int__(self) -> int:
        self._inspected()

    def __iter__(self):
        self._inspected()

    def __str__(self) -> str:
        self._inspected()

    def exists(self) -> bool:
        self._inspected()

    def stat(self):
        self._inspected()


@pytest.mark.parametrize(
    "operation",
    (
        "detect_encoding",
        "prepare_csv",
        "prepare_csv_with_profile",
        "prepare_csv_batch",
        "prepare_csv_with_profile_batch",
        "profile_csv_columns",
        "excel_to_csv",
        "split_excel_account_sections",
    ),
)
def test_import_accelerator_rejects_before_case_arguments_are_inspected(
    tmp_path: Path,
    operation: str,
) -> None:
    marker = tmp_path / "argument-inspected"
    poison = _PoisonCaseArgument(marker)
    calls = {
        "detect_encoding": lambda: import_accelerator.detect_encoding(poison),
        "prepare_csv": lambda: import_accelerator.prepare_csv(
            poison,
            limit=poison,
            out_path=poison,
            encoding=poison,
        ),
        "prepare_csv_with_profile": lambda: import_accelerator.prepare_csv_with_profile(
            poison,
            limit=poison,
            encoding=poison,
        ),
        "prepare_csv_batch": lambda: import_accelerator.prepare_csv_batch(poison),
        "prepare_csv_with_profile_batch": lambda: import_accelerator.prepare_csv_with_profile_batch(poison),
        "profile_csv_columns": lambda: import_accelerator.profile_csv_columns(
            poison,
            encoding=poison,
        ),
        "excel_to_csv": lambda: import_accelerator.excel_to_csv(poison, out_path=poison),
        "split_excel_account_sections": lambda: import_accelerator.split_excel_account_sections(
            poison,
            output_dir=poison,
        ),
    }

    with pytest.raises(import_accelerator.ImportAcceleratorUnavailableError) as raised:
        calls[operation]()

    assert str(raised.value) == _BLOCKER
    assert not marker.exists()


def test_import_accelerator_optional_fallbacks_do_not_inspect_case_arguments(tmp_path: Path) -> None:
    marker = tmp_path / "fallback-argument-inspected"
    poison = _PoisonCaseArgument(marker)

    assert import_accelerator.accelerated_file_hashes(poison, poison) is None
    assert import_accelerator.accelerated_excel_to_csv(poison, out_path=poison) is False
    assert import_accelerator._run_accelerator(poison) is None
    assert not marker.exists()


def test_import_accelerator_env_binary_and_global_authority_cannot_enable_consumer(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    execution_marker = tmp_path / "binary-executed"
    binary = tmp_path / "analytix-import-accelerator"
    binary.write_text(
        f"#!{sys.executable}\nfrom pathlib import Path\nPath({os.fspath(execution_marker)!r}).write_text('unsafe')\n",
        encoding="utf-8",
    )
    binary.chmod(0o700)
    monkeypatch.setenv("ANALYTIX_IMPORT_ACCELERATOR_BIN", os.fspath(binary))
    monkeypatch.setenv("ANALYTIX_IMPORT_ACCELERATOR_FORCE", "1")

    class GloballyAvailableAuthority:
        def status(self) -> managed_subprocess.ManagedExecutionAuthorityStatus:
            return managed_subprocess.ManagedExecutionAuthorityStatus(
                available=True,
                protocol_version=1,
                platform=sys.platform,
                reason_code="",
                guarantees=managed_subprocess._required_authority_guarantees(sys.platform),
            )

        def execute(self, _request) -> managed_subprocess.ManagedProcessResult:
            execution_marker.write_text("unsafe", encoding="utf-8")
            return managed_subprocess.ManagedProcessResult(returncode=0, stdout=b'{"ok":true}')

    monkeypatch.setattr(
        managed_subprocess,
        "_production_execution_authority",
        lambda: GloballyAvailableAuthority(),
    )

    source = tmp_path / f"case-{_SENSITIVE_ACCOUNT}.xlsx"
    output = tmp_path / "unauthorized-output.csv"
    assert import_accelerator.accelerated_excel_to_csv(source, out_path=output) is False
    with pytest.raises(import_accelerator.ImportAcceleratorUnavailableError) as raised:
        import_accelerator.excel_to_csv(source, out_path=output)

    assert str(raised.value) == _BLOCKER
    assert _SENSITIVE_ACCOUNT not in str(raised.value)
    assert not execution_marker.exists()
    assert not output.exists()


def test_import_accelerator_batch_boundary_creates_no_manifest_or_output(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    source = tmp_path / f"case-{_SENSITIVE_ACCOUNT}.csv"
    output = tmp_path / "unauthorized-output.csv"
    temp_root = tmp_path / "temp"
    temp_root.mkdir()
    monkeypatch.setenv("TMPDIR", os.fspath(temp_root))
    monkeypatch.setattr(tempfile, "tempdir", os.fspath(temp_root))
    before = list(temp_root.iterdir())

    item = import_accelerator.PrepareCsvBatchItem(
        path=source,
        out_path=output,
        limit=1,
        clean_if_needed=True,
    )
    with pytest.raises(import_accelerator.ImportAcceleratorUnavailableError) as raised:
        import_accelerator.prepare_csv_batch([item])

    assert str(raised.value) == _BLOCKER
    assert _SENSITIVE_ACCOUNT not in str(raised.value)
    assert list(temp_root.iterdir()) == before
    assert not output.exists()
