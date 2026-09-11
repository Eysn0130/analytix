/* Flow layout mode implementation */
/* eslint-disable no-restricted-globals */

(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : typeof window !== "undefined" ? window : null;
  const engine = root?.AnalytixLayoutEngine;
  if (!engine || typeof engine.registerLayoutModeRunner !== "function") return;

  const {
    buildDegreeMap,
    buildLayoutAdjacency,
    collectLayoutComponents,
    resolveDirectedEdge,
    layoutScale,
    computeLayoutCenter,
    computeLayoutBoundsCenter,
    recenterLayout,
    recenterLayoutByBounds,
    stretchLayoutAroundCenter,
    computeAdaptiveLayoutStretch,
    relaxLayoutEdgeLength,
    relaxLayoutNodeDistance,
    clampLayout,
  } = engine;

function estimateFlowTextUnits(text) {
  const raw = String(text || "");
  if (!raw) return 0;
  let units = 0;
  for (const ch of raw) {
    const cp = ch.codePointAt(0) || 0;
    if (/\s/.test(ch)) units += 0.33;
    else if (cp > 255) units += 1.0;
    else if (/[A-Z0-9]/.test(ch)) units += 0.64;
    else units += 0.56;
  }
  return units;
}

function estimateFlowNodeHorizontalHalf(node) {
  const baseSize = clampLayout(Number(node?.fontSize) || 13, 8, 36);
  const subSize = Math.max(8, baseSize - 1);
  const name = String(node?.name || "").trim();
  const title = String(node?.title || node?.label || node?.id || "").trim();
  const displayId = String(node?.display_id || node?.displayId || "").trim();
  const mainText = name ? name || title : displayId || title;
  const subText = name ? displayId || title : "";
  const singleText = name ? "" : displayId || title;
  const mainWidth = Math.min(220, estimateFlowTextUnits(mainText) * Math.max(1, baseSize));
  const subWidth = Math.min(260, estimateFlowTextUnits(subText) * Math.max(1, subSize));
  const singleWidth = Math.min(248, estimateFlowTextUnits(singleText) * Math.max(1, baseSize));
  return Math.max(18, mainWidth / 2, subWidth / 2, singleWidth / 2);
}

function estimateFlowNodeVerticalExtents(node) {
  const r = Math.max(8, Number(node?.r) || 18);
  const baseSize = clampLayout(Number(node?.fontSize) || 13, 8, 36);
  const subSize = Math.max(8, baseSize - 1);
  const hasName = String(node?.name || "").trim().length > 0;
  const mainHalf = Math.max(4, baseSize * 0.56);
  const subHalf = Math.max(4, subSize * 0.56);
  const top = r;
  const bottom = hasName ? Math.max(r + 16 + mainHalf, r + 34 + subHalf) : Math.max(r + 20 + mainHalf, r);
  return { top, bottom };
}

function computeFlowCrossAxisSafeGap(nodes) {
  let maxTop = 18;
  let maxBottom = 58;
  (nodes || []).forEach((node) => {
    const ext = estimateFlowNodeVerticalExtents(node);
    maxTop = Math.max(maxTop, Number(ext?.top) || 0);
    maxBottom = Math.max(maxBottom, Number(ext?.bottom) || 0);
  });
  const need = maxTop + maxBottom + 8;
  return clampLayout(need, 82, 172);
}

function computeFlowMainAxisSafeGap(nodes, isVertical = false) {
  if (isVertical) {
    let maxTop = 18;
    let maxBottom = 58;
    (nodes || []).forEach((node) => {
      const ext = estimateFlowNodeVerticalExtents(node);
      maxTop = Math.max(maxTop, Number(ext?.top) || 0);
      maxBottom = Math.max(maxBottom, Number(ext?.bottom) || 0);
    });
    const need = maxTop + maxBottom + 52;
    return clampLayout(need, 126, 252);
  }
  let maxHalfWidth = 56;
  let maxRadius = 18;
  (nodes || []).forEach((node) => {
    maxHalfWidth = Math.max(maxHalfWidth, estimateFlowNodeHorizontalHalf(node));
    maxRadius = Math.max(maxRadius, Math.max(8, Number(node?.r) || 18));
  });
  const need = maxHalfWidth * 2 + maxRadius * 0.9 + 28;
  return clampLayout(need, 156, 436);
}

function applyAdaptiveLayoutSpacing(nodes, edges, mode, focusId, layoutOpts = {}) {
  const layoutMode = String(mode || "").toLowerCase();
  if (layoutMode !== "flow" && layoutMode !== "hierarchy") return;
  if (!Array.isArray(nodes) || nodes.length < 2) return;
  const edgeRows = Array.isArray(edges) ? edges : [];
  const center = computeLayoutCenter(nodes) || computeLayoutBoundsCenter(nodes);
  let factor = computeAdaptiveLayoutStretch(nodes.length, edgeRows.length, layoutMode);
  if (!Number.isFinite(factor) || factor < 1) factor = 1;
  if (factor > 1.001) {
    stretchLayoutAroundCenter(nodes, factor, center);
  }
  const isHierarchy = layoutMode === "hierarchy";
  const crossGapFloor = computeFlowCrossAxisSafeGap(nodes);
  const mainGapFloor = computeFlowMainAxisSafeGap(nodes, isHierarchy);
  let nodeMin = 64 * Math.min(1.88, factor * 1.05);
  let edgeMin = 134 * Math.min(1.95, factor * 1.1);
  if (isHierarchy) {
    nodeMin = Math.max(nodeMin, crossGapFloor, mainGapFloor * 0.76);
    edgeMin = Math.max(edgeMin, crossGapFloor + 42, mainGapFloor * 0.88);
  } else {
    nodeMin = Math.max(nodeMin, crossGapFloor, mainGapFloor * 0.84);
    edgeMin = Math.max(edgeMin, crossGapFloor + 42, mainGapFloor);
  }
  if (nodes.length <= 6) {
    nodeMin = Math.max(crossGapFloor, nodeMin * (isHierarchy ? 0.92 : 0.96));
    edgeMin = Math.max(crossGapFloor + 28, edgeMin * (isHierarchy ? 0.88 : 0.92));
  }
  const n = nodes.length;
  const edgePass = n > 1800 ? 1 : n > 700 ? 2 : 3;
  const nodePass = n > 1800 ? 2 : n > 700 ? 3 : 4;
  const fixedIds = new Set();
  if (focusId) fixedIds.add(String(focusId));
  if (
    layoutMode === "hierarchy" &&
    String(layoutOpts?.corePin || "").toLowerCase() === "hard"
  ) {
    nodes.forEach((node) => {
      if (!node?.id) return;
      if (String(node?.layoutBand || "") === "core") fixedIds.add(String(node.id));
    });
  }
  relaxLayoutEdgeLength(nodes, edgeRows, edgeMin, edgePass, fixedIds);
  relaxLayoutNodeDistance(nodes, nodeMin, nodePass, fixedIds);
  relaxLayoutNodeDistance(nodes, Math.max(crossGapFloor, nodeMin * 0.92), 1, fixedIds);
  recenterLayout(nodes, center);
}

function computeDirectionalLevels(compIds, compDirected, degMap = null, opts = {}) {
  const ids = Array.from(compIds);
  const deg = degMap || new Map(ids.map((id) => [id, 0]));
  const inEdges = new Map(ids.map((id) => [id, []]));
  const outEdges = new Map(ids.map((id) => [id, []]));
  const indeg = new Map(ids.map((id) => [id, 0]));
  compDirected.forEach((e) => {
    if (!inEdges.has(e.to) || !outEdges.has(e.from)) return;
    inEdges.get(e.to).push(e);
    outEdges.get(e.from).push(e);
    indeg.set(e.to, (indeg.get(e.to) || 0) + 1);
  });

  const backEdgeSet = new Set();
  const remain = new Set(ids);
  const dynIndeg = new Map(indeg);
  const topo = [];
  const idCmp = (a, b) =>
    (deg.get(b) || 0) - (deg.get(a) || 0) || String(a).localeCompare(String(b), "zh-CN");

  while (remain.size) {
    let ready = Array.from(remain).filter((id) => (dynIndeg.get(id) || 0) === 0);
    if (!ready.length) {
      const fallback = Array.from(remain).sort(
        (a, b) =>
          (dynIndeg.get(a) || 0) - (dynIndeg.get(b) || 0) ||
          (outEdges.get(b)?.length || 0) - (outEdges.get(a)?.length || 0) ||
          String(a).localeCompare(String(b), "zh-CN")
      )[0];
      const incoming = inEdges.get(fallback) || [];
      incoming.forEach((edge) => {
        if (!remain.has(edge.from)) return;
        if (backEdgeSet.has(edge.index)) return;
        backEdgeSet.add(edge.index);
        dynIndeg.set(fallback, Math.max(0, (dynIndeg.get(fallback) || 0) - 1));
      });
      if ((dynIndeg.get(fallback) || 0) > 0) dynIndeg.set(fallback, 0);
      ready = [fallback];
    }
    ready.sort(idCmp);
    ready.forEach((id) => {
      if (!remain.has(id)) return;
      remain.delete(id);
      topo.push(id);
      (outEdges.get(id) || []).forEach((edge) => {
        if (backEdgeSet.has(edge.index)) return;
        if (!remain.has(edge.to)) return;
        dynIndeg.set(edge.to, Math.max(0, (dynIndeg.get(edge.to) || 0) - 1));
      });
    });
  }

  const level = new Map(ids.map((id) => [id, 0]));
  topo.forEach((id) => {
    let best = 0;
    (inEdges.get(id) || []).forEach((edge) => {
      if (backEdgeSet.has(edge.index)) return;
      best = Math.max(best, (level.get(edge.from) || 0) + 1);
    });
    level.set(id, best);
  });

  let maxLevel = 0;
  const buckets = new Map();
  ids.forEach((id) => {
    const lv = level.get(id) || 0;
    maxLevel = Math.max(maxLevel, lv);
    if (!buckets.has(lv)) buckets.set(lv, []);
    buckets.get(lv).push(id);
  });
  const dagOut = new Map(ids.map((id) => [id, 0]));
  const dagIn = new Map(ids.map((id) => [id, 0]));
  compDirected.forEach((edge) => {
    if (backEdgeSet.has(edge.index)) return;
    dagOut.set(edge.from, (dagOut.get(edge.from) || 0) + 1);
    dagIn.set(edge.to, (dagIn.get(edge.to) || 0) + 1);
  });

  if (!opts.skipOrder) {
    const orderPos = new Map();
    const setOrderPos = () => {
      buckets.forEach((list) => {
        list.forEach((id, idx) => orderPos.set(id, idx));
      });
    };
    buckets.forEach((list) => {
      list.sort(
        (a, b) =>
          (deg.get(b) || 0) - (deg.get(a) || 0) || String(a).localeCompare(String(b), "zh-CN")
      );
    });
    setOrderPos();
    const pred = new Map(ids.map((id) => [id, []]));
    const succ = new Map(ids.map((id) => [id, []]));
    compDirected.forEach((edge) => {
      if (backEdgeSet.has(edge.index)) return;
      pred.get(edge.to)?.push(edge.from);
      succ.get(edge.from)?.push(edge.to);
    });
    const bary = (arr) => {
      if (!arr || !arr.length) return null;
      let sum = 0;
      let count = 0;
      arr.forEach((id) => {
        const p = orderPos.get(id);
        if (!Number.isFinite(p)) return;
        sum += p;
        count += 1;
      });
      return count ? sum / count : null;
    };
    for (let pass = 0; pass < 3; pass += 1) {
      for (let lv = 1; lv <= maxLevel; lv += 1) {
        const list = buckets.get(lv);
        if (!list || list.length <= 2) continue;
        list.sort((a, b) => {
          const ba = bary(pred.get(a));
          const bb = bary(pred.get(b));
          if (ba != null && bb != null && ba !== bb) return ba - bb;
          if (ba != null && bb == null) return -1;
          if (bb != null && ba == null) return 1;
          return (
            (deg.get(b) || 0) - (deg.get(a) || 0) || String(a).localeCompare(String(b), "zh-CN")
          );
        });
      }
      setOrderPos();
      for (let lv = maxLevel - 1; lv >= 0; lv -= 1) {
        const list = buckets.get(lv);
        if (!list || list.length <= 2) continue;
        list.sort((a, b) => {
          const ba = bary(succ.get(a));
          const bb = bary(succ.get(b));
          if (ba != null && bb != null && ba !== bb) return ba - bb;
          if (ba != null && bb == null) return -1;
          if (bb != null && ba == null) return 1;
          return (
            (deg.get(b) || 0) - (deg.get(a) || 0) || String(a).localeCompare(String(b), "zh-CN")
          );
        });
      }
      setOrderPos();
    }
  }

  return { level, buckets, maxLevel, backEdgeSet, dagOut, dagIn };
}

function analyzeDirectionalStructure(nodes, edges) {
  const nodeIds = new Set((nodes || []).map((n) => n.id));
  const { adjacency } = buildLayoutAdjacency(nodeIds, edges);
  const components = collectLayoutComponents(nodeIds, adjacency);
  const directed = [];
  (edges || []).forEach((edge, index) => {
    const dir = resolveDirectedEdge(edge);
    if (!dir.directed) return;
    if (!nodeIds.has(dir.from) || !nodeIds.has(dir.to)) return;
    directed.push({ index, from: dir.from, to: dir.to });
  });
  const compOf = new Map();
  components.forEach((comp, idx) => comp.forEach((id) => compOf.set(id, idx)));
  const compDirected = components.map(() => []);
  directed.forEach((edge) => {
    const ci = compOf.get(edge.from);
    const cj = compOf.get(edge.to);
    if (ci == null || ci !== cj) return;
    compDirected[ci].push(edge);
  });
  const deg = buildDegreeMap(edges);
  const backEdgeSet = new Set();
  const outDegree = new Map(Array.from(nodeIds).map((id) => [id, 0]));
  const inDegree = new Map(Array.from(nodeIds).map((id) => [id, 0]));
  components.forEach((comp, idx) => {
    const info = computeDirectionalLevels(comp, compDirected[idx] || [], deg, { skipOrder: true });
    info.backEdgeSet.forEach((edgeIndex) => backEdgeSet.add(edgeIndex));
    info.dagOut.forEach((v, id) => outDegree.set(id, (outDegree.get(id) || 0) + v));
    info.dagIn.forEach((v, id) => inDegree.set(id, (inDegree.get(id) || 0) + v));
  });
  return { backEdgeSet, outDegree, inDegree };
}

function median(values) {
  const rows = (Array.isArray(values) ? values : [])
    .map((v) => Number(v))
    .filter((v) => Number.isFinite(v))
    .sort((a, b) => a - b);
  if (!rows.length) return NaN;
  const mid = Math.floor(rows.length / 2);
  if (rows.length % 2 === 1) return rows[mid];
  return (rows[mid - 1] + rows[mid]) / 2;
}

function applyFlowPolylineRoutingHints(nodes, edges, mode, opts = {}) {
  if (!Array.isArray(edges) || !edges.length) return;
  const layoutMode = String(mode || "").toLowerCase();
  const flowMode = layoutMode === "flow";
  const orthEnabled = opts?.orthEnabled !== false;
  const nodeMap = new Map((nodes || []).map((n) => [String(n?.id || ""), n]));
  const bendEps = Math.max(4, Number(opts?.bendEps) || 8);
  edges.forEach((edge) => {
    if (!edge || typeof edge !== "object") return;
    const route = String(edge.edgeRoute || edge.route || "").toLowerCase();
    const isOrth = route === "orthogonal";
    const isDouble = String(edge.mode || "single").toLowerCase() === "double";
    const axis = String(edge.orthAxis || "").toLowerCase() === "vertical" ? "vertical" : "horizontal";
    const s = nodeMap.get(String(edge.source || ""));
    const t = nodeMap.get(String(edge.target || ""));
    const sx = Number(s?.x) || 0;
    const sy = Number(s?.y) || 0;
    const tx = Number(t?.x) || 0;
    const ty = Number(t?.y) || 0;
    const hasTurn = axis === "vertical" ? Math.abs(sx - tx) > bendEps : Math.abs(sy - ty) > bendEps;
    if (flowMode && orthEnabled && isOrth && !isDouble) {
      // Flow routing policy:
      // - turned orthogonal edges: keep side-aligned labels
      // - near-straight orthogonal edges: keep center-snap labels
      edge.edgeLabelAnchorMode = hasTurn ? "side" : "center-snap";
      edge.edgeLabelOffsetY = 0;
      return;
    }
    if ("edgeLabelAnchorMode" in edge) delete edge.edgeLabelAnchorMode;
    if ("edgeLabelOffsetY" in edge) delete edge.edgeLabelOffsetY;
    if ("edgeLabelSideOffset" in edge) delete edge.edgeLabelSideOffset;
    if ("edgeLabelMiddleRatio" in edge) delete edge.edgeLabelMiddleRatio;
    if ("edgeLabelSideCapRatio" in edge) delete edge.edgeLabelSideCapRatio;
    if ("edgeLabelPreferDegree" in edge) delete edge.edgeLabelPreferDegree;
  });
}

function applyFlowEdgeRouting(nodes, edges, analysis = null, opts = {}) {
  if (!Array.isArray(edges) || !edges.length) return;
  const orthEnabled = opts?.orthEnabled !== false;
  const nodeMap = new Map((nodes || []).map((node) => [String(node?.id || ""), node]));
  const directional = analysis && typeof analysis === "object" ? analysis : {};
  const backEdgeSet = directional.backEdgeSet instanceof Set ? directional.backEdgeSet : new Set();
  const outDegree = directional.outDegree instanceof Map ? directional.outDegree : new Map();
  const inDegree = directional.inDegree instanceof Map ? directional.inDegree : new Map();
  const deg = buildDegreeMap(edges);
  const defaultBias = clampLayout(Number(opts?.flowBias) || 0.54, 0.18, 0.82);
  const sharedLeftRatio = clampLayout(Number(opts?.sharedLeftRatio) || 0.36, 0.24, 0.78);
  const sharedRightRatio = clampLayout(Number(opts?.sharedRightRatio) || 0.36, 0.24, 0.78);
  const sharedColMinLeg = Math.max(6, Number(opts?.sharedColMinLeg) || 14);
  const groupMap = new Map();
  const edgeCtxList = [];
  const pushGroupMember = (key, hubX, farX, edgeIndex) => {
    if (!key) return;
    if (!Number.isFinite(hubX) || !Number.isFinite(farX)) return;
    if (!groupMap.has(key)) {
      groupMap.set(key, {
        key,
        hubX,
        side: String(key).includes(":left") ? "left" : "right",
        members: [],
      });
    }
    const group = groupMap.get(key);
    if (!group) return;
    group.members.push({ edgeIndex, farX });
  };
  edges.forEach((edge, index) => {
    if (!edge || typeof edge !== "object") return;
    const dir = resolveDirectedEdge(edge);
    const from = dir.directed ? dir.from : edge.source;
    const to = dir.directed ? dir.to : edge.target;
    const backflow = backEdgeSet.has(index);
    edge.layoutBackflow = backflow;
    edge.__layoutBackflow = backflow;
    delete edge.orthSharedCoord;
    const coreFan =
      (outDegree.get(from) || 0) >= 2 ||
      (inDegree.get(to) || 0) >= 2 ||
      (deg.get(from) || 0) >= 5 ||
      (deg.get(to) || 0) >= 5;
    const orthEligible =
      orthEnabled &&
      !backflow &&
      dir.directed &&
      edge.source !== edge.target &&
      String(edge.mode || "single").toLowerCase() !== "double" &&
      coreFan;
    if (!orthEligible) {
      delete edge.edgeRoute;
      delete edge.orthAxis;
      delete edge.orthBias;
      return;
    }
    edge.edgeRoute = "orthogonal";
    edge.orthAxis = "horizontal";
    edge.orthBias = defaultBias;
    const sourceNode = nodeMap.get(String(edge.source || ""));
    const targetNode = nodeMap.get(String(edge.target || ""));
    const sourceX = Number(sourceNode?.x);
    const targetX = Number(targetNode?.x);
    const inSide = sourceX <= targetX ? "left" : "right";
    const outSide = targetX >= sourceX ? "right" : "left";
    const inKey = `in:${String(to || "")}:${inSide}`;
    const outKey = `out:${String(from || "")}:${outSide}`;
    pushGroupMember(inKey, targetX, sourceX, index);
    pushGroupMember(outKey, sourceX, targetX, index);
    edgeCtxList.push({
      edge,
      defaultBias,
      sourceX,
      targetX,
      inKey,
      outKey,
    });
  });
  const sharedColByKey = new Map();
  groupMap.forEach((group, key) => {
    if (!group || !Array.isArray(group.members) || group.members.length < 2) return;
    const hubX = Number(group.hubX);
    if (!Number.isFinite(hubX)) return;
    const farXs = group.members.map((row) => Number(row?.farX)).filter((v) => Number.isFinite(v));
    if (farXs.length < 2) return;
    const medFarX = median(farXs);
    if (!Number.isFinite(medFarX)) return;
    const ratio = group.side === "left" ? sharedLeftRatio : sharedRightRatio;
    let preferred = hubX + (medFarX - hubX) * ratio;
    if (group.side === "left") {
      const lowerBound = Math.max(...farXs) + sharedColMinLeg;
      const upperBound = hubX - sharedColMinLeg;
      preferred = lowerBound <= upperBound ? clampLayout(preferred, lowerBound, upperBound) : (lowerBound + upperBound) / 2;
    } else {
      const lowerBound = hubX + sharedColMinLeg;
      const upperBound = Math.min(...farXs) - sharedColMinLeg;
      preferred = lowerBound <= upperBound ? clampLayout(preferred, lowerBound, upperBound) : (lowerBound + upperBound) / 2;
    }
    if (!Number.isFinite(preferred)) return;
    sharedColByKey.set(key, preferred);
  });
  edgeCtxList.forEach((ctx) => {
    const edge = ctx?.edge;
    if (!edge) return;
    const inCount = groupMap.get(ctx.inKey)?.members?.length || 0;
    const outCount = groupMap.get(ctx.outKey)?.members?.length || 0;
    const hasInShared = sharedColByKey.has(ctx.inKey);
    const hasOutShared = sharedColByKey.has(ctx.outKey);
    let columnX = NaN;
    if ((inCount >= 2 || outCount >= 2) && (hasInShared || hasOutShared)) {
      if (inCount > outCount && hasInShared) columnX = Number(sharedColByKey.get(ctx.inKey));
      else if (outCount > inCount && hasOutShared) columnX = Number(sharedColByKey.get(ctx.outKey));
      else if (hasInShared) columnX = Number(sharedColByKey.get(ctx.inKey));
      else if (hasOutShared) columnX = Number(sharedColByKey.get(ctx.outKey));
    }
    if (!Number.isFinite(columnX)) {
      edge.orthBias = ctx.defaultBias;
      delete edge.orthSharedCoord;
      return;
    }
    const dx = Number(ctx.targetX) - Number(ctx.sourceX);
    if (!Number.isFinite(dx) || Math.abs(dx) <= 1e-6) {
      edge.orthBias = ctx.defaultBias;
      delete edge.orthSharedCoord;
      return;
    }
    const bias = clampLayout((columnX - Number(ctx.sourceX)) / dx, 0.05, 0.95);
    edge.orthBias = Number.isFinite(bias) ? bias : ctx.defaultBias;
    edge.orthSharedCoord = columnX;
  });
  applyFlowPolylineRoutingHints(nodes, edges, "flow", { orthEnabled, bendEps: opts?.bendEps });
}

function layoutFlow(nodes, edges, focusId, opts = {}) {
  if (!nodes.length) return nodes;
  const deg = buildDegreeMap(edges);
  const nodeMap = new Map(nodes.map((n) => [n.id, n]));
  const nodeIds = new Set(nodes.map((n) => n.id));
  const { adjacency } = buildLayoutAdjacency(nodeIds, edges);
  const direction = opts.direction || "horizontal";
  const isVertical = direction === "vertical" || direction === "vertical-reverse";
  const isReverse = direction === "vertical-reverse" || direction === "horizontal-reverse";
  const scale = layoutScale(nodes.length);
  const safeCrossGap = isVertical ? 0 : computeFlowCrossAxisSafeGap(nodes);
  const mainGapFloor = computeFlowMainAxisSafeGap(nodes, isVertical);
  const mainGapBase = (isVertical ? 188 : 236) * scale * (nodes.length > 140 ? 1.05 : 1);
  const smallGraph = nodes.length <= 6;
  const mainGap = smallGraph
    ? clampLayout(
        mainGapBase * (isVertical ? 0.58 : 0.64),
        mainGapFloor,
        mainGapFloor + (isVertical ? 80 : 124)
      )
    : Math.max(mainGapBase, mainGapFloor);
  const rawCrossGap = (isVertical ? 122 : 104) * scale * (nodes.length > 140 ? 0.94 : 1.02);
  const crossGap = isVertical ? rawCrossGap : Math.max(rawCrossGap, safeCrossGap);

  const directed = [];
  (edges || []).forEach((edge, index) => {
    const dir = resolveDirectedEdge(edge);
    if (!dir.directed) return;
    if (!nodeIds.has(dir.from) || !nodeIds.has(dir.to)) return;
    directed.push({ index, from: dir.from, to: dir.to });
  });

  const components = collectLayoutComponents(nodeIds, adjacency);
  const compOf = new Map();
  components.forEach((comp, idx) => comp.forEach((id) => compOf.set(id, idx)));
  const compDirected = components.map(() => []);
  directed.forEach((edge) => {
    const ci = compOf.get(edge.from);
    const cj = compOf.get(edge.to);
    if (ci == null || ci !== cj) return;
    compDirected[ci].push(edge);
  });

  const compInfos = components.map((comp, idx) => {
    const info = computeDirectionalLevels(comp, compDirected[idx] || [], deg);
    const compNodes = Array.from(comp).map((id) => nodeMap.get(id)).filter(Boolean);
    const maxLevel = Math.max(0, info.maxLevel || 0);
    const levelCenter = maxLevel / 2;
    let maxBucket = 1;
    for (let lv = 0; lv <= maxLevel; lv += 1) {
      const list = info.buckets.get(lv) || [];
      if (!list.length) continue;
      maxBucket = Math.max(maxBucket, list.length);
      list.forEach((id, order) => {
        const node = nodeMap.get(id);
        if (!node) return;
        let main = (lv - levelCenter) * mainGap;
        if (isReverse) main = -main;
        const cross = (order - (list.length - 1) / 2) * crossGap;
        if (isVertical) {
          node.x = cross;
          node.y = main;
        } else {
          node.x = main;
          node.y = cross;
        }
        node.layoutBand = isVertical ? "hierarchy-level" : "flow-column";
        node.layoutLevel = lv;
      });
    }
    const crossSize = maxBucket * crossGap;
    const weight = compNodes.length * 12 + maxLevel * 9 + crossSize;
    return { ids: comp, nodes: compNodes, crossSize, weight, maxBucket };
  });

  const focusIdx = focusId ? compInfos.findIndex((c) => c.ids.has(focusId)) : -1;
  if (focusIdx > 0) {
    const [focusComp] = compInfos.splice(focusIdx, 1);
    compInfos.unshift(focusComp);
  }
  const ordered =
    compInfos.length <= 1
      ? compInfos
      : [compInfos[0], ...compInfos.slice(1).sort((a, b) => b.weight - a.weight)];
  const compGap = crossGap * 2.25;
  let posExtent = ordered.length ? ordered[0].crossSize / 2 : 0;
  let negExtent = posExtent;
  ordered.forEach((comp, idx) => {
    let offset = 0;
    if (idx > 0) {
      const half = comp.crossSize / 2;
      if (posExtent <= negExtent) {
        offset = posExtent + compGap + half;
        posExtent = offset + half;
      } else {
        offset = -(negExtent + compGap + half);
        negExtent = -offset + half;
      }
    }
    comp.nodes.forEach((node) => {
      if (!Number.isFinite(node.x) || !Number.isFinite(node.y)) return;
      if (isVertical) node.x += offset;
      else node.y += offset;
    });
  });

  let minCross = Infinity;
  let maxCross = -Infinity;
  nodes.forEach((node) => {
    const cross = Number(isVertical ? node?.x : node?.y);
    if (!Number.isFinite(cross)) return;
    minCross = Math.min(minCross, cross);
    maxCross = Math.max(maxCross, cross);
  });
  const crossSpan = maxCross - minCross;
  const crossCapBase = isVertical ? 2400 : 2600;
  const crossCap = crossCapBase * Math.max(1, Math.min(1.8, Math.log(nodes.length + 1) / 2.2));
  const maxBucket = ordered.reduce((acc, comp) => Math.max(acc, Number(comp?.maxBucket) || 1), 1);
  const safeSpanFloor = !isVertical ? Math.max(0, (maxBucket - 1) * safeCrossGap) : 0;
  const effectiveCrossCap = !isVertical ? Math.max(crossCap, safeSpanFloor) : crossCap;
  if (Number.isFinite(crossSpan) && crossSpan > effectiveCrossCap && crossSpan > 1e-3) {
    const k = effectiveCrossCap / crossSpan;
    const c = (maxCross + minCross) / 2;
    nodes.forEach((node) => {
      if (isVertical) {
        if (!Number.isFinite(node?.x)) return;
        node.x = c + (node.x - c) * k;
      } else {
        if (!Number.isFinite(node?.y)) return;
        node.y = c + (node.y - c) * k;
      }
    });
  }

  return nodes;
}

  const exportsMap = {
    computeDirectionalLevels,
    analyzeDirectionalStructure,
    layoutFlow,
    applyFlowEdgeRouting,
    applyFlowPolylineRoutingHints,
    applyAdaptiveLayoutSpacing,
  };
  if (typeof engine.registerLayoutExports === "function") {
    engine.registerLayoutExports(exportsMap);
  } else {
    Object.entries(exportsMap).forEach(([key, fn]) => {
      if (!key || typeof fn !== "function") return;
      engine[key] = fn;
    });
  }

  engine.registerLayoutModeRunner("flow", (ctx) => {
    const direction = ctx?.dir?.flow === "left" ? "horizontal-reverse" : "horizontal";
    layoutFlow(ctx.nodes, ctx.edges, ctx.focusId, { direction });
    const hints = ctx?.hints || {};
    if (typeof engine.applyAdaptiveLayoutSpacing === "function") {
      engine.applyAdaptiveLayoutSpacing(ctx.nodes, ctx.edges, "flow", ctx.focusId, {
        corePin: hints.corePin,
        corePlacement: hints.corePlacement,
        coreSource: hints.coreSource,
      });
    }
    return true;
  });
})();
