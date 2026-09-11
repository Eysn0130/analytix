use crate::flow_layout_network_sector::TWO_PI;
use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Value};
use std::f64::consts::PI;
use std::{fs, path::PathBuf};

pub(crate) struct NetworkBoundaryScoreContractArgs {
    pub(crate) payload: Value,
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<NetworkBoundaryScoreContractArgs> {
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
                        "read network boundary score contract input {}",
                        input_path.display()
                    )
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!(
                        "parse network boundary score contract input {}",
                        input_path.display()
                    )
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute contract-flow-layout-network-boundary-score <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported network boundary score contract flag: {other}"),
        }
    }

    Ok(NetworkBoundaryScoreContractArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_network_boundary_score_contract(payload: &Value) -> Value {
    let config = payload.get("config").unwrap_or(payload);
    let bucket_count =
        js_or_default(number_field(config, "ANGULAR_BUCKETS"), 36.0).max(12.0) as usize;
    let r_leaf_min = js_or_default(number_field(config, "R_LEAF_MIN"), 220.0).max(80.0);
    let center = payload.get("center").unwrap_or(payload);
    let center_x = js_or_default(number_field(center, "x"), 0.0);
    let center_y = js_or_default(number_field(center, "y"), 0.0);
    let safe_bounds = payload.get("safeBounds").unwrap_or(&Value::Null);
    let step = TWO_PI / (bucket_count as f64);
    let mut buckets = Vec::with_capacity(bucket_count);

    for index in 0..bucket_count {
        let angle = -PI + step * (index as f64);
        let x = center_x + angle.cos() * r_leaf_min;
        let y = center_y + angle.sin() * r_leaf_min;
        let boundary_penalty = score_boundary_penalty(x, y, safe_bounds);
        buckets.push(json!({
            "index": index,
            "angle": angle,
            "x": x,
            "y": y,
            "boundaryPenalty": boundary_penalty,
        }));
    }

    json!({
        "bucketCount": bucket_count,
        "buckets": buckets,
    })
}

fn score_boundary_penalty(x: f64, y: f64, safe_bounds: &Value) -> f64 {
    let min_x = js_or_default(number_field(safe_bounds, "minX"), f64::NEG_INFINITY);
    let max_x = js_or_default(number_field(safe_bounds, "maxX"), f64::INFINITY);
    let min_y = js_or_default(number_field(safe_bounds, "minY"), f64::NEG_INFINITY);
    let max_y = js_or_default(number_field(safe_bounds, "maxY"), f64::INFINITY);
    let mut boundary_penalty = 0.0;
    if x < min_x {
        boundary_penalty += (js_or_default(number_field(safe_bounds, "minX"), 0.0) - x) * 0.8;
    }
    if x > max_x {
        boundary_penalty += (x - js_or_default(number_field(safe_bounds, "maxX"), 0.0)) * 0.8;
    }
    if y < min_y {
        boundary_penalty += (js_or_default(number_field(safe_bounds, "minY"), 0.0) - y) * 0.8;
    }
    if y > max_y {
        boundary_penalty += (y - js_or_default(number_field(safe_bounds, "maxY"), 0.0)) * 0.8;
    }
    boundary_penalty
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
    fn network_plan_boundary_score_matches_safe_bounds_penalty() {
        let output = project_network_boundary_score_contract(&json!({
            "config": {"ANGULAR_BUCKETS": 12, "R_LEAF_MIN": 100},
            "center": {"x": 0, "y": 0},
            "safeBounds": {"minX": -40, "maxX": 60, "minY": -50, "maxY": 70}
        }));
        let buckets = output["buckets"].as_array().expect("buckets");
        let left_bucket = buckets
            .iter()
            .find(|row| row["index"] == json!(0))
            .expect("left bucket");
        let right_bucket = buckets
            .iter()
            .find(|row| row["index"] == json!(6))
            .expect("right bucket");

        assert_eq!(left_bucket["boundaryPenalty"], json!(48.0));
        assert_eq!(right_bucket["boundaryPenalty"], json!(32.0));
    }

    #[test]
    fn network_plan_boundary_score_preserves_js_zero_bound_default() {
        assert_eq!(score_boundary_penalty(10.0, 0.0, &json!({"maxX": 0})), 0.0);
    }
}
