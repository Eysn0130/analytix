import { afterEach, describe, expect, it, vi } from "vitest";
import {
  applyDesktopBackendRuntimeState,
  clearDesktopBackendRuntimeCacheForTests,
} from "../../../../services/desktop/client";
import { invalidateSharedWsRuntimes } from "../../../../services/ws/shared-runtime";
import { createEmbeddedFlowBridgeService } from "./flow-bridge-service";

const LAUNCH_ID = "F".repeat(43);
const API_BASE = "http://127.0.0.1:18731";
const WS_BASE = "ws://127.0.0.1:18731/ws/events";
const PUBLIC_REF = `flowref_v1_${"a".repeat(64)}`;

function runningState(generation: number) {
  return {
    phase: "running" as const,
    generation,
    launchId: LAUNCH_ID,
    apiBase: API_BASE,
    wsBase: WS_BASE,
  };
}

class FakeWebSocket {
  static readonly OPEN = 1;
  static readonly instances: FakeWebSocket[] = [];

  readonly close = vi.fn();
  readonly send = vi.fn();
  readonly url: string;
  readyState = FakeWebSocket.OPEN;

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }

  addEventListener(): void {}
}

function publicResult(): Record<string, unknown> {
  return {
    contract: "FlowPublicResultBoundaryV1",
    publication_status: "blocked",
    fact_answer_allowed: false,
    content_access: "controlled_artifact_required",
    nodes: [],
    edges: [],
    stats: { build_ms: 1, cache_hit: false },
    runtime_graph: { nodes: [], edges: [] },
    result_snapshot_ref: {
      contract: "FlowPublicOpaqueSnapshotRefV1",
      opaque_ref: PUBLIC_REF,
      content_access: "controlled_artifact_required",
    },
  };
}

