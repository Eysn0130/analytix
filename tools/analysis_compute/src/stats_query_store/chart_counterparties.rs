use anyhow::{Context, Result};
use duckdb::Connection;
use serde_json::{json, Map, Value};

use super::args::QueryChartCounterpartiesArgs;
use super::chart_common::{
    build_chart_context_where_sql, chart_dedupe_enabled, chart_detail_source_table_sql,
    counterparty_label_expr, metric_value_from_counters, normalize_direction_mode,
    normalize_metric_mode, normalize_selected_keys, require_chart_amount_query_ready,
    require_chart_scope, round2, ChartSourceRequest,
};
use super::session::VerifiedStatsQuerySession;

struct ChartCounterpartiesRequest {
    source: ChartSourceRequest,
    metric_mode: &'static str,
    direction_mode: &'static str,
}

struct CounterpartyItemRow {
    label: String,
    bank: String,
    name: String,
    account: String,
    in_amount: f64,
    out_amount: f64,
    in_count: i64,
    out_count: i64,
    in_counterparty_count: i64,
    out_counterparty_count: i64,
    all_counterparty_count: i64,
}

#[derive(Default)]
struct CounterpartyViewRows {
    name: Vec<CounterpartyItemRow>,
    account: Vec<CounterpartyItemRow>,
    bank: Vec<CounterpartyItemRow>,
}

struct CounterpartyQueryRow {
    view_key: String,
    item: CounterpartyItemRow,
}

impl ChartCounterpartiesRequest {
    fn from_args(args: &QueryChartCounterpartiesArgs) -> Self {
        let selected = normalize_selected_keys(&args.selected_keys);
        Self {
            source: ChartSourceRequest {
                selected,
                date_start: args.date_start.trim().to_string(),
                date_end: args.date_end.trim().to_string(),
                success_filter: args.success_filter.trim().to_string(),
                cash_filter: args.cash_filter.trim().to_string(),
                chart_filters: args.chart_filters.clone(),
            },
            metric_mode: normalize_metric_mode(&args.metric_mode),
            direction_mode: normalize_direction_mode(&args.direction_mode),
        }
    }
}

pub(crate) fn query_chart_counterparties(args: &QueryChartCounterpartiesArgs) -> Result<Value> {
    let request = ChartCounterpartiesRequest::from_args(args);
    require_chart_scope(&request.source.selected)?;

    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let result = query_chart_counterparties_for_request(&mut session, &request)?;
    session.commit()?;
    Ok(result)
}

pub(crate) fn query_chart_counterparties_with_session(
    session: &mut VerifiedStatsQuerySession<'_>,
    args: &QueryChartCounterpartiesArgs,
) -> Result<Value> {
    let request = ChartCounterpartiesRequest::from_args(args);
    require_chart_scope(&request.source.selected)?;
    query_chart_counterparties_for_request(session, &request)
}

fn query_chart_counterparties_for_request(
    session: &mut VerifiedStatsQuerySession<'_>,
    request: &ChartCounterpartiesRequest,
) -> Result<Value> {
    let where_sql = build_counterparties_where_sql(request);
    let dedupe_enabled = chart_dedupe_enabled(session.conn(), &request.source.selected)?;
    require_chart_amount_query_ready(session, &where_sql, dedupe_enabled)?;
    let views = query_counterparty_views(session.conn(), &where_sql, request, dedupe_enabled)?;
    build_chart_counterparties_payload(request, views)
}

fn build_counterparties_where_sql(request: &ChartCounterpartiesRequest) -> String {
    let mut where_sql = build_chart_context_where_sql(&request.source);
    if request.direction_mode == "in" {
        where_sql.push_str(" AND dc_val = '进'");
    } else if request.direction_mode == "out" {
        where_sql.push_str(" AND dc_val = '出'");
    }
    where_sql
}

fn query_counterparty_views(
    conn: &Connection,
    where_sql: &str,
    request: &ChartCounterpartiesRequest,
    dedupe_enabled: bool,
) -> Result<Map<String, Value>> {
    let rows = query_counterparty_view_rows(conn, where_sql, dedupe_enabled)?;
    Ok(counterparty_views_to_json(rows, request))
}

fn query_counterparty_view_rows(
    conn: &Connection,
    where_sql: &str,
    dedupe_enabled: bool,
) -> Result<CounterpartyViewRows> {
    let sql = counterparty_views_sql(where_sql, dedupe_enabled);
    let mut stmt = conn.prepare(&sql)?;
    let mapped = stmt.query_map([], counterparty_query_row_from_row)?;
    let mut rows = CounterpartyViewRows::default();
    for item in mapped {
        let item = item?;
        match item.view_key.as_str() {
            "name" => rows.name.push(item.item),
            "account" => rows.account.push(item.item),
            "bank" => rows.bank.push(item.item),
            _ => {}
        }
    }
    Ok(rows)
}

