use icu_collator::{options::CollatorOptions, Collator};
use icu_locale_core::locale;
use serde_json::{json, Value};
use std::collections::HashSet;

use super::value_helpers::{
    bool_field, clamp_number, compare_f64_desc, js_truthy, number_field, parse_unique_list,
    text_value,
};

#[derive(Clone, Debug)]
struct ViewportExpandHit {
    id: String,
    distance: f64,
    amount: f64,
    count: f64,
    tile_ids: Vec<String>,
}

#[derive(Clone, Debug)]
struct ViewportExpandConfig<'a> {
    width: f64,
    height: f64,
    padding_x: f64,
    padding_y: f64,
    center_x: f64,
    center_y: f64,
    prefer_tiles: bool,
    project_dot_cluster_targets: bool,
    tiles_per_cluster: usize,
    projection: &'a Value,
}

pub(super) fn project_viewport_expand_targets(contract: &Value) -> Value {
    let viewport = contract
        .get("viewport")
        .or_else(|| contract.get("viewPort"))
        .or_else(|| contract.get("view_port"))
        .unwrap_or(&Value::Null);
    let width = viewport_option_number(contract, "width")
        .or_else(|| viewport_size_option_number(viewport, "width"))
        .unwrap_or(0.0)
        .max(0.0);
    let height = viewport_option_number(contract, "height")
        .or_else(|| viewport_size_option_number(viewport, "height"))
        .unwrap_or(0.0)
        .max(0.0);
    if width == 0.0 || height == 0.0 {
        return empty_viewport_expand_targets();
    }
    let padding = viewport_option_number(contract, "padding")
        .map(|value| clamp_number(value, 0.0, 240.0))
        .unwrap_or(0.0);
    let padding = if padding == 0.0 { 72.0 } else { padding };
    let padding_x = viewport_option_number(contract, "paddingX")
        .or_else(|| viewport_option_number(contract, "padding_x"))
        .map(|value| value.max(0.0))
        .unwrap_or(padding);
    let padding_y = viewport_option_number(contract, "paddingY")
        .or_else(|| viewport_option_number(contract, "padding_y"))
        .map(|value| value.max(0.0))
        .unwrap_or(padding);
    let max_clusters = viewport_option_number(contract, "maxClusters")
        .or_else(|| viewport_option_number(contract, "max_clusters"))
        .map(|value| clamp_number(value, 1.0, 128.0))
        .unwrap_or(1.0) as usize;
    let max_tiles = viewport_option_number(contract, "maxTiles")
        .or_else(|| viewport_option_number(contract, "max_tiles"))
        .map(|value| clamp_number(value, 1.0, 512.0))
        .unwrap_or(1.0) as usize;
    let max_nodes = viewport_option_number(contract, "maxNodes")
        .or_else(|| viewport_option_number(contract, "max_nodes"))
        .map(|value| clamp_number(value, 1.0, 1024.0))
        .unwrap_or(1.0) as usize;
    let tiles_per_cluster = viewport_option_number(contract, "tilesPerCluster")
        .or_else(|| viewport_option_number(contract, "tiles_per_cluster"))
        .map(|value| clamp_number(value, 1.0, 6.0))
        .unwrap_or(1.0) as usize;
    let prefer_tiles = !matches!(
        contract
            .get("preferTiles")
            .or_else(|| contract.get("prefer_tiles")),
        Some(Value::Bool(false))
    );
    let project_dot_cluster_targets = bool_field(
        contract
            .get("projectDotClusterTargets")
            .or_else(|| contract.get("project_dot_cluster_targets")),
    );
    let projection = contract.get("projection").unwrap_or(&Value::Null);
    let center_x = width / 2.0;
    let center_y = height / 2.0;
    let config = ViewportExpandConfig {
        width,
        height,
        padding_x,
        padding_y,
        center_x,
        center_y,
        prefer_tiles,
        project_dot_cluster_targets,
        tiles_per_cluster,
        projection,
    };
    let mut cluster_hits = Vec::new();
    let mut node_hits = Vec::new();

    let entries = contract.get("entries").and_then(Value::as_array);
    if let Some(entries) = entries {
        for entry in entries {
            let x = nullish_or_fallback_number(
                entry.get("canvasX").or_else(|| entry.get("canvas_x")),
                entry.get("x"),
            );
            let y = nullish_or_fallback_number(
                entry.get("canvasY").or_else(|| entry.get("canvas_y")),
                entry.get("y"),
            );
            let Some(x) = x else {
                continue;
            };
            let Some(y) = y else {
                continue;
            };
            push_viewport_expand_hit(entry, x, y, &config, &mut cluster_hits, &mut node_hits);
        }
    } else {
        push_runtime_graph_viewport_hits(
            contract,
            viewport,
            &config,
            &mut cluster_hits,
            &mut node_hits,
        );
    }

    sort_viewport_expand_hits(&mut cluster_hits);
    sort_viewport_expand_hits(&mut node_hits);
    let mut tile_ids = Vec::new();
    let mut cluster_ids = Vec::new();
    for hit in cluster_hits.iter().take(max_clusters) {
        let next_tile_ids = unique_strings(hit.tile_ids.iter().cloned());
        if prefer_tiles && !next_tile_ids.is_empty() {
            for tile_id in next_tile_ids {
                if tile_ids.len() >= max_tiles {
                    break;
                }
                if !tile_ids.contains(&tile_id) {
                    tile_ids.push(tile_id);
                }
            }
            continue;
        }
        if !cluster_ids.contains(&hit.id) {
            cluster_ids.push(hit.id.clone());
        }
    }
    let node_ids = node_hits
        .iter()
        .take(max_nodes)
        .map(|hit| hit.id.clone())
        .collect::<Vec<_>>();

    json!({
        "clusterIds": cluster_ids,
        "tileIds": tile_ids,
        "nodeIds": node_ids,
    })
}

