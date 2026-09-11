import type { EmbeddedFlowBridgeService } from "../api/flow-bridge-service";
import { exportFlowGraphFromPayload } from "../adapters/flow-export-adapter";
import {
  type EmbeddedFlowCallback,
  invoke,
  parseJson,
  text,
} from "./flow-runtime-backend-utils";

export interface FlowRuntimeOpsBackendMethods {
  getFlowLogConfig: (callback?: EmbeddedFlowCallback) => void;
  logFlowDebug: (payloadJson?: string, callback?: EmbeddedFlowCallback) => void;
  getFlowPerfSnapshot: () => Record<string, unknown>;
  exportFlowGraph: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
}

interface CreateFlowRuntimeOpsBackendMethodsOptions {
  service: EmbeddedFlowBridgeService;
  targetWindow: Window;
}

const DEFAULT_LOG_CONFIG = {
  enabled: true,
  minLevel: "INFO",
  console: true,
  backend: false,
  core: true,
  dataFeed: true,
  graphTrace: true,
  graphSample: false,
  graphRender: true,
  layout: true,
  interaction: false,
  perf: false,
  debug: false,
  layoutVerbose: false,
  webglWarn: false,
  handoffProbe: false,
  canvasMetrics: false,
};

const SAFE_LOG_TOPICS = new Set([
  "canvasMetrics",
  "core",
  "dataFeed",
  "debug",
  "graphRender",
  "graphSample",
  "graphTrace",
  "handoffProbe",
  "interaction",
  "layout",
  "perf",
  "runtimeHost",
]);

export function createFlowRuntimeOpsBackendMethods(
  options: CreateFlowRuntimeOpsBackendMethodsOptions
): FlowRuntimeOpsBackendMethods {
  const { service, targetWindow } = options;

  return {
    getFlowLogConfig(callback) {
      invoke(callback, JSON.stringify({ ok: true, logConfig: DEFAULT_LOG_CONFIG }));
    },
    logFlowDebug(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const requestedLevel = text(payload.level || "INFO").toUpperCase();
      const level = requestedLevel === "WARN" || requestedLevel === "ERROR" ? requestedLevel : "INFO";
      const requestedTopic = text(payload.topic);
      const topic = SAFE_LOG_TOPICS.has(requestedTopic) ? requestedTopic : "debug";
      console.log(`[Viz][Runtime][${level}] ${topic}`);
      invoke(callback, JSON.stringify({ ok: true }));
    },
    getFlowPerfSnapshot() {
      try {
        return service.getFlowPerfSnapshot();
      } catch {
        return {};
      }
    },
    async exportFlowGraph(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      try {
        const result = await exportFlowGraphFromPayload(targetWindow, payload);
        invoke(callback, JSON.stringify(result));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "导出失败" }));
      }
    },
  };
}
