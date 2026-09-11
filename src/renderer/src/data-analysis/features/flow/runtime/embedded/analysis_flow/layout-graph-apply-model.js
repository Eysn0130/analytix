/* Layout graph apply model.
 * Responsibilities: derive rendered graph target maps and UI apply plans.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof window !== "undefined" ? window : null;
  if (!root) return;

  function buildLayoutNodeTargets(nodes) {
    return new Map(
      (Array.isArray(nodes) ? nodes : []).map((node) => [
        node?.id,
        { x: node?.x, y: node?.y },
      ])
    );
  }

  function resolveLayoutApplyPlan(nodes, deferApply = {}, graphLayoutConfig = {}) {
    const nodeCount = Array.isArray(nodes) ? nodes.length : Math.max(0, Number(nodes?.length) || 0);
    const preserveCenter = !!deferApply?.preserveCenter;
    const smoothSmallGraph = nodeCount > 1 && nodeCount <= 20 && !preserveCenter;
    const defaultDuration = smoothSmallGraph ? 760 : 560;
    const applyDuration = Math.max(260, Number(deferApply?.duration) || defaultDuration);
    const applyEasing =
      deferApply?.easing || (smoothSmallGraph ? "easeOutCubic" : graphLayoutConfig?.ANIM_EASING);
    return {
      nodeCount,
      preserveCenter,
      smoothSmallGraph,
      applyDuration,
      applyEasing,
      shouldFit: !!deferApply?.fit || smoothSmallGraph,
      animate: !!deferApply?.animate,
    };
  }

  root.__ANALYTIX_FLOW_LAYOUT_GRAPH_APPLY_MODEL__ = {
    buildLayoutNodeTargets,
    resolveLayoutApplyPlan,
  };
})();
