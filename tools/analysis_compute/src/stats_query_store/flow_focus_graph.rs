use std::collections::{BTreeSet, HashMap, HashSet};

use anyhow::{bail, Result};
use chrono::{Days, NaiveDate, NaiveDateTime};
use duckdb::Row;
use serde_json::{json, Map, Value};

use super::amount_coverage::require_complete_detail_amount_coverage;
use super::args::QueryFlowFocusGraphArgs;
use super::dedupe_sql::{deduped_source_ctes, detail_stats_dedupe_key_expr};
use super::session::VerifiedStatsQuerySession;
use super::values::{direction_value, sql_literal_list, FlowFocusRow};

const UNKNOWN_NAME_LABEL: &str = "未知户名";
const UNKNOWN_CARD_LABEL: &str = "未知账号";
const UNKNOWN_PREFIX: &str = "__unknown_cp__name::";
const PLACEHOLDER_PREFIX: &str = "__cp_placeholder__::";
const MIRROR_TOLERANCE_SECONDS: i64 = 90;

#[derive(Clone, Debug)]
struct EdgeAggregate {
    a: String,
    b: String,
    amount: f64,
    count: i64,
    out_amount: f64,
    in_amount: f64,
    first_ts: String,
    last_ts: String,
}

pub(crate) fn query_flow_focus_graph(args: &QueryFlowFocusGraphArgs) -> Result<Option<Value>> {
    if !args.rows.min_amount.is_finite() {
        bail!("--min-amount must be finite");
    }
    if args
        .expected_total_amount
        .is_some_and(|value| !value.is_finite())
    {
        bail!("--expected-total-amount must be finite");
    }
    let conn = crate::open_readonly_connection(&args.rows.db_path)?;
    crate::configure_connection(&conn)?;
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let rows = query_flow_focus_detail_rows(args, &mut session)?;
    let graph = build_flow_focus_graph_from_detail_rows(args, &rows)?;
    session.commit()?;
    Ok(graph)
}

