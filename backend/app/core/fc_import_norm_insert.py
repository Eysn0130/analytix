from __future__ import annotations

from typing import Optional, Set

from app.core.fc_account_norm_insert_sql import build_account_norm_insert_sql
from app.core.fc_import_norm_exprs import (
    clean_text_expr,
    norm_key_expr,
    row_hash_expr,
    sanitize_precleaned_text_expr,
    sanitize_text_expr,
    ts_norm_expr,
)
from app.core.fc_import_projection import FcProjectionSchema, raw_col
from app.core.fc_transaction_norm_insert_sql import build_transaction_norm_insert_sql

DELTA_JOIN_IDENTITY_COLUMNS = ("case_id", "file_id", "row_no", "row_hash")


def build_norm_insert_sql(
    schema: FcProjectionSchema,
    raw_table: str,
    norm_table: str,
    *,
    case_id: Optional[str] = None,
    file_id: Optional[str] = None,
    delta_table: Optional[str] = None,
    precleaned_source: bool = False,
    skip_existing_check: bool = False,
    input_values_cleaned: bool = False,
    missing_headers: Optional[Set[str]] = None,
) -> tuple[str, tuple]:
    missing_header_set = set(missing_headers or [])
    clean_expr = (
        (lambda expr: expr)
        if input_values_cleaned
        else (sanitize_precleaned_text_expr if precleaned_source else clean_text_expr)
    )
    where = ["r.case_id IS NOT NULL"]
    params: list = []
    if case_id:
        where.append("r.case_id=?")
        params.append(case_id)
    if file_id:
        where.append("r.file_id=?")
        params.append(file_id)
    delta_join = ""
    if delta_table:
        delta_join = _delta_join_sql(delta_table)

    base_col_pairs = [(header, schema.col_map[header]) for header in schema.headers]
    insert_base_cols = [
        column for header, column in base_col_pairs if header not in missing_header_set
    ]
    select_exprs = [
        "r.case_id",
        "r.file_id",
        "r.row_no",
        "r.imported_at",
        "r.row_hash",
    ]
    select_exprs.extend([f"r.{raw_col(column)} AS {column}" for column in insert_base_cols])

    insert_cols = ["case_id", "file_id", "row_no", "imported_at", "row_hash"]
    insert_cols.extend(insert_base_cols)

    if schema.table == "fc_transaction":
        return build_transaction_norm_insert_sql(
            raw_table=raw_table,
            norm_table=norm_table,
            delta_join=delta_join,
            where=where,
            params=params,
            precleaned_source=precleaned_source,
            skip_existing_check=skip_existing_check,
            input_values_cleaned=input_values_cleaned,
            missing_header_set=missing_header_set,
            insert_base_cols=insert_base_cols,
            insert_cols=insert_cols,
            clean_expr=clean_expr,
        )
    if schema.table == "fc_account":
        return build_account_norm_insert_sql(
            raw_table=raw_table,
            norm_table=norm_table,
            delta_join=delta_join,
            where=where,
            params=params,
            skip_existing_check=skip_existing_check,
            missing_header_set=missing_header_set,
            insert_base_cols=insert_base_cols,
            insert_cols=insert_cols,
            clean_expr=clean_expr,
            precleaned_source=precleaned_source,
            input_values_cleaned=input_values_cleaned,
        )
    elif schema.table == "fc_sub_account":
        insert_cols.extend(["extra_json"])
        select_exprs.extend(["r.extra_json AS extra_json"])

    existing_check = ""
    if not skip_existing_check:
        existing_check = (
            f" AND NOT EXISTS ("
            f"  SELECT 1 FROM {norm_table} n "
            f"  WHERE n.case_id=r.case_id AND n.row_hash=r.row_hash"
            f")"
        )
    sql = (
        f"INSERT INTO {norm_table}({', '.join(insert_cols)}) "
        f"SELECT {', '.join(select_exprs)} "
        f"FROM {raw_table} r{delta_join} "
        f"WHERE {' AND '.join(where)}"
        f"{existing_check}"
    )
    return sql, tuple(params)


def _delta_join_sql(delta_table: str) -> str:
    predicates = " AND ".join(f"d.{column}=r.{column}" for column in DELTA_JOIN_IDENTITY_COLUMNS)
    return f" JOIN {delta_table} d ON {predicates}"
