use anyhow::{bail, Context, Result};
use serde_json::{json, Value};
use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{SystemTime, UNIX_EPOCH};

static TEMP_JSON_SEQUENCE: AtomicU64 = AtomicU64::new(0);

struct TempJson {
    path: PathBuf,
}

impl TempJson {
    fn new(label: &str, payload: &Value) -> Result<Self> {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .context("system clock before UNIX_EPOCH")?
            .as_nanos();
        let sequence = TEMP_JSON_SEQUENCE.fetch_add(1, Ordering::Relaxed);
        let path = std::env::temp_dir().join(format!(
            "analytix-{label}-{}-{unique}-{sequence}.json",
            std::process::id(),
        ));
        let mut file = OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&path)
            .context("create unique projection input JSON")?;
        file.write_all(&serde_json::to_vec(payload)?)
            .context("write projection input JSON")?;
        file.sync_all().context("sync projection input JSON")?;
        Ok(Self { path })
    }

    fn path(&self) -> &Path {
        &self.path
    }
}

impl Drop for TempJson {
    fn drop(&mut self) {
        let _ = fs::remove_file(&self.path);
    }
}

#[test]
fn project_flow_skeleton_clusters_cli_returns_golden_projection() -> Result<()> {
    let input = TempJson::new("flow-projection", &fixture_payload())?;
    let output = run_project_flow_skeleton_clusters(input.path())?;

    assert_eq!(output["ok"], true);
    assert_eq!(
        output["projection"]["anchor_ids"],
        json!(["focus", "seed", "leaf-10", "leaf-09", "solo-a"])
    );
    assert_eq!(
        output["projection"]["search_match_node_ids"],
        json!(["leaf-08"])
    );
    assert_eq!(output["projection"]["base_projection_node_ids"], json!([]));
    assert_eq!(output["projection"]["path_entity_ids"], json!([]));
    assert_eq!(
        output["projection"]["requested_cluster_materializations"],
        json!([])
    );
    assert_eq!(
        output["projection"]["explicit_entity_ids"],
        json!([
            "focus", "leaf-01", "leaf-02", "leaf-03", "leaf-04", "leaf-05", "leaf-06", "leaf-07",
            "leaf-08", "leaf-09", "leaf-10", "seed", "solo-a", "solo-b"
        ])
    );
    assert_eq!(
        output["projection"]["clusters"],
        json!([
            {
                "cluster_id": "__cluster__::seed::19bcd2b64a1fa678",
                "anchor_id": "seed",
                "anchor_ids": ["seed"],
                "member_ids": ["leaf-01", "leaf-02", "leaf-03", "leaf-04", "leaf-05", "leaf-06", "leaf-07", "leaf-08"],
                "ranked_member_ids": ["leaf-08", "leaf-07", "leaf-06", "leaf-05", "leaf-04", "leaf-03", "leaf-02", "leaf-01"],
                "tiles": [
                    {"tile_id": "__cluster__::seed::19bcd2b64a1fa678::tile::0000", "cluster_id": "__cluster__::seed::19bcd2b64a1fa678", "member_ids": ["leaf-08", "leaf-07", "leaf-06"], "member_count": 3, "total_amount": 321.0, "total_count": 21},
                    {"tile_id": "__cluster__::seed::19bcd2b64a1fa678::tile::0001", "cluster_id": "__cluster__::seed::19bcd2b64a1fa678", "member_ids": ["leaf-05", "leaf-04", "leaf-03"], "member_count": 3, "total_amount": 312.0, "total_count": 12},
                    {"tile_id": "__cluster__::seed::19bcd2b64a1fa678::tile::0002", "cluster_id": "__cluster__::seed::19bcd2b64a1fa678", "member_ids": ["leaf-02", "leaf-01"], "member_count": 2, "total_amount": 203.0, "total_count": 3}
                ],
            }
        ])
    );
    assert_eq!(
        output["projection"]["cluster_visibility_plan"]["collapsed_member_to_cluster"],
        json!({})
    );
    assert_eq!(
        output["projection"]["cluster_visibility_plan"]["expanded_cluster_ids"],
        json!(["__cluster__::seed::19bcd2b64a1fa678"])
    );
    assert_eq!(
        output["projection"]["cluster_visibility_plan"]["expanded_tile_ids"],
        json!([
            "__cluster__::seed::19bcd2b64a1fa678::tile::0000",
            "__cluster__::seed::19bcd2b64a1fa678::tile::0001",
            "__cluster__::seed::19bcd2b64a1fa678::tile::0002"
        ])
    );
    assert_eq!(
        output["projection"]["cluster_visibility_plan"]["clusters"][0]["expanded"],
        json!(true)
    );
    assert_eq!(
        output["projection"]["cluster_visibility_plan"]["clusters"][0]["collapsed_member_ids"],
        json!([])
    );

    Ok(())
}