fn build_flow_focus_graph(
    args: &QueryFlowFocusGraphArgs,
    rows: &[FlowFocusRow],
) -> Result<Option<Value>> {
    let view_mode = normalize_view_mode(&args.view_mode);
    let seen_seed = args
        .seed_ids
        .iter()
        .map(|item| item.trim().to_string())
        .filter(|item| !item.is_empty())
        .collect::<HashSet<_>>();
    let mut edges: HashMap<(String, String), EdgeAggregate> = HashMap::new();
    let mut node_amount: HashMap<String, f64> = HashMap::new();
    let mut node_count: HashMap<String, i64> = HashMap::new();
    let mut node_name: HashMap<String, String> = HashMap::new();
    let mut node_name_source: HashMap<String, String> = HashMap::new();
    let mut node_display: HashMap<String, String> = HashMap::new();
    let mut cp_display_ids: HashMap<String, BTreeSet<String>> = HashMap::new();
    let mut node_display_list: HashMap<String, Vec<String>> = HashMap::new();
    let mut acct_seen: HashSet<String> = HashSet::new();
    let mut cp_name_missing: HashSet<String> = HashSet::new();

    for row in rows {
        if row.dc_val() != "进" && row.dc_val() != "出" {
            continue;
        }
        let Some((acct_key, cp_key)) = apply_row_context(
            row,
            &mut acct_seen,
            &mut node_name,
            &mut node_name_source,
            &mut node_display,
            &mut cp_display_ids,
            &mut cp_name_missing,
        ) else {
            continue;
        };
        let (flow_src, flow_tgt) = if row.dc_val() == "出" {
            (acct_key, cp_key)
        } else {
            (cp_key, acct_key)
        };
        apply_flow_agg(
            &mut edges,
            &mut node_amount,
            &mut node_count,
            &flow_src,
            &flow_tgt,
            row.amt_sum(),
            row.txn_count(),
            row.first_ts(),
            row.last_ts(),
        );
    }

    for (node_id, ids) in cp_display_ids {
        if ids.is_empty() {
            continue;
        }
        let show_ids = ids.into_iter().collect::<Vec<_>>();
        let suffix = if show_ids.len() > 6 {
            format!(" 等{}个", show_ids.len())
        } else {
            String::new()
        };
        node_display.insert(
            node_id.clone(),
            format!(
                "{}{}",
                show_ids[..show_ids.len().min(6)].join(" / "),
                suffix
            ),
        );
        node_display_list.insert(node_id, show_ids);
    }

    let mut edge_list = edges.into_values().collect::<Vec<_>>();
    edge_list.sort_by(|a, b| {
        b.amount
            .partial_cmp(&a.amount)
            .unwrap_or(std::cmp::Ordering::Equal)
    });
    let self_loop_edges = edge_list.iter().filter(|edge| edge.a == edge.b).count() as i64;

    let mut node_ids = BTreeSet::new();
    for edge in &edge_list {
        node_ids.insert(edge.a.clone());
        node_ids.insert(edge.b.clone());
    }
    if args.rows.include_missing_counterparty && !node_ids.is_empty() {
        for node_id in node_ids.iter() {
            if node_id.starts_with(UNKNOWN_PREFIX) && !node_display.contains_key(node_id) {
                node_display.insert(node_id.clone(), UNKNOWN_CARD_LABEL.to_string());
            }
            if is_placeholder_node_id(node_id) && !node_display.contains_key(node_id) {
                node_display.insert(
                    node_id.clone(),
                    placeholder_kind_label(&placeholder_kind_from_node_id(node_id))
                        .unwrap_or(node_id)
                        .to_string(),
                );
            }
        }
    }

    let mut nodes = Vec::new();
    for node_id in node_ids {
        let mut name = node_name.get(&node_id).cloned().unwrap_or_default();
        if name.trim().is_empty()
            && !args.focus_label.trim().is_empty()
            && !args.focus_id_raw.trim().is_empty()
            && norm_focus_key(&node_id) == norm_focus_key(&args.focus_id_raw)
        {
            name = args.focus_label.trim().to_string();
        }
        let display_id = node_display
            .get(&node_id)
            .cloned()
            .unwrap_or_else(|| node_id.clone());
        let title = if is_placeholder_node_id(&node_id) {
            if name.trim().is_empty() {
                display_id.clone()
            } else {
                format!("{display_id} | {name}")
            }
        } else if name.trim().is_empty() {
            node_id.clone()
        } else {
            name.clone()
        };
        let display_ids = node_display_list
            .get(&node_id)
            .map(|items| Value::Array(items.iter().cloned().map(Value::String).collect()))
            .unwrap_or(Value::Null);
        let ntype = if seen_seed.contains(&node_id) {
            "seed"
        } else if acct_seen.contains(&node_id) {
            "account"
        } else {
            "node"
        };
        let Some(total_amount) = node_amount.get(&node_id).copied() else {
            bail!("stats_query_numeric_state_invalid");
        };
        let Some(total_count) = node_count.get(&node_id).copied() else {
            bail!("stats_query_numeric_state_invalid");
        };
        nodes.push(json!({
            "id": node_id,
            "title": title,
            "name": name,
            "display_id": display_id,
            "display_ids": display_ids,
            "ntype": ntype,
            "total_amount": crate::round2(total_amount),
            "total_count": total_count,
        }));
    }

    let mut out_edges = Vec::new();
    let mut total_amount = 0.0_f64;
    for edge in &edge_list {
        if view_mode == "net" {
            let net = edge.out_amount - edge.in_amount;
            if net.abs() < 1e-9 {
                continue;
            }
            let (source, target) = if net > 0.0 {
                (&edge.a, &edge.b)
            } else {
                (&edge.b, &edge.a)
            };
            let amount = net.abs();
            total_amount += amount;
            out_edges.push(json!({
                "id": format!("{}=={}", edge.a, edge.b),
                "source": source,
                "target": target,
                "amount": crate::round2(amount),
                "count": edge.count,
                "out_amount": crate::round2(edge.out_amount),
                "in_amount": crate::round2(edge.in_amount),
                "forward_amount": crate::round2(amount),
                "reverse_amount": 0.0,
                "mode": "single",
                "label": format!("￥{amount:.2}"),
                "first_time": edge.first_ts,
                "last_time": edge.last_ts,
            }));
            continue;
        }

        let (source, target) = if edge.out_amount > 0.0 && edge.in_amount == 0.0 {
            (&edge.a, &edge.b)
        } else if edge.in_amount > 0.0 && edge.out_amount == 0.0 {
            (&edge.b, &edge.a)
        } else if edge.out_amount >= edge.in_amount {
            (&edge.a, &edge.b)
        } else {
            (&edge.b, &edge.a)
        };
        let (forward_amount, reverse_amount) = if source == &edge.a && target == &edge.b {
            (edge.out_amount, edge.in_amount)
        } else {
            (edge.in_amount, edge.out_amount)
        };
        total_amount += edge.amount;
        out_edges.push(json!({
            "id": format!("{}=={}", edge.a, edge.b),
            "source": source,
            "target": target,
            "amount": crate::round2(edge.amount),
            "count": edge.count,
            "out_amount": crate::round2(edge.out_amount),
            "in_amount": crate::round2(edge.in_amount),
            "forward_amount": crate::round2(forward_amount),
            "reverse_amount": crate::round2(reverse_amount),
            "mode": if edge.out_amount > 0.0 && edge.in_amount > 0.0 { "double" } else { "single" },
            "first_time": edge.first_ts,
            "last_time": edge.last_ts,
        }));
    }

    let total_amount = crate::round2(total_amount);

    let mut context = Map::new();
    context.insert(
        "source".to_string(),
        Value::String(args.source.trim().to_string()),
    );
    context.insert(
        "request_id".to_string(),
        Value::String(args.request_id.trim().to_string()),
    );
    context.insert(
        "date_start".to_string(),
        Value::String(args.rows.date_start.trim().to_string()),
    );
    context.insert(
        "date_end".to_string(),
        Value::String(date_end_label(&args.rows.date_end_excl)),
    );
    context.insert("focus_only".to_string(), Value::Bool(true));
    context.insert("focus_counterparty_strict".to_string(), Value::Bool(true));
    context.insert(
        "focus_id_count".to_string(),
        json!(args
            .rows
            .selected_focus_ids
            .iter()
            .filter(|item| !item.trim().is_empty())
            .count()),
    );
    context.insert("focus_name_count".to_string(), json!(0));
    context.insert("focus_unknown_name".to_string(), Value::Bool(false));
    context.insert(
        "include_missing_counterparty".to_string(),
        Value::Bool(args.rows.include_missing_counterparty),
    );
    context.insert(
        "focus_key_type".to_string(),
        Value::String(args.focus_key_type.trim().to_string()),
    );
    context.insert(
        "build_mode".to_string(),
        Value::String("stats_focus_account_fast".to_string()),
    );

    let mut stats = Map::new();
    stats.insert("requested_depth".to_string(), json!(args.depth));
    stats.insert(
        "direction".to_string(),
        Value::String(args.rows.direction.trim().to_string()),
    );
    stats.insert("seed_count".to_string(), json!(args.seed_ids.len()));
    stats.insert("node_count".to_string(), json!(nodes.len()));
    stats.insert("edge_count".to_string(), json!(out_edges.len()));
    stats.insert("self_loop_edge_count".to_string(), json!(self_loop_edges));
    stats.insert("total_amount".to_string(), json!(total_amount));
    stats.insert("view_mode".to_string(), Value::String(view_mode));
    stats.insert("context_applied".to_string(), Value::Object(context));
    if let Some(expected) = args.expected_total_amount {
        stats.insert(
            "expected_total_amount".to_string(),
            json!(crate::round2(expected)),
        );
        stats.insert(
            "expected_total_amount_delta".to_string(),
            json!(crate::round2(total_amount - expected)),
        );
    }
    if let Some(expected) = args.expected_row_count {
        stats.insert("expected_row_count".to_string(), json!(expected));
        stats.insert(
            "expected_row_count_delta".to_string(),
            json!(out_edges.len() as i64 - expected),
        );
    }

    Ok(Some(json!({
        "nodes": nodes,
        "edges": out_edges,
        "stats": Value::Object(stats),
    })))
}

