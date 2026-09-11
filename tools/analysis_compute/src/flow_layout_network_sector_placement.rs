use crate::flow_layout_network_bucket_score::project_network_bucket_projection_from_input;
use crate::flow_layout_network_candidate_score::{
    collides_with_corridors as candidate_collides_with_corridors,
    collides_with_zones as candidate_collides_with_zones,
    overlap_penalty as candidate_overlap_penalty,
    project_network_candidate_attempt_selection_model,
    ray_cluster_penalty as candidate_ray_cluster_penalty, AngleRange as CandidateAngleRange,
    HardCorridor as CandidateHardCorridor, NetworkCandidateAttemptSelectionInput,
    NetworkCandidateSelection, Zone as CandidateZone,
};
use crate::flow_layout_network_model::{
    NetworkBucketAngleRange, NetworkBucketSectorPlan, NetworkCircleZone, NetworkHardCorridor,
    NetworkLeafSector, NetworkPlacementLeafRow, NetworkSectorPlacementInput,
};
use crate::flow_layout_network_node_metrics::NetworkNodeMetrics;
use crate::flow_layout_network_ownership::NetworkCenterOwnershipChecker;
use crate::flow_layout_network_sector::{
    number_field, project_sector_leaf_plan, round_two, stable_hash_unit, NetworkSectorLeafPlanInput,
};
use crate::flow_layout_network_sector_envelope::{
    apply_sector_envelope, zone_json, AngleRange as EnvelopeAngleRange, Center as EnvelopeCenter,
    EnvelopeOptions, EnvelopeResult, HardCorridor as EnvelopeHardCorridor,
    NodeModel as EnvelopeNodeModel, OwnerCenter as EnvelopeOwnerCenter, Sector as EnvelopeSector,
    Zone as EnvelopeZone,
};
use crate::flow_layout_network_sector_ring::{
    clamp_f64, layer_span_scale_for_count, network_sector_layer_limit, plan_sector_fast_ring_layers,
};
use crate::flow_layout_network_sector_uniformity::{apply_sector_uniformity, UniformityResult};
use serde_json::{json, Value};
use std::collections::HashMap;

fn resolve_network_nominal_arc_gap(
    leaf_count: usize,
    avg_label_pad: f64,
    avg_node_radius: f64,
    avg_collision: f64,
) -> f64 {
    let count = leaf_count as f64;
    let min_gap = if leaf_count >= 160 {
        82.0
    } else if leaf_count >= 80 {
        68.0
    } else {
        36.0
    };
    let max_gap = if leaf_count >= 240 {
        190.0
    } else if leaf_count >= 120 {
        168.0
    } else if leaf_count >= 80 {
        142.0
    } else {
        118.0
    };
    clamp_f64(
        28.0 + avg_label_pad.max(0.0) * 0.62
            + avg_node_radius.max(0.0) * 0.52
            + avg_collision.max(0.0) * 0.42
            + count.max(1.0).sqrt() * 1.15,
        min_gap,
        max_gap,
    )
}

pub(crate) fn project_network_sector_placement(payload: &Value) -> Value {
    let input = NetworkSectorPlacementInput::from_payload(payload);
    let bucket_projection = project_network_bucket_projection_from_input(&input.bucket_context);
    let leaf_ids = input
        .leaf_rows
        .iter()
        .map(|row| row.id.clone())
        .collect::<Vec<_>>();
    let center_type = if input.center.center_type.is_empty() {
        "layoutAnchor"
    } else {
        input.center.center_type.as_str()
    };
    let simple_plan = project_sector_leaf_plan(NetworkSectorLeafPlanInput {
        center_id: &input.center.id,
        center_type,
        center_x: input.center.x,
        center_y: input.center.y,
        leaf_ids: &leaf_ids,
        sector_plan: &bucket_projection.sector_plan,
        phase_key: &format!("network-placement-contract|{}", input.center.id),
        jitter_key_prefix: "network-placement-contract-jitter",
        config: &input.config,
    });
    let simple_node_update_count = simple_plan.node_updates.len();
    let simple_center_report = simple_plan.center_report.into_json();
    let bucket_contract = bucket_projection.contract_json();
    let fast_ring_placement = project_fast_ring_placement(&input, &bucket_projection.sector_plan);
    let general_attempt_selection =
        project_general_attempt_selection(&input, &bucket_projection.sector_plan);
    let general_batch_placement =
        project_general_batch_placement(&input, &bucket_projection.sector_plan);
    let general_batch_placements =
        project_general_batch_placements(&input, &bucket_projection.sector_plan);
    let label_pad_total = input.leaf_rows.iter().map(|row| row.label_pad).sum::<f64>();
    let node_radius_total = input
        .leaf_rows
        .iter()
        .map(|row| row.node_radius)
        .sum::<f64>();
    let collision_radius_total = input
        .leaf_rows
        .iter()
        .map(|row| row.collision_radius)
        .sum::<f64>();
    let leaf_count = input.leaf_rows.len();

    json!({
        "contextSummary": {
            "center": {
                "id": input.center.id,
                "type": center_type,
                "x": input.center.x,
                "y": input.center.y,
                "nodeRadius": input.center.node_radius,
                "collisionRadius": input.center.collision_radius,
            },
            "leafCount": leaf_count,
            "avgLabelPad": if leaf_count == 0 {
                0.0
            } else {
                label_pad_total / (leaf_count as f64)
            },
            "avgNodeRadius": if leaf_count == 0 {
                0.0
            } else {
                node_radius_total / (leaf_count as f64)
            },
            "avgCollisionRadius": if leaf_count == 0 {
                0.0
            } else {
                collision_radius_total / (leaf_count as f64)
            },
            "occupiedZoneCount": input.occupied_zones.len(),
            "hardZoneCount": input.hard_zones.len(),
            "hardCorridorCount": input.hard_corridors.len(),
            "territoryRangeCount": input.territory_ranges.len(),
            "preferredAngleCount": input.preferred_angles.len(),
            "ownerCenterCount": input.owner_centers.len(),
            "strictCenterOwnership": input.strict_center_ownership,
            "allowTerritoryOverflow": input.allow_territory_overflow,
            "maxLeafRadius": input.max_leaf_radius,
        },
        "bucketProjection": bucket_contract,
        "simplePlacement": {
            "nodeUpdateCount": simple_node_update_count,
            "centerReport": simple_center_report,
        },
        "fastRingPlacement": fast_ring_placement,
        "generalAttemptSelection": general_attempt_selection,
        "generalBatchPlacement": general_batch_placement,
        "generalBatchPlacements": general_batch_placements,
    })
}

struct GeneralBatchPlan<'a> {
    dense_sector: bool,
    min_radius: f64,
    max_leaf_radius: f64,
    layer_gap: f64,
    radius: f64,
    layer: usize,
    sector_index: usize,
    leaf_start_index: usize,
    sector: NetworkLeafSector,
    span: f64,
    keep_in_sector: bool,
    take: usize,
    items: Vec<&'a NetworkPlacementLeafRow>,
}

struct GeneralAttemptParams {
    base_angle: f64,
    target_radius: f64,
    attempt_max: usize,
    shift_step: f64,
    angle_jitter_base: f64,
    attempt_phase_seed: f64,
    ray_angle_threshold: f64,
    ray_radial_threshold: f64,
}

struct GeneralBest {
    x: f64,
    y: f64,
    angle: f64,
}

struct GeneralNodePlacement {
    selection: NetworkCandidateSelection,
    best: Option<GeneralBest>,
    fallback_placement: bool,
    pushout_placement: bool,
    territory_overflow_hits: usize,
}

#[derive(Clone, Debug)]
struct GeneralPlacementUpdate {
    id: String,
    x: f64,
    y: f64,
    owner_center_id: String,
}

impl GeneralPlacementUpdate {
    fn into_json(self) -> Value {
        json!({
            "id": self.id,
            "x": self.x,
            "y": self.y,
            "ownerCenterId": self.owner_center_id,
        })
    }
}

#[derive(Clone, Debug)]
struct GeneralPlacementLeafZone {
    id: String,
    x: f64,
    y: f64,
    r: f64,
    owner_center_id: String,
    owner_sector_index: usize,
}

impl GeneralPlacementLeafZone {
    fn into_json(self) -> Value {
        json!({
            "id": self.id,
            "x": self.x,
            "y": self.y,
            "r": self.r,
            "ownerCenterId": self.owner_center_id,
            "ownerSectorIndex": self.owner_sector_index,
        })
    }
}

#[derive(Clone, Debug)]
struct FastRingPlacementUpdate {
    id: String,
    x: f64,
    y: f64,
    layout_layer: usize,
    owner_center_id: String,
}

impl FastRingPlacementUpdate {
    fn into_json(self) -> Value {
        json!({
            "id": self.id,
            "x": self.x,
            "y": self.y,
            "layoutLayer": self.layout_layer,
            "ownerCenterId": self.owner_center_id,
        })
    }
}

