use anyhow::{bail, Context, Result};
use duckdb::Connection;
use serde_json::Value;
use std::io::Write;

use super::args::QueryChartDetailRowsArgs;
use super::chart_common::{
    build_chart_detail_rows_where_sql, chart_dedupe_enabled, chart_detail_source_table_sql,
    chart_filters_supported_by_native_dashboard, normalize_selected_keys,
    require_chart_amount_query_ready, require_chart_scope, ChartSourceRequest,
};
use super::session::VerifiedStatsQuerySession;
use super::values::{
    query_stats_txn_row_values_with_total, write_stats_txn_row_values_json_with_total,
};

pub(crate) struct QueryChartDetailRowsResult {
    pub(crate) rows: Vec<Value>,
    pub(crate) total: i64,
}

pub(crate) struct QueryChartDetailRowsJsonWriteResult {
    pub(crate) total: i64,
    pub(crate) row_count: usize,
}

struct QueryChartDetailRowsRequest {
    source: ChartSourceRequest,
    direction_mode: String,
    sort_col: &'static str,
    sort_dir: &'static str,
    limit: i64,
    offset: i64,
}

impl QueryChartDetailRowsRequest {
    fn from_args(args: &QueryChartDetailRowsArgs) -> Self {
        let selected = normalize_selected_keys(&args.selected_keys);
        let sort_col = match args.sort_col.trim() {
            "amount" => "amount",
            "balance" => "balance",
            "counterparty_name" => "counterparty_name",
            "counterparty_bank" => "counterparty_bank",
            "location" => "location",
            "txn_type" => "txn_type",
            "is_success" => "is_success",
            _ => "txn_time",
        };
        let sort_dir = match args.sort_dir.trim().to_lowercase().as_str() {
            "-1" | "desc" | "down" => "desc",
            _ => "asc",
        };
        let limit = args.limit.clamp(1, 5000);
        let page = args.page.max(1);
        Self {
            source: ChartSourceRequest {
                selected,
                date_start: args.date_start.trim().to_string(),
                date_end: args.date_end.trim().to_string(),
                success_filter: args.success_filter.trim().to_string(),
                cash_filter: args.cash_filter.trim().to_string(),
                chart_filters: args.chart_filters.clone(),
            },
            direction_mode: args.direction_mode.trim().to_string(),
            sort_col,
            sort_dir,
            limit,
            offset: (page - 1) * limit,
        }
    }

    fn order_sql(&self) -> String {
        let dir_sql = if self.sort_dir == "desc" {
            "DESC"
        } else {
            "ASC"
        };
        match self.sort_col {
            "amount" => format!(
                "CASE WHEN amount IS NOT NULL AND isfinite(amount) THEN 0 ELSE 1 END ASC, \
                 CASE WHEN amount IS NOT NULL AND isfinite(amount) THEN ABS(amount) END {dir_sql} NULLS LAST, \
                 COALESCE(txn_time, '') {dir_sql}, id {dir_sql}"
            ),
            "balance" => format!(
                "CASE WHEN balance IS NOT NULL AND isfinite(balance) THEN 0 ELSE 1 END ASC, \
                 CASE WHEN balance IS NOT NULL AND isfinite(balance) THEN balance END {dir_sql} NULLS LAST, \
                 COALESCE(txn_time, '') {dir_sql}, id {dir_sql}"
            ),
            "counterparty_name" | "counterparty_bank" | "location" | "txn_type" | "is_success" => {
                format!(
                    "COALESCE({col}, '') {dir_sql}, COALESCE(txn_time, '') {dir_sql}",
                    col = self.sort_col
                )
            }
            _ => format!(
                "({ts} IS NULL) {dir_sql}, {ts} {dir_sql}, COALESCE(NULLIF(TRIM(txn_id), ''), CAST(id AS VARCHAR)) {dir_sql}",
                ts = chart_ts_expr()
            ),
        }
    }
}

struct ChartDetailManualMappingSql {
    join_sql: String,
    account_open_name_expr: String,
    opener_id_no_expr: String,
}

pub(crate) fn query_chart_detail_rows(
    args: &QueryChartDetailRowsArgs,
) -> Result<QueryChartDetailRowsResult> {
    if !chart_filters_supported_by_native_dashboard(&args.chart_filters) {
        bail!("unsupported chart filter for native chart detail rows");
    }
    let request = QueryChartDetailRowsRequest::from_args(args);
    require_chart_scope(&request.source.selected)?;

    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let manual_mapping = build_manual_mapping_sql(session.conn(), &args.case_id)?;
    let result = query_chart_detail_rows_with_session(&mut session, &request, &manual_mapping)?;
    session.commit()?;
    Ok(result)
}

