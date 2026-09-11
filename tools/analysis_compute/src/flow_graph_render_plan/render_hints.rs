use serde_json::{json, Map, Value};

use super::value_helpers::{
    clamp_number, first_truthy_text, js_truthy, number_field, numeric_json, text_value,
};

pub(super) fn normalize_graph_tier(value: String, node_count: usize) -> String {
    let tier = value.trim().to_ascii_lowercase();
    if matches!(tier.as_str(), "small" | "medium" | "large" | "xlarge") {
        return tier;
    }
    if node_count <= 1000 {
        return "small".to_string();
    }
    if node_count <= 10000 {
        return "medium".to_string();
    }
    if node_count <= 100000 {
        return "large".to_string();
    }
    "xlarge".to_string()
}

pub(super) fn normalize_render_plan_hints(
    contract: &Value,
    ctx: &Value,
    hints: &Value,
    node_count: usize,
    edge_count: usize,
) -> Value {
    let ctx_hints = ctx.get("renderHints").unwrap_or(&Value::Null);
    let source = if hints.is_null() || hints.as_object().is_some_and(|row| row.is_empty()) {
        ctx_hints
    } else {
        hints
    };
    let view_mode = first_truthy_text(&[
        source.get("view_mode"),
        source.get("viewMode"),
        ctx.get("graphMode"),
        ctx.get("view"),
        ctx.get("viewMode"),
        ctx_hints.get("view_mode"),
        ctx_hints.get("viewMode"),
        contract.get("viewMode"),
        contract.get("view_mode"),
    ]);
    normalize_graph_render_hints(source, node_count, edge_count, &view_mode)
}

pub(super) fn normalize_graph_render_hints(
    source: &Value,
    node_count: usize,
    edge_count: usize,
    view_mode: &str,
) -> Value {
    let total_nodes = node_count;
    let total_edges = edge_count;
    let tier_source = first_truthy_text(&[
        source.get("tier"),
        source.get("graph_tier"),
        source.get("graphTier"),
    ]);
    let tier = if tier_source.is_empty() && !source.is_object() {
        normalize_graph_tier(text_value(Some(source)), total_nodes)
    } else {
        normalize_graph_tier(tier_source, total_nodes)
    };
    let defaults = build_default_graph_render_hints(&tier, total_nodes, total_edges, view_mode);
    let mut out = defaults.as_object().cloned().unwrap_or_default();
    if let Some(source_object) = source.as_object() {
        for (key, value) in source_object {
            out.insert(key.clone(), value.clone());
        }
    }
    out.insert("tier".to_string(), json!(tier));

    normalize_hint_bool_not_false(&mut out, "all_nodes_visible");
    normalize_hint_text(&mut out, &defaults, "default_node_mode", "entity");
    normalize_hint_text(&mut out, &defaults, "view_mode", "relation");
    normalize_hint_text(&mut out, &defaults, "edge_mode", "full");
    normalize_hint_text(&mut out, &defaults, "animation_mode", "light");
    normalize_hint_round_min(&mut out, &defaults, "entity_node_limit", 0.0, 0.0, false);
    normalize_hint_round_min(&mut out, &defaults, "focus_entity_limit", 0.0, 0.0, false);
    normalize_hint_round_min(&mut out, &defaults, "t0_label_limit", 0.0, 0.0, false);
    normalize_hint_round_min(
        &mut out,
        &defaults,
        "t1_reveal_batch_size",
        24.0,
        24.0,
        true,
    );
    normalize_hint_round_min(&mut out, &defaults, "t1_reveal_delay_ms", 0.0, 0.0, false);
    normalize_hint_round_min(&mut out, &defaults, "t1_batch_gap_ms", 8.0, 8.0, true);
    normalize_hint_bool_not_false(&mut out, "show_edge_labels");
    normalize_hint_bool_truthy(&mut out, "show_detail_edge_labels");
    normalize_hint_round_min(&mut out, &defaults, "edge_batch_threshold", 0.0, 0.0, false);
    normalize_hint_round_min(&mut out, &defaults, "edge_batch_size", 48.0, 48.0, true);
    normalize_hint_round_min(
        &mut out,
        &defaults,
        "edge_refresh_batch_size",
        96.0,
        96.0,
        true,
    );
    normalize_hint_round_min(
        &mut out,
        &defaults,
        "layout_switch_animate_max_nodes",
        0.0,
        0.0,
        false,
    );
    normalize_hint_round_min(
        &mut out,
        &defaults,
        "layout_switch_animate_max_edges",
        0.0,
        0.0,
        false,
    );
    normalize_hint_bool_not_false(&mut out, "projection_auto_expand");
    normalize_hint_round_min(
        &mut out,
        &defaults,
        "projection_auto_expand_delay_ms",
        120.0,
        120.0,
        true,
    );
    normalize_hint_round_min(
        &mut out,
        &defaults,
        "projection_auto_expand_cooldown_ms",
        300.0,
        300.0,
        true,
    );
    normalize_hint_clamped_number(
        &mut out,
        &defaults,
        "projection_auto_expand_min_zoom",
        0.05,
        4.0,
    );
    normalize_hint_round_min(
        &mut out,
        &defaults,
        "projection_auto_expand_max_clusters",
        1.0,
        1.0,
        true,
    );
    normalize_hint_round_min(
        &mut out,
        &defaults,
        "projection_auto_expand_max_nodes",
        1.0,
        1.0,
        true,
    );
    normalize_hint_round_min(
        &mut out,
        &defaults,
        "projection_viewport_materialize_limit",
        500.0,
        500.0,
        true,
    );
    normalize_hint_bool_not_false(&mut out, "prefer_fast_first_paint");

    Value::Object(out)
}

