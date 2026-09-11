use crate::flow_layout_network_node_metrics::project_node_metrics;
use crate::flow_layout_network_sector::TWO_PI;
use crate::flow_layout_network_sector_ring::clamp_f64;
use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Value};
use std::cmp::Ordering;
use std::collections::HashMap;
use std::{fs, path::PathBuf};

pub(crate) struct NetworkSectorEnvelopeContractArgs {
    pub(crate) payload: Value,
}

#[derive(Clone)]
pub(crate) struct Center {
    pub(crate) id: String,
    pub(crate) x: f64,
    pub(crate) y: f64,
}

#[derive(Clone, Copy)]
pub(crate) struct Sector {
    pub(crate) start: f64,
    pub(crate) end: f64,
}

#[derive(Clone)]
pub(crate) struct Zone {
    pub(crate) id: String,
    pub(crate) x: f64,
    pub(crate) y: f64,
    pub(crate) r: f64,
    pub(crate) owner_center_id: String,
    pub(crate) owner_sector_index: usize,
}

#[derive(Clone)]
pub(crate) struct HardCorridor {
    pub(crate) x1: f64,
    pub(crate) y1: f64,
    pub(crate) x2: f64,
    pub(crate) y2: f64,
    pub(crate) half_width: f64,
}

#[derive(Clone)]
pub(crate) struct AngleRange {
    pub(crate) start: f64,
    pub(crate) end: f64,
    pub(crate) span: Option<f64>,
}

#[derive(Clone)]
pub(crate) struct OwnerCenter {
    pub(crate) id: String,
    pub(crate) x: f64,
    pub(crate) y: f64,
}

#[derive(Clone)]
pub(crate) struct NodeModel {
    pub(crate) radius: f64,
    pub(crate) collision_radius: f64,
    pub(crate) label_pad: f64,
}

pub(crate) struct EnvelopeOptions {
    pub(crate) hard_zones: Vec<Zone>,
    pub(crate) hard_corridors: Vec<HardCorridor>,
    pub(crate) territory_ranges: Vec<AngleRange>,
    pub(crate) allow_territory_overflow: bool,
    pub(crate) owner_centers: Vec<OwnerCenter>,
    pub(crate) strict_center_ownership: bool,
    pub(crate) min_radius_floor: f64,
    pub(crate) max_radius_cap: f64,
}

