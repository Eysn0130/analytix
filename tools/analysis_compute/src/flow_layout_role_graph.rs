use crate::flow_layout_cluster_plan::project_cluster_layout_plan;
use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Map, Value};
use std::cmp::Ordering;
use std::collections::{BTreeMap, BTreeSet, HashMap, VecDeque};
use std::{fs, path::PathBuf};

pub(crate) struct LayoutRoleGraphArgs {
    pub(crate) payload: Value,
}

#[derive(Clone)]
struct PairMeta {
    source: String,
    target: String,
    line_type: String,
    edge_indexes: Vec<usize>,
}

struct RoleGraphData {
    node_ids: Vec<String>,
    node_row_ids: Vec<String>,
    raw_node_count: usize,
    node_by_id: HashMap<String, Value>,
    adjacency: HashMap<String, BTreeSet<String>>,
    raw_degree_by_id: HashMap<String, usize>,
    neighbor_edge_map: HashMap<String, Map<String, Value>>,
    unique_undirected_edges: Vec<PairMeta>,
    components: Vec<Vec<String>>,
    component_by_node: HashMap<String, usize>,
}

#[derive(Clone, Copy)]
struct HistoryRow {
    total: f64,
    kept: f64,
    stable_score: f64,
}

#[derive(Clone)]
struct SeedMetaInput {
    is_seed_selected: bool,
    seed_weight: f64,
    seed_stability_score: f64,
}

#[derive(Clone)]
struct NodeRoleMeta {
    id: String,
    role: String,
    seed_core_candidate: bool,
    is_seed_selected: bool,
    demote_reason: Option<String>,
    is_path_promoted_adjacent: bool,
    is_bridge_adjacent: bool,
    is_hub_adjacent: bool,
    layout_anchor_for_cluster: bool,
    why: Vec<String>,
    weight: f64,
    seed_weight: f64,
    seed_stability_score: f64,
}

