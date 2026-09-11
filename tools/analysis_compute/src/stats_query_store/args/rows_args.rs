use anyhow::{bail, Context, Result};
use serde::Deserialize;
use std::path::PathBuf;

use super::common::require_case_id_and_db_path;

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct QueryStatsRowsArgs {
    pub(crate) case_id: String,
    pub(crate) db_path: PathBuf,
    pub(crate) mode: String,
    pub(crate) selected_keys: Vec<String>,
    pub(crate) date_start: String,
    pub(crate) date_end: String,
    pub(crate) search_text: String,
    pub(crate) row_sort_col: String,
    pub(crate) row_sort_dir: String,
    pub(crate) row_offset: i64,
    pub(crate) row_limit: i64,
    #[serde(default = "default_row_format")]
    pub(crate) row_format: String,
    #[serde(default)]
    pub(crate) fields: Vec<String>,
    pub(crate) output_json: Option<PathBuf>,
}

fn default_row_format() -> String {
    "object".to_string()
}

pub(crate) fn parse_query_stats_rows_args(
    iter: impl Iterator<Item = String>,
) -> Result<QueryStatsRowsArgs> {
    let mut case_id = String::new();
    let mut db_path = PathBuf::new();
    let mut mode = String::new();
    let mut selected_keys = Vec::new();
    let mut date_start = String::new();
    let mut date_end = String::new();
    let mut search_text = String::new();
    let mut row_sort_col = String::new();
    let mut row_sort_dir = String::from("desc");
    let mut row_offset = 0_i64;
    let mut row_limit = 0_i64;
    let mut row_format = String::from("object");
    let mut fields = Vec::new();
    let mut output_json = None;
    let mut args = iter.peekable();
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--case-id" => case_id = crate::required_value(&mut args, "--case-id")?,
            "--db-path" => db_path = PathBuf::from(crate::required_value(&mut args, "--db-path")?),
            "--mode" => mode = crate::required_value(&mut args, "--mode")?,
            "--selected-key" => {
                let value = crate::required_value(&mut args, "--selected-key")?;
                if !value.trim().is_empty() {
                    selected_keys.push(value);
                }
            }
            "--date-start" => date_start = crate::required_value(&mut args, "--date-start")?,
            "--date-end" => date_end = crate::required_value(&mut args, "--date-end")?,
            "--search-text" => search_text = crate::required_value(&mut args, "--search-text")?,
            "--row-sort-col" => row_sort_col = crate::required_value(&mut args, "--row-sort-col")?,
            "--row-sort-dir" => row_sort_dir = crate::required_value(&mut args, "--row-sort-dir")?,
            "--row-format" => row_format = crate::required_value(&mut args, "--row-format")?,
            "--field" => {
                let value = crate::required_value(&mut args, "--field")?;
                if !value.trim().is_empty() {
                    fields.push(value);
                }
            }
            "--row-offset" => {
                row_offset = crate::required_value(&mut args, "--row-offset")?
                    .parse()
                    .context("--row-offset must be an integer")?
            }
            "--row-limit" => {
                row_limit = crate::required_value(&mut args, "--row-limit")?
                    .parse()
                    .context("--row-limit must be an integer")?
            }
            "--output-json" => {
                output_json = Some(PathBuf::from(crate::required_value(
                    &mut args,
                    "--output-json",
                )?))
            }
            other => bail!("unknown argument: {other}"),
        }
    }
    require_case_id_and_db_path(&case_id, &db_path)?;
    Ok(QueryStatsRowsArgs {
        case_id,
        db_path,
        mode,
        selected_keys,
        date_start,
        date_end,
        search_text,
        row_sort_col,
        row_sort_dir,
        row_offset: row_offset.max(0),
        row_limit: row_limit.max(0),
        row_format,
        fields,
        output_json,
    })
}
