use crate::flow_layout_network_model::{NetworkBucketSectorPlan, NetworkLeafSector};
use crate::flow_layout_network_sector_ring::{
    clamp_f64, layer_span_scale_for_count, plan_sector_ring_layers_with_arc_gap,
};
use serde_json::{json, Map, Value};
use std::cmp::Ordering;
use std::f64::consts::PI;

pub(crate) const TWO_PI: f64 = PI * 2.0;

pub(crate) struct NetworkSectorLeafPlan {
    pub(crate) node_updates: Vec<NetworkSectorNodeUpdate>,
    pub(crate) center_report: NetworkSectorCenterReport,
}

#[derive(Clone, Debug)]
pub(crate) struct NetworkSectorNodeUpdate {
    id: String,
    x: f64,
    y: f64,
    layout_band: String,
    layout_case: String,
    layout_layer: Option<usize>,
    owner_center_id: Option<String>,
}

impl NetworkSectorNodeUpdate {
    pub(crate) fn new(
        id: impl Into<String>,
        x: f64,
        y: f64,
        layout_band: impl Into<String>,
        layout_case: impl Into<String>,
    ) -> Self {
        Self {
            id: id.into(),
            x: round_two(x),
            y: round_two(y),
            layout_band: layout_band.into(),
            layout_case: layout_case.into(),
            layout_layer: None,
            owner_center_id: None,
        }
    }

    fn leaf(id: impl Into<String>, x: f64, y: f64, layer: usize, owner_center_id: &str) -> Self {
        Self {
            id: id.into(),
            x: round_two(x),
            y: round_two(y),
            layout_band: "leaf".to_string(),
            layout_case: "single-center-sector".to_string(),
            layout_layer: Some(layer),
            owner_center_id: Some(owner_center_id.to_string()),
        }
    }

    pub(crate) fn into_json(self) -> Value {
        let mut out = Map::new();
        out.insert("id".to_string(), json!(self.id));
        out.insert("x".to_string(), number_json(self.x));
        out.insert("y".to_string(), number_json(self.y));
        out.insert("layoutBand".to_string(), json!(self.layout_band));
        out.insert("layoutCase".to_string(), json!(self.layout_case));
        if let Some(layer) = self.layout_layer {
            out.insert("layoutLayer".to_string(), json!(layer));
        }
        if let Some(owner_center_id) = self.owner_center_id {
            out.insert("ownerCenterId".to_string(), json!(owner_center_id));
        }
        Value::Object(out)
    }

    pub(crate) fn id(&self) -> &str {
        self.id.as_str()
    }

    pub(crate) fn layout_band(&self) -> &str {
        self.layout_band.as_str()
    }

    pub(crate) fn owner_center_id(&self) -> Option<&str> {
        self.owner_center_id.as_deref()
    }

    pub(crate) fn x(&self) -> f64 {
        self.x
    }

    pub(crate) fn y(&self) -> f64 {
        self.y
    }
}

#[derive(Clone, Debug)]
pub(crate) struct NetworkSectorCenterReport {
    center_id: String,
    center_type: String,
    x: f64,
    y: f64,
    leaf_count: usize,
    layer_count: usize,
    sector_ranges: Vec<NetworkLeafSector>,
    leaf_mode: String,
    sector_fast_path: Option<String>,
}

impl NetworkSectorCenterReport {
    fn new(
        center_id: &str,
        center_type: &str,
        center_x: f64,
        center_y: f64,
        leaf_count: usize,
        layer_count: usize,
        sector_ranges: Vec<NetworkLeafSector>,
        sector_fast_path: Option<&str>,
    ) -> Self {
        Self {
            center_id: center_id.to_string(),
            center_type: center_type.to_string(),
            x: round_two(center_x),
            y: round_two(center_y),
            leaf_count,
            layer_count,
            sector_ranges,
            leaf_mode: "rust-sector".to_string(),
            sector_fast_path: sector_fast_path.map(str::to_string),
        }
    }