#[derive(Default)]
struct GeneralBatchReport {
    avoidance_hits: usize,
    hard_reject_hits: usize,
    corridor_reject_hits: usize,
    territory_overflow_hits: usize,
    fallback_placements: usize,
    pushout_placements: usize,
}

impl GeneralBatchReport {
    fn add_selection(&mut self, selection: &NetworkCandidateSelection) {
        self.avoidance_hits += selection.avoidance_hits;
        self.hard_reject_hits += selection.hard_reject_hits;
        self.corridor_reject_hits += selection.corridor_reject_hits;
        self.territory_overflow_hits += selection.territory_overflow_hits;
    }

    fn add_batch(&mut self, batch: &GeneralBatchReport) {
        self.avoidance_hits += batch.avoidance_hits;
        self.hard_reject_hits += batch.hard_reject_hits;
        self.corridor_reject_hits += batch.corridor_reject_hits;
        self.territory_overflow_hits += batch.territory_overflow_hits;
        self.fallback_placements += batch.fallback_placements;
        self.pushout_placements += batch.pushout_placements;
    }

    fn to_json(&self) -> Value {
        json!({
            "avoidanceHits": self.avoidance_hits,
            "hardRejectHits": self.hard_reject_hits,
            "corridorRejectHits": self.corridor_reject_hits,
            "territoryOverflowHits": self.territory_overflow_hits,
            "fallbackPlacements": self.fallback_placements,
            "pushoutPlacements": self.pushout_placements,
        })
    }
}

struct GeneralBatchPlacement {
    layer: usize,
    sector_index: usize,
    leaf_start_index: usize,
    take: usize,
    completed: bool,
    stopped_at: Option<usize>,
    updates: Vec<GeneralPlacementUpdate>,
    leaf_zones: Vec<GeneralPlacementLeafZone>,
    report: GeneralBatchReport,
}

impl GeneralBatchPlacement {
    fn into_json(self) -> Value {
        json!({
            "layer": self.layer,
            "sectorIndex": self.sector_index,
            "leafStartIndex": self.leaf_start_index,
            "take": self.take,
            "completed": self.completed,
            "stoppedAt": self.stopped_at,
            "updates": self.updates.into_iter().map(GeneralPlacementUpdate::into_json).collect::<Vec<_>>(),
            "leafZones": self.leaf_zones.into_iter().map(GeneralPlacementLeafZone::into_json).collect::<Vec<_>>(),
            "report": self.report.to_json(),
        })
    }
}

fn project_general_attempt_selection(
    input: &NetworkSectorPlacementInput,
    sector_plan: &NetworkBucketSectorPlan,
) -> Value {
    let Some(plan) = project_general_batch_plan(
        input,
        sector_plan,
        input.general_batch_start_index,
        input.general_batch_layer,
        input.general_batch_sector_index,
    ) else {
        return Value::Null;
    };
    let Some(item) = plan.items.first().copied() else {
        return Value::Null;
    };

    let occupied_zones = candidate_zones_from_circle(&input.occupied_zones);
    let hard_zones = candidate_zones_from_circle(&input.hard_zones);
    let hard_corridors = candidate_hard_corridors_from_model(&input.hard_corridors);
    let territory_ranges = candidate_angle_ranges_from_model(&input.territory_ranges);
    project_general_batch_candidate(
        input,
        &plan,
        item,
        0,
        &occupied_zones,
        &hard_zones,
        &hard_corridors,
        &territory_ranges,
    )
}

fn project_general_batch_placement(
    input: &NetworkSectorPlacementInput,
    sector_plan: &NetworkBucketSectorPlan,
) -> Value {
    let Some(plan) = project_general_batch_plan(
        input,
        sector_plan,
        input.general_batch_start_index,
        input.general_batch_layer,
        input.general_batch_sector_index,
    ) else {
        return Value::Null;
    };
    let mut occupied_zones = candidate_zones_from_circle(&input.occupied_zones);
    let hard_zones = candidate_zones_from_circle(&input.hard_zones);
    let hard_corridors = candidate_hard_corridors_from_model(&input.hard_corridors);
    let territory_ranges = candidate_angle_ranges_from_model(&input.territory_ranges);
    project_general_batch_placement_from_plan(
        input,
        &plan,
        &mut occupied_zones,
        &hard_zones,
        &hard_corridors,
        &territory_ranges,
    )
    .into_json()
}

fn project_general_batch_placement_from_plan(
    input: &NetworkSectorPlacementInput,
    plan: &GeneralBatchPlan<'_>,
    occupied_zones: &mut Vec<CandidateZone>,
    hard_zones: &[CandidateZone],
    hard_corridors: &[CandidateHardCorridor],
    territory_ranges: &[CandidateAngleRange],
) -> GeneralBatchPlacement {
    let mut updates = Vec::new();
    let mut leaf_zones = Vec::new();
    let mut report = GeneralBatchReport::default();
    let mut completed = true;
    let mut stopped_at: Option<usize> = None;

    for (index, item) in plan.items.iter().enumerate() {
        let placement = project_general_node_placement(
            input,
            plan,
            item,
            index,
            occupied_zones,
            hard_zones,
            hard_corridors,
            territory_ranges,
        );
        let selection = &placement.selection;
        report.add_selection(selection);
        if placement.fallback_placement {
            report.fallback_placements += 1;
        }
        if placement.pushout_placement {
            report.pushout_placements += 1;
        }
        report.territory_overflow_hits += placement.territory_overflow_hits;

        let Some(best) = placement.best else {
            completed = false;
            stopped_at = Some(index);
            break;
        };
        let zone_r = item.collision_radius + 4.0;
        occupied_zones.push(CandidateZone {
            x: best.x,
            y: best.y,
            r: zone_r,
            owner_center_id: input.center.id.clone(),
        });
        if input.include_general_updates {
            updates.push(GeneralPlacementUpdate {
                id: item.id.clone(),
                x: best.x,
                y: best.y,
                owner_center_id: input.center.id.clone(),
            });
        }
        leaf_zones.push(GeneralPlacementLeafZone {
            id: item.id.clone(),
            x: best.x,
            y: best.y,
            r: zone_r,
            owner_center_id: input.center.id.clone(),
            owner_sector_index: plan.sector_index,
        });
    }

    GeneralBatchPlacement {
        layer: plan.layer,
        sector_index: plan.sector_index,
        leaf_start_index: plan.leaf_start_index,
        take: plan.take,
        completed,
        stopped_at,
        updates,
        leaf_zones,
        report,
    }
}

fn project_general_batch_placements(
    input: &NetworkSectorPlacementInput,
    sector_plan: &NetworkBucketSectorPlan,
) -> Value {
    let leaf_count = input.leaf_rows.len();
    let Some(first_plan) = project_general_batch_plan(
        input,
        sector_plan,
        input.general_batch_start_index,
        input.general_batch_layer,
        input.general_batch_sector_index,
    ) else {
        return Value::Null;
    };
    let first_min_radius = first_plan.min_radius;
    let first_max_leaf_radius = first_plan.max_leaf_radius;
    let first_layer_gap = first_plan.layer_gap;
    let max_leaf_radius_json = if first_max_leaf_radius.is_finite() {
        json!(first_max_leaf_radius)
    } else {
        Value::Null
    };
    let mut occupied_zones = candidate_zones_from_circle(&input.occupied_zones);
    let hard_zones = candidate_zones_from_circle(&input.hard_zones);
    let hard_corridors = candidate_hard_corridors_from_model(&input.hard_corridors);
    let territory_ranges = candidate_angle_ranges_from_model(&input.territory_ranges);
    let mut batches = Vec::new();
    let mut updates = Vec::new();
    let mut leaf_zones = Vec::new();
    let mut report = GeneralBatchReport::default();
    let mut completed = true;
    let mut stopped_at: Option<usize> = None;
    let mut leaf_start_index = first_plan.leaf_start_index;
    let mut layer = first_plan.layer;
    let mut sector_index = first_plan.sector_index;
    let sector_len = sector_plan.sectors.len().max(1);
    let mut first_plan = Some(first_plan);
    let mut batch_index = 0usize;

    while leaf_start_index < leaf_count {
        let plan = if let Some(plan) = first_plan.take() {
            plan
        } else {
            let Some(plan) = project_general_batch_plan(
                input,
                sector_plan,
                leaf_start_index,
                layer,
                sector_index,
            ) else {
                completed = false;
                stopped_at = Some(batch_index);
                break;
            };
            plan
        };
        let batch = project_general_batch_placement_from_plan(
            input,
            &plan,
            &mut occupied_zones,
            &hard_zones,
            &hard_corridors,
            &territory_ranges,
        );
        report.add_batch(&batch.report);
        let batch_completed = batch.completed;
        if input.include_general_batches {
            updates.extend(batch.updates.iter().cloned());
            leaf_zones.extend(batch.leaf_zones.iter().cloned());
            batches.push(batch.into_json());
        } else {
            updates.extend(batch.updates);
            leaf_zones.extend(batch.leaf_zones);
        }
        if !batch_completed {
            completed = false;
            stopped_at = Some(batch_index);
            break;
        }
        leaf_start_index += plan.take;
        sector_index += 1;
        if sector_index >= sector_len {
            sector_index = 0;
            layer += 1;
        }
        batch_index += 1;
    }
    let post_envelope_result = project_general_post_envelope_result(
        input,
        sector_plan,
        &leaf_zones,
        first_min_radius,
        first_max_leaf_radius,
    );
    let post_uniform_result = project_post_uniformity_result(
        input,
        sector_plan,
        &post_envelope_result.zones,
        first_min_radius,
        first_max_leaf_radius,
    );
    let post_uniform_envelope_result = project_post_envelope_result(
        input,
        sector_plan,
        &post_uniform_result.zones,
        first_min_radius * 0.78,
        first_max_leaf_radius,
    );

    json!({
        "completed": completed,
        "stoppedAt": stopped_at,
        "startIndex": input.general_batch_start_index.min(leaf_count),
        "startLayer": input.general_batch_layer,
        "startSectorIndex": input.general_batch_sector_index.min(sector_len.saturating_sub(1)),
        "minRadius": first_min_radius,
        "maxLeafRadius": max_leaf_radius_json,
        "layerGap": first_layer_gap,
        "batches": if input.include_general_batches { Value::Array(batches) } else { Value::Array(Vec::new()) },
        "updates": updates.into_iter().map(GeneralPlacementUpdate::into_json).collect::<Vec<_>>(),
        "leafZones": leaf_zones.into_iter().map(GeneralPlacementLeafZone::into_json).collect::<Vec<_>>(),
        "postEnvelopePlacement": envelope_result_json(&post_envelope_result),
        "postUniformPlacement": uniformity_result_json(&post_uniform_result),
        "postUniformEnvelopePlacement": envelope_result_json(&post_uniform_envelope_result),
        "report": report.to_json(),
    })
}

