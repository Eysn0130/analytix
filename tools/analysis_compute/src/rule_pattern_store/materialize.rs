use anyhow::{Context, Result};
use duckdb::Connection;
use serde_json::{json, Value};

use crate::rule_pattern_args::RulePatternIndexArgs;
use crate::rule_pattern_features::compute_rule_pattern_features;

use super::load_rows::{load_rule_pattern_txn_rows, RulePatternInputCoverage};
use super::meta::{
    retire_legacy_rule_pattern_index_meta, rule_pattern_index_is_current,
    rule_pattern_source_snapshot, upsert_rule_pattern_index_meta,
};
use super::write_features::{
    create_rule_pattern_staging, insert_rule_pattern_feature, swap_rule_pattern_index,
};

#[derive(Debug)]
pub(crate) struct RulePatternIndexResult {
    pub(crate) row_count: i64,
    pub(crate) rebuilt: bool,
    pub(crate) input_coverage: RulePatternInputCoverage,
    pub(crate) feature_readiness: Value,
}

fn feature_readiness(coverage: &RulePatternInputCoverage) -> Value {
    json!({
        "cash_dependent_rules": {
            "status": if coverage.cash_dependent_ready() { "complete" } else { "partial" },
            "requested_rows": coverage.total_rows,
            "eligible_rows": coverage.cash_covered_rows,
            "unknown_cash_rows": coverage.cash_unknown_rows,
            "conflict_cash_rows": coverage.cash_conflict_rows,
            "blocker": if coverage.cash_dependent_ready() {
                Value::Null
            } else {
                Value::String("cash_classification_coverage_incomplete".to_string())
            },
        },
        "cash_independent_rules": {
            "status": "complete",
            "requested_rows": coverage.total_rows,
            "eligible_rows": coverage.accepted_rows,
            "blocker": Value::Null,
        },
    })
}

pub(crate) fn materialize_rule_pattern_index(
    args: &RulePatternIndexArgs,
) -> Result<RulePatternIndexResult> {
    let conn = Connection::open(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    crate::ensure_required_table(&conn, crate::RULE_TXN_INDEX_TABLE)?;
    crate::ensure_meta_table(&conn)?;

    let source_snapshot = rule_pattern_source_snapshot(&conn, &args.case_id)?;
    let loaded = load_rule_pattern_txn_rows(&conn, &args.case_id)?;
    if loaded.coverage.total_rows != source_snapshot.row_count || !loaded.coverage.is_complete() {
        anyhow::bail!(loaded.coverage.blocker());
    }
    if !args.force && rule_pattern_index_is_current(&conn, &source_snapshot, &args.param_signature)?
    {
        let row_count = conn.query_row(
            &format!(
                "SELECT COUNT(1) FROM {} WHERE case_id={}",
                crate::RULE_PATTERN_TABLE,
                crate::sql_literal(&args.case_id)
            ),
            [],
            |row| row.get::<_, i64>(0),
        )?;
        return Ok(RulePatternIndexResult {
            row_count,
            rebuilt: false,
            feature_readiness: feature_readiness(&loaded.coverage),
            input_coverage: loaded.coverage,
        });
    }

    let features = compute_rule_pattern_features(args, &loaded.rows)?;
    create_rule_pattern_staging(&conn)?;
    for feature in &features {
        insert_rule_pattern_feature(&conn, args, feature)?;
    }
    let row_count = conn.query_row(
        &format!(
            "SELECT COUNT(1) FROM {} WHERE case_id={}",
            crate::RULE_PATTERN_STAGING_TABLE,
            crate::sql_literal(&args.case_id)
        ),
        [],
        |row| row.get::<_, i64>(0),
    )?;
    let result_signature = crate::canonical_case_table_signature(
        &conn,
        crate::RULE_PATTERN_STAGING_TABLE,
        &args.case_id,
        "analytix.rule-pattern-index-result/v9",
        "r.param_signature, r.feature_code, r.account_key, r.direction, r.first_time, r.last_time",
    )?;
    conn.execute_batch("BEGIN TRANSACTION")?;
    let publish_result = (|| -> Result<()> {
        swap_rule_pattern_index(&conn)?;
        upsert_rule_pattern_index_meta(
            &conn,
            &source_snapshot,
            &args.param_signature,
            row_count,
            &result_signature,
        )?;
        retire_legacy_rule_pattern_index_meta(&conn)?;
        Ok(())
    })();
    if let Err(error) = publish_result {
        let _ = conn.execute_batch("ROLLBACK");
        return Err(error);
    }
    if let Err(error) = conn.execute_batch("COMMIT") {
        let _ = conn.execute_batch("ROLLBACK");
        return Err(error.into());
    }
    Ok(RulePatternIndexResult {
        row_count,
        rebuilt: true,
        feature_readiness: feature_readiness(&loaded.coverage),
        input_coverage: loaded.coverage,
    })
}
