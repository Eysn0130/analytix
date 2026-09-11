use anyhow::{bail, Context, Result};
use serde_json::{json, Value};
use std::{
    fs,
    path::{Path, PathBuf},
};

pub(crate) struct GraphParityContractArgs {
    workflow: Vec<NamedPayload>,
    data_merge: Vec<NamedPayload>,
    layout_projection: Vec<NamedPayload>,
    search_filter: Vec<NamedPayload>,
}

struct NamedPayload {
    name: String,
    payload: Value,
}

impl GraphParityContractArgs {
    fn is_empty(&self) -> bool {
        self.workflow.is_empty()
            && self.data_merge.is_empty()
            && self.layout_projection.is_empty()
            && self.search_filter.is_empty()
    }
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<GraphParityContractArgs> {
    let mut args = GraphParityContractArgs {
        workflow: Vec::new(),
        data_merge: Vec::new(),
        layout_projection: Vec::new(),
        search_filter: Vec::new(),
    };

    while let Some(flag) = iter.next() {
        match flag.as_str() {
            "--input-path" => {
                let input_path = PathBuf::from(crate::required_value(&mut iter, "--input-path")?);
                let payload = read_payload_path(&input_path, "graph contract input")?;
                push_auto_payload(&mut args, path_name(&input_path, "graph"), payload)?;
            }
            "--payload-json" => {
                let raw_payload = crate::required_value(&mut iter, "--payload-json")?;
                let payload = serde_json::from_str(&raw_payload)
                    .with_context(|| "parse --payload-json as JSON")?;
                push_auto_payload(&mut args, "payload".to_string(), payload)?;
            }
            "--workflow-input-path" => {
                push_path(
                    &mut args.workflow,
                    crate::required_value(&mut iter, "--workflow-input-path")?,
                    "workflow",
                )?;
            }
            "--data-merge-input-path" => {
                push_path(
                    &mut args.data_merge,
                    crate::required_value(&mut iter, "--data-merge-input-path")?,
                    "data-merge",
                )?;
            }
            "--layout-input-path" | "--layout-projection-input-path" => {
                push_path(
                    &mut args.layout_projection,
                    crate::required_value(&mut iter, flag.as_str())?,
                    "layout",
                )?;
            }
            "--search-filter-input-path" => {
                push_path(
                    &mut args.search_filter,
                    crate::required_value(&mut iter, "--search-filter-input-path")?,
                    "search-filter",
                )?;
            }
            "--workflow-payload-json" => {
                push_json(
                    &mut args.workflow,
                    "workflow",
                    crate::required_value(&mut iter, "--workflow-payload-json")?,
                )?;
            }
            "--data-merge-payload-json" => {
                push_json(
                    &mut args.data_merge,
                    "data-merge",
                    crate::required_value(&mut iter, "--data-merge-payload-json")?,
                )?;
            }
            "--layout-payload-json" | "--layout-projection-payload-json" => {
                push_json(
                    &mut args.layout_projection,
                    "layout",
                    crate::required_value(&mut iter, flag.as_str())?,
                )?;
            }
            "--search-filter-payload-json" => {
                push_json(
                    &mut args.search_filter,
                    "search-filter",
                    crate::required_value(&mut iter, "--search-filter-payload-json")?,
                )?;
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute contract-flow-graph [--input-path <fixture> ... | --workflow-input-path <path> --data-merge-input-path <path> --layout-input-path <path> --search-filter-input-path <path>]");
            }
            other => bail!("unsupported graph contract runner flag: {other}"),
        }
    }

    if args.is_empty() {
        bail!("at least one graph contract input is required");
    }
    Ok(args)
}

pub(crate) fn project_graph_parity_contract(args: &GraphParityContractArgs) -> Result<Value> {
    let workflow = args
        .workflow
        .iter()
        .map(project_workflow_entry)
        .collect::<Result<Vec<_>>>()?;
    let search_filter = args
        .search_filter
        .iter()
        .map(project_search_filter_entry)
        .collect::<Result<Vec<_>>>()?;
    Ok(json!({
        "workflow": workflow,
        "dataMerge": args.data_merge.iter().map(project_data_merge_entry).collect::<Vec<_>>(),
        "layoutProjection": args.layout_projection.iter().map(project_layout_entry).collect::<Vec<_>>(),
        "searchFilter": search_filter,
    }))
}

fn project_workflow_entry(entry: &NamedPayload) -> Result<Value> {
    Ok(json!({
        "name": entry.name,
        "contract": crate::flow_graph_contract::project_graph_workflow_contract(&entry.payload)?,
    }))
}

fn project_data_merge_entry(entry: &NamedPayload) -> Value {
    json!({
        "name": entry.name,
        "dataMerge": crate::flow_graph_data_merge_contract::project_graph_data_merge_contract(&entry.payload),
    })
}

fn project_layout_entry(entry: &NamedPayload) -> Value {
    json!({
        "name": entry.name,
        "layoutProjection": crate::flow_graph_layout_contract::project_graph_layout_contract(&entry.payload),
    })
}

fn project_search_filter_entry(entry: &NamedPayload) -> Result<Value> {
    Ok(json!({
        "name": entry.name,
        "searchFilter": crate::flow_graph_search_filter_contract::project_graph_search_filter_contract(&entry.payload)?,
    }))
}

fn push_path(target: &mut Vec<NamedPayload>, path: String, fallback_name: &str) -> Result<()> {
    let input_path = PathBuf::from(path);
    target.push(NamedPayload {
        name: path_name(&input_path, fallback_name),
        payload: read_payload_path(&input_path, fallback_name)?,
    });
    Ok(())
}

fn push_json(target: &mut Vec<NamedPayload>, name: &str, raw_payload: String) -> Result<()> {
    target.push(NamedPayload {
        name: name.to_string(),
        payload: serde_json::from_str(&raw_payload)
            .with_context(|| format!("parse --{name}-payload-json as JSON"))?,
    });
    Ok(())
}

fn push_auto_payload(
    args: &mut GraphParityContractArgs,
    name: String,
    payload: Value,
) -> Result<()> {
    if push_manifest(args, &name, &payload)? {
        return Ok(());
    }
    if payload.get("patchContracts").is_some() || payload.get("patchContract").is_some() {
        args.data_merge.push(NamedPayload { name, payload });
        return Ok(());
    }
    if payload.get("deltaPatch").is_some() {
        args.workflow.push(NamedPayload { name, payload });
        return Ok(());
    }
    if payload.get("layoutProjectionContract").is_some()
        || payload.get("nodePositions").is_some()
        || payload.get("node_positions").is_some()
    {
        args.layout_projection.push(NamedPayload { name, payload });
        return Ok(());
    }
    if payload.get("searchContracts").is_some()
        || payload.get("searchContract").is_some()
        || payload.get("filterContracts").is_some()
        || payload.get("filterContract").is_some()
    {
        args.search_filter.push(NamedPayload { name, payload });
        return Ok(());
    }
    bail!("unable to classify graph contract payload: {name}");
}

fn push_manifest(args: &mut GraphParityContractArgs, name: &str, payload: &Value) -> Result<bool> {
    let mut matched = false;
    matched |= push_manifest_rows(
        args,
        "workflow",
        name,
        payload.get("workflow"),
        payload.get("workflows"),
    )?;
    matched |= push_manifest_rows(
        args,
        "data-merge",
        name,
        payload
            .get("dataMerge")
            .or_else(|| payload.get("data_merge")),
        payload
            .get("dataMerges")
            .or_else(|| payload.get("data_merges")),
    )?;
    matched |= push_manifest_rows(
        args,
        "layout",
        name,
        payload
            .get("layoutProjection")
            .or_else(|| payload.get("layout_projection")),
        payload
            .get("layoutProjections")
            .or_else(|| payload.get("layout_projections")),
    )?;
    matched |= push_manifest_rows(
        args,
        "search-filter",
        name,
        payload
            .get("searchFilter")
            .or_else(|| payload.get("search_filter")),
        payload
            .get("searchFilters")
            .or_else(|| payload.get("search_filters")),
    )?;
    Ok(matched)
}

fn push_manifest_rows(
    args: &mut GraphParityContractArgs,
    kind: &str,
    manifest_name: &str,
    singular: Option<&Value>,
    plural: Option<&Value>,
) -> Result<bool> {
    let mut matched = false;
    if let Some(value) = singular {
        push_manifest_row(args, kind, manifest_name, 0, value)?;
        matched = true;
    }
    if let Some(rows) = plural.and_then(Value::as_array) {
        for (index, value) in rows.iter().enumerate() {
            push_manifest_row(args, kind, manifest_name, index, value)?;
        }
        matched = true;
    }
    Ok(matched)
}

fn push_manifest_row(
    args: &mut GraphParityContractArgs,
    kind: &str,
    manifest_name: &str,
    index: usize,
    value: &Value,
) -> Result<()> {
    let payload = if let Some(path) = value.get("inputPath").or_else(|| value.get("input_path")) {
        read_payload_path(&PathBuf::from(text_value(Some(path))), kind)?
    } else {
        value.clone()
    };
    let name = value
        .get("name")
        .map(|value| text_value(Some(value)))
        .filter(|value| !value.is_empty())
        .unwrap_or_else(|| format!("{manifest_name}:{kind}:{index}"));
    match kind {
        "workflow" => args.workflow.push(NamedPayload { name, payload }),
        "data-merge" => args.data_merge.push(NamedPayload { name, payload }),
        "layout" => args.layout_projection.push(NamedPayload { name, payload }),
        "search-filter" => args.search_filter.push(NamedPayload { name, payload }),
        _ => bail!("unsupported graph contract manifest kind: {kind}"),
    }
    Ok(())
}

fn read_payload_path(path: &Path, label: &str) -> Result<Value> {
    let raw_payload =
        fs::read_to_string(path).with_context(|| format!("read {label} {}", path.display()))?;
    serde_json::from_str(&raw_payload).with_context(|| format!("parse {label} {}", path.display()))
}

fn path_name(path: &Path, fallback: &str) -> String {
    path.file_stem()
        .and_then(|name| name.to_str())
        .map(|name| name.trim().to_string())
        .filter(|name| !name.is_empty())
        .unwrap_or_else(|| fallback.to_string())
}

fn text_value(value: Option<&Value>) -> String {
    match value {
        Some(Value::String(text)) => text.trim().to_string(),
        Some(Value::Number(number)) => number.to_string(),
        Some(Value::Bool(value)) => value.to_string(),
        _ => String::new(),
    }
}
