from __future__ import annotations

import errno
import hashlib
import math
import os
import re
import secrets
import stat
import tempfile
import time
from contextlib import contextmanager
from collections.abc import Iterator
from pathlib import Path


_INVALID_FS_CHARS = re.compile(r'[<>:"/\\|?*\x00-\x1f]')
_RESERVED_NAMES = {
    "CON",
    "PRN",
    "AUX",
    "NUL",
    *(f"COM{i}" for i in range(1, 10)),
    *(f"LPT{i}" for i in range(1, 10)),
}


def safe_fs_name(name: str, fallback: str) -> str:
    cleaned = (name or "").replace("\uFFFD", "_")
    cleaned = _INVALID_FS_CHARS.sub("_", cleaned)
    cleaned = cleaned.strip().strip(".")
    if not cleaned:
        cleaned = fallback
    if cleaned.upper() in _RESERVED_NAMES:
        cleaned = f"{cleaned}_"
    return cleaned


def _validated_case_storage_id(case_id: object) -> str:
    if not isinstance(case_id, str):
        raise ValueError("case_storage_key_invalid")
    normalized = case_id.strip()
    if not normalized or normalized != case_id or normalized.lower() == "active":
        raise ValueError("case_storage_key_invalid")
    return normalized


def case_bound_storage_name(case_id: object) -> str:
    """Return a collision-resistant component for host-validated case storage.

    Mutable aliases such as ``active`` and implicit empty fallbacks are never
    valid factual storage authority. The digest binds the component to the
    complete original identifier even when filesystem sanitization collides.
    """

    normalized = _validated_case_storage_id(case_id)
    digest = hashlib.sha256(normalized.encode("utf-8", errors="strict")).hexdigest()
    return f"case-{digest}"


def legacy_case_bound_storage_name_v1(case_id: object) -> str:
    """Return the retired prefix-plus-truncated-digest component for purge only."""

    normalized = _validated_case_storage_id(case_id)
    safe_prefix = safe_fs_name(normalized, "case")[:80]
    digest = hashlib.sha256(normalized.encode("utf-8", errors="strict")).hexdigest()[:16]
    return f"{safe_prefix}-{digest}"


def _supports_private_dirfd_io() -> bool:
    return (
        os.name != "nt"
        and os.open in os.supports_dir_fd
        and os.mkdir in os.supports_dir_fd
        and os.stat in os.supports_dir_fd
        and os.link in os.supports_dir_fd
        and os.unlink in os.supports_dir_fd
        and bool(getattr(os, "O_DIRECTORY", 0))
        and bool(getattr(os, "O_NOFOLLOW", 0))
    )


def _validate_private_directory_descriptor(descriptor: int) -> os.stat_result:
    observed = os.fstat(descriptor)
    if (
        not stat.S_ISDIR(observed.st_mode)
        or int(observed.st_mode) & 0o077 != 0
        or (hasattr(os, "geteuid") and int(observed.st_uid) != int(os.geteuid()))
    ):
        raise OSError("private_artifact_directory_invalid")
    return observed


def _write_all(descriptor: int, data: bytes) -> None:
    view = memoryview(data)
    while view:
        written = os.write(descriptor, view)
        if written <= 0:
            raise OSError("private_artifact_write_failed")
        view = view[written:]


