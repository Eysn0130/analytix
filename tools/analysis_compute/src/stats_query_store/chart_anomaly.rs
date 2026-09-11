use anyhow::{bail, Result};
use duckdb::Connection;
use serde_json::{json, Value};

use super::args::QueryChartDashboardArgs;
use super::chart_common::{
    build_chart_detail_where_sql, chart_dedupe_enabled, chart_detail_source_table_sql,
    counterparty_label_expr, normalize_direction_mode, normalize_selected_keys,
    require_chart_amount_query_ready, require_chart_scope, round2, ChartSourceRequest,
};
use super::session::VerifiedStatsQuerySession;

const REMARK_KEYWORD_TOP_LIMIT: usize = 160;

struct ChartAnomalyRequest {
    source: ChartSourceRequest,
    direction_mode: &'static str,
    large_txn_threshold: f64,
}

struct AnomalyAggregates {
    amount_buckets: Vec<Value>,
    ip_top: Vec<Value>,
    mac_top: Vec<Value>,
    failure_reason_top: Vec<Value>,
    txn_type_top: Vec<Value>,
    summary_keyword_top: Vec<Value>,
    remark_keyword_top: Vec<Value>,
    large_txns: Vec<Value>,
    large_txn_count: i64,
    ip_non_empty_rate: f64,
}

impl ChartAnomalyRequest {
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
            direction_mode: normalize_direction_mode(&args.direction_mode),
            large_txn_threshold: args.large_txn_threshold,
        }
    }
}

pub(crate) fn query_chart_anomaly_with_session(
    session: &mut VerifiedStatsQuerySession<'_>,
    args: &QueryChartDashboardArgs,
) -> Result<Value> {
    let request = ChartAnomalyRequest::from_dashboard_args(args);
    require_chart_scope(&request.source.selected)?;
    if !request.large_txn_threshold.is_finite() || request.large_txn_threshold < 0.0 {
        bail!("--large-txn-threshold must be finite and non-negative");
    }
    let where_sql = build_chart_detail_where_sql(&request.source, Some(request.direction_mode));
    let dedupe_enabled = chart_dedupe_enabled(session.conn(), &request.source.selected)?;
    require_chart_amount_query_ready(session, &where_sql, dedupe_enabled)?;
    let aggregates = query_anomaly_aggregates(
        session.conn(),
        &where_sql,
        request.large_txn_threshold,
        dedupe_enabled,
    )?;
    Ok(build_chart_anomaly_payload(&request, aggregates))
}

fn query_anomaly_aggregates(
    conn: &Connection,
    where_sql: &str,
    large_txn_threshold: f64,
    dedupe_enabled: bool,
) -> Result<AnomalyAggregates> {
    let threshold_sql = numeric_sql(large_txn_threshold);
    query_anomaly_summary(conn, where_sql, &threshold_sql, dedupe_enabled)
}

