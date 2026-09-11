/* Layout worker result model.
 * Responsibilities: adopt Rust node-plan results.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof window !== "undefined" ? window : null;
  if (!root) return;

  const updateModel = root.__ANALYTIX_FLOW_LAYOUT_NODE_UPDATE_MODEL__;

  function requireFunction(fn, message) {
    if (typeof fn !== "function") {
      throw new Error(message);
    }
    return fn;
  }

  const adoptRustNodePlanUpdates = requireFunction(
    updateModel?.adoptRustNodePlanUpdates,
    "layout-node-update-model-missing"
  );

  async function applyLayoutWorkerNodePlan(nodes, workerNodes, options = {}) {
    const nodeRows = Array.isArray(nodes) ? nodes : [];
    const workerRows = Array.isArray(workerNodes) ? workerNodes : [];
    if (!nodeRows.length || !workerRows.length) {
      return { applied: false, source: "empty", matched: 0 };
    }
    const computeLayoutNodePlan =
      typeof options?.computeLayoutNodePlan === "function" ? options.computeLayoutNodePlan : null;
    if (!computeLayoutNodePlan) {
      return { applied: false, source: "plan-missing", matched: 0 };
    }
    let result = null;
    try {
      result = await computeLayoutNodePlan({
        operation: "apply_worker",
        nodes: nodeRows,
        workerNodes: workerRows,
        metaKeys: Array.isArray(options?.metaKeys) ? options.metaKeys : [],
      });
    } catch (error) {
      return { applied: false, source: "compute-failed", matched: 0 };
    }
    if (typeof options?.beforeApply === "function" && options.beforeApply() === false) {
      return { applied: false, source: "stale", matched: 0 };
    }
    if (adoptRustNodePlanUpdates(nodeRows, result, options?.metaKeys)) {
      return {
        applied: true,
        source: String(result?.source || "plan"),
        matched: Number(result?.matched) || nodeRows.length,
        gridFallbackLikely: result?.gridFallbackLikely === true,
      };
    }
    return { applied: false, source: String(result?.source || "apply-failed"), matched: 0 };
  }

  root.__ANALYTIX_FLOW_LAYOUT_WORKER_RESULT_MODEL__ = {
    adoptRustNodePlanUpdates,
    applyLayoutWorkerNodePlan,
  };
})();
