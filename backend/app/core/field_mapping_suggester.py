from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import Callable, Dict, List, Optional, Sequence

from app.core.fc_import_schema import FC_SCHEMAS, HEADER_ALIASES_BY_TABLE
from app.core.import_accelerator import CsvColumnProfile, CsvColumnProfiles
from app.core.local_field_mapping_model import (
    LocalFieldMappingModel,
    LocalFieldMappingRequest,
    LocalFieldMappingResponse,
)


@dataclass(frozen=True)
class FieldMappingCandidate:
    target_key: str
    target_header: str
    source_header: str
    source_index: int
    confidence: float
    method: str
    evidence: str


@dataclass(frozen=True)
class FieldMappingSuggestion:
    kind: str
    status: str
    method: str
    confidence: float
    mappings: Dict[str, str] = field(default_factory=dict)
    required_missing: List[str] = field(default_factory=list)
    candidates: List[FieldMappingCandidate] = field(default_factory=list)
    ai_attempted: bool = False
    ai_available: bool = False
    message: str = ""


LocalMappingClassifier = Callable[[LocalFieldMappingRequest], LocalFieldMappingResponse]

_PARENS_RE = re.compile(r"[（(].*?[）)]")
_TOKEN_NOISE_RE = re.compile(r"[\s_（）()【】\[\]/:：,，.\-*＊]+")


def suggest_field_mapping(
    *,
    file_name: str,
    kind: str,
    headers: Sequence[str],
    profiles: Optional[CsvColumnProfiles] = None,
    classifier: Optional[LocalMappingClassifier] = None,
) -> FieldMappingSuggestion:
    normalized_kind = str(kind or "").strip()
    if normalized_kind not in FC_SCHEMAS:
        return FieldMappingSuggestion(
            kind=normalized_kind,
            status="review",
            method="unsupported_kind",
            confidence=0.0,
            message="当前类型尚未配置字段映射规则。",
        )

    schema = FC_SCHEMAS[normalized_kind]
    unique_headers = _unique_headers(headers)
    profile_by_header = {profile.header: profile for profile in (profiles.columns if profiles else [])}
    candidates = _rule_candidates(normalized_kind, unique_headers, profile_by_header)
    applied_candidates = _with_derived_candidates(
        normalized_kind,
        _apply_candidates(candidates),
        unique_headers,
        profile_by_header,
    )
    mappings = {candidate.target_key: candidate.source_header for candidate in applied_candidates}
    missing = _required_missing(normalized_kind, mappings)

    if _looks_like_no_business_rows(normalized_kind, unique_headers, profile_by_header):
        method = _suggestion_method(applied_candidates, ai_attempted=False)
        if method in {"none", "ai_unavailable"}:
            method = "exact_header"
        return FieldMappingSuggestion(
            kind=normalized_kind,
            status="ready",
            method=method,
            confidence=min((candidate.confidence for candidate in applied_candidates), default=1.0),
            mappings=mappings,
            required_missing=[],
            candidates=applied_candidates,
            ai_attempted=False,
            ai_available=False,
            message="未发现可导入业务记录，导入时将自动跳过空记录/无记录反馈行。",
        )

    ai_attempted = False
    ai_available = False
    ai_message = ""
    if missing and unique_headers:
        ai_attempted = True
        classifier_response = _call_classifier(
            classifier=classifier,
            file_name=file_name,
            kind=normalized_kind,
            headers=unique_headers,
            profile_by_header=profile_by_header,
            target_headers=missing,
            excluded_source_headers=set(mappings.values()),
        )
        ai_available = classifier_response.available
        ai_message = classifier_response.message
        ai_candidates = _ai_candidates(
            normalized_kind,
            classifier_response.mappings,
            unique_headers,
            profile_by_header,
        )
        if ai_candidates:
            applied_candidates = _with_derived_candidates(
                normalized_kind,
                _apply_candidates([*applied_candidates, *ai_candidates]),
                unique_headers,
                profile_by_header,
            )
            mappings = {candidate.target_key: candidate.source_header for candidate in applied_candidates}
            missing = _required_missing(normalized_kind, mappings)

    if _looks_like_no_business_rows(normalized_kind, unique_headers, profile_by_header):
        method = _suggestion_method(applied_candidates, ai_attempted=ai_attempted)
        if method in {"none", "ai_unavailable"}:
            method = "exact_header"
        return FieldMappingSuggestion(
            kind=normalized_kind,
            status="ready",
            method=method,
            confidence=min((candidate.confidence for candidate in applied_candidates), default=1.0),
            mappings=mappings,
            required_missing=[],
            candidates=applied_candidates,
            ai_attempted=ai_attempted,
            ai_available=ai_available,
            message="未发现可导入业务记录，导入时将自动跳过空记录/无记录反馈行。",
        )

    status = "ready" if not missing else "review"
    confidence = min((candidate.confidence for candidate in applied_candidates), default=0.0)
    method = _suggestion_method(applied_candidates, ai_attempted=ai_attempted)
    message = _suggestion_message(
        status=status,
        kind=normalized_kind,
        mapping_count=len(mappings),
        missing=missing,
        ai_attempted=ai_attempted,
        ai_available=ai_available,
        ai_message=ai_message,
    )
    return FieldMappingSuggestion(
        kind=normalized_kind,
        status=status,
        method=method,
        confidence=confidence,
        mappings=mappings,
        required_missing=missing,
        candidates=applied_candidates,
        ai_attempted=ai_attempted,
        ai_available=ai_available,
        message=message,
    )


