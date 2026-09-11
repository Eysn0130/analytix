use anyhow::Result;
use duckdb::Connection;
use serde::Serialize;

use crate::rule_pattern_features::{
    classify_rule_cash_txn, normalize_rule_direction, CashClass, RulePatternTxnRow,
};

#[derive(Clone, Debug, Serialize)]
pub(crate) struct RulePatternInputCoverage {
    pub(crate) total_rows: i64,
    pub(crate) accepted_rows: i64,
    pub(crate) rejected_rows: i64,
    pub(crate) account_key_covered_rows: i64,
    pub(crate) transaction_id_covered_rows: i64,
    pub(crate) transaction_time_covered_rows: i64,
    pub(crate) amount_covered_rows: i64,
    pub(crate) direction_covered_rows: i64,
    pub(crate) cash_covered_rows: i64,
    pub(crate) cash_unknown_rows: i64,
    pub(crate) cash_conflict_rows: i64,
}

impl RulePatternInputCoverage {
    pub(crate) fn is_complete(&self) -> bool {
        self.total_rows > 0
            && self.accepted_rows == self.total_rows
            && self.rejected_rows == 0
            && self.account_key_covered_rows == self.total_rows
            && self.transaction_id_covered_rows == self.total_rows
            && self.transaction_time_covered_rows == self.total_rows
            && self.amount_covered_rows == self.total_rows
            && self.direction_covered_rows == self.total_rows
    }

    pub(crate) fn blocker(&self) -> String {
        format!(
            "rule_pattern_required_field_coverage_incomplete:total={}:accepted={}:rejected={}:account_key={}:transaction_id={}:transaction_time={}:amount={}:direction={}",
            self.total_rows,
            self.accepted_rows,
            self.rejected_rows,
            self.account_key_covered_rows,
            self.transaction_id_covered_rows,
            self.transaction_time_covered_rows,
            self.amount_covered_rows,
            self.direction_covered_rows,
        )
    }

    pub(crate) fn cash_dependent_ready(&self) -> bool {
        self.total_rows > 0
            && self.cash_covered_rows == self.total_rows
            && self.cash_unknown_rows == 0
            && self.cash_conflict_rows == 0
    }
}

pub(super) struct RulePatternTxnLoad {
    pub(super) rows: Vec<RulePatternTxnRow>,
    pub(super) coverage: RulePatternInputCoverage,
}

