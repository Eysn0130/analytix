use super::*;
use serde_json::{json, Value};

#[test]
fn render_plan_matches_node_ranking_and_modes() {
    let projected = project_graph_render_plan(&json!({
        "renderPlans": [{
            "name": "large-mixed",
            "nodes": [
                {"id": "forced", "ntype": "seed"},
                {"id": "ranked", "amount": 500},
                {"id": "dotty", "amount": 1}
            ],
            "edges": [{"source": "forced", "target": "ranked"}],
            "ctx": {"graphTier": "large", "selected": ["forced"]},
            "hints": {"tier": "large", "default_node_mode": "mixed", "entity_node_limit": 1, "focus_entity_limit": 0}
        }]
    }));
    let result = &projected["renderResults"][0];
    assert_eq!(
        result["summary"],
        json!({"tier": "large", "entityCount": 1, "dotCount": 2, "focusCount": 1})
    );
    assert_eq!(
        result["nodeRenderModes"],
        json!([
            {"id": "forced", "nodeRenderMode": "focus"},
            {"id": "ranked", "nodeRenderMode": "dot"},
            {"id": "dotty", "nodeRenderMode": "dot"}
        ])
    );
    assert_eq!(
        result["renderPlan"]["nodeRenderUpdates"],
        json!([
            {
                "index": 0,
                "id": "forced",
                "r": 18.0,
                "lineWidth": 3.0,
                "nodeShadow": false,
                "icon": "",
                "iconSymbol": "",
                "iconSize": 0.0,
                "nodeLabelHidden": false,
                "nodeRenderMode": "focus"
            },
            {
                "index": 1,
                "id": "ranked",
                "r": 6.12,
                "lineWidth": 1.5,
                "nodeShadow": false,
                "icon": "",
                "iconSymbol": "",
                "iconSize": 0.0,
                "nodeLabelHidden": true,
                "nodeRenderMode": "dot",
                "__tierBaseR": 18.0,
                "__tierBaseLineWidth": 2.0,
                "__tierBaseNodeShadow": false,
                "__tierBaseIcon": "",
                "__tierBaseIconSymbol": "",
                "__tierBaseIconSize": 16.0,
                "__tierBaseNodeLabelHidden": false,
                "__tierLabelSuppressed": true
            },
            {
                "index": 2,
                "id": "dotty",
                "r": 6.12,
                "lineWidth": 1.5,
                "nodeShadow": false,
                "icon": "",
                "iconSymbol": "",
                "iconSize": 0.0,
                "nodeLabelHidden": true,
                "nodeRenderMode": "dot",
                "__tierBaseR": 18.0,
                "__tierBaseLineWidth": 2.0,
                "__tierBaseNodeShadow": false,
                "__tierBaseIcon": "",
                "__tierBaseIconSymbol": "",
                "__tierBaseIconSize": 16.0,
                "__tierBaseNodeLabelHidden": false,
                "__tierLabelSuppressed": true
            }
        ])
    );
    assert_eq!(result["ranking"][0]["id"], "forced");
    assert_eq!(result["ranking"][0]["keepRank"], 6);
    assert_eq!(result["normalizedRenderHints"]["tier"], json!("large"));
    assert_eq!(
        result["renderPlan"]["normalizedRenderHints"]["tier"],
        json!("large")
    );
    assert_eq!(
        result["normalizedRenderHints"]["edge_mode"],
        json!("batched")
    );
}

