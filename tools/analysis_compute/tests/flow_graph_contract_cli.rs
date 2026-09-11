use anyhow::{bail, Context, Result};
use serde_json::{json, Value};
use std::path::{Path, PathBuf};
use std::process::Command;

#[test]
fn contract_flow_graph_workflow_matches_shared_fixture() -> Result<()> {
    let fixture_path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../apps/web/tests/fixtures/flow/graph-workflow-basic.json");
    let fixture: Value = serde_json::from_slice(
        &std::fs::read(&fixture_path)
            .with_context(|| format!("read graph workflow fixture {}", fixture_path.display()))?,
    )
    .context("parse graph workflow fixture")?;

    let output = run_contract_flow_graph_workflow(&fixture_path)?;
    let layout_output = run_contract_flow_graph_layout_projection(&fixture_path)?;

    assert_eq!(output["ok"], true);
    assert_eq!(layout_output["ok"], true);
    assert_eq!(
        output["contract"]["runtimeGraph"],
        fixture["expectedRuntimeGraph"]
    );
    assert_eq!(
        output["contract"]["patchResult"]["targetSnapshotRef"],
        fixture["deltaPatch"]["result_snapshot_ref"]
    );
    assert_eq!(
        output["contract"]["edgeSemantics"],
        json!([
            {
                "id": "edge:ab",
                "relationType": "owns",
                "txCount": 2,
                "amountTotal": 2500
            },
            {
                "id": "edge:bc",
                "relationType": "pays",
                "txCount": 1,
                "amountTotal": 80
            }
        ])
    );
    assert_eq!(
        output["contract"]["searchResult"]["nodeIds"],
        fixture["searchContract"]["expectedNodeIds"]
    );
    assert_eq!(
        output["contract"]["filterResult"]["edgeIds"],
        fixture["filterContract"]["expectedEdgeIds"]
    );
    assert_eq!(
        output["contract"]["filterResult"]["nodeIds"],
        fixture["filterContract"]["expectedNodeIds"]
    );
    assert_eq!(
        output["contract"]["layoutProjection"]["nodeOrder"],
        fixture["layoutProjectionContract"]["expectedNodeOrder"]
    );
    assert_eq!(
        output["contract"]["layoutProjection"]["syncRows"],
        fixture["layoutProjectionContract"]["expectedSyncRows"]
    );
    assert_eq!(
        output["contract"]["layoutProjection"]["bounds"],
        fixture["layoutProjectionContract"]["expectedBounds"]
    );
    assert_eq!(
        output["contract"]["layoutProjection"]["signature"],
        fixture["layoutProjectionContract"]["expectedSignature"]
    );
    assert_eq!(
        layout_output["layoutProjection"],
        output["contract"]["layoutProjection"]
    );
    assert_eq!(
        output["contract"]["mismatchResult"],
        json!({
            "data": null,
            "targetSnapshotRef": fixture["deltaPatch"]["result_snapshot_ref"],
            "patchKind": "delta",
            "errorMessage": "runtime revision mismatch"
        })
    );

    Ok(())
}

#[test]
fn contract_flow_graph_layout_projection_matches_complex_fixture() -> Result<()> {
    let fixture_path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../apps/web/tests/fixtures/flow/graph-layout-projection-complex.json");
    let fixture: Value = serde_json::from_slice(
        &std::fs::read(&fixture_path)
            .with_context(|| format!("read graph layout fixture {}", fixture_path.display()))?,
    )
    .context("parse graph layout fixture")?;

    let output = run_contract_flow_graph_layout_projection(&fixture_path)?;
    let layout = &output["layoutProjection"];
    let contract = &fixture["layoutProjectionContract"];

    assert_eq!(output["ok"], true);
    assert_eq!(layout["nodeOrder"], contract["expectedNodeOrder"]);
    assert_eq!(layout["syncRows"], contract["expectedSyncRows"]);
    assert_eq!(layout["bounds"], contract["expectedBounds"]);
    assert_eq!(layout["signature"], contract["expectedSignature"]);

    Ok(())
}

