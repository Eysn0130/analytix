from __future__ import annotations

from dataclasses import dataclass

from app.repositories.txn_daily_materialization_plan_constants import (
    NULL_DOUBLE,
    NULL_TEXT,
    NULL_TIMESTAMP,
)
from app.repositories.txn_projection_expr_helpers import (
    account_key_expr,
    amount_parse_failed_expr,
    amount_source_present_expr,
    amount_value_expr,
    coalesce_expr,
    counterparty_display_expr,
    counterparty_key_expr,
    counterparty_raw_expr,
    dc_value_expr,
    finite_num_value_expr,
    num_col_expr,
    text_col_expr,
    trim_expr,
    txn_ts_expr,
)
from app.utils.counterparty_placeholders import placeholder_token_sql


@dataclass(frozen=True)
class TxnProjectionExprs:
    acct_key: str
    cp_key: str
    cp_display: str
    cp_raw: str
    dc: str
    amount: str
    amount_source_present: str
    amount_parse_failed: str
    txn_ts: str
    open_name: str
    card: str
    acct: str
    balance: str
    opener_id: str
    cp_name: str
    txn_time: str
    bank: str
    location: str
    counterparty_acct: str
    cash_flag: str
    counterparty_id: str
    summary: str
    currency: str
    branch: str
    branch_code: str
    success: str
    voucher_no: str
    terminal_no: str
    ip: str
    mac: str
    counterparty_balance: str
    txn_id: str
    file_id: str
    log_id: str
    voucher_type: str
    voucher_id: str
    teller: str
    merchant_name: str
    merchant_no: str
    remark: str
    txn_type: str
    feedback: str
    where_sql: str
    resolved_name: str


