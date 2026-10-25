"""Fixed-source SQLGlot 26/30 comparison, separate from combined upgrades."""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tarfile
import time
import tomllib
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET
import zipfile

TRUSTED = "4084b7aba1058dac480633faa59223d21edaa8c5"
CORPUS = "343bec8a9e175420bf22d8ba851f29092f70837ea5d3617c653d6ea33d49d7d9"
WHEEL = "https://files.pythonhosted.org/packages/68/5f/d78f74306cc026c5f40f8808a47356dfd37461eb15eb759a20e5e9efe7d7/sqlglot-30.24.0-py3-none-any.whl"
WHEEL_SHA = "9488828cc018453b7a3ba01e65d2749c9a731314c14583695163ca1f358203fe"

ARM = r"""
import hashlib, importlib.metadata as metadata, json, pathlib, sys
overlay, source, expected, corpus, report, environment = sys.argv[1:]
sys.path.insert(0, source)
if overlay:
    sys.path.insert(0, overlay)
import pytest, sqlglot
from app.repositories import analysis_repository as owner
assert pytest.__version__ == '8.4.2'
assert sqlglot.__version__ == expected == metadata.version('sqlglot')
assert pathlib.Path(owner.__file__).resolve() == pathlib.Path(source, 'app/repositories/analysis_repository.py').resolve()
assert pathlib.Path(sqlglot.__file__).resolve().is_relative_to(pathlib.Path(overlay or environment).resolve())
rows = []
for distribution in metadata.distributions():
    name = distribution.metadata['Name'].lower().replace('_', '-')
    if name == 'sqlglot':
        continue
    content = '\n'.join(distribution.read_text(p) or '' for p in ['METADATA', 'RECORD'])
    rows.append([name, distribution.version, hashlib.sha256(content.encode()).hexdigest()])
fingerprint = hashlib.sha256(json.dumps(sorted(rows), separators=(',', ':')).encode()).hexdigest()
code = pytest.main([corpus, '-q', '--junitxml=' + report])
print('PROBE=' + json.dumps({'version': expected, 'pytest': pytest.__version__, 'other_installed_fingerprint': fingerprint, 'other_installed_count': len(rows)}))
raise SystemExit(code)
"""


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def private_file(path: Path, data: bytes) -> None:
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as output:
        output.write(data)


def private_directory(path: Path) -> None:
    path.mkdir(mode=0o700)
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o700 or info.st_uid != os.getuid():
        raise ValueError("private_directory_identity")
    if any("posix_acl" in name for name in os.listxattr(path)):
        raise ValueError("private_directory_acl")
    if sys.platform == "darwin":
        acl = subprocess.run(["/bin/ls", "-lde", str(path)], capture_output=True, text=True, check=True)
        if len(acl.stdout.splitlines()) != 1:
            raise ValueError("private_directory_acl")


def member_path(root: Path, name: str) -> Path:
    relative = Path(name)
    if relative.is_absolute() or ".." in relative.parts:
        raise ValueError("archive_path")
    target = root / relative
    target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    return target


