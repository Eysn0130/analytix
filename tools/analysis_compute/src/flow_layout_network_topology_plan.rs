use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Value};
use std::cmp::Ordering;
use std::collections::{BTreeMap, BTreeSet, HashMap, VecDeque};
use std::f64::consts::PI;
use std::{fs, path::PathBuf, time::Instant};

const PLAN_VERSION: &str = "network-topology-plan-v1";
const TWO_PI: f64 = PI * 2.0;
const XLARGE_PRIMARY_LIMIT: usize = 16;
const XLARGE_SECONDARY_LIMIT: usize = 184;
const XLARGE_BRIDGE_COMMUNITY_VISIBLE_CAP: usize = 4;
const XLARGE_EDGE_UPDATE_LIMIT: usize = 160;
const LARGE_EDGE_UPDATE_LIMIT: usize = 240;
const XLARGE_EDGE_ENDPOINT_LIMIT: usize = 8;
const LARGE_EDGE_ENDPOINT_LIMIT: usize = 14;
const XLARGE_EDGE_MIN_FILL_RATIO: f64 = 0.72;
const LARGE_EDGE_MIN_FILL_RATIO: f64 = 0.55;
const XLARGE_RUNTIME_NODE_BUDGET: usize = 260;
const XLARGE_RUNTIME_EDGE_BUDGET: usize = 640;
const LARGE_RUNTIME_NODE_BUDGET: usize = 520;
const LARGE_RUNTIME_EDGE_BUDGET: usize = 900;
const XLARGE_CROSS_PAIR_LIMIT: usize = 1;
const LARGE_CROSS_PAIR_LIMIT: usize = 2;
const XLARGE_CENTER_COMMUNITY_LIMIT: usize = 260;
const XLARGE_CENTER_SIZE_LIMIT: usize = 220;
const XLARGE_CENTER_BRIDGE_EDGE_LIMIT: usize = 320;
const COMMUNITY_QUALITY_LIMIT: usize = 12;
const COMMUNITY_QUALITY_NODE_LIMIT: usize = 220;
const COMMUNITY_QUALITY_EDGE_LIMIT: usize = 280;

#[derive(Clone, Debug)]
pub(crate) struct NetworkTopologyPlanArgs {
    pub(crate) payload: Value,
}

#[derive(Clone, Debug)]
struct LayoutNode {
    id: String,
    index: usize,
    role: String,
    cluster_id: String,
    layout_community_id: String,
    x: f64,
    y: f64,
    seed_x: f64,
    seed_y: f64,
    vx: f64,
    vy: f64,
    radius: f64,
    collision_radius: f64,
    label_half_width: f64,
    label_height: f64,
    degree: usize,
    weighted_degree: f64,
    community: String,
    bridge_score: f64,
    hub_score: f64,
    community_role: String,
    visibility_tier: String,
    label_tier: String,
    has_layout_visibility_tier: bool,
    has_layout_label_tier: bool,
}

#[derive(Clone, Debug)]
struct LayoutEdge {
    source: usize,
    target: usize,
    raw_weight: f64,
    weight: f64,
}

#[derive(Clone, Debug)]
struct EdgeBundleStats {
    pair: String,
    edge_count: usize,
    total_weight: f64,
    rank: usize,
}

#[derive(Clone, Debug)]
struct CommunitySuperEdgeStats {
    source_community: String,
    target_community: String,
    edge_count: usize,
    total_weight: f64,
    representative_source: String,
    representative_target: String,
    representative_weight: f64,
    rank: usize,
}

#[derive(Clone, Debug)]
struct EdgeBundleReport {
    super_edge_count: usize,
    bundled_edge_count: usize,
    largest_super_edge_size: usize,
    routed_super_edge_count: usize,
}

#[derive(Clone, Debug)]
struct CommunitySummary {
    id: String,
    node_ids: Vec<String>,
    node_count: usize,
    edge_weight: f64,
    x: f64,
    y: f64,
    radius: f64,
    bbox: (f64, f64, f64, f64),
}

#[derive(Clone, Debug)]
struct TopologySeedRow {
    key: String,
    indexes: Vec<usize>,
    edge_count: usize,
    isolate_bin: bool,
}

#[derive(Clone, Debug, Default)]
struct CommunityQualityHints {
    community_ids: BTreeSet<String>,
    primary_node_ids: BTreeSet<String>,
    node_ids: BTreeSet<String>,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum NetworkPlanMode {
    Small,
    Medium,
    Large,
    XLarge,
}

impl CommunityQualityHints {
    fn from_payload(payload: &Value) -> Self {
        let mut hints = Self::default();
        let semantic = payload
            .get("semantic")
            .filter(|value| value.is_object())
            .unwrap_or(&Value::Null);
        let nested_semantic = semantic
            .get("semantic")
            .filter(|value| value.is_object())
            .unwrap_or(&Value::Null);
        let seed_context = payload
            .get("seedContext")
            .or_else(|| payload.get("seed_context"))
            .filter(|value| value.is_object())
            .unwrap_or(&Value::Null);

        for source in [semantic, nested_semantic] {
            extend_text_set(
                &mut hints.community_ids,
                source.get("communityQualityCommunityIds"),
            );
            extend_text_set(
                &mut hints.community_ids,
                source.get("community_quality_community_ids"),
            );
            extend_text_set(&mut hints.community_ids, source.get("expandedCommunityIds"));
            extend_text_set(
                &mut hints.community_ids,
                source.get("expanded_community_ids"),
            );
            extend_text_set(&mut hints.community_ids, source.get("expandedClusterIds"));
            extend_text_set(&mut hints.community_ids, source.get("expanded_cluster_ids"));
            extend_text_set(&mut hints.community_ids, source.get("clusterIds"));
            extend_text_set(&mut hints.community_ids, source.get("cluster_ids"));

            extend_text_set(&mut hints.node_ids, source.get("communityQualityNodeIds"));
            extend_text_set(
                &mut hints.node_ids,
                source.get("community_quality_node_ids"),
            );
            extend_text_set(
                &mut hints.primary_node_ids,
                source.get("communityQualityPrimaryNodeIds"),
            );
            extend_text_set(
                &mut hints.primary_node_ids,
                source.get("community_quality_primary_node_ids"),
            );
            extend_text_set(&mut hints.node_ids, source.get("searchMatchNodeIds"));
            extend_text_set(&mut hints.node_ids, source.get("search_match_node_ids"));
            extend_text_set(&mut hints.node_ids, source.get("pathNodeIds"));
            extend_text_set(&mut hints.node_ids, source.get("path_node_ids"));
            extend_text_set(&mut hints.node_ids, source.get("expandedNodeIds"));
            extend_text_set(&mut hints.node_ids, source.get("expanded_node_ids"));
            extend_text_set(&mut hints.node_ids, source.get("selectedIds"));
            extend_text_set(&mut hints.node_ids, source.get("selected_ids"));
            extend_text_set(
                &mut hints.primary_node_ids,
                source.get("searchMatchNodeIds"),
            );
            extend_text_set(
                &mut hints.primary_node_ids,
                source.get("search_match_node_ids"),
            );
            extend_text_set(&mut hints.primary_node_ids, source.get("pathNodeIds"));
            extend_text_set(&mut hints.primary_node_ids, source.get("path_node_ids"));
            extend_text_set(&mut hints.primary_node_ids, source.get("expandedNodeIds"));
            extend_text_set(&mut hints.primary_node_ids, source.get("expanded_node_ids"));
        }
        extend_text_set(&mut hints.node_ids, seed_context.get("selectedIds"));
        extend_text_set(&mut hints.node_ids, seed_context.get("selected_ids"));
        for node_id in hints.primary_node_ids.clone() {
            hints.node_ids.insert(node_id);
        }
        hints
    }

    fn community_rank_for_indexes(
        &self,
        community: &str,
        nodes: &[LayoutNode],
        indexes: &[usize],
    ) -> usize {
        let community_match =
            !community.trim().is_empty() && self.community_ids.contains(community);
        let primary_node_match = !self.primary_node_ids.is_empty()
            && indexes
                .iter()
                .filter_map(|index| nodes.get(*index))
                .any(|node| self.primary_node_ids.contains(node.id.as_str()));
        let node_match = !self.node_ids.is_empty()
            && indexes
                .iter()
                .filter_map(|index| nodes.get(*index))
                .any(|node| self.node_ids.contains(node.id.as_str()));
        if community_match && primary_node_match {
            5
        } else if primary_node_match {
            4
        } else if community_match && node_match {
            3
        } else if community_match {
            2
        } else if node_match {
            1
        } else {
            0
        }
    }
}

impl NetworkPlanMode {
    fn as_str(self) -> &'static str {
        match self {
            Self::Small => "small",
            Self::Medium => "medium",
            Self::Large => "large",
            Self::XLarge => "xlarge",
        }
    }
}

fn node_update_limit_for_mode(mode: NetworkPlanMode, node_count: usize) -> usize {
    if matches!(mode, NetworkPlanMode::XLarge) {
        XLARGE_PRIMARY_LIMIT + XLARGE_SECONDARY_LIMIT
    } else {
        node_count
    }
}

fn edge_update_limit_for_mode(mode: NetworkPlanMode) -> usize {
    if matches!(mode, NetworkPlanMode::XLarge) {
        XLARGE_EDGE_UPDATE_LIMIT
    } else if matches!(mode, NetworkPlanMode::Large) {
        LARGE_EDGE_UPDATE_LIMIT
    } else {
        0
    }
}

fn edge_endpoint_limit_for_mode(mode: NetworkPlanMode) -> usize {
    if matches!(mode, NetworkPlanMode::XLarge) {
        XLARGE_EDGE_ENDPOINT_LIMIT
    } else if matches!(mode, NetworkPlanMode::Large) {
        LARGE_EDGE_ENDPOINT_LIMIT
    } else {
        0
    }
}

fn edge_min_fill_ratio_for_mode(mode: NetworkPlanMode) -> f64 {
    if matches!(mode, NetworkPlanMode::XLarge) {
        XLARGE_EDGE_MIN_FILL_RATIO
    } else if matches!(mode, NetworkPlanMode::Large) {
        LARGE_EDGE_MIN_FILL_RATIO
    } else {
        0.0
    }
}

fn runtime_node_budget_for_mode(mode: NetworkPlanMode, node_count: usize) -> usize {
    if matches!(mode, NetworkPlanMode::XLarge) {
        XLARGE_RUNTIME_NODE_BUDGET
    } else if matches!(mode, NetworkPlanMode::Large) {
        LARGE_RUNTIME_NODE_BUDGET
    } else {
        node_count
    }
}

fn runtime_edge_budget_for_mode(mode: NetworkPlanMode, edge_count: usize) -> usize {
    if matches!(mode, NetworkPlanMode::XLarge) {
        XLARGE_RUNTIME_EDGE_BUDGET
    } else if matches!(mode, NetworkPlanMode::Large) {
        LARGE_RUNTIME_EDGE_BUDGET
    } else {
        edge_count
    }
}

fn count_update_tiers(rows: &[Value], key: &str, fallback: &str) -> BTreeMap<String, usize> {
    let mut tiers = BTreeMap::new();
    for row in rows {
        let tier = row
            .get(key)
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .unwrap_or(fallback);
        *tiers.entry(tier.to_string()).or_insert(0) += 1;
    }
    tiers
}

