use anyhow::{bail, Context, Result};
use std::collections::HashSet;
use std::path::PathBuf;

#[derive(Debug)]
pub(crate) struct RulePatternIndexArgs {
    pub(crate) case_id: String,
    pub(crate) db_path: PathBuf,
    pub(crate) force: bool,
    pub(crate) param_signature: String,
    pub(crate) round_unit: f64,
    pub(crate) round_min_amount: f64,
    pub(crate) round_min_count: i64,
    pub(crate) round_min_total_amount: f64,
    pub(crate) small_fast_window_minutes: i64,
    pub(crate) small_fast_ratio: f64,
    pub(crate) small_fast_min_amount: f64,
    pub(crate) small_fast_max_amount: f64,
    pub(crate) cash_quick_window_minutes: i64,
    pub(crate) cash_quick_min_amount: f64,
    pub(crate) cash_candidate_window_minutes: i64,
    pub(crate) cash_candidate_min_amount: f64,
    pub(crate) cash_candidate_min_ratio: f64,
    pub(crate) cash_candidate_max_ratio: f64,
    pub(crate) near_threshold_amount: f64,
    pub(crate) near_threshold_lower_rate: f64,
    pub(crate) near_threshold_window_minutes: i64,
    pub(crate) near_threshold_min_count: i64,
    pub(crate) near_threshold_min_total_amount: f64,
    pub(crate) repeated_amount_window_minutes: i64,
    pub(crate) repeated_amount_min_amount: f64,
    pub(crate) repeated_amount_min_count: i64,
    pub(crate) repeated_amount_min_total_amount: f64,
    pub(crate) threshold_split_window_minutes: i64,
    pub(crate) threshold_split_amount: f64,
    pub(crate) threshold_split_tolerance_rate: f64,
    pub(crate) threshold_split_min_count: i64,
    pub(crate) high_freq_small_amount_threshold: f64,
    pub(crate) high_freq_window_minutes: i64,
    pub(crate) high_freq_count_threshold: i64,
    pub(crate) high_freq_min_total_amount: f64,
    pub(crate) night_start_hour: i64,
    pub(crate) night_end_hour: i64,
    pub(crate) night_min_count: i64,
    pub(crate) night_min_total_amount: f64,
}