fn project_general_post_envelope_result(
    input: &NetworkSectorPlacementInput,
    sector_plan: &NetworkBucketSectorPlan,
    leaf_zones: &[GeneralPlacementLeafZone],
    min_radius: f64,
    max_leaf_radius: f64,
) -> EnvelopeResult {
    let envelope_leaf_zones = leaf_zones
        .iter()
        .map(|zone| EnvelopeZone {
            id: zone.id.clone(),
            x: zone.x,
            y: zone.y,
            r: zone.r,
            owner_center_id: zone.owner_center_id.clone(),
            owner_sector_index: zone.owner_sector_index,
        })
        .collect::<Vec<_>>();
    project_post_envelope_result(
        input,
        sector_plan,
        &envelope_leaf_zones,
        min_radius * 0.78,
        max_leaf_radius,
    )
}

fn project_general_batch_plan<'a>(
    input: &'a NetworkSectorPlacementInput,
    sector_plan: &NetworkBucketSectorPlan,
    leaf_start_index_input: usize,
    layer_input: usize,
    sector_index_input: usize,
) -> Option<GeneralBatchPlan<'a>> {
    let leaf_count = input.leaf_rows.len();
    if leaf_count == 0
        || sector_plan.sectors.is_empty()
        || is_fast_ring_eligible(input, sector_plan)
    {
        return None;
    }

    let sectors = &sector_plan.sectors;
    let leaf_start_index = leaf_start_index_input.min(leaf_count);
    let remaining = leaf_count.saturating_sub(leaf_start_index);
    if remaining == 0 {
        return None;
    }
    let avg_label_pad = if sector_plan.avg_label_pad.is_finite() && sector_plan.avg_label_pad > 0.0
    {
        sector_plan.avg_label_pad
    } else {
        input.leaf_rows.iter().map(|row| row.label_pad).sum::<f64>() / (leaf_count as f64)
    };
    let dense_sector = sector_plan.dense_sector;
    let min_radius_base =
        if sector_plan.min_radius_base.is_finite() && sector_plan.min_radius_base != 0.0 {
            sector_plan.min_radius_base
        } else {
            number_field(&input.config, "R_LEAF_MIN")
                .unwrap_or(220.0)
                .max(80.0)
        };
    let min_radius_boost = (1.0 + (leaf_count as f64).sqrt() / 15.0).clamp(1.0, 2.2);
    let dense_radius_scale = if dense_sector {
        clamp_f64(1.0 - (leaf_count.max(1) as f64).sqrt() / 26.0, 0.58, 0.86)
    } else {
        1.0
    };
    let multi_sector_radius_scale = if sectors.len() > 1 {
        clamp_f64(1.0 - ((sectors.len() - 1) as f64) * 0.08, 0.72, 1.0)
    } else {
        1.0
    };
    let min_radius =
        min_radius_base * min_radius_boost * dense_radius_scale * multi_sector_radius_scale;
    let max_leaf_radius = input
        .max_leaf_radius
        .filter(|value| value.is_finite())
        .map(|value| value.max(min_radius + 24.0))
        .unwrap_or(f64::INFINITY);
    let layer_gap_base = number_field(&input.config, "NETWORK_SECTOR_LAYER_GAP")
        .unwrap_or(68.0)
        .max(22.0);
    let layer_gap_dense_boost = if dense_sector {
        clamp_f64(1.0 + (leaf_count.max(1) as f64).sqrt() / 12.5, 1.26, 2.4)
    } else {
        1.0
    };
    let layer_gap_sector_boost = if sectors.len() > 1 {
        clamp_f64(1.0 + (sectors.len() as f64) * 0.08, 1.0, 1.45)
    } else {
        1.0
    };
    let layer_gap = layer_gap_base
        * (1.0 + (leaf_count as f64).sqrt() / 22.0).clamp(1.0, 1.9)
        * layer_gap_dense_boost
        * layer_gap_sector_boost;
    let min_node_gap_base = (34.0 + (leaf_count as f64).sqrt() * 2.2).clamp(36.0, 96.0);
    let label_gap_boost = clamp_f64(0.92 + avg_label_pad / 36.0, 0.94, 1.86);
    let min_node_gap = min_node_gap_base * label_gap_boost;
    let target_layer_count = if dense_sector {
        ((leaf_count.max(1) as f64).sqrt() / if sectors.len() > 1 { 1.35 } else { 1.55 })
            .ceil()
            .max(5.0) as usize
    } else {
        0
    };
    let layer_capacity_cap = if dense_sector {
        ((leaf_count as f64) / (target_layer_count.max(1) as f64))
            .ceil()
            .max(4.0) as usize
    } else {
        usize::MAX
    };
    let layer = layer_input;
    let sector_index = sector_index_input.min(sectors.len().saturating_sub(1));
    let sector = sectors[sector_index];
    let span = (sector.end - sector.start).max(0.24);
    let keep_in_sector = dense_sector || leaf_count >= 24;
    let radius_raw = min_radius + (layer as f64) * layer_gap;
    let radius = radius_raw.min(max_leaf_radius);
    let at_radius_cap = radius_raw >= max_leaf_radius - 1e-6;
    let effective_node_gap = if at_radius_cap {
        (min_node_gap * 0.82).max(24.0)
    } else {
        min_node_gap
    };
    let raw_capacity = ((span * radius) / effective_node_gap).floor().max(2.0) as usize;
    let sectors_left = sectors.len().saturating_sub(sector_index).max(1);
    let fair_cap_scale = if dense_sector && sectors.len() > 1 {
        0.94
    } else {
        1.02
    };
    let fair_cap = if sectors.len() > 1 {
        ((remaining as f64) / (sectors_left as f64) * fair_cap_scale)
            .ceil()
            .max(2.0) as usize
    } else {
        remaining
    };
    let capacity = raw_capacity.min(layer_capacity_cap).min(fair_cap).max(2);
    let take = capacity.min(remaining);
    if take == 0 {
        return None;
    }

    let mut items = input
        .leaf_rows
        .iter()
        .skip(leaf_start_index)
        .take(take)
        .collect::<Vec<_>>();
    items.sort_by(|a, b| {
        b.label_pad
            .total_cmp(&a.label_pad)
            .then_with(|| a.id.cmp(&b.id))
    });
    if items.is_empty() {
        return None;
    }

    Some(GeneralBatchPlan {
        dense_sector,
        min_radius,
        max_leaf_radius,
        layer_gap,
        radius,
        layer,
        sector_index,
        leaf_start_index,
        sector,
        span,
        keep_in_sector,
        take,
        items,
    })
}

#[allow(clippy::too_many_arguments)]
fn project_general_batch_candidate(
    input: &NetworkSectorPlacementInput,
    plan: &GeneralBatchPlan<'_>,
    item: &NetworkPlacementLeafRow,
    index: usize,
    occupied_zones: &[CandidateZone],
    hard_zones: &[CandidateZone],
    hard_corridors: &[CandidateHardCorridor],
    territory_ranges: &[CandidateAngleRange],
) -> Value {
    project_general_batch_candidate_model(
        input,
        plan,
        item,
        index,
        occupied_zones,
        hard_zones,
        hard_corridors,
        territory_ranges,
    )
    .to_json()
}

