from __future__ import annotations

from typing import Callable, Set

from app.core.fc_import_norm_exprs import norm_key_expr, num_norm_expr, ts_norm_expr
from app.core.fc_import_projection import raw_col
from app.core.fc_norm_sql_helpers import dedupe_preserve_order, select_list_sql


ACCOUNT_META_COLUMNS: tuple[str, ...] = ("case_id", "file_id", "row_no", "imported_at", "row_hash")


def build_account_norm_insert_sql(
    *,
    raw_table: str,
    norm_table: str,
    delta_join: str,
    where: list[str],
    params: list,
    skip_existing_check: bool,
    missing_header_set: Set[str],
    insert_base_cols: list[str],
    insert_cols: list[str],
    clean_expr: Callable[[str], str],
    precleaned_source: bool,
    input_values_cleaned: bool,
) -> tuple[str, tuple]:
    missing_open_time = "账号开户时间" in missing_header_set
    missing_balance = "账户余额" in missing_header_set
    missing_available_balance = "可用余额" in missing_header_set
    missing_card = "交易卡号" in missing_header_set
    missing_acct = "交易账号" in missing_header_set

    raw_open = f"src.{raw_col('open_time')}"
    raw_bal = f"src.{raw_col('balance')}"
    raw_avail = f"src.{raw_col('available_balance')}"
    raw_card = f"src.{raw_col('card_no')}"
    raw_acct = f"src.{raw_col('acct_no')}"

    norm_base_exprs: list[tuple[str, str]] = []
    if not missing_open_time:
        norm_base_exprs.append(("__open_ts", ts_norm_expr(clean_expr(raw_open))))

    if not missing_balance:
        norm_base_exprs.append(("__bal_clean", num_norm_expr(clean_expr(raw_bal))))

    if not missing_available_balance:
        norm_base_exprs.append(("__avail_clean", num_norm_expr(clean_expr(raw_avail))))

    if not missing_card:
        norm_base_exprs.append(
            (
                "__card_norm",
                norm_key_expr(
                    raw_card,
                    precleaned_source=precleaned_source,
                    input_values_cleaned=input_values_cleaned,
                ),
            )
        )

    if not missing_acct:
        norm_base_exprs.append(
            (
                "__acct_norm",
                norm_key_expr(
                    raw_acct,
                    precleaned_source=precleaned_source,
                    input_values_cleaned=input_values_cleaned,
                ),
            )
        )

    out = "out"
    out_card = f"{out}.{raw_col('card_no')}"
    out_acct = f"{out}.{raw_col('acct_no')}"
    suffix_fixed = _suffix_fixed_expr(
        out=out,
        out_card=out_card,
        out_acct=out_acct,
        missing_card=missing_card,
        missing_acct=missing_acct,
    )

    account_select_exprs = [
        f"{out}.case_id",
        f"{out}.file_id",
        f"{out}.row_no",
        f"{out}.imported_at",
        f"{out}.row_hash",
    ]
    account_select_exprs.extend([f"{out}.{raw_col(column)} AS {column}" for column in insert_base_cols])
    account_extra_cols = [
        ("open_time_ts", f"{out}.__open_ts AS open_time_ts", missing_open_time),
        ("balance_val", f"{out}.__bal_val AS balance_val", missing_balance),
        ("available_balance_val", f"{out}.__avail_val AS available_balance_val", missing_available_balance),
        ("card_no_norm", f"{out}.__card_norm AS card_no_norm", missing_card),
        ("acct_no_norm", f"{out}.__acct_norm AS acct_no_norm", missing_acct),
        ("clean_card_no", f"{out}.__card_norm AS clean_card_no", missing_card),
        ("clean_acct_no", f"{out}.__acct_norm AS clean_acct_no", missing_acct),
        ("clean_suffix_fixed", f"{suffix_fixed} AS clean_suffix_fixed", missing_card and missing_acct),
        ("extra_json", f"{out}.extra_json AS extra_json", False),
    ]
    for column, expr, omit_column in account_extra_cols:
        if omit_column:
            continue
        insert_cols.append(column)
        account_select_exprs.append(expr)

    account_where = list(where)
    if not skip_existing_check:
        account_where.append(
            f"NOT EXISTS ("
            f"  SELECT 1 FROM {norm_table} n "
            f"  WHERE n.case_id=r.case_id AND n.row_hash=r.row_hash"
            f")"
        )
    src_columns = _account_src_columns(insert_base_cols, missing_header_set=missing_header_set)
    src_select_sql = select_list_sql(f"r.{column}" for column in src_columns)
    norm_base_select_sql = select_list_sql(
        (
            *(f"src.{column}" for column in src_columns),
            *(f"{expr} AS {column}" for column, expr in norm_base_exprs),
        )
    )
    norm_base_columns = (*src_columns, *(column for column, _ in norm_base_exprs))
    typed_exprs = _typed_exprs(
        missing_balance=missing_balance,
        missing_available_balance=missing_available_balance,
    )
    typed_select_sql = select_list_sql(
        (
            *(f"norm_base.{column}" for column in norm_base_columns),
            *(f"{expr} AS {column}" for column, expr in typed_exprs),
        )
    )
    sql = (
        f"INSERT INTO {norm_table}({', '.join(insert_cols)}) "
        "WITH src AS ("
        f"SELECT {src_select_sql} FROM {raw_table} r{delta_join} WHERE {' AND '.join(account_where)}"
        "), norm_base AS ("
        f"SELECT {norm_base_select_sql} "
        "FROM src"
        "), typed AS ("
        f"SELECT {typed_select_sql} "
        "FROM norm_base"
        ") "
        f"SELECT {', '.join(account_select_exprs)} FROM typed {out}"
    )
    return sql, tuple(params)


def _suffix_fixed_expr(
    *,
    out: str,
    out_card: str,
    out_acct: str,
    missing_card: bool,
    missing_acct: bool,
) -> str:
    checks: list[str] = []
    if not missing_card:
        checks.append(f"WHEN {out}.__card_norm IS NOT NULL AND {out}.__card_norm<>COALESCE(TRIM({out_card}), '') THEN 1")
    if not missing_acct:
        checks.append(f"WHEN {out}.__acct_norm IS NOT NULL AND {out}.__acct_norm<>COALESCE(TRIM({out_acct}), '') THEN 1")
    if not checks:
        return "0"
    return "CASE " + " ".join(checks) + " ELSE 0 END"


def _account_src_columns(insert_base_cols: list[str], *, missing_header_set: Set[str]) -> tuple[str, ...]:
    raw_columns = [
        raw_col(column)
        for column in (
            *insert_base_cols,
            *_account_required_norm_columns(missing_header_set),
        )
    ]
    return dedupe_preserve_order((*ACCOUNT_META_COLUMNS, *raw_columns, "extra_json"))


def _account_required_norm_columns(missing_header_set: Set[str]) -> tuple[str, ...]:
    header_by_column = {
        "open_time": "账号开户时间",
        "balance": "账户余额",
        "available_balance": "可用余额",
        "card_no": "交易卡号",
        "acct_no": "交易账号",
    }
    return tuple(
        column
        for column, header in header_by_column.items()
        if header not in missing_header_set
    )


def _typed_exprs(*, missing_balance: bool, missing_available_balance: bool) -> tuple[tuple[str, str], ...]:
    exprs: list[tuple[str, str]] = []
    if not missing_balance:
        exprs.append(("__bal_val", "TRY_CAST(__bal_clean AS DOUBLE)"))
    if not missing_available_balance:
        exprs.append(("__avail_val", "TRY_CAST(__avail_clean AS DOUBLE)"))
    return tuple(exprs)