pub(crate) struct EnvelopeResult {
    pub(crate) adjusted: usize,
    pub(crate) pulled_by_radius: usize,
    pub(crate) pulled_by_angle: usize,
    pub(crate) zones: Vec<Zone>,
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<NetworkSectorEnvelopeContractArgs> {
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
                        "read network sector envelope contract input {}",
                        input_path.display()
                    )
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!(
                        "parse network sector envelope contract input {}",
                        input_path.display()
                    )
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute contract-flow-layout-network-sector-envelope <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported network sector envelope contract flag: {other}"),
        }
    }

    Ok(NetworkSectorEnvelopeContractArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_network_sector_envelope_contract(payload: &Value) -> Value {
    let center = read_center(payload.get("center"));
    let sectors = read_sectors(payload.get("sectors"));
    let leaf_zones = read_zones(payload.get("leafZones"));
    let occupied_zones = if payload
        .get("occupiedZones")
        .and_then(Value::as_array)
        .is_some()
    {
        read_zones(payload.get("occupiedZones"))
    } else {
        leaf_zones.clone()
    };
    let nodes = read_nodes(payload.get("nodes"));
    let min_radius_floor = js_or_default(number_field(payload, "minRadiusFloor"), 0.0).max(0.0);
    let max_radius_cap = match number_field(payload, "maxRadiusCap") {
        Some(value) if value.is_finite() => value.max(min_radius_floor + 16.0),
        _ => f64::INFINITY,
    };
    let options = EnvelopeOptions {
        hard_zones: read_zones(payload.get("hardZones")),
        hard_corridors: read_hard_corridors(payload.get("hardCorridors")),
        territory_ranges: read_angle_ranges(payload.get("territoryRanges")),
        allow_territory_overflow: truthy_field(payload, "allowTerritoryOverflow"),
        owner_centers: read_owner_centers(payload.get("ownerCenters")),
        strict_center_ownership: !matches!(
            payload.get("strictCenterOwnership"),
            Some(Value::Bool(false))
        ),
        min_radius_floor,
        max_radius_cap,
    };
    let result = apply_sector_envelope(
        &center,
        &sectors,
        leaf_zones,
        &occupied_zones,
        &nodes,
        &options,
    );

    json!({
        "adjusted": result.adjusted,
        "pulledByRadius": result.pulled_by_radius,
        "pulledByAngle": result.pulled_by_angle,
        "zones": result.zones.iter().map(zone_json).collect::<Vec<_>>(),
    })
}

pub(crate) fn apply_sector_envelope(
    center: &Center,
    sectors: &[Sector],
    mut leaf_zones: Vec<Zone>,
    occupied_zones: &[Zone],
    nodes: &HashMap<String, NodeModel>,
    options: &EnvelopeOptions,
) -> EnvelopeResult {
    if leaf_zones.is_empty() || sectors.is_empty() {
        return EnvelopeResult {
            adjusted: 0,
            pulled_by_radius: 0,
            pulled_by_angle: 0,
            zones: leaf_zones,
        };
    }

    let mut adjusted = 0usize;
    let mut pulled_by_radius = 0usize;
    let mut pulled_by_angle = 0usize;
    let visible_zones = if center.id.is_empty() {
        occupied_zones.to_vec()
    } else {
        occupied_zones
            .iter()
            .filter(|zone| zone.owner_center_id.is_empty() || zone.owner_center_id == center.id)
            .cloned()
            .collect::<Vec<_>>()
    };
    let ownership = OwnershipChecker::new(
        &center.id,
        center.x,
        center.y,
        &options.owner_centers,
        options.strict_center_ownership,
    );

    for (sector_index, sector) in sectors.iter().enumerate() {
        let span = (sector.end - sector.start).max(0.0);
        if span <= 1e-5 {
            continue;
        }
        let entries = leaf_zones
            .iter()
            .enumerate()
            .filter(|(_, zone)| zone.owner_sector_index == sector_index)
            .filter_map(|(zone_index, zone)| {
                let node = nodes.get(&zone.id)?;
                let dx = zone.x - center.x;
                let dy = zone.y - center.y;
                Some((
                    zone_index,
                    node.clone(),
                    hypot(dx, dy),
                    dy.atan2(dx),
                    zone.id.clone(),
                ))
            })
            .collect::<Vec<_>>();
        if entries.len() < 4 {
            continue;
        }
        let mut radii = entries.iter().map(|row| row.2).collect::<Vec<_>>();
        radii.sort_by(|a, b| a.partial_cmp(b).unwrap_or(Ordering::Equal));
        let q1 = quantile_sorted_values(&radii, 0.25);
        let q3 = quantile_sorted_values(&radii, 0.75);
        let q90 = quantile_sorted_values(&radii, 0.9);
        let iqr = (q3 - q1).max(0.0);
        let radius_lower = options.min_radius_floor.max(q1 - 20.0_f64.max(iqr * 1.1));
        let radius_upper = options
            .max_radius_cap
            .min((q90 + 6.0).max(q3 + 24.0_f64.max(iqr * 1.3)));
        let safe_upper = (radius_lower + 16.0).max(radius_upper);
        let angle_step = clamp_f64(span / (entries.len() as f64 * 0.72).max(28.0), 0.012, 0.08);
        let radial_step = clamp_f64((safe_upper - radius_lower) / 9.0, 8.0, 26.0);
        let mut angle_offsets = vec![0.0];
        for idx in 1..=7 {
            angle_offsets.push((idx as f64) * angle_step);
            angle_offsets.push(-(idx as f64) * angle_step);
        }
        let mut radial_offsets = vec![0.0];
        for idx in 1..=6 {
            radial_offsets.push(-(idx as f64) * radial_step);
            radial_offsets.push((idx as f64) * radial_step);
        }

        for (zone_index, node, current_radius, current_angle, zone_id) in entries {
            let out_by_angle = !is_angle_inside_sector(current_angle, sector.start, span, 0.012);
            let out_by_radius =
                current_radius > safe_upper + 4.0 || current_radius < radius_lower - 4.0;
            if !out_by_angle && !out_by_radius {
                continue;
            }

            let base_angle = clamp_angle_to_arc(current_angle, sector.start, span);
            let base_radius = clamp_f64(current_radius, radius_lower, safe_upper);
            let current_zone = leaf_zones[zone_index].clone();
            let current_penalty = score_candidate(
                center,
                current_zone.x,
                current_zone.y,
                &node,
                &current_zone,
                sector,
                &visible_zones,
                options,
                &ownership,
            );
            let mut best: Option<(f64, f64)> = None;
            let mut best_penalty = f64::INFINITY;

            for rr_shift in &radial_offsets {
                let rr = clamp_f64(base_radius + *rr_shift, radius_lower, safe_upper);
                for aa_shift in &angle_offsets {
                    let angle = clamp_angle_to_arc(base_angle + *aa_shift, sector.start, span);
                    let x = center.x + angle.cos() * rr;
                    let y = center.y + angle.sin() * rr;
                    let penalty = score_candidate(
                        center,
                        x,
                        y,
                        &node,
                        &current_zone,
                        sector,
                        &visible_zones,
                        options,
                        &ownership,
                    );
                    if penalty >= best_penalty {
                        continue;
                    }
                    best_penalty = penalty;
                    best = Some((x, y));
                    if penalty <= 0.08 {
                        break;
                    }
                }
                if best_penalty <= 0.08 {
                    break;
                }
            }

            let Some((x, y)) = best else {
                continue;
            };
            if !best_penalty.is_finite() {
                continue;
            }
            let should_apply = !current_penalty.is_finite()
                || best_penalty + 0.05 < current_penalty
                || out_by_angle
                || out_by_radius;
            if !should_apply {
                continue;
            }
            if let Some(zone) = leaf_zones.get_mut(zone_index) {
                zone.x = js_or_default(Some(x), 0.0);
                zone.y = js_or_default(Some(y), 0.0);
            }
            let _ = zone_id;
            adjusted += 1;
            if out_by_radius {
                pulled_by_radius += 1;
            }
            if out_by_angle {
                pulled_by_angle += 1;
            }
        }
    }

    EnvelopeResult {
        adjusted,
        pulled_by_radius,
        pulled_by_angle,
        zones: leaf_zones,
    }
}

fn score_candidate(
    center: &Center,
    x: f64,
    y: f64,
    node: &NodeModel,
    zone: &Zone,
    sector: &Sector,
    visible_zones: &[Zone],
    options: &EnvelopeOptions,
    ownership: &OwnershipChecker,
) -> f64 {
    if !ownership.owns(x, y) {
        return f64::INFINITY;
    }
    let angle = (y - center.y).atan2(x - center.x);
    let span = (sector.end - sector.start).max(0.0);
    if !is_angle_inside_sector(angle, sector.start, span, 0.012) {
        return f64::INFINITY;
    }
    if !options.allow_territory_overflow
        && !options.territory_ranges.is_empty()
        && !is_angle_in_ranges(angle, &options.territory_ranges)
    {
        return f64::INFINITY;
    }
    if collides_with_zones(x, y, node.collision_radius, &options.hard_zones, 10.0) {
        return f64::INFINITY;
    }
    if collides_with_corridors(x, y, node, &options.hard_corridors, 10.0) {
        return f64::INFINITY;
    }
    let overlap_penalty =
        overlap_penalty_excluding_self(x, y, node.collision_radius, &zone.id, visible_zones);
    let ray_penalty = ray_cluster_penalty(
        center,
        x,
        y,
        visible_zones,
        0.09,
        (node.label_pad * 1.5).max(22.0),
    );
    overlap_penalty * 7.2 + ray_penalty
}

fn overlap_penalty_excluding_self(
    x: f64,
    y: f64,
    radius: f64,
    ignore_id: &str,
    zones: &[Zone],
) -> f64 {
    let mut penalty = 0.0;
    for zone in zones {
        if zone.id == ignore_id {
            continue;
        }
        let dx = zone.x - x;
        let dy = zone.y - y;
        let dist = hypot(dx, dy);
        let threshold = radius + zone.r + 8.0;
        if dist < threshold {
            penalty += threshold - dist;
        }
    }
    penalty
}

fn collides_with_zones(x: f64, y: f64, radius: f64, zones: &[Zone], padding: f64) -> bool {
    zones.iter().any(|zone| {
        let dx = zone.x - x;
        let dy = zone.y - y;
        let threshold = radius + zone.r + padding.max(0.0);
        dx * dx + dy * dy < threshold * threshold
    })
}

fn collides_with_corridors(
    x: f64,
    y: f64,
    node: &NodeModel,
    corridors: &[HardCorridor],
    padding: f64,
) -> bool {
    if corridors.is_empty() {
        return false;
    }
    let radius = node.radius + (node.label_pad * 0.34).max(8.0) + padding.max(0.0);
    corridors.iter().any(|corridor| {
        let dist =
            distance_point_to_segment(x, y, corridor.x1, corridor.y1, corridor.x2, corridor.y2);
        let threshold = corridor.half_width.max(6.0) + radius;
        dist < threshold
    })
}

fn ray_cluster_penalty(
    center: &Center,
    x: f64,
    y: f64,
    zones: &[Zone],
    angle_threshold: f64,
    radial_threshold: f64,
) -> f64 {
    let target_angle = (y - center.y).atan2(x - center.x);
    let target_radius = hypot(x - center.x, y - center.y);
    let angle_threshold = clamp_f64(js_or_default(Some(angle_threshold), 0.08), 0.02, 0.24);
    let radial_threshold = js_or_default(Some(radial_threshold), 34.0).max(12.0);
    let mut penalty = 0.0;
    for zone in zones {
        if !center.id.is_empty()
            && !zone.owner_center_id.is_empty()
            && zone.owner_center_id != center.id
        {
            continue;
        }
        let angle = (zone.y - center.y).atan2(zone.x - center.x);
        let radius = hypot(zone.x - center.x, zone.y - center.y);
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
    }
    penalty
}

fn quantile_sorted_values(rows: &[f64], q: f64) -> f64 {
    if rows.is_empty() {
        return 0.0;
    }
    let qq = clamp_f64(js_or_default(Some(q), 0.0), 0.0, 1.0);
    let idx = (rows.len().saturating_sub(1) as f64) * qq;
    let lo = idx.floor() as usize;
    let hi = idx.ceil() as usize;
    let lv = js_or_default(rows.get(lo).copied(), 0.0);
    let hv = js_or_default(rows.get(hi).copied(), lv);
    if hi <= lo {
        return lv;
    }
    lv + (hv - lv) * (idx - lo as f64)
}

fn is_angle_inside_sector(angle: f64, start: f64, span: f64, tolerance: f64) -> bool {
    let arc_span = clamp_f64(js_or_default(Some(span), 0.0), 0.0, TWO_PI);
    if arc_span >= TWO_PI - 1e-6 {
        return true;
    }
    let clamped = clamp_angle_to_arc(angle, start, arc_span);
    angle_diff(angle, clamped) <= tolerance.max(1e-6)
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

fn read_center(value: Option<&Value>) -> Center {
    let row = value.unwrap_or(&Value::Null);
    Center {
        id: text_field(row, "id").unwrap_or_default(),
        x: js_or_default(number_field(row, "x"), 0.0),
        y: js_or_default(number_field(row, "y"), 0.0),
    }
}

fn read_sectors(value: Option<&Value>) -> Vec<Sector> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .filter(|row| !row.is_null())
                .map(|row| Sector {
                    start: js_or_default(number_field(row, "start"), 0.0),
                    end: js_or_default(number_field(row, "end"), 0.0),
                })
                .collect()
        })
        .unwrap_or_default()
}