#[derive(Clone, Debug)]
struct DetailFlowRow {
    acct_key: String,
    cp_key: String,
    cp_key_raw: String,
    dc_val: String,
    amount: f64,
    txn_ts: String,
    open_name: String,
    cp_name: String,
    primary: bool,
}

impl DetailFlowRow {
    fn dc_val(&self) -> &str {
        self.dc_val.trim()
    }

    fn first_ts(&self) -> &str {
        self.txn_ts.trim()
    }

    fn last_ts(&self) -> &str {
        self.txn_ts.trim()
    }
}

#[derive(Clone, Debug)]
struct PendingMirrorRow {
    row: DetailFlowRow,
}

fn build_flow_focus_graph_from_detail_rows(
    args: &QueryFlowFocusGraphArgs,
    rows: &[DetailFlowRow],
) -> Result<Option<Value>> {
    let aggregate_rows = aggregate_detail_rows_with_mirror_dedupe(args, rows);
    build_flow_focus_graph(args, &aggregate_rows)
}

fn aggregate_detail_rows_with_mirror_dedupe(
    args: &QueryFlowFocusGraphArgs,
    rows: &[DetailFlowRow],
) -> Vec<FlowFocusRow> {
    let mut aggregates: HashMap<(String, String, String, String), FlowFocusRowAccumulator> =
        HashMap::new();
    let mut pending: HashMap<(String, String, String), MirrorBuckets> = HashMap::new();

    for row in rows {
        let dc = row.dc_val();
        if dc != "进" && dc != "出" {
            continue;
        }
        if row.acct_key.trim().is_empty() || row.cp_key.trim().is_empty() {
            continue;
        }
        let (flow_src, flow_tgt, side) = if dc == "出" {
            (row.acct_key.trim(), row.cp_key.trim(), MirrorSide::Source)
        } else {
            (row.cp_key.trim(), row.acct_key.trim(), MirrorSide::Target)
        };
        let amount = row.amount.abs();
        if flow_src != flow_tgt {
            let key = (
                flow_src.to_string(),
                flow_tgt.to_string(),
                format!("{:.2}", crate::round2(amount)),
            );
            let buckets = pending.entry(key).or_default();
            if let Some(matched) = buckets.take_opposite(side, row.first_ts()) {
                if !row.primary && !matched.row.primary {
                    continue;
                }
                let mut merged = if row.primary {
                    row.clone()
                } else if matched.row.primary {
                    matched.row.clone()
                } else {
                    row.clone()
                };
                merged.txn_ts = merge_first_ts(matched.row.first_ts(), row.first_ts());
                add_detail_aggregate(&mut aggregates, &merged, amount, 1);
                continue;
            }
            buckets.push(side, PendingMirrorRow { row: row.clone() });
            continue;
        }
        add_detail_aggregate(&mut aggregates, row, amount, 1);
    }

    for buckets in pending.into_values() {
        for pending_row in buckets.source.into_iter().chain(buckets.target.into_iter()) {
            if !pending_row.row.primary {
                continue;
            }
            add_detail_aggregate(
                &mut aggregates,
                &pending_row.row,
                pending_row.row.amount.abs(),
                1,
            );
        }
    }

    let mut out = aggregates
        .into_values()
        .map(FlowFocusRowAccumulator::into_flow_focus_row)
        .collect::<Vec<_>>();
    out.sort_by(|a, b| {
        b.amt_sum()
            .partial_cmp(&a.amt_sum())
            .unwrap_or(std::cmp::Ordering::Equal)
    });
    if args.rows.min_amount > 0.0 {
        out.retain(|row| row.amt_sum() >= args.rows.min_amount);
    }
    out
}

