use anyhow::{anyhow, Result};
use serde_json::{json, Map, Value};
use std::collections::{BTreeMap, BTreeSet};

use super::{
    helpers::{projection_use_point_layer, row_text, row_text_list, sum_projection_node_totals},
    input_model::{money_value, ProjectionNodeFacts},
    materialization_plan::collect_projection_cluster_requests,
};

pub(super) fn build_cluster_visibility_plan(
    clusters: &[Value],
    request_context: Option<&Map<String, Value>>,
    explicit_entity_ids: &[String],
    node_map: &BTreeMap<String, Value>,
    node_facts: &BTreeMap<String, ProjectionNodeFacts>,
    materialization_plans: &[Value],
) -> Result<Value> {
    let active_clusters =
        collect_projection_cluster_requests(clusters, request_context, node_map, node_facts)?;
    let mut explicit_set: BTreeSet<String> = explicit_entity_ids
        .iter()
        .filter(|item| node_map.contains_key(*item))
        .cloned()
        .collect();
    let mut materialization_lookup: BTreeMap<String, Vec<String>> = BTreeMap::new();
    for raw_plan in materialization_plans {
        let cluster_id = row_text(raw_plan, "cluster_id");
        if cluster_id.is_empty() {
            return Err(anyhow!("projection materialization is missing cluster_id"));
        }
        let member_ids = row_text_list(raw_plan, "materialized_member_ids");
        if member_ids
            .iter()
            .any(|node_id| !node_map.contains_key(node_id))
        {
            return Err(anyhow!(
                "projection materialization references unknown node coverage"
            ));
        }
        if materialization_lookup
            .insert(cluster_id.clone(), member_ids)
            .is_some()
        {
            return Err(anyhow!(
                "projection materialization duplicates cluster {cluster_id}"
            ));
        }
    }
    let active_cluster_ids: BTreeSet<&str> = active_clusters
        .iter()
        .map(|cluster| cluster.cluster_id.as_str())
        .collect();
    if let Some(stale_cluster_id) = materialization_lookup
        .keys()
        .find(|cluster_id| !active_cluster_ids.contains(cluster_id.as_str()))
    {
        return Err(anyhow!(
            "projection materialization references unknown cluster {stale_cluster_id}"
        ));
    }

    let mut materialized_by_cluster: BTreeMap<String, Vec<String>> = BTreeMap::new();
    for cluster in &active_clusters {
        let cluster_member_set: BTreeSet<&str> =
            cluster.member_ids.iter().map(String::as_str).collect();
        let selected_ids: BTreeSet<String> = materialization_lookup
            .get(&cluster.cluster_id)
            .cloned()
            .unwrap_or_default()
            .into_iter()
            .filter(|node_id| !explicit_set.contains(node_id))
            .collect();
        if selected_ids
            .iter()
            .any(|node_id| !cluster_member_set.contains(node_id.as_str()))
        {
            return Err(anyhow!(
                "projection materialization is inconsistent with cluster {}",
                cluster.cluster_id
            ));
        }
        if selected_ids.is_empty() {
            continue;
        }
        for node_id in &selected_ids {
            explicit_set.insert(node_id.clone());
        }
        materialized_by_cluster.insert(
            cluster.cluster_id.clone(),
            selected_ids.into_iter().collect(),
        );
    }

    let use_point_layer = projection_use_point_layer(request_context);
    let mut collapsed_member_to_cluster = Map::new();
    let mut expanded_cluster_ids = BTreeSet::new();
    let mut expanded_tile_ids = BTreeSet::new();
    let mut cluster_rows = Vec::new();
    let mut point_layer_buckets = Vec::new();
    let mut point_layer_total = 0usize;

    for cluster in active_clusters {
        let materialized_ids: BTreeSet<String> = materialized_by_cluster
            .get(&cluster.cluster_id)
            .cloned()
            .unwrap_or_default()
            .into_iter()
            .collect();
        let mut collapsed_ids = Vec::new();
        for node_id in &cluster.member_ids {
            if explicit_set.contains(node_id) || materialized_ids.contains(node_id) {
                continue;
            }
            collapsed_member_to_cluster
                .insert(node_id.clone(), Value::String(cluster.cluster_id.clone()));
            collapsed_ids.push(node_id.clone());
        }

        let mut expanded_cluster_tile_ids = Vec::new();
        let mut remaining_cluster_tiles: Vec<(String, usize, i64)> = Vec::new();
        for tile in &cluster.tiles {
            let remaining_tile_member_ids: Vec<String> = tile
                .member_ids
                .iter()
                .filter(|node_id| !explicit_set.contains(*node_id))
                .cloned()
                .collect();
            if remaining_tile_member_ids.is_empty() {
                expanded_cluster_tile_ids.push(tile.tile_id.clone());
                continue;
            }
            remaining_cluster_tiles.push((
                tile.tile_id.clone(),
                remaining_tile_member_ids.len(),
                tile.total_amount_cents,
            ));
        }
        remaining_cluster_tiles.sort_by(|left, right| {
            right
                .1
                .cmp(&left.1)
                .then_with(|| right.2.cmp(&left.2))
                .then_with(|| left.0.cmp(&right.0))
        });
        let remaining_tile_ids: Vec<String> = remaining_cluster_tiles
            .into_iter()
            .map(|(tile_id, _, _)| tile_id)
            .collect();
        let expanded = collapsed_ids.is_empty();
        let partially_expanded = (cluster.requested_expand
            || !expanded_cluster_tile_ids.is_empty())
            && !collapsed_ids.is_empty()
            && cluster
                .member_ids
                .iter()
                .any(|node_id| explicit_set.contains(node_id));
        if cluster.requested_expand
            || expanded
            || !materialized_ids.is_empty()
            || !expanded_cluster_tile_ids.is_empty()
        {
            expanded_cluster_ids.insert(cluster.cluster_id.clone());
        }
        for tile_id in &expanded_cluster_tile_ids {
            expanded_tile_ids.insert(tile_id.clone());
        }

        if use_point_layer && !expanded && !collapsed_ids.is_empty() {
            point_layer_total += collapsed_ids.len();
            let totals = sum_projection_node_totals(&collapsed_ids, node_facts)?;
            point_layer_buckets.push(json!({
                "cluster_id": cluster.cluster_id,
                "anchor_id": cluster.anchor_id,
                "point_count": collapsed_ids.len(),
                "member_count": collapsed_ids.len(),
                "tile_count": cluster.tiles.len(),
                "remaining_tile_count": remaining_tile_ids.len(),
                "next_tile_ids": remaining_tile_ids.iter().take(3).cloned().collect::<Vec<_>>(),
                "sample_node_ids": collapsed_ids.iter().take(16).cloned().collect::<Vec<_>>(),
                "total_amount": money_value(totals.total_amount_cents),
                "total_count": totals.total_count,
            }));
        }

        cluster_rows.push(json!({
            "cluster_id": cluster.cluster_id,
            "anchor_id": cluster.anchor_id,
            "anchor_ids": cluster.anchor_ids,
            "member_ids": cluster.member_ids,
            "ranked_member_ids": cluster.ranked_member_ids,
            "requested_expand": cluster.requested_expand,
            "requested_tile_ids": cluster.requested_tile_ids,
            "materialized_member_ids": materialized_ids.into_iter().collect::<Vec<_>>(),
            "collapsed_member_ids": collapsed_ids,
            "tiles": cluster.tiles.iter().map(|tile| {
                json!({
                    "tile_id": tile.tile_id,
                    "cluster_id": cluster.cluster_id,
                    "member_ids": tile.member_ids,
                    "member_count": tile.member_ids.len(),
                    "total_amount": money_value(tile.total_amount_cents),
                    "total_count": tile.total_count,
                })
            }).collect::<Vec<_>>(),
            "tile_ids": cluster.tiles.iter().map(|tile| tile.tile_id.clone()).collect::<Vec<_>>(),
            "expanded_tile_ids": expanded_cluster_tile_ids,
            "remaining_tile_ids": remaining_tile_ids,
            "expanded": expanded,
            "partially_expanded": partially_expanded,
        }));
    }

    Ok(json!({
        "clusters": cluster_rows,
        "collapsed_member_to_cluster": Value::Object(collapsed_member_to_cluster),
        "expanded_cluster_ids": expanded_cluster_ids.into_iter().collect::<Vec<_>>(),
        "expanded_tile_ids": expanded_tile_ids.into_iter().collect::<Vec<_>>(),
        "point_layer_total": point_layer_total,
        "point_layer_buckets": point_layer_buckets,
    }))
}

