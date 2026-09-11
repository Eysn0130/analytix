import { ReactNode, useEffect, useLayoutEffect, useRef, useState } from "react";
import { ConfirmProvider } from "./components/feedback/ConfirmCenter";
import { PromptProvider } from "./components/feedback/PromptCenter";
import { TaskProvider } from "./components/feedback/TaskCenter";
import { ToastProvider } from "./components/feedback/ToastCenter";
import { emitStatsCacheInvalidation } from "./services/analysis/stats-cache-events";
import {
  preloadSharedCaseOverview,
  setSharedActiveCaseContext,
} from "./services/analysis/stats-case-overview-resource";
import { invalidateSharedStatsChartDetail } from "./services/analysis/stats-chart-detail-resource";
import { invalidateSharedStatsTree } from "./services/analysis/stats-tree-resource";
import {
  subscribeDesktopBackendRuntimeState,
  syncDesktopBackendRuntimeBase,
} from "./services/desktop/client";
import { fetchBackendHealth } from "./services/http/system";
import { projectSafeWsEvent } from "./services/ws/domain-events";
import { getSharedWsRuntime } from "./services/ws/shared-runtime";
import { AppStoreProvider, type BackendHealthStatus, useAppStore } from "./store/app-store";
import { applyTheme } from "./theme/theme";
import { ImportPage } from "./features/import/ImportPage";
import { CleaningPage } from "./features/cleaning/CleaningPage";
import { AnalysisPage } from "./features/analysis/AnalysisPage";
import { GraphPage } from "./features/flow/graph/GraphPage";
import "./DataAnalysisSurface.css";

export type DataAnalysisItemId = "import" | "cleaning" | "stats" | "visual";

type DataAnalysisSurfaceProps = {
  workspaceRoot: string;
  activeItemId: DataAnalysisItemId;
  onNavigate?: (itemId: DataAnalysisItemId) => void;
};

function isTerminalBackendAuthority(state: {
  terminal?: boolean;
  blocker?: string;
  authority?: string;
} | null | undefined): boolean {
  return Boolean(
    state?.terminal ||
    state?.authority === "unavailable" ||
    state?.blocker === "data_analysis_native_authority_unavailable"
  );
}

function RuntimeBootstrap({ workspaceRoot }: { workspaceRoot: string }): null {
  const {
    state: {
      session: { activeCaseId }
    },
    actions
  } = useAppStore();
  const wsRuntimeRef = useRef<ReturnType<typeof getSharedWsRuntime> | null>(null);
  const activeCaseIdRef = useRef(activeCaseId);
  const normalizedActiveCaseId = String(activeCaseId || "").trim();
  activeCaseIdRef.current = normalizedActiveCaseId;
  const [backendRuntimeReady, setBackendRuntimeReady] = useState(false);
  const [backendRuntimeTerminal, setBackendRuntimeTerminal] = useState(false);
  const [backendRuntimeGeneration, setBackendRuntimeGeneration] = useState(0);

  useEffect(() => {
    let canceled = false;
    const unsubscribe = subscribeDesktopBackendRuntimeState((state, ready) => {
      setBackendRuntimeTerminal(isTerminalBackendAuthority(state));
      setBackendRuntimeReady(ready);
      setBackendRuntimeGeneration(ready ? Math.max(0, Number(state?.generation || 0)) : 0);
    });
    void window.analytix.dataAnalysis.ensureBackend()
      .then((state) => {
        const terminal = isTerminalBackendAuthority(state);
        if (!canceled) {
          setBackendRuntimeTerminal(terminal);
          setBackendRuntimeGeneration(terminal ? 0 : Math.max(0, Number(state?.generation || 0)));
        }
        return terminal ? false : syncDesktopBackendRuntimeBase();
      })
      .then((ready) => {
        if (!canceled) {
          setBackendRuntimeReady(ready);
          if (!ready) setBackendRuntimeGeneration(0);
        }
      })
      .catch(() => {
        if (!canceled) {
          setBackendRuntimeReady(false);
          setBackendRuntimeGeneration(0);
        }
      });
    return () => {
      canceled = true;
      unsubscribe();
    };
  }, []);

  useEffect(() => {
    let canceled = false;

    const ensureCase = async (): Promise<void> => {
      if (backendRuntimeTerminal) {
        actions.setBackendHealth("down");
        return;
      }
      const normalizedWorkspaceRoot = workspaceRoot.trim();
      if (!normalizedWorkspaceRoot) {
        return;
      }
      try {
        await window.analytix.dataAnalysis.ensureBackend();
        await syncDesktopBackendRuntimeBase();
        const result = await window.analytix.dataAnalysis.ensureWorkspaceCase({
          workspaceRoot: normalizedWorkspaceRoot
        });
        if (!canceled && result.ok) {
          actions.setActiveCaseId(result.caseId);
          setSharedActiveCaseContext({ caseId: result.caseId, workspaceRoot: normalizedWorkspaceRoot });
          void preloadSharedCaseOverview(result.caseId).catch(() => {});
        }
      } catch {
        if (!canceled) {
          actions.setBackendHealth("down");
        }
      }
    };

    void ensureCase();
    return () => {
      canceled = true;
    };
  }, [actions, backendRuntimeTerminal, workspaceRoot]);

  useEffect(() => {
    let canceled = false;

    if (backendRuntimeTerminal) {
      actions.setBackendHealth("down");
      return () => {
        canceled = true;
      };
    }

    const syncHealth = async (): Promise<void> => {
      try {
        await window.analytix.dataAnalysis.ensureBackend();
        await syncDesktopBackendRuntimeBase();
        const health = await fetchBackendHealth();
        if (canceled) {
          return;
        }
        const status: BackendHealthStatus = health.status === "ok" ? "ok" : "degraded";
        actions.setBackendHealth(status);
      } catch {
        if (!canceled) {
          actions.setBackendHealth("down");
        }
      }
    };

    void syncHealth();
    const timer = window.setInterval(() => {
      void syncHealth();
    }, 15000);
    return () => {
      canceled = true;
      window.clearInterval(timer);
    };
  }, [actions, backendRuntimeTerminal]);

  useEffect(() => {
    if (!backendRuntimeReady || !normalizedActiveCaseId) {
      return undefined;
    }
    const frozenCaseId = normalizedActiveCaseId;
    const runtime = getSharedWsRuntime({ caseId: frozenCaseId });
    const invalidationEventIds = new Set<string>();
    wsRuntimeRef.current = runtime;
    const release = runtime.retain();

    const offStatus = runtime.onStatus((status) => {
      if (activeCaseIdRef.current !== frozenCaseId) {
        return;
      }
      actions.setWsStatus(status);
    }, { emitCurrent: true });
    const offEvent = runtime.onEvent((event) => {
      if (activeCaseIdRef.current !== frozenCaseId) {
        return;
      }
      const safeEvent = projectSafeWsEvent(event, frozenCaseId);
      if (!safeEvent) {
        return;
      }
      actions.setWsEvent(safeEvent);
      if (
        safeEvent.event !== "import.job.completed" &&
        safeEvent.event !== "cleaning.job.completed"
      ) {
        return;
      }
      const caseId = safeEvent.case_id;
      const eventId = safeEvent.payload.event_id;
      if (invalidationEventIds.has(eventId)) {
        return;
      }
      invalidationEventIds.add(eventId);
      if (invalidationEventIds.size > 256) {
        invalidationEventIds.clear();
        invalidationEventIds.add(eventId);
      }
      invalidateSharedStatsTree(caseId);
      invalidateSharedStatsChartDetail(caseId);
      const occurredAt = Date.parse(safeEvent.timestamp);
      emitStatsCacheInvalidation({
        caseId,
        eventId,
        source: `ws:${safeEvent.event}`,
        occurredAt: Number.isFinite(occurredAt) ? occurredAt : Date.now(),
      });
    }, { emitLast: true });

    return () => {
      offStatus();
      offEvent();
      release();
      if (wsRuntimeRef.current === runtime) {
        wsRuntimeRef.current = null;
      }
    };
  }, [actions, backendRuntimeGeneration, backendRuntimeReady, normalizedActiveCaseId]);

  useEffect(() => {
    const caseId = normalizedActiveCaseId;
    if (caseId) {
      setSharedActiveCaseContext({ caseId, workspaceRoot });
      void preloadSharedCaseOverview(caseId).catch(() => {});
    }
  }, [normalizedActiveCaseId, workspaceRoot]);

  return null;
}

