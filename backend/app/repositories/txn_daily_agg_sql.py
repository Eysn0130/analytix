from __future__ import annotations

from dataclasses import dataclass

from app.repositories.txn_daily_materialization_plan_constants import (
    AGG_STAGING_TABLE,
    DETAIL_STAGING_TABLE,
)


@dataclass(frozen=True)
class DailyAggSelectItem:
    output_name: str
    expression: str

    @property
    def sql(self) -> str:
        return f"{self.expression} AS {self.output_name}"


DAILY_AGG_SELECT_COLUMNS: tuple[DailyAggSelectItem, ...] = (
    DailyAggSelectItem("txn_day", "txn_day"),
    DailyAggSelectItem("acct_key", "acct_key"),
    DailyAggSelectItem("cp_key", "cp_key"),
    DailyAggSelectItem("cp_display", "MAX(cp_display)"),
    DailyAggSelectItem("cp_raw", "cp_raw"),
    DailyAggSelectItem("cp_placeholder_kind", "cp_placeholder_kind"),
    DailyAggSelectItem("cp_name", "cp_name"),
    DailyAggSelectItem("cp_name_pick", "MAX(cp_name_pick)"),
    DailyAggSelectItem("cp_name_pick_cnt", "MAX(cp_name_pick_cnt)"),
    DailyAggSelectItem("stats_name_key", "MAX(stats_name_key)"),
    DailyAggSelectItem("dc_val", "dc_val"),
    DailyAggSelectItem("txn_count", "COUNT(1)"),
    DailyAggSelectItem("amount_source_present_count", "SUM(amount_source_present)"),
    DailyAggSelectItem("amount_valid_count", "SUM(CASE WHEN amount IS NOT NULL THEN 1 ELSE 0 END)"),
    DailyAggSelectItem("amount_missing_count", "SUM(CASE WHEN amount_source_present=0 THEN 1 ELSE 0 END)"),
    DailyAggSelectItem("amount_parse_failed_count", "SUM(amount_parse_failed)"),
    DailyAggSelectItem(
        "amt_sum",
        "CASE WHEN COUNT(1)=SUM(CASE WHEN amount IS NOT NULL THEN 1 ELSE 0 END) "
        "THEN ROUND(SUM(amt_abs), 2) ELSE NULL END",
    ),
    DailyAggSelectItem("first_ts", "MIN(txn_ts)"),
    DailyAggSelectItem("last_ts", "MAX(txn_ts)"),
    DailyAggSelectItem("open_name", "MAX(account_open_name)"),
    DailyAggSelectItem("counterparty_bank", "MAX(counterparty_bank)"),
    DailyAggSelectItem("location", "MAX(location)"),
)

DAILY_AGG_GROUP_COLUMNS: tuple[str, ...] = (
    "txn_day",
    "acct_key",
    "cp_key",
    "cp_raw",
    "cp_placeholder_kind",
    "cp_name",
    "dc_val",
)

def daily_agg_select_items() -> tuple[DailyAggSelectItem, ...]:
    return DAILY_AGG_SELECT_COLUMNS


def daily_agg_group_sql() -> str:
    return ", ".join(DAILY_AGG_GROUP_COLUMNS)


def daily_agg_detail_select_items() -> tuple[DailyAggSelectItem, ...]:
    return (
        DailyAggSelectItem("txn_day", "txn_day"),
        DailyAggSelectItem("acct_key", "acct_key"),
        DailyAggSelectItem("cp_key", "cp_key"),
        DailyAggSelectItem(
            "cp_display",
            "COALESCE(NULLIF(counterparty_acct, ''), NULLIF(cp_raw, ''), cp_key)",
        ),
        DailyAggSelectItem("cp_raw", "cp_raw"),
        DailyAggSelectItem("cp_placeholder_kind", "cp_placeholder_kind"),
        DailyAggSelectItem("cp_name", "cp_name"),
        DailyAggSelectItem("cp_name_pick", "cp_name_pick"),
        DailyAggSelectItem("cp_name_pick_cnt", "cp_name_pick_cnt"),
        DailyAggSelectItem("stats_name_key", "stats_name_key"),
        DailyAggSelectItem("dc_val", "dc_val"),
        DailyAggSelectItem("amount", "amount"),
        DailyAggSelectItem("amount_source_present", "amount_source_present"),
        DailyAggSelectItem("amount_parse_failed", "amount_parse_failed"),
        DailyAggSelectItem("amt_abs", "ABS(amount)"),
        DailyAggSelectItem("txn_ts", "txn_ts"),
        DailyAggSelectItem("account_open_name", "account_open_name"),
        DailyAggSelectItem("counterparty_bank", "counterparty_bank"),
        DailyAggSelectItem("location", "location"),
    )


def daily_agg_detail_select_sql() -> str:
    return ",\n                ".join(item.sql for item in daily_agg_detail_select_items())


def build_daily_agg_sql() -> str:
    select_sql = ",\n              ".join(item.sql for item in daily_agg_select_items())
    return f"""
            CREATE TABLE {AGG_STAGING_TABLE} AS
            WITH detail_base AS (
              SELECT
                {daily_agg_detail_select_sql()}
              FROM {DETAIL_STAGING_TABLE}
            )
            SELECT
              {select_sql}
            FROM detail_base
            GROUP BY {daily_agg_group_sql()}
            """