fn read_zones(value: Option<&Value>) -> Vec<Zone> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .filter(|row| !row.is_null())
                .map(|row| Zone {
                    id: text_field(row, "id").unwrap_or_default(),
                    x: js_or_default(number_field(row, "x"), 0.0),
                    y: js_or_default(number_field(row, "y"), 0.0),
                    r: js_or_default(number_field(row, "r"), 0.0),
                    owner_center_id: text_field(row, "ownerCenterId").unwrap_or_default(),
                    owner_sector_index: js_or_default(number_field(row, "ownerSectorIndex"), 0.0)
                        .max(0.0) as usize,
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

fn read_owner_centers(value: Option<&Value>) -> Vec<OwnerCenter> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .filter_map(|row| {
                    let id = text_field(row, "id").unwrap_or_default();
                    if id.is_empty() {
                        return None;
                    }
                    Some(OwnerCenter {
                        id,
                        x: js_or_default(number_field(row, "x"), 0.0),
                        y: js_or_default(number_field(row, "y"), 0.0),
                    })
                })
                .collect()
        })
        .unwrap_or_default()
}

fn read_nodes(value: Option<&Value>) -> HashMap<String, NodeModel> {
    let mut out = HashMap::new();
    if let Some(rows) = value.and_then(Value::as_array) {
        for row in rows {
            let id = text_field(row, "id").unwrap_or_default();
            if id.is_empty() {
                continue;
            }
            let metrics = project_node_metrics(Some(row));
            out.insert(
                id,
                NodeModel {
                    radius: metrics.radius,
                    collision_radius: metrics.collision_radius,
                    label_pad: metrics.label,
                },
            );
        }
    }
    out
}

