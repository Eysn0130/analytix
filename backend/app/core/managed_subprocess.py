from __future__ import annotations

import hashlib
import math
import os
import secrets
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Callable, Protocol, Sequence

from app.core.process_environment import build_sanitized_subprocess_env

_AUTHORITY_PROTOCOL_VERSION = 3
_AUTHORITY_UNAVAILABLE = "managed_process_execution_authority_unavailable"
_MAX_ARGUMENT_COUNT = 256
_MAX_ARGUMENT_BYTES = 1024 * 1024
_MAX_INPUT_BYTES = 64 * 1024 * 1024
_MAX_CAPTURED_STDOUT_BYTES = 8 * 1024 * 1024
_MAX_ENVIRONMENT_COUNT = 64
_MAX_ENVIRONMENT_BYTES = 64 * 1024
_SHA256_HEX_LENGTH = 64
_ENVIRONMENT_DIGEST_DOMAIN = b"analytix-managed-process-environment-v1\0"


class ManagedProcessTimeout(RuntimeError):
    pass


class ManagedProcessLaunchError(RuntimeError):
    pass


class ManagedProcessTerminationError(RuntimeError):
    pass


class ManagedProcessOutputLimit(RuntimeError):
    pass


@dataclass(frozen=True)
class ManagedExecutionReceipt:
    protocol_version: int
    launch_nonce: str
    cwd_device: int
    cwd_inode: int
    executable_sha256: str
    environment_sha256: str


@dataclass(frozen=True)
class ManagedProcessResult:
    returncode: int
    stdout: bytes
    execution_receipt: ManagedExecutionReceipt | None = field(
        default=None,
        compare=False,
        repr=False,
    )


@dataclass(frozen=True)
class ManagedExecutionAuthorityStatus:
    """Non-sensitive proof surface exposed before any case data is submitted.

    An available production adapter must implement every platform guarantee.
    Merely constructing this value does not grant authority: the production
    factory below is closed and currently returns only the unavailable adapter.
    Tests may replace that factory to exercise the facade contract without
    creating a process.
    """

    available: bool
    protocol_version: int
    platform: str
    reason_code: str
    guarantees: frozenset[str]


@dataclass(frozen=True)
class ManagedExecutionRequest:
    """Versioned hand-off to a platform execution authority.

    The adapter owns process creation. On Darwin it must start a package-owned
    Mach-O helper suspended, set the cwd from ``cwd_fd`` (never by reopening
    ``cwd``),
    compare the loaded CodeDirectory/CDHash obtained with ``csops`` to the
    CodeDirectory parsed from the already-open executable fd, and kill/wait on
    any mismatch before SIGCONT. After resume, it must verify a nonce-bound
    readiness response and the helper's ``RLIMIT_NPROC=0`` state before sending
    argv, stdin, or other case data. Unmodified helpers that cannot prove this
    bootstrap are not admissible.
    """

    protocol_version: int
    launch_nonce: str
    argv: tuple[str, ...]
    # ``cwd`` is diagnostic identity only. The adapter MUST fchdir this pinned
    # descriptor and MUST NOT resolve the path again. The facade owns and
    # closes the descriptor after execute returns.
    cwd: Path
    cwd_fd: int
    cwd_device: int
    cwd_inode: int
    input_bytes: bytes | None
    timeout_seconds: float
    expected_executable_sha256: str
    # The adapter MUST replace, rather than merge with, its ambient process
    # environment. The tuple is case-insensitively unique, byte-bounded, and
    # built from an empty source plus the fixed platform baseline.
    environment: tuple[tuple[str, str], ...]
    environment_sha256: str
    capture_stdout: bool
    resource_monitor: Callable[[], None] | None


class ManagedExecutionAuthority(Protocol):
    def status(self) -> ManagedExecutionAuthorityStatus:
        """Return readiness without receiving argv, cwd, stdin, or case data."""
        ...

    def execute(self, request: ManagedExecutionRequest) -> ManagedProcessResult:
        """Execute only after enforcing every guarantee declared by status()."""
        ...