pub(crate) fn parse_args(mut iter: impl Iterator<Item = String>) -> Result<LayoutRoleGraphArgs> {
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
                    format!("read layout role graph input {}", input_path.display())
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!("parse layout role graph input {}", input_path.display())
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute project-layout-role-graph <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported layout role graph flag: {other}"),
        }
    }

    Ok(LayoutRoleGraphArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_layout_role_graph(payload: &Value) -> Value {
    let role_graph_data = build_role_graph_data(payload);
    let mut projection = Map::new();
    projection.insert("roleGraph".to_string(), role_graph_json(&role_graph_data));
    let traversal = payload
        .get("traversal")
        .map(project_traversal)
        .unwrap_or(Value::Null);
    projection.insert("traversal".to_string(), traversal);
    if let Some(pipeline_payload) = payload.get("semanticPipeline") {
        let semantic = project_semantic_pipeline(&role_graph_data, pipeline_payload);
        projection.insert("demotion".to_string(), semantic.demotion);
        projection.insert("promotion".to_string(), semantic.promotion);
        projection.insert("roleResult".to_string(), semantic.role_result);
        projection.insert("clusterResult".to_string(), semantic.cluster_result);
        projection.insert("preferredCoreIds".to_string(), semantic.preferred_core_ids);
        projection.insert("labelPolicy".to_string(), semantic.label_policy);
        if let Some(cluster_layout_plan) = semantic.cluster_layout_plan {
            projection.insert("clusterLayoutPlan".to_string(), cluster_layout_plan);
        }
        return Value::Object(projection);
    }
    if let Some(demotion_payload) = payload.get("demotion") {
        projection.insert(
            "demotion".to_string(),
            project_seed_demotion(&role_graph_data, demotion_payload),
        );
    }
    if let Some(promotion_payload) = payload.get("promotion") {
        projection.insert(
            "promotion".to_string(),
            project_path_promotion(&role_graph_data, promotion_payload),
        );
    }
    let role_result = payload
        .get("roleResolution")
        .map(|role_payload| project_role_resolution(&role_graph_data, role_payload));
    if let Some(role_result) = &role_result {
        projection.insert("roleResult".to_string(), role_result.clone());
    }
    if let Some(cluster_payload) = payload.get("clusterResolution") {
        projection.insert(
            "clusterResult".to_string(),
            project_cluster_detection(
                &role_graph_data,
                role_result
                    .as_ref()
                    .or_else(|| cluster_payload.get("roles")),
            ),
        );
    }
    Value::Object(projection)
}

struct SemanticPipelineProjection {
    demotion: Value,
    promotion: Value,
    role_result: Value,
    cluster_result: Value,
    cluster_layout_plan: Option<Value>,
    preferred_core_ids: Value,
    label_policy: Value,
}

fn project_semantic_pipeline(graph: &RoleGraphData, payload: &Value) -> SemanticPipelineProjection {
    let options = payload.get("options").unwrap_or(payload);
    let demotion_payload = json!({
        "seedCoreCandidates": payload.get("seedCoreCandidates").cloned().unwrap_or(Value::Array(Vec::new())),
        "options": options,
    });
    let demotion = project_seed_demotion(graph, &demotion_payload);
    let core_ids = demotion
        .get("coreIds")
        .cloned()
        .unwrap_or(Value::Array(Vec::new()));
    let promotion_payload = json!({
        "coreIds": core_ids,
        "options": options,
    });
    let promotion = project_path_promotion(graph, &promotion_payload);
    let role_payload = json!({
        "coreIds": demotion.get("coreIds").cloned().unwrap_or(Value::Array(Vec::new())),
        "pathPromotedAdjacentIds": promotion
            .get("pathPromotedAdjacentIds")
            .cloned()
            .unwrap_or(Value::Array(Vec::new())),
        "demoteTags": demotion.get("demoteTags").cloned().unwrap_or(Value::Object(Map::new())),
        "options": {
            "seedMeta": demotion.get("seedMeta").cloned().unwrap_or(Value::Object(Map::new())),
        },
    });
    let mut role_result = project_role_resolution(graph, &role_payload);
    let mut cluster_result = project_cluster_detection(graph, Some(&role_result));
    let preferred_core_ids = preferred_core_ids_json(graph, &role_result, payload.get("focusId"));
    let label_policy = label_policy_json(graph, &role_result, &cluster_result);
    let cluster_layout_plan = payload.get("clusterLayout").map(|cluster_layout_payload| {
        let mut cluster_layout_payload = cluster_layout_payload.clone();
        if let Some(obj) = cluster_layout_payload.as_object_mut() {
            obj.insert(
                "nodeRowsById".to_string(),
                Value::Object(
                    graph
                        .node_by_id
                        .iter()
                        .map(|(id, node)| (id.clone(), node.clone()))
                        .collect(),
                ),
            );
        }
        project_cluster_layout_plan(
            &graph.adjacency,
            &mut role_result,
            &mut cluster_result,
            &cluster_layout_payload,
        )
    });
    SemanticPipelineProjection {
        demotion,
        promotion,
        role_result,
        cluster_result,
        cluster_layout_plan,
        preferred_core_ids,
        label_policy,
    }
}

fn build_role_graph_data(graph: &Value) -> RoleGraphData {
    let empty = Vec::new();
    let node_rows = graph
        .get("nodes")
        .and_then(Value::as_array)
        .unwrap_or(&empty);
    let edge_rows = graph
        .get("edges")
        .and_then(Value::as_array)
        .unwrap_or(&empty);

    let mut node_by_id: HashMap<String, Value> = HashMap::new();
    let mut node_row_ids = Vec::new();
    for node in node_rows {
        let id = node
            .get("id")
            .map(js_or_empty_string)
            .unwrap_or_default()
            .trim()
            .to_string();
        if id.is_empty() {
            continue;
        }
        node_row_ids.push(id.clone());
        if node_by_id.contains_key(&id) {
            continue;
        }
        node_by_id.insert(id, node.clone());
    }

    let node_ids = sort_stable_ids(node_by_id.keys().cloned().collect());
    let node_id_set: BTreeSet<String> = node_ids.iter().cloned().collect();
    let mut adjacency: HashMap<String, BTreeSet<String>> = HashMap::new();
    let mut neighbor_edge_map: HashMap<String, Map<String, Value>> = HashMap::new();
    for id in &node_ids {
        adjacency.insert(id.clone(), BTreeSet::new());
        neighbor_edge_map.insert(id.clone(), Map::new());
    }

    let mut pair_index: HashMap<String, usize> = HashMap::new();
    let mut pair_meta: Vec<PairMeta> = Vec::new();
    let mut raw_degree_by_id: HashMap<String, usize> = HashMap::new();
    for (index, edge) in edge_rows.iter().enumerate() {
        if let (Some(source_key), Some(target_key)) = (
            edge_degree_key(edge.get("source")),
            edge_degree_key(edge.get("target")),
        ) {
            *raw_degree_by_id.entry(source_key).or_default() += 1;
            *raw_degree_by_id.entry(target_key).or_default() += 1;
        }
        if edge.get("isVisible") == Some(&Value::Bool(false)) {
            continue;
        }
        let source = edge
            .get("source")
            .map(js_or_empty_string)
            .unwrap_or_default()
            .trim()
            .to_string();
        let target = edge
            .get("target")
            .map(js_or_empty_string)
            .unwrap_or_default()
            .trim()
            .to_string();
        if source.is_empty()
            || target.is_empty()
            || source == target
            || !node_id_set.contains(&source)
            || !node_id_set.contains(&target)
        {
            continue;
        }
        adjacency
            .entry(source.clone())
            .or_default()
            .insert(target.clone());
        adjacency
            .entry(target.clone())
            .or_default()
            .insert(source.clone());
        let pair_key = role_pair_key(&source, &target);
        if pair_key.is_empty() {
            continue;
        }
        let mode = edge
            .get("mode")
            .map(js_or_empty_string)
            .unwrap_or_default()
            .to_lowercase();
        let arrow = first_non_empty_text(edge.get("edgeArrow"), edge.get("arrow")).to_lowercase();
        let line_type = if mode == "double" || arrow == "both" {
            "double"
        } else {
            "single"
        };
        if let Some(meta_index) = pair_index.get(&pair_key).copied() {
            let meta = &mut pair_meta[meta_index];
            meta.edge_indexes.push(index);
            if line_type == "double" {
                meta.line_type = "double".to_string();
            }
        } else {
            let (left, right) = ordered_pair(&source, &target);
            pair_index.insert(pair_key.clone(), pair_meta.len());
            pair_meta.push(PairMeta {
                source: left,
                target: right,
                line_type: line_type.to_string(),
                edge_indexes: vec![index],
            });
        }
        if let Some(map) = neighbor_edge_map.get_mut(&source) {
            map.insert(target.clone(), Value::String(pair_key.clone()));
        }
        if let Some(map) = neighbor_edge_map.get_mut(&target) {
            map.insert(source.clone(), Value::String(pair_key));
        }
    }

    let components = connected_components(&node_ids, &adjacency);
    let mut component_by_node = HashMap::new();
    for (index, component) in components.iter().enumerate() {
        for id in component {
            component_by_node.insert(id.clone(), index);
        }
    }

    RoleGraphData {
        node_ids,
        node_row_ids,
        raw_node_count: node_rows.len(),
        node_by_id,
        adjacency,
        raw_degree_by_id,
        neighbor_edge_map,
        unique_undirected_edges: pair_meta,
        components,
        component_by_node,
    }
}

fn role_graph_json(graph: &RoleGraphData) -> Value {
    let mut component_by_node = Map::new();
    for id in &graph.node_ids {
        if let Some(index) = graph.component_by_node.get(id) {
            component_by_node.insert(id.clone(), json!(index));
        }
    }
    json!({
        "nodeIds": graph.node_ids.clone(),
        "adjacency": adjacency_json(&graph.node_ids, &graph.adjacency),
        "neighborCount": neighbor_count_json(&graph.node_ids, &graph.adjacency),
        "neighborEdgeMap": neighbor_edge_map_json(&graph.node_ids, &graph.neighbor_edge_map),
        "uniqueUndirectedEdges": pair_meta_json(graph.unique_undirected_edges.clone()),
        "components": graph.components.clone(),
        "componentByNode": component_by_node,
    })
}

fn project_seed_demotion(graph: &RoleGraphData, payload: &Value) -> Value {
    let options = payload.get("options").unwrap_or(payload);
    let seed_list = payload
        .get("seedCoreCandidates")
        .and_then(Value::as_array)
        .map(|rows| rows.iter().map(js_or_empty_string).collect::<Vec<_>>())
        .unwrap_or_default();
    let seed_ids = sort_stable_ids(seed_list)
        .into_iter()
        .filter(|id| graph.node_by_id.contains_key(id))
        .collect::<BTreeSet<_>>()
        .into_iter()
        .collect::<Vec<_>>();
    let seed_set = seed_ids.iter().cloned().collect::<BTreeSet<_>>();
    let mut history = clone_seed_core_history(options.get("seedHistory"));
    let mut core_set = BTreeSet::new();
    let mut demote_tags: BTreeMap<String, String> = BTreeMap::new();
    let mut seed_meta: BTreeMap<String, (f64, f64)> = BTreeMap::new();
    let mut seed_components: BTreeMap<usize, Vec<String>> = BTreeMap::new();

    for seed_id in &seed_ids {
        if let Some(component) = graph.component_by_node.get(seed_id) {
            seed_components
                .entry(*component)
                .or_default()
                .push(seed_id.clone());
        }
    }

    let bypass_weight = options
        .get("SEED_DEMOTION_BYPASS_WEIGHT")
        .and_then(js_number);
    let enable_seed_protection = js_truthy(options.get("ENABLE_SEED_PROTECTION"));
    for seed_id in &seed_ids {
        let deg = graph
            .adjacency
            .get(seed_id)
            .map(BTreeSet::len)
            .unwrap_or_default();
        let neighbors = graph
            .adjacency
            .get(seed_id)
            .map(|items| items.iter().cloned().collect::<Vec<_>>())
            .unwrap_or_default();
        let seed_weight = derive_seed_weight(graph.node_by_id.get(seed_id));
        let stable_score = history
            .get(seed_id)
            .map(|row| row.stable_score)
            .filter(|value| value.is_finite())
            .unwrap_or(0.0);
        seed_meta.insert(seed_id.clone(), (seed_weight, stable_score));

        let bypass = enable_seed_protection
            && bypass_weight.is_some_and(|weight| weight.is_finite() && weight > 0.0)
            && seed_weight >= bypass_weight.unwrap_or(0.0);
        if bypass {
            core_set.insert(seed_id.clone());
            continue;
        }
        if deg == 1 {
            demote_tags.insert(seed_id.clone(), "deg1".to_string());
            continue;
        }
        if deg == 2 {
            demote_tags.insert(seed_id.clone(), "deg2".to_string());
            continue;
        }
        let all_neighbor_seeds =
            !neighbors.is_empty() && neighbors.iter().all(|id| seed_set.contains(id));
        if !all_neighbor_seeds {
            core_set.insert(seed_id.clone());
            continue;
        }
        let intra_seed_degree = neighbors.iter().filter(|id| seed_set.contains(*id)).count();
        let component = graph.component_by_node.get(seed_id).copied();
        let comp_seed_size = component
            .and_then(|component| seed_components.get(&component).map(Vec::len))
            .unwrap_or_default();
        let desired_hub_degree = 3usize.max(((comp_seed_size as f64) * 0.65).ceil() as usize);
        if intra_seed_degree >= desired_hub_degree {
            core_set.insert(seed_id.clone());
            continue;
        }
        demote_tags.insert(seed_id.clone(), "coreOnly".to_string());
    }

    for ids in seed_components.values() {
        let pool = sort_stable_ids(ids.clone());
        let core_only_demoted = pool
            .iter()
            .filter(|id| demote_tags.get(*id).map(String::as_str) == Some("coreOnly"))
            .cloned()
            .collect::<Vec<_>>();
        if core_only_demoted.is_empty() {
            continue;
        }
        let kept_count = pool.iter().filter(|id| core_set.contains(*id)).count();
        let desired_keep = if pool.len() >= 9 {
            3
        } else if pool.len() >= 5 {
            2
        } else {
            1
        };
        if kept_count >= desired_keep {
            continue;
        }
        let mut ranked = core_only_demoted;
        ranked.sort_by(|a, b| compare_seed_restore_rank(graph, &seed_meta, a, b));
        let mut restored = 0usize;
        for id in ranked {
            if kept_count + restored >= desired_keep {
                break;
            }
            demote_tags.remove(&id);
            core_set.insert(id);
            restored += 1;
        }
    }

    for seed_id in &seed_ids {
        let old = history.get(seed_id).copied().unwrap_or(HistoryRow {
            total: 0.0,
            kept: 0.0,
            stable_score: 0.0,
        });
        let total = old.total.max(0.0) + 1.0;
        let kept = old.kept.max(0.0) + if core_set.contains(seed_id) { 1.0 } else { 0.0 };
        let stable_score = round_three(kept / total);
        history.insert(
            seed_id.clone(),
            HistoryRow {
                total,
                kept,
                stable_score,
            },
        );
        if let Some(meta) = seed_meta.get_mut(seed_id) {
            meta.1 = stable_score;
        }
    }

    json!({
        "coreIds": core_set.into_iter().collect::<Vec<_>>(),
        "demoteTags": string_map_json(demote_tags),
        "seedMeta": seed_meta_json(seed_meta),
        "nextSeedCoreHistory": seed_history_json(history),
    })
}

fn project_path_promotion(graph: &RoleGraphData, payload: &Value) -> Value {
    let options = payload.get("options").unwrap_or(payload);
    let core_ids = payload
        .get("coreIds")
        .and_then(Value::as_array)
        .map(|rows| rows.iter().map(js_or_empty_string).collect::<Vec<_>>())
        .unwrap_or_default();
    let core_set = sort_stable_ids(core_ids)
        .into_iter()
        .filter(|id| graph.node_by_id.contains_key(id))
        .collect::<BTreeSet<_>>();
    let max_hops = option_usize(options, "MAX_PROMOTION_HOPS", 4).max(2);
    let core_neighborhood_hops = option_usize(options, "CORE_NEIGHBORHOOD_HOPS", 2).max(1);
    let max_paths = option_usize(options, "MAX_SHORTEST_PATHS", 24).max(2);
    let max_pairs = option_usize(options, "MAX_PROMOTION_CORE_PAIRS", 320).max(4);
    let suppress_noisy = options
        .get("SUPPRESS_NOISY_PATH_PROMOTION")
        .map(|value| value != &Value::Bool(false))
        .unwrap_or(true);

    let mut promoted_adjacent = BTreeSet::new();
    let mut core_pairs = Vec::new();
    let mut rejected = Vec::new();
    let mut truncated = false;

    for component_node_ids in &graph.components {
        let comp_core_ids = sort_stable_ids(
            component_node_ids
                .iter()
                .filter(|id| core_set.contains(*id))
                .cloned()
                .collect(),
        );
        if comp_core_ids.len() < 2 {
            continue;
        }
        let core_dist = collect_core_distances(graph, &comp_core_ids);
        let mut pair_count = 0usize;
        for i in 0..comp_core_ids.len() {
            for j in (i + 1)..comp_core_ids.len() {
                let a = &comp_core_ids[i];
                let b = &comp_core_ids[j];
                pair_count += 1;
                if pair_count > max_pairs {
                    truncated = true;
                    continue;
                }
                let shortest = collect_shortest_paths(&graph.adjacency, a, b, max_hops, max_paths);
                if shortest.paths.is_empty() {
                    rejected.push(json!({
                        "pair": [a, b],
                        "reason": "unreachable_or_over_limit",
                    }));
                    continue;
                }
                if shortest.distance > max_hops {
                    rejected.push(json!({
                        "pair": [a, b],
                        "reason": "over_max_hops",
                        "distance": shortest.distance,
                        "maxHops": max_hops,
                    }));
                    continue;
                }
                let path_rows = shortest
                    .paths
                    .iter()
                    .map(|path| promotion_path_row(graph, &core_dist, path))
                    .collect::<Vec<_>>();
                let mut candidate_rows = path_rows
                    .iter()
                    .filter(|row| row.max_core_dist <= core_neighborhood_hops)
                    .cloned()
                    .collect::<Vec<_>>();
                if candidate_rows.is_empty() {
                    rejected.push(json!({
                        "pair": [a, b],
                        "reason": "out_of_core_neighborhood",
                        "coreNeighborhoodHops": core_neighborhood_hops,
                        "candidatePathCount": path_rows.len(),
                    }));
                    continue;
                }
                candidate_rows.sort_by(compare_promotion_path_row);
                let mut promoted = Vec::new();
                let mut basis = "best_single_path";
                if candidate_rows.len() > 1 {
                    let mut intersection = candidate_rows[0]
                        .mids
                        .iter()
                        .cloned()
                        .collect::<BTreeSet<_>>();
                    for row in candidate_rows.iter().skip(1) {
                        let row_set = row.mids.iter().cloned().collect::<BTreeSet<_>>();
                        intersection.retain(|id| row_set.contains(id));
                    }
                    if intersection.is_empty() {
                        promoted = sort_stable_ids(candidate_rows[0].mids.clone());
                        basis = "lowest_degree_shortest_path";
                    } else {
                        promoted = sort_stable_ids(intersection.into_iter().collect());
                        basis = "shortest_path_intersection";
                    }
                } else if let Some(row) = candidate_rows.first() {
                    promoted = sort_stable_ids(row.mids.clone());
                }

                let promoted_raw = promoted.clone();
                let mut dropped_noisy_nodes = Vec::new();
                if suppress_noisy && !promoted.is_empty() {
                    promoted.retain(|id| {
                        let keep = !is_noisy_promotion_node_id(id);
                        if !keep {
                            dropped_noisy_nodes.push(id.clone());
                        }
                        keep
                    });
                }
                for id in &promoted {
                    if !core_set.contains(id) {
                        promoted_adjacent.insert(id.clone());
                    }
                }
                core_pairs.push(json!({
                    "pair": [a, b],
                    "shortestHops": shortest.distance,
                    "candidatePathCount": candidate_rows.len(),
                    "neighborhoodFilteredPathCount": path_rows.len(),
                    "selectedBasis": basis,
                    "promotedNodes": promoted,
                    "promotedNodesRaw": promoted_raw,
                    "droppedNoisyNodes": dropped_noisy_nodes,
                }));
            }
        }
    }

    json!({
        "pathPromotedAdjacentIds": promoted_adjacent.into_iter().collect::<Vec<_>>(),
        "promotionReport": {
            "maxHops": max_hops,
            "corePairs": core_pairs,
            "rejected": rejected,
            "truncated": truncated,
        }
    })
}

fn project_role_resolution(graph: &RoleGraphData, payload: &Value) -> Value {
    let options = payload.get("options").unwrap_or(payload);
    let core_set = read_string_set(payload.get("coreIds"));
    let path_promoted_adjacent_set = read_string_set(payload.get("pathPromotedAdjacentIds"));
    let demote_tags = read_string_object(payload.get("demoteTags"));
    let seed_meta = read_seed_meta(options.get("seedMeta").or_else(|| payload.get("seedMeta")));
    let articulation = articulation_points(&graph.node_ids, &graph.adjacency);
    let mut roles_by_id: BTreeMap<String, String> = BTreeMap::new();
    let mut node_meta_by_id: BTreeMap<String, NodeRoleMeta> = BTreeMap::new();
    let mut leaf_entry_by_id: BTreeMap<String, String> = BTreeMap::new();
    let mut adjacent_set = BTreeSet::new();
    let mut leaf_set = BTreeSet::new();
    let mut bridge_set = BTreeSet::new();
    let mut hub_set = BTreeSet::new();

    for id in &graph.node_ids {
        let is_core = core_set.contains(id);
        let seed = seed_meta.get(id).cloned().unwrap_or(SeedMetaInput {
            is_seed_selected: false,
            seed_weight: 0.0,
            seed_stability_score: 0.0,
        });
        let demote_reason = demote_tags.get(id).cloned();
        let mut meta = NodeRoleMeta {
            id: id.clone(),
            role: if is_core {
                "core".to_string()
            } else {
                String::new()
            },
            seed_core_candidate: seed.is_seed_selected,
            is_seed_selected: seed.is_seed_selected,
            demote_reason,
            is_path_promoted_adjacent: false,
            is_bridge_adjacent: false,
            is_hub_adjacent: false,
            layout_anchor_for_cluster: false,
            why: Vec::new(),
            weight: seed.seed_weight,
            seed_weight: seed.seed_weight,
            seed_stability_score: seed.seed_stability_score,
        };
        if is_core {
            roles_by_id.insert(id.clone(), "core".to_string());
            meta.role = "core".to_string();
            if seed.is_seed_selected {
                meta.why.push("seed_core_kept".to_string());
            }
        }
        node_meta_by_id.insert(id.clone(), meta);
    }

    for id in &graph.node_ids {
        if core_set.contains(id) {
            continue;
        }
        let neighbors = sorted_neighbors(&graph.adjacency, id);
        let core_neighbors = neighbors
            .iter()
            .filter(|nid| core_set.contains(*nid))
            .count();
        let non_core_neighbors = neighbors
            .iter()
            .filter(|nid| !core_set.contains(*nid))
            .count();
        let degree = graph
            .adjacency
            .get(id)
            .map(BTreeSet::len)
            .unwrap_or_default();
        if let Some(meta) = node_meta_by_id.get_mut(id) {
            if path_promoted_adjacent_set.contains(id) {
                adjacent_set.insert(id.clone());
                meta.is_path_promoted_adjacent = true;
                meta.why.push("path_promoted_adjacent".to_string());
            }
            let demote = demote_tags.get(id).map(String::as_str).unwrap_or("");
            if demote == "deg2" || demote == "coreOnly" {
                adjacent_set.insert(id.clone());
                meta.why.push(if demote == "deg2" {
                    "demote_deg2".to_string()
                } else {
                    "demote_coreOnly".to_string()
                });
            }
            if core_neighbors >= 2 {
                adjacent_set.insert(id.clone());
                meta.why.push("adjacent_multi_core".to_string());
            } else if core_neighbors == 1 && non_core_neighbors >= 1 {
                adjacent_set.insert(id.clone());
                meta.why.push("adjacent_core_plus_noncore".to_string());
            }
            if articulation.contains(id) && degree >= 2 {
                adjacent_set.insert(id.clone());
                bridge_set.insert(id.clone());
            }
            if degree >= 5 {
                adjacent_set.insert(id.clone());
                hub_set.insert(id.clone());
            }
        }
    }

    for id in &graph.node_ids {
        if core_set.contains(id) || adjacent_set.contains(id) {
            continue;
        }
        let degree = graph
            .adjacency
            .get(id)
            .map(BTreeSet::len)
            .unwrap_or_default();
        let neighbors = sorted_neighbors(&graph.adjacency, id);
        let skeleton_neighbors = neighbors
            .iter()
            .filter(|nid| core_set.contains(*nid) || adjacent_set.contains(*nid))
            .cloned()
            .collect::<Vec<_>>();
        let demote = demote_tags.get(id).map(String::as_str).unwrap_or("");
        if let Some(meta) = node_meta_by_id.get_mut(id) {
            if demote == "deg1" && !path_promoted_adjacent_set.contains(id) {
                leaf_set.insert(id.clone());
                meta.why.push("demote_deg1".to_string());
                if !skeleton_neighbors.is_empty() {
                    leaf_entry_by_id
                        .insert(id.clone(), sort_stable_ids(skeleton_neighbors)[0].clone());
                }
                continue;
            }
            if skeleton_neighbors.len() == 1 {
                leaf_set.insert(id.clone());
                leaf_entry_by_id.insert(id.clone(), skeleton_neighbors[0].clone());
                meta.why.push("leaf_single_entry".to_string());
                continue;
            }
            if degree <= 1 {
                leaf_set.insert(id.clone());
                meta.why.push("leaf_single_entry".to_string());
                if !skeleton_neighbors.is_empty() {
                    leaf_entry_by_id
                        .insert(id.clone(), sort_stable_ids(skeleton_neighbors)[0].clone());
                }
                continue;
            }
            if degree <= 2 && skeleton_neighbors.len() <= 1 && !articulation.contains(id) {
                leaf_set.insert(id.clone());
                meta.why.push("leaf_single_entry".to_string());
                if !skeleton_neighbors.is_empty() {
                    leaf_entry_by_id
                        .insert(id.clone(), sort_stable_ids(skeleton_neighbors)[0].clone());
                }
                continue;
            }
            adjacent_set.insert(id.clone());
            meta.why.push("adjacent_structural".to_string());
        }
    }

    for id in &graph.node_ids {
        let role = if core_set.contains(id) {
            "core"
        } else if adjacent_set.contains(id) {
            "adjacent"
        } else if leaf_set.contains(id) {
            "leaf"
        } else {
            adjacent_set.insert(id.clone());
            "adjacent"
        };
        roles_by_id.insert(id.clone(), role.to_string());
        if let Some(meta) = node_meta_by_id.get_mut(id) {
            meta.role = role.to_string();
            if bridge_set.contains(id) {
                meta.is_bridge_adjacent = true;
                meta.why.push("bridge_adjacent".to_string());
            }
            if hub_set.contains(id) {
                meta.is_hub_adjacent = true;
                meta.why.push("hub_adjacent".to_string());
            }
            if meta.why.is_empty() && role == "leaf" {
                meta.why.push("leaf_single_entry".to_string());
            }
        }
    }

    json!({
        "rolesById": string_map_json(roles_by_id),
        "nodeMetaById": node_role_meta_json(node_meta_by_id),
        "leafEntryById": string_map_json(leaf_entry_by_id),
        "coreIds": sort_stable_ids(core_set.into_iter().collect()),
        "adjacentIds": adjacent_set.into_iter().collect::<Vec<_>>(),
        "leafIds": leaf_set.into_iter().collect::<Vec<_>>(),
        "pathPromotedAdjacentIds": sort_stable_ids(path_promoted_adjacent_set.into_iter().collect()),
        "bridgeAdjacentIds": bridge_set.into_iter().collect::<Vec<_>>(),
        "hubAdjacentIds": hub_set.into_iter().collect::<Vec<_>>(),
    })
}

fn project_cluster_detection(graph: &RoleGraphData, roles: Option<&Value>) -> Value {
    let roles_by_id = read_string_object(roles.and_then(|value| value.get("rolesById")));
    let mut node_meta_by_id = read_object_map(roles.and_then(|value| value.get("nodeMetaById")));
    let mut clusters = Vec::new();
    let mut node_cluster_by_id: BTreeMap<String, String> = BTreeMap::new();

    for (component_index, component_node_ids) in graph.components.iter().enumerate() {
        let comp_ids = sort_stable_ids(component_node_ids.clone());
        let skeleton_ids = comp_ids
            .iter()
            .filter(|id| {
                matches!(
                    roles_by_id
                        .get(*id)
                        .map(|role| role.to_lowercase())
                        .as_deref(),
                    Some("core" | "adjacent")
                )
            })
            .cloned()
            .collect::<Vec<_>>();
        let skeleton_set = skeleton_ids.iter().cloned().collect::<BTreeSet<_>>();
        let mut skeleton_adj: HashMap<String, BTreeSet<String>> = HashMap::new();
        for id in &skeleton_ids {
            let neighbors = graph
                .adjacency
                .get(id)
                .map(|items| {
                    items
                        .iter()
                        .filter(|nid| skeleton_set.contains(*nid))
                        .cloned()
                        .collect::<BTreeSet<_>>()
                })
                .unwrap_or_default();
            skeleton_adj.insert(id.clone(), neighbors);
        }

        let bridge_edges = bridge_edges(&skeleton_ids, &skeleton_adj);
        let mut reduced_adj: HashMap<String, BTreeSet<String>> = HashMap::new();
        for id in &skeleton_ids {
            reduced_adj.insert(id.clone(), BTreeSet::new());
        }
        for id in &skeleton_ids {
            for nid in sorted_neighbors(&skeleton_adj, id) {
                let key = role_pair_key(id, &nid);
                let id_degree = skeleton_adj.get(id).map(BTreeSet::len).unwrap_or_default();
                let nid_degree = skeleton_adj
                    .get(&nid)
                    .map(BTreeSet::len)
                    .unwrap_or_default();
                let should_cut = bridge_edges.contains(&key) && id_degree >= 2 && nid_degree >= 2;
                if should_cut {
                    continue;
                }
                reduced_adj.entry(id.clone()).or_default().insert(nid);
            }
        }

        let mut communities = if skeleton_ids.is_empty() {
            Vec::new()
        } else {
            connected_components(&skeleton_ids, &reduced_adj)
        };
        if communities.is_empty() {
            communities = vec![comp_ids.clone()];
        }

        let mut skeleton_community_by_id: BTreeMap<String, usize> = BTreeMap::new();
        for (index, community) in communities.iter().enumerate() {
            for id in community {
                skeleton_community_by_id.insert(id.clone(), index);
            }
        }

        for id in &skeleton_ids {
            if roles_by_id.get(id).map(String::as_str) != Some("adjacent") {
                continue;
            }
            let mut neighbor_communities = BTreeSet::new();
            for nid in sorted_neighbors(&graph.adjacency, id) {
                if let Some(community) = skeleton_community_by_id.get(&nid) {
                    neighbor_communities.insert(*community);
                }
            }
            if neighbor_communities.len() >= 2 {
                set_node_meta_bool(&mut node_meta_by_id, id, "isBridgeAdjacent", true);
                push_node_meta_why_once(&mut node_meta_by_id, id, "bridge_adjacent");
            }
        }

        let mut local_assign: BTreeMap<String, usize> = BTreeMap::new();
        for (index, community) in communities.iter().enumerate() {
            for id in community {
                local_assign.insert(id.clone(), index);
            }
        }

        for node_id in comp_ids
            .iter()
            .filter(|id| !local_assign.contains_key(*id))
            .cloned()
            .collect::<Vec<_>>()
        {
            let mut chosen = None;
            let mut seen = BTreeSet::from([node_id.clone()]);
            let mut queue = VecDeque::from([node_id.clone()]);
            while let Some(current) = queue.pop_front() {
                if chosen.is_some() {
                    break;
                }
                for nid in sorted_neighbors(&graph.adjacency, &current) {
                    if seen.contains(&nid) {
                        continue;
                    }
                    seen.insert(nid.clone());
                    if let Some(community) = local_assign.get(&nid).copied() {
                        chosen = Some(community);
                        break;
                    }
                    queue.push_back(nid);
                }
            }
            local_assign.insert(node_id, chosen.unwrap_or(0));
        }

        let mut cluster_node_map: BTreeMap<usize, Vec<String>> = BTreeMap::new();
        for id in &comp_ids {
            let bucket = local_assign.get(id).copied().unwrap_or(0);
            cluster_node_map.entry(bucket).or_default().push(id.clone());
        }

        for (local_id, ids) in cluster_node_map {
            let node_ids = sort_stable_ids(ids);
            let node_set = node_ids.iter().cloned().collect::<BTreeSet<_>>();
            let core_ids = sort_stable_ids(
                node_ids
                    .iter()
                    .filter(|id| roles_by_id.get(*id).map(String::as_str) == Some("core"))
                    .cloned()
                    .collect(),
            );
            let adjacent_ids = sort_stable_ids(
                node_ids
                    .iter()
                    .filter(|id| roles_by_id.get(*id).map(String::as_str) == Some("adjacent"))
                    .cloned()
                    .collect(),
            );
            let leaf_ids = sort_stable_ids(
                node_ids
                    .iter()
                    .filter(|id| roles_by_id.get(*id).map(String::as_str) == Some("leaf"))
                    .cloned()
                    .collect(),
            );
            let cluster_id = format!("cluster-{component_index}-{local_id}");
            let mut local_adjacency = Map::new();
            for id in &node_ids {
                local_adjacency.insert(
                    id.clone(),
                    Value::Array(
                        sorted_neighbors(&graph.adjacency, id)
                            .into_iter()
                            .filter(|nid| node_set.contains(nid))
                            .map(Value::String)
                            .collect(),
                    ),
                );
            }
            let edge_count = graph
                .unique_undirected_edges
                .iter()
                .filter(|edge| node_set.contains(&edge.source) && node_set.contains(&edge.target))
                .count();
            clusters.push(json!({
                "clusterId": cluster_id,
                "componentId": component_index,
                "nodeIds": node_ids.clone(),
                "coreIds": core_ids.clone(),
                "adjacentIds": adjacent_ids,
                "leafIds": leaf_ids,
                "layoutAnchorIds": [],
                "layoutCase": "general",
                "bbox": null,
                "hasCore": !core_ids.is_empty(),
                "edgeCount": edge_count,
                "localAdjacency": Value::Object(local_adjacency),
            }));
            for id in &node_ids {
                node_cluster_by_id.insert(id.clone(), cluster_id.clone());
                set_node_meta_string(&mut node_meta_by_id, id, "clusterId", &cluster_id);
            }
        }
    }

    json!({
        "clusters": clusters,
        "nodeClusterById": string_map_json(node_cluster_by_id),
        "nodeMetaById": object_map_json(node_meta_by_id),
    })
}

fn preferred_core_ids_json(
    graph: &RoleGraphData,
    role_result: &Value,
    focus_id: Option<&Value>,
) -> Value {
    let roles_by_id = read_string_object(role_result.get("rolesById"));
    let core_ids = graph
        .node_row_ids
        .iter()
        .filter(|id| {
            roles_by_id
                .get(*id)
                .map(|role| role.eq_ignore_ascii_case("core"))
                .unwrap_or(false)
        })
        .cloned()
        .collect::<Vec<_>>();
    if !core_ids.is_empty() {
        return Value::Array(
            sort_stable_ids(core_ids)
                .into_iter()
                .map(Value::String)
                .collect(),
        );
    }

    let focus = focus_id
        .map(js_or_empty_string)
        .unwrap_or_default()
        .trim()
        .to_string();
    if !focus.is_empty() && graph.node_row_ids.iter().any(|id| id == &focus) {
        return Value::Array(vec![Value::String(focus)]);
    }

    let mut rows = graph.node_row_ids.clone();
    rows.sort_by(|a, b| {
        let degree_order = graph
            .raw_degree_by_id
            .get(b)
            .copied()
            .unwrap_or_default()
            .cmp(&graph.raw_degree_by_id.get(a).copied().unwrap_or_default());
        if degree_order == Ordering::Equal {
            a.cmp(b)
        } else {
            degree_order
        }
    });
    Value::Array(
        rows.into_iter()
            .take(3)
            .filter(|id| !id.is_empty())
            .map(Value::String)
            .collect(),
    )
}

fn label_policy_json(graph: &RoleGraphData, role_result: &Value, cluster_result: &Value) -> Value {
    let roles_by_id = read_string_object(role_result.get("rolesById"));
    let total = graph.raw_node_count;
    let leaf_count = graph
        .node_row_ids
        .iter()
        .filter(|id| {
            roles_by_id
                .get(*id)
                .map(|role| role.eq_ignore_ascii_case("leaf"))
                .unwrap_or(false)
        })
        .count();
    let adjacent_count = graph
        .node_row_ids
        .iter()
        .filter(|id| {
            roles_by_id
                .get(*id)
                .map(|role| role.eq_ignore_ascii_case("adjacent"))
                .unwrap_or(false)
        })
        .count();
    let largest_cluster_node_count = cluster_result
        .get("clusters")
        .and_then(Value::as_array)
        .map(|clusters| {
            clusters
                .iter()
                .filter_map(|cluster| cluster.get("nodeIds").and_then(Value::as_array))
                .map(Vec::len)
                .max()
                .unwrap_or(0)
        })
        .unwrap_or(0);
    let leaf_ratio = if total > 0 {
        round_three(leaf_count as f64 / total as f64)
    } else {
        0.0
    };
    json!({
        "enabled": false,
        "reason": "disabled_by_user",
        "totalNodes": total,
        "leafCount": leaf_count,
        "adjacentCount": adjacent_count,
        "leafRatio": number_json(leaf_ratio),
        "largestClusterNodeCount": largest_cluster_node_count,
        "keepLeafLabels": leaf_count,
        "hiddenLeafLabels": 0,
        "keepAdjacentLabels": adjacent_count,
        "hiddenAdjacentLabels": 0,
        "keepLeafSample": [],
        "keepAdjacentSample": [],
    })
}

#[derive(Clone)]
struct ShortestPaths {
    distance: usize,
    paths: Vec<Vec<String>>,
}

#[derive(Clone)]
struct PromotionPathRow {
    mids: Vec<String>,
    degree_sum: usize,
    max_core_dist: usize,
    path_key: String,
}

fn collect_core_distances(graph: &RoleGraphData, core_ids: &[String]) -> HashMap<String, usize> {
    let mut dist = HashMap::new();
    let mut queue = VecDeque::new();
    for id in core_ids {
        dist.insert(id.clone(), 0);
        queue.push_back(id.clone());
    }
    while let Some(current) = queue.pop_front() {
        let cur_dist = *dist.get(&current).unwrap_or(&0);
        for next in sorted_neighbors(&graph.adjacency, &current) {
            if dist.contains_key(&next) {
                continue;
            }
            dist.insert(next.clone(), cur_dist + 1);
            queue.push_back(next);
        }
    }
    dist
}

fn collect_shortest_paths(
    adjacency: &HashMap<String, BTreeSet<String>>,
    start_id: &str,
    end_id: &str,
    max_hops: usize,
    max_paths: usize,
) -> ShortestPaths {
    if start_id.is_empty() || end_id.is_empty() || start_id == end_id {
        return ShortestPaths {
            distance: 0,
            paths: Vec::new(),
        };
    }
    if !adjacency.contains_key(start_id) || !adjacency.contains_key(end_id) {
        return ShortestPaths {
            distance: 0,
            paths: Vec::new(),
        };
    }
    let mut dist: HashMap<String, usize> = HashMap::new();
    let mut parents: HashMap<String, Vec<String>> = HashMap::new();
    let mut queue = VecDeque::from([start_id.to_string()]);
    dist.insert(start_id.to_string(), 0);
    parents.insert(start_id.to_string(), Vec::new());
    while let Some(current) = queue.pop_front() {
        let cur_dist = *dist.get(&current).unwrap_or(&0);
        if cur_dist >= max_hops {
            continue;
        }
        for next in sorted_neighbors(adjacency, &current) {
            let next_dist = cur_dist + 1;
            if next_dist > max_hops {
                continue;
            }
            if !dist.contains_key(&next) {
                dist.insert(next.clone(), next_dist);
                parents.insert(next.clone(), vec![current.clone()]);
                queue.push_back(next);
                continue;
            }
            if dist.get(&next).copied() == Some(next_dist) {
                parents.entry(next).or_default().push(current.clone());
            }
        }
    }
    let Some(distance) = dist.get(end_id).copied() else {
        return ShortestPaths {
            distance: 0,
            paths: Vec::new(),
        };
    };

    let mut paths = Vec::new();
    let mut stack = vec![vec![end_id.to_string()]];
    while let Some(path) = stack.pop() {
        if paths.len() >= max_paths {
            break;
        }
        let head = path.last().cloned().unwrap_or_default();
        if head == start_id {
            let mut full_path = path.clone();
            full_path.reverse();
            paths.push(full_path);
            continue;
        }
        for parent in sort_stable_ids(parents.get(&head).cloned().unwrap_or_default()) {
            if path.contains(&parent) {
                continue;
            }
            let mut next_path = path.clone();
            next_path.push(parent);
            stack.push(next_path);
        }
    }

    ShortestPaths { distance, paths }
}

fn promotion_path_row(
    graph: &RoleGraphData,
    core_dist: &HashMap<String, usize>,
    path: &[String],
) -> PromotionPathRow {
    let mids = if path.len() > 2 {
        path[1..path.len() - 1].to_vec()
    } else {
        Vec::new()
    };
    let degree_sum = mids
        .iter()
        .map(|id| {
            graph
                .adjacency
                .get(id)
                .map(BTreeSet::len)
                .unwrap_or_default()
        })
        .sum();
    let max_core_dist = mids
        .iter()
        .map(|id| core_dist.get(id).copied().unwrap_or(usize::MAX))
        .max()
        .unwrap_or(0);
    let path_key = mids.join("|");
    PromotionPathRow {
        mids,
        degree_sum,
        max_core_dist,
        path_key,
    }
}

fn compare_promotion_path_row(a: &PromotionPathRow, b: &PromotionPathRow) -> Ordering {
    match a.degree_sum.cmp(&b.degree_sum) {
        Ordering::Equal => {}
        ordering => return ordering,
    }
    match a.mids.len().cmp(&b.mids.len()) {
        Ordering::Equal => a.path_key.cmp(&b.path_key),
        ordering => ordering,
    }
}

fn is_noisy_promotion_node_id(id: &str) -> bool {
    let text = id.trim();
    if text.is_empty() || is_placeholder_node_id(text) || text == "\\N" {
        return true;
    }
    let lower = text.to_lowercase();
    text.contains("__unknown_cp__name::__empty__")
        || lower == "unknown"
        || lower.starts_with("unknown_")
}

fn is_placeholder_node_id(value: &str) -> bool {
    placeholder_kind_from_token(value).is_some()
}

fn placeholder_kind_from_token(value: &str) -> Option<&str> {
    let remainder = value.strip_prefix("__cp_placeholder__::")?;
    let kind = remainder
        .split("::name::")
        .next()
        .unwrap_or_default()
        .trim();
    if matches!(
        kind,
        "db_null"
            | "empty"
            | "slash_n"
            | "dash"
            | "emdash"
            | "fw_dash"
            | "literal_null"
            | "literal_none"
            | "literal_nan"
    ) {
        Some(kind)
    } else {
        None
    }
}

fn option_usize(options: &Value, key: &str, default_value: usize) -> usize {
    options
        .get(key)
        .and_then(js_number)
        .filter(|value| value.is_finite())
        .map(|value| value as usize)
        .filter(|value| *value > 0)
        .unwrap_or(default_value)
}

fn compare_seed_restore_rank(
    graph: &RoleGraphData,
    seed_meta: &BTreeMap<String, (f64, f64)>,
    a: &str,
    b: &str,
) -> Ordering {
    let wa = seed_meta.get(a).map(|row| row.0).unwrap_or(0.0);
    let wb = seed_meta.get(b).map(|row| row.0).unwrap_or(0.0);
    match wb.partial_cmp(&wa).unwrap_or(Ordering::Equal) {
        Ordering::Equal => {}
        ordering => return ordering,
    }
    let da = graph
        .adjacency
        .get(a)
        .map(BTreeSet::len)
        .unwrap_or_default();
    let db = graph
        .adjacency
        .get(b)
        .map(BTreeSet::len)
        .unwrap_or_default();
    match db.cmp(&da) {
        Ordering::Equal => a.cmp(b),
        ordering => ordering,
    }
}

fn clone_seed_core_history(value: Option<&Value>) -> BTreeMap<String, HistoryRow> {
    let mut out = BTreeMap::new();
    let Some(Value::Object(rows)) = value else {
        return out;
    };
    for (seed_id, row) in rows {
        let id = seed_id.trim().to_string();
        if id.is_empty() || !row.is_object() {
            continue;
        }
        let total = row.get("total").and_then(js_number).unwrap_or(0.0).max(0.0);
        let kept = row.get("kept").and_then(js_number).unwrap_or(0.0).max(0.0);
        let stable_score = row
            .get("stableScore")
            .and_then(js_number)
            .filter(|score| score.is_finite())
            .unwrap_or_else(|| {
                if total > 0.0 {
                    round_three(kept / total)
                } else {
                    0.0
                }
            });
        out.insert(
            id,
            HistoryRow {
                total,
                kept,
                stable_score,
            },
        );
    }
    out
}

fn derive_seed_weight(node: Option<&Value>) -> f64 {
    let Some(Value::Object(row)) = node else {
        return 0.0;
    };
    for key in ["total_amount", "total_count", "weight"] {
        let multiplier = if key == "total_count" { 1000.0 } else { 1.0 };
        let value = row
            .get(key)
            .and_then(js_number)
            .map(f64::abs)
            .unwrap_or(0.0);
        if value.is_finite() && value > 0.0 {
            return value * multiplier;
        }
    }
    0.0
}

fn string_map_json(rows: BTreeMap<String, String>) -> Value {
    let mut out = Map::new();
    for (key, value) in rows {
        out.insert(key, Value::String(value));
    }
    Value::Object(out)
}

fn read_string_set(value: Option<&Value>) -> BTreeSet<String> {
    value
        .and_then(Value::as_array)
        .map(|rows| rows.iter().map(js_or_empty_string).collect::<Vec<_>>())
        .map(sort_stable_ids)
        .unwrap_or_default()
        .into_iter()
        .collect()
}

fn read_string_object(value: Option<&Value>) -> BTreeMap<String, String> {
    let mut out = BTreeMap::new();
    let Some(Value::Object(rows)) = value else {
        return out;
    };
    for (key, row) in rows {
        let id = key.trim().to_string();
        if id.is_empty() {
            continue;
        }
        out.insert(id, js_or_empty_string(row));
    }
    out
}

fn read_object_map(value: Option<&Value>) -> BTreeMap<String, Value> {
    let mut out = BTreeMap::new();
    let Some(Value::Object(rows)) = value else {
        return out;
    };
    for (key, row) in rows {
        let id = key.trim().to_string();
        if id.is_empty() || !row.is_object() {
            continue;
        }
        out.insert(id, row.clone());
    }
    out
}

fn object_map_json(rows: BTreeMap<String, Value>) -> Value {
    let mut out = Map::new();
    for (key, value) in rows {
        out.insert(key, value);
    }
    Value::Object(out)
}

fn ensure_node_meta_object<'a>(
    rows: &'a mut BTreeMap<String, Value>,
    id: &str,
) -> &'a mut Map<String, Value> {
    let entry = rows
        .entry(id.to_string())
        .or_insert_with(|| json!({"id": id, "why": []}));
    if !entry.is_object() {
        *entry = json!({"id": id, "why": []});
    }
    let object = entry.as_object_mut().expect("node meta object");
    object
        .entry("id".to_string())
        .or_insert_with(|| Value::String(id.to_string()));
    object
        .entry("why".to_string())
        .or_insert_with(|| Value::Array(Vec::new()));
    object
}

