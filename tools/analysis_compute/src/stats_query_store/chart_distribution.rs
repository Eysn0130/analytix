use anyhow::{Context, Result};
use duckdb::Connection;
use serde_json::{json, Map, Value};

use super::args::QueryChartDistributionArgs;
use super::chart_common::{
    build_chart_context_where_sql, chart_dedupe_enabled, chart_detail_source_table_sql,
    counterparty_label_expr, metric_value_from_counters, normalize_direction_mode,
    normalize_metric_mode, normalize_selected_keys, normalize_selection_mode,
    require_chart_amount_query_ready, require_chart_scope, round2, ChartSourceRequest,
};
use super::session::VerifiedStatsQuerySession;

struct ChartDistributionRequest {
    source: ChartSourceRequest,
    metric_mode: &'static str,
    direction_mode: &'static str,
    selection_mode: String,
}

struct DistributionQuality {
    total_rows: i64,
    bank_non_empty: i64,
    location_non_empty: i64,
    branch_non_empty: i64,
}

struct DistributionItemRow {
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
struct DistributionViewRows {
    bank: Vec<DistributionItemRow>,
    location: Vec<DistributionItemRow>,
    branch: Vec<DistributionItemRow>,
}

struct DistributionData {
    quality: DistributionQuality,
    views: DistributionViewRows,
}

struct DistributionQueryRow {
    section: String,
    view_key: String,
    item: DistributionItemRow,
    total_rows: i64,
    bank_non_empty: i64,
    location_non_empty: i64,
    branch_non_empty: i64,
}

impl ChartDistributionRequest {
    fn from_args(args: &QueryChartDistributionArgs) -> Self {
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
        }
    }
}

pub(crate) fn query_chart_distribution(args: &QueryChartDistributionArgs) -> Result<Value> {
    let request = ChartDistributionRequest::from_args(args);
    require_chart_scope(&request.source.selected)?;

    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let result = query_chart_distribution_for_request(&mut session, &request)?;
    session.commit()?;
    Ok(result)
}

pub(crate) fn query_chart_distribution_with_session(
    session: &mut VerifiedStatsQuerySession<'_>,
    args: &QueryChartDistributionArgs,
) -> Result<Value> {
    let request = ChartDistributionRequest::from_args(args);
    require_chart_scope(&request.source.selected)?;
    query_chart_distribution_for_request(session, &request)
}

fn query_chart_distribution_for_request(
    session: &mut VerifiedStatsQuerySession<'_>,
    request: &ChartDistributionRequest,
) -> Result<Value> {
    let where_sql = build_distribution_where_sql(request);
    let dedupe_enabled = chart_dedupe_enabled(session.conn(), &request.source.selected)?;
    require_chart_amount_query_ready(session, &where_sql, dedupe_enabled)?;
    let data = query_distribution_data(session.conn(), &where_sql, dedupe_enabled)?;
    let views = distribution_views_to_json(data.views, request);
    Ok(build_chart_distribution_payload(
        request,
        data.quality,
        views,
    ))
}

fn build_distribution_where_sql(request: &ChartDistributionRequest) -> String {
    let mut where_sql = build_chart_context_where_sql(&request.source);
    if request.direction_mode == "in" {
        where_sql.push_str(" AND dc_val = '进'");
    } else if request.direction_mode == "out" {
        where_sql.push_str(" AND dc_val = '出'");
    }
    where_sql
}

fn query_distribution_data(
    conn: &Connection,
    where_sql: &str,
    dedupe_enabled: bool,
) -> Result<DistributionData> {
    let sql = distribution_data_sql(where_sql, dedupe_enabled);
    let mut stmt = conn.prepare(&sql)?;
    let mapped = stmt.query_map([], distribution_query_row_from_row)?;
    let mut quality = DistributionQuality {
        total_rows: 0,
        bank_non_empty: 0,
        location_non_empty: 0,
        branch_non_empty: 0,
    };
    let mut views = DistributionViewRows::default();
    for item in mapped {
        let row = item?;
        if row.section == "quality" {
            quality = DistributionQuality {
                total_rows: row.total_rows,
                bank_non_empty: row.bank_non_empty,
                location_non_empty: row.location_non_empty,
                branch_non_empty: row.branch_non_empty,
            };
            continue;
        }
        match row.view_key.as_str() {
            "bank" => views.bank.push(row.item),
            "location" => views.location.push(row.item),
            "branch" => views.branch.push(row.item),
            _ => {}
        }
    }
    Ok(DistributionData { quality, views })
}