#[test]
fn render_plan_normalizes_render_hints_contract() {
    let hints = render_hints::normalize_graph_render_hints(
        &json!({
            "graph_tier": "xlarge",
            "default_node_mode": "MIXED",
            "show_edge_labels": false,
            "show_detail_edge_labels": true,
            "edge_batch_size": 12,
            "t1_batch_gap_ms": 2,
            "projection_auto_expand_min_zoom": 9,
            "projection_auto_expand_max_clusters": 0,
            "projection_viewport_materialize_limit": 10
        }),
        150_001,
        9_000,
        "Network",
    );

    assert_eq!(hints["tier"], json!("xlarge"));
    assert_eq!(hints["view_mode"], json!("network"));
    assert_eq!(hints["default_node_mode"], json!("mixed"));
    assert_eq!(hints["show_edge_labels"], json!(false));
    assert_eq!(hints["show_detail_edge_labels"], json!(true));
    assert_eq!(hints["edge_mode"], json!("batched"));
    assert_eq!(hints["edge_batch_size"].as_f64(), Some(48.0));
    assert_eq!(hints["t1_batch_gap_ms"].as_f64(), Some(8.0));
    assert_eq!(hints["projection_auto_expand_min_zoom"].as_f64(), Some(4.0));
    assert_eq!(
        hints["projection_auto_expand_max_clusters"].as_f64(),
        Some(1.0)
    );
    assert_eq!(
        hints["projection_viewport_materialize_limit"].as_f64(),
        Some(500.0)
    );

    let tier_only = render_hints::normalize_render_plan_hints(
        &json!({"hints": "large"}),
        &Value::Null,
        &json!("large"),
        3,
        0,
    );
    assert_eq!(tier_only["tier"], json!("large"));

    let ctx_fallback = render_hints::normalize_render_plan_hints(
        &Value::Null,
        &json!({"renderHints": {"tier": "medium", "view_mode": "flow"}}),
        &json!({}),
        3,
        0,
    );
    assert_eq!(ctx_fallback["tier"], json!("medium"));
    assert_eq!(ctx_fallback["view_mode"], json!("flow"));
}

#[test]
fn render_plan_preserves_projection_modes_except_focus() {
    let projected = project_graph_render_plan(&json!({
        "renderPlan": {
            "nodes": [
                {"id": "cluster", "cluster_node": true, "nodeRenderMode": "dot"},
                {"id": "focus", "nodeRenderMode": "dot"}
            ],
            "ctx": {"graphTier": "large", "graphProjection": {"mode": "skeleton"}, "selected": ["focus"]},
            "hints": {"tier": "large", "default_node_mode": "mixed", "entity_node_limit": 0, "focus_entity_limit": 0}
        }
    }));
    assert_eq!(
        projected["renderResults"][0]["nodeRenderModes"],
        json!([
            {"id": "cluster", "nodeRenderMode": "dot"},
            {"id": "focus", "nodeRenderMode": "focus"}
        ])
    );
}

#[test]
fn render_plan_projects_edge_label_updates() {
    let projected = project_graph_render_plan(&json!({
        "renderPlans": [
            {
                "name": "hide-labels",
                "nodes": [],
                "edges": [{"id": "e1", "label": "总额", "labelTop": "出", "labelBottom": "入", "detailLabel": true}],
                "hints": {"show_edge_labels": false}
            },
            {
                "name": "restore-labels",
                "nodes": [],
                "edges": [{
                    "id": "e2",
                    "label": "",
                    "labelTop": "",
                    "labelBottom": "",
                    "detailLabel": false,
                    "__tierLabelSuppressed": true,
                    "__tierBaseLabel": "总额",
                    "__tierBaseLabelTop": "出",
                    "__tierBaseLabelBottom": "入",
                    "__tierBaseDetailLabel": true
                }],
                "hints": {"show_edge_labels": true}
            },
            {
                "name": "preserve-labels",
                "preserveEdgeLabels": true,
                "nodes": [],
                "edges": [{
                    "id": "e3",
                    "label": "",
                    "labelTop": "",
                    "labelBottom": "",
                    "detailLabel": false,
                    "__tierLabelSuppressed": true,
                    "__tierBaseLabel": "总额",
                    "__tierBaseLabelTop": "出",
                    "__tierBaseLabelBottom": "入",
                    "__tierBaseDetailLabel": true
                }],
                "hints": {"show_edge_labels": true}
            }
        ]
    }));

    assert_eq!(
        projected["renderResults"][0]["edgeRenderUpdates"],
        json!([{
            "id": "e1",
            "label": "",
            "labelTop": "",
            "labelBottom": "",
            "detailLabel": false,
            "edgeArrow": "end",
            "showArrow": true,
            "edgeLabelHidden": true,
            "__tierBaseLabel": "总额",
            "__tierBaseLabelTop": "出",
            "__tierBaseLabelBottom": "入",
            "__tierBaseDetailLabel": true,
            "__tierLabelSuppressed": true,
        }])
    );
    assert_eq!(
        projected["renderResults"][1]["edgeRenderUpdates"],
        json!([{
            "id": "e2",
            "label": "总额",
            "labelTop": "出",
            "labelBottom": "入",
            "detailLabel": true,
            "edgeArrow": "end",
            "showArrow": true,
            "edgeLabelHidden": false,
            "__tierBaseLabel": "总额",
            "__tierBaseLabelTop": "出",
            "__tierBaseLabelBottom": "入",
            "__tierBaseDetailLabel": true,
            "__tierLabelSuppressed": false,
        }])
    );
    assert_eq!(
        projected["renderResults"][2]["edgeRenderUpdates"],
        json!([{
            "id": "e3",
            "label": "",
            "labelTop": "",
            "labelBottom": "",
            "detailLabel": false,
            "edgeArrow": "end",
            "showArrow": true,
            "edgeLabelHidden": false,
            "__tierBaseLabel": "总额",
            "__tierBaseLabelTop": "出",
            "__tierBaseLabelBottom": "入",
            "__tierBaseDetailLabel": true,
            "__tierLabelSuppressed": true,
        }])
    );
}