fn set_node_meta_bool(rows: &mut BTreeMap<String, Value>, id: &str, key: &str, value: bool) {
    ensure_node_meta_object(rows, id).insert(key.to_string(), Value::Bool(value));
}

fn set_node_meta_string(rows: &mut BTreeMap<String, Value>, id: &str, key: &str, value: &str) {
    ensure_node_meta_object(rows, id).insert(key.to_string(), Value::String(value.to_string()));
}

fn push_node_meta_why_once(rows: &mut BTreeMap<String, Value>, id: &str, reason: &str) {
    let object = ensure_node_meta_object(rows, id);
    let why = object
        .entry("why".to_string())
        .or_insert_with(|| Value::Array(Vec::new()));
    if !why.is_array() {
        *why = Value::Array(Vec::new());
    }
    let Some(items) = why.as_array_mut() else {
        return;
    };
    if !items.iter().any(|item| item.as_str() == Some(reason)) {
        items.push(Value::String(reason.to_string()));
    }
}

fn read_seed_meta(value: Option<&Value>) -> BTreeMap<String, SeedMetaInput> {
    let mut out = BTreeMap::new();
    let Some(Value::Object(rows)) = value else {
        return out;
    };
    for (key, row) in rows {
        let id = key.trim().to_string();
        if id.is_empty() {
            continue;
        }
        let Some(meta) = row.as_object() else {
            continue;
        };
        out.insert(
            id,
            SeedMetaInput {
                is_seed_selected: js_truthy(meta.get("isSeedSelected")),
                seed_weight: meta.get("seedWeight").and_then(js_number).unwrap_or(0.0),
                seed_stability_score: meta
                    .get("seedStabilityScore")
                    .and_then(js_number)
                    .unwrap_or(0.0),
            },
        );
    }
    out
}