function DataAnalysisProviders({
  workspaceRoot,
  children
}: {
  workspaceRoot: string;
  children: ReactNode;
}): JSX.Element {
  return (
    <AppStoreProvider>
      <TaskProvider>
        <PromptProvider>
          <ConfirmProvider>
            <ToastProvider>
              <RuntimeBootstrap workspaceRoot={workspaceRoot} />
              {children}
            </ToastProvider>
          </ConfirmProvider>
        </PromptProvider>
      </TaskProvider>
    </AppStoreProvider>
  );
}

function DataAnalysisSurfaceWorkspace({
  activeItemId,
  onNavigate
}: Omit<DataAnalysisSurfaceProps, "workspaceRoot">): JSX.Element {
  const {
    state: { resolvedTheme }
  } = useAppStore();
  const surfaceRef = useRef<HTMLDivElement | null>(null);

  useLayoutEffect(() => {
    if (surfaceRef.current) {
      applyTheme(resolvedTheme, surfaceRef.current);
    }
  }, [resolvedTheme]);

  useEffect(() => {
    const handleNavigate = (event: Event): void => {
      const detail = event instanceof CustomEvent ? event.detail : null;
      const itemId = String(detail?.itemId || "").trim();
      if (itemId === "import" || itemId === "cleaning" || itemId === "stats" || itemId === "visual") {
        onNavigate?.(itemId);
      }
    };
    window.addEventListener("data-analysis:navigate", handleNavigate);
    return () => window.removeEventListener("data-analysis:navigate", handleNavigate);
  }, [onNavigate]);

  const activePage = (() => {
    switch (activeItemId) {
      case "cleaning":
        return <CleaningPage active />;
      case "stats":
        return <AnalysisPage active />;
      case "visual":
        return <GraphPage active />;
      case "import":
      default:
        return <ImportPage active />;
    }
  })();

  return (
    <div
      ref={surfaceRef}
      className="data-analysis-surface"
      data-active-item={activeItemId}
      data-theme={resolvedTheme}
    >
      <section className="data-analysis-surface__page">
        {activePage}
      </section>
    </div>
  );
}

export function DataAnalysisSurface({
  workspaceRoot,
  activeItemId,
  onNavigate
}: DataAnalysisSurfaceProps): JSX.Element {
  return (
    <DataAnalysisProviders workspaceRoot={workspaceRoot}>
      <DataAnalysisSurfaceWorkspace activeItemId={activeItemId} onNavigate={onNavigate} />
    </DataAnalysisProviders>
  );
}
