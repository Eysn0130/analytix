use super::amount_coverage::require_complete_detail_amount_coverage;
use super::args::QueryChartFilterArg;
use super::dedupe_sql::{deduped_source_ctes, detail_stats_dedupe_key_expr};
use super::values::sql_literal_list;
use crate::KEYWORD_TABLE;
use anyhow::{bail, Result};

use super::session::VerifiedStatsQuerySession;

pub(super) struct ChartSourceRequest {
    pub(super) selected: Vec<String>,
    pub(super) date_start: String,
    pub(super) date_end: String,
    pub(super) success_filter: String,
    pub(super) cash_filter: String,
    pub(super) chart_filters: Vec<QueryChartFilterArg>,
}

pub(super) const STATS_QUERY_SCOPE_REQUIRED: &str = "stats_query_scope_required";
pub(super) const STATS_QUERY_SCOPE_EMPTY: &str = "stats_query_scope_empty";

pub(super) fn require_chart_scope(selected: &[String]) -> Result<()> {
    if selected.is_empty() {
        bail!(STATS_QUERY_SCOPE_REQUIRED);
    }
    Ok(())
}

pub(super) fn require_chart_amount_query_ready(
    session: &mut VerifiedStatsQuerySession<'_>,
    where_sql: &str,
    dedupe_enabled: bool,
) -> Result<()> {
    require_complete_detail_amount_coverage(session.conn())?;
    let source = chart_detail_source_table_sql(where_sql, dedupe_enabled);
    session.require_nonempty_query("chart_detail_scope", &format!("SELECT * FROM {source}"))?;
    Ok(())
}

pub(super) fn normalize_selected_keys(selected_keys: &[String]) -> Vec<String> {
    selected_keys
        .iter()
        .map(|item| item.trim().to_string())
        .filter(|item| !item.is_empty())
        .collect::<Vec<_>>()
}

pub(super) fn chart_dedupe_enabled(
    conn: &duckdb::Connection,
    selected: &[String],
) -> anyhow::Result<bool> {
    Ok(selected.len() > 1 && crate::table_exists(conn, crate::ACCOUNT_DIM_TABLE)?)
}

pub(super) fn chart_detail_source_table_sql(where_sql: &str, dedupe_enabled: bool) -> String {
    let source_sql = format!(
        "SELECT d.*, {} AS stats_dedupe_key FROM {} d WHERE {where_sql}",
        detail_stats_dedupe_key_expr("d"),
        crate::DETAIL_TABLE,
    );
    format!(
        "(WITH {} SELECT * FROM filtered)",
        deduped_source_ctes(&source_sql, dedupe_enabled)
    )
}

pub(super) fn normalize_selection_mode(selected: &[String], selection_mode: &str) -> String {
    match selection_mode.trim() {
        "single-card" => "single-card".to_string(),
        "multi-card" => "multi-card".to_string(),
        _ => {
            if selected.len() == 1 {
                "single-card".to_string()
            } else {
                "multi-card".to_string()
            }
        }
    }
}

pub(super) fn normalize_metric_mode(value: &str) -> &'static str {
    match value.trim() {
        "count" => "count",
        "counterparty" => "counterparty",
        _ => "amount",
    }
}

pub(super) fn normalize_direction_mode(value: &str) -> &'static str {
    match value.trim() {
        "in" => "in",
        "out" => "out",
        "net" => "net",
        _ => "all",
    }
}

pub(super) fn normalize_granularity(value: &str) -> &'static str {
    match value.trim() {
        "hour" => "hour",
        "week" => "week",
        "month" => "month",
        _ => "day",
    }
}

pub(super) fn build_chart_detail_where_sql(
    request: &ChartSourceRequest,
    direction_mode: Option<&str>,
) -> String {
    let mut where_parts = chart_source_where_parts(request);
    where_parts.push("dc_val IN ('进','出')".to_string());
    if let Some(direction_mode) = direction_mode {
        let direction = normalize_direction_mode(direction_mode);
        if direction == "in" {
            where_parts.push("dc_val = '进'".to_string());
        } else if direction == "out" {
            where_parts.push("dc_val = '出'".to_string());
        }
    }
    where_parts.join(" AND ")
}

