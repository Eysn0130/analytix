use anyhow::Result;
use serde_json::json;
use std::collections::HashMap;

use crate::round2;
use crate::rule_pattern_args::RulePatternIndexArgs;

use super::super::model::{RulePatternFeature, RulePatternTxnRow};

pub(crate) fn compute_near_threshold_features(
    args: &RulePatternIndexArgs,
    account_key: &str,
    rows: &[&RulePatternTxnRow],
) -> Result<Vec<RulePatternFeature>> {
    if args.near_threshold_amount <= 0.0 {
        return Ok(Vec::new());
    }
    let threshold = args.near_threshold_amount;
    let lower_rate = args.near_threshold_lower_rate.clamp(0.0, 0.99);
    let lower_amount = threshold * lower_rate;
    let window_seconds = args.near_threshold_window_minutes.max(1) * 60;

    let mut direction_order: Vec<String> = Vec::new();
    let mut by_direction: HashMap<String, Vec<&RulePatternTxnRow>> = HashMap::new();
    for row in rows {
        if !(row.direction == "in" || row.direction == "out") {
            continue;
        }
        if row.amount < lower_amount || row.amount >= threshold {
            continue;
        }
        if !by_direction.contains_key(&row.direction) {
            direction_order.push(row.direction.clone());
        }
        by_direction
            .entry(row.direction.clone())
            .or_default()
            .push(*row);
    }

    let mut features = Vec::new();
    for direction in direction_order {
        let Some(direction_rows) = by_direction.get(direction.as_str()) else {
            continue;
        };
        let mut start = 0_usize;
        while start < direction_rows.len() {
            let mut end = start;
            let window_start_epoch = direction_rows[start].txn_epoch;
            while end < direction_rows.len()
                && direction_rows[end].txn_epoch <= window_start_epoch + window_seconds
            {
                end += 1;
            }
            let window_rows = &direction_rows[start..end];
            if window_rows.len() >= args.near_threshold_min_count as usize {
                let total_amount = round2(window_rows.iter().map(|row| row.amount).sum::<f64>());
                if total_amount < args.near_threshold_min_total_amount {
                    start += 1;
                    continue;
                }
                let txn_ids = window_rows
                    .iter()
                    .map(|row| row.txn_id.clone())
                    .filter(|item| !item.trim().is_empty())
                    .collect::<Vec<_>>();
                let amounts = window_rows.iter().map(|row| row.amount).collect::<Vec<_>>();
                let amount_min = amounts
                    .iter()
                    .copied()
                    .fold(f64::INFINITY, |acc, value| acc.min(value));
                let amount_max = amounts
                    .iter()
                    .copied()
                    .fold(f64::NEG_INFINITY, |acc, value| acc.max(value));
                let detail = json!({
                    "threshold": threshold,
                    "lower_rate": lower_rate,
                    "lower_amount": round2(lower_amount),
                    "amount_min": round2(amount_min),
                    "amount_max": round2(amount_max),
                });
                features.push(RulePatternFeature {
                    feature_code: "NEAR_THRESHOLD_STRUCTURING",
                    account_key: account_key.to_string(),
                    direction: direction.clone(),
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
                    min_amount: lower_amount,
                    min_count: args.near_threshold_min_count,
                    min_total_amount: args.near_threshold_min_total_amount,
                    start_hour: None,
                    end_hour: None,
                });
                start = end;
                continue;
            }
            start += 1;
        }
    }
    Ok(features)
}

pub(crate) fn compute_threshold_split_features(
    args: &RulePatternIndexArgs,
    account_key: &str,
    rows: &[&RulePatternTxnRow],
) -> Result<Vec<RulePatternFeature>> {
    let threshold = args.threshold_split_amount;
    let tolerance_rate = args.threshold_split_tolerance_rate.max(0.0);
    let lower = threshold * (1.0 - tolerance_rate);
    let upper = threshold * (1.0 + tolerance_rate);
    let window_seconds = args.threshold_split_window_minutes.max(1) * 60;

    let mut direction_order: Vec<String> = Vec::new();
    let mut by_direction: HashMap<String, Vec<&RulePatternTxnRow>> = HashMap::new();
    for row in rows {
        if !(row.direction == "in" || row.direction == "out") {
            continue;
        }
        if row.amount < lower || row.amount > upper {
            continue;
        }
        if !by_direction.contains_key(&row.direction) {
            direction_order.push(row.direction.clone());
        }
        by_direction
            .entry(row.direction.clone())
            .or_default()
            .push(*row);
    }

    let mut features = Vec::new();
    for direction in direction_order {
        let Some(direction_rows) = by_direction.get(direction.as_str()) else {
            continue;
        };
        let mut start = 0_usize;
        while start < direction_rows.len() {
            let mut end = start;
            let window_start_epoch = direction_rows[start].txn_epoch;
            while end < direction_rows.len()
                && direction_rows[end].txn_epoch <= window_start_epoch + window_seconds
            {
                end += 1;
            }
            let window_rows = &direction_rows[start..end];
            if window_rows.len() >= args.threshold_split_min_count as usize {
                let txn_ids = window_rows
                    .iter()
                    .map(|row| row.txn_id.clone())
                    .filter(|item| !item.trim().is_empty())
                    .collect::<Vec<_>>();
                let amounts = window_rows.iter().map(|row| row.amount).collect::<Vec<_>>();
                let amount_min = amounts
                    .iter()
                    .copied()
                    .fold(f64::INFINITY, |acc, value| acc.min(value));
                let amount_max = amounts
                    .iter()
                    .copied()
                    .fold(f64::NEG_INFINITY, |acc, value| acc.max(value));
                let total_amount = round2(window_rows.iter().map(|row| row.amount).sum::<f64>());
                let detail = json!({
                    "threshold": threshold,
                    "tolerance_rate": tolerance_rate,
                    "amount_min": round2(amount_min),
                    "amount_max": round2(amount_max),
                });
                features.push(RulePatternFeature {
                    feature_code: "THRESHOLD_SPLIT",
                    account_key: account_key.to_string(),
                    direction: direction.clone(),
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
                    min_amount: lower,
                    min_count: args.threshold_split_min_count,
                    min_total_amount: 0.0,
                    start_hour: None,
                    end_hour: None,
                });
                start = end;
                continue;
            }
            start += 1;
        }
    }
    Ok(features)
}
