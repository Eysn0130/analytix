use crate::flow_layout_network_model::NetworkOwnerCenter;
use crate::flow_layout_network_node_metrics::{project_node_metrics, NetworkNodeMetrics};
use crate::flow_layout_network_ownership::NetworkCenterOwnershipChecker;
use crate::flow_layout_network_sector::TWO_PI;
use crate::flow_layout_network_sector_ring::clamp_f64;
use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Value};
use std::{fs, path::PathBuf};

pub(crate) struct NetworkCandidateScoreContractArgs {
    pub(crate) payload: Value,
}

pub(crate) struct Zone {
    pub(crate) x: f64,
    pub(crate) y: f64,
    pub(crate) r: f64,
    pub(crate) owner_center_id: String,
}

pub(crate) struct HardCorridor {
    pub(crate) x1: f64,
    pub(crate) y1: f64,
    pub(crate) x2: f64,
    pub(crate) y2: f64,
    pub(crate) half_width: f64,
}

pub(crate) struct AngleRange {
    pub(crate) start: f64,
    pub(crate) end: f64,
    pub(crate) span: Option<f64>,
}

struct CandidatePoint {
    x: f64,
    y: f64,
    angle: Option<f64>,
    defer_territory_overflow: bool,
}

#[derive(Clone, Copy, Debug)]
pub(crate) struct NetworkCandidateBest {
    pub(crate) x: f64,
    pub(crate) y: f64,
    pub(crate) angle: f64,
}

#[derive(Clone, Debug)]
pub(crate) struct NetworkCandidateSelection {
    pub(crate) best: Option<NetworkCandidateBest>,
    pub(crate) best_penalty: Option<f64>,
    pub(crate) best_index: i64,
    pub(crate) checked_count: usize,
    pub(crate) avoidance_hits: usize,
    pub(crate) hard_reject_hits: usize,
    pub(crate) corridor_reject_hits: usize,
    pub(crate) territory_overflow_hits: usize,
    pub(crate) accepted_reason: String,
    pub(crate) candidate_count: Option<usize>,
}

impl NetworkCandidateSelection {
    pub(crate) fn to_json(&self) -> Value {
        let mut out = json!({
            "found": self.best.is_some(),
            "best": self.best.map(|best| {
                json!({
                    "x": best.x,
                    "y": best.y,
                    "angle": best.angle,
                })
            }).unwrap_or(Value::Null),
            "bestPenalty": self.best_penalty.map(Value::from).unwrap_or(Value::Null),
            "bestIndex": self.best_index,
            "checkedCount": self.checked_count,
            "avoidanceHits": self.avoidance_hits,
            "hardRejectHits": self.hard_reject_hits,
            "corridorRejectHits": self.corridor_reject_hits,
            "territoryOverflowHits": self.territory_overflow_hits,
            "acceptedReason": self.accepted_reason,
        });
        if let (Some(map), Some(candidate_count)) = (out.as_object_mut(), self.candidate_count) {
            map.insert("candidateCount".to_string(), json!(candidate_count));
        }
        out
    }
}

pub(crate) struct NetworkCandidateAttemptSelectionInput<'a> {
    pub(crate) center_id: &'a str,
    pub(crate) center_x: f64,
    pub(crate) center_y: f64,
    pub(crate) metrics: NetworkNodeMetrics,
    pub(crate) occupied_zones: &'a [Zone],
    pub(crate) hard_zones: &'a [Zone],
    pub(crate) hard_corridors: &'a [HardCorridor],
    pub(crate) territory_ranges: &'a [AngleRange],
    pub(crate) owner_centers: &'a [NetworkOwnerCenter],
    pub(crate) strict_center_ownership: bool,
    pub(crate) allow_territory_overflow: bool,
    pub(crate) sector_start: f64,
    pub(crate) sector_span: f64,
    pub(crate) keep_in_sector: bool,
    pub(crate) base_angle: f64,
    pub(crate) target_radius: f64,
    pub(crate) min_radius: f64,
    pub(crate) max_leaf_radius: f64,
    pub(crate) layer_gap: f64,
    pub(crate) attempt_max: usize,
    pub(crate) shift_step: f64,
    pub(crate) angle_jitter_base: f64,
    pub(crate) attempt_phase_seed: f64,
    pub(crate) dense_sector: bool,
    pub(crate) hard_padding: f64,
    pub(crate) hard_corridor_padding: f64,
    pub(crate) overlap_padding: f64,
    pub(crate) overlap_weight: f64,
    pub(crate) ray_angle_threshold: f64,
    pub(crate) ray_radial_threshold: f64,
    pub(crate) early_penalty: f64,
    pub(crate) clean_ray_threshold: f64,
}

