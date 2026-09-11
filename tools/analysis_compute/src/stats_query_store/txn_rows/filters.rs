use super::super::values::{direction_value, placeholder_kind_from_token_value, sql_literal_list};
use super::request::QueryStatsTxnRowsRequest;

pub(super) fn build_base_where_parts(request: &QueryStatsTxnRowsRequest) -> Vec<String> {
    let mut where_parts = Vec::new();
    if !request.selected.is_empty() {
        where_parts.push(format!(
            "acct_key IN ({})",
            sql_literal_list(&request.selected)
        ));
    }
    if !request.start_time.is_empty() {
        where_parts.push(format!(
            "txn_ts >= CAST({} AS TIMESTAMP)",
            crate::sql_literal(&request.start_time)
        ));
    } else if !request.date_start.is_empty() {
        where_parts.push(format!(
            "txn_day >= CAST({} AS DATE)",
            crate::sql_literal(&request.date_start)
        ));
    }
    if !request.end_time.is_empty() {
        where_parts.push(format!(
            "txn_ts <= CAST({} AS TIMESTAMP)",
            crate::sql_literal(&request.end_time)
        ));
    } else if !request.date_end.is_empty() {
        where_parts.push(format!(
            "txn_day <= CAST({} AS DATE)",
            crate::sql_literal(&request.date_end)
        ));
    }
    if let Some(dir_value) = direction_value(&request.direction) {
        where_parts.push(format!("dc_val = {}", crate::sql_literal(dir_value)));
    }
    push_key_where_parts(&mut where_parts, request);
    where_parts
}

pub(super) fn base_where_sql(where_parts: &[String]) -> String {
    if where_parts.is_empty() {
        "1=1".to_string()
    } else {
        where_parts.join(" AND ")
    }
}

fn push_key_where_parts(target: &mut Vec<String>, request: &QueryStatsTxnRowsRequest) {
    if request.key_type == "name" {
        push_value_filter(
            target,
            "stats_name_key",
            &non_empty_values(&request.key_values),
        );
    } else if request.selected.is_empty() {
        push_nullable_value_filter(target, "acct_key", &request.key_values);
    } else {
        push_counterparty_value_filter(target, &request.key_values);
    }
}

fn non_empty_values(values: &[String]) -> Vec<String> {
    values
        .iter()
        .filter(|value| !value.is_empty())
        .cloned()
        .collect()
}

fn push_value_filter(target: &mut Vec<String>, column: &str, values: &[String]) {
    if values.is_empty() {
        return;
    }
    if values.len() == 1 {
        target.push(format!("{column} = {}", crate::sql_literal(&values[0])));
    } else {
        target.push(format!("{column} IN ({})", sql_literal_list(values)));
    }
}

fn push_nullable_value_filter(target: &mut Vec<String>, column: &str, values: &[String]) {
    let mut parts = Vec::new();
    let normal_values = non_empty_values(values);
    if values.iter().any(|value| value.is_empty()) {
        parts.push(format!("{column} IS NULL"));
    }
    if normal_values.len() == 1 {
        parts.push(format!(
            "{column} = {}",
            crate::sql_literal(&normal_values[0])
        ));
    } else if normal_values.len() > 1 {
        parts.push(format!(
            "{column} IN ({})",
            sql_literal_list(&normal_values)
        ));
    }
    push_combined_filter(target, parts);
}

fn push_counterparty_value_filter(target: &mut Vec<String>, values: &[String]) {
    let mut parts = Vec::new();
    let mut placeholder_kinds = Vec::new();
    let mut normal_values = Vec::new();
    let mut has_empty = false;

    for value in values {
        if value.is_empty() {
            has_empty = true;
        } else if let Some(kind) = placeholder_kind_from_token_value(value) {
            if !placeholder_kinds.iter().any(|item| item == kind) {
                placeholder_kinds.push(kind.to_string());
            }
        } else if !normal_values.iter().any(|item| item == value) {
            normal_values.push(value.clone());
        }
    }

    if has_empty {
        parts.push("cp_key IS NULL".to_string());
    }
    if normal_values.len() == 1 {
        parts.push(format!(
            "cp_key = {}",
            crate::sql_literal(&normal_values[0])
        ));
    } else if normal_values.len() > 1 {
        parts.push(format!("cp_key IN ({})", sql_literal_list(&normal_values)));
    }
    if placeholder_kinds.len() == 1 {
        parts.push(format!(
            "cp_placeholder_kind = {}",
            crate::sql_literal(&placeholder_kinds[0])
        ));
    } else if placeholder_kinds.len() > 1 {
        parts.push(format!(
            "cp_placeholder_kind IN ({})",
            sql_literal_list(&placeholder_kinds)
        ));
    }

    push_combined_filter(target, parts);
}

