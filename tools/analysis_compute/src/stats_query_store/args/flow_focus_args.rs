use anyhow::{bail, Context, Result};
use std::path::PathBuf;

use super::super::values::normalize_placeholder_kind;
use super::common::{parse_finite_f64_arg, require_case_id_and_db_path};

#[derive(Debug)]
pub(crate) struct QueryFlowFocusRowsArgs {
    pub(crate) case_id: String,
    pub(crate) db_path: PathBuf,
    pub(crate) query_seed_ids: Vec<String>,
    pub(crate) selected_focus_ids: Vec<String>,
    pub(crate) selected_placeholder_kinds: Vec<String>,
    pub(crate) include_missing_counterparty: bool,
    pub(crate) direction: String,
    pub(crate) min_amount: f64,
    pub(crate) date_start: String,
    pub(crate) date_end_excl: String,
}

#[derive(Debug)]
pub(crate) struct QueryFlowFocusGraphArgs {
    pub(crate) rows: QueryFlowFocusRowsArgs,
    pub(crate) seed_ids: Vec<String>,
    pub(crate) depth: i64,
    pub(crate) view_mode: String,
    pub(crate) focus_id_raw: String,
    pub(crate) focus_label: String,
    pub(crate) request_id: String,
    pub(crate) source: String,
    pub(crate) focus_key_type: String,
    pub(crate) expected_total_amount: Option<f64>,
    pub(crate) expected_row_count: Option<i64>,
    pub(crate) output_json: Option<PathBuf>,
}

pub(crate) fn parse_query_flow_focus_rows_args(
    iter: impl Iterator<Item = String>,
) -> Result<QueryFlowFocusRowsArgs> {
    let mut case_id = String::new();
    let mut db_path = PathBuf::new();
    let mut query_seed_ids = Vec::new();
    let mut selected_focus_ids = Vec::new();
    let mut selected_placeholder_kinds = Vec::new();
    let mut include_missing_counterparty = false;
    let mut direction = String::from("all");
    let mut min_amount = 0.0_f64;
    let mut date_start = String::new();
    let mut date_end_excl = String::new();
    let mut args = iter.peekable();
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--case-id" => case_id = crate::required_value(&mut args, "--case-id")?,
            "--db-path" => db_path = PathBuf::from(crate::required_value(&mut args, "--db-path")?),
            "--query-seed-id" => {
                let value = crate::required_value(&mut args, "--query-seed-id")?;
                if !value.trim().is_empty() {
                    query_seed_ids.push(value);
                }
            }
            "--selected-focus-id" => {
                let value = crate::required_value(&mut args, "--selected-focus-id")?;
                if !value.trim().is_empty() {
                    selected_focus_ids.push(value);
                }
            }
            "--selected-placeholder-kind" => {
                let value = crate::required_value(&mut args, "--selected-placeholder-kind")?;
                if let Some(kind) = normalize_placeholder_kind(&value) {
                    selected_placeholder_kinds.push(kind.to_string());
                }
            }
            "--include-missing-counterparty" => {
                include_missing_counterparty = crate::parse_bool(&crate::required_value(
                    &mut args,
                    "--include-missing-counterparty",
                )?)
            }
            "--direction" => direction = crate::required_value(&mut args, "--direction")?,
            "--min-amount" => {
                min_amount = parse_finite_f64_arg(
                    &crate::required_value(&mut args, "--min-amount")?,
                    "--min-amount",
                )?
            }
            "--date-start" => date_start = crate::required_value(&mut args, "--date-start")?,
            "--date-end-excl" => {
                date_end_excl = crate::required_value(&mut args, "--date-end-excl")?
            }
            other => bail!("unknown argument: {other}"),
        }
    }
    require_case_id_and_db_path(&case_id, &db_path)?;
    Ok(QueryFlowFocusRowsArgs {
        case_id,
        db_path,
        query_seed_ids,
        selected_focus_ids,
        selected_placeholder_kinds,
        include_missing_counterparty,
        direction,
        min_amount,
        date_start,
        date_end_excl,
    })
}

