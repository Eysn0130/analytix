import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { HttpClient } from "./client";
import { clearRuntimeBaseUrls } from "../runtime-base";
import {
  applyDesktopBackendRuntimeState,
  clearDesktopBackendRuntimeCacheForTests,
} from "../desktop/client";

const LAUNCH_ID = "H".repeat(43);

function runningState(generation: number, port = 18731) {
  return {
    phase: "running" as const,
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

function jsonResponse(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), {
    status,
    headers: {
      "Content-Type": "application/json"
    }
  });
}

beforeEach(() => {
  stubDesktopBridge(async () => runningState(1));
  expect(applyDesktopBackendRuntimeState(runningState(1))).toBe(true);
});

afterEach(() => {
  clearDesktopBackendRuntimeCacheForTests();
  clearRuntimeBaseUrls();
  vi.unstubAllGlobals();
});

describe("HttpClient", () => {
  it("keeps concurrent identical GET requests request-local", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({
        request_id: "req_1",
        timestamp: "2026-06-30T00:00:00.000Z",
        data: { value: 41 }
      }))
      .mockResolvedValueOnce(jsonResponse({
        request_id: "req_2",
        timestamp: "2026-06-30T00:00:01.000Z",
        data: { value: 42 }
      }));
    vi.stubGlobal("fetch", fetch);

    const client = new HttpClient("http://127.0.0.1:18731");
    const first = client.get<{ value: number }>("/api/demo");
    const second = client.get<{ value: number }>("/api/demo");

    await expect(Promise.all([first, second])).resolves.toEqual([
      {
        request_id: "req_1",
        timestamp: "2026-06-30T00:00:00.000Z",
        data: { value: 41 }
      },
      {
        request_id: "req_2",
        timestamp: "2026-06-30T00:00:01.000Z",
        data: { value: 42 }
      }
    ]);
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it("keeps a failed GET isolated from the next request", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({
        request_id: "req_bad",
        timestamp: "2026-06-30T00:00:00.000Z",
        error: {
          code: "BROKEN",
          message: "temporary failure",
          retryable: true
        }
      }, 500))
      .mockResolvedValueOnce(jsonResponse({
        request_id: "req_ok",
        timestamp: "2026-06-30T00:00:01.000Z",
        data: { ok: true }
      }));
    vi.stubGlobal("fetch", fetch);

    const client = new HttpClient("http://127.0.0.1:18731");

    await expect(client.get<{ ok: boolean }>("/api/demo")).rejects.toThrow("temporary failure");
    await expect(client.get<{ ok: boolean }>("/api/demo")).resolves.toMatchObject({
      data: { ok: true }
    });
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it("aborts in-flight requests when the backend generation is revoked", async () => {
    stubDesktopBridge(async () => runningState(1));
    expect(applyDesktopBackendRuntimeState(runningState(1))).toBe(true);
    const fetch = vi.fn(async (_input: unknown, init?: RequestInit) => new Promise<Response>((_resolve, reject) => {
      const signal = init?.signal;
      if (signal?.aborted) {
        reject(new DOMException("aborted", "AbortError"));
        return;
      }
      signal?.addEventListener(
        "abort",
        () => reject(new DOMException("aborted", "AbortError")),
        { once: true }
      );
    }));
    vi.stubGlobal("fetch", fetch);
    const client = new HttpClient("http://127.0.0.1:18731");

    const request = client.get("/api/demo");
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
    expect(applyDesktopBackendRuntimeState({ phase: "stopped", generation: 1 })).toBe(false);

    await expect(request).rejects.toMatchObject({ name: "AbortError" });
  });

  it("ElectronHttpWithoutLeaseNeverFetches", async () => {
    clearDesktopBackendRuntimeCacheForTests();
    stubDesktopBridge(async () => ({ phase: "starting", generation: 1 }));
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);

    await expect(new HttpClient().get("/api/demo")).rejects.toThrow(
      "数据分析后端服务未就绪",
    );
    expect(fetch).not.toHaveBeenCalled();
  });

  it("BrowserFileHttpWithoutExplicitNeverFetches", async () => {
    vi.stubGlobal("window", {
      location: { protocol: "file:", origin: "null", href: "file:///tmp/analytix.html" },
    });
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);

    await expect(new HttpClient().get("/api/demo")).rejects.toThrow(
      "data_analysis_backend_authority_missing",
    );
    expect(fetch).not.toHaveBeenCalled();
  });

  it("BrowserHttpAndExplicitEndpointNeverFetch", async () => {
    vi.stubGlobal("window", {
      location: {
        protocol: "https:",
        origin: "https://analytix.example",
        href: "https://analytix.example/workbench",
      },
    });
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);

    await expect(new HttpClient("https://analytix.example").post(
      "/api/v1/analysis/stats/v2/meta",
      { case_id: "case-secret" },
    )).rejects.toThrow("data_analysis_backend_authority_missing");
    expect(fetch).not.toHaveBeenCalled();
  });

  it("ExplicitHttpClientCannotSendAfterTerminalOrReusedPort", async () => {
    let backendState: Record<string, unknown> = runningState(1);
    stubDesktopBridge(async () => backendState);
    expect(applyDesktopBackendRuntimeState(runningState(1))).toBe(true);
    const client = new HttpClient("http://127.0.0.1:18731");
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);

    backendState = { phase: "stopped", generation: 1 };
    expect(applyDesktopBackendRuntimeState(backendState)).toBe(false);
    await expect(client.post("/api/demo", { case_id: "case-a" })).rejects.toThrow(
      "数据分析后端服务未就绪",
    );
    expect(fetch).not.toHaveBeenCalled();

    backendState = runningState(2);
    expect(applyDesktopBackendRuntimeState(backendState)).toBe(true);
    await expect(client.post("/api/demo", { case_id: "case-a" })).rejects.toThrow(
      "data_analysis_backend_generation_stale",
    );
    expect(fetch).not.toHaveBeenCalled();
  });

  it("forces no-store and rejects redirects even when the caller asks to follow", async () => {
    const fetch = vi.fn(async () => jsonResponse({
      request_id: "req_safe",
      timestamp: "2026-07-14T00:00:00.000Z",
      data: { ok: true },
    }));
    vi.stubGlobal("fetch", fetch);
    const client = new HttpClient("http://127.0.0.1:18731");

    await client.get("/api/demo", { cache: "force-cache", redirect: "follow" });

    expect(fetch).toHaveBeenCalledWith(
      "http://127.0.0.1:18731/api/demo",
      expect.objectContaining({ cache: "no-store", redirect: "error" }),
    );
  });

  it("rejects a late response body after its backend authority is revoked", async () => {
    stubDesktopBridge(async () => runningState(1));
    expect(applyDesktopBackendRuntimeState(runningState(1))).toBe(true);
    let resolveText!: (value: string) => void;
    const textPromise = new Promise<string>((resolve) => {
      resolveText = resolve;
    });
    const response = {
      ok: true,
      status: 200,
      statusText: "OK",
      headers: new Headers({ "Content-Type": "application/json" }),
      text: vi.fn(async () => textPromise),
    } as unknown as Response;
    vi.stubGlobal("fetch", vi.fn(async () => response));
    const client = new HttpClient("http://127.0.0.1:18731");

    const request = client.get("/api/demo");
    await vi.waitFor(() => expect(response.text).toHaveBeenCalledTimes(1));
    expect(applyDesktopBackendRuntimeState({ phase: "stopped", generation: 1 })).toBe(false);
    resolveText(JSON.stringify({ request_id: "late", timestamp: "", data: { case_id: "case-a" } }));

    await expect(request).rejects.toThrow("data_analysis_backend_generation_stale");
  });
});
