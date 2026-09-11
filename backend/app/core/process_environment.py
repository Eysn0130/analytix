from __future__ import annotations

import ctypes
import os
import re
import sys
from collections.abc import Iterable, Mapping
from os import PathLike


_DATA_ANALYSIS_ENV_PREFIX = "ANALYTIX_DATA_ANALYSIS_"
_SENSITIVE_DATA_ANALYSIS_ENV_TOKENS = frozenset(
    {
        "AUTH",
        "CHALLENGE",
        "CREDENTIAL",
        "CREDENTIALS",
        "FILE",
        "KEY",
        "PASSWORD",
        "PROOF",
        "SECRET",
        "TOKEN",
    }
)

_COMMON_SUBPROCESS_ENVIRONMENT_NAMES = frozenset(
    {
        "PATH",
        "LANG",
        "LANGUAGE",
        "LC_ADDRESS",
        "LC_ALL",
        "LC_COLLATE",
        "LC_CTYPE",
        "LC_IDENTIFICATION",
        "LC_MEASUREMENT",
        "LC_MESSAGES",
        "LC_MONETARY",
        "LC_NAME",
        "LC_NUMERIC",
        "LC_PAPER",
        "LC_TELEPHONE",
        "LC_TIME",
        "TZ",
    }
)
_WINDOWS_SUBPROCESS_ENVIRONMENT_NAMES: frozenset[str] = frozenset()
_DARWIN_SUBPROCESS_ENVIRONMENT_NAMES = frozenset({"__CF_USER_TEXT_ENCODING"})

_ALWAYS_DENIED_SUBPROCESS_ENVIRONMENT_PREFIXES = (
    "AWS",
    "GITHUB",
    "NPM",
)
_ALWAYS_DENIED_SUBPROCESS_ENVIRONMENT_NAMES = frozenset(
    {
        "CI_JOB_JWT",
        "CI_JOB_JWT_V2",
        "DBUS_SESSION_BUS_ADDRESS",
        "DOCKER_HOST",
        "GPG_AGENT_INFO",
        "KUBECONFIG",
        "NODE_AUTH_TOKEN",
        "SSH_AGENT_PID",
        "SSH_AUTH_SOCK",
        "VAULT_ADDR",
        "WAYLAND_DISPLAY",
        "XDG_RUNTIME_DIR",
    }
)
_ALWAYS_DENIED_SUBPROCESS_ENVIRONMENT_TOKENS = frozenset(
    {
        "FILE",
        "KEY",
        "PASSWORD",
        "PROXY",
        "SECRET",
        "SOCK",
        "SOCKET",
        "TOKEN",
    }
)
_ENVIRONMENT_NAME_TOKEN_RE = re.compile(r"[^A-Z0-9]+")
_POSIX_TRUSTED_EXECUTABLE_PATH = "/usr/bin:/bin"
_DARWIN_TRUSTED_EXECUTABLE_PATH = "/usr/bin:/bin:/usr/sbin:/sbin"


def is_sensitive_data_analysis_environment_name(name: str) -> bool:
    normalized = str(name or "").strip().upper()
    if not normalized.startswith(_DATA_ANALYSIS_ENV_PREFIX):
        return False
    suffix = normalized.removeprefix(_DATA_ANALYSIS_ENV_PREFIX)
    tokens = frozenset(suffix.split("_"))
    return suffix == "LAUNCH_ID" or bool(tokens & _SENSITIVE_DATA_ANALYSIS_ENV_TOKENS)


def clear_sensitive_data_analysis_environment() -> None:
    for name in tuple(os.environ):
        if is_sensitive_data_analysis_environment_name(name):
            os.environ.pop(name, None)


def is_forbidden_subprocess_environment_name(name: str) -> bool:
    normalized = str(name or "").strip().upper()
    if not normalized:
        return True
    if normalized in _ALWAYS_DENIED_SUBPROCESS_ENVIRONMENT_NAMES:
        return True
    if normalized.startswith(_ALWAYS_DENIED_SUBPROCESS_ENVIRONMENT_PREFIXES):
        return True
    tokens = frozenset(
        token
        for token in _ENVIRONMENT_NAME_TOKEN_RE.split(normalized)
        if token
    )
    return bool(tokens & _ALWAYS_DENIED_SUBPROCESS_ENVIRONMENT_TOKENS)


