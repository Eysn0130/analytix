from __future__ import annotations

import hashlib
import ast
import inspect
import json
import os
import select
import shutil
import threading
from collections.abc import Iterator
from dataclasses import replace
from pathlib import Path

import pytest

from app.core import archive_extraction
from app.core.archive_extraction import ARCHIVE_EXTRACTOR_ENV, extract_archive
from app.core.external_runtime import ExternalRuntimeError, HostRuntimeResolution
from app.core.execution_authority_health import ExecutionAuthorityHealth
from app.core.managed_subprocess import ManagedProcessResult, ManagedProcessTimeout


def _sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _runtime(path: Path) -> HostRuntimeResolution:
    return HostRuntimeResolution(path, "", _sha256(path))


def _fake_runtime(tmp_path: Path) -> Path:
    runtime = tmp_path / "7zz"
    runtime.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
    runtime.chmod(0o700)
    return runtime


def _attempt_dirs(parent: Path) -> list[Path]:
    return sorted(parent.glob(".analytix-archive-attempt-*"))


def _staged_attempt_outputs(parent: Path) -> list[Path]:
    return sorted(
        path / archive_extraction._ARCHIVE_RUNTIME_DIR / "out"
        for path in _attempt_dirs(parent)
        if (path / archive_extraction._ARCHIVE_RUNTIME_DIR / "out").is_dir()
    )


@pytest.fixture(autouse=True)
def _admit_archive_execution_capability(
    monkeypatch: pytest.MonkeyPatch,
) -> Iterator[None]:
    archive_extraction._reset_archive_generation_registry_after_fork()
    monkeypatch.setattr(
        archive_extraction,
        "_archive_execution_capability_health",
        lambda: ExecutionAuthorityHealth(available=True, reason_code=""),
    )
    yield
    archive_extraction._reset_archive_generation_registry_after_fork()


