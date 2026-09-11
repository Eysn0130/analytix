from __future__ import annotations

import hashlib
import json
import os
import stat
import subprocess
import sys
import zipfile
from concurrent.futures import ThreadPoolExecutor
from contextlib import contextmanager
from pathlib import Path

import pytest

from app.core import archive_extraction, immutable_generation
from app.core.external_runtime import ExternalRuntimeError, HostRuntimeResolution
from app.core.execution_authority_health import ExecutionAuthorityHealth
from app.core.immutable_generation import (
    ImmutableGenerationError,
    ensure_private_generation_root,
    inspect_regular_source,
    publish_bytes,
    publish_generated,
    publish_source,
    private_work_directory,
    read_immutable_bytes,
)
from app.domain.import_service import ImportService
from app.core.managed_subprocess import ManagedProcessResult
from app.repositories import import_repository as import_repository_module
from app.repositories.import_repository import (
    ImportArchiveItemInput,
    ImportRepository,
    PreparedImportFile,
)
from app.schemas.import_jobs import ImportFileSpec


def _write_private(path: Path, payload: bytes) -> Path:
    path.write_bytes(payload)
    path.chmod(0o600)
    return path


class _CaseStorage:
    def __init__(self, root: Path) -> None:
        self.root = root

    def case_dir(self, case_id: str) -> Path:
        assert case_id == "case-a"
        return self.root


def _admit_fake_archive_extraction(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
    entries: dict[str, bytes],
) -> None:
    runtime = _write_private(tmp_path / "7zz", b"test-runtime")
    runtime.chmod(0o700)
    monkeypatch.setattr(
        archive_extraction,
        "_archive_execution_capability_health",
        lambda: ExecutionAuthorityHealth(available=True, reason_code=""),
    )
    monkeypatch.setattr(
        archive_extraction,
        "archive_extractor_runtime",
        lambda: HostRuntimeResolution(
            runtime,
            "",
            hashlib.sha256(runtime.read_bytes()).hexdigest(),
        ),
    )

    def fake_run(_command, **options):
        output = Path(options["cwd"]) / "out"
        for relative, payload in entries.items():
            target = output / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(payload)
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)


def test_import_service_carries_expected_size_through_shared_metadata_builder() -> None:
    spec = ImportFileSpec(
        file_name="source.csv",
        source_path="/controlled/source.csv",
        expected_sha256="A" * 64,
        expected_size=123,
    )

    built = ImportService._import_file_input_from_metadata(
        spec.model_dump(),
        password=None,
    )

    assert built.expected_sha256 == "A" * 64
    assert built.expected_size == 123


def test_import_service_carries_archive_child_size_and_rejects_duplicate_identity() -> None:
    built = ImportService._import_file_input_from_metadata(
        {
            "file_name": "source.7z",
            "source_path": "/controlled/source.7z",
            "expected_sha256": "A" * 64,
            "expected_size": 321,
            "archive_items": [
                {
                    "archive_path": "outer.zip::流水.csv",
                    "expected_sha256": "B" * 64,
                    "expected_size": 456,
                }
            ],
        },
        password=None,
    )

    assert built.archive_items is not None
    assert built.archive_items[0].expected_size == 456
    duplicate = ImportArchiveItemInput(
        archive_path="outer.zip::流水.csv",
        expected_sha256="B" * 64,
        expected_size=456,
    )
    with pytest.raises(ValueError, match="^duplicate archive entry override:"):
        import_repository_module._archive_override_map([duplicate, duplicate])


def test_raw_generation_is_full_sha256_content_addressed_and_idempotent(tmp_path: Path) -> None:
    source = _write_private(tmp_path / "用户显示名.csv", b"account,amount\n62220001,12.50\n")
    receipt = inspect_regular_source(source)
    raw_root = tmp_path / "raw"

    first = ImportRepository._copy_to_raw(
        source,
        raw_root,
        verified_source=receipt,
        encrypted=False,
    )
    second = ImportRepository._copy_to_raw(
        source,
        raw_root,
        verified_source=receipt,
        encrypted=False,
    )

    assert first.path.name == f"{receipt.sha256.lower()}.csv"
    assert first.path.read_bytes() == source.read_bytes()
    assert stat.S_IMODE(first.path.stat().st_mode) == 0o600
    assert first.reused is False
    assert second.path == first.path
    assert second.reused is True
    assert second.receipt()["lineage"] == {
        "kind": "raw_source",
        "encrypted_source": False,
        "derived": False,
        "source_sha256": receipt.sha256,
    }


def test_data_prepare_preserves_display_name_but_consumes_internal_generation_receipt(tmp_path: Path) -> None:
    case_root = tmp_path / "case-a"
    case_root.mkdir(mode=0o700)
    ensure_private_generation_root(case_root / "raw")
    source = _write_private(tmp_path / "银行流水原名.csv", "交易账号,交易时间,交易金额\n6222,2026-01-01,10\n".encode())
    source_receipt = inspect_regular_source(source)
    repository = object.__new__(ImportRepository)
    repository._storage = _CaseStorage(case_root)

    prepared = repository._prepare_from_data_file(
        "case-a",
        source,
        display_name="银行流水原名.csv",
        source_label="受控来源/银行流水原名.csv",
        kind_hint="fc_transaction",
        password=None,
        field_mapping=None,
        verified_sha256=source_receipt.sha256,
        verified_source=source_receipt,
    )

    assert len(prepared) == 1
    item = prepared[0]
    assert item.display_name == "银行流水原名.csv"
    assert item.display_path == "受控来源/银行流水原名.csv"
    assert item.real_path.name == f"{source_receipt.sha256.lower()}.csv"
    assert item.real_path != source
    assert item.ingestion_receipt["source"]["sha256"] == source_receipt.sha256
    repository._bind_prepared_items_to_case(
        case_id="case-a",
        authority_root=case_root / "raw",
        items=prepared,
    )
    repository._validate_prepared_generation(item.real_path, item.ingestion_receipt)
    item.size += 1
    with pytest.raises(ImmutableGenerationError, match="size_mismatch"):
        repository._validate_prepared_item("case-a", item)


