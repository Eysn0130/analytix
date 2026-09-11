use anyhow::{Context, Result};
use duckdb::Connection;
use serde_json::{json, Value};
use std::collections::HashMap;

use super::args::QueryChartTrendArgs;
use super::chart_common::{
    build_chart_detail_where_sql, chart_dedupe_enabled, chart_detail_source_table_sql,
    counterparty_label_expr, metric_value_from_counters, normalize_direction_mode,
    normalize_granularity, normalize_metric_mode, normalize_selected_keys,
    normalize_selection_mode, require_chart_amount_query_ready, require_chart_scope, round2,
    ChartSourceRequest,
};
use super::session::VerifiedStatsQuerySession;

struct ChartTrendRequest {
    source: ChartSourceRequest,
    metric_mode: &'static str,
    direction_mode: &'static str,
    granularity: &'static str,
    selection_mode: String,
}

struct TrendBucketRow {
    bucket: String,
    label: String,
    in_amount: f64,
    out_amount: f64,
    in_count: i64,
    out_count: i64,
    in_counterparty_count: i64,
    out_counterparty_count: i64,
    all_counterparty_count: i64,
}

struct TrendQueryData {
    buckets: Vec<TrendBucketRow>,
    latest_balance_by_bucket: HashMap<String, f64>,
    balance_markers: Vec<Value>,
}

struct TrendQueryRow {
    section: String,
    bucket: String,
    label: String,
    in_amount: f64,
    out_amount: f64,
    in_count: i64,
    out_count: i64,
    in_counterparty_count: i64,
    out_counterparty_count: i64,
    all_counterparty_count: i64,
    txn_id: String,
    txn_time: String,
    amount: f64,
    balance: f64,
    direction: String,
    counterparty: String,
    summary: String,
}

struct BalanceMarker {
    value: Value,
    amount_rank: f64,
    delta_rank: f64,
    txn_time: String,
}

impl ChartTrendRequest {
    fn from_args(args: &QueryChartTrendArgs) -> Self {
        let selected = normalize_selected_keys(&args.selected_keys);
        Self {
            selection_mode: normalize_selection_mode(&selected, &args.selection_mode),
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
            granularity: normalize_granularity(&args.granularity),
        }
    }
}

pub(crate) fn query_chart_trend(args: &QueryChartTrendArgs) -> Result<Value> {
    let request = ChartTrendRequest::from_args(args);
    require_chart_scope(&request.source.selected)?;

    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let result = query_chart_trend_for_request(&mut session, &request)?;
    session.commit()?;
    Ok(result)
}

pub(crate) fn query_chart_trend_with_session(
    session: &mut VerifiedStatsQuerySession<'_>,
    args: &QueryChartTrendArgs,
) -> Result<Value> {
    let request = ChartTrendRequest::from_args(args);
    require_chart_scope(&request.source.selected)?;
    query_chart_trend_for_request(session, &request)
}

fn query_chart_trend_for_request(
    session: &mut VerifiedStatsQuerySession<'_>,
    request: &ChartTrendRequest,
) -> Result<Value> {
    let where_sql = build_chart_detail_where_sql(&request.source, None);
    let dedupe_enabled = chart_dedupe_enabled(session.conn(), &request.source.selected)?;
    require_chart_amount_query_ready(session, &where_sql, dedupe_enabled)?;
    let trend_data = query_trend_data(
        session.conn(),
        &where_sql,
        request.granularity,
        dedupe_enabled,
    )?;
    Ok(build_chart_trend_payload(
        request,
        trend_data.buckets,
        trend_data.latest_balance_by_bucket,
        trend_data.balance_markers,
    ))
}