    pub(crate) fn into_json(self) -> Value {
        let sector_count = self.sector_ranges.len();
        let mut out = Map::new();
        out.insert("centerId".to_string(), json!(self.center_id));
        out.insert("centerType".to_string(), json!(self.center_type));
        out.insert("x".to_string(), number_json(self.x));
        out.insert("y".to_string(), number_json(self.y));
        out.insert("leafCount".to_string(), json!(self.leaf_count));
        out.insert("layerCount".to_string(), json!(self.layer_count));
        out.insert("sectorCount".to_string(), json!(sector_count));
        out.insert(
            "sectorRanges".to_string(),
            Value::Array(
                self.sector_ranges
                    .into_iter()
                    .map(NetworkLeafSector::report_json)
                    .collect(),
            ),
        );
        out.insert("multiSectorUsed".to_string(), json!(sector_count > 1));
        out.insert("leafMode".to_string(), json!(self.leaf_mode));
        if let Some(sector_fast_path) = self.sector_fast_path {
            out.insert("sectorFastPath".to_string(), json!(sector_fast_path));
        }
        Value::Object(out)
    }

    pub(crate) fn center_id(&self) -> &str {
        self.center_id.as_str()
    }
}

pub(crate) struct NetworkSectorLeafPlanInput<'a> {
    pub(crate) center_id: &'a str,
    pub(crate) center_type: &'a str,
    pub(crate) center_x: f64,
    pub(crate) center_y: f64,
    pub(crate) leaf_ids: &'a [String],
    pub(crate) sector_plan: &'a NetworkBucketSectorPlan,
    pub(crate) phase_key: &'a str,
    pub(crate) jitter_key_prefix: &'a str,
    pub(crate) config: &'a Value,
}

pub(crate) fn project_core_leaf_sector(
    adjacent_angles: &[f64],
    leaf_count: usize,
) -> NetworkLeafSector {
    let preferred_span = sector_span_for_leaf_count(leaf_count);
    if adjacent_angles.is_empty() {
        return NetworkLeafSector::full_circle();
    }
    if adjacent_angles.len() == 1 {
        let center = adjacent_angles[0] + PI;
        return NetworkLeafSector::around(center, preferred_span);
    }

    let mut normalized = adjacent_angles
        .iter()
        .copied()
        .filter(|angle| angle.is_finite())
        .map(normalize_angle_positive)
        .collect::<Vec<_>>();
    normalized.sort_by(|a, b| a.partial_cmp(b).unwrap_or(Ordering::Equal));
    if normalized.is_empty() {
        return NetworkLeafSector::full_circle();
    }

    let mut best_start = normalized[0];
    let mut best_gap = 0.0;
    for index in 0..normalized.len() {
        let start = normalized[index];
        let end = if index + 1 < normalized.len() {
            normalized[index + 1]
        } else {
            normalized[0] + TWO_PI
        };
        let gap = end - start;
        if gap > best_gap {
            best_gap = gap;
            best_start = start;
        }
    }
    let guard = 0.28;
    let span = preferred_span.min((best_gap - guard).max(PI / 3.0));
    let center = best_start + best_gap / 2.0;
    NetworkLeafSector::around(center, span)
}

