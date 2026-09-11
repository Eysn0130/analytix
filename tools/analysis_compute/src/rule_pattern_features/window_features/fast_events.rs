use anyhow::{anyhow, Result};
use serde_json::json;

use crate::round2;
use crate::rule_pattern_args::RulePatternIndexArgs;

use super::super::model::{CashClass, RulePatternFeature, RulePatternTxnRow};
use super::super::normalization::round4;

pub(crate) fn compute_small_fast_event_features(
    args: &RulePatternIndexArgs,
    account_key: &str,
    rows: &[&RulePatternTxnRow],
) -> Result<Vec<RulePatternFeature>> {
    let in_rows = rows
        .iter()
        .copied()
        .filter(|row| row.direction == "in" && row.cash_class == CashClass::NonCash)
        .collect::<Vec<_>>();
    let out_rows = rows
        .iter()
        .copied()
        .filter(|row| row.direction == "out")
        .collect::<Vec<_>>();
    let window_seconds = args.small_fast_window_minutes * 60;
    let mut features = Vec::new();
    for (index, in_row) in in_rows.iter().enumerate() {
        let in_amount = in_row.amount;
        if in_amount < args.small_fast_min_amount || in_amount >= args.small_fast_max_amount {
            continue;
        }
        let next_in_epoch = in_rows.get(index + 1).map(|row| row.txn_epoch);
        let mut window_end = in_row.txn_epoch + window_seconds;
        if let Some(next_epoch) = next_in_epoch {
            if next_epoch < window_end {
                window_end = next_epoch;
            }
        }
        let matching_outs = out_rows
            .iter()
            .copied()
            .filter(|row| {
                row.txn_epoch > in_row.txn_epoch
                    && row.txn_epoch <= window_end
                    && row.amount >= args.small_fast_min_amount
                    && row.amount < args.small_fast_max_amount
                    && row.cash_class == CashClass::NonCash
            })
            .collect::<Vec<_>>();
        if matching_outs.is_empty() {
            continue;
        }
        let out_amount = round2(matching_outs.iter().map(|row| row.amount).sum::<f64>());
        let out_ratio = out_amount / in_amount.max(0.01);
        if out_ratio < args.small_fast_ratio {
            continue;
        }
        let last_out = matching_outs
            .last()
            .ok_or_else(|| anyhow!("small fast matching outs unexpectedly empty"))?;
        let out_txn_ids = matching_outs
            .iter()
            .map(|row| row.txn_id.clone())
            .filter(|item| !item.trim().is_empty())
            .collect::<Vec<_>>();
        let mut txn_ids = Vec::new();
        if !in_row.txn_id.trim().is_empty() {
            txn_ids.push(in_row.txn_id.clone());
        }
        txn_ids.extend(out_txn_ids.iter().cloned());
        let duration_minutes = round2((last_out.txn_epoch - in_row.txn_epoch) as f64 / 60.0);
        let detail = json!({
            "in_txn_id": in_row.txn_id,
            "out_txn_ids": out_txn_ids,
            "in_amount": round2(in_amount),
            "out_amount": out_amount,
            "out_ratio": round4(out_ratio),
            "duration_minutes": duration_minutes,
            "start_time": in_row.txn_time,
            "end_time": last_out.txn_time,
        });
        features.push(RulePatternFeature {
            feature_code: "SMALL_FAST_EVENT",
            account_key: account_key.to_string(),
            direction: String::new(),
            txn_count: txn_ids.len() as i64,
            total_amount: round2(in_amount),
            txn_ids_json: serde_json::to_string(&txn_ids)?,
            amounts_json: "[]".to_string(),
            detail_json: serde_json::to_string(&detail)?,
            first_time: in_row.txn_time.clone(),
            last_time: last_out.txn_time.clone(),
            round_unit: None,
            min_amount: args.small_fast_min_amount,
            min_count: 1,
            min_total_amount: 0.0,
            start_hour: None,
            end_hour: None,
        });
    }
    Ok(features)
}

