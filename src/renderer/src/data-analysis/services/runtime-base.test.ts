import { afterEach, describe, expect, it, vi } from "vitest";
import flowBridgeSource from "../features/flow/graph/api/flow-bridge-service.ts?raw";
import embeddedFlowSource from "../features/flow/graph/runtime/embedded-flow-runtime.ts?raw";
import graphPageSource from "../features/flow/graph/GraphPage.tsx?raw";
import httpClientSource from "./http/client.ts?raw";
import httpSystemSource from "./http/system.ts?raw";
import runtimeBaseSource from "./runtime-base.ts?raw";
import sharedWsSource from "./ws/shared-runtime.ts?raw";
import wsClientSource from "./ws/ws-client.ts?raw";
import {
  activateRuntimeBaseUrls,
  clearRuntimeBaseUrls,
  getActiveDataAnalysisRuntimeBaseLease,
  hasDesktopDataAnalysisBridge,
  resolveDataAnalysisRuntimeEndpointAuthority,
} from "./runtime-base";

function stubBrowser(
  href: string,
  options: { bridge?: "desktop" | "browser-preview"; forgedApiBase?: string; forgedWsBase?: string } = {},
): void {
  const url = new URL(href);
  const dataset: Record<string, string> = {};
  if (options.bridge === "browser-preview") {
    dataset.bridge = "browser-preview";
  }
  vi.stubGlobal("document", {
    documentElement: { dataset },
  });
  vi.stubGlobal("window", {
    location: {
      href: url.toString(),
      origin: url.origin,
      protocol: url.protocol,
    },
    analytix: options.bridge
      ? { dataAnalysis: {} }
      : undefined,
    __ANALYTIX_API_BASE__: options.forgedApiBase,
    __ANALYTIX_WS_BASE__: options.forgedWsBase,
  });
}

afterEach(() => {
  clearRuntimeBaseUrls();
  vi.unstubAllGlobals();
});

describe("data-analysis runtime endpoint authority", () => {
  it("DataAnalysisEndpointAuthorityIsDesktopLeaseOnly", () => {
    const endpointSources = [
      runtimeBaseSource,
      httpClientSource,
      httpSystemSource,
      sharedWsSource,
      wsClientSource,
      graphPageSource,
      flowBridgeSource,
      embeddedFlowSource,
    ];
    for (const source of endpointSources) {
      expect(source).not.toContain("browser_explicit");
      expect(source).not.toContain("browser_same_origin");
      expect(source).not.toContain("VITE_API_BASE_URL");
      expect(source).not.toContain("VITE_WS_BASE_URL");
    }
  });

  it("RuntimeBaseDoesNotFallBackToFixedLoopback", () => {
    stubBrowser("file:///tmp/analytix.html");

    expect(() => resolveDataAnalysisRuntimeEndpointAuthority()).toThrow(
      "data_analysis_backend_authority_missing",
    );
    expect(runtimeBaseSource).not.toContain("127.0.0.1:18731");
  });

  it("ForgedWindowRuntimeBaseDoesNotCreateAuthority", () => {
    stubBrowser("file:///tmp/analytix.html", {
      forgedApiBase: "http://127.0.0.1:18731",
      forgedWsBase: "ws://127.0.0.1:18731/ws/events",
    });

    expect(() => resolveDataAnalysisRuntimeEndpointAuthority()).toThrow(
      "data_analysis_backend_authority_missing",
    );
  });

  it("BrowserHttpOriginCannotCreateDataAnalysisAuthority", () => {
    stubBrowser("https://analytix.example/workbench");

    expect(() => resolveDataAnalysisRuntimeEndpointAuthority()).toThrow(
      "data_analysis_backend_authority_missing",
    );
  });

  it("BrowserCallerEndpointsCannotCreateDataAnalysisAuthority", () => {
    stubBrowser("https://analytix.example/workbench");

    expect(() => resolveDataAnalysisRuntimeEndpointAuthority({
      requestedApiBaseUrl: "https://analytix.example",
      requestedWsBaseUrl: "wss://analytix.example/ws/events",
    })).toThrow("data_analysis_backend_authority_missing");
    expect(() => resolveDataAnalysisRuntimeEndpointAuthority({
      requestedApiBaseUrl: "http://127.0.0.1:18731",
      requestedWsBaseUrl: "ws://127.0.0.1:18731/ws/events",
    })).toThrow("data_analysis_backend_authority_missing");
  });

  it("BrowserPreviewStubIsNotDesktopAuthority", () => {
    stubBrowser("http://localhost:5173", { bridge: "browser-preview" });

    expect(hasDesktopDataAnalysisBridge()).toBe(false);
    expect(() => resolveDataAnalysisRuntimeEndpointAuthority()).toThrow(
      "data_analysis_browser_preview_unavailable",
    );
  });

  it("ElectronEndpointRequiresCurrentHostLease", () => {
    stubBrowser("file:///Applications/analytix/index.html", { bridge: "desktop" });

    expect(() => resolveDataAnalysisRuntimeEndpointAuthority()).toThrow(
      "data_analysis_backend_authority_missing",
    );
    expect(activateRuntimeBaseUrls(
      "http://127.0.0.1:18731",
      "ws://127.0.0.1:18731/ws/events",
      7,
      "7:launch",
    )).toMatchObject({ generation: 7 });
    expect(resolveDataAnalysisRuntimeEndpointAuthority()).toMatchObject({
      kind: "desktop_lease",
      apiBaseUrl: "http://127.0.0.1:18731",
      wsBaseUrl: "ws://127.0.0.1:18731/ws/events",
      authorityId: "7:launch",
    });
  });

  it("InvalidDesktopEndpointCannotCreateLease", () => {
    expect(activateRuntimeBaseUrls(
      "http://attacker.example:18731",
      "ws://attacker.example:18731/ws/events",
      1,
      "1:launch",
    )).toMatchObject({ generation: 0 });
    expect(getActiveDataAnalysisRuntimeBaseLease()).toBeNull();
  });
});
