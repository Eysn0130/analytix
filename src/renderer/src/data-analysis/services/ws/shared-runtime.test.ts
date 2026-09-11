import { afterEach, describe, expect, it, vi } from "vitest";
import dataAnalysisSurfaceSource from "../../DataAnalysisSurface.tsx?raw";
import { getSharedWsRuntime, invalidateSharedWsRuntimes } from "./shared-runtime";
import {
  applyDesktopBackendRuntimeState,
  clearDesktopBackendRuntimeCacheForTests,
} from "../desktop/client";
import { createEmbeddedFlowBridgeService } from "../../features/flow/graph/api/flow-bridge-service";

const LAUNCH_ID = "W".repeat(43);

function runningState(generation: number, port = 18731) {
  return {
    phase: "running" as const,
    generation,
    launchId: LAUNCH_ID,
    apiBase: `http://127.0.0.1:${port}`,
    wsBase: `ws://127.0.0.1:${port}/ws/events`,
  };
}

function stubDesktopAuthority(generation = 1, port = 18731): void {
  vi.stubGlobal("window", {
    analytix: { dataAnalysis: {} },
    setTimeout: vi.fn(() => 1),
    clearTimeout: vi.fn(),
  });
  expect(applyDesktopBackendRuntimeState(runningState(generation, port))).toBe(true);
}

function runtimeEvent(caseId: string, event = "import.job.completed"): Record<string, unknown> {
  return {
    version: "v1",
    event,
    type: "success",
    channel: event.startsWith("import.") ? "import" : "analysis",
    case_id: caseId,
    sequence: 1,
    payload: { event_id: `${caseId}-event` },
    timestamp: "2026-07-17T00:00:00.000Z",
  };
}

function emitTransportEvent(runtime: ReturnType<typeof getSharedWsRuntime>, event: unknown): void {
  const client = (runtime as unknown as {
    client: { eventListeners: Set<(value: unknown) => void> };
  }).client;
  client.eventListeners.forEach((listener) => listener(event));
}

function sentControlRequests(socket: FakeWebSocket): Array<Record<string, unknown>> {
  return socket.send.mock.calls.map(([payload]) => JSON.parse(String(payload)) as Record<string, unknown>);
}

function captureWindowTimerCallbacks(): Array<() => void> {
  const callbacks: Array<() => void> = [];
  vi.mocked(window.setTimeout).mockImplementation((handler: TimerHandler) => {
    if (typeof handler === "function") {
      callbacks.push(() => handler());
    }
    return callbacks.length;
  });
  vi.mocked(window.clearTimeout).mockImplementation(() => undefined);
  return callbacks;
}

function emitRuntimeStatusResponse(
  socket: FakeWebSocket,
  options: {
    requestId: string;
    sessionId: string;
    checkpointVersion: number;
    caseId?: string;
    rawCanary?: string;
  },
): void {
  socket.emit("message", {
    data: JSON.stringify({
      event: "llm.runtime.control.response",
      type: "info",
      case_id: options.caseId ?? "case-a",
      payload: {
        request_id: options.requestId,
        data: {
          session_status: {
            session_id: options.sessionId,
            status: { state: "running", raw: options.rawCanary },
            thread: { id: options.rawCanary ?? "thread-safe" },
            session: { title: options.rawCanary ?? "session-safe" },
            replay: { checkpoint_version: options.checkpointVersion, raw: options.rawCanary },
            runtime_state: { body: options.rawCanary ?? "runtime-safe" },
          },
        },
      },
    }),
  });
}

class FakeWebSocket {
  static readonly OPEN = 1;
  static readonly instances: FakeWebSocket[] = [];

  readonly close = vi.fn();
  readonly send = vi.fn();
  readonly url: string;
  readyState = FakeWebSocket.OPEN;
  private readonly listeners = new Map<string, Array<(event: { data?: unknown }) => void>>();

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }

  addEventListener(name: string, listener: (event: { data?: unknown }) => void): void {
    const listeners = this.listeners.get(name) ?? [];
    listeners.push(listener);
    this.listeners.set(name, listeners);
  }

  emit(name: string, event: { data?: unknown } = {}): void {
    this.listeners.get(name)?.forEach((listener) => listener(event));
  }
}

