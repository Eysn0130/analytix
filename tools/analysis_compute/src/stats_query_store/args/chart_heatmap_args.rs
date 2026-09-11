use anyhow::{bail, Result};
use std::path::PathBuf;

use super::{common::require_case_id_and_db_path, QueryChartFilterArg};

#[derive(Debug)]
pub(crate) struct QueryChartHeatmapArgs {
    pub(crate) case_id: String,
    pub(crate) db_path: PathBuf,
    pub(crate) selected_keys: Vec<String>,
    pub(crate) date_start: String,
    pub(crate) date_end: String,
    pub(crate) metric_mode: String,
    pub(crate) direction_mode: String,
    pub(crate) success_filter: String,
    pub(crate) cash_filter: String,
    pub(crate) chart_filters: Vec<QueryChartFilterArg>,
}

pub(crate) fn parse_query_chart_heatmap_args(
    iter: impl Iterator<Item = String>,
) -> Result<QueryChartHeatmapArgs> {
    let mut case_id = String::new();
    let mut db_path = PathBuf::new();
    let mut selected_keys = Vec::new();
    let mut date_start = String::new();
    let mut date_end = String::new();
    let mut metric_mode = String::from("amount");
    let mut direction_mode = String::from("all");
    let mut success_filter = String::from("all");
    let mut cash_filter = String::from("all");
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
            "--success-filter" => {
                success_filter = crate::required_value(&mut args, "--success-filter")?
            }
            "--cash-filter" => cash_filter = crate::required_value(&mut args, "--cash-filter")?,
            other => bail!("unknown argument: {other}"),
        }
    }
    require_case_id_and_db_path(&case_id, &db_path)?;
    Ok(QueryChartHeatmapArgs {
        case_id,
        db_path,
        selected_keys,
        date_start,
        date_end,
        metric_mode,
        direction_mode,
        success_filter,
        cash_filter,
        chart_filters: Vec::new(),
    })
}
