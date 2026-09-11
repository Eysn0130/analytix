use anyhow::{bail, Context, Result};
use serde::Deserialize;
use serde_json::Value;
use std::path::PathBuf;

use super::common::require_case_id_and_db_path;

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct QueryStatsTxnRowsArgs {
    pub(crate) case_id: String,
    pub(crate) db_path: PathBuf,
    pub(crate) key_type: String,
    pub(crate) key_value: String,
    #[serde(default)]
    pub(crate) key_values: Vec<String>,
    pub(crate) selected_keys: Vec<String>,
    pub(crate) date_start: String,
    pub(crate) date_end: String,
    pub(crate) start_time: String,
    pub(crate) end_time: String,
    pub(crate) direction: String,
    pub(crate) sort_col: String,
    pub(crate) sort_dir: String,
    pub(crate) limit: i64,
    pub(crate) cursor: Option<Value>,
    #[serde(default = "default_row_format")]
    pub(crate) row_format: String,
    #[serde(default)]
    pub(crate) fields: Vec<String>,
    pub(crate) output_json: Option<PathBuf>,
}

fn default_row_format() -> String {
    "object".to_string()
}

pub(crate) fn parse_query_stats_txn_rows_args(
    iter: impl Iterator<Item = String>,
) -> Result<QueryStatsTxnRowsArgs> {
    let mut case_id = String::new();
    let mut db_path = PathBuf::new();
    let mut key_type = String::from("account");
    let mut key_value = String::new();
    let mut key_values = Vec::new();
    let mut selected_keys = Vec::new();
    let mut date_start = String::new();
    let mut date_end = String::new();
    let mut start_time = String::new();
    let mut end_time = String::new();
    let mut direction = String::from("all");
    let mut sort_col = String::from("txn_time");
    let mut sort_dir = String::from("asc");
    let mut limit = 0_i64;
    let mut cursor = None;
    let mut row_format = String::from("object");
    let mut fields = Vec::new();
    let mut output_json = None;
    let mut args = iter.peekable();
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--case-id" => case_id = crate::required_value(&mut args, "--case-id")?,
            "--db-path" => db_path = PathBuf::from(crate::required_value(&mut args, "--db-path")?),
            "--key-type" => key_type = crate::required_value(&mut args, "--key-type")?,
            "--key-value" => {
                let value = crate::required_value(&mut args, "--key-value")?;
                key_value = value.clone();
                key_values.push(value);
            }
            "--selected-key" => {
                let value = crate::required_value(&mut args, "--selected-key")?;
                if !value.trim().is_empty() {
                    selected_keys.push(value);
                }
            }
            "--date-start" => date_start = crate::required_value(&mut args, "--date-start")?,
            "--date-end" => date_end = crate::required_value(&mut args, "--date-end")?,
            "--start-time" => start_time = crate::required_value(&mut args, "--start-time")?,
            "--end-time" => end_time = crate::required_value(&mut args, "--end-time")?,
            "--direction" => direction = crate::required_value(&mut args, "--direction")?,
            "--sort-col" => sort_col = crate::required_value(&mut args, "--sort-col")?,
            "--sort-dir" => sort_dir = crate::required_value(&mut args, "--sort-dir")?,
            "--row-format" => row_format = crate::required_value(&mut args, "--row-format")?,
            "--field" => {
                let value = crate::required_value(&mut args, "--field")?;
                if !value.trim().is_empty() {
                    fields.push(value);
                }
            }
            "--limit" => {
                limit = crate::required_value(&mut args, "--limit")?
                    .parse()
                    .context("--limit must be an integer")?
            }
            "--cursor-json" => {
                let value = crate::required_value(&mut args, "--cursor-json")?;
                if !value.trim().is_empty() {
                    cursor =
                        Some(serde_json::from_str(&value).context("--cursor-json must be JSON")?);
                }
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
    if cursor.is_some() && limit <= 0 {
        limit = 200;
    }
    Ok(QueryStatsTxnRowsArgs {
        case_id,
        db_path,
        key_type,
        key_value,
        key_values,
        selected_keys,
        date_start,
        date_end,
        start_time,
        end_time,
        direction,
        sort_col,
        sort_dir,
        limit: limit.max(0),
        cursor,
        row_format,
        fields,
        output_json,
    })
}
