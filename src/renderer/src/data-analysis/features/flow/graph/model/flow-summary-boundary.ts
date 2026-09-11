export const FLOW_SUMMARY_CONTRACT = "FlowGraphSummaryV1" as const;

export type FlowSummaryStatus = "unloaded" | "blocked" | "unknown" | "partial" | "verified";

interface FlowSummaryProjectionBase {
  contract: typeof FLOW_SUMMARY_CONTRACT;
  layoutLabel: string;
  boundaryText: string;
  coverageText: string;
}

export interface FlowSummaryBoundaryProjection extends FlowSummaryProjectionBase {
  status: Exclude<FlowSummaryStatus, "verified">;
  factAnswerAllowed: false;
  nodes: null;
  edges: null;
  amount: null;
}

export interface FlowSummaryVerifiedProjection extends FlowSummaryProjectionBase {
  status: "verified";
  factAnswerAllowed: true;
  nodes: number;
  edges: number;
  amount: number;
}

export type FlowSummaryProjection = FlowSummaryBoundaryProjection | FlowSummaryVerifiedProjection;

const BOUNDARY_TEXT: Record<Exclude<FlowSummaryStatus, "verified">, string> = {
  unloaded: "尚未加载图谱结果",
  blocked: "图谱事实发布已阻断",
  unknown: "图谱统计尚未通过宿主证据验证",
  partial: "仅有部分数据覆盖，不显示全案合计",
};

const BOUNDARY_CODE: Record<Exclude<FlowSummaryStatus, "verified">, string> = {
  unloaded: "no_result_loaded",
  blocked: "publication_blocked",
  unknown: "host_verification_missing",
  partial: "partial_coverage",
};

function asObject(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : null;
}

function hasExactKeys(source: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(source).sort();
  const expected = [...keys].sort();
  return actual.length === expected.length && actual.every((key, index) => key === expected[index]);
}

function text(value: unknown): string {
  return String(value == null ? "" : value).trim();
}

export function createFlowSummaryBoundary(
  status: Exclude<FlowSummaryStatus, "verified">,
  layoutLabel = "布局"
): FlowSummaryBoundaryProjection {
  return {
    contract: FLOW_SUMMARY_CONTRACT,
    status,
    factAnswerAllowed: false,
    layoutLabel: text(layoutLabel) || "布局",
    nodes: null,
    edges: null,
    amount: null,
    boundaryText: BOUNDARY_TEXT[status],
    coverageText: "",
  };
}

/**
 * Projects the internal Flow shell summary into display-safe data.
 *
 * The external Flow bridge never forwards a serialized `verified` summary.
 * Until a host-registry adapter can prove receipt membership, same-context
 * binding, and complete snapshot coverage, every serialized verified-shaped
 * value remains unknown. Hash-shaped strings are not authority.
 */
export function projectFlowSummaryForChrome(value: unknown): FlowSummaryProjection {
  const source = asObject(value);
  if (!source) {
    return createFlowSummaryBoundary("unloaded");
  }
  const layoutLabel = text(source.layoutLabel) || "布局";
  if (source.contract !== FLOW_SUMMARY_CONTRACT) {
    return createFlowSummaryBoundary("unknown", layoutLabel);
  }

  if (source.status !== "verified") {
    const status = source.status;
    if (status !== "unloaded" && status !== "blocked" && status !== "unknown" && status !== "partial") {
      return createFlowSummaryBoundary("unknown", layoutLabel);
    }
    if (
      !hasExactKeys(source, [
        "contract",
        "status",
        "factAnswerAllowed",
        "layoutLabel",
        "nodes",
        "edges",
        "amount",
        "boundaryCode",
      ]) ||
      source.factAnswerAllowed !== false ||
      source.nodes !== null ||
      source.edges !== null ||
      source.amount !== null ||
      source.boundaryCode !== BOUNDARY_CODE[status]
    ) {
      return createFlowSummaryBoundary("unknown", layoutLabel);
    }
    return createFlowSummaryBoundary(status, layoutLabel);
  }

  return createFlowSummaryBoundary("unknown", layoutLabel);
}