fn counterparty_views_to_json(
    rows: CounterpartyViewRows,
    request: &ChartCounterpartiesRequest,
) -> Map<String, Value> {
    [
        ("name", rows.name),
        ("account", rows.account),
        ("bank", rows.bank),
    ]
    .into_iter()
    .map(|(view_key, rows)| (view_key.to_string(), view_to_json(rows, request)))
    .collect::<Map<String, Value>>()
}

fn counterparty_views_sql(where_sql: &str, dedupe_enabled: bool) -> String {
    let cp_label_expr = counterparty_label_expr();
    let txn_ts = counterparty_ts_expr();
    let name_label = counterparty_label_expr();
    let account_label = fallback_label_expr("counterparty_acct", "未知账号");
    let bank_label = fallback_label_expr("counterparty_bank", "未知银行");
    let aggregate_columns = counterparty_aggregate_columns();
    let detail_source = chart_detail_source_table_sql(where_sql, dedupe_enabled);
    format!(
        "
        WITH source AS (
          SELECT
            COALESCE(NULLIF(TRIM(counterparty_bank), ''), '') AS bank,
            COALESCE(NULLIF(TRIM(counterparty_name), ''), '') AS name,
            COALESCE(NULLIF(TRIM(counterparty_acct), ''), '') AS account,
            {name_label} AS name_label,
            {account_label} AS account_label,
            {bank_label} AS bank_label,
            dc_val,
            ABS(amount) AS amount_abs,
            {cp_label_expr} AS counterparty_label,
            {txn_ts} AS txn_ts_sort,
            COALESCE(NULLIF(TRIM(txn_id), ''), CAST(id AS VARCHAR)) AS txn_sort_id
          FROM {detail_source}
        ), view_input AS (
          SELECT 'name' AS view_key, name_label AS label, bank, name, account, dc_val, amount_abs, counterparty_label, txn_ts_sort, txn_sort_id FROM source
          UNION ALL
          SELECT 'account' AS view_key, account_label AS label, bank, name, account, dc_val, amount_abs, counterparty_label, txn_ts_sort, txn_sort_id FROM source
          UNION ALL
          SELECT 'bank' AS view_key, bank_label AS label, bank, name, account, dc_val, amount_abs, counterparty_label, txn_ts_sort, txn_sort_id FROM source
        ), ranked AS (
          SELECT
            *,
            ROW_NUMBER() OVER (
              PARTITION BY view_key, label
              ORDER BY (txn_ts_sort IS NULL) ASC, txn_ts_sort ASC, txn_sort_id ASC
            ) AS label_row_num
          FROM view_input
        ), grouped AS (
          SELECT
            view_key,
            label,
            COALESCE(MAX(CASE WHEN label_row_num = 1 THEN bank ELSE NULL END), '') AS bank,
            COALESCE(MAX(CASE WHEN label_row_num = 1 THEN name ELSE NULL END), '') AS name,
            COALESCE(MAX(CASE WHEN label_row_num = 1 THEN account ELSE NULL END), '') AS account,
            {aggregate_columns},
            CAST(SUM(CASE WHEN dc_val IN ('进','出') THEN amount_abs ELSE 0 END) AS DOUBLE) AS sort_amount
          FROM ranked
          GROUP BY view_key, label
        )
        SELECT
          view_key,
          label,
          bank,
          name,
          account,
          in_amount,
          out_amount,
          in_count,
          out_count,
          in_counterparty_count,
          out_counterparty_count,
          all_counterparty_count
        FROM grouped
        ORDER BY
          CASE
            WHEN view_key = 'name' THEN 0
            WHEN view_key = 'account' THEN 1
            ELSE 2
          END ASC,
          sort_amount DESC,
          label ASC
        ",
    )
}

