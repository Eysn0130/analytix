use anyhow::{bail, Result};
use duckdb::Connection;

pub(super) fn create_rule_txn_index_staging(
    conn: &Connection,
    case_id: &str,
    cols: &[String],
) -> Result<()> {
    let clean_acceptance_expr = crate::clean_row_acceptance_expr("t", cols)
        .ok_or_else(|| anyhow::anyhow!("rule transaction cleaning authority is unavailable"))?;
    let invalid_clean_state_count = crate::scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(1) FROM fc_transaction_norm t WHERE t.case_id={} AND (\
             t.clean_invalid IS NULL OR t.clean_invalid NOT IN (0,1) OR \
             t.clean_failed IS NULL OR t.clean_failed NOT IN (0,1) OR \
             t.clean_reversal IS NULL OR t.clean_reversal NOT IN (0,1))",
            crate::sql_literal(case_id)
        ),
    )?;
    if invalid_clean_state_count != 0 {
        bail!("rule transaction cleaning state is unresolved");
    }
    let account_key_expr = crate::first_text_expr(
        "t",
        cols,
        &[
            "clean_acct_no",
            "acct_no_norm",
            "acct_no",
            "clean_card_no",
            "card_no_norm",
            "card_no",
        ],
    )
    .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    if account_key_expr == crate::NULL_TEXT {
        bail!("rule transaction account lineage is unavailable");
    }

    let account_name_expr = crate::first_text_expr("t", cols, &["account_open_name"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let txn_id_expr = crate::first_text_expr("t", cols, &["txn_id"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let row_id_expr = if crate::has_col(cols, "id") {
        "CAST(t.id AS VARCHAR)".to_string()
    } else {
        crate::NULL_TEXT.to_string()
    };
    let txn_ts_expr = crate::txn_ts_expr("t", cols);
    let txn_time_expr = if txn_ts_expr != crate::NULL_TIMESTAMP {
        format!("CAST({txn_ts_expr} AS VARCHAR)")
    } else {
        crate::first_text_expr("t", cols, &["txn_time"])
            .unwrap_or_else(|| crate::NULL_TEXT.to_string())
    };
    let amount_expr = crate::amount_value_expr("t", cols);
    let balance_expr =
        crate::finite_num_value_expr("t", cols, &["balance_val", "clean_balance", "balance"]);
    let direction_expr =
        crate::first_text_expr("t", cols, &["dc_final", "clean_dc_flag", "dc_flag"])
            .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let success_expr =
        crate::first_text_expr("t", cols, &["success_flag_norm", "is_success", "is_succ"])
            .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let reason_expr = crate::first_text_expr(
        "t",
        cols,
        &["query_feedback_reason_norm", "query_feedback_reason"],
    )
    .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let opener_id_expr = crate::first_text_expr("t", cols, &["opener_id_no"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let ip_expr = crate::first_text_expr("t", cols, &["ip_addr"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let mac_expr = crate::first_text_expr("t", cols, &["mac_addr"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let counterparty_acct_expr =
        crate::first_text_expr("t", cols, &["counterparty_acct_norm", "counterparty_acct"])
            .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let counterparty_name_expr = crate::first_text_expr("t", cols, &["counterparty_name"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let counterparty_id_expr = crate::first_text_expr("t", cols, &["counterparty_id_no"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let counterparty_bank_expr = crate::first_text_expr("t", cols, &["counterparty_bank"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let summary_expr = crate::first_text_expr("t", cols, &["summary"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let currency_expr = crate::first_text_expr("t", cols, &["currency"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let branch_expr = crate::first_text_expr("t", cols, &["branch_name"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let location_expr = crate::first_text_expr("t", cols, &["location", "txn_location"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let cash_expr = crate::first_text_expr("t", cols, &["cash_flag_norm", "is_cash", "cash_flag"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let voucher_no_expr = crate::first_text_expr("t", cols, &["voucher_no"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let receipt_no_expr = crate::first_text_expr("t", cols, &["receipt_no"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let log_no_expr = crate::first_text_expr("t", cols, &["log_no", "log_id"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let voucher_type_expr = crate::first_text_expr("t", cols, &["voucher_type"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let voucher_id_expr = crate::first_text_expr("t", cols, &["voucher_id"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let teller_expr = crate::first_text_expr("t", cols, &["teller_no"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let remark_expr = crate::first_text_expr("t", cols, &["remark"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let txn_type_expr = crate::first_text_expr("t", cols, &["txn_type"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let file_id_expr = crate::first_text_expr("t", cols, &["file_id"])
        .unwrap_or_else(|| crate::NULL_TEXT.to_string());
    let order_sql = if txn_ts_expr != crate::NULL_TIMESTAMP {
        format!(
            "({txn_ts_expr} IS NULL) ASC, {txn_ts_expr} ASC, COALESCE({txn_id_expr}, {row_id_expr}, {account_key_expr}) ASC"
        )
    } else {
        format!("{account_key_expr} ASC, COALESCE({txn_id_expr}, {row_id_expr}) ASC")
    };

    conn.execute_batch(&format!(
        "
        DROP TABLE IF EXISTS {};
        CREATE TABLE {} AS
        SELECT
          t.case_id AS case_id,
          {account_key_expr} AS account_key,
          COALESCE({account_name_expr}, {account_key_expr}) AS account_name,
          {txn_id_expr} AS txn_id,
          {row_id_expr} AS row_id,
          {txn_time_expr} AS txn_time,
          {txn_ts_expr} AS txn_ts_val,
          ROUND(ABS({amount_expr}), 2) AS amount_val,
          ROUND({balance_expr}, 2) AS balance_val,
          {direction_expr} AS direction_raw,
          {success_expr} AS success_raw,
          {reason_expr} AS reason_raw,
          {opener_id_expr} AS opener_id_no,
          {ip_expr} AS ip_addr,
          {mac_expr} AS mac_addr,
          {counterparty_acct_expr} AS counterparty_acct,
          {counterparty_name_expr} AS counterparty_name,
          {counterparty_id_expr} AS counterparty_id_no,
          {counterparty_bank_expr} AS counterparty_bank,
          {summary_expr} AS summary,
          {currency_expr} AS currency,
          {branch_expr} AS branch_name,
          {location_expr} AS location,
          {cash_expr} AS cash_raw,
          {voucher_no_expr} AS voucher_no,
          {receipt_no_expr} AS receipt_no,
          {log_no_expr} AS log_no,
          {voucher_type_expr} AS voucher_type,
          {voucher_id_expr} AS voucher_id,
          {teller_expr} AS teller_no,
          {remark_expr} AS remark,
          {txn_type_expr} AS txn_type,
          {file_id_expr} AS file_id
        FROM fc_transaction_norm t
        WHERE t.case_id={}
          AND {clean_acceptance_expr}
          AND {account_key_expr} IS NOT NULL
        ORDER BY {order_sql}
        ",
        crate::RULE_TXN_INDEX_STAGING_TABLE,
        crate::RULE_TXN_INDEX_STAGING_TABLE,
        crate::sql_literal(case_id)
    ))?;
    Ok(())
}

pub(super) fn swap_rule_txn_index(conn: &Connection) -> Result<()> {
    conn.execute_batch(&format!(
        "
        DROP TABLE IF EXISTS {};
        ALTER TABLE {} RENAME TO {};
        CREATE INDEX IF NOT EXISTS idx_{}_case_time ON {}(case_id, txn_ts_val);
        CREATE INDEX IF NOT EXISTS idx_{}_case_account ON {}(case_id, account_key);
        CREATE INDEX IF NOT EXISTS idx_{}_case_file ON {}(case_id, file_id);
        ",
        crate::RULE_TXN_INDEX_TABLE,
        crate::RULE_TXN_INDEX_STAGING_TABLE,
        crate::RULE_TXN_INDEX_TABLE,
        crate::RULE_TXN_INDEX_TABLE,
        crate::RULE_TXN_INDEX_TABLE,
        crate::RULE_TXN_INDEX_TABLE,
        crate::RULE_TXN_INDEX_TABLE,
        crate::RULE_TXN_INDEX_TABLE,
        crate::RULE_TXN_INDEX_TABLE
    ))?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn columns(conn: &Connection) -> Vec<String> {
        crate::table_columns(conn, "fc_transaction_norm").expect("load transaction columns")
    }

    #[test]
    fn rule_index_missing_values_never_become_known_zero() {
        let conn = Connection::open_in_memory().expect("open fixture");
        conn.execute_batch(
            "CREATE TABLE fc_transaction_norm( \
               id BIGINT, case_id TEXT, acct_no TEXT, clean_amount TEXT, amount TEXT, \
               clean_balance TEXT, balance TEXT, \
               clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER \
             ); \
             INSERT INTO fc_transaction_norm VALUES \
               (1, 'case-a', 'acct-a', NULL, NULL, NULL, NULL, 0, 0, 0), \
               (2, 'case-a', 'acct-a', NULL, '0', NULL, '0', 0, 0, 0), \
               (3, 'case-a', 'acct-a', NULL, 'NaN', NULL, 'Inf', 0, 0, 0), \
               (4, 'case-a', 'acct-a', 'NaN', '123.45', 'Inf', '456.78', 0, 0, 0);",
        )
        .expect("seed normalized transactions");

        create_rule_txn_index_staging(&conn, "case-a", &columns(&conn))
            .expect("materialize rule transaction staging");

        let mut statement = conn
            .prepare(&format!(
                "SELECT amount_val, balance_val FROM {} ORDER BY TRY_CAST(row_id AS BIGINT)",
                crate::RULE_TXN_INDEX_STAGING_TABLE
            ))
            .expect("prepare staging inspection");
        let rows = statement
            .query_map([], |row| {
                Ok((row.get::<_, Option<f64>>(0)?, row.get::<_, Option<f64>>(1)?))
            })
            .expect("query staging inspection")
            .collect::<std::result::Result<Vec<_>, _>>()
            .expect("collect staging inspection");
        assert_eq!(
            rows,
            vec![
                (None, None),
                (Some(0.0), Some(0.0)),
                (None, None),
                (None, None)
            ]
        );
    }

    #[test]
    fn rule_index_rejects_unknown_or_missing_cleaning_authority() {
        for schema_and_row in [
            "CREATE TABLE fc_transaction_norm( \
               id BIGINT, case_id TEXT, acct_no TEXT, amount TEXT, \
               clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER \
             ); \
             INSERT INTO fc_transaction_norm VALUES \
               (1, 'case-a', 'acct-a', '1', 0, NULL, 0);",
            "CREATE TABLE fc_transaction_norm( \
               id BIGINT, case_id TEXT, acct_no TEXT, amount TEXT, \
               clean_invalid INTEGER, clean_failed INTEGER \
             ); \
             INSERT INTO fc_transaction_norm VALUES \
               (1, 'case-a', 'acct-a', '1', 0, 0);",
        ] {
            let conn = Connection::open_in_memory().expect("open fixture");
            conn.execute_batch(schema_and_row)
                .expect("seed unresolved cleaning state");
            assert!(create_rule_txn_index_staging(&conn, "case-a", &columns(&conn)).is_err());
        }
    }
}
