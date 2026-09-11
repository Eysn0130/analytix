use anyhow::{anyhow, bail, Context, Result};
use icu_collator::{options::CollatorOptions, Collator};
use icu_locale_core::locale;
use serde_json::{json, Map, Number, Value};
use std::{fs, path::PathBuf};

const PROJECTION_LAYOUT_SYNC_NODE_LIMIT: usize = 50_000;

pub(crate) struct ProjectionLayoutSyncArgs {
    payload: Value,
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<ProjectionLayoutSyncArgs> {
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
                    format!("read projection layout sync input {}", input_path.display())
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!(
                        "parse projection layout sync input {}",
                        input_path.display()
                    )
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute project-flow-projection-layout-sync <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported projection layout sync flag: {other}"),
        }
    }

    Ok(ProjectionLayoutSyncArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_projection_layout_sync_payload(payload: &Value) -> Value {
    let source = payload.as_object();
    let nodes = source
        .and_then(|obj| obj.get("nodes"))
        .and_then(Value::as_array)
        .map(Vec::as_slice)
        .unwrap_or(&[]);
    let viewport = source
        .and_then(|obj| obj.get("viewport"))
        .unwrap_or(&Value::Null);
    let graph_size = source
        .and_then(|obj| obj.get("graph_size"))
        .unwrap_or(&Value::Null);
    let current_nodes = source
        .and_then(|obj| {
            obj.get("current_nodes")
                .or_else(|| obj.get("currentNodes"))
                .or_else(|| obj.get("base_nodes"))
                .or_else(|| obj.get("baseNodes"))
                .or_else(|| obj.get("existing_nodes"))
                .or_else(|| obj.get("existingNodes"))
        })
        .and_then(Value::as_array)
        .map(Vec::as_slice)
        .unwrap_or(&[]);

    let normalized_viewport = normalize_projection_viewport(viewport);
    let normalized_graph_size = normalize_graph_size(graph_size);
    let sync_rows = project_layout_sync_rows(nodes);
    let snapshot_id = source
        .and_then(|obj| {
            obj.get("snapshotId")
                .or_else(|| obj.get("snapshot_id"))
                .or_else(|| obj.get("resultSnapshotId"))
                .or_else(|| obj.get("result_snapshot_id"))
        })
        .map(|value| js_or_empty_string(Some(value)))
        .unwrap_or_default();
    let signature = build_projection_layout_sync_signature(&snapshot_id, &sync_rows, viewport);
    let projected = project_layout_rows(&sync_rows);
    let node_updates = project_layout_node_updates(current_nodes, &projected.layout_by_id);
    let node_update_count = node_updates.len();

    json!({
        "nodes": sync_rows,
        "signature": signature,
        "layout_by_id": projected.layout_by_id,
        "node_updates": node_updates,
        "node_update_count": node_update_count,
        "layout_index_summary": {
            "version": 1,
            "signature": signature,
            "indexed_node_count": projected.indexed_node_count,
            "cluster_node_count": projected.cluster_node_count,
            "dot_node_count": projected.dot_node_count,
            "bounds": {
                "min_x": projected.min_x,
                "max_x": projected.max_x,
                "min_y": projected.min_y,
                "max_y": projected.max_y,
            },
            "viewport": normalized_viewport,
            "graph_size": normalized_graph_size,
        },
    })
}

pub(crate) fn project_projection_layout_sync(args: &ProjectionLayoutSyncArgs) -> Value {
    project_projection_layout_sync_payload(&args.payload)
}

struct ProjectedLayoutRows {
    layout_by_id: Map<String, Value>,
    indexed_node_count: usize,
    cluster_node_count: usize,
    dot_node_count: usize,
    min_x: Option<f64>,
    max_x: Option<f64>,
    min_y: Option<f64>,
    max_y: Option<f64>,
}

fn project_layout_sync_rows(models: &[Value]) -> Vec<Value> {
    let mut rows = models
        .iter()
        .take(PROJECTION_LAYOUT_SYNC_NODE_LIMIT)
        .filter_map(project_layout_sync_row)
        .collect::<Vec<_>>();
    let collator = Collator::try_new(locale!("zh-CN").into(), CollatorOptions::default()).ok();
    rows.sort_by(|left, right| {
        let left_id = js_text_value(left.get("id"));
        let right_id = js_text_value(right.get("id"));
        match &collator {
            Some(collator) => collator
                .compare(left_id.as_str(), right_id.as_str())
                .then_with(|| left_id.cmp(&right_id)),
            None => left_id.cmp(&right_id),
        }
    });
    rows
}

fn project_layout_sync_row(model: &Value) -> Option<Value> {
    let id = js_or_empty_string(model.get("id")).trim().to_string();
    if id.is_empty() {
        return None;
    }
    let x = js_number(model.get("x"))?;
    let y = js_number(model.get("y"))?;
    let cluster_node = is_cluster_projection_node(model, &id);
    let node_render_mode =
        first_js_truthy(model.get("nodeRenderMode"), model.get("node_render_mode"))
            .map(|value| js_text_value(Some(value)))
            .unwrap_or_default()
            .trim()
            .to_ascii_lowercase();
    let node_render_mode = if node_render_mode.is_empty() {
        "entity".to_string()
    } else {
        node_render_mode
    };
    let collapsed = cluster_node || node_render_mode == "dot";
    if !collapsed {
        return None;
    }
    let projection_cluster_id =
        first_js_truthy(model.get("projection_cluster_id"), model.get("cluster_id"))
            .map(|value| js_text_value(Some(value)))
            .unwrap_or_default()
            .trim()
            .to_string();
    let projection_visible = match model.get("projection_visible") {
        Some(Value::Bool(value)) => *value,
        other => js_truthy(other),
    };
    let projection_collapsed = match model.get("projection_collapsed") {
        Some(Value::Bool(value)) => *value,
        _ => node_render_mode == "dot",
    };

    Some(json!({
        "id": id,
        "x": number_value(js_round_two(x)),
        "y": number_value(js_round_two(y)),
        "cluster_node": cluster_node,
        "projection_cluster_id": projection_cluster_id,
        "node_render_mode": node_render_mode,
        "projection_visible": projection_visible,
        "projection_collapsed": projection_collapsed,
    }))
}

fn is_cluster_projection_node(model: &Value, id: &str) -> bool {
    js_truthy(model.get("cluster_node")) || id.starts_with("__cluster__::")
}

fn project_layout_rows(rows: &[Value]) -> ProjectedLayoutRows {
    let mut layout_by_id = Map::new();
    let mut cluster_node_count = 0usize;
    let mut dot_node_count = 0usize;
    let mut min_x: Option<f64> = None;
    let mut max_x: Option<f64> = None;
    let mut min_y: Option<f64> = None;
    let mut max_y: Option<f64> = None;

    for row in rows.iter().take(PROJECTION_LAYOUT_SYNC_NODE_LIMIT) {
        let Some(item) = row.as_object() else {
            continue;
        };
        let node_id = item
            .get("id")
            .map(py_or_empty_string)
            .unwrap_or_default()
            .trim()
            .to_string();
        let x = item.get("x").and_then(py_optional_float);
        let y = item.get("y").and_then(py_optional_float);
        let (Some(x), Some(y)) = (x, y) else {
            continue;
        };
        if node_id.is_empty() {
            continue;
        }

        let cluster_node = item.get("cluster_node").map(py_bool).unwrap_or(false);
        let node_render_mode =
            first_truthy(item.get("node_render_mode"), item.get("nodeRenderMode"))
                .map(py_string)
                .unwrap_or_default()
                .trim()
                .to_string();
        if cluster_node {
            cluster_node_count += 1;
        }
        if node_render_mode.trim().eq_ignore_ascii_case("dot") {
            dot_node_count += 1;
        }
        let projection_cluster_id = item
            .get("projection_cluster_id")
            .map(py_or_empty_string)
            .unwrap_or_default()
            .trim()
            .to_string();

        layout_by_id.insert(
            node_id,
            json!({
                "x": x,
                "y": y,
                "cluster_node": cluster_node,
                "projection_cluster_id": projection_cluster_id,
                "nodeRenderMode": node_render_mode,
                "projection_visible": item.get("projection_visible").cloned().unwrap_or(Value::Null),
                "projection_collapsed": item.get("projection_collapsed").cloned().unwrap_or(Value::Null),
            }),
        );

        min_x = Some(min_x.map_or(x, |current| current.min(x)));
        max_x = Some(max_x.map_or(x, |current| current.max(x)));
        min_y = Some(min_y.map_or(y, |current| current.min(y)));
        max_y = Some(max_y.map_or(y, |current| current.max(y)));
    }

    ProjectedLayoutRows {
        indexed_node_count: layout_by_id.len(),
        layout_by_id,
        cluster_node_count,
        dot_node_count,
        min_x,
        max_x,
        min_y,
        max_y,
    }
}

fn project_layout_node_updates(
    current_nodes: &[Value],
    layout_by_id: &Map<String, Value>,
) -> Vec<Value> {
    if current_nodes.is_empty() || layout_by_id.is_empty() {
        return Vec::new();
    }

    let mut updates = Vec::new();
    for (index, node) in current_nodes.iter().enumerate() {
        let Some(current) = node.as_object() else {
            continue;
        };
        let node_id = current
            .get("id")
            .map(py_or_empty_string)
            .unwrap_or_default()
            .trim()
            .to_string();
        if node_id.is_empty() {
            continue;
        }
        let Some(next) = layout_by_id.get(&node_id).and_then(Value::as_object) else {
            continue;
        };
        let Some(x) = next.get("x").and_then(py_optional_float) else {
            continue;
        };
        let Some(y) = next.get("y").and_then(py_optional_float) else {
            continue;
        };
        let cluster_node = next.get("cluster_node").map(py_bool).unwrap_or(false);

        let mut update = Map::new();
        update.insert("index".to_string(), json!(index));
        update.insert("id".to_string(), json!(node_id));
        update.insert("x".to_string(), number_value(x));
        update.insert("y".to_string(), number_value(y));
        update.insert("cluster_node".to_string(), json!(cluster_node));

        let mut changed = current.get("x").and_then(py_optional_float) != Some(x)
            || current.get("y").and_then(py_optional_float) != Some(y)
            || current.get("cluster_node").map(py_bool) != Some(cluster_node);

        let projection_cluster_id = next
            .get("projection_cluster_id")
            .map(py_or_empty_string)
            .unwrap_or_default()
            .trim()
            .to_string();
        if !projection_cluster_id.is_empty() {
            update.insert(
                "projection_cluster_id".to_string(),
                json!(projection_cluster_id),
            );
            changed |= current
                .get("projection_cluster_id")
                .map(py_or_empty_string)
                .unwrap_or_default()
                .trim()
                != projection_cluster_id;
        }

        let node_render_mode = next
            .get("nodeRenderMode")
            .filter(|value| py_bool(value))
            .map(py_string)
            .unwrap_or_default()
            .trim()
            .to_string();
        if !node_render_mode.is_empty() {
            update.insert("nodeRenderMode".to_string(), json!(node_render_mode));
            changed |= current
                .get("nodeRenderMode")
                .map(py_or_empty_string)
                .unwrap_or_default()
                .trim()
                != node_render_mode;
        }

        if let Some(value) = next
            .get("projection_visible")
            .filter(|value| !value.is_null())
        {
            let projection_visible = py_bool(value);
            update.insert("projection_visible".to_string(), json!(projection_visible));
            changed |= current.get("projection_visible").map(py_bool) != Some(projection_visible);
        }

        if let Some(value) = next
            .get("projection_collapsed")
            .filter(|value| !value.is_null())
        {
            let projection_collapsed = py_bool(value);
            update.insert(
                "projection_collapsed".to_string(),
                json!(projection_collapsed),
            );
            changed |=
                current.get("projection_collapsed").map(py_bool) != Some(projection_collapsed);
        }

        if changed {
            updates.push(Value::Object(update));
        }
    }
    updates
}

fn build_projection_layout_sync_signature(
    snapshot_id: &str,
    sync_rows: &[Value],
    viewport: &Value,
) -> String {
    let center = viewport.get("center").unwrap_or(&Value::Null);
    let size = viewport.get("size").unwrap_or(&Value::Null);
    let zoom = js_number(viewport.get("zoom"))
        .map(js_round_two)
        .unwrap_or(0.0);
    let mut min_x: Option<f64> = None;
    let mut max_x: Option<f64> = None;
    let mut min_y: Option<f64> = None;
    let mut max_y: Option<f64> = None;
    let mut samples = Vec::new();
    let sample_step = ((sync_rows.len() + 47) / 48).max(1);
    for (index, row) in sync_rows.iter().enumerate() {
        let Some(x) = js_number(row.get("x")) else {
            continue;
        };
        let Some(y) = js_number(row.get("y")) else {
            continue;
        };
        min_x = Some(min_x.map_or(x, |current| current.min(x)));
        max_x = Some(max_x.map_or(x, |current| current.max(x)));
        min_y = Some(min_y.map_or(y, |current| current.min(y)));
        max_y = Some(max_y.map_or(y, |current| current.max(y)));
        if index < 96 || index % sample_step == 0 {
            samples.push(format!(
                "{}:{}:{}:{}",
                js_text_value(row.get("id")),
                js_round_string(x),
                js_round_string(y),
                js_text_value(row.get("node_render_mode"))
            ));
        }
    }

    format!(
        "{}::n:{}::z:{}::c:{},{}::s:{}x{}::b:{},{},{},{}::m:{}",
        snapshot_id,
        sync_rows.len(),
        js_number_string(zoom),
        js_round_string(js_number(center.get("x")).unwrap_or(0.0)),
        js_round_string(js_number(center.get("y")).unwrap_or(0.0)),
        js_round_string(js_number(size.get("width")).unwrap_or(0.0)),
        js_round_string(js_number(size.get("height")).unwrap_or(0.0)),
        js_round_string(min_x.unwrap_or(0.0)),
        js_round_string(max_x.unwrap_or(0.0)),
        js_round_string(min_y.unwrap_or(0.0)),
        js_round_string(max_y.unwrap_or(0.0)),
        samples.join("|"),
    )
}

fn normalize_projection_viewport(value: &Value) -> Value {
    let Some(source) = value.as_object() else {
        return json!({});
    };
    let center = source.get("center").and_then(Value::as_object);
    let size = source.get("size").and_then(Value::as_object);
    let zoom = source.get("zoom").and_then(py_optional_float);
    let center_x = center
        .and_then(|obj| obj.get("x"))
        .and_then(py_optional_float);
    let center_y = center
        .and_then(|obj| obj.get("y"))
        .and_then(py_optional_float);
    let width_source = size
        .and_then(|obj| obj.get("width"))
        .filter(|candidate| py_bool(candidate))
        .or_else(|| source.get("width"));
    let height_source = size
        .and_then(|obj| obj.get("height"))
        .filter(|candidate| py_bool(candidate))
        .or_else(|| source.get("height"));
    let width = width_source.and_then(py_optional_float);
    let height = height_source.and_then(py_optional_float);

    let mut out = Map::new();
    if zoom.is_some_and(|value| value > 0.0) {
        out.insert("zoom".to_string(), number_value(zoom.unwrap()));
    }
    if let (Some(center_x), Some(center_y)) = (center_x, center_y) {
        out.insert("center".to_string(), json!({"x": center_x, "y": center_y}));
    }
    if let (Some(width), Some(height)) = (width, height) {
        if width > 0.0 && height > 0.0 {
            out.insert(
                "size".to_string(),
                json!({"width": width, "height": height}),
            );
        }
    }
    Value::Object(out)
}

fn normalize_graph_size(value: &Value) -> Value {
    let Some(source) = value.as_object() else {
        return json!({"width": Value::Null, "height": Value::Null});
    };
    json!({
        "width": source.get("width").and_then(py_optional_float),
        "height": source.get("height").and_then(py_optional_float),
    })
}

fn first_truthy<'a>(first: Option<&'a Value>, second: Option<&'a Value>) -> Option<&'a Value> {
    first.filter(|value| py_bool(value)).or(second)
}

fn first_js_truthy<'a>(first: Option<&'a Value>, second: Option<&'a Value>) -> Option<&'a Value> {
    first.filter(|value| js_truthy(Some(value))).or(second)
}

fn js_number(value: Option<&Value>) -> Option<f64> {
    let number = match value {
        Some(Value::Null) => 0.0,
        Some(Value::Bool(flag)) => {
            if *flag {
                1.0
            } else {
                0.0
            }
        }
        Some(Value::Number(number)) => number.as_f64()?,
        Some(Value::String(text)) => {
            let trimmed = text.trim();
            if trimmed.is_empty() {
                0.0
            } else {
                trimmed.parse::<f64>().ok()?
            }
        }
        Some(Value::Array(_)) | Some(Value::Object(_)) | None => return None,
    };
    number.is_finite().then_some(number)
}

fn js_or_empty_string(value: Option<&Value>) -> String {
    if js_truthy(value) {
        js_text_value(value)
    } else {
        String::new()
    }
}

fn js_text_value(value: Option<&Value>) -> String {
    match value {
        Some(Value::String(text)) => text.trim().to_string(),
        Some(Value::Number(number)) => number.to_string(),
        Some(Value::Bool(flag)) => flag.to_string(),
        Some(Value::Null) | None => String::new(),
        Some(Value::Array(values)) => values
            .iter()
            .map(|item| js_text_value(Some(item)))
            .collect::<Vec<_>>()
            .join(","),
        Some(Value::Object(_)) => "[object Object]".to_string(),
    }
}

fn js_truthy(value: Option<&Value>) -> bool {
    match value {
        Some(Value::Bool(flag)) => *flag,
        Some(Value::Number(number)) => number.as_f64().is_some_and(|value| value != 0.0),
        Some(Value::String(text)) => !text.is_empty(),
        Some(Value::Array(_)) | Some(Value::Object(_)) => true,
        Some(Value::Null) | None => false,
    }
}

fn py_optional_float(value: &Value) -> Option<f64> {
    let number = match value {
        Value::Bool(flag) => {
            if *flag {
                1.0
            } else {
                0.0
            }
        }
        Value::Number(number) => number.as_f64()?,
        Value::String(text) => text.parse::<f64>().ok()?,
        Value::Null | Value::Array(_) | Value::Object(_) => return None,
    };
    if number.is_finite() {
        Some(number)
    } else {
        None
    }
}

fn py_or_empty_string(value: &Value) -> String {
    if py_bool(value) {
        py_string(value)
    } else {
        String::new()
    }
}

fn py_string(value: &Value) -> String {
    match value {
        Value::Null => "None".to_string(),
        Value::Bool(flag) => {
            if *flag {
                "True".to_string()
            } else {
                "False".to_string()
            }
        }
        Value::Number(number) => number.to_string(),
        Value::String(text) => text.clone(),
        Value::Array(values) => format!(
            "[{}]",
            values
                .iter()
                .map(py_string)
                .collect::<Vec<String>>()
                .join(", ")
        ),
        Value::Object(_) => "[object]".to_string(),
    }
}

fn py_bool(value: &Value) -> bool {
    match value {
        Value::Null => false,
        Value::Bool(flag) => *flag,
        Value::Number(number) => number.as_f64().is_some_and(|value| value != 0.0),
        Value::String(text) => !text.is_empty(),
        Value::Array(values) => !values.is_empty(),
        Value::Object(values) => !values.is_empty(),
    }
}

fn number_value(value: f64) -> Value {
    Number::from_f64(value)
        .map(Value::Number)
        .unwrap_or(Value::Null)
}

fn js_round_two(value: f64) -> f64 {
    js_round(value * 100.0) / 100.0
}

fn js_round_string(value: f64) -> String {
    js_number_string(js_round(value))
}

fn js_round(value: f64) -> f64 {
    (value + 0.5).floor()
}

fn js_number_string(value: f64) -> String {
    if value.fract() == 0.0 && value >= i64::MIN as f64 && value <= i64::MAX as f64 {
        (value as i64).to_string()
    } else {
        let mut text = value.to_string();
        if text.contains('.') {
            while text.ends_with('0') {
                text.pop();
            }
            if text.ends_with('.') {
                text.pop();
            }
        }
        text
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn project_projection_layout_sync_indexes_valid_rows_and_summary() {
        let payload = json!({
            "viewport": {
                "zoom": "1.5",
                "center": {"x": "10", "y": -5},
                "size": {"width": 0, "height": 480},
                "width": 960
            },
            "graph_size": {"width": "1200", "height": "-1"},
            "current_nodes": [
                {
                    "id": "cluster",
                    "x": 0,
                    "y": 2,
                    "cluster_node": false,
                    "nodeRenderMode": "entity",
                    "projection_visible": true,
                    "projection_collapsed": true
                },
                {
                    "id": "dot",
                    "x": 5,
                    "y": -4,
                    "cluster_node": false,
                    "nodeRenderMode": "dot",
                    "projection_cluster_id": "cluster",
                    "projection_visible": false,
                    "projection_collapsed": false
                }
            ],
            "nodes": [
                {"id": "cluster", "x": "1.234", "y": 2, "cluster_node": true, "node_render_mode": "entity", "projection_visible": true, "projection_collapsed": true},
                {"id": "dot", "x": 5, "y": "-4", "nodeRenderMode": "dot", "projection_cluster_id": "cluster", "projection_visible": false, "projection_collapsed": false},
                {"id": "entity", "x": 7, "y": 8, "nodeRenderMode": "entity"},
                {"id": "bad", "x": "NaN", "y": 3},
                {"id": "", "x": 1, "y": 1}
            ]
        });

        assert_eq!(
            project_projection_layout_sync_payload(&payload),
            json!({
                "nodes": [
                    {
                        "id": "cluster",
                        "x": 1.23,
                        "y": 2.0,
                        "cluster_node": true,
                        "projection_cluster_id": "",
                        "node_render_mode": "entity",
                        "projection_visible": true,
                        "projection_collapsed": true
                    },
                    {
                        "id": "dot",
                        "x": 5.0,
                        "y": -4.0,
                        "cluster_node": false,
                        "projection_cluster_id": "cluster",
                        "node_render_mode": "dot",
                        "projection_visible": false,
                        "projection_collapsed": false
                    }
                ],
                "signature": "::n:2::z:1.5::c:10,-5::s:0x480::b:1,5,-4,2::m:cluster:1:2:entity|dot:5:-4:dot",
                "layout_by_id": {
                    "cluster": {
                        "x": 1.23,
                        "y": 2.0,
                        "cluster_node": true,
                        "projection_cluster_id": "",
                        "nodeRenderMode": "entity",
                        "projection_visible": true,
                        "projection_collapsed": true
                    },
                    "dot": {
                        "x": 5.0,
                        "y": -4.0,
                        "cluster_node": false,
                        "projection_cluster_id": "cluster",
                        "nodeRenderMode": "dot",
                        "projection_visible": false,
                        "projection_collapsed": false
                    }
                },
                "node_updates": [
                    {
                        "index": 0,
                        "id": "cluster",
                        "x": 1.23,
                        "y": 2.0,
                        "cluster_node": true,
                        "nodeRenderMode": "entity",
                        "projection_visible": true,
                        "projection_collapsed": true
                    }
                ],
                "node_update_count": 1,
                "layout_index_summary": {
                    "version": 1,
                    "signature": "::n:2::z:1.5::c:10,-5::s:0x480::b:1,5,-4,2::m:cluster:1:2:entity|dot:5:-4:dot",
                    "indexed_node_count": 2,
                    "cluster_node_count": 1,
                    "dot_node_count": 1,
                    "bounds": {
                        "min_x": 1.23,
                        "max_x": 5.0,
                        "min_y": -4.0,
                        "max_y": 2.0
                    },
                    "viewport": {
                        "zoom": 1.5,
                        "center": {"x": 10.0, "y": -5.0},
                        "size": {"width": 960.0, "height": 480.0}
                    },
                    "graph_size": {
                        "width": 1200.0,
                        "height": -1.0
                    }
                }
            })
        );
    }

    #[test]
    fn project_projection_layout_sync_keeps_viewport_without_nodes() {
        let payload = json!({
            "viewport": {"zoom": 1, "center": {"x": 0, "y": 0}, "size": {"width": 10, "height": 20}},
            "nodes": [{"id": "bad", "x": {}, "y": 1}]
        });
        let projected = project_projection_layout_sync_payload(&payload);

        assert_eq!(projected["layout_by_id"], json!({}));
        assert_eq!(projected["node_updates"], json!([]));
        assert_eq!(projected["node_update_count"], json!(0));
        assert_eq!(
            projected["signature"],
            json!("::n:0::z:1::c:0,0::s:10x20::b:0,0,0,0::m:")
        );
        assert_eq!(
            projected["layout_index_summary"]["viewport"],
            json!({"zoom": 1.0, "center": {"x": 0.0, "y": 0.0}, "size": {"width": 10.0, "height": 20.0}})
        );
        assert_eq!(
            projected["layout_index_summary"]["bounds"],
            json!({"min_x": null, "max_x": null, "min_y": null, "max_y": null})
        );
    }
}