class _UnavailableExecutionAuthority:
    def status(self) -> ManagedExecutionAuthorityStatus:
        return ManagedExecutionAuthorityStatus(
            available=False,
            protocol_version=_AUTHORITY_PROTOCOL_VERSION,
            platform=sys.platform,
            reason_code=_AUTHORITY_UNAVAILABLE,
            guarantees=frozenset(),
        )

    def execute(self, request: ManagedExecutionRequest) -> ManagedProcessResult:
        del request
        raise AssertionError("unavailable execution authority must not receive case data")


_UNAVAILABLE_EXECUTION_AUTHORITY = _UnavailableExecutionAuthority()


def _production_execution_authority() -> ManagedExecutionAuthority:
    """Return the closed production adapter.

    Darwin intentionally remains unavailable until a package-owned helper can
    prove the suspended-image, pinned-cwd, nonce-readiness, no-descendant, and
    kill/wait invariants. Linux needs sealed-fd execution plus pidfd/seccomp;
    Windows needs a suspended process admitted to a no-breakaway kill-on-close
    Job Object. Path-based ``Popen`` is not a fallback on any platform.
    """

    return _UNAVAILABLE_EXECUTION_AUTHORITY


def managed_execution_authority_status() -> ManagedExecutionAuthorityStatus:
    """Return a safe capability projection without evaluating case input."""

    try:
        status = _production_execution_authority().status()
    except Exception:
        return _UNAVAILABLE_EXECUTION_AUTHORITY.status()
    if not _authority_status_is_valid(status):
        return _UNAVAILABLE_EXECUTION_AUTHORITY.status()
    return status


def run_managed_process(
    command: Sequence[str],
    *,
    cwd: Path,
    input_bytes: bytes | None,
    timeout_seconds: float,
    expected_executable_sha256: str,
    capture_stdout: bool = False,
    resource_monitor: Callable[[], None] | None = None,
    cwd_fd: int | None = None,
    expected_cwd_device: int | None = None,
    expected_cwd_inode: int | None = None,
) -> ManagedProcessResult:
    """Run a helper only through a complete platform execution authority.

    Capability admission deliberately happens before command, cwd, or stdin is
    copied into an execution request. The production adapter is fail-closed on
    every platform today; callers receive one fixed boundary error and must not
    fall back to an unverified path launch.
    """

    authority = _production_execution_authority()
    try:
        status = authority.status()
    except Exception:
        raise ManagedProcessLaunchError(_AUTHORITY_UNAVAILABLE) from None
    if not _authority_status_is_valid(status) or not status.available:
        raise ManagedProcessLaunchError(_AUTHORITY_UNAVAILABLE)

    request = _validated_execution_request(
        command,
        cwd=cwd,
        input_bytes=input_bytes,
        timeout_seconds=timeout_seconds,
        expected_executable_sha256=expected_executable_sha256,
        capture_stdout=capture_stdout,
        resource_monitor=resource_monitor,
        cwd_fd=cwd_fd,
        expected_cwd_device=expected_cwd_device,
        expected_cwd_inode=expected_cwd_inode,
    )
    try:
        try:
            result = authority.execute(request)
        except (
            ManagedProcessTimeout,
            ManagedProcessLaunchError,
            ManagedProcessTerminationError,
            ManagedProcessOutputLimit,
        ):
            raise
        except Exception:
            raise ManagedProcessLaunchError("managed_process_launch_failed") from None
        return _validated_execution_result(
            result,
            request=request,
            capture_stdout=capture_stdout,
        )
    finally:
        try:
            os.close(request.cwd_fd)
        except OSError:
            pass


