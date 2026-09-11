from __future__ import annotations

import hashlib
import json
import os
import platform
import re
import stat
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Collection


_SHA256_RE = re.compile(r"^[0-9a-fA-F]{64}$")
_ARCHIVE_MANIFEST_NAME = "analytix-archive-extractor-manifest.json"
_DOCUMENT_MANIFEST_NAME = "analytix-document-runtime-manifest.json"
_DOCUMENT_AUTHORITY_LOCK_NAME = "analytix-document-runtime-authority-lock.json"
_DOCUMENT_AUTHORITY_LOCK_CONTRACT = "analytix.document-runtime-authority-lock/v1"
_DOCUMENT_AUTHORITY_LOCK_SHA256 = "d1fa4ec449edbef8d310a2fdfb8bc236609aacf75b9dd6cb295c4b374d0226f3"
_ARCHIVE_MANIFEST_KEYS = frozenset(
    {
        "schemaVersion",
        "tool",
        "packageName",
        "packageVersion",
        "source",
        "target",
        "sha256",
        "generatedAt",
    }
)
_DOCUMENT_MANIFEST_KEYS = frozenset(
    {
        "schemaVersion",
        "contract",
        "target",
        "sources",
        "licenses",
        "capabilities",
        "files",
        "treeSha256",
    }
)
_DOCUMENT_TARGET_KEYS = frozenset({"key", "platform", "arch", "triple"})
_DOCUMENT_SOURCE_KEYS = frozenset({"id", "name", "version", "sourceUri", "archiveSha256"})
_DOCUMENT_LICENSE_KEYS = frozenset({"id", "spdxExpression", "sourceId", "path"})
_DOCUMENT_CAPABILITY_KEYS = frozenset(
    {"sofficeBinary", "tesseractBinary", "tessdataDirectory", "ocrLanguages"}
)
_DOCUMENT_FILE_KEYS = frozenset(
    {"path", "kind", "sourceId", "licenseId", "sha256", "byteLength", "platformSignature", "dependencies"}
)
_DOCUMENT_CONTENT_SIGNATURE_KEYS = frozenset({"kind", "targetKey"})
_DOCUMENT_NATIVE_SIGNATURE_KEYS = frozenset(
    {"kind", "targetKey", "format", "arch", "payloadSha256", "payloadByteLength"}
)
_DOCUMENT_TREE_DOMAIN = b"AnalytixDocumentRuntimeTreeV2\0"
_DOCUMENT_SOURCE_SET_DOMAIN = b"AnalytixDocumentRuntimeSourceSetV1\0"
_DOCUMENT_LICENSE_SET_DOMAIN = b"AnalytixDocumentRuntimeLicenseSetV1\0"
_DOCUMENT_CONTRACT = "analytix.document-runtime/v2"
_DOCUMENT_RUNTIME_DIRECTORY = "document-runtime"
_DOCUMENT_ID_RE = re.compile(r"^[a-z0-9][a-z0-9._-]{0,127}$")
_DOCUMENT_SPDX_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9.+() -]{0,127}$")
_DOCUMENT_LANGUAGE_RE = re.compile(r"^[A-Za-z0-9_.+-]{1,64}$")
_DOCUMENT_AUTHORITY_LOCK_KEYS = frozenset({"schemaVersion", "contract", "targets"})
_DOCUMENT_AUTHORITY_TARGET_KEYS = frozenset(
    {"targetKey", "assetReleaseId", "manifestSha256", "treeSha256", "sourceSetSha256", "licenseSetSha256"}
)
_DOCUMENT_MAX_NATIVE_BYTES = 256 * 1024 * 1024
_MACH_O_DYLIB_COMMANDS = frozenset({0x0C, 0x80000018, 0x8000001F, 0x20, 0x80000023})
_MACH_O_RPATH_COMMAND = 0x8000001C
_MACH_O_SYSTEM_PREFIXES = (
    "/usr/lib/",
    "/System/Library/Frameworks/",
    "/System/Library/PrivateFrameworks/",
)
_WINDOWS_SYSTEM_DLLS = frozenset(
    {
        "advapi32.dll", "bcrypt.dll", "cabinet.dll", "comctl32.dll", "comdlg32.dll",
        "crypt32.dll", "dwmapi.dll", "gdi32.dll", "imm32.dll", "kernel32.dll",
        "msimg32.dll", "netapi32.dll", "ntdll.dll", "ole32.dll", "oleaut32.dll",
        "rpcrt4.dll", "secur32.dll", "setupapi.dll", "shell32.dll", "shlwapi.dll",
        "user32.dll", "userenv.dll", "version.dll", "winhttp.dll", "wininet.dll",
        "winmm.dll", "ws2_32.dll",
    }
)


class ExternalRuntimeError(ValueError):
    """A fixed-code external-runtime failure safe for persistence and APIs."""

    def __init__(self, code: str) -> None:
        normalized = str(code or "external_runtime_failed").strip() or "external_runtime_failed"
        self.code = normalized
        super().__init__(normalized)


