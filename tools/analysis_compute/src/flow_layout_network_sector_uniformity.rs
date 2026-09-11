use crate::flow_layout_network_sector::{stable_hash_unit, TWO_PI};
use crate::flow_layout_network_sector_envelope::{
    Center, EnvelopeOptions, HardCorridor, NodeModel, Sector, Zone,
};
use crate::flow_layout_network_sector_ring::{
    clamp_f64, layer_span_scale_for_count, network_sector_layer_limit,
};
use std::cmp::Ordering;
use std::collections::{HashMap, HashSet};

pub(crate) struct UniformityResult {
    pub(crate) adjusted: usize,
    pub(crate) planned: usize,
    pub(crate) rejected: usize,
    pub(crate) sparse_layer_rebalances: usize,
    pub(crate) fallback_rescues: usize,
    pub(crate) zones: Vec<Zone>,
}

#[derive(Clone)]
struct UniformityEntry {
    zone_index: usize,
    node: NodeModel,
    rel: f64,
    radius: f64,
    id: String,
}

#[derive(Clone)]
struct Assignment {
    entry: UniformityEntry,
    count_in_layer: usize,
    target_angle: f64,
    target_radius: f64,
}

struct ScoreContext {
    external_zones: Vec<Zone>,
    draft_zones: Vec<Zone>,
}

struct OwnershipChecker {
    enabled: bool,
    own_id: String,
    own_x: f64,
    own_y: f64,
    centers: Vec<(String, f64, f64)>,
    guaranteed_own_radius_sq: f64,
}

fn resolve_network_nominal_arc_gap(
    leaf_count: usize,
    avg_label_pad: f64,
    avg_node_radius: f64,
    avg_collision: f64,
) -> f64 {
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
            + (leaf_count.max(1) as f64).sqrt() * 1.15,
        min_gap,
        max_gap,
    )
}

