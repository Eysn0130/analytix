use anyhow::{anyhow, Result};
use serde_json::{json, Value};
use std::collections::{BTreeMap, BTreeSet};

use super::super::helpers::{row_bool, row_text, row_text_list, sum_projection_node_totals};
use super::super::input_model::{money_value, ProjectionNodeFacts};
use super::row_model::{ClusterProjectionRow, ProjectedNodeRow};

pub(super) struct CollapsedClusterProjection {
    pub(super) projected_node: ProjectedNodeRow,
    pub(super) cluster_row: ClusterProjectionRow,
}

pub(super) fn render_collapsed_cluster_projection(
    cluster_id: &str,
    cluster: &Value,
    anchor_set: &BTreeSet<String>,
    node_map: &BTreeMap<String, Value>,
    node_facts: &BTreeMap<String, ProjectionNodeFacts>,
    use_point_layer: bool,
) -> Result<Option<CollapsedClusterProjection>> {
    let member_ids = known_cluster_node_ids(cluster, "collapsed_member_ids", node_map)?;
    if member_ids.is_empty() {
        return Ok(None);
    }
    let total_member_ids = known_cluster_node_ids(cluster, "member_ids", node_map)?;
    let anchor_id = row_text(cluster, "anchor_id");
    let anchor_title = projection_anchor_title(node_map.get(&anchor_id), &anchor_id);
    let totals = sum_projection_node_totals(&member_ids, node_facts)?;
    let total_member_count = total_member_ids.len();
    let materialized_member_count = total_member_count.saturating_sub(member_ids.len());
    let title = cluster_projection_title(
        &anchor_title,
        member_ids.len(),
        total_member_count,
        materialized_member_count,
    );

    let expanded_tile_ids = row_text_list(cluster, "expanded_tile_ids");
    let expanded_tile_count = expanded_tile_ids.len();
    let remaining_tile_ids = row_text_list(cluster, "remaining_tile_ids");
    let remaining_tile_count = remaining_tile_ids.len();
    let next_tile_ids = remaining_tile_ids
        .iter()
        .take(3)
        .cloned()
        .collect::<Vec<_>>();
    let visible_member_ids = if use_point_layer {
        Vec::new()
    } else {
        total_member_ids.clone()
    };
    let visible_remaining_member_ids = if use_point_layer {
        Vec::new()
    } else {
        member_ids.clone()
    };

    Ok(Some(CollapsedClusterProjection {
        projected_node: ProjectedNodeRow::from_object_value(json!({
            "id": cluster_id,
            "title": title,
            "name": title,
            "display_id": title,
            "display_ids": member_ids.iter().take(24).cloned().collect::<Vec<_>>(),
            "ntype": "cluster",
            "total_amount": money_value(totals.total_amount_cents),
            "total_count": totals.total_count,
            "nodeRenderMode": if anchor_set.contains(&anchor_id) { "focus" } else { "entity" },
            "cluster_node": true,
            "cluster_member_count": member_ids.len(),
            "cluster_total_member_count": total_member_count,
            "cluster_materialized_member_count": materialized_member_count,
            "cluster_anchor_id": anchor_id,
            "projection_visible": true,
        })),
        cluster_row: ClusterProjectionRow::from_object_value(json!({
            "cluster_id": cluster_id,
            "anchor_id": anchor_id,
            "anchor_ids": row_text_list(cluster, "anchor_ids"),
            "member_ids": visible_member_ids,
            "remaining_member_ids": visible_remaining_member_ids,
            "member_count": total_member_count,
            "remaining_member_count": member_ids.len(),
            "materialized_member_count": materialized_member_count,
            "expanded": false,
            "partially_expanded": row_bool(cluster, "partially_expanded")
                .ok_or_else(|| anyhow!("projection cluster is missing partially_expanded state"))?,
            "tile_count": row_text_list(cluster, "tile_ids").len(),
            "expanded_tile_ids": expanded_tile_ids,
            "expanded_tile_count": expanded_tile_count,
            "remaining_tile_ids": remaining_tile_ids,
            "remaining_tile_count": remaining_tile_count,
            "next_tile_ids": next_tile_ids,
            "title": title,
            "total_amount": money_value(totals.total_amount_cents),
            "total_count": totals.total_count,
        })),
    }))
}

