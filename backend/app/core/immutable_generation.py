from __future__ import annotations

import contextlib
import hashlib
import io
import json
import os
import secrets
import stat
from dataclasses import dataclass
from pathlib import Path
from typing import BinaryIO, Callable, Iterator, Mapping, Optional


_BUFFER_SIZE = 1024 * 1024
_PRIVATE_DIRECTORY_MODE = 0o700
_PRIVATE_FILE_MODE = 0o600


class ImmutableGenerationError(RuntimeError):
    """Raised when an immutable publication cannot be proven safe."""


@dataclass(frozen=True)
class RegularSourceReceipt:
    device: int
    inode: int
    uid: int
    mode: int
    links: int
    size: int
    mtime_ns: int
    ctime_ns: int
    generation: int
    sha256: str

    def as_dict(self) -> dict[str, object]:
        return {
            "device": self.device,
            "inode": self.inode,
            "uid": self.uid,
            "mode": self.mode,
            "links": self.links,
            "size": self.size,
            "mtime_ns": self.mtime_ns,
            "ctime_ns": self.ctime_ns,
            "generation": self.generation,
            "sha256": self.sha256,
        }


@dataclass(frozen=True)
class PrivateGenerationRootReceipt:
    device: int
    inode: int
    uid: int
    mode: int

    def as_dict(self) -> dict[str, int]:
        return {
            "device": self.device,
            "inode": self.inode,
            "uid": self.uid,
            "mode": self.mode,
        }


@dataclass(frozen=True)
class ImmutableGeneration:
    path: Path
    sha256: str
    size: int
    device: int
    inode: int
    root_device: int
    root_inode: int
    reused: bool
    lineage: Mapping[str, object]
    source: Optional[RegularSourceReceipt] = None

    def receipt(self) -> dict[str, object]:
        payload: dict[str, object] = {
            "schema_version": 1,
            "publication": "immutable_no_overwrite_generation",
            "path": str(self.path),
            "sha256": self.sha256,
            "size": self.size,
            "device": self.device,
            "inode": self.inode,
            "root_device": self.root_device,
            "root_inode": self.root_inode,
            "reused": self.reused,
            "lineage": dict(self.lineage),
        }
        if self.source is not None:
            payload["source"] = self.source.as_dict()
        return payload


def _no_follow_flag() -> int:
    value = int(getattr(os, "O_NOFOLLOW", 0))
    if value == 0:
        raise ImmutableGenerationError("immutable_generation_no_follow_unavailable")
    return value


def _directory_flags() -> int:
    directory = int(getattr(os, "O_DIRECTORY", 0))
    if directory == 0:
        raise ImmutableGenerationError("immutable_generation_dirfd_unavailable")
    return os.O_RDONLY | directory | _no_follow_flag() | int(getattr(os, "O_CLOEXEC", 0))


def _file_read_flags() -> int:
    return os.O_RDONLY | _no_follow_flag() | int(getattr(os, "O_CLOEXEC", 0))


def _absolute_lexical(path: Path) -> Path:
    candidate = Path(path).expanduser()
    if not candidate.is_absolute():
        candidate = Path.cwd() / candidate
    if any(part in {"", ".", ".."} for part in candidate.parts[1:]):
        raise ImmutableGenerationError("immutable_generation_path_invalid")
    return candidate


def _stat_ns(value: os.stat_result, field: str, fallback: str) -> int:
    return int(getattr(value, field, int(getattr(value, fallback) * 1_000_000_000)))


def _validate_regular_stat(
    value: os.stat_result,
    *,
    private: bool,
    allow_multiple_links: bool = False,
) -> None:
    if not stat.S_ISREG(value.st_mode):
        raise ImmutableGenerationError("immutable_generation_regular_file_required")
    if int(value.st_nlink) < 1 or (int(value.st_nlink) != 1 and not allow_multiple_links):
        raise ImmutableGenerationError("immutable_generation_hardlink_rejected")
    if hasattr(os, "geteuid") and int(value.st_uid) != int(os.geteuid()):
        raise ImmutableGenerationError("immutable_generation_owner_mismatch")
    permissions = stat.S_IMODE(value.st_mode)
    if permissions & 0o022:
        raise ImmutableGenerationError("immutable_generation_writable_by_others")
    if private and permissions & 0o077:
        raise ImmutableGenerationError("immutable_generation_private_mode_required")


def _validate_private_directory(value: os.stat_result) -> None:
    if not stat.S_ISDIR(value.st_mode):
        raise ImmutableGenerationError("immutable_generation_directory_required")
    if hasattr(os, "geteuid") and int(value.st_uid) != int(os.geteuid()):
        raise ImmutableGenerationError("immutable_generation_root_owner_mismatch")
    if stat.S_IMODE(value.st_mode) & 0o077:
        raise ImmutableGenerationError("immutable_generation_root_private_mode_required")


