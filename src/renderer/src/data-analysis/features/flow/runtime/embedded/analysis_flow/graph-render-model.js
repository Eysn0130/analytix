/* Graph render model.
 * Responsibilities: graph tier thresholds and render hint normalization only.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : typeof window !== "undefined" ? window : null;
  if (!root) return;

  const GRAPH_TIER_SMALL_MAX_NODES = 1000;
  const GRAPH_TIER_MEDIUM_MAX_NODES = 10000;
  const GRAPH_TIER_LARGE_MAX_NODES = 100000;

  function clampNumber(value, min, max) {
    const num = Number(value);
    if (!Number.isFinite(num)) return min;
    return Math.max(min, Math.min(max, num));
  }

  function resolveGraphTierByNodeCount(nodeCount = 0) {
    const total = Math.max(0, Number(nodeCount) || 0);
    if (total <= GRAPH_TIER_SMALL_MAX_NODES) return "small";
    if (total <= GRAPH_TIER_MEDIUM_MAX_NODES) return "medium";
    if (total <= GRAPH_TIER_LARGE_MAX_NODES) return "large";
    return "xlarge";
  }

  function normalizeGraphTier(value, nodeCount = 0) {
    const next = String(value || "").trim().toLowerCase();
    if (next === "small" || next === "medium" || next === "large" || next === "xlarge") {
      return next;
    }
    return resolveGraphTierByNodeCount(nodeCount);
  }

  function buildDefaultGraphRenderHints(tier, nodeCount = 0, edgeCount = 0, viewMode = "") {
    const totalNodes = Math.max(0, Number(nodeCount) || 0);
    const totalEdges = Math.max(0, Number(edgeCount) || 0);
    const normalizedTier = normalizeGraphTier(tier, totalNodes);
    const normalizedViewMode = String(viewMode || "").trim().toLowerCase() || "relation";
    if (normalizedTier === "small") {
      return {
        tier: normalizedTier,
        view_mode: normalizedViewMode,
        all_nodes_visible: true,
        default_node_mode: "entity",
        entity_node_limit: totalNodes,
        focus_entity_limit: totalNodes,
        t0_label_limit: totalNodes,
        t1_reveal_batch_size: Math.max(48, Math.min(totalNodes || 48, 120)),
        t1_reveal_delay_ms: 90,
        t1_batch_gap_ms: 12,
        show_edge_labels: totalEdges <= 800,
        show_detail_edge_labels: totalEdges <= 480,
        edge_mode: "full",
        edge_batch_threshold: Math.max(1500, totalEdges + 1),
        edge_batch_size: 80,
        edge_refresh_batch_size: 180,
        animation_mode: "full",
        layout_switch_animate_max_nodes: totalNodes,
        layout_switch_animate_max_edges: totalEdges,
        prefer_fast_first_paint: true,
        projection_auto_expand: false,
      };
    }
    if (normalizedTier === "medium") {
      return {
        tier: normalizedTier,
        view_mode: normalizedViewMode,
        all_nodes_visible: true,
        default_node_mode: "entity",
        entity_node_limit: totalNodes,
        focus_entity_limit: Math.min(totalNodes, 420),
        t0_label_limit: Math.min(totalNodes, totalNodes <= 6000 ? 420 : 560),
        t1_reveal_batch_size: totalNodes <= 3000 ? 180 : 240,
        t1_reveal_delay_ms: totalNodes <= 3000 ? 140 : 180,
        t1_batch_gap_ms: totalNodes <= 3000 ? 18 : 22,
        show_edge_labels: totalEdges <= 2400,
        show_detail_edge_labels: false,
        edge_mode: totalEdges >= 1500 ? "batched" : "full",
        edge_batch_threshold: 1500,
        edge_batch_size: totalEdges < 6000 ? 120 : 180,
        edge_refresh_batch_size: 240,
        animation_mode: "light",
        layout_switch_animate_max_nodes: 1200,
        layout_switch_animate_max_edges: 3600,
        prefer_fast_first_paint: true,
        projection_auto_expand: false,
      };
    }
    if (normalizedTier === "large") {
      return {
        tier: normalizedTier,
        view_mode: normalizedViewMode,
        all_nodes_visible: true,
        default_node_mode: "mixed",
        entity_node_limit: Math.min(Math.max(1800, Math.round(totalNodes / 6)), 4200),
        focus_entity_limit: Math.min(totalNodes, 720),
        t0_label_limit: Math.min(totalNodes, 240),
        t1_reveal_batch_size: 220,
        t1_reveal_delay_ms: 180,
        t1_batch_gap_ms: 24,
        show_edge_labels: false,
        show_detail_edge_labels: false,
        edge_mode: "batched",
        edge_batch_threshold: 900,
        edge_batch_size: 180,
        edge_refresh_batch_size: 320,
        animation_mode: "minimal",
        layout_switch_animate_max_nodes: 0,
        layout_switch_animate_max_edges: 0,
        prefer_fast_first_paint: true,
        projection_auto_expand: false,
      };
    }
    return {
      tier: normalizedTier,
      view_mode: normalizedViewMode,
      all_nodes_visible: true,
      default_node_mode: "mixed",
      entity_node_limit: Math.min(Math.max(1200, Math.round(totalNodes / 10)), 2200),
      focus_entity_limit: Math.min(totalNodes, 480),
      t0_label_limit: Math.min(totalNodes, 120),
      t1_reveal_batch_size: 160,
      t1_reveal_delay_ms: 220,
      t1_batch_gap_ms: 28,
      show_edge_labels: false,
      show_detail_edge_labels: false,
      edge_mode: "batched",
      edge_batch_threshold: 600,
      edge_batch_size: 220,
      edge_refresh_batch_size: 360,
      animation_mode: "minimal",
      layout_switch_animate_max_nodes: 0,
      layout_switch_animate_max_edges: 0,
      prefer_fast_first_paint: true,
      projection_auto_expand: true,
      projection_auto_expand_delay_ms: 280,
      projection_auto_expand_cooldown_ms: 1200,
      projection_auto_expand_min_zoom: 0.18,
      projection_auto_expand_max_clusters: 24,
      projection_auto_expand_max_nodes: 240,
      projection_viewport_materialize_limit: 7500,
    };
  }

  function normalizeGraphRenderHints(value, nodeCount = 0, edgeCount = 0, viewMode = "") {
    const source = value && typeof value === "object" && !Array.isArray(value) ? value : {};
    const totalNodes = Math.max(0, Number(nodeCount) || 0);
    const totalEdges = Math.max(0, Number(edgeCount) || 0);
    const tier = normalizeGraphTier(source.tier || source.graph_tier || value, totalNodes);
    const defaults = buildDefaultGraphRenderHints(tier, totalNodes, totalEdges, viewMode);
    const out = { ...defaults, ...source, tier };
    out.all_nodes_visible = out.all_nodes_visible !== false;
    out.default_node_mode = String(out.default_node_mode || defaults.default_node_mode || "entity").trim().toLowerCase() || "entity";
    out.view_mode = String(out.view_mode || defaults.view_mode || "relation").trim().toLowerCase() || "relation";
    out.edge_mode = String(out.edge_mode || defaults.edge_mode || "full").trim().toLowerCase() || defaults.edge_mode;
    out.animation_mode = String(out.animation_mode || defaults.animation_mode || "light").trim().toLowerCase() || defaults.animation_mode;
    out.entity_node_limit = Math.max(0, Math.round(Number(out.entity_node_limit ?? defaults.entity_node_limit) || 0));
    out.focus_entity_limit = Math.max(0, Math.round(Number(out.focus_entity_limit ?? defaults.focus_entity_limit) || 0));
    out.t0_label_limit = Math.max(0, Math.round(Number(out.t0_label_limit ?? defaults.t0_label_limit) || 0));
    out.t1_reveal_batch_size = Math.max(24, Math.round(Number(out.t1_reveal_batch_size ?? defaults.t1_reveal_batch_size) || 24));
    out.t1_reveal_delay_ms = Math.max(0, Math.round(Number(out.t1_reveal_delay_ms ?? defaults.t1_reveal_delay_ms) || 0));
    out.t1_batch_gap_ms = Math.max(8, Math.round(Number(out.t1_batch_gap_ms ?? defaults.t1_batch_gap_ms) || 8));
    out.show_edge_labels = out.show_edge_labels !== false;
    out.show_detail_edge_labels = !!out.show_detail_edge_labels;
    out.edge_batch_threshold = Math.max(0, Math.round(Number(out.edge_batch_threshold ?? defaults.edge_batch_threshold) || 0));
    out.edge_batch_size = Math.max(48, Math.round(Number(out.edge_batch_size ?? defaults.edge_batch_size) || 48));
    out.edge_refresh_batch_size = Math.max(
      96,
      Math.round(Number(out.edge_refresh_batch_size ?? defaults.edge_refresh_batch_size) || 96)
    );
    out.layout_switch_animate_max_nodes = Math.max(
      0,
      Math.round(Number(out.layout_switch_animate_max_nodes ?? defaults.layout_switch_animate_max_nodes) || 0)
    );
    out.layout_switch_animate_max_edges = Math.max(
      0,
      Math.round(Number(out.layout_switch_animate_max_edges ?? defaults.layout_switch_animate_max_edges) || 0)
    );
    out.projection_auto_expand = out.projection_auto_expand !== false;
    out.projection_auto_expand_delay_ms = Math.max(
      120,
      Math.round(Number(out.projection_auto_expand_delay_ms ?? defaults.projection_auto_expand_delay_ms) || 120)
    );
    out.projection_auto_expand_cooldown_ms = Math.max(
      300,
      Math.round(Number(out.projection_auto_expand_cooldown_ms ?? defaults.projection_auto_expand_cooldown_ms) || 300)
    );
    out.projection_auto_expand_min_zoom = clampNumber(
      Number(out.projection_auto_expand_min_zoom ?? defaults.projection_auto_expand_min_zoom),
      0.05,
      4
    );
    out.projection_auto_expand_max_clusters = Math.max(
      1,
      Math.round(Number(out.projection_auto_expand_max_clusters ?? defaults.projection_auto_expand_max_clusters) || 1)
    );
    out.projection_auto_expand_max_nodes = Math.max(
      1,
      Math.round(Number(out.projection_auto_expand_max_nodes ?? defaults.projection_auto_expand_max_nodes) || 1)
    );
    out.projection_viewport_materialize_limit = Math.max(
      500,
      Math.round(Number(out.projection_viewport_materialize_limit ?? defaults.projection_viewport_materialize_limit) || 500)
    );
    out.prefer_fast_first_paint = out.prefer_fast_first_paint !== false;
    return out;
  }

  function resolveNodeBaseRadius(node, graphStyle = {}) {
    const styleBase = Number(graphStyle?.nodeSize ?? 18) || 18;
    const preferred = Number(node?.__baseR);
    if (Number.isFinite(preferred) && preferred > 0) return preferred;
    const fallback = Number(node?.r);
    if (Number.isFinite(fallback) && fallback > 0) return fallback;
    return styleBase;
  }

  function resolveNodeBaseLineWidth(node, graphStyle = {}) {
    const styleBase = Number(graphStyle?.nodeWidth ?? 2) || 2;
    const preferred = Number(node?.__baseLineWidth);
    if (Number.isFinite(preferred) && preferred > 0) return preferred;
    const fallback = Number(node?.lineWidth);
    if (Number.isFinite(fallback) && fallback > 0) return fallback;
    return styleBase;
  }

  const NODE_TIER_RENDER_STATE_KEYS = Object.freeze([
    "__tierBaseR",
    "__tierBaseLineWidth",
    "__tierBaseNodeShadow",
    "__tierBaseIcon",
    "__tierBaseIconSymbol",
    "__tierBaseIconSize",
    "__tierBaseNodeLabelHidden",
    "__tierLabelSuppressed",
  ]);
  const EDGE_TIER_RENDER_STATE_KEYS = Object.freeze([
    "__tierBaseLabel",
    "__tierBaseLabelTop",
    "__tierBaseLabelBottom",
    "__tierBaseDetailLabel",
    "__tierLabelSuppressed",
  ]);

  function hasOwnField(row, key) {
    return !!row && Object.prototype.hasOwnProperty.call(row, key);
  }

  function buildGraphNodeRenderUpdate(node) {
    const row = node && typeof node === "object" ? node : {};
    return {
      r: row.r,
      lineWidth: row.lineWidth,
      nodeShadow: !!row.nodeShadow,
      icon: row.icon || "",
      iconSymbol: row.iconSymbol || "",
      iconSize: Number(row.iconSize || 0),
      nodeLabelHidden: !!row.nodeLabelHidden,
      nodeRenderMode: row.nodeRenderMode || "entity",
    };
  }

  function buildGraphEdgeRenderUpdate(edge) {
    const row = edge && typeof edge === "object" ? edge : {};
    return {
      label: row.label || "",
      labelTop: row.labelTop || "",
      labelBottom: row.labelBottom || "",
      detailLabel: !!row.detailLabel,
      edgeLabelHidden: !!row.edgeLabelHidden,
    };
  }

  function resolveRenderUpdateRow(rows, update) {
    if (!Array.isArray(rows) || !update || typeof update !== "object") return null;
    const targetIndex = Number.isInteger(update.index) ? update.index : -1;
    if (targetIndex < 0 || targetIndex >= rows.length) return null;
    const target = rows[targetIndex];
    if (!target || typeof target !== "object") return null;
    const updateId = String(update.id || "").trim();
    if (updateId && String(target.id || "").trim() !== updateId) return null;
    return target;
  }

  function applyRenderUpdateRow(rows, update, keys) {
    const target = resolveRenderUpdateRow(rows, update);
    if (!target) return false;
    keys.forEach((key) => {
      if (hasOwnField(update, key)) target[key] = update[key];
    });
    return true;
  }

  function applyGraphRenderPlanUpdates(nodes = [], edges = [], renderPlan = {}) {
    const nodeUpdateKeys = [
      "r",
      "lineWidth",
      "nodeShadow",
      "icon",
      "iconSymbol",
      "iconSize",
      "nodeLabelHidden",
      "nodeRenderMode",
      ...NODE_TIER_RENDER_STATE_KEYS,
    ];
    const edgeUpdateKeys = [
      "label",
      "labelTop",
      "labelBottom",
      "detailLabel",
      "mode",
      "edgeArrow",
      "showArrow",
      "edgeLabelHidden",
      ...EDGE_TIER_RENDER_STATE_KEYS,
    ];
    let nodeUpdates = 0;
    let edgeUpdates = 0;
    (Array.isArray(renderPlan?.nodeRenderUpdates) ? renderPlan.nodeRenderUpdates : []).forEach((update) => {
      if (applyRenderUpdateRow(nodes, update, nodeUpdateKeys)) nodeUpdates += 1;
    });
    (Array.isArray(renderPlan?.edgeRenderUpdates) ? renderPlan.edgeRenderUpdates : []).forEach((update) => {
      if (applyRenderUpdateRow(edges, update, edgeUpdateKeys)) edgeUpdates += 1;
    });
    return { nodeUpdates, edgeUpdates };
  }

  function syncGraphRenderUpdateItems(graph, rows, updates, buildUpdate) {
    if (!graph || typeof graph.findById !== "function" || typeof graph.updateItem !== "function") return 0;
    if (!Array.isArray(rows) || !Array.isArray(updates) || typeof buildUpdate !== "function") return 0;
    let applied = 0;
    updates.forEach((update) => {
      const source = resolveRenderUpdateRow(rows, update);
      if (!source) return;
      const id = String(update?.id || source.id || "").trim();
      if (!id) return;
      const item = graph.findById(id);
      if (!item) return;
      graph.updateItem(item, buildUpdate(source));
      applied += 1;
    });
    return applied;
  }

  function syncGraphRenderItems(graph, nodes = [], edges = [], renderPlan = {}) {
    if (!graph || typeof graph.updateItem !== "function") return { nodeUpdates: 0, edgeUpdates: 0 };
    const nodeUpdates = syncGraphRenderUpdateItems(
      graph,
      nodes,
      Array.isArray(renderPlan?.nodeRenderUpdates) ? renderPlan.nodeRenderUpdates : [],
      buildGraphNodeRenderUpdate
    );
    const edgeUpdates = syncGraphRenderUpdateItems(
      graph,
      edges,
      Array.isArray(renderPlan?.edgeRenderUpdates) ? renderPlan.edgeRenderUpdates : [],
      buildGraphEdgeRenderUpdate
    );
    return { nodeUpdates, edgeUpdates };
  }

  function applyGraphNodeTargets(graph, nodes = [], updates = [], { paint = true } = {}) {
    if (!graph) return 0;
    if (!Array.isArray(nodes) || !Array.isArray(updates) || !updates.length) return 0;
    graph.setAutoPaint?.(false);
    let applied = 0;
    try {
      applied = syncGraphItemPositionsFromUpdates(graph, nodes, updates, { paint: false });
    } catch (e) {}
    graph.setAutoPaint?.(true);
    if (paint) graph.paint?.();
    return applied;
  }

  function buildGraphPositionUpdates(nodes = []) {
    const updates = [];
    (Array.isArray(nodes) ? nodes : []).forEach((row, index) => {
      if (!row || typeof row !== "object") return;
      const id = String(row.id || "").trim();
      if (!id) return;
      const x = Number(row.x);
      const y = Number(row.y);
      if (!Number.isFinite(x) && !Number.isFinite(y)) return;
      const update = { index, id };
      if (Number.isFinite(x)) update.x = x;
      if (Number.isFinite(y)) update.y = y;
      updates.push(update);
    });
    return updates;
  }

  function syncGraphItemPositionsFromUpdates(graph, nodes = [], updates = [], { paint = true } = {}) {
    if (!graph || typeof graph.findById !== "function" || !Array.isArray(nodes) || !Array.isArray(updates)) return 0;
    let applied = 0;
    updates.forEach((update) => {
      const source = resolveRenderUpdateRow(nodes, update);
      if (!source) return;
      const id = String(update?.id || source.id || "").trim();
      if (!id) return;
      const item = graph.findById(id);
      const model = item?.getModel?.();
      if (!model) return;
      let changed = false;
      if (Number.isFinite(update.x)) {
        model.x = update.x;
        changed = true;
      }
      if (Number.isFinite(update.y)) {
        model.y = update.y;
        changed = true;
      }
      if (changed) applied += 1;
    });
    graph.refreshPositions?.();
    if (paint) graph.paint?.();
    return applied;
  }

  function summarizeGraphItemLayout(graph) {
    const nodes = graph?.getNodes?.() || [];
    const out = { count: nodes.length, finite: 0, minX: null, maxX: null, minY: null, maxY: null };
    for (const node of nodes) {
      const model = node?.getModel?.() || {};
      const x = Number(model.x);
      const y = Number(model.y);
      if (!Number.isFinite(x) || !Number.isFinite(y)) continue;
      out.finite += 1;
      out.minX = out.minX == null ? x : Math.min(out.minX, x);
      out.maxX = out.maxX == null ? x : Math.max(out.maxX, x);
      out.minY = out.minY == null ? y : Math.min(out.minY, y);
      out.maxY = out.maxY == null ? y : Math.max(out.maxY, y);
    }
    return out;
  }

  function isLayoutCollapsed(summary) {
    if (!summary || summary.count < 2) return false;
    const spanX = summary.maxX != null && summary.minX != null ? summary.maxX - summary.minX : 0;
    const spanY = summary.maxY != null && summary.minY != null ? summary.maxY - summary.minY : 0;
    return !summary.finite || summary.finite < summary.count || (spanX < 1 && spanY < 1);
  }

  function syncGraphItemPositionsFromData(graph, nodes, reason = "", { force = false, log = null } = {}) {
    if (!graph || !Array.isArray(nodes) || nodes.length < 2) return false;
    const before = summarizeGraphItemLayout(graph);
    if (!force && !isLayoutCollapsed(before)) return false;
    const updates = buildGraphPositionUpdates(nodes);
    graph.setAutoPaint(false);
    try {
      syncGraphItemPositionsFromUpdates(graph, nodes, updates, { paint: false });
    } catch (e) {}
    graph.setAutoPaint(true);
    graph.paint?.();
    const after = summarizeGraphItemLayout(graph);
    try {
      if (typeof log === "function") {
        log("WARN", "graph item layout collapsed, positions synced", {
          reason,
          before,
          after,
          forced: !!force,
        });
      }
    } catch (e) {}
    return true;
  }

  root.AnalytixGraphRenderModel = Object.freeze({
    GRAPH_TIER_SMALL_MAX_NODES,
    GRAPH_TIER_MEDIUM_MAX_NODES,
    GRAPH_TIER_LARGE_MAX_NODES,
    resolveGraphTierByNodeCount,
    normalizeGraphTier,
    buildDefaultGraphRenderHints,
    normalizeGraphRenderHints,
    resolveNodeBaseRadius,
    resolveNodeBaseLineWidth,
    buildGraphNodeRenderUpdate,
    buildGraphEdgeRenderUpdate,
    applyGraphRenderPlanUpdates,
    syncGraphRenderItems,
    applyGraphNodeTargets,
    buildGraphPositionUpdates,
    syncGraphItemPositionsFromUpdates,
    summarizeGraphItemLayout,
    isLayoutCollapsed,
    syncGraphItemPositionsFromData,
  });
})();