#[test]
fn render_plan_projects_relation_edge_display() {
    let projected = project_graph_render_plan(&json!({
        "renderPlan": {
            "name": "relation-edge-display",
            "nodes": [],
            "edges": [
                {
                    "id": "double",
                    "source": "a",
                    "target": "b",
                    "forward_amount": 120,
                    "reverse_amount": 40,
                    "label": "总额",
                    "edgeArrow": "end",
                    "showArrow": true
                },
                {
                    "id": "reverse",
                    "source": "b",
                    "target": "a",
                    "amount": 88.5,
                    "edgeArrow": "start"
                },
                {
                    "id": "labels",
                    "source": "a",
                    "target": "c",
                    "labelTop": "出 ￥1,234.50",
                    "labelBottom": "入 ￥50"
                },
                {
                    "id": "plain",
                    "source": "c",
                    "target": "d",
                    "edgeArrow": "none",
                    "label": "普通"
                }
            ],
            "hints": {"view_mode": "relation", "show_edge_labels": true}
        }
    }));

    assert_eq!(
        projected["renderResults"][0]["edgeRenderUpdates"],
        json!([
            {
                "id": "double",
                "label": "总额",
                "labelTop": "￥120",
                "labelBottom": "￥40",
                "detailLabel": false,
                "mode": "double",
                "edgeArrow": "both",
                "showArrow": true,
                "edgeLabelHidden": false,
                "__tierBaseLabel": "总额",
                "__tierBaseLabelTop": "￥120",
                "__tierBaseLabelBottom": "￥40",
                "__tierBaseDetailLabel": false,
            },
            {
                "id": "reverse",
                "label": "￥88.5",
                "labelTop": "",
                "labelBottom": "",
                "detailLabel": false,
                "mode": "single",
                "edgeArrow": "start",
                "showArrow": true,
                "edgeLabelHidden": false,
                "__tierBaseLabel": "￥88.5",
                "__tierBaseLabelTop": "",
                "__tierBaseLabelBottom": "",
                "__tierBaseDetailLabel": false,
            },
            {
                "id": "labels",
                "label": "",
                "labelTop": "￥1,234.5",
                "labelBottom": "￥50",
                "detailLabel": false,
                "mode": "double",
                "edgeArrow": "both",
                "showArrow": true,
                "edgeLabelHidden": false,
                "__tierBaseLabel": "",
                "__tierBaseLabelTop": "￥1,234.5",
                "__tierBaseLabelBottom": "￥50",
                "__tierBaseDetailLabel": false,
            },
            {
                "id": "plain",
                "label": "普通",
                "labelTop": "",
                "labelBottom": "",
                "detailLabel": false,
                "edgeArrow": "none",
                "showArrow": false,
                "edgeLabelHidden": false,
                "__tierBaseLabel": "普通",
                "__tierBaseLabelTop": "",
                "__tierBaseLabelBottom": "",
                "__tierBaseDetailLabel": false,
            }
        ])
    );
}

