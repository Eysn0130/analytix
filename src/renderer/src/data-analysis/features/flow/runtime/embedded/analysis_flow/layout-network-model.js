/* Layout network model.
 * Responsibilities: summarize network layout bounds used by the page.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof window !== "undefined" ? window : null;
  if (!root) return;

  function summarizeLayout(nodes) {
    const rows = Array.isArray(nodes) ? nodes : [];
    const out = { count: rows.length, finite: 0, minX: null, maxX: null, minY: null, maxY: null };
    for (const n of rows) {
      const x = Number(n.x);
      const y = Number(n.y);
      if (!Number.isFinite(x) || !Number.isFinite(y)) continue;
      out.finite += 1;
      out.minX = out.minX == null ? x : Math.min(out.minX, x);
      out.maxX = out.maxX == null ? x : Math.max(out.maxX, x);
      out.minY = out.minY == null ? y : Math.min(out.minY, y);
      out.maxY = out.maxY == null ? y : Math.max(out.maxY, y);
    }
    return out;
  }

  root.__ANALYTIX_FLOW_LAYOUT_NETWORK_MODEL__ = {
    summarizeLayout,
  };
})();