fn build_default_graph_render_hints(
    tier: &str,
    node_count: usize,
    edge_count: usize,
    view_mode: &str,
) -> Value {
    let total_nodes = node_count as f64;
    let total_edges = edge_count as f64;
    let normalized_tier = normalize_graph_tier(tier.to_string(), node_count);
    let normalized_view_mode = {
        let value = view_mode.trim().to_ascii_lowercase();
        if value.is_empty() {
            "relation".to_string()
        } else {
            value
        }
    };
    if normalized_tier == "small" {
        return json!({
            "tier": normalized_tier,
            "view_mode": normalized_view_mode,
            "all_nodes_visible": true,
            "default_node_mode": "entity",
            "entity_node_limit": numeric_json(total_nodes),
            "focus_entity_limit": numeric_json(total_nodes),
            "t0_label_limit": numeric_json(total_nodes),
            "t1_reveal_batch_size": numeric_json(48.0_f64.max((if total_nodes > 0.0 { total_nodes } else { 48.0 }).min(120.0))),
            "t1_reveal_delay_ms": 90,
            "t1_batch_gap_ms": 12,
            "show_edge_labels": total_edges <= 800.0,
            "show_detail_edge_labels": total_edges <= 480.0,
            "edge_mode": "full",
            "edge_batch_threshold": numeric_json(1500.0_f64.max(total_edges + 1.0)),
            "edge_batch_size": 80,
            "edge_refresh_batch_size": 180,
            "animation_mode": "full",
            "layout_switch_animate_max_nodes": numeric_json(total_nodes),
            "layout_switch_animate_max_edges": numeric_json(total_edges),
            "prefer_fast_first_paint": true,
            "projection_auto_expand": false,
        });
    }
    if normalized_tier == "medium" {
        return json!({
            "tier": normalized_tier,
            "view_mode": normalized_view_mode,
            "all_nodes_visible": true,
            "default_node_mode": "entity",
            "entity_node_limit": numeric_json(total_nodes),
            "focus_entity_limit": numeric_json(total_nodes.min(420.0)),
            "t0_label_limit": numeric_json(total_nodes.min(if total_nodes <= 6000.0 { 420.0 } else { 560.0 })),
            "t1_reveal_batch_size": if total_nodes <= 3000.0 { 180 } else { 240 },
            "t1_reveal_delay_ms": if total_nodes <= 3000.0 { 140 } else { 180 },
            "t1_batch_gap_ms": if total_nodes <= 3000.0 { 18 } else { 22 },
            "show_edge_labels": total_edges <= 2400.0,
            "show_detail_edge_labels": false,
            "edge_mode": if total_edges >= 1500.0 { "batched" } else { "full" },
            "edge_batch_threshold": 1500,
            "edge_batch_size": if total_edges < 6000.0 { 120 } else { 180 },
            "edge_refresh_batch_size": 240,
            "animation_mode": "light",
            "layout_switch_animate_max_nodes": 1200,
            "layout_switch_animate_max_edges": 3600,
            "prefer_fast_first_paint": true,
            "projection_auto_expand": false,
        });
    }
    if normalized_tier == "large" {
        return json!({
            "tier": normalized_tier,
            "view_mode": normalized_view_mode,
            "all_nodes_visible": true,
            "default_node_mode": "mixed",
            "entity_node_limit": numeric_json((1800.0_f64.max((total_nodes / 6.0).round())).min(4200.0)),
            "focus_entity_limit": numeric_json(total_nodes.min(720.0)),
            "t0_label_limit": numeric_json(total_nodes.min(240.0)),
            "t1_reveal_batch_size": 220,
            "t1_reveal_delay_ms": 180,
            "t1_batch_gap_ms": 24,
            "show_edge_labels": false,
            "show_detail_edge_labels": false,
            "edge_mode": "batched",
            "edge_batch_threshold": 900,
            "edge_batch_size": 180,
            "edge_refresh_batch_size": 320,
            "animation_mode": "minimal",
            "layout_switch_animate_max_nodes": 0,
            "layout_switch_animate_max_edges": 0,
            "prefer_fast_first_paint": true,
            "projection_auto_expand": false,
        });
    }
    json!({
        "tier": normalized_tier,
        "view_mode": normalized_view_mode,
        "all_nodes_visible": true,
        "default_node_mode": "mixed",
        "entity_node_limit": numeric_json((1200.0_f64.max((total_nodes / 10.0).round())).min(2200.0)),
        "focus_entity_limit": numeric_json(total_nodes.min(480.0)),
        "t0_label_limit": numeric_json(total_nodes.min(120.0)),
        "t1_reveal_batch_size": 160,
        "t1_reveal_delay_ms": 220,
        "t1_batch_gap_ms": 28,
        "show_edge_labels": false,
        "show_detail_edge_labels": false,
        "edge_mode": "batched",
        "edge_batch_threshold": 600,
        "edge_batch_size": 220,
        "edge_refresh_batch_size": 360,
        "animation_mode": "minimal",
        "layout_switch_animate_max_nodes": 0,
        "layout_switch_animate_max_edges": 0,
        "prefer_fast_first_paint": true,
        "projection_auto_expand": true,
        "projection_auto_expand_delay_ms": 280,
        "projection_auto_expand_cooldown_ms": 1200,
        "projection_auto_expand_min_zoom": 0.18,
        "projection_auto_expand_max_clusters": 24,
        "projection_auto_expand_max_nodes": 240,
        "projection_viewport_materialize_limit": 7500,
    })
}

