import { afterEach, describe, expect, it, vi } from "vitest";

import type { EmbeddedFlowBridgeService } from "../api/flow-bridge-service";
import { createFlowRuntimeDomainBackendMethods } from "./flow-runtime-domain-backend";

afterEach(() => {
  vi.unstubAllGlobals();
});

function installWindow(): void {
  vi.stubGlobal("window", {
    setTimeout: globalThis.setTimeout,
  });
}

async function invokeJson(
  invoke: (callback: (value: unknown) => void) => Promise<void>
): Promise<Record<string, unknown>> {
  const value = await new Promise<unknown>((resolve, reject) => {
    void invoke(resolve).catch(reject);
  });
  return JSON.parse(String(value)) as Record<string, unknown>;
}

describe("flow runtime domain evidence propagation", () => {
  it("preserves unavailable tree semantics instead of returning ok empty", async () => {
    installWindow();
    const service = {
      getStatsTree: vi.fn(async () => ({
        contract: "StatsTreePublicBoundaryV1",
        semantic_status: "source_unavailable",
        fact_answer_allowed: false,
        blocker: "materialization_unavailable",
        groups: [{ id: "acct-a", title: "account", meta: "", extra: "", items: [] }],
      })),
    } as unknown as EmbeddedFlowBridgeService;
    const methods = createFlowRuntimeDomainBackendMethods({ service, getCurrentCaseId: () => "case-a" });

    const result = await invokeJson((callback) => methods.getStatsTree(JSON.stringify({
      caseId: "case-a",
      tab: "byName",
    }), callback));

    expect(result).toMatchObject({
      ok: false,
      semanticStatus: "source_unavailable",
      factAnswerAllowed: false,
      blocker: "materialization_unavailable",
      groups: [],
    });
  });

  it("preserves blocked account-row semantics instead of returning ok empty", async () => {
    installWindow();
    const service = {
      getAccountTxnRows: vi.fn(async () => ({
        contract: "StatsAccountTxnRowsPublicBoundaryV1",
        semantic_status: "source_unavailable",
        fact_answer_allowed: false,
        raw_details_exposed: false,
        blocker: "materialization_unavailable",
        rows: [{ amount: 0, dc_flag: "进" }],
        done: false,
        next_cursor: null,
      })),
    } as unknown as EmbeddedFlowBridgeService;
    const methods = createFlowRuntimeDomainBackendMethods({ service, getCurrentCaseId: () => "case-a" });

    const result = await invokeJson((callback) => methods.getAccountTxnRows(JSON.stringify({
      caseId: "case-a",
      accountKey: "acct-a",
    }), callback));

    expect(result).toMatchObject({
      ok: false,
      semanticStatus: "source_unavailable",
      factAnswerAllowed: false,
      blocker: "materialization_unavailable",
      rows: [],
    });
  });

  it("preserves the direct-query fact gate instead of treating boundary rows as no-hit", async () => {
    installWindow();
    const service = {
      getStatsTxnRows: vi.fn(async () => ({
        contract: "StatsTxnRowsPublicBoundaryV1",
        fact_answer_allowed: false,
        raw_details_exposed: false,
        rows: [],
        done: false,
        next_cursor: null,
      })),
    } as unknown as EmbeddedFlowBridgeService;
    const methods = createFlowRuntimeDomainBackendMethods({ service, getCurrentCaseId: () => "case-a" });

    const result = await invokeJson((callback) => methods.getStatsTxnRows(JSON.stringify({
      caseId: "case-a",
      selected: ["acct-a"],
    }), callback));

    expect(result).toMatchObject({
      ok: false,
      factAnswerAllowed: false,
      rows: [],
      error: "host_evidence_receipt_required",
    });
  });
});