pub(crate) fn apply_sector_uniformity(
    center: &Center,
    sectors: &[Sector],
    mut leaf_zones: Vec<Zone>,
    occupied_zones: &[Zone],
    nodes: &HashMap<String, NodeModel>,
    options: &EnvelopeOptions,
) -> UniformityResult {
    if leaf_zones.is_empty() || sectors.is_empty() {
        return UniformityResult {
            adjusted: 0,
            planned: 0,
            rejected: 0,
            sparse_layer_rebalances: 0,
            fallback_rescues: 0,
            zones: leaf_zones,
        };
    }

    let leaf_ids = leaf_zones
        .iter()
        .map(|zone| zone.id.clone())
        .collect::<HashSet<_>>();
    let external_occupied_zones = occupied_zones
        .iter()
        .filter(|zone| zone.id.is_empty() || !leaf_ids.contains(&zone.id))
        .cloned()
        .collect::<Vec<_>>();
    let ownership = OwnershipChecker::new(
        &center.id,
        center.x,
        center.y,
        &options
            .owner_centers
            .iter()
            .map(|row| (row.id.clone(), row.x, row.y))
            .collect::<Vec<_>>(),
        options.strict_center_ownership,
    );

    let mut adjusted = 0usize;
    let mut planned = 0usize;
    let mut rejected = 0usize;
    let mut sparse_layer_rebalances = 0usize;
    let mut fallback_rescues = 0usize;

    for (sector_index, sector) in sectors.iter().enumerate() {
        let span = (sector.end - sector.start).max(0.0);
        if span <= 1e-5 {
            continue;
        }
        let mut entries = leaf_zones
            .iter()
            .enumerate()
            .filter(|(_, zone)| zone.owner_sector_index == sector_index)
            .filter_map(|(zone_index, zone)| {
                let node = nodes.get(&zone.id)?;
                let dx = zone.x - center.x;
                let dy = zone.y - center.y;
                let angle = dy.atan2(dx);
                Some(UniformityEntry {
                    zone_index,
                    node: node.clone(),
                    rel: normalize_angle_positive(angle - sector.start),
                    radius: hypot(dx, dy),
                    id: zone.id.clone(),
                })
            })
            .collect::<Vec<_>>();
        entries.sort_by(|a, b| a.rel.partial_cmp(&b.rel).unwrap_or(Ordering::Equal));
        if entries.len() < 6 {
            continue;
        }

        let gap_threshold = clamp_f64(
            0.14_f64.max(span / 3.2_f64.max((entries.len() as f64).sqrt() * 1.35)),
            0.14,
            0.78,
        );
        let mut groups: Vec<Vec<UniformityEntry>> = Vec::new();
        let mut current_group: Vec<UniformityEntry> = Vec::new();
        for (idx, entry) in entries.iter().cloned().enumerate() {
            if idx == 0 {
                current_group.push(entry);
                continue;
            }
            let prev = &entries[idx - 1];
            let gap = (entry.rel - prev.rel).max(0.0);
            if gap > gap_threshold && !current_group.is_empty() {
                groups.push(current_group);
                current_group = vec![entry];
            } else {
                current_group.push(entry);
            }
        }
        if !current_group.is_empty() {
            groups.push(current_group);
        }
        if groups.is_empty() {
            continue;
        }
        let largest_group_size = groups.iter().map(Vec::len).max().unwrap_or(0);
        let use_whole_sector_uniform =
            entries.len() >= 18 || (largest_group_size as f64) >= (entries.len() as f64) * 0.58;
        let uniform_groups = if use_whole_sector_uniform {
            vec![entries.clone()]
        } else {
            groups
        };

        for (group_index, group_rows) in uniform_groups.into_iter().enumerate() {
            if group_rows.len() < 2 {
                continue;
            }
            let group_count = group_rows.len();
            let mut radii = group_rows.iter().map(|row| row.radius).collect::<Vec<_>>();
            radii.sort_by(|a, b| a.partial_cmp(b).unwrap_or(Ordering::Equal));
            let q10 = quantile_sorted_values(&radii, 0.1);
            let q25 = quantile_sorted_values(&radii, 0.25);
            let q50 = quantile_sorted_values(&radii, 0.5);
            let q75 = quantile_sorted_values(&radii, 0.75);
            let q90 = quantile_sorted_values(&radii, 0.9);
            let iqr = (q75 - q25).max(0.0);
            let avg_label =
                group_rows.iter().map(|row| row.node.label_pad).sum::<f64>() / group_count as f64;
            let avg_node_radius =
                group_rows.iter().map(|row| row.node.radius).sum::<f64>() / group_count as f64;
            let avg_collision = group_rows
                .iter()
                .map(|row| row.node.collision_radius)
                .sum::<f64>()
                / group_count as f64;
            let nominal_arc_gap = resolve_network_nominal_arc_gap(
                group_count,
                avg_label,
                avg_node_radius,
                avg_collision,
            );
            let group_start_rel = group_rows.first().map(|row| row.rel).unwrap_or(0.0);
            let group_end_rel = group_rows
                .last()
                .map(|row| row.rel)
                .unwrap_or(group_start_rel);
            let raw_group_span = (group_end_rel - group_start_rel).max(0.05);
            let dense_group = group_count >= 24;
            let edge_pad = clamp_f64(span * 0.018, 0.012, 0.12);
            let (mut usable_start_rel, mut usable_end_rel) =
                if dense_group || raw_group_span >= span * 0.62 {
                    (edge_pad, span - edge_pad)
                } else {
                    let extra = clamp_f64(
                        raw_group_span * 0.38 + if dense_group { 0.28 } else { 0.18 },
                        0.16,
                        0.16_f64.max(span * 0.36),
                    );
                    let start = clamp_f64(
                        group_start_rel - extra,
                        edge_pad,
                        edge_pad.max(span - edge_pad - 0.06),
                    );
                    let end = clamp_f64(group_end_rel + extra, start + 0.06, span - edge_pad);
                    (start, end)
                };
            let mut usable_span = (usable_end_rel - usable_start_rel).max(0.05);
            let radius_cap = if options.max_radius_cap.is_finite() {
                (options.min_radius_floor + 20.0).max(options.max_radius_cap)
            } else {
                9_007_199_254_740_991.0
            };
            let mid_radius_for_cap = clamp_f64(
                js_or_default(q50, options.min_radius_floor + 40.0),
                options.min_radius_floor + 8.0,
                radius_cap,
            );
            let band_capacity = 2usize.max(
                ((usable_span * mid_radius_for_cap.max(options.min_radius_floor + 12.0))
                    / nominal_arc_gap)
                    .floor() as usize,
            );
            let layer_count_raw = ((group_count as f64) / band_capacity.max(1) as f64).ceil();
            let layer_count = 1usize.max(
                group_count
                    .min((layer_count_raw as usize).min(network_sector_layer_limit(group_count))),
            );
            let band_gap = clamp_f64(
                18.0_f64.max(
                    avg_label * 0.28 + avg_node_radius * 0.42 + (group_count as f64).sqrt() * 0.7,
                ),
                18.0,
                68.0,
            );
            let jitter_amp = clamp_f64(
                8.0_f64.max(iqr * 0.2 + (group_count as f64).sqrt() * 0.58),
                8.0,
                34.0,
            );
            let ideal_span = clamp_f64(
                (group_count as f64 / band_capacity.max(1) as f64) * 0.14
                    + (group_count.saturating_sub(1) as f64)
                        * (nominal_arc_gap / 140.0_f64.max(mid_radius_for_cap)),
                0.08,
                span * 0.98,
            );
            if usable_span < ideal_span {
                let center_rel = (group_start_rel + group_end_rel) * 0.5;
                let half = ideal_span * 0.5;
                usable_start_rel = clamp_f64(
                    center_rel - half,
                    edge_pad,
                    edge_pad.max(span - edge_pad - ideal_span),
                );
                usable_end_rel = clamp_f64(
                    usable_start_rel + ideal_span,
                    usable_start_rel + 0.05,
                    span - edge_pad,
                );
                usable_span = (usable_end_rel - usable_start_rel).max(0.05);
            }

            let local_cap = if options.max_radius_cap.is_finite() {
                options.max_radius_cap
            } else {
                q50 + 120.0_f64.max(band_gap * 9.0)
            };
            let spread = clamp_f64(
                (band_gap * layer_count.saturating_sub(1).max(1) as f64 + 16.0)
                    .max(iqr + band_gap * 1.15),
                22.0,
                24.0_f64.max(local_cap - options.min_radius_floor),
            );
            let radius_center_base = clamp_f64(
                js_or_default(q50, options.min_radius_floor + spread * 0.5),
                options.min_radius_floor + spread * 0.5,
                local_cap - spread * 0.5,
            );
            let mut radius_low = clamp_f64(
                options.min_radius_floor.max(q10 - 16.0_f64.max(iqr * 0.45)),
                options.min_radius_floor,
                local_cap - 8.0,
            );
            let mut radius_high = clamp_f64(
                (radius_low + 8.0).max(q90 + 20.0_f64.max(iqr * 0.66)),
                radius_low + 8.0,
                local_cap,
            );
            let spread_needed = layer_count.saturating_sub(1) as f64 * 10.0_f64.max(band_gap * 0.9);
            let current_spread = (radius_high - radius_low).max(0.0);
            if current_spread < spread_needed {
                let missing = spread_needed - current_spread;
                let can_grow_up = (local_cap - radius_high).max(0.0);
                let grow_up = can_grow_up.min(missing * 0.75);
                let can_grow_down = (radius_low - options.min_radius_floor).max(0.0);
                let grow_down = can_grow_down.min((missing - grow_up).max(0.0));
                radius_high += grow_up;
                radius_low -= grow_down;
            }
            let radius_center = clamp_f64(
                radius_center_base,
                radius_low + 8.0_f64.min((radius_high - radius_low) * 0.28),
                radius_high - 8.0_f64.min((radius_high - radius_low) * 0.28),
            );
            let span_radius = (radius_high - radius_low).max(8.0);
            let mut layer_radii = Vec::new();
            for li in 0..layer_count {
                if layer_count <= 1 {
                    layer_radii.push(radius_center);
                } else {
                    let t = li as f64 / (layer_count - 1).max(1) as f64;
                    layer_radii.push(clamp_f64(
                        radius_low + span_radius * t,
                        radius_low,
                        radius_high,
                    ));
                }
            }
            let layer_caps = layer_radii
                .iter()
                .map(|rr| {
                    2usize.max(
                        ((usable_span * rr.max(options.min_radius_floor + 12.0)) / nominal_arc_gap)
                            .floor() as usize,
                    )
                })
                .collect::<Vec<_>>();
            let mut allocated = allocate_layer_counts(group_count, &layer_caps);
            let min_layer_nodes = if group_count >= 120 {
                7
            } else if group_count >= 90 {
                6
            } else if group_count >= 56 {
                5
            } else if group_count >= 28 {
                4
            } else {
                3
            };
            let sparse_before = allocated
                .iter()
                .filter(|value| **value > 0 && **value < min_layer_nodes)
                .count();
            for _ in 0..2048 {
                let sparse = allocated
                    .iter()
                    .enumerate()
                    .filter(|(_, value)| **value > 0 && **value < min_layer_nodes)
                    .min_by(|a, b| a.1.cmp(b.1).then_with(|| a.0.cmp(&b.0)))
                    .map(|(idx, _)| idx);
                let Some(sparse_idx) = sparse else {
                    break;
                };
                let donor = allocated
                    .iter()
                    .enumerate()
                    .filter(|(_, value)| **value > min_layer_nodes + 1)
                    .max_by(|a, b| a.1.cmp(b.1).then_with(|| b.0.cmp(&a.0)))
                    .map(|(idx, _)| idx);
                let Some(donor_idx) = donor else {
                    break;
                };
                allocated[donor_idx] -= 1;
                allocated[sparse_idx] += 1;
                sparse_layer_rebalances += 1;
            }
            let sparse_after = allocated
                .iter()
                .filter(|value| **value > 0 && **value < min_layer_nodes)
                .count();
            if sparse_after > sparse_before {
                sparse_layer_rebalances += 1;
            }

            let mut ordered_entries = group_rows.clone();
            ordered_entries.sort_by(|a, b| {
                if (a.rel - b.rel).abs() > 1e-9 {
                    return a.rel.partial_cmp(&b.rel).unwrap_or(Ordering::Equal);
                }
                a.id.cmp(&b.id)
            });
            let mut assignments = Vec::new();
            let mut entry_cursor = 0usize;
            for (li, count_raw) in allocated.iter().enumerate() {
                if entry_cursor >= ordered_entries.len() {
                    break;
                }
                let count = *count_raw;
                if count == 0 {
                    continue;
                }
                let phase = stable_hash_unit(&format!(
                    "sector-uniform-phase|{}|{}|{}|{}",
                    center.id, sector_index, group_index, li
                ));
                let layer_span = clamp_f64(
                    layer_span_scale_for_count(count) * usable_span,
                    0.04,
                    usable_span,
                );
                let layer_start_rel = clamp_f64(
                    usable_start_rel + (usable_span - layer_span) * 0.5,
                    usable_start_rel,
                    usable_end_rel - layer_span,
                );
                for i in 0..count {
                    if entry_cursor >= ordered_entries.len() {
                        break;
                    }
                    let entry = ordered_entries[entry_cursor].clone();
                    let ratio = if count <= 1 {
                        0.5
                    } else {
                        ((i as f64 + 0.5 + phase) % count as f64) / count as f64
                    };
                    let target_rel = layer_start_rel + layer_span * ratio;
                    let target_angle =
                        clamp_angle_to_arc(sector.start + target_rel, sector.start, span);
                    let radial_noise =
                        (stable_hash_unit(&format!("sector-uniform-r|{}|{}", center.id, entry.id))
                            - 0.5)
                            * jitter_amp
                            * 0.65;
                    let target_radius =
                        clamp_f64(layer_radii[li] + radial_noise, radius_low, radius_high);
                    assignments.push(Assignment {
                        entry,
                        count_in_layer: count,
                        target_angle,
                        target_radius,
                    });
                    entry_cursor += 1;
                }
            }
            while entry_cursor < ordered_entries.len() {
                let entry = ordered_entries[entry_cursor].clone();
                let li = (entry_cursor % layer_radii.len().max(1))
                    .min(layer_radii.len().saturating_sub(1));
                let ratio = if ordered_entries.len() <= 1 {
                    0.5
                } else {
                    (entry_cursor as f64 + 0.5) / ordered_entries.len() as f64
                };
                let target_rel = usable_start_rel + usable_span * ratio;
                let target_angle =
                    clamp_angle_to_arc(sector.start + target_rel, sector.start, span);
                let target_radius = clamp_f64(layer_radii[li], radius_low, radius_high);
                assignments.push(Assignment {
                    entry,
                    count_in_layer: 1usize.max(*allocated.get(li).unwrap_or(&1)),
                    target_angle,
                    target_radius,
                });
                entry_cursor += 1;
            }

            let ignore_ids = group_rows
                .iter()
                .map(|row| row.id.clone())
                .collect::<HashSet<_>>();
            let visible_zones = external_occupied_zones
                .iter()
                .cloned()
                .chain(leaf_zones.iter().cloned())
                .collect::<Vec<_>>();
            let external_zones = visible_zones
                .into_iter()
                .filter(|zone| !ignore_ids.contains(&zone.id))
                .collect::<Vec<_>>();
            let mut scoring_ctx = ScoreContext {
                external_zones,
                draft_zones: Vec::new(),
            };

            for item in assignments {
                planned += 1;
                let entry = item.entry;
                let zone_index = entry.zone_index;
                let zone = leaf_zones.get(zone_index).cloned();
                let Some(zone) = zone else {
                    continue;
                };
                let target_angle = item.target_angle;
                let target_radius = item.target_radius;
                let count_in_layer = item.count_in_layer.max(1);
                let local_angle_step = clamp_f64(
                    usable_span / 14.0_f64.max(count_in_layer as f64 * 1.25),
                    0.006,
                    0.075,
                );
                let local_radial_step = clamp_f64(
                    6.0_f64.max(span_radius / 4.0_f64.max(layer_count as f64 * 1.5)),
                    6.0,
                    24.0,
                );
                let local_angle_offsets = [
                    0.0,
                    local_angle_step,
                    -local_angle_step,
                    local_angle_step * 2.0,
                    -local_angle_step * 2.0,
                    local_angle_step * 3.0,
                    -local_angle_step * 3.0,
                    local_angle_step * 4.0,
                    -local_angle_step * 4.0,
                    local_angle_step * 5.0,
                    -local_angle_step * 5.0,
                ];
                let local_radial_offsets = [
                    0.0,
                    -local_radial_step,
                    local_radial_step,
                    -local_radial_step * 2.0,
                    local_radial_step * 2.0,
                    -local_radial_step * 3.0,
                    local_radial_step * 3.0,
                    -local_radial_step * 4.0,
                    local_radial_step * 4.0,
                ];
                let current_x = zone.x;
                let current_y = zone.y;
                let current_angle = (current_y - center.y).atan2(current_x - center.x);
                let current_radius = hypot(current_x - center.x, current_y - center.y);
                let current_penalty = score_candidate(
                    center,
                    current_x,
                    current_y,
                    &entry.node,
                    sector,
                    target_angle,
                    target_radius,
                    &scoring_ctx,
                    options,
                    &ownership,
                );
                let mut best: Option<(f64, f64, f64, f64)> = None;
                let mut best_penalty = f64::INFINITY;
                let mut used_fallback = false;
                for rr_shift in local_radial_offsets {
                    let rr = clamp_f64(target_radius + rr_shift, radius_low, radius_high);
                    for aa_shift in local_angle_offsets {
                        let angle = clamp_angle_to_arc(target_angle + aa_shift, sector.start, span);
                        let x = center.x + angle.cos() * rr;
                        let y = center.y + angle.sin() * rr;
                        let penalty = score_candidate(
                            center,
                            x,
                            y,
                            &entry.node,
                            sector,
                            target_angle,
                            target_radius,
                            &scoring_ctx,
                            options,
                            &ownership,
                        );
                        if penalty >= best_penalty {
                            continue;
                        }
                        best_penalty = penalty;
                        best = Some((x, y, angle, rr));
                        if penalty <= 0.08 {
                            break;
                        }
                    }
                    if best_penalty <= 0.08 {
                        break;
                    }
                }
                if best.is_none() || !best_penalty.is_finite() {
                    let fallback_phase = stable_hash_unit(&format!(
                        "sector-uniform-fallback|{}|{}|{}|{}",
                        center.id, sector_index, group_index, zone.id
                    ));
                    let sweep_count = 64usize.max(360usize.min(72usize.max(
                        (usable_span / 0.01_f64.max(local_angle_step) * 1.6).round() as usize,
                    )));
                    let radial_stride = clamp_f64(8.0_f64.max(local_radial_step * 0.9), 8.0, 36.0);
                    let mut radial_candidates = Vec::new();
                    for ri in 0..=14 {
                        let inward = clamp_f64(
                            target_radius - ri as f64 * radial_stride,
                            radius_low,
                            radius_high,
                        );
                        radial_candidates.push(inward);
                        if ri > 0 {
                            let outward = clamp_f64(
                                target_radius + ri as f64 * radial_stride,
                                radius_low,
                                radius_high,
                            );
                            radial_candidates.push(outward);
                        }
                    }
                    let mut seen_radius = HashSet::new();
                    let unique_radials = radial_candidates
                        .into_iter()
                        .filter(|rr| seen_radius.insert(format!("{rr:.2}")))
                        .collect::<Vec<_>>();
                    let mut fallback_best: Option<(f64, f64, f64, f64)> = None;
                    let mut fallback_penalty = f64::INFINITY;
                    for rr in unique_radials {
                        for si in 0..sweep_count {
                            let ratio = ((si as f64 + fallback_phase) % sweep_count as f64)
                                / sweep_count as f64;
                            let target_rel = usable_start_rel + usable_span * ratio;
                            let angle =
                                clamp_angle_to_arc(sector.start + target_rel, sector.start, span);
                            let x = center.x + angle.cos() * rr;
                            let y = center.y + angle.sin() * rr;
                            let penalty = score_candidate(
                                center,
                                x,
                                y,
                                &entry.node,
                                sector,
                                target_angle,
                                target_radius,
                                &scoring_ctx,
                                options,
                                &ownership,
                            );
                            if penalty >= fallback_penalty {
                                continue;
                            }
                            fallback_penalty = penalty;
                            fallback_best = Some((x, y, angle, rr));
                            if penalty <= 0.08 {
                                break;
                            }
                        }
                        if fallback_penalty <= 0.08 {
                            break;
                        }
                    }
                    if fallback_best.is_some() && fallback_penalty.is_finite() {
                        best = fallback_best;
                        best_penalty = fallback_penalty;
                        used_fallback = true;
                        fallback_rescues += 1;
                    }
                }
                let Some((best_x, best_y, _best_angle, _best_radius)) = best else {
                    rejected += 1;
                    scoring_ctx.draft_zones.push(Zone {
                        id: zone.id,
                        x: current_x,
                        y: current_y,
                        r: if zone.r != 0.0 {
                            zone.r
                        } else {
                            entry.node.collision_radius
                        },
                        owner_center_id: String::new(),
                        owner_sector_index: 0,
                    });
                    continue;
                };
                if !best_penalty.is_finite() {
                    rejected += 1;
                    scoring_ctx.draft_zones.push(Zone {
                        id: zone.id,
                        x: current_x,
                        y: current_y,
                        r: if zone.r != 0.0 {
                            zone.r
                        } else {
                            entry.node.collision_radius
                        },
                        owner_center_id: String::new(),
                        owner_sector_index: 0,
                    });
                    continue;
                }
                let angular_drift = angle_diff(current_angle, target_angle);
                let radial_drift = (current_radius - target_radius).abs();
                let should_apply = used_fallback
                    || !current_penalty.is_finite()
                    || best_penalty + 0.06 < current_penalty
                    || angular_drift > local_angle_step * 0.65
                    || radial_drift > local_radial_step * 0.65;
                if should_apply {
                    if let Some(target_zone) = leaf_zones.get_mut(zone_index) {
                        target_zone.x = js_or_default(best_x, 0.0);
                        target_zone.y = js_or_default(best_y, 0.0);
                    }
                    adjusted += 1;
                }
                if let Some(target_zone) = leaf_zones.get(zone_index) {
                    scoring_ctx.draft_zones.push(Zone {
                        id: target_zone.id.clone(),
                        x: target_zone.x,
                        y: target_zone.y,
                        r: if target_zone.r != 0.0 {
                            target_zone.r
                        } else {
                            entry.node.collision_radius
                        },
                        owner_center_id: String::new(),
                        owner_sector_index: 0,
                    });
                }
            }
        }
    }

    UniformityResult {
        adjusted,
        planned,
        rejected,
        sparse_layer_rebalances,
        fallback_rescues,
        zones: leaf_zones,
    }
}