def _validated_execution_request(
    command: Sequence[str],
    *,
    cwd: Path,
    input_bytes: bytes | None,
    timeout_seconds: float,
    expected_executable_sha256: str,
    capture_stdout: bool,
    resource_monitor: Callable[[], None] | None,
    cwd_fd: int | None,
    expected_cwd_device: int | None,
    expected_cwd_inode: int | None,
) -> ManagedExecutionRequest:
    try:
        raw_argv = tuple(command)
    except (TypeError, MemoryError):
        raise ManagedProcessLaunchError("managed_process_command_invalid") from None
    if not raw_argv or len(raw_argv) > _MAX_ARGUMENT_COUNT:
        raise ManagedProcessLaunchError("managed_process_command_invalid")
    if any(not isinstance(value, str) or not value or "\x00" in value for value in raw_argv):
        raise ManagedProcessLaunchError("managed_process_command_invalid")
    try:
        argument_bytes = sum(len(value.encode("utf-8")) + 1 for value in raw_argv)
    except (UnicodeError, MemoryError):
        raise ManagedProcessLaunchError("managed_process_command_invalid") from None
    if argument_bytes > _MAX_ARGUMENT_BYTES:
        raise ManagedProcessLaunchError("managed_process_command_invalid")

    executable = raw_argv[0]
    if not os.path.isabs(executable):
        raise ManagedProcessLaunchError("managed_process_executable_invalid")
    cwd_text = os.fspath(cwd) if isinstance(cwd, (str, os.PathLike)) else ""
    if not cwd_text or "\x00" in cwd_text or not os.path.isabs(cwd_text):
        raise ManagedProcessLaunchError("managed_process_workdir_invalid")
    if os.path.abspath(cwd_text) != cwd_text or os.path.normpath(cwd_text) != cwd_text:
        raise ManagedProcessLaunchError("managed_process_workdir_invalid")

    expected_digest = str(expected_executable_sha256 or "").strip().lower()
    if len(expected_digest) != _SHA256_HEX_LENGTH or any(
        character not in "0123456789abcdef" for character in expected_digest
    ):
        raise ManagedProcessLaunchError("managed_process_executable_identity_invalid")
    if input_bytes is not None and not isinstance(input_bytes, bytes):
        raise ManagedProcessLaunchError("managed_process_input_invalid")
    if input_bytes is not None and len(input_bytes) > _MAX_INPUT_BYTES:
        raise ManagedProcessLaunchError("managed_process_input_limit")
    try:
        timeout = float(timeout_seconds)
    except (TypeError, ValueError, OverflowError):
        raise ManagedProcessLaunchError("managed_process_timeout_invalid") from None
    if not math.isfinite(timeout) or timeout <= 0:
        raise ManagedProcessLaunchError("managed_process_timeout_invalid")

    environment, environment_sha256 = _closed_execution_environment(executable)

    pinned_cwd_fd = -1
    try:
        pinned_cwd_fd, cwd_stat = _open_managed_cwd_descriptor(
            cwd_text,
            supplied_fd=cwd_fd,
            expected_device=expected_cwd_device,
            expected_inode=expected_cwd_inode,
        )
        request = ManagedExecutionRequest(
            protocol_version=_AUTHORITY_PROTOCOL_VERSION,
            launch_nonce=secrets.token_hex(32),
            argv=raw_argv,
            cwd=Path(cwd_text),
            cwd_fd=pinned_cwd_fd,
            cwd_device=int(cwd_stat.st_dev),
            cwd_inode=int(cwd_stat.st_ino),
            input_bytes=input_bytes,
            timeout_seconds=timeout,
            expected_executable_sha256=expected_digest,
            environment=environment,
            environment_sha256=environment_sha256,
            capture_stdout=bool(capture_stdout),
            resource_monitor=resource_monitor,
        )
        pinned_cwd_fd = -1
        return request
    finally:
        if pinned_cwd_fd >= 0:
            os.close(pinned_cwd_fd)


