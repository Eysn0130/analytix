import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import flowStatsLinkSource from "./flow-stats-link.ts?raw";

const { postMock } = vi.hoisted(() => ({ postMock: vi.fn() }));

vi.mock("../http/client", () => ({
  httpClient: {
    post: postMock,
  },
}));

import { getSharedStatsFlowWarmService } from "./flow-stats-link";

function directPayload(): Record<string, unknown> {
  return {
    case_id: "case-a",
    seeds: ["seed-a"],
    depth: 1,
    direction: "both",
    min_amount: 0,
    source: "stats",
    focus_only: true,
    focus_unknown_name: false,
    include_missing_counterparty: false,
    focus_counterparty_strict: false,
    expected_total_amount: 0,
    expected_row_count: 0,
  };
}

beforeEach(() => {
  postMock.mockReset();
  postMock.mockResolvedValue({ data: {} });
  vi.stubGlobal("window", {});
  getSharedStatsFlowWarmService().setCaseId("");
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("stats flow warm missing-value boundary", () => {
  it("does not warm or populate a request cache for unknown direct-build facts", async () => {
    const service = getSharedStatsFlowWarmService();
    service.setCaseId("case-a");

    await service.warmFlowGraph({ ...directPayload(), expected_total_amount: null });
    await service.warmFlowGraph({ ...directPayload(), expected_row_count: "0" });
    await service.warmFlowGraph({ ...directPayload(), min_amount: undefined });
    await service.warmFlowGraph({ ...directPayload(), focus_unknown_name: "false" });

    expect(postMock).not.toHaveBeenCalled();

    await service.warmFlowGraph(directPayload());
    await service.warmFlowGraph(directPayload());

    expect(postMock).toHaveBeenCalledTimes(2);
    expect(postMock).toHaveBeenCalledWith(
      "/api/v1/analysis/flow/graph",
      expect.objectContaining({
        expected_total_amount: 0,
        expected_row_count: 0,
        min_amount: 0,
        focus_unknown_name: false,
      }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    for (const call of postMock.mock.calls) {
      const sent = call[1] as Record<string, unknown>;
      expect(sent.trace_id).toMatch(/^flow-warm:[a-z0-9]+:[a-z0-9]+$/);
      expect(sent.trace_id).not.toContain("case-a");
      expect(sent).not.toHaveProperty("traceId");
    }
    expect(flowStatsLinkSource).not.toContain("BoundedCache");
    expect(flowStatsLinkSource).not.toContain("SharedStatsFlowWarmStore");
    expect(flowStatsLinkSource).not.toContain("__ANALYTIX_SHARED_STATS_FLOW_WARM_STORE__");
    expect(flowStatsLinkSource).not.toContain("store.inflight");
  });

  it("keeps concurrent identical warm requests request-local without shared inflight dedup", async () => {
    const resolvers: Array<() => void> = [];
    postMock.mockImplementation(() => new Promise((resolve) => {
      resolvers.push(() => resolve({ data: {} }));
    }));
    const service = getSharedStatsFlowWarmService();
    service.setCaseId("case-a");

    const first = service.warmFlowGraph(directPayload());
    const second = service.warmFlowGraph(directPayload());

    await vi.waitFor(() => expect(postMock).toHaveBeenCalledTimes(2));
    const firstSignal = postMock.mock.calls[0]?.[2]?.signal;
    const secondSignal = postMock.mock.calls[1]?.[2]?.signal;
    expect(firstSignal).toBeInstanceOf(AbortSignal);
    expect(secondSignal).toBeInstanceOf(AbortSignal);
    expect(firstSignal).not.toBe(secondSignal);
    for (const resolve of resolvers) resolve();
    await expect(Promise.all([first, second])).resolves.toEqual([undefined, undefined]);
  });

  it("aborts an old-case request and rejects its late completion", async () => {
    let resolveOld!: () => void;
    postMock.mockImplementationOnce(() => new Promise((resolve) => {
      resolveOld = () => resolve({ data: {} });
    }));
    const service = getSharedStatsFlowWarmService();
    service.setCaseId("case-a");
    const oldRequest = service.warmFlowGraph(directPayload());
    await vi.waitFor(() => expect(postMock).toHaveBeenCalledTimes(1));
    const oldSignal = postMock.mock.calls[0]?.[2]?.signal;

    service.setCaseId("case-b");
    expect(oldSignal?.aborted).toBe(true);
    resolveOld();
    await expect(oldRequest).rejects.toThrow("stats_flow_warm_context_revoked");

    await service.warmFlowGraph({ ...directPayload(), case_id: "case-b" });
    expect(postMock).toHaveBeenCalledTimes(2);
  });
});