def atomic_write_private_text(path: Path, content: str) -> None:
    """Atomically replace a private text artifact with owner-only permissions."""

    if os.name == "nt":
        # Python 3.11 cannot prove owner-only DACLs or reparse-point-stable
        # replacement through this path. Private publication must fail closed
        # until the Windows handle/ACL adapter is available.
        raise OSError("private_artifact_windows_authority_unavailable")
    target = Path(path)
    parent = target.parent
    if _supports_private_dirfd_io():
        parent_descriptor = _open_or_create_absolute_directory_private(parent)
        temporary_name = f".{target.name}.analytix-{secrets.token_hex(16)}.tmp"
        descriptor = -1
        try:
            _validate_private_directory_descriptor(parent_descriptor)
            descriptor = os.open(
                temporary_name,
                os.O_WRONLY
                | os.O_CREAT
                | os.O_EXCL
                | int(getattr(os, "O_CLOEXEC", 0))
                | int(getattr(os, "O_NOFOLLOW", 0)),
                0o600,
                dir_fd=parent_descriptor,
            )
            os.fchmod(descriptor, 0o600)
            _write_all(descriptor, str(content or "").encode("utf-8", errors="strict"))
            os.fsync(descriptor)
            os.close(descriptor)
            descriptor = -1
            os.replace(
                temporary_name,
                target.name,
                src_dir_fd=parent_descriptor,
                dst_dir_fd=parent_descriptor,
            )
            os.fsync(parent_descriptor)
            return
        finally:
            if descriptor >= 0:
                os.close(descriptor)
            try:
                os.unlink(temporary_name, dir_fd=parent_descriptor)
            except FileNotFoundError:
                pass
            finally:
                os.close(parent_descriptor)
    parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(
        prefix=f".{target.name}.analytix-",
        suffix=".tmp",
        dir=parent,
    )
    temporary = Path(temporary_name)
    try:
        os.fchmod(descriptor, 0o600)
        data = str(content or "").encode("utf-8")
        view = memoryview(data)
        while view:
            written = os.write(descriptor, view)
            if written <= 0:
                raise OSError("private_artifact_write_failed")
            view = view[written:]
        os.fsync(descriptor)
        os.close(descriptor)
        descriptor = -1
        os.replace(temporary, target)
        if os.name != "nt":
            directory_descriptor = os.open(parent, os.O_RDONLY | int(getattr(os, "O_CLOEXEC", 0)))
            try:
                os.fsync(directory_descriptor)
            finally:
                os.close(directory_descriptor)
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        try:
            temporary.unlink(missing_ok=True)
        except OSError:
            pass


def atomic_create_private_text(path: Path, content: str) -> bool:
    """Atomically publish a new owner-only file without replacing an existing one."""

    if os.name == "nt":
        raise OSError("private_artifact_windows_authority_unavailable")
    target = Path(path)
    parent = target.parent
    if _supports_private_dirfd_io():
        parent_descriptor = _open_or_create_absolute_directory_private(parent)
        temporary_name = f".{target.name}.analytix-{secrets.token_hex(16)}.tmp"
        descriptor = -1
        try:
            _validate_private_directory_descriptor(parent_descriptor)
            descriptor = os.open(
                temporary_name,
                os.O_WRONLY
                | os.O_CREAT
                | os.O_EXCL
                | int(getattr(os, "O_CLOEXEC", 0))
                | int(getattr(os, "O_NOFOLLOW", 0)),
                0o600,
                dir_fd=parent_descriptor,
            )
            os.fchmod(descriptor, 0o600)
            _write_all(descriptor, str(content or "").encode("utf-8", errors="strict"))
            os.fsync(descriptor)
            os.close(descriptor)
            descriptor = -1
            try:
                os.link(
                    temporary_name,
                    target.name,
                    src_dir_fd=parent_descriptor,
                    dst_dir_fd=parent_descriptor,
                    follow_symlinks=False,
                )
            except FileExistsError:
                return False
            os.fsync(parent_descriptor)
            return True
        finally:
            if descriptor >= 0:
                os.close(descriptor)
            try:
                os.unlink(temporary_name, dir_fd=parent_descriptor)
            except FileNotFoundError:
                pass
            finally:
                os.close(parent_descriptor)
    parent.mkdir(parents=True, exist_ok=True)
    if parent.resolve(strict=True) != parent.absolute() or parent.is_symlink():
        raise OSError("private_artifact_write_invalid")
    if os.name != "nt":
        os.chmod(parent, 0o700)
    descriptor, temporary_name = tempfile.mkstemp(
        prefix=f".{target.name}.analytix-",
        suffix=".tmp",
        dir=parent,
    )
    temporary = Path(temporary_name)
    try:
        os.fchmod(descriptor, 0o600)
        data = str(content or "").encode("utf-8")
        view = memoryview(data)
        while view:
            written = os.write(descriptor, view)
            if written <= 0:
                raise OSError("private_artifact_write_failed")
            view = view[written:]
        os.fsync(descriptor)
        os.close(descriptor)
        descriptor = -1
        try:
            os.link(temporary, target)
        except FileExistsError:
            return False
        if os.name != "nt":
            directory_descriptor = os.open(parent, os.O_RDONLY | int(getattr(os, "O_CLOEXEC", 0)))
            try:
                os.fsync(directory_descriptor)
            finally:
                os.close(directory_descriptor)
        return True
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        try:
            temporary.unlink(missing_ok=True)
        except OSError:
            pass


