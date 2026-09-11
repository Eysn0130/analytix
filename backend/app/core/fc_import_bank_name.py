from __future__ import annotations

import re
from typing import Optional

from app.core.fc_import_projection import sql_literal


_EXT_RE = re.compile(r"\.(?:csv|xlsx?|txt)$", re.IGNORECASE)
_BRACKET_RE = re.compile(r"【.*?】|\[.*?\]")
_BANK_SUFFIXES = (
    "关联子账户信息",
    "强制措施信息",
    "交易明细信息",
    "交易明细",
    "账户交易明细",
    "交易流水",
    "账户信息",
    "人员信息",
    "任务信息(成功)",
    "任务信息(失败)",
    "任务信息（成功）",
    "任务信息（失败）",
)
_BANK_NAME_HINT_RE = re.compile(r"(银行|农村信用社|信用社|农信社)")


def derive_bank_name_from_file_name(file_name: str, *, kind: str = "") -> Optional[str]:
    value = str(file_name or "").replace("\\", "/").rsplit("/", 1)[-1]
    value = value.rsplit("::", 1)[-1]
    value = _EXT_RE.sub("", value).strip()
    value = _BRACKET_RE.sub("", value).strip()
    for suffix in _BANK_SUFFIXES:
        if value.endswith(suffix):
            value = value[: -len(suffix)].strip()
            break
    if not value or not _BANK_NAME_HINT_RE.search(value):
        return None
    return value


def bank_name_from_file_name_sql(file_name_expr: str) -> str:
    expr = str(file_name_expr or "").strip()
    if not expr:
        return "NULL"
    cleaned = f"regexp_replace(CAST({expr} AS TEXT), '^.*::', '')"
    cleaned = f"regexp_replace({cleaned}, '^.*[/\\\\]', '')"
    cleaned = f"regexp_replace({cleaned}, '\\\\.(csv|xlsx|xls|txt)$', '', 'i')"
    cleaned = f"regexp_replace({cleaned}, '【.*?】|\\\\[.*?\\\\]', '')"
    for suffix in _BANK_SUFFIXES:
        cleaned = f"regexp_replace({cleaned}, {sql_literal(re.escape(suffix) + '$')}, '')"
    stripped = f"NULLIF(TRIM({cleaned}), '')"
    return (
        "CASE "
        f"WHEN {stripped} IS NOT NULL AND regexp_matches({stripped}, '(银行|农村信用社|信用社|农信社)') "
        f"THEN {stripped} "
        "ELSE NULL END"
    )


def derived_bank_headers_for_kind(kind: str) -> tuple[str, ...]:
    normalized = str(kind or "").strip()
    if normalized in {"fc_sub_account", "fc_coercive_measure"}:
        return ("银行名称",)
    if normalized == "fc_account":
        return ("账号开户银行",)
    return ()
