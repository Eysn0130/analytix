use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Value};
use std::{fs, path::PathBuf};

pub(crate) struct GraphSearchFilterContractArgs {
    pub(crate) payload: Value,
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<GraphSearchFilterContractArgs> {
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
                    format!(
                        "read graph search/filter contract input {}",
                        input_path.display()
                    )
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!(
                        "parse graph search/filter contract input {}",
                        input_path.display()
                    )
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute contract-flow-graph-search-filter <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported graph search/filter contract flag: {other}"),
        }
    }

    Ok(GraphSearchFilterContractArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_graph_search_filter_contract(payload: &Value) -> Result<Value> {
    let graph = payload
        .get("graph")
        .or_else(|| payload.get("runtimeGraph"))
        .or_else(|| payload.get("runtime_graph"))
        .or_else(|| payload.get("expectedRuntimeGraph"))
        .unwrap_or(&Value::Null);
    let search_results = contract_rows(payload, "searchContracts", "searchContract")
        .iter()
        .map(|contract| crate::flow_graph_search::project_search_contract(Some(contract), graph))
        .collect::<Vec<_>>();
    let filter_results = contract_rows(payload, "filterContracts", "filterContract")
        .iter()
        .map(|contract| project_filter_contract(Some(contract), graph))
        .collect::<Result<Vec<_>>>()?;

    Ok(json!({
        "searchResults": search_results,
        "filterResults": filter_results,
    }))
}

pub(crate) fn project_filter_contract(
    contract: Option<&Value>,
    runtime_graph: &Value,
) -> Result<Value> {
    let min_amount =
        contract_bound(contract, "minAmount", "min_amount")?.map(|value| value.max(0.0));
    let max_amount =
        contract_bound(contract, "maxAmount", "max_amount")?.map(|value| value.max(0.0));
    if matches!((min_amount, max_amount), (Some(min_value), Some(max_value)) if min_value > max_value)
    {
        bail!("graph filter contract range is invalid");
    }
    let collapse_children = bool_value(contract.and_then(|contract| {
        contract
            .get("collapseChildren")
            .or_else(|| contract.get("collapse_children"))
    }));
    let analysis_result = crate::flow_analysis_graph_data::project_analysis_graph_data(&json!({
        "source": runtime_graph,
        "filterMin": min_amount.map(numeric_json).unwrap_or(Value::Null),
        "filterMax": max_amount.map(numeric_json).unwrap_or(Value::Null),
        "collapseChildren": collapse_children,
    }))?;
    let edges = clone_array(analysis_result.get("edges"));
    let nodes = clone_array(analysis_result.get("nodes"));
    let edge_ids = edges
        .iter()
        .enumerate()
        .map(|(index, edge)| {
            json!(crate::flow_graph_data_merge_contract::runtime_edge_identity(edge, index))
        })
        .collect::<Vec<_>>();
    let node_ids = nodes
        .iter()
        .enumerate()
        .map(|(index, node)| {
            json!(crate::flow_graph_data_merge_contract::runtime_node_identity(node, index))
        })
        .collect::<Vec<_>>();
    let edge_summaries = edges
        .iter()
        .enumerate()
        .map(|(index, edge)| edge_summary(edge, index))
        .collect::<Vec<_>>();
    let node_summaries = nodes.iter().map(node_summary).collect::<Vec<_>>();

    Ok(json!({
        "minAmount": min_amount.map(numeric_json).unwrap_or(Value::Null),
        "maxAmount": max_amount.map(numeric_json).unwrap_or(Value::Null),
        "collapseChildren": collapse_children,
        "edgeIds": edge_ids,
        "nodeIds": node_ids,
        "edgeSummaries": edge_summaries,
        "nodeSummaries": node_summaries,
        "coreIds": analysis_result.get("coreIds").cloned().unwrap_or_else(|| json!([])),
        "childMap": analysis_result.get("childMap").cloned().unwrap_or_else(|| json!({})),
        "analysisGraphData": analysis_result,
    }))
}

fn contract_rows(payload: &Value, plural_key: &str, singular_key: &str) -> Vec<Value> {
    if let Some(rows) = payload.get(plural_key).and_then(Value::as_array) {
        return rows.to_vec();
    }
    payload
        .get(singular_key)
        .cloned()
        .map(|row| vec![row])
        .unwrap_or_default()
}

fn edge_summary(edge: &Value, index: usize) -> Value {
    let amount_for_filter = edge
        .as_object()
        .and_then(crate::flow_analysis_graph_data::edge_amount_total)
        .map(numeric_json)
        .unwrap_or(Value::Null);
    json!({
        "id": crate::flow_graph_data_merge_contract::runtime_edge_identity(edge, index),
        "source": text_value(edge.get("source").or_else(|| edge.get("from_node_id"))),
        "target": text_value(edge.get("target").or_else(|| edge.get("to_node_id"))),
        "relationType": edge.get("relation_type").cloned().unwrap_or(Value::Null),
        "txCount": edge.get("tx_count").or_else(|| edge.get("count")).cloned().unwrap_or(Value::Null),
        "amountForFilter": amount_for_filter,
    })
}

fn node_summary(node: &Value) -> Value {
    json!({
        "id": text_value(node.get("id").or_else(|| node.get("node_id"))),
        "projectionVisible": bool_value(node.get("projection_visible")),
        "nodeRenderMode": text_value(node.get("nodeRenderMode").or_else(|| node.get("node_render_mode"))),
    })
}

fn clone_array(value: Option<&Value>) -> Vec<Value> {
    value
        .and_then(Value::as_array)
        .map(|items| items.to_vec())
        .unwrap_or_default()
}

fn text_value(value: Option<&Value>) -> String {
    match value {
        Some(Value::String(text)) => text.trim().to_string(),
        Some(Value::Number(number)) => number.to_string(),
        Some(Value::Bool(value)) => value.to_string(),
        _ => String::new(),
    }
}

fn finite_f64(value: Option<&Value>) -> Option<f64> {
    match value {
        Some(Value::Number(number)) => number.as_f64().filter(|next| next.is_finite()),
        Some(Value::String(text)) => text
            .trim()
            .parse::<f64>()
            .ok()
            .filter(|next| next.is_finite()),
        _ => None,
    }
}

fn contract_bound(contract: Option<&Value>, primary: &str, alternate: &str) -> Result<Option<f64>> {
    let Some(value) =
        contract.and_then(|contract| contract.get(primary).or_else(|| contract.get(alternate)))
    else {
        return Ok(None);
    };
    if value.is_null() || value.as_str().is_some_and(|text| text.trim().is_empty()) {
        return Ok(None);
    }
    finite_f64(Some(value))
        .map(Some)
        .ok_or_else(|| anyhow!("graph filter contract bound is invalid"))
}

fn numeric_json(number: f64) -> Value {
    if number.fract() == 0.0 && number >= i64::MIN as f64 && number <= i64::MAX as f64 {
        json!(number as i64)
    } else {
        json!(number)
    }
}

fn bool_value(value: Option<&Value>) -> bool {
    match value {
        Some(Value::Bool(value)) => *value,
        Some(Value::Number(number)) => number.as_f64().map(|value| value != 0.0).unwrap_or(false),
        Some(Value::String(text)) => {
            let text = text.trim().to_ascii_lowercase();
            if text.is_empty()
                || matches!(
                    text.as_str(),
                    "0" | "false" | "no" | "off" | "null" | "undefined" | "nan"
                )
            {
                return false;
            }
            true
        }
        Some(Value::Array(_)) | Some(Value::Object(_)) => true,
        _ => false,
    }
}