def _receipt_from_stat(value: os.stat_result, sha256: str) -> RegularSourceReceipt:
    return RegularSourceReceipt(
        device=int(value.st_dev),
        inode=int(value.st_ino),
        uid=int(value.st_uid),
        mode=stat.S_IMODE(value.st_mode),
        links=int(value.st_nlink),
        size=int(value.st_size),
        mtime_ns=_stat_ns(value, "st_mtime_ns", "st_mtime"),
        ctime_ns=_stat_ns(value, "st_ctime_ns", "st_ctime"),
        generation=int(getattr(value, "st_gen", 0) or 0),
        sha256=sha256,
    )


def _same_receipt(left: RegularSourceReceipt, right: RegularSourceReceipt) -> bool:
    return left == right


def _stat_matches_receipt(value: os.stat_result, receipt: RegularSourceReceipt) -> bool:
    return (
        int(value.st_dev) == receipt.device
        and int(value.st_ino) == receipt.inode
        and int(value.st_uid) == receipt.uid
        and stat.S_IMODE(value.st_mode) == receipt.mode
        and int(value.st_nlink) == receipt.links
        and int(value.st_size) == receipt.size
        and _stat_ns(value, "st_mtime_ns", "st_mtime") == receipt.mtime_ns
        and _stat_ns(value, "st_ctime_ns", "st_ctime") == receipt.ctime_ns
        and int(getattr(value, "st_gen", 0) or 0) == receipt.generation
    )


def _hash_fd(fd: int) -> tuple[str, int]:
    digest = hashlib.sha256()
    size = 0
    os.lseek(fd, 0, os.SEEK_SET)
    while True:
        chunk = os.read(fd, _BUFFER_SIZE)
        if not chunk:
            break
        digest.update(chunk)
        size += len(chunk)
    os.lseek(fd, 0, os.SEEK_SET)
    return digest.hexdigest().upper(), size


def _inspect_open_fd(fd: int, *, private: bool) -> RegularSourceReceipt:
    before = os.fstat(fd)
    _validate_regular_stat(before, private=private)
    digest, size = _hash_fd(fd)
    after = os.fstat(fd)
    _validate_regular_stat(after, private=private)
    before_receipt = _receipt_from_stat(before, digest)
    after_receipt = _receipt_from_stat(after, digest)
    if before_receipt != after_receipt or size != before_receipt.size:
        raise ImmutableGenerationError("immutable_generation_source_changed_during_read")
    return before_receipt


def _open_regular_path(path: Path) -> int:
    candidate = _absolute_lexical(path)
    parent_fd = -1
    try:
        parent_fd, name = _open_parent(candidate)
        return os.open(name, _file_read_flags(), dir_fd=parent_fd)
    except OSError as exc:
        raise ImmutableGenerationError("immutable_generation_source_open_failed") from exc
    finally:
        if parent_fd >= 0:
            os.close(parent_fd)


def inspect_regular_source(
    path: Path,
    *,
    expected_sha256: Optional[str] = None,
    expected_size: Optional[int] = None,
) -> RegularSourceReceipt:
    fd = _open_regular_path(path)
    try:
        receipt = _inspect_open_fd(fd, private=False)
    finally:
        os.close(fd)
    expected_digest = str(expected_sha256 or "").strip().upper()
    if expected_digest and (len(expected_digest) != 64 or receipt.sha256 != expected_digest):
        raise ImmutableGenerationError("immutable_generation_source_hash_mismatch")
    if expected_size is not None and receipt.size != int(expected_size):
        raise ImmutableGenerationError("immutable_generation_source_size_mismatch")
    return receipt


@contextlib.contextmanager
def open_verified_source(path: Path, receipt: RegularSourceReceipt) -> Iterator[BinaryIO]:
    fd = _open_regular_path(path)
    file_object: Optional[BinaryIO] = None
    try:
        actual = _inspect_open_fd(fd, private=False)
        if not _same_receipt(actual, receipt):
            raise ImmutableGenerationError("immutable_generation_source_identity_mismatch")
        file_object = os.fdopen(fd, "rb", closefd=True)
        fd = -1
        yield file_object
        after = os.fstat(file_object.fileno())
        _validate_regular_stat(after, private=False)
        if _receipt_from_stat(after, receipt.sha256) != receipt:
            raise ImmutableGenerationError("immutable_generation_source_changed_during_use")
    finally:
        if file_object is not None:
            file_object.close()
        elif fd >= 0:
            os.close(fd)


@contextlib.contextmanager
def open_verified_text(
    path: Path,
    receipt: RegularSourceReceipt,
    *,
    encoding: str,
    errors: str = "strict",
    newline: Optional[str] = None,
) -> Iterator[io.TextIOWrapper]:
    with open_verified_source(path, receipt) as source:
        duplicate = os.fdopen(os.dup(source.fileno()), "rb", closefd=True)
        with io.TextIOWrapper(duplicate, encoding=encoding, errors=errors, newline=newline) as text:
            yield text