pub(crate) fn parse_args(iter: impl Iterator<Item = String>) -> Result<RulePatternIndexArgs> {
    let mut case_id = String::new();
    let mut db_path = PathBuf::new();
    let mut force = false;
    let mut param_signature = String::new();
    let mut round_unit = 10_000.0_f64;
    let mut round_min_amount = 10_000.0_f64;
    let mut round_min_count = 3_i64;
    let mut round_min_total_amount = 50_000.0_f64;
    let mut small_fast_window_minutes = 60_i64;
    let mut small_fast_ratio = 0.75_f64;
    let mut small_fast_min_amount = 1000.0_f64;
    let mut small_fast_max_amount = 50000.0_f64;
    let mut cash_quick_window_minutes = 60_i64;
    let mut cash_quick_min_amount = 5000.0_f64;
    let mut cash_candidate_window_minutes = 60_i64;
    let mut cash_candidate_min_amount = 1000.0_f64;
    let mut cash_candidate_min_ratio = 0.9_f64;
    let mut cash_candidate_max_ratio = 1.1_f64;
    let mut near_threshold_amount = 50000.0_f64;
    let mut near_threshold_lower_rate = 0.9_f64;
    let mut near_threshold_window_minutes = 14400_i64;
    let mut near_threshold_min_count = 2_i64;
    let mut near_threshold_min_total_amount = 90000.0_f64;
    let mut repeated_amount_window_minutes = 1440_i64;
    let mut repeated_amount_min_amount = 1000.0_f64;
    let mut repeated_amount_min_count = 3_i64;
    let mut repeated_amount_min_total_amount = 10000.0_f64;
    let mut threshold_split_window_minutes = 30_i64;
    let mut threshold_split_amount = 50000.0_f64;
    let mut threshold_split_tolerance_rate = 0.05_f64;
    let mut threshold_split_min_count = 3_i64;
    let mut high_freq_small_amount_threshold = 5000.0_f64;
    let mut high_freq_window_minutes = 30_i64;
    let mut high_freq_count_threshold = 8_i64;
    let mut high_freq_min_total_amount = 20000.0_f64;
    let mut night_start_hour = 22_i64;
    let mut night_end_hour = 6_i64;
    let mut night_min_count = 3_i64;
    let mut night_min_total_amount = 20_000.0_f64;
    let mut args = iter.peekable();
    let mut seen = HashSet::new();
    while let Some(arg) = args.next() {
        if !seen.insert(arg.clone()) {
            bail!("duplicate argument: {arg}");
        }
        match arg.as_str() {
            "--case-id" => case_id = crate::required_value(&mut args, "--case-id")?,
            "--db-path" => db_path = PathBuf::from(crate::required_value(&mut args, "--db-path")?),
            "--force" => {
                force = parse_bool_strict(&crate::required_value(&mut args, "--force")?, "--force")?
            }
            "--param-signature" => {
                param_signature = crate::required_value(&mut args, "--param-signature")?
            }
            "--round-unit" => {
                round_unit = crate::required_value(&mut args, "--round-unit")?
                    .parse()
                    .context("--round-unit must be numeric")?
            }
            "--round-min-amount" => {
                round_min_amount = crate::required_value(&mut args, "--round-min-amount")?
                    .parse()
                    .context("--round-min-amount must be numeric")?
            }
            "--round-min-count" => {
                round_min_count = crate::required_value(&mut args, "--round-min-count")?
                    .parse()
                    .context("--round-min-count must be an integer")?
            }
            "--round-min-total-amount" => {
                round_min_total_amount =
                    crate::required_value(&mut args, "--round-min-total-amount")?
                        .parse()
                        .context("--round-min-total-amount must be numeric")?
            }
            "--small-fast-window-minutes" => {
                small_fast_window_minutes =
                    crate::required_value(&mut args, "--small-fast-window-minutes")?
                        .parse()
                        .context("--small-fast-window-minutes must be an integer")?
            }
            "--small-fast-ratio" => {
                small_fast_ratio = crate::required_value(&mut args, "--small-fast-ratio")?
                    .parse()
                    .context("--small-fast-ratio must be numeric")?
            }
            "--small-fast-min-amount" => {
                small_fast_min_amount = crate::required_value(&mut args, "--small-fast-min-amount")?
                    .parse()
                    .context("--small-fast-min-amount must be numeric")?
            }
            "--small-fast-max-amount" => {
                small_fast_max_amount = crate::required_value(&mut args, "--small-fast-max-amount")?
                    .parse()
                    .context("--small-fast-max-amount must be numeric")?
            }
            "--cash-quick-window-minutes" => {
                cash_quick_window_minutes =
                    crate::required_value(&mut args, "--cash-quick-window-minutes")?
                        .parse()
                        .context("--cash-quick-window-minutes must be an integer")?
            }
            "--cash-quick-min-amount" => {
                cash_quick_min_amount = crate::required_value(&mut args, "--cash-quick-min-amount")?
                    .parse()
                    .context("--cash-quick-min-amount must be numeric")?
            }
            "--cash-candidate-window-minutes" => {
                cash_candidate_window_minutes =
                    crate::required_value(&mut args, "--cash-candidate-window-minutes")?
                        .parse()
                        .context("--cash-candidate-window-minutes must be an integer")?
            }
            "--cash-candidate-min-amount" => {
                cash_candidate_min_amount =
                    crate::required_value(&mut args, "--cash-candidate-min-amount")?
                        .parse()
                        .context("--cash-candidate-min-amount must be numeric")?
            }
            "--cash-candidate-min-ratio" => {
                cash_candidate_min_ratio =
                    crate::required_value(&mut args, "--cash-candidate-min-ratio")?
                        .parse()
                        .context("--cash-candidate-min-ratio must be numeric")?
            }
            "--cash-candidate-max-ratio" => {
                cash_candidate_max_ratio =
                    crate::required_value(&mut args, "--cash-candidate-max-ratio")?
                        .parse()
                        .context("--cash-candidate-max-ratio must be numeric")?
            }
            "--near-threshold-amount" => {
                near_threshold_amount = crate::required_value(&mut args, "--near-threshold-amount")?
                    .parse()
                    .context("--near-threshold-amount must be numeric")?
            }
            "--near-threshold-lower-rate" => {
                near_threshold_lower_rate =
                    crate::required_value(&mut args, "--near-threshold-lower-rate")?
                        .parse()
                        .context("--near-threshold-lower-rate must be numeric")?
            }
            "--near-threshold-window-minutes" => {
                near_threshold_window_minutes =
                    crate::required_value(&mut args, "--near-threshold-window-minutes")?
                        .parse()
                        .context("--near-threshold-window-minutes must be an integer")?
            }
            "--near-threshold-min-count" => {
                near_threshold_min_count =
                    crate::required_value(&mut args, "--near-threshold-min-count")?
                        .parse()
                        .context("--near-threshold-min-count must be an integer")?
            }
            "--near-threshold-min-total-amount" => {
                near_threshold_min_total_amount =
                    crate::required_value(&mut args, "--near-threshold-min-total-amount")?
                        .parse()
                        .context("--near-threshold-min-total-amount must be numeric")?
            }
            "--repeated-amount-window-minutes" => {
                repeated_amount_window_minutes =
                    crate::required_value(&mut args, "--repeated-amount-window-minutes")?
                        .parse()
                        .context("--repeated-amount-window-minutes must be an integer")?
            }
            "--repeated-amount-min-amount" => {
                repeated_amount_min_amount =
                    crate::required_value(&mut args, "--repeated-amount-min-amount")?
                        .parse()
                        .context("--repeated-amount-min-amount must be numeric")?
            }
            "--repeated-amount-min-count" => {
                repeated_amount_min_count =
                    crate::required_value(&mut args, "--repeated-amount-min-count")?
                        .parse()
                        .context("--repeated-amount-min-count must be an integer")?
            }
            "--repeated-amount-min-total-amount" => {
                repeated_amount_min_total_amount =
                    crate::required_value(&mut args, "--repeated-amount-min-total-amount")?
                        .parse()
                        .context("--repeated-amount-min-total-amount must be numeric")?
            }
            "--threshold-split-window-minutes" => {
                threshold_split_window_minutes =
                    crate::required_value(&mut args, "--threshold-split-window-minutes")?
                        .parse()
                        .context("--threshold-split-window-minutes must be an integer")?
            }
            "--threshold-split-amount" => {
                threshold_split_amount =
                    crate::required_value(&mut args, "--threshold-split-amount")?
                        .parse()
                        .context("--threshold-split-amount must be numeric")?
            }
            "--threshold-split-tolerance-rate" => {
                threshold_split_tolerance_rate =
                    crate::required_value(&mut args, "--threshold-split-tolerance-rate")?
                        .parse()
                        .context("--threshold-split-tolerance-rate must be numeric")?
            }
            "--threshold-split-min-count" => {
                threshold_split_min_count =
                    crate::required_value(&mut args, "--threshold-split-min-count")?
                        .parse()
                        .context("--threshold-split-min-count must be an integer")?
            }
            "--high-freq-small-amount-threshold" => {
                high_freq_small_amount_threshold =
                    crate::required_value(&mut args, "--high-freq-small-amount-threshold")?
                        .parse()
                        .context("--high-freq-small-amount-threshold must be numeric")?
            }
            "--high-freq-window-minutes" => {
                high_freq_window_minutes =
                    crate::required_value(&mut args, "--high-freq-window-minutes")?
                        .parse()
                        .context("--high-freq-window-minutes must be an integer")?
            }
            "--high-freq-count-threshold" => {
                high_freq_count_threshold =
                    crate::required_value(&mut args, "--high-freq-count-threshold")?
                        .parse()
                        .context("--high-freq-count-threshold must be an integer")?
            }
            "--high-freq-min-total-amount" => {
                high_freq_min_total_amount =
                    crate::required_value(&mut args, "--high-freq-min-total-amount")?
                        .parse()
                        .context("--high-freq-min-total-amount must be numeric")?
            }
            "--night-start-hour" => {
                night_start_hour = crate::required_value(&mut args, "--night-start-hour")?
                    .parse()
                    .context("--night-start-hour must be an integer")?
            }
            "--night-end-hour" => {
                night_end_hour = crate::required_value(&mut args, "--night-end-hour")?
                    .parse()
                    .context("--night-end-hour must be an integer")?
            }
            "--night-min-count" => {
                night_min_count = crate::required_value(&mut args, "--night-min-count")?
                    .parse()
                    .context("--night-min-count must be an integer")?
            }
            "--night-min-total-amount" => {
                night_min_total_amount =
                    crate::required_value(&mut args, "--night-min-total-amount")?
                        .parse()
                        .context("--night-min-total-amount must be numeric")?
            }
            other => bail!("unknown argument: {other}"),
        }
    }
    if case_id.trim().is_empty() {
        bail!("missing --case-id");
    }
    if db_path.as_os_str().is_empty() {
        bail!("missing --db-path");
    }
    if param_signature.trim().is_empty() {
        bail!("missing --param-signature");
    }
    require_min_f64("--round-unit", round_unit, 1.0)?;
    require_min_f64("--round-min-amount", round_min_amount, 0.0)?;
    require_min_i64("--round-min-count", round_min_count, 2)?;
    require_min_f64("--round-min-total-amount", round_min_total_amount, 0.0)?;
    require_min_i64("--small-fast-window-minutes", small_fast_window_minutes, 1)?;
    require_min_f64("--small-fast-ratio", small_fast_ratio, 0.1)?;
    require_min_f64("--small-fast-min-amount", small_fast_min_amount, 0.0)?;
    require_min_f64("--small-fast-max-amount", small_fast_max_amount, 0.0)?;
    require_ordered_f64(
        "--small-fast-max-amount",
        small_fast_max_amount,
        "--small-fast-min-amount",
        small_fast_min_amount,
    )?;
    require_min_i64("--cash-quick-window-minutes", cash_quick_window_minutes, 1)?;
    require_min_f64("--cash-quick-min-amount", cash_quick_min_amount, 0.0)?;
    require_min_i64(
        "--cash-candidate-window-minutes",
        cash_candidate_window_minutes,
        1,
    )?;
    require_min_f64(
        "--cash-candidate-min-amount",
        cash_candidate_min_amount,
        0.0,
    )?;
    require_min_f64("--cash-candidate-min-ratio", cash_candidate_min_ratio, 0.1)?;
    require_min_f64("--cash-candidate-max-ratio", cash_candidate_max_ratio, 0.1)?;
    require_ordered_f64(
        "--cash-candidate-max-ratio",
        cash_candidate_max_ratio,
        "--cash-candidate-min-ratio",
        cash_candidate_min_ratio,
    )?;
    require_min_f64("--near-threshold-amount", near_threshold_amount, 0.0)?;
    require_range_f64(
        "--near-threshold-lower-rate",
        near_threshold_lower_rate,
        0.0,
        0.99,
    )?;
    require_min_i64(
        "--near-threshold-window-minutes",
        near_threshold_window_minutes,
        1,
    )?;
    require_min_i64("--near-threshold-min-count", near_threshold_min_count, 2)?;
    require_min_f64(
        "--near-threshold-min-total-amount",
        near_threshold_min_total_amount,
        0.0,
    )?;
    require_min_i64(
        "--repeated-amount-window-minutes",
        repeated_amount_window_minutes,
        1,
    )?;
    require_min_f64(
        "--repeated-amount-min-amount",
        repeated_amount_min_amount,
        0.0,
    )?;
    require_min_i64("--repeated-amount-min-count", repeated_amount_min_count, 2)?;
    require_min_f64(
        "--repeated-amount-min-total-amount",
        repeated_amount_min_total_amount,
        0.0,
    )?;
    require_min_i64(
        "--threshold-split-window-minutes",
        threshold_split_window_minutes,
        1,
    )?;
    require_min_f64("--threshold-split-amount", threshold_split_amount, 0.0)?;
    require_min_f64(
        "--threshold-split-tolerance-rate",
        threshold_split_tolerance_rate,
        0.0,
    )?;
    require_min_i64("--threshold-split-min-count", threshold_split_min_count, 2)?;
    require_min_f64(
        "--high-freq-small-amount-threshold",
        high_freq_small_amount_threshold,
        0.0,
    )?;
    require_min_i64("--high-freq-window-minutes", high_freq_window_minutes, 1)?;
    require_min_i64("--high-freq-count-threshold", high_freq_count_threshold, 2)?;
    require_min_f64(
        "--high-freq-min-total-amount",
        high_freq_min_total_amount,
        0.0,
    )?;
    require_range_i64("--night-start-hour", night_start_hour, 0, 23)?;
    require_range_i64("--night-end-hour", night_end_hour, 0, 23)?;
    require_min_i64("--night-min-count", night_min_count, 2)?;
    require_min_f64("--night-min-total-amount", night_min_total_amount, 0.0)?;
    Ok(RulePatternIndexArgs {
        case_id,
        db_path,
        force,
        param_signature,
        round_unit,
        round_min_amount,
        round_min_count,
        round_min_total_amount,
        small_fast_window_minutes,
        small_fast_ratio,
        small_fast_min_amount,
        small_fast_max_amount,
        cash_quick_window_minutes,
        cash_quick_min_amount,
        cash_candidate_window_minutes,
        cash_candidate_min_amount,
        cash_candidate_min_ratio,
        cash_candidate_max_ratio,
        near_threshold_amount,
        near_threshold_lower_rate,
        near_threshold_window_minutes,
        near_threshold_min_count,
        near_threshold_min_total_amount,
        repeated_amount_window_minutes,
        repeated_amount_min_amount,
        repeated_amount_min_count,
        repeated_amount_min_total_amount,
        threshold_split_window_minutes,
        threshold_split_amount,
        threshold_split_tolerance_rate,
        threshold_split_min_count,
        high_freq_small_amount_threshold,
        high_freq_window_minutes,
        high_freq_count_threshold,
        high_freq_min_total_amount,
        night_start_hour,
        night_end_hour,
        night_min_count,
        night_min_total_amount,
    })
}