#[test]
fn contract_flow_graph_data_merge_matches_complex_fixture() -> Result<()> {
    let fixture_path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../apps/web/tests/fixtures/flow/graph-data-merge-complex.json");
    let fixture: Value =
        serde_json::from_slice(&std::fs::read(&fixture_path).with_context(|| {
            format!("read graph data/merge fixture {}", fixture_path.display())
        })?)
        .context("parse graph data/merge fixture")?;

    let output = run_contract_flow_graph_data_merge(&fixture_path)?;
    let data_merge = &output["dataMerge"];

    assert_eq!(output["ok"], true);
    assert_eq!(
        data_merge["baseGraphData"],
        fixture["expectedBaseGraphData"]
    );
    assert_eq!(
        data_merge["baseRuntimeSummary"],
        fixture["expectedBaseRuntimeSummary"]
    );
    let patch_contracts = fixture["patchContracts"]
        .as_array()
        .cloned()
        .unwrap_or_default();
    for (index, contract) in patch_contracts.iter().enumerate() {
        let result = &data_merge["patchResults"][index];
        assert_eq!(result["name"], contract["name"]);
        assert_eq!(result["result"], contract["expectedResult"]);
        assert_eq!(result["runtimeSummary"], contract["expectedRuntimeSummary"]);
    }

    Ok(())
}

#[test]
fn contract_flow_graph_search_filter_matches_complex_fixture() -> Result<()> {
    let fixture_path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../apps/web/tests/fixtures/flow/graph-search-filter-complex.json");
    let fixture: Value =
        serde_json::from_slice(&std::fs::read(&fixture_path).with_context(|| {
            format!(
                "read graph search/filter fixture {}",
                fixture_path.display()
            )
        })?)
        .context("parse graph search/filter fixture")?;

    let output = run_contract_flow_graph_search_filter(&fixture_path)?;
    let search_filter = &output["searchFilter"];

    assert_eq!(output["ok"], true);
    let search_contracts = fixture["searchContracts"]
        .as_array()
        .cloned()
        .unwrap_or_default();
    for (index, contract) in search_contracts.iter().enumerate() {
        let result = &search_filter["searchResults"][index];
        assert_eq!(result["nodeIds"], contract["expectedNodeIds"]);
        assert_eq!(result["firstNodeId"], contract["expectedFirstNodeId"]);
        assert_eq!(
            result["indexSummary"],
            fixture["expectedSearchIndexSummary"]
        );
    }
    let filter_contracts = fixture["filterContracts"]
        .as_array()
        .cloned()
        .unwrap_or_default();
    for (index, contract) in filter_contracts.iter().enumerate() {
        let result = &search_filter["filterResults"][index];
        assert_eq!(result["edgeIds"], contract["expectedEdgeIds"]);
        assert_eq!(result["nodeIds"], contract["expectedNodeIds"]);
        assert_eq!(result["coreIds"], contract["expectedCoreIds"]);
        assert_eq!(result["childMap"], contract["expectedChildMap"]);
        assert_eq!(result["edgeSummaries"], contract["expectedEdgeSummaries"]);
        let hidden_node_ids = result["nodeSummaries"]
            .as_array()
            .map(|nodes| nodes.as_slice())
            .unwrap_or(&[])
            .iter()
            .filter(|node| node["projectionVisible"] == false && node["nodeRenderMode"] == "dot")
            .map(|node| node["id"].clone())
            .collect::<Vec<_>>();
        assert_eq!(
            Value::Array(hidden_node_ids),
            contract["expectedHiddenNodeIds"]
        );
    }

    Ok(())
}

