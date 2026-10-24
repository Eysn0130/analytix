"""Verify the exact Windows lxml target under an explicit lock contract."""
from __future__ import annotations

import argparse
import email.parser
import hashlib
import importlib
import importlib.machinery
import json
import os
from pathlib import Path, PurePosixPath, PureWindowsPath
import re
import shutil
import stat
import struct
import subprocess
import sys
import urllib.request
import zipfile

VERSION = "6.1.3"
FILENAME = "lxml-6.1.3-cp311-cp311-win_amd64.whl"
SIZE = 4032700
SHA256 = "7ae4949f212a53b007dbc355884fda122545c5764a54256c9217e419a62a6559"
URL = "https://files.pythonhosted.org/packages/45/5a/15531a0d307c96282fe8b639b3d74e8bd783e4ab4cb2b0781146ac4161b8/" + FILENAME
OLD_PIN = ("6.1.1", "abbefa31eee84842140f67acef1c828e28bba8bbf0c3bc6e5492a9af88152c28")
OLD_REQUIREMENTS_SHA256 = "1f8dfa741fb95e082a26f6766389a1b6196d0efa03807cea591efdeaa4b176d8"
OLD_RUNTIME_LOCK_SHA256 = "d4c039a542277f724ab7ec7487c1b63d908dff42a686fb6c1a834401ebbda033"


class ProbeError(Exception):
    def __init__(self, code: str):
        self.code = code
        super().__init__(code)


def require(condition: bool, code: str) -> None:
    if not condition:
        raise ProbeError(code)


def lock_contract(repo: Path, mode: str) -> str:
    requirements = (repo / "scripts/windows-backend-requirements.lock.txt").read_bytes()
    require(len(requirements) <= 65536, "requirements_too_large")
    pins = [line for line in requirements.decode("utf-8").splitlines() if line.startswith("lxml")]
    require(len(pins) == 1, "lock_lxml_pin_not_unique")
    match = re.fullmatch(r"lxml==(\d+\.\d+\.\d+) --hash=sha256:([0-9a-f]{64})", pins[0])
    require(match is not None, "lock_lxml_pin_invalid")
    pin = match.groups()
    lock_bytes = (repo / "scripts/windows-python-runtime-lock.json").read_bytes()
    if mode == "future-target":
        require(pin == OLD_PIN, "future_target_old_pin_required")
        require(hashlib.sha256(requirements).hexdigest() == OLD_REQUIREMENTS_SHA256 and hashlib.sha256(lock_bytes).hexdigest() == OLD_RUNTIME_LOCK_SHA256, "future_target_baseline_mismatch")
    else:
        require(pin == (VERSION, SHA256), "accepted_target_pin_required")
    lock = json.loads(lock_bytes)
    require(lock["sitePackages"]["requirementsLockSha256"] == hashlib.sha256(requirements).hexdigest(), "lock_requirements_hash_mismatch")
    require(lock["sitePackages"]["platform"] == "win_amd64" and lock["pythonRuntime"]["series"] == "3.11", "lock_platform_mismatch")
    # The carrier must switch its workflow to accepted-required when updating locks.
    return mode


def wheel_input(wheel: Path, expected_version: str) -> None:
    require(wheel.exists(), "wheel_missing")
    info = wheel.lstat()
    require(stat.S_ISREG(info.st_mode) and not wheel.is_symlink() and info.st_nlink == 1, "wheel_not_regular")
    require(wheel.name == FILENAME, "wheel_filename_mismatch")
    require(info.st_size == SIZE, "wheel_size_mismatch")
    data = wheel.read_bytes()
    require(hashlib.sha256(data).hexdigest() == SHA256, "wheel_sha256_mismatch")
    with zipfile.ZipFile(wheel) as archive:
        metadata = email.parser.BytesParser().parsebytes(archive.read(f"lxml-{VERSION}.dist-info/METADATA"))
        require(metadata["Name"] == "lxml" and metadata["Version"] == expected_version == VERSION, "wheel_version_mismatch")
        tags = email.parser.BytesParser().parsebytes(archive.read(f"lxml-{VERSION}.dist-info/WHEEL"))
        require(tags["Root-Is-Purelib"] == "false" and tags.get_all("Tag") == ["cp311-cp311-win_amd64"], "wheel_tag_mismatch")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *_args, **_kwargs):
        raise ProbeError("wheel_download_redirect")


