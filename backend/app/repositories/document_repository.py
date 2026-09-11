from __future__ import annotations

import hashlib
import io
import json
import math
import os
import re
import secrets
import stat
import tempfile
import zipfile
from dataclasses import dataclass, replace
from datetime import datetime
from pathlib import Path
from typing import Any, Iterable, List, Optional
from xml.etree import ElementTree as ET

from pypdf import PdfReader

from app.core.case_project_doc import (
    CASE_DIRECTION_DOC_DOCUMENT_ID,
    CASE_DIRECTION_DOC_FILE_ID,
    CASE_DIRECTION_DOC_FILENAME,
    CASE_PROJECT_DOC_DOCUMENT_ID,
    CASE_PROJECT_DOC_FILE_ID,
    CASE_PROJECT_DOC_FILENAME,
    CASE_PROJECT_DOC_KIND,
    CASE_PROJECT_DOC_PARSER,
    case_direction_doc_path,
    case_project_doc_path,
    read_case_direction_doc,
    read_case_project_doc,
)
from app.core.db_engine import DuckDBEngine
from app.core.diagnostic_duckdb_migration import DiagnosticDuckDBMigrationError
from app.core.external_runtime import (
    ExternalRuntimeError,
    HostRuntimeResolution,
    resolve_host_runtime_binary,
)
from app.core.execution_authority_health import (
    ExecutionAuthorityHealth,
    execution_authority_health,
    unadmitted_execution_capability_health,
)
from app.core.fc_import_file_identity import detect_file_encoding
from app.domain.controlled_artifact_gate import require_controlled_source_ingestion
from app.core.managed_subprocess import (
    ManagedProcessLaunchError,
    ManagedProcessOutputLimit,
    ManagedProcessTerminationError,
    ManagedProcessTimeout,
    run_managed_process,
)
from app.core.storage import CaseStorage


_PUNCTUATION_BOUNDARY_RE = re.compile(r"[\n。！？；.!?;，,、]")
_WHITESPACE_RE = re.compile(r"[ \t\r\f\v]+")
_DOCX_NS = {"w": "http://schemas.openxmlformats.org/wordprocessingml/2006/main"}
_OCR_LANGUAGE_RE = re.compile(r"^[A-Za-z0-9_.+-]{1,64}$")
_DOCUMENT_SOFFICE_ENV = "ANALYTIX_DOCUMENT_SOFFICE_BIN"
_DOCUMENT_TEXTUTIL_ENV = "ANALYTIX_DOCUMENT_TEXTUTIL_BIN"
_DOCUMENT_ANTIWORD_ENV = "ANALYTIX_DOCUMENT_ANTIWORD_BIN"
_DOCUMENT_TESSERACT_ENV = "ANALYTIX_DOCUMENT_TESSERACT_BIN"
_DOCUMENT_GENERATION_NAME_RE = re.compile(r"^[0-9a-f]{64}\.txt$")
_DOCUMENT_INTENT_ID_RE = re.compile(r"^[0-9a-f]{64}$")
_DOCUMENT_TEMPORARY_NAME_RE = re.compile(
    r"^\.(?:generation-stage-v1|document)-[0-9a-f]{48}\.tmp$"
)
_DOCUMENT_INTENT_SCHEMA = "DocumentGenerationIntentV1"
_DOCUMENT_INTENT_ACTIVE_SCHEMA = "DocumentGenerationIntentActiveV1"
_DOCUMENT_INTENT_PHASES = (
    (10, "publish_started"),
    (20, "published"),
    (30, "db_started"),
    (40, "db_committed"),
)
_DOCUMENT_INTENT_TERMINAL_SEQUENCE = 50
_DOCUMENT_ASSET_DIGEST_FIELDS = (
    "document_id",
    "case_id",
    "file_id",
    "kind",
    "filename",
    "display_path",
    "stored_path",
    "file_type",
    "size",
    "md5",
    "sha256",
    "duplicate_of_document_id",
    "duplicate_reason",
    "llm_ready",
    "parser",
    "extraction_status",
    "ocr_status",
    "vector_status",
    "vector_model",
    "content_path",
    "content_excerpt",
    "content_chars",
    "page_count",
    "chunk_count",
    "token_estimate",
    "last_error",
    "created_at",
    "updated_at",
    "indexed_at",
)
_DOCUMENT_CHUNK_DIGEST_FIELDS = (
    "chunk_id",
    "case_id",
    "document_id",
    "file_id",
    "chunk_index",
    "page_from",
    "page_to",
    "text",
    "text_chars",
    "token_estimate",
    "vector_model",
    "vector_dim",
    "vector_data",
    "created_at",
)


@dataclass
class TextSegment:
    text: str
    page_from: Optional[int] = None
    page_to: Optional[int] = None


@dataclass
class DocumentExtraction:
    segments: List[TextSegment]
    parser: str
    page_count: int = 0
    ocr_status: str = "not_needed"
    last_error: str = ""
    unsupported: bool = False


@dataclass
class DocumentIngestResult:
    document_id: str
    file_id: str
    extraction_status: str
    ocr_status: str
    vector_status: str
    chunk_count: int
    llm_ready: bool
    duplicate_of_document_id: str = ""
    last_error: str = ""
    note: str = ""


@dataclass(frozen=True)
class _DocumentTextGeneration:
    path: Path
    document_key: str
    target_name: str
    sha256: str
    created: bool


class _DocumentDatabaseMutationError(RuntimeError):
    def __init__(self, code: str, *, safe_to_remove_new_generation: bool) -> None:
        self.code = str(code)
        self.safe_to_remove_new_generation = bool(safe_to_remove_new_generation)
        super().__init__(self.code)


class DocumentPublicMetadataUnavailableError(RuntimeError):
    """The read-only public metadata source cannot establish authoritative state."""


@dataclass(frozen=True)
class _DocumentIntentJournalState:
    active_sha256: str
    intent_id: str
    prepared: dict[str, Any]
    prepared_sha256: str
    latest_phase: str
    latest_sequence: int
    latest_sha256: str
    terminal_outcome: str


@dataclass(frozen=True)
class DocumentGenerationRecoveryPlan:
    action: str
    reason: str = ""
    intent_id: str = ""
    phase: str = ""
    state: _DocumentIntentJournalState | None = None


def _now_iso() -> str:
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")


def _resolved_document_runtime(env_name: str, allowed_names: set[str]) -> HostRuntimeResolution:
    _require_document_execution_capability()
    return resolve_host_runtime_binary(
        env_name,
        allowed_names=allowed_names,
        require_document_manifest=True,
    )


def _document_execution_capability_health() -> ExecutionAuthorityHealth:
    return unadmitted_execution_capability_health(execution_authority_health())


def _require_document_execution_capability() -> None:
    authority = _document_execution_capability_health()
    if not authority.available:
        raise ExternalRuntimeError(authority.reason_code)


def _copy_regular_source(source: Path, target: Path) -> str:
    source_fd = -1
    target_fd = -1
    digest = hashlib.sha256()
    copied_bytes = 0
    try:
        before = source.lstat()
        source_fd = os.open(
            source,
            os.O_RDONLY | int(getattr(os, "O_CLOEXEC", 0)) | int(getattr(os, "O_NOFOLLOW", 0)),
        )
        opened = os.fstat(source_fd)
        if (
            stat.S_ISLNK(before.st_mode)
            or not stat.S_ISREG(before.st_mode)
            or not stat.S_ISREG(opened.st_mode)
            or int(opened.st_nlink) != 1
            or (int(before.st_dev), int(before.st_ino)) != (int(opened.st_dev), int(opened.st_ino))
        ):
            raise OSError
        target_fd = os.open(
            target,
            os.O_WRONLY
            | os.O_CREAT
            | os.O_EXCL
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0)),
            0o600,
        )
        target_opened = os.fstat(target_fd)
        if not stat.S_ISREG(target_opened.st_mode) or int(target_opened.st_nlink) != 1:
            raise OSError
        while True:
            chunk = os.read(source_fd, 1024 * 1024)
            if not chunk:
                break
            digest.update(chunk)
            _write_document_fd(target_fd, chunk)
            copied_bytes += len(chunk)
        os.fsync(target_fd)
        source_after = os.fstat(source_fd)
        source_path_after = source.lstat()
        target_after = os.fstat(target_fd)
        target_path_after = target.lstat()
        if (
            _document_file_identity(opened) != _document_file_identity(source_after)
            or _document_file_identity(source_after) != _document_file_identity(source_path_after)
            or _document_mutable_identity(opened) != _document_mutable_identity(source_after)
            or _document_file_identity(target_opened) != _document_file_identity(target_after)
            or _document_file_identity(target_after) != _document_file_identity(target_path_after)
            or copied_bytes != int(target_after.st_size)
        ):
            raise OSError
    except OSError:
        raise ExternalRuntimeError("document_source_unavailable") from None
    finally:
        if source_fd >= 0:
            os.close(source_fd)
        if target_fd >= 0:
            os.close(target_fd)
    return digest.hexdigest()


def _safe_document_error_code(exc: BaseException) -> str:
    if isinstance(exc, _DocumentDatabaseMutationError):
        return exc.code
    if isinstance(exc, ExternalRuntimeError):
        return exc.code
    if isinstance(exc, FileNotFoundError):
        return "document_source_unavailable"
    return "document_extract_failed"


def _safe_ocr_error_code(exc: BaseException) -> str:
    if isinstance(exc, ExternalRuntimeError) and exc.code.startswith("ocr_"):
        return exc.code
    return "ocr_failed"


def _normalized_document_sha256(value: str) -> str:
    normalized = str(value or "").strip().lower()
    if len(normalized) != 64 or any(character not in "0123456789abcdef" for character in normalized):
        raise ExternalRuntimeError("document_source_identity_invalid")
    return normalized


def _regular_file_sha256(path: Path, *, missing_code: str) -> str:
    descriptor = -1
    digest = hashlib.sha256()
    try:
        before = path.lstat()
    except OSError:
        raise ExternalRuntimeError(missing_code) from None
    try:
        descriptor = os.open(
            path,
            os.O_RDONLY | int(getattr(os, "O_CLOEXEC", 0)) | int(getattr(os, "O_NOFOLLOW", 0)),
        )
        opened = os.fstat(descriptor)
        if (
            stat.S_ISLNK(before.st_mode)
            or not stat.S_ISREG(before.st_mode)
            or not stat.S_ISREG(opened.st_mode)
            or int(opened.st_nlink) != 1
            or _document_file_identity(before) != _document_file_identity(opened)
        ):
            raise OSError
        while True:
            chunk = os.read(descriptor, 1024 * 1024)
            if not chunk:
                break
            digest.update(chunk)
        opened_after = os.fstat(descriptor)
        path_after = path.lstat()
        if (
            _document_file_identity(opened) != _document_file_identity(opened_after)
            or _document_file_identity(opened_after) != _document_file_identity(path_after)
            or _document_mutable_identity(opened) != _document_mutable_identity(opened_after)
        ):
            raise OSError
    except OSError:
        raise ExternalRuntimeError("document_output_invalid") from None
    finally:
        if descriptor >= 0:
            os.close(descriptor)
    return digest.hexdigest()


def _open_private_document_target_directory(case_root: Path, source_sha256: str) -> int:
    if os.name == "nt":
        raise ExternalRuntimeError("document_publish_platform_unavailable")
    raw = os.fspath(case_root)
    if not raw or "\x00" in raw or not os.path.isabs(raw):
        raise ExternalRuntimeError("document_publish_root_untrusted")
    absolute = Path(os.path.abspath(raw))
    try:
        canonical = absolute.resolve(strict=True)
    except (OSError, RuntimeError):
        raise ExternalRuntimeError("document_publish_root_untrusted") from None
    if os.path.normcase(os.fspath(absolute)) != os.path.normcase(os.fspath(canonical)):
        raise ExternalRuntimeError("document_publish_root_untrusted")
    current = Path(absolute.anchor)
    try:
        for part in absolute.parts[1:]:
            current = current / part
            if stat.S_ISLNK(current.lstat().st_mode):
                raise OSError
    except OSError:
        raise ExternalRuntimeError("document_publish_root_untrusted") from None

    descriptor = -1
    try:
        descriptor = os.open(
            absolute,
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0))
            | int(getattr(os, "O_DIRECTORY", 0)),
        )
        root_stat = os.fstat(descriptor)
        get_euid = getattr(os, "geteuid", None)
        if (
            not stat.S_ISDIR(root_stat.st_mode)
            or (get_euid is not None and int(root_stat.st_uid) != int(get_euid()))
        ):
            raise OSError
        for name in ("knowledge", "_docconvert", source_sha256):
            child = _open_or_create_private_child_directory(descriptor, name)
            os.close(descriptor)
            descriptor = child
        return descriptor
    except ExternalRuntimeError:
        if descriptor >= 0:
            os.close(descriptor)
        raise
    except OSError:
        if descriptor >= 0:
            os.close(descriptor)
        raise ExternalRuntimeError("document_publish_root_untrusted") from None


def _open_or_create_private_child_directory(parent_fd: int, name: str) -> int:
    if not name or Path(name).name != name or name in {".", ".."}:
        raise ExternalRuntimeError("document_publish_root_untrusted")
    try:
        os.mkdir(name, mode=0o700, dir_fd=parent_fd)
        os.fsync(parent_fd)
    except FileExistsError:
        pass
    descriptor = -1
    try:
        descriptor = os.open(
            name,
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0))
            | int(getattr(os, "O_DIRECTORY", 0)),
            dir_fd=parent_fd,
        )
        opened = os.fstat(descriptor)
        path_stat = os.stat(name, dir_fd=parent_fd, follow_symlinks=False)
        get_euid = getattr(os, "geteuid", None)
        if (
            not stat.S_ISDIR(opened.st_mode)
            or stat.S_ISLNK(path_stat.st_mode)
            or _document_file_identity(opened) != _document_file_identity(path_stat)
            or (get_euid is not None and int(opened.st_uid) != int(get_euid()))
        ):
            raise OSError
        os.fchmod(descriptor, 0o700)
        os.fsync(descriptor)
        return descriptor
    except OSError:
        if descriptor >= 0:
            os.close(descriptor)
        raise ExternalRuntimeError("document_publish_root_untrusted") from None


def _document_target_digest(directory_fd: int, name: str) -> str | None:
    descriptor = -1
    try:
        path_stat = os.stat(name, dir_fd=directory_fd, follow_symlinks=False)
    except FileNotFoundError:
        return None
    except OSError:
        raise ExternalRuntimeError("document_publish_failed") from None
    try:
        descriptor = os.open(
            name,
            os.O_RDONLY | int(getattr(os, "O_CLOEXEC", 0)) | int(getattr(os, "O_NOFOLLOW", 0)),
            dir_fd=directory_fd,
        )
        opened = os.fstat(descriptor)
        get_euid = getattr(os, "geteuid", None)
        if (
            stat.S_ISLNK(path_stat.st_mode)
            or not stat.S_ISREG(path_stat.st_mode)
            or not stat.S_ISREG(opened.st_mode)
            or int(opened.st_nlink) != 1
            or _document_file_identity(path_stat) != _document_file_identity(opened)
            or (stat.S_IMODE(opened.st_mode) & 0o077) != 0
            or (get_euid is not None and int(opened.st_uid) != int(get_euid()))
        ):
            raise OSError
        digest = hashlib.sha256()
        while True:
            chunk = os.read(descriptor, 1024 * 1024)
            if not chunk:
                break
            digest.update(chunk)
        opened_after = os.fstat(descriptor)
        path_after = os.stat(name, dir_fd=directory_fd, follow_symlinks=False)
        if (
            _document_file_identity(opened) != _document_file_identity(opened_after)
            or _document_file_identity(opened_after) != _document_file_identity(path_after)
            or _document_mutable_identity(opened) != _document_mutable_identity(opened_after)
        ):
            raise OSError
        return digest.hexdigest()
    except OSError:
        raise ExternalRuntimeError("document_publish_failed") from None
    finally:
        if descriptor >= 0:
            os.close(descriptor)


def _cleanup_document_temporaries(directory_fd: int) -> None:
    try:
        names = os.listdir(directory_fd)
    except OSError:
        raise ExternalRuntimeError("document_publish_failed") from None
    for name in names:
        if not _DOCUMENT_TEMPORARY_NAME_RE.fullmatch(name):
            continue
        descriptor = -1
        try:
            path_stat = os.stat(name, dir_fd=directory_fd, follow_symlinks=False)
            descriptor = os.open(
                name,
                os.O_RDONLY | int(getattr(os, "O_CLOEXEC", 0)) | int(getattr(os, "O_NOFOLLOW", 0)),
                dir_fd=directory_fd,
            )
            opened = os.fstat(descriptor)
            get_euid = getattr(os, "geteuid", None)
            if (
                stat.S_ISLNK(path_stat.st_mode)
                or not stat.S_ISREG(path_stat.st_mode)
                or not stat.S_ISREG(opened.st_mode)
                or _document_file_identity(path_stat) != _document_file_identity(opened)
                or (get_euid is not None and int(opened.st_uid) != int(get_euid()))
            ):
                raise OSError
            os.unlink(name, dir_fd=directory_fd)
        except OSError:
            raise ExternalRuntimeError("document_publish_failed") from None
        finally:
            if descriptor >= 0:
                os.close(descriptor)
    os.fsync(directory_fd)


def _open_document_publish_lock(directory_fd: int) -> int:
    import fcntl

    descriptor = -1
    try:
        descriptor = os.open(
            ".analytix-document-publish.lock",
            os.O_RDWR
            | os.O_CREAT
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0)),
            0o600,
            dir_fd=directory_fd,
        )
        opened = os.fstat(descriptor)
        get_euid = getattr(os, "geteuid", None)
        if (
            not stat.S_ISREG(opened.st_mode)
            or int(opened.st_nlink) != 1
            or (get_euid is not None and int(opened.st_uid) != int(get_euid()))
        ):
            raise OSError
        os.fchmod(descriptor, 0o600)
        fcntl.flock(descriptor, fcntl.LOCK_EX)
        return descriptor
    except OSError:
        if descriptor >= 0:
            os.close(descriptor)
        raise ExternalRuntimeError("document_publish_failed") from None


