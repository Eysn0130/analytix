import { afterEach, describe, expect, it, vi } from "vitest";
import type { EmbeddedFlowBridgeService } from "../api/flow-bridge-service";
import { createFlowRuntimeGraphBackendMethods } from "./flow-runtime-graph-backend";

function createTargetWindow(): Window {
  const target = {
    setTimeout(callback: () => void): number {
      globalThis.setTimeout(callback, 0);
      return 1;
    },
    __ANALYTIX_FLOW_BRIDGE_MAPPER__: {
      buildFlowRuntimeGraphResponse: () => ({ ok: true, nodes: [], edges: [] }),
      mapBackendViewToRuntimeView: () => ({}),
      sanitizeFlowRuntimeGraphPayload: () => ({ nodes: [], edges: [] }),
      sanitizeSnapshotRef: (value: unknown) => value,
    },
  };
  vi.stubGlobal("window", target);
  return target as unknown as Window;
}

async function callGetFlowGraph(
  payload: Record<string, unknown>,
  buildFlowGraph = vi.fn(async () => ({
    nodes: [],
    edges: [],
    stats: {},
    runtime_graph: { nodes: [], edges: [] },
    result_snapshot_ref: null,
  }))
): Promise<{ response: Record<string, unknown>; buildFlowGraph: typeof buildFlowGraph }> {
  const targetWindow = createTargetWindow();
  const methods = createFlowRuntimeGraphBackendMethods({
    service: { buildFlowGraph } as unknown as EmbeddedFlowBridgeService,
    targetWindow,
    getCurrentCaseId: () => "case-alpha",
  });
  const response = await new Promise<Record<string, unknown>>((resolve) => {
    void methods.getFlowGraph(JSON.stringify(payload), (value) => {
      resolve(typeof value === "string" ? (JSON.parse(value) as Record<string, unknown>) : (value as Record<string, unknown>));
    });
  });
  return { response, buildFlowGraph };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("flow runtime graph evidence boundaries", () => {
  it("flow_self_only_placeholder_has_no_factual_zero", async () => {
    const { response, buildFlowGraph } = await callGetFlowGraph({
      caseId: "case-alpha",
      focusSelfOnly: true,
      focusId: "account-a",
      focusName: "Account A",
    });

    expect(response).toMatchObject({
      ok: true,
      semantic_status: "blocked",
      publication_status: "blocked",
      fact_answer_allowed: false,
      blocker: "flow_self_only_placeholder_requires_evidence",
      edges: [],
      stats: {},
    });
    expect(response.nodes).toEqual([
      expect.objectContaining({
        id: "account-a",
        placeholder: true,
        evidence_status: "unverified",
        fact_answer_allowed: false,
      }),
    ]);
    expect(JSON.stringify(response)).not.toContain("total_amount");
    expect(JSON.stringify(response)).not.toContain("total_count");
    expect(buildFlowGraph).not.toHaveBeenCalled();
  });

  it("flow_runtime_request_preserves_unknown_expected_metrics", async () => {
    const { response, buildFlowGraph } = await callGetFlowGraph({
      caseId: "case-alpha",
      seeds: ["account-a"],
      expectedTotalAmount: null,
      expectedRowCount: 1.5,
    });

    expect(response.ok).toBe(true);
    expect(buildFlowGraph).toHaveBeenCalledWith(
      expect.objectContaining({
        expected_total_amount: null,
        expected_row_count: null,
      })
    );
  });

  it("flow_runtime_filter_booleans_use_schema_defaults_and_reject_coercion", async () => {
    const valid = await callGetFlowGraph({ caseId: "case-alpha", seeds: ["account-a"] });
    expect(valid.response.ok).toBe(true);
    expect(valid.buildFlowGraph).toHaveBeenCalledWith(
      expect.objectContaining({
        focus_only: false,
        focus_unknown_name: false,
        include_missing_counterparty: false,
        focus_counterparty_strict: false,
      })
    );

    const invalid = await callGetFlowGraph({
      caseId: "case-alpha",
      seeds: ["account-a"],
      focusOnly: "false",
    });
    expect(invalid.response).toEqual({ ok: false, error: "flow_filter_boolean_invalid" });
    expect(invalid.buildFlowGraph).not.toHaveBeenCalled();
  });
});
