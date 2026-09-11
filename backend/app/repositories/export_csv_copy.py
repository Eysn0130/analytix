from __future__ import annotations

import re
from pathlib import Path
from typing import Any, Sequence

_ALIAS_RE = re.compile(r"\s+AS\s+([A-Za-z0-9_]+)$", re.IGNORECASE)
UTF8_BOM = "\ufeff"


def quote_identifier(value: str) -> str:
    return '"' + str(value or "").replace('"', '""') + '"'


def split_alias(column_expr: str) -> tuple[str, str]:
    match = _ALIAS_RE.search(column_expr)
    if match:
        alias = match.group(1)
        expr = column_expr[: match.start()].strip()
        return expr, alias
    return column_expr, column_expr


def build_csv_copy_sql(
    *,
    table: str,
    selected_cols: Sequence[str],
    headers: Sequence[str],
    has_case: bool,
    case_id: str,
    file_path: Path,
    include_utf8_bom: bool = True,
) -> tuple[str, tuple[Any, ...]]:
    if selected_cols:
        header_names = list(headers) or [split_alias(col)[1] for col in selected_cols]
        select_items: list[str] = []
        for idx, col in enumerate(selected_cols):
            expr, alias = split_alias(col)
            header = header_names[idx] if idx < len(header_names) else alias
            if idx == 0 and include_utf8_bom and not header.startswith(UTF8_BOM):
                header = f"{UTF8_BOM}{header}"
            select_items.append(f"{expr} AS {quote_identifier(header)}")
        cols_sql = ", ".join(select_items)
    else:
        cols_sql = "*"

    select_sql = f"SELECT {cols_sql} FROM {table}"
    params: tuple[Any, ...] = (str(file_path),)
    if has_case:
        select_sql += " WHERE case_id=?"
        params = (str(file_path), case_id)
    return f"COPY ({select_sql}) TO ? (FORMAT CSV, HEADER TRUE)", params