def required_missing_for_mapping(kind: str, mappings: Dict[str, str]) -> List[str]:
    normalized_kind = str(kind or "").strip()
    if normalized_kind not in FC_SCHEMAS:
        return []
    return _required_missing(normalized_kind, mappings)


def validate_field_mapping_candidate(
    *,
    kind: str,
    target_key: str,
    source_header: str,
    headers: Sequence[str],
    profiles: Optional[CsvColumnProfiles],
) -> bool:
    normalized_kind = str(kind or "").strip()
    schema = FC_SCHEMAS.get(normalized_kind)
    if schema is None:
        return False
    header_by_key = {value: key for key, value in schema.col_map.items()}
    target_key = str(target_key or "").strip()
    source_header = str(source_header or "").strip()
    if target_key not in header_by_key or not source_header:
        return False
    valid_source = _resolve_header(source_header, headers)
    if not valid_source:
        return False
    if profiles is None:
        return False
    profile_by_header = {profile.header: profile for profile in profiles.columns}
    profile = profile_by_header.get(valid_source)
    if profile is None:
        return False
    return _profile_supports_target(
        target_key,
        header_by_key[target_key],
        valid_source,
        profile,
        trusted_header=False,
    )


def _rule_candidates(
    kind: str,
    headers: Sequence[str],
    profile_by_header: Dict[str, CsvColumnProfile],
) -> List[FieldMappingCandidate]:
    schema = FC_SCHEMAS[kind]
    alias_map = HEADER_ALIASES_BY_TABLE.get(kind, {})
    header_index = {header: index for index, header in enumerate(headers)}
    candidates: List[FieldMappingCandidate] = []
    for target_header in schema.headers:
        target_key = schema.col_map[target_header]
        aliases = [target_header, *alias_map.get(target_header, [])]
        normalized_aliases = {_normalize_header(alias) for alias in aliases if str(alias or "").strip()}
        for source_header in headers:
            normalized_source = _normalize_header(source_header)
            confidence = 0.0
            method = ""
            evidence = ""
            if source_header == target_header:
                confidence = 1.0
                method = "exact_header"
                evidence = "表头与标准字段完全一致"
            elif normalized_source in normalized_aliases:
                confidence = 0.93
                method = "alias_header"
                evidence = "表头命中字段别名"
            elif _enterprise_query_id_header(
                kind=kind,
                target_key=target_key,
                source_header=source_header,
                headers=headers,
            ):
                confidence = 0.74
                method = "rule_header"
                evidence = "企业查询条件证照列已通过样本校验"
            elif _strong_header_similarity(normalized_source, normalized_aliases):
                confidence = 0.78
                method = "rule_header"
                evidence = "表头与目标字段强相关"
            if confidence <= 0:
                continue
            profile = profile_by_header.get(source_header)
            if profile and profile.non_empty <= 0:
                continue
            if method == "rule_header" and profile is None:
                continue
            if profile and not _profile_supports_target(
                target_key,
                target_header,
                source_header,
                profile,
                trusted_header=method == "exact_header",
            ):
                continue
            candidates.append(
                FieldMappingCandidate(
                    target_key=target_key,
                    target_header=target_header,
                    source_header=source_header,
                    source_index=header_index.get(source_header, -1),
                    confidence=confidence,
                    method=method,
                    evidence=evidence,
                )
            )
    return candidates


