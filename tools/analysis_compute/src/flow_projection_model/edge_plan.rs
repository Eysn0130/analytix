use anyhow::Result;
use serde_json::{Map, Value};

use super::input_model::ProjectionEdge;
use super::visibility_plan;

mod aggregation;
mod plan_model;
mod render_model;

use aggregation::aggregate_projected_edges;
use plan_model::{projected_edge_view_mode, render_projected_edge_plan};

pub(super) fn build_projected_edge_plan(
    edges: &[ProjectionEdge],
    request_context: Option<&Map<String, Value>>,
    visibility_plan_value: &Value,
) -> Result<Value> {
    let collapsed_member_to_cluster =
        visibility_plan::collapsed_member_to_cluster(visibility_plan_value)?;
    let aggregation = aggregate_projected_edges(edges, &collapsed_member_to_cluster)?;
    let view_mode = projected_edge_view_mode(request_context);

    render_projected_edge_plan(aggregation, view_mode)
}
