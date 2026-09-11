use crate::flow_layout_network_model::{
    NetworkBucketSectorPlan, NetworkLeafSector, NetworkOwnerCenter,
};
use crate::flow_layout_network_node_metrics::{label_pads_for_ids, project_node_metrics};
use crate::flow_layout_network_ownership::{
    resolve_core_adjacent_leaf_owner_groups, NetworkCenterOwnershipChecker, NetworkOwnerGroups,
};
use crate::flow_layout_network_sector::{
    number_field, number_json, project_core_leaf_sector, project_sector_leaf_plan,
    sector_span_for_leaf_count, stable_hash_unit, NetworkSectorCenterReport,
    NetworkSectorLeafPlanInput, NetworkSectorNodeUpdate, TWO_PI,
};
use crate::flow_layout_network_territory::{
    build_center_territory_ranges, constrain_sector_to_territory, intersect_angle_ranges,
    territory_sample_radius,
};
use serde_json::{json, Map, Value};
use std::collections::{BTreeMap, BTreeSet};
use std::f64::consts::PI;

struct NetworkClusterPlan {
    node_updates: Vec<NetworkSectorNodeUpdate>,
    center_reports: Vec<NetworkSectorCenterReport>,
}

struct AdjacentCenterPlan {
    id: String,
    x: f64,
    y: f64,
    angle: f64,
    leaf_ids: Vec<String>,
}

fn resolve_adjacent_center_radius(
    owner_groups: &NetworkOwnerGroups,
    config: &Value,
    node_by_id: Option<&Map<String, Value>>,
) -> f64 {
    let adjacent_count = owner_groups.adjacent_groups.len();
    let base_radius = number_field(config, "NETWORK_ADJ_SPACING")
        .or_else(|| number_field(config, "R_ADJ_MIN"))
        .unwrap_or(118.0)
        .max(74.0);
    if adjacent_count == 0 {
        return base_radius;
    }

    let core_metrics =
        project_node_metrics(node_by_id.and_then(|rows| rows.get(&owner_groups.core_id)));
    let mut max_adj_collision = 38.0_f64;
    let mut max_adj_half_width = 24.0_f64;
    for group in &owner_groups.adjacent_groups {
        let metrics = project_node_metrics(node_by_id.and_then(|rows| rows.get(&group.id)));
        max_adj_collision = max_adj_collision.max(metrics.collision_radius);
        max_adj_half_width = max_adj_half_width.max(metrics.label_half_width);
    }

    let total_leaf_count = owner_groups.core_leaf_ids.len()
        + owner_groups
            .adjacent_groups
            .iter()
            .map(|row| row.leaf_ids.len())
            .sum::<usize>();
    let pair_gap = (max_adj_collision * 2.0 + 54.0)
        .max(max_adj_half_width * 2.0 + 78.0)
        .max(172.0);
    let angular_radius_need = if adjacent_count > 1 {
        let slot_angle = (PI / adjacent_count as f64).sin().max(0.08);
        pair_gap / (2.0 * slot_angle)
    } else {
        0.0
    };
    let core_clearance_need = core_metrics.collision_radius + max_adj_collision + 54.0;
    let density_need = base_radius
        + 84.0
        + (total_leaf_count.max(1) as f64).sqrt() * 4.8
        + (adjacent_count.max(1) as f64).sqrt() * 22.0;

    angular_radius_need
        .max(core_clearance_need)
        .max(density_need)
        .max(base_radius)
        .clamp(base_radius, 1800.0)
}

