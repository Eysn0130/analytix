use crate::flow_layout_network_sector::TWO_PI;
use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Value};
use std::f64::consts::PI;
use std::{fs, path::PathBuf};

pub(crate) struct NetworkHardCorridorScoreContractArgs {
    pub(crate) payload: Value,
}

struct HardCorridor {
    x1: f64,
    y1: f64,
    x2: f64,
    y2: f64,
    half_width: f64,
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<NetworkHardCorridorScoreContractArgs> {
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
                        "read network hard-corridor score contract input {}",
                        input_path.display()
                    )
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!(
                        "parse network hard-corridor score contract input {}",
                        input_path.display()
                    )
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute contract-flow-layout-network-hard-corridor-score <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported network hard-corridor score contract flag: {other}"),
        }
    }

    Ok(NetworkHardCorridorScoreContractArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_network_hard_corridor_score_contract(payload: &Value) -> Value {
    let config = payload.get("config").unwrap_or(payload);
    let bucket_count =
        js_or_default(number_field(config, "ANGULAR_BUCKETS"), 36.0).max(12.0) as usize;
    let r_leaf_min = js_or_default(number_field(config, "R_LEAF_MIN"), 220.0).max(80.0);
    let center = payload.get("center").unwrap_or(payload);
    let center_x = js_or_default(number_field(center, "x"), 0.0);
    let center_y = js_or_default(number_field(center, "y"), 0.0);
    let hard_corridors = read_hard_corridors(payload.get("hardCorridors"));
    let step = TWO_PI / (bucket_count as f64);
    let mut buckets = Vec::with_capacity(bucket_count);

    for index in 0..bucket_count {
        let angle = -PI + step * (index as f64);
        let x = center_x + angle.cos() * r_leaf_min;
        let y = center_y + angle.sin() * r_leaf_min;
        let hard_corridor_penalty = score_hard_corridor_penalty(x, y, &hard_corridors);
        buckets.push(json!({
            "index": index,
            "angle": angle,
            "x": x,
            "y": y,
            "hardCorridorPenalty": hard_corridor_penalty,
        }));
    }

    json!({
        "bucketCount": bucket_count,
        "buckets": buckets,
    })
}

fn score_hard_corridor_penalty(x: f64, y: f64, hard_corridors: &[HardCorridor]) -> f64 {
    let mut hard_corridor_penalty = 0.0;
    for corridor in hard_corridors {
        let dist =
            distance_point_to_segment(x, y, corridor.x1, corridor.y1, corridor.x2, corridor.y2);
        let threshold = corridor.half_width.max(8.0) + 20.0;
        if dist < threshold {
            hard_corridor_penalty += (threshold - dist) * 3.2 + 44.0;
        }
    }
    hard_corridor_penalty
}

fn distance_point_to_segment(px: f64, py: f64, x1: f64, y1: f64, x2: f64, y2: f64) -> f64 {
    let dx = x2 - x1;
    let dy = y2 - y1;
    let denom = dx * dx + dy * dy;
    if !denom.is_finite() || denom <= 1e-9 {
        return ((px - x1).powi(2) + (py - y1).powi(2)).sqrt();
    }
    let t_raw = ((px - x1) * dx + (py - y1) * dy) / denom;
    let t = if t_raw < 0.0 {
        0.0
    } else if t_raw > 1.0 {
        1.0
    } else {
        t_raw
    };
    let cx = x1 + dx * t;
    let cy = y1 + dy * t;
    ((px - cx).powi(2) + (py - cy).powi(2)).sqrt()
}

fn read_hard_corridors(value: Option<&Value>) -> Vec<HardCorridor> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .map(|row| HardCorridor {
                    x1: js_or_default(number_field(row, "x1"), 0.0),
                    y1: js_or_default(number_field(row, "y1"), 0.0),
                    x2: js_or_default(number_field(row, "x2"), 0.0),
                    y2: js_or_default(number_field(row, "y2"), 0.0),
                    half_width: js_or_default(number_field(row, "halfWidth"), 0.0),
                })
                .collect()
        })
        .unwrap_or_default()
}

fn js_or_default(value: Option<f64>, default_value: f64) -> f64 {
    match value {
        Some(value) if value != 0.0 && !value.is_nan() => value,
        _ => default_value,
    }
}

fn number_field(value: &Value, key: &str) -> Option<f64> {
    match value.get(key) {
        Some(Value::Number(number)) => number.as_f64(),
        Some(Value::String(text)) => text.trim().parse::<f64>().ok(),
        Some(Value::Bool(value)) => Some(if *value { 1.0 } else { 0.0 }),
        Some(Value::Null) => Some(0.0),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn network_plan_hard_corridor_score_matches_segment_penalty() {
        let output = project_network_hard_corridor_score_contract(&json!({
            "config": {"ANGULAR_BUCKETS": 12, "R_LEAF_MIN": 100},
            "center": {"x": 0, "y": 0},
            "hardCorridors": [{"x1": 96, "y1": -40, "x2": 96, "y2": 40, "halfWidth": 12}]
        }));
        let buckets = output["buckets"].as_array().expect("buckets");
        let hit_bucket = buckets
            .iter()
            .find(|row| row["index"] == json!(6))
            .expect("right bucket");
        let miss_bucket = buckets
            .iter()
            .find(|row| row["index"] == json!(0))
            .expect("left bucket");

        assert!(
            (hit_bucket["hardCorridorPenalty"]
                .as_f64()
                .unwrap_or_default()
                - 133.6)
                .abs()
                < 1e-9
        );
        assert_eq!(miss_bucket["hardCorridorPenalty"], json!(0.0));
    }

    #[test]
    fn network_plan_hard_corridor_score_preserves_js_zero_width_floor() {
        let corridors = vec![HardCorridor {
            x1: 0.0,
            y1: 0.0,
            x2: 0.0,
            y2: 0.0,
            half_width: 0.0,
        }];
        assert!((score_hard_corridor_penalty(0.0, 0.0, &corridors) - 133.6).abs() < 1e-9);
    }
}
