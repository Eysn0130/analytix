use anyhow::{bail, Result};
use serde_json::{json, Map, Value};
use std::collections::{BTreeMap, BTreeSet, HashMap, HashSet};

use super::super::helpers::{projection_has_viewport, projection_use_point_layer};
use super::super::input_model::ProjectionNodeFacts;
use super::cluster_request::{
    collect_projection_cluster_requests, ProjectionClusterRequest, ProjectionTileRequest,
};
use super::request_model::{projection_materialize_limit, request_text_list};

pub(in crate::flow_projection_model) fn build_requested_cluster_materializations(
    clusters: &[Value],
    request_context: Option<&Map<String, Value>>,
    anchor_ids: &[String],
    base_projection_node_ids: &[String],
    explicit_entity_ids: &[String],
    node_map: &BTreeMap<String, Value>,
    node_facts: &BTreeMap<String, ProjectionNodeFacts>,
    tile_size: usize,
) -> Result<Vec<Value>> {
    let requested_cluster_ids = request_text_list(request_context, &["cluster_ids", "clusterIds"]);
    let requested_tile_ids = request_text_list(request_context, &["tile_ids", "tileIds"]);
    let requested_tile_order: BTreeMap<String, usize> = requested_tile_ids
        .iter()
        .enumerate()
        .map(|(index, item)| (item.clone(), index))
        .collect();
    let requested_cluster_order: BTreeMap<String, usize> = requested_cluster_ids
        .iter()
        .enumerate()
        .map(|(index, item)| (item.clone(), index))
        .collect();
    let active_clusters =
        collect_projection_cluster_requests(clusters, request_context, node_map, node_facts)?;

    let mut requested_clusters: Vec<ProjectionClusterRequest> = active_clusters
        .into_iter()
        .filter(|cluster| cluster.requested_expand || !cluster.requested_tile_ids.is_empty())
        .collect();
    let default_tile_order = requested_tile_ids.len() + 1;
    let default_cluster_order = requested_cluster_ids.len() + 1;
    requested_clusters.sort_by(|left, right| {
        let left_tile_order = left
            .requested_tile_ids
            .iter()
            .filter_map(|tile_id| requested_tile_order.get(tile_id).copied())
            .min()
            .unwrap_or(default_tile_order);
        let right_tile_order = right
            .requested_tile_ids
            .iter()
            .filter_map(|tile_id| requested_tile_order.get(tile_id).copied())
            .min()
            .unwrap_or(default_tile_order);
        left_tile_order.cmp(&right_tile_order).then_with(|| {
            requested_cluster_order
                .get(&left.cluster_id)
                .copied()
                .unwrap_or(default_cluster_order)
                .cmp(
                    &requested_cluster_order
                        .get(&right.cluster_id)
                        .copied()
                        .unwrap_or(default_cluster_order),
                )
        })
    });

    let anchor_set: HashSet<String> = anchor_ids.iter().cloned().collect();
    let base_projection_set: HashSet<String> = base_projection_node_ids.iter().cloned().collect();
    let mut explicit_set: BTreeSet<String> = explicit_entity_ids
        .iter()
        .filter(|item| node_map.contains_key(*item))
        .cloned()
        .collect();
    let requested_extra_count = explicit_set
        .iter()
        .filter(|item| !anchor_set.contains(*item) && !base_projection_set.contains(*item))
        .count();
    let materialize_limit = projection_materialize_limit(request_context);
    let mut remaining_cluster_budget = materialize_limit.saturating_sub(requested_extra_count);
    let mut remaining_requested_cluster_count = requested_clusters.len();
    let use_point_layer = projection_use_point_layer(request_context);
    let has_viewport = projection_has_viewport(request_context);
    let mut plans = Vec::new();

    for cluster in requested_clusters {
        let mut selected_ids = Vec::new();
        let mut selected_set: HashSet<String> = HashSet::new();
        let tile_lookup: HashMap<&str, &ProjectionTileRequest> = cluster
            .tiles
            .iter()
            .map(|tile| (tile.tile_id.as_str(), tile))
            .collect();
        let mut pending_tile_ids: Vec<String> = cluster
            .requested_tile_ids
            .iter()
            .filter(|tile_id| tile_lookup.contains_key(tile_id.as_str()))
            .cloned()
            .collect();
        if pending_tile_ids.is_empty() && cluster.requested_expand && use_point_layer {
            let mut auto_tile_candidates = Vec::new();
            for tile in &cluster.tiles {
                let remaining_tile_member_count = tile
                    .member_ids
                    .iter()
                    .filter(|node_id| {
                        node_map.contains_key(*node_id) && !explicit_set.contains(*node_id)
                    })
                    .count();
                if remaining_tile_member_count == 0 {
                    continue;
                }
                auto_tile_candidates.push((
                    tile.tile_id.clone(),
                    remaining_tile_member_count,
                    tile.total_amount_cents,
                ));
            }
            auto_tile_candidates.sort_by(|left, right| {
                right
                    .1
                    .cmp(&left.1)
                    .then_with(|| right.2.cmp(&left.2))
                    .then_with(|| left.0.cmp(&right.0))
            });
            let auto_tile_batch = if has_viewport { 3 } else { 1 };
            pending_tile_ids = auto_tile_candidates
                .into_iter()
                .take(auto_tile_batch.max(1))
                .map(|(tile_id, _, _)| tile_id)
                .collect();
        }
        if !pending_tile_ids.is_empty() && remaining_cluster_budget > 0 {
            let fair_share =
                remaining_cluster_budget.div_ceil(remaining_requested_cluster_count.max(1));
            let tile_budget = remaining_cluster_budget.min(fair_share.max(tile_size.max(1)));
            for tile_id in pending_tile_ids {
                let Some(tile) = tile_lookup.get(tile_id.as_str()) else {
                    continue;
                };
                let tile_member_ids: Vec<String> = tile
                    .member_ids
                    .iter()
                    .filter(|node_id| {
                        node_map.contains_key(*node_id)
                            && !explicit_set.contains(*node_id)
                            && !selected_set.contains(*node_id)
                    })
                    .cloned()
                    .collect();
                if tile_member_ids.is_empty() {
                    continue;
                }
                if !selected_ids.is_empty()
                    && selected_ids.len() + tile_member_ids.len() > tile_budget
                {
                    break;
                }
                if tile_member_ids.len() > tile_budget {
                    break;
                }
                for node_id in tile_member_ids {
                    selected_set.insert(node_id.clone());
                    selected_ids.push(node_id);
                }
            }
            if !selected_ids.is_empty() {
                for node_id in &selected_ids {
                    explicit_set.insert(node_id.clone());
                }
                remaining_cluster_budget =
                    remaining_cluster_budget.saturating_sub(selected_ids.len());
            }
        }
        if selected_ids.is_empty() && cluster.requested_expand && remaining_cluster_budget > 0 {
            let candidate_ids: Vec<String> = cluster
                .member_ids
                .iter()
                .filter(|node_id| {
                    node_map.contains_key(*node_id) && !explicit_set.contains(*node_id)
                })
                .cloned()
                .collect();
            let candidate_set: HashSet<String> = candidate_ids.iter().cloned().collect();
            let ranked_candidates: Vec<String> = cluster
                .ranked_member_ids
                .iter()
                .filter(|node_id| candidate_set.contains(*node_id))
                .cloned()
                .collect();
            if !candidate_ids.is_empty() && ranked_candidates.is_empty() {
                bail!("project-flow-skeleton-clusters returned no ranked candidates");
            }
            if !ranked_candidates.is_empty() {
                let fair_share =
                    remaining_cluster_budget.div_ceil(remaining_requested_cluster_count.max(1));
                let take_count = ranked_candidates
                    .len()
                    .min(remaining_cluster_budget)
                    .min(fair_share);
                selected_ids = ranked_candidates.into_iter().take(take_count).collect();
                for node_id in &selected_ids {
                    explicit_set.insert(node_id.clone());
                }
                remaining_cluster_budget =
                    remaining_cluster_budget.saturating_sub(selected_ids.len());
            }
        }
        if !selected_ids.is_empty() {
            plans.push(json!({
                "cluster_id": cluster.cluster_id,
                "materialized_member_ids": selected_ids,
            }));
        }
        remaining_requested_cluster_count = remaining_requested_cluster_count.saturating_sub(1);
    }
    Ok(plans)
}