def build_sanitized_subprocess_env(
    source: Mapping[str, str] | None = None,
    *,
    executable: str | PathLike[str] | None = None,
    additional_allowed_names: Iterable[str] = (),
    safe_temp_directory: str | PathLike[str] | None = None,
    platform: str | None = None,
) -> dict[str, str]:
    """Build a child environment from a closed platform baseline.

    ``additional_allowed_names`` is an executable-specific increment. It is
    deliberately unavailable unless the caller also identifies the executable,
    and secret/proxy/capability names cannot be opted back in.
    """

    inherited = os.environ if source is None else source
    executable_text = os.fspath(executable).strip() if executable is not None else ""
    if "\x00" in executable_text:
        raise ValueError("subprocess executable contains NUL")

    additional_names: set[str] = set()
    for raw_name in additional_allowed_names:
        normalized = str(raw_name or "").strip().upper()
        if not normalized:
            raise ValueError("subprocess environment allowlist contains an empty name")
        if is_forbidden_subprocess_environment_name(normalized):
            raise ValueError(f"forbidden subprocess environment name cannot be allowed: {normalized}")
        additional_names.add(normalized)
    if additional_names and not executable_text:
        raise ValueError("executable is required for subprocess environment increments")

    normalized_platform = str(platform or sys.platform).strip().lower()
    platform_names: frozenset[str] = frozenset()
    if normalized_platform.startswith("win"):
        platform_names = _WINDOWS_SUBPROCESS_ENVIRONMENT_NAMES
    elif normalized_platform == "darwin":
        platform_names = _DARWIN_SUBPROCESS_ENVIRONMENT_NAMES
    allowed_names = _COMMON_SUBPROCESS_ENVIRONMENT_NAMES | platform_names | additional_names
    seen_names: set[str] = set()
    trusted_path, fixed_platform_environment = _trusted_platform_environment(normalized_platform)
    result: dict[str, str] = {"PATH": trusted_path, **fixed_platform_environment}
    if safe_temp_directory is not None:
        temporary = _validated_safe_temp_directory(safe_temp_directory)
        result.update({"TEMP": temporary, "TMP": temporary, "TMPDIR": temporary})
    for raw_name, raw_value in inherited.items():
        name = str(raw_name)
        normalized = name.strip().upper()
        if not normalized:
            continue
        if normalized in seen_names:
            raise ValueError(f"duplicate case-insensitive subprocess environment name: {normalized}")
        seen_names.add(normalized)
        if normalized == "PATH":
            continue
        if normalized in fixed_platform_environment:
            continue
        if normalized not in allowed_names or is_forbidden_subprocess_environment_name(normalized):
            continue
        value = str(raw_value)
        if "\x00" in name or "\x00" in value:
            raise ValueError(f"subprocess environment contains NUL: {normalized}")
        result[name] = value
    return result


def _validated_safe_temp_directory(value: str | PathLike[str]) -> str:
    raw = os.fspath(value).strip()
    if not raw or "\x00" in raw or not os.path.isabs(raw):
        raise ValueError("safe subprocess temp directory is invalid")
    absolute = os.path.abspath(raw)
    canonical = os.path.realpath(raw)
    if os.path.normcase(absolute) != os.path.normcase(canonical):
        raise ValueError("safe subprocess temp directory traverses a symlink")
    try:
        os.lstat(canonical)
    except OSError:
        raise ValueError("safe subprocess temp directory is unavailable") from None
    if not os.path.isdir(canonical) or os.path.islink(canonical):
        raise ValueError("safe subprocess temp directory is not a regular directory")
    return canonical


def _trusted_platform_environment(platform_name: str) -> tuple[str, dict[str, str]]:
    if platform_name == "darwin":
        return _DARWIN_TRUSTED_EXECUTABLE_PATH, {}
    if platform_name.startswith("win"):
        system_root = _trusted_windows_directory()
        drive = system_root[:2]
        return (
            f"{system_root}\\System32;{system_root}",
            {
                "SYSTEMDRIVE": drive,
                "SYSTEMROOT": system_root,
                "WINDIR": system_root,
            },
        )
    return _POSIX_TRUSTED_EXECUTABLE_PATH, {}


def _trusted_windows_directory() -> str:
    if not sys.platform.lower().startswith("win"):
        return r"C:\Windows"
    kernel32 = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel32.GetWindowsDirectoryW.argtypes = [ctypes.c_wchar_p, ctypes.c_uint]
    kernel32.GetWindowsDirectoryW.restype = ctypes.c_uint
    buffer = ctypes.create_unicode_buffer(32768)
    length = int(kernel32.GetWindowsDirectoryW(buffer, len(buffer)))
    if length <= 0 or length >= len(buffer):
        raise ValueError("trusted Windows directory is unavailable")
    candidate = str(buffer.value or "").strip().rstrip("\\/")
    if not re.fullmatch(r"[A-Za-z]:\\[^\x00\r\n]+", candidate):
        raise ValueError("trusted Windows directory is invalid")
    return candidate
