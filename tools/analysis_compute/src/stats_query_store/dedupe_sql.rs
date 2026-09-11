pub(super) fn detail_stats_dedupe_key_expr(alias: &str) -> String {
    let counterparty_account =
        coalesced_text_part(alias, &["counterparty_acct", "cp_raw", "cp_key"]);
    // Replacement-card duplicates should differ only by the account/card
    // identifiers. Keep source file identity out, but require all transaction
    // facts and channel facts to match exactly.
    let parts = [
        text_part(alias, "account_open_name"),
        text_part(alias, "opener_id_no"),
        text_part(alias, "dc_val"),
        format!("COALESCE(CAST({alias}.txn_ts AS VARCHAR), '<null>')"),
        numeric_state_part(alias, "amount", true),
        numeric_state_part(alias, "balance", false),
        numeric_state_part(alias, "counterparty_balance", false),
        counterparty_account,
        text_part(alias, "counterparty_name"),
        text_part(alias, "counterparty_id_no"),
        text_part(alias, "counterparty_bank"),
        text_part(alias, "is_success"),
        text_part(alias, "summary"),
        text_part(alias, "remark"),
        text_part(alias, "txn_type"),
        text_part(alias, "currency"),
        text_part(alias, "branch_name"),
        text_part(alias, "branch_code"),
        text_part(alias, "location"),
        text_part(alias, "cash_flag"),
        text_part(alias, "voucher_no"),
        text_part(alias, "terminal_no"),
        text_part(alias, "ip_addr"),
        text_part(alias, "mac_addr"),
        text_part(alias, "txn_id"),
        text_part(alias, "log_id"),
        text_part(alias, "voucher_type"),
        text_part(alias, "voucher_id"),
        text_part(alias, "teller_no"),
        text_part(alias, "merchant_name"),
        text_part(alias, "merchant_no"),
        text_part(alias, "query_feedback_reason"),
    ];
    format!(
        "CASE \
           WHEN NULLIF(TRIM(COALESCE({alias}.account_open_name, '')), '') IS NOT NULL \
            AND NULLIF(TRIM(COALESCE({alias}.opener_id_no, '')), '') IS NOT NULL \
            AND NULLIF(TRIM(COALESCE({alias}.dc_val, '')), '') IS NOT NULL \
            AND {alias}.txn_ts IS NOT NULL \
            AND {alias}.balance IS NOT NULL \
            AND isfinite({alias}.balance) \
            AND {alias}.amount IS NOT NULL \
            AND isfinite({alias}.amount) \
           THEN {} \
           ELSE NULL \
         END",
        parts.join(" || '|#|' || ")
    )
}

