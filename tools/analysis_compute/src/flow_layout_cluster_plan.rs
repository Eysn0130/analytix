use crate::flow_layout_network_plan::project_network_cluster_plan;
use serde_json::{json, Map, Value};
use std::cmp::Ordering;
use std::collections::{BTreeMap, BTreeSet, HashMap};
use std::f64::consts::PI;

pub(crate) fn project_cluster_layout_plan(
    adjacency: &HashMap<String, BTreeSet<String>>,
    role_result: &mut Value,
    cluster_result: &mut Value,
    payload: &Value,
) -> Value {
    let mode = normalize_layout_slot_mode(
        payload
            .get("mode")
            .map(js_or_empty_string)
            .unwrap_or_default()
            .as_str(),
    );
    let config = payload
        .get("config")
        .or_else(|| payload.get("options"))
        .unwrap_or(payload);
    let prev_anchor_by_signature = read_string_object(payload.get("prevAnchorBySignature"));
    let next_anchor_by_signature = assign_cluster_layout_anchors(
        adjacency,
        role_result,
        cluster_result,
        &prev_anchor_by_signature,
    );
    apply_anchor_leaf_entries(role_result, cluster_result);
    classify_cluster_layout_cases(cluster_result, &mode);
    let margin = read_number(payload.get("margin"))
        .unwrap_or_else(|| if mode == "network" { 136.0 } else { 110.0 })
        .max(0.0);
    let placement_plan =
        build_cluster_placement_plan(cluster_result, payload, config, &mode, margin);
    let mut out = placement_plan.as_object().cloned().unwrap_or_default();
    if mode == "network" {
        if let Some(cluster_placement_by_id) = out.get("clusterPlacementById") {
            let node_by_id = payload.get("nodeRowsById").and_then(Value::as_object);
            let network_plan = project_network_cluster_plan(
                cluster_result,
                cluster_placement_by_id,
                config,
                node_by_id,
            );
            if let Some(network_obj) = network_plan.as_object() {
                for (key, value) in network_obj {
                    out.insert(key.clone(), value.clone());
                }
            }
        }
    }
    out.insert(
        "nextAnchorBySignature".to_string(),
        string_map_json(next_anchor_by_signature),
    );
    Value::Object(out)
}

fn assign_cluster_layout_anchors(
    adjacency: &HashMap<String, BTreeSet<String>>,
    role_result: &mut Value,
    cluster_result: &mut Value,
    prev_anchor_by_signature: &BTreeMap<String, String>,
) -> BTreeMap<String, String> {
    let mut next_anchor_by_signature = BTreeMap::new();
    let Some(clusters) = cluster_result
        .get_mut("clusters")
        .and_then(Value::as_array_mut)
    else {
        return next_anchor_by_signature;
    };
    let mut anchor_meta_ids = Vec::new();
    for cluster in clusters {
        let Some(cluster_obj) = cluster.as_object_mut() else {
            continue;
        };
        cluster_obj.insert("layoutAnchorIds".to_string(), Value::Array(Vec::new()));
        if !read_string_vec(cluster_obj.get("coreIds")).is_empty() {
            continue;
        }
        let signature = cluster_signature_from_obj(cluster_obj);
        let nodes = read_string_vec(cluster_obj.get("nodeIds"));
        if nodes.is_empty() {
            continue;
        }
        let prev_anchor = prev_anchor_by_signature
            .get(&signature)
            .map(|value| value.trim().to_string())
            .filter(|value| !value.is_empty() && nodes.contains(value));
        let anchor = prev_anchor.unwrap_or_else(|| {
            let mut ranked = nodes.clone();
            ranked.sort_by(|a, b| {
                let da = adjacency.get(a).map(BTreeSet::len).unwrap_or_default();
                let db = adjacency.get(b).map(BTreeSet::len).unwrap_or_default();
                match db.cmp(&da) {
                    Ordering::Equal => {}
                    ordering => return ordering,
                }
                let wa = read_node_meta_weight(role_result, a);
                let wb = read_node_meta_weight(role_result, b);
                match wb.partial_cmp(&wa).unwrap_or(Ordering::Equal) {
                    Ordering::Equal => a.cmp(b),
                    ordering => ordering,
                }
            });
            ranked.first().cloned().unwrap_or_default()
        });
        if anchor.is_empty() {
            continue;
        }
        cluster_obj.insert(
            "layoutAnchorIds".to_string(),
            Value::Array(vec![Value::String(anchor.clone())]),
        );
        next_anchor_by_signature.insert(signature, anchor.clone());
        set_meta_bool(role_result, &anchor, "layoutAnchorForCluster", true);
        anchor_meta_ids.push(anchor);
    }
    for anchor in anchor_meta_ids {
        set_meta_bool(cluster_result, &anchor, "layoutAnchorForCluster", true);
    }
    next_anchor_by_signature
}

fn apply_anchor_leaf_entries(role_result: &mut Value, cluster_result: &Value) {
    let clusters = cluster_result
        .get("clusters")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    for cluster in clusters {
        let core_count = read_string_vec(cluster.get("coreIds")).len();
        let anchor_id = read_string_vec(cluster.get("layoutAnchorIds"))
            .first()
            .cloned()
            .unwrap_or_default();
        if core_count > 0 || anchor_id.is_empty() {
            continue;
        }
        let skeleton_set = read_string_vec(cluster.get("coreIds"))
            .into_iter()
            .chain(read_string_vec(cluster.get("adjacentIds")))
            .chain(read_string_vec(cluster.get("layoutAnchorIds")))
            .collect::<BTreeSet<_>>();
        for leaf_id in read_string_vec(cluster.get("leafIds")) {
            let current_entry = read_leaf_entry(role_result, &leaf_id);
            if !current_entry.is_empty() && skeleton_set.contains(&current_entry) {
                continue;
            }
            set_string_map_entry(role_result, "leafEntryById", &leaf_id, &anchor_id);
            push_meta_why_once(role_result, &leaf_id, "leaf_single_entry");
        }
    }
}

fn classify_cluster_layout_cases(cluster_result: &mut Value, mode: &str) {
    let Some(clusters) = cluster_result
        .get_mut("clusters")
        .and_then(Value::as_array_mut)
    else {
        return;
    };
    for cluster in clusters {
        let layout_case = classify_cluster_layout_case(cluster, mode);
        if let Some(obj) = cluster.as_object_mut() {
            obj.insert("layoutCase".to_string(), Value::String(layout_case));
        }
    }
}

fn classify_cluster_layout_case(cluster: &Value, mode: &str) -> String {
    let node_ids = read_string_vec(cluster.get("nodeIds"));
    let node_count = node_ids.len();
    if node_count <= 1 {
        return "single".to_string();
    }
    if node_count == 2 {
        return "pair-horizontal".to_string();
    }
    if node_count == 3 {
        let edge_count = node_ids
            .iter()
            .map(|id| {
                read_string_vec(cluster.get("localAdjacency").and_then(|row| row.get(id))).len()
            })
            .sum::<usize>() as f64
            / 2.0;
        if edge_count.round() >= 3.0 {
            return "triple-triangle".to_string();
        }
        return "triple-chain".to_string();
    }
    let core_count = read_string_vec(cluster.get("coreIds")).len();
    let adjacent_count = read_string_vec(cluster.get("adjacentIds")).len();
    let leaf_count = read_string_vec(cluster.get("leafIds")).len();
    let anchor_count = read_string_vec(cluster.get("layoutAnchorIds")).len();
    if node_count == 4 && core_count == 0 {
        return "quad-square".to_string();
    }
    let mostly_leaf = leaf_count
        >= 1usize.max(node_count.saturating_sub(1usize.max(core_count + adjacent_count)));
    if (core_count == 1 && leaf_count >= 1) || (core_count == 0 && anchor_count == 1 && mostly_leaf)
    {
        if mode == "network" {
            "single-center-sector".to_string()
        } else {
            "single-center-circle".to_string()
        }
    } else {
        "general".to_string()
    }
}

