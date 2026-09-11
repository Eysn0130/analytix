from __future__ import annotations

import ctypes
import errno
import hashlib
import json
import os
import secrets
import shutil
import stat
import sys
import time
from collections import OrderedDict
from contextlib import contextmanager
from dataclasses import dataclass
from pathlib import Path
from threading import RLock, local
from types import MappingProxyType
from typing import BinaryIO, Dict, Iterator, List, Mapping, Optional

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
from app.core.managed_subprocess import (
    ManagedProcessLaunchError,
    ManagedProcessTerminationError,
    ManagedProcessTimeout,
    run_managed_process,
)
from app.core.immutable_generation import (
    ImmutableGeneration,
    ImmutableGenerationError,
    publish_generated,
)


SUPPORTED_ARCHIVE_EXTS = frozenset({".zip", ".rar", ".7z", ".tar", ".gz", ".tgz", ".bz2", ".tbz", ".xz", ".txz"})
ARCHIVE_EXTRACTOR_ENV = "ANALYTIX_ARCHIVE_EXTRACTOR_BIN"
_MARKER_FILE = ".analytix-archive-extract.json"
_MARKER_KEYS = frozenset(
    {
        "schema_version",
        "publication_nonce",
        "source_device",
        "source_inode",
        "source_size",
        "source_mtime_ns",
        "source_ctime_ns",
        "source_sha256",
        "output_entry_count",
        "output_file_count",
        "output_total_bytes",
        "output_tree_sha256",
        "output_inventory",
    }
)
_INVENTORY_ENTRY_KEYS = frozenset(
    {
        "path",
        "type",
        "size",
        "sha256",
        "device",
        "inode",
        "uid",
        "mode",
        "links",
        "mtime_ns",
        "ctime_ns",
        "generation",
    }
)
_INTERNAL_EXTRACT_DIRS = frozenset({".import_preview_cache"})
_ARCHIVE_RUNTIME_DIR = ".analytix-runtime-v1"
_ARCHIVE_EXTRACTOR_NAMES = frozenset({"7z", "7z.exe", "7za", "7za.exe", "7zz", "7zz.exe", "bsdtar", "tar"})
_SEVEN_ZIP_EXTS = frozenset(SUPPORTED_ARCHIVE_EXTS - {".rar"})
_SEVEN_ZIP_PASSWORD_EXTS = frozenset({".7z", ".zip"})
_BSDTAR_EXTS = frozenset({".zip", ".tar", ".gz", ".tgz", ".bz2", ".tbz", ".xz", ".txz"})
_TAR_EXTS = frozenset({".tar", ".gz", ".tgz", ".bz2", ".tbz", ".xz", ".txz"})
_ARCHIVE_TIMEOUT_SECONDS = 300.0
_ARCHIVE_MAX_ENTRIES = 50_000
_ARCHIVE_MAX_TOTAL_BYTES = 2 * 1024 * 1024 * 1024
_ARCHIVE_MAX_FILE_BYTES = 512 * 1024 * 1024
_ARCHIVE_MAX_DEPTH = 32
_ARCHIVE_MAX_PATH_BYTES = 1024
_ARCHIVE_MAX_PARENT_COMPONENTS = 128
_ARCHIVE_MIN_FREE_BYTES = 64 * 1024 * 1024
_ARCHIVE_MAX_MARKER_BYTES = 128 * 1024 * 1024
_ARCHIVE_PUBLICATION_LOCK_TIMEOUT_SECONDS = 2.0
_ARCHIVE_MAX_RETAINED_ATTEMPTS_PER_PARENT = 32
_ARCHIVE_MAX_RETAINED_BYTES_PER_PARENT = 4 * 1024 * 1024 * 1024


@dataclass(frozen=True)
class _ArchiveSourceIdentity:
    device: int
    inode: int
    size: int
    mtime_ns: int
    ctime_ns: int
    sha256: str

    def marker(self) -> dict[str, object]:
        return {
            "schema_version": 4,
            "source_device": self.device,
            "source_inode": self.inode,
            "source_size": self.size,
            "source_mtime_ns": self.mtime_ns,
            "source_ctime_ns": self.ctime_ns,
            "source_sha256": self.sha256,
        }


@dataclass(frozen=True)
class _ArchiveTreeEntry:
    path: str
    entry_type: str
    size: int
    sha256: str
    device: int
    inode: int
    uid: int
    mode: int
    links: int
    mtime_ns: int
    ctime_ns: int
    generation: int

    def marker(self) -> dict[str, object]:
        return {
            "path": self.path,
            "type": self.entry_type,
            "size": self.size,
            "sha256": self.sha256,
            "device": self.device,
            "inode": self.inode,
            "uid": self.uid,
            "mode": self.mode,
            "links": self.links,
            "mtime_ns": self.mtime_ns,
            "ctime_ns": self.ctime_ns,
            "generation": self.generation,
        }

    def content_record(self) -> bytes:
        return _tree_record(self.path, self.entry_type, self.size, self.sha256)


@dataclass(frozen=True)
class _ArchiveTreeSummary:
    entry_count: int
    file_count: int
    total_bytes: int
    sha256: str
    inventory: tuple[_ArchiveTreeEntry, ...]

    def marker(self) -> dict[str, object]:
        return {
            "output_entry_count": self.entry_count,
            "output_file_count": self.file_count,
            "output_total_bytes": self.total_bytes,
            "output_tree_sha256": self.sha256,
            "output_inventory": [entry.marker() for entry in self.inventory],
        }


@dataclass(frozen=True)
class _PublishedArchiveGenerationV1:
    publication_nonce: str
    source_identity: _ArchiveSourceIdentity
    root_identity: tuple[int, int]
    root_state: tuple[int, ...]
    summary: _ArchiveTreeSummary
    root_fd: int


@dataclass(frozen=True)
class _RegisteredArchiveGenerationV1:
    publication_nonce: str
    source_identity: _ArchiveSourceIdentity
    root_identity: tuple[int, int]
    root_state: tuple[int, ...]
    summary: _ArchiveTreeSummary
    root_fd: int


@dataclass(frozen=True)
class _ArchiveGenerationReservationV1:
    token: str
    logical_path: str


_ArchiveGenerationRegistryKey = tuple[tuple[tuple[int, int], ...], str]


@dataclass(frozen=True)
class _ArchiveGenerationRegistryStateV1:
    process_id: int
    registry: Mapping[
        _ArchiveGenerationRegistryKey,
        _RegisteredArchiveGenerationV1,
    ]
    reservations: Mapping[str, _ArchiveGenerationReservationV1]
    reservations_by_path: Mapping[str, str]
    reservations_by_key: Mapping[_ArchiveGenerationRegistryKey, str]
    accepted_logical_paths: Mapping[str, _ArchiveGenerationRegistryKey]


def _new_archive_generation_registry_state(
    *,
    process_id: int,
    registry: Mapping[
        _ArchiveGenerationRegistryKey,
        _RegisteredArchiveGenerationV1,
    ] | None = None,
    reservations: Mapping[str, _ArchiveGenerationReservationV1] | None = None,
    reservations_by_path: Mapping[str, str] | None = None,
    reservations_by_key: Mapping[_ArchiveGenerationRegistryKey, str] | None = None,
    accepted_logical_paths: Mapping[str, _ArchiveGenerationRegistryKey] | None = None,
) -> _ArchiveGenerationRegistryStateV1:
    return _ArchiveGenerationRegistryStateV1(
        process_id=process_id,
        registry=MappingProxyType(OrderedDict(registry or {})),
        reservations=MappingProxyType(dict(reservations or {})),
        reservations_by_path=MappingProxyType(dict(reservations_by_path or {})),
        reservations_by_key=MappingProxyType(dict(reservations_by_key or {})),
        accepted_logical_paths=MappingProxyType(dict(accepted_logical_paths or {})),
    )


class _ArchiveGenerationRegistryView(Mapping[_ArchiveGenerationRegistryKey, _RegisteredArchiveGenerationV1]):
    """Read-only compatibility view; canonical state is one immutable snapshot."""

    def __getitem__(
        self,
        key: _ArchiveGenerationRegistryKey,
    ) -> _RegisteredArchiveGenerationV1:
        return _ARCHIVE_GENERATION_STATE.registry[key]

    def __iter__(self):
        return iter(_ARCHIVE_GENERATION_STATE.registry)

    def __len__(self) -> int:
        return len(_ARCHIVE_GENERATION_STATE.registry)


_ARCHIVE_PUBLICATION_LOCK_STATE = local()
_ARCHIVE_GENERATION_REGISTRY_LOCK = RLock()
_ARCHIVE_GENERATION_REGISTRY_MAX = 1024
_ARCHIVE_GENERATION_STATE = _new_archive_generation_registry_state(
    process_id=os.getpid(),
)
_ARCHIVE_GENERATION_REGISTRY: Mapping[
    _ArchiveGenerationRegistryKey,
    _RegisteredArchiveGenerationV1,
] = _ArchiveGenerationRegistryView()


def _install_archive_generation_state(
    state: _ArchiveGenerationRegistryStateV1,
) -> None:
    global _ARCHIVE_GENERATION_STATE

    _ARCHIVE_GENERATION_STATE = state


def _reset_archive_generation_registry_after_fork() -> None:
    global _ARCHIVE_PUBLICATION_LOCK_STATE
    global _ARCHIVE_GENERATION_REGISTRY_LOCK
    global _ARCHIVE_GENERATION_STATE

    previous_state = _ARCHIVE_GENERATION_STATE
    _ARCHIVE_PUBLICATION_LOCK_STATE = local()
    _ARCHIVE_GENERATION_REGISTRY_LOCK = RLock()
    empty_state = _new_archive_generation_registry_state(process_id=os.getpid())
    try:
        _install_archive_generation_state(empty_state)
    except BaseException:
        if _ARCHIVE_GENERATION_STATE is empty_state:
            _close_archive_generation_registry_state(previous_state)
        raise
    # The empty snapshot becomes observable before any descriptor from the
    # old process image is closed. A signal can therefore never observe a live
    # registry record whose authority FD has already been invalidated.
    _close_archive_generation_registry_state(previous_state)


def _close_archive_generation_registry_state(
    state: _ArchiveGenerationRegistryStateV1,
) -> None:
    interrupted: BaseException | None = None
    for record in tuple(state.registry.values()):
        try:
            os.close(record.root_fd)
        except OSError:
            pass
        except BaseException as error:
            try:
                os.close(record.root_fd)
            except BaseException:
                pass
            if interrupted is None:
                interrupted = error
    if interrupted is not None:
        raise interrupted


if hasattr(os, "register_at_fork"):
    os.register_at_fork(after_in_child=_reset_archive_generation_registry_after_fork)


@dataclass
class _PinnedArchivePublishParent:
    path: Path
    output_name: str
    descriptors: tuple[int, ...]
    edge_names: tuple[str, ...]
    identities: tuple[tuple[int, int], ...]

    @property
    def descriptor(self) -> int:
        if not self.descriptors:
            raise ExternalRuntimeError("archive_publish_indeterminate")
        return self.descriptors[-1]

    def verify(self) -> None:
        if len(self.descriptors) != len(self.identities) or len(self.edge_names) + 1 != len(self.descriptors):
            raise ExternalRuntimeError("archive_publish_indeterminate")
        try:
            for descriptor, expected in zip(self.descriptors, self.identities):
                opened = os.fstat(descriptor)
                if not stat.S_ISDIR(opened.st_mode) or _identity(opened) != expected:
                    raise OSError
            for index, name in enumerate(self.edge_names):
                child = os.stat(name, dir_fd=self.descriptors[index], follow_symlinks=False)
                if stat.S_ISLNK(child.st_mode) or not stat.S_ISDIR(child.st_mode):
                    raise OSError
                if _identity(child) != self.identities[index + 1]:
                    raise OSError
        except OSError:
            raise ExternalRuntimeError("archive_publish_indeterminate") from None

    def close(self) -> None:
        interrupted: BaseException | None = None
        for descriptor in reversed(self.descriptors):
            try:
                os.close(descriptor)
            except OSError:
                pass
            except BaseException as error:
                try:
                    os.close(descriptor)
                except BaseException:
                    pass
                if interrupted is None:
                    interrupted = error
        self.descriptors = ()
        if interrupted is not None:
            raise interrupted


def _archive_generation_registry_key(
    parent: _PinnedArchivePublishParent,
) -> tuple[tuple[tuple[int, int], ...], str]:
    return parent.identities, parent.output_name


def _ensure_archive_generation_registry_process() -> None:
    if _ARCHIVE_GENERATION_STATE.process_id != os.getpid():
        _reset_archive_generation_registry_after_fork()


def _reserve_archive_generation_path(
    output_dir: Path,
) -> _ArchiveGenerationReservationV1:
    _ensure_archive_generation_registry_process()
    _validate_archive_output_path(output_dir)
    logical_path = os.fspath(output_dir)
    reservation: _ArchiveGenerationReservationV1 | None = None
    try:
        with _ARCHIVE_GENERATION_REGISTRY_LOCK:
            state = _ARCHIVE_GENERATION_STATE
            if logical_path in state.accepted_logical_paths:
                raise ExternalRuntimeError("archive_publish_indeterminate")
            if logical_path in state.reservations_by_path:
                raise ExternalRuntimeError("archive_generation_in_progress")
            if (
                len(state.registry) + len(state.reservations)
                >= _ARCHIVE_GENERATION_REGISTRY_MAX
            ):
                raise ExternalRuntimeError("archive_generation_registry_capacity_exceeded")
            reservation = _ArchiveGenerationReservationV1(
                token=secrets.token_hex(32),
                logical_path=logical_path,
            )
            reservations = dict(state.reservations)
            reservations[reservation.token] = reservation
            reservations_by_path = dict(state.reservations_by_path)
            reservations_by_path[logical_path] = reservation.token
            _install_archive_generation_state(
                _new_archive_generation_registry_state(
                    process_id=state.process_id,
                    registry=state.registry,
                    reservations=reservations,
                    reservations_by_path=reservations_by_path,
                    reservations_by_key=state.reservations_by_key,
                    accepted_logical_paths=state.accepted_logical_paths,
                )
            )
        return reservation
    except BaseException:
        # A signal may arrive after the one-pointer commit but before the
        # reservation is returned to its caller. Revoke that complete snapshot
        # rather than leaving an ownerless capacity/path claim.
        _release_archive_generation_reservation(reservation)
        raise


def _bind_archive_generation_reservation(
    parent: _PinnedArchivePublishParent,
    reservation: _ArchiveGenerationReservationV1,
) -> None:
    _ensure_archive_generation_registry_process()
    key = _archive_generation_registry_key(parent)
    with _ARCHIVE_GENERATION_REGISTRY_LOCK:
        state = _ARCHIVE_GENERATION_STATE
        current = state.reservations.get(reservation.token)
        if (
            current != reservation
            or reservation.logical_path
            != os.fspath(parent.path / parent.output_name)
            or state.reservations_by_path.get(reservation.logical_path)
            != reservation.token
            or key in state.registry
            or key in state.reservations_by_key
        ):
            raise ExternalRuntimeError("archive_generation_reservation_invalid")
        reservations_by_key = dict(state.reservations_by_key)
        reservations_by_key[key] = reservation.token
        _install_archive_generation_state(
            _new_archive_generation_registry_state(
                process_id=state.process_id,
                registry=state.registry,
                reservations=state.reservations,
                reservations_by_path=state.reservations_by_path,
                reservations_by_key=reservations_by_key,
                accepted_logical_paths=state.accepted_logical_paths,
            )
        )


def _assert_archive_generation_reservation(
    parent: _PinnedArchivePublishParent,
    reservation: _ArchiveGenerationReservationV1,
) -> None:
    _ensure_archive_generation_registry_process()
    key = _archive_generation_registry_key(parent)
    with _ARCHIVE_GENERATION_REGISTRY_LOCK:
        state = _ARCHIVE_GENERATION_STATE
        if (
            state.reservations.get(reservation.token) != reservation
            or state.reservations_by_key.get(key)
            != reservation.token
            or state.reservations_by_path.get(reservation.logical_path)
            != reservation.token
        ):
            raise ExternalRuntimeError("archive_generation_reservation_invalid")


def _release_archive_generation_reservation(
    reservation: _ArchiveGenerationReservationV1 | None,
) -> None:
    if reservation is None:
        return
    _ensure_archive_generation_registry_process()
    with _ARCHIVE_GENERATION_REGISTRY_LOCK:
        state = _ARCHIVE_GENERATION_STATE
        if state.reservations.get(reservation.token) != reservation:
            return
        reservations = dict(state.reservations)
        reservations.pop(reservation.token, None)
        reservations_by_path = dict(state.reservations_by_path)
        if reservations_by_path.get(reservation.logical_path) == reservation.token:
            reservations_by_path.pop(reservation.logical_path, None)
        reservations_by_key = {
            key: token
            for key, token in state.reservations_by_key.items()
            if token != reservation.token
        }
        _install_archive_generation_state(
            _new_archive_generation_registry_state(
                process_id=state.process_id,
                registry=state.registry,
                reservations=reservations,
                reservations_by_path=reservations_by_path,
                reservations_by_key=reservations_by_key,
                accepted_logical_paths=state.accepted_logical_paths,
            )
        )