#[allow(clippy::too_many_arguments)]
fn project_general_batch_candidate_model(
    input: &NetworkSectorPlacementInput,
    plan: &GeneralBatchPlan<'_>,
    item: &NetworkPlacementLeafRow,
    index: usize,
    occupied_zones: &[CandidateZone],
    hard_zones: &[CandidateZone],
    hard_corridors: &[CandidateHardCorridor],
    territory_ranges: &[CandidateAngleRange],
) -> NetworkCandidateSelection {
    let attempt = project_general_attempt_params(input, plan, item, index);
    project_network_candidate_attempt_selection_model(NetworkCandidateAttemptSelectionInput {
        center_id: &input.center.id,
        center_x: input.center.x,
        center_y: input.center.y,
        metrics: NetworkNodeMetrics {
            radius: item.node_radius,
            label: item.label_pad,
            collision_radius: item.collision_radius,
            label_half_width: 24.0,
            label_bottom: 42.0,
        },
        occupied_zones,
        hard_zones,
        hard_corridors,
        territory_ranges,
        owner_centers: &input.owner_centers,
        strict_center_ownership: input.strict_center_ownership,
        allow_territory_overflow: input.allow_territory_overflow,
        sector_start: plan.sector.start,
        sector_span: plan.span,
        keep_in_sector: plan.keep_in_sector,
        base_angle: attempt.base_angle,
        target_radius: attempt.target_radius,
        min_radius: plan.min_radius,
        max_leaf_radius: plan.max_leaf_radius,
        layer_gap: plan.layer_gap,
        attempt_max: attempt.attempt_max,
        shift_step: attempt.shift_step,
        angle_jitter_base: attempt.angle_jitter_base,
        attempt_phase_seed: attempt.attempt_phase_seed,
        dense_sector: plan.dense_sector,
        hard_padding: 12.0,
        hard_corridor_padding: 12.0,
        overlap_padding: 8.0,
        overlap_weight: 7.2,
        ray_angle_threshold: attempt.ray_angle_threshold,
        ray_radial_threshold: attempt.ray_radial_threshold,
        early_penalty: 0.08,
        clean_ray_threshold: 0.35,
    })
}

fn project_general_attempt_params(
    input: &NetworkSectorPlacementInput,
    plan: &GeneralBatchPlan<'_>,
    item: &NetworkPlacementLeafRow,
    index: usize,
) -> GeneralAttemptParams {
    let phase_seed = stable_hash_unit(&format!(
        "{}|layer|{}|{}",
        input.center.id, plan.layer, plan.sector_index
    ));
    let layer_phase = if plan.dense_sector {
        ((plan.layer as f64) * 0.5 + (plan.sector_index as f64) * 0.19 + phase_seed * 0.12) % 1.0
    } else {
        (phase_seed * 0.35 + (plan.layer as f64) * 0.19 + (plan.sector_index as f64) * 0.11) % 1.0
    };
    let ratio = if plan.take <= 1 {
        0.5
    } else {
        ((index as f64) + 0.5 + layer_phase) / (plan.take as f64) % 1.0
    };
    let angle_jitter_base = clamp_f64(
        plan.span / 18.0_f64.max((plan.take as f64) * 1.42),
        0.018,
        if plan.dense_sector { 0.13 } else { 0.09 },
    );
    let base_angle_noise = (stable_hash_unit(&format!(
        "sector-angle|{}|{}|{}|{}",
        input.center.id, item.id, plan.layer, plan.sector_index
    )) - 0.5)
        * 2.0
        * angle_jitter_base;
    let base_angle_raw = plan.sector.start + plan.span * ratio + base_angle_noise;
    let base_angle = if plan.keep_in_sector {
        clamp_angle_to_arc(base_angle_raw, plan.sector.start, plan.span)
    } else {
        base_angle_raw
    };
    let radial_spread = 52.0_f64.min(plan.layer_gap * 0.42);
    let radial_jitter_range = if plan.dense_sector {
        radial_spread
    } else {
        24.0_f64.min(plan.layer_gap * 0.2)
    };
    let radial_jitter = (stable_hash_unit(&format!(
        "sector-radial|{}|{}|{}",
        input.center.id, item.id, plan.layer
    )) - 0.5)
        * radial_jitter_range;
    let target_radius = clamp_f64(
        plan.radius + radial_jitter,
        plan.min_radius * 0.78,
        plan.max_leaf_radius,
    );
    let attempt_max = if plan.dense_sector {
        if plan.take > 24 {
            30
        } else {
            24
        }
    } else if plan.take > 14 {
        18
    } else {
        12
    };
    let shift_step = clamp_f64(
        plan.span / 10.0_f64.max((plan.take as f64) * 0.9),
        0.045,
        if plan.dense_sector { 0.2 } else { 0.16 },
    );
    let ray_angle_threshold = clamp_f64(
        plan.span / 18.0_f64.max((plan.take as f64) * if plan.dense_sector { 1.22 } else { 1.08 }),
        0.022,
        if plan.dense_sector { 0.1 } else { 0.13 },
    );
    let attempt_phase_seed = stable_hash_unit(&format!(
        "sector-attempt|{}|{}|{}|{}",
        input.center.id, item.id, plan.layer, plan.sector_index
    ));

    GeneralAttemptParams {
        base_angle,
        target_radius,
        attempt_max,
        shift_step,
        angle_jitter_base,
        attempt_phase_seed,
        ray_angle_threshold,
        ray_radial_threshold: (item.label_pad * 1.45).max(20.0),
    }
}