fn query_trend_data(
    conn: &Connection,
    where_sql: &str,
    granularity: &str,
    dedupe_enabled: bool,
) -> Result<TrendQueryData> {
    let sql = trend_data_sql(where_sql, granularity, dedupe_enabled);
    let mut stmt = conn.prepare(&sql)?;
    let mapped = stmt.query_map([], trend_query_row_from_row)?;

    let mut buckets = Vec::new();
    let mut latest_balance_by_bucket = HashMap::new();
    let mut ranked_markers: Vec<BalanceMarker> = Vec::new();
    let mut previous_balance: Option<f64> = None;

    for item in mapped {
        let row = item?;
        if row.section == "bucket" {
            buckets.push(TrendBucketRow {
                bucket: row.bucket,
                label: row.label,
                in_amount: row.in_amount,
                out_amount: row.out_amount,
                in_count: row.in_count,
                out_count: row.out_count,
                in_counterparty_count: row.in_counterparty_count,
                out_counterparty_count: row.out_counterparty_count,
                all_counterparty_count: row.all_counterparty_count,
            });
            continue;
        }

        if row.section == "balance" {
            latest_balance_by_bucket.insert(row.bucket.clone(), row.balance);
            let balance_delta = previous_balance.map(|previous| row.balance - previous);
            previous_balance = Some(row.balance);
            let marker = json!({
                "txn_id": row.txn_id,
                "txn_time": row.txn_time,
                "bucket": row.bucket,
                "label": row.label,
                "amount": round2(row.amount),
                "balance": round2(row.balance),
                "balance_delta": balance_delta.map(round2),
                "direction": row.direction,
                "counterparty": row.counterparty,
                "summary": row.summary,
            });
            ranked_markers.push(BalanceMarker {
                value: marker,
                amount_rank: -row.amount.abs(),
                delta_rank: balance_delta.map(|delta| -delta.abs()).unwrap_or(f64::MAX),
                txn_time: row.txn_time,
            });
        }
    }

    ranked_markers.sort_by(|left, right| {
        left.amount_rank
            .partial_cmp(&right.amount_rank)
            .unwrap_or(std::cmp::Ordering::Equal)
            .then_with(|| {
                left.delta_rank
                    .partial_cmp(&right.delta_rank)
                    .unwrap_or(std::cmp::Ordering::Equal)
            })
            .then_with(|| left.txn_time.cmp(&right.txn_time))
    });
    let balance_markers = ranked_markers
        .into_iter()
        .take(6)
        .map(|item| item.value)
        .collect::<Vec<_>>();

    Ok(TrendQueryData {
        buckets,
        latest_balance_by_bucket,
        balance_markers,
    })
}

fn trend_data_sql(where_sql: &str, granularity: &str, dedupe_enabled: bool) -> String {
    let (bucket_expr, label_expr) = trend_bucket_sql(granularity);
    let cp_label_expr = counterparty_label_expr();
    let aggregate_columns = trend_aggregate_columns();
    let detail_source = chart_detail_source_table_sql(where_sql, dedupe_enabled);
    format!(
        "
        WITH filtered AS (
          SELECT
            {bucket_expr} AS bucket,
            {label_expr} AS label,
            id,
            COALESCE(NULLIF(TRIM(txn_id), ''), CAST(id AS VARCHAR)) AS txn_id,
            COALESCE(NULLIF(TRIM(txn_time), ''), CAST({txn_ts} AS VARCHAR), '') AS txn_time,
            ABS(amount) AS amount_abs,
            balance,
            dc_val,
            {cp_label_expr} AS counterparty,
            COALESCE(summary, '') AS summary,
            {txn_ts} AS txn_ts_sort
          FROM {detail_source}
        ), bucket_rows AS (
          SELECT
            bucket,
            label,
            {aggregate_columns}
          FROM filtered
          GROUP BY bucket, label
        ), balance_rows AS (
          SELECT
            bucket,
            label,
            id,
            txn_id,
            txn_time,
            amount_abs,
            balance,
            dc_val,
            counterparty,
            summary,
            txn_ts_sort
          FROM filtered
          WHERE balance IS NOT NULL AND isfinite(balance)
        )
        SELECT
          section,
          bucket,
          label,
          in_amount,
          out_amount,
          in_count,
          out_count,
          in_counterparty_count,
          out_counterparty_count,
          all_counterparty_count,
          txn_id,
          txn_time,
          amount,
          balance,
          direction,
          counterparty,
          summary
        FROM (
          SELECT
            0 AS section_order,
            bucket AS bucket_order,
            FALSE AS txn_ts_null,
            CAST(NULL AS TIMESTAMP) AS txn_ts_sort,
            0 AS id,
            'bucket' AS section,
            bucket,
            label,
            in_amount,
            out_amount,
            in_count,
            out_count,
            in_counterparty_count,
            out_counterparty_count,
            all_counterparty_count,
            '' AS txn_id,
            '' AS txn_time,
            0.0 AS amount,
            0.0 AS balance,
            '' AS direction,
            '' AS counterparty,
            '' AS summary
          FROM bucket_rows
          UNION ALL
          SELECT
            1 AS section_order,
            bucket AS bucket_order,
            (txn_ts_sort IS NULL) AS txn_ts_null,
            txn_ts_sort,
            id,
            'balance' AS section,
            bucket,
            label,
            0.0 AS in_amount,
            0.0 AS out_amount,
            0 AS in_count,
            0 AS out_count,
            0 AS in_counterparty_count,
            0 AS out_counterparty_count,
            0 AS all_counterparty_count,
            txn_id,
            txn_time,
            amount_abs AS amount,
            balance,
            CASE WHEN dc_val = '进' THEN 'in' WHEN dc_val = '出' THEN 'out' ELSE 'other' END AS direction,
            counterparty,
            summary
          FROM balance_rows
        )
        ORDER BY
          section_order ASC,
          CASE WHEN section = 'bucket' THEN bucket_order ELSE '' END ASC,
          txn_ts_null ASC,
          txn_ts_sort ASC,
          id ASC
        ",
        txn_ts = trend_ts_expr(),
    )
}