pub(super) fn build_chart_context_where_sql(request: &ChartSourceRequest) -> String {
    chart_source_where_parts(request).join(" AND ")
}

pub(super) fn build_chart_detail_rows_where_sql(
    request: &ChartSourceRequest,
    direction_mode: &str,
) -> String {
    let mut where_parts = chart_source_where_parts(request);
    let direction = normalize_direction_mode(direction_mode);
    if direction == "in" {
        where_parts.push("dc_val = '进'".to_string());
    } else if direction == "out" {
        where_parts.push("dc_val = '出'".to_string());
    }
    where_parts.join(" AND ")
}

pub(super) fn chart_filters_supported_by_native_dashboard(filters: &[QueryChartFilterArg]) -> bool {
    filters
        .iter()
        .all(|filter| chart_filter_where_sql(filter).is_some())
}

fn chart_source_where_parts(request: &ChartSourceRequest) -> Vec<String> {
    let mut where_parts = Vec::new();
    where_parts.push(format!(
        "acct_key IN ({})",
        sql_literal_list(&request.selected)
    ));
    if !request.date_start.is_empty() {
        where_parts.push(format!(
            "txn_day >= CAST({} AS DATE)",
            crate::sql_literal(&request.date_start)
        ));
    }
    if !request.date_end.is_empty() {
        where_parts.push(format!(
            "txn_day <= CAST({} AS DATE)",
            crate::sql_literal(&request.date_end)
        ));
    }
    if request.success_filter == "success" {
        where_parts.push(normalized_success_sql("is_success", "success"));
    } else if request.success_filter == "failed" {
        where_parts.push(normalized_success_sql("is_success", "failed"));
    }
    if request.cash_filter == "cash" {
        where_parts.push(normalized_cash_sql("cash_flag", "cash"));
    } else if request.cash_filter == "non-cash" {
        where_parts.push(normalized_cash_sql("cash_flag", "non-cash"));
    }
    for filter in &request.chart_filters {
        where_parts.push(chart_filter_where_sql(filter).unwrap_or_else(|| "1 = 0".to_string()));
    }
    where_parts
}

fn chart_filter_where_sql(filter: &QueryChartFilterArg) -> Option<String> {
    let dimension = filter.dimension.trim();
    let value = filter.value.trim();
    match dimension {
        "" => Some("1 = 1".to_string()),
        "direction" => Some(direction_filter_sql(value)),
        "counterparty_name" => Some(format!(
            "{} = {}",
            counterparty_label_expr(),
            crate::sql_literal(if value.is_empty() {
                "未知对手"
            } else {
                value
            })
        )),
        "counterparty_account" => Some(label_filter_sql("counterparty_acct", "未知账号", value)),
        "counterparty_bank" => Some(label_filter_sql("counterparty_bank", "未知银行", value)),
        "location" => Some(label_filter_sql("location", "未知地域", value)),
        "branch_name" => Some(label_filter_sql("branch_name", "未知网点", value)),
        "success" => Some(success_filter_sql(value)),
        "cash" => Some(cash_filter_sql(value)),
        "currency" => Some(label_filter_sql("currency", "未知币种", value)),
        "txn_type" => Some(label_filter_sql("txn_type", "未知类型", value)),
        "query_feedback_reason" => Some(label_filter_sql(
            "query_feedback_reason",
            "无反馈原因",
            value,
        )),
        "summary_keyword" => Some(keyword_index_filter_sql("summary", value)),
        "remark_keyword" => Some(keyword_index_filter_sql("remark", value)),
        "ip_addr" => Some(label_filter_sql("ip_addr", "未知IP", value)),
        "mac_addr" => Some(label_filter_sql("mac_addr", "未知MAC", value)),
        "amount_bucket" => Some(amount_bucket_filter_sql(value)),
        "hour" => Some(hour_filter_sql(value)),
        "weekday" => Some(weekday_filter_sql(value)),
        "heatmap_cell" => Some(heatmap_cell_filter_sql(filter)),
        "time_bucket" => Some(time_bucket_filter_sql(filter)),
        "flow_counterparty" => Some(flow_counterparty_filter_sql(filter)),
        "txn_id" => Some(format!(
            "COALESCE(NULLIF(TRIM(COALESCE(txn_id, '')), ''), CAST(id AS VARCHAR)) = {}",
            crate::sql_literal(value)
        )),
        _ => None,
    }
}

