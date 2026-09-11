use super::super::dedupe_sql::{deduped_source_ctes, detail_stats_dedupe_key_expr};
use super::request::QueryStatsRowsRequest;

pub(super) fn build_stats_rows_agg_sql(
    request: &QueryStatsRowsRequest,
    where_sql: &str,
    has_account_dim: bool,
    has_doc_status: bool,
) -> String {
    let source_sql = build_detail_source_sql(request.group_key, where_sql);
    let base_sql = format!(
        "WITH {} ",
        deduped_source_ctes(&source_sql, request.selected.len() > 1 && has_account_dim)
    );

    let select = build_group_select(request.group_key);
    let select_cols = format!(
        "CAST({} AS VARCHAR) AS key_value, \
         {} AS counterparty_account, \
         {} AS counterparty_name, \
         MAX(counterparty_bank) AS bank, \
         MAX(location) AS location, \
         {} AS placeholder_kind, \
         {} AS key_label, \
         CAST(SUM(CASE WHEN dc_val='进' THEN amt_sum ELSE 0 END) AS DOUBLE) AS in_amount, \
         CAST(SUM(CASE WHEN dc_val='出' THEN amt_sum ELSE 0 END) AS DOUBLE) AS out_amount, \
         CAST(SUM(CASE WHEN dc_val='进' THEN txn_count ELSE 0 END) AS BIGINT) AS in_count, \
         CAST(SUM(CASE WHEN dc_val='出' THEN txn_count ELSE 0 END) AS BIGINT) AS out_count, \
         MIN(first_ts) AS first_time, \
         MAX(last_ts) AS last_time",
        select.group_key_expr,
        select.counterparty_account_expr,
        select.counterparty_name_expr,
        select.placeholder_kind_expr,
        select.key_label_expr
    );
    let doc_ctes = build_doc_status_ctes(request, has_account_dim, has_doc_status);
    let doc_joins = build_doc_status_joins(has_account_dim, has_doc_status);
    let doc_expr = build_doc_label_expr("grouped.key_value", has_account_dim, has_doc_status);
    format!(
        "{base_sql}, grouped AS (\
           SELECT {select_cols} FROM filtered WHERE {} GROUP BY {}\
         ){doc_ctes} \
         SELECT grouped.key_value AS key_value, grouped.counterparty_account AS counterparty_account, \
                grouped.counterparty_name AS counterparty_name, grouped.bank AS bank, \
                grouped.location AS location, grouped.placeholder_kind AS placeholder_kind, \
                grouped.key_label AS key_label, grouped.in_amount AS in_amount, \
                grouped.out_amount AS out_amount, grouped.in_count AS in_count, \
                grouped.out_count AS out_count, CAST(grouped.first_time AS VARCHAR) AS first_time, \
                CAST(grouped.last_time AS VARCHAR) AS last_time, {doc_expr} AS doc \
           FROM grouped{doc_joins}",
        select.group_filter, select.group_key_expr
    )
}

fn build_detail_source_sql(group_key: &str, where_sql: &str) -> String {
    let base_columns = build_base_columns(group_key);
    format!(
        "SELECT {base_columns}, \
                {} AS stats_dedupe_key \
           FROM {} d \
          WHERE {where_sql}",
        detail_stats_dedupe_key_expr("d"),
        crate::DETAIL_TABLE
    )
}

fn build_base_columns(group_key: &str) -> &'static str {
    if group_key == "cp_key" {
        return "id, acct_key, cp_key, \
                COALESCE(NULLIF(counterparty_acct, ''), NULLIF(cp_raw, ''), cp_key) AS cp_display, \
                cp_placeholder_kind, cp_name, cp_name_pick, cp_name_pick_cnt, dc_val, \
                CAST(1 AS BIGINT) AS txn_count, ABS(amount) AS amt_sum, \
                txn_ts AS first_ts, txn_ts AS last_ts, counterparty_bank, location";
    }
    "id, acct_key, stats_name_key, \
     COALESCE(NULLIF(counterparty_acct, ''), NULLIF(cp_raw, ''), cp_key) AS cp_display, \
     dc_val, CAST(1 AS BIGINT) AS txn_count, ABS(amount) AS amt_sum, \
     txn_ts AS first_ts, txn_ts AS last_ts, counterparty_bank, location"
}

struct GroupSelect {
    group_key_expr: String,
    group_filter: String,
    placeholder_kind_expr: String,
    key_label_expr: String,
    counterparty_account_expr: String,
    counterparty_name_expr: String,
}

