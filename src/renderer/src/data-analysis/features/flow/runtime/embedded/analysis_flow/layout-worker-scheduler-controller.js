/* Layout worker scheduler controller.
 * Responsibilities: build layout worker requests and manage pending worker submissions.
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

  function createLayoutWorkerSchedulerController(deps = {}) {
    const state = deps?.state && typeof deps.state === "object" ? deps.state : null;
    if (!state) throw new Error("layout-worker-scheduler-state-missing");
    const runtimeAdapter =
      deps?.runtimeAdapter && typeof deps.runtimeAdapter === "object" ? deps.runtimeAdapter : null;
    if (!runtimeAdapter) throw new Error("layout-worker-scheduler-runtime-missing");
    const lifecycleController =
      deps?.lifecycleController && typeof deps.lifecycleController === "object"
        ? deps.lifecycleController
        : null;
    if (!lifecycleController) throw new Error("layout-worker-scheduler-lifecycle-missing");

    const buildLayoutWorkerRequestPayload = requireFunction(
      runtimeAdapter?.buildLayoutWorkerRequestPayload,
      "layout-worker-scheduler-payload-missing"
    );
    const handleTimeout = requireFunction(
      lifecycleController?.handleTimeout,
      "layout-worker-scheduler-timeout-missing"
    );
    const handlePostMessageError = requireFunction(
      lifecycleController?.handlePostMessageError,
      "layout-worker-scheduler-post-error-missing"
    );
    const traceCancellation = requireFunction(
      lifecycleController?.traceCancellation,
      "layout-worker-scheduler-trace-missing"
    );
    const recordLayoutWorkerMetrics = requireFunction(
      deps?.recordLayoutWorkerMetrics,
      "layout-worker-scheduler-metrics-missing"
    );
    const resetLayoutWorkerInstance = requireFunction(
      deps?.resetLayoutWorkerInstance,
      "layout-worker-scheduler-reset-missing"
    );
    const createLayoutTraceId = requireFunction(
      deps?.createLayoutTraceId,
      "layout-worker-scheduler-trace-id-missing"
    );
    const logLayoutCoreMissing =
      typeof deps?.logLayoutCoreMissing === "function" ? deps.logLayoutCoreMissing : () => {};
    const graphLayoutConfig = deps?.graphLayoutConfig || {};
    const timeoutBaseMs = Math.max(0, Number(deps?.timeoutBaseMs) || 0);
    const setTimeoutFn =
      typeof deps?.setTimeoutFn === "function"
        ? deps.setTimeoutFn
        : typeof root.setTimeout === "function"
          ? root.setTimeout.bind(root)
          : () => 0;
    const clearTimeoutFn =
      typeof deps?.clearTimeoutFn === "function"
        ? deps.clearTimeoutFn
        : typeof root.clearTimeout === "function"
          ? root.clearTimeout.bind(root)
          : () => {};
    const performanceApi = deps?.performanceApi || root.performance || null;

    function now() {
      const value = Number(performanceApi?.now?.());
      return Number.isFinite(value) ? value : Date.now();
    }

    function clearTimer(timer) {
      if (!timer) return false;
      try {
        clearTimeoutFn(timer);
      } catch (error) {}
      return true;
    }

    function shouldMergeWithPending(prev, nodes, edges, preset, focusId) {
      return (
        !!prev &&
        prev.nodes === nodes &&
        prev.edges === edges &&
        String(prev.preset || "") === String(preset || "compact") &&
        String(prev.focusId || "") === String(focusId || "")
      );
    }

    function mergePendingApplyIntent(prev) {
      prev.deferApply = {
        ...(prev.deferApply || {}),
        fit: !!(prev.deferApply?.fit || state.graphLoading),
      };
      state.layoutWorkerStage = String(prev.phase || state.layoutWorkerStage || "queued");
      return true;
    }

    function cancelSuperseded(prev, seq) {
      if (!prev) return false;
      clearTimer(prev.timer);
      traceCancellation(prev, "superseded", { nextSeq: seq });
      recordLayoutWorkerMetrics("cancel", {
        seq: prev.seq || 0,
        stage: "idle",
        traceId: prev.traceId || "",
        reason: "superseded",
        preset: prev.preset || "",
        nodes: prev.nodes?.length || 0,
        edges: prev.edges?.length || 0,
      });
      resetLayoutWorkerInstance("layout-worker-superseded");
      return true;
    }

    function buildPending({ seq, traceId, nodes, edges, preset, focusId, payload, layoutCacheKey }) {
      const smallGraphLoadingAnim =
        !!state.graphLoading && (nodes?.length || 0) > 1 && (nodes?.length || 0) <= 20;
      return {
        seq,
        token: state.requestId || "",
        traceId,
        nodes,
        edges,
        preset,
        focusId,
        layoutHints: payload.layoutHints || {},
        layoutCacheKey: String(layoutCacheKey || ""),
        deferApply: {
          animate: smallGraphLoadingAnim,
          duration: smallGraphLoadingAnim ? 760 : 560,
          easing: smallGraphLoadingAnim ? "easeOutCubic" : graphLayoutConfig.ANIM_EASING,
          fit: !!state.graphLoading,
          preserveCenter: null,
        },
        startedAt: now(),
        phase: "queued",
      };
    }

    function resolveTimeoutMs(nodes) {
      return Math.max(1200, timeoutBaseMs + Math.floor((nodes?.length || 0) / 260) * 240);
    }

    function scheduleLayoutWorker(nodes, edges, preset, focusId, layoutHints = {}, layoutCacheKey = "") {
      if (!state.layoutWorker) return false;
      const prev = state.layoutWorkerPending;
      if (shouldMergeWithPending(prev, nodes, edges, preset, focusId)) {
        return mergePendingApplyIntent(prev);
      }
      const seq = ++state.layoutWorkerSeq;
      if (prev) {
        cancelSuperseded(prev, seq);
        if (!state.layoutWorker) return false;
      }

      const payload = buildLayoutWorkerRequestPayload(
        {
          seq,
          preset,
          focusId,
          layoutHints,
          layoutDirection: state.layoutDirection || {},
          nodes,
          edges,
        },
        { onMissing: logLayoutCoreMissing }
      );
      const traceId = createLayoutTraceId(preset);
      const pending = buildPending({
        seq,
        traceId,
        nodes,
        edges,
        preset,
        focusId,
        payload,
        layoutCacheKey,
      });
      state.layoutWorkerPending = pending;
      state.layoutWorkerStage = "queued";
      recordLayoutWorkerMetrics("start", {
        seq,
        stage: "queued",
        traceId,
        preset,
        nodes: nodes?.length || 0,
        edges: edges?.length || 0,
      });

      const timeoutMs = resolveTimeoutMs(nodes);
      pending.timer = setTimeoutFn(() => {
        handleTimeout(seq, traceId, timeoutMs);
      }, timeoutMs);
      try {
        state.layoutWorker.postMessage({ ...payload, trace_id: traceId });
      } catch (error) {
        handlePostMessageError(error, {
          seq,
          traceId,
          preset,
          nodes,
          edges,
        });
        return false;
      }
      return true;
    }

    return {
      scheduleLayoutWorker,
      buildPending,
      resolveTimeoutMs,
      shouldMergeWithPending,
    };
  }

  root.__ANALYTIX_FLOW_LAYOUT_WORKER_SCHEDULER_CONTROLLER__ = {
    createLayoutWorkerSchedulerController,
  };
})();