def _open_parent(path: Path) -> tuple[int, str]:
    candidate = _absolute_lexical(path)
    if candidate == Path(candidate.anchor):
        raise ImmutableGenerationError("immutable_generation_root_path_invalid")
    parts = candidate.parts
    fd = os.open(candidate.anchor, _directory_flags())
    try:
        for component in parts[1:-1]:
            next_fd = os.open(component, _directory_flags(), dir_fd=fd)
            os.close(fd)
            fd = next_fd
        return fd, parts[-1]
    except BaseException:
        os.close(fd)
        raise


def _open_or_create_private_root(path: Path) -> tuple[int, os.stat_result, Path]:
    candidate = _absolute_lexical(path)
    parent_fd, name = _open_parent(candidate)
    fd = -1
    try:
        try:
            fd = os.open(name, _directory_flags(), dir_fd=parent_fd)
        except FileNotFoundError:
            try:
                os.mkdir(name, _PRIVATE_DIRECTORY_MODE, dir_fd=parent_fd)
            except FileExistsError:
                pass
            fd = os.open(name, _directory_flags(), dir_fd=parent_fd)
            os.fsync(parent_fd)
        value = os.fstat(fd)
        if not stat.S_ISDIR(value.st_mode):
            raise ImmutableGenerationError("immutable_generation_directory_required")
        if hasattr(os, "geteuid") and int(value.st_uid) != int(os.geteuid()):
            raise ImmutableGenerationError("immutable_generation_root_owner_mismatch")
        if stat.S_IMODE(value.st_mode) & 0o077:
            os.fchmod(fd, _PRIVATE_DIRECTORY_MODE)
            os.fsync(fd)
            value = os.fstat(fd)
        _validate_private_directory(value)
        return fd, value, candidate
    except BaseException:
        if fd >= 0:
            os.close(fd)
        raise
    finally:
        os.close(parent_fd)


def ensure_private_generation_root(path: Path) -> Path:
    fd, _, candidate = _open_or_create_private_root(path)
    os.close(fd)
    return candidate


def inspect_private_generation_root(
    path: Path,
    *,
    expected_device: Optional[int] = None,
    expected_inode: Optional[int] = None,
) -> PrivateGenerationRootReceipt:
    candidate = _absolute_lexical(path)
    parent_fd, name = _open_parent(candidate)
    root_fd = -1
    try:
        root_fd = os.open(name, _directory_flags(), dir_fd=parent_fd)
        value = os.fstat(root_fd)
        _validate_private_directory(value)
        if expected_device is not None and int(value.st_dev) != int(expected_device):
            raise ImmutableGenerationError("immutable_generation_authority_root_identity_mismatch")
        if expected_inode is not None and int(value.st_ino) != int(expected_inode):
            raise ImmutableGenerationError("immutable_generation_authority_root_identity_mismatch")
        _validate_root_path_identity(candidate, value)
        return PrivateGenerationRootReceipt(
            device=int(value.st_dev),
            inode=int(value.st_ino),
            uid=int(value.st_uid),
            mode=stat.S_IMODE(value.st_mode),
        )
    except OSError as exc:
        raise ImmutableGenerationError("immutable_generation_authority_root_open_failed") from exc
    finally:
        if root_fd >= 0:
            os.close(root_fd)
        os.close(parent_fd)


def _validate_root_path_identity(path: Path, expected: os.stat_result) -> None:
    parent_fd, name = _open_parent(path)
    root_fd = -1
    try:
        root_fd = os.open(name, _directory_flags(), dir_fd=parent_fd)
        current = os.fstat(root_fd)
        _validate_private_directory(current)
        if int(current.st_dev) != int(expected.st_dev) or int(current.st_ino) != int(expected.st_ino):
            raise ImmutableGenerationError("immutable_generation_root_identity_mismatch")
    finally:
        if root_fd >= 0:
            os.close(root_fd)
        os.close(parent_fd)


