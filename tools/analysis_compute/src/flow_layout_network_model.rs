use serde_json::{json, Value};
use std::f64::consts::PI;

#[derive(Clone, Debug)]
pub(crate) struct NetworkOwnerCenter {
    pub(crate) id: String,
    pub(crate) x: f64,
    pub(crate) y: f64,
}

#[derive(Clone, Debug)]
pub(crate) struct NetworkAngleRange {
    pub(crate) start: f64,
    pub(crate) end: f64,
    pub(crate) span: f64,
}

impl NetworkAngleRange {
    pub(crate) fn full() -> Self {
        Self {
            start: -std::f64::consts::PI,
            end: std::f64::consts::PI,
            span: std::f64::consts::PI * 2.0,
        }
    }

    pub(crate) fn is_full(&self) -> bool {
        self.span >= std::f64::consts::PI * 2.0 - 1e-3
    }
}

#[derive(Clone, Debug)]
pub(crate) struct NetworkCircleZone {
    pub(crate) x: f64,
    pub(crate) y: f64,
    pub(crate) r: f64,
}

#[derive(Clone, Debug)]
pub(crate) struct NetworkClusterBox {
    pub(crate) min_x: f64,
    pub(crate) min_y: f64,
    pub(crate) max_x: f64,
    pub(crate) max_y: f64,
    pub(crate) weight: f64,
}

#[derive(Clone, Debug)]
pub(crate) struct NetworkHardCorridor {
    pub(crate) x1: f64,
    pub(crate) y1: f64,
    pub(crate) x2: f64,
    pub(crate) y2: f64,
    pub(crate) half_width: f64,
}

#[derive(Clone, Debug)]
pub(crate) struct NetworkBucketAngleRange {
    pub(crate) start: f64,
    pub(crate) end: f64,
    pub(crate) span: Option<f64>,
}

#[derive(Clone, Debug)]
pub(crate) struct NetworkPreferredAngle {
    pub(crate) angle: f64,
    pub(crate) width: f64,
    pub(crate) weight: f64,
}

#[derive(Clone, Debug)]
pub(crate) struct NetworkPlacementContext {
    pub(crate) config: serde_json::Value,
    pub(crate) bucket_count: usize,
    pub(crate) r_leaf_min: f64,
    pub(crate) center_x: f64,
    pub(crate) center_y: f64,
    pub(crate) local_zones: Vec<NetworkCircleZone>,
    pub(crate) label_zones: Vec<NetworkCircleZone>,
    pub(crate) global_leaf_zones: Vec<NetworkCircleZone>,
    pub(crate) cluster_boxes: Vec<NetworkClusterBox>,
    pub(crate) safe_bounds: serde_json::Value,
    pub(crate) hard_zones: Vec<NetworkCircleZone>,
    pub(crate) hard_corridors: Vec<NetworkHardCorridor>,
    pub(crate) territory_ranges: Vec<NetworkBucketAngleRange>,
    pub(crate) preferred_angles: Vec<NetworkPreferredAngle>,
    pub(crate) allow_territory_overflow: bool,
    pub(crate) prefer_ring_mode: bool,
    pub(crate) enforce_single_sector: bool,
    pub(crate) preferred_multi_sector_count: usize,
    pub(crate) multi_sector_gap: f64,
    pub(crate) leaf_label_pads: Vec<f64>,
}