def _archive_generation_registry_contains(parent: _PinnedArchivePublishParent) -> bool:
    _ensure_archive_generation_registry_process()
    key = _archive_generation_registry_key(parent)
    with _ARCHIVE_GENERATION_REGISTRY_LOCK:
        return key in _ARCHIVE_GENERATION_STATE.registry


def _archive_generation_registry_accepts_identity(
    parent: _PinnedArchivePublishParent,
    expected_identity: tuple[int, int],
) -> bool:
    _ensure_archive_generation_registry_process()
    key = _archive_generation_registry_key(parent)
    logical_path = os.fspath(parent.path / parent.output_name)
    with _ARCHIVE_GENERATION_REGISTRY_LOCK:
        state = _ARCHIVE_GENERATION_STATE
        record = state.registry.get(key)
        return bool(
            record is not None
            and record.root_identity == expected_identity
            and state.accepted_logical_paths.get(logical_path) == key
        )


def _archive_logical_path_accepted(output_dir: Path) -> bool:
    _ensure_archive_generation_registry_process()
    with _ARCHIVE_GENERATION_REGISTRY_LOCK:
        return os.fspath(output_dir) in _ARCHIVE_GENERATION_STATE.accepted_logical_paths


def _open_registered_archive_generation(
    parent: _PinnedArchivePublishParent,
    visible: os.stat_result,
    *,
    expected_source: _ArchiveSourceIdentity | None = None,
) -> _PublishedArchiveGenerationV1 | None:
    _ensure_archive_generation_registry_process()
    key = _archive_generation_registry_key(parent)
    duplicate = -1
    with _ARCHIVE_GENERATION_REGISTRY_LOCK:
        record = _ARCHIVE_GENERATION_STATE.registry.get(key)
        if (
            record is None
            or record.root_identity != _identity(visible)
            or (expected_source is not None and record.source_identity != expected_source)
        ):
            return None
        try:
            duplicate = _reopen_archive_directory_fd(record.root_fd)
        except OSError:
            return None
        publication_nonce = record.publication_nonce
        source_identity = record.source_identity
        root_identity = record.root_identity
        root_state = record.root_state
        summary = record.summary

    valid = False
    try:
        opened = os.fstat(duplicate)
        visible_after = _stat_archive_sibling(parent, parent.output_name)
        marker = _read_extract_marker_fd(duplicate)
        current_summary = _snapshot_extracted_tree_fd(duplicate)
        valid = bool(
            visible_after is not None
            and _identity(opened) == root_identity
            and _identity(visible_after) == root_identity
            and _archive_object_state(opened) == root_state
            and current_summary == summary
            and marker
            == {
                **source_identity.marker(),
                **summary.marker(),
                "publication_nonce": publication_nonce,
            }
        )
        parent.verify()
    except (OSError, ExternalRuntimeError):
        valid = False
    except BaseException:
        try:
            os.close(duplicate)
        except OSError:
            pass
        raise

    if not valid:
        os.close(duplicate)
        return None
    with _ARCHIVE_GENERATION_REGISTRY_LOCK:
        still_registered = (
            _ARCHIVE_GENERATION_STATE.process_id == os.getpid()
            and _ARCHIVE_GENERATION_STATE.registry.get(key) is record
        )
    if not still_registered:
        os.close(duplicate)
        return None
    return _PublishedArchiveGenerationV1(
        publication_nonce=publication_nonce,
        source_identity=source_identity,
        root_identity=root_identity,
        root_state=root_state,
        summary=summary,
        root_fd=duplicate,
    )


def _registered_archive_generation_matches(
    parent: _PinnedArchivePublishParent,
    visible: os.stat_result,
) -> bool:
    published = _open_registered_archive_generation(parent, visible)
    if published is None:
        return False
    os.close(published.root_fd)
    return True


def _register_archive_generation(
    parent: _PinnedArchivePublishParent,
    published: _PublishedArchiveGenerationV1,
    reservation: _ArchiveGenerationReservationV1,
) -> None:
    _ensure_archive_generation_registry_process()
    key = _archive_generation_registry_key(parent)
    duplicate = -1
    record: _RegisteredArchiveGenerationV1 | None = None
    try:
        current = _stat_archive_sibling(parent, parent.output_name)
        if (
            current is None
            or _identity(current) != published.root_identity
            or not _published_archive_generation_fd_matches(published)
        ):
            raise OSError
        duplicate = _reopen_archive_directory_fd(published.root_fd)
        record = _RegisteredArchiveGenerationV1(
            publication_nonce=published.publication_nonce,
            source_identity=published.source_identity,
            root_identity=published.root_identity,
            root_state=published.root_state,
            summary=published.summary,
            root_fd=duplicate,
        )
        with _ARCHIVE_GENERATION_REGISTRY_LOCK:
            state = _ARCHIVE_GENERATION_STATE
            if (
                state.reservations.get(reservation.token) != reservation
                or reservation.logical_path
                != os.fspath(parent.path / parent.output_name)
                or state.reservations_by_path.get(reservation.logical_path)
                != reservation.token
                or state.reservations_by_key.get(key) != reservation.token
                or key in state.registry
                or reservation.logical_path in state.accepted_logical_paths
            ):
                raise ExternalRuntimeError("archive_generation_reservation_invalid")
            registry = OrderedDict(state.registry)
            registry[key] = record
            accepted_logical_paths = dict(state.accepted_logical_paths)
            accepted_logical_paths[reservation.logical_path] = key
            reservations = dict(state.reservations)
            reservations.pop(reservation.token, None)
            reservations_by_path = dict(state.reservations_by_path)
            reservations_by_path.pop(reservation.logical_path, None)
            reservations_by_key = dict(state.reservations_by_key)
            reservations_by_key.pop(key, None)
            _install_archive_generation_state(
                _new_archive_generation_registry_state(
                    process_id=state.process_id,
                    registry=registry,
                    reservations=reservations,
                    reservations_by_path=reservations_by_path,
                    reservations_by_key=reservations_by_key,
                    accepted_logical_paths=accepted_logical_paths,
                )
            )
    except BaseException as error:
        # The registry owns the duplicate only if the single snapshot pointer
        # already references this exact record. This inspection also closes
        # signal windows after dup and immediately after the pointer swap.
        with _ARCHIVE_GENERATION_REGISTRY_LOCK:
            committed = (
                record is not None
                and _ARCHIVE_GENERATION_STATE.registry.get(key) is record
            )
        if not committed and duplicate >= 0:
            os.close(duplicate)
        if isinstance(error, OSError):
            raise ExternalRuntimeError("archive_publish_indeterminate") from None
        raise


def _published_archive_marker(
    published: _PublishedArchiveGenerationV1,
) -> dict[str, object]:
    return {
        **published.source_identity.marker(),
        **published.summary.marker(),
        "publication_nonce": published.publication_nonce,
    }


def _published_archive_generation_fd_matches(
    published: _PublishedArchiveGenerationV1,
    *,
    require_root_state: bool = True,
) -> bool:
    try:
        opened = os.fstat(published.root_fd)
        return bool(
            _identity(opened) == published.root_identity
            and (
                not require_root_state
                or not published.root_state
                or _archive_object_state(opened) == published.root_state
            )
            and _snapshot_extracted_tree_fd(published.root_fd) == published.summary
            and _read_extract_marker_fd(published.root_fd)
            == _published_archive_marker(published)
        )
    except (OSError, ExternalRuntimeError):
        return False


@dataclass(frozen=True)
class ArchiveMemberReceiptV1:
    relative_path: str
    entry_type: str
    size: int
    sha256: str
    device: int
    inode: int
    uid: int
    mode: int
    links: int
    mtime_ns: int
    ctime_ns: int
    generation: int


@dataclass(frozen=True)
class ArchiveExtractionReceiptV1:
    schema_version: int
    publication_nonce: str
    source_sha256: str
    source_device: int
    source_inode: int
    source_size: int
    source_mtime_ns: int
    source_ctime_ns: int
    root_device: int
    root_inode: int
    entry_count: int
    file_count: int
    total_bytes: int
    tree_sha256: str
    members: tuple[ArchiveMemberReceiptV1, ...]


class ArchiveExtractionLeaseV1:
    def __init__(
        self,
        *,
        parent: _PinnedArchivePublishParent,
        root_fd: int,
        root_identity: tuple[int, int],
        root_state: tuple[int, ...],
        summary: _ArchiveTreeSummary,
        receipt: ArchiveExtractionReceiptV1,
    ) -> None:
        self._parent = parent
        self._root_fd = root_fd
        self._root_identity = root_identity
        self._root_state = root_state
        self._summary = summary
        self.receipt = receipt
        self._entries = {entry.path: entry for entry in summary.inventory}
        self._closed = False

    def file_members(self) -> tuple[ArchiveMemberReceiptV1, ...]:
        self._require_open()
        return tuple(
            member
            for member in self.receipt.members
            if member.entry_type == "file"
        )

    def publish_member(
        self,
        member: ArchiveMemberReceiptV1,
        generation_root: Path,
        *,
        lineage: Mapping[str, object],
    ) -> ImmutableGeneration:
        self._require_open()
        if _is_archive_runtime_entry(member.relative_path):
            raise ExternalRuntimeError("archive_member_receipt_invalid")
        entry = self._entries.get(member.relative_path)
        if entry is None or _member_receipt(entry) != member or entry.entry_type != "file":
            raise ExternalRuntimeError("archive_member_receipt_invalid")
        parent_fd = -1
        file_fd = -1
        try:
            parent_fd, file_fd, leaf_name = _open_archive_member_fd(self._root_fd, entry, self._entries)
            before = os.fstat(file_fd)
            if not _stat_matches_tree_entry(before, entry, expected_type="file"):
                raise ExternalRuntimeError("archive_member_identity_mismatch")

            def produce(output: BinaryIO) -> None:
                os.lseek(file_fd, 0, os.SEEK_SET)
                while True:
                    chunk = os.read(file_fd, 1024 * 1024)
                    if not chunk:
                        break
                    if output.write(chunk) != len(chunk):
                        raise ImmutableGenerationError("immutable_generation_partial_write")
                after = os.fstat(file_fd)
                visible = os.stat(leaf_name, dir_fd=parent_fd, follow_symlinks=False)
                if (
                    not _stat_matches_tree_entry(after, entry, expected_type="file")
                    or not _stat_matches_tree_entry(visible, entry, expected_type="file")
                ):
                    raise ExternalRuntimeError("archive_member_changed_during_use")

            required_lineage = {
                **dict(lineage),
                "schema_version": 1,
                "derivation": "verified_archive_member",
                "source_archive_sha256": self.receipt.source_sha256,
                "extraction_tree_sha256": self.receipt.tree_sha256,
                "archive_member_path": member.relative_path,
                "archive_member_sha256": member.sha256,
                "archive_member_device": member.device,
                "archive_member_inode": member.inode,
            }
            generation = publish_generated(
                generation_root,
                suffix=Path(member.relative_path).suffix,
                producer=produce,
                lineage=required_lineage,
                expected_sha256=member.sha256,
                expected_size=member.size,
            )
            if generation.sha256.lower() != member.sha256 or generation.size != member.size:
                raise ExternalRuntimeError("archive_member_publish_mismatch")
            return generation
        except ExternalRuntimeError:
            raise
        except (OSError, ImmutableGenerationError):
            raise ExternalRuntimeError("archive_member_publish_failed") from None
        finally:
            if file_fd >= 0:
                os.close(file_fd)
            if parent_fd >= 0:
                os.close(parent_fd)

    def verify(self) -> None:
        self._require_open()
        current = os.fstat(self._root_fd)
        visible = _stat_archive_sibling(self._parent, self._parent.output_name)
        marker = _read_extract_marker_fd(self._root_fd)
        if (
            _identity(current) != self._root_identity
            or _archive_object_state(current) != self._root_state
            or visible is None
            or _identity(visible) != self._root_identity
            or _snapshot_extracted_tree_fd(self._root_fd) != self._summary
            or marker
            != {
                "schema_version": 4,
                "source_device": self.receipt.source_device,
                "source_inode": self.receipt.source_inode,
                "source_size": self.receipt.source_size,
                "source_mtime_ns": self.receipt.source_mtime_ns,
                "source_ctime_ns": self.receipt.source_ctime_ns,
                "source_sha256": self.receipt.source_sha256,
                **self._summary.marker(),
                "publication_nonce": self.receipt.publication_nonce,
            }
        ):
            raise ExternalRuntimeError("archive_inventory_changed_during_use")
        self._parent.verify()

    def _require_open(self) -> None:
        if self._closed or self._root_fd < 0:
            raise ExternalRuntimeError("archive_extraction_lease_closed")

    def _close(self) -> None:
        self._closed = True
        self._root_fd = -1


def _member_receipt(entry: _ArchiveTreeEntry) -> ArchiveMemberReceiptV1:
    return ArchiveMemberReceiptV1(
        relative_path=entry.path,
        entry_type=entry.entry_type,
        size=entry.size,
        sha256=entry.sha256,
        device=entry.device,
        inode=entry.inode,
        uid=entry.uid,
        mode=entry.mode,
        links=entry.links,
        mtime_ns=entry.mtime_ns,
        ctime_ns=entry.ctime_ns,
        generation=entry.generation,
    )


def _tree_entry_from_member(member: ArchiveMemberReceiptV1) -> _ArchiveTreeEntry:
    return _ArchiveTreeEntry(
        path=member.relative_path,
        entry_type=member.entry_type,
        size=member.size,
        sha256=member.sha256,
        device=member.device,
        inode=member.inode,
        uid=member.uid,
        mode=member.mode,
        links=member.links,
        mtime_ns=member.mtime_ns,
        ctime_ns=member.ctime_ns,
        generation=member.generation,
    )


def _is_archive_runtime_entry(relative_path: str) -> bool:
    return relative_path == _ARCHIVE_RUNTIME_DIR or relative_path.startswith(
        f"{_ARCHIVE_RUNTIME_DIR}/"
    )


def _stat_matches_tree_entry(
    value: os.stat_result,
    entry: _ArchiveTreeEntry,
    *,
    expected_type: str,
) -> bool:
    type_matches = stat.S_ISREG(value.st_mode) if expected_type == "file" else stat.S_ISDIR(value.st_mode)
    return bool(
        type_matches
        and int(value.st_dev) == entry.device
        and int(value.st_ino) == entry.inode
        and int(value.st_uid) == entry.uid
        and stat.S_IMODE(value.st_mode) == entry.mode
        and int(value.st_nlink) == entry.links
        and (expected_type != "file" or int(value.st_size) == entry.size)
        and _stat_ns(value, "st_mtime_ns", "st_mtime") == entry.mtime_ns
        and _stat_ns(value, "st_ctime_ns", "st_ctime") == entry.ctime_ns
        and int(getattr(value, "st_gen", 0) or 0) == entry.generation
    )


def _open_archive_member_fd(
    root_fd: int,
    member: _ArchiveTreeEntry,
    entries: Mapping[str, _ArchiveTreeEntry],
) -> tuple[int, int, str]:
    parts = member.path.split("/")
    if not parts or any(part in {"", ".", ".."} for part in parts):
        raise ExternalRuntimeError("archive_member_receipt_invalid")
    directory_fd = _reopen_archive_directory_fd(root_fd)
    file_fd = -1
    prefix: list[str] = []
    directory_flags = (
        os.O_RDONLY
        | int(getattr(os, "O_CLOEXEC", 0))
        | int(getattr(os, "O_NOFOLLOW", 0))
        | int(getattr(os, "O_DIRECTORY", 0))
    )
    try:
        for part in parts[:-1]:
            prefix.append(part)
            expected = entries.get("/".join(prefix))
            if expected is None or expected.entry_type != "directory":
                raise ExternalRuntimeError("archive_member_receipt_invalid")
            child_fd = os.open(part, directory_flags, dir_fd=directory_fd)
            try:
                opened = os.fstat(child_fd)
                visible = os.stat(part, dir_fd=directory_fd, follow_symlinks=False)
                if (
                    not _stat_matches_tree_entry(opened, expected, expected_type="directory")
                    or not _stat_matches_tree_entry(visible, expected, expected_type="directory")
                ):
                    raise ExternalRuntimeError("archive_member_identity_mismatch")
            except BaseException:
                os.close(child_fd)
                raise
            os.close(directory_fd)
            directory_fd = child_fd
        leaf_name = parts[-1]
        file_fd = os.open(
            leaf_name,
            os.O_RDONLY | int(getattr(os, "O_CLOEXEC", 0)) | int(getattr(os, "O_NOFOLLOW", 0)),
            dir_fd=directory_fd,
        )
        opened = os.fstat(file_fd)
        visible = os.stat(leaf_name, dir_fd=directory_fd, follow_symlinks=False)
        if (
            not _stat_matches_tree_entry(opened, member, expected_type="file")
            or not _stat_matches_tree_entry(visible, member, expected_type="file")
        ):
            raise ExternalRuntimeError("archive_member_identity_mismatch")
        return directory_fd, file_fd, leaf_name
    except ExternalRuntimeError:
        if file_fd >= 0:
            os.close(file_fd)
        os.close(directory_fd)
        raise
    except OSError:
        if file_fd >= 0:
            os.close(file_fd)
        os.close(directory_fd)
        raise ExternalRuntimeError("archive_member_identity_mismatch") from None
    except BaseException:
        if file_fd >= 0:
            try:
                os.close(file_fd)
            except OSError:
                pass
        try:
            os.close(directory_fd)
        except OSError:
            pass
        raise