pub(crate) fn project_network_cluster_plan(
    cluster_result: &Value,
    cluster_placement_by_id: &Value,
    config: &Value,
    node_by_id: Option<&Map<String, Value>>,
) -> Value {
    let Some(clusters) = cluster_result.get("clusters").and_then(Value::as_array) else {
        return empty_network_plan();
    };
    let mut updates_by_cluster = Map::new();
    let mut center_reports_by_cluster = Map::new();
    let mut leaf_zones_by_cluster = Map::new();
    let mut covered_clusters = 0usize;
    let mut covered_nodes = 0usize;

    for cluster in clusters {
        let cluster_id = text_field(cluster, "clusterId");
        if cluster_id.is_empty() {
            continue;
        }
        let layout_case = text_field(cluster, "layoutCase");
        if layout_case != "single-center-sector" {
            continue;
        }
        let Some(placement) = cluster_placement_by_id.get(&cluster_id) else {
            continue;
        };
        let Some(plan) =
            project_single_center_sector_cluster(cluster, placement, config, node_by_id)
        else {
            continue;
        };
        let Some(leaf_zone_rows) =
            project_network_leaf_zone_rows(&plan.node_updates, &plan.center_reports)
        else {
            continue;
        };
        covered_nodes += plan.node_updates.len();
        covered_clusters += 1;
        updates_by_cluster.insert(
            cluster_id.clone(),
            Value::Array(
                plan.node_updates
                    .into_iter()
                    .map(NetworkSectorNodeUpdate::into_json)
                    .collect(),
            ),
        );
        center_reports_by_cluster.insert(
            cluster_id.clone(),
            Value::Array(
                plan.center_reports
                    .into_iter()
                    .map(NetworkSectorCenterReport::into_json)
                    .collect(),
            ),
        );
        leaf_zones_by_cluster.insert(cluster_id, Value::Array(leaf_zone_rows));
    }

    json!({
        "networkNodeUpdatesByClusterId": updates_by_cluster,
        "networkCenterReportsByClusterId": center_reports_by_cluster,
        "networkLeafZonesByClusterId": leaf_zones_by_cluster,
        "networkNodeUpdateSummary": {
            "clusterCount": covered_clusters,
            "nodeCount": covered_nodes,
            "coveredCases": {
                "single-center-sector": covered_clusters,
            },
        },
    })
}

fn empty_network_plan() -> Value {
    json!({
        "networkNodeUpdatesByClusterId": {},
        "networkCenterReportsByClusterId": {},
        "networkLeafZonesByClusterId": {},
        "networkNodeUpdateSummary": {
            "clusterCount": 0,
            "nodeCount": 0,
            "coveredCases": {
                "single-center-sector": 0,
            },
        },
    })
}

fn project_network_leaf_zone_rows(
    node_updates: &[NetworkSectorNodeUpdate],
    center_reports: &[NetworkSectorCenterReport],
) -> Option<Vec<Value>> {
    let center_index_by_id = center_reports
        .iter()
        .enumerate()
        .map(|(index, row)| (row.center_id().to_string(), index))
        .collect::<BTreeMap<_, _>>();
    let mut out = Vec::new();
    for row in node_updates {
        if row.layout_band() != "leaf" {
            continue;
        }
        let owner_center_id = row.owner_center_id()?.to_string();
        let owner_sector_index = *center_index_by_id.get(&owner_center_id)?;
        out.push(json!({
            "id": row.id(),
            "x": number_json(row.x()),
            "y": number_json(row.y()),
            "ownerCenterId": owner_center_id,
            "ownerSectorIndex": owner_sector_index,
        }));
    }
    Some(out)
}

fn project_single_center_sector_cluster(
    cluster: &Value,
    placement: &Value,
    config: &Value,
    node_by_id: Option<&Map<String, Value>>,
) -> Option<NetworkClusterPlan> {
    if let Some(plan) =
        project_core_adjacent_leaf_sector_cluster(cluster, placement, config, node_by_id)
    {
        return Some(plan);
    }
    project_anchor_leaf_sector_cluster(cluster, placement, config, node_by_id)
}

