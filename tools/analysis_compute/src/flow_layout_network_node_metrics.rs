use serde_json::{json, Map, Value};

const NODE_LABEL_MAIN_MAX_WIDTH: f64 = 244.0;
const NODE_LABEL_SUB_MAX_WIDTH: f64 = 350.0;
const NODE_LABEL_SINGLE_MAX_WIDTH: f64 = 328.0;

#[derive(Clone, Debug)]
pub(crate) struct NetworkNodeMetrics {
    pub(crate) radius: f64,
    pub(crate) label: f64,
    pub(crate) collision_radius: f64,
    pub(crate) label_half_width: f64,
    pub(crate) label_bottom: f64,
}

pub(crate) fn project_node_metrics(node: Option<&Value>) -> NetworkNodeMetrics {
    let Some(row) = node.filter(|value| value.is_object()) else {
        return NetworkNodeMetrics {
            radius: 18.0,
            label: 20.0,
            collision_radius: 38.0,
            label_half_width: 24.0,
            label_bottom: 42.0,
        };
    };
    if truthy_field(row, "__networkPlacementMetrics") {
        let radius = number_field(row, "r")
            .or_else(|| number_field(row, "nodeRadius"))
            .unwrap_or(18.0)
            .max(9.0);
        let label = number_field(row, "labelPad").unwrap_or(0.0).max(0.0);
        return NetworkNodeMetrics {
            radius,
            label,
            collision_radius: number_field(row, "collisionRadius")
                .unwrap_or(radius + label)
                .max(0.0),
            label_half_width: number_field(row, "labelHalfWidth").unwrap_or(24.0).max(0.0),
            label_bottom: number_field(row, "labelBottom").unwrap_or(42.0).max(0.0),
        };
    }
    let radius = number_field(row, "r").unwrap_or(18.0).max(9.0);
    let profile = estimate_node_label_profile(row, radius);
    let name_len = text_field(row, "title")
        .or_else(|| text_field(row, "name"))
        .map(|text| text.trim().chars().count() as f64)
        .unwrap_or(0.0);
    let weighted = 12.0
        + (name_len * 0.52).min(26.0)
        + (name_len - 18.0).max(0.0) * 0.34
        + profile.text_density * 28.0
        + profile.bottom.max(0.0).sqrt() * 0.62;
    let label = clamp_layout(weighted, 18.0, 124.0);
    let collision_radius = (radius + label)
        .max(radius + profile.bottom * 0.64)
        .max(radius * 0.16 + profile.half_width * 0.9);

    NetworkNodeMetrics {
        radius,
        label,
        collision_radius,
        label_half_width: profile.half_width,
        label_bottom: profile.bottom,
    }
}

pub(crate) fn label_pads_for_ids(
    ids: &[String],
    node_by_id: Option<&Map<String, Value>>,
) -> Vec<f64> {
    ids.iter()
        .map(|id| {
            node_by_id
                .and_then(|rows| rows.get(id))
                .map(|node| project_node_metrics(Some(node)).label)
                .unwrap_or(0.0)
        })
        .collect()
}

pub(crate) fn project_network_node_metrics_contract(payload: &Value) -> Value {
    let rows = payload
        .get("nodes")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    json!({
        "nodeCount": rows.len(),
        "metrics": rows.iter().map(|row| {
            let metrics = project_node_metrics(Some(row));
            json!({
                "id": text_field(row, "id").unwrap_or_default(),
                "radius": number_json(round_two(metrics.radius)),
                "label": number_json(round_two(metrics.label)),
                "collisionRadius": number_json(round_two(metrics.collision_radius)),
                "labelHalfWidth": number_json(round_two(metrics.label_half_width)),
                "labelBottom": number_json(round_two(metrics.label_bottom)),
            })
        }).collect::<Vec<_>>(),
    })
}

struct LabelProfile {
    half_width: f64,
    bottom: f64,
    text_density: f64,
}

fn estimate_node_label_profile(node: &Value, radius: f64) -> LabelProfile {
    let r = radius.max(9.0);
    let base_size = clamp_layout(number_field(node, "fontSize").unwrap_or(13.0), 8.0, 36.0);
    let sub_size = (base_size - 1.0).max(8.0);
    let name = text_field(node, "name").unwrap_or_default();
    let title = text_field(node, "title")
        .or_else(|| text_field(node, "label"))
        .or_else(|| text_field(node, "id"))
        .unwrap_or_default();
    let display_id = resolve_display_label_text(node);
    let main_text = if !name.is_empty() {
        name.as_str()
    } else if !display_id.is_empty() {
        display_id.as_str()
    } else {
        title.as_str()
    };
    let sub_text = if !name.is_empty() {
        if !display_id.is_empty() {
            display_id.as_str()
        } else {
            title.as_str()
        }
    } else {
        ""
    };
    let single_text = if !name.is_empty() {
        ""
    } else if !display_id.is_empty() {
        display_id.as_str()
    } else {
        title.as_str()
    };
    let main_width =
        NODE_LABEL_MAIN_MAX_WIDTH.min(estimate_text_units(main_text) * base_size.max(1.0));
    let sub_width = NODE_LABEL_SUB_MAX_WIDTH.min(estimate_text_units(sub_text) * sub_size.max(1.0));
    let single_width =
        NODE_LABEL_SINGLE_MAX_WIDTH.min(estimate_text_units(single_text) * base_size.max(1.0));
    let half_width = 18.0_f64
        .max(main_width / 2.0)
        .max(sub_width / 2.0)
        .max(single_width / 2.0);
    let main_half = (base_size * 0.56).max(4.0);
    let sub_half = (sub_size * 0.56).max(4.0);
    let bottom = if !name.is_empty() {
        (r + 16.0 + main_half).max(r + 34.0 + sub_half)
    } else {
        (r + 20.0 + main_half).max(r)
    };

    LabelProfile {
        half_width,
        bottom,
        text_density: clamp_layout((half_width - 18.0) / 116.0, 0.0, 1.25),
    }
}

