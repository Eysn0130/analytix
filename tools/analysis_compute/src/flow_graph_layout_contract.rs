use anyhow::{anyhow, bail, Context, Result};
use icu_collator::{options::CollatorOptions, Collator};
use icu_locale_core::locale;
use serde_json::{json, Value};
use std::{fs, path::PathBuf};

pub(crate) struct GraphLayoutContractArgs {
    pub(crate) payload: Value,
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<GraphLayoutContractArgs> {
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
                    format!("read graph layout contract input {}", input_path.display())
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!("parse graph layout contract input {}", input_path.display())
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute contract-flow-graph-layout-projection <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported graph layout contract flag: {other}"),
        }
    }

    Ok(GraphLayoutContractArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_graph_layout_contract(payload: &Value) -> Value {
    project_layout_projection_contract(
        payload
            .get("layoutProjectionContract")
            .or_else(|| payload.get("contract"))
            .or(Some(payload)),
    )
}

pub(crate) fn project_layout_projection_contract(contract: Option<&Value>) -> Value {
    let Some(contract) = contract else {
        return empty_layout_projection_contract();
    };
    let positions = clone_array(
        contract
            .get("nodePositions")
            .or_else(|| contract.get("node_positions")),
    );
    let mut sync_rows = positions
        .iter()
        .filter_map(projection_layout_sync_row)
        .collect::<Vec<_>>();
    let collator = Collator::try_new(locale!("zh-CN").into(), CollatorOptions::default()).ok();
    sync_rows.sort_by(|left, right| {
        let left_id = text_value(left.get("id"));
        let right_id = text_value(right.get("id"));
        match &collator {
            Some(collator) => collator
                .compare(left_id.as_str(), right_id.as_str())
                .then_with(|| left_id.cmp(&right_id)),
            None => left_id.cmp(&right_id),
        }
    });
    let node_order = sync_rows
        .iter()
        .map(|row| json!(text_value(row.get("id"))))
        .collect::<Vec<_>>();
    let bounds = projection_layout_bounds(&sync_rows);
    let signature = build_projection_layout_sync_signature(
        &text_value(
            contract
                .get("snapshotId")
                .or_else(|| contract.get("snapshot_id")),
        ),
        &sync_rows,
        contract.get("viewport"),
    );

    json!({
        "mode": text_value(contract.get("mode")),
        "nodeOrder": node_order,
        "bounds": bounds,
        "syncRows": sync_rows,
        "signature": signature,
    })
}

fn empty_layout_projection_contract() -> Value {
    json!({
        "mode": "",
        "nodeOrder": [],
        "bounds": Value::Null,
        "syncRows": [],
        "signature": "",
    })
}

fn projection_layout_sync_row(model: &Value) -> Option<Value> {
    let id = text_value(model.get("id"));
    if id.is_empty() {
        return None;
    }
    let x = finite_f64(model.get("x"))?;
    let y = finite_f64(model.get("y"))?;
    let cluster_node = is_cluster_projection_node(model);
    let node_render_mode = text_value(
        model
            .get("nodeRenderMode")
            .or_else(|| model.get("node_render_mode")),
    )
    .to_ascii_lowercase();
    let node_render_mode = if node_render_mode.is_empty() {
        "entity".to_string()
    } else {
        node_render_mode
    };
    if !cluster_node && node_render_mode != "dot" {
        return None;
    }
    let projection_cluster_id = text_value(
        model
            .get("projection_cluster_id")
            .or_else(|| model.get("cluster_id")),
    );
    let projection_collapsed = match model.get("projection_collapsed") {
        Some(Value::Bool(value)) => *value,
        _ => node_render_mode == "dot",
    };
    Some(json!({
        "id": id,
        "x": numeric_json(round_two(x)),
        "y": numeric_json(round_two(y)),
        "cluster_node": cluster_node,
        "projection_cluster_id": projection_cluster_id,
        "node_render_mode": node_render_mode,
        "projection_visible": js_truthy(model.get("projection_visible")),
        "projection_collapsed": projection_collapsed,
    }))
}

fn is_cluster_projection_node(model: &Value) -> bool {
    js_truthy(model.get("cluster_node")) || text_value(model.get("id")).starts_with("__cluster__::")
}

fn projection_layout_bounds(sync_rows: &[Value]) -> Value {
    let mut min_x: Option<f64> = None;
    let mut min_y: Option<f64> = None;
    let mut max_x: Option<f64> = None;
    let mut max_y: Option<f64> = None;
    for row in sync_rows {
        let Some(x) = finite_f64(row.get("x")) else {
            continue;
        };
        let Some(y) = finite_f64(row.get("y")) else {
            continue;
        };
        min_x = Some(min_x.map_or(x, |current| current.min(x)));
        min_y = Some(min_y.map_or(y, |current| current.min(y)));
        max_x = Some(max_x.map_or(x, |current| current.max(x)));
        max_y = Some(max_y.map_or(y, |current| current.max(y)));
    }
    match (min_x, min_y, max_x, max_y) {
        (Some(min_x), Some(min_y), Some(max_x), Some(max_y)) => json!({
            "minX": numeric_json(min_x),
            "minY": numeric_json(min_y),
            "maxX": numeric_json(max_x),
            "maxY": numeric_json(max_y),
        }),
        _ => Value::Null,
    }
}

fn build_projection_layout_sync_signature(
    snapshot_id: &str,
    sync_rows: &[Value],
    viewport: Option<&Value>,
) -> String {
    let bounds = projection_layout_bounds(sync_rows);
    let center = viewport
        .and_then(|value| value.get("center"))
        .unwrap_or(&Value::Null);
    let size = viewport
        .and_then(|value| value.get("size"))
        .unwrap_or(&Value::Null);
    let zoom = finite_f64(viewport.and_then(|value| value.get("zoom")))
        .map(round_two)
        .unwrap_or(0.0);
    let samples = sync_rows
        .iter()
        .enumerate()
        .filter_map(|(index, row)| {
            if index >= 96 && index % ((sync_rows.len() + 47) / 48).max(1) != 0 {
                return None;
            }
            let x = finite_f64(row.get("x"))?;
            let y = finite_f64(row.get("y"))?;
            Some(format!(
                "{}:{}:{}:{}",
                text_value(row.get("id")),
                round_string(x),
                round_string(y),
                text_value(row.get("node_render_mode"))
            ))
        })
        .collect::<Vec<_>>()
        .join("|");
    format!(
        "{}::n:{}::z:{}::c:{},{}::s:{}x{}::b:{},{},{},{}::m:{}",
        snapshot_id,
        sync_rows.len(),
        number_string(zoom),
        round_string(finite_f64(center.get("x")).unwrap_or(0.0)),
        round_string(finite_f64(center.get("y")).unwrap_or(0.0)),
        round_string(finite_f64(size.get("width")).unwrap_or(0.0)),
        round_string(finite_f64(size.get("height")).unwrap_or(0.0)),
        round_string(finite_f64(bounds.get("minX")).unwrap_or(0.0)),
        round_string(finite_f64(bounds.get("maxX")).unwrap_or(0.0)),
        round_string(finite_f64(bounds.get("minY")).unwrap_or(0.0)),
        round_string(finite_f64(bounds.get("maxY")).unwrap_or(0.0)),
        samples,
    )
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

fn numeric_json(number: f64) -> Value {
    if number.fract() == 0.0 && number >= i64::MIN as f64 && number <= i64::MAX as f64 {
        json!(number as i64)
    } else {
        json!(number)
    }
}

fn round_two(value: f64) -> f64 {
    js_round(value * 100.0) / 100.0
}

fn round_string(value: f64) -> String {
    number_string(js_round(value))
}

fn js_round(value: f64) -> f64 {
    (value + 0.5).floor()
}

fn number_string(value: f64) -> String {
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

fn js_truthy(value: Option<&Value>) -> bool {
    match value {
        Some(Value::Bool(value)) => *value,
        Some(Value::Number(number)) => number.as_f64().map(|value| value != 0.0).unwrap_or(false),
        Some(Value::String(text)) => !text.is_empty(),
        Some(Value::Array(_)) | Some(Value::Object(_)) => true,
        _ => false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn layout_contract_matches_js_rounding_for_negative_halves() {
        assert_eq!(round_string(-1.5), "-1");
        assert_eq!(round_string(-1.6), "-2");
        assert_eq!(round_two(-1.235), -1.24);
        assert_eq!(round_two(1.235), 1.24);
    }

    #[test]
    fn layout_contract_uses_zh_cn_order_for_non_ascii_ids() {
        let projected = project_layout_projection_contract(Some(&json!({
            "mode": "skeleton",
            "snapshotId": "snap-layout-non-ascii",
            "viewport": {"zoom": 1, "center": {"x": 0, "y": 0}, "size": {"width": 1, "height": 1}},
            "nodePositions": [
                {"id": "节点-二", "x": 20, "y": 2, "nodeRenderMode": "dot"},
                {"id": "节点-一", "x": 10, "y": 1, "nodeRenderMode": "dot"},
                {"id": "node-a", "x": 90, "y": 9, "nodeRenderMode": "dot"},
                {"id": "账户-张", "x": 60, "y": 6, "nodeRenderMode": "dot"},
                {"id": "账户-李", "x": 50, "y": 5, "nodeRenderMode": "dot"},
                {"id": "__cluster__::中", "x": -10, "y": -1, "cluster_node": true},
                {"id": "__cluster__::a", "x": -20, "y": -2, "cluster_node": true},
                {"id": "é-node", "x": 70, "y": 7, "nodeRenderMode": "dot"},
                {"id": "e-node", "x": 80, "y": 8, "nodeRenderMode": "dot"}
            ]
        })));

        assert_eq!(
            projected["nodeOrder"],
            json!([
                "__cluster__::中",
                "__cluster__::a",
                "节点-二",
                "节点-一",
                "账户-李",
                "账户-张",
                "é-node",
                "e-node",
                "node-a"
            ])
        );
    }
}