impl NetworkPlacementContext {
    pub(crate) fn from_payload(payload: &Value) -> Self {
        let config = payload.get("config").unwrap_or(payload);
        let config_value = config.clone();
        let bucket_count =
            js_or_default(number_field(config, "ANGULAR_BUCKETS"), 36.0).max(12.0) as usize;
        let r_leaf_min = js_or_default(number_field(config, "R_LEAF_MIN"), 220.0).max(80.0);
        let center = payload.get("center").unwrap_or(payload);
        let center_x = js_or_default(number_field(center, "x"), 0.0);
        let center_y = js_or_default(number_field(center, "y"), 0.0);
        let local_map = payload.get("localMap").unwrap_or(&Value::Null);
        let global_map = payload.get("globalMap").unwrap_or(&Value::Null);
        let preferred_multi_sector_count =
            js_or_default(number_field(payload, "preferredMultiSectorCount"), 1.0)
                .floor()
                .max(1.0)
                .min(5.0) as usize;

        Self {
            config: config_value,
            bucket_count,
            r_leaf_min,
            center_x,
            center_y,
            local_zones: read_circle_zones(local_map.get("zones")),
            label_zones: read_circle_zones(local_map.get("labelZones")),
            global_leaf_zones: read_circle_zones(global_map.get("leafZones")),
            cluster_boxes: read_cluster_boxes(global_map.get("clusterBBoxes")),
            safe_bounds: global_map.get("safeBounds").cloned().unwrap_or(Value::Null),
            hard_zones: read_circle_zones(payload.get("hardZones")),
            hard_corridors: read_hard_corridors(payload.get("hardCorridors")),
            territory_ranges: read_angle_ranges(payload.get("territoryRanges")),
            preferred_angles: read_preferred_angles(payload.get("preferredAngles")),
            allow_territory_overflow: truthy_field(payload, "allowTerritoryOverflow"),
            prefer_ring_mode: truthy_field(payload, "preferRingMode"),
            enforce_single_sector: !matches!(
                payload.get("enforceSingleSector"),
                Some(Value::Bool(false))
            ),
            preferred_multi_sector_count,
            multi_sector_gap: clamp_layout(
                js_or_default(number_field(payload, "multiSectorGap"), 0.22),
                0.08,
                PI / 2.0,
            ),
            leaf_label_pads: read_number_array(payload.get("leafLabelPads")),
        }
    }
}

#[derive(Clone, Debug)]
pub(crate) struct NetworkPlacementCenter {
    pub(crate) id: String,
    pub(crate) center_type: String,
    pub(crate) x: f64,
    pub(crate) y: f64,
    pub(crate) node_radius: f64,
    pub(crate) collision_radius: f64,
}

#[derive(Clone, Debug)]
pub(crate) struct NetworkPlacementLeafRow {
    pub(crate) id: String,
    pub(crate) label_pad: f64,
    pub(crate) node_radius: f64,
    pub(crate) collision_radius: f64,
}

#[derive(Clone, Debug)]
pub(crate) struct NetworkSectorPlacementInput {
    pub(crate) config: Value,
    pub(crate) center: NetworkPlacementCenter,
    pub(crate) leaf_rows: Vec<NetworkPlacementLeafRow>,
    pub(crate) bucket_context: NetworkPlacementContext,
    pub(crate) general_batch_start_index: usize,
    pub(crate) general_batch_layer: usize,
    pub(crate) general_batch_sector_index: usize,
    pub(crate) include_general_batches: bool,
    pub(crate) include_general_updates: bool,
    pub(crate) occupied_zones: Vec<NetworkCircleZone>,
    pub(crate) hard_zones: Vec<NetworkCircleZone>,
    pub(crate) hard_corridors: Vec<NetworkHardCorridor>,
    pub(crate) territory_ranges: Vec<NetworkBucketAngleRange>,
    pub(crate) preferred_angles: Vec<NetworkPreferredAngle>,
    pub(crate) owner_centers: Vec<NetworkOwnerCenter>,
    pub(crate) strict_center_ownership: bool,
    pub(crate) allow_territory_overflow: bool,
    pub(crate) max_leaf_radius: Option<f64>,
}