pub(super) fn collapsed_member_to_cluster(
    visibility_plan: &Value,
) -> Result<BTreeMap<String, String>> {
    let rows = visibility_plan
        .as_object()
        .and_then(|row| row.get("collapsed_member_to_cluster"))
        .and_then(Value::as_object)
        .ok_or_else(|| anyhow!("cluster visibility plan is missing collapsed member mapping"))?;
    let mut mapping = BTreeMap::new();
    for (node_id, cluster_id) in rows {
        let node_id = node_id.trim();
        let cluster_id = cluster_id
            .as_str()
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| anyhow!("cluster visibility mapping for {node_id} is invalid"))?;
        if node_id.is_empty() {
            return Err(anyhow!("cluster visibility mapping has an empty node id"));
        }
        mapping.insert(node_id.to_string(), cluster_id.to_string());
    }
    Ok(mapping)
}

pub(super) fn text_list(visibility_plan: &Value, key: &str) -> Result<Vec<String>> {
    let values = visibility_plan
        .as_object()
        .and_then(|row| row.get(key))
        .and_then(Value::as_array)
        .ok_or_else(|| anyhow!("cluster visibility field {key} is missing or invalid"))?;
    values
        .iter()
        .enumerate()
        .map(|(index, value)| {
            value
                .as_str()
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(str::to_string)
                .ok_or_else(|| anyhow!("cluster visibility field {key}[{index}] is invalid"))
        })
        .collect()
}

pub(super) fn array(visibility_plan: &Value, key: &str) -> Result<Vec<Value>> {
    visibility_plan
        .as_object()
        .and_then(|row| row.get(key))
        .and_then(Value::as_array)
        .cloned()
        .ok_or_else(|| anyhow!("cluster visibility field {key} is missing or invalid"))
}

pub(super) fn usize_field(visibility_plan: &Value, key: &str) -> Result<usize> {
    visibility_plan
        .as_object()
        .and_then(|row| row.get(key))
        .and_then(Value::as_u64)
        .and_then(|value| usize::try_from(value).ok())
        .ok_or_else(|| anyhow!("cluster visibility field {key} is missing or invalid"))
}