struct NetworkCandidateSelectionInput<'a> {
    center_id: &'a str,
    center_x: f64,
    center_y: f64,
    metrics: &'a NetworkNodeMetrics,
    candidates: &'a [CandidatePoint],
    occupied_zones: &'a [Zone],
    hard_zones: &'a [Zone],
    hard_corridors: &'a [HardCorridor],
    territory_ranges: &'a [AngleRange],
    owner_centers: &'a [NetworkOwnerCenter],
    strict_center_ownership: bool,
    allow_territory_overflow: bool,
    hard_padding: f64,
    hard_corridor_padding: f64,
    overlap_padding: f64,
    overlap_weight: f64,
    ray_angle_threshold: f64,
    ray_radial_threshold: f64,
    early_penalty: f64,
    clean_ray_threshold: f64,
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<NetworkCandidateScoreContractArgs> {
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
                        "read network candidate score contract input {}",
                        input_path.display()
                    )
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!(
                        "parse network candidate score contract input {}",
                        input_path.display()
                    )
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute contract-flow-layout-network-candidate-score <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported network candidate score contract flag: {other}"),
        }
    }

    Ok(NetworkCandidateScoreContractArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_network_candidate_score_contract(payload: &Value) -> Value {
    let center = payload.get("center").unwrap_or(payload);
    let target = payload.get("target").unwrap_or(payload);
    let node = payload.get("node").unwrap_or(&Value::Null);
    let center_x = js_or_default(number_field(center, "x"), 0.0);
    let center_y = js_or_default(number_field(center, "y"), 0.0);
    let center_id = text_field(payload, "ownerCenterId")
        .or_else(|| text_field(center, "id"))
        .unwrap_or_default();
    let x = js_or_default(number_field(target, "x"), 0.0);
    let y = js_or_default(number_field(target, "y"), 0.0);
    let metrics = project_node_metrics(Some(node));
    let zones = read_zones(
        payload
            .get("occupiedZones")
            .or_else(|| payload.get("zones")),
    );
    let hard_zones = read_zones(payload.get("hardZones"));
    let hard_corridors = read_hard_corridors(payload.get("hardCorridors"));
    let territory_ranges = read_angle_ranges(payload.get("territoryRanges"));
    let owner_centers = read_owner_centers(payload.get("ownerCenters"));
    let strict_center_ownership = !matches!(
        payload.get("strictCenterOwnership"),
        Some(Value::Bool(false))
    );
    let ownership =
        NetworkCenterOwnershipChecker::new(&center_id, &owner_centers, strict_center_ownership);
    let owned_by_center = ownership.as_ref().is_some_and(|checker| checker.owns(x, y));
    let angle = (y - center_y).atan2(x - center_x);
    let has_territory_constraint = !territory_ranges.is_empty()
        && !territory_ranges
            .iter()
            .any(|row| row.span.unwrap_or(0.0) >= TWO_PI - 1e-3);
    let in_territory = !has_territory_constraint || is_angle_in_ranges(angle, &territory_ranges);
    let allow_territory_overflow = truthy_field(payload, "allowTerritoryOverflow");
    let hard_padding = js_or_default(number_field(payload, "hardPadding"), 12.0).max(0.0);
    let corridor_padding =
        js_or_default(number_field(payload, "hardCorridorPadding"), hard_padding).max(0.0);
    let overlap_padding = js_or_default(number_field(payload, "overlapPadding"), 8.0).max(0.0);
    let overlap_weight = js_or_default(number_field(payload, "overlapWeight"), 7.2).max(0.0);
    let ray_angle_threshold = number_field(payload, "rayAngleThreshold").unwrap_or(0.08);
    let ray_radial_threshold = number_field(payload, "rayRadialThreshold").unwrap_or(34.0);
    let ray_max_penalty = match number_field(payload, "rayMaxPenalty") {
        Some(value) if value.is_finite() => value.max(0.0),
        _ => f64::INFINITY,
    };
    let hard_collision =
        collides_with_zones(x, y, metrics.collision_radius, &hard_zones, hard_padding);
    let hard_corridor_collision = collides_with_corridors(
        x,
        y,
        metrics.radius,
        metrics.label,
        &hard_corridors,
        corridor_padding,
    );
    let overlap_penalty = overlap_penalty(
        x,
        y,
        metrics.collision_radius,
        &zones,
        overlap_padding,
        f64::INFINITY,
    );
    let overlap_weighted = overlap_penalty * overlap_weight;
    let ray_penalty = ray_cluster_penalty(
        center_x,
        center_y,
        x,
        y,
        &zones,
        &center_id,
        ray_angle_threshold,
        ray_radial_threshold,
        ray_max_penalty,
    );
    let candidate_penalty = overlap_weighted + ray_penalty;
    let rejected = !owned_by_center
        || (!in_territory && has_territory_constraint && !allow_territory_overflow)
        || hard_collision
        || hard_corridor_collision;

    json!({
        "ownedByCenter": owned_by_center,
        "hasTerritoryConstraint": has_territory_constraint,
        "inTerritory": in_territory,
        "hardCollision": hard_collision,
        "hardCorridorCollision": hard_corridor_collision,
        "overlapPenalty": overlap_penalty,
        "overlapWeighted": overlap_weighted,
        "rayPenalty": ray_penalty,
        "candidatePenalty": candidate_penalty,
        "rejected": rejected,
    })
}

pub(crate) fn project_network_candidate_selection_contract(payload: &Value) -> Value {
    let center = payload.get("center").unwrap_or(payload);
    let node = payload.get("node").unwrap_or(&Value::Null);
    let center_x = js_or_default(number_field(center, "x"), 0.0);
    let center_y = js_or_default(number_field(center, "y"), 0.0);
    let center_id = text_field(payload, "ownerCenterId")
        .or_else(|| text_field(center, "id"))
        .unwrap_or_default();
    let metrics = project_node_metrics(Some(node));
    let candidates = read_candidate_points(payload.get("candidates"));
    let zones = read_zones(
        payload
            .get("occupiedZones")
            .or_else(|| payload.get("zones")),
    );
    let hard_zones = read_zones(payload.get("hardZones"));
    let hard_corridors = read_hard_corridors(payload.get("hardCorridors"));
    let territory_ranges = read_angle_ranges(payload.get("territoryRanges"));
    let owner_centers = read_owner_centers(payload.get("ownerCenters"));
    let strict_center_ownership = !matches!(
        payload.get("strictCenterOwnership"),
        Some(Value::Bool(false))
    );
    let allow_territory_overflow = truthy_field(payload, "allowTerritoryOverflow");
    let hard_padding = js_or_default(number_field(payload, "hardPadding"), 12.0).max(0.0);
    let corridor_padding =
        js_or_default(number_field(payload, "hardCorridorPadding"), hard_padding).max(0.0);
    let overlap_padding = js_or_default(number_field(payload, "overlapPadding"), 8.0).max(0.0);
    let overlap_weight = js_or_default(number_field(payload, "overlapWeight"), 7.2).max(0.0);
    let early_penalty = js_or_default(number_field(payload, "earlyPenalty"), 0.08).max(0.0);
    let clean_ray_threshold =
        js_or_default(number_field(payload, "cleanRayThreshold"), 0.35).max(0.0);
    let ray_angle_threshold = number_field(payload, "rayAngleThreshold").unwrap_or(0.08);
    let ray_radial_threshold = number_field(payload, "rayRadialThreshold").unwrap_or(34.0);

    project_network_candidate_selection_from_input(NetworkCandidateSelectionInput {
        center_id: &center_id,
        center_x,
        center_y,
        metrics: &metrics,
        candidates: &candidates,
        occupied_zones: &zones,
        hard_zones: &hard_zones,
        hard_corridors: &hard_corridors,
        territory_ranges: &territory_ranges,
        owner_centers: &owner_centers,
        strict_center_ownership,
        allow_territory_overflow,
        hard_padding,
        hard_corridor_padding: corridor_padding,
        overlap_padding,
        overlap_weight,
        ray_angle_threshold,
        ray_radial_threshold,
        early_penalty,
        clean_ray_threshold,
    })
    .to_json()
}

pub(crate) fn project_network_candidate_attempt_selection(
    input: NetworkCandidateAttemptSelectionInput<'_>,
) -> Value {
    project_network_candidate_attempt_selection_model(input).to_json()
}

pub(crate) fn project_network_candidate_attempt_selection_model(
    input: NetworkCandidateAttemptSelectionInput<'_>,
) -> NetworkCandidateSelection {
    let candidates = project_candidate_attempt_rows_from_values(
        input.center_x,
        input.center_y,
        input.sector_start,
        input.sector_span,
        input.keep_in_sector,
        input.base_angle,
        input.target_radius,
        input.min_radius,
        input.max_leaf_radius,
        input.layer_gap,
        input.attempt_max,
        input.shift_step,
        input.angle_jitter_base,
        input.attempt_phase_seed,
        input.dense_sector,
        input.territory_ranges,
        input.allow_territory_overflow,
    );
    let mut selection =
        project_network_candidate_selection_from_input(NetworkCandidateSelectionInput {
            center_id: input.center_id,
            center_x: input.center_x,
            center_y: input.center_y,
            metrics: &input.metrics,
            candidates: &candidates,
            occupied_zones: input.occupied_zones,
            hard_zones: input.hard_zones,
            hard_corridors: input.hard_corridors,
            territory_ranges: input.territory_ranges,
            owner_centers: input.owner_centers,
            strict_center_ownership: input.strict_center_ownership,
            allow_territory_overflow: input.allow_territory_overflow,
            hard_padding: input.hard_padding,
            hard_corridor_padding: input.hard_corridor_padding,
            overlap_padding: input.overlap_padding,
            overlap_weight: input.overlap_weight,
            ray_angle_threshold: input.ray_angle_threshold,
            ray_radial_threshold: input.ray_radial_threshold,
            early_penalty: input.early_penalty,
            clean_ray_threshold: input.clean_ray_threshold,
        });
    selection.candidate_count = Some(candidates.len());
    selection
}

fn project_network_candidate_selection_from_input(
    input: NetworkCandidateSelectionInput<'_>,
) -> NetworkCandidateSelection {
    let ownership = NetworkCenterOwnershipChecker::new(
        input.center_id,
        input.owner_centers,
        input.strict_center_ownership,
    );
    let has_territory_constraint = !input.territory_ranges.is_empty();
    let mut best: Option<(usize, f64, f64, f64)> = None;
    let mut best_penalty = f64::INFINITY;
    let mut avoidance_hits = 0usize;
    let mut hard_reject_hits = 0usize;
    let mut corridor_reject_hits = 0usize;
    let mut territory_overflow_hits = 0usize;
    let mut checked_count = 0usize;
    let mut accepted_reason = String::new();

    for (index, candidate) in input.candidates.iter().enumerate() {
        let angle = candidate
            .angle
            .unwrap_or_else(|| (candidate.y - input.center_y).atan2(candidate.x - input.center_x));
        let in_territory =
            !has_territory_constraint || is_angle_in_ranges(angle, input.territory_ranges);
        if !in_territory && has_territory_constraint && !input.allow_territory_overflow {
            continue;
        }
        if !in_territory
            && has_territory_constraint
            && input.allow_territory_overflow
            && candidate.defer_territory_overflow
        {
            continue;
        }
        if !ownership
            .as_ref()
            .is_some_and(|checker| checker.owns(candidate.x, candidate.y))
        {
            continue;
        }
        if collides_with_zones(
            candidate.x,
            candidate.y,
            input.metrics.collision_radius,
            input.hard_zones,
            input.hard_padding,
        ) {
            hard_reject_hits += 1;
            continue;
        }
        if collides_with_corridors(
            candidate.x,
            candidate.y,
            input.metrics.radius,
            input.metrics.label,
            input.hard_corridors,
            input.hard_corridor_padding,
        ) {
            corridor_reject_hits += 1;
            continue;
        }
        checked_count += 1;
        let overlap_stop_at = if best_penalty.is_finite() {
            (best_penalty / input.overlap_weight).max(0.0)
        } else {
            f64::INFINITY
        };
        let overlap_penalty = overlap_penalty(
            candidate.x,
            candidate.y,
            input.metrics.collision_radius,
            input.occupied_zones,
            input.overlap_padding,
            overlap_stop_at,
        );
        let overlap_weighted = overlap_penalty * input.overlap_weight;
        if best_penalty.is_finite() && overlap_weighted >= best_penalty {
            avoidance_hits += 1;
            continue;
        }
        let ray_stop_at = if best_penalty.is_finite() {
            (best_penalty - overlap_weighted).max(0.0)
        } else {
            f64::INFINITY
        };
        let ray_penalty = ray_cluster_penalty(
            input.center_x,
            input.center_y,
            candidate.x,
            candidate.y,
            input.occupied_zones,
            input.center_id,
            input.ray_angle_threshold,
            input.ray_radial_threshold,
            ray_stop_at,
        );
        let candidate_penalty = overlap_weighted + ray_penalty;
        if candidate_penalty <= input.early_penalty {
            best = Some((index, candidate.x, candidate.y, angle));
            best_penalty = candidate_penalty;
            accepted_reason = "early".to_string();
            if !in_territory && has_territory_constraint {
                territory_overflow_hits += 1;
            }
            break;
        }
        avoidance_hits += 1;
        if candidate_penalty < best_penalty {
            best = Some((index, candidate.x, candidate.y, angle));
            best_penalty = candidate_penalty;
        }
        if overlap_penalty <= 1e-6 && ray_penalty <= input.clean_ray_threshold {
            best = Some((index, candidate.x, candidate.y, angle));
            best_penalty = candidate_penalty;
            accepted_reason = "clean-ray".to_string();
            if !in_territory && has_territory_constraint {
                territory_overflow_hits += 1;
            }
            break;
        }
    }

    let (best_index, best) = best
        .map(|(index, x, y, angle)| (index as i64, Some(NetworkCandidateBest { x, y, angle })))
        .unwrap_or((-1, None));

    NetworkCandidateSelection {
        best,
        best_penalty: best_penalty.is_finite().then_some(best_penalty),
        best_index,
        checked_count,
        avoidance_hits,
        hard_reject_hits,
        corridor_reject_hits,
        territory_overflow_hits,
        accepted_reason,
        candidate_count: None,
    }
}

pub(crate) fn project_network_candidate_attempt_selection_contract(payload: &Value) -> Value {
    let center = payload.get("center").unwrap_or(payload);
    let node = payload.get("node").unwrap_or(&Value::Null);
    let center_x = js_or_default(number_field(center, "x"), 0.0);
    let center_y = js_or_default(number_field(center, "y"), 0.0);
    let center_id = text_field(payload, "ownerCenterId")
        .or_else(|| text_field(center, "id"))
        .unwrap_or_default();
    let metrics = project_node_metrics(Some(node));
    let zones = read_zones(
        payload
            .get("occupiedZones")
            .or_else(|| payload.get("zones")),
    );
    let hard_zones = read_zones(payload.get("hardZones"));
    let hard_corridors = read_hard_corridors(payload.get("hardCorridors"));
    let territory_ranges = read_angle_ranges(payload.get("territoryRanges"));
    let owner_centers = read_owner_centers(payload.get("ownerCenters"));
    let strict_center_ownership = !matches!(
        payload.get("strictCenterOwnership"),
        Some(Value::Bool(false))
    );
    let max_leaf_radius = match number_field(payload, "maxLeafRadius") {
        Some(value) if value.is_finite() => value,
        _ => f64::INFINITY,
    };

    project_network_candidate_attempt_selection(NetworkCandidateAttemptSelectionInput {
        center_id: &center_id,
        center_x,
        center_y,
        metrics,
        occupied_zones: &zones,
        hard_zones: &hard_zones,
        hard_corridors: &hard_corridors,
        territory_ranges: &territory_ranges,
        owner_centers: &owner_centers,
        strict_center_ownership,
        allow_territory_overflow: truthy_field(payload, "allowTerritoryOverflow"),
        sector_start: js_or_default(number_field(payload, "sectorStart"), 0.0),
        sector_span: js_or_default(number_field(payload, "sectorSpan"), 0.0).max(0.0),
        keep_in_sector: truthy_field(payload, "keepInSector"),
        base_angle: js_or_default(number_field(payload, "baseAngle"), 0.0),
        target_radius: js_or_default(number_field(payload, "targetRadius"), 0.0),
        min_radius: js_or_default(number_field(payload, "minRadius"), 0.0).max(0.0),
        max_leaf_radius,
        layer_gap: js_or_default(number_field(payload, "layerGap"), 0.0).max(0.0),
        attempt_max: js_or_default(number_field(payload, "attemptMax"), 0.0)
            .max(0.0)
            .floor() as usize,
        shift_step: js_or_default(number_field(payload, "shiftStep"), 0.0).max(0.0),
        angle_jitter_base: js_or_default(number_field(payload, "angleJitterBase"), 0.0).max(0.0),
        attempt_phase_seed: js_or_default(number_field(payload, "attemptPhaseSeed"), 0.0),
        dense_sector: truthy_field(payload, "denseSector"),
        hard_padding: js_or_default(number_field(payload, "hardPadding"), 12.0).max(0.0),
        hard_corridor_padding: js_or_default(
            number_field(payload, "hardCorridorPadding"),
            js_or_default(number_field(payload, "hardPadding"), 12.0).max(0.0),
        )
        .max(0.0),
        overlap_padding: js_or_default(number_field(payload, "overlapPadding"), 8.0).max(0.0),
        overlap_weight: js_or_default(number_field(payload, "overlapWeight"), 7.2).max(0.0),
        ray_angle_threshold: number_field(payload, "rayAngleThreshold").unwrap_or(0.08),
        ray_radial_threshold: number_field(payload, "rayRadialThreshold").unwrap_or(34.0),
        early_penalty: js_or_default(number_field(payload, "earlyPenalty"), 0.08).max(0.0),
        clean_ray_threshold: js_or_default(number_field(payload, "cleanRayThreshold"), 0.35)
            .max(0.0),
    })
}

#[allow(clippy::too_many_arguments)]
fn project_candidate_attempt_rows_from_values(
    center_x: f64,
    center_y: f64,
    sector_start: f64,
    sector_span: f64,
    keep_in_sector: bool,
    base_angle: f64,
    target_radius: f64,
    min_radius: f64,
    max_leaf_radius: f64,
    layer_gap: f64,
    attempt_max: usize,
    shift_step: f64,
    angle_jitter_base: f64,
    attempt_phase_seed: f64,
    dense_sector: bool,
    territory_ranges: &[AngleRange],
    allow_territory_overflow: bool,
) -> Vec<CandidatePoint> {
    let has_territory_constraint = !territory_ranges.is_empty();
    let ring_step = (layer_gap * 0.2).max(8.0);
    let radius_low = min_radius * 0.78;
    let mut candidates = Vec::new();

    for attempt in 0..=attempt_max {
        let sign = if attempt % 2 == 0 { 1.0 } else { -1.0 };
        let shift = if attempt == 0 {
            0.0
        } else {
            sign * shift_step * ((attempt as f64) / 2.0).ceil()
        };
        let attempt_phase = (attempt_phase_seed + (attempt as f64) * 0.6180339887498949) % 1.0;
        let micro_jitter =
            (attempt_phase - 0.5) * 2.0 * angle_jitter_base * if dense_sector { 1.2 } else { 1.0 };
        let raw_angle = base_angle + shift + micro_jitter;
        let angle = if keep_in_sector {
            clamp_angle_to_arc(raw_angle, sector_start, sector_span)
        } else {
            raw_angle
        };
        let in_territory = !has_territory_constraint || is_angle_in_ranges(angle, territory_ranges);
        if !in_territory && has_territory_constraint && !allow_territory_overflow {
            continue;
        }
        if !in_territory && has_territory_constraint && allow_territory_overflow && attempt < 4 {
            continue;
        }
        let rr_raw = target_radius
            + ((attempt as f64) / 5.0).floor() * ring_step
            + (attempt_phase - 0.5) * ring_step * 0.6;
        let rr = clamp_f64(rr_raw, radius_low, max_leaf_radius);
        candidates.push(CandidatePoint {
            x: center_x + angle.cos() * rr,
            y: center_y + angle.sin() * rr,
            angle: Some(angle),
            defer_territory_overflow: false,
        });
    }

    candidates
}

pub(crate) fn overlap_penalty(
    x: f64,
    y: f64,
    radius: f64,
    zones: &[Zone],
    padding: f64,
    max_penalty: f64,
) -> f64 {
    let mut penalty = 0.0;
    for zone in zones {
        let dx = zone.x - x;
        let dy = zone.y - y;
        let threshold = radius + zone.r + padding;
        let dist_sq = dx * dx + dy * dy;
        if dist_sq < threshold * threshold {
            penalty += threshold - dist_sq.max(0.0).sqrt();
            if penalty > max_penalty {
                return penalty;
            }
        }
    }
    penalty
}

pub(crate) fn collides_with_zones(
    x: f64,
    y: f64,
    radius: f64,
    zones: &[Zone],
    padding: f64,
) -> bool {
    zones.iter().any(|zone| {
        let dx = zone.x - x;
        let dy = zone.y - y;
        let threshold = radius + zone.r + padding;
        dx * dx + dy * dy < threshold * threshold
    })
}

pub(crate) fn collides_with_corridors(
    x: f64,
    y: f64,
    node_radius: f64,
    label_pad: f64,
    corridors: &[HardCorridor],
    padding: f64,
) -> bool {
    corridors.iter().any(|corridor| {
        let dist =
            distance_point_to_segment(x, y, corridor.x1, corridor.y1, corridor.x2, corridor.y2);
        let radius = node_radius.max(0.0) + (label_pad * 0.34).max(8.0) + padding;
        let threshold = corridor.half_width.max(6.0) + radius;
        dist < threshold
    })
}

pub(crate) fn ray_cluster_penalty(
    center_x: f64,
    center_y: f64,
    x: f64,
    y: f64,
    zones: &[Zone],
    owner_center_id: &str,
    angle_threshold: f64,
    radial_threshold: f64,
    max_penalty: f64,
) -> f64 {
    let angle_threshold = clamp_f64(js_or_default(Some(angle_threshold), 0.08), 0.02, 0.24);
    let radial_threshold = js_or_default(Some(radial_threshold), 34.0).max(12.0);
    let target_angle = (y - center_y).atan2(x - center_x);
    let target_radius = hypot(x - center_x, y - center_y);
    let mut penalty = 0.0;
    for zone in zones {
        if !owner_center_id.is_empty()
            && !zone.owner_center_id.is_empty()
            && zone.owner_center_id != owner_center_id
        {
            continue;
        }
        let angle = (zone.y - center_y).atan2(zone.x - center_x);
        let radius = hypot(zone.x - center_x, zone.y - center_y);
        let ad = angle_diff(target_angle, angle);
        if ad >= angle_threshold {
            continue;
        }
        let rd = (target_radius - radius).abs();
        let angle_weight = (angle_threshold - ad) / angle_threshold.max(1e-6);
        let radial_weight = if rd < radial_threshold {
            1.85
        } else if rd < radial_threshold * 2.1 {
            1.12
        } else {
            0.55
        };
        penalty += angle_weight * radial_weight * 22.0;
        if penalty > max_penalty {
            return penalty;
        }
    }
    penalty
}

fn is_angle_in_ranges(angle: f64, ranges: &[AngleRange]) -> bool {
    if ranges.is_empty() {
        return true;
    }
    let value = normalize_angle_positive(angle);
    ranges.iter().any(|range| {
        if range.span.unwrap_or(0.0) >= TWO_PI - 1e-3 {
            return true;
        }
        let start = normalize_angle_positive(range.start);
        let end = normalize_angle_positive(range.end);
        if start <= end {
            value >= start && value <= end
        } else {
            value >= start || value <= end
        }
    })
}

fn read_zones(value: Option<&Value>) -> Vec<Zone> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .filter(|row| !row.is_null())
                .map(|row| Zone {
                    x: js_or_default(number_field(row, "x"), 0.0),
                    y: js_or_default(number_field(row, "y"), 0.0),
                    r: js_or_default(number_field(row, "r"), 0.0),
                    owner_center_id: text_field(row, "ownerCenterId").unwrap_or_default(),
                })
                .collect()
        })
        .unwrap_or_default()
}