#[test]
fn contract_flow_graph_filter_never_projects_unknown_amount_as_zero() -> Result<()> {
    let binary = PathBuf::from(env!("CARGO_BIN_EXE_analytix-analysis-compute"));
    let unknown_payload = json!({
        "graph": {
            "nodes": [{"id": "a"}, {"id": "b"}],
            "edges": [{"id": "unknown", "source": "a", "target": "b"}]
        },
        "filterContract": {"minAmount": 0}
    });
    let rejected = Command::new(&binary)
        .arg("contract-flow-graph-search-filter")
        .arg("--payload-json")
        .arg(unknown_payload.to_string())
        .output()
        .context("run graph filter with unknown amount")?;
    assert!(!rejected.status.success());
    assert!(rejected.stdout.is_empty());
    assert!(String::from_utf8_lossy(&rejected.stderr)
        .contains("analysis graph edge amount is unavailable"));

    let zero_payload = json!({
        "graph": {
            "nodes": [{"id": "a"}, {"id": "b"}],
            "edges": [{"id": "zero", "source": "a", "target": "b", "amount": 0}]
        },
        "filterContract": {"minAmount": 0, "maxAmount": 0}
    });
    let accepted = Command::new(binary)
        .arg("contract-flow-graph-search-filter")
        .arg("--payload-json")
        .arg(zero_payload.to_string())
        .output()
        .context("run graph filter with explicit zero amount")?;
    assert!(accepted.status.success());
    let result: Value = serde_json::from_slice(&accepted.stdout)
        .context("parse explicit-zero graph filter output")?;
    assert_eq!(
        result["searchFilter"]["filterResults"][0]["edgeIds"],
        json!(["zero"])
    );
    assert_eq!(
        result["searchFilter"]["filterResults"][0]["edgeSummaries"][0]["amountForFilter"],
        0
    );

    Ok(())
}

#[test]
fn project_flow_graph_search_matches_complex_fixture_first_result() -> Result<()> {
    let fixture_path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../apps/web/tests/fixtures/flow/graph-search-filter-complex.json");
    let fixture: Value =
        serde_json::from_slice(&std::fs::read(&fixture_path).with_context(|| {
            format!(
                "read graph search/filter fixture {}",
                fixture_path.display()
            )
        })?)
        .context("parse graph search/filter fixture")?;
    let contract = &fixture["searchContracts"][0];
    let output = run_project_flow_graph_search(&json!({
        "graph": fixture["graph"],
        "query": contract["query"],
        "limit": 1,
    }))?;

    assert_eq!(output["ok"], true);
    assert_eq!(
        output["search"]["nodeIds"],
        json!([contract["expectedFirstNodeId"].clone()])
    );
    assert_eq!(
        output["search"]["firstNodeId"],
        contract["expectedFirstNodeId"]
    );
    assert_eq!(
        output["search"]["indexSummary"],
        fixture["expectedSearchIndexSummary"]
    );

    Ok(())
}

#[test]
fn contract_flow_graph_runner_projects_all_parity_fixtures() -> Result<()> {
    let root = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../apps/web/tests/fixtures/flow");
    let workflow_path = root.join("graph-workflow-basic.json");
    let data_merge_path = root.join("graph-data-merge-complex.json");
    let layout_path = root.join("graph-layout-projection-complex.json");
    let search_filter_path = root.join("graph-search-filter-complex.json");

    let output = run_contract_flow_graph(&[
        workflow_path.as_path(),
        data_merge_path.as_path(),
        layout_path.as_path(),
        search_filter_path.as_path(),
    ])?;
    let graph_contract = &output["graphContract"];

    assert_eq!(output["ok"], true);
    assert_eq!(graph_contract["workflow"].as_array().map(Vec::len), Some(1));
    assert_eq!(
        graph_contract["dataMerge"].as_array().map(Vec::len),
        Some(1)
    );
    assert_eq!(
        graph_contract["layoutProjection"].as_array().map(Vec::len),
        Some(1)
    );
    assert_eq!(
        graph_contract["searchFilter"].as_array().map(Vec::len),
        Some(1)
    );
    assert_eq!(
        graph_contract["workflow"][0]["contract"],
        run_contract_flow_graph_workflow(&workflow_path)?["contract"]
    );
    assert_eq!(
        graph_contract["dataMerge"][0]["dataMerge"],
        run_contract_flow_graph_data_merge(&data_merge_path)?["dataMerge"]
    );
    assert_eq!(
        graph_contract["layoutProjection"][0]["layoutProjection"],
        run_contract_flow_graph_layout_projection(&layout_path)?["layoutProjection"]
    );
    assert_eq!(
        graph_contract["searchFilter"][0]["searchFilter"],
        run_contract_flow_graph_search_filter(&search_filter_path)?["searchFilter"]
    );

    Ok(())
}

