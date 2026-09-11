use anyhow::{bail, Result};
use serde_json::{json, Map, Value};
use std::collections::{BTreeMap, BTreeSet, HashSet, VecDeque};

use super::helpers::{row_text, to_text_list};

const SKELETON_CLUSTER_MIN_SIZE: usize = 8;

mod anchor_picker;
mod state_model;
mod tile_model;

use super::input_model::ProjectionNodeFacts;
use anchor_picker::pick_projection_anchor_ids;
use state_model::projection_state;
use tile_model::{build_projection_cluster_tiles, cluster_node_id};

pub(super) fn build_projection_clusters(
    nodes: &[Value],
    request_context: Option<&Map<String, Value>>,
    base_projection: Option<&Map<String, Value>>,
    node_map: &BTreeMap<String, Value>,
    node_facts: &BTreeMap<String, ProjectionNodeFacts>,
    degree_map: &BTreeMap<String, i64>,
    adjacency: &BTreeMap<String, BTreeSet<String>>,
    tile_size: usize,
) -> Result<Map<String, Value>> {
    let existing_anchor_ids = base_projection
        .and_then(|projection| {
            projection
                .get("anchor_node_ids")
                .or_else(|| projection.get("anchorNodeIds"))
        })
        .map(|value| to_text_list(Some(value)).into_iter().collect::<Vec<_>>())
        .unwrap_or_default();
    let existing_clusters = base_projection
        .and_then(|projection| projection.get("clusters"))
        .and_then(Value::as_array);
    if let Some(existing_clusters) = existing_clusters {
        let mut clusters = Vec::new();
        let mut member_to_cluster = BTreeMap::new();
        for raw in existing_clusters {
            let cluster_id =
                row_text(raw, "cluster_id").or_else_empty(|| row_text(raw, "clusterId"));
            let anchor_id = row_text(raw, "anchor_id").or_else_empty(|| row_text(raw, "anchorId"));
            let member_ids = raw
                .as_object()
                .map(|row| {
                    row.get("member_ids")
                        .or_else(|| row.get("memberIds"))
                        .map(|value| to_text_list(Some(value)).into_iter().collect::<Vec<_>>())
                        .unwrap_or_default()
                })
                .unwrap_or_default();
            if cluster_id.is_empty() || anchor_id.is_empty() || member_ids.is_empty() {
                bail!("base projection cluster is missing identity or member coverage");
            }
            if !node_map.contains_key(&anchor_id)
                || member_ids.iter().any(|item| !node_map.contains_key(item))
            {
                bail!("base projection cluster references unknown node coverage");
            }
            for node_id in &member_ids {
                member_to_cluster.insert(node_id.clone(), cluster_id.clone());
            }
            let (ranked_member_ids, tiles) = build_projection_cluster_tiles(
                &cluster_id,
                &member_ids,
                node_map,
                node_facts,
                degree_map,
                tile_size,
            )?;
            clusters.push(json!({
                "cluster_id": cluster_id,
                "anchor_id": anchor_id,
                "anchor_ids": [anchor_id],
                "member_ids": member_ids,
                "ranked_member_ids": ranked_member_ids,
                "tiles": tiles,
            }));
        }
        if !clusters.is_empty() {
            return Ok(projection_state(
                existing_anchor_ids,
                clusters,
                member_to_cluster,
                degree_map,
                adjacency,
            ));
        }
    }

    let anchor_ids = if existing_anchor_ids.is_empty() {
        pick_projection_anchor_ids(
            nodes,
            request_context,
            node_map,
            node_facts,
            degree_map,
            adjacency,
        )?
    } else {
        existing_anchor_ids
    };
    let anchor_set: HashSet<String> = anchor_ids.iter().cloned().collect();
    let mut assignment: BTreeMap<String, String> = BTreeMap::new();
    let mut queue = VecDeque::new();
    for anchor_id in &anchor_ids {
        assignment.insert(anchor_id.clone(), anchor_id.clone());
        queue.push_back(anchor_id.clone());
    }
    while let Some(current) = queue.pop_front() {
        let current_anchor = assignment
            .get(&current)
            .cloned()
            .unwrap_or_else(|| current.clone());
        if let Some(neighbors) = adjacency.get(&current) {
            for next_id in neighbors {
                if assignment.contains_key(next_id) {
                    continue;
                }
                assignment.insert(next_id.clone(), current_anchor.clone());
                queue.push_back(next_id.clone());
            }
        }
    }
    for node_id in node_map.keys() {
        assignment
            .entry(node_id.clone())
            .or_insert_with(|| node_id.clone());
    }

    let mut grouped: BTreeMap<String, Vec<String>> = BTreeMap::new();
    for (node_id, anchor_id) in assignment {
        if anchor_set.contains(&node_id) {
            continue;
        }
        grouped
            .entry(anchor_id.trim().to_string())
            .or_default()
            .push(node_id);
    }

    let mut clusters = Vec::new();
    let mut member_to_cluster = BTreeMap::new();
    for (anchor_id, mut member_ids) in grouped {
        member_ids.sort();
        if member_ids.len() < SKELETON_CLUSTER_MIN_SIZE {
            continue;
        }
        let cluster_id = cluster_node_id(&anchor_id, &member_ids);
        for node_id in &member_ids {
            member_to_cluster.insert(node_id.clone(), cluster_id.clone());
        }
        let (ranked_member_ids, tiles) = build_projection_cluster_tiles(
            &cluster_id,
            &member_ids,
            node_map,
            node_facts,
            degree_map,
            tile_size,
        )?;
        clusters.push(json!({
            "cluster_id": cluster_id,
            "anchor_id": anchor_id,
            "anchor_ids": if anchor_id.is_empty() { Vec::<String>::new() } else { vec![anchor_id] },
            "member_ids": member_ids,
            "ranked_member_ids": ranked_member_ids,
            "tiles": tiles,
        }));
    }

    Ok(projection_state(
        anchor_ids,
        clusters,
        member_to_cluster,
        degree_map,
        adjacency,
    ))
}

trait OrElseEmpty {
    fn or_else_empty(self, fallback: impl FnOnce() -> String) -> String;
}

impl OrElseEmpty for String {
    fn or_else_empty(self, fallback: impl FnOnce() -> String) -> String {
        if self.is_empty() {
            fallback()
        } else {
            self
        }
    }
}
