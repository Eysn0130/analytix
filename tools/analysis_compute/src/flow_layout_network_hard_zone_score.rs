use crate::flow_layout_network_sector::TWO_PI;
use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Value};
use std::f64::consts::PI;
use std::{fs, path::PathBuf};

pub(crate) struct NetworkHardZoneScoreContractArgs {
    pub(crate) payload: Value,
}

struct HardZone {
    x: f64,
    y: f64,
    r: f64,
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<NetworkHardZoneScoreContractArgs> {
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
                        "read network hard-zone score contract input {}",
                        input_path.display()
                    )
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!(
                        "parse network hard-zone score contract input {}",
                        input_path.display()
                    )
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute contract-flow-layout-network-hard-zone-score <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported network hard-zone score contract flag: {other}"),
        }
    }

    Ok(NetworkHardZoneScoreContractArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_network_hard_zone_score_contract(payload: &Value) -> Value {
    let config = payload.get("config").unwrap_or(payload);
    let bucket_count =
        js_or_default(number_field(config, "ANGULAR_BUCKETS"), 36.0).max(12.0) as usize;
    let r_leaf_min = js_or_default(number_field(config, "R_LEAF_MIN"), 220.0).max(80.0);
    let center = payload.get("center").unwrap_or(payload);
    let center_x = js_or_default(number_field(center, "x"), 0.0);
    let center_y = js_or_default(number_field(center, "y"), 0.0);
    let hard_zones = read_hard_zones(payload.get("hardZones"));
    let step = TWO_PI / (bucket_count as f64);
    let mut buckets = Vec::with_capacity(bucket_count);

    for index in 0..bucket_count {
        let angle = -PI + step * (index as f64);
        let x = center_x + angle.cos() * r_leaf_min;
        let y = center_y + angle.sin() * r_leaf_min;
        let hard_collision_penalty = score_hard_zone_penalty(x, y, &hard_zones);
        buckets.push(json!({
            "index": index,
            "angle": angle,
            "x": x,
            "y": y,
            "hardCollisionPenalty": hard_collision_penalty,
        }));
    }

    json!({
        "bucketCount": bucket_count,
        "buckets": buckets,
    })
}

fn score_hard_zone_penalty(x: f64, y: f64, hard_zones: &[HardZone]) -> f64 {
    let mut hard_collision_penalty = 0.0;
    for zone in hard_zones {
        let dx = zone.x - x;
        let dy = zone.y - y;
        let dist = (dx * dx + dy * dy).sqrt();
        let threshold = zone.r + 22.0;
        if dist < threshold {
            hard_collision_penalty += (threshold - dist) * 2.6 + 38.0;
        }
    }
    hard_collision_penalty
}

fn read_hard_zones(value: Option<&Value>) -> Vec<HardZone> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .map(|row| HardZone {
                    x: js_or_default(number_field(row, "x"), 0.0),
                    y: js_or_default(number_field(row, "y"), 0.0),
                    r: js_or_default(number_field(row, "r"), 0.0),
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
    fn network_plan_hard_zone_score_matches_circle_penalty() {
        let output = project_network_hard_zone_score_contract(&json!({
            "config": {"ANGULAR_BUCKETS": 12, "R_LEAF_MIN": 100},
            "center": {"x": 0, "y": 0},
            "hardZones": [{"x": 100, "y": 0, "r": 60}]
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
            (hit_bucket["hardCollisionPenalty"]
                .as_f64()
                .unwrap_or_default()
                - 251.2)
                .abs()
                < 1e-9
        );
        assert_eq!(miss_bucket["hardCollisionPenalty"], json!(0.0));
    }

    #[test]
    fn network_plan_hard_zone_score_preserves_js_zero_radius_default() {
        let zones = vec![HardZone {
            x: 0.0,
            y: 0.0,
            r: 0.0,
        }];
        assert_eq!(score_hard_zone_penalty(0.0, 0.0, &zones), 95.2);
    }
}
