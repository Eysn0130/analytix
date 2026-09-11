use anyhow::{bail, Result};
use serde_json::Value;

use super::super::args::QueryStatsTxnRowsArgs;
use super::super::values::{
    cursor_bool, cursor_f64, cursor_i64, cursor_string, normalize_key_type,
};

pub(super) struct QueryStatsTxnRowsRequest {
    pub(super) selected: Vec<String>,
    pub(super) key_type: &'static str,
    pub(super) key_values: Vec<String>,
    pub(super) date_start: String,
    pub(super) date_end: String,
    pub(super) start_time: String,
    pub(super) end_time: String,
    pub(super) direction: String,
    pub(super) sort_col: &'static str,
    sort_dir: &'static str,
    pub(super) limit: i64,
    pub(super) cursor: Option<Value>,
    pub(super) row_format: String,
    pub(super) fields: Vec<String>,
}

impl QueryStatsTxnRowsRequest {
    pub(super) fn from_args(args: &QueryStatsTxnRowsArgs) -> Result<Self> {
        let sort_dir = args.sort_dir.trim().to_lowercase();
        let sort_dir = if sort_dir == "-1" || sort_dir == "desc" || sort_dir == "down" {
            "desc"
        } else {
            "asc"
        };
        let sort_col = if args.sort_col.trim() == "amount" {
            "amount"
        } else {
            "txn_time"
        };
        validate_cursor(args.cursor.as_ref(), sort_col)?;
        let key_values = normalize_key_values(args);

        Ok(Self {
            selected: args
                .selected_keys
                .iter()
                .map(|item| item.trim().to_string())
                .filter(|item| !item.is_empty())
                .collect::<Vec<_>>(),
            key_type: normalize_key_type(&args.key_type),
            key_values,
            date_start: args.date_start.trim().to_string(),
            date_end: args.date_end.trim().to_string(),
            start_time: args.start_time.trim().to_string(),
            end_time: args.end_time.trim().to_string(),
            direction: args.direction.trim().to_string(),
            sort_col,
            sort_dir,
            limit: args.limit.max(0),
            cursor: args.cursor.clone(),
            row_format: args.row_format.trim().to_string(),
            fields: args
                .fields
                .iter()
                .map(|item| item.trim().to_string())
                .filter(|item| !item.is_empty())
                .collect(),
        })
    }

    pub(super) fn should_return_empty(&self) -> bool {
        let has_key_value = self.key_values.iter().any(|item| !item.is_empty());
        (self.selected.is_empty() && (self.key_type == "name" || !has_key_value))
            || (self.key_type == "name" && !has_key_value)
    }

    pub(super) fn dir_sql(&self) -> &'static str {
        if self.sort_dir == "desc" {
            "DESC"
        } else {
            "ASC"
        }
    }

    pub(super) fn order_sql(&self) -> String {
        let dir_sql = self.dir_sql();
        if self.sort_col == "amount" {
            format!(
                "CASE WHEN amount IS NOT NULL AND isfinite(amount) THEN 0 ELSE 1 END ASC, \
                 CASE WHEN amount IS NOT NULL AND isfinite(amount) THEN ABS(amount) END {dir_sql} NULLS LAST, \
                 id {dir_sql}"
            )
        } else {
            format!("txn_ts {dir_sql} NULLS LAST, id {dir_sql}")
        }
    }
}

fn normalize_key_values(args: &QueryStatsTxnRowsArgs) -> Vec<String> {
    let primary = args.key_value.trim();
    let mut source = if args.key_values.is_empty() {
        vec![args.key_value.as_str()]
    } else {
        args.key_values
            .iter()
            .map(String::as_str)
            .collect::<Vec<_>>()
    };
    if !primary.is_empty() && !source.iter().any(|value| value.trim() == primary) {
        source.insert(0, args.key_value.as_str());
    }
    let mut out = Vec::new();
    for value in source {
        let normalized = value.trim().to_string();
        if out.iter().any(|item| item == &normalized) {
            continue;
        }
        out.push(normalized);
    }
    if out.is_empty() {
        out.push(String::new());
    }
    out
}

#[cfg(test)]
mod tests {
    use serde_json::Value;
    use std::path::PathBuf;

    use super::*;

    fn args(sort_dir: &str) -> QueryStatsTxnRowsArgs {
        QueryStatsTxnRowsArgs {
            case_id: "case-1".to_string(),
            db_path: PathBuf::from("case.duckdb"),
            key_type: "account".to_string(),
            key_value: "CARD-001".to_string(),
            key_values: Vec::new(),
            selected_keys: Vec::new(),
            date_start: String::new(),
            date_end: String::new(),
            start_time: String::new(),
            end_time: String::new(),
            direction: "all".to_string(),
            sort_col: "txn_time".to_string(),
            sort_dir: sort_dir.to_string(),
            limit: 200,
            cursor: None::<Value>,
            row_format: String::new(),
            fields: Vec::new(),
            output_json: None,
        }
    }

    #[test]
    fn txn_time_order_uses_native_nulls_last() {
        let asc = QueryStatsTxnRowsRequest::from_args(&args("asc")).expect("asc request");
        let desc = QueryStatsTxnRowsRequest::from_args(&args("desc")).expect("desc request");

        assert_eq!(asc.order_sql(), "txn_ts ASC NULLS LAST, id ASC");
        assert_eq!(desc.order_sql(), "txn_ts DESC NULLS LAST, id DESC");
    }
}

fn validate_cursor(cursor: Option<&Value>, sort_col: &str) -> Result<()> {
    let Some(cursor) = cursor else {
        return Ok(());
    };
    if !cursor.is_object() {
        bail!("--cursor-json must be a JSON object");
    }
    if cursor_i64(cursor, "id").is_none() {
        bail!("--cursor-json requires numeric id");
    }
    if sort_col == "amount" {
        let abs_null = cursor_bool(cursor, "absNull");
        let abs = cursor_f64(cursor, "abs");
        if abs_null && cursor.get("abs").is_some() {
            bail!("--cursor-json amount cursor cannot combine abs with absNull=true");
        }
        if abs.is_some_and(|value| value < 0.0) {
            bail!("--cursor-json abs must be non-negative");
        }
        if !abs_null && abs.is_none() {
            bail!("--cursor-json requires numeric abs for amount sorting");
        }
        return Ok(());
    }

    let ts = cursor_string(cursor, "ts").trim();
    if ts.is_empty() && !cursor_bool(cursor, "tsNull") {
        bail!("--cursor-json requires ts or tsNull=true for txn_time sorting");
    }
    Ok(())
}
