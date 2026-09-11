use anyhow::{bail, Context, Result};
use duckdb::Connection;
use serde_json::{json, Value};

use super::args::QueryChartSummaryArgs;
use super::chart_common::{
    build_chart_context_where_sql, chart_dedupe_enabled, chart_detail_source_table_sql,
    normalize_selected_keys, normalize_selection_mode, require_chart_amount_query_ready,
    require_chart_scope, round2, ChartSourceRequest,
};
use super::session::VerifiedStatsQuerySession;

struct ChartSummaryRequest {
    case_id: String,
    source: ChartSourceRequest,
    selection_mode: String,
    large_txn_threshold: f64,
}

#[derive(Default)]
struct ChartSummaryAggregate {
    total_in: f64,
    total_out: f64,
    total_count: i64,
    success_count: i64,
    active_counterparty_count: i64,
    max_single_amount: f64,
    large_txn_count: i64,
}

struct LatestBalance {
    balance: f64,
    txn_time: String,
}

#[derive(Default)]
struct AccountIdentity {
    account_name: String,
    account_key: String,
    card_no: String,
}

struct ChartSummaryData {
    aggregate: ChartSummaryAggregate,
    latest_balance: Option<LatestBalance>,
    identity: AccountIdentity,
}

struct ChartSummaryQueryRow {
    section: String,
    aggregate: ChartSummaryAggregate,
    latest_balance: Option<LatestBalance>,
    identity: AccountIdentity,
}

impl ChartSummaryRequest {
    fn from_args(args: &QueryChartSummaryArgs) -> Self {
        let selected = normalize_selected_keys(&args.selected_keys);
        Self {
            case_id: args.case_id.trim().to_string(),
            selection_mode: normalize_selection_mode(&selected, &args.selection_mode),
            source: ChartSourceRequest {
                selected,
                date_start: args.date_start.trim().to_string(),
                date_end: args.date_end.trim().to_string(),
                success_filter: args.success_filter.trim().to_string(),
                cash_filter: args.cash_filter.trim().to_string(),
                chart_filters: args.chart_filters.clone(),
            },
            large_txn_threshold: args.large_txn_threshold,
        }
    }
}

pub(crate) fn query_chart_summary(args: &QueryChartSummaryArgs) -> Result<Value> {
    let request = ChartSummaryRequest::from_args(args);
    validate_large_txn_threshold(request.large_txn_threshold)?;
    require_chart_scope(&request.source.selected)?;

    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let result = query_chart_summary_for_request(&mut session, &request)?;
    session.commit()?;
    Ok(result)
}

pub(crate) fn query_chart_summary_with_session(
    session: &mut VerifiedStatsQuerySession<'_>,
    args: &QueryChartSummaryArgs,
) -> Result<Value> {
    let request = ChartSummaryRequest::from_args(args);
    validate_large_txn_threshold(request.large_txn_threshold)?;
    require_chart_scope(&request.source.selected)?;
    query_chart_summary_for_request(session, &request)
}

fn query_chart_summary_for_request(
    session: &mut VerifiedStatsQuerySession<'_>,
    request: &ChartSummaryRequest,
) -> Result<Value> {
    let conn = session.conn();
    let where_sql = build_chart_context_where_sql(&request.source);
    let dedupe_enabled = chart_dedupe_enabled(conn, &request.source.selected)?;
    require_chart_amount_query_ready(session, &where_sql, dedupe_enabled)?;
    let data = query_summary_data(
        session.conn(),
        &request.case_id,
        &where_sql,
        request.large_txn_threshold,
        request.source.selected.len(),
    )?;
    Ok(build_chart_summary_payload(
        &request,
        data.aggregate,
        data.latest_balance,
        data.identity,
    ))
}

fn query_summary_data(
    conn: &Connection,
    case_id: &str,
    where_sql: &str,
    large_txn_threshold: f64,
    selected_count: usize,
) -> Result<ChartSummaryData> {
    let manual_table_exists = crate::table_exists(conn, "analysis_manual_account_mapping")?;
    let account_dim_exists = crate::table_exists(conn, crate::ACCOUNT_DIM_TABLE)?;
    let sql = summary_data_sql(
        case_id,
        where_sql,
        large_txn_threshold,
        manual_table_exists,
        account_dim_exists,
        selected_count > 1 && account_dim_exists,
    );
    let mut stmt = conn.prepare(&sql)?;
    let mapped = stmt.query_map([], chart_summary_query_row_from_row)?;
    let mut data = ChartSummaryData {
        aggregate: ChartSummaryAggregate::default(),
        latest_balance: None,
        identity: AccountIdentity::default(),
    };
    for item in mapped {
        let row = item?;
        match row.section.as_str() {
            "aggregate" => data.aggregate = row.aggregate,
            "latest_balance" => data.latest_balance = row.latest_balance,
            "identity" => data.identity = row.identity,
            _ => {}
        }
    }
    Ok(data)
}

