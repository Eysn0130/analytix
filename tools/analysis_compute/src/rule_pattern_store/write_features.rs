use anyhow::Result;
use duckdb::Connection;

use crate::rule_pattern_args::RulePatternIndexArgs;
use crate::rule_pattern_features::RulePatternFeature;

pub(super) fn create_rule_pattern_staging(conn: &Connection) -> Result<()> {
    conn.execute_batch(&format!(
        "
        DROP TABLE IF EXISTS {};
        CREATE TABLE {}(
          case_id TEXT,
          param_signature TEXT,
          feature_code TEXT,
          account_key TEXT,
          direction TEXT,
          txn_count BIGINT,
          total_amount DOUBLE,
          txn_ids_json TEXT,
          amounts_json TEXT,
          detail_json TEXT,
          first_time TEXT,
          last_time TEXT,
          round_unit DOUBLE,
          min_amount DOUBLE,
          min_count BIGINT,
          min_total_amount DOUBLE,
          start_hour BIGINT,
          end_hour BIGINT
        );
        ",
        crate::RULE_PATTERN_STAGING_TABLE,
        crate::RULE_PATTERN_STAGING_TABLE
    ))?;
    Ok(())
}

pub(super) fn insert_rule_pattern_feature(
    conn: &Connection,
    args: &RulePatternIndexArgs,
    feature: &RulePatternFeature,
) -> Result<()> {
    let round_unit_sql = feature
        .round_unit
        .map(|value| value.to_string())
        .unwrap_or_else(|| "NULL".to_string());
    let start_hour_sql = feature
        .start_hour
        .map(|value| value.to_string())
        .unwrap_or_else(|| "NULL".to_string());
    let end_hour_sql = feature
        .end_hour
        .map(|value| value.to_string())
        .unwrap_or_else(|| "NULL".to_string());
    conn.execute_batch(&format!(
        "
        INSERT INTO {}(
          case_id, param_signature, feature_code, account_key, direction,
          txn_count, total_amount, txn_ids_json, amounts_json, detail_json, first_time, last_time,
          round_unit, min_amount, min_count, min_total_amount, start_hour, end_hour
        ) VALUES (
          {}, {}, {}, {}, {},
          {}, {}, {}, {}, {}, {}, {},
          {}, {}, {}, {}, {}, {}
        )
        ",
        crate::RULE_PATTERN_STAGING_TABLE,
        crate::sql_literal(&args.case_id),
        crate::sql_literal(&args.param_signature),
        crate::sql_literal(feature.feature_code),
        crate::sql_literal(&feature.account_key),
        crate::sql_literal(&feature.direction),
        feature.txn_count,
        feature.total_amount,
        crate::sql_literal(&feature.txn_ids_json),
        crate::sql_literal(&feature.amounts_json),
        crate::sql_literal(&feature.detail_json),
        crate::sql_literal(&feature.first_time),
        crate::sql_literal(&feature.last_time),
        round_unit_sql,
        feature.min_amount,
        feature.min_count,
        feature.min_total_amount,
        start_hour_sql,
        end_hour_sql
    ))?;
    Ok(())
}

pub(super) fn swap_rule_pattern_index(conn: &Connection) -> Result<()> {
    conn.execute_batch(&format!(
        "
        DROP TABLE IF EXISTS {};
        ALTER TABLE {} RENAME TO {};
        CREATE INDEX IF NOT EXISTS idx_{}_case_sig ON {}(case_id, param_signature);
        CREATE INDEX IF NOT EXISTS idx_{}_case_feature ON {}(case_id, feature_code, account_key);
        ",
        crate::RULE_PATTERN_TABLE,
        crate::RULE_PATTERN_STAGING_TABLE,
        crate::RULE_PATTERN_TABLE,
        crate::RULE_PATTERN_TABLE,
        crate::RULE_PATTERN_TABLE,
        crate::RULE_PATTERN_TABLE,
        crate::RULE_PATTERN_TABLE
    ))?;
    Ok(())
}
