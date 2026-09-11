use crate::flow_layout_network_model::{NetworkAngleRange, NetworkOwnerCenter};
use crate::flow_layout_network_sector::TWO_PI;
use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Map, Value};
use std::cmp::Ordering;
use std::f64::consts::PI;
use std::{fs, path::PathBuf};

pub(crate) struct NetworkTerritoryContractArgs {
    pub(crate) payload: Value,
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<NetworkTerritoryContractArgs> {
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
                        "read network territory contract input {}",
                        input_path.display()
                    )
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!(
                        "parse network territory contract input {}",
                        input_path.display()
                    )
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute contract-flow-layout-network-territory <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported network territory contract flag: {other}"),
        }
    }

    Ok(NetworkTerritoryContractArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_network_territory_contract(payload: &Value) -> Value {
    let centers = read_owner_centers(payload.get("centers"));
    let center_ids = read_string_vec(payload.get("centerIds"));
    let selected_ids = if center_ids.is_empty() {
        centers.iter().map(|row| row.id.clone()).collect::<Vec<_>>()
    } else {
        center_ids
    };
    let sample_radius = number_field(payload, "sampleRadius")
        .or_else(|| number_field(payload, "sample_radius"))
        .unwrap_or(0.0);
    let bucket_count = number_field(payload, "bucketCount")
        .or_else(|| number_field(payload, "bucket_count"))
        .filter(|value| *value > 0.0)
        .map(|value| value as usize)
        .unwrap_or(192);
    let guard = number_field(payload, "guard")
        .filter(|value| *value != 0.0)
        .unwrap_or(0.11);
    let mut ranges_by_center_id = Map::new();

    for center_id in selected_ids {
        let Some(center) = centers.iter().find(|row| row.id == center_id) else {
            continue;
        };
        let ranges =
            build_center_territory_ranges(center, &centers, sample_radius, bucket_count, guard);
        ranges_by_center_id.insert(
            center_id,
            Value::Array(ranges.into_iter().map(range_json).collect()),
        );
    }

    json!({
        "rangesByCenterId": ranges_by_center_id,
    })
}

pub(crate) fn build_center_territory_ranges(
    center: &NetworkOwnerCenter,
    centers: &[NetworkOwnerCenter],
    sample_radius: f64,
    bucket_count: usize,
    guard: f64,
) -> Vec<NetworkAngleRange> {
    let center_rows = centers
        .iter()
        .filter(|row| !row.id.trim().is_empty())
        .cloned()
        .collect::<Vec<_>>();
    if center.id.trim().is_empty() || center_rows.len() <= 1 {
        return vec![NetworkAngleRange::full()];
    }
    let others = center_rows
        .iter()
        .filter(|row| row.id.trim() != center.id.trim())
        .collect::<Vec<_>>();
    if others.is_empty() {
        return vec![NetworkAngleRange::full()];
    }

    let mut min_dist = f64::INFINITY;
    for row in others {
        let dx = row.x - center.x;
        let dy = row.y - center.y;
        let dist = (dx * dx + dy * dy).sqrt();
        if dist.is_finite() && dist > 1e-3 {
            min_dist = min_dist.min(dist);
        }
    }

    let fallback_radius = sample_radius.max(200.0);
    let sampling_radius = if min_dist.is_finite() {
        min_dist.mul_add(0.92, 0.0).min(fallback_radius).max(180.0)
    } else {
        fallback_radius
    };
    let bucket_count = bucket_count.max(48);
    let mut marks = vec![false; bucket_count];
    for (index, mark) in marks.iter_mut().enumerate() {
        let angle = -PI + (((index as f64) + 0.5) / (bucket_count as f64)) * TWO_PI;
        let px = center.x + angle.cos() * sampling_radius;
        let py = center.y + angle.sin() * sampling_radius;
        if nearest_center_id_at(px, py, &center_rows).as_deref() == Some(center.id.trim()) {
            *mark = true;
        }
    }

    let step = TWO_PI / (bucket_count as f64);
    let mut ranges = Vec::new();
    let mut cursor = 0usize;
    while cursor < bucket_count {
        if !marks[cursor] {
            cursor += 1;
            continue;
        }
        let start_index = cursor;
        while cursor < bucket_count && marks[cursor] {
            cursor += 1;
        }
        let end_index = cursor - 1;
        let start = -PI + (start_index as f64) * step;
        let end = -PI + ((end_index + 1) as f64) * step;
        ranges.push(NetworkAngleRange {
            start,
            end,
            span: end - start,
        });
    }

    if ranges.is_empty() {
        return vec![NetworkAngleRange::full()];
    }
    if marks[0] && marks[bucket_count - 1] && ranges.len() > 1 {
        let first = ranges[0].clone();
        let last = ranges.pop().unwrap_or_else(NetworkAngleRange::full);
        ranges[0] = NetworkAngleRange {
            start: last.start,
            end: first.end,
            span: first.end - last.start + TWO_PI,
        };
    }

    let guard = guard.clamp(0.03, 0.2);
    let guarded = ranges
        .into_iter()
        .filter_map(|row| {
            if row.is_full() {
                return Some(NetworkAngleRange::full());
            }
            if row.span <= guard * 2.0 {
                return None;
            }
            Some(NetworkAngleRange {
                start: row.start + guard,
                end: row.end - guard,
                span: row.span - guard * 2.0,
            })
        })
        .collect::<Vec<_>>();
    if guarded.is_empty() {
        vec![NetworkAngleRange::full()]
    } else {
        guarded
    }
}

pub(crate) fn intersect_angle_ranges(
    ranges: &[NetworkAngleRange],
    angle: f64,
    span: f64,
) -> Vec<NetworkAngleRange> {
    let base = ranges_to_positive_segments(ranges);
    if base.is_empty() {
        return Vec::new();
    }
    let window = arc_to_positive_segments(angle - span / 2.0, span);
    let mut out = Vec::new();
    for a in &base {
        for b in &window {
            let start = a.0.max(b.0);
            let end = a.1.min(b.1);
            if end - start > 1e-5 {
                out.push((start, end));
            }
        }
    }
    merge_segments(out)
        .into_iter()
        .filter_map(|(start, end)| {
            let end = end.min(TWO_PI - 1e-6);
            let span = end - start;
            if !span.is_finite() || span <= 1e-4 {
                return None;
            }
            Some(NetworkAngleRange { start, end, span })
        })
        .collect()
}

pub(crate) fn constrain_sector_to_territory(
    sector_start: f64,
    sector_end: f64,
    territory_ranges: &[NetworkAngleRange],
) -> Option<(f64, f64)> {
    if territory_ranges.is_empty() || territory_ranges.iter().any(NetworkAngleRange::is_full) {
        return Some((sector_start, sector_end));
    }
    let span = sector_end - sector_start;
    if !span.is_finite() || span <= 1e-5 {
        return None;
    }
    let intersections = intersect_angle_ranges(territory_ranges, sector_start + span / 2.0, span);
    intersections
        .into_iter()
        .max_by(|a, b| a.span.partial_cmp(&b.span).unwrap_or(Ordering::Equal))
        .map(|row| (row.start, row.end))
}

pub(crate) fn territory_sample_radius(r_leaf_min: f64, leaf_count: usize) -> f64 {
    let r_leaf_min = r_leaf_min.max(80.0);
    (r_leaf_min * 1.8).max(r_leaf_min + (leaf_count.max(1) as f64).sqrt() * 48.0)
}

fn nearest_center_id_at(x: f64, y: f64, centers: &[NetworkOwnerCenter]) -> Option<String> {
    let mut nearest_id = None;
    let mut nearest_dist_sq = f64::INFINITY;
    for row in centers {
        let id = row.id.trim();
        if id.is_empty() || !row.x.is_finite() || !row.y.is_finite() {
            continue;
        }
        let dx = row.x - x;
        let dy = row.y - y;
        let dist_sq = dx * dx + dy * dy;
        if !dist_sq.is_finite() {
            continue;
        }
        if dist_sq < nearest_dist_sq {
            nearest_dist_sq = dist_sq;
            nearest_id = Some(id.to_string());
        }
    }
    nearest_id
}

fn ranges_to_positive_segments(ranges: &[NetworkAngleRange]) -> Vec<(f64, f64)> {
    let mut out = Vec::new();
    for row in ranges {
        if row.is_full() {
            return vec![(0.0, TWO_PI)];
        }
        out.extend(arc_to_positive_segments(row.start, row.span));
    }
    merge_segments(out)
}

fn arc_to_positive_segments(start: f64, span: f64) -> Vec<(f64, f64)> {
    let span = span.clamp(0.0, TWO_PI);
    if span >= TWO_PI - 1e-6 {
        return vec![(0.0, TWO_PI)];
    }
    let start = normalize_angle_positive(start);
    let end = normalize_angle_positive(start + span);
    if start <= end {
        vec![(start, end)]
    } else {
        vec![(start, TWO_PI), (0.0, end)]
    }
}

fn merge_segments(mut rows: Vec<(f64, f64)>) -> Vec<(f64, f64)> {
    rows.retain(|row| row.1 > row.0);
    rows.sort_by(|a, b| a.0.partial_cmp(&b.0).unwrap_or(Ordering::Equal));
    let mut out: Vec<(f64, f64)> = Vec::new();
    for row in rows {
        let Some(prev) = out.last_mut() else {
            out.push(row);
            continue;
        };
        if row.0 <= prev.1 + 1e-6 {
            prev.1 = prev.1.max(row.1);
        } else {
            out.push(row);
        }
    }
    out
}

fn normalize_angle_positive(value: f64) -> f64 {
    let mut out = value % TWO_PI;
    if out < 0.0 {
        out += TWO_PI;
    }
    out
}

fn read_owner_centers(value: Option<&Value>) -> Vec<NetworkOwnerCenter> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .filter_map(|row| {
                    let id = text_field(row, "id");
                    if id.is_empty() {
                        return None;
                    }
                    Some(NetworkOwnerCenter {
                        id,
                        x: number_field(row, "x").unwrap_or(0.0),
                        y: number_field(row, "y").unwrap_or(0.0),
                    })
                })
                .collect()
        })
        .unwrap_or_default()
}

