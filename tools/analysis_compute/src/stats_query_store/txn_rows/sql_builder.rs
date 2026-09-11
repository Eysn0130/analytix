use anyhow::Result;
use duckdb::Connection;

use super::super::dedupe_sql::{deduped_source_ctes, detail_stats_dedupe_key_expr};

pub(super) struct TxnRowsManualMappingSql {
    join_sql: String,
    account_open_name_expr: String,
    opener_id_no_expr: String,
    has_mapping: bool,
}

#[derive(Clone)]
pub(super) struct TxnRowsManualMappingSchema {
    columns: Option<Vec<String>>,
}

pub(super) fn build_manual_mapping_sql(
    conn: &Connection,
    case_id: &str,
) -> Result<TxnRowsManualMappingSql> {
    Ok(load_manual_mapping_schema(conn)?.build_sql(case_id))
}

pub(super) fn load_manual_mapping_schema(conn: &Connection) -> Result<TxnRowsManualMappingSchema> {
    if !crate::table_exists(conn, "analysis_manual_account_mapping")? {
        return Ok(TxnRowsManualMappingSchema { columns: None });
    }
    let columns = crate::table_columns(conn, "analysis_manual_account_mapping")?;
    if !has_column(&columns, "case_id") || !has_column(&columns, "account_key") {
        return Ok(TxnRowsManualMappingSchema { columns: None });
    }
    Ok(TxnRowsManualMappingSchema {
        columns: Some(columns),
    })
}

impl TxnRowsManualMappingSchema {
    pub(super) fn build_sql(&self, case_id: &str) -> TxnRowsManualMappingSql {
        let Some(columns) = self.columns.as_ref() else {
            return TxnRowsManualMappingSql::empty();
        };

        let account_open_name = manual_text_expr(&columns, "account_open_name");
        let opener_id_no = manual_text_expr(&columns, "opener_id_no");
        TxnRowsManualMappingSql {
        join_sql: format!(
            "LEFT JOIN analysis_manual_account_mapping m \
              ON m.case_id = {} \
              AND m.account_key = d.acct_key",
            crate::sql_literal(case_id)
        ),
        account_open_name_expr: format!("CASE WHEN m.account_key IS NOT NULL THEN COALESCE({account_open_name}, '') ELSE d.account_open_name END"),
        opener_id_no_expr: format!(
            "CASE WHEN m.account_key IS NOT NULL THEN COALESCE({opener_id_no}, '') ELSE d.opener_id_no END"
        ),
        has_mapping: true,
        }
    }
}

pub(super) fn build_txn_rows_sql(
    where_sql: &str,
    order_sql: &str,
    limit: i64,
    manual_mapping: &TxnRowsManualMappingSql,
    dedupe_enabled: bool,
) -> String {
    if !manual_mapping.has_mapping {
        return build_direct_txn_rows_sql(where_sql, order_sql, limit, dedupe_enabled);
    }
    let mut sql = format!(
        "{}\
         SELECT \
           id, card_no, acct_no, account_open_name, opener_id_no, txn_time, amount, balance, dc_val, \
           counterparty_acct, cash_flag, counterparty_name, counterparty_id_no, counterparty_bank, summary, \
           currency, branch_name, branch_code, location, is_success, voucher_no, terminal_no, ip_addr, mac_addr, counterparty_balance, \
           txn_id, log_id, voucher_type, voucher_id, teller_no, merchant_name, merchant_no, remark, txn_type, query_feedback_reason, \
           CAST(txn_ts AS VARCHAR) AS txn_ts_str \
         FROM base \
         ORDER BY {order_sql}",
        build_base_sql(where_sql, manual_mapping, dedupe_enabled)
    );
    if limit > 0 {
        sql.push_str(&format!(" LIMIT {limit}"));
    }
    sql
}

fn build_direct_txn_rows_sql(
    where_sql: &str,
    order_sql: &str,
    limit: i64,
    dedupe_enabled: bool,
) -> String {
    if dedupe_enabled {
        let mut sql = format!(
            "WITH {} \
             SELECT \
               id, card_no, acct_no, account_open_name, opener_id_no, txn_time, amount, balance, dc_val, \
               counterparty_acct, cash_flag, counterparty_name, counterparty_id_no, counterparty_bank, summary, \
               currency, branch_name, branch_code, location, is_success, voucher_no, terminal_no, ip_addr, mac_addr, counterparty_balance, \
               txn_id, log_id, voucher_type, voucher_id, teller_no, merchant_name, merchant_no, remark, txn_type, query_feedback_reason, \
               CAST(txn_ts AS VARCHAR) AS txn_ts_str \
             FROM filtered \
             ORDER BY {order_sql}",
            deduped_source_ctes(&raw_detail_source_sql(where_sql), true)
        );
        if limit > 0 {
            sql.push_str(&format!(" LIMIT {limit}"));
        }
        return sql;
    }

    let mut sql = format!(
        "SELECT \
           id, card_no, acct_no, account_open_name, opener_id_no, txn_time, amount, balance, dc_val, \
           counterparty_acct, cash_flag, counterparty_name, counterparty_id_no, counterparty_bank, summary, \
           currency, branch_name, branch_code, location, is_success, voucher_no, terminal_no, ip_addr, mac_addr, counterparty_balance, \
           txn_id, log_id, voucher_type, voucher_id, teller_no, merchant_name, merchant_no, remark, txn_type, query_feedback_reason, \
           CAST(txn_ts AS VARCHAR) AS txn_ts_str \
         FROM {} \
         WHERE {where_sql} \
         ORDER BY {order_sql}",
        crate::DETAIL_TABLE
    );
    if limit > 0 {
        sql.push_str(&format!(" LIMIT {limit}"));
    }
    sql
}

