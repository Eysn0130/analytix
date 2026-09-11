use anyhow::Result;
use serde_json::{json, Value};
use std::collections::BTreeSet;

use super::super::visibility_plan;
use super::row_model::{ClusterProjectionRow, ProjectedNodeRow};
use super::{cluster_rows::ClusterProjectionRows, visible_rows::ProjectedNodeRows};

pub(super) struct ProjectedNodeRenderPlan {
    nodes: Vec<ProjectedNodeRow>,
    clusters: Vec<ClusterProjectionRow>,
    dot_count: usize,
    entity_count: usize,
}

impl ProjectedNodeRenderPlan {
    pub(super) fn from_visible_rows(node_rows: ProjectedNodeRows) -> Self {
        Self {
            nodes: node_rows.nodes,
            clusters: Vec::new(),
            dot_count: node_rows.dot_count,
            entity_count: node_rows.entity_count,
        }
    }

    pub(super) fn append_cluster_rows(&mut self, mut cluster_plan: ClusterProjectionRows) {
        self.nodes.append(&mut cluster_plan.projected_nodes);
        self.clusters.append(&mut cluster_plan.cluster_rows);
        self.entity_count += cluster_plan.projected_node_count;
    }

    pub(super) fn render(
        self,
        explicit_set: &BTreeSet<String>,
        anchor_set: &BTreeSet<String>,
        visibility_plan_value: &Value,
    ) -> Result<Value> {
        let expanded_node_ids: Vec<String> = explicit_set.difference(anchor_set).cloned().collect();
        Ok(json!({
            "nodes": ProjectedNodeRow::into_values(self.nodes),
            "clusters": ClusterProjectionRow::into_values(self.clusters),
            "dot_count": self.dot_count,
            "entity_count": self.entity_count,
            "expanded_cluster_ids": visibility_plan::text_list(visibility_plan_value, "expanded_cluster_ids")?,
            "expanded_tile_ids": visibility_plan::text_list(visibility_plan_value, "expanded_tile_ids")?,
            "expanded_node_ids": expanded_node_ids,
            "point_layer_total": visibility_plan::usize_field(visibility_plan_value, "point_layer_total")?,
            "point_layer_buckets": visibility_plan::array(visibility_plan_value, "point_layer_buckets")?,
        }))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn render_plan_appends_cluster_nodes_and_preserves_counts() {
        let mut render_plan = ProjectedNodeRenderPlan::from_visible_rows(ProjectedNodeRows {
            nodes: vec![ProjectedNodeRow::from_object_value(
                json!({"id": "visible"}),
            )],
            dot_count: 2,
            entity_count: 3,
        });
        render_plan.append_cluster_rows(ClusterProjectionRows {
            projected_nodes: vec![ProjectedNodeRow::from_object_value(
                json!({"id": "cluster-node"}),
            )],
            cluster_rows: vec![ClusterProjectionRow::from_object_value(
                json!({"cluster_id": "cluster-a"}),
            )],
            projected_node_count: 1,
        });

        let explicit_set = BTreeSet::from(["anchor".to_string(), "expanded".to_string()]);
        let anchor_set = BTreeSet::from(["anchor".to_string()]);
        let visibility_plan_value = json!({
            "expanded_cluster_ids": ["cluster-a"],
            "expanded_tile_ids": ["tile-a"],
            "point_layer_total": 7,
            "point_layer_buckets": [{"bucket": "b1"}],
        });

        assert_eq!(
            render_plan
                .render(&explicit_set, &anchor_set, &visibility_plan_value)
                .unwrap(),
            json!({
                "nodes": [{"id": "visible"}, {"id": "cluster-node"}],
                "clusters": [{"cluster_id": "cluster-a"}],
                "dot_count": 2,
                "entity_count": 4,
                "expanded_cluster_ids": ["cluster-a"],
                "expanded_tile_ids": ["tile-a"],
                "expanded_node_ids": ["expanded"],
                "point_layer_total": 7,
                "point_layer_buckets": [{"bucket": "b1"}],
            })
        );
    }
}
