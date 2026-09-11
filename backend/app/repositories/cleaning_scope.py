from __future__ import annotations

from typing import Any, List

from app.repositories import cleaning_duckdb_meta


def resolve_scope_file_ids(executor: Any, cur: Any) -> List[str]:
    if executor.file_ids:
        return list(dict.fromkeys(executor.file_ids))
    if not cleaning_duckdb_meta.table_exists(cur, "import_file_log"):
        return []
    # Default to the full active fc_* file set so repeated cleaning and
    # force-rebuild runs operate on the same deterministic scope.
    cur.execute(
        "SELECT file_id, COALESCE(finished_at, created_at) AS import_at "
        "FROM import_file_log "
        "WHERE case_id=? AND kind LIKE 'fc_%' "
        "ORDER BY COALESCE(finished_at, created_at) DESC, file_id DESC",
        (executor.case_id,),
    )
    rows = [(r[0], r[1]) for r in cur.fetchall() if r and r[0]]

    if not rows:
        return []
    return list(dict.fromkeys([file_id for file_id, _ in rows]))


def init_clean_scope(executor: Any, cur: Any) -> None:
    executor.scope_state.scope_file_ids = resolve_scope_file_ids(executor, cur)
    if not executor.scope_state.scope_file_ids:
        executor.scope_state.txn_where_base = "case_id=? AND 1=0"
        executor.scope_state.txn_where_base_params = [executor.case_id]
        executor.scope_state.txn_where = executor.scope_state.txn_where_base
        executor.scope_state.txn_params = [executor.case_id]
        executor.scope_state.txn_where_t = executor.scope_state.txn_where_base.replace("case_id", "t.case_id")
        executor.scope_state.acc_where = "case_id=? AND 1=0"
        executor.scope_state.acc_params = [executor.case_id]
        executor.scope_state.acc_where_a = executor.scope_state.acc_where.replace("case_id", "a.case_id")
        executor.scope_state.txn_scope_file_ids = []
        executor.scope_state.acc_scope_file_ids = []
        return

    scope_ids = list(executor.scope_state.scope_file_ids)
    txn_ids = list(scope_ids)
    acc_ids = list(scope_ids)
    if cleaning_duckdb_meta.table_exists(cur, "import_file_log"):
        placeholders = ",".join(["?"] * len(scope_ids))
        cur.execute(
            f"SELECT file_id, kind FROM import_file_log WHERE case_id=? AND file_id IN ({placeholders})",
            (executor.case_id, *scope_ids),
        )
        kind_map = {r[0]: r[1] for r in cur.fetchall() if r and r[0]}
        txn_ids = [fid for fid in scope_ids if kind_map.get(fid) == "fc_transaction"]
        acc_ids = [fid for fid in scope_ids if kind_map.get(fid) == "fc_account"]

    if txn_ids:
        placeholders = ",".join(["?"] * len(txn_ids))
        executor.scope_state.txn_where_base = f"case_id=? AND file_id IN ({placeholders})"
        executor.scope_state.txn_where_base_params = [executor.case_id, *txn_ids]
        executor.scope_state.txn_where = executor.scope_state.txn_where_base
        executor.scope_state.txn_params = [executor.case_id, *txn_ids]
        executor.scope_state.txn_where_t = executor.scope_state.txn_where_base.replace("case_id", "t.case_id").replace("file_id", "t.file_id")
        executor.scope_state.txn_scope_file_ids = list(txn_ids)
    else:
        executor.scope_state.txn_where_base = "case_id=?"
        executor.scope_state.txn_where_base_params = [executor.case_id]
        executor.scope_state.txn_where = executor.scope_state.txn_where_base
        executor.scope_state.txn_params = [executor.case_id]
        executor.scope_state.txn_where_t = "t.case_id=?"
        executor.scope_state.txn_scope_file_ids = []

    if acc_ids:
        placeholders = ",".join(["?"] * len(acc_ids))
        executor.scope_state.acc_where = f"case_id=? AND file_id IN ({placeholders})"
        executor.scope_state.acc_params = [executor.case_id, *acc_ids]
        executor.scope_state.acc_where_a = executor.scope_state.acc_where.replace("case_id", "a.case_id").replace("file_id", "a.file_id")
        executor.scope_state.acc_scope_file_ids = list(acc_ids)
    else:
        executor.scope_state.acc_where = "case_id=? AND 1=0"
        executor.scope_state.acc_params = [executor.case_id]
        executor.scope_state.acc_where_a = "a.case_id=? AND 1=0"
        executor.scope_state.acc_scope_file_ids = []


def create_txn_scope(executor: Any, cur: Any) -> None:
    cur.execute("DROP TABLE IF EXISTS txn_scope")
    cur.execute(
        f"CREATE TEMP TABLE txn_scope AS SELECT id FROM fc_transaction_norm WHERE {executor.scope_state.txn_where_base}",
        tuple(executor.scope_state.txn_where_base_params),
    )
    executor.scope_state.txn_where = "id IN (SELECT id FROM txn_scope)"
    executor.scope_state.txn_params = []
    executor.scope_state.txn_where_t = "t.id IN (SELECT id FROM txn_scope)"


def rebuild_cleaning_scope(executor: Any, cur: Any) -> None:
    init_clean_scope(executor, cur)
    create_txn_scope(executor, cur)


__all__ = [
    "create_txn_scope",
    "init_clean_scope",
    "rebuild_cleaning_scope",
    "resolve_scope_file_ids",
]
