from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Dict, List, Optional, Protocol, Sequence, Set, Tuple

from app.core.fc_import_norm_exprs import (
    row_hash_expr,
    sanitize_precleaned_text_expr,
    sanitize_precleaned_text_keep_empty_expr,
    sanitize_text_expr,
    sanitize_text_keep_empty_expr,
)
from app.core.fc_import_semantic_filter import build_semantic_row_filter_expr


class FcProjectionSchema(Protocol):
    table: str
    headers: List[str]
    col_map: Dict[str, str]
    store_raw_json: bool


@dataclass(frozen=True)
class ImportProjection:
    select_cols: List[str]
    extra_json_expr: str
    raw_json_expr: str
    hash_expr: str
    non_empty_filter_expr: str
    force_not_null_sql: str
    mapping_used_headers: Set[str]
    mapping_alias_used: List[str]
    mapping_missing: List[str]
    mapping_derived: List[str]


EXTRA_JSON_TABLES = {"fc_transaction", "fc_account", "fc_sub_account"}
ROW_FILTER_IGNORED_HEADERS_BY_TABLE: Dict[str, Set[str]] = {
    "fc_transaction": {"查询反馈结果原因"},
    "fc_sub_account": {"银行名称"},
    "fc_coercive_measure": {"银行名称"},
}
COALESCE_SOURCE_HEADERS_BY_TABLE: Dict[str, Dict[str, Tuple[str, ...]]] = {
    "fc_transaction": {
        "交易账号": ("交易账号", "查询账号", "查询帐号", "本方账号", "账号", "账户账号", "账户", "账号/卡号"),
        "交易卡号": ("交易卡号", "查询卡号", "本方卡号", "卡号", "账卡号"),
        "交易对手账卡号": (
            "交易对手账卡号",
            "交易对方账卡号",
            "交易对方帐卡号",
            "交易对方账号",
            "交易对方卡号",
            "对方账号",
            "对方卡号",
            "对手账号",
            "对手卡号",
        ),
    }
}

_PARENS_RE = re.compile(r"[（(].*?[）)]")
_SPACE_RE = re.compile(r"\s+")


def sanitize_header(name: str) -> str:
    return (name or "").replace("\ufeff", "").replace("\t", "").strip()


def normalize_header_for_match(name: str) -> str:
    text = sanitize_header(name)
    text = text.strip("*＊")
    text = _PARENS_RE.sub("", text)
    text = text.replace("\u3000", " ")
    return _SPACE_RE.sub("", text)


def header_map(raw_headers: Sequence[str]) -> Dict[str, str]:
    mapped: Dict[str, str] = {}
    for raw in raw_headers:
        raw_text = (raw or "").replace("\ufeff", "")
        sanitized = sanitize_header(raw_text)
        if sanitized and sanitized not in mapped:
            mapped[sanitized] = raw_text
    return mapped


def header_index(raw_headers: Sequence[str]) -> Dict[str, int]:
    indexed: Dict[str, int] = {}
    for index, raw in enumerate(raw_headers):
        raw_text = (raw or "").replace("\ufeff", "")
        sanitized = sanitize_header(raw_text)
        if sanitized and sanitized not in indexed:
            indexed[sanitized] = index
    return indexed


def quote_ident(name: str) -> str:
    return '"' + (name or "").replace('"', '""') + '"'