def inspect_published_generation(
    path: Path,
    *,
    expected_sha256: Optional[str] = None,
    expected_size: Optional[int] = None,
    expected_device: Optional[int] = None,
    expected_inode: Optional[int] = None,
    expected_root_device: Optional[int] = None,
    expected_root_inode: Optional[int] = None,
) -> ImmutableGeneration:
    candidate = _absolute_lexical(path)
    root_path = candidate.parent
    parent_fd, root_name = _open_parent(root_path)
    try:
        root_fd = os.open(root_name, _directory_flags(), dir_fd=parent_fd)
    except BaseException:
        os.close(parent_fd)
        raise
    os.close(parent_fd)
    try:
        root_stat = os.fstat(root_fd)
        _validate_private_directory(root_stat)
        if expected_root_device is not None and int(root_stat.st_dev) != int(expected_root_device):
            raise ImmutableGenerationError("immutable_generation_root_identity_mismatch")
        if expected_root_inode is not None and int(root_stat.st_ino) != int(expected_root_inode):
            raise ImmutableGenerationError("immutable_generation_root_identity_mismatch")
        file_fd, file_stat, digest, size = _open_generation(root_fd, candidate.name)
        os.close(file_fd)
        expected_digest = str(expected_sha256 or "").strip().upper()
        if expected_digest and digest != expected_digest:
            raise ImmutableGenerationError("immutable_generation_hash_mismatch")
        if expected_size is not None and size != int(expected_size):
            raise ImmutableGenerationError("immutable_generation_size_mismatch")
        if expected_device is not None and int(file_stat.st_dev) != int(expected_device):
            raise ImmutableGenerationError("immutable_generation_identity_mismatch")
        if expected_inode is not None and int(file_stat.st_ino) != int(expected_inode):
            raise ImmutableGenerationError("immutable_generation_identity_mismatch")
        return ImmutableGeneration(
            path=root_path / candidate.name,
            sha256=digest,
            size=size,
            device=int(file_stat.st_dev),
            inode=int(file_stat.st_ino),
            root_device=int(root_stat.st_dev),
            root_inode=int(root_stat.st_ino),
            reused=True,
            lineage={},
            source=None,
        )
    finally:
        os.close(root_fd)


def inspect_authorized_generation(
    authority_root: Path,
    relative_path: str,
    *,
    expected_authority_device: int,
    expected_authority_inode: int,
    expected_sha256: str,
    expected_size: int,
    expected_device: int,
    expected_inode: int,
    expected_root_device: int,
    expected_root_inode: int,
) -> ImmutableGeneration:
    root_path = _absolute_lexical(authority_root)
    relative = Path(str(relative_path or ""))
    if (
        not relative.parts
        or relative.is_absolute()
        or any(part in {"", ".", ".."} for part in relative.parts)
    ):
        raise ImmutableGenerationError("immutable_generation_authority_relative_path_invalid")

    parent_fd, root_name = _open_parent(root_path)
    authority_fd = -1
    generation_root_fd = -1
    try:
        authority_fd = os.open(root_name, _directory_flags(), dir_fd=parent_fd)
        authority_stat = os.fstat(authority_fd)
        _validate_private_directory(authority_stat)
        if (
            int(authority_stat.st_dev) != int(expected_authority_device)
            or int(authority_stat.st_ino) != int(expected_authority_inode)
        ):
            raise ImmutableGenerationError("immutable_generation_authority_root_identity_mismatch")

        generation_root_fd = os.dup(authority_fd)
        for component in relative.parts[:-1]:
            next_fd = os.open(component, _directory_flags(), dir_fd=generation_root_fd)
            os.close(generation_root_fd)
            generation_root_fd = next_fd

        generation_root_stat = os.fstat(generation_root_fd)
        _validate_private_directory(generation_root_stat)
        if (
            int(generation_root_stat.st_dev) != int(expected_root_device)
            or int(generation_root_stat.st_ino) != int(expected_root_inode)
        ):
            raise ImmutableGenerationError("immutable_generation_root_identity_mismatch")

        file_fd, file_stat, digest, size = _open_generation(generation_root_fd, relative.parts[-1])
        os.close(file_fd)
        normalized_digest = str(expected_sha256 or "").strip().upper()
        if len(normalized_digest) != 64 or digest != normalized_digest:
            raise ImmutableGenerationError("immutable_generation_hash_mismatch")
        if size != int(expected_size):
            raise ImmutableGenerationError("immutable_generation_size_mismatch")
        if int(file_stat.st_dev) != int(expected_device) or int(file_stat.st_ino) != int(expected_inode):
            raise ImmutableGenerationError("immutable_generation_identity_mismatch")

        _validate_root_path_identity(root_path, authority_stat)
        return ImmutableGeneration(
            path=root_path.joinpath(*relative.parts),
            sha256=digest,
            size=size,
            device=int(file_stat.st_dev),
            inode=int(file_stat.st_ino),
            root_device=int(generation_root_stat.st_dev),
            root_inode=int(generation_root_stat.st_ino),
            reused=True,
            lineage={},
            source=None,
        )
    except OSError as exc:
        raise ImmutableGenerationError("immutable_generation_authorized_member_open_failed") from exc
    finally:
        if generation_root_fd >= 0:
            os.close(generation_root_fd)
        if authority_fd >= 0:
            os.close(authority_fd)
        os.close(parent_fd)


def _safe_suffix(value: str) -> str:
    suffix = str(value or "").strip().lower()
    if not suffix:
        return ""
    if not suffix.startswith("."):
        suffix = "." + suffix
    if len(suffix) > 24 or any(not (char.isalnum() or char in {".", "_", "-"}) for char in suffix):
        raise ImmutableGenerationError("immutable_generation_suffix_invalid")
    return suffix


