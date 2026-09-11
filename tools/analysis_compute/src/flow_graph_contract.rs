use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Value};
use std::{fs, path::PathBuf};

pub(crate) struct GraphContractArgs {
    pub(crate) payload: Value,
}

pub(crate) fn parse_args(mut iter: impl Iterator<Item = String>) -> Result<GraphContractArgs> {
    let mut payload: Option<Value> = None;

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
                    format!("read graph contract input {}", input_path.display())
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!("parse graph contract input {}", input_path.display())
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute contract-flow-graph-workflow <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported graph contract flag: {other}"),
        }
    }

    Ok(GraphContractArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_graph_workflow_contract(payload: &Value) -> Result<Value> {
    let base_graph = crate::flow_graph_data_merge_contract::clone_graph_data(
        payload.get("baseGraphData").unwrap_or(&Value::Null),
    );
    let delta_patch = payload.get("deltaPatch").unwrap_or(&Value::Null);
    let patch_result = crate::flow_graph_data_merge_contract::build_graph_data_from_patch_payload(
        delta_patch,
        Some(&base_graph),
    );
    let mismatch_payload =
        crate::flow_graph_data_merge_contract::with_runtime_revision(delta_patch, json!(99));
    let mismatch_result =
        crate::flow_graph_data_merge_contract::build_graph_data_from_patch_payload(
            &mismatch_payload,
            Some(&base_graph),
        );
    let runtime_graph = patch_result
        .get("data")
        .and_then(|data| data.get("runtime_graph"))
        .cloned()
        .unwrap_or(Value::Null);
    let edge_semantics = patch_result
        .get("data")
        .and_then(|data| data.get("edges"))
        .and_then(Value::as_array)
        .map(|edges| edges.iter().map(edge_semantics).collect::<Vec<_>>())
        .unwrap_or_default();
    let search_result = crate::flow_graph_search::project_search_contract(
        payload.get("searchContract"),
        &runtime_graph,
    );
    let filter_result = crate::flow_graph_search_filter_contract::project_filter_contract(
        payload.get("filterContract"),
        &runtime_graph,
    )?;
    let layout_projection = crate::flow_graph_layout_contract::project_layout_projection_contract(
        payload.get("layoutProjectionContract"),
    );

    Ok(json!({
        "baseGraphData": base_graph,
        "patchResult": patch_result,
        "runtimeGraph": runtime_graph,
        "edgeSemantics": edge_semantics,
        "searchResult": search_result,
        "filterResult": filter_result,
        "layoutProjection": layout_projection,
        "mismatchResult": mismatch_result,
    }))
}

fn edge_semantics(edge: &Value) -> Value {
    json!({
        "id": edge.get("edge_id").cloned().unwrap_or(Value::Null),
        "relationType": edge.get("relation_type").cloned().unwrap_or(Value::Null),
        "txCount": edge.get("tx_count").cloned().unwrap_or(Value::Null),
        "amountTotal": edge.get("amount_total").cloned().unwrap_or(Value::Null),
    })
}
