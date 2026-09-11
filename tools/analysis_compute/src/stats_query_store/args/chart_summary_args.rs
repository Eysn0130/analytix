use anyhow::{bail, Result};
use std::path::PathBuf;

use super::{
    common::{parse_finite_f64_arg, require_case_id_and_db_path},
    QueryChartFilterArg,
};

#[derive(Debug)]
pub(crate) struct QueryChartSummaryArgs {
    pub(crate) case_id: String,
    pub(crate) db_path: PathBuf,
    pub(crate) selected_keys: Vec<String>,
    pub(crate) date_start: String,
    pub(crate) date_end: String,
    pub(crate) success_filter: String,
    pub(crate) cash_filter: String,
    pub(crate) selection_mode: String,
    pub(crate) large_txn_threshold: f64,
    pub(crate) chart_filters: Vec<QueryChartFilterArg>,
}

pub(crate) fn parse_query_chart_summary_args(
    iter: impl Iterator<Item = String>,
) -> Result<QueryChartSummaryArgs> {
    let mut case_id = String::new();
    let mut db_path = PathBuf::new();
    let mut selected_keys = Vec::new();
    let mut date_start = String::new();
    let mut date_end = String::new();
    let mut success_filter = String::from("all");
    let mut cash_filter = String::from("all");
    let mut selection_mode = String::new();
    let mut large_txn_threshold = 50000.0;
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
            other => bail!("unknown argument: {other}"),
        }
    }
    require_case_id_and_db_path(&case_id, &db_path)?;
    if large_txn_threshold < 0.0 {
        bail!("--large-txn-threshold must be non-negative");
    }
    Ok(QueryChartSummaryArgs {
        case_id,
        db_path,
        selected_keys,
        date_start,
        date_end,
        success_filter,
        cash_filter,
        selection_mode,
        large_txn_threshold,
        chart_filters: Vec::new(),
    })
}