#[test]
fn project_flow_skeleton_clusters_cli_rejects_unknown_facts_before_candidate_output() -> Result<()>
{
    let mut missing_node_amount = fixture_payload();
    missing_node_amount["nodes"][1]
        .as_object_mut()
        .context("node should be an object")?
        .remove("total_amount");
    assert_projection_rejected_without_candidate(&missing_node_amount, "nodes[1].total_amount")?;

    let mut missing_node_count = fixture_payload();
    missing_node_count["nodes"][2]
        .as_object_mut()
        .context("node should be an object")?
        .remove("total_count");
    assert_projection_rejected_without_candidate(&missing_node_count, "nodes[2].total_count")?;

    let mut mixed_valid_and_missing = fixture_payload();
    mixed_valid_and_missing["edges"][4]
        .as_object_mut()
        .context("edge should be an object")?
        .remove("count");
    assert_projection_rejected_without_candidate(&mixed_valid_and_missing, "edges[4].count")?;

    let mut nonfinite_direction = fixture_payload();
    nonfinite_direction["edges"][0]["forward_amount"] = json!("Infinity");
    assert_projection_rejected_without_candidate(&nonfinite_direction, "edges[0].forward_amount")?;

    let mut mismatched_direction = fixture_payload();
    mismatched_direction["edges"][0]["amount"] = json!(1.0);
    assert_projection_rejected_without_candidate(
        &mismatched_direction,
        "direction coverage mismatch",
    )?;

    let mut unknown_target = fixture_payload();
    unknown_target["edges"][0]["target"] = json!("not-covered-by-nodes");
    assert_projection_rejected_without_candidate(&unknown_target, "references unknown node")?;

    let mut stale_base_projection = fixture_payload();
    stale_base_projection["base_projection"] = json!({
        "anchor_node_ids": ["seed"],
        "expanded_node_ids": ["not-covered-by-nodes"],
        "clusters": []
    });
    assert_projection_rejected_without_candidate(
        &stale_base_projection,
        "base_projection.expanded_node_ids[0] references unknown node",
    )?;

    Ok(())
}

#[test]
fn project_layout_node_plan_cli_matches_worker_node_plan_contract() -> Result<()> {
    let output = run_project_layout_node_plan(&json!({
        "nodes": [
            {"id": "n1", "x": "10", "y": 20, "layoutBand": "core", "layoutRole": ""},
            {"id": "n2", "x": 30, "y": "40", "layoutBand": "leaf"}
        ],
        "metaKeys": ["layoutBand", "layoutRole"]
    }))?;

    assert_eq!(output["ok"], true);
    assert_eq!(
        output["nodePlan"],
        json!({
            "n1": {"id": "n1", "x": 10.0, "y": 20.0, "layoutBand": "core"},
            "n2": {"id": "n2", "x": 30.0, "y": 40.0, "layoutBand": "leaf"}
        })
    );
    assert_eq!(
        output["updates"],
        json!([
            {"index": 0, "id": "n1", "x": 10.0, "y": 20.0, "layoutBand": "core"},
            {"index": 1, "id": "n2", "x": 30.0, "y": 40.0, "layoutBand": "leaf"}
        ])
    );

    let invalid = run_project_layout_node_plan(&json!({
        "nodes": [{"id": "bad", "x": "x", "y": 1}],
        "metaKeys": ["layoutBand"]
    }))?;
    assert_eq!(invalid["nodePlan"], Value::Null);
    assert_eq!(invalid["updates"], json!([]));

    Ok(())
}

