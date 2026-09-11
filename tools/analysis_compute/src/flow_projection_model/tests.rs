use super::*;

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

#[test]
fn projection_model_rejects_missing_node_and_edge_facts_without_partial_projection() {
    let mut missing_node_amount = fixture_payload();
    missing_node_amount["nodes"][1]
        .as_object_mut()
        .unwrap()
        .remove("total_amount");
    assert!(project_projection_model(&missing_node_amount)
        .unwrap_err()
        .to_string()
        .contains("nodes[1].total_amount"));

    let mut missing_node_count = fixture_payload();
    missing_node_count["nodes"][2]
        .as_object_mut()
        .unwrap()
        .remove("total_count");
    assert!(project_projection_model(&missing_node_count)
        .unwrap_err()
        .to_string()
        .contains("nodes[2].total_count"));

    let mut mixed_valid_and_missing = fixture_payload();
    mixed_valid_and_missing["edges"][5]
        .as_object_mut()
        .unwrap()
        .remove("amount");
    assert!(project_projection_model(&mixed_valid_and_missing)
        .unwrap_err()
        .to_string()
        .contains("edges[5].amount"));
}

#[test]
fn projection_model_rejects_invalid_direction_and_endpoint_coverage() {
    let mut nonfinite_direction = fixture_payload();
    nonfinite_direction["edges"][0]["forward_amount"] = json!("NaN");
    assert!(project_projection_model(&nonfinite_direction)
        .unwrap_err()
        .to_string()
        .contains("edges[0].forward_amount"));

    let mut mismatched_direction = fixture_payload();
    mismatched_direction["edges"][0]["amount"] = json!(1.0);
    assert!(project_projection_model(&mismatched_direction)
        .unwrap_err()
        .to_string()
        .contains("direction coverage mismatch"));

    let mut unknown_target = fixture_payload();
    unknown_target["edges"][0]["target"] = json!("missing-node");
    assert!(project_projection_model(&unknown_target)
        .unwrap_err()
        .to_string()
        .contains("references unknown node"));
}

#[test]
fn projection_model_preserves_real_zero_and_optional_times() {
    let result = project_projection_model(&fixture_payload()).unwrap();
    let edges = result["projected_edge_plan"]["edges"].as_array().unwrap();
    assert!(!edges.is_empty());
    assert!(edges.iter().all(|edge| edge["amount"] == json!(0.0)));
    assert!(edges.iter().all(|edge| edge["count"] == json!(0)));
    assert!(edges.iter().all(|edge| edge["first_time"].is_null()));
    assert!(edges.iter().all(|edge| edge["last_time"].is_null()));
}

#[test]
fn projection_model_matches_anchor_cluster_tile_golden() {
    let result = project_projection_model(&fixture_payload()).unwrap();
    assert_eq!(
        result.get("anchor_ids").unwrap(),
        &json!(["focus", "seed", "leaf-10", "leaf-09", "solo-a"])
    );
    assert_eq!(
        result.get("search_match_node_ids").unwrap(),
        &json!(["leaf-08"])
    );
    assert_eq!(result.get("base_projection_node_ids").unwrap(), &json!([]));
    assert_eq!(result.get("path_entity_ids").unwrap(), &json!([]));
    assert_eq!(
        result.get("requested_cluster_materializations").unwrap(),
        &json!([])
    );
    assert_eq!(
        result.get("explicit_entity_ids").unwrap(),
        &json!([
            "focus", "leaf-01", "leaf-02", "leaf-03", "leaf-04", "leaf-05", "leaf-06", "leaf-07",
            "leaf-08", "leaf-09", "leaf-10", "seed", "solo-a", "solo-b"
        ])
    );
    let clusters = result.get("clusters").unwrap().as_array().unwrap();
    assert_eq!(clusters.len(), 1);
    let cluster = &clusters[0];
    assert_eq!(
        cluster,
        &json!({
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
        })
    );
}