fn counterparty_query_row_from_row(row: &duckdb::Row<'_>) -> duckdb::Result<CounterpartyQueryRow> {
    Ok(CounterpartyQueryRow {
        view_key: row.get::<_, Option<String>>(0)?.unwrap_or_default(),
        item: CounterpartyItemRow {
            label: row.get::<_, Option<String>>(1)?.unwrap_or_default(),
            bank: row.get::<_, Option<String>>(2)?.unwrap_or_default(),
            name: row.get::<_, Option<String>>(3)?.unwrap_or_default(),
            account: row.get::<_, Option<String>>(4)?.unwrap_or_default(),
            in_amount: row.get::<_, f64>(5)?,
            out_amount: row.get::<_, f64>(6)?,
            in_count: row.get::<_, i64>(7)?,
            out_count: row.get::<_, i64>(8)?,
            in_counterparty_count: row.get::<_, i64>(9)?,
            out_counterparty_count: row.get::<_, i64>(10)?,
            all_counterparty_count: row.get::<_, i64>(11)?,
        },
    })
}

fn counterparty_aggregate_columns() -> &'static str {
    "CAST(SUM(CASE WHEN dc_val = '进' THEN amount_abs ELSE 0 END) AS DOUBLE) AS in_amount, \
     CAST(SUM(CASE WHEN dc_val = '出' THEN amount_abs ELSE 0 END) AS DOUBLE) AS out_amount, \
     CAST(COUNT(CASE WHEN dc_val = '进' THEN 1 END) AS BIGINT) AS in_count, \
     CAST(COUNT(CASE WHEN dc_val = '出' THEN 1 END) AS BIGINT) AS out_count, \
     CAST(COUNT(DISTINCT CASE WHEN dc_val = '进' THEN counterparty_label END) AS BIGINT) AS in_counterparty_count, \
     CAST(COUNT(DISTINCT CASE WHEN dc_val = '出' THEN counterparty_label END) AS BIGINT) AS out_counterparty_count, \
     CAST(COUNT(DISTINCT counterparty_label) AS BIGINT) AS all_counterparty_count"
}

fn view_to_json(rows: Vec<CounterpartyItemRow>, request: &ChartCounterpartiesRequest) -> Value {
    let all_items = rows
        .iter()
        .map(|item| item_to_json(item, request))
        .collect::<Vec<_>>();
    let top_items = all_items.iter().take(10).cloned().collect::<Vec<_>>();
    json!({
        "top_items": top_items,
        "all_items": all_items,
    })
}

fn build_chart_counterparties_payload(
    request: &ChartCounterpartiesRequest,
    views: Map<String, Value>,
) -> Result<Value> {
    let name_view = views.get("name").unwrap_or(&Value::Null);
    let top_items = name_view
        .get("top_items")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    let all_items = name_view
        .get("all_items")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    let total_top = top_items
        .iter()
        .map(item_abs_value)
        .collect::<Result<Vec<_>>>()?
        .into_iter()
        .sum::<f64>();
    let total_all = all_items
        .iter()
        .map(item_abs_value)
        .collect::<Result<Vec<_>>>()?
        .into_iter()
        .sum::<f64>();
    Ok(json!({
        "metric_mode": request.metric_mode,
        "direction_mode": request.direction_mode,
        "default_view": "name",
        "views": views,
        "top10_concentration": if total_all > 0.0 { round2((total_top / total_all) * 100.0) } else { 0.0 },
    }))
}

fn item_to_json(item: &CounterpartyItemRow, request: &ChartCounterpartiesRequest) -> Value {
    let total_amount = item.in_amount + item.out_amount;
    let total_count = item.in_count + item.out_count;
    let value = metric_value_from_counters(
        request.metric_mode,
        request.direction_mode,
        item.in_amount,
        item.out_amount,
        item.in_count,
        item.out_count,
        item.in_counterparty_count,
        item.out_counterparty_count,
        item.all_counterparty_count,
    );
    json!({
        "label": item.label,
        "bank": item.bank,
        "name": item.name,
        "account": item.account,
        "in_amount": round2(item.in_amount),
        "out_amount": round2(item.out_amount),
        "net_amount": round2(item.in_amount - item.out_amount),
        "total_amount": round2(total_amount),
        "in_count": item.in_count,
        "out_count": item.out_count,
        "total_count": total_count,
        "value": round2(value),
    })
}

fn item_abs_value(item: &Value) -> Result<f64> {
    let Some(value) = item.get("value").and_then(Value::as_f64) else {
        anyhow::bail!("stats_query_numeric_state_invalid");
    };
    if !value.is_finite() {
        anyhow::bail!("stats_query_numeric_state_invalid");
    }
    Ok(value.abs())
}

fn fallback_label_expr(column: &str, empty_label: &str) -> String {
    format!(
        "COALESCE(NULLIF(TRIM({column}), ''), {})",
        crate::sql_literal(empty_label)
    )
}

fn counterparty_ts_expr() -> &'static str {
    "COALESCE(txn_ts, TRY_CAST(txn_time AS TIMESTAMP))"
}