@contextmanager
def private_exclusive_file_lock(path: Path, *, timeout_seconds: float = 5.0) -> Iterator[None]:
    """Hold an owner-private, cross-process exclusive lease for one file.

    The lease file is opened descriptor-relative beneath a no-follow directory
    chain. A regular, single-link, owner-only inode is required both before and
    after acquisition so callers never mistake an attacker-controlled path for
    publication authority.
    """

    if os.name == "nt" or not _supports_private_dirfd_io():
        raise OSError("private_artifact_lock_authority_unavailable")
    if not isinstance(timeout_seconds, (int, float)) or isinstance(timeout_seconds, bool):
        raise OSError("private_artifact_lock_invalid")
    timeout = float(timeout_seconds)
    if not math.isfinite(timeout) or timeout <= 0 or timeout > 60:
        raise OSError("private_artifact_lock_invalid")
    target = Path(path)
    if target.name in {"", ".", ".."} or Path(target.name).name != target.name:
        raise OSError("private_artifact_lock_invalid")

    parent_descriptor = -1
    descriptor = -1
    locked = False
    try:
        try:
            import fcntl

            parent_descriptor = _open_or_create_absolute_directory_private(target.parent)
            _validate_private_directory_descriptor(parent_descriptor)
            descriptor = os.open(
                target.name,
                os.O_RDWR
                | os.O_CREAT
                | int(getattr(os, "O_CLOEXEC", 0))
                | int(getattr(os, "O_NOFOLLOW", 0)),
                0o600,
                dir_fd=parent_descriptor,
            )
            os.fchmod(descriptor, 0o600)
            observed = os.fstat(descriptor)
            if (
                not stat.S_ISREG(observed.st_mode)
                or int(observed.st_nlink) != 1
                or int(observed.st_mode) & 0o077 != 0
                or (hasattr(os, "geteuid") and int(observed.st_uid) != int(os.geteuid()))
            ):
                raise OSError("private_artifact_lock_invalid")
            deadline = time.monotonic() + timeout
            while True:
                try:
                    fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    locked = True
                    break
                except OSError as exc:
                    if exc.errno not in {errno.EACCES, errno.EAGAIN}:
                        raise
                    remaining = deadline - time.monotonic()
                    if remaining <= 0:
                        raise OSError("private_artifact_lock_timeout") from None
                    time.sleep(min(0.01, remaining))
            acquired = os.fstat(descriptor)
            path_observed = os.stat(target.name, dir_fd=parent_descriptor, follow_symlinks=False)
            if (
                not stat.S_ISREG(acquired.st_mode)
                or int(acquired.st_nlink) != 1
                or (int(observed.st_dev), int(observed.st_ino))
                != (int(acquired.st_dev), int(acquired.st_ino))
                or (int(path_observed.st_dev), int(path_observed.st_ino))
                != (int(acquired.st_dev), int(acquired.st_ino))
                or not stat.S_ISREG(path_observed.st_mode)
                or int(path_observed.st_nlink) != 1
                or int(acquired.st_mode) & 0o077 != 0
                or (hasattr(os, "geteuid") and int(acquired.st_uid) != int(os.geteuid()))
            ):
                raise OSError("private_artifact_lock_invalid")
        except OSError:
            raise
        except Exception:
            raise OSError("private_artifact_lock_authority_unavailable") from None
        yield
    finally:
        if descriptor >= 0:
            if locked:
                try:
                    import fcntl

                    fcntl.flock(descriptor, fcntl.LOCK_UN)
                except (ImportError, OSError):
                    pass
            os.close(descriptor)
        if parent_descriptor >= 0:
            os.close(parent_descriptor)