fn resolve_display_label_text(node: &Value) -> String {
    let direct = text_field(node, "display_id")
        .or_else(|| text_field(node, "displayId"))
        .or_else(|| text_field(node, "displayIdRaw"))
        .or_else(|| text_field(node, "id"))
        .unwrap_or_default();
    let ids = node
        .get("display_ids")
        .or_else(|| node.get("displayIds"))
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .filter_map(value_text)
                .map(|text| text.trim().to_string())
                .filter(|text| !text.is_empty())
                .take(3)
                .collect::<Vec<_>>()
        })
        .unwrap_or_default();
    if ids.is_empty() {
        return direct;
    }
    let grouped = ids.join("/");
    if grouped.chars().count() > direct.chars().count() {
        grouped
    } else {
        direct
    }
}

fn estimate_text_units(text: &str) -> f64 {
    if text.is_empty() {
        return 0.0;
    }
    text.chars()
        .map(|ch| {
            let cp = ch as u32;
            if ch.is_whitespace() {
                0.34
            } else if cp > 255 {
                1.03
            } else if ch.is_ascii_digit() {
                0.8
            } else if ch.is_ascii_uppercase() {
                0.72
            } else if ch.is_ascii_lowercase() {
                0.64
            } else {
                0.7
            }
        })
        .sum()
}

fn number_field(value: &Value, key: &str) -> Option<f64> {
    match value.get(key) {
        Some(Value::Number(number)) => number.as_f64().filter(|value| value.is_finite()),
        Some(Value::String(text)) => text
            .trim()
            .parse::<f64>()
            .ok()
            .filter(|value| value.is_finite()),
        Some(Value::Bool(value)) => Some(if *value { 1.0 } else { 0.0 }),
        Some(Value::Null) => Some(0.0),
        _ => None,
    }
}

fn text_field(value: &Value, key: &str) -> Option<String> {
    value
        .get(key)
        .and_then(value_text)
        .map(|text| text.trim().to_string())
}

fn truthy_field(value: &Value, key: &str) -> bool {
    match value.get(key) {
        Some(Value::Bool(value)) => *value,
        Some(Value::Number(number)) => number.as_f64().is_some_and(|value| value != 0.0),
        Some(Value::String(text)) => {
            let normalized = text.trim().to_ascii_lowercase();
            !normalized.is_empty() && normalized != "0" && normalized != "false"
        }
        _ => false,
    }
}

fn value_text(value: &Value) -> Option<String> {
    match value {
        Value::String(text) => Some(text.to_string()),
        Value::Number(number) => Some(number.to_string()),
        Value::Bool(value) => Some(value.to_string()),
        _ => None,
    }
}

fn clamp_layout(value: f64, min: f64, max: f64) -> f64 {
    value.max(min).min(max)
}

fn round_two(value: f64) -> f64 {
    ((value * 100.0) + 0.5).floor() / 100.0
}

fn number_json(value: f64) -> Value {
    if value.fract() == 0.0 && value >= i64::MIN as f64 && value <= i64::MAX as f64 {
        json!(value as i64)
    } else {
        json!(value)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn network_node_metrics_handles_multilingual_labels() {
        let metrics = project_node_metrics(Some(&json!({
            "id": "n1",
            "title": "账户流水节点",
            "name": "账户流水节点",
            "display_id": "ACC-001",
            "r": 20,
            "fontSize": 14
        })));

        assert_eq!(round_two(metrics.radius), 20.0);
        assert!(metrics.label > 24.0);
        assert!(metrics.collision_radius > metrics.radius + 20.0);
        assert!(metrics.label_half_width > 30.0);
    }

    #[test]
    fn network_node_metrics_prefers_grouped_display_ids_when_longer() {
        let output = project_network_node_metrics_contract(&json!({
            "nodes": [{
                "id": "n2",
                "display_id": "A",
                "display_ids": ["A", "B234", "C567"],
                "r": 18
            }]
        }));
        let row = &output["metrics"][0];

        assert_eq!(row["id"], json!("n2"));
        assert!(row["labelHalfWidth"].as_f64().unwrap_or_default() > 22.0);
    }
}