fn build_group_select(group_key: &str) -> GroupSelect {
    if group_key == "cp_key" {
        let group_key_expr = format!(
            "CASE WHEN cp_placeholder_kind IS NOT NULL THEN {} ELSE cp_key END",
            crate::placeholder_token_sql("cp_placeholder_kind")
        );
        return GroupSelect {
            group_filter: "1=1".to_string(),
            placeholder_kind_expr: "MAX(cp_placeholder_kind)".to_string(),
            counterparty_account_expr: format!(
                "CASE WHEN MAX(cp_placeholder_kind) IS NOT NULL THEN {} ELSE MAX(cp_display) END",
                crate::placeholder_label_sql("MAX(cp_placeholder_kind)")
            ),
            key_label_expr: format!(
                "CASE WHEN MAX(cp_placeholder_kind) IS NOT NULL THEN {} ELSE CAST({group_key_expr} AS VARCHAR) END",
                crate::placeholder_label_sql("MAX(cp_placeholder_kind)")
            ),
            counterparty_name_expr:
                "CASE WHEN COALESCE(MAX(cp_name_pick_cnt),0) >= 3 THEN MAX(cp_name_pick) ELSE MAX(cp_name) END"
                    .to_string(),
            group_key_expr,
        };
    }

    GroupSelect {
        group_key_expr: "stats_name_key".to_string(),
        group_filter: "stats_name_key IS NOT NULL AND TRIM(CAST(stats_name_key AS VARCHAR)) <> ''"
            .to_string(),
        placeholder_kind_expr: "NULL".to_string(),
        key_label_expr: "CAST(stats_name_key AS VARCHAR)".to_string(),
        counterparty_account_expr: "MAX(cp_display)".to_string(),
        counterparty_name_expr: "MAX(stats_name_key)".to_string(),
    }
}

fn build_doc_status_ctes(
    request: &QueryStatsRowsRequest,
    has_account_dim: bool,
    has_doc_status: bool,
) -> String {
    let mut ctes = Vec::new();
    if has_account_dim {
        ctes.push(if request.group_key == "cp_key" {
            format!(
                "resolved_doc_keys AS (\
                   SELECT DISTINCT CAST(account_key AS VARCHAR) AS key_value \
                     FROM {account_dim}\
                 )",
                account_dim = crate::ACCOUNT_DIM_TABLE
            )
        } else {
            format!(
                "resolved_doc_keys AS (\
                   SELECT DISTINCT CAST(doc_agg.stats_name_key AS VARCHAR) AS key_value \
                     FROM {agg_table} doc_agg \
                     JOIN {account_dim} account_doc ON account_doc.account_key = doc_agg.cp_key \
                    WHERE doc_agg.stats_name_key IS NOT NULL \
                      AND TRIM(CAST(doc_agg.stats_name_key AS VARCHAR)) <> ''\
                 )",
                agg_table = crate::AGG_TABLE,
                account_dim = crate::ACCOUNT_DIM_TABLE
            )
        });
    }
    if has_doc_status {
        let key_type = if request.group_key == "cp_key" {
            "account"
        } else {
            "name"
        };
        ctes.push(format!(
            "pending_doc_keys AS (\
               SELECT DISTINCT CAST(key_value AS VARCHAR) AS key_value \
                 FROM analysis_doc_status \
                WHERE case_id = {case_id} \
                  AND key_type = {key_type} \
                  AND status = 'pending'\
             )",
            case_id = crate::sql_literal(&request.case_id),
            key_type = crate::sql_literal(key_type),
        ));
    }
    if ctes.is_empty() {
        String::new()
    } else {
        format!(", {}", ctes.join(", "))
    }
}

fn build_doc_status_joins(has_account_dim: bool, has_doc_status: bool) -> String {
    let mut joins = Vec::new();
    if has_account_dim {
        joins.push(
            " LEFT JOIN resolved_doc_keys resolved_doc \
                ON resolved_doc.key_value = CAST(grouped.key_value AS VARCHAR)"
                .to_string(),
        );
    }
    if has_doc_status {
        joins.push(
            " LEFT JOIN pending_doc_keys pending_doc \
                ON pending_doc.key_value = CAST(grouped.key_value AS VARCHAR)"
                .to_string(),
        );
    }
    joins.join("")
}