fn direction_filter_sql(value: &str) -> String {
    match value {
        "in" | "进" | "流入" | "进账" => "dc_val = '进'".to_string(),
        "out" | "出" | "流出" | "出账" => "dc_val = '出'".to_string(),
        "other" | "其他" => "(dc_val IS NULL OR dc_val NOT IN ('进','出'))".to_string(),
        _ => "1 = 0".to_string(),
    }
}

fn label_filter_sql(column: &str, empty_label: &str, value: &str) -> String {
    let expected = if value.is_empty() { empty_label } else { value };
    format!(
        "{} = {}",
        fallback_label_expr(column, empty_label),
        crate::sql_literal(expected)
    )
}

fn fallback_label_expr(column: &str, empty_label: &str) -> String {
    format!(
        "COALESCE(NULLIF(TRIM(COALESCE({column}, '')), ''), {})",
        crate::sql_literal(empty_label)
    )
}

fn success_filter_sql(value: &str) -> String {
    match value {
        "success" | "成功" | "是" => normalized_success_sql("is_success", "success"),
        "failed" | "失败" | "否" => normalized_success_sql("is_success", "failed"),
        "unknown" | "未知" => format!(
            "NOT ({}) AND NOT ({})",
            normalized_success_sql("is_success", "success"),
            normalized_success_sql("is_success", "failed")
        ),
        _ => "1 = 0".to_string(),
    }
}

fn cash_filter_sql(value: &str) -> String {
    match value {
        "cash" | "现金" | "是" => normalized_cash_sql("cash_flag", "cash"),
        "non-cash" | "非现金" | "转账" | "否" => normalized_cash_sql("cash_flag", "non-cash"),
        "unknown" | "未知" => format!(
            "NOT ({}) AND NOT ({})",
            normalized_cash_sql("cash_flag", "cash"),
            normalized_cash_sql("cash_flag", "non-cash")
        ),
        _ => "1 = 0".to_string(),
    }
}

fn amount_bucket_filter_sql(value: &str) -> String {
    let amount = "ABS(amount)";
    let known = "amount IS NOT NULL AND isfinite(amount)";
    match value {
        "1千以下" => format!("{known} AND {amount} < 1000"),
        "1千-1万" => format!("{known} AND {amount} >= 1000 AND {amount} < 10000"),
        "1万-5万" => format!("{known} AND {amount} >= 10000 AND {amount} < 50000"),
        "5万-10万" => format!("{known} AND {amount} >= 50000 AND {amount} < 100000"),
        "10万以上" => format!("{known} AND {amount} >= 100000"),
        _ => "1 = 0".to_string(),
    }
}

fn keyword_index_filter_sql(kind: &str, value: &str) -> String {
    if value.is_empty() {
        return "1 = 0".to_string();
    }
    format!(
        "EXISTS (SELECT 1 FROM {KEYWORD_TABLE} kw \
         WHERE kw.txn_row_id = id AND kw.kind = {} AND kw.token = {})",
        crate::sql_literal(kind),
        crate::sql_literal(value)
    )
}

fn hour_filter_sql(value: &str) -> String {
    match value.parse::<i64>() {
        Ok(hour) if (0..=23).contains(&hour) => format!(
            "CAST(STRFTIME({}, '%H') AS BIGINT) = {hour}",
            chart_ts_expr()
        ),
        _ => "1 = 0".to_string(),
    }
}