pub(crate) fn compute_cash_quick_event_features(
    args: &RulePatternIndexArgs,
    account_key: &str,
    rows: &[&RulePatternTxnRow],
) -> Result<Vec<RulePatternFeature>> {
    let mut features = Vec::new();
    features.extend(compute_cash_quick_events_for_params(
        "CASH_QUICK_EVENT",
        account_key,
        rows,
        args.cash_quick_window_minutes,
        args.cash_quick_min_amount,
        None,
    )?);
    features.extend(compute_cash_quick_events_for_params(
        "CASH_QUICK_CANDIDATE_EVENT",
        account_key,
        rows,
        args.cash_candidate_window_minutes,
        args.cash_candidate_min_amount,
        Some((args.cash_candidate_min_ratio, args.cash_candidate_max_ratio)),
    )?);
    Ok(features)
}

fn compute_cash_quick_events_for_params(
    feature_code: &'static str,
    account_key: &str,
    rows: &[&RulePatternTxnRow],
    window_minutes: i64,
    min_amount: f64,
    ratio_range: Option<(f64, f64)>,
) -> Result<Vec<RulePatternFeature>> {
    let in_rows = rows
        .iter()
        .copied()
        .filter(|row| {
            row.direction == "in" && row.cash_class == CashClass::Cash && row.amount >= min_amount
        })
        .collect::<Vec<_>>();
    let out_rows = rows
        .iter()
        .copied()
        .filter(|row| {
            row.direction == "out" && row.cash_class == CashClass::Cash && row.amount >= min_amount
        })
        .collect::<Vec<_>>();
    let window_seconds = window_minutes.max(1) * 60;
    let mut features = Vec::new();
    for (index, in_row) in in_rows.iter().enumerate() {
        let next_in_epoch = in_rows.get(index + 1).map(|row| row.txn_epoch);
        let mut window_end = in_row.txn_epoch + window_seconds;
        if let Some(next_epoch) = next_in_epoch {
            if next_epoch < window_end {
                window_end = next_epoch;
            }
        }
        let matching_outs = out_rows
            .iter()
            .copied()
            .filter(|row| row.txn_epoch > in_row.txn_epoch && row.txn_epoch <= window_end)
            .collect::<Vec<_>>();
        if matching_outs.is_empty() {
            continue;
        }
        let in_amount = in_row.amount;
        let out_amount = round2(matching_outs.iter().map(|row| row.amount).sum::<f64>());
        let out_ratio = out_amount / in_amount.max(0.01);
        if let Some((min_ratio, max_ratio)) = ratio_range {
            if out_ratio < min_ratio || out_ratio > max_ratio {
                continue;
            }
        }
        let last_out = matching_outs
            .last()
            .ok_or_else(|| anyhow!("cash quick matching outs unexpectedly empty"))?;
        let out_txn_ids = matching_outs
            .iter()
            .map(|row| row.txn_id.clone())
            .filter(|item| !item.trim().is_empty())
            .collect::<Vec<_>>();
        let mut txn_ids = Vec::new();
        if !in_row.txn_id.trim().is_empty() {
            txn_ids.push(in_row.txn_id.clone());
        }
        txn_ids.extend(out_txn_ids.iter().cloned());
        let duration_minutes = round2((last_out.txn_epoch - in_row.txn_epoch) as f64 / 60.0);
        let detail = json!({
            "in_txn_id": in_row.txn_id,
            "out_txn_ids": out_txn_ids,
            "in_amount": round2(in_amount),
            "out_amount": out_amount,
            "out_ratio": round4(out_ratio),
            "duration_minutes": duration_minutes,
            "start_time": in_row.txn_time,
            "end_time": last_out.txn_time,
        });
        features.push(RulePatternFeature {
            feature_code,
            account_key: account_key.to_string(),
            direction: String::new(),
            txn_count: txn_ids.len() as i64,
            total_amount: round2(in_amount),
            txn_ids_json: serde_json::to_string(&txn_ids)?,
            amounts_json: "[]".to_string(),
            detail_json: serde_json::to_string(&detail)?,
            first_time: in_row.txn_time.clone(),
            last_time: last_out.txn_time.clone(),
            round_unit: None,
            min_amount,
            min_count: 1,
            min_total_amount: 0.0,
            start_hour: None,
            end_hour: None,
        });
    }
    Ok(features)
}
