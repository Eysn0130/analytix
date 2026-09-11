import { describe, expect, it } from "vitest";
import {
  FLOW_SUMMARY_CONTRACT,
  projectFlowSummaryForChrome,
} from "./flow-summary-boundary";

function boundary(status: "unloaded" | "blocked" | "unknown" | "partial", boundaryCode: string) {
  return {
    contract: FLOW_SUMMARY_CONTRACT,
    status,
    factAnswerAllowed: false,
    layoutLabel: "关联图布局",
    nodes: null,
    edges: null,
    amount: null,
    boundaryCode,
  };
}

describe("Flow summary evidence boundary", () => {
  it("BlockedFlowSummaryNeverShowsZeros", () => {
    const projected = projectFlowSummaryForChrome(boundary("blocked", "publication_blocked"));

    expect(projected).toMatchObject({
      status: "blocked",
      factAnswerAllowed: false,
      nodes: null,
      edges: null,
      amount: null,
      boundaryText: "图谱事实发布已阻断",
    });
    expect(JSON.stringify(projected)).not.toContain('"nodes":0');
    expect(JSON.stringify(projected)).not.toContain('"amount":0');
  });

  it("UnloadedFlowSummaryNeverShowsZeros", () => {
    const projected = projectFlowSummaryForChrome(null);

    expect(projected).toMatchObject({
      status: "unloaded",
      factAnswerAllowed: false,
      nodes: null,
      edges: null,
      amount: null,
      boundaryText: "尚未加载图谱结果",
    });
  });

  it("PartialFlowSummaryNoWholeCaseTotals", () => {
    const projected = projectFlowSummaryForChrome({
      ...boundary("partial", "partial_coverage"),
      nodes: 12,
      edges: 8,
      amount: 99,
    });

    expect(projected).toMatchObject({
      status: "unknown",
      factAnswerAllowed: false,
      nodes: null,
      edges: null,
      amount: null,
    });

    const validPartial = projectFlowSummaryForChrome(boundary("partial", "partial_coverage"));
    expect(validPartial).toMatchObject({
      status: "partial",
      nodes: null,
      edges: null,
      amount: null,
      boundaryText: "仅有部分数据覆盖，不显示全案合计",
    });
  });

  it("SerializedVerifiedEmptyCannotMintZeroWithHashShapedFields", () => {
    const projected = projectFlowSummaryForChrome({
      contract: FLOW_SUMMARY_CONTRACT,
      status: "verified",
      factAnswerAllowed: true,
      layoutLabel: "关联图布局",
      nodes: 0,
      edges: 0,
      amount: 0,
      coverage: {
        completeness: "complete",
        datasetSnapshotId: "snapshot-empty",
        contextDigest: "a".repeat(64),
        registryIntegrityProof: "b".repeat(64),
        nodeCount: 0,
        edgeCount: 0,
        amountCoverage: "complete",
      },
    });

    expect(projected).toEqual({
      contract: FLOW_SUMMARY_CONTRACT,
      status: "unknown",
      factAnswerAllowed: false,
      layoutLabel: "关联图布局",
      nodes: null,
      edges: null,
      amount: null,
      boundaryText: "图谱统计尚未通过宿主证据验证",
      coverageText: "",
    });
  });

  it("serialized verified claims without complete host coverage fail closed", () => {
    const projected = projectFlowSummaryForChrome({
      contract: FLOW_SUMMARY_CONTRACT,
      status: "verified",
      factAnswerAllowed: true,
      layoutLabel: "关联图布局",
      nodes: 0,
      edges: 0,
      amount: 0,
      receiptId: "model-reported-receipt",
    });

    expect(projected).toMatchObject({
      status: "unknown",
      factAnswerAllowed: false,
      nodes: null,
      edges: null,
      amount: null,
    });
  });
});