fn read_hard_corridors(value: Option<&Value>) -> Vec<HardCorridor> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .filter(|row| !row.is_null())
                .map(|row| HardCorridor {
                    x1: js_or_default(number_field(row, "x1"), 0.0),
                    y1: js_or_default(number_field(row, "y1"), 0.0),
                    x2: js_or_default(number_field(row, "x2"), 0.0),
                    y2: js_or_default(number_field(row, "y2"), 0.0),
                    half_width: js_or_default(number_field(row, "halfWidth"), 0.0).max(0.0),
                })
                .collect()
        })
        .unwrap_or_default()
}

fn read_angle_ranges(value: Option<&Value>) -> Vec<AngleRange> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .filter(|row| !row.is_null())
                .map(|row| AngleRange {
                    start: js_or_default(number_field(row, "start"), 0.0),
                    end: js_or_default(number_field(row, "end"), 0.0),
                    span: number_field(row, "span"),
                })
                .collect()
        })
        .unwrap_or_default()
}

fn read_candidate_points(value: Option<&Value>) -> Vec<CandidatePoint> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .filter(|row| !row.is_null())
                .map(|row| CandidatePoint {
                    x: js_or_default(number_field(row, "x"), 0.0),
                    y: js_or_default(number_field(row, "y"), 0.0),
                    angle: number_field(row, "angle"),
                    defer_territory_overflow: truthy_field(row, "deferTerritoryOverflow"),
                })
                .collect()
        })
        .unwrap_or_default()
}

