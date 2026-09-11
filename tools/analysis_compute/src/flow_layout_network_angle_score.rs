use crate::flow_layout_network_model::NetworkAngleRange;
use crate::flow_layout_network_sector::TWO_PI;
use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Value};
use std::f64::consts::PI;
use std::{fs, path::PathBuf};

pub(crate) struct NetworkAngleScoreContractArgs {
    pub(crate) payload: Value,
}

struct PreferredAngle {
    angle: f64,
    width: f64,
    weight: f64,
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<NetworkAngleScoreContractArgs> {
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
                        "read network angle score contract input {}",
                        input_path.display()
                    )
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!(
                        "parse network angle score contract input {}",
                        input_path.display()
                    )
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute contract-flow-layout-network-angle-score <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported network angle score contract flag: {other}"),
        }
    }

    Ok(NetworkAngleScoreContractArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_network_angle_score_contract(payload: &Value) -> Value {
    let config = payload.get("config").unwrap_or(payload);
    let bucket_count = number_field(config, "ANGULAR_BUCKETS")
        .or_else(|| number_field(payload, "bucketCount"))
        .or_else(|| number_field(payload, "bucket_count"))
        .map(|value| if value == 0.0 { 36.0 } else { value })
        .unwrap_or(36.0)
        .max(12.0) as usize;
    let territory_ranges = read_angle_ranges(payload.get("territoryRanges"));
    let preferred_angles = read_preferred_angles(payload.get("preferredAngles"));
    let allow_territory_overflow = bool_field(payload, "allowTerritoryOverflow");
    let buckets = score_angle_constraint_buckets(
        &territory_ranges,
        &preferred_angles,
        allow_territory_overflow,
        bucket_count,
    );

    json!({
        "bucketCount": bucket_count,
        "buckets": buckets,
    })
}

fn score_angle_constraint_buckets(
    territory_ranges: &[NetworkAngleRange],
    preferred_angles: &[PreferredAngle],
    allow_territory_overflow: bool,
    bucket_count: usize,
) -> Vec<Value> {
    let bucket_count = bucket_count.max(12);
    let step = TWO_PI / (bucket_count as f64);
    let mut out = Vec::with_capacity(bucket_count);
    for index in 0..bucket_count {
        let angle = -PI + step * (index as f64);
        let territory_penalty = if is_angle_in_ranges(angle, territory_ranges) {
            0.0
        } else if allow_territory_overflow {
            260.0
        } else {
            1200.0
        };
        let mut direction_bonus = 0.0;
        for row in preferred_angles {
            let diff = angle_diff(angle, row.angle);
            let gain = (1.0 - diff / row.width).max(0.0);
            direction_bonus += gain * row.weight * 64.0;
        }
        out.push(json!({
            "index": index,
            "angle": angle,
            "territoryPenalty": territory_penalty,
            "directionBonus": direction_bonus,
            "angleConstraintScore": direction_bonus - territory_penalty,
        }));
    }
    out
}

fn read_preferred_angles(value: Option<&Value>) -> Vec<PreferredAngle> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .filter_map(|row| {
                    let angle = number_field(row, "angle")?;
                    Some(PreferredAngle {
                        angle,
                        width: number_field(row, "width").unwrap_or(1.55).clamp(0.35, PI),
                        weight: number_field(row, "weight").unwrap_or(1.0).clamp(-3.2, 3.2),
                    })
                })
                .collect()
        })
        .unwrap_or_default()
}

fn read_angle_ranges(value: Option<&Value>) -> Vec<NetworkAngleRange> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .filter_map(|row| {
                    Some(NetworkAngleRange {
                        start: number_field(row, "start")?,
                        end: number_field(row, "end")?,
                        span: number_field(row, "span")?,
                    })
                })
                .collect()
        })
        .unwrap_or_default()
}

fn is_angle_in_ranges(angle: f64, ranges: &[NetworkAngleRange]) -> bool {
    if ranges.is_empty() {
        return true;
    }
    let value = normalize_angle_positive(angle);
    for row in ranges {
        let start = normalize_angle_positive(row.start);
        let end = normalize_angle_positive(row.end);
        if row.span.is_finite() && row.span >= TWO_PI - 1e-3 {
            return true;
        }
        if start <= end {
            if value >= start && value <= end {
                return true;
            }
        } else if value >= start || value <= end {
            return true;
        }
    }
    false
}

fn angle_diff(a: f64, b: f64) -> f64 {
    let x = normalize_angle_positive(a);
    let y = normalize_angle_positive(b);
    let raw = (x - y).abs();
    raw.min(TWO_PI - raw)
}

fn normalize_angle_positive(value: f64) -> f64 {
    let mut out = value % TWO_PI;
    if out < 0.0 {
        out += TWO_PI;
    }
    out
}

fn bool_field(value: &Value, key: &str) -> bool {
    match value.get(key) {
        Some(Value::Bool(value)) => *value,
        Some(Value::String(text)) => matches!(text.trim(), "true" | "1"),
        Some(Value::Number(number)) => number.as_f64().unwrap_or(0.0) != 0.0,
        _ => false,
    }
}

fn number_field(value: &Value, key: &str) -> Option<f64> {
    match value.get(key) {
        Some(Value::Number(number)) => number.as_f64().filter(|value| value.is_finite()),
        Some(Value::String(text)) => text
            .trim()
            .parse::<f64>()
            .ok()
            .filter(|value| value.is_finite()),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn network_plan_angle_score_applies_territory_penalty_and_direction_bonus() {
        let output = project_network_angle_score_contract(&json!({
            "config": {"ANGULAR_BUCKETS": 12},
            "territoryRanges": [{"start": -0.5, "end": 0.5, "span": 1.0}],
            "preferredAngles": [{"angle": 0, "width": 1.0, "weight": 2.0}],
            "allowTerritoryOverflow": false
        }));
        let buckets = output["buckets"].as_array().expect("buckets");
        let center_bucket = buckets
            .iter()
            .find(|row| row["index"] == json!(6))
            .expect("zero angle bucket");
        let outside_bucket = buckets
            .iter()
            .find(|row| row["index"] == json!(0))
            .expect("outside bucket");

        assert_eq!(center_bucket["territoryPenalty"], json!(0.0));
        assert_eq!(center_bucket["directionBonus"], json!(128.0));
        assert_eq!(outside_bucket["territoryPenalty"], json!(1200.0));
    }
}
