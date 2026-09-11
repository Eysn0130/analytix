import { Suspense, lazy, useEffect, useRef, useState } from "react";
import { useAppStore } from "../../../store/app-store";
import { resolveDataAnalysisRuntimeEndpointAuthority } from "../../../services/runtime-base";
import {
  ensureDesktopBackendRuntime,
  hasDesktopBridge,
  subscribeDesktopBackendRuntimeState,
} from "../../../services/desktop/client";
import {
  buildFlowPerfAlertToken,
  buildFlowPerfSampleToken,
  readFlowPerfFlags,
  readFlowPerfSnapshot,
} from "./adapters/flow-page-perf";
import {
  clearStagedFlowTransferPayload,
  hasStagedFlowTransferPayload,
  readFlowTransferPayload,
  text,
  type FlowRequestPayload,
} from "./adapters/flow-page-transfer";
import type {
  FlowRuntimeAdapter,
  FlowShellBridge,
  FlowShellMounts,
  FlowShellState
} from "./runtime/flow-runtime";
import { FlowCanvasChrome } from "./interactions/FlowCanvasChrome";
import "./styles/flow-host.css";

const FlowShell = lazy(async () => {
  const mod = await import("./components/FlowShell");
  return { default: mod.FlowShell };
});

const FlowPerfPanel = lazy(async () => {
  const mod = await import("./render/FlowPerfPanel");
  return { default: mod.FlowPerfPanel };
});

const FlowPerfStatusBar = lazy(async () => {
  const mod = await import("./render/FlowPerfStatusBar");
  return { default: mod.FlowPerfStatusBar };
});

function resolveApiBaseUrl(): string {
  return resolveDataAnalysisRuntimeEndpointAuthority().apiBaseUrl;
}