def _ai_candidates(
    kind: str,
    mappings: Dict[str, str],
    headers: Sequence[str],
    profile_by_header: Dict[str, CsvColumnProfile],
) -> List[FieldMappingCandidate]:
    if not mappings:
        return []
    schema = FC_SCHEMAS[kind]
    header_by_key = {value: key for key, value in schema.col_map.items()}
    valid_headers = set(headers)
    header_index = {header: index for index, header in enumerate(headers)}
    candidates: List[FieldMappingCandidate] = []
    for target_key, source_header in mappings.items():
        target_key = str(target_key or "").strip()
        source_header = str(source_header or "").strip()
        if target_key not in header_by_key or source_header not in valid_headers:
            continue
        profile = profile_by_header.get(source_header)
        if profile is None:
            continue
        target_header = header_by_key[target_key]
        if profile and not _profile_supports_target(
            target_key,
            target_header,
            source_header,
            profile,
            trusted_header=False,
        ):
            continue
        candidates.append(
            FieldMappingCandidate(
                target_key=target_key,
                target_header=target_header,
                source_header=source_header,
                source_index=header_index.get(source_header, -1),
                confidence=0.74,
                method="ai_validated",
                evidence="本地模型建议已通过样本值校验",
            )
        )
    return candidates


def _apply_candidates(candidates: Sequence[FieldMappingCandidate]) -> List[FieldMappingCandidate]:
    ordered = sorted(
        candidates,
        key=lambda item: (item.confidence, _method_rank(item.method), -max(item.source_index, 0)),
        reverse=True,
    )
    used_targets: set[str] = set()
    used_sources: set[str] = set()
    applied: List[FieldMappingCandidate] = []
    for candidate in ordered:
        if candidate.target_key in used_targets or candidate.source_header in used_sources:
            continue
        used_targets.add(candidate.target_key)
        used_sources.add(candidate.source_header)
        applied.append(candidate)
    return applied


def _with_derived_candidates(
    kind: str,
    candidates: Sequence[FieldMappingCandidate],
    headers: Sequence[str],
    profile_by_header: Dict[str, CsvColumnProfile],
) -> List[FieldMappingCandidate]:
    if kind != "fc_sub_account":
        return list(candidates)
    mappings = {candidate.target_key: candidate.source_header for candidate in candidates}
    header_by_key = {value: key for key, value in FC_SCHEMAS[kind].col_map.items()}
    header_index = {header: index for index, header in enumerate(headers)}
    out = list(candidates)

    def add(target_key: str, source_key: str, evidence: str) -> None:
        if target_key in mappings or source_key not in mappings:
            return
        source_header = mappings[source_key]
        profile = profile_by_header.get(source_header)
        if profile is not None and profile.non_empty <= 0:
            return
        out.append(
            FieldMappingCandidate(
                target_key=target_key,
                target_header=header_by_key[target_key],
                source_header=source_header,
                source_index=header_index.get(source_header, -1),
                confidence=0.72,
                method="rule_header",
                evidence=evidence,
            )
        )
        mappings[target_key] = source_header

    add("sub_acct", "parent_acct", "子账户账号为空时复用开户账号作为子账户标识")
    add("parent_acct", "sub_acct", "开户账号为空时复用子账户账号作为开户账号标识")
    return out


