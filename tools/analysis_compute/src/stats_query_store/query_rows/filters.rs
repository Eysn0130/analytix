use super::super::values::sql_literal_list;
use super::request::QueryStatsRowsRequest;

pub(super) fn build_source_where_sql(request: &QueryStatsRowsRequest) -> String {
    let mut where_parts = Vec::new();
    where_parts.push(format!(
        "acct_key IN ({})",
        sql_literal_list(&request.selected)
    ));
    if !request.date_start.is_empty() {
        where_parts.push(format!(
            "txn_day >= CAST({} AS DATE)",
            crate::sql_literal(&request.date_start)
        ));
    }
    if !request.date_end.is_empty() {
        where_parts.push(format!(
            "txn_day <= CAST({} AS DATE)",
            crate::sql_literal(&request.date_end)
        ));
    }
    where_parts.join(" AND ")
}

pub(super) fn build_search_where_sql(search_text: &str) -> String {
    if search_text.is_empty() {
        return "1=1".to_string();
    }
    let like = crate::sql_literal(&format!("%{search_text}%"));
    format!(
        "LOWER(COALESCE(counterparty_account, '')) LIKE {like} \
         OR LOWER(COALESCE(counterparty_name, '')) LIKE {like} \
         OR LOWER(COALESCE(bank, '')) LIKE {like} \
         OR LOWER(COALESCE(location, '')) LIKE {like} \
         OR LOWER(COALESCE(key_label, CAST(key_value AS VARCHAR), '')) LIKE {like}"
    )
}
