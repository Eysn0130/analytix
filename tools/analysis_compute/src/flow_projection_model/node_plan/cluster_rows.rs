use anyhow::{anyhow, Result};
use serde_json::Value;
use std::collections::{BTreeMap, BTreeSet};

use super::super::helpers::{row_bool, row_text};
use super::super::input_model::ProjectionNodeFacts;
use super::cluster_render_model::{
    render_collapsed_cluster_projection, render_expanded_cluster_row,
};
use super::row_model::{ClusterProjectionRow, ProjectedNodeRow};

pub(super) struct ClusterProjectionRows {
    pub(super) projected_nodes: Vec<ProjectedNodeRow>,
    pub(super) cluster_rows: Vec<ClusterProjectionRow>,
    pub(super) projected_node_count: usize,
}

pub(super) fn build_cluster_projection_rows(
    active_clusters: &[&Value],
    anchor_set: &BTreeSet<String>,
    node_map: &BTreeMap<String, Value>,
    node_facts: &BTreeMap<String, ProjectionNodeFacts>,
    use_point_layer: bool,
) -> Result<ClusterProjectionRows> {
    let mut projected_nodes = Vec::new();
    let mut cluster_rows = Vec::new();
    let mut projected_node_count = 0usize;

    let mut cluster_by_id: BTreeMap<String, &Value> = BTreeMap::new();
    for cluster in active_clusters {
        let cluster_id = row_text(cluster, "cluster_id");
        if !cluster_id.is_empty() {
            cluster_by_id.insert(cluster_id, cluster);
        }
    }

    for (cluster_id, cluster) in &cluster_by_id {
        if row_bool(cluster, "expanded")
            .ok_or_else(|| anyhow!("projection cluster is missing expanded state"))?
        {
            continue;
        }
        let Some(projection) = render_collapsed_cluster_projection(
            cluster_id,
            cluster,
            anchor_set,
            node_map,
            node_facts,
            use_point_layer,
        )?
        else {
            continue;
        };
        projected_nodes.push(projection.projected_node);
        projected_node_count += 1;
        cluster_rows.push(projection.cluster_row);
    }

    for cluster in active_clusters {
        if !row_bool(cluster, "expanded")
            .ok_or_else(|| anyhow!("projection cluster is missing expanded state"))?
        {
            continue;
        }
        cluster_rows.push(render_expanded_cluster_row(
            cluster,
            node_map,
            use_point_layer,
        )?);
    }

    Ok(ClusterProjectionRows {
        projected_nodes,
        cluster_rows,
        projected_node_count,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn cluster_expanded_state_must_be_an_explicit_boolean() {
        let missing = json!({"cluster_id": "cluster-a"});
        let missing_rows = [&missing];
        let error = build_cluster_projection_rows(
            &missing_rows,
            &BTreeSet::new(),
            &BTreeMap::new(),
            &BTreeMap::new(),
            false,
        )
        .err()
        .expect("missing expanded state must fail");
        assert!(error
            .to_string()
            .contains("projection cluster is missing expanded state"));

        let string_false = json!({"cluster_id": "cluster-a", "expanded": "false"});
        let string_rows = [&string_false];
        assert!(build_cluster_projection_rows(
            &string_rows,
            &BTreeSet::new(),
            &BTreeMap::new(),
            &BTreeMap::new(),
            false,
        )
        .is_err());
    }
}