def _looks_like_no_business_rows(
    kind: str,
    headers: Sequence[str],
    profile_by_header: Dict[str, CsvColumnProfile],
) -> bool:
    if not profile_by_header:
        return False
    non_empty_headers = {
        header
        for header in headers
        if (profile_by_header.get(header) is not None and int(profile_by_header[header].non_empty or 0) > 0)
    }
    if not non_empty_headers:
        return True
    ignored_by_kind = {
        "fc_transaction": {"查询反馈结果原因"},
        "fc_sub_account": {"银行名称"},
        "fc_coercive_measure": {"银行名称"},
    }
    ignored = {_normalize_header(header) for header in ignored_by_kind.get(kind, set())}
    if ignored and all(_normalize_header(header) in ignored for header in non_empty_headers):
        return True
    if kind == "fc_person":
        id_profiles = _profiles_for_headers(
            headers,
            profile_by_header,
            {"证照号码", "证件号码", "身份证号", "身份证号码", "开户人证件号码"},
        )
        return bool(id_profiles) and not any(_profile_has_business_value(profile) for profile in id_profiles)
    if kind == "fc_account":
        key_profiles = _profiles_for_headers(
            headers,
            profile_by_header,
            {"交易卡号", "交易账号", "卡号", "账号", "账户账号", "银行卡号"},
        )
        if not any(_profile_has_business_value(profile) for profile in key_profiles):
            return True
        meaningful_profiles = _profiles_for_headers(
            headers,
            profile_by_header,
            {
                "账户开户名称",
                "开户人证件号码",
                "账号开户时间",
                "可用余额",
                "币种",
                "开户网点代码",
                "开户网点",
                "账户状态",
                "钞汇标志名称",
                "销户日期",
                "账户类型",
                "备注",
                "销户网点",
            },
        )
        if any(_profile_has_business_value(profile) for profile in meaningful_profiles):
            return False
        balance_profiles = _profiles_for_headers(headers, profile_by_header, {"账户余额", "余额"})
        return not any(_profile_has_non_zero_amount(profile) for profile in balance_profiles)
    return False


def _profiles_for_headers(
    headers: Sequence[str],
    profile_by_header: Dict[str, CsvColumnProfile],
    candidates: set[str],
) -> List[CsvColumnProfile]:
    normalized_candidates = {_normalize_header(candidate) for candidate in candidates}
    out: List[CsvColumnProfile] = []
    for header in headers:
        profile = profile_by_header.get(header)
        if profile is None:
            continue
        if _normalize_header(header) in normalized_candidates:
            out.append(profile)
    return out


def _profile_has_business_value(profile: CsvColumnProfile) -> bool:
    if profile.non_empty <= 0:
        return False
    samples = [*profile.first_non_empty, *profile.fixed_seed_samples]
    if not samples:
        return profile.non_empty > 0
    return any(not _is_placeholder_value(sample) for sample in samples)


def _profile_has_non_zero_amount(profile: CsvColumnProfile) -> bool:
    if profile.non_empty <= 0:
        return False
    samples = [*profile.first_non_empty, *profile.fixed_seed_samples]
    if not samples:
        return False
    for sample in samples:
        value = str(sample or "").replace(",", "").replace("，", "").strip()
        if not value or _is_placeholder_value(value):
            continue
        try:
            if float(value) != 0:
                return True
        except ValueError:
            continue
    return False


def _is_placeholder_value(value: str) -> bool:
    normalized = str(value or "").strip().upper()
    if not normalized:
        return True
    return bool(
        re.fullmatch(
            r"(0+|NULL|N/A|NA|NONE|-+|—+|无|暂无|未知|不详|未找到相关数据|未查询到.*|未找到.*|没有.*(记录|数据)|无.*(记录|数据))",
            normalized,
        )
    )


def _enterprise_query_id_header(
    *,
    kind: str,
    target_key: str,
    source_header: str,
    headers: Sequence[str],
) -> bool:
    if kind != "fc_transaction" or target_key != "opener_id_no":
        return False
    normalized_source = _normalize_header(source_header)
    if normalized_source != _normalize_header("未知(查询条件)"):
        return False
    normalized_headers = {_normalize_header(header) for header in headers}
    return _normalize_header("企业名称(查询条件)") in normalized_headers


