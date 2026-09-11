use crate::flow_layout_network_model::NetworkOwnerCenter;
use serde_json::Value;
use std::collections::BTreeSet;

pub(crate) struct NetworkAdjacentOwnerGroup {
    pub(crate) id: String,
    pub(crate) leaf_ids: Vec<String>,
}

pub(crate) struct NetworkOwnerGroups {
    pub(crate) core_id: String,
    pub(crate) core_leaf_ids: Vec<String>,
    pub(crate) adjacent_groups: Vec<NetworkAdjacentOwnerGroup>,
}

pub(crate) struct NetworkCenterOwnershipChecker {
    own_id: String,
    own_x: f64,
    own_y: f64,
    centers: Vec<NetworkOwnerCenter>,
    strict: bool,
    guaranteed_own_radius_sq: f64,
}

impl NetworkCenterOwnershipChecker {
    pub(crate) fn new(
        center_id: &str,
        centers: &[NetworkOwnerCenter],
        strict: bool,
    ) -> Option<Self> {
        if !strict || centers.is_empty() {
            return Some(Self {
                own_id: center_id.trim().to_string(),
                own_x: 0.0,
                own_y: 0.0,
                centers: Vec::new(),
                strict,
                guaranteed_own_radius_sq: f64::INFINITY,
            });
        }

        let own_id = center_id.trim().to_string();
        let own_center = centers.iter().find(|row| row.id.trim() == own_id)?;
        let own_x = own_center.x;
        let own_y = own_center.y;
        let mut min_other_dist = f64::INFINITY;
        for row in centers {
            if row.id.trim() == own_id {
                continue;
            }
            let dx = row.x - own_x;
            let dy = row.y - own_y;
            let dist = (dx * dx + dy * dy).sqrt();
            if dist.is_finite() && dist > 1e-6 {
                min_other_dist = min_other_dist.min(dist);
            }
        }
        let guaranteed_own_radius_sq = if min_other_dist.is_finite() {
            (min_other_dist * 0.5 - 1e-6).max(0.0).powi(2)
        } else {
            f64::INFINITY
        };

        Some(Self {
            own_id,
            own_x,
            own_y,
            centers: centers.to_vec(),
            strict,
            guaranteed_own_radius_sq,
        })
    }

    pub(crate) fn owns(&self, x: f64, y: f64) -> bool {
        if !self.strict || self.centers.is_empty() {
            return true;
        }
        if self.own_id.is_empty() {
            return nearest_center_id_at(x, y, &self.centers).is_none();
        }
        let dx = x - self.own_x;
        let dy = y - self.own_y;
        let own_dist_sq = dx * dx + dy * dy;
        if own_dist_sq <= self.guaranteed_own_radius_sq {
            return true;
        }
        nearest_center_id_at(x, y, &self.centers).as_deref() == Some(self.own_id.as_str())
    }
}