pub(super) fn render_expanded_cluster_row(
    cluster: &Value,
    node_map: &BTreeMap<String, Value>,
    use_point_layer: bool,
) -> Result<ClusterProjectionRow> {
    let member_ids = known_cluster_node_ids(cluster, "member_ids", node_map)?;
    let tile_ids = row_text_list(cluster, "tile_ids");
    let tile_count = tile_ids.len();
    let visible_member_ids = if use_point_layer {
        Vec::new()
    } else {
        member_ids.clone()
    };
    Ok(ClusterProjectionRow::from_object_value(json!({
        "cluster_id": row_text(cluster, "cluster_id"),
        "anchor_id": row_text(cluster, "anchor_id"),
        "anchor_ids": row_text_list(cluster, "anchor_ids"),
        "member_ids": visible_member_ids,
        "member_count": member_ids.len(),
        "remaining_member_count": 0,
        "materialized_member_count": member_ids.len(),
        "expanded": true,
        "partially_expanded": false,
        "tile_count": tile_count,
        "expanded_tile_ids": tile_ids,
        "expanded_tile_count": tile_count,
        "remaining_tile_ids": Vec::<String>::new(),
        "remaining_tile_count": 0,
        "next_tile_ids": Vec::<String>::new(),
    })))
}

fn known_cluster_node_ids(
    cluster: &Value,
    field: &str,
    node_map: &BTreeMap<String, Value>,
) -> Result<Vec<String>> {
    let values = cluster
        .as_object()
        .and_then(|row| row.get(field))
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
                return Err(anyhow!(
                    "projection cluster field {field}[{index}] references unknown node {node_id}"
                ));
            }
            Ok(node_id.to_string())
        })
        .collect()
}

fn cluster_projection_title(
    anchor_title: &str,
    member_count: usize,
    total_member_count: usize,
    materialized_member_count: usize,
) -> String {
    if materialized_member_count > 0 && total_member_count > member_count {
        format!("{anchor_title} 关联簇 ({member_count}/{total_member_count})")
    } else {
        format!("{anchor_title} 关联簇 ({member_count})")
    }
}