fn project_anchor_leaf_sector_cluster(
    cluster: &Value,
    placement: &Value,
    config: &Value,
    node_by_id: Option<&Map<String, Value>>,
) -> Option<NetworkClusterPlan> {
    let node_ids = read_string_vec(cluster.get("nodeIds"));
    if node_ids.len() < 2 {
        return None;
    }
    let anchor_id = read_string_vec(cluster.get("layoutAnchorIds"))
        .first()
        .cloned()
        .or_else(|| read_string_vec(cluster.get("coreIds")).first().cloned())
        .or_else(|| node_ids.first().cloned())
        .unwrap_or_default();
    if anchor_id.is_empty() || !node_ids.iter().any(|id| id == &anchor_id) {
        return None;
    }
    let center_x = number_field(placement, "x").unwrap_or(0.0);
    let center_y = number_field(placement, "y").unwrap_or(0.0);
    let leaf_ids = read_string_vec(cluster.get("leafIds"));
    let leaf_set = leaf_ids.iter().cloned().collect::<BTreeSet<_>>();
    let mut ring_ids = node_ids
        .iter()
        .filter(|id| *id != &anchor_id)
        .filter(|id| leaf_set.is_empty() || leaf_set.contains(*id))
        .cloned()
        .collect::<Vec<_>>();
    if ring_ids.is_empty() {
        ring_ids = node_ids
            .iter()
            .filter(|id| *id != &anchor_id)
            .cloned()
            .collect::<Vec<_>>();
    }
    if ring_ids.is_empty() {
        return None;
    }
    if ring_ids.len() + 1 != node_ids.len() {
        return None;
    }
    ring_ids.sort();

    let mut out = vec![NetworkSectorNodeUpdate::new(
        anchor_id.clone(),
        center_x,
        center_y,
        "center",
        "single-center-sector",
    )];

    let center_type = if read_string_vec(cluster.get("coreIds"))
        .iter()
        .any(|id| id == &anchor_id)
    {
        "core"
    } else {
        "layoutAnchor"
    };
    let phase_key = format!("network-sector|{}", text_field(cluster, "clusterId"));
    let leaf_label_pads = label_pads_for_ids(&ring_ids, node_by_id);
    let sector_plan = NetworkBucketSectorPlan::from_static_sectors(
        vec![NetworkLeafSector::full_circle()],
        &leaf_label_pads,
        config,
    );
    let leaf_plan = project_sector_leaf_plan(NetworkSectorLeafPlanInput {
        center_id: &anchor_id,
        center_type,
        center_x,
        center_y,
        leaf_ids: &ring_ids,
        sector_plan: &sector_plan,
        phase_key: &phase_key,
        jitter_key_prefix: "network-sector-r",
        config,
    });
    out.extend(leaf_plan.node_updates);

    Some(NetworkClusterPlan {
        node_updates: out,
        center_reports: vec![leaf_plan.center_report],
    })
}