#[test]
fn apply_and_clear_layout_node_plan_cli_match_worker_node_plan_contract() -> Result<()> {
    let apply_output = run_layout_node_plan_command(
        "apply-layout-node-plan",
        &json!({
            "nodes": [
                {"id": "n1", "x": 0, "y": 0, "layoutBand": "old", "layoutRole": "old"},
                {"id": "n2", "x": 5, "y": 6, "layoutBand": "old"}
            ],
            "nodesById": {
                "n1": {"id": "n1", "x": "10", "y": 20, "layoutBand": "core", "layoutRole": ""},
                "n2": {"id": "n2", "x": 30, "y": "40", "layoutBand": "leaf"}
            },
            "metaKeys": ["layoutBand", "layoutRole"]
        }),
    )?;

    assert_eq!(
        apply_output["result"],
        json!({
            "applied": true,
            "nodes": [
                {"id": "n1", "x": 10.0, "y": 20.0, "layoutBand": "core"},
                {"id": "n2", "x": 30.0, "y": 40.0, "layoutBand": "leaf"}
            ]
        })
    );

    let invalid_apply = run_layout_node_plan_command(
        "apply-layout-node-plan",
        &json!({
            "nodes": [
                {"id": "n1", "x": 0, "y": 0},
                {"id": "n2", "x": 5, "y": 6}
            ],
            "nodesById": {
                "n1": {"id": "n1", "x": 10, "y": 20},
                "n2": {"id": "n2", "x": "bad", "y": 40}
            },
            "metaKeys": ["layoutBand"]
        }),
    )?;
    assert_eq!(
        invalid_apply["result"],
        json!({
            "applied": false,
            "nodes": [
                {"id": "n1", "x": 0, "y": 0},
                {"id": "n2", "x": 5, "y": 6}
            ]
        })
    );

    let worker_apply = run_layout_node_plan_command(
        "apply-layout-worker-node-plan",
        &json!({
            "nodes": [
                {"id": "n1", "x": 0, "y": 0, "layoutBand": "old", "layoutRole": "old"},
                {"id": "n2", "x": 5, "y": 6, "layoutBand": "old"}
            ],
            "workerNodes": [
                {"id": "n1", "x": "10", "y": 20, "layoutBand": "core", "layoutRole": ""},
                {"id": "n2", "x": 30, "y": "40", "layoutBand": "leaf"}
            ],
            "metaKeys": ["layoutBand", "layoutRole"]
        }),
    )?;
    assert_eq!(
        worker_apply["result"],
        json!({
            "applied": true,
            "source": "plan",
            "matched": 2,
            "gridFallbackLikely": false,
            "updates": [
                {"index": 0, "id": "n1", "x": 10.0, "y": 20.0, "layoutBand": "core"},
                {"index": 1, "id": "n2", "x": 30.0, "y": 40.0, "layoutBand": "leaf"}
            ],
            "nodes": [
                {"id": "n1", "x": 10.0, "y": 20.0, "layoutBand": "core"},
                {"id": "n2", "x": 30.0, "y": 40.0, "layoutBand": "leaf"}
            ]
        })
    );

    let missing_worker_apply = run_layout_node_plan_command(
        "apply-layout-worker-node-plan",
        &json!({
            "nodes": [
                {"id": "n1", "x": 0, "y": 0, "layoutBand": "old"},
                {"id": "n2", "x": 5, "y": 6, "layoutBand": "old"}
            ],
            "workerNodes": [
                {"id": "n1", "x": 10, "y": 20, "layoutBand": "core"}
            ],
            "metaKeys": ["layoutBand"]
        }),
    )?;
    assert_eq!(
        missing_worker_apply["result"],
        json!({
            "applied": false,
            "source": "apply-failed",
            "matched": 0,
            "gridFallbackLikely": false,
            "updates": [],
            "nodes": [
                {"id": "n1", "x": 0, "y": 0, "layoutBand": "old"},
                {"id": "n2", "x": 5, "y": 6, "layoutBand": "old"}
            ]
        })
    );

    let worker_meta_clear_apply = run_layout_node_plan_command(
        "apply-layout-worker-node-plan",
        &json!({
            "nodes": [
                {"id": "n1", "x": 0, "y": 0, "layoutBand": "old", "role": "old"},
                {"id": "n2", "x": 5, "y": 6, "layoutBand": "old", "role": "old"}
            ],
            "workerNodes": [
                {"id": "n1", "x": "10", "y": 20, "layoutBand": null, "role": ""},
                {"id": "n2", "x": 30, "y": "40", "role": "leaf"}
            ],
            "metaKeys": ["layoutBand", "role"]
        }),
    )?;
    assert_eq!(
        worker_meta_clear_apply["result"],
        json!({
            "applied": true,
            "source": "plan",
            "matched": 2,
            "gridFallbackLikely": false,
            "updates": [
                {"index": 0, "id": "n1", "x": 10.0, "y": 20.0},
                {"index": 1, "id": "n2", "x": 30.0, "y": 40.0, "role": "leaf"}
            ],
            "nodes": [
                {"id": "n1", "x": 10.0, "y": 20.0},
                {"id": "n2", "x": 30.0, "y": 40.0, "role": "leaf"}
            ]
        })
    );

    let clear_output = run_layout_node_plan_command(
        "clear-layout-node-meta",
        &json!({
            "nodes": [
                {"id": "n1", "x": 1, "y": 2, "layoutBand": "core", "layoutRole": "kept"},
                {"id": "n2", "x": 3, "y": 4, "layoutBand": "leaf"}
            ],
            "metaKeys": ["layoutBand"]
        }),
    )?;
    assert_eq!(
        clear_output["nodes"],
        json!([
            {"id": "n1", "x": 1, "y": 2, "layoutRole": "kept"},
            {"id": "n2", "x": 3, "y": 4}
        ])
    );

    Ok(())
}

