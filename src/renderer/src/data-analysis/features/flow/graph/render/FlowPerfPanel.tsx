import { startTransition, useEffect, useState } from "react";
import { projectOrdinaryStructuredValue } from "../../../shared/ordinary-pii-projection";
import { readFlowPerfSnapshot } from "../adapters/flow-page-perf";
import type { FlowShellBridge } from "../runtime/flow-runtime";

interface FlowPerfPanelProps {
  bridge: FlowShellBridge;
  logToConsole?: boolean;
}

interface FlowPerfSnapshot {
  graphPatch?: Record<string, unknown>;
  graphPatchBridge?: Record<string, unknown>;
  build?: Record<string, unknown>;
  graphCommands?: Record<string, unknown>;
  canvasInteraction?: Record<string, unknown>;
  layoutWorker?: Record<string, unknown>;
  textAtlas?: {
    current?: Record<string, unknown>;
    peaks?: Record<string, unknown>;
  };
  windows?: Record<string, unknown>;
  traces?: Record<string, unknown>;
  baselines?: Record<string, unknown>;
  runtimeHost?: Record<string, unknown>;
  transport?: {
    ws?: Record<string, unknown>;
  };
  runtimeBridge?: Record<string, unknown>;
  alerts?: Array<Record<string, unknown>>;
  alertHistory?: Array<Record<string, unknown>>;
  timestamp?: number;
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

function asNumber(value: unknown, fallback = 0): number {
  const next = Number(value);
  return Number.isFinite(next) ? next : fallback;
}

function text(value: unknown, fallback = ""): string {
  const next = String(value == null ? "" : value).trim();
  return next || fallback;
}

function formatBytes(value: unknown): string {
  const bytes = Math.max(0, asNumber(value, 0));
  if (bytes >= 1024 * 1024) {
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  }
  if (bytes >= 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }
  return `${bytes} B`;
}

function formatMs(value: unknown): string {
  return `${Math.max(0, Math.round(asNumber(value, 0)))} ms`;
}

async function exportSnapshot(snapshot: FlowPerfSnapshot | null): Promise<void> {
  if (!snapshot) {
    return;
  }
  const payload = JSON.stringify(
    {
      exportedAt: Date.now(),
      snapshot: projectOrdinaryStructuredValue(snapshot),
    },
    null,
    2
  );
  try {
    if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(payload);
      return;
    }
  } catch {
    // Clipboard writes can be unavailable in restricted Electron/browser contexts.
  }
  const blob = new Blob([payload], { type: "application/json;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `flow-perf-${Date.now()}.json`;
  link.click();
  window.setTimeout(() => {
    URL.revokeObjectURL(url);
  }, 0);
}

export function FlowPerfPanel({ bridge, logToConsole = false }: FlowPerfPanelProps): JSX.Element | null {
  const [snapshot, setSnapshot] = useState<FlowPerfSnapshot | null>(null);

  useEffect(() => {
    let disposed = false;
    let lastLogToken = "";
    const sample = (): void => {
      if (disposed) {
        return;
      }
      const next = readFlowPerfSnapshot(bridge) as FlowPerfSnapshot;
      startTransition(() => {
        setSnapshot(next);
      });
      if (logToConsole) {
        const patch = asRecord(next.graphPatch);
        const patchBridge = asRecord(next.graphPatchBridge);
        const worker = asRecord(next.layoutWorker);
        const textAtlas = asRecord(asRecord(next.textAtlas).current);
        const token = [
          text(next.timestamp),
          text(asRecord(next.build).lastTraceId),
          text(patch.lastPatchScope),
          text(patch.lastPatchKind),
          formatMs(asRecord(patchBridge.publish).lastDurationMs),
          formatMs(asRecord(patchBridge.apply).lastDurationMs),
          text(worker.lastStage),
          text(textAtlas.glyphCount),
          text(textAtlas.approxTextureBytes),
        ].join("|");
        if (token && token !== lastLogToken) {
          lastLogToken = token;
          console.info("[flow-perf]", projectOrdinaryStructuredValue(next));
        }
      }
    };

    sample();
    const timer = window.setInterval(sample, 1000);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [bridge, logToConsole]);

  if (!snapshot) {
    return null;
  }

  const patch = asRecord(snapshot.graphPatch);
  const patchBridge = asRecord(snapshot.graphPatchBridge);
  const patchPublish = asRecord(patchBridge.publish);
  const patchApply = asRecord(patchBridge.apply);
  const build = asRecord(snapshot.build);
  const commands = asRecord(snapshot.graphCommands);
  const canvas = asRecord(snapshot.canvasInteraction);
  const worker = asRecord(snapshot.layoutWorker);
  const atlasRoot = asRecord(snapshot.textAtlas);
  const atlasCurrent = asRecord(atlasRoot.current);
  const atlasPeaks = asRecord(atlasRoot.peaks);
  const windows = asRecord(snapshot.windows);
  const layoutWindow = asRecord(windows.layoutDuration);
  const patchWindow = asRecord(windows.patchApply);
  const buildWindow = asRecord(windows.buildDuration);
  const traces = asRecord(snapshot.traces);
  const traceBuild = asRecord(traces.build);
  const traceLayout = asRecord(traces.layout);
  const tracePatch = asRecord(asRecord(traces.patch).apply);
  const baselines = asRecord(snapshot.baselines);
  const runtimeHost = asRecord(snapshot.runtimeHost);
  const transport = asRecord(snapshot.transport);
  const ws = asRecord(transport.ws);
  const runtimeBridge = asRecord(snapshot.runtimeBridge);
  const alerts = Array.isArray(snapshot.alerts) ? snapshot.alerts.map((item) => asRecord(item)) : [];
  const alertHistory = Array.isArray(snapshot.alertHistory) ? snapshot.alertHistory.map((item) => asRecord(item)) : [];
  const alertSummary = alerts.length
    ? alerts
        .map((item) => `${text(item.id)}>${text(item.baseline)}`)
        .filter(Boolean)
        .join(" · ")
    : "OK";

  return (
    <aside className="flow-perf-panel" aria-label="图谱性能面板">
      <div className="flow-perf-title">Flow Perf</div>
      <div className="flow-perf-actions">
        <button type="button" className="flow-perf-export" onClick={() => void exportSnapshot(snapshot)}>
          导出调试快照
        </button>
      </div>
      <div className="flow-perf-grid">
        <div className="flow-perf-row">
          <span>Build</span>
          <strong>{`${text(build.lastStatus, "-")} / ${formatMs(build.lastDurationMs)}`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Build Window</span>
          <strong>{`${formatMs(buildWindow.avg)} avg / ${formatMs(buildWindow.max)} max`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Patch</span>
          <strong>{`${text(patch.lastPatchScope, "-")} / ${text(patch.lastPatchKind, "-")}`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Patch Ops</span>
          <strong>{`${asNumber(patch.lastOpCount)} / ${asNumber(patch.lastTargetEntityCount)}`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Patch Count</span>
          <strong>{`${asNumber(patch.publishedCount)} pub / ${asNumber(patch.failedCount)} fail`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Patch Bridge</span>
          <strong>{`${formatMs(patchPublish.lastDurationMs)} pub / ${formatMs(patchApply.lastDurationMs)} apply`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Patch Window</span>
          <strong>{`${formatMs(patchWindow.avg)} avg / ${formatMs(patchWindow.max)} max`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>State</span>
          <strong>{`${asNumber(commands.replaceCount)} replace / ${asNumber(commands.clearCount)} clear`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Canvas</span>
          <strong>{`${asNumber(canvas.selectionChangeCount)} sel / ${asNumber(canvas.hitCount)} hit`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Layout</span>
          <strong>{`${text(worker.lastStage, "-")} / ${formatMs(worker.lastDurationMs)}`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Layout Window</span>
          <strong>{`${formatMs(layoutWindow.avg)} avg / ${formatMs(layoutWindow.max)} max`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Layout Count</span>
          <strong>{`${asNumber(worker.completedCount)} ok / ${asNumber(worker.fallbackCount)} fb`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Atlas</span>
          <strong>{`${asNumber(atlasCurrent.glyphCount)} glyph / ${formatBytes(atlasCurrent.approxTextureBytes)}`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Atlas Peak</span>
          <strong>{`${asNumber(atlasPeaks.glyphCount)} / ${formatBytes(atlasPeaks.approxTextureBytes)}`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Baseline</span>
          <strong>{`${formatMs(baselines.layoutDurationMs)} / ${formatMs(baselines.patchApplyMs)} / ${formatBytes(baselines.textAtlasPeakTextureBytes)}`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Alerts</span>
          <strong>{alertSummary}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Alert Log</span>
          <strong>{`${alertHistory.length} samples`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>WS</span>
          <strong>{`${text(ws.status, "-")} / ${text(ws.lastEventName, "-")}`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>WS Counters</span>
          <strong>{`${asNumber(ws.openCount)} open / ${asNumber(ws.reconnectCount)} retry / ${asNumber(ws.errorCount)} err`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Runtime</span>
          <strong>{`${text(runtimeHost.lastStage, "-")} / ${formatMs(runtimeHost.lastDurationMs)}`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Self Heal</span>
          <strong>{`${asNumber(runtimeHost.selfHealSuccessCount)} ok / ${asNumber(runtimeHost.selfHealCount)} try`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Assets</span>
          <strong>{`${asNumber(runtimeHost.assetLoadCount)} load / ${asNumber(runtimeHost.assetErrorCount)} fail`}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Bridge</span>
          <strong>{text(asRecord(runtimeBridge.activeBuild).jobId, "idle")}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Runtime Error</span>
          <strong>{text(runtimeHost.lastErrorCode, "OK")}</strong>
        </div>
        <div className="flow-perf-row">
          <span>Trace</span>
          <strong>{`${text(traceBuild.traceId || tracePatch.traceId || traceLayout.traceId, "-")}`}</strong>
        </div>
      </div>
    </aside>
  );
}