pub(crate) fn parse_args(
    mut iter: impl Iterator<Item = String>,
) -> Result<NetworkTopologyPlanArgs> {
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
                    format!("read network layout plan input {}", input_path.display())
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!("parse network layout plan input {}", input_path.display())
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute <project-flow-layout-network-plan|project-flow-layout-network-community-quality> <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported network layout plan flag: {other}"),
        }
    }

    Ok(NetworkTopologyPlanArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_flow_layout_network_plan(payload: &Value) -> Value {
    let started_at = Instant::now();
    let hydrated_payload = hydrate_network_payload_from_snapshot_path(payload);
    let hydrated_at = Instant::now();
    let payload = hydrated_payload.as_ref().unwrap_or(payload);
    let mut graph = LayoutGraph::from_payload(payload);
    let community_quality_hints = CommunityQualityHints::from_payload(payload);
    let graph_ready_at = Instant::now();
    let mode = resolve_mode(graph.nodes.len(), graph.edges.len());
    if graph.nodes.is_empty() {
        let updates_ready_at = Instant::now();
        let node_updates = Vec::new();
        let edge_updates = Vec::new();
        let lod = graph.lod_payload(mode, &node_updates, &edge_updates);
        let supergraph = graph.supergraph_payload(mode, &[], &edge_updates);
        return json!({
                "version": PLAN_VERSION,
                "mode": mode.as_str(),
                "nodeUpdates": node_updates,
                "edgeUpdates": edge_updates,
                "communities": [],
                "lod": lod,
                "supergraph": supergraph,
                "quality": quality_payload(mode, 0, 0, 0, 0, 0.0, 0, started_at.elapsed().as_secs_f64() * 1000.0, 0, 0, 0, &BTreeMap::new(), ""),
                "sampleQuality": Value::Null,
                "communityQuality": [],
            "report": {
                "nodeCount": 0,
                "edgeCount": 0,
                "sourceSnapshotPath": hydrated_payload.is_some(),
                "phaseDurationMs": phase_duration_payload(
                    started_at,
                    hydrated_at,
                    graph_ready_at,
                    graph_ready_at,
                    updates_ready_at,
                    updates_ready_at,
                ),
                "iterations": iteration_budget(mode, graph.nodes.len(), graph.edges.len()),
                "partial": false,
            },
        });
    }

    graph.assign_communities(mode);
    graph.seed_topology_mode(mode);
    graph.run_force(mode);
    graph.remove_collisions(mode);
    let label_collision_suppressed_count = 0usize;
    let actual_community_count = graph.community_count();
    let communities = graph.community_summaries();
    let layout_ready_at = Instant::now();
    let quality = graph.quality(
        mode,
        &communities,
        started_at.elapsed().as_secs_f64() * 1000.0,
    );
    let sample_quality = graph.sample_quality(mode, started_at.elapsed().as_secs_f64() * 1000.0);
    let community_quality = graph.community_quality(
        mode,
        started_at.elapsed().as_secs_f64() * 1000.0,
        &community_quality_hints,
    );
    let quality_ready_at = Instant::now();
    let node_updates = graph.node_updates();
    let edge_updates = Vec::new();
    let lod = graph.lod_payload(mode, &node_updates, &edge_updates);
    let supergraph = graph.supergraph_payload(mode, &communities, &edge_updates);
    let community_super_node_count = supergraph
        .get("nodeCount")
        .and_then(Value::as_u64)
        .unwrap_or(0);
    let community_super_edge_count = supergraph
        .get("edgeCount")
        .and_then(Value::as_u64)
        .unwrap_or(0);
    let edge_bundle_report = edge_bundle_report_from_updates(&edge_updates);
    let community_quality_count = community_quality.len();
    let updates_ready_at = Instant::now();

    json!({
        "version": PLAN_VERSION,
        "mode": mode.as_str(),
        "nodeUpdates": node_updates,
        "edgeUpdates": edge_updates,
        "communities": communities.iter().map(community_to_json).collect::<Vec<_>>(),
        "lod": lod,
        "supergraph": supergraph,
        "quality": quality,
        "sampleQuality": sample_quality,
        "communityQuality": community_quality,
        "report": {
            "nodeCount": graph.nodes.len(),
            "edgeCount": graph.edges.len(),
            "communityCount": actual_community_count,
            "visibleCommunityCount": communities.len(),
            "communityQualityCount": community_quality_count,
            "communitySuperNodeCount": community_super_node_count,
            "communitySuperEdgeCount": community_super_edge_count,
            "visibleBridgeNodeCount": graph.visible_bridge_node_count(),
            "superEdgeCount": edge_bundle_report.super_edge_count,
            "bundledEdgeCount": edge_bundle_report.bundled_edge_count,
            "largestSuperEdgeSize": edge_bundle_report.largest_super_edge_size,
            "routedSuperEdgeCount": edge_bundle_report.routed_super_edge_count,
            "labelCollisionSuppressedCount": label_collision_suppressed_count,
            "sourceSnapshotPath": hydrated_payload.is_some(),
            "phaseDurationMs": phase_duration_payload(
                started_at,
                hydrated_at,
                graph_ready_at,
                layout_ready_at,
                quality_ready_at,
                updates_ready_at,
            ),
            "iterations": iteration_budget(mode, graph.nodes.len(), graph.edges.len()),
            "partial": matches!(mode, NetworkPlanMode::Large | NetworkPlanMode::XLarge),
        },
    })
}

pub(crate) fn project_flow_layout_network_community_quality(payload: &Value) -> Value {
    let started_at = Instant::now();
    let hydrated_payload = hydrate_network_payload_from_snapshot_path(payload);
    let hydrated_at = Instant::now();
    let payload = hydrated_payload.as_ref().unwrap_or(payload);
    let mut graph = LayoutGraph::from_payload(payload);
    let community_quality_hints = CommunityQualityHints::from_payload(payload);
    let graph_ready_at = Instant::now();
    let mode = resolve_mode(graph.nodes.len(), graph.edges.len());
    if graph.nodes.is_empty() {
        let updates_ready_at = Instant::now();
        return json!({
            "version": PLAN_VERSION,
            "mode": mode.as_str(),
            "communityQuality": [],
            "quality": quality_payload(mode, 0, 0, 0, 0, 0.0, 0, started_at.elapsed().as_secs_f64() * 1000.0, 0, 0, 0, &BTreeMap::new(), "community-refresh"),
            "report": {
                "nodeCount": 0,
                "edgeCount": 0,
                "communityCount": 0,
                "communityQualityCount": 0,
                "qualityOnly": true,
                "qualityOnlyFastPath": true,
                "sourceSnapshotPath": hydrated_payload.is_some(),
                "phaseDurationMs": phase_duration_payload(
                    started_at,
                    hydrated_at,
                    graph_ready_at,
                    graph_ready_at,
                    updates_ready_at,
                    updates_ready_at,
                ),
                "iterations": iteration_budget(mode, graph.nodes.len(), graph.edges.len()),
                "partial": false,
            },
        });
    }

    let quality_only_fast_path = graph.prepare_community_quality_only(mode);
    let actual_community_count = graph.community_count();
    let layout_ready_at = Instant::now();
    let duration_ms = started_at.elapsed().as_secs_f64() * 1000.0;
    let community_quality = graph.community_quality(mode, duration_ms, &community_quality_hints);
    let community_quality_count = community_quality.len();
    let quality_node_indexes = graph.sample_quality_node_indexes(mode);
    let quality_edge_indexes = graph.sample_quality_edge_indexes(mode, &quality_node_indexes);
    let quality_label_indexes = quality_node_indexes
        .iter()
        .copied()
        .filter(|index| {
            graph
                .nodes
                .get(*index)
                .map(|node| node.label_tier != "hidden")
                .unwrap_or(false)
        })
        .collect::<Vec<_>>();
    let mut tier_stats = graph.node_tier_stats(Some(&quality_node_indexes));
    graph.add_edge_tier_stats(&quality_edge_indexes, &mut tier_stats);
    let quality = graph.quality_for_scope(
        mode,
        "community-refresh",
        Some(&quality_node_indexes),
        Some(&quality_label_indexes),
        Some(&quality_edge_indexes),
        &[],
        duration_ms,
        tier_stats,
    );
    let quality_ready_at = Instant::now();
    let updates_ready_at = Instant::now();

    json!({
        "version": PLAN_VERSION,
        "mode": mode.as_str(),
        "communityQuality": community_quality,
        "quality": quality,
        "report": {
            "nodeCount": graph.nodes.len(),
            "edgeCount": graph.edges.len(),
            "communityCount": actual_community_count,
            "communityQualityCount": community_quality_count,
            "qualityOnly": true,
            "qualityOnlyFastPath": quality_only_fast_path,
            "sourceSnapshotPath": hydrated_payload.is_some(),
            "phaseDurationMs": phase_duration_payload(
                started_at,
                hydrated_at,
                graph_ready_at,
                layout_ready_at,
                quality_ready_at,
                updates_ready_at,
            ),
            "iterations": iteration_budget(mode, graph.nodes.len(), graph.edges.len()),
            "partial": matches!(mode, NetworkPlanMode::Large | NetworkPlanMode::XLarge),
        },
    })
}

fn phase_duration_payload(
    started_at: Instant,
    hydrated_at: Instant,
    graph_ready_at: Instant,
    layout_ready_at: Instant,
    quality_ready_at: Instant,
    updates_ready_at: Instant,
) -> Value {
    json!({
        "hydrate": round2(hydrated_at.duration_since(started_at).as_secs_f64() * 1000.0),
        "graphBuild": round2(graph_ready_at.duration_since(hydrated_at).as_secs_f64() * 1000.0),
        "layout": round2(layout_ready_at.duration_since(graph_ready_at).as_secs_f64() * 1000.0),
        "quality": round2(quality_ready_at.duration_since(layout_ready_at).as_secs_f64() * 1000.0),
        "updates": round2(updates_ready_at.duration_since(quality_ready_at).as_secs_f64() * 1000.0),
        "totalBeforeSerialize": round2(updates_ready_at.duration_since(started_at).as_secs_f64() * 1000.0),
    })
}

fn hydrate_network_payload_from_snapshot_path(payload: &Value) -> Option<Value> {
    let source = payload.get("source").filter(|value| value.is_object())?;
    let path = source
        .get("resultSnapshotPath")
        .or_else(|| source.get("result_snapshot_path"))
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim();
    if path.is_empty() {
        return None;
    }
    let raw = fs::read_to_string(path).ok()?;
    let mut snapshot: Value = serde_json::from_str(&raw).ok()?;
    let (nodes, edges) = take_snapshot_graph_arrays(&mut snapshot)?;
    let mut next = payload.clone();
    if let Value::Object(ref mut map) = next {
        map.insert("nodes".to_string(), nodes);
        map.insert("edges".to_string(), edges);
    }
    Some(next)
}

fn take_snapshot_graph_arrays(snapshot: &mut Value) -> Option<(Value, Value)> {
    if let Some(graph) = snapshot.get_mut("graph") {
        if let Some(values) = take_graph_arrays(graph) {
            return Some(values);
        }
    }
    if let Some(graph) = snapshot.get_mut("runtime_graph") {
        if let Some(values) = take_graph_arrays(graph) {
            return Some(values);
        }
    }
    if let Some(graph) = snapshot
        .get_mut("result")
        .and_then(|result| result.get_mut("runtime_graph"))
    {
        if let Some(values) = take_graph_arrays(graph) {
            return Some(values);
        }
    }
    None
}

fn take_graph_arrays(graph: &mut Value) -> Option<(Value, Value)> {
    let nodes_len = graph.get("nodes").and_then(Value::as_array).map(Vec::len)?;
    if nodes_len == 0 || graph.get("edges").and_then(Value::as_array).is_none() {
        return None;
    }
    let nodes = graph.get_mut("nodes")?.take();
    let edges = graph.get_mut("edges")?.take();
    Some((nodes, edges))
}

fn resolve_mode(node_count: usize, edge_count: usize) -> NetworkPlanMode {
    if node_count > 5000 || edge_count > 15000 {
        NetworkPlanMode::XLarge
    } else if node_count > 1200 || edge_count > 3500 {
        NetworkPlanMode::Large
    } else if node_count > 150 || edge_count > 400 {
        NetworkPlanMode::Medium
    } else {
        NetworkPlanMode::Small
    }
}

fn iteration_budget(mode: NetworkPlanMode, node_count: usize, edge_count: usize) -> usize {
    match mode {
        NetworkPlanMode::Small => {
            if node_count <= 40 && edge_count <= 120 {
                120
            } else {
                150
            }
        }
        NetworkPlanMode::Medium => 150,
        NetworkPlanMode::Large => 32,
        NetworkPlanMode::XLarge => 8,
    }
}

fn edge_bundle_report_from_updates(edge_updates: &[Value]) -> EdgeBundleReport {
    let mut super_edge_count = 0usize;
    let mut bundled_edge_count = 0usize;
    let mut largest_super_edge_size = 0usize;
    let mut routed_super_edge_count = 0usize;
    for row in edge_updates {
        let bundle_size = row
            .get("layoutEdgeBundleSize")
            .and_then(Value::as_u64)
            .unwrap_or(1) as usize;
        if bundle_size > 1 {
            super_edge_count += 1;
            bundled_edge_count += bundle_size.saturating_sub(1);
            largest_super_edge_size = largest_super_edge_size.max(bundle_size);
        }
        if row
            .get("layoutEdgeBundleRouted")
            .and_then(Value::as_bool)
            .unwrap_or(false)
        {
            routed_super_edge_count += 1;
        }
    }
    EdgeBundleReport {
        super_edge_count,
        bundled_edge_count,
        largest_super_edge_size,
        routed_super_edge_count,
    }
}

#[derive(Clone, Debug)]
struct LayoutGraph {
    nodes: Vec<LayoutNode>,
    edges: Vec<LayoutEdge>,
    adjacency: Vec<Vec<(usize, f64)>>,
}

impl LayoutGraph {
    fn from_payload(payload: &Value) -> Self {
        let node_values = payload.get("nodes").and_then(Value::as_array);
        let node_value_count = node_values.map_or(0, Vec::len);
        let mut id_to_index = HashMap::with_capacity(node_value_count);
        let mut nodes = Vec::new();
        for (index, value) in node_values.into_iter().flatten().enumerate() {
            let id = text_field(value, "id");
            if id.is_empty() || id_to_index.contains_key(&id) {
                continue;
            }
            let role = first_text_field(value, &["role", "layoutBand"])
                .unwrap_or_else(|| "leaf".to_string());
            let cluster_id =
                first_text_field(value, &["layoutClusterId", "clusterId", "layoutCommunity"])
                    .unwrap_or_default();
            let layout_community_id = text_field(value, "layoutCommunity");
            let radius = finite_field(value, "r")
                .or_else(|| finite_field(value, "radius"))
                .unwrap_or(18.0)
                .clamp(8.0, 64.0);
            let label_half_width = finite_field(value, "labelHalfWidth")
                .unwrap_or_else(|| estimate_label_half_width(value))
                .clamp(18.0, 260.0);
            let label_height = finite_field(value, "labelBottom")
                .unwrap_or_else(|| radius + 36.0)
                .clamp(28.0, 180.0);
            let collision_radius = finite_field(value, "collisionRadius")
                .unwrap_or_else(|| {
                    (radius + 24.0)
                        .max(label_half_width * 0.55 + radius * 0.2)
                        .max(label_height * 0.58)
                })
                .clamp(radius + 8.0, 320.0);
            let _amount_weight = first_finite_field(
                value,
                &[
                    "weight",
                    "total_amount",
                    "totalAmount",
                    "total_amt",
                    "totalAmt",
                    "amount",
                    "total_count",
                ],
            )
            .unwrap_or(1.0)
            .abs()
            .max(1.0);
            let seed = seed_position(value, &id, index, node_value_count);
            let raw_visibility_tier =
                first_text_field(value, &["layoutVisibilityTier", "visibilityTier"]);
            let has_layout_visibility_tier = raw_visibility_tier.is_some();
            let raw_label_tier = first_text_field(value, &["layoutLabelTier", "labelTier"]);
            let node_label_hidden = value
                .get("nodeLabelHidden")
                .or_else(|| value.get("node_label_hidden"))
                .and_then(Value::as_bool)
                .unwrap_or(false);
            let has_layout_label_tier = raw_label_tier.is_some() || node_label_hidden;
            let visibility_tier =
                normalize_layout_tier(raw_visibility_tier.as_deref(), "secondary");
            let label_tier = if node_label_hidden {
                "hidden".to_string()
            } else {
                normalize_layout_tier(raw_label_tier.as_deref(), "secondary")
            };
            id_to_index.insert(id.clone(), nodes.len());
            nodes.push(LayoutNode {
                id,
                index,
                role: normalize_role(&role),
                cluster_id,
                layout_community_id,
                x: seed.0,
                y: seed.1,
                seed_x: seed.0,
                seed_y: seed.1,
                vx: 0.0,
                vy: 0.0,
                radius,
                collision_radius,
                label_half_width,
                label_height,
                degree: 0,
                weighted_degree: 0.0,
                community: String::new(),
                bridge_score: 0.0,
                hub_score: 0.0,
                community_role: String::new(),
                visibility_tier,
                label_tier,
                has_layout_visibility_tier,
                has_layout_label_tier,
            });
        }
        disperse_duplicate_positions(&mut nodes);

        let mut edges = Vec::new();
        let mut max_raw_weight = 1.0_f64;
        for value in payload
            .get("edges")
            .and_then(Value::as_array)
            .into_iter()
            .flatten()
        {
            let source_id = edge_endpoint(value.get("source"));
            let target_id = edge_endpoint(value.get("target"));
            if source_id.is_empty() || target_id.is_empty() || source_id == target_id {
                continue;
            }
            let Some(&source) = id_to_index.get(&source_id) else {
                continue;
            };
            let Some(&target) = id_to_index.get(&target_id) else {
                continue;
            };
            let raw_weight = edge_weight(&value);
            max_raw_weight = max_raw_weight.max(raw_weight);
            edges.push(LayoutEdge {
                source,
                target,
                raw_weight,
                weight: raw_weight,
            });
        }
        let denom = max_raw_weight.ln_1p().max(1.0);
        for edge in &mut edges {
            edge.weight = (0.35 + edge.raw_weight.ln_1p() / denom * 2.65).clamp(0.35, 3.0);
        }

        let mut adjacency = vec![Vec::new(); nodes.len()];
        for edge in &edges {
            adjacency[edge.source].push((edge.target, edge.weight));
            adjacency[edge.target].push((edge.source, edge.weight));
        }
        for (index, rows) in adjacency.iter().enumerate() {
            nodes[index].degree = rows.len();
            nodes[index].weighted_degree = rows.iter().map(|(_, weight)| *weight).sum::<f64>();
        }

        Self {
            nodes,
            edges,
            adjacency,
        }
    }

    fn assign_communities(&mut self, mode: NetworkPlanMode) {
        for node in &mut self.nodes {
            node.community = if !node.cluster_id.is_empty() {
                node.cluster_id.clone()
            } else {
                format!("seed:{}", node.id)
            };
        }
        if self.edges.is_empty() {
            self.score_bridge_and_hub();
            return;
        }

        let max_iterations = match mode {
            NetworkPlanMode::Small => 8,
            NetworkPlanMode::Medium => 6,
            NetworkPlanMode::Large => 4,
            NetworkPlanMode::XLarge => 2,
        };
        for _ in 0..max_iterations {
            let mut changed = false;
            for index in 0..self.nodes.len() {
                let mut scores: BTreeMap<String, f64> = BTreeMap::new();
                let own = self.nodes[index].community.clone();
                *scores.entry(own.clone()).or_insert(0.0) += 0.28;
                for (neighbor, weight) in &self.adjacency[index] {
                    let key = self.nodes[*neighbor].community.clone();
                    *scores.entry(key).or_insert(0.0) += *weight;
                }
                let mut best_key = own.clone();
                let mut best_score = *scores.get(&own).unwrap_or(&0.0);
                for (key, score) in scores {
                    if score > best_score + 1e-9
                        || ((score - best_score).abs() <= 1e-9 && key < best_key)
                    {
                        best_key = key;
                        best_score = score;
                    }
                }
                if best_key != own {
                    self.nodes[index].community = best_key;
                    changed = true;
                }
            }
            if !changed {
                break;
            }
        }
        self.compact_community_ids();
        if matches!(mode, NetworkPlanMode::Small | NetworkPlanMode::Medium) {
            self.split_disconnected_communities();
            self.compact_community_ids();
        }
        if matches!(mode, NetworkPlanMode::Large) {
            let refined_groups = self.refine_xlarge_skeleton_groups(self.community_index_groups());
            if refined_groups.len() > self.community_count() {
                self.apply_community_index_groups(refined_groups);
                self.compact_community_ids();
            }
        }
        self.score_bridge_and_hub();
    }

    fn split_disconnected_communities(&mut self) {
        let groups = self.community_index_groups();
        for (community, indexes) in groups {
            if indexes.len() <= 1 {
                continue;
            }
            let mut local = BTreeSet::new();
            for index in &indexes {
                local.insert(*index);
            }
            let mut visited = BTreeSet::new();
            let mut part = 0usize;
            let mut starts = indexes.clone();
            starts.sort_by(|left, right| self.nodes[*left].id.cmp(&self.nodes[*right].id));
            for start in starts {
                if visited.contains(&start) {
                    continue;
                }
                part += 1;
                let mut queue = VecDeque::from([start]);
                visited.insert(start);
                let next_community = if part == 1 {
                    community.clone()
                } else {
                    format!("{community}:part:{part}")
                };
                while let Some(index) = queue.pop_front() {
                    self.nodes[index].community = next_community.clone();
                    let mut neighbors = self.adjacency[index]
                        .iter()
                        .filter_map(|(neighbor, _)| {
                            if local.contains(neighbor) {
                                Some(*neighbor)
                            } else {
                                None
                            }
                        })
                        .collect::<Vec<_>>();
                    neighbors
                        .sort_by(|left, right| self.nodes[*left].id.cmp(&self.nodes[*right].id));
                    for neighbor in neighbors {
                        if visited.insert(neighbor) {
                            queue.push_back(neighbor);
                        }
                    }
                }
            }
        }
    }

    fn community_index_groups(&self) -> BTreeMap<String, Vec<usize>> {
        let mut groups: BTreeMap<String, Vec<usize>> = BTreeMap::new();
        for (index, node) in self.nodes.iter().enumerate() {
            groups
                .entry(node.community.clone())
                .or_default()
                .push(index);
        }
        groups
    }

    fn apply_community_index_groups(&mut self, groups: BTreeMap<String, Vec<usize>>) {
        for (community, indexes) in groups {
            for index in indexes {
                if index < self.nodes.len() {
                    self.nodes[index].community = community.clone();
                }
            }
        }
    }

    fn compact_community_ids(&mut self) {
        let mut groups: BTreeMap<String, Vec<String>> = BTreeMap::new();
        for node in &self.nodes {
            groups
                .entry(node.community.clone())
                .or_default()
                .push(node.id.clone());
        }
        let mut group_rows = groups
            .into_iter()
            .map(|(key, mut ids)| {
                ids.sort();
                (key, ids.len(), ids)
            })
            .collect::<Vec<_>>();
        group_rows.sort_by(|a, b| b.1.cmp(&a.1).then_with(|| a.0.cmp(&b.0)));
        let mut remap = BTreeMap::new();
        for (index, (key, _, _)) in group_rows.iter().enumerate() {
            remap.insert(key.clone(), format!("c{}", index + 1));
        }
        for node in &mut self.nodes {
            if let Some(next) = remap.get(&node.community) {
                node.community = next.clone();
            }
        }
    }

    fn score_bridge_and_hub(&mut self) {
        let max_weighted = self
            .nodes
            .iter()
            .map(|node| node.weighted_degree)
            .fold(0.0_f64, f64::max)
            .max(1.0);
        for index in 0..self.nodes.len() {
            let own = self.nodes[index].community.clone();
            let mut neighbor_communities = BTreeSet::new();
            let mut external = 0.0;
            let mut total = 0.0;
            for (neighbor, weight) in &self.adjacency[index] {
                let community = self.nodes[*neighbor].community.clone();
                neighbor_communities.insert(community.clone());
                total += *weight;
                if community != own {
                    external += *weight;
                }
            }
            let diversity = neighbor_communities.len().saturating_sub(1) as f64;
            let external_ratio = if total > 0.0 { external / total } else { 0.0 };
            self.nodes[index].bridge_score =
                (external_ratio * 0.72 + (diversity / 4.0).min(1.0) * 0.28).clamp(0.0, 1.0);
            self.nodes[index].hub_score =
                (self.nodes[index].weighted_degree / max_weighted).clamp(0.0, 1.0);
        }
    }

    fn prepare_community_quality_only(&mut self, mode: NetworkPlanMode) -> bool {
        let mut tier_hint_count = 0usize;
        for node in &mut self.nodes {
            node.community = if !node.layout_community_id.is_empty() {
                node.layout_community_id.clone()
            } else if !node.cluster_id.is_empty() {
                node.cluster_id.clone()
            } else {
                format!("seed:{}", node.id)
            };
            if node.has_layout_visibility_tier || node.has_layout_label_tier {
                tier_hint_count += 1;
            }
            if node.visibility_tier == "hidden" && !node.has_layout_label_tier {
                node.label_tier = "hidden".to_string();
            }
        }
        self.score_bridge_and_hub();
        let has_tier_coverage = tier_hint_count.saturating_mul(2) >= self.nodes.len().max(1);
        if !has_tier_coverage {
            self.score_visibility(mode);
        }
        has_tier_coverage
    }

    fn assign_xlarge_skeleton_fast(&mut self) {
        let mut base_groups: BTreeMap<String, Vec<usize>> = BTreeMap::new();
        let max_weighted = self
            .nodes
            .iter()
            .map(|node| node.weighted_degree)
            .fold(0.0_f64, f64::max)
            .max(1.0);
        for (index, node) in self.nodes.iter_mut().enumerate() {
            let community = if !node.cluster_id.is_empty() {
                node.cluster_id.clone()
            } else {
                format!("x{}", index / 80)
            };
            node.community = community.clone();
            node.bridge_score = 0.0;
            node.hub_score = (node.weighted_degree / max_weighted).clamp(0.0, 1.0);
            node.visibility_tier = "hidden".to_string();
            node.label_tier = "hidden".to_string();
            base_groups.entry(community).or_default().push(index);
        }

        let groups = self.refine_xlarge_skeleton_groups(base_groups);
        for (community, indexes) in &groups {
            for index in indexes {
                self.nodes[*index].community = community.clone();
            }
        }

        let mut external_weight = vec![0.0_f64; self.nodes.len()];
        let mut total_weight = vec![0.0_f64; self.nodes.len()];
        for edge in &self.edges {
            total_weight[edge.source] += edge.weight;
            total_weight[edge.target] += edge.weight;
            if self.nodes[edge.source].community != self.nodes[edge.target].community {
                external_weight[edge.source] += edge.weight;
                external_weight[edge.target] += edge.weight;
            }
        }
        for index in 0..self.nodes.len() {
            self.nodes[index].bridge_score = if total_weight[index] > 0.0 {
                (external_weight[index] / total_weight[index]).clamp(0.0, 1.0)
            } else {
                0.0
            };
        }

        let center_communities = self.xlarge_center_community_summaries(&groups);
        let centers = self.topology_community_centers(&center_communities, NetworkPlanMode::XLarge);
        let mut group_rows = groups.into_iter().collect::<Vec<_>>();
        group_rows.sort_by(|a, b| b.1.len().cmp(&a.1.len()).then_with(|| a.0.cmp(&b.0)));
        let mut primary_promoted = 0usize;
        let mut secondary_promoted = 0usize;
        for (community, indexes) in group_rows.iter() {
            let Some((cx, cy)) = centers.get(community).copied() else {
                continue;
            };
            for index in indexes {
                self.nodes[*index].x = cx;
                self.nodes[*index].y = cy;
            }
            let (first, second) = self.best_two_indexes(indexes);
            if let Some(index) = first {
                if primary_promoted < XLARGE_PRIMARY_LIMIT {
                    self.place_xlarge_visible_node(
                        index,
                        cx,
                        cy,
                        24.0,
                        "primary",
                        "primary",
                        "community-anchor",
                    );
                    primary_promoted += 1;
                } else if secondary_promoted < XLARGE_SECONDARY_LIMIT {
                    self.place_xlarge_visible_node(
                        index,
                        cx,
                        cy,
                        52.0,
                        "secondary",
                        "hidden",
                        "hub",
                    );
                    secondary_promoted += 1;
                }
            }
            if let Some(index) = second {
                if secondary_promoted < XLARGE_SECONDARY_LIMIT {
                    self.place_xlarge_visible_node(
                        index,
                        cx,
                        cy,
                        52.0,
                        "secondary",
                        "hidden",
                        "hub",
                    );
                    secondary_promoted += 1;
                }
            }
        }
        self.promote_xlarge_bridge_endpoints(&centers, &mut secondary_promoted);
        self.reinforce_xlarge_visible_edge_coverage(&centers);
    }

    fn xlarge_center_community_summaries(
        &self,
        groups: &BTreeMap<String, Vec<usize>>,
    ) -> Vec<CommunitySummary> {
        if groups.is_empty() {
            return Vec::new();
        }

        let mut selected: BTreeSet<String> = BTreeSet::new();
        let mut rows = groups
            .iter()
            .map(|(id, indexes)| (id.clone(), indexes.len()))
            .collect::<Vec<_>>();
        rows.sort_by(|a, b| b.1.cmp(&a.1).then_with(|| a.0.cmp(&b.0)));
        for (id, _) in rows
            .iter()
            .take(XLARGE_CENTER_SIZE_LIMIT.min(XLARGE_CENTER_COMMUNITY_LIMIT))
        {
            selected.insert(id.clone());
        }

        let mut bridge_edges = self
            .edges
            .iter()
            .enumerate()
            .filter(|(_, edge)| {
                self.nodes[edge.source].community != self.nodes[edge.target].community
            })
            .map(|(index, edge)| {
                let source = &self.nodes[edge.source];
                let target = &self.nodes[edge.target];
                let score = edge.weight
                    * (1.0
                        + (source.bridge_score + target.bridge_score) * 0.85
                        + (source.hub_score + target.hub_score) * 0.35);
                (index, score)
            })
            .collect::<Vec<_>>();
        bridge_edges.sort_by(|left, right| {
            right
                .1
                .partial_cmp(&left.1)
                .unwrap_or(Ordering::Equal)
                .then_with(|| left.0.cmp(&right.0))
        });
        for (edge_index, _) in bridge_edges
            .into_iter()
            .take(XLARGE_CENTER_BRIDGE_EDGE_LIMIT)
        {
            if selected.len() >= XLARGE_CENTER_COMMUNITY_LIMIT {
                break;
            }
            let edge = &self.edges[edge_index];
            selected.insert(self.nodes[edge.source].community.clone());
            if selected.len() >= XLARGE_CENTER_COMMUNITY_LIMIT {
                break;
            }
            selected.insert(self.nodes[edge.target].community.clone());
        }

        if selected.is_empty() {
            if let Some((id, _)) = rows.first() {
                selected.insert(id.clone());
            }
        }

        let mut out = Vec::new();
        for id in selected {
            let Some(indexes) = groups.get(&id) else {
                continue;
            };
            out.push(self.community_summary_for_indexes(&id, indexes, false));
        }
        out.sort_by(|a, b| {
            b.node_count
                .cmp(&a.node_count)
                .then_with(|| a.id.cmp(&b.id))
        });
        out
    }

    fn place_xlarge_visible_node(
        &mut self,
        index: usize,
        cx: f64,
        cy: f64,
        radius: f64,
        visibility_tier: &str,
        label_tier: &str,
        community_role: &str,
    ) {
        let community = self.nodes[index].community.clone();
        let local_angle = TWO_PI
            * stable_hash_unit(&format!(
                "network-xlarge-local|{}|{}|{}",
                community, visibility_tier, self.nodes[index].id
            ));
        self.nodes[index].x = cx + local_angle.cos() * radius;
        self.nodes[index].y = cy + local_angle.sin() * radius;
        self.nodes[index].visibility_tier = visibility_tier.to_string();
        self.nodes[index].label_tier = label_tier.to_string();
        self.nodes[index].community_role = community_role.to_string();
    }

    fn promote_xlarge_bridge_endpoints(
        &mut self,
        centers: &BTreeMap<String, (f64, f64)>,
        secondary_promoted: &mut usize,
    ) {
        if *secondary_promoted >= XLARGE_SECONDARY_LIMIT || self.edges.is_empty() {
            return;
        }
        let mut visible_by_community: BTreeMap<String, usize> = BTreeMap::new();
        for node in &self.nodes {
            if node.visibility_tier != "hidden" {
                *visible_by_community
                    .entry(node.community.clone())
                    .or_insert(0) += 1;
            }
        }
        let mut bridge_edges = self
            .edges
            .iter()
            .enumerate()
            .filter(|(_, edge)| {
                self.nodes[edge.source].community != self.nodes[edge.target].community
            })
            .map(|(index, edge)| {
                let source = &self.nodes[edge.source];
                let target = &self.nodes[edge.target];
                let bridge_score = source.bridge_score + target.bridge_score;
                let hub_score = source.hub_score + target.hub_score;
                (
                    index,
                    edge.weight * (1.0 + bridge_score * 0.85 + hub_score * 0.35),
                )
            })
            .collect::<Vec<_>>();
        bridge_edges.sort_by(|left, right| {
            right
                .1
                .partial_cmp(&left.1)
                .unwrap_or(Ordering::Equal)
                .then_with(|| left.0.cmp(&right.0))
        });
        for (edge_index, _) in bridge_edges {
            if *secondary_promoted >= XLARGE_SECONDARY_LIMIT {
                break;
            }
            let edge = &self.edges[edge_index];
            let endpoints = [edge.source, edge.target];
            for endpoint in endpoints {
                if *secondary_promoted >= XLARGE_SECONDARY_LIMIT {
                    break;
                }
                if self.nodes[endpoint].visibility_tier != "hidden" {
                    continue;
                }
                let community = self.nodes[endpoint].community.clone();
                let visible_count = visible_by_community.get(&community).copied().unwrap_or(0);
                if visible_count >= XLARGE_BRIDGE_COMMUNITY_VISIBLE_CAP {
                    continue;
                }
                let Some((cx, cy)) = centers.get(&community).copied() else {
                    continue;
                };
                self.place_xlarge_visible_node(
                    endpoint,
                    cx,
                    cy,
                    78.0,
                    "secondary",
                    "hidden",
                    "bridge",
                );
                *visible_by_community.entry(community).or_insert(0) += 1;
                *secondary_promoted += 1;
            }
        }
    }

    fn reinforce_xlarge_visible_edge_coverage(&mut self, centers: &BTreeMap<String, (f64, f64)>) {
        if self.edges.is_empty() {
            return;
        }
        let visible_target = self
            .nodes
            .iter()
            .filter(|node| node.visibility_tier != "hidden")
            .count()
            .clamp(
                XLARGE_PRIMARY_LIMIT,
                XLARGE_PRIMARY_LIMIT + XLARGE_SECONDARY_LIMIT,
            );
        if visible_target <= XLARGE_PRIMARY_LIMIT {
            return;
        }
        let primary: BTreeSet<usize> = self
            .nodes
            .iter()
            .enumerate()
            .filter(|(_, node)| node.visibility_tier == "primary")
            .map(|(index, _)| index)
            .collect();
        let mut keep: BTreeSet<usize> = primary.clone();
        let mut endpoint_counts: BTreeMap<usize, usize> = BTreeMap::new();
        let mut edge_rows = self
            .edges
            .iter()
            .enumerate()
            .map(|(index, edge)| {
                let source = &self.nodes[edge.source];
                let target = &self.nodes[edge.target];
                let cross = source.community != target.community;
                let primary_touch =
                    primary.contains(&edge.source) || primary.contains(&edge.target);
                let score = edge.weight * if cross { 2.6 } else { 1.25 }
                    + (source.bridge_score + target.bridge_score) * 2.8
                    + (source.hub_score + target.hub_score) * 1.4
                    + if primary_touch { 4.0 } else { 0.0 };
                (index, score)
            })
            .collect::<Vec<_>>();
        edge_rows.sort_by(|left, right| {
            right
                .1
                .partial_cmp(&left.1)
                .unwrap_or(Ordering::Equal)
                .then_with(|| left.0.cmp(&right.0))
        });
        let endpoint_limit = (XLARGE_EDGE_ENDPOINT_LIMIT * 2).max(10);
        let mut selected_edges = 0usize;
        for (edge_index, _) in edge_rows {
            if keep.len() >= visible_target || selected_edges >= XLARGE_EDGE_UPDATE_LIMIT {
                break;
            }
            let edge = &self.edges[edge_index];
            let mut added = 0usize;
            if !keep.contains(&edge.source) {
                added += 1;
            }
            if !keep.contains(&edge.target) {
                added += 1;
            }
            if keep.len() + added > visible_target {
                continue;
            }
            if (endpoint_counts.get(&edge.source).copied().unwrap_or(0) >= endpoint_limit)
                && (endpoint_counts.get(&edge.target).copied().unwrap_or(0) >= endpoint_limit)
            {
                continue;
            }
            keep.insert(edge.source);
            keep.insert(edge.target);
            *endpoint_counts.entry(edge.source).or_insert(0) += 1;
            *endpoint_counts.entry(edge.target).or_insert(0) += 1;
            selected_edges += 1;
        }
        if keep.len() < visible_target {
            let mut existing_visible = self
                .nodes
                .iter()
                .enumerate()
                .filter(|(index, node)| node.visibility_tier != "hidden" && !keep.contains(index))
                .map(|(index, node)| {
                    (
                        index,
                        node.hub_score
                            + node.bridge_score * 1.8
                            + node.weighted_degree.ln_1p() * 0.04,
                    )
                })
                .collect::<Vec<_>>();
            existing_visible.sort_by(|left, right| {
                right
                    .1
                    .partial_cmp(&left.1)
                    .unwrap_or(Ordering::Equal)
                    .then_with(|| left.0.cmp(&right.0))
            });
            for (index, _) in existing_visible {
                if keep.len() >= visible_target {
                    break;
                }
                keep.insert(index);
            }
        }
        for index in 0..self.nodes.len() {
            if primary.contains(&index) {
                continue;
            }
            if keep.contains(&index) {
                if self.nodes[index].visibility_tier == "hidden" {
                    let community = self.nodes[index].community.clone();
                    if let Some((cx, cy)) = centers.get(&community).copied() {
                        self.place_xlarge_visible_node(
                            index,
                            cx,
                            cy,
                            78.0,
                            "secondary",
                            "hidden",
                            "bridge",
                        );
                    } else {
                        self.nodes[index].visibility_tier = "secondary".to_string();
                        self.nodes[index].label_tier = "hidden".to_string();
                        self.nodes[index].community_role = "bridge".to_string();
                    }
                }
            } else if self.nodes[index].visibility_tier != "hidden" {
                self.nodes[index].visibility_tier = "hidden".to_string();
                self.nodes[index].label_tier = "hidden".to_string();
                self.nodes[index].community_role = "hidden".to_string();
            }
        }
    }

    fn refine_xlarge_skeleton_groups(
        &self,
        groups: BTreeMap<String, Vec<usize>>,
    ) -> BTreeMap<String, Vec<usize>> {
        let has_oversized = groups.values().any(|indexes| indexes.len() > 420);
        if !has_oversized || self.edges.is_empty() {
            return groups;
        }

        let mut refined: BTreeMap<String, Vec<usize>> = BTreeMap::new();
        for (key, indexes) in groups {
            if indexes.len() <= 420 {
                refined.insert(key, indexes);
                continue;
            }

            let target_buckets = ((indexes.len() + 119) / 120).clamp(2, 260);
            let membership = indexes.iter().copied().collect::<BTreeSet<_>>();
            let mut ranked = indexes.clone();
            ranked.sort_by(|a, b| {
                if self.is_better_xlarge_seed(*b, *a) {
                    std::cmp::Ordering::Greater
                } else if self.is_better_xlarge_seed(*a, *b) {
                    std::cmp::Ordering::Less
                } else {
                    self.nodes[*a].id.cmp(&self.nodes[*b].id)
                }
            });
            let seeds = ranked.into_iter().take(target_buckets).collect::<Vec<_>>();
            let mut bucket_by_index: BTreeMap<usize, usize> = BTreeMap::new();
            let mut queue = VecDeque::new();
            for (bucket, index) in seeds.into_iter().enumerate() {
                bucket_by_index.insert(index, bucket);
                queue.push_back(index);
            }
            while let Some(current) = queue.pop_front() {
                let Some(bucket) = bucket_by_index.get(&current).copied() else {
                    continue;
                };
                for (neighbor, _) in &self.adjacency[current] {
                    if !membership.contains(neighbor) || bucket_by_index.contains_key(neighbor) {
                        continue;
                    }
                    bucket_by_index.insert(*neighbor, bucket);
                    queue.push_back(*neighbor);
                }
            }

            let mut buckets = vec![Vec::new(); target_buckets];
            for index in indexes {
                let bucket = bucket_by_index.get(&index).copied().unwrap_or_else(|| {
                    ((stable_hash_unit(&format!(
                        "network-xlarge-bucket|{}|{}",
                        key, self.nodes[index].id
                    )) * target_buckets as f64)
                        .floor() as usize)
                        .min(target_buckets - 1)
                });
                buckets[bucket].push(index);
            }
            for (bucket, bucket_indexes) in buckets.into_iter().enumerate() {
                if bucket_indexes.is_empty() {
                    continue;
                }
                refined.insert(format!("{key}#{bucket}"), bucket_indexes);
            }
        }
        refined
    }

    fn is_better_xlarge_seed(&self, left: usize, right: usize) -> bool {
        let a = &self.nodes[left];
        let b = &self.nodes[right];
        role_rank(&a.role)
            .cmp(&role_rank(&b.role))
            .then_with(|| {
                a.weighted_degree
                    .partial_cmp(&b.weighted_degree)
                    .unwrap_or(std::cmp::Ordering::Equal)
            })
            .then_with(|| a.degree.cmp(&b.degree))
            .then_with(|| b.id.cmp(&a.id))
            .is_gt()
    }

    fn best_two_indexes(&self, indexes: &[usize]) -> (Option<usize>, Option<usize>) {
        let mut first: Option<usize> = None;
        let mut second: Option<usize> = None;
        for index in indexes {
            let idx = *index;
            if first
                .map(|current| self.is_better_ranked_node(idx, current))
                .unwrap_or(true)
            {
                second = first;
                first = Some(idx);
            } else if second
                .map(|current| self.is_better_ranked_node(idx, current))
                .unwrap_or(true)
            {
                second = Some(idx);
            }
        }
        (first, second)
    }

    fn is_better_ranked_node(&self, left: usize, right: usize) -> bool {
        let a = &self.nodes[left];
        let b = &self.nodes[right];
        a.bridge_score
            .partial_cmp(&b.bridge_score)
            .unwrap_or(std::cmp::Ordering::Equal)
            .then_with(|| {
                a.hub_score
                    .partial_cmp(&b.hub_score)
                    .unwrap_or(std::cmp::Ordering::Equal)
            })
            .then_with(|| {
                a.weighted_degree
                    .partial_cmp(&b.weighted_degree)
                    .unwrap_or(std::cmp::Ordering::Equal)
            })
            .then_with(|| a.degree.cmp(&b.degree))
            .then_with(|| b.id.cmp(&a.id))
            .is_gt()
    }

    fn seed_topology_mode(&mut self, mode: NetworkPlanMode) {
        if matches!(mode, NetworkPlanMode::Small | NetworkPlanMode::Medium) {
            self.seed_small_medium_topology(mode);
            return;
        }
        if !matches!(mode, NetworkPlanMode::Large | NetworkPlanMode::XLarge) {
            return;
        }
        let communities = self.community_summaries();
        if communities.len() <= 1 {
            return;
        }
        let centers = self.topology_community_centers(&communities, mode);
        let mut ordinal_by_community: BTreeMap<String, usize> = BTreeMap::new();
        for node in &mut self.nodes {
            let ordinal = ordinal_by_community
                .entry(node.community.clone())
                .or_insert(0);
            let Some((cx, cy)) = centers.get(&node.community).copied() else {
                continue;
            };
            let local_angle = TWO_PI * stable_hash_unit(&format!("{}|{}", node.community, node.id));
            let local_r = 26.0 + (*ordinal as f64).sqrt() * 32.0;
            let target_x = cx + local_angle.cos() * local_r;
            let target_y = cy + local_angle.sin() * local_r;
            node.x = node.x * 0.34 + target_x * 0.66;
            node.y = node.y * 0.34 + target_y * 0.66;
            *ordinal += 1;
        }
    }

    fn semantic_group_key_for_index(&self, index: usize) -> String {
        let Some(node) = self.nodes.get(index) else {
            return String::new();
        };
        let key = semantic_group_key(node);
        if key.is_empty() {
            format!("seed:{}", node.id)
        } else {
            key.to_string()
        }
    }

    fn same_semantic_group(&self, left: usize, right: usize) -> bool {
        let (Some(left_node), Some(right_node)) = (self.nodes.get(left), self.nodes.get(right))
        else {
            return false;
        };
        let left_key = semantic_group_key(left_node);
        let right_key = semantic_group_key(right_node);
        if left_key.is_empty() || right_key.is_empty() {
            return left_node.community == right_node.community;
        }
        left_key == right_key
    }

    fn seed_small_medium_topology(&mut self, mode: NetworkPlanMode) {
        if !matches!(mode, NetworkPlanMode::Small | NetworkPlanMode::Medium)
            || self.nodes.len() <= 1
        {
            return;
        }
        let components = self.connected_components();
        if components.len() <= 1 && self.edges.is_empty() {
            return;
        }
        let mut rows: Vec<TopologySeedRow> = Vec::new();
        let mut isolates_by_semantic: BTreeMap<String, Vec<usize>> = BTreeMap::new();
        let mut connected_semantic_keys = BTreeSet::new();
        for indexes in components {
            let edge_count = self.component_edge_count(&indexes);
            if edge_count == 0 {
                for index in indexes {
                    isolates_by_semantic
                        .entry(self.semantic_group_key_for_index(index))
                        .or_default()
                        .push(index);
                }
            } else {
                for index in &indexes {
                    connected_semantic_keys.insert(self.semantic_group_key_for_index(*index));
                }
                let min_id = indexes
                    .iter()
                    .filter_map(|index| self.nodes.get(*index))
                    .map(|node| node.id.clone())
                    .min()
                    .unwrap_or_default();
                rows.push(TopologySeedRow {
                    key: format!("component:{min_id}"),
                    indexes,
                    edge_count,
                    isolate_bin: false,
                });
            }
        }
        for indexes in isolates_by_semantic.values_mut() {
            indexes.sort_by(|left, right| self.nodes[*left].id.cmp(&self.nodes[*right].id));
        }
        for (semantic_key, indexes) in &isolates_by_semantic {
            if connected_semantic_keys.contains(semantic_key) {
                continue;
            }
            rows.push(TopologySeedRow {
                key: format!("isolates:{semantic_key}"),
                indexes: indexes.clone(),
                edge_count: 0,
                isolate_bin: true,
            });
        }
        if rows.is_empty() {
            return;
        }
        rows.sort_by(|left, right| {
            right
                .edge_count
                .cmp(&left.edge_count)
                .then_with(|| right.indexes.len().cmp(&left.indexes.len()))
                .then_with(|| left.key.cmp(&right.key))
        });

        let spacing = if matches!(mode, NetworkPlanMode::Small) {
            360.0
        } else {
            430.0
        };
        let mut semantic_anchor_acc: BTreeMap<String, (f64, f64, usize)> = BTreeMap::new();
        for (row_index, row) in rows.iter().enumerate() {
            let jitter_x = (stable_hash_unit(&format!("network-topology-seed-x|{}", row.key))
                - 0.5)
                * spacing
                * 0.10;
            let jitter_y = (stable_hash_unit(&format!("network-topology-seed-y|{}", row.key))
                - 0.5)
                * spacing
                * 0.10;
            let (cx, cy) = if row_index == 0 {
                (jitter_x * 0.25, jitter_y * 0.25)
            } else {
                let ordinal = (row_index - 1) as f64;
                let angle = ordinal * 2.399963229728653
                    + (stable_hash_unit(&format!("network-topology-ring-angle|{}", row.key)) - 0.5)
                        * 0.34;
                let radius = spacing * (0.72 + (ordinal + 1.0).sqrt() * 0.48);
                (
                    angle.cos() * radius + jitter_x,
                    angle.sin() * radius * 0.78 + jitter_y,
                )
            };
            if row.isolate_bin {
                self.seed_isolate_bin(&row.indexes, cx, cy, mode);
            } else {
                self.seed_connected_component(&row.indexes, cx, cy, mode);
                for index in &row.indexes {
                    if self
                        .nodes
                        .get(*index)
                        .map(|node| node.degree == 0)
                        .unwrap_or(true)
                    {
                        continue;
                    }
                    let key = self.semantic_group_key_for_index(*index);
                    let node = &self.nodes[*index];
                    semantic_anchor_acc
                        .entry(key)
                        .and_modify(|(sum_x, sum_y, count)| {
                            *sum_x += node.x;
                            *sum_y += node.y;
                            *count += 1;
                        })
                        .or_insert((node.x, node.y, 1));
                }
            }
        }
        let semantic_anchors = semantic_anchor_acc
            .into_iter()
            .filter_map(|(key, (sum_x, sum_y, count))| {
                if count == 0 {
                    None
                } else {
                    Some((key, (sum_x / count as f64, sum_y / count as f64)))
                }
            })
            .collect::<BTreeMap<_, _>>();
        for (semantic_key, indexes) in &isolates_by_semantic {
            if !connected_semantic_keys.contains(semantic_key) {
                continue;
            }
            if let Some((cx, cy)) = semantic_anchors.get(semantic_key).copied() {
                self.seed_isolate_satellites(indexes, cx, cy, mode, semantic_key);
            }
        }
    }

    fn connected_components(&self) -> Vec<Vec<usize>> {
        let mut visited = vec![false; self.nodes.len()];
        let mut components = Vec::new();
        for start in 0..self.nodes.len() {
            if visited[start] {
                continue;
            }
            visited[start] = true;
            let mut queue = VecDeque::from([start]);
            let mut component = Vec::new();
            while let Some(index) = queue.pop_front() {
                component.push(index);
                for (neighbor, _) in &self.adjacency[index] {
                    if *neighbor >= visited.len() || visited[*neighbor] {
                        continue;
                    }
                    visited[*neighbor] = true;
                    queue.push_back(*neighbor);
                }
            }
            component.sort_by(|left, right| self.nodes[*left].id.cmp(&self.nodes[*right].id));
            components.push(component);
        }
        components
    }

    fn component_edge_count(&self, indexes: &[usize]) -> usize {
        if indexes.is_empty() {
            return 0;
        }
        let mut in_component = vec![false; self.nodes.len()];
        for index in indexes {
            if *index < in_component.len() {
                in_component[*index] = true;
            }
        }
        self.edges
            .iter()
            .filter(|edge| {
                in_component.get(edge.source).copied().unwrap_or(false)
                    && in_component.get(edge.target).copied().unwrap_or(false)
            })
            .count()
    }

    fn seed_isolate_bin(&mut self, indexes: &[usize], cx: f64, cy: f64, mode: NetworkPlanMode) {
        let count = indexes.len().max(1);
        let cols = (count as f64).sqrt().ceil().max(1.0) as usize;
        let spacing = if matches!(mode, NetworkPlanMode::Small) {
            70.0
        } else {
            76.0
        };
        for (ordinal, index) in indexes.iter().enumerate() {
            if *index >= self.nodes.len() {
                continue;
            }
            let col = ordinal % cols;
            let row = ordinal / cols;
            let jitter =
                (stable_hash_unit(&format!("network-isolate-seed|{}", self.nodes[*index].id))
                    - 0.5)
                    * 8.0;
            let target_x =
                cx + (col as f64 - (cols.saturating_sub(1) as f64) / 2.0) * spacing + jitter;
            let target_y = cy
                + (row as f64 - (((count + cols - 1) / cols).saturating_sub(1) as f64) / 2.0)
                    * spacing
                - jitter;
            self.apply_topology_seed(*index, target_x, target_y, 0.90);
        }
    }

    fn seed_isolate_satellites(
        &mut self,
        indexes: &[usize],
        cx: f64,
        cy: f64,
        mode: NetworkPlanMode,
        semantic_key: &str,
    ) {
        if indexes.is_empty() {
            return;
        }
        let base_radius = if matches!(mode, NetworkPlanMode::Small) {
            112.0
        } else {
            132.0
        };
        let ring_step = if matches!(mode, NetworkPlanMode::Small) {
            58.0
        } else {
            66.0
        };
        let per_ring = if matches!(mode, NetworkPlanMode::Small) {
            8
        } else {
            10
        };
        let base_angle =
            TWO_PI * stable_hash_unit(&format!("network-isolate-anchor|{semantic_key}"));
        for (ordinal, index) in indexes.iter().enumerate() {
            if *index >= self.nodes.len() {
                continue;
            }
            let ring = ordinal / per_ring;
            let slot = ordinal % per_ring;
            let slots = per_ring.min(indexes.len().saturating_sub(ring * per_ring).max(1));
            let jitter = (stable_hash_unit(&format!(
                "network-isolate-satellite|{}",
                self.nodes[*index].id
            )) - 0.5)
                * 18.0;
            let radius = base_radius + ring as f64 * ring_step + jitter;
            let angle = base_angle + slot as f64 / slots as f64 * TWO_PI + ring as f64 * 0.41;
            let target_x = cx + angle.cos() * radius;
            let target_y = cy + angle.sin() * radius * 0.78;
            self.apply_topology_seed(*index, target_x, target_y, 0.94);
        }
    }

    fn seed_connected_component(
        &mut self,
        indexes: &[usize],
        cx: f64,
        cy: f64,
        mode: NetworkPlanMode,
    ) {
        if indexes.is_empty() {
            return;
        }
        let in_component = indexes.iter().copied().collect::<BTreeSet<_>>();
        let root = indexes
            .iter()
            .copied()
            .max_by(|left, right| {
                self.nodes[*left]
                    .degree
                    .cmp(&self.nodes[*right].degree)
                    .then_with(|| {
                        self.nodes[*left]
                            .weighted_degree
                            .partial_cmp(&self.nodes[*right].weighted_degree)
                            .unwrap_or(Ordering::Equal)
                    })
                    .then_with(|| {
                        role_rank(&self.nodes[*left].role).cmp(&role_rank(&self.nodes[*right].role))
                    })
                    .then_with(|| self.nodes[*right].id.cmp(&self.nodes[*left].id))
            })
            .unwrap_or(indexes[0]);
        let mut depth: BTreeMap<usize, usize> = BTreeMap::new();
        let mut queue = VecDeque::from([root]);
        depth.insert(root, 0);
        while let Some(index) = queue.pop_front() {
            let next_depth = depth.get(&index).copied().unwrap_or(0) + 1;
            let mut neighbors = self.adjacency[index]
                .iter()
                .filter_map(|(neighbor, _)| {
                    if in_component.contains(neighbor) {
                        Some(*neighbor)
                    } else {
                        None
                    }
                })
                .collect::<Vec<_>>();
            neighbors.sort_by(|left, right| self.nodes[*left].id.cmp(&self.nodes[*right].id));
            for neighbor in neighbors {
                if depth.contains_key(&neighbor) {
                    continue;
                }
                depth.insert(neighbor, next_depth);
                queue.push_back(neighbor);
            }
        }
        let mut by_depth: BTreeMap<usize, Vec<usize>> = BTreeMap::new();
        for index in indexes {
            by_depth
                .entry(depth.get(index).copied().unwrap_or(1))
                .or_default()
                .push(*index);
        }
        for rows in by_depth.values_mut() {
            rows.sort_by(|left, right| {
                self.nodes[*right]
                    .degree
                    .cmp(&self.nodes[*left].degree)
                    .then_with(|| self.nodes[*left].id.cmp(&self.nodes[*right].id))
            });
        }
        let component_scale = (indexes.len() as f64).sqrt();
        let base_ring = if matches!(mode, NetworkPlanMode::Small) {
            76.0
        } else {
            88.0
        };
        for (level, rows) in by_depth {
            if level == 0 {
                self.apply_topology_seed(root, cx, cy, 0.88);
                continue;
            }
            let ring_count = rows.len().max(1);
            let radius = base_ring
                + (level.saturating_sub(1) as f64) * base_ring * 0.92
                + component_scale * 8.0;
            let base_angle = TWO_PI
                * stable_hash_unit(&format!(
                    "network-component-angle|{}|{level}",
                    self.nodes[root].id
                ));
            for (ordinal, index) in rows.into_iter().enumerate() {
                let angle = base_angle
                    + (ordinal as f64 / ring_count as f64) * TWO_PI
                    + level as f64 * 0.31;
                let jitter =
                    (stable_hash_unit(&format!("network-component-node|{}", self.nodes[index].id))
                        - 0.5)
                        * 18.0;
                let target_x = cx + angle.cos() * (radius + jitter);
                let target_y = cy + angle.sin() * (radius * 0.82 - jitter * 0.15);
                self.apply_topology_seed(index, target_x, target_y, 0.82);
            }
        }
    }

    fn apply_topology_seed(
        &mut self,
        index: usize,
        target_x: f64,
        target_y: f64,
        topology_weight: f64,
    ) {
        if index >= self.nodes.len() {
            return;
        }
        let weight = topology_weight.clamp(0.0, 1.0);
        let semantic_x = self.nodes[index].x;
        let semantic_y = self.nodes[index].y;
        self.nodes[index].x = semantic_x * (1.0 - weight) + target_x * weight;
        self.nodes[index].y = semantic_y * (1.0 - weight) + target_y * weight;
        self.nodes[index].seed_x = target_x;
        self.nodes[index].seed_y = target_y;
    }

    fn topology_community_centers(
        &self,
        communities: &[CommunitySummary],
        mode: NetworkPlanMode,
    ) -> BTreeMap<String, (f64, f64)> {
        let mut rows = communities
            .iter()
            .map(|community| {
                let radius =
                    ((community.node_count as f64).sqrt() * 24.0 + 92.0).clamp(110.0, 420.0);
                (community.id.clone(), community.node_count, radius)
            })
            .collect::<Vec<_>>();
        rows.sort_by(|a, b| b.1.cmp(&a.1).then_with(|| a.0.cmp(&b.0)));
        if rows.is_empty() {
            return BTreeMap::new();
        }
        if rows.len() == 1 {
            return BTreeMap::from([(rows[0].0.clone(), (0.0, 0.0))]);
        }

        let mut index_by_id = BTreeMap::new();
        for (index, (id, _, _)) in rows.iter().enumerate() {
            index_by_id.insert(id.clone(), index);
        }

        let count = rows.len();
        let cols = (count as f64).sqrt().ceil().max(1.0) as usize;
        let total_rows = ((count + cols - 1) / cols).max(1);
        let spacing = match mode {
            NetworkPlanMode::XLarge => 420.0,
            NetworkPlanMode::Large => 520.0,
            NetworkPlanMode::Medium => 470.0,
            NetworkPlanMode::Small => 420.0,
        };
        let mut x = vec![0.0_f64; count];
        let mut y = vec![0.0_f64; count];
        let radius = rows
            .iter()
            .map(|(_, _, radius)| *radius)
            .collect::<Vec<_>>();
        for (index, (id, _, _)) in rows.iter().enumerate() {
            let col = index % cols;
            let row = index / cols;
            let jitter_x = (stable_hash_unit(&format!("network-community-center-x|{id}")) - 0.5)
                * spacing
                * 0.18;
            let jitter_y = (stable_hash_unit(&format!("network-community-center-y|{id}")) - 0.5)
                * spacing
                * 0.18;
            x[index] = (col as f64 - (cols.saturating_sub(1) as f64) / 2.0) * spacing + jitter_x;
            y[index] =
                (row as f64 - (total_rows.saturating_sub(1) as f64) / 2.0) * spacing + jitter_y;
        }

        let mut pair_weight: BTreeMap<(usize, usize), f64> = BTreeMap::new();
        for edge in &self.edges {
            let source_community = &self.nodes[edge.source].community;
            let target_community = &self.nodes[edge.target].community;
            if source_community == target_community {
                continue;
            }
            let Some(source) = index_by_id.get(source_community).copied() else {
                continue;
            };
            let Some(target) = index_by_id.get(target_community).copied() else {
                continue;
            };
            let key = if source < target {
                (source, target)
            } else {
                (target, source)
            };
            *pair_weight.entry(key).or_insert(0.0) += edge.weight.max(0.05);
        }
        let max_pair_weight = pair_weight
            .values()
            .copied()
            .fold(0.0_f64, f64::max)
            .max(1.0);
        let edges = pair_weight.into_iter().collect::<Vec<_>>();
        let iterations = match mode {
            NetworkPlanMode::XLarge => 24,
            NetworkPlanMode::Large => 64,
            _ => 56,
        };
        for iter in 0..iterations {
            let cooling = 1.0 - (iter as f64 / iterations as f64) * 0.72;
            let max_step = match mode {
                NetworkPlanMode::XLarge => 58.0,
                NetworkPlanMode::Large => 66.0,
                _ => 54.0,
            } * cooling.max(0.26);
            let mut fx = vec![0.0_f64; count];
            let mut fy = vec![0.0_f64; count];
            for i in 0..count {
                for j in (i + 1)..count {
                    let mut dx = x[j] - x[i];
                    let mut dy = y[j] - y[i];
                    if dx.abs() + dy.abs() < 1e-6 {
                        let angle = TWO_PI
                            * stable_hash_unit(&format!(
                                "network-community-overlap|{}|{}",
                                rows[i].0, rows[j].0
                            ));
                        dx = angle.cos() * 0.1;
                        dy = angle.sin() * 0.1;
                    }
                    let dist_sq = (dx * dx + dy * dy).max(16.0);
                    let dist = dist_sq.sqrt();
                    let min_dist = radius[i] + radius[j] + 120.0;
                    let mut force = (min_dist * min_dist * 0.32) / dist_sq;
                    if dist < min_dist {
                        force += (min_dist - dist) * 0.075;
                    }
                    let ux = dx / dist;
                    let uy = dy / dist;
                    fx[i] -= ux * force;
                    fy[i] -= uy * force;
                    fx[j] += ux * force;
                    fy[j] += uy * force;
                }
            }
            for ((source, target), weight) in &edges {
                let dx = x[*target] - x[*source];
                let dy = y[*target] - y[*source];
                let dist = (dx * dx + dy * dy).sqrt().max(0.01);
                let normalized_weight =
                    (weight.ln_1p() / max_pair_weight.ln_1p().max(1.0)).clamp(0.18, 1.0);
                let ideal = (radius[*source] + radius[*target] + 220.0 / normalized_weight.sqrt())
                    .clamp(260.0, spacing * 1.5);
                let force = (dist - ideal) * 0.010 * normalized_weight * cooling;
                let ux = dx / dist;
                let uy = dy / dist;
                fx[*source] += ux * force;
                fy[*source] += uy * force;
                fx[*target] -= ux * force;
                fy[*target] -= uy * force;
            }
            for i in 0..count {
                fx[i] -= x[i] * 0.0024;
                fy[i] -= y[i] * 0.0024;
                x[i] += fx[i].clamp(-max_step, max_step);
                y[i] += fy[i].clamp(-max_step, max_step);
            }
        }

        rows.into_iter()
            .enumerate()
            .map(|(index, (id, _, _))| (id, (x[index], y[index])))
            .collect()
    }

    fn run_force(&mut self, mode: NetworkPlanMode) {
        let n = self.nodes.len();
        if n <= 1 {
            return;
        }
        let iterations = iteration_budget(mode, n, self.edges.len());
        let repel_target = match mode {
            NetworkPlanMode::Small | NetworkPlanMode::Medium => 420,
            NetworkPlanMode::Large => 160,
            NetworkPlanMode::XLarge => 56,
        };
        let repel_sample_step = if n <= repel_target {
            1
        } else {
            (n as f64 / repel_target as f64).ceil().max(1.0) as usize
        };
        let attraction_scale = match mode {
            NetworkPlanMode::Small => 0.012,
            NetworkPlanMode::Medium => 0.010,
            NetworkPlanMode::Large => 0.007,
            NetworkPlanMode::XLarge => 0.005,
        };
        let repel_scale = match mode {
            NetworkPlanMode::Small => 5200.0,
            NetworkPlanMode::Medium => 4300.0,
            NetworkPlanMode::Large => 3100.0,
            NetworkPlanMode::XLarge => 2300.0,
        };
        let anchor_scale = match mode {
            NetworkPlanMode::Small => 0.010,
            NetworkPlanMode::Medium => 0.006,
            NetworkPlanMode::Large => 0.0025,
            NetworkPlanMode::XLarge => 0.0016,
        };
        let gravity_scale = match mode {
            NetworkPlanMode::Small => 0.0008,
            NetworkPlanMode::Medium => 0.0006,
            NetworkPlanMode::Large => 0.00042,
            NetworkPlanMode::XLarge => 0.00032,
        };

        for iter in 0..iterations {
            let cooling = 1.0 - (iter as f64 / iterations as f64) * 0.74;
            let max_step = match mode {
                NetworkPlanMode::Small => 28.0,
                NetworkPlanMode::Medium => 24.0,
                NetworkPlanMode::Large => 18.0,
                NetworkPlanMode::XLarge => 14.0,
            } * cooling.max(0.22);
            let mut fx = vec![0.0_f64; n];
            let mut fy = vec![0.0_f64; n];

            for edge in &self.edges {
                let (sx, sy) = (self.nodes[edge.source].x, self.nodes[edge.source].y);
                let (tx, ty) = (self.nodes[edge.target].x, self.nodes[edge.target].y);
                let dx = tx - sx;
                let dy = ty - sy;
                let dist = (dx * dx + dy * dy).sqrt().max(0.01);
                let ideal = (178.0 / edge.weight.sqrt()).clamp(76.0, 240.0)
                    + (self.nodes[edge.source].radius + self.nodes[edge.target].radius) * 0.55;
                let force = (dist - ideal) * attraction_scale * edge.weight * cooling;
                let ux = dx / dist;
                let uy = dy / dist;
                fx[edge.source] += ux * force;
                fy[edge.source] += uy * force;
                fx[edge.target] -= ux * force;
                fy[edge.target] -= uy * force;
            }

            for i in 0..n {
                for j in (i + 1)..n {
                    if repel_sample_step > 1 && ((i * 31 + j * 17) % repel_sample_step) != 0 {
                        continue;
                    }
                    let mut dx = self.nodes[j].x - self.nodes[i].x;
                    let mut dy = self.nodes[j].y - self.nodes[i].y;
                    if dx.abs() + dy.abs() < 1e-6 {
                        let angle = TWO_PI
                            * stable_hash_unit(&format!(
                                "network-overlap|{}|{}",
                                self.nodes[i].id, self.nodes[j].id
                            ));
                        dx = angle.cos() * 0.1;
                        dy = angle.sin() * 0.1;
                    }
                    let dist_sq = (dx * dx + dy * dy).max(9.0);
                    let dist = dist_sq.sqrt();
                    let min_dist =
                        self.nodes[i].collision_radius + self.nodes[j].collision_radius + 10.0;
                    let same_community = self.nodes[i].community == self.nodes[j].community;
                    let same_semantic = self.same_semantic_group(i, j);
                    let community_scale = if same_community || same_semantic {
                        0.68
                    } else {
                        1.12
                    };
                    let sampled_scale = repel_sample_step as f64;
                    let zero_i = self.nodes[i].degree == 0;
                    let zero_j = self.nodes[j].degree == 0;
                    let isolate_scale = match (zero_i, zero_j, same_semantic) {
                        (true, true, true) => 0.22,
                        (true, true, false) => 0.34,
                        (true, false, true) | (false, true, true) => 0.46,
                        (true, false, false) | (false, true, false) => 0.58,
                        _ => 1.0,
                    };
                    let mut force =
                        repel_scale * community_scale * isolate_scale * sampled_scale / dist_sq;
                    if dist < min_dist {
                        let collision_scale = if zero_i && zero_j {
                            0.45
                        } else if zero_i || zero_j {
                            0.70
                        } else {
                            1.0
                        };
                        force += (min_dist - dist) * 0.12 * collision_scale * sampled_scale;
                    }
                    let ux = dx / dist;
                    let uy = dy / dist;
                    fx[i] -= ux * force;
                    fy[i] -= uy * force;
                    fx[j] += ux * force;
                    fy[j] += uy * force;
                }
            }

            let centroids = self.community_centroids();
            for i in 0..n {
                if let Some((cx, cy, count)) = centroids.get(&self.nodes[i].community).copied() {
                    if count > 1 {
                        fx[i] += (cx - self.nodes[i].x) * 0.0028 * cooling;
                        fy[i] += (cy - self.nodes[i].y) * 0.0028 * cooling;
                    }
                }
                let role_anchor = match self.nodes[i].role.as_str() {
                    "core" => 2.4,
                    "adjacent" => 1.5,
                    _ => 0.82,
                };
                let isolate_anchor = if self.nodes[i].degree == 0 { 2.25 } else { 1.0 };
                fx[i] += (self.nodes[i].seed_x - self.nodes[i].x)
                    * anchor_scale
                    * role_anchor
                    * isolate_anchor
                    * cooling;
                fy[i] += (self.nodes[i].seed_y - self.nodes[i].y)
                    * anchor_scale
                    * role_anchor
                    * isolate_anchor
                    * cooling;
                fx[i] -= self.nodes[i].x * gravity_scale;
                fy[i] -= self.nodes[i].y * gravity_scale;
            }

            for i in 0..n {
                self.nodes[i].vx = (self.nodes[i].vx * 0.62 + fx[i]).clamp(-max_step, max_step);
                self.nodes[i].vy = (self.nodes[i].vy * 0.62 + fy[i]).clamp(-max_step, max_step);
                self.nodes[i].x += self.nodes[i].vx;
                self.nodes[i].y += self.nodes[i].vy;
            }
        }
    }

    fn remove_collisions(&mut self, mode: NetworkPlanMode) {
        let n = self.nodes.len();
        if n <= 1 {
            return;
        }
        let passes = match mode {
            NetworkPlanMode::Small => 12,
            NetworkPlanMode::Medium => 8,
            NetworkPlanMode::Large => 2,
            NetworkPlanMode::XLarge => 0,
        };
        let collision_target = match mode {
            NetworkPlanMode::Small | NetworkPlanMode::Medium => 520,
            NetworkPlanMode::Large => 220,
            NetworkPlanMode::XLarge => 96,
        };
        let sample_step = if n <= collision_target {
            1
        } else {
            (n as f64 / collision_target as f64).ceil() as usize
        };
        for _ in 0..passes {
            for i in 0..n {
                for j in (i + 1)..n {
                    if sample_step > 1 && ((i * 13 + j * 23) % sample_step) != 0 {
                        continue;
                    }
                    let mut dx = self.nodes[j].x - self.nodes[i].x;
                    let mut dy = self.nodes[j].y - self.nodes[i].y;
                    if dx.abs() + dy.abs() < 1e-6 {
                        let angle = TWO_PI
                            * stable_hash_unit(&format!(
                                "network-collision|{}|{}",
                                self.nodes[i].id, self.nodes[j].id
                            ));
                        dx = angle.cos() * 0.1;
                        dy = angle.sin() * 0.1;
                    }
                    let dist = (dx * dx + dy * dy).sqrt().max(0.01);
                    let label_min_dist =
                        self.nodes[i].collision_radius + self.nodes[j].collision_radius + 8.0;
                    let node_min_dist = self.nodes[i].radius + self.nodes[j].radius + 10.0;
                    let zero_i = self.nodes[i].degree == 0;
                    let zero_j = self.nodes[j].degree == 0;
                    let min_dist = if zero_i && zero_j {
                        node_min_dist.max(label_min_dist * 0.44)
                    } else if zero_i || zero_j {
                        node_min_dist.max(label_min_dist * 0.68)
                    } else {
                        label_min_dist
                    };
                    if dist >= min_dist {
                        continue;
                    }
                    let push_scale = if zero_i && zero_j {
                        0.34
                    } else if zero_i || zero_j {
                        0.44
                    } else {
                        0.52
                    };
                    let push = (min_dist - dist) * push_scale;
                    let ux = dx / dist;
                    let uy = dy / dist;
                    self.nodes[i].x -= ux * push;
                    self.nodes[i].y -= uy * push;
                    self.nodes[j].x += ux * push;
                    self.nodes[j].y += uy * push;
                }
            }
        }
    }

    fn remove_visible_node_overlaps(&mut self, mode: NetworkPlanMode) {
        let visible_indexes = self
            .nodes
            .iter()
            .enumerate()
            .filter(|(_, node)| {
                !matches!(mode, NetworkPlanMode::Large | NetworkPlanMode::XLarge)
                    || node.visibility_tier != "hidden"
            })
            .map(|(index, _)| index)
            .collect::<Vec<_>>();
        if visible_indexes.len() <= 1 {
            return;
        }
        let passes = match mode {
            NetworkPlanMode::Small => 1,
            NetworkPlanMode::Medium => 2,
            NetworkPlanMode::Large => 5,
            NetworkPlanMode::XLarge => 0,
        };
        for _ in 0..passes {
            for left_pos in 0..visible_indexes.len() {
                for right_pos in (left_pos + 1)..visible_indexes.len() {
                    let i = visible_indexes[left_pos];
                    let j = visible_indexes[right_pos];
                    let mut dx = self.nodes[j].x - self.nodes[i].x;
                    let mut dy = self.nodes[j].y - self.nodes[i].y;
                    if dx.abs() + dy.abs() < 1e-6 {
                        let angle = TWO_PI
                            * stable_hash_unit(&format!(
                                "network-visible-collision|{}|{}",
                                self.nodes[i].id, self.nodes[j].id
                            ));
                        dx = angle.cos() * 0.1;
                        dy = angle.sin() * 0.1;
                    }
                    let dist = (dx * dx + dy * dy).sqrt().max(0.01);
                    let min_dist = self.nodes[i].radius + self.nodes[j].radius + 12.0;
                    if dist >= min_dist {
                        continue;
                    }
                    let push = (min_dist - dist)
                        * if matches!(mode, NetworkPlanMode::Large) {
                            0.68
                        } else {
                            0.58
                        };
                    let ux = dx / dist;
                    let uy = dy / dist;
                    let left_lock = if self.nodes[i].visibility_tier == "primary" {
                        0.82
                    } else {
                        1.0
                    };
                    let right_lock = if self.nodes[j].visibility_tier == "primary" {
                        0.82
                    } else {
                        1.0
                    };
                    self.nodes[i].x -= ux * push * left_lock;
                    self.nodes[i].y -= uy * push * left_lock;
                    self.nodes[j].x += ux * push * right_lock;
                    self.nodes[j].y += uy * push * right_lock;
                }
            }
        }
    }

    fn score_visibility(&mut self, mode: NetworkPlanMode) {
        let mut by_community: BTreeMap<String, Vec<usize>> = BTreeMap::new();
        for (index, node) in self.nodes.iter().enumerate() {
            by_community
                .entry(node.community.clone())
                .or_default()
                .push(index);
        }
        for indexes in by_community.values_mut() {
            indexes.sort_by(|a, b| {
                self.nodes[*b]
                    .bridge_score
                    .partial_cmp(&self.nodes[*a].bridge_score)
                    .unwrap_or(std::cmp::Ordering::Equal)
                    .then_with(|| {
                        self.nodes[*b]
                            .hub_score
                            .partial_cmp(&self.nodes[*a].hub_score)
                            .unwrap_or(std::cmp::Ordering::Equal)
                    })
                    .then_with(|| {
                        self.nodes[*b]
                            .weighted_degree
                            .partial_cmp(&self.nodes[*a].weighted_degree)
                            .unwrap_or(std::cmp::Ordering::Equal)
                    })
                    .then_with(|| self.nodes[*a].id.cmp(&self.nodes[*b].id))
            });
        }

        for node in &mut self.nodes {
            let primary =
                node.role == "core" || node.hub_score >= 0.66 || node.bridge_score >= 0.42;
            let secondary =
                node.role == "adjacent" || node.hub_score >= 0.24 || node.bridge_score >= 0.18;
            let (visibility, label) = match mode {
                NetworkPlanMode::Small => {
                    if primary {
                        ("primary", "primary")
                    } else {
                        ("secondary", "secondary")
                    }
                }
                NetworkPlanMode::Medium => {
                    if primary {
                        ("primary", "primary")
                    } else if secondary {
                        ("secondary", "secondary")
                    } else {
                        ("secondary", "hidden")
                    }
                }
                NetworkPlanMode::Large => {
                    if node.role == "core" || node.bridge_score >= 0.42 {
                        ("primary", "primary")
                    } else {
                        ("hidden", "hidden")
                    }
                }
                NetworkPlanMode::XLarge => {
                    if node.role == "core" || node.bridge_score >= 0.42 {
                        ("primary", "primary")
                    } else {
                        ("hidden", "hidden")
                    }
                }
            };
            node.visibility_tier = visibility.to_string();
            node.label_tier = label.to_string();
        }

        let primary_per_community = match mode {
            NetworkPlanMode::Large => 1,
            NetworkPlanMode::XLarge => 1,
            _ => 0,
        };
        let mut ranked_communities = by_community.values().collect::<Vec<_>>();
        ranked_communities.sort_by(|a, b| {
            b.len().cmp(&a.len()).then_with(|| {
                let left = a
                    .first()
                    .map(|index| self.nodes[*index].id.as_str())
                    .unwrap_or("");
                let right = b
                    .first()
                    .map(|index| self.nodes[*index].id.as_str())
                    .unwrap_or("");
                left.cmp(right)
            })
        });
        let primary_promotion_limit = match mode {
            NetworkPlanMode::Large => 120usize,
            NetworkPlanMode::XLarge => 40usize,
            _ => usize::MAX,
        };
        let secondary_promotion_limit = match mode {
            NetworkPlanMode::Large => 280usize,
            NetworkPlanMode::XLarge => 120usize,
            _ => usize::MAX,
        };
        let mut primary_promoted = 0usize;
        for indexes in &ranked_communities {
            for index in indexes.iter().take(primary_per_community) {
                if primary_promoted >= primary_promotion_limit {
                    break;
                }
                self.nodes[*index].visibility_tier = "primary".to_string();
                self.nodes[*index].label_tier = "primary".to_string();
                primary_promoted += 1;
            }
        }

        let per_community_primary = if matches!(mode, NetworkPlanMode::XLarge) {
            1
        } else {
            3
        };
        let mut secondary_promoted = 0usize;
        for indexes in &ranked_communities {
            for index in indexes.iter().take(per_community_primary) {
                if secondary_promoted >= secondary_promotion_limit {
                    break;
                }
                if self.nodes[*index].visibility_tier == "hidden" {
                    self.nodes[*index].visibility_tier = "secondary".to_string();
                }
                if self.nodes[*index].label_tier == "hidden"
                    && !matches!(mode, NetworkPlanMode::XLarge)
                {
                    self.nodes[*index].label_tier = "secondary".to_string();
                }
                secondary_promoted += 1;
            }
        }
    }

    fn suppress_overlapping_visible_labels(&mut self, mode: NetworkPlanMode) -> usize {
        let max_labels = match mode {
            NetworkPlanMode::Small => 56usize,
            NetworkPlanMode::Medium => 72usize,
            NetworkPlanMode::Large => 64usize,
            NetworkPlanMode::XLarge => 8usize,
        };
        let mut candidates = self
            .nodes
            .iter()
            .enumerate()
            .filter(|(_, node)| node.visibility_tier != "hidden" && node.label_tier != "hidden")
            .map(|(index, _)| index)
            .collect::<Vec<_>>();
        candidates.sort_by(|left, right| {
            self.label_priority(*right)
                .partial_cmp(&self.label_priority(*left))
                .unwrap_or(Ordering::Equal)
                .then_with(|| {
                    self.nodes[*left]
                        .community
                        .cmp(&self.nodes[*right].community)
                })
                .then_with(|| self.nodes[*left].id.cmp(&self.nodes[*right].id))
        });
        let padding = match mode {
            NetworkPlanMode::Small => 8.0,
            NetworkPlanMode::Medium | NetworkPlanMode::Large => 10.0,
            NetworkPlanMode::XLarge => 14.0,
        };
        let mut accepted: Vec<(f64, f64, f64, f64)> = Vec::new();
        let mut suppressed = 0usize;
        for index in candidates {
            let label_box = expand_box(label_box(&self.nodes[index]), padding);
            let collides = accepted
                .iter()
                .any(|other| boxes_overlap(label_box, *other));
            if accepted.len() >= max_labels || collides {
                self.nodes[index].label_tier = "hidden".to_string();
                suppressed += 1;
            } else {
                accepted.push(label_box);
            }
        }
        suppressed
    }

    fn label_priority(&self, index: usize) -> f64 {
        let node = &self.nodes[index];
        let role_rank = match node.role.as_str() {
            "core" => 4.0,
            "bridge" => 3.0,
            "adjacent" => 2.0,
            _ => 1.0,
        };
        let visibility_rank = if node.visibility_tier == "primary" {
            2.0
        } else {
            1.0
        };
        visibility_rank * 10_000.0
            + role_rank * 1_000.0
            + node.bridge_score * 260.0
            + node.hub_score * 220.0
            + node.weighted_degree.max(0.0).ln_1p() * 80.0
    }

    fn community_centroids(&self) -> BTreeMap<String, (f64, f64, usize)> {
        let mut sum: BTreeMap<String, (f64, f64, usize)> = BTreeMap::new();
        for node in &self.nodes {
            let entry = sum.entry(node.community.clone()).or_insert((0.0, 0.0, 0));
            entry.0 += node.x;
            entry.1 += node.y;
            entry.2 += 1;
        }
        for value in sum.values_mut() {
            let denom = value.2.max(1) as f64;
            value.0 /= denom;
            value.1 /= denom;
        }
        sum
    }

    fn community_summaries(&self) -> Vec<CommunitySummary> {
        let mut groups: BTreeMap<String, Vec<usize>> = BTreeMap::new();
        for (index, node) in self.nodes.iter().enumerate() {
            groups
                .entry(node.community.clone())
                .or_default()
                .push(index);
        }
        let mut edge_weight_by_community: BTreeMap<String, f64> = BTreeMap::new();
        for edge in &self.edges {
            let source_community = self.nodes[edge.source].community.clone();
            let target_community = self.nodes[edge.target].community.clone();
            if source_community == target_community {
                *edge_weight_by_community
                    .entry(source_community)
                    .or_insert(0.0) += edge.weight;
            }
        }
        let mut out = Vec::new();
        for (id, indexes) in groups {
            let mut summary = self.community_summary_for_indexes(&id, &indexes, true);
            summary.edge_weight = *edge_weight_by_community.get(&id).unwrap_or(&0.0);
            out.push(summary);
        }
        out.sort_by(|a, b| {
            b.node_count
                .cmp(&a.node_count)
                .then_with(|| a.id.cmp(&b.id))
        });
        out
    }

    fn community_summary_for_indexes(
        &self,
        id: &str,
        indexes: &[usize],
        include_node_ids: bool,
    ) -> CommunitySummary {
        let mut min_x = f64::INFINITY;
        let mut min_y = f64::INFINITY;
        let mut max_x = f64::NEG_INFINITY;
        let mut max_y = f64::NEG_INFINITY;
        let mut sx = 0.0;
        let mut sy = 0.0;
        let mut node_ids = if include_node_ids {
            Vec::with_capacity(indexes.len())
        } else {
            Vec::new()
        };
        for index in indexes {
            let node = &self.nodes[*index];
            min_x = min_x.min(node.x - node.collision_radius);
            min_y = min_y.min(node.y - node.collision_radius);
            max_x = max_x.max(node.x + node.collision_radius);
            max_y = max_y.max(node.y + node.collision_radius);
            sx += node.x;
            sy += node.y;
            if include_node_ids {
                node_ids.push(node.id.clone());
            }
        }
        if include_node_ids {
            node_ids.sort();
        }
        let count = indexes.len().max(1);
        CommunitySummary {
            id: id.to_string(),
            node_ids,
            node_count: count,
            edge_weight: 0.0,
            x: sx / count as f64,
            y: sy / count as f64,
            radius: ((max_x - min_x).hypot(max_y - min_y) / 2.0).max(36.0),
            bbox: (min_x, min_y, max_x, max_y),
        }
    }

    fn community_count(&self) -> usize {
        let mut keys = BTreeSet::new();
        for node in &self.nodes {
            keys.insert(node.community.as_str());
        }
        keys.len()
    }

    fn visible_bridge_node_count(&self) -> usize {
        self.nodes
            .iter()
            .filter(|node| node.visibility_tier != "hidden" && node.bridge_score > 0.0)
            .count()
    }

    fn visible_community_summaries(&self) -> Vec<CommunitySummary> {
        let mut groups: BTreeMap<String, Vec<usize>> = BTreeMap::new();
        for (index, node) in self.nodes.iter().enumerate() {
            if node.visibility_tier == "hidden" {
                continue;
            }
            groups
                .entry(node.community.clone())
                .or_default()
                .push(index);
        }
        let mut out = Vec::new();
        for (id, indexes) in groups {
            let mut min_x = f64::INFINITY;
            let mut min_y = f64::INFINITY;
            let mut max_x = f64::NEG_INFINITY;
            let mut max_y = f64::NEG_INFINITY;
            let mut sx = 0.0;
            let mut sy = 0.0;
            let mut node_ids = Vec::new();
            for index in &indexes {
                let node = &self.nodes[*index];
                min_x = min_x.min(node.x - node.collision_radius);
                min_y = min_y.min(node.y - node.collision_radius);
                max_x = max_x.max(node.x + node.collision_radius);
                max_y = max_y.max(node.y + node.collision_radius);
                sx += node.x;
                sy += node.y;
                node_ids.push(node.id.clone());
            }
            node_ids.sort();
            let count = indexes.len().max(1);
            out.push(CommunitySummary {
                id: id.clone(),
                node_ids,
                node_count: count,
                edge_weight: 0.0,
                x: sx / count as f64,
                y: sy / count as f64,
                radius: ((max_x - min_x).hypot(max_y - min_y) / 2.0).max(36.0),
                bbox: (min_x, min_y, max_x, max_y),
            });
        }
        out.sort_by(|a, b| {
            b.node_count
                .cmp(&a.node_count)
                .then_with(|| a.id.cmp(&b.id))
        });
        out
    }

    fn community_summaries_for_indexes(&self, indexes: &[usize]) -> Vec<CommunitySummary> {
        let mut groups: BTreeMap<String, Vec<usize>> = BTreeMap::new();
        for index in indexes {
            if let Some(node) = self.nodes.get(*index) {
                groups
                    .entry(node.community.clone())
                    .or_default()
                    .push(*index);
            }
        }
        let mut out = Vec::new();
        for (id, indexes) in groups {
            let mut min_x = f64::INFINITY;
            let mut min_y = f64::INFINITY;
            let mut max_x = f64::NEG_INFINITY;
            let mut max_y = f64::NEG_INFINITY;
            let mut sx = 0.0;
            let mut sy = 0.0;
            let mut node_ids = Vec::new();
            for index in &indexes {
                let node = &self.nodes[*index];
                min_x = min_x.min(node.x - node.collision_radius);
                min_y = min_y.min(node.y - node.collision_radius);
                max_x = max_x.max(node.x + node.collision_radius);
                max_y = max_y.max(node.y + node.collision_radius);
                sx += node.x;
                sy += node.y;
                node_ids.push(node.id.clone());
            }
            node_ids.sort();
            let count = indexes.len().max(1);
            out.push(CommunitySummary {
                id: id.clone(),
                node_ids,
                node_count: count,
                edge_weight: 0.0,
                x: sx / count as f64,
                y: sy / count as f64,
                radius: ((max_x - min_x).hypot(max_y - min_y) / 2.0).max(36.0),
                bbox: (min_x, min_y, max_x, max_y),
            });
        }
        out.sort_by(|a, b| {
            b.node_count
                .cmp(&a.node_count)
                .then_with(|| a.id.cmp(&b.id))
        });
        out
    }

    fn node_updates(&self) -> Vec<Value> {
        let mut rows = self.nodes.iter().collect::<Vec<_>>();
        rows.sort_by_key(|node| node.index);
        rows.into_iter()
            .map(|node| {
                json!({
                    "index": node.index,
                    "id": node.id.as_str(),
                    "x": round2(node.x),
                    "y": round2(node.y),
                })
            })
            .collect()
    }

    fn community_visibility_counts(&self) -> BTreeMap<String, (usize, usize)> {
        let mut rows: BTreeMap<String, (usize, usize)> = BTreeMap::new();
        for node in &self.nodes {
            let entry = rows.entry(node.community.clone()).or_insert((0, 0));
            entry.0 += 1;
            if node.visibility_tier != "hidden" {
                entry.1 += 1;
            }
        }
        rows
    }

    fn node_community_role(&self, node: &LayoutNode) -> &'static str {
        match node.community_role.as_str() {
            "community-anchor" => "community-anchor",
            "bridge" => "bridge",
            "hub" => "hub",
            _ if node.visibility_tier == "hidden" => "hidden",
            _ if node.bridge_score >= 0.42 => "bridge",
            _ if node.hub_score >= 0.46 => "hub",
            _ => "member",
        }
    }

    fn edge_bundle_stats_by_pair(&self) -> BTreeMap<String, EdgeBundleStats> {
        let mut stats: BTreeMap<String, EdgeBundleStats> = BTreeMap::new();
        for edge in &self.edges {
            let pair = self.edge_update_community_pair(edge);
            let entry = stats.entry(pair.clone()).or_insert(EdgeBundleStats {
                pair,
                edge_count: 0,
                total_weight: 0.0,
                rank: 0,
            });
            entry.edge_count += 1;
            entry.total_weight += edge.raw_weight.max(edge.weight);
        }
        let mut rows = stats
            .values()
            .map(|row| (row.pair.clone(), row.total_weight, row.edge_count))
            .collect::<Vec<_>>();
        rows.sort_by(|left, right| {
            right
                .1
                .partial_cmp(&left.1)
                .unwrap_or(Ordering::Equal)
                .then_with(|| right.2.cmp(&left.2))
                .then_with(|| left.0.cmp(&right.0))
        });
        for (rank, (pair, _, _)) in rows.into_iter().enumerate() {
            if let Some(row) = stats.get_mut(&pair) {
                row.rank = rank + 1;
            }
        }
        stats
    }

    fn community_superedge_stats(
        &self,
        visible_community_ids: &BTreeSet<String>,
    ) -> Vec<CommunitySuperEdgeStats> {
        let mut stats: BTreeMap<(String, String), CommunitySuperEdgeStats> = BTreeMap::new();
        for edge in &self.edges {
            let source_community = self.nodes[edge.source].community.clone();
            let target_community = self.nodes[edge.target].community.clone();
            if source_community == target_community {
                continue;
            }
            if !visible_community_ids.is_empty()
                && (!visible_community_ids.contains(&source_community)
                    || !visible_community_ids.contains(&target_community))
            {
                continue;
            }
            let (left, right, representative_source, representative_target) =
                if source_community <= target_community {
                    (
                        source_community,
                        target_community,
                        self.nodes[edge.source].id.clone(),
                        self.nodes[edge.target].id.clone(),
                    )
                } else {
                    (
                        target_community,
                        source_community,
                        self.nodes[edge.target].id.clone(),
                        self.nodes[edge.source].id.clone(),
                    )
                };
            let key = (left.clone(), right.clone());
            let candidate_weight = edge.raw_weight.max(edge.weight);
            let entry = stats.entry(key).or_insert(CommunitySuperEdgeStats {
                source_community: left,
                target_community: right,
                edge_count: 0,
                total_weight: 0.0,
                representative_source: representative_source.clone(),
                representative_target: representative_target.clone(),
                representative_weight: candidate_weight,
                rank: 0,
            });
            entry.edge_count += 1;
            entry.total_weight += candidate_weight;
            if candidate_weight > entry.representative_weight
                || (candidate_weight == entry.representative_weight
                    && (
                        representative_source.as_str(),
                        representative_target.as_str(),
                    ) < (
                        entry.representative_source.as_str(),
                        entry.representative_target.as_str(),
                    ))
            {
                entry.representative_source = representative_source;
                entry.representative_target = representative_target;
                entry.representative_weight = candidate_weight;
            }
        }
        let mut rows = stats.into_values().collect::<Vec<_>>();
        rows.sort_by(|left, right| {
            right
                .total_weight
                .partial_cmp(&left.total_weight)
                .unwrap_or(Ordering::Equal)
                .then_with(|| right.edge_count.cmp(&left.edge_count))
                .then_with(|| left.source_community.cmp(&right.source_community))
                .then_with(|| left.target_community.cmp(&right.target_community))
        });
        for (rank, row) in rows.iter_mut().enumerate() {
            row.rank = rank + 1;
        }
        rows
    }

    fn supergraph_payload(
        &self,
        mode: NetworkPlanMode,
        communities: &[CommunitySummary],
        edge_updates: &[Value],
    ) -> Value {
        let community_counts = self.community_visibility_counts();
        let visible_community_ids = communities
            .iter()
            .map(|community| community.id.clone())
            .collect::<BTreeSet<_>>();
        let nodes = communities
            .iter()
            .map(|community| {
                let (node_count, visible_node_count) = community_counts
                    .get(&community.id)
                    .copied()
                    .unwrap_or((community.node_count, community.node_count));
                let hidden_node_count = node_count.saturating_sub(visible_node_count);
                json!({
                    "id": format!("community:{}", community.id),
                    "communityId": community.id.as_str(),
                    "layoutRole": "community-supernode",
                    "x": round2(community.x),
                    "y": round2(community.y),
                    "radius": round2(community.radius),
                    "nodeCount": node_count,
                    "visibleNodeCount": visible_node_count,
                    "hiddenNodeCount": hidden_node_count,
                    "edgeWeight": round2(community.edge_weight),
                    "representativeNodeIds": community.node_ids.iter().take(8).cloned().collect::<Vec<_>>(),
                    "qualityScope": "all",
                })
            })
            .collect::<Vec<_>>();
        let edge_limit = match mode {
            NetworkPlanMode::XLarge => 320usize,
            NetworkPlanMode::Large => 480usize,
            _ => 800usize,
        };
        let edges = self
            .community_superedge_stats(&visible_community_ids)
            .into_iter()
            .take(edge_limit)
            .map(|row| {
                let source_community = row.source_community;
                let target_community = row.target_community;
                json!({
                    "id": format!("{source_community}|{target_community}"),
                    "sourceCommunity": source_community.as_str(),
                    "targetCommunity": target_community.as_str(),
                    "source": format!("community:{source_community}"),
                    "target": format!("community:{target_community}"),
                    "layoutRole": "community-superedge",
                    "edgeCount": row.edge_count,
                    "weight": round2(row.total_weight),
                    "rank": row.rank,
                    "representativeSource": row.representative_source,
                    "representativeTarget": row.representative_target,
                    "representativeWeight": round2(row.representative_weight),
                })
            })
            .collect::<Vec<_>>();
        json!({
            "mode": mode.as_str(),
            "qualityScope": "all",
            "nodes": nodes,
            "edges": edges,
            "nodeCount": nodes.len(),
            "edgeCount": edges.len(),
            "visibleNodeUpdateCount": communities.iter().map(|row| row.node_ids.len()).sum::<usize>(),
            "visibleEdgeUpdateCount": edge_updates.len(),
            "edgeLimit": edge_limit,
        })
    }

    fn edge_update_candidate_score(
        &self,
        edge: &LayoutEdge,
        bundle_stats: &BTreeMap<String, EdgeBundleStats>,
    ) -> f64 {
        let pair = self.edge_update_community_pair(edge);
        let bundle = bundle_stats.get(&pair);
        let same = self.nodes[edge.source].community == self.nodes[edge.target].community;
        let bundle_weight = bundle.map(|row| row.total_weight).unwrap_or(edge.weight);
        let bundle_count = bundle.map(|row| row.edge_count).unwrap_or(1) as f64;
        let bundle_score = bundle_weight.max(0.0).ln_1p() + bundle_count.sqrt() * 0.3;
        edge.weight * if same { 1.0 } else { 0.75 } + bundle_score * if same { 0.2 } else { 2.4 }
    }

    fn edge_bundle_route(
        &self,
        edge: &LayoutEdge,
        bundle_id: &str,
        bundle_size: usize,
        bundle_weight: f64,
    ) -> Option<(f64, &'static str, f64)> {
        if self.nodes[edge.source].community == self.nodes[edge.target].community
            || bundle_size <= 1
        {
            return None;
        }
        let source = &self.nodes[edge.source];
        let target = &self.nodes[edge.target];
        let dx = target.x - source.x;
        let dy = target.y - source.y;
        let distance = (dx * dx + dy * dy).sqrt();
        if !distance.is_finite() || distance <= 1.0 {
            return None;
        }
        let hash = stable_hash_unit(&format!("network-superedge-route|{bundle_id}"));
        let sign = if hash >= 0.5 { 1.0 } else { -1.0 };
        let magnitude =
            (18.0 + (bundle_size as f64).ln_1p() * 11.0 + bundle_weight.max(0.0).ln_1p() * 0.9)
                .clamp(22.0, 92.0);
        let axis = if dx.abs() >= dy.abs() {
            "horizontal"
        } else {
            "vertical"
        };
        let bias = 0.34 + stable_hash_unit(&format!("network-superedge-bias|{bundle_id}")) * 0.32;
        Some((round2(sign * magnitude), axis, round4(bias)))
    }

    fn edge_updates(&self, mode: NetworkPlanMode) -> Vec<Value> {
        let _ = mode;
        Vec::new()
    }

    fn lod_payload(
        &self,
        mode: NetworkPlanMode,
        node_updates: &[Value],
        _edge_updates: &[Value],
    ) -> Value {
        json!({
            "mode": mode.as_str(),
            "visibleNodeCount": node_updates.len(),
            "visibleEdgeCount": self.edges.len(),
            "totalNodeCount": self.nodes.len(),
            "totalEdgeCount": self.edges.len(),
            "budgets": {
                "nodeUpdateLimit": node_update_limit_for_mode(mode, self.nodes.len()),
                "edgeUpdateLimit": edge_update_limit_for_mode(mode),
                "runtimeNodeBudget": runtime_node_budget_for_mode(mode, self.nodes.len()),
                "runtimeEdgeBudget": runtime_edge_budget_for_mode(mode, self.edges.len()),
                "edgeEndpointLimit": edge_endpoint_limit_for_mode(mode),
                "minEdgeFillRatio": edge_min_fill_ratio_for_mode(mode),
            },
        })
    }

    fn node_tier_counts(&self) -> BTreeMap<String, usize> {
        let mut tiers = BTreeMap::new();
        for node in &self.nodes {
            let tier = if node.visibility_tier.trim().is_empty() {
                "visible"
            } else {
                node.visibility_tier.as_str()
            };
            *tiers.entry(tier.to_string()).or_insert(0) += 1;
        }
        tiers
    }

    fn label_tier_counts(&self) -> BTreeMap<String, usize> {
        let mut tiers = BTreeMap::new();
        for node in &self.nodes {
            let tier = if node.label_tier.trim().is_empty() {
                "visible"
            } else {
                node.label_tier.as_str()
            };
            *tiers.entry(tier.to_string()).or_insert(0) += 1;
        }
        tiers
    }

    fn edge_tier_counts(&self, mode: NetworkPlanMode) -> BTreeMap<String, usize> {
        let mut tiers = BTreeMap::new();
        for edge in &self.edges {
            *tiers
                .entry(self.edge_lod_tier(mode, edge).to_string())
                .or_insert(0) += 1;
        }
        tiers
    }

    fn edge_lod_tier(&self, mode: NetworkPlanMode, edge: &LayoutEdge) -> &'static str {
        if self.nodes[edge.source].visibility_tier == "hidden"
            || self.nodes[edge.target].visibility_tier == "hidden"
        {
            "hidden"
        } else if matches!(mode, NetworkPlanMode::Small | NetworkPlanMode::Medium) {
            if edge.weight >= 2.05
                || self.nodes[edge.source].community != self.nodes[edge.target].community
            {
                "primary"
            } else {
                "secondary"
            }
        } else if edge.weight >= 2.05
            || self.nodes[edge.source].community != self.nodes[edge.target].community
        {
            "primary"
        } else if edge.weight >= 1.05 {
            "secondary"
        } else {
            "hidden"
        }
    }

    fn edge_update_community_pair(&self, edge: &LayoutEdge) -> String {
        let source = self.nodes[edge.source].community.as_str();
        let target = self.nodes[edge.target].community.as_str();
        if source <= target {
            format!("{source}|{target}")
        } else {
            format!("{target}|{source}")
        }
    }

    fn can_take_edge_update(
        &self,
        mode: NetworkPlanMode,
        edge: &LayoutEdge,
        endpoint_limit: usize,
        community_pair_limit: usize,
        endpoint_counts: &BTreeMap<usize, usize>,
        community_pair_counts: &BTreeMap<String, usize>,
        relaxed: bool,
    ) -> bool {
        let pair = self.edge_update_community_pair(edge);
        let same = self.nodes[edge.source].community == self.nodes[edge.target].community;
        let pair_limit = if same {
            community_pair_limit
        } else if matches!(mode, NetworkPlanMode::XLarge) {
            if relaxed {
                XLARGE_CROSS_PAIR_LIMIT * 2
            } else {
                XLARGE_CROSS_PAIR_LIMIT
            }
        } else if matches!(mode, NetworkPlanMode::Large) {
            if relaxed {
                LARGE_CROSS_PAIR_LIMIT * 2
            } else {
                LARGE_CROSS_PAIR_LIMIT
            }
        } else {
            community_pair_limit
        };
        endpoint_counts.get(&edge.source).copied().unwrap_or(0) < endpoint_limit
            && endpoint_counts.get(&edge.target).copied().unwrap_or(0) < endpoint_limit
            && community_pair_counts.get(&pair).copied().unwrap_or(0) < pair_limit
    }

    fn remember_edge_update(
        &self,
        edge: &LayoutEdge,
        endpoint_counts: &mut BTreeMap<usize, usize>,
        community_pair_counts: &mut BTreeMap<String, usize>,
    ) {
        *endpoint_counts.entry(edge.source).or_insert(0) += 1;
        *endpoint_counts.entry(edge.target).or_insert(0) += 1;
        let pair = self.edge_update_community_pair(edge);
        *community_pair_counts.entry(pair).or_insert(0) += 1;
    }

    fn quality(
        &self,
        mode: NetworkPlanMode,
        communities: &[CommunitySummary],
        duration_ms: f64,
    ) -> Value {
        let quality_node_indexes = self.quality_node_indexes(mode);
        let quality_label_indexes = self.quality_label_indexes(mode);
        let quality_edge_indexes = self.quality_edge_indexes(mode, quality_node_indexes.as_deref());
        self.quality_for_scope(
            mode,
            "",
            quality_node_indexes.as_deref(),
            quality_label_indexes.as_deref(),
            quality_edge_indexes.as_deref(),
            communities,
            duration_ms,
            self.node_tier_stats(None),
        )
    }

    fn sample_quality(&self, mode: NetworkPlanMode, duration_ms: f64) -> Option<Value> {
        if !matches!(mode, NetworkPlanMode::Large | NetworkPlanMode::XLarge) {
            return None;
        }
        let quality_node_indexes = self.sample_quality_node_indexes(mode);
        if quality_node_indexes.is_empty() {
            return None;
        }
        let quality_label_indexes = quality_node_indexes
            .iter()
            .copied()
            .filter(|index| {
                self.nodes
                    .get(*index)
                    .map(|node| node.label_tier != "hidden")
                    .unwrap_or(false)
            })
            .collect::<Vec<_>>();
        let quality_edge_indexes = self.sample_quality_edge_indexes(mode, &quality_node_indexes);
        let communities = self.community_summaries_for_indexes(&quality_node_indexes);
        let mut tier_stats = self.node_tier_stats(Some(&quality_node_indexes));
        *tier_stats.entry("edge:visible".to_string()).or_insert(0) += quality_edge_indexes.len();
        let mut quality = self.quality_for_scope(
            mode,
            "all-sample",
            Some(&quality_node_indexes),
            Some(&quality_label_indexes),
            Some(&quality_edge_indexes),
            &communities,
            duration_ms,
            tier_stats,
        );
        if let Value::Object(ref mut map) = quality {
            map.insert("sampled".to_string(), Value::Bool(true));
            map.insert(
                "source".to_string(),
                Value::String("network-rust-sample".to_string()),
            );
            map.insert("sampleNodeTotal".to_string(), json!(self.nodes.len()));
            map.insert("sampleEdgeTotal".to_string(), json!(self.edges.len()));
        }
        Some(quality)
    }

    fn community_quality(
        &self,
        mode: NetworkPlanMode,
        duration_ms: f64,
        hints: &CommunityQualityHints,
    ) -> Vec<Value> {
        if !matches!(mode, NetworkPlanMode::Large | NetworkPlanMode::XLarge) {
            return Vec::new();
        }
        let groups = self.community_index_groups();
        if groups.is_empty() {
            return Vec::new();
        }
        let mut rows = groups
            .iter()
            .map(|(community, indexes)| {
                let visible_count = indexes.len();
                let bridge_score = indexes
                    .iter()
                    .filter_map(|index| self.nodes.get(*index))
                    .map(|node| node.bridge_score + node.hub_score * 0.35)
                    .fold(0.0_f64, f64::max);
                let intent_rank = hints.community_rank_for_indexes(community, &self.nodes, indexes);
                (
                    community.clone(),
                    indexes.len(),
                    visible_count,
                    bridge_score,
                    intent_rank,
                )
            })
            .collect::<Vec<_>>();
        rows.sort_by(|left, right| {
            right
                .4
                .cmp(&left.4)
                .then_with(|| right.2.cmp(&left.2))
                .then_with(|| right.1.cmp(&left.1))
                .then_with(|| right.3.partial_cmp(&left.3).unwrap_or(Ordering::Equal))
                .then_with(|| left.0.cmp(&right.0))
        });

        rows.into_iter()
            .take(COMMUNITY_QUALITY_LIMIT)
            .filter_map(|(community, node_total, visible_count, _, intent_rank)| {
                let indexes = groups.get(&community)?;
                let quality_node_indexes = self.community_quality_node_indexes(indexes);
                if quality_node_indexes.is_empty() {
                    return None;
                }
                let quality_label_indexes = quality_node_indexes.clone();
                let quality_edge_indexes =
                    self.community_quality_edge_indexes(&community, &quality_node_indexes);
                let edge_total = self
                    .edges
                    .iter()
                    .filter(|edge| {
                        self.nodes[edge.source].community == community
                            && self.nodes[edge.target].community == community
                    })
                    .count();
                let mut tier_stats = self.node_tier_stats(Some(&quality_node_indexes));
                self.add_edge_tier_stats(&quality_edge_indexes, &mut tier_stats);
                let summary =
                    self.community_summary_for_indexes(&community, &quality_node_indexes, false);
                let mut quality = self.quality_for_scope(
                    mode,
                    "community",
                    Some(&quality_node_indexes),
                    Some(&quality_label_indexes),
                    Some(&quality_edge_indexes),
                    &[summary],
                    duration_ms,
                    tier_stats,
                );
                if let Value::Object(ref mut map) = quality {
                    map.insert("sampled".to_string(), Value::Bool(true));
                    map.insert(
                        "source".to_string(),
                        Value::String("network-rust-community".to_string()),
                    );
                    map.insert("communityId".to_string(), Value::String(community));
                    map.insert("communityNodeTotal".to_string(), json!(node_total));
                    map.insert("communityEdgeTotal".to_string(), json!(edge_total));
                    map.insert("visibleNodeCount".to_string(), json!(visible_count));
                    map.insert("intentMatched".to_string(), json!(intent_rank > 0));
                    map.insert("intentRank".to_string(), json!(intent_rank));
                    map.insert(
                        "strongEdgeLengthAvg".to_string(),
                        json!(round2(self.average_edge_length_for_indexes(
                            &quality_edge_indexes,
                            |edge| edge.weight >= 2.05,
                        ))),
                    );
                    map.insert(
                        "weakEdgeLengthAvg".to_string(),
                        json!(round2(self.average_edge_length_for_indexes(
                            &quality_edge_indexes,
                            |edge| edge.weight <= 1.05,
                        ))),
                    );
                    map.insert(
                        "internalEdgeLengthAvg".to_string(),
                        json!(round2(self.average_edge_length_for_indexes(
                            &quality_edge_indexes,
                            |_| true,
                        ))),
                    );
                }
                Some(quality)
            })
            .collect()
    }

    fn quality_for_scope(
        &self,
        mode: NetworkPlanMode,
        quality_scope: &str,
        quality_node_indexes: Option<&[usize]>,
        quality_label_indexes: Option<&[usize]>,
        quality_edge_indexes: Option<&[usize]>,
        communities: &[CommunitySummary],
        duration_ms: f64,
        tier_stats: BTreeMap<String, usize>,
    ) -> Value {
        let node_overlap_count = self.node_overlap_count(quality_node_indexes);
        let label_overlap_estimate = self.label_overlap_estimate(quality_label_indexes);
        let edge_crossing_sample = self.edge_crossing_sample(quality_edge_indexes);
        let edge_length_std_dev = self.edge_length_std_dev(quality_edge_indexes);
        let cluster_bbox_overlap_count = cluster_bbox_overlap_count(communities);
        quality_payload(
            mode,
            node_overlap_count,
            label_overlap_estimate,
            edge_crossing_sample,
            cluster_bbox_overlap_count,
            edge_length_std_dev,
            communities.len(),
            duration_ms,
            quality_node_indexes
                .as_ref()
                .map(|rows| rows.len())
                .unwrap_or(self.nodes.len()),
            quality_label_indexes
                .as_ref()
                .map(|rows| rows.len())
                .unwrap_or(self.nodes.len()),
            quality_edge_indexes
                .map(|rows| rows.len())
                .unwrap_or(self.edges.len()),
            &tier_stats,
            quality_scope,
        )
    }

    fn node_tier_stats(&self, indexes: Option<&[usize]>) -> BTreeMap<String, usize> {
        let mut tier_stats: BTreeMap<String, usize> = BTreeMap::new();
        let count = indexes.map(|rows| rows.len()).unwrap_or(self.nodes.len());
        tier_stats.insert("node:visible".to_string(), count);
        tier_stats.insert("label:visible".to_string(), count);
        tier_stats
    }

    fn add_edge_tier_stats(&self, indexes: &[usize], tier_stats: &mut BTreeMap<String, usize>) {
        *tier_stats.entry("edge:visible".to_string()).or_insert(0) += indexes.len();
    }

    fn community_quality_node_indexes(&self, indexes: &[usize]) -> Vec<usize> {
        let target = COMMUNITY_QUALITY_NODE_LIMIT.min(indexes.len());
        if target == 0 {
            return Vec::new();
        }
        let mut ranked = indexes.to_vec();
        ranked.sort_by(|left, right| {
            self.label_priority(*right)
                .partial_cmp(&self.label_priority(*left))
                .unwrap_or(Ordering::Equal)
                .then_with(|| self.nodes[*left].id.cmp(&self.nodes[*right].id))
        });
        ranked.truncate(target);
        ranked.sort_unstable();
        ranked
    }

    fn community_quality_edge_indexes(
        &self,
        community: &str,
        quality_node_indexes: &[usize],
    ) -> Vec<usize> {
        let mut sampled_node = vec![false; self.nodes.len()];
        for index in quality_node_indexes {
            if *index < sampled_node.len() {
                sampled_node[*index] = true;
            }
        }
        let mut rows = self
            .edges
            .iter()
            .enumerate()
            .filter(|(_, edge)| {
                self.nodes[edge.source].community == community
                    && self.nodes[edge.target].community == community
                    && sampled_node.get(edge.source).copied().unwrap_or(false)
                    && sampled_node.get(edge.target).copied().unwrap_or(false)
            })
            .map(|(index, edge)| {
                let score = edge.weight * 10.0
                    + self.nodes[edge.source].bridge_score
                    + self.nodes[edge.target].bridge_score
                    + (self.nodes[edge.source].hub_score + self.nodes[edge.target].hub_score) * 0.5;
                (index, score)
            })
            .collect::<Vec<_>>();
        rows.sort_by(|left, right| {
            right
                .1
                .partial_cmp(&left.1)
                .unwrap_or(Ordering::Equal)
                .then_with(|| left.0.cmp(&right.0))
        });
        rows.into_iter()
            .take(COMMUNITY_QUALITY_EDGE_LIMIT)
            .map(|(index, _)| index)
            .collect()
    }

    fn average_edge_length_for_indexes<F>(&self, indexes: &[usize], predicate: F) -> f64
    where
        F: Fn(&LayoutEdge) -> bool,
    {
        let mut total = 0.0_f64;
        let mut count = 0usize;
        for index in indexes {
            let Some(edge) = self.edges.get(*index) else {
                continue;
            };
            if !predicate(edge) {
                continue;
            }
            let source = &self.nodes[edge.source];
            let target = &self.nodes[edge.target];
            total += (target.x - source.x).hypot(target.y - source.y);
            count += 1;
        }
        if count == 0 {
            0.0
        } else {
            total / count as f64
        }
    }

    fn sample_quality_node_indexes(&self, mode: NetworkPlanMode) -> Vec<usize> {
        let target = if matches!(mode, NetworkPlanMode::XLarge) {
            720usize
        } else {
            640usize
        }
        .min(self.nodes.len());
        let seed_cap = if matches!(mode, NetworkPlanMode::XLarge) {
            220usize
        } else {
            360usize
        }
        .min(target);
        let priorities = (0..self.nodes.len())
            .map(|index| self.label_priority(index))
            .collect::<Vec<_>>();
        let mut selected_flags = vec![false; self.nodes.len()];
        let mut selected = Vec::with_capacity(target);
        let remember = |index: usize,
                        selected_flags: &mut [bool],
                        selected: &mut Vec<usize>|
         -> bool {
            if index >= selected_flags.len() || selected_flags[index] || selected.len() >= target {
                return false;
            }
            selected_flags[index] = true;
            selected.push(index);
            true
        };
        let mut seed_rows = self
            .nodes
            .iter()
            .enumerate()
            .map(|(index, _)| index)
            .collect::<Vec<_>>();
        seed_rows.sort_by(|left, right| {
            priorities[*right]
                .partial_cmp(&priorities[*left])
                .unwrap_or(Ordering::Equal)
                .then_with(|| self.nodes[*left].id.cmp(&self.nodes[*right].id))
        });
        for index in seed_rows.into_iter().take(seed_cap) {
            remember(index, &mut selected_flags, &mut selected);
        }

        let mut best_by_community: BTreeMap<String, usize> = BTreeMap::new();
        for (index, node) in self.nodes.iter().enumerate() {
            best_by_community
                .entry(node.community.clone())
                .and_modify(|current| {
                    if priorities[index] > priorities[*current]
                        || (priorities[index] == priorities[*current]
                            && self.nodes[index].id < self.nodes[*current].id)
                    {
                        *current = index;
                    }
                })
                .or_insert(index);
        }
        let mut community_reps = best_by_community.values().copied().collect::<Vec<_>>();
        community_reps.sort_by(|left, right| {
            priorities[*right]
                .partial_cmp(&priorities[*left])
                .unwrap_or(Ordering::Equal)
                .then_with(|| {
                    self.nodes[*left]
                        .community
                        .cmp(&self.nodes[*right].community)
                })
                .then_with(|| self.nodes[*left].id.cmp(&self.nodes[*right].id))
        });
        for index in community_reps {
            if selected.len() >= target {
                break;
            }
            remember(index, &mut selected_flags, &mut selected);
        }

        if selected.len() < target {
            let len = self.nodes.len().max(1);
            let mut stride = (len / target.max(1)).max(1) * 2 + 1;
            if stride >= len {
                stride = 1;
            }
            let mut cursor =
                (stable_hash_unit("network-sample-node-fill") * len as f64).floor() as usize % len;
            let max_attempts = len.saturating_mul(2).max(target);
            for _ in 0..max_attempts {
                if selected.len() >= target {
                    break;
                }
                remember(cursor, &mut selected_flags, &mut selected);
                cursor = (cursor + stride) % len;
            }
            if selected.len() < target {
                for index in 0..self.nodes.len() {
                    if selected.len() >= target {
                        break;
                    }
                    remember(index, &mut selected_flags, &mut selected);
                }
            }
        }
        selected.sort_unstable();
        selected
    }

    fn sample_quality_edge_indexes(
        &self,
        mode: NetworkPlanMode,
        quality_node_indexes: &[usize],
    ) -> Vec<usize> {
        let target = if matches!(mode, NetworkPlanMode::XLarge) {
            900usize
        } else {
            760usize
        }
        .min(self.edges.len());
        if target == 0 {
            return Vec::new();
        }
        let mut sampled_node = vec![false; self.nodes.len()];
        for index in quality_node_indexes {
            if *index < sampled_node.len() {
                sampled_node[*index] = true;
            }
        }
        let edge_score = |edge: &LayoutEdge| {
            let source = &self.nodes[edge.source];
            let target = &self.nodes[edge.target];
            let bridge = source.bridge_score + target.bridge_score;
            let hub = source.hub_score + target.hub_score;
            let cross = if source.community != target.community {
                2.2
            } else {
                1.0
            };
            edge.weight * cross + bridge * 2.4 + hub * 1.2
        };
        let mut selected: BTreeSet<usize> = BTreeSet::new();
        let mut scoped = self
            .edges
            .iter()
            .enumerate()
            .filter(|(_, edge)| {
                sampled_node.get(edge.source).copied().unwrap_or(false)
                    && sampled_node.get(edge.target).copied().unwrap_or(false)
            })
            .map(|(index, _)| index)
            .collect::<Vec<_>>();
        scoped.sort_by(|left, right| {
            edge_score(&self.edges[*right])
                .partial_cmp(&edge_score(&self.edges[*left]))
                .unwrap_or(Ordering::Equal)
                .then_with(|| left.cmp(right))
        });
        for index in scoped.into_iter().take(target) {
            selected.insert(index);
        }
        let min_fill = (target / 3).max(1);
        if selected.len() < min_fill {
            let mut global = (0..self.edges.len()).collect::<Vec<_>>();
            global.sort_by(|left, right| {
                edge_score(&self.edges[*right])
                    .partial_cmp(&edge_score(&self.edges[*left]))
                    .unwrap_or(Ordering::Equal)
                    .then_with(|| left.cmp(right))
            });
            for index in global {
                if selected.len() >= target {
                    break;
                }
                selected.insert(index);
            }
        }
        selected.into_iter().collect()
    }

    fn quality_node_indexes(&self, mode: NetworkPlanMode) -> Option<Vec<usize>> {
        if !matches!(mode, NetworkPlanMode::Large | NetworkPlanMode::XLarge) {
            return None;
        }
        Some((0..self.nodes.len()).collect())
    }

    fn quality_label_indexes(&self, mode: NetworkPlanMode) -> Option<Vec<usize>> {
        if !matches!(mode, NetworkPlanMode::Large | NetworkPlanMode::XLarge) {
            return None;
        }
        Some((0..self.nodes.len()).collect())
    }

    fn quality_edge_indexes(
        &self,
        mode: NetworkPlanMode,
        quality_node_indexes: Option<&[usize]>,
    ) -> Option<Vec<usize>> {
        if !matches!(mode, NetworkPlanMode::Large | NetworkPlanMode::XLarge) {
            return None;
        }
        let _ = quality_node_indexes;
        Some((0..self.edges.len()).collect())
    }

    fn node_overlap_count(&self, indexes: Option<&[usize]>) -> usize {
        let index_rows = indexes
            .map(|rows| rows.to_vec())
            .unwrap_or_else(|| (0..self.nodes.len()).collect());
        let n = index_rows.len();
        let target = if n > 5000 {
            120
        } else if n > 1200 {
            300
        } else {
            720
        };
        let step = if n <= target {
            1
        } else {
            (n as f64 / target as f64).ceil() as usize
        };
        let mut count = 0usize;
        for left_pos in 0..n {
            let i = index_rows[left_pos];
            for right_pos in (left_pos + 1)..n {
                if step > 1 && ((left_pos * 19 + right_pos * 29) % step) != 0 {
                    continue;
                }
                let j = index_rows[right_pos];
                let dx = self.nodes[j].x - self.nodes[i].x;
                let dy = self.nodes[j].y - self.nodes[i].y;
                let min_dist = self.nodes[i].radius + self.nodes[j].radius + 8.0;
                if dx * dx + dy * dy < min_dist * min_dist {
                    count += step;
                }
            }
        }
        count
    }

    fn label_overlap_estimate(&self, indexes: Option<&[usize]>) -> usize {
        let index_rows = indexes
            .map(|rows| rows.to_vec())
            .unwrap_or_else(|| (0..self.nodes.len()).collect());
        let n = index_rows.len();
        let target = if n > 5000 {
            80
        } else if n > 1200 {
            180
        } else {
            520
        };
        let step = if n <= target {
            1
        } else {
            (n as f64 / target as f64).ceil() as usize
        };
        let mut count = 0usize;
        for left_pos in 0..n {
            let i = index_rows[left_pos];
            for right_pos in (left_pos + 1)..n {
                if step > 1 && ((left_pos * 11 + right_pos * 37) % step) != 0 {
                    continue;
                }
                let j = index_rows[right_pos];
                let a = label_box(&self.nodes[i]);
                let b = label_box(&self.nodes[j]);
                if boxes_overlap(a, b) {
                    count += step;
                }
            }
        }
        count
    }

    fn edge_crossing_sample(&self, indexes: Option<&[usize]>) -> usize {
        let index_rows = indexes
            .map(|rows| rows.to_vec())
            .unwrap_or_else(|| (0..self.edges.len()).collect());
        let m = index_rows.len();
        if m < 2 {
            return 0;
        }
        let max_edges = if self.nodes.len() > 5000 {
            120usize
        } else if self.nodes.len() > 1200 {
            180usize
        } else {
            420usize
        };
        let step = if m <= max_edges {
            1
        } else {
            (m as f64 / max_edges as f64).ceil() as usize
        };
        let sampled = index_rows
            .iter()
            .enumerate()
            .filter(|(index, _)| index % step == 0)
            .filter_map(|(_, edge_index)| self.edges.get(*edge_index))
            .collect::<Vec<_>>();
        let mut count = 0usize;
        for i in 0..sampled.len() {
            for j in (i + 1)..sampled.len() {
                let a = sampled[i];
                let b = sampled[j];
                if a.source == b.source
                    || a.source == b.target
                    || a.target == b.source
                    || a.target == b.target
                {
                    continue;
                }
                if segments_cross(
                    (self.nodes[a.source].x, self.nodes[a.source].y),
                    (self.nodes[a.target].x, self.nodes[a.target].y),
                    (self.nodes[b.source].x, self.nodes[b.source].y),
                    (self.nodes[b.target].x, self.nodes[b.target].y),
                ) {
                    count += step;
                }
            }
        }
        count
    }

    fn edge_length_std_dev(&self, indexes: Option<&[usize]>) -> f64 {
        let index_rows = indexes
            .map(|rows| rows.to_vec())
            .unwrap_or_else(|| (0..self.edges.len()).collect());
        if index_rows.is_empty() {
            return 0.0;
        }
        let lengths = index_rows
            .iter()
            .filter_map(|edge_index| self.edges.get(*edge_index))
            .map(|edge| {
                let a = &self.nodes[edge.source];
                let b = &self.nodes[edge.target];
                (b.x - a.x).hypot(b.y - a.y)
            })
            .collect::<Vec<_>>();
        if lengths.is_empty() {
            return 0.0;
        }
        let mean = lengths.iter().sum::<f64>() / lengths.len() as f64;
        let variance = lengths
            .iter()
            .map(|value| {
                let delta = *value - mean;
                delta * delta
            })
            .sum::<f64>()
            / lengths.len() as f64;
        variance.sqrt()
    }
}