@dataclass(frozen=True)
class HostRuntimeResolution:
    path: Path | None
    reason: str
    sha256: str = ""


def trusted_app_resource_root() -> Path:
    # In development this is the repository root. In packaged Python resources
    # the same relative layout resolves to the Electron resources root.
    return Path(__file__).resolve().parents[3]


def resolve_host_runtime_binary(
    env_name: str,
    *,
    allowed_names: Collection[str],
    require_archive_manifest: bool = False,
    require_document_manifest: bool = False,
) -> HostRuntimeResolution:
    if require_archive_manifest and require_document_manifest:
        return HostRuntimeResolution(None, "HOST_RUNTIME_INTEGRITY_FAILED")
    raw = str(os.environ.get(env_name) or "").strip()
    if not raw:
        return HostRuntimeResolution(None, "HOST_RUNTIME_NOT_CONFIGURED")
    if "\x00" in raw:
        return HostRuntimeResolution(None, "HOST_RUNTIME_UNTRUSTED")

    raw_path = Path(raw).expanduser()
    if not raw_path.is_absolute():
        return HostRuntimeResolution(None, "HOST_RUNTIME_UNTRUSTED")

    try:
        resource_root = trusted_app_resource_root().resolve(strict=True)
        runtime_root = resource_root / "runtime"
        candidate = Path(os.path.abspath(raw_path))
        canonical = candidate.resolve(strict=True)
    except (OSError, RuntimeError):
        return HostRuntimeResolution(None, "HOST_RUNTIME_UNTRUSTED")

    if os.path.normcase(os.fspath(candidate)) != os.path.normcase(os.fspath(canonical)):
        return HostRuntimeResolution(None, "HOST_RUNTIME_UNTRUSTED")
    if require_document_manifest:
        document_root = runtime_root / _DOCUMENT_RUNTIME_DIRECTORY
        try:
            canonical.relative_to(document_root)
        except ValueError:
            return HostRuntimeResolution(None, "HOST_RUNTIME_UNTRUSTED")
    elif canonical.parent != runtime_root:
        return HostRuntimeResolution(None, "HOST_RUNTIME_UNTRUSTED")
    if canonical.name.lower() not in {name.lower() for name in allowed_names}:
        return HostRuntimeResolution(None, "HOST_RUNTIME_UNTRUSTED")
    if not _path_is_regular_without_symlink(canonical, resource_root):
        return HostRuntimeResolution(None, "HOST_RUNTIME_UNTRUSTED")
    if os.name != "nt" and not os.access(canonical, os.X_OK):
        return HostRuntimeResolution(None, "HOST_RUNTIME_UNTRUSTED")
    expected_sha256 = ""
    if require_archive_manifest:
        expected_sha256 = _archive_manifest_sha256(runtime_root, canonical)
    elif require_document_manifest:
        expected_sha256 = _document_manifest_sha256(runtime_root, canonical)
    if (require_archive_manifest or require_document_manifest) and not expected_sha256:
        return HostRuntimeResolution(None, "HOST_RUNTIME_INTEGRITY_FAILED")
    return HostRuntimeResolution(canonical, "", expected_sha256)


def _path_is_regular_without_symlink(path: Path, boundary_root: Path) -> bool:
    try:
        boundary = Path(os.path.abspath(boundary_root))
        candidate = Path(os.path.abspath(path))
        relative = candidate.relative_to(boundary)
        current = boundary
        boundary_stat = current.lstat()
        if not stat.S_ISDIR(boundary_stat.st_mode) or stat.S_ISLNK(boundary_stat.st_mode):
            return False
        for part in relative.parts:
            current = current / part
            current_stat = current.lstat()
            if stat.S_ISLNK(current_stat.st_mode):
                return False
        final_stat = candidate.lstat()
        return stat.S_ISREG(final_stat.st_mode) and not stat.S_ISLNK(final_stat.st_mode)
    except (OSError, ValueError):
        return False


def _archive_manifest_sha256(runtime_root: Path, binary: Path) -> str:
    manifest_path = runtime_root / _ARCHIVE_MANIFEST_NAME
    if not _path_is_regular_without_symlink(manifest_path, runtime_root.parent):
        return ""
    try:
        payload = _read_strict_manifest(manifest_path)
    except (OSError, UnicodeError, json.JSONDecodeError, ValueError):
        return ""
    if (
        not isinstance(payload, dict)
        or frozenset(payload) != _ARCHIVE_MANIFEST_KEYS
        or type(payload.get("schemaVersion")) is not int
        or payload.get("schemaVersion") != 1
    ):
        return ""
    for key in ("tool", "packageName", "packageVersion", "source", "target", "generatedAt"):
        if not isinstance(payload.get(key), str) or not str(payload[key]).strip():
            return ""
    expected = str(payload.get("sha256") or "").strip().lower()
    if not _SHA256_RE.fullmatch(expected):
        return ""
    tool = str(payload.get("tool") or "").strip().lower()
    if tool not in {"7z", "7za", "7zz", "bsdtar", "tar"}:
        return ""
    if binary.stem.lower() != tool:
        return ""
    return expected if _sha256_regular_file(binary) == expected else ""