export function GraphPage({ active = true }: { active?: boolean }): JSX.Element {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const runtimeRef = useRef<FlowRuntimeAdapter | null>(null);
  const pendingRequestRef = useRef<FlowRequestPayload | null>(null);
  const {
    state: {
      session: { activeCaseId }
    },
    actions: { setActiveCaseId }
  } = useAppStore();
  const [runtimeReady, setRuntimeReady] = useState(false);
  const [runtimeError, setRuntimeError] = useState("");
  const [shellBridge, setShellBridge] = useState<FlowShellBridge | null>(null);
  const [shellMounts, setShellMounts] = useState<FlowShellMounts | null>(null);
  const [shellState, setShellState] = useState<FlowShellState | null>(null);
  const [perfPanelEnabled, setPerfPanelEnabled] = useState(false);
  const [perfLogEnabled, setPerfLogEnabled] = useState(false);
  const [perfStatusEnabled, setPerfStatusEnabled] = useState(false);
  const [backendAuthorityRevision, setBackendAuthorityRevision] = useState(0);
  const activeCaseIdRef = useRef(activeCaseId);

  useEffect(() => {
    activeCaseIdRef.current = activeCaseId;
  }, [activeCaseId]);

  useEffect(() => {
    if (!hasDesktopBridge()) {
      return () => {};
    }
    return subscribeDesktopBackendRuntimeState(() => {
      setBackendAuthorityRevision((revision) => revision + 1);
    });
  }, []);

  useEffect(() => {
    let cancelled = false;
    const host = hostRef.current;
    const mountHost = async (): Promise<void> => {
      try {
        if (hasDesktopBridge()) {
          const ready = await ensureDesktopBackendRuntime();
          if (!ready) {
            throw new Error("data_analysis_backend_not_ready");
          }
        }
        const runtimeModule = await import("./runtime/flow-runtime");
        const runtime = await runtimeModule.ensureFlowRuntimeAdapter(resolveApiBaseUrl());
        if (cancelled) {
          return;
        }
        runtimeRef.current = runtime;
        if (host) {
          await runtime.attach(host);
        }
        const bridge = runtime.getShellBridge();
        setShellBridge(bridge);
        setShellMounts(runtime.getShellMounts());
        setShellState(bridge?.getState() ?? null);
        setRuntimeReady(true);
        setRuntimeError("");
        if (!hasStagedFlowTransferPayload()) {
          runtime.setCaseId(activeCaseIdRef.current);
        }
        window.setTimeout(() => runtime.refit(), 80);
        const perfFlags = readFlowPerfFlags();
        setPerfPanelEnabled(perfFlags.panel);
        setPerfLogEnabled(perfFlags.log);
        setPerfStatusEnabled(perfFlags.status);
      } catch (error) {
        if (cancelled) {
          return;
        }
        const message = error instanceof Error ? error.message : "flow host init failed";
        setRuntimeError(message);
        setRuntimeReady(false);
        setShellBridge(null);
        setShellMounts(null);
        setShellState(null);
      }
    };

    void mountHost();
    return () => {
      cancelled = true;
      const runtime = runtimeRef.current;
      if (host && runtime) {
        runtime.detach(host);
      }
      if (runtimeRef.current === runtime) {
        runtimeRef.current = null;
      }
      setShellBridge(null);
      setShellMounts(null);
      setShellState(null);
    };
  }, [backendAuthorityRevision]);

  useEffect(() => {
    if (!runtimeReady || !runtimeRef.current) {
      return;
    }
    const bridge = runtimeRef.current.getShellBridge();
    if (!bridge) {
      return;
    }
    setShellBridge(bridge);
    setShellMounts(runtimeRef.current.getShellMounts());
    setShellState(bridge.getState());
    const unsubscribe = bridge.subscribe((nextState) => {
      setShellState(nextState);
    });
    return () => {
      if (typeof unsubscribe === "function") {
        unsubscribe();
      }
    };
  }, [runtimeReady]);

  useEffect(() => {
    if (!runtimeReady || !runtimeRef.current) {
      return;
    }
    if (active && (pendingRequestRef.current || hasStagedFlowTransferPayload())) {
      return;
    }
    runtimeRef.current.setCaseId(activeCaseId);
  }, [active, activeCaseId, runtimeReady]);

  useEffect(() => {
    if (!active) {
      return;
    }
    if (!pendingRequestRef.current) {
      // FlowPage lives in a keep-alive shell, so stats -> flow handoff must be
      // re-read whenever the page becomes active again, not only on first mount.
      pendingRequestRef.current = readFlowTransferPayload();
    }
    const pendingCaseId = text(pendingRequestRef.current?.caseId || pendingRequestRef.current?.case_id);
    if (pendingCaseId && pendingCaseId !== activeCaseId) {
      setActiveCaseId(pendingCaseId);
    }
    if (!runtimeReady || !runtimeRef.current || !pendingRequestRef.current) {
      return;
    }
    // openRequest already carries the destination case id and applies it inside
    // the embedded runtime. Do not issue an extra setCaseId here, otherwise the
    // async tree/views refresh can race with the graph handoff and clear the
    // graph that was just rendered from the incoming request.
    runtimeRef.current.openRequest(pendingRequestRef.current);
    pendingRequestRef.current = null;
    clearStagedFlowTransferPayload();
  }, [active, activeCaseId, runtimeReady, setActiveCaseId]);

  useEffect(() => {
    if (!active || !runtimeReady || !runtimeRef.current) {
      return;
    }
    const timer = window.setTimeout(() => {
      runtimeRef.current?.refit();
    }, 80);
    return () => {
      window.clearTimeout(timer);
    };
  }, [active, runtimeReady]);

  useEffect(() => {
    if (!runtimeReady || !runtimeRef.current || !hostRef.current) {
      return;
    }

    const emitRefit = (): void => {
      runtimeRef.current?.refit();
    };

    const timer = window.setTimeout(emitRefit, 80);
    window.addEventListener("resize", emitRefit);

    const host = hostRef.current;
    const observer =
      typeof ResizeObserver !== "undefined"
        ? new ResizeObserver(() => {
            emitRefit();
          })
        : null;

    observer?.observe(host);
    return () => {
      observer?.disconnect();
      window.clearTimeout(timer);
      window.removeEventListener("resize", emitRefit);
    };
  }, [runtimeReady]);

  useEffect(() => {
    const syncPerfFlags = (): void => {
      const perfFlags = readFlowPerfFlags();
      setPerfPanelEnabled(perfFlags.panel);
      setPerfLogEnabled(perfFlags.log);
      setPerfStatusEnabled(perfFlags.status);
    };
    window.addEventListener("storage", syncPerfFlags);
    window.addEventListener("focus", syncPerfFlags);
    return () => {
      window.removeEventListener("storage", syncPerfFlags);
      window.removeEventListener("focus", syncPerfFlags);
    };
  }, []);

  useEffect(() => {
    if (!runtimeReady || !shellBridge || !perfLogEnabled) {
      return;
    }
    let disposed = false;
    let lastToken = "";
    const sample = (): void => {
      if (disposed) {
        return;
      }
      const snapshot = readFlowPerfSnapshot(shellBridge);
      const token = buildFlowPerfSampleToken(snapshot);
      if (!token || token === lastToken) {
        return;
      }
      lastToken = token;
      console.info("[flow-perf-sample]", snapshot);
    };
    sample();
    const timer = window.setInterval(sample, 3000);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [perfLogEnabled, runtimeReady, shellBridge]);

  useEffect(() => {
    if (!runtimeReady || !shellBridge || !(perfLogEnabled || perfPanelEnabled || perfStatusEnabled)) {
      return;
    }
    let disposed = false;
    let lastAlertToken = "";
    const sample = (): void => {
      if (disposed) {
        return;
      }
      const snapshot = readFlowPerfSnapshot(shellBridge);
      const token = buildFlowPerfAlertToken(snapshot);
      if (!token || token === lastAlertToken) {
        return;
      }
      lastAlertToken = token;
      console.warn("[flow-perf-alert]", snapshot);
    };
    sample();
    const timer = window.setInterval(sample, 1600);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [perfLogEnabled, perfPanelEnabled, perfStatusEnabled, runtimeReady, shellBridge]);

  return (
    <section className="flow-page" aria-label="可视化分析">
      <div ref={hostRef} className="flow-host" />
      {runtimeReady && shellBridge && shellMounts && shellState ? (
        <>
          <FlowCanvasChrome bridge={shellBridge} mounts={shellMounts} state={shellState} />
          <Suspense fallback={null}>
            <FlowShell bridge={shellBridge} mounts={shellMounts} state={shellState} />
          </Suspense>
          {perfPanelEnabled ? (
            <Suspense fallback={null}>
              <FlowPerfPanel bridge={shellBridge} logToConsole={perfLogEnabled} />
            </Suspense>
          ) : null}
          {perfStatusEnabled ? (
            <Suspense fallback={null}>
              <FlowPerfStatusBar bridge={shellBridge} />
            </Suspense>
          ) : null}
        </>
      ) : null}
      {!runtimeReady && !runtimeError ? (
        <div className="flow-status" role="status" aria-live="polite">
          正在加载可视化分析界面…
        </div>
      ) : null}
      {runtimeError ? (
        <div className="flow-status flow-status-error" role="alert">
          {runtimeError}
        </div>
      ) : null}
    </section>
  );
}

export const FlowPage = GraphPage;