afterEach(() => {
  clearDesktopBackendRuntimeCacheForTests();
  invalidateSharedWsRuntimes();
  FakeWebSocket.instances.splice(0);
  vi.unstubAllGlobals();
});

describe("shared data-analysis WebSocket runtime", () => {
  it("disconnects and retires every socket from a revoked backend generation", async () => {
    stubDesktopAuthority();
    vi.stubGlobal("WebSocket", FakeWebSocket);
    const runtime = getSharedWsRuntime({
      apiBaseUrl: "http://127.0.0.1:18731",
      wsBaseUrl: "ws://127.0.0.1:18731/ws/events",
      caseId: "case-a",
    });
    const release = runtime.retain();

    invalidateSharedWsRuntimes();

    expect(FakeWebSocket.instances[0]?.close).toHaveBeenCalledWith(1000, "manual disconnect");
    expect(runtime.getSnapshot()).toMatchObject({
      status: "closed",
      retainCount: 0,
    });
    await expect(
      runtime.requestControl({
        request_id: "req_1",
        type: "llm.control_request",
        action: "runtime_status",
      }),
    ).rejects.toThrow("runtime_control_generation_invalidated");
    expect(getSharedWsRuntime({
      apiBaseUrl: "http://127.0.0.1:18731",
      wsBaseUrl: "ws://127.0.0.1:18731/ws/events",
      caseId: "case-a",
    })).not.toBe(runtime);
    release();
  });

  it("BrowserFileWsWithoutExplicitNeverConstructsSocket", () => {
    vi.stubGlobal("window", {
      location: { protocol: "file:", origin: "null", href: "file:///tmp/analytix.html" },
    });
    vi.stubGlobal("WebSocket", FakeWebSocket);

    expect(() => getSharedWsRuntime()).toThrow("data_analysis_backend_authority_missing");
    expect(FakeWebSocket.instances).toHaveLength(0);
  });

  it("BrowserSameOriginNeverConstructsSocket", () => {
    vi.stubGlobal("window", {
      location: {
        protocol: "https:",
        origin: "https://analytix.example",
        href: "https://analytix.example/workbench",
      },
    });
    vi.stubGlobal("WebSocket", FakeWebSocket);

    expect(() => getSharedWsRuntime({ caseId: "case-a" })).toThrow(
      "data_analysis_backend_authority_missing",
    );
    expect(FakeWebSocket.instances).toHaveLength(0);
  });

  it("FlowWsCannotConnectBeforeAuthenticatedGeneration", () => {
    vi.stubGlobal("window", {
      analytix: { dataAnalysis: {} },
    });
    vi.stubGlobal("WebSocket", FakeWebSocket);

    expect(() => getSharedWsRuntime({
      wsBaseUrl: "ws://127.0.0.1:18731/ws/events",
      caseId: "case-a",
    })).toThrow("data_analysis_backend_authority_missing");
    expect(FakeWebSocket.instances).toHaveLength(0);
  });

  it("invalidates the old socket authority across a same-port relaunch", () => {
    vi.stubGlobal("window", {
      analytix: { dataAnalysis: {} },
    });
    vi.stubGlobal("WebSocket", FakeWebSocket);
    expect(applyDesktopBackendRuntimeState(runningState(1))).toBe(true);
    const oldRuntime = getSharedWsRuntime({ caseId: "case-a" });
    oldRuntime.retain();
    expect(FakeWebSocket.instances).toHaveLength(1);

    expect(applyDesktopBackendRuntimeState({ phase: "stopped", generation: 1 })).toBe(false);
    expect(applyDesktopBackendRuntimeState(runningState(2))).toBe(true);
    expect(() => oldRuntime.retain({ caseId: "case-a" })).toThrow(
      "runtime_control_generation_invalidated",
    );
    const newRuntime = getSharedWsRuntime({ caseId: "case-a" });
    expect(newRuntime).not.toBe(oldRuntime);
    newRuntime.retain();
    expect(FakeWebSocket.instances).toHaveLength(2);
  });

  it("rejects a late socket open after generation invalidation and never puts credentials in the URL", () => {
    vi.stubGlobal("window", {
      analytix: { dataAnalysis: {} },
    });
    vi.stubGlobal("WebSocket", FakeWebSocket);
    expect(applyDesktopBackendRuntimeState(runningState(1))).toBe(true);
    const runtime = getSharedWsRuntime({ caseId: "case-a" });
    runtime.retain();
    const socket = FakeWebSocket.instances[0]!;
    expect(socket.url).toContain("case_id=case-a");
    expect(socket.url).not.toContain("auth");
    expect(socket.url).not.toContain("token");

    expect(applyDesktopBackendRuntimeState({ phase: "failed", generation: 1 })).toBe(false);
    socket.emit("open");

    expect(socket.close).toHaveBeenCalledWith(1008, "runtime authority revoked");
    expect(runtime.getSnapshot()).toMatchObject({ openCount: 0, status: "closed" });
  });

  it("rejects cross-case events before state, control handling, or listener delivery", () => {
    stubDesktopAuthority();
    vi.stubGlobal("WebSocket", FakeWebSocket);
    const runtime = getSharedWsRuntime({ caseId: "case-a" });
    const listener = vi.fn();
    runtime.onEvent(listener);
    runtime.retain();

    FakeWebSocket.instances[0]?.emit("message", {
      data: JSON.stringify(runtimeEvent("case-b")),
    });
    FakeWebSocket.instances[0]?.emit("message", {
      data: JSON.stringify(runtimeEvent("")),
    });

    expect(runtime.getLastEvent()).toBeNull();
    expect(runtime.getSnapshot()).toMatchObject({
      caseId: "case-a",
      eventCount: 0,
      lastEventName: "",
      lastEventCaseId: "",
    });
    expect(listener).not.toHaveBeenCalled();

    FakeWebSocket.instances[0]?.emit("message", {
      data: JSON.stringify(runtimeEvent("case-a")),
    });
    expect(runtime.getSnapshot()).toMatchObject({
      eventCount: 1,
      lastEventName: "import.job.completed",
      lastEventCaseId: "case-a",
    });
    expect(listener).toHaveBeenCalledTimes(1);
  });

  it("projects before generic cache, listener, getLastEvent, and emitLast boundaries", () => {
    stubDesktopAuthority();
    vi.stubGlobal("WebSocket", FakeWebSocket);
    const runtime = getSharedWsRuntime({ caseId: "case-a" });
    const listener = vi.fn();
    runtime.onEvent(listener);
    runtime.retain();

    const account = "6222020202020202020";
    const phone = "13800138000";
    const authorityRef = `cer1_${"a".repeat(64)}`;
    const localPath = "/Users/synthetic/private/funds.duckdb";
    let hostileGetterCalls = 0;
    const payloadTarget = Object.create(null) as Record<string, unknown>;
    Object.defineProperties(payloadTarget, {
      progress: { enumerable: true, value: 44 },
      message: { enumerable: true, value: account },
      path: { enumerable: true, value: localPath },
      rows: { enumerable: true, value: [{ account, phone }] },
      authority_ref: { enumerable: true, value: authorityRef },
      details: {
        enumerable: true,
        get() {
          hostileGetterCalls += 1;
          throw new Error(phone);
        },
      },
      toJSON: {
        enumerable: false,
        get() {
          hostileGetterCalls += 1;
          throw new Error("raw toJSON must not run");
        },
      },
    });
    const payload = new Proxy(payloadTarget, {
      get() {
        throw new Error("raw payload value access is forbidden");
      },
      ownKeys() {
        throw new Error("raw payload enumeration is forbidden");
      },
    });

    emitTransportEvent(runtime, {
      version: "v1",
      event: "import.job.progress",
      type: "progress",
      channel: "import",
      job_id: "123e4567-e89b-42d3-a456-426614174000",
      case_id: "case-a",
      sequence: 7,
      payload,
      timestamp: "2026-08-23T02:03:04Z",
    });

    const cached = runtime.getLastEvent();
    const emitLastListener = vi.fn();
    runtime.onEvent(emitLastListener, { emitLast: true });
    const listenerEvent = listener.mock.calls[0]?.[0];
    const replayedEvent = emitLastListener.mock.calls[0]?.[0];
    for (const value of [cached, listenerEvent, replayedEvent]) {
      const serialized = JSON.stringify(value);
      expect(serialized).toContain("import.job.progress");
      expect(serialized).toContain('"progress":44');
      for (const canary of [account, phone, authorityRef, localPath]) {
        expect(serialized).not.toContain(canary);
      }
    }
    expect(listener).toHaveBeenCalledTimes(1);
    expect(emitLastListener).toHaveBeenCalledTimes(1);
    expect(replayedEvent).toBe(cached);
    expect(hostileGetterCalls).toBe(0);

    emitTransportEvent(runtime, {
      version: "v1",
      event: "unknown.private.event",
      type: "info",
      channel: "system",
      case_id: "case-a",
      sequence: 8,
      get payload() {
        hostileGetterCalls += 1;
        throw new Error(authorityRef);
      },
      timestamp: "2026-08-23T02:03:05Z",
    });
    expect(listener).toHaveBeenCalledTimes(1);
    expect(runtime.getLastEvent()).toBe(cached);
    expect(hostileGetterCalls).toBe(0);

    invalidateSharedWsRuntimes();
    expect(runtime.getLastEvent()).toBeNull();
    expect(runtime.getSnapshot()).toMatchObject({
      eventListenerCount: 0,
      lastEventName: "",
      lastEventAt: 0,
      lastEventCaseId: "",
    });
  });

  it("keeps only enrolled exact-session control status private and restores it on reconnect", async () => {
    stubDesktopAuthority();
    vi.stubGlobal("WebSocket", FakeWebSocket);
    const runtime = getSharedWsRuntime({ caseId: "case-a" });
    const listener = vi.fn();
    runtime.onEvent(listener);
    runtime.retain();
    const socket = FakeWebSocket.instances[0]!;
    socket.emit("open");

    const account = "6222020202020202020";
    const releaseSession = runtime.retainRuntimeSession({
      caseId: "case-a",
      sessionId: "session-1",
    });
    const enrollmentRequest = sentControlRequests(socket).at(-1)!;
    emitRuntimeStatusResponse(socket, {
      requestId: String(enrollmentRequest.request_id),
      sessionId: "session-1",
      checkpointVersion: 7,
      rawCanary: account,
    });

    const cachedStatus = runtime.getRuntimeSessionStatus("session-1");
    expect(cachedStatus).toMatchObject({
      contract: "SafeRuntimeSessionStatusV1",
      session_id: "session-1",
      replay: { checkpoint_version: 7 },
    });
    expect(JSON.stringify(cachedStatus)).not.toContain(account);
    expect(listener).not.toHaveBeenCalled();
    expect(runtime.getLastEvent()).toBeNull();
    expect(runtime.getSnapshot()).toMatchObject({
      eventCount: 0,
      lastEventName: "",
      lastEventCaseId: "",
    });

    socket.emit("message", {
      data: JSON.stringify({
        event: "llm.runtime.session.updated",
        type: "info",
        case_id: "case-a",
        payload: {
          session_id: "session-2",
          status: { state: "idle" },
          thread: { id: account },
          session: { title: account },
          replay: { checkpoint_version: 8 },
          runtime_state: { body: account },
        },
      }),
    });
    expect(runtime.getRuntimeSessionStatus("session-2")).toBeNull();

    socket.emit("message", {
      data: JSON.stringify({
        event: "llm.runtime.session.updated",
        type: "info",
        case_id: "case-a",
        payload: {
          session_id: "session-1",
          status: { state: "idle" },
          thread: { id: account },
          session: { title: account },
          replay: { checkpoint_version: 8 },
          runtime_state: { body: account },
        },
      }),
    });
    expect(runtime.getRuntimeSessionStatus("session-1")).toMatchObject({
      session_id: "session-1",
      replay: { checkpoint_version: 8 },
    });
    expect(listener).not.toHaveBeenCalled();
    expect(runtime.getLastEvent()).toBeNull();

    await Promise.resolve();
    await Promise.resolve();
    socket.emit("open");
    const reconnectRequest = sentControlRequests(socket).at(-1)!;
    expect(reconnectRequest).toMatchObject({
      action: "runtime_status",
      session_id: "session-1",
      transport: { reason: "reconnect", checkpoint_version: 8 },
    });
    emitRuntimeStatusResponse(socket, {
      requestId: String(reconnectRequest.request_id),
      sessionId: "session-1",
      checkpointVersion: 9,
      rawCanary: account,
    });
    expect(runtime.getRuntimeSessionStatus("session-1")).toMatchObject({
      replay: { checkpoint_version: 9 },
    });

    expect(applyDesktopBackendRuntimeState({ phase: "stopped", generation: 1 })).toBe(false);
    expect(runtime.getRuntimeSessionStatus("session-1")).toBeNull();
    expect(runtime.getSnapshot()).toMatchObject({ status: "closed" });

    releaseSession();
    invalidateSharedWsRuntimes();
    expect(runtime.getRuntimeSessionStatus("session-1")).toBeNull();
    expect(runtime.getRuntimeSessionStatus("session-2")).toBeNull();
  });

  it("rejects private-reference and numeric session identities at every cache entry", async () => {
    stubDesktopAuthority();
    vi.stubGlobal("WebSocket", FakeWebSocket);
    const runtime = getSharedWsRuntime({ caseId: "case-a" });
    const listener = vi.fn();
    runtime.onEvent(listener);
    runtime.retain();
    const socket = FakeWebSocket.instances[0]!;
    socket.emit("open");

    const hostileSessionIds = [
      `cer1_${"a".repeat(64)}`,
      `srow1_${"b".repeat(64)}`,
      "6222020202020202020",
      "13800138000",
    ];
    for (const [index, sessionId] of hostileSessionIds.entries()) {
      if (sessionId.startsWith("cer1_") || sessionId.startsWith("srow1_")) {
        expect(() => getSharedWsRuntime({ caseId: sessionId })).toThrow(
          "runtime_control_case_binding_invalid",
        );
      }
      expect(() => runtime.retainRuntimeSession({ caseId: "case-a", sessionId })).toThrow(
        "runtime_control_session_id_invalid",
      );
      expect(runtime.getRuntimeSessionStatus(sessionId)).toBeNull();
      await expect(runtime.requestControl({
        request_id: `request-hostile-${index}`,
        type: "llm.control_request",
        action: "runtime_status",
        session_id: sessionId,
      })).rejects.toThrow("runtime_control_session_id_invalid");

      emitTransportEvent(runtime, {
        event: "llm.runtime.session.updated",
        type: "info",
        case_id: "case-a",
        payload: {
          session_id: sessionId,
          status: { state: "idle" },
          thread: { available: true },
          session: { available: true },
          replay: { checkpoint_version: 11 },
          runtime_state: { available: true },
        },
      });
      expect(runtime.getRuntimeSessionStatus(sessionId)).toBeNull();
    }

    let hostileAccessorCalls = 0;
    const payloadTarget = Object.create(null) as Record<string, unknown>;
    Object.defineProperties(payloadTarget, {
      session_id: { enumerable: true, value: hostileSessionIds[0] },
      status: {
        enumerable: true,
        get() {
          hostileAccessorCalls += 1;
          throw new Error("raw status getter must not run");
        },
      },
      toJSON: {
        enumerable: false,
        get() {
          hostileAccessorCalls += 1;
          throw new Error("raw toJSON getter must not run");
        },
      },
    });
    const payload = new Proxy(payloadTarget, {
      get() {
        hostileAccessorCalls += 1;
        throw new Error("raw control value access must not run");
      },
      ownKeys() {
        hostileAccessorCalls += 1;
        throw new Error("raw control enumeration must not run");
      },
    });
    emitTransportEvent(runtime, {
      event: "llm.runtime.session.updated",
      type: "info",
      case_id: "case-a",
      payload,
    });

    expect(socket.send).not.toHaveBeenCalled();
    expect(listener).not.toHaveBeenCalled();
    expect(runtime.getLastEvent()).toBeNull();
    expect(JSON.stringify(runtime.getSnapshot())).not.toContain("6222020202020202020");
    expect(JSON.stringify(runtime.getSnapshot())).not.toContain("13800138000");
    expect(hostileAccessorCalls).toBe(0);
  });

  it("rejects a duplicate pending request id without replacing or resending the first request", async () => {
    stubDesktopAuthority();
    vi.stubGlobal("WebSocket", FakeWebSocket);
    const timerCallbacks = captureWindowTimerCallbacks();
    const runtime = getSharedWsRuntime({ caseId: "case-a" });
    runtime.retain();
    const socket = FakeWebSocket.instances[0]!;
    socket.emit("open");

    const firstOutcome = vi.fn();
    const duplicateOutcome = vi.fn();
    const first = runtime.requestControl({
      request_id: "request-duplicate-1",
      type: "llm.control_request",
      action: "turn_steer",
    });
    void first.then(
      (value) => firstOutcome("resolved", value),
      (error: Error) => firstOutcome("rejected", error.message),
    );
    const duplicate = runtime.requestControl({
      request_id: "request-duplicate-1",
      type: "llm.control_request",
      action: "thread_compact",
    });
    void duplicate.then(
      (value) => duplicateOutcome("resolved", value),
      (error: Error) => duplicateOutcome("rejected", error.message),
    );

    await Promise.resolve();
    expect(duplicateOutcome).toHaveBeenCalledWith(
      "rejected",
      "runtime_control_request_id_duplicate",
    );
    expect(socket.send).toHaveBeenCalledTimes(1);
    expect(timerCallbacks).toHaveLength(1);

    socket.emit("message", {
      data: JSON.stringify({
        event: "llm.runtime.control.response",
        type: "info",
        case_id: "case-a",
        payload: { request_id: "request-duplicate-1", data: {} },
      }),
    });
    await Promise.resolve();
    expect(firstOutcome).toHaveBeenCalledWith("resolved", {});
    expect(duplicateOutcome).toHaveBeenCalledTimes(1);
  });

  it("does not let a stale timeout delete a later exact pending entry with the same id", async () => {
    stubDesktopAuthority();
    vi.stubGlobal("WebSocket", FakeWebSocket);
    const timerCallbacks = captureWindowTimerCallbacks();
    const runtime = getSharedWsRuntime({ caseId: "case-a" });
    runtime.retain();
    const socket = FakeWebSocket.instances[0]!;
    socket.emit("open");

    const first = runtime.requestControl({
      request_id: "request-aba-1",
      type: "llm.control_request",
      action: "turn_steer",
    });
    const firstOutcome = vi.fn();
    void first.then(
      (value) => firstOutcome("resolved", value),
      (error: Error) => firstOutcome("rejected", error.message),
    );
    const staleTimer = timerCallbacks[0]!;
    socket.emit("message", {
      data: JSON.stringify({
        event: "llm.runtime.control.response",
        type: "success",
        case_id: "case-a",
        payload: { request_id: "request-aba-1", data: {} },
      }),
    });
    await Promise.resolve();
    expect(firstOutcome).toHaveBeenCalledWith("resolved", {});

    const reusedOutcome = vi.fn();
    const reused = runtime.requestControl({
      request_id: "request-aba-1",
      type: "llm.control_request",
      action: "thread_compact",
    });
    void reused.then(
      (value) => reusedOutcome("resolved", value),
      (error: Error) => reusedOutcome("rejected", error.message),
    );
    expect(timerCallbacks).toHaveLength(2);
    staleTimer();
    socket.emit("message", {
      data: JSON.stringify({
        event: "llm.runtime.control.response",
        type: "info",
        case_id: "case-a",
        payload: { request_id: "request-aba-1", data: {} },
      }),
    });
    await Promise.resolve();

    expect(socket.send).toHaveBeenCalledTimes(2);
    expect(reusedOutcome).toHaveBeenCalledWith("resolved", {});
    expect(reusedOutcome).toHaveBeenCalledTimes(1);
  });

  it("rejects private-reference and numeric request ids before request or cancel transport", async () => {
    stubDesktopAuthority();
    vi.stubGlobal("WebSocket", FakeWebSocket);
    const runtime = getSharedWsRuntime({ caseId: "case-a" });
    const listener = vi.fn();
    runtime.onEvent(listener);
    runtime.retain();
    const socket = FakeWebSocket.instances[0]!;
    socket.emit("open");

    const hostileRequestIds = [
      `cer1_${"a".repeat(64)}`,
      `srow1_${"b".repeat(64)}`,
      "6222020202020202020",
      "13800138000",
    ];
    for (const requestId of hostileRequestIds) {
      const rejection = vi.fn();
      void runtime.requestControl({
        request_id: requestId,
        type: "llm.control_request",
        action: "turn_steer",
      }).catch((error: Error) => rejection(error.message));
      await Promise.resolve();
      expect(rejection).toHaveBeenCalledWith("runtime_control_request_id_invalid");
      expect(runtime.cancelControlRequest(requestId)).toBe(false);
    }

    expect(socket.send).not.toHaveBeenCalled();
    expect(listener).not.toHaveBeenCalled();
    expect(runtime.getLastEvent()).toBeNull();
    const serializedPublicState = JSON.stringify(runtime.getSnapshot());
    for (const requestId of hostileRequestIds) {
      expect(serializedPublicState).not.toContain(requestId);
    }

    expect(runtime.cancelControlRequest("request-cancel-1")).toBe(true);
    expect(sentControlRequests(socket)).toEqual([{
      type: "llm.control_cancel",
      request_id: "request-cancel-1",
      case_id: "case-a",
    }]);
  });

  it("binds runtime-status responses to the exact pending enrolled session and authority", async () => {
    stubDesktopAuthority();
    vi.stubGlobal("WebSocket", FakeWebSocket);
    const runtime = getSharedWsRuntime({ caseId: "case-a" });
    const listener = vi.fn();
    runtime.onEvent(listener);
    runtime.retain();
    const socket = FakeWebSocket.instances[0]!;
    socket.emit("open");
    runtime.retainRuntimeSession({ caseId: "case-a", sessionId: "session-a" });
    const enrollmentRequest = sentControlRequests(socket).at(-1)!;

    let unboundDataGetterCalls = 0;
    const unboundPayload = Object.create(null) as Record<string, unknown>;
    Object.defineProperties(unboundPayload, {
      request_id: { enumerable: true, value: "wrong-request-id" },
      data: {
        enumerable: true,
        get() {
          unboundDataGetterCalls += 1;
          throw new Error("unbound control data must not be read");
        },
      },
      toJSON: {
        enumerable: false,
        get() {
          unboundDataGetterCalls += 1;
          throw new Error("unbound control toJSON must not be read");
        },
      },
    });
    emitTransportEvent(runtime, {
      event: "llm.runtime.control.response",
      type: "info",
      case_id: "case-a",
      payload: new Proxy(unboundPayload, {
        get() {
          unboundDataGetterCalls += 1;
          throw new Error("unbound control value must not be read");
        },
        ownKeys() {
          unboundDataGetterCalls += 1;
          throw new Error("unbound control payload must not be enumerated");
        },
      }),
    });
    expect(unboundDataGetterCalls).toBe(0);

    emitRuntimeStatusResponse(socket, {
      requestId: "wrong-request-id",
      sessionId: "session-a",
      checkpointVersion: 31,
    });
    expect(runtime.getRuntimeSessionStatus("session-a")).toBeNull();

    emitRuntimeStatusResponse(socket, {
      requestId: String(enrollmentRequest.request_id),
      sessionId: "session-b",
      checkpointVersion: 32,
    });
    expect(runtime.getRuntimeSessionStatus("session-a")).toBeNull();
    expect(runtime.getRuntimeSessionStatus("session-b")).toBeNull();

    await Promise.resolve();
    await Promise.resolve();
    socket.emit("open");
    const unknownTypeRequest = sentControlRequests(socket).at(-1)!;
    socket.emit("message", {
      data: JSON.stringify({
        event: "llm.runtime.control.response",
        type: "trace",
        case_id: "case-a",
        payload: {
          request_id: unknownTypeRequest.request_id,
          data: {
            session_status: {
              session_id: "session-a",
              status: { state: "running" },
              thread: { available: true },
              session: { available: true },
              replay: { checkpoint_version: 33 },
              runtime_state: { available: true },
            },
          },
        },
      }),
    });
    expect(runtime.getRuntimeSessionStatus("session-a")).toBeNull();

    await Promise.resolve();
    await Promise.resolve();
    socket.emit("open");
    const reconnectRequest = sentControlRequests(socket).at(-1)!;
    emitRuntimeStatusResponse(socket, {
      requestId: String(reconnectRequest.request_id),
      sessionId: "session-a",
      checkpointVersion: 34,
      caseId: "case-b",
    });
    expect(runtime.getRuntimeSessionStatus("session-a")).toBeNull();

    expect(applyDesktopBackendRuntimeState({ phase: "stopped", generation: 1 })).toBe(false);
    emitRuntimeStatusResponse(socket, {
      requestId: String(reconnectRequest.request_id),
      sessionId: "session-a",
      checkpointVersion: 35,
    });
    expect(runtime.getRuntimeSessionStatus("session-a")).toBeNull();
    expect(listener).not.toHaveBeenCalled();
    expect(runtime.getLastEvent()).toBeNull();
  });

  it("isolates concurrent case consumers with immutable registry bindings", () => {
    stubDesktopAuthority();
    vi.stubGlobal("WebSocket", FakeWebSocket);

    const caseARuntime = getSharedWsRuntime({ caseId: "case-a" });
    const caseBRuntime = getSharedWsRuntime({ caseId: "case-b" });
    expect(caseARuntime).not.toBe(caseBRuntime);
    caseARuntime.retain();
    caseBRuntime.retain();

    expect(FakeWebSocket.instances.map((socket) => socket.url)).toEqual([
      "ws://127.0.0.1:18731/ws/events?case_id=case-a",
      "ws://127.0.0.1:18731/ws/events?case_id=case-b",
    ]);
    expect(() => caseARuntime.retain({ caseId: "case-b" })).toThrow(
      "runtime_control_case_binding_mismatch",
    );
    expect(() => caseARuntime.setCaseId("case-b")).toThrow(
      "runtime_control_case_binding_immutable",
    );
    expect(caseARuntime.getSnapshot()).toMatchObject({ caseId: "case-a", retainCount: 1 });
    expect(caseBRuntime.getSnapshot()).toMatchObject({ caseId: "case-b", retainCount: 1 });
  });

  it("does not create a socket without a non-empty frozen case binding", () => {
    stubDesktopAuthority();
    vi.stubGlobal("WebSocket", FakeWebSocket);

    expect(() => getSharedWsRuntime({ caseId: "  " })).toThrow(
      "runtime_control_case_binding_missing",
    );
    expect(FakeWebSocket.instances).toHaveLength(0);
  });

  it("Flow service delays transport until a case is selected and releases each frozen binding", () => {
    stubDesktopAuthority();
    vi.stubGlobal("WebSocket", FakeWebSocket);
    const service = createEmbeddedFlowBridgeService({
      apiBaseUrl: "http://127.0.0.1:18731",
    });

    expect(FakeWebSocket.instances).toHaveLength(0);
    service.setCaseId("");
    expect(FakeWebSocket.instances).toHaveLength(0);

    service.setCaseId("case-a");
    expect(FakeWebSocket.instances[0]?.url).toBe(
      "ws://127.0.0.1:18731/ws/events?case_id=case-a",
    );
    service.setCaseId("case-b");
    expect(FakeWebSocket.instances[0]?.close).toHaveBeenCalledWith(1000, "manual disconnect");
    expect(FakeWebSocket.instances[1]?.url).toBe(
      "ws://127.0.0.1:18731/ws/events?case_id=case-b",
    );

    service.dispose();
    service.dispose();
    expect(FakeWebSocket.instances[1]?.close).toHaveBeenCalledTimes(1);
  });

  it("Surface checks the current frozen case and projects an event before publishing it to the store", () => {
    const wsEffectStart = dataAnalysisSurfaceSource.indexOf("if (!backendRuntimeReady || !normalizedActiveCaseId)");
    const runtimeAcquire = dataAnalysisSurfaceSource.indexOf("getSharedWsRuntime({ caseId: frozenCaseId })", wsEffectStart);
    const listenerStart = dataAnalysisSurfaceSource.indexOf("const offEvent = runtime.onEvent");
    const caseGuard = dataAnalysisSurfaceSource.indexOf("activeCaseIdRef.current !== frozenCaseId", listenerStart);
    const safeProjection = dataAnalysisSurfaceSource.indexOf("projectSafeWsEvent(event, frozenCaseId)", listenerStart);
    const storeWrite = dataAnalysisSurfaceSource.indexOf("actions.setWsEvent(safeEvent)", listenerStart);

    expect(wsEffectStart).toBeGreaterThanOrEqual(0);
    expect(runtimeAcquire).toBeGreaterThan(wsEffectStart);
    expect(listenerStart).toBeGreaterThanOrEqual(0);
    expect(caseGuard).toBeGreaterThan(listenerStart);
    expect(safeProjection).toBeGreaterThan(caseGuard);
    expect(storeWrite).toBeGreaterThan(safeProjection);
    expect(dataAnalysisSurfaceSource).not.toContain("activateCase");
    expect(dataAnalysisSurfaceSource).not.toContain("/activate");
  });
});
