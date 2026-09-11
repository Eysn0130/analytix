from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path

import pytest

from app.core import native_component_paths as paths


def _native_fixture(format_name: str, arch: str) -> bytes:
    if format_name == "mach-o":
        payload = bytearray(0x220)
        payload[0:4] = (0xFEEDFACF).to_bytes(4, "little")
        payload[4:8] = (0x0100000C if arch == "arm64" else 0x01000007).to_bytes(4, "little")
        payload[12:16] = (2).to_bytes(4, "little")
        payload[16:20] = (3).to_bytes(4, "little")
        payload[20:24] = (160).to_bytes(4, "little")
        payload[32:36] = (0x19).to_bytes(4, "little")
        payload[36:40] = (72).to_bytes(4, "little")
        payload[40:46] = b"__TEXT"
        payload[80:88] = (0x180).to_bytes(8, "little")
        payload[88:92] = (7).to_bytes(4, "little")
        payload[92:96] = (5).to_bytes(4, "little")
        link_edit = 104
        payload[link_edit : link_edit + 4] = (0x19).to_bytes(4, "little")
        payload[link_edit + 4 : link_edit + 8] = (72).to_bytes(4, "little")
        payload[link_edit + 8 : link_edit + 18] = b"__LINKEDIT"
        payload[link_edit + 32 : link_edit + 40] = (len(payload) - 0x180).to_bytes(8, "little")
        payload[link_edit + 40 : link_edit + 48] = (0x180).to_bytes(8, "little")
        payload[link_edit + 48 : link_edit + 56] = (len(payload) - 0x180).to_bytes(8, "little")
        payload[link_edit + 56 : link_edit + 60] = (1).to_bytes(4, "little")
        payload[link_edit + 60 : link_edit + 64] = (1).to_bytes(4, "little")
        signature = 176
        payload[signature : signature + 4] = (0x1D).to_bytes(4, "little")
        payload[signature + 4 : signature + 8] = (16).to_bytes(4, "little")
        payload[signature + 8 : signature + 12] = (0x200).to_bytes(4, "little")
        payload[signature + 12 : signature + 16] = (0x20).to_bytes(4, "little")
        payload[0x200:0x204] = (0xFADE0CC0).to_bytes(4, "big")
        payload[0x204:0x208] = (0x20).to_bytes(4, "big")
        payload[0x208:0x20C] = (1).to_bytes(4, "big")
        payload[0x210:0x214] = (0x14).to_bytes(4, "big")
        return bytes(payload)
    if format_name == "pe":
        payload = bytearray(0x400)
        payload[:2] = b"MZ"
        pe_offset = 0x80
        payload[0x3C:0x40] = pe_offset.to_bytes(4, "little")
        payload[pe_offset : pe_offset + 4] = b"PE\0\0"
        payload[pe_offset + 4 : pe_offset + 6] = (0xAA64 if arch == "arm64" else 0x8664).to_bytes(2, "little")
        payload[pe_offset + 6 : pe_offset + 8] = (1).to_bytes(2, "little")
        payload[pe_offset + 20 : pe_offset + 22] = (0xF0).to_bytes(2, "little")
        payload[pe_offset + 22 : pe_offset + 24] = (0x22).to_bytes(2, "little")
        optional = pe_offset + 24
        payload[optional : optional + 2] = (0x20B).to_bytes(2, "little")
        payload[optional + 16 : optional + 20] = (0x1000).to_bytes(4, "little")
        payload[optional + 56 : optional + 60] = (0x2000).to_bytes(4, "little")
        payload[optional + 60 : optional + 64] = (0x200).to_bytes(4, "little")
        payload[optional + 108 : optional + 112] = (16).to_bytes(4, "little")
        section = optional + 0xF0
        payload[section : section + 5] = b".text"
        payload[section + 16 : section + 20] = (0x200).to_bytes(4, "little")
        payload[section + 20 : section + 24] = (0x200).to_bytes(4, "little")
        payload[section + 36 : section + 40] = (0x60000020).to_bytes(4, "little")
        return bytes(payload)
    payload = bytearray(0x200)
    payload[:4] = b"\x7fELF"
    payload[4:7] = b"\x02\x01\x01"
    payload[16:18] = (3).to_bytes(2, "little")
    payload[18:20] = (183 if arch == "arm64" else 62).to_bytes(2, "little")
    payload[20:24] = (1).to_bytes(4, "little")
    payload[32:40] = (64).to_bytes(8, "little")
    payload[52:54] = (64).to_bytes(2, "little")
    payload[54:56] = (56).to_bytes(2, "little")
    payload[56:58] = (1).to_bytes(2, "little")
    payload[64:68] = (1).to_bytes(4, "little")
    payload[68:72] = (5).to_bytes(4, "little")
    payload[96:104] = len(payload).to_bytes(8, "little")
    return bytes(payload)


