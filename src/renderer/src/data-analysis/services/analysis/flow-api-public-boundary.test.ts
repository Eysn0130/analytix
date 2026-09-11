import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  patch: vi.fn(),
  delete: vi.fn(),
}));

vi.mock("../http/client", () => ({
  httpClient: mocks,
}));

import {
  buildFlowGraph,
  createFlowView,
  getFlowBuildJob,
  listFlowViews,
} from "./flow-api";
import {
  FLOW_PUBLIC_BOUNDARY_REJECTED,
} from "./flow-public-boundary";

const CASE_ID = "case-alpha";

function graphRequest() {
  return {
    case_id: CASE_ID,
    seeds: ["seed"],
    depth: 1,
    direction: "both" as const,
    min_amount: 0,
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
    stats: { build_ms: null, cache_hit: null },
    runtime_graph: { nodes: [], edges: [] },
    result_snapshot_ref: null,
  };
}

function publicViewList() {
  return {
    items: [],
    page: { page: 1, page_size: 50, total: 0, total_pages: 0, has_next: false },
  };
}

function publicView() {
  const graph = { nodes: [], edges: [] };
  return {
    contract: "FlowPublicViewV1",
    view_id: "11111111-1111-4111-8111-111111111111",
    case_id: CASE_ID,
    view_name: "Saved flow view",
    graph_query: {},
    view_state: {
      schema_version: 1,
      runtime_view: {},
      view_state_v2: { schema_version: 2, graph, filters: {}, counts: {} },
      graph,
      filters: {},
      counts: {},
    },
    created_at: "",
    updated_at: "",
    content_access: "controlled_artifact_required",
  };
}

beforeEach(() => {
  mocks.get.mockReset();
  mocks.post.mockReset();
  mocks.patch.mockReset();
  mocks.delete.mockReset();
});

describe("flow API public-boundary wiring", () => {
  it("returns only a validated host projection", async () => {
    mocks.post.mockResolvedValue({ data: publicResult() });

    await expect(buildFlowGraph(graphRequest())).resolves.toMatchObject({
      contract: "FlowPublicResultBoundaryV1",
      fact_answer_allowed: false,
      nodes: [],
      edges: [],
      stats: {},
    });
  });

  it("rejects a successful HTTP response that contains raw graph facts", async () => {
    mocks.post.mockResolvedValue({
      data: {
        ...publicResult(),
        nodes: [{ node_id: "62220202020202020202" }],
      },
    });

    await expect(buildFlowGraph(graphRequest())).rejects.toThrow(FLOW_PUBLIC_BOUNDARY_REJECTED);
  });

  it("revalidates the exact case on every job read", async () => {
    mocks.get.mockResolvedValue({
      data: {
        job_id: "flow-job-1",
        case_id: "case-bravo",
        status: "running",
        progress: 1,
        summary: { build_ms: null, cache_hit: null },
        error: null,
        result_available: false,
        created_at: "2026-07-14T00:00:00+00:00",
        updated_at: "2026-07-14T00:00:01+00:00",
      },
    });

    await expect(getFlowBuildJob("flow-job-1", CASE_ID)).rejects.toThrow(FLOW_PUBLIC_BOUNDARY_REJECTED);
    expect(mocks.get).toHaveBeenCalledWith(expect.stringContaining("case_id=case-alpha"));
  });

  it("never requests saved-view graph hydration", async () => {
    mocks.get.mockResolvedValue({ data: publicViewList() });

    await expect(listFlowViews({ caseId: CASE_ID })).resolves.toEqual(publicViewList());
    const url = String(mocks.get.mock.calls[0]?.[0] || "");
    expect(url).not.toContain("hydrate_graph");
    expect(url).not.toContain("graph-snapshots");
  });

  it("creates saved-view metadata without sending graph facts", async () => {
    mocks.post.mockResolvedValue({ data: publicView() });

    await expect(createFlowView({ case_id: CASE_ID })).resolves.toMatchObject({
      view_id: "11111111-1111-4111-8111-111111111111",
      graph_query: {},
    });
    expect(mocks.post).toHaveBeenCalledWith("/api/v1/analysis/flow/views", { case_id: CASE_ID });
  });
});
