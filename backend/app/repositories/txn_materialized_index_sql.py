from __future__ import annotations

from dataclasses import dataclass
from typing import Sequence

from app.repositories.txn_daily_materialization_plan_constants import (
    ACCOUNT_DIM_TABLE,
    DETAIL_TABLE,
)
from app.repositories.txn_materialization_sql_steps import (
    MaterializationSqlStatement,
    MaterializationSqlStep,
)


@dataclass(frozen=True)
class MaterializedIndexPlan:
    phase_name: str
    index_name: str
    table_name: str
    columns_sql: str

    @property
    def sql(self) -> str:
        return f"CREATE INDEX IF NOT EXISTS idx_{self.index_name} ON {self.table_name}({self.columns_sql})"


MATERIALIZED_INDEX_PLANS: tuple[MaterializedIndexPlan, ...] = (
    MaterializedIndexPlan("create_index_detail_acct_ts", "detail_acct_ts", DETAIL_TABLE, "acct_key, txn_ts"),
    MaterializedIndexPlan("create_index_detail_txn_id", "detail_txn_id", DETAIL_TABLE, "txn_id"),
    MaterializedIndexPlan("create_index_detail_file_ts", "detail_file_ts", DETAIL_TABLE, "file_id, txn_ts"),
    MaterializedIndexPlan("create_index_account_dim_key", "account_dim_key", ACCOUNT_DIM_TABLE, "account_key"),
)


def create_materialized_index_plans() -> tuple[MaterializedIndexPlan, ...]:
    return MATERIALIZED_INDEX_PLANS


def create_materialized_index_sql_steps(
    plans: Sequence[MaterializedIndexPlan] = MATERIALIZED_INDEX_PLANS,
) -> tuple[MaterializationSqlStep, ...]:
    return tuple(
        MaterializationSqlStep(plan.phase_name, (MaterializationSqlStatement(plan.sql),))
        for plan in plans
    )