fn distribution_views_to_json(
    rows: DistributionViewRows,
    request: &ChartDistributionRequest,
) -> Map<String, Value> {
    [
        ("bank", rows.bank),
        ("location", rows.location),
        ("branch", rows.branch),
    ]
    .into_iter()
    .map(|(view_key, rows)| {
        (
            view_key.to_string(),
            Value::Array(
                rows.into_iter()
                    .map(|item| item_to_json(&item, request))
                    .collect::<Vec<_>>(),
            ),
        )
    })
    .collect::<Map<String, Value>>()
}

fn distribution_data_sql(where_sql: &str, dedupe_enabled: bool) -> String {
    let cp_label_expr = counterparty_label_expr();
    let bank_label = fallback_label_expr("counterparty_bank", "未知银行");
    let location_label = fallback_label_expr("location", "未知地域");
    let branch_label = fallback_label_expr("branch_name", "未知网点");
    let aggregate_columns = distribution_aggregate_columns();
    let detail_source = chart_detail_source_table_sql(where_sql, dedupe_enabled);
    format!(
        "
        WITH filtered AS (
          SELECT
            dc_val,
            ABS(amount) AS amount_abs,
            {cp_label_expr} AS counterparty,
            {bank_label} AS bank_label,
            {location_label} AS location_label,
            {branch_label} AS branch_label,
            CASE WHEN NULLIF(TRIM(COALESCE(counterparty_bank, '')), '') IS NOT NULL THEN 1 ELSE 0 END AS bank_non_empty,
            CASE WHEN NULLIF(TRIM(COALESCE(location, '')), '') IS NOT NULL THEN 1 ELSE 0 END AS location_non_empty,
            CASE WHEN NULLIF(TRIM(COALESCE(branch_name, '')), '') IS NOT NULL THEN 1 ELSE 0 END AS branch_non_empty
          FROM {detail_source}
        ), quality AS (
          SELECT
            CAST(COUNT(1) AS BIGINT) AS total_rows,
            COALESCE(CAST(SUM(bank_non_empty) AS BIGINT), 0) AS bank_non_empty,
            COALESCE(CAST(SUM(location_non_empty) AS BIGINT), 0) AS location_non_empty,
            COALESCE(CAST(SUM(branch_non_empty) AS BIGINT), 0) AS branch_non_empty
          FROM filtered
        ), view_input AS (
          SELECT 'bank' AS view_key, bank_label AS label, dc_val, amount_abs, counterparty FROM filtered
          UNION ALL
          SELECT 'location' AS view_key, location_label AS label, dc_val, amount_abs, counterparty FROM filtered
          UNION ALL
          SELECT 'branch' AS view_key, branch_label AS label, dc_val, amount_abs, counterparty FROM filtered
        ), grouped AS (
          SELECT
            view_key,
            label,
            {aggregate_columns},
            CAST(SUM(CASE WHEN dc_val IN ('进','出') THEN amount_abs ELSE 0 END) AS DOUBLE) AS sort_amount
          FROM view_input
          GROUP BY view_key, label
        )
        SELECT
          section,
          view_key,
          label,
          in_amount,
          out_amount,
          in_count,
          out_count,
          in_counterparty_count,
          out_counterparty_count,
          all_counterparty_count,
          total_rows,
          bank_non_empty,
          location_non_empty,
          branch_non_empty
        FROM (
          SELECT
            0 AS section_order,
            0 AS view_order,
            0.0 AS sort_amount,
            'quality' AS section,
            '' AS view_key,
            '' AS label,
            0.0 AS in_amount,
            0.0 AS out_amount,
            0 AS in_count,
            0 AS out_count,
            0 AS in_counterparty_count,
            0 AS out_counterparty_count,
            0 AS all_counterparty_count,
            total_rows,
            bank_non_empty,
            location_non_empty,
            branch_non_empty
          FROM quality
          UNION ALL
          SELECT
            1 AS section_order,
            CASE
              WHEN view_key = 'bank' THEN 0
              WHEN view_key = 'location' THEN 1
              ELSE 2
            END AS view_order,
            sort_amount,
            'view' AS section,
            view_key,
            label,
            in_amount,
            out_amount,
            in_count,
            out_count,
            in_counterparty_count,
            out_counterparty_count,
            all_counterparty_count,
            0 AS total_rows,
            0 AS bank_non_empty,
            0 AS location_non_empty,
            0 AS branch_non_empty
          FROM grouped
        )
        ORDER BY section_order ASC, view_order ASC, sort_amount DESC, label ASC
        ",
    )
}