def _call_classifier(
    *,
    classifier: Optional[LocalMappingClassifier],
    file_name: str,
    kind: str,
    headers: Sequence[str],
    profile_by_header: Dict[str, CsvColumnProfile],
    target_headers: Sequence[str],
    excluded_source_headers: set[str],
) -> LocalFieldMappingResponse:
    schema = FC_SCHEMAS[kind]
    requested_targets = [
        (schema.col_map[header], header)
        for header in target_headers
        if header in schema.col_map
    ]
    candidate_headers = _ai_candidate_headers(
        headers=headers,
        requested_targets=requested_targets,
        profile_by_header=profile_by_header,
        excluded_source_headers=excluded_source_headers,
    )
    request = LocalFieldMappingRequest(
        file_name=file_name,
        kind=kind,
        headers=candidate_headers,
        target_keys=[key for key, _ in requested_targets],
        target_labels={key: header for key, header in requested_targets},
        samples_by_header={
            header: _profile_samples(profile_by_header.get(header))
            for header in candidate_headers
        },
    )
    if classifier is not None:
        return classifier(request)
    return LocalFieldMappingModel().suggest(request)


def _ai_candidate_headers(
    *,
    headers: Sequence[str],
    requested_targets: Sequence[tuple[str, str]],
    profile_by_header: Dict[str, CsvColumnProfile],
    excluded_source_headers: set[str],
) -> List[str]:
    remaining_headers = [header for header in headers if header not in excluded_source_headers]
    if not profile_by_header:
        return list(remaining_headers)
    candidate_headers: List[str] = []
    for source_header in remaining_headers:
        profile = profile_by_header.get(source_header)
        if profile is None:
            continue
        if any(
            _profile_supports_target(
                target_key,
                target_header,
                source_header,
                profile,
                trusted_header=False,
            )
            for target_key, target_header in requested_targets
        ):
            candidate_headers.append(source_header)
    return candidate_headers


def _profile_samples(profile: Optional[CsvColumnProfile]) -> List[str]:
    if profile is None:
        return []
    samples = [
        *profile.first_non_empty,
        *profile.fixed_seed_samples,
        profile.feature_samples.longest,
        profile.feature_samples.min_amount,
        profile.feature_samples.max_amount,
        profile.feature_samples.date_like,
    ]
    return [str(item) for item in samples if str(item or "").strip()][:8]


def _profile_supports_target(
    target_key: str,
    target_header: str,
    source_header: str,
    profile: CsvColumnProfile,
    *,
    trusted_header: bool,
) -> bool:
    if profile.non_empty <= 0:
        return False
    if trusted_header:
        return True
    date_targets = {"txn_time", "open_time", "last_txn_time", "close_date", "start_date", "end_date"}
    amount_targets = {"amount", "balance", "available_balance", "counterparty_balance"}
    account_targets = {"acct_no", "card_no", "counterparty_acct", "parent_acct", "sub_acct"}
    id_targets = {"opener_id_no", "id_no", "agent_id_no", "counterparty_id_no"}
    phone_targets = {"org_phone", "home_phone", "contact_phone"}
    text_targets = {"summary", "remark"}
    enum_targets = {"currency", "cash_flag", "is_success", "txn_type"}
    if target_key in date_targets or "日期" in target_header or "时间" in target_header:
        return profile.date_like_ratio >= 0.45 or profile.datetime_like > 0
    if target_key in amount_targets or "金额" in target_header or "余额" in target_header:
        return profile.amount_like_ratio >= 0.45 and _amount_header_supports_target(target_key, target_header, source_header)
    if target_key in account_targets or "账号" in target_header or "卡号" in target_header:
        if trusted_header:
            return True
        normalized_source = _normalize_header(source_header)
        source_mentions_account = _contains_any(normalized_source, ("账号", "账户", "卡号", "账卡号", "account", "card"))
        if profile.date_like_ratio >= 0.45:
            return False
        if profile.amount_like_ratio >= 0.45 and not source_mentions_account:
            return False
        if _profile_ratio(profile.phone_like, profile) >= 0.35:
            return False
        if _profile_ratio(profile.id_no_like, profile) >= 0.35 and not source_mentions_account:
            return False
        account_ratio = _profile_ratio(profile.account_like, profile)
        return account_ratio >= 0.35 or source_mentions_account
    if target_key in id_targets or "证件" in target_header or "身份证" in target_header:
        if trusted_header:
            return True
        return _profile_ratio(profile.id_no_like, profile) >= 0.35
    if target_key in phone_targets or "电话" in target_header or "联系" in target_header:
        if trusted_header:
            return True
        return _profile_ratio(profile.phone_like, profile) >= 0.35
    if target_key == "ip_addr":
        return profile.ip_like > 0
    if target_key == "mac_addr":
        return profile.mac_like > 0
    if target_key in text_targets:
        return _text_header_supports_target(target_key, source_header)
    if target_key in enum_targets:
        return _enum_header_supports_target(target_key, source_header, profile)
    return True