fn empty_viewport_expand_targets() -> Value {
    json!({
        "clusterIds": [],
        "tileIds": [],
        "nodeIds": [],
    })
}

fn viewport_option_number(contract: &Value, key: &str) -> Option<f64> {
    number_field(contract.get(key))
}

fn viewport_size_option_number(viewport: &Value, key: &str) -> Option<f64> {
    let size = viewport.get("size").unwrap_or(&Value::Null);
    number_field(size.get(key)).or_else(|| number_field(viewport.get(key)))
}

fn viewport_center_option_number(viewport: &Value, key: &str) -> Option<f64> {
    let center = viewport.get("center").unwrap_or(&Value::Null);
    number_field(center.get(key))
}

fn nullish_or_fallback_number(primary: Option<&Value>, fallback: Option<&Value>) -> Option<f64> {
    match primary {
        None | Some(Value::Null) => number_field(fallback),
        Some(_) => number_field(primary),
    }
}

fn push_runtime_graph_viewport_hits(
    contract: &Value,
    viewport: &Value,
    config: &ViewportExpandConfig<'_>,
    cluster_hits: &mut Vec<ViewportExpandHit>,
    node_hits: &mut Vec<ViewportExpandHit>,
) {
    let Some(zoom) = number_field(viewport.get("zoom")).filter(|value| *value > 0.0) else {
        return;
    };
    let Some(center_x) = viewport_center_option_number(viewport, "x") else {
        return;
    };
    let Some(center_y) = viewport_center_option_number(viewport, "y") else {
        return;
    };
    let graph = contract
        .get("runtimeGraph")
        .or_else(|| contract.get("runtime_graph"))
        .or_else(|| contract.get("graph"))
        .unwrap_or(&Value::Null);
    let Some(nodes) = graph.get("nodes").and_then(Value::as_array) else {
        return;
    };
    for node in nodes {
        let Some(world_x) = number_field(node.get("x")) else {
            continue;
        };
        let Some(world_y) = number_field(node.get("y")) else {
            continue;
        };
        let canvas_x = (world_x - center_x) * zoom + config.width / 2.0;
        let canvas_y = (world_y - center_y) * zoom + config.height / 2.0;
        push_viewport_expand_hit(node, canvas_x, canvas_y, config, cluster_hits, node_hits);
    }
}

