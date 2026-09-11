from __future__ import annotations

import hashlib
import json
import os
import stat
from contextlib import contextmanager
from pathlib import Path
from typing import Callable, Iterator, Literal, Mapping


_JOURNAL_VERSION = 1
_PHASES = frozenset({"staged", "visible"})
_SURFACES = frozenset({"case_audit", "task_store"})
_SHA256_HEX_LENGTH = 64


class DiagnosticFileTransactionError(RuntimeError):
    """A fixed-code failure safe for readiness and ordinary diagnostics."""

    def __init__(self, code: str = "diagnostic_migration_unresolved") -> None:
        self.code = str(code or "diagnostic_migration_unresolved")
        super().__init__(self.code)


FaultHook = Callable[[str], None]
Transform = Callable[[bytes | None], bytes]


class DiagnosticFileTransactionLease:
    """One recovered file generation held under its cross-process lease."""

    def __init__(
        self,
        target: Path,
        data: bytes | None,
        *,
        surface: str,
        max_bytes: int,
    ) -> None:
        self._target = target
        self._data = data
        self._surface = surface
        self._max_bytes = max_bytes

    def read(self) -> bytes | None:
        return self._data

    def replace(self, data: bytes, *, fault_hook: FaultHook | None = None) -> bool:
        if not isinstance(data, bytes) or len(data) > self._max_bytes:
            raise DiagnosticFileTransactionError("diagnostic_migration_output_invalid")
        changed = _replace_locked(
            self._target,
            before=self._data,
            data=data,
            surface=self._surface,
            max_bytes=self._max_bytes,
            fault_hook=fault_hook,
        )
        self._data = data
        return changed


@contextmanager
def diagnostic_file_transaction_lease(
    target: Path,
    *,
    surface: Literal["case_audit", "task_store"],
    max_bytes: int,
) -> Iterator[DiagnosticFileTransactionLease]:
    """Recover and hold one lease across a caller-owned read-modify-write."""

    normalized_target = _validated_target(target, surface=surface, max_bytes=max_bytes)
    with _exclusive_file_lock(_sidecar(normalized_target, "lock")):
        _recover_locked(normalized_target, surface=surface, max_bytes=max_bytes)
        data = _read_regular_file(normalized_target, max_bytes=max_bytes, allow_missing=True)
        yield DiagnosticFileTransactionLease(
            normalized_target,
            data,
            surface=surface,
            max_bytes=max_bytes,
        )


def recover_diagnostic_file_transaction(
    target: Path,
    *,
    surface: Literal["case_audit", "task_store"],
    max_bytes: int,
) -> None:
    """Recover one interrupted migration without inspecting diagnostic text."""

    with diagnostic_file_transaction_lease(
        target,
        surface=surface,
        max_bytes=max_bytes,
    ):
        return


def read_diagnostic_file_transactionally(
    target: Path,
    *,
    surface: Literal["case_audit", "task_store"],
    max_bytes: int,
) -> bytes | None:
    """Recover, pin, and read one ordinary diagnostic file under its lease."""

    with diagnostic_file_transaction_lease(
        target,
        surface=surface,
        max_bytes=max_bytes,
    ) as lease:
        return lease.read()


def replace_diagnostic_file_transactionally(
    target: Path,
    data: bytes,
    *,
    surface: Literal["case_audit", "task_store"],
    max_bytes: int,
    fault_hook: FaultHook | None = None,
) -> bool:
    """Durably replace a diagnostic file and retain the prior generation until settlement.

    The journal contains only fixed schema values, byte counts, and complete
    file digests. It never contains source text, paths, case identifiers, SQL,
    account values, or reasoning. The caller owns semantic projection before
    passing ``data``.
    """

    with diagnostic_file_transaction_lease(
        target,
        surface=surface,
        max_bytes=max_bytes,
    ) as lease:
        return lease.replace(data, fault_hook=fault_hook)


def transform_diagnostic_file_transactionally(
    target: Path,
    transform: Transform,
    *,
    surface: Literal["case_audit", "task_store"],
    max_bytes: int,
    fault_hook: FaultHook | None = None,
) -> bytes:
    """Read/project/publish one file while holding the same cross-process lease."""

    with diagnostic_file_transaction_lease(
        target,
        surface=surface,
        max_bytes=max_bytes,
    ) as lease:
        before = lease.read()
        try:
            data = transform(before)
        except DiagnosticFileTransactionError:
            raise
        except Exception:
            raise DiagnosticFileTransactionError("diagnostic_migration_projection_failed") from None
        if not isinstance(data, bytes) or len(data) > max_bytes:
            raise DiagnosticFileTransactionError("diagnostic_migration_output_invalid")
        lease.replace(data, fault_hook=fault_hook)
        return data