fn build_base_sql(
    where_sql: &str,
    manual_mapping: &TxnRowsManualMappingSql,
    dedupe_enabled: bool,
) -> String {
    let source_ctes = if dedupe_enabled {
        deduped_source_ctes(&raw_detail_source_sql(where_sql), true)
    } else {
        format!(
            "filtered AS (\
             SELECT \
               id, acct_key, txn_ts, txn_time, amount, balance, dc_val, card_no, acct_no, account_open_name, opener_id_no, \
               counterparty_acct, cash_flag, counterparty_name, counterparty_id_no, counterparty_bank, \
               summary, currency, branch_name, branch_code, location, is_success, voucher_no, terminal_no, ip_addr, mac_addr, \
               counterparty_balance, txn_id, log_id, voucher_type, voucher_id, teller_no, merchant_name, merchant_no, remark, \
               txn_type, query_feedback_reason \
             FROM {} \
             WHERE {where_sql}\
           )",
            crate::DETAIL_TABLE
        )
    };
    format!(
        "WITH {source_ctes}, base AS (\
         SELECT \
           d.id, d.acct_key, d.txn_ts, d.txn_time, d.amount, d.balance, d.dc_val, d.card_no, d.acct_no, \
           {} AS account_open_name, {} AS opener_id_no, \
           d.counterparty_acct, d.cash_flag, d.counterparty_name, d.counterparty_id_no, d.counterparty_bank, \
           d.summary, d.currency, d.branch_name, d.branch_code, d.location, d.is_success, d.voucher_no, d.terminal_no, d.ip_addr, d.mac_addr, \
           d.counterparty_balance, d.txn_id, d.log_id, d.voucher_type, d.voucher_id, d.teller_no, d.merchant_name, d.merchant_no, d.remark, \
           d.txn_type, d.query_feedback_reason \
         FROM filtered d \
         {}\
       ) ",
        manual_mapping.account_open_name_expr,
        manual_mapping.opener_id_no_expr,
        manual_mapping.join_sql,
    )
}

fn raw_detail_source_sql(where_sql: &str) -> String {
    format!(
        "SELECT d.*, {} AS stats_dedupe_key \
           FROM {} d \
          WHERE {where_sql}",
        detail_stats_dedupe_key_expr("d"),
        crate::DETAIL_TABLE
    )
}

impl TxnRowsManualMappingSql {
    fn empty() -> Self {
        Self {
            join_sql: String::new(),
            account_open_name_expr: "d.account_open_name".to_string(),
            opener_id_no_expr: "d.opener_id_no".to_string(),
            has_mapping: false,
        }
    }
}

fn has_column(columns: &[String], name: &str) -> bool {
    columns.iter().any(|item| item == name)
}

fn manual_text_expr(columns: &[String], name: &str) -> String {
    if has_column(columns, name) {
        format!("NULLIF(TRIM(m.{name}), '')")
    } else {
        "NULL".to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn txn_rows_sql_skips_mapping_ctes_when_mapping_is_empty() {
        let sql = build_txn_rows_sql(
            "acct_key = 'CARD-001'",
            "txn_ts DESC NULLS LAST, id DESC",
            300,
            &TxnRowsManualMappingSql::empty(),
            false,
        );

        assert!(sql.contains("FROM analysis_txn_detail_idx"));
        assert!(sql.contains("WHERE acct_key = 'CARD-001'"));
        assert!(sql.contains("CAST(txn_ts AS VARCHAR) AS txn_ts_str"));
        assert!(sql.contains("ORDER BY txn_ts DESC NULLS LAST, id DESC"));
        assert!(sql.ends_with(" LIMIT 300"));
        assert!(!sql.contains("WITH filtered AS"));
        assert!(!sql.contains("analysis_manual_account_mapping"));
    }

    #[test]
    fn txn_rows_sql_keeps_mapping_ctes_when_mapping_exists() {
        let sql = build_txn_rows_sql(
            "acct_key = 'CARD-001'",
            "txn_ts DESC NULLS LAST, id DESC",
            300,
            &TxnRowsManualMappingSql {
                join_sql:
                    "LEFT JOIN analysis_manual_account_mapping m ON m.account_key = d.acct_key"
                        .to_string(),
                account_open_name_expr: "COALESCE(m.account_open_name, d.account_open_name)"
                    .to_string(),
                opener_id_no_expr: "COALESCE(m.opener_id_no, d.opener_id_no)".to_string(),
                has_mapping: true,
            },
            false,
        );

        assert!(sql.contains("WITH filtered AS"));
        assert!(sql.contains("LEFT JOIN analysis_manual_account_mapping"));
        assert!(
            sql.contains("COALESCE(m.account_open_name, d.account_open_name) AS account_open_name")
        );
    }

    #[test]
    fn txn_rows_sql_can_dedupe_cross_account_detail_rows() {
        let sql = build_txn_rows_sql(
            "acct_key IN ('CARD-001','CARD-002')",
            "txn_ts DESC NULLS LAST, id DESC",
            300,
            &TxnRowsManualMappingSql::empty(),
            true,
        );

        assert!(sql.contains("signature_stats AS"));
        assert!(sql.contains("COUNT(DISTINCT acct_key)"));
        assert!(sql.contains("stats_dedupe_key"));
        assert!(sql.contains("FROM filtered"));
    }
}