#[test]
fn project_layout_role_graph_cli_matches_js_role_graph_contract() -> Result<()> {
    let output = run_layout_node_plan_command(
        "project-layout-role-graph",
        &json!({
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
        }),
    )?;

    assert_eq!(output["ok"], true);
    assert_eq!(
        output["projection"],
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

    Ok(())
}

#[test]
fn project_layout_role_graph_cli_projects_role_resolution() -> Result<()> {
    let output = run_layout_node_plan_command(
        "project-layout-role-graph",
        &json!({
            "nodes": [
                {"id": "a"},
                {"id": "b"},
                {"id": "c"},
                {"id": "leaf"}
            ],
            "edges": [
                {"source": "a", "target": "b"},
                {"source": "b", "target": "c"},
                {"source": "b", "target": "leaf"}
            ],
            "roleResolution": {
                "coreIds": ["a", "c"],
                "pathPromotedAdjacentIds": ["b"],
                "options": {
                    "seedMeta": {
                        "a": {
                            "isSeedSelected": true,
                            "seedWeight": 7,
                            "seedStabilityScore": 0.5
                        }
                    }
                }
            }
        }),
    )?;

    assert_eq!(output["ok"], true);
    assert_eq!(
        output["projection"]["roleResult"],
        json!({
            "rolesById": {"a": "core", "b": "adjacent", "c": "core", "leaf": "leaf"},
            "nodeMetaById": {
                "a": {
                    "id": "a",
                    "role": "core",
                    "seedCoreCandidate": true,
                    "isSeedSelected": true,
                    "demoteReason": null,
                    "isPathPromotedAdjacent": false,
                    "isBridgeAdjacent": false,
                    "isHubAdjacent": false,
                    "layoutAnchorForCluster": false,
                    "why": ["seed_core_kept"],
                    "weight": 7,
                    "seedWeight": 7,
                    "seedStabilityScore": 0.5
                },
                "b": {
                    "id": "b",
                    "role": "adjacent",
                    "seedCoreCandidate": false,
                    "isSeedSelected": false,
                    "demoteReason": null,
                    "isPathPromotedAdjacent": true,
                    "isBridgeAdjacent": true,
                    "isHubAdjacent": false,
                    "layoutAnchorForCluster": false,
                    "why": ["path_promoted_adjacent", "adjacent_multi_core", "bridge_adjacent"],
                    "weight": 0,
                    "seedWeight": 0,
                    "seedStabilityScore": 0
                },
                "c": {
                    "id": "c",
                    "role": "core",
                    "seedCoreCandidate": false,
                    "isSeedSelected": false,
                    "demoteReason": null,
                    "isPathPromotedAdjacent": false,
                    "isBridgeAdjacent": false,
                    "isHubAdjacent": false,
                    "layoutAnchorForCluster": false,
                    "why": [],
                    "weight": 0,
                    "seedWeight": 0,
                    "seedStabilityScore": 0
                },
                "leaf": {
                    "id": "leaf",
                    "role": "leaf",
                    "seedCoreCandidate": false,
                    "isSeedSelected": false,
                    "demoteReason": null,
                    "isPathPromotedAdjacent": false,
                    "isBridgeAdjacent": false,
                    "isHubAdjacent": false,
                    "layoutAnchorForCluster": false,
                    "why": ["leaf_single_entry"],
                    "weight": 0,
                    "seedWeight": 0,
                    "seedStabilityScore": 0
                }
            },
            "leafEntryById": {"leaf": "b"},
            "coreIds": ["a", "c"],
            "adjacentIds": ["b"],
            "leafIds": ["leaf"],
            "pathPromotedAdjacentIds": ["b"],
            "bridgeAdjacentIds": ["b"],
            "hubAdjacentIds": []
        })
    );

    Ok(())
}

#[test]
fn project_layout_role_graph_cli_projects_cluster_detection() -> Result<()> {
    let output = run_layout_node_plan_command(
        "project-layout-role-graph",
        &json!({
            "nodes": [
                {"id": "A"},
                {"id": "B"},
                {"id": "C"},
                {"id": "D"},
                {"id": "L1"},
                {"id": "L2"}
            ],
            "edges": [
                {"source": "A", "target": "B"},
                {"source": "B", "target": "C"},
                {"source": "C", "target": "D"},
                {"source": "B", "target": "L1"},
                {"source": "C", "target": "L2"}
            ],
            "clusterResolution": {
                "roles": {
                    "rolesById": {
                        "A": "core",
                        "B": "adjacent",
                        "C": "adjacent",
                        "D": "core",
                        "L1": "leaf",
                        "L2": "leaf"
                    },
                    "nodeMetaById": {
                        "A": {"id": "A", "role": "core", "why": []},
                        "B": {"id": "B", "role": "adjacent", "why": []},
                        "C": {"id": "C", "role": "adjacent", "why": []},
                        "D": {"id": "D", "role": "core", "why": []},
                        "L1": {"id": "L1", "role": "leaf", "why": []},
                        "L2": {"id": "L2", "role": "leaf", "why": []}
                    }
                }
            }
        }),
    )?;

    assert_eq!(output["ok"], true);
    let result = &output["projection"]["clusterResult"];
    assert_eq!(
        result["clusters"],
        json!([
            {
                "clusterId": "cluster-0-0",
                "componentId": 0,
                "nodeIds": ["A", "B", "L1"],
                "coreIds": ["A"],
                "adjacentIds": ["B"],
                "leafIds": ["L1"],
                "layoutAnchorIds": [],
                "layoutCase": "general",
                "bbox": null,
                "hasCore": true,
                "edgeCount": 2,
                "localAdjacency": {"A": ["B"], "B": ["A", "L1"], "L1": ["B"]}
            },
            {
                "clusterId": "cluster-0-1",
                "componentId": 0,
                "nodeIds": ["C", "D", "L2"],
                "coreIds": ["D"],
                "adjacentIds": ["C"],
                "leafIds": ["L2"],
                "layoutAnchorIds": [],
                "layoutCase": "general",
                "bbox": null,
                "hasCore": true,
                "edgeCount": 2,
                "localAdjacency": {"C": ["D", "L2"], "D": ["C"], "L2": ["C"]}
            }
        ])
    );
    assert_eq!(
        result["nodeClusterById"],
        json!({
            "A": "cluster-0-0",
            "B": "cluster-0-0",
            "L1": "cluster-0-0",
            "C": "cluster-0-1",
            "D": "cluster-0-1",
            "L2": "cluster-0-1"
        })
    );
    assert_eq!(result["nodeMetaById"]["B"]["isBridgeAdjacent"], true);
    assert_eq!(
        result["nodeMetaById"]["B"]["why"],
        json!(["bridge_adjacent"])
    );
    assert_eq!(result["nodeMetaById"]["C"]["isBridgeAdjacent"], true);
    assert_eq!(
        result["nodeMetaById"]["C"]["clusterId"],
        json!("cluster-0-1")
    );

    Ok(())
}

#[test]
fn project_layout_role_graph_cli_projects_semantic_pipeline() -> Result<()> {
    let output = run_layout_node_plan_command(
        "project-layout-role-graph",
        &json!({
            "nodes": [
                {"id": "A", "total_amount": 1000},
                {"id": "B"},
                {"id": "C", "total_amount": 1000},
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
        }),
    )?;

    assert_eq!(output["ok"], true);
    let projection = &output["projection"];
    assert_eq!(projection["demotion"]["coreIds"], json!(["A", "C"]));
    assert_eq!(
        projection["promotion"]["pathPromotedAdjacentIds"],
        json!(["B"])
    );
    assert_eq!(
        projection["roleResult"]["rolesById"],
        json!({"A": "core", "B": "adjacent", "C": "core", "L": "leaf"})
    );
    assert_eq!(
        projection["clusterResult"]["clusters"][0]["nodeIds"],
        json!(["A", "B", "C", "L"])
    );
    assert_eq!(
        projection["clusterResult"]["nodeClusterById"],
        json!({"A": "cluster-0-0", "B": "cluster-0-0", "C": "cluster-0-0", "L": "cluster-0-0"})
    );

    Ok(())
}

#[test]
fn project_flow_projection_layout_sync_cli_returns_layout_index_summary() -> Result<()> {
    let output = run_layout_node_plan_command(
        "project-flow-projection-layout-sync",
        &json!({
            "viewport": {
                "zoom": "2",
                "center": {"x": 10, "y": "-5"},
                "size": {"width": 0, "height": 480},
                "width": 960
            },
            "graph_size": {"width": "1200", "height": 800},
            "currentNodes": [
                {
                    "id": "cluster",
                    "x": 0,
                    "y": 2,
                    "cluster_node": true,
                    "nodeRenderMode": "entity",
                    "projection_visible": true,
                    "projection_collapsed": true
                },
                {
                    "id": "dot",
                    "x": 5,
                    "y": -4,
                    "cluster_node": false,
                    "nodeRenderMode": "dot",
                    "projection_cluster_id": "cluster",
                    "projection_visible": false,
                    "projection_collapsed": false
                }
            ],
            "nodes": [
                {"id": "cluster", "x": "1", "y": 2, "cluster_node": true, "node_render_mode": "entity", "projection_visible": true, "projection_collapsed": true},
                {"id": "dot", "x": 5, "y": "-4", "nodeRenderMode": "dot", "projection_cluster_id": "cluster", "projection_visible": false, "projection_collapsed": false},
                {"id": "invalid", "x": "NaN", "y": 3}
            ]
        }),
    )?;

    assert_eq!(output["ok"], true);
    assert_eq!(
        output["projection"],
        json!({
            "nodes": [
                {
                    "id": "cluster",
                    "x": 1.0,
                    "y": 2.0,
                    "cluster_node": true,
                    "projection_cluster_id": "",
                    "node_render_mode": "entity",
                    "projection_visible": true,
                    "projection_collapsed": true
                },
                {
                    "id": "dot",
                    "x": 5.0,
                    "y": -4.0,
                    "cluster_node": false,
                    "projection_cluster_id": "cluster",
                    "node_render_mode": "dot",
                    "projection_visible": false,
                    "projection_collapsed": false
                }
            ],
            "signature": "::n:2::z:2::c:10,-5::s:0x480::b:1,5,-4,2::m:cluster:1:2:entity|dot:5:-4:dot",
            "layout_by_id": {
                "cluster": {
                    "x": 1.0,
                    "y": 2.0,
                    "cluster_node": true,
                    "projection_cluster_id": "",
                    "nodeRenderMode": "entity",
                    "projection_visible": true,
                    "projection_collapsed": true
                },
                "dot": {
                    "x": 5.0,
                    "y": -4.0,
                    "cluster_node": false,
                    "projection_cluster_id": "cluster",
                    "nodeRenderMode": "dot",
                    "projection_visible": false,
                    "projection_collapsed": false
                }
            },
            "node_updates": [
                {
                    "index": 0,
                    "id": "cluster",
                    "x": 1.0,
                    "y": 2.0,
                    "cluster_node": true,
                    "nodeRenderMode": "entity",
                    "projection_visible": true,
                    "projection_collapsed": true
                }
            ],
            "node_update_count": 1,
            "layout_index_summary": {
                "version": 1,
                "signature": "::n:2::z:2::c:10,-5::s:0x480::b:1,5,-4,2::m:cluster:1:2:entity|dot:5:-4:dot",
                "indexed_node_count": 2,
                "cluster_node_count": 1,
                "dot_node_count": 1,
                "bounds": {"min_x": 1.0, "max_x": 5.0, "min_y": -4.0, "max_y": 2.0},
                "viewport": {
                    "zoom": 2.0,
                    "center": {"x": 10.0, "y": -5.0},
                    "size": {"width": 960.0, "height": 480.0}
                },
                "graph_size": {"width": 1200.0, "height": 800.0}
            }
        })
    );

    Ok(())
}

#[test]
fn project_flow_skeleton_clusters_cli_plans_point_layer_visibility() -> Result<()> {
    let mut payload = fixture_payload();
    payload["edges"][0]["amount"] = json!(101.0);
    payload["edges"][0]["count"] = json!(2);
    payload["edges"][0]["out_amount"] = json!(0.0);
    payload["edges"][0]["in_amount"] = json!(101.0);
    payload["edges"][0]["forward_amount"] = json!(101.0);
    payload["edges"][0]["reverse_amount"] = json!(0.0);
    payload["edges"][0]["first_time"] = json!("2026-01-01 09:00:00");
    payload["edges"][0]["last_time"] = json!("2026-01-02 09:00:00");
    payload["request_context"] = json!({
        "cluster_ids": ["existing-cluster"],
        "include_neighbors": false,
        "materialize_limit": 3,
        "use_point_layer": true,
    });
    payload["base_projection"] = json!({
        "anchor_node_ids": ["seed"],
        "clusters": [
            {
                "cluster_id": "existing-cluster",
                "anchor_id": "seed",
                "member_ids": ["leaf-01", "leaf-02", "leaf-03", "leaf-04"],
            }
        ],
    });
    let input = TempJson::new("flow-projection-point-layer", &payload)?;
    let output = run_project_flow_skeleton_clusters(input.path())?;

    assert_eq!(output["ok"], true);
    assert_eq!(
        output["projection"]["requested_cluster_materializations"],
        json!([
            {
                "cluster_id": "existing-cluster",
                "materialized_member_ids": ["leaf-04", "leaf-03", "leaf-02"],
            }
        ])
    );
    assert_eq!(
        output["projection"]["cluster_visibility_plan"]["collapsed_member_to_cluster"],
        json!({"leaf-01": "existing-cluster"})
    );
    assert_eq!(
        output["projection"]["cluster_visibility_plan"]["point_layer_total"],
        json!(1)
    );
    assert_eq!(
        output["projection"]["cluster_visibility_plan"]["point_layer_buckets"],
        json!([
            {
                "cluster_id": "existing-cluster",
                "anchor_id": "seed",
                "point_count": 1,
                "member_count": 1,
                "tile_count": 2,
                "remaining_tile_count": 1,
                "next_tile_ids": ["existing-cluster::tile::0001"],
                "sample_node_ids": ["leaf-01"],
                "total_amount": 101.0,
                "total_count": 1,
            }
        ])
    );
    let node_plan = &output["projection"]["projected_node_plan"];
    assert_eq!(node_plan["dot_count"], json!(0));
    assert_eq!(node_plan["entity_count"], json!(14));
    assert_eq!(
        node_plan["expanded_cluster_ids"],
        json!(["existing-cluster"])
    );
    assert_eq!(
        node_plan["expanded_tile_ids"],
        json!(["existing-cluster::tile::0000"])
    );
    assert_eq!(
        node_plan["expanded_node_ids"],
        json!(["leaf-02", "leaf-03", "leaf-04"])
    );
    let projected_nodes = node_plan["nodes"]
        .as_array()
        .context("projected node plan nodes should be an array")?;
    let node_ids: Vec<&str> = projected_nodes
        .iter()
        .map(|node| node["id"].as_str().unwrap_or(""))
        .collect();
    assert!(!node_ids.contains(&"leaf-01"));
    assert!(node_ids.contains(&"existing-cluster"));
    let seed_node = projected_nodes
        .iter()
        .find(|node| node["id"] == "seed")
        .context("seed node should be projected")?;
    assert_eq!(seed_node["nodeRenderMode"], json!("focus"));
    assert_eq!(seed_node["projection_visible"], json!(true));
    assert_eq!(seed_node["projection_collapsed"], json!(false));
    for materialized_id in ["leaf-02", "leaf-03", "leaf-04"] {
        let node = projected_nodes
            .iter()
            .find(|node| node["id"] == materialized_id)
            .with_context(|| format!("{materialized_id} should be projected"))?;
        assert_eq!(node["nodeRenderMode"], json!("entity"));
        assert_eq!(node["projection_visible"], json!(true));
        assert_eq!(node["projection_collapsed"], json!(false));
    }
    let cluster_node = projected_nodes
        .iter()
        .find(|node| node["id"] == "existing-cluster")
        .context("existing-cluster virtual node should be projected")?;
    assert_eq!(cluster_node["cluster_node"], json!(true));
    assert_eq!(cluster_node["projection_visible"], json!(true));
    assert_eq!(cluster_node["nodeRenderMode"], json!("focus"));
    assert_eq!(
        node_plan["clusters"],
        json!([
            {
                "cluster_id": "existing-cluster",
                "anchor_id": "seed",
                "anchor_ids": ["seed"],
                "member_ids": [],
                "remaining_member_ids": [],
                "member_count": 4,
                "remaining_member_count": 1,
                "materialized_member_count": 3,
                "expanded": false,
                "partially_expanded": true,
                "tile_count": 2,
                "expanded_tile_ids": ["existing-cluster::tile::0000"],
                "expanded_tile_count": 1,
                "remaining_tile_ids": ["existing-cluster::tile::0001"],
                "remaining_tile_count": 1,
                "next_tile_ids": ["existing-cluster::tile::0001"],
                "title": "主账户 关联簇 (1/4)",
                "total_amount": 101.0,
                "total_count": 1,
            }
        ])
    );
    let edge_plan = &output["projection"]["projected_edge_plan"];
    assert_eq!(edge_plan["total_amount"], json!(101.0));
    assert_eq!(edge_plan["total_count"], json!(2));
    let projected_edges = edge_plan["edges"]
        .as_array()
        .context("projected edge plan edges should be an array")?;
    let edge_ids: Vec<&str> = projected_edges
        .iter()
        .map(|edge| edge["id"].as_str().unwrap_or(""))
        .collect();
    assert_eq!(
        edge_ids,
        vec![
            "existing-cluster==seed",
            "leaf-02==seed",
            "leaf-03==seed",
            "leaf-04==seed",
            "leaf-05==seed",
            "leaf-06==seed",
            "leaf-07==seed",
            "leaf-08==seed",
            "leaf-09==seed",
            "leaf-10==seed",
            "solo-a==solo-b",
        ]
    );
    assert_eq!(
        projected_edges[0],
        json!({
            "id": "existing-cluster==seed",
            "source": "seed",
            "target": "existing-cluster",
            "amount": 101.0,
            "count": 2,
            "out_amount": 0.0,
            "in_amount": 101.0,
            "forward_amount": 101.0,
            "reverse_amount": 0.0,
            "mode": "single",
            "first_time": "2026-01-01 09:00:00",
            "last_time": "2026-01-02 09:00:00",
            "projection_edge": true,
        })
    );
    for edge in projected_edges.iter().skip(1) {
        assert_eq!(edge["amount"], json!(0.0));
        assert_eq!(edge["count"], json!(0));
        assert_eq!(edge["out_amount"], json!(0.0));
        assert_eq!(edge["in_amount"], json!(0.0));
        assert_eq!(edge["projection_edge"], json!(true));
    }

    Ok(())
}

fn fixture_payload() -> Value {
    let mut nodes = vec![
        json!({
            "id": "seed",
            "title": "主账户",
            "name": "主账户",
            "display_id": "62230000",
            "ntype": "seed",
            "total_amount": 5.0,
            "total_count": 9,
        }),
        json!({
            "id": "focus",
            "title": "重点账户",
            "name": "重点账户",
            "display_id": "62239999",
            "ntype": "account",
            "total_amount": 900.0,
            "total_count": 1,
        }),
        json!({"id": "solo-a", "title": "孤立甲", "name": "孤立甲", "display_id": "solo-a", "total_amount": 60.0, "total_count": 2}),
        json!({"id": "solo-b", "title": "孤立乙", "name": "孤立乙", "display_id": "solo-b", "total_amount": 40.0, "total_count": 1}),
    ];
    for index in 1..=10 {
        nodes.push(json!({
            "id": format!("leaf-{index:02}"),
            "title": format!("叶子{index:02}"),
            "name": format!("叶子{index:02}"),
            "display_id": format!("7333{index:04}"),
            "display_ids": [format!("alias-{index:02}")],
            "ntype": "account",
            "total_amount": 100.0 + index as f64,
            "total_count": index,
        }));
    }
    let mut edges = Vec::new();
    for index in 1..=10 {
        edges.push(json!({
            "id": format!("seed==leaf-{index:02}"),
            "source": "seed",
            "target": format!("leaf-{index:02}"),
            "amount": 0.0,
            "count": 0,
            "out_amount": 0.0,
            "in_amount": 0.0,
            "forward_amount": 0.0,
            "reverse_amount": 0.0,
            "mode": "single",
        }));
    }
    edges.push(json!({
        "id": "solo-a==solo-b",
        "source": "solo-a",
        "target": "solo-b",
        "amount": 0.0,
        "count": 0,
        "out_amount": 0.0,
        "in_amount": 0.0,
        "forward_amount": 0.0,
        "reverse_amount": 0.0,
        "mode": "single",
    }));
    json!({
        "nodes": nodes,
        "edges": edges,
        "request_context": {"focus_ids": ["focus"], "search_query": "73330008", "search_limit": 8},
        "tile_size": 3,
    })
}

fn run_project_flow_skeleton_clusters(path: &Path) -> Result<Value> {
    let output = run_project_flow_skeleton_clusters_raw(path)?;
    if !output.status.success() {
        bail!(
            "project-flow-skeleton-clusters failed: stdout={} stderr={}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
    }
    let stdout = String::from_utf8(output.stdout).context("decode command stdout")?;
    serde_json::from_str(stdout.trim()).context("parse command stdout JSON")
}

fn run_project_flow_skeleton_clusters_raw(path: &Path) -> Result<std::process::Output> {
    let binary = PathBuf::from(env!("CARGO_BIN_EXE_analytix-analysis-compute"));
    Command::new(binary)
        .arg("project-flow-skeleton-clusters")
        .arg("--input-path")
        .arg(path)
        .output()
        .context("run project-flow-skeleton-clusters")
}

fn assert_projection_rejected_without_candidate(
    payload: &Value,
    expected_stderr: &str,
) -> Result<()> {
    let input = TempJson::new("flow-projection-invalid", payload)?;
    let output = run_project_flow_skeleton_clusters_raw(input.path())?;
    assert!(!output.status.success());
    assert!(
        output.stdout.is_empty(),
        "invalid projection emitted candidate stdout: {}",
        String::from_utf8_lossy(&output.stdout)
    );
    let stderr = String::from_utf8(output.stderr).context("decode command stderr")?;
    assert!(
        stderr.contains(expected_stderr),
        "stderr did not contain {expected_stderr:?}: {stderr}"
    );
    Ok(())
}

fn run_project_layout_node_plan(payload: &Value) -> Result<Value> {
    run_layout_node_plan_command("project-layout-node-plan", payload)
}

fn run_layout_node_plan_command(command: &str, payload: &Value) -> Result<Value> {
    let input = TempJson::new("layout-node-plan", payload)?;
    let binary = PathBuf::from(env!("CARGO_BIN_EXE_analytix-analysis-compute"));
    let output = Command::new(binary)
        .arg(command)
        .arg("--input-path")
        .arg(input.path())
        .output()
        .with_context(|| format!("run {command}"))?;
    if !output.status.success() {
        bail!(
            "{} failed: stdout={} stderr={}",
            command,
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
    }
    let stdout = String::from_utf8(output.stdout).context("decode command stdout")?;
    serde_json::from_str(stdout.trim()).context("parse command stdout JSON")
}