#[derive(Clone)]
struct PlacementRow {
    cluster_id: String,
    signature: String,
    slot: Slot,
    prev_center: Option<(f64, f64)>,
}

#[derive(Clone, Copy)]
struct Slot {
    width: f64,
    height: f64,
}

fn build_cluster_placement_plan(
    cluster_result: &Value,
    payload: &Value,
    config: &Value,
    mode: &str,
    margin: f64,
) -> Value {
    let prev_center_by_signature = payload.get("prevCenterBySignature");
    let slot_by_signature = payload.get("slotBySignature");
    let clusters = cluster_result
        .get("clusters")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    let mut rows = clusters
        .iter()
        .filter_map(|cluster| {
            let cluster_id = cluster
                .get("clusterId")
                .map(js_or_empty_string)
                .unwrap_or_default()
                .trim()
                .to_string();
            if cluster_id.is_empty() {
                return None;
            }
            let signature = cluster_signature(cluster);
            let slot = read_cluster_slot_from_cache(slot_by_signature, &signature, mode)
                .unwrap_or_else(|| estimate_cluster_slot(cluster, mode, config));
            Some(PlacementRow {
                cluster_id,
                signature: signature.clone(),
                slot,
                prev_center: read_prev_center(prev_center_by_signature, &signature),
            })
        })
        .collect::<Vec<_>>();
    rows.sort_by(compare_placement_row);
    let count = rows.len().max(1);
    let columns = (count as f64).sqrt().ceil().max(1.0) as usize;
    let row_buckets = rows
        .chunks(columns)
        .map(|chunk| chunk.to_vec())
        .collect::<Vec<_>>();
    let row_heights = row_buckets
        .iter()
        .map(|bucket| {
            bucket
                .iter()
                .fold(0.0_f64, |acc, row| acc.max(row.slot.height))
        })
        .collect::<Vec<_>>();
    let row_widths = row_buckets
        .iter()
        .map(|bucket| {
            if bucket.is_empty() {
                0.0
            } else {
                bucket.iter().map(|row| row.slot.width).sum::<f64>()
                    + bucket.len().saturating_sub(1) as f64 * margin
            }
        })
        .collect::<Vec<_>>();
    let total_height =
        row_heights.iter().sum::<f64>() + row_buckets.len().saturating_sub(1) as f64 * margin;
    let mut cursor_y = -total_height / 2.0;
    let mut cluster_placement_by_id = Map::new();
    let mut next_center_by_signature = Map::new();
    for (row_index, bucket) in row_buckets.iter().enumerate() {
        let row_height = row_heights.get(row_index).copied().unwrap_or(0.0);
        let row_width = row_widths.get(row_index).copied().unwrap_or(0.0);
        let mut cursor_x = -row_width / 2.0;
        for row in bucket {
            let cx = cursor_x + row.slot.width / 2.0;
            let cy = cursor_y + row_height / 2.0;
            let placement = json!({
                "x": number_json(round_two(cx)),
                "y": number_json(round_two(cy)),
                "width": number_json(row.slot.width),
                "height": number_json(row.slot.height),
            });
            cluster_placement_by_id.insert(row.cluster_id.clone(), placement.clone());
            next_center_by_signature.insert(row.signature.clone(), placement);
            cursor_x += row.slot.width + margin;
        }
        cursor_y += row_height + margin;
    }
    json!({
        "clusterPlacementById": cluster_placement_by_id,
        "nextCenterBySignature": next_center_by_signature,
        "margin": number_json(margin),
        "columns": columns,
    })
}

fn compare_placement_row(a: &PlacementRow, b: &PlacementRow) -> Ordering {
    match (a.prev_center, b.prev_center) {
        (Some((ax, ay)), Some((bx, by))) => {
            if (ay - by).abs() > 1e-6 {
                return ay.partial_cmp(&by).unwrap_or(Ordering::Equal);
            }
            if (ax - bx).abs() > 1e-6 {
                return ax.partial_cmp(&bx).unwrap_or(Ordering::Equal);
            }
        }
        (Some(_), None) => return Ordering::Less,
        (None, Some(_)) => return Ordering::Greater,
        (None, None) => {}
    }
    a.cluster_id.cmp(&b.cluster_id)
}

fn estimate_cluster_slot(cluster: &Value, mode: &str, config: &Value) -> Slot {
    let node_count = read_string_vec(cluster.get("nodeIds")).len();
    let leaf_count = read_string_vec(cluster.get("leafIds")).len();
    let core_count = read_string_vec(cluster.get("coreIds")).len();
    let adjacent_count = read_string_vec(cluster.get("adjacentIds")).len();
    let layout_case = cluster
        .get("layoutCase")
        .map(js_or_empty_string)
        .unwrap_or_else(|| "general".to_string());
    let base_leaf = (read_number(config.get("R_LEAF_MIN")).unwrap_or(0.0) + 34.0).max(200.0);
    let (mut width, mut height) = match layout_case.as_str() {
        "single" => (140.0, 120.0),
        "pair-horizontal" => (250.0, 140.0),
        "triple-chain" => (300.0, 168.0),
        "triple-triangle" => (220.0, 220.0),
        "quad-square" => {
            let side = base_leaf.mul_add(0.98, 0.0).clamp(220.0, 360.0);
            (side + 48.0, side + 38.0)
        }
        "single-center-circle" | "single-center-sector" => {
            let ring_level = ((leaf_count as f64) / 9.0).ceil().max(1.0);
            let mut radius = base_leaf + ring_level * if mode == "network" { 64.0 } else { 52.0 };
            if mode == "network" && adjacent_count > 0 {
                let adjacent_gap = (base_leaf * 0.9).clamp(184.0, 340.0);
                let angular_need = if adjacent_count > 1 {
                    let slot_angle = (PI / adjacent_count as f64).sin().max(0.08);
                    adjacent_gap / (2.0 * slot_angle)
                } else {
                    base_leaf + 72.0
                };
                let owner_count = adjacent_count + usize::from(core_count > 0);
                let owner_leaf_level = ((leaf_count as f64) / owner_count.max(1) as f64 / 8.0)
                    .ceil()
                    .max(1.0);
                let sector_depth = base_leaf + owner_leaf_level * 72.0;
                radius = radius
                    .max(angular_need + sector_depth)
                    .max(base_leaf + (leaf_count.max(1) as f64).sqrt() * 38.0);
            }
            (radius * 2.0 + 96.0, radius * 2.0 + 72.0)
        }
        _ => {
            let backbone_scale =
                1usize.max(core_count + ((adjacent_count as f64) / 2.0).ceil() as usize);
            let density = 1.0f64.max((node_count as f64).sqrt().ceil());
            (
                base_leaf + backbone_scale as f64 * 62.0 + density * 42.0,
                base_leaf + backbone_scale as f64 * 54.0 + density * 34.0,
            )
        }
    };
    width = width.round();
    height = height.round();
    let min_w = if mode == "network" { 240.0 } else { 210.0 };
    let min_h = if mode == "network" { 210.0 } else { 190.0 };
    Slot {
        width: width.max(min_w),
        height: height.max(min_h),
    }
}

