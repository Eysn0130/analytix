use super::input_model::ProjectionNodeFacts;
use anyhow::Result;
use serde_json::{Map, Value};
use std::collections::BTreeMap;

mod cluster_render_model;
mod cluster_rows;
mod plan_model;
mod render_plan;
mod row_model;
mod visible_render_model;
mod visible_rows;

pub(super) fn build_projected_node_plan(
    nodes: &[Value],
    request_context: Option<&Map<String, Value>>,
    anchor_ids: &[String],
    explicit_entity_ids: &[String],
    node_map: &BTreeMap<String, Value>,
    node_facts: &BTreeMap<String, ProjectionNodeFacts>,
    visibility_plan_value: &Value,
) -> Result<Value> {
    plan_model::build_projected_node_plan(
        nodes,
        request_context,
        anchor_ids,
        explicit_entity_ids,
        node_map,
        node_facts,
        visibility_plan_value,
    )
}