def _document_manifest_sha256(runtime_root: Path, binary: Path) -> str:
    document_root = runtime_root / _DOCUMENT_RUNTIME_DIRECTORY
    manifest_path = document_root / _DOCUMENT_MANIFEST_NAME
    if not _path_is_regular_without_symlink(manifest_path, runtime_root.parent):
        return ""
    try:
        payload = _read_strict_manifest(manifest_path)
    except (OSError, UnicodeError, json.JSONDecodeError, ValueError):
        return ""
    target = _document_runtime_target()
    if not _valid_document_manifest(payload, target):
        return ""
    canonical_payload = json.dumps(payload, ensure_ascii=False, separators=(",", ":"))
    try:
        if manifest_path.read_text(encoding="utf-8") != canonical_payload:
            return ""
    except (OSError, UnicodeError):
        return ""
    manifest_sha256, _manifest_size = _regular_file_identity(manifest_path)
    if not manifest_sha256 or not _document_authority_lock_accepts(
        document_root,
        payload,
        manifest_sha256,
    ):
        return ""
    expected_files = {
        _DOCUMENT_MANIFEST_NAME,
        _DOCUMENT_AUTHORITY_LOCK_NAME,
        *(str(item["path"]) for item in payload["files"]),
    }
    actual_inventory = _document_tree_inventory(document_root)
    if actual_inventory is None:
        return ""
    actual_files, actual_directories = actual_inventory
    expected_directories = _expected_document_directories(expected_files)
    if actual_files != expected_files or actual_directories != expected_directories:
        return ""
    by_path: dict[str, dict[str, object]] = {}
    for entry in payload["files"]:
        entry_path = document_root.joinpath(*str(entry["path"]).split("/"))
        if not _path_is_regular_without_symlink(entry_path, runtime_root.parent):
            return ""
        digest, size = _regular_file_identity(entry_path)
        if digest != entry["sha256"] or size != entry["byteLength"]:
            return ""
        if entry["kind"] in {"binary", "library"} and not _native_header_matches(
            entry_path,
            target,
        ):
            return ""
        by_path[str(entry["path"])] = entry
    if not _verify_document_native_dependencies(document_root, payload, target):
        return ""
    try:
        relative_binary = binary.relative_to(document_root).as_posix()
    except ValueError:
        return ""
    capabilities = payload["capabilities"]
    if relative_binary not in {
        capabilities["sofficeBinary"],
        capabilities["tesseractBinary"],
    }:
        return ""
    matched = by_path.get(relative_binary)
    if not matched or matched.get("kind") != "binary":
        return ""
    if relative_binary == capabilities["tesseractBinary"]:
        expected_tessdata = document_root.joinpath(*str(capabilities["tessdataDirectory"]).split("/"))
        configured_tessdata = str(os.environ.get("TESSDATA_PREFIX") or "").strip()
        if not configured_tessdata or os.path.normcase(os.path.abspath(configured_tessdata)) != os.path.normcase(
            os.fspath(expected_tessdata)
        ):
            return ""
    return str(matched["sha256"])


def _expected_document_directories(paths: Collection[str]) -> set[str]:
    expected: set[str] = set()
    for value in paths:
        parts = Path(value).parts[:-1]
        for index in range(1, len(parts) + 1):
            expected.add(Path(*parts[:index]).as_posix())
    return expected


def _document_tree_inventory(root: Path) -> tuple[set[str], set[str]] | None:
    files: set[str] = set()
    directories: set[str] = set()
    try:
        for current_text, directory_names, file_names in os.walk(root, topdown=True, followlinks=False):
            current = Path(current_text)
            for name in directory_names:
                path = current / name
                item_stat = path.lstat()
                if stat.S_ISLNK(item_stat.st_mode) or not stat.S_ISDIR(item_stat.st_mode):
                    return None
                directories.add(path.relative_to(root).as_posix())
            for name in file_names:
                path = current / name
                item_stat = path.lstat()
                if stat.S_ISLNK(item_stat.st_mode) or not stat.S_ISREG(item_stat.st_mode):
                    return None
                files.add(path.relative_to(root).as_posix())
    except (OSError, ValueError):
        return None
    return files, directories