fn read_cluster_slot_from_cache(
    cache: Option<&Value>,
    signature: &str,
    mode: &str,
) -> Option<Slot> {
    let key = format!("{}|{}", normalize_layout_slot_mode(mode), signature.trim());
    if signature.trim().is_empty() {
        return None;
    }
    normalize_cluster_slot_value(cache.and_then(|rows| rows.get(&key)), mode)
}

fn normalize_cluster_slot_value(slot: Option<&Value>, mode: &str) -> Option<Slot> {
    let width = read_number(slot.and_then(|row| row.get("width")))?;
    let height = read_number(slot.and_then(|row| row.get("height")))?;
    if !width.is_finite() || !height.is_finite() || width <= 0.0 || height <= 0.0 {
        return None;
    }
    let kind = normalize_layout_slot_mode(mode);
    let min_w = if kind == "network" { 240.0 } else { 210.0 };
    let min_h = if kind == "network" { 210.0 } else { 190.0 };
    Some(Slot {
        width: width.round().max(min_w),
        height: height.round().max(min_h),
    })
}

fn read_prev_center(value: Option<&Value>, signature: &str) -> Option<(f64, f64)> {
    let row = value.and_then(|rows| rows.get(signature))?;
    if !row.is_object() {
        return None;
    }
    Some((
        read_number(row.get("x")).unwrap_or(0.0),
        read_number(row.get("y")).unwrap_or(0.0),
    ))
}

fn read_node_meta_weight(role_result: &Value, id: &str) -> f64 {
    role_result
        .get("nodeMetaById")
        .and_then(|rows| rows.get(id))
        .and_then(|row| read_number(row.get("weight")))
        .unwrap_or(0.0)
}

fn read_leaf_entry(role_result: &Value, id: &str) -> String {
    role_result
        .get("leafEntryById")
        .and_then(|rows| rows.get(id))
        .map(js_or_empty_string)
        .unwrap_or_default()
        .trim()
        .to_string()
}

fn set_string_map_entry(root: &mut Value, map_key: &str, key: &str, value: &str) {
    let Some(obj) = root.as_object_mut() else {
        return;
    };
    let map = obj
        .entry(map_key.to_string())
        .or_insert_with(|| Value::Object(Map::new()));
    if !map.is_object() {
        *map = Value::Object(Map::new());
    }
    if let Some(rows) = map.as_object_mut() {
        rows.insert(key.to_string(), Value::String(value.to_string()));
    }
}

fn set_meta_bool(root: &mut Value, id: &str, key: &str, value: bool) {
    if let Some(meta) = ensure_meta_object(root, id) {
        meta.insert(key.to_string(), Value::Bool(value));
    }
}

fn push_meta_why_once(root: &mut Value, id: &str, reason: &str) {
    let Some(meta) = ensure_meta_object(root, id) else {
        return;
    };
    let why = meta
        .entry("why".to_string())
        .or_insert_with(|| Value::Array(Vec::new()));
    if !why.is_array() {
        *why = Value::Array(Vec::new());
    }
    let Some(items) = why.as_array_mut() else {
        return;
    };
    if !items.iter().any(|item| item.as_str() == Some(reason)) {
        items.push(Value::String(reason.to_string()));
    }
}

fn ensure_meta_object<'a>(root: &'a mut Value, id: &str) -> Option<&'a mut Map<String, Value>> {
    let obj = root.as_object_mut()?;
    let rows = obj
        .entry("nodeMetaById".to_string())
        .or_insert_with(|| Value::Object(Map::new()));
    if !rows.is_object() {
        *rows = Value::Object(Map::new());
    }
    let row_map = rows.as_object_mut()?;
    let row = row_map
        .entry(id.to_string())
        .or_insert_with(|| json!({"id": id, "why": []}));
    if !row.is_object() {
        *row = json!({"id": id, "why": []});
    }
    let meta = row.as_object_mut()?;
    meta.entry("id".to_string())
        .or_insert_with(|| Value::String(id.to_string()));
    meta.entry("why".to_string())
        .or_insert_with(|| Value::Array(Vec::new()));
    Some(meta)
}

fn cluster_signature(cluster: &Value) -> String {
    read_string_vec(cluster.get("nodeIds")).join("|")
}

fn cluster_signature_from_obj(cluster: &Map<String, Value>) -> String {
    read_string_vec(cluster.get("nodeIds")).join("|")
}

fn read_string_object(value: Option<&Value>) -> BTreeMap<String, String> {
    let mut out = BTreeMap::new();
    let Some(Value::Object(rows)) = value else {
        return out;
    };
    for (key, row) in rows {
        let id = key.trim().to_string();
        if id.is_empty() {
            continue;
        }
        out.insert(id, js_or_empty_string(row).trim().to_string());
    }
    out
}

fn string_map_json(rows: BTreeMap<String, String>) -> Value {
    let mut out = Map::new();
    for (key, value) in rows {
        out.insert(key, Value::String(value));
    }
    Value::Object(out)
}

fn read_string_vec(value: Option<&Value>) -> Vec<String> {
    let mut out = value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .map(js_or_empty_string)
                .map(|item| item.trim().to_string())
                .filter(|item| !item.is_empty())
                .collect::<Vec<_>>()
        })
        .unwrap_or_default();
    out.sort();
    out
}

fn normalize_layout_slot_mode(mode: &str) -> String {
    match mode.trim().to_lowercase().as_str() {
        "network" => "network".to_string(),
        _ => "compact".to_string(),
    }
}