fn parse_bool_strict(value: &str, flag: &str) -> Result<bool> {
    match value.trim().to_ascii_lowercase().as_str() {
        "1" | "true" | "yes" | "on" => Ok(true),
        "0" | "false" | "no" | "off" => Ok(false),
        _ => bail!("{flag} must be a boolean"),
    }
}

fn require_min_f64(flag: &str, value: f64, minimum: f64) -> Result<()> {
    if !value.is_finite() || value < minimum {
        bail!("{flag} must be finite and >= {minimum}");
    }
    Ok(())
}

fn require_range_f64(flag: &str, value: f64, minimum: f64, maximum: f64) -> Result<()> {
    if !value.is_finite() || value < minimum || value > maximum {
        bail!("{flag} must be finite and between {minimum} and {maximum}");
    }
    Ok(())
}

fn require_min_i64(flag: &str, value: i64, minimum: i64) -> Result<()> {
    if value < minimum {
        bail!("{flag} must be >= {minimum}");
    }
    Ok(())
}

fn require_range_i64(flag: &str, value: i64, minimum: i64, maximum: i64) -> Result<()> {
    if value < minimum || value > maximum {
        bail!("{flag} must be between {minimum} and {maximum}");
    }
    Ok(())
}

fn require_ordered_f64(upper_flag: &str, upper: f64, lower_flag: &str, lower: f64) -> Result<()> {
    if upper < lower {
        bail!("{upper_flag} must be >= {lower_flag}");
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::parse_args;

    fn required_args() -> Vec<String> {
        [
            "--case-id",
            "case-a",
            "--db-path",
            "case.duckdb",
            "--param-signature",
            "signature-a",
        ]
        .into_iter()
        .map(str::to_string)
        .collect()
    }

    fn parse_with(flag: &str, value: &str) -> anyhow::Result<super::RulePatternIndexArgs> {
        let mut args = required_args();
        args.extend([flag.to_string(), value.to_string()]);
        parse_args(args.into_iter())
    }

    #[test]
    fn absent_parameters_use_the_schema_defaults() {
        let args = parse_args(required_args().into_iter()).expect("parse defaults");
        assert_eq!(args.round_unit, 10_000.0);
        assert_eq!(args.round_min_amount, 10_000.0);
        assert_eq!(args.round_min_count, 3);
        assert_eq!(args.round_min_total_amount, 50_000.0);
        assert_eq!(args.night_min_count, 3);
        assert_eq!(args.night_min_total_amount, 20_000.0);
    }

    #[test]
    fn explicit_zero_is_preserved_for_zero_valid_amounts() {
        let args = parse_with("--round-min-amount", "0").expect("zero is valid");
        assert_eq!(args.round_min_amount, 0.0);
    }

    #[test]
    fn duplicate_arguments_are_rejected() {
        let mut args = required_args();
        args.extend(["--round-unit".to_string(), "100".to_string()]);
        args.extend(["--round-unit".to_string(), "200".to_string()]);
        let error = parse_args(args.into_iter()).expect_err("duplicate must fail");
        assert!(error
            .to_string()
            .contains("duplicate argument: --round-unit"));
    }

    #[test]
    fn explicit_nonfinite_and_out_of_range_parameters_are_rejected() {
        for (flag, value) in [
            ("--round-unit", "NaN"),
            ("--round-min-amount", "inf"),
            ("--round-min-count", "1"),
            ("--small-fast-window-minutes", "0"),
            ("--small-fast-ratio", "0.09"),
            ("--near-threshold-lower-rate", "1"),
            ("--repeated-amount-min-count", "-1"),
            ("--threshold-split-tolerance-rate", "-0.1"),
            ("--high-freq-count-threshold", "1"),
            ("--night-start-hour", "24"),
            ("--night-end-hour", "-1"),
        ] {
            assert!(parse_with(flag, value).is_err(), "{flag}={value} must fail");
        }
    }

    #[test]
    fn dependent_bounds_and_invalid_booleans_are_rejected() {
        assert!(parse_with("--small-fast-max-amount", "999").is_err());
        assert!(parse_with("--cash-candidate-max-ratio", "0.5").is_err());
        assert!(parse_with("--force", "sometimes").is_err());
    }
}