def run(args: list[str], cwd: Path, env: dict[str, str], log: Path, timeout: int = 180) -> subprocess.CompletedProcess:
    result = subprocess.run(args, cwd=cwd, env=env, capture_output=True, timeout=timeout)
    private_file(log, (result.stdout + result.stderr)[:512 * 1024])
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--trusted-source", required=True, choices=[TRUSTED])
    parser.add_argument("--corpus", required=True)
    parser.add_argument("--work-dir", required=True, type=Path)
    parser.add_argument("--python", required=True)
    parser.add_argument("--uv", required=True)
    options = parser.parse_args()
    report = {"status": "fail", "stage": "inputs"}
    started = time.monotonic()
    try:
        checkout = Path.cwd()
        corpus = Path(options.corpus)
        if corpus.is_symlink() or not corpus.is_file() or digest(corpus.read_bytes()) != CORPUS:
            raise ValueError("corpus_identity")
        if subprocess.run(["git", "diff", "--quiet", TRUSTED, "--", "backend/app"], cwd=checkout, capture_output=True).returncode:
            raise ValueError("product_source_drift")
        os.umask(0o077)
        root = options.work_dir.absolute()
        private_directory(root)
        for name in ["source", "overlay", "uv-cache", "tmp"]:
            private_directory(root / name)
        env = dict(os.environ, UV_CACHE_DIR=str(root / "uv-cache"), TMPDIR=str(root / "tmp"),
                   UV_PROJECT_ENVIRONMENT=str(root / "base-venv"), UV_PYTHON_DOWNLOADS="never", UV_NO_CONFIG="1")
        report["stage"] = "trusted_archive"
        archive = subprocess.run(["git", "archive", "--format=tar", TRUSTED, "--", "backend"],
                                 cwd=checkout, capture_output=True, check=True, timeout=30).stdout
        if len(archive) > 64 * 1024 * 1024:
            raise ValueError("archive_size")
        with tarfile.open(fileobj=io.BytesIO(archive)) as source:
            for member in source:
                if not (member.name == "backend" or member.name.startswith("backend/")) or not (member.isdir() or member.isfile()):
                    raise ValueError("archive_member")
                target = member_path(root / "source", member.name)
                if member.isdir():
                    target.mkdir(mode=0o700, exist_ok=True)
                else:
                    private_file(target, source.extractfile(member).read())
        backend = root / "source/backend"
        lock_hash = digest((backend / "uv.lock").read_bytes())
        packages = tomllib.loads((backend / "uv.lock").read_text())["package"]
        if [package["version"] for package in packages if package["name"] == "sqlglot"] != ["26.33.0"]:
            raise ValueError("trusted_sqlglot_version")
        other_locked_nodes = len([package for package in packages if package["name"] != "sqlglot"])
        copied_corpus = backend / "tests/test_analysis_sqlglot_compatibility.py"
        private_file(copied_corpus, corpus.read_bytes())
        report["stage"] = "base_resolution"
        install = run([options.uv, "sync", "--project", str(backend), "--locked", "--extra", "dev",
                       "--python", options.python], checkout, env, root / "base-install.log", timeout=600)
        if install.returncode:
            raise ValueError("base_install_failed")
        report["stage"] = "wheel_hash"
        with urllib.request.urlopen(WHEEL, timeout=30) as response:
            if urllib.parse.urlparse(response.geturl()).hostname != "files.pythonhosted.org":
                raise ValueError("wheel_origin")
            wheel = response.read(2 * 1024 * 1024 + 1)
        if len(wheel) > 2 * 1024 * 1024 or digest(wheel) != WHEEL_SHA:
            raise ValueError("wheel_integrity")
        private_file(root / "sqlglot-30.24.0.whl", wheel)
        with zipfile.ZipFile(io.BytesIO(wheel)) as package:
            for member in package.infolist():
                if stat.S_ISLNK(member.external_attr >> 16) or not member.filename.startswith(("sqlglot/", "sqlglot-30.24.0.dist-info/")):
                    raise ValueError("wheel_member")
                target = member_path(root / "overlay", member.filename)
                if member.is_dir():
                    target.mkdir(mode=0o700, exist_ok=True)
                else:
                    private_file(target, package.read(member))
        results = []
        for arm, overlay, version in [("old", "", "26.33.0"), ("new", str(root / "overlay"), "30.24.0")]:
            report["stage"] = arm + "_corpus"
            xml = root / (arm + ".xml")
            result = run([str(root / "base-venv/bin/python"), "-I", "-B", "-c", ARM, overlay,
                          str(backend), version, str(copied_corpus), str(xml), str(root / "base-venv")],
                         backend, env, root / (arm + ".log"))
            cases = list(ET.parse(xml).iter("testcase")) if xml.is_file() else []
            if result.returncode or len(cases) != 24 or any(list(case) for case in cases):
                report["failed_tests"] = [case.get("name", "").split("[")[0] for case in cases if list(case)][:4]
                raise ValueError(arm + "_corpus_failed")
            lines = [line[6:] for line in result.stdout.decode().splitlines() if line.startswith("PROBE=")]
            if len(lines) != 1:
                raise ValueError("arm_receipt")
            results.append({**json.loads(lines[0]), "cases": sorted(case.get("name") for case in cases)})
        report["stage"] = "equality"
        if (results[0]["other_installed_fingerprint"] != results[1]["other_installed_fingerprint"]
                or results[0]["cases"] != results[1]["cases"]
                or digest((backend / "uv.lock").read_bytes()) != lock_hash):
            raise ValueError("comparison_inputs_changed")
        report.update(status="pass", source=TRUSTED, corpus_sha256=CORPUS, wheel_sha256=WHEEL_SHA,
                      base_lock_sha256=lock_hash, cases_per_arm=24, skipped=0,
                      other_locked_nodes=other_locked_nodes,
                      other_installed_count=results[0]["other_installed_count"],
                      other_installed_fingerprint=results[0]["other_installed_fingerprint"],
                      versions=[row["version"] for row in results], pytest="8.4.2")
    except Exception as error:
        report["error"] = str(error) if isinstance(error, ValueError) else type(error).__name__
    report["seconds"] = round(time.monotonic() - started, 3)
    print(json.dumps(report, sort_keys=True))
    return 0 if report["status"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
