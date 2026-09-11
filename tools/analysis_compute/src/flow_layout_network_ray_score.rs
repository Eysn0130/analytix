use crate::flow_layout_network_sector::TWO_PI;
use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Value};
use std::{fs, path::PathBuf};

pub(crate) struct NetworkRayScoreContractArgs {
    pub(crate) payload: Value,
}

#[derive(Clone)]
struct RayZone {
    x: f64,
    y: f64,
    owner_center_id: String,
}

#[derive(Clone)]
struct RayIndexEntry {
    angle_pos: f64,
    radius: f64,
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<NetworkRayScoreContractArgs> {
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
                        "read network ray score contract input {}",
                        input_path.display()
                    )
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!(
                        "parse network ray score contract input {}",
                        input_path.display()
                    )
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute contract-flow-layout-network-ray-score <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported network ray score contract flag: {other}"),
        }
    }

    Ok(NetworkRayScoreContractArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_network_ray_score_contract(payload: &Value) -> Value {
    let center = payload.get("center").unwrap_or(payload);
    let target = payload.get("target").unwrap_or(payload);
    let center_x = js_or_default(number_field(center, "x"), 0.0);
    let center_y = js_or_default(number_field(center, "y"), 0.0);
    let target_x = js_or_default(number_field(target, "x"), 0.0);
    let target_y = js_or_default(number_field(target, "y"), 0.0);
    let owner_center_id = truthy_text_field(payload, "ownerCenterId");
    let bucket_count = js_or_default(number_field(payload, "bucketCount"), 240.0)
        .max(96.0)
        .min(720.0) as usize;
    let angle_threshold = clamp_f64(
        js_or_default(number_field(payload, "angleThreshold"), 0.08),
        0.02,
        0.24,
    );
    let radial_threshold = js_or_default(number_field(payload, "radialThreshold"), 34.0).max(12.0);
    let max_penalty = match number_field(payload, "maxPenalty") {
        Some(value) if value.is_finite() => value.max(0.0),
        _ => f64::INFINITY,
    };
    let zones = read_ray_zones(payload.get("zones"));
    let included_zone_count = zones
        .iter()
        .filter(|zone| should_include_zone(zone, &owner_center_id))
        .count();
    let direct_penalty = ray_cluster_penalty_direct(
        center_x,
        center_y,
        target_x,
        target_y,
        &zones,
        &owner_center_id,
        angle_threshold,
        radial_threshold,
        max_penalty,
    );
    let indexed_penalty = ray_cluster_penalty_indexed(
        center_x,
        center_y,
        target_x,
        target_y,
        &zones,
        &owner_center_id,
        bucket_count.max(1),
        angle_threshold,
        radial_threshold,
        max_penalty,
    );

    json!({
        "penalty": indexed_penalty,
        "directPenalty": direct_penalty,
        "indexedPenalty": indexed_penalty,
        "includedZoneCount": included_zone_count,
        "bucketCount": bucket_count,
    })
}

fn ray_cluster_penalty_direct(
    center_x: f64,
    center_y: f64,
    target_x: f64,
    target_y: f64,
    zones: &[RayZone],
    owner_center_id: &str,
    angle_threshold: f64,
    radial_threshold: f64,
    max_penalty: f64,
) -> f64 {
    let target_angle = (target_y - center_y).atan2(target_x - center_x);
    let target_radius = hypot(target_x - center_x, target_y - center_y);
    let mut penalty = 0.0;
    for zone in zones {
        if !should_include_zone(zone, owner_center_id) {
            continue;
        }
        let angle = (zone.y - center_y).atan2(zone.x - center_x);
        let radius = hypot(zone.x - center_x, zone.y - center_y);
        let ad = angle_diff(target_angle, angle);
        if ad >= angle_threshold {
            continue;
        }
        penalty += ray_zone_penalty(
            ad,
            (target_radius - radius).abs(),
            angle_threshold,
            radial_threshold,
        );
        if penalty > max_penalty {
            return penalty;
        }
    }
    penalty
}

fn ray_cluster_penalty_indexed(
    center_x: f64,
    center_y: f64,
    target_x: f64,
    target_y: f64,
    zones: &[RayZone],
    owner_center_id: &str,
    bucket_count: usize,
    angle_threshold: f64,
    radial_threshold: f64,
    max_penalty: f64,
) -> f64 {
    let bucket_step = TWO_PI / (bucket_count as f64);
    let mut buckets: Vec<Vec<RayIndexEntry>> = (0..bucket_count).map(|_| Vec::new()).collect();
    for zone in zones {
        if !should_include_zone(zone, owner_center_id) {
            continue;
        }
        let angle_pos = normalize_angle_positive((zone.y - center_y).atan2(zone.x - center_x));
        let radius = hypot(zone.x - center_x, zone.y - center_y);
        let idx_raw = (angle_pos / bucket_step).floor() as isize;
        let idx = idx_raw
            .max(0)
            .min((bucket_count.saturating_sub(1)) as isize) as usize;
        buckets[idx].push(RayIndexEntry { angle_pos, radius });
    }

    let target_angle = normalize_angle_positive((target_y - center_y).atan2(target_x - center_x));
    let target_radius = hypot(target_x - center_x, target_y - center_y);
    let base_idx = ((target_angle / bucket_step).floor() as isize)
        .max(0)
        .min((bucket_count.saturating_sub(1)) as isize);
    let span_buckets = ((angle_threshold / bucket_step).ceil() as isize + 1)
        .min((bucket_count.saturating_sub(1)) as isize)
        .max(0);
    let mut penalty = 0.0;

    for offset in -span_buckets..=span_buckets {
        let idx = (base_idx + offset).rem_euclid(bucket_count as isize) as usize;
        for row in &buckets[idx] {
            let ad = angle_diff(target_angle, row.angle_pos);
            if ad >= angle_threshold {
                continue;
            }
            penalty += ray_zone_penalty(
                ad,
                (target_radius - row.radius).abs(),
                angle_threshold,
                radial_threshold,
            );
            if penalty > max_penalty {
                return penalty;
            }
        }
    }
    penalty
}

fn ray_zone_penalty(
    angle_diff_value: f64,
    radial_diff: f64,
    angle_threshold: f64,
    radial_threshold: f64,
) -> f64 {
    let angle_weight = (angle_threshold - angle_diff_value) / angle_threshold.max(1e-6);
    let radial_weight = if radial_diff < radial_threshold {
        1.85
    } else if radial_diff < radial_threshold * 2.1 {
        1.12
    } else {
        0.55
    };
    angle_weight * radial_weight * 22.0
}

fn should_include_zone(zone: &RayZone, owner_center_id: &str) -> bool {
    owner_center_id.is_empty()
        || zone.owner_center_id.is_empty()
        || zone.owner_center_id == owner_center_id
}

fn read_ray_zones(value: Option<&Value>) -> Vec<RayZone> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .filter(|row| !row.is_null())
                .map(|row| RayZone {
                    x: js_or_default(number_field(row, "x"), 0.0),
                    y: js_or_default(number_field(row, "y"), 0.0),
                    owner_center_id: truthy_text_field(row, "ownerCenterId"),
                })
                .collect()
        })
        .unwrap_or_default()
}