fn trend_query_row_from_row(row: &duckdb::Row<'_>) -> duckdb::Result<TrendQueryRow> {
    Ok(TrendQueryRow {
        section: row.get::<_, Option<String>>(0)?.unwrap_or_default(),
        bucket: row
            .get::<_, Option<String>>(1)?
            .unwrap_or_else(|| "未知".to_string()),
        label: row
            .get::<_, Option<String>>(2)?
            .unwrap_or_else(|| "未知".to_string()),
        in_amount: row.get::<_, f64>(3)?,
        out_amount: row.get::<_, f64>(4)?,
        in_count: row.get::<_, i64>(5)?,
        out_count: row.get::<_, i64>(6)?,
        in_counterparty_count: row.get::<_, i64>(7)?,
        out_counterparty_count: row.get::<_, i64>(8)?,
        all_counterparty_count: row.get::<_, i64>(9)?,
        txn_id: row.get::<_, Option<String>>(10)?.unwrap_or_default(),
        txn_time: row.get::<_, Option<String>>(11)?.unwrap_or_default(),
        amount: row.get::<_, f64>(12)?,
        balance: row.get::<_, f64>(13)?,
        direction: row.get::<_, Option<String>>(14)?.unwrap_or_default(),
        counterparty: row
            .get::<_, Option<String>>(15)?
            .unwrap_or_else(|| "未知对手".to_string()),
        summary: row.get::<_, Option<String>>(16)?.unwrap_or_default(),
    })
}

fn trend_aggregate_columns() -> &'static str {
    "CAST(SUM(CASE WHEN dc_val = '进' THEN amount_abs ELSE 0 END) AS DOUBLE) AS in_amount, \
     CAST(SUM(CASE WHEN dc_val = '出' THEN amount_abs ELSE 0 END) AS DOUBLE) AS out_amount, \
     CAST(COUNT(CASE WHEN dc_val = '进' THEN 1 END) AS BIGINT) AS in_count, \
     CAST(COUNT(CASE WHEN dc_val = '出' THEN 1 END) AS BIGINT) AS out_count, \
     CAST(COUNT(DISTINCT CASE WHEN dc_val = '进' THEN counterparty END) AS BIGINT) AS in_counterparty_count, \
     CAST(COUNT(DISTINCT CASE WHEN dc_val = '出' THEN counterparty END) AS BIGINT) AS out_counterparty_count, \
     CAST(COUNT(DISTINCT counterparty) AS BIGINT) AS all_counterparty_count"
}