def _replace_locked(
    normalized_target: Path,
    *,
    before: bytes | None,
    data: bytes,
    surface: str,
    max_bytes: int,
    fault_hook: FaultHook | None,
) -> bool:
    if (before is None and data == b"") or (before is not None and before == data):
        return False

    stage = _sidecar(normalized_target, "stage")
    backup = _sidecar(normalized_target, "previous")
    journal = _sidecar(normalized_target, "journal")
    if any(_lexists(path) for path in (stage, backup, journal)):
        raise DiagnosticFileTransactionError()

    _write_new_file(stage, data)
    _fault(fault_hook, "stage_fsynced")
    after_sha256 = _sha256(data)
    journal_payload = {
        "version": _JOURNAL_VERSION,
        "surface": surface,
        "phase": "staged",
        "had_target": before is not None,
        "before_sha256": _sha256(before or b""),
        "before_bytes": len(before or b""),
        "after_sha256": after_sha256,
        "after_bytes": len(data),
    }
    _write_journal(journal, journal_payload)
    _fault(fault_hook, "intent_fsynced")

    if before is not None:
        os.replace(normalized_target, backup)
        _fsync_directory(normalized_target.parent)
    _fault(fault_hook, "previous_moved")
    os.replace(stage, normalized_target)
    _fsync_directory(normalized_target.parent)
    _fault(fault_hook, "target_visible")

    journal_payload["phase"] = "visible"
    _write_journal(journal, journal_payload)
    _fault(fault_hook, "visible_fsynced")
    _settle_visible(normalized_target, journal_payload, max_bytes=max_bytes)
    _fault(fault_hook, "settled")
    return True


def _recover_locked(target: Path, *, surface: str, max_bytes: int) -> None:
    journal_path = _sidecar(target, "journal")
    journal_temp = _sidecar(target, "journal-next")
    stage = _sidecar(target, "stage")
    backup = _sidecar(target, "previous")

    if not _lexists(journal_path) and _lexists(journal_temp):
        payload = _read_journal(journal_temp, expected_surface=surface)
        os.replace(journal_temp, journal_path)
        _fsync_directory(target.parent)
    elif _lexists(journal_temp):
        _unlink_regular(journal_temp)
        _fsync_directory(target.parent)

    if not _lexists(journal_path):
        if _lexists(backup):
            raise DiagnosticFileTransactionError()
        if _lexists(stage):
            _unlink_regular(stage)
            _fsync_directory(target.parent)
        if _lexists(target):
            _read_regular_file(target, max_bytes=max_bytes, allow_missing=False)
        return

    payload = _read_journal(journal_path, expected_surface=surface)
    target_generation = _generation(target, payload, max_bytes=max_bytes)
    stage_generation = _generation(stage, payload, max_bytes=max_bytes)
    backup_generation = _generation(backup, payload, max_bytes=max_bytes)

    if target_generation == "after":
        if stage_generation is not None:
            raise DiagnosticFileTransactionError()
        if bool(payload["had_target"]) and backup_generation not in {"before", None}:
            raise DiagnosticFileTransactionError()
        if not bool(payload["had_target"]) and backup_generation is not None:
            raise DiagnosticFileTransactionError()
        _settle_visible(target, payload, max_bytes=max_bytes)
        return

    if target_generation == "before":
        if not bool(payload["had_target"]) or stage_generation != "after" or backup_generation is not None:
            raise DiagnosticFileTransactionError()
        os.replace(target, backup)
        _fsync_directory(target.parent)
        os.replace(stage, target)
        _fsync_directory(target.parent)
        _settle_visible(target, payload, max_bytes=max_bytes)
        return

    if target_generation is None and stage_generation == "after":
        if bool(payload["had_target"]):
            if backup_generation != "before":
                raise DiagnosticFileTransactionError()
        elif backup_generation is not None:
            raise DiagnosticFileTransactionError()
        os.replace(stage, target)
        _fsync_directory(target.parent)
        _settle_visible(target, payload, max_bytes=max_bytes)
        return

    # If the new generation cannot be resumed, restore the exact previous
    # generation when it is unambiguous. The journal is removed only after the
    # rollback is durable and verified.
    if bool(payload["had_target"]) and target_generation is None and backup_generation == "before":
        if _lexists(stage):
            _unlink_regular(stage)
        os.replace(backup, target)
        _fsync_directory(target.parent)
        if _generation(target, payload, max_bytes=max_bytes) != "before":
            raise DiagnosticFileTransactionError()
        _unlink_regular(journal_path)
        _fsync_directory(target.parent)
        return
    raise DiagnosticFileTransactionError()