def download(directory: Path) -> Path:
    directory.mkdir(mode=0o700)
    partial = directory / (FILENAME + ".part")
    opener = urllib.request.build_opener(NoRedirect())
    with opener.open(URL, timeout=30) as response, partial.open("xb") as output:
        total = 0
        while chunk := response.read(65536):
            total += len(chunk)
            require(total <= SIZE, "wheel_size_mismatch")
            output.write(chunk)
    require(partial.stat().st_size == SIZE and hashlib.sha256(partial.read_bytes()).hexdigest() == SHA256, "wheel_download_identity_mismatch")
    wheel = directory / FILENAME
    partial.rename(wheel)
    return wheel


def extract(wheel: Path, destination: Path) -> None:
    destination.mkdir(mode=0o700)
    with zipfile.ZipFile(wheel) as archive:
        members = archive.infolist()
        require(len(members) <= 2048 and sum(item.file_size for item in members) <= 64 * 1024 * 1024, "wheel_inventory_budget")
        seen = set()
        for item in members:
            name = item.filename
            relative = PurePosixPath(name)
            require(bool(name) and "\\" not in name and not relative.is_absolute() and not PureWindowsPath(name).drive and ".." not in relative.parts, "wheel_member_path")
            key = name.casefold()
            require(key not in seen, "wheel_member_duplicate")
            seen.add(key)
            mode = (item.external_attr >> 16) & 0o170000
            require(mode in (0, stat.S_IFREG, stat.S_IFDIR), "wheel_member_type")
            target = destination.joinpath(*relative.parts)
            if item.is_dir():
                target.mkdir(mode=0o700, parents=True, exist_ok=True)
            else:
                target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
                with archive.open(item) as source, target.open("xb") as output:
                    shutil.copyfileobj(source, output)


def native_probe(wheel: Path, work: Path, expected_version: str) -> dict:
    wheel_input(wheel, expected_version)
    require(sys.platform == "win32" and struct.calcsize("P") == 8, "windows_native_required")
    require(sys.version_info[:2] == (3, 11), "python_version_mismatch")
    require(sys.flags.isolated and sys.dont_write_bytecode, "isolated_python_required")
    require(not any(name == "lxml" or name.startswith("lxml.") for name in sys.modules), "lxml_preloaded")
    work.mkdir(mode=0o700)
    site = work / "site"
    extract(wheel, site)
    sys.path.insert(0, str(site))
    lxml = importlib.import_module("lxml")
    etree = importlib.import_module("lxml.etree")
    origin = Path(etree.__file__).resolve()
    require(origin.is_relative_to(site.resolve()) and origin.suffix.lower() == ".pyd", "native_origin_mismatch")
    require(isinstance(etree.__loader__, importlib.machinery.ExtensionFileLoader), "native_loader_mismatch")
    require(lxml.__version__ == VERSION and tuple(etree.LXML_VERSION[:3]) == (6, 1, 3), "native_version_mismatch")
    runtime, compiled = tuple(etree.LIBXML_VERSION), tuple(etree.LIBXML_COMPILED_VERSION)
    require(len(runtime) == len(compiled) == 3 and runtime[0] > 0 and runtime == compiled, "libxml_version_mismatch")
    xml = b'<!DOCTYPE root [<!ENTITY word "bounded-value">]><root>&word;</root>'
    resolved = etree.fromstring(xml, etree.XMLParser(resolve_entities=True, no_network=True))
    require(resolved.text == "bounded-value", "internal_entity_positive_failed")
    preserved = etree.fromstring(xml, etree.XMLParser(resolve_entities=False, no_network=True))
    require(preserved.text != "bounded-value" and b"&word;" in etree.tostring(preserved), "disabled_entity_negative_failed")

    class RejectExternal(etree.Resolver):
        calls = 0

        def resolve(self, *_args):
            self.calls += 1
            raise ProbeError("external_resolution_attempted")

    parser = etree.XMLParser(resolve_entities=False, load_dtd=False, no_network=True)
    resolver = RejectExternal()
    parser.resolvers.add(resolver)
    external = etree.fromstring(b'<!DOCTYPE root [<!ENTITY blocked SYSTEM "urn:analytix:synthetic:never-resolve">]><root>&blocked;</root>', parser)
    require(resolver.calls == 0 and b"&blocked;" in etree.tostring(external), "external_entity_negative_failed")
    try:
        etree.fromstring(b"<root>&undefined;</root>")
    except etree.XMLSyntaxError:
        pass
    else:
        raise ProbeError("undefined_entity_not_rejected")
    return {"native": True, "platform": sys.platform, "python": list(sys.version_info[:3]), "wheel_sha256": SHA256, "lxml": lxml.__version__, "native_origin_matches": True, "native_module": origin.relative_to(site.resolve()).as_posix(), "native_loader": type(etree.__loader__).__name__, "libxml_runtime": runtime, "libxml_compiled": compiled, "xml_checks": ["internal-entity-resolved", "entities-disabled", "external-resolver-unused", "undefined-entity-rejected"]}


