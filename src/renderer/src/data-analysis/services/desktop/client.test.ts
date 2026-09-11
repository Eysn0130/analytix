import { afterEach, describe, expect, it, vi } from "vitest";
import {
  applyDesktopBackendRuntimeState,
  clearDesktopBackendRuntimeCacheForTests,
  ensureDesktopBackendRuntime,
} from "./client";
import {
  getActiveDataAnalysisRuntimeBaseLease,
  getDataAnalysisRuntimeAbortSignal,
} from "../runtime-base";

const LAUNCH_ID = "L".repeat(43);

function runningState(
  generation = 1,
  port = 18731,
): {
  phase: "running";
  generation: number;
  launchId: string;
  apiBase: string;
  wsBase: string;
} {
  return {
    phase: "running",
    generation,
    launchId: LAUNCH_ID,
    apiBase: `http://127.0.0.1:${port}`,
    wsBase: `ws://127.0.0.1:${port}/ws/events`,
  };
}

function stubDesktopBridge(ensureBackend: () => Promise<unknown>): void {
  vi.stubGlobal("window", {
    analytix: {
      dataAnalysis: {
        ensureBackend,
      },
    },
  });
}

afterEach(() => {
  clearDesktopBackendRuntimeCacheForTests();
  vi.unstubAllGlobals();
});

describe("desktop data-analysis runtime client", () => {
  it("coalesces concurrent authenticated backend startup requests", async () => {
    const ensureBackend = vi.fn(async () => runningState());
    stubDesktopBridge(ensureBackend);

    await expect(Promise.all([ensureDesktopBackendRuntime(), ensureDesktopBackendRuntime()])).resolves.toEqual([
      true,
      true,
    ]);

    expect(ensureBackend).toHaveBeenCalledTimes(1);
  });

  it("reuses the recent authenticated ready state without waking the backend again", async () => {
    const ensureBackend = vi.fn(async () => runningState());
    stubDesktopBridge(ensureBackend);

    await expect(ensureDesktopBackendRuntime()).resolves.toBe(true);
    await expect(ensureDesktopBackendRuntime()).resolves.toBe(true);

    expect(ensureBackend).toHaveBeenCalledTimes(1);
  });

  it("treats native authority unavailable as terminal and never polls the bridge again", async () => {
    const ensureBackend = vi.fn(async () => ({
      phase: "failed" as const,
      generation: 0,
      authority: "unavailable" as const,
      terminal: true,
      blocker: "data_analysis_native_authority_unavailable" as const,
    }));
    stubDesktopBridge(ensureBackend);

    await expect(ensureDesktopBackendRuntime()).resolves.toBe(false);
    await expect(ensureDesktopBackendRuntime()).resolves.toBe(false);

    expect(ensureBackend).toHaveBeenCalledTimes(1);
    expect(getActiveDataAnalysisRuntimeBaseLease()).toBeNull();
  });

  it("TerminalClearsEndpoints and rejects a stale running response from that generation", () => {
    stubDesktopBridge(async () => runningState());

    expect(applyDesktopBackendRuntimeState(runningState())).toBe(true);
    const oldSignal = getDataAnalysisRuntimeAbortSignal();
    expect(getActiveDataAnalysisRuntimeBaseLease()).toMatchObject({
      apiBaseUrl: "http://127.0.0.1:18731",
      wsBaseUrl: "ws://127.0.0.1:18731/ws/events",
    });
    expect(oldSignal?.aborted).toBe(false);

    expect(applyDesktopBackendRuntimeState({ phase: "stopped", generation: 1 })).toBe(false);
    expect(getActiveDataAnalysisRuntimeBaseLease()).toBeNull();
    expect(oldSignal?.aborted).toBe(true);

    expect(applyDesktopBackendRuntimeState(runningState())).toBe(false);
    expect(getActiveDataAnalysisRuntimeBaseLease()).toBeNull();
  });

  it("keeps starting endpoint-free and activates only a newer authenticated generation", () => {
    stubDesktopBridge(async () => runningState());

    expect(applyDesktopBackendRuntimeState({ phase: "starting", generation: 2 })).toBe(false);
    expect(getActiveDataAnalysisRuntimeBaseLease()).toBeNull();
    expect(applyDesktopBackendRuntimeState(runningState(2, 18732))).toBe(true);
    expect(getActiveDataAnalysisRuntimeBaseLease()).toMatchObject({
      apiBaseUrl: "http://127.0.0.1:18732",
    });
  });

  it("rejects endpoint, launch, and generation fields that are not exact", () => {
    stubDesktopBridge(async () => runningState());

    expect(
      applyDesktopBackendRuntimeState({
        ...runningState(),
        apiBase: "http://attacker.example:18731",
      }),
    ).toBe(false);
    expect(getActiveDataAnalysisRuntimeBaseLease()).toBeNull();
    expect(
      applyDesktopBackendRuntimeState({
        ...runningState(2),
        launchId: "not-a-launch-id",
      }),
    ).toBe(false);
    expect(getActiveDataAnalysisRuntimeBaseLease()).toBeNull();
  });
});
