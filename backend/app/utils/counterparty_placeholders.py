from __future__ import annotations

from typing import Any, Iterable

PLACEHOLDER_TOKEN_PREFIX = "__cp_placeholder__::"
PLACEHOLDER_NAME_SUFFIX = "::name::"

PLACEHOLDER_KIND_LABELS: dict[str, str] = {
    "db_null": "NULL",
    "empty": "空串",
    "slash_n": "\\N",
    "dash": "-",
    "emdash": "—",
    "fw_dash": "－",
    "literal_null": "null",
    "literal_none": "none",
    "literal_nan": "nan",
}

PLACEHOLDER_KIND_ORDER = tuple(PLACEHOLDER_KIND_LABELS.keys())
PLACEHOLDER_KIND_SET = set(PLACEHOLDER_KIND_ORDER)


def classify_counterparty_placeholder(value: Any) -> str | None:
    if value is None:
        return "db_null"
    text = str(value).strip()
    if text == "":
        return "empty"
    if text == "\\N":
        return "slash_n"
    if text == "-":
        return "dash"
    if text == "—":
        return "emdash"
    if text == "－":
        return "fw_dash"
    lowered = text.lower()
    if lowered == "null":
        return "literal_null"
    if lowered == "none":
        return "literal_none"
    if lowered == "nan":
        return "literal_nan"
    return None


def normalize_placeholder_kind(value: Any) -> str:
    text = str(value or "").strip()
    return text if text in PLACEHOLDER_KIND_SET else ""


def normalize_placeholder_kinds(values: Iterable[Any]) -> list[str]:
    out: list[str] = []
    seen: set[str] = set()
    for item in values or []:
        kind = normalize_placeholder_kind(item)
        if not kind or kind in seen:
            continue
        seen.add(kind)
        out.append(kind)
    return out


def placeholder_kind_label(kind: Any) -> str:
    return PLACEHOLDER_KIND_LABELS.get(str(kind or "").strip(), "")


def placeholder_key_token(kind: Any) -> str:
    normalized = normalize_placeholder_kind(kind)
    if not normalized:
        return ""
    return f"{PLACEHOLDER_TOKEN_PREFIX}{normalized}"


def placeholder_kind_from_token(value: Any) -> str:
    text = str(value or "").strip()
    if not text.startswith(PLACEHOLDER_TOKEN_PREFIX):
        return ""
    remainder = text[len(PLACEHOLDER_TOKEN_PREFIX) :]
    if PLACEHOLDER_NAME_SUFFIX in remainder:
        remainder = remainder.split(PLACEHOLDER_NAME_SUFFIX, 1)[0]
    return normalize_placeholder_kind(remainder)


def is_placeholder_key_token(value: Any) -> bool:
    return bool(placeholder_kind_from_token(value))


def placeholder_node_id(kind: Any, name: Any = "") -> str:
    base = placeholder_key_token(kind)
    if not base:
        return ""
    node_name = str(name or "").strip()
    if not node_name:
        return base
    return f"{base}{PLACEHOLDER_NAME_SUFFIX}{node_name}"


def placeholder_kind_sql(expr: str) -> str:
    trimmed = f"TRIM({expr})"
    lowered = f"LOWER({trimmed})"
    return (
        "CASE "
        f"WHEN {expr} IS NULL THEN 'db_null' "
        f"WHEN {trimmed} = '' THEN 'empty' "
        f"WHEN {trimmed} = (CHR(92) || 'N') THEN 'slash_n' "
        f"WHEN {trimmed} = '-' THEN 'dash' "
        f"WHEN {trimmed} = '—' THEN 'emdash' "
        f"WHEN {trimmed} = '－' THEN 'fw_dash' "
        f"WHEN {lowered} = 'null' THEN 'literal_null' "
        f"WHEN {lowered} = 'none' THEN 'literal_none' "
        f"WHEN {lowered} = 'nan' THEN 'literal_nan' "
        "ELSE NULL END"
    )


def placeholder_label_sql(kind_expr: str) -> str:
    return (
        "CASE "
        f"WHEN {kind_expr} = 'db_null' THEN 'NULL' "
        f"WHEN {kind_expr} = 'empty' THEN '空串' "
        f"WHEN {kind_expr} = 'slash_n' THEN '{chr(92)}N' "
        f"WHEN {kind_expr} = 'dash' THEN '-' "
        f"WHEN {kind_expr} = 'emdash' THEN '—' "
        f"WHEN {kind_expr} = 'fw_dash' THEN '－' "
        f"WHEN {kind_expr} = 'literal_null' THEN 'null' "
        f"WHEN {kind_expr} = 'literal_none' THEN 'none' "
        f"WHEN {kind_expr} = 'literal_nan' THEN 'nan' "
        "ELSE '' END"
    )


def placeholder_token_sql(kind_expr: str) -> str:
    return (
        "CASE "
        f"WHEN {kind_expr} = 'db_null' THEN '{placeholder_key_token('db_null')}' "
        f"WHEN {kind_expr} = 'empty' THEN '{placeholder_key_token('empty')}' "
        f"WHEN {kind_expr} = 'slash_n' THEN '{placeholder_key_token('slash_n')}' "
        f"WHEN {kind_expr} = 'dash' THEN '{placeholder_key_token('dash')}' "
        f"WHEN {kind_expr} = 'emdash' THEN '{placeholder_key_token('emdash')}' "
        f"WHEN {kind_expr} = 'fw_dash' THEN '{placeholder_key_token('fw_dash')}' "
        f"WHEN {kind_expr} = 'literal_null' THEN '{placeholder_key_token('literal_null')}' "
        f"WHEN {kind_expr} = 'literal_none' THEN '{placeholder_key_token('literal_none')}' "
        f"WHEN {kind_expr} = 'literal_nan' THEN '{placeholder_key_token('literal_nan')}' "
        "ELSE NULL END"
    )
