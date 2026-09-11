/* Hierarchy layout mode implementation */
/* eslint-disable no-restricted-globals */

(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : typeof window !== "undefined" ? window : null;
  const engine = root?.AnalytixLayoutEngine;
  if (!engine || typeof engine.registerLayoutModeRunner !== "function") return;

  function clamp(value, min, max) {
    if (typeof engine.clampLayout === "function") return engine.clampLayout(value, min, max);
    const num = Number(value);
    if (!Number.isFinite(num)) return min;
    return Math.max(min, Math.min(max, num));
  }

  function computeHierarchyCenterY(nodes) {
    let minY = Infinity;
    let maxY = -Infinity;
    (nodes || []).forEach((node) => {
      const y = Number(node?.y);
      if (!Number.isFinite(y)) return;
      if (y < minY) minY = y;
      if (y > maxY) maxY = y;
    });
    return Number.isFinite(minY) && Number.isFinite(maxY) ? (minY + maxY) / 2 : 0;
  }

  function applyHierarchyLayoutWidth(nodes, opts = {}) {
    if (!Array.isArray(nodes) || nodes.length < 2) return;
    const direction = String(opts?.direction || "vertical");
    const isVertical = direction === "vertical" || direction === "vertical-reverse";
    if (!isVertical) return;
    const requestedScale = Number(opts?.widthScale ?? 1.24);
    const widthScale = clamp(requestedScale, 1, 1.8);
    if (!Number.isFinite(widthScale) || widthScale <= 1.001) return;
    const center = typeof engine.computeLayoutCenter === "function" ? engine.computeLayoutCenter(nodes) : null;
    let cx = Number(center?.x);
    if (!Number.isFinite(cx)) {
      let minX = Infinity;
      let maxX = -Infinity;
      nodes.forEach((node) => {
        const x = Number(node?.x);
        if (!Number.isFinite(x)) return;
        if (x < minX) minX = x;
        if (x > maxX) maxX = x;
      });
      cx = Number.isFinite(minX) && Number.isFinite(maxX) ? (minX + maxX) / 2 : 0;
    }
    nodes.forEach((node) => {
      if (!node || typeof node !== "object") return;
      const x = Number(node.x);
      if (!Number.isFinite(x)) return;
      node.x = cx + (x - cx) * widthScale;
    });
  }

  function clearHierarchyLabelMeta(edge) {
    if (!edge || typeof edge !== "object") return;
    delete edge.edgeLabelAnchorMode;
    delete edge.edgeLabelOffsetY;
    delete edge.edgeLabelSideOffset;
    delete edge.edgeLabelMiddleRatio;
    delete edge.edgeLabelSideCapRatio;
    delete edge.edgeLabelPreferDegree;
  }

  function median(values) {
    const nums = (Array.isArray(values) ? values : [])
      .map((v) => Number(v))
      .filter((v) => Number.isFinite(v))
      .sort((a, b) => a - b);
    if (!nums.length) return NaN;
    const mid = Math.floor(nums.length / 2);
    if (nums.length % 2 === 1) return nums[mid];
    return (nums[mid - 1] + nums[mid]) / 2;
  }

  function applyHierarchyPolylineRoutingHints(nodes, edges, opts = {}) {
    if (!Array.isArray(edges) || !edges.length) return;
    const orthEnabled = opts?.orthEnabled !== false;
    const nodeMap = new Map((nodes || []).map((n) => [String(n?.id || ""), n]));
    const bendEps = Math.max(4, Number(opts?.bendEps) || 8);
    edges.forEach((edge) => {
      if (!edge || typeof edge !== "object") return;
      const route = String(edge.edgeRoute || edge.route || "").toLowerCase();
      const isOrth = route === "orthogonal";
      const isDouble = String(edge.mode || "single").toLowerCase() === "double";
      if (!orthEnabled || !isOrth || isDouble) {
        clearHierarchyLabelMeta(edge);
        return;
      }
      const axis = String(edge.orthAxis || "").toLowerCase() === "vertical" ? "vertical" : "horizontal";
      const s = nodeMap.get(String(edge.source || ""));
      const t = nodeMap.get(String(edge.target || ""));
      const sx = Number(s?.x) || 0;
      const sy = Number(s?.y) || 0;
      const tx = Number(t?.x) || 0;
      const ty = Number(t?.y) || 0;
      const hasTurn = axis === "vertical" ? Math.abs(sx - tx) > bendEps : Math.abs(sy - ty) > bendEps;
      edge.edgeLabelAnchorMode = "side";
      edge.edgeLabelOffsetY = 0;
      edge.edgeLabelSideOffset = axis === "vertical" ? (hasTurn ? 138 : 146) : hasTurn ? 110 : 118;
      edge.edgeLabelMiddleRatio = axis === "vertical" ? 0.5 : 0.47;
      edge.edgeLabelSideCapRatio = axis === "vertical" ? 0.56 : 0.52;
      edge.edgeLabelPreferDegree = true;
    });
  }

  function applyHierarchyEdgeRouting(nodes, edges, analysis = null, opts = {}) {
    if (!Array.isArray(edges) || !edges.length) return;
    const orthEnabled = opts?.orthEnabled !== false;
    const nodeMap = new Map((nodes || []).map((node) => [String(node?.id || ""), node]));
    const directional = analysis && typeof analysis === "object" ? analysis : {};
    const backEdgeSet = directional.backEdgeSet instanceof Set ? directional.backEdgeSet : new Set();
    const centerY = computeHierarchyCenterY(nodes);
    const upperBias = clamp(Number(opts?.upperBias) || 0.64, 0.5, 0.9);
    const lowerBias = clamp(Number(opts?.lowerBias) || 0.6, 0.5, 0.9);
    const sharedTopRatio = clamp(Number(opts?.sharedRowTopRatio) || 0.36, 0.2, 0.72);
    const sharedBottomRatio = clamp(Number(opts?.sharedRowBottomRatio) || 0.36, 0.2, 0.72);
    const sharedRowMinLeg = Math.max(6, Number(opts?.sharedRowMinLeg) || 14);
    const groupMap = new Map();
    const edgeCtxList = [];
    const pushGroupMember = (key, hubY, farY, edgeIndex) => {
      if (!Number.isFinite(hubY) || !Number.isFinite(farY) || !key) return;
      if (!groupMap.has(key)) {
        groupMap.set(key, {
          key,
          hubY,
          side: String(key).includes(":top") ? "top" : "bottom",
          members: [],
        });
      }
      const group = groupMap.get(key);
      if (!group) return;
      group.members.push({ edgeIndex, farY });
    };
    edges.forEach((edge, index) => {
      if (!edge || typeof edge !== "object") return;
      const dir =
        typeof engine.resolveDirectedEdge === "function"
          ? engine.resolveDirectedEdge(edge)
          : { directed: false, from: edge.source, to: edge.target };
      const from = dir?.directed ? dir.from : edge.source;
      const to = dir?.directed ? dir.to : edge.target;
      const backflow = backEdgeSet.has(index);
      edge.layoutBackflow = backflow;
      edge.__layoutBackflow = backflow;
      const orthEligible =
        orthEnabled &&
        !backflow &&
        dir?.directed &&
        edge.source !== edge.target &&
        String(edge.mode || "single").toLowerCase() !== "double";
      delete edge.orthSharedCoord;
      if (orthEligible) {
        const fromNode = nodeMap.get(String(from || "")) || nodeMap.get(String(edge.source || ""));
        const toNode = nodeMap.get(String(to || "")) || nodeMap.get(String(edge.target || ""));
        const fromY = Number(fromNode?.y);
        const toY = Number(toNode?.y);
        const hierarchyUpperEdge =
          Number.isFinite(fromY) &&
          Number.isFinite(toY) &&
          Math.max(fromY, toY) <= centerY + 8;
        edge.edgeRoute = "orthogonal";
        edge.orthAxis = "vertical";
        const defaultBias = hierarchyUpperEdge ? upperBias : lowerBias;
        edge.orthBias = defaultBias;
        const sourceNode = nodeMap.get(String(edge.source || ""));
        const targetNode = nodeMap.get(String(edge.target || ""));
        const sourceY = Number(sourceNode?.y);
        const targetY = Number(targetNode?.y);
        const inSide = sourceY <= targetY ? "top" : "bottom";
        const outSide = targetY >= sourceY ? "bottom" : "top";
        const inKey = `in:${String(edge.target || "")}:${inSide}`;
        const outKey = `out:${String(edge.source || "")}:${outSide}`;
        pushGroupMember(inKey, targetY, sourceY, index);
        pushGroupMember(outKey, sourceY, targetY, index);
        edgeCtxList.push({
          edge,
          index,
          defaultBias,
          sourceY,
          targetY,
          inKey,
          outKey,
        });
      } else {
        delete edge.edgeRoute;
        delete edge.orthAxis;
        delete edge.orthBias;
        delete edge.orthSharedCoord;
      }
    });
    const sharedRowByKey = new Map();
    groupMap.forEach((group, key) => {
      if (!group || !Array.isArray(group.members) || group.members.length < 2) return;
      const hubY = Number(group.hubY);
      if (!Number.isFinite(hubY)) return;
      const farYs = group.members.map((row) => Number(row?.farY)).filter((v) => Number.isFinite(v));
      if (farYs.length < 2) return;
      const medFarY = median(farYs);
      if (!Number.isFinite(medFarY)) return;
      const ratio = group.side === "top" ? sharedTopRatio : sharedBottomRatio;
      let preferred = hubY + (medFarY - hubY) * ratio;
      if (group.side === "top") {
        const lowerBound = Math.max(...farYs) + sharedRowMinLeg;
        const upperBound = hubY - sharedRowMinLeg;
        preferred = lowerBound <= upperBound ? clamp(preferred, lowerBound, upperBound) : (lowerBound + upperBound) / 2;
      } else {
        const lowerBound = hubY + sharedRowMinLeg;
        const upperBound = Math.min(...farYs) - sharedRowMinLeg;
        preferred = lowerBound <= upperBound ? clamp(preferred, lowerBound, upperBound) : (lowerBound + upperBound) / 2;
      }
      if (!Number.isFinite(preferred)) return;
      sharedRowByKey.set(key, preferred);
    });
    edgeCtxList.forEach((ctx) => {
      const edge = ctx?.edge;
      if (!edge) return;
      const inCount = groupMap.get(ctx.inKey)?.members?.length || 0;
      const outCount = groupMap.get(ctx.outKey)?.members?.length || 0;
      const hasInShared = sharedRowByKey.has(ctx.inKey);
      const hasOutShared = sharedRowByKey.has(ctx.outKey);
      let rowY = NaN;
      if ((inCount >= 2 || outCount >= 2) && (hasInShared || hasOutShared)) {
        if (inCount > outCount && hasInShared) rowY = Number(sharedRowByKey.get(ctx.inKey));
        else if (outCount > inCount && hasOutShared) rowY = Number(sharedRowByKey.get(ctx.outKey));
        else if (hasInShared) rowY = Number(sharedRowByKey.get(ctx.inKey));
        else if (hasOutShared) rowY = Number(sharedRowByKey.get(ctx.outKey));
      }
      if (!Number.isFinite(rowY)) {
        edge.orthBias = ctx.defaultBias;
        delete edge.orthSharedCoord;
        return;
      }
      const dy = Number(ctx.targetY) - Number(ctx.sourceY);
      if (!Number.isFinite(dy) || Math.abs(dy) <= 1e-6) {
        edge.orthBias = ctx.defaultBias;
        delete edge.orthSharedCoord;
        return;
      }
      const bias = clamp((rowY - Number(ctx.sourceY)) / dy, 0.05, 0.95);
      edge.orthBias = Number.isFinite(bias) ? bias : ctx.defaultBias;
      edge.orthSharedCoord = rowY;
    });
    applyHierarchyPolylineRoutingHints(nodes, edges, opts);
  }

  function layoutHierarchy(nodes, edges, focusId, opts = {}) {
    const direction = String(opts?.direction || "vertical");
    if (typeof engine.layoutFlow === "function") {
      engine.layoutFlow(nodes, edges, focusId, { direction });
      return nodes;
    }
    return nodes;
  }

  if (typeof engine.registerLayoutExports === "function") {
    engine.registerLayoutExports({
      layoutHierarchy,
      applyHierarchyLayoutWidth,
      applyHierarchyPolylineRoutingHints,
      applyHierarchyEdgeRouting,
    });
  } else {
    engine.layoutHierarchy = layoutHierarchy;
    engine.applyHierarchyLayoutWidth = applyHierarchyLayoutWidth;
    engine.applyHierarchyPolylineRoutingHints = applyHierarchyPolylineRoutingHints;
    engine.applyHierarchyEdgeRouting = applyHierarchyEdgeRouting;
  }

  engine.registerLayoutModeRunner("hierarchy", (ctx) => {
    const direction = ctx?.dir?.hierarchy === "up" ? "vertical-reverse" : "vertical";
    layoutHierarchy(ctx.nodes, ctx.edges, ctx.focusId, { direction });
    const hints = ctx?.hints || {};
    if (typeof engine.applyAdaptiveLayoutSpacing === "function") {
      engine.applyAdaptiveLayoutSpacing(ctx.nodes, ctx.edges, "hierarchy", ctx.focusId, {
        corePin: hints.corePin,
        corePlacement: hints.corePlacement,
        coreSource: hints.coreSource,
      });
    }
    applyHierarchyLayoutWidth(ctx.nodes, {
      direction,
      widthScale: Number(hints?.hierarchyWidthScale) || 1.24,
    });
    return true;
  });
})();
