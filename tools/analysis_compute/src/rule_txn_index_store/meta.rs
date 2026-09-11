use anyhow::Result;
use duckdb::Connection;

use super::source_snapshot::RuleTxnSourceSnapshot;

pub(super) fn rule_txn_index_is_current(
    conn: &Connection,
    source_snapshot: &RuleTxnSourceSnapshot,
) -> Result<bool> {
    if !crate::table_exists(conn, crate::RULE_TXN_INDEX_TABLE)? {
        return Ok(false);
    }
    if !crate::table_exists(conn, crate::META_TABLE)? {
        return Ok(false);
    }
    let mut stmt = conn.prepare(&format!(
        "SELECT agg_version, COALESCE(case_id, ''), source_revision, source_row_count, \
                COALESCE(CAST(source_max_txn_ts AS VARCHAR), ''), source_max_id, \
                COALESCE(source_signature, ''), COALESCE(result_signature, ''), row_count \
         FROM {} WHERE agg_name={}",
        crate::META_TABLE,
        crate::sql_literal(crate::RULE_TXN_INDEX_CACHE_KEY)
    ))?;
    let row = stmt.query_row([], |row| {
        Ok((
            row.get::<_, i64>(0)?,
            row.get::<_, String>(1)?,
            row.get::<_, i64>(2)?,
            row.get::<_, i64>(3)?,
            row.get::<_, String>(4)?,
            row.get::<_, i64>(5)?,
            row.get::<_, String>(6)?,
            row.get::<_, String>(7)?,
            row.get::<_, i64>(8)?,
        ))
    });
    let Ok((
        saved_version,
        saved_case_id,
        saved_revision,
        saved_count,
        saved_ts,
        saved_id,
        saved_source_signature,
        saved_result_signature,
        saved_index_count,
    )) = row
    else {
        return Ok(false);
    };
    let actual_index_count = crate::scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM {} WHERE case_id={}",
            crate::RULE_TXN_INDEX_TABLE,
            crate::sql_literal(&source_snapshot.case_id)
        ),
    )?;
    let actual_result_signature = crate::canonical_case_table_signature(
        conn,
        crate::RULE_TXN_INDEX_TABLE,
        &source_snapshot.case_id,
        "analytix.rule-transaction-index-result/v2",
        "TRY_CAST(r.row_id AS BIGINT), r.row_id, r.account_key, r.txn_id",
    );
    let Ok(actual_result_signature) = actual_result_signature else {
        return Ok(false);
    };
    Ok(saved_version == crate::RULE_TXN_INDEX_VERSION
        && saved_case_id == source_snapshot.case_id
        && saved_revision == source_snapshot.source_revision
        && saved_count == source_snapshot.row_count
        && saved_ts == source_snapshot.max_txn_ts.as_str()
        && saved_id == source_snapshot.max_id
        && saved_source_signature == source_snapshot.source_signature
        && crate::is_sha256_hex(&saved_result_signature)
        && saved_result_signature == actual_result_signature
        && saved_index_count >= 0
        && saved_index_count == actual_index_count)
}

pub(super) fn upsert_rule_txn_index_meta(
    conn: &Connection,
    source_snapshot: &RuleTxnSourceSnapshot,
    row_count: i64,
    result_signature: &str,
) -> Result<()> {
    if !crate::is_sha256_hex(result_signature) {
        anyhow::bail!("rule transaction result signature is invalid");
    }
    let max_ts_sql = if source_snapshot.max_txn_ts.trim().is_empty() {
        "NULL".to_string()
    } else {
        format!(
            "CAST({} AS TIMESTAMP)",
            crate::sql_literal(&source_snapshot.max_txn_ts)
        )
    };
    conn.execute_batch(&format!(
        "
        INSERT INTO {}(
            agg_name, agg_version, case_id, source_revision, source_row_count,
            source_max_txn_ts, source_max_id, source_signature, result_signature,
            built_at, row_count
        )
        VALUES ({}, {}, {}, {}, {}, {max_ts_sql}, {}, {}, {}, now(), {row_count})
        ON CONFLICT(agg_name) DO UPDATE
        SET agg_version=excluded.agg_version,
            case_id=excluded.case_id,
            source_revision=excluded.source_revision,
            source_row_count=excluded.source_row_count,
            source_max_txn_ts=excluded.source_max_txn_ts,
            source_max_id=excluded.source_max_id,
            source_signature=excluded.source_signature,
            result_signature=excluded.result_signature,
            built_at=excluded.built_at,
            row_count=excluded.row_count
        ",
        crate::META_TABLE,
        crate::sql_literal(crate::RULE_TXN_INDEX_CACHE_KEY),
        crate::RULE_TXN_INDEX_VERSION,
        crate::sql_literal(&source_snapshot.case_id),
        source_snapshot.source_revision,
        source_snapshot.row_count,
        source_snapshot.max_id,
        crate::sql_literal(&source_snapshot.source_signature),
        crate::sql_literal(result_signature)
    ))?;
    Ok(())
}
