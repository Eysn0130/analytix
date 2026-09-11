import { describe, expect, it } from "vitest";
import browserBridgeMapperScript from "../../runtime/embedded/analysis_flow/browser-bridge-mapper.js?raw";
import canvasShellAdapterScript from "../../runtime/embedded/analysis_flow/canvas-shell-adapter.js?raw";
import embeddedGraphDataModelScript from "../../runtime/embedded/analysis_flow/graph-data-model.js?raw";
import { projectFlowSummaryForChrome } from "./flow-summary-boundary";

interface EmbeddedFlowTestWindow {
  __ANALYTIX_ORDINARY_PII_PROJECTION__: {
    projectDetected(value: unknown): string;
    projectField(_field: string, value: unknown): string;
  };
  __ANALYTIX_FLOW_BRIDGE_MAPPER__?: {
    buildFlowRuntimeGraphResponse(request: Record<string, unknown>, data: Record<string, unknown>): Record<string, unknown>;
  };
  __ANALYTIX_FLOW_CANVAS_ADAPTER__?: {
    buildGraphStatsSummary(input: { graphData: Record<string, unknown> | null; layoutPreset: string }): unknown;
  };
  AnalytixGraphDataModel?: {
    cloneGraphData(value: Record<string, unknown>): Record<string, unknown>;
  };
}

function runEmbeddedScript(script: string, targetWindow: EmbeddedFlowTestWindow): void {
  Function("window", script)(targetWindow);
}

function createTargetWindow(): EmbeddedFlowTestWindow {
  return {
    __ANALYTIX_ORDINARY_PII_PROJECTION__: {
      projectDetected: (value) => String(value == null ? "" : value),
      projectField: (_field, value) => String(value == null ? "" : value),
    },
  };
}

describe("Flow summary propagation", () => {
  it("BlockedPublicFlowBoundaryReachesChromeWithoutFactualZeros", () => {
    const target = createTargetWindow();
    runEmbeddedScript(browserBridgeMapperScript, target);
    runEmbeddedScript(embeddedGraphDataModelScript, target);
    runEmbeddedScript(canvasShellAdapterScript, target);

    const response = target.__ANALYTIX_FLOW_BRIDGE_MAPPER__?.buildFlowRuntimeGraphResponse(
      {},
      {
        contract: "FlowPublicResultBoundaryV1",
        publication_status: "blocked",
        fact_answer_allowed: false,
        content_access: "controlled_artifact_required",
        nodes: [],
        edges: [],
        stats: {},
        runtime_graph: { nodes: [], edges: [] },
        result_snapshot_ref: null,
      }
    );
    expect(response).toBeDefined();

    const graphData = target.AnalytixGraphDataModel?.cloneGraphData(response ?? {});
    const shellSummary = target.__ANALYTIX_FLOW_CANVAS_ADAPTER__?.buildGraphStatsSummary({
      graphData: graphData ?? null,
      layoutPreset: "compact",
    });
    const chromeSummary = projectFlowSummaryForChrome(shellSummary);

    expect(chromeSummary).toMatchObject({
      status: "blocked",
      factAnswerAllowed: false,
      nodes: null,
      edges: null,
      amount: null,
      boundaryText: "图谱事实发布已阻断",
    });
    expect(JSON.stringify(chromeSummary)).not.toContain('"nodes":0');
    expect(JSON.stringify(chromeSummary)).not.toContain('"edges":0');
    expect(JSON.stringify(chromeSummary)).not.toContain('"amount":0');
  });

  it("SkeletonProjectionRemainsPartialWithoutWholeCaseTotals", () => {
    const target = createTargetWindow();
    runEmbeddedScript(canvasShellAdapterScript, target);

    const shellSummary = target.__ANALYTIX_FLOW_CANVAS_ADAPTER__?.buildGraphStatsSummary({
      graphData: {
        nodes: [{ id: "visible-node" }],
        edges: [],
        projection: { mode: "skeleton" },
      },
      layoutPreset: "network",
    });

    expect(projectFlowSummaryForChrome(shellSummary)).toMatchObject({
      status: "partial",
      factAnswerAllowed: false,
      nodes: null,
      edges: null,
      amount: null,
    });
  });

  it("ModelReportedVerifiedSummaryCannotCrossTheBridge", () => {
    const target = createTargetWindow();
    runEmbeddedScript(browserBridgeMapperScript, target);
    runEmbeddedScript(embeddedGraphDataModelScript, target);
    runEmbeddedScript(canvasShellAdapterScript, target);

    const response = target.__ANALYTIX_FLOW_BRIDGE_MAPPER__?.buildFlowRuntimeGraphResponse(
      {},
      {
        nodes: [],
        edges: [],
        stats: {},
        runtime_graph: { nodes: [], edges: [] },
        result_snapshot_ref: null,
        flow_summary: {
          contract: "FlowGraphSummaryV1",
          status: "verified",
          factAnswerAllowed: true,
          nodes: 0,
          edges: 0,
          amount: 0,
          receiptId: "model-reported-receipt",
        },
      }
    );

    expect(response).not.toHaveProperty("flow_summary");
    const graphData = target.AnalytixGraphDataModel?.cloneGraphData(response ?? {});
    const shellSummary = target.__ANALYTIX_FLOW_CANVAS_ADAPTER__?.buildGraphStatsSummary({
      graphData: graphData ?? null,
      layoutPreset: "compact",
    });
    expect(projectFlowSummaryForChrome(shellSummary)).toMatchObject({
      status: "unknown",
      factAnswerAllowed: false,
      nodes: null,
      edges: null,
      amount: null,
    });
  });
});