def test_authorized_prepare_requires_independent_hash_and_size_before_root_creation(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    case_root = tmp_path / "case-a"
    case_root.mkdir(mode=0o700)
    source = _write_private(tmp_path / "source.csv", b"account,amount\n1,2\n")
    source_receipt = inspect_regular_source(source)
    repository = object.__new__(ImportRepository)
    repository._storage = _CaseStorage(case_root)
    monkeypatch.setattr(import_repository_module, "require_controlled_source_ingestion", lambda: None)

    with pytest.raises(ValueError, match="SHA-256 and size"):
        repository.prepare_files(
            "case-a",
            [import_repository_module.ImportFileInput(file_name="source.csv", source_path=str(source))],
        )
    assert not (case_root / "raw").exists()

    with pytest.raises(ValueError, match="预检后已发生变更"):
        repository.prepare_files(
            "case-a",
            [
                import_repository_module.ImportFileInput(
                    file_name="source.csv",
                    source_path=str(source),
                    expected_sha256=source_receipt.sha256,
                    expected_size=source_receipt.size + 1,
                )
            ],
        )
    assert not (case_root / "raw").exists()


def test_authorized_prepare_binds_case_registry_and_expected_size_end_to_end(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    case_root = tmp_path / "case-a"
    case_root.mkdir(mode=0o700)
    source = _write_private(
        tmp_path / "source.csv",
        "交易账号,交易时间,交易金额\n6222,2026-01-01,10\n".encode("utf-8"),
    )
    source_receipt = inspect_regular_source(source)
    repository = object.__new__(ImportRepository)
    repository._storage = _CaseStorage(case_root)
    monkeypatch.setattr(import_repository_module, "require_controlled_source_ingestion", lambda: None)

    prepared = repository.prepare_files(
        "case-a",
        [
            import_repository_module.ImportFileInput(
                file_name="source.csv",
                source_path=str(source),
                file_kind="fc_transaction",
                expected_sha256=source_receipt.sha256,
                expected_size=source_receipt.size,
            )
        ],
    )

    assert len(prepared) == 1
    item = prepared[0]
    assert item.ingestion_receipt["schema_version"] == 2
    assert item.ingestion_receipt["case_id"] == "case-a"
    assert item.ingestion_receipt["authority_relative_path"] == item.real_path.relative_to(case_root / "raw").as_posix()
    repository._validate_prepared_item("case-a", item)


def test_zip_prepare_uses_verified_archive_fd_and_immutable_entry_generation(tmp_path: Path) -> None:
    case_root = tmp_path / "case-a"
    case_root.mkdir(mode=0o700)
    ensure_private_generation_root(case_root / "raw")
    archive = tmp_path / "流水.zip"
    member_payload = "交易账号,交易时间,交易金额\n6222,2026-01-01,10\n".encode()
    with zipfile.ZipFile(archive, "w") as output:
        output.writestr("目录/流水.csv", member_payload)
    archive.chmod(0o600)
    archive_receipt = inspect_regular_source(archive)
    repository = object.__new__(ImportRepository)
    repository._storage = _CaseStorage(case_root)

    prepared = repository._prepare_from_zip(
        "case-a",
        archive,
        file_name="流水.zip",
        kind_hint="fc_transaction",
        password=None,
        field_mapping=None,
        archive_items=[
            ImportArchiveItemInput(
                archive_path="目录/流水.csv",
                expected_sha256=hashlib.sha256(member_payload).hexdigest(),
                expected_size=len(member_payload),
            )
        ],
        verified_archive_sha256=archive_receipt.sha256,
        verified_source=archive_receipt,
    )

    assert len(prepared) == 1
    item = prepared[0]
    assert item.display_name == "流水.csv"
    assert item.real_path.name == f"{hashlib.sha256(item.real_path.read_bytes()).hexdigest()}.csv"
    assert item.ingestion_receipt["lineage"]["derivation"] == "zip_archive_entry"
    assert item.ingestion_receipt["lineage"]["source_entry"] == "目录/流水.csv"
    repository._validate_prepared_generation(item.real_path, item.ingestion_receipt)


def test_prepare_archive_late_csv_is_never_prepared(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    case_root = tmp_path / "case-a"
    case_root.mkdir(mode=0o700)
    ensure_private_generation_root(case_root / "raw")
    archive = _write_private(tmp_path / "流水.7z", b"opaque-archive")
    archive_receipt = inspect_regular_source(archive)
    runtime = _write_private(tmp_path / "7zz", b"test-runtime")
    runtime.chmod(0o700)
    monkeypatch.setattr(
        archive_extraction,
        "_archive_execution_capability_health",
        lambda: ExecutionAuthorityHealth(available=True, reason_code=""),
    )
    monkeypatch.setattr(
        archive_extraction,
        "archive_extractor_runtime",
        lambda: HostRuntimeResolution(runtime, "", hashlib.sha256(runtime.read_bytes()).hexdigest()),
    )

    def fake_run(_command, **options):
        (Path(options["cwd"]) / "out" / "accepted.csv").write_bytes(
            b"account,amount\n6222020202020202020,10\n"
        )
        return ManagedProcessResult(returncode=0, stdout=b"")

    monkeypatch.setattr(archive_extraction, "run_managed_process", fake_run)
    real_open = import_repository_module.open_archive_extraction_v1

    @contextmanager
    def inject_late_member(*args, **kwargs):
        output_dir = Path(args[1])
        with real_open(*args, **kwargs) as lease:
            late = output_dir / "late.csv"
            late.write_bytes(b"fabricated,amount\nX,999\n")
            try:
                yield lease
            finally:
                late.unlink()

    monkeypatch.setattr(
        import_repository_module,
        "open_archive_extraction_v1",
        inject_late_member,
    )
    repository = object.__new__(ImportRepository)
    repository._storage = _CaseStorage(case_root)
    monkeypatch.setattr(repository, "_prepare_import_csv_metadata_batch", lambda *_args, **_kwargs: None)
    accepted_payload = b"account,amount\n6222020202020202020,10\n"
    archive_items = [
        ImportArchiveItemInput(
            archive_path="accepted.csv",
            expected_sha256=hashlib.sha256(accepted_payload).hexdigest(),
            expected_size=len(accepted_payload),
        )
    ]

    with pytest.raises(ExternalRuntimeError, match="^archive_inventory_changed_during_use$"):
        repository._prepare_from_archive(
            "case-a",
            archive,
            file_name="流水.7z",
            kind_hint="fc_transaction",
            password=None,
            field_mapping=None,
            archive_items=archive_items,
            verified_archive_sha256=archive_receipt.sha256,
            verified_source=archive_receipt,
        )

    member_root = next((case_root / "raw" / "_archivecache").glob("*/_inventory_quarantine"))
    payloads = [path.read_bytes() for path in member_root.glob("*.csv")]
    assert payloads == [b"account,amount\n6222020202020202020,10\n"]
    assert all(b"fabricated" not in payload for payload in payloads)


def test_external_archive_preserves_root_source_and_consumed_identity(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    case_root = tmp_path / "case-a"
    case_root.mkdir(mode=0o700)
    ensure_private_generation_root(case_root / "raw")
    archive = _write_private(
        tmp_path / "accounts.7z",
        b"opaque-root-archive-account-6222020202020202020",
    )
    archive_receipt = inspect_regular_source(archive)
    member_payload = b"account,amount\n6222020202020202020,10\n"
    _admit_fake_archive_extraction(
        monkeypatch,
        tmp_path,
        {"accepted.csv": member_payload},
    )
    repository = object.__new__(ImportRepository)
    repository._storage = _CaseStorage(case_root)
    monkeypatch.setattr(
        repository,
        "_prepare_import_csv_metadata_batch",
        lambda *_args, **_kwargs: None,
    )

    prepared = repository._prepare_from_archive(
        "case-a",
        archive,
        file_name="accounts.7z",
        kind_hint="fc_transaction",
        password=None,
        field_mapping=None,
        archive_items=[
            ImportArchiveItemInput(
                archive_path="accepted.csv",
                expected_sha256=hashlib.sha256(member_payload).hexdigest(),
                expected_size=len(member_payload),
            )
        ],
        verified_archive_sha256=archive_receipt.sha256,
        verified_source=archive_receipt,
    )

    assert len(prepared) == 1
    receipt = prepared[0].ingestion_receipt
    assert receipt["source"] == archive_receipt.as_dict()
    assert receipt["sha256"] == hashlib.sha256(member_payload).hexdigest().upper()
    assert receipt["lineage"]["root_source_sha256"] == archive_receipt.sha256
    assert receipt["lineage"]["source_entry"] == "accepted.csv"
    assert receipt["lineage"]["source_generation_lineage"][
        "archive_member_path"
    ] == "accepted.csv"


def test_external_archive_override_set_is_checked_before_leaf_publish(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    case_root = tmp_path / "case-a"
    case_root.mkdir(mode=0o700)
    ensure_private_generation_root(case_root / "raw")
    archive = _write_private(tmp_path / "accounts.7z", b"opaque")
    archive_receipt = inspect_regular_source(archive)
    member_payload = b"account,amount\n1,2\n"
    _admit_fake_archive_extraction(
        monkeypatch,
        tmp_path,
        {"accepted.csv": member_payload},
    )
    published_members = 0
    real_publish = archive_extraction.ArchiveExtractionLeaseV1.publish_member

    def counted_publish(self, *args, **kwargs):
        nonlocal published_members
        published_members += 1
        return real_publish(self, *args, **kwargs)

    monkeypatch.setattr(
        archive_extraction.ArchiveExtractionLeaseV1,
        "publish_member",
        counted_publish,
    )
    repository = object.__new__(ImportRepository)
    repository._storage = _CaseStorage(case_root)

    with pytest.raises(
        ValueError,
        match="^archive entry override does not match extracted inventory$",
    ):
        repository._prepare_from_archive(
            "case-a",
            archive,
            file_name="accounts.7z",
            kind_hint="fc_transaction",
            password=None,
            field_mapping=None,
            archive_items=[
                ImportArchiveItemInput(
                    archive_path="accepted.csv",
                    expected_sha256=hashlib.sha256(member_payload).hexdigest(),
                    expected_size=len(member_payload),
                ),
                ImportArchiveItemInput(
                    archive_path="extra.csv",
                    expected_sha256="A" * 64,
                    expected_size=1,
                ),
            ],
            verified_archive_sha256=archive_receipt.sha256,
            verified_source=archive_receipt,
        )

    assert published_members == 0
    assert not list(
        (case_root / "raw" / "_archivecache").glob(
            "*/_inventory_quarantine/*.csv"
        )
    )


def test_equal_size_different_sha_is_rejected_before_case_admission(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    case_root = tmp_path / "case-a"
    case_root.mkdir(mode=0o700)
    ensure_private_generation_root(case_root / "raw")
    archive = _write_private(tmp_path / "accounts.7z", b"opaque")
    archive_receipt = inspect_regular_source(archive)
    member_payload = b"account,amount\n1,2\n"
    _admit_fake_archive_extraction(
        monkeypatch,
        tmp_path,
        {"accepted.csv": member_payload},
    )
    repository = object.__new__(ImportRepository)
    repository._storage = _CaseStorage(case_root)

    with pytest.raises(
        ValueError,
        match="^archive entry receipt mismatch: accepted.csv$",
    ):
        repository._prepare_from_archive(
            "case-a",
            archive,
            file_name="accounts.7z",
            kind_hint="fc_transaction",
            password=None,
            field_mapping=None,
            archive_items=[
                ImportArchiveItemInput(
                    archive_path="accepted.csv",
                    expected_sha256="F" * 64,
                    expected_size=len(member_payload),
                )
            ],
            verified_archive_sha256=archive_receipt.sha256,
            verified_source=archive_receipt,
        )

    assert not getattr(repository, "_prepared_generation_registry", {})


def test_duplicate_zip_entry_is_rejected_before_raw_copy(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    case_root = tmp_path / "case-a"
    case_root.mkdir(mode=0o700)
    ensure_private_generation_root(case_root / "raw")
    archive = tmp_path / "duplicate.zip"
    payload = b"account,amount\n1,2\n"
    with pytest.warns(UserWarning, match="Duplicate name"):
        with zipfile.ZipFile(archive, "w") as output:
            output.writestr("accepted.csv", payload)
            output.writestr("accepted.csv", payload)
    archive.chmod(0o600)
    archive_receipt = inspect_regular_source(archive)
    repository = object.__new__(ImportRepository)
    repository._storage = _CaseStorage(case_root)
    copied = False

    def must_not_copy(*_args, **_kwargs):
        nonlocal copied
        copied = True
        raise AssertionError("duplicate inventory must fail before raw copy")

    monkeypatch.setattr(repository, "_copy_to_raw", must_not_copy)
    with pytest.raises(
        ValueError,
        match="^duplicate archive entry identity: accepted.csv$",
    ):
        repository._prepare_from_zip(
            "case-a",
            archive,
            file_name="duplicate.zip",
            kind_hint="fc_transaction",
            password=None,
            field_mapping=None,
            archive_items=[
                ImportArchiveItemInput(
                    archive_path="accepted.csv",
                    expected_sha256=hashlib.sha256(payload).hexdigest(),
                    expected_size=len(payload),
                )
            ],
            verified_archive_sha256=archive_receipt.sha256,
            verified_source=archive_receipt,
        )
    assert copied is False


def test_existing_owner_root_is_tightened_without_touching_prior_generation(tmp_path: Path) -> None:
    root = tmp_path / "raw"
    root.mkdir(mode=0o755)
    prior = _write_private(root / "prior.bin", b"prior-accepted")

    assert ensure_private_generation_root(root) == root

    assert stat.S_IMODE(root.stat().st_mode) == 0o700
    assert prior.read_bytes() == b"prior-accepted"
    assert prior.stat().st_ino == (root / "prior.bin").stat().st_ino


def test_source_identity_and_expected_size_are_rechecked_at_copy_time(tmp_path: Path) -> None:
    source = _write_private(tmp_path / "source.csv", b"old")
    receipt = inspect_regular_source(source)
    original = tmp_path / "original.csv"
    source.rename(original)
    _write_private(source, b"new")

    with pytest.raises(ImmutableGenerationError, match="source_identity_mismatch"):
        publish_source(source, tmp_path / "raw", expected=receipt)
    assert list((tmp_path / "raw").glob("[!.]*")) == []

    with pytest.raises(ImmutableGenerationError, match="source_size_mismatch"):
        inspect_regular_source(original, expected_size=receipt.size + 1)


def test_source_symlink_and_hardlink_are_rejected_before_any_root_write(tmp_path: Path) -> None:
    source = _write_private(tmp_path / "source.csv", b"source")
    symlink = tmp_path / "source-link.csv"
    symlink.symlink_to(source)
    with pytest.raises(ImmutableGenerationError, match="source_open_failed"):
        inspect_regular_source(symlink)

    hardlink = tmp_path / "source-hardlink.csv"
    os.link(source, hardlink)
    with pytest.raises(ImmutableGenerationError, match="hardlink_rejected"):
        inspect_regular_source(source)
    assert not (tmp_path / "raw").exists()


def test_execution_time_copy_hash_mismatch_never_publishes_generation(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    source = _write_private(tmp_path / "source.csv", b"authoritative")
    receipt = inspect_regular_source(source)

    def corrupted_copy(_path, _receipt, output) -> None:
        output.write(b"fabricated")

    monkeypatch.setattr(immutable_generation, "_copy_source_to_output", corrupted_copy)
    with pytest.raises(ImmutableGenerationError, match="generated_(hash|size)_mismatch"):
        publish_source(source, tmp_path / "raw", expected=receipt)
    assert not list((tmp_path / "raw").glob("[!.]*"))
    assert not list((tmp_path / "raw").glob(".pending-v1-*"))


def test_symlink_and_external_hardlink_targets_fail_closed(tmp_path: Path) -> None:
    source = _write_private(tmp_path / "source.csv", b"same-content")
    receipt = inspect_regular_source(source)
    root = ensure_private_generation_root(tmp_path / "raw")
    target = root / f"{receipt.sha256.lower()}.csv"
    outside = _write_private(tmp_path / "outside", b"same-content")

    target.symlink_to(outside)
    with pytest.raises((ImmutableGenerationError, OSError)):
        publish_source(source, root, expected=receipt)
    assert target.is_symlink()
    target.unlink()

    os.link(outside, target)
    with pytest.raises(ImmutableGenerationError, match="external_hardlink_rejected"):
        publish_source(source, root, expected=receipt)
    assert target.stat().st_ino == outside.stat().st_ino
    assert target.stat().st_nlink == 2


def test_target_creation_race_is_never_overwritten(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    source = _write_private(tmp_path / "source.csv", b"authoritative")
    receipt = inspect_regular_source(source)
    root = ensure_private_generation_root(tmp_path / "raw")
    target_name = f"{receipt.sha256.lower()}.csv"
    attacker = _write_private(tmp_path / "attacker", b"attacker")

    def race_link(_source_name, destination_name, *, src_dir_fd, dst_dir_fd, follow_symlinks) -> None:
        assert destination_name == target_name
        os.symlink(attacker, destination_name, dir_fd=dst_dir_fd)
        raise FileExistsError(destination_name)

    monkeypatch.setattr(immutable_generation.os, "link", race_link)
    with pytest.raises((ImmutableGenerationError, OSError)):
        publish_source(source, root, expected=receipt)
    target = root / target_name
    assert target.is_symlink()
    assert target.resolve() == attacker
    assert not list(root.glob(".pending-v1-*"))


def test_root_replacement_during_publish_fails_and_removes_unaccepted_target(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    source = _write_private(tmp_path / "source.csv", b"root-race")
    receipt = inspect_regular_source(source)
    root = ensure_private_generation_root(tmp_path / "raw")
    displaced = tmp_path / "raw-displaced"
    real_link = immutable_generation.os.link

    def swap_root_after_link(*args, **kwargs) -> None:
        real_link(*args, **kwargs)
        root.rename(displaced)
        root.mkdir(mode=0o700)

    monkeypatch.setattr(immutable_generation.os, "link", swap_root_after_link)
    with pytest.raises(ImmutableGenerationError, match="root_identity_mismatch"):
        publish_source(source, root, expected=receipt)
    target_name = f"{receipt.sha256.lower()}.csv"
    assert not (root / target_name).exists()
    assert not (displaced / target_name).exists()
    assert not list(displaced.glob(".pending-v1-*"))


def test_concurrent_publishers_share_one_exact_generation_without_race_overwrite(tmp_path: Path) -> None:
    source = _write_private(tmp_path / "source.csv", b"concurrent-generation")
    receipt = inspect_regular_source(source)
    root = tmp_path / "raw"

    with ThreadPoolExecutor(max_workers=8) as pool:
        generations = list(pool.map(lambda _index: publish_source(source, root, expected=receipt), range(24)))

    assert len({generation.path for generation in generations}) == 1
    target = generations[0].path
    assert target.read_bytes() == b"concurrent-generation"
    assert target.stat().st_nlink == 1
    assert sum(not generation.reused for generation in generations) == 1
    assert not list(root.glob(".pending-v1-*"))


def test_failure_and_fsync_failure_never_replace_prior_generation(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    root = tmp_path / "manifests"
    prior = publish_bytes(root, b"accepted", suffix=".json", target_name="head-v1.json")

    with pytest.raises(ImmutableGenerationError, match="existing_target_mismatch"):
        publish_bytes(root, b"replacement", suffix=".json", target_name="head-v1.json")
    assert read_immutable_bytes(prior.path) == b"accepted"

    def fail_after_partial(output) -> None:
        output.write(b"partial")
        raise RuntimeError("fault-injected producer failure")

    with pytest.raises(RuntimeError, match="fault-injected"):
        publish_generated(root, suffix=".csv", producer=fail_after_partial)
    assert read_immutable_bytes(prior.path) == b"accepted"
    assert not list(root.glob(".pending-v1-*"))

    monkeypatch.setattr(immutable_generation.os, "fsync", lambda _fd: (_ for _ in ()).throw(OSError("fsync")))
    with pytest.raises(OSError, match="fsync"):
        publish_bytes(root, b"another", suffix=".json", target_name="other-v1.json")
    assert read_immutable_bytes(prior.path) == b"accepted"
    assert not (root / "other-v1.json").exists()


@pytest.mark.parametrize("fsync_cut", (1, 2, 3))
def test_each_publication_fsync_cut_removes_unaccepted_generation(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    fsync_cut: int,
) -> None:
    root = tmp_path / "raw"
    prior = publish_bytes(root, b"prior", target_name="prior-v1.bin")
    real_fsync = immutable_generation.os.fsync
    calls = 0

    def fail_at_cut(descriptor: int) -> None:
        nonlocal calls
        calls += 1
        if calls == fsync_cut:
            raise OSError(f"fsync-cut-{fsync_cut}")
        real_fsync(descriptor)

    monkeypatch.setattr(immutable_generation.os, "fsync", fail_at_cut)
    with pytest.raises(OSError, match=f"fsync-cut-{fsync_cut}"):
        publish_bytes(root, b"new", target_name="new-v1.bin")
    assert read_immutable_bytes(prior.path) == b"prior"
    assert not (root / "new-v1.bin").exists()
    assert not list(root.glob(".pending-v1-*"))


def test_cancel_after_link_leaves_no_published_target(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    root = tmp_path / "raw"
    payload = b"cancel-after-link"
    target = root / f"{hashlib.sha256(payload).hexdigest()}.bin"
    real_link = immutable_generation.os.link

    def cancel_after_link(*args, **kwargs) -> None:
        real_link(*args, **kwargs)
        raise KeyboardInterrupt("cancel-after-link")

    monkeypatch.setattr(immutable_generation.os, "link", cancel_after_link)
    with pytest.raises(KeyboardInterrupt, match="cancel-after-link"):
        publish_bytes(root, payload, suffix=".bin")

    assert not target.exists()
    assert not list(root.glob(".pending-v1-*"))


def test_restart_retry_repairs_only_matching_interrupted_pending_link(tmp_path: Path) -> None:
    source = _write_private(tmp_path / "source.csv", b"restart-safe")
    receipt = inspect_regular_source(source)
    root = ensure_private_generation_root(tmp_path / "raw")
    target = root / f"{receipt.sha256.lower()}.csv"
    pending = root / ".pending-v1-crash-cut"
    _write_private(target, source.read_bytes())
    os.link(target, pending)
    assert target.stat().st_nlink == 2

    recovered = publish_source(source, root, expected=receipt)

    assert recovered.reused is True
    assert recovered.path == target
    assert target.stat().st_nlink == 1
    assert not pending.exists()
    assert read_immutable_bytes(target, expected_sha256=receipt.sha256) == b"restart-safe"


def test_actual_process_crash_after_link_is_recovered_by_fresh_process(tmp_path: Path) -> None:
    source = _write_private(tmp_path / "source.csv", b"process-restart-safe")
    root = tmp_path / "raw"
    environment = {**os.environ, "PYTHONPATH": str(Path(__file__).parents[1])}
    crash_script = """
import os
from pathlib import Path
from app.core import immutable_generation as generation
source = Path(os.environ['SOURCE'])
root = Path(os.environ['ROOT'])
receipt = generation.inspect_regular_source(source)
real_link = generation.os.link
def crash_after_link(*args, **kwargs):
    real_link(*args, **kwargs)
    os._exit(73)
generation.os.link = crash_after_link
generation.publish_source(source, root, expected=receipt)
"""
    crashed = subprocess.run(
        [sys.executable, "-c", crash_script],
        env={**environment, "SOURCE": str(source), "ROOT": str(root)},
        check=False,
    )
    assert crashed.returncode == 73
    target = root / f"{hashlib.sha256(source.read_bytes()).hexdigest()}.csv"
    pending = list(root.glob(".pending-v1-*"))
    assert target.exists()
    assert len(pending) == 1
    assert target.stat().st_ino == pending[0].stat().st_ino
    assert target.stat().st_nlink == 2

    recover_script = """
import json, os
from pathlib import Path
from app.core import immutable_generation as generation
source = Path(os.environ['SOURCE'])
result = generation.publish_source(source, Path(os.environ['ROOT']), expected=generation.inspect_regular_source(source))
print(json.dumps({'path': str(result.path), 'reused': result.reused}))
"""
    recovered = subprocess.run(
        [sys.executable, "-c", recover_script],
        env={**environment, "SOURCE": str(source), "ROOT": str(root)},
        check=True,
        capture_output=True,
        text=True,
    )
    payload = json.loads(recovered.stdout)
    assert payload == {"path": str(target), "reused": True}
    assert target.stat().st_nlink == 1
    assert not list(root.glob(".pending-v1-*"))


def test_actual_process_crash_before_link_leaves_only_inert_generation_and_retry_cleans_it(tmp_path: Path) -> None:
    source = _write_private(tmp_path / "source.csv", b"pre-link-restart-safe")
    root = tmp_path / "raw"
    environment = {
        **os.environ,
        "PYTHONPATH": str(Path(__file__).parents[1]),
        "SOURCE": str(source),
        "ROOT": str(root),
    }
    crash_script = """
import os
from pathlib import Path
from app.core import immutable_generation as generation
source = Path(os.environ['SOURCE'])
receipt = generation.inspect_regular_source(source)
def crash_during_copy(_path, _receipt, output):
    output.write(b'partial')
    output.flush()
    os._exit(74)
generation._copy_source_to_output = crash_during_copy
generation.publish_source(source, Path(os.environ['ROOT']), expected=receipt)
"""
    crashed = subprocess.run([sys.executable, "-c", crash_script], env=environment, check=False)
    assert crashed.returncode == 74
    assert len(list(root.glob(".pending-v1-*"))) == 1
    assert not list(root.glob("[!.]*"))

    recovered = publish_source(source, root, expected=inspect_regular_source(source))
    assert recovered.path.read_bytes() == b"pre-link-restart-safe"
    assert not list(root.glob(".pending-v1-*"))


def test_actual_process_crash_in_private_transform_workdir_is_cleaned_on_retry(tmp_path: Path) -> None:
    root = tmp_path / "work-root"
    environment = {
        **os.environ,
        "PYTHONPATH": str(Path(__file__).parents[1]),
        "ROOT": str(root),
    }
    crash_script = """
import os
from pathlib import Path
from app.core.immutable_generation import private_work_directory
with private_work_directory(Path(os.environ['ROOT'])) as work:
    output = work / 'sensitive.partial'
    output.write_bytes(b'partial-sensitive-data')
    output.chmod(0o600)
    os._exit(75)
"""
    crashed = subprocess.run([sys.executable, "-c", crash_script], env=environment, check=False)
    assert crashed.returncode == 75
    stale = list(root.glob(".work-v1-*"))
    assert len(stale) == 1
    assert (stale[0] / "sensitive.partial").exists()

    with private_work_directory(root) as current:
        assert not stale[0].exists()
        assert current.exists()
    assert not list(root.glob(".work-v1-*"))


def test_prepared_receipt_rejects_root_replacement_and_intermediate_symlink(tmp_path: Path) -> None:
    source = _write_private(tmp_path / "source.csv", b"root-bound")
    source_receipt = inspect_regular_source(source)
    generation = publish_source(source, tmp_path / "raw", expected=source_receipt)
    receipt = generation.receipt()
    ImportRepository._validate_prepared_generation(generation.path, receipt)

    old_root = tmp_path / "raw-old"
    generation.path.parent.rename(old_root)
    replacement_root = ensure_private_generation_root(tmp_path / "raw")
    _write_private(replacement_root / generation.path.name, b"root-bound")
    with pytest.raises(ImmutableGenerationError, match="root_identity_mismatch"):
        ImportRepository._validate_prepared_generation(generation.path, receipt)

    replacement_file = replacement_root / generation.path.name
    replacement_file.unlink()
    replacement_root.rmdir()
    (tmp_path / "raw").symlink_to(old_root, target_is_directory=True)
    with pytest.raises((ImmutableGenerationError, OSError)):
        ImportRepository._validate_prepared_generation(generation.path, receipt)


def test_run_import_rejects_missing_generation_receipt_before_engine_access(tmp_path: Path) -> None:
    source = _write_private(tmp_path / "legacy.csv", b"account,amount\n1,2\n")
    item = PreparedImportFile(
        file_id="file-1",
        display_name="legacy.csv",
        display_path="legacy.csv",
        real_path=source,
        file_type="CSV",
        size=source.stat().st_size,
        rows_total=1,
    )

    with pytest.raises(ImmutableGenerationError, match="receipt_required"):
        object.__new__(ImportRepository).run_file_import(
            engine=object(),
            case_id="case-a",
            item=item,
        )

    item.duckdb_csv_path = source
    item.csv_headers = ["交易账号", "交易时间", "交易金额"]
    item.kind_hint = "fc_transaction"
    with pytest.raises(ImmutableGenerationError, match="receipt_required"):
        object.__new__(ImportRepository).run_file_import_group(
            engine=object(),
            case_id="case-a",
            items=[item, item],
        )


def test_cross_case_prepared_generation_is_rejected(tmp_path: Path) -> None:
    case_a = tmp_path / "case-a"
    case_b = tmp_path / "case-b"
    case_a.mkdir(mode=0o700)
    case_b.mkdir(mode=0o700)
    raw_a = ensure_private_generation_root(case_a / "raw")
    ensure_private_generation_root(case_b / "raw")
    source = _write_private(tmp_path / "source.csv", b"account,amount\n1,2\n")
    generation = publish_source(source, raw_a, expected=inspect_regular_source(source), suffix=".csv")

    class Storage:
        def case_dir(self, case_id: str) -> Path:
            return {"case-a": case_a, "case-b": case_b}[case_id]

    repository = object.__new__(ImportRepository)
    repository._storage = Storage()
    item = PreparedImportFile(
        file_id="file-a",
        display_name="source.csv",
        display_path="controlled/source.csv",
        real_path=generation.path,
        file_type="CSV",
        size=generation.size,
        rows_total=1,
        sha256=generation.sha256,
        source_sha256=inspect_regular_source(source).sha256,
        source_size=inspect_regular_source(source).size,
        ingestion_receipt=repository._register_case_bound_generation(
            case_id="case-a",
            authority_root=raw_a,
            path=generation.path,
            receipt=generation.receipt(),
        ),
    )

    repository._validate_prepared_item("case-a", item)
    with pytest.raises(ImmutableGenerationError, match="case_binding_mismatch"):
        repository._validate_prepared_item("case-b", item)


def test_missing_source_receipt_is_rejected(tmp_path: Path) -> None:
    root = ensure_private_generation_root(tmp_path / "raw")
    generation = publish_bytes(root, b"account,amount\n1,2\n", suffix=".csv")

    with pytest.raises(
        ImmutableGenerationError,
        match="^prepared_import_source_receipt_invalid$",
    ):
        ImportRepository._validate_prepared_generation(
            generation.path,
            generation.receipt(),
        )


def test_case_bound_batch_admission_is_atomic(tmp_path: Path) -> None:
    case_root = tmp_path / "case-a"
    case_root.mkdir(mode=0o700)
    raw_root = ensure_private_generation_root(case_root / "raw")
    source_path = _write_private(
        tmp_path / "source.csv",
        b"account,amount\n1,2\n",
    )
    source_receipt = inspect_regular_source(source_path)
    valid = publish_source(
        source_path,
        raw_root,
        expected=source_receipt,
        suffix=".csv",
    )
    invalid = publish_bytes(
        raw_root,
        b"account,amount\n3,4\n",
        suffix=".csv",
    )
    repository = object.__new__(ImportRepository)
    repository._storage = _CaseStorage(case_root)
    items = [
        PreparedImportFile(
            file_id="valid",
            display_name="valid.csv",
            display_path="controlled/valid.csv",
            real_path=valid.path,
            file_type="CSV",
            size=valid.size,
            rows_total=1,
            ingestion_receipt=valid.receipt(),
        ),
        PreparedImportFile(
            file_id="invalid",
            display_name="invalid.csv",
            display_path="controlled/invalid.csv",
            real_path=invalid.path,
            file_type="CSV",
            size=invalid.size,
            rows_total=1,
            ingestion_receipt=invalid.receipt(),
        ),
    ]

    with pytest.raises(
        ImmutableGenerationError,
        match="^prepared_import_source_receipt_invalid$",
    ):
        repository._bind_prepared_items_to_case(
            case_id="case-a",
            authority_root=raw_root,
            items=items,
        )

    assert not getattr(repository, "_prepared_generation_registry", {})
    assert items[0].ingestion_receipt.get("schema_version") == 1
    assert "receipt_id" not in items[0].ingestion_receipt


def test_prepared_generation_requires_registry_membership(tmp_path: Path) -> None:
    case_root = tmp_path / "case-a"
    case_root.mkdir(mode=0o700)
    raw_root = ensure_private_generation_root(case_root / "raw")
    source = _write_private(tmp_path / "source.csv", b"account,amount\n1,2\n")
    generation = publish_source(source, raw_root, expected=inspect_regular_source(source), suffix=".csv")
    producer = object.__new__(ImportRepository)
    producer._storage = _CaseStorage(case_root)
    receipt = producer._register_case_bound_generation(
        case_id="case-a",
        authority_root=raw_root,
        path=generation.path,
        receipt=generation.receipt(),
    )
    item = PreparedImportFile(
        file_id="file-a",
        display_name="source.csv",
        display_path="controlled/source.csv",
        real_path=generation.path,
        file_type="CSV",
        size=generation.size,
        rows_total=1,
        ingestion_receipt=receipt,
    )

    restarted = object.__new__(ImportRepository)
    restarted._storage = _CaseStorage(case_root)
    with pytest.raises(ImmutableGenerationError, match="registry_membership_required"):
        restarted._validate_prepared_item("case-a", item)


def test_mismatched_prepared_derivative_is_rejected(tmp_path: Path) -> None:
    case_root = tmp_path / "case-a"
    case_root.mkdir(mode=0o700)
    raw_root = ensure_private_generation_root(case_root / "raw")
    derivative_root = ensure_private_generation_root(raw_root / "_duckdb_csv")
    source_a_path = _write_private(tmp_path / "source-a.csv", b"account,amount\nA,1\n")
    source_b_path = _write_private(tmp_path / "source-b.csv", b"account,amount\nB,999\n")
    source_a_receipt = inspect_regular_source(source_a_path)
    source_b_receipt = inspect_regular_source(source_b_path)
    source_a = publish_source(
        source_a_path,
        raw_root,
        expected=source_a_receipt,
        suffix=".csv",
    )
    source_b = publish_source(
        source_b_path,
        raw_root,
        expected=source_b_receipt,
        suffix=".csv",
    )
    derivative_b = publish_bytes(
        derivative_root,
        b"account,amount\nB,999\n",
        suffix=".csv",
        lineage={
            "kind": "duckdb_csv_derivative",
            "derived": True,
            "derived_from_sha256": source_b.sha256,
            "transform": "prepare_csv_clean_v1",
            "encrypted_source": False,
        },
    )
    repository = object.__new__(ImportRepository)
    repository._storage = _CaseStorage(case_root)
    item = PreparedImportFile(
        file_id="file-a",
        display_name="source.csv",
        display_path="controlled/source.csv",
        real_path=source_a.path,
        file_type="CSV",
        size=source_a.size,
        rows_total=1,
        sha256=source_a.sha256,
        source_sha256=source_a_receipt.sha256,
        source_size=source_a_receipt.size,
        ingestion_receipt=repository._register_case_bound_generation(
            case_id="case-a",
            authority_root=raw_root,
            path=source_a.path,
            receipt=source_a.receipt(),
        ),
        duckdb_csv_path=derivative_b.path,
        duckdb_csv_receipt=repository._register_case_bound_generation(
            case_id="case-a",
            authority_root=raw_root,
            path=derivative_b.path,
            receipt=repository._derived_generation_receipt(
                derivative_b.path,
                source_generation=source_b,
                root_source=source_b_receipt,
                derivation="duckdb_csv_derivative",
                encrypted_source=False,
            ),
        ),
    )

    with pytest.raises(ImmutableGenerationError, match="derivative_lineage_mismatch"):
        repository._validate_prepared_item("case-a", item)


def test_excel_normalization_publishes_immutable_generation_without_fixed_temp(tmp_path: Path) -> None:
    raw_csv = _write_private(
        tmp_path / "raw.csv",
        "说明行\n交易账号,交易时间,交易金额\n6222,2026-01-01,10.00\n".encode("utf-8"),
    )
    output_hint = tmp_path / "normalized" / "legacy.import.csv"
    repository = object.__new__(ImportRepository)

    first = repository._normalize_excel_import_csv(raw_csv=raw_csv, output=output_hint)
    second = repository._normalize_excel_import_csv(raw_csv=raw_csv, output=output_hint)

    payload = first.read_text(encoding="utf-8")
    assert payload.startswith("交易账号,交易时间,交易金额")
    assert "说明行" not in payload
    assert first == second
    assert first.name == f"{hashlib.sha256(first.read_bytes()).hexdigest()}.csv"
    assert not list(first.parent.glob("*.tmp"))
    assert not list(first.parent.glob(".pending-v1-*"))


def test_preview_manifest_is_versioned_immutable_and_binds_full_lineage(tmp_path: Path) -> None:
    source = _write_private(tmp_path / "source.xlsx", b"not-encrypted-fixture")
    csv_input = _write_private(tmp_path / "normalized-input.csv", b"account,amount\n1,2\n")
    preview_root = ensure_private_generation_root(tmp_path / ".import_preview_cache")
    csv_root = ensure_private_generation_root(preview_root / "_excelcsv")
    csv_path = publish_source(csv_input, csv_root, expected=inspect_regular_source(csv_input), suffix=".csv").path
    repository = object.__new__(ImportRepository)

    repository._record_excel_csv_preview_manifest(
        source_path=source,
        source_key="sheet-1",
        excel_path=source,
        csv_path=csv_path,
        rows_total=1,
    )
    entry = repository._load_excel_csv_preview_manifest_entry(source, source_key="sheet-1")
    assert entry is not None
    assert entry["csv_sha256"] == hashlib.sha256(csv_path.read_bytes()).hexdigest().upper()
    assert entry["excel_sha256"] == hashlib.sha256(source.read_bytes()).hexdigest().upper()

    manifest_path = repository._excel_csv_preview_manifest_path(source, source_key="sheet-1")
    manifest = json.loads(read_immutable_bytes(manifest_path).decode("utf-8"))
    assert manifest["schema_version"] == 2
    assert manifest["lineage"] == {
        "kind": "excel_preview_manifest",
        "derived": True,
        "derived_from_sha256": hashlib.sha256(source.read_bytes()).hexdigest().upper(),
        "encrypted_source": False,
        "transform": "excel_import_header_normalize_v1",
    }

    restarted_repository = object.__new__(ImportRepository)
    assert restarted_repository._load_excel_csv_preview_manifest_entry(source, source_key="sheet-1") is None

    csv_path.write_bytes(b"account,amount\n1,999\n")
    assert repository._load_excel_csv_preview_manifest_entry(source, source_key="sheet-1") is None


def test_preview_manifest_swap_after_validation_cannot_change_consumed_bytes(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    source = _write_private(tmp_path / "source.xlsx", b"excel-source")
    csv_input = _write_private(tmp_path / "normalized-input.csv", b"account,amount\n1,2\n")
    preview_root = ensure_private_generation_root(tmp_path / ".import_preview_cache")
    csv_root = ensure_private_generation_root(preview_root / "_excelcsv")
    csv_path = publish_source(csv_input, csv_root, expected=inspect_regular_source(csv_input), suffix=".csv").path
    repository = object.__new__(ImportRepository)
    repository._record_excel_csv_preview_manifest(
        source_path=source,
        source_key="sheet-1",
        excel_path=source,
        csv_path=csv_path,
        rows_total=1,
    )
    manifest_path = repository._excel_csv_preview_manifest_path(source, source_key="sheet-1")
    replacement = json.loads(read_immutable_bytes(manifest_path).decode("utf-8"))
    replacement["artifact"]["rows_total"] = 999
    replacement_bytes = json.dumps(
        replacement,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    ).encode("utf-8")
    original_validate = repository._validate_prepared_generation

    def swap_after_validation(path: Path, receipt: dict[str, object]) -> None:
        original_validate(path, receipt)
        swap = path.parent / ".manifest-swap"
        _write_private(swap, replacement_bytes)
        os.replace(swap, path)

    monkeypatch.setattr(repository, "_validate_prepared_generation", swap_after_validation)
    assert repository._load_excel_csv_preview_manifest_entry(source, source_key="sheet-1") is None
