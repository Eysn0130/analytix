from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import platform as host_platform
import re
import stat
import sys
from typing import Any, Optional


_RECEIPT_FILE_NAME = "analytix-native-components-receipt.json"
_MAX_NATIVE_BINARY_BYTES = 2 * 1024 * 1024 * 1024
_COMPONENTS = (
    ("import-accelerator", "tools/import_accelerator", "analytix-import-accelerator"),
    ("cleaning-ops", "tools/cleaning_ops", "analytix-cleaning-ops"),
    ("analysis-compute", "tools/analysis_compute", "analytix-analysis-compute"),
    ("data-engine", "tools/data_engine", "analytix-data-engine"),
)
_COMPONENT_BY_ID = {item[0]: item for item in _COMPONENTS}
_FROZEN_COMPONENTS = (
    {
        "id": "import-accelerator",
        "sourceRoot": "tools/import_accelerator",
        "cargoManifest": "tools/import_accelerator/Cargo.toml",
        "binaryName": "analytix-import-accelerator",
        "role": "immutable-data-import",
        "agentCore": False,
        "consumers": [
            "backend/app/core/import_accelerator.py",
            "backend/app/core/native_component_paths.py",
            "packages/runtime-go/internal/adapters/outbound/nativecomponentregistry/load_darwin.go",
        ],
        "packagePath": "runtime/analytix-import-accelerator",
        "executionProbe": {
            "schemaVersion": 1,
            "authorityProtocol": "analytix-native-build-probe-v1",
            "componentProtocol": "analytix-native-v1",
            "policySha256": "0cc3872164fdadfb687a8642846913bf0f2394cd21c26799a5d483dee1ea8d2a",
            "authorityTargets": ["darwin-arm64", "darwin-x64"],
        },
        "authorization": "existing-data-plane-boundary@33d6de1d3c40691b73033f698af8bb518b15300d",
    },
    {
        "id": "cleaning-ops",
        "sourceRoot": "tools/cleaning_ops",
        "cargoManifest": "tools/cleaning_ops/Cargo.toml",
        "binaryName": "analytix-cleaning-ops",
        "role": "case-data-cleaning",
        "agentCore": False,
        "consumers": [
            "backend/app/repositories/cleaning_native_runtime.py",
            "backend/app/core/native_component_paths.py",
            "packages/runtime-go/internal/adapters/outbound/nativecomponentregistry/load_darwin.go",
        ],
        "packagePath": "runtime/analytix-cleaning-ops",
        "executionProbe": {
            "schemaVersion": 1,
            "authorityProtocol": "analytix-native-build-probe-v1",
            "componentProtocol": "analytix-native-v1",
            "policySha256": "59c4287d312b4972eb6890883865e3c37d226650caf78f65168363413ac85e35",
            "authorityTargets": ["darwin-arm64", "darwin-x64"],
        },
        "authorization": "existing-data-plane-boundary@33d6de1d3c40691b73033f698af8bb518b15300d",
    },
    {
        "id": "analysis-compute",
        "sourceRoot": "tools/analysis_compute",
        "cargoManifest": "tools/analysis_compute/Cargo.toml",
        "binaryName": "analytix-analysis-compute",
        "role": "bounded-analysis-compute",
        "agentCore": False,
        "consumers": [
            "backend/app/core/analysis_compute_runner.py",
            "backend/app/core/native_component_paths.py",
            "packages/runtime-go/internal/adapters/outbound/nativecomponentregistry/load_darwin.go",
        ],
        "packagePath": "runtime/analytix-analysis-compute",
        "executionProbe": {
            "schemaVersion": 1,
            "authorityProtocol": "analytix-native-build-probe-v1",
            "componentProtocol": "analytix-native-v1",
            "policySha256": "fd2227234c61de1505e50aa33e1e24c2f58c2545fe4d07e41b8064709c18f4d8",
            "authorityTargets": ["darwin-arm64", "darwin-x64"],
        },
        "authorization": "existing-data-plane-boundary@33d6de1d3c40691b73033f698af8bb518b15300d",
    },
    {
        "id": "data-engine",
        "sourceRoot": "tools/data_engine",
        "cargoManifest": "tools/data_engine/Cargo.toml",
        "binaryName": "analytix-data-engine",
        "role": "single-owner-case-database",
        "agentCore": False,
        "consumers": [
            "backend/app/core/data_engine_client.py",
            "backend/app/core/db_engine.py",
            "backend/app/core/native_component_paths.py",
            "packages/runtime-go/internal/adapters/outbound/nativecomponentregistry/load_darwin.go",
            "packages/runtime-go/internal/adapters/outbound/nativecomponentrunner/runner_darwin.go",
        ],
        "packagePath": "runtime/analytix-data-engine",
        "executionProbe": {
            "schemaVersion": 1,
            "authorityProtocol": "analytix-native-build-probe-v1",
            "componentProtocol": "analytix-native-v1",
            "policySha256": "97ce13396fb6f5ba7e9d851ab4abc2f23ac8abdd13acaa3e983a326d6bf4a1e0",
            "authorityTargets": ["darwin-arm64", "darwin-x64"],
        },
        "authorization": "existing-data-plane-boundary@af0ea1967c76c7b771aaff1c0d5606ca4d5b832c",
    },
)
_PROBE_POLICY_BY_ID = {
    component["id"]: component["executionProbe"]["policySha256"]
    for component in _FROZEN_COMPONENTS
}
_MANIFEST_COMPONENT_KEYS = {
    "id",
    "sourceRoot",
    "cargoManifest",
    "binaryName",
    "role",
    "agentCore",
    "consumers",
    "packagePath",
    "supportedTargets",
    "executionProbe",
    "authorization",
}
_SUPPORTED_TARGETS = {
    "darwin-arm64": ("darwin", "arm64", "mach-o", "aarch64-apple-darwin"),
    "darwin-x64": ("darwin", "x64", "mach-o", "x86_64-apple-darwin"),
    "linux-x64": ("linux", "x64", "elf", "x86_64-unknown-linux-gnu"),
    "win32-x64": ("win32", "x64", "pe", "x86_64-pc-windows-msvc"),
}
_NATIVE_BUILD_CONTEXT_PATHS = (
    "scripts/build-data-analysis-native-tools.cjs",
    "scripts/native-component-contract.cjs",
    "scripts/native-build-probe-authority.cjs",
    "scripts/native-components.json",
    "scripts/lib/strict-json.cjs",
    "scripts/go-runtime-build-contract.cjs",
    "scripts/go-runtime-toolchain.json",
    "scripts/rust-native-toolchain-contract.cjs",
    "scripts/rust-native-toolchain.json",
    "packages/runtime-go/go.mod",
    "packages/runtime-go/go.sum",
    "packages/runtime-go/cmd/native-component-build-probe/main.go",
    "packages/runtime-go/internal/adapters/outbound/processauthority/build_probe.go",
    "packages/runtime-go/internal/adapters/outbound/processauthority/build_probe_darwin.go",
    "packages/runtime-go/internal/adapters/outbound/processauthority/build_probe_bootstrap_darwin.go",
    "packages/runtime-go/internal/adapters/outbound/processauthority/build_probe_identity.go",
    ".cargo/config",
    ".cargo/config.toml",
    "rust-toolchain",
    "rust-toolchain.toml",
    "tools/.cargo/config",
    "tools/.cargo/config.toml",
    "tools/rust-toolchain",
    "tools/rust-toolchain.toml",
)
_RECEIPT_KEYS = {
    "schemaVersion",
    "manifestSha256",
    "sourceSetSha256",
    "buildEnvironmentSha256",
    "toolchain",
    "targetKey",
    "targetTriple",
    "platform",
    "arch",
    "executionAuthority",
    "publicationAuthority",
    "components",
}
_COMPONENT_RECEIPT_KEYS = {
    "id",
    "binaryName",
    "packagePath",
    "sourceDigest",
    "cargoLockSha256",
    "buildEnvironmentSha256",
    "rawBuildSha256",
    "rawBuildSize",
    "stagedImageSha256",
    "stagedImageSize",
    "payloadSha256",
    "payloadSize",
    "format",
    "arch",
    "executionProbe",
}
_EXECUTION_AUTHORITY_KEYS = {
    "schemaVersion",
    "trustClass",
    "protocol",
    "targetKey",
    "binarySha256",
    "binarySize",
    "sourceSetSha256",
    "buildEnvironmentSha256",
    "goToolchainKey",
    "goExecutableSha256",
}
_EXECUTION_PROBE_KEYS = {
    "kind",
    "schema_version",
    "status",
    "component_id",
    "request_nonce",
    "executable_sha256",
    "executable_size",
    "manifest_sha256",
    "policy_sha256",
    "authority_sha256",
    "host_platform",
    "host_arch",
    "loaded_image_bound",
    "working_directory_bound",
    "guardian_authenticated",
    "process_tree_empty",
}
_PUBLICATION_AUTHORITY_KEYS = {
    "schemaVersion",
    "trustClass",
    "protocol",
    "targetKey",
    "cargoExecutionId",
    "cargoExecutionReceiptSha256",
    "publicationBindingSha256",
}