#[allow(clippy::too_many_arguments)]
fn project_general_node_placement(
    input: &NetworkSectorPlacementInput,
    plan: &GeneralBatchPlan<'_>,
    item: &NetworkPlacementLeafRow,
    index: usize,
    occupied_zones: &[CandidateZone],
    hard_zones: &[CandidateZone],
    hard_corridors: &[CandidateHardCorridor],
    territory_ranges: &[CandidateAngleRange],
) -> GeneralNodePlacement {
    let attempt = project_general_attempt_params(input, plan, item, index);
    let selection = project_general_batch_candidate_model(
        input,
        plan,
        item,
        index,
        occupied_zones,
        hard_zones,
        hard_corridors,
        territory_ranges,
    );
    let mut best = selection.best.map(|best| GeneralBest {
        x: best.x,
        y: best.y,
        angle: best.angle,
    });
    let best_penalty = selection.best_penalty.unwrap_or(f64::INFINITY);
    let ownership = NetworkCenterOwnershipChecker::new(
        &input.center.id,
        &input.owner_centers,
        input.strict_center_ownership,
    );
    let owns = |x: f64, y: f64| ownership.as_ref().is_some_and(|checker| checker.owns(x, y));
    let is_in_territory = |angle: f64| is_angle_in_territory(angle, &input.territory_ranges);
    let has_territory_constraint = !input.territory_ranges.is_empty()
        && !input
            .territory_ranges
            .iter()
            .any(|row| row.span.unwrap_or(0.0) >= std::f64::consts::PI * 2.0 - 1e-3);
    let mut fallback_placement = false;
    let mut pushout_placement = false;
    let mut territory_overflow_hits = 0usize;

    if best.is_none() {
        let preferred_angle = input
            .preferred_angles
            .first()
            .map(|row| row.angle)
            .filter(|value| value.is_finite());
        let fallback_base = if !plan.keep_in_sector {
            preferred_angle.unwrap_or(attempt.base_angle)
        } else {
            attempt.base_angle
        };
        let mut fallback_angle = if has_territory_constraint
            && !input.allow_territory_overflow
            && !is_in_territory(fallback_base)
        {
            pick_representative_angle(&input.territory_ranges, fallback_base)
        } else {
            fallback_base
        };
        if plan.keep_in_sector {
            fallback_angle = clamp_angle_to_arc(fallback_angle, plan.sector.start, plan.span);
        }

        let mut fallback_best = None;
        let aa_max = if plan.dense_sector { 96 } else { 48 };
        let golden = 0.6180339887498949;
        let fallback_phase = stable_hash_unit(&format!(
            "sector-fallback|{}|{}|{}|{}",
            input.center.id, item.id, plan.layer, plan.sector_index
        ));
        let fallback_sweep = if plan.keep_in_sector {
            plan.span
        } else if has_territory_constraint && !input.allow_territory_overflow {
            (plan.span * 1.04).max(std::f64::consts::PI * 0.66)
        } else {
            std::f64::consts::PI * 2.0
        };
        let layer_gap_config =
            number_field(&input.config, "NETWORK_SECTOR_LAYER_GAP").unwrap_or(0.0);
        for ra in 0..=14 {
            let rr_raw = attempt.target_radius + (ra as f64) * (layer_gap_config * 0.18).max(10.0);
            let rr = clamp_f64(rr_raw, plan.min_radius * 0.78, plan.max_leaf_radius);
            for aa in 0..=aa_max {
                let t = (fallback_phase + (aa as f64) * golden) % 1.0;
                let shift = (t - 0.5) * fallback_sweep;
                let angle = if plan.keep_in_sector {
                    plan.sector.start + t * plan.span
                } else {
                    fallback_angle + shift
                };
                let in_territory = is_in_territory(angle);
                if !in_territory && has_territory_constraint && !input.allow_territory_overflow {
                    continue;
                }
                if !in_territory
                    && has_territory_constraint
                    && input.allow_territory_overflow
                    && ra < 4
                {
                    continue;
                }
                let x = input.center.x + angle.cos() * rr;
                let y = input.center.y + angle.sin() * rr;
                if !owns(x, y)
                    || candidate_collides_with_zones(x, y, item.collision_radius, hard_zones, 10.0)
                    || candidate_collides_with_corridors(
                        x,
                        y,
                        item.node_radius,
                        item.label_pad,
                        hard_corridors,
                        10.0,
                    )
                {
                    continue;
                }
                let overlap = candidate_overlap_penalty(
                    x,
                    y,
                    item.collision_radius,
                    occupied_zones,
                    8.0,
                    1e-6,
                );
                if overlap > 1e-6 {
                    continue;
                }
                let ray = candidate_ray_cluster_penalty(
                    input.center.x,
                    input.center.y,
                    x,
                    y,
                    occupied_zones,
                    &input.center.id,
                    attempt.ray_angle_threshold,
                    attempt.ray_radial_threshold,
                    1.2,
                );
                if ray > 1.2 {
                    continue;
                }
                fallback_best = Some(GeneralBest { x, y, angle });
                if !in_territory && has_territory_constraint {
                    territory_overflow_hits += 1;
                }
                break;
            }
            if fallback_best.is_some() {
                break;
            }
        }

        if fallback_best.is_none() {
            let mut safest = None;
            let mut safest_penalty = f64::INFINITY;
            let sample_count = if plan.keep_in_sector { 120 } else { 80 };
            let base_radius = clamp_f64(
                attempt.target_radius,
                plan.min_radius * 0.78,
                plan.max_leaf_radius,
            );
            for aa in 0..sample_count {
                let t = ((aa as f64) + 0.5) / (sample_count as f64);
                let angle = if plan.keep_in_sector {
                    plan.sector.start + t * plan.span
                } else {
                    fallback_angle + (t - 0.5) * std::f64::consts::PI * 2.0
                };
                let x = input.center.x + angle.cos() * base_radius;
                let y = input.center.y + angle.sin() * base_radius;
                if !owns(x, y)
                    || candidate_collides_with_zones(x, y, item.collision_radius, hard_zones, 10.0)
                    || candidate_collides_with_corridors(
                        x,
                        y,
                        item.node_radius,
                        item.label_pad,
                        hard_corridors,
                        10.0,
                    )
                {
                    continue;
                }
                let overlap_stop_at = if safest_penalty.is_finite() {
                    (safest_penalty / 7.2).max(0.0)
                } else {
                    f64::INFINITY
                };
                let overlap = candidate_overlap_penalty(
                    x,
                    y,
                    item.collision_radius,
                    occupied_zones,
                    8.0,
                    overlap_stop_at,
                );
                let overlap_weighted = overlap * 7.2;
                if safest_penalty.is_finite() && overlap_weighted >= safest_penalty {
                    continue;
                }
                let ray_stop_at = if safest_penalty.is_finite() {
                    (safest_penalty - overlap_weighted).max(0.0)
                } else {
                    f64::INFINITY
                };
                let ray = candidate_ray_cluster_penalty(
                    input.center.x,
                    input.center.y,
                    x,
                    y,
                    occupied_zones,
                    &input.center.id,
                    attempt.ray_angle_threshold,
                    attempt.ray_radial_threshold,
                    ray_stop_at,
                );
                let total_penalty = overlap_weighted + ray;
                if total_penalty < safest_penalty {
                    safest = Some(GeneralBest { x, y, angle });
                    safest_penalty = total_penalty;
                    if total_penalty <= 0.08 {
                        break;
                    }
                }
            }
            fallback_best = safest;
        }

        best = fallback_best;
        if best.is_none() {
            let mut final_angle = if plan.keep_in_sector {
                clamp_angle_to_arc(fallback_angle, plan.sector.start, plan.span)
            } else {
                fallback_angle
            };
            let final_radius = clamp_f64(
                attempt.target_radius,
                plan.min_radius * 0.78,
                plan.max_leaf_radius,
            );
            let mut final_x = input.center.x + final_angle.cos() * final_radius;
            let mut final_y = input.center.y + final_angle.sin() * final_radius;
            if !owns(final_x, final_y) {
                for aa in 0..180 {
                    let t = ((aa as f64) + 0.5) / 180.0;
                    let angle = if plan.keep_in_sector {
                        plan.sector.start + t * plan.span
                    } else {
                        final_angle + (t - 0.5) * std::f64::consts::PI * 2.0
                    };
                    let x = input.center.x + angle.cos() * final_radius;
                    let y = input.center.y + angle.sin() * final_radius;
                    if !owns(x, y) {
                        continue;
                    }
                    final_angle = angle;
                    final_x = x;
                    final_y = y;
                    break;
                }
            }
            best = Some(GeneralBest {
                x: final_x,
                y: final_y,
                angle: final_angle,
            });
        }
        fallback_placement = best.is_some();
    } else if best_penalty > 0.08 {
        let mut pushed = None;
        let radial_step = (plan.layer_gap * 0.22).max(10.0);
        if let Some(current) = &best {
            for step in 1..=16 {
                let rr_raw = attempt.target_radius + (step as f64) * radial_step;
                let rr = clamp_f64(rr_raw, plan.min_radius * 0.78, plan.max_leaf_radius);
                let swing = clamp_f64(
                    attempt.shift_step * (1.9 + (step as f64) * 0.35),
                    0.05,
                    plan.span * 0.46,
                );
                for angle_raw in [current.angle, current.angle + swing, current.angle - swing] {
                    let angle = if plan.keep_in_sector {
                        clamp_angle_to_arc(angle_raw, plan.sector.start, plan.span)
                    } else {
                        angle_raw
                    };
                    let x = input.center.x + angle.cos() * rr;
                    let y = input.center.y + angle.sin() * rr;
                    if !owns(x, y)
                        || candidate_collides_with_zones(
                            x,
                            y,
                            item.collision_radius,
                            hard_zones,
                            10.0,
                        )
                        || candidate_collides_with_corridors(
                            x,
                            y,
                            item.node_radius,
                            item.label_pad,
                            hard_corridors,
                            10.0,
                        )
                    {
                        continue;
                    }
                    let overlap = candidate_overlap_penalty(
                        x,
                        y,
                        item.collision_radius,
                        occupied_zones,
                        8.0,
                        1e-6,
                    );
                    if overlap > 1e-6 {
                        continue;
                    }
                    let ray = candidate_ray_cluster_penalty(
                        input.center.x,
                        input.center.y,
                        x,
                        y,
                        occupied_zones,
                        &input.center.id,
                        attempt.ray_angle_threshold,
                        attempt.ray_radial_threshold,
                        1.2,
                    );
                    if ray > 1.2 {
                        continue;
                    }
                    pushed = Some(GeneralBest { x, y, angle });
                    break;
                }
                if pushed.is_some() {
                    break;
                }
            }
        }
        if pushed.is_some() {
            best = pushed;
            pushout_placement = true;
        }
    }

    GeneralNodePlacement {
        selection,
        best,
        fallback_placement,
        pushout_placement,
        territory_overflow_hits,
    }
}

fn is_fast_ring_eligible(
    input: &NetworkSectorPlacementInput,
    sector_plan: &NetworkBucketSectorPlan,
) -> bool {
    if input.leaf_rows.len() < 36
        || sector_plan.sectors.len() != 1
        || (input.strict_center_ownership && input.owner_centers.len() > 1)
    {
        return false;
    }
    (sector_plan.sectors[0].end - sector_plan.sectors[0].start).max(0.0) > 0.12
}

fn candidate_zones_from_circle(zones: &[NetworkCircleZone]) -> Vec<CandidateZone> {
    zones
        .iter()
        .map(|zone| CandidateZone {
            x: zone.x,
            y: zone.y,
            r: zone.r,
            owner_center_id: String::new(),
        })
        .collect()
}

fn candidate_hard_corridors_from_model(
    corridors: &[NetworkHardCorridor],
) -> Vec<CandidateHardCorridor> {
    corridors
        .iter()
        .map(|corridor| CandidateHardCorridor {
            x1: corridor.x1,
            y1: corridor.y1,
            x2: corridor.x2,
            y2: corridor.y2,
            half_width: corridor.half_width,
        })
        .collect()
}

fn candidate_angle_ranges_from_model(
    ranges: &[NetworkBucketAngleRange],
) -> Vec<CandidateAngleRange> {
    ranges
        .iter()
        .map(|range| CandidateAngleRange {
            start: range.start,
            end: range.end,
            span: range.span,
        })
        .collect()
}