fn summary_data_sql(
    case_id: &str,
    where_sql: &str,
    large_txn_threshold: f64,
    manual_table_exists: bool,
    account_dim_exists: bool,
    dedupe_enabled: bool,
) -> String {
    let threshold_sql = finite_number_sql(large_txn_threshold);
    let txn_ts = trend_ts_expr();
    let detail_source = chart_detail_source_table_sql(where_sql, dedupe_enabled);
    let dim_join = if account_dim_exists {
        format!(
            "LEFT JOIN {} ad ON ad.account_key = d.acct_key",
            crate::ACCOUNT_DIM_TABLE
        )
    } else {
        String::new()
    };
    let dim_name_expr = if account_dim_exists {
        "NULLIF(TRIM(ad.open_name), ''), "
    } else {
        ""
    };
    let dim_card_expr = if account_dim_exists {
        "NULLIF(TRIM(ad.card_display), ''), "
    } else {
        ""
    };
    let identity_sql = if manual_table_exists {
        format!(
            "SELECT
               COALESCE(NULLIF(TRIM(m.account_open_name), ''), NULLIF(TRIM(d.account_open_name), ''), {dim_name_expr}'') AS account_name,
               COALESCE(NULLIF(TRIM(d.acct_no), ''), d.acct_key, '') AS account_key,
               COALESCE({dim_card_expr}NULLIF(TRIM(d.card_no), ''), '') AS card_no
             FROM filtered d
             {dim_join}
             LEFT JOIN analysis_manual_account_mapping m \
               ON m.case_id = {} \
              AND m.account_key = d.acct_key \
             WHERE (NULLIF(TRIM(COALESCE(m.account_open_name, d.account_open_name, '')), '') IS NOT NULL
                 OR NULLIF(TRIM(COALESCE(d.acct_no, '')), '') IS NOT NULL
                 OR NULLIF(TRIM(COALESCE(d.card_no, '')), '') IS NOT NULL)
             ORDER BY (d.summary_ts_sort IS NULL) ASC, d.summary_ts_sort DESC,
               d.summary_txn_sort_id DESC
             LIMIT 1",
            crate::sql_literal(case_id),
        )
    } else {
        format!(
            "SELECT
           COALESCE(NULLIF(TRIM(d.account_open_name), ''), {dim_name_expr}'') AS account_name,
           COALESCE(NULLIF(TRIM(d.acct_no), ''), d.acct_key, '') AS account_key,
           COALESCE({dim_card_expr}NULLIF(TRIM(d.card_no), ''), '') AS card_no
         FROM filtered d
         {dim_join}
         WHERE (NULLIF(TRIM(COALESCE(d.account_open_name, '')), '') IS NOT NULL
             OR NULLIF(TRIM(COALESCE(d.acct_no, '')), '') IS NOT NULL
             OR NULLIF(TRIM(COALESCE(d.card_no, '')), '') IS NOT NULL)
         ORDER BY (d.summary_ts_sort IS NULL) ASC, d.summary_ts_sort DESC,
           d.summary_txn_sort_id DESC
         LIMIT 1"
        )
    };
    format!(
        "
        WITH filtered AS (
          SELECT
            d.*,
            {txn_ts} AS summary_ts_sort,
            COALESCE(NULLIF(TRIM(txn_id), ''), CAST(id AS VARCHAR)) AS summary_txn_sort_id
          FROM {detail_source} d
        ), aggregate_row AS (
          SELECT
            CAST(SUM(CASE WHEN dc_val = '进' THEN ABS(amount) ELSE 0 END) AS DOUBLE) AS total_in,
            CAST(SUM(CASE WHEN dc_val = '出' THEN ABS(amount) ELSE 0 END) AS DOUBLE) AS total_out,
            CAST(COUNT(1) AS BIGINT) AS total_count,
            CAST(SUM(CASE WHEN LOWER(TRIM(COALESCE(is_success, ''))) IN ('是','成功','true','1','y','yes','success','ok') THEN 1 ELSE 0 END) AS BIGINT) AS success_count,
            CAST(COUNT(DISTINCT
              COALESCE(NULLIF(TRIM(counterparty_acct), ''), '未知账号') || '::' ||
              COALESCE(NULLIF(TRIM(counterparty_name), ''), '未知户名')
            ) AS BIGINT) AS active_counterparty_count,
            CAST(MAX(ABS(amount)) AS DOUBLE) AS max_single_amount,
            CAST(SUM(CASE WHEN ABS(amount) >= {threshold_sql} THEN 1 ELSE 0 END) AS BIGINT) AS large_txn_count
          FROM filtered
        ), latest_balance_row AS (
          SELECT
            balance,
            COALESCE(NULLIF(TRIM(txn_time), ''), CAST(summary_ts_sort AS VARCHAR), '') AS txn_time
          FROM filtered
          WHERE balance IS NOT NULL AND isfinite(balance)
          ORDER BY (summary_ts_sort IS NULL) DESC, summary_ts_sort DESC,
            summary_txn_sort_id DESC
          LIMIT 1
        ), identity_row AS (
          {identity_sql}
        )
        SELECT
          section,
          total_in,
          total_out,
          total_count,
          success_count,
          active_counterparty_count,
          max_single_amount,
          large_txn_count,
          balance,
          latest_txn_time,
          account_name,
          account_key,
          card_no
        FROM (
          SELECT
            0 AS section_order,
            'aggregate' AS section,
            total_in,
            total_out,
            total_count,
            success_count,
            active_counterparty_count,
            max_single_amount,
            large_txn_count,
            CAST(NULL AS DOUBLE) AS balance,
            '' AS latest_txn_time,
            '' AS account_name,
            '' AS account_key,
            '' AS card_no
          FROM aggregate_row
          UNION ALL
          SELECT
            1 AS section_order,
            'latest_balance' AS section,
            0.0 AS total_in,
            0.0 AS total_out,
            CAST(0 AS BIGINT) AS total_count,
            CAST(0 AS BIGINT) AS success_count,
            CAST(0 AS BIGINT) AS active_counterparty_count,
            0.0 AS max_single_amount,
            CAST(0 AS BIGINT) AS large_txn_count,
            balance,
            txn_time AS latest_txn_time,
            '' AS account_name,
            '' AS account_key,
            '' AS card_no
          FROM latest_balance_row
          UNION ALL
          SELECT
            2 AS section_order,
            'identity' AS section,
            0.0 AS total_in,
            0.0 AS total_out,
            CAST(0 AS BIGINT) AS total_count,
            CAST(0 AS BIGINT) AS success_count,
            CAST(0 AS BIGINT) AS active_counterparty_count,
            0.0 AS max_single_amount,
            CAST(0 AS BIGINT) AS large_txn_count,
            CAST(NULL AS DOUBLE) AS balance,
            '' AS latest_txn_time,
            account_name,
            account_key,
            card_no
          FROM identity_row
        )
        ORDER BY section_order ASC
        ",
    )
}

