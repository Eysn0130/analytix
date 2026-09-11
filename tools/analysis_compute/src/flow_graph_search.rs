use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Value};
use std::{fs, path::PathBuf};

use crate::flow_graph_search_core::GraphSearchIndex;

pub(crate) struct GraphSearchArgs {
    pub(crate) payload: Value,
}

pub(crate) fn parse_args(mut iter: impl Iterator<Item = String>) -> Result<GraphSearchArgs> {
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
                let raw_payload = fs::read_to_string(&input_path)
                    .with_context(|| format!("read graph search input {}", input_path.display()))?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!("parse graph search input {}", input_path.display())
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute project-flow-graph-search <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported graph search flag: {other}"),
        }
    }

    Ok(GraphSearchArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_graph_search(payload: &Value) -> Value {
    let graph = payload
        .get("graph")
        .or_else(|| payload.get("runtimeGraph"))
        .or_else(|| payload.get("runtime_graph"))
        .or_else(|| payload.get("expectedRuntimeGraph"))
        .unwrap_or(payload);
    project_search_contract(Some(payload), graph)
}

pub(crate) fn project_search_contract(contract: Option<&Value>, runtime_graph: &Value) -> Value {
    let query = contract
        .and_then(|contract| {
            contract
                .get("query")
                .or_else(|| contract.get("searchQuery"))
                .or_else(|| contract.get("search_query"))
        })
        .map(|value| text_value(Some(value)))
        .unwrap_or_default();
    let limit = contract
        .and_then(|contract| {
            contract
                .get("limit")
                .or_else(|| contract.get("searchLimit"))
                .or_else(|| contract.get("search_limit"))
        })
        .and_then(positive_usize);
    let nodes = clone_array(runtime_graph.get("nodes"));
    let search_index = GraphSearchIndex::from_nodes(&nodes);
    let node_ids = search_index
        .query_raw_node_ids(&query, limit, |node, index| {
            crate::flow_graph_data_merge_contract::runtime_node_identity(node, index)
        })
        .into_iter()
        .map(Value::String)
        .collect::<Vec<_>>();
    let first_node_id = node_ids.first().cloned().unwrap_or(Value::Null);

    json!({
        "query": query,
        "nodeIds": node_ids,
        "firstNodeId": first_node_id,
        "indexSummary": search_index.summary(),
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

fn positive_usize(value: &Value) -> Option<usize> {
    let number = match value {
        Value::Number(number) => number.as_u64()?,
        Value::String(text) => text.trim().parse::<u64>().ok()?,
        _ => return None,
    };
    usize::try_from(number).ok().filter(|value| *value > 0)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn graph_search_matches_display_ids_and_respects_limit() {
        let payload = json!({
            "query": "9555",
            "limit": 1,
            "graph": {
                "nodes": [
                    {"id": "a", "title": "Alice", "displayId": "6222"},
                    {"id": "b", "title": "B", "displayIdRaw": "账号: 9555 / 9666"},
                    {"id": "c", "title": "C", "displayIds": ["9555"]},
                    {"id": "d", "name": "9555 holder"}
                ]
            }
        });

        assert_eq!(
            project_graph_search(&payload),
            json!({
                "query": "9555",
                "nodeIds": ["b"],
                "firstNodeId": "b",
                "indexSummary": {
                    "rowCount": 4,
                    "indexedRowCount": 4,
                    "textCount": 14,
                    "maxTextCount": 5,
                },
            })
        );
    }

    #[test]
    fn graph_search_matches_name_field() {
        let payload = json!({
            "query": "Alice Alias",
            "limit": 1,
            "graph": {
                "nodes": [
                    {"id": "holder:a", "name": "Alice Alias", "displayId": "6222"},
                    {"id": "holder:b", "title": "Alice Alias"}
                ]
            }
        });

        assert_eq!(
            project_graph_search(&payload),
            json!({
                "query": "Alice Alias",
                "nodeIds": ["holder:a"],
                "firstNodeId": "holder:a",
                "indexSummary": {
                    "rowCount": 2,
                    "indexedRowCount": 2,
                    "textCount": 6,
                    "maxTextCount": 4,
                },
            })
        );
    }
}
