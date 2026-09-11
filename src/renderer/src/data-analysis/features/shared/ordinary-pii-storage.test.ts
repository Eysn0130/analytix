import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import analysisPageSource from "../analysis/AnalysisPage.tsx?raw";
import flowTransferSource from "../flow/graph/adapters/flow-page-transfer.ts?raw";
import statsFlowContextSource from "../../services/analysis/stats-flow-context.ts?raw";
import {
  clearStagedFlowTransferPayload,
  FLOW_TRANSFER_STORAGE_KEY,
  hasStagedFlowTransferPayload,
  readFlowTransferPayload,
  stageFlowTransferPayload,
} from "../flow/graph/adapters/flow-page-transfer";
import {
  readPersistedStatsFlowContexts,
  writePersistedStatsFlowContexts,
} from "../../services/analysis/stats-flow-context";

type TestStorage = Storage & Record<string, unknown>;

function createStorage(): TestStorage {
  const storage = {
    getItem: vi.fn((key: string) => typeof storage[key] === "string" ? storage[key] as string : null),
    setItem: vi.fn((key: string, value: string) => {
      storage[key] = value;
    }),
    removeItem: vi.fn((key: string) => {
      delete storage[key];
    }),
    clear: vi.fn(() => {
      Object.keys(storage).forEach((key) => {
        if (typeof storage[key] === "string") delete storage[key];
      });
    }),
    key: vi.fn((index: number) => Object.keys(storage).filter((key) => typeof storage[key] === "string")[index] ?? null),
    length: 0,
  } as TestStorage;
  return storage;
}

let storage: TestStorage;

beforeEach(() => {
  storage = createStorage();
  vi.stubGlobal("localStorage", storage);
  vi.stubGlobal("window", { localStorage: storage });
  clearStagedFlowTransferPayload();
  vi.mocked(storage.removeItem).mockClear();
});

afterEach(() => {
  clearStagedFlowTransferPayload();
  vi.unstubAllGlobals();
});

describe("ordinary data-analysis PII persistence quarantine", () => {
  it("uses a one-shot in-memory flow handoff and never writes the full account to storage", () => {
    const rawAccount = "6214600780000579708";
    stageFlowTransferPayload({ caseId: "case-a", focusId: rawAccount, focusIds: [rawAccount] });

    expect(hasStagedFlowTransferPayload()).toBe(true);
    expect(readFlowTransferPayload()).toMatchObject({ caseId: "case-a", focusId: rawAccount });
    expect(storage.setItem).not.toHaveBeenCalled();
    expect(JSON.stringify(storage)).not.toContain(rawAccount);

    clearStagedFlowTransferPayload();
    expect(hasStagedFlowTransferPayload()).toBe(false);
  });

  it("deletes and ignores legacy raw flow/page context storage", () => {
    const rawAccount = "6214600780000579708";
    storage[FLOW_TRANSFER_STORAGE_KEY] = JSON.stringify({ caseId: "case-a", focusId: rawAccount });
    storage["analytix:stats:flow-context:case-a"] = JSON.stringify({ relationPayload: { focusId: rawAccount } });

    expect(readFlowTransferPayload()).toBeNull();
    expect(readPersistedStatsFlowContexts("case-a")).toBeNull();
    writePersistedStatsFlowContexts("case-a", {
      relationPayload: { focusId: rawAccount },
      flowPayload: { focusId: rawAccount },
    });

    expect(storage[FLOW_TRANSFER_STORAGE_KEY]).toBeUndefined();
    expect(storage["analytix:stats:flow-context:case-a"]).toBeUndefined();
    expect(storage.setItem).not.toHaveBeenCalled();
    expect(JSON.stringify(storage)).not.toContain(rawAccount);
  });

  it("keeps page, context, and handoff sources free of localStorage writes", () => {
    for (const source of [analysisPageSource, flowTransferSource, statsFlowContextSource]) {
      expect(source).not.toContain("localStorage.setItem");
    }
    expect(analysisPageSource).toContain("stageFlowTransferPayload(payload)");
    expect(flowTransferSource).not.toContain("stagedFlowTransferRaw");
  });
});