def _valid_document_manifest(payload: object, target: dict[str, str]) -> bool:
    if (
        not isinstance(payload, dict)
        or frozenset(payload) != _DOCUMENT_MANIFEST_KEYS
        or type(payload.get("schemaVersion")) is not int
        or payload.get("schemaVersion") != 2
        or payload.get("contract") != _DOCUMENT_CONTRACT
        or not _exact_string_mapping(payload.get("target"), _DOCUMENT_TARGET_KEYS)
        or payload.get("target") != target
        or not isinstance(payload.get("sources"), list)
        or not payload["sources"]
        or not isinstance(payload.get("licenses"), list)
        or not payload["licenses"]
        or not isinstance(payload.get("capabilities"), dict)
        or frozenset(payload["capabilities"]) != _DOCUMENT_CAPABILITY_KEYS
        or not isinstance(payload.get("files"), list)
        or len(payload["files"]) < 5
        or not isinstance(payload.get("treeSha256"), str)
        or not _SHA256_RE.fullmatch(payload["treeSha256"])
    ):
        return False
    sources = payload["sources"]
    licenses = payload["licenses"]
    files = payload["files"]
    if sources != sorted(sources, key=lambda item: str(item.get("id") if isinstance(item, dict) else "")):
        return False
    if licenses != sorted(licenses, key=lambda item: str(item.get("id") if isinstance(item, dict) else "")):
        return False
    if files != sorted(files, key=lambda item: str(item.get("path") if isinstance(item, dict) else "")):
        return False
    source_ids: set[str] = set()
    for source in sources:
        if (
            not _exact_string_mapping(source, _DOCUMENT_SOURCE_KEYS)
            or not _DOCUMENT_ID_RE.fullmatch(source["id"])
            or not _SHA256_RE.fullmatch(source["archiveSha256"])
            or not source["name"].strip()
            or not source["version"].strip()
            or not source["sourceUri"].startswith(("https://", "git+https://"))
            or source["id"] in source_ids
        ):
            return False
        source_ids.add(source["id"])
    license_ids: set[str] = set()
    for license_entry in licenses:
        if (
            not _exact_string_mapping(license_entry, _DOCUMENT_LICENSE_KEYS)
            or not _DOCUMENT_ID_RE.fullmatch(license_entry["id"])
            or not _DOCUMENT_SPDX_RE.fullmatch(license_entry["spdxExpression"])
            or license_entry["sourceId"] not in source_ids
            or not _valid_relative_path(license_entry["path"])
            or license_entry["id"] in license_ids
        ):
            return False
        license_ids.add(license_entry["id"])
    file_paths: set[str] = set()
    by_path: dict[str, dict[str, object]] = {}
    for entry in files:
        if (
            not isinstance(entry, dict)
            or frozenset(entry) != _DOCUMENT_FILE_KEYS
            or not _valid_relative_path(entry.get("path"))
            or entry.get("kind") not in {"binary", "library", "data", "license"}
            or entry.get("sourceId") not in source_ids
            or entry.get("licenseId") not in license_ids
            or not isinstance(entry.get("sha256"), str)
            or not _SHA256_RE.fullmatch(entry["sha256"])
            or type(entry.get("byteLength")) is not int
            or entry["byteLength"] <= 0
            or str(entry["path"]).lower() in file_paths
            or not _valid_document_platform_signature(entry, target)
            or not isinstance(entry.get("dependencies"), list)
            or any(not _valid_relative_path(dependency) for dependency in entry["dependencies"])
            or entry["dependencies"] != sorted(entry["dependencies"])
            or len(set(entry["dependencies"])) != len(entry["dependencies"])
            or (entry["kind"] not in {"binary", "library"} and entry["dependencies"])
        ):
            return False
        file_paths.add(str(entry["path"]).lower())
        by_path[str(entry["path"])] = entry
    for license_entry in licenses:
        license_file = by_path.get(license_entry["path"])
        if not license_file or license_file.get("kind") != "license" or license_file.get("sourceId") != license_entry["sourceId"]:
            return False
    capabilities = payload["capabilities"]
    languages = capabilities.get("ocrLanguages")
    if (
        not all(_valid_relative_path(capabilities.get(name)) for name in ("sofficeBinary", "tesseractBinary", "tessdataDirectory"))
        or not isinstance(languages, list)
        or len(languages) < 2
        or languages != sorted(languages)
        or len(set(languages)) != len(languages)
        or not all(isinstance(language, str) and _DOCUMENT_LANGUAGE_RE.fullmatch(language) for language in languages)
        or not {"chi_sim", "eng"}.issubset(languages)
        or by_path.get(capabilities["sofficeBinary"], {}).get("kind") != "binary"
        or by_path.get(capabilities["tesseractBinary"], {}).get("kind") != "binary"
    ):
        return False
    if any(by_path.get(f"{capabilities['tessdataDirectory']}/{language}.traineddata", {}).get("kind") != "data" for language in languages):
        return False
    if any(
        by_path.get(dependency, {}).get("kind") != "library"
        for entry in files
        for dependency in entry["dependencies"]
    ):
        return False
    reachable_libraries: set[str] = set()
    queue = [capabilities["sofficeBinary"], capabilities["tesseractBinary"]]
    while queue:
        current = by_path.get(queue.pop())
        if not current:
            return False
        for dependency in current["dependencies"]:
            if dependency not in reachable_libraries:
                reachable_libraries.add(dependency)
                queue.append(dependency)
    if any(entry["kind"] == "library" and entry["path"] not in reachable_libraries for entry in files):
        return False
    tree_payload = {
        "target": payload["target"],
        "sources": sources,
        "licenses": licenses,
        "capabilities": capabilities,
        "files": files,
    }
    canonical_tree = json.dumps(tree_payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    return hashlib.sha256(_DOCUMENT_TREE_DOMAIN + canonical_tree).hexdigest() == payload["treeSha256"]


def _exact_string_mapping(value: object, keys: frozenset[str]) -> bool:
    return isinstance(value, dict) and frozenset(value) == keys and all(
        isinstance(value.get(key), str) for key in keys
    )


def _valid_relative_path(value: object) -> bool:
    if not isinstance(value, str) or not value or "\\" in value or "\x00" in value or value.endswith("/"):
        return False
    path = Path(value)
    return not path.is_absolute() and value == path.as_posix() and all(part not in {"", ".", ".."} for part in path.parts)


def _valid_document_platform_signature(entry: dict[str, object], target: dict[str, str]) -> bool:
    signature = entry.get("platformSignature")
    if not isinstance(signature, dict):
        return False
    if entry.get("kind") in {"binary", "library"}:
        return (
            frozenset(signature) == _DOCUMENT_NATIVE_SIGNATURE_KEYS
            and signature.get("kind") == "native"
            and signature.get("targetKey") == target.get("key")
            and signature.get("format") == ("mach-o" if target.get("platform") == "darwin" else "pe")
            and signature.get("arch") == target.get("arch")
            and isinstance(signature.get("payloadSha256"), str)
            and bool(_SHA256_RE.fullmatch(str(signature["payloadSha256"])))
            and type(signature.get("payloadByteLength")) is int
            and 0 < int(signature["payloadByteLength"]) <= int(entry["byteLength"])
        )
    return frozenset(signature) == _DOCUMENT_CONTENT_SIGNATURE_KEYS and signature == {
        "kind": "content",
        "targetKey": target.get("key"),
    }


def _document_authority_lock_accepts(
    document_root: Path,
    manifest: dict[str, object],
    manifest_sha256: str,
) -> bool:
    lock_path = document_root / _DOCUMENT_AUTHORITY_LOCK_NAME
    if not _path_is_regular_without_symlink(lock_path, document_root.parent.parent):
        return False
    lock_sha256, _lock_size = _regular_file_identity(lock_path)
    if lock_sha256 != _DOCUMENT_AUTHORITY_LOCK_SHA256:
        return False
    try:
        lock = _read_strict_manifest(lock_path)
        lock_text = lock_path.read_text(encoding="utf-8")
    except (OSError, UnicodeError, json.JSONDecodeError, ValueError):
        return False
    if (
        not isinstance(lock, dict)
        or frozenset(lock) != _DOCUMENT_AUTHORITY_LOCK_KEYS
        or type(lock.get("schemaVersion")) is not int
        or lock.get("schemaVersion") != 1
        or lock.get("contract") != _DOCUMENT_AUTHORITY_LOCK_CONTRACT
        or not isinstance(lock.get("targets"), list)
        or lock_text != json.dumps(lock, ensure_ascii=False, separators=(",", ":"))
    ):
        return False
    targets = lock["targets"]
    if targets != sorted(targets, key=lambda item: str(item.get("targetKey") if isinstance(item, dict) else "")):
        return False
    seen: set[str] = set()
    for entry in targets:
        if (
            not isinstance(entry, dict)
            or frozenset(entry) != _DOCUMENT_AUTHORITY_TARGET_KEYS
            or not all(isinstance(entry.get(key), str) for key in _DOCUMENT_AUTHORITY_TARGET_KEYS)
            or not _DOCUMENT_ID_RE.fullmatch(entry["assetReleaseId"])
            or any(
                not _SHA256_RE.fullmatch(entry[key])
                for key in ("manifestSha256", "treeSha256", "sourceSetSha256", "licenseSetSha256")
            )
            or entry["targetKey"] in seen
        ):
            return False
        seen.add(entry["targetKey"])
    target_key = str(manifest["target"]["key"])
    authorized = next((entry for entry in targets if entry["targetKey"] == target_key), None)
    if not authorized:
        return False
    source_bytes = json.dumps(manifest["sources"], ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    license_bytes = json.dumps(manifest["licenses"], ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    expected = {
        "targetKey": target_key,
        "assetReleaseId": authorized["assetReleaseId"],
        "manifestSha256": manifest_sha256,
        "treeSha256": manifest["treeSha256"],
        "sourceSetSha256": hashlib.sha256(_DOCUMENT_SOURCE_SET_DOMAIN + source_bytes).hexdigest(),
        "licenseSetSha256": hashlib.sha256(_DOCUMENT_LICENSE_SET_DOMAIN + license_bytes).hexdigest(),
    }
    return authorized == expected


def _document_runtime_target() -> dict[str, str]:
    machine = platform.machine().strip().lower()
    if sys.platform == "darwin" and machine in {"arm64", "aarch64"}:
        return {"key": "darwin-arm64", "platform": "darwin", "arch": "arm64", "triple": "aarch64-apple-darwin"}
    if sys.platform.startswith("win") and machine in {"amd64", "x86_64"}:
        return {"key": "win32-x64", "platform": "win32", "arch": "x64", "triple": "x86_64-pc-windows-msvc"}
    return {}


def _native_header_matches(path: Path, target: dict[str, str]) -> bool:
    try:
        with path.open("rb") as handle:
            header = handle.read(512)
        if target.get("platform") == "darwin":
            if len(header) < 8 or header[:4] not in {b"\xcf\xfa\xed\xfe", b"\xfe\xed\xfa\xcf"}:
                return False
            byte_order = "little" if header[:4] == b"\xcf\xfa\xed\xfe" else "big"
            cpu = int.from_bytes(header[4:8], byte_order)
            return cpu == 0x0100000C and target.get("arch") == "arm64"
        if target.get("platform") == "win32":
            if len(header) < 0x40 or header[:2] != b"MZ":
                return False
            pe_offset = int.from_bytes(header[0x3C:0x40], "little")
            if pe_offset + 6 > len(header) or header[pe_offset:pe_offset + 4] != b"PE\0\0":
                return False
            machine = int.from_bytes(header[pe_offset + 4:pe_offset + 6], "little")
            return machine == 0x8664 and target.get("arch") == "x64"
    except OSError:
        return False
    return False


def _read_native_bounded(path: Path) -> bytes:
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
            or (int(before.st_dev), int(before.st_ino)) != (int(opened.st_dev), int(opened.st_ino))
            or int(opened.st_size) <= 0
            or int(opened.st_size) > _DOCUMENT_MAX_NATIVE_BYTES
            or bool(int(opened.st_mode) & 0o022)
        ):
            raise OSError
        remaining = int(opened.st_size)
        chunks: list[bytes] = []
        while remaining:
            chunk = os.read(descriptor, min(1024 * 1024, remaining))
            if not chunk:
                raise OSError
            chunks.append(chunk)
            remaining -= len(chunk)
        if os.read(descriptor, 1):
            raise OSError
        after = path.lstat()
        after_opened = os.fstat(descriptor)
        if (
            (int(after.st_dev), int(after.st_ino)) != (int(opened.st_dev), int(opened.st_ino))
            or int(after_opened.st_size) != int(opened.st_size)
            or int(getattr(after_opened, "st_mtime_ns", 0)) != int(getattr(opened, "st_mtime_ns", 0))
        ):
            raise OSError
        return b"".join(chunks)
    finally:
        if descriptor >= 0:
            os.close(descriptor)


def _bounded_c_string(payload: bytes, start: int, end: int) -> str:
    if start < 0 or start >= end or end > len(payload):
        raise ValueError
    terminator = payload.find(b"\0", start, end)
    if terminator <= start or terminator - start > 4096:
        raise ValueError
    raw = payload[start:terminator]
    value = raw.decode("utf-8", errors="strict")
    if not value or len(value.encode("utf-8")) != len(raw) or any(ord(character) < 32 or ord(character) == 127 for character in value):
        raise ValueError
    return value


def _parse_macho_imports(payload: bytes) -> tuple[list[str], list[str]]:
    if len(payload) < 32:
        raise ValueError
    magic = payload[:4]
    if magic == b"\xfe\xed\xfa\xcf":
        byte_order = "big"
    elif magic == b"\xcf\xfa\xed\xfe":
        byte_order = "little"
    else:
        raise ValueError
    read32 = lambda offset: int.from_bytes(payload[offset:offset + 4], byte_order)
    command_count = read32(16)
    commands_end = 32 + read32(20)
    if command_count <= 0 or command_count > 4096 or commands_end > len(payload):
        raise ValueError
    imports: list[str] = []
    rpaths: list[str] = []
    offset = 32
    for _index in range(command_count):
        if offset + 8 > commands_end:
            raise ValueError
        command = read32(offset)
        command_size = read32(offset + 4)
        if command_size < 8 or command_size % 4 or offset + command_size > commands_end:
            raise ValueError
        if command in _MACH_O_DYLIB_COMMANDS:
            if command_size < 24:
                raise ValueError
            name_offset = read32(offset + 8)
            if name_offset < 24 or name_offset >= command_size:
                raise ValueError
            imports.append(_bounded_c_string(payload, offset + name_offset, offset + command_size))
        elif command == _MACH_O_RPATH_COMMAND:
            if command_size < 12:
                raise ValueError
            name_offset = read32(offset + 8)
            if name_offset < 12 or name_offset >= command_size:
                raise ValueError
            rpaths.append(_bounded_c_string(payload, offset + name_offset, offset + command_size))
        offset += command_size
    if offset != commands_end:
        raise ValueError
    return imports, rpaths


def _parse_pe_imports(payload: bytes) -> tuple[list[str], list[str]]:
    if len(payload) < 0x40 or payload[:2] != b"MZ":
        raise ValueError
    pe_offset = int.from_bytes(payload[0x3C:0x40], "little")
    if pe_offset < 0x40 or pe_offset + 24 > len(payload) or payload[pe_offset:pe_offset + 4] != b"PE\0\0":
        raise ValueError
    section_count = int.from_bytes(payload[pe_offset + 6:pe_offset + 8], "little")
    optional_size = int.from_bytes(payload[pe_offset + 20:pe_offset + 22], "little")
    optional_offset = pe_offset + 24
    if (
        section_count <= 0
        or section_count > 96
        or optional_size < 152
        or optional_offset + optional_size > len(payload)
        or int.from_bytes(payload[optional_offset:optional_offset + 2], "little") != 0x20B
    ):
        raise ValueError
    directory_count = int.from_bytes(payload[optional_offset + 108:optional_offset + 112], "little")
    if directory_count < 2 or directory_count > 32:
        raise ValueError
    size_of_headers = int.from_bytes(payload[optional_offset + 60:optional_offset + 64], "little")
    section_table = optional_offset + optional_size
    if section_table + section_count * 40 > len(payload):
        raise ValueError
    sections: list[tuple[int, int, int, int]] = []
    for index in range(section_count):
        offset = section_table + index * 40
        virtual_size = int.from_bytes(payload[offset + 8:offset + 12], "little")
        virtual_address = int.from_bytes(payload[offset + 12:offset + 16], "little")
        raw_size = int.from_bytes(payload[offset + 16:offset + 20], "little")
        raw_offset = int.from_bytes(payload[offset + 20:offset + 24], "little")
        if raw_size and (raw_offset < size_of_headers or raw_offset + raw_size > len(payload)):
            raise ValueError
        sections.append((virtual_size, virtual_address, raw_size, raw_offset))

    def rva_offset(rva: int, minimum_bytes: int = 1) -> int:
        if rva <= 0 or minimum_bytes <= 0:
            raise ValueError
        if rva < size_of_headers:
            if rva + minimum_bytes > size_of_headers or rva + minimum_bytes > len(payload):
                raise ValueError
            return rva
        matches = [
            item
            for item in sections
            if rva >= item[1] and rva < item[1] + max(item[0], item[2])
        ]
        if len(matches) != 1:
            raise ValueError
        _virtual_size, virtual_address, raw_size, raw_offset = matches[0]
        delta = rva - virtual_address
        if delta + minimum_bytes > raw_size or raw_offset + delta + minimum_bytes > len(payload):
            raise ValueError
        return raw_offset + delta

    imports: list[str] = []

    def read_directory(index: int, delay: bool) -> None:
        if index >= directory_count:
            return
        directory_offset = optional_offset + 112 + index * 8
        rva = int.from_bytes(payload[directory_offset:directory_offset + 4], "little")
        size = int.from_bytes(payload[directory_offset + 4:directory_offset + 8], "little")
        if (rva == 0) != (size == 0):
            raise ValueError
        if not rva:
            return
        item_size = 32 if delay else 20
        if size < item_size or size > 1024 * 1024:
            raise ValueError
        start = rva_offset(rva, item_size)
        terminated = False
        for cursor in range(0, size - item_size + 1, item_size):
            if cursor // item_size >= 4096:
                break
            offset = start + cursor
            if offset + item_size > len(payload):
                raise ValueError
            record = payload[offset:offset + item_size]
            if not any(record):
                terminated = True
                break
            if delay and not (int.from_bytes(record[:4], "little") & 1):
                raise ValueError
            name_offset_in_record = 4 if delay else 12
            name_rva = int.from_bytes(record[name_offset_in_record:name_offset_in_record + 4], "little")
            name_offset = rva_offset(name_rva)
            imports.append(_bounded_c_string(payload, name_offset, min(len(payload), name_offset + 4097)))
        if not terminated:
            raise ValueError

    read_directory(1, False)
    if directory_count > 13:
        read_directory(13, True)
    return imports, []


def _normalized_package_candidate(base: str, suffix: str) -> str:
    candidate = (Path(base) / suffix).as_posix()
    normalized = os.path.normpath(candidate).replace(os.sep, "/")
    return normalized if _valid_relative_path(normalized) else ""


def _mac_bundled_dependency(
    import_name: str,
    importer: dict[str, object],
    rpaths: list[str],
    manifest: dict[str, object],
) -> str:
    if import_name.startswith(_MACH_O_SYSTEM_PREFIXES):
        return ""
    files = manifest["files"]
    by_path = {str(entry["path"]): entry for entry in files}
    importer_directory = Path(str(importer["path"])).parent.as_posix()
    capabilities = manifest["capabilities"]
    binary_directories = sorted(
        {
            Path(str(capabilities["sofficeBinary"])).parent.as_posix(),
            Path(str(capabilities["tesseractBinary"])).parent.as_posix(),
        }
    )
    candidates: list[str] = []
    if import_name.startswith("@loader_path/"):
        candidates.append(_normalized_package_candidate(importer_directory, import_name[len("@loader_path/"):]))
    elif import_name.startswith("@executable_path/"):
        suffix = import_name[len("@executable_path/"):]
        bases = [importer_directory] if importer["kind"] == "binary" else binary_directories
        candidates.extend(_normalized_package_candidate(base, suffix) for base in bases)
    elif import_name.startswith("@rpath/"):
        suffix = import_name[len("@rpath/"):]
        for rpath in rpaths:
            if rpath.startswith("@loader_path/"):
                candidates.append(
                    _normalized_package_candidate(
                        importer_directory,
                        f"{rpath[len('@loader_path/'): ]}/{suffix}",
                    )
                )
            elif rpath.startswith("@executable_path/"):
                for base in binary_directories:
                    candidates.append(
                        _normalized_package_candidate(
                            base,
                            f"{rpath[len('@executable_path/'): ]}/{suffix}",
                        )
                    )
            elif rpath.startswith(_MACH_O_SYSTEM_PREFIXES):
                candidates.append("")
            else:
                raise ValueError
    else:
        raise ValueError
    bundled = sorted(
        {
            candidate
            for candidate in candidates
            if candidate and by_path.get(candidate, {}).get("kind") == "library"
        }
    )
    if len(bundled) != 1:
        raise ValueError
    return bundled[0]


def _windows_bundled_dependency(
    import_name: str,
    manifest: dict[str, object],
) -> str:
    if not re.fullmatch(r"[A-Za-z0-9_.+-]{1,255}\.dll", import_name, flags=re.IGNORECASE) or Path(import_name).name != import_name:
        raise ValueError
    folded = import_name.lower()
    if folded.startswith(("api-ms-win-", "ext-ms-win-")) or folded in _WINDOWS_SYSTEM_DLLS:
        return ""
    matches = [
        str(entry["path"])
        for entry in manifest["files"]
        if entry["kind"] == "library" and Path(str(entry["path"])).name.lower() == folded
    ]
    if len(matches) != 1:
        raise ValueError
    return matches[0]


def _verify_document_native_dependencies(
    document_root: Path,
    manifest: dict[str, object],
    target: dict[str, str],
) -> bool:
    try:
        for entry in manifest["files"]:
            if entry["kind"] not in {"binary", "library"}:
                continue
            path = document_root.joinpath(*str(entry["path"]).split("/"))
            payload = _read_native_bounded(path)
            if target.get("platform") == "darwin":
                imports, rpaths = _parse_macho_imports(payload)
                actual = {
                    dependency
                    for dependency in (
                        _mac_bundled_dependency(import_name, entry, rpaths, manifest)
                        for import_name in imports
                    )
                    if dependency
                }
            elif target.get("platform") == "win32":
                imports, _rpaths = _parse_pe_imports(payload)
                actual = {
                    dependency
                    for dependency in (
                        _windows_bundled_dependency(import_name, manifest)
                        for import_name in imports
                    )
                    if dependency
                }
            else:
                return False
            if sorted(actual) != entry["dependencies"]:
                return False
    except (OSError, UnicodeError, ValueError, KeyError, TypeError):
        return False
    return True


def _read_strict_manifest(path: Path) -> object:
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
            or (int(before.st_dev), int(before.st_ino)) != (int(opened.st_dev), int(opened.st_ino))
            or int(opened.st_size) > 1024 * 1024
        ):
            raise OSError
        with os.fdopen(descriptor, "r", encoding="utf-8", closefd=True) as handle:
            descriptor = -1
            return json.load(handle, object_pairs_hook=_strict_json_object)
    finally:
        if descriptor >= 0:
            os.close(descriptor)


def _sha256_regular_file(path: Path) -> str:
    return _regular_file_identity(path)[0]


def _regular_file_identity(path: Path) -> tuple[str, int]:
    digest = hashlib.sha256()
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
            or (int(before.st_dev), int(before.st_ino)) != (int(opened.st_dev), int(opened.st_ino))
            or bool(int(opened.st_mode) & 0o022)
        ):
            return "", 0
        while True:
            chunk = os.read(descriptor, 1024 * 1024)
            if not chunk:
                break
            digest.update(chunk)
        after = path.lstat()
        after_opened = os.fstat(descriptor)
        if (
            (int(after.st_dev), int(after.st_ino)) != (int(opened.st_dev), int(opened.st_ino))
            or int(after_opened.st_size) != int(opened.st_size)
            or int(getattr(after_opened, "st_mtime_ns", 0)) != int(getattr(opened, "st_mtime_ns", 0))
        ):
            return "", 0
    except OSError:
        return "", 0
    finally:
        if descriptor >= 0:
            os.close(descriptor)
    return digest.hexdigest(), int(opened.st_size)


def _strict_json_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate manifest key")
        result[key] = value
    return result