fn node_role_meta_json(rows: BTreeMap<String, NodeRoleMeta>) -> Value {
    let mut out = Map::new();
    for (id, row) in rows {
        out.insert(
            id,
            json!({
                "id": row.id,
                "role": row.role,
                "seedCoreCandidate": row.seed_core_candidate,
                "isSeedSelected": row.is_seed_selected,
                "demoteReason": row.demote_reason,
                "isPathPromotedAdjacent": row.is_path_promoted_adjacent,
                "isBridgeAdjacent": row.is_bridge_adjacent,
                "isHubAdjacent": row.is_hub_adjacent,
                "layoutAnchorForCluster": row.layout_anchor_for_cluster,
                "why": row.why,
                "weight": number_json(row.weight),
                "seedWeight": number_json(row.seed_weight),
                "seedStabilityScore": number_json(row.seed_stability_score),
            }),
        );
    }
    Value::Object(out)
}

fn seed_meta_json(rows: BTreeMap<String, (f64, f64)>) -> Value {
    let mut out = Map::new();
    for (id, (seed_weight, seed_stability_score)) in rows {
        out.insert(
            id,
            json!({
                "isSeedSelected": true,
                "seedWeight": number_json(seed_weight),
                "seedStabilityScore": number_json(seed_stability_score),
            }),
        );
    }
    Value::Object(out)
}