def _safe_target_name(value: str) -> str:
    name = str(value or "")
    if not name or name in {".", ".."} or Path(name).name != name or len(os.fsencode(name)) > 255:
        raise ImmutableGenerationError("immutable_generation_target_name_invalid")
    return name


def _lock_generation_root(root_fd: int) -> None:
    try:
        import fcntl
    except ImportError as exc:
        raise ImmutableGenerationError("immutable_generation_root_lock_unavailable") from exc
    try:
        fcntl.flock(root_fd, fcntl.LOCK_EX)
    except OSError as exc:
        raise ImmutableGenerationError("immutable_generation_root_lock_failed") from exc


def _cleanup_inert_pending_generations(root_fd: int) -> None:
    changed = False
    for entry in os.listdir(root_fd):
        if not entry.startswith(".pending-v1-"):
            continue
        entry_name = _safe_target_name(entry)
        try:
            entry_fd = os.open(entry_name, _file_read_flags(), dir_fd=root_fd)
        except FileNotFoundError:
            continue
        except OSError as exc:
            raise ImmutableGenerationError("immutable_generation_pending_invalid") from exc
        try:
            entry_stat = os.fstat(entry_fd)
            _validate_regular_stat(entry_stat, private=True, allow_multiple_links=True)
        finally:
            os.close(entry_fd)
        _unlink_exact(root_fd, entry_name, entry_stat)
        changed = True
    if changed:
        os.fsync(root_fd)


def _cleanup_exact_work_directory(root_fd: int, name: str) -> None:
    work_fd = os.open(name, _directory_flags(), dir_fd=root_fd)
    try:
        value = os.fstat(work_fd)
        _validate_private_directory(value)
        for entry in os.listdir(work_fd):
            entry_name = _safe_target_name(entry)
            entry_fd = os.open(entry_name, _file_read_flags(), dir_fd=work_fd)
            try:
                entry_stat = os.fstat(entry_fd)
                if not stat.S_ISREG(entry_stat.st_mode) or int(entry_stat.st_nlink) != 1:
                    raise ImmutableGenerationError("immutable_generation_work_output_invalid")
                if hasattr(os, "geteuid") and int(entry_stat.st_uid) != int(os.geteuid()):
                    raise ImmutableGenerationError("immutable_generation_work_output_owner_mismatch")
                if stat.S_IMODE(entry_stat.st_mode) != _PRIVATE_FILE_MODE:
                    os.fchmod(entry_fd, _PRIVATE_FILE_MODE)
                    os.fsync(entry_fd)
                    entry_stat = os.fstat(entry_fd)
                _validate_regular_stat(entry_stat, private=True)
            finally:
                os.close(entry_fd)
            os.unlink(entry_name, dir_fd=work_fd)
        os.fsync(work_fd)
    finally:
        os.close(work_fd)
    os.rmdir(name, dir_fd=root_fd)
    os.fsync(root_fd)


def _cleanup_inert_work_directories(root_fd: int) -> None:
    for entry in os.listdir(root_fd):
        if not entry.startswith(".work-v1-"):
            continue
        _cleanup_exact_work_directory(root_fd, _safe_target_name(entry))


def _open_generation(root_fd: int, name: str) -> tuple[int, os.stat_result, str, int]:
    fd = os.open(name, _file_read_flags(), dir_fd=root_fd)
    try:
        receipt = _inspect_open_fd(fd, private=True)
        value = os.fstat(fd)
        return fd, value, receipt.sha256, receipt.size
    except BaseException:
        os.close(fd)
        raise


def _recover_or_open_existing_generation(
    root_fd: int,
    target_name: str,
    *,
    expected_digest: str,
    expected_size: int,
) -> os.stat_result:
    target_fd = os.open(target_name, _file_read_flags(), dir_fd=root_fd)
    try:
        before = os.fstat(target_fd)
        _validate_regular_stat(before, private=True, allow_multiple_links=True)
        digest, size = _hash_fd(target_fd)
        after = os.fstat(target_fd)
        _validate_regular_stat(after, private=True, allow_multiple_links=True)
        if (
            int(before.st_dev) != int(after.st_dev)
            or int(before.st_ino) != int(after.st_ino)
            or int(before.st_nlink) != int(after.st_nlink)
            or digest != expected_digest
            or size != expected_size
        ):
            raise ImmutableGenerationError("immutable_generation_existing_target_mismatch")
        target_stat = after
    finally:
        os.close(target_fd)

    if int(target_stat.st_nlink) == 1:
        return target_stat

    aliases: list[tuple[str, os.stat_result]] = []
    for entry in os.listdir(root_fd):
        if entry == target_name:
            continue
        entry_name = _safe_target_name(entry)
        try:
            entry_fd = os.open(entry_name, _file_read_flags(), dir_fd=root_fd)
        except (FileNotFoundError, OSError):
            continue
        try:
            entry_stat = os.fstat(entry_fd)
        finally:
            os.close(entry_fd)
        if int(entry_stat.st_dev) != int(target_stat.st_dev) or int(entry_stat.st_ino) != int(target_stat.st_ino):
            continue
        if not entry_name.startswith(".pending-v1-"):
            raise ImmutableGenerationError("immutable_generation_unrecognized_hardlink")
        aliases.append((entry_name, entry_stat))

    if len(aliases) + 1 != int(target_stat.st_nlink):
        raise ImmutableGenerationError("immutable_generation_external_hardlink_rejected")
    for entry_name, entry_stat in aliases:
        _unlink_exact(root_fd, entry_name, entry_stat)
    os.fsync(root_fd)
    reopened_fd, reopened_stat, reopened_digest, reopened_size = _open_generation(root_fd, target_name)
    os.close(reopened_fd)
    if reopened_digest != expected_digest or reopened_size != expected_size:
        raise ImmutableGenerationError("immutable_generation_recovery_readback_mismatch")
    return reopened_stat