fn project_fast_ring_placement(
    input: &NetworkSectorPlacementInput,
    sector_plan: &NetworkBucketSectorPlan,
) -> Value {
    let leaf_count = input.leaf_rows.len();
    if leaf_count < 36
        || sector_plan.sectors.len() != 1
        || (input.strict_center_ownership && input.owner_centers.len() > 1)
    {
        return Value::Null;
    }
    let sector = sector_plan.sectors[0];
    let span = (sector.end - sector.start).max(0.0);
    if span <= 0.12 {
        return Value::Null;
    }
    let avg_label_pad = if sector_plan.avg_label_pad.is_finite() && sector_plan.avg_label_pad > 0.0
    {
        sector_plan.avg_label_pad
    } else {
        input.leaf_rows.iter().map(|row| row.label_pad).sum::<f64>() / (leaf_count.max(1) as f64)
    };
    let avg_node_radius = input
        .leaf_rows
        .iter()
        .map(|row| row.node_radius)
        .sum::<f64>()
        / (leaf_count.max(1) as f64);
    let avg_collision = input
        .leaf_rows
        .iter()
        .map(|row| row.collision_radius)
        .sum::<f64>()
        / (leaf_count.max(1) as f64);
    let min_radius_base = sector_plan.min_radius_base.max(
        number_field(&input.config, "R_LEAF_MIN")
            .unwrap_or(220.0)
            .max(80.0),
    );
    let min_radius_boost = (1.0 + (leaf_count as f64).sqrt() / 15.0).clamp(1.0, 2.2);
    let dense_radius_scale = clamp_f64(1.0 - (leaf_count.max(1) as f64).sqrt() / 26.0, 0.58, 0.86);
    let radius_low = (min_radius_base * min_radius_boost * dense_radius_scale * 0.78)
        .max(input.center.collision_radius + avg_collision + 24.0);
    let nominal_arc_gap =
        resolve_network_nominal_arc_gap(leaf_count, avg_label_pad, avg_node_radius, avg_collision);
    let edge_pad = clamp_f64(span * 0.018, 0.012, 0.12);
    let usable_start_rel = edge_pad;
    let usable_end_rel = (span - edge_pad).max(edge_pad + 0.05);
    let usable_span = (usable_end_rel - usable_start_rel).max(0.05);
    let layer_limit = network_sector_layer_limit(leaf_count);
    let layer_gap = number_field(&input.config, "NETWORK_SECTOR_LAYER_GAP")
        .unwrap_or(68.0)
        .max(22.0);
    let per_layer_target = ((leaf_count as f64) / (layer_limit.max(1) as f64))
        .ceil()
        .max(2.0);
    let radius_for_arc_capacity = (per_layer_target * nominal_arc_gap) / usable_span.max(0.12);
    let dense_radius_high_floor = radius_low
        + 120.0_f64
            .max((leaf_count as f64).sqrt() * 34.0)
            .max((layer_limit.saturating_sub(1) as f64) * layer_gap * 0.52)
            .max(radius_for_arc_capacity * 0.42);
    let radius_high = input
        .max_leaf_radius
        .filter(|value| value.is_finite())
        .map(|value| value.max(radius_low + 16.0).max(dense_radius_high_floor))
        .unwrap_or(dense_radius_high_floor);
    let ring_plan = plan_sector_fast_ring_layers(
        leaf_count,
        usable_span,
        radius_low,
        radius_high,
        nominal_arc_gap,
    );
    if ring_plan.is_empty() || ring_plan.iter().map(|row| row.count).sum::<usize>() != leaf_count {
        return Value::Null;
    }

    let angle_offset_steps = [0.0, 1.0, -1.0, 2.0, -2.0, 3.0, -3.0, 4.0, -4.0, 5.0, -5.0];
    let radial_offset_steps = [0.0, 1.0, -1.0];
    let radial_step = clamp_f64(
        (radius_high - radius_low) / 6.0_f64.max((ring_plan.len() as f64) * 1.8),
        6.0,
        22.0,
    );
    let has_territory_constraint = !input.territory_ranges.is_empty()
        && !input
            .territory_ranges
            .iter()
            .any(|row| row.span.unwrap_or(0.0) >= std::f64::consts::PI * 2.0 - 1e-3);
    let mut visible_zones = input
        .occupied_zones
        .iter()
        .map(|zone| (zone.x, zone.y, zone.r.max(0.0)))
        .collect::<Vec<_>>();
    let hard_zones = input
        .hard_zones
        .iter()
        .map(|zone| (zone.x, zone.y, zone.r.max(0.0)))
        .collect::<Vec<_>>();
    let has_hard_zones = !hard_zones.is_empty();
    let has_hard_corridors = !input.hard_corridors.is_empty();
    let Some(ownership) = NetworkCenterOwnershipChecker::new(
        &input.center.id,
        &input.owner_centers,
        input.strict_center_ownership,
    ) else {
        return Value::Null;
    };
    let mut cursor = 0usize;
    let mut updates: Vec<FastRingPlacementUpdate> = Vec::new();
    let mut leaf_zones = Vec::new();
    let mut hard_reject_hits = 0usize;
    let mut corridor_reject_hits = 0usize;
    for (layer_index, layer) in ring_plan.iter().enumerate() {
        if cursor >= leaf_count {
            break;
        }
        let count = layer.count;
        if count == 0 {
            continue;
        }
        let radius = layer.radius;
        let layer_span = clamp_f64(
            usable_span * layer_span_scale_for_count(count),
            0.04,
            usable_span,
        );
        let layer_start_rel = clamp_f64(
            usable_start_rel + (usable_span - layer_span) * 0.5,
            usable_start_rel,
            usable_end_rel - layer_span,
        );
        let phase = stable_hash_unit(&format!(
            "network-fast-sector|{}|{layer_index}",
            input.center.id
        ));
        let angle_step = clamp_f64(
            layer_span / 10.0_f64.max((count as f64) * 1.25),
            0.008,
            0.075,
        );

        for index in 0..count {
            if cursor >= leaf_count {
                break;
            }
            let row = &input.leaf_rows[cursor];
            cursor += 1;
            let ratio = if count <= 1 {
                0.5
            } else {
                (((index as f64) + 0.5 + phase) % (count as f64)) / (count as f64)
            };
            let target_angle = clamp_angle_to_arc(
                sector.start + layer_start_rel + layer_span * ratio,
                sector.start,
                span,
            );
            let jitter = (stable_hash_unit(&format!(
                "network-fast-radius|{}|{}",
                input.center.id, row.id
            )) - 0.5)
                * radial_step
                * 0.72;
            let mut picked = None;
            for rr_mul in radial_offset_steps {
                let radius = clamp_f64(
                    radius + jitter + rr_mul * radial_step,
                    radius_low,
                    radius_high,
                );
                for aa_mul in angle_offset_steps {
                    let angle =
                        clamp_angle_to_arc(target_angle + aa_mul * angle_step, sector.start, span);
                    if has_territory_constraint
                        && !is_angle_in_territory(angle, &input.territory_ranges)
                    {
                        continue;
                    }
                    let x = input.center.x + angle.cos() * radius;
                    let y = input.center.y + angle.sin() * radius;
                    if !ownership.owns(x, y) {
                        continue;
                    }
                    if has_hard_zones
                        && collides_with_zones(x, y, row.collision_radius, &hard_zones, 12.0)
                    {
                        hard_reject_hits += 1;
                        continue;
                    }
                    if has_hard_corridors
                        && collides_with_corridors(
                            x,
                            y,
                            row.node_radius,
                            row.label_pad,
                            &input.hard_corridors,
                            12.0,
                        )
                    {
                        corridor_reject_hits += 1;
                        continue;
                    }
                    if !collides_with_zones(x, y, row.collision_radius, &visible_zones, 8.0) {
                        picked = Some((radius, angle));
                        break;
                    }
                }
                if picked.is_some() {
                    break;
                }
            }
            let fallback_angle = if has_territory_constraint
                && !is_angle_in_territory(target_angle, &input.territory_ranges)
            {
                pick_representative_angle(&input.territory_ranges, target_angle)
            } else {
                target_angle
            };
            let (radius, angle) = if let Some(row) = picked {
                row
            } else {
                let fallback_x = input.center.x + fallback_angle.cos() * radius;
                let fallback_y = input.center.y + fallback_angle.sin() * radius;
                if !ownership.owns(fallback_x, fallback_y)
                    || (has_hard_zones
                        && collides_with_zones(
                            fallback_x,
                            fallback_y,
                            row.collision_radius,
                            &hard_zones,
                            12.0,
                        ))
                    || (has_hard_corridors
                        && collides_with_corridors(
                            fallback_x,
                            fallback_y,
                            row.node_radius,
                            row.label_pad,
                            &input.hard_corridors,
                            12.0,
                        ))
                {
                    return Value::Null;
                }
                (radius, fallback_angle)
            };
            let x = input.center.x + angle.cos() * radius;
            let y = input.center.y + angle.sin() * radius;
            updates.push(FastRingPlacementUpdate {
                id: row.id.clone(),
                x: round_two(x),
                y: round_two(y),
                layout_layer: layer_index,
                owner_center_id: input.center.id.clone(),
            });
            visible_zones.push((x, y, row.collision_radius + 4.0));
            leaf_zones.push(EnvelopeZone {
                id: row.id.clone(),
                x,
                y,
                r: row.collision_radius + 4.0,
                owner_center_id: input.center.id.clone(),
                owner_sector_index: 0,
            });
        }
    }

    let post_envelope_placement =
        project_fast_ring_post_envelope(input, sector_plan, &leaf_zones, radius_low, radius_high);

    json!({
        "nodeUpdateCount": updates.len(),
        "layerCount": ring_plan.len(),
        "radiusLow": round_two(radius_low),
        "radiusHigh": round_two(radius_high),
        "nominalArcGap": round_two(nominal_arc_gap),
        "hardRejectHits": hard_reject_hits,
        "corridorRejectHits": corridor_reject_hits,
        "zones": leaf_zones.iter().map(zone_json).collect::<Vec<_>>(),
        "postEnvelopePlacement": post_envelope_placement,
        "updates": updates.into_iter().map(FastRingPlacementUpdate::into_json).collect::<Vec<_>>(),
    })
}