fn push_combined_filter(target: &mut Vec<String>, parts: Vec<String>) {
    match parts.len() {
        0 => {}
        1 => target.push(parts[0].clone()),
        _ => target.push(format!("({})", parts.join(" OR "))),
    }
}

#[cfg(test)]
mod tests {
    use serde_json::Value;
    use std::path::PathBuf;

    use super::super::super::args::QueryStatsTxnRowsArgs;
    use super::super::request::QueryStatsTxnRowsRequest;
    use super::*;

    fn request(
        key_type: &str,
        key_value: &str,
        selected_keys: Vec<&str>,
    ) -> QueryStatsTxnRowsRequest {
        QueryStatsTxnRowsRequest::from_args(&QueryStatsTxnRowsArgs {
            case_id: "case-1".to_string(),
            db_path: PathBuf::from("case.duckdb"),
            key_type: key_type.to_string(),
            key_value: key_value.to_string(),
            key_values: Vec::new(),
            selected_keys: selected_keys.into_iter().map(str::to_string).collect(),
            date_start: "2026-01-01".to_string(),
            date_end: "2026-12-31".to_string(),
            start_time: String::new(),
            end_time: String::new(),
            direction: "all".to_string(),
            sort_col: "txn_time".to_string(),
            sort_dir: "asc".to_string(),
            limit: 200,
            cursor: None::<Value>,
            row_format: String::new(),
            fields: Vec::new(),
            output_json: None,
        })
        .expect("valid txn rows request")
    }

    #[test]
    fn base_where_pushes_counterparty_key_filter_for_selected_accounts() {
        let sql = base_where_sql(&build_base_where_parts(&request(
            "account",
            "CP-001",
            vec!["CARD-001"],
        )));

        assert!(sql.contains("acct_key IN ('CARD-001')"));
        assert!(sql.contains("cp_key = 'CP-001'"));
        assert!(!sql.contains("base.cp_key"));
    }

    #[test]
    fn base_where_pushes_account_key_filter_without_selected_accounts() {
        let sql = base_where_sql(&build_base_where_parts(&request(
            "account",
            "CARD-001",
            Vec::new(),
        )));

        assert!(sql.contains("acct_key = 'CARD-001'"));
    }

    #[test]
    fn base_where_pushes_name_key_filter() {
        let sql = base_where_sql(&build_base_where_parts(&request(
            "name",
            "张三",
            vec!["CARD-001"],
        )));

        assert!(sql.contains("stats_name_key = '张三'"));
    }

    #[test]
    fn base_where_batches_counterparty_key_filters_for_selected_accounts() {
        let mut request = request("account", "", vec!["CARD-001"]);
        request.key_values = vec!["CP-001".to_string(), "CP-002".to_string()];
        let sql = base_where_sql(&build_base_where_parts(&request));

        assert!(sql.contains("acct_key IN ('CARD-001')"));
        assert!(sql.contains("cp_key IN ('CP-001','CP-002')"));
        assert!(!sql.contains("cp_key = 'CP-001'"));
    }

    #[test]
    fn base_where_combines_empty_placeholder_and_normal_counterparty_filters() {
        let mut request = request("account", "", vec!["CARD-001"]);
        request.key_values = vec![
            String::new(),
            "__cp_placeholder__::empty".to_string(),
            "CP-001".to_string(),
        ];
        let sql = base_where_sql(&build_base_where_parts(&request));

        assert!(
            sql.contains("(cp_key IS NULL OR cp_key = 'CP-001' OR cp_placeholder_kind = 'empty')")
        );
    }
}
