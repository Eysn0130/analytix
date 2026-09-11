use super::super::values::{direction_value, sql_literal_list};
use super::request::FlowFocusRowsRequest;

pub(super) fn build_flow_focus_where_sql(request: &FlowFocusRowsRequest) -> String {
    let mut filters = Vec::new();
    filters.push(format!(
        "acct_key IN ({})",
        sql_literal_list(&request.seeds)
    ));
    filters.push("dc_val IN ('进','出')".to_string());
    if !request.date_start.is_empty() {
        filters.push(format!(
            "txn_day >= CAST({} AS DATE)",
            crate::sql_literal(&request.date_start)
        ));
    }
    if !request.date_end_excl.is_empty() {
        filters.push(format!(
            "txn_day < CAST({} AS DATE)",
            crate::sql_literal(&request.date_end_excl)
        ));
    }

    filters.push(build_focus_match_sql(request));
    if let Some(dir_value) = direction_value(&request.direction) {
        filters.push(format!("dc_val = {}", crate::sql_literal(dir_value)));
    }
    filters.join(" AND ")
}

fn build_focus_match_sql(request: &FlowFocusRowsRequest) -> String {
    let mut focus_match_parts = Vec::new();
    if !request.focus_ids.is_empty() {
        focus_match_parts.push(format!(
            "(cp_placeholder_kind IS NULL AND cp_key IN ({}))",
            sql_literal_list(&request.focus_ids)
        ));
    }
    if !request.placeholder_kinds.is_empty() {
        focus_match_parts.push(format!(
            "(cp_placeholder_kind IN ({}))",
            sql_literal_list(&request.placeholder_kinds)
        ));
    }
    format!("({})", focus_match_parts.join(" OR "))
}