fn read_number(value: Option<&Value>) -> Option<f64> {
    let number = match value? {
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
        Value::Array(values) if values.len() == 1 => read_number(values.first())?,
        Value::Array(_) | Value::Object(_) => return None,
    };
    if number.is_finite() {
        Some(number)
    } else {
        None
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

fn round_two(value: f64) -> f64 {
    (value * 100.0).round() / 100.0
}

fn number_json(value: f64) -> Value {
    if value.is_finite()
        && value.fract() == 0.0
        && value >= i64::MIN as f64
        && value <= i64::MAX as f64
    {
        json!(value as i64)
    } else {
        json!(value)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn projects_anchor_case_and_placement() {
        let mut adjacency = HashMap::new();
        adjacency.insert("a".to_string(), BTreeSet::from(["b".to_string()]));
        adjacency.insert(
            "b".to_string(),
            BTreeSet::from(["a".to_string(), "c".to_string()]),
        );
        adjacency.insert("c".to_string(), BTreeSet::from(["b".to_string()]));
        adjacency.insert("x".to_string(), BTreeSet::from(["y".to_string()]));
        adjacency.insert("y".to_string(), BTreeSet::from(["x".to_string()]));
        let mut role_result = json!({
            "rolesById": {"a": "leaf", "b": "adjacent", "c": "leaf", "x": "leaf", "y": "leaf"},
            "nodeMetaById": {
                "a": {"id": "a", "role": "leaf", "why": []},
                "b": {"id": "b", "role": "adjacent", "why": [], "weight": 0},
                "c": {"id": "c", "role": "leaf", "why": []},
                "x": {"id": "x", "role": "leaf", "why": []},
                "y": {"id": "y", "role": "leaf", "why": []}
            },
            "leafEntryById": {}
        });
        let mut cluster_result = json!({
            "clusters": [
                {
                    "clusterId": "cluster-0-0",
                    "nodeIds": ["a", "b", "c"],
                    "coreIds": [],
                    "adjacentIds": ["b"],
                    "leafIds": ["a", "c"],
                    "layoutAnchorIds": [],
                    "localAdjacency": {"a": ["b"], "b": ["a", "c"], "c": ["b"]}
                },
                {
                    "clusterId": "cluster-1-0",
                    "nodeIds": ["x", "y"],
                    "coreIds": [],
                    "adjacentIds": [],
                    "leafIds": ["x", "y"],
                    "layoutAnchorIds": [],
                    "localAdjacency": {"x": ["y"], "y": ["x"]}
                }
            ],
            "nodeClusterById": {"a": "cluster-0-0", "b": "cluster-0-0", "c": "cluster-0-0", "x": "cluster-1-0", "y": "cluster-1-0"},
            "nodeMetaById": {}
        });
        let plan = project_cluster_layout_plan(
            &adjacency,
            &mut role_result,
            &mut cluster_result,
            &json!({
                "mode": "compact",
                "margin": 20,
                "slotBySignature": {
                    "compact|a|b|c": {"width": 300, "height": 200}
                },
                "prevCenterBySignature": {
                    "a|b|c": {"x": 30, "y": 10},
                    "x|y": {"x": -20, "y": 10}
                },
                "config": {"R_LEAF_MIN": 180}
            }),
        );
        assert_eq!(
            cluster_result["clusters"][0]["layoutAnchorIds"],
            json!(["b"])
        );
        assert_eq!(
            cluster_result["clusters"][0]["layoutCase"],
            json!("triple-chain")
        );
        assert_eq!(
            cluster_result["clusters"][1]["layoutAnchorIds"],
            json!(["x"])
        );
        assert_eq!(
            cluster_result["clusters"][1]["layoutCase"],
            json!("pair-horizontal")
        );
        assert_eq!(
            role_result["nodeMetaById"]["b"]["layoutAnchorForCluster"],
            json!(true)
        );
        assert_eq!(
            role_result["leafEntryById"],
            json!({"a": "b", "c": "b", "x": "x", "y": "x"})
        );
        assert_eq!(
            plan,
            json!({
                "clusterPlacementById": {
                    "cluster-1-0": {"x": -160, "y": 0, "width": 250, "height": 190},
                    "cluster-0-0": {"x": 135, "y": 0, "width": 300, "height": 200}
                },
                "nextCenterBySignature": {
                    "x|y": {"x": -160, "y": 0, "width": 250, "height": 190},
                    "a|b|c": {"x": 135, "y": 0, "width": 300, "height": 200}
                },
                "margin": 20,
                "columns": 2,
                "nextAnchorBySignature": {"a|b|c": "b", "x|y": "x"}
            })
        );
    }

    #[test]
    fn network_plan_projects_single_center_sector_updates() {
        let mut adjacency = HashMap::new();
        adjacency.insert(
            "anchor".to_string(),
            BTreeSet::from([
                "leaf-1".to_string(),
                "leaf-2".to_string(),
                "leaf-3".to_string(),
                "leaf-4".to_string(),
            ]),
        );
        for leaf in ["leaf-1", "leaf-2", "leaf-3", "leaf-4"] {
            adjacency.insert(leaf.to_string(), BTreeSet::from(["anchor".to_string()]));
        }
        let mut role_result = json!({
            "rolesById": {
                "anchor": "leaf",
                "leaf-1": "leaf",
                "leaf-2": "leaf",
                "leaf-3": "leaf",
                "leaf-4": "leaf"
            },
            "nodeMetaById": {
                "anchor": {"id": "anchor", "role": "leaf", "why": [], "weight": 10},
                "leaf-1": {"id": "leaf-1", "role": "leaf", "why": []},
                "leaf-2": {"id": "leaf-2", "role": "leaf", "why": []},
                "leaf-3": {"id": "leaf-3", "role": "leaf", "why": []},
                "leaf-4": {"id": "leaf-4", "role": "leaf", "why": []}
            },
            "leafEntryById": {}
        });
        let mut cluster_result = json!({
            "clusters": [{
                "clusterId": "cluster-sector",
                "nodeIds": ["anchor", "leaf-1", "leaf-2", "leaf-3", "leaf-4"],
                "coreIds": [],
                "adjacentIds": [],
                "leafIds": ["anchor", "leaf-1", "leaf-2", "leaf-3", "leaf-4"],
                "layoutAnchorIds": [],
                "localAdjacency": {
                    "anchor": ["leaf-1", "leaf-2", "leaf-3", "leaf-4"],
                    "leaf-1": ["anchor"],
                    "leaf-2": ["anchor"],
                    "leaf-3": ["anchor"],
                    "leaf-4": ["anchor"]
                }
            }],
            "nodeClusterById": {
                "anchor": "cluster-sector",
                "leaf-1": "cluster-sector",
                "leaf-2": "cluster-sector",
                "leaf-3": "cluster-sector",
                "leaf-4": "cluster-sector"
            },
            "nodeMetaById": {}
        });

        let plan = project_cluster_layout_plan(
            &adjacency,
            &mut role_result,
            &mut cluster_result,
            &json!({
                "mode": "network",
                "margin": 20,
                "slotBySignature": {
                    "network|anchor|leaf-1|leaf-2|leaf-3|leaf-4": {"width": 600, "height": 500}
                },
                "config": {"R_LEAF_MIN": 180, "NETWORK_SECTOR_LAYER_GAP": 64}
            }),
        );

        assert_eq!(
            cluster_result["clusters"][0]["layoutCase"],
            json!("single-center-sector")
        );
        assert_eq!(
            plan["networkNodeUpdateSummary"],
            json!({
                "clusterCount": 1,
                "nodeCount": 5,
                "coveredCases": {"single-center-sector": 1}
            })
        );
        let updates = plan["networkNodeUpdatesByClusterId"]["cluster-sector"]
            .as_array()
            .expect("network updates");
        assert_eq!(updates.len(), 5);
        assert_eq!(updates[0]["id"], json!("anchor"));
        assert_eq!(updates[0]["x"], json!(0));
        assert_eq!(updates[0]["y"], json!(0));
        assert_eq!(
            plan["networkCenterReportsByClusterId"]["cluster-sector"][0],
            json!({
                "centerId": "anchor",
                "centerType": "layoutAnchor",
                "x": 0,
                "y": 0,
                "leafCount": 4,
                "layerCount": 1,
                "sectorCount": 1,
                "sectorRanges": [{"start": -3.142, "end": 3.142, "score": null}],
                "multiSectorUsed": false,
                "leafMode": "rust-sector",
                "sectorFastPath": "rust-sector-ring"
            })
        );
        assert!(updates
            .iter()
            .skip(1)
            .all(|row| row.get("x").and_then(Value::as_f64).is_some()
                || row.get("x").and_then(Value::as_i64).is_some()));
        assert!(updates
            .iter()
            .skip(1)
            .all(|row| row.get("y").and_then(Value::as_f64).is_some()
                || row.get("y").and_then(Value::as_i64).is_some()));
    }

    #[test]
    fn network_plan_projects_core_leaf_sector_updates() {
        let mut adjacency = HashMap::new();
        adjacency.insert(
            "core".to_string(),
            BTreeSet::from([
                "leaf-1".to_string(),
                "leaf-2".to_string(),
                "leaf-3".to_string(),
            ]),
        );
        for leaf in ["leaf-1", "leaf-2", "leaf-3"] {
            adjacency.insert(leaf.to_string(), BTreeSet::from(["core".to_string()]));
        }
        let mut role_result = json!({
            "rolesById": {
                "core": "core",
                "leaf-1": "leaf",
                "leaf-2": "leaf",
                "leaf-3": "leaf"
            },
            "nodeMetaById": {
                "core": {"id": "core", "role": "core", "why": [], "weight": 10},
                "leaf-1": {"id": "leaf-1", "role": "leaf", "why": []},
                "leaf-2": {"id": "leaf-2", "role": "leaf", "why": []},
                "leaf-3": {"id": "leaf-3", "role": "leaf", "why": []}
            },
            "leafEntryById": {
                "leaf-1": "core",
                "leaf-2": "core",
                "leaf-3": "core"
            }
        });
        let mut cluster_result = json!({
            "clusters": [{
                "clusterId": "cluster-core",
                "nodeIds": ["core", "leaf-1", "leaf-2", "leaf-3"],
                "coreIds": ["core"],
                "adjacentIds": [],
                "leafIds": ["leaf-1", "leaf-2", "leaf-3"],
                "layoutAnchorIds": [],
                "localAdjacency": {
                    "core": ["leaf-1", "leaf-2", "leaf-3"],
                    "leaf-1": ["core"],
                    "leaf-2": ["core"],
                    "leaf-3": ["core"]
                }
            }],
            "nodeClusterById": {
                "core": "cluster-core",
                "leaf-1": "cluster-core",
                "leaf-2": "cluster-core",
                "leaf-3": "cluster-core"
            },
            "nodeMetaById": {}
        });

        let plan = project_cluster_layout_plan(
            &adjacency,
            &mut role_result,
            &mut cluster_result,
            &json!({
                "mode": "network",
                "margin": 20,
                "slotBySignature": {
                    "network|core|leaf-1|leaf-2|leaf-3": {"width": 500, "height": 420}
                },
                "config": {"R_LEAF_MIN": 180, "NETWORK_SECTOR_LAYER_GAP": 64}
            }),
        );

        assert_eq!(
            cluster_result["clusters"][0]["layoutCase"],
            json!("single-center-sector")
        );
        let updates = plan["networkNodeUpdatesByClusterId"]["cluster-core"]
            .as_array()
            .expect("network updates");
        assert_eq!(
            updates
                .iter()
                .map(|row| row["id"].clone())
                .collect::<Vec<_>>(),
            vec![
                json!("core"),
                json!("leaf-1"),
                json!("leaf-2"),
                json!("leaf-3")
            ]
        );
        assert_eq!(updates[0]["layoutBand"], json!("center"));
        assert_eq!(
            plan["networkCenterReportsByClusterId"]["cluster-core"][0]["centerType"],
            json!("core")
        );
        assert_eq!(
            plan["networkCenterReportsByClusterId"]["cluster-core"][0]["sectorRanges"],
            json!([{"start": -3.142, "end": 3.142, "score": null}])
        );
        assert!(updates
            .iter()
            .skip(1)
            .all(|row| row["layoutBand"] == json!("leaf")));
    }

    #[test]
    fn network_plan_projects_large_sector_layer_summary() {
        let leaf_ids = (1..=120)
            .map(|index| format!("leaf-{index:03}"))
            .collect::<Vec<_>>();
        let mut node_ids = vec!["anchor".to_string()];
        node_ids.extend(leaf_ids.clone());
        let mut adjacency = HashMap::new();
        adjacency.insert(
            "anchor".to_string(),
            leaf_ids.iter().cloned().collect::<BTreeSet<_>>(),
        );
        for leaf_id in &leaf_ids {
            adjacency.insert(leaf_id.clone(), BTreeSet::from(["anchor".to_string()]));
        }
        let mut local_adjacency = Map::new();
        local_adjacency.insert(
            "anchor".to_string(),
            Value::Array(leaf_ids.iter().cloned().map(Value::String).collect()),
        );
        for leaf_id in &leaf_ids {
            local_adjacency.insert(
                leaf_id.clone(),
                Value::Array(vec![Value::String("anchor".to_string())]),
            );
        }
        let mut role_result = json!({
            "rolesById": {},
            "nodeMetaById": {},
            "leafEntryById": {}
        });
        let signature = format!("network|{}", node_ids.join("|"));
        let mut slot_by_signature = Map::new();
        slot_by_signature.insert(signature, json!({"width": 1600, "height": 1400}));
        let mut cluster_result = json!({
            "clusters": [{
                "clusterId": "cluster-large-sector",
                "nodeIds": node_ids,
                "coreIds": [],
                "adjacentIds": [],
                "leafIds": leaf_ids,
                "layoutAnchorIds": [],
                "localAdjacency": Value::Object(local_adjacency)
            }],
            "nodeClusterById": {},
            "nodeMetaById": {}
        });

        let plan = project_cluster_layout_plan(
            &adjacency,
            &mut role_result,
            &mut cluster_result,
            &json!({
                "mode": "network",
                "margin": 20,
                "slotBySignature": Value::Object(slot_by_signature),
                "config": {"R_LEAF_MIN": 180, "NETWORK_SECTOR_LAYER_GAP": 64}
            }),
        );

        let updates = plan["networkNodeUpdatesByClusterId"]["cluster-large-sector"]
            .as_array()
            .expect("network updates");
        assert_eq!(updates.len(), 121);
        assert_eq!(
            plan["networkCenterReportsByClusterId"]["cluster-large-sector"][0]["layerCount"],
            json!(4)
        );
        let max_layer = updates
            .iter()
            .filter_map(|row| row.get("layoutLayer").and_then(Value::as_u64))
            .max()
            .unwrap_or_default();
        assert_eq!(max_layer, 3);
        assert_eq!(
            plan["networkCenterReportsByClusterId"]["cluster-large-sector"][0]["sectorFastPath"],
            json!("rust-sector-ring")
        );
    }

    #[test]
    fn network_plan_projects_core_adjacent_sector_updates() {
        let leaf_ids = (1..=5)
            .map(|index| format!("leaf-{index}"))
            .collect::<Vec<_>>();
        let mut adjacency = HashMap::new();
        adjacency.insert("core".to_string(), BTreeSet::from(["adjacent".to_string()]));
        let mut adjacent_neighbors = BTreeSet::from(["core".to_string()]);
        for leaf_id in &leaf_ids {
            adjacent_neighbors.insert(leaf_id.clone());
            adjacency.insert(leaf_id.clone(), BTreeSet::from(["adjacent".to_string()]));
        }
        adjacency.insert("adjacent".to_string(), adjacent_neighbors);
        let mut local_adjacency = Map::new();
        local_adjacency.insert(
            "core".to_string(),
            Value::Array(vec![Value::String("adjacent".to_string())]),
        );
        local_adjacency.insert(
            "adjacent".to_string(),
            Value::Array(
                std::iter::once(Value::String("core".to_string()))
                    .chain(leaf_ids.iter().cloned().map(Value::String))
                    .collect(),
            ),
        );
        for leaf_id in &leaf_ids {
            local_adjacency.insert(
                leaf_id.clone(),
                Value::Array(vec![Value::String("adjacent".to_string())]),
            );
        }
        let mut role_result = json!({
            "rolesById": {
                "core": "core",
                "adjacent": "adjacent",
                "leaf-1": "leaf",
                "leaf-2": "leaf",
                "leaf-3": "leaf",
                "leaf-4": "leaf",
                "leaf-5": "leaf"
            },
            "nodeMetaById": {},
            "leafEntryById": {
                "leaf-1": "adjacent",
                "leaf-2": "adjacent",
                "leaf-3": "adjacent",
                "leaf-4": "adjacent",
                "leaf-5": "adjacent"
            }
        });
        let mut cluster_result = json!({
            "clusters": [{
                "clusterId": "cluster-adjacent",
                "nodeIds": ["core", "adjacent", "leaf-1", "leaf-2", "leaf-3", "leaf-4", "leaf-5"],
                "coreIds": ["core"],
                "adjacentIds": ["adjacent"],
                "leafIds": ["leaf-1", "leaf-2", "leaf-3", "leaf-4", "leaf-5"],
                "layoutAnchorIds": [],
                "localAdjacency": Value::Object(local_adjacency)
            }],
            "nodeClusterById": {
                "core": "cluster-adjacent",
                "adjacent": "cluster-adjacent",
                "leaf-1": "cluster-adjacent",
                "leaf-2": "cluster-adjacent",
                "leaf-3": "cluster-adjacent",
                "leaf-4": "cluster-adjacent",
                "leaf-5": "cluster-adjacent"
            },
            "nodeMetaById": {}
        });

        let plan = project_cluster_layout_plan(
            &adjacency,
            &mut role_result,
            &mut cluster_result,
            &json!({
                "mode": "network",
                "margin": 20,
                "slotBySignature": {
                    "network|adjacent|core|leaf-1|leaf-2|leaf-3|leaf-4|leaf-5": {"width": 620, "height": 520}
                },
                "config": {
                    "R_LEAF_MIN": 180,
                    "NETWORK_ADJ_SPACING": 118,
                    "NETWORK_SECTOR_LAYER_GAP": 64
                }
            }),
        );

        assert_eq!(
            plan["networkNodeUpdateSummary"],
            json!({
                "clusterCount": 1,
                "nodeCount": 7,
                "coveredCases": {"single-center-sector": 1}
            })
        );
        let updates = plan["networkNodeUpdatesByClusterId"]["cluster-adjacent"]
            .as_array()
            .expect("network updates");
        assert_eq!(updates.len(), 7);
        assert_eq!(updates[0]["layoutBand"], json!("core"));
        assert_eq!(updates[1]["layoutBand"], json!("adjacent"));
        assert!(updates
            .iter()
            .skip(2)
            .all(|row| row["layoutBand"] == json!("leaf")));
        assert_eq!(
            plan["networkCenterReportsByClusterId"]["cluster-adjacent"][0]["centerId"],
            json!("adjacent")
        );
        assert_eq!(
            plan["networkCenterReportsByClusterId"]["cluster-adjacent"][0]["centerType"],
            json!("adjacent")
        );
        assert_eq!(
            plan["networkCenterReportsByClusterId"]["cluster-adjacent"][0]["leafCount"],
            json!(5)
        );
        assert_eq!(
            plan["networkCenterReportsByClusterId"]["cluster-adjacent"][0]["leafMode"],
            json!("rust-sector")
        );
    }

    #[test]
    fn network_plan_projects_multi_adjacent_sector_updates() {
        let mut adjacency = HashMap::new();
        adjacency.insert(
            "core".to_string(),
            BTreeSet::from(["adj-a".to_string(), "adj-b".to_string()]),
        );
        adjacency.insert(
            "adj-a".to_string(),
            BTreeSet::from([
                "core".to_string(),
                "leaf-a1".to_string(),
                "leaf-a2".to_string(),
            ]),
        );
        adjacency.insert(
            "adj-b".to_string(),
            BTreeSet::from([
                "core".to_string(),
                "leaf-b1".to_string(),
                "leaf-b2".to_string(),
            ]),
        );
        adjacency.insert("leaf-a1".to_string(), BTreeSet::from(["adj-a".to_string()]));
        adjacency.insert("leaf-a2".to_string(), BTreeSet::from(["adj-a".to_string()]));
        adjacency.insert("leaf-b1".to_string(), BTreeSet::from(["adj-b".to_string()]));
        adjacency.insert("leaf-b2".to_string(), BTreeSet::from(["adj-b".to_string()]));
        let mut role_result = json!({
            "rolesById": {
                "core": "core",
                "adj-a": "adjacent",
                "adj-b": "adjacent",
                "leaf-a1": "leaf",
                "leaf-a2": "leaf",
                "leaf-b1": "leaf",
                "leaf-b2": "leaf"
            },
            "nodeMetaById": {},
            "leafEntryById": {
                "leaf-a1": "adj-a",
                "leaf-a2": "adj-a",
                "leaf-b1": "adj-b",
                "leaf-b2": "adj-b"
            }
        });
        let mut cluster_result = json!({
            "clusters": [{
                "clusterId": "cluster-multi-adjacent",
                "nodeIds": ["core", "adj-a", "adj-b", "leaf-a1", "leaf-a2", "leaf-b1", "leaf-b2"],
                "coreIds": ["core"],
                "adjacentIds": ["adj-a", "adj-b"],
                "leafIds": ["leaf-a1", "leaf-a2", "leaf-b1", "leaf-b2"],
                "layoutAnchorIds": [],
                "localAdjacency": {
                    "core": ["adj-a", "adj-b"],
                    "adj-a": ["core", "leaf-a1", "leaf-a2"],
                    "adj-b": ["core", "leaf-b1", "leaf-b2"],
                    "leaf-a1": ["adj-a"],
                    "leaf-a2": ["adj-a"],
                    "leaf-b1": ["adj-b"],
                    "leaf-b2": ["adj-b"]
                }
            }],
            "nodeClusterById": {
                "core": "cluster-multi-adjacent",
                "adj-a": "cluster-multi-adjacent",
                "adj-b": "cluster-multi-adjacent",
                "leaf-a1": "cluster-multi-adjacent",
                "leaf-a2": "cluster-multi-adjacent",
                "leaf-b1": "cluster-multi-adjacent",
                "leaf-b2": "cluster-multi-adjacent"
            },
            "nodeMetaById": {}
        });

        let plan = project_cluster_layout_plan(
            &adjacency,
            &mut role_result,
            &mut cluster_result,
            &json!({
                "mode": "network",
                "margin": 20,
                "slotBySignature": {
                    "network|adj-a|adj-b|core|leaf-a1|leaf-a2|leaf-b1|leaf-b2": {"width": 680, "height": 560}
                },
                "config": {
                    "R_LEAF_MIN": 180,
                    "NETWORK_ADJ_SPACING": 118,
                    "NETWORK_SECTOR_LAYER_GAP": 64
                }
            }),
        );

        assert_eq!(
            cluster_result["clusters"][0]["layoutCase"],
            json!("single-center-sector")
        );
        assert_eq!(
            plan["networkNodeUpdateSummary"],
            json!({
                "clusterCount": 1,
                "nodeCount": 7,
                "coveredCases": {"single-center-sector": 1}
            })
        );
        let updates = plan["networkNodeUpdatesByClusterId"]["cluster-multi-adjacent"]
            .as_array()
            .expect("network updates");
        assert_eq!(
            updates
                .iter()
                .map(|row| row["id"].clone())
                .collect::<Vec<_>>(),
            vec![
                json!("core"),
                json!("adj-a"),
                json!("leaf-a1"),
                json!("leaf-a2"),
                json!("adj-b"),
                json!("leaf-b1"),
                json!("leaf-b2")
            ]
        );
        let owner_rows = updates
            .iter()
            .filter(|row| row["layoutBand"] == json!("leaf"))
            .map(|row| (row["id"].clone(), row["ownerCenterId"].clone()))
            .collect::<Vec<_>>();
        assert_eq!(
            owner_rows,
            vec![
                (json!("leaf-a1"), json!("adj-a")),
                (json!("leaf-a2"), json!("adj-a")),
                (json!("leaf-b1"), json!("adj-b")),
                (json!("leaf-b2"), json!("adj-b"))
            ]
        );
        let center_reports = plan["networkCenterReportsByClusterId"]["cluster-multi-adjacent"]
            .as_array()
            .expect("center reports");
        assert_eq!(center_reports.len(), 2);
        assert_eq!(center_reports[0]["centerId"], json!("adj-a"));
        assert_eq!(center_reports[0]["leafCount"], json!(2));
        assert_eq!(center_reports[1]["centerId"], json!("adj-b"));
        assert_eq!(center_reports[1]["leafCount"], json!(2));
    }

    #[test]
    fn network_plan_spreads_dense_adjacent_sector_centers() {
        let adjacent_ids = (1..=12)
            .map(|index| format!("adj-{index:02}"))
            .collect::<Vec<_>>();
        let leaf_ids = (1..=12)
            .map(|index| format!("leaf-{index:02}"))
            .collect::<Vec<_>>();
        let mut node_ids = vec!["core".to_string()];
        node_ids.extend(adjacent_ids.clone());
        node_ids.extend(leaf_ids.clone());

        let mut adjacency = HashMap::new();
        adjacency.insert(
            "core".to_string(),
            adjacent_ids.iter().cloned().collect::<BTreeSet<_>>(),
        );
        let mut local_adjacency = Map::new();
        local_adjacency.insert(
            "core".to_string(),
            Value::Array(adjacent_ids.iter().cloned().map(Value::String).collect()),
        );
        for (adjacent_id, leaf_id) in adjacent_ids.iter().zip(leaf_ids.iter()) {
            adjacency.insert(
                adjacent_id.clone(),
                BTreeSet::from(["core".to_string(), leaf_id.clone()]),
            );
            adjacency.insert(leaf_id.clone(), BTreeSet::from([adjacent_id.clone()]));
            local_adjacency.insert(
                adjacent_id.clone(),
                Value::Array(vec![
                    Value::String("core".to_string()),
                    Value::String(leaf_id.clone()),
                ]),
            );
            local_adjacency.insert(
                leaf_id.clone(),
                Value::Array(vec![Value::String(adjacent_id.clone())]),
            );
        }

        let mut roles_by_id = Map::new();
        roles_by_id.insert("core".to_string(), json!("core"));
        for adjacent_id in &adjacent_ids {
            roles_by_id.insert(adjacent_id.clone(), json!("adjacent"));
        }
        for leaf_id in &leaf_ids {
            roles_by_id.insert(leaf_id.clone(), json!("leaf"));
        }
        let mut leaf_entry_by_id = Map::new();
        for (adjacent_id, leaf_id) in adjacent_ids.iter().zip(leaf_ids.iter()) {
            leaf_entry_by_id.insert(leaf_id.clone(), json!(adjacent_id));
        }
        let mut node_rows_by_id = Map::new();
        node_rows_by_id.insert(
            "core".to_string(),
            json!({"id": "core", "title": "核心账户"}),
        );
        for adjacent_id in &adjacent_ids {
            node_rows_by_id.insert(
                adjacent_id.clone(),
                json!({
                    "id": adjacent_id,
                    "title": format!("贵州省密集邻接中心测试账户{adjacent_id}"),
                    "display_id": format!("2402018311990000{adjacent_id}")
                }),
            );
        }
        for leaf_id in &leaf_ids {
            node_rows_by_id.insert(
                leaf_id.clone(),
                json!({"id": leaf_id, "title": format!("叶子账户{leaf_id}")}),
            );
        }

        let mut role_result = json!({
            "rolesById": Value::Object(roles_by_id),
            "nodeMetaById": {},
            "leafEntryById": Value::Object(leaf_entry_by_id)
        });
        let mut cluster_result = json!({
            "clusters": [{
                "clusterId": "cluster-dense-adjacent",
                "nodeIds": node_ids,
                "coreIds": ["core"],
                "adjacentIds": adjacent_ids,
                "leafIds": leaf_ids,
                "layoutAnchorIds": [],
                "localAdjacency": Value::Object(local_adjacency)
            }],
            "nodeClusterById": {},
            "nodeMetaById": {}
        });

        let plan = project_cluster_layout_plan(
            &adjacency,
            &mut role_result,
            &mut cluster_result,
            &json!({
                "mode": "network",
                "margin": 20,
                "nodeRowsById": Value::Object(node_rows_by_id),
                "config": {
                    "R_LEAF_MIN": 180,
                    "NETWORK_ADJ_SPACING": 118,
                    "NETWORK_SECTOR_LAYER_GAP": 64
                }
            }),
        );

        let updates = plan["networkNodeUpdatesByClusterId"]["cluster-dense-adjacent"]
            .as_array()
            .expect("network updates");
        let adjacent_positions = updates
            .iter()
            .filter(|row| row["layoutBand"] == json!("adjacent"))
            .map(|row| {
                (
                    row["x"]
                        .as_f64()
                        .or_else(|| row["x"].as_i64().map(|value| value as f64))
                        .unwrap(),
                    row["y"]
                        .as_f64()
                        .or_else(|| row["y"].as_i64().map(|value| value as f64))
                        .unwrap(),
                )
            })
            .collect::<Vec<_>>();
        assert_eq!(adjacent_positions.len(), 12);
        let min_core_distance = adjacent_positions
            .iter()
            .map(|(x, y)| (x * x + y * y).sqrt())
            .fold(f64::INFINITY, f64::min);
        assert!(
            min_core_distance > 360.0,
            "dense adjacent centers should not stay on the fixed 118px ring"
        );
        let mut min_pair_distance = f64::INFINITY;
        for i in 0..adjacent_positions.len() {
            for j in (i + 1)..adjacent_positions.len() {
                let dx = adjacent_positions[i].0 - adjacent_positions[j].0;
                let dy = adjacent_positions[i].1 - adjacent_positions[j].1;
                min_pair_distance = min_pair_distance.min((dx * dx + dy * dy).sqrt());
            }
        }
        assert!(
            min_pair_distance > 220.0,
            "dense adjacent centers should reserve label/collision room"
        );
    }

    #[test]
    fn network_plan_projects_core_and_adjacent_owned_sector_updates() {
        let mut adjacency = HashMap::new();
        adjacency.insert(
            "core".to_string(),
            BTreeSet::from([
                "adjacent".to_string(),
                "leaf-core-1".to_string(),
                "leaf-core-2".to_string(),
            ]),
        );
        adjacency.insert(
            "adjacent".to_string(),
            BTreeSet::from([
                "core".to_string(),
                "leaf-adj-1".to_string(),
                "leaf-adj-2".to_string(),
            ]),
        );
        adjacency.insert(
            "leaf-core-1".to_string(),
            BTreeSet::from(["core".to_string()]),
        );
        adjacency.insert(
            "leaf-core-2".to_string(),
            BTreeSet::from(["core".to_string()]),
        );
        adjacency.insert(
            "leaf-adj-1".to_string(),
            BTreeSet::from(["adjacent".to_string()]),
        );
        adjacency.insert(
            "leaf-adj-2".to_string(),
            BTreeSet::from(["adjacent".to_string()]),
        );
        let mut role_result = json!({
            "rolesById": {
                "core": "core",
                "adjacent": "adjacent",
                "leaf-core-1": "leaf",
                "leaf-core-2": "leaf",
                "leaf-adj-1": "leaf",
                "leaf-adj-2": "leaf"
            },
            "nodeMetaById": {},
            "leafEntryById": {
                "leaf-core-1": "core",
                "leaf-core-2": "core",
                "leaf-adj-1": "adjacent",
                "leaf-adj-2": "adjacent"
            }
        });
        let mut cluster_result = json!({
            "clusters": [{
                "clusterId": "cluster-core-adjacent-owned",
                "nodeIds": ["core", "adjacent", "leaf-core-1", "leaf-core-2", "leaf-adj-1", "leaf-adj-2"],
                "coreIds": ["core"],
                "adjacentIds": ["adjacent"],
                "leafIds": ["leaf-core-1", "leaf-core-2", "leaf-adj-1", "leaf-adj-2"],
                "layoutAnchorIds": [],
                "localAdjacency": {
                    "core": ["adjacent", "leaf-core-1", "leaf-core-2"],
                    "adjacent": ["core", "leaf-adj-1", "leaf-adj-2"],
                    "leaf-core-1": ["core"],
                    "leaf-core-2": ["core"],
                    "leaf-adj-1": ["adjacent"],
                    "leaf-adj-2": ["adjacent"]
                }
            }],
            "nodeClusterById": {
                "core": "cluster-core-adjacent-owned",
                "adjacent": "cluster-core-adjacent-owned",
                "leaf-core-1": "cluster-core-adjacent-owned",
                "leaf-core-2": "cluster-core-adjacent-owned",
                "leaf-adj-1": "cluster-core-adjacent-owned",
                "leaf-adj-2": "cluster-core-adjacent-owned"
            },
            "nodeMetaById": {}
        });

        let plan = project_cluster_layout_plan(
            &adjacency,
            &mut role_result,
            &mut cluster_result,
            &json!({
                "mode": "network",
                "margin": 20,
                "slotBySignature": {
                    "network|adjacent|core|leaf-adj-1|leaf-adj-2|leaf-core-1|leaf-core-2": {"width": 680, "height": 560}
                },
                "config": {
                    "R_LEAF_MIN": 180,
                    "NETWORK_ADJ_SPACING": 118,
                    "NETWORK_SECTOR_LAYER_GAP": 64
                }
            }),
        );

        assert_eq!(
            cluster_result["clusters"][0]["layoutCase"],
            json!("single-center-sector")
        );
        assert_eq!(
            plan["networkNodeUpdateSummary"],
            json!({
                "clusterCount": 1,
                "nodeCount": 6,
                "coveredCases": {"single-center-sector": 1}
            })
        );
        let updates = plan["networkNodeUpdatesByClusterId"]["cluster-core-adjacent-owned"]
            .as_array()
            .expect("network updates");
        assert_eq!(
            updates
                .iter()
                .map(|row| row["id"].clone())
                .collect::<Vec<_>>(),
            vec![
                json!("core"),
                json!("leaf-core-1"),
                json!("leaf-core-2"),
                json!("adjacent"),
                json!("leaf-adj-1"),
                json!("leaf-adj-2")
            ]
        );
        let owner_rows = updates
            .iter()
            .filter(|row| row["layoutBand"] == json!("leaf"))
            .map(|row| (row["id"].clone(), row["ownerCenterId"].clone()))
            .collect::<Vec<_>>();
        assert_eq!(
            owner_rows,
            vec![
                (json!("leaf-core-1"), json!("core")),
                (json!("leaf-core-2"), json!("core")),
                (json!("leaf-adj-1"), json!("adjacent")),
                (json!("leaf-adj-2"), json!("adjacent")),
            ]
        );
        let center_reports = plan["networkCenterReportsByClusterId"]["cluster-core-adjacent-owned"]
            .as_array()
            .expect("center reports");
        assert_eq!(
            center_reports
                .iter()
                .map(|row| (
                    row["centerId"].clone(),
                    row["centerType"].clone(),
                    row["leafCount"].clone()
                ))
                .collect::<Vec<_>>(),
            vec![
                (json!("core"), json!("core"), json!(2)),
                (json!("adjacent"), json!("adjacent"), json!(2))
            ]
        );
    }

    #[test]
    fn network_plan_skips_incomplete_core_adjacent_sector_updates() {
        let mut adjacency = HashMap::new();
        adjacency.insert(
            "core".to_string(),
            BTreeSet::from([
                "adjacent".to_string(),
                "leaf-1".to_string(),
                "leaf-2".to_string(),
                "leaf-3".to_string(),
            ]),
        );
        adjacency.insert(
            "adjacent".to_string(),
            BTreeSet::from(["core".to_string(), "leaf-1".to_string()]),
        );
        for leaf in ["leaf-1", "leaf-2", "leaf-3"] {
            adjacency
                .entry(leaf.to_string())
                .or_default()
                .insert("core".to_string());
        }
        let mut role_result = json!({
            "rolesById": {
                "core": "core",
                "adjacent": "adjacent",
                "leaf-1": "leaf",
                "leaf-2": "leaf",
                "leaf-3": "leaf"
            },
            "nodeMetaById": {},
            "leafEntryById": {
                "leaf-1": "adjacent",
                "leaf-2": "core",
                "leaf-3": "core"
            }
        });
        let mut cluster_result = json!({
            "clusters": [{
                "clusterId": "cluster-adjacent",
                "nodeIds": ["core", "adjacent", "leaf-1", "leaf-2", "leaf-3"],
                "coreIds": ["core"],
                "adjacentIds": ["adjacent"],
                "leafIds": ["leaf-1", "leaf-2", "leaf-3"],
                "layoutAnchorIds": [],
                "localAdjacency": {
                    "core": ["adjacent", "leaf-1", "leaf-2", "leaf-3"],
                    "adjacent": ["core", "leaf-1"],
                    "leaf-1": ["core", "adjacent"],
                    "leaf-2": ["core"],
                    "leaf-3": ["core"]
                }
            }],
            "nodeClusterById": {
                "core": "cluster-adjacent",
                "adjacent": "cluster-adjacent",
                "leaf-1": "cluster-adjacent",
                "leaf-2": "cluster-adjacent",
                "leaf-3": "cluster-adjacent"
            },
            "nodeMetaById": {}
        });

        let plan = project_cluster_layout_plan(
            &adjacency,
            &mut role_result,
            &mut cluster_result,
            &json!({
                "mode": "network",
                "margin": 20,
                "slotBySignature": {
                    "network|adjacent|core|leaf-1|leaf-2|leaf-3": {"width": 560, "height": 460}
                },
                "config": {"R_LEAF_MIN": 180, "NETWORK_SECTOR_LAYER_GAP": 64}
            }),
        );

        assert_eq!(
            cluster_result["clusters"][0]["layoutCase"],
            json!("single-center-sector")
        );
        assert_eq!(
            plan["networkNodeUpdateSummary"],
            json!({
                "clusterCount": 0,
                "nodeCount": 0,
                "coveredCases": {"single-center-sector": 0}
            })
        );
        assert_eq!(plan["networkNodeUpdatesByClusterId"], json!({}));
        assert_eq!(plan["networkCenterReportsByClusterId"], json!({}));
    }
}
