use anyhow::{anyhow, Result};
use serde_json::Value;
use std::collections::{BTreeMap, HashSet};

use super::super::input_model::ProjectionNodeFacts;

pub(in crate::flow_projection_model) fn rank_projection_materialize_node_ids(
    node_ids: &[String],
    node_map: &BTreeMap<String, Value>,
    node_facts: &BTreeMap<String, ProjectionNodeFacts>,
    degree_map: &BTreeMap<String, i64>,
) -> Result<Vec<String>> {
    let mut unique_ids = Vec::new();
    let mut seen = HashSet::new();
    for raw in node_ids {
        let node_id = raw.trim().to_string();
        if node_id.is_empty() || !seen.insert(node_id.clone()) || !node_map.contains_key(&node_id) {
            continue;
        }
        if !node_facts.contains_key(&node_id) || !degree_map.contains_key(&node_id) {
            return Err(anyhow!("projection ranking facts missing for {node_id}"));
        }
        unique_ids.push(node_id);
    }
    unique_ids.sort_by(|left, right| {
        let left_facts = &node_facts[left];
        let right_facts = &node_facts[right];
        right_facts
            .total_amount_cents
            .cmp(&left_facts.total_amount_cents)
            .then_with(|| right_facts.total_count.cmp(&left_facts.total_count))
            .then_with(|| degree_map[right].cmp(&degree_map[left]))
            .then_with(|| left.cmp(right))
    });
    Ok(unique_ids)
}