def _unlink_exact(root_fd: int, name: str, expected: os.stat_result) -> None:
    try:
        current_fd = os.open(name, _file_read_flags(), dir_fd=root_fd)
    except FileNotFoundError:
        return
    try:
        current = os.fstat(current_fd)
        if int(current.st_dev) != int(expected.st_dev) or int(current.st_ino) != int(expected.st_ino):
            raise ImmutableGenerationError("immutable_generation_cleanup_identity_mismatch")
    finally:
        os.close(current_fd)
    os.unlink(name, dir_fd=root_fd)


def _publish_staged(
    *,
    root_fd: int,
    root_stat: os.stat_result,
    root_path: Path,
    staged_name: str,
    staged_stat: os.stat_result,
    digest: str,
    size: int,
    target_name: str,
    lineage: Mapping[str, object],
    source: Optional[RegularSourceReceipt],
) -> ImmutableGeneration:
    target_name = _safe_target_name(target_name)
    reused = False
    target_stat: Optional[os.stat_result] = None
    linked = False
    staged_removed = False
    try:
        try:
            os.link(
                staged_name,
                target_name,
                src_dir_fd=root_fd,
                dst_dir_fd=root_fd,
                follow_symlinks=False,
            )
            linked = True
            os.fsync(root_fd)
            _unlink_exact(root_fd, staged_name, staged_stat)
            staged_removed = True
            os.fsync(root_fd)
        except FileExistsError:
            target_stat = _recover_or_open_existing_generation(
                root_fd,
                target_name,
                expected_digest=digest,
                expected_size=size,
            )
            reused = True

        if linked:
            target_fd, target_stat, target_digest, target_size = _open_generation(root_fd, target_name)
            os.close(target_fd)
            if target_digest != digest or target_size != size:
                raise ImmutableGenerationError("immutable_generation_publish_readback_mismatch")

        if not staged_removed:
            _unlink_exact(root_fd, staged_name, staged_stat)
            staged_removed = True
            os.fsync(root_fd)
        if target_stat is None:
            raise ImmutableGenerationError("immutable_generation_target_missing")
        _validate_root_path_identity(root_path, root_stat)
        return ImmutableGeneration(
            path=root_path / target_name,
            sha256=digest,
            size=size,
            device=int(target_stat.st_dev),
            inode=int(target_stat.st_ino),
            root_device=int(root_stat.st_dev),
            root_inode=int(root_stat.st_ino),
            reused=reused,
            lineage=dict(lineage),
            source=source,
        )
    except BaseException as original_error:
        cleanup_error: Optional[BaseException] = None
        try:
            target_fd = os.open(target_name, _file_read_flags(), dir_fd=root_fd)
            try:
                current = os.fstat(target_fd)
            finally:
                os.close(target_fd)
            if int(current.st_dev) == int(staged_stat.st_dev) and int(current.st_ino) == int(staged_stat.st_ino):
                os.unlink(target_name, dir_fd=root_fd)
        except FileNotFoundError:
            pass
        except BaseException as exc:
            cleanup_error = exc
        if not staged_removed:
            try:
                _unlink_exact(root_fd, staged_name, staged_stat)
            except BaseException as exc:
                cleanup_error = cleanup_error or exc
        try:
            os.fsync(root_fd)
        except BaseException as exc:
            cleanup_error = cleanup_error or exc
        if cleanup_error is not None:
            raise ImmutableGenerationError("immutable_generation_failure_cleanup_indeterminate") from original_error
        raise