fn angle_diff(a: f64, b: f64) -> f64 {
    let x = normalize_angle_positive(a);
    let y = normalize_angle_positive(b);
    let raw = (x - y).abs();
    raw.min(TWO_PI - raw)
}

fn normalize_angle_positive(angle: f64) -> f64 {
    if !angle.is_finite() {
        return 0.0;
    }
    let mut value = angle % TWO_PI;
    if !value.is_finite() {
        return 0.0;
    }
    if value < 0.0 {
        value += TWO_PI;
    }
    value
}

fn hypot(dx: f64, dy: f64) -> f64 {
    (dx * dx + dy * dy).sqrt()
}

fn clamp_f64(value: f64, min: f64, max: f64) -> f64 {
    value.max(min).min(max)
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
        _ => None,
    }
}

fn truthy_text_field(value: &Value, key: &str) -> String {
    match value.get(key) {
        Some(Value::String(text)) if !text.is_empty() => text.to_string(),
        Some(Value::Number(number)) => match number.as_f64() {
            Some(value) if value != 0.0 && !value.is_nan() => number.to_string(),
            _ => String::new(),
        },
        Some(Value::Bool(true)) => "true".to_string(),
        _ => String::new(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn network_ray_score_filters_owner_and_caps_penalty() {
        let contract = project_network_ray_score_contract(&json!({
            "center": {"x": 12, "y": -8},
            "target": {"x": 172, "y": -6},
            "zones": [
                {"x": 102, "y": -8, "ownerCenterId": "center"},
                {"x": 152, "y": -4, "ownerCenterId": "other"},
                {"x": 214, "y": 6}
            ],
            "ownerCenterId": "center",
            "angleThreshold": 0.09,
            "radialThreshold": 42,
            "maxPenalty": 30,
            "bucketCount": 192
        }));

        assert_eq!(contract["includedZoneCount"], json!(2));
        assert_eq!(contract["bucketCount"], json!(192));
        assert_eq!(contract["penalty"], contract["indexedPenalty"]);
        assert!(contract["directPenalty"].as_f64().unwrap_or(0.0) > 0.0);
        assert!(contract["indexedPenalty"].as_f64().unwrap_or(0.0) > 0.0);
    }
}
