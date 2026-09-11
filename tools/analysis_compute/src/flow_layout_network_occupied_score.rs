use crate::flow_layout_network_sector::TWO_PI;
use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Value};
use std::f64::consts::PI;
use std::{fs, path::PathBuf};

pub(crate) struct NetworkOccupiedScoreContractArgs {
    pub(crate) payload: Value,
}

struct CircleZone {
    x: f64,
    y: f64,
    r: f64,
}

struct ClusterBox {
    min_x: f64,
    min_y: f64,
    max_x: f64,
    max_y: f64,
    weight: f64,
}

struct OccupiedScore {
    blankness: f64,
    local_collision_penalty: f64,
    global_collision_penalty: f64,
    label_crowding_penalty: f64,
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<NetworkOccupiedScoreContractArgs> {
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
                        "read network occupied score contract input {}",
                        input_path.display()
                    )
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!(
                        "parse network occupied score contract input {}",
                        input_path.display()
                    )
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute contract-flow-layout-network-occupied-score <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported network occupied score contract flag: {other}"),
        }
    }

    Ok(NetworkOccupiedScoreContractArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_network_occupied_score_contract(payload: &Value) -> Value {
    let config = payload.get("config").unwrap_or(payload);
    let bucket_count =
        js_or_default(number_field(config, "ANGULAR_BUCKETS"), 36.0).max(12.0) as usize;
    let r_leaf_min = js_or_default(number_field(config, "R_LEAF_MIN"), 220.0).max(80.0);
    let center = payload.get("center").unwrap_or(payload);
    let center_x = js_or_default(number_field(center, "x"), 0.0);
    let center_y = js_or_default(number_field(center, "y"), 0.0);
    let local_map = payload.get("localMap").unwrap_or(&Value::Null);
    let global_map = payload.get("globalMap").unwrap_or(&Value::Null);
    let local_zones = read_circle_zones(local_map.get("zones"));
    let label_zones = read_circle_zones(local_map.get("labelZones"));
    let global_leaf_zones = read_circle_zones(global_map.get("leafZones"));
    let cluster_boxes = read_cluster_boxes(global_map.get("clusterBBoxes"));
    let step = TWO_PI / (bucket_count as f64);
    let mut buckets = Vec::with_capacity(bucket_count);

    for index in 0..bucket_count {
        let angle = -PI + step * (index as f64);
        let x = center_x + angle.cos() * r_leaf_min;
        let y = center_y + angle.sin() * r_leaf_min;
        let score = score_occupied_bucket(
            x,
            y,
            &local_zones,
            &global_leaf_zones,
            &cluster_boxes,
            &label_zones,
        );
        buckets.push(json!({
            "index": index,
            "angle": angle,
            "x": x,
            "y": y,
            "blankness": score.blankness,
            "localCollisionPenalty": score.local_collision_penalty,
            "globalCollisionPenalty": score.global_collision_penalty,
            "labelCrowdingPenalty": score.label_crowding_penalty,
        }));
    }

    json!({
        "bucketCount": bucket_count,
        "buckets": buckets,
    })
}

fn score_occupied_bucket(
    x: f64,
    y: f64,
    local_zones: &[CircleZone],
    global_leaf_zones: &[CircleZone],
    cluster_boxes: &[ClusterBox],
    label_zones: &[CircleZone],
) -> OccupiedScore {
    let mut blankness = 120.0;
    let mut local_collision_penalty = 0.0;
    let mut global_collision_penalty = 0.0;
    let mut label_crowding_penalty = 0.0;

    for zone in local_zones {
        let dist = hypot(zone.x - x, zone.y - y);
        let threshold = zone.r + 14.0;
        if dist < threshold {
            local_collision_penalty += (threshold - dist) * 0.9;
        }
        blankness += ((dist - threshold) * 0.02).max(0.0).min(8.0);
    }

    for zone in global_leaf_zones {
        let dist = hypot(zone.x - x, zone.y - y);
        let threshold = zone.r + 16.0;
        if dist < threshold {
            global_collision_penalty += threshold - dist;
        }
    }

    for bbox in cluster_boxes {
        let dist = distance_to_box(x, y, bbox);
        if dist < 18.0 {
            global_collision_penalty += (18.0 - dist) * 1.8 * bbox.weight;
        }
        blankness += (dist * 0.04).min(12.0);
    }

    for zone in label_zones {
        let dist = hypot(zone.x - x, zone.y - y);
        let threshold = zone.r + 20.0;
        if dist < threshold {
            label_crowding_penalty += (threshold - dist) * 0.85;
        }
    }

    OccupiedScore {
        blankness,
        local_collision_penalty,
        global_collision_penalty,
        label_crowding_penalty,
    }
}

fn distance_to_box(x: f64, y: f64, bbox: &ClusterBox) -> f64 {
    let dx = if x < bbox.min_x {
        bbox.min_x - x
    } else if x > bbox.max_x {
        x - bbox.max_x
    } else {
        0.0
    };
    let dy = if y < bbox.min_y {
        bbox.min_y - y
    } else if y > bbox.max_y {
        y - bbox.max_y
    } else {
        0.0
    };
    hypot(dx, dy)
}

fn hypot(dx: f64, dy: f64) -> f64 {
    (dx * dx + dy * dy).sqrt()
}

fn read_circle_zones(value: Option<&Value>) -> Vec<CircleZone> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .map(|row| CircleZone {
                    x: js_or_default(number_field(row, "x"), 0.0),
                    y: js_or_default(number_field(row, "y"), 0.0),
                    r: js_or_default(number_field(row, "r"), 0.0),
                })
                .collect()
        })
        .unwrap_or_default()
}

