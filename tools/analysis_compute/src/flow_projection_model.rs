use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Value};
use std::fs;
use std::path::PathBuf;

mod cluster_model;
mod edge_plan;
mod helpers;
mod input_model;
mod materialization_plan;
mod node_plan;
mod visibility_plan;

use cluster_model::build_projection_clusters;
use edge_plan::build_projected_edge_plan;
use helpers::{build_runtime_degree_map, to_text_list};
use input_model::validate_projection_input;
use materialization_plan::{
    build_projection_explicit_entity_ids, build_requested_cluster_materializations,
    build_search_match_node_ids,
};
use node_plan::build_projected_node_plan;
use visibility_plan::build_cluster_visibility_plan;

pub(crate) struct FlowProjectionArgs {
    input_path: PathBuf,
}

pub(crate) fn parse_args(iter: impl Iterator<Item = String>) -> Result<FlowProjectionArgs> {
    let mut input_path: Option<PathBuf> = None;
    let mut args = iter.peekable();
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--input-path" => {
                input_path = Some(PathBuf::from(crate::required_value(
                    &mut args,
                    "--input-path",
                )?))
            }
            other => bail!("unknown project-flow-skeleton-clusters arg: {other}"),
        }
    }
    Ok(FlowProjectionArgs {
        input_path: input_path.ok_or_else(|| anyhow!("--input-path is required"))?,
    })
}

pub(crate) fn project_flow_skeleton_clusters(args: &FlowProjectionArgs) -> Result<Value> {
    let raw = fs::read_to_string(&args.input_path)
        .with_context(|| format!("read projection input {}", args.input_path.display()))?;
    let payload: Value = serde_json::from_str(&raw).context("parse projection input JSON")?;
    project_projection_model(&payload)
}

fn project_projection_model(payload: &Value) -> Result<Value> {
    let input = validate_projection_input(payload)?;
    let nodes = input.nodes.as_slice();
    let edges = input.edges.as_slice();
    let request_context = input.request_context.as_ref();
    let base_projection = input.base_projection.as_ref();
    let node_map = &input.node_map;
    let node_facts = &input.node_facts;
    let tile_size = input.tile_size;
    let (degree_map, adjacency) = build_runtime_degree_map(node_map, edges)?;
    let mut cluster_state = build_projection_clusters(
        nodes,
        request_context,
        base_projection,
        node_map,
        node_facts,
        &degree_map,
        &adjacency,
        tile_size,
    )?;
    let search_match_node_ids = build_search_match_node_ids(nodes, request_context, node_facts)?;
    cluster_state.insert(
        "search_match_node_ids".to_string(),
        json!(search_match_node_ids.clone()),
    );
    let anchor_ids = to_text_list(cluster_state.get("anchor_ids"));
    let explicit_state = build_projection_explicit_entity_ids(
        &anchor_ids,
        request_context,
        base_projection,
        &search_match_node_ids,
        node_map,
        &adjacency,
    );
    let clusters = cluster_state
        .get("clusters")
        .and_then(Value::as_array)
        .ok_or_else(|| anyhow!("projection cluster state is missing clusters"))?;
    let requested_cluster_materializations = build_requested_cluster_materializations(
        clusters,
        request_context,
        &anchor_ids,
        &explicit_state.base_projection_node_ids,
        &explicit_state.explicit_entity_ids,
        node_map,
        node_facts,
        tile_size,
    )?;
    let cluster_visibility_plan = build_cluster_visibility_plan(
        clusters,
        request_context,
        &explicit_state.explicit_entity_ids,
        node_map,
        node_facts,
        &requested_cluster_materializations,
    )?;
    let projected_node_plan = build_projected_node_plan(
        nodes,
        request_context,
        &anchor_ids,
        &explicit_state.explicit_entity_ids,
        node_map,
        node_facts,
        &cluster_visibility_plan,
    )?;
    let projected_edge_plan =
        build_projected_edge_plan(edges, request_context, &cluster_visibility_plan)?;
    cluster_state.insert(
        "base_projection_node_ids".to_string(),
        json!(explicit_state.base_projection_node_ids),
    );
    cluster_state.insert(
        "path_entity_ids".to_string(),
        json!(explicit_state.path_entity_ids),
    );
    cluster_state.insert(
        "explicit_entity_ids".to_string(),
        json!(explicit_state.explicit_entity_ids),
    );
    cluster_state.insert(
        "requested_cluster_materializations".to_string(),
        json!(requested_cluster_materializations),
    );
    cluster_state.insert(
        "cluster_visibility_plan".to_string(),
        cluster_visibility_plan,
    );
    cluster_state.insert("projected_node_plan".to_string(), projected_node_plan);
    cluster_state.insert("projected_edge_plan".to_string(), projected_edge_plan);
    Ok(Value::Object(cluster_state))
}

#[cfg(test)]
mod tests;
