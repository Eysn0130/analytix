use anyhow::Result;
use serde_json::json;
use std::collections::HashMap;

use crate::round2;
use crate::rule_pattern_args::RulePatternIndexArgs;

use super::super::model::{RulePatternFeature, RulePatternTxnRow};

pub(crate) fn compute_repeated_amount_features(
    args: &RulePatternIndexArgs,
    account_key: &str,
    rows: &[&RulePatternTxnRow],
) -> Result<Vec<RulePatternFeature>> {
    let window_seconds = args.repeated_amount_window_minutes.max(1) * 60;
    let mut group_order: Vec<(String, i64)> = Vec::new();
    let mut groups: HashMap<(String, i64), Vec<&RulePatternTxnRow>> = HashMap::new();
    for row in rows {
        if !(row.direction == "in" || row.direction == "out") {
            continue;
        }
        if row.amount < args.repeated_amount_min_amount {
            continue;
        }
        let amount_cents = (round2(row.amount) * 100.0).round() as i64;
        let key = (row.direction.clone(), amount_cents);
        if !groups.contains_key(&key) {
            group_order.push(key.clone());
        }
        groups.entry(key).or_default().push(*row);
    }

    let mut features = Vec::new();
    for (direction, amount_cents) in group_order {
        let Some(group_rows) = groups.get(&(direction.clone(), amount_cents)) else {
            continue;
        };
        if group_rows.len() < args.repeated_amount_min_count as usize {
            continue;
        }
        let amount_bucket = amount_cents as f64 / 100.0;
        let mut start = 0_usize;
        while start < group_rows.len() {
            let mut end = start;
            let window_start_epoch = group_rows[start].txn_epoch;
            while end < group_rows.len()
                && group_rows[end].txn_epoch <= window_start_epoch + window_seconds
            {
                end += 1;
            }
            let window_rows = &group_rows[start..end];
            let total_amount = round2(window_rows.iter().map(|row| row.amount).sum::<f64>());
            if window_rows.len() >= args.repeated_amount_min_count as usize
                && total_amount >= args.repeated_amount_min_total_amount
            {
                let txn_ids = window_rows
                    .iter()
                    .map(|row| row.txn_id.clone())
                    .filter(|item| !item.trim().is_empty())
                    .collect::<Vec<_>>();
                let detail = json!({
                    "amount": amount_bucket,
                });
                features.push(RulePatternFeature {
                    feature_code: "REPEATED_AMOUNT_PATTERN",
                    account_key: account_key.to_string(),
                    direction: direction.clone(),
                    txn_count: window_rows.len() as i64,
                    total_amount,
                    txn_ids_json: serde_json::to_string(&txn_ids)?,
                    amounts_json: serde_json::to_string(&vec![amount_bucket])?,
                    detail_json: serde_json::to_string(&detail)?,
                    first_time: window_rows
                        .first()
                        .map(|row| row.txn_time.clone())
                        .unwrap_or_default(),
                    last_time: window_rows
                        .last()
                        .map(|row| row.txn_time.clone())
                        .unwrap_or_default(),
                    round_unit: None,
                    min_amount: args.repeated_amount_min_amount,
                    min_count: args.repeated_amount_min_count,
                    min_total_amount: args.repeated_amount_min_total_amount,
                    start_hour: None,
                    end_hour: None,
                });
                break;
            }
            start += 1;
        }
    }
    Ok(features)
}