#[test]
fn render_plan_projects_staged_label_plan_when_requested() {
    let projected = project_graph_render_plan(&json!({
        "renderPlan": {
            "name": "staged",
            "stagedLabel": {"enabled": true},
            "stagedFocusIds": ["focus"],
            "nodes": [
                {"id": "focus", "name": "核心", "displayId": "F", "amount": 10},
                {"id": "plain-a", "name": "甲", "displayId": "A", "amount": 100},
                {"id": "plain-b", "name": "乙", "displayId": "B", "amount": 50},
                {"id": "plain-c", "title": "丙", "amount": 1}
            ],
            "edges": [
                {"id": "e1", "source": "focus", "target": "plain-a", "label": "总额", "labelTop": "出", "labelBottom": "入", "detailLabel": true},
                {"id": "e2", "source": "focus", "target": "plain-b", "label": "往来"},
                {"id": "e3", "source": "plain-a", "target": "plain-c"}
            ],
            "ctx": {"graphTier": "large"},
            "hints": {
                "tier": "large",
                "default_node_mode": "mixed",
                "entity_node_limit": 4,
                "focus_entity_limit": 0,
                "t0_label_limit": 2,
                "t1_reveal_batch_size": 32,
                "show_edge_labels": true
            }
        }
    }));
    let plan = &projected["renderResults"][0]["stagedLabelPlan"];

    assert_eq!(plan["enabled"], true);
    assert_eq!(plan["nodeCount"], 4);
    assert_eq!(plan["edgeCount"], 3);
    assert_eq!(plan["t0Count"], 2);
    assert_eq!(plan["hiddenNodeCount"], 2);
    assert_eq!(plan["suppressedSubtitleCount"], 1);
    assert_eq!(plan["hiddenEdgeLabelCount"], 2);
    assert_eq!(plan["deferredNodeCount"], 3);
    assert_eq!(plan["deferredEdgeCount"], 2);
    assert_eq!(plan["budget"]["t0Limit"], 2);
    assert_eq!(plan["budget"]["revealBatchSize"], 32);
    assert_eq!(
        plan["nodeLabelUpdates"],
        json!([
            {"index": 1, "id": "plain-a", "nodeLabelHidden": false, "displayId": "", "display_id": ""},
            {"index": 2, "id": "plain-b", "nodeLabelHidden": true},
            {"index": 3, "id": "plain-c", "nodeLabelHidden": true}
        ])
    );
    assert_eq!(
        plan["edgeLabelUpdates"],
        json!([
            {"index": 0, "id": "e1", "label": "", "labelTop": "", "labelBottom": "", "detailLabel": false},
            {"index": 1, "id": "e2", "label": "", "labelTop": "", "labelBottom": "", "detailLabel": false}
        ])
    );
}

