use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Map, Value};
use std::{collections::HashMap, fs, path::PathBuf};

pub(crate) struct GraphDataMergeContractArgs {
    pub(crate) payload: Value,
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<GraphDataMergeContractArgs> {
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
                        "read graph data/merge contract input {}",
                        input_path.display()
                    )
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!(
                        "parse graph data/merge contract input {}",
                        input_path.display()
                    )
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute contract-flow-graph-data-merge <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported graph data/merge contract flag: {other}"),
        }
    }

    Ok(GraphDataMergeContractArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_graph_data_merge_contract(payload: &Value) -> Value {
    let base_graph = clone_graph_data(payload.get("baseGraphData").unwrap_or(&Value::Null));
    let patch_results = contract_rows(payload, "patchContracts", "patchContract")
        .iter()
        .map(|contract| {
            let patch_payload = contract
                .get("patchPayload")
                .or_else(|| contract.get("patch_payload"))
                .unwrap_or(contract);
            let use_base = contract
                .get("useBaseGraph")
                .or_else(|| contract.get("use_base_graph"))
                .map(bool_value)
                .unwrap_or(true);
            let result = build_graph_data_from_patch_payload(
                patch_payload,
                if use_base { Some(&base_graph) } else { None },
            );
            let runtime_summary = result
                .get("data")
                .and_then(|data| data.get("runtime_graph"))
                .map(runtime_graph_identity_summary)
                .unwrap_or(Value::Null);
            json!({
                "name": text_value(contract.get("name")),
                "result": result,
                "runtimeSummary": runtime_summary,
            })
        })
        .collect::<Vec<_>>();

    json!({
        "baseGraphData": base_graph,
        "baseRuntimeSummary": runtime_graph_identity_summary(
            base_graph.get("runtime_graph").unwrap_or(&Value::Null),
        ),
        "patchResults": patch_results,
    })
}

pub(crate) fn build_graph_data_from_patch_payload(
    patch_payload: &Value,
    cached_base: Option<&Value>,
) -> Value {
    let target_snapshot_ref = as_flow_snapshot_ref(patch_payload.get("result_snapshot_ref"));
    let patch_kind = text_value(patch_payload.get("patch_kind"));
    let patch_kind = if patch_kind.is_empty() {
        "full".to_string()
    } else {
        patch_kind.to_ascii_lowercase()
    };
    let next_runtime_revision = runtime_revision_value(patch_payload.get("runtime_revision"));

    if patch_kind == "delta" {
        let Some(base) = cached_base else {
            return patch_result(
                Value::Null,
                target_snapshot_ref,
                &patch_kind,
                "missing patch base",
            );
        };
        let Some(runtime_graph_patch) = patch_payload.get("runtime_graph_patch") else {
            return patch_result(
                Value::Null,
                target_snapshot_ref,
                &patch_kind,
                "missing patch base",
            );
        };
        let base_revision = runtime_revision_f64(patch_payload.get("base_runtime_revision"));
        let cached_revision = base
            .get("runtime_graph")
            .and_then(|runtime_graph| runtime_revision_f64(runtime_graph.get("runtime_revision")));
        if let (Some(expected), Some(actual)) = (base_revision, cached_revision) {
            if (expected - actual).abs() > f64::EPSILON {
                return patch_result(
                    Value::Null,
                    target_snapshot_ref,
                    &patch_kind,
                    "runtime revision mismatch",
                );
            }
        }

        let runtime_graph = apply_runtime_graph_patch(
            base.get("runtime_graph").unwrap_or(&Value::Null),
            runtime_graph_patch,
            next_runtime_revision,
        );
        let data = build_graph_data(patch_payload, runtime_graph, target_snapshot_ref.clone());
        return patch_result(data, target_snapshot_ref, &patch_kind, "");
    }

    let mut runtime_graph = patch_payload
        .get("runtime_graph")
        .map(clone_runtime_graph)
        .unwrap_or_else(|| clone_runtime_graph(&Value::Null));
    if let Some(revision) = next_runtime_revision {
        if let Some(object) = runtime_graph.as_object_mut() {
            object.insert("runtime_revision".to_string(), revision);
        }
    }
    let data = build_graph_data(patch_payload, runtime_graph, target_snapshot_ref.clone());
    patch_result(data, target_snapshot_ref, &patch_kind, "")
}

pub(crate) fn clone_graph_data(data: &Value) -> Value {
    json!({
        "nodes": clone_array(data.get("nodes")),
        "edges": clone_array(data.get("edges")),
        "stats": clone_object(data.get("stats")),
        "graph_tier": text_value(data.get("graph_tier")),
        "render_hints": clone_optional_object(data.get("render_hints")),
        "projection": clone_projection(data.get("projection")),
        "runtime_graph": clone_runtime_graph(data.get("runtime_graph").unwrap_or(&Value::Null)),
        "result_snapshot_ref": as_flow_snapshot_ref(data.get("result_snapshot_ref")),
    })
}

pub(crate) fn runtime_graph_identity_summary(graph: &Value) -> Value {
    if !graph.is_object() {
        return Value::Null;
    }
    let nodes = clone_array(graph.get("nodes"));
    let edges = clone_array(graph.get("edges"));
    json!({
        "runtimeRevision": graph
            .get("runtime_revision")
            .and_then(|value| runtime_revision_value(Some(value)))
            .unwrap_or(Value::Null),
        "nodeIds": nodes
            .iter()
            .enumerate()
            .map(|(index, node)| json!(runtime_node_identity(node, index)))
            .collect::<Vec<_>>(),
        "edgeIds": edges
            .iter()
            .enumerate()
            .map(|(index, edge)| json!(runtime_edge_identity(edge, index)))
            .collect::<Vec<_>>(),
    })
}

pub(crate) fn with_runtime_revision(value: &Value, revision: Value) -> Value {
    let mut object = value.as_object().cloned().unwrap_or_default();
    object.insert("base_runtime_revision".to_string(), revision);
    Value::Object(object)
}

pub(crate) fn runtime_node_identity(row: &Value, index: usize) -> String {
    for field in ["id", "node_id", "display_id", "label", "title", "name"] {
        let value = row
            .get(field)
            .map(|value| text_value(Some(value)))
            .unwrap_or_default();
        if !value.is_empty() {
            return value;
        }
    }
    format!("node:{index}")
}

pub(crate) fn runtime_edge_identity(row: &Value, index: usize) -> String {
    for field in ["id", "edge_id"] {
        let value = row
            .get(field)
            .map(|value| text_value(Some(value)))
            .unwrap_or_default();
        if !value.is_empty() {
            return value;
        }
    }
    let source = row
        .get("source")
        .or_else(|| row.get("from_node_id"))
        .map(|value| text_value(Some(value)))
        .unwrap_or_default();
    let target = row
        .get("target")
        .or_else(|| row.get("to_node_id"))
        .map(|value| text_value(Some(value)))
        .unwrap_or_default();
    if !source.is_empty() && !target.is_empty() {
        let mode = row
            .get("mode")
            .map(|value| text_value(Some(value)))
            .unwrap_or_default();
        let arrow = row
            .get("edgeArrow")
            .or_else(|| row.get("arrow"))
            .map(|value| text_value(Some(value)))
            .unwrap_or_default();
        let label = row
            .get("label")
            .map(|value| text_value(Some(value)))
            .unwrap_or_default();
        let amount = row
            .get("amount_total")
            .or_else(|| row.get("amount"))
            .map(value_key_text)
            .unwrap_or_default();
        let count = row
            .get("tx_count")
            .or_else(|| row.get("count"))
            .map(value_key_text)
            .unwrap_or_default();
        return format!(
            "{source}->{target}|m:{mode}|a:{arrow}|l:{label}|amt:{amount}|cnt:{count}|idx:{index}"
        );
    }
    format!("edge:{index}")
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

fn patch_result(
    data: Value,
    target_snapshot_ref: Value,
    patch_kind: &str,
    error_message: &str,
) -> Value {
    json!({
        "data": data,
        "targetSnapshotRef": target_snapshot_ref,
        "patchKind": patch_kind,
        "errorMessage": error_message,
    })
}

fn build_graph_data(source: &Value, runtime_graph: Value, result_snapshot_ref: Value) -> Value {
    json!({
        "nodes": clone_array(source.get("nodes")),
        "edges": clone_array(source.get("edges")),
        "stats": clone_object(source.get("stats")),
        "graph_tier": text_value(source.get("graph_tier")),
        "render_hints": clone_object(source.get("render_hints")),
        "projection": clone_projection_for_patch(source.get("projection")),
        "runtime_graph": runtime_graph,
        "result_snapshot_ref": result_snapshot_ref,
    })
}

fn clone_runtime_graph(graph: &Value) -> Value {
    let source = graph.as_object();
    let mut payload = Map::new();
    payload.insert(
        "nodes".to_string(),
        Value::Array(clone_array(source.and_then(|object| object.get("nodes")))),
    );
    payload.insert(
        "edges".to_string(),
        Value::Array(clone_array(source.and_then(|object| object.get("edges")))),
    );
    if let Some(revision) = source
        .and_then(|object| object.get("runtime_revision"))
        .and_then(|value| runtime_revision_value(Some(value)))
    {
        payload.insert("runtime_revision".to_string(), revision);
    }
    Value::Object(payload)
}

fn apply_runtime_graph_patch(
    base_graph: &Value,
    patch: &Value,
    runtime_revision: Option<Value>,
) -> Value {
    let base = clone_runtime_graph(base_graph);
    let Some(patch_object) = patch.as_object() else {
        return base;
    };

    let nodes = apply_row_patch(
        base.get("nodes"),
        patch_object.get("remove_node_ids"),
        patch_object.get("upsert_nodes"),
        runtime_node_identity,
    );
    let edges = apply_row_patch(
        base.get("edges"),
        patch_object.get("remove_edge_ids"),
        patch_object.get("upsert_edges"),
        runtime_edge_identity,
    );

    let mut next = Map::new();
    next.insert("nodes".to_string(), Value::Array(nodes));
    next.insert("edges".to_string(), Value::Array(edges));
    if let Some(revision) = runtime_revision.or_else(|| base.get("runtime_revision").cloned()) {
        next.insert("runtime_revision".to_string(), revision);
    }
    Value::Object(next)
}

fn apply_row_patch(
    base_rows: Option<&Value>,
    remove_ids: Option<&Value>,
    upsert_rows: Option<&Value>,
    identity: fn(&Value, usize) -> String,
) -> Vec<Value> {
    let mut rows: Vec<Option<Value>> = Vec::new();
    let mut index_by_id: HashMap<String, usize> = HashMap::new();

    for (index, row) in clone_array(base_rows).into_iter().enumerate() {
        let row_id = identity(&row, index);
        if let Some(existing_index) = index_by_id.get(&row_id).copied() {
            rows[existing_index] = Some(row);
        } else {
            index_by_id.insert(row_id, rows.len());
            rows.push(Some(row));
        }
    }

    for remove_id in clone_array(remove_ids) {
        let row_id = text_value(Some(&remove_id));
        if let Some(index) = index_by_id.remove(&row_id) {
            rows[index] = None;
        }
    }

    for (index, row) in clone_array(upsert_rows).into_iter().enumerate() {
        let row_id = identity(&row, index);
        if let Some(existing_index) = index_by_id.get(&row_id).copied() {
            rows[existing_index] = Some(row);
        } else {
            index_by_id.insert(row_id, rows.len());
            rows.push(Some(row));
        }
    }

    rows.into_iter().flatten().collect()
}

fn clone_projection(value: Option<&Value>) -> Value {
    let Some(object) = value.and_then(Value::as_object) else {
        return Value::Null;
    };
    clone_projection_object(object)
}

fn clone_projection_for_patch(value: Option<&Value>) -> Value {
    match value.and_then(Value::as_object) {
        Some(object) => clone_projection_object(object),
        None => json!({
            "source_result_snapshot_ref": Value::Null,
        }),
    }
}

fn clone_projection_object(object: &Map<String, Value>) -> Value {
    let mut projection = object.clone();
    projection.insert(
        "source_result_snapshot_ref".to_string(),
        as_flow_snapshot_ref(object.get("source_result_snapshot_ref")),
    );
    Value::Object(projection)
}

fn as_flow_snapshot_ref(value: Option<&Value>) -> Value {
    let Some(object) = value.and_then(Value::as_object) else {
        return Value::Null;
    };
    let snapshot_id = first_text(
        object,
        &["snapshot_id", "snapshotId", "graph_hash", "graphHash"],
    );
    let graph_hash = first_text(object, &["graph_hash", "graphHash"]);
    let graph_hash = if graph_hash.is_empty() {
        snapshot_id.clone()
    } else {
        graph_hash
    };
    if snapshot_id.is_empty() && graph_hash.is_empty() {
        return Value::Null;
    }
    json!({
        "snapshot_id": if snapshot_id.is_empty() { graph_hash.clone() } else { snapshot_id },
        "graph_hash": graph_hash,
        "node_count": non_negative_i64(object.get("node_count").or_else(|| object.get("nodeCount"))),
        "edge_count": non_negative_i64(object.get("edge_count").or_else(|| object.get("edgeCount"))),
        "stored_at": text_value(object.get("stored_at").or_else(|| object.get("storedAt"))),
    })
}

fn clone_array(value: Option<&Value>) -> Vec<Value> {
    value
        .and_then(Value::as_array)
        .map(|items| items.to_vec())
        .unwrap_or_default()
}

fn clone_object(value: Option<&Value>) -> Value {
    value
        .and_then(Value::as_object)
        .map(|object| Value::Object(object.clone()))
        .unwrap_or_else(|| json!({}))
}

fn clone_optional_object(value: Option<&Value>) -> Value {
    value
        .and_then(Value::as_object)
        .map(|object| Value::Object(object.clone()))
        .unwrap_or(Value::Null)
}

fn first_text(object: &Map<String, Value>, fields: &[&str]) -> String {
    for field in fields {
        let value = text_value(object.get(*field));
        if !value.is_empty() {
            return value;
        }
    }
    String::new()
}

fn text_value(value: Option<&Value>) -> String {
    match value {
        Some(Value::String(text)) => text.trim().to_string(),
        Some(Value::Number(number)) => number.to_string(),
        Some(Value::Bool(value)) => value.to_string(),
        _ => String::new(),
    }
}

fn value_key_text(value: &Value) -> String {
    match value {
        Value::Null => String::new(),
        Value::String(text) => text.clone(),
        Value::Number(number) => number.to_string(),
        Value::Bool(value) => value.to_string(),
        _ => value.to_string(),
    }
}

fn runtime_revision_f64(value: Option<&Value>) -> Option<f64> {
    finite_f64(value)
}

fn runtime_revision_value(value: Option<&Value>) -> Option<Value> {
    let next = finite_f64(value)?;
    Some(numeric_json(next))
}

fn numeric_json(number: f64) -> Value {
    if number.fract() == 0.0 && number >= i64::MIN as f64 && number <= i64::MAX as f64 {
        json!(number as i64)
    } else {
        json!(number)
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

fn non_negative_i64(value: Option<&Value>) -> i64 {
    finite_f64(value)
        .map(|number| number.max(0.0) as i64)
        .unwrap_or(0)
}

fn bool_value(value: &Value) -> bool {
    match value {
        Value::Bool(value) => *value,
        Value::Number(number) => number.as_f64().map(|value| value != 0.0).unwrap_or(false),
        Value::String(text) => {
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
        Value::Array(_) | Value::Object(_) => true,
        Value::Null => false,
    }
}
