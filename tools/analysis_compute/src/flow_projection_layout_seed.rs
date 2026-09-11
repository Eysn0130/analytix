use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Value};
use std::{collections::HashMap, fs, path::PathBuf};

pub(crate) struct ProjectionLayoutSeedArgs {
    pub(crate) payload: Value,
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<ProjectionLayoutSeedArgs> {
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
                    format!("read projection layout seed input {}", input_path.display())
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!(
                        "parse projection layout seed input {}",
                        input_path.display()
                    )
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute project-flow-projection-layout-seed <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported projection layout seed flag: {other}"),
        }
    }

    Ok(ProjectionLayoutSeedArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_projection_layout_seed(payload: &Value) -> Value {
    let nodes = clone_array(
        payload
            .get("nodes")
            .or_else(|| payload.get("nextNodes"))
            .or_else(|| payload.get("next_nodes")),
    );
    let base_nodes = clone_array(
        payload
            .get("baseNodes")
            .or_else(|| payload.get("base_nodes"))
            .or_else(|| payload.get("previousNodes"))
            .or_else(|| payload.get("previous_nodes")),
    );

    let mut summary = SeedSummary::default();
    if nodes.is_empty() || base_nodes.is_empty() {
        return seed_result(Vec::new(), summary);
    }

    let mut by_id: HashMap<String, Point> = HashMap::new();
    let mut cluster_centers: HashMap<String, Point> = HashMap::new();
    let mut center_x = 0.0f64;
    let mut center_y = 0.0f64;
    let mut center_count = 0usize;

    for node in &base_nodes {
        let id = text_value(node.get("id"));
        let x = js_number(node.get("x"));
        let y = js_number(node.get("y"));
        let (Some(x), Some(y)) = (x, y) else {
            continue;
        };
        if id.is_empty() {
            continue;
        }
        let point = Point { x, y };
        by_id.insert(id.clone(), point);
        center_x += x;
        center_y += y;
        center_count += 1;
        if is_cluster_projection_node(node) {
            cluster_centers.insert(id, point);
        }
    }

    if by_id.is_empty() {
        return seed_result(Vec::new(), summary);
    }

    let fallback_center = Point {
        x: if center_count > 0 {
            center_x / center_count as f64
        } else {
            0.0
        },
        y: if center_count > 0 {
            center_y / center_count as f64
        } else {
            0.0
        },
    };

    let mut updates = Vec::new();
    for (index, node) in nodes.iter().enumerate() {
        if !node.is_object() {
            continue;
        }
        if js_number(node.get("x")).is_some() && js_number(node.get("y")).is_some() {
            continue;
        }
        let id = text_value(node.get("id"));
        if id.is_empty() {
            continue;
        }
        if let Some(point) = by_id.get(&id) {
            summary.reused += 1;
            updates.push(seed_update(index, &id, *point, "reused"));
            continue;
        }
        let cluster_id = text_value(first_truthy(
            node.get("projection_cluster_id"),
            node.get("cluster_id"),
        ));
        if let Some(point) = cluster_centers
            .get(&cluster_id)
            .or_else(|| by_id.get(&cluster_id))
        {
            let seeded = Point {
                x: point.x + stable_projection_node_jitter(&id, "x") as f64 * 2.8,
                y: point.y + stable_projection_node_jitter(&id, "y") as f64 * 2.8,
            };
            summary.cluster_seeded += 1;
            updates.push(seed_update(index, &id, seeded, "cluster"));
            continue;
        }
        let seeded = Point {
            x: fallback_center.x + ((index % 17) as f64 - 8.0) * 18.0,
            y: fallback_center.y + (((index / 17) % 17) as f64 - 8.0) * 18.0,
        };
        summary.fallback_seeded += 1;
        updates.push(seed_update(index, &id, seeded, "fallback"));
    }

    seed_result(updates, summary)
}

#[derive(Clone, Copy)]
struct Point {
    x: f64,
    y: f64,
}

#[derive(Default)]
struct SeedSummary {
    reused: usize,
    cluster_seeded: usize,
    fallback_seeded: usize,
}

fn seed_result(updates: Vec<Value>, summary: SeedSummary) -> Value {
    json!({
        "summary": {
            "reused": summary.reused,
            "clusterSeeded": summary.cluster_seeded,
            "fallbackSeeded": summary.fallback_seeded,
        },
        "updates": updates,
    })
}

fn seed_update(index: usize, id: &str, point: Point, source: &str) -> Value {
    json!({
        "index": index,
        "id": id,
        "x": numeric_json(point.x),
        "y": numeric_json(point.y),
        "source": source,
    })
}

fn stable_projection_node_jitter(node_id: &str, axis: &str) -> i64 {
    let axis_offset = if axis == "y" { 17u32 } else { 0u32 };
    let mut hash = 2166136261u32;
    for code_unit in node_id.encode_utf16() {
        hash ^= u32::from(code_unit).wrapping_add(axis_offset);
        hash = hash.wrapping_mul(16777619);
    }
    i64::from(hash % 31) - 15
}

fn is_cluster_projection_node(node: &Value) -> bool {
    js_truthy(node.get("cluster_node")) || text_value(node.get("id")).starts_with("__cluster__::")
}

fn first_truthy<'a>(first: Option<&'a Value>, second: Option<&'a Value>) -> Option<&'a Value> {
    first.filter(|value| js_truthy(Some(*value))).or(second)
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

fn js_number(value: Option<&Value>) -> Option<f64> {
    let number = match value {
        Some(Value::Null) => 0.0,
        Some(Value::Bool(value)) => {
            if *value {
                1.0
            } else {
                0.0
            }
        }
        Some(Value::Number(number)) => number.as_f64()?,
        Some(Value::String(text)) => {
            let text = text.trim();
            if text.is_empty() {
                0.0
            } else {
                text.parse::<f64>().ok()?
            }
        }
        Some(Value::Array(_)) | Some(Value::Object(_)) | None => return None,
    };
    if number.is_finite() {
        Some(number)
    } else {
        None
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

fn numeric_json(number: f64) -> Value {
    if number.fract() == 0.0 && number >= i64::MIN as f64 && number <= i64::MAX as f64 {
        json!(number as i64)
    } else {
        json!(number)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn projection_layout_seed_matches_js_seed_summary_and_updates() {
        let payload = json!({
            "nodes": [
                {"id": "same"},
                {"id": "child", "projection_cluster_id": "__cluster__::a"},
                {"id": "fallback"},
                {"id": "already", "x": 7, "y": 8}
            ],
            "baseNodes": [
                {"id": "same", "x": 10, "y": 20},
                {"id": "__cluster__::a", "x": 100, "y": 200, "cluster_node": true}
            ]
        });

        assert_eq!(
            project_projection_layout_seed(&payload),
            json!({
                "summary": {"reused": 1, "clusterSeeded": 1, "fallbackSeeded": 1},
                "updates": [
                    {"index": 0, "id": "same", "x": 10, "y": 20, "source": "reused"},
                    {"index": 1, "id": "child", "x": 125.2, "y": 236.4, "source": "cluster"},
                    {"index": 2, "id": "fallback", "x": -53, "y": -34, "source": "fallback"}
                ],
            })
        );
        assert_eq!(stable_projection_node_jitter("node-new", "x"), 0);
        assert_eq!(stable_projection_node_jitter("node-new", "y"), -6);
    }
}