def _open_managed_cwd_descriptor(
    cwd_text: str,
    *,
    supplied_fd: int | None,
    expected_device: int | None,
    expected_inode: int | None,
) -> tuple[int, os.stat_result]:
    if (expected_device is None) != (expected_inode is None):
        raise ManagedProcessLaunchError("managed_process_workdir_identity_invalid")
    if expected_device is not None and (
        not isinstance(expected_device, int)
        or isinstance(expected_device, bool)
        or expected_device < 0
        or not isinstance(expected_inode, int)
        or isinstance(expected_inode, bool)
        or expected_inode < 0
    ):
        raise ManagedProcessLaunchError("managed_process_workdir_identity_invalid")
    if supplied_fd is not None and (
        not isinstance(supplied_fd, int)
        or isinstance(supplied_fd, bool)
        or supplied_fd < 0
    ):
        raise ManagedProcessLaunchError("managed_process_workdir_identity_invalid")

    path_fd = -1
    pinned_fd = -1
    transferred = False
    try:
        path_fd = _open_absolute_directory_no_symlinks(cwd_text)
        path_stat = os.fstat(path_fd)
        if supplied_fd is None:
            pinned_fd = path_fd
            path_fd = -1
        else:
            pinned_fd = os.open(
                ".",
                os.O_RDONLY
                | int(getattr(os, "O_CLOEXEC", 0))
                | int(getattr(os, "O_NOFOLLOW", 0))
                | int(getattr(os, "O_DIRECTORY", 0)),
                dir_fd=supplied_fd,
            )
        pinned_stat = os.fstat(pinned_fd)
        if (
            not _same_file_identity(path_stat, pinned_stat)
            or (expected_device is not None and int(pinned_stat.st_dev) != expected_device)
            or (expected_inode is not None and int(pinned_stat.st_ino) != expected_inode)
        ):
            raise OSError
        transferred = True
        return pinned_fd, pinned_stat
    except ManagedProcessLaunchError:
        raise
    except (OSError, TypeError, ValueError):
        raise ManagedProcessLaunchError("managed_process_workdir_identity_invalid") from None
    finally:
        if path_fd >= 0:
            os.close(path_fd)
        if pinned_fd >= 0 and not transferred:
            os.close(pinned_fd)


def _open_absolute_directory_no_symlinks(path_text: str) -> int:
    path = Path(path_text)
    parts = path.parts
    if not parts or any(part in {"", ".", ".."} for part in parts[1:]):
        raise ManagedProcessLaunchError("managed_process_workdir_invalid")
    flags = (
        os.O_RDONLY
        | int(getattr(os, "O_CLOEXEC", 0))
        | int(getattr(os, "O_NOFOLLOW", 0))
        | int(getattr(os, "O_DIRECTORY", 0))
    )
    descriptor = -1
    try:
        descriptor = os.open(path.anchor, flags)
        for part in parts[1:]:
            child = os.open(part, flags, dir_fd=descriptor)
            os.close(descriptor)
            descriptor = child
        return descriptor
    except OSError:
        if descriptor >= 0:
            os.close(descriptor)
        raise ManagedProcessLaunchError("managed_process_workdir_identity_invalid") from None


def _same_file_identity(left: os.stat_result, right: os.stat_result) -> bool:
    return int(left.st_dev) == int(right.st_dev) and int(left.st_ino) == int(right.st_ino)


def _closed_execution_environment(executable: str) -> tuple[tuple[tuple[str, str], ...], str]:
    """Build the exact closed environment consumed by the execution port.

    No ambient value is inherited. Helper-specific increments need their own
    future capability contract; silently copying host locale, temp, credential,
    loader, proxy, or tool configuration into a native helper is forbidden.
    """

    try:
        raw = build_sanitized_subprocess_env(
            {},
            executable=executable,
            platform=sys.platform,
        )
    except (OSError, TypeError, ValueError):
        raise ManagedProcessLaunchError("managed_process_environment_invalid") from None
    if len(raw) > _MAX_ENVIRONMENT_COUNT:
        raise ManagedProcessLaunchError("managed_process_environment_invalid")

    seen: set[str] = set()
    entries: list[tuple[str, str]] = []
    total_bytes = 0
    for name, value in sorted(raw.items(), key=lambda item: item[0].upper()):
        if not isinstance(name, str) or not isinstance(value, str):
            raise ManagedProcessLaunchError("managed_process_environment_invalid")
        canonical_name = name.strip().upper()
        if (
            not canonical_name
            or canonical_name in seen
            or "\x00" in name
            or "\x00" in value
        ):
            raise ManagedProcessLaunchError("managed_process_environment_invalid")
        try:
            name_bytes = name.encode("utf-8", errors="strict")
            value_bytes = value.encode("utf-8", errors="strict")
        except UnicodeError:
            raise ManagedProcessLaunchError("managed_process_environment_invalid") from None
        total_bytes += len(name_bytes) + len(value_bytes) + 2
        if total_bytes > _MAX_ENVIRONMENT_BYTES:
            raise ManagedProcessLaunchError("managed_process_environment_invalid")
        seen.add(canonical_name)
        entries.append((name, value))

    environment = tuple(entries)
    return environment, _execution_environment_sha256(environment)


