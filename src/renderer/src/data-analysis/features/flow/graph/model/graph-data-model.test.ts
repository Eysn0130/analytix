import { describe, expect, it } from "vitest";
import type { FlowGraphData } from "../../../../services/analysis/flow-api-contracts";
import browserBridgeMapperScript from "../../runtime/embedded/analysis_flow/browser-bridge-mapper.js?raw";
import embeddedGraphDataModelScript from "../../runtime/embedded/analysis_flow/graph-data-model.js?raw";
import embeddedFlowAppScript from "../../runtime/embedded/analysis_flow/app.js?raw";
import flowViewSyncStoreScript from "../../runtime/embedded/analysis_flow/flow-view-sync-store.js?raw";
import {
  asFlowSnapshotRef,
  asRuntimeRevision,
  cloneFlowGraphData,
  cloneRuntimeGraph,
} from "./graph-data-model";

type EmbeddedFlowWindow = {
  __ANALYTIX_FLOW_BRIDGE_MAPPER__?: {
    sanitizeSnapshotRef(value: unknown): unknown;
    mapBackendViewToRuntimeView(value: unknown): unknown;
  };
  AnalytixGraphDataModel?: {
    cloneResultSnapshotRef(value: unknown): unknown;
  };
};

function runEmbeddedScript(script: string, targetWindow: EmbeddedFlowWindow): void {
  Function("window", script)(targetWindow);
}

describe("Flow graph data cloning", () => {
  it("snapshot_ref_missing_counts_is_invalid_not_zero", () => {
    expect(
      asFlowSnapshotRef({
        snapshot_id: "snapshot-a",
        graph_hash: "hash-a",
        stored_at: "2026-07-21T00:00:00Z",
      })
    ).toBeNull();
    expect(
      asFlowSnapshotRef({
        snapshot_id: "snapshot-a",
        graph_hash: "hash-a",
        node_count: null,
        edge_count: null,
        stored_at: "2026-07-21T00:00:00Z",
      })
    ).toBeNull();
  });

  it("snapshot_ref_explicit_zero_counts_remain_valid", () => {
    expect(
      asFlowSnapshotRef({
        snapshot_id: "snapshot-a",
        graph_hash: "hash-a",
        node_count: 0,
        edge_count: 0,
        stored_at: "2026-07-21T00:00:00Z",
      })
    ).toEqual({
      snapshot_id: "snapshot-a",
      graph_hash: "hash-a",
      node_count: 0,
      edge_count: 0,
      stored_at: "2026-07-21T00:00:00Z",
    });
  });

  it("runtime_graph_null_revision_remains_unknown", () => {
    expect(asRuntimeRevision(null)).toBeNull();
    expect(asRuntimeRevision("0")).toBeNull();
    expect(asRuntimeRevision(0)).toBe(0);
    expect(cloneRuntimeGraph({ nodes: [], edges: [], runtime_revision: null })).not.toHaveProperty("runtime_revision");
  });

  it("preserves an opaque public reference without inventing graph counts", () => {
    const opaqueRef = `flowref_v1_${"a".repeat(64)}`;
    const source: FlowGraphData = {
      contract: "FlowPublicResultBoundaryV1",
      publication_status: "blocked",
      fact_answer_allowed: false,
      content_access: "controlled_artifact_required",
      nodes: [],
      edges: [],
      stats: {},
      runtime_graph: { nodes: [], edges: [] },
      result_snapshot_ref: {
        contract: "FlowPublicOpaqueSnapshotRefV1",
        opaque_ref: opaqueRef,
        content_access: "controlled_artifact_required",
      },
    };

    expect(cloneFlowGraphData(source).result_snapshot_ref).toEqual({
      contract: "FlowPublicOpaqueSnapshotRefV1",
      opaque_ref: opaqueRef,
      content_access: "controlled_artifact_required",
    });
  });

  it("keeps the embedded private snapshot adapters from upgrading an opaque public reference", () => {
    const opaqueRef = `flowref_v1_${"b".repeat(64)}`;
    const source = {
      contract: "FlowPublicOpaqueSnapshotRefV1",
      opaque_ref: opaqueRef,
      content_access: "controlled_artifact_required",
    };
    const targetWindow: EmbeddedFlowWindow = {};
    runEmbeddedScript(browserBridgeMapperScript, targetWindow);
    runEmbeddedScript(embeddedGraphDataModelScript, targetWindow);

    expect(targetWindow.__ANALYTIX_FLOW_BRIDGE_MAPPER__?.sanitizeSnapshotRef(source)).toBeNull();
    expect(targetWindow.AnalytixGraphDataModel?.cloneResultSnapshotRef(source)).toBeNull();
  });

  it("SavedViewMetadataCannotHydrateGraphFacts", () => {
    const account = "62220202020202020202";
    const targetWindow: EmbeddedFlowWindow = {};
    runEmbeddedScript(browserBridgeMapperScript, targetWindow);

    const projected = targetWindow.__ANALYTIX_FLOW_BRIDGE_MAPPER__?.mapBackendViewToRuntimeView({
      view_id: "11111111-1111-4111-8111-111111111111",
      case_id: "case-alpha",
      view_name: account,
      graph_query: { account },
      view_state: {
        graph_snapshot_ref: { snapshot_id: account },
        graph: { nodes: [{ account }], edges: [] },
      },
    });

    expect(projected).toEqual({
      id: "11111111-1111-4111-8111-111111111111",
      title: "Saved flow view",
      mode: "relation",
      focusId: "",
      focusName: "",
      focusIds: [],
      focusNames: [],
      focusPlaceholderKinds: [],
      focusOnly: false,
      focusCounterpartyStrict: false,
      filters: { dir: "all", hop: 1, minAmount: 0, maxEdges: 800 },
      style: null,
      viewport: null,
      graph: { nodes: [], edges: [] },
      counts: { nodes: 0, edges: 0 },
      saved: true,
      caseId: "case-alpha",
      createdAt: "",
      updatedAt: "",
    });
    expect(JSON.stringify(projected)).not.toContain(account);
    expect(browserBridgeMapperScript).not.toContain("graphSnapshotRef");
    expect(browserBridgeMapperScript).not.toContain("graph_snapshot_ref");
    expect(embeddedFlowAppScript).not.toContain("getFlowGraphSnapshot");
    expect(embeddedFlowAppScript).not.toContain("graphSnapshotRef");
    expect(flowViewSyncStoreScript).not.toContain("graphSnapshotRef");
  });
});
