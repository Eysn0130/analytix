use anyhow::{Context, Result};
use duckdb::Connection;
use serde_json::{json, Value};

use super::args::QueryChartHeatmapArgs;
use super::chart_common::{
    build_chart_context_where_sql, chart_dedupe_enabled, chart_detail_source_table_sql,
    counterparty_label_expr, metric_value_from_counters, normalize_direction_mode,
    normalize_metric_mode, normalize_selected_keys, require_chart_amount_query_ready,
    require_chart_scope, round2, ChartSourceRequest,
};
use super::session::VerifiedStatsQuerySession;

struct ChartHeatmapRequest {
    source: ChartSourceRequest,
    metric_mode: &'static str,
    direction_mode: &'static str,
}

struct HeatmapBucketRow {
    in_amount: f64,
    out_amount: f64,
    in_count: i64,
    out_count: i64,
    in_counterparty_count: i64,
    out_counterparty_count: i64,
    all_counterparty_count: i64,
}

#[derive(Default)]
struct HeatmapBuckets {
    cells: Vec<(i64, i64, HeatmapBucketRow)>,
    hours: Vec<(i64, HeatmapBucketRow)>,
    weekdays: Vec<(i64, HeatmapBucketRow)>,
}

struct HeatmapQueryRow {
    section: String,
    weekday: i64,
    hour: i64,
    bucket: HeatmapBucketRow,
}

