from __future__ import annotations

import time
from typing import Any, Optional, Sequence

from app.repositories.txn_daily_materialization_profile import TxnDailyMaterializationProfile
from app.repositories.txn_materialization_sql_steps import MaterializationSqlStep


def execute_materialization_sql_steps(
    con: Any,
    steps: Sequence[MaterializationSqlStep],
    *,
    profile: Optional[TxnDailyMaterializationProfile] = None,
) -> None:
    for step in steps:
        started = time.perf_counter()
        _execute_step(con, step)
        if profile is not None:
            profile.record_since(step.phase_name, started)


def execute_materialized_index_sql_steps(
    con: Any,
    steps: Sequence[MaterializationSqlStep],
    *,
    profile: Optional[TxnDailyMaterializationProfile] = None,
) -> None:
    for step in steps:
        started = time.perf_counter()
        try:
            _execute_step(con, step)
        except Exception:
            pass
        finally:
            if profile is not None:
                profile.record_index_elapsed(step.phase_name, time.perf_counter() - started)


def _execute_step(con: Any, step: MaterializationSqlStep) -> None:
    for statement in step.statements:
        if statement.params:
            con.execute(statement.sql, statement.params)
        else:
            con.execute(statement.sql)