fn project_fast_ring_post_envelope(
    input: &NetworkSectorPlacementInput,
    sector_plan: &NetworkBucketSectorPlan,
    leaf_zones: &[EnvelopeZone],
    radius_low: f64,
    radius_high: f64,
) -> Value {
    envelope_result_json(&project_post_envelope_result(
        input,
        sector_plan,
        leaf_zones,
        radius_low,
        radius_high,
    ))
}

fn project_post_envelope_result(
    input: &NetworkSectorPlacementInput,
    sector_plan: &NetworkBucketSectorPlan,
    leaf_zones: &[EnvelopeZone],
    min_radius_floor: f64,
    max_radius_cap: f64,
) -> EnvelopeResult {
    let sectors = sector_plan
        .sectors
        .iter()
        .map(|row| EnvelopeSector {
            start: row.start,
            end: row.end,
        })
        .collect::<Vec<_>>();
    let occupied_zones = input
        .occupied_zones
        .iter()
        .map(|zone| EnvelopeZone {
            id: String::new(),
            x: zone.x,
            y: zone.y,
            r: zone.r,
            owner_center_id: String::new(),
            owner_sector_index: 0,
        })
        .chain(leaf_zones.iter().cloned())
        .collect::<Vec<_>>();
    let nodes = input
        .leaf_rows
        .iter()
        .map(|row| {
            (
                row.id.clone(),
                EnvelopeNodeModel {
                    radius: row.node_radius,
                    collision_radius: row.collision_radius,
                    label_pad: row.label_pad,
                },
            )
        })
        .collect::<HashMap<_, _>>();
    let options = EnvelopeOptions {
        hard_zones: input
            .hard_zones
            .iter()
            .map(|zone| EnvelopeZone {
                id: String::new(),
                x: zone.x,
                y: zone.y,
                r: zone.r,
                owner_center_id: String::new(),
                owner_sector_index: 0,
            })
            .collect(),
        hard_corridors: input
            .hard_corridors
            .iter()
            .map(|row| EnvelopeHardCorridor {
                x1: row.x1,
                y1: row.y1,
                x2: row.x2,
                y2: row.y2,
                half_width: row.half_width,
            })
            .collect(),
        territory_ranges: input
            .territory_ranges
            .iter()
            .map(|row| EnvelopeAngleRange {
                start: row.start,
                end: row.end,
                span: row.span,
            })
            .collect(),
        allow_territory_overflow: input.allow_territory_overflow,
        owner_centers: input
            .owner_centers
            .iter()
            .map(|row| EnvelopeOwnerCenter {
                id: row.id.clone(),
                x: row.x,
                y: row.y,
            })
            .collect(),
        strict_center_ownership: input.strict_center_ownership,
        min_radius_floor: min_radius_floor.max(0.0),
        max_radius_cap: max_radius_cap.max(min_radius_floor + 16.0),
    };
    apply_sector_envelope(
        &EnvelopeCenter {
            id: input.center.id.clone(),
            x: input.center.x,
            y: input.center.y,
        },
        &sectors,
        leaf_zones.to_vec(),
        &occupied_zones,
        &nodes,
        &options,
    )
}

fn project_post_uniformity_result(
    input: &NetworkSectorPlacementInput,
    sector_plan: &NetworkBucketSectorPlan,
    leaf_zones: &[EnvelopeZone],
    min_radius: f64,
    max_leaf_radius: f64,
) -> UniformityResult {
    let sectors = sector_plan
        .sectors
        .iter()
        .map(|row| EnvelopeSector {
            start: row.start,
            end: row.end,
        })
        .collect::<Vec<_>>();
    let occupied_zones = input
        .occupied_zones
        .iter()
        .map(|zone| EnvelopeZone {
            id: String::new(),
            x: zone.x,
            y: zone.y,
            r: zone.r,
            owner_center_id: String::new(),
            owner_sector_index: 0,
        })
        .chain(leaf_zones.iter().cloned())
        .collect::<Vec<_>>();
    let nodes = input
        .leaf_rows
        .iter()
        .map(|row| {
            (
                row.id.clone(),
                EnvelopeNodeModel {
                    radius: row.node_radius,
                    collision_radius: row.collision_radius,
                    label_pad: row.label_pad,
                },
            )
        })
        .collect::<HashMap<_, _>>();
    let options = EnvelopeOptions {
        hard_zones: input
            .hard_zones
            .iter()
            .map(|zone| EnvelopeZone {
                id: String::new(),
                x: zone.x,
                y: zone.y,
                r: zone.r,
                owner_center_id: String::new(),
                owner_sector_index: 0,
            })
            .collect(),
        hard_corridors: input
            .hard_corridors
            .iter()
            .map(|row| EnvelopeHardCorridor {
                x1: row.x1,
                y1: row.y1,
                x2: row.x2,
                y2: row.y2,
                half_width: row.half_width,
            })
            .collect(),
        territory_ranges: input
            .territory_ranges
            .iter()
            .map(|row| EnvelopeAngleRange {
                start: row.start,
                end: row.end,
                span: row.span,
            })
            .collect(),
        allow_territory_overflow: input.allow_territory_overflow,
        owner_centers: input
            .owner_centers
            .iter()
            .map(|row| EnvelopeOwnerCenter {
                id: row.id.clone(),
                x: row.x,
                y: row.y,
            })
            .collect(),
        strict_center_ownership: input.strict_center_ownership,
        min_radius_floor: (min_radius * 0.78).max(0.0),
        max_radius_cap: max_leaf_radius.max(min_radius * 0.78 + 16.0),
    };
    apply_sector_uniformity(
        &EnvelopeCenter {
            id: input.center.id.clone(),
            x: input.center.x,
            y: input.center.y,
        },
        &sectors,
        leaf_zones.to_vec(),
        &occupied_zones,
        &nodes,
        &options,
    )
}

fn envelope_result_json(result: &EnvelopeResult) -> Value {
    let updates = result
        .zones
        .iter()
        .map(|zone| {
            json!({
                "id": zone.id.clone(),
                "x": round_two(zone.x),
                "y": round_two(zone.y),
                "ownerCenterId": zone.owner_center_id.clone(),
            })
        })
        .collect::<Vec<_>>();

    json!({
        "adjusted": result.adjusted,
        "pulledByRadius": result.pulled_by_radius,
        "pulledByAngle": result.pulled_by_angle,
        "updates": updates,
        "zones": result.zones.iter().map(zone_json).collect::<Vec<_>>(),
    })
}

fn uniformity_result_json(result: &UniformityResult) -> Value {
    let updates = result
        .zones
        .iter()
        .map(|zone| {
            json!({
                "id": zone.id.clone(),
                "x": round_two(zone.x),
                "y": round_two(zone.y),
                "ownerCenterId": zone.owner_center_id.clone(),
            })
        })
        .collect::<Vec<_>>();

    json!({
        "adjusted": result.adjusted,
        "planned": result.planned,
        "rejected": result.rejected,
        "sparseLayerRebalances": result.sparse_layer_rebalances,
        "fallbackRescues": result.fallback_rescues,
        "updates": updates,
        "zones": result.zones.iter().map(zone_json).collect::<Vec<_>>(),
    })
}

fn collides_with_zones(
    x: f64,
    y: f64,
    radius: f64,
    zones: &[(f64, f64, f64)],
    padding: f64,
) -> bool {
    let radius = radius.max(0.0);
    let padding = padding.max(0.0);
    zones.iter().any(|(zx, zy, zr)| {
        let dx = *zx - x;
        let dy = *zy - y;
        let threshold = radius + zr.max(0.0) + padding;
        dx * dx + dy * dy < threshold * threshold
    })
}

