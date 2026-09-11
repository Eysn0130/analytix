use std::cmp::Ordering;

pub(crate) struct SectorRingLayer {
    pub(crate) radius: f64,
    pub(crate) count: usize,
}

pub(crate) fn plan_sector_ring_layers_with_arc_gap(
    leaf_count: usize,
    sector_span: f64,
    r_leaf_min: f64,
    layer_gap: f64,
    nominal_arc_gap: f64,
) -> Vec<SectorRingLayer> {
    if leaf_count == 0 {
        return Vec::new();
    }
    let span = sector_span.max(0.05);
    let nominal_arc_gap = clamp_f64(nominal_arc_gap, 18.0, 96.0);
    let mut radii = Vec::new();
    let mut caps = Vec::new();
    let mut capacity_total = 0usize;
    for layer in 0..14usize {
        let radius = r_leaf_min + (layer as f64) * layer_gap;
        let cap = ((span * radius) / nominal_arc_gap).floor().max(2.0) as usize;
        radii.push(radius);
        caps.push(cap);
        capacity_total += cap;
        if capacity_total >= leaf_count {
            break;
        }
    }
    let counts = allocate_layer_counts(leaf_count, &caps);
    radii
        .into_iter()
        .zip(counts)
        .filter(|(_, count)| *count > 0)
        .map(|(radius, count)| SectorRingLayer { radius, count })
        .collect()
}

pub(crate) fn plan_sector_fast_ring_layers(
    leaf_count: usize,
    sector_span: f64,
    radius_low: f64,
    radius_high: f64,
    nominal_arc_gap: f64,
) -> Vec<SectorRingLayer> {
    if leaf_count == 0 {
        return Vec::new();
    }
    let span = sector_span.max(0.05);
    let low = radius_low.max(1.0);
    let high = radius_high.max(low + 16.0);
    let arc_gap = nominal_arc_gap.max(1.0);
    let mid_radius = (low + high) * 0.5;
    let mid_capacity = ((span * mid_radius.max(low + 12.0)) / arc_gap)
        .floor()
        .max(2.0) as usize;
    let layer_limit = network_sector_layer_limit(leaf_count);
    let layer_count = ((leaf_count as f64) / (mid_capacity.max(1) as f64))
        .ceil()
        .max(1.0)
        .min(layer_limit as f64) as usize;
    let mut radii = Vec::with_capacity(layer_count);
    let mut caps = Vec::with_capacity(layer_count);
    for layer in 0..layer_count {
        let t = if layer_count <= 1 {
            0.5
        } else {
            layer as f64 / (layer_count.saturating_sub(1) as f64)
        };
        let radius = clamp_f64(low + (high - low) * t, low, high);
        let cap = ((span * radius.max(low + 12.0)) / arc_gap).floor().max(2.0) as usize;
        radii.push(radius);
        caps.push(cap);
    }
    let counts = allocate_layer_counts(leaf_count, &caps);
    radii
        .into_iter()
        .zip(counts)
        .filter(|(_, count)| *count > 0)
        .map(|(radius, count)| SectorRingLayer { radius, count })
        .collect()
}

pub(crate) fn network_sector_layer_limit(leaf_count: usize) -> usize {
    if leaf_count >= 260 {
        24
    } else if leaf_count >= 180 {
        21
    } else if leaf_count >= 120 {
        18
    } else if leaf_count >= 80 {
        16
    } else {
        14
    }
}

pub(crate) fn layer_span_scale_for_count(count: usize) -> f64 {
    match count {
        16.. => 0.94,
        12..=15 => 0.9,
        10..=11 => 0.86,
        9 => 0.82,
        8 => 0.78,
        7 => 0.74,
        6 => 0.68,
        5 => 0.62,
        4 => 0.54,
        3 => 0.44,
        2 => 0.34,
        _ => 0.24,
    }
}

pub(crate) fn clamp_f64(value: f64, min: f64, max: f64) -> f64 {
    value.max(min).min(max)
}

