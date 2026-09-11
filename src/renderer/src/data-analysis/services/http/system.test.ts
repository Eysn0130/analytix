import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchBackendHealth } from "./system";
import {
  applyDesktopBackendRuntimeState,
  clearDesktopBackendRuntimeCacheForTests,
} from "../desktop/client";

const LAUNCH_ID = "S".repeat(43);

function runningState(generation = 1) {
  return {
    phase: "running" as const,
    generation,
    launchId: LAUNCH_ID,
    apiBase: "http://127.0.0.1:18731",
    wsBase: "ws://127.0.0.1:18731/ws/events",
  };
}

function stubDesktopBridge(): void {
  vi.stubGlobal("window", {
    analytix: {
      dataAnalysis: {
        ensureBackend: vi.fn(async () => runningState()),
      },
    },
  });
}

afterEach(() => {
  clearDesktopBackendRuntimeCacheForTests();
  vi.unstubAllGlobals();
});

describe("data-analysis backend health", () => {
  it("uses the authenticated ready endpoint without redirects or persistent cache", async () => {
    stubDesktopBridge();
    const fetch = vi.fn(async () => new Response(JSON.stringify({
      status: "ok",
      service: "analytix-data-analysis",
    }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }));
    vi.stubGlobal("fetch", fetch);

    await expect(fetchBackendHealth()).resolves.toMatchObject({ status: "ok" });

    expect(fetch).toHaveBeenCalledWith(
      "http://127.0.0.1:18731/health/ready",
      expect.objectContaining({ cache: "no-store", redirect: "error" }),
    );
  });

  it("rejects a health body that completes after the backend generation is revoked", async () => {
    stubDesktopBridge();
    expect(applyDesktopBackendRuntimeState(runningState())).toBe(true);
    let resolvePayload!: (payload: unknown) => void;
    const payloadPromise = new Promise<unknown>((resolve) => {
      resolvePayload = resolve;
    });
    vi.stubGlobal("fetch", vi.fn(async () => ({
      ok: true,
      status: 200,
      json: vi.fn(async () => payloadPromise),
    }) as unknown as Response));

    const request = fetchBackendHealth();
    await vi.waitFor(() => expect(globalThis.fetch).toHaveBeenCalledTimes(1));
    expect(applyDesktopBackendRuntimeState({ phase: "failed", generation: 1 })).toBe(false);
    resolvePayload({ status: "ok", service: "late" });

    await expect(request).rejects.toThrow("data_analysis_backend_generation_stale");
  });
});
