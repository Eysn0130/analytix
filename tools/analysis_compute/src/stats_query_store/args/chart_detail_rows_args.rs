use anyhow::{bail, Context, Result};
use std::path::PathBuf;

use super::{
    common::require_case_id_and_db_path, parse_chart_filter_args_json, QueryChartFilterArg,
};

#[derive(Debug)]
pub(crate) struct QueryChartDetailRowsArgs {
    pub(crate) case_id: String,
    pub(crate) db_path: PathBuf,
    pub(crate) selected_keys: Vec<String>,
    pub(crate) date_start: String,
    pub(crate) date_end: String,
    pub(crate) direction_mode: String,
    pub(crate) success_filter: String,
    pub(crate) cash_filter: String,
    pub(crate) sort_col: String,
    pub(crate) sort_dir: String,
    pub(crate) page: i64,
    pub(crate) limit: i64,
    pub(crate) chart_filters: Vec<QueryChartFilterArg>,
    pub(crate) output_json: Option<PathBuf>,
}

pub(crate) fn parse_query_chart_detail_rows_args(
    iter: impl Iterator<Item = String>,
) -> Result<QueryChartDetailRowsArgs> {
    let mut case_id = String::new();
    let mut db_path = PathBuf::new();
    let mut selected_keys = Vec::new();
    let mut date_start = String::new();
    let mut date_end = String::new();
    let mut direction_mode = String::from("all");
    let mut success_filter = String::from("all");
    let mut cash_filter = String::from("all");
    let mut sort_col = String::from("txn_time");
    let mut sort_dir = String::from("asc");
    let mut page = 1_i64;
    let mut limit = 200_i64;
    let mut chart_filters = Vec::new();
    let mut output_json = None;
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
            "--direction-mode" => {
                direction_mode = crate::required_value(&mut args, "--direction-mode")?
            }
            "--success-filter" => {
                success_filter = crate::required_value(&mut args, "--success-filter")?
            }
            "--cash-filter" => cash_filter = crate::required_value(&mut args, "--cash-filter")?,
            "--sort-col" => sort_col = crate::required_value(&mut args, "--sort-col")?,
            "--sort-dir" => sort_dir = crate::required_value(&mut args, "--sort-dir")?,
            "--page" => {
                page = crate::required_value(&mut args, "--page")?
                    .parse()
                    .context("--page must be an integer")?
            }
            "--limit" => {
                limit = crate::required_value(&mut args, "--limit")?
                    .parse()
                    .context("--limit must be an integer")?
            }
            "--chart-filters-json" => {
                let value = crate::required_value(&mut args, "--chart-filters-json")?;
                chart_filters.extend(parse_chart_filter_args_json(&value)?);
            }
            "--output-json" => {
                output_json = Some(PathBuf::from(crate::required_value(
                    &mut args,
                    "--output-json",
                )?));
            }
            other => bail!("unknown argument: {other}"),
        }
    }
    require_case_id_and_db_path(&case_id, &db_path)?;
    Ok(QueryChartDetailRowsArgs {
        case_id,
        db_path,
        selected_keys,
        date_start,
        date_end,
        direction_mode,
        success_filter,
        cash_filter,
        sort_col,
        sort_dir,
        page: page.max(1),
        limit: limit.clamp(1, 5000),
        chart_filters,
        output_json,
    })
}
