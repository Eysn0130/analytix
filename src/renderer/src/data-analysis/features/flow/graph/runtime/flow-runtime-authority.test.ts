import { afterEach, describe, expect, it, vi } from "vitest";
import {
  applyDesktopBackendRuntimeState,
  clearDesktopBackendRuntimeCacheForTests,
} from "../../../../services/desktop/client";
import { ensureFlowCanvasAdapter } from "./embedded-flow-runtime";

const LAUNCH_ID = "F".repeat(43);

afterEach(() => {
  clearDesktopBackendRuntimeCacheForTests();
  vi.unstubAllGlobals();
});

describe("embedded flow runtime endpoint authority", () => {
  it("FlowRuntimeRejectsMissingOrStaleEndpointBeforeSideEffects", async () => {
    const fetch = vi.fn();
    const WebSocket = vi.fn();
    vi.stubGlobal("fetch", fetch);
    vi.stubGlobal("WebSocket", WebSocket);
    vi.stubGlobal("window", {
      location: { protocol: "file:", origin: "null", href: "file:///tmp/analytix.html" },
    });

    await expect(ensureFlowCanvasAdapter("")).rejects.toThrow(
      "data_analysis_backend_authority_missing",
    );
    expect(fetch).not.toHaveBeenCalled();
    expect(WebSocket).not.toHaveBeenCalled();

    vi.stubGlobal("window", {
      analytix: { dataAnalysis: {} },
    });
    expect(applyDesktopBackendRuntimeState({
      phase: "running",
      generation: 1,
      launchId: LAUNCH_ID,
      apiBase: "http://127.0.0.1:18731",
      wsBase: "ws://127.0.0.1:18731/ws/events",
    })).toBe(true);

    await expect(ensureFlowCanvasAdapter("http://127.0.0.1:18732")).rejects.toThrow(
      "data_analysis_backend_endpoint_stale",
    );
    expect(fetch).not.toHaveBeenCalled();
    expect(WebSocket).not.toHaveBeenCalled();
  });

  it("FlowRuntimeCapturedLeaseCannotUpgradeToANewerGeneration", async () => {
    const host = {
      className: "",
      style: {},
      contains: vi.fn(() => false),
      remove: vi.fn(),
    };
    vi.stubGlobal("DOMParser", class {
      parseFromString(): object {
        return {
          querySelectorAll: () => [],
          body: { innerHTML: "" },
        };
      }
    });
    vi.stubGlobal("document", {
      documentElement: { dataset: {} },
      createElement: vi.fn(() => host),
    });
    vi.stubGlobal("window", {
      analytix: { dataAnalysis: {} },
      location: { href: "file:///tmp/analytix.html" },
    });

    expect(applyDesktopBackendRuntimeState({
      phase: "running",
      generation: 1,
      launchId: LAUNCH_ID,
      apiBase: "http://127.0.0.1:18731",
      wsBase: "ws://127.0.0.1:18731/ws/events",
    })).toBe(true);
    const staleAdapter = await ensureFlowCanvasAdapter("http://127.0.0.1:18731");
    const replaceChildren = vi.fn();
    const container = {
      replaceChildren,
      childElementCount: 0,
    } as unknown as HTMLElement;

    expect(applyDesktopBackendRuntimeState({
      phase: "running",
      generation: 2,
      launchId: LAUNCH_ID,
      apiBase: "http://127.0.0.1:18731",
      wsBase: "ws://127.0.0.1:18731/ws/events",
    })).toBe(true);

    await expect(staleAdapter.attach(container)).rejects.toThrow(
      "data_analysis_backend_authority_invalidated",
    );
    expect(replaceChildren).not.toHaveBeenCalled();
    expect(container.childElementCount).toBe(0);
  });

  it("BrowserFlowEndpointCannotCreateAuthorityOrDOMSideEffects", async () => {
    const createElement = vi.fn();
    const fetch = vi.fn();
    const WebSocket = vi.fn();
    vi.stubGlobal("document", {
      documentElement: { dataset: {} },
      createElement,
    });
    vi.stubGlobal("window", {
      location: {
        protocol: "https:",
        origin: "https://analytix.example",
        href: "https://analytix.example/workbench",
      },
    });
    vi.stubGlobal("fetch", fetch);
    vi.stubGlobal("WebSocket", WebSocket);

    await expect(ensureFlowCanvasAdapter("https://analytix.example")).rejects.toThrow(
      "data_analysis_backend_authority_missing",
    );
    expect(createElement).not.toHaveBeenCalled();
    expect(fetch).not.toHaveBeenCalled();
    expect(WebSocket).not.toHaveBeenCalled();
  });
});