def normalize_native_platform(value: str | None = None) -> Optional[str]:
    raw = str(value if value is not None else sys.platform).strip().lower()
    if raw.startswith("win"):
        return "win32"
    if raw == "darwin":
        return "darwin"
    if raw.startswith("linux"):
        return "linux"
    return None


def normalize_native_arch(value: str | None = None) -> Optional[str]:
    raw = str(value if value is not None else host_platform.machine()).strip().lower()
    if raw in {"amd64", "x86_64", "x64"}:
        return "x64"
    if raw in {"aarch64", "arm64"}:
        return "arm64"
    return None


def native_target_key(
    platform_name: str | None = None,
    machine: str | None = None,
) -> Optional[str]:
    normalized_platform = normalize_native_platform(platform_name)
    normalized_arch = normalize_native_arch(machine)
    if normalized_platform is None or normalized_arch is None:
        return None
    key = f"{normalized_platform}-{normalized_arch}"
    return key if key in _SUPPORTED_TARGETS else None


def canonical_native_stage_path(
    root: Path,
    component_id: str,
    *,
    platform_name: str | None = None,
    machine: str | None = None,
) -> Optional[Path]:
    del root, component_id, platform_name, machine
    return None


def resolve_explicit_native_binary(raw_path: str, root: Path) -> Optional[Path]:
    del raw_path, root
    return None


