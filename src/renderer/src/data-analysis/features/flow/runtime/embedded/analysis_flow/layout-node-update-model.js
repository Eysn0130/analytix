/* Layout node update model.
 * Responsibilities: validate and apply Rust layout update rows.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof window !== "undefined" ? window : null;
  if (!root) return;

  function normalizeLayoutNodeUpdates(value) {
    const rows = Array.isArray(value) ? value : [];
    if (!rows.length) return null;
    const updates = [];
    for (let i = 0; i < rows.length; i += 1) {
      const next = rows[i];
      if (!next || typeof next !== "object") return null;
      const index = Number(next?.index);
      const id = String(next?.id || "").trim();
      if (!Number.isInteger(index) || index !== i || !id) return null;
      const x = Number(next?.x);
      const y = Number(next?.y);
      if (!Number.isFinite(x) || !Number.isFinite(y)) return null;
      updates.push({ ...next, index: i, id, x, y });
    }
    return updates;
  }

  function applyLayoutNodeUpdates(nodeRows, updates, metaKeys = []) {
    const rows = Array.isArray(nodeRows) ? nodeRows : [];
    const nextRows = normalizeLayoutNodeUpdates(updates);
    if (!rows.length || !nextRows || rows.length !== nextRows.length) return false;
    for (let i = 0; i < rows.length; i += 1) {
      const node = rows[i];
      const next = nextRows[i];
      if (!node || typeof node !== "object") return false;
      const id = String(node?.id || "").trim();
      if (!id || String(next?.id || "").trim() !== id) return false;
    }
    rows.forEach((node, i) => {
      const next = nextRows[i];
      node.x = Number(next.x);
      node.y = Number(next.y);
      (Array.isArray(metaKeys) ? metaKeys : []).forEach((metaKey) => {
        const value = next?.[metaKey];
        if (value == null || value === "") {
          delete node[metaKey];
          return;
        }
        node[metaKey] = value;
      });
    });
    return true;
  }

  function adoptRustNodePlanUpdates(nodeRows, result, metaKeys = []) {
    if (result?.applied !== true) return false;
    return applyLayoutNodeUpdates(nodeRows, result?.updates, metaKeys);
  }

  root.__ANALYTIX_FLOW_LAYOUT_NODE_UPDATE_MODEL__ = {
    normalizeLayoutNodeUpdates,
    applyLayoutNodeUpdates,
    adoptRustNodePlanUpdates,
  };
})();
