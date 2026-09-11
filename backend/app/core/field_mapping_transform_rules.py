from __future__ import annotations

import re
from typing import Dict, List, Optional, Sequence

from app.core.fc_import_projection import normalize_header_for_match
from app.core.fc_import_schema import FC_SCHEMAS, HEADER_ALIASES_BY_TABLE
from app.core.import_accelerator import CsvColumnProfile, CsvColumnProfiles


def field_mapping_transform_messages(
    *,
    kind: str,
    headers: Sequence[str],
    mappings: Dict[str, str],
    profiles: Optional[CsvColumnProfiles],
) -> List[str]:
    if str(kind or "").strip() != "fc_transaction":
        return []

    unique_headers = _unique_headers(headers)
    if not unique_headers:
        return []

    source_by_target = _resolve_sources_by_target(kind, unique_headers, mappings)
    profile_by_header = {profile.header: profile for profile in (profiles.columns if profiles else [])}
    messages: List[str] = []

    amount_source = source_by_target.get("amount")
    dc_source = source_by_target.get("dc_flag")
    if amount_source and not dc_source:
        messages.append("收付标志=交易金额正负规则推断")
    if amount_source and _profile_has_negative_amount(profile_by_header.get(amount_source)):
        messages.append("交易金额=按绝对值规范化")

    date_source = _resolve_header(unique_headers, ("交易日期", "日期", "发生日期", "入账日期"))
    time_source = _resolve_header(unique_headers, ("时间", "发生时间", "交易时刻"))
    if date_source and time_source and date_source != time_source:
        messages.append("交易时间=日期+时间规则合成")

    txn_time_source = source_by_target.get("txn_time")
    if txn_time_source and _profile_has_compact_datetime(profile_by_header.get(txn_time_source)):
        messages.append("交易时间=连续数字日期时间规则解析")

    return list(dict.fromkeys(messages))


def _resolve_sources_by_target(kind: str, headers: Sequence[str], mappings: Dict[str, str]) -> Dict[str, str]:
    schema = FC_SCHEMAS[kind]
    target_header_by_key = {target_key: header for header, target_key in schema.col_map.items()}
    source_by_target: Dict[str, str] = {}
    for target_key, target_header in target_header_by_key.items():
        explicit = _resolve_header(headers, (str(mappings.get(target_key) or ""), str(mappings.get(target_header) or "")))
        if explicit:
            source_by_target[target_key] = explicit
            continue
        candidates = [target_header, *HEADER_ALIASES_BY_TABLE.get(kind, {}).get(target_header, [])]
        resolved = _resolve_header(headers, candidates)
        if resolved:
            source_by_target[target_key] = resolved
    return source_by_target


def _resolve_header(headers: Sequence[str], candidates: Sequence[str]) -> Optional[str]:
    normalized_headers = {
        normalize_header_for_match(header): header
        for header in headers
        if str(header or "").strip()
    }
    for candidate in candidates:
        normalized = normalize_header_for_match(str(candidate or ""))
        if normalized and normalized in normalized_headers:
            return normalized_headers[normalized]
    return None


def _profile_has_negative_amount(profile: Optional[CsvColumnProfile]) -> bool:
    if profile is None:
        return False
    if int(profile.negative_amount or 0) > 0:
        return True
    return any(_looks_negative_amount(sample) for sample in _profile_samples(profile))


def _profile_has_compact_datetime(profile: Optional[CsvColumnProfile]) -> bool:
    if profile is None:
        return False
    if int(profile.datetime_like or 0) <= 0:
        return False
    return any(re.fullmatch(r"\d{12}|\d{14}", sample) for sample in _profile_samples(profile))


def _profile_samples(profile: CsvColumnProfile) -> List[str]:
    samples = [
        *profile.first_non_empty,
        *profile.fixed_seed_samples,
        profile.feature_samples.longest,
        profile.feature_samples.min_amount,
        profile.feature_samples.max_amount,
        profile.feature_samples.date_like,
    ]
    return [str(item).strip() for item in samples if str(item or "").strip()]


def _looks_negative_amount(value: str) -> bool:
    text = str(value or "").strip()
    return bool(re.search(r"(^|[^\d])-+\s*\d", text) or re.fullmatch(r"\(.*\d.*\)", text))


def _unique_headers(headers: Sequence[str]) -> List[str]:
    seen: set[str] = set()
    result: List[str] = []
    for header in headers:
        value = str(header or "").strip()
        if value and value not in seen:
            seen.add(value)
            result.append(value)
    return result