impl NetworkSectorPlacementInput {
    pub(crate) fn from_payload(payload: &Value) -> Self {
        let bucket_payload = payload
            .get("bucketPayload")
            .or_else(|| {
                payload
                    .get("bucketProjection")
                    .and_then(|row| row.get("payload"))
            })
            .unwrap_or(payload);
        let bucket_context = NetworkPlacementContext::from_payload(bucket_payload);
        let config = payload
            .get("config")
            .or_else(|| bucket_payload.get("config"))
            .cloned()
            .unwrap_or_else(|| bucket_context.config.clone());
        let center = read_placement_center(payload.get("center"));
        let general_batch = payload.get("generalBatch").unwrap_or(&Value::Null);
        let general_batch_start_index = finite_usize_field(general_batch, "startIndex")
            .or_else(|| finite_usize_field(payload, "generalBatchStartIndex"))
            .unwrap_or(0);
        let general_batch_layer = finite_usize_field(general_batch, "layer")
            .or_else(|| finite_usize_field(payload, "generalBatchLayer"))
            .unwrap_or(0);
        let general_batch_sector_index = finite_usize_field(general_batch, "sectorIndex")
            .or_else(|| finite_usize_field(payload, "generalBatchSectorIndex"))
            .unwrap_or(0);

        Self {
            config,
            center,
            leaf_rows: read_placement_leaf_rows(payload.get("leafRows")),
            bucket_context,
            general_batch_start_index,
            general_batch_layer,
            general_batch_sector_index,
            include_general_batches: !matches!(
                payload.get("includeGeneralBatches"),
                Some(Value::Bool(false))
            ),
            include_general_updates: !matches!(
                payload.get("includeGeneralUpdates"),
                Some(Value::Bool(false))
            ),
            occupied_zones: read_circle_zones(payload.get("occupiedZones")),
            hard_zones: read_circle_zones(payload.get("hardZones")),
            hard_corridors: read_hard_corridors(payload.get("hardCorridors")),
            territory_ranges: read_angle_ranges(payload.get("territoryRanges")),
            preferred_angles: read_preferred_angles(payload.get("preferredAngles")),
            owner_centers: read_owner_centers(payload.get("ownerCenters")),
            strict_center_ownership: !matches!(
                payload.get("strictCenterOwnership"),
                Some(Value::Bool(false))
            ),
            allow_territory_overflow: truthy_field(payload, "allowTerritoryOverflow"),
            max_leaf_radius: finite_number_field(payload, "maxLeafRadius"),
        }
    }
}

#[derive(Clone, Copy, Debug)]
pub(crate) struct NetworkLeafSector {
    pub(crate) start: f64,
    pub(crate) end: f64,
    pub(crate) score: Option<f64>,
}

impl NetworkLeafSector {
    pub(crate) fn new(start: f64, end: f64) -> Self {
        Self {
            start,
            end,
            score: None,
        }
    }

    pub(crate) fn full_circle() -> Self {
        Self::new(-PI, PI)
    }

    pub(crate) fn around(center: f64, span: f64) -> Self {
        Self::new(center - span / 2.0, center + span / 2.0)
    }

    pub(crate) fn with_bounds(self, start: f64, end: f64) -> Self {
        Self {
            start,
            end,
            score: self.score,
        }
    }

    pub(crate) fn with_score(self, score: f64) -> Self {
        Self {
            start: self.start,
            end: self.end,
            score: Some(score),
        }
    }

    pub(crate) fn span(self) -> f64 {
        self.end - self.start
    }

    pub(crate) fn report_json(self) -> Value {
        json!({
            "start": number_json(round_three(self.start)),
            "end": number_json(round_three(self.end)),
            "score": self.score.map(|score| number_json(round_three(score))).unwrap_or(Value::Null),
        })
    }
}

pub(crate) struct NetworkBucketSectorPlan {
    pub(crate) sectors: Vec<NetworkLeafSector>,
    pub(crate) bucket_top_score: Option<f64>,
    pub(crate) leaf_count: usize,
    pub(crate) avg_label_pad: f64,
    pub(crate) dense_sector: bool,
    pub(crate) bucket_step: f64,
    pub(crate) target_arc: f64,
    pub(crate) min_radius_base: f64,
}

pub(crate) struct NetworkBucketSectorProjection {
    pub(crate) bucket_count: usize,
    pub(crate) sector_plan: NetworkBucketSectorPlan,
}

pub(crate) struct NetworkBucketProjection {
    pub(crate) buckets: Vec<Value>,
    pub(crate) sector_plan: NetworkBucketSectorPlan,
}

impl NetworkBucketSectorProjection {
    pub(crate) fn contract_json(&self) -> Value {
        json!({
            "bucketCount": self.bucket_count,
            "sectorPlan": self.sector_plan.contract_json(),
        })
    }
}