impl ChartHeatmapRequest {
    fn from_args(args: &QueryChartHeatmapArgs) -> Self {
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

pub(crate) fn query_chart_heatmap(args: &QueryChartHeatmapArgs) -> Result<Value> {
    let request = ChartHeatmapRequest::from_args(args);
    require_chart_scope(&request.source.selected)?;

    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let result = query_chart_heatmap_for_request(&mut session, &request)?;
    session.commit()?;
    Ok(result)
}

pub(crate) fn query_chart_heatmap_with_session(
    session: &mut VerifiedStatsQuerySession<'_>,
    args: &QueryChartHeatmapArgs,
) -> Result<Value> {
    let request = ChartHeatmapRequest::from_args(args);
    require_chart_scope(&request.source.selected)?;
    query_chart_heatmap_for_request(session, &request)
}

fn query_chart_heatmap_for_request(
    session: &mut VerifiedStatsQuerySession<'_>,
    request: &ChartHeatmapRequest,
) -> Result<Value> {
    let where_sql = build_chart_context_where_sql(&request.source);
    let dedupe_enabled = chart_dedupe_enabled(session.conn(), &request.source.selected)?;
    require_chart_amount_query_ready(session, &where_sql, dedupe_enabled)?;
    let buckets = query_heatmap_buckets(session.conn(), &where_sql, dedupe_enabled)?;
    Ok(build_chart_heatmap_payload(
        request,
        buckets.cells,
        buckets.hours,
        buckets.weekdays,
    ))
}

fn query_heatmap_buckets(
    conn: &Connection,
    where_sql: &str,
    dedupe_enabled: bool,
) -> Result<HeatmapBuckets> {
    let sql = heatmap_buckets_sql(where_sql, dedupe_enabled);
    let mut stmt = conn.prepare(&sql)?;
    let mapped = stmt.query_map([], heatmap_query_row_from_row)?;
    let mut buckets = HeatmapBuckets::default();
    for item in mapped {
        let row = item?;
        match row.section.as_str() {
            "cell" => buckets.cells.push((row.weekday, row.hour, row.bucket)),
            "hour" => buckets.hours.push((row.hour, row.bucket)),
            "weekday" => buckets.weekdays.push((row.weekday, row.bucket)),
            _ => {}
        }
    }
    Ok(buckets)
}

fn heatmap_buckets_sql(where_sql: &str, dedupe_enabled: bool) -> String {
    let ts = heatmap_ts_expr();
    let cp_label_expr = counterparty_label_expr();
    let aggregate_columns = heatmap_aggregate_columns();
    let detail_source = chart_detail_source_table_sql(where_sql, dedupe_enabled);
    format!(
        "
        WITH filtered AS (
          SELECT
            ((CAST(STRFTIME({ts}, '%w') AS BIGINT) + 6) % 7) AS weekday,
            CAST(STRFTIME({ts}, '%H') AS BIGINT) AS hour,
            dc_val,
            ABS(amount) AS amount_abs,
            {cp_label_expr} AS counterparty
          FROM {detail_source}
          WHERE {ts} IS NOT NULL
        ), buckets AS (
          SELECT
            'cell' AS section,
            weekday,
            hour,
            {aggregate_columns}
          FROM filtered
          GROUP BY weekday, hour
          UNION ALL
          SELECT
            'hour' AS section,
            CAST(NULL AS BIGINT) AS weekday,
            hour,
            {aggregate_columns}
          FROM filtered
          GROUP BY hour
          UNION ALL
          SELECT
            'weekday' AS section,
            weekday,
            CAST(NULL AS BIGINT) AS hour,
            {aggregate_columns}
          FROM filtered
          GROUP BY weekday
        )
        SELECT
          section,
          COALESCE(weekday, -1) AS weekday,
          COALESCE(hour, -1) AS hour,
          in_amount,
          out_amount,
          in_count,
          out_count,
          in_counterparty_count,
          out_counterparty_count,
          all_counterparty_count
        FROM buckets
        ORDER BY
          CASE
            WHEN section = 'cell' THEN 0
            WHEN section = 'hour' THEN 1
            ELSE 2
          END ASC,
          weekday ASC,
          hour ASC
        ",
    )
}

fn heatmap_query_row_from_row(row: &duckdb::Row<'_>) -> duckdb::Result<HeatmapQueryRow> {
    Ok(HeatmapQueryRow {
        section: row.get::<_, Option<String>>(0)?.unwrap_or_default(),
        weekday: row.get::<_, i64>(1)?,
        hour: row.get::<_, i64>(2)?,
        bucket: bucket_from_row(row, 3)?,
    })
}

fn heatmap_aggregate_columns() -> &'static str {
    "CAST(SUM(CASE WHEN dc_val = '进' THEN amount_abs ELSE 0 END) AS DOUBLE) AS in_amount, \
     CAST(SUM(CASE WHEN dc_val = '出' THEN amount_abs ELSE 0 END) AS DOUBLE) AS out_amount, \
     CAST(COUNT(CASE WHEN dc_val = '进' THEN 1 END) AS BIGINT) AS in_count, \
     CAST(COUNT(CASE WHEN dc_val = '出' THEN 1 END) AS BIGINT) AS out_count, \
     CAST(COUNT(DISTINCT CASE WHEN dc_val = '进' THEN counterparty END) AS BIGINT) AS in_counterparty_count, \
     CAST(COUNT(DISTINCT CASE WHEN dc_val = '出' THEN counterparty END) AS BIGINT) AS out_counterparty_count, \
     CAST(COUNT(DISTINCT counterparty) AS BIGINT) AS all_counterparty_count"
}

fn bucket_from_row(row: &duckdb::Row<'_>, offset: usize) -> duckdb::Result<HeatmapBucketRow> {
    Ok(HeatmapBucketRow {
        in_amount: row.get::<_, f64>(offset)?,
        out_amount: row.get::<_, f64>(offset + 1)?,
        in_count: row.get::<_, i64>(offset + 2)?,
        out_count: row.get::<_, i64>(offset + 3)?,
        in_counterparty_count: row.get::<_, i64>(offset + 4)?,
        out_counterparty_count: row.get::<_, i64>(offset + 5)?,
        all_counterparty_count: row.get::<_, i64>(offset + 6)?,
    })
}

fn build_chart_heatmap_payload(
    request: &ChartHeatmapRequest,
    cell_rows: Vec<(i64, i64, HeatmapBucketRow)>,
    hour_rows: Vec<(i64, HeatmapBucketRow)>,
    weekday_rows: Vec<(i64, HeatmapBucketRow)>,
) -> Value {
    let mut cells = Vec::new();
    for (weekday, hour, row) in cell_rows {
        let cell_value = bucket_value(&row, request.metric_mode, request.direction_mode);
        cells.push(json!({
            "weekday": weekday,
            "hour": hour,
            "in_amount": round2(row.in_amount),
            "out_amount": round2(row.out_amount),
            "in_count": row.in_count,
            "out_count": row.out_count,
            "value": round2(cell_value),
        }));
    }

    let hours = hour_rows
        .into_iter()
        .filter_map(|(hour, bucket)| {
            hour_index(hour).map(|index| {
                let label = format!("{index:02}:00");
                bucket_to_json(&bucket, &label, request.metric_mode, request.direction_mode)
            })
        })
        .collect::<Vec<_>>();
    let weekday_labels = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"];
    let weekdays = weekday_rows
        .into_iter()
        .filter_map(|(weekday, bucket)| {
            weekday_index(weekday).map(|index| {
                bucket_to_json(
                    &bucket,
                    weekday_labels[index],
                    request.metric_mode,
                    request.direction_mode,
                )
            })
        })
        .collect::<Vec<_>>();

    json!({
        "cells": cells,
        "hours": hours,
        "weekdays": weekdays,
        "metric_mode": request.metric_mode,
        "direction_mode": request.direction_mode,
    })
}

fn bucket_to_json(
    bucket: &HeatmapBucketRow,
    label: &str,
    metric_mode: &str,
    direction_mode: &str,
) -> Value {
    let value = bucket_value(bucket, metric_mode, direction_mode);
    json!({
        "label": label,
        "in_amount": round2(bucket.in_amount),
        "out_amount": round2(bucket.out_amount),
        "in_count": bucket.in_count,
        "out_count": bucket.out_count,
        "value": round2(value),
    })
}

fn bucket_value(bucket: &HeatmapBucketRow, metric_mode: &str, direction_mode: &str) -> f64 {
    metric_value_from_counters(
        metric_mode,
        direction_mode,
        bucket.in_amount,
        bucket.out_amount,
        bucket.in_count,
        bucket.out_count,
        bucket.in_counterparty_count,
        bucket.out_counterparty_count,
        bucket.all_counterparty_count,
    )
}

fn hour_index(value: i64) -> Option<usize> {
    if (0..=23).contains(&value) {
        Some(value as usize)
    } else {
        None
    }
}

fn weekday_index(value: i64) -> Option<usize> {
    if (0..=6).contains(&value) {
        Some(value as usize)
    } else {
        None
    }
}

fn heatmap_ts_expr() -> &'static str {
    "COALESCE(txn_ts, TRY_CAST(txn_time AS TIMESTAMP))"
}
