/* Layout worker request/result contract.
 * Responsibilities: normalize worker payloads and project layout results only.
 */
/* eslint-disable no-restricted-globals */

(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : typeof window !== "undefined" ? window : null;
  if (!root) return;

  const LAYOUT_META_KEYS = [
    "layoutBand",
    "role",
    "clusterId",
    "seedCoreCandidate",
    "isSeedSelected",
    "demoteReason",
    "isPathPromotedAdjacent",
    "isBridgeAdjacent",
    "isHubAdjacent",
    "layoutAnchorForCluster",
    "weight",
    "prevX",
    "prevY",
    "layoutCase",
    "layoutClusterId",
    "layoutRoleWhy",
    "layoutCore",
    "layoutFanRole",
    "layoutFanRadius",
    "layoutFanAmount",
    "layoutCorridor",
    "layoutCorridorLevel",
    "layoutCorridorT",
    "layoutCorridorOffset",
    "layoutTerritory",
    "layoutCorridorRank",
    "layoutBridge",
    "layoutCommunity",
    "layoutLevel",
    "layoutVisibilityTier",
    "layoutLabelTier",
    "layoutBridgeScore",
    "layoutExternalAngle",
  ];
  const LAYOUT_CONTRACT_VERSION = "layout-contract-v1";
  const LAYOUT_PRESET_ALIASES = Object.freeze({
    compact: "compact",
    relation: "compact",
    network: "network",
    hierarchy: "hierarchy",
    flow: "flow",
  });
  const LAYOUT_PREWARM_NODE_FIELDS = [
    "id",
    "ntype",
    "title",
    "name",
    "display_id",
    "displayId",
    "displayIdRaw",
    "display_ids",
    "displayIds",
    "mergedDisplay",
    "x",
    "y",
    "r",
    "fontSize",
    "total_amount",
    "totalAmount",
    "amount",
    "total_amt",
    "totalAmt",
    "total_count",
    "weight",
  ];
  const LAYOUT_PREWARM_EDGE_FIELDS = [
    "id",
    "source",
    "target",
    "mode",
    "edgeArrow",
    "showArrow",
    "arrow",
    "isVisible",
    "amount",
    "forward_amount",
    "reverse_amount",
    "out_amount",
    "in_amount",
  ];

  function normalizeLayoutPreset(value) {
    const raw = String(value || "").trim().toLowerCase();
    return LAYOUT_PRESET_ALIASES[raw] || "compact";
  }

  function normalizeLayoutDirection(dir) {
    const input = dir && typeof dir === "object" ? dir : {};
    return {
      hierarchy: String(input?.hierarchy || "").trim().toLowerCase() === "up" ? "up" : "down",
      flow: String(input?.flow || "").trim().toLowerCase() === "left" ? "left" : "right",
    };
  }

  function normalizeLayoutStringList(values, limit = 256) {
    const out = [];
    const seen = new Set();
    (Array.isArray(values) ? values : []).forEach((value) => {
      if (out.length >= Math.max(1, Number(limit) || 0)) return;
      const text = String(value || "").trim();
      if (!text || seen.has(text)) return;
      seen.add(text);
      out.push(text);
    });
    return out;
  }

  function normalizeLayoutObjectMap(value) {
    return value && typeof value === "object" && !Array.isArray(value) ? value : {};
  }

  function normalizeCorePlacement(value) {
    const v = String(value || "").toLowerCase();
    if (v === "ring") return "ring";
    if (v === "soft-dynamic") return "soft-dynamic";
    return "semantic-corridor";
  }

  function normalizeCorePin(value) {
    return String(value || "").toLowerCase() === "hard" ? "hard" : "none";
  }

  function normalizeCoreSource(value) {
    return String(value || "").toLowerCase() === "hint-first" ? "hint-first" : "structure-auto";
  }

  function normalizeLayoutWorkerHints(hints = {}) {
    const src = hints && typeof hints === "object" ? hints : {};
    const out = {
      preferredCoreIds: normalizeLayoutStringList(src?.preferredCoreIds, 128),
      corePlacement: normalizeCorePlacement(src?.corePlacement),
      corePin: normalizeCorePin(src?.corePin),
      coreSource: normalizeCoreSource(src?.coreSource),
    };
    if (src?.collectNetworkSectorPlacementPayloads === true) {
      out.collectNetworkSectorPlacementPayloads = true;
    }
    if (src?.semantic && typeof src.semantic === "object") {
      const semantic = src.semantic;
      out.semantic = {
        mode: semantic?.mode,
        config: semantic?.config,
        nodeMetaById: semantic?.nodeMetaById,
        leafEntryById: semantic?.leafEntryById,
        clusters: semantic?.clusters,
        clusterPlacementById: semantic?.clusterPlacementById,
        networkNodeUpdatesByClusterId: normalizeLayoutObjectMap(semantic?.networkNodeUpdatesByClusterId),
        networkCenterReportsByClusterId: normalizeLayoutObjectMap(semantic?.networkCenterReportsByClusterId),
        networkLeafZonesByClusterId: normalizeLayoutObjectMap(semantic?.networkLeafZonesByClusterId),
        canvasBounds: semantic?.canvasBounds,
      };
    }
    return out;
  }

  function buildLayoutWorkerNodesPayload(nodes) {
    return (Array.isArray(nodes) ? nodes : []).map((node) => {
      const row = node && typeof node === "object" ? node : {};
      const out = {
        id: row.id,
        ntype: row.ntype || "",
        title: row.title || "",
        name: row.name || "",
        display_id: row.display_id || row.displayId || row.displayIdRaw || "",
        displayId: row.displayId || row.display_id || row.displayIdRaw || "",
        displayIdRaw: row.displayIdRaw || row.display_id || row.displayId || "",
      };
      if (Array.isArray(row?.display_ids)) out.display_ids = row.display_ids.slice(0, 8);
      if (Array.isArray(row?.displayIds)) out.displayIds = row.displayIds.slice(0, 8);
      if (row?.mergedDisplay != null) out.mergedDisplay = !!row.mergedDisplay;
      if (Number.isFinite(Number(row?.x))) out.x = Number(row.x);
      if (Number.isFinite(Number(row?.y))) out.y = Number(row.y);
      if (Number.isFinite(Number(row?.r))) out.r = Number(row.r);
      if (Number.isFinite(Number(row?.fontSize))) out.fontSize = Number(row.fontSize);
      return out;
    });
  }

  function buildLayoutWorkerEdgesPayload(edges) {
    return (Array.isArray(edges) ? edges : []).map((edge) => {
      const row = edge && typeof edge === "object" ? edge : {};
      const out = {
        source: row.source,
        target: row.target,
        mode: row.mode,
        edgeArrow: row.edgeArrow,
        showArrow: row.showArrow,
        arrow: row.arrow,
      };
      if (Number.isFinite(Number(row?.amount))) out.amount = Number(row.amount);
      if (Number.isFinite(Number(row?.forward_amount))) out.forward_amount = Number(row.forward_amount);
      if (Number.isFinite(Number(row?.reverse_amount))) out.reverse_amount = Number(row.reverse_amount);
      if (Number.isFinite(Number(row?.out_amount))) out.out_amount = Number(row.out_amount);
      if (Number.isFinite(Number(row?.in_amount))) out.in_amount = Number(row.in_amount);
      return out;
    });
  }

  function projectLayoutNode(node, metaKeys = LAYOUT_META_KEYS) {
    if (!node || typeof node !== "object") return null;
    const id = String(node?.id || "").trim();
    if (!id) return null;
    const row = { id };
    const x = Number(node?.x);
    const y = Number(node?.y);
    if (Number.isFinite(x)) row.x = x;
    if (Number.isFinite(y)) row.y = y;
    (Array.isArray(metaKeys) ? metaKeys : []).forEach((key) => {
      const value = node[key];
      if (value == null || value === "") return;
      row[key] = value;
    });
    return row;
  }

  function projectLayoutNodes(nodes, metaKeys = LAYOUT_META_KEYS) {
    const out = [];
    (Array.isArray(nodes) ? nodes : []).forEach((node) => {
      const row = projectLayoutNode(node, metaKeys);
      if (!row) return;
      out.push(row);
    });
    return out;
  }

  function summarizeProjectedLayout(nodes) {
    const rows = Array.isArray(nodes) ? nodes : [];
    const bandCounts = {};
    let positionedCount = 0;
    const coreIds = [];
    rows.forEach((row) => {
      const x = Number(row?.x);
      const y = Number(row?.y);
      if (Number.isFinite(x) && Number.isFinite(y)) positionedCount += 1;
      const band = String(row?.layoutBand || "").trim();
      if (band) bandCounts[band] = (bandCounts[band] || 0) + 1;
      if (coreIds.length < 20 && band === "core" && row?.id != null) {
        coreIds.push(String(row.id));
      }
    });
    return {
      nodeCount: rows.length,
      positionedCount,
      coreCount: coreIds.length,
      coreIds,
      bandCounts,
    };
  }

  function buildLayoutWorkerRequest(payload = {}) {
    const src = payload && typeof payload === "object" ? payload : {};
    return {
      type: "layout",
      contractVersion: LAYOUT_CONTRACT_VERSION,
      seq: Number(src?.seq) || 0,
      trace_id: String(src?.trace_id || src?.traceId || "").trim(),
      preset: normalizeLayoutPreset(src?.preset),
      focusId: String(src?.focusId || "").trim(),
      layoutDirection: normalizeLayoutDirection(src?.layoutDirection || src?.dir || {}),
      layoutHints: normalizeLayoutWorkerHints(src?.layoutHints || src?.hints || {}),
      nodes: buildLayoutWorkerNodesPayload(src?.nodes),
      edges: buildLayoutWorkerEdgesPayload(src?.edges),
    };
  }

  function clonePrewarmLayoutRow(row, fields) {
    const src = row && typeof row === "object" ? row : {};
    const out = {};
    (Array.isArray(fields) ? fields : []).forEach((key) => {
      const value = src[key];
      if (value == null) return;
      out[key] = value;
    });
    return out;
  }

  function clonePrewarmLayoutInput(nodes, edges) {
    const clonedNodes = (Array.isArray(nodes) ? nodes : []).map((node) =>
      clonePrewarmLayoutRow(node, LAYOUT_PREWARM_NODE_FIELDS)
    );
    const clonedEdges = (Array.isArray(edges) ? edges : []).map((edge) =>
      clonePrewarmLayoutRow(edge, LAYOUT_PREWARM_EDGE_FIELDS)
    );
    return { clonedNodes, clonedEdges };
  }

  function buildLayoutWorkerResult(payload = {}) {
    const src = payload && typeof payload === "object" ? payload : {};
    const nodes = projectLayoutNodes(src?.nodes, src?.metaKeys);
    const summary = summarizeProjectedLayout(nodes);
    const result = {
      seq: Number(src?.seq) || 0,
      trace_id: String(src?.trace_id || src?.traceId || "").trim(),
      contractVersion: LAYOUT_CONTRACT_VERSION,
      preset: normalizeLayoutPreset(src?.preset),
      direction: normalizeLayoutDirection(src?.layoutDirection || src?.dir || {}),
      nodes,
      algoVersion: String(src?.algoVersion || ""),
      coreCount: summary.coreCount,
      coreIds: summary.coreIds,
      summary,
      layoutReport:
        src?.layoutReport && typeof src.layoutReport === "object"
          ? src.layoutReport
          : null,
    };
    const error = String(src?.error || "").trim();
    if (error) result.error = error;
    return result;
  }

  root.__ANALYTIX_LAYOUT_WORKER_CONTRACT__ = {
    LAYOUT_META_KEYS: [...LAYOUT_META_KEYS],
    LAYOUT_CONTRACT_VERSION,
    normalizeLayoutPreset,
    normalizeLayoutDirection,
    normalizeLayoutWorkerHints,
    buildLayoutWorkerNodesPayload,
    buildLayoutWorkerEdgesPayload,
    projectLayoutNode,
    projectLayoutNodes,
    summarizeProjectedLayout,
    buildLayoutWorkerRequest,
    clonePrewarmLayoutInput,
    buildLayoutWorkerResult,
  };
})();