def _require_no_symlink_path(
    path: Path,
    boundary_root: Path,
    *,
    allow_missing: bool = False,
) -> None:
    boundary = Path(os.path.abspath(boundary_root))
    candidate = Path(os.path.abspath(path))
    try:
        relative = candidate.relative_to(boundary)
    except ValueError as error:
        raise ValueError(f"native provenance escapes repository boundary: {candidate}") from error
    _require_regular_directory(boundary, "native repository boundary")
    current = boundary
    for part in relative.parts:
        current = current / part
        if not os.path.lexists(current):
            if allow_missing:
                return
            raise ValueError(f"native provenance path is missing: {current}")
        if stat.S_ISLNK(current.lstat().st_mode):
            raise ValueError(f"native provenance traverses a symlink: {current}")


def _source_tree_digest(root_text: str, boundary_root_text: str | None = None) -> str:
    root = Path(os.path.abspath(root_text))
    boundary_root = Path(os.path.abspath(boundary_root_text or root_text))
    _require_no_symlink_path(root, boundary_root)
    _require_regular_directory(root, "native source root")
    files: list[Path] = []

    def collect(current: Path) -> None:
        for entry in os.scandir(current):
            path = Path(entry.path)
            if entry.is_symlink():
                raise ValueError(f"native source provenance contains a symlink: {path}")
            if entry.is_dir(follow_symlinks=False):
                if current == root and entry.name in {"target", ".git"}:
                    continue
                collect(path)
            elif entry.is_file(follow_symlinks=False):
                files.append(path)
            else:
                raise ValueError(f"native source provenance contains a special file: {path}")

    collect(root)
    digest = hashlib.sha256()
    for path in sorted(files, key=lambda item: item.relative_to(root).as_posix().encode("utf-8")):
        payload = path.read_bytes()
        relative_path = path.relative_to(root).as_posix()
        digest.update(relative_path.encode("utf-8"))
        digest.update(b"\0")
        digest.update(str(len(payload)).encode("ascii"))
        digest.update(b"\0")
        digest.update(hashlib.sha256(payload).hexdigest().encode("ascii"))
        digest.update(b"\0")
    return digest.hexdigest()


def _native_build_context_digest(root_text: str) -> str:
    root = Path(os.path.abspath(root_text))
    digest = hashlib.sha256()
    for relative_path in _NATIVE_BUILD_CONTEXT_PATHS:
        path = root / relative_path
        digest.update(relative_path.encode("utf-8"))
        digest.update(b"\0")
        _require_no_symlink_path(path, root, allow_missing=True)
        if not os.path.lexists(path):
            digest.update(b"missing\0")
            continue
        _require_regular_file(path, "native build context input")
        payload = path.read_bytes()
        digest.update(str(len(payload)).encode("ascii"))
        digest.update(b"\0")
        digest.update(hashlib.sha256(payload).hexdigest().encode("ascii"))
        digest.update(b"\0")
    return digest.hexdigest()