pub(crate) fn parse_query_flow_focus_graph_args(
    iter: impl Iterator<Item = String>,
) -> Result<QueryFlowFocusGraphArgs> {
    let mut rows = QueryFlowFocusRowsArgs {
        case_id: String::new(),
        db_path: PathBuf::new(),
        query_seed_ids: Vec::new(),
        selected_focus_ids: Vec::new(),
        selected_placeholder_kinds: Vec::new(),
        include_missing_counterparty: false,
        direction: String::from("all"),
        min_amount: 0.0,
        date_start: String::new(),
        date_end_excl: String::new(),
    };
    let mut seed_ids = Vec::new();
    let mut depth = 1_i64;
    let mut view_mode = String::from("relation");
    let mut focus_id_raw = String::new();
    let mut focus_label = String::new();
    let mut request_id = String::new();
    let mut source = String::new();
    let mut focus_key_type = String::new();
    let mut expected_total_amount = None;
    let mut expected_row_count = None;
    let mut output_json = None;
    let mut args = iter.peekable();
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--case-id" => rows.case_id = crate::required_value(&mut args, "--case-id")?,
            "--db-path" => {
                rows.db_path = PathBuf::from(crate::required_value(&mut args, "--db-path")?)
            }
            "--query-seed-id" => {
                let value = crate::required_value(&mut args, "--query-seed-id")?;
                if !value.trim().is_empty() {
                    rows.query_seed_ids.push(value);
                }
            }
            "--seed-id" => {
                let value = crate::required_value(&mut args, "--seed-id")?;
                if !value.trim().is_empty() {
                    seed_ids.push(value);
                }
            }
            "--selected-focus-id" => {
                let value = crate::required_value(&mut args, "--selected-focus-id")?;
                if !value.trim().is_empty() {
                    rows.selected_focus_ids.push(value);
                }
            }
            "--selected-placeholder-kind" => {
                let value = crate::required_value(&mut args, "--selected-placeholder-kind")?;
                if let Some(kind) = normalize_placeholder_kind(&value) {
                    rows.selected_placeholder_kinds.push(kind.to_string());
                }
            }
            "--include-missing-counterparty" => {
                rows.include_missing_counterparty = crate::parse_bool(&crate::required_value(
                    &mut args,
                    "--include-missing-counterparty",
                )?)
            }
            "--direction" => rows.direction = crate::required_value(&mut args, "--direction")?,
            "--min-amount" => {
                rows.min_amount = parse_finite_f64_arg(
                    &crate::required_value(&mut args, "--min-amount")?,
                    "--min-amount",
                )?
            }
            "--date-start" => rows.date_start = crate::required_value(&mut args, "--date-start")?,
            "--date-end-excl" => {
                rows.date_end_excl = crate::required_value(&mut args, "--date-end-excl")?
            }
            "--depth" => {
                depth = crate::required_value(&mut args, "--depth")?
                    .parse()
                    .context("--depth must be numeric")?
            }
            "--view-mode" => view_mode = crate::required_value(&mut args, "--view-mode")?,
            "--focus-id-raw" => focus_id_raw = crate::required_value(&mut args, "--focus-id-raw")?,
            "--focus-label" => focus_label = crate::required_value(&mut args, "--focus-label")?,
            "--request-id" => request_id = crate::required_value(&mut args, "--request-id")?,
            "--source" => source = crate::required_value(&mut args, "--source")?,
            "--focus-key-type" => {
                focus_key_type = crate::required_value(&mut args, "--focus-key-type")?
            }
            "--expected-total-amount" => {
                expected_total_amount = Some(parse_finite_f64_arg(
                    &crate::required_value(&mut args, "--expected-total-amount")?,
                    "--expected-total-amount",
                )?)
            }
            "--expected-row-count" => {
                expected_row_count = Some(
                    crate::required_value(&mut args, "--expected-row-count")?
                        .parse()
                        .context("--expected-row-count must be numeric")?,
                )
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
    require_case_id_and_db_path(&rows.case_id, &rows.db_path)?;
    Ok(QueryFlowFocusGraphArgs {
        rows,
        seed_ids,
        depth: depth.max(1),
        view_mode,
        focus_id_raw,
        focus_label,
        request_id,
        source,
        focus_key_type,
        expected_total_amount,
        expected_row_count,
        output_json,
    })
}
