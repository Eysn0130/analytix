import { describe, expect, it } from "vitest";
import {
  FLOW_CONTENT_CONTROLLED_ARTIFACT_REQUIRED,
  FLOW_PUBLIC_BOUNDARY_REJECTED,
  projectFlowBuildJobBoundary,
  projectFlowPublicResultBoundary,
  projectFlowPublicViewBoundary,
  projectFlowPublicViewListBoundary,
  requireControlledFlowArtifact,
} from "./flow-public-boundary";

const CASE_ID = "case-alpha";
const VIEW_ID = "11111111-1111-4111-8111-111111111111";
const PUBLIC_REF = `flowref_v1_${"a".repeat(64)}`;
const FULL_ACCOUNT = "62220202020202020202";

function publicSnapshotRef() {
  return {
    contract: "FlowPublicOpaqueSnapshotRefV1",
    opaque_ref: PUBLIC_REF,
    content_access: "controlled_artifact_required",
  };
}

function publicResult() {
  return {
    contract: "FlowPublicResultBoundaryV1",
    publication_status: "blocked",
    fact_answer_allowed: false,
    content_access: "controlled_artifact_required",
    nodes: [],
    edges: [],
    stats: { build_ms: 17, cache_hit: false },
    runtime_graph: { nodes: [], edges: [] },
    result_snapshot_ref: publicSnapshotRef(),
  };
}

function publicView() {
  const graph = { nodes: [], edges: [] };
  return {
    contract: "FlowPublicViewV1",
    view_id: VIEW_ID,
    case_id: CASE_ID,
    view_name: "Saved flow view",
    graph_query: {},
    view_state: {
      schema_version: 1,
      runtime_view: {},
      view_state_v2: {
        schema_version: 2,
        graph,
        filters: {},
        counts: {},
      },
      graph,
      filters: {},
      counts: {},
    },
    created_at: "",
    updated_at: "",
    content_access: "controlled_artifact_required",
  };
}

describe("flow ordinary renderer boundary", () => {
  it("reconstructs the closed result without copying response-owned graph fields", () => {
    const projected = projectFlowPublicResultBoundary(publicResult());

    expect(projected).toEqual(publicResult());
    expect(JSON.stringify(projected)).not.toContain(FULL_ACCOUNT);
  });

  it("rejects raw graph facts, unknown fields, and forged snapshot references", () => {
    const hostileValues = [
      { ...publicResult(), nodes: [{ node_id: FULL_ACCOUNT }] },
      { ...publicResult(), account: FULL_ACCOUNT },
      { ...publicResult(), result_snapshot_ref: { ...publicSnapshotRef(), opaque_ref: FULL_ACCOUNT } },
      { ...publicResult(), result_snapshot_ref: { ...publicSnapshotRef(), node_count: 0 } },
      { ...publicResult(), stats: { amount_total: 987654321.23 } },
    ];

    for (const value of hostileValues) {
      expect(() => projectFlowPublicResultBoundary(value)).toThrow(FLOW_PUBLIC_BOUNDARY_REJECTED);
    }
  });

  it("requires exact case-bound public views and rejects nested raw view payloads", () => {
    expect(projectFlowPublicViewBoundary(publicView(), CASE_ID)).toEqual(publicView());
    expect(() => projectFlowPublicViewBoundary(publicView(), "case-bravo")).toThrow(FLOW_PUBLIC_BOUNDARY_REJECTED);

    const hostile = publicView();
    hostile.view_state.filters = { account: FULL_ACCOUNT } as never;
    expect(() => projectFlowPublicViewBoundary(hostile, CASE_ID)).toThrow(FLOW_PUBLIC_BOUNDARY_REJECTED);

    const staleSnapshot = {
      ...publicView(),
      view_state: { ...publicView().view_state, graph_snapshot_ref: publicSnapshotRef() },
    };
    expect(() => projectFlowPublicViewBoundary(staleSnapshot, CASE_ID)).toThrow(FLOW_PUBLIC_BOUNDARY_REJECTED);

    const nestedFact = {
      ...publicView(),
      view_state: {
        ...publicView().view_state,
        graph: { nodes: [{ account: FULL_ACCOUNT }], edges: [] },
      },
    };
    expect(() => projectFlowPublicViewBoundary(nestedFact, CASE_ID)).toThrow(FLOW_PUBLIC_BOUNDARY_REJECTED);
  });

  it("validates every item and pagination field in a public view list", () => {
    const list = {
      items: [publicView()],
      page: { page: 1, page_size: 50, total: 1, total_pages: 1, has_next: false },
    };
    expect(projectFlowPublicViewListBoundary(list, CASE_ID)).toEqual(list);

    const crossCase = { ...publicView(), case_id: "case-bravo" };
    expect(() => projectFlowPublicViewListBoundary({ ...list, items: [crossCase] }, CASE_ID)).toThrow(
      FLOW_PUBLIC_BOUNDARY_REJECTED,
    );
  });

  it("accepts only exact operational job state and fixed terminal errors", () => {
    const job = {
      job_id: "flow-job-1",
      case_id: CASE_ID,
      status: "succeeded",
      progress: 100,
      summary: { build_ms: null, cache_hit: null },
      error: null,
      result_available: true,
      created_at: "2026-07-14T00:00:00+00:00",
      updated_at: "2026-07-14T00:00:01+00:00",
    };
    expect(projectFlowBuildJobBoundary(job, CASE_ID)).toMatchObject({
      job_id: "flow-job-1",
      case_id: CASE_ID,
      status: "succeeded",
      summary: {},
      error: null,
      result_available: true,
    });

    expect(() => projectFlowBuildJobBoundary({ ...job, error: FULL_ACCOUNT }, CASE_ID)).toThrow(
      FLOW_PUBLIC_BOUNDARY_REJECTED,
    );
    expect(() => projectFlowBuildJobBoundary({ ...job, result_available: false }, CASE_ID)).toThrow(
      FLOW_PUBLIC_BOUNDARY_REJECTED,
    );
  });

  it("blocks raw snapshot hydration before an HTTP path can be used", () => {
    expect(() => requireControlledFlowArtifact()).toThrow(FLOW_CONTENT_CONTROLLED_ARTIFACT_REQUIRED);
  });
});