fn run_contract_flow_graph_workflow(path: &Path) -> Result<Value> {
    let binary = PathBuf::from(env!("CARGO_BIN_EXE_analytix-analysis-compute"));
    let output = Command::new(binary)
        .arg("contract-flow-graph-workflow")
        .arg("--input-path")
        .arg(path)
        .output()
        .context("run contract-flow-graph-workflow")?;
    if !output.status.success() {
        bail!(
            "contract-flow-graph-workflow failed: stdout={} stderr={}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
    }
    let stdout = String::from_utf8(output.stdout).context("decode command stdout")?;
    serde_json::from_str(stdout.trim()).context("parse command stdout JSON")
}

fn run_contract_flow_graph(paths: &[&Path]) -> Result<Value> {
    let binary = PathBuf::from(env!("CARGO_BIN_EXE_analytix-analysis-compute"));
    let mut command = Command::new(binary);
    command.arg("contract-flow-graph");
    for path in paths {
        command.arg("--input-path").arg(path);
    }
    let output = command.output().context("run contract-flow-graph")?;
    if !output.status.success() {
        bail!(
            "contract-flow-graph failed: stdout={} stderr={}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
    }
    let stdout = String::from_utf8(output.stdout).context("decode command stdout")?;
    serde_json::from_str(stdout.trim()).context("parse command stdout JSON")
}

fn run_contract_flow_graph_data_merge(path: &Path) -> Result<Value> {
    let binary = PathBuf::from(env!("CARGO_BIN_EXE_analytix-analysis-compute"));
    let output = Command::new(binary)
        .arg("contract-flow-graph-data-merge")
        .arg("--input-path")
        .arg(path)
        .output()
        .context("run contract-flow-graph-data-merge")?;
    if !output.status.success() {
        bail!(
            "contract-flow-graph-data-merge failed: stdout={} stderr={}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
    }
    let stdout = String::from_utf8(output.stdout).context("decode command stdout")?;
    serde_json::from_str(stdout.trim()).context("parse command stdout JSON")
}

fn run_contract_flow_graph_search_filter(path: &Path) -> Result<Value> {
    let binary = PathBuf::from(env!("CARGO_BIN_EXE_analytix-analysis-compute"));
    let output = Command::new(binary)
        .arg("contract-flow-graph-search-filter")
        .arg("--input-path")
        .arg(path)
        .output()
        .context("run contract-flow-graph-search-filter")?;
    if !output.status.success() {
        bail!(
            "contract-flow-graph-search-filter failed: stdout={} stderr={}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
    }
    let stdout = String::from_utf8(output.stdout).context("decode command stdout")?;
    serde_json::from_str(stdout.trim()).context("parse command stdout JSON")
}

fn run_project_flow_graph_search(payload: &Value) -> Result<Value> {
    let binary = PathBuf::from(env!("CARGO_BIN_EXE_analytix-analysis-compute"));
    let output = Command::new(binary)
        .arg("project-flow-graph-search")
        .arg("--payload-json")
        .arg(payload.to_string())
        .output()
        .context("run project-flow-graph-search")?;
    if !output.status.success() {
        bail!(
            "project-flow-graph-search failed: stdout={} stderr={}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
    }
    let stdout = String::from_utf8(output.stdout).context("decode command stdout")?;
    serde_json::from_str(stdout.trim()).context("parse command stdout JSON")
}

fn run_contract_flow_graph_layout_projection(path: &Path) -> Result<Value> {
    let binary = PathBuf::from(env!("CARGO_BIN_EXE_analytix-analysis-compute"));
    let output = Command::new(binary)
        .arg("contract-flow-graph-layout-projection")
        .arg("--input-path")
        .arg(path)
        .output()
        .context("run contract-flow-graph-layout-projection")?;
    if !output.status.success() {
        bail!(
            "contract-flow-graph-layout-projection failed: stdout={} stderr={}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
    }
    let stdout = String::from_utf8(output.stdout).context("decode command stdout")?;
    serde_json::from_str(stdout.trim()).context("parse command stdout JSON")
}