pub(crate) fn query_chart_detail_rows_to_json_writer<W: Write>(
    args: &QueryChartDetailRowsArgs,
    writer: &mut W,
) -> Result<QueryChartDetailRowsJsonWriteResult> {
    if !chart_filters_supported_by_native_dashboard(&args.chart_filters) {
        bail!("unsupported chart filter for native chart detail rows");
    }
    let request = QueryChartDetailRowsRequest::from_args(args);
    require_chart_scope(&request.source.selected)?;

    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let manual_mapping = build_manual_mapping_sql(session.conn(), &args.case_id)?;
    let result =
        write_chart_detail_rows_with_session(&mut session, &request, &manual_mapping, writer)?;
    session.commit()?;
    Ok(result)
}

fn query_chart_detail_rows_with_session(
    session: &mut VerifiedStatsQuerySession<'_>,
    request: &QueryChartDetailRowsRequest,
    manual_mapping: &ChartDetailManualMappingSql,
) -> Result<QueryChartDetailRowsResult> {
    let where_sql = build_chart_detail_rows_where_sql(&request.source, &request.direction_mode);
    let dedupe_enabled = chart_dedupe_enabled(session.conn(), &request.source.selected)?;
    require_chart_amount_query_ready(session, &where_sql, dedupe_enabled)?;
    let sql = build_detail_rows_sql(
        &where_sql,
        &request.order_sql(),
        request.limit,
        request.offset,
        manual_mapping,
        dedupe_enabled,
    );
    session.require_nonempty_query("chart_detail_rows_page", &sql)?;
    let (rows_with_cursor, total_from_page) =
        query_stats_txn_row_values_with_total(session.conn(), &sql)?;
    let rows = rows_with_cursor
        .iter()
        .map(|item| serde_json::to_value(&item.row))
        .collect::<serde_json::Result<Vec<_>>>()?;
    Ok(QueryChartDetailRowsResult {
        rows,
        total: total_from_page,
    })
}

fn write_chart_detail_rows_with_session<W: Write>(
    session: &mut VerifiedStatsQuerySession<'_>,
    request: &QueryChartDetailRowsRequest,
    manual_mapping: &ChartDetailManualMappingSql,
    writer: &mut W,
) -> Result<QueryChartDetailRowsJsonWriteResult> {
    let where_sql = build_chart_detail_rows_where_sql(&request.source, &request.direction_mode);
    let dedupe_enabled = chart_dedupe_enabled(session.conn(), &request.source.selected)?;
    require_chart_amount_query_ready(session, &where_sql, dedupe_enabled)?;
    let sql = build_detail_rows_sql(
        &where_sql,
        &request.order_sql(),
        request.limit,
        request.offset,
        manual_mapping,
        dedupe_enabled,
    );
    session.require_nonempty_query("chart_detail_rows_page", &sql)?;
    writer.write_all(b"{\"rows\":")?;
    let write_result = write_stats_txn_row_values_json_with_total(session.conn(), &sql, writer)?;
    let total = write_result.total;
    writer.write_all(b",\"total\":")?;
    write!(writer, "{total}")?;
    writer.write_all(b"}")?;
    Ok(QueryChartDetailRowsJsonWriteResult {
        total,
        row_count: write_result.row_count,
    })
}

fn build_manual_mapping_sql(
    conn: &Connection,
    case_id: &str,
) -> Result<ChartDetailManualMappingSql> {
    if !crate::table_exists(conn, "analysis_manual_account_mapping")? {
        return Ok(ChartDetailManualMappingSql::empty());
    }
    let columns = crate::table_columns(conn, "analysis_manual_account_mapping")?;
    if !has_column(&columns, "case_id") || !has_column(&columns, "account_key") {
        return Ok(ChartDetailManualMappingSql::empty());
    }
    let account_open_name = manual_text_expr(&columns, "account_open_name");
    let opener_id_no = manual_text_expr(&columns, "opener_id_no");
    Ok(ChartDetailManualMappingSql {
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
    })
}