def _write_native_stage(root: Path, *, target_key: str = "darwin-arm64") -> dict[str, Path]:
    platform_name, arch, format_name, target_triple = paths._SUPPORTED_TARGETS[target_key]
    manifest_path = root / "scripts" / "native-components.json"
    manifest_path.parent.mkdir(parents=True)
    manifest_bytes = (Path(__file__).parents[2] / "scripts" / "native-components.json").read_bytes()
    manifest_path.write_bytes(manifest_bytes)
    manifest_sha256 = hashlib.sha256(manifest_bytes).hexdigest()
    execution_authority = {
        "schemaVersion": 1,
        "trustClass": "controlled_release",
        "protocol": "analytix-native-build-probe-v1",
        "targetKey": target_key,
        "binarySha256": hashlib.sha256(b"native-build-probe-authority").hexdigest(),
        "binarySize": 4096,
        "sourceSetSha256": hashlib.sha256(b"native-build-probe-source").hexdigest(),
        "buildEnvironmentSha256": hashlib.sha256(b"native-build-probe-environment").hexdigest(),
        "goToolchainKey": target_key,
        "goExecutableSha256": hashlib.sha256(b"native-build-probe-go").hexdigest(),
    }
    publication_authority = {
        "schemaVersion": 1,
        "trustClass": "controlled_release",
        "protocol": "analytix-cargo-execution-authority-v1",
        "targetKey": target_key,
        "cargoExecutionId": hashlib.sha256(b"cargo-execution").hexdigest(),
        "cargoExecutionReceiptSha256": hashlib.sha256(b"cargo-execution-receipt").hexdigest(),
        "publicationBindingSha256": hashlib.sha256(b"publication-binding").hexdigest(),
    }

    for component_id, source_root, _binary_name in paths._COMPONENTS:
        source = root / source_root
        (source / "src").mkdir(parents=True)
        (source / "Cargo.lock").write_text(f"lock:{component_id}\n", encoding="utf-8")
        (source / "src" / "main.rs").write_text(f"fn main() {{ /* {component_id} */ }}\n", encoding="utf-8")

    stage = root / "runtime" / "data-native" / target_key
    stage.mkdir(parents=True)
    binaries: dict[str, Path] = {}
    component_receipts = []
    for component_id, source_root, binary_base_name in paths._COMPONENTS:
        binary_name = f"{binary_base_name}.exe" if platform_name == "win32" else binary_base_name
        binary = stage / binary_name
        payload = _native_fixture(format_name, arch)
        binary.write_bytes(payload)
        binary.chmod(0o755)
        binaries[component_id] = binary
        source = root / source_root
        _actual_format, _actual_arch, payload_sha256, payload_size = paths._native_payload_identity(payload)
        component_receipts.append(
            {
                "id": component_id,
                "binaryName": binary_name,
                "packagePath": f"runtime/{binary_name}",
                "sourceDigest": paths._source_tree_digest(str(source)),
                "cargoLockSha256": hashlib.sha256((source / "Cargo.lock").read_bytes()).hexdigest(),
                "buildEnvironmentSha256": "3" * 64,
                "rawBuildSha256": hashlib.sha256(payload).hexdigest(),
                "rawBuildSize": len(payload),
                "stagedImageSha256": hashlib.sha256(payload).hexdigest(),
                "stagedImageSize": len(payload),
                "payloadSha256": payload_sha256,
                "payloadSize": payload_size,
                "format": format_name,
                "arch": arch,
                "executionProbe": {
                    "kind": "analytix_native_build_probe_receipt",
                    "schema_version": 1,
                    "status": "passed",
                    "component_id": component_id,
                    "request_nonce": hashlib.sha256(f"nonce:{component_id}".encode()).hexdigest(),
                    "executable_sha256": hashlib.sha256(payload).hexdigest(),
                    "executable_size": len(payload),
                    "manifest_sha256": manifest_sha256,
                    "policy_sha256": paths._PROBE_POLICY_BY_ID[component_id],
                    "authority_sha256": execution_authority["binarySha256"],
                    "host_platform": platform_name,
                    "host_arch": arch,
                    "loaded_image_bound": True,
                    "working_directory_bound": True,
                    "guardian_authenticated": True,
                    "process_tree_empty": True,
                },
            }
        )
    receipt = {
        "schemaVersion": 6,
        "manifestSha256": manifest_sha256,
        "sourceSetSha256": paths._source_set_digest(str(root)),
        "buildEnvironmentSha256": "0" * 64,
        "toolchain": {
            "cargoExecutableSha256": "1" * 64,
            "cargoVersion": "cargo 1.94.1 test fixture",
            "rustcExecutableSha256": "2" * 64,
            "rustcVersion": "rustc 1.94.1 test fixture",
        },
        "targetKey": target_key,
        "targetTriple": target_triple,
        "platform": platform_name,
        "arch": arch,
        "executionAuthority": execution_authority,
        "publicationAuthority": publication_authority,
        "components": component_receipts,
    }
    (stage / paths._RECEIPT_FILE_NAME).write_text(
        json.dumps(receipt, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    return binaries


def _validate_native_stage(root: Path, *, target_key: str = "darwin-arm64") -> None:
    manifest_path = root / "scripts" / "native-components.json"
    manifest_bytes = manifest_path.read_bytes()
    manifest = paths._load_json_without_duplicates(manifest_bytes.decode("utf-8"))
    paths._validate_manifest(manifest)

    stage = root / "runtime" / "data-native" / target_key
    receipt = paths._load_json_without_duplicates(
        (stage / paths._RECEIPT_FILE_NAME).read_text(encoding="utf-8")
    )
    paths._validate_receipt(
        receipt,
        manifest_sha256=hashlib.sha256(manifest_bytes).hexdigest(),
        source_set_sha256=paths._source_set_digest(str(root)),
        target_key=target_key,
    )
    assert isinstance(receipt["executionAuthority"], dict)
    for component_record, component in zip(receipt["components"], paths._COMPONENTS, strict=True):
        binary_name = f"{component[2]}.exe" if target_key.startswith("win32-") else component[2]
        paths._validate_component_record(
            component_record,
            component,
            target_key,
            stage / binary_name,
            manifest_sha256=hashlib.sha256(manifest_bytes).hexdigest(),
            authority=receipt["executionAuthority"],
        )


def _set_nested_receipt_value(value: object, path: tuple[str | int, ...], replacement: object) -> None:
    current = value
    for segment in path[:-1]:
        if isinstance(segment, int):
            assert isinstance(current, list)
            current = current[segment]
        else:
            assert isinstance(current, dict)
            current = current[segment]
    final = path[-1]
    if isinstance(final, int):
        assert isinstance(current, list)
        current[final] = replacement
    else:
        assert isinstance(current, dict)
        current[final] = replacement


def test_native_target_normalization_is_fail_closed() -> None:
    assert paths.native_target_key("darwin", "aarch64") == "darwin-arm64"
    assert paths.native_target_key("darwin", "x86_64") == "darwin-x64"
    assert paths.native_target_key("windows", "AMD64") == "win32-x64"
    assert paths.native_target_key("linux", "x64") == "linux-x64"
    assert paths.native_target_key("linux", "arm64") is None
    assert paths.native_target_key("freebsd", "x64") is None


def test_resolver_never_selects_native_components_outside_go_authority(tmp_path: Path) -> None:
    _write_native_stage(tmp_path)

    assert paths.resolve_canonical_native_component(
        tmp_path,
        "data-engine",
        platform_name="darwin",
        machine="arm64",
    ) is None
    assert paths.resolve_canonical_native_component(
        tmp_path,
        "data-engine",
        platform_name="darwin",
        machine="x64",
    ) is None


def test_receipt_v6_mirror_validates_a_complete_controlled_bundle(tmp_path: Path) -> None:
    _write_native_stage(tmp_path)

    _validate_native_stage(tmp_path)


def test_resolver_rejects_binary_and_receipt_symlinks(tmp_path: Path) -> None:
    binaries = _write_native_stage(tmp_path)
    binary = binaries["cleaning-ops"]
    replacement = tmp_path / "replacement"
    replacement.write_bytes(binary.read_bytes())
    replacement.chmod(0o755)
    binary.unlink()
    binary.symlink_to(replacement)

    with pytest.raises(ValueError, match="executable regular file"):
        _validate_native_stage(tmp_path)
    assert paths.resolve_canonical_native_component(
        tmp_path,
        "cleaning-ops",
        platform_name="darwin",
        machine="arm64",
    ) is None

    explicit = tmp_path / "explicit"
    explicit.symlink_to(replacement)
    assert paths.resolve_explicit_native_binary(str(explicit), tmp_path) is None


def test_explicit_binary_is_never_an_execution_authority(tmp_path: Path) -> None:
    binary = tmp_path / "bin" / "analytix-data-engine"
    binary.parent.mkdir()
    binary.write_text("native", encoding="utf-8")
    if os.name != "nt":
        binary.chmod(0o644)
        assert paths.resolve_explicit_native_binary(str(binary), tmp_path) is None
        binary.chmod(0o755)
    assert paths.resolve_explicit_native_binary(str(binary), tmp_path) is None


def test_source_closure_includes_nested_target_and_cargo_context(tmp_path: Path) -> None:
    _write_native_stage(tmp_path)
    before = paths._source_set_digest(str(tmp_path))
    nested_target = tmp_path / paths._COMPONENTS[0][1] / "src" / "target" / "included.bin"
    nested_target.parent.mkdir(parents=True)
    nested_target.write_bytes(b"included source input")
    with_nested_target = paths._source_set_digest(str(tmp_path))
    assert with_nested_target != before

    cargo_config = tmp_path / ".cargo" / "config.toml"
    cargo_config.parent.mkdir()
    cargo_config.write_text('[build]\nrustflags = ["-C", "opt-level=2"]\n', encoding="utf-8")
    assert paths._source_set_digest(str(tmp_path)) != with_nested_target
    cargo_config.unlink()
    cargo_config.symlink_to("missing-config")
    try:
        paths._source_set_digest(str(tmp_path))
    except ValueError as error:
        assert "symlink" in str(error)
    else:
        raise AssertionError("broken Cargo config symlink was accepted as a missing input")

    unicode_root = tmp_path / "unicode"
    unicode_root.mkdir()
    (unicode_root / "\uE000.txt").write_text("bmp", encoding="utf-8")
    (unicode_root / "\U00010000.txt").write_text("astral", encoding="utf-8")
    assert paths._source_tree_digest(str(unicode_root)) == (
        "ee16b29c304cb973ad21a41f0f0d54169fd73631da1c37e9f2139abef7c4acda"
    )


def test_source_closure_rejects_symlinked_ancestor(tmp_path: Path) -> None:
    real_root = tmp_path / "real"
    alias_root = tmp_path / "alias"
    source_root = real_root / paths._COMPONENTS[0][1]
    source_root.mkdir(parents=True)
    (source_root / "Cargo.toml").write_text('[package]\nname = "linked"\n', encoding="utf-8")
    alias_root.mkdir()
    (alias_root / "tools").symlink_to(real_root / "tools", target_is_directory=True)

    try:
        paths._source_tree_digest(
            str(alias_root / paths._COMPONENTS[0][1]),
            str(alias_root),
        )
    except ValueError as error:
        assert "symlink" in str(error)
    else:
        raise AssertionError("symlinked provenance ancestor was accepted")


def test_resolver_validates_the_complete_ordered_native_bundle(tmp_path: Path) -> None:
    binaries = _write_native_stage(tmp_path)
    sibling = binaries["import-accelerator"]
    payload = bytearray(sibling.read_bytes())
    payload[0x1F0] ^= 0xFF
    sibling.write_bytes(payload)

    with pytest.raises(ValueError, match="payload digest mismatch"):
        _validate_native_stage(tmp_path)
    assert paths.resolve_canonical_native_component(
        tmp_path,
        "data-engine",
        platform_name="darwin",
        machine="arm64",
    ) is None


def test_receipt_v6_mirror_rejects_every_authority_and_identity_downgrade(tmp_path: Path) -> None:
    _write_native_stage(tmp_path)
    receipt_path = (
        tmp_path / "runtime" / "data-native" / "darwin-arm64" / paths._RECEIPT_FILE_NAME
    )
    original = receipt_path.read_text(encoding="utf-8")
    mutations: tuple[tuple[str, tuple[str | int, ...], object], ...] = (
        ("old schema", ("schemaVersion",), 5),
        ("authority schema", ("executionAuthority", "schemaVersion"), 2),
        ("unknown trust", ("executionAuthority", "trustClass"), "release"),
        ("authority protocol", ("executionAuthority", "protocol"), "unknown"),
        ("authority target", ("executionAuthority", "targetKey"), "linux-x64"),
        ("authority digest", ("executionAuthority", "binarySha256"), "0" * 64),
        ("authority size", ("executionAuthority", "binarySize"), 0),
        ("authority source", ("executionAuthority", "sourceSetSha256"), "0"),
        ("authority environment", ("executionAuthority", "buildEnvironmentSha256"), "0"),
        ("authority toolchain", ("executionAuthority", "goToolchainKey"), "darwin-x64"),
        ("authority go", ("executionAuthority", "goExecutableSha256"), "0"),
        ("publication missing", ("publicationAuthority",), None),
        ("publication schema", ("publicationAuthority", "schemaVersion"), 2),
        ("publication trust", ("publicationAuthority", "trustClass"), "local"),
        ("publication protocol", ("publicationAuthority", "protocol"), "unknown"),
        ("publication target", ("publicationAuthority", "targetKey"), "linux-x64"),
        ("publication execution", ("publicationAuthority", "cargoExecutionId"), "0"),
        (
            "publication receipt",
            ("publicationAuthority", "cargoExecutionReceiptSha256"),
            "0",
        ),
        ("publication binding", ("publicationAuthority", "publicationBindingSha256"), "0"),
        ("publication unknown", ("publicationAuthority", "unknown"), True),
        ("raw digest", ("components", 0, "rawBuildSha256"), "0"),
        ("raw size", ("components", 0, "rawBuildSize"), 0),
        ("raw size bound", ("components", 0, "rawBuildSize"), paths._MAX_NATIVE_BINARY_BYTES + 1),
        ("staged digest", ("components", 0, "stagedImageSha256"), "0"),
        ("staged size", ("components", 0, "stagedImageSize"), 0),
        (
            "staged size bound",
            ("components", 0, "stagedImageSize"),
            paths._MAX_NATIVE_BINARY_BYTES + 1,
        ),
        ("payload exceeds raw", ("components", 0, "rawBuildSize"), 1),
        ("payload exceeds staged", ("components", 0, "stagedImageSize"), 1),
        ("probe kind", ("components", 0, "executionProbe", "kind"), "unknown"),
        ("probe schema", ("components", 0, "executionProbe", "schema_version"), 2),
        ("failed probe", ("components", 0, "executionProbe", "status"), "failed"),
        ("component", ("components", 0, "executionProbe", "component_id"), "cleaning-ops"),
        ("nonce", ("components", 0, "executionProbe", "request_nonce"), "0"),
        ("binary digest", ("components", 0, "executionProbe", "executable_sha256"), "0" * 64),
        ("binary size", ("components", 0, "executionProbe", "executable_size"), 1),
        ("manifest", ("components", 0, "executionProbe", "manifest_sha256"), "0" * 64),
        ("policy", ("components", 0, "executionProbe", "policy_sha256"), "0" * 64),
        ("authority", ("components", 0, "executionProbe", "authority_sha256"), "0" * 64),
        ("host", ("components", 0, "executionProbe", "host_arch"), "x64"),
        ("host platform", ("components", 0, "executionProbe", "host_platform"), "linux"),
        ("image", ("components", 0, "executionProbe", "loaded_image_bound"), False),
        ("cwd", ("components", 0, "executionProbe", "working_directory_bound"), False),
        ("guardian", ("components", 0, "executionProbe", "guardian_authenticated"), False),
        ("tree", ("components", 0, "executionProbe", "process_tree_empty"), False),
        ("unknown field", ("components", 0, "executionProbe", "unknown"), True),
    )
    for name, path, replacement in mutations:
        receipt = json.loads(original)
        _set_nested_receipt_value(receipt, path, replacement)
        receipt_path.write_text(
            json.dumps(receipt, ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
        )
        with pytest.raises(ValueError) as captured:
            _validate_native_stage(tmp_path)
        assert str(captured.value), name
        receipt_path.write_text(original, encoding="utf-8")

    cross_target_replay = json.loads(original)
    cross_target_replay["executionAuthority"]["targetKey"] = "darwin-x64"
    cross_target_replay["executionAuthority"]["goToolchainKey"] = "darwin-x64"
    for component in cross_target_replay["components"]:
        component["executionProbe"]["host_arch"] = "x64"
    receipt_path.write_text(
        json.dumps(cross_target_replay, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    with pytest.raises(ValueError, match="build environment receipt"):
        _validate_native_stage(tmp_path)


def test_resolver_rejects_duplicate_receipt_keys_and_stage_ancestor_symlink(tmp_path: Path) -> None:
    _write_native_stage(tmp_path)
    receipt = tmp_path / "runtime" / "data-native" / "darwin-arm64" / paths._RECEIPT_FILE_NAME
    receipt.write_text(
        receipt.read_text(encoding="utf-8").replace(
            '{\n  "schemaVersion": 6,',
            '{\n  "schemaVersion": 6,\n  "schemaVersion": 6,',
            1,
        ),
        encoding="utf-8",
    )
    with pytest.raises(ValueError, match="duplicate JSON key"):
        paths._load_json_without_duplicates(receipt.read_text(encoding="utf-8"))
    assert paths.resolve_canonical_native_component(
        tmp_path,
        "data-engine",
        platform_name="darwin",
        machine="arm64",
    ) is None

    linked_root = tmp_path / "linked"
    _write_native_stage(linked_root)
    real_runtime = tmp_path / "real-runtime"
    (linked_root / "runtime").rename(real_runtime)
    (linked_root / "runtime").symlink_to(real_runtime, target_is_directory=True)
    assert paths.resolve_canonical_native_component(
        linked_root,
        "data-engine",
        platform_name="darwin",
        machine="arm64",
    ) is None