fn projection_anchor_title(anchor_row: Option<&Value>, anchor_id: &str) -> String {
    let Some(anchor_row) = anchor_row else {
        return if anchor_id.is_empty() {
            "聚类".to_string()
        } else {
            anchor_id.to_string()
        };
    };
    for key in ["name", "title", "display_id"] {
        let text = row_text(anchor_row, key);
        if !text.is_empty() {
            return text;
        }
    }
    if anchor_id.is_empty() {
        "聚类".to_string()
    } else {
        anchor_id.to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn node_map() -> BTreeMap<String, Value> {
        BTreeMap::from([
            (
                "anchor".to_string(),
                json!({"id": "anchor", "name": "主账户", "total_amount": 0.0, "total_count": 0}),
            ),
            (
                "m1".to_string(),
                json!({"id": "m1", "total_amount": 10.0, "total_count": 1}),
            ),
            (
                "m2".to_string(),
                json!({"id": "m2", "total_amount": 2.5, "total_count": 2}),
            ),
            (
                "m3".to_string(),
                json!({"id": "m3", "total_amount": 3.0, "total_count": 1}),
            ),
        ])
    }

    fn node_facts() -> BTreeMap<String, ProjectionNodeFacts> {
        BTreeMap::from([
            (
                "anchor".to_string(),
                ProjectionNodeFacts {
                    total_amount_cents: 0,
                    total_count: 0,
                },
            ),
            (
                "m1".to_string(),
                ProjectionNodeFacts {
                    total_amount_cents: 1_000,
                    total_count: 1,
                },
            ),
            (
                "m2".to_string(),
                ProjectionNodeFacts {
                    total_amount_cents: 250,
                    total_count: 2,
                },
            ),
            (
                "m3".to_string(),
                ProjectionNodeFacts {
                    total_amount_cents: 300,
                    total_count: 1,
                },
            ),
        ])
    }

    #[test]
    fn collapsed_cluster_projection_renders_virtual_node_and_cluster_row() {
        let cluster = json!({
            "cluster_id": "cluster-a",
            "anchor_id": "anchor",
            "anchor_ids": ["anchor"],
            "collapsed_member_ids": ["m1", "m2"],
            "member_ids": ["m1", "m2", "m3"],
            "partially_expanded": true,
            "tile_ids": ["tile-1", "tile-2"],
            "expanded_tile_ids": ["tile-1"],
            "remaining_tile_ids": ["tile-2", "tile-3", "tile-4", "tile-5"],
        });

        let projection = render_collapsed_cluster_projection(
            "cluster-a",
            &cluster,
            &BTreeSet::from(["anchor".to_string()]),
            &node_map(),
            &node_facts(),
            false,
        )
        .unwrap()
        .unwrap();

        assert_eq!(
            projection.projected_node.into_value(),
            json!({
                "id": "cluster-a",
                "title": "主账户 关联簇 (2/3)",
                "name": "主账户 关联簇 (2/3)",
                "display_id": "主账户 关联簇 (2/3)",
                "display_ids": ["m1", "m2"],
                "ntype": "cluster",
                "total_amount": 12.5,
                "total_count": 3,
                "nodeRenderMode": "focus",
                "cluster_node": true,
                "cluster_member_count": 2,
                "cluster_total_member_count": 3,
                "cluster_materialized_member_count": 1,
                "cluster_anchor_id": "anchor",
                "projection_visible": true,
            })
        );
        assert_eq!(
            projection.cluster_row.into_value(),
            json!({
                "cluster_id": "cluster-a",
                "anchor_id": "anchor",
                "anchor_ids": ["anchor"],
                "member_ids": ["m1", "m2", "m3"],
                "remaining_member_ids": ["m1", "m2"],
                "member_count": 3,
                "remaining_member_count": 2,
                "materialized_member_count": 1,
                "expanded": false,
                "partially_expanded": true,
                "tile_count": 2,
                "expanded_tile_ids": ["tile-1"],
                "expanded_tile_count": 1,
                "remaining_tile_ids": ["tile-2", "tile-3", "tile-4", "tile-5"],
                "remaining_tile_count": 4,
                "next_tile_ids": ["tile-2", "tile-3", "tile-4"],
                "title": "主账户 关联簇 (2/3)",
                "total_amount": 12.5,
                "total_count": 3,
            })
        );
    }

    #[test]
    fn collapsed_cluster_projection_redacts_member_lists_for_point_layer() {
        let cluster = json!({
            "anchor_id": "anchor",
            "collapsed_member_ids": ["m1"],
            "member_ids": ["m1", "m2"],
            "partially_expanded": false,
        });

        let projection = render_collapsed_cluster_projection(
            "cluster-a",
            &cluster,
            &BTreeSet::new(),
            &node_map(),
            &node_facts(),
            true,
        )
        .unwrap()
        .unwrap();

        let row = projection.cluster_row.into_value();
        assert_eq!(row["member_ids"], json!([]));
        assert_eq!(row["remaining_member_ids"], json!([]));
        assert_eq!(row["member_count"], 2);
        assert_eq!(row["remaining_member_count"], 1);
    }

    #[test]
    fn collapsed_cluster_projection_rejects_unknown_members() {
        let cluster = json!({"collapsed_member_ids": ["missing"], "member_ids": ["missing"]});
        assert!(render_collapsed_cluster_projection(
            "cluster-a",
            &cluster,
            &BTreeSet::new(),
            &node_map(),
            &node_facts(),
            false,
        )
        .is_err());
    }

    #[test]
    fn expanded_cluster_row_keeps_counts_and_redacts_members_for_point_layer() {
        let cluster = json!({
            "cluster_id": "cluster-a",
            "anchor_id": "anchor",
            "anchor_ids": ["anchor"],
            "member_ids": ["m1", "m2"],
            "tile_ids": ["tile-1", "tile-2"],
        });

        assert_eq!(
            render_expanded_cluster_row(&cluster, &node_map(), false)
                .unwrap()
                .into_value(),
            json!({
                "cluster_id": "cluster-a",
                "anchor_id": "anchor",
                "anchor_ids": ["anchor"],
                "member_ids": ["m1", "m2"],
                "member_count": 2,
                "remaining_member_count": 0,
                "materialized_member_count": 2,
                "expanded": true,
                "partially_expanded": false,
                "tile_count": 2,
                "expanded_tile_ids": ["tile-1", "tile-2"],
                "expanded_tile_count": 2,
                "remaining_tile_ids": [],
                "remaining_tile_count": 0,
                "next_tile_ids": [],
            })
        );
        assert_eq!(
            render_expanded_cluster_row(&cluster, &node_map(), true)
                .unwrap()
                .into_value()["member_ids"],
            json!([])
        );

        let stale_cluster = json!({"member_ids": ["missing"], "tile_ids": []});
        assert!(render_expanded_cluster_row(&stale_cluster, &node_map(), false).is_err());
    }
}