fn build_chart_trend_payload(
    request: &ChartTrendRequest,
    buckets: Vec<TrendBucketRow>,
    latest_balance_by_bucket: HashMap<String, f64>,
    balance_markers: Vec<Value>,
) -> Value {
    let mut points = Vec::new();
    let mut balance_points = Vec::new();
    let mut cumulative_net_points = Vec::new();
    let mut cumulative_net = 0.0;
    let mut balance_available_count = 0_i64;

    for bucket in buckets {
        let bucket_key = bucket.bucket;
        let bucket_label = bucket.label;
        let latest_balance = latest_balance_by_bucket.get(&bucket_key).copied();
        if latest_balance.is_some() {
            balance_available_count += 1;
        }
        let total_amount = bucket.in_amount + bucket.out_amount;
        let total_count = bucket.in_count + bucket.out_count;
        cumulative_net += bucket.in_amount - bucket.out_amount;
        let value = metric_value_from_counters(
            request.metric_mode,
            request.direction_mode,
            bucket.in_amount,
            bucket.out_amount,
            bucket.in_count,
            bucket.out_count,
            bucket.in_counterparty_count,
            bucket.out_counterparty_count,
            bucket.all_counterparty_count,
        );
        points.push(json!({
            "bucket": bucket_key.clone(),
            "label": bucket_label.clone(),
            "in_amount": round2(bucket.in_amount),
            "out_amount": round2(bucket.out_amount),
            "net_amount": round2(bucket.in_amount - bucket.out_amount),
            "total_amount": round2(total_amount),
            "in_count": bucket.in_count,
            "out_count": bucket.out_count,
            "net_count": bucket.in_count - bucket.out_count,
            "total_count": total_count,
            "counterparty_count": bucket.all_counterparty_count,
            "latest_balance": latest_balance.map(round2),
            "value": round2(value),
        }));
        balance_points.push(json!({
            "bucket": bucket_key.clone(),
            "label": bucket_label.clone(),
            "balance": latest_balance.map(round2),
        }));
        cumulative_net_points.push(json!({
            "bucket": bucket_key,
            "label": bucket_label,
            "value": round2(cumulative_net),
        }));
    }

    let point_count = balance_points.len() as f64;
    let balance_coverage_rate = if point_count > 0.0 {
        round2((balance_available_count as f64 / point_count) * 100.0)
    } else {
        0.0
    };
    let balance_mode = if request.selection_mode == "single-card" && balance_available_count > 0 {
        "balance"
    } else {
        "cumulative-net"
    };

    json!({
        "granularity": request.granularity,
        "metric_mode": request.metric_mode,
        "direction_mode": request.direction_mode,
        "points": points,
        "balance_points": balance_points,
        "cumulative_net_points": cumulative_net_points,
        "balance_available": balance_available_count > 0,
        "balance_mode": balance_mode,
        "balance_coverage_rate": balance_coverage_rate,
        "balance_markers": balance_markers,
    })
}

fn trend_bucket_sql(granularity: &str) -> (String, String) {
    let ts = trend_ts_expr();
    match granularity {
        "hour" => {
            let expr = format!("STRFTIME({ts}, '%Y-%m-%d %H:00')");
            (
                format!("CASE WHEN {ts} IS NULL THEN '未知' ELSE {expr} END"),
                format!("CASE WHEN {ts} IS NULL THEN '未知' ELSE {expr} END"),
            )
        }
        "week" => {
            let key = format!("CAST(CAST(DATE_TRUNC('week', {ts}) AS DATE) AS VARCHAR)");
            (
                format!("CASE WHEN {ts} IS NULL THEN '未知' ELSE {key} END"),
                format!("CASE WHEN {ts} IS NULL THEN '未知' ELSE {key} || ' 周' END"),
            )
        }
        "month" => {
            let expr = format!("STRFTIME({ts}, '%Y-%m')");
            (
                format!("CASE WHEN {ts} IS NULL THEN '未知' ELSE {expr} END"),
                format!("CASE WHEN {ts} IS NULL THEN '未知' ELSE {expr} END"),
            )
        }
        _ => {
            let expr = format!("CAST(CAST({ts} AS DATE) AS VARCHAR)");
            (
                format!("CASE WHEN {ts} IS NULL THEN '未知' ELSE {expr} END"),
                format!("CASE WHEN {ts} IS NULL THEN '未知' ELSE {expr} END"),
            )
        }
    }
}

fn trend_ts_expr() -> &'static str {
    "COALESCE(txn_ts, TRY_CAST(txn_time AS TIMESTAMP))"
}