fn quality_payload(
    mode: NetworkPlanMode,
    node_overlap_count: usize,
    label_overlap_estimate: usize,
    edge_crossing_sample: usize,
    cluster_bbox_overlap_count: usize,
    edge_length_std_dev: f64,
    community_count: usize,
    duration_ms: f64,
    quality_node_count: usize,
    quality_label_count: usize,
    quality_edge_count: usize,
    tier_stats: &BTreeMap<String, usize>,
    quality_scope: &str,
) -> Value {
    let resolved_quality_scope = if !quality_scope.trim().is_empty() {
        quality_scope.trim()
    } else {
        "all"
    };
    json!({
        "durationMs": round2(duration_ms),
        "nodeOverlapCount": node_overlap_count,
        "labelOverlapEstimate": label_overlap_estimate,
        "edgeCrossingSample": edge_crossing_sample,
        "edgeLengthStdDev": round2(edge_length_std_dev),
        "clusterBBoxOverlapCount": cluster_bbox_overlap_count,
        "mode": mode.as_str(),
        "communityCount": community_count,
        "qualityScope": resolved_quality_scope,
        "qualityNodeCount": quality_node_count,
        "qualityLabelCount": quality_label_count,
        "qualityEdgeCount": quality_edge_count,
        "visibleTierStats": tier_stats,
    })
}