def read_private_text(path: Path, *, max_bytes: int) -> str:
    """Read one owner-only regular file without following links."""

    if os.name == "nt":
        raise OSError("private_artifact_windows_authority_unavailable")
    target = Path(path)
    if not isinstance(max_bytes, int) or isinstance(max_bytes, bool) or max_bytes <= 0:
        raise OSError("private_artifact_read_invalid")
    if _supports_private_dirfd_io():
        parent_descriptor = -1
        descriptor = -1
        try:
            parent_descriptor = _open_absolute_directory_no_follow(Path(os.path.abspath(target.parent)))
            if parent_descriptor < 0:
                raise OSError
            _validate_private_directory_descriptor(parent_descriptor)
            before = os.stat(target.name, dir_fd=parent_descriptor, follow_symlinks=False)
            if (
                stat.S_ISLNK(before.st_mode)
                or not stat.S_ISREG(before.st_mode)
                or int(before.st_nlink) != 1
                or int(before.st_size) > max_bytes
                or int(before.st_mode) & 0o077 != 0
                or (hasattr(os, "geteuid") and int(before.st_uid) != int(os.geteuid()))
            ):
                raise OSError
            descriptor = os.open(
                target.name,
                os.O_RDONLY
                | int(getattr(os, "O_CLOEXEC", 0))
                | int(getattr(os, "O_NOFOLLOW", 0))
                | int(getattr(os, "O_NONBLOCK", 0)),
                dir_fd=parent_descriptor,
            )
            opened = os.fstat(descriptor)
            if (
                stat.S_ISLNK(before.st_mode)
                or not stat.S_ISREG(before.st_mode)
                or not stat.S_ISREG(opened.st_mode)
                or int(opened.st_nlink) != 1
                or (int(before.st_dev), int(before.st_ino)) != (int(opened.st_dev), int(opened.st_ino))
                or int(opened.st_size) > max_bytes
                or int(opened.st_mode) & 0o077 != 0
                or (hasattr(os, "geteuid") and int(opened.st_uid) != int(os.geteuid()))
            ):
                raise OSError
            chunks: list[bytes] = []
            remaining = max_bytes + 1
            while remaining > 0:
                chunk = os.read(descriptor, min(1024 * 1024, remaining))
                if not chunk:
                    break
                chunks.append(chunk)
                remaining -= len(chunk)
            data = b"".join(chunks)
            if len(data) > max_bytes:
                raise OSError
            return data.decode("utf-8", errors="strict")
        except (OSError, UnicodeError):
            raise OSError("private_artifact_read_invalid") from None
        finally:
            if descriptor >= 0:
                os.close(descriptor)
            if parent_descriptor >= 0:
                os.close(parent_descriptor)
    descriptor = -1
    try:
        parent = target.parent
        if parent.resolve(strict=True) != parent.absolute() or parent.is_symlink():
            raise OSError
        before = target.lstat()
        if (
            stat.S_ISLNK(before.st_mode)
            or not stat.S_ISREG(before.st_mode)
            or int(before.st_nlink) != 1
            or int(before.st_size) > max_bytes
            or int(before.st_mode) & 0o077 != 0
        ):
            raise OSError
        descriptor = os.open(
            target,
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0))
            | int(getattr(os, "O_NONBLOCK", 0)),
        )
        opened = os.fstat(descriptor)
        if (
            stat.S_ISLNK(before.st_mode)
            or not stat.S_ISREG(before.st_mode)
            or not stat.S_ISREG(opened.st_mode)
            or int(opened.st_nlink) != 1
            or (int(before.st_dev), int(before.st_ino)) != (int(opened.st_dev), int(opened.st_ino))
            or int(opened.st_size) > max_bytes
            or (os.name != "nt" and int(opened.st_mode) & 0o077 != 0)
        ):
            raise OSError("private_artifact_read_invalid")
        chunks: list[bytes] = []
        remaining = max_bytes + 1
        while remaining > 0:
            chunk = os.read(descriptor, min(1024 * 1024, remaining))
            if not chunk:
                break
            chunks.append(chunk)
            remaining -= len(chunk)
        data = b"".join(chunks)
        if len(data) > max_bytes:
            raise OSError("private_artifact_read_invalid")
        return data.decode("utf-8", errors="strict")
    except (OSError, UnicodeError):
        raise OSError("private_artifact_read_invalid") from None
    finally:
        if descriptor >= 0:
            os.close(descriptor)