fn chart_summary_query_row_from_row(row: &duckdb::Row<'_>) -> duckdb::Result<ChartSummaryQueryRow> {
    let section = row.get::<_, Option<String>>(0)?.unwrap_or_default();
    let latest_balance = if section == "latest_balance" {
        Some(LatestBalance {
            balance: row.get::<_, f64>(8)?,
            txn_time: row.get::<_, Option<String>>(9)?.unwrap_or_default(),
        })
    } else {
        None
    };
    Ok(ChartSummaryQueryRow {
        section,
        aggregate: ChartSummaryAggregate {
            total_in: row.get::<_, f64>(1)?,
            total_out: row.get::<_, f64>(2)?,
            total_count: row.get::<_, i64>(3)?,
            success_count: row.get::<_, i64>(4)?,
            active_counterparty_count: row.get::<_, i64>(5)?,
            max_single_amount: row.get::<_, f64>(6)?,
            large_txn_count: row.get::<_, i64>(7)?,
        },
        latest_balance,
        identity: AccountIdentity {
            account_name: row.get::<_, Option<String>>(10)?.unwrap_or_default(),
            account_key: row.get::<_, Option<String>>(11)?.unwrap_or_default(),
            card_no: row.get::<_, Option<String>>(12)?.unwrap_or_default(),
        },
    })
}

fn build_chart_summary_payload(
    request: &ChartSummaryRequest,
    aggregate: ChartSummaryAggregate,
    latest_balance: Option<LatestBalance>,
    identity: AccountIdentity,
) -> Value {
    let success_rate = if aggregate.total_count > 0 {
        round2((aggregate.success_count as f64 / aggregate.total_count as f64) * 100.0)
    } else {
        0.0
    };
    json!({
        "total_in_amount": round2(aggregate.total_in),
        "total_out_amount": round2(aggregate.total_out),
        "net_in_amount": round2(aggregate.total_in - aggregate.total_out),
        "txn_total_count": aggregate.total_count,
        "active_counterparty_count": aggregate.active_counterparty_count,
        "success_rate": success_rate,
        "latest_balance": latest_balance.as_ref().map(|item| round2(item.balance)),
        "max_single_amount": round2(aggregate.max_single_amount),
        "large_txn_threshold": round2(request.large_txn_threshold),
        "large_txn_count": aggregate.large_txn_count,
        "latest_txn_time": latest_balance
            .as_ref()
            .map(|item| item.txn_time.clone())
            .unwrap_or_default(),
        "account_name": identity.account_name,
        "account_key": identity.account_key,
        "card_no": identity.card_no,
        "balance_available": latest_balance.is_some(),
        "selection_mode": request.selection_mode,
    })
}

fn finite_number_sql(value: f64) -> String {
    format!("{value:.6}")
}

fn validate_large_txn_threshold(value: f64) -> Result<()> {
    if !value.is_finite() || value < 0.0 {
        bail!("--large-txn-threshold must be finite and non-negative");
    }
    Ok(())
}

fn trend_ts_expr() -> &'static str {
    "COALESCE(txn_ts, TRY_CAST(txn_time AS TIMESTAMP))"
}