fn seed_history_json(rows: BTreeMap<String, HistoryRow>) -> Value {
    let mut out = Map::new();
    for (id, row) in rows {
        out.insert(
            id,
            json!({
                "total": number_json(row.total),
                "kept": number_json(row.kept),
                "stableScore": number_json(row.stable_score),
            }),
        );
    }
    Value::Object(out)
}

fn number_json(value: f64) -> Value {
    if value.is_finite()
        && value.fract() == 0.0
        && value >= i64::MIN as f64
        && value <= i64::MAX as f64
    {
        json!(value as i64)
    } else {
        json!(value)
    }
}

fn round_three(value: f64) -> f64 {
    (value * 1000.0).round() / 1000.0
}

fn js_truthy(value: Option<&Value>) -> bool {
    match value {
        None | Some(Value::Null) => false,
        Some(Value::Bool(flag)) => *flag,
        Some(Value::Number(number)) => number.as_f64().is_some_and(|value| value != 0.0),
        Some(Value::String(text)) => !text.is_empty(),
        Some(Value::Array(_)) | Some(Value::Object(_)) => true,
    }
}

fn js_number(value: &Value) -> Option<f64> {
    let number = match value {
        Value::Null => 0.0,
        Value::Bool(flag) => {
            if *flag {
                1.0
            } else {
                0.0
            }
        }
        Value::Number(number) => number.as_f64()?,
        Value::String(text) => {
            let trimmed = text.trim();
            if trimmed.is_empty() {
                0.0
            } else {
                trimmed.parse::<f64>().ok()?
            }
        }
        Value::Array(values) if values.is_empty() => 0.0,
        Value::Array(values) if values.len() == 1 => js_number(&values[0])?,
        Value::Array(_) | Value::Object(_) => return None,
    };

    if number.is_finite() {
        Some(number)
    } else {
        None
    }
}