fn add_detail_aggregate(
    aggregates: &mut HashMap<(String, String, String, String), FlowFocusRowAccumulator>,
    row: &DetailFlowRow,
    amount: f64,
    count: i64,
) {
    let key = (
        row.acct_key.trim().to_string(),
        row.cp_key.trim().to_string(),
        row.cp_key_raw.trim().to_string(),
        row.dc_val().to_string(),
    );
    let entry = aggregates
        .entry(key)
        .or_insert_with(|| FlowFocusRowAccumulator::from_detail_row(row));
    entry.txn_count += count;
    entry.amt_sum += amount;
    entry.first_ts = merge_first_ts(&entry.first_ts, row.first_ts());
    entry.last_ts = merge_last_ts(&entry.last_ts, row.last_ts());
    if entry.open_name.trim().is_empty() && !row.open_name.trim().is_empty() {
        entry.open_name = row.open_name.trim().to_string();
    }
    if entry.cp_name.trim().is_empty() && !row.cp_name.trim().is_empty() {
        entry.cp_name = row.cp_name.trim().to_string();
    }
}

#[derive(Clone, Debug)]
struct FlowFocusRowAccumulator {
    acct_key: String,
    cp_key: String,
    cp_key_raw: String,
    dc_val: String,
    txn_count: i64,
    amt_sum: f64,
    first_ts: String,
    last_ts: String,
    open_name: String,
    cp_name: String,
}