def publish_generated(
    root: Path,
    *,
    suffix: str,
    producer: Callable[[BinaryIO], None],
    lineage: Optional[Mapping[str, object]] = None,
    target_name: Optional[str] = None,
    expected_sha256: Optional[str] = None,
    expected_size: Optional[int] = None,
) -> ImmutableGeneration:
    root_fd, root_stat, root_path = _open_or_create_private_root(root)
    staged_name = f".pending-v1-{secrets.token_hex(16)}"
    staged_fd = -1
    staged_stat: Optional[os.stat_result] = None
    try:
        _lock_generation_root(root_fd)
        _cleanup_inert_pending_generations(root_fd)
        staged_fd = os.open(
            staged_name,
            os.O_RDWR
            | os.O_CREAT
            | os.O_EXCL
            | _no_follow_flag()
            | int(getattr(os, "O_CLOEXEC", 0)),
            _PRIVATE_FILE_MODE,
            dir_fd=root_fd,
        )
        staged_stat = os.fstat(staged_fd)
        _validate_regular_stat(staged_stat, private=True)
        with os.fdopen(os.dup(staged_fd), "wb", closefd=True) as output:
            producer(output)
            output.flush()
        os.fsync(staged_fd)
        receipt = _inspect_open_fd(staged_fd, private=True)
        expected_digest = str(expected_sha256 or "").strip().upper()
        if expected_digest and receipt.sha256 != expected_digest:
            raise ImmutableGenerationError("immutable_generation_generated_hash_mismatch")
        if expected_size is not None and receipt.size != int(expected_size):
            raise ImmutableGenerationError("immutable_generation_generated_size_mismatch")
        final_name = target_name or f"{receipt.sha256.lower()}{_safe_suffix(suffix)}"
        return _publish_staged(
            root_fd=root_fd,
            root_stat=root_stat,
            root_path=root_path,
            staged_name=staged_name,
            staged_stat=staged_stat,
            digest=receipt.sha256,
            size=receipt.size,
            target_name=final_name,
            lineage=dict(lineage or {}),
            source=None,
        )
    except BaseException:
        if staged_fd >= 0:
            try:
                os.close(staged_fd)
            except OSError:
                pass
            staged_fd = -1
        if staged_stat is not None:
            try:
                _unlink_exact(root_fd, staged_name, staged_stat)
                os.fsync(root_fd)
            except (OSError, ImmutableGenerationError):
                pass
        raise
    finally:
        if staged_fd >= 0:
            os.close(staged_fd)
        os.close(root_fd)


def publish_bytes(
    root: Path,
    data: bytes,
    *,
    suffix: str = "",
    lineage: Optional[Mapping[str, object]] = None,
    target_name: Optional[str] = None,
) -> ImmutableGeneration:
    payload = bytes(data)

    def produce(output: BinaryIO) -> None:
        if output.write(payload) != len(payload):
            raise ImmutableGenerationError("immutable_generation_partial_write")

    return publish_generated(
        root,
        suffix=suffix,
        producer=produce,
        lineage=lineage,
        target_name=target_name,
    )


def publish_json(
    root: Path,
    value: Mapping[str, object],
    *,
    target_name: Optional[str] = None,
    lineage: Optional[Mapping[str, object]] = None,
) -> ImmutableGeneration:
    payload = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")
    return publish_bytes(
        root,
        payload,
        suffix=".json",
        lineage=lineage,
        target_name=target_name,
    )


def publish_source(
    source_path: Path,
    root: Path,
    *,
    expected: Optional[RegularSourceReceipt] = None,
    expected_sha256: Optional[str] = None,
    expected_size: Optional[int] = None,
    suffix: Optional[str] = None,
    lineage: Optional[Mapping[str, object]] = None,
) -> ImmutableGeneration:
    receipt = expected or inspect_regular_source(
        source_path,
        expected_sha256=expected_sha256,
        expected_size=expected_size,
    )
    if expected_sha256 and receipt.sha256 != str(expected_sha256).strip().upper():
        raise ImmutableGenerationError("immutable_generation_source_hash_mismatch")
    if expected_size is not None and receipt.size != int(expected_size):
        raise ImmutableGenerationError("immutable_generation_source_size_mismatch")

    generation = publish_generated(
        root,
        suffix=suffix if suffix is not None else Path(source_path).suffix,
        producer=lambda output: _copy_source_to_output(source_path, receipt, output),
        lineage=lineage,
        expected_sha256=receipt.sha256,
        expected_size=receipt.size,
    )
    if generation.sha256 != receipt.sha256 or generation.size != receipt.size:
        raise ImmutableGenerationError("immutable_generation_source_copy_mismatch")
    return ImmutableGeneration(
        path=generation.path,
        sha256=generation.sha256,
        size=generation.size,
        device=generation.device,
        inode=generation.inode,
        root_device=generation.root_device,
        root_inode=generation.root_inode,
        reused=generation.reused,
        lineage=generation.lineage,
        source=receipt,
    )