pub(super) fn load_rule_pattern_txn_rows(
    conn: &Connection,
    case_id: &str,
) -> Result<RulePatternTxnLoad> {
    let sql = format!(
        "
        SELECT
          account_key,
          txn_id,
          row_id,
          CAST(txn_ts_val AS VARCHAR) AS txn_time,
          date_diff('second', TIMESTAMP '1970-01-01', txn_ts_val) AS txn_epoch,
          CAST(EXTRACT(hour FROM txn_ts_val) AS BIGINT) AS txn_hour,
          amount_val,
          direction_raw,
          cash_raw,
          summary,
          txn_type,
          remark,
          voucher_type
        FROM {}
        WHERE case_id={}
        ORDER BY account_key ASC NULLS LAST, txn_ts_val ASC NULLS LAST,
                 COALESCE(NULLIF(txn_id, ''), NULLIF(row_id, ''), '') ASC
        ",
        crate::RULE_TXN_INDEX_TABLE,
        crate::sql_literal(case_id)
    );
    let mut stmt = conn.prepare(&sql)?;
    let mapped = stmt.query_map([], |row| {
        Ok((
            row.get::<_, Option<String>>(0)?,
            row.get::<_, Option<String>>(1)?,
            row.get::<_, Option<String>>(2)?,
            row.get::<_, Option<String>>(3)?,
            row.get::<_, Option<i64>>(4)?,
            row.get::<_, Option<i64>>(5)?,
            row.get::<_, Option<f64>>(6)?,
            row.get::<_, Option<String>>(7)?,
            row.get::<_, Option<String>>(8)?,
            row.get::<_, Option<String>>(9)?,
            row.get::<_, Option<String>>(10)?,
            row.get::<_, Option<String>>(11)?,
            row.get::<_, Option<String>>(12)?,
        ))
    })?;

    let mut rows = Vec::new();
    let mut coverage = RulePatternInputCoverage {
        total_rows: 0,
        accepted_rows: 0,
        rejected_rows: 0,
        account_key_covered_rows: 0,
        transaction_id_covered_rows: 0,
        transaction_time_covered_rows: 0,
        amount_covered_rows: 0,
        direction_covered_rows: 0,
        cash_covered_rows: 0,
        cash_unknown_rows: 0,
        cash_conflict_rows: 0,
    };
    for item in mapped {
        let (
            account_key,
            txn_id_raw,
            row_id,
            txn_time,
            txn_epoch,
            txn_hour,
            amount,
            direction_raw,
            cash_raw,
            summary,
            txn_type,
            remark,
            voucher_type,
        ) = item?;
        coverage.total_rows += 1;

        let account_key = account_key.unwrap_or_default().trim().to_string();
        let txn_id_raw = txn_id_raw.unwrap_or_default().trim().to_string();
        let row_id = row_id.unwrap_or_default().trim().to_string();
        let txn_id = if valid_source_identifier(&txn_id_raw) {
            txn_id_raw
        } else if valid_source_identifier(&row_id) {
            format!("row_{row_id}")
        } else {
            String::new()
        };
        let txn_time = txn_time.unwrap_or_default().trim().to_string();
        let direction = normalize_rule_direction(direction_raw.as_deref().unwrap_or(""));
        let normalized_amount = amount
            .filter(|value| value.is_finite())
            .map(|value| crate::round2(value.abs()))
            .filter(|value| value.is_finite());
        let account_key_covered = valid_source_identifier(&account_key);
        let transaction_id_covered = !txn_id.is_empty();
        let transaction_time_covered =
            !txn_time.is_empty() && txn_epoch.is_some() && txn_hour.is_some();
        let amount_covered = normalized_amount.is_some();
        let direction_covered = direction == "in" || direction == "out";
        let cash_class = classify_rule_cash_txn(
            cash_raw.as_deref().unwrap_or(""),
            &[
                summary.as_deref().unwrap_or(""),
                txn_type.as_deref().unwrap_or(""),
                remark.as_deref().unwrap_or(""),
                voucher_type.as_deref().unwrap_or(""),
            ],
        );

        coverage.account_key_covered_rows += i64::from(account_key_covered);
        coverage.transaction_id_covered_rows += i64::from(transaction_id_covered);
        coverage.transaction_time_covered_rows += i64::from(transaction_time_covered);
        coverage.amount_covered_rows += i64::from(amount_covered);
        coverage.direction_covered_rows += i64::from(direction_covered);
        coverage.cash_covered_rows += i64::from(cash_class.is_known());
        coverage.cash_unknown_rows += i64::from(cash_class == CashClass::Unknown);
        coverage.cash_conflict_rows += i64::from(cash_class == CashClass::Conflict);

        if !(account_key_covered
            && transaction_id_covered
            && transaction_time_covered
            && amount_covered
            && direction_covered)
        {
            coverage.rejected_rows += 1;
            continue;
        }

        rows.push(RulePatternTxnRow {
            account_key,
            txn_id,
            txn_time,
            txn_epoch: txn_epoch.expect("timestamp coverage checked"),
            txn_hour: txn_hour.expect("timestamp coverage checked"),
            amount: normalized_amount.expect("amount coverage checked"),
            direction,
            cash_class,
        });
        coverage.accepted_rows += 1;
    }
    Ok(RulePatternTxnLoad { rows, coverage })
}

fn valid_source_identifier(value: &str) -> bool {
    let normalized = value.trim().to_ascii_lowercase();
    !normalized.is_empty()
        && !matches!(
            normalized.as_str(),
            "unknown" | "null" | "none" | "na" | "n/a" | "-" | "--" | "无" | "未知" | "查无信息"
        )
}