def _source_set_digest(root_text: str) -> str:
    root = Path(root_text)
    digest = hashlib.sha256()
    for component_id, source_root, _binary_name in _COMPONENTS:
        digest.update(component_id.encode("utf-8"))
        digest.update(b"\0")
        digest.update(_source_tree_digest(str(root / source_root), str(root)).encode("ascii"))
        digest.update(b"\0")
    digest.update(b"build-context\0")
    digest.update(_native_build_context_digest(str(root)).encode("ascii"))
    digest.update(b"\0")
    return digest.hexdigest()


def _load_json_without_duplicates(text: str) -> Any:
    def pairs_hook(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
        value: dict[str, Any] = {}
        for key, item in pairs:
            if key in value:
                raise ValueError(f"duplicate JSON key: {key}")
            value[key] = item
        return value

    return json.loads(text, object_pairs_hook=pairs_hook)


def resolve_canonical_native_component(
    root: Path,
    component_id: str,
    *,
    platform_name: str | None = None,
    machine: str | None = None,
) -> Optional[Path]:
    del root, component_id, platform_name, machine
    return None


def _validate_manifest(value: Any) -> None:
    if not isinstance(value, dict) or set(value) != {"schemaVersion", "receiptSchemaVersion", "components"}:
        raise ValueError("invalid native component registry")
    if (
        value.get("schemaVersion") != 4
        or value.get("receiptSchemaVersion") != 6
        or not isinstance(value.get("components"), list)
    ):
        raise ValueError("invalid native component registry version")
    if len(value["components"]) != len(_FROZEN_COMPONENTS):
        raise ValueError("native component registry inventory drift")
    supported_targets = list(_SUPPORTED_TARGETS)
    for actual, expected in zip(value["components"], _FROZEN_COMPONENTS, strict=True):
        if not isinstance(actual, dict) or set(actual) != _MANIFEST_COMPONENT_KEYS:
            raise ValueError("native component registry entry schema drift")
        frozen = {**expected, "supportedTargets": supported_targets}
        if actual != frozen:
            raise ValueError("native component registry boundary drift")


def _validate_receipt(
    value: Any,
    *,
    manifest_sha256: str,
    source_set_sha256: str,
    target_key: str,
) -> None:
    if not isinstance(value, dict) or set(value) != _RECEIPT_KEYS or value.get("schemaVersion") != 6:
        raise ValueError("invalid native target receipt")
    target_platform, target_arch, _target_format, target_triple = _SUPPORTED_TARGETS[target_key]
    if target_platform != "darwin":
        raise ValueError(f"native execution authority is unavailable for target {target_key}")
    if (
        value.get("manifestSha256") != manifest_sha256
        or value.get("sourceSetSha256") != source_set_sha256
        or value.get("targetKey") != target_key
        or value.get("targetTriple") != target_triple
        or value.get("platform") != target_platform
        or value.get("arch") != target_arch
        or not isinstance(value.get("components"), list)
        or len(value["components"]) != len(_COMPONENTS)
    ):
        raise ValueError("stale or mismatched native target receipt")
    toolchain = value.get("toolchain")
    authority = value.get("executionAuthority")
    publication_authority = value.get("publicationAuthority")
    if (
        not isinstance(value.get("buildEnvironmentSha256"), str)
        or re.fullmatch(r"[0-9a-f]{64}", value["buildEnvironmentSha256"]) is None
        or not isinstance(toolchain, dict)
        or set(toolchain)
        != {"cargoExecutableSha256", "cargoVersion", "rustcExecutableSha256", "rustcVersion"}
        or not isinstance(toolchain.get("cargoVersion"), str)
        or re.match(r"^cargo 1\.94\.1\b", toolchain["cargoVersion"]) is None
        or not isinstance(toolchain.get("rustcVersion"), str)
        or re.match(r"^rustc 1\.94\.1\b", toolchain["rustcVersion"]) is None
        or re.fullmatch(r"[0-9a-f]{64}", str(toolchain.get("cargoExecutableSha256", ""))) is None
        or re.fullmatch(r"[0-9a-f]{64}", str(toolchain.get("rustcExecutableSha256", ""))) is None
        or not isinstance(authority, dict)
        or set(authority) != _EXECUTION_AUTHORITY_KEYS
        or authority.get("schemaVersion") != 1
        or authority.get("trustClass") != "controlled_release"
        or authority.get("protocol") != "analytix-native-build-probe-v1"
        or authority.get("targetKey") not in {"darwin-arm64", "darwin-x64"}
        or authority.get("targetKey") != target_key
        or authority.get("goToolchainKey") != authority.get("targetKey")
        or not isinstance(authority.get("binarySize"), int)
        or authority["binarySize"] <= 0
        or any(
            re.fullmatch(r"[0-9a-f]{64}", str(authority.get(field, ""))) is None
            for field in (
                "binarySha256",
                "sourceSetSha256",
                "buildEnvironmentSha256",
                "goExecutableSha256",
            )
        )
        or not isinstance(publication_authority, dict)
        or set(publication_authority) != _PUBLICATION_AUTHORITY_KEYS
        or publication_authority.get("schemaVersion") != 1
        or publication_authority.get("trustClass") != "controlled_release"
        or publication_authority.get("protocol") != "analytix-cargo-execution-authority-v1"
        or publication_authority.get("targetKey") != target_key
        or any(
            re.fullmatch(r"[0-9a-f]{64}", str(publication_authority.get(field, ""))) is None
            for field in (
                "cargoExecutionId",
                "cargoExecutionReceiptSha256",
                "publicationBindingSha256",
            )
        )
    ):
        raise ValueError("invalid native build environment receipt")


def _validate_component_record(
    value: Any,
    component: tuple[str, str, str],
    target_key: str,
    candidate: Path,
    *,
    manifest_sha256: str,
    authority: dict[str, Any],
) -> None:
    if not isinstance(value, dict) or set(value) != _COMPONENT_RECEIPT_KEYS:
        raise ValueError("invalid native component receipt")
    target_platform, target_arch, target_format, _target_triple = _SUPPORTED_TARGETS[target_key]
    binary_name = f"{component[2]}.exe" if target_platform == "win32" else component[2]
    package_path = f"runtime/{binary_name}"
    probe = value.get("executionProbe")
    authority_target = _SUPPORTED_TARGETS.get(str(authority.get("targetKey", "")))
    if (
        value.get("id") != component[0]
        or
        value.get("binaryName") != binary_name
        or value.get("packagePath") != package_path
        or value.get("format") != target_format
        or value.get("arch") != target_arch
        or not isinstance(probe, dict)
        or set(probe) != _EXECUTION_PROBE_KEYS
        or probe.get("kind") != "analytix_native_build_probe_receipt"
        or probe.get("schema_version") != 1
        or probe.get("status") != "passed"
        or probe.get("component_id") != component[0]
        or re.fullmatch(r"[0-9a-f]{64}", str(probe.get("request_nonce", ""))) is None
        or probe.get("executable_sha256") != value.get("stagedImageSha256")
        or probe.get("executable_size") != value.get("stagedImageSize")
        or probe.get("manifest_sha256") != manifest_sha256
        or probe.get("policy_sha256") != _PROBE_POLICY_BY_ID[component[0]]
        or probe.get("authority_sha256") != authority.get("binarySha256")
        or authority_target is None
        or probe.get("host_platform") != authority_target[0]
        or probe.get("host_arch") != authority_target[1]
        or probe.get("loaded_image_bound") is not True
        or probe.get("working_directory_bound") is not True
        or probe.get("guardian_authenticated") is not True
        or probe.get("process_tree_empty") is not True
        or not isinstance(value.get("rawBuildSize"), int)
        or value["rawBuildSize"] <= 0
        or value["rawBuildSize"] > _MAX_NATIVE_BINARY_BYTES
        or not isinstance(value.get("stagedImageSize"), int)
        or value["stagedImageSize"] <= 0
        or value["stagedImageSize"] > _MAX_NATIVE_BINARY_BYTES
        or not isinstance(value.get("payloadSize"), int)
        or value["payloadSize"] <= 0
        or value["payloadSize"] > value["rawBuildSize"]
        or value["payloadSize"] > value["stagedImageSize"]
    ):
        raise ValueError("mismatched native component receipt")
    if not _is_executable_regular_file(candidate):
        raise ValueError("native component binary is not an executable regular file")
    payload = candidate.read_bytes()
    for field in (
        "sourceDigest",
        "cargoLockSha256",
        "buildEnvironmentSha256",
        "rawBuildSha256",
        "stagedImageSha256",
        "payloadSha256",
    ):
        if not isinstance(value.get(field), str) or re.fullmatch(r"[0-9a-f]{64}", value[field]) is None:
            raise ValueError("invalid native component digest")
    actual_format, actual_arch, payload_sha256, payload_size = _native_payload_identity(payload)
    if actual_format != target_format or actual_arch != target_arch:
        raise ValueError("native component binary target mismatch")
    if value.get("payloadSize") != payload_size or value.get("payloadSha256") != payload_sha256:
        raise ValueError("native component payload digest mismatch")
    repository_root = candidate.parents[3]
    if value.get("sourceDigest") != _source_tree_digest(
        str(repository_root / component[1]),
        str(repository_root),
    ):
        raise ValueError("native component source digest mismatch")
    cargo_lock = candidate.parents[3] / component[1] / "Cargo.lock"
    _require_regular_file(cargo_lock, "native component Cargo.lock")
    if value.get("cargoLockSha256") != hashlib.sha256(cargo_lock.read_bytes()).hexdigest():
        raise ValueError("native component Cargo.lock digest mismatch")


def _uint(payload: bytes | bytearray, offset: int, size: int, byteorder: str) -> int:
    if offset < 0 or offset + size > len(payload):
        raise ValueError("native executable field is truncated")
    return int.from_bytes(payload[offset : offset + size], byteorder)


def _put_uint(payload: bytearray, offset: int, size: int, value: int, byteorder: str) -> None:
    if value < 0 or value >= 1 << (size * 8) or offset < 0 or offset + size > len(payload):
        raise ValueError("native executable canonical field is invalid")
    payload[offset : offset + size] = value.to_bytes(size, byteorder)


def _mach_o_payload_identity(payload: bytes) -> tuple[str, str, str, int]:
    if len(payload) < 32:
        raise ValueError("Mach-O header is truncated")
    if _uint(payload, 0, 4, "big") == 0xFEEDFACF:
        byteorder = "big"
    elif _uint(payload, 0, 4, "little") == 0xFEEDFACF:
        byteorder = "little"
    else:
        raise ValueError("unsupported Mach-O magic")
    cpu = _uint(payload, 4, 4, byteorder)
    if _uint(payload, 12, 4, byteorder) != 2:
        raise ValueError("Mach-O is not an executable")
    command_count = _uint(payload, 16, 4, byteorder)
    commands_size = _uint(payload, 20, 4, byteorder)
    commands_end = 32 + commands_size
    if command_count == 0 or command_count > 4096 or commands_size < 8 or commands_end > len(payload):
        raise ValueError("Mach-O load commands are invalid")

    executable_segment = False
    link_edit: list[tuple[int, int, int]] = []
    signatures: list[tuple[int, int, int]] = []
    command_offset = 32
    for _index in range(command_count):
        command = _uint(payload, command_offset, 4, byteorder)
        command_size = _uint(payload, command_offset + 4, 4, byteorder)
        if command_size < 8 or command_size % 4 != 0 or command_offset + command_size > commands_end:
            raise ValueError("Mach-O load command size is invalid")
        if command == 0x19:
            if command_size < 72:
                raise ValueError("Mach-O segment command is truncated")
            segment_name = payload[command_offset + 8 : command_offset + 24].split(b"\0", 1)[0]
            file_offset = _uint(payload, command_offset + 40, 8, byteorder)
            file_size = _uint(payload, command_offset + 48, 8, byteorder)
            initial_protection = _uint(payload, command_offset + 60, 4, byteorder)
            if file_offset + file_size > len(payload):
                raise ValueError("Mach-O segment file range is invalid")
            if initial_protection & 0x4 and file_size > 0:
                executable_segment = True
            if segment_name == b"__LINKEDIT":
                link_edit.append((command_offset, file_offset, file_size))
        elif command == 0x1D:
            if command_size != 16:
                raise ValueError("Mach-O code-signature command size is invalid")
            signatures.append(
                (
                    command_offset,
                    _uint(payload, command_offset + 8, 4, byteorder),
                    _uint(payload, command_offset + 12, 4, byteorder),
                )
            )
        command_offset += command_size
    if command_offset != commands_end or not executable_segment or len(link_edit) != 1 or len(signatures) != 1:
        raise ValueError("Mach-O signed payload structure is invalid")

    signature_command, signature_offset, signature_size = signatures[0]
    link_command, link_offset, link_size = link_edit[0]
    super_blob_size = _uint(payload, signature_offset + 4, 4, "big")
    if (
        signature_size < 12
        or signature_offset < commands_end
        or signature_offset % 8 != 0
        or signature_offset + signature_size != len(payload)
        or link_offset > signature_offset
        or link_offset + link_size != len(payload)
        or _uint(payload, signature_offset, 4, "big") != 0xFADE0CC0
        or super_blob_size < 12
        or super_blob_size > signature_size
    ):
        raise ValueError("Mach-O code-signature range is invalid or ambiguous")

    canonical = bytearray(payload[:signature_offset])
    link_payload_size = signature_offset - link_offset
    _put_uint(canonical, link_command + 32, 8, link_payload_size, byteorder)
    _put_uint(canonical, link_command + 48, 8, link_payload_size, byteorder)
    _put_uint(canonical, signature_command + 12, 4, 0, byteorder)
    arch = "x64" if cpu == 0x01000007 else "arm64" if cpu == 0x0100000C else f"cpu-{cpu}"
    return "mach-o", arch, hashlib.sha256(canonical).hexdigest(), len(canonical)


def _pe_payload_identity(payload: bytes) -> tuple[str, str, str, int]:
    if len(payload) < 0x40 or payload[:2] != b"MZ":
        raise ValueError("PE DOS header is invalid")
    pe_offset = _uint(payload, 0x3C, 4, "little")
    if pe_offset < 0x40 or payload[pe_offset : pe_offset + 4] != b"PE\0\0":
        raise ValueError("PE signature is invalid")
    machine = _uint(payload, pe_offset + 4, 2, "little")
    section_count = _uint(payload, pe_offset + 6, 2, "little")
    optional_size = _uint(payload, pe_offset + 20, 2, "little")
    characteristics = _uint(payload, pe_offset + 22, 2, "little")
    optional_offset = pe_offset + 24
    optional_end = optional_offset + optional_size
    if (
        section_count == 0
        or section_count > 96
        or optional_size < 152
        or optional_end > len(payload)
        or _uint(payload, optional_offset, 2, "little") != 0x20B
        or not characteristics & 0x0002
        or _uint(payload, optional_offset + 16, 4, "little") == 0
        or _uint(payload, optional_offset + 108, 4, "little") < 5
    ):
        raise ValueError("PE32+ executable header is invalid")
    size_of_headers = _uint(payload, optional_offset + 60, 4, "little")
    if size_of_headers == 0 or size_of_headers > len(payload) or optional_end + section_count * 40 > len(payload):
        raise ValueError("PE section table is invalid")

    executable_section = False
    section_end = 0
    for index in range(section_count):
        section_offset = optional_end + index * 40
        raw_size = _uint(payload, section_offset + 16, 4, "little")
        raw_offset = _uint(payload, section_offset + 20, 4, "little")
        section_flags = _uint(payload, section_offset + 36, 4, "little")
        if raw_size:
            if raw_offset < size_of_headers or raw_offset + raw_size > len(payload):
                raise ValueError("PE section range is invalid")
            section_end = max(section_end, raw_offset + raw_size)
            if section_flags & 0x20000000:
                executable_section = True
    if not executable_section:
        raise ValueError("PE executable section is missing")

    checksum_offset = optional_offset + 64
    certificate_directory = optional_offset + 144
    certificate_offset = _uint(payload, certificate_directory, 4, "little")
    certificate_size = _uint(payload, certificate_directory + 4, 4, "little")
    if (certificate_offset == 0) != (certificate_size == 0):
        raise ValueError("PE certificate table offset and size must be paired")
    payload_end = len(payload)
    if certificate_offset == 0:
        if section_end != len(payload):
            raise ValueError("unsigned PE contains an unsupported overlay or gap")
    else:
        if (
            certificate_offset % 8 != 0
            or certificate_size < 8
            or certificate_size % 8 != 0
            or certificate_offset != section_end
            or certificate_offset + certificate_size != len(payload)
        ):
            raise ValueError("PE certificate table range is invalid or ambiguous")
        cursor = certificate_offset
        while cursor < len(payload):
            record_length = _uint(payload, cursor, 4, "little")
            revision = _uint(payload, cursor + 4, 2, "little")
            certificate_type = _uint(payload, cursor + 6, 2, "little")
            aligned_length = record_length
            if (
                record_length < 8
                or record_length % 8 != 0
                or cursor + aligned_length > len(payload)
                or revision not in {0x0100, 0x0200}
                or certificate_type != 0x0002
            ):
                raise ValueError("PE WIN_CERTIFICATE record is invalid")
            cursor += aligned_length
        if cursor != len(payload):
            raise ValueError("PE certificate table walk is incomplete")
        payload_end = certificate_offset

    canonical = bytearray(payload[:payload_end])
    canonical[checksum_offset : checksum_offset + 4] = b"\0" * 4
    canonical[certificate_directory : certificate_directory + 8] = b"\0" * 8
    arch = "x64" if machine == 0x8664 else "arm64" if machine == 0xAA64 else f"machine-{machine}"
    return "pe", arch, hashlib.sha256(canonical).hexdigest(), len(canonical)


def _elf_payload_identity(payload: bytes) -> tuple[str, str, str, int]:
    if (
        len(payload) < 64
        or payload[:4] != b"\x7fELF"
        or payload[4] != 2
        or payload[5] not in {1, 2}
        or payload[6] != 1
    ):
        raise ValueError("ELF64 identification is invalid")
    byteorder = "little" if payload[5] == 1 else "big"
    file_type = _uint(payload, 16, 2, byteorder)
    machine = _uint(payload, 18, 2, byteorder)
    program_offset = _uint(payload, 32, 8, byteorder)
    header_size = _uint(payload, 52, 2, byteorder)
    program_size = _uint(payload, 54, 2, byteorder)
    program_count = _uint(payload, 56, 2, byteorder)
    if (
        file_type not in {2, 3}
        or _uint(payload, 20, 4, byteorder) != 1
        or header_size < 64
        or program_size < 56
        or program_count == 0
        or program_offset < header_size
        or program_offset + program_size * program_count > len(payload)
    ):
        raise ValueError("ELF64 executable header is invalid")
    executable_segment = False
    for index in range(program_count):
        item = program_offset + index * program_size
        file_offset = _uint(payload, item + 8, 8, byteorder)
        file_size = _uint(payload, item + 32, 8, byteorder)
        if file_offset + file_size > len(payload):
            raise ValueError("ELF64 segment range is invalid")
        if _uint(payload, item, 4, byteorder) == 1 and _uint(payload, item + 4, 4, byteorder) & 0x1 and file_size:
            executable_segment = True
    if not executable_segment:
        raise ValueError("ELF64 executable segment is missing")
    arch = "x64" if machine == 62 else "arm64" if machine == 183 else f"machine-{machine}"
    return "elf", arch, hashlib.sha256(payload).hexdigest(), len(payload)


def _native_payload_identity(payload: bytes) -> tuple[str, str, str, int]:
    if len(payload) >= 4 and (
        int.from_bytes(payload[:4], "big") == 0xFEEDFACF
        or int.from_bytes(payload[:4], "little") == 0xFEEDFACF
    ):
        return _mach_o_payload_identity(payload)
    if payload[:2] == b"MZ":
        return _pe_payload_identity(payload)
    if payload[:4] == b"\x7fELF":
        return _elf_payload_identity(payload)
    raise ValueError("unrecognized native executable format")


def _require_regular_directory(path: Path, label: str) -> None:
    mode = path.lstat().st_mode
    if stat.S_ISLNK(mode) or not stat.S_ISDIR(mode):
        raise ValueError(f"{label} must be a regular non-symlink directory: {path}")


def _require_regular_file(path: Path, label: str) -> None:
    mode = path.lstat().st_mode
    if stat.S_ISLNK(mode) or not stat.S_ISREG(mode):
        raise ValueError(f"{label} must be a regular non-symlink file: {path}")


def _is_executable_regular_file(path: Path) -> bool:
    try:
        _require_regular_file(path, "native binary")
    except (OSError, ValueError):
        return False
    return os.name == "nt" or os.access(path, os.X_OK)


__all__ = [
    "canonical_native_stage_path",
    "native_target_key",
    "normalize_native_arch",
    "normalize_native_platform",
    "resolve_canonical_native_component",
    "resolve_explicit_native_binary",
]
