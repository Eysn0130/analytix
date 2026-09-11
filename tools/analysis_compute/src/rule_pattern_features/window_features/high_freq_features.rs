use anyhow::Result;
use serde_json::json;

use crate::round2;
use crate::rule_pattern_args::RulePatternIndexArgs;

use super::super::model::{RulePatternFeature, RulePatternTxnRow};

pub(crate) fn compute_high_freq_small_out_features(
    args: &RulePatternIndexArgs,
    account_key: &str,
    rows: &[&RulePatternTxnRow],
) -> Result<Vec<RulePatternFeature>> {
    let small_out_rows = rows
        .iter()
        .copied()
        .filter(|row| row.direction == "out" && row.amount <= args.high_freq_small_amount_threshold)
        .collect::<Vec<_>>();
    let window_seconds = args.high_freq_window_minutes.max(1) * 60;
    let mut features = Vec::new();
    let mut left = 0_usize;
    while left < small_out_rows.len() {
        let mut right = left;
        let window_start_epoch = small_out_rows[left].txn_epoch;
        while right < small_out_rows.len()
            && small_out_rows[right].txn_epoch <= window_start_epoch + window_seconds
        {
            right += 1;
        }
        let window_rows = &small_out_rows[left..right];
        if window_rows.len() >= args.high_freq_count_threshold as usize {
            let total_amount = round2(window_rows.iter().map(|row| row.amount).sum::<f64>());
            if total_amount < args.high_freq_min_total_amount {
                left += 1;
                continue;
            }
            let txn_ids = window_rows
                .iter()
                .map(|row| row.txn_id.clone())
                .filter(|item| !item.trim().is_empty())
                .collect::<Vec<_>>();
            let amounts = window_rows.iter().map(|row| row.amount).collect::<Vec<_>>();
            let detail = json!({
                "small_amount_threshold": args.high_freq_small_amount_threshold,
                "min_total_amount": args.high_freq_min_total_amount,
                "total_out_amount": total_amount,
            });
            features.push(RulePatternFeature {
                feature_code: "HIGH_FREQ_SMALL_OUT",
                account_key: account_key.to_string(),
                direction: "out".to_string(),
                txn_count: window_rows.len() as i64,
                total_amount,
                txn_ids_json: serde_json::to_string(&txn_ids)?,
                amounts_json: serde_json::to_string(&amounts)?,
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
                min_amount: 0.0,
                min_count: args.high_freq_count_threshold,
                min_total_amount: args.high_freq_min_total_amount,
                start_hour: None,
                end_hour: None,
            });
            left = right;
            continue;
        }
        left += 1;
    }
    Ok(features)
}