@contextmanager
def open_archive_extraction_v1(
    archive_path: Path,
    output_dir: Path,
    *,
    password: Optional[str],
    expected_sha256: str,
    reuse: bool = True,
) -> Iterator[ArchiveExtractionLeaseV1]:
    expected_digest = _normalized_sha256(expected_sha256)
    published = _extract_archive_generation(
        archive_path,
        output_dir,
        password=password,
        expected_sha256=expected_digest,
        reuse=reuse,
    )
    root_fd = published.root_fd
    lease: ArchiveExtractionLeaseV1 | None = None
    try:
        source_identity = _read_regular_archive_source(archive_path)
        if source_identity.sha256 != expected_digest or source_identity != published.source_identity:
            raise ExternalRuntimeError("archive_source_hash_mismatch")
        with _open_canonical_publish_parent(output_dir) as parent:
            with _archive_publication_lock(parent):
                _recover_archive_publication(parent)
                current = _stat_archive_sibling(parent, parent.output_name)
                if current is None or stat.S_ISLNK(current.st_mode) or not stat.S_ISDIR(current.st_mode):
                    raise ExternalRuntimeError("archive_cache_invalid")
                root_stat = os.fstat(root_fd)
                if (
                    _identity(root_stat) != _identity(current)
                    or _identity(root_stat) != published.root_identity
                    or _archive_object_state(root_stat) != published.root_state
                ):
                    raise ExternalRuntimeError("archive_cache_invalid")
                marker = _read_extract_marker_fd(root_fd)
                summary = _snapshot_extracted_tree_fd(root_fd)
                if (
                    summary != published.summary
                    or marker
                    != {
                        **published.source_identity.marker(),
                        **published.summary.marker(),
                        "publication_nonce": published.publication_nonce,
                    }
                ):
                    raise ExternalRuntimeError("archive_cache_invalid")
                public_summary = _public_archive_tree_summary(summary)
                receipt = ArchiveExtractionReceiptV1(
                    schema_version=1,
                    publication_nonce=published.publication_nonce,
                    source_sha256=source_identity.sha256,
                    source_device=source_identity.device,
                    source_inode=source_identity.inode,
                    source_size=source_identity.size,
                    source_mtime_ns=source_identity.mtime_ns,
                    source_ctime_ns=source_identity.ctime_ns,
                    root_device=int(root_stat.st_dev),
                    root_inode=int(root_stat.st_ino),
                    entry_count=public_summary.entry_count,
                    file_count=public_summary.file_count,
                    total_bytes=public_summary.total_bytes,
                    tree_sha256=public_summary.sha256,
                    members=tuple(
                        _member_receipt(entry)
                        for entry in public_summary.inventory
                    ),
                )
                lease = ArchiveExtractionLeaseV1(
                    parent=parent,
                    root_fd=root_fd,
                    root_identity=_identity(root_stat),
                    root_state=_archive_object_state(root_stat),
                    summary=summary,
                    receipt=receipt,
                )
            yield lease
            lease.verify()
    finally:
        if lease is not None:
            lease._close()
        os.close(root_fd)


def archive_entry_leaf_name(archive_path: str) -> str:
    identity = archive_path_identity(archive_path or "entry")
    return identity.split("::")[-1].split("/")[-1]


def archive_path_identity(value: str) -> str:
    identity = str(value or "")
    if not identity or identity != identity.strip():
        raise ExternalRuntimeError("archive_member_identity_invalid")
    segments = identity.split("::")
    if any(not segment for segment in segments):
        raise ExternalRuntimeError("archive_member_identity_invalid")
    for segment in segments:
        _validate_archive_member_identity_segment(segment)
    return identity


def _validate_archive_member_identity_segment(segment: str) -> None:
    if (
        not segment
        or segment != segment.strip()
        or "\x00" in segment
        or "\\" in segment
        or "::" in segment
        or segment.startswith("/")
        or any(ord(character) < 32 or ord(character) == 127 for character in segment)
    ):
        raise ExternalRuntimeError("archive_member_identity_invalid")
    parts = segment.split("/")
    if any(
        not part
        or part in {".", ".."}
        or part != part.strip()
        for part in parts
    ):
        raise ExternalRuntimeError("archive_member_identity_invalid")


def archive_cache_output_name(value: str) -> str:
    leaf_name = archive_entry_leaf_name(value)
    inner = Path(leaf_name)
    invalid_chars = set('<>:"/\\|?*')

    def safe_part(text: str, fallback: str) -> str:
        cleaned = "".join("_" if char in invalid_chars or ord(char) < 32 else char for char in str(text or ""))
        cleaned = cleaned.strip(" .")
        return cleaned or fallback

    stem = safe_part(inner.stem, "entry")
    suffix_text = "".join(
        "_" if char in invalid_chars or ord(char) < 32 else char
        for char in str(inner.suffix or "")
    )
    suffix = suffix_text if suffix_text.startswith(".") else f".{suffix_text.strip(' .')}" if suffix_text.strip(" .") else ""
    tag = _short_stable_tag(value or leaf_name)
    return f"{stem}_{tag}{suffix}" if suffix else f"{stem}_{tag}"


def archive_cache_dir_name(archive_path: Path, archive_sha256: str) -> str:
    invalid_chars = set('<>:"/\\|?*')
    safe_stem = "".join(
        "_" if char in invalid_chars or ord(char) < 32 else char
        for char in str(archive_path.stem or "archive")
    ).strip(" .") or "archive"
    digest = str(archive_sha256 or "").strip().lower()
    tag = digest.upper() if len(digest) == 64 and all(char in "0123456789abcdef" for char in digest) else _short_stable_tag(str(archive_path)).upper()
    return f"{safe_stem}_{tag}"


def combine_archive_path(parent: str, child: str) -> str:
    parent_text = str(parent or "")
    child_text = str(child or "")
    _validate_archive_member_identity_segment(child_text)
    if not parent_text:
        return child_text
    archive_path_identity(parent_text)
    return f"{parent_text}::{child_text}"


def archive_extractor_runtime() -> HostRuntimeResolution:
    _require_archive_execution_capability()
    return resolve_host_runtime_binary(
        ARCHIVE_EXTRACTOR_ENV,
        allowed_names=_ARCHIVE_EXTRACTOR_NAMES,
        require_archive_manifest=True,
    )


def _archive_execution_capability_health() -> ExecutionAuthorityHealth:
    platform_reason = _archive_platform_admission_reason()
    if platform_reason:
        return ExecutionAuthorityHealth(available=False, reason_code=platform_reason)
    return unadmitted_execution_capability_health(execution_authority_health())


def _archive_platform_admission_reason() -> str:
    required_dir_fd = (
        os.open,
        os.stat,
        os.mkdir,
    )
    if (
        os.name != "posix"
        or not hasattr(os, "O_NOFOLLOW")
        or not hasattr(os, "O_DIRECTORY")
        or any(function not in os.supports_dir_fd for function in required_dir_fd)
        or os.stat not in os.supports_follow_symlinks
        or not _archive_noreplace_rename_available()
        or not _archive_exchange_rename_available()
    ):
        return "archive_publish_platform_unavailable"
    try:
        import fcntl  # noqa: F401
    except ImportError:
        return "archive_publish_platform_unavailable"
    return ""


def _require_archive_execution_capability() -> None:
    platform_reason = _archive_platform_admission_reason()
    if platform_reason:
        raise ExternalRuntimeError(platform_reason)
    authority = _archive_execution_capability_health()
    if not authority.available:
        raise ExternalRuntimeError(authority.reason_code)


def archive_extractor_binaries() -> List[Path]:
    resolution = archive_extractor_runtime()
    return [resolution.path] if resolution.path is not None else []


def archive_extractor_health() -> Dict[str, object]:
    authority = _archive_execution_capability_health()
    if not authority.available:
        return {
            "archive_extraction_available": False,
            "archive_extraction_reason": authority.reason_code,
            "archive_extraction_bin": "",
            "archive_extraction_bin_source": "",
            "archive_extraction_supported_exts": [],
        }
    resolution = archive_extractor_runtime()
    if resolution.path is not None:
        return {
            "archive_extraction_available": True,
            "archive_extraction_reason": "",
            "archive_extraction_bin": str(resolution.path),
            "archive_extraction_bin_source": "host_resource_runtime",
            "archive_extraction_supported_exts": sorted(_extractor_supported_exts(resolution.path)),
        }
    return {
        "archive_extraction_available": False,
        "archive_extraction_reason": resolution.reason or "HOST_RUNTIME_UNAVAILABLE",
        "archive_extraction_bin": "",
        "archive_extraction_bin_source": "",
        "archive_extraction_supported_exts": [],
    }


def extract_archive(
    archive_path: Path,
    output_dir: Path,
    *,
    password: Optional[str],
    expected_sha256: str,
    reuse: bool = True,
) -> None:
    published = _extract_archive_generation(
        archive_path,
        output_dir,
        password=password,
        expected_sha256=expected_sha256,
        reuse=reuse,
    )
    os.close(published.root_fd)


def _extract_archive_generation(
    archive_path: Path,
    output_dir: Path,
    *,
    password: Optional[str],
    expected_sha256: str,
    reuse: bool = True,
) -> _PublishedArchiveGenerationV1:
    return _extract_archive_generation_serialized(
        archive_path,
        output_dir,
        password=password,
        expected_sha256=expected_sha256,
        reuse=reuse,
    )


def _extract_archive_generation_serialized(
    archive_path: Path,
    output_dir: Path,
    *,
    password: Optional[str],
    expected_sha256: str,
    reuse: bool = True,
) -> _PublishedArchiveGenerationV1:
    # Static capability admission precedes path inspection. Target-filesystem
    # semantics are then proven before source/password/helper inspection. A
    # libc symbol, package manifest, and global process adapter are not, by
    # themselves, authorization to expose case bytes to this helper class.
    _require_archive_execution_capability()
    expected_digest = _normalized_sha256(expected_sha256)
    _validate_archive_output_path(output_dir)
    _require_archive_target_filesystem_capability(output_dir)
    parent_preexisting = os.path.lexists(os.fspath(output_dir.parent))
    registered = (
        _assert_archive_publication_ready(output_dir)
        if parent_preexisting
        else None
    )
    reservation = (
        _reserve_archive_generation_path(output_dir)
        if registered is None
        else None
    )
    try:
        source_probe = _read_regular_archive_source(archive_path)
        if source_probe.sha256 != expected_digest:
            raise ExternalRuntimeError("archive_source_hash_mismatch")
        _validate_archive_source_budget(source_probe)
        # Disk state never authorizes reuse. A byte-for-byte source identity
        # may reuse only a live host registry capability that also revalidates
        # the pinned root, full inventory, marker, and publication nonce.
        _ = reuse
        if registered is not None:
            if registered.source_identity != source_probe:
                raise ExternalRuntimeError("archive_publish_indeterminate")
            transferred = registered
            registered = None
            return transferred

        password_text = str(password or "")
        if any(char in password_text for char in ("\x00", "\r", "\n")):
            raise ExternalRuntimeError("archive_password_invalid")

        runtime = archive_extractor_runtime()
        if runtime.path is None or not runtime.sha256:
            raise ExternalRuntimeError("archive_runtime_unavailable")
        extractor = runtime.path
        suffix = archive_path.suffix.lower()
        if suffix not in _extractor_supported_exts(extractor):
            raise ExternalRuntimeError("archive_format_unavailable")
        if password_text and (
            not _archive_extractor_uses_7z(extractor)
            or suffix not in _SEVEN_ZIP_PASSWORD_EXTS
        ):
            raise ExternalRuntimeError("archive_encrypted_unavailable")
        if reservation is None:
            raise ExternalRuntimeError("archive_generation_reservation_invalid")

        _extract_archive_into_new_generation(
            archive_path,
            output_dir,
            source_probe=source_probe,
            expected_digest=expected_digest,
            runtime=runtime,
            extractor=extractor,
            suffix=suffix,
            password_text=password_text,
            reservation=reservation,
        )
        published = _assert_archive_publication_ready(output_dir)
        if published is None:
            raise ExternalRuntimeError("archive_publish_indeterminate")
        return published
    finally:
        if registered is not None:
            os.close(registered.root_fd)
        _release_archive_generation_reservation(reservation)


def _archive_extractor_uses_7z(binary: Path) -> bool:
    name = binary.name.lower()
    return name in {"7z", "7z.exe", "7zz", "7zz.exe", "7za", "7za.exe"}


def _extractor_supported_exts(binary: Path) -> frozenset[str]:
    name = binary.name.lower()
    if _archive_extractor_uses_7z(binary):
        return _SEVEN_ZIP_EXTS
    if name == "bsdtar":
        return _BSDTAR_EXTS
    if name == "tar":
        return _TAR_EXTS
    return frozenset()


def _validate_extracted_tree(root: Path) -> int:
    try:
        root_stat = root.lstat()
        if stat.S_ISLNK(root_stat.st_mode) or not stat.S_ISDIR(root_stat.st_mode):
            raise OSError
        if shutil.disk_usage(root).free < _ARCHIVE_MIN_FREE_BYTES:
            raise ExternalRuntimeError("archive_output_quota_exceeded")
    except ExternalRuntimeError:
        raise
    except OSError:
        raise ExternalRuntimeError("archive_output_invalid") from None

    entry_count = 0
    file_count = 0
    total_bytes = 0

    def visit(directory: Path, depth: int) -> None:
        nonlocal entry_count, file_count, total_bytes
        if depth > _ARCHIVE_MAX_DEPTH:
            raise ExternalRuntimeError("archive_output_quota_exceeded")
        try:
            entries = os.scandir(directory)
        except OSError:
            raise ExternalRuntimeError("archive_output_invalid") from None
        with entries:
            for entry in entries:
                entry_count += 1
                if entry_count > _ARCHIVE_MAX_ENTRIES:
                    raise ExternalRuntimeError("archive_output_quota_exceeded")
                try:
                    relative = Path(entry.path).relative_to(root)
                    if len(os.fsencode(relative.as_posix())) > _ARCHIVE_MAX_PATH_BYTES:
                        raise ExternalRuntimeError("archive_output_quota_exceeded")
                    entry_stat = entry.stat(follow_symlinks=False)
                except ExternalRuntimeError:
                    raise
                except (OSError, ValueError):
                    raise ExternalRuntimeError("archive_output_invalid") from None
                if stat.S_ISLNK(entry_stat.st_mode):
                    raise ExternalRuntimeError("archive_output_invalid")
                path = Path(entry.path)
                if stat.S_ISDIR(entry_stat.st_mode):
                    visit(path, depth + 1)
                    continue
                if not stat.S_ISREG(entry_stat.st_mode) or int(entry_stat.st_nlink) != 1:
                    raise ExternalRuntimeError("archive_output_invalid")
                file_count += int(relative.as_posix() != _MARKER_FILE)
                file_size = int(entry_stat.st_size)
                if file_size > _ARCHIVE_MAX_FILE_BYTES:
                    raise ExternalRuntimeError("archive_output_quota_exceeded")
                total_bytes += file_size
                if total_bytes > _ARCHIVE_MAX_TOTAL_BYTES:
                    raise ExternalRuntimeError("archive_output_quota_exceeded")

    visit(root, 0)
    return file_count


def _validate_extracted_tree_fd_quota(root_fd: int) -> tuple[int, int]:
    """Validate live helper output through a pinned descriptor.

    This monitor intentionally does not treat helper-selected mode bits as an
    authority. The host hardens modes after the helper exits and before any
    bytes are copied into the public generation.
    """

    return _walk_archive_private_tree(
        root_fd,
        harden=False,
        max_file_bytes=_ARCHIVE_MAX_FILE_BYTES,
        max_total_bytes=_ARCHIVE_MAX_TOTAL_BYTES,
    )


def _harden_archive_private_tree(root_fd: int) -> None:
    _walk_archive_private_tree(
        root_fd,
        harden=True,
        max_file_bytes=_ARCHIVE_MAX_FILE_BYTES,
        max_total_bytes=_ARCHIVE_MAX_TOTAL_BYTES,
    )