def _execution_environment_sha256(environment: tuple[tuple[str, str], ...]) -> str:
    digest = hashlib.sha256()
    digest.update(_ENVIRONMENT_DIGEST_DOMAIN)
    for name, value in environment:
        name_bytes = name.encode("utf-8", errors="strict")
        value_bytes = value.encode("utf-8", errors="strict")
        digest.update(len(name_bytes).to_bytes(8, "big"))
        digest.update(name_bytes)
        digest.update(len(value_bytes).to_bytes(8, "big"))
        digest.update(value_bytes)
    return digest.hexdigest()


def _validated_execution_result(
    result: ManagedProcessResult,
    *,
    request: ManagedExecutionRequest,
    capture_stdout: bool,
) -> ManagedProcessResult:
    if not isinstance(result, ManagedProcessResult):
        raise ManagedProcessLaunchError("managed_process_result_invalid")
    if not isinstance(result.returncode, int) or isinstance(result.returncode, bool):
        raise ManagedProcessLaunchError("managed_process_result_invalid")
    if not isinstance(result.stdout, bytes):
        raise ManagedProcessLaunchError("managed_process_result_invalid")
    if len(result.stdout) > _MAX_CAPTURED_STDOUT_BYTES:
        raise ManagedProcessOutputLimit("managed_process_stdout_limit")
    if not capture_stdout and result.stdout:
        raise ManagedProcessLaunchError("managed_process_result_invalid")
    receipt = result.execution_receipt
    if (
        not isinstance(receipt, ManagedExecutionReceipt)
        or receipt.protocol_version != request.protocol_version
        or receipt.launch_nonce != request.launch_nonce
        or receipt.cwd_device != request.cwd_device
        or receipt.cwd_inode != request.cwd_inode
        or receipt.executable_sha256 != request.expected_executable_sha256
        or receipt.environment_sha256 != request.environment_sha256
    ):
        raise ManagedProcessLaunchError("managed_process_execution_receipt_invalid")
    return result


def _authority_status_is_valid(status: object) -> bool:
    if not isinstance(status, ManagedExecutionAuthorityStatus):
        return False
    if status.protocol_version != _AUTHORITY_PROTOCOL_VERSION:
        return False
    if status.platform != sys.platform:
        return False
    if status.available:
        return status.reason_code == "" and status.guarantees == _required_authority_guarantees(sys.platform)
    return status.reason_code == _AUTHORITY_UNAVAILABLE and not status.guarantees


def _required_authority_guarantees(platform: str) -> frozenset[str]:
    normalized = str(platform or "").lower()
    common = {
        "bounded_stdio",
        "closed_sanitized_environment",
        "kill_and_wait_every_terminal_path",
        "nonce_ready_before_case_data",
        "no_descendant_before_case_data",
        "opened_cwd_fd_identity",
        "opened_executable_fd_identity",
    }
    if normalized == "darwin":
        return frozenset(
            common
            | {
                "csops_loaded_cdhash_matches_open_fd_codedirectory",
                "posix_spawn_fchdir",
                "posix_spawn_start_suspended",
                "rlimit_nproc_zero_before_case_data",
            }
        )
    if normalized.startswith("linux"):
        return frozenset(
            common
            | {
                "execveat_sealed_executable_fd",
                "fchdir_opened_cwd_fd",
                "pidfd_lifecycle",
                "seccomp_blocks_descendant_creation",
            }
        )
    if normalized.startswith("win"):
        return frozenset(
            common
            | {
                "create_suspended",
                "job_active_process_limit_one",
                "job_kill_on_close_no_breakaway",
                "locked_executable_and_cwd_handles",
            }
        )
    return frozenset(common | {"unsupported_platform_fail_closed"})
