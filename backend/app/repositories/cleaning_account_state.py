from __future__ import annotations

from typing import Any

from app.core.fc_import_norm_insert import ts_norm_expr as _ts_norm_expr
from app.repositories import cleaning_duckdb_meta, cleaning_sql_expr


def update_acct_state(executor: Any, cur: Any, updated_at: str) -> None:
    if not cleaning_duckdb_meta.table_exists(cur, "acct_state"):
        return

    acct_key_expr = cleaning_sql_expr.account_key_expr()
    txn_ts_expr = f"COALESCE(txn_ts, {_ts_norm_expr('txn_time')})"
    bal_val = "COALESCE(balance_val, TRY_CAST(clean_balance AS DOUBLE), TRY_CAST(balance AS DOUBLE))"
    dc_final = "COALESCE(NULLIF(dc_final,''), NULLIF(clean_dc_flag,''), dc_norm)"
    txn_where = executor.scope_state.txn_where_base or executor.scope_state.txn_where
    txn_params = list(executor.scope_state.txn_where_base_params or executor.scope_state.txn_params)
    sql = f"""
        WITH scoped AS (
            SELECT {acct_key_expr} AS acct_key
            FROM fc_transaction_norm
            WHERE {txn_where}
        ),
        affected AS (
            SELECT DISTINCT acct_key
            FROM scoped
            WHERE acct_key IS NOT NULL AND acct_key<>''
        ),
        candidates AS (
            SELECT {acct_key_expr} AS acct_key,
                   {txn_ts_expr} AS txn_ts,
                   txn_time,
                   row_no,
                   id,
                   {bal_val} AS bal_val,
                   {dc_final} AS dc_final
            FROM fc_transaction_norm
            WHERE case_id=?
        ),
        ranked AS (
            SELECT c.acct_key,
                   c.txn_ts,
                   c.bal_val,
                   c.dc_final,
                   ROW_NUMBER() OVER (
                       PARTITION BY c.acct_key
                       ORDER BY CASE WHEN c.txn_ts IS NULL THEN 1 ELSE 0 END,
                                c.txn_ts DESC,
                                c.row_no DESC,
                                c.txn_time DESC,
                                c.id DESC
                   ) AS rn
            FROM candidates AS c
            JOIN affected AS a ON a.acct_key=c.acct_key
        )
        INSERT INTO acct_state(case_id, acct_key, last_txn_ts, last_balance, last_dc, updated_at)
        SELECT ?, acct_key, txn_ts, bal_val, dc_final, ?
        FROM ranked
        WHERE rn=1 AND acct_key IS NOT NULL AND acct_key<>''
        ON CONFLICT(case_id, acct_key) DO UPDATE SET
            last_txn_ts=excluded.last_txn_ts,
            last_balance=excluded.last_balance,
            last_dc=excluded.last_dc,
            updated_at=excluded.updated_at
    """
    cur.execute(sql, (*txn_params, executor.case_id, executor.case_id, updated_at))


__all__ = ["update_acct_state"]