def list_private_regular_files(directory: Path, *, suffix: str) -> list[tuple[Path, os.stat_result]]:
    """List owner-private single-link files from a no-follow directory."""

    if os.name == "nt":
        raise OSError("private_artifact_windows_authority_unavailable")
    root = Path(os.path.abspath(directory))
    if not isinstance(suffix, str) or not suffix:
        raise OSError("private_artifact_list_invalid")
    supports_descriptor_listing = (
        os.name != "nt"
        and os.open in os.supports_dir_fd
        and os.stat in os.supports_dir_fd
        and os.listdir in os.supports_fd
        and bool(getattr(os, "O_DIRECTORY", 0))
        and bool(getattr(os, "O_NOFOLLOW", 0))
    )
    if supports_descriptor_listing:
        try:
            descriptor = _open_absolute_directory_no_follow(root)
        except OSError:
            raise OSError("private_artifact_list_invalid") from None
        if descriptor < 0:
            return []
        try:
            _validate_private_directory_descriptor(descriptor)
            out: list[tuple[Path, os.stat_result]] = []
            for name in os.listdir(descriptor):
                if not name.endswith(suffix) or Path(name).name != name:
                    continue
                try:
                    item = os.stat(name, dir_fd=descriptor, follow_symlinks=False)
                except OSError:
                    raise OSError("private_artifact_list_invalid") from None
                if (
                    not stat.S_ISREG(item.st_mode)
                    or int(item.st_nlink) != 1
                    or (os.name != "nt" and int(item.st_mode) & 0o077 != 0)
                    or (hasattr(os, "geteuid") and int(item.st_uid) != int(os.geteuid()))
                ):
                    raise OSError("private_artifact_list_invalid")
                out.append((root / name, item))
            return out
        finally:
            os.close(descriptor)

    try:
        if root.is_symlink() or root.resolve(strict=True) != root:
            raise OSError
        root_stat = root.stat()
        if os.name != "nt" and int(root_stat.st_mode) & 0o077 != 0:
            raise OSError
        out = []
        with os.scandir(root) as entries:
            for entry in entries:
                if not entry.name.endswith(suffix):
                    continue
                item = entry.stat(follow_symlinks=False)
                if (
                    entry.is_symlink()
                    or not stat.S_ISREG(item.st_mode)
                    or int(item.st_nlink) != 1
                    or (os.name != "nt" and int(item.st_mode) & 0o077 != 0)
                    or (
                        os.name != "nt"
                        and hasattr(os, "geteuid")
                        and int(item.st_uid) != int(os.geteuid())
                    )
                ):
                    raise OSError("private_artifact_list_invalid")
                out.append((root / entry.name, item))
        return out
    except FileNotFoundError:
        return []
    except OSError:
        raise OSError("private_artifact_list_invalid") from None


