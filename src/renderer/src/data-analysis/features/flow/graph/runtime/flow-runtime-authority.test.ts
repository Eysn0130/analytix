// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  applyDesktopBackendRuntimeState,
  clearDesktopBackendRuntimeCacheForTests,
} from "../../../../services/desktop/client";
import { ensureFlowCanvasAdapter } from "./embedded-flow-runtime";
import { ordinaryPiiProjection } from "../../../shared/ordinary-pii-projection";

const LAUNCH_ID = "F".repeat(43);

afterEach(() => {
  clearDesktopBackendRuntimeCacheForTests();
  vi.restoreAllMocks();
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


  it.each(["before-load", "after-load", "replaced-after-load"] as const)(
    "rejects %s PII API failure and clears the iframe even when script onload succeeds",
    async failure => {
      const container = document.createElement("div");
      document.body.append(container);
      const createElement = document.createElement.bind(document);
      const frames: HTMLIFrameElement[] = [];
      const loadedProjections: unknown[] = [];
      const fetch = vi.fn(), WebSocket = vi.fn();
      vi.stubGlobal("fetch", fetch);
      vi.stubGlobal("WebSocket", WebSocket);
      vi.stubGlobal("window", {
        analytix: { dataAnalysis: {} },
        location: { href: "file:///tmp/analytix.html" },
        setTimeout,
        clearTimeout,
      });
      vi.spyOn(console, "warn").mockImplementation(() => {});
      vi.spyOn(console, "error").mockImplementation(() => {});
      vi.spyOn(document, "createElement").mockImplementation((tagName, options) => {
        const element = createElement(tagName, options);
        if (tagName !== "iframe") return element;
        const frame = element as HTMLIFrameElement;
        frames.push(frame);
        const frameDocument = document.implementation.createHTMLDocument("Isolated Flow boot");
        const frameWindow: Record<string, unknown> = {
          setTimeout,
          clearTimeout,
          __ANALYTIX_VIZ_SHELL__: {
            commands: {},
            getState: () => ({}),
            getPerfSnapshot: () => ({}),
          },
        };
        if (failure === "before-load") {
          Object.defineProperty(frameWindow, "__ANALYTIX_ORDINARY_PII_PROJECTION__", {
            configurable: true,
            get: () => undefined,
            set: () => {},
          });
        }
        Object.defineProperty(frame, "contentWindow", { value: frameWindow });
        Object.defineProperty(frame, "contentDocument", { value: frameDocument });
        const close = frameDocument.close.bind(frameDocument);
        vi.spyOn(frameDocument, "close").mockImplementation(() => {
          close();
          expect(frame.isConnected).toBe(true);
          const appendChild = frameDocument.body.appendChild.bind(frameDocument.body);
          vi.spyOn(frameDocument.body, "appendChild").mockImplementation(<T extends Node>(node: T): T => {
            appendChild(node);
            const script = node as unknown as HTMLScriptElement;
            loadedProjections.push(frameWindow.__ANALYTIX_ORDINARY_PII_PROJECTION__);
            if (failure === "replaced-after-load") {
              frameWindow.__ANALYTIX_ORDINARY_PII_PROJECTION__ = {
                ...ordinaryPiiProjection,
                projectDetected: (value: unknown) => String(value ?? ""),
              };
            } else {
              delete frameWindow.__ANALYTIX_ORDINARY_PII_PROJECTION__;
            }
            frameWindow.__ANALYTIX_FLOW_RUNTIME_BOOT_PROMISE__ = Promise.resolve();
            queueMicrotask(() => script.onload?.(new Event("load")));
            return node;
          });
        });
        return frame;
      });

      expect(applyDesktopBackendRuntimeState({
        phase: "running",
        generation: 1,
        launchId: LAUNCH_ID,
        apiBase: "http://127.0.0.1:18731",
        wsBase: "ws://127.0.0.1:18731/ws/events",
      })).toBe(true);
      const adapter = await ensureFlowCanvasAdapter("http://127.0.0.1:18731");
      try {
        await expect(adapter.attach(container)).rejects.toThrow("ordinary PII host projection missing");
        // attach has one existing retry; neither attempt may retain a frame or shell.
        expect(frames).toHaveLength(2);
        expect(frames.every(frame => !frame.isConnected)).toBe(true);
        expect(container.querySelector("iframe")).toBeNull();
        expect(adapter.getShellBridge()).toBeNull();
        expect(Object.values(adapter.getShellMounts()).every(value => value === null)).toBe(true);
        expect(loadedProjections).toHaveLength(failure === "before-load" ? 0 : 2);
        loadedProjections.forEach(projection => expect(projection).toBe(ordinaryPiiProjection));
        expect(fetch).not.toHaveBeenCalled();
        expect(WebSocket).not.toHaveBeenCalled();
      } finally {
        adapter.detach(container);
        container.remove();
      }
    },
  );
});