def test_archive_execution_admission_precedes_runtime_source_password_and_output(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    blocker = "managed_process_capability_admission_unavailable"
    archive_path = tmp_path / "case-account-6222020202020202020.7z"
    output_dir = tmp_path / "must-not-exist" / "out"
    monkeypatch.setattr(
        archive_extraction,
        "_archive_execution_capability_health",
        lambda: ExecutionAuthorityHealth(available=False, reason_code=blocker),
    )

    def must_not_resolve():
        raise AssertionError("runtime resolution preceded capability admission")

    def must_not_read(*_args, **_kwargs):
        raise AssertionError("source/cache inspection preceded capability admission")

    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", must_not_resolve)
    monkeypatch.setattr(archive_extraction, "_read_regular_archive_source", must_not_read)

    with pytest.raises(ExternalRuntimeError, match=f"^{blocker}$"):
        extract_archive(
            archive_path,
            output_dir,
            password="secret\ncase-data",
            expected_sha256="not-inspected",
            reuse=True,
        )
    assert not output_dir.parent.exists()

    health = archive_extraction.archive_extractor_health()
    assert health == {
        "archive_extraction_available": False,
        "archive_extraction_reason": blocker,
        "archive_extraction_bin": "",
        "archive_extraction_bin_source": "",
        "archive_extraction_supported_exts": [],
    }


def test_unsupported_archive_platform_has_zero_source_exposure(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    reason = "archive_publish_platform_unavailable"
    archive_path = tmp_path / "account-6222020202020202020.7z"
    output_dir = tmp_path / "must-not-exist" / "out"
    monkeypatch.setattr(archive_extraction, "_archive_platform_admission_reason", lambda: reason)

    def must_not_touch(*_args, **_kwargs):
        raise AssertionError("platform admission must precede source, runtime, and output access")

    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", must_not_touch)
    monkeypatch.setattr(archive_extraction, "_assert_archive_publication_ready", must_not_touch)
    monkeypatch.setattr(archive_extraction, "_copy_regular_archive_source", must_not_touch)

    with pytest.raises(ExternalRuntimeError, match=f"^{reason}$"):
        extract_archive(
            archive_path,
            output_dir,
            password="not-inspected",
            expected_sha256="not-inspected",
            reuse=True,
        )
    assert not output_dir.parent.exists()


def test_archive_target_filesystem_exchange_probe_precedes_source_and_helper(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "account-6222020202020202020.7z"
    output_dir = tmp_path / "cache" / "out"
    source_touched = False
    helper_touched = False

    def noop_exchange(*_args, **_kwargs) -> None:
        return None

    def source_read(*_args, **_kwargs):
        nonlocal source_touched
        source_touched = True
        raise AssertionError("source read preceded target filesystem admission")

    def helper(*_args, **_kwargs):
        nonlocal helper_touched
        helper_touched = True
        raise AssertionError("helper preceded target filesystem admission")

    monkeypatch.setattr(archive_extraction, "_rename_archive_exchange", noop_exchange)
    monkeypatch.setattr(archive_extraction, "_read_regular_archive_source", source_read)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", helper)
    monkeypatch.setattr(archive_extraction, "run_managed_process", helper)

    with pytest.raises(
        ExternalRuntimeError,
        match="^archive_publish_platform_unavailable$",
    ):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256="0" * 64,
            reuse=False,
        )

    assert source_touched is False
    assert helper_touched is False
    assert output_dir.parent.is_dir()
    assert list(output_dir.parent.iterdir()) == []


def test_archive_target_filesystem_probe_rejects_replacement_capable_rename(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "account-6222020202020202020.7z"
    output_dir = tmp_path / "cache" / "out"
    source_touched = False

    def replacement_rename(
        source_fd: int,
        source_name: str,
        target_fd: int,
        target_name: str,
    ) -> None:
        os.rename(
            source_name,
            target_name,
            src_dir_fd=source_fd,
            dst_dir_fd=target_fd,
        )

    def source_read(*_args, **_kwargs):
        nonlocal source_touched
        source_touched = True
        raise AssertionError("source read preceded no-replace collision admission")

    monkeypatch.setattr(
        archive_extraction,
        "_rename_archive_noreplace",
        replacement_rename,
    )
    monkeypatch.setattr(
        archive_extraction,
        "_read_regular_archive_source",
        source_read,
    )

    with pytest.raises(
        ExternalRuntimeError,
        match="^archive_publish_platform_unavailable$",
    ):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256="0" * 64,
            reuse=False,
        )

    assert source_touched is False
    assert output_dir.parent.is_dir()
    assert list(output_dir.parent.iterdir()) == []


def test_archive_logical_path_identity_is_injective_or_fails_closed() -> None:
    nested = archive_extraction.combine_archive_path("outer.7z", "目录/流水.csv")
    assert nested == "outer.7z::目录/流水.csv"
    assert archive_extraction.archive_entry_leaf_name(nested) == "流水.csv"
    assert archive_extraction.combine_archive_path("", "my file.csv") == "my file.csv"

    for ambiguous in (
        "outer.7z::目录/流水.csv",
        " 流水.csv",
        "流水.csv ",
        "目录\\流水.csv",
        "目录/../流水.csv",
    ):
        with pytest.raises(ExternalRuntimeError, match="^archive_member_identity_invalid$"):
            archive_extraction.combine_archive_path("", ambiguous)


def test_extract_archive_rejects_untrusted_env_extractor_without_path_fallback(tmp_path, monkeypatch) -> None:
    extractor = tmp_path / "bsdtar"
    extractor.write_text(
        "\n".join(
            [
                "#!/bin/sh",
                "while [ \"$#\" -gt 0 ]; do",
                "  if [ \"$1\" = \"-C\" ]; then",
                "    shift",
                "    mkdir -p \"$1\"",
                "    printf 'account,amount\\n1,2\\n' > \"$1/inside.csv\"",
                "  fi",
                "  shift",
                "done",
                "exit 0",
                "",
            ]
        ),
        encoding="utf-8",
    )
    extractor.chmod(0o755)

    archive_path = tmp_path / "sample.rar"
    archive_path.write_bytes(b"not a real archive; fake extractor handles it")
    output_dir = tmp_path / "out"

    monkeypatch.setenv(ARCHIVE_EXTRACTOR_ENV, os.fspath(extractor))

    with pytest.raises(ValueError, match="^archive_runtime_unavailable$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=hashlib.sha256(archive_path.read_bytes()).hexdigest(),
            reuse=False,
        )

    assert not output_dir.exists()


def test_extract_archive_requires_independent_full_expected_sha256(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"archive-a")
    launched = False

    def must_not_launch(*_args, **_kwargs):
        nonlocal launched
        launched = True
        raise AssertionError

    monkeypatch.setattr(archive_extraction, "run_managed_process", must_not_launch)
    with pytest.raises(ExternalRuntimeError, match="^archive_source_identity_invalid$"):
        extract_archive(
            archive_path,
            tmp_path / "out",
            password=None,
            expected_sha256="",
            reuse=False,
        )
    assert launched is False

    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    with pytest.raises(ExternalRuntimeError, match="^archive_source_hash_mismatch$"):
        extract_archive(
            archive_path,
            tmp_path / "out",
            password=None,
            expected_sha256="0" * 64,
            reuse=False,
        )
    assert launched is False


def test_noncanonical_output_alias_has_zero_mutation_and_zero_source_exposure(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "account-6222020202020202020.7z"
    archive_path.write_bytes(b"archive")
    output_dir = tmp_path / "missing" / ".." / "cache" / "out"
    inspected = False

    def must_not_inspect(*_args, **_kwargs):
        nonlocal inspected
        inspected = True
        raise AssertionError("non-canonical output must fail before source or runtime inspection")

    monkeypatch.setattr(archive_extraction, "_read_regular_archive_source", must_not_inspect)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", must_not_inspect)

    with pytest.raises(ExternalRuntimeError, match="^archive_output_untrusted$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )

    assert inspected is False
    assert not (tmp_path / "missing").exists()
    assert not (tmp_path / "cache").exists()


def test_oversized_source_is_rejected_before_output_or_helper(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "oversized.7z"
    archive_path.write_bytes(b"12345")
    output_dir = tmp_path / "must-not-exist" / "out"
    launched = False
    monkeypatch.setattr(archive_extraction, "_ARCHIVE_MAX_FILE_BYTES", 4)

    def must_not_launch(*_args, **_kwargs):
        nonlocal launched
        launched = True
        raise AssertionError("source budget admission must precede runtime resolution")

    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", must_not_launch)
    monkeypatch.setattr(archive_extraction, "run_managed_process", must_not_launch)

    with pytest.raises(ExternalRuntimeError, match="^archive_source_quota_exceeded$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )

    assert launched is False
    # Target-filesystem exchange admission is intentionally performed before
    # source inspection. It may materialize the trusted parent, but leaves no
    # logical output or probe residue and never starts the helper.
    assert output_dir.parent.is_dir()
    assert list(output_dir.parent.iterdir()) == []



def test_archive_same_source_reuses_exact_live_generation_and_rejects_different_source(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"archive-A")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    launches: list[bytes] = []

    def fake_run(_command, **options):
        work = Path(options["cwd"])
        work_identity = work.stat()
        pinned_identity = os.fstat(options["cwd_fd"])
        assert (int(work_identity.st_dev), int(work_identity.st_ino)) == (
            int(pinned_identity.st_dev),
            int(pinned_identity.st_ino),
        )
        assert options["expected_cwd_device"] == int(pinned_identity.st_dev)
        assert options["expected_cwd_inode"] == int(pinned_identity.st_ino)
        source_bytes = (work / "input.7z").read_bytes()
        launches.append(source_bytes)
        (work / "out" / "inside.bin").write_bytes(source_bytes)
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    first_sha = _sha256(archive_path)
    extract_archive(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=first_sha,
        reuse=True,
    )
    first_root = output_dir.stat()
    extract_archive(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=first_sha,
        reuse=True,
    )

    assert launches == [b"archive-A"]
    reused_root = output_dir.stat()
    assert (int(reused_root.st_dev), int(reused_root.st_ino)) == (
        int(first_root.st_dev),
        int(first_root.st_ino),
    )
    assert (output_dir / "inside.bin").read_bytes() == b"archive-A"

    archive_path.write_bytes(b"archive-B")
    with pytest.raises(ExternalRuntimeError, match="^archive_publish_indeterminate$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=True,
        )

    blocked_root = output_dir.stat()
    assert launches == [b"archive-A"]
    assert (int(blocked_root.st_dev), int(blocked_root.st_ino)) == (
        int(first_root.st_dev),
        int(first_root.st_ino),
    )
    assert (output_dir / "inside.bin").read_bytes() == b"archive-A"


def test_public_receipt_excludes_private_runtime_and_execution_rejects_it(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "accounts.7z"
    archive_path.write_bytes(b"account\n6222020202020202020\n")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))

    def fake_run(_command, **options):
        work = Path(options["cwd"])
        (work / "out" / "accounts.csv").write_bytes(
            b"account\n6222020202020202020\n"
        )
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    with archive_extraction.open_archive_extraction_v1(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=_sha256(archive_path),
        reuse=False,
    ) as lease:
        assert [member.relative_path for member in lease.receipt.members] == [
            "accounts.csv"
        ]
        assert lease.receipt.file_count == 1
        assert lease.receipt.total_bytes == len(
            b"account\n6222020202020202020\n"
        )
        internal_receipt = replace(
            lease.receipt.members[0],
            relative_path=f"{archive_extraction._ARCHIVE_RUNTIME_DIR}/input.7z",
            sha256=_sha256(archive_path),
            size=archive_path.stat().st_size,
        )
        with pytest.raises(
            ExternalRuntimeError,
            match="^archive_member_receipt_invalid$",
        ):
            lease.publish_member(
                internal_receipt,
                tmp_path / "published",
                lineage={"case_id": "case-a"},
            )
        assert not (tmp_path / "published").exists()

def test_archive_resource_monitor_blocks_live_quota_and_registers_nothing(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"archive")
    output_dir = tmp_path / "out"
    runtime = tmp_path / "7zz"
    runtime.write_bytes(b"test extractor fixture")
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    monkeypatch.setattr(archive_extraction, "_ARCHIVE_MAX_FILE_BYTES", 64 * 1024)
    monkeypatch.setattr(archive_extraction, "_ARCHIVE_MAX_TOTAL_BYTES", 128 * 1024)

    def fake_run(_command, **options):
        (Path(options["cwd"]) / "out" / "bomb.bin").write_bytes(b"x" * (1024 * 1024))
        options["resource_monitor"]()
        raise AssertionError("quota monitor must reject before process completion")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)

    with pytest.raises(ExternalRuntimeError, match="^archive_output_quota_exceeded$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )
    assert not output_dir.exists()
    attempts = _attempt_dirs(tmp_path)
    assert len(attempts) == 1
    assert (
        attempts[0]
        / archive_extraction._ARCHIVE_RUNTIME_DIR
        / "out"
        / "bomb.bin"
    ).is_file()
    with pytest.raises(ExternalRuntimeError, match="^archive_output_quota_exceeded$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )
    assert not output_dir.exists()
    assert len(_attempt_dirs(tmp_path)) == 2


def test_wrong_password_then_correct_password_uses_a_fresh_attempt(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "encrypted.7z"
    archive_path.write_bytes(b"encrypted-account-6222020202020202020")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    helper_inputs: list[bytes | None] = []

    def fake_run(_command, **options):
        helper_inputs.append(options["input_bytes"])
        if options["input_bytes"] == b"wrong\n":
            return ManagedProcessResult(returncode=2, stdout=b"")
        work = Path(options["cwd"])
        (work / "out" / "accounts.csv").write_bytes(
            b"account\n6222020202020202020\n"
        )
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    with pytest.raises(
        ExternalRuntimeError,
        match="^archive_password_or_format_invalid$",
    ):
        extract_archive(
            archive_path,
            output_dir,
            password="wrong",
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )
    assert not output_dir.exists()
    assert len(_attempt_dirs(tmp_path)) == 1

    extract_archive(
        archive_path,
        output_dir,
        password="correct",
        expected_sha256=_sha256(archive_path),
        reuse=False,
    )

    assert helper_inputs == [b"wrong\n", b"correct\n"]
    assert (output_dir / "accounts.csv").read_bytes() == (
        b"account\n6222020202020202020\n"
    )
    assert len(_attempt_dirs(tmp_path)) == 2


def test_timeout_residue_does_not_poison_retry(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"archive")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    calls = 0

    def fake_run(_command, **options):
        nonlocal calls
        calls += 1
        if calls == 1:
            raise ManagedProcessTimeout("timeout")
        work = Path(options["cwd"])
        (work / "out" / "inside.bin").write_bytes(b"verified")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    with pytest.raises(ExternalRuntimeError, match="^archive_extract_timeout$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )
    assert not output_dir.exists()

    extract_archive(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=_sha256(archive_path),
        reuse=False,
    )
    assert calls == 2
    assert (output_dir / "inside.bin").read_bytes() == b"verified"


def test_concurrent_last_registry_slot_admits_one_before_source_and_helper(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archives = [tmp_path / "a.7z", tmp_path / "b.7z"]
    outputs = [tmp_path / "case-a" / "out", tmp_path / "case-b" / "out"]
    for index, archive in enumerate(archives):
        archive.write_bytes(f"archive-{index}".encode())
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    monkeypatch.setattr(archive_extraction, "_ARCHIVE_GENERATION_REGISTRY_MAX", 1)
    real_reserve = archive_extraction._reserve_archive_generation_path
    reserve_barrier = threading.Barrier(2)

    def synchronized_reserve(output_dir: Path):
        reserve_barrier.wait(timeout=5)
        return real_reserve(output_dir)

    monkeypatch.setattr(
        archive_extraction,
        "_reserve_archive_generation_path",
        synchronized_reserve,
    )
    real_source_read = archive_extraction._read_regular_archive_source
    source_reads = {os.fspath(path): 0 for path in archives}
    count_lock = threading.Lock()
    helper_launches = 0

    def counted_source_read(path: Path):
        with count_lock:
            source_reads[os.fspath(path)] += 1
        return real_source_read(path)

    def fake_run(_command, **options):
        nonlocal helper_launches
        with count_lock:
            helper_launches += 1
        work = Path(options["cwd"])
        (work / "out" / "inside.bin").write_bytes(b"verified")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "_read_regular_archive_source", counted_source_read)
    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    results: list[tuple[int, str]] = []

    def worker(index: int) -> None:
        try:
            extract_archive(
                archives[index],
                outputs[index],
                password=None,
                expected_sha256=_sha256(archives[index]),
                reuse=False,
            )
            result = "ok"
        except ExternalRuntimeError as error:
            result = str(error)
        with count_lock:
            results.append((index, result))

    threads = [threading.Thread(target=worker, args=(index,)) for index in range(2)]
    for thread in threads:
        thread.start()
    for thread in threads:
        thread.join(timeout=10)
        assert not thread.is_alive()

    assert sorted(result for _, result in results) == [
        "archive_generation_registry_capacity_exceeded",
        "ok",
    ]
    winner = next(index for index, result in results if result == "ok")
    loser = 1 - winner
    assert helper_launches == 1
    assert source_reads[os.fspath(archives[loser])] == 0
    assert outputs[winner].is_dir()
    assert not outputs[loser].exists()


def test_reservation_token_cannot_commit_twice_or_cross_logical_keys(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"archive")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    captured: list[archive_extraction._ArchiveGenerationReservationV1] = []
    real_reserve = archive_extraction._reserve_archive_generation_path

    def capture(output: Path):
        reservation = real_reserve(output)
        captured.append(reservation)
        return reservation

    def fake_run(_command, **options):
        work = Path(options["cwd"])
        (work / "out" / "inside.bin").write_bytes(b"verified")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "_reserve_archive_generation_path", capture)
    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    published = archive_extraction._extract_archive_generation(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=_sha256(archive_path),
        reuse=False,
    )
    try:
        assert len(captured) == 1
        with archive_extraction._open_canonical_publish_parent(output_dir) as parent:
            with pytest.raises(
                ExternalRuntimeError,
                match="^archive_generation_reservation_invalid$",
            ):
                archive_extraction._register_archive_generation(
                    parent,
                    published,
                    captured[0],
                )

        other_output = tmp_path / "other"
        other_reservation = real_reserve(other_output)
        try:
            with archive_extraction._open_canonical_publish_parent(output_dir) as parent:
                with pytest.raises(
                    ExternalRuntimeError,
                    match="^archive_generation_reservation_invalid$",
                ):
                    archive_extraction._bind_archive_generation_reservation(
                        parent,
                        other_reservation,
                    )
        finally:
            archive_extraction._release_archive_generation_reservation(
                other_reservation
            )
    finally:
        os.close(published.root_fd)



def test_archive_unregistered_output_is_never_replaced_or_deleted(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "account-source.7z"
    archive_path.write_bytes(b"archive")
    output_dir = tmp_path / "out"
    output_dir.mkdir(mode=0o700)
    bank_accounts = output_dir / "bank-accounts.csv"
    bank_accounts.write_bytes(b"account\n6222020202020202020\n")
    root_before = output_dir.stat()
    file_before = bank_accounts.stat()

    def must_not_read_or_launch(*_args, **_kwargs):
        raise AssertionError("unregistered output must block before source exposure or helper launch")

    monkeypatch.setattr(archive_extraction, "_read_regular_archive_source", must_not_read_or_launch)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", must_not_read_or_launch)
    monkeypatch.setattr(archive_extraction, "run_managed_process", must_not_read_or_launch)

    with pytest.raises(ExternalRuntimeError, match="^archive_publish_indeterminate$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )

    root_after = output_dir.stat()
    file_after = bank_accounts.stat()
    assert (int(root_after.st_dev), int(root_after.st_ino)) == (
        int(root_before.st_dev),
        int(root_before.st_ino),
    )
    assert (int(file_after.st_dev), int(file_after.st_ino)) == (
        int(file_before.st_dev),
        int(file_before.st_ino),
    )
    assert bank_accounts.read_bytes() == b"account\n6222020202020202020\n"

@pytest.mark.parametrize("mutation", ["content", "addition", "deletion", "type"])
def test_archive_output_tree_drift_blocks_fresh_generation_without_mutation(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    mutation: str,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"stable-source")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    launches = 0

    def fake_run(_command, **options):
        nonlocal launches
        launches += 1
        work = Path(options["cwd"])
        nested = work / "out" / "nested"
        nested.mkdir()
        (work / "out" / "first.bin").write_bytes(b"AAAA")
        (nested / "second.bin").write_bytes(b"BBBB")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    expected_sha256 = _sha256(archive_path)
    extract_archive(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=expected_sha256,
        reuse=True,
    )
    assert launches == 1
    marker = json.loads((output_dir / ".analytix-archive-extract.json").read_text(encoding="utf-8"))
    assert marker["schema_version"] == 4
    assert len(marker["publication_nonce"]) == 64
    inventory_paths = {entry["path"] for entry in marker["output_inventory"]}
    assert {"first.bin", "nested", "nested/second.bin"} <= inventory_paths
    assert archive_extraction._ARCHIVE_RUNTIME_DIR not in inventory_paths
    assert marker["output_entry_count"] == len(marker["output_inventory"])
    assert len(marker["output_tree_sha256"]) == 64
    assert [
        entry["path"]
        for entry in marker["output_inventory"]
        if not archive_extraction._is_archive_runtime_entry(entry["path"])
    ] == [
        "first.bin",
        "nested",
        "nested/second.bin",
    ]

    if mutation == "content":
        target = output_dir / "first.bin"
        original = target.stat()
        target.write_bytes(b"ZZZZ")
        os.utime(target, ns=(original.st_atime_ns, original.st_mtime_ns))
    elif mutation == "addition":
        (output_dir / "unexpected.bin").write_bytes(b"extra")
    elif mutation == "deletion":
        (output_dir / "nested" / "second.bin").unlink()
    else:
        shutil.rmtree(output_dir / "nested")
        (output_dir / "nested").write_bytes(b"BBBB")

    with pytest.raises(ExternalRuntimeError, match="^archive_publish_indeterminate$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=expected_sha256,
            reuse=True,
        )

    assert launches == 1
    if mutation == "content":
        assert (output_dir / "first.bin").read_bytes() == b"ZZZZ"
    elif mutation == "addition":
        assert (output_dir / "unexpected.bin").read_bytes() == b"extra"
    elif mutation == "deletion":
        assert not (output_dir / "nested" / "second.bin").exists()
    else:
        assert (output_dir / "nested").is_file()


def test_forged_archive_marker_cannot_authorize_cache(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"source")
    output_dir = tmp_path / "out"
    output_dir.mkdir(mode=0o700)
    fabricated = output_dir / "fabricated.csv"
    fabricated.write_bytes(b"account,amount\nforged,999\n")
    fabricated.chmod(0o600)
    root_fd = os.open(output_dir, os.O_RDONLY | os.O_DIRECTORY)
    try:
        source_identity = archive_extraction._read_regular_archive_source(archive_path)
        summary = archive_extraction._snapshot_extracted_tree_fd(root_fd)
        marker = {
            **source_identity.marker(),
            **summary.marker(),
            "publication_nonce": "a" * 64,
        }
    finally:
        os.close(root_fd)
    marker_path = output_dir / ".analytix-archive-extract.json"
    marker_path.write_text(json.dumps(marker), encoding="utf-8")
    marker_path.chmod(0o600)

    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    launches = 0

    def fake_run(_command, **options):
        nonlocal launches
        launches += 1
        (Path(options["cwd"]) / "out" / "accepted.csv").write_bytes(b"account,amount\nreal,1\n")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    with pytest.raises(ExternalRuntimeError, match="^archive_publish_indeterminate$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=True,
        )

    assert launches == 0
    assert fabricated.read_bytes() == b"account,amount\nforged,999\n"
    assert not (output_dir / "accepted.csv").exists()


def test_archive_disk_generation_without_live_host_registry_is_not_reused(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"accepted-generation")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    launches = 0

    def fake_run(_command, **options):
        nonlocal launches
        launches += 1
        work = Path(options["cwd"])
        (work / "out" / "inside.bin").write_bytes((work / "input.7z").read_bytes())
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    extract_archive(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=_sha256(archive_path),
        reuse=True,
    )
    root_identity = output_dir.stat()
    before = (output_dir / "inside.bin").read_bytes()
    archive_extraction._reset_archive_generation_registry_after_fork()

    with pytest.raises(ExternalRuntimeError, match="^archive_publish_indeterminate$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=True,
        )

    after = output_dir.stat()
    assert launches == 1
    assert (int(after.st_dev), int(after.st_ino)) == (
        int(root_identity.st_dev),
        int(root_identity.st_ino),
    )
    assert (output_dir / "inside.bin").read_bytes() == before



def test_concurrent_archive_leases_share_exact_live_authority_without_blocking(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"accepted-generation")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    launches = 0
    launches_lock = threading.Lock()

    def fake_run(_command, **options):
        nonlocal launches
        with launches_lock:
            launches += 1
        work = Path(options["cwd"])
        (work / "out" / "inside.bin").write_bytes((work / "input.7z").read_bytes())
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    first_entered = threading.Event()
    release_first = threading.Event()
    second_entered = threading.Event()
    errors: list[BaseException] = []
    root_identities: list[tuple[int, int]] = []

    def use_lease(*, first: bool) -> None:
        try:
            with archive_extraction.open_archive_extraction_v1(
                archive_path,
                output_dir,
                password=None,
                expected_sha256=_sha256(archive_path),
                reuse=False,
            ) as lease:
                root_identities.append(
                    (lease.receipt.root_device, lease.receipt.root_inode)
                )
                if first:
                    first_entered.set()
                    assert release_first.wait(timeout=5)
                else:
                    second_entered.set()
                lease.verify()
        except BaseException as error:
            errors.append(error)

    first_thread = threading.Thread(target=use_lease, kwargs={"first": True})
    second_thread = threading.Thread(target=use_lease, kwargs={"first": False})
    first_thread.start()
    assert first_entered.wait(timeout=5)
    second_thread.start()
    assert second_entered.wait(timeout=5)
    release_first.set()
    first_thread.join(timeout=5)
    second_thread.join(timeout=5)

    assert not first_thread.is_alive()
    assert not second_thread.is_alive()
    assert errors == []
    assert launches == 1
    assert len(root_identities) == 2
    assert root_identities[0] == root_identities[1]
    assert (output_dir / "inside.bin").read_bytes() == b"accepted-generation"

def test_archive_tree_quota_is_enforced_while_scandir_is_streamed(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    root = tmp_path / "out"
    root.mkdir()
    for index in range(10):
        (root / f"entry-{index}.bin").write_bytes(b"x")
    real_scandir = archive_extraction.os.scandir
    yielded = 0

    class GuardedScandir:
        def __init__(self, path) -> None:
            self._iterator = real_scandir(path)

        def __enter__(self):
            return self

        def __exit__(self, *_args) -> None:
            self._iterator.close()

        def __iter__(self):
            return self

        def __next__(self):
            nonlocal yielded
            yielded += 1
            if yielded > 3:
                raise AssertionError("directory was materialized before quota enforcement")
            return next(self._iterator)

    monkeypatch.setattr(archive_extraction, "_ARCHIVE_MAX_ENTRIES", 2)
    monkeypatch.setattr(archive_extraction.os, "scandir", lambda path: GuardedScandir(path))

    with pytest.raises(ExternalRuntimeError, match="^archive_output_quota_exceeded$"):
        archive_extraction._validate_extracted_tree(root)
    assert yielded == 3



def test_archive_attempt_directory_fsync_failure_does_not_poison_retry(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"new-generation")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    launches = 0

    def fake_run(_command, **options):
        nonlocal launches
        launches += 1
        work = Path(options["cwd"])
        (work / "out" / "inside.bin").write_bytes((work / "input.7z").read_bytes())
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    real_sync = archive_extraction._sync_archive_parent
    injected = False

    def fail_created_directory_sync(parent, phase: str, **options) -> None:
        nonlocal injected
        if not injected and phase == "attempt_directory_created":
            injected = True
            raise ExternalRuntimeError("archive_publish_failed")
        real_sync(parent, phase, **options)

    monkeypatch.setattr(archive_extraction, "_sync_archive_parent", fail_created_directory_sync)
    with pytest.raises(ExternalRuntimeError, match="^archive_publish_failed$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )

    assert injected is True
    assert launches == 0
    assert not output_dir.exists()
    attempts = _attempt_dirs(tmp_path)
    assert len(attempts) == 1
    assert list(attempts[0].iterdir()) == []

    extract_archive(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=_sha256(archive_path),
        reuse=False,
    )

    assert launches == 1
    assert (output_dir / "inside.bin").read_bytes() == b"new-generation"
    assert len(_attempt_dirs(tmp_path)) == 2


@pytest.mark.parametrize("failure_point", ["marker", "registry"])
def test_archive_failed_publication_never_leaves_the_logical_output_visible(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    failure_point: str,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"new-generation")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    launches = 0

    def fake_run(_command, **options):
        nonlocal launches
        launches += 1
        work = Path(options["cwd"])
        (work / "out" / "inside.bin").write_bytes((work / "input.7z").read_bytes())
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    if failure_point == "marker":
        monkeypatch.setattr(
            archive_extraction,
            "_write_extract_marker_fd",
            lambda *_args, **_kwargs: (_ for _ in ()).throw(
                ExternalRuntimeError("archive_publish_failed")
            ),
        )
    else:
        monkeypatch.setattr(
            archive_extraction,
            "_register_archive_generation",
            lambda *_args, **_kwargs: (_ for _ in ()).throw(
                ExternalRuntimeError("archive_publish_failed")
            ),
        )

    with pytest.raises(ExternalRuntimeError, match="^archive_publish_failed$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )

    assert launches == 1
    assert not output_dir.exists()
    staged_outputs = _staged_attempt_outputs(tmp_path)
    assert len(staged_outputs) == 1
    assert (staged_outputs[0] / "inside.bin").read_bytes() == b"new-generation"
    assert (staged_outputs[0] / archive_extraction._MARKER_FILE).exists() is (
        failure_point == "registry"
    )
    assert archive_extraction._ARCHIVE_GENERATION_REGISTRY == {}

def test_forged_archive_publication_state_has_zero_mutation(
    tmp_path: Path,
) -> None:
    output_dir = tmp_path / "out"
    output_dir.mkdir(mode=0o700)
    valuable = output_dir / "accepted.csv"
    valuable.write_bytes(b"account,amount\n6222020202020202020,1\n")
    valuable.chmod(0o600)
    identity = output_dir.stat()
    state_path = tmp_path / ".out.analytix-publish-state"
    state_path.write_text(
        json.dumps(
            {
                "schema_version": 2,
                "publish_name": ".out.analytix-publish-forged",
                "new_device": int(identity.st_dev),
                "new_inode": int(identity.st_ino),
                "had_previous": False,
                "previous_device": 0,
                "previous_inode": 0,
                "backup_name": ".out.analytix-previous-forged",
                "quarantine_name": ".out.analytix-quarantine-forged",
            }
        ),
        encoding="utf-8",
    )
    state_path.chmod(0o600)
    before_state = state_path.read_bytes()
    before_value = valuable.read_bytes()
    before_identity = (int(identity.st_dev), int(identity.st_ino))

    with archive_extraction._open_canonical_publish_parent(output_dir) as parent:
        with archive_extraction._archive_publication_lock(parent):
            with pytest.raises(ExternalRuntimeError, match="^archive_publish_indeterminate$"):
                archive_extraction._recover_archive_publication(parent)

    after_identity = output_dir.stat()
    assert (int(after_identity.st_dev), int(after_identity.st_ino)) == before_identity
    assert valuable.read_bytes() == before_value
    assert state_path.read_bytes() == before_state


def test_archive_publication_lock_is_released_before_parent_authority_closes(
    tmp_path: Path,
) -> None:
    import fcntl

    output_dir = tmp_path / "out"
    with archive_extraction._open_canonical_publish_parent(output_dir) as first:
        with archive_extraction._open_canonical_publish_parent(output_dir) as second:
            contender = os.dup(second.descriptor)
            try:
                with archive_extraction._archive_publication_lock(first):
                    with pytest.raises((BlockingIOError, OSError)):
                        fcntl.flock(contender, fcntl.LOCK_EX | fcntl.LOCK_NB)
                fcntl.flock(contender, fcntl.LOCK_EX | fcntl.LOCK_NB)
                fcntl.flock(contender, fcntl.LOCK_UN)
            finally:
                os.close(contender)



def test_archive_registry_capacity_admission_never_evicts_live_generation(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(archive_extraction, "_ARCHIVE_GENERATION_REGISTRY_MAX", 1)
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    launches: list[bytes] = []

    def fake_run(_command, **options):
        work = Path(options["cwd"])
        source_bytes = (work / "input.7z").read_bytes()
        launches.append(source_bytes)
        (work / "out" / "inside.bin").write_bytes(source_bytes)
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    first_source = tmp_path / "first.7z"
    first_source.write_bytes(b"first")
    first_output = tmp_path / "first-output"
    extract_archive(
        first_source,
        first_output,
        password=None,
        expected_sha256=_sha256(first_source),
        reuse=True,
    )
    first_identity = first_output.stat()

    second_source = tmp_path / "second.7z"
    second_source.write_bytes(b"second")
    second_output = tmp_path / "second-output"
    with pytest.raises(
        ExternalRuntimeError,
        match="^archive_generation_registry_capacity_exceeded$",
    ):
        extract_archive(
            second_source,
            second_output,
            password=None,
            expected_sha256=_sha256(second_source),
            reuse=True,
        )

    assert launches == [b"first"]
    assert not second_output.exists()
    extract_archive(
        first_source,
        first_output,
        password=None,
        expected_sha256=_sha256(first_source),
        reuse=True,
    )
    reused_identity = first_output.stat()
    assert launches == [b"first"]
    assert (int(reused_identity.st_dev), int(reused_identity.st_ino)) == (
        int(first_identity.st_dev),
        int(first_identity.st_ino),
    )
    assert (first_output / "inside.bin").read_bytes() == b"first"


def test_nested_archive_leases_do_not_self_deadlock(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    launches = 0

    def fake_run(_command, **options):
        nonlocal launches
        launches += 1
        work = Path(options["cwd"])
        (work / "out" / "inside.bin").write_bytes((work / "input.7z").read_bytes())
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    outer_source = tmp_path / "outer.7z"
    inner_source = tmp_path / "inner.7z"
    outer_source.write_bytes(b"outer")
    inner_source.write_bytes(b"inner")

    with archive_extraction.open_archive_extraction_v1(
        outer_source,
        tmp_path / "outer-output",
        password=None,
        expected_sha256=_sha256(outer_source),
    ) as outer:
        with archive_extraction.open_archive_extraction_v1(
            inner_source,
            tmp_path / "inner-output",
            password=None,
            expected_sha256=_sha256(inner_source),
        ) as inner:
            outer.verify()
            inner.verify()

    assert launches == 2

def test_unreceipted_quarantine_blocks_visible_output_without_mutation(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"accepted-generation")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    launches = 0

    def fake_run(_command, **options):
        nonlocal launches
        launches += 1
        work = Path(options["cwd"])
        (work / "out" / "inside.bin").write_bytes((work / "input.7z").read_bytes())
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    expected_sha256 = _sha256(archive_path)
    extract_archive(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=expected_sha256,
        reuse=True,
    )
    quarantine = tmp_path / ".out.analytix-quarantine-deadbeef"
    quarantine.mkdir()
    (quarantine / "obsolete.bin").write_bytes(b"prior accepted bytes")

    with pytest.raises(ExternalRuntimeError, match="^archive_publish_indeterminate$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=expected_sha256,
            reuse=True,
        )

    assert launches == 1
    assert quarantine.is_dir()
    assert (quarantine / "obsolete.bin").read_bytes() == b"prior accepted bytes"
    assert (output_dir / "inside.bin").read_bytes() == b"accepted-generation"



def test_nested_archive_publication_lock_fails_closed_without_deadlock(
    tmp_path: Path,
) -> None:
    first_output = tmp_path / "first" / "out"
    second_output = tmp_path / "second" / "out"

    with archive_extraction._open_canonical_publish_parent(first_output) as first:
        with archive_extraction._open_canonical_publish_parent(second_output) as second:
            with archive_extraction._archive_publication_lock(first):
                with pytest.raises(
                    ExternalRuntimeError,
                    match="^archive_publish_indeterminate$",
                ):
                    with archive_extraction._archive_publication_lock(second):
                        raise AssertionError("nested independent flock must not be acquired")

@pytest.mark.skipif(os.name == "nt", reason="POSIX no-follow directory authority")
def test_archive_publish_parent_symlink_ancestor_has_zero_outside_side_effect(
    tmp_path: Path,
) -> None:
    outside = tmp_path / "outside"
    outside.mkdir()
    alias = tmp_path / "alias"
    alias.symlink_to(outside, target_is_directory=True)
    output_dir = alias / "must-not-be-created" / "out"

    with pytest.raises(ExternalRuntimeError, match="^archive_output_untrusted$"):
        with archive_extraction._open_canonical_publish_parent(output_dir):
            raise AssertionError("symlink ancestor must not be admitted")

    assert not (outside / "must-not-be-created").exists()


@pytest.mark.skipif(os.name == "nt", reason="POSIX dir-fd copy authority")
def test_archive_copy_rejects_directory_swap_without_reading_outside_tree(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    source = tmp_path / "source"
    nested = source / "nested"
    nested.mkdir(parents=True)
    (nested / "inside.bin").write_bytes(b"inside")
    outside = tmp_path / "outside"
    outside.mkdir()
    outside_secret = outside / "must-not-copy.bin"
    outside_secret.write_bytes(b"outside-secret")
    target = tmp_path / "target"
    target.mkdir()
    displaced = source / "nested-displaced"
    real_open = archive_extraction.os.open
    swapped = False

    def swap_before_directory_open(path, flags, *args, **kwargs):
        nonlocal swapped
        if not swapped and path == "nested" and kwargs.get("dir_fd") is not None:
            swapped = True
            nested.rename(displaced)
            nested.symlink_to(outside, target_is_directory=True)
        return real_open(path, flags, *args, **kwargs)

    monkeypatch.setattr(archive_extraction.os, "open", swap_before_directory_open)
    with pytest.raises(ExternalRuntimeError, match="^archive_output_invalid$"):
        archive_extraction._copy_extracted_tree_secure(source, target)

    assert swapped is True
    assert not (target / "nested" / outside_secret.name).exists()
    assert b"outside-secret" not in [
        path.read_bytes()
        for path in target.rglob("*")
        if path.is_file() and not path.is_symlink()
    ]


def test_archive_inventory_mutation_after_validation_cannot_be_consumed(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"archive")
    output_dir = tmp_path / "cache" / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))

    def fake_run(_command, **options):
        (Path(options["cwd"]) / "out" / "accepted.csv").write_bytes(b"account,amount\nA,1\n")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    expected_sha256 = _sha256(archive_path)
    accepted_generation = None
    with pytest.raises(ExternalRuntimeError, match="^archive_inventory_changed_during_use$"):
        with archive_extraction.open_archive_extraction_v1(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=expected_sha256,
        ) as lease:
            (output_dir / "late.csv").write_bytes(b"fabricated,amount\nX,999\n")
            members = lease.file_members()
            assert [member.relative_path for member in members] == ["accepted.csv"]
            accepted_generation = lease.publish_member(
                members[0],
                tmp_path / "verified-members",
                lineage={"kind": "test"},
            )
            with pytest.raises(ExternalRuntimeError, match="^archive_member_receipt_invalid$"):
                lease.publish_member(
                    replace(members[0], relative_path="late.csv"),
                    tmp_path / "verified-members",
                    lineage={"kind": "test"},
                )

    assert accepted_generation is not None
    assert accepted_generation.path.read_bytes() == b"account,amount\nA,1\n"
    assert b"fabricated" not in [path.read_bytes() for path in (tmp_path / "verified-members").glob("*.csv")]


def test_archive_member_replacement_cannot_be_materialized(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"archive")
    output_dir = tmp_path / "cache" / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))

    def fake_run(_command, **options):
        (Path(options["cwd"]) / "out" / "accepted.csv").write_bytes(b"accepted")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    with pytest.raises(ExternalRuntimeError, match="^archive_inventory_changed_during_use$"):
        with archive_extraction.open_archive_extraction_v1(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
        ) as lease:
            member = lease.file_members()[0]
            (output_dir / "accepted.csv").rename(output_dir / "accepted-original.csv")
            (output_dir / "accepted.csv").write_bytes(b"replacement")
            with pytest.raises(ExternalRuntimeError, match="^archive_member_identity_mismatch$"):
                lease.publish_member(
                    member,
                    tmp_path / "verified-members",
                    lineage={"kind": "test"},
                )

    assert not (tmp_path / "verified-members").exists()


def test_archive_lease_root_swap_back_reads_only_pinned_generation(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"archive")
    output_dir = tmp_path / "cache" / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))

    def fake_run(_command, **options):
        (Path(options["cwd"]) / "out" / "accepted.csv").write_bytes(b"accepted")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    displaced = output_dir.parent / "out-displaced"
    decoy = output_dir.parent / "out-decoy"
    with pytest.raises(ExternalRuntimeError, match="^archive_inventory_changed_during_use$"):
        with archive_extraction.open_archive_extraction_v1(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
        ) as lease:
            output_dir.rename(displaced)
            output_dir.mkdir()
            (output_dir / "accepted.csv").write_bytes(b"decoy")
            generation = lease.publish_member(
                lease.file_members()[0],
                tmp_path / "verified-members",
                lineage={"kind": "test"},
            )
            output_dir.rename(decoy)
            displaced.rename(output_dir)

    assert generation.path.read_bytes() == b"accepted"
    assert (decoy / "accepted.csv").read_bytes() == b"decoy"



def test_archive_publication_parent_swap_back_uses_pinned_parent_descriptor(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"new-generation")
    parent_path = tmp_path / "cache"
    output_dir = parent_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))

    def fake_run(_command, **options):
        (Path(options["cwd"]) / "out" / "inside.bin").write_bytes(b"new-generation")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    real_sync = archive_extraction._sync_archive_parent
    displaced = tmp_path / "cache-displaced"
    decoy_saved = tmp_path / "cache-decoy"
    swapped = False

    def swap_back_during_directory_sync(parent, phase: str, **options) -> None:
        nonlocal swapped
        if not swapped and phase == "attempt_directory_created":
            swapped = True
            parent_path.rename(displaced)
            parent_path.mkdir()
            (parent_path / "sentinel").write_bytes(b"decoy")
            os.fsync(parent.descriptor)
            parent_path.rename(decoy_saved)
            displaced.rename(parent_path)
        real_sync(parent, phase, **options)

    monkeypatch.setattr(archive_extraction, "_sync_archive_parent", swap_back_during_directory_sync)
    extract_archive(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=_sha256(archive_path),
        reuse=False,
    )

    assert swapped is True
    assert (output_dir / "inside.bin").read_bytes() == b"new-generation"
    assert (decoy_saved / "sentinel").read_bytes() == b"decoy"
    assert not (decoy_saved / "out").exists()


def test_archive_parent_swap_failure_never_writes_or_deletes_decoy_parent(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"new-generation")
    parent_path = tmp_path / "cache"
    output_dir = parent_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))

    def fake_run(_command, **options):
        (Path(options["cwd"]) / "out" / "inside.bin").write_bytes(b"new-generation")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    real_sync = archive_extraction._sync_archive_parent
    displaced = tmp_path / "cache-displaced"
    swapped = False

    def swap_then_fail(parent, phase: str, **options) -> None:
        nonlocal swapped
        if not swapped and phase == "attempt_directory_created":
            swapped = True
            parent_path.rename(displaced)
            parent_path.mkdir(mode=0o700)
            (parent_path / "bank-accounts.csv").write_bytes(
                b"account\n6222020202020202020\n"
            )
            raise ExternalRuntimeError("archive_publish_failed")
        real_sync(parent, phase, **options)

    monkeypatch.setattr(archive_extraction, "_sync_archive_parent", swap_then_fail)
    with pytest.raises(ExternalRuntimeError, match="^archive_publish_failed$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )

    assert swapped is True
    assert not (displaced / "out").exists()
    attempts = _attempt_dirs(displaced)
    assert len(attempts) == 1
    assert list(attempts[0].iterdir()) == []
    assert (parent_path / "bank-accounts.csv").read_bytes() == (
        b"account\n6222020202020202020\n"
    )
    assert not (parent_path / "out").exists()


def test_archive_exchange_placeholder_prevents_recreated_rollback_target(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"generation")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(
        archive_extraction,
        "archive_extractor_runtime",
        lambda: _runtime(runtime),
    )

    def fake_run(_command, **options):
        (Path(options["cwd"]) / "out" / "inside.bin").write_bytes(
            b"generation"
        )
        return ManagedProcessResult(returncode=0, stdout=b"")

    real_sync = archive_extraction._sync_archive_directory_fd
    recreation_blocked = False

    def recreate_then_fail(descriptor: int, phase: str) -> None:
        nonlocal recreation_blocked
        if phase == "output_generation_source_published":
            try:
                os.mkdir("out", mode=0o700, dir_fd=descriptor)
            except FileExistsError:
                recreation_blocked = True
            else:
                raise AssertionError("exchange source slot became absent")
            raise ExternalRuntimeError("archive_publish_failed")
        real_sync(descriptor, phase)

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    monkeypatch.setattr(
        archive_extraction,
        "_sync_archive_directory_fd",
        recreate_then_fail,
    )

    with pytest.raises(ExternalRuntimeError, match="^archive_publish_failed$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )

    assert recreation_blocked is True
    assert not output_dir.exists()
    staged_outputs = _staged_attempt_outputs(tmp_path)
    assert len(staged_outputs) == 1
    assert (staged_outputs[0] / "inside.bin").read_bytes() == b"generation"
    assert archive_extraction._ARCHIVE_GENERATION_REGISTRY == {}


@pytest.mark.parametrize("failure_point", ["source_fsync", "parent_fsync", "registry"])
def test_archive_every_post_visibility_fsync_cut_rolls_back_exact_generation(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    failure_point: str,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"generation")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))

    def fake_run(_command, **options):
        work = Path(options["cwd"])
        (work / "out" / "inside.bin").write_bytes(b"generation")
        (work / "out" / "second.bin").write_bytes(b"second")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    if failure_point == "source_fsync":
        real_sync_directory = archive_extraction._sync_archive_directory_fd

        def fail_source_sync(descriptor: int, phase: str) -> None:
            if phase == "output_generation_source_published":
                raise ExternalRuntimeError("archive_publish_failed")
            real_sync_directory(descriptor, phase)

        monkeypatch.setattr(
            archive_extraction,
            "_sync_archive_directory_fd",
            fail_source_sync,
        )
    elif failure_point == "parent_fsync":
        real_sync_parent = archive_extraction._sync_archive_parent

        def fail_parent_sync(parent, phase: str, **options) -> None:
            if phase == "output_generation_published":
                raise ExternalRuntimeError("archive_publish_failed")
            real_sync_parent(parent, phase, **options)

        monkeypatch.setattr(
            archive_extraction,
            "_sync_archive_parent",
            fail_parent_sync,
        )
    else:
        monkeypatch.setattr(
            archive_extraction,
            "_register_archive_generation",
            lambda *_args, **_kwargs: (_ for _ in ()).throw(
                ExternalRuntimeError("archive_publish_failed")
            ),
        )

    with pytest.raises(ExternalRuntimeError, match="^archive_publish_failed$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )

    assert not output_dir.exists()
    staged_outputs = _staged_attempt_outputs(tmp_path)
    assert len(staged_outputs) == 1
    staged = staged_outputs[0]
    assert (staged / "inside.bin").read_bytes() == b"generation"
    assert (staged / "second.bin").read_bytes() == b"second"
    assert (staged / archive_extraction._MARKER_FILE).is_file()
    staged_identity = staged.stat()
    first_identity = (staged / "inside.bin").stat()
    second_identity = (staged / "second.bin").stat()
    assert (int(staged.stat().st_dev), int(staged.stat().st_ino)) == (
        int(staged_identity.st_dev),
        int(staged_identity.st_ino),
    )
    assert (
        int((staged / "inside.bin").stat().st_dev),
        int((staged / "inside.bin").stat().st_ino),
    ) == (int(first_identity.st_dev), int(first_identity.st_ino))
    assert (
        int((staged / "second.bin").stat().st_dev),
        int((staged / "second.bin").stat().st_ino),
    ) == (int(second_identity.st_dev), int(second_identity.st_ino))
    assert archive_extraction._ARCHIVE_GENERATION_REGISTRY == {}


def test_archive_signal_immediately_after_noreplace_cannot_leave_unaccepted_output(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"generation")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))

    def fake_run(_command, **options):
        (Path(options["cwd"]) / "out" / "inside.bin").write_bytes(b"generation")
        return ManagedProcessResult(returncode=0, stdout=b"")

    real_rename = archive_extraction._rename_archive_noreplace
    injected = False

    def rename_then_interrupt(source_fd, source_name, target_fd, target_name) -> None:
        nonlocal injected
        real_rename(source_fd, source_name, target_fd, target_name)
        if (
            not injected
            and source_name.startswith(".analytix-archive-placeholder-")
            and target_name == "out"
        ):
            injected = True
            raise KeyboardInterrupt

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    monkeypatch.setattr(
        archive_extraction,
        "_rename_archive_noreplace",
        rename_then_interrupt,
    )

    with pytest.raises(KeyboardInterrupt):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )

    assert injected is True
    assert not output_dir.exists()
    staged_outputs = _staged_attempt_outputs(tmp_path)
    assert len(staged_outputs) == 1
    assert (staged_outputs[0] / "inside.bin").read_bytes() == b"generation"
    assert archive_extraction._ARCHIVE_GENERATION_REGISTRY == {}


def test_archive_registry_commit_is_not_rolled_back_after_acceptance(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"accepted")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))

    def fake_run(_command, **options):
        (Path(options["cwd"]) / "out" / "inside.bin").write_bytes(b"accepted")
        return ManagedProcessResult(returncode=0, stdout=b"")

    real_register = archive_extraction._register_archive_generation

    def register_then_interrupt(*args, **kwargs) -> None:
        real_register(*args, **kwargs)
        raise KeyboardInterrupt

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    monkeypatch.setattr(
        archive_extraction,
        "_register_archive_generation",
        register_then_interrupt,
    )

    extract_archive(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=_sha256(archive_path),
        reuse=False,
    )

    assert (output_dir / "inside.bin").read_bytes() == b"accepted"
    assert len(archive_extraction._ARCHIVE_GENERATION_REGISTRY) == 1
    assert _staged_attempt_outputs(tmp_path) == []


def test_archive_registry_snapshot_interrupt_before_commit_is_all_or_nothing(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"unaccepted")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(
        archive_extraction,
        "archive_extractor_runtime",
        lambda: _runtime(runtime),
    )

    def fake_run(_command, **options):
        (Path(options["cwd"]) / "out" / "inside.bin").write_bytes(
            b"unaccepted"
        )
        return ManagedProcessResult(returncode=0, stdout=b"")

    real_install = archive_extraction._install_archive_generation_state
    injected = False

    def interrupt_before_registry_commit(state) -> None:
        nonlocal injected
        if state.registry and not injected:
            injected = True
            raise KeyboardInterrupt
        real_install(state)

    descriptors_before = len(os.listdir("/dev/fd"))
    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    monkeypatch.setattr(
        archive_extraction,
        "_install_archive_generation_state",
        interrupt_before_registry_commit,
    )

    with pytest.raises(KeyboardInterrupt):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
            reuse=False,
        )

    assert injected is True
    state = archive_extraction._ARCHIVE_GENERATION_STATE
    assert dict(state.registry) == {}
    assert dict(state.accepted_logical_paths) == {}
    assert dict(state.reservations) == {}
    assert dict(state.reservations_by_path) == {}
    assert dict(state.reservations_by_key) == {}
    assert not output_dir.exists()
    assert len(os.listdir("/dev/fd")) == descriptors_before