def _walk_archive_private_tree(
    root_fd: int,
    *,
    harden: bool,
    max_file_bytes: int,
    max_total_bytes: int,
) -> tuple[int, int]:
    directory_flags = (
        os.O_RDONLY
        | int(getattr(os, "O_CLOEXEC", 0))
        | int(getattr(os, "O_NOFOLLOW", 0))
        | int(getattr(os, "O_DIRECTORY", 0))
    )
    file_flags = (
        os.O_RDONLY
        | int(getattr(os, "O_CLOEXEC", 0))
        | int(getattr(os, "O_NOFOLLOW", 0))
    )
    scan_root = -1
    entry_count = 0
    file_count = 0
    total_bytes = 0
    try:
        scan_root = _reopen_archive_directory_fd(root_fd)
        if harden:
            os.fchmod(scan_root, 0o700)

        def visit(directory_fd: int, prefix: str, depth: int) -> None:
            nonlocal entry_count, file_count, total_bytes
            if depth > _ARCHIVE_MAX_DEPTH:
                raise ExternalRuntimeError("archive_output_quota_exceeded")
            directory_before = os.fstat(directory_fd)
            entries = os.scandir(directory_fd)
            with entries:
                for entry in entries:
                    relative = f"{prefix}/{entry.name}" if prefix else entry.name
                    entry_count += 1
                    if (
                        entry_count > _ARCHIVE_MAX_ENTRIES
                        or len(os.fsencode(relative)) > _ARCHIVE_MAX_PATH_BYTES
                    ):
                        raise ExternalRuntimeError("archive_output_quota_exceeded")
                    before = entry.stat(follow_symlinks=False)
                    if stat.S_ISLNK(before.st_mode):
                        raise ExternalRuntimeError("archive_output_invalid")
                    if stat.S_ISDIR(before.st_mode):
                        child_fd = os.open(
                            entry.name,
                            directory_flags,
                            dir_fd=directory_fd,
                        )
                        try:
                            opened = os.fstat(child_fd)
                            visible = os.stat(
                                entry.name,
                                dir_fd=directory_fd,
                                follow_symlinks=False,
                            )
                            if (
                                not stat.S_ISDIR(opened.st_mode)
                                or _identity(before) != _identity(opened)
                                or _identity(opened) != _identity(visible)
                            ):
                                raise OSError
                            if harden:
                                os.fchmod(child_fd, 0o700)
                            visit(child_fd, relative, depth + 1)
                            after = os.fstat(child_fd)
                            visible_after = os.stat(
                                entry.name,
                                dir_fd=directory_fd,
                                follow_symlinks=False,
                            )
                            if _identity(after) != _identity(visible_after):
                                raise OSError
                        finally:
                            os.close(child_fd)
                        continue
                    if not stat.S_ISREG(before.st_mode) or int(before.st_nlink) != 1:
                        raise ExternalRuntimeError("archive_output_invalid")
                    file_fd = os.open(entry.name, file_flags, dir_fd=directory_fd)
                    try:
                        opened = os.fstat(file_fd)
                        visible = os.stat(
                            entry.name,
                            dir_fd=directory_fd,
                            follow_symlinks=False,
                        )
                        if (
                            not stat.S_ISREG(opened.st_mode)
                            or int(opened.st_nlink) != 1
                            or _identity(before) != _identity(opened)
                            or _identity(opened) != _identity(visible)
                        ):
                            raise OSError
                        if harden:
                            os.fchmod(file_fd, 0o600)
                        size = int(os.fstat(file_fd).st_size)
                        if size > max_file_bytes:
                            raise ExternalRuntimeError("archive_output_quota_exceeded")
                        total_bytes += size
                        if total_bytes > max_total_bytes:
                            raise ExternalRuntimeError("archive_output_quota_exceeded")
                        file_count += 1
                        after = os.fstat(file_fd)
                        visible_after = os.stat(
                            entry.name,
                            dir_fd=directory_fd,
                            follow_symlinks=False,
                        )
                        if _identity(after) != _identity(visible_after):
                            raise OSError
                    finally:
                        os.close(file_fd)
            directory_after = os.fstat(directory_fd)
            if _identity(directory_before) != _identity(directory_after):
                raise OSError

        visit(scan_root, "", 0)
        return file_count, total_bytes
    except ExternalRuntimeError:
        raise
    except OSError:
        raise ExternalRuntimeError("archive_output_invalid") from None
    finally:
        if scan_root >= 0:
            os.close(scan_root)


def _snapshot_extracted_tree(root: Path) -> _ArchiveTreeSummary:
    """Hash one exact, stable output generation without following path aliases.

    Directory enumeration is streamed and quota-checked before an entry is
    retained. The retained inventory is therefore bounded by
    ``_ARCHIVE_MAX_ENTRIES`` and is sorted only after the bounded scan so the
    root digest is deterministic across filesystems.
    """

    root_fd = -1
    try:
        root_before = root.lstat()
        if stat.S_ISLNK(root_before.st_mode) or not stat.S_ISDIR(root_before.st_mode):
            raise OSError
        root_fd = os.open(
            root,
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0))
            | int(getattr(os, "O_DIRECTORY", 0)),
        )
        root_opened = os.fstat(root_fd)
        if not stat.S_ISDIR(root_opened.st_mode) or _identity(root_before) != _identity(root_opened):
            raise OSError
        summary = _snapshot_extracted_tree_fd(root_fd)
        root_after = os.fstat(root_fd)
        root_path_after = root.lstat()
        if (
            _identity(root_opened) != _identity(root_after)
            or _identity(root_after) != _identity(root_path_after)
            or _mutable_identity(root_opened) != _mutable_identity(root_after)
        ):
            raise OSError
        return summary
    except ExternalRuntimeError:
        raise
    except (OSError, ValueError):
        raise ExternalRuntimeError("archive_output_invalid") from None
    finally:
        if root_fd >= 0:
            os.close(root_fd)


def _fsync_archive_tree_fd(root_fd: int) -> None:
    scan_root = -1
    visited = 0
    try:
        scan_root = _reopen_archive_directory_fd(root_fd)

        def visit(directory_fd: int, depth: int) -> None:
            nonlocal visited
            if depth > _ARCHIVE_MAX_DEPTH:
                raise ExternalRuntimeError("archive_output_quota_exceeded")
            directory_before = os.fstat(directory_fd)
            entries = os.scandir(directory_fd)
            with entries:
                for entry in entries:
                    visited += 1
                    if visited > _ARCHIVE_MAX_ENTRIES + 1:
                        raise ExternalRuntimeError("archive_output_quota_exceeded")
                    before = entry.stat(follow_symlinks=False)
                    if stat.S_ISLNK(before.st_mode):
                        raise ExternalRuntimeError("archive_output_invalid")
                    if stat.S_ISDIR(before.st_mode):
                        child_fd = os.open(
                            entry.name,
                            os.O_RDONLY
                            | int(getattr(os, "O_CLOEXEC", 0))
                            | int(getattr(os, "O_NOFOLLOW", 0))
                            | int(getattr(os, "O_DIRECTORY", 0)),
                            dir_fd=directory_fd,
                        )
                        try:
                            opened = os.fstat(child_fd)
                            visible = os.stat(
                                entry.name,
                                dir_fd=directory_fd,
                                follow_symlinks=False,
                            )
                            if (
                                not stat.S_ISDIR(opened.st_mode)
                                or _identity(before) != _identity(opened)
                                or _identity(opened) != _identity(visible)
                            ):
                                raise OSError
                            visit(child_fd, depth + 1)
                        finally:
                            os.close(child_fd)
                    elif stat.S_ISREG(before.st_mode):
                        file_fd = os.open(
                            entry.name,
                            os.O_RDONLY
                            | int(getattr(os, "O_CLOEXEC", 0))
                            | int(getattr(os, "O_NOFOLLOW", 0)),
                            dir_fd=directory_fd,
                        )
                        try:
                            opened = os.fstat(file_fd)
                            visible = os.stat(
                                entry.name,
                                dir_fd=directory_fd,
                                follow_symlinks=False,
                            )
                            if (
                                not stat.S_ISREG(opened.st_mode)
                                or int(opened.st_nlink) != 1
                                or _identity(before) != _identity(opened)
                                or _identity(opened) != _identity(visible)
                            ):
                                raise OSError
                            os.fsync(file_fd)
                            after = os.fstat(file_fd)
                            visible_after = os.stat(
                                entry.name,
                                dir_fd=directory_fd,
                                follow_symlinks=False,
                            )
                            if (
                                _identity(opened) != _identity(after)
                                or _identity(after) != _identity(visible_after)
                                or _mutable_identity(opened) != _mutable_identity(after)
                            ):
                                raise OSError
                        finally:
                            os.close(file_fd)
                    else:
                        raise ExternalRuntimeError("archive_output_invalid")
            os.fsync(directory_fd)
            directory_after = os.fstat(directory_fd)
            if _identity(directory_before) != _identity(directory_after):
                raise OSError

        visit(scan_root, 0)
    except ExternalRuntimeError:
        raise
    except OSError:
        raise ExternalRuntimeError("archive_publish_failed") from None
    finally:
        if scan_root >= 0:
            os.close(scan_root)


def _snapshot_extracted_tree_fd(root_fd: int) -> _ArchiveTreeSummary:
    inventory: list[_ArchiveTreeEntry] = []
    entry_count = 0
    file_count = 0
    total_bytes = 0

    try:
        root_before = os.fstat(root_fd)
        if not stat.S_ISDIR(root_before.st_mode):
            raise OSError
        filesystem = os.fstatvfs(root_fd)
        if int(filesystem.f_bavail) * int(filesystem.f_frsize) < _ARCHIVE_MIN_FREE_BYTES:
            raise ExternalRuntimeError("archive_output_quota_exceeded")

        def visit(directory_fd: int, prefix: str, depth: int) -> None:
            nonlocal entry_count, file_count, total_bytes
            if depth > _ARCHIVE_MAX_DEPTH:
                raise ExternalRuntimeError("archive_output_quota_exceeded")
            directory_before = os.fstat(directory_fd)
            try:
                entries = os.scandir(directory_fd)
            except OSError:
                raise ExternalRuntimeError("archive_output_invalid") from None
            with entries:
                for entry in entries:
                    relative = f"{prefix}/{entry.name}" if prefix else entry.name
                    if not prefix and entry.name == _MARKER_FILE:
                        continue
                    if any(part in _INTERNAL_EXTRACT_DIRS for part in relative.split("/")):
                        raise ExternalRuntimeError("archive_output_invalid")
                    entry_count += 1
                    if entry_count > _ARCHIVE_MAX_ENTRIES:
                        raise ExternalRuntimeError("archive_output_quota_exceeded")
                    relative = relative.replace(os.sep, "/")
                    if len(os.fsencode(relative)) > _ARCHIVE_MAX_PATH_BYTES:
                        raise ExternalRuntimeError("archive_output_quota_exceeded")
                    try:
                        entry_before = entry.stat(follow_symlinks=False)
                    except OSError:
                        raise ExternalRuntimeError("archive_output_invalid") from None
                    if stat.S_ISLNK(entry_before.st_mode):
                        raise ExternalRuntimeError("archive_output_invalid")

                    if stat.S_ISDIR(entry_before.st_mode):
                        child_fd = -1
                        try:
                            child_fd = os.open(
                                entry.name,
                                os.O_RDONLY
                                | int(getattr(os, "O_CLOEXEC", 0))
                                | int(getattr(os, "O_NOFOLLOW", 0))
                                | int(getattr(os, "O_DIRECTORY", 0)),
                                dir_fd=directory_fd,
                            )
                            child_opened = os.fstat(child_fd)
                            child_path = os.stat(entry.name, dir_fd=directory_fd, follow_symlinks=False)
                            if (
                                not stat.S_ISDIR(child_opened.st_mode)
                                or stat.S_ISLNK(child_path.st_mode)
                                or _identity(entry_before) != _identity(child_opened)
                                or _identity(child_opened) != _identity(child_path)
                                or (hasattr(os, "geteuid") and int(child_opened.st_uid) != int(os.geteuid()))
                                or (
                                    not _is_archive_runtime_entry(relative)
                                    and stat.S_IMODE(child_opened.st_mode) & 0o077
                                )
                            ):
                                raise OSError
                            visit(child_fd, relative, depth + 1)
                            child_after = os.fstat(child_fd)
                            child_path_after = os.stat(entry.name, dir_fd=directory_fd, follow_symlinks=False)
                            if (
                                _identity(child_opened) != _identity(child_after)
                                or _identity(child_after) != _identity(child_path_after)
                                or _mutable_identity(child_opened) != _mutable_identity(child_after)
                            ):
                                raise OSError
                            inventory.append(
                                _ArchiveTreeEntry(
                                    path=relative,
                                    entry_type="directory",
                                    size=0,
                                    sha256="",
                                    device=int(child_after.st_dev),
                                    inode=int(child_after.st_ino),
                                    uid=int(child_after.st_uid),
                                    mode=stat.S_IMODE(child_after.st_mode),
                                    links=int(child_after.st_nlink),
                                    mtime_ns=_stat_ns(child_after, "st_mtime_ns", "st_mtime"),
                                    ctime_ns=_stat_ns(child_after, "st_ctime_ns", "st_ctime"),
                                    generation=int(getattr(child_after, "st_gen", 0) or 0),
                                )
                            )
                        except ExternalRuntimeError:
                            raise
                        except OSError:
                            raise ExternalRuntimeError("archive_output_invalid") from None
                        finally:
                            if child_fd >= 0:
                                os.close(child_fd)
                        continue

                    if not stat.S_ISREG(entry_before.st_mode) or int(entry_before.st_nlink) != 1:
                        raise ExternalRuntimeError("archive_output_invalid")
                    file_fd = -1
                    try:
                        file_fd = os.open(
                            entry.name,
                            os.O_RDONLY
                            | int(getattr(os, "O_CLOEXEC", 0))
                            | int(getattr(os, "O_NOFOLLOW", 0)),
                            dir_fd=directory_fd,
                        )
                        file_opened = os.fstat(file_fd)
                        file_path = os.stat(entry.name, dir_fd=directory_fd, follow_symlinks=False)
                        if (
                            not stat.S_ISREG(file_opened.st_mode)
                            or int(file_opened.st_nlink) != 1
                            or _identity(entry_before) != _identity(file_opened)
                            or _identity(file_opened) != _identity(file_path)
                            or (hasattr(os, "geteuid") and int(file_opened.st_uid) != int(os.geteuid()))
                            or (
                                not _is_archive_runtime_entry(relative)
                                and stat.S_IMODE(file_opened.st_mode) & 0o077
                            )
                        ):
                            raise OSError
                        file_size = int(file_opened.st_size)
                        if file_size > _ARCHIVE_MAX_FILE_BYTES:
                            raise ExternalRuntimeError("archive_output_quota_exceeded")
                        total_bytes += file_size
                        if total_bytes > _ARCHIVE_MAX_TOTAL_BYTES:
                            raise ExternalRuntimeError("archive_output_quota_exceeded")
                        file_digest = hashlib.sha256()
                        while True:
                            chunk = os.read(file_fd, 1024 * 1024)
                            if not chunk:
                                break
                            file_digest.update(chunk)
                        file_after = os.fstat(file_fd)
                        file_path_after = os.stat(entry.name, dir_fd=directory_fd, follow_symlinks=False)
                        if (
                            _identity(file_opened) != _identity(file_after)
                            or _identity(file_after) != _identity(file_path_after)
                            or _mutable_identity(file_opened) != _mutable_identity(file_after)
                        ):
                            raise OSError
                        file_count += 1
                        inventory.append(
                            _ArchiveTreeEntry(
                                path=relative,
                                entry_type="file",
                                size=file_size,
                                sha256=file_digest.hexdigest(),
                                device=int(file_after.st_dev),
                                inode=int(file_after.st_ino),
                                uid=int(file_after.st_uid),
                                mode=stat.S_IMODE(file_after.st_mode),
                                links=int(file_after.st_nlink),
                                mtime_ns=_stat_ns(file_after, "st_mtime_ns", "st_mtime"),
                                ctime_ns=_stat_ns(file_after, "st_ctime_ns", "st_ctime"),
                                generation=int(getattr(file_after, "st_gen", 0) or 0),
                            )
                        )
                    except ExternalRuntimeError:
                        raise
                    except OSError:
                        raise ExternalRuntimeError("archive_output_invalid") from None
                    finally:
                        if file_fd >= 0:
                            os.close(file_fd)
            directory_after = os.fstat(directory_fd)
            if (
                _identity(directory_before) != _identity(directory_after)
                or _mutable_identity(directory_before) != _mutable_identity(directory_after)
            ):
                raise ExternalRuntimeError("archive_output_invalid")

        visit(root_fd, "", 0)
        root_after = os.fstat(root_fd)
        if (
            _identity(root_before) != _identity(root_after)
            or _mutable_identity(root_before) != _mutable_identity(root_after)
        ):
            raise OSError
    except ExternalRuntimeError:
        raise
    except (OSError, ValueError):
        raise ExternalRuntimeError("archive_output_invalid") from None

    ordered = tuple(sorted(inventory, key=lambda item: os.fsencode(item.path)))
    digest = hashlib.sha256(b"analytix-archive-output-tree-v1\x00")
    for entry in ordered:
        record = entry.content_record()
        digest.update(len(record).to_bytes(8, "big"))
        digest.update(record)
    return _ArchiveTreeSummary(
        entry_count=entry_count,
        file_count=file_count,
        total_bytes=total_bytes,
        sha256=digest.hexdigest(),
        inventory=ordered,
    )


def _tree_record(relative: str, entry_type: str, size: int, sha256: str) -> bytes:
    return json.dumps(
        {
            "path": relative.replace(os.sep, "/"),
            "type": entry_type,
            "size": int(size),
            "sha256": sha256,
        },
        ensure_ascii=True,
        sort_keys=True,
        separators=(",", ":"),
    ).encode("utf-8")


def _safe_archive_sibling_name(name: str) -> str:
    value = str(name or "")
    if not value or "\x00" in value or Path(value).name != value or value in {".", ".."}:
        raise ExternalRuntimeError("archive_publish_indeterminate")
    return value


def _stat_archive_sibling(
    parent: _PinnedArchivePublishParent,
    name: str,
) -> os.stat_result | None:
    try:
        return os.stat(
            _safe_archive_sibling_name(name),
            dir_fd=parent.descriptor,
            follow_symlinks=False,
        )
    except FileNotFoundError:
        return None
    except OSError:
        raise ExternalRuntimeError("archive_publish_failed") from None