fn project_traversal(payload: &Value) -> Value {
    let node_ids = payload
        .get("nodeIds")
        .and_then(Value::as_array)
        .map(|rows| rows.iter().map(js_or_empty_string).collect::<Vec<_>>())
        .unwrap_or_default();
    let adjacency = read_adjacency(payload.get("adjacency"));
    json!({
        "components": connected_components(&node_ids, &adjacency),
        "bridgeEdges": sort_stable_ids(bridge_edges(&node_ids, &adjacency).into_iter().collect()),
        "articulationPoints": sort_stable_ids(articulation_points(&node_ids, &adjacency).into_iter().collect()),
    })
}

fn adjacency_json(node_ids: &[String], adjacency: &HashMap<String, BTreeSet<String>>) -> Value {
    let mut out = Map::new();
    for id in node_ids {
        out.insert(
            id.clone(),
            Value::Array(
                sort_stable_ids(
                    adjacency
                        .get(id)
                        .map(|set| set.iter().cloned().collect())
                        .unwrap_or_default(),
                )
                .into_iter()
                .map(Value::String)
                .collect(),
            ),
        );
    }
    Value::Object(out)
}

fn neighbor_count_json(
    node_ids: &[String],
    adjacency: &HashMap<String, BTreeSet<String>>,
) -> Value {
    let mut out = Map::new();
    for id in node_ids {
        out.insert(
            id.clone(),
            json!(adjacency.get(id).map(BTreeSet::len).unwrap_or(0)),
        );
    }
    Value::Object(out)
}