def _required_missing(kind: str, mappings: Dict[str, str]) -> List[str]:
    schema = FC_SCHEMAS[kind]
    if kind == "fc_transaction":
        required = ["acct_no", "txn_time", "amount"]
    elif kind == "fc_account":
        required = [] if ("acct_no" in mappings or "card_no" in mappings) else ["acct_no"]
    elif kind == "fc_person":
        required = ["id_no"]
    elif kind == "fc_sub_account":
        required = ["bank_name", "parent_acct", "sub_acct"]
    elif kind == "fc_person_address":
        required = ["open_name", "id_no", "home_addr"]
    elif kind == "fc_person_contact":
        required = ["open_name", "contact_phone"]
    elif kind == "fc_coercive_measure":
        required = ["bank_name", "acct_no", "measure_type"]
    else:
        required = []
    labels_by_key = {value: header for header, value in schema.col_map.items()}
    return [labels_by_key.get(key, key) for key in required if key not in mappings]


def _profile_ratio(count: int, profile: CsvColumnProfile) -> float:
    return max(0, int(count or 0)) / max(profile.non_empty, 1)


def _amount_header_supports_target(target_key: str, target_header: str, source_header: str) -> bool:
    normalized_target = _normalize_header(target_header)
    normalized_source = _normalize_header(source_header)
    source_is_balance = _contains_any(normalized_source, ("余额", "balance"))
    source_is_counterparty = _contains_any(normalized_source, ("对手", "对方", "交易对手", "counterparty"))
    source_is_available = _contains_any(normalized_source, ("可用", "available"))
    source_is_flow_amount = _contains_any(
        normalized_source,
        ("交易金额", "发生额", "收入", "支出", "收支", "借方", "贷方", "amount"),
    )

    if target_key == "amount":
        return not source_is_balance or source_is_flow_amount
    if target_key == "counterparty_balance":
        return source_is_balance and source_is_counterparty
    if target_key == "available_balance":
        return source_is_balance and source_is_available and not source_is_counterparty
    if target_key == "balance" or "余额" in normalized_target:
        return source_is_balance and not source_is_counterparty
    return True


def _contains_any(value: str, needles: Sequence[str]) -> bool:
    return any(needle in value for needle in needles)


def _text_header_supports_target(target_key: str, source_header: str) -> bool:
    normalized_source = _normalize_header(source_header)
    if target_key == "summary":
        return _contains_any(normalized_source, ("摘要", "用途", "交易说明"))
    if target_key == "remark":
        return _contains_any(normalized_source, ("备注", "附言"))
    return True


