from __future__ import annotations

from typing import Callable, Set

from app.core.fc_import_norm_exprs import dc_norm_expr, norm_key_expr, num_norm_expr, ts_norm_expr
from app.core.fc_import_projection import raw_col
from app.core.fc_norm_sql_helpers import dedupe_preserve_order, select_list_sql


TRANSACTION_META_COLUMNS: tuple[str, ...] = ("case_id", "file_id", "row_no", "imported_at", "row_hash")
TRANSACTION_NORMALIZED_RAW_COLUMNS: tuple[str, ...] = (
    "txn_time",
    "amount",
    "balance",
    "dc_flag",
    "card_no",
    "acct_no",
    "counterparty_acct",
)


def build_transaction_norm_insert_sql(
    *,
    raw_table: str,
    norm_table: str,
    delta_join: str,
    where: list[str],
    params: list,
    precleaned_source: bool,
    skip_existing_check: bool,
    input_values_cleaned: bool,
    missing_header_set: Set[str],
    insert_base_cols: list[str],
    insert_cols: list[str],
    clean_expr: Callable[[str], str],
) -> tuple[str, tuple]:
    missing_txn_time = "交易时间" in missing_header_set
    missing_amount = "交易金额" in missing_header_set
    missing_balance = "交易余额" in missing_header_set
    missing_dc = "收付标志" in missing_header_set
    missing_card = "交易卡号" in missing_header_set
    missing_acct = "交易账号" in missing_header_set
    missing_cpty_acct = "交易对手账卡号" in missing_header_set
    raw_txn = f"src.{raw_col('txn_time')}"
    raw_amt = f"src.{raw_col('amount')}"
    raw_bal = f"src.{raw_col('balance')}"
    raw_dc = f"src.{raw_col('dc_flag')}"
    raw_card = f"src.{raw_col('card_no')}"
    raw_acct = f"src.{raw_col('acct_no')}"
    raw_cpty_acct = f"src.{raw_col('counterparty_acct')}"

    norm_base_exprs: list[tuple[str, str]] = []
    if not missing_txn_time:
        norm_base_exprs.append(("__txn_ts", ts_norm_expr(clean_expr(raw_txn))))
    if not missing_amount:
        norm_base_exprs.append(("__amt_clean", num_norm_expr(clean_expr(raw_amt))))
    if not missing_balance:
        norm_base_exprs.append(("__bal_clean", num_norm_expr(clean_expr(raw_bal))))
    if not missing_dc:
        norm_base_exprs.append(("__dc_norm", dc_norm_expr(clean_expr(raw_dc))))
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
    if not missing_cpty_acct:
        norm_base_exprs.append(
            (
                "__cpty_norm",
                norm_key_expr(
                    raw_cpty_acct,
                    precleaned_source=precleaned_source,
                    input_values_cleaned=input_values_cleaned,
                ),
            )
        )

    out = "out"
    out_amt = f"{out}.{raw_col('amount')}"
    out_bal = f"{out}.{raw_col('balance')}"
    out_dc = f"{out}.{raw_col('dc_flag')}"
    out_card = f"{out}.{raw_col('card_no')}"
    out_acct = f"{out}.{raw_col('acct_no')}"
    dc_final_expr = _dc_final_expr(out=out, missing_dc=missing_dc, missing_amount=missing_amount)
    amt_fixed = (
        f"CASE WHEN {out}.__amt_abs_clean IS NOT NULL "
        f"AND COALESCE(TRIM({out_amt}), '')<>'' "
        f"AND {out}.__amt_abs_clean<>COALESCE(TRIM({out_amt}), '') THEN 1 ELSE 0 END"
    )
    bal_fixed = (
        f"CASE WHEN {out}.__bal_clean IS NOT NULL "
        f"AND COALESCE(TRIM({out_bal}), '')<>'' "
        f"AND {out}.__bal_clean<>COALESCE(TRIM({out_bal}), '') THEN 1 ELSE 0 END"
    )
    amt_failed = (
        f"CASE WHEN {out}.__amt_clean IS NULL AND COALESCE(TRIM({out_amt}), '')<>'' THEN 1 ELSE 0 END"
    )
    bal_failed = (
        f"CASE WHEN {out}.__bal_clean IS NULL AND COALESCE(TRIM({out_bal}), '')<>'' THEN 1 ELSE 0 END"
    )
    suffix_fixed = _suffix_fixed_expr(
        out=out,
        out_card=out_card,
        out_acct=out_acct,
        missing_card=missing_card,
        missing_acct=missing_acct,
    )
    dc_norm_flag = f"CASE WHEN {out}.__dc_norm IS NOT NULL THEN 1 ELSE 0 END"
    dc_inferred_flag = _dc_inferred_flag_expr(out=out, missing_dc=missing_dc)

    txn_select_exprs = [
        f"{out}.case_id",
        f"{out}.file_id",
        f"{out}.row_no",
        f"{out}.imported_at",
        f"{out}.row_hash",
    ]
    txn_select_exprs.extend([f"{out}.{raw_col(column)} AS {column}" for column in insert_base_cols])
    txn_extra_cols = [
        ("txn_ts", f"{out}.__txn_ts AS txn_ts", missing_txn_time),
        ("amount_val", f"ABS({out}.__amt_val) AS amount_val", missing_amount),
        ("balance_val", f"{out}.__bal_val AS balance_val", missing_balance),
        ("dc_norm", f"{out}.__dc_norm AS dc_norm", missing_dc),
        ("card_no_norm", f"{out}.__card_norm AS card_no_norm", missing_card),
        ("acct_no_norm", f"{out}.__acct_norm AS acct_no_norm", missing_acct),
        ("counterparty_acct_norm", f"{out}.__cpty_norm AS counterparty_acct_norm", missing_cpty_acct),
        ("dc_final", f"{dc_final_expr} AS dc_final", missing_dc and missing_amount),
        ("orig_amount", f"{out_amt} AS orig_amount", missing_amount),
        ("orig_balance", f"{out_bal} AS orig_balance", missing_balance),
        ("orig_dc_flag", f"{out_dc} AS orig_dc_flag", missing_dc),
        ("orig_card_no", f"{out_card} AS orig_card_no", missing_card),
        ("clean_amount", f"{out}.__amt_abs_clean AS clean_amount", missing_amount),
        ("clean_balance", f"{out}.__bal_clean AS clean_balance", missing_balance),
        ("clean_dc_flag", f"{dc_final_expr} AS clean_dc_flag", missing_dc and missing_amount),
        ("clean_card_no", f"{out}.__card_norm AS clean_card_no", missing_card),
        ("clean_acct_no", f"{out}.__acct_norm AS clean_acct_no", missing_acct),
        ("clean_suffix_fixed", f"{suffix_fixed} AS clean_suffix_fixed", missing_card and missing_acct),
        ("clean_amt_fixed", f"{amt_fixed} AS clean_amt_fixed", missing_amount),
        ("clean_amt_failed", f"{amt_failed} AS clean_amt_failed", missing_amount),
        ("clean_bal_fixed", f"{bal_fixed} AS clean_bal_fixed", missing_balance),
        ("clean_bal_failed", f"{bal_failed} AS clean_bal_failed", missing_balance),
        ("clean_dc_normalized", f"{dc_norm_flag} AS clean_dc_normalized", missing_dc),
        ("clean_dc_inferred", f"{dc_inferred_flag} AS clean_dc_inferred", missing_amount),
        ("extra_json", f"{out}.extra_json AS extra_json", False),
    ]
    for column, expr, omit_column in txn_extra_cols:
        if omit_column:
            continue
        insert_cols.append(column)
        txn_select_exprs.append(expr)
    txn_where = list(where)
    if not skip_existing_check:
        txn_where.append(
            f"NOT EXISTS ("
            f"  SELECT 1 FROM {norm_table} n "
            f"  WHERE n.case_id=r.case_id AND n.row_hash=r.row_hash"
            f")"
        )
    src_columns = _transaction_src_columns(insert_base_cols, missing_header_set=missing_header_set)
    src_select_sql = select_list_sql(f"r.{column}" for column in src_columns)
    norm_base_select_sql = select_list_sql(
        (
            *(f"src.{column}" for column in src_columns),
            *(f"{expr} AS {column}" for column, expr in norm_base_exprs),
        )
    )
    norm_base_columns = (*src_columns, *(column for column, _ in norm_base_exprs))
    typed_exprs = _typed_exprs(missing_amount=missing_amount, missing_balance=missing_balance)
    typed_select_sql = select_list_sql(
        (
            *(f"norm_base.{column}" for column in norm_base_columns),
            *(f"{expr} AS {column}" for column, expr in typed_exprs),
        )
    )
    typed_columns = (*norm_base_columns, *(column for column, _ in typed_exprs))
    out_exprs = _out_exprs(missing_amount=missing_amount)
    out_select_sql = select_list_sql(
        (
            *(f"typed.{column}" for column in typed_columns),
            *(f"{expr} AS {column}" for column, expr in out_exprs),
        )
    )
    sql = (
        f"INSERT INTO {norm_table}({', '.join(insert_cols)}) "
        "WITH src AS ("
        f"SELECT {src_select_sql} FROM {raw_table} r{delta_join} WHERE {' AND '.join(txn_where)}"
        "), norm_base AS ("
        f"SELECT {norm_base_select_sql} "
        "FROM src"
        "), typed AS ("
        f"SELECT {typed_select_sql} "
        "FROM norm_base"
        "), out AS ("
        f"SELECT {out_select_sql} "
        "FROM typed"
        ") "
        f"SELECT {', '.join(txn_select_exprs)} FROM {out}"
    )
    return sql, tuple(params)