#[test]
fn projection_model_rejects_stale_base_projection_coverage() {
    let mut payload = fixture_payload();
    payload.as_object_mut().unwrap().insert(
        "base_projection".to_string(),
        json!({
            "anchor_node_ids": ["seed", "missing"],
            "clusters": [
                {
                    "cluster_id": "existing-cluster",
                    "anchor_id": "seed",
                    "member_ids": ["leaf-03", "missing", "leaf-04"]
                }
            ]
        }),
    );
    assert!(project_projection_model(&payload)
        .unwrap_err()
        .to_string()
        .contains("base_projection.anchor_node_ids[1] references unknown node"));

    let mut stale_expanded = fixture_payload();
    stale_expanded["base_projection"] = json!({
        "anchor_node_ids": ["seed"],
        "expanded_node_ids": ["missing"],
        "clusters": []
    });
    assert!(project_projection_model(&stale_expanded)
        .unwrap_err()
        .to_string()
        .contains("base_projection.expanded_node_ids[0] references unknown node"));
}

#[test]
fn projection_model_materializes_base_path_search_and_request_ids() {
    let mut payload = fixture_payload();
    payload.as_object_mut().unwrap().insert(
        "request_context".to_string(),
        json!({
            "node_ids": ["solo-a"],
            "path_node_ids": ["leaf-01", "leaf-02"],
            "search_query": "alias-03",
            "search_limit": 8,
            "include_neighbors": false,
            "materialize_limit": 20,
        }),
    );
    payload.as_object_mut().unwrap().insert(
        "base_projection".to_string(),
        json!({
            "anchor_node_ids": ["seed"],
            "expanded_node_ids": ["leaf-04"],
            "clusters": [
                {
                    "cluster_id": "existing-cluster",
                    "anchor_id": "seed",
                    "member_ids": ["leaf-01", "leaf-02", "leaf-03", "leaf-04"]
                }
            ]
        }),
    );

    let result = project_projection_model(&payload).unwrap();

    assert_eq!(result.get("anchor_ids").unwrap(), &json!(["seed"]));
    assert_eq!(
        result.get("search_match_node_ids").unwrap(),
        &json!(["leaf-03"])
    );
    assert_eq!(
        result.get("base_projection_node_ids").unwrap(),
        &json!(["leaf-04"])
    );
    assert_eq!(
        result.get("path_entity_ids").unwrap(),
        &json!(["leaf-01", "leaf-02", "seed"])
    );
    assert_eq!(
        result.get("explicit_entity_ids").unwrap(),
        &json!(["leaf-01", "leaf-02", "leaf-03", "leaf-04", "seed", "solo-a"])
    );
    assert_eq!(
        result.get("requested_cluster_materializations").unwrap(),
        &json!([])
    );
}

#[test]
fn projection_model_plans_requested_cluster_materialization() {
    let mut payload = fixture_payload();
    payload.as_object_mut().unwrap().insert(
        "request_context".to_string(),
        json!({
            "cluster_ids": ["existing-cluster"],
            "include_neighbors": false,
            "materialize_limit": 3,
        }),
    );
    payload.as_object_mut().unwrap().insert(
        "base_projection".to_string(),
        json!({
            "anchor_node_ids": ["seed"],
            "clusters": [
                {
                    "cluster_id": "existing-cluster",
                    "anchor_id": "seed",
                    "member_ids": ["leaf-01", "leaf-02", "leaf-03", "leaf-04"]
                }
            ]
        }),
    );

    let result = project_projection_model(&payload).unwrap();

    assert_eq!(result.get("explicit_entity_ids").unwrap(), &json!(["seed"]));
    assert_eq!(
        result.get("requested_cluster_materializations").unwrap(),
        &json!([
            {
                "cluster_id": "existing-cluster",
                "materialized_member_ids": ["leaf-04", "leaf-03", "leaf-02"],
            }
        ])
    );
}
