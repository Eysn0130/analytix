use serde_json::Value;
use std::collections::{BTreeMap, BTreeSet};

use super::row_model::ProjectedNodeRow;
use super::visible_render_model::{render_visible_projected_node, VisibleProjectedNodeKind};

pub(super) struct ProjectedNodeRows {
    pub(super) nodes: Vec<ProjectedNodeRow>,
    pub(super) dot_count: usize,
    pub(super) entity_count: usize,
}

pub(super) fn build_visible_projected_nodes(
    nodes: &[Value],
    anchor_set: &BTreeSet<String>,
    collapsed_member_to_cluster: &BTreeMap<String, String>,
    use_point_layer: bool,
) -> ProjectedNodeRows {
    let mut projected_nodes = Vec::new();
    let mut dot_count = 0usize;
    let mut entity_count = 0usize;

    for row in nodes {
        let Some(projected_node) = render_visible_projected_node(
            row,
            anchor_set,
            collapsed_member_to_cluster,
            use_point_layer,
        ) else {
            continue;
        };
        match projected_node.kind {
            VisibleProjectedNodeKind::Entity => entity_count += 1,
            VisibleProjectedNodeKind::Dot => dot_count += 1,
        }
        projected_nodes.push(projected_node.row);
    }

    ProjectedNodeRows {
        nodes: projected_nodes,
        dot_count,
        entity_count,
    }
}