fn read_owner_centers(value: Option<&Value>) -> Vec<NetworkOwnerCenter> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .filter_map(|row| {
                    let id = text_field(row, "id").unwrap_or_default();
                    if id.is_empty() {
                        return None;
                    }
                    Some(NetworkOwnerCenter {
                        id,
                        x: js_or_default(number_field(row, "x"), 0.0),
                        y: js_or_default(number_field(row, "y"), 0.0),
                    })
                })
                .collect()
        })
        .unwrap_or_default()
}

fn distance_point_to_segment(px: f64, py: f64, x1: f64, y1: f64, x2: f64, y2: f64) -> f64 {
    let dx = x2 - x1;
    let dy = y2 - y1;
    let denom = dx * dx + dy * dy;
    if !denom.is_finite() || denom <= 1e-9 {
        return hypot(px - x1, py - y1);
    }
    let t = (((px - x1) * dx + (py - y1) * dy) / denom).clamp(0.0, 1.0);
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

fn clamp_angle_to_arc(angle: f64, start: f64, span: f64) -> f64 {
    let arc_span = clamp_f64(span, 0.0, TWO_PI);
    if arc_span >= TWO_PI - 1e-6 {
        return angle;
    }
    let mut rel = (angle - start) % TWO_PI;
    while rel < 0.0 {
        rel += TWO_PI;
    }
    start + clamp_f64(rel, 0.0, arc_span)
}

fn hypot(dx: f64, dy: f64) -> f64 {
    (dx * dx + dy * dy).sqrt()
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

fn text_field(value: &Value, key: &str) -> Option<String> {
    match value.get(key) {
        Some(Value::String(text)) => Some(text.to_string()),
        Some(Value::Number(number)) => Some(number.to_string()),
        Some(Value::Bool(value)) => Some(value.to_string()),
        _ => None,
    }
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

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn network_candidate_score_combines_overlap_and_ray_penalty() {
        let contract = project_network_candidate_score_contract(&json!({
            "center": {"id": "center", "x": 0, "y": 0},
            "target": {"x": 140, "y": 2},
            "node": {"id": "leaf", "title": "Leaf", "r": 18},
            "occupiedZones": [
                {"x": 110, "y": 0, "r": 46, "ownerCenterId": "center"},
                {"x": -120, "y": 0, "r": 42, "ownerCenterId": "other"}
            ],
            "ownerCenterId": "center",
            "ownerCenters": [{"id": "center", "x": 0, "y": 0}],
            "strictCenterOwnership": true,
            "rayAngleThreshold": 0.09,
            "rayRadialThreshold": 42
        }));

        assert_eq!(contract["ownedByCenter"], json!(true));
        assert_eq!(contract["hardCollision"], json!(false));
        assert_eq!(contract["hardCorridorCollision"], json!(false));
        assert!(contract["overlapPenalty"].as_f64().unwrap_or_default() > 0.0);
        assert!(contract["rayPenalty"].as_f64().unwrap_or_default() > 0.0);
    }

    #[test]
    fn network_candidate_selection_keeps_first_clean_candidate() {
        let contract = project_network_candidate_selection_contract(&json!({
            "center": {"id": "center", "x": 0, "y": 0},
            "node": {"id": "leaf", "title": "Leaf", "r": 18},
            "candidates": [
                {"x": 112, "y": 0, "angle": 0},
                {"x": 240, "y": 40, "angle": 0.16514867741462683}
            ],
            "occupiedZones": [
                {"x": 110, "y": 0, "r": 46, "ownerCenterId": "center"}
            ],
            "hardZones": [{"x": -80, "y": 0, "r": 24}],
            "ownerCenterId": "center",
            "ownerCenters": [{"id": "center", "x": 0, "y": 0}],
            "strictCenterOwnership": true,
            "rayAngleThreshold": 0.09,
            "rayRadialThreshold": 42
        }));

        assert_eq!(contract["found"], json!(true));
        assert_eq!(contract["bestIndex"], json!(1));
        assert_eq!(contract["acceptedReason"], json!("early"));
        assert!(contract["avoidanceHits"].as_u64().unwrap_or_default() >= 1);
    }

    #[test]
    fn network_candidate_attempt_selection_generates_and_selects_candidate() {
        let contract = project_network_candidate_attempt_selection_contract(&json!({
            "center": {"id": "center", "x": 0, "y": 0},
            "node": {"id": "leaf", "title": "Leaf", "r": 18},
            "sectorStart": -0.4,
            "sectorSpan": 0.8,
            "keepInSector": true,
            "baseAngle": 0.02,
            "targetRadius": 150,
            "minRadius": 130,
            "maxLeafRadius": 260,
            "layerGap": 64,
            "attemptMax": 10,
            "shiftStep": 0.08,
            "angleJitterBase": 0.02,
            "attemptPhaseSeed": 0.37,
            "denseSector": false,
            "occupiedZones": [{"x": 148, "y": 0, "r": 48, "ownerCenterId": "center"}],
            "ownerCenterId": "center",
            "ownerCenters": [{"id": "center", "x": 0, "y": 0}],
            "strictCenterOwnership": true,
            "rayAngleThreshold": 0.09,
            "rayRadialThreshold": 42
        }));

        assert_eq!(contract["found"], json!(true));
        assert!(contract["candidateCount"].as_u64().unwrap_or_default() > 0);
        assert!(contract["bestIndex"].as_i64().unwrap_or(-1) >= 0);
    }
}