function responseFor(data: Record<string, unknown> = publicResult()): Response {
  return new Response(JSON.stringify({
    request_id: "request-1",
    timestamp: "2026-07-17T00:00:00.000Z",
    data,
  }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function publicJob(caseId: string, status: "queued" | "succeeded"): Record<string, unknown> {
  return {
    job_id: "flow-job-1",
    case_id: caseId,
    status,
    progress: status === "succeeded" ? 100 : 0,
    summary: { build_ms: null, cache_hit: null },
    error: null,
    result_available: status === "succeeded",
    created_at: "2026-07-17T00:00:00+00:00",
    updated_at: "2026-07-17T00:00:01+00:00",
  };
}

function directPayload(caseAlias: "case_id" | "caseId", caseId: string): Record<string, unknown> {
  return {
    [caseAlias]: caseId,
    seeds: ["seed-1"],
    depth: 1,
    direction: "both",
    min_amount: 0,
    source: "stats",
    focus_only: true,
    focus_unknown_name: false,
    include_missing_counterparty: false,
    focus_counterparty_strict: false,
    expected_total_amount: 0,
    expected_row_count: 1,
  };
}

function installDesktopAuthority(initialGeneration = 1): {
  setGeneration: (generation: number) => void;
} {
  let state = runningState(initialGeneration);
  const bridge = {
    ensureBackend: vi.fn(async () => state),
    getRuntimeInfo: vi.fn(async () => ({ backend: state })),
  };
  vi.stubGlobal("window", {
    analytix: { dataAnalysis: bridge },
    setTimeout: globalThis.setTimeout,
    clearTimeout: globalThis.clearTimeout,
    setInterval: globalThis.setInterval,
    clearInterval: globalThis.clearInterval,
  });
  vi.stubGlobal("WebSocket", FakeWebSocket);
  expect(applyDesktopBackendRuntimeState(state)).toBe(true);
  return {
    setGeneration(generation: number): void {
      state = runningState(generation);
      expect(applyDesktopBackendRuntimeState(state)).toBe(true);
    },
  };
}

afterEach(() => {
  clearDesktopBackendRuntimeCacheForTests();
  invalidateSharedWsRuntimes();
  FakeWebSocket.instances.splice(0);
  vi.unstubAllGlobals();
});

describe("Flow direct build authority and case binding", () => {
  it("canonicalizes caseId/case_id and never reuses factual responses within or across cases", async () => {
    installDesktopAuthority();
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => responseFor());
    vi.stubGlobal("fetch", fetchMock);
    const service = createEmbeddedFlowBridgeService({ apiBaseUrl: API_BASE });

    service.setCaseId("case-a");
    await service.buildFlowGraph(directPayload("caseId", "case-a"));
    service.setCaseId("case-b");
    await service.buildFlowGraph(directPayload("caseId", "case-b"));
    await service.buildFlowGraph(directPayload("case_id", "case-b"));

    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toMatchObject({
      case_id: "case-a",
    });
    expect(JSON.parse(String(fetchMock.mock.calls[1]?.[1]?.body))).toMatchObject({
      case_id: "case-b",
    });
    expect(JSON.parse(String(fetchMock.mock.calls[2]?.[1]?.body))).toMatchObject({
      case_id: "case-b",
    });
    expect(String(fetchMock.mock.calls[0]?.[1]?.body)).not.toContain("caseId");
    expect(String(fetchMock.mock.calls[1]?.[1]?.body)).not.toContain("caseId");
    expect(String(fetchMock.mock.calls[2]?.[1]?.body)).not.toContain("caseId");
  });

  it("rejects conflicting case aliases before transport", async () => {
    installDesktopAuthority();
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => responseFor());
    vi.stubGlobal("fetch", fetchMock);
    const service = createEmbeddedFlowBridgeService({ apiBaseUrl: API_BASE });
    service.setCaseId("case-a");

    await expect(service.buildFlowGraph({
      ...directPayload("case_id", "case-a"),
      caseId: "case-b",
    })).rejects.toThrow("flow_case_binding_conflict");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rejects every case-scoped stats and view operation before cross-case transport", async () => {
    installDesktopAuthority();
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => responseFor());
    vi.stubGlobal("fetch", fetchMock);
    const service = createEmbeddedFlowBridgeService({ apiBaseUrl: API_BASE });
    service.setCaseId("case-b");

    const crossCaseOperations = [
      service.getCaseDetail("case-a"),
      service.getFundsMeta("case-a"),
      service.getStatsTree({ case_id: "case-a", tab: "byName" }),
      service.getAccountTxnRows({ case_id: "case-a", account_key: "account-a" }),
      service.getStatsTxnRows({ case_id: "case-a", selected: [] }),
      service.listFlowViews("case-a"),
      service.saveFlowView({
        case_id: "case-a",
      }),
      service.deleteFlowView("view-a", "case-a"),
      service.renameFlowView("view-a", "view", "case-a"),
      service.reorderFlowViews({ case_id: "case-a", order: [] }),
    ];
    for (const operation of crossCaseOperations) {
      await expect(operation).rejects.toThrow("flow_bridge_case_binding_mismatch");
    }
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("projects every same-case stats read locally without serializing case or account data", async () => {
    installDesktopAuthority();
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => responseFor({}));
    vi.stubGlobal("fetch", fetchMock);
    const service = createEmbeddedFlowBridgeService({ apiBaseUrl: API_BASE });
    service.setCaseId("case-a");

    const [meta, tree, accountRows, txnRows] = await Promise.all([
      service.getFundsMeta("case-a"),
      service.getStatsTree({ case_id: "case-a", tab: "byCard" }),
      service.getAccountTxnRows({ case_id: "case-a", account_key: "6217000012345678901" }),
      service.getStatsTxnRows({
        case_id: "case-a",
        selected: ["6217000012345678901"],
        key_value: "Bearer secret-value",
      }),
    ]);

    expect(meta).toEqual({
      contract: "StatsMetaPublicBoundaryV1",
      semantic_status: "source_unavailable",
      fact_answer_allowed: false,
      blocker: "host_evidence_receipt_required",
      case_id: "",
      funds_status: "source_unavailable",
      date_min: "",
      date_max: "",
    });
    expect(tree).toMatchObject({ fact_answer_allowed: false, groups: [] });
    expect(accountRows).toMatchObject({ fact_answer_allowed: false, raw_details_exposed: false, rows: [] });
    expect(txnRows).toMatchObject({ fact_answer_allowed: false, raw_details_exposed: false, rows: [] });
    expect(JSON.stringify([meta, tree, accountRows, txnRows])).not.toContain("case-a");
    expect(JSON.stringify([meta, tree, accountRows, txnRows])).not.toContain("6217000012345678901");
    expect(JSON.stringify([meta, tree, accountRows, txnRows])).not.toContain("secret-value");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("binds every request to the full desktop generation and rejects the old service", async () => {
    const authority = installDesktopAuthority(1);
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => responseFor());
    vi.stubGlobal("fetch", fetchMock);
    const oldService = createEmbeddedFlowBridgeService({ apiBaseUrl: API_BASE });
    oldService.setCaseId("case-a");
    await oldService.buildFlowGraph(directPayload("case_id", "case-a"));

    authority.setGeneration(2);
    await expect(oldService.buildFlowGraph(directPayload("case_id", "case-a"))).rejects.toThrow(
      "flow_bridge_authority_stale",
    );

    const newService = createEmbeddedFlowBridgeService({ apiBaseUrl: API_BASE });
    newService.setCaseId("case-a");
    await newService.buildFlowGraph(directPayload("case_id", "case-a"));
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("revokes a pending case-A direct build before case B can own the service", async () => {
    installDesktopAuthority();
    let resolveCaseA!: (response: Response) => void;
    const caseAResponse = new Promise<Response>((resolve) => {
      resolveCaseA = resolve;
    });
    const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>()
      .mockImplementationOnce(async () => caseAResponse)
      .mockImplementation(async () => responseFor());
    vi.stubGlobal("fetch", fetchMock);
    const service = createEmbeddedFlowBridgeService({ apiBaseUrl: API_BASE });
    service.setCaseId("case-a");

    const pendingCaseA = service.buildFlowGraph(directPayload("case_id", "case-a"));
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    service.setCaseId("case-b");

    await expect(pendingCaseA).rejects.toThrow("flow_direct_build_context_revoked");
    await expect(service.buildFlowGraph(directPayload("case_id", "case-a"))).rejects.toThrow(
      "flow_bridge_case_binding_mismatch",
    );
    expect(FakeWebSocket.instances).toHaveLength(2);

    resolveCaseA(responseFor());
    await Promise.resolve();
    await Promise.resolve();
    await service.buildFlowGraph(directPayload("case_id", "case-b"));
    service.setCaseId("case-a");
    await service.buildFlowGraph(directPayload("case_id", "case-a"));

    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it("disposal synchronously revokes a direct build and prevents late factual reuse", async () => {
    installDesktopAuthority();
    let resolvePending!: (response: Response) => void;
    const pendingResponse = new Promise<Response>((resolve) => {
      resolvePending = resolve;
    });
    const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>()
      .mockImplementationOnce(async () => pendingResponse)
      .mockImplementation(async () => responseFor());
    vi.stubGlobal("fetch", fetchMock);
    const disposedService = createEmbeddedFlowBridgeService({ apiBaseUrl: API_BASE });
    disposedService.setCaseId("case-a");

    const pending = disposedService.buildFlowGraph(directPayload("case_id", "case-a"));
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    disposedService.dispose();
    await expect(pending).rejects.toThrow("flow_direct_build_context_revoked");
    await expect(disposedService.buildFlowGraph(directPayload("case_id", "case-a"))).rejects.toThrow(
      "flow_bridge_disposed",
    );

    resolvePending(responseFor());
    await Promise.resolve();
    await Promise.resolve();
    const freshService = createEmbeddedFlowBridgeService({ apiBaseUrl: API_BASE });
    freshService.setCaseId("case-a");
    await freshService.buildFlowGraph(directPayload("case_id", "case-a"));
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("rejects a non-direct job result that arrives after an A-B-A switch", async () => {
    installDesktopAuthority();
    let resolveResult!: (response: Response) => void;
    const pendingResult = new Promise<Response>((resolve) => {
      resolveResult = resolve;
    });
    const fetchMock = vi.fn(async (input: RequestInfo | URL): Promise<Response> => {
      const url = String(input);
      if (url.includes("/flow/jobs/flow-job-1/result")) return pendingResult;
      if (url.includes("/flow/jobs/flow-job-1?") || url.endsWith("/flow/jobs/flow-job-1")) {
        return responseFor(publicJob("case-a", "succeeded"));
      }
      if (url.includes("/flow/jobs") && !url.includes("/cancel")) {
        return responseFor(publicJob("case-a", "queued"));
      }
      return responseFor({ ok: true });
    });
    vi.stubGlobal("fetch", fetchMock);
    const service = createEmbeddedFlowBridgeService({ apiBaseUrl: API_BASE });
    service.setCaseId("case-a");

    const pending = service.buildFlowGraph({
      ...directPayload("case_id", "case-a"),
      depth: 2,
      focus_only: false,
    });
    const pendingRejection = expect(pending).rejects.toThrow("图谱生成已取消");
    await vi.waitFor(() => expect(fetchMock.mock.calls.length).toBeGreaterThanOrEqual(3), { timeout: 1_000 });
    service.setCaseId("case-b");
    service.setCaseId("case-a");
    resolveResult(responseFor());

    await pendingRejection;
  });
});
