use serde_json::{json, Map, Value};
use std::collections::{BTreeMap, BTreeSet};

pub(super) fn projection_state(
    anchor_ids: Vec<String>,
    clusters: Vec<Value>,
    member_to_cluster: BTreeMap<String, String>,
    degree_map: &BTreeMap<String, i64>,
    adjacency: &BTreeMap<String, BTreeSet<String>>,
) -> Map<String, Value> {
    let mut state = Map::new();
    state.insert("anchor_ids".to_string(), json!(anchor_ids));
    state.insert("clusters".to_string(), Value::Array(clusters));
    state.insert("member_to_cluster".to_string(), json!(member_to_cluster));
    state.insert("degree_map".to_string(), json!(degree_map));
    let adjacency_json: BTreeMap<String, Vec<String>> = adjacency
        .iter()
        .map(|(node_id, neighbors)| (node_id.clone(), neighbors.iter().cloned().collect()))
        .collect();
    state.insert("adjacency".to_string(), json!(adjacency_json));
    state
}