def _open_existing_document_publish_lock(directory_fd: int) -> int:
    import fcntl

    descriptor = -1
    try:
        descriptor = os.open(
            ".analytix-document-publish.lock",
            os.O_RDWR
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0)),
            dir_fd=directory_fd,
        )
        opened = os.fstat(descriptor)
        get_euid = getattr(os, "geteuid", None)
        if (
            not stat.S_ISREG(opened.st_mode)
            or int(opened.st_nlink) != 1
            or (stat.S_IMODE(opened.st_mode) & 0o077) != 0
            or (get_euid is not None and int(opened.st_uid) != int(get_euid()))
        ):
            raise OSError
        fcntl.flock(descriptor, fcntl.LOCK_EX)
        return descriptor
    except OSError:
        if descriptor >= 0:
            os.close(descriptor)
        raise ExternalRuntimeError("document_intent_recovery_blocked") from None


def _copy_regular_document_output(source: Path, target_fd: int) -> str:
    source_fd = -1
    digest = hashlib.sha256()
    try:
        before = source.lstat()
        source_fd = os.open(
            source,
            os.O_RDONLY | int(getattr(os, "O_CLOEXEC", 0)) | int(getattr(os, "O_NOFOLLOW", 0)),
        )
        opened = os.fstat(source_fd)
        if (
            stat.S_ISLNK(before.st_mode)
            or not stat.S_ISREG(before.st_mode)
            or not stat.S_ISREG(opened.st_mode)
            or int(opened.st_nlink) != 1
            or _document_file_identity(before) != _document_file_identity(opened)
        ):
            raise OSError
        while True:
            chunk = os.read(source_fd, 1024 * 1024)
            if not chunk:
                break
            digest.update(chunk)
            _write_document_fd(target_fd, chunk)
        opened_after = os.fstat(source_fd)
        path_after = source.lstat()
        if (
            _document_file_identity(opened) != _document_file_identity(opened_after)
            or _document_file_identity(opened_after) != _document_file_identity(path_after)
            or _document_mutable_identity(opened) != _document_mutable_identity(opened_after)
        ):
            raise OSError
        return digest.hexdigest()
    except OSError:
        raise ExternalRuntimeError("document_output_invalid") from None
    finally:
        if source_fd >= 0:
            os.close(source_fd)


def _write_document_fd(descriptor: int, payload: bytes) -> None:
    offset = 0
    while offset < len(payload):
        written = os.write(descriptor, payload[offset:])
        if written <= 0:
            raise OSError
        offset += int(written)


def _document_file_identity(value: os.stat_result) -> tuple[int, int]:
    return int(value.st_dev), int(value.st_ino)


def _document_mutable_identity(value: os.stat_result) -> tuple[int, int, int]:
    return (
        int(value.st_size),
        int(getattr(value, "st_mtime_ns", int(value.st_mtime * 1_000_000_000))),
        int(getattr(value, "st_ctime_ns", int(value.st_ctime * 1_000_000_000))),
    )


def _document_text_storage_key(document_id: str) -> str:
    try:
        payload = str(document_id).encode("utf-8", errors="strict")
    except (UnicodeError, MemoryError):
        raise ExternalRuntimeError("document_identity_invalid") from None
    return hashlib.sha256(b"analytix-document-text-v1\0" + payload).hexdigest()


def _document_binding_hash(kind: str, value: str) -> str:
    try:
        payload = str(value).encode("utf-8", errors="strict")
    except (UnicodeError, MemoryError):
        raise ExternalRuntimeError("document_identity_invalid") from None
    return hashlib.sha256(
        b"analytix-document-generation-binding-v1\0"
        + str(kind).encode("ascii", errors="strict")
        + b"\0"
        + payload
    ).hexdigest()


def _canonical_document_json(value: Any) -> bytes:
    try:
        return json.dumps(
            value,
            ensure_ascii=True,
            sort_keys=True,
            separators=(",", ":"),
            allow_nan=False,
        ).encode("ascii", errors="strict")
    except (TypeError, ValueError, UnicodeError, MemoryError):
        raise ExternalRuntimeError("document_intent_invalid") from None


def _document_json_sha256(value: Any) -> str:
    return hashlib.sha256(_canonical_document_json(value)).hexdigest()


def _document_case_root_identity(case_root: Path) -> dict[str, Any]:
    raw = os.fspath(case_root)
    if not raw or "\x00" in raw or not os.path.isabs(raw):
        raise ExternalRuntimeError("document_publish_root_untrusted")
    absolute = Path(os.path.abspath(raw))
    descriptor = -1
    try:
        canonical = absolute.resolve(strict=True)
        if os.path.normcase(os.fspath(absolute)) != os.path.normcase(os.fspath(canonical)):
            raise OSError
        current = Path(absolute.anchor)
        for part in absolute.parts[1:]:
            current = current / part
            if stat.S_ISLNK(current.lstat().st_mode):
                raise OSError
        descriptor = os.open(
            absolute,
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0))
            | int(getattr(os, "O_DIRECTORY", 0)),
        )
        opened = os.fstat(descriptor)
        get_euid = getattr(os, "geteuid", None)
        if (
            not stat.S_ISDIR(opened.st_mode)
            or (get_euid is not None and int(opened.st_uid) != int(get_euid()))
        ):
            raise OSError
        return {
            "pathSha256": _document_binding_hash("case-root", os.fspath(absolute)),
            "device": int(opened.st_dev),
            "inode": int(opened.st_ino),
            "ownerUid": int(opened.st_uid),
            "mode": int(stat.S_IMODE(opened.st_mode)),
        }
    except (OSError, RuntimeError):
        raise ExternalRuntimeError("document_publish_root_untrusted") from None
    finally:
        if descriptor >= 0:
            os.close(descriptor)


def _open_private_document_intent_directory(
    case_root: Path,
    document_key: str | None,
    *,
    create: bool,
) -> tuple[int, Path] | None:
    if document_key is not None and (
        len(document_key) != 64
        or any(character not in "0123456789abcdef" for character in document_key)
    ):
        raise ExternalRuntimeError("document_identity_invalid")
    raw = os.fspath(case_root)
    if not raw or "\x00" in raw or not os.path.isabs(raw):
        raise ExternalRuntimeError("document_publish_root_untrusted")
    absolute = Path(os.path.abspath(raw))
    directory_path = absolute / "knowledge" / "document-intents-v1"
    if document_key is not None:
        directory_path = directory_path / document_key
    if not create:
        try:
            directory_path.lstat()
        except FileNotFoundError:
            return None
        except OSError:
            raise ExternalRuntimeError("document_intent_recovery_blocked") from None
    try:
        canonical = absolute.resolve(strict=True)
    except (OSError, RuntimeError):
        raise ExternalRuntimeError("document_publish_root_untrusted") from None
    if os.path.normcase(os.fspath(absolute)) != os.path.normcase(os.fspath(canonical)):
        raise ExternalRuntimeError("document_publish_root_untrusted")
    current = Path(absolute.anchor)
    try:
        for part in absolute.parts[1:]:
            current = current / part
            if stat.S_ISLNK(current.lstat().st_mode):
                raise OSError
    except OSError:
        raise ExternalRuntimeError("document_publish_root_untrusted") from None

    descriptor = -1
    try:
        descriptor = os.open(
            absolute,
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0))
            | int(getattr(os, "O_DIRECTORY", 0)),
        )
        opened = os.fstat(descriptor)
        get_euid = getattr(os, "geteuid", None)
        if (
            not stat.S_ISDIR(opened.st_mode)
            or (get_euid is not None and int(opened.st_uid) != int(get_euid()))
        ):
            raise OSError
        components = ["knowledge", "document-intents-v1"]
        if document_key is not None:
            components.append(document_key)
        for name in components:
            if create:
                child = _open_or_create_private_child_directory(descriptor, name)
            else:
                child = _open_existing_private_child_directory(descriptor, name)
            os.close(descriptor)
            descriptor = child
        return descriptor, directory_path
    except ExternalRuntimeError:
        if descriptor >= 0:
            os.close(descriptor)
        raise
    except OSError:
        if descriptor >= 0:
            os.close(descriptor)
        raise ExternalRuntimeError("document_intent_recovery_blocked") from None


def _document_intent_phase_name(intent_id: str, sequence: int, phase: str) -> str:
    if not _DOCUMENT_INTENT_ID_RE.fullmatch(intent_id):
        raise ExternalRuntimeError("document_intent_invalid")
    if sequence < 0 or sequence > 99 or not re.fullmatch(r"[a-z_]{1,32}", phase):
        raise ExternalRuntimeError("document_intent_invalid")
    return f"{intent_id}.{sequence:02d}-{phase}.json"


def _read_private_document_record(
    directory_fd: int,
    name: str,
    *,
    missing_ok: bool,
) -> tuple[bytes, str] | None:
    if not name or Path(name).name != name or name in {".", ".."}:
        raise ExternalRuntimeError("document_intent_invalid")
    descriptor = -1
    try:
        path_stat = os.stat(name, dir_fd=directory_fd, follow_symlinks=False)
    except FileNotFoundError:
        if missing_ok:
            return None
        raise ExternalRuntimeError("document_intent_recovery_blocked") from None
    except OSError:
        raise ExternalRuntimeError("document_intent_recovery_blocked") from None
    try:
        descriptor = os.open(
            name,
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0)),
            dir_fd=directory_fd,
        )
        opened = os.fstat(descriptor)
        get_euid = getattr(os, "geteuid", None)
        if (
            stat.S_ISLNK(path_stat.st_mode)
            or not stat.S_ISREG(path_stat.st_mode)
            or not stat.S_ISREG(opened.st_mode)
            or int(opened.st_nlink) != 1
            or _document_file_identity(path_stat) != _document_file_identity(opened)
            or (stat.S_IMODE(opened.st_mode) & 0o077) != 0
            or (get_euid is not None and int(opened.st_uid) != int(get_euid()))
        ):
            raise OSError
        payload = bytearray()
        while len(payload) <= 128 * 1024:
            chunk = os.read(descriptor, min(16 * 1024, 128 * 1024 + 1 - len(payload)))
            if not chunk:
                break
            payload.extend(chunk)
        if len(payload) > 128 * 1024:
            raise OSError
        opened_after = os.fstat(descriptor)
        path_after = os.stat(name, dir_fd=directory_fd, follow_symlinks=False)
        if (
            _document_file_identity(opened) != _document_file_identity(opened_after)
            or _document_file_identity(opened_after) != _document_file_identity(path_after)
            or _document_mutable_identity(opened) != _document_mutable_identity(opened_after)
        ):
            raise OSError
        raw = bytes(payload)
        return raw, hashlib.sha256(raw).hexdigest()
    except OSError:
        raise ExternalRuntimeError("document_intent_recovery_blocked") from None
    finally:
        if descriptor >= 0:
            os.close(descriptor)


def _write_private_document_record(directory_fd: int, name: str, value: Any) -> str:
    payload = _canonical_document_json(value)
    expected_sha256 = hashlib.sha256(payload).hexdigest()
    descriptor = -1
    try:
        descriptor = os.open(
            name,
            os.O_WRONLY
            | os.O_CREAT
            | os.O_EXCL
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0)),
            0o600,
            dir_fd=directory_fd,
        )
        _write_document_fd(descriptor, payload)
        os.fsync(descriptor)
        os.close(descriptor)
        descriptor = -1
        os.fsync(directory_fd)
    except FileExistsError:
        existing = _read_private_document_record(directory_fd, name, missing_ok=False)
        if existing is None or existing[0] != payload:
            raise ExternalRuntimeError("document_intent_recovery_blocked") from None
        return existing[1]
    except OSError:
        raise ExternalRuntimeError("document_intent_journal_failed") from None
    finally:
        if descriptor >= 0:
            os.close(descriptor)
    reloaded = _read_private_document_record(directory_fd, name, missing_ok=False)
    if reloaded is None or reloaded[0] != payload or reloaded[1] != expected_sha256:
        raise ExternalRuntimeError("document_intent_journal_failed")
    return expected_sha256


def _open_existing_private_child_directory(parent_fd: int, name: str) -> int:
    if not name or Path(name).name != name or name in {".", ".."}:
        raise ExternalRuntimeError("document_publish_root_untrusted")
    descriptor = -1
    try:
        descriptor = os.open(
            name,
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0))
            | int(getattr(os, "O_DIRECTORY", 0)),
            dir_fd=parent_fd,
        )
        opened = os.fstat(descriptor)
        path_stat = os.stat(name, dir_fd=parent_fd, follow_symlinks=False)
        get_euid = getattr(os, "geteuid", None)
        if (
            not stat.S_ISDIR(opened.st_mode)
            or stat.S_ISLNK(path_stat.st_mode)
            or _document_file_identity(opened) != _document_file_identity(path_stat)
            or (get_euid is not None and int(opened.st_uid) != int(get_euid()))
        ):
            raise OSError
        return descriptor
    except OSError:
        if descriptor >= 0:
            os.close(descriptor)
        raise ExternalRuntimeError("document_publish_root_untrusted") from None


def _open_private_document_text_directory(
    case_root: Path,
    document_key: str | None,
    *,
    create: bool,
) -> tuple[int, Path]:
    if document_key is not None and (
        len(document_key) != 64
        or any(character not in "0123456789abcdef" for character in document_key)
    ):
        raise ExternalRuntimeError("document_identity_invalid")
    raw = os.fspath(case_root)
    if not raw or "\x00" in raw or not os.path.isabs(raw):
        raise ExternalRuntimeError("document_publish_root_untrusted")
    absolute = Path(os.path.abspath(raw))
    try:
        canonical = absolute.resolve(strict=True)
    except (OSError, RuntimeError):
        raise ExternalRuntimeError("document_publish_root_untrusted") from None
    if os.path.normcase(os.fspath(absolute)) != os.path.normcase(os.fspath(canonical)):
        raise ExternalRuntimeError("document_publish_root_untrusted")
    current = Path(absolute.anchor)
    try:
        for part in absolute.parts[1:]:
            current = current / part
            if stat.S_ISLNK(current.lstat().st_mode):
                raise OSError
    except OSError:
        raise ExternalRuntimeError("document_publish_root_untrusted") from None

    descriptor = -1
    try:
        descriptor = os.open(
            absolute,
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0))
            | int(getattr(os, "O_DIRECTORY", 0)),
        )
        root_stat = os.fstat(descriptor)
        get_euid = getattr(os, "geteuid", None)
        if (
            not stat.S_ISDIR(root_stat.st_mode)
            or (get_euid is not None and int(root_stat.st_uid) != int(get_euid()))
        ):
            raise OSError
        components = ["knowledge", "texts"]
        if document_key is not None:
            components.append(document_key)
        for name in components:
            if create:
                child = _open_or_create_private_child_directory(descriptor, name)
            else:
                child = _open_existing_private_child_directory(descriptor, name)
            os.close(descriptor)
            descriptor = child
        directory_path = absolute / "knowledge" / "texts"
        if document_key is not None:
            directory_path = directory_path / document_key
        return descriptor, directory_path
    except ExternalRuntimeError:
        if descriptor >= 0:
            os.close(descriptor)
        raise
    except OSError:
        if descriptor >= 0:
            os.close(descriptor)
        raise ExternalRuntimeError("document_publish_root_untrusted") from None


def _try_open_private_document_text_directory(
    case_root: Path,
    document_key: str,
) -> tuple[int, Path] | None:
    absolute = Path(os.path.abspath(os.fspath(case_root)))
    expected = absolute / "knowledge" / "texts" / document_key
    try:
        expected.lstat()
    except FileNotFoundError:
        return None
    except OSError:
        raise ExternalRuntimeError("document_intent_recovery_blocked") from None
    try:
        return _open_private_document_text_directory(
            case_root,
            document_key,
            create=False,
        )
    except ExternalRuntimeError:
        raise ExternalRuntimeError("document_intent_recovery_blocked") from None


def _revalidate_document_directory_path(directory_fd: int, directory_path: Path) -> None:
    recheck_fd = -1
    try:
        recheck_fd = os.open(
            directory_path,
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0))
            | int(getattr(os, "O_DIRECTORY", 0)),
        )
        if _document_file_identity(os.fstat(directory_fd)) != _document_file_identity(os.fstat(recheck_fd)):
            raise OSError
    except OSError:
        raise ExternalRuntimeError("document_publish_root_changed") from None
    finally:
        if recheck_fd >= 0:
            os.close(recheck_fd)


def _unlink_owned_document_target(
    directory_fd: int,
    target_name: str,
    *,
    expected_sha256: str,
    missing_ok: bool,
) -> bool:
    descriptor = -1
    try:
        path_stat = os.stat(target_name, dir_fd=directory_fd, follow_symlinks=False)
    except FileNotFoundError:
        if missing_ok:
            return False
        raise ExternalRuntimeError("document_publish_failed") from None
    except OSError:
        raise ExternalRuntimeError("document_publish_failed") from None
    try:
        descriptor = os.open(
            target_name,
            os.O_RDONLY | int(getattr(os, "O_CLOEXEC", 0)) | int(getattr(os, "O_NOFOLLOW", 0)),
            dir_fd=directory_fd,
        )
        opened = os.fstat(descriptor)
        get_euid = getattr(os, "geteuid", None)
        if (
            stat.S_ISLNK(path_stat.st_mode)
            or not stat.S_ISREG(path_stat.st_mode)
            or not stat.S_ISREG(opened.st_mode)
            or int(opened.st_nlink) != 1
            or _document_file_identity(path_stat) != _document_file_identity(opened)
            or (stat.S_IMODE(opened.st_mode) & 0o077) != 0
            or (get_euid is not None and int(opened.st_uid) != int(get_euid()))
        ):
            raise OSError
        digest = hashlib.sha256()
        while True:
            chunk = os.read(descriptor, 1024 * 1024)
            if not chunk:
                break
            digest.update(chunk)
        opened_after = os.fstat(descriptor)
        path_after = os.stat(target_name, dir_fd=directory_fd, follow_symlinks=False)
        if (
            digest.hexdigest() != expected_sha256
            or _document_file_identity(opened) != _document_file_identity(opened_after)
            or _document_file_identity(opened_after) != _document_file_identity(path_after)
            or _document_mutable_identity(opened) != _document_mutable_identity(opened_after)
        ):
            raise OSError
        os.unlink(target_name, dir_fd=directory_fd)
        os.fsync(directory_fd)
        return True
    except OSError:
        raise ExternalRuntimeError("document_generation_cleanup_failed") from None
    finally:
        if descriptor >= 0:
            os.close(descriptor)