def sql_literal(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def raw_col(name: str) -> str:
    return f"{name}_raw"


def is_generated_unnamed_header(name: str) -> bool:
    text = sanitize_header(name)
    if not text.startswith("Unnamed:"):
        return False
    suffix = text.split(":", 1)[1].strip()
    return suffix.isdigit()


def _dedupe_headers(headers: Sequence[str]) -> List[str]:
    out: List[str] = []
    seen: Set[str] = set()
    for header in headers:
        value = str(header or "").strip()
        if not value or value in seen:
            continue
        out.append(value)
        seen.add(value)
    return out


def build_import_projection(
    *,
    schema: FcProjectionSchema,
    raw_headers: Sequence[str],
    header_map_value: Dict[str, str],
    header_set: Set[str],
    header_index_value: Dict[str, int],
    use_positional: bool,
    col_names: Sequence[str],
    field_mapping: Optional[Dict[str, str]],
    alias_map: Dict[str, List[str]],
    precleaned_source: bool,
    supports_json_object: bool,
    derived_header_values: Optional[Dict[str, str]] = None,
) -> ImportProjection:
    derived_values = {
        str(header): str(expr)
        for header, expr in (derived_header_values or {}).items()
        if str(expr or "").strip()
    }
    norm_header_to_sanitized: Dict[str, str] = {}
    for header in header_set:
        key = normalize_header_for_match(header)
        if key and key not in norm_header_to_sanitized:
            norm_header_to_sanitized[key] = header

    force_not_null_sql = _force_not_null_sql(
        header_map_value=header_map_value,
        header_index_value=header_index_value,
        use_positional=use_positional,
        col_names=col_names,
    )
    mapping_overrides = {
        str(key or "").strip(): str(value or "").strip()
        for key, value in (field_mapping or {}).items()
        if str(key or "").strip() and str(value or "").strip()
    }
    mapping_used_headers: Set[str] = set()
    mapping_missing: List[str] = []
    mapping_derived: List[str] = []
    mapping_alias_used: List[str] = []

    def resolve_column_ref(header: str) -> Optional[str]:
        if use_positional:
            index = header_index_value.get(header)
            if index is None:
                index = header_index_value.get(norm_header_to_sanitized.get(normalize_header_for_match(header), ""))
            if index is None or index >= len(col_names):
                return None
            return col_names[index]

        if header in header_set:
            return quote_ident(header_map_value.get(header, header))

        normalized = normalize_header_for_match(header)
        mapped = norm_header_to_sanitized.get(normalized)
        if mapped and mapped in header_set:
            return quote_ident(header_map_value.get(mapped, mapped))
        return None

    def candidate_headers(std_header: str, *, include_all: bool) -> List[str]:
        configured = COALESCE_SOURCE_HEADERS_BY_TABLE.get(schema.table, {}).get(std_header)
        if include_all and configured:
            return _dedupe_headers((*configured, *alias_map.get(std_header, [])))
        return _dedupe_headers((std_header, *alias_map.get(std_header, [])))

    def find_refs_with_aliases(std_header: str, *, include_all: bool = False) -> List[Tuple[str, str]]:
        out: List[Tuple[str, str]] = []
        seen_refs: Set[str] = set()

        def add_candidate(source_header: str) -> None:
            ref = resolve_column_ref(source_header)
            if not ref or ref in seen_refs:
                return
            seen_refs.add(ref)
            out.append((ref, source_header))
            mapping_used_headers.add(source_header)
            if source_header != std_header:
                mapping_alias_used.append(f"{std_header}←{source_header}")

        manual_header = mapping_overrides.get(std_header)
        if manual_header:
            add_candidate(manual_header)
            if out and not include_all:
                return out
        for candidate in candidate_headers(std_header, include_all=include_all):
            if candidate == manual_header:
                continue
            add_candidate(candidate)
            if out and not include_all:
                return out
        return out

    def find_ref_with_aliases(std_header: str) -> Optional[Tuple[str, str]]:
        refs = find_refs_with_aliases(std_header)
        return refs[0] if refs else None

    def value_expr(expr: str) -> str:
        return sanitize_precleaned_text_expr(expr) if precleaned_source else sanitize_text_expr(expr)

    def value_keep_empty_expr(expr: str) -> str:
        return (
            sanitize_precleaned_text_keep_empty_expr(expr)
            if precleaned_source
            else sanitize_text_keep_empty_expr(expr)
        )

    def coalesced_value_expr(refs: Sequence[Tuple[str, str]], *, keep_empty_tail: bool = False) -> str:
        if not refs:
            return "NULL"
        exprs = [value_expr(ref) for ref, _ in refs]
        if keep_empty_tail:
            exprs[-1] = value_keep_empty_expr(refs[-1][0])
        if len(exprs) == 1:
            return exprs[0]
        return "COALESCE(" + ", ".join(exprs) + ")"

    def time_expr() -> str:
        found = find_ref_with_aliases("交易时间")
        ref_time = found[0] if found else None
        ref_date = None
        for header in ("交易日期", "日期", "发生日期", "入账日期"):
            resolved = resolve_column_ref(header)
            if resolved:
                ref_date = resolved
                mapping_used_headers.add(header)
                break
        ref_time2 = None
        for header in ("交易时间", "时间", "发生时间", "交易时刻"):
            resolved = resolve_column_ref(header)
            if resolved:
                ref_time2 = resolved
                mapping_used_headers.add(header)
                break

        if ref_date and ref_time2:
            date_expr = value_expr(ref_date)
            time_value_expr = value_expr(ref_time2)
            normalized_date = norm_date_expr(date_expr)
            normalized_time = norm_time_expr(time_value_expr)
            normalized_datetime = norm_datetime_expr(time_value_expr)
            has_date = (
                f"(regexp_matches({time_value_expr}, '^[0-9]{{4}}[-/][0-9]{{1,2}}[-/][0-9]{{1,2}}') "
                f"OR regexp_matches({time_value_expr}, '^[0-9]{{8}}$') "
                f"OR regexp_matches({time_value_expr}, '^[0-9]{{12}}$') "
                f"OR regexp_matches({time_value_expr}, '^[0-9]{{14}}$'))"
            )
            mapping_derived.append("交易时间=日期+时间")
            return (
                "CASE "
                f"WHEN {has_date} THEN {normalized_datetime} "
                f"WHEN {normalized_date} IS NOT NULL AND {normalized_time} IS NOT NULL "
                f"THEN {normalized_date} || ' ' || {normalized_time} "
                f"ELSE COALESCE({normalized_datetime}, {normalized_date}) END"
            )
        if ref_time:
            return norm_datetime_expr(value_expr(ref_time))
        if ref_date:
            return norm_date_expr(value_expr(ref_date))
        return "NULL"

    select_cols: List[str] = []
    for header in schema.headers:
        column = raw_col(schema.col_map[header])
        if schema.table == "fc_transaction" and header == "交易时间":
            expr = time_expr()
            if expr != "NULL":
                select_cols.append(f"{expr} AS {column}")
            else:
                mapping_missing.append(header)
                select_cols.append(f"CAST(NULL AS TEXT) AS {column}")
            continue

        if schema.table in EXTRA_JSON_TABLES:
            if header in COALESCE_SOURCE_HEADERS_BY_TABLE.get(schema.table, {}):
                refs = find_refs_with_aliases(header, include_all=True)
                ref = refs[0][0] if refs else None
            else:
                refs = []
                resolved = find_ref_with_aliases(header)
                ref = resolved[0] if resolved else None
        else:
            refs = []
            ref = resolve_column_ref(header)

        if ref:
            if refs:
                expr = coalesced_value_expr(refs, keep_empty_tail=header == "交易对手账卡号")
            elif schema.table == "fc_transaction" and header == "交易对手账卡号":
                expr = value_keep_empty_expr(ref)
            else:
                expr = value_expr(ref)
            if header in derived_values:
                expr = f"COALESCE({expr}, {derived_values[header]})"
                mapping_derived.append(f"{header}=文件名")
            select_cols.append(f"{expr} AS {column}")
        else:
            derived_expr = derived_values.get(header)
            if derived_expr:
                mapping_derived.append(f"{header}=文件名")
                select_cols.append(f"{derived_expr} AS {column}")
            else:
                mapping_missing.append(header)
                select_cols.append(f"CAST(NULL AS TEXT) AS {column}")

    extra_json_expr = build_extra_json_expr(
        schema=schema,
        raw_headers=raw_headers,
        mapping_used_headers=mapping_used_headers,
        resolve_column_ref=resolve_column_ref,
        value_expr=value_expr,
        supports_json_object=supports_json_object,
    )
    raw_json_expr = build_raw_json_expr(
        schema=schema,
        resolve_column_ref=resolve_column_ref,
        value_expr=value_expr,
        supports_json_object=supports_json_object,
    )
    hash_expr = row_hash_expr(
        [f"base.{raw_col(schema.col_map[header])}" for header in schema.headers],
        precleaned_source=precleaned_source,
    )
    filter_headers = [
        header
        for header in schema.headers
        if header not in ROW_FILTER_IGNORED_HEADERS_BY_TABLE.get(schema.table, set())
    ] or list(schema.headers)
    non_empty_filter_expr = " OR ".join(
        f"NULLIF(TRIM(COALESCE(CAST(base.{quote_ident(raw_col(schema.col_map[header]))} AS VARCHAR), '')), '') IS NOT NULL"
        for header in filter_headers
    )
    semantic_filter_expr = build_semantic_row_filter_expr(schema, alias="base")
    if semantic_filter_expr:
        non_empty_filter_expr = f"({non_empty_filter_expr}) AND ({semantic_filter_expr})"
    return ImportProjection(
        select_cols=select_cols,
        extra_json_expr=extra_json_expr,
        raw_json_expr=raw_json_expr,
        hash_expr=hash_expr,
        non_empty_filter_expr=non_empty_filter_expr,
        force_not_null_sql=force_not_null_sql,
        mapping_used_headers=mapping_used_headers,
        mapping_alias_used=mapping_alias_used,
        mapping_missing=mapping_missing,
        mapping_derived=mapping_derived,
    )


def build_extra_json_expr(
    *,
    schema: FcProjectionSchema,
    raw_headers: Sequence[str],
    mapping_used_headers: Set[str],
    resolve_column_ref,
    value_expr,
    supports_json_object: bool,
) -> str:
    if schema.table not in EXTRA_JSON_TABLES:
        return ""
    if not supports_json_object:
        return ", NULL AS extra_json"
    standard_headers = set(schema.headers)
    extra_pairs: List[str] = []
    for raw_name in raw_headers:
        sanitized = sanitize_header(raw_name)
        if (
            not sanitized
            or sanitized in standard_headers
            or sanitized in mapping_used_headers
            or is_generated_unnamed_header(sanitized)
        ):
            continue
        ref = resolve_column_ref(sanitized)
        if ref:
            extra_pairs.append(f"{sql_literal(sanitized)}, {value_expr(ref)}")
    return f", json_object({', '.join(extra_pairs)}) AS extra_json" if extra_pairs else ", NULL AS extra_json"


def build_raw_json_expr(
    *,
    schema: FcProjectionSchema,
    resolve_column_ref,
    value_expr,
    supports_json_object: bool,
) -> str:
    if not schema.store_raw_json:
        return ""
    if not supports_json_object:
        return ", NULL AS raw_json"
    json_pairs = []
    for header in schema.headers:
        ref = resolve_column_ref(header)
        val_expr = value_expr(ref) if ref else "NULL"
        json_pairs.append(f"{sql_literal(header)}, {val_expr}")
    return f", json_object({', '.join(json_pairs)}) AS raw_json"


def norm_date_expr(expr: str) -> str:
    return (
        "CASE "
        f"WHEN {expr} IS NULL THEN NULL "
        f"WHEN regexp_matches({expr}, '^[0-9]{{8}}$') THEN "
        f"  substr({expr},1,4) || '-' || substr({expr},5,2) || '-' || substr({expr},7,2) "
        f"ELSE {expr} END"
    )


def norm_time_expr(expr: str) -> str:
    return (
        "CASE "
        f"WHEN {expr} IS NULL THEN NULL "
        f"WHEN regexp_matches({expr}, '^[0-9]{{6}}$') THEN "
        f"  substr({expr},1,2) || ':' || substr({expr},3,2) || ':' || substr({expr},5,2) "
        f"WHEN regexp_matches({expr}, '^[0-9]{{4}}$') THEN "
        f"  substr({expr},1,2) || ':' || substr({expr},3,2) || ':00' "
        f"ELSE {expr} END"
    )


def norm_datetime_expr(expr: str) -> str:
    return (
        "CASE "
        f"WHEN {expr} IS NULL THEN NULL "
        f"WHEN regexp_matches({expr}, '^[0-9]{{14}}$') THEN "
        f"  substr({expr},1,4) || '-' || substr({expr},5,2) || '-' || substr({expr},7,2) || "
        f"  ' ' || substr({expr},9,2) || ':' || substr({expr},11,2) || ':' || substr({expr},13,2) "
        f"WHEN regexp_matches({expr}, '^[0-9]{{12}}$') THEN "
        f"  substr({expr},1,4) || '-' || substr({expr},5,2) || '-' || substr({expr},7,2) || "
        f"  ' ' || substr({expr},9,2) || ':' || substr({expr},11,2) || ':00' "
        f"WHEN regexp_matches({expr}, '^[0-9]{{8}}$') THEN "
        f"  substr({expr},1,4) || '-' || substr({expr},5,2) || '-' || substr({expr},7,2) "
        f"ELSE {expr} END"
    )


def _force_not_null_sql(
    *,
    header_map_value: Dict[str, str],
    header_index_value: Dict[str, int],
    use_positional: bool,
    col_names: Sequence[str],
) -> str:
    raw_counterparty_header = header_map_value.get("交易对手账卡号")
    if not raw_counterparty_header:
        return ""
    if not use_positional:
        return "[" + sql_literal(raw_counterparty_header) + "]"
    raw_index = header_index_value.get(raw_counterparty_header)
    if raw_index is None or raw_index >= len(col_names):
        return ""
    return "[" + sql_literal(col_names[raw_index]) + "]"
