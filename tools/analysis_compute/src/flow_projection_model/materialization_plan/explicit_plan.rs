use serde_json::{Map, Value};
use std::collections::{BTreeMap, BTreeSet, VecDeque};

use super::request_model::{
    projection_include_neighbors, projection_materialize_limit, projection_neighbor_depth,
    request_text_list,
};

pub(in crate::flow_projection_model) struct ProjectionExplicitState {
    pub(in crate::flow_projection_model) base_projection_node_ids: Vec<String>,
    pub(in crate::flow_projection_model) path_entity_ids: Vec<String>,
    pub(in crate::flow_projection_model) explicit_entity_ids: Vec<String>,
}

pub(in crate::flow_projection_model) fn build_projection_explicit_entity_ids(
    anchor_ids: &[String],
    request_context: Option<&Map<String, Value>>,
    base_projection: Option<&Map<String, Value>>,
    search_match_node_ids: &[String],
    node_map: &BTreeMap<String, Value>,
    adjacency: &BTreeMap<String, BTreeSet<String>>,
) -> ProjectionExplicitState {
    let materialize_limit = projection_materialize_limit(request_context);
    let base_projection_node_ids: BTreeSet<String> =
        request_text_list(base_projection, &["expanded_node_ids", "expandedNodeIds"])
            .into_iter()
            .filter(|item| node_map.contains_key(item))
            .collect();
    let path_entity_ids: BTreeSet<String> = materialize_projection_path(
        adjacency,
        &request_text_list(request_context, &["path_node_ids", "pathNodeIds"]),
        materialize_limit,
    )
    .into_iter()
    .filter(|item| node_map.contains_key(item))
    .collect();

    let mut explicit_entity_ids = BTreeSet::new();
    for node_id in anchor_ids {
        if node_map.contains_key(node_id) {
            explicit_entity_ids.insert(node_id.clone());
        }
    }
    explicit_entity_ids.extend(base_projection_node_ids.iter().cloned());
    for node_id in request_text_list(request_context, &["node_ids", "nodeIds"]) {
        if node_map.contains_key(&node_id) {
            explicit_entity_ids.insert(node_id);
        }
    }
    for node_id in search_match_node_ids {
        if node_map.contains_key(node_id) {
            explicit_entity_ids.insert(node_id.clone());
        }
    }
    explicit_entity_ids.extend(path_entity_ids.iter().cloned());

    if projection_include_neighbors(request_context) {
        explicit_entity_ids = expand_projection_neighbors(
            explicit_entity_ids,
            adjacency,
            node_map,
            projection_neighbor_depth(request_context),
            materialize_limit,
        );
    }

    ProjectionExplicitState {
        base_projection_node_ids: base_projection_node_ids.into_iter().collect(),
        path_entity_ids: path_entity_ids.into_iter().collect(),
        explicit_entity_ids: explicit_entity_ids.into_iter().collect(),
    }
}

fn materialize_projection_path(
    adjacency: &BTreeMap<String, BTreeSet<String>>,
    path_node_ids: &[String],
    limit: usize,
) -> BTreeSet<String> {
    let wanted: Vec<String> = path_node_ids
        .iter()
        .map(|item| item.trim().to_string())
        .filter(|item| !item.is_empty())
        .collect();
    let mut out: BTreeSet<String> = wanted.iter().cloned().collect();
    if wanted.len() < 2 {
        return out;
    }
    let search_limit = limit.saturating_mul(4).max(512);
    for pair in wanted.windows(2) {
        let [start, target] = pair else {
            continue;
        };
        if start == target {
            out.insert(start.clone());
            continue;
        }
        let mut queue = VecDeque::from([start.clone()]);
        let mut prev: BTreeMap<String, Option<String>> = BTreeMap::from([(start.clone(), None)]);
        let mut found = false;
        while let Some(current) = queue.pop_front() {
            if current == *target {
                found = true;
                break;
            }
            if prev.len() > search_limit {
                break;
            }
            if let Some(neighbors) = adjacency.get(&current) {
                for next_id in neighbors {
                    if prev.contains_key(next_id) {
                        continue;
                    }
                    prev.insert(next_id.clone(), Some(current.clone()));
                    queue.push_back(next_id.clone());
                }
            }
        }
        if !found && !prev.contains_key(target) {
            continue;
        }
        let mut cursor = Some(target.clone());
        while let Some(node_id) = cursor {
            if out.len() >= limit {
                break;
            }
            out.insert(node_id.clone());
            cursor = prev.get(&node_id).cloned().flatten();
        }
    }
    out
}

fn expand_projection_neighbors(
    explicit_entity_ids: BTreeSet<String>,
    adjacency: &BTreeMap<String, BTreeSet<String>>,
    node_map: &BTreeMap<String, Value>,
    steps: usize,
    limit: usize,
) -> BTreeSet<String> {
    let mut visited = explicit_entity_ids;
    let mut frontier: Vec<String> = visited.iter().cloned().collect();
    for _ in 0..steps {
        if frontier.is_empty() || visited.len() >= limit {
            break;
        }
        let mut next_frontier = Vec::new();
        for node_id in frontier {
            if let Some(neighbors) = adjacency.get(&node_id) {
                for next_id in neighbors {
                    if visited.contains(next_id) {
                        continue;
                    }
                    visited.insert(next_id.clone());
                    next_frontier.push(next_id.clone());
                    if visited.len() >= limit {
                        break;
                    }
                }
            }
            if visited.len() >= limit {
                break;
            }
        }
        frontier = next_frontier;
    }
    visited
        .into_iter()
        .filter(|item| node_map.contains_key(item))
        .collect()
}