fn read_string_vec(value: Option<&Value>) -> Vec<String> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .map(js_or_empty_string)
                .map(|value| value.trim().to_string())
                .filter(|value| !value.is_empty())
                .collect()
        })
        .unwrap_or_default()
}

fn range_json(row: NetworkAngleRange) -> Value {
    json!({
        "start": row.start,
        "end": row.end,
        "span": row.span,
    })
}

fn text_field(value: &Value, key: &str) -> String {
    value
        .get(key)
        .map(js_or_empty_string)
        .unwrap_or_default()
        .trim()
        .to_string()
}

fn js_or_empty_string(value: &Value) -> String {
    match value {
        Value::String(text) => text.to_string(),
        Value::Number(number) => number.to_string(),
        Value::Bool(value) => value.to_string(),
        _ => String::new(),
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

    fn center(id: &str, x: f64, y: f64) -> NetworkOwnerCenter {
        NetworkOwnerCenter {
            id: id.to_string(),
            x,
            y,
        }
    }

    #[test]
    fn network_plan_territory_ranges_split_two_horizontal_centers_with_guard() {
        let centers = vec![center("left", -100.0, 0.0), center("right", 100.0, 0.0)];
        let ranges = build_center_territory_ranges(&centers[0], &centers, 240.0, 192, 0.14);
        let span_total = ranges.iter().map(|row| row.span).sum::<f64>();

        assert_eq!(ranges.len(), 1);
        assert!(span_total > PI);
        assert!(span_total < TWO_PI - 1.0);
        assert!(ranges[0].start > 0.0);
        assert!(ranges[0].end < 0.0);
        assert!(ranges[0].start > ranges[0].end);
    }

    #[test]
    fn network_plan_territory_ranges_merge_wraparound_buckets() {
        let centers = vec![
            center("left", -100.0, 0.0),
            center("right", 100.0, 0.0),
            center("top", 0.0, 120.0),
        ];
        let ranges = build_center_territory_ranges(&centers[0], &centers, 260.0, 216, 0.14);

        assert_eq!(ranges.len(), 1);
        assert!(ranges[0].start > ranges[0].end);
        assert!(ranges[0].span > PI / 2.0);
    }

    #[test]
    fn network_plan_territory_ranges_fallback_to_full_circle() {
        let centers = vec![center("only", 0.0, 0.0)];
        let ranges = build_center_territory_ranges(&centers[0], &centers, 240.0, 192, 0.14);

        assert_eq!(ranges.len(), 1);
        assert!(ranges[0].is_full());
    }

    #[test]
    fn network_plan_territory_constrains_sector_to_largest_overlap() {
        let ranges = vec![NetworkAngleRange {
            start: PI / 2.0,
            end: PI,
            span: PI / 2.0,
        }];
        let constrained = constrain_sector_to_territory(0.0, PI, &ranges).expect("sector");

        assert!(constrained.0 >= PI / 2.0 - 1e-6);
        assert!(constrained.1 <= PI + 1e-6);
    }
}