pub(crate) fn project_sector_leaf_plan(
    input: NetworkSectorLeafPlanInput<'_>,
) -> NetworkSectorLeafPlan {
    let center_id = input.center_id;
    let center_type = input.center_type;
    let center_x = input.center_x;
    let center_y = input.center_y;
    let leaf_ids = input.leaf_ids;
    let sector_plan = input.sector_plan;
    let sectors = sector_plan.sectors.as_slice();
    let phase_key = input.phase_key;
    let jitter_key_prefix = input.jitter_key_prefix;
    let config = input.config;
    let leaf_count = leaf_ids.len();
    if sectors.is_empty() {
        return NetworkSectorLeafPlan {
            node_updates: Vec::new(),
            center_report: NetworkSectorCenterReport::new(
                center_id,
                center_type,
                center_x,
                center_y,
                leaf_count,
                0,
                Vec::new(),
                None,
            ),
        };
    }
    let r_leaf_min = number_field(config, "R_LEAF_MIN")
        .unwrap_or(220.0)
        .max(80.0);
    let layer_gap = number_field(config, "NETWORK_SECTOR_LAYER_GAP")
        .unwrap_or(68.0)
        .max(22.0);
    let sector_count = sectors.len();
    let phase = stable_hash_unit(phase_key);
    let mut max_layer = 0usize;
    let mut node_updates = Vec::new();
    let mut cursor = 0usize;

    for (sector_index, sector) in sectors.iter().enumerate() {
        let remaining = leaf_count.saturating_sub(cursor);
        if remaining == 0 {
            break;
        }
        let sectors_left = sector_count - sector_index;
        let take = ((remaining as f64) / (sectors_left as f64)).ceil() as usize;
        let group_count = take.min(remaining).max(1);
        let sector_start = sector.start;
        let sector_span = sector.span().max(0.05);
        let nominal_arc_gap = clamp_f64(
            30.0 + sector_plan.avg_label_pad.max(0.0) * 0.24 + (group_count as f64).sqrt() * 0.95,
            30.0,
            82.0,
        );
        let layer_plan = plan_sector_ring_layers_with_arc_gap(
            group_count,
            sector_span,
            r_leaf_min,
            layer_gap,
            nominal_arc_gap,
        );
        let edge_pad = clamp_f64(sector_span * 0.018, 0.012, 0.12);
        let usable_span = (sector_span - edge_pad * 2.0).max(0.05);
        let mut local_index = 0usize;

        for (layer, layer_row) in layer_plan.iter().enumerate() {
            let count_in_layer = layer_row.count;
            if count_in_layer == 0 {
                continue;
            }
            max_layer = max_layer.max(layer);
            let layer_span = clamp_f64(
                usable_span * layer_span_scale_for_count(count_in_layer),
                0.04,
                usable_span,
            );
            let layer_start = edge_pad + (usable_span - layer_span) * 0.5;
            let layer_phase =
                (phase + stable_hash_unit(&format!("{phase_key}|{sector_index}|{layer}"))).fract();

            for pos_in_layer in 0..count_in_layer {
                if local_index >= group_count {
                    break;
                }
                let id = &leaf_ids[cursor + local_index];
                let ratio = if count_in_layer <= 1 {
                    0.5
                } else {
                    ((pos_in_layer as f64) + 0.5 + layer_phase) / (count_in_layer as f64)
                };
                let angle = sector_start + layer_start + layer_span * ratio.fract();
                let jitter = (stable_hash_unit(&format!("{jitter_key_prefix}|{id}|{layer}")) - 0.5)
                    * layer_gap
                    * 0.18;
                let radius = (layer_row.radius + jitter).max(r_leaf_min * 0.78);
                let x = center_x + angle.cos() * radius;
                let y = center_y + angle.sin() * radius;
                node_updates.push(NetworkSectorNodeUpdate::leaf(
                    id.clone(),
                    x,
                    y,
                    layer,
                    center_id,
                ));
                local_index += 1;
            }
        }
        cursor += group_count;
    }

    NetworkSectorLeafPlan {
        node_updates,
        center_report: NetworkSectorCenterReport::new(
            center_id,
            center_type,
            center_x,
            center_y,
            leaf_count,
            max_layer + 1,
            sectors.to_vec(),
            Some("rust-sector-ring"),
        ),
    }
}

pub(crate) fn sector_span_for_leaf_count(leaf_count: usize) -> f64 {
    if leaf_count >= 24 {
        PI * 1.55
    } else if leaf_count >= 8 {
        PI * 1.35
    } else {
        PI * 1.16
    }
}

fn normalize_angle_positive(value: f64) -> f64 {
    let mut out = value % TWO_PI;
    if out < 0.0 {
        out += TWO_PI;
    }
    out
}