def _settle_visible(target: Path, payload: Mapping[str, object], *, max_bytes: int) -> None:
    if _generation(target, payload, max_bytes=max_bytes) != "after":
        raise DiagnosticFileTransactionError()
    stage = _sidecar(target, "stage")
    backup = _sidecar(target, "previous")
    journal = _sidecar(target, "journal")
    if _lexists(stage):
        raise DiagnosticFileTransactionError()
    if _lexists(backup):
        if _generation(backup, payload, max_bytes=max_bytes) != "before":
            raise DiagnosticFileTransactionError()
        _unlink_regular(backup)
        _fsync_directory(target.parent)
    if _lexists(journal):
        _unlink_regular(journal)
        _fsync_directory(target.parent)


def _validated_target(target: Path, *, surface: str, max_bytes: int) -> Path:
    if surface not in _SURFACES or not isinstance(max_bytes, int) or isinstance(max_bytes, bool) or max_bytes <= 0:
        raise DiagnosticFileTransactionError("diagnostic_migration_request_invalid")
    try:
        candidate = Path(target)
        if not candidate.is_absolute() or candidate.name in {"", ".", ".."}:
            raise OSError
        parent = candidate.parent
        canonical_parent = parent.resolve(strict=True)
        if canonical_parent != parent:
            raise OSError
        parent_stat = parent.lstat()
        if stat.S_ISLNK(parent_stat.st_mode) or not stat.S_ISDIR(parent_stat.st_mode):
            raise OSError
    except (OSError, RuntimeError):
        raise DiagnosticFileTransactionError("diagnostic_migration_target_invalid") from None
    return candidate


def _sidecar(target: Path, suffix: str) -> Path:
    return target.with_name(f".{target.name}.analytix-diagnostic-v1.{suffix}")


def _read_regular_file(path: Path, *, max_bytes: int, allow_missing: bool) -> bytes | None:
    descriptor = -1
    try:
        before = path.lstat()
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
            or (int(before.st_dev), int(before.st_ino)) != (int(opened.st_dev), int(opened.st_ino))
            or int(opened.st_size) > max_bytes
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
        after = path.lstat()
        after_opened = os.fstat(descriptor)
        if (
            len(data) > max_bytes
            or (int(after.st_dev), int(after.st_ino)) != (int(opened.st_dev), int(opened.st_ino))
            or int(after_opened.st_size) != int(opened.st_size)
            or int(getattr(after_opened, "st_mtime_ns", 0)) != int(getattr(opened, "st_mtime_ns", 0))
        ):
            raise OSError
        return data
    except FileNotFoundError:
        if allow_missing:
            return None
        raise DiagnosticFileTransactionError() from None
    except OSError:
        raise DiagnosticFileTransactionError() from None
    finally:
        if descriptor >= 0:
            os.close(descriptor)


def _write_new_file(path: Path, data: bytes) -> None:
    descriptor = -1
    try:
        descriptor = os.open(
            path,
            os.O_WRONLY
            | os.O_CREAT
            | os.O_EXCL
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0)),
            0o600,
        )
        view = memoryview(data)
        while view:
            written = os.write(descriptor, view)
            if written <= 0:
                raise OSError
            view = view[written:]
        os.fsync(descriptor)
    except OSError:
        raise DiagnosticFileTransactionError() from None
    finally:
        if descriptor >= 0:
            os.close(descriptor)


def _write_journal(path: Path, payload: Mapping[str, object]) -> None:
    _validate_journal(payload, expected_surface=str(payload.get("surface") or ""))
    temp = path.with_name(path.name.replace(".journal", ".journal-next"))
    if _lexists(temp):
        _unlink_regular(temp)
    encoded = json.dumps(payload, ensure_ascii=True, separators=(",", ":"), sort_keys=True).encode("ascii")
    _write_new_file(temp, encoded)
    os.replace(temp, path)
    _fsync_directory(path.parent)


def _read_journal(path: Path, *, expected_surface: str) -> dict[str, object]:
    raw = _read_regular_file(path, max_bytes=4096, allow_missing=False)
    try:
        payload = json.loads((raw or b"").decode("ascii"), object_pairs_hook=_strict_object)
    except (UnicodeError, json.JSONDecodeError, ValueError):
        raise DiagnosticFileTransactionError() from None
    if not isinstance(payload, dict):
        raise DiagnosticFileTransactionError()
    _validate_journal(payload, expected_surface=expected_surface)
    return payload