def list_private_directory_entries(directory: Path) -> list[tuple[Path, os.stat_result]]:
    """List every immediate entry from a stable owner-private no-follow root."""

    if os.name == "nt":
        raise OSError("private_artifact_windows_authority_unavailable")
    root = Path(os.path.abspath(directory))
    supports_descriptor_listing = (
        os.open in os.supports_dir_fd
        and os.stat in os.supports_dir_fd
        and os.listdir in os.supports_fd
        and bool(getattr(os, "O_DIRECTORY", 0))
        and bool(getattr(os, "O_NOFOLLOW", 0))
    )
    if not supports_descriptor_listing:
        raise OSError("private_artifact_list_invalid")
    try:
        descriptor = _open_absolute_directory_no_follow(root)
    except OSError:
        raise OSError("private_artifact_list_invalid") from None
    if descriptor < 0:
        return []
    try:
        before = _validate_private_directory_descriptor(descriptor)
        out: list[tuple[Path, os.stat_result]] = []
        for name in os.listdir(descriptor):
            if name in {"", ".", ".."} or Path(name).name != name:
                raise OSError("private_artifact_list_invalid")
            try:
                item = os.stat(name, dir_fd=descriptor, follow_symlinks=False)
            except OSError:
                raise OSError("private_artifact_list_invalid") from None
            out.append((root / name, item))
        after = os.fstat(descriptor)
        if (
            (int(before.st_dev), int(before.st_ino)) != (int(after.st_dev), int(after.st_ino))
            or int(before.st_mtime_ns) != int(after.st_mtime_ns)
            or int(before.st_ctime_ns) != int(after.st_ctime_ns)
        ):
            raise OSError("private_artifact_list_changed")
        return sorted(out, key=lambda entry: entry[0].name)
    finally:
        os.close(descriptor)


def list_private_directories(directory: Path) -> list[Path]:
    """List every owner-private child directory from a closed directory."""

    out: list[Path] = []
    for path, item in list_private_directory_entries(directory):
        if (
            not stat.S_ISDIR(item.st_mode)
            or int(item.st_mode) & 0o077 != 0
            or (hasattr(os, "geteuid") and int(item.st_uid) != int(os.geteuid()))
        ):
            raise OSError("private_artifact_list_invalid")
        out.append(path)
    return out


def remove_path_no_follow(path: Path) -> None:
    """Remove a file tree without following a symlink at any level."""

    if os.name == "nt":
        raise OSError("private_artifact_windows_authority_unavailable")
    target = Path(path)
    try:
        target_stat = target.lstat()
    except FileNotFoundError:
        return
    if stat.S_ISLNK(target_stat.st_mode) or not stat.S_ISDIR(target_stat.st_mode):
        target.unlink()
        return
    with os.scandir(target) as entries:
        children = [Path(entry.path) for entry in entries]
    for child in children:
        remove_path_no_follow(child)
    target.rmdir()