fn project_core_adjacent_leaf_sector_cluster(
    cluster: &Value,
    placement: &Value,
    config: &Value,
    node_by_id: Option<&Map<String, Value>>,
) -> Option<NetworkClusterPlan> {
    let owner_groups = resolve_core_adjacent_leaf_owner_groups(cluster)?;
    let core_id = owner_groups.core_id.as_str();
    let core_x = number_field(placement, "x").unwrap_or(0.0);
    let core_y = number_field(placement, "y").unwrap_or(0.0);
    let adjacent_radius = resolve_adjacent_center_radius(&owner_groups, config, node_by_id);
    let r_leaf_min = number_field(config, "R_LEAF_MIN")
        .unwrap_or(220.0)
        .max(80.0);
    let adjacent_phase = stable_hash_unit(&format!(
        "network-adjacent-sector|{}|{}",
        text_field(cluster, "clusterId"),
        core_id
    ));
    let mut out = vec![NetworkSectorNodeUpdate::new(
        core_id,
        core_x,
        core_y,
        "core",
        "single-center-sector",
    )];
    let mut center_reports = Vec::new();
    let mut adjacent_plans = Vec::new();
    let mut owner_centers = vec![NetworkOwnerCenter {
        id: core_id.to_string(),
        x: core_x,
        y: core_y,
    }];

    for (adjacent_index, adjacent_group) in owner_groups.adjacent_groups.iter().enumerate() {
        let adjacent_ratio = if owner_groups.adjacent_groups.len() <= 1 {
            adjacent_phase
        } else {
            ((adjacent_index as f64) + 0.5 + adjacent_phase)
                / (owner_groups.adjacent_groups.len() as f64)
        };
        let adjacent_angle = -PI + adjacent_ratio.fract() * TWO_PI;
        let adjacent_x = core_x + adjacent_angle.cos() * adjacent_radius;
        let adjacent_y = core_y + adjacent_angle.sin() * adjacent_radius;
        owner_centers.push(NetworkOwnerCenter {
            id: adjacent_group.id.clone(),
            x: adjacent_x,
            y: adjacent_y,
        });
        adjacent_plans.push(AdjacentCenterPlan {
            id: adjacent_group.id.clone(),
            x: adjacent_x,
            y: adjacent_y,
            angle: adjacent_angle,
            leaf_ids: adjacent_group.leaf_ids.clone(),
        });
    }

    if !owner_groups.core_leaf_ids.is_empty() {
        let sector = project_core_leaf_sector(
            adjacent_plans
                .iter()
                .map(|row| row.angle)
                .collect::<Vec<_>>()
                .as_slice(),
            owner_groups.core_leaf_ids.len(),
        );
        let core_territory = build_center_territory_ranges(
            &owner_centers[0],
            &owner_centers,
            territory_sample_radius(r_leaf_min, owner_groups.core_leaf_ids.len()),
            216,
            0.14,
        );
        let (sector_start, sector_end) =
            constrain_sector_to_territory(sector.start, sector.end, &core_territory)?;
        let sector = sector.with_bounds(sector_start, sector_end);
        let leaf_label_pads = label_pads_for_ids(&owner_groups.core_leaf_ids, node_by_id);
        let sector_plan =
            NetworkBucketSectorPlan::from_static_sectors(vec![sector], &leaf_label_pads, config);
        let phase_key = format!(
            "network-core-leaf-sector|{}|{}",
            text_field(cluster, "clusterId"),
            core_id
        );
        let leaf_plan = project_sector_leaf_plan(NetworkSectorLeafPlanInput {
            center_id: core_id,
            center_type: "core",
            center_x: core_x,
            center_y: core_y,
            leaf_ids: &owner_groups.core_leaf_ids,
            sector_plan: &sector_plan,
            phase_key: &phase_key,
            jitter_key_prefix: "network-core-sector-r",
            config,
        });
        out.extend(leaf_plan.node_updates);
        center_reports.push(leaf_plan.center_report);
    }

    for adjacent in adjacent_plans {
        out.push(NetworkSectorNodeUpdate::new(
            adjacent.id.clone(),
            adjacent.x,
            adjacent.y,
            "adjacent",
            "single-center-sector",
        ));
        if adjacent.leaf_ids.is_empty() {
            continue;
        }

        let sector = NetworkLeafSector::around(
            adjacent.angle,
            sector_span_for_leaf_count(adjacent.leaf_ids.len()),
        );
        let adjacent_center = owner_centers
            .iter()
            .find(|row| row.id == adjacent.id)
            .cloned()?;
        let mut adjacent_territory = build_center_territory_ranges(
            &adjacent_center,
            &owner_centers,
            territory_sample_radius(r_leaf_min, adjacent.leaf_ids.len()),
            216,
            0.14,
        );
        let outward_span =
            (1.72 + (adjacent.leaf_ids.len().max(1) as f64).sqrt() * 0.13).clamp(1.48, PI * 1.34);
        let outward_territory =
            intersect_angle_ranges(&adjacent_territory, adjacent.angle, outward_span);
        if !outward_territory.is_empty() {
            adjacent_territory = outward_territory;
        }
        let (sector_start, sector_end) =
            constrain_sector_to_territory(sector.start, sector.end, &adjacent_territory)?;
        let sector = sector.with_bounds(sector_start, sector_end);
        let leaf_label_pads = label_pads_for_ids(&adjacent.leaf_ids, node_by_id);
        let sector_plan =
            NetworkBucketSectorPlan::from_static_sectors(vec![sector], &leaf_label_pads, config);
        let phase_key = format!(
            "network-adjacent-leaf-sector|{}|{}",
            text_field(cluster, "clusterId"),
            adjacent.id
        );
        let leaf_plan = project_sector_leaf_plan(NetworkSectorLeafPlanInput {
            center_id: &adjacent.id,
            center_type: "adjacent",
            center_x: adjacent.x,
            center_y: adjacent.y,
            leaf_ids: &adjacent.leaf_ids,
            sector_plan: &sector_plan,
            phase_key: &phase_key,
            jitter_key_prefix: "network-adjacent-sector-r",
            config,
        });
        out.extend(leaf_plan.node_updates);
        center_reports.push(leaf_plan.center_report);
    }

    if !projected_leaf_owners_are_nearest(&out, &owner_centers) {
        return None;
    }

    Some(NetworkClusterPlan {
        node_updates: out,
        center_reports,
    })
}

fn projected_leaf_owners_are_nearest(
    node_updates: &[NetworkSectorNodeUpdate],
    centers: &[NetworkOwnerCenter],
) -> bool {
    if centers.len() <= 1 {
        return true;
    }
    for row in node_updates {
        if row.layout_band() != "leaf" {
            continue;
        }
        let Some(owner_id) = row.owner_center_id() else {
            return false;
        };
        let Some(checker) = NetworkCenterOwnershipChecker::new(owner_id, centers, true) else {
            return false;
        };
        if !checker.owns(row.x(), row.y()) {
            return false;
        }
    }
    true
}

fn read_string_vec(value: Option<&Value>) -> Vec<String> {
    let mut out = value
        .and_then(Value::as_array)
        .map(|rows| rows.iter().map(js_or_empty_string).collect::<Vec<_>>())
        .unwrap_or_default()
        .into_iter()
        .map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
        .collect::<Vec<_>>();
    out.sort();
    out.dedup();
    out
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
