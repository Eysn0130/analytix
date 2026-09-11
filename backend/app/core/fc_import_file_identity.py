from __future__ import annotations

import csv
import hashlib
from pathlib import Path
from typing import Dict, List, Optional, Sequence, Tuple

from app.core import fc_import_schema
from app.core.fc_import_projection import (
    header_index as build_header_index,
    header_map as build_header_map,
    normalize_header_for_match,
    sanitize_header,
)
from app.core.import_accelerator import accelerated_file_hashes, detect_encoding as rust_detect_encoding


def detect_file_encoding(path: Path) -> str:
    return rust_detect_encoding(path)


def read_csv_headers_raw(path: Path, encoding: str) -> List[str]:
    with open(path, "r", encoding=encoding, errors="replace", newline="") as handle:
        reader = csv.reader(handle, skipinitialspace=True)
        return next(reader, [])


def read_csv_headers(path: Path, encoding: str) -> List[str]:
    raw = read_csv_headers_raw(path, encoding)
    return [sanitize_header(header) for header in raw]


def read_csv_header_map(
    path: Path, encoding: str
) -> Tuple[List[str], Dict[str, str], set[str], Dict[str, int]]:
    raw_headers = read_csv_headers_raw(path, encoding)
    mapped_headers = build_header_map(raw_headers)
    header_set = set(mapped_headers.keys())
    indexed_headers = build_header_index(raw_headers)
    return raw_headers, mapped_headers, header_set, indexed_headers


def detect_fc_kind(filename: str, headers: List[str]) -> Optional[str]:
    """Return the funds-control kind inferred from a filename and CSV headers."""
    name_lower = (filename or "").lower()
    filename = filename or ""

    if "任务信息" in filename:
        if "失败" in filename or "fail" in name_lower:
            return "fc_task_fail"
        return "fc_task_success"

    if "交易明细" in filename or "流水" in filename:
        return "fc_transaction"
    if "强制措施" in filename or "冻结" in filename or "止付" in filename:
        return "fc_coercive_measure"
    if "关联子账户" in filename or "子账户" in filename:
        return "fc_sub_account"
    if "账户信息" in filename:
        return "fc_account"
    if "人员住址" in filename or "住址" in filename:
        return "fc_person_address"
    if "人员联系方式" in filename or "联系方式" in filename:
        return "fc_person_contact"
    if "人员信息" in filename:
        return "fc_person"

    best_kind, best_score = None, 0
    got = set(headers)
    got_norm = {normalize_header_for_match(header) for header in headers if header}
    sub_account_hint = any(("子账户" in header or "子帐户" in header) for header in got_norm)
    for kind, schema in fc_import_schema.FC_SCHEMAS.items():
        score = 0
        for header in schema.headers:
            if header in got:
                score += 1
                continue
            if normalize_header_for_match(header) in got_norm:
                score += 1
                continue
            aliases = fc_import_schema.HEADER_ALIASES_BY_TABLE.get(kind, {}).get(header, [])
            if any(normalize_header_for_match(alias) in got_norm for alias in aliases):
                score += 1
        if kind == "fc_sub_account" and sub_account_hint:
            score += 3
        if score > best_score:
            best_kind, best_score = kind, score

    if best_score >= 3:
        return best_kind
    return None


def file_hash(path: Path, algo: str) -> str:
    accelerated = accelerated_file_hashes(path, [algo])
    normalized_algo = str(algo or "").strip().lower()
    if accelerated and normalized_algo in accelerated:
        return accelerated[normalized_algo].upper()
    digest = hashlib.new(algo)
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest().upper()


def file_hashes(path: Path, algos: Sequence[str]) -> Dict[str, str]:
    accelerated = accelerated_file_hashes(path, algos)
    if accelerated is not None:
        return {name: value.upper() for name, value in accelerated.items()}
    digesters = {str(algo): hashlib.new(str(algo)) for algo in algos if str(algo).strip()}
    if not digesters:
        return {}
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            for digest in digesters.values():
                digest.update(chunk)
    return {name: digest.hexdigest().upper() for name, digest in digesters.items()}