def remove_path_no_follow_under(root: Path, path: Path) -> None:
    """Remove one descendant without following any ancestor or child link.

    POSIX removal is descriptor-relative from a no-follow opened absolute root,
    so a concurrent directory-to-symlink swap cannot redirect deletion outside
    the owned tree. Other platforms use a fail-closed ancestry check before the
    existing link-aware walker.
    """

    if os.name == "nt":
        raise OSError("private_artifact_windows_authority_unavailable")
    owned_root = Path(os.path.abspath(root))
    target = Path(os.path.abspath(path))
    try:
        relative = target.relative_to(owned_root)
    except ValueError:
        raise OSError("private_artifact_remove_invalid") from None
    parts = relative.parts
    if not parts or any(part in {"", ".", ".."} for part in parts):
        raise OSError("private_artifact_remove_invalid")

    supports_descriptor_removal = (
        os.name != "nt"
        and os.open in os.supports_dir_fd
        and os.stat in os.supports_dir_fd
        and os.unlink in os.supports_dir_fd
        and os.rmdir in os.supports_dir_fd
        and os.listdir in os.supports_fd
        and bool(getattr(os, "O_DIRECTORY", 0))
        and bool(getattr(os, "O_NOFOLLOW", 0))
    )
    if supports_descriptor_removal:
        root_descriptor = _open_absolute_directory_no_follow(owned_root)
        if root_descriptor < 0:
            return
        descriptors = [root_descriptor]
        try:
            parent_descriptor = root_descriptor
            for component in parts[:-1]:
                try:
                    parent_descriptor = os.open(
                        component,
                        os.O_RDONLY
                        | int(getattr(os, "O_CLOEXEC", 0))
                        | int(getattr(os, "O_DIRECTORY", 0))
                        | int(getattr(os, "O_NOFOLLOW", 0)),
                        dir_fd=parent_descriptor,
                    )
                except FileNotFoundError:
                    return
                descriptors.append(parent_descriptor)
            _remove_entry_no_follow_at(parent_descriptor, parts[-1])
            return
        except OSError:
            raise OSError("private_artifact_remove_invalid") from None
        finally:
            for descriptor in reversed(descriptors):
                try:
                    os.close(descriptor)
                except OSError:
                    pass

    try:
        if not owned_root.exists() and not owned_root.is_symlink():
            return
        if owned_root.is_symlink() or owned_root.resolve(strict=True) != owned_root:
            raise OSError
        current = owned_root
        for component in parts[:-1]:
            current = current / component
            if not current.exists() and not current.is_symlink():
                return
            if current.is_symlink() or not current.is_dir() or current.resolve(strict=True) != current:
                raise OSError
        remove_path_no_follow(target)
    except OSError:
        raise OSError("private_artifact_remove_invalid") from None


def remove_private_regular_file_under_if_matches(
    root: Path,
    path: Path,
    *,
    device: int,
    inode: int,
    size: int,
    mtime_ns: int,
) -> bool:
    """Unlink one private file only while its captured identity is unchanged."""

    if os.name == "nt":
        raise OSError("private_artifact_windows_authority_unavailable")
    owned_root = Path(os.path.abspath(root))
    target = Path(os.path.abspath(path))
    try:
        relative = target.relative_to(owned_root)
    except ValueError:
        raise OSError("private_artifact_remove_invalid") from None
    parts = relative.parts
    if not parts or any(component in {"", ".", ".."} for component in parts):
        raise OSError("private_artifact_remove_invalid")
    expected = (int(device), int(inode), int(size), int(mtime_ns))

    if _supports_private_dirfd_io():
        root_descriptor = _open_absolute_directory_no_follow(owned_root)
        if root_descriptor < 0:
            return False
        descriptors = [root_descriptor]
        try:
            parent_descriptor = root_descriptor
            for component in parts[:-1]:
                try:
                    parent_descriptor = os.open(
                        component,
                        os.O_RDONLY
                        | int(getattr(os, "O_CLOEXEC", 0))
                        | int(getattr(os, "O_DIRECTORY", 0))
                        | int(getattr(os, "O_NOFOLLOW", 0)),
                        dir_fd=parent_descriptor,
                    )
                except FileNotFoundError:
                    return False
                descriptors.append(parent_descriptor)
            try:
                observed = os.stat(parts[-1], dir_fd=parent_descriptor, follow_symlinks=False)
            except FileNotFoundError:
                return False
            identity = (
                int(observed.st_dev),
                int(observed.st_ino),
                int(observed.st_size),
                int(observed.st_mtime_ns),
            )
            if (
                identity != expected
                or not stat.S_ISREG(observed.st_mode)
                or int(observed.st_nlink) != 1
                or int(observed.st_mode) & 0o077 != 0
            ):
                raise OSError("private_artifact_remove_changed")
            os.unlink(parts[-1], dir_fd=parent_descriptor)
            os.fsync(parent_descriptor)
            return True
        except OSError:
            raise OSError("private_artifact_remove_invalid") from None
        finally:
            for descriptor in reversed(descriptors):
                try:
                    os.close(descriptor)
                except OSError:
                    pass

    try:
        observed = target.lstat()
        identity = (
            int(observed.st_dev),
            int(observed.st_ino),
            int(observed.st_size),
            int(observed.st_mtime_ns),
        )
        if (
            identity != expected
            or not stat.S_ISREG(observed.st_mode)
            or stat.S_ISLNK(observed.st_mode)
            or (os.name != "nt" and int(observed.st_mode) & 0o077 != 0)
        ):
            raise OSError
        target.unlink()
        return True
    except FileNotFoundError:
        return False
    except OSError:
        raise OSError("private_artifact_remove_invalid") from None