def _sync_archive_parent(
    parent: _PinnedArchivePublishParent,
    phase: str,
    *,
    verify_binding: bool = True,
) -> None:
    if not phase:
        raise ExternalRuntimeError("archive_publish_indeterminate")
    try:
        os.fsync(parent.descriptor)
    except OSError:
        raise ExternalRuntimeError("archive_publish_failed") from None
    if verify_binding:
        parent.verify()


def _sync_archive_directory_fd(descriptor: int, phase: str) -> None:
    if descriptor < 0 or not phase:
        raise ExternalRuntimeError("archive_publish_indeterminate")
    try:
        opened = os.fstat(descriptor)
        if not stat.S_ISDIR(opened.st_mode):
            raise OSError
        os.fsync(descriptor)
        after = os.fstat(descriptor)
        if _identity(opened) != _identity(after):
            raise OSError
    except OSError:
        raise ExternalRuntimeError("archive_publish_failed") from None


def _archive_noreplace_rename_available() -> bool:
    if os.name != "posix":
        return False
    try:
        libc = ctypes.CDLL(None, use_errno=True)
    except OSError:
        return False
    if sys.platform == "darwin":
        return hasattr(libc, "renameatx_np")
    if sys.platform.startswith("linux"):
        return hasattr(libc, "renameat2")
    return False


def _archive_exchange_rename_available() -> bool:
    # Both supported kernels expose no-replace and exchange through the same
    # descriptor-relative entry point, but each flag still receives a real
    # target-filesystem semantic probe before any case source is read.
    return _archive_noreplace_rename_available()


def _rename_archive_noreplace(
    source_directory_fd: int,
    source_name: str,
    target_directory_fd: int,
    target_name: str,
) -> None:
    source = os.fsencode(_safe_archive_sibling_name(source_name))
    target = os.fsencode(_safe_archive_sibling_name(target_name))
    libc = ctypes.CDLL(None, use_errno=True)
    ctypes.set_errno(0)
    if sys.platform == "darwin" and hasattr(libc, "renameatx_np"):
        rename = libc.renameatx_np
        rename.argtypes = [
            ctypes.c_int,
            ctypes.c_char_p,
            ctypes.c_int,
            ctypes.c_char_p,
            ctypes.c_uint,
        ]
        rename.restype = ctypes.c_int
        # RENAME_EXCL prevents replacement; RENAME_NOFOLLOW_ANY rejects any
        # symlink encountered by the kernel while resolving either leaf.
        result = rename(
            source_directory_fd,
            source,
            target_directory_fd,
            target,
            0x00000004 | 0x00000010,
        )
    elif sys.platform.startswith("linux") and hasattr(libc, "renameat2"):
        rename = libc.renameat2
        rename.argtypes = [
            ctypes.c_int,
            ctypes.c_char_p,
            ctypes.c_int,
            ctypes.c_char_p,
            ctypes.c_uint,
        ]
        rename.restype = ctypes.c_int
        result = rename(
            source_directory_fd,
            source,
            target_directory_fd,
            target,
            0x00000001,
        )
    else:
        raise ExternalRuntimeError("archive_publish_platform_unavailable")
    if result == 0:
        return
    error_number = ctypes.get_errno() or errno.EIO
    if error_number == errno.EEXIST:
        raise FileExistsError(error_number, os.strerror(error_number), os.fsdecode(target))
    if error_number in {
        errno.EINVAL,
        errno.ENOSYS,
        errno.ENOTSUP,
        errno.EXDEV,
        getattr(errno, "EOPNOTSUPP", errno.ENOTSUP),
    }:
        raise ExternalRuntimeError("archive_publish_platform_unavailable")
    raise OSError(error_number, os.strerror(error_number))


def _rename_archive_exchange(
    first_directory_fd: int,
    first_name: str,
    second_directory_fd: int,
    second_name: str,
) -> None:
    first = os.fsencode(_safe_archive_sibling_name(first_name))
    second = os.fsencode(_safe_archive_sibling_name(second_name))
    libc = ctypes.CDLL(None, use_errno=True)
    ctypes.set_errno(0)
    if sys.platform == "darwin" and hasattr(libc, "renameatx_np"):
        rename = libc.renameatx_np
        rename.argtypes = [
            ctypes.c_int,
            ctypes.c_char_p,
            ctypes.c_int,
            ctypes.c_char_p,
            ctypes.c_uint,
        ]
        rename.restype = ctypes.c_int
        # RENAME_SWAP is the atomic publication/rollback primitive;
        # RENAME_NOFOLLOW_ANY rejects symlink traversal in either resolution.
        result = rename(
            first_directory_fd,
            first,
            second_directory_fd,
            second,
            0x00000002 | 0x00000010,
        )
    elif sys.platform.startswith("linux") and hasattr(libc, "renameat2"):
        rename = libc.renameat2
        rename.argtypes = [
            ctypes.c_int,
            ctypes.c_char_p,
            ctypes.c_int,
            ctypes.c_char_p,
            ctypes.c_uint,
        ]
        rename.restype = ctypes.c_int
        result = rename(
            first_directory_fd,
            first,
            second_directory_fd,
            second,
            0x00000002,
        )
    else:
        raise ExternalRuntimeError("archive_publish_platform_unavailable")
    if result == 0:
        return
    error_number = ctypes.get_errno() or errno.EIO
    if error_number in {
        errno.EINVAL,
        errno.ENOSYS,
        errno.ENOTSUP,
        errno.EXDEV,
        getattr(errno, "EOPNOTSUPP", errno.ENOTSUP),
    }:
        raise ExternalRuntimeError("archive_publish_platform_unavailable")
    raise OSError(error_number, os.strerror(error_number))


def _archive_entry_identity(
    directory_fd: int,
    name: str,
) -> tuple[int, int] | None:
    try:
        opened = os.stat(name, dir_fd=directory_fd, follow_symlinks=False)
    except FileNotFoundError:
        return None
    if stat.S_ISLNK(opened.st_mode) or not stat.S_ISDIR(opened.st_mode):
        raise ExternalRuntimeError("archive_publish_indeterminate")
    return _identity(opened)


def _remove_exact_empty_archive_directory(
    directory_fd: int,
    name: str,
    *,
    expected_identity: tuple[int, int],
) -> None:
    if _archive_entry_identity(directory_fd, name) != expected_identity:
        raise ExternalRuntimeError("archive_publish_indeterminate")
    try:
        os.rmdir(_safe_archive_sibling_name(name), dir_fd=directory_fd)
    except OSError:
        raise ExternalRuntimeError("archive_publish_indeterminate") from None


def _probe_archive_publish_semantics(
    parent: _PinnedArchivePublishParent,
) -> None:
    """Prove no-replace and exchange on the actual destination filesystem."""

    token = secrets.token_hex(16)
    first_name = f".analytix-archive-admission-{token}-a"
    second_name = f".analytix-archive-admission-{token}-b"
    moved_name = f".analytix-archive-admission-{token}-c"
    created: dict[str, tuple[int, int]] = {}
    owned_identities: set[tuple[int, int]] = set()
    try:
        for name in (first_name, second_name):
            os.mkdir(name, mode=0o700, dir_fd=parent.descriptor)
            identity = _archive_entry_identity(parent.descriptor, name)
            if identity is None:
                raise OSError
            created[name] = identity
            owned_identities.add(identity)
        _rename_archive_noreplace(
            parent.descriptor,
            first_name,
            parent.descriptor,
            moved_name,
        )
        created[moved_name] = created.pop(first_name)
        collision_rejected = False
        try:
            _rename_archive_noreplace(
                parent.descriptor,
                moved_name,
                parent.descriptor,
                second_name,
            )
        except FileExistsError:
            collision_rejected = True
        if (
            not collision_rejected
            or _archive_entry_identity(parent.descriptor, moved_name)
            != created[moved_name]
            or _archive_entry_identity(parent.descriptor, second_name)
            != created[second_name]
        ):
            raise OSError
        _rename_archive_exchange(
            parent.descriptor,
            moved_name,
            parent.descriptor,
            second_name,
        )
        if (
            _archive_entry_identity(parent.descriptor, moved_name)
            != created[second_name]
            or _archive_entry_identity(parent.descriptor, second_name)
            != created[moved_name]
        ):
            raise OSError
        _rename_archive_exchange(
            parent.descriptor,
            moved_name,
            parent.descriptor,
            second_name,
        )
        if (
            _archive_entry_identity(parent.descriptor, moved_name)
            != created[moved_name]
            or _archive_entry_identity(parent.descriptor, second_name)
            != created[second_name]
        ):
            raise OSError
        for name in (moved_name, second_name):
            _remove_exact_empty_archive_directory(
                parent.descriptor,
                name,
                expected_identity=created[name],
            )
            created.pop(name)
        _sync_archive_parent(parent, "archive_publish_semantic_admission")
    except BaseException as error:
        # Cleanup is identity-bound and never removes an attacker replacement.
        for name in (first_name, second_name, moved_name):
            try:
                expected_identity = _archive_entry_identity(
                    parent.descriptor,
                    name,
                )
                if expected_identity not in owned_identities:
                    continue
                _remove_exact_empty_archive_directory(
                    parent.descriptor,
                    name,
                    expected_identity=expected_identity,
                )
            except ExternalRuntimeError:
                pass
        if not isinstance(error, Exception):
            raise
        raise ExternalRuntimeError("archive_publish_platform_unavailable") from None


def _require_archive_target_filesystem_capability(output_dir: Path) -> None:
    with _open_canonical_publish_parent(output_dir) as parent:
        with _archive_publication_lock(parent):
            _probe_archive_publish_semantics(parent)


def _create_archive_publish_placeholder(
    source_directory_fd: int,
) -> tuple[int, str, tuple[int, int]]:
    name = f".analytix-archive-placeholder-{secrets.token_hex(16)}"
    descriptor = -1
    created = False
    try:
        os.mkdir(name, mode=0o700, dir_fd=source_directory_fd)
        created = True
        descriptor = os.open(
            name,
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0))
            | int(getattr(os, "O_DIRECTORY", 0)),
            dir_fd=source_directory_fd,
        )
        opened = os.fstat(descriptor)
        if (
            not stat.S_ISDIR(opened.st_mode)
            or _archive_entry_identity(source_directory_fd, name) != _identity(opened)
        ):
            raise OSError
        with os.scandir(descriptor) as entries:
            if next(entries, None) is not None:
                raise OSError
        # The extractor has exited before this object exists. Keep owner-only
        # search permission because Darwin's RENAME_NOFOLLOW_ANY rejects a
        # source directory leaf whose mode is 000 even when its parent FD is
        # pinned. Namespace authority still comes from the held inode/FD and
        # atomic exchange, not from this mode bit.
        os.fsync(descriptor)
        _sync_archive_directory_fd(
            source_directory_fd,
            "output_generation_placeholder_created",
        )
        return descriptor, name, _identity(os.fstat(descriptor))
    except BaseException:
        if descriptor >= 0:
            try:
                os.close(descriptor)
            except OSError:
                pass
        if created:
            try:
                os.rmdir(name, dir_fd=source_directory_fd)
            except OSError:
                pass
        raise


def _rollback_visible_archive_generation(
    parent: _PinnedArchivePublishParent,
    *,
    expected_identity: tuple[int, int],
    placeholder_identity: tuple[int, int],
    source_directory_fd: int,
) -> None:
    current = _archive_entry_identity(parent.descriptor, parent.output_name)
    hidden = _archive_entry_identity(source_directory_fd, "out")
    if current != expected_identity or hidden is None:
        raise ExternalRuntimeError("archive_publish_indeterminate")
    try:
        # The hidden slot is never absent: it is the exact placeholder that
        # was installed before publication. Exchange therefore cannot be
        # defeated by recreating the old rollback target after visibility.
        _rename_archive_exchange(
            parent.descriptor,
            parent.output_name,
            source_directory_fd,
            "out",
        )
    except (OSError, ExternalRuntimeError):
        raise ExternalRuntimeError("archive_publish_indeterminate") from None
    visible_after = _archive_entry_identity(parent.descriptor, parent.output_name)
    hidden_after = _archive_entry_identity(source_directory_fd, "out")
    if visible_after != placeholder_identity or hidden_after != expected_identity:
        raise ExternalRuntimeError("archive_publish_indeterminate")
    _remove_exact_empty_archive_directory(
        parent.descriptor,
        parent.output_name,
        expected_identity=placeholder_identity,
    )
    try:
        _sync_archive_directory_fd(
            source_directory_fd,
            "output_generation_source_rolled_back",
        )
        _sync_archive_parent(parent, "output_generation_rolled_back")
    except ExternalRuntimeError:
        # The generated facts are hidden again, but a failed durability
        # barrier remains an indeterminate restart state and is never retried.
        raise ExternalRuntimeError("archive_publish_indeterminate") from None


def _settle_failed_archive_publication(
    parent: _PinnedArchivePublishParent,
    *,
    published: _PublishedArchiveGenerationV1,
    placeholder_fd: int,
    placeholder_source_name: str,
    source_directory_fd: int,
) -> bool:
    """Return True only when the exact generation is already host-accepted."""

    try:
        if not _published_archive_generation_fd_matches(published):
            raise ExternalRuntimeError("archive_publish_indeterminate")
        expected_identity = published.root_identity
        placeholder_identity = _identity(os.fstat(placeholder_fd))
        visible = _stat_archive_sibling(parent, parent.output_name)
        visible_identity = _identity(visible) if visible is not None else None
        hidden_identity = _archive_entry_identity(source_directory_fd, "out")
        source_placeholder_identity = _archive_entry_identity(
            source_directory_fd,
            placeholder_source_name,
        )
        if _archive_generation_registry_accepts_identity(
            parent,
            expected_identity,
        ):
            if visible_identity != expected_identity:
                raise ExternalRuntimeError("archive_publish_indeterminate")
            # Registry acceptance is the commit point. Placeholder cleanup is
            # non-factual residue cleanup and cannot revoke an accepted fact
            # generation or convert a signal into a false failed result.
            if hidden_identity == placeholder_identity:
                try:
                    _remove_exact_empty_archive_directory(
                        source_directory_fd,
                        "out",
                        expected_identity=placeholder_identity,
                    )
                    _sync_archive_directory_fd(
                        source_directory_fd,
                        "output_generation_placeholder_removed",
                    )
                except ExternalRuntimeError:
                    pass
            return True
        if visible_identity == expected_identity:
            if hidden_identity is None:
                raise ExternalRuntimeError("archive_publish_indeterminate")
            _rollback_visible_archive_generation(
                parent,
                expected_identity=expected_identity,
                placeholder_identity=placeholder_identity,
                source_directory_fd=source_directory_fd,
            )
            if not _published_archive_generation_fd_matches(
                published,
                require_root_state=False,
            ):
                raise ExternalRuntimeError("archive_publish_indeterminate")
            return False
        if visible_identity == placeholder_identity and hidden_identity == expected_identity:
            _remove_exact_empty_archive_directory(
                parent.descriptor,
                parent.output_name,
                expected_identity=placeholder_identity,
            )
            _sync_archive_directory_fd(
                source_directory_fd,
                "output_generation_source_rolled_back",
            )
            _sync_archive_parent(parent, "output_generation_rolled_back")
            if not _published_archive_generation_fd_matches(
                published,
                require_root_state=False,
            ):
                raise ExternalRuntimeError("archive_publish_indeterminate")
            return False
        if visible_identity is not None or hidden_identity != expected_identity:
            raise ExternalRuntimeError("archive_publish_indeterminate")
        if source_placeholder_identity == placeholder_identity:
            _remove_exact_empty_archive_directory(
                source_directory_fd,
                placeholder_source_name,
                expected_identity=placeholder_identity,
            )
            _sync_archive_directory_fd(
                source_directory_fd,
                "output_generation_unused_placeholder_removed",
            )
        elif source_placeholder_identity is not None:
            raise ExternalRuntimeError("archive_publish_indeterminate")
        parent.verify()
        return False
    except ExternalRuntimeError:
        raise
    except OSError:
        raise ExternalRuntimeError("archive_publish_indeterminate") from None


def _create_archive_attempt_directory(
    parent: _PinnedArchivePublishParent,
    *,
    name: str,
) -> tuple[int, os.stat_result]:
    directory_flags = (
        os.O_RDONLY
        | int(getattr(os, "O_CLOEXEC", 0))
        | int(getattr(os, "O_NOFOLLOW", 0))
        | int(getattr(os, "O_DIRECTORY", 0))
    )
    descriptor = -1
    directory_name = _safe_archive_sibling_name(name)
    if not directory_name.startswith(".analytix-archive-attempt-"):
        raise ExternalRuntimeError("archive_publish_indeterminate")
    try:
        # Only the unique private attempt name is created with mkdir. The
        # logical output name appears later in one kernel no-replace rename.
        os.mkdir(directory_name, mode=0o700, dir_fd=parent.descriptor)
        descriptor = os.open(directory_name, directory_flags, dir_fd=parent.descriptor)
        opened = os.fstat(descriptor)
        visible = os.stat(directory_name, dir_fd=parent.descriptor, follow_symlinks=False)
        if (
            not stat.S_ISDIR(opened.st_mode)
            or stat.S_ISLNK(visible.st_mode)
            or _identity(opened) != _identity(visible)
        ):
            raise OSError
        _sync_archive_parent(parent, "attempt_directory_created")
        return descriptor, opened
    except FileExistsError:
        raise ExternalRuntimeError("archive_publish_indeterminate") from None
    except (OSError, ExternalRuntimeError):
        # Once mkdir succeeds, path-selected cleanup could delete an unknown
        # object swapped into this name. Preserve the blocking residue and
        # require host-authorized reconciliation instead.
        if descriptor >= 0:
            os.close(descriptor)
        raise ExternalRuntimeError("archive_publish_failed") from None
    except BaseException:
        if descriptor >= 0:
            try:
                os.close(descriptor)
            except OSError:
                pass
        raise


