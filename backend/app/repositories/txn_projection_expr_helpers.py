from __future__ import annotations

from typing import Optional, Sequence

from app.core.fc_import_norm_insert import ts_norm_expr
from app.repositories.txn_daily_materialization_plan_constants import (
    ACCOUNT_KEY_CANDIDATES,
    NULL_DOUBLE,
    NULL_TEXT,
    NULL_TIMESTAMP,
)


def trim_expr(expr: str) -> str:
    return f"NULLIF(TRIM(CAST({expr} AS VARCHAR)), '')"


def clean_text_expr(expr: str) -> str:
    cleaned = trim_expr(expr)
    cleaned = f"NULLIF({cleaned}, '-')"
    cleaned = f"NULLIF({cleaned}, '—')"
    cleaned = f"NULLIF({cleaned}, '－')"
    return cleaned


def coalesce_expr(exprs: Sequence[Optional[str]], default: str = "NULL") -> str:
    parts = [expr for expr in exprs if expr]
    if not parts:
        return default
    if len(parts) == 1:
        return parts[0]
    return "COALESCE(" + ",".join(parts) + ")"


def trim_keep_empty_expr(expr: str) -> str:
    return f"CASE WHEN {expr} IS NULL THEN NULL ELSE TRIM(CAST({expr} AS VARCHAR)) END"


def text_col_expr(prefix: str, columns: set[str], col: str, *, clean_invalid: bool = False) -> Optional[str]:
    if col not in columns:
        return None
    if clean_invalid:
        return clean_text_expr(f"{prefix}.{col}")
    return trim_expr(f"{prefix}.{col}")


def text_col_expr_minlen(prefix: str, columns: set[str], col: str, min_len: int = 2) -> Optional[str]:
    expr = text_col_expr(prefix, columns, col, clean_invalid=True)
    if not expr:
        return None
    return f"CASE WHEN {expr} IS NULL THEN NULL WHEN LENGTH({expr}) < {int(min_len)} THEN NULL ELSE {expr} END"


def num_col_expr(prefix: str, columns: set[str], col: str) -> Optional[str]:
    if col not in columns:
        return None
    if col.endswith("_val"):
        return f"{prefix}.{col}"
    return f"TRY_CAST({trim_expr(f'{prefix}.{col}')} AS DOUBLE)"


def account_key_expr(prefix: str, columns: set[str], candidates: Sequence[str] = ACCOUNT_KEY_CANDIDATES) -> str:
    return coalesce_expr([text_col_expr(prefix, columns, candidate, clean_invalid=True) for candidate in candidates], NULL_TEXT)


def counterparty_key_expr(prefix: str, columns: set[str]) -> str:
    return coalesce_expr(
        [text_col_expr(prefix, columns, column) for column in ("counterparty_acct_norm", "counterparty_acct")],
        NULL_TEXT,
    )


def counterparty_display_expr(prefix: str, columns: set[str]) -> str:
    return coalesce_expr(
        [text_col_expr(prefix, columns, column) for column in ("counterparty_acct", "counterparty_acct_norm")],
        NULL_TEXT,
    )


def counterparty_raw_expr(prefix: str, columns: set[str]) -> str:
    return coalesce_expr(
        [trim_keep_empty_expr(f"{prefix}.{column}") for column in ("counterparty_acct", "counterparty_acct_norm") if column in columns],
        NULL_TEXT,
    )


def dc_value_expr(prefix: str, columns: set[str]) -> str:
    return coalesce_expr([text_col_expr(prefix, columns, column) for column in ("dc_final", "clean_dc_flag", "dc_flag")], NULL_TEXT)


def amount_value_expr(prefix: str, columns: set[str]) -> str:
    return finite_num_value_expr(prefix, columns, ("clean_amount", "amount_val", "amount"))


def finite_num_value_expr(prefix: str, columns: set[str], candidates: Sequence[str]) -> str:
    branches = []
    for column in candidates:
        candidate = num_col_expr(prefix, columns, column)
        if candidate:
            present = (
                f"{prefix}.{column} IS NOT NULL"
                if column.endswith("_val")
                else f"{trim_expr(f'{prefix}.{column}')} IS NOT NULL"
            )
            branches.append(
                f"WHEN {present} THEN CASE WHEN ({candidate}) IS NOT NULL AND isfinite({candidate}) "
                f"THEN {candidate} ELSE NULL END"
            )
    return f"CASE {' '.join(branches)} ELSE {NULL_DOUBLE} END" if branches else NULL_DOUBLE


def amount_source_present_expr(prefix: str, columns: set[str]) -> str:
    source_parts = [
        f"{trim_expr(f'{prefix}.{column}')} IS NOT NULL"
        for column in ("clean_amount", "amount_val", "amount")
        if column in columns
    ]
    if not source_parts:
        return "0"
    return f"CASE WHEN ({' OR '.join(source_parts)}) THEN 1 ELSE 0 END"


def amount_parse_failed_expr(prefix: str, columns: set[str], amount_expr: str) -> str:
    source_present = amount_source_present_expr(prefix, columns)
    return f"CASE WHEN ({source_present})=1 AND ({amount_expr}) IS NULL THEN 1 ELSE 0 END"


def txn_ts_expr(prefix: str, columns: set[str]) -> str:
    parts: list[str] = []
    if "txn_ts" in columns:
        parts.append(f"{prefix}.txn_ts")
    if "txn_time" in columns:
        parts.append(ts_norm_expr(f"{prefix}.txn_time"))
    return coalesce_expr(parts, NULL_TIMESTAMP)