class DocumentRepository:
    def __init__(
        self,
        *,
        chunk_size: int = 1200,
        chunk_overlap: int = 160,
        ocr_enabled: bool = True,
        ocr_language: str = "chi_sim+eng",
        vector_mode: str = "hash256",
    ) -> None:
        self._storage = CaseStorage()
        self._chunk_size = max(400, int(chunk_size))
        self._chunk_overlap = max(40, min(int(chunk_overlap), self._chunk_size // 2))
        self._ocr_enabled = bool(ocr_enabled)
        self._ocr_language = str(ocr_language or "chi_sim+eng")
        self._vector_mode = str(vector_mode or "hash256").strip() or "hash256"

    @property
    def storage(self) -> CaseStorage:
        return self._storage

    def ensure_tables(self, engine: DuckDBEngine) -> None:
        engine.execute(
            """CREATE TABLE IF NOT EXISTS document_assets(
                document_id TEXT PRIMARY KEY,
                case_id TEXT,
                file_id TEXT,
                kind TEXT,
                filename TEXT,
                display_path TEXT,
                stored_path TEXT,
                file_type TEXT,
                size BIGINT DEFAULT 0,
                md5 TEXT,
                sha256 TEXT,
                duplicate_of_document_id TEXT,
                duplicate_reason TEXT,
                selected_for_llm BOOLEAN DEFAULT FALSE,
                llm_ready BOOLEAN DEFAULT FALSE,
                parser TEXT,
                extraction_status TEXT,
                ocr_status TEXT,
                vector_status TEXT,
                vector_model TEXT,
                content_path TEXT,
                content_excerpt TEXT,
                content_chars BIGINT DEFAULT 0,
                page_count BIGINT DEFAULT 0,
                chunk_count BIGINT DEFAULT 0,
                token_estimate BIGINT DEFAULT 0,
                last_error TEXT,
                created_at TEXT,
                updated_at TEXT,
                indexed_at TEXT
            )"""
        )
        engine.execute("CREATE INDEX IF NOT EXISTS idx_document_assets_case_updated ON document_assets(case_id, updated_at)")
        engine.execute("CREATE UNIQUE INDEX IF NOT EXISTS uq_document_assets_case_file ON document_assets(case_id, file_id)")
        engine.execute("CREATE INDEX IF NOT EXISTS idx_document_assets_case_selected ON document_assets(case_id, selected_for_llm)")
        engine.execute("CREATE INDEX IF NOT EXISTS idx_document_assets_case_sha256 ON document_assets(case_id, sha256)")

        engine.execute(
            """CREATE TABLE IF NOT EXISTS document_chunks(
                chunk_id TEXT PRIMARY KEY,
                case_id TEXT,
                document_id TEXT,
                file_id TEXT,
                chunk_index BIGINT,
                page_from BIGINT,
                page_to BIGINT,
                text TEXT,
                text_chars BIGINT DEFAULT 0,
                token_estimate BIGINT DEFAULT 0,
                vector_model TEXT,
                vector_dim BIGINT DEFAULT 0,
                vector_data TEXT,
                created_at TEXT
            )"""
        )
        engine.execute("CREATE INDEX IF NOT EXISTS idx_document_chunks_case_doc ON document_chunks(case_id, document_id, chunk_index)")

        try:
            existing_cols = {
                row[0]
                for row in engine.query(
                    "SELECT column_name FROM information_schema.columns "
                    "WHERE table_schema='main' AND table_name='document_assets'"
                )
            }
            alters = []
            if "duplicate_of_document_id" not in existing_cols:
                alters.append("ALTER TABLE document_assets ADD COLUMN duplicate_of_document_id TEXT")
            if "duplicate_reason" not in existing_cols:
                alters.append("ALTER TABLE document_assets ADD COLUMN duplicate_reason TEXT")
            if "selected_for_llm" not in existing_cols:
                alters.append("ALTER TABLE document_assets ADD COLUMN selected_for_llm BOOLEAN DEFAULT FALSE")
            if "llm_ready" not in existing_cols:
                alters.append("ALTER TABLE document_assets ADD COLUMN llm_ready BOOLEAN DEFAULT FALSE")
            if "vector_model" not in existing_cols:
                alters.append("ALTER TABLE document_assets ADD COLUMN vector_model TEXT")
            if "content_path" not in existing_cols:
                alters.append("ALTER TABLE document_assets ADD COLUMN content_path TEXT")
            if "content_excerpt" not in existing_cols:
                alters.append("ALTER TABLE document_assets ADD COLUMN content_excerpt TEXT")
            if "content_chars" not in existing_cols:
                alters.append("ALTER TABLE document_assets ADD COLUMN content_chars BIGINT DEFAULT 0")
            if "page_count" not in existing_cols:
                alters.append("ALTER TABLE document_assets ADD COLUMN page_count BIGINT DEFAULT 0")
            if "chunk_count" not in existing_cols:
                alters.append("ALTER TABLE document_assets ADD COLUMN chunk_count BIGINT DEFAULT 0")
            if "token_estimate" not in existing_cols:
                alters.append("ALTER TABLE document_assets ADD COLUMN token_estimate BIGINT DEFAULT 0")
            if "last_error" not in existing_cols:
                alters.append("ALTER TABLE document_assets ADD COLUMN last_error TEXT")
            if "indexed_at" not in existing_cols:
                alters.append("ALTER TABLE document_assets ADD COLUMN indexed_at TEXT")
            for stmt in alters:
                engine.execute(stmt)
        except Exception:
            pass

    def ingest_imported_file(
        self,
        *,
        engine: DuckDBEngine,
        case_id: str,
        file_id: str,
        filename: str,
        display_path: str,
        stored_path: str,
        file_type: str,
        size: int,
        md5: str,
        sha256: str,
        kind: str,
    ) -> DocumentIngestResult:
        require_controlled_source_ingestion()
        document_id = str(file_id)
        try:
            if self._document_engine_transaction_is_active(engine):
                raise _DocumentDatabaseMutationError(
                    "document_database_update_failed",
                    safe_to_remove_new_generation=True,
                )
        except _DocumentDatabaseMutationError as exc:
            return self._failed_document_ingest_result(
                document_id=str(file_id),
                file_id=file_id,
                file_type=file_type,
                stored_path=stored_path,
                exc=exc,
            )
        intent_root_fd = -1
        intent_root_lock_fd = -1
        intent_directory_fd = -1
        intent_lock_fd = -1
        try:
            opened_intent_root = _open_private_document_intent_directory(
                self._storage.case_dir(case_id),
                None,
                create=True,
            )
            if opened_intent_root is None:
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            intent_root_fd, _intent_root_path = opened_intent_root
            intent_root_lock_fd = _open_document_publish_lock(intent_root_fd)
            opened_intent_directory = _open_private_document_intent_directory(
                self._storage.case_dir(case_id),
                _document_text_storage_key(document_id),
                create=True,
            )
            if opened_intent_directory is None:
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            intent_directory_fd, _intent_directory_path = opened_intent_directory
            intent_lock_fd = _open_document_publish_lock(intent_directory_fd)
            recovery_plan = self._plan_document_generation_recovery_in_directory(
                engine,
                case_id=case_id,
                document_id=document_id,
                file_id=file_id,
                directory_fd=intent_directory_fd,
            )
            if recovery_plan.action != "none":
                recovery_plan = self._apply_document_generation_recovery_in_directory(
                    engine,
                    case_id=case_id,
                    document_id=document_id,
                    file_id=file_id,
                    directory_fd=intent_directory_fd,
                    plan=recovery_plan,
                )
            if recovery_plan.action == "quarantine":
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            return self._ingest_imported_file_locked(
                engine=engine,
                case_id=case_id,
                file_id=file_id,
                filename=filename,
                display_path=display_path,
                stored_path=stored_path,
                file_type=file_type,
                size=size,
                md5=md5,
                sha256=sha256,
                kind=kind,
                intent_directory_fd=intent_directory_fd,
            )
        except Exception as exc:
            return self._failed_document_ingest_result(
                document_id=document_id,
                file_id=file_id,
                file_type=file_type,
                stored_path=stored_path,
                exc=exc,
            )
        finally:
            if intent_lock_fd >= 0:
                os.close(intent_lock_fd)
            if intent_directory_fd >= 0:
                os.close(intent_directory_fd)
            if intent_root_lock_fd >= 0:
                os.close(intent_root_lock_fd)
            if intent_root_fd >= 0:
                os.close(intent_root_fd)

    def _ingest_imported_file_locked(
        self,
        *,
        engine: DuckDBEngine,
        case_id: str,
        file_id: str,
        filename: str,
        display_path: str,
        stored_path: str,
        file_type: str,
        size: int,
        md5: str,
        sha256: str,
        kind: str,
        intent_directory_fd: int,
    ) -> DocumentIngestResult:
        self.ensure_tables(engine)
        now = _now_iso()
        document_id = str(file_id)
        existing = self._get_asset_row(engine, case_id=case_id, document_id=document_id)

        duplicate = self._find_duplicate_asset(
            engine,
            case_id=case_id,
            sha256=sha256,
            exclude_document_id=document_id,
        )
        if duplicate and bool(duplicate.get("llm_ready")):
            duplicate_result = DocumentIngestResult(
                document_id=document_id,
                file_id=file_id,
                extraction_status="duplicate",
                ocr_status=str(duplicate.get("ocr_status") or "not_needed"),
                vector_status=str(duplicate.get("vector_status") or "completed"),
                chunk_count=int(duplicate.get("chunk_count") or 0),
                llm_ready=bool(duplicate.get("llm_ready")),
                duplicate_of_document_id=str(duplicate.get("document_id") or ""),
                last_error="",
                note="检测到相同内容文件，已复用既有知识索引。",
            )
            duplicate_content_path = str(duplicate.get("content_path") or "")
            duplicate_asset_row = {
                "document_id": document_id,
                "case_id": case_id,
                "file_id": file_id,
                "kind": kind,
                "filename": filename,
                "display_path": display_path,
                "stored_path": stored_path,
                "file_type": file_type,
                "size": int(size or 0),
                "md5": md5,
                "sha256": sha256,
                "duplicate_of_document_id": str(duplicate.get("document_id") or ""),
                "duplicate_reason": "same_sha256",
                "selected_for_llm": bool(existing.get("selected_for_llm")) if existing else False,
                "llm_ready": bool(duplicate.get("llm_ready")),
                "parser": f"duplicate:{duplicate.get('parser') or 'indexed'}",
                "extraction_status": "duplicate",
                "ocr_status": str(duplicate.get("ocr_status") or "not_needed"),
                "vector_status": str(duplicate.get("vector_status") or "completed"),
                "vector_model": str(duplicate.get("vector_model") or self._vector_mode),
                "content_path": duplicate_content_path,
                "content_excerpt": str(duplicate.get("content_excerpt") or ""),
                "content_chars": int(duplicate.get("content_chars") or 0),
                "page_count": int(duplicate.get("page_count") or 0),
                "chunk_count": int(duplicate.get("chunk_count") or 0),
                "token_estimate": int(duplicate.get("token_estimate") or 0),
                "last_error": "",
                "created_at": str(existing.get("created_at") or now) if existing else now,
                "updated_at": now,
                "indexed_at": now,
            }
            try:
                duplicate_generation = self._describe_document_content_generation(
                    case_id=case_id,
                    document_id=document_id,
                    content_path=duplicate_content_path,
                )
                if duplicate_generation.get("kind") != "immutable_v1":
                    raise ExternalRuntimeError("document_intent_recovery_blocked")
                if self._document_database_state(
                    engine,
                    case_id=case_id,
                    document_id=document_id,
                ) == self._document_database_state_from_rows(duplicate_asset_row, []):
                    self._verify_intent_generation(
                        case_id=case_id,
                        document_id=document_id,
                        descriptor=duplicate_generation,
                        required=True,
                    )
                    return duplicate_result
                intent_state = self._begin_document_generation_intent(
                    engine,
                    case_id=case_id,
                    document_id=document_id,
                    file_id=file_id,
                    old_asset=existing,
                    new_asset=duplicate_asset_row,
                    new_chunks=[],
                    new_content_generation=duplicate_generation,
                    directory_fd=intent_directory_fd,
                )
                intent_state = self._append_document_intent_phase(
                    intent_directory_fd,
                    intent_state,
                    sequence=10,
                    phase="publish_started",
                )
                self._document_generation_crash_cut("publish_started_recorded")
                self._verify_intent_generation(
                    case_id=case_id,
                    document_id=document_id,
                    descriptor=duplicate_generation,
                    required=True,
                )
                intent_state = self._append_document_intent_phase(
                    intent_directory_fd,
                    intent_state,
                    sequence=20,
                    phase="published",
                )
                self._document_generation_crash_cut("published_recorded")
                intent_state = self._append_document_intent_phase(
                    intent_directory_fd,
                    intent_state,
                    sequence=30,
                    phase="db_started",
                )
                self._document_generation_crash_cut("db_started_recorded")
                self._replace_document_database_generation(
                    engine,
                    case_id=case_id,
                    document_id=document_id,
                    chunk_rows=[],
                    asset_row=duplicate_asset_row,
                )
                intent_state = self._append_document_intent_phase(
                    intent_directory_fd,
                    intent_state,
                    sequence=40,
                    phase="db_committed",
                )
                self._document_generation_crash_cut("db_committed_recorded")
                recovery = self._plan_document_generation_recovery_in_directory(
                    engine,
                    case_id=case_id,
                    document_id=document_id,
                    file_id=file_id,
                    directory_fd=intent_directory_fd,
                )
                recovery = self._apply_document_generation_recovery_in_directory(
                    engine,
                    case_id=case_id,
                    document_id=document_id,
                    file_id=file_id,
                    directory_fd=intent_directory_fd,
                    plan=recovery,
                )
                if recovery.action == "quarantine":
                    raise ExternalRuntimeError("document_intent_recovery_blocked")
            except Exception as exc:
                exc = self._recover_document_intent_after_exception(
                    engine,
                    case_id=case_id,
                    document_id=document_id,
                    file_id=file_id,
                    directory_fd=intent_directory_fd,
                    original=exc,
                )
                if exc is None:
                    return duplicate_result
                return self._failed_document_ingest_result(
                    document_id=document_id,
                    file_id=file_id,
                    file_type=file_type,
                    stored_path=stored_path,
                    exc=exc,
                )
            return duplicate_result

        source_path = Path(stored_path).expanduser()
        try:
            extraction = self._extract_document(case_id=case_id, path=source_path)
            full_text = self._join_segments(extraction.segments)
            chunk_rows = self._build_chunk_rows(
                case_id=case_id,
                document_id=document_id,
                file_id=file_id,
                segments=extraction.segments,
            )
            if not full_text.strip() or not chunk_rows:
                raise ExternalRuntimeError("document_content_unavailable")

            chunk_vector_statuses = {str(row.get("_vector_status") or "") for row in chunk_rows}
            if chunk_vector_statuses == {"completed"}:
                vector_status = "completed"
            elif "completed" in chunk_vector_statuses:
                vector_status = "partial"
            elif "skipped" in chunk_vector_statuses and len(chunk_vector_statuses) == 1:
                vector_status = "skipped"
            else:
                vector_status = "failed"
            extraction_status = "completed"
            last_error = str(extraction.last_error or "")
            note = "正文已抽取并完成分块入库，可在后续页面勾选供大模型使用。"
            if extraction.ocr_status == "completed":
                note = "正文已抽取，缺失页面已通过 OCR 补全并完成分块入库。"

            content_sha256 = hashlib.sha256(full_text.encode("utf-8", errors="strict")).hexdigest()
            new_content_generation = {
                "kind": "immutable_v1",
                "documentKey": _document_text_storage_key(document_id),
                "targetName": f"{content_sha256}.txt",
                "contentSha256": content_sha256,
                "stagingName": f".generation-stage-v1-{secrets.token_hex(24)}.tmp",
            }
            generation_path = self._document_generation_path_from_descriptor(
                case_id=case_id,
                document_id=document_id,
                descriptor=new_content_generation,
            )
            if generation_path is None:
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            self._assert_intent_staging_absent(
                case_id=case_id,
                descriptor=new_content_generation,
            )
            asset_row = {
                "document_id": document_id,
                "case_id": case_id,
                "file_id": file_id,
                "kind": kind,
                "filename": filename,
                "display_path": display_path,
                "stored_path": stored_path,
                "file_type": file_type,
                "size": int(size or 0),
                "md5": md5,
                "sha256": sha256,
                "duplicate_of_document_id": "",
                "duplicate_reason": "",
                "selected_for_llm": bool(existing.get("selected_for_llm")) if existing else False,
                "llm_ready": True,
                "parser": extraction.parser,
                "extraction_status": extraction_status,
                "ocr_status": extraction.ocr_status,
                "vector_status": vector_status,
                "vector_model": "" if self._vector_disabled else self._vector_mode,
                "content_path": str(generation_path),
                "content_excerpt": self._excerpt(full_text),
                "content_chars": len(full_text),
                "page_count": int(extraction.page_count or 0),
                "chunk_count": len(chunk_rows),
                "token_estimate": self._estimate_tokens(full_text),
                "last_error": last_error,
                "created_at": str(existing.get("created_at") or now) if existing else now,
                "updated_at": now,
                "indexed_at": now,
            }
            if self._document_database_state(
                engine,
                case_id=case_id,
                document_id=document_id,
            ) == self._document_database_state_from_rows(asset_row, chunk_rows):
                self._verify_intent_generation(
                    case_id=case_id,
                    document_id=document_id,
                    descriptor=new_content_generation,
                    required=True,
                )
                return DocumentIngestResult(
                    document_id=document_id,
                    file_id=file_id,
                    extraction_status=extraction_status,
                    ocr_status=extraction.ocr_status,
                    vector_status=vector_status,
                    chunk_count=len(chunk_rows),
                    llm_ready=True,
                    duplicate_of_document_id="",
                    last_error=last_error,
                    note=note,
                )
            intent_state = self._begin_document_generation_intent(
                engine,
                case_id=case_id,
                document_id=document_id,
                file_id=file_id,
                old_asset=existing,
                new_asset=asset_row,
                new_chunks=chunk_rows,
                new_content_generation=new_content_generation,
                directory_fd=intent_directory_fd,
            )
            intent_state = self._append_document_intent_phase(
                intent_directory_fd,
                intent_state,
                sequence=10,
                phase="publish_started",
            )
            self._document_generation_crash_cut("publish_started_recorded")
            generation = self._publish_document_text_generation(
                case_id=case_id,
                document_id=document_id,
                content=full_text,
                temporary_name=new_content_generation["stagingName"],
            )
            if (
                generation.sha256 != content_sha256
                or generation.target_name != new_content_generation["targetName"]
                or generation.document_key != new_content_generation["documentKey"]
                or generation.path != generation_path
            ):
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            self._document_generation_crash_cut("text_published")
            intent_state = self._append_document_intent_phase(
                intent_directory_fd,
                intent_state,
                sequence=20,
                phase="published",
            )
            self._document_generation_crash_cut("published_recorded")
            intent_state = self._append_document_intent_phase(
                intent_directory_fd,
                intent_state,
                sequence=30,
                phase="db_started",
            )
            self._document_generation_crash_cut("db_started_recorded")
            self._replace_document_database_generation(
                engine,
                case_id=case_id,
                document_id=document_id,
                chunk_rows=chunk_rows,
                asset_row=asset_row,
            )
            intent_state = self._append_document_intent_phase(
                intent_directory_fd,
                intent_state,
                sequence=40,
                phase="db_committed",
            )
            self._document_generation_crash_cut("db_committed_recorded")
            recovery = self._plan_document_generation_recovery_in_directory(
                engine,
                case_id=case_id,
                document_id=document_id,
                file_id=file_id,
                directory_fd=intent_directory_fd,
            )
            recovery = self._apply_document_generation_recovery_in_directory(
                engine,
                case_id=case_id,
                document_id=document_id,
                file_id=file_id,
                directory_fd=intent_directory_fd,
                plan=recovery,
            )
            if recovery.action == "quarantine":
                raise ExternalRuntimeError("document_intent_recovery_blocked")
        except Exception as exc:
            exc = self._recover_document_intent_after_exception(
                engine,
                case_id=case_id,
                document_id=document_id,
                file_id=file_id,
                directory_fd=intent_directory_fd,
                original=exc,
            )
            if exc is None:
                return DocumentIngestResult(
                    document_id=document_id,
                    file_id=file_id,
                    extraction_status=extraction_status,
                    ocr_status=extraction.ocr_status,
                    vector_status=vector_status,
                    chunk_count=len(chunk_rows),
                    llm_ready=True,
                    duplicate_of_document_id="",
                    last_error=last_error,
                    note=note,
                )
            return self._failed_document_ingest_result(
                document_id=document_id,
                file_id=file_id,
                file_type=file_type,
                stored_path=stored_path,
                exc=exc,
            )

        return DocumentIngestResult(
            document_id=document_id,
            file_id=file_id,
            extraction_status=extraction_status,
            ocr_status=extraction.ocr_status,
            vector_status=vector_status,
            chunk_count=len(chunk_rows),
            llm_ready=True,
            duplicate_of_document_id="",
            last_error=last_error,
            note=note,
        )

    def _failed_document_ingest_result(
        self,
        *,
        document_id: str,
        file_id: str,
        file_type: str,
        stored_path: str,
        exc: BaseException,
    ) -> DocumentIngestResult:
        return DocumentIngestResult(
            document_id=document_id,
            file_id=file_id,
            extraction_status="failed",
            ocr_status=(
                "failed"
                if self._looks_like_pdf(file_type=file_type, stored_path=stored_path)
                else "not_needed"
            ),
            vector_status="skipped",
            chunk_count=0,
            llm_ready=False,
            last_error=_safe_document_error_code(exc),
            note="文件已登记到知识台账，但正文抽取失败。",
        )

    def sync_case_documents(self, case_id: str) -> dict:
        require_controlled_source_ingestion()
        engine = self._storage.open_case_engine(case_id)
        try:
            self.ensure_tables(engine)
            exists = engine.query(
                "SELECT 1 FROM information_schema.tables WHERE table_schema='main' AND table_name='import_file_log' LIMIT 1"
            )
            if not exists:
                return {"indexed": 0, "failed": 0, "total": 0}
            rows = engine.query(
                """SELECT file_id, kind, filename, display_path, stored_path, file_type, size, md5, sha256
                   FROM import_file_log
                   WHERE case_id=? AND kind=? AND status='已完成'
                     AND NOT EXISTS (
                       SELECT 1 FROM document_assets d
                       WHERE d.case_id=import_file_log.case_id AND d.file_id=import_file_log.file_id
                     )
                   ORDER BY created_at ASC""",
                (case_id, "support_file"),
            )
            indexed = 0
            failed = 0
            for row in rows:
                result = self.ingest_imported_file(
                    engine=engine,
                    case_id=case_id,
                    file_id=str(row[0] or ""),
                    kind=str(row[1] or "support_file"),
                    filename=str(row[2] or ""),
                    display_path=str(row[3] or ""),
                    stored_path=str(row[4] or ""),
                    file_type=str(row[5] or ""),
                    size=int(row[6] or 0),
                    md5=str(row[7] or ""),
                    sha256=str(row[8] or ""),
                )
                if result.llm_ready:
                    indexed += 1
                else:
                    failed += 1
            return {"indexed": indexed, "failed": failed, "total": len(rows)}
        finally:
            try:
                engine.close()
            except Exception:
                pass

    def list_documents(
        self,
        case_id: str,
        *,
        selected_for_llm: Optional[bool] = None,
        llm_ready: Optional[bool] = None,
        include_system: bool = False,
    ) -> List[dict]:
        engine = self._storage.open_case_engine(case_id)
        try:
            self.ensure_tables(engine)
            clauses = ["case_id=?"]
            params: List[object] = [case_id]
            if selected_for_llm is not None:
                clauses.append("selected_for_llm=?")
                params.append(bool(selected_for_llm))
            if llm_ready is not None:
                clauses.append("llm_ready=?")
                params.append(bool(llm_ready))
            rows = engine.query(
                f"""SELECT document_id, case_id, file_id, kind, filename, display_path, stored_path, file_type,
                           size, md5, sha256, duplicate_of_document_id, duplicate_reason,
                           selected_for_llm, llm_ready, parser, extraction_status, ocr_status,
                           vector_status, vector_model, content_path, content_excerpt, content_chars,
                           page_count, chunk_count, token_estimate, last_error, created_at, updated_at, indexed_at
                    FROM document_assets
                    WHERE {' AND '.join(clauses)}
                    ORDER BY COALESCE(NULLIF(created_at, ''), NULLIF(updated_at, ''), '') DESC, filename ASC""",
                params,
            )
            items = [self._asset_row_to_dict(row) for row in rows]
            if include_system:
                system_assets = [
                    item
                    for item in self._case_project_doc_assets(case_id)
                    if self._matches_document_filters(
                        item,
                        selected_for_llm=selected_for_llm,
                        llm_ready=llm_ready,
                    )
                ]
                items = [*system_assets, *items]
            return items
        finally:
            try:
                engine.close()
            except Exception:
                pass

    def list_document_public_metadata(
        self,
        case_id: str,
        *,
        selected_for_llm: Optional[bool] = None,
        llm_ready: Optional[bool] = None,
        include_system: bool = False,
    ) -> List[dict]:
        """Return the closed ordinary-API metadata projection without raw reads.

        This path is deliberately read-only and does not synchronize or ingest
        documents. It selects no filename, path, excerpt, chunk, error text, or
        other source-controlled string.
        """

        items: List[dict] = []
        engine: Optional[DuckDBEngine] = None
        try:
            engine = self._storage.open_case_engine(case_id, read_only=True)
            exists = engine.query(
                "SELECT 1 FROM information_schema.tables "
                "WHERE table_schema='main' AND table_name='document_assets' LIMIT 1"
            )
            if not exists:
                raise DocumentPublicMetadataUnavailableError(
                    "document_public_metadata_source_unavailable"
                )
            clauses = ["case_id=?"]
            params: List[object] = [case_id]
            if selected_for_llm is not None:
                clauses.append("selected_for_llm=?")
                params.append(bool(selected_for_llm))
            if llm_ready is not None:
                clauses.append("llm_ready=?")
                params.append(bool(llm_ready))
            rows = engine.query(
                f"""SELECT document_id, case_id, kind, duplicate_of_document_id,
                           selected_for_llm, llm_ready
                    FROM document_assets
                    WHERE {' AND '.join(clauses)}
                    ORDER BY document_id ASC""",
                params,
            )
            for row in rows:
                if (
                    len(row) != 6
                    or (row[4] is not None and type(row[4]) is not bool)
                    or (row[5] is not None and type(row[5]) is not bool)
                ):
                    raise DocumentPublicMetadataUnavailableError(
                        "document_public_metadata_state_invalid"
                    )
                items.append(
                    {
                        "document_id": str(row[0] or ""),
                        "case_id": str(row[1] or ""),
                        "kind": str(row[2] or ""),
                        "duplicate_of_document_id": str(row[3] or ""),
                        "selected_for_llm": row[4],
                        "llm_ready": row[5],
                    }
                )
        except (FileNotFoundError, DiagnosticDuckDBMigrationError) as exc:
            raise DocumentPublicMetadataUnavailableError(
                "document_public_metadata_source_unavailable"
            ) from exc
        finally:
            if engine is not None:
                try:
                    engine.close()
                except Exception:
                    pass

        if include_system:
            case_root = self._storage.case_dir(case_id)
            for document_id, path in (
                (CASE_DIRECTION_DOC_DOCUMENT_ID, case_direction_doc_path(case_root)),
                (CASE_PROJECT_DOC_DOCUMENT_ID, case_project_doc_path(case_root)),
            ):
                try:
                    path_stat = path.lstat()
                    ready = stat.S_ISREG(path_stat.st_mode) and path_stat.st_size > 0
                except OSError:
                    continue
                item = {
                    "document_id": document_id,
                    "case_id": case_id,
                    "kind": CASE_PROJECT_DOC_KIND,
                    "duplicate_of_document_id": "",
                    "selected_for_llm": False,
                    "llm_ready": ready,
                }
                if self._matches_document_filters(
                    item,
                    selected_for_llm=selected_for_llm,
                    llm_ready=llm_ready,
                ):
                    items.append(item)
        return items

    def get_document_detail(
        self,
        case_id: str,
        document_id: str,
        *,
        include_chunks: bool,
        chunk_limit: int,
    ) -> Optional[dict]:
        normalized_document_id = str(document_id or "").strip()
        if normalized_document_id in {CASE_PROJECT_DOC_DOCUMENT_ID, CASE_DIRECTION_DOC_DOCUMENT_ID}:
            return self._case_project_doc_detail(
                case_id=case_id,
                document_id=normalized_document_id,
                include_chunks=include_chunks,
                chunk_limit=chunk_limit,
            )
        engine = self._storage.open_case_engine(case_id)
        try:
            self.ensure_tables(engine)
            asset = self._get_asset_row(engine, case_id=case_id, document_id=document_id)
            if asset is None:
                return None
            resolved_document_id = str(asset.get("duplicate_of_document_id") or asset.get("document_id") or "")
            content_path = str(asset.get("content_path") or "")
            if not content_path and resolved_document_id and resolved_document_id != document_id:
                source_asset = self._get_asset_row(engine, case_id=case_id, document_id=resolved_document_id)
                if source_asset is not None:
                    content_path = str(source_asset.get("content_path") or "")
            chunks: List[dict] = []
            if include_chunks:
                chunks = self._list_document_chunks(
                    engine,
                    case_id=case_id,
                    document_id=resolved_document_id or document_id,
                    limit=max(1, int(chunk_limit)),
                )
            return {
                "document": asset,
                "resolved_document_id": resolved_document_id or document_id,
                "text_preview": self._read_text_preview(content_path),
                "chunks": chunks,
            }
        finally:
            try:
                engine.close()
            except Exception:
                pass

    def delete_by_file_id(self, engine: DuckDBEngine, *, case_id: str, file_id: str) -> None:
        require_controlled_source_ingestion()
        self.ensure_tables(engine)
        asset = self._get_asset_row(engine, case_id=case_id, document_id=file_id)
        if asset is None:
            return
        dependents = [
            str(row[0] or "")
            for row in engine.query(
                "SELECT document_id FROM document_assets WHERE case_id=? AND duplicate_of_document_id=?",
                (case_id, file_id),
            )
        ]
        self._delete_document_chunks(engine, case_id=case_id, document_id=file_id)
        self._delete_managed_text(case_id=case_id, document_id=file_id, content_path=str(asset.get("content_path") or ""))
        engine.execute("DELETE FROM document_assets WHERE case_id=? AND document_id=?", (case_id, file_id))
        for dependent_id in dependents:
            dependent_asset = self._get_asset_row(engine, case_id=case_id, document_id=dependent_id)
            if dependent_asset is None:
                continue
            self.ingest_imported_file(
                engine=engine,
                case_id=case_id,
                file_id=str(dependent_asset.get("file_id") or dependent_id),
                kind=str(dependent_asset.get("kind") or "support_file"),
                filename=str(dependent_asset.get("filename") or ""),
                display_path=str(dependent_asset.get("display_path") or ""),
                stored_path=str(dependent_asset.get("stored_path") or ""),
                file_type=str(dependent_asset.get("file_type") or ""),
                size=int(dependent_asset.get("size") or 0),
                md5=str(dependent_asset.get("md5") or ""),
                sha256=str(dependent_asset.get("sha256") or ""),
            )

    @property
    def _vector_disabled(self) -> bool:
        return self._vector_mode.lower() in {"", "none", "disabled", "off"}

    def _extract_document(self, *, case_id: str, path: Path) -> DocumentExtraction:
        if path.suffix.lower() == ".doc":
            _require_document_execution_capability()
        if not path.exists():
            raise FileNotFoundError(str(path))

        suffix = path.suffix.lower()
        if suffix == ".txt":
            return self._extract_text_file(path, parser_name="txt")
        if suffix == ".md":
            return self._extract_text_file(path, parser_name="md")
        if suffix == ".pdf":
            return self._extract_pdf(path)
        if suffix == ".docx":
            return self._extract_docx(path)
        if suffix == ".doc":
            return self._extract_doc(case_id=case_id, path=path)
        return DocumentExtraction(
            segments=[],
            parser=self._file_type_parser(path.suffix.replace(".", "")),
            last_error="document_format_unsupported",
            unsupported=True,
        )

    def _extract_text_file(self, path: Path, *, parser_name: str) -> DocumentExtraction:
        encoding = detect_file_encoding(path)
        text = path.read_text(encoding=encoding, errors="ignore")
        return DocumentExtraction(
            segments=[TextSegment(text=text)],
            parser=f"{parser_name}:{encoding}",
        )

    def _extract_docx(self, path: Path) -> DocumentExtraction:
        with zipfile.ZipFile(path, "r") as archive:
            xml_bytes = archive.read("word/document.xml")
        root = ET.fromstring(xml_bytes)
        paragraphs: List[str] = []
        for paragraph in root.findall(".//w:p", _DOCX_NS):
            runs = [
                node.text or ""
                for node in paragraph.findall(".//w:t", _DOCX_NS)
                if (node.text or "").strip()
            ]
            if runs:
                paragraphs.append("".join(runs))
        text = "\n".join(paragraphs)
        return DocumentExtraction(
            segments=[TextSegment(text=text)],
            parser="docx:xml",
        )

    def _extract_doc(self, *, case_id: str, path: Path) -> DocumentExtraction:
        _require_document_execution_capability()
        office_runtime = _resolved_document_runtime(
            _DOCUMENT_SOFFICE_ENV,
            {"soffice", "soffice.exe", "libreoffice", "libreoffice.exe"},
        )
        if office_runtime.path is not None and office_runtime.sha256:
            converted = self._convert_doc_with_soffice(
                case_id=case_id,
                office_binary=os.fspath(office_runtime.path),
                expected_executable_sha256=office_runtime.sha256,
                path=path,
            )
            extraction = self._extract_docx(converted)
            extraction.parser = "doc:soffice"
            return extraction

        textutil_runtime = _resolved_document_runtime(
            _DOCUMENT_TEXTUTIL_ENV,
            {"textutil", "textutil.exe"},
        )
        if textutil_runtime.path is not None and textutil_runtime.sha256:
            converted = self._convert_doc_with_textutil(
                case_id=case_id,
                textutil_binary=os.fspath(textutil_runtime.path),
                expected_executable_sha256=textutil_runtime.sha256,
                path=path,
            )
            extraction = self._extract_text_file(converted, parser_name="doc")
            extraction.parser = "doc:textutil"
            return extraction

        text_runtime = _resolved_document_runtime(
            _DOCUMENT_ANTIWORD_ENV,
            {"antiword", "antiword.exe", "catdoc", "catdoc.exe"},
        )
        if text_runtime.path is not None and text_runtime.sha256:
            text_binary = text_runtime.path
            with tempfile.TemporaryDirectory(prefix="analytix-document-runtime-") as temporary:
                runtime_dir = Path(temporary).resolve(strict=True)
                os.chmod(runtime_dir, 0o700)
                opaque_source = runtime_dir / "input.doc"
                _copy_regular_source(path, opaque_source)
                runtime_error = ""
                try:
                    completed = run_managed_process(
                        [os.fspath(text_binary), opaque_source.name],
                        cwd=runtime_dir,
                        input_bytes=None,
                        timeout_seconds=60,
                        expected_executable_sha256=text_runtime.sha256,
                        capture_stdout=True,
                    )
                except ManagedProcessTimeout:
                    runtime_error = "document_extract_timeout"
                except ManagedProcessTerminationError:
                    runtime_error = "document_process_tree_termination_failed"
                except ManagedProcessLaunchError:
                    runtime_error = "document_runtime_launch_failed"
                except ManagedProcessOutputLimit:
                    runtime_error = "document_output_limit"
                if runtime_error:
                    raise ExternalRuntimeError(runtime_error)
            if completed.returncode != 0:
                raise ExternalRuntimeError("document_extract_failed")
            stdout = completed.stdout.decode("utf-8", errors="ignore").strip()
            if stdout:
                return DocumentExtraction(
                    segments=[TextSegment(text=stdout)],
                    parser=f"doc:{text_binary.name}",
                )

        return DocumentExtraction(
            segments=[],
            parser="doc",
            last_error="document_runtime_unavailable",
            unsupported=True,
        )

    def _extract_pdf(self, path: Path) -> DocumentExtraction:
        reader = PdfReader(str(path))
        segments: List[TextSegment] = []
        empty_pages: List[int] = []
        for index, page in enumerate(reader.pages, start=1):
            text = str(page.extract_text() or "").strip()
            if text:
                segments.append(TextSegment(text=text, page_from=index, page_to=index))
            else:
                empty_pages.append(index)

        ocr_status = "not_needed"
        last_error = ""
        if empty_pages and self._ocr_enabled:
            ocr_segments, ocr_status, last_error = self._extract_pdf_with_ocr(path, empty_pages)
            if ocr_segments:
                segments.extend(ocr_segments)
        elif empty_pages:
            ocr_status = "skipped"

        segments.sort(key=lambda item: (item.page_from or 0, item.page_to or 0))
        return DocumentExtraction(
            segments=segments,
            parser="pdf:pypdf",
            page_count=len(reader.pages),
            ocr_status=ocr_status,
            last_error=last_error,
        )

    def _extract_pdf_with_ocr(self, path: Path, target_pages: Iterable[int]) -> tuple[List[TextSegment], str, str]:
        _require_document_execution_capability()
        tesseract_runtime = _resolved_document_runtime(
            _DOCUMENT_TESSERACT_ENV,
            {"tesseract", "tesseract.exe"},
        )
        if tesseract_runtime.path is None or not tesseract_runtime.sha256:
            return [], "unavailable", "ocr_runtime_unavailable"
        tesseract_binary = tesseract_runtime.path
        if not _OCR_LANGUAGE_RE.fullmatch(self._ocr_language):
            return [], "unavailable", "ocr_language_invalid"

        try:
            import pypdfium2 as pdfium
        except ImportError:
            return [], "unavailable", "ocr_dependency_unavailable"

        segments: List[TextSegment] = []
        document = None
        try:
            document = pdfium.PdfDocument(str(path))
            for page_number in target_pages:
                page = document.get_page(page_number - 1)
                try:
                    bitmap = page.render(scale=2.0)
                    pil_image = bitmap.to_pil()
                    image_bytes = io.BytesIO()
                    pil_image.save(image_bytes, format="PNG")
                    with tempfile.TemporaryDirectory(prefix="analytix-ocr-runtime-") as temporary:
                        os.chmod(temporary, 0o700)
                        runtime_error = ""
                        try:
                            completed = run_managed_process(
                                [os.fspath(tesseract_binary), "stdin", "stdout", "-l", self._ocr_language],
                                cwd=Path(temporary).resolve(strict=True),
                                input_bytes=image_bytes.getvalue(),
                                timeout_seconds=120,
                                expected_executable_sha256=tesseract_runtime.sha256,
                                capture_stdout=True,
                            )
                        except ManagedProcessTimeout:
                            runtime_error = "ocr_timeout"
                        except ManagedProcessTerminationError:
                            runtime_error = "ocr_process_tree_termination_failed"
                        except ManagedProcessLaunchError:
                            runtime_error = "ocr_runtime_launch_failed"
                        except ManagedProcessOutputLimit:
                            runtime_error = "ocr_output_limit"
                        if runtime_error:
                            raise ExternalRuntimeError(runtime_error)
                    if completed.returncode != 0:
                        raise ExternalRuntimeError("ocr_failed")
                    text = completed.stdout.decode("utf-8", errors="replace").strip()
                finally:
                    page.close()
                if text:
                    segments.append(TextSegment(text=text, page_from=page_number, page_to=page_number))
            if segments:
                return segments, "completed", ""
            return [], "skipped", "ocr_no_text"
        except Exception as exc:
            return [], "failed", _safe_ocr_error_code(exc)
        finally:
            if document is not None:
                try:
                    document.close()
                except Exception:
                    pass

    def _convert_doc_with_soffice(
        self,
        *,
        case_id: str,
        office_binary: str,
        expected_executable_sha256: str,
        path: Path,
    ) -> Path:
        _require_document_execution_capability()
        with tempfile.TemporaryDirectory(prefix="analytix-document-runtime-") as temporary:
            runtime_dir = Path(temporary).resolve(strict=True)
            os.chmod(runtime_dir, 0o700)
            opaque_source = runtime_dir / "input.doc"
            staged_output = runtime_dir / "out"
            staged_output.mkdir(mode=0o700)
            source_sha256 = _copy_regular_source(path, opaque_source)
            runtime_error = ""
            try:
                completed = run_managed_process(
                    [office_binary, "--headless", "--convert-to", "docx", "--outdir", "out", opaque_source.name],
                    cwd=runtime_dir,
                    input_bytes=None,
                    timeout_seconds=120,
                    expected_executable_sha256=expected_executable_sha256,
                )
            except ManagedProcessTimeout:
                runtime_error = "document_extract_timeout"
            except ManagedProcessTerminationError:
                runtime_error = "document_process_tree_termination_failed"
            except ManagedProcessLaunchError:
                runtime_error = "document_runtime_launch_failed"
            if runtime_error:
                raise ExternalRuntimeError(runtime_error)
            if completed.returncode != 0:
                raise ExternalRuntimeError("document_extract_failed")
            staged_converted = staged_output / "input.docx"
            return self._publish_converted_document(
                case_id=case_id,
                source_sha256=source_sha256,
                staged_path=staged_converted,
                suffix=".docx",
            )

    def _convert_doc_with_textutil(
        self,
        *,
        case_id: str,
        textutil_binary: str,
        expected_executable_sha256: str,
        path: Path,
    ) -> Path:
        _require_document_execution_capability()
        with tempfile.TemporaryDirectory(prefix="analytix-document-runtime-") as temporary:
            runtime_dir = Path(temporary).resolve(strict=True)
            os.chmod(runtime_dir, 0o700)
            opaque_source = runtime_dir / "input.doc"
            staged_converted = runtime_dir / "output.txt"
            source_sha256 = _copy_regular_source(path, opaque_source)
            runtime_error = ""
            try:
                completed = run_managed_process(
                    [textutil_binary, "-convert", "txt", "-output", staged_converted.name, opaque_source.name],
                    cwd=runtime_dir,
                    input_bytes=None,
                    timeout_seconds=120,
                    expected_executable_sha256=expected_executable_sha256,
                )
            except ManagedProcessTimeout:
                runtime_error = "document_extract_timeout"
            except ManagedProcessTerminationError:
                runtime_error = "document_process_tree_termination_failed"
            except ManagedProcessLaunchError:
                runtime_error = "document_runtime_launch_failed"
            if runtime_error:
                raise ExternalRuntimeError(runtime_error)
            if completed.returncode != 0:
                raise ExternalRuntimeError("document_extract_failed")
            return self._publish_converted_document(
                case_id=case_id,
                source_sha256=source_sha256,
                staged_path=staged_converted,
                suffix=".txt",
            )

    def _publish_converted_document(
        self,
        *,
        case_id: str,
        source_sha256: str,
        staged_path: Path,
        suffix: str,
    ) -> Path:
        source_digest = _normalized_document_sha256(source_sha256)
        if suffix not in {".docx", ".txt"}:
            raise ExternalRuntimeError("document_output_invalid")
        staged_digest = _regular_file_sha256(staged_path, missing_code="document_output_missing")
        target_dir = self._storage.case_dir(case_id) / "knowledge" / "_docconvert" / source_digest
        target_name = f"document{suffix}"
        target = target_dir / target_name
        target_dir_fd = -1
        publish_lock_fd = -1
        temporary_name = f".generation-stage-v1-{secrets.token_hex(24)}.tmp"
        temporary_fd = -1
        published_new = False
        completed = False
        rollback_error: ExternalRuntimeError | None = None
        try:
            target_dir_fd = _open_private_document_target_directory(
                self._storage.case_dir(case_id),
                source_digest,
            )
            publish_lock_fd = _open_document_publish_lock(target_dir_fd)
            _revalidate_document_directory_path(target_dir_fd, target_dir)
            _cleanup_document_temporaries(target_dir_fd)
            existing_digest = _document_target_digest(target_dir_fd, target_name)
            if existing_digest is not None:
                if existing_digest != staged_digest:
                    raise ExternalRuntimeError("document_publish_conflict")
                return target

            temporary_fd = os.open(
                temporary_name,
                os.O_WRONLY
                | os.O_CREAT
                | os.O_EXCL
                | int(getattr(os, "O_CLOEXEC", 0))
                | int(getattr(os, "O_NOFOLLOW", 0)),
                0o600,
                dir_fd=target_dir_fd,
            )
            copied_digest = _copy_regular_document_output(staged_path, temporary_fd)
            if copied_digest != staged_digest:
                raise ExternalRuntimeError("document_output_invalid")
            os.fsync(temporary_fd)
            os.close(temporary_fd)
            temporary_fd = -1
            try:
                os.link(
                    temporary_name,
                    target_name,
                    src_dir_fd=target_dir_fd,
                    dst_dir_fd=target_dir_fd,
                    follow_symlinks=False,
                )
                published_new = True
            except FileExistsError:
                if _document_target_digest(target_dir_fd, target_name) != staged_digest:
                    raise ExternalRuntimeError("document_publish_conflict") from None
            os.fsync(target_dir_fd)
            os.unlink(temporary_name, dir_fd=target_dir_fd)
            os.fsync(target_dir_fd)
            _revalidate_document_directory_path(target_dir_fd, target_dir)
            if _document_target_digest(target_dir_fd, target_name) != staged_digest:
                raise ExternalRuntimeError("document_publish_readback_failed")
            completed = True
            return target
        except ExternalRuntimeError:
            raise
        except OSError:
            raise ExternalRuntimeError("document_publish_failed") from None
        finally:
            if temporary_fd >= 0:
                os.close(temporary_fd)
            if target_dir_fd >= 0:
                try:
                    os.unlink(temporary_name, dir_fd=target_dir_fd)
                except FileNotFoundError:
                    pass
                except OSError:
                    pass
                if published_new and not completed:
                    try:
                        _unlink_owned_document_target(
                            target_dir_fd,
                            target_name,
                            expected_sha256=staged_digest,
                            missing_ok=True,
                        )
                    except ExternalRuntimeError as exc:
                        rollback_error = exc
                if publish_lock_fd >= 0:
                    os.close(publish_lock_fd)
                os.close(target_dir_fd)
            if rollback_error is not None:
                raise ExternalRuntimeError("document_publish_indeterminate") from None

    def _build_chunk_rows(
        self,
        *,
        case_id: str,
        document_id: str,
        file_id: str,
        segments: List[TextSegment],
    ) -> List[dict]:
        rows: List[dict] = []
        chunk_index = 1
        now = _now_iso()
        for segment in segments:
            for chunk_text in self._split_text(segment.text):
                vector_data, vector_dim, vector_model, vector_status = self._vectorize_text(chunk_text)
                rows.append(
                    {
                        "chunk_id": hashlib.md5(f"{document_id}:{chunk_index}:{chunk_text}".encode("utf-8")).hexdigest()[:24],
                        "case_id": case_id,
                        "document_id": document_id,
                        "file_id": file_id,
                        "chunk_index": chunk_index,
                        "page_from": segment.page_from,
                        "page_to": segment.page_to,
                        "text": chunk_text,
                        "text_chars": len(chunk_text),
                        "token_estimate": self._estimate_tokens(chunk_text),
                        "vector_model": vector_model,
                        "vector_dim": vector_dim,
                        "vector_data": vector_data,
                        "created_at": now,
                        "_vector_status": vector_status,
                    }
                )
                chunk_index += 1
        return rows

    def _insert_chunk_row(self, engine: DuckDBEngine, row: dict) -> None:
        engine.execute(
            """INSERT INTO document_chunks(
                   chunk_id, case_id, document_id, file_id, chunk_index, page_from, page_to,
                   text, text_chars, token_estimate, vector_model, vector_dim, vector_data, created_at
               ) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)""",
            (
                row["chunk_id"],
                row["case_id"],
                row["document_id"],
                row["file_id"],
                int(row["chunk_index"]),
                row["page_from"],
                row["page_to"],
                row["text"],
                int(row["text_chars"]),
                int(row["token_estimate"]),
                row["vector_model"],
                int(row["vector_dim"]),
                row["vector_data"],
                row["created_at"],
            ),
        )

    def plan_document_generation_recovery(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_id: str,
        file_id: str,
    ) -> DocumentGenerationRecoveryPlan:
        directory_fd = -1
        try:
            opened = _open_private_document_intent_directory(
                self._storage.case_dir(case_id),
                _document_text_storage_key(document_id),
                create=False,
            )
            if opened is None:
                return DocumentGenerationRecoveryPlan(action="none")
            directory_fd, _directory_path = opened
            return self._plan_document_generation_recovery_in_directory(
                engine,
                case_id=case_id,
                document_id=document_id,
                file_id=file_id,
                directory_fd=directory_fd,
            )
        except Exception:
            return DocumentGenerationRecoveryPlan(
                action="quarantine",
                reason="document_intent_recovery_blocked",
            )
        finally:
            if directory_fd >= 0:
                os.close(directory_fd)

    def apply_document_generation_recovery(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_id: str,
        file_id: str,
    ) -> DocumentGenerationRecoveryPlan:
        require_controlled_source_ingestion()
        with engine.connection_operation():
            return self._apply_document_generation_recovery_exclusive(
                engine,
                case_id=case_id,
                document_id=document_id,
                file_id=file_id,
            )

    def _apply_document_generation_recovery_exclusive(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_id: str,
        file_id: str,
    ) -> DocumentGenerationRecoveryPlan:
        root_fd = -1
        root_lock_fd = -1
        directory_fd = -1
        lock_fd = -1
        try:
            if self._document_engine_transaction_is_active(engine):
                return DocumentGenerationRecoveryPlan(
                    action="quarantine",
                    reason="document_intent_recovery_blocked",
                )
            opened_root = _open_private_document_intent_directory(
                self._storage.case_dir(case_id),
                None,
                create=False,
            )
            if opened_root is None:
                return DocumentGenerationRecoveryPlan(action="none")
            root_fd, _root_path = opened_root
            root_lock_fd = _open_existing_document_publish_lock(root_fd)
            opened = _open_private_document_intent_directory(
                self._storage.case_dir(case_id),
                _document_text_storage_key(document_id),
                create=False,
            )
            if opened is None:
                return DocumentGenerationRecoveryPlan(action="none")
            directory_fd, _directory_path = opened
            lock_fd = _open_existing_document_publish_lock(directory_fd)
            plan = self._plan_document_generation_recovery_in_directory(
                engine,
                case_id=case_id,
                document_id=document_id,
                file_id=file_id,
                directory_fd=directory_fd,
            )
            return self._apply_document_generation_recovery_in_directory(
                engine,
                case_id=case_id,
                document_id=document_id,
                file_id=file_id,
                directory_fd=directory_fd,
                plan=plan,
            )
        except Exception:
            return DocumentGenerationRecoveryPlan(
                action="quarantine",
                reason="document_intent_recovery_blocked",
            )
        finally:
            if lock_fd >= 0:
                os.close(lock_fd)
            if directory_fd >= 0:
                os.close(directory_fd)
            if root_lock_fd >= 0:
                os.close(root_lock_fd)
            if root_fd >= 0:
                os.close(root_fd)

    def _plan_document_generation_recovery_in_directory(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_id: str,
        file_id: str,
        directory_fd: int,
    ) -> DocumentGenerationRecoveryPlan:
        loaded = self._load_document_intent_journal(directory_fd)
        if loaded is None:
            return DocumentGenerationRecoveryPlan(action="none")
        prepared = loaded.prepared
        binding = prepared.get("binding")
        if not isinstance(binding, dict) or binding != {
            "caseIdSha256": _document_binding_hash("case", case_id),
            "documentIdSha256": _document_binding_hash("document", document_id),
            "fileIdSha256": _document_binding_hash("file", file_id),
        }:
            return DocumentGenerationRecoveryPlan(
                action="quarantine",
                reason="document_intent_binding_mismatch",
                intent_id=loaded.intent_id,
                phase=loaded.latest_phase,
                state=loaded,
            )
        if prepared.get("rootIdentity") != _document_case_root_identity(
            self._storage.case_dir(case_id)
        ):
            return DocumentGenerationRecoveryPlan(
                action="quarantine",
                reason="document_intent_root_identity_mismatch",
                intent_id=loaded.intent_id,
                phase=loaded.latest_phase,
                state=loaded,
            )
        old_state = prepared.get("oldState")
        new_state = prepared.get("newState")
        if not isinstance(old_state, dict) or not isinstance(new_state, dict):
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        current = self._document_database_state(
            engine,
            case_id=case_id,
            document_id=document_id,
        )
        current_digest = str(current.get("stateSha256") or "")
        old_digest = str(old_state.get("stateSha256") or "")
        new_digest = str(new_state.get("stateSha256") or "")
        matches_old = bool(current_digest) and current_digest == old_digest
        matches_new = bool(current_digest) and current_digest == new_digest
        if matches_old == matches_new:
            return DocumentGenerationRecoveryPlan(
                action="quarantine",
                reason="document_intent_database_state_ambiguous",
                intent_id=loaded.intent_id,
                phase=loaded.latest_phase,
                state=loaded,
            )
        expected_outcome = "rolled_back" if matches_old else "committed"
        if loaded.terminal_outcome and loaded.terminal_outcome != expected_outcome:
            return DocumentGenerationRecoveryPlan(
                action="quarantine",
                reason="document_intent_terminal_state_mismatch",
                intent_id=loaded.intent_id,
                phase=loaded.latest_phase,
                state=loaded,
            )
        return DocumentGenerationRecoveryPlan(
            action="rollback_new" if matches_old else "finalize_new",
            reason="document_intent_recovery_required",
            intent_id=loaded.intent_id,
            phase=loaded.latest_phase,
            state=loaded,
        )

    def _apply_document_generation_recovery_in_directory(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_id: str,
        file_id: str,
        directory_fd: int,
        plan: DocumentGenerationRecoveryPlan,
    ) -> DocumentGenerationRecoveryPlan:
        _ = file_id
        if plan.action in {"none", "quarantine"}:
            return plan
        state = plan.state
        if state is None:
            return DocumentGenerationRecoveryPlan(
                action="quarantine",
                reason="document_intent_recovery_blocked",
            )
        prepared = state.prepared
        old_state = prepared["oldState"]
        new_state = prepared["newState"]
        if plan.action == "rollback_new":
            self._verify_old_intent_generation(
                engine,
                case_id=case_id,
                document_id=document_id,
                descriptor=old_state.get("contentGeneration"),
            )
            self._resolve_intent_staging_state(
                case_id=case_id,
                descriptor=new_state.get("contentGeneration"),
                target_required=False,
            )
            self._remove_intent_generation_if_unreferenced(
                engine,
                case_id=case_id,
                document_id=document_id,
                descriptor=new_state.get("contentGeneration"),
            )
            outcome = "rolled_back"
        elif plan.action == "finalize_new":
            self._resolve_intent_staging_state(
                case_id=case_id,
                descriptor=new_state.get("contentGeneration"),
                target_required=True,
            )
            self._verify_intent_generation(
                case_id=case_id,
                document_id=document_id,
                descriptor=new_state.get("contentGeneration"),
                required=True,
            )
            self._remove_intent_generation_if_unreferenced(
                engine,
                case_id=case_id,
                document_id=document_id,
                descriptor=old_state.get("contentGeneration"),
            )
            outcome = "committed"
        else:
            return DocumentGenerationRecoveryPlan(
                action="quarantine",
                reason="document_intent_recovery_blocked",
                intent_id=state.intent_id,
                phase=state.latest_phase,
                state=state,
            )

        if state.terminal_outcome:
            terminal_state = state
        else:
            terminal_state = self._append_document_intent_phase(
                directory_fd,
                state,
                sequence=_DOCUMENT_INTENT_TERMINAL_SEQUENCE,
                phase="terminal",
                outcome=outcome,
            )
        self._document_generation_crash_cut("terminal_recorded")
        _unlink_owned_document_target(
            directory_fd,
            ".active-intent-v1.json",
            expected_sha256=terminal_state.active_sha256,
            missing_ok=False,
        )
        return DocumentGenerationRecoveryPlan(
            action="none",
            reason=f"document_intent_{outcome}",
        )

    def _verify_old_intent_generation(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_id: str,
        descriptor: Any,
    ) -> None:
        self._validate_document_content_generation(descriptor)
        if descriptor["kind"] == "none":
            return
        if descriptor["kind"] != "unmanaged":
            self._verify_intent_generation(
                case_id=case_id,
                document_id=document_id,
                descriptor=descriptor,
                required=True,
            )
            return
        asset = self._get_asset_row(engine, case_id=case_id, document_id=document_id)
        content_path = str(asset.get("content_path") or "") if asset else ""
        if (
            not content_path
            or _document_binding_hash("content-path", content_path)
            != descriptor["pathSha256"]
        ):
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        digest = _regular_file_sha256(
            Path(content_path),
            missing_code="document_intent_recovery_blocked",
        )
        if digest != descriptor["contentSha256"]:
            raise ExternalRuntimeError("document_intent_recovery_blocked")

    def _resolve_intent_staging_state(
        self,
        *,
        case_id: str,
        descriptor: Any,
        target_required: bool,
    ) -> None:
        self._validate_document_content_generation(descriptor)
        if descriptor["kind"] != "immutable_v1" or not descriptor["stagingName"]:
            return
        opened = _try_open_private_document_text_directory(
            self._storage.case_dir(case_id),
            descriptor["documentKey"],
        )
        if opened is None:
            if target_required:
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            return
        directory_fd, directory_path = opened
        staging_name = descriptor["stagingName"]
        target_name = descriptor["targetName"]
        publish_lock_fd = -1
        staging_fd = -1
        target_fd = -1
        try:
            publish_lock_fd = _open_document_publish_lock(directory_fd)
            _revalidate_document_directory_path(directory_fd, directory_path)
            try:
                path_stat = os.stat(staging_name, dir_fd=directory_fd, follow_symlinks=False)
            except FileNotFoundError:
                return
            staging_fd = os.open(
                staging_name,
                os.O_RDONLY
                | int(getattr(os, "O_CLOEXEC", 0))
                | int(getattr(os, "O_NOFOLLOW", 0)),
                dir_fd=directory_fd,
            )
            opened_stat = os.fstat(staging_fd)
            get_euid = getattr(os, "geteuid", None)
            if (
                stat.S_ISLNK(path_stat.st_mode)
                or not stat.S_ISREG(path_stat.st_mode)
                or not stat.S_ISREG(opened_stat.st_mode)
                or int(opened_stat.st_nlink) not in {1, 2}
                or _document_file_identity(path_stat) != _document_file_identity(opened_stat)
                or (stat.S_IMODE(opened_stat.st_mode) & 0o077) != 0
                or (get_euid is not None and int(opened_stat.st_uid) != int(get_euid()))
            ):
                raise OSError
            try:
                target_stat = os.stat(target_name, dir_fd=directory_fd, follow_symlinks=False)
            except FileNotFoundError:
                if target_required or int(opened_stat.st_nlink) != 1:
                    raise OSError
                target_stat = None
            if target_stat is not None:
                target_fd = os.open(
                    target_name,
                    os.O_RDONLY
                    | int(getattr(os, "O_CLOEXEC", 0))
                    | int(getattr(os, "O_NOFOLLOW", 0)),
                    dir_fd=directory_fd,
                )
                target_opened = os.fstat(target_fd)
                if (
                    stat.S_ISLNK(target_stat.st_mode)
                    or not stat.S_ISREG(target_stat.st_mode)
                    or not stat.S_ISREG(target_opened.st_mode)
                    or int(opened_stat.st_nlink) != 2
                    or int(target_opened.st_nlink) != 2
                    or _document_file_identity(opened_stat)
                    != _document_file_identity(target_opened)
                    or _document_file_identity(target_stat)
                    != _document_file_identity(target_opened)
                ):
                    raise OSError
                digest = hashlib.sha256()
                while True:
                    chunk = os.read(target_fd, 1024 * 1024)
                    if not chunk:
                        break
                    digest.update(chunk)
                if digest.hexdigest() != descriptor["contentSha256"]:
                    raise OSError
            staging_after = os.stat(staging_name, dir_fd=directory_fd, follow_symlinks=False)
            if _document_file_identity(opened_stat) != _document_file_identity(staging_after):
                raise OSError
            os.unlink(staging_name, dir_fd=directory_fd)
            os.fsync(directory_fd)
        except OSError:
            raise ExternalRuntimeError("document_intent_recovery_blocked") from None
        finally:
            if target_fd >= 0:
                os.close(target_fd)
            if staging_fd >= 0:
                os.close(staging_fd)
            if publish_lock_fd >= 0:
                os.close(publish_lock_fd)
            os.close(directory_fd)

    def _assert_intent_staging_absent(
        self,
        *,
        case_id: str,
        descriptor: Any,
    ) -> None:
        self._validate_document_content_generation(descriptor)
        if descriptor["kind"] != "immutable_v1" or not descriptor["stagingName"]:
            return
        opened = _try_open_private_document_text_directory(
            self._storage.case_dir(case_id),
            descriptor["documentKey"],
        )
        if opened is None:
            return
        directory_fd, directory_path = opened
        try:
            _revalidate_document_directory_path(directory_fd, directory_path)
            try:
                os.stat(
                    descriptor["stagingName"],
                    dir_fd=directory_fd,
                    follow_symlinks=False,
                )
            except FileNotFoundError:
                return
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        finally:
            os.close(directory_fd)

    def _recover_document_intent_after_exception(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_id: str,
        file_id: str,
        directory_fd: int,
        original: BaseException,
    ) -> BaseException | None:
        try:
            with engine.connection_operation():
                return self._recover_document_intent_after_exception_exclusive(
                    engine,
                    case_id=case_id,
                    document_id=document_id,
                    file_id=file_id,
                    directory_fd=directory_fd,
                    original=original,
                )
        except Exception:
            return ExternalRuntimeError("document_database_state_indeterminate")

    def _recover_document_intent_after_exception_exclusive(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_id: str,
        file_id: str,
        directory_fd: int,
        original: BaseException,
    ) -> BaseException | None:
        try:
            if self._document_engine_transaction_is_active(engine):
                return ExternalRuntimeError("document_database_state_indeterminate")
            plan = self._plan_document_generation_recovery_in_directory(
                engine,
                case_id=case_id,
                document_id=document_id,
                file_id=file_id,
                directory_fd=directory_fd,
            )
            recovery_action = plan.action
            if plan.action != "none":
                plan = self._apply_document_generation_recovery_in_directory(
                    engine,
                    case_id=case_id,
                    document_id=document_id,
                    file_id=file_id,
                    directory_fd=directory_fd,
                    plan=plan,
                )
            if plan.action == "quarantine":
                return ExternalRuntimeError("document_intent_recovery_blocked")
            if recovery_action == "finalize_new":
                return None
            return original
        except Exception:
            return ExternalRuntimeError("document_intent_recovery_blocked")

    def _load_document_intent_journal(
        self,
        directory_fd: int,
    ) -> _DocumentIntentJournalState | None:
        active_record = _read_private_document_record(
            directory_fd,
            ".active-intent-v1.json",
            missing_ok=True,
        )
        if active_record is None:
            return None
        active_bytes, active_sha256 = active_record
        active = self._decode_canonical_document_record(active_bytes)
        if set(active) != {"schema", "intentId", "preparedRecordSha256"}:
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        intent_id = str(active.get("intentId") or "")
        prepared_sha256 = str(active.get("preparedRecordSha256") or "")
        if (
            active.get("schema") != _DOCUMENT_INTENT_ACTIVE_SCHEMA
            or not _DOCUMENT_INTENT_ID_RE.fullmatch(intent_id)
            or not _DOCUMENT_INTENT_ID_RE.fullmatch(prepared_sha256)
        ):
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        prepared_name = _document_intent_phase_name(intent_id, 0, "prepared")
        prepared_record = _read_private_document_record(
            directory_fd,
            prepared_name,
            missing_ok=False,
        )
        if prepared_record is None or prepared_record[1] != prepared_sha256:
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        prepared = self._decode_canonical_document_record(prepared_record[0])
        required_prepared_keys = {
            "schema",
            "intentId",
            "phase",
            "sequence",
            "previousRecordSha256",
            "binding",
            "rootIdentity",
            "oldState",
            "newState",
            "dbMutationDigest",
        }
        if (
            set(prepared) != required_prepared_keys
            or prepared.get("schema") != _DOCUMENT_INTENT_SCHEMA
            or prepared.get("intentId") != intent_id
            or prepared.get("phase") != "prepared"
            or prepared.get("sequence") != 0
            or prepared.get("previousRecordSha256") != ""
        ):
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        self._validate_document_intent_prepared(prepared)

        latest_phase = "prepared"
        latest_sequence = 0
        latest_sha256 = prepared_sha256
        saw_gap = False
        for sequence, phase in _DOCUMENT_INTENT_PHASES:
            record = _read_private_document_record(
                directory_fd,
                _document_intent_phase_name(intent_id, sequence, phase),
                missing_ok=True,
            )
            if record is None:
                saw_gap = True
                continue
            if saw_gap:
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            value = self._decode_canonical_document_record(record[0])
            self._validate_document_intent_phase_record(
                value,
                intent_id=intent_id,
                sequence=sequence,
                phase=phase,
                previous_sha256=latest_sha256,
                prepared_sha256=prepared_sha256,
                outcome="",
            )
            latest_phase = phase
            latest_sequence = sequence
            latest_sha256 = record[1]

        terminal_outcome = ""
        terminal_record = _read_private_document_record(
            directory_fd,
            _document_intent_phase_name(
                intent_id,
                _DOCUMENT_INTENT_TERMINAL_SEQUENCE,
                "terminal",
            ),
            missing_ok=True,
        )
        if terminal_record is not None:
            terminal_value = self._decode_canonical_document_record(terminal_record[0])
            terminal_outcome = str(terminal_value.get("outcome") or "")
            if terminal_outcome not in {"rolled_back", "committed"}:
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            self._validate_document_intent_phase_record(
                terminal_value,
                intent_id=intent_id,
                sequence=_DOCUMENT_INTENT_TERMINAL_SEQUENCE,
                phase="terminal",
                previous_sha256=latest_sha256,
                prepared_sha256=prepared_sha256,
                outcome=terminal_outcome,
            )
            latest_phase = "terminal"
            latest_sequence = _DOCUMENT_INTENT_TERMINAL_SEQUENCE
            latest_sha256 = terminal_record[1]

        return _DocumentIntentJournalState(
            active_sha256=active_sha256,
            intent_id=intent_id,
            prepared=prepared,
            prepared_sha256=prepared_sha256,
            latest_phase=latest_phase,
            latest_sequence=latest_sequence,
            latest_sha256=latest_sha256,
            terminal_outcome=terminal_outcome,
        )

    @staticmethod
    def _decode_canonical_document_record(payload: bytes) -> dict[str, Any]:
        try:
            value = json.loads(payload.decode("ascii", errors="strict"))
        except (UnicodeError, json.JSONDecodeError, MemoryError):
            raise ExternalRuntimeError("document_intent_recovery_blocked") from None
        if not isinstance(value, dict) or _canonical_document_json(value) != payload:
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        return value

    @staticmethod
    def _validate_document_intent_phase_record(
        value: dict[str, Any],
        *,
        intent_id: str,
        sequence: int,
        phase: str,
        previous_sha256: str,
        prepared_sha256: str,
        outcome: str,
    ) -> None:
        if set(value) != {
            "schema",
            "intentId",
            "phase",
            "sequence",
            "previousRecordSha256",
            "preparedRecordSha256",
            "outcome",
        }:
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        if value != {
            "schema": _DOCUMENT_INTENT_SCHEMA,
            "intentId": intent_id,
            "phase": phase,
            "sequence": sequence,
            "previousRecordSha256": previous_sha256,
            "preparedRecordSha256": prepared_sha256,
            "outcome": outcome,
        }:
            raise ExternalRuntimeError("document_intent_recovery_blocked")

    @staticmethod
    def _validate_document_intent_prepared(prepared: dict[str, Any]) -> None:
        binding = prepared.get("binding")
        root_identity = prepared.get("rootIdentity")
        old_state = prepared.get("oldState")
        new_state = prepared.get("newState")
        if not isinstance(binding, dict) or set(binding) != {
            "caseIdSha256",
            "documentIdSha256",
            "fileIdSha256",
        }:
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        if not isinstance(root_identity, dict) or set(root_identity) != {
            "pathSha256",
            "device",
            "inode",
            "ownerUid",
            "mode",
        }:
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        required_state_keys = {
            "assetSha256",
            "chunksSha256",
            "stateSha256",
            "contentGeneration",
        }
        if (
            not isinstance(old_state, dict)
            or not isinstance(new_state, dict)
            or set(old_state) != required_state_keys
            or set(new_state) != required_state_keys
        ):
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        hashes = [
            *binding.values(),
            root_identity.get("pathSha256"),
            old_state.get("assetSha256"),
            old_state.get("chunksSha256"),
            old_state.get("stateSha256"),
            new_state.get("assetSha256"),
            new_state.get("chunksSha256"),
            new_state.get("stateSha256"),
            prepared.get("dbMutationDigest"),
        ]
        if any(not isinstance(item, str) or not _DOCUMENT_INTENT_ID_RE.fullmatch(item) for item in hashes):
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        DocumentRepository._validate_document_content_generation(
            old_state.get("contentGeneration")
        )
        DocumentRepository._validate_document_content_generation(
            new_state.get("contentGeneration")
        )
        if new_state["contentGeneration"].get("kind") != "immutable_v1":
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        expected_mutation_digest = _document_json_sha256(
            {
                "binding": binding,
                "rootIdentity": root_identity,
                "oldStateSha256": old_state["stateSha256"],
                "newStateSha256": new_state["stateSha256"],
                "newContentGeneration": new_state["contentGeneration"],
            }
        )
        if prepared.get("dbMutationDigest") != expected_mutation_digest:
            raise ExternalRuntimeError("document_intent_recovery_blocked")

    @staticmethod
    def _validate_document_content_generation(descriptor: Any) -> None:
        if not isinstance(descriptor, dict) or "kind" not in descriptor:
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        kind = descriptor.get("kind")
        if kind == "none":
            if descriptor != {"kind": "none"}:
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            return
        if kind == "unmanaged":
            if set(descriptor) != {"kind", "pathSha256", "contentSha256"}:
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            hashes = (descriptor.get("pathSha256"), descriptor.get("contentSha256"))
        elif kind == "legacy":
            if set(descriptor) != {"kind", "contentSha256"}:
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            hashes = (descriptor.get("contentSha256"),)
        elif kind == "immutable_v1":
            if set(descriptor) != {
                "kind",
                "documentKey",
                "targetName",
                "contentSha256",
                "stagingName",
            }:
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            document_key = descriptor.get("documentKey")
            target_name = descriptor.get("targetName")
            staging_name = descriptor.get("stagingName")
            if (
                not isinstance(document_key, str)
                or not _DOCUMENT_INTENT_ID_RE.fullmatch(document_key)
                or not isinstance(target_name, str)
                or not _DOCUMENT_GENERATION_NAME_RE.fullmatch(target_name)
                or target_name != f"{descriptor.get('contentSha256')}.txt"
                or not isinstance(staging_name, str)
                or (
                    staging_name
                    and not _DOCUMENT_TEMPORARY_NAME_RE.fullmatch(staging_name)
                )
            ):
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            hashes = (descriptor.get("contentSha256"),)
        else:
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        if any(not isinstance(item, str) or not _DOCUMENT_INTENT_ID_RE.fullmatch(item) for item in hashes):
            raise ExternalRuntimeError("document_intent_recovery_blocked")

    def _begin_document_generation_intent(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_id: str,
        file_id: str,
        old_asset: dict | None,
        new_asset: dict,
        new_chunks: List[dict],
        new_content_generation: dict[str, Any],
        directory_fd: int,
    ) -> _DocumentIntentJournalState:
        old_database_state = self._document_database_state(
            engine,
            case_id=case_id,
            document_id=document_id,
        )
        new_database_state = self._document_database_state_from_rows(new_asset, new_chunks)
        old_content_generation = self._describe_document_content_generation(
            case_id=case_id,
            document_id=document_id,
            content_path=str(old_asset.get("content_path") or "") if old_asset else "",
        )
        old_state = {
            **old_database_state,
            "contentGeneration": old_content_generation,
        }
        new_state = {
            **new_database_state,
            "contentGeneration": new_content_generation,
        }
        binding = {
            "caseIdSha256": _document_binding_hash("case", case_id),
            "documentIdSha256": _document_binding_hash("document", document_id),
            "fileIdSha256": _document_binding_hash("file", file_id),
        }
        root_identity = _document_case_root_identity(self._storage.case_dir(case_id))
        mutation_digest = _document_json_sha256(
            {
                "binding": binding,
                "rootIdentity": root_identity,
                "oldStateSha256": old_state["stateSha256"],
                "newStateSha256": new_state["stateSha256"],
                "newContentGeneration": new_content_generation,
            }
        )
        intent_id = secrets.token_hex(32)
        prepared = {
            "schema": _DOCUMENT_INTENT_SCHEMA,
            "intentId": intent_id,
            "phase": "prepared",
            "sequence": 0,
            "previousRecordSha256": "",
            "binding": binding,
            "rootIdentity": root_identity,
            "oldState": old_state,
            "newState": new_state,
            "dbMutationDigest": mutation_digest,
        }
        prepared_sha256 = _write_private_document_record(
            directory_fd,
            _document_intent_phase_name(intent_id, 0, "prepared"),
            prepared,
        )
        active = {
            "schema": _DOCUMENT_INTENT_ACTIVE_SCHEMA,
            "intentId": intent_id,
            "preparedRecordSha256": prepared_sha256,
        }
        active_sha256 = _write_private_document_record(
            directory_fd,
            ".active-intent-v1.json",
            active,
        )
        state = _DocumentIntentJournalState(
            active_sha256=active_sha256,
            intent_id=intent_id,
            prepared=prepared,
            prepared_sha256=prepared_sha256,
            latest_phase="prepared",
            latest_sequence=0,
            latest_sha256=prepared_sha256,
            terminal_outcome="",
        )
        self._document_generation_crash_cut("intent_prepared")
        return state

    def _append_document_intent_phase(
        self,
        directory_fd: int,
        state: _DocumentIntentJournalState,
        *,
        sequence: int,
        phase: str,
        outcome: str = "",
    ) -> _DocumentIntentJournalState:
        if sequence <= state.latest_sequence or state.terminal_outcome:
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        value = {
            "schema": _DOCUMENT_INTENT_SCHEMA,
            "intentId": state.intent_id,
            "phase": phase,
            "sequence": sequence,
            "previousRecordSha256": state.latest_sha256,
            "preparedRecordSha256": state.prepared_sha256,
            "outcome": outcome,
        }
        record_sha256 = _write_private_document_record(
            directory_fd,
            _document_intent_phase_name(state.intent_id, sequence, phase),
            value,
        )
        return replace(
            state,
            latest_phase=phase,
            latest_sequence=sequence,
            latest_sha256=record_sha256,
            terminal_outcome=outcome if phase == "terminal" else "",
        )

    def _document_database_state(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_id: str,
    ) -> dict[str, str]:
        asset = self._get_asset_row(engine, case_id=case_id, document_id=document_id)
        rows = engine.query(
            """SELECT chunk_id, case_id, document_id, file_id, chunk_index, page_from, page_to,
                      text, text_chars, token_estimate, vector_model, vector_dim, vector_data, created_at
               FROM document_chunks
               WHERE case_id=? AND document_id=?
               ORDER BY chunk_index ASC, chunk_id ASC""",
            (case_id, document_id),
        )
        chunks = [dict(zip(_DOCUMENT_CHUNK_DIGEST_FIELDS, row)) for row in rows]
        return self._document_database_state_from_rows(asset, chunks)

    @staticmethod
    def _document_database_state_from_rows(
        asset: dict | None,
        chunks: List[dict],
    ) -> dict[str, str]:
        asset_projection = (
            None
            if asset is None
            else {name: asset.get(name) for name in _DOCUMENT_ASSET_DIGEST_FIELDS}
        )
        chunk_projection = [
            {name: row.get(name) for name in _DOCUMENT_CHUNK_DIGEST_FIELDS}
            for row in chunks
        ]
        asset_sha256 = _document_json_sha256(asset_projection)
        chunks_sha256 = _document_json_sha256(chunk_projection)
        return {
            "assetSha256": asset_sha256,
            "chunksSha256": chunks_sha256,
            "stateSha256": _document_json_sha256(
                {
                    "assetSha256": asset_sha256,
                    "chunksSha256": chunks_sha256,
                }
            ),
        }

    def _describe_document_content_generation(
        self,
        *,
        case_id: str,
        document_id: str,
        content_path: str,
    ) -> dict[str, Any]:
        if not content_path:
            return {"kind": "none"}
        target = Path(content_path)
        if not target.is_absolute():
            raise ExternalRuntimeError("document_intent_recovery_blocked")
        case_root = Path(os.path.abspath(os.fspath(self._storage.case_dir(case_id))))
        immutable_root = case_root / "knowledge" / "texts"
        if (
            target.parent.parent == immutable_root
            and _DOCUMENT_INTENT_ID_RE.fullmatch(target.parent.name)
            and _DOCUMENT_GENERATION_NAME_RE.fullmatch(target.name)
        ):
            descriptor = {
                "kind": "immutable_v1",
                "documentKey": target.parent.name,
                "targetName": target.name,
                "contentSha256": target.stem,
                "stagingName": "",
            }
            self._verify_intent_generation(
                case_id=case_id,
                document_id=document_id,
                descriptor=descriptor,
                required=True,
            )
            return descriptor
        legacy_name = f"{document_id}.txt"
        if (
            Path(legacy_name).name == legacy_name
            and target == immutable_root / legacy_name
        ):
            digest = _regular_file_sha256(target, missing_code="document_intent_recovery_blocked")
            return {"kind": "legacy", "contentSha256": digest}
        digest = _regular_file_sha256(target, missing_code="document_intent_recovery_blocked")
        return {
            "kind": "unmanaged",
            "pathSha256": _document_binding_hash("content-path", content_path),
            "contentSha256": digest,
        }

    def _document_generation_path_from_descriptor(
        self,
        *,
        case_id: str,
        document_id: str,
        descriptor: Any,
    ) -> Path | None:
        self._validate_document_content_generation(descriptor)
        kind = descriptor["kind"]
        case_root = Path(os.path.abspath(os.fspath(self._storage.case_dir(case_id))))
        if kind == "immutable_v1":
            return (
                case_root
                / "knowledge"
                / "texts"
                / descriptor["documentKey"]
                / descriptor["targetName"]
            )
        if kind == "legacy":
            legacy_name = f"{document_id}.txt"
            if Path(legacy_name).name != legacy_name:
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            return case_root / "knowledge" / "texts" / legacy_name
        return None

    def _verify_intent_generation(
        self,
        *,
        case_id: str,
        document_id: str,
        descriptor: Any,
        required: bool,
    ) -> bool:
        self._validate_document_content_generation(descriptor)
        if descriptor["kind"] in {"none", "unmanaged"}:
            if required and descriptor["kind"] == "none":
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            return descriptor["kind"] == "unmanaged"
        target = self._document_generation_path_from_descriptor(
            case_id=case_id,
            document_id=document_id,
            descriptor=descriptor,
        )
        if target is None:
            if required:
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            return False
        if descriptor["kind"] == "legacy":
            try:
                digest = _regular_file_sha256(
                    target,
                    missing_code="document_intent_recovery_blocked",
                )
            except ExternalRuntimeError:
                if required:
                    raise
                return False
            return digest == descriptor["contentSha256"]
        opened = _try_open_private_document_text_directory(
            self._storage.case_dir(case_id),
            descriptor["documentKey"],
        )
        if opened is None:
            if required:
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            return False
        directory_fd, directory_path = opened
        try:
            _revalidate_document_directory_path(directory_fd, directory_path)
            digest = _document_target_digest(directory_fd, descriptor["targetName"])
            if digest is None:
                if required:
                    raise ExternalRuntimeError("document_intent_recovery_blocked")
                return False
            if digest != descriptor["contentSha256"]:
                raise ExternalRuntimeError("document_intent_recovery_blocked")
            return True
        finally:
            os.close(directory_fd)

    def _remove_intent_generation_if_unreferenced(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_id: str,
        descriptor: Any,
    ) -> None:
        self._validate_document_content_generation(descriptor)
        if descriptor["kind"] in {"none", "unmanaged"}:
            return
        target = self._document_generation_path_from_descriptor(
            case_id=case_id,
            document_id=document_id,
            descriptor=descriptor,
        )
        if target is None:
            return
        references = engine.query(
            "SELECT 1 FROM document_assets WHERE case_id=? AND content_path=? LIMIT 1",
            (case_id, str(target)),
        )
        if references:
            self._verify_intent_generation(
                case_id=case_id,
                document_id=document_id,
                descriptor=descriptor,
                required=True,
            )
            return
        if not self._verify_intent_generation(
            case_id=case_id,
            document_id=document_id,
            descriptor=descriptor,
            required=False,
        ):
            return
        if descriptor["kind"] == "legacy":
            self._remove_managed_text_target(
                case_root=Path(os.path.abspath(os.fspath(self._storage.case_dir(case_id)))),
                document_key=None,
                target_name=f"{document_id}.txt",
                expected_sha256=descriptor["contentSha256"],
            )
            return
        self._remove_managed_text_target(
            case_root=Path(os.path.abspath(os.fspath(self._storage.case_dir(case_id)))),
            document_key=descriptor["documentKey"],
            target_name=descriptor["targetName"],
            expected_sha256=descriptor["contentSha256"],
        )

    def _document_generation_crash_cut(self, phase: str) -> None:
        _ = phase

    def _replace_document_database_generation(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_id: str,
        chunk_rows: List[dict],
        asset_row: dict,
    ) -> None:
        with engine.connection_operation():
            if self._document_engine_transaction_is_active(engine):
                raise _DocumentDatabaseMutationError(
                    "document_database_update_failed",
                    safe_to_remove_new_generation=True,
                )
            transaction_open = False
            begin_attempted = False
            target_content_path = str(asset_row.get("content_path") or "")
            try:
                begin_attempted = True
                engine.execute("BEGIN TRANSACTION")
                transaction_open = True
                self._document_generation_crash_cut("db_begun")
                self._delete_document_chunks(engine, case_id=case_id, document_id=document_id)
                for row in chunk_rows:
                    self._insert_chunk_row(engine, row)
                self._upsert_asset_row(engine, asset_row)
                self._document_generation_crash_cut("db_mutated")
                engine.execute("COMMIT")
                transaction_open = False
                self._document_generation_crash_cut("db_commit_visible")
                return
            except Exception:
                rollback_succeeded = False
                if transaction_open:
                    try:
                        engine.execute("ROLLBACK")
                        rollback_succeeded = True
                    except Exception:
                        rollback_succeeded = False
                elif begin_attempted:
                    try:
                        if self._document_engine_transaction_is_active(engine):
                            engine.execute("ROLLBACK")
                        rollback_succeeded = True
                    except Exception:
                        rollback_succeeded = False
                references_new_generation = self._database_references_content_path(
                    engine,
                    case_id=case_id,
                    document_id=document_id,
                    content_path=target_content_path,
                )
                safe_to_remove = (
                    (not transaction_open or rollback_succeeded)
                    and not references_new_generation
                )
                raise _DocumentDatabaseMutationError(
                    (
                        "document_database_update_failed"
                        if safe_to_remove
                        else "document_database_state_indeterminate"
                    ),
                    safe_to_remove_new_generation=safe_to_remove,
                ) from None

    def _document_engine_transaction_is_active(self, engine: DuckDBEngine) -> bool:
        if bool(getattr(engine, "_remote", False)):
            return bool(getattr(engine, "_remote_in_transaction", False))
        try:
            with engine.connection_operation() as connection:
                first_row = connection.execute("SELECT current_transaction_id()").fetchone()
                second_row = connection.execute("SELECT current_transaction_id()").fetchone()
        except Exception:
            raise _DocumentDatabaseMutationError(
                "document_database_state_indeterminate",
                safe_to_remove_new_generation=False,
            ) from None
        if not first_row or not second_row:
            raise _DocumentDatabaseMutationError(
                "document_database_state_indeterminate",
                safe_to_remove_new_generation=False,
            )
        return int(first_row[0]) == int(second_row[0])

    def _database_references_content_path(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_id: str,
        content_path: str,
    ) -> bool:
        if not content_path:
            return False
        try:
            rows = engine.query(
                "SELECT content_path FROM document_assets WHERE case_id=? AND document_id=? LIMIT 1",
                (case_id, document_id),
            )
        except Exception:
            return True
        return bool(rows and str(rows[0][0] or "") == content_path)

    def _upsert_asset_row(self, engine: DuckDBEngine, row: dict) -> None:
        exists = engine.query("SELECT 1 FROM document_assets WHERE document_id=? LIMIT 1", (row["document_id"],))
        values = (
            row["case_id"],
            row["file_id"],
            row["kind"],
            row["filename"],
            row["display_path"],
            row["stored_path"],
            row["file_type"],
            int(row["size"] or 0),
            row["md5"],
            row["sha256"],
            row["duplicate_of_document_id"],
            row["duplicate_reason"],
            bool(row["selected_for_llm"]),
            bool(row["llm_ready"]),
            row["parser"],
            row["extraction_status"],
            row["ocr_status"],
            row["vector_status"],
            row["vector_model"],
            row["content_path"],
            row["content_excerpt"],
            int(row["content_chars"] or 0),
            int(row["page_count"] or 0),
            int(row["chunk_count"] or 0),
            int(row["token_estimate"] or 0),
            row["last_error"],
            row["updated_at"],
            row["indexed_at"],
            row["document_id"],
        )
        if exists:
            engine.execute(
                """UPDATE document_assets
                   SET case_id=?, file_id=?, kind=?, filename=?, display_path=?, stored_path=?, file_type=?,
                       size=?, md5=?, sha256=?, duplicate_of_document_id=?, duplicate_reason=?,
                       selected_for_llm=COALESCE(selected_for_llm, ?), llm_ready=?, parser=?, extraction_status=?, ocr_status=?,
                       vector_status=?, vector_model=?, content_path=?, content_excerpt=?, content_chars=?,
                       page_count=?, chunk_count=?, token_estimate=?, last_error=?, updated_at=?, indexed_at=?
                   WHERE document_id=?""",
                values,
            )
            return

        engine.execute(
            """INSERT INTO document_assets(
                   document_id, case_id, file_id, kind, filename, display_path, stored_path, file_type,
                   size, md5, sha256, duplicate_of_document_id, duplicate_reason, selected_for_llm,
                   llm_ready, parser, extraction_status, ocr_status, vector_status, vector_model,
                   content_path, content_excerpt, content_chars, page_count, chunk_count, token_estimate,
                   last_error, created_at, updated_at, indexed_at
               ) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)""",
            (
                row["document_id"],
                row["case_id"],
                row["file_id"],
                row["kind"],
                row["filename"],
                row["display_path"],
                row["stored_path"],
                row["file_type"],
                int(row["size"] or 0),
                row["md5"],
                row["sha256"],
                row["duplicate_of_document_id"],
                row["duplicate_reason"],
                bool(row["selected_for_llm"]),
                bool(row["llm_ready"]),
                row["parser"],
                row["extraction_status"],
                row["ocr_status"],
                row["vector_status"],
                row["vector_model"],
                row["content_path"],
                row["content_excerpt"],
                int(row["content_chars"] or 0),
                int(row["page_count"] or 0),
                int(row["chunk_count"] or 0),
                int(row["token_estimate"] or 0),
                row["last_error"],
                row["created_at"],
                row["updated_at"],
                row["indexed_at"],
            ),
        )

    def _delete_document_chunks(self, engine: DuckDBEngine, *, case_id: str, document_id: str) -> None:
        engine.execute("DELETE FROM document_chunks WHERE case_id=? AND document_id=?", (case_id, document_id))

    def _get_asset_row(self, engine: DuckDBEngine, *, case_id: str, document_id: str) -> Optional[dict]:
        rows = engine.query(
            """SELECT document_id, case_id, file_id, kind, filename, display_path, stored_path, file_type,
                      size, md5, sha256, duplicate_of_document_id, duplicate_reason,
                      selected_for_llm, llm_ready, parser, extraction_status, ocr_status,
                      vector_status, vector_model, content_path, content_excerpt, content_chars,
                      page_count, chunk_count, token_estimate, last_error, created_at, updated_at, indexed_at
               FROM document_assets
               WHERE case_id=? AND document_id=?
               LIMIT 1""",
            (case_id, document_id),
        )
        if not rows:
            return None
        return self._asset_row_to_dict(rows[0])

    def _find_duplicate_asset(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        sha256: str,
        exclude_document_id: str,
    ) -> Optional[dict]:
        if not sha256:
            return None
        rows = engine.query(
            """SELECT document_id, case_id, file_id, kind, filename, display_path, stored_path, file_type,
                      size, md5, sha256, duplicate_of_document_id, duplicate_reason,
                      selected_for_llm, llm_ready, parser, extraction_status, ocr_status,
                      vector_status, vector_model, content_path, content_excerpt, content_chars,
                      page_count, chunk_count, token_estimate, last_error, created_at, updated_at, indexed_at
               FROM document_assets
               WHERE case_id=? AND sha256=? AND document_id<>? AND llm_ready=TRUE
               ORDER BY indexed_at DESC
               LIMIT 1""",
            (case_id, sha256, exclude_document_id),
        )
        if not rows:
            return None
        return self._asset_row_to_dict(rows[0])

    def _list_document_chunks(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_id: str,
        limit: int,
    ) -> List[dict]:
        rows = engine.query(
            """SELECT chunk_id, chunk_index, page_from, page_to, text, text_chars,
                      token_estimate, vector_model, vector_dim
               FROM document_chunks
               WHERE case_id=? AND document_id=?
               ORDER BY chunk_index ASC
               LIMIT ?""",
            (case_id, document_id, int(limit)),
        )
        return [
            {
                "chunk_id": str(row[0] or ""),
                "chunk_index": int(row[1] or 0),
                "page_from": int(row[2]) if row[2] is not None else None,
                "page_to": int(row[3]) if row[3] is not None else None,
                "text": str(row[4] or ""),
                "text_chars": int(row[5] or 0),
                "token_estimate": int(row[6] or 0),
                "vector_model": str(row[7] or ""),
                "vector_dim": int(row[8] or 0),
            }
            for row in rows
        ]

    def _write_document_text(self, *, case_id: str, document_id: str, content: str) -> Path:
        return self._publish_document_text_generation(
            case_id=case_id,
            document_id=document_id,
            content=content,
        ).path

    def _publish_document_text_generation(
        self,
        *,
        case_id: str,
        document_id: str,
        content: str,
        temporary_name: str | None = None,
    ) -> _DocumentTextGeneration:
        if not isinstance(content, str) or not content.strip():
            raise ExternalRuntimeError("document_content_unavailable")
        try:
            payload = content.encode("utf-8", errors="strict")
        except (UnicodeError, MemoryError):
            raise ExternalRuntimeError("document_content_invalid") from None
        content_sha256 = hashlib.sha256(payload).hexdigest()
        target_name = f"{content_sha256}.txt"
        document_key = _document_text_storage_key(document_id)
        directory_fd = -1
        publish_lock_fd = -1
        temporary_fd = -1
        resolved_temporary_name = temporary_name or f".generation-stage-v1-{secrets.token_hex(24)}.tmp"
        if not _DOCUMENT_TEMPORARY_NAME_RE.fullmatch(resolved_temporary_name):
            raise ExternalRuntimeError("document_intent_invalid")
        published_new = False
        completed = False
        rollback_error: ExternalRuntimeError | None = None
        try:
            directory_fd, directory_path = _open_private_document_text_directory(
                self._storage.case_dir(case_id),
                document_key,
                create=True,
            )
            publish_lock_fd = _open_document_publish_lock(directory_fd)
            _revalidate_document_directory_path(directory_fd, directory_path)
            existing_digest = _document_target_digest(directory_fd, target_name)
            if existing_digest is not None:
                if existing_digest != content_sha256:
                    raise ExternalRuntimeError("document_publish_conflict")
                completed = True
                return _DocumentTextGeneration(
                    path=directory_path / target_name,
                    document_key=document_key,
                    target_name=target_name,
                    sha256=content_sha256,
                    created=False,
                )

            temporary_fd = os.open(
                resolved_temporary_name,
                os.O_WRONLY
                | os.O_CREAT
                | os.O_EXCL
                | int(getattr(os, "O_CLOEXEC", 0))
                | int(getattr(os, "O_NOFOLLOW", 0)),
                0o600,
                dir_fd=directory_fd,
            )
            _write_document_fd(temporary_fd, payload)
            os.fsync(temporary_fd)
            os.close(temporary_fd)
            temporary_fd = -1
            _revalidate_document_directory_path(directory_fd, directory_path)
            try:
                os.link(
                    resolved_temporary_name,
                    target_name,
                    src_dir_fd=directory_fd,
                    dst_dir_fd=directory_fd,
                    follow_symlinks=False,
                )
                published_new = True
            except FileExistsError:
                if _document_target_digest(directory_fd, target_name) != content_sha256:
                    raise ExternalRuntimeError("document_publish_conflict") from None
            os.fsync(directory_fd)
            os.unlink(resolved_temporary_name, dir_fd=directory_fd)
            os.fsync(directory_fd)
            _revalidate_document_directory_path(directory_fd, directory_path)
            if _document_target_digest(directory_fd, target_name) != content_sha256:
                raise ExternalRuntimeError("document_publish_readback_failed")
            completed = True
            return _DocumentTextGeneration(
                path=directory_path / target_name,
                document_key=document_key,
                target_name=target_name,
                sha256=content_sha256,
                created=published_new,
            )
        except ExternalRuntimeError:
            raise
        except OSError:
            raise ExternalRuntimeError("document_publish_failed") from None
        finally:
            if temporary_fd >= 0:
                os.close(temporary_fd)
            if directory_fd >= 0:
                try:
                    os.unlink(resolved_temporary_name, dir_fd=directory_fd)
                    try:
                        os.fsync(directory_fd)
                    except OSError:
                        pass
                except FileNotFoundError:
                    pass
                except OSError:
                    pass
                if published_new and not completed:
                    try:
                        _unlink_owned_document_target(
                            directory_fd,
                            target_name,
                            expected_sha256=content_sha256,
                            missing_ok=True,
                        )
                    except ExternalRuntimeError as exc:
                        rollback_error = exc
                if publish_lock_fd >= 0:
                    os.close(publish_lock_fd)
                os.close(directory_fd)
            if rollback_error is not None:
                raise ExternalRuntimeError("document_publish_indeterminate") from None

    def _best_effort_remove_text_generation(
        self,
        *,
        case_id: str,
        generation: _DocumentTextGeneration,
    ) -> None:
        try:
            directory_fd, directory_path = _open_private_document_text_directory(
                self._storage.case_dir(case_id),
                generation.document_key,
                create=False,
            )
        except ExternalRuntimeError:
            return
        publish_lock_fd = -1
        try:
            publish_lock_fd = _open_document_publish_lock(directory_fd)
            _revalidate_document_directory_path(directory_fd, directory_path)
            _unlink_owned_document_target(
                directory_fd,
                generation.target_name,
                expected_sha256=generation.sha256,
                missing_ok=True,
            )
        except ExternalRuntimeError:
            pass
        finally:
            if publish_lock_fd >= 0:
                os.close(publish_lock_fd)
            os.close(directory_fd)

    def _delete_managed_text(self, *, case_id: str, document_id: str, content_path: str) -> None:
        if not content_path:
            return
        target = Path(content_path)
        if not target.is_absolute():
            return
        case_root = Path(os.path.abspath(os.fspath(self._storage.case_dir(case_id))))
        document_key = _document_text_storage_key(document_id)
        expected_directory = case_root / "knowledge" / "texts" / document_key
        if target.parent == expected_directory and _DOCUMENT_GENERATION_NAME_RE.fullmatch(target.name):
            self._remove_managed_text_target(
                case_root=case_root,
                document_key=document_key,
                target_name=target.name,
                expected_sha256=target.stem,
            )
            return

        legacy_name = f"{document_id}.txt"
        if Path(legacy_name).name != legacy_name:
            return
        legacy_directory = case_root / "knowledge" / "texts"
        if target.parent != legacy_directory or target.name != legacy_name:
            return
        self._remove_managed_text_target(
            case_root=case_root,
            document_key=None,
            target_name=legacy_name,
            expected_sha256=None,
        )

    def _best_effort_delete_managed_text(
        self,
        *,
        case_id: str,
        document_id: str,
        content_path: str,
    ) -> None:
        try:
            self._delete_managed_text(
                case_id=case_id,
                document_id=document_id,
                content_path=content_path,
            )
        except ExternalRuntimeError:
            pass

    def _best_effort_delete_unreferenced_managed_text(
        self,
        *,
        engine: DuckDBEngine,
        case_id: str,
        document_id: str,
        content_path: str,
    ) -> None:
        if not content_path:
            return
        try:
            references = engine.query(
                "SELECT 1 FROM document_assets WHERE case_id=? AND content_path=? LIMIT 1",
                (case_id, content_path),
            )
        except Exception:
            return
        if references:
            return
        self._best_effort_delete_managed_text(
            case_id=case_id,
            document_id=document_id,
            content_path=content_path,
        )

    def _remove_managed_text_target(
        self,
        *,
        case_root: Path,
        document_key: str | None,
        target_name: str,
        expected_sha256: str | None,
    ) -> None:
        directory_fd, directory_path = _open_private_document_text_directory(
            case_root,
            document_key,
            create=False,
        )
        publish_lock_fd = -1
        try:
            publish_lock_fd = _open_document_publish_lock(directory_fd)
            _revalidate_document_directory_path(directory_fd, directory_path)
            digest = expected_sha256 or _document_target_digest(directory_fd, target_name)
            if digest is None:
                return
            _unlink_owned_document_target(
                directory_fd,
                target_name,
                expected_sha256=digest,
                missing_ok=True,
            )
        finally:
            if publish_lock_fd >= 0:
                os.close(publish_lock_fd)
            os.close(directory_fd)

    def delete_managed_text_path(self, *, case_id: str, content_path: str) -> None:
        if not content_path:
            return
        target = Path(content_path)
        if not target.is_absolute() or not _DOCUMENT_GENERATION_NAME_RE.fullmatch(target.name):
            return
        document_key = target.parent.name
        if len(document_key) != 64 or any(
            character not in "0123456789abcdef" for character in document_key
        ):
            return
        case_root = Path(os.path.abspath(os.fspath(self._storage.case_dir(case_id))))
        expected = case_root / "knowledge" / "texts" / document_key / target.name
        if target != expected:
            return
        try:
            self._remove_managed_text_target(
                case_root=case_root,
                document_key=document_key,
                target_name=target.name,
                expected_sha256=target.stem,
            )
        except ExternalRuntimeError:
            return

    def _case_project_doc_assets(self, case_id: str) -> List[dict]:
        assets: List[dict] = []
        direction = self._case_project_doc_asset(
            case_id,
            document_id=CASE_DIRECTION_DOC_DOCUMENT_ID,
            file_id=CASE_DIRECTION_DOC_FILE_ID,
            filename=CASE_DIRECTION_DOC_FILENAME,
            path_resolver=case_direction_doc_path,
            reader=read_case_direction_doc,
        )
        if direction is not None:
            assets.append(direction)
        brief = self._case_project_doc_asset(
            case_id,
            document_id=CASE_PROJECT_DOC_DOCUMENT_ID,
            file_id=CASE_PROJECT_DOC_FILE_ID,
            filename=CASE_PROJECT_DOC_FILENAME,
            path_resolver=case_project_doc_path,
            reader=read_case_project_doc,
        )
        if brief is not None:
            assets.append(brief)
        return assets

    def _case_project_doc_asset(
        self,
        case_id: str,
        *,
        document_id: str,
        file_id: str,
        filename: str,
        path_resolver,
        reader,
    ) -> Optional[dict]:
        try:
            self._storage.sync_case_project_doc(case_id, source="document")
            path = path_resolver(self._storage.case_dir(case_id))
        except Exception:
            path = path_resolver(self._storage.case_dir(case_id))
        if not path.exists():
            return None

        try:
            raw_bytes = path.read_bytes()
        except OSError:
            raw_bytes = b""
        text = (
            raw_bytes.decode("utf-8", errors="ignore")
            if raw_bytes
            else reader(self._storage.case_dir(case_id))
        )
        try:
            timestamp = datetime.fromtimestamp(path.stat().st_mtime).strftime("%Y-%m-%d %H:%M:%S")
        except OSError:
            timestamp = _now_iso()
        content_bytes = raw_bytes or text.encode("utf-8")
        return {
            "document_id": document_id,
            "case_id": str(case_id or ""),
            "file_id": file_id,
            "kind": CASE_PROJECT_DOC_KIND,
            "filename": filename,
            "display_path": f"案件目录/{filename}",
            "stored_path": str(path),
            "file_type": "md",
            "size": len(content_bytes),
            "md5": hashlib.md5(content_bytes).hexdigest(),
            "sha256": hashlib.sha256(content_bytes).hexdigest(),
            "duplicate_of_document_id": "",
            "duplicate_reason": "",
            "selected_for_llm": False,
            "llm_ready": bool(text.strip()),
            "parser": CASE_PROJECT_DOC_PARSER,
            "extraction_status": "completed" if text.strip() else "pending",
            "ocr_status": "not_needed",
            "vector_status": "not_required",
            "vector_model": "",
            "content_path": str(path),
            "content_excerpt": self._excerpt(text),
            "content_chars": len(text),
            "page_count": 0,
            "chunk_count": 1 if text.strip() else 0,
            "token_estimate": self._estimate_tokens(text),
            "last_error": "" if text.strip() else "case project doc missing",
            "created_at": timestamp,
            "updated_at": timestamp,
            "indexed_at": timestamp,
        }

    @staticmethod
    def _matches_document_filters(
        item: dict,
        *,
        selected_for_llm: Optional[bool],
        llm_ready: Optional[bool],
    ) -> bool:
        if selected_for_llm is not None and bool(item.get("selected_for_llm")) != bool(selected_for_llm):
            return False
        if llm_ready is not None and bool(item.get("llm_ready")) != bool(llm_ready):
            return False
        return True

    def _case_project_doc_detail(
        self,
        *,
        case_id: str,
        document_id: str,
        include_chunks: bool,
        chunk_limit: int,
    ) -> Optional[dict]:
        if document_id == CASE_DIRECTION_DOC_DOCUMENT_ID:
            asset = self._case_project_doc_asset(
                case_id,
                document_id=CASE_DIRECTION_DOC_DOCUMENT_ID,
                file_id=CASE_DIRECTION_DOC_FILE_ID,
                filename=CASE_DIRECTION_DOC_FILENAME,
                path_resolver=case_direction_doc_path,
                reader=read_case_direction_doc,
            )
        else:
            asset = self._case_project_doc_asset(
                case_id,
                document_id=CASE_PROJECT_DOC_DOCUMENT_ID,
                file_id=CASE_PROJECT_DOC_FILE_ID,
                filename=CASE_PROJECT_DOC_FILENAME,
                path_resolver=case_project_doc_path,
                reader=read_case_project_doc,
            )
        if asset is None:
            return None
        text = self._read_text_preview(str(asset.get("content_path") or ""), limit=80_000)
        chunks: List[dict] = []
        if include_chunks and text.strip():
            for index, chunk_text in enumerate(self._split_text(text)[: max(1, int(chunk_limit))]):
                chunks.append(
                    {
                        "chunk_id": f"{document_id}:chunk:{index}",
                        "chunk_index": index,
                        "page_from": None,
                        "page_to": None,
                        "text": chunk_text,
                        "text_chars": len(chunk_text),
                        "token_estimate": self._estimate_tokens(chunk_text),
                        "vector_model": "",
                        "vector_dim": 0,
                    }
                )
        return {
            "document": asset,
            "resolved_document_id": document_id,
            "text_preview": text,
            "chunks": chunks,
        }

    @staticmethod
    def _read_text_preview(content_path: str, limit: int = 4000) -> str:
        if not content_path:
            return ""
        target = Path(content_path)
        if not target.exists():
            return ""
        try:
            return target.read_text(encoding="utf-8", errors="ignore")[:limit]
        except Exception:
            return ""

    @staticmethod
    def _asset_row_to_dict(row: tuple) -> dict:
        return {
            "document_id": str(row[0] or ""),
            "case_id": str(row[1] or ""),
            "file_id": str(row[2] or ""),
            "kind": str(row[3] or ""),
            "filename": str(row[4] or ""),
            "display_path": str(row[5] or ""),
            "stored_path": str(row[6] or ""),
            "file_type": str(row[7] or ""),
            "size": int(row[8] or 0),
            "md5": str(row[9] or ""),
            "sha256": str(row[10] or ""),
            "duplicate_of_document_id": str(row[11] or ""),
            "duplicate_reason": str(row[12] or ""),
            "selected_for_llm": bool(row[13]),
            "llm_ready": bool(row[14]),
            "parser": str(row[15] or ""),
            "extraction_status": str(row[16] or ""),
            "ocr_status": str(row[17] or ""),
            "vector_status": str(row[18] or ""),
            "vector_model": str(row[19] or ""),
            "content_path": str(row[20] or ""),
            "content_excerpt": str(row[21] or ""),
            "content_chars": int(row[22] or 0),
            "page_count": int(row[23] or 0),
            "chunk_count": int(row[24] or 0),
            "token_estimate": int(row[25] or 0),
            "last_error": str(row[26] or ""),
            "created_at": str(row[27] or ""),
            "updated_at": str(row[28] or ""),
            "indexed_at": str(row[29] or ""),
        }

    @staticmethod
    def _join_segments(segments: List[TextSegment]) -> str:
        parts = [DocumentRepository._normalize_text(item.text) for item in segments]
        return "\n\n".join([item for item in parts if item])

    def _split_text(self, text: str) -> List[str]:
        normalized = self._normalize_text(text)
        if not normalized:
            return []
        chunks: List[str] = []
        start = 0
        total = len(normalized)
        while start < total:
            end = min(start + self._chunk_size, total)
            if end < total:
                boundary = self._seek_boundary(normalized, start, end)
                if boundary > start + max(180, self._chunk_size // 2):
                    end = boundary
            chunk = normalized[start:end].strip()
            if chunk:
                chunks.append(chunk)
            if end >= total:
                break
            start = max(end - self._chunk_overlap, start + 1)
        return chunks

    @staticmethod
    def _normalize_text(text: str) -> str:
        compact = _WHITESPACE_RE.sub(" ", str(text or ""))
        compact = re.sub(r"\n{3,}", "\n\n", compact.replace("\r\n", "\n").replace("\r", "\n"))
        return compact.strip()

    @staticmethod
    def _seek_boundary(text: str, start: int, end: int) -> int:
        for index in range(end - 1, start, -1):
            if _PUNCTUATION_BOUNDARY_RE.match(text[index]):
                return index + 1
        return end

    def _vectorize_text(self, text: str) -> tuple[str, int, str, str]:
        if self._vector_disabled:
            return "", 0, "", "skipped"

        dims = 256
        values = [0.0] * dims
        normalized = self._normalize_text(text).lower()
        if not normalized:
            return "[]", dims, self._vector_mode, "failed"

        source = normalized if len(normalized) <= 4096 else normalized[:4096]
        grams = [source] if len(source) <= 3 else [source[index : index + 3] for index in range(len(source) - 2)]
        for gram in grams:
            digest = hashlib.sha256(gram.encode("utf-8")).digest()
            bucket = int.from_bytes(digest[:2], "big") % dims
            sign = 1.0 if digest[2] % 2 == 0 else -1.0
            values[bucket] += sign

        norm = math.sqrt(sum(value * value for value in values))
        if norm > 0:
            values = [round(value / norm, 6) for value in values]
        return json.dumps(values, ensure_ascii=False), dims, self._vector_mode, "completed"

    @staticmethod
    def _estimate_tokens(text: str) -> int:
        normalized = DocumentRepository._normalize_text(text)
        if not normalized:
            return 0
        return max(1, int(math.ceil(len(normalized) / 2.2)))

    @staticmethod
    def _excerpt(text: str, limit: int = 240) -> str:
        normalized = DocumentRepository._normalize_text(text)
        if len(normalized) <= limit:
            return normalized
        return normalized[: limit - 1].rstrip() + "…"

    @staticmethod
    def _file_type_parser(file_type: str) -> str:
        suffix = str(file_type or "").strip().lower()
        return suffix or "document"

    @staticmethod
    def _looks_like_pdf(*, file_type: str, stored_path: str) -> bool:
        if str(file_type or "").strip().lower() == "pdf":
            return True
        return Path(stored_path).suffix.lower() == ".pdf"