pub(crate) fn resolve_core_adjacent_leaf_owner_groups(
    cluster: &Value,
) -> Option<NetworkOwnerGroups> {
    let node_ids = read_string_vec(cluster.get("nodeIds"));
    let core_ids = read_string_vec(cluster.get("coreIds"));
    let adjacent_ids = read_string_vec(cluster.get("adjacentIds"));
    let leaf_ids = read_string_vec(cluster.get("leafIds"));
    if node_ids.len() < 3 || core_ids.len() != 1 || adjacent_ids.is_empty() || leaf_ids.is_empty() {
        return None;
    }

    let node_set = node_ids.iter().cloned().collect::<BTreeSet<_>>();
    let core_id = core_ids.first()?.clone();
    if !node_set.contains(&core_id) || adjacent_ids.iter().any(|id| !node_set.contains(id)) {
        return None;
    }

    let adjacent_set = adjacent_ids.iter().cloned().collect::<BTreeSet<_>>();
    let mut center_ids = vec![core_id.clone()];
    center_ids.extend(adjacent_ids.iter().cloned());
    let center_set = center_ids.iter().cloned().collect::<BTreeSet<_>>();
    let leaf_set = leaf_ids.iter().cloned().collect::<BTreeSet<_>>();
    if leaf_set.len() != leaf_ids.len()
        || leaf_ids
            .iter()
            .any(|id| !node_set.contains(id) || id == &core_id || adjacent_set.contains(id))
        || leaf_ids.len() + adjacent_ids.len() + 1 != node_ids.len()
    {
        return None;
    }

    let local_adjacency = cluster.get("localAdjacency")?;
    if adjacent_ids.iter().any(|id| {
        !read_string_vec(local_adjacency.get(id))
            .iter()
            .any(|neighbor_id| neighbor_id == &core_id)
    }) {
        return None;
    }

    let mut leaf_ids_by_center = center_ids
        .iter()
        .map(|id| (id.clone(), Vec::<String>::new()))
        .collect::<Vec<_>>();
    for leaf_id in &leaf_ids {
        let owner_ids = read_string_vec(local_adjacency.get(leaf_id))
            .into_iter()
            .filter(|id| center_set.contains(id))
            .collect::<Vec<_>>();
        if owner_ids.len() != 1 {
            return None;
        }
        let owner_id = owner_ids.first()?.clone();
        if let Some((_, rows)) = leaf_ids_by_center
            .iter_mut()
            .find(|(center_id, _)| center_id == &owner_id)
        {
            rows.push(leaf_id.clone());
        }
    }

    let core_leaf_ids = leaf_ids_by_center
        .iter()
        .find(|(center_id, _)| center_id == &core_id)
        .map(|(_, rows)| rows.clone())
        .unwrap_or_default();
    let adjacent_groups = adjacent_ids
        .iter()
        .map(|id| NetworkAdjacentOwnerGroup {
            id: id.clone(),
            leaf_ids: leaf_ids_by_center
                .iter()
                .find(|(center_id, _)| center_id == id)
                .map(|(_, rows)| rows.clone())
                .unwrap_or_default(),
        })
        .collect::<Vec<_>>();

    Some(NetworkOwnerGroups {
        core_id,
        core_leaf_ids,
        adjacent_groups,
    })
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

fn js_or_empty_string(value: &Value) -> String {
    match value {
        Value::String(text) => text.to_string(),
        Value::Number(number) => number.to_string(),
        Value::Bool(value) => value.to_string(),
        _ => String::new(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn network_plan_projected_leaf_owners_require_nearest_center() {
        let centers = vec![
            NetworkOwnerCenter {
                id: "core".to_string(),
                x: 0.0,
                y: 0.0,
            },
            NetworkOwnerCenter {
                id: "adjacent".to_string(),
                x: 100.0,
                y: 0.0,
            },
        ];
        let core_checker =
            NetworkCenterOwnershipChecker::new("core", &centers, true).expect("core checker");
        let adjacent_checker = NetworkCenterOwnershipChecker::new("adjacent", &centers, true)
            .expect("adjacent checker");

        assert!(core_checker.owns(20.0, 0.0));
        assert!(adjacent_checker.owns(80.0, 0.0));
        assert!(!adjacent_checker.owns(20.0, 0.0));
    }

    #[test]
    fn network_plan_center_ownership_checker_matches_fast_and_nearest_paths() {
        let centers = vec![
            NetworkOwnerCenter {
                id: "left".to_string(),
                x: -100.0,
                y: 0.0,
            },
            NetworkOwnerCenter {
                id: "center".to_string(),
                x: 0.0,
                y: 0.0,
            },
            NetworkOwnerCenter {
                id: "right".to_string(),
                x: 100.0,
                y: 0.0,
            },
        ];
        let checker =
            NetworkCenterOwnershipChecker::new("center", &centers, true).expect("checker");

        assert!(checker.owns(20.0, 0.0));
        assert!(checker.owns(0.0, 60.0));
        assert!(!checker.owns(80.0, 0.0));
        assert!(
            NetworkCenterOwnershipChecker::new("missing", &centers, false)
                .expect("non-strict checker")
                .owns(80.0, 0.0)
        );
    }
}
