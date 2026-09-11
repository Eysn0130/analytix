from __future__ import annotations

import hashlib
import json
import os
import stat
import threading
from pathlib import Path

import pytest

from app.core.db_engine import DuckDBEngine
from app.core.execution_authority_health import ExecutionAuthorityHealth
from app.repositories import document_repository as document_repository_module
from app.repositories.document_repository import (
    DocumentExtraction,
    DocumentGenerationRecoveryPlan,
    DocumentRepository,
    TextSegment,
)


_CASE_ID = "case-a"
_FILE_ID = "document-a"
_SENSITIVE_ACCOUNT = "6222020202020202020"


class _SimulatedProcessCrash(BaseException):
    pass


class _TestStorage:
    def __init__(self, case_root: Path) -> None:
        self._case_root = case_root

    def case_dir(self, case_id: str) -> Path:
        assert case_id == _CASE_ID
        return self._case_root


@pytest.fixture
def document_runtime(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
):
    case_root = tmp_path.resolve() / "cases" / _CASE_ID
    case_root.mkdir(parents=True)
    case_root.chmod(0o700)
    repository = DocumentRepository(vector_mode="none")
    repository._storage = _TestStorage(case_root)
    engine = DuckDBEngine(tmp_path / "document-generation.duckdb")
    monkeypatch.setattr(
        document_repository_module,
        "require_controlled_source_ingestion",
        lambda: None,
    )
    try:
        yield repository, engine, case_root
    finally:
        engine.close()


def _install_extraction(
    repository: DocumentRepository,
    monkeypatch: pytest.MonkeyPatch,
    state: dict[str, object],
) -> None:
    def extract(*, case_id: str, path: Path) -> DocumentExtraction:
        assert case_id == _CASE_ID
        assert path.name.endswith(".txt")
        error = state.get("error")
        if isinstance(error, BaseException):
            raise error
        return DocumentExtraction(
            segments=[TextSegment(text=str(state.get("text") or ""))],
            parser="fixture:text",
        )

    monkeypatch.setattr(repository, "_extract_document", extract)


def _ingest(
    repository: DocumentRepository,
    engine: DuckDBEngine,
    *,
    content_identity: str,
    file_id: str = _FILE_ID,
):
    return repository.ingest_imported_file(
        engine=engine,
        case_id=_CASE_ID,
        file_id=file_id,
        filename="evidence.txt",
        display_path="controlled-source",
        stored_path=f"/not-read/case-{_SENSITIVE_ACCOUNT}.txt",
        file_type="txt",
        size=len(content_identity),
        md5=hashlib.md5(content_identity.encode("utf-8")).hexdigest(),
        sha256=hashlib.sha256(content_identity.encode("utf-8")).hexdigest(),
        kind="support_file",
    )


def _snapshot(
    repository: DocumentRepository,
    engine: DuckDBEngine,
) -> dict[str, object]:
    asset = repository._get_asset_row(engine, case_id=_CASE_ID, document_id=_FILE_ID)
    assert asset is not None
    content_path = Path(str(asset["content_path"]))
    payload = content_path.read_bytes()
    chunks = engine.query(
        "SELECT chunk_id, chunk_index, text, vector_model, vector_data "
        "FROM document_chunks WHERE case_id=? AND document_id=? ORDER BY chunk_index",
        (_CASE_ID, _FILE_ID),
    )
    return {
        "asset": asset,
        "content_path": content_path,
        "payload": payload,
        "sha256": hashlib.sha256(payload).hexdigest(),
        "chunks": chunks,
    }


def _assert_prior_unchanged(
    repository: DocumentRepository,
    engine: DuckDBEngine,
    prior: dict[str, object],
) -> None:
    current = _snapshot(repository, engine)
    assert current == prior
    prior_path = prior["content_path"]
    assert isinstance(prior_path, Path)
    assert prior_path.exists()
    assert prior_path.read_bytes() == prior["payload"]
    assert hashlib.sha256(prior_path.read_bytes()).hexdigest() == prior["sha256"]


