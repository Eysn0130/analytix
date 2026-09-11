use anyhow::{anyhow, Result};
use serde_json::{Map, Value};
use std::collections::{BTreeMap, BTreeSet, HashSet};

use super::super::{
    helpers::{row_text, to_text_list, value_text},
    input_model::ProjectionNodeFacts,
};

const SKELETON_MAX_ANCHORS: usize = 96;

pub(super) fn pick_projection_anchor_ids(
    nodes: &[Value],
    request_context: Option<&Map<String, Value>>,
    node_map: &BTreeMap<String, Value>,
    node_facts: &BTreeMap<String, ProjectionNodeFacts>,
    degree_map: &BTreeMap<String, i64>,
    adjacency: &BTreeMap<String, BTreeSet<String>>,
) -> Result<Vec<String>> {
    let focus_ids = build_projection_focus_ids(request_context);
    let mut ranked: Vec<(i64, i64, i64, i64, String)> = Vec::new();
    for row in nodes {
        let node_id = row_text(row, "id");
        if node_id.is_empty() {
            continue;
        }
        let is_seed = row_text(row, "ntype").to_lowercase() == "seed";
        let is_focus = focus_ids.contains(&node_id);
        let priority = if is_focus {
            5
        } else if is_seed {
            4
        } else {
            0
        };
        let facts = node_facts
            .get(&node_id)
            .ok_or_else(|| anyhow!("projection anchor facts missing for {node_id}"))?;
        let degree = degree_map
            .get(&node_id)
            .copied()
            .ok_or_else(|| anyhow!("projection degree missing for {node_id}"))?;
        ranked.push((
            priority,
            degree,
            facts.total_amount_cents,
            facts.total_count,
            node_id,
        ));
    }
    ranked.sort_by(|left, right| {
        right
            .0
            .cmp(&left.0)
            .then_with(|| right.1.cmp(&left.1))
            .then_with(|| right.2.cmp(&left.2))
            .then_with(|| right.3.cmp(&left.3))
            .then_with(|| left.4.cmp(&right.4))
    });

    let mut anchors = Vec::new();
    let mut seen = HashSet::new();
    for (_, _, _, _, node_id) in ranked.into_iter().take(anchor_budget(nodes.len())) {
        if seen.insert(node_id.clone()) {
            anchors.push(node_id);
        }
    }

    add_component_anchor_ids(
        &mut anchors,
        &mut seen,
        node_map,
        node_facts,
        degree_map,
        adjacency,
    )?;
    Ok(anchors)
}

fn build_projection_focus_ids(request_context: Option<&Map<String, Value>>) -> HashSet<String> {
    let mut out = HashSet::new();
    let Some(request) = request_context else {
        return out;
    };
    for key in [
        "seeds",
        "left_seeds",
        "leftSeeds",
        "focus_ids",
        "focusIds",
        "node_ids",
        "nodeIds",
    ] {
        for item in to_text_list(request.get(key)) {
            out.insert(item);
        }
    }
    for key in ["focus_id", "focusId"] {
        let text = request.get(key).map(value_text).unwrap_or_default();
        if !text.is_empty() {
            out.insert(text);
        }
    }
    out
}

fn anchor_budget(node_count: usize) -> usize {
    if node_count <= 100 {
        4
    } else if node_count <= 1_000 {
        12
    } else if node_count <= 10_000 {
        24
    } else if node_count <= 100_000 {
        64
    } else {
        SKELETON_MAX_ANCHORS
    }
}

fn add_component_anchor_ids(
    anchors: &mut Vec<String>,
    seen: &mut HashSet<String>,
    node_map: &BTreeMap<String, Value>,
    node_facts: &BTreeMap<String, ProjectionNodeFacts>,
    degree_map: &BTreeMap<String, i64>,
    adjacency: &BTreeMap<String, BTreeSet<String>>,
) -> Result<()> {
    let mut visited = HashSet::new();
    for node_id in node_map.keys() {
        if visited.contains(node_id) {
            continue;
        }
        let mut component = collect_component_node_ids(node_id, adjacency, &mut visited);
        if component.iter().any(|item| seen.contains(item)) {
            continue;
        }
        let mut ranked_component = Vec::with_capacity(component.len());
        for item in component.drain(..) {
            let degree = degree_map
                .get(&item)
                .copied()
                .ok_or_else(|| anyhow!("projection component degree missing for {item}"))?;
            let facts = node_facts
                .get(&item)
                .ok_or_else(|| anyhow!("projection component facts missing for {item}"))?;
            ranked_component.push((item, degree, facts.total_amount_cents));
        }
        ranked_component.sort_by(|left, right| {
            right
                .1
                .cmp(&left.1)
                .then_with(|| right.2.cmp(&left.2))
                .then_with(|| left.0.cmp(&right.0))
        });
        if let Some((leader, _, _)) = ranked_component.first() {
            if seen.insert(leader.clone()) {
                anchors.push(leader.clone());
            }
        }
    }
    Ok(())
}

fn collect_component_node_ids(
    start_node_id: &str,
    adjacency: &BTreeMap<String, BTreeSet<String>>,
    visited: &mut HashSet<String>,
) -> Vec<String> {
    let mut stack = vec![start_node_id.to_string()];
    let mut component = Vec::new();
    while let Some(current) = stack.pop() {
        if !visited.insert(current.clone()) {
            continue;
        }
        component.push(current.clone());
        if let Some(neighbors) = adjacency.get(&current) {
            for next_id in neighbors {
                if !visited.contains(next_id) {
                    stack.push(next_id.clone());
                }
            }
        }
    }
    component
}
