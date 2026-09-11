import { afterEach, describe, expect, it, vi } from "vitest";
import toastCenterSource from "../../../../components/feedback/ToastCenter.tsx?raw";
import graphPageSource from "../GraphPage.tsx?raw";
import flowPagePerfSource from "../adapters/flow-page-perf.ts?raw";
import flowPerfPanelSource from "../render/FlowPerfPanel.tsx?raw";
import embeddedAppSource from "../../runtime/embedded/analysis_flow/app.js?raw";
import graphEngineSource from "../../runtime/embedded/analysis_flow/graph_engine.js?raw";
import embeddedHostSource from "./embedded-flow-runtime.ts?raw";
import flowRuntimeSource from "./flow-runtime.ts?raw";
import { projectFlowPerfSnapshot } from "./flow-perf-public-projection";
import { createFlowRuntimeOpsBackendMethods } from "./flow-runtime-ops-backend";

function loadEmbeddedObjectMethod(source: string, signature: string): (...args: unknown[]) => unknown {
  const start = source.indexOf(signature);
  const openBrace = start < 0 ? -1 : start + signature.lastIndexOf("{");
  if (start < 0 || openBrace < 0) {
    throw new Error(`embedded method not found: ${signature}`);
  }
  let depth = 0;
  let end = -1;
  for (let index = openBrace; index < source.length; index += 1) {
    const token = source[index];
    if (token === "{") depth += 1;
    if (token === "}") depth -= 1;
    if (depth === 0) {
      end = index;
      break;
    }
  }
  if (end < 0) {
    throw new Error(`embedded method is incomplete: ${signature}`);
  }
  const methodName = signature.slice(0, signature.indexOf("(")).trim();
  const factory = new Function(`return ({${source.slice(start, end + 1)}});`) as () => Record<
    string,
    (...args: unknown[]) => unknown
  >;
  return factory()[methodName];
}

afterEach(() => {
  vi.unstubAllEnvs();
  vi.restoreAllMocks();
});