fn community_to_json(row: &CommunitySummary) -> Value {
    json!({
        "id": row.id.as_str(),
        "nodeCount": row.node_count,
        "nodeIds": row.node_ids.clone(),
        "edgeWeight": round2(row.edge_weight),
        "x": round2(row.x),
        "y": round2(row.y),
        "radius": round2(row.radius),
        "bbox": {
            "minX": round2(row.bbox.0),
            "minY": round2(row.bbox.1),
            "maxX": round2(row.bbox.2),
            "maxY": round2(row.bbox.3),
        },
    })
}

fn cluster_bbox_overlap_count(communities: &[CommunitySummary]) -> usize {
    let mut count = 0usize;
    for i in 0..communities.len() {
        for j in (i + 1)..communities.len() {
            if boxes_overlap(communities[i].bbox, communities[j].bbox) {
                count += 1;
            }
        }
    }
    count
}

fn seed_position(value: &Value, id: &str, index: usize, len: usize) -> (f64, f64) {
    let x = finite_field(value, "x")
        .or_else(|| finite_field(value, "prevX"))
        .unwrap_or(f64::NAN);
    let y = finite_field(value, "y")
        .or_else(|| finite_field(value, "prevY"))
        .unwrap_or(f64::NAN);
    if x.is_finite() && y.is_finite() {
        let jitter = stable_hash_unit(&format!("seed-jitter|{id}")) - 0.5;
        return (x + jitter * 0.4, y - jitter * 0.4);
    }
    let count = len.max(1) as f64;
    let angle = TWO_PI * (((index as f64) + stable_hash_unit(id) * 0.37) / count);
    let radius = count.sqrt() * 72.0 + 80.0;
    (angle.cos() * radius, angle.sin() * radius)
}