def _assert_archive_publication_ready(
    output_dir: Path,
) -> _PublishedArchiveGenerationV1 | None:
    with _open_canonical_publish_parent(output_dir) as parent:
        with _archive_publication_lock(parent):
            _recover_archive_publication(parent)
            output_stat = _stat_archive_sibling(parent, parent.output_name)
            if output_stat is None:
                if (
                    _archive_generation_registry_contains(parent)
                    or _archive_logical_path_accepted(output_dir)
                ):
                    raise ExternalRuntimeError("archive_publish_indeterminate")
                return None
            if output_stat is not None and (
                stat.S_ISLNK(output_stat.st_mode) or not stat.S_ISDIR(output_stat.st_mode)
            ):
                raise ExternalRuntimeError("archive_output_untrusted")
            published = _open_registered_archive_generation(
                parent,
                output_stat,
            )
            if published is None:
                raise ExternalRuntimeError("archive_publish_indeterminate")
            return published


def _extract_archive_into_new_generation(
    archive_path: Path,
    output_dir: Path,
    *,
    source_probe: _ArchiveSourceIdentity,
    expected_digest: str,
    runtime: HostRuntimeResolution,
    extractor: Path,
    suffix: str,
    password_text: str,
    reservation: _ArchiveGenerationReservationV1,
) -> None:
    attempt_fd = -1
    runtime_fd = -1
    staged_output_fd = -1
    placeholder_fd = -1
    placeholder_name = ""
    placeholder_identity: tuple[int, int] | None = None
    output_identity: tuple[int, int] | None = None
    publish_attempted = False
    authority: _PublishedArchiveGenerationV1 | None = None
    try:
        with _open_canonical_publish_parent(output_dir) as parent:
            with _archive_publication_lock(parent):
                _recover_archive_publication(parent)
                _bind_archive_generation_reservation(parent, reservation)
                _assert_archive_generation_reservation(parent, reservation)
                output_stat = _stat_archive_sibling(parent, parent.output_name)
                if output_stat is not None:
                    if stat.S_ISLNK(output_stat.st_mode) or not stat.S_ISDIR(output_stat.st_mode):
                        raise ExternalRuntimeError("archive_output_untrusted")
                    registered = _open_registered_archive_generation(
                        parent,
                        output_stat,
                        expected_source=source_probe,
                    )
                    if registered is not None:
                        os.close(registered.root_fd)
                        return
                    raise ExternalRuntimeError("archive_publish_indeterminate")
                if _archive_generation_registry_contains(parent):
                    # A live authority for this logical output may have been
                    # renamed away. Never reissue or displace that authority.
                    raise ExternalRuntimeError("archive_publish_indeterminate")

                attempt_name = _archive_attempt_name(parent, reservation)
                _validate_archive_retained_attempt_budget(
                    parent,
                    anticipated_source_bytes=source_probe.size,
                )
                attempt_fd, _attempt_stat = _create_archive_attempt_directory(
                    parent,
                    name=attempt_name,
                )
                try:
                    (
                        runtime_fd,
                        staged_output_fd,
                        opaque_source_name,
                        source_identity,
                        staged_source_identity,
                    ) = _create_archive_runtime_workspace(
                        archive_path,
                        attempt_fd,
                        suffix=suffix,
                    )
                    if (
                        source_identity.sha256 != expected_digest
                        or source_identity != source_probe
                    ):
                        raise ExternalRuntimeError("archive_source_hash_mismatch")

                    runtime_dir = (
                        parent.path
                        / attempt_name
                        / _ARCHIVE_RUNTIME_DIR
                    )
                    if _archive_extractor_uses_7z(extractor):
                        command = [
                            str(extractor),
                            "x",
                            "-y",
                            "-oout",
                            opaque_source_name,
                        ]
                        if password_text:
                            command[2:2] = ["-p", "-sccUTF-8"]
                    else:
                        command = [
                            str(extractor),
                            "-xf",
                            opaque_source_name,
                            "-C",
                            "out",
                        ]
                    runtime_error = ""
                    try:
                        completed = run_managed_process(
                            command,
                            cwd=runtime_dir,
                            cwd_fd=runtime_fd,
                            expected_cwd_device=int(os.fstat(runtime_fd).st_dev),
                            expected_cwd_inode=int(os.fstat(runtime_fd).st_ino),
                            input_bytes=(password_text + "\n").encode("utf-8")
                            if password_text
                            else None,
                            timeout_seconds=_ARCHIVE_TIMEOUT_SECONDS,
                            expected_executable_sha256=runtime.sha256,
                            resource_monitor=lambda: _validate_extracted_tree_fd_quota(
                                staged_output_fd
                            ),
                        )
                    except ManagedProcessTimeout:
                        runtime_error = "archive_extract_timeout"
                    except ManagedProcessTerminationError:
                        runtime_error = "archive_process_tree_termination_failed"
                    except ManagedProcessLaunchError:
                        runtime_error = "archive_runtime_launch_failed"
                    if runtime_error:
                        raise ExternalRuntimeError(runtime_error)
                    if completed.returncode != 0:
                        code = (
                            "archive_password_or_format_invalid"
                            if password_text
                            else "archive_extract_failed"
                        )
                        raise ExternalRuntimeError(code)

                    _harden_archive_private_tree(staged_output_fd)

                    source_after = _read_regular_archive_source(archive_path)
                    staged_source_after = _read_regular_archive_source_at(
                        runtime_fd,
                        opaque_source_name,
                    )
                    runtime_visible = os.stat(
                        _ARCHIVE_RUNTIME_DIR,
                        dir_fd=attempt_fd,
                        follow_symlinks=False,
                    )
                    staged_visible = os.stat(
                        "out",
                        dir_fd=runtime_fd,
                        follow_symlinks=False,
                    )
                    if (
                        source_after != source_probe
                        or staged_source_after != staged_source_identity
                        or _identity(runtime_visible)
                        != _identity(os.fstat(runtime_fd))
                        or _identity(staged_visible)
                        != _identity(os.fstat(staged_output_fd))
                    ):
                        raise ExternalRuntimeError("archive_source_hash_mismatch")

                    tree_summary = _snapshot_extracted_tree_fd(staged_output_fd)
                    publication_nonce = secrets.token_hex(32)
                    _write_extract_marker_fd(
                        staged_output_fd,
                        publication_nonce=publication_nonce,
                        source_identity=source_identity,
                        tree_summary=tree_summary,
                    )
                    _fsync_archive_tree_fd(staged_output_fd)
                    if (
                        _snapshot_extracted_tree_fd(staged_output_fd) != tree_summary
                        or _read_extract_marker_fd(staged_output_fd)
                        != {
                            **source_identity.marker(),
                            **tree_summary.marker(),
                            "publication_nonce": publication_nonce,
                        }
                    ):
                        raise ExternalRuntimeError("archive_output_invalid")
                    output_stat = os.fstat(staged_output_fd)
                    output_identity = _identity(output_stat)
                    authority = _PublishedArchiveGenerationV1(
                        publication_nonce=publication_nonce,
                        source_identity=source_identity,
                        root_identity=output_identity,
                        # Rename/exchange may update directory metadata. Empty
                        # state authorizes rollback validation only; registry
                        # commit receives the post-exchange frozen state below.
                        root_state=(),
                        summary=tree_summary,
                        root_fd=staged_output_fd,
                    )
                    (
                        placeholder_fd,
                        placeholder_name,
                        placeholder_identity,
                    ) = _create_archive_publish_placeholder(runtime_fd)
                    publish_attempted = True
                    try:
                        _rename_archive_noreplace(
                            runtime_fd,
                            placeholder_name,
                            parent.descriptor,
                            parent.output_name,
                        )
                    except FileExistsError:
                        raise ExternalRuntimeError("archive_publish_indeterminate") from None
                    except OSError:
                        raise ExternalRuntimeError("archive_publish_failed") from None
                    _sync_archive_directory_fd(
                        runtime_fd,
                        "output_generation_placeholder_reserved",
                    )
                    _sync_archive_parent(parent, "output_generation_placeholder_visible")
                    try:
                        _rename_archive_exchange(
                            runtime_fd,
                            "out",
                            parent.descriptor,
                            parent.output_name,
                        )
                    except OSError:
                        raise ExternalRuntimeError("archive_publish_failed") from None
                    opened_after_exchange = os.fstat(staged_output_fd)
                    authority = _PublishedArchiveGenerationV1(
                        publication_nonce=publication_nonce,
                        source_identity=source_identity,
                        root_identity=output_identity,
                        root_state=_archive_object_state(opened_after_exchange),
                        summary=tree_summary,
                        root_fd=staged_output_fd,
                    )
                    _sync_archive_directory_fd(
                        runtime_fd,
                        "output_generation_source_published",
                    )
                    _sync_archive_parent(parent, "output_generation_published")
                    opened_after = os.fstat(staged_output_fd)
                    visible_after = _stat_archive_sibling(
                        parent,
                        parent.output_name,
                    )
                    if (
                        visible_after is None
                        or _identity(opened_after) != _identity(output_stat)
                        or _identity(visible_after) != _identity(output_stat)
                        or _snapshot_extracted_tree_fd(staged_output_fd) != tree_summary
                    ):
                        raise ExternalRuntimeError("archive_output_invalid")
                    parent.verify()
                    _register_archive_generation(parent, authority, reservation)
                    if placeholder_identity is None:
                        raise ExternalRuntimeError("archive_publish_indeterminate")
                    _remove_exact_empty_archive_directory(
                        runtime_fd,
                        "out",
                        expected_identity=placeholder_identity,
                    )
                    _sync_archive_directory_fd(
                        runtime_fd,
                        "output_generation_placeholder_removed",
                    )
                    return
                except BaseException as error:
                    if publish_attempted:
                        if (
                            output_identity is None
                            or authority is None
                            or placeholder_fd < 0
                            or not placeholder_name
                        ):
                            raise ExternalRuntimeError(
                                "archive_publish_indeterminate"
                            ) from None
                        accepted = _settle_failed_archive_publication(
                            parent,
                            published=authority,
                            placeholder_fd=placeholder_fd,
                            placeholder_source_name=placeholder_name,
                            source_directory_fd=runtime_fd,
                        )
                        if accepted:
                            return
                    if isinstance(error, ExternalRuntimeError):
                        raise
                    if not isinstance(error, Exception):
                        raise
                    raise ExternalRuntimeError("archive_publish_failed") from None
    except ExternalRuntimeError:
        raise
    except Exception:
        raise ExternalRuntimeError("archive_publish_failed") from None
    finally:
        if staged_output_fd >= 0:
            os.close(staged_output_fd)
        if runtime_fd >= 0:
            os.close(runtime_fd)
        if attempt_fd >= 0:
            os.close(attempt_fd)
        if placeholder_fd >= 0:
            os.close(placeholder_fd)


def _create_archive_runtime_workspace(
    archive_path: Path,
    output_fd: int,
    *,
    suffix: str,
) -> tuple[int, int, str, _ArchiveSourceIdentity, _ArchiveSourceIdentity]:
    directory_flags = (
        os.O_RDONLY
        | int(getattr(os, "O_CLOEXEC", 0))
        | int(getattr(os, "O_NOFOLLOW", 0))
        | int(getattr(os, "O_DIRECTORY", 0))
    )
    runtime_fd = -1
    staged_output_fd = -1
    transferred = False
    try:
        os.mkdir(_ARCHIVE_RUNTIME_DIR, mode=0o700, dir_fd=output_fd)
        runtime_fd = os.open(
            _ARCHIVE_RUNTIME_DIR,
            directory_flags,
            dir_fd=output_fd,
        )
        runtime_opened = os.fstat(runtime_fd)
        runtime_visible = os.stat(
            _ARCHIVE_RUNTIME_DIR,
            dir_fd=output_fd,
            follow_symlinks=False,
        )
        if (
            not stat.S_ISDIR(runtime_opened.st_mode)
            or stat.S_ISLNK(runtime_visible.st_mode)
            or _identity(runtime_opened) != _identity(runtime_visible)
        ):
            raise OSError

        os.mkdir("out", mode=0o700, dir_fd=runtime_fd)
        staged_output_fd = os.open("out", directory_flags, dir_fd=runtime_fd)
        staged_opened = os.fstat(staged_output_fd)
        staged_visible = os.stat("out", dir_fd=runtime_fd, follow_symlinks=False)
        if (
            not stat.S_ISDIR(staged_opened.st_mode)
            or stat.S_ISLNK(staged_visible.st_mode)
            or _identity(staged_opened) != _identity(staged_visible)
        ):
            raise OSError

        opaque_source_name = f"input{suffix}"
        source_identity, staged_source_identity = (
            _copy_regular_archive_source_to_directory(
                archive_path,
                runtime_fd,
                opaque_source_name,
            )
        )
        os.fsync(staged_output_fd)
        os.fsync(runtime_fd)
        os.fsync(output_fd)
        result = (
            runtime_fd,
            staged_output_fd,
            opaque_source_name,
            source_identity,
            staged_source_identity,
        )
        transferred = True
        return result
    except ExternalRuntimeError:
        raise
    except OSError:
        raise ExternalRuntimeError("archive_publish_failed") from None
    finally:
        if not transferred:
            if staged_output_fd >= 0:
                os.close(staged_output_fd)
            if runtime_fd >= 0:
                os.close(runtime_fd)

def _read_extract_marker_fd(root_fd: int) -> dict[str, object]:
    descriptor = -1
    try:
        before = os.stat(_MARKER_FILE, dir_fd=root_fd, follow_symlinks=False)
        descriptor = os.open(
            _MARKER_FILE,
            os.O_RDONLY | int(getattr(os, "O_CLOEXEC", 0)) | int(getattr(os, "O_NOFOLLOW", 0)),
            dir_fd=root_fd,
        )
        opened = os.fstat(descriptor)
        if (
            stat.S_ISLNK(before.st_mode)
            or not stat.S_ISREG(before.st_mode)
            or not stat.S_ISREG(opened.st_mode)
            or int(opened.st_nlink) != 1
            or int(opened.st_size) > _ARCHIVE_MAX_MARKER_BYTES
            or _identity(before) != _identity(opened)
        ):
            raise OSError
        with os.fdopen(descriptor, "r", encoding="utf-8", closefd=False) as handle:
            payload = json.load(handle, object_pairs_hook=_strict_json_object)
        after = os.fstat(descriptor)
        visible = os.stat(_MARKER_FILE, dir_fd=root_fd, follow_symlinks=False)
        if (
            _identity(opened) != _identity(after)
            or _identity(after) != _identity(visible)
            or _mutable_identity(opened) != _mutable_identity(after)
        ):
            raise OSError
    except (OSError, UnicodeError, json.JSONDecodeError, ValueError):
        raise ExternalRuntimeError("archive_cache_invalid") from None
    finally:
        if descriptor >= 0:
            os.close(descriptor)
    _validate_extract_marker(payload)
    return payload