def _copy_source_to_output(path: Path, receipt: RegularSourceReceipt, output: BinaryIO) -> None:
    source_fd = _open_regular_path(path)
    try:
        before = os.fstat(source_fd)
        _validate_regular_stat(before, private=False)
        if not _stat_matches_receipt(before, receipt):
            raise ImmutableGenerationError("immutable_generation_source_identity_mismatch")
        while True:
            chunk = os.read(source_fd, _BUFFER_SIZE)
            if not chunk:
                break
            if output.write(chunk) != len(chunk):
                raise ImmutableGenerationError("immutable_generation_partial_write")
        after = os.fstat(source_fd)
        _validate_regular_stat(after, private=False)
        if not _stat_matches_receipt(after, receipt):
            raise ImmutableGenerationError("immutable_generation_source_changed_during_copy")
    finally:
        os.close(source_fd)


def read_immutable_bytes(
    path: Path,
    *,
    expected_sha256: Optional[str] = None,
    max_bytes: int = 16 * 1024 * 1024,
) -> bytes:
    receipt = inspect_regular_source(path, expected_sha256=expected_sha256)
    limit = max(int(max_bytes), 0)
    if receipt.size > limit:
        raise ImmutableGenerationError("immutable_generation_read_limit_exceeded")
    with open_verified_source(path, receipt) as source:
        payload = source.read()
    if len(payload) != receipt.size or hashlib.sha256(payload).hexdigest().upper() != receipt.sha256:
        raise ImmutableGenerationError("immutable_generation_readback_mismatch")
    return payload


def read_published_generation_bytes(
    path: Path,
    *,
    expected_sha256: str,
    expected_size: int,
    expected_device: int,
    expected_inode: int,
    expected_root_device: int,
    expected_root_inode: int,
    max_bytes: int = 16 * 1024 * 1024,
) -> bytes:
    candidate = _absolute_lexical(path)
    parent_fd, root_name = _open_parent(candidate.parent)
    root_fd = -1
    file_fd = -1
    try:
        root_fd = os.open(root_name, _directory_flags(), dir_fd=parent_fd)
        root_stat = os.fstat(root_fd)
        _validate_private_directory(root_stat)
        if (
            int(root_stat.st_dev) != int(expected_root_device)
            or int(root_stat.st_ino) != int(expected_root_inode)
        ):
            raise ImmutableGenerationError("immutable_generation_root_identity_mismatch")
        file_fd, file_stat, digest, size = _open_generation(root_fd, candidate.name)
        normalized_digest = str(expected_sha256 or "").strip().upper()
        if len(normalized_digest) != 64 or digest != normalized_digest:
            raise ImmutableGenerationError("immutable_generation_hash_mismatch")
        if size != int(expected_size):
            raise ImmutableGenerationError("immutable_generation_size_mismatch")
        if int(file_stat.st_dev) != int(expected_device) or int(file_stat.st_ino) != int(expected_inode):
            raise ImmutableGenerationError("immutable_generation_identity_mismatch")
        if size > max(int(max_bytes), 0):
            raise ImmutableGenerationError("immutable_generation_read_limit_exceeded")

        os.lseek(file_fd, 0, os.SEEK_SET)
        payload = bytearray()
        while True:
            chunk = os.read(file_fd, min(_BUFFER_SIZE, max(size - len(payload), 1)))
            if not chunk:
                break
            payload.extend(chunk)
            if len(payload) > size:
                raise ImmutableGenerationError("immutable_generation_readback_mismatch")
        after = os.fstat(file_fd)
        if (
            int(after.st_dev) != int(file_stat.st_dev)
            or int(after.st_ino) != int(file_stat.st_ino)
            or int(after.st_size) != size
            or len(payload) != size
            or hashlib.sha256(payload).hexdigest().upper() != digest
        ):
            raise ImmutableGenerationError("immutable_generation_readback_mismatch")
        _validate_root_path_identity(candidate.parent, root_stat)
        return bytes(payload)
    except OSError as exc:
        raise ImmutableGenerationError("immutable_generation_read_failed") from exc
    finally:
        if file_fd >= 0:
            os.close(file_fd)
        if root_fd >= 0:
            os.close(root_fd)
        os.close(parent_fd)


@contextlib.contextmanager
def private_work_directory(root: Path) -> Iterator[Path]:
    root_fd, _, root_path = _open_or_create_private_root(root)
    name = f".work-v1-{secrets.token_hex(16)}"
    work_fd = -1
    try:
        _lock_generation_root(root_fd)
        _cleanup_inert_work_directories(root_fd)
        os.mkdir(name, _PRIVATE_DIRECTORY_MODE, dir_fd=root_fd)
        os.fsync(root_fd)
        work_fd = os.open(name, _directory_flags(), dir_fd=root_fd)
        _validate_private_directory(os.fstat(work_fd))
        try:
            yield root_path / name
        finally:
            os.close(work_fd)
            work_fd = -1
            _cleanup_exact_work_directory(root_fd, name)
    finally:
        if work_fd >= 0:
            os.close(work_fd)
        os.close(root_fd)