fn value_or_default<'a>(
    out: &'a Map<String, Value>,
    defaults: &'a Value,
    key: &str,
) -> Option<&'a Value> {
    out.get(key)
        .filter(|value| !value.is_null())
        .or_else(|| defaults.get(key))
}

fn normalize_hint_bool_not_false(out: &mut Map<String, Value>, key: &str) {
    let next = !matches!(out.get(key), Some(Value::Bool(false)));
    out.insert(key.to_string(), json!(next));
}

fn normalize_hint_bool_truthy(out: &mut Map<String, Value>, key: &str) {
    let next = js_truthy(out.get(key));
    out.insert(key.to_string(), json!(next));
}

fn normalize_hint_text(out: &mut Map<String, Value>, defaults: &Value, key: &str, fallback: &str) {
    let source = out
        .get(key)
        .filter(|value| js_truthy(Some(*value)))
        .or_else(|| defaults.get(key).filter(|value| js_truthy(Some(*value))));
    let mut next = text_value(source).trim().to_ascii_lowercase();
    if next.is_empty() {
        next = fallback.to_string();
    }
    out.insert(key.to_string(), json!(next));
}

fn normalize_hint_round_min(
    out: &mut Map<String, Value>,
    defaults: &Value,
    key: &str,
    min: f64,
    fallback: f64,
    zero_uses_fallback: bool,
) {
    let mut value = value_or_default(out, defaults, key)
        .and_then(|value| number_field(Some(value)))
        .unwrap_or(fallback);
    if zero_uses_fallback && value == 0.0 {
        value = fallback;
    }
    out.insert(key.to_string(), numeric_json(min.max(value.round())));
}

fn normalize_hint_clamped_number(
    out: &mut Map<String, Value>,
    defaults: &Value,
    key: &str,
    min: f64,
    max: f64,
) {
    let value = value_or_default(out, defaults, key)
        .and_then(|value| number_field(Some(value)))
        .unwrap_or(min);
    out.insert(key.to_string(), numeric_json(clamp_number(value, min, max)));
}