def _validate_extract_marker(payload: object) -> None:
    if not isinstance(payload, dict) or frozenset(payload) != _MARKER_KEYS:
        raise ExternalRuntimeError("archive_cache_invalid")
    if type(payload.get("schema_version")) is not int or payload["schema_version"] != 4:
        raise ExternalRuntimeError("archive_cache_invalid")
    string_keys = {"publication_nonce", "source_sha256", "output_tree_sha256"}
    integer_keys = _MARKER_KEYS - {"schema_version", "output_inventory", *string_keys}
    if any(type(payload.get(key)) is not int for key in integer_keys):
        raise ExternalRuntimeError("archive_cache_invalid")
    if any(int(payload[key]) < 0 for key in ("output_entry_count", "output_file_count", "output_total_bytes")):
        raise ExternalRuntimeError("archive_cache_invalid")
    if any(
        not isinstance(payload.get(key), str)
        or len(str(payload[key])) != 64
        or any(character not in "0123456789abcdef" for character in str(payload[key]))
        for key in string_keys
    ):
        raise ExternalRuntimeError("archive_cache_invalid")
    raw_inventory = payload.get("output_inventory")
    if not isinstance(raw_inventory, list) or len(raw_inventory) > _ARCHIVE_MAX_ENTRIES:
        raise ExternalRuntimeError("archive_cache_invalid")
    inventory: list[_ArchiveTreeEntry] = []
    prior_path_bytes: bytes | None = None
    for raw_entry in raw_inventory:
        if not isinstance(raw_entry, dict) or frozenset(raw_entry) != _INVENTORY_ENTRY_KEYS:
            raise ExternalRuntimeError("archive_cache_invalid")
        path = raw_entry.get("path")
        entry_type = raw_entry.get("type")
        sha256 = raw_entry.get("sha256")
        if (
            not isinstance(path, str)
            or not path
            or "\x00" in path
            or "\\" in path
            or path.startswith("/")
            or any(part in {"", ".", ".."} for part in path.split("/"))
            or any(part in _INTERNAL_EXTRACT_DIRS for part in path.split("/"))
            or len(os.fsencode(path)) > _ARCHIVE_MAX_PATH_BYTES
            or entry_type not in {"file", "directory"}
            or not isinstance(sha256, str)
        ):
            raise ExternalRuntimeError("archive_cache_invalid")
        path_bytes = os.fsencode(path)
        if prior_path_bytes is not None and path_bytes <= prior_path_bytes:
            raise ExternalRuntimeError("archive_cache_invalid")
        prior_path_bytes = path_bytes
        integer_fields = (
            "size",
            "device",
            "inode",
            "uid",
            "mode",
            "links",
            "mtime_ns",
            "ctime_ns",
            "generation",
        )
        if any(type(raw_entry.get(key)) is not int or int(raw_entry[key]) < 0 for key in integer_fields):
            raise ExternalRuntimeError("archive_cache_invalid")
        if int(raw_entry["inode"]) == 0:
            raise ExternalRuntimeError("archive_cache_invalid")
        if int(raw_entry["mode"]) > 0o7777 or (
            not _is_archive_runtime_entry(path)
            and int(raw_entry["mode"]) & 0o077
        ):
            raise ExternalRuntimeError("archive_cache_invalid")
        if hasattr(os, "geteuid") and int(raw_entry["uid"]) != int(os.geteuid()):
            raise ExternalRuntimeError("archive_cache_invalid")
        if entry_type == "directory":
            if int(raw_entry["size"]) != 0 or sha256 or int(raw_entry["links"]) < 1:
                raise ExternalRuntimeError("archive_cache_invalid")
        elif (
            int(raw_entry["links"]) != 1
            or len(sha256) != 64
            or any(character not in "0123456789abcdef" for character in sha256)
        ):
            raise ExternalRuntimeError("archive_cache_invalid")
        inventory.append(
            _ArchiveTreeEntry(
                path=path,
                entry_type=str(entry_type),
                size=int(raw_entry["size"]),
                sha256=sha256,
                device=int(raw_entry["device"]),
                inode=int(raw_entry["inode"]),
                uid=int(raw_entry["uid"]),
                mode=int(raw_entry["mode"]),
                links=int(raw_entry["links"]),
                mtime_ns=int(raw_entry["mtime_ns"]),
                ctime_ns=int(raw_entry["ctime_ns"]),
                generation=int(raw_entry["generation"]),
            )
        )
    file_entries = [entry for entry in inventory if entry.entry_type == "file"]
    if (
        len(inventory) != int(payload["output_entry_count"])
        or len(file_entries) != int(payload["output_file_count"])
        or sum(entry.size for entry in file_entries) != int(payload["output_total_bytes"])
    ):
        raise ExternalRuntimeError("archive_cache_invalid")
    digest = hashlib.sha256(b"analytix-archive-output-tree-v1\x00")
    for entry in inventory:
        record = entry.content_record()
        digest.update(len(record).to_bytes(8, "big"))
        digest.update(record)
    if digest.hexdigest() != payload["output_tree_sha256"]:
        raise ExternalRuntimeError("archive_cache_invalid")


def _write_extract_marker_fd(
    root_fd: int,
    *,
    publication_nonce: str,
    source_identity: _ArchiveSourceIdentity,
    tree_summary: _ArchiveTreeSummary,
) -> None:
    payload = json.dumps(
        {
            **source_identity.marker(),
            **tree_summary.marker(),
            "publication_nonce": publication_nonce,
        },
        ensure_ascii=True,
        sort_keys=True,
        separators=(",", ":"),
    ).encode("utf-8")
    if len(payload) > _ARCHIVE_MAX_MARKER_BYTES:
        raise ExternalRuntimeError("archive_output_quota_exceeded")
    descriptor = -1
    try:
        descriptor = os.open(
            _MARKER_FILE,
            os.O_WRONLY
            | os.O_CREAT
            | os.O_EXCL
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0)),
            0o600,
            dir_fd=root_fd,
        )
        _write_all(descriptor, payload)
        os.fsync(descriptor)
    except OSError:
        raise ExternalRuntimeError("archive_publish_failed") from None
    finally:
        if descriptor >= 0:
            os.close(descriptor)


def _read_regular_archive_source(source: Path) -> _ArchiveSourceIdentity:
    return _read_or_copy_regular_archive_source(source, None)


def _copy_regular_archive_source(source: Path, target: Path) -> _ArchiveSourceIdentity:
    return _read_or_copy_regular_archive_source(source, target)


def _copy_regular_archive_source_to_directory(
    source: Path,
    target_directory_fd: int,
    target_name: str,
) -> tuple[_ArchiveSourceIdentity, _ArchiveSourceIdentity]:
    source_fd = -1
    target_fd = -1
    digest = hashlib.sha256()
    try:
        before = source.lstat()
        source_fd = os.open(
            source,
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0)),
        )
        opened = os.fstat(source_fd)
        if (
            stat.S_ISLNK(before.st_mode)
            or not stat.S_ISREG(before.st_mode)
            or not stat.S_ISREG(opened.st_mode)
            or _identity(before) != _identity(opened)
        ):
            raise OSError
        target_fd = os.open(
            _safe_archive_sibling_name(target_name),
            os.O_WRONLY
            | os.O_CREAT
            | os.O_EXCL
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0)),
            0o600,
            dir_fd=target_directory_fd,
        )
        target_opened = os.fstat(target_fd)
        if not stat.S_ISREG(target_opened.st_mode) or int(target_opened.st_nlink) != 1:
            raise OSError
        while True:
            chunk = os.read(source_fd, 1024 * 1024)
            if not chunk:
                break
            digest.update(chunk)
            _write_all(target_fd, chunk)
        os.fsync(target_fd)
        opened_after = os.fstat(source_fd)
        path_after = source.lstat()
        target_after = os.fstat(target_fd)
        target_visible = os.stat(
            target_name,
            dir_fd=target_directory_fd,
            follow_symlinks=False,
        )
        if (
            _identity(opened) != _identity(opened_after)
            or _identity(opened_after) != _identity(path_after)
            or _mutable_identity(opened) != _mutable_identity(opened_after)
            or stat.S_ISLNK(path_after.st_mode)
            or not stat.S_ISREG(path_after.st_mode)
            or _identity(target_opened) != _identity(target_after)
            or _identity(target_after) != _identity(target_visible)
            or int(target_after.st_size) != int(opened_after.st_size)
        ):
            raise OSError
        sha256 = digest.hexdigest()
        return (
            _source_identity(opened_after, sha256),
            _source_identity(target_after, sha256),
        )
    except OSError:
        raise ExternalRuntimeError("archive_source_unavailable") from None
    finally:
        if source_fd >= 0:
            os.close(source_fd)
        if target_fd >= 0:
            os.close(target_fd)


def _read_regular_archive_source_at(
    directory_fd: int,
    name: str,
) -> _ArchiveSourceIdentity:
    descriptor = -1
    digest = hashlib.sha256()
    try:
        before = os.stat(name, dir_fd=directory_fd, follow_symlinks=False)
        descriptor = os.open(
            _safe_archive_sibling_name(name),
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0)),
            dir_fd=directory_fd,
        )
        opened = os.fstat(descriptor)
        if (
            stat.S_ISLNK(before.st_mode)
            or not stat.S_ISREG(before.st_mode)
            or not stat.S_ISREG(opened.st_mode)
            or int(opened.st_nlink) != 1
            or _identity(before) != _identity(opened)
        ):
            raise OSError
        while True:
            chunk = os.read(descriptor, 1024 * 1024)
            if not chunk:
                break
            digest.update(chunk)
        after = os.fstat(descriptor)
        visible = os.stat(name, dir_fd=directory_fd, follow_symlinks=False)
        if (
            _identity(opened) != _identity(after)
            or _identity(after) != _identity(visible)
            or _mutable_identity(opened) != _mutable_identity(after)
        ):
            raise OSError
        return _source_identity(after, digest.hexdigest())
    except OSError:
        raise ExternalRuntimeError("archive_source_unavailable") from None
    finally:
        if descriptor >= 0:
            os.close(descriptor)


def _source_identity(
    value: os.stat_result,
    sha256: str,
) -> _ArchiveSourceIdentity:
    return _ArchiveSourceIdentity(
        device=int(value.st_dev),
        inode=int(value.st_ino),
        size=int(value.st_size),
        mtime_ns=_stat_ns(value, "st_mtime_ns", "st_mtime"),
        ctime_ns=_stat_ns(value, "st_ctime_ns", "st_ctime"),
        sha256=sha256,
    )


def _read_or_copy_regular_archive_source(source: Path, target: Path | None) -> _ArchiveSourceIdentity:
    source_fd = -1
    target_fd = -1
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
            or _identity(before) != _identity(opened)
        ):
            raise OSError
        if target is not None:
            target_fd = os.open(
                target,
                os.O_WRONLY
                | os.O_CREAT
                | os.O_EXCL
                | int(getattr(os, "O_CLOEXEC", 0))
                | int(getattr(os, "O_NOFOLLOW", 0)),
                0o600,
            )
        while True:
            chunk = os.read(source_fd, 1024 * 1024)
            if not chunk:
                break
            digest.update(chunk)
            if target_fd >= 0:
                _write_all(target_fd, chunk)
        if target_fd >= 0:
            os.fsync(target_fd)
        opened_after = os.fstat(source_fd)
        path_after = source.lstat()
        if (
            _identity(opened) != _identity(opened_after)
            or _identity(opened_after) != _identity(path_after)
            or _mutable_identity(opened) != _mutable_identity(opened_after)
            or stat.S_ISLNK(path_after.st_mode)
            or not stat.S_ISREG(path_after.st_mode)
        ):
            raise OSError
        return _source_identity(opened_after, digest.hexdigest())
    except OSError:
        raise ExternalRuntimeError("archive_source_unavailable") from None
    finally:
        if source_fd >= 0:
            os.close(source_fd)
        if target_fd >= 0:
            os.close(target_fd)


def _copy_extracted_tree_secure(source_root: Path | int, target_root: Path | int) -> None:
    source_root_fd = -1
    target_root_fd = -1
    entry_count = 0
    total_bytes = 0
    try:
        directory_flags = (
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0))
            | int(getattr(os, "O_DIRECTORY", 0))
        )
        if isinstance(source_root, int):
            source_root_fd = _reopen_archive_directory_fd(source_root)
            source_before = os.fstat(source_root_fd)
        else:
            source_before = source_root.lstat()
            if stat.S_ISLNK(source_before.st_mode) or not stat.S_ISDIR(source_before.st_mode):
                raise OSError
            source_root_fd = os.open(source_root, directory_flags)
        if isinstance(target_root, int):
            target_root_fd = _reopen_archive_directory_fd(target_root)
            target_before = os.fstat(target_root_fd)
        else:
            target_before = target_root.lstat()
            if stat.S_ISLNK(target_before.st_mode) or not stat.S_ISDIR(target_before.st_mode):
                raise OSError
            target_root_fd = os.open(target_root, directory_flags)
        source_opened = os.fstat(source_root_fd)
        target_opened = os.fstat(target_root_fd)
        if (
            not stat.S_ISDIR(source_opened.st_mode)
            or not stat.S_ISDIR(target_opened.st_mode)
            or _identity(source_before) != _identity(source_opened)
            or _identity(target_before) != _identity(target_opened)
        ):
            raise OSError

        def copy_directory(source_fd: int, target_fd: int, prefix: str, depth: int) -> None:
            nonlocal entry_count, total_bytes
            if depth > _ARCHIVE_MAX_DEPTH:
                raise ExternalRuntimeError("archive_output_quota_exceeded")
            source_directory_before = os.fstat(source_fd)
            try:
                entries = os.scandir(source_fd)
            except OSError:
                raise ExternalRuntimeError("archive_output_invalid") from None
            with entries:
                for entry in entries:
                    relative = f"{prefix}/{entry.name}" if prefix else entry.name
                    entry_count += 1
                    if entry_count > _ARCHIVE_MAX_ENTRIES:
                        raise ExternalRuntimeError("archive_output_quota_exceeded")
                    if len(os.fsencode(relative.replace(os.sep, "/"))) > _ARCHIVE_MAX_PATH_BYTES:
                        raise ExternalRuntimeError("archive_output_quota_exceeded")
                    try:
                        entry_before = entry.stat(follow_symlinks=False)
                    except OSError:
                        raise ExternalRuntimeError("archive_output_invalid") from None
                    if stat.S_ISLNK(entry_before.st_mode):
                        raise ExternalRuntimeError("archive_output_invalid")
                    if stat.S_ISDIR(entry_before.st_mode):
                        source_child_fd = -1
                        target_child_fd = -1
                        try:
                            source_child_fd = os.open(
                                entry.name,
                                directory_flags,
                                dir_fd=source_fd,
                            )
                            source_child_opened = os.fstat(source_child_fd)
                            source_child_path = os.stat(entry.name, dir_fd=source_fd, follow_symlinks=False)
                            if (
                                not stat.S_ISDIR(source_child_opened.st_mode)
                                or stat.S_ISLNK(source_child_path.st_mode)
                                or _identity(entry_before) != _identity(source_child_opened)
                                or _identity(source_child_opened) != _identity(source_child_path)
                            ):
                                raise OSError
                            os.mkdir(entry.name, mode=0o700, dir_fd=target_fd)
                            target_child_fd = os.open(entry.name, directory_flags, dir_fd=target_fd)
                            target_child_opened = os.fstat(target_child_fd)
                            target_child_path = os.stat(entry.name, dir_fd=target_fd, follow_symlinks=False)
                            if (
                                not stat.S_ISDIR(target_child_opened.st_mode)
                                or stat.S_ISLNK(target_child_path.st_mode)
                                or _identity(target_child_opened) != _identity(target_child_path)
                            ):
                                raise OSError
                            copy_directory(source_child_fd, target_child_fd, relative, depth + 1)
                            os.fsync(target_child_fd)
                            source_child_after = os.fstat(source_child_fd)
                            source_child_path_after = os.stat(
                                entry.name,
                                dir_fd=source_fd,
                                follow_symlinks=False,
                            )
                            target_child_after = os.fstat(target_child_fd)
                            target_child_path_after = os.stat(
                                entry.name,
                                dir_fd=target_fd,
                                follow_symlinks=False,
                            )
                            if (
                                _identity(source_child_opened) != _identity(source_child_after)
                                or _identity(source_child_after) != _identity(source_child_path_after)
                                or _mutable_identity(source_child_opened) != _mutable_identity(source_child_after)
                                or _identity(target_child_opened) != _identity(target_child_after)
                                or _identity(target_child_after) != _identity(target_child_path_after)
                            ):
                                raise OSError
                        except ExternalRuntimeError:
                            raise
                        except OSError:
                            raise ExternalRuntimeError("archive_output_invalid") from None
                        finally:
                            if source_child_fd >= 0:
                                os.close(source_child_fd)
                            if target_child_fd >= 0:
                                os.close(target_child_fd)
                        continue
                    if not stat.S_ISREG(entry_before.st_mode) or int(entry_before.st_nlink) != 1:
                        raise ExternalRuntimeError("archive_output_invalid")
                    expected_file_size = int(entry_before.st_size)
                    if expected_file_size > _ARCHIVE_MAX_FILE_BYTES:
                        raise ExternalRuntimeError("archive_output_quota_exceeded")
                    if total_bytes + expected_file_size > _ARCHIVE_MAX_TOTAL_BYTES:
                        raise ExternalRuntimeError("archive_output_quota_exceeded")
                    file_size = _copy_regular_output_file(
                        source_fd,
                        target_fd,
                        entry.name,
                        entry_before,
                    )
                    if file_size != expected_file_size:
                        raise ExternalRuntimeError("archive_output_invalid")
                    total_bytes += file_size
            source_directory_after = os.fstat(source_fd)
            if (
                _identity(source_directory_before) != _identity(source_directory_after)
                or _mutable_identity(source_directory_before) != _mutable_identity(source_directory_after)
            ):
                raise ExternalRuntimeError("archive_output_invalid")

        copy_directory(source_root_fd, target_root_fd, "", 0)
        os.fsync(target_root_fd)
        source_after = os.fstat(source_root_fd)
        source_path_after = (
            os.fstat(source_root_fd)
            if isinstance(source_root, int)
            else source_root.lstat()
        )
        target_after = os.fstat(target_root_fd)
        target_path_after = target_root.lstat() if isinstance(target_root, Path) else target_after
        if (
            _identity(source_opened) != _identity(source_after)
            or _identity(source_after) != _identity(source_path_after)
            or _mutable_identity(source_opened) != _mutable_identity(source_after)
            or _identity(target_opened) != _identity(target_after)
            or _identity(target_after) != _identity(target_path_after)
        ):
            raise OSError
    except ExternalRuntimeError:
        raise
    except OSError:
        raise ExternalRuntimeError("archive_output_invalid") from None
    finally:
        if source_root_fd >= 0:
            os.close(source_root_fd)
        if target_root_fd >= 0:
            os.close(target_root_fd)


