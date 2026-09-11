/* Layout worker lifecycle controller.
 * Responsibilities: keep worker pending state, metrics, and fallback transitions consistent.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof window !== "undefined" ? window : null;
  if (!root) return;

  function requireFunction(fn, message) {
    if (typeof fn !== "function") {
      throw new Error(message);
    }
    return fn;
  }

  function createLayoutWorkerLifecycleController(deps = {}) {
    const state = deps?.state && typeof deps.state === "object" ? deps.state : null;
    if (!state) throw new Error("layout-worker-lifecycle-state-missing");
    const recordLayoutWorkerMetrics = requireFunction(
      deps?.recordLayoutWorkerMetrics,
      "layout-worker-lifecycle-metrics-missing"
    );
    const logGraphTrace = requireFunction(
      deps?.logGraphTrace,
      "layout-worker-lifecycle-trace-missing"
    );
    const computeGraphTotalAmount = requireFunction(
      deps?.computeGraphTotalAmount,
      "layout-worker-lifecycle-amount-missing"
    );
    const resetLayoutWorkerInstance = requireFunction(
      deps?.resetLayoutWorkerInstance,
      "layout-worker-lifecycle-reset-missing"
    );
    const applyPendingLayoutFallback = requireFunction(
      deps?.applyPendingLayoutFallback,
      "layout-worker-lifecycle-fallback-missing"
    );
    const updateLayoutButtons = requireFunction(
      deps?.updateLayoutButtons,
      "layout-worker-lifecycle-buttons-missing"
    );
    const log = typeof deps?.log === "function" ? deps.log : () => {};
    const scheduleReinit = typeof deps?.scheduleReinit === "function" ? deps.scheduleReinit : () => {};
    const performanceApi = deps?.performanceApi || root.performance || null;
    const clearTimeoutFn =
      typeof deps?.clearTimeoutFn === "function"
        ? deps.clearTimeoutFn
        : typeof root.clearTimeout === "function"
          ? root.clearTimeout.bind(root)
          : () => {};

    function now() {
      const value = Number(performanceApi?.now?.());
      return Number.isFinite(value) ? value : Date.now();
    }

    function pendingDurationMs(pending) {
      return Math.max(0, Math.round(now() - Number(pending?.startedAt || now())));
    }

    function clearPendingTimer(pending) {
      if (!pending?.timer) return false;
      try {
        clearTimeoutFn(pending.timer);
      } catch (error) {}
      return true;
    }

    function pendingMetricDetails(pending, data = {}, extra = {}) {
      return {
        seq: pending?.seq || 0,
        stage: extra.stage || state.layoutWorkerStage || "",
        traceId: pending?.traceId || data?.trace_id || "",
        preset: pending?.preset || "",
        nodes: pending?.nodes?.length || 0,
        edges: pending?.edges?.length || 0,
        ...extra,
      };
    }

    function traceCancellation(pending, reason = "", extra = {}) {
      try {
        logGraphTrace("layout-worker-cancelled", {
          view: pending?.preset || "compact",
          focusId: pending?.focusId || "",
          counts: { nodes: pending?.nodes?.length || 0, edges: pending?.edges?.length || 0 },
          amount: computeGraphTotalAmount(pending?.edges || []),
          reason: reason || "layout-worker",
          workerSeq: pending?.seq || 0,
          sourceRequestId: pending?.token || "",
          targetRequestId: state.requestId || "",
          ...extra,
        });
      } catch (error) {}
    }

    function recordProgress(pending, data = {}) {
      pending.phase = String(data?.phase || "running").trim().toLowerCase() || "running";
      pending.progress = Number(data?.progress) || 0;
      state.layoutWorkerStage = pending.phase;
      recordLayoutWorkerMetrics(
        "progress",
        pendingMetricDetails(pending, data, { stage: pending.phase })
      );
    }

    function recordComplete(pending, data = {}) {
      state.layoutWorkerStage = "complete";
      recordLayoutWorkerMetrics(
        "complete",
        pendingMetricDetails(pending, data, {
          stage: "complete",
          durationMs: pendingDurationMs(pending),
        })
      );
    }

    function cancelStaleRequest(pending, data = {}) {
      traceCancellation(pending, "stale-request");
      state.layoutWorkerPending = null;
      state.layoutWorkerStage = "idle";
      recordLayoutWorkerMetrics(
        "cancel",
        pendingMetricDetails(pending, data, { stage: "idle", reason: "stale-request" })
      );
      return true;
    }

    function fallbackToSync(pending, data = {}, reason = "", { disableWorker = false } = {}) {
      state.layoutWorkerPending = null;
      if (disableWorker) {
        if (reason) state.layoutWorkerDisabledReason = String(reason || "");
        resetLayoutWorkerInstance(reason || "layout-worker-disabled", { reinit: false });
      }
      try {
        log("WARN", "layout worker fallback sync", {
          reason: String(reason || "unknown"),
          preset: pending?.preset || "",
          nodes: pending?.nodes?.length || 0,
          edges: pending?.edges?.length || 0,
        });
      } catch (error) {}
      recordLayoutWorkerMetrics(
        "fallback",
        pendingMetricDetails(pending, data, {
          stage: "idle",
          reason: reason || "layout-worker",
          durationMs: pendingDurationMs(pending),
        })
      );
      applyPendingLayoutFallback(pending, reason || "layout-worker");
      return true;
    }

    function recordStaleAfterNodePlan(pending, data = {}) {
      recordLayoutWorkerMetrics(
        "cancel",
        pendingMetricDetails(pending, data, {
          stage: "idle",
          reason: "stale-request-after-node-plan",
        })
      );
      return true;
    }

    function failNodePlan(pending, data = {}, nodePlanResult = {}) {
      const reason = `layout-worker-node-plan-${nodePlanResult?.source || "failed"}`;
      state.layoutWorkerPending = null;
      state.layoutWorkerStage = "idle";
      recordLayoutWorkerMetrics(
        "error",
        pendingMetricDetails(pending, data, {
          stage: "idle",
          reason,
          durationMs: pendingDurationMs(pending),
        })
      );
      try {
        log("WARN", "layout worker node plan failed", {
          reason,
          preset: pending?.preset || "",
          nodes: pending?.nodes?.length || 0,
          edges: pending?.edges?.length || 0,
        });
      } catch (error) {}
      updateLayoutButtons();
      return reason;
    }

    function handleRuntimeError(event, pending) {
      clearPendingTimer(pending);
      state.layoutWorkerPending = null;
      state.layoutWorkerStage = "idle";
      recordLayoutWorkerMetrics(
        "error",
        pendingMetricDetails(pending, {}, {
          stage: "idle",
          reason: "runtime-error",
        })
      );
      try {
        log("WARN", "layout worker runtime failed", {
          message: String(event?.message || ""),
          line: Number(event?.lineno) || 0,
          col: Number(event?.colno) || 0,
        });
      } catch (error) {}
      resetLayoutWorkerInstance("layout-worker-runtime-failed", { reinit: false });
      if (pending) applyPendingLayoutFallback(pending, "layout-worker-runtime");
      scheduleReinit();
      return true;
    }

    function handleTimeout(seq, traceId, timeoutMs) {
      const pending = state.layoutWorkerPending;
      if (!pending || pending.seq !== seq) return false;
      state.layoutWorkerPending = null;
      state.layoutWorkerStage = "idle";
      recordLayoutWorkerMetrics(
        "fallback",
        pendingMetricDetails(pending, { trace_id: traceId }, {
          stage: "idle",
          traceId: pending?.traceId || traceId || "",
          reason: "layout-worker-timeout",
          durationMs: pendingDurationMs(pending),
        })
      );
      try {
        log("WARN", "layout worker timeout fallback", {
          preset: pending?.preset || "",
          nodes: pending?.nodes?.length || 0,
          edges: pending?.edges?.length || 0,
          timeoutMs,
        });
      } catch (error) {}
      resetLayoutWorkerInstance("layout-worker-timeout", { reinit: false });
      applyPendingLayoutFallback(pending, "layout-worker-timeout");
      scheduleReinit();
      return true;
    }

    function handlePostMessageError(error, context = {}) {
      const pending = state.layoutWorkerPending;
      clearPendingTimer(pending);
      state.layoutWorkerPending = null;
      state.layoutWorkerStage = "idle";
      const metricPending =
        pending || {
          seq: context.seq || 0,
          traceId: context.traceId || "",
          preset: context.preset || "",
          nodes: context.nodes || [],
          edges: context.edges || [],
        };
      recordLayoutWorkerMetrics(
        "error",
        pendingMetricDetails(metricPending, {}, {
          stage: "idle",
          reason: "post-message-failed",
        })
      );
      try {
        log("WARN", "layout worker postMessage failed", { message: String(error?.message || error) });
      } catch (logError) {}
      resetLayoutWorkerInstance("layout-worker-post-failed", { reinit: false });
      if (pending) applyPendingLayoutFallback(pending, "layout-worker-post");
      scheduleReinit();
      return false;
    }

    return {
      clearPendingTimer,
      recordProgress,
      recordComplete,
      cancelStaleRequest,
      fallbackToSync,
      recordStaleAfterNodePlan,
      failNodePlan,
      handleRuntimeError,
      handleTimeout,
      handlePostMessageError,
      traceCancellation,
    };
  }

  root.__ANALYTIX_FLOW_LAYOUT_WORKER_LIFECYCLE_CONTROLLER__ = {
    createLayoutWorkerLifecycleController,
  };
})();