fn weekday_filter_sql(value: &str) -> String {
    match value.parse::<i64>() {
        Ok(weekday) if (0..=6).contains(&weekday) => format!(
            "((CAST(STRFTIME({}, '%w') AS BIGINT) + 6) % 7) = {weekday}",
            chart_ts_expr()
        ),
        _ => "1 = 0".to_string(),
    }
}

fn heatmap_cell_filter_sql(filter: &QueryChartFilterArg) -> String {
    let weekday = filter_payload_i64(filter, "weekday");
    let hour = filter_payload_i64(filter, "hour");
    match (weekday, hour) {
        (Some(weekday), Some(hour)) if (0..=6).contains(&weekday) && (0..=23).contains(&hour) => {
            format!(
                "{} AND {}",
                weekday_filter_sql(&weekday.to_string()),
                hour_filter_sql(&hour.to_string())
            )
        }
        _ => "1 = 0".to_string(),
    }
}

fn time_bucket_filter_sql(filter: &QueryChartFilterArg) -> String {
    let granularity = normalize_granularity(&filter_payload_text(filter, "granularity"));
    let bucket = {
        let payload_bucket = filter_payload_text(filter, "bucket");
        if payload_bucket.is_empty() {
            filter.value.trim().to_string()
        } else {
            payload_bucket
        }
    };
    let bucket_sql = time_bucket_value_filter_sql(granularity, &bucket);
    let range_start = filter_payload_text(filter, "range_start");
    let range_end = filter_payload_text(filter, "range_end");
    if range_start.is_empty() && range_end.is_empty() {
        return bucket_sql;
    }

    let ts = chart_ts_expr();
    let mut range_parts = Vec::new();
    if !range_start.is_empty() {
        range_parts.push(format!(
            "{ts} >= CAST({} AS TIMESTAMP)",
            crate::sql_literal(&range_start)
        ));
    }
    if !range_end.is_empty() {
        range_parts.push(format!(
            "{ts} <= CAST({} AS TIMESTAMP)",
            crate::sql_literal(&range_end)
        ));
    }
    let range_sql = format!("{ts} IS NOT NULL AND {}", range_parts.join(" AND "));
    if bucket_sql == "1 = 0" {
        return format!("({range_sql})");
    }
    format!("(({range_sql}) OR ({ts} IS NULL AND ({bucket_sql})))")
}

fn time_bucket_value_filter_sql(granularity: &str, bucket: &str) -> String {
    if bucket.trim().is_empty() {
        return "1 = 0".to_string();
    }
    let literal = crate::sql_literal(bucket.trim());
    format!(
        "({} = {literal} OR CAST(txn_day AS VARCHAR) = {literal})",
        trend_bucket_key_expr(granularity)
    )
}

fn trend_bucket_key_expr(granularity: &str) -> String {
    let ts = chart_ts_expr();
    match granularity {
        "hour" => {
            let expr = format!("STRFTIME({ts}, '%Y-%m-%d %H:00')");
            format!("CASE WHEN {ts} IS NULL THEN '未知' ELSE {expr} END")
        }
        "week" => {
            let expr = format!("CAST(CAST(DATE_TRUNC('week', {ts}) AS DATE) AS VARCHAR)");
            format!("CASE WHEN {ts} IS NULL THEN '未知' ELSE {expr} END")
        }
        "month" => {
            let expr = format!("STRFTIME({ts}, '%Y-%m')");
            format!("CASE WHEN {ts} IS NULL THEN '未知' ELSE {expr} END")
        }
        _ => {
            let expr = format!("CAST(CAST({ts} AS DATE) AS VARCHAR)");
            format!("CASE WHEN {ts} IS NULL THEN '未知' ELSE {expr} END")
        }
    }
}

fn flow_counterparty_filter_sql(filter: &QueryChartFilterArg) -> String {
    let mut parts = vec![format!(
        "{} = {}",
        counterparty_label_expr(),
        crate::sql_literal(filter.value.trim())
    )];
    match filter_payload_text(filter, "side").as_str() {
        "in" => parts.push("dc_val = '进'".to_string()),
        "out" => parts.push("dc_val = '出'".to_string()),
        _ => {}
    }
    parts.join(" AND ")
}

