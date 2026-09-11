use anyhow::{Context, Result};
use duckdb::Connection;

use super::args::RuleTxnIndexArgs;
use super::meta::{rule_txn_index_is_current, upsert_rule_txn_index_meta};
use super::source_snapshot::rule_txn_source_snapshot;
use super::staging::{create_rule_txn_index_staging, swap_rule_txn_index};

pub(crate) struct RuleTxnIndexResult {
    pub(crate) row_count: i64,
    pub(crate) rebuilt: bool,
}

pub(crate) fn materialize_rule_txn_index(args: &RuleTxnIndexArgs) -> Result<RuleTxnIndexResult> {
    let conn = Connection::open(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    crate::ensure_required_table(&conn, "fc_transaction_norm")?;
    crate::ensure_meta_table(&conn)?;

    let cols = crate::table_columns(&conn, "fc_transaction_norm")?;
    let source_snapshot = rule_txn_source_snapshot(&conn, &args.case_id, &cols)?;
    if !args.force && rule_txn_index_is_current(&conn, &source_snapshot)? {
        let row_count = conn.query_row(
            &format!(
                "SELECT COUNT(1) FROM {} WHERE case_id={}",
                crate::RULE_TXN_INDEX_TABLE,
                crate::sql_literal(&args.case_id)
            ),
            [],
            |row| row.get::<_, i64>(0),
        )?;
        return Ok(RuleTxnIndexResult {
            row_count,
            rebuilt: false,
        });
    }

    create_rule_txn_index_staging(&conn, &args.case_id, &cols)?;
    swap_rule_txn_index(&conn)?;
    let row_count = conn.query_row(
        &format!(
            "SELECT COUNT(1) FROM {} WHERE case_id={}",
            crate::RULE_TXN_INDEX_TABLE,
            crate::sql_literal(&args.case_id)
        ),
        [],
        |row| row.get::<_, i64>(0),
    )?;
    let result_signature = crate::canonical_case_table_signature(
        &conn,
        crate::RULE_TXN_INDEX_TABLE,
        &args.case_id,
        "analytix.rule-transaction-index-result/v2",
        "TRY_CAST(r.row_id AS BIGINT), r.row_id, r.account_key, r.txn_id",
    )?;
    upsert_rule_txn_index_meta(&conn, &source_snapshot, row_count, &result_signature)?;
    Ok(RuleTxnIndexResult {
        row_count,
        rebuilt: true,
    })
}