def test_reingest_switches_file_and_database_only_after_complete_generation(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, _case_root = document_runtime
    state: dict[str, object] = {"text": "prior verified document text"}
    _install_extraction(repository, monkeypatch, state)

    first = _ingest(repository, engine, content_identity="prior-source")
    assert first.llm_ready is True
    prior = _snapshot(repository, engine)
    prior_path = prior["content_path"]
    assert isinstance(prior_path, Path)
    assert prior_path.stem == prior["sha256"]

    state["text"] = "replacement verified document text"
    second = _ingest(repository, engine, content_identity="replacement-source")

    assert second.llm_ready is True
    current = _snapshot(repository, engine)
    current_path = current["content_path"]
    assert isinstance(current_path, Path)
    assert current_path != prior_path
    assert current_path.exists()
    assert current_path.stem == current["sha256"]
    assert current_path.read_text(encoding="utf-8") == "replacement verified document text"
    assert current["asset"]["content_path"] == str(current_path)
    assert all("replacement verified" in str(row[2]) for row in current["chunks"])
    assert not prior_path.exists()


def test_reingest_keeps_prior_generation_while_duplicate_asset_references_it(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, _case_root = document_runtime
    state: dict[str, object] = {"text": "shared verified document text"}
    _install_extraction(repository, monkeypatch, state)

    assert _ingest(repository, engine, content_identity="shared-source").llm_ready is True
    prior = _snapshot(repository, engine)
    prior_path = prior["content_path"]
    assert isinstance(prior_path, Path)
    duplicate = _ingest(
        repository,
        engine,
        content_identity="shared-source",
        file_id="document-b",
    )
    assert duplicate.extraction_status == "duplicate"

    state["text"] = "replacement verified document text"
    assert _ingest(repository, engine, content_identity="replacement-source").llm_ready is True

    duplicate_rows = engine.query(
        "SELECT content_path FROM document_assets WHERE case_id=? AND document_id=?",
        (_CASE_ID, "document-b"),
    )
    assert duplicate_rows == [(str(prior_path),)]
    assert prior_path.read_text(encoding="utf-8") == "shared verified document text"


def test_identical_same_timestamp_reingest_is_verified_noop_not_ambiguous_intent(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    monkeypatch.setattr(document_repository_module, "_now_iso", lambda: "2026-07-15 12:00:00")
    state: dict[str, object] = {"text": "identical accepted document text"}
    _install_extraction(repository, monkeypatch, state)
    first = _ingest(repository, engine, content_identity="identical-source")
    assert first.llm_ready is True
    prior = _snapshot(repository, engine)
    journal_before = _journal_snapshot(_intent_directory(case_root))

    second = _ingest(repository, engine, content_identity="identical-source")

    assert second.llm_ready is True
    _assert_prior_unchanged(repository, engine, prior)
    assert _journal_snapshot(_intent_directory(case_root)) == journal_before
    assert not (_intent_directory(case_root) / ".active-intent-v1.json").exists()


@pytest.mark.parametrize("failure", ("extract", "chunk", "empty"))
def test_extraction_or_chunk_failure_preserves_prior_generation(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
    failure: str,
) -> None:
    repository, engine, _case_root = document_runtime
    state: dict[str, object] = {"text": "prior verified document text"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)

    if failure == "extract":
        state["error"] = RuntimeError(f"secret-{_SENSITIVE_ACCOUNT}")
    elif failure == "chunk":
        monkeypatch.setattr(
            repository,
            "_build_chunk_rows",
            lambda **_kwargs: (_ for _ in ()).throw(RuntimeError(f"secret-{_SENSITIVE_ACCOUNT}")),
        )
        state["text"] = "replacement text"
    else:
        state["text"] = ""

    result = _ingest(repository, engine, content_identity=f"failed-{failure}")

    assert result.llm_ready is False
    assert result.extraction_status == "failed"
    assert result.last_error in {"document_extract_failed", "document_content_unavailable"}
    assert _SENSITIVE_ACCOUNT not in result.last_error
    _assert_prior_unchanged(repository, engine, prior)


@pytest.mark.parametrize(
    "failure_point",
    ("begin", "begin_after_open", "chunk_insert", "asset_upsert", "commit"),
)
def test_database_failure_rolls_back_and_preserves_prior_generation(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
    failure_point: str,
) -> None:
    repository, engine, _case_root = document_runtime
    state: dict[str, object] = {"text": "prior verified document text"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    state["text"] = "replacement text that must be rolled back"

    real_execute = engine.execute

    def failing_execute(sql: str, params=None) -> None:
        normalized = " ".join(str(sql).lower().split())
        if failure_point == "begin_after_open" and normalized == "begin transaction":
            real_execute(sql, params)
            raise RuntimeError(f"database-secret-{_SENSITIVE_ACCOUNT}")
        should_fail = (
            (failure_point == "begin" and normalized == "begin transaction")
            or (failure_point == "chunk_insert" and normalized.startswith("insert into document_chunks"))
            or (failure_point == "asset_upsert" and normalized.startswith("update document_assets"))
            or (failure_point == "commit" and normalized == "commit")
        )
        if should_fail:
            raise RuntimeError(f"database-secret-{_SENSITIVE_ACCOUNT}")
        real_execute(sql, params)

    monkeypatch.setattr(engine, "execute", failing_execute)
    result = _ingest(repository, engine, content_identity=f"replacement-{failure_point}")

    assert result.llm_ready is False
    assert result.last_error == "document_database_update_failed"
    assert _SENSITIVE_ACCOUNT not in result.last_error
    assert repository._document_engine_transaction_is_active(engine) is False
    _assert_prior_unchanged(repository, engine, prior)
    prior_path = prior["content_path"]
    assert isinstance(prior_path, Path)
    assert list(prior_path.parent.glob("*.txt")) == [prior_path]


def test_existing_outer_transaction_fails_closed_without_mutating_prior(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, _case_root = document_runtime
    state: dict[str, object] = {"text": "prior verified document text"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    state["text"] = "replacement must not enter caller transaction"

    engine.execute("BEGIN TRANSACTION")
    try:
        result = _ingest(repository, engine, content_identity="nested-source")
        assert result.llm_ready is False
        assert result.last_error == "document_database_update_failed"
        _assert_prior_unchanged(repository, engine, prior)
    finally:
        engine.execute("ROLLBACK")


def test_existing_outer_transaction_is_rejected_before_schema_or_file_mutation(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    monkeypatch.setattr(
        repository,
        "_extract_document",
        lambda **_kwargs: pytest.fail("extraction must not run inside a caller transaction"),
    )

    engine.execute("BEGIN TRANSACTION")
    try:
        result = _ingest(repository, engine, content_identity="nested-first-ingest")
        assert result.llm_ready is False
        assert result.last_error == "document_database_update_failed"
        assert engine.query(
            "SELECT table_name FROM information_schema.tables "
            "WHERE table_schema='main' AND table_name IN ('document_assets', 'document_chunks')"
        ) == []
        assert not (case_root / "knowledge").exists()
    finally:
        engine.execute("ROLLBACK")


def test_text_fsync_failure_preserves_prior_generation(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, _case_root = document_runtime
    state: dict[str, object] = {"text": "prior verified document text"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    state["text"] = "replacement text blocked by fsync"
    real_fsync = document_repository_module.os.fsync
    failed = False
    publication_started = False

    def mark_publication_started(phase: str) -> None:
        nonlocal publication_started
        if phase == "publish_started_recorded":
            publication_started = True

    def fail_new_regular_file(descriptor: int) -> None:
        nonlocal failed
        if publication_started and not failed and stat.S_ISREG(os.fstat(descriptor).st_mode):
            failed = True
            raise OSError("fsync failed")
        real_fsync(descriptor)

    monkeypatch.setattr(repository, "_document_generation_crash_cut", mark_publication_started)
    monkeypatch.setattr(document_repository_module.os, "fsync", fail_new_regular_file)
    result = _ingest(repository, engine, content_identity="replacement-fsync")

    assert failed is True
    assert result.llm_ready is False
    assert result.last_error == "document_publish_failed"
    _assert_prior_unchanged(repository, engine, prior)


def test_document_source_swap_back_is_rejected_before_helper_input_creation(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    source = tmp_path / "source.doc"
    source.write_bytes(b"host-pinned-source")
    displaced = tmp_path / "source.displaced"
    attacker = tmp_path / "attacker.doc"
    attacker.write_bytes(b"attacker-controlled-source")
    target = tmp_path / "opaque.doc"
    real_open = document_repository_module.os.open
    swapped = False

    def swap_back_before_open(path, flags, *args, **kwargs):
        nonlocal swapped
        if not swapped and Path(path) == source:
            swapped = True
            source.rename(displaced)
            attacker.rename(source)
            descriptor = real_open(path, flags, *args, **kwargs)
            source.rename(attacker)
            displaced.rename(source)
            return descriptor
        return real_open(path, flags, *args, **kwargs)

    monkeypatch.setattr(document_repository_module.os, "open", swap_back_before_open)
    with pytest.raises(
        document_repository_module.ExternalRuntimeError,
        match="^document_source_unavailable$",
    ):
        document_repository_module._copy_regular_source(source, target)

    assert swapped is True
    assert source.read_bytes() == b"host-pinned-source"
    assert attacker.read_bytes() == b"attacker-controlled-source"
    assert not target.exists()


def test_document_execution_admission_precedes_runtime_source_render_and_temp(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    blocker = "managed_process_capability_admission_unavailable"
    repository = DocumentRepository(vector_mode="none")
    monkeypatch.setattr(
        document_repository_module,
        "_document_execution_capability_health",
        lambda: ExecutionAuthorityHealth(available=False, reason_code=blocker),
    )

    def must_not_run(*_args, **_kwargs):
        raise AssertionError("runtime/source/render/temp work preceded capability admission")

    monkeypatch.setattr(document_repository_module, "_resolved_document_runtime", must_not_run)
    monkeypatch.setattr(document_repository_module, "_copy_regular_source", must_not_run)
    monkeypatch.setattr(document_repository_module.tempfile, "TemporaryDirectory", must_not_run)

    with pytest.raises(
        document_repository_module.ExternalRuntimeError,
        match=f"^{blocker}$",
    ):
        repository._extract_document(
            case_id=_CASE_ID,
            path=tmp_path / "case-account-6222020202020202020.doc",
        )
    with pytest.raises(
        document_repository_module.ExternalRuntimeError,
        match=f"^{blocker}$",
    ):
        repository._convert_doc_with_soffice(
            case_id=_CASE_ID,
            office_binary="/not-inspected/soffice",
            expected_executable_sha256="not-inspected",
            path=tmp_path / "not-read.doc",
        )
    with pytest.raises(
        document_repository_module.ExternalRuntimeError,
        match=f"^{blocker}$",
    ):
        repository._extract_pdf_with_ocr(
            tmp_path / "not-rendered.pdf",
            [1],
        )


def test_converted_document_post_visibility_fsync_failure_removes_new_generation(
    document_runtime,
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, _engine, case_root = document_runtime
    staged = tmp_path / "converted.txt"
    staged.write_bytes(b"derived exact bytes")
    source_sha256 = hashlib.sha256(b"source exact bytes").hexdigest()
    target = case_root / "knowledge" / "_docconvert" / source_sha256 / "document.txt"
    real_fsync = document_repository_module.os.fsync
    failed = False

    def fail_first_directory_fsync_after_visibility(descriptor: int) -> None:
        nonlocal failed
        if not failed and target.exists() and stat.S_ISDIR(os.fstat(descriptor).st_mode):
            failed = True
            raise OSError("post-visibility fsync failure")
        real_fsync(descriptor)

    monkeypatch.setattr(
        document_repository_module.os,
        "fsync",
        fail_first_directory_fsync_after_visibility,
    )
    with pytest.raises(
        document_repository_module.ExternalRuntimeError,
        match="^document_publish_failed$",
    ):
        repository._publish_converted_document(
            case_id=_CASE_ID,
            source_sha256=source_sha256,
            staged_path=staged,
            suffix=".txt",
        )

    assert failed is True
    assert not target.exists()
    assert not list(target.parent.glob(".generation-stage-v1-*.tmp"))


def test_converted_document_conflict_preserves_prior_accepted_generation(
    document_runtime,
    tmp_path: Path,
) -> None:
    repository, _engine, _case_root = document_runtime
    source_sha256 = hashlib.sha256(b"one source generation").hexdigest()
    first = tmp_path / "first.txt"
    first.write_bytes(b"prior accepted derivative")
    target = repository._publish_converted_document(
        case_id=_CASE_ID,
        source_sha256=source_sha256,
        staged_path=first,
        suffix=".txt",
    )
    second = tmp_path / "second.txt"
    second.write_bytes(b"conflicting derivative")

    with pytest.raises(
        document_repository_module.ExternalRuntimeError,
        match="^document_publish_conflict$",
    ):
        repository._publish_converted_document(
            case_id=_CASE_ID,
            source_sha256=source_sha256,
            staged_path=second,
            suffix=".txt",
        )

    assert target.read_bytes() == b"prior accepted derivative"


def test_symlink_generation_target_is_rejected_without_overwrite(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, _case_root = document_runtime
    state: dict[str, object] = {"text": "prior verified document text"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    prior_path = prior["content_path"]
    assert isinstance(prior_path, Path)

    replacement = "replacement text blocked by target symlink"
    replacement_name = f"{hashlib.sha256(replacement.encode('utf-8')).hexdigest()}.txt"
    attacker = prior_path.parent / "attacker-owned"
    attacker.write_text("attacker bytes", encoding="utf-8")
    target = prior_path.parent / replacement_name
    target.symlink_to(attacker)
    state["text"] = replacement

    result = _ingest(repository, engine, content_identity="replacement-symlink")

    assert result.llm_ready is False
    assert result.last_error == "document_intent_recovery_blocked"
    assert target.is_symlink()
    assert attacker.read_text(encoding="utf-8") == "attacker bytes"
    _assert_prior_unchanged(repository, engine, prior)


def test_target_appearance_race_is_rejected_and_not_deleted(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, _case_root = document_runtime
    state: dict[str, object] = {"text": "prior verified document text"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    replacement = "replacement text blocked by target race"
    replacement_name = f"{hashlib.sha256(replacement.encode('utf-8')).hexdigest()}.txt"
    real_link = document_repository_module.os.link
    raced = False

    def racing_link(src, dst, *, src_dir_fd=None, dst_dir_fd=None, follow_symlinks=True):
        nonlocal raced
        if str(dst) == replacement_name:
            raced = True
            descriptor = os.open(
                dst,
                os.O_WRONLY | os.O_CREAT | os.O_EXCL,
                0o600,
                dir_fd=dst_dir_fd,
            )
            try:
                os.write(descriptor, b"attacker race bytes")
            finally:
                os.close(descriptor)
            raise FileExistsError
        return real_link(
            src,
            dst,
            src_dir_fd=src_dir_fd,
            dst_dir_fd=dst_dir_fd,
            follow_symlinks=follow_symlinks,
        )

    monkeypatch.setattr(document_repository_module.os, "link", racing_link)
    state["text"] = replacement
    result = _ingest(repository, engine, content_identity="replacement-race")

    assert raced is True
    assert result.llm_ready is False
    assert result.last_error == "document_intent_recovery_blocked"
    _assert_prior_unchanged(repository, engine, prior)
    prior_path = prior["content_path"]
    assert isinstance(prior_path, Path)
    raced_target = prior_path.parent / replacement_name
    assert raced_target.read_bytes() == b"attacker race bytes"


def _intent_directory(case_root: Path, document_id: str = _FILE_ID) -> Path:
    document_key = document_repository_module._document_text_storage_key(document_id)
    return case_root / "knowledge" / "document-intents-v1" / document_key


def _journal_snapshot(directory: Path) -> dict[str, tuple[int, int, bytes]]:
    if not directory.exists():
        return {}
    result: dict[str, tuple[int, int, bytes]] = {}
    for path in sorted(directory.iterdir(), key=lambda item: item.name):
        metadata = path.lstat()
        if path.is_file() and not path.is_symlink():
            result[path.name] = (int(metadata.st_ino), stat.S_IMODE(metadata.st_mode), path.read_bytes())
        else:
            result[path.name] = (int(metadata.st_ino), stat.S_IMODE(metadata.st_mode), b"")
    return result


@pytest.mark.parametrize(
    ("crash_cut", "expected_action"),
    (
        ("intent_prepared", "rollback_new"),
        ("publish_started_recorded", "rollback_new"),
        ("text_published", "rollback_new"),
        ("published_recorded", "rollback_new"),
        ("db_started_recorded", "rollback_new"),
        ("db_begun", "rollback_new"),
        ("db_mutated", "rollback_new"),
        ("db_commit_visible", "finalize_new"),
        ("db_committed_recorded", "finalize_new"),
        ("terminal_recorded", "finalize_new"),
    ),
)
def test_reopen_recovery_uses_journal_and_database_state_for_every_crash_cut(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
    crash_cut: str,
    expected_action: str,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    prior_path = prior["content_path"]
    assert isinstance(prior_path, Path)
    state["text"] = "replacement generation after durable recovery"

    def crash_at(phase: str) -> None:
        if phase == crash_cut:
            raise _SimulatedProcessCrash(phase)

    monkeypatch.setattr(repository, "_document_generation_crash_cut", crash_at)
    with pytest.raises(_SimulatedProcessCrash):
        _ingest(repository, engine, content_identity=f"replacement-{crash_cut}")
    engine.close()

    reopened_engine = DuckDBEngine(engine.path)
    reopened_repository = DocumentRepository(vector_mode="none")
    reopened_repository._storage = _TestStorage(case_root)
    try:
        journal_before = _journal_snapshot(_intent_directory(case_root))
        plan = reopened_repository.plan_document_generation_recovery(
            reopened_engine,
            case_id=_CASE_ID,
            document_id=_FILE_ID,
            file_id=_FILE_ID,
        )
        assert plan.action == expected_action
        assert _journal_snapshot(_intent_directory(case_root)) == journal_before

        applied = reopened_repository.apply_document_generation_recovery(
            reopened_engine,
            case_id=_CASE_ID,
            document_id=_FILE_ID,
            file_id=_FILE_ID,
        )
        assert applied.action == "none"
        assert not (_intent_directory(case_root) / ".active-intent-v1.json").exists()
        if expected_action == "rollback_new":
            _assert_prior_unchanged(reopened_repository, reopened_engine, prior)
            replacement_name = (
                hashlib.sha256(b"replacement generation after durable recovery").hexdigest()
                + ".txt"
            )
            assert not (prior_path.parent / replacement_name).exists()
        else:
            current = _snapshot(reopened_repository, reopened_engine)
            current_path = current["content_path"]
            assert isinstance(current_path, Path)
            assert current_path.read_text(encoding="utf-8") == (
                "replacement generation after durable recovery"
            )
            assert current_path.stem == current["sha256"]
            assert not prior_path.exists()
    finally:
        reopened_engine.close()


@pytest.mark.parametrize(
    ("regular_fsync_number", "expected_action"),
    (
        (1, "none"),
        (2, "rollback_new"),
        (3, "rollback_new"),
        (4, "rollback_new"),
        (5, "rollback_new"),
        (6, "rollback_new"),
        (7, "finalize_new"),
        (8, "finalize_new"),
    ),
)
def test_reopen_recovery_handles_each_regular_file_fsync_crash_cut(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
    regular_fsync_number: int,
    expected_action: str,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    state["text"] = "replacement generation at fsync boundary"
    real_fsync = document_repository_module.os.fsync
    regular_calls = 0

    def crash_on_regular_fsync(descriptor: int) -> None:
        nonlocal regular_calls
        if stat.S_ISREG(os.fstat(descriptor).st_mode):
            regular_calls += 1
            if regular_calls == regular_fsync_number:
                raise _SimulatedProcessCrash(f"regular-fsync-{regular_calls}")
        real_fsync(descriptor)

    monkeypatch.setattr(document_repository_module.os, "fsync", crash_on_regular_fsync)
    with pytest.raises(_SimulatedProcessCrash):
        _ingest(repository, engine, content_identity=f"fsync-{regular_fsync_number}")
    monkeypatch.setattr(document_repository_module.os, "fsync", real_fsync)
    engine.close()

    reopened_engine = DuckDBEngine(engine.path)
    reopened_repository = DocumentRepository(vector_mode="none")
    reopened_repository._storage = _TestStorage(case_root)
    try:
        plan = reopened_repository.plan_document_generation_recovery(
            reopened_engine,
            case_id=_CASE_ID,
            document_id=_FILE_ID,
            file_id=_FILE_ID,
        )
        assert plan.action == expected_action
        applied = reopened_repository.apply_document_generation_recovery(
            reopened_engine,
            case_id=_CASE_ID,
            document_id=_FILE_ID,
            file_id=_FILE_ID,
        )
        assert applied.action == "none"
        if expected_action in {"none", "rollback_new"}:
            _assert_prior_unchanged(reopened_repository, reopened_engine, prior)
        else:
            current = _snapshot(reopened_repository, reopened_engine)
            assert Path(str(current["content_path"])).read_text(encoding="utf-8") == (
                "replacement generation at fsync boundary"
            )
    finally:
        reopened_engine.close()


@pytest.mark.parametrize(
    ("directory_fsync_number", "expected_action", "expected_generation"),
    tuple((number, "none", "prior") for number in range(1, 7))
    + tuple((number, "rollback_new", "prior") for number in range(7, 16))
    + tuple((number, "finalize_new", "replacement") for number in range(16, 19))
    + ((19, "none", "replacement"),),
)
def test_reopen_recovery_handles_each_durable_directory_fsync_crash_cut(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
    directory_fsync_number: int,
    expected_action: str,
    expected_generation: str,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    state["text"] = "replacement generation at directory fsync boundary"
    real_fsync = document_repository_module.os.fsync
    directory_calls = 0

    def crash_on_directory_fsync(descriptor: int) -> None:
        nonlocal directory_calls
        if stat.S_ISDIR(os.fstat(descriptor).st_mode):
            directory_calls += 1
            if directory_calls == directory_fsync_number:
                raise _SimulatedProcessCrash(f"directory-fsync-{directory_calls}")
        real_fsync(descriptor)

    monkeypatch.setattr(document_repository_module.os, "fsync", crash_on_directory_fsync)
    with pytest.raises(_SimulatedProcessCrash):
        _ingest(repository, engine, content_identity=f"dir-fsync-{directory_fsync_number}")
    monkeypatch.setattr(document_repository_module.os, "fsync", real_fsync)
    engine.close()

    reopened_engine = DuckDBEngine(engine.path)
    reopened_repository = DocumentRepository(vector_mode="none")
    reopened_repository._storage = _TestStorage(case_root)
    try:
        plan = reopened_repository.plan_document_generation_recovery(
            reopened_engine,
            case_id=_CASE_ID,
            document_id=_FILE_ID,
            file_id=_FILE_ID,
        )
        assert plan.action == expected_action
        applied = reopened_repository.apply_document_generation_recovery(
            reopened_engine,
            case_id=_CASE_ID,
            document_id=_FILE_ID,
            file_id=_FILE_ID,
        )
        assert applied.action == "none"
        if expected_generation == "prior":
            _assert_prior_unchanged(reopened_repository, reopened_engine, prior)
        else:
            current = _snapshot(reopened_repository, reopened_engine)
            assert Path(str(current["content_path"])).read_text(encoding="utf-8") == (
                "replacement generation at directory fsync boundary"
            )
    finally:
        reopened_engine.close()


def test_intent_journal_contains_only_hash_bindings_and_fixed_metadata(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    state["text"] = f"sensitive replacement {_SENSITIVE_ACCOUNT}"

    def crash_at_prepared(phase: str) -> None:
        if phase == "intent_prepared":
            raise _SimulatedProcessCrash

    monkeypatch.setattr(repository, "_document_generation_crash_cut", crash_at_prepared)
    with pytest.raises(_SimulatedProcessCrash):
        _ingest(repository, engine, content_identity="sensitive-source")

    journal_bytes = b"\n".join(
        path.read_bytes()
        for path in _intent_directory(case_root).iterdir()
        if path.suffix == ".json" and path.is_file() and not path.is_symlink()
    )
    for forbidden in (
        _SENSITIVE_ACCOUNT,
        _CASE_ID,
        _FILE_ID,
        "sensitive replacement",
        "/not-read/",
    ):
        assert forbidden.encode("utf-8") not in journal_bytes

    active = json.loads(
        (_intent_directory(case_root) / ".active-intent-v1.json").read_text(encoding="ascii")
    )
    prepared_path = (
        _intent_directory(case_root)
        / f"{active['intentId']}.00-prepared.json"
    )
    prepared_before = prepared_path.read_bytes()
    directory_fd = os.open(_intent_directory(case_root), os.O_RDONLY | os.O_DIRECTORY)
    try:
        with pytest.raises(document_repository_module.ExternalRuntimeError):
            document_repository_module._write_private_document_record(
                directory_fd,
                prepared_path.name,
                {"schema": "replacement-must-not-overwrite"},
            )
    finally:
        os.close(directory_fd)
    assert prepared_path.read_bytes() == prepared_before


def test_reopen_recovery_removes_only_exact_intent_staging_file_without_scan(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    prior_path = prior["content_path"]
    assert isinstance(prior_path, Path)
    state["text"] = "replacement with interrupted staging write"

    def crash_before_publish(phase: str) -> None:
        if phase == "publish_started_recorded":
            raise _SimulatedProcessCrash

    monkeypatch.setattr(repository, "_document_generation_crash_cut", crash_before_publish)
    with pytest.raises(_SimulatedProcessCrash):
        _ingest(repository, engine, content_identity="staging-crash-source")
    active = json.loads(
        (_intent_directory(case_root) / ".active-intent-v1.json").read_text(encoding="ascii")
    )
    prepared_path = _intent_directory(case_root) / (
        f"{active['intentId']}.00-prepared.json"
    )
    prepared = json.loads(prepared_path.read_text(encoding="ascii"))
    staging_name = prepared["newState"]["contentGeneration"]["stagingName"]
    staging_path = prior_path.parent / staging_name
    staging_path.write_bytes(b"partial utf-8 payload")
    staging_path.chmod(0o600)
    unrelated = prior_path.parent / ".document-unrelated.tmp"
    unrelated.write_bytes(b"must remain")
    engine.close()

    reopened_engine = DuckDBEngine(engine.path)
    reopened_repository = DocumentRepository(vector_mode="none")
    reopened_repository._storage = _TestStorage(case_root)
    real_listdir = document_repository_module.os.listdir
    monkeypatch.setattr(
        document_repository_module.os,
        "listdir",
        lambda *_args, **_kwargs: pytest.fail("recovery must use exact staging authority"),
    )
    try:
        assert reopened_repository.apply_document_generation_recovery(
            reopened_engine,
            case_id=_CASE_ID,
            document_id=_FILE_ID,
            file_id=_FILE_ID,
        ).action == "none"
        assert not staging_path.exists()
        assert unrelated.read_bytes() == b"must remain"
        _assert_prior_unchanged(reopened_repository, reopened_engine, prior)
    finally:
        monkeypatch.setattr(document_repository_module.os, "listdir", real_listdir)
        reopened_engine.close()


def test_reopen_recovery_resolves_exact_hardlinked_publish_crash_state(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    prior_path = prior["content_path"]
    assert isinstance(prior_path, Path)
    replacement = "replacement linked but not journaled"
    state["text"] = replacement

    def crash_before_publish(phase: str) -> None:
        if phase == "publish_started_recorded":
            raise _SimulatedProcessCrash

    monkeypatch.setattr(repository, "_document_generation_crash_cut", crash_before_publish)
    with pytest.raises(_SimulatedProcessCrash):
        _ingest(repository, engine, content_identity="hardlink-crash-source")
    active = json.loads(
        (_intent_directory(case_root) / ".active-intent-v1.json").read_text(encoding="ascii")
    )
    prepared = json.loads(
        (
            _intent_directory(case_root)
            / f"{active['intentId']}.00-prepared.json"
        ).read_text(encoding="ascii")
    )
    generation = prepared["newState"]["contentGeneration"]
    staging_path = prior_path.parent / generation["stagingName"]
    target_path = prior_path.parent / generation["targetName"]
    staging_path.write_text(replacement, encoding="utf-8")
    staging_path.chmod(0o600)
    os.link(staging_path, target_path)
    assert staging_path.stat().st_nlink == 2
    engine.close()

    reopened_engine = DuckDBEngine(engine.path)
    reopened_repository = DocumentRepository(vector_mode="none")
    reopened_repository._storage = _TestStorage(case_root)
    try:
        assert reopened_repository.apply_document_generation_recovery(
            reopened_engine,
            case_id=_CASE_ID,
            document_id=_FILE_ID,
            file_id=_FILE_ID,
        ).action == "none"
        assert not staging_path.exists()
        assert not target_path.exists()
        _assert_prior_unchanged(reopened_repository, reopened_engine, prior)
    finally:
        reopened_engine.close()


def test_corrupt_or_symlink_active_intent_is_quarantined_without_mutation(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    state["text"] = "replacement blocked by corrupt intent"

    def crash_at_prepared(phase: str) -> None:
        if phase == "intent_prepared":
            raise _SimulatedProcessCrash

    monkeypatch.setattr(repository, "_document_generation_crash_cut", crash_at_prepared)
    with pytest.raises(_SimulatedProcessCrash):
        _ingest(repository, engine, content_identity="corrupt-source")
    active = _intent_directory(case_root) / ".active-intent-v1.json"
    active.write_bytes(b'{"schema":"corrupt"}')

    plan = repository.plan_document_generation_recovery(
        engine,
        case_id=_CASE_ID,
        document_id=_FILE_ID,
        file_id=_FILE_ID,
    )
    assert plan.action == "quarantine"
    applied = repository.apply_document_generation_recovery(
        engine,
        case_id=_CASE_ID,
        document_id=_FILE_ID,
        file_id=_FILE_ID,
    )
    assert applied.action == "quarantine"
    _assert_prior_unchanged(repository, engine, prior)

    active.unlink()
    attacker = _intent_directory(case_root) / "attacker-active"
    attacker.write_text("attacker", encoding="utf-8")
    active.symlink_to(attacker)
    symlink_plan = repository.plan_document_generation_recovery(
        engine,
        case_id=_CASE_ID,
        document_id=_FILE_ID,
        file_id=_FILE_ID,
    )
    assert symlink_plan.action == "quarantine"
    assert active.is_symlink()
    assert attacker.read_text(encoding="utf-8") == "attacker"
    _assert_prior_unchanged(repository, engine, prior)


def test_wrong_binding_and_phase_gap_are_quarantined_without_directory_scan(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    state["text"] = "replacement with phase chain"

    def crash_after_published(phase: str) -> None:
        if phase == "published_recorded":
            raise _SimulatedProcessCrash

    monkeypatch.setattr(repository, "_document_generation_crash_cut", crash_after_published)
    with pytest.raises(_SimulatedProcessCrash):
        _ingest(repository, engine, content_identity="phase-gap-source")
    wrong_binding = repository.plan_document_generation_recovery(
        engine,
        case_id=_CASE_ID,
        document_id=_FILE_ID,
        file_id="different-file",
    )
    assert wrong_binding.action == "quarantine"

    active_payload = json.loads(
        (_intent_directory(case_root) / ".active-intent-v1.json").read_text(encoding="ascii")
    )
    intent_id = str(active_payload["intentId"])
    publish_started = _intent_directory(case_root) / f"{intent_id}.10-publish_started.json"
    publish_started.unlink()
    real_listdir = document_repository_module.os.listdir
    monkeypatch.setattr(
        document_repository_module.os,
        "listdir",
        lambda *_args, **_kwargs: pytest.fail("recovery must not scan intent directories"),
    )
    gap_plan = repository.plan_document_generation_recovery(
        engine,
        case_id=_CASE_ID,
        document_id=_FILE_ID,
        file_id=_FILE_ID,
    )
    monkeypatch.setattr(document_repository_module.os, "listdir", real_listdir)
    assert gap_plan.action == "quarantine"
    _assert_prior_unchanged(repository, engine, prior)


def test_root_identity_or_nonmatching_database_state_quarantines_without_cleanup(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    state["text"] = "replacement awaiting recovery"

    def crash_after_publish(phase: str) -> None:
        if phase == "text_published":
            raise _SimulatedProcessCrash

    monkeypatch.setattr(repository, "_document_generation_crash_cut", crash_after_publish)
    with pytest.raises(_SimulatedProcessCrash):
        _ingest(repository, engine, content_identity="identity-crash-source")

    case_root.chmod(0o750)
    try:
        assert repository.plan_document_generation_recovery(
            engine,
            case_id=_CASE_ID,
            document_id=_FILE_ID,
            file_id=_FILE_ID,
        ).reason == "document_intent_root_identity_mismatch"
    finally:
        case_root.chmod(0o700)

    engine.execute(
        "UPDATE document_assets SET filename=? WHERE case_id=? AND document_id=?",
        ("tampered-state", _CASE_ID, _FILE_ID),
    )
    plan = repository.plan_document_generation_recovery(
        engine,
        case_id=_CASE_ID,
        document_id=_FILE_ID,
        file_id=_FILE_ID,
    )
    assert plan.action == "quarantine"
    assert plan.reason == "document_intent_database_state_ambiguous"
    assert repository.apply_document_generation_recovery(
        engine,
        case_id=_CASE_ID,
        document_id=_FILE_ID,
        file_id=_FILE_ID,
    ).action == "quarantine"
    prior_path = prior["content_path"]
    assert isinstance(prior_path, Path)
    assert prior_path.exists()
    assert (_intent_directory(case_root) / ".active-intent-v1.json").exists()


def test_concurrent_reingest_serializes_intent_and_leaves_one_consistent_generation(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    database_path = engine.path
    engine.close()
    barrier = threading.Barrier(2)
    results: list[object] = []
    errors: list[BaseException] = []

    def worker(label: str) -> None:
        worker_engine = DuckDBEngine(database_path)
        worker_repository = DocumentRepository(vector_mode="none")
        worker_repository._storage = _TestStorage(case_root)
        monkeypatch.setattr(
            worker_repository,
            "_extract_document",
            lambda **_kwargs: DocumentExtraction(
                segments=[TextSegment(text=f"concurrent generation {label}")],
                parser="fixture:text",
            ),
        )
        try:
            barrier.wait(timeout=5)
            results.append(
                _ingest(
                    worker_repository,
                    worker_engine,
                    content_identity=f"concurrent-{label}",
                )
            )
        except BaseException as exc:
            errors.append(exc)
        finally:
            worker_engine.close()

    threads = [threading.Thread(target=worker, args=(label,)) for label in ("one", "two")]
    for thread in threads:
        thread.start()
    for thread in threads:
        thread.join(timeout=10)
    assert all(not thread.is_alive() for thread in threads)
    assert errors == []
    assert len(results) == 2
    assert all(getattr(result, "llm_ready", False) for result in results)

    final_engine = DuckDBEngine(database_path)
    final_repository = DocumentRepository(vector_mode="none")
    final_repository._storage = _TestStorage(case_root)
    try:
        current = _snapshot(final_repository, final_engine)
        content = Path(str(current["content_path"])).read_text(encoding="utf-8")
        assert content in {"concurrent generation one", "concurrent generation two"}
        assert not (_intent_directory(case_root) / ".active-intent-v1.json").exists()
        assert final_repository.plan_document_generation_recovery(
            final_engine,
            case_id=_CASE_ID,
            document_id=_FILE_ID,
            file_id=_FILE_ID,
        ).action == "none"
    finally:
        final_engine.close()


def test_case_wide_lock_prevents_cross_document_reference_cleanup_race(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior shared generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    prior_path = prior["content_path"]
    assert isinstance(prior_path, Path)
    database_path = engine.path
    engine.close()

    committed = threading.Event()
    release_cleanup = threading.Event()
    second_extracted = threading.Event()
    results: dict[str, object] = {}
    errors: list[BaseException] = []

    def first_worker() -> None:
        worker_engine = DuckDBEngine(database_path)
        worker_repository = DocumentRepository(vector_mode="none")
        worker_repository._storage = _TestStorage(case_root)
        worker_repository._extract_document = lambda **_kwargs: DocumentExtraction(
            segments=[TextSegment(text="first replacement generation")],
            parser="fixture:text",
        )

        def pause_after_commit(phase: str) -> None:
            if phase == "db_commit_visible":
                committed.set()
                if not release_cleanup.wait(timeout=5):
                    raise RuntimeError("test synchronization timeout")

        worker_repository._document_generation_crash_cut = pause_after_commit
        try:
            results["first"] = _ingest(
                worker_repository,
                worker_engine,
                content_identity="first-replacement-source",
            )
        except BaseException as exc:
            errors.append(exc)
        finally:
            worker_engine.close()

    def second_worker() -> None:
        worker_engine = DuckDBEngine(database_path)
        worker_repository = DocumentRepository(vector_mode="none")
        worker_repository._storage = _TestStorage(case_root)

        def extract_second(**_kwargs) -> DocumentExtraction:
            second_extracted.set()
            return DocumentExtraction(
                segments=[TextSegment(text="second independent generation")],
                parser="fixture:text",
            )

        worker_repository._extract_document = extract_second
        try:
            results["second"] = _ingest(
                worker_repository,
                worker_engine,
                content_identity="prior-source",
                file_id="document-b",
            )
        except BaseException as exc:
            errors.append(exc)
        finally:
            worker_engine.close()

    first_thread = threading.Thread(target=first_worker)
    first_thread.start()
    assert committed.wait(timeout=5)
    second_thread = threading.Thread(target=second_worker)
    second_thread.start()
    assert not second_extracted.wait(timeout=0.1)
    release_cleanup.set()
    first_thread.join(timeout=10)
    second_thread.join(timeout=10)

    assert not first_thread.is_alive()
    assert not second_thread.is_alive()
    assert errors == []
    assert getattr(results.get("first"), "llm_ready", False)
    assert getattr(results.get("second"), "llm_ready", False)
    final_engine = DuckDBEngine(database_path)
    try:
        paths = final_engine.query(
            "SELECT document_id, content_path FROM document_assets "
            "WHERE case_id=? ORDER BY document_id",
            (_CASE_ID,),
        )
        assert len(paths) == 2
        assert all(Path(str(path)).is_file() for _document_id, path in paths)
        assert all(str(path) != str(prior_path) for _document_id, path in paths)
        assert not prior_path.exists()
    finally:
        final_engine.close()


def test_apply_recovery_rejects_outer_database_transaction_before_file_cleanup(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    prior_path = prior["content_path"]
    assert isinstance(prior_path, Path)
    state["text"] = "replacement committed before recovery"

    def crash_after_commit(phase: str) -> None:
        if phase == "db_commit_visible":
            raise _SimulatedProcessCrash(phase)

    monkeypatch.setattr(repository, "_document_generation_crash_cut", crash_after_commit)
    with pytest.raises(_SimulatedProcessCrash):
        _ingest(repository, engine, content_identity="replacement-outer-transaction")
    monkeypatch.setattr(repository, "_document_generation_crash_cut", lambda _phase: None)
    active_path = _intent_directory(case_root) / ".active-intent-v1.json"
    assert active_path.exists()
    journal_before = _journal_snapshot(_intent_directory(case_root))
    current_before = _snapshot(repository, engine)
    current_path = current_before["content_path"]
    assert isinstance(current_path, Path)
    assert current_path.exists()
    assert prior_path.exists()

    engine.execute("BEGIN TRANSACTION")
    try:
        blocked = repository.apply_document_generation_recovery(
            engine,
            case_id=_CASE_ID,
            document_id=_FILE_ID,
            file_id=_FILE_ID,
        )
        assert blocked.action == "quarantine"
        assert blocked.reason == "document_intent_recovery_blocked"
        assert _journal_snapshot(_intent_directory(case_root)) == journal_before
        assert current_path.exists()
        assert prior_path.exists()
    finally:
        engine.execute("ROLLBACK")

    applied = repository.apply_document_generation_recovery(
        engine,
        case_id=_CASE_ID,
        document_id=_FILE_ID,
        file_id=_FILE_ID,
    )
    assert applied.action == "none"
    assert not active_path.exists()
    assert current_path.exists()
    assert not prior_path.exists()


def test_commit_acknowledgement_loss_returns_verified_committed_result(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    prior_path = prior["content_path"]
    assert isinstance(prior_path, Path)
    state["text"] = "replacement with lost commit acknowledgement"
    real_execute = engine.execute
    commit_ack_lost = False

    def execute_with_lost_commit_ack(sql: str, params=None) -> None:
        nonlocal commit_ack_lost
        real_execute(sql, params)
        if str(sql).strip().upper() == "COMMIT" and not commit_ack_lost:
            commit_ack_lost = True
            raise RuntimeError("commit acknowledgement lost")

    monkeypatch.setattr(engine, "execute", execute_with_lost_commit_ack)
    result = _ingest(repository, engine, content_identity="replacement-commit-ack")

    assert commit_ack_lost is True
    assert result.llm_ready is True
    assert result.last_error == ""
    current = _snapshot(repository, engine)
    current_path = current["content_path"]
    assert isinstance(current_path, Path)
    assert current_path.read_text(encoding="utf-8") == (
        "replacement with lost commit acknowledgement"
    )
    assert not prior_path.exists()
    assert not (_intent_directory(case_root) / ".active-intent-v1.json").exists()


def test_duplicate_commit_acknowledgement_loss_returns_verified_duplicate_result(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "shared accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="shared-source").llm_ready is True
    real_execute = engine.execute
    commit_ack_lost = False

    def execute_with_lost_commit_ack(sql: str, params=None) -> None:
        nonlocal commit_ack_lost
        real_execute(sql, params)
        if str(sql).strip().upper() == "COMMIT" and not commit_ack_lost:
            commit_ack_lost = True
            raise RuntimeError("commit acknowledgement lost")

    monkeypatch.setattr(engine, "execute", execute_with_lost_commit_ack)
    result = _ingest(
        repository,
        engine,
        content_identity="shared-source",
        file_id="document-b",
    )

    assert commit_ack_lost is True
    assert result.llm_ready is True
    assert result.extraction_status == "duplicate"
    assert result.duplicate_of_document_id == _FILE_ID
    duplicate = repository._get_asset_row(
        engine,
        case_id=_CASE_ID,
        document_id="document-b",
    )
    assert duplicate is not None
    assert duplicate["duplicate_of_document_id"] == _FILE_ID
    assert not (_intent_directory(case_root, "document-b") / ".active-intent-v1.json").exists()


def test_commit_and_rollback_failure_leave_intent_for_restart_recovery(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    prior = _snapshot(repository, engine)
    prior_path = prior["content_path"]
    assert isinstance(prior_path, Path)
    state["text"] = "replacement trapped in uncommitted transaction"
    real_execute = engine.execute
    commit_failed = False
    rollback_failed = False

    def execute_with_unresolved_transaction(sql: str, params=None) -> None:
        nonlocal commit_failed, rollback_failed
        command = str(sql).strip().upper()
        if command == "COMMIT" and not commit_failed:
            commit_failed = True
            raise RuntimeError("commit failed before application")
        if command == "ROLLBACK" and commit_failed and not rollback_failed:
            rollback_failed = True
            raise RuntimeError("rollback acknowledgement unavailable")
        real_execute(sql, params)

    monkeypatch.setattr(engine, "execute", execute_with_unresolved_transaction)
    result = _ingest(repository, engine, content_identity="replacement-unresolved-txn")

    assert commit_failed is True
    assert rollback_failed is True
    assert result.llm_ready is False
    assert result.last_error == "document_database_state_indeterminate"
    active_path = _intent_directory(case_root) / ".active-intent-v1.json"
    assert active_path.exists()
    assert prior_path.exists()
    active = json.loads(active_path.read_text(encoding="ascii"))
    terminal_path = _intent_directory(case_root) / f"{active['intentId']}.50-terminal.json"
    assert not terminal_path.exists()

    engine.close()
    reopened_engine = DuckDBEngine(engine.path)
    reopened_repository = DocumentRepository(vector_mode="none")
    reopened_repository._storage = _TestStorage(case_root)
    try:
        plan = reopened_repository.plan_document_generation_recovery(
            reopened_engine,
            case_id=_CASE_ID,
            document_id=_FILE_ID,
            file_id=_FILE_ID,
        )
        assert plan.action == "rollback_new"
        applied = reopened_repository.apply_document_generation_recovery(
            reopened_engine,
            case_id=_CASE_ID,
            document_id=_FILE_ID,
            file_id=_FILE_ID,
        )
        assert applied.action == "none"
        _assert_prior_unchanged(reopened_repository, reopened_engine, prior)
        assert not active_path.exists()
        assert terminal_path.exists()
        assert sorted(prior_path.parent.glob("*.txt")) == [prior_path]
    finally:
        reopened_engine.close()


def test_delete_by_file_id_is_blocked_before_database_or_generation_side_effects(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "accepted generation must survive blocked delete"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="delete-gate-source").llm_ready is True
    prior = _snapshot(repository, engine)
    journal_before = _journal_snapshot(_intent_directory(case_root))

    def reject_delete() -> None:
        raise document_repository_module.ExternalRuntimeError(
            "controlled_source_ingestion_required"
        )

    monkeypatch.setattr(
        document_repository_module,
        "require_controlled_source_ingestion",
        reject_delete,
    )
    with pytest.raises(
        document_repository_module.ExternalRuntimeError,
        match="controlled_source_ingestion_required",
    ):
        repository.delete_by_file_id(engine, case_id=_CASE_ID, file_id=_FILE_ID)

    _assert_prior_unchanged(repository, engine, prior)
    assert _journal_snapshot(_intent_directory(case_root)) == journal_before


def test_ingest_uses_only_locked_recovery_plan(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, _case_root = document_runtime
    state: dict[str, object] = {"text": "locked recovery plan only"}
    _install_extraction(repository, monkeypatch, state)
    monkeypatch.setattr(
        repository,
        "plan_document_generation_recovery",
        lambda *_args, **_kwargs: pytest.fail("ingest used an unlocked recovery plan"),
    )

    result = _ingest(repository, engine, content_identity="locked-plan-source")

    assert result.llm_ready is True


def test_selection_change_does_not_quarantine_or_get_lost_during_reingest(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    state["text"] = "replacement after concurrent selection"
    selection_written = False

    def select_during_ingest(phase: str) -> None:
        nonlocal selection_written
        if phase == "db_started_recorded" and not selection_written:
            engine.execute(
                "UPDATE document_assets SET selected_for_llm=TRUE "
                "WHERE case_id=? AND document_id=?",
                (_CASE_ID, _FILE_ID),
            )
            selection_written = True

    monkeypatch.setattr(repository, "_document_generation_crash_cut", select_during_ingest)
    result = _ingest(repository, engine, content_identity="replacement-selection")

    assert result.llm_ready is True
    assert selection_written is True
    asset = repository._get_asset_row(engine, case_id=_CASE_ID, document_id=_FILE_ID)
    assert asset is not None
    assert asset["selected_for_llm"] is True
    assert not (_intent_directory(case_root) / ".active-intent-v1.json").exists()


@pytest.mark.parametrize(
    ("crash_cut", "expected_action"),
    (("published_recorded", "rollback_new"), ("db_commit_visible", "finalize_new")),
)
def test_selection_change_does_not_invalidate_stale_intent_recovery(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
    crash_cut: str,
    expected_action: str,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    state["text"] = f"replacement for {expected_action}"

    def crash_at(phase: str) -> None:
        if phase == crash_cut:
            raise _SimulatedProcessCrash(phase)

    monkeypatch.setattr(repository, "_document_generation_crash_cut", crash_at)
    with pytest.raises(_SimulatedProcessCrash):
        _ingest(repository, engine, content_identity=f"selection-{expected_action}")
    monkeypatch.setattr(repository, "_document_generation_crash_cut", lambda _phase: None)
    engine.execute(
        "UPDATE document_assets SET selected_for_llm=TRUE "
        "WHERE case_id=? AND document_id=?",
        (_CASE_ID, _FILE_ID),
    )

    plan = repository.plan_document_generation_recovery(
        engine,
        case_id=_CASE_ID,
        document_id=_FILE_ID,
        file_id=_FILE_ID,
    )
    assert plan.action == expected_action
    assert repository.apply_document_generation_recovery(
        engine,
        case_id=_CASE_ID,
        document_id=_FILE_ID,
        file_id=_FILE_ID,
    ).action == "none"
    asset = repository._get_asset_row(engine, case_id=_CASE_ID, document_id=_FILE_ID)
    assert asset is not None
    assert asset["selected_for_llm"] is True
    assert not (_intent_directory(case_root) / ".active-intent-v1.json").exists()


def test_recovery_holds_database_operation_lock_through_terminal_cleanup(
    document_runtime,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository, engine, case_root = document_runtime
    state: dict[str, object] = {"text": "prior accepted generation"}
    _install_extraction(repository, monkeypatch, state)
    assert _ingest(repository, engine, content_identity="prior-source").llm_ready is True
    state["text"] = "replacement awaiting serialized recovery"

    def crash_after_commit(phase: str) -> None:
        if phase == "db_commit_visible":
            raise _SimulatedProcessCrash(phase)

    monkeypatch.setattr(repository, "_document_generation_crash_cut", crash_after_commit)
    with pytest.raises(_SimulatedProcessCrash):
        _ingest(repository, engine, content_identity="serialized-recovery")
    monkeypatch.setattr(repository, "_document_generation_crash_cut", lambda _phase: None)

    checked = threading.Event()
    release_recovery = threading.Event()
    transaction_started = threading.Event()
    real_check = repository._document_engine_transaction_is_active
    paused = False

    def check_and_pause(checked_engine: DuckDBEngine) -> bool:
        nonlocal paused
        result = real_check(checked_engine)
        if not paused:
            paused = True
            checked.set()
            if not release_recovery.wait(timeout=5):
                raise RuntimeError("test synchronization timeout")
        return result

    monkeypatch.setattr(repository, "_document_engine_transaction_is_active", check_and_pause)
    recovery_results: list[DocumentGenerationRecoveryPlan] = []
    errors: list[BaseException] = []

    def recover() -> None:
        try:
            recovery_results.append(repository.apply_document_generation_recovery(
                engine,
                case_id=_CASE_ID,
                document_id=_FILE_ID,
                file_id=_FILE_ID,
            ))
        except BaseException as exc:
            errors.append(exc)

    def begin_transaction() -> None:
        try:
            engine.execute("BEGIN TRANSACTION")
            transaction_started.set()
            engine.execute("ROLLBACK")
        except BaseException as exc:
            errors.append(exc)

    recovery_thread = threading.Thread(target=recover)
    recovery_thread.start()
    assert checked.wait(timeout=5)
    transaction_thread = threading.Thread(target=begin_transaction)
    transaction_thread.start()
    assert not transaction_started.wait(timeout=0.1)
    release_recovery.set()
    recovery_thread.join(timeout=10)
    transaction_thread.join(timeout=10)

    assert not recovery_thread.is_alive()
    assert not transaction_thread.is_alive()
    assert errors == []
    assert len(recovery_results) == 1
    assert recovery_results[0].action == "none"
    assert transaction_started.is_set()
    assert not (_intent_directory(case_root) / ".active-intent-v1.json").exists()
