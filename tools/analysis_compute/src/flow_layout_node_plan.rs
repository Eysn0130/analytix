use anyhow::{anyhow, bail, Context, Result};
use icu_collator::{options::CollatorOptions, Collator};
use icu_locale_core::locale;
use serde_json::{json, Map, Number, Value};
use std::{fs, path::PathBuf};

const DEFAULT_LAYOUT_META_KEYS: &[&str] = &[
    "layoutBand",
    "role",
    "clusterId",
    "seedCoreCandidate",
    "isSeedSelected",
    "demoteReason",
    "isPathPromotedAdjacent",
    "isBridgeAdjacent",
    "isHubAdjacent",
    "layoutAnchorForCluster",
    "weight",
    "prevX",
    "prevY",
    "layoutCase",
    "layoutClusterId",
    "layoutRoleWhy",
    "layoutCore",
    "layoutFanRole",
    "layoutFanRadius",
    "layoutFanAmount",
    "layoutCorridor",
    "layoutCorridorLevel",
    "layoutCorridorT",
    "layoutCorridorOffset",
    "layoutTerritory",
    "layoutCorridorRank",
    "layoutBridge",
    "layoutCommunity",
    "layoutLevel",
    "layoutVisibilityTier",
    "layoutLabelTier",
    "layoutBridgeScore",
    "layoutExternalAngle",
];
const FALLBACK_GRID_MIN_NODES: usize = 64;
const FALLBACK_GRID_MIN_AXIS_COUNT: usize = 6;
const FALLBACK_GRID_MIN_STEP: f64 = 40.0;
const FALLBACK_GRID_MAX_STEP: f64 = 180.0;
const FALLBACK_GRID_NEAR_STEP_RATIO: f64 = 0.85;
const FALLBACK_GRID_MIN_FILL_RATIO: f64 = 0.72;

pub(crate) struct LayoutNodePlanArgs {
    pub(crate) payload: Value,
}

