/* Layout prewarm controller.
 * Responsibilities: run background layout workers and warm layout plan cache.
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

  function createLayoutPrewarmController(deps = {}) {
    const state = deps?.state && typeof deps.state === "object" ? deps.state : null;
    if (!state) throw new Error("layout-prewarm-state-missing");
    const runtimeAdapter =
      deps?.runtimeAdapter && typeof deps.runtimeAdapter === "object" ? deps.runtimeAdapter : null;
    if (!runtimeAdapter) throw new Error("layout-prewarm-runtime-missing");
    const layoutPlanCacheController =
      deps?.layoutPlanCacheController && typeof deps.layoutPlanCacheController === "object"
        ? deps.layoutPlanCacheController
        : null;
    if (!layoutPlanCacheController) throw new Error("layout-prewarm-cache-controller-missing");

    const clonePrewarmLayoutInput = requireFunction(
      runtimeAdapter?.clonePrewarmLayoutInput,
      "layout-prewarm-clone-missing"
    );
    const buildLayoutWorkerRequestPayload = requireFunction(
      runtimeAdapter?.buildLayoutWorkerRequestPayload,
      "layout-prewarm-payload-missing"
    );
    const createLayoutWorker = requireFunction(
      runtimeAdapter?.createLayoutWorker,
      "layout-prewarm-worker-missing"
    );
    const makeCacheKey = requireFunction(
      layoutPlanCacheController?.makeCacheKey,
      "layout-prewarm-cache-key-missing"
    );
    const prepareCacheKey = requireFunction(
      layoutPlanCacheController?.prepareCacheKey,
      "layout-prewarm-cache-key-prepare-missing"
    );
    const readCache = requireFunction(
      layoutPlanCacheController?.readCache,
      "layout-prewarm-cache-read-missing"
    );
    const rememberCache = requireFunction(
      layoutPlanCacheController?.rememberCache,
      "layout-prewarm-cache-remember-missing"
    );
    const resolveFocusIdFromNodes = requireFunction(
      deps?.resolveFocusIdFromNodes,
      "layout-prewarm-focus-missing"
    );
    const collectLayoutHints = requireFunction(
      deps?.collectLayoutHints,
      "layout-prewarm-hints-missing"
    );
    const prepareLayoutSemanticProjection = requireFunction(
      deps?.prepareLayoutSemanticProjection,
      "layout-prewarm-semantic-projection-missing"
    );
    const ensureLayoutSemanticState = requireFunction(
      deps?.ensureLayoutSemanticState,
      "layout-prewarm-semantic-state-missing"
    );
    const updateClusterSlotCacheFromLayoutReport = requireFunction(
      deps?.updateClusterSlotCacheFromLayoutReport,
      "layout-prewarm-cluster-cache-missing"
    );
    const prefetchNetworkSectorPlacement = requireFunction(
      deps?.prefetchNetworkSectorPlacement,
      "layout-prewarm-sector-placement-prefetch-missing"
    );
    const logLayoutCoreMissing =
      typeof deps?.logLayoutCoreMissing === "function" ? deps.logLayoutCoreMissing : () => {};
    const shouldLayoutVerboseLog =
      typeof deps?.shouldLayoutVerboseLog === "function" ? deps.shouldLayoutVerboseLog : () => false;
    const log = typeof deps?.log === "function" ? deps.log : () => {};
    const networkMinNodes = Math.max(0, Number(deps?.networkMinNodes) || 0);
    const compactMinNodes = Math.max(0, Number(deps?.compactMinNodes) || 0);
    const timeoutMs = Math.max(0, Number(deps?.timeoutMs) || 0);
    const workerScript = String(deps?.workerScript || "");
    const workerSupported =
      typeof deps?.workerSupported === "boolean"
        ? deps.workerSupported
        : typeof root.Worker !== "undefined";
    const performanceApi = deps?.performanceApi || root.performance || null;
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
    const requestIdleCallbackFn =
      typeof deps?.requestIdleCallbackFn === "function"
        ? deps.requestIdleCallbackFn
        : typeof root.requestIdleCallback === "function"
          ? root.requestIdleCallback.bind(root)
          : null;

    function now() {
      const value = Number(performanceApi?.now?.());
      return Number.isFinite(value) ? value : Date.now();
    }

    function normalizeMode(mode) {
      return String(mode || "").trim().toLowerCase() === "network" ? "network" : "compact";
    }

    function ensureTaskStore() {
      if (!state.layoutPrewarmTasks || typeof state.layoutPrewarmTasks !== "object") {
        state.layoutPrewarmTasks = { network: null, compact: null };
      }
      return state.layoutPrewarmTasks;
    }

    function getTask(mode) {
      const key = normalizeMode(mode);
      return ensureTaskStore()[key] || null;
    }

    function setTask(mode, task) {
      const key = normalizeMode(mode);
      ensureTaskStore()[key] = task || null;
      return true;
    }

    function clearTaskIfCurrent(mode, task) {
      if (getTask(mode) === task) {
        setTask(mode, null);
      }
    }

    function clearTimer(timer) {
      if (!timer) return false;
      try {
        clearTimeoutFn(timer);
      } catch (error) {}
      return true;
    }

    function finishTask(task) {
      if (!task) return false;
      clearTimer(task.timer);
      try {
        task.worker?.terminate?.();
      } catch (error) {}
      task.active = false;
      clearTaskIfCurrent(task.mode, task);
      return true;
    }

    function captureSemanticState(semanticState) {
      return {
        clusterAnchorBySignature: semanticState.clusterAnchorBySignature || {},
        clusterCenterBySignature: semanticState.clusterCenterBySignature || {},
        seedCoreHistory: { ...(semanticState.seedCoreHistory || {}) },
        roleByNodeId: semanticState.roleByNodeId || {},
        clusterByNodeId: semanticState.clusterByNodeId || {},
        layoutCaseByClusterId: semanticState.layoutCaseByClusterId || {},
        lastReports: semanticState.lastReports || null,
      };
    }

    function restoreSemanticState(semanticState, backup) {
      semanticState.clusterAnchorBySignature = backup.clusterAnchorBySignature;
      semanticState.clusterCenterBySignature = backup.clusterCenterBySignature;
      semanticState.seedCoreHistory = backup.seedCoreHistory;
      semanticState.roleByNodeId = backup.roleByNodeId;
      semanticState.clusterByNodeId = backup.clusterByNodeId;
      semanticState.layoutCaseByClusterId = backup.layoutCaseByClusterId;
      semanticState.lastReports = backup.lastReports;
    }

    function resolveInputRows(inputNodes, inputEdges) {
      const nodes = Array.isArray(inputNodes)
        ? inputNodes
        : Array.isArray(state.graph?.data?.nodes)
          ? state.graph.data.nodes
          : [];
      const edges = Array.isArray(inputEdges)
        ? inputEdges
        : Array.isArray(state.graph?.data?.edges)
          ? state.graph.data.edges
          : [];
      return { nodes, edges };
    }

    function shouldSkipForMode(targetMode) {
      const activeMode = String(state.layoutPreset || "").trim().toLowerCase();
      if (targetMode === "network" && activeMode === "network") return true;
      if (targetMode === "compact" && activeMode !== "network") return true;
      if (!workerSupported || state.layoutWorkerDisabledReason) return true;
      if (state.layoutTopologyDirty) return true;
      return false;
    }

    function buildPrewarmPayload(task, nodes, edges, focusId) {
      let layoutHints = {};
      let payload = null;
      const semanticState = ensureLayoutSemanticState();
      const semanticBackup = captureSemanticState(semanticState);
      try {
        const { clonedNodes, clonedEdges } = clonePrewarmLayoutInput(nodes, edges, {
          onMissing: logLayoutCoreMissing,
        });
        layoutHints = collectLayoutHints(clonedNodes, clonedEdges, task.mode, {
          logSource: "prewarm",
          logReason: task.reason || "",
          semanticProjection: task.semanticProjection,
          suppressSemanticLog: !(shouldLayoutVerboseLog() || state.debugTrace || state.debugLayout),
        });
        if (task.mode === "network") {
          layoutHints.collectNetworkSectorPlacementPayloads = true;
        }
        payload = buildLayoutWorkerRequestPayload(
          {
            seq: 1,
            preset: task.mode,
            focusId,
            layoutHints,
            layoutDirection: state.layoutDirection || {},
            nodes: clonedNodes,
            edges: clonedEdges,
          },
          { onMissing: logLayoutCoreMissing }
        );
      } finally {
        restoreSemanticState(semanticState, semanticBackup);
      }
      return { layoutHints, payload };
    }

    function runTask(task, context) {
      if (getTask(task.mode) !== task || !task.active) return false;
      let layoutHints = {};
      let payload = null;
      try {
        const built = buildPrewarmPayload(task, context.nodes, context.edges, context.focusId);
        layoutHints = built.layoutHints;
        payload = built.payload;
      } catch (error) {
        task.semanticProjection = null;
        finishTask(task);
        return false;
      }
      task.semanticProjection = null;

      try {
        task.worker = createLayoutWorker({ script: workerScript });
      } catch (error) {
        finishTask(task);
        return false;
      }
      const worker = task.worker;
      if (!worker) {
        finishTask(task);
        return false;
      }

      task.timer = setTimeoutFn(() => {
        if (!task.active) return;
        finishTask(task);
      }, timeoutMs);
      worker.onmessage = (event) => {
        const data = event?.data || {};
        if (!task.active) return;
        if (Number(data?.seq) !== 1) return;
        const rows = Array.isArray(data?.nodes) ? data.nodes : [];
        const layoutReport =
          data?.layoutReport && typeof data.layoutReport === "object" ? { ...data.layoutReport } : null;
        rememberCache(context.cacheKey, task.mode, rows);
        if (task.mode === "network" && layoutReport) {
          prefetchNetworkSectorPlacementPayloads(layoutReport, task);
          delete layoutReport.networkSectorPlacementPayloads;
        }
        if (layoutReport) {
          updateClusterSlotCacheFromLayoutReport(layoutReport, layoutHints);
        }
        finishTask(task);
      };
      worker.onerror = () => {
        if (!task.active) return;
        finishTask(task);
      };
      try {
        worker.postMessage(payload);
      } catch (error) {
        finishTask(task);
        return false;
      }
      return true;
    }

    function prefetchNetworkSectorPlacementPayloads(layoutReport, task) {
      const rows = Array.isArray(layoutReport?.networkSectorPlacementPayloads)
        ? layoutReport.networkSectorPlacementPayloads
        : [];
      if (!rows.length) return { requested: 0, accepted: 0 };
      let requested = 0;
      let accepted = 0;
      rows.forEach((row) => {
        const payload =
          row?.payload && typeof row.payload === "object"
            ? row.payload
            : row && typeof row === "object"
              ? row
              : null;
        if (!payload) return;
        requested += 1;
        if (prefetchNetworkSectorPlacement(payload)) accepted += 1;
      });
      if (requested > 0 && (state.debugTrace || state.debugLayout)) {
        try {
          log("INFO", "network sector placement prewarm queued", {
            mode: String(task?.mode || ""),
            key: String(task?.key || "").slice(0, 96),
            requested,
            accepted,
          });
        } catch (error) {}
      }
      return { requested, accepted };
    }

    function scheduleIdle(task, context) {
      if (requestIdleCallbackFn) {
        requestIdleCallbackFn(() => runTask(task, context), { timeout: 120 });
        return true;
      }
      setTimeoutFn(() => runTask(task, context), 16);
      return true;
    }

    async function scheduleLayoutPrewarm(mode, { reason = "", nodes: inputNodes = null, edges: inputEdges = null } = {}) {
      const targetMode = normalizeMode(mode);
      if (shouldSkipForMode(targetMode)) return false;
      const { nodes, edges } = resolveInputRows(inputNodes, inputEdges);
      if (!nodes.length || !edges.length) return false;
      const minNodes = targetMode === "network" ? networkMinNodes : compactMinNodes;
      if (nodes.length < minNodes) return false;

      const focusId = resolveFocusIdFromNodes(nodes, state.focusId, state.focusName);
      const cacheKey =
        makeCacheKey(targetMode, focusId, nodes, edges) ||
        (await prepareCacheKey(targetMode, focusId, nodes, edges));
      if (shouldSkipForMode(targetMode)) return false;
      if (!cacheKey) return false;
      if (readCache(cacheKey)) return true;
      const existingTask = getTask(targetMode);
      if (existingTask?.key === cacheKey && existingTask?.active) return true;
      terminateLayoutPrewarmTask("start-next-prewarm", targetMode);
      const task = {
        key: cacheKey,
        mode: targetMode,
        reason: String(reason || ""),
        semanticProjection: null,
        active: true,
        worker: null,
        timer: 0,
        startedAt: now(),
      };
      setTask(targetMode, task);
      let semanticProjection = null;
      try {
        semanticProjection = await prepareLayoutSemanticProjection(nodes, edges, targetMode, {
          reason: `prewarm:${String(reason || targetMode)}`,
          commitSemanticState: false,
        });
      } catch (error) {
        finishTask(task);
        return false;
      }
      if (getTask(targetMode) !== task || !task.active) return false;
      if (shouldSkipForMode(targetMode)) {
        finishTask(task);
        return false;
      }
      if (!semanticProjection || typeof semanticProjection !== "object") {
        finishTask(task);
        return false;
      }
      task.semanticProjection = semanticProjection;
      scheduleIdle(task, { nodes, edges, focusId, cacheKey });
      return true;
    }

    function terminateLayoutPrewarmTask(reason = "", mode = "") {
      const normalizedMode = String(mode || "").trim().toLowerCase();
      const taskModes =
        normalizedMode === "network" || normalizedMode === "compact"
          ? [normalizedMode]
          : ["network", "compact"];
      let cancelled = false;
      taskModes.forEach((taskMode) => {
        const task = getTask(taskMode);
        if (!task) return;
        cancelled = finishTask(task) || cancelled;
        if (state.debugTrace) {
          try {
            log("INFO", "layout prewarm cancelled", {
              reason: String(reason || "cancel"),
              mode: String(task?.mode || taskMode),
              key: String(task?.key || "").slice(0, 96),
            });
          } catch (error) {}
        }
      });
      return cancelled;
    }

    return {
      scheduleLayoutPrewarm,
      terminateLayoutPrewarmTask,
      getTask,
      setTask,
      buildPrewarmPayload,
      runTask,
      prefetchNetworkSectorPlacementPayloads,
    };
  }

  root.__ANALYTIX_FLOW_LAYOUT_PREWARM_CONTROLLER__ = {
    createLayoutPrewarmController,
  };
})();