fn push_viewport_expand_hit(
    entry: &Value,
    x: f64,
    y: f64,
    config: &ViewportExpandConfig<'_>,
    cluster_hits: &mut Vec<ViewportExpandHit>,
    node_hits: &mut Vec<ViewportExpandHit>,
) {
    if !is_collapsed_projection_entry(entry) {
        return;
    }
    if x < -config.padding_x
        || x > config.width + config.padding_x
        || y < -config.padding_y
        || y > config.height + config.padding_y
    {
        return;
    }
    let id = text_value(entry.get("id"));
    if id.is_empty() {
        return;
    }
    let is_cluster = is_cluster_projection_entry(entry);
    let projection_cluster_id = text_value(
        entry
            .get("projection_cluster_id")
            .or_else(|| entry.get("cluster_id")),
    );
    let target_cluster_id = if is_cluster {
        id.clone()
    } else if config.project_dot_cluster_targets {
        projection_cluster_id
    } else {
        String::new()
    };
    let cluster_meta = if !target_cluster_id.is_empty() {
        projection_cluster_meta(&target_cluster_id, config.projection)
    } else {
        None
    };
    let tile_ids = if config.prefer_tiles {
        cluster_meta
            .map(|meta| projection_cluster_next_tile_ids(meta, config.tiles_per_cluster))
            .unwrap_or_default()
    } else {
        Vec::new()
    };
    let hit = ViewportExpandHit {
        id,
        distance: ((x - config.center_x).powi(2) + (y - config.center_y).powi(2)).sqrt(),
        amount: number_field(entry.get("total_amount")).unwrap_or(0.0).abs(),
        count: number_field(entry.get("total_count"))
            .unwrap_or(0.0)
            .max(0.0),
        tile_ids,
    };
    if is_cluster || !target_cluster_id.is_empty() {
        upsert_viewport_expand_hit(
            cluster_hits,
            ViewportExpandHit {
                id: target_cluster_id,
                ..hit
            },
        );
    } else {
        node_hits.push(hit);
    }
}

fn is_cluster_projection_entry(entry: &Value) -> bool {
    if !matches!(entry, Value::Object(_)) {
        return false;
    }
    if js_truthy(entry.get("cluster_node")) {
        return true;
    }
    text_value(entry.get("id")).starts_with("__cluster__::")
}

fn is_collapsed_projection_entry(entry: &Value) -> bool {
    if !matches!(entry, Value::Object(_)) {
        return false;
    }
    if is_cluster_projection_entry(entry) {
        return true;
    }
    let render_mode = text_value(
        entry
            .get("nodeRenderMode")
            .or_else(|| entry.get("node_render_mode")),
    )
    .to_ascii_lowercase();
    render_mode == "dot"
}

fn projection_cluster_meta<'a>(cluster_id: &str, projection: &'a Value) -> Option<&'a Value> {
    if cluster_id.trim().is_empty() {
        return None;
    }
    projection
        .get("clusters")
        .and_then(Value::as_array)?
        .iter()
        .find(|row| {
            let row_id = text_value(row.get("cluster_id").or_else(|| row.get("clusterId")));
            !row_id.is_empty() && row_id == cluster_id
        })
}

fn projection_cluster_next_tile_ids(cluster_meta: &Value, limit: usize) -> Vec<String> {
    parse_unique_list(
        cluster_meta
            .get("next_tile_ids")
            .or_else(|| cluster_meta.get("nextTileIds"))
            .or_else(|| cluster_meta.get("remaining_tile_ids"))
            .or_else(|| cluster_meta.get("remainingTileIds")),
        true,
    )
    .into_iter()
    .take(limit.max(1))
    .collect()
}

fn unique_strings(items: impl Iterator<Item = String>) -> Vec<String> {
    let mut out = Vec::new();
    let mut seen = HashSet::new();
    for item in items {
        let value = item.trim().to_string();
        if value.is_empty() || seen.contains(&value) {
            continue;
        }
        seen.insert(value.clone());
        out.push(value);
    }
    out
}

fn upsert_viewport_expand_hit(hits: &mut Vec<ViewportExpandHit>, hit: ViewportExpandHit) {
    if hit.id.is_empty() {
        return;
    }
    if let Some(existing) = hits.iter_mut().find(|row| row.id == hit.id) {
        if hit.distance < existing.distance
            || hit.amount > existing.amount
            || hit.count > existing.count
        {
            *existing = hit;
        }
        return;
    }
    hits.push(hit);
}

fn sort_viewport_expand_hits(hits: &mut [ViewportExpandHit]) {
    let collator = Collator::try_new(locale!("zh-CN").into(), CollatorOptions::default()).ok();
    hits.sort_by(|left, right| {
        left.distance
            .partial_cmp(&right.distance)
            .unwrap_or(std::cmp::Ordering::Equal)
            .then_with(|| compare_f64_desc(left.amount, right.amount))
            .then_with(|| compare_f64_desc(left.count, right.count))
            .then_with(|| match &collator {
                Some(collator) => collator.compare(left.id.as_str(), right.id.as_str()),
                None => left.id.cmp(&right.id),
            })
    });
}