def test_archive_reopen_fd_is_closed_when_signal_arrives_after_open(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    root = tmp_path / "root"
    root.mkdir(mode=0o700)
    descriptor = os.open(root, os.O_RDONLY | os.O_DIRECTORY)
    real_fstat = os.fstat
    fstat_calls = 0

    def interrupt_second_fstat(candidate: int):
        nonlocal fstat_calls
        fstat_calls += 1
        if fstat_calls == 2:
            raise KeyboardInterrupt
        return real_fstat(candidate)

    descriptors_before = len(os.listdir("/dev/fd"))
    try:
        monkeypatch.setattr(archive_extraction.os, "fstat", interrupt_second_fstat)
        with pytest.raises(KeyboardInterrupt):
            archive_extraction._reopen_archive_directory_fd(descriptor)
    finally:
        monkeypatch.setattr(archive_extraction.os, "fstat", real_fstat)
        os.close(descriptor)

    assert fstat_calls == 2
    assert len(os.listdir("/dev/fd")) == descriptors_before - 1


def test_registered_generation_validation_interrupt_closes_duplicate_fd(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"registered")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(
        archive_extraction,
        "archive_extractor_runtime",
        lambda: _runtime(runtime),
    )

    def fake_run(_command, **options):
        (Path(options["cwd"]) / "out" / "inside.bin").write_bytes(
            b"registered"
        )
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    extract_archive(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=_sha256(archive_path),
        reuse=False,
    )
    descriptors_before = len(os.listdir("/dev/fd"))
    monkeypatch.setattr(
        archive_extraction,
        "_read_extract_marker_fd",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(KeyboardInterrupt()),
    )

    with pytest.raises(KeyboardInterrupt):
        archive_extraction._assert_archive_publication_ready(output_dir)

    assert len(os.listdir("/dev/fd")) == descriptors_before


def test_archive_attempt_creation_interrupt_closes_open_descriptor(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    output_dir = tmp_path / "cache" / "out"
    with archive_extraction._open_canonical_publish_parent(output_dir) as parent:
        descriptors_before = len(os.listdir("/dev/fd"))
        monkeypatch.setattr(
            archive_extraction,
            "_sync_archive_parent",
            lambda *_args, **_kwargs: (_ for _ in ()).throw(
                KeyboardInterrupt()
            ),
        )
        with pytest.raises(KeyboardInterrupt):
            archive_extraction._create_archive_attempt_directory(
                parent,
                name=".analytix-archive-attempt-interrupted",
            )
        assert len(os.listdir("/dev/fd")) == descriptors_before


def test_archive_member_validation_interrupt_closes_open_descriptors(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"member")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(
        archive_extraction,
        "archive_extractor_runtime",
        lambda: _runtime(runtime),
    )

    def fake_run(_command, **options):
        (Path(options["cwd"]) / "out" / "inside.bin").write_bytes(b"member")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    published = archive_extraction._extract_archive_generation(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=_sha256(archive_path),
        reuse=False,
    )
    member = next(
        entry
        for entry in published.summary.inventory
        if entry.path == "inside.bin"
    )
    entries = {entry.path: entry for entry in published.summary.inventory}
    real_stat = archive_extraction.os.stat
    injected = False

    def interrupt_leaf_stat(path, *args, **kwargs):
        nonlocal injected
        if path == "inside.bin" and kwargs.get("dir_fd") is not None:
            injected = True
            raise KeyboardInterrupt
        return real_stat(path, *args, **kwargs)

    descriptors_before = len(os.listdir("/dev/fd"))
    try:
        monkeypatch.setattr(archive_extraction.os, "stat", interrupt_leaf_stat)
        with pytest.raises(KeyboardInterrupt):
            archive_extraction._open_archive_member_fd(
                published.root_fd,
                member,
                entries,
            )
    finally:
        monkeypatch.setattr(archive_extraction.os, "stat", real_stat)
        os.close(published.root_fd)

    assert injected is True
    assert len(os.listdir("/dev/fd")) == descriptors_before - 1


def test_pinned_archive_parent_close_continues_after_interrupt(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    first = os.open(tmp_path, os.O_RDONLY | os.O_DIRECTORY)
    second = os.open(tmp_path, os.O_RDONLY | os.O_DIRECTORY)
    authority = archive_extraction._PinnedArchivePublishParent(
        path=tmp_path,
        output_name="out",
        descriptors=(first, second),
        edge_names=("edge",),
        identities=((1, 1), (2, 2)),
    )
    real_close = os.close
    interrupted = False

    def interrupt_once(descriptor: int) -> None:
        nonlocal interrupted
        if descriptor == second and not interrupted:
            interrupted = True
            raise KeyboardInterrupt
        real_close(descriptor)

    monkeypatch.setattr(archive_extraction.os, "close", interrupt_once)
    with pytest.raises(KeyboardInterrupt):
        authority.close()
    monkeypatch.setattr(archive_extraction.os, "close", real_close)

    assert interrupted is True
    for descriptor in (first, second):
        with pytest.raises(OSError):
            os.fstat(descriptor)
    assert authority.descriptors == ()


def test_archive_registry_state_close_continues_after_interrupt(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    first = os.open(tmp_path, os.O_RDONLY | os.O_DIRECTORY)
    second = os.open(tmp_path, os.O_RDONLY | os.O_DIRECTORY)
    source = archive_extraction._ArchiveSourceIdentity(
        device=1,
        inode=1,
        size=0,
        mtime_ns=0,
        ctime_ns=0,
        sha256="0" * 64,
    )
    summary = archive_extraction._ArchiveTreeSummary(
        entry_count=0,
        file_count=0,
        total_bytes=0,
        sha256="0" * 64,
        inventory=(),
    )

    def record(descriptor: int):
        return archive_extraction._RegisteredArchiveGenerationV1(
            publication_nonce="nonce",
            source_identity=source,
            root_identity=(1, descriptor),
            root_state=(),
            summary=summary,
            root_fd=descriptor,
        )

    state = archive_extraction._new_archive_generation_registry_state(
        process_id=os.getpid(),
        registry={
            (((1, 1),), "first"): record(first),
            (((2, 2),), "second"): record(second),
        },
    )
    real_close = os.close
    interrupted = False

    def interrupt_once(descriptor: int) -> None:
        nonlocal interrupted
        if descriptor == first and not interrupted:
            interrupted = True
            raise KeyboardInterrupt
        real_close(descriptor)

    monkeypatch.setattr(archive_extraction.os, "close", interrupt_once)
    with pytest.raises(KeyboardInterrupt):
        archive_extraction._close_archive_generation_registry_state(state)
    monkeypatch.setattr(archive_extraction.os, "close", real_close)

    assert interrupted is True
    for descriptor in (first, second):
        with pytest.raises(OSError):
            os.fstat(descriptor)


def test_archive_registry_snapshot_interrupt_after_commit_is_fully_accepted(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"accepted")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(
        archive_extraction,
        "archive_extractor_runtime",
        lambda: _runtime(runtime),
    )

    def fake_run(_command, **options):
        (Path(options["cwd"]) / "out" / "inside.bin").write_bytes(b"accepted")
        return ManagedProcessResult(returncode=0, stdout=b"")

    real_install = archive_extraction._install_archive_generation_state
    injected = False

    def interrupt_after_registry_commit(state) -> None:
        nonlocal injected
        real_install(state)
        if state.registry and not injected:
            injected = True
            raise KeyboardInterrupt

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    monkeypatch.setattr(
        archive_extraction,
        "_install_archive_generation_state",
        interrupt_after_registry_commit,
    )

    extract_archive(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=_sha256(archive_path),
        reuse=False,
    )

    assert injected is True
    state = archive_extraction._ARCHIVE_GENERATION_STATE
    assert len(state.registry) == 1
    assert len(state.accepted_logical_paths) == 1
    assert dict(state.reservations) == {}
    assert dict(state.reservations_by_path) == {}
    assert dict(state.reservations_by_key) == {}
    assert (output_dir / "inside.bin").read_bytes() == b"accepted"


def test_archive_registry_reset_publishes_empty_snapshot_before_fd_close(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"accepted")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(
        archive_extraction,
        "archive_extractor_runtime",
        lambda: _runtime(runtime),
    )

    def fake_run(_command, **options):
        (Path(options["cwd"]) / "out" / "inside.bin").write_bytes(b"accepted")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    extract_archive(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=_sha256(archive_path),
        reuse=False,
    )
    record_fd = next(
        iter(archive_extraction._ARCHIVE_GENERATION_STATE.registry.values())
    ).root_fd
    real_close = os.close
    observed_empty_before_close: list[bool] = []

    def observe_close(descriptor: int) -> None:
        if descriptor == record_fd:
            observed_empty_before_close.append(
                len(archive_extraction._ARCHIVE_GENERATION_STATE.registry) == 0
            )
        real_close(descriptor)

    with monkeypatch.context() as patch_context:
        patch_context.setattr(archive_extraction.os, "close", observe_close)
        archive_extraction._reset_archive_generation_registry_after_fork()

    assert observed_empty_before_close == [True]
    assert archive_extraction._ARCHIVE_GENERATION_REGISTRY == {}


def test_live_registry_missing_visible_root_never_reissues_generation(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"accepted")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    launches = 0

    def fake_run(_command, **options):
        nonlocal launches
        launches += 1
        work = Path(options["cwd"])
        (work / "out" / "inside.bin").write_bytes(b"accepted")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    extract_archive(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=_sha256(archive_path),
    )
    with archive_extraction._open_canonical_publish_parent(output_dir) as parent:
        registry_key = archive_extraction._archive_generation_registry_key(parent)
    record = archive_extraction._ARCHIVE_GENERATION_REGISTRY[registry_key]
    record_identity = archive_extraction._identity(os.fstat(record.root_fd))
    displaced = tmp_path / "out-displaced"
    output_dir.rename(displaced)

    with pytest.raises(ExternalRuntimeError, match="^archive_publish_indeterminate$"):
        extract_archive(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
        )

    assert launches == 1
    assert not output_dir.exists()
    assert (displaced / "inside.bin").read_bytes() == b"accepted"
    assert archive_extraction._ARCHIVE_GENERATION_REGISTRY[registry_key] is record
    assert archive_extraction._identity(os.fstat(record.root_fd)) == record_identity


def test_marker_mutation_during_lease_invalidates_receipt(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"accepted")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))

    def fake_run(_command, **options):
        work = Path(options["cwd"])
        (work / "out" / "inside.bin").write_bytes(b"accepted")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    with pytest.raises(
        ExternalRuntimeError,
        match="^archive_inventory_changed_during_use$",
    ):
        with archive_extraction.open_archive_extraction_v1(
            archive_path,
            output_dir,
            password=None,
            expected_sha256=_sha256(archive_path),
        ) as lease:
            marker_path = output_dir / archive_extraction._MARKER_FILE
            marker = marker_path.read_bytes()
            original_nonce = lease.receipt.publication_nonce.encode("ascii")
            replacement = (
                (b"0" if original_nonce[:1] != b"0" else b"1")
                + original_nonce[1:]
            )
            mutated = marker.replace(original_nonce, replacement, 1)
            assert len(mutated) == len(marker)
            with marker_path.open("r+b", buffering=0) as handle:
                handle.write(mutated)
                os.fsync(handle.fileno())


def test_registered_generation_fd_is_closed_on_source_probe_failure(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    archive_path = tmp_path / "sample.7z"
    archive_path.write_bytes(b"accepted")
    output_dir = tmp_path / "out"
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))

    def fake_run(_command, **options):
        work = Path(options["cwd"])
        (work / "out" / "inside.bin").write_bytes(b"accepted")
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    extract_archive(
        archive_path,
        output_dir,
        password=None,
        expected_sha256=_sha256(archive_path),
    )
    descriptors_before = len(os.listdir("/dev/fd"))
    monkeypatch.setattr(
        archive_extraction,
        "_read_regular_archive_source",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(
            ExternalRuntimeError("archive_source_unavailable")
        ),
    )

    for _ in range(20):
        with pytest.raises(
            ExternalRuntimeError,
            match="^archive_source_unavailable$",
        ):
            extract_archive(
                archive_path,
                output_dir,
                password=None,
                expected_sha256=_sha256(archive_path),
            )

    assert len(os.listdir("/dev/fd")) == descriptors_before


@pytest.mark.skipif(os.name == "nt", reason="POSIX flock authority")
def test_archive_publication_lock_has_bounded_wait(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    parent_path = tmp_path / "cache"
    parent_path.mkdir(mode=0o700)
    ready_read, ready_write = os.pipe()
    release_read, release_write = os.pipe()
    child = os.fork()
    if child == 0:
        try:
            os.close(ready_read)
            os.close(release_write)
            import fcntl

            descriptor = os.open(parent_path, os.O_RDONLY | os.O_DIRECTORY)
            fcntl.flock(descriptor, fcntl.LOCK_EX)
            os.write(ready_write, b"1")
            os.read(release_read, 1)
            os.close(descriptor)
            os._exit(0)
        except BaseException:
            os._exit(1)

    os.close(ready_write)
    os.close(release_read)
    try:
        readable, _, _ = select.select([ready_read], [], [], 5)
        assert readable == [ready_read]
        assert os.read(ready_read, 1) == b"1"
        monkeypatch.setattr(
            archive_extraction,
            "_ARCHIVE_PUBLICATION_LOCK_TIMEOUT_SECONDS",
            0.05,
        )
        started = archive_extraction.time.monotonic()
        with archive_extraction._open_canonical_publish_parent(
            parent_path / "out"
        ) as parent:
            with pytest.raises(
                ExternalRuntimeError,
                match="^archive_publish_lock_timeout$",
            ):
                with archive_extraction._archive_publication_lock(parent):
                    raise AssertionError("contended lock must not be acquired")
        assert archive_extraction.time.monotonic() - started < 1
    finally:
        os.write(release_write, b"1")
        os.close(release_write)
        os.close(ready_read)
        _, status = os.waitpid(child, 0)
        assert os.waitstatus_to_exitcode(status) == 0


def test_slow_archive_does_not_block_unrelated_case_admission(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    runtime = _fake_runtime(tmp_path)
    monkeypatch.setattr(archive_extraction, "archive_extractor_runtime", lambda: _runtime(runtime))
    first_started = threading.Event()
    release_first = threading.Event()
    second_finished = threading.Event()
    errors: list[BaseException] = []

    def fake_run(_command, **options):
        work = Path(options["cwd"])
        if "case-a" in work.parts:
            first_started.set()
            assert release_first.wait(timeout=5)
        (work / "out" / "inside.bin").write_bytes((work / "input.7z").read_bytes())
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    first_source = tmp_path / "first.7z"
    second_source = tmp_path / "second.7z"
    first_source.write_bytes(b"first")
    second_source.write_bytes(b"second")

    def extract_one(source: Path, output: Path, finished: threading.Event | None = None) -> None:
        try:
            extract_archive(
                source,
                output,
                password=None,
                expected_sha256=_sha256(source),
            )
            if finished is not None:
                finished.set()
        except BaseException as error:
            errors.append(error)

    first_thread = threading.Thread(
        target=extract_one,
        args=(first_source, tmp_path / "case-a" / "out"),
    )
    second_thread = threading.Thread(
        target=extract_one,
        args=(second_source, tmp_path / "case-b" / "out", second_finished),
    )
    first_thread.start()
    assert first_started.wait(timeout=5)
    second_thread.start()
    assert second_finished.wait(timeout=5)
    release_first.set()
    first_thread.join(timeout=5)
    second_thread.join(timeout=5)

    assert not first_thread.is_alive()
    assert not second_thread.is_alive()
    assert errors == []


def test_archive_publication_and_consumption_have_no_path_fallback() -> None:
    source = inspect.getsource(archive_extraction)
    tree = ast.parse(source)
    violations: list[str] = []

    class BoundaryVisitor(ast.NodeVisitor):
        def __init__(self) -> None:
            self.functions: list[str] = []

        def visit_FunctionDef(self, node: ast.FunctionDef) -> None:
            self.functions.append(node.name)
            self.generic_visit(node)
            self.functions.pop()

        def visit_Call(self, node: ast.Call) -> None:
            owner = self.functions[-1] if self.functions else "<module>"
            if isinstance(node.func, ast.Attribute) and isinstance(node.func.value, ast.Name):
                qualified = f"{node.func.value.id}.{node.func.attr}"
                if qualified == "os.rmdir" and any(
                    keyword.arg == "dir_fd" for keyword in node.keywords
                ):
                    self.generic_visit(node)
                    return
                if qualified in {
                    "os.replace",
                    "os.rename",
                    "os.unlink",
                    "os.rmdir",
                    "shutil.rmtree",
                    "shutil.move",
                }:
                    violations.append(f"{owner}: {qualified}")
                if qualified in {
                    "tempfile.TemporaryDirectory",
                    "tempfile.mkdtemp",
                    "Path.rglob",
                }:
                    violations.append(f"{owner}: {qualified}")
            self.generic_visit(node)

    BoundaryVisitor().visit(tree)
    assert violations == []
    assert "_replace_archive_sibling" not in source
    assert "_remove_archive_publication_tree" not in source
    assert "_clear_archive_publication_directory_fd" not in source
    assert "_create_archive_output_directory" not in source
    assert "iter_extracted_files" not in source

    repository_source = (
        Path(__file__).resolve().parents[1] / "app" / "repositories" / "import_repository.py"
    ).read_text(encoding="utf-8")
    for forbidden in ("iter_extracted_files", ".rglob(", "os.walk(", "os.scandir("):
        assert forbidden not in repository_source