fn query_detail_total(conn: &Connection, where_sql: &str, dedupe_enabled: bool) -> Result<i64> {
    let detail_source = chart_detail_source_table_sql(where_sql, dedupe_enabled);
    let sql = format!("SELECT CAST(COUNT(1) AS BIGINT) FROM {detail_source}");
    let mut stmt = conn.prepare(&sql)?;
    Ok(stmt
        .query_row([], |row| row.get::<_, Option<i64>>(0))?
        .unwrap_or(0))
}

fn build_detail_rows_sql(
    where_sql: &str,
    order_sql: &str,
    limit: i64,
    offset: i64,
    manual_mapping: &ChartDetailManualMappingSql,
    dedupe_enabled: bool,
) -> String {
    let detail_source = chart_detail_source_table_sql(where_sql, dedupe_enabled);
    format!(
        "WITH filtered AS (\
         SELECT \
           id, acct_key, txn_ts, txn_time, amount, balance, card_no, acct_no, account_open_name, opener_id_no, dc_val, \
           counterparty_acct, cash_flag, counterparty_name, counterparty_id_no, counterparty_bank, summary, \
           currency, branch_name, branch_code, location, is_success, voucher_no, terminal_no, ip_addr, mac_addr, counterparty_balance, \
           txn_id, log_id, voucher_type, voucher_id, teller_no, merchant_name, merchant_no, remark, txn_type, query_feedback_reason \
         FROM {detail_source} \
       ), base AS (\
         SELECT \
           d.id, d.acct_key, d.txn_ts, d.txn_time, d.amount, d.balance, d.card_no, d.acct_no, \
           {} AS account_open_name, {} AS opener_id_no, d.dc_val, \
           d.counterparty_acct, d.cash_flag, d.counterparty_name, d.counterparty_id_no, d.counterparty_bank, d.summary, \
           d.currency, d.branch_name, d.branch_code, d.location, d.is_success, d.voucher_no, d.terminal_no, d.ip_addr, d.mac_addr, d.counterparty_balance, \
           d.txn_id, d.log_id, d.voucher_type, d.voucher_id, d.teller_no, d.merchant_name, d.merchant_no, d.remark, d.txn_type, d.query_feedback_reason \
         FROM filtered d \
         {}\
       ) \
         SELECT \
           id, card_no, acct_no, account_open_name, opener_id_no, txn_time, amount, balance, dc_val, \
           counterparty_acct, cash_flag, counterparty_name, counterparty_id_no, counterparty_bank, summary, \
           currency, branch_name, branch_code, location, is_success, voucher_no, terminal_no, ip_addr, mac_addr, counterparty_balance, \
           txn_id, log_id, voucher_type, voucher_id, teller_no, merchant_name, merchant_no, remark, txn_type, query_feedback_reason, \
           CAST({ts} AS VARCHAR) AS txn_ts_str, \
           CAST(COUNT(1) OVER() AS BIGINT) AS total_count \
         FROM base \
         ORDER BY {order_sql} \
         LIMIT {limit} OFFSET {offset}",
        manual_mapping.account_open_name_expr,
        manual_mapping.opener_id_no_expr,
        manual_mapping.join_sql,
        ts = chart_ts_expr()
    )
}

fn chart_ts_expr() -> &'static str {
    "COALESCE(txn_ts, TRY_CAST(txn_time AS TIMESTAMP))"
}

impl ChartDetailManualMappingSql {
    fn empty() -> Self {
        Self {
            join_sql: String::new(),
            account_open_name_expr: "d.account_open_name".to_string(),
            opener_id_no_expr: "d.opener_id_no".to_string(),
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

    fn request(sort_col: &'static str) -> QueryChartDetailRowsRequest {
        QueryChartDetailRowsRequest {
            source: ChartSourceRequest {
                selected: vec!["CARD-001".to_string()],
                date_start: String::new(),
                date_end: String::new(),
                success_filter: "all".to_string(),
                cash_filter: "all".to_string(),
                chart_filters: Vec::new(),
            },
            direction_mode: "all".to_string(),
            sort_col,
            sort_dir: "asc",
            limit: 10,
            offset: 0,
        }
    }

    #[test]
    fn numeric_order_keeps_missing_state_last_without_zero_fill() {
        for sort_col in ["amount", "balance"] {
            let sql = request(sort_col).order_sql();
            assert!(sql.contains(&format!("{sort_col} IS NOT NULL")));
            assert!(sql.contains(&format!("isfinite({sort_col})")));
            assert!(sql.contains("NULLS LAST"));
            assert!(!sql.contains(&format!("COALESCE({sort_col}, 0")));
        }
    }
}
