use anyhow::{bail, Result};
use duckdb::Connection;
use sha2::{Digest, Sha256};

#[derive(Debug)]
pub(super) struct RuleTxnSourceSnapshot {
    pub(super) case_id: String,
    pub(super) row_count: i64,
    pub(super) max_txn_ts: String,
    pub(super) max_id: i64,
    pub(super) source_revision: i64,
    pub(super) source_signature: String,
}

pub(super) fn rule_txn_source_snapshot(
    conn: &Connection,
    case_id: &str,
    cols: &[String],
) -> Result<RuleTxnSourceSnapshot> {
    if !crate::has_col(cols, "id") {
        bail!("rule transaction row lineage is unavailable");
    }
    if crate::clean_row_acceptance_expr("t", cols).is_none() {
        bail!("rule transaction cleaning authority is unavailable");
    }
    crate::ensure_required_table(conn, "analysis_revision_state")?;
    let revision = conn.query_row(
        "SELECT COUNT(1), COALESCE(MIN(revision), 0), COALESCE(MAX(revision), 0) \
           FROM analysis_revision_state WHERE revision_key='stats_flow_source'",
        [],
        |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, i64>(1)?,
                row.get::<_, i64>(2)?,
            ))
        },
    )?;
    if revision.0 != 1 || revision.1 <= 0 || revision.1 != revision.2 {
        bail!("rule transaction source revision is unavailable");
    }
    let row_identity = conn.query_row(
        &format!(
            "SELECT COUNT(1), COUNT(TRY_CAST(t.id AS BIGINT)), \
                    COUNT(DISTINCT TRY_CAST(t.id AS BIGINT)) \
               FROM fc_transaction_norm t WHERE t.case_id={}",
            crate::sql_literal(case_id)
        ),
        [],
        |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, i64>(1)?,
                row.get::<_, i64>(2)?,
            ))
        },
    )?;
    if row_identity.0 != row_identity.1 || row_identity.0 != row_identity.2 {
        bail!("rule transaction row lineage is ambiguous");
    }
    let clean_state = conn.query_row(
        &format!(
            "SELECT COUNT(1) FROM fc_transaction_norm t WHERE t.case_id={} AND (\
             t.clean_invalid IS NULL OR t.clean_invalid NOT IN (0,1) OR \
             t.clean_failed IS NULL OR t.clean_failed NOT IN (0,1) OR \
             t.clean_reversal IS NULL OR t.clean_reversal NOT IN (0,1))",
            crate::sql_literal(case_id)
        ),
        [],
        |row| row.get::<_, i64>(0),
    )?;
    if clean_state != 0 {
        bail!("rule transaction cleaning state is unresolved");
    }
    let txn_ts_expr = crate::txn_ts_expr("t", cols);
    let max_ts_expr = if txn_ts_expr != crate::NULL_TIMESTAMP {
        format!("COALESCE(CAST(MAX({txn_ts_expr}) AS VARCHAR), '')")
    } else {
        "''".to_string()
    };
    let max_id_expr = "COALESCE(MAX(TRY_CAST(t.id AS BIGINT)), 0)";
    let mut stmt = conn.prepare(&format!(
        "SELECT COUNT(1), {max_ts_expr}, {max_id_expr} FROM fc_transaction_norm t WHERE t.case_id={}",
        crate::sql_literal(case_id)
    ))?;
    let mut snapshot = stmt.query_row([], |row| {
        Ok(RuleTxnSourceSnapshot {
            case_id: case_id.to_string(),
            row_count: row.get::<_, i64>(0)?,
            max_txn_ts: row.get::<_, String>(1)?,
            max_id: row.get::<_, i64>(2)?,
            source_revision: revision.1,
            source_signature: String::new(),
        })
    })?;
    let mut hasher = Sha256::new();
    hash_framed(&mut hasher, b"analytix.rule-transaction-source/v2");
    hash_framed(&mut hasher, case_id.as_bytes());
    for column in cols {
        hash_framed(&mut hasher, column.as_bytes());
    }
    let mut rows = conn.prepare(&format!(
        "SELECT to_json(t) FROM fc_transaction_norm t WHERE t.case_id={} \
         ORDER BY TRY_CAST(t.id AS BIGINT), to_json(t)",
        crate::sql_literal(case_id)
    ))?;
    let mapped = rows.query_map([], |row| row.get::<_, String>(0))?;
    for row in mapped {
        hash_framed(&mut hasher, row?.as_bytes());
    }
    snapshot.source_signature = format!("{:x}", hasher.finalize());
    Ok(snapshot)
}

fn hash_framed(hasher: &mut Sha256, value: &[u8]) {
    hasher.update((value.len() as u64).to_be_bytes());
    hasher.update(value);
}