fn collides_with_corridors(
    x: f64,
    y: f64,
    node_radius: f64,
    label_pad: f64,
    corridors: &[NetworkHardCorridor],
    padding: f64,
) -> bool {
    if corridors.is_empty() {
        return false;
    }
    let radius = node_radius.max(0.0) + (label_pad * 0.34).max(8.0) + padding.max(0.0);
    corridors.iter().any(|corridor| {
        let dist =
            distance_point_to_segment(x, y, corridor.x1, corridor.y1, corridor.x2, corridor.y2);
        let threshold = corridor.half_width.max(6.0) + radius;
        dist < threshold
    })
}

fn distance_point_to_segment(px: f64, py: f64, x1: f64, y1: f64, x2: f64, y2: f64) -> f64 {
    let dx = x2 - x1;
    let dy = y2 - y1;
    let denom = dx * dx + dy * dy;
    if !denom.is_finite() || denom <= 1e-9 {
        return ((px - x1) * (px - x1) + (py - y1) * (py - y1)).sqrt();
    }
    let t = (((px - x1) * dx + (py - y1) * dy) / denom).clamp(0.0, 1.0);
    let cx = x1 + dx * t;
    let cy = y1 + dy * t;
    ((px - cx) * (px - cx) + (py - cy) * (py - cy)).sqrt()
}

fn is_angle_in_territory(
    angle: f64,
    ranges: &[crate::flow_layout_network_model::NetworkBucketAngleRange],
) -> bool {
    if ranges.is_empty()
        || ranges
            .iter()
            .any(|row| row.span.unwrap_or(0.0) >= std::f64::consts::PI * 2.0 - 1e-3)
    {
        return true;
    }
    let value = normalize_angle_positive(angle);
    ranges.iter().any(|row| {
        let start = normalize_angle_positive(row.start);
        let end = normalize_angle_positive(row.end);
        if start <= end {
            value >= start && value <= end
        } else {
            value >= start || value <= end
        }
    })
}

fn pick_representative_angle(
    ranges: &[crate::flow_layout_network_model::NetworkBucketAngleRange],
    fallback: f64,
) -> f64 {
    let Some(best) = ranges.iter().max_by(|left, right| {
        left.span
            .unwrap_or(0.0)
            .partial_cmp(&right.span.unwrap_or(0.0))
            .unwrap_or(std::cmp::Ordering::Equal)
    }) else {
        return fallback;
    };
    let best_span = best.span.unwrap_or(0.0);
    if best_span >= std::f64::consts::PI * 2.0 - 1e-3 {
        return 0.0;
    }
    let start_pos = normalize_angle_positive(best.start);
    let end_pos = normalize_angle_positive(best.end);
    let start_norm = if start_pos <= end_pos {
        start_pos
    } else {
        start_pos - std::f64::consts::PI * 2.0
    };
    let end_norm = end_pos;
    normalize_angle_positive(start_norm + (end_norm - start_norm) / 2.0)
}

fn normalize_angle_positive(angle: f64) -> f64 {
    let two_pi = std::f64::consts::PI * 2.0;
    let mut value = angle % two_pi;
    if value < 0.0 {
        value += two_pi;
    }
    value
}

fn clamp_angle_to_arc(angle: f64, start: f64, span: f64) -> f64 {
    let two_pi = std::f64::consts::PI * 2.0;
    let arc_span = clamp_f64(span, 0.0, two_pi);
    if arc_span >= two_pi - 1e-6 {
        return angle;
    }
    let mut rel = (angle - start) % two_pi;
    while rel < 0.0 {
        rel += two_pi;
    }
    start + clamp_f64(rel, 0.0, arc_span)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::{json, Value};

    #[test]
    fn network_sector_placement_contract_parses_full_context() {
        let mut payload = json!({
            "config": {"ANGULAR_BUCKETS": 24, "R_LEAF_MIN": 180, "NETWORK_SECTOR_LAYER_GAP": 64},
            "center": {
                "id": "center",
                "type": "layoutAnchor",
                "x": 12,
                "y": -8,
                "nodeRadius": 20,
                "collisionRadius": 54
            },
            "leafRows": [
                {"id": "leaf-1", "labelPad": 28, "nodeRadius": 18, "collisionRadius": 45},
                {"id": "leaf-2", "labelPad": 34, "nodeRadius": 18, "collisionRadius": 51}
            ],
            "bucketPayload": {
                "config": {"ANGULAR_BUCKETS": 24, "R_LEAF_MIN": 180, "NETWORK_SECTOR_LAYER_GAP": 64},
                "center": {"x": 12, "y": -8},
                "leafLabelPads": [28, 34],
                "preferredAngles": [{"angle": 0, "width": 1.1, "weight": 2}],
                "enforceSingleSector": true
            },
            "bucketProjection": {
                "bucketCount": 24,
                "buckets": [],
                "sectorPlan": null
            },
            "occupiedZones": [{"x": 4, "y": 8, "r": 12}],
            "hardZones": [{"x": 100, "y": 0, "r": 24}],
            "hardCorridors": [{"x1": 0, "y1": 0, "x2": 40, "y2": 0, "halfWidth": 8}],
            "territoryRanges": [{"start": -1, "end": 1, "span": 2}],
            "preferredAngles": [{"angle": 0, "width": 1.1, "weight": 2}],
            "ownerCenters": [{"id": "center", "x": 12, "y": -8}],
            "strictCenterOwnership": true,
            "allowTerritoryOverflow": false,
            "maxLeafRadius": 320
        });
        let contract = project_network_sector_placement(&payload);

        assert_eq!(contract["contextSummary"]["leafCount"], json!(2));
        assert_eq!(contract["contextSummary"]["avgNodeRadius"], json!(18.0));
        assert_eq!(
            contract["contextSummary"]["avgCollisionRadius"],
            json!(48.0)
        );
        assert_eq!(contract["contextSummary"]["occupiedZoneCount"], json!(1));
        assert_eq!(contract["contextSummary"]["hardCorridorCount"], json!(1));
        assert_eq!(
            contract["bucketProjection"]["sectorPlan"]["leafCount"],
            json!(2)
        );
        assert_eq!(contract["simplePlacement"]["nodeUpdateCount"], json!(2));

        if let Some(row) = payload.as_object_mut() {
            row.insert("includeGeneralBatches".to_string(), Value::Bool(false));
            row.insert("includeGeneralUpdates".to_string(), Value::Bool(false));
        }
        let compact_contract = project_network_sector_placement(&payload);
        assert_eq!(
            compact_contract["generalBatchPlacements"]["batches"]
                .as_array()
                .map(Vec::len),
            Some(0)
        );
        assert_eq!(
            compact_contract["generalBatchPlacements"]["updates"]
                .as_array()
                .map(Vec::len),
            Some(0)
        );
        assert_eq!(
            compact_contract["generalBatchPlacements"]["leafZones"]
                .as_array()
                .map(Vec::len),
            Some(2)
        );
    }

    #[test]
    fn fast_ring_respects_strict_owner_gate() {
        let leaf_rows = (0..40)
            .map(|index| {
                json!({
                    "id": format!("leaf-{index}"),
                    "labelPad": 30,
                    "nodeRadius": 18,
                    "collisionRadius": 44
                })
            })
            .collect::<Vec<_>>();
        let leaf_label_pads = (0..40).map(|_| json!(30)).collect::<Vec<_>>();
        let payload = json!({
            "config": {"ANGULAR_BUCKETS": 48, "R_LEAF_MIN": 180, "NETWORK_SECTOR_LAYER_GAP": 64},
            "center": {
                "id": "center",
                "type": "layoutAnchor",
                "x": 0,
                "y": 0,
                "nodeRadius": 24,
                "collisionRadius": 54
            },
            "leafRows": leaf_rows,
            "bucketPayload": {
                "config": {"ANGULAR_BUCKETS": 48, "R_LEAF_MIN": 180, "NETWORK_SECTOR_LAYER_GAP": 64},
                "center": {"x": 0, "y": 0},
                "leafLabelPads": leaf_label_pads,
                "enforceSingleSector": true
            },
            "ownerCenters": [
                {"id": "center", "x": 0, "y": 0},
                {"id": "other", "x": 260, "y": 120}
            ],
            "strictCenterOwnership": true,
            "maxLeafRadius": 360
        });

        let strict_contract = project_network_sector_placement(&payload);
        assert_eq!(
            strict_contract["contextSummary"]["ownerCenterCount"],
            json!(2)
        );
        assert_eq!(
            strict_contract["contextSummary"]["strictCenterOwnership"],
            json!(true)
        );
        assert!(strict_contract["fastRingPlacement"].is_null());

        let mut loose_payload = payload.clone();
        if let Some(row) = loose_payload.as_object_mut() {
            row.insert("strictCenterOwnership".to_string(), Value::Bool(false));
        }
        let loose_contract = project_network_sector_placement(&loose_payload);
        assert_eq!(
            loose_contract["contextSummary"]["strictCenterOwnership"],
            json!(false)
        );
        assert_eq!(
            loose_contract["fastRingPlacement"]["nodeUpdateCount"],
            json!(40)
        );
    }
}
