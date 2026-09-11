use serde_json::Value;
use std::collections::{BTreeMap, BTreeSet};

use super::super::helpers::row_text;
use super::row_model::ProjectedNodeRow;

#[derive(Debug, Eq, PartialEq)]
pub(super) enum VisibleProjectedNodeKind {
    Entity,
    Dot,
}

pub(super) struct VisibleProjectedNode {
    pub(super) row: ProjectedNodeRow,
    pub(super) kind: VisibleProjectedNodeKind,
}

pub(super) fn render_visible_projected_node(
    row: &Value,
    anchor_set: &BTreeSet<String>,
    collapsed_member_to_cluster: &BTreeMap<String, String>,
    use_point_layer: bool,
) -> Option<VisibleProjectedNode> {
    let node_id = row_text(row, "id");
    if node_id.is_empty() {
        return None;
    }
    let cluster_id = collapsed_member_to_cluster
        .get(&node_id)
        .cloned()
        .unwrap_or_default();
    if use_point_layer && !cluster_id.is_empty() {
        return None;
    }

    let mut next_row = row.as_object().cloned().unwrap_or_default();
    next_row.insert(
        "projection_cluster_id".to_string(),
        Value::String(cluster_id.clone()),
    );
    next_row.insert("projection_visible".to_string(), Value::Bool(true));
    if cluster_id.is_empty() {
        next_row.insert(
            "nodeRenderMode".to_string(),
            Value::String(if anchor_set.contains(&node_id) {
                "focus".to_string()
            } else {
                "entity".to_string()
            }),
        );
        next_row.insert("projection_collapsed".to_string(), Value::Bool(false));
        return Some(VisibleProjectedNode {
            row: ProjectedNodeRow::from_object_value(Value::Object(next_row)),
            kind: VisibleProjectedNodeKind::Entity,
        });
    }

    next_row.insert(
        "nodeRenderMode".to_string(),
        Value::String("dot".to_string()),
    );
    next_row.insert("nodeLabelHidden".to_string(), Value::Bool(true));
    next_row.insert("projection_collapsed".to_string(), Value::Bool(true));
    Some(VisibleProjectedNode {
        row: ProjectedNodeRow::from_object_value(Value::Object(next_row)),
        kind: VisibleProjectedNodeKind::Dot,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn render_visible_projected_node_marks_anchor_entities_and_regular_entities() {
        let anchor_set = BTreeSet::from(["seed".to_string()]);
        let collapsed = BTreeMap::new();

        let seed = render_visible_projected_node(
            &json!({"id": "seed", "name": "主账户"}),
            &anchor_set,
            &collapsed,
            false,
        )
        .unwrap();
        assert_eq!(seed.kind, VisibleProjectedNodeKind::Entity);
        assert_eq!(
            seed.row.into_value(),
            json!({
                "id": "seed",
                "name": "主账户",
                "projection_cluster_id": "",
                "projection_visible": true,
                "nodeRenderMode": "focus",
                "projection_collapsed": false,
            })
        );

        let regular = render_visible_projected_node(
            &json!({"id": "counterparty"}),
            &anchor_set,
            &collapsed,
            false,
        )
        .unwrap();
        assert_eq!(regular.kind, VisibleProjectedNodeKind::Entity);
        assert_eq!(regular.row.into_value()["nodeRenderMode"], "entity");
    }

    #[test]
    fn render_visible_projected_node_marks_collapsed_nodes_as_dots_or_hides_point_layer() {
        let anchor_set = BTreeSet::new();
        let collapsed = BTreeMap::from([("leaf".to_string(), "cluster-a".to_string())]);

        let dot = render_visible_projected_node(
            &json!({"id": "leaf", "title": "叶子"}),
            &anchor_set,
            &collapsed,
            false,
        )
        .unwrap();
        assert_eq!(dot.kind, VisibleProjectedNodeKind::Dot);
        assert_eq!(
            dot.row.into_value(),
            json!({
                "id": "leaf",
                "title": "叶子",
                "projection_cluster_id": "cluster-a",
                "projection_visible": true,
                "nodeRenderMode": "dot",
                "nodeLabelHidden": true,
                "projection_collapsed": true,
            })
        );

        assert!(render_visible_projected_node(
            &json!({"id": "leaf"}),
            &anchor_set,
            &collapsed,
            true,
        )
        .is_none());
    }

    #[test]
    fn render_visible_projected_node_skips_empty_ids() {
        assert!(render_visible_projected_node(
            &json!({"id": ""}),
            &BTreeSet::new(),
            &BTreeMap::new(),
            false,
        )
        .is_none());
    }
}