impl FlowFocusRowAccumulator {
    fn from_detail_row(row: &DetailFlowRow) -> Self {
        Self {
            acct_key: row.acct_key.trim().to_string(),
            cp_key: row.cp_key.trim().to_string(),
            cp_key_raw: row.cp_key_raw.trim().to_string(),
            dc_val: row.dc_val().to_string(),
            txn_count: 0,
            amt_sum: 0.0,
            first_ts: String::new(),
            last_ts: String::new(),
            open_name: row.open_name.trim().to_string(),
            cp_name: row.cp_name.trim().to_string(),
        }
    }

    fn into_flow_focus_row(self) -> FlowFocusRow {
        FlowFocusRow::new(
            self.acct_key,
            self.cp_key,
            self.cp_key_raw,
            self.dc_val,
            self.txn_count,
            self.amt_sum,
            self.first_ts,
            self.last_ts,
            self.open_name,
            self.cp_name,
        )
    }
}

#[derive(Clone, Copy, Debug)]
enum MirrorSide {
    Source,
    Target,
}

#[derive(Default)]
struct MirrorBuckets {
    source: Vec<PendingMirrorRow>,
    target: Vec<PendingMirrorRow>,
}

impl MirrorBuckets {
    fn push(&mut self, side: MirrorSide, row: PendingMirrorRow) {
        match side {
            MirrorSide::Source => self.source.push(row),
            MirrorSide::Target => self.target.push(row),
        }
    }

    fn take_opposite(&mut self, side: MirrorSide, txn_ts: &str) -> Option<PendingMirrorRow> {
        let bucket = match side {
            MirrorSide::Source => &mut self.target,
            MirrorSide::Target => &mut self.source,
        };
        let index = bucket
            .iter()
            .position(|candidate| timestamps_within_tolerance(candidate.row.first_ts(), txn_ts))?;
        Some(bucket.remove(index))
    }
}

fn timestamps_within_tolerance(left: &str, right: &str) -> bool {
    let Some(left_ts) = parse_ts(left) else {
        return false;
    };
    let Some(right_ts) = parse_ts(right) else {
        return false;
    };
    (left_ts - right_ts).num_seconds().abs() <= MIRROR_TOLERANCE_SECONDS
}

fn parse_ts(value: &str) -> Option<NaiveDateTime> {
    let text = value.trim();
    if text.is_empty() {
        return None;
    }
    NaiveDateTime::parse_from_str(text, "%Y-%m-%d %H:%M:%S")
        .or_else(|_| NaiveDateTime::parse_from_str(text, "%Y-%m-%dT%H:%M:%S"))
        .ok()
}

fn query_flow_focus_detail_rows(
    args: &QueryFlowFocusGraphArgs,
    session: &mut VerifiedStatsQuerySession<'_>,
) -> Result<Vec<DetailFlowRow>> {
    let seeds = trim_non_empty(&args.rows.query_seed_ids);
    let focus_ids = trim_non_empty(&args.rows.selected_focus_ids);
    let placeholder_kinds = trim_non_empty(&args.rows.selected_placeholder_kinds);
    if seeds.is_empty() || (focus_ids.is_empty() && placeholder_kinds.is_empty()) {
        bail!("stats_query_scope_required");
    }
    let acct_ids = merge_trimmed_unique(&seeds, &focus_ids);

    require_complete_detail_amount_coverage(session.conn())?;

    let dedupe_enabled =
        acct_ids.len() > 1 && crate::table_exists(session.conn(), crate::ACCOUNT_DIM_TABLE)?;
    let sql = build_detail_flow_focus_sql(
        args,
        &seeds,
        &focus_ids,
        &placeholder_kinds,
        &acct_ids,
        dedupe_enabled,
    );
    session.require_nonempty_query("flow_focus_graph_rows", &sql)?;
    let mut stmt = session.conn().prepare(&sql)?;
    let mapped = stmt.query_map([], detail_flow_row_from_row)?;
    let mut rows = Vec::new();
    for item in mapped {
        rows.push(item?);
    }
    Ok(rows)
}