fn distribution_query_row_from_row(row: &duckdb::Row<'_>) -> duckdb::Result<DistributionQueryRow> {
    Ok(DistributionQueryRow {
        section: row.get::<_, Option<String>>(0)?.unwrap_or_default(),
        view_key: row.get::<_, Option<String>>(1)?.unwrap_or_default(),
        item: DistributionItemRow {
            label: row.get::<_, Option<String>>(2)?.unwrap_or_default(),
            in_amount: row.get::<_, f64>(3)?,
            out_amount: row.get::<_, f64>(4)?,
            in_count: row.get::<_, i64>(5)?,
            out_count: row.get::<_, i64>(6)?,
            in_counterparty_count: row.get::<_, i64>(7)?,
            out_counterparty_count: row.get::<_, i64>(8)?,
            all_counterparty_count: row.get::<_, i64>(9)?,
        },
        total_rows: row.get::<_, i64>(10)?,
        bank_non_empty: row.get::<_, i64>(11)?,
        location_non_empty: row.get::<_, i64>(12)?,
        branch_non_empty: row.get::<_, i64>(13)?,
    })
}

fn distribution_aggregate_columns() -> &'static str {
    "CAST(SUM(CASE WHEN dc_val = '进' THEN amount_abs ELSE 0 END) AS DOUBLE) AS in_amount, \
     CAST(SUM(CASE WHEN dc_val = '出' THEN amount_abs ELSE 0 END) AS DOUBLE) AS out_amount, \
     CAST(COUNT(CASE WHEN dc_val = '进' THEN 1 END) AS BIGINT) AS in_count, \
     CAST(COUNT(CASE WHEN dc_val = '出' THEN 1 END) AS BIGINT) AS out_count, \
     CAST(COUNT(DISTINCT CASE WHEN dc_val = '进' THEN counterparty END) AS BIGINT) AS in_counterparty_count, \
     CAST(COUNT(DISTINCT CASE WHEN dc_val = '出' THEN counterparty END) AS BIGINT) AS out_counterparty_count, \
     CAST(COUNT(DISTINCT counterparty) AS BIGINT) AS all_counterparty_count"
}

fn build_chart_distribution_payload(
    request: &ChartDistributionRequest,
    quality: DistributionQuality,
    views: Map<String, Value>,
) -> Value {
    let default_view = if request.selection_mode != "single-card"
        && quality.location_non_empty > quality.bank_non_empty
    {
        "location"
    } else {
        "bank"
    };
    json!({
        "selection_mode": request.selection_mode,
        "default_view": default_view,
        "views": views,
        "quality": {
            "bank_non_empty_rate": percent(quality.bank_non_empty, quality.total_rows),
            "location_non_empty_rate": percent(quality.location_non_empty, quality.total_rows),
            "branch_non_empty_rate": percent(quality.branch_non_empty, quality.total_rows),
        },
    })
}

fn item_to_json(item: &DistributionItemRow, request: &ChartDistributionRequest) -> Value {
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
        "in_amount": round2(item.in_amount),
        "out_amount": round2(item.out_amount),
        "total_amount": round2(total_amount),
        "net_amount": round2(item.in_amount - item.out_amount),
        "in_count": item.in_count,
        "out_count": item.out_count,
        "total_count": total_count,
        "value": round2(value),
        "count": total_count,
    })
}

fn fallback_label_expr(column: &str, empty_label: &str) -> String {
    format!(
        "COALESCE(NULLIF(TRIM({column}), ''), {})",
        crate::sql_literal(empty_label)
    )
}

fn percent(numerator: i64, denominator: i64) -> f64 {
    if denominator <= 0 {
        0.0
    } else {
        round2((numerator as f64 / denominator as f64) * 100.0)
    }
}
