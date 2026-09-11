use anyhow::Result;
use duckdb::Connection;

#[derive(Debug)]
pub(super) struct RulePatternSourceSnapshot {
    pub(super) case_id: String,
    pub(super) source_revision: i64,
    pub(super) row_count: i64,
    pub(super) max_txn_ts: String,
    pub(super) max_id: i64,
    pub(super) source_signature: String,
}

pub(super) fn rule_pattern_source_snapshot(
    conn: &Connection,
    case_id: &str,
) -> Result<RulePatternSourceSnapshot> {
    let source_meta = conn.query_row(
        &format!(
            "SELECT agg_version, COALESCE(case_id, ''), source_revision, \
                    COALESCE(result_signature, ''), row_count \
               FROM {} WHERE agg_name={}",
            crate::META_TABLE,
            crate::sql_literal(crate::RULE_TXN_INDEX_CACHE_KEY)
        ),
        [],
        |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, String>(1)?,
                row.get::<_, i64>(2)?,
                row.get::<_, String>(3)?,
                row.get::<_, i64>(4)?,
            ))
        },
    )?;
    if source_meta.0 != crate::RULE_TXN_INDEX_VERSION
        || source_meta.1 != case_id
        || source_meta.2 <= 0
        || !crate::is_sha256_hex(&source_meta.3)
    {
        anyhow::bail!("rule pattern source authority is unavailable");
    }
    let actual_source_signature = crate::canonical_case_table_signature(
        conn,
        crate::RULE_TXN_INDEX_TABLE,
        case_id,
        "analytix.rule-transaction-index-result/v2",
        "TRY_CAST(r.row_id AS BIGINT), r.row_id, r.account_key, r.txn_id",
    )?;
    if actual_source_signature != source_meta.3 {
        anyhow::bail!("rule pattern source result integrity is unavailable");
    }
    let mut stmt = conn.prepare(&format!(
        "SELECT COUNT(1), COALESCE(CAST(MAX(txn_ts_val) AS VARCHAR), ''), COALESCE(MAX(TRY_CAST(row_id AS BIGINT)), 0) \
         FROM {} WHERE case_id={}",
        crate::RULE_TXN_INDEX_TABLE,
        crate::sql_literal(case_id)
    ))?;
    let snapshot = stmt.query_row([], |row| {
        Ok(RulePatternSourceSnapshot {
            case_id: case_id.to_string(),
            source_revision: source_meta.2,
            row_count: row.get::<_, i64>(0)?,
            max_txn_ts: row.get::<_, String>(1)?,
            max_id: row.get::<_, i64>(2)?,
            source_signature: source_meta.3.clone(),
        })
    })?;
    if snapshot.row_count != source_meta.4 {
        anyhow::bail!("rule pattern source row count is inconsistent");
    }
    Ok(snapshot)
}

pub(super) fn rule_pattern_index_is_current(
    conn: &Connection,
    source_snapshot: &RulePatternSourceSnapshot,
    param_signature: &str,
) -> Result<bool> {
    if !crate::table_exists(conn, crate::RULE_PATTERN_TABLE)? {
        return Ok(false);
    }
    if !crate::table_exists(conn, crate::META_TABLE)? {
        return Ok(false);
    }
    let mut stmt = conn.prepare(&format!(
        "SELECT agg_version, COALESCE(case_id, ''), source_revision, source_row_count, \
                COALESCE(CAST(source_max_txn_ts AS VARCHAR), ''), source_max_id, \
                COALESCE(source_signature, ''), COALESCE(source_parameter_signature, ''), \
                COALESCE(result_signature, ''), row_count \
         FROM {} WHERE agg_name={}",
        crate::META_TABLE,
        crate::sql_literal(crate::RULE_PATTERN_CACHE_KEY)
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
            row.get::<_, String>(8)?,
            row.get::<_, i64>(9)?,
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
        saved_parameter_signature,
        saved_result_signature,
        saved_row_count,
    )) = row
    else {
        return Ok(false);
    };
    let actual_row_count = conn.query_row(
        &format!(
            "SELECT COUNT(1) FROM {} WHERE case_id={}",
            crate::RULE_PATTERN_TABLE,
            crate::sql_literal(&source_snapshot.case_id)
        ),
        [],
        |row| row.get::<_, i64>(0),
    )?;
    let actual_result_signature = crate::canonical_case_table_signature(
        conn,
        crate::RULE_PATTERN_TABLE,
        &source_snapshot.case_id,
        "analytix.rule-pattern-index-result/v9",
        "r.param_signature, r.feature_code, r.account_key, r.direction, r.first_time, r.last_time",
    );
    let Ok(actual_result_signature) = actual_result_signature else {
        return Ok(false);
    };
    Ok(saved_version == crate::RULE_PATTERN_VERSION
        && saved_case_id == source_snapshot.case_id
        && saved_revision == source_snapshot.source_revision
        && saved_count == source_snapshot.row_count
        && saved_ts == source_snapshot.max_txn_ts.as_str()
        && saved_id == source_snapshot.max_id
        && saved_source_signature == source_snapshot.source_signature
        && saved_parameter_signature == param_signature
        && crate::is_sha256_hex(&saved_result_signature)
        && saved_result_signature == actual_result_signature
        && saved_row_count >= 0
        && saved_row_count == actual_row_count)
}

pub(super) fn upsert_rule_pattern_index_meta(
    conn: &Connection,
    source_snapshot: &RulePatternSourceSnapshot,
    param_signature: &str,
    row_count: i64,
    result_signature: &str,
) -> Result<()> {
    if !crate::is_sha256_hex(result_signature) {
        anyhow::bail!("rule pattern result signature is invalid");
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
            source_max_txn_ts, source_max_id, source_signature,
            source_parameter_signature, result_signature, built_at, row_count
        )
        VALUES ({}, {}, {}, {}, {}, {max_ts_sql}, {}, {}, {}, {}, now(), {row_count})
        ON CONFLICT(agg_name) DO UPDATE
        SET agg_version=excluded.agg_version,
            case_id=excluded.case_id,
            source_revision=excluded.source_revision,
            source_row_count=excluded.source_row_count,
            source_max_txn_ts=excluded.source_max_txn_ts,
            source_max_id=excluded.source_max_id,
            source_signature=excluded.source_signature,
            source_parameter_signature=excluded.source_parameter_signature,
            result_signature=excluded.result_signature,
            built_at=excluded.built_at,
            row_count=excluded.row_count
        ",
        crate::META_TABLE,
        crate::sql_literal(crate::RULE_PATTERN_CACHE_KEY),
        crate::RULE_PATTERN_VERSION,
        crate::sql_literal(&source_snapshot.case_id),
        source_snapshot.source_revision,
        source_snapshot.row_count,
        source_snapshot.max_id,
        crate::sql_literal(&source_snapshot.source_signature),
        crate::sql_literal(param_signature),
        crate::sql_literal(result_signature)
    ))?;
    Ok(())
}

pub(super) fn retire_legacy_rule_pattern_index_meta(conn: &Connection) -> Result<()> {
    conn.execute_batch(&format!(
        "DELETE FROM {} WHERE agg_name='rule_pattern_index:v8'",
        crate::META_TABLE
    ))?;
    Ok(())
}