def negative_controls(wheel: Path, work: Path, repo: Path, lock_mode: str) -> list:
    checks = []
    for label, target, version, code in [("missing-wheel", work / "missing.whl", VERSION, "wheel_missing"), ("wrong-version", wheel, "6.1.2", "wheel_version_mismatch")]:
        command = [sys.executable, "-I", "-B", str(Path(__file__).resolve()), "probe", "--wheel", str(target), "--work-dir", str(work / label), "--expected-version", version, "--repo-root", str(repo), "--lock-mode", lock_mode]
        result = subprocess.run(command, text=True, capture_output=True, timeout=30)
        try:
            error = json.loads(result.stderr)
        except ValueError:
            raise ProbeError("negative_control_missing_error_code") from None
        require(result.returncode == 2 and error.get("code") == code, "negative_control_not_rejected")
        checks.append({"case": label, "exit": result.returncode, "code": code})
    return checks


def main() -> dict:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("run", "controls", "probe", "contract"))
    parser.add_argument("--lock-mode", choices=("future-target", "accepted-required"), required=True)
    parser.add_argument("--wheel", type=Path)
    parser.add_argument("--work-dir", type=Path)
    parser.add_argument("--expected-version", default=VERSION)
    parser.add_argument("--repo-root", type=Path, default=Path(__file__).resolve().parents[1])
    args = parser.parse_args()
    os.umask(0o077)
    mode = lock_contract(args.repo_root, args.lock_mode)
    if args.mode == "contract":
        return {"status": "pass", "lock_mode": mode, "target_version": VERSION}
    require(args.work_dir is not None, "work_directory_required")
    if args.mode == "probe":
        require(args.wheel is not None, "wheel_argument_required")
        return {"status": "pass", "lock_mode": mode, **native_probe(args.wheel, args.work_dir, args.expected_version)}
    args.work_dir.mkdir(mode=0o700)
    wheel = args.wheel if args.wheel is not None else download(args.work_dir / "wheelhouse")
    wheel_input(wheel, VERSION)
    checks = negative_controls(wheel, args.work_dir, args.repo_root, args.lock_mode)
    result = {"status": "pass", "lock_mode": mode, "input_negative_controls": checks, "native": False}
    if args.mode == "run":
        result.update(native_probe(wheel, args.work_dir / "native", VERSION))
    with (args.work_dir / "receipt.json").open("x", encoding="utf-8") as output:
        json.dump(result, output, indent=2)
        output.write("\n")
    return result


if __name__ == "__main__":
    try:
        print(json.dumps(main()))
    except ProbeError as error:
        print(json.dumps({"status": "error", "code": error.code}), file=sys.stderr)
        sys.exit(2)
    except Exception as error:
        print(json.dumps({"status": "error", "code": "unexpected_error", "type": type(error).__name__}), file=sys.stderr)
        sys.exit(3)
