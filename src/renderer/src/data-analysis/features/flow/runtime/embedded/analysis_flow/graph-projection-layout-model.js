/* Graph projection layout model.
 * Responsibilities: projection node classification and Rust-backed projection
 * layout seed client. Layout sync rows live in graph-projection-layout-sync-model.js;
 * viewport expansion target projection is Rust-backed via graph render plan.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : typeof window !== "undefined" ? window : null;
  if (!root) return;

  function isClusterProjectionNode(node) {
    if (!node || typeof node !== "object") return false;
    const id = String(node?.id || "").trim();
    return !!node?.cluster_node || id.startsWith("__cluster__::");
  }

  function isCollapsedProjectionNode(node) {
    if (!node || typeof node !== "object") return false;
    if (isClusterProjectionNode(node)) return true;
    const renderMode = String(node?.nodeRenderMode || node?.node_render_mode || "").trim().toLowerCase();
    return renderMode === "dot";
  }

  function isSkeletonProjectionMode(projection) {
    return String(projection?.mode || "").trim().toLowerCase() === "skeleton";
  }

  function buildProjectionAutoMaterializeSignature(snapshotId, viewport) {
    const center = viewport?.center && typeof viewport.center === "object" ? viewport.center : {};
    const size = viewport?.size && typeof viewport.size === "object" ? viewport.size : {};
    const cx = Number.isFinite(Number(center?.x)) ? Math.round(Number(center.x) / 24) : 0;
    const cy = Number.isFinite(Number(center?.y)) ? Math.round(Number(center.y) / 24) : 0;
    const zoom = Number.isFinite(Number(viewport?.zoom)) ? Math.round(Number(viewport.zoom) * 20) / 20 : 0;
    return [
      String(snapshotId || ""),
      `z:${zoom}`,
      `c:${cx},${cy}`,
      `s:${Math.round(Number(size?.width) || 0)}x${Math.round(Number(size?.height) || 0)}`,
    ].join("::");
  }

  function requireFunction(value, message) {
    if (typeof value !== "function") throw new Error(message);
    return value;
  }

  function applyProjectionLayoutSeedUpdates(nodes = [], updates = []) {
    const nodeRows = Array.isArray(nodes) ? nodes : [];
    const updateRows = Array.isArray(updates) ? updates : [];
    if (!nodeRows.length || !updateRows.length) return false;
    for (const update of updateRows) {
      const index = Number(update?.index);
      const id = String(update?.id || "").trim();
      const x = Number(update?.x);
      const y = Number(update?.y);
      if (!Number.isInteger(index) || index < 0 || index >= nodeRows.length || !id || !Number.isFinite(x) || !Number.isFinite(y)) {
        return false;
      }
      const node = nodeRows[index];
      if (!node || typeof node !== "object" || String(node?.id || "").trim() !== id) {
        return false;
      }
    }
    updateRows.forEach((update) => {
      const node = nodeRows[Number(update.index)];
      node.x = Number(update.x);
      node.y = Number(update.y);
    });
    return true;
  }

  function createProjectionLayoutSeedClient(deps = {}) {
    const backendCall = requireFunction(deps?.backendCall, "projection-layout-seed-client-backend-call-missing");
    const parseJSONSafe = requireFunction(deps?.parseJSONSafe, "projection-layout-seed-client-json-missing");
    const getCaseId =
      typeof deps?.getCaseId === "function"
        ? deps.getCaseId
        : () => String(deps?.caseId || "").trim();

    async function projectProjectionLayoutSeed(nodes = [], baseNodes = []) {
      const nextNodes = Array.isArray(nodes) ? nodes : [];
      const previousNodes = Array.isArray(baseNodes) ? baseNodes : [];
      const raw = await backendCall(
        "projectProjectionLayoutSeed",
        JSON.stringify({
          caseId: String(getCaseId() || "").trim(),
          nodes: nextNodes,
          baseNodes: previousNodes,
        })
      );
      const result = parseJSONSafe(raw, {});
      if (!result || result.ok !== true) {
        throw new Error(String(result?.error || "").trim() || "projection-layout-seed-failed");
      }
      const updates = Array.isArray(result.updates) ? result.updates : [];
      if (updates.length && !applyProjectionLayoutSeedUpdates(nextNodes, updates)) {
        throw new Error("projection-layout-seed-updates-invalid");
      }
      const summary = result.summary && typeof result.summary === "object" ? result.summary : {};
      return {
        reused: Number(summary.reused) || 0,
        clusterSeeded: Number(summary.clusterSeeded) || 0,
        fallbackSeeded: Number(summary.fallbackSeeded) || 0,
      };
    }

    return { projectProjectionLayoutSeed };
  }

  root.__ANALYTIX_FLOW_GRAPH_PROJECTION_LAYOUT_MODEL__ = Object.freeze({
    isClusterProjectionNode,
    isSkeletonProjectionMode,
    isCollapsedProjectionNode,
    buildProjectionAutoMaterializeSignature,
    applyProjectionLayoutSeedUpdates,
    createProjectionLayoutSeedClient,
  });
})();