def _open_or_create_absolute_directory_private(path: Path) -> int:
    absolute = Path(os.path.abspath(path))
    flags = (
        os.O_RDONLY
        | int(getattr(os, "O_CLOEXEC", 0))
        | int(getattr(os, "O_DIRECTORY", 0))
        | int(getattr(os, "O_NOFOLLOW", 0))
    )
    anchor = Path(absolute.anchor or os.path.sep)
    descriptor = os.open(anchor, flags)
    try:
        for component in absolute.parts[1:]:
            try:
                next_descriptor = os.open(component, flags, dir_fd=descriptor)
            except FileNotFoundError:
                try:
                    os.mkdir(component, 0o700, dir_fd=descriptor)
                except FileExistsError:
                    pass
                next_descriptor = os.open(component, flags, dir_fd=descriptor)
            os.close(descriptor)
            descriptor = next_descriptor
        observed = os.fstat(descriptor)
        if (
            not stat.S_ISDIR(observed.st_mode)
            or (hasattr(os, "geteuid") and int(observed.st_uid) != int(os.geteuid()))
        ):
            raise OSError("private_artifact_directory_invalid")
        os.fchmod(descriptor, 0o700)
        _validate_private_directory_descriptor(descriptor)
        return descriptor
    except Exception:
        try:
            os.close(descriptor)
        except OSError:
            pass
        raise


def _open_absolute_directory_no_follow(path: Path) -> int:
    flags = (
        os.O_RDONLY
        | int(getattr(os, "O_CLOEXEC", 0))
        | int(getattr(os, "O_DIRECTORY", 0))
        | int(getattr(os, "O_NOFOLLOW", 0))
    )
    anchor = Path(path.anchor or os.path.sep)
    try:
        descriptor = os.open(anchor, flags)
    except FileNotFoundError:
        return -1
    try:
        for component in path.parts[1:]:
            try:
                next_descriptor = os.open(component, flags, dir_fd=descriptor)
            except FileNotFoundError:
                os.close(descriptor)
                return -1
            os.close(descriptor)
            descriptor = next_descriptor
        return descriptor
    except Exception:
        try:
            os.close(descriptor)
        except OSError:
            pass
        raise


def _remove_entry_no_follow_at(parent_descriptor: int, name: str) -> None:
    try:
        entry = os.stat(name, dir_fd=parent_descriptor, follow_symlinks=False)
    except FileNotFoundError:
        return
    if not stat.S_ISDIR(entry.st_mode) or stat.S_ISLNK(entry.st_mode):
        os.unlink(name, dir_fd=parent_descriptor)
        return
    child_descriptor = os.open(
        name,
        os.O_RDONLY
        | int(getattr(os, "O_CLOEXEC", 0))
        | int(getattr(os, "O_DIRECTORY", 0))
        | int(getattr(os, "O_NOFOLLOW", 0)),
        dir_fd=parent_descriptor,
    )
    try:
        for child_name in os.listdir(child_descriptor):
            _remove_entry_no_follow_at(child_descriptor, child_name)
    finally:
        os.close(child_descriptor)
    os.rmdir(name, dir_fd=parent_descriptor)