fn build_doc_label_expr(
    group_key_expr: &str,
    has_account_dim: bool,
    has_doc_status: bool,
) -> String {
    let key_value_expr = format!("CAST({group_key_expr} AS VARCHAR)");
    let resolved_expr = if has_account_dim {
        "resolved_doc.key_value IS NOT NULL".to_string()
    } else {
        "FALSE".to_string()
    };
    let pending_expr = if has_doc_status {
        "pending_doc.key_value IS NOT NULL".to_string()
    } else {
        "FALSE".to_string()
    };
    format!(
        "CASE \
           WHEN COALESCE({key_value_expr}, '') <> '' AND {resolved_expr} THEN '已调单' \
           WHEN COALESCE({key_value_expr}, '') <> '' AND {pending_expr} THEN '待调单' \
           ELSE '未调单' \
         END"
    )
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;

    use super::super::super::args::QueryStatsRowsArgs;
    use super::super::request::QueryStatsRowsRequest;
    use super::*;

    fn request(mode: &str) -> QueryStatsRowsRequest {
        request_with_selected(mode, vec!["CARD-001".to_string()])
    }

    fn request_with_selected(mode: &str, selected_keys: Vec<String>) -> QueryStatsRowsRequest {
        QueryStatsRowsRequest::from_args(&QueryStatsRowsArgs {
            case_id: "case-1".to_string(),
            db_path: PathBuf::from("case.duckdb"),
            mode: mode.to_string(),
            selected_keys,
            date_start: String::new(),
            date_end: String::new(),
            search_text: String::new(),
            row_sort_col: String::new(),
            row_sort_dir: "desc".to_string(),
            row_offset: 0,
            row_limit: 0,
            row_format: String::new(),
            fields: Vec::new(),
            output_json: None,
        })
    }

    #[test]
    fn stats_rows_doc_status_uses_joined_doc_key_ctes_for_account_mode() {
        let sql = build_stats_rows_agg_sql(&request("inAccount"), "1=1", true, true);

        assert!(sql.contains("resolved_doc_keys AS"));
        assert!(sql.contains("pending_doc_keys AS"));
        assert!(sql.contains("LEFT JOIN resolved_doc_keys resolved_doc"));
        assert!(sql.contains("LEFT JOIN pending_doc_keys pending_doc"));
        assert!(sql.contains("resolved_doc.key_value IS NOT NULL"));
        assert!(!sql.contains("EXISTS ("));
    }

    #[test]
    fn stats_rows_doc_status_preserves_name_mode_resolved_keys() {
        let sql = build_stats_rows_agg_sql(&request("inName"), "1=1", true, false);

        assert!(sql.contains("doc_agg.stats_name_key"));
        assert!(sql.contains(crate::AGG_TABLE));
        assert!(sql.contains(crate::ACCOUNT_DIM_TABLE));
        assert!(sql.contains("LEFT JOIN resolved_doc_keys resolved_doc"));
        assert!(!sql.contains("pending_doc_keys AS"));
    }

    #[test]
    fn stats_rows_base_projection_matches_group_mode() {
        let account_sql = build_stats_rows_agg_sql(&request("inAccount"), "1=1", false, false);
        assert!(account_sql.contains("FROM analysis_txn_detail_idx d"));
        assert!(account_sql.contains("id, acct_key, cp_key"));
        assert!(account_sql.contains("COALESCE(NULLIF(counterparty_acct, '')"));
        assert!(account_sql.contains("cp_placeholder_kind"));
        assert!(!account_sql.contains("id, acct_key, stats_name_key"));

        let name_sql = build_stats_rows_agg_sql(&request("inName"), "1=1", false, false);
        assert!(name_sql.contains("id, acct_key, stats_name_key"));
        assert!(!name_sql.contains("cp_placeholder_kind, cp_name"));
    }

    #[test]
    fn stats_rows_agg_projects_display_time_strings() {
        let sql = build_stats_rows_agg_sql(&request("inAccount"), "1=1", false, false);

        assert!(sql.contains("CAST(grouped.first_time AS VARCHAR) AS first_time"));
        assert!(sql.contains("CAST(grouped.last_time AS VARCHAR) AS last_time"));
    }

    #[test]
    fn stats_rows_dedupes_only_multi_account_selection() {
        let single = build_stats_rows_agg_sql(&request("outName"), "1=1", true, false);
        assert!(!single.contains("signature_stats AS"));

        let without_account_dim = build_stats_rows_agg_sql(
            &request_with_selected(
                "outName",
                vec!["CARD-001".to_string(), "CARD-002".to_string()],
            ),
            "1=1",
            false,
            false,
        );
        assert!(!without_account_dim.contains("signature_stats AS"));

        let multi = build_stats_rows_agg_sql(
            &request_with_selected(
                "outName",
                vec!["CARD-001".to_string(), "CARD-002".to_string()],
            ),
            "1=1",
            true,
            false,
        );
        assert!(multi.contains("signature_stats AS"));
        assert!(multi.contains("COUNT(DISTINCT acct_key)"));
        assert!(multi.contains("stats_dedupe_key"));
        assert!(!multi.contains("stats_dedupe_bank"));
        assert!(!multi.contains("stats_dedupe_acct_type"));
    }
}