fn disperse_duplicate_positions(nodes: &mut [LayoutNode]) {
    let mut groups: BTreeMap<String, Vec<usize>> = BTreeMap::new();
    for (index, node) in nodes.iter().enumerate() {
        let key = format!("{:.1}|{:.1}", node.x, node.y);
        groups.entry(key).or_default().push(index);
    }
    for indexes in groups.values() {
        if indexes.len() <= 1 {
            continue;
        }
        let radius = (indexes.len() as f64).sqrt() * 24.0;
        for (local_index, index) in indexes.iter().enumerate() {
            let angle = TWO_PI
                * (((local_index as f64) + stable_hash_unit(&nodes[*index].id) * 0.31)
                    / indexes.len() as f64);
            let dx = angle.cos() * radius;
            let dy = angle.sin() * radius;
            nodes[*index].x += dx;
            nodes[*index].y += dy;
            nodes[*index].seed_x += dx;
            nodes[*index].seed_y += dy;
        }
    }
}

fn normalize_role(role: &str) -> String {
    let raw = role.trim().to_lowercase();
    if raw == "core" || raw == "adjacent" || raw == "leaf" {
        raw
    } else {
        "leaf".to_string()
    }
}

fn normalize_layout_tier(value: Option<&str>, fallback: &str) -> String {
    match value.unwrap_or("").trim().to_lowercase().as_str() {
        "primary" => "primary".to_string(),
        "secondary" | "visible" => "secondary".to_string(),
        "hidden" => "hidden".to_string(),
        _ => fallback.to_string(),
    }
}