fn score_candidate(
    center: &Center,
    x: f64,
    y: f64,
    node: &NodeModel,
    sector: &Sector,
    target_angle: f64,
    target_radius: f64,
    scoring_ctx: &ScoreContext,
    options: &EnvelopeOptions,
    ownership: &OwnershipChecker,
) -> f64 {
    if !ownership.owns(x, y) {
        return f64::INFINITY;
    }
    let angle = (y - center.y).atan2(x - center.x);
    let radius = hypot(x - center.x, y - center.y);
    let span = (sector.end - sector.start).max(0.0);
    if !is_angle_inside_sector(angle, sector.start, span, 0.01) {
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
    if collides_with_corridors(x, y, node, &options.hard_corridors, 5.0) {
        return f64::INFINITY;
    }
    let overlap_penalty = overlap_penalty(x, y, node.collision_radius, scoring_ctx);
    let ray_threshold = 20.0_f64.max(node.label_pad * 1.42);
    let ray_penalty = ray_penalty_with_draft(center, x, y, scoring_ctx, 0.085, ray_threshold);
    let anchor_penalty =
        angle_diff(angle, target_angle) * 16.0_f64.max(target_radius * 0.08) * 0.85
            + (radius - target_radius).abs() * 0.11;
    overlap_penalty * 2.2 + ray_penalty * 0.9 + anchor_penalty
}

fn overlap_penalty(x: f64, y: f64, radius: f64, scoring_ctx: &ScoreContext) -> f64 {
    scoring_ctx
        .external_zones
        .iter()
        .chain(scoring_ctx.draft_zones.iter())
        .fold(0.0, |sum, zone| {
            let dist = hypot(zone.x - x, zone.y - y);
            let threshold = radius + zone.r + 8.0;
            if dist < threshold {
                sum + threshold - dist
            } else {
                sum
            }
        })
}

fn ray_penalty_with_draft(
    center: &Center,
    x: f64,
    y: f64,
    scoring_ctx: &ScoreContext,
    angle_threshold: f64,
    radial_threshold: f64,
) -> f64 {
    let at = clamp_f64(js_or_default(angle_threshold, 0.085), 0.02, 0.24);
    let rt = 12.0_f64.max(js_or_default(radial_threshold, 34.0));
    let target_angle = (y - center.y).atan2(x - center.x);
    let target_radius = hypot(x - center.x, y - center.y);
    scoring_ctx
        .external_zones
        .iter()
        .chain(scoring_ctx.draft_zones.iter())
        .fold(0.0, |sum, zone| {
            let angle = (zone.y - center.y).atan2(zone.x - center.x);
            let radius = hypot(zone.x - center.x, zone.y - center.y);
            let ad = angle_diff(target_angle, angle);
            if ad >= at {
                return sum;
            }
            let rd = (target_radius - radius).abs();
            let angle_weight = (at - ad) / 1e-6_f64.max(at);
            let radial_weight = if rd < rt {
                1.85
            } else if rd < rt * 2.1 {
                1.12
            } else {
                0.55
            };
            sum + angle_weight * radial_weight * 22.0
        })
}

fn allocate_layer_counts(total_count: usize, layer_caps: &[usize]) -> Vec<usize> {
    let caps = layer_caps
        .iter()
        .copied()
        .filter(|value| *value > 0)
        .collect::<Vec<_>>();
    if total_count == 0 || caps.is_empty() {
        return Vec::new();
    }
    let total_cap = caps.iter().sum::<usize>().max(1) as f64;
    let target = caps
        .iter()
        .map(|value| (*value as f64 / total_cap) * total_count as f64)
        .collect::<Vec<_>>();
    let mut allocated = target
        .iter()
        .map(|value| value.floor().max(0.0) as usize)
        .collect::<Vec<_>>();
    let mut allocated_total = allocated.iter().sum::<usize>();
    if allocated_total < total_count {
        let mut residues = target
            .iter()
            .enumerate()
            .map(|(idx, value)| (idx, value - value.floor()))
            .collect::<Vec<_>>();
        residues.sort_by(|a, b| {
            b.1.partial_cmp(&a.1)
                .unwrap_or(Ordering::Equal)
                .then_with(|| a.0.cmp(&b.0))
        });
        let mut cursor = 0usize;
        while allocated_total < total_count && !residues.is_empty() {
            let idx = residues[cursor % residues.len()].0;
            allocated[idx] += 1;
            allocated_total += 1;
            cursor += 1;
        }
    } else if allocated_total > total_count {
        let mut order = allocated
            .iter()
            .enumerate()
            .map(|(idx, value)| (idx, *value))
            .collect::<Vec<_>>();
        order.sort_by(|a, b| b.1.cmp(&a.1).then_with(|| a.0.cmp(&b.0)));
        let mut cursor = 0usize;
        while allocated_total > total_count && !order.is_empty() {
            let pick = order[cursor % order.len()].0;
            if allocated[pick] > 0 {
                allocated[pick] -= 1;
                allocated_total -= 1;
            }
            cursor += 1;
            if cursor > total_count + order.len() + 8 {
                break;
            }
        }
    }
    allocated
}

impl OwnershipChecker {
    fn new(
        center_id: &str,
        center_x: f64,
        center_y: f64,
        centers: &[(String, f64, f64)],
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
                if row.0 != own_id {
                    continue;
                }
                for other in centers {
                    if other.0 == own_id {
                        continue;
                    }
                    let dist = hypot(other.1 - center_x, other.2 - center_y);
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

fn nearest_center_id_at(x: f64, y: f64, centers: &[(String, f64, f64)]) -> String {
    let mut nearest_id = String::new();
    let mut nearest_dist_sq = f64::INFINITY;
    for row in centers {
        if row.0.is_empty() {
            continue;
        }
        let dx = row.1 - x;
        let dy = row.2 - y;
        let dist_sq = dx * dx + dy * dy;
        if !dist_sq.is_finite() {
            continue;
        }
        if dist_sq < nearest_dist_sq {
            nearest_dist_sq = dist_sq;
            nearest_id = row.0.clone();
        }
    }
    nearest_id
}

fn collides_with_zones(x: f64, y: f64, radius: f64, zones: &[Zone], padding: f64) -> bool {
    zones.iter().any(|zone| {
        let threshold = radius + zone.r + padding.max(0.0);
        let dx = zone.x - x;
        let dy = zone.y - y;
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
    corridors.iter().any(|row| {
        let dist = distance_point_to_segment(x, y, row.x1, row.y1, row.x2, row.y2);
        let radius = node.radius + 8.0_f64.max(node.label_pad * 0.34) + padding.max(0.0);
        let threshold = 6.0_f64.max(row.half_width) + radius;
        dist < threshold
    })
}

fn is_angle_inside_sector(angle: f64, start: f64, span: f64, tolerance: f64) -> bool {
    if span >= TWO_PI - 1e-6 {
        return true;
    }
    let rel = normalize_angle_positive(angle - start);
    rel >= -tolerance && rel <= span + tolerance
}

fn is_angle_in_ranges(
    angle: f64,
    ranges: &[crate::flow_layout_network_sector_envelope::AngleRange],
) -> bool {
    if ranges.is_empty() {
        return true;
    }
    let value = normalize_angle_positive(angle);
    for row in ranges {
        let start = normalize_angle_positive(row.start);
        let end = normalize_angle_positive(row.end);
        if row.span.unwrap_or(0.0) >= TWO_PI - 1e-3 {
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

fn clamp_angle_to_arc(angle: f64, start: f64, span: f64) -> f64 {
    let arc_span = clamp_f64(span, 0.0, TWO_PI);
    if arc_span >= TWO_PI - 1e-6 {
        return js_or_default(angle, 0.0);
    }
    let rel = normalize_angle_positive(js_or_default(angle, 0.0) - start);
    start + clamp_f64(rel, 0.0, arc_span)
}

fn quantile_sorted_values(values: &[f64], q: f64) -> f64 {
    if values.is_empty() {
        return 0.0;
    }
    if values.len() == 1 {
        return js_or_default(values[0], 0.0);
    }
    let pos = clamp_f64(q, 0.0, 1.0) * (values.len() - 1) as f64;
    let low = pos.floor() as usize;
    let high = pos.ceil() as usize;
    if low == high {
        return js_or_default(values[low], 0.0);
    }
    let weight = pos - low as f64;
    values[low] * (1.0 - weight) + values[high] * weight
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

fn hypot(dx: f64, dy: f64) -> f64 {
    (dx * dx + dy * dy).sqrt()
}

fn js_or_default(value: f64, default_value: f64) -> f64 {
    if value != 0.0 && !value.is_nan() {
        value
    } else {
        default_value
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::flow_layout_network_sector_envelope::OwnerCenter;

    #[test]
    fn network_sector_uniformity_skips_tiny_groups() {
        let center = Center {
            id: "center".to_string(),
            x: 0.0,
            y: 0.0,
        };
        let sectors = vec![Sector {
            start: -1.0,
            end: 1.0,
        }];
        let leaf_zones = (0..5)
            .map(|idx| Zone {
                id: format!("leaf-{idx}"),
                x: 180.0 + idx as f64 * 8.0,
                y: idx as f64 * 4.0,
                r: 24.0,
                owner_center_id: "center".to_string(),
                owner_sector_index: 0,
            })
            .collect::<Vec<_>>();
        let nodes = leaf_zones
            .iter()
            .map(|zone| {
                (
                    zone.id.clone(),
                    NodeModel {
                        radius: 18.0,
                        collision_radius: 24.0,
                        label_pad: 28.0,
                    },
                )
            })
            .collect::<HashMap<_, _>>();
        let options = EnvelopeOptions {
            hard_zones: Vec::new(),
            hard_corridors: Vec::new(),
            territory_ranges: Vec::new(),
            allow_territory_overflow: false,
            owner_centers: vec![OwnerCenter {
                id: "center".to_string(),
                x: 0.0,
                y: 0.0,
            }],
            strict_center_ownership: true,
            min_radius_floor: 120.0,
            max_radius_cap: 360.0,
        };
        let result = apply_sector_uniformity(
            &center,
            &sectors,
            leaf_zones.clone(),
            &leaf_zones,
            &nodes,
            &options,
        );
        assert_eq!(result.adjusted, 0);
        assert_eq!(result.planned, 0);
        assert_eq!(result.zones.len(), leaf_zones.len());
    }
}
