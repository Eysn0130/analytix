use anyhow::Result;
use duckdb::Connection;
use std::time::Instant;

use super::profile::PhaseProfile;

use crate::{
    account_key_expr, amount_value_expr, coalesce_expr, counterparty_key_expr,
    counterparty_raw_expr, dc_value_expr, finite_num_value_expr, has_col, placeholder_kind_sql,
    placeholder_token_sql, sql_literal, text_col_expr, trim_expr, txn_ts_expr,
    ACCOUNT_DIM_STAGING_TABLE, ACCOUNT_KEY_CANDIDATES, AGG_STAGING_TABLE, DETAIL_STAGING_TABLE,
    NULL_TEXT, NULL_TIMESTAMP,
};

pub(super) fn create_txn_daily_staging_tables(
    conn: &Connection,
    case_id: &str,
    cols: &[String],
) -> Result<PhaseProfile> {
    let mut profile = PhaseProfile::default();
    let started = Instant::now();
    let acct_key_expr = account_key_expr("t", cols, ACCOUNT_KEY_CANDIDATES);
    let mut cp_key_expr = counterparty_key_expr("t", cols);
    let mut cp_raw_expr = counterparty_raw_expr("t", cols);
    let cp_placeholder_kind_expr = if cp_raw_expr != NULL_TEXT {
        placeholder_kind_sql(&cp_raw_expr)
    } else {
        NULL_TEXT.to_string()
    };
    let dc_expr = dc_value_expr("t", cols);
    let amt_expr = amount_value_expr("t", cols);
    let amount_source_parts = ["clean_amount", "amount_val", "amount"]
        .iter()
        .filter(|column| has_col(cols, column))
        .map(|column| format!("{} IS NOT NULL", trim_expr(&format!("t.{column}"))))
        .collect::<Vec<_>>();
    let amount_source_present_expr = if amount_source_parts.is_empty() {
        "FALSE".to_string()
    } else {
        format!("({})", amount_source_parts.join(" OR "))
    };
    let amount_source_present_flag_expr =
        format!("CASE WHEN {amount_source_present_expr} THEN 1 ELSE 0 END");
    let amount_parse_failed_expr = format!(
        "CASE WHEN {amount_source_present_expr} AND ({amt_expr}) IS NULL THEN 1 ELSE 0 END"
    );
    let ts_expr = txn_ts_expr("t", cols);
    let open_name_expr = text_col_expr("t", cols, "account_open_name", true)
        .unwrap_or_else(|| NULL_TEXT.to_string());
    let card_expr = coalesce_expr(
        &[
            text_col_expr("t", cols, "clean_card_no", true),
            text_col_expr("t", cols, "card_no_norm", true),
            text_col_expr("t", cols, "card_no", true),
        ],
        NULL_TEXT,
    );
    let acct_expr = coalesce_expr(
        &[
            text_col_expr("t", cols, "clean_acct_no", false),
            text_col_expr("t", cols, "acct_no_norm", false),
            text_col_expr("t", cols, "acct_no", false),
        ],
        NULL_TEXT,
    );
    let balance_expr =
        finite_num_value_expr("t", cols, &["balance_val", "clean_balance", "balance"]);
    let opener_id_expr =
        text_col_expr("t", cols, "opener_id_no", true).unwrap_or_else(|| NULL_TEXT.to_string());
    let mut cp_name_expr = text_col_expr("t", cols, "counterparty_name", true)
        .unwrap_or_else(|| NULL_TEXT.to_string());
    let txn_time_expr = if has_col(cols, "txn_time") {
        format!(
            "COALESCE({}, CAST({} AS VARCHAR))",
            trim_expr("t.txn_time"),
            ts_expr
        )
    } else if ts_expr != NULL_TIMESTAMP {
        format!("CAST({ts_expr} AS VARCHAR)")
    } else {
        NULL_TEXT.to_string()
    };
    let mut bank_expr = text_col_expr("t", cols, "counterparty_bank", false)
        .unwrap_or_else(|| NULL_TEXT.to_string());
    let location_expr =
        text_col_expr("t", cols, "location", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let mut counterparty_acct_expr = text_col_expr("t", cols, "counterparty_acct", false)
        .unwrap_or_else(|| NULL_TEXT.to_string());
    let cash_flag_expr =
        text_col_expr("t", cols, "cash_flag", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let counterparty_id_expr = text_col_expr("t", cols, "counterparty_id_no", false)
        .unwrap_or_else(|| NULL_TEXT.to_string());
    let summary_expr =
        text_col_expr("t", cols, "summary", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let mut currency_expr =
        text_col_expr("t", cols, "currency", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let mut raw_source_join_sql = String::new();
    if crate::table_exists(conn, "fc_transaction_raw")? {
        let raw_columns = crate::table_columns(conn, "fc_transaction_raw")?;
        let raw_locator_columns = ["case_id", "file_id", "row_no"];
        if raw_locator_columns
            .iter()
            .all(|column| has_col(cols, column) && has_col(&raw_columns, column))
        {
            let exact_raw_value = |column: &str, alias: &str| {
                if has_col(&raw_columns, column) {
                    format!(
                        "CASE WHEN COUNT(*)=1 \
                              THEN MIN(NULLIF(TRIM(CAST({column} AS VARCHAR)), '')) \
                              ELSE NULL END AS {alias}"
                    )
                } else {
                    format!("CAST(NULL AS VARCHAR) AS {alias}")
                }
            };
            raw_source_join_sql = format!(
                "LEFT JOIN ( \
                   SELECT case_id, file_id, row_no, {}, {}, {}, {} \
                     FROM fc_transaction_raw \
                    WHERE case_id={} \
                    GROUP BY case_id, file_id, row_no \
                 ) raw_source \
                   ON raw_source.case_id=t.case_id \
                  AND raw_source.file_id=t.file_id \
                  AND raw_source.row_no=t.row_no",
                exact_raw_value("currency_raw", "currency"),
                exact_raw_value("counterparty_acct_raw", "counterparty_acct"),
                exact_raw_value("counterparty_name_raw", "counterparty_name"),
                exact_raw_value("counterparty_bank_raw", "counterparty_bank"),
                sql_literal(case_id),
            );
            currency_expr = format!("COALESCE({currency_expr}, raw_source.currency)");
            cp_key_expr = format!("COALESCE({cp_key_expr}, raw_source.counterparty_acct)");
            cp_raw_expr = format!("COALESCE({cp_raw_expr}, raw_source.counterparty_acct)");
            cp_name_expr = format!("COALESCE({cp_name_expr}, raw_source.counterparty_name)");
            bank_expr = format!("COALESCE({bank_expr}, raw_source.counterparty_bank)");
            counterparty_acct_expr =
                format!("COALESCE({counterparty_acct_expr}, raw_source.counterparty_acct)");
        }
    }
    let branch_expr =
        text_col_expr("t", cols, "branch_name", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let branch_code_expr =
        text_col_expr("t", cols, "branch_code", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let success_expr =
        text_col_expr("t", cols, "is_success", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let voucher_no_expr =
        text_col_expr("t", cols, "voucher_no", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let terminal_no_expr =
        text_col_expr("t", cols, "terminal_no", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let ip_expr =
        text_col_expr("t", cols, "ip_addr", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let mac_expr =
        text_col_expr("t", cols, "mac_addr", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let counterparty_balance_expr = finite_num_value_expr("t", cols, &["counterparty_balance"]);
    let txn_id_expr =
        text_col_expr("t", cols, "txn_id", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let file_id_expr =
        text_col_expr("t", cols, "file_id", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let log_id_expr =
        text_col_expr("t", cols, "log_id", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let voucher_type_expr =
        text_col_expr("t", cols, "voucher_type", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let voucher_id_expr =
        text_col_expr("t", cols, "voucher_id", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let teller_expr =
        text_col_expr("t", cols, "teller_no", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let merchant_name_expr =
        text_col_expr("t", cols, "merchant_name", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let merchant_no_expr =
        text_col_expr("t", cols, "merchant_no", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let remark_expr =
        text_col_expr("t", cols, "remark", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let txn_type_expr =
        text_col_expr("t", cols, "txn_type", false).unwrap_or_else(|| NULL_TEXT.to_string());
    let feedback_expr = text_col_expr("t", cols, "query_feedback_reason", false)
        .unwrap_or_else(|| NULL_TEXT.to_string());

    let clean_acceptance_expr = crate::clean_row_acceptance_expr("t", cols)
        .ok_or_else(|| anyhow::anyhow!("transaction cleaning authority is unavailable"))?;
    let where_parts = vec![
        format!("t.case_id={}", sql_literal(case_id)),
        clean_acceptance_expr,
    ];
    let where_sql = where_parts.join(" AND ");
    let resolved_name_expr = format!(
        "CASE WHEN base.cp_name IS NOT NULL AND TRIM(base.cp_name) <> '' THEN base.cp_name \
         WHEN base.cp_placeholder_kind IS NOT NULL THEN {} \
         WHEN COALESCE(name_pick.pick_cnt, 0) >= 3 THEN name_pick.pick_name ELSE base.cp_key END",
        placeholder_token_sql("base.cp_placeholder_kind")
    );
    profile.record_since("prepare_staging_expressions", started);

    let started = Instant::now();
    conn.execute_batch(&format!(
        "
        DROP TABLE IF EXISTS {AGG_STAGING_TABLE};
        DROP TABLE IF EXISTS {DETAIL_STAGING_TABLE};
        DROP TABLE IF EXISTS {ACCOUNT_DIM_STAGING_TABLE};
        "
    ))?;
    profile.record_since("drop_staging_tables", started);

    let started = Instant::now();
    conn.execute_batch(&format!(
        "
        CREATE TABLE {DETAIL_STAGING_TABLE} AS
        WITH base AS (
          SELECT
            t.id AS id,
            CAST({ts_expr} AS DATE) AS txn_day,
            {acct_key_expr} AS acct_key,
            {cp_key_expr} AS cp_key,
            {cp_raw_expr} AS cp_raw,
            {cp_placeholder_kind_expr} AS cp_placeholder_kind,
            {cp_name_expr} AS cp_name,
            {dc_expr} AS dc_val,
            {ts_expr} AS txn_ts,
            {txn_time_expr} AS txn_time,
            {amt_expr} AS amount,
            {amount_source_present_flag_expr} AS amount_source_present,
            {amount_parse_failed_expr} AS amount_parse_failed,
            {balance_expr} AS balance,
            {card_expr} AS card_no,
            {acct_expr} AS acct_no,
            {open_name_expr} AS account_open_name,
            {opener_id_expr} AS opener_id_no,
            {counterparty_acct_expr} AS counterparty_acct,
            {cash_flag_expr} AS cash_flag,
            {counterparty_id_expr} AS counterparty_id_no,
            {bank_expr} AS counterparty_bank,
            {summary_expr} AS summary,
            {currency_expr} AS currency,
            {branch_expr} AS branch_name,
            {branch_code_expr} AS branch_code,
            {location_expr} AS location,
            {success_expr} AS is_success,
            {voucher_no_expr} AS voucher_no,
            {terminal_no_expr} AS terminal_no,
            {ip_expr} AS ip_addr,
            {mac_expr} AS mac_addr,
            {counterparty_balance_expr} AS counterparty_balance,
            {txn_id_expr} AS txn_id,
            {file_id_expr} AS file_id,
            {log_id_expr} AS log_id,
            {voucher_type_expr} AS voucher_type,
            {voucher_id_expr} AS voucher_id,
            {teller_expr} AS teller_no,
            {merchant_name_expr} AS merchant_name,
            {merchant_no_expr} AS merchant_no,
            {remark_expr} AS remark,
            {txn_type_expr} AS txn_type,
            {feedback_expr} AS query_feedback_reason
          FROM fc_transaction_norm t
          {raw_source_join_sql}
          WHERE {where_sql}
            AND {acct_key_expr} IS NOT NULL
            AND TRIM({acct_key_expr}) <> ''
        ), name_stats AS (
          SELECT cp_key, cp_name, COUNT(1) AS name_cnt, MAX(txn_ts) AS name_last_ts
          FROM base
          WHERE cp_key IS NOT NULL AND TRIM(cp_key) <> '' AND cp_name IS NOT NULL AND TRIM(cp_name) <> ''
          GROUP BY cp_key, cp_name
        ), name_ranked AS (
          SELECT cp_key, cp_name, name_cnt, name_last_ts,
                 ROW_NUMBER() OVER (PARTITION BY cp_key ORDER BY name_cnt DESC, name_last_ts DESC, cp_name DESC) AS rn
          FROM name_stats
        ), name_pick AS (
          SELECT cp_key AS pick_key, cp_name AS pick_name, name_cnt AS pick_cnt
          FROM name_ranked WHERE rn = 1
        )
        SELECT
          base.id AS id,
          base.txn_day AS txn_day,
          base.acct_key AS acct_key,
          base.cp_key AS cp_key,
          base.cp_raw AS cp_raw,
          base.cp_placeholder_kind AS cp_placeholder_kind,
          base.cp_name AS cp_name,
          name_pick.pick_name AS cp_name_pick,
          COALESCE(name_pick.pick_cnt, 0) AS cp_name_pick_cnt,
          {resolved_name_expr} AS stats_name_key,
          base.dc_val AS dc_val,
          base.txn_ts AS txn_ts,
          base.txn_time AS txn_time,
          base.amount AS amount,
          base.amount_source_present AS amount_source_present,
          base.amount_parse_failed AS amount_parse_failed,
          base.balance AS balance,
          base.card_no AS card_no,
          base.acct_no AS acct_no,
          base.account_open_name AS account_open_name,
          base.opener_id_no AS opener_id_no,
          base.counterparty_acct AS counterparty_acct,
          base.cash_flag AS cash_flag,
          base.cp_name AS counterparty_name,
          base.counterparty_id_no AS counterparty_id_no,
          base.counterparty_bank AS counterparty_bank,
          base.summary AS summary,
          base.currency AS currency,
          base.branch_name AS branch_name,
          base.branch_code AS branch_code,
          base.location AS location,
          base.is_success AS is_success,
          base.voucher_no AS voucher_no,
          base.terminal_no AS terminal_no,
          base.ip_addr AS ip_addr,
          base.mac_addr AS mac_addr,
          base.counterparty_balance AS counterparty_balance,
          base.txn_id AS txn_id,
          base.file_id AS file_id,
          base.log_id AS log_id,
          base.voucher_type AS voucher_type,
          base.voucher_id AS voucher_id,
          base.teller_no AS teller_no,
          base.merchant_name AS merchant_name,
          base.merchant_no AS merchant_no,
          base.remark AS remark,
          base.txn_type AS txn_type,
          base.query_feedback_reason AS query_feedback_reason
        FROM base
        LEFT JOIN name_pick ON name_pick.pick_key = base.cp_key
        "
    ))?;
    profile.record_since("build_detail_idx_staging", started);

    let started = Instant::now();
    conn.execute_batch(&format!(
        "
        CREATE TABLE {AGG_STAGING_TABLE} AS
        WITH detail_base AS (
          SELECT
            txn_day,
            acct_key,
            cp_key,
            COALESCE(NULLIF(counterparty_acct, ''), NULLIF(cp_raw, ''), cp_key) AS cp_display,
            cp_raw,
            cp_placeholder_kind,
            cp_name,
            cp_name_pick,
            cp_name_pick_cnt,
            stats_name_key,
            dc_val,
            amount,
            amount_source_present,
            amount_parse_failed,
            ABS(amount) AS amt_abs,
            txn_ts,
            account_open_name AS open_name,
            counterparty_bank,
            location
          FROM {DETAIL_STAGING_TABLE}
        )
        SELECT
          txn_day,
          acct_key,
          cp_key,
          MAX(cp_display) AS cp_display,
          cp_raw,
          cp_placeholder_kind,
          cp_name,
          MAX(cp_name_pick) AS cp_name_pick,
          MAX(cp_name_pick_cnt) AS cp_name_pick_cnt,
          MAX(stats_name_key) AS stats_name_key,
          dc_val,
          COUNT(1) AS txn_count,
          SUM(amount_source_present) AS amount_source_present_count,
          SUM(CASE WHEN amount IS NOT NULL THEN 1 ELSE 0 END) AS amount_valid_count,
          SUM(CASE WHEN amount_source_present=0 THEN 1 ELSE 0 END) AS amount_missing_count,
          SUM(amount_parse_failed) AS amount_parse_failed_count,
          CASE WHEN COUNT(1)=SUM(CASE WHEN amount IS NOT NULL THEN 1 ELSE 0 END)
               THEN ROUND(SUM(amt_abs), 2) ELSE NULL END AS amt_sum,
          MIN(txn_ts) AS first_ts,
          MAX(txn_ts) AS last_ts,
          MAX(open_name) AS open_name,
          MAX(counterparty_bank) AS counterparty_bank,
          MAX(location) AS location
        FROM detail_base
        GROUP BY txn_day, acct_key, cp_key, cp_raw, cp_placeholder_kind, cp_name, dc_val
        "
    ))?;
    profile.record_since("build_daily_agg_staging", started);
    Ok(profile)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn detail_materialization_preserves_missing_and_parse_failed_amounts() {
        let conn = Connection::open_in_memory().expect("open fixture");
        conn.execute_batch(
            "CREATE TABLE fc_transaction_norm( \
               id BIGINT, case_id TEXT, acct_no TEXT, amount TEXT, \
               clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER \
             ); \
             INSERT INTO fc_transaction_norm VALUES \
               (1, 'case-a', 'acct-a', '12.34', 0, 0, 0), \
               (2, 'case-a', 'acct-a', NULL, 0, 0, 0), \
               (3, 'case-a', 'acct-a', 'not-a-number', 0, 0, 0), \
               (4, 'case-a', 'acct-a', '0', 0, 0, 0), \
               (5, 'case-a', 'acct-a', 'NaN', 0, 0, 0), \
               (6, 'case-a', 'acct-a', 'Inf', 0, 0, 0), \
               (7, 'case-a', 'acct-a', '-Inf', 0, 0, 0), \
               (8, 'case-a', 'acct-a', '1e309', 0, 0, 0);",
        )
        .expect("seed normalized transactions");
        let columns = crate::table_columns(&conn, "fc_transaction_norm").expect("load columns");

        create_txn_daily_staging_tables(&conn, "case-a", &columns)
            .expect("materialize transaction detail");

        let mut statement = conn
            .prepare(&format!(
                "SELECT amount, amount_source_present, amount_parse_failed \
                   FROM {DETAIL_STAGING_TABLE} ORDER BY id"
            ))
            .expect("prepare detail inspection");
        let rows = statement
            .query_map([], |row| {
                Ok((
                    row.get::<_, Option<f64>>(0)?,
                    row.get::<_, i64>(1)?,
                    row.get::<_, i64>(2)?,
                ))
            })
            .expect("query detail inspection")
            .collect::<std::result::Result<Vec<_>, _>>()
            .expect("collect detail inspection");

        assert_eq!(
            rows,
            vec![
                (Some(12.34), 1, 0),
                (None, 0, 0),
                (None, 1, 1),
                (Some(0.0), 1, 0),
                (None, 1, 1),
                (None, 1, 1),
                (None, 1, 1),
                (None, 1, 1),
            ]
        );

        let aggregate = conn
            .query_row(
                &format!(
                    "SELECT txn_count, amt_sum, amount_source_present_count, amount_valid_count, \
                            amount_missing_count, amount_parse_failed_count \
                       FROM {AGG_STAGING_TABLE}"
                ),
                [],
                |row| {
                    Ok((
                        row.get::<_, i64>(0)?,
                        row.get::<_, Option<f64>>(1)?,
                        row.get::<_, i64>(2)?,
                        row.get::<_, i64>(3)?,
                        row.get::<_, i64>(4)?,
                        row.get::<_, i64>(5)?,
                    ))
                },
            )
            .expect("query aggregate coverage");
        assert_eq!(aggregate, (8, None, 7, 2, 1, 5));
    }

    #[test]
    fn detail_materialization_preserves_balance_authority_and_finite_semantics() {
        let conn = Connection::open_in_memory().expect("open fixture");
        conn.execute_batch(
            "CREATE TABLE fc_transaction_norm( \
               id BIGINT, case_id TEXT, acct_no TEXT, amount TEXT, \
               balance_val DOUBLE, clean_balance TEXT, balance TEXT, \
               counterparty_balance TEXT, \
               clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER \
             ); \
             INSERT INTO fc_transaction_norm VALUES \
               (1, 'case-a', 'acct-a', '1', CAST('NaN' AS DOUBLE), '101', '201', 'NaN', 0, 0, 0), \
               (2, 'case-a', 'acct-a', '1', CAST('Inf' AS DOUBLE), '102', '202', 'Inf', 0, 0, 0), \
               (3, 'case-a', 'acct-a', '1', CAST('-Inf' AS DOUBLE), '103', '203', '-Inf', 0, 0, 0), \
               (4, 'case-a', 'acct-a', '1', NULL, 'not-a-number', '204', 'not-a-number', 0, 0, 0), \
               (5, 'case-a', 'acct-a', '1', NULL, '  ', '0', '0', 0, 0, 0), \
               (6, 'case-a', 'acct-a', '1', NULL, NULL, NULL, NULL, 0, 0, 0), \
               (7, 'case-a', 'acct-a', '1', 0, '107', '207', '12.5', 0, 0, 0), \
               (8, 'case-a', 'acct-a', '1', NULL, '0', '208', '  ', 0, 0, 0);",
        )
        .expect("seed balance variants");
        let columns = crate::table_columns(&conn, "fc_transaction_norm").expect("load columns");

        create_txn_daily_staging_tables(&conn, "case-a", &columns)
            .expect("materialize transaction detail");

        let mut statement = conn
            .prepare(&format!(
                "SELECT id, balance, counterparty_balance \
                   FROM {DETAIL_STAGING_TABLE} ORDER BY id"
            ))
            .expect("prepare balance inspection");
        let rows = statement
            .query_map([], |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, Option<f64>>(1)?,
                    row.get::<_, Option<f64>>(2)?,
                ))
            })
            .expect("query balance inspection")
            .collect::<std::result::Result<Vec<_>, _>>()
            .expect("collect balance inspection");

        assert_eq!(
            rows,
            vec![
                (1, None, None),
                (2, None, None),
                (3, None, None),
                (4, None, None),
                (5, Some(0.0), Some(0.0)),
                (6, None, None),
                (7, Some(0.0), Some(12.5)),
                (8, Some(0.0), None),
            ]
        );
    }

    #[test]
    fn detail_materialization_projects_only_unique_raw_locator_fields() {
        let conn = Connection::open_in_memory().expect("open fixture");
        conn.execute_batch(
            "CREATE TABLE fc_transaction_norm( \
               id BIGINT, case_id TEXT, file_id TEXT, row_no BIGINT, acct_no TEXT, amount TEXT, \
               clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER \
             ); \
             CREATE TABLE fc_transaction_raw( \
               case_id TEXT, file_id TEXT, row_no BIGINT, currency_raw TEXT, \
               counterparty_acct_raw TEXT, counterparty_name_raw TEXT, counterparty_bank_raw TEXT \
             ); \
             INSERT INTO fc_transaction_norm VALUES \
               (1, 'case-a', 'file-a', 1, 'acct-a', '12.50', 0, 0, 0), \
               (2, 'case-a', 'file-a', 2, 'acct-a', '8.00', 0, 0, 0); \
             INSERT INTO fc_transaction_raw VALUES \
               ('case-a', 'file-a', 1, ' CNY ', ' 6217009876543210 ', ' 合成对手 ', ' 合成银行 '), \
               ('case-a', 'file-a', 2, 'CNY', '6217009876543210', '合成对手', '合成银行'), \
               ('case-a', 'file-a', 2, 'USD', '6217009876543211', '另一对手', '另一银行'), \
               ('case-b', 'file-a', 1, 'USD', '6217009876543212', '跨案对手', '跨案银行');",
        )
        .expect("seed normalized and raw transactions");
        let columns = crate::table_columns(&conn, "fc_transaction_norm").expect("load columns");

        create_txn_daily_staging_tables(&conn, "case-a", &columns)
            .expect("materialize raw locator field projection");

        let mut statement = conn
            .prepare(&format!(
                "SELECT id, currency, cp_key, counterparty_name, counterparty_bank, counterparty_acct \
                   FROM {DETAIL_STAGING_TABLE} ORDER BY id"
            ))
            .expect("prepare currency inspection");
        let rows = statement
            .query_map([], |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, Option<String>>(1)?,
                    row.get::<_, Option<String>>(2)?,
                    row.get::<_, Option<String>>(3)?,
                    row.get::<_, Option<String>>(4)?,
                    row.get::<_, Option<String>>(5)?,
                ))
            })
            .expect("query currency inspection")
            .collect::<std::result::Result<Vec<_>, _>>()
            .expect("collect currency inspection");

        assert_eq!(
            rows,
            vec![
                (
                    1,
                    Some("CNY".to_string()),
                    Some("6217009876543210".to_string()),
                    Some("合成对手".to_string()),
                    Some("合成银行".to_string()),
                    Some("6217009876543210".to_string()),
                ),
                (2, None, None, None, None, None),
            ]
        );
    }

    #[test]
    fn detail_materialization_requires_explicit_cleaning_authority() {
        let missing_flag_conn = Connection::open_in_memory().expect("open missing flag fixture");
        missing_flag_conn
            .execute_batch(
                "CREATE TABLE fc_transaction_norm( \
                   id BIGINT, case_id TEXT, acct_no TEXT, amount TEXT, \
                   clean_invalid INTEGER, clean_failed INTEGER \
                 );",
            )
            .expect("create missing flag schema");
        let missing_columns = crate::table_columns(&missing_flag_conn, "fc_transaction_norm")
            .expect("load missing flag columns");
        assert!(
            create_txn_daily_staging_tables(&missing_flag_conn, "case-a", &missing_columns)
                .is_err()
        );

        let unknown_flag_conn = Connection::open_in_memory().expect("open unknown flag fixture");
        unknown_flag_conn
            .execute_batch(
                "CREATE TABLE fc_transaction_norm( \
                   id BIGINT, case_id TEXT, acct_no TEXT, amount TEXT, \
                   clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER \
                 ); \
                 INSERT INTO fc_transaction_norm VALUES \
                   (1, 'case-a', 'acct-a', '12.34', 0, NULL, 0), \
                   (2, 'case-a', 'acct-a', '56.78', 0, 0, 0);",
            )
            .expect("seed unknown flag rows");
        let unknown_columns = crate::table_columns(&unknown_flag_conn, "fc_transaction_norm")
            .expect("load unknown flag columns");
        create_txn_daily_staging_tables(&unknown_flag_conn, "case-a", &unknown_columns)
            .expect("materialize only explicitly clean rows");
        let row_count = crate::scalar_i64(
            &unknown_flag_conn,
            &format!("SELECT COUNT(1) FROM {DETAIL_STAGING_TABLE}"),
        )
        .expect("count accepted detail rows");
        assert_eq!(row_count, 1);
    }
}