describe("flow runtime ordinary diagnostics", () => {
  it("drops arbitrary message and payload bytes before console logging", () => {
    const sentinel = "case-a account 6214600780000579708 /private/case-a.csv";
    const log = vi.spyOn(console, "log").mockImplementation(() => undefined);
    const methods = createFlowRuntimeOpsBackendMethods({
      service: { getFlowPerfSnapshot: () => ({}) } as never,
      targetWindow: {} as Window,
    });

    methods.logFlowDebug(JSON.stringify({
      level: "ERROR",
      topic: "runtimeHost",
      message: sentinel,
      data: { account_no: "6214600780000579708", path: "/private/case-a.csv" },
    }));

    expect(log).toHaveBeenCalledWith("[Viz][Runtime][ERROR] runtimeHost");
    expect(JSON.stringify(log.mock.calls)).not.toContain(sentinel);
    expect(JSON.stringify(log.mock.calls)).not.toContain("6214600780000579708");
    expect(JSON.stringify(log.mock.calls)).not.toContain("/private/case-a.csv");
  });

  it("keeps embedded console/backend envelopes and host snapshots value-free", () => {
    expect(embeddedAppSource).toContain("topic: safeTopic");
    expect(embeddedAppSource).toContain("hasData: data !== undefined && data !== null");
    expect(embeddedAppSource).not.toMatch(/console\.log\([^\n]*message[^\n]*data/u);
    expect(embeddedAppSource).not.toContain("JSON.stringify({ level: safeLevel, message, data:");

    for (const forbiddenField of ["lastCaseId", "lastAssetUrl", "lastAssetError:", "lastError:"]) {
      expect(embeddedHostSource).not.toContain(forbiddenField);
    }
    expect(embeddedHostSource).not.toContain("payload.data");
    expect(embeddedHostSource).toContain("hasCaseContext");
    expect(embeddedHostSource).toContain("lastErrorCode");
    expect(toastCenterSource).toContain('console.error("[toast action failed]")');
    expect(toastCenterSource).not.toContain('console.error("[toast action failed]",');
  });

  it("keeps embedded and React flow console owners on fixed or projected values", () => {
    expect(graphEngineSource).toContain('console.warn("[Viz][WebGL] event=webgl_warning");');
    expect(graphEngineSource).not.toContain("console.warn(...args);");
    expect(graphEngineSource).toContain('console.log("[Viz][WheelRoute] event=wheel_route");');
    expect(graphEngineSource).not.toContain('console.log("[Viz][WheelRoute]", kind, payload);');
    expect(flowRuntimeSource).toContain('console.warn("[flow-runtime] event=unsupported_runtime_fallback");');
    expect(flowRuntimeSource).not.toContain('unsupported runtime "${raw}"');
    expect(flowRuntimeSource).not.toContain("invalidRuntimeWarned.add(raw)");

    expect(flowPagePerfSource).toContain("return projectFlowPerfSnapshot(bridge.getPerfSnapshot?.());");
    expect(graphPageSource).toContain("const snapshot = readFlowPerfSnapshot(shellBridge);");
    expect(graphPageSource).toContain('console.info("[flow-perf-sample]", snapshot);');
    expect(graphPageSource).toContain('console.warn("[flow-perf-alert]", snapshot);');
    expect(flowPerfPanelSource).toContain("const next = readFlowPerfSnapshot(bridge) as FlowPerfSnapshot;");
    expect(flowPerfPanelSource).toContain('console.info("[flow-perf]", projectOrdinaryStructuredValue(next));');
    expect(embeddedHostSource).toContain('method(`[FlowRuntimeHost][${level}] runtimeHost`);');
  });

  it("withholds an unsupported configured runtime id from the console", async () => {
    const sentinel = "FLOW_RUNTIME_ID_CANARY_7F3C_/private/case.csv";
    vi.stubEnv("VITE_FLOW_RUNTIME_ID", sentinel);
    vi.resetModules();
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    const { resolveConfiguredFlowRuntimeId } = await import("./flow-runtime");

    expect(resolveConfiguredFlowRuntimeId()).toBe("embedded");
    expect(warn).toHaveBeenCalledWith("[flow-runtime] event=unsupported_runtime_fallback");
    expect(JSON.stringify(warn.mock.calls)).not.toContain(sentinel);
  });

  it("withholds embedded WebGL and wheel-route payload canaries at runtime", () => {
    const canary = "EMBEDDED_GRAPH_PAYLOAD_CANARY_7F3C_/private/case.csv";
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    const log = vi.spyOn(console, "log").mockImplementation(() => undefined);
    const emitWebglWarn = loadEmbeddedObjectMethod(graphEngineSource, "_emitWebglWarn(key, ..._args) {");
    const wheelRouteLog = loadEmbeddedObjectMethod(graphEngineSource, "_wheelRouteLog(_kind, _payload = {}) {");

    emitWebglWarn.call({ _webglWarn: true, _webglWarnOnce: new Set<string>() }, "fixed-key", canary, {
      infoLog: canary
    });
    wheelRouteLog.call(
      {
        _isWheelRouteDebugEnabled: () => true,
        _wheelRouteLogKey: "",
        _wheelRouteLogAt: 0
      },
      canary,
      { target: canary, route: canary, x: 7, y: 3 }
    );

    expect(warn).toHaveBeenCalledWith("[Viz][WebGL] event=webgl_warning");
    expect(log).toHaveBeenCalledWith("[Viz][WheelRoute] event=wheel_route");
    expect(JSON.stringify([...warn.mock.calls, ...log.mock.calls])).not.toContain(canary);
  });

  it("projects an untrusted performance snapshot to closed numeric and enum diagnostics", () => {
    const sentinel = "case-a account 6214600780000579708 /private/case-a.csv";
    const projected = projectFlowPerfSnapshot({
      build: { lastStatus: sentinel, lastDurationMs: 12, secret: sentinel },
      graphPatch: { lastPatchScope: sentinel, lastOpCount: 3 },
      layoutWorker: { lastStage: sentinel, completedCount: 2 },
      runtimeHost: {
        lastStage: "attach:failed",
        lastErrorCode: sentinel,
        lastCaseId: sentinel,
        lastAssetUrl: sentinel,
        hasCaseContext: true,
      },
      transport: { ws: { status: sentinel, lastEventName: sentinel, openCount: 1 } },
      traces: { raw: sentinel },
      alerts: [{ id: sentinel, value: sentinel }],
      alertHistory: [{ raw: sentinel }],
    });

    expect(JSON.stringify(projected)).not.toContain(sentinel);
    expect(JSON.stringify(projected)).not.toContain("6214600780000579708");
    expect(projected.build).toEqual({ lastDurationMs: 12, lastStatus: "" });
    expect(projected.runtimeHost).toMatchObject({
      lastStage: "attach:failed",
      lastErrorCode: "",
      hasCaseContext: true,
    });
    expect(projected.alerts).toEqual([{ id: "alert-1" }]);
    expect(projected.alertHistory).toEqual([{}]);
  });
});