fn semantic_group_key(node: &LayoutNode) -> &str {
    if !node.layout_community_id.is_empty() {
        &node.layout_community_id
    } else if !node.cluster_id.is_empty() {
        &node.cluster_id
    } else {
        &node.community
    }
}

fn role_rank(role: &str) -> usize {
    match role {
        "core" => 3,
        "adjacent" => 2,
        _ => 1,
    }
}

fn edge_endpoint(value: Option<&Value>) -> String {
    match value {
        Some(Value::String(text)) => text.trim().to_string(),
        Some(Value::Number(number)) => number.to_string(),
        Some(Value::Object(obj)) => obj
            .get("id")
            .map(js_or_empty_string)
            .unwrap_or_default()
            .trim()
            .to_string(),
        _ => String::new(),
    }
}

fn edge_weight(value: &Value) -> f64 {
    first_finite_field(
        value,
        &[
            "amount",
            "weight",
            "total_amount",
            "totalAmount",
            "forward_amount",
            "reverse_amount",
            "out_amount",
            "in_amount",
            "count",
        ],
    )
    .unwrap_or(1.0)
    .abs()
    .max(1.0)
}

fn estimate_label_half_width(value: &Value) -> f64 {
    let text = first_text_field(
        value,
        &[
            "name",
            "title",
            "label",
            "display_id",
            "displayId",
            "displayIdRaw",
            "id",
        ],
    )
    .unwrap_or_default();
    let units = text
        .chars()
        .map(|ch| {
            if ch as u32 > 255 {
                1.02
            } else if ch.is_ascii_uppercase() {
                0.72
            } else {
                0.62
            }
        })
        .sum::<f64>();
    let font_size = finite_field(value, "fontSize")
        .unwrap_or(13.0)
        .clamp(8.0, 36.0);
    (units * font_size * 0.5).max(22.0)
}