impl NetworkBucketSectorPlan {
    pub(crate) fn from_static_sectors(
        sectors: Vec<NetworkLeafSector>,
        leaf_label_pads: &[f64],
        config: &Value,
    ) -> Self {
        let leaf_count = leaf_label_pads.len();
        let configured_bucket_count =
            js_or_default(number_field(config, "ANGULAR_BUCKETS"), 36.0).max(12.0) as usize;
        let bucket_step = (PI * 2.0) / (configured_bucket_count as f64);
        let min_radius_base = js_or_default(number_field(config, "R_LEAF_MIN"), 220.0).max(80.0);
        let avg_label_pad = leaf_label_pads
            .iter()
            .copied()
            .filter(|value| value.is_finite())
            .sum::<f64>()
            / (leaf_count.max(1) as f64);
        let target_arc = sectors
            .iter()
            .map(|sector| (sector.end - sector.start).max(0.0))
            .reduce(f64::max)
            .unwrap_or(0.0);
        let bucket_top_score = sectors
            .iter()
            .filter_map(|sector| sector.score)
            .reduce(f64::max)
            .map(round_three);

        Self {
            sectors,
            bucket_top_score,
            leaf_count,
            avg_label_pad,
            dense_sector: leaf_count >= 36,
            bucket_step,
            target_arc,
            min_radius_base,
        }
    }

    pub(crate) fn contract_json(&self) -> Value {
        json!({
            "sectors": self.sectors.iter().map(bucket_sector_json).collect::<Vec<_>>(),
            "bucketTopScore": self.bucket_top_score.map_or(Value::Null, |value| json!(value)),
            "leafCount": self.leaf_count,
            "avgLabelPad": self.avg_label_pad,
            "denseSector": self.dense_sector,
            "bucketStep": self.bucket_step,
            "targetArc": self.target_arc,
            "minRadiusBase": self.min_radius_base,
        })
    }
}

impl NetworkBucketProjection {
    pub(crate) fn contract_json(&self) -> Value {
        json!({
            "bucketCount": self.buckets.len(),
            "buckets": self.buckets,
            "sectorPlan": self.sector_plan.contract_json(),
        })
    }
}

fn bucket_sector_json(sector: &NetworkLeafSector) -> Value {
    json!({
        "start": sector.start,
        "end": sector.end,
        "score": sector.score.unwrap_or(0.0),
    })
}

fn number_json(value: f64) -> Value {
    if value.fract() == 0.0 && value >= i64::MIN as f64 && value <= i64::MAX as f64 {
        json!(value as i64)
    } else {
        json!(value)
    }
}

fn round_three(value: f64) -> f64 {
    ((value * 1000.0) + 0.5).floor() / 1000.0
}

fn read_circle_zones(value: Option<&Value>) -> Vec<NetworkCircleZone> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .map(|row| NetworkCircleZone {
                    x: js_or_default(number_field(row, "x"), 0.0),
                    y: js_or_default(number_field(row, "y"), 0.0),
                    r: js_or_default(number_field(row, "r"), 0.0),
                })
                .collect()
        })
        .unwrap_or_default()
}

fn read_cluster_boxes(value: Option<&Value>) -> Vec<NetworkClusterBox> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .map(|row| NetworkClusterBox {
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

fn read_hard_corridors(value: Option<&Value>) -> Vec<NetworkHardCorridor> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .map(|row| NetworkHardCorridor {
                    x1: js_or_default(number_field(row, "x1"), 0.0),
                    y1: js_or_default(number_field(row, "y1"), 0.0),
                    x2: js_or_default(number_field(row, "x2"), 0.0),
                    y2: js_or_default(number_field(row, "y2"), 0.0),
                    half_width: js_or_default(number_field(row, "halfWidth"), 0.0),
                })
                .collect()
        })
        .unwrap_or_default()
}

fn read_angle_ranges(value: Option<&Value>) -> Vec<NetworkBucketAngleRange> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .map(|row| NetworkBucketAngleRange {
                    start: js_or_default(number_field(row, "start"), 0.0),
                    end: js_or_default(number_field(row, "end"), 0.0),
                    span: number_field(row, "span"),
                })
                .collect()
        })
        .unwrap_or_default()
}

