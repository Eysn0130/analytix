use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Map, Value};
use std::{fs, path::PathBuf};

mod edge_offset;
mod edge_render;
mod node_render;
mod node_visibility;
mod render_hints;
mod staged_label;
mod value_helpers;
mod viewport_expand;

#[cfg(test)]
mod tests;

use value_helpers::{bool_field, clone_array, parse_unique_list, render_plan_rows, text_value};

pub(crate) struct GraphRenderPlanArgs {
    pub(crate) payload: Value,
    pub(crate) output_json: Option<PathBuf>,
}

pub(crate) fn parse_args(mut iter: impl Iterator<Item = String>) -> Result<GraphRenderPlanArgs> {
    let mut payload: Option<Value> = None;
    let mut output_json = None;

    while let Some(flag) = iter.next() {
        match flag.as_str() {
            "--payload-json" => {
                let raw_payload = crate::required_value(&mut iter, "--payload-json")?;
                payload = Some(
                    serde_json::from_str(&raw_payload)
                        .with_context(|| "parse --payload-json as JSON")?,
                );
            }
            "--input-path" => {
                let input_path = PathBuf::from(crate::required_value(&mut iter, "--input-path")?);
                let raw_payload = fs::read_to_string(&input_path).with_context(|| {
                    format!("read graph render contract input {}", input_path.display())
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!("parse graph render contract input {}", input_path.display())
                })?);
            }
            "--output-json" => {
                output_json = Some(PathBuf::from(crate::required_value(
                    &mut iter,
                    "--output-json",
                )?));
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute project-flow-graph-render-plan <--payload-json <json>|--input-path <path>> [--output-json <path>]");
            }
            other => bail!("unsupported graph render contract flag: {other}"),
        }
    }

    Ok(GraphRenderPlanArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
        output_json,
    })
}

pub(crate) fn project_graph_render_plan(payload: &Value) -> Value {
    let render_results = render_plan_rows(payload, "renderPlans", "renderPlan")
        .iter()
        .map(project_render_plan)
        .collect::<Vec<_>>();

    let mut result = Map::new();
    result.insert("renderResults".to_string(), json!(render_results));
    if let Some(contract) = payload
        .get("viewportExpandTargets")
        .or_else(|| payload.get("viewport_expand_targets"))
    {
        result.insert(
            "viewportExpandTargets".to_string(),
            viewport_expand::project_viewport_expand_targets(contract),
        );
    }
    Value::Object(result)
}

fn project_render_plan(contract: &Value) -> Value {
    let nodes = clone_array(contract.get("nodes"));
    let edges = clone_array(contract.get("edges"));
    let ctx = contract.get("ctx").unwrap_or(&Value::Null);
    let raw_hints = contract.get("hints").unwrap_or(&Value::Null);
    let normalized_hints = render_hints::normalize_render_plan_hints(
        contract,
        ctx,
        raw_hints,
        nodes.len(),
        edges.len(),
    );
    let hints = &normalized_hints;
    let preserve_edge_labels = bool_field(
        contract
            .get("preserveEdgeLabels")
            .or_else(|| contract.get("preserve_edge_labels")),
    );
    let graph_style = contract
        .get("graphStyle")
        .or_else(|| contract.get("graph_style"))
        .unwrap_or(&Value::Null);
    let node_facts = node_visibility::collect_render_node_facts(&nodes);
    let staged_focus_ids = parse_unique_list(
        contract
            .get("stagedFocusIds")
            .or_else(|| contract.get("staged_focus_ids")),
        true,
    );
    let topology = node_visibility::GraphTopologyIndex::from_edges(&edges);
    let ranking = node_visibility::build_node_visibility_ranking(
        &node_facts,
        &topology,
        ctx,
        &staged_focus_ids,
    );
    let summary = node_visibility::project_graph_tier_render_plan(
        nodes.len(),
        &node_facts,
        ctx,
        hints,
        &ranking,
    );
    let edge_render_updates =
        edge_render::project_edge_render_updates(&edges, hints, preserve_edge_labels);
    let node_render_updates =
        node_render::project_node_render_updates(&nodes, &summary.node_mode_rows, graph_style);
    let render_plan = json!({
        "summary": summary.summary.clone(),
        "normalizedRenderHints": normalized_hints.clone(),
        "nodeRenderModes": summary.node_render_modes.clone(),
        "nodeRenderUpdates": node_render_updates.clone(),
        "edgeRenderUpdates": add_render_update_indexes(&edge_render_updates),
    });
    let staged_label_plan = staged_label::project_staged_label_plan(
        contract,
        &nodes,
        &node_facts,
        &edges,
        &topology,
        ctx,
        hints,
        &summary.node_mode_rows,
        &node_render_updates,
        &edge_render_updates,
        &staged_focus_ids,
    );

    let mut result = Map::new();
    result.insert("name".to_string(), json!(text_value(contract.get("name"))));
    result.insert(
        "normalizedRenderHints".to_string(),
        normalized_hints.clone(),
    );
    result.insert("summary".to_string(), summary.summary);
    result.insert(
        "nodeRenderModes".to_string(),
        json!(summary.node_render_modes),
    );
    result.insert("edgeRenderUpdates".to_string(), json!(edge_render_updates));
    result.insert(
        "edgeOffsetUpdates".to_string(),
        json!(edge_offset::project_edge_offset_updates(&edges)),
    );
    if !staged_label_plan.is_null() {
        result.insert("stagedLabelPlan".to_string(), staged_label_plan);
    }
    result.insert("renderPlan".to_string(), render_plan);
    result.insert(
        "ranking".to_string(),
        json!(ranking
            .entries
            .iter()
            .map(node_visibility::render_ranking_entry)
            .collect::<Vec<_>>()),
    );
    result.insert(
        "focusIds".to_string(),
        json!(ranking.focus_ids.iter().cloned().collect::<Vec<_>>()),
    );
    Value::Object(result)
}

fn add_render_update_indexes(updates: &[Value]) -> Vec<Value> {
    updates
        .iter()
        .enumerate()
        .map(|(index, update)| {
            let mut object = update.as_object().cloned().unwrap_or_default();
            object.insert("index".to_string(), json!(index));
            Value::Object(object)
        })
        .collect()
}