fn chart_ts_expr() -> &'static str {
    "COALESCE(txn_ts, TRY_CAST(txn_time AS TIMESTAMP))"
}

fn filter_payload_text(filter: &QueryChartFilterArg, key: &str) -> String {
    filter
        .payload
        .as_object()
        .and_then(|payload| payload.get(key))
        .map(|value| match value {
            serde_json::Value::String(text) => text.trim().to_string(),
            serde_json::Value::Null => String::new(),
            other => other.to_string().trim().to_string(),
        })
        .unwrap_or_default()
}

fn filter_payload_i64(filter: &QueryChartFilterArg, key: &str) -> Option<i64> {
    filter_payload_text(filter, key).parse::<i64>().ok()
}

pub(super) fn counterparty_label_expr() -> &'static str {
    "CASE \
       WHEN cp_placeholder_kind IS NOT NULL \
       THEN COALESCE(NULLIF(TRIM(counterparty_name), ''), '未知对手') \
       ELSE COALESCE(NULLIF(TRIM(stats_name_key), ''), NULLIF(TRIM(counterparty_name), ''), \
                     NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), \
                     NULLIF(TRIM(counterparty_acct), ''), '未知对手') \
     END"
}

pub(super) fn metric_value_from_counters(
    metric_mode: &str,
    direction_mode: &str,
    in_amount: f64,
    out_amount: f64,
    in_count: i64,
    out_count: i64,
    in_counterparty_count: i64,
    out_counterparty_count: i64,
    all_counterparty_count: i64,
) -> f64 {
    let metric = normalize_metric_mode(metric_mode);
    let direction = normalize_direction_mode(direction_mode);
    if metric == "count" {
        if direction == "in" {
            return in_count as f64;
        }
        if direction == "out" {
            return out_count as f64;
        }
        if direction == "net" {
            return (in_count - out_count) as f64;
        }
        return (in_count + out_count) as f64;
    }
    if metric == "counterparty" {
        if direction == "in" {
            return in_counterparty_count as f64;
        }
        if direction == "out" {
            return out_counterparty_count as f64;
        }
        if direction == "net" {
            return (in_counterparty_count - out_counterparty_count) as f64;
        }
        return all_counterparty_count as f64;
    }
    if direction == "in" {
        return in_amount;
    }
    if direction == "out" {
        return out_amount;
    }
    if direction == "net" {
        return in_amount - out_amount;
    }
    in_amount + out_amount
}

pub(super) fn round2(value: f64) -> f64 {
    (value * 100.0).round() / 100.0
}

fn normalized_success_sql(column: &str, bucket: &str) -> String {
    let expr = format!("LOWER(TRIM(COALESCE({column}, '')))");
    let values = if bucket == "success" {
        "'是','成功','true','1','y','yes','success','ok'"
    } else {
        "'否','失败','false','0','n','no','failed','fail','error'"
    };
    format!("{expr} IN ({values})")
}

fn normalized_cash_sql(column: &str, bucket: &str) -> String {
    let expr = format!("LOWER(TRIM(COALESCE({column}, '')))");
    let values = if bucket == "cash" {
        "'是','现金','cash','1','y','yes'"
    } else {
        "'否','非现金','转账','non-cash','nocash','0','n','no'"
    };
    format!("{expr} IN ({values})")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn amount_bucket_requires_known_finite_amount_without_zero_fill() {
        for label in ["1千以下", "1千-1万", "1万-5万", "5万-10万", "10万以上"] {
            let sql = amount_bucket_filter_sql(label);
            assert!(
                sql.contains("amount IS NOT NULL"),
                "label={label} sql={sql}"
            );
            assert!(sql.contains("isfinite(amount)"), "label={label} sql={sql}");
            assert!(
                !sql.contains("COALESCE(amount, 0)"),
                "label={label} sql={sql}"
            );
        }
    }
}