def build_txn_projection_exprs(columns: set[str]) -> TxnProjectionExprs:
    clean_columns = {"clean_invalid", "clean_failed", "clean_reversal"}
    if not clean_columns.issubset(columns):
        raise ValueError("transaction cleaning authority is unavailable")
    acct_key_expr = account_key_expr("t", columns)
    cp_key_expr = counterparty_key_expr("t", columns)
    cp_display_expr = counterparty_display_expr("t", columns)
    cp_raw_expr = counterparty_raw_expr("t", columns)
    dc_expr = dc_value_expr("t", columns)
    amt_expr = amount_value_expr("t", columns)
    amount_source_present = amount_source_present_expr("t", columns)
    amount_parse_failed = amount_parse_failed_expr("t", columns, amt_expr)
    ts_expr = txn_ts_expr("t", columns)
    open_name_expr = text_col_expr("t", columns, "account_open_name", clean_invalid=True) or NULL_TEXT
    card_expr = coalesce_expr(
        [
            text_col_expr("t", columns, "clean_card_no", clean_invalid=True),
            text_col_expr("t", columns, "card_no_norm", clean_invalid=True),
            text_col_expr("t", columns, "card_no", clean_invalid=True),
        ],
        NULL_TEXT,
    )
    acct_expr = coalesce_expr(
        [
            text_col_expr("t", columns, "clean_acct_no"),
            text_col_expr("t", columns, "acct_no_norm"),
            text_col_expr("t", columns, "acct_no"),
        ],
        NULL_TEXT,
    )
    balance_expr = finite_num_value_expr("t", columns, ("balance_val", "clean_balance", "balance"))
    opener_id_expr = text_col_expr("t", columns, "opener_id_no", clean_invalid=True) or NULL_TEXT
    cp_name_expr = text_col_expr("t", columns, "counterparty_name", clean_invalid=True) or NULL_TEXT
    txn_time_expr = (
        f"COALESCE({trim_expr('t.txn_time')}, CAST({ts_expr} AS VARCHAR))"
        if "txn_time" in columns
        else (f"CAST({ts_expr} AS VARCHAR)" if ts_expr != NULL_TIMESTAMP else NULL_TEXT)
    )
    bank_expr = text_col_expr("t", columns, "counterparty_bank") or NULL_TEXT
    location_expr = text_col_expr("t", columns, "location") or NULL_TEXT
    counterparty_acct_expr = text_col_expr("t", columns, "counterparty_acct") or NULL_TEXT
    cash_flag_expr = text_col_expr("t", columns, "cash_flag") or NULL_TEXT
    counterparty_id_expr = text_col_expr("t", columns, "counterparty_id_no") or NULL_TEXT
    summary_expr = text_col_expr("t", columns, "summary") or NULL_TEXT
    currency_expr = text_col_expr("t", columns, "currency") or NULL_TEXT
    branch_expr = text_col_expr("t", columns, "branch_name") or NULL_TEXT
    branch_code_expr = text_col_expr("t", columns, "branch_code") or NULL_TEXT
    success_expr = text_col_expr("t", columns, "is_success") or NULL_TEXT
    voucher_no_expr = text_col_expr("t", columns, "voucher_no") or NULL_TEXT
    terminal_no_expr = text_col_expr("t", columns, "terminal_no") or NULL_TEXT
    ip_expr = text_col_expr("t", columns, "ip_addr") or NULL_TEXT
    mac_expr = text_col_expr("t", columns, "mac_addr") or NULL_TEXT
    counterparty_balance_expr = finite_num_value_expr("t", columns, ("counterparty_balance",))
    txn_id_expr = text_col_expr("t", columns, "txn_id") or NULL_TEXT
    file_id_expr = text_col_expr("t", columns, "file_id") or NULL_TEXT
    log_id_expr = text_col_expr("t", columns, "log_id") or NULL_TEXT
    voucher_type_expr = text_col_expr("t", columns, "voucher_type") or NULL_TEXT
    voucher_id_expr = text_col_expr("t", columns, "voucher_id") or NULL_TEXT
    teller_expr = text_col_expr("t", columns, "teller_no") or NULL_TEXT
    merchant_name_expr = text_col_expr("t", columns, "merchant_name") or NULL_TEXT
    merchant_no_expr = text_col_expr("t", columns, "merchant_no") or NULL_TEXT
    remark_expr = text_col_expr("t", columns, "remark") or NULL_TEXT
    txn_type_expr = text_col_expr("t", columns, "txn_type") or NULL_TEXT
    feedback_expr = text_col_expr("t", columns, "query_feedback_reason") or NULL_TEXT

    where = [
        "t.case_id=?",
        "t.clean_invalid=0",
        "t.clean_failed=0",
        "t.clean_reversal=0",
    ]
    where_sql = " AND ".join(where)
    resolved_name_expr = (
        "CASE "
        "WHEN base.cp_name IS NOT NULL AND TRIM(base.cp_name) <> '' THEN base.cp_name "
        f"WHEN base.cp_placeholder_kind IS NOT NULL THEN {placeholder_token_sql('base.cp_placeholder_kind')} "
        "WHEN COALESCE(name_pick.pick_cnt, 0) >= 3 THEN name_pick.pick_name "
        "ELSE base.cp_key END"
    )

    return TxnProjectionExprs(
        acct_key=acct_key_expr,
        cp_key=cp_key_expr,
        cp_display=cp_display_expr,
        cp_raw=cp_raw_expr,
        dc=dc_expr,
        amount=amt_expr,
        amount_source_present=amount_source_present,
        amount_parse_failed=amount_parse_failed,
        txn_ts=ts_expr,
        open_name=open_name_expr,
        card=card_expr,
        acct=acct_expr,
        balance=balance_expr,
        opener_id=opener_id_expr,
        cp_name=cp_name_expr,
        txn_time=txn_time_expr,
        bank=bank_expr,
        location=location_expr,
        counterparty_acct=counterparty_acct_expr,
        cash_flag=cash_flag_expr,
        counterparty_id=counterparty_id_expr,
        summary=summary_expr,
        currency=currency_expr,
        branch=branch_expr,
        branch_code=branch_code_expr,
        success=success_expr,
        voucher_no=voucher_no_expr,
        terminal_no=terminal_no_expr,
        ip=ip_expr,
        mac=mac_expr,
        counterparty_balance=counterparty_balance_expr,
        txn_id=txn_id_expr,
        file_id=file_id_expr,
        log_id=log_id_expr,
        voucher_type=voucher_type_expr,
        voucher_id=voucher_id_expr,
        teller=teller_expr,
        merchant_name=merchant_name_expr,
        merchant_no=merchant_no_expr,
        remark=remark_expr,
        txn_type=txn_type_expr,
        feedback=feedback_expr,
        where_sql=where_sql,
        resolved_name=resolved_name_expr,
    )