#[test]
fn render_plan_reuses_graph_style_for_node_updates() {
    let projected = project_graph_render_plan(&json!({
        "renderPlan": {
            "nodes": [
                {"id": "focus", "nodeRenderMode": "entity"},
                {"id": "dot", "nodeRenderMode": "entity"}
            ],
            "edges": [],
            "ctx": {"graphTier": "large", "selected": ["focus"]},
            "graphStyle": {"nodeSize": 24, "nodeWidth": 4, "iconSize": 22, "nodeShadow": true},
            "hints": {"tier": "large", "default_node_mode": "mixed", "entity_node_limit": 0, "focus_entity_limit": 0}
        }
    }));

    assert_eq!(
        projected["renderResults"][0]["nodeRenderModes"],
        json!([
            {"id": "focus", "nodeRenderMode": "focus"},
            {"id": "dot", "nodeRenderMode": "dot"}
        ])
    );
    assert_eq!(
        projected["renderResults"][0]["renderPlan"]["nodeRenderUpdates"],
        json!([
            {
                "index": 0,
                "id": "focus",
                "r": 24.0,
                "lineWidth": 5.0,
                "nodeShadow": true,
                "icon": "",
                "iconSymbol": "",
                "iconSize": 0.0,
                "nodeLabelHidden": false,
                "nodeRenderMode": "focus"
            },
            {
                "index": 1,
                "id": "dot",
                "r": 7.2,
                "lineWidth": 1.5,
                "nodeShadow": false,
                "icon": "",
                "iconSymbol": "",
                "iconSize": 0.0,
                "nodeLabelHidden": true,
                "nodeRenderMode": "dot",
                "__tierBaseR": 24.0,
                "__tierBaseLineWidth": 4.0,
                "__tierBaseNodeShadow": false,
                "__tierBaseIcon": "",
                "__tierBaseIconSymbol": "",
                "__tierBaseIconSize": 22.0,
                "__tierBaseNodeLabelHidden": false,
                "__tierLabelSuppressed": true
            }
        ])
    );
}

#[test]
fn render_plan_projects_multi_edge_offsets() {
    let projected = project_graph_render_plan(&json!({
        "renderPlan": {
            "nodes": [],
            "edges": [
                {"id": "边-乙", "source": "账户A", "target": "账户B"},
                {"id": "loop", "source": "账户A", "target": "账户A", "edgeOffset": 99},
                {"id": "边-甲", "source": "账户B", "target": "账户A"},
                {"id": "single", "source": "账户C", "target": "账户D", "edgeOffset": 88},
                {"id": "", "source": "", "target": "账户D", "edgeOffset": 77}
            ]
        }
    }));

    assert_eq!(
        projected["renderResults"][0]["edgeOffsetUpdates"],
        json!([
            {"index": 0, "id": "边-乙", "edgeOffset": 6.0},
            {"index": 1, "id": "loop", "edgeOffset": 0.0},
            {"index": 2, "id": "边-甲", "edgeOffset": -6.0},
            {"index": 3, "id": "single", "edgeOffset": 0.0}
        ])
    );
}

#[test]
fn render_plan_projects_viewport_expand_targets() {
    let projected = project_graph_render_plan(&json!({
        "viewportExpandTargets": {
            "entries": [
                {"id": "__cluster__::far", "cluster_node": true, "x": 280, "y": 120, "total_amount": 999, "total_count": 4},
                {"id": "__cluster__::a", "cluster_node": true, "x": 105, "y": 90, "total_amount": 10, "total_count": 1},
                {"id": "__cluster__::b", "cluster_node": true, "x": 110, "y": 90, "total_amount": 900, "total_count": 9},
                {"id": "node-2", "nodeRenderMode": "dot", "x": 120, "y": 92, "total_amount": 20, "total_count": 7},
                {"id": "node-1", "nodeRenderMode": "dot", "canvasX": 104, "canvasY": 88, "x": 999, "y": 999, "total_amount": 10, "total_count": 9},
                {"id": "outside", "nodeRenderMode": "dot", "x": 999, "y": 999}
            ],
            "width": 200,
            "height": 180,
            "padding": 10,
            "maxClusters": 2,
            "maxTiles": 2,
            "maxNodes": 2,
            "tilesPerCluster": 2,
            "projection": {
                "clusters": [
                    {"cluster_id": "__cluster__::a", "next_tile_ids": ["tile-1", "tile-2", "tile-1", ""]},
                    {"clusterId": "__cluster__::b", "remainingTileIds": "tile-b"}
                ]
            }
        }
    }));

    assert_eq!(
        projected["viewportExpandTargets"],
        json!({
            "clusterIds": [],
            "tileIds": ["tile-1", "tile-2"],
            "nodeIds": ["node-1", "node-2"]
        })
    );
}