fn build_detail_flow_focus_sql(
    args: &QueryFlowFocusGraphArgs,
    seeds: &[String],
    focus_ids: &[String],
    placeholder_kinds: &[String],
    acct_ids: &[String],
    dedupe_enabled: bool,
) -> String {
    let mut filters = Vec::new();
    filters.push(format!("acct_key IN ({})", sql_literal_list(acct_ids)));
    filters.push("dc_val IN ('进','出')".to_string());
    if !args.rows.date_start.trim().is_empty() {
        filters.push(format!(
            "txn_day >= CAST({} AS DATE)",
            crate::sql_literal(args.rows.date_start.trim())
        ));
    }
    if !args.rows.date_end_excl.trim().is_empty() {
        filters.push(format!(
            "txn_day < CAST({} AS DATE)",
            crate::sql_literal(args.rows.date_end_excl.trim())
        ));
    }
    if args.rows.min_amount > 0.0 {
        filters.push(format!("ABS(amount) >= {}", args.rows.min_amount));
    }
    if let Some(dir_value) = direction_value(&args.rows.direction) {
        filters.push(format!("dc_val = {}", crate::sql_literal(dir_value)));
    }
    let mut primary_parts = Vec::new();
    if !focus_ids.is_empty() {
        primary_parts.push(format!(
            "(cp_placeholder_kind IS NULL AND cp_key IN ({}))",
            sql_literal_list(focus_ids)
        ));
    }
    if !placeholder_kinds.is_empty() {
        primary_parts.push(format!(
            "(cp_placeholder_kind IN ({}))",
            sql_literal_list(placeholder_kinds)
        ));
    }
    let selected_sql = format!("({})", primary_parts.join(" OR "));
    let primary_sql = format!(
        "(acct_key IN ({}) AND {selected_sql})",
        sql_literal_list(seeds)
    );
    let mut focus_parts = vec![primary_sql.clone()];
    if !seeds.is_empty() {
        focus_parts.push(format!(
            "(cp_placeholder_kind IS NULL AND cp_key IN ({}))",
            sql_literal_list(seeds)
        ));
    }
    filters.push(format!("({})", focus_parts.join(" OR ")));
    let source_sql = format!(
        "SELECT d.*, {} AS stats_dedupe_key FROM {} d WHERE {}",
        detail_stats_dedupe_key_expr("d"),
        crate::DETAIL_TABLE,
        filters.join(" AND "),
    );
    format!(
        "WITH {} SELECT acct_key, \
                CASE \
                  WHEN cp_placeholder_kind IS NOT NULL AND counterparty_name IS NOT NULL AND TRIM(counterparty_name) <> '' THEN {} || '::name::' || counterparty_name \
                  WHEN cp_placeholder_kind IS NOT NULL THEN {} \
                  WHEN cp_key IS NOT NULL THEN cp_key \
                  WHEN counterparty_name IS NOT NULL AND TRIM(counterparty_name) <> '' THEN {} || counterparty_name \
                  ELSE {} END AS cp_key, \
                cp_raw AS cp_key_raw, dc_val, ABS(amount) AS amount, \
                CAST(txn_ts AS VARCHAR) AS txn_ts, account_open_name AS open_name, \
                COALESCE(cp_name_pick, counterparty_name) AS cp_name, \
                {primary_sql} AS primary_match \
           FROM filtered \
          ORDER BY txn_ts ASC, amount DESC, acct_key ASC, cp_key ASC, dc_val ASC",
        deduped_source_ctes(&source_sql, dedupe_enabled),
        crate::placeholder_token_sql("cp_placeholder_kind"),
        crate::placeholder_token_sql("cp_placeholder_kind"),
        crate::sql_literal(UNKNOWN_PREFIX),
        crate::sql_literal(&format!("{UNKNOWN_PREFIX}__empty__")),
    )
}

fn detail_flow_row_from_row(row: &Row<'_>) -> duckdb::Result<DetailFlowRow> {
    Ok(DetailFlowRow {
        acct_key: row.get::<_, Option<String>>(0)?.unwrap_or_default(),
        cp_key: row.get::<_, Option<String>>(1)?.unwrap_or_default(),
        cp_key_raw: row.get::<_, Option<String>>(2)?.unwrap_or_default(),
        dc_val: row.get::<_, Option<String>>(3)?.unwrap_or_default(),
        amount: row.get::<_, f64>(4)?,
        txn_ts: row.get::<_, Option<String>>(5)?.unwrap_or_default(),
        open_name: row.get::<_, Option<String>>(6)?.unwrap_or_default(),
        cp_name: row.get::<_, Option<String>>(7)?.unwrap_or_default(),
        primary: row.get::<_, Option<bool>>(8)?.unwrap_or(false),
    })
}