pub(crate) fn project_network_sector_ring_contract(
    payload: &serde_json::Value,
) -> serde_json::Value {
    let leaf_count = number_field(payload, "leafCount")
        .or_else(|| number_field(payload, "leaf_count"))
        .unwrap_or(0.0)
        .max(0.0) as usize;
    let sector_span = number_field(payload, "sectorSpan")
        .or_else(|| number_field(payload, "sector_span"))
        .unwrap_or(std::f64::consts::PI * 2.0);
    let radius_low = number_field(payload, "radiusLow")
        .or_else(|| number_field(payload, "radius_low"))
        .unwrap_or_else(|| {
            number_field(payload, "rLeafMin")
                .or_else(|| number_field(payload, "r_leaf_min"))
                .unwrap_or(220.0)
        });
    let radius_high = number_field(payload, "radiusHigh")
        .or_else(|| number_field(payload, "radius_high"))
        .unwrap_or(radius_low);
    let nominal_arc_gap = number_field(payload, "nominalArcGap")
        .or_else(|| number_field(payload, "nominal_arc_gap"))
        .unwrap_or(42.0);
    let layers = plan_sector_fast_ring_layers(
        leaf_count,
        sector_span,
        radius_low,
        radius_high,
        nominal_arc_gap,
    );

    serde_json::json!({
        "leafCount": leaf_count,
        "layerCount": layers.len(),
        "totalCount": layers.iter().map(|row| row.count).sum::<usize>(),
        "layers": layers.iter().map(|row| {
            serde_json::json!({
                "radius": row.radius,
                "count": row.count,
                "spanScale": layer_span_scale_for_count(row.count),
            })
        }).collect::<Vec<_>>(),
    })
}

fn number_field(value: &serde_json::Value, key: &str) -> Option<f64> {
    match value.get(key) {
        Some(serde_json::Value::Number(number)) => {
            number.as_f64().filter(|value| value.is_finite())
        }
        Some(serde_json::Value::String(text)) => text
            .trim()
            .parse::<f64>()
            .ok()
            .filter(|value| value.is_finite()),
        _ => None,
    }
}

fn allocate_layer_counts(total_count: usize, layer_caps: &[usize]) -> Vec<usize> {
    if total_count == 0 || layer_caps.is_empty() {
        return Vec::new();
    }
    let total_cap = layer_caps.iter().sum::<usize>().max(1) as f64;
    let targets = layer_caps
        .iter()
        .map(|cap| (*cap as f64 / total_cap) * total_count as f64)
        .collect::<Vec<_>>();
    let mut allocated = targets
        .iter()
        .map(|value| value.floor().max(0.0) as usize)
        .collect::<Vec<_>>();
    let mut allocated_total = allocated.iter().sum::<usize>();
    if allocated_total < total_count {
        let mut residues = targets
            .iter()
            .enumerate()
            .map(|(idx, value)| (idx, value - value.floor()))
            .collect::<Vec<_>>();
        residues.sort_by(|(left_idx, left), (right_idx, right)| {
            right
                .partial_cmp(left)
                .unwrap_or(Ordering::Equal)
                .then_with(|| left_idx.cmp(right_idx))
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
            .copied()
            .enumerate()
            .collect::<Vec<(usize, usize)>>();
        order.sort_by(|(left_idx, left), (right_idx, right)| {
            right.cmp(left).then_with(|| left_idx.cmp(right_idx))
        });
        let mut cursor = 0usize;
        while allocated_total > total_count && !order.is_empty() {
            let idx = order[cursor % order.len()].0;
            if allocated[idx] > 0 {
                allocated[idx] -= 1;
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

#[cfg(test)]
mod tests {
    use super::*;
    use std::f64::consts::PI;

    #[test]
    fn network_sector_ring_layers_allocate_all_leaves() {
        let layers = plan_sector_ring_layers_with_arc_gap(
            120,
            PI * 2.0,
            180.0,
            64.0,
            clamp_f64(30.0 + 120.0_f64.sqrt() * 0.95, 30.0, 82.0),
        );

        assert_eq!(layers.iter().map(|row| row.count).sum::<usize>(), 120);
        assert_eq!(layers.len(), 4);
        assert!(layers
            .windows(2)
            .all(|rows| rows[0].radius < rows[1].radius));
    }

    #[test]
    fn network_sector_ring_layers_keep_small_groups_compact() {
        let layers = plan_sector_ring_layers_with_arc_gap(
            4,
            PI * 2.0,
            180.0,
            64.0,
            clamp_f64(30.0 + 4.0_f64.sqrt() * 0.95, 30.0, 82.0),
        );

        assert_eq!(layers.iter().map(|row| row.count).sum::<usize>(), 4);
        assert_eq!(layers.len(), 1);
    }

    #[test]
    fn network_sector_fast_ring_layers_match_network_runtime_shape() {
        let layers = plan_sector_fast_ring_layers(60, PI * 2.0, 142.0, 282.0, 42.0);

        assert_eq!(layers.iter().map(|row| row.count).sum::<usize>(), 60);
        assert_eq!(layers.len(), 2);
        assert!(layers[0].radius < layers[1].radius);
    }

    #[test]
    fn network_sector_layer_span_scale_matches_dense_thresholds() {
        assert_eq!(layer_span_scale_for_count(16), 0.94);
        assert_eq!(layer_span_scale_for_count(8), 0.78);
        assert_eq!(layer_span_scale_for_count(1), 0.24);
    }
}
