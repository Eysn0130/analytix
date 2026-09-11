from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path

import pytest

from app.core import external_runtime


def _sha256(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def _macho(import_name: str = "") -> bytes:
    payload = bytearray(0x220)
    payload[0:4] = (0xFEEDFACF).to_bytes(4, "little")
    payload[4:8] = (0x0100000C).to_bytes(4, "little")
    payload[12:16] = (2).to_bytes(4, "little")
    import_size = ((24 + len(import_name.encode("utf-8")) + 1 + 7) // 8) * 8 if import_name else 0
    payload[16:20] = (4 if import_name else 3).to_bytes(4, "little")
    payload[20:24] = (160 + import_size).to_bytes(4, "little")
    payload[32:36] = (0x19).to_bytes(4, "little")
    payload[36:40] = (72).to_bytes(4, "little")
    payload[40:46] = b"__TEXT"
    payload[80:88] = (0x180).to_bytes(8, "little")
    payload[88:92] = (7).to_bytes(4, "little")
    payload[92:96] = (5).to_bytes(4, "little")
    link_edit = 104
    payload[link_edit:link_edit + 4] = (0x19).to_bytes(4, "little")
    payload[link_edit + 4:link_edit + 8] = (72).to_bytes(4, "little")
    payload[link_edit + 8:link_edit + 18] = b"__LINKEDIT"
    payload[link_edit + 32:link_edit + 40] = (len(payload) - 0x180).to_bytes(8, "little")
    payload[link_edit + 40:link_edit + 48] = (0x180).to_bytes(8, "little")
    payload[link_edit + 48:link_edit + 56] = (len(payload) - 0x180).to_bytes(8, "little")
    payload[link_edit + 56:link_edit + 60] = (1).to_bytes(4, "little")
    payload[link_edit + 60:link_edit + 64] = (1).to_bytes(4, "little")
    signature = 176
    if import_name:
        payload[signature:signature + 4] = (0x0C).to_bytes(4, "little")
        payload[signature + 4:signature + 8] = import_size.to_bytes(4, "little")
        payload[signature + 8:signature + 12] = (24).to_bytes(4, "little")
        payload[signature + 24:signature + 24 + len(import_name)] = import_name.encode("utf-8")
        signature += import_size
    payload[signature:signature + 4] = (0x1D).to_bytes(4, "little")
    payload[signature + 4:signature + 8] = (16).to_bytes(4, "little")
    payload[signature + 8:signature + 12] = (0x200).to_bytes(4, "little")
    payload[signature + 12:signature + 16] = (0x20).to_bytes(4, "little")
    payload[0x200:0x204] = (0xFADE0CC0).to_bytes(4, "big")
    payload[0x204:0x208] = (0x20).to_bytes(4, "big")
    payload[0x208:0x20C] = (1).to_bytes(4, "big")
    payload[0x210:0x214] = (0x14).to_bytes(4, "big")
    return bytes(payload)


def _pe(import_name: str = "") -> bytes:
    payload = bytearray(0x400)
    payload[0:2] = b"MZ"
    pe_offset = 0x80
    payload[0x3C:0x40] = pe_offset.to_bytes(4, "little")
    payload[pe_offset:pe_offset + 4] = b"PE\0\0"
    payload[pe_offset + 4:pe_offset + 6] = (0x8664).to_bytes(2, "little")
    payload[pe_offset + 6:pe_offset + 8] = (1).to_bytes(2, "little")
    payload[pe_offset + 20:pe_offset + 22] = (0xF0).to_bytes(2, "little")
    payload[pe_offset + 22:pe_offset + 24] = (0x22).to_bytes(2, "little")
    optional = pe_offset + 24
    payload[optional:optional + 2] = (0x20B).to_bytes(2, "little")
    payload[optional + 16:optional + 20] = (0x1000).to_bytes(4, "little")
    payload[optional + 32:optional + 36] = (0x1000).to_bytes(4, "little")
    payload[optional + 36:optional + 40] = (0x200).to_bytes(4, "little")
    payload[optional + 56:optional + 60] = (0x2000).to_bytes(4, "little")
    payload[optional + 60:optional + 64] = (0x200).to_bytes(4, "little")
    payload[optional + 108:optional + 112] = (16).to_bytes(4, "little")
    if import_name:
        payload[optional + 120:optional + 124] = (0x1100).to_bytes(4, "little")
        payload[optional + 124:optional + 128] = (40).to_bytes(4, "little")
    section = optional + 0xF0
    payload[section:section + 5] = b".text"
    payload[section + 8:section + 12] = (0x100).to_bytes(4, "little")
    payload[section + 12:section + 16] = (0x1000).to_bytes(4, "little")
    payload[section + 16:section + 20] = (0x200).to_bytes(4, "little")
    payload[section + 20:section + 24] = (0x200).to_bytes(4, "little")
    payload[section + 36:section + 40] = (0x60000020).to_bytes(4, "little")
    if import_name:
        payload[0x30C:0x310] = (0x1150).to_bytes(4, "little")
        payload[0x350:0x350 + len(import_name)] = import_name.encode("ascii")
    return bytes(payload)


def _canonical(value: object) -> bytes:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode("utf-8")


def _write(path: Path, content: bytes, *, executable: bool = False) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(content)
    path.chmod(0o700 if executable else 0o600)


def _authorize(root: Path, manifest: dict[str, object]) -> tuple[bytes, str]:
    manifest_bytes = _canonical(manifest)
    _write(root / external_runtime._DOCUMENT_MANIFEST_NAME, manifest_bytes)
    lock_entry = {
        "targetKey": manifest["target"]["key"],
        "assetReleaseId": "python-test-assets",
        "manifestSha256": _sha256(manifest_bytes),
        "treeSha256": manifest["treeSha256"],
        "sourceSetSha256": _sha256(external_runtime._DOCUMENT_SOURCE_SET_DOMAIN + _canonical(manifest["sources"])),
        "licenseSetSha256": _sha256(external_runtime._DOCUMENT_LICENSE_SET_DOMAIN + _canonical(manifest["licenses"])),
    }
    lock = {
        "schemaVersion": 1,
        "contract": external_runtime._DOCUMENT_AUTHORITY_LOCK_CONTRACT,
        "targets": [lock_entry],
    }
    lock_bytes = _canonical(lock)
    _write(root / external_runtime._DOCUMENT_AUTHORITY_LOCK_NAME, lock_bytes)
    return lock_bytes, _sha256(lock_bytes)


def _fixture(resource_root: Path, platform_name: str) -> tuple[Path, Path, Path, dict[str, object], str]:
    root = resource_root / "runtime" / "document-runtime"
    target = (
        {"key": "darwin-arm64", "platform": "darwin", "arch": "arm64", "triple": "aarch64-apple-darwin"}
        if platform_name == "darwin"
        else {"key": "win32-x64", "platform": "win32", "arch": "x64", "triple": "x86_64-pc-windows-msvc"}
    )
    extension = ".exe" if platform_name == "win32" else ""
    library_name = "document-runtime-helper.dll" if platform_name == "win32" else "document-runtime-helper.dylib"
    library_path = f"lib/{library_name}"
    import_name = library_name if platform_name == "win32" else f"@loader_path/../{library_path}"
    factory = _pe if platform_name == "win32" else _macho
    paths = {
        f"bin/soffice{extension}": ("binary", factory(import_name), [library_path]),
        f"bin/tesseract{extension}": ("binary", factory(import_name), [library_path]),
        library_path: ("library", factory(), []),
        "licenses/document-runtime.LICENSE": ("license", b"fixture license\n", []),
        "share/tessdata/chi_sim.traineddata": ("data", b"chi-sim-fixture", []),
        "share/tessdata/eng.traineddata": ("data", b"eng-fixture", []),
    }
    files = []
    for relative_path, (kind, content, dependencies) in sorted(paths.items()):
        _write(root / relative_path, content, executable=kind in {"binary", "library"})
        files.append(
            {
                "path": relative_path,
                "kind": kind,
                "sourceId": "document-runtime-source",
                "licenseId": "document-runtime-license",
                "sha256": _sha256(content),
                "byteLength": len(content),
                "platformSignature": (
                    {
                        "kind": "native",
                        "targetKey": target["key"],
                        "format": "mach-o" if platform_name == "darwin" else "pe",
                        "arch": target["arch"],
                        "payloadSha256": _sha256(content),
                        "payloadByteLength": len(content),
                    }
                    if kind in {"binary", "library"}
                    else {"kind": "content", "targetKey": target["key"]}
                ),
                "dependencies": dependencies,
            }
        )
    sources = [
        {
            "id": "document-runtime-source",
            "name": "Document Runtime Fixture",
            "version": "1.0.0",
            "sourceUri": "https://example.invalid/document-runtime.tar.zst",
            "archiveSha256": _sha256(b"source archive"),
        }
    ]
    licenses = [
        {
            "id": "document-runtime-license",
            "spdxExpression": "Apache-2.0",
            "sourceId": "document-runtime-source",
            "path": "licenses/document-runtime.LICENSE",
        }
    ]
    capabilities = {
        "sofficeBinary": f"bin/soffice{extension}",
        "tesseractBinary": f"bin/tesseract{extension}",
        "tessdataDirectory": "share/tessdata",
        "ocrLanguages": ["chi_sim", "eng"],
    }
    tree_payload = {
        "target": target,
        "sources": sources,
        "licenses": licenses,
        "capabilities": capabilities,
        "files": files,
    }
    manifest: dict[str, object] = {
        "schemaVersion": 2,
        "contract": external_runtime._DOCUMENT_CONTRACT,
        **tree_payload,
        "treeSha256": _sha256(external_runtime._DOCUMENT_TREE_DOMAIN + _canonical(tree_payload)),
    }
    _lock_bytes, lock_sha256 = _authorize(root, manifest)
    return root, root / capabilities["sofficeBinary"], root / capabilities["tesseractBinary"], manifest, lock_sha256


@pytest.mark.parametrize("platform_name", ["darwin", "win32"])
def test_document_runtime_v2_verifies_full_tree_authority_and_native_imports(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    platform_name: str,
) -> None:
    resource_root = tmp_path / "resources"
    root, soffice, tesseract, manifest, lock_sha256 = _fixture(resource_root, platform_name)
    monkeypatch.setattr(external_runtime, "trusted_app_resource_root", lambda: resource_root)
    monkeypatch.setattr(external_runtime, "_document_runtime_target", lambda: manifest["target"])
    monkeypatch.setattr(external_runtime, "_DOCUMENT_AUTHORITY_LOCK_SHA256", lock_sha256)
    monkeypatch.setenv("ANALYTIX_DOCUMENT_SOFFICE_BIN", os.fspath(soffice))
    monkeypatch.setenv("ANALYTIX_DOCUMENT_TESSERACT_BIN", os.fspath(tesseract))
    monkeypatch.setenv("TESSDATA_PREFIX", os.fspath(root / "share/tessdata"))

    assert external_runtime.resolve_host_runtime_binary(
        "ANALYTIX_DOCUMENT_SOFFICE_BIN",
        allowed_names={soffice.name},
        require_document_manifest=True,
    ).path == soffice
    assert external_runtime.resolve_host_runtime_binary(
        "ANALYTIX_DOCUMENT_TESSERACT_BIN",
        allowed_names={tesseract.name},
        require_document_manifest=True,
    ).path == tesseract

    (root / "share/tessdata/eng.traineddata").write_bytes(b"tampered")
    assert external_runtime.resolve_host_runtime_binary(
        "ANALYTIX_DOCUMENT_SOFFICE_BIN",
        allowed_names={soffice.name},
        require_document_manifest=True,
    ).reason == "HOST_RUNTIME_INTEGRITY_FAILED"


def test_document_runtime_v2_rejects_manifest_dependency_not_present_in_native_imports(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    resource_root = tmp_path / "resources"
    root, soffice, _tesseract, manifest, _lock_sha256 = _fixture(resource_root, "darwin")
    soffice_entry = next(item for item in manifest["files"] if item["path"] == "bin/soffice")
    soffice_entry["dependencies"] = []
    tree_payload = {key: manifest[key] for key in ("target", "sources", "licenses", "capabilities", "files")}
    manifest["treeSha256"] = _sha256(external_runtime._DOCUMENT_TREE_DOMAIN + _canonical(tree_payload))
    _lock_bytes, lock_sha256 = _authorize(root, manifest)
    monkeypatch.setattr(external_runtime, "trusted_app_resource_root", lambda: resource_root)
    monkeypatch.setattr(external_runtime, "_document_runtime_target", lambda: manifest["target"])
    monkeypatch.setattr(external_runtime, "_DOCUMENT_AUTHORITY_LOCK_SHA256", lock_sha256)
    monkeypatch.setenv("ANALYTIX_DOCUMENT_SOFFICE_BIN", os.fspath(soffice))

    assert external_runtime.resolve_host_runtime_binary(
        "ANALYTIX_DOCUMENT_SOFFICE_BIN",
        allowed_names={"soffice"},
        require_document_manifest=True,
    ).reason == "HOST_RUNTIME_INTEGRITY_FAILED"


def test_document_runtime_v2_rejects_extra_empty_directory(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    resource_root = tmp_path / "resources"
    root, soffice, _tesseract, manifest, lock_sha256 = _fixture(resource_root, "darwin")
    (root / "unlisted-empty-directory").mkdir()
    monkeypatch.setattr(external_runtime, "trusted_app_resource_root", lambda: resource_root)
    monkeypatch.setattr(external_runtime, "_document_runtime_target", lambda: manifest["target"])
    monkeypatch.setattr(external_runtime, "_DOCUMENT_AUTHORITY_LOCK_SHA256", lock_sha256)
    monkeypatch.setenv("ANALYTIX_DOCUMENT_SOFFICE_BIN", os.fspath(soffice))

    assert external_runtime.resolve_host_runtime_binary(
        "ANALYTIX_DOCUMENT_SOFFICE_BIN",
        allowed_names={"soffice"},
        require_document_manifest=True,
    ).reason == "HOST_RUNTIME_INTEGRITY_FAILED"