pub(crate) fn number_field(value: &Value, key: &str) -> Option<f64> {
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

pub(crate) fn number_json(value: f64) -> Value {
    if value.fract() == 0.0 && value >= i64::MIN as f64 && value <= i64::MAX as f64 {
        json!(value as i64)
    } else {
        json!(value)
    }
}

pub(crate) fn round_two(value: f64) -> f64 {
    ((value * 100.0) + 0.5).floor() / 100.0
}

pub(crate) fn stable_hash_unit(value: &str) -> f64 {
    let mut hash = 2_166_136_261u32;
    for byte in value.as_bytes() {
        hash ^= *byte as u32;
        hash = hash.wrapping_mul(16_777_619);
    }
    hash as f64 / u32::MAX as f64
}

#[cfg(test)]
mod tests {
    use super::*;

    fn test_sector_plan(
        sectors: Vec<NetworkLeafSector>,
        leaf_count: usize,
    ) -> NetworkBucketSectorPlan {
        NetworkBucketSectorPlan {
            sectors,
            bucket_top_score: None,
            leaf_count,
            avg_label_pad: 0.0,
            dense_sector: leaf_count >= 36,
            bucket_step: TWO_PI / 36.0,
            target_arc: TWO_PI,
            min_radius_base: 180.0,
        }
    }

    fn test_sector_plan_with_label_pad(
        sectors: Vec<NetworkLeafSector>,
        leaf_count: usize,
        avg_label_pad: f64,
    ) -> NetworkBucketSectorPlan {
        NetworkBucketSectorPlan {
            avg_label_pad,
            ..test_sector_plan(sectors, leaf_count)
        }
    }

    #[test]
    fn network_leaf_sector_reports_round_bounds_and_score() {
        let sector = NetworkLeafSector {
            start: -PI,
            end: PI,
            score: Some(12.3456),
        };

        assert_eq!(
            sector.report_json(),
            json!({"start": -3.142, "end": 3.142, "score": 12.346})
        );
    }

    #[test]
    fn network_sector_leaf_plan_uses_sector_model_for_report() {
        let leaf_ids = ["leaf-a".to_string(), "leaf-b".to_string()];
        let config = json!({"R_LEAF_MIN": 180, "NETWORK_SECTOR_LAYER_GAP": 64});
        let sector_plan = test_sector_plan(vec![NetworkLeafSector::new(-1.0, 1.0)], leaf_ids.len());
        let plan = project_sector_leaf_plan(NetworkSectorLeafPlanInput {
            center_id: "center",
            center_type: "layoutAnchor",
            center_x: 10.0,
            center_y: -5.0,
            leaf_ids: &leaf_ids,
            sector_plan: &sector_plan,
            phase_key: "phase",
            jitter_key_prefix: "jitter",
            config: &config,
        });

        let center_report = plan.center_report.into_json();
        assert_eq!(
            center_report["sectorRanges"],
            json!([{"start": -1, "end": 1, "score": null}])
        );
        assert_eq!(plan.node_updates.len(), 2);
    }

    #[test]
    fn network_sector_leaf_plan_accepts_multiple_sectors() {
        let leaf_ids = [
            "leaf-a".to_string(),
            "leaf-b".to_string(),
            "leaf-c".to_string(),
            "leaf-d".to_string(),
        ];
        let sectors = [
            NetworkLeafSector::new(-1.0, -0.2).with_score(9.0),
            NetworkLeafSector::new(0.2, 1.0).with_score(8.0),
        ];
        let config = json!({"R_LEAF_MIN": 180, "NETWORK_SECTOR_LAYER_GAP": 64});
        let sector_plan = test_sector_plan(sectors.to_vec(), leaf_ids.len());
        let plan = project_sector_leaf_plan(NetworkSectorLeafPlanInput {
            center_id: "center",
            center_type: "layoutAnchor",
            center_x: 0.0,
            center_y: 0.0,
            leaf_ids: &leaf_ids,
            sector_plan: &sector_plan,
            phase_key: "phase",
            jitter_key_prefix: "jitter",
            config: &config,
        });

        assert_eq!(plan.node_updates.len(), 4);
        let center_report = plan.center_report.into_json();
        assert_eq!(center_report["sectorCount"], json!(2));
        assert_eq!(center_report["multiSectorUsed"], json!(true));
        assert_eq!(
            center_report["sectorRanges"],
            json!([
                {"start": -1, "end": -0.2, "score": 9},
                {"start": 0.2, "end": 1, "score": 8}
            ])
        );
    }

    #[test]
    fn network_sector_leaf_plan_uses_label_pad_for_layer_capacity() {
        let leaf_ids = (0..60)
            .map(|index| format!("leaf-{index}"))
            .collect::<Vec<_>>();
        let config = json!({"R_LEAF_MIN": 180, "NETWORK_SECTOR_LAYER_GAP": 64});
        let compact_plan = test_sector_plan(vec![NetworkLeafSector::full_circle()], leaf_ids.len());
        let wide_label_plan = test_sector_plan_with_label_pad(
            vec![NetworkLeafSector::full_circle()],
            leaf_ids.len(),
            80.0,
        );
        let compact = project_sector_leaf_plan(NetworkSectorLeafPlanInput {
            center_id: "center",
            center_type: "layoutAnchor",
            center_x: 0.0,
            center_y: 0.0,
            leaf_ids: &leaf_ids,
            sector_plan: &compact_plan,
            phase_key: "phase",
            jitter_key_prefix: "jitter",
            config: &config,
        });
        let wide = project_sector_leaf_plan(NetworkSectorLeafPlanInput {
            center_id: "center",
            center_type: "layoutAnchor",
            center_x: 0.0,
            center_y: 0.0,
            leaf_ids: &leaf_ids,
            sector_plan: &wide_label_plan,
            phase_key: "phase",
            jitter_key_prefix: "jitter",
            config: &config,
        });

        assert!(
            wide.center_report.clone().into_json()["layerCount"]
                .as_u64()
                .unwrap_or_default()
                >= compact.center_report.into_json()["layerCount"]
                    .as_u64()
                    .unwrap_or_default()
        );
    }
}
