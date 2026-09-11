/* Graph edge offset model.
 * Responsibilities: apply Rust-projected edge offsets and summarize offset state.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : typeof window !== "undefined" ? window : null;
  if (!root) return;

  function edgeText(value) {
    return String(value || "").trim();
  }

  function edgeId(edge) {
    return String(edge?.id || "");
  }

  function buildEdgeOffsetIndex(edges = []) {
    const out = new Map();
    (Array.isArray(edges) ? edges : []).forEach((edge, index) => {
      const id = edgeId(edge);
      if (id) out.set(id, index);
    });
    return out;
  }

  function applyEdgeOffsetUpdates(edges = [], updates = []) {
    const rows = Array.isArray(edges) ? edges : [];
    const updateRows = Array.isArray(updates) ? updates : [];
    if (!rows.length || !updateRows.length) return 0;
    const idIndex = buildEdgeOffsetIndex(rows);
    let applied = 0;
    updateRows.forEach((update) => {
      if (!update || typeof update !== "object") return;
      let index = Number.isInteger(update.index) ? update.index : -1;
      if (index < 0 || index >= rows.length) {
        const id = String(update.id || "");
        index = id ? Number(idIndex.get(id)) : -1;
      }
      const edge = rows[index];
      if (!edge) return;
      edge.edgeOffset = update.edgeOffset;
      applied += 1;
    });
    return applied;
  }

  function summarizeEdgeOffsets(edges = []) {
    if (!Array.isArray(edges) || edges.length === 0) {
      return { pairs: 0, multiPairs: 0, offsetEdges: 0, maxPerPair: 0, samples: [] };
    }
    const pairMap = new Map();
    edges.forEach((edge) => {
      if (!edge) return;
      const source = edgeText(edge.source);
      const target = edgeText(edge.target);
      if (!source || !target || source === target) return;
      const key = source < target ? `${source}::${target}` : `${target}::${source}`;
      if (!pairMap.has(key)) pairMap.set(key, []);
      pairMap.get(key).push(edge);
    });
    let multiPairs = 0;
    let maxPerPair = 0;
    let offsetEdges = 0;
    const samples = [];
    for (const [key, list] of pairMap.entries()) {
      const count = list.length;
      if (count > 1) {
        multiPairs += 1;
        if (samples.length < 3) samples.push({ pair: key, count });
      }
      if (count > maxPerPair) maxPerPair = count;
      list.forEach((edge) => {
        if (Number(edge.edgeOffset || 0)) offsetEdges += 1;
      });
    }
    return {
      pairs: pairMap.size,
      multiPairs,
      offsetEdges,
      maxPerPair,
      samples,
    };
  }

  root.__ANALYTIX_FLOW_GRAPH_EDGE_OFFSET_MODEL__ = Object.freeze({
    applyEdgeOffsetUpdates,
    summarizeEdgeOffsets,
  });
})();