def _transaction_src_columns(insert_base_cols: list[str], *, missing_header_set: Set[str]) -> tuple[str, ...]:
    raw_columns = [
        raw_col(column)
        for column in (
            *insert_base_cols,
            *_transaction_required_norm_columns(missing_header_set),
        )
    ]
    return dedupe_preserve_order((*TRANSACTION_META_COLUMNS, *raw_columns, "extra_json"))


def _transaction_required_norm_columns(missing_header_set: Set[str]) -> tuple[str, ...]:
    header_by_column = {
        "txn_time": "交易时间",
        "amount": "交易金额",
        "balance": "交易余额",
        "dc_flag": "收付标志",
        "card_no": "交易卡号",
        "acct_no": "交易账号",
        "counterparty_acct": "交易对手账卡号",
    }
    return tuple(
        column
        for column in TRANSACTION_NORMALIZED_RAW_COLUMNS
        if header_by_column[column] not in missing_header_set
    )


def _typed_exprs(*, missing_amount: bool, missing_balance: bool) -> tuple[tuple[str, str], ...]:
    exprs: list[tuple[str, str]] = []
    if not missing_amount:
        exprs.append(("__amt_val", "TRY_CAST(__amt_clean AS DOUBLE)"))
    if not missing_balance:
        exprs.append(("__bal_val", "TRY_CAST(__bal_clean AS DOUBLE)"))
    return tuple(exprs)


def _out_exprs(*, missing_amount: bool) -> tuple[tuple[str, str], ...]:
    if missing_amount:
        return ()
    return (
        (
            "__amt_abs_clean",
            "CASE WHEN __amt_val < 0 THEN regexp_replace(__amt_clean, '^-', '') ELSE __amt_clean END",
        ),
        (
            "__dc_from_sign",
            "CASE WHEN __amt_val < 0 THEN '出' WHEN __amt_val > 0 THEN '进' ELSE NULL END",
        ),
    )


def _dc_final_expr(*, out: str, missing_dc: bool, missing_amount: bool) -> str:
    if missing_dc and missing_amount:
        return "NULL"
    if missing_dc:
        return f"{out}.__dc_from_sign"
    if missing_amount:
        return f"{out}.__dc_norm"
    return f"COALESCE({out}.__dc_norm, {out}.__dc_from_sign)"


def _dc_inferred_flag_expr(*, out: str, missing_dc: bool) -> str:
    if missing_dc:
        return f"CASE WHEN {out}.__dc_from_sign IS NOT NULL THEN 1 ELSE 0 END"
    return f"CASE WHEN {out}.__dc_norm IS NULL AND {out}.__dc_from_sign IS NOT NULL THEN 1 ELSE 0 END"


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