def _enum_header_supports_target(target_key: str, source_header: str, profile: CsvColumnProfile) -> bool:
    normalized_source = _normalize_header(source_header)
    values = [_normalize_enum_value(value) for value in _profile_samples(profile)]
    values = [value for value in values if value]

    if target_key == "currency":
        return _contains_any(normalized_source, ("币种", "币别", "货币", "currency")) or _values_match_any(
            values,
            {
                "cny",
                "rmb",
                "usd",
                "hkd",
                "eur",
                "jpy",
                "gbp",
                "aud",
                "cad",
                "sgd",
                "人民币",
                "美元",
                "港币",
                "欧元",
                "日元",
                "英镑",
                "澳元",
                "加元",
                "新加坡元",
            },
        )
    if target_key == "cash_flag":
        return _contains_any(normalized_source, ("现金标志", "现金标识", "现金类型", "现钞", "现汇")) or _values_match_any(
            values,
            {"现金", "非现金", "现钞", "现汇", "钞", "汇"},
        )
    if target_key == "is_success":
        if _contains_any(normalized_source, ("原因", "反馈原因")):
            return False
        if _contains_any(normalized_source, ("交易是否成功", "是否成功", "交易结果", "成功标志")):
            return True
        return bool(values) and _all_values_match(
            values,
            {"成功", "失败", "是", "否", "true", "false", "1", "0", "yes", "no"},
        )
    if target_key == "txn_type":
        if _contains_any(
            normalized_source,
            ("账户类型", "账号类型", "卡类型", "证件类型", "凭证类型", "措施类型", "客户类型", "账户类别", "子账户类别"),
        ):
            return False
        return _contains_any(normalized_source, ("交易类型", "交易种类", "业务类型", "业务种类", "交易类别", "业务类别"))
    return True


def _normalize_enum_value(value: str) -> str:
    return _TOKEN_NOISE_RE.sub("", str(value or "").strip().lower())


def _values_match_any(values: Sequence[str], allowed: set[str]) -> bool:
    return any(value in allowed for value in values)


def _all_values_match(values: Sequence[str], allowed: set[str]) -> bool:
    return all(value in allowed for value in values)


def _suggestion_method(candidates: Sequence[FieldMappingCandidate], *, ai_attempted: bool) -> str:
    methods = {candidate.method for candidate in candidates}
    if "ai_validated" in methods:
        return "ai_validated"
    if methods == {"exact_header"}:
        return "exact_header"
    if methods:
        return "rule"
    return "ai_unavailable" if ai_attempted else "none"


def _suggestion_message(
    *,
    status: str,
    kind: str,
    mapping_count: int,
    missing: Sequence[str],
    ai_attempted: bool,
    ai_available: bool,
    ai_message: str,
) -> str:
    if status == "ready":
        suffix = "，AI 校验参与后已通过。" if ai_attempted and ai_available else "。"
        return f"已自动确认 {mapping_count} 项字段映射{suffix}"
    if ai_attempted and not ai_available:
        return f"规则无法确认关键字段：{', '.join(missing)}；本地 AI 不可用，需人工确认。"
    if ai_attempted and ai_message:
        return f"规则和本地 AI 均未能确认关键字段：{', '.join(missing)}；{ai_message}"
    return f"仍缺少关键字段映射：{', '.join(missing)}，需人工确认。"


def _unique_headers(headers: Sequence[str]) -> List[str]:
    seen: set[str] = set()
    result: List[str] = []
    for header in headers:
        value = str(header or "").strip()
        if value and value not in seen:
            seen.add(value)
            result.append(value)
    return result


def _resolve_header(source_header: str, headers: Sequence[str]) -> str:
    source_header = str(source_header or "").strip()
    if not source_header:
        return ""
    for header in headers:
        value = str(header or "").strip()
        if value == source_header:
            return value
    normalized_source = _normalize_header(source_header)
    for header in headers:
        value = str(header or "").strip()
        if value and _normalize_header(value) == normalized_source:
            return value
    return ""


def _normalize_header(value: str) -> str:
    text = str(value or "").replace("\ufeff", "").lower()
    text = _PARENS_RE.sub("", text)
    return _TOKEN_NOISE_RE.sub("", text)


def _strong_header_similarity(source: str, aliases: set[str]) -> bool:
    if len(source) < 2:
        return False
    for alias in aliases:
        if len(alias) < 2:
            continue
        if source == alias:
            return True
        if len(alias) >= 3 and (source in alias or alias in source):
            return True
    return False


def _method_rank(method: str) -> int:
    return {
        "exact_header": 4,
        "alias_header": 3,
        "rule_header": 2,
        "ai_validated": 1,
    }.get(method, 0)
