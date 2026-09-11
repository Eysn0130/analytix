use anyhow::{anyhow, Result};
use serde_json::{Map, Value};
use std::collections::{BTreeMap, BTreeSet};

use super::super::{
    helpers::{projection_use_point_layer, row_text_list},
    input_model::ProjectionNodeFacts,
    visibility_plan,
};
use super::{
    cluster_rows::build_cluster_projection_rows, render_plan::ProjectedNodeRenderPlan,
    visible_rows::build_visible_projected_nodes,
};

pub(super) fn build_projected_node_plan(
    nodes: &[Value],
    request_context: Option<&Map<String, Value>>,
    anchor_ids: &[String],
    explicit_entity_ids: &[String],
    node_map: &BTreeMap<String, Value>,
    node_facts: &BTreeMap<String, ProjectionNodeFacts>,
    visibility_plan_value: &Value,
) -> Result<Value> {
    let use_point_layer = projection_use_point_layer(request_context);
    let anchor_set: BTreeSet<String> = anchor_ids.iter().cloned().collect();
    let collapsed_member_to_cluster =
        visibility_plan::collapsed_member_to_cluster(visibility_plan_value)?;
    let active_clusters = active_projection_clusters(visibility_plan_value)?;

    let explicit_set =
        expanded_entity_set(explicit_entity_ids, &active_clusters, node_map, &anchor_set)?;

    let node_rows = build_visible_projected_nodes(
        nodes,
        &anchor_set,
        &collapsed_member_to_cluster,
        use_point_layer,
    );
    let cluster_plan = build_cluster_projection_rows(
        &active_clusters,
        &anchor_set,
        node_map,
        node_facts,
        use_point_layer,
    )?;
    let mut render_plan = ProjectedNodeRenderPlan::from_visible_rows(node_rows);
    render_plan.append_cluster_rows(cluster_plan);
    render_plan.render(&explicit_set, &anchor_set, visibility_plan_value)
}

fn active_projection_clusters(visibility_plan_value: &Value) -> Result<Vec<&Value>> {
    visibility_plan_value
        .as_object()
        .and_then(|row| row.get("clusters"))
        .and_then(Value::as_array)
        .map(|items| items.iter().collect())
        .ok_or_else(|| anyhow!("cluster visibility plan is missing clusters"))
}

fn expanded_entity_set(
    explicit_entity_ids: &[String],
    active_clusters: &[&Value],
    node_map: &BTreeMap<String, Value>,
    anchor_set: &BTreeSet<String>,
) -> Result<BTreeSet<String>> {
    let mut explicit_set: BTreeSet<String> = explicit_entity_ids
        .iter()
        .filter(|node_id| node_map.contains_key(*node_id))
        .cloned()
        .collect();
    for cluster in active_clusters {
        for node_id in row_text_list(cluster, "materialized_member_ids") {
            if !node_map.contains_key(&node_id) {
                return Err(anyhow!(
                    "projection materialized member references unknown node {node_id}"
                ));
            }
            if !anchor_set.contains(&node_id) {
                explicit_set.insert(node_id);
            }
        }
    }
    Ok(explicit_set)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn expanded_entity_set_keeps_known_explicit_and_materialized_non_anchor_nodes() {
        let node_map = BTreeMap::from([
            ("anchor".to_string(), json!({"id": "anchor"})),
            ("explicit".to_string(), json!({"id": "explicit"})),
            ("materialized".to_string(), json!({"id": "materialized"})),
        ]);
        let anchor_set = BTreeSet::from(["anchor".to_string()]);
        let clusters = vec![json!({
            "materialized_member_ids": ["materialized", "anchor"],
        })];
        let active_clusters: Vec<&Value> = clusters.iter().collect();

        assert_eq!(
            expanded_entity_set(
                &["explicit".to_string(), "missing".to_string()],
                &active_clusters,
                &node_map,
                &anchor_set,
            )
            .unwrap(),
            BTreeSet::from(["explicit".to_string(), "materialized".to_string()])
        );

        let stale_clusters = vec![json!({"materialized_member_ids": ["missing"]})];
        let stale_active_clusters: Vec<&Value> = stale_clusters.iter().collect();
        assert!(expanded_entity_set(&[], &stale_active_clusters, &node_map, &anchor_set,).is_err());
    }
}