def _copy_regular_output_file(
    source_directory_fd: int,
    target_directory_fd: int,
    name: str,
    before: os.stat_result,
) -> int:
    source_fd = -1
    target_fd = -1
    copied_bytes = 0
    try:
        source_fd = os.open(
            name,
            os.O_RDONLY | int(getattr(os, "O_CLOEXEC", 0)) | int(getattr(os, "O_NOFOLLOW", 0)),
            dir_fd=source_directory_fd,
        )
        opened = os.fstat(source_fd)
        if not stat.S_ISREG(opened.st_mode) or int(opened.st_nlink) != 1 or _identity(before) != _identity(opened):
            raise OSError
        target_fd = os.open(
            name,
            os.O_WRONLY
            | os.O_CREAT
            | os.O_EXCL
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0)),
            0o600,
            dir_fd=target_directory_fd,
        )
        target_opened = os.fstat(target_fd)
        if not stat.S_ISREG(target_opened.st_mode) or int(target_opened.st_nlink) != 1:
            raise OSError
        while True:
            chunk = os.read(source_fd, 1024 * 1024)
            if not chunk:
                break
            _write_all(target_fd, chunk)
            copied_bytes += len(chunk)
        os.fsync(target_fd)
        opened_after = os.fstat(source_fd)
        path_after = os.stat(name, dir_fd=source_directory_fd, follow_symlinks=False)
        target_after = os.fstat(target_fd)
        target_path_after = os.stat(name, dir_fd=target_directory_fd, follow_symlinks=False)
        if (
            _identity(opened) != _identity(opened_after)
            or _identity(opened_after) != _identity(path_after)
            or _mutable_identity(opened) != _mutable_identity(opened_after)
            or copied_bytes != int(opened_after.st_size)
            or _identity(target_opened) != _identity(target_after)
            or _identity(target_after) != _identity(target_path_after)
        ):
            raise OSError
        return copied_bytes
    except OSError:
        raise ExternalRuntimeError("archive_output_invalid") from None
    finally:
        if source_fd >= 0:
            os.close(source_fd)
        if target_fd >= 0:
            os.close(target_fd)


@contextmanager
def _open_canonical_publish_parent(output_dir: Path) -> Iterator[_PinnedArchivePublishParent]:
    raw_output = os.fspath(output_dir)
    _validate_archive_output_path(output_dir)
    parent = Path(raw_output).parent
    parts = parent.parts[1:]
    if len(parts) > _ARCHIVE_MAX_PARENT_COMPONENTS or len(os.fsencode(parent)) > _ARCHIVE_MAX_PATH_BYTES * 4:
        raise ExternalRuntimeError("archive_output_untrusted")
    descriptors: list[int] = []
    edge_names: list[str] = []
    identities: list[tuple[int, int]] = []
    authority: _PinnedArchivePublishParent | None = None
    try:
        descriptor = os.open(
            parent.anchor,
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0))
            | int(getattr(os, "O_DIRECTORY", 0)),
        )
        descriptors.append(descriptor)
        identities.append(_identity(os.fstat(descriptor)))
        for part in parts:
            child_fd = -1
            created_or_raced = False
            try:
                child_fd = os.open(
                    part,
                    os.O_RDONLY
                    | int(getattr(os, "O_CLOEXEC", 0))
                    | int(getattr(os, "O_NOFOLLOW", 0))
                    | int(getattr(os, "O_DIRECTORY", 0)),
                    dir_fd=descriptors[-1],
                )
            except FileNotFoundError:
                owner = os.fstat(descriptors[-1])
                get_euid = getattr(os, "geteuid", None)
                if (
                    not stat.S_ISDIR(owner.st_mode)
                    or (get_euid is not None and int(owner.st_uid) != int(get_euid()))
                    or stat.S_IMODE(owner.st_mode) & 0o022
                ):
                    raise OSError
                try:
                    os.mkdir(part, mode=0o700, dir_fd=descriptors[-1])
                    os.fsync(descriptors[-1])
                except FileExistsError:
                    pass
                created_or_raced = True
                child_fd = os.open(
                    part,
                    os.O_RDONLY
                    | int(getattr(os, "O_CLOEXEC", 0))
                    | int(getattr(os, "O_NOFOLLOW", 0))
                    | int(getattr(os, "O_DIRECTORY", 0)),
                    dir_fd=descriptors[-1],
                )
            try:
                child_opened = os.fstat(child_fd)
                child_path = os.stat(part, dir_fd=descriptors[-1], follow_symlinks=False)
                get_euid = getattr(os, "geteuid", None)
                if (
                    not stat.S_ISDIR(child_opened.st_mode)
                    or stat.S_ISLNK(child_path.st_mode)
                    or _identity(child_opened) != _identity(child_path)
                    or (
                        created_or_raced
                        and (
                            (get_euid is not None and int(child_opened.st_uid) != int(get_euid()))
                            or stat.S_IMODE(child_opened.st_mode) & 0o022
                        )
                    )
                ):
                    raise OSError
            except BaseException:
                if child_fd >= 0:
                    os.close(child_fd)
                    child_fd = -1
                raise
            descriptors.append(child_fd)
            edge_names.append(part)
            identities.append(_identity(child_opened))
            child_fd = -1
        final_stat = os.fstat(descriptors[-1])
        get_euid = getattr(os, "geteuid", None)
        if (
            not stat.S_ISDIR(final_stat.st_mode)
            or (get_euid is not None and int(final_stat.st_uid) != int(get_euid()))
            or stat.S_IMODE(final_stat.st_mode) & 0o022
        ):
            raise OSError
        authority = _PinnedArchivePublishParent(
            path=parent,
            output_name=Path(raw_output).name,
            descriptors=tuple(descriptors),
            edge_names=tuple(edge_names),
            identities=tuple(identities),
        )
        authority.verify()
        yield authority
    except OSError:
        raise ExternalRuntimeError("archive_output_untrusted") from None
    finally:
        if authority is not None:
            authority.close()
        else:
            for descriptor in reversed(descriptors):
                try:
                    os.close(descriptor)
                except OSError:
                    pass


@contextmanager
def _archive_publication_lock(parent: _PinnedArchivePublishParent) -> Iterator[None]:
    import fcntl

    if bool(getattr(_ARCHIVE_PUBLICATION_LOCK_STATE, "active", False)):
        raise ExternalRuntimeError("archive_publish_indeterminate")
    _ARCHIVE_PUBLICATION_LOCK_STATE.active = True
    descriptor = -1
    acquired = False
    try:
        # Lock the already-pinned parent inode itself. A mutable lock filename
        # can be replaced and create two authorities for one namespace.
        descriptor = os.dup(parent.descriptor)
        opened = os.fstat(descriptor)
        if not stat.S_ISDIR(opened.st_mode) or _identity(opened) != parent.identities[-1]:
            raise OSError
        deadline = time.monotonic() + _ARCHIVE_PUBLICATION_LOCK_TIMEOUT_SECONDS
        while True:
            try:
                fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
                acquired = True
                break
            except OSError as error:
                if error.errno not in {errno.EACCES, errno.EAGAIN}:
                    raise
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise ExternalRuntimeError("archive_publish_lock_timeout") from None
                time.sleep(min(0.01, remaining))
        parent.verify()
        yield
    except ExternalRuntimeError:
        raise
    except OSError:
        raise ExternalRuntimeError("archive_publish_failed") from None
    finally:
        if descriptor >= 0:
            if acquired:
                try:
                    fcntl.flock(descriptor, fcntl.LOCK_UN)
                except OSError:
                    pass
            os.close(descriptor)
        _ARCHIVE_PUBLICATION_LOCK_STATE.active = False


def _recover_archive_publication(parent: _PinnedArchivePublishParent) -> None:
    # Disk state is not a host-issued transaction capability. Even a
    # schema-valid journal can be forged by the same filesystem principal, so
    # restart recovery is deliberately zero-mutation until a durable host
    # transaction registry exists. Runtime recovery never mutates unexplained
    # legacy or interrupted publication state.
    state_name = f".{parent.output_name}.analytix-publish-state"
    if _stat_archive_sibling(parent, state_name) is not None:
        raise ExternalRuntimeError("archive_publish_indeterminate")

    prefix = f".{parent.output_name}.analytix-previous-"
    quarantine_prefix = f".{parent.output_name}.analytix-quarantine-"
    publish_prefix = f".{parent.output_name}.analytix-publish-"
    backups: list[str] = []
    quarantines: list[str] = []
    publishes: list[str] = []
    scanned_entries = 0
    try:
        entries = os.scandir(parent.descriptor)
        with entries:
            for entry in entries:
                scanned_entries += 1
                if scanned_entries > _ARCHIVE_MAX_ENTRIES:
                    raise ExternalRuntimeError("archive_publish_indeterminate")
                if entry.name.startswith(prefix):
                    backups.append(entry.name)
                elif entry.name.startswith(quarantine_prefix):
                    quarantines.append(entry.name)
                elif entry.name.startswith(publish_prefix):
                    publishes.append(entry.name)
                if len(backups) > 1 or len(publishes) > 1 or len(quarantines) > _ARCHIVE_MAX_ENTRIES:
                    raise ExternalRuntimeError("archive_publish_indeterminate")
    except ExternalRuntimeError:
        raise
    except OSError:
        raise ExternalRuntimeError("archive_publish_failed") from None
    for candidate in (*backups, *quarantines, *publishes):
        candidate_stat = _stat_archive_sibling(parent, candidate)
        if candidate_stat is None or stat.S_ISLNK(candidate_stat.st_mode) or not stat.S_ISDIR(candidate_stat.st_mode):
            raise ExternalRuntimeError("archive_publish_indeterminate")
    output_stat = _stat_archive_sibling(parent, parent.output_name)
    if output_stat is None:
        if quarantines or backups or publishes:
            raise ExternalRuntimeError("archive_publish_indeterminate")
        return
    if stat.S_ISLNK(output_stat.st_mode) or not stat.S_ISDIR(output_stat.st_mode):
        raise ExternalRuntimeError("archive_output_untrusted")
    if backups or quarantines or publishes:
        # A visible output plus any unreceipted transaction residue is an
        # unknown commit state, never proof of a completed publication.
        raise ExternalRuntimeError("archive_publish_indeterminate")


def _normalized_sha256(value: str) -> str:
    normalized = str(value or "").strip().lower()
    if len(normalized) != 64 or any(character not in "0123456789abcdef" for character in normalized):
        raise ExternalRuntimeError("archive_source_identity_invalid")
    return normalized


def _validate_archive_output_path(output_dir: Path) -> None:
    raw_output = os.fspath(output_dir)
    if (
        not raw_output
        or "\x00" in raw_output
        or not os.path.isabs(raw_output)
        or os.path.abspath(raw_output) != raw_output
        or os.path.normpath(raw_output) != raw_output
        or Path(raw_output).name in {"", ".", ".."}
    ):
        raise ExternalRuntimeError("archive_output_untrusted")


def _validate_archive_source_budget(source: _ArchiveSourceIdentity) -> None:
    if (
        source.size < 0
        or source.size > _ARCHIVE_MAX_FILE_BYTES
        or source.size > _ARCHIVE_MAX_TOTAL_BYTES
    ):
        raise ExternalRuntimeError("archive_source_quota_exceeded")


def _public_archive_tree_summary(summary: _ArchiveTreeSummary) -> _ArchiveTreeSummary:
    inventory = tuple(
        entry
        for entry in summary.inventory
        if not _is_archive_runtime_entry(entry.path)
    )
    file_entries = tuple(entry for entry in inventory if entry.entry_type == "file")
    digest = hashlib.sha256(b"analytix-archive-output-tree-v1\x00")
    for entry in inventory:
        record = entry.content_record()
        digest.update(len(record).to_bytes(8, "big"))
        digest.update(record)
    return _ArchiveTreeSummary(
        entry_count=len(inventory),
        file_count=len(file_entries),
        total_bytes=sum(entry.size for entry in file_entries),
        sha256=digest.hexdigest(),
        inventory=inventory,
    )


def _short_stable_tag(value: str) -> str:
    return hashlib.sha256(str(value or "entry").encode("utf-8", errors="strict")).hexdigest()[:32]


def _archive_attempt_name(
    parent: _PinnedArchivePublishParent,
    reservation: _ArchiveGenerationReservationV1,
) -> str:
    logical_tag = hashlib.sha256(
        os.fspath(parent.path / parent.output_name).encode("utf-8", errors="strict")
    ).hexdigest()[:32]
    return _safe_archive_sibling_name(
        f".analytix-archive-attempt-{logical_tag}-{reservation.token}"
    )


def _validate_archive_retained_attempt_budget(
    parent: _PinnedArchivePublishParent,
    *,
    anticipated_source_bytes: int,
) -> None:
    if anticipated_source_bytes < 0:
        raise ExternalRuntimeError("archive_retained_attempt_budget_exceeded")
    directory_flags = (
        os.O_RDONLY
        | int(getattr(os, "O_CLOEXEC", 0))
        | int(getattr(os, "O_NOFOLLOW", 0))
        | int(getattr(os, "O_DIRECTORY", 0))
    )
    scan_fd = -1
    attempt_count = 0
    retained_bytes = 0
    try:
        scan_fd = _reopen_archive_directory_fd(parent.descriptor)
        entries = os.scandir(scan_fd)
        with entries:
            for entry in entries:
                if not entry.name.startswith(".analytix-archive-attempt-"):
                    continue
                attempt_count += 1
                if attempt_count >= _ARCHIVE_MAX_RETAINED_ATTEMPTS_PER_PARENT:
                    raise ExternalRuntimeError(
                        "archive_retained_attempt_budget_exceeded"
                    )
                before = entry.stat(follow_symlinks=False)
                if stat.S_ISLNK(before.st_mode) or not stat.S_ISDIR(before.st_mode):
                    raise ExternalRuntimeError("archive_retained_attempt_untrusted")
                attempt_fd = os.open(entry.name, directory_flags, dir_fd=scan_fd)
                try:
                    opened = os.fstat(attempt_fd)
                    if _identity(before) != _identity(opened):
                        raise OSError
                    _, attempt_bytes = _walk_archive_private_tree(
                        attempt_fd,
                        harden=False,
                        max_file_bytes=_ARCHIVE_MAX_RETAINED_BYTES_PER_PARENT,
                        max_total_bytes=_ARCHIVE_MAX_RETAINED_BYTES_PER_PARENT,
                    )
                    retained_bytes += attempt_bytes
                    if (
                        retained_bytes + anticipated_source_bytes
                        > _ARCHIVE_MAX_RETAINED_BYTES_PER_PARENT
                    ):
                        raise ExternalRuntimeError(
                            "archive_retained_attempt_budget_exceeded"
                        )
                finally:
                    os.close(attempt_fd)
        parent.verify()
    except ExternalRuntimeError as error:
        if str(error) in {
            "archive_output_invalid",
            "archive_output_quota_exceeded",
        }:
            raise ExternalRuntimeError("archive_retained_attempt_untrusted") from None
        raise
    except OSError:
        raise ExternalRuntimeError("archive_retained_attempt_untrusted") from None
    finally:
        if scan_fd >= 0:
            os.close(scan_fd)


def _strict_json_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate key")
        result[key] = value
    return result


def _identity(value: os.stat_result) -> tuple[int, int]:
    return int(value.st_dev), int(value.st_ino)


def _reopen_archive_directory_fd(descriptor: int) -> int:
    reopened = -1
    try:
        expected = os.fstat(descriptor)
        reopened = os.open(
            ".",
            os.O_RDONLY
            | int(getattr(os, "O_CLOEXEC", 0))
            | int(getattr(os, "O_NOFOLLOW", 0))
            | int(getattr(os, "O_DIRECTORY", 0)),
            dir_fd=descriptor,
        )
        current = os.fstat(reopened)
        if (
            not stat.S_ISDIR(expected.st_mode)
            or not stat.S_ISDIR(current.st_mode)
            or _identity(expected) != _identity(current)
        ):
            raise OSError
        return reopened
    except BaseException as error:
        if reopened >= 0:
            os.close(reopened)
        if isinstance(error, OSError):
            raise ExternalRuntimeError("archive_publish_indeterminate") from None
        raise


def _mutable_identity(value: os.stat_result) -> tuple[int, int, int]:
    return (
        int(value.st_size),
        _stat_ns(value, "st_mtime_ns", "st_mtime"),
        _stat_ns(value, "st_ctime_ns", "st_ctime"),
    )


def _archive_object_state(value: os.stat_result) -> tuple[int, ...]:
    return (
        int(value.st_dev),
        int(value.st_ino),
        int(value.st_uid),
        stat.S_IMODE(value.st_mode),
        int(value.st_nlink),
        int(value.st_size),
        _stat_ns(value, "st_mtime_ns", "st_mtime"),
        _stat_ns(value, "st_ctime_ns", "st_ctime"),
        int(getattr(value, "st_gen", 0) or 0),
    )


def _stat_ns(value: os.stat_result, ns_name: str, seconds_name: str) -> int:
    exact = getattr(value, ns_name, None)
    if exact is not None:
        return int(exact)
    return int(float(getattr(value, seconds_name)) * 1_000_000_000)


def _write_all(descriptor: int, payload: bytes) -> None:
    offset = 0
    while offset < len(payload):
        written = os.write(descriptor, payload[offset:])
        if written <= 0:
            raise OSError
        offset += int(written)