#[test]
fn render_plan_projects_viewport_cluster_targets_without_tiles() {
    let projected = project_graph_render_plan(&json!({
        "viewport_expand_targets": {
            "entries": [
                {"id": "__cluster__::b", "cluster_node": true, "x": 110, "y": 90, "total_amount": 900, "total_count": 9},
                {"id": "__cluster__::a", "cluster_node": true, "x": 105, "y": 90, "total_amount": 10, "total_count": 1}
            ],
            "width": 200,
            "height": 180,
            "maxClusters": 2,
            "preferTiles": false,
            "projection": {
                "clusters": [
                    {"cluster_id": "__cluster__::a", "next_tile_ids": ["tile-1", "tile-2"]},
                    {"clusterId": "__cluster__::b", "remainingTileIds": "tile-b"}
                ]
            }
        }
    }));

    assert_eq!(
        projected["viewportExpandTargets"],
        json!({
            "clusterIds": ["__cluster__::a", "__cluster__::b"],
            "tileIds": [],
            "nodeIds": []
        })
    );
}

#[test]
fn render_plan_projects_viewport_dot_cluster_targets() {
    let projected = project_graph_render_plan(&json!({
        "viewportExpandTargets": {
            "entries": [
                {"id": "dot-a", "nodeRenderMode": "dot", "projection_cluster_id": "__cluster__::a", "canvasX": 45, "canvasY": 50, "total_amount": 10, "total_count": 1},
                {"id": "dot-b", "nodeRenderMode": "dot", "projection_cluster_id": "__cluster__::b", "canvasX": 55, "canvasY": 50, "total_amount": 900, "total_count": 9},
                {"id": "dot-a-stronger", "nodeRenderMode": "dot", "projection_cluster_id": "__cluster__::a", "canvasX": 80, "canvasY": 50, "total_amount": 1000, "total_count": 1},
                {"id": "solo", "nodeRenderMode": "dot", "canvasX": 50, "canvasY": 50, "total_amount": 5, "total_count": 1},
                {"id": "wide", "nodeRenderMode": "dot", "canvasX": 112, "canvasY": 50},
                {"id": "tall", "nodeRenderMode": "dot", "canvasX": 50, "canvasY": 111}
            ],
            "width": 100,
            "height": 100,
            "paddingX": 12,
            "paddingY": 10,
            "maxClusters": 2,
            "maxNodes": 3,
            "preferTiles": false,
            "projectDotClusterTargets": true
        }
    }));

    assert_eq!(
        projected["viewportExpandTargets"],
        json!({
            "clusterIds": ["__cluster__::b", "__cluster__::a"],
            "tileIds": [],
            "nodeIds": ["solo", "wide"]
        })
    );
}

#[test]
fn render_plan_projects_runtime_graph_viewport_expand_targets() {
    let projected = project_graph_render_plan(&json!({
        "viewportExpandTargets": {
            "runtimeGraph": {
                "nodes": [
                    {"id": "__cluster__::a", "cluster_node": true, "x": 10, "y": 20, "total_amount": 10, "total_count": 1},
                    {"id": "dot-a", "nodeRenderMode": "dot", "projection_cluster_id": "__cluster__::a", "x": 12, "y": 20, "total_amount": 900, "total_count": 9},
                    {"id": "solo", "nodeRenderMode": "dot", "x": 9, "y": 19, "total_amount": 5, "total_count": 4},
                    {"id": "entity", "nodeRenderMode": "entity", "x": 10, "y": 20},
                    {"id": "outside", "nodeRenderMode": "dot", "x": 1000, "y": 20},
                    {"id": "bad", "nodeRenderMode": "dot", "x": "NaN", "y": 20}
                ]
            },
            "viewport": {
                "zoom": 2,
                "center": {"x": 10, "y": 20},
                "size": {"width": 100, "height": 80}
            },
            "paddingX": 4,
            "paddingY": 4,
            "maxClusters": 2,
            "maxNodes": 2,
            "preferTiles": false,
            "projectDotClusterTargets": true
        }
    }));

    assert_eq!(
        projected["viewportExpandTargets"],
        json!({
            "clusterIds": ["__cluster__::a"],
            "tileIds": [],
            "nodeIds": ["solo"]
        })
    );
}