fn neighbor_edge_map_json(
    node_ids: &[String],
    neighbor_edge_map: &HashMap<String, Map<String, Value>>,
) -> Value {
    let mut out = Map::new();
    for id in node_ids {
        out.insert(
            id.clone(),
            Value::Object(neighbor_edge_map.get(id).cloned().unwrap_or_default()),
        );
    }
    Value::Object(out)
}

fn pair_meta_json(pair_meta: Vec<PairMeta>) -> Value {
    Value::Array(
        pair_meta
            .into_iter()
            .map(|meta| {
                json!({
                    "source": meta.source,
                    "target": meta.target,
                    "lineType": meta.line_type,
                    "edgeIndexes": meta.edge_indexes,
                })
            })
            .collect(),
    )
}

fn read_adjacency(value: Option<&Value>) -> HashMap<String, BTreeSet<String>> {
    let mut adjacency = HashMap::new();
    let Some(Value::Object(rows)) = value else {
        return adjacency;
    };
    for (key, row) in rows {
        let id = key.trim().to_string();
        if id.is_empty() {
            continue;
        }
        let neighbors = row
            .as_array()
            .map(|items| {
                items
                    .iter()
                    .map(js_or_empty_string)
                    .map(|item| item.trim().to_string())
                    .filter(|item| !item.is_empty())
                    .collect::<BTreeSet<_>>()
            })
            .unwrap_or_default();
        adjacency.insert(id, neighbors);
    }
    adjacency
}

fn connected_components(
    node_ids: &[String],
    adjacency: &HashMap<String, BTreeSet<String>>,
) -> Vec<Vec<String>> {
    let mut components = Vec::new();
    let mut visited = BTreeSet::new();
    for id in sort_stable_ids(node_ids.to_vec()) {
        if id.is_empty() || visited.contains(&id) {
            continue;
        }
        let mut queue = VecDeque::from([id.clone()]);
        let mut component = Vec::new();
        visited.insert(id);
        while let Some(current) = queue.pop_front() {
            component.push(current.clone());
            for next in sorted_neighbors(adjacency, &current) {
                if visited.contains(&next) {
                    continue;
                }
                visited.insert(next.clone());
                queue.push_back(next);
            }
        }
        components.push(component);
    }
    components
}

fn articulation_points(
    node_ids: &[String],
    adjacency: &HashMap<String, BTreeSet<String>>,
) -> BTreeSet<String> {
    struct DfsState {
        disc: HashMap<String, usize>,
        low: HashMap<String, usize>,
        parent: HashMap<String, String>,
        points: BTreeSet<String>,
        timer: usize,
    }

    fn dfs(node: &str, adjacency: &HashMap<String, BTreeSet<String>>, state: &mut DfsState) {
        state.disc.insert(node.to_string(), state.timer);
        state.low.insert(node.to_string(), state.timer);
        state.timer += 1;
        let mut child_count = 0usize;
        for next in sorted_neighbors(adjacency, node) {
            if !state.disc.contains_key(&next) {
                state.parent.insert(next.clone(), node.to_string());
                child_count += 1;
                dfs(&next, adjacency, state);
                let next_low = *state.low.get(&next).unwrap_or(&0);
                let node_low = *state.low.get(node).unwrap_or(&0);
                state.low.insert(node.to_string(), node_low.min(next_low));
                let parent = state.parent.get(node);
                let node_disc = *state.disc.get(node).unwrap_or(&0);
                if (parent.is_none() && child_count > 1)
                    || (parent.is_some() && next_low >= node_disc)
                {
                    state.points.insert(node.to_string());
                }
            } else if state.parent.get(node).map(String::as_str) != Some(next.as_str()) {
                let next_disc = *state.disc.get(&next).unwrap_or(&0);
                let node_low = *state.low.get(node).unwrap_or(&0);
                state.low.insert(node.to_string(), node_low.min(next_disc));
            }
        }
    }

    let mut state = DfsState {
        disc: HashMap::new(),
        low: HashMap::new(),
        parent: HashMap::new(),
        points: BTreeSet::new(),
        timer: 0,
    };
    for id in sort_stable_ids(node_ids.to_vec()) {
        if !state.disc.contains_key(&id) {
            dfs(&id, adjacency, &mut state);
        }
    }
    state.points
}

fn bridge_edges(
    node_ids: &[String],
    adjacency: &HashMap<String, BTreeSet<String>>,
) -> BTreeSet<String> {
    struct DfsState {
        disc: HashMap<String, usize>,
        low: HashMap<String, usize>,
        parent: HashMap<String, String>,
        bridges: BTreeSet<String>,
        timer: usize,
    }

    fn dfs(node: &str, adjacency: &HashMap<String, BTreeSet<String>>, state: &mut DfsState) {
        state.disc.insert(node.to_string(), state.timer);
        state.low.insert(node.to_string(), state.timer);
        state.timer += 1;
        for next in sorted_neighbors(adjacency, node) {
            if !state.disc.contains_key(&next) {
                state.parent.insert(next.clone(), node.to_string());
                dfs(&next, adjacency, state);
                let next_low = *state.low.get(&next).unwrap_or(&0);
                let node_low = *state.low.get(node).unwrap_or(&0);
                state.low.insert(node.to_string(), node_low.min(next_low));
                let node_disc = *state.disc.get(node).unwrap_or(&0);
                if next_low > node_disc {
                    let key = role_pair_key(node, &next);
                    if !key.is_empty() {
                        state.bridges.insert(key);
                    }
                }
            } else if state.parent.get(node).map(String::as_str) != Some(next.as_str()) {
                let next_disc = *state.disc.get(&next).unwrap_or(&0);
                let node_low = *state.low.get(node).unwrap_or(&0);
                state.low.insert(node.to_string(), node_low.min(next_disc));
            }
        }
    }

    let mut state = DfsState {
        disc: HashMap::new(),
        low: HashMap::new(),
        parent: HashMap::new(),
        bridges: BTreeSet::new(),
        timer: 0,
    };
    for id in sort_stable_ids(node_ids.to_vec()) {
        if !state.disc.contains_key(&id) {
            dfs(&id, adjacency, &mut state);
        }
    }
    state.bridges
}

fn sorted_neighbors(adjacency: &HashMap<String, BTreeSet<String>>, id: &str) -> Vec<String> {
    sort_stable_ids(
        adjacency
            .get(id)
            .map(|set| set.iter().cloned().collect())
            .unwrap_or_default(),
    )
}

fn sort_stable_ids(ids: Vec<String>) -> Vec<String> {
    let mut normalized = ids
        .into_iter()
        .map(|id| id.trim().to_string())
        .filter(|id| !id.is_empty())
        .collect::<Vec<_>>();
    normalized.sort();
    normalized
}

