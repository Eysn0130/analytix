import type { FlowShellBridge } from "../runtime/flow-runtime";
import { projectFlowPerfSnapshot } from "../runtime/flow-perf-public-projection";

export interface FlowPerfFlags {
  panel: boolean;
  log: boolean;
  status: boolean;
}

type FlowDebugWindow = typeof window & {
  __ANALYTIX_DEBUG_PERF_PANEL?: boolean;
  __ANALYTIX_DEBUG_PERF_STATUS?: boolean;
};

function readLocalStorageFlag(key: string): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  try {
    return localStorage.getItem(key) === "1";
  } catch {
    return false;
  }
}

export function readFlowPerfFlags(): FlowPerfFlags {
  if (typeof window === "undefined") {
    return { panel: false, log: false, status: false };
  }
  const debugWindow = window as FlowDebugWindow;
  return {
    panel: debugWindow.__ANALYTIX_DEBUG_PERF_PANEL === true || readLocalStorageFlag("analytix:flow:perf-panel"),
    log: readLocalStorageFlag("analytix:flow:perf-log"),
    status: debugWindow.__ANALYTIX_DEBUG_PERF_STATUS === true || readLocalStorageFlag("analytix:flow:perf-status"),
  };
}

function asObject(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

export function buildFlowPerfSampleToken(snapshot: Record<string, unknown>): string {
  const patchBridge = asObject(snapshot.graphPatchBridge);
  const publish = asObject(patchBridge.publish);
  const apply = asObject(patchBridge.apply);
  const layoutWorker = asObject(snapshot.layoutWorker);
  const build = asObject(snapshot.build);
  const traces = asObject(snapshot.traces);
  const patchTrace = asObject(asObject(traces.patch).apply);
  const alerts = Array.isArray(snapshot.alerts) ? (snapshot.alerts as Array<Record<string, unknown>>) : [];
  return [
    String(snapshot.timestamp || ""),
    String(build.lastTraceId || ""),
    String(publish.lastDurationMs || ""),
    String(apply.lastDurationMs || ""),
    String(layoutWorker.lastDurationMs || ""),
    String(patchTrace.traceId || ""),
    String(alerts.length || 0),
  ].join("|");
}

export function buildFlowPerfAlertToken(snapshot: Record<string, unknown>): string {
  const alerts = Array.isArray(snapshot.alerts) ? (snapshot.alerts as Array<Record<string, unknown>>) : [];
  return alerts.map((item) => `${String(item.id || "")}:${String(item.value || "")}:${String(item.traceId || "")}`).join("|");
}

export function readFlowPerfSnapshot(bridge: FlowShellBridge): Record<string, unknown> {
  return projectFlowPerfSnapshot(bridge.getPerfSnapshot?.());
}
