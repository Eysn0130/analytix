from __future__ import annotations

from dataclasses import dataclass

from app.repositories.txn_daily_materialization_plan_constants import (
    DETAIL_STAGING_TABLE,
)
from app.repositories.txn_daily_projection_sql import TxnProjectionExprs
from app.repositories.txn_name_pick_sql import build_name_pick_ctes_sql
from app.utils.counterparty_placeholders import placeholder_kind_sql


@dataclass(frozen=True)
class DetailIndexSelectItem:
    output_name: str
    expression: str

    @property
    def sql(self) -> str:
        return f"{self.expression} AS {self.output_name}"


BASE_DETAIL_INDEX_COLUMNS: tuple[DetailIndexSelectItem, ...] = (
    DetailIndexSelectItem("id", "base.id"),
    DetailIndexSelectItem("txn_day", "base.txn_day"),
    DetailIndexSelectItem("acct_key", "base.acct_key"),
    DetailIndexSelectItem("cp_key", "base.cp_key"),
    DetailIndexSelectItem("cp_raw", "base.cp_raw"),
    DetailIndexSelectItem("cp_placeholder_kind", "base.cp_placeholder_kind"),
    DetailIndexSelectItem("cp_name", "base.cp_name"),
    DetailIndexSelectItem("dc_val", "base.dc_val"),
    DetailIndexSelectItem("txn_ts", "base.txn_ts"),
    DetailIndexSelectItem("txn_time", "base.txn_time"),
    DetailIndexSelectItem("amount", "base.amount"),
    DetailIndexSelectItem("amount_source_present", "base.amount_source_present"),
    DetailIndexSelectItem("amount_parse_failed", "base.amount_parse_failed"),
    DetailIndexSelectItem("balance", "base.balance"),
    DetailIndexSelectItem("card_no", "base.card_no"),
    DetailIndexSelectItem("acct_no", "base.acct_no"),
    DetailIndexSelectItem("account_open_name", "base.account_open_name"),
    DetailIndexSelectItem("opener_id_no", "base.opener_id_no"),
    DetailIndexSelectItem("counterparty_acct", "base.counterparty_acct"),
    DetailIndexSelectItem("cash_flag", "base.cash_flag"),
    DetailIndexSelectItem("counterparty_name", "base.cp_name"),
    DetailIndexSelectItem("counterparty_id_no", "base.counterparty_id_no"),
    DetailIndexSelectItem("counterparty_bank", "base.counterparty_bank"),
    DetailIndexSelectItem("summary", "base.summary"),
    DetailIndexSelectItem("currency", "base.currency"),
    DetailIndexSelectItem("branch_name", "base.branch_name"),
    DetailIndexSelectItem("branch_code", "base.branch_code"),
    DetailIndexSelectItem("location", "base.location"),
    DetailIndexSelectItem("is_success", "base.is_success"),
    DetailIndexSelectItem("voucher_no", "base.voucher_no"),
    DetailIndexSelectItem("terminal_no", "base.terminal_no"),
    DetailIndexSelectItem("ip_addr", "base.ip_addr"),
    DetailIndexSelectItem("mac_addr", "base.mac_addr"),
    DetailIndexSelectItem("counterparty_balance", "base.counterparty_balance"),
    DetailIndexSelectItem("txn_id", "base.txn_id"),
    DetailIndexSelectItem("file_id", "base.file_id"),
    DetailIndexSelectItem("log_id", "base.log_id"),
    DetailIndexSelectItem("voucher_type", "base.voucher_type"),
    DetailIndexSelectItem("voucher_id", "base.voucher_id"),
    DetailIndexSelectItem("teller_no", "base.teller_no"),
    DetailIndexSelectItem("merchant_name", "base.merchant_name"),
    DetailIndexSelectItem("merchant_no", "base.merchant_no"),
    DetailIndexSelectItem("remark", "base.remark"),
    DetailIndexSelectItem("txn_type", "base.txn_type"),
    DetailIndexSelectItem("query_feedback_reason", "base.query_feedback_reason"),
)


def detail_index_select_items(exprs: TxnProjectionExprs) -> tuple[DetailIndexSelectItem, ...]:
    return (
        *BASE_DETAIL_INDEX_COLUMNS[:7],
        DetailIndexSelectItem("cp_name_pick", "name_pick.pick_name"),
        DetailIndexSelectItem("cp_name_pick_cnt", "COALESCE(name_pick.pick_cnt, 0)"),
        DetailIndexSelectItem("stats_name_key", exprs.resolved_name),
        *BASE_DETAIL_INDEX_COLUMNS[7:],
    )