fn role_pair_key(a: &str, b: &str) -> String {
    let s = a.trim();
    let t = b.trim();
    if s.is_empty() || t.is_empty() || s == t {
        return String::new();
    }
    let (left, right) = ordered_pair(s, t);
    format!("{left}||{right}")
}

fn ordered_pair(a: &str, b: &str) -> (String, String) {
    if a < b {
        (a.to_string(), b.to_string())
    } else {
        (b.to_string(), a.to_string())
    }
}

fn edge_degree_key(value: Option<&Value>) -> Option<String> {
    let Some(Value::String(text)) = value else {
        return None;
    };
    if text.is_empty() {
        None
    } else {
        Some(text.clone())
    }
}

fn first_non_empty_text(left: Option<&Value>, right: Option<&Value>) -> String {
    let left_text = left.map(js_or_empty_string).unwrap_or_default();
    if !left_text.is_empty() {
        return left_text;
    }
    right.map(js_or_empty_string).unwrap_or_default()
}

fn js_or_empty_string(value: &Value) -> String {
    match value {
        Value::Null => String::new(),
        Value::Bool(false) => String::new(),
        Value::Number(number) if number.as_f64() == Some(0.0) => String::new(),
        Value::String(text) if text.is_empty() => String::new(),
        _ => js_string(value),
    }
}

fn js_string(value: &Value) -> String {
    match value {
        Value::Null => String::new(),
        Value::Bool(flag) => flag.to_string(),
        Value::Number(number) => number.to_string(),
        Value::String(text) => text.clone(),
        Value::Array(values) => values.iter().map(js_string).collect::<Vec<_>>().join(","),
        Value::Object(_) => "[object Object]".to_string(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn semantic_pipeline_projects_preferred_core_ids() {
        let core_payload = json!({
            "nodes": [
                {"id": "C", "total_amount": 1000},
                {"id": "B"},
                {"id": "A", "total_amount": 1000},
                {"id": "L"}
            ],
            "edges": [
                {"source": "A", "target": "B"},
                {"source": "B", "target": "C"},
                {"source": "B", "target": "L"}
            ],
            "semanticPipeline": {
                "seedCoreCandidates": ["A", "C"],
                "options": {
                    "ENABLE_SEED_PROTECTION": true,
                    "SEED_DEMOTION_BYPASS_WEIGHT": 100
                }
            }
        });
        assert_eq!(
            project_layout_role_graph(&core_payload)
                .get("preferredCoreIds")
                .cloned()
                .unwrap_or(Value::Null),
            json!(["A", "C"])
        );
        assert_eq!(
            project_layout_role_graph(&core_payload)
                .get("labelPolicy")
                .cloned()
                .unwrap_or(Value::Null),
            json!({
                "enabled": false,
                "reason": "disabled_by_user",
                "totalNodes": 4,
                "leafCount": 1,
                "adjacentCount": 1,
                "leafRatio": 0.25,
                "largestClusterNodeCount": 4,
                "keepLeafLabels": 1,
                "hiddenLeafLabels": 0,
                "keepAdjacentLabels": 1,
                "hiddenAdjacentLabels": 0,
                "keepLeafSample": [],
                "keepAdjacentSample": [],
            })
        );

        let focus_payload = json!({
            "nodes": [{"id": "n2"}, {"id": "n1"}],
            "edges": [{"source": "n1", "target": "n2"}],
            "semanticPipeline": {
                "seedCoreCandidates": [],
                "focusId": "n2"
            }
        });
        assert_eq!(
            project_layout_role_graph(&focus_payload)
                .get("preferredCoreIds")
                .cloned()
                .unwrap_or(Value::Null),
            json!(["n2"])
        );

        let degree_payload = json!({
            "nodes": [{"id": "n1"}, {"id": "n2"}, {"id": "n3"}, {"id": "n4"}],
            "edges": [
                {"source": "n1", "target": "n2"},
                {"source": "n2", "target": "n3"},
                {"source": "n2", "target": "n4"},
                {"source": "n4", "target": "n4", "isVisible": false},
                {"source": "n4", "target": "n3", "isVisible": false}
            ],
            "semanticPipeline": {
                "seedCoreCandidates": []
            }
        });
        assert_eq!(
            project_layout_role_graph(&degree_payload)
                .get("preferredCoreIds")
                .cloned()
                .unwrap_or(Value::Null),
            json!(["n4", "n2", "n3"])
        );
    }

    #[test]
    fn project_layout_role_graph_matches_js_contract_shape() {
        let payload = json!({
            "nodes": [
                {"id": " b ", "marker": "first-b"},
                {"id": "b", "marker": "duplicate-b"},
                {"id": "a", "marker": "node-a"},
                {"id": "c", "marker": "node-c"},
                {"id": "isolated", "marker": "node-isolated"},
                {"id": "d", "marker": "node-d"}
            ],
            "edges": [
                {"source": " b ", "target": " a ", "mode": "single"},
                {"source": "b", "target": "c", "isVisible": false},
                {"source": "a", "target": "a", "mode": "double"},
                {"source": "a", "target": "missing"},
                {"source": "c", "target": "d", "mode": "double"},
                {"source": "a", "target": "b", "edgeArrow": "both"},
                {"source": "d", "target": "a", "arrow": "both"}
            ],
            "traversal": {
                "nodeIds": ["root", "leaf", "c", "b", "a", "isolated"],
                "adjacency": {
                    "root": ["c", "leaf"],
                    "leaf": ["root"],
                    "c": ["a", "b", "root"],
                    "b": ["a", "c"],
                    "a": ["b", "c"],
                    "isolated": []
                }
            },
            "demotion": {
                "seedCoreCandidates": ["b", "a", "d", "isolated"],
                "options": {
                    "seedHistory": {
                        "b": {"total": 2, "kept": 1, "stableScore": 0.5}
                    }
                }
            },
            "promotion": {
                "coreIds": ["a", "c"],
                "options": {
                    "MAX_PROMOTION_HOPS": 2,
                    "CORE_NEIGHBORHOOD_HOPS": 1,
                    "MAX_SHORTEST_PATHS": 8,
                    "SUPPRESS_NOISY_PATH_PROMOTION": true
                }
            }
        });

        assert_eq!(
            project_layout_role_graph(&payload),
            json!({
                "roleGraph": {
                    "nodeIds": ["a", "b", "c", "d", "isolated"],
                    "adjacency": {
                        "a": ["b", "d"],
                        "b": ["a"],
                        "c": ["d"],
                        "d": ["a", "c"],
                        "isolated": []
                    },
                    "neighborCount": {"a": 2, "b": 1, "c": 1, "d": 2, "isolated": 0},
                    "neighborEdgeMap": {
                        "a": {"b": "a||b", "d": "a||d"},
                        "b": {"a": "a||b"},
                        "c": {"d": "c||d"},
                        "d": {"a": "a||d", "c": "c||d"},
                        "isolated": {}
                    },
                    "uniqueUndirectedEdges": [
                        {"source": "a", "target": "b", "lineType": "double", "edgeIndexes": [0, 5]},
                        {"source": "c", "target": "d", "lineType": "double", "edgeIndexes": [4]},
                        {"source": "a", "target": "d", "lineType": "double", "edgeIndexes": [6]}
                    ],
                    "components": [["a", "b", "d", "c"], ["isolated"]],
                    "componentByNode": {"a": 0, "b": 0, "c": 0, "d": 0, "isolated": 1}
                },
                "traversal": {
                    "components": [["a", "b", "c", "root", "leaf"], ["isolated"]],
                    "bridgeEdges": ["c||root", "leaf||root"],
                    "articulationPoints": ["c", "root"]
                },
                "demotion": {
                    "coreIds": ["isolated"],
                    "demoteTags": {"a": "deg2", "b": "deg1", "d": "deg2"},
                    "seedMeta": {
                        "a": {"isSeedSelected": true, "seedWeight": 0, "seedStabilityScore": 0},
                        "b": {"isSeedSelected": true, "seedWeight": 0, "seedStabilityScore": 0.333},
                        "d": {"isSeedSelected": true, "seedWeight": 0, "seedStabilityScore": 0},
                        "isolated": {"isSeedSelected": true, "seedWeight": 0, "seedStabilityScore": 1}
                    },
                    "nextSeedCoreHistory": {
                        "a": {"total": 1, "kept": 0, "stableScore": 0},
                        "b": {"total": 3, "kept": 1, "stableScore": 0.333},
                        "d": {"total": 1, "kept": 0, "stableScore": 0},
                        "isolated": {"total": 1, "kept": 1, "stableScore": 1}
                    }
                },
                "promotion": {
                    "pathPromotedAdjacentIds": ["d"],
                    "promotionReport": {
                        "maxHops": 2,
                        "corePairs": [
                            {
                                "pair": ["a", "c"],
                                "shortestHops": 2,
                                "candidatePathCount": 1,
                                "neighborhoodFilteredPathCount": 1,
                                "selectedBasis": "best_single_path",
                                "promotedNodes": ["d"],
                                "promotedNodesRaw": ["d"],
                                "droppedNoisyNodes": []
                            }
                        ],
                        "rejected": [],
                        "truncated": false
                    }
                }
            })
        );
    }
}
