use anyhow::{anyhow, bail, Result};
use serde_json::{Map, Value};
use std::collections::{BTreeMap, HashSet};

use super::super::{helpers::sum_projection_node_totals, input_model::ProjectionNodeFacts};
use super::request_model::request_text_list;

#[derive(Clone)]
pub(in crate::flow_projection_model) struct ProjectionTileRequest {
    pub(in crate::flow_projection_model) tile_id: String,
    pub(in crate::flow_projection_model) member_ids: Vec<String>,
    pub(in crate::flow_projection_model) total_amount_cents: i64,
    pub(in crate::flow_projection_model) total_count: i64,
}

#[derive(Clone)]
pub(in crate::flow_projection_model) struct ProjectionClusterRequest {
    pub(in crate::flow_projection_model) cluster_id: String,
    pub(in crate::flow_projection_model) anchor_id: String,
    pub(in crate::flow_projection_model) anchor_ids: Vec<String>,
    pub(in crate::flow_projection_model) member_ids: Vec<String>,
    pub(in crate::flow_projection_model) ranked_member_ids: Vec<String>,
    pub(in crate::flow_projection_model) requested_expand: bool,
    pub(in crate::flow_projection_model) requested_tile_ids: Vec<String>,
    pub(in crate::flow_projection_model) tiles: Vec<ProjectionTileRequest>,
}

pub(in crate::flow_projection_model) fn collect_projection_cluster_requests(
    clusters: &[Value],
    request_context: Option<&Map<String, Value>>,
    node_map: &BTreeMap<String, Value>,
    node_facts: &BTreeMap<String, ProjectionNodeFacts>,
) -> Result<Vec<ProjectionClusterRequest>> {
    let requested_cluster_ids = request_text_list(request_context, &["cluster_ids", "clusterIds"]);
    let requested_cluster_set: HashSet<String> = requested_cluster_ids.iter().cloned().collect();
    let requested_tile_ids = request_text_list(request_context, &["tile_ids", "tileIds"]);
    let mut active_clusters = Vec::new();
    let mut seen_cluster_ids = HashSet::new();
    let mut seen_cluster_members = HashSet::new();
    for raw_cluster in clusters {
        let row = raw_cluster
            .as_object()
            .ok_or_else(|| anyhow!("projection cluster must be an object"))?;
        let cluster_id = required_string(row, "cluster_id")?;
        let anchor_id = required_string(row, "anchor_id")?;
        let anchor_ids = required_known_node_ids(row, "anchor_ids", node_map)?;
        let member_ids = required_known_node_ids(row, "member_ids", node_map)?;
        let ranked_member_ids = required_known_node_ids(row, "ranked_member_ids", node_map)?;
        if member_ids.is_empty() || anchor_ids.is_empty() {
            bail!("projection cluster is missing id or covered members");
        }
        if !seen_cluster_ids.insert(cluster_id.clone()) {
            bail!("projection cluster id {cluster_id} is duplicated");
        }
        if !node_map.contains_key(&anchor_id) || !anchor_ids.contains(&anchor_id) {
            bail!("projection cluster anchor references unknown node {anchor_id}");
        }
        let member_set: HashSet<&str> = member_ids.iter().map(String::as_str).collect();
        let ranked_member_set: HashSet<&str> =
            ranked_member_ids.iter().map(String::as_str).collect();
        if member_set.len() != member_ids.len()
            || ranked_member_set != member_set
            || ranked_member_ids.len() != member_ids.len()
        {
            bail!("projection cluster ranked member coverage is inconsistent");
        }
        for member_id in &member_ids {
            if !seen_cluster_members.insert(member_id.clone()) {
                bail!("projection cluster member {member_id} is assigned more than once");
            }
        }
        let mut tiles = Vec::new();
        let raw_tiles = row
            .get("tiles")
            .and_then(Value::as_array)
            .ok_or_else(|| anyhow!("projection cluster is missing tiles"))?;
        let mut covered_tile_members = HashSet::new();
        let mut tile_ids = HashSet::new();
        for tile in raw_tiles {
            let tile_row = tile
                .as_object()
                .ok_or_else(|| anyhow!("projection cluster tile must be an object"))?;
            let tile_id = required_string(tile_row, "tile_id")?;
            if !tile_ids.insert(tile_id.clone()) {
                bail!("projection cluster tile id {tile_id} is duplicated");
            }
            let member_ids = required_known_node_ids(tile_row, "member_ids", node_map)?;
            if member_ids.is_empty() {
                bail!("projection cluster tile has no covered members");
            }
            for member_id in &member_ids {
                if !member_set.contains(member_id.as_str())
                    || !covered_tile_members.insert(member_id.clone())
                {
                    bail!("projection cluster tile member coverage is inconsistent");
                }
            }
            let totals = sum_projection_node_totals(&member_ids, node_facts)?;
            tiles.push(ProjectionTileRequest {
                tile_id,
                member_ids,
                total_amount_cents: totals.total_amount_cents,
                total_count: totals.total_count,
            });
        }
        if covered_tile_members.len() != member_set.len() {
            bail!("projection cluster tile coverage is incomplete");
        }
        let tile_id_set: HashSet<String> = tiles.iter().map(|tile| tile.tile_id.clone()).collect();
        active_clusters.push(ProjectionClusterRequest {
            requested_expand: requested_cluster_set.contains(&cluster_id),
            requested_tile_ids: requested_tile_ids
                .iter()
                .filter(|tile_id| tile_id_set.contains(*tile_id))
                .cloned()
                .collect(),
            ranked_member_ids,
            cluster_id,
            anchor_id,
            anchor_ids,
            member_ids,
            tiles,
        });
    }
    Ok(active_clusters)
}

fn required_string(row: &Map<String, Value>, field: &str) -> Result<String> {
    row.get(field)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_string)
        .ok_or_else(|| anyhow!("projection cluster field {field} is missing or invalid"))
}

fn required_known_node_ids(
    row: &Map<String, Value>,
    field: &str,
    node_map: &BTreeMap<String, Value>,
) -> Result<Vec<String>> {
    let values = row
        .get(field)
        .and_then(Value::as_array)
        .ok_or_else(|| anyhow!("projection cluster field {field} is missing or invalid"))?;
    values
        .iter()
        .enumerate()
        .map(|(index, value)| {
            let node_id = value
                .as_str()
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .ok_or_else(|| anyhow!("projection cluster field {field}[{index}] is invalid"))?;
            if !node_map.contains_key(node_id) {
                bail!(
                    "projection cluster field {field}[{index}] references unknown node {node_id}"
                );
            }
            Ok(node_id.to_string())
        })
        .collect()
}