fn read_preferred_angles(value: Option<&Value>) -> Vec<NetworkPreferredAngle> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .filter_map(|row| {
                    let angle = number_field(row, "angle")?;
                    Some(NetworkPreferredAngle {
                        angle,
                        width: js_or_default(number_field(row, "width"), 1.55).clamp(0.35, PI),
                        weight: js_or_default(number_field(row, "weight"), 1.0).clamp(-3.2, 3.2),
                    })
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
                .map(|row| NetworkOwnerCenter {
                    id: text_field(row, "id").unwrap_or_default(),
                    x: js_or_default(number_field(row, "x"), 0.0),
                    y: js_or_default(number_field(row, "y"), 0.0),
                })
                .collect()
        })
        .unwrap_or_default()
}

fn read_placement_center(value: Option<&Value>) -> NetworkPlacementCenter {
    let row = value.unwrap_or(&Value::Null);
    NetworkPlacementCenter {
        id: text_field(row, "id").unwrap_or_default(),
        center_type: text_field(row, "type").unwrap_or_default(),
        x: js_or_default(number_field(row, "x"), 0.0),
        y: js_or_default(number_field(row, "y"), 0.0),
        node_radius: js_or_default(number_field(row, "nodeRadius"), 18.0).max(0.0),
        collision_radius: js_or_default(number_field(row, "collisionRadius"), 38.0).max(0.0),
    }
}

fn read_placement_leaf_rows(value: Option<&Value>) -> Vec<NetworkPlacementLeafRow> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .filter_map(|row| {
                    let id = text_field(row, "id").unwrap_or_default();
                    if id.is_empty() {
                        return None;
                    }
                    Some(NetworkPlacementLeafRow {
                        id,
                        label_pad: js_or_default(number_field(row, "labelPad"), 0.0).max(0.0),
                        node_radius: js_or_default(number_field(row, "nodeRadius"), 18.0).max(0.0),
                        collision_radius: js_or_default(number_field(row, "collisionRadius"), 38.0)
                            .max(0.0),
                    })
                })
                .collect()
        })
        .unwrap_or_default()
}

fn read_number_array(value: Option<&Value>) -> Vec<f64> {
    value
        .and_then(Value::as_array)
        .map(|rows| rows.iter().map(js_number_or_zero).collect())
        .unwrap_or_default()
}

fn finite_number_field(value: &Value, key: &str) -> Option<f64> {
    number_field(value, key).filter(|number| number.is_finite())
}

fn finite_usize_field(value: &Value, key: &str) -> Option<usize> {
    finite_number_field(value, key).map(|number| number.floor().max(0.0) as usize)
}

fn text_field(value: &Value, key: &str) -> Option<String> {
    value
        .get(key)
        .and_then(value_text)
        .map(|text| text.trim().to_string())
}

fn value_text(value: &Value) -> Option<String> {
    match value {
        Value::String(text) => Some(text.to_string()),
        Value::Number(number) => Some(number.to_string()),
        Value::Bool(value) => Some(value.to_string()),
        _ => None,
    }
}

fn truthy_field(value: &Value, key: &str) -> bool {
    match value.get(key) {
        Some(Value::Bool(value)) => *value,
        Some(Value::Number(number)) => number.as_f64().is_some_and(|value| value != 0.0),
        Some(Value::String(text)) => !text.is_empty(),
        Some(Value::Null) | None => false,
        Some(Value::Array(_)) | Some(Value::Object(_)) => true,
    }
}

fn js_number_or_zero(value: &Value) -> f64 {
    match value {
        Value::Number(number) => number.as_f64().unwrap_or(0.0),
        Value::String(text) => text.trim().parse::<f64>().unwrap_or(0.0),
        Value::Bool(value) => {
            if *value {
                1.0
            } else {
                0.0
            }
        }
        _ => 0.0,
    }
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

fn number_field(value: &Value, key: &str) -> Option<f64> {
    match value.get(key) {
        Some(Value::Number(number)) => number.as_f64(),
        Some(Value::String(text)) => text.trim().parse::<f64>().ok(),
        Some(Value::Bool(value)) => Some(if *value { 1.0 } else { 0.0 }),
        Some(Value::Null) => Some(0.0),
        _ => None,
    }
}