fn read_cluster_boxes(value: Option<&Value>) -> Vec<ClusterBox> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .map(|row| ClusterBox {
                    min_x: js_or_default(number_field(row, "minX"), 0.0),
                    min_y: js_or_default(number_field(row, "minY"), 0.0),
                    max_x: js_or_default(number_field(row, "maxX"), 0.0),
                    max_y: js_or_default(number_field(row, "maxY"), 0.0),
                    weight: js_or_default(number_field(row, "weight"), 1.0).clamp(0.25, 2.2),
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
    fn network_plan_occupied_score_matches_circle_and_box_penalties() {
        let output = project_network_occupied_score_contract(&json!({
            "config": {"ANGULAR_BUCKETS": 12, "R_LEAF_MIN": 100},
            "center": {"x": 0, "y": 0},
            "localMap": {
                "zones": [{"x": 100, "y": 0, "r": 60}],
                "labelZones": [{"x": 100, "y": 0, "r": 20}]
            },
            "globalMap": {
                "leafZones": [{"x": -100, "y": 0, "r": 40}],
                "clusterBBoxes": [{"minX": 95, "minY": -10, "maxX": 105, "maxY": 10, "weight": 2}]
            }
        }));
        let buckets = output["buckets"].as_array().expect("buckets");
        let hit_bucket = buckets
            .iter()
            .find(|row| row["index"] == json!(6))
            .expect("right bucket");

        assert!(
            (hit_bucket["localCollisionPenalty"]
                .as_f64()
                .unwrap_or_default()
                - 66.6)
                .abs()
                < 1e-9
        );
        assert!(
            (hit_bucket["globalCollisionPenalty"]
                .as_f64()
                .unwrap_or_default()
                - 64.8)
                .abs()
                < 1e-9
        );
        assert!(
            (hit_bucket["labelCrowdingPenalty"]
                .as_f64()
                .unwrap_or_default()
                - 34.0)
                .abs()
                < 1e-9
        );
        assert_eq!(hit_bucket["blankness"], json!(120.0));
    }

    #[test]
    fn network_plan_occupied_score_preserves_js_zero_radius_defaults() {
        let local_zones = vec![CircleZone {
            x: 0.0,
            y: 0.0,
            r: 0.0,
        }];
        let label_zones = vec![CircleZone {
            x: 0.0,
            y: 0.0,
            r: 0.0,
        }];
        let score = score_occupied_bucket(0.0, 0.0, &local_zones, &[], &[], &label_zones);

        assert!((score.local_collision_penalty - 12.6).abs() < 1e-9);
        assert_eq!(score.blankness, 120.0);
        assert_eq!(score.label_crowding_penalty, 17.0);
    }
}
