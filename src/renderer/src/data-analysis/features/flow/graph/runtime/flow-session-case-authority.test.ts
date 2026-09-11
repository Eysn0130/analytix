import { describe, expect, it, vi } from "vitest";
import flowSessionStoreScript from "../../runtime/embedded/analysis_flow/flow-session-store.js?raw";

type SessionStore = {
  setCaseId(caseId: string): Promise<void>;
};

type EmbeddedSessionWindow = {
  __ANALYTIX_FLOW_SESSION_STORE__?: {
    createFlowSessionStore(options: Record<string, unknown>): SessionStore;
  };
};

function runEmbeddedScript(script: string, targetWindow: EmbeddedSessionWindow): void {
  Function("window", script)(targetWindow);
}

describe("embedded Flow session case authority", () => {
  it("does not let an A response mutate badge, tree, funds, or views after B wins", async () => {
    const targetWindow: EmbeddedSessionWindow = {};
    runEmbeddedScript(flowSessionStoreScript, targetWindow);
    const nameCallbacks: Array<(name: string) => void> = [];
    const state = {
      caseId: "",
      tab: "byName",
      viewsLoaded: false,
      backend: {
        getCaseName(callback: (name: string) => void): void {
          nameCallbacks.push(callback);
        },
      },
    };
    const setCaseBadge = vi.fn();
    const loadTreeData = vi.fn(async () => undefined);
    const syncFundsStatus = vi.fn();
    const loadViewsFromBackend = vi.fn(async () => undefined);
    const store = targetWindow.__ANALYTIX_FLOW_SESSION_STORE__?.createFlowSessionStore({
      state,
      deps: {
        setCaseBadge,
        loadTreeData,
        syncFundsStatus,
        loadViewsFromBackend,
        clearGraph: vi.fn(),
      },
    });
    expect(store).toBeTruthy();

    const caseA = store!.setCaseId("case-a");
    const caseB = store!.setCaseId("case-b");
    expect(nameCallbacks).toHaveLength(2);
    nameCallbacks[1]("Case B");
    await caseB;
    nameCallbacks[0]("Case A");
    await caseA;

    expect(state.caseId).toBe("case-b");
    expect(setCaseBadge).toHaveBeenCalledTimes(1);
    expect(setCaseBadge).toHaveBeenCalledWith("Case B", "case-b");
    expect(loadTreeData).toHaveBeenCalledTimes(1);
    expect(syncFundsStatus).toHaveBeenCalledTimes(1);
    expect(loadViewsFromBackend).toHaveBeenCalledTimes(1);
  });
});