pub(super) fn deduped_source_ctes(source_sql: &str, dedupe_enabled: bool) -> String {
    if !dedupe_enabled {
        return format!(
            "source AS ({source_sql}), \
             filtered AS (SELECT * FROM source)"
        );
    }
    let account_dim_table = crate::ACCOUNT_DIM_TABLE;
    let detail_table = crate::DETAIL_TABLE;
    format!(
        "source AS ({source_sql}), \
         account_latest AS (\
           SELECT acct_key, MAX(txn_ts) AS acct_last_ts, MAX(id) AS acct_last_id \
             FROM {detail_table} \
            GROUP BY acct_key\
         ), \
         family_source AS (\
           SELECT source.*, \
                  COALESCE(NULLIF(TRIM(CAST(account_dim.card_display AS VARCHAR)), ''), '') AS stats_dedupe_card_display, \
                  COALESCE(NULLIF(TRIM(CAST(account_dim.acct_display AS VARCHAR)), ''), '') AS stats_dedupe_acct_display, \
                  account_latest.acct_last_ts AS stats_dedupe_acct_last_ts, \
                  account_latest.acct_last_id AS stats_dedupe_acct_last_id \
             FROM source \
             LEFT JOIN {account_dim_table} account_dim \
               ON account_dim.account_key = source.acct_key \
             LEFT JOIN account_latest \
               ON account_latest.acct_key = source.acct_key\
         ), \
         signature_stats AS (\
           SELECT stats_dedupe_key, COUNT(DISTINCT acct_key) AS acct_count \
             FROM family_source \
            WHERE stats_dedupe_key IS NOT NULL \
            GROUP BY stats_dedupe_key \
           HAVING COUNT(DISTINCT acct_key) > 1\
         ), \
         ranked AS (\
           SELECT family_source.*, \
                  CASE WHEN sig.stats_dedupe_key IS NULL THEN NULL ELSE \
                    ROW_NUMBER() OVER (\
                      PARTITION BY family_source.stats_dedupe_key \
                      ORDER BY family_source.stats_dedupe_acct_last_ts DESC NULLS LAST, \
                               family_source.stats_dedupe_acct_last_id DESC NULLS LAST, \
                               family_source.stats_dedupe_card_display DESC, \
                               family_source.stats_dedupe_acct_display DESC, \
                               family_source.acct_key DESC, \
                               family_source.id ASC\
                    ) \
                  END AS stats_dedupe_rank \
             FROM family_source \
             LEFT JOIN signature_stats sig \
               ON sig.stats_dedupe_key = family_source.stats_dedupe_key\
         ), \
         filtered AS (\
           SELECT * \
             FROM ranked \
            WHERE stats_dedupe_rank IS NULL OR stats_dedupe_rank = 1\
         )"
    )
}

fn text_part(alias: &str, column: &str) -> String {
    format!("COALESCE(NULLIF(TRIM(CAST({alias}.{column} AS VARCHAR)), ''), '<null>')")
}

fn coalesced_text_part(alias: &str, columns: &[&str]) -> String {
    let expressions = columns
        .iter()
        .map(|column| format!("NULLIF(TRIM(CAST({alias}.{column} AS VARCHAR)), '')"))
        .collect::<Vec<_>>();
    format!("COALESCE({}, '<null>')", expressions.join(", "))
}

fn numeric_state_part(alias: &str, column: &str, absolute: bool) -> String {
    let value = if absolute {
        format!("ABS({alias}.{column})")
    } else {
        format!("{alias}.{column}")
    };
    format!(
        "CASE WHEN {alias}.{column} IS NULL THEN 'missing' \
         WHEN isfinite({alias}.{column}) THEN 'known:' || CAST(ROUND({value}, 2) AS VARCHAR) \
         ELSE 'invalid' END"
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use duckdb::Connection;

    #[test]
    fn stats_same_fact_dedupe_distinguishes_missing_invalid_and_known_zero() {
        let expression = numeric_state_part("d", "amount", true);
        assert!(expression.contains("THEN 'missing'"));
        assert!(expression.contains("THEN 'known:'"));
        assert!(expression.contains("ELSE 'invalid'"));

        let conn = Connection::open_in_memory().expect("open numeric-state fixture");
        conn.execute_batch(
            "CREATE TABLE detail(amount DOUBLE); \
             INSERT INTO detail VALUES (NULL), (0), ('NaN'::DOUBLE), ('Inf'::DOUBLE);",
        )
        .expect("seed numeric states");
        let mut statement = conn
            .prepare(&format!("SELECT {expression} FROM detail d ORDER BY rowid"))
            .expect("prepare numeric state query");
        let states = statement
            .query_map([], |row| row.get::<_, String>(0))
            .expect("query numeric states")
            .collect::<std::result::Result<Vec<_>, _>>()
            .expect("collect numeric states");
        assert_eq!(states[0], "missing");
        assert!(states[1].starts_with("known:0"));
        assert_eq!(states[2], "invalid");
        assert_eq!(states[3], "invalid");

        let dedupe = detail_stats_dedupe_key_expr("d");
        assert!(!dedupe.contains("COALESCE(d.amount, 0)"));
        assert!(dedupe.contains("d.amount IS NOT NULL"));
        assert!(dedupe.contains("isfinite(d.amount)"));
    }
}
