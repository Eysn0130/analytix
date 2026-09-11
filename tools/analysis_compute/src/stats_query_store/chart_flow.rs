use anyhow::{Context, Result};
use duckdb::Connection;
use serde_json::{json, Value};

use super::args::QueryChartFilterArg;
use super::args::QueryChartFlowArgs;
use super::chart_common::{
    build_chart_detail_where_sql, chart_dedupe_enabled, chart_detail_source_table_sql,
    counterparty_label_expr, normalize_selected_keys, normalize_selection_mode,
    require_chart_amount_query_ready, require_chart_scope, round2, ChartSourceRequest,
};
use super::session::VerifiedStatsQuerySession;

#[derive(Clone)]
struct ChartFlowItem {
    label: String,
    direction: &'static str,
    amount: f64,
    count: i64,
    first_time: String,
    last_time: String,
}

struct ChartFlowQueryData {
    center_name: String,
    items: Vec<ChartFlowItem>,
}

struct ChartFlowQueryRow {
    section: String,
    center_name: String,
    dc_val: String,
    item: ChartFlowItem,
}

struct ChartFlowRequest {
    selected: Vec<String>,
    date_start: String,
    date_end: String,
    direction: String,
    success_filter: String,
    cash_filter: String,
    selection_mode: String,
    chart_filters: Vec<QueryChartFilterArg>,
}

impl ChartFlowRequest {
    fn from_args(args: &QueryChartFlowArgs) -> Self {
        let selected = normalize_selected_keys(&args.selected_keys);
        let selection_mode = normalize_selection_mode(&selected, &args.selection_mode);
        Self {
            selected,
            date_start: args.date_start.trim().to_string(),
            date_end: args.date_end.trim().to_string(),
            direction: args.direction.trim().to_string(),
            success_filter: args.success_filter.trim().to_string(),
            cash_filter: args.cash_filter.trim().to_string(),
            selection_mode,
            chart_filters: args.chart_filters.clone(),
        }
    }
}

impl ChartFlowRequest {
    fn source_request(&self) -> ChartSourceRequest {
        ChartSourceRequest {
            selected: self.selected.clone(),
            date_start: self.date_start.clone(),
            date_end: self.date_end.clone(),
            success_filter: self.success_filter.clone(),
            cash_filter: self.cash_filter.clone(),
            chart_filters: self.chart_filters.clone(),
        }
    }
}

pub(crate) fn query_chart_flow(args: &QueryChartFlowArgs) -> Result<Value> {
    let request = ChartFlowRequest::from_args(args);
    require_chart_scope(&request.selected)?;

    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let result = query_chart_flow_for_request(&mut session, &request)?;
    session.commit()?;
    Ok(result)
}

pub(crate) fn query_chart_flow_with_session(
    session: &mut VerifiedStatsQuerySession<'_>,
    args: &QueryChartFlowArgs,
) -> Result<Value> {
    let request = ChartFlowRequest::from_args(args);
    require_chart_scope(&request.selected)?;
    query_chart_flow_for_request(session, &request)
}

fn query_chart_flow_for_request(
    session: &mut VerifiedStatsQuerySession<'_>,
    request: &ChartFlowRequest,
) -> Result<Value> {
    let where_sql =
        build_chart_detail_where_sql(&request.source_request(), Some(&request.direction));
    let dedupe_enabled = chart_dedupe_enabled(session.conn(), &request.selected)?;
    require_chart_amount_query_ready(session, &where_sql, dedupe_enabled)?;
    let data = query_chart_flow_data(session.conn(), &where_sql, dedupe_enabled)?;
    let center_label = center_label_from_request(request, &data.center_name);
    Ok(build_chart_flow_payload(center_label, data.items))
}

fn center_label_from_request(request: &ChartFlowRequest, current_name: &str) -> String {
    if request.selection_mode == "single-card" {
        let selected_key = request.selected.first().map(String::as_str).unwrap_or("");
        let current_name = current_name.trim();
        if !current_name.is_empty() {
            return current_name.to_string();
        }
        if !selected_key.is_empty() {
            return selected_key.to_string();
        }
        return "当前账户".to_string();
    }
    "已选账户集合".to_string()
}

fn query_chart_flow_data(
    conn: &Connection,
    where_sql: &str,
    dedupe_enabled: bool,
) -> Result<ChartFlowQueryData> {
    let sql = chart_flow_data_sql(where_sql, dedupe_enabled);
    let mut stmt = conn.prepare(&sql)?;
    let mapped = stmt.query_map([], chart_flow_query_row_from_row)?;
    let mut center_name = String::new();
    let mut items = Vec::new();
    for item in mapped {
        let row = item?;
        if row.section == "center" {
            center_name = row.center_name;
            continue;
        }
        if row.dc_val == "进" {
            items.push(ChartFlowItem {
                direction: "in",
                ..row.item
            });
        } else if row.dc_val == "出" {
            items.push(ChartFlowItem {
                direction: "out",
                ..row.item
            });
        }
    }
    Ok(ChartFlowQueryData { center_name, items })
}