fn trim_non_empty(values: &[String]) -> Vec<String> {
    values
        .iter()
        .map(|item| item.trim().to_string())
        .filter(|item| !item.is_empty())
        .collect()
}

fn merge_trimmed_unique(left: &[String], right: &[String]) -> Vec<String> {
    let mut seen = HashSet::new();
    let mut out = Vec::new();
    for item in left.iter().chain(right.iter()) {
        let text = item.trim();
        if text.is_empty() || !seen.insert(text.to_string()) {
            continue;
        }
        out.push(text.to_string());
    }
    out
}

fn apply_row_context(
    row: &FlowFocusRow,
    acct_seen: &mut HashSet<String>,
    node_name: &mut HashMap<String, String>,
    node_name_source: &mut HashMap<String, String>,
    node_display: &mut HashMap<String, String>,
    cp_display_ids: &mut HashMap<String, BTreeSet<String>>,
    cp_name_missing: &mut HashSet<String>,
) -> Option<(String, String)> {
    let acct_key = row.acct_key();
    let cp_key = row.cp_key();
    let cp_key_raw = row.cp_key_raw();
    let open_name = row.open_name();
    let cp_name = row.cp_name();
    if acct_key.is_empty() || cp_key.is_empty() {
        return None;
    }
    let is_placeholder_cp = is_placeholder_node_id(cp_key);
    acct_seen.insert(acct_key.to_string());
    if is_placeholder_cp {
        if let Some(label) = placeholder_kind_label(&placeholder_kind_from_node_id(cp_key)) {
            node_display.insert(cp_key.to_string(), label.to_string());
        }
    } else if !cp_key_raw.is_empty()
        && cp_key.starts_with(UNKNOWN_PREFIX)
        && !is_missing_account_key(cp_key_raw)
    {
        cp_display_ids
            .entry(cp_key.to_string())
            .or_default()
            .insert(cp_key_raw.to_string());
    }

    if !open_name.is_empty() && node_name_source.get(acct_key).map(String::as_str) != Some("open") {
        node_name.insert(acct_key.to_string(), open_name.to_string());
        node_name_source.insert(acct_key.to_string(), "open".to_string());
    }
    if cp_name.is_empty() && !cp_key.starts_with(UNKNOWN_PREFIX) && !is_placeholder_cp {
        cp_name_missing.insert(cp_key.to_string());
        if node_name_source.get(cp_key).map(String::as_str) == Some("cp") {
            node_name.insert(cp_key.to_string(), UNKNOWN_NAME_LABEL.to_string());
            node_name_source.insert(cp_key.to_string(), "unknown".to_string());
        }
    }
    if !cp_name.is_empty()
        && (is_placeholder_cp || !cp_name_missing.contains(cp_key))
        && node_name_source.get(cp_key).map(String::as_str) != Some("open")
        && is_unknown_or_empty(node_name.get(cp_key))
    {
        node_name.insert(cp_key.to_string(), cp_name.to_string());
        node_name_source.insert(cp_key.to_string(), "cp".to_string());
    }
    Some((acct_key.to_string(), cp_key.to_string()))
}

fn apply_flow_agg(
    edges: &mut HashMap<(String, String), EdgeAggregate>,
    node_amount: &mut HashMap<String, f64>,
    node_count: &mut HashMap<String, i64>,
    flow_src: &str,
    flow_tgt: &str,
    amount: f64,
    count: i64,
    first_ts: &str,
    last_ts: &str,
) {
    if flow_src.is_empty() || flow_tgt.is_empty() {
        return;
    }
    let (a, b) = if flow_src <= flow_tgt {
        (flow_src.to_string(), flow_tgt.to_string())
    } else {
        (flow_tgt.to_string(), flow_src.to_string())
    };
    let edge = edges
        .entry((a.clone(), b.clone()))
        .or_insert_with(|| EdgeAggregate {
            a: a.clone(),
            b: b.clone(),
            amount: 0.0,
            count: 0,
            out_amount: 0.0,
            in_amount: 0.0,
            first_ts: String::new(),
            last_ts: String::new(),
        });
    edge.amount += amount;
    edge.count += count;
    if flow_src == edge.a {
        edge.out_amount += amount;
    } else {
        edge.in_amount += amount;
    }
    edge.first_ts = merge_first_ts(&edge.first_ts, first_ts);
    edge.last_ts = merge_last_ts(&edge.last_ts, last_ts);

    if flow_src == flow_tgt {
        add_node_total(node_amount, node_count, flow_src, amount, count);
        return;
    }
    add_node_total(node_amount, node_count, flow_src, amount, count);
    add_node_total(node_amount, node_count, flow_tgt, amount, count);
}

