mod aggregate_features;
mod model;
mod normalization;
#[cfg(test)]
mod tests;
mod window_features;

use anyhow::Result;
use std::collections::HashMap;

use crate::rule_pattern_args::RulePatternIndexArgs;

use aggregate_features::{compute_night_activity_feature, compute_round_amount_features};
pub(crate) use model::{CashClass, RulePatternFeature, RulePatternTxnRow};
pub(crate) use normalization::{classify_rule_cash_txn, normalize_rule_direction};
use window_features::{
    compute_cash_quick_event_features, compute_high_freq_small_out_features,
    compute_near_threshold_features, compute_repeated_amount_features,
    compute_small_fast_event_features, compute_threshold_split_features,
};

pub(crate) fn compute_rule_pattern_features(
    args: &RulePatternIndexArgs,
    rows: &[RulePatternTxnRow],
) -> Result<Vec<RulePatternFeature>> {
    let mut by_account: HashMap<&str, Vec<&RulePatternTxnRow>> = HashMap::new();
    for row in rows {
        by_account
            .entry(row.account_key.as_str())
            .or_default()
            .push(row);
    }

    let mut features = Vec::new();
    let cash_dependent_ready = rows.iter().all(|row| row.cash_class.is_known());
    for (account_key, account_rows) in by_account {
        if cash_dependent_ready {
            features.extend(compute_small_fast_event_features(
                args,
                account_key,
                &account_rows,
            )?);
            features.extend(compute_cash_quick_event_features(
                args,
                account_key,
                &account_rows,
            )?);
        }
        features.extend(compute_near_threshold_features(
            args,
            account_key,
            &account_rows,
        )?);
        features.extend(compute_repeated_amount_features(
            args,
            account_key,
            &account_rows,
        )?);
        features.extend(compute_threshold_split_features(
            args,
            account_key,
            &account_rows,
        )?);
        features.extend(compute_high_freq_small_out_features(
            args,
            account_key,
            &account_rows,
        )?);
        features.extend(compute_round_amount_features(
            args,
            account_key,
            &account_rows,
        )?);
        if let Some(night_feature) =
            compute_night_activity_feature(args, account_key, &account_rows)?
        {
            features.push(night_feature);
        }
    }
    Ok(features)
}