fn chart_flow_data_sql(where_sql: &str, dedupe_enabled: bool) -> String {
    let label_expr = counterparty_label_expr();
    let detail_source = chart_detail_source_table_sql(where_sql, dedupe_enabled);
    format!(
        "
        WITH filtered AS (
          SELECT
            id,
            account_open_name,
            {label_expr} AS label,
            dc_val,
            txn_ts,
            txn_time,
            ABS(amount) AS amount_abs
          FROM {detail_source}
        ), center_row AS (
          SELECT account_open_name
          FROM filtered
          WHERE NULLIF(TRIM(COALESCE(account_open_name, '')), '') IS NOT NULL
          ORDER BY (txn_ts IS NULL) ASC, txn_ts ASC, id ASC
          LIMIT 1
        ), flow_rows AS (
          SELECT
            label,
            dc_val,
            CAST(SUM(amount_abs) AS DOUBLE) AS amount,
            CAST(COUNT(1) AS BIGINT) AS txn_count,
            COALESCE(CAST(MIN(txn_ts) AS VARCHAR), MIN(NULLIF(TRIM(txn_time), ''))) AS first_time,
            COALESCE(CAST(MAX(txn_ts) AS VARCHAR), MAX(NULLIF(TRIM(txn_time), ''))) AS last_time
          FROM filtered
          GROUP BY label, dc_val
        )
        SELECT
          section,
          center_name,
          label,
          dc_val,
          amount,
          txn_count,
          first_time,
          last_time
        FROM (
          SELECT
            0 AS section_order,
            'center' AS section,
            COALESCE(account_open_name, '') AS center_name,
            '' AS label,
            '' AS dc_val,
            0.0 AS amount,
            0 AS txn_count,
            '' AS first_time,
            '' AS last_time
          FROM center_row
          UNION ALL
          SELECT
            1 AS section_order,
            'item' AS section,
            '' AS center_name,
            label,
            COALESCE(dc_val, '') AS dc_val,
            amount,
            txn_count,
            COALESCE(first_time, '') AS first_time,
            COALESCE(last_time, '') AS last_time
          FROM flow_rows
        )
        ORDER BY section_order ASC, amount DESC, label ASC
        ",
    )
}

fn chart_flow_query_row_from_row(row: &duckdb::Row<'_>) -> duckdb::Result<ChartFlowQueryRow> {
    Ok(ChartFlowQueryRow {
        section: row.get::<_, Option<String>>(0)?.unwrap_or_default(),
        center_name: row.get::<_, Option<String>>(1)?.unwrap_or_default(),
        dc_val: row.get::<_, Option<String>>(3)?.unwrap_or_default(),
        item: ChartFlowItem {
            label: row
                .get::<_, Option<String>>(2)?
                .unwrap_or_else(|| "未知对手".to_string()),
            direction: "other",
            amount: row.get::<_, f64>(4)?,
            count: row.get::<_, i64>(5)?,
            first_time: row.get::<_, Option<String>>(6)?.unwrap_or_default(),
            last_time: row.get::<_, Option<String>>(7)?.unwrap_or_default(),
        },
    })
}

fn build_chart_flow_payload(center_label: String, rows: Vec<ChartFlowItem>) -> Value {
    let center_id = "selected-center";
    let mut inbound = Vec::new();
    let mut outbound = Vec::new();
    let mut total_in_amount = 0.0;
    let mut total_out_amount = 0.0;
    let mut total_in_count = 0_i64;
    let mut total_out_count = 0_i64;

    for row in rows {
        let item = row.clone();
        if item.direction == "in" {
            total_in_amount += item.amount;
            total_in_count += item.count;
            inbound.push(item);
        } else if item.direction == "out" {
            total_out_amount += item.amount;
            total_out_count += item.count;
            outbound.push(item);
        }
    }

    sort_flow_items(&mut inbound);
    sort_flow_items(&mut outbound);

    let mut nodes = vec![json!({
        "id": center_id,
        "label": center_label,
        "side": "center",
        "value": 0,
        "amount": 0,
        "count": 0,
    })];
    let mut links = Vec::new();
    for (index, item) in inbound.iter().take(10).enumerate() {
        let node_id = format!("in-{index}");
        nodes.push(flow_node(&node_id, item, "in"));
        links.push(flow_link(&node_id, center_id, item, "in"));
    }
    for (index, item) in outbound.iter().take(10).enumerate() {
        let node_id = format!("out-{index}");
        nodes.push(flow_node(&node_id, item, "out"));
        links.push(flow_link(center_id, &node_id, item, "out"));
    }

    json!({
        "center_label": center_label,
        "nodes": nodes,
        "links": links,
        "totals": {
            "in_amount": round2(total_in_amount),
            "out_amount": round2(total_out_amount),
            "in_count": total_in_count,
            "out_count": total_out_count,
        },
        "inbound_items": inbound.iter().map(flow_item_value).collect::<Vec<_>>(),
        "outbound_items": outbound.iter().map(flow_item_value).collect::<Vec<_>>(),
    })
}

fn sort_flow_items(items: &mut [ChartFlowItem]) {
    items.sort_by(|left, right| {
        right
            .amount
            .partial_cmp(&left.amount)
            .unwrap_or(std::cmp::Ordering::Equal)
            .then_with(|| left.label.cmp(&right.label))
    });
}

fn flow_item_value(item: &ChartFlowItem) -> Value {
    json!({
        "label": item.label,
        "amount": round2(item.amount),
        "count": item.count,
        "first_time": item.first_time,
        "last_time": item.last_time,
    })
}

fn flow_node(id: &str, item: &ChartFlowItem, side: &str) -> Value {
    json!({
        "id": id,
        "label": item.label,
        "side": side,
        "value": round2(item.amount),
        "amount": round2(item.amount),
        "count": item.count,
        "first_time": item.first_time,
        "last_time": item.last_time,
    })
}

fn flow_link(source: &str, target: &str, item: &ChartFlowItem, direction: &str) -> Value {
    json!({
        "source": source,
        "target": target,
        "label": item.label,
        "direction": direction,
        "amount": round2(item.amount),
        "count": item.count,
        "value": round2(item.amount),
        "first_time": item.first_time,
        "last_time": item.last_time,
    })
}