fn label_box(node: &LayoutNode) -> (f64, f64, f64, f64) {
    (
        node.x - node.label_half_width,
        node.y + node.radius + 6.0,
        node.x + node.label_half_width,
        node.y + node.radius + 6.0 + node.label_height,
    )
}

fn expand_box(box_: (f64, f64, f64, f64), padding: f64) -> (f64, f64, f64, f64) {
    (
        box_.0 - padding,
        box_.1 - padding,
        box_.2 + padding,
        box_.3 + padding,
    )
}

fn boxes_overlap(a: (f64, f64, f64, f64), b: (f64, f64, f64, f64)) -> bool {
    a.0 <= b.2 && a.2 >= b.0 && a.1 <= b.3 && a.3 >= b.1
}

fn segments_cross(a: (f64, f64), b: (f64, f64), c: (f64, f64), d: (f64, f64)) -> bool {
    fn orient(p: (f64, f64), q: (f64, f64), r: (f64, f64)) -> f64 {
        (q.0 - p.0) * (r.1 - p.1) - (q.1 - p.1) * (r.0 - p.0)
    }
    let o1 = orient(a, b, c);
    let o2 = orient(a, b, d);
    let o3 = orient(c, d, a);
    let o4 = orient(c, d, b);
    (o1 > 0.0 && o2 < 0.0 || o1 < 0.0 && o2 > 0.0) && (o3 > 0.0 && o4 < 0.0 || o3 < 0.0 && o4 > 0.0)
}

fn text_field(value: &Value, key: &str) -> String {
    value
        .get(key)
        .map(js_or_empty_string)
        .unwrap_or_default()
        .trim()
        .to_string()
}

fn first_text_field(value: &Value, keys: &[&str]) -> Option<String> {
    keys.iter()
        .filter_map(|key| {
            let text = text_field(value, key);
            if text.is_empty() {
                None
            } else {
                Some(text)
            }
        })
        .next()
}

fn first_finite_field(value: &Value, keys: &[&str]) -> Option<f64> {
    keys.iter()
        .filter_map(|key| finite_field(value, key))
        .next()
}

fn extend_text_set(out: &mut BTreeSet<String>, value: Option<&Value>) {
    match value {
        Some(Value::String(text)) => {
            let trimmed = text.trim();
            if !trimmed.is_empty() {
                out.insert(trimmed.to_string());
            }
        }
        Some(Value::Array(rows)) => {
            for row in rows {
                let text = js_or_empty_string(row);
                let trimmed = text.trim();
                if !trimmed.is_empty() {
                    out.insert(trimmed.to_string());
                }
            }
        }
        Some(Value::Number(_) | Value::Bool(_)) => {
            let text = value.map(js_or_empty_string).unwrap_or_default();
            let trimmed = text.trim();
            if !trimmed.is_empty() {
                out.insert(trimmed.to_string());
            }
        }
        _ => {}
    }
}

fn finite_field(value: &Value, key: &str) -> Option<f64> {
    value
        .get(key)
        .and_then(js_number)
        .filter(|value| value.is_finite())
}

fn js_number(value: &Value) -> Option<f64> {
    match value {
        Value::Number(number) => number.as_f64(),
        Value::String(text) => text.trim().parse::<f64>().ok(),
        _ => None,
    }
}

fn js_or_empty_string(value: &Value) -> String {
    match value {
        Value::String(text) => text.to_string(),
        Value::Number(number) => number.to_string(),
        Value::Bool(value) => value.to_string(),
        _ => String::new(),
    }
}

fn stable_hash_unit(value: &str) -> f64 {
    let mut h: u32 = 2_166_136_261;
    for byte in value.as_bytes() {
        h ^= *byte as u32;
        h = h.wrapping_mul(16_777_619);
    }
    (h as f64) / (u32::MAX as f64)
}

fn round2(value: f64) -> f64 {
    (value * 100.0).round() / 100.0
}

