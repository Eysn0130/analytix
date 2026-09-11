use anyhow::{bail, Result};
use std::path::PathBuf;

use super::{
    common::{parse_finite_f64_arg, require_case_id_and_db_path},
    parse_chart_filter_args_json, QueryChartFilterArg,
};

#[derive(Debug)]
pub(crate) struct QueryChartDashboardArgs {
    pub(crate) case_id: String,
    pub(crate) db_path: PathBuf,
    pub(crate) selected_keys: Vec<String>,
    pub(crate) date_start: String,
    pub(crate) date_end: String,
    pub(crate) metric_mode: String,
    pub(crate) direction_mode: String,
    pub(crate) granularity: String,
    pub(crate) success_filter: String,
    pub(crate) cash_filter: String,
    pub(crate) selection_mode: String,
    pub(crate) large_txn_threshold: f64,
    pub(crate) chart_filters: Vec<QueryChartFilterArg>,
}

pub(crate) fn parse_query_chart_dashboard_args(
    iter: impl Iterator<Item = String>,
) -> Result<QueryChartDashboardArgs> {
    let mut case_id = String::new();
    let mut db_path = PathBuf::new();
    let mut selected_keys = Vec::new();
    let mut date_start = String::new();
    let mut date_end = String::new();
    let mut metric_mode = String::from("amount");
    let mut direction_mode = String::from("all");
    let mut granularity = String::from("day");
    let mut success_filter = String::from("all");
    let mut cash_filter = String::from("all");
    let mut selection_mode = String::new();
    let mut large_txn_threshold = 50000.0;
    let mut chart_filters = Vec::new();
    let mut args = iter.peekable();
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--case-id" => case_id = crate::required_value(&mut args, "--case-id")?,
            "--db-path" => db_path = PathBuf::from(crate::required_value(&mut args, "--db-path")?),
            "--selected-key" => {
                let value = crate::required_value(&mut args, "--selected-key")?;
                if !value.trim().is_empty() {
                    selected_keys.push(value);
                }
            }
            "--date-start" => date_start = crate::required_value(&mut args, "--date-start")?,
            "--date-end" => date_end = crate::required_value(&mut args, "--date-end")?,
            "--metric-mode" => metric_mode = crate::required_value(&mut args, "--metric-mode")?,
            "--direction-mode" => {
                direction_mode = crate::required_value(&mut args, "--direction-mode")?
            }
            "--granularity" => granularity = crate::required_value(&mut args, "--granularity")?,
            "--success-filter" => {
                success_filter = crate::required_value(&mut args, "--success-filter")?
            }
            "--cash-filter" => cash_filter = crate::required_value(&mut args, "--cash-filter")?,
            "--selection-mode" => {
                selection_mode = crate::required_value(&mut args, "--selection-mode")?
            }
            "--large-txn-threshold" => {
                let value = crate::required_value(&mut args, "--large-txn-threshold")?;
                large_txn_threshold = parse_finite_f64_arg(&value, "--large-txn-threshold")?;
            }
            "--chart-filters-json" => {
                let value = crate::required_value(&mut args, "--chart-filters-json")?;
                chart_filters.extend(parse_chart_filter_args_json(&value)?);
            }
            other => bail!("unknown argument: {other}"),
        }
    }
    require_case_id_and_db_path(&case_id, &db_path)?;
    if large_txn_threshold < 0.0 {
        bail!("--large-txn-threshold must be non-negative");
    }
    Ok(QueryChartDashboardArgs {
        case_id,
        db_path,
        selected_keys,
        date_start,
        date_end,
        metric_mode,
        direction_mode,
        granularity,
        success_filter,
        cash_filter,
        selection_mode,
        large_txn_threshold,
        chart_filters,
    })
}
