mod aggregate_tests;
mod cash_classification_vectors_v1;
mod fast_events_tests;
mod normalization_tests;
mod structuring_tests;

use std::path::PathBuf;

use crate::rule_pattern_args::RulePatternIndexArgs;

use super::model::{CashClass, RulePatternTxnRow};

fn default_args() -> RulePatternIndexArgs {
    RulePatternIndexArgs {
        case_id: "case-1".to_string(),
        db_path: PathBuf::new(),
        force: false,
        param_signature: "sig".to_string(),
        round_unit: 1.0,
        round_min_amount: 0.0,
        round_min_count: 2,
        round_min_total_amount: 0.0,
        small_fast_window_minutes: 60,
        small_fast_ratio: 0.75,
        small_fast_min_amount: 1000.0,
        small_fast_max_amount: 50000.0,
        cash_quick_window_minutes: 60,
        cash_quick_min_amount: 5000.0,
        cash_candidate_window_minutes: 60,
        cash_candidate_min_amount: 1000.0,
        cash_candidate_min_ratio: 0.9,
        cash_candidate_max_ratio: 1.1,
        near_threshold_amount: 50000.0,
        near_threshold_lower_rate: 0.9,
        near_threshold_window_minutes: 14400,
        near_threshold_min_count: 2,
        near_threshold_min_total_amount: 90000.0,
        repeated_amount_window_minutes: 1440,
        repeated_amount_min_amount: 1000.0,
        repeated_amount_min_count: 3,
        repeated_amount_min_total_amount: 10000.0,
        threshold_split_window_minutes: 30,
        threshold_split_amount: 50000.0,
        threshold_split_tolerance_rate: 0.05,
        threshold_split_min_count: 3,
        high_freq_small_amount_threshold: 5000.0,
        high_freq_window_minutes: 30,
        high_freq_count_threshold: 8,
        high_freq_min_total_amount: 20000.0,
        night_start_hour: 22,
        night_end_hour: 6,
        night_min_count: 2,
        night_min_total_amount: 0.0,
    }
}

fn txn_row(txn_id: &str, epoch: i64, amount: f64, direction: &str) -> RulePatternTxnRow {
    let total_minutes = 13 * 60 + epoch.div_euclid(60);
    let hour = total_minutes.div_euclid(60);
    let minute = total_minutes.rem_euclid(60);
    RulePatternTxnRow {
        account_key: "A-011".to_string(),
        txn_id: txn_id.to_string(),
        txn_time: format!("2026-04-11 {hour:02}:{minute:02}:00"),
        txn_epoch: epoch,
        txn_hour: hour,
        amount,
        direction: direction.to_string(),
        cash_class: CashClass::NonCash,
    }
}

fn cash_txn_row(txn_id: &str, epoch: i64, amount: f64, direction: &str) -> RulePatternTxnRow {
    RulePatternTxnRow {
        cash_class: CashClass::Cash,
        ..txn_row(txn_id, epoch, amount, direction)
    }
}

fn txn_row_at(
    txn_id: &str,
    txn_time: &str,
    txn_epoch: i64,
    txn_hour: i64,
    amount: f64,
    direction: &str,
) -> RulePatternTxnRow {
    RulePatternTxnRow {
        account_key: "A-011".to_string(),
        txn_id: txn_id.to_string(),
        txn_time: txn_time.to_string(),
        txn_epoch,
        txn_hour,
        amount,
        direction: direction.to_string(),
        cash_class: CashClass::NonCash,
    }
}
