use anyhow::Result;
use std::collections::HashMap;

use crate::round2;
use crate::rule_pattern_args::RulePatternIndexArgs;

use super::model::{RulePatternFeature, RulePatternTxnRow};
use super::normalization::is_night_or_offhour;

pub(super) fn compute_round_amount_features(
    args: &RulePatternIndexArgs,
    account_key: &str,
    rows: &[&RulePatternTxnRow],
) -> Result<Vec<RulePatternFeature>> {
    let mut by_direction: HashMap<&str, Vec<&RulePatternTxnRow>> = HashMap::new();
    for row in rows {
        if row.amount >= args.round_min_amount
            && row.amount.rem_euclid(args.round_unit) < 0.01
            && (row.direction == "in" || row.direction == "out")
        {
            by_direction
                .entry(row.direction.as_str())
                .or_default()
                .push(row);
        }
    }

    let mut features = Vec::new();
    for (direction, direction_rows) in by_direction {
        let total_amount = round2(direction_rows.iter().map(|row| row.amount).sum::<f64>());
        if direction_rows.len() < args.round_min_count as usize
            || total_amount < args.round_min_total_amount
        {
            continue;
        }
        let txn_ids = direction_rows
            .iter()
            .take(20)
            .map(|row| row.txn_id.clone())
            .filter(|item| !item.trim().is_empty())
            .collect::<Vec<_>>();
        let mut amounts = direction_rows
            .iter()
            .map(|row| row.amount)
            .collect::<Vec<_>>();
        amounts.sort_by(|left, right| left.partial_cmp(right).unwrap_or(std::cmp::Ordering::Equal));
        amounts.dedup_by(|left, right| (*left - *right).abs() < 0.0000001);
        amounts.truncate(20);
        features.push(RulePatternFeature {
            feature_code: "ROUND_AMOUNT_PATTERN",
            account_key: account_key.to_string(),
            direction: direction.to_string(),
            txn_count: direction_rows.len() as i64,
            total_amount,
            txn_ids_json: serde_json::to_string(&txn_ids)?,
            amounts_json: serde_json::to_string(&amounts)?,
            detail_json: "{}".to_string(),
            first_time: direction_rows
                .first()
                .map(|row| row.txn_time.clone())
                .unwrap_or_default(),
            last_time: direction_rows
                .last()
                .map(|row| row.txn_time.clone())
                .unwrap_or_default(),
            round_unit: Some(args.round_unit),
            min_amount: args.round_min_amount,
            min_count: args.round_min_count,
            min_total_amount: args.round_min_total_amount,
            start_hour: None,
            end_hour: None,
        });
    }
    Ok(features)
}

pub(super) fn compute_night_activity_feature(
    args: &RulePatternIndexArgs,
    account_key: &str,
    rows: &[&RulePatternTxnRow],
) -> Result<Option<RulePatternFeature>> {
    let night_rows = rows
        .iter()
        .copied()
        .filter(|row| is_night_or_offhour(row.txn_hour, args.night_start_hour, args.night_end_hour))
        .collect::<Vec<_>>();
    let total_amount = round2(night_rows.iter().map(|row| row.amount).sum::<f64>());
    if night_rows.len() < args.night_min_count as usize
        || total_amount < args.night_min_total_amount
    {
        return Ok(None);
    }
    let txn_ids = night_rows
        .iter()
        .take(20)
        .map(|row| row.txn_id.clone())
        .filter(|item| !item.trim().is_empty())
        .collect::<Vec<_>>();
    Ok(Some(RulePatternFeature {
        feature_code: "NIGHT_OFFHOUR_ACTIVITY",
        account_key: account_key.to_string(),
        direction: String::new(),
        txn_count: night_rows.len() as i64,
        total_amount,
        txn_ids_json: serde_json::to_string(&txn_ids)?,
        amounts_json: "[]".to_string(),
        detail_json: "{}".to_string(),
        first_time: night_rows
            .first()
            .map(|row| row.txn_time.clone())
            .unwrap_or_default(),
        last_time: night_rows
            .last()
            .map(|row| row.txn_time.clone())
            .unwrap_or_default(),
        round_unit: None,
        min_amount: 0.0,
        min_count: args.night_min_count,
        min_total_amount: args.night_min_total_amount,
        start_hour: Some(args.night_start_hour),
        end_hour: Some(args.night_end_hour),
    }))
}
