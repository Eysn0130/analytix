from __future__ import annotations

from dataclasses import dataclass
from typing import Optional

from app.repositories.txn_daily_materialization_plan_constants import (
    ACCOUNT_DIM_STAGING_TABLE,
    ACCOUNT_DIM_TABLE,
    AGG_STAGING_TABLE,
    AGG_TABLE,
    DETAIL_STAGING_TABLE,
    DETAIL_TABLE,
    LEGACY_MATERIALIZATION_STAGING_TABLES,
)
from app.repositories.txn_account_dim_sql import build_account_dim_sql
from app.repositories.txn_daily_agg_sql import build_daily_agg_sql
from app.repositories.txn_daily_detail_sql import build_detail_idx_sql
from app.repositories.txn_daily_projection_sql import build_txn_projection_exprs
from app.repositories.txn_materialization_sql_steps import (
    MaterializationSqlStatement,
    MaterializationSqlStep,
    statements_from_sql,
)


@dataclass(frozen=True)
class TxnDailyMaterializationPlan:
    drop_staging_sql: tuple[str, ...]
    daily_agg_sql: str
    daily_agg_params: tuple
    detail_idx_sql: str
    detail_idx_params: tuple
    account_dim_sql: str
    account_dim_params: tuple
    swap_table_sql: tuple[str, ...]


def create_materialization_sql_steps(plan: TxnDailyMaterializationPlan) -> tuple[MaterializationSqlStep, ...]:
    return (
        MaterializationSqlStep("drop_staging_tables", statements_from_sql(plan.drop_staging_sql)),
        MaterializationSqlStep(
            "build_detail_idx",
            (MaterializationSqlStatement(plan.detail_idx_sql, plan.detail_idx_params),),
        ),
        MaterializationSqlStep(
            "build_daily_agg",
            (MaterializationSqlStatement(plan.daily_agg_sql, plan.daily_agg_params),),
        ),
        MaterializationSqlStep(
            "build_account_dim",
            (MaterializationSqlStatement(plan.account_dim_sql, plan.account_dim_params),),
        ),
        MaterializationSqlStep("swap_tables", statements_from_sql(plan.swap_table_sql)),
    )


def build_materialization_plan(
    *,
    case_id: str,
    txn_columns: set[str],
    account_columns: Optional[set[str]],
    sub_account_columns: Optional[set[str]] = None,
) -> TxnDailyMaterializationPlan:
    exprs = build_txn_projection_exprs(txn_columns)
    account_dim_params: tuple = ()
    if account_columns is not None:
        account_dim_params = (case_id,)
        if sub_account_columns is not None and {"case_id", "parent_acct", "sub_acct"}.issubset(sub_account_columns):
            account_dim_params = (case_id, case_id)
    return TxnDailyMaterializationPlan(
        drop_staging_sql=(
            *(f"DROP TABLE IF EXISTS {table_name}" for table_name in LEGACY_MATERIALIZATION_STAGING_TABLES),
            f"DROP TABLE IF EXISTS {AGG_STAGING_TABLE}",
            f"DROP TABLE IF EXISTS {DETAIL_STAGING_TABLE}",
            f"DROP TABLE IF EXISTS {ACCOUNT_DIM_STAGING_TABLE}",
        ),
        daily_agg_sql=build_daily_agg_sql(),
        daily_agg_params=(),
        detail_idx_sql=build_detail_idx_sql(exprs),
        detail_idx_params=(case_id,),
        account_dim_sql=build_account_dim_sql(account_columns, sub_account_columns),
        account_dim_params=account_dim_params,
        swap_table_sql=(
            f"DROP TABLE IF EXISTS {AGG_TABLE}",
            f"ALTER TABLE {AGG_STAGING_TABLE} RENAME TO {AGG_TABLE}",
            f"DROP TABLE IF EXISTS {DETAIL_TABLE}",
            f"ALTER TABLE {DETAIL_STAGING_TABLE} RENAME TO {DETAIL_TABLE}",
            f"DROP TABLE IF EXISTS {ACCOUNT_DIM_TABLE}",
            f"ALTER TABLE {ACCOUNT_DIM_STAGING_TABLE} RENAME TO {ACCOUNT_DIM_TABLE}",
        ),
    )