fn add_node_total(
    node_amount: &mut HashMap<String, f64>,
    node_count: &mut HashMap<String, i64>,
    node_id: &str,
    amount: f64,
    count: i64,
) {
    *node_amount.entry(node_id.to_string()).or_insert(0.0) += amount;
    *node_count.entry(node_id.to_string()).or_insert(0) += count;
}

fn normalize_view_mode(value: &str) -> String {
    match value.trim().to_ascii_lowercase().as_str() {
        "net" => "net".to_string(),
        _ => "relation".to_string(),
    }
}

fn is_unknown_or_empty(value: Option<&String>) -> bool {
    match value {
        None => true,
        Some(item) => item.trim().is_empty() || item == UNKNOWN_NAME_LABEL,
    }
}

fn is_placeholder_node_id(value: &str) -> bool {
    !placeholder_kind_from_node_id(value).is_empty()
}

fn placeholder_kind_from_node_id(value: &str) -> String {
    let text = value.trim();
    let Some(remainder) = text.strip_prefix(PLACEHOLDER_PREFIX) else {
        return String::new();
    };
    let kind = remainder.split("::name::").next().unwrap_or_default();
    normalize_placeholder_kind(kind)
        .unwrap_or_default()
        .to_string()
}

fn placeholder_kind_label(kind: &str) -> Option<&'static str> {
    match kind.trim() {
        "db_null" => Some("NULL"),
        "empty" => Some("空串"),
        "slash_n" => Some("\\N"),
        "dash" => Some("-"),
        "emdash" => Some("—"),
        "fw_dash" => Some("－"),
        "literal_null" => Some("null"),
        "literal_none" => Some("none"),
        "literal_nan" => Some("nan"),
        _ => None,
    }
}

fn normalize_placeholder_kind(value: &str) -> Option<&'static str> {
    match value.trim() {
        "db_null" => Some("db_null"),
        "empty" => Some("empty"),
        "slash_n" => Some("slash_n"),
        "dash" => Some("dash"),
        "emdash" => Some("emdash"),
        "fw_dash" => Some("fw_dash"),
        "literal_null" => Some("literal_null"),
        "literal_none" => Some("literal_none"),
        "literal_nan" => Some("literal_nan"),
        _ => None,
    }
}

fn is_missing_account_key(value: &str) -> bool {
    let text = value.trim();
    if text.is_empty() || text == "\\N" || text == "-" || text == "—" || text == "－" {
        return true;
    }
    matches!(text.to_ascii_lowercase().as_str(), "null" | "none" | "nan")
}

fn norm_focus_key(value: &str) -> String {
    let mut text = value.split_whitespace().collect::<String>();
    let dash = text.find('-');
    let under = text.find('_');
    let cut = match (dash, under) {
        (Some(a), Some(b)) if a > 0 && b > 0 => Some(a.min(b)),
        (Some(a), _) if a > 0 => Some(a),
        (_, Some(b)) if b > 0 => Some(b),
        _ => None,
    };
    if let Some(index) = cut {
        text.truncate(index);
    }
    text
}

fn merge_first_ts(current: &str, next: &str) -> String {
    if current.is_empty() {
        return next.to_string();
    }
    if next.is_empty() {
        return current.to_string();
    }
    if current <= next {
        current.to_string()
    } else {
        next.to_string()
    }
}

fn merge_last_ts(current: &str, next: &str) -> String {
    if current.is_empty() {
        return next.to_string();
    }
    if next.is_empty() {
        return current.to_string();
    }
    if current >= next {
        current.to_string()
    } else {
        next.to_string()
    }
}

fn date_end_label(date_end_excl: &str) -> String {
    let text = date_end_excl.trim();
    if text.is_empty() {
        return String::new();
    }
    NaiveDate::parse_from_str(text, "%Y-%m-%d")
        .ok()
        .and_then(|date| date.checked_sub_days(Days::new(1)))
        .map(|date| date.format("%Y-%m-%d").to_string())
        .unwrap_or_default()
}
