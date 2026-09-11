from __future__ import annotations

import re
from typing import Optional

from app.core.fc_import_projection import sql_literal


def extract_invalid_duckdb_csv_param(message: str) -> Optional[str]:
    match = re.search(r'Invalid named parameter "([^"]+)"', message or "")
    if not match:
        return None
    return match.group(1).strip().lower()


def duckdb_csv_read_expr(
    *,
    csv_sql: str,
    encoding_sql: str,
    use_positional: bool,
    column_map_sql: str,
    force_not_null_sql: str,
    precleaned_csv: bool,
    all_varchar: bool,
    minimal: bool = False,
    drop_opts: Optional[set[str]] = None,
) -> str:
    opts = duckdb_csv_read_options(
        encoding_sql=encoding_sql,
        use_positional=use_positional,
        column_map_sql=column_map_sql,
        force_not_null_sql=force_not_null_sql,
        precleaned_csv=precleaned_csv,
        all_varchar=all_varchar,
        minimal=minimal,
    )
    if drop_opts:
        opts = [opt for opt in opts if opt[0].lower() not in drop_opts]
    if not opts:
        return f"read_csv_auto({csv_sql})"
    opts_sql = ", ".join(f"{name}={value}" for name, value in opts)
    reader = "read_csv" if precleaned_csv and not minimal else "read_csv_auto"
    return f"{reader}({csv_sql}, {opts_sql})"


def duckdb_csv_read_options(
    *,
    encoding_sql: str,
    use_positional: bool,
    column_map_sql: str,
    force_not_null_sql: str,
    precleaned_csv: bool,
    all_varchar: bool,
    minimal: bool = False,
) -> list[tuple[str, str]]:
    opts: list[tuple[str, str]] = []
    if not minimal:
        opts.append(("encoding", encoding_sql))
        opts.append(("parallel", "true"))
        if not precleaned_csv:
            opts.append(("ignore_errors", "true"))
        else:
            opts.append(("delim", sql_literal(",")))
            opts.append(("quote", sql_literal('"')))
            opts.append(("escape", sql_literal('"')))
    if use_positional:
        opts.append(("header", "false"))
        opts.append(("skip", "1"))
        if column_map_sql:
            opts.append(("columns", column_map_sql))
    else:
        opts.append(("header", "true"))
        if all_varchar and not minimal:
            opts.append(("all_varchar", "true"))
    if force_not_null_sql:
        opts.append(("force_not_null", force_not_null_sql))
    return opts