def _validate_journal(payload: Mapping[str, object], *, expected_surface: str) -> None:
    if set(payload) != {
        "version",
        "surface",
        "phase",
        "had_target",
        "before_sha256",
        "before_bytes",
        "after_sha256",
        "after_bytes",
    }:
        raise DiagnosticFileTransactionError()
    if (
        payload.get("version") != _JOURNAL_VERSION
        or payload.get("surface") != expected_surface
        or expected_surface not in _SURFACES
        or payload.get("phase") not in _PHASES
        or type(payload.get("had_target")) is not bool
    ):
        raise DiagnosticFileTransactionError()
    for key in ("before_sha256", "after_sha256"):
        value = payload.get(key)
        if not isinstance(value, str) or len(value) != _SHA256_HEX_LENGTH or any(
            character not in "0123456789abcdef" for character in value
        ):
            raise DiagnosticFileTransactionError()
    for key in ("before_bytes", "after_bytes"):
        value = payload.get(key)
        if type(value) is not int or value < 0:
            raise DiagnosticFileTransactionError()


def _generation(path: Path, payload: Mapping[str, object], *, max_bytes: int) -> str | None:
    data = _read_regular_file(path, max_bytes=max_bytes, allow_missing=True)
    if data is None:
        return None
    digest = _sha256(data)
    size = len(data)
    if digest == payload["before_sha256"] and size == payload["before_bytes"]:
        return "before"
    if digest == payload["after_sha256"] and size == payload["after_bytes"]:
        return "after"
    return "unknown"


def _unlink_regular(path: Path) -> None:
    try:
        current = path.lstat()
        if stat.S_ISLNK(current.st_mode) or not stat.S_ISREG(current.st_mode) or int(current.st_nlink) != 1:
            raise OSError
        path.unlink()
    except OSError:
        raise DiagnosticFileTransactionError() from None


def _fsync_directory(path: Path) -> None:
    descriptor = -1
    try:
        descriptor = os.open(path, os.O_RDONLY | int(getattr(os, "O_CLOEXEC", 0)))
        os.fsync(descriptor)
    except OSError:
        raise DiagnosticFileTransactionError() from None
    finally:
        if descriptor >= 0:
            os.close(descriptor)


@contextmanager
def _exclusive_file_lock(path: Path) -> Iterator[None]:
    descriptor = -1
    try:
        descriptor = os.open(
            path,
            os.O_RDWR
            | os.O_CREAT
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0)),
            0o600,
        )
        opened = os.fstat(descriptor)
        if not stat.S_ISREG(opened.st_mode) or int(opened.st_nlink) != 1:
            raise OSError
        if os.name == "nt":
            import msvcrt

            if os.fstat(descriptor).st_size == 0:
                os.write(descriptor, b"0")
                os.lseek(descriptor, 0, os.SEEK_SET)
            msvcrt.locking(descriptor, msvcrt.LK_LOCK, 1)
        else:
            import fcntl

            fcntl.flock(descriptor, fcntl.LOCK_EX)
        yield
    except DiagnosticFileTransactionError:
        raise
    except OSError:
        raise DiagnosticFileTransactionError() from None
    finally:
        if descriptor >= 0:
            try:
                if os.name == "nt":
                    import msvcrt

                    os.lseek(descriptor, 0, os.SEEK_SET)
                    msvcrt.locking(descriptor, msvcrt.LK_UNLCK, 1)
                else:
                    import fcntl

                    fcntl.flock(descriptor, fcntl.LOCK_UN)
            except OSError:
                pass
            os.close(descriptor)


def _strict_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate journal key")
        result[key] = value
    return result


def _sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _lexists(path: Path) -> bool:
    try:
        path.lstat()
        return True
    except FileNotFoundError:
        return False
    except OSError:
        raise DiagnosticFileTransactionError() from None


def _fault(hook: FaultHook | None, point: str) -> None:
    if hook is not None:
        hook(point)


__all__ = [
    "DiagnosticFileTransactionLease",
    "DiagnosticFileTransactionError",
    "diagnostic_file_transaction_lease",
    "read_diagnostic_file_transactionally",
    "recover_diagnostic_file_transaction",
    "replace_diagnostic_file_transactionally",
    "transform_diagnostic_file_transactionally",
]