pub(crate) fn zone_json(zone: &Zone) -> Value {
    json!({
        "id": zone.id,
        "x": zone.x,
        "y": zone.y,
        "r": zone.r,
        "ownerCenterId": zone.owner_center_id,
        "ownerSectorIndex": zone.owner_sector_index,
    })
}

fn hypot(dx: f64, dy: f64) -> f64 {
    (dx * dx + dy * dy).sqrt()
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

struct OwnershipChecker {
    enabled: bool,
    own_id: String,
    own_x: f64,
    own_y: f64,
    centers: Vec<OwnerCenter>,
    guaranteed_own_radius_sq: f64,
}

impl OwnershipChecker {
    fn new(
        center_id: &str,
        center_x: f64,
        center_y: f64,
        centers: &[OwnerCenter],
        strict: bool,
    ) -> Self {
        if !strict || centers.is_empty() {
            return Self {
                enabled: false,
                own_id: String::new(),
                own_x: center_x,
                own_y: center_y,
                centers: Vec::new(),
                guaranteed_own_radius_sq: f64::INFINITY,
            };
        }
        let own_id = center_id.to_string();
        let mut min_other_dist = f64::INFINITY;
        if !own_id.is_empty() {
            for row in centers {
                if row.id != own_id {
                    continue;
                }
                for other in centers {
                    if other.id == own_id {
                        continue;
                    }
                    let dist = hypot(other.x - center_x, other.y - center_y);
                    if dist.is_finite() && dist > 1e-6 {
                        min_other_dist = min_other_dist.min(dist);
                    }
                }
                break;
            }
        }
        let guaranteed_own_radius_sq = if min_other_dist.is_finite() {
            (min_other_dist * 0.5 - 1e-6).max(0.0).powi(2)
        } else {
            f64::INFINITY
        };
        Self {
            enabled: true,
            own_id,
            own_x: center_x,
            own_y: center_y,
            centers: centers.to_vec(),
            guaranteed_own_radius_sq,
        }
    }

    fn owns(&self, x: f64, y: f64) -> bool {
        if !self.enabled {
            return true;
        }
        if self.own_id.is_empty() {
            return nearest_center_id_at(x, y, &self.centers).is_empty();
        }
        let dx = x - self.own_x;
        let dy = y - self.own_y;
        if dx * dx + dy * dy <= self.guaranteed_own_radius_sq {
            return true;
        }
        nearest_center_id_at(x, y, &self.centers) == self.own_id
    }
}

fn nearest_center_id_at(x: f64, y: f64, centers: &[OwnerCenter]) -> String {
    let mut nearest_id = String::new();
    let mut nearest_dist_sq = f64::INFINITY;
    for row in centers {
        if row.id.is_empty() {
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
            nearest_id = row.id.clone();
        }
    }
    nearest_id
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
    fn network_sector_envelope_pulls_radius_and_angle_outliers() {
        let contract = project_network_sector_envelope_contract(&json!({
            "center": {"id": "center", "x": 0, "y": 0},
            "sectors": [{"start": -0.8, "end": 0.8}],
            "nodes": [
                {"id": "a", "title": "Alpha", "r": 18},
                {"id": "b", "title": "Beta", "r": 18},
                {"id": "c", "title": "Gamma", "r": 18},
                {"id": "d", "title": "Delta", "r": 18},
                {"id": "e", "title": "Epsilon", "r": 18}
            ],
            "leafZones": [
                {"id": "a", "x": 120, "y": -20, "r": 54, "ownerCenterId": "center", "ownerSectorIndex": 0},
                {"id": "b", "x": 140, "y": 6, "r": 54, "ownerCenterId": "center", "ownerSectorIndex": 0},
                {"id": "c", "x": 160, "y": 30, "r": 54, "ownerCenterId": "center", "ownerSectorIndex": 0},
                {"id": "d", "x": 210, "y": 48, "r": 54, "ownerCenterId": "center", "ownerSectorIndex": 0},
                {"id": "e", "x": -280, "y": 180, "r": 54, "ownerCenterId": "center", "ownerSectorIndex": 0}
            ],
            "minRadiusFloor": 90,
            "maxRadiusCap": 260
        }));

        assert!(contract["adjusted"].as_u64().unwrap_or(0) > 0);
        assert!(contract["pulledByAngle"].as_u64().unwrap_or(0) > 0);
        assert_eq!(contract["zones"].as_array().map(Vec::len), Some(5));
    }
}
