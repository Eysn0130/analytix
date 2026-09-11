use crate::flow_layout_network_model::{
    NetworkBucketAngleRange as AngleRange, NetworkBucketProjection, NetworkBucketSectorPlan,
    NetworkBucketSectorProjection, NetworkCircleZone as CircleZone,
    NetworkClusterBox as ClusterBox, NetworkHardCorridor as HardCorridor, NetworkLeafSector,
    NetworkPlacementContext, NetworkPreferredAngle as PreferredAngle,
};
use crate::flow_layout_network_sector::TWO_PI;
use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Value};
use std::f64::consts::PI;
use std::{fs, path::PathBuf};

pub(crate) struct NetworkBucketScoreContractArgs {
    pub(crate) payload: Value,
}

struct BucketScore {
    score: f64,
    blankness: f64,
    local_collision_penalty: f64,
    global_collision_penalty: f64,
    boundary_penalty: f64,
    label_crowding_penalty: f64,
    hard_collision_penalty: f64,
    hard_corridor_penalty: f64,
    territory_penalty: f64,
    direction_bonus: f64,
}

#[derive(Clone)]
struct BucketWindow {
    start: usize,
    window_size: usize,
    avg: f64,
    buckets: Vec<Value>,
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<NetworkBucketScoreContractArgs> {
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
                        "read network bucket score contract input {}",
                        input_path.display()
                    )
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!(
                        "parse network bucket score contract input {}",
                        input_path.display()
                    )
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute contract-flow-layout-network-bucket-score <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported network bucket score contract flag: {other}"),
        }
    }

    Ok(NetworkBucketScoreContractArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_network_bucket_score_contract(payload: &Value) -> Value {
    let buckets = project_network_bucket_rows(payload);
    json!({
        "bucketCount": buckets.len(),
        "buckets": buckets,
    })
}

pub(crate) fn project_network_bucket_window_contract(payload: &Value) -> Value {
    let buckets = project_network_bucket_rows(payload);
    let needed_buckets = js_or_default(number_field(payload, "neededBuckets"), 1.0).max(1.0);
    let best_window = select_best_window_json(&buckets, needed_buckets as usize);

    json!({
        "bucketCount": buckets.len(),
        "neededBuckets": needed_buckets as usize,
        "bestWindow": best_window,
    })
}

pub(crate) fn project_network_bucket_sector_contract(payload: &Value) -> Value {
    project_network_bucket_sector_projection(payload).contract_json()
}

pub(crate) fn project_network_bucket_projection_contract(payload: &Value) -> Value {
    project_network_bucket_projection(payload).contract_json()
}

pub(crate) fn project_network_bucket_projection(payload: &Value) -> NetworkBucketProjection {
    let input = NetworkPlacementContext::from_payload(payload);
    project_network_bucket_projection_from_input(&input)
}

pub(crate) fn project_network_bucket_projection_from_input(
    input: &NetworkPlacementContext,
) -> NetworkBucketProjection {
    let buckets = input.project_bucket_rows();
    let sector_plan = plan_bucket_sectors(&buckets, input);

    NetworkBucketProjection {
        buckets,
        sector_plan,
    }
}

pub(crate) fn project_network_bucket_sector_projection(
    payload: &Value,
) -> NetworkBucketSectorProjection {
    let projection = project_network_bucket_projection(payload);

    NetworkBucketSectorProjection {
        bucket_count: projection.buckets.len(),
        sector_plan: projection.sector_plan,
    }
}

fn project_network_bucket_rows(payload: &Value) -> Vec<Value> {
    NetworkPlacementContext::from_payload(payload).project_bucket_rows()
}

impl NetworkPlacementContext {
    fn project_bucket_rows(&self) -> Vec<Value> {
        let step = TWO_PI / (self.bucket_count as f64);
        let mut buckets = Vec::with_capacity(self.bucket_count);

        for index in 0..self.bucket_count {
            let angle = -PI + step * (index as f64);
            let x = self.center_x + angle.cos() * self.r_leaf_min;
            let y = self.center_y + angle.sin() * self.r_leaf_min;
            let score = score_bucket(
                angle,
                x,
                y,
                &self.local_zones,
                &self.global_leaf_zones,
                &self.cluster_boxes,
                &self.label_zones,
                &self.safe_bounds,
                &self.hard_zones,
                &self.hard_corridors,
                &self.territory_ranges,
                &self.preferred_angles,
                self.allow_territory_overflow,
            );
            buckets.push(json!({
                "index": index,
                "angle": angle,
                "x": x,
                "y": y,
                "score": score.score,
                "blankness": score.blankness,
                "localCollisionPenalty": score.local_collision_penalty,
                "globalCollisionPenalty": score.global_collision_penalty,
                "boundaryPenalty": score.boundary_penalty,
                "labelCrowdingPenalty": score.label_crowding_penalty,
                "hardCollisionPenalty": score.hard_collision_penalty,
                "hardCorridorPenalty": score.hard_corridor_penalty,
                "territoryPenalty": score.territory_penalty,
                "directionBonus": score.direction_bonus,
            }));
        }

        buckets
    }
}

fn select_best_window_json(scored_buckets: &[Value], needed: usize) -> Value {
    select_best_window_model(scored_buckets, needed)
        .map(bucket_window_json)
        .unwrap_or(Value::Null)
}

fn select_best_window_model(scored_buckets: &[Value], needed: usize) -> Option<BucketWindow> {
    let count = scored_buckets.len();
    if count == 0 {
        return None;
    }
    let window_size = needed.max(1).min(count);
    let scores = scored_buckets
        .iter()
        .map(|row| js_or_default(number_field(row, "score"), 0.0))
        .collect::<Vec<_>>();
    let mut sum = scores.iter().take(window_size).sum::<f64>();
    let mut best_start = 0usize;
    let mut best_avg = sum / (window_size as f64);
    for start in 1..count {
        let remove_idx = start - 1;
        let add_idx = (start + window_size - 1) % count;
        sum += scores[add_idx] - scores[remove_idx];
        let avg = sum / (window_size as f64);
        if avg > best_avg {
            best_start = start;
            best_avg = avg;
        }
    }
    let buckets = (0..window_size)
        .map(|index| scored_buckets[(best_start + index) % count].clone())
        .collect::<Vec<_>>();
    Some(BucketWindow {
        start: best_start,
        window_size,
        avg: best_avg,
        buckets,
    })
}

fn bucket_window_json(window: BucketWindow) -> Value {
    json!({
        "start": window.start,
        "windowSize": window.window_size,
        "avg": window.avg,
        "buckets": window.buckets,
    })
}

fn plan_bucket_sectors(
    scored_buckets: &[Value],
    input: &NetworkPlacementContext,
) -> NetworkBucketSectorPlan {
    let config = &input.config;
    let territory_ranges = &input.territory_ranges;
    let allow_territory_overflow = input.allow_territory_overflow;
    let prefer_ring_mode = input.prefer_ring_mode;
    let enforce_single_sector = input.enforce_single_sector;
    let preferred_multi_sector_count = input.preferred_multi_sector_count;
    let multi_sector_gap = input.multi_sector_gap;
    let leaf_label_pads = &input.leaf_label_pads;
    let leaf_count = leaf_label_pads.len();
    let avg_label_pad = leaf_label_pads.iter().sum::<f64>() / (leaf_count.max(1) as f64);
    let dense_sector = leaf_count >= 36;
    let scored_rows = scored_buckets
        .iter()
        .filter(|row| !row.is_null())
        .cloned()
        .collect::<Vec<_>>();
    let effective_scored_rows = if territory_ranges.is_empty() {
        Vec::new()
    } else {
        scored_rows
            .iter()
            .filter(|row| {
                is_angle_in_ranges(
                    js_or_default(number_field(row, "angle"), 0.0),
                    &territory_ranges,
                )
            })
            .cloned()
            .collect::<Vec<_>>()
    };
    let layout_rows = if !effective_scored_rows.is_empty() {
        effective_scored_rows
    } else {
        scored_rows.clone()
    };
    let configured_bucket_count =
        js_or_default(number_field(config, "ANGULAR_BUCKETS"), 36.0).max(12.0) as usize;
    let bucket_count = scored_rows.len().max(configured_bucket_count).max(12);
    let bucket_step = TWO_PI / (bucket_count as f64);
    let arc_label_scale = clamp_layout(0.94 + avg_label_pad / 130.0, 0.92, 1.28);
    let min_arc = (js_or_default(number_field(config, "NETWORK_SECTOR_MIN_ANGLE"), PI / 5.5)
        * arc_label_scale)
        .max(0.4);
    let max_arc_base = min_arc.max(
        js_or_default(number_field(config, "NETWORK_SECTOR_MAX_ANGLE"), PI * 1.55)
            * clamp_layout(0.96 + avg_label_pad / 100.0, 0.94, 1.34),
    );
    let max_arc = if dense_sector {
        max_arc_base.max((PI * 1.68).min(max_arc_base * 1.08))
    } else {
        max_arc_base
    };
    let min_radius_base = js_or_default(number_field(config, "R_LEAF_MIN"), 220.0).max(80.0);
    let layer_gap_for_arc =
        js_or_default(number_field(config, "NETWORK_SECTOR_LAYER_GAP"), 68.0).max(18.0);
    let planned_layers_for_arc = if dense_sector {
        ((leaf_count.max(1) as f64).sqrt() / 1.22).ceil().max(4.0)
    } else {
        ((leaf_count.max(1) as f64).sqrt() / 1.68).ceil().max(2.0)
    };
    let per_layer_for_arc = ((leaf_count as f64) / planned_layers_for_arc.max(1.0))
        .ceil()
        .max(1.0);
    let arc_gap_for_arc = clamp_layout(18.0 + avg_label_pad * 0.34, 22.0, 64.0);
    let arc_radius_ref = (min_radius_base * 1.06)
        .max(min_radius_base + (planned_layers_for_arc - 1.0).max(0.0) * layer_gap_for_arc * 0.52);
    let arc_needed = (per_layer_for_arc * arc_gap_for_arc) / 72.0_f64.max(arc_radius_ref);
    let arc_density_boost = if dense_sector { 1.18 } else { 1.1 };
    let target_arc = clamp_layout(arc_needed * arc_density_boost, min_arc, max_arc);
    let territory_span_total = territory_span_total(territory_ranges);
    let max_territory_buckets = if !territory_ranges.is_empty() && !allow_territory_overflow {
        ((territory_span_total / bucket_step).floor() as usize).max(1)
    } else {
        bucket_count
    };
    let needed_buckets = max_territory_buckets
        .min((target_arc / bucket_step).ceil().max(1.0) as usize)
        .max(1);
    let territory_slack_buckets = max_territory_buckets.saturating_sub(needed_buckets);
    let dense_extra_demand = if dense_sector {
        ((leaf_count.max(1) as f64).sqrt() * 1.05).ceil() as usize
    } else {
        0
    };
    let extra_buckets = territory_slack_buckets.min(dense_extra_demand);
    let expanded_buckets = needed_buckets.max(needed_buckets + extra_buckets);
    let best_window = select_best_window_model(&layout_rows, expanded_buckets);
    let best_buckets = best_window
        .as_ref()
        .filter(|window| !window.buckets.is_empty())
        .map(|window| window.buckets.clone())
        .unwrap_or_else(|| {
            layout_rows
                .iter()
                .take(needed_buckets)
                .cloned()
                .collect::<Vec<_>>()
        });
    let bucket_top_score = best_buckets
        .iter()
        .map(|row| js_or_default(number_field(row, "score"), 0.0))
        .reduce(f64::max)
        .map(round_three);
    let mut sectors = Vec::new();

    if prefer_ring_mode {
        sectors.push(
            NetworkLeafSector::full_circle()
                .with_score(best_window.as_ref().map(|window| window.avg).unwrap_or(0.0)),
        );
    } else {
        let desired_sector_count = if enforce_single_sector {
            1
        } else {
            preferred_multi_sector_count.max(1).min(5)
        };
        let per_sector_arc_cap = if desired_sector_count > 1 {
            bucket_step.max(max_arc.min(
                ((TWO_PI - multi_sector_gap * (desired_sector_count as f64))
                    / (desired_sector_count as f64))
                    * if dense_sector { 1.28 } else { 1.18 },
            ))
        } else {
            max_arc
        };
        let mut window_rows = layout_rows.clone();
        if let Some(ref window) = best_window {
            push_window_as_sector(
                &mut sectors,
                &mut window_rows,
                &window,
                0.6,
                bucket_step,
                target_arc,
                per_sector_arc_cap,
                multi_sector_gap,
            );
            for sector_index in 2..=desired_sector_count {
                let window_factor = if dense_sector {
                    match sector_index {
                        2 => 0.74,
                        3 => 0.62,
                        4 => 0.52,
                        _ => 0.44,
                    }
                } else if sector_index == 2 {
                    0.74
                } else {
                    0.58
                };
                let min_factor = if dense_sector {
                    match sector_index {
                        2 => 0.48,
                        3 => 0.4,
                        4 => 0.34,
                        _ => 0.3,
                    }
                } else if sector_index == 2 {
                    0.44
                } else {
                    0.36
                };
                let window_size =
                    ((expanded_buckets as f64) * window_factor).floor().max(1.0) as usize;
                let Some(next) = select_best_window_model(&window_rows, window_size) else {
                    break;
                };
                if next.buckets.is_empty() {
                    break;
                }
                push_window_as_sector(
                    &mut sectors,
                    &mut window_rows,
                    &next,
                    min_factor,
                    bucket_step,
                    target_arc,
                    per_sector_arc_cap,
                    multi_sector_gap,
                );
            }
        }
        if sectors.is_empty() {
            sectors.push(
                NetworkLeafSector::new(-target_arc / 2.0, target_arc / 2.0)
                    .with_score(best_window.as_ref().map(|window| window.avg).unwrap_or(0.0)),
            );
        }
    }

    let sectors =
        normalize_sectors_with_gap(sectors, multi_sector_gap, (bucket_step * 0.92).max(0.18));
    NetworkBucketSectorPlan {
        sectors,
        bucket_top_score,
        leaf_count,
        avg_label_pad,
        dense_sector,
        bucket_step,
        target_arc,
        min_radius_base,
    }
}

fn push_window_as_sector(
    sectors: &mut Vec<NetworkLeafSector>,
    window_rows: &mut [Value],
    window: &BucketWindow,
    min_factor: f64,
    bucket_step: f64,
    target_arc: f64,
    per_sector_arc_cap: f64,
    multi_sector_gap: f64,
) {
    if window.buckets.is_empty() {
        return;
    }
    let first_angle = number_field(&window.buckets[0], "angle");
    let start_angle = first_angle
        .filter(|value| value.is_finite())
        .map(|value| value - bucket_step * 0.5)
        .unwrap_or(-target_arc / 2.0);
    let window_arc = bucket_step.max((window.buckets.len() as f64) * bucket_step);
    let arc_span = clamp_layout(
        (target_arc * min_factor).max(window_arc),
        bucket_step,
        per_sector_arc_cap,
    );
    sectors
        .push(NetworkLeafSector::new(start_angle, start_angle + arc_span).with_score(window.avg));
    let window_mid = start_angle + arc_span * 0.5;
    let exclusion = multi_sector_gap.max(arc_span * 0.54);
    for row in window_rows {
        let angle = js_or_default(number_field(row, "angle"), 0.0);
        let distance = angle_diff(angle, window_mid);
        if distance < exclusion {
            let current = js_or_default(number_field(row, "score"), 0.0);
            row["score"] = json!(current - (exclusion - distance) * 1300.0 - 2000.0);
        }
    }
}

fn normalize_sectors_with_gap(
    sectors: Vec<NetworkLeafSector>,
    min_gap: f64,
    min_span: f64,
) -> Vec<NetworkLeafSector> {
    let mut list = sectors
        .into_iter()
        .filter_map(|sector| {
            let span = (sector.end - sector.start).max(0.0);
            if !sector.start.is_finite() || !sector.end.is_finite() || span <= 0.0 {
                return None;
            }
            let mid = normalize_angle_positive(sector.start + span * 0.5);
            Some((sector, mid, span))
        })
        .collect::<Vec<_>>();
    if list.len() <= 1 {
        return list.into_iter().map(|row| row.0).collect();
    }
    let gap = clamp_layout(min_gap, 0.0, PI / 2.0);
    let floor_span = clamp_layout(min_span, 0.08, PI);
    list.sort_by(|a, b| a.1.partial_cmp(&b.1).unwrap_or(std::cmp::Ordering::Equal));
    let mids = list.iter().map(|row| row.1).collect::<Vec<_>>();
    let mut spans = list
        .iter()
        .map(|row| row.2.max(floor_span))
        .collect::<Vec<_>>();
    let len = list.len();
    for _ in 0..8 {
        for index in 0..len {
            let next = (index + 1) % len;
            let left_mid = mids[index];
            let right_mid = if next == 0 {
                mids[0] + TWO_PI
            } else {
                mids[next]
            };
            let distance = (right_mid - left_mid).max(1e-6);
            let limit = floor_span.max(distance - gap);
            let pair_half = spans[index] * 0.5 + spans[next] * 0.5;
            if pair_half <= limit + 1e-6 {
                continue;
            }
            let scale = clamp_layout(limit / pair_half, 0.1, 1.0);
            spans[index] = floor_span.max(spans[index] * scale);
            spans[next] = floor_span.max(spans[next] * scale);
        }
    }
    list.into_iter()
        .enumerate()
        .map(|(index, row)| {
            let span = spans[index];
            let start = row.1 - span * 0.5;
            row.0.with_bounds(start, start + span)
        })
        .collect()
}

#[allow(clippy::too_many_arguments)]
fn score_bucket(
    angle: f64,
    x: f64,
    y: f64,
    local_zones: &[CircleZone],
    global_leaf_zones: &[CircleZone],
    cluster_boxes: &[ClusterBox],
    label_zones: &[CircleZone],
    safe_bounds: &Value,
    hard_zones: &[CircleZone],
    hard_corridors: &[HardCorridor],
    territory_ranges: &[AngleRange],
    preferred_angles: &[PreferredAngle],
    allow_territory_overflow: bool,
) -> BucketScore {
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

    let boundary_penalty = score_boundary_penalty(x, y, safe_bounds);
    let hard_collision_penalty = score_hard_zone_penalty(x, y, hard_zones);
    let hard_corridor_penalty = score_hard_corridor_penalty(x, y, hard_corridors);
    let territory_penalty = if is_angle_in_ranges(angle, territory_ranges) {
        0.0
    } else if allow_territory_overflow {
        260.0
    } else {
        1200.0
    };
    let direction_bonus = score_direction_bonus(angle, preferred_angles);
    let score = blankness
        - local_collision_penalty
        - global_collision_penalty
        - boundary_penalty
        - label_crowding_penalty
        - hard_collision_penalty
        - hard_corridor_penalty
        - territory_penalty
        + direction_bonus;

    BucketScore {
        score,
        blankness,
        local_collision_penalty,
        global_collision_penalty,
        boundary_penalty,
        label_crowding_penalty,
        hard_collision_penalty,
        hard_corridor_penalty,
        territory_penalty,
        direction_bonus,
    }
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

fn score_hard_zone_penalty(x: f64, y: f64, hard_zones: &[CircleZone]) -> f64 {
    let mut hard_collision_penalty = 0.0;
    for zone in hard_zones {
        let dist = hypot(zone.x - x, zone.y - y);
        let threshold = zone.r + 22.0;
        if dist < threshold {
            hard_collision_penalty += (threshold - dist) * 2.6 + 38.0;
        }
    }
    hard_collision_penalty
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

fn score_direction_bonus(angle: f64, preferred_angles: &[PreferredAngle]) -> f64 {
    let mut direction_bonus = 0.0;
    for row in preferred_angles {
        let diff = angle_diff(angle, row.angle);
        let gain = (1.0 - diff / row.width).max(0.0);
        direction_bonus += gain * row.weight * 64.0;
    }
    direction_bonus
}

fn is_angle_in_ranges(angle: f64, ranges: &[AngleRange]) -> bool {
    if ranges.is_empty() {
        return true;
    }
    let value = normalize_angle_positive(angle);
    for row in ranges {
        let start = normalize_angle_positive(row.start);
        let end = normalize_angle_positive(row.end);
        if row.span.is_some_and(|span| span >= TWO_PI - 1e-3) {
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

fn distance_point_to_segment(px: f64, py: f64, x1: f64, y1: f64, x2: f64, y2: f64) -> f64 {
    let dx = x2 - x1;
    let dy = y2 - y1;
    let denom = dx * dx + dy * dy;
    if !denom.is_finite() || denom <= 1e-9 {
        return hypot(px - x1, py - y1);
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
    hypot(px - cx, py - cy)
}

fn angle_diff(a: f64, b: f64) -> f64 {
    let x = normalize_angle_positive(a);
    let y = normalize_angle_positive(b);
    let raw = (x - y).abs();
    raw.min(TWO_PI - raw)
}

fn normalize_angle_positive(value: f64) -> f64 {
    if !value.is_finite() {
        return 0.0;
    }
    let mut out = value % TWO_PI;
    if !out.is_finite() {
        return 0.0;
    }
    if out < 0.0 {
        out += TWO_PI;
    }
    out
}

fn hypot(dx: f64, dy: f64) -> f64 {
    (dx * dx + dy * dy).sqrt()
}

fn territory_span_total(ranges: &[AngleRange]) -> f64 {
    ranges
        .iter()
        .map(|row| row.span.unwrap_or(0.0).max(0.0))
        .sum()
}

fn js_or_default(value: Option<f64>, default_value: f64) -> f64 {
    match value {
        Some(value) if value != 0.0 && !value.is_nan() => value,
        _ => default_value,
    }
}

fn clamp_layout(value: f64, min: f64, max: f64) -> f64 {
    value.max(min).min(max)
}

fn round_three(value: f64) -> f64 {
    ((value * 1000.0) + 0.5).floor() / 1000.0
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
    fn network_plan_bucket_score_combines_penalties_and_bonus() {
        let output = project_network_bucket_score_contract(&json!({
            "config": {"ANGULAR_BUCKETS": 12, "R_LEAF_MIN": 100},
            "center": {"x": 0, "y": 0},
            "localMap": {
                "zones": [{"x": 100, "y": 0, "r": 60}],
                "labelZones": [{"x": 100, "y": 0, "r": 20}]
            },
            "globalMap": {
                "leafZones": [{"x": -100, "y": 0, "r": 40}],
                "clusterBBoxes": [{"minX": 95, "minY": -10, "maxX": 105, "maxY": 10, "weight": 2}],
                "safeBounds": {"maxX": 60}
            },
            "hardZones": [{"x": 100, "y": 0, "r": 60}],
            "hardCorridors": [{"x1": 96, "y1": -40, "x2": 96, "y2": 40, "halfWidth": 12}],
            "territoryRanges": [{"start": -0.5, "end": 0.5, "span": 1.0}],
            "preferredAngles": [{"angle": 0, "width": 1.0, "weight": 2.0}],
            "allowTerritoryOverflow": false
        }));
        let buckets = output["buckets"].as_array().expect("buckets");
        let hit_bucket = buckets
            .iter()
            .find(|row| row["index"] == json!(6))
            .expect("right bucket");

        assert_eq!(hit_bucket["territoryPenalty"], json!(0.0));
        assert_eq!(hit_bucket["directionBonus"], json!(128.0));
        assert!((hit_bucket["score"].as_f64().unwrap_or_default() + 334.2).abs() < 1e-9);
    }

    #[test]
    fn network_plan_bucket_score_preserves_js_truthy_overflow_string() {
        let output = project_network_bucket_score_contract(&json!({
            "config": {"ANGULAR_BUCKETS": 12, "R_LEAF_MIN": 100},
            "center": {"x": 0, "y": 0},
            "territoryRanges": [{"start": -0.5, "end": 0.5, "span": 1.0}],
            "allowTerritoryOverflow": "false"
        }));
        let buckets = output["buckets"].as_array().expect("buckets");
        let outside_bucket = buckets
            .iter()
            .find(|row| row["index"] == json!(0))
            .expect("outside bucket");

        assert_eq!(outside_bucket["territoryPenalty"], json!(260.0));
    }

    #[test]
    fn network_plan_bucket_window_selects_best_average() {
        let output = project_network_bucket_window_contract(&json!({
            "config": {"ANGULAR_BUCKETS": 12, "R_LEAF_MIN": 100},
            "center": {"x": 0, "y": 0},
            "preferredAngles": [{"angle": 0, "width": 1.2, "weight": 2}],
            "neededBuckets": 3
        }));
        let best_window = output["bestWindow"].as_object().expect("best window");

        assert_eq!(best_window["start"], json!(5));
        assert_eq!(best_window["windowSize"], json!(3));
        assert!(best_window["avg"].as_f64().unwrap_or_default() > 200.0);
    }

    #[test]
    fn network_plan_bucket_sector_projects_multi_sector_plan() {
        let output = project_network_bucket_sector_contract(&json!({
            "config": {"ANGULAR_BUCKETS": 24, "R_LEAF_MIN": 180},
            "center": {"x": 0, "y": 0},
            "preferredAngles": [
                {"angle": 0, "width": 1.2, "weight": 2},
                {"angle": 3.141592653589793, "width": 1.2, "weight": 1.6}
            ],
            "leafLabelPads": [28, 34, 40, 46, 52, 58, 44, 38, 32, 36, 48, 54],
            "enforceSingleSector": false,
            "preferredMultiSectorCount": 2,
            "multiSectorGap": 0.24
        }));
        let sector_plan = output["sectorPlan"].as_object().expect("sector plan");
        let sectors = sector_plan["sectors"].as_array().expect("sectors");

        assert_eq!(sector_plan["leafCount"], json!(12));
        assert_eq!(sectors.len(), 2);
        assert!(sector_plan["bucketTopScore"].as_f64().unwrap_or_default() > 0.0);
    }

    #[test]
    fn network_plan_bucket_sector_returns_typed_leaf_sectors() {
        let payload = json!({
            "config": {"ANGULAR_BUCKETS": 24, "R_LEAF_MIN": 180},
            "center": {"x": 0, "y": 0},
            "preferredAngles": [
                {"angle": 0, "width": 1.2, "weight": 2},
                {"angle": 3.141592653589793, "width": 1.2, "weight": 1.6}
            ],
            "leafLabelPads": [28, 34, 40, 46, 52, 58],
            "enforceSingleSector": false,
            "preferredMultiSectorCount": 2,
            "multiSectorGap": 0.24
        });
        let projection = project_network_bucket_sector_projection(&payload);
        let plan = &projection.sector_plan;

        assert_eq!(projection.bucket_count, 24);
        assert_eq!(plan.leaf_count, 6);
        assert_eq!(plan.sectors.len(), 2);
        assert!(plan.sectors.iter().all(|sector| sector.score.is_some()));
        assert_eq!(
            projection.contract_json()["sectorPlan"]["sectors"]
                .as_array()
                .expect("sector json")
                .len(),
            plan.sectors.len()
        );
    }

    #[test]
    fn network_plan_bucket_projection_keeps_buckets_and_sector_together() {
        let payload = json!({
            "config": {"ANGULAR_BUCKETS": 24, "R_LEAF_MIN": 180},
            "center": {"x": 12, "y": -8},
            "localMap": {
                "zones": [{"x": 182, "y": -8, "r": 48}],
                "labelZones": [{"x": 12, "y": 172, "r": 28}]
            },
            "globalMap": {
                "leafZones": [{"x": 192, "y": -8, "r": 36}],
                "clusterBBoxes": [{"minX": -20, "minY": -48, "maxX": 54, "maxY": 20, "weight": 2.2}],
                "safeBounds": {"minX": -84, "maxX": 98, "minY": -76, "maxY": 94}
            },
            "leafLabelPads": [28, 34, 40, 46],
            "preferredAngles": [{"angle": 0, "width": 1.2, "weight": 2}],
            "enforceSingleSector": true
        });
        let projection = project_network_bucket_projection(&payload);
        let contract = projection.contract_json();

        assert_eq!(projection.buckets.len(), 24);
        assert_eq!(projection.sector_plan.leaf_count, 4);
        assert_eq!(contract["bucketCount"], json!(24));
        assert_eq!(
            contract["buckets"].as_array().expect("buckets").len(),
            projection.buckets.len()
        );
        assert_eq!(contract["sectorPlan"]["leafCount"], json!(4));
    }

    #[test]
    fn network_plan_bucket_projection_from_input_matches_payload_projection() {
        let payload = json!({
            "config": {"ANGULAR_BUCKETS": 24, "R_LEAF_MIN": 180},
            "center": {"x": 12, "y": -8},
            "localMap": {
                "zones": [{"x": 182, "y": -8, "r": 48}],
                "labelZones": [{"x": 12, "y": 172, "r": 28}]
            },
            "globalMap": {
                "leafZones": [{"x": 192, "y": -8, "r": 36}],
                "clusterBBoxes": [{"minX": -20, "minY": -48, "maxX": 54, "maxY": 20, "weight": 2.2}],
                "safeBounds": {"minX": -84, "maxX": 98, "minY": -76, "maxY": 94}
            },
            "hardZones": [{"x": 150, "y": -8, "r": 28}],
            "hardCorridors": [{"x1": 24, "y1": -100, "x2": 24, "y2": 120, "halfWidth": 10}],
            "territoryRanges": [{"start": -0.8, "end": 0.9, "span": 1.7}],
            "leafLabelPads": [28, 34, 40, 46, 52, 58, 44, 38],
            "preferredAngles": [{"angle": 0, "width": 1.2, "weight": 2}],
            "allowTerritoryOverflow": false,
            "enforceSingleSector": false,
            "preferredMultiSectorCount": 2,
            "multiSectorGap": 0.24
        });
        let input = NetworkPlacementContext::from_payload(&payload);
        let from_payload = project_network_bucket_projection(&payload);
        let from_input = project_network_bucket_projection_from_input(&input);

        assert_eq!(from_input.contract_json(), from_payload.contract_json());
        assert_eq!(from_input.sector_plan.leaf_count, 8);
        assert_eq!(from_input.sector_plan.sectors.len(), 2);
    }

    #[test]
    fn network_plan_static_sector_plan_preserves_existing_sector_contract() {
        let plan = NetworkBucketSectorPlan::from_static_sectors(
            vec![NetworkLeafSector::full_circle()],
            &[18.0, 22.0, 26.0, 30.0],
            &json!({"ANGULAR_BUCKETS": 24, "R_LEAF_MIN": 180}),
        );

        assert_eq!(plan.leaf_count, 4);
        assert_eq!(plan.avg_label_pad, 24.0);
        assert_eq!(plan.bucket_top_score, None);
        assert_eq!(plan.bucket_step, TWO_PI / 24.0);
        assert_eq!(plan.min_radius_base, 180.0);
        assert_eq!(
            plan.contract_json()["sectors"],
            json!([{"start": -3.141592653589793, "end": 3.141592653589793, "score": 0.0}])
        );
    }
}
