use anyhow::Result;
use duckdb::Connection;
use serde_json::{json, Map, Value};

use super::args::QueryChartDashboardArgs;
use super::chart_common::{
    build_chart_context_where_sql, chart_dedupe_enabled, chart_detail_source_table_sql,
    counterparty_label_expr, metric_value_from_counters, normalize_direction_mode,
    normalize_metric_mode, normalize_selected_keys, require_chart_amount_query_ready,
    require_chart_scope, round2, ChartSourceRequest,
};
use super::session::VerifiedStatsQuerySession;

struct ChartStructureRequest {
    source: ChartSourceRequest,
    metric_mode: &'static str,
    direction_mode: &'static str,
}

struct StructureBucketRow {
    label: String,
    in_amount: f64,
    out_amount: f64,
    in_count: i64,
    out_count: i64,
    in_counterparty_count: i64,
    out_counterparty_count: i64,
    all_counterparty_count: i64,
}

#[derive(Default)]
struct StructureViewRows {
    direction: Vec<StructureBucketRow>,
    success: Vec<StructureBucketRow>,
    txn_type: Vec<StructureBucketRow>,
    currency: Vec<StructureBucketRow>,
}

struct StructureQueryRow {
    view_key: String,
    bucket: StructureBucketRow,
}

impl ChartStructureRequest {
    fn from_dashboard_args(args: &QueryChartDashboardArgs) -> Self {
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

pub(crate) fn query_chart_structure_with_session(
    session: &mut VerifiedStatsQuerySession<'_>,
    args: &QueryChartDashboardArgs,
) -> Result<Value> {
    let request = ChartStructureRequest::from_dashboard_args(args);
    require_chart_scope(&request.source.selected)?;

    let where_sql = build_chart_context_where_sql(&request.source);
    let dedupe_enabled = chart_dedupe_enabled(session.conn(), &request.source.selected)?;
    require_chart_amount_query_ready(session, &where_sql, dedupe_enabled)?;
    let rows = query_structure_views(session.conn(), &where_sql, dedupe_enabled)?;
    let views = vec![
        ("direction", rows_to_json(rows.direction, &request)),
        ("success", rows_to_json(rows.success, &request)),
        ("txn_type", rows_to_json(rows.txn_type, &request)),
        ("currency", rows_to_json(rows.currency, &request)),
    ];

    Ok(build_chart_structure_payload(&request, views))
}

fn query_structure_views(
    conn: &Connection,
    where_sql: &str,
    dedupe_enabled: bool,
) -> Result<StructureViewRows> {
    let sql = structure_views_sql(where_sql, dedupe_enabled);
    let mut stmt = conn.prepare(&sql)?;
    let mapped = stmt.query_map([], structure_query_row_from_row)?;
    let mut rows = StructureViewRows::default();
    for item in mapped {
        let item = item?;
        match item.view_key.as_str() {
            "direction" => rows.direction.push(item.bucket),
            "success" => rows.success.push(item.bucket),
            "txn_type" => rows.txn_type.push(item.bucket),
            "currency" => rows.currency.push(item.bucket),
            _ => {}
        }
    }
    Ok(rows)
}

fn structure_views_sql(where_sql: &str, dedupe_enabled: bool) -> String {
    let cp_label_expr = counterparty_label_expr();
    let direction_label = direction_label_expr();
    let success_label = success_label_expr();
    let txn_type_label = fallback_label_expr("txn_type", "未知类型");
    let currency_label = fallback_label_expr("currency", "未知币种");
    let aggregate_columns = structure_aggregate_columns();
    let detail_source = chart_detail_source_table_sql(where_sql, dedupe_enabled);
    let sql = format!(
        "
        WITH filtered AS (
          SELECT
            dc_val,
            ABS(amount) AS amount_abs,
            {cp_label_expr} AS counterparty,
            {direction_label} AS direction_label,
            {success_label} AS success_label,
            {txn_type_label} AS txn_type_label,
            {currency_label} AS currency_label
          FROM {detail_source}
        ), view_input AS (
          SELECT 'direction' AS view_key, direction_label AS label, dc_val, amount_abs, counterparty FROM filtered
          UNION ALL
          SELECT 'success' AS view_key, success_label AS label, dc_val, amount_abs, counterparty FROM filtered
          UNION ALL
          SELECT 'txn_type' AS view_key, txn_type_label AS label, dc_val, amount_abs, counterparty FROM filtered
          UNION ALL
          SELECT 'currency' AS view_key, currency_label AS label, dc_val, amount_abs, counterparty FROM filtered
        ), grouped AS (
          SELECT
            view_key,
            label,
            {aggregate_columns}
          FROM view_input
          GROUP BY view_key, label
        )
        SELECT
          view_key,
          label,
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
            WHEN view_key = 'direction' THEN 0
            WHEN view_key = 'success' THEN 1
            WHEN view_key = 'txn_type' THEN 2
            ELSE 3
          END ASC,
          label ASC
        ",
    );
    sql
}

fn structure_query_row_from_row(row: &duckdb::Row<'_>) -> duckdb::Result<StructureQueryRow> {
    Ok(StructureQueryRow {
        view_key: row.get::<_, Option<String>>(0)?.unwrap_or_default(),
        bucket: StructureBucketRow {
            label: row.get::<_, Option<String>>(1)?.unwrap_or_default(),
            in_amount: row.get::<_, f64>(2)?,
            out_amount: row.get::<_, f64>(3)?,
            in_count: row.get::<_, i64>(4)?,
            out_count: row.get::<_, i64>(5)?,
            in_counterparty_count: row.get::<_, i64>(6)?,
            out_counterparty_count: row.get::<_, i64>(7)?,
            all_counterparty_count: row.get::<_, i64>(8)?,
        },
    })
}

fn structure_aggregate_columns() -> &'static str {
    "CAST(SUM(CASE WHEN dc_val = '进' THEN amount_abs ELSE 0 END) AS DOUBLE) AS in_amount, \
     CAST(SUM(CASE WHEN dc_val = '出' THEN amount_abs ELSE 0 END) AS DOUBLE) AS out_amount, \
     CAST(COUNT(CASE WHEN dc_val = '进' THEN 1 END) AS BIGINT) AS in_count, \
     CAST(COUNT(CASE WHEN dc_val = '出' THEN 1 END) AS BIGINT) AS out_count, \
     CAST(COUNT(DISTINCT CASE WHEN dc_val = '进' THEN counterparty END) AS BIGINT) AS in_counterparty_count, \
     CAST(COUNT(DISTINCT CASE WHEN dc_val = '出' THEN counterparty END) AS BIGINT) AS out_counterparty_count, \
     CAST(COUNT(DISTINCT counterparty) AS BIGINT) AS all_counterparty_count"
}

fn rows_to_json(rows: Vec<StructureBucketRow>, request: &ChartStructureRequest) -> Vec<Value> {
    let mut rows = rows;
    rows.sort_by(|left, right| {
        let left_value = structure_item_value(left, request).abs();
        let right_value = structure_item_value(right, request).abs();
        right_value
            .partial_cmp(&left_value)
            .unwrap_or(std::cmp::Ordering::Equal)
            .then_with(|| left.label.cmp(&right.label))
    });
    rows.into_iter()
        .map(|row| structure_item_to_json(&row, request))
        .collect()
}

fn structure_item_to_json(row: &StructureBucketRow, request: &ChartStructureRequest) -> Value {
    let total_amount = row.in_amount + row.out_amount;
    let value = structure_item_value(row, request);
    json!({
        "label": row.label,
        "in_amount": round2(row.in_amount),
        "out_amount": round2(row.out_amount),
        "net_amount": round2(row.in_amount - row.out_amount),
        "total_amount": round2(total_amount),
        "value": round2(value),
        "count": row.in_count + row.out_count,
    })
}

fn structure_item_value(row: &StructureBucketRow, request: &ChartStructureRequest) -> f64 {
    metric_value_from_counters(
        request.metric_mode,
        request.direction_mode,
        row.in_amount,
        row.out_amount,
        row.in_count,
        row.out_count,
        row.in_counterparty_count,
        row.out_counterparty_count,
        row.all_counterparty_count,
    )
}

fn build_chart_structure_payload(
    request: &ChartStructureRequest,
    view_items: Vec<(&'static str, Vec<Value>)>,
) -> Value {
    let mut weak_views = Vec::new();
    for (view_key, items) in &view_items {
        if matches!(*view_key, "success" | "currency") && items.len() <= 1 {
            weak_views.push(*view_key);
        }
    }
    let views = view_items
        .into_iter()
        .map(|(view_key, items)| (view_key.to_string(), Value::Array(items)))
        .collect::<Map<_, _>>();
    json!({
        "views": views,
        "default_view": "direction",
        "weak_views": weak_views,
        "metric_mode": request.metric_mode,
        "direction_mode": request.direction_mode,
    })
}

fn direction_label_expr() -> String {
    "CASE WHEN dc_val = '进' THEN '流入' WHEN dc_val = '出' THEN '流出' ELSE '其他' END".to_string()
}

fn success_label_expr() -> String {
    "CASE \
       WHEN LOWER(TRIM(COALESCE(is_success, ''))) IN ('是','成功','true','1','y','yes','success','ok') THEN '成功' \
       WHEN LOWER(TRIM(COALESCE(is_success, ''))) IN ('否','失败','false','0','n','no','failed','fail','error') THEN '失败' \
       ELSE '未知' \
     END"
        .to_string()
}

fn fallback_label_expr(column: &str, empty_label: &str) -> String {
    format!(
        "COALESCE(NULLIF(TRIM({column}), ''), {})",
        crate::sql_literal(empty_label)
    )
}
