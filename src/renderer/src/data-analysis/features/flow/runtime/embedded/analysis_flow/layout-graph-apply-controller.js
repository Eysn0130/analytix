/* Layout graph apply controller.
 * Responsibilities: apply prepared layout node positions to the rendered graph.
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

  function createLayoutGraphApplyController(deps = {}) {
    const buildLayoutNodeTargets = requireFunction(
      deps?.buildLayoutNodeTargets,
      "layout-graph-apply-targets-missing"
    );
    const resolveLayoutApplyPlan = requireFunction(
      deps?.resolveLayoutApplyPlan,
      "layout-graph-apply-plan-missing"
    );
    const animateToLayout = requireFunction(
      deps?.animateToLayout,
      "layout-graph-apply-animate-missing"
    );
    const applyGraphNodeTargets = requireFunction(
      deps?.applyGraphNodeTargets,
      "layout-graph-apply-target-apply-missing"
    );
    const buildGraphPositionUpdates = requireFunction(
      deps?.buildGraphPositionUpdates,
      "layout-graph-apply-position-updates-missing"
    );
    const centerGraphOnLayout = requireFunction(
      deps?.centerGraphOnLayout,
      "layout-graph-apply-center-missing"
    );
    const finalizeView = requireFunction(
      deps?.finalizeView,
      "layout-graph-apply-finalize-missing"
    );
    const updateMinimap = requireFunction(
      deps?.updateMinimap,
      "layout-graph-apply-minimap-missing"
    );
    const scheduleProjectionLayoutIndexSync = requireFunction(
      deps?.scheduleProjectionLayoutIndexSync,
      "layout-graph-apply-sync-missing"
    );
    const publishGraphPatch =
      typeof deps?.publishGraphPatch === "function" ? deps.publishGraphPatch : () => {};
    const graphLayoutConfig = deps?.graphLayoutConfig || {};

    function isLayoutPositionMap(value) {
      return (
        value instanceof Map ||
        (!!value &&
          typeof value === "object" &&
          typeof value.get === "function" &&
          typeof value.has === "function" &&
          typeof value.size === "number")
      );
    }

    function paintGraph(graph) {
      graph?.refreshPositions?.();
      graph?.paint?.();
    }

    function applyTargetsOrPaint(graph, nodes, updates) {
      const applied = applyGraphNodeTargets(graph, nodes, updates, { paint: true });
      if (!applied) paintGraph(graph);
      return applied;
    }

    function scheduleAfterPaint(task) {
      if (typeof task !== "function") return;
      const run = () => setTimeout(task, 0);
      if (typeof requestAnimationFrame === "function") requestAnimationFrame(run);
      else run();
    }

    function applyPreparedLayout({
      graph = null,
      nodes = [],
      deferApply = {},
      syncReason = "",
      patchMeta = null,
      allowAnimation = false,
    } = {}) {
      if (!graph) return false;
      const nodeRows = Array.isArray(nodes) ? nodes : [];
      const applyPlan = resolveLayoutApplyPlan(nodeRows, deferApply || {}, graphLayoutConfig);
      const targets = buildLayoutNodeTargets(nodeRows);
      const targetUpdates = buildGraphPositionUpdates(nodeRows);
      const finalizeApply = () => {
        if (applyPlan.smoothSmallGraph) centerGraphOnLayout(graph, nodeRows);
        if (applyPlan.shouldFit) finalizeView({ fit: true });
        else updateMinimap();
        scheduleProjectionLayoutIndexSync(syncReason || "layout-graph-apply", { delayMs: 80 });
        if (patchMeta) publishGraphPatch(patchMeta);
      };
      const graphNodeCount = graph.getNodes?.().length || 0;
      const canAnimate =
        !!allowAnimation && !!applyPlan.animate && graphNodeCount > 0 && targets.size > 0;
      if (canAnimate) {
        const animated = animateToLayout(
          isLayoutPositionMap(deferApply?.startPositions) ? deferApply.startPositions : null,
          targets,
          {
            graph,
            duration: applyPlan.applyDuration,
            easing: applyPlan.applyEasing,
            finalize: false,
            fit: false,
            onComplete: () => {
              finalizeApply();
            },
          }
        );
        if (animated) return true;
      }
      applyTargetsOrPaint(graph, nodeRows, targetUpdates);
      if (deferApply?.deferFinalize) scheduleAfterPaint(finalizeApply);
      else finalizeApply();
      return true;
    }

    return { applyPreparedLayout };
  }

  root.__ANALYTIX_FLOW_LAYOUT_GRAPH_APPLY_CONTROLLER__ = {
    createLayoutGraphApplyController,
  };
})();