def detail_source_select_items(exprs: TxnProjectionExprs) -> tuple[DetailIndexSelectItem, ...]:
    return (
        DetailIndexSelectItem("id", "t.id"),
        DetailIndexSelectItem("acct_key", exprs.acct_key),
        DetailIndexSelectItem("cp_key", exprs.cp_key),
        DetailIndexSelectItem("cp_raw", exprs.cp_raw),
        DetailIndexSelectItem("cp_placeholder_kind", placeholder_kind_sql("cp_raw")),
        DetailIndexSelectItem("cp_name", exprs.cp_name),
        DetailIndexSelectItem("dc_val", exprs.dc),
        DetailIndexSelectItem("txn_ts", exprs.txn_ts),
        DetailIndexSelectItem("txn_day", "CAST(txn_ts AS DATE)"),
        DetailIndexSelectItem("txn_time", exprs.txn_time),
        DetailIndexSelectItem("amount", exprs.amount),
        DetailIndexSelectItem("amount_source_present", exprs.amount_source_present),
        DetailIndexSelectItem("amount_parse_failed", exprs.amount_parse_failed),
        DetailIndexSelectItem("balance", exprs.balance),
        DetailIndexSelectItem("card_no", exprs.card),
        DetailIndexSelectItem("acct_no", exprs.acct),
        DetailIndexSelectItem("account_open_name", exprs.open_name),
        DetailIndexSelectItem("opener_id_no", exprs.opener_id),
        DetailIndexSelectItem("counterparty_acct", exprs.counterparty_acct),
        DetailIndexSelectItem("cash_flag", exprs.cash_flag),
        DetailIndexSelectItem("counterparty_id_no", exprs.counterparty_id),
        DetailIndexSelectItem("counterparty_bank", exprs.bank),
        DetailIndexSelectItem("summary", exprs.summary),
        DetailIndexSelectItem("currency", exprs.currency),
        DetailIndexSelectItem("branch_name", exprs.branch),
        DetailIndexSelectItem("branch_code", exprs.branch_code),
        DetailIndexSelectItem("location", exprs.location),
        DetailIndexSelectItem("is_success", exprs.success),
        DetailIndexSelectItem("voucher_no", exprs.voucher_no),
        DetailIndexSelectItem("terminal_no", exprs.terminal_no),
        DetailIndexSelectItem("ip_addr", exprs.ip),
        DetailIndexSelectItem("mac_addr", exprs.mac),
        DetailIndexSelectItem("counterparty_balance", exprs.counterparty_balance),
        DetailIndexSelectItem("txn_id", exprs.txn_id),
        DetailIndexSelectItem("file_id", exprs.file_id),
        DetailIndexSelectItem("log_id", exprs.log_id),
        DetailIndexSelectItem("voucher_type", exprs.voucher_type),
        DetailIndexSelectItem("voucher_id", exprs.voucher_id),
        DetailIndexSelectItem("teller_no", exprs.teller),
        DetailIndexSelectItem("merchant_name", exprs.merchant_name),
        DetailIndexSelectItem("merchant_no", exprs.merchant_no),
        DetailIndexSelectItem("remark", exprs.remark),
        DetailIndexSelectItem("txn_type", exprs.txn_type),
        DetailIndexSelectItem("query_feedback_reason", exprs.feedback),
    )


DETAIL_NAME_PICK_COLUMNS: tuple[DetailIndexSelectItem, ...] = (
    DetailIndexSelectItem("acct_key", "detail_base.acct_key"),
    DetailIndexSelectItem("cp_key", "detail_base.cp_key"),
    DetailIndexSelectItem("cp_name", "detail_base.cp_name"),
    DetailIndexSelectItem("txn_ts", "detail_base.txn_ts"),
)


def detail_name_base_select_items() -> tuple[DetailIndexSelectItem, ...]:
    return DETAIL_NAME_PICK_COLUMNS


def detail_name_base_select_sql() -> str:
    return ",\n                ".join(item.sql for item in detail_name_base_select_items())


def detail_base_select_sql(exprs: TxnProjectionExprs) -> str:
    return ",\n                ".join(item.sql for item in detail_source_select_items(exprs))


def build_detail_idx_sql(exprs: TxnProjectionExprs) -> str:
    select_sql = ",\n              ".join(item.sql for item in detail_index_select_items(exprs))
    return f"""
            CREATE TABLE {DETAIL_STAGING_TABLE} AS
            WITH detail_base AS (
              SELECT
                {detail_base_select_sql(exprs)}
              FROM fc_transaction_norm t
              WHERE {exprs.where_sql}
                AND acct_key IS NOT NULL
                AND TRIM(acct_key) <> ''
            ), detail_name_base AS (
              SELECT
                {detail_name_base_select_sql()}
              FROM detail_base
            ), {build_name_pick_ctes_sql('detail_name_base')}
            SELECT
              {select_sql}
            FROM detail_base base
            LEFT JOIN name_pick ON name_pick.pick_key = base.cp_key
            """