fn round4(value: f64) -> f64 {
    (value * 10000.0).round() / 10000.0
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn distance(plan: &Value, a: &str, b: &str) -> f64 {
        let rows = plan["nodeUpdates"].as_array().unwrap();
        let get = |id: &str| {
            let row = rows.iter().find(|row| row["id"] == id).unwrap();
            (row["x"].as_f64().unwrap(), row["y"].as_f64().unwrap())
        };
        let pa = get(a);
        let pb = get(b);
        (pb.0 - pa.0).hypot(pb.1 - pa.1)
    }

    fn node_position_map(plan: &Value) -> BTreeMap<String, (f64, f64)> {
        plan["nodeUpdates"]
            .as_array()
            .unwrap()
            .iter()
            .map(|row| {
                (
                    row["id"].as_str().unwrap().to_string(),
                    (row["x"].as_f64().unwrap(), row["y"].as_f64().unwrap()),
                )
            })
            .collect()
    }

    fn assert_position_only_network_plan(plan: &Value, node_count: usize) {
        let node_updates = plan["nodeUpdates"].as_array().unwrap();
        assert_eq!(node_updates.len(), node_count);
        assert!(plan["edgeUpdates"].as_array().unwrap().is_empty());
        for row in node_updates {
            let keys = row.as_object().unwrap();
            assert!(keys.contains_key("id"));
            assert!(keys.contains_key("x"));
            assert!(keys.contains_key("y"));
            assert!(keys.contains_key("index"));
            assert!(!keys.contains_key("layoutVisibilityTier"));
            assert!(!keys.contains_key("layoutLabelTier"));
            assert!(!keys.contains_key("layoutEdgeTier"));
            assert!(!keys.contains_key("layoutHiddenBySkeleton"));
            assert!(!keys.contains_key("nodeLabelHidden"));
            assert!(!keys.contains_key("edgeLabelHidden"));
            assert!(!keys.contains_key("layoutEdgeLabelTier"));
        }
    }

    fn graph_edge_lengths(plan: &Value, edges: &[Value]) -> Vec<f64> {
        let positions = node_position_map(plan);
        let mut lengths = edges
            .iter()
            .filter_map(|edge| {
                let source = edge_endpoint(edge.get("source"));
                let target = edge_endpoint(edge.get("target"));
                let source_position = positions.get(&source)?;
                let target_position = positions.get(&target)?;
                Some(
                    (target_position.0 - source_position.0)
                        .hypot(target_position.1 - source_position.1),
                )
            })
            .collect::<Vec<_>>();
        lengths.sort_by(|left, right| left.partial_cmp(right).unwrap_or(Ordering::Equal));
        lengths
    }

    fn node_grid_occupancy(plan: &Value, columns: usize, rows: usize) -> (usize, f64) {
        let positions = node_position_map(plan);
        let min_x = positions
            .values()
            .map(|(x, _)| *x)
            .fold(f64::INFINITY, f64::min);
        let max_x = positions
            .values()
            .map(|(x, _)| *x)
            .fold(f64::NEG_INFINITY, f64::max);
        let min_y = positions
            .values()
            .map(|(_, y)| *y)
            .fold(f64::INFINITY, f64::min);
        let max_y = positions
            .values()
            .map(|(_, y)| *y)
            .fold(f64::NEG_INFINITY, f64::max);
        let mut cells = vec![0usize; columns * rows];
        for (x, y) in positions.values() {
            let column = (((*x - min_x) / (max_x - min_x).max(1.0)) * columns as f64)
                .floor()
                .clamp(0.0, columns.saturating_sub(1) as f64) as usize;
            let row = (((*y - min_y) / (max_y - min_y).max(1.0)) * rows as f64)
                .floor()
                .clamp(0.0, rows.saturating_sub(1) as f64) as usize;
            cells[row * columns + column] += 1;
        }
        let max_cell = cells.iter().copied().max().unwrap_or(0);
        (
            cells.iter().filter(|count| **count > 0).count(),
            max_cell as f64 / positions.len().max(1) as f64,
        )
    }

    #[test]
    fn network_plan_is_deterministic() {
        let payload = json!({
            "nodes": [
                {"id": "a", "x": -80, "y": 0, "role": "core"},
                {"id": "b", "x": 80, "y": 0, "role": "leaf"},
                {"id": "c", "x": 0, "y": 140, "role": "leaf"}
            ],
            "edges": [
                {"source": "a", "target": "b", "amount": 100},
                {"source": "b", "target": "c", "amount": 20}
            ]
        });
        let left = project_flow_layout_network_plan(&payload);
        let right = project_flow_layout_network_plan(&payload);
        assert_eq!(left["nodeUpdates"], right["nodeUpdates"]);
        assert_eq!(left["mode"], "small");
    }

    #[test]
    fn strong_edges_resolve_shorter_than_weak_edges() {
        let payload = json!({
            "nodes": [
                {"id": "a", "x": -180, "y": 0, "role": "core"},
                {"id": "b", "x": 0, "y": 0, "role": "leaf"},
                {"id": "c", "x": 220, "y": 0, "role": "leaf"}
            ],
            "edges": [
                {"source": "a", "target": "b", "amount": 10000},
                {"source": "b", "target": "c", "amount": 10}
            ]
        });
        let plan = project_flow_layout_network_plan(&payload);
        assert!(distance(&plan, "a", "b") < distance(&plan, "b", "c"));
    }

    #[test]
    fn community_edges_are_tighter_than_cross_edges() {
        let payload = json!({
            "nodes": [
                {"id": "a1", "x": -260, "y": -80, "clusterId": "left"},
                {"id": "a2", "x": -180, "y": 80, "clusterId": "left"},
                {"id": "b1", "x": 180, "y": -80, "clusterId": "right"},
                {"id": "b2", "x": 260, "y": 80, "clusterId": "right"}
            ],
            "edges": [
                {"source": "a1", "target": "a2", "amount": 500},
                {"source": "b1", "target": "b2", "amount": 500},
                {"source": "a2", "target": "b1", "amount": 20}
            ]
        });
        let plan = project_flow_layout_network_plan(&payload);
        let inner = (distance(&plan, "a1", "a2") + distance(&plan, "b1", "b2")) / 2.0;
        let cross = distance(&plan, "a2", "b1");
        assert!(inner < cross);
    }

    #[test]
    fn collision_pass_reduces_node_overlap() {
        let nodes = (0..24)
            .map(|index| json!({"id": format!("n{index}"), "x": 0, "y": 0, "r": 18}))
            .collect::<Vec<_>>();
        let edges = (1..24)
            .map(|index| json!({"source": "n0", "target": format!("n{index}"), "amount": 100}))
            .collect::<Vec<_>>();
        let plan = project_flow_layout_network_plan(&json!({ "nodes": nodes, "edges": edges }));
        assert!(plan["quality"]["nodeOverlapCount"].as_u64().unwrap() < 6);
    }

    #[test]
    fn small_dense_network_plan_keeps_all_nodes_and_labels_in_contract() {
        let nodes = (0..80usize)
            .map(|index| {
                json!({
                    "id": format!("n{index}"),
                    "x": (index % 8) as f64 * 12.0,
                    "y": (index / 8) as f64 * 12.0,
                    "clusterId": "dense",
                    "role": if index == 0 { "core" } else { "leaf" },
                    "label": format!("重叠标签{index}"),
                    "labelHalfWidth": 72,
                    "labelBottom": 18,
                    "r": 18,
                })
            })
            .collect::<Vec<_>>();
        let edges = (1..80usize)
            .map(|index| json!({"source": "n0", "target": format!("n{index}"), "amount": 120}))
            .collect::<Vec<_>>();

        let plan = project_flow_layout_network_plan(&json!({ "nodes": nodes, "edges": edges }));
        let quality = &plan["quality"];
        let tier_stats = quality["visibleTierStats"].as_object().unwrap();

        assert_eq!(plan["mode"], "small");
        assert_position_only_network_plan(&plan, 80);
        assert_eq!(quality["qualityNodeCount"].as_u64().unwrap(), 80);
        assert_eq!(quality["qualityLabelCount"].as_u64().unwrap(), 80);
        assert_eq!(
            tier_stats
                .get("node:visible")
                .and_then(Value::as_u64)
                .unwrap_or(0),
            80
        );
        assert_eq!(
            tier_stats
                .get("label:visible")
                .and_then(Value::as_u64)
                .unwrap_or(0),
            80
        );
        assert_eq!(
            plan["report"]["labelCollisionSuppressedCount"]
                .as_u64()
                .unwrap(),
            0
        );
    }

    #[test]
    fn medium_sparse_network_keeps_topology_and_labels_readable() {
        let nodes = (0..320)
            .map(|index| {
                let angle = TWO_PI * index as f64 / 320.0;
                let radius = 900.0 + (index % 4) as f64 * 260.0;
                json!({
                    "id": format!("n{index}"),
                    "x": angle.cos() * radius,
                    "y": angle.sin() * radius * 0.72,
                    "clusterId": format!("semantic-{}", index % 16),
                    "role": if index % 40 == 0 { "core" } else { "leaf" },
                    "r": 18,
                })
            })
            .collect::<Vec<_>>();
        let mut edges = Vec::new();
        for component in 0..12 {
            let start = component * 16;
            for offset in 0..9 {
                edges.push(json!({
                    "source": format!("n{}", start + offset),
                    "target": format!("n{}", start + offset + 1),
                    "amount": if offset == 0 { 5000 } else { 120 },
                }));
            }
            edges.push(json!({
                "source": format!("n{start}"),
                "target": format!("n{}", start + 8),
                "amount": 900,
            }));
        }
        for component in 0..12 {
            edges.push(json!({
                "source": format!("n{}", component * 16 + 4),
                "target": format!("n{}", ((component + 1) % 12) * 16 + 4),
                "amount": 80,
            }));
        }

        let plan = project_flow_layout_network_plan(&json!({ "nodes": nodes, "edges": edges }));
        let edge_rows = plan["report"]["edgeCount"].as_u64().unwrap();
        assert_eq!(plan["mode"], "medium");
        assert_position_only_network_plan(&plan, 320);
        assert_eq!(edge_rows, 132);

        let payload_edges = (0..12)
            .flat_map(|component| {
                let start = component * 16;
                let mut rows = (0..9)
                    .map(|offset| {
                        json!({
                            "source": format!("n{}", start + offset),
                            "target": format!("n{}", start + offset + 1),
                        })
                    })
                    .collect::<Vec<_>>();
                rows.push(json!({
                    "source": format!("n{start}"),
                    "target": format!("n{}", start + 8),
                }));
                rows
            })
            .chain((0..12).map(|component| {
                json!({
                    "source": format!("n{}", component * 16 + 4),
                    "target": format!("n{}", ((component + 1) % 12) * 16 + 4),
                })
            }))
            .collect::<Vec<_>>();
        let lengths = graph_edge_lengths(&plan, &payload_edges);
        let average = lengths.iter().sum::<f64>() / lengths.len().max(1) as f64;
        let p90 = lengths[(lengths.len() as f64 * 0.90).floor() as usize];
        let max_length = *lengths.last().unwrap();
        let (occupied_cells, max_cell_ratio) = node_grid_occupancy(&plan, 12, 8);
        let tier_stats = plan["quality"]["visibleTierStats"].as_object().unwrap();
        let positions = node_position_map(&plan);
        let mut degree = vec![0usize; 320];
        for edge in &payload_edges {
            let source = edge_endpoint(edge.get("source"));
            let target = edge_endpoint(edge.get("target"));
            let source_index = source.trim_start_matches('n').parse::<usize>().unwrap();
            let target_index = target.trim_start_matches('n').parse::<usize>().unwrap();
            degree[source_index] += 1;
            degree[target_index] += 1;
        }
        let mut semantic_connected_centroids: BTreeMap<usize, (f64, f64, usize)> = BTreeMap::new();
        for index in 0..320usize {
            if degree[index] == 0 {
                continue;
            }
            let (x, y) = positions[&format!("n{index}")];
            semantic_connected_centroids
                .entry(index % 16)
                .and_modify(|(sum_x, sum_y, count)| {
                    *sum_x += x;
                    *sum_y += y;
                    *count += 1;
                })
                .or_insert((x, y, 1));
        }
        let mut isolate_semantic_distance_total = 0.0_f64;
        let mut isolate_semantic_distance_count = 0usize;
        let mut far_isolates = 0usize;
        for index in 0..320usize {
            if degree[index] > 0 {
                continue;
            }
            let Some((sum_x, sum_y, count)) =
                semantic_connected_centroids.get(&(index % 16)).copied()
            else {
                continue;
            };
            let (x, y) = positions[&format!("n{index}")];
            let distance = (x - sum_x / count as f64).hypot(y - sum_y / count as f64);
            isolate_semantic_distance_total += distance;
            isolate_semantic_distance_count += 1;
            if distance > 680.0 {
                far_isolates += 1;
            }
        }
        let average_isolate_semantic_distance =
            isolate_semantic_distance_total / isolate_semantic_distance_count.max(1) as f64;

        assert!(average < 320.0, "average edge length was {average}");
        assert!(p90 < 430.0, "p90 edge length was {p90}");
        assert!(max_length < 620.0, "max edge length was {max_length}");
        assert!(occupied_cells >= 48, "occupied cells were {occupied_cells}");
        assert!(
            max_cell_ratio <= 0.09,
            "max cell ratio was {max_cell_ratio}"
        );
        assert!(
            average_isolate_semantic_distance < 430.0,
            "average isolate semantic distance was {average_isolate_semantic_distance}"
        );
        assert!(
            far_isolates <= 8,
            "far semantic isolates were {far_isolates}"
        );
        assert_eq!(plan["quality"]["nodeOverlapCount"].as_u64().unwrap(), 0);
        assert_eq!(
            tier_stats
                .get("node:visible")
                .and_then(Value::as_u64)
                .unwrap_or(0),
            320
        );
        assert_eq!(
            tier_stats
                .get("label:visible")
                .and_then(Value::as_u64)
                .unwrap_or(0),
            320
        );
        assert_eq!(plan["lod"]["visibleNodeCount"].as_u64().unwrap_or(0), 320);
        assert_eq!(
            plan["lod"]["visibleEdgeCount"].as_u64().unwrap_or(0),
            plan["report"]["edgeCount"].as_u64().unwrap_or(0)
        );
        assert_eq!(
            plan["report"]["labelCollisionSuppressedCount"]
                .as_u64()
                .unwrap(),
            0
        );
    }

    #[test]
    fn large_plan_reports_all_scope_and_position_only_updates() {
        let node_count = 1301usize;
        let community_size = 50usize;
        let nodes = (0..node_count)
            .map(|index| {
                json!({
                    "id": format!("n{index}"),
                    "x": (index % 40) as f64 * 24.0,
                    "y": (index / 40) as f64 * 24.0,
                    "clusterId": format!("c{}", index / community_size),
                    "role": if index % community_size == 0 {
                        "core"
                    } else if index % community_size < 6 {
                        "adjacent"
                    } else {
                        "leaf"
                    },
                    "r": 18,
                })
            })
            .collect::<Vec<_>>();
        let mut edges = Vec::new();
        for index in 0..node_count {
            let community_end = ((index / community_size) + 1) * community_size;
            if index + 1 < node_count && index + 1 < community_end {
                edges.push(json!({
                    "source": format!("n{index}"),
                    "target": format!("n{}", index + 1),
                    "amount": 3000,
                    "weight": 3,
                }));
            }
            if index + 5 < node_count && index + 5 < community_end {
                edges.push(json!({
                    "source": format!("n{index}"),
                    "target": format!("n{}", index + 5),
                    "amount": 400,
                    "weight": 1.4,
                }));
            }
        }
        for start in (0..node_count).step_by(community_size) {
            if start + community_size >= node_count {
                break;
            }
            edges.push(json!({
                "source": format!("n{start}"),
                "target": format!("n{}", start + community_size),
                "amount": 10,
                "weight": 0.8,
            }));
        }

        let plan = project_flow_layout_network_plan(&json!({ "nodes": nodes, "edges": edges }));
        let quality = &plan["quality"];
        let sample_quality = &plan["sampleQuality"];
        let community_quality = plan["communityQuality"].as_array().unwrap();
        assert_eq!(plan["mode"], "large");
        assert_position_only_network_plan(&plan, node_count);
        assert_eq!(quality["qualityScope"], "all");
        assert_eq!(
            quality["qualityNodeCount"].as_u64().unwrap(),
            node_count as u64
        );
        assert_eq!(sample_quality["qualityScope"], "all-sample");
        assert!(!community_quality.is_empty());
        assert!(community_quality.len() <= COMMUNITY_QUALITY_LIMIT);
        assert!(community_quality
            .iter()
            .all(|row| row["qualityScope"] == "community"));
        assert!(community_quality
            .iter()
            .all(|row| row["qualityNodeCount"].as_u64().unwrap_or(0) > 0));
        assert_eq!(
            sample_quality["sampleNodeTotal"].as_u64().unwrap(),
            node_count as u64
        );
        assert!(sample_quality["qualityNodeCount"].as_u64().unwrap() <= node_count as u64);
        assert_eq!(
            quality["qualityLabelCount"].as_u64().unwrap(),
            node_count as u64
        );
        assert_eq!(
            quality["qualityEdgeCount"].as_u64().unwrap(),
            plan["report"]["edgeCount"].as_u64().unwrap()
        );
        assert_eq!(plan["lod"]["mode"], "large");
        assert_eq!(
            plan["lod"]["budgets"]["edgeUpdateLimit"].as_u64().unwrap(),
            LARGE_EDGE_UPDATE_LIMIT as u64
        );
        assert_eq!(
            plan["lod"]["budgets"]["runtimeEdgeBudget"]
                .as_u64()
                .unwrap(),
            LARGE_RUNTIME_EDGE_BUDGET as u64
        );
        let stats = quality["visibleTierStats"].as_object().unwrap();
        assert_eq!(
            stats
                .get("node:visible")
                .and_then(Value::as_u64)
                .unwrap_or(0),
            node_count as u64
        );
        assert_eq!(
            stats
                .get("label:visible")
                .and_then(Value::as_u64)
                .unwrap_or(0),
            node_count as u64
        );
    }

    #[test]
    fn position_only_plan_does_not_suppress_labels() {
        let nodes = (0..16usize)
            .map(|index| {
                json!({
                    "id": format!("n{index}"),
                    "clusterId": "dense",
                    "role": if index == 0 { "core" } else { "bridge" },
                    "label": format!("重叠标签{index}"),
                    "labelHalfWidth": 64,
                    "labelBottom": 18,
                    "r": 18,
                })
            })
            .collect::<Vec<_>>();
        let plan = project_flow_layout_network_plan(&json!({ "nodes": nodes, "edges": [] }));
        assert_position_only_network_plan(&plan, 16);
        assert_eq!(plan["quality"]["qualityLabelCount"].as_u64().unwrap(), 16);
        assert_eq!(
            plan["report"]["labelCollisionSuppressedCount"]
                .as_u64()
                .unwrap(),
            0
        );
    }

    #[test]
    fn xlarge_single_giant_component_returns_full_position_updates() {
        let node_count = 5201usize;
        let nodes = (0..node_count)
            .map(|index| {
                json!({
                    "id": format!("n{index}"),
                    "clusterId": "giant",
                    "role": if index == 0 { "core" } else { "leaf" },
                    "r": 18,
                })
            })
            .collect::<Vec<_>>();
        let edges = (1..node_count)
            .map(|index| {
                json!({
                    "source": format!("n{}", index - 1),
                    "target": format!("n{index}"),
                    "weight": if index % 25 == 0 { 3 } else { 1 },
                })
            })
            .collect::<Vec<_>>();

        let plan = project_flow_layout_network_plan(&json!({ "nodes": nodes, "edges": edges }));
        let quality = &plan["quality"];
        let sample_quality = &plan["sampleQuality"];
        let community_quality = plan["communityQuality"].as_array().unwrap();
        assert_eq!(plan["mode"], "xlarge");
        assert_position_only_network_plan(&plan, node_count);
        assert_eq!(quality["qualityScope"], "all");
        assert_eq!(sample_quality["qualityScope"], "all-sample");
        assert_eq!(plan["supergraph"]["mode"], "xlarge");
        assert_eq!(plan["supergraph"]["qualityScope"], "all");
        assert!(
            plan["supergraph"]["nodeCount"].as_u64().unwrap()
                <= plan["report"]["visibleCommunityCount"].as_u64().unwrap()
        );
        assert!(plan["supergraph"]["nodeCount"].as_u64().unwrap() >= 1);
        assert!(plan["supergraph"]["edgeCount"].as_u64().unwrap() <= node_count as u64);
        assert!(!community_quality.is_empty());
        assert!(community_quality.len() <= COMMUNITY_QUALITY_LIMIT);
        assert!(community_quality
            .iter()
            .all(|row| row["qualityScope"] == "community"));
        assert_eq!(
            sample_quality["sampleNodeTotal"].as_u64().unwrap(),
            node_count as u64
        );
        assert!(sample_quality["qualityNodeCount"].as_u64().unwrap() <= node_count as u64);
        assert!(plan["report"]["communityCount"].as_u64().unwrap() >= 1);
        assert_eq!(
            quality["qualityNodeCount"].as_u64().unwrap(),
            node_count as u64
        );
        assert_eq!(plan["lod"]["mode"], "xlarge");
        assert_eq!(
            plan["lod"]["budgets"]["edgeUpdateLimit"].as_u64().unwrap(),
            XLARGE_EDGE_UPDATE_LIMIT as u64
        );
        assert_eq!(
            plan["lod"]["budgets"]["runtimeEdgeBudget"]
                .as_u64()
                .unwrap(),
            XLARGE_RUNTIME_EDGE_BUDGET as u64
        );
        assert_eq!(
            plan["lod"]["visibleNodeCount"].as_u64().unwrap(),
            node_count as u64
        );
        assert_eq!(
            plan["lod"]["visibleEdgeCount"].as_u64().unwrap(),
            plan["report"]["edgeCount"].as_u64().unwrap()
        );
    }

    #[test]
    fn xlarge_many_components_keep_full_position_updates_bounded() {
        let node_count = 6200usize;
        let nodes = (0..node_count)
            .map(|index| {
                json!({
                    "id": format!("n{index}"),
                    "clusterId": format!("component-{index:04}"),
                    "role": if index % 97 == 0 { "core" } else { "leaf" },
                })
            })
            .collect::<Vec<_>>();

        let plan = project_flow_layout_network_plan(&json!({ "nodes": nodes, "edges": [] }));
        let quality = &plan["quality"];

        assert_eq!(plan["mode"], "xlarge");
        assert_position_only_network_plan(&plan, node_count);
        assert_eq!(quality["qualityScope"], "all");
        assert_eq!(
            plan["report"]["communityCount"].as_u64().unwrap(),
            node_count as u64
        );
        assert_eq!(
            quality["qualityNodeCount"].as_u64().unwrap(),
            node_count as u64
        );
        assert_eq!(plan["communities"].as_array().unwrap().len(), node_count);
        assert!(plan["communityQuality"].as_array().unwrap().len() <= COMMUNITY_QUALITY_LIMIT);
        assert!(quality["durationMs"].as_f64().unwrap() >= 0.0);
    }

    #[test]
    fn community_quality_prioritizes_user_intent_hints() {
        let node_count = 5201usize;
        let nodes = (0..node_count)
            .map(|index| {
                let cluster = if index < 80 { "target" } else { "background" };
                json!({
                    "id": format!("n{index}"),
                    "clusterId": cluster,
                    "role": if index == 7 { "core" } else { "leaf" },
                })
            })
            .collect::<Vec<_>>();

        let plan = project_flow_layout_network_plan(&json!({
            "nodes": nodes,
            "edges": [],
            "semantic": {
                "semantic": {
                    "communityQualityNodeIds": ["n7"]
                }
            }
        }));
        let community_quality = plan["communityQuality"].as_array().unwrap();

        assert_eq!(plan["mode"], "xlarge");
        assert!(!community_quality.is_empty());
        assert_eq!(community_quality[0]["communityId"], "target");
        assert_eq!(community_quality[0]["qualityScope"], "community");
        assert_eq!(community_quality[0]["intentMatched"], true);
    }

    #[test]
    fn community_quality_primary_intent_outranks_selected_fallback() {
        let node_count = 5201usize;
        let nodes = (0..node_count)
            .map(|index| {
                let cluster = if index < 80 { "target" } else { "background" };
                json!({
                    "id": format!("n{index}"),
                    "clusterId": cluster,
                    "role": if index >= 80 { "core" } else { "leaf" },
                    "amount": if index >= 80 { 10000 } else { 10 },
                })
            })
            .collect::<Vec<_>>();

        let plan = project_flow_layout_network_plan(&json!({
            "nodes": nodes,
            "edges": [],
            "semantic": {
                "semantic": {
                    "communityQualityPrimaryNodeIds": ["n7"],
                    "communityQualityNodeIds": ["n5000"]
                }
            }
        }));
        let community_quality = plan["communityQuality"].as_array().unwrap();

        assert_eq!(plan["mode"], "xlarge");
        assert!(!community_quality.is_empty());
        assert_eq!(community_quality[0]["communityId"], "target");
        assert_eq!(community_quality[0]["intentMatched"], true);
        assert_eq!(community_quality[0]["intentRank"], 4);
    }

    #[test]
    fn community_quality_path_nodes_are_primary_intent() {
        let node_count = 5201usize;
        let nodes = (0..node_count)
            .map(|index| {
                let cluster = if index < 40 {
                    "path-target"
                } else {
                    "background"
                };
                json!({
                    "id": format!("n{index}"),
                    "clusterId": cluster,
                    "role": if index >= 40 { "core" } else { "leaf" },
                    "amount": if index >= 40 { 10000 } else { 10 },
                })
            })
            .collect::<Vec<_>>();

        let plan = project_flow_layout_network_community_quality(&json!({
            "nodes": nodes,
            "edges": [],
            "semantic": {
                "semantic": {
                    "pathNodeIds": ["n12"],
                    "communityQualityNodeIds": ["n5000"]
                }
            }
        }));
        let community_quality = plan["communityQuality"].as_array().unwrap();

        assert_eq!(plan["mode"], "xlarge");
        assert!(!community_quality.is_empty());
        assert_eq!(community_quality[0]["communityId"], "path-target");
        assert_eq!(community_quality[0]["intentMatched"], true);
        assert_eq!(community_quality[0]["intentRank"], 4);
        assert!(plan["report"]["phaseDurationMs"].is_object());
        assert!(plan.get("nodeUpdates").is_none());
    }

    #[test]
    fn network_plan_hydrates_full_graph_from_snapshot_path() {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        let path = std::env::temp_dir().join(format!("analytix-network-plan-{unique}.json"));
        let snapshot = json!({
            "version": 1,
            "graph": {
                "nodes": [
                    {"id": "snap-a", "clusterId": "left", "role": "core"},
                    {"id": "snap-b", "clusterId": "left", "role": "leaf"},
                    {"id": "snap-c", "clusterId": "right", "role": "leaf"}
                ],
                "edges": [
                    {"source": "snap-a", "target": "snap-b", "amount": 9000},
                    {"source": "snap-b", "target": "snap-c", "amount": 300}
                ]
            }
        });
        fs::write(&path, serde_json::to_vec(&snapshot).unwrap()).unwrap();

        let plan = project_flow_layout_network_plan(&json!({
            "nodes": [],
            "edges": [],
            "source": {"resultSnapshotPath": path.to_string_lossy()},
        }));
        let _ = fs::remove_file(&path);

        assert_eq!(plan["mode"], "small");
        assert_eq!(plan["report"]["nodeCount"], 3);
        assert_eq!(plan["report"]["edgeCount"], 2);
        assert_eq!(plan["report"]["sourceSnapshotPath"], true);
        assert_eq!(plan["nodeUpdates"].as_array().unwrap().len(), 3);
        assert!(plan["nodeUpdates"]
            .as_array()
            .unwrap()
            .iter()
            .any(|row| row["id"] == "snap-a"));
    }

    #[test]
    fn community_quality_hydrates_full_graph_from_snapshot_path() {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        let path =
            std::env::temp_dir().join(format!("analytix-network-community-quality-{unique}.json"));
        let nodes = (0..1301)
            .map(|index| {
                json!({
                    "id": format!("n{index}"),
                    "clusterId": if (8..36).contains(&index) { "target" } else { "other" },
                    "layoutCommunity": if (8..36).contains(&index) { "target" } else { "other" },
                    "layoutVisibilityTier": if index % 43 == 0 || (8..36).contains(&index) { "primary" } else { "hidden" },
                    "layoutLabelTier": if index % 43 == 0 || (8..36).contains(&index) { "primary" } else { "hidden" },
                })
            })
            .collect::<Vec<_>>();
        let edges = (8..35)
            .map(|index| {
                json!({
                    "source": format!("n{index}"),
                    "target": format!("n{}", index + 1),
                    "amount": 300,
                })
            })
            .collect::<Vec<_>>();
        let snapshot = json!({
            "version": 1,
            "graph": {
                "nodes": nodes,
                "edges": edges,
            }
        });
        fs::write(&path, serde_json::to_vec(&snapshot).unwrap()).unwrap();

        let plan = project_flow_layout_network_community_quality(&json!({
            "nodes": [],
            "edges": [],
            "source": {"resultSnapshotPath": path.to_string_lossy()},
            "semantic": {"semantic": {"communityQualityNodeIds": ["n18"]}},
        }));
        let _ = fs::remove_file(&path);

        assert_eq!(plan["mode"], "large");
        assert_eq!(plan["report"]["nodeCount"], 1301);
        assert_eq!(plan["report"]["edgeCount"], 27);
        assert_eq!(plan["report"]["sourceSnapshotPath"], true);
        assert!(plan["report"]["phaseDurationMs"].is_object());
        assert_eq!(plan["communityQuality"][0]["communityId"], "target");
        assert!(plan.get("nodeUpdates").is_none());
    }

    #[test]
    fn community_quality_refresh_is_deterministic_and_layout_free() {
        let node_count = 1301usize;
        let nodes = (0..node_count)
            .map(|index| {
                let target = (7..35).contains(&index);
                json!({
                    "id": format!("n{index}"),
                    "x": (index % 60) as f64 * 18.0,
                    "y": (index / 60) as f64 * 18.0,
                    "layoutCommunity": if target { "target" } else { "other" },
                    "layoutVisibilityTier": if index % 41 == 0 || target { "primary" } else { "hidden" },
                    "layoutLabelTier": if index % 41 == 0 || target { "primary" } else { "hidden" },
                    "r": 18,
                })
            })
            .collect::<Vec<_>>();
        let edges = (7..34)
            .map(|index| {
                json!({
                    "source": format!("n{index}"),
                    "target": format!("n{}", index + 1),
                    "amount": 300,
                })
            })
            .collect::<Vec<_>>();
        let payload = json!({
            "nodes": nodes,
            "edges": edges,
            "semantic": {
                "semantic": {
                    "communityQualityNodeIds": ["n18"]
                }
            }
        });

        let left = project_flow_layout_network_community_quality(&payload);
        let right = project_flow_layout_network_community_quality(&payload);
        let community_quality = left["communityQuality"].as_array().unwrap();
        let mut left_without_duration = left["communityQuality"].clone();
        let mut right_without_duration = right["communityQuality"].clone();
        for value in left_without_duration.as_array_mut().into_iter().flatten() {
            if let Some(row) = value.as_object_mut() {
                row.remove("durationMs");
            }
        }
        for value in right_without_duration.as_array_mut().into_iter().flatten() {
            if let Some(row) = value.as_object_mut() {
                row.remove("durationMs");
            }
        }

        assert!(left.get("nodeUpdates").is_none());
        assert_eq!(left["mode"], "large");
        assert_eq!(left["report"]["qualityOnly"], true);
        assert_eq!(left["report"]["qualityOnlyFastPath"], true);
        assert!(left["report"]["phaseDurationMs"].is_object());
        assert_eq!(left_without_duration, right_without_duration);
        assert!(!community_quality.is_empty());
        assert_eq!(community_quality[0]["communityId"], "target");
        assert_eq!(community_quality[0]["qualityScope"], "community");
        assert_eq!(community_quality[0]["intentMatched"], true);
    }

    #[test]
    fn xlarge_edge_updates_cap_single_hub_bundles() {
        let node_count = 5201usize;
        let nodes = (0..node_count)
            .map(|index| {
                json!({
                    "id": format!("n{index}"),
                    "clusterId": "giant",
                    "role": if index == 0 { "core" } else { "leaf" },
                })
            })
            .collect::<Vec<_>>();
        let edges = (1..node_count)
            .map(|index| {
                json!({
                    "source": "n0",
                    "target": format!("n{index}"),
                    "weight": 5,
                })
            })
            .collect::<Vec<_>>();

        let plan = project_flow_layout_network_plan(&json!({ "nodes": nodes, "edges": edges }));
        let edge_updates = plan["edgeUpdates"].as_array().unwrap();
        let hub_edges = edge_updates
            .iter()
            .filter(|row| row["source"] == "n0" || row["target"] == "n0")
            .count();
        assert_position_only_network_plan(&plan, node_count);
        assert!(edge_updates.is_empty());
        assert!(hub_edges <= 16);
        assert_eq!(
            plan["lod"]["budgets"]["edgeEndpointLimit"]
                .as_u64()
                .unwrap(),
            XLARGE_EDGE_ENDPOINT_LIMIT as u64
        );
    }

    #[test]
    fn xlarge_single_community_returns_positions_without_edge_updates() {
        let node_count = 5201usize;
        let nodes = (0..node_count)
            .map(|index| {
                json!({
                    "id": format!("n{index}"),
                    "clusterId": "mega",
                    "role": if index < 16 { "core" } else { "member" },
                })
            })
            .collect::<Vec<_>>();
        let edges = (0..240usize)
            .map(|index| {
                json!({
                    "source": format!("n{}", index * 2),
                    "target": format!("n{}", index * 2 + 1),
                    "weight": 8 + (index % 7),
                })
            })
            .collect::<Vec<_>>();

        let plan = project_flow_layout_network_plan(&json!({ "nodes": nodes, "edges": edges }));
        let edge_updates = plan["edgeUpdates"].as_array().unwrap();

        assert_eq!(plan["mode"], "xlarge");
        assert_position_only_network_plan(&plan, node_count);
        assert!(edge_updates.is_empty());
    }

    #[test]
    fn xlarge_bridge_graph_returns_all_node_positions_without_skeleton_edges() {
        let community_count = 130usize;
        let community_size = 40usize;
        let nodes = (0..community_count)
            .flat_map(|community| {
                (0..community_size).map(move |index| {
                    json!({
                        "id": format!("c{community}-n{index}"),
                        "clusterId": format!("c{community}"),
                        "role": if index < 2 { "core" } else { "leaf" },
                    })
                })
            })
            .collect::<Vec<_>>();
        let mut edges = Vec::new();
        for community in 0..community_count {
            for leaf in 4..12 {
                edges.push(json!({
                    "source": format!("c{community}-n0"),
                    "target": format!("c{community}-n{leaf}"),
                    "weight": 30,
                }));
                edges.push(json!({
                    "source": format!("c{community}-n1"),
                    "target": format!("c{community}-n{leaf}"),
                    "weight": 24,
                }));
            }
        }
        for community in 0..(community_count - 1) {
            edges.push(json!({
                "source": format!("c{community}-n2"),
                "target": format!("c{}-n2", community + 1),
                "weight": 8,
            }));
        }

        let plan = project_flow_layout_network_plan(&json!({ "nodes": nodes, "edges": edges }));
        let edge_updates = plan["edgeUpdates"].as_array().unwrap();
        assert_eq!(plan["mode"], "xlarge");
        assert_position_only_network_plan(&plan, community_count * community_size);
        assert!(edge_updates.is_empty());
    }

    #[test]
    fn xlarge_repeated_cross_community_edges_do_not_emit_display_edge_updates() {
        let community_count = 130usize;
        let community_size = 40usize;
        let nodes = (0..community_count)
            .flat_map(|community| {
                (0..community_size).map(move |index| {
                    json!({
                        "id": format!("c{community}-n{index}"),
                        "clusterId": format!("c{community}"),
                        "role": if index < 2 { "core" } else { "leaf" },
                    })
                })
            })
            .collect::<Vec<_>>();
        let mut edges = Vec::new();
        for community in 0..community_count {
            for leaf in 4..12 {
                edges.push(json!({
                    "source": format!("c{community}-n0"),
                    "target": format!("c{community}-n{leaf}"),
                    "weight": 30,
                }));
            }
        }
        for offset in 2..26 {
            edges.push(json!({
                "source": format!("c0-n{offset}"),
                "target": format!("c1-n{offset}"),
                "weight": 9,
            }));
        }

        let plan = project_flow_layout_network_plan(&json!({ "nodes": nodes, "edges": edges }));
        let edge_updates = plan["edgeUpdates"].as_array().unwrap();

        assert_eq!(plan["mode"], "xlarge");
        assert_position_only_network_plan(&plan, community_count * community_size);
        assert!(edge_updates.is_empty());
        assert_eq!(plan["report"]["superEdgeCount"].as_u64().unwrap(), 0);
        assert_eq!(plan["report"]["bundledEdgeCount"].as_u64().unwrap(), 0);
        assert_eq!(plan["report"]["largestSuperEdgeSize"].as_u64().unwrap(), 0);
        assert_eq!(plan["report"]["routedSuperEdgeCount"].as_u64().unwrap(), 0);
    }
}