pub(crate) fn parse_args(mut iter: impl Iterator<Item = String>) -> Result<LayoutNodePlanArgs> {
    let mut payload: Option<Value> = None;

    while let Some(flag) = iter.next() {
        match flag.as_str() {
            "--payload-json" => {
                let raw_payload = crate::required_value(&mut iter, "--payload-json")?;
                payload = Some(
                    serde_json::from_str(&raw_payload)
                        .with_context(|| "parse --payload-json as JSON")?,
                );
            }
            "--input-path" => {
                let input_path = PathBuf::from(crate::required_value(&mut iter, "--input-path")?);
                let raw_payload = fs::read_to_string(&input_path).with_context(|| {
                    format!("read layout node plan input {}", input_path.display())
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!("parse layout node plan input {}", input_path.display())
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute <project-layout-node-plan|project-layout-cache-key|apply-layout-node-plan|apply-layout-worker-node-plan|clear-layout-node-meta> <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported layout node plan flag: {other}"),
        }
    }

    Ok(LayoutNodePlanArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_layout_node_plan_result(payload: &Value) -> Value {
    let meta_keys = payload_meta_keys(payload);
    let projected_nodes = project_layout_nodes(payload.get("nodes"), &meta_keys);
    let node_plan = build_layout_node_plan_index(projected_nodes.clone());
    let updates = if node_plan.is_null() {
        Vec::new()
    } else {
        build_projected_layout_node_updates(&projected_nodes)
    };
    json!({
        "nodePlan": node_plan,
        "updates": updates,
    })
}

pub(crate) fn project_layout_cache_key(payload: &Value) -> Value {
    let nodes = payload
        .get("nodes")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    let edges = payload
        .get("edges")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    let algo_version = text_field(payload, "algoVersion")
        .or_else(|| text_field(payload, "algo_version"))
        .unwrap_or_default();
    let mode = normalize_layout_preset(
        &text_field(payload, "mode")
            .or_else(|| text_field(payload, "preset"))
            .unwrap_or_default(),
    );
    let focus = text_field(payload, "focusId")
        .or_else(|| text_field(payload, "focus_id"))
        .unwrap_or_default()
        .trim()
        .to_string();
    let mutation_seq = number_field(payload, "graphMutationSeq")
        .or_else(|| number_field(payload, "mutationSeq"))
        .unwrap_or(0.0);
    let graph_signature = compute_layout_graph_signature(&nodes, &edges, &algo_version);
    let direction = layout_direction_token_by_mode(
        &mode,
        payload
            .get("layoutDirection")
            .or_else(|| payload.get("layout_direction")),
    );
    let seed_signature = compute_layout_seed_signature(
        payload
            .get("seedContext")
            .or_else(|| payload.get("seed_context")),
    );

    json!({
        "key": format!("{algo_version}|{mode}|{direction}|{focus}|{seed_signature}|{graph_signature}"),
        "mode": mode,
        "direction": direction,
        "seedSignature": seed_signature,
        "graphSignature": graph_signature,
        "graphSignatureCache": {
            "nodesLen": nodes.len(),
            "edgesLen": edges.len(),
            "mutationSeq": mutation_seq,
            "signature": graph_signature,
        },
        "graphSignatureReused": false,
    })
}

pub(crate) fn apply_layout_node_plan_payload(payload: &Value) -> Value {
    let meta_keys = payload_meta_keys(payload);
    let mut nodes = payload
        .get("nodes")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    let applied = match payload.get("nodesById") {
        Some(nodes_by_id) => apply_layout_node_plan(&mut nodes, nodes_by_id, &meta_keys),
        None => false,
    };
    json!({
        "applied": applied,
        "nodes": nodes,
    })
}

pub(crate) fn apply_layout_worker_node_plan_payload(payload: &Value) -> Value {
    let meta_keys = payload_meta_keys(payload);
    let mut nodes = payload
        .get("nodes")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    let grid_fallback_likely = is_likely_worker_fallback_grid(payload.get("workerNodes"));
    let worker_plan =
        build_layout_node_plan_index(project_layout_nodes(payload.get("workerNodes"), &meta_keys));
    let (applied, source) = if worker_plan.is_null() {
        (false, "invalid-plan")
    } else if apply_layout_node_plan(&mut nodes, &worker_plan, &meta_keys) {
        (true, "plan")
    } else {
        (false, "apply-failed")
    };
    let updates = if applied {
        build_layout_node_updates(&nodes, &meta_keys)
    } else {
        Vec::new()
    };
    json!({
        "applied": applied,
        "source": source,
        "matched": if applied { nodes.len() } else { 0 },
        "gridFallbackLikely": grid_fallback_likely,
        "updates": updates,
        "nodes": nodes,
    })
}

pub(crate) fn clear_layout_meta_payload(payload: &Value) -> Value {
    let meta_keys = payload_meta_keys(payload);
    let mut nodes = payload
        .get("nodes")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    clear_layout_meta(&mut nodes, &meta_keys);
    Value::Array(nodes)
}

fn payload_meta_keys(payload: &Value) -> Vec<String> {
    match payload.get("metaKeys") {
        None => DEFAULT_LAYOUT_META_KEYS
            .iter()
            .map(|key| (*key).to_string())
            .collect(),
        Some(Value::Array(keys)) => keys.iter().map(js_string).collect(),
        Some(_) => Vec::new(),
    }
}

fn project_layout_nodes(nodes: Option<&Value>, meta_keys: &[String]) -> Vec<Value> {
    let Some(Value::Array(rows)) = nodes else {
        return Vec::new();
    };

    rows.iter()
        .filter_map(|node| project_layout_node(node, meta_keys))
        .collect()
}

fn project_layout_node(node: &Value, meta_keys: &[String]) -> Option<Value> {
    let Value::Object(node_obj) = node else {
        return None;
    };

    let id = node_obj
        .get("id")
        .map(js_or_empty_string)
        .unwrap_or_default()
        .trim()
        .to_string();
    if id.is_empty() {
        return None;
    }

    let mut row = Map::new();
    row.insert("id".to_string(), Value::String(id));
    if let Some(x) = node_obj.get("x").and_then(js_number_value) {
        row.insert("x".to_string(), x);
    }
    if let Some(y) = node_obj.get("y").and_then(js_number_value) {
        row.insert("y".to_string(), y);
    }

    for key in meta_keys {
        let Some(value) = node_obj.get(key) else {
            continue;
        };
        if value.is_null() || value.as_str() == Some("") {
            continue;
        }
        row.insert(key.clone(), value.clone());
    }

    Some(Value::Object(row))
}

fn build_layout_node_plan_index(projected_nodes: Vec<Value>) -> Value {
    if projected_nodes.is_empty() {
        return Value::Null;
    }

    let mut index = Map::new();
    for row in projected_nodes {
        let Some(row_obj) = row.as_object() else {
            return Value::Null;
        };
        let Some(id) = row_obj.get("id").and_then(Value::as_str) else {
            return Value::Null;
        };
        if row_obj.get("x").and_then(js_number).is_none()
            || row_obj.get("y").and_then(js_number).is_none()
        {
            return Value::Null;
        }
        index.insert(id.to_string(), row);
    }

    Value::Object(index)
}

fn build_layout_node_updates(nodes: &[Value], meta_keys: &[String]) -> Vec<Value> {
    nodes
        .iter()
        .enumerate()
        .filter_map(|(index, node)| {
            let mut row = project_layout_node(node, meta_keys)?.as_object()?.clone();
            row.insert("index".to_string(), json!(index));
            Some(Value::Object(row))
        })
        .collect()
}

fn build_projected_layout_node_updates(projected_nodes: &[Value]) -> Vec<Value> {
    projected_nodes
        .iter()
        .enumerate()
        .filter_map(|(index, node)| {
            let mut row = node.as_object()?.clone();
            row.insert("index".to_string(), json!(index));
            Some(Value::Object(row))
        })
        .collect()
}

fn compute_layout_graph_signature(nodes: &[Value], edges: &[Value], algo_version: &str) -> String {
    let mut hash = FNV_OFFSET;
    hash = mix_fnv1a(hash, algo_version);
    for node in nodes {
        hash = mix_fnv1a(hash, &object_field_or_empty(node, "id"));
        hash = mix_fnv1a(hash, &object_field_or_empty(node, "ntype"));
        hash = mix_fnv1a(hash, &object_field_or_empty(node, "title"));
        hash = mix_fnv1a(hash, &object_field_or_empty(node, "name"));
        hash = mix_fnv1a(hash, &number_or_zero_string(object_field(node, "r")));
        hash = mix_fnv1a(hash, &number_or_zero_string(object_field(node, "fontSize")));
        hash = mix_fnv1a(
            hash,
            &number_or_zero_string(object_field(node, "total_amount")),
        );
        hash = mix_fnv1a(
            hash,
            &number_or_zero_string(object_field(node, "totalAmount")),
        );
        hash = mix_fnv1a(hash, &number_or_zero_string(object_field(node, "amount")));
        hash = mix_fnv1a(
            hash,
            &number_or_zero_string(object_field(node, "total_amt")),
        );
        hash = mix_fnv1a(hash, &number_or_zero_string(object_field(node, "totalAmt")));
        hash = mix_fnv1a(hash, &number_or_zero_string(object_field(node, "weight")));
    }
    for edge in edges {
        hash = mix_fnv1a(hash, &object_field_or_empty(edge, "source"));
        hash = mix_fnv1a(hash, &object_field_or_empty(edge, "target"));
        hash = mix_fnv1a(hash, &object_field_or_empty(edge, "mode"));
        hash = mix_fnv1a(hash, &object_field_or_empty(edge, "edgeArrow"));
        hash = mix_fnv1a(hash, &object_field_or_empty(edge, "arrow"));
        hash = mix_fnv1a(
            hash,
            if object_bool_truthy(edge, "showArrow") {
                "1"
            } else {
                "0"
            },
        );
        hash = mix_fnv1a(hash, &number_or_zero_string(object_field(edge, "amount")));
        hash = mix_fnv1a(
            hash,
            &number_or_zero_string(object_field(edge, "forward_amount")),
        );
        hash = mix_fnv1a(
            hash,
            &number_or_zero_string(object_field(edge, "reverse_amount")),
        );
        hash = mix_fnv1a(
            hash,
            &number_or_zero_string(object_field(edge, "out_amount")),
        );
        hash = mix_fnv1a(
            hash,
            &number_or_zero_string(object_field(edge, "in_amount")),
        );
    }
    format!("{}|{}|{:x}", nodes.len(), edges.len(), hash)
}

const FNV_OFFSET: u32 = 2166136261;
const FNV_PRIME: u32 = 16777619;

fn mix_fnv1a(hash: u32, input: &str) -> u32 {
    let mut next = hash;
    for unit in input.encode_utf16() {
        next ^= u32::from(unit);
        next = next.wrapping_mul(FNV_PRIME);
    }
    next
}

fn normalize_layout_preset(value: &str) -> String {
    let mode = value.trim().to_lowercase();
    if mode == "relation" {
        "compact".to_string()
    } else if mode.is_empty() {
        "compact".to_string()
    } else {
        mode
    }
}

fn layout_direction_token_by_mode(mode: &str, layout_direction: Option<&Value>) -> String {
    let source = layout_direction.and_then(Value::as_object);
    if mode == "hierarchy" {
        return if source
            .and_then(|row| row.get("hierarchy"))
            .map(js_or_empty_string)
            .unwrap_or_default()
            == "up"
        {
            "up"
        } else {
            "down"
        }
        .to_string();
    }
    if mode == "flow" {
        return if source
            .and_then(|row| row.get("flow"))
            .map(js_or_empty_string)
            .unwrap_or_default()
            == "left"
        {
            "left"
        } else {
            "right"
        }
        .to_string();
    }
    "na".to_string()
}

fn compute_layout_seed_signature(seed_context: Option<&Value>) -> String {
    let mut hash = FNV_OFFSET;
    for id in sorted_id_rows(seed_context.and_then(|ctx| ctx.get("selectedIds"))) {
        hash = mix_fnv1a(hash, &id);
    }
    for id in sorted_id_rows(seed_context.and_then(|ctx| ctx.get("leftSeeds"))) {
        hash = mix_fnv1a(hash, &id);
    }
    hash = mix_fnv1a(hash, &text_field_opt(seed_context, "focusKeyType"));
    hash = mix_fnv1a(hash, &text_field_opt(seed_context, "tab"));
    hash = mix_fnv1a(hash, &text_field_opt(seed_context, "source"));
    hash = mix_fnv1a(
        hash,
        if bool_field(seed_context, "focusUnknownName") {
            "1"
        } else {
            "0"
        },
    );
    hash = mix_fnv1a(
        hash,
        if bool_field(seed_context, "focusCounterpartyStrict") {
            "1"
        } else {
            "0"
        },
    );
    format!("{:x}", hash)
}

fn sorted_id_rows(value: Option<&Value>) -> Vec<String> {
    let Some(Value::Array(rows)) = value else {
        return Vec::new();
    };
    let mut out = rows
        .iter()
        .map(js_or_empty_string)
        .map(|id| id.trim().to_string())
        .filter(|id| !id.is_empty())
        .collect::<Vec<_>>();
    let collator = Collator::try_new(locale!("zh-CN").into(), CollatorOptions::default()).ok();
    out.sort_by(|left, right| match &collator {
        Some(collator) => collator.compare(left.as_str(), right.as_str()),
        None => left.cmp(right),
    });
    out
}

fn object_field<'a>(value: &'a Value, key: &str) -> Option<&'a Value> {
    value.as_object().and_then(|row| row.get(key))
}

fn object_field_or_empty(value: &Value, key: &str) -> String {
    object_field(value, key)
        .map(js_or_empty_string)
        .unwrap_or_default()
}

fn object_bool_truthy(value: &Value, key: &str) -> bool {
    object_field(value, key).map(js_truthy).unwrap_or(false)
}

fn number_or_zero_string(value: Option<&Value>) -> String {
    js_number(value.unwrap_or(&Value::Null))
        .map(format_js_number)
        .unwrap_or_else(|| "0".to_string())
}

fn format_js_number(value: f64) -> String {
    if value == 0.0 {
        return "0".to_string();
    }
    if value.fract() == 0.0 {
        format!("{}", value as i64)
    } else {
        let mut text = value.to_string();
        if text.contains('.') {
            while text.ends_with('0') {
                text.pop();
            }
            if text.ends_with('.') {
                text.pop();
            }
        }
        text
    }
}

fn text_field(payload: &Value, key: &str) -> Option<String> {
    payload.get(key).map(js_or_empty_string)
}

fn text_field_opt(payload: Option<&Value>, key: &str) -> String {
    payload
        .and_then(|row| row.get(key))
        .map(js_or_empty_string)
        .unwrap_or_default()
}

fn number_field(payload: &Value, key: &str) -> Option<f64> {
    payload.get(key).and_then(js_number)
}

fn bool_field(payload: Option<&Value>, key: &str) -> bool {
    payload
        .and_then(|row| row.get(key))
        .map(js_truthy)
        .unwrap_or(false)
}

fn js_truthy(value: &Value) -> bool {
    match value {
        Value::Null => false,
        Value::Bool(flag) => *flag,
        Value::Number(number) => number
            .as_f64()
            .map(|n| n != 0.0 && !n.is_nan())
            .unwrap_or(false),
        Value::String(text) => !text.is_empty(),
        Value::Array(_) | Value::Object(_) => true,
    }
}

fn is_likely_worker_fallback_grid(rows: Option<&Value>) -> bool {
    let Some(Value::Array(nodes)) = rows else {
        return false;
    };
    if nodes.len() < FALLBACK_GRID_MIN_NODES {
        return false;
    }

    let mut xs: Vec<f64> = Vec::new();
    let mut ys: Vec<f64> = Vec::new();
    for node in nodes {
        let Some(node_obj) = node.as_object() else {
            return false;
        };
        let Some(x) = node_obj.get("x").and_then(js_number) else {
            return false;
        };
        let Some(y) = node_obj.get("y").and_then(js_number) else {
            return false;
        };
        push_unique_sorted_value(&mut xs, round_two_decimals(x));
        push_unique_sorted_value(&mut ys, round_two_decimals(y));
    }
    if xs.len() < FALLBACK_GRID_MIN_AXIS_COUNT || ys.len() < FALLBACK_GRID_MIN_AXIS_COUNT {
        return false;
    }
    if !axis_has_grid_steps(&xs) || !axis_has_grid_steps(&ys) {
        return false;
    }
    let fill_ratio = nodes.len() as f64 / ((xs.len() * ys.len()).max(1) as f64);
    fill_ratio >= FALLBACK_GRID_MIN_FILL_RATIO
}

fn push_unique_sorted_value(values: &mut Vec<f64>, value: f64) {
    if values.iter().any(|existing| *existing == value) {
        return;
    }
    values.push(value);
}

fn round_two_decimals(value: f64) -> f64 {
    format!("{value:.2}").parse::<f64>().unwrap_or(value)
}

fn axis_has_grid_steps(values: &[f64]) -> bool {
    let mut sorted = values.to_vec();
    sorted.sort_by(|left, right| left.total_cmp(right));
    let mut deltas: Vec<f64> = Vec::new();
    for pair in sorted.windows(2) {
        let delta = pair[1] - pair[0];
        if delta.is_finite() && delta > 0.5 {
            deltas.push(delta);
        }
    }
    if deltas.len() < 2 {
        return false;
    }
    deltas.sort_by(|left, right| left.total_cmp(right));
    let median = deltas[deltas.len() / 2];
    if !median.is_finite() || !(FALLBACK_GRID_MIN_STEP..=FALLBACK_GRID_MAX_STEP).contains(&median) {
        return false;
    }
    let near_count = deltas
        .iter()
        .filter(|delta| {
            let delta = **delta;
            let multi = (delta / median).round().max(1.0);
            let nearest = median * multi;
            (delta - nearest).abs() <= 1.5_f64.max(median * 0.12)
        })
        .count();
    near_count as f64 / deltas.len() as f64 >= FALLBACK_GRID_NEAR_STEP_RATIO
}

fn apply_layout_node_plan(nodes: &mut [Value], nodes_by_id: &Value, meta_keys: &[String]) -> bool {
    let Some(by_id) = nodes_by_id.as_object() else {
        return false;
    };

    for node in nodes.iter() {
        let Some(node_obj) = node.as_object() else {
            return false;
        };
        let id = node_obj
            .get("id")
            .map(js_or_empty_string)
            .unwrap_or_default()
            .trim()
            .to_string();
        if id.is_empty() {
            return false;
        }
        let Some(cached) = by_id.get(&id).and_then(Value::as_object) else {
            return false;
        };
        if cached.get("x").and_then(js_number).is_none()
            || cached.get("y").and_then(js_number).is_none()
        {
            return false;
        }
    }

    for node in nodes.iter_mut() {
        let Some(node_obj) = node.as_object_mut() else {
            return false;
        };
        let id = node_obj
            .get("id")
            .map(js_or_empty_string)
            .unwrap_or_default()
            .trim()
            .to_string();
        let Some(cached) = by_id.get(&id).and_then(Value::as_object) else {
            return false;
        };
        if let Some(x) = cached.get("x").and_then(js_number_value) {
            node_obj.insert("x".to_string(), x);
        }
        if let Some(y) = cached.get("y").and_then(js_number_value) {
            node_obj.insert("y".to_string(), y);
        }
        for key in meta_keys {
            let value = cached.get(key);
            if value.is_none()
                || value == Some(&Value::Null)
                || value.and_then(Value::as_str) == Some("")
            {
                node_obj.remove(key);
                continue;
            }
            if let Some(value) = value {
                node_obj.insert(key.clone(), value.clone());
            }
        }
    }

    true
}

fn clear_layout_meta(nodes: &mut [Value], meta_keys: &[String]) {
    for node in nodes {
        let Some(node_obj) = node.as_object_mut() else {
            continue;
        };
        for key in meta_keys {
            node_obj.remove(key);
        }
    }
}

fn js_or_empty_string(value: &Value) -> String {
    match value {
        Value::Null => String::new(),
        Value::Bool(false) => String::new(),
        Value::Number(number) if number.as_f64() == Some(0.0) => String::new(),
        Value::String(text) if text.is_empty() => String::new(),
        _ => js_string(value),
    }
}

fn js_string(value: &Value) -> String {
    match value {
        Value::Null => String::new(),
        Value::Bool(flag) => flag.to_string(),
        Value::Number(number) => number.to_string(),
        Value::String(text) => text.clone(),
        Value::Array(values) => values.iter().map(js_string).collect::<Vec<_>>().join(","),
        Value::Object(_) => "[object Object]".to_string(),
    }
}

fn js_number_value(value: &Value) -> Option<Value> {
    js_number(value).and_then(|number| Number::from_f64(number).map(Value::Number))
}

fn js_number(value: &Value) -> Option<f64> {
    let number = match value {
        Value::Null => 0.0,
        Value::Bool(flag) => {
            if *flag {
                1.0
            } else {
                0.0
            }
        }
        Value::Number(number) => number.as_f64()?,
        Value::String(text) => {
            let trimmed = text.trim();
            if trimmed.is_empty() {
                0.0
            } else {
                trimmed.parse::<f64>().ok()?
            }
        }
        Value::Array(values) if values.is_empty() => 0.0,
        Value::Array(values) if values.len() == 1 => js_number(&values[0])?,
        Value::Array(_) | Value::Object(_) => return None,
    };

    if number.is_finite() {
        Some(number)
    } else {
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn project_layout_node_plan_indexes_valid_projected_nodes() {
        let payload = json!({
            "nodes": [
                {"id": " n1 ", "x": "10", "y": 20, "layoutBand": "core", "layoutRole": ""},
                {"id": "n2", "x": 30, "y": "40", "layoutBand": "leaf", "layoutRole": null}
            ],
            "metaKeys": ["layoutBand", "layoutRole"]
        });

        assert_eq!(
            project_layout_node_plan_result(&payload)["nodePlan"],
            json!({
                "n1": {"id": "n1", "x": 10.0, "y": 20.0, "layoutBand": "core"},
                "n2": {"id": "n2", "x": 30.0, "y": 40.0, "layoutBand": "leaf"}
            })
        );
    }

    #[test]
    fn project_layout_node_plan_result_returns_cache_ready_updates() {
        let payload = json!({
            "nodes": [
                {"id": " n1 ", "x": "10", "y": 20, "layoutBand": "core", "layoutRole": ""},
                {"id": "n2", "x": 30, "y": "40", "layoutBand": "leaf"}
            ],
            "metaKeys": ["layoutBand", "layoutRole"]
        });

        assert_eq!(
            project_layout_node_plan_result(&payload),
            json!({
                "nodePlan": {
                    "n1": {"id": "n1", "x": 10.0, "y": 20.0, "layoutBand": "core"},
                    "n2": {"id": "n2", "x": 30.0, "y": 40.0, "layoutBand": "leaf"}
                },
                "updates": [
                    {"index": 0, "id": "n1", "x": 10.0, "y": 20.0, "layoutBand": "core"},
                    {"index": 1, "id": "n2", "x": 30.0, "y": 40.0, "layoutBand": "leaf"}
                ]
            })
        );
    }

    #[test]
    fn project_layout_cache_key_matches_js_signature_contract() {
        let payload = json!({
            "algoVersion": "algo-v1",
            "mode": "relation",
            "focusId": "acct-1",
            "graphMutationSeq": 2,
            "layoutDirection": {"hierarchy": "up", "flow": "left"},
            "seedContext": {
                "selectedIds": ["acct-2", "acct-1"],
                "leftSeeds": ["seed-b", "seed-a"],
                "focusKeyType": "account",
                "tab": "relations",
                "source": "stats",
                "focusUnknownName": true,
                "focusCounterpartyStrict": false
            },
            "nodes": [
                {"id": "acct-2", "ntype": "account", "title": "B", "name": "户B", "r": 18, "fontSize": 13, "total_amount": 200, "weight": 2},
                {"id": "acct-1", "ntype": "account", "title": "A", "name": "户A", "r": 20, "fontSize": 14, "amount": 100, "totalAmt": 1000, "weight": 3}
            ],
            "edges": [
                {"source": "acct-1", "target": "acct-2", "mode": "double", "edgeArrow": "both", "showArrow": true, "amount": 300, "forward_amount": 200, "reverse_amount": 100, "out_amount": 50, "in_amount": 20}
            ]
        });

        assert_eq!(
            project_layout_cache_key(&payload),
            json!({
                "key": "algo-v1|compact|na|acct-1|3fb1428d|2|1|4ca0d875",
                "mode": "compact",
                "direction": "na",
                "seedSignature": "3fb1428d",
                "graphSignature": "2|1|4ca0d875",
                "graphSignatureCache": {
                    "nodesLen": 2,
                    "edgesLen": 1,
                    "mutationSeq": 2.0,
                    "signature": "2|1|4ca0d875"
                },
                "graphSignatureReused": false
            })
        );
    }

    #[test]
    fn project_layout_node_plan_uses_default_meta_keys_when_omitted() {
        let payload = json!({
            "nodes": [
                {"id": "n1", "x": 1, "y": 2, "layoutBand": "core", "layoutRole": "ignored"}
            ]
        });

        assert_eq!(
            project_layout_node_plan_result(&payload)["nodePlan"],
            json!({
                "n1": {"id": "n1", "x": 1.0, "y": 2.0, "layoutBand": "core"}
            })
        );
    }

    #[test]
    fn project_layout_node_plan_returns_null_when_position_is_invalid() {
        let payload = json!({
            "nodes": [
                {"id": "bad", "x": "x", "y": 1, "layoutBand": "core"}
            ],
            "metaKeys": ["layoutBand"]
        });

        assert_eq!(
            project_layout_node_plan_result(&payload)["nodePlan"],
            Value::Null
        );
    }

    #[test]
    fn project_layout_node_plan_returns_null_when_no_nodes_are_projected() {
        assert_eq!(
            project_layout_node_plan_result(&json!({"nodes": [{"id": "", "x": 1, "y": 2}]}))
                ["nodePlan"],
            Value::Null
        );
        assert_eq!(
            project_layout_node_plan_result(&json!({"nodes": null}))["nodePlan"],
            Value::Null
        );
    }

    #[test]
    fn apply_layout_node_plan_payload_applies_full_valid_plan() {
        let payload = json!({
            "nodes": [
                {"id": "n1", "x": 0, "y": 0, "layoutBand": "old", "layoutRole": "old"},
                {"id": "n2", "x": 5, "y": 6, "layoutBand": "old"}
            ],
            "nodesById": {
                "n1": {"id": "n1", "x": "10", "y": 20, "layoutBand": "core", "layoutRole": ""},
                "n2": {"id": "n2", "x": 30, "y": "40", "layoutBand": "leaf"}
            },
            "metaKeys": ["layoutBand", "layoutRole"]
        });

        assert_eq!(
            apply_layout_node_plan_payload(&payload),
            json!({
                "applied": true,
                "nodes": [
                    {"id": "n1", "x": 10.0, "y": 20.0, "layoutBand": "core"},
                    {"id": "n2", "x": 30.0, "y": 40.0, "layoutBand": "leaf"}
                ]
            })
        );
    }

    #[test]
    fn apply_layout_worker_node_plan_payload_projects_and_applies_in_one_pass() {
        let payload = json!({
            "nodes": [
                {"id": "n1", "x": 0, "y": 0, "layoutBand": "old", "layoutRole": "old"},
                {"id": "n2", "x": 5, "y": 6, "layoutBand": "old"}
            ],
            "workerNodes": [
                {"id": "n1", "x": "10", "y": 20, "layoutBand": "core", "layoutRole": ""},
                {"id": "n2", "x": 30, "y": "40", "layoutBand": "leaf"}
            ],
            "metaKeys": ["layoutBand", "layoutRole"]
        });

        assert_eq!(
            apply_layout_worker_node_plan_payload(&payload),
            json!({
                "applied": true,
                "source": "plan",
                "matched": 2,
                "gridFallbackLikely": false,
                "updates": [
                    {"index": 0, "id": "n1", "x": 10.0, "y": 20.0, "layoutBand": "core"},
                    {"index": 1, "id": "n2", "x": 30.0, "y": 40.0, "layoutBand": "leaf"}
                ],
                "nodes": [
                    {"id": "n1", "x": 10.0, "y": 20.0, "layoutBand": "core"},
                    {"id": "n2", "x": 30.0, "y": 40.0, "layoutBand": "leaf"}
                ]
            })
        );
    }

    #[test]
    fn apply_layout_worker_node_plan_payload_detects_worker_fallback_grid() {
        let mut nodes = Vec::new();
        let mut worker_nodes = Vec::new();
        for y in 0..8 {
            for x in 0..8 {
                let id = format!("n-{x}-{y}");
                nodes.push(json!({"id": id, "x": 0, "y": 0}));
                worker_nodes.push(json!({"id": format!("n-{x}-{y}"), "x": x * 120, "y": y * 120}));
            }
        }
        let payload = json!({
            "nodes": nodes,
            "workerNodes": worker_nodes,
        });

        let result = apply_layout_worker_node_plan_payload(&payload);
        assert_eq!(result.get("applied").and_then(Value::as_bool), Some(true));
        assert_eq!(
            result.get("gridFallbackLikely").and_then(Value::as_bool),
            Some(true)
        );
        assert_eq!(result.get("matched").and_then(Value::as_u64), Some(64));
    }

    #[test]
    fn apply_layout_worker_node_plan_payload_rejects_invalid_worker_plan_without_mutation() {
        let payload = json!({
            "nodes": [
                {"id": "n1", "x": 0, "y": 0, "layoutBand": "old"},
                {"id": "n2", "x": 5, "y": 6, "layoutBand": "old"}
            ],
            "workerNodes": [
                {"id": "n1", "x": "10", "y": 20, "layoutBand": "core"},
                {"id": "n2", "x": "bad", "y": 40, "layoutBand": "leaf"}
            ],
            "metaKeys": ["layoutBand"]
        });

        assert_eq!(
            apply_layout_worker_node_plan_payload(&payload),
            json!({
                "applied": false,
                "source": "invalid-plan",
                "matched": 0,
                "gridFallbackLikely": false,
                "updates": [],
                "nodes": [
                    {"id": "n1", "x": 0, "y": 0, "layoutBand": "old"},
                    {"id": "n2", "x": 5, "y": 6, "layoutBand": "old"}
                ]
            })
        );
    }

    #[test]
    fn apply_layout_worker_node_plan_payload_rejects_missing_worker_ids_without_mutation() {
        let payload = json!({
            "nodes": [
                {"id": "n1", "x": 0, "y": 0, "layoutBand": "old"},
                {"id": "n2", "x": 5, "y": 6, "layoutBand": "old"}
            ],
            "workerNodes": [
                {"id": "n1", "x": "10", "y": 20, "layoutBand": "core"}
            ],
            "metaKeys": ["layoutBand"]
        });

        assert_eq!(
            apply_layout_worker_node_plan_payload(&payload),
            json!({
                "applied": false,
                "source": "apply-failed",
                "matched": 0,
                "gridFallbackLikely": false,
                "updates": [],
                "nodes": [
                    {"id": "n1", "x": 0, "y": 0, "layoutBand": "old"},
                    {"id": "n2", "x": 5, "y": 6, "layoutBand": "old"}
                ]
            })
        );
    }

    #[test]
    fn apply_layout_worker_node_plan_payload_removes_missing_or_empty_meta() {
        let payload = json!({
            "nodes": [
                {"id": "n1", "x": 0, "y": 0, "layoutBand": "old", "role": "old"},
                {"id": "n2", "x": 5, "y": 6, "layoutBand": "old", "role": "old"}
            ],
            "workerNodes": [
                {"id": "n1", "x": "10", "y": 20, "layoutBand": null, "role": ""},
                {"id": "n2", "x": 30, "y": "40", "role": "leaf"}
            ],
            "metaKeys": ["layoutBand", "role"]
        });

        assert_eq!(
            apply_layout_worker_node_plan_payload(&payload),
            json!({
                "applied": true,
                "source": "plan",
                "matched": 2,
                "gridFallbackLikely": false,
                "updates": [
                    {"index": 0, "id": "n1", "x": 10.0, "y": 20.0},
                    {"index": 1, "id": "n2", "x": 30.0, "y": 40.0, "role": "leaf"}
                ],
                "nodes": [
                    {"id": "n1", "x": 10.0, "y": 20.0},
                    {"id": "n2", "x": 30.0, "y": 40.0, "role": "leaf"}
                ]
            })
        );
    }

    #[test]
    fn apply_layout_node_plan_payload_rejects_partial_or_invalid_plans_without_mutation() {
        let payload = json!({
            "nodes": [
                {"id": "n1", "x": 0, "y": 0},
                {"id": "n2", "x": 5, "y": 6}
            ],
            "nodesById": {
                "n1": {"id": "n1", "x": 10, "y": 20},
                "n2": {"id": "n2", "x": "bad", "y": 40}
            },
            "metaKeys": ["layoutBand"]
        });

        assert_eq!(
            apply_layout_node_plan_payload(&payload),
            json!({
                "applied": false,
                "nodes": [
                    {"id": "n1", "x": 0, "y": 0},
                    {"id": "n2", "x": 5, "y": 6}
                ]
            })
        );
    }

    #[test]
    fn clear_layout_meta_payload_removes_only_layout_meta_keys() {
        let payload = json!({
            "nodes": [
                {"id": "n1", "x": 1, "y": 2, "layoutBand": "core", "layoutRole": "kept"},
                {"id": "n2", "x": 3, "y": 4, "layoutBand": "leaf"}
            ],
            "metaKeys": ["layoutBand"]
        });

        assert_eq!(
            clear_layout_meta_payload(&payload),
            json!([
                {"id": "n1", "x": 1, "y": 2, "layoutRole": "kept"},
                {"id": "n2", "x": 3, "y": 4}
            ])
        );
    }
}
