const SAFE_BUILD_STATUS = new Set(["idle", "pending", "running", "ok", "success", "failed", "error", "cancelled"]);
const SAFE_PATCH_SCOPE = new Set(["graph", "node", "edge", "selection", "view", "full", "partial"]);
const SAFE_PATCH_KIND = new Set(["replace", "merge", "update", "clear", "incremental"]);
const SAFE_LAYOUT_STAGE = new Set(["idle", "queued", "running", "completed", "fallback", "failed", "cancelled"]);
const SAFE_WS_STATUS = new Set(["idle", "connecting", "open", "closed", "reconnecting", "failed", "error"]);
const SAFE_WS_EVENT = new Set(["open", "close", "error", "message", "reconnect"]);
const SAFE_RUNTIME_STAGE = new Set([
  "idle",
  "attach:start",
  "attach:ready",
  "attach:failed",
  "attach:recovered",
  "attach:retry-failed",
  "boot:start",
  "boot:iframe-ready",
  "boot:document-ready",
  "boot:asset-load",
  "boot:scripts-ready",
  "boot:store-ready",
  "boot:shell-ready",
  "boot:failed",
  "runtime:open-request",
  "runtime:refit",
  "runtime:set-case",
]);
const SAFE_RUNTIME_ERROR_CODE = new Set([
  "",
  "asset_load_failed",
  "embedded_runtime_boot_failed",
  "embedded_runtime_attach_failed",
  "embedded_runtime_attach_retry_failed",
]);

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function boundedNumber(value: unknown, max = Number.MAX_SAFE_INTEGER): number {
  const next = Number(value);
  if (!Number.isFinite(next)) return 0;
  return Math.min(max, Math.max(0, next));
}

function safeCode(value: unknown, allowed: ReadonlySet<string>): string {
  const next = String(value ?? "").trim().toLowerCase();
  return allowed.has(next) ? next : "";
}

function numericRecord(value: unknown, fields: readonly string[]): Record<string, number> {
  const source = asRecord(value);
  return Object.fromEntries(fields.map((field) => [field, boundedNumber(source[field])])) as Record<string, number>;
}

function durationWindow(value: unknown): Record<string, number> {
  return numericRecord(value, ["avg", "max"]);
}

export function projectFlowPerfSnapshot(value: unknown): Record<string, unknown> {
  const source = asRecord(value);
  const patch = asRecord(source.graphPatch);
  const patchBridge = asRecord(source.graphPatchBridge);
  const build = asRecord(source.build);
  const layoutWorker = asRecord(source.layoutWorker);
  const textAtlas = asRecord(source.textAtlas);
  const windows = asRecord(source.windows);
  const runtimeHost = asRecord(source.runtimeHost);
  const ws = asRecord(asRecord(source.transport).ws);
  const alerts = Array.isArray(source.alerts) ? source.alerts.slice(0, 100) : [];
  const alertHistory = Array.isArray(source.alertHistory) ? source.alertHistory.slice(0, 100) : [];

  return {
    timestamp: boundedNumber(source.timestamp),
    graphPatch: {
      ...numericRecord(patch, ["lastOpCount", "lastTargetEntityCount", "publishedCount", "failedCount"]),
      lastPatchScope: safeCode(patch.lastPatchScope, SAFE_PATCH_SCOPE),
      lastPatchKind: safeCode(patch.lastPatchKind, SAFE_PATCH_KIND),
    },
    graphPatchBridge: {
      publish: numericRecord(patchBridge.publish, ["lastDurationMs"]),
      apply: numericRecord(patchBridge.apply, ["lastDurationMs"]),
    },
    build: {
      ...numericRecord(build, ["lastDurationMs"]),
      lastStatus: safeCode(build.lastStatus, SAFE_BUILD_STATUS),
    },
    graphCommands: numericRecord(source.graphCommands, ["replaceCount", "clearCount"]),
    canvasInteraction: numericRecord(source.canvasInteraction, ["selectionChangeCount", "hitCount"]),
    layoutWorker: {
      ...numericRecord(layoutWorker, ["lastDurationMs", "completedCount", "fallbackCount"]),
      lastStage: safeCode(layoutWorker.lastStage, SAFE_LAYOUT_STAGE),
    },
    textAtlas: {
      current: numericRecord(textAtlas.current, ["glyphCount", "approxTextureBytes"]),
      peaks: numericRecord(textAtlas.peaks, ["glyphCount", "approxTextureBytes"]),
    },
    windows: {
      layoutDuration: durationWindow(windows.layoutDuration),
      patchApply: durationWindow(windows.patchApply),
      buildDuration: durationWindow(windows.buildDuration),
    },
    baselines: numericRecord(source.baselines, ["layoutDurationMs", "patchApplyMs", "textAtlasPeakTextureBytes"]),
    runtimeHost: {
      ...numericRecord(runtimeHost, [
        "attachCount",
        "attachSuccessCount",
        "attachFailureCount",
        "selfHealCount",
        "selfHealSuccessCount",
        "lastDurationMs",
        "lastAttemptAt",
        "lastReadyAt",
        "assetLoadCount",
        "assetErrorCount",
        "shellReadyCount",
        "openRequestCount",
        "lastOpenRequestAt",
        "refitCount",
        "lastRefitAt",
        "lastSelfHealAt",
      ]),
      lastStage: safeCode(runtimeHost.lastStage, SAFE_RUNTIME_STAGE),
      lastErrorCode: safeCode(runtimeHost.lastErrorCode, SAFE_RUNTIME_ERROR_CODE),
      hasCaseContext: runtimeHost.hasCaseContext === true,
    },
    transport: {
      ws: {
        ...numericRecord(ws, ["openCount", "reconnectCount", "errorCount"]),
        status: safeCode(ws.status, SAFE_WS_STATUS),
        lastEventName: safeCode(ws.lastEventName, SAFE_WS_EVENT),
      },
    },
    alerts: alerts.map((_item, index) => ({ id: `alert-${index + 1}` })),
    alertHistory: alertHistory.map(() => ({})),
  };
}
