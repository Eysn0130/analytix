pub(super) fn build_flow_focus_sql(where_sql: &str, include_missing_counterparty: bool) -> String {
    let cp_key_norm_expr = build_cp_key_norm_expr(include_missing_counterparty);
    format!(
        "WITH base AS (\
           SELECT acct_key, cp_key, cp_raw AS cp_key_raw, cp_placeholder_kind, dc_val, \
                  txn_count, amt_sum, first_ts, last_ts, open_name, cp_name, cp_name_pick \
             FROM {} \
            WHERE {where_sql}\
         ) \
         SELECT \
           acct_key, {cp_key_norm_expr} AS cp_key, cp_key_raw, dc_val, \
           CAST(SUM(txn_count) AS BIGINT) AS txn_count, \
           CAST(SUM(amt_sum) AS DOUBLE) AS amt_sum, \
           CAST(MIN(first_ts) AS VARCHAR) AS first_ts, \
           CAST(MAX(last_ts) AS VARCHAR) AS last_ts, \
           MAX(open_name) AS open_name, \
           COALESCE(MAX(cp_name_pick), MAX(cp_name)) AS cp_name \
         FROM base \
         GROUP BY acct_key, {cp_key_norm_expr}, cp_key, cp_key_raw, dc_val \
         ORDER BY amt_sum DESC",
        crate::AGG_TABLE
    )
}

fn build_cp_key_norm_expr(include_missing_counterparty: bool) -> String {
    if !include_missing_counterparty {
        return "cp_key".to_string();
    }
    let unknown_prefix = "__unknown_cp__name::";
    let unknown_cp_id_empty = format!("{unknown_prefix}__empty__");
    format!(
        "CASE \
         WHEN cp_placeholder_kind IS NOT NULL AND cp_name IS NOT NULL THEN {} || '::name::' || cp_name \
         WHEN cp_placeholder_kind IS NOT NULL THEN {} \
         WHEN cp_key IS NOT NULL THEN cp_key \
         WHEN cp_name IS NOT NULL THEN {} || cp_name \
         ELSE {} END",
        crate::placeholder_token_sql("cp_placeholder_kind"),
        crate::placeholder_token_sql("cp_placeholder_kind"),
        crate::sql_literal(unknown_prefix),
        crate::sql_literal(&unknown_cp_id_empty)
    )
}
