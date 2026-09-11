import { startTransition, useEffect, useState } from "react";
import { readFlowPerfSnapshot } from "../adapters/flow-page-perf";
import type { FlowShellBridge } from "../runtime/flow-runtime";

interface FlowPerfStatusBarProps {
  bridge: FlowShellBridge;
}

interface FlowPerfSnapshot {
  build?: Record<string, unknown>;
  graphPatchBridge?: Record<string, unknown>;
  layoutWorker?: Record<string, unknown>;
  windows?: Record<string, unknown>;
  runtimeHost?: Record<string, unknown>;
  transport?: {
    ws?: Record<string, unknown>;
  };
  textAtlas?: {
    peaks?: Record<string, unknown>;
  };
  alerts?: Array<Record<string, unknown>>;
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

function asNumber(value: unknown, fallback = 0): number {
  const next = Number(value);
  return Number.isFinite(next) ? next : fallback;
}

function formatMs(value: unknown): string {
  return `${Math.max(0, Math.round(asNumber(value, 0)))}ms`;
}

function formatBytesMb(value: unknown): string {
  const bytes = Math.max(0, asNumber(value, 0));
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
}

export function FlowPerfStatusBar({ bridge }: FlowPerfStatusBarProps): JSX.Element | null {
  const [snapshot, setSnapshot] = useState<FlowPerfSnapshot | null>(null);

  useEffect(() => {
    let disposed = false;
    const sample = (): void => {
      if (disposed) {
        return;
      }
      const next = readFlowPerfSnapshot(bridge) as FlowPerfSnapshot;
      startTransition(() => {
        setSnapshot(next);
      });
    };
    sample();
    const timer = window.setInterval(sample, 1200);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [bridge]);

  if (!snapshot) {
    return null;
  }

  const patchBridge = asRecord(snapshot.graphPatchBridge);
  const publish = asRecord(patchBridge.publish);
  const apply = asRecord(patchBridge.apply);
  const build = asRecord(snapshot.build);
  const worker = asRecord(snapshot.layoutWorker);
  const windows = asRecord(snapshot.windows);
  const patchWindow = asRecord(windows.patchApply);
  const textAtlasPeaks = asRecord(asRecord(snapshot.textAtlas).peaks);
  const runtimeHost = asRecord(snapshot.runtimeHost);
  const ws = asRecord(asRecord(snapshot.transport).ws);
  const alerts = Array.isArray(snapshot.alerts) ? snapshot.alerts.length : 0;

  return (
    <aside className={`flow-perf-statusbar${alerts > 0 ? " has-alerts" : ""}`} aria-label="图谱性能状态栏">
      <div className="flow-perf-statusbar-item">
        <span>Build</span>
        <strong>{formatMs(build.lastDurationMs)}</strong>
      </div>
      <div className="flow-perf-statusbar-item">
        <span>Layout</span>
        <strong>{formatMs(worker.lastDurationMs)}</strong>
      </div>
      <div className="flow-perf-statusbar-item">
        <span>Patch</span>
        <strong>{`${formatMs(publish.lastDurationMs)} / ${formatMs(apply.lastDurationMs)}`}</strong>
      </div>
      <div className="flow-perf-statusbar-item">
        <span>Patch Avg</span>
        <strong>{formatMs(patchWindow.avg)}</strong>
      </div>
      <div className="flow-perf-statusbar-item">
        <span>Atlas Peak</span>
        <strong>{formatBytesMb(textAtlasPeaks.approxTextureBytes)}</strong>
      </div>
      <div className="flow-perf-statusbar-item">
        <span>WS</span>
        <strong>{String(ws.status || "-")}</strong>
      </div>
      <div className="flow-perf-statusbar-item">
        <span>Heal</span>
        <strong>{`${asNumber(runtimeHost.selfHealSuccessCount)} / ${asNumber(runtimeHost.selfHealCount)}`}</strong>
      </div>
      <div className="flow-perf-statusbar-item">
        <span>Alerts</span>
        <strong>{String(alerts)}</strong>
      </div>
    </aside>
  );
}