fn query_anomaly_summary(
    conn: &Connection,
    where_sql: &str,
    threshold_sql: &str,
    dedupe_enabled: bool,
) -> Result<AnomalyAggregates> {
    let cp_label_expr = counterparty_label_expr();
    let detail_source = chart_detail_source_table_sql(where_sql, dedupe_enabled);
    let sql = format!(
        "
        WITH filtered AS (
          SELECT
            id,
            ABS(amount) AS amount_abs,
            COALESCE(NULLIF(TRIM(txn_id), ''), CAST(id AS VARCHAR)) AS stable_txn_id,
            COALESCE(NULLIF(TRIM(txn_time), ''), CAST({txn_ts} AS VARCHAR), '') AS stable_txn_time,
            CASE WHEN dc_val = '进' THEN 'in' WHEN dc_val = '出' THEN 'out' ELSE 'other' END AS direction,
            {cp_label_expr} AS counterparty,
            COALESCE(NULLIF(TRIM(counterparty_bank), ''), '') AS counterparty_bank,
            COALESCE(summary, '') AS summary,
            COALESCE(NULLIF(TRIM(ip_addr), ''), '未知IP') AS ip_label,
            COALESCE(NULLIF(TRIM(mac_addr), ''), '未知MAC') AS mac_label,
            COALESCE(NULLIF(TRIM(query_feedback_reason), ''), '无反馈原因') AS failure_reason_label,
            COALESCE(NULLIF(TRIM(txn_type), ''), '未知类型') AS txn_type_label,
            CASE WHEN NULLIF(TRIM(ip_addr), '') IS NOT NULL THEN 1 ELSE 0 END AS ip_non_empty,
            ROW_NUMBER() OVER (
              ORDER BY ({txn_ts} IS NULL) ASC,
                       {txn_ts} ASC,
                       COALESCE(NULLIF(TRIM(txn_id), ''), CAST(id AS VARCHAR)) ASC
            ) - 1 AS row_order,
            COALESCE(NULLIF(TRIM(account_open_name), ''), '') AS account_name
          FROM {detail_source}
        ), amount_bucketed AS (
          SELECT
            CASE
              WHEN amount_abs < 1000 THEN 0
              WHEN amount_abs < 10000 THEN 1
              WHEN amount_abs < 50000 THEN 2
              WHEN amount_abs < 100000 THEN 3
              ELSE 4
            END AS bucket_order,
            CASE
              WHEN amount_abs < 1000 THEN '1千以下'
              WHEN amount_abs < 10000 THEN '1千-1万'
              WHEN amount_abs < 50000 THEN '1万-5万'
              WHEN amount_abs < 100000 THEN '5万-10万'
              ELSE '10万以上'
            END AS label,
            amount_abs
          FROM filtered
        ), amount_rows AS (
          SELECT
            'amount_buckets' AS section,
            label,
            ROUND(SUM(amount_abs), 2) AS amount_sum,
            CAST(COUNT(1) AS BIGINT) AS item_count,
            bucket_order
          FROM amount_bucketed
          GROUP BY bucket_order, label
        ), group_input AS (
          SELECT 'ip_top' AS section, ip_label AS label, amount_abs FROM filtered
          UNION ALL
          SELECT 'mac_top' AS section, mac_label AS label, amount_abs FROM filtered
          UNION ALL
          SELECT 'failure_reason_top' AS section, failure_reason_label AS label, amount_abs FROM filtered
          UNION ALL
          SELECT 'txn_type_top' AS section, txn_type_label AS label, amount_abs FROM filtered
        ), group_rows AS (
          SELECT
            section,
            label,
            ROUND(SUM(amount_abs), 2) AS amount_sum,
            CAST(COUNT(1) AS BIGINT) AS item_count
          FROM group_input
          GROUP BY section, label
        ), ranked_group_rows AS (
          SELECT
            section,
            label,
            amount_sum,
            item_count,
            ROW_NUMBER() OVER (
              PARTITION BY section
              ORDER BY amount_sum DESC, item_count DESC, label ASC
            ) AS rn
          FROM group_rows
        ), stats AS (
          SELECT
            CAST(COUNT(1) AS BIGINT) AS total_count,
            COALESCE(CAST(SUM(ip_non_empty) AS BIGINT), 0) AS ip_non_empty_count,
            COALESCE(CAST(SUM(CASE WHEN amount_abs >= {threshold_sql} THEN 1 ELSE 0 END) AS BIGINT), 0) AS large_txn_count
          FROM filtered
        ), self_names AS (
          SELECT DISTINCT account_name
          FROM filtered
          WHERE account_name <> ''
        ), keyword_counts AS (
          SELECT
            kw.kind AS kind,
            kw.token AS token,
            CAST(COUNT(1) AS BIGINT) AS token_count,
            MIN(CAST(filtered.row_order AS BIGINT) * 1000000 + kw.token_order) AS first_order
          FROM filtered
          JOIN {keyword_table} kw ON kw.txn_row_id = filtered.id
          WHERE kw.kind IN ('summary', 'remark')
            AND NULLIF(TRIM(kw.token), '') IS NOT NULL
            AND (
              kw.kind <> 'remark'
              OR NOT EXISTS (
                SELECT 1
                FROM self_names
                WHERE self_names.account_name = kw.token
              )
            )
          GROUP BY kw.kind, kw.token
        ), ranked_keywords AS (
          SELECT
            kind,
            token,
            token_count,
            ROW_NUMBER() OVER (
              PARTITION BY kind
              ORDER BY token_count DESC, first_order ASC, token ASC
            ) AS rn
          FROM keyword_counts
        ), large_txn_rows AS (
          SELECT
            stable_txn_id AS txn_id,
            stable_txn_time AS txn_time,
            direction,
            ROUND(amount_abs, 2) AS amount_sum,
            counterparty,
            counterparty_bank,
            summary,
            amount_abs >= {threshold_sql} AS is_large,
            ROW_NUMBER() OVER (
              ORDER BY ROUND(amount_abs, 2) DESC,
                       stable_txn_time ASC,
                       stable_txn_id ASC
            ) AS rn
          FROM filtered
        )
        SELECT
          section,
          label,
          amount_sum,
          item_count,
          txn_id,
          txn_time,
          direction,
          counterparty,
          counterparty_bank,
          summary,
          is_large
        FROM (
          SELECT
            0 AS section_order,
            bucket_order AS item_order,
            section,
            label,
            amount_sum,
            item_count,
            CAST(NULL AS VARCHAR) AS txn_id,
            CAST(NULL AS VARCHAR) AS txn_time,
            CAST(NULL AS VARCHAR) AS direction,
            CAST(NULL AS VARCHAR) AS counterparty,
            CAST(NULL AS VARCHAR) AS counterparty_bank,
            CAST(NULL AS VARCHAR) AS summary,
            CAST(FALSE AS BOOLEAN) AS is_large
          FROM amount_rows
          UNION ALL
          SELECT
            CASE
              WHEN section = 'ip_top' THEN 1
              WHEN section = 'mac_top' THEN 2
              WHEN section = 'failure_reason_top' THEN 3
              ELSE 4
            END AS section_order,
            rn AS item_order,
            section,
            label,
            amount_sum,
            item_count,
            CAST(NULL AS VARCHAR) AS txn_id,
            CAST(NULL AS VARCHAR) AS txn_time,
            CAST(NULL AS VARCHAR) AS direction,
            CAST(NULL AS VARCHAR) AS counterparty,
            CAST(NULL AS VARCHAR) AS counterparty_bank,
            CAST(NULL AS VARCHAR) AS summary,
            CAST(FALSE AS BOOLEAN) AS is_large
          FROM ranked_group_rows
          UNION ALL
          SELECT
            5 AS section_order,
            0 AS item_order,
            'large_txn_count' AS section,
            '' AS label,
            0.0 AS amount_sum,
            large_txn_count AS item_count,
            CAST(NULL AS VARCHAR) AS txn_id,
            CAST(NULL AS VARCHAR) AS txn_time,
            CAST(NULL AS VARCHAR) AS direction,
            CAST(NULL AS VARCHAR) AS counterparty,
            CAST(NULL AS VARCHAR) AS counterparty_bank,
            CAST(NULL AS VARCHAR) AS summary,
            CAST(FALSE AS BOOLEAN) AS is_large
          FROM stats
          UNION ALL
          SELECT
            6 AS section_order,
            0 AS item_order,
            'ip_non_empty_rate' AS section,
            '' AS label,
            CASE
              WHEN total_count > 0 THEN ROUND((CAST(ip_non_empty_count AS DOUBLE) / CAST(total_count AS DOUBLE)) * 100.0, 2)
              ELSE 0.0
            END AS amount_sum,
            0 AS item_count,
            CAST(NULL AS VARCHAR) AS txn_id,
            CAST(NULL AS VARCHAR) AS txn_time,
            CAST(NULL AS VARCHAR) AS direction,
            CAST(NULL AS VARCHAR) AS counterparty,
            CAST(NULL AS VARCHAR) AS counterparty_bank,
            CAST(NULL AS VARCHAR) AS summary,
            CAST(FALSE AS BOOLEAN) AS is_large
          FROM stats
          UNION ALL
          SELECT
            CASE WHEN kind = 'summary' THEN 7 ELSE 8 END AS section_order,
            rn AS item_order,
            CASE WHEN kind = 'summary' THEN 'summary_keyword_top' ELSE 'remark_keyword_top' END AS section,
            token AS label,
            0.0 AS amount_sum,
            token_count AS item_count,
            CAST(NULL AS VARCHAR) AS txn_id,
            CAST(NULL AS VARCHAR) AS txn_time,
            CAST(NULL AS VARCHAR) AS direction,
            CAST(NULL AS VARCHAR) AS counterparty,
            CAST(NULL AS VARCHAR) AS counterparty_bank,
            CAST(NULL AS VARCHAR) AS summary,
            CAST(FALSE AS BOOLEAN) AS is_large
          FROM ranked_keywords
          WHERE (kind = 'summary' AND rn <= 10)
             OR (kind = 'remark' AND rn <= {remark_limit})
          UNION ALL
          SELECT
            9 AS section_order,
            rn AS item_order,
            'large_txns' AS section,
            '' AS label,
            amount_sum,
            0 AS item_count,
            txn_id,
            txn_time,
            direction,
            counterparty,
            counterparty_bank,
            summary,
            is_large
          FROM large_txn_rows
          WHERE rn <= 10
        )
        ORDER BY section_order ASC, item_order ASC
        ",
        keyword_table = crate::KEYWORD_TABLE,
        remark_limit = REMARK_KEYWORD_TOP_LIMIT,
        cp_label_expr = cp_label_expr,
        txn_ts = anomaly_ts_expr(),
    );
    let mut stmt = conn.prepare(&sql)?;
    let mapped = stmt.query_map([], |row| {
        Ok((
            row.get::<_, Option<String>>(0)?.unwrap_or_default(),
            row.get::<_, Option<String>>(1)?.unwrap_or_default(),
            row.get::<_, f64>(2)?,
            row.get::<_, i64>(3)?,
            row.get::<_, Option<String>>(4)?.unwrap_or_default(),
            row.get::<_, Option<String>>(5)?.unwrap_or_default(),
            row.get::<_, Option<String>>(6)?.unwrap_or_default(),
            row.get::<_, Option<String>>(7)?.unwrap_or_default(),
            row.get::<_, Option<String>>(8)?.unwrap_or_default(),
            row.get::<_, Option<String>>(9)?.unwrap_or_default(),
            row.get::<_, Option<bool>>(10)?.unwrap_or(false),
        ))
    })?;
    let mut aggregates = AnomalyAggregates {
        amount_buckets: Vec::new(),
        ip_top: Vec::new(),
        mac_top: Vec::new(),
        failure_reason_top: Vec::new(),
        txn_type_top: Vec::new(),
        summary_keyword_top: Vec::new(),
        remark_keyword_top: Vec::new(),
        large_txns: Vec::new(),
        large_txn_count: 0,
        ip_non_empty_rate: 0.0,
    };
    for item in mapped {
        let (
            section,
            label,
            amount,
            count,
            txn_id,
            txn_time,
            direction,
            counterparty,
            counterparty_bank,
            summary,
            is_large,
        ) = item?;
        if section == "summary_keyword_top" {
            aggregates
                .summary_keyword_top
                .push(json!({"label": label, "count": count}));
            continue;
        }
        if section == "remark_keyword_top" {
            aggregates
                .remark_keyword_top
                .push(json!({"label": label, "count": count}));
            continue;
        }
        if section == "large_txns" {
            aggregates.large_txns.push(json!({
                "txn_id": txn_id,
                "txn_time": txn_time,
                "direction": direction,
                "amount": round2(amount),
                "counterparty": counterparty,
                "counterparty_bank": counterparty_bank,
                "summary": summary,
                "is_large": is_large,
            }));
            continue;
        }
        let top_item = json!({
            "label": label,
            "amount": round2(amount),
            "count": count,
        });
        match section.as_str() {
            "amount_buckets" => aggregates.amount_buckets.push(top_item),
            "ip_top" => aggregates.ip_top.push(top_item),
            "mac_top" => aggregates.mac_top.push(top_item),
            "failure_reason_top" => aggregates.failure_reason_top.push(top_item),
            "txn_type_top" => aggregates.txn_type_top.push(top_item),
            "large_txn_count" => aggregates.large_txn_count = count,
            "ip_non_empty_rate" => aggregates.ip_non_empty_rate = round2(amount),
            _ => {}
        }
    }
    Ok(aggregates)
}

fn build_chart_anomaly_payload(
    request: &ChartAnomalyRequest,
    aggregates: AnomalyAggregates,
) -> Value {
    json!({
        "amount_buckets": aggregates.amount_buckets,
        "large_txn_threshold": round2(request.large_txn_threshold),
        "large_txn_count": aggregates.large_txn_count,
        "ip_top": aggregates.ip_top,
        "mac_top": aggregates.mac_top,
        "failure_reason_top": aggregates.failure_reason_top,
        "txn_type_top": aggregates.txn_type_top,
        "summary_keyword_top": aggregates.summary_keyword_top,
        "remark_keyword_top": aggregates.remark_keyword_top,
        "large_txns": aggregates.large_txns,
        "quality": {"ip_non_empty_rate": aggregates.ip_non_empty_rate},
    })
}

fn numeric_sql(value: f64) -> String {
    value.to_string()
}

fn anomaly_ts_expr() -> &'static str {
    "COALESCE(txn_ts, TRY_CAST(txn_time AS TIMESTAMP))"
}
