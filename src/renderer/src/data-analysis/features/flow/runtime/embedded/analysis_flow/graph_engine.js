/* global window */
(() => {
  const HIGHLIGHT_COLOR = "#ff8a00";
  const EPS = 1e-6;
  const GPU_TEXT_PREFETCH_MARGIN_PX = 260;
  const graphEnginePrimitives = window.__ANALYTIX_GRAPH_ENGINE_PRIMITIVES__ || null;
  const graphRendererBootstrap = window.__ANALYTIX_GRAPH_RENDERER_BOOTSTRAP__ || null;
  const graphRendererOrchestration = window.__ANALYTIX_GRAPH_RENDERER_ORCHESTRATION__ || null;
  const graphRendererInit = window.__ANALYTIX_GRAPH_RENDERER_INIT__ || null;
  const graphAttributeBinding = window.__ANALYTIX_GRAPH_ATTRIBUTE_BINDING__ || null;
  const graphLabelPipeline = window.__ANALYTIX_GRAPH_LABEL_PIPELINE__ || null;
  const piiProjection = window.__ANALYTIX_ORDINARY_PII_PROJECTION__ || null;
  const graphTextRenderPass = window.__ANALYTIX_GRAPH_TEXT_RENDER_PASS__ || null;
  const graphEdgeWorkerBootstrap = window.__ANALYTIX_GRAPH_EDGE_WORKER_BOOTSTRAP__ || null;
  const graphEdgeRouting = window.__ANALYTIX_GRAPH_EDGE_ROUTING__ || null;
  const graphTextAtlas = window.__ANALYTIX_GRAPH_TEXT_ATLAS__ || null;
  const graphMinimapCore = window.__ANALYTIX_GRAPH_MINIMAP_CORE__ || null;
  if (!graphEnginePrimitives) {
    throw new Error("graph engine primitives missing");
  }
  if (!graphRendererBootstrap) {
    throw new Error("graph renderer bootstrap missing");
  }
  if (!graphRendererOrchestration) {
    throw new Error("graph renderer orchestration missing");
  }
  if (!graphRendererInit) {
    throw new Error("graph renderer init missing");
  }
  if (!graphAttributeBinding) {
    throw new Error("graph attribute binding missing");
  }
  if (!graphLabelPipeline) {
    throw new Error("graph label pipeline missing");
  }
  if (!piiProjection || typeof piiProjection.projectField !== "function" || typeof piiProjection.projectDetected !== "function") {
    throw new Error("ordinary PII projection missing for graph engine");
  }
  if (!graphTextRenderPass) {
    throw new Error("graph text render pass missing");
  }
  if (!graphEdgeWorkerBootstrap) {
    throw new Error("graph edge worker bootstrap missing");
  }
  if (!graphEdgeRouting) {
    throw new Error("graph edge routing missing");
  }
  if (!graphTextAtlas) {
    throw new Error("graph text atlas missing");
  }
  if (!graphMinimapCore) {
    throw new Error("graph minimap core missing");
  }
  const {
    clamp,
    normalizeTextQualityMode,
    asBool,
    parseColor,
    toOpaqueColor,
    mixColor,
    resolveDash,
    normalizeVector,
    computeEdgeEndpoints,
    computeArrowLineTrim,
    computeLabelSpan,
    estimateTextWidth,
    estimateTextBox,
    truncateTextByWorldWidth,
  } = graphEnginePrimitives;
  const { WEBGL_CLEAR_FALLBACK, createGraphCanvas, createGraphGlContext } = graphRendererBootstrap;
  const { compileGraphShader, createGraphProgram, drawGraphInstanced, drawGraphScene } = graphRendererOrchestration;
  const { initGraphRendererGl } = graphRendererInit;
  const {
    bindEdgeAttributes,
    bindNodeAttributes,
    bindArrowAttributes,
    bindTextAttributes,
    bindPickNodeAttributes,
    bindPickEdgeAttributes,
  } = graphAttributeBinding;
  const { resolveNodeLabelRows, resolveEdgeLabelRows, buildNodeLabelAvoidRects } = graphLabelPipeline;
  const { drawGraphTextBatches } = graphTextRenderPass;
  const { createEdgeWorker } = graphEdgeWorkerBootstrap;
  const {
    buildGapSegments,
    buildOrthogonalSegments,
    trimPolylineSegments,
    segmentLength,
    segmentAxis,
    pickSegmentIndex,
    collectAxisSegmentIndices,
    pointDistSq,
    resolveEdgeArrowMode,
    resolveFlowOriginSide,
    resolveLabelSide,
    resolveOrthogonalLabelSegmentIndex,
    resolveOrthogonalLabelPoint,
    computeOrthogonalEndpoints,
    buildOrthogonalRouteData,
    applyGapToSegments,
    sealPolylineJoints,
    resolveOrthogonalLabelAnchorOpts,
    resolveEdgeLabelYOffset,
  } = graphEdgeRouting;
  const {
    DEFAULT_FONT_FAMILY,
    TEXT_BASE_SIZE,
    TEXT_ATLAS_PADDING,
    TEXT_ATLAS_SIZE,
    TEXT_ATLAS_SCALE,
    TEXT_ATLAS_UV_INSET,
    TEXT_ATLAS_SET_CACHE_LIMIT,
    TEXT_ATLAS_CACHE_LIMIT_BYTES,
    TEXT_ATLAS_IDLE_EVICT_MS,
    TextAtlas,
    TextAtlasSet,
  } = graphTextAtlas;
  const { Minimap } = graphMinimapCore;

  function splitAxisAlignedSegmentByRect(seg, rect) {
    if (!seg || !rect) return seg ? [seg] : [];
    const sx = Number(seg.sx) || 0;
    const sy = Number(seg.sy) || 0;
    const tx = Number(seg.tx) || 0;
    const ty = Number(seg.ty) || 0;
    const minX = Number(rect.minX);
    const maxX = Number(rect.maxX);
    const minY = Number(rect.minY);
    const maxY = Number(rect.maxY);
    if (![minX, maxX, minY, maxY].every(Number.isFinite)) return [seg];
    const dx = tx - sx;
    const dy = ty - sy;
    if (Math.abs(dx) <= EPS && Math.abs(dy) <= EPS) return [seg];
    if (Math.abs(dx) <= EPS) {
      if (sx < minX - EPS || sx > maxX + EPS) return [seg];
      const lo = Math.min(sy, ty);
      const hi = Math.max(sy, ty);
      const cutLo = Math.max(lo, minY);
      const cutHi = Math.min(hi, maxY);
      if (cutHi <= cutLo + EPS) return [seg];
      const asc = ty >= sy;
      const pieces = [];
      if (cutLo > lo + EPS) pieces.push(asc ? { sx, sy: lo, tx, ty: cutLo } : { sx, sy: cutLo, tx, ty: lo });
      if (cutHi < hi - EPS) pieces.push(asc ? { sx, sy: cutHi, tx, ty: hi } : { sx, sy: hi, tx, ty: cutHi });
      return pieces;
    }
    if (Math.abs(dy) <= EPS) {
      if (sy < minY - EPS || sy > maxY + EPS) return [seg];
      const lo = Math.min(sx, tx);
      const hi = Math.max(sx, tx);
      const cutLo = Math.max(lo, minX);
      const cutHi = Math.min(hi, maxX);
      if (cutHi <= cutLo + EPS) return [seg];
      const ltr = tx >= sx;
      const pieces = [];
      if (cutLo > lo + EPS) pieces.push(ltr ? { sx: lo, sy, tx: cutLo, ty } : { sx: cutLo, sy, tx: lo, ty });
      if (cutHi < hi - EPS) pieces.push(ltr ? { sx: cutHi, sy, tx: hi, ty } : { sx: hi, sy, tx: cutHi, ty });
      return pieces;
    }
    return [seg];
  }

  function applyLabelAvoidRectsToSegments(segments, rects) {
    let out = Array.isArray(segments) ? segments.slice() : [];
    const boxes = Array.isArray(rects) ? rects : [];
    if (!out.length || !boxes.length) return out;
    for (let i = 0; i < boxes.length; i += 1) {
      const rect = boxes[i];
      if (!rect) continue;
      const next = [];
      for (let j = 0; j < out.length; j += 1) {
        const seg = out[j];
        const pieces = splitAxisAlignedSegmentByRect(seg, rect);
        if (!pieces.length) continue;
        pieces.forEach((piece) =>
          next.push({
            ...seg,
            sx: Number(piece.sx) || 0,
            sy: Number(piece.sy) || 0,
            tx: Number(piece.tx) || 0,
            ty: Number(piece.ty) || 0,
          })
        );
      }
      out = next;
      if (!out.length) break;
    }
    return out;
  }

  function segmentMid(seg) {
    if (!seg) return { x: 0, y: 0 };
    return {
      x: ((Number(seg.sx) || 0) + (Number(seg.tx) || 0)) / 2,
      y: ((Number(seg.sy) || 0) + (Number(seg.ty) || 0)) / 2,
    };
  }
  function computeSingleEdgeLabelAnchor(edge, sizeScale = 1, opts = {}) {
    const disableNeighborSnap = !!opts?.disableNeighborSnap;
    const source = edge?.getSource?.();
    const target = edge?.getTarget?.();
    const m = edge?.getModel?.();
    const sm = source?.getModel?.();
    const tm = target?.getModel?.();
    if (!m || !sm || !tm) return null;
    const sx = Number(sm.x) || 0;
    const sy = Number(sm.y) || 0;
    const tx = Number(tm.x) || 0;
    const ty = Number(tm.y) || 0;
    const edgeOffset = (Number(m.edgeOffset) || 0) * sizeScale;
    const baseDir = normalizeVector(tx - sx, ty - sy);
    const basePerp = { x: -baseDir.y, y: baseDir.x };
    const sx0 = sx + basePerp.x * edgeOffset;
    const sy0 = sy + basePerp.y * edgeOffset;
    const tx0 = tx + basePerp.x * edgeOffset;
    const ty0 = ty + basePerp.y * edgeOffset;

    const route = String(m.edgeRoute || m.route || "").toLowerCase();
    const isDouble = String(m.mode || "").toLowerCase() === "double";
    const axis = String(m.orthAxis || "").toLowerCase() === "vertical" ? "vertical" : "horizontal";
    const sr = (Number(sm.r) || 18) * sizeScale;
    const tr = (Number(tm.r) || 18) * sizeScale;
    const baseLineWidth = (Number(m.lineWidth) || 2) * sizeScale;
    const states = edge?.getStates?.() || [];
    const isSelected = states.includes("selected") || states.includes("active");
    const isHover = !isSelected && states.includes("hover");
    let edgeLineWidth = baseLineWidth;
    if (isSelected) edgeLineWidth = baseLineWidth + 2 * sizeScale;
    else if (isHover) edgeLineWidth = baseLineWidth + 1 * sizeScale;
    const sStroke = Number(sm.lineWidth ?? 2) || 2;
    const tStroke = Number(tm.lineWidth ?? 2) || 2;
    const sSelected = source?.hasState?.("selected") || source?.hasState?.("active");
    const tSelected = target?.hasState?.("selected") || target?.hasState?.("active");
    const useOrthogonal = !isDouble && route === "orthogonal";
    const sPad =
      (Math.max(0.9, sStroke / 2 + 0.4) + (useOrthogonal ? 0 : sSelected ? 1.8 : 0)) * sizeScale;
    const tPad =
      (Math.max(0.9, tStroke / 2 + 0.4) + (useOrthogonal ? 0 : tSelected ? 1.8 : 0)) * sizeScale;
    const outPad = (Number(m.outPad ?? 0.8) || 0) * sizeScale;
    const holePad = (Number(m.holePad ?? 2) || 0) * sizeScale;
    const extraS = outPad + holePad + sPad;
    const extraT = outPad + holePad + tPad;
    const routingLineWidth = useOrthogonal ? baseLineWidth : edgeLineWidth;
    const endpoints = useOrthogonal
      ? computeOrthogonalEndpoints(sx0, sy0, tx0, ty0, sr, tr, routingLineWidth, extraS, extraT, axis)
      : computeEdgeEndpoints(sx0, sy0, tx0, ty0, sr, tr, edgeLineWidth, extraS, extraT);
    const arrowMode = m.edgeArrow || (m.showArrow === false ? "none" : "end");
    const showArrow = m.showArrow !== false && arrowMode !== "none";
    const showEnd = arrowMode === "end" || arrowMode === "both";
    const showStart = arrowMode === "start" || arrowMode === "both";
    const arrowSize = Math.max((Number(m.arrowSize) || 10) * sizeScale, edgeLineWidth * 3);
    const arrowTrim = computeArrowLineTrim(arrowSize, edgeLineWidth, sizeScale);
    const label = String(m.label || "");
    const labelSize = Math.max(8, clamp(Number(m.fontSize ?? 13), 8, 36) - 1);
    const labelDim = label
      ? {
          w: Math.max(24, estimateTextWidth(label, labelSize)) * sizeScale,
          h: Math.max(12, labelSize + 6) * sizeScale,
        }
      : { w: 0, h: 0 };
    const labelAnchorMode = String(m.edgeLabelAnchorMode || m.edgeLabelAnchor || m.labelAnchor || "")
      .trim()
      .toLowerCase();
    const useCenterSnapAnchor = labelAnchorMode === "center-snap" && useOrthogonal;
    const useSideAnchor = labelAnchorMode === "side";
    const orthoAnchorOpts = resolveOrthogonalLabelAnchorOpts(m, axis, sizeScale);
    const startTrim = showArrow && showStart ? arrowTrim : 0;
    const endTrim = showArrow && showEnd ? arrowTrim : 0;
    if (!useOrthogonal) {
      const p0 = {
        x: endpoints.sx + (showStart ? startTrim : 0) * ((endpoints?.dir?.x) || 0),
        y: endpoints.sy + (showStart ? startTrim : 0) * ((endpoints?.dir?.y) || 0),
      };
      const p1 = {
        x: endpoints.tx - (showEnd ? endTrim : 0) * ((endpoints?.dir?.x) || 0),
        y: endpoints.ty - (showEnd ? endTrim : 0) * ((endpoints?.dir?.y) || 0),
      };
      const midpoint = { x: (p0.x + p1.x) / 2, y: (p0.y + p1.y) / 2 };
      if (!useSideAnchor) return midpoint;

      const baseAbsDx = Math.abs(tx0 - sx0);
      const baseAbsDy = Math.abs(ty0 - sy0);
      const mostlyHorizontal = baseAbsDx >= baseAbsDy * 0.8;
      // Horizontal column alignment works for near-horizontal links only.
      // For oblique links it moves the label off the line and desyncs label-gap placement.
      const useHorizontalColumnAlign = mostlyHorizontal && baseAbsDy <= Math.max(1, baseAbsDx) * 0.45;
      const side = resolveLabelSide(edge, useHorizontalColumnAlign ? "horizontal" : "vertical");
      const near = side === "source" ? p0 : p1;
      const far = side === "source" ? p1 : p0;
      const deltaX = far.x - near.x;
      const deltaY = far.y - near.y;
      const absDx = Math.abs(deltaX);
      const absDy = Math.abs(deltaY);
      const axisSpan = absDx >= absDy ? absDx : absDy;
      const middleRatioRaw = Number(m.edgeLabelMiddleRatio);
      const sideCapRatioRaw = Number(m.edgeLabelSideCapRatio);
      const baseOffset = Math.max(26 * sizeScale, orthoAnchorOpts.sideOffset);
      const middleRatio = Number.isFinite(middleRatioRaw)
        ? clamp(middleRatioRaw, 0.18, 0.6)
        : mostlyHorizontal
        ? 0.47
        : 0.38;
      const sideCapRatio = Number.isFinite(sideCapRatioRaw) ? clamp(sideCapRatioRaw, 0.38, 0.7) : 0.52;
      const cappedBase = Math.min(baseOffset, axisSpan * sideCapRatio);
      const endInset = Math.max(8 * sizeScale, 12 * sizeScale);
      const maxUsable = Math.max(0, axisSpan - endInset);
      const d = Math.min(Math.max(cappedBase, axisSpan * middleRatio), maxUsable);
      const t = axisSpan > EPS ? d / axisSpan : 0;
      const anchor = {
        x: near.x + deltaX * t,
        y: near.y + deltaY * t,
      };
      if (!disableNeighborSnap && useHorizontalColumnAlign) {
        const snapped = snapHorizontalLabelToNeighborColumns(edge, sizeScale, p0, p1, anchor.x);
        if (snapped) {
          return alignLabelColumnPoint(snapped, edge, side, labelDim.w, true, 0);
        }
      }
      return alignLabelColumnPoint(
        anchor,
        edge,
        side,
        labelDim.w,
        useHorizontalColumnAlign,
        useHorizontalColumnAlign ? 0.24 : 0
      );
    }
    const routeData = buildOrthogonalRouteData(
      { x: endpoints.sx, y: endpoints.sy },
      { x: endpoints.tx, y: endpoints.ty },
      axis,
      Number(m.orthBias),
      startTrim,
      endTrim,
      axis,
      Number.isFinite(Number(m.orthSharedCoord)) ? Number(m.orthSharedCoord) : null
    );
    if (useCenterSnapAnchor) {
      const p0 = {
        x: endpoints.sx + (showStart ? startTrim : 0) * ((endpoints?.dir?.x) || 0),
        y: endpoints.sy + (showStart ? startTrim : 0) * ((endpoints?.dir?.y) || 0),
      };
      const p1 = {
        x: endpoints.tx - (showEnd ? endTrim : 0) * ((endpoints?.dir?.x) || 0),
        y: endpoints.ty - (showEnd ? endTrim : 0) * ((endpoints?.dir?.y) || 0),
      };
      const midpoint = { x: (p0.x + p1.x) / 2, y: (p0.y + p1.y) / 2 };
      if (!disableNeighborSnap && axis === "horizontal") {
        const snapped = snapHorizontalLabelToNeighborColumns(edge, sizeScale, p0, p1, midpoint.x);
        if (snapped) {
          const side = resolveLabelSide(edge, axis);
          return alignLabelColumnPoint(snapped, edge, side, labelDim.w, true, 0);
        }
      }
      return midpoint;
    }
    if (!useSideAnchor) {
      if (routeData?.labelPoint && Number.isFinite(routeData.labelPoint.x) && Number.isFinite(routeData.labelPoint.y)) {
        return routeData.labelPoint;
      }
      return { x: (endpoints.sx + endpoints.tx) / 2, y: (endpoints.sy + endpoints.ty) / 2 };
    }
    const picked = resolveOrthogonalLabelPoint(edge, routeData, axis, {
      sideOffset: orthoAnchorOpts.sideOffset,
      middleRatio: orthoAnchorOpts.middleRatio,
      sideCapRatio: orthoAnchorOpts.sideCapRatio,
    });
    if (picked?.point && Number.isFinite(picked.point.x) && Number.isFinite(picked.point.y)) {
      if (!disableNeighborSnap && axis === "horizontal") {
        const verticalIdx = collectAxisSegmentIndices(routeData.segments, "vertical");
        const hasVertical = verticalIdx.some((idx) => segmentLength(routeData.segments?.[idx]) > EPS);
        if (!hasVertical) {
          const seg = routeData.segments?.[picked.index];
          if (seg && segmentAxis(seg) === "horizontal") {
            const snapped = snapHorizontalLabelToNeighborColumns(
              edge,
              sizeScale,
              { x: Number(seg.sx) || 0, y: Number(seg.sy) || 0 },
              { x: Number(seg.tx) || 0, y: Number(seg.ty) || 0 },
              Number(picked.point.x)
            );
            if (snapped) {
              const side = resolveLabelSide(edge, axis);
              return alignLabelColumnPoint(snapped, edge, side, labelDim.w, true, 0);
            }
          }
        }
      }
      if (!disableNeighborSnap && axis === "vertical") {
        const horizontalIdx = collectAxisSegmentIndices(routeData.segments, "horizontal");
        const hasHorizontal = horizontalIdx.some((idx) => segmentLength(routeData.segments?.[idx]) > EPS);
        if (!hasHorizontal) {
          const seg = routeData.segments?.[picked.index];
          if (seg && segmentAxis(seg) === "vertical") {
            const snapped = snapVerticalLabelToNeighborRows(
              edge,
              sizeScale,
              { x: Number(seg.sx) || 0, y: Number(seg.sy) || 0 },
              { x: Number(seg.tx) || 0, y: Number(seg.ty) || 0 },
              Number(picked.point.y)
            );
            if (snapped) return snapped;
          }
        }
      }
      const side = resolveLabelSide(edge, axis);
      return alignLabelColumnPoint(
        picked.point,
        edge,
        side,
        labelDim.w,
        axis === "horizontal",
        axis === "horizontal" ? 0 : 0.5
      );
    }
    return routeData.labelPoint;
  }

  function resolveDoubleGap(model, sizeScale = 1) {
    const raw = Number(model?.gap);
    const gap = Number.isFinite(raw) && raw > 0 ? raw : DEFAULT_DOUBLE_GAP;
    return gap * (Number(sizeScale) || 1);
  }

  function resolveLabelAdvanceSign(edge, side, axis = "horizontal") {
    const sm = edge?.getSource?.()?.getModel?.() || null;
    const tm = edge?.getTarget?.()?.getModel?.() || null;
    if (!sm || !tm) return side === "target" ? -1 : 1;
    const sx = Number(sm.x) || 0;
    const sy = Number(sm.y) || 0;
    const tx = Number(tm.x) || 0;
    const ty = Number(tm.y) || 0;
    if (axis === "vertical") {
      const delta = side === "source" ? ty - sy : sy - ty;
      if (Math.abs(delta) > EPS) return delta >= 0 ? 1 : -1;
      return side === "target" ? -1 : 1;
    }
    const delta = side === "source" ? tx - sx : sx - tx;
    if (Math.abs(delta) > EPS) return delta >= 0 ? 1 : -1;
    return side === "target" ? -1 : 1;
  }

  function collectNeighborHorizontalLabelColumns(edge, sizeScale = 1, maxCols = 20) {
    const source = edge?.getSource?.();
    const target = edge?.getTarget?.();
    if (!source && !target) return [];
    const candidates = [];
    const seen = new Set();
    const pushEdges = (node) => {
      const list = node?.getEdges?.() || [];
      for (let i = 0; i < list.length; i += 1) {
        const item = list[i];
        if (!item || item === edge || seen.has(item)) continue;
        seen.add(item);
        candidates.push(item);
        if (candidates.length >= 64) break;
      }
    };
    pushEdges(source);
    if (candidates.length < 64) pushEdges(target);
    const cols = [];
    for (let i = 0; i < candidates.length; i += 1) {
      const other = candidates[i];
      const om = other?.getModel?.() || {};
      const route = String(om.edgeRoute || om.route || "").toLowerCase();
      if (route !== "orthogonal") continue;
      const axis = String(om.orthAxis || "").toLowerCase() === "vertical" ? "vertical" : "horizontal";
      if (axis !== "horizontal") continue;
      if (om.source === om.target) continue;
      const anchor = computeSingleEdgeLabelAnchor(other, sizeScale, { disableNeighborSnap: true });
      if (!anchor || !Number.isFinite(anchor.x)) continue;
      cols.push(anchor.x);
      if (cols.length >= maxCols) break;
    }
    return cols;
  }

  function collectNeighborVerticalLabelRows(edge, sizeScale = 1, maxRows = 20) {
    const source = edge?.getSource?.();
    const target = edge?.getTarget?.();
    if (!source && !target) return [];
    const candidates = [];
    const seen = new Set();
    const pushEdges = (node) => {
      const list = node?.getEdges?.() || [];
      for (let i = 0; i < list.length; i += 1) {
        const item = list[i];
        if (!item || item === edge || seen.has(item)) continue;
        seen.add(item);
        candidates.push(item);
        if (candidates.length >= 64) break;
      }
    };
    pushEdges(source);
    if (candidates.length < 64) pushEdges(target);
    const rows = [];
    const bendEps = Math.max(4, 8 * (Number(sizeScale) || 1));
    for (let i = 0; i < candidates.length; i += 1) {
      const other = candidates[i];
      const om = other?.getModel?.() || {};
      const route = String(om.edgeRoute || om.route || "").toLowerCase();
      if (route !== "orthogonal") continue;
      const axis = String(om.orthAxis || "").toLowerCase() === "vertical" ? "vertical" : "horizontal";
      if (axis !== "vertical") continue;
      if (om.source === om.target) continue;
      const os = other?.getSource?.()?.getModel?.() || null;
      const ot = other?.getTarget?.()?.getModel?.() || null;
      const osx = Number(os?.x) || 0;
      const otx = Number(ot?.x) || 0;
      // Use turned edges as row baseline.
      if (Math.abs(osx - otx) <= bendEps) continue;
      const anchor = computeSingleEdgeLabelAnchor(other, sizeScale, { disableNeighborSnap: true });
      if (!anchor || !Number.isFinite(anchor.y)) continue;
      rows.push(anchor.y);
      if (rows.length >= maxRows) break;
    }
    return rows;
  }

  function pickSegmentColumnX(columns, start, end, preferX, inset = 0) {
    const list = Array.isArray(columns) ? columns : [];
    if (!list.length || !start || !end) return null;
    const pad = Math.max(0, Number(inset) || 0);
    const minX = Math.min(Number(start.x) || 0, Number(end.x) || 0) + pad;
    const maxX = Math.max(Number(start.x) || 0, Number(end.x) || 0) - pad;
    if (maxX <= minX + EPS) return null;
    const px = Number.isFinite(preferX) ? preferX : (minX + maxX) / 2;
    let best = null;
    let bestScore = Infinity;
    list.forEach((raw) => {
      const x = Number(raw);
      if (!Number.isFinite(x)) return;
      if (x < minX - EPS || x > maxX + EPS) return;
      const score = Math.abs(x - px);
      if (score + 1e-6 < bestScore) {
        best = x;
        bestScore = score;
      }
    });
    return Number.isFinite(best) ? best : null;
  }

  function pickSegmentRowY(rows, start, end, preferY, inset = 0) {
    const list = Array.isArray(rows) ? rows : [];
    if (!list.length || !start || !end) return null;
    const pad = Math.max(0, Number(inset) || 0);
    const minY = Math.min(Number(start.y) || 0, Number(end.y) || 0) + pad;
    const maxY = Math.max(Number(start.y) || 0, Number(end.y) || 0) - pad;
    if (maxY <= minY + EPS) return null;
    const py = Number.isFinite(preferY) ? preferY : (minY + maxY) / 2;
    let best = null;
    let bestScore = Infinity;
    list.forEach((raw) => {
      const y = Number(raw);
      if (!Number.isFinite(y)) return;
      if (y < minY - EPS || y > maxY + EPS) return;
      const score = Math.abs(y - py);
      if (score + 1e-6 < bestScore) {
        best = y;
        bestScore = score;
      }
    });
    return Number.isFinite(best) ? best : null;
  }

  function projectPointToSegmentByX(start, end, x, inset = 0) {
    if (!start || !end || !Number.isFinite(x)) return null;
    const sx = Number(start.x) || 0;
    const sy = Number(start.y) || 0;
    const tx = Number(end.x) || 0;
    const ty = Number(end.y) || 0;
    const dx = tx - sx;
    const dy = ty - sy;
    const len = Math.hypot(dx, dy);
    if (len <= EPS || Math.abs(dx) <= EPS) return null;
    const pad = Math.max(0, Number(inset) || 0);
    const bound = clamp(pad / len, 0, 0.49);
    const minT = bound;
    const maxT = 1 - bound;
    const tRaw = (x - sx) / dx;
    const t = clamp(tRaw, minT, maxT);
    return { x: sx + dx * t, y: sy + dy * t };
  }

  function projectPointToSegmentByY(start, end, y, inset = 0) {
    if (!start || !end || !Number.isFinite(y)) return null;
    const sx = Number(start.x) || 0;
    const sy = Number(start.y) || 0;
    const tx = Number(end.x) || 0;
    const ty = Number(end.y) || 0;
    const dx = tx - sx;
    const dy = ty - sy;
    const len = Math.hypot(dx, dy);
    if (len <= EPS || Math.abs(dy) <= EPS) return null;
    const pad = Math.max(0, Number(inset) || 0);
    const bound = clamp(pad / len, 0, 0.49);
    const minT = bound;
    const maxT = 1 - bound;
    const tRaw = (y - sy) / dy;
    const t = clamp(tRaw, minT, maxT);
    return { x: sx + dx * t, y: sy + dy * t };
  }

  function snapHorizontalLabelToNeighborColumns(edge, sizeScale, start, end, preferX) {
    const columns = collectNeighborHorizontalLabelColumns(edge, sizeScale);
    if (!columns.length) return null;
    const snapInset = Math.max(8 * sizeScale, 12 * sizeScale);
    const snapX = pickSegmentColumnX(columns, start, end, preferX, snapInset);
    if (!Number.isFinite(snapX)) return null;
    return projectPointToSegmentByX(start, end, snapX, snapInset);
  }

  function snapVerticalLabelToNeighborRows(edge, sizeScale, start, end, preferY) {
    const rows = collectNeighborVerticalLabelRows(edge, sizeScale);
    if (!rows.length) return null;
    const snapInset = Math.max(8 * sizeScale, 12 * sizeScale);
    const snapY = pickSegmentRowY(rows, start, end, preferY, snapInset);
    if (!Number.isFinite(snapY)) return null;
    return projectPointToSegmentByY(start, end, snapY, snapInset);
  }

  function alignLabelColumnPoint(point, edge, side, labelWidth = 0, horizontal = true, widthBias = 0.5) {
    if (!point || !horizontal) return point;
    const w = Math.max(0, Number(labelWidth) || 0);
    if (w <= EPS) return point;
    const bias = clamp(Number(widthBias), 0, 0.5);
    if (bias <= EPS) return point;
    const sign = resolveLabelAdvanceSign(edge, side, "horizontal");
    return { x: point.x - sign * w * bias, y: point.y };
  }

  const DEFAULT_DOUBLE_GAP = 20;
  const SELECTED_PULSE_LIMIT = 30;
  const EDGE_AA_V2_ENABLED = true;
  const EDGE_MIN_SCREEN_WIDTH_PX = 1.5;
  const EDGE_DASH_LOD_PERIOD_PX = 4.0;
  const EDGE_DASH_LOD_BLEND_PX = 1.1;
  const EDGE_AA_MIN_PX = 1.2;
  const NODE_AA_PAD_PX = 4.5;
  // Keep edge/text contrast stable at normal zoom, then adapt at tiny zoom for readability.
  const EDGE_BASE_CONTRAST = 1;
  const EDGE_LOW_ZOOM_CONTRAST_MIN = 0.58;
  const NODE_BASE_CONTRAST = 1;
  const NODE_LOW_ZOOM_CONTRAST_MAX = 1.24;
  const LABEL_BASE_CONTRAST = 1;
  const LABEL_BASE_ALPHA = 1;
  const ZOOM_VISUAL_ADAPT_START = 0.58;
  const ZOOM_VISUAL_ADAPT_END = 0.16;
  const EDGE_ABSTRACT_FADE_START = 0.54;
  const EDGE_ABSTRACT_FADE_END = 0.16;
  const EDGE_ABSTRACT_ALPHA_MIN = 0.1;
  // Defer label reduction to smaller zoom levels so normal analysis stays readable.
  const NODE_LABEL_FOCUS_ZOOM = 0.3;
  const NODE_LABEL_MINIMAL_ZOOM = 0.18;
  const NODE_LABEL_FOCUS_KEEP_FACTOR = 3.4;
  const NODE_LABEL_FOCUS_KEEP_MIN = 18;
  const NODE_LABEL_FOCUS_KEEP_MAX = 140;
  const NODE_LABEL_MIN_KEEP_FACTOR = 1.5;
  const NODE_LABEL_MIN_KEEP_MIN = 10;
  const NODE_LABEL_MIN_KEEP_MAX = 40;

  const ICON_DATA_URL_CACHE = new Map();
  const ICON_IMAGE_CACHE = new Map();
  let ICON_MEASURE_CANVAS = null;
  let ICON_MEASURE_CTX = null;

  function computeImageVisualOffset(img) {
    // Compute visual bbox center and nudge icon so perceived center matches node center.
    if (!img) return { offsetX: 0, offsetY: 0 };
    const w = img.naturalWidth || img.width || 0;
    const h = img.naturalHeight || img.height || 0;
    if (!w || !h) return { offsetX: 0, offsetY: 0 };
    const limit = 96;
    const scale = Math.min(1, limit / Math.max(w, h));
    const sw = Math.max(1, Math.round(w * scale));
    const sh = Math.max(1, Math.round(h * scale));
    if (!ICON_MEASURE_CANVAS) {
      ICON_MEASURE_CANVAS = document.createElement("canvas");
      ICON_MEASURE_CTX = ICON_MEASURE_CANVAS.getContext("2d", { willReadFrequently: true });
    }
    if (!ICON_MEASURE_CTX) return { offsetX: 0, offsetY: 0 };
    ICON_MEASURE_CANVAS.width = sw;
    ICON_MEASURE_CANVAS.height = sh;
    ICON_MEASURE_CTX.clearRect(0, 0, sw, sh);
    ICON_MEASURE_CTX.drawImage(img, 0, 0, sw, sh);
    let data;
    try {
      data = ICON_MEASURE_CTX.getImageData(0, 0, sw, sh).data;
    } catch (e) {
      return { offsetX: 0, offsetY: 0 };
    }
    let minX = sw;
    let minY = sh;
    let maxX = -1;
    let maxY = -1;
    for (let y = 0; y < sh; y += 1) {
      const row = y * sw * 4;
      for (let x = 0; x < sw; x += 1) {
        const a = data[row + x * 4 + 3];
        if (a < 12) continue;
        if (x < minX) minX = x;
        if (x > maxX) maxX = x;
        if (y < minY) minY = y;
        if (y > maxY) maxY = y;
      }
    }
    if (maxX < minX || maxY < minY) return { offsetX: 0, offsetY: 0 };
    const cx = (minX + maxX + 1) * 0.5;
    const cy = (minY + maxY + 1) * 0.5;
    const dx = sw * 0.5 - cx;
    const dy = sh * 0.5 - cy;
    // Normalized to icon size; clamp to avoid over-correction.
    const offsetX = clamp(dx / sw, -0.18, 0.18);
    const offsetY = clamp(dy / sh, -0.18, 0.18);
    return { offsetX, offsetY };
  }

  function buildIconDataUrl(symbolId, color = "#ffffff") {
    const raw = String(symbolId || "").trim();
    if (!raw) return "";
    const id = raw.startsWith("#") ? raw.slice(1) : raw;
    if (!id) return "";
    const key = `${id}|${color}`;
    if (ICON_DATA_URL_CACHE.has(key)) return ICON_DATA_URL_CACHE.get(key);
    const symbol = document.getElementById(id);
    if (!symbol) return "";
    const viewBox = symbol.getAttribute("viewBox") || "0 0 24 24";
    const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${viewBox}" width="24" height="24" fill="none" color="${color}" shape-rendering="geometricPrecision">${symbol.innerHTML}</svg>`;
    const dataUrl = `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
    ICON_DATA_URL_CACHE.set(key, dataUrl);
    return dataUrl;
  }

  function getIconImageRecord(url, owner) {
    if (!url) return null;
    let rec = ICON_IMAGE_CACHE.get(url);
    if (!rec) {
      const img = new Image();
      rec = { url, img, ready: false, error: false, width: 0, height: 0, subscribers: new Set() };
      img.onload = () => {
        rec.ready = true;
        rec.width = img.naturalWidth || img.width || 0;
        rec.height = img.naturalHeight || img.height || 0;
        const visual = computeImageVisualOffset(img);
        rec.offsetX = visual.offsetX || 0;
        rec.offsetY = visual.offsetY || 0;
        rec.subscribers.forEach((graph) => {
          try {
            graph._labelsDirty = true;
            graph._textDirty = true;
            graph._scheduleRender();
          } catch (e) {}
        });
        rec.subscribers.clear();
      };
      img.onerror = () => {
        rec.error = true;
        rec.subscribers.clear();
      };
      img.src = url;
      ICON_IMAGE_CACHE.set(url, rec);
    }
    if (!rec.ready && !rec.error && owner) rec.subscribers.add(owner);
    return rec;
  }

  function encodePickColor(id) {
    const r = id & 255;
    const g = (id >> 8) & 255;
    const b = (id >> 16) & 255;
    const a = (id >> 24) & 255;
    return [r / 255, g / 255, b / 255, a / 255];
  }

  class GraphItem {
    constructor(model) {
      this._model = model;
      this._states = new Set();
    }
    getModel() {
      return this._model;
    }
    getContainer() {
      return null;
    }
    getStates() {
      return Array.from(this._states);
    }
    hasState(state) {
      return this._states.has(state);
    }
    setState(state, value) {
      if (value) this._states.add(state);
      else this._states.delete(state);
    }
  }

  class NodeItem extends GraphItem {
    constructor(model) {
      super(model);
      this._edges = [];
    }
    addEdge(edge) {
      this._edges.push(edge);
    }
    getEdges() {
      return this._edges;
    }
    getID() {
      return this._model?.id;
    }
  }

  class EdgeItem extends GraphItem {
    constructor(model, source, target) {
      super(model);
      this._source = source;
      this._target = target;
    }
    getSource() {
      return this._source;
    }
    getTarget() {
      return this._target;
    }
    getID() {
      return this._model?.id;
    }
  }

  class GraphAdapter {
    constructor(opts = {}) {
      this.container = opts.container;
      this.width = opts.width || (this.container?.clientWidth || 800);
      this.height = opts.height || (this.container?.clientHeight || 600);
      this.minZoom = Number.isFinite(opts.minZoom) ? opts.minZoom : 0.05;
      this.maxZoom = Number.isFinite(opts.maxZoom) ? opts.maxZoom : 16;
      this.modes = opts.modes || { default: [] };
      this.defaultNode = opts.defaultNode || {};
      this.defaultEdge = opts.defaultEdge || {};
      this.renderer = opts.renderer || "webgl";
      this.autoPaint = true;
      this._events = new Map();
      this._plugins = [];
      this._nodes = [];
      this._edges = [];
      this._nodeMap = new Map();
      this._edgeMap = new Map();
      this._data = { nodes: [], edges: [] };
      this._dirty = true;
      this._positionsDirty = true;
      this._labelsDirty = true;
      this._draggingNode = null;
      this._draggingCanvas = false;
      this._lastHoverNode = null;
      this._lastHoverEdge = null;
      this._interactionActive = false;
      this._pickGrid = new Map();
      this._pickGridSize = 120;
      this._pickGridDirty = true;
      this._edgeGrid = new Map();
      this._edgeGridSize = 220;
      this._edgeGridDirty = true;
      this._partialNodes = null;
      this._partialEdges = null;
      this._haloNodes = new Set();
      this._haloDirty = true;
      this._shadowNodes = new Set();
      this._shadowNodesDirty = true;
      this._edgeGeomDirty = true;
      this._scaleWithView = opts.scaleWithView !== false;
      this._textAtlasScale = Number(opts.textAtlasScale) || TEXT_ATLAS_SCALE;
      this._textQuality = normalizeTextQualityMode(opts.textQuality || "gpu");
      this._useGpuText = opts.useGpuText != null ? !!opts.useGpuText : true;
      this._textMode = "gpu";
      this._textBackend = "text-atlas";
      this._textFallbackAllowed = false;
      this._textPolicyLocked = true;
      this._textBaseSize = TEXT_BASE_SIZE;
      this._textAtlases = new Map();
      this._textBatches = [];
      this._textStride = 12;
      this._textDirty = true;
      this._textBufferMap = new Map();
      this._textUnitBuffer = null;
      this._textProgram = null;
      this._snapTextToPixel = opts.snapTextToPixel !== false;
      this._snapGlyphToPixel = opts.snapGlyphToPixel !== false;
      this._labelCountEstimate = 0;
      this._programInfo = {};
      this._shaderInfo = {};
      this._webglWarn = opts.webglWarn != null ? !!opts.webglWarn : false;
      this._webglWarnOnce = new Set();
      this._wheelTimer = 0;
      this._useGpuPick = true;
      this._pickDataDirty = true;
      this._pickRenderDirty = true;
      this._pickNodeCount = 0;
      this._pickEdgeCount = 0;
      this._pickFbo = null;
      this._pickTex = null;
      this._pickDepth = null;
      this._pickNodeData = null;
      this._pickEdgeData = null;
      this._pickNodeBuffer = null;
      this._pickEdgeBuffer = null;
      this._pickNodeProgram = null;
      this._pickEdgeProgram = null;
      this._forceFullBufferRefresh = false;
      this._pixelRatio = window.devicePixelRatio || 1;
      this._highDpi = !!opts.highDpi;
      this._highDpiMax = Number.isFinite(opts.highDpiMax) ? Number(opts.highDpiMax) : 4;
      this._scale = 1;
      this._translate = { x: this.width / 2, y: this.height / 2 };
      this._lastSizeScale = this._getSizeScale();
      this._pickLastScale = this._scale;
      this._pickLastTranslate = { x: this._translate.x, y: this._translate.y };
      this._lastPickHit = null;
      this._hoverRaf = 0;
      this._hoverEvent = null;
      this._gpuTextEnableShadow = !!opts.gpuTextShadow;
      this._deferEdgeUpdate = false;
      this._deferEdgeUpdateThreshold = 420;
      this._disablePickDuringInteraction = true;
      this._wheelPickTimer = 0;
      this._wheelActive = false;
      this._wheelRouteDebug = !!(opts.debugWheelRoute || opts.debugTraceWheel || opts.debugTrace);
      this._wheelRouteLogAt = 0;
      this._edgeLiteMode = false;
      this._edgeLiteThreshold = 600;
      this._edgeCullEnabled = true;
      this._edgeCullThreshold = 1200;
      this._edgeViewDirty = true;
      this._edgeCullActive = false;
      this._edgeVisibleBuffer = null;
      this._arrowVisibleBuffer = null;
      this._edgeVisibleData = null;
      this._arrowVisibleData = null;
      this._edgeVisibleCount = 0;
      this._arrowVisibleCount = 0;
      this._highlightEdges = new Set();
      this._highlightDirty = true;
      this._highlightEdgeBuffer = null;
      this._highlightEdgeData = null;
      this._highlightEdgeCount = 0;
      this._edgeWorker = null;
      this._edgeWorkerBusy = false;
      this._edgeWorkerSeq = 0;
      this._edgeWorkerPending = null;
      this._edgeWorkerSupported = typeof Worker !== "undefined";
      this._edgeWorkerThreshold = 900;
      this._renderRepairCount = 0;
      this._renderRepairReason = "";
      this._renderRepairAt = 0;
      this._pulseActive = false;
      this._pulseStart = performance.now();
      this._raf = 0;
      this._boundCanvasEvents = [];
      this._destroyed = false;
      this._clearColor = WEBGL_CLEAR_FALLBACK.slice();
      this._initCanvases();
      this._initGL();
      this._applyModes(this.modes);
      this._bindEvents();
    }

    // --- public API ---
    get(name) {
      if (name === "container") return this.container;
      if (name === "width") return this.width;
      if (name === "height") return this.height;
      if (name === "renderer") return this.renderer;
      if (name === "modes") return this.modes;
      return undefined;
    }

    set(name, value) {
      if (name === "modes") {
        this.modes = value || { default: [] };
        this._applyModes(this.modes);
      }
      if (name === "width") this.width = value;
      if (name === "height") this.height = value;
      if (name === "webglWarn") {
        this._webglWarn = !!value;
        if (!this._webglWarn) this._webglWarnOnce.clear();
      }
    }

    getRenderer() {
      return this.renderer;
    }

    getZoom() {
      return this._scale;
    }

    setAutoPaint(flag) {
      this.autoPaint = !!flag;
    }

    setMode(name) {
      this._activeMode = name || "default";
    }

    _deleteGlBuffer(buffer) {
      if (!this.gl || !buffer) return;
      try {
        this.gl.deleteBuffer(buffer);
      } catch (e) {}
    }

    _deleteGlProgram(program) {
      if (!this.gl || !program) return;
      try {
        this.gl.deleteProgram(program);
      } catch (e) {}
    }

    _releaseTextAtlases() {
      const gl = this.gl;
      Array.from(this._textAtlases.entries()).forEach(([key, set]) => {
        (Array.isArray(set?.atlases) ? set.atlases : []).forEach((atlas) => {
          const buffer = this._textBufferMap.get(atlas);
          if (gl && buffer) {
            try {
              gl.deleteBuffer(buffer);
            } catch (e) {}
          }
          this._textBufferMap.delete(atlas);
        });
        try {
          set?.destroy?.();
        } catch (e) {}
        this._textAtlases.delete(key);
      });
      this._textBatches = [];
      this._textDirty = true;
    }

    _releaseGlPrograms() {
      this._deleteGlProgram(this._edgeProgram);
      this._deleteGlProgram(this._nodeProgram);
      this._deleteGlProgram(this._arrowProgram);
      this._deleteGlProgram(this._textProgram);
      this._deleteGlProgram(this._pickNodeProgram);
      this._deleteGlProgram(this._pickEdgeProgram);
      this._edgeProgram = null;
      this._nodeProgram = null;
      this._arrowProgram = null;
      this._textProgram = null;
      this._pickNodeProgram = null;
      this._pickEdgeProgram = null;
    }

    _releaseGlBuffers() {
      [
        "_edgeBuffer",
        "_edgeUnitBuffer",
        "_nodeBuffer",
        "_nodeUnitBuffer",
        "_shadowBuffer",
        "_arrowBuffer",
        "_arrowUnitBuffer",
        "_textUnitBuffer",
        "_pickNodeBuffer",
        "_pickEdgeBuffer",
        "_edgeVisibleBuffer",
        "_arrowVisibleBuffer",
        "_highlightEdgeBuffer",
      ].forEach((field) => {
        this._deleteGlBuffer(this[field]);
        this[field] = null;
      });
      this._textBufferMap.clear();
    }

    on(event, handler) {
      if (!this._events.has(event)) this._events.set(event, []);
      this._events.get(event).push(handler);
    }

    emit(event, payload) {
      const list = this._events.get(event);
      if (!list) return;
      list.forEach((fn) => {
        try {
          fn(payload);
        } catch (e) {}
      });
    }

    destroy() {
      if (this._destroyed) return;
      this._destroyed = true;
      this._events.clear();
      if (this._raf) {
        try {
          cancelAnimationFrame(this._raf);
        } catch (e) {}
        this._raf = 0;
      }
      if (this._hoverRaf) {
        try {
          cancelAnimationFrame(this._hoverRaf);
        } catch (e) {}
        this._hoverRaf = 0;
      }
      if (this._wheelTimer) {
        try {
          clearTimeout(this._wheelTimer);
        } catch (e) {}
        this._wheelTimer = 0;
      }
      if (this._wheelPickTimer) {
        try {
          clearTimeout(this._wheelPickTimer);
        } catch (e) {}
        this._wheelPickTimer = 0;
      }
      this._hoverEvent = null;
      this._edgeWorkerSeq += 1;
      this._edgeWorkerPending = false;
      this._edgeWorkerBusy = false;
      if (this._edgeWorker && typeof this._edgeWorker.terminate === "function") {
        try {
          this._edgeWorker.terminate();
        } catch (e) {}
      }
      this._edgeWorker = null;
      this._unbindEvents();
      if (Array.isArray(this._plugins) && this._plugins.length) {
        this._plugins.forEach((plugin) => {
          try {
            plugin?.destroy?.();
          } catch (e) {}
        });
      }
      this._plugins = [];
      try {
        this._clearGeometryGpuBuffers();
      } catch (e) {}
      try {
        this._releaseTextAtlases();
      } catch (e) {}
      const gl = this.gl;
      if (gl) {
        try {
          if (this._pickFbo) gl.deleteFramebuffer(this._pickFbo);
        } catch (e) {}
        try {
          if (this._pickTex) gl.deleteTexture(this._pickTex);
        } catch (e) {}
        try {
          if (this._pickDepth) gl.deleteRenderbuffer(this._pickDepth);
        } catch (e) {}
      }
      this._releaseGlBuffers();
      this._releaseGlPrograms();
      this._pickFbo = null;
      this._pickTex = null;
      this._pickDepth = null;
      if (this.canvas && this.canvas.parentNode) {
        try {
          this.canvas.parentNode.removeChild(this.canvas);
        } catch (e) {}
      }
      this._nodes = [];
      this._edges = [];
      this._nodeMap.clear();
      this._edgeMap.clear();
      this._data = { nodes: [], edges: [] };
      this.canvas = null;
      this.gl = null;
      this.container = null;
    }

    data(payload) {
      this._data = payload || { nodes: [], edges: [] };
      // Invalidate any late edge-worker result from previous topology.
      this._edgeWorkerSeq += 1;
      this._edgeWorkerPending = false;
      // Drop stale geometry counts immediately to avoid one-frame ghost draw.
      this._edgeSegmentsCount = 0;
      this._arrowCount = 0;
      this._edgeVisibleCount = 0;
      this._arrowVisibleCount = 0;
      this._highlightEdgeCount = 0;
      this._edgeViewDirty = true;
      this._dirty = true;
      this._edgeGeomDirty = true;
      this._pickGridDirty = true;
      this._edgeGridDirty = true;
      this._highlightEdges.clear();
      this._highlightDirty = true;
      this._positionsDirty = true;
      this._labelsDirty = true;
      this._labelCountEstimate = this._estimateLabelCount();
      this._clearGeometryGpuBuffers();
      if (!this._interactionActive) {
        this._applyTextModePolicy("data");
      }
      if (this.autoPaint) this.render();
    }

    changeData(payload) {
      this.data(payload);
      if (!this.autoPaint) return;
      this.render();
    }

    clear() {
      this._data = { nodes: [], edges: [] };
      // Invalidate any late edge-worker result from previous topology.
      this._edgeWorkerSeq += 1;
      this._edgeWorkerPending = false;
      this._nodes = [];
      this._edges = [];
      this._nodeMap.clear();
      this._edgeMap.clear();
      this._highlightEdges.clear();
      this._highlightDirty = true;
      // Reset geometry counters up front so draw pass won't reuse stale buffers.
      this._edgeSegmentsCount = 0;
      this._arrowCount = 0;
      this._edgeVisibleCount = 0;
      this._arrowVisibleCount = 0;
      this._highlightEdgeCount = 0;
      this._edgeViewDirty = true;
      this._dirty = true;
      this._edgeGeomDirty = true;
      this._pickGridDirty = true;
      this._edgeGridDirty = true;
      this._positionsDirty = true;
      this._labelsDirty = true;
      this._clearGeometryGpuBuffers();
      this._renderNow();
    }

    render() {
      this._scheduleRender();
    }

    paint() {
      this._renderNow();
    }

    addItem(type, model) {
      if (!model) return null;
      if (type === "node") {
        this._data.nodes = this._data.nodes || [];
        this._data.nodes.push(model);
        if (!Number.isFinite(model.x)) model.x = 0;
        if (!Number.isFinite(model.y)) model.y = 0;
        const item = new NodeItem(model);
        this._nodes.push(item);
        this._nodeMap.set(model.id, item);
        this._reindexRuntimeItems();
        this._pickGridDirty = true;
        this._pickDataDirty = true;
        this._pickRenderDirty = true;
        this._positionsDirty = true;
        this._labelsDirty = true;
        this._shadowNodesDirty = true;
        this._forceFullBufferRefresh = true;
        if (this.autoPaint) this.render();
        return item;
      }
      if (type === "edge") {
        this._data.edges = this._data.edges || [];
        this._data.edges.push(model);
        const source = this._nodeMap.get(model.source);
        const target = this._nodeMap.get(model.target);
        if (!source || !target) {
          this._dirty = true;
          this._edgeGeomDirty = true;
          this._edgeGridDirty = true;
        } else {
          const item = new EdgeItem(model, source, target);
          this._edges.push(item);
          this._edgeMap.set(model.id, item);
          source.addEdge(item);
          target.addEdge(item);
          this._reindexRuntimeItems();
          this._edgeGeomDirty = true;
          this._edgeGridDirty = true;
          this._pickDataDirty = true;
          this._pickRenderDirty = true;
          this._positionsDirty = true;
          this._labelsDirty = true;
          if (this.autoPaint) this.render();
          return item;
        }
        this._reindexRuntimeItems();
        this._positionsDirty = true;
        this._labelsDirty = true;
        this._pickDataDirty = true;
        this._pickRenderDirty = true;
        if (this.autoPaint) this.render();
        return this.findById(model.id);
      }
      return null;
    }

    removeItem(item) {
      if (!item) return;
      const model = item.getModel ? item.getModel() : null;
      if (!model) return;
      if (this._data.nodes) {
        this._data.nodes = this._data.nodes.filter((n) => n.id !== model.id);
      }
      if (this._data.edges) {
        this._data.edges = this._data.edges.filter((e) => e.id !== model.id);
      }
      this._dirty = true;
      this._edgeGeomDirty = true;
      this._pickGridDirty = true;
      this._edgeGridDirty = true;
      this._positionsDirty = true;
      this._labelsDirty = true;
      if (this.autoPaint) this.render();
    }

    updateItem(item, cfg = {}) {
      if (!item) return;
      const model = item.getModel ? item.getModel() : null;
      if (!model) return;
      Object.assign(model, cfg);
      if (Object.prototype.hasOwnProperty.call(cfg, "x") || Object.prototype.hasOwnProperty.call(cfg, "y")) {
        this._pickGridDirty = true;
      }
      if (Object.prototype.hasOwnProperty.call(cfg, "nodeShadow")) {
        this._shadowNodesDirty = true;
      }
      const structural = cfg.id || cfg.source || cfg.target;
      if (structural) this._dirty = true;
      const isEdge =
        item instanceof EdgeItem || (model && model.source != null && model.target != null && model.id != null);
      if (isEdge) {
        const geomKeys = [
          "mode",
          "edgeArrow",
          "showArrow",
          "edgeDash",
          "edgeDashStyle",
          "source",
          "target",
          "edgeRoute",
          "route",
          "orthAxis",
          "orthBias",
          "orthSharedCoord",
          "layoutBackflow",
          "__layoutBackflow",
        ];
        if (geomKeys.some((k) => Object.prototype.hasOwnProperty.call(cfg, k))) {
          this._edgeGeomDirty = true;
          this._edgeGridDirty = true;
        } else {
          if (!this._partialEdges) this._partialEdges = new Set();
          this._partialEdges.add(item);
        }
      } else {
        if (!this._partialNodes) this._partialNodes = new Set();
        this._partialNodes.add(item);
      }
      this._positionsDirty = true;
      this._labelsDirty = true;
      if (this.autoPaint) this.render();
    }

    refreshItem() {
      this._positionsDirty = true;
      this._labelsDirty = true;
      if (this.autoPaint) this.render();
    }

    refreshPositions() {
      this._positionsDirty = true;
      this._pickGridDirty = true;
      this._edgeGridDirty = true;
      this._edgeViewDirty = true;
      this._labelsDirty = true;
      if (this.autoPaint) this.render();
    }

    setItemState(item, state, value) {
      if (!item) return;
      item.setState(state, value);
      if (item instanceof NodeItem) {
        if (!this._partialNodes) this._partialNodes = new Set();
        this._partialNodes.add(item);
        if (state === "selected" || state === "active" || state === "hover") {
          if (value) this._haloNodes.add(item);
          else if (!item.hasState("selected") && !item.hasState("active") && !item.hasState("hover")) {
            this._haloNodes.delete(item);
          }
          this._haloDirty = true;
        }
      } else if (item instanceof EdgeItem) {
        if (!this._partialEdges) this._partialEdges = new Set();
        this._partialEdges.add(item);
        if (state === "selected" || state === "active") {
          const model = item.getModel?.();
          if (model) {
            if (value) {
              const lane = String(model.__hitLane || model.__selectedLane || "top").toLowerCase();
              model.__selectedLane = lane === "bottom" ? "bottom" : "top";
            } else if (!item.hasState("selected") && !item.hasState("active")) {
              delete model.__selectedLane;
            }
          }
        }
        if (state === "selected" || state === "active" || state === "hover") {
          this._updateHighlightEdge(item);
        }
      }
      this._positionsDirty = true;
      if (this.autoPaint) this.render();
    }

    getNodes() {
      return this._nodes;
    }

    getEdges() {
      return this._edges;
    }

    findById(id) {
      if (!id) return null;
      return this._nodeMap.get(id) || this._edgeMap.get(id) || null;
    }

    findAllByState(type, state) {
      if (type === "node") return this._nodes.filter((n) => n.hasState(state));
      if (type === "edge") return this._edges.filter((e) => e.hasState(state));
      return [];
    }

    focusItem(item, animate = false, opts = {}) {
      if (!item) return;
      const model = item.getModel ? item.getModel() : null;
      if (!model) return;
      const target = { x: Number(model.x) || 0, y: Number(model.y) || 0 };
      const to = {
        x: this.width / 2 - target.x * this._scale,
        y: this.height / 2 - target.y * this._scale,
      };
      if (!animate) {
        this._translate.x = to.x;
        this._translate.y = to.y;
        this._markEdgeViewDirty();
        this._labelsDirty = true;
        this._scheduleRender();
        this.emit("viewportchange", { type: "focus" });
        return;
      }
      const duration = Number(opts.duration) || 240;
      const start = { x: this._translate.x, y: this._translate.y };
      const t0 = performance.now();
      const step = () => {
        const t = clamp((performance.now() - t0) / duration, 0, 1);
        const k = t * (2 - t);
        this._translate.x = start.x + (to.x - start.x) * k;
        this._translate.y = start.y + (to.y - start.y) * k;
        this._markEdgeViewDirty();
        this._labelsDirty = true;
        this._renderNow();
        if (t < 1) requestAnimationFrame(step);
      };
      requestAnimationFrame(step);
    }

    fitView(padding = 40) {
      const nodes = this._nodes;
      if (!nodes.length) return;
      let minX = Infinity;
      let minY = Infinity;
      let maxX = -Infinity;
      let maxY = -Infinity;
      nodes.forEach((node) => {
        const m = node.getModel();
        const r = Number(m.r) || 0;
        const x = Number(m.x) || 0;
        const y = Number(m.y) || 0;
        minX = Math.min(minX, x - r);
        maxX = Math.max(maxX, x + r);
        minY = Math.min(minY, y - r);
        maxY = Math.max(maxY, y + r);
      });
      if (!Number.isFinite(minX)) return;
      const spanX = maxX - minX || 1;
      const spanY = maxY - minY || 1;
      const pad = Number(padding) || 0;
      const scaleX = (this.width - pad * 2) / spanX;
      const scaleY = (this.height - pad * 2) / spanY;
      const nextScale = clamp(Math.min(scaleX, scaleY), this.minZoom, this.maxZoom);
      this._scale = nextScale;
      const cx = (minX + maxX) / 2;
      const cy = (minY + maxY) / 2;
      this._translate.x = this.width / 2 - cx * nextScale;
      this._translate.y = this.height / 2 - cy * nextScale;
      this._markEdgeViewDirty();
      this._labelsDirty = true;
      this._pickRenderDirty = true;
      this._renderNow();
      this.emit("viewportchange", { type: "fit" });
    }

    fitCenter() {
      const nodes = this._nodes;
      if (!nodes.length) return;
      let minX = Infinity;
      let minY = Infinity;
      let maxX = -Infinity;
      let maxY = -Infinity;
      nodes.forEach((node) => {
        const m = node.getModel();
        const x = Number(m.x) || 0;
        const y = Number(m.y) || 0;
        minX = Math.min(minX, x);
        maxX = Math.max(maxX, x);
        minY = Math.min(minY, y);
        maxY = Math.max(maxY, y);
      });
      if (!Number.isFinite(minX)) return;
      const cx = (minX + maxX) / 2;
      const cy = (minY + maxY) / 2;
      this._translate.x = this.width / 2 - cx * this._scale;
      this._translate.y = this.height / 2 - cy * this._scale;
      this._markEdgeViewDirty();
      this._labelsDirty = true;
      this._pickRenderDirty = true;
      this._renderNow();
      this.emit("viewportchange", { type: "center" });
    }

    zoomTo(zoom, center) {
      const next = clamp(zoom, this.minZoom, this.maxZoom);
      const cx = center?.x ?? this.width / 2;
      const cy = center?.y ?? this.height / 2;
      const world = this.getPointByCanvas(cx, cy);
      this._scale = next;
      this._translate.x = cx - world.x * next;
      this._translate.y = cy - world.y * next;
      this._markEdgeViewDirty();
      this._positionsDirty = true;
      this._labelsDirty = true;
      this._pickRenderDirty = true;
      this._renderNow();
      this.emit("viewportchange", { type: "zoom" });
    }

    translate(dx, dy, opts = {}) {
      this._translate.x += dx;
      this._translate.y += dy;
      this._markEdgeViewDirty();
      const forceLabelRefresh =
        opts && Object.prototype.hasOwnProperty.call(opts, "refreshLabels")
          ? !!opts.refreshLabels
          : !this._shouldUseGpuText();
      if (forceLabelRefresh) this._labelsDirty = true;
      this._pickRenderDirty = true;
      this._renderNow();
      this.emit("viewportchange", { type: "pan" });
    }

    changeSize(w, h) {
      this.width = w;
      this.height = h;
      this._resize();
      this._markEdgeViewDirty();
      this._labelsDirty = true;
      this._pickRenderDirty = true;
      this._renderNow();
    }

    getPointByCanvas(x, y) {
      const gx = (x - this._translate.x) / this._scale;
      const gy = (y - this._translate.y) / this._scale;
      return { x: gx, y: gy };
    }

    getPointByClient(clientX, clientY) {
      const rect = this.canvas.getBoundingClientRect();
      const x = clientX - rect.left;
      const y = clientY - rect.top;
      return this.getPointByCanvas(x, y);
    }

    getCanvasByPoint(x, y) {
      return { x: x * this._scale + this._translate.x, y: y * this._scale + this._translate.y };
    }

    getClientByPoint(x, y) {
      const rect = this.canvas.getBoundingClientRect();
      const pt = this.getCanvasByPoint(x, y);
      return { x: rect.left + pt.x, y: rect.top + pt.y };
    }

    getViewBox() {
      const left = (-this._translate.x) / this._scale;
      const top = (-this._translate.y) / this._scale;
      const right = (this.width - this._translate.x) / this._scale;
      const bottom = (this.height - this._translate.y) / this._scale;
      return { x: left, y: top, width: right - left, height: bottom - top };
    }

    save() {
      return {
        nodes: (this._data.nodes || []).map((n) => ({ ...n })),
        edges: (this._data.edges || []).map((e) => ({ ...e })),
      };
    }

    toDataURL(type = "image/png", opts = {}) {
      const ratio = Number(opts?.ratio) || 1;
      const bg = opts?.backgroundColor || "transparent";
      try {
        this._renderNow();
      } catch (e) {}
      const out = document.createElement("canvas");
      out.width = Math.max(1, Math.round(this.width * ratio));
      out.height = Math.max(1, Math.round(this.height * ratio));
      const ctx = out.getContext("2d");
      ctx.fillStyle = bg;
      ctx.fillRect(0, 0, out.width, out.height);
      ctx.drawImage(this.canvas, 0, 0, out.width, out.height);
      return out.toDataURL(type);
    }

    addPlugin(plugin) {
      if (!plugin) return;
      if (typeof plugin.init === "function") plugin.init(this);
      this._plugins.push(plugin);
    }

    getEdgesDataBounds() {
      return this._computeBounds();
    }

    getDebugInfo() {
      const programs = {};
      if (this._programInfo) {
        Object.entries(this._programInfo).forEach(([key, info]) => {
          if (!info) return;
          programs[key] = { linked: !!info.linked, log: info.log || "" };
        });
      }
      const shaders = {};
      if (this._shaderInfo) {
        Object.entries(this._shaderInfo).forEach(([key, info]) => {
          if (!info || info.compiled) return;
          shaders[key] = { compiled: false, log: info.log || "" };
        });
      }
      return {
        renderer: this.renderer,
        webgl2: !!this._isWebGL2,
        instancing: !!(this._isWebGL2 || this._instancedExt),
        derivatives: !!this._hasDerivatives,
        pixelRatio: this._pixelRatio,
        scale: this._scale,
        translate: { x: this._translate.x, y: this._translate.y },
        counts: {
          nodes: this._nodeCount || this._nodes.length,
          edges: this._edges.length,
          edgeSegments: this._edgeSegmentsCount || 0,
          arrows: this._arrowCount || 0,
          edgeVisible: this._edgeVisibleCount || 0,
        },
        cull: {
          enabled: !!this._edgeCullEnabled,
          active: !!this._edgeCullActive,
          viewDirty: !!this._edgeViewDirty,
          threshold: this._edgeCullThreshold,
        },
        buffers: {
          node: this._nodeData ? this._nodeData.length : 0,
          edge: this._edgeData ? this._edgeData.length : 0,
          arrow: this._arrowData ? this._arrowData.length : 0,
        },
        worker: {
          supported: !!this._edgeWorkerSupported,
          busy: !!this._edgeWorkerBusy,
          threshold: this._edgeWorkerThreshold,
        },
        text: {
          gpu: !!this._useGpuText,
          active: !!this._shouldUseGpuText(),
          mode: this._textMode || "",
          quality: this._textQuality || "",
          backend: this._textBackend || "text-atlas",
          policyLocked: !!this._textPolicyLocked,
          textFallbackAllowed: !!this._textFallbackAllowed,
          labels: Number(this._labelCountEstimate) || 0,
          sets: this._textAtlases ? this._textAtlases.size : 0,
          atlases: this.getTextAtlasStats().atlasCount,
          glyphs: this.getTextAtlasStats().glyphCount,
          textureBytes: this.getTextAtlasStats().approxTextureBytes,
        },
        repair: {
          count: this._renderRepairCount || 0,
          reason: this._renderRepairReason || "",
          at: this._renderRepairAt || 0,
        },
        edgeQuality: {
          aaV2: !!EDGE_AA_V2_ENABLED,
          minScreenWidthPx: EDGE_MIN_SCREEN_WIDTH_PX,
          dashLodPeriodPx: EDGE_DASH_LOD_PERIOD_PX,
          dashLodBlendPx: EDGE_DASH_LOD_BLEND_PX,
          aaMinPx: EDGE_AA_MIN_PX,
          derivatives: !!this._hasDerivatives,
        },
        visualFade: this._computeZoomVisualStyle(),
        programs,
        shaders,
      };
    }

    // --- internals ---
    _resolveCanvasClearColor() {
      const fallback = WEBGL_CLEAR_FALLBACK;
      if (!this.container || typeof window === "undefined" || typeof window.getComputedStyle !== "function") {
        return fallback.slice();
      }
      try {
        const style = window.getComputedStyle(this.container);
        const bg = style?.backgroundColor || "";
        return toOpaqueColor(bg, fallback);
      } catch (e) {
        return fallback.slice();
      }
    }

    _clearGeometryGpuBuffers() {
      const gl = this.gl;
      if (!gl) return;
      try {
        const empty = new Float32Array(0);
        const clearBuf = (buf) => {
          if (!buf) return;
          gl.bindBuffer(gl.ARRAY_BUFFER, buf);
          gl.bufferData(gl.ARRAY_BUFFER, empty, gl.DYNAMIC_DRAW);
        };
        clearBuf(this._edgeBuffer);
        clearBuf(this._arrowBuffer);
        clearBuf(this._edgeVisibleBuffer);
        clearBuf(this._arrowVisibleBuffer);
        clearBuf(this._highlightEdgeBuffer);
        clearBuf(this._nodeBuffer);
        clearBuf(this._shadowBuffer);
      } catch (e) {}
    }

    _initCanvases() {
      if (!this.container) return;
      const clearColor = this._resolveCanvasClearColor();
      this._clearColor = clearColor;
      const canvas = createGraphCanvas(this.container, clearColor);
      this.canvas = canvas;
      this._resize();
    }

    _initGL() {
      return initGraphRendererGl(this, {
        createGraphGlContext,
        WEBGL_CLEAR_FALLBACK,
        EDGE_AA_V2_ENABLED,
        EDGE_MIN_SCREEN_WIDTH_PX,
        EDGE_DASH_LOD_PERIOD_PX,
        EDGE_DASH_LOD_BLEND_PX,
        EDGE_AA_MIN_PX,
        NODE_AA_PAD_PX,
      });
      if (!this.canvas) return;
      const clearColor = this._clearColor || WEBGL_CLEAR_FALLBACK;
      const bootstrap = createGraphGlContext(this.canvas, clearColor);
      const gl = bootstrap.gl;
      this.gl = gl;
      this._isWebGL2 = !!bootstrap.isWebGL2;
      this._instancedExt = bootstrap.instancedExt || null;
      if (!gl) {
        this.renderer = "webgl";
        this._useGpuText = true;
        this._textMode = "gpu";
        this._useGpuPick = false;
        throw new Error("WebGL context unavailable");
      }
      this.renderer = "webgl";
      this._fragPrecision = bootstrap.fragPrecision || "mediump";
      const extDerivatives = gl.getExtension("OES_standard_derivatives");
      let useDerivatives = !!extDerivatives || this._isWebGL2;
      const buildPrograms = (withDerivatives) => {
        const derivativePrefix =
          !this._isWebGL2 && withDerivatives ? "#extension GL_OES_standard_derivatives : enable\n" : "";
        const fragPrecision = this._fragPrecision || "mediump";
        this._programInfo = {};
        this._shaderInfo = {};

        const edgeVertV1 =
          `attribute vec2 a_unit;\nattribute vec2 a_start;\nattribute vec2 a_end;\nattribute float a_width;\nattribute vec4 a_color;\nattribute float a_dashType;\nattribute float a_dashSize;\nattribute float a_dashGap;\nuniform vec2 u_resolution;\nuniform float u_scale;\nuniform vec2 u_translate;\nvarying vec4 v_color;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvarying float v_t;\nvarying float v_edgeY;\nvoid main(){\n  vec2 dir = a_end - a_start;\n  float len = length(dir);\n  if(len < 0.0001){ dir = vec2(1.0,0.0); len = 1.0; }\n  vec2 dirN = dir / len;\n  vec2 perp = vec2(-dirN.y, dirN.x);\n  float safeScale = max(abs(u_scale), 0.0001);\n  float halfWidthPx = max(0.5 * a_width * safeScale, 0.0001);\n  float aaPad = clamp(1.05 / halfWidthPx, 0.0, 1.35);\n  float edgeExtent = 1.0 + aaPad;\n  vec2 world = a_start + dirN * (a_unit.x * len) + perp * (a_unit.y * edgeExtent * 0.5 * a_width);\n  vec2 screen = world * u_scale + u_translate;\n  vec2 clip = screen / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clip.x, -clip.y, 0.0, 1.0);\n  v_color = a_color;\n  v_dashType = a_dashType;\n  v_dashSize = a_dashSize;\n  v_dashGap = a_dashGap;\n  v_t = a_unit.x * len;\n  v_edgeY = a_unit.y * edgeExtent;\n}\n`;
        const edgeFragV1 = withDerivatives
          ? `${derivativePrefix}precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec4 v_color;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvarying float v_t;\nvarying float v_edgeY;\nvoid main(){\n  if(v_color.a <= 0.0) discard;\n  if(v_dashType > 0.5){\n    float period = v_dashSize + v_dashGap;\n    float pos = mod(v_t, period);\n    if(pos > v_dashSize) discard;\n  }\n  float edge = abs(v_edgeY);\n  if(edge > 1.0) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, v_color.a);\n}\n`
          : `precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec4 v_color;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvarying float v_t;\nvarying float v_edgeY;\nvoid main(){\n  if(v_color.a <= 0.0) discard;\n  if(v_dashType > 0.5){\n    float period = v_dashSize + v_dashGap;\n    float pos = mod(v_t, period);\n    if(pos > v_dashSize) discard;\n  }\n  float edge = abs(v_edgeY);\n  if(edge > 1.0) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, v_color.a);\n}\n`;
        const edgeVertV2 =
          `attribute vec2 a_unit;\nattribute vec2 a_start;\nattribute vec2 a_end;\nattribute float a_width;\nattribute vec4 a_color;\nattribute float a_dashType;\nattribute float a_dashSize;\nattribute float a_dashGap;\nuniform vec2 u_resolution;\nuniform float u_scale;\nuniform vec2 u_translate;\nvarying vec4 v_color;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvarying float v_t;\nvarying float v_edgeY;\nvarying float v_halfWidthPx;\nvarying float v_scaleAbs;\nvoid main(){\n  vec2 dir = a_end - a_start;\n  float len = length(dir);\n  if(len < 0.0001){ dir = vec2(1.0,0.0); len = 1.0; }\n  vec2 dirN = dir / len;\n  vec2 perp = vec2(-dirN.y, dirN.x);\n  float safeScale = max(abs(u_scale), 0.0001);\n  float rawHalfWidthPx = max(0.5 * a_width * safeScale, 0.0001);\n  float clampedHalfWidthPx = max(rawHalfWidthPx, ${EDGE_MIN_SCREEN_WIDTH_PX.toFixed(4)} * 0.5);\n  float aaPad = clamp(${EDGE_AA_MIN_PX.toFixed(4)} / clampedHalfWidthPx, 0.0, 1.35);\n  float edgeExtent = 1.0 + aaPad;\n  float halfWorld = clampedHalfWidthPx / safeScale;\n  vec2 world = a_start + dirN * (a_unit.x * len) + perp * (a_unit.y * edgeExtent * halfWorld);\n  vec2 screen = world * u_scale + u_translate;\n  vec2 clip = screen / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clip.x, -clip.y, 0.0, 1.0);\n  v_color = a_color;\n  v_dashType = a_dashType;\n  v_dashSize = a_dashSize;\n  v_dashGap = a_dashGap;\n  v_t = a_unit.x * len;\n  v_edgeY = a_unit.y * edgeExtent;\n  v_halfWidthPx = clampedHalfWidthPx;\n  v_scaleAbs = safeScale;\n}\n`;
        const edgeFragV2 = withDerivatives
          ? `${derivativePrefix}precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec4 v_color;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvarying float v_t;\nvarying float v_edgeY;\nvarying float v_halfWidthPx;\nvarying float v_scaleAbs;\nvoid main(){\n  if(v_color.a <= 0.0) discard;\n  float dashMaskRaw = 1.0;\n  float period = max(v_dashSize + v_dashGap, 0.0001);\n  if(v_dashType > 0.5){\n    float pos = mod(max(v_t, 0.0), period);\n    dashMaskRaw = step(pos, v_dashSize);\n  }\n  float periodPx = period * v_scaleAbs;\n  float lod = smoothstep(${(EDGE_DASH_LOD_PERIOD_PX - EDGE_DASH_LOD_BLEND_PX).toFixed(4)}, ${(EDGE_DASH_LOD_PERIOD_PX + EDGE_DASH_LOD_BLEND_PX).toFixed(4)}, periodPx);\n  float dashMask = mix(1.0, dashMaskRaw, lod);\n  float edge = abs(v_edgeY);\n  float aa = max(fwidth(v_edgeY), ${EDGE_AA_MIN_PX.toFixed(4)} / max(v_halfWidthPx, 0.0001));\n  float edgeMask = 1.0 - smoothstep(1.0 - aa, 1.0 + aa, edge);\n  float alpha = v_color.a * edgeMask * dashMask;\n  if(alpha <= 0.001) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, alpha);\n}\n`
          : `precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec4 v_color;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvarying float v_t;\nvarying float v_edgeY;\nvarying float v_halfWidthPx;\nvarying float v_scaleAbs;\nvoid main(){\n  if(v_color.a <= 0.0) discard;\n  float dashMaskRaw = 1.0;\n  float period = max(v_dashSize + v_dashGap, 0.0001);\n  if(v_dashType > 0.5){\n    float pos = mod(max(v_t, 0.0), period);\n    dashMaskRaw = step(pos, v_dashSize);\n  }\n  float periodPx = period * v_scaleAbs;\n  float lod = smoothstep(${(EDGE_DASH_LOD_PERIOD_PX - EDGE_DASH_LOD_BLEND_PX).toFixed(4)}, ${(EDGE_DASH_LOD_PERIOD_PX + EDGE_DASH_LOD_BLEND_PX).toFixed(4)}, periodPx);\n  float dashMask = mix(1.0, dashMaskRaw, lod);\n  float edge = abs(v_edgeY);\n  float aa = clamp(${EDGE_AA_MIN_PX.toFixed(4)} / max(v_halfWidthPx, 0.0001), 0.02, 0.7);\n  float edgeMask = 1.0 - smoothstep(1.0 - aa, 1.0 + aa, edge);\n  float alpha = v_color.a * edgeMask * dashMask;\n  if(alpha <= 0.001) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, alpha);\n}\n`;
        const edgeVert = EDGE_AA_V2_ENABLED ? edgeVertV2 : edgeVertV1;
        const edgeFrag = EDGE_AA_V2_ENABLED ? edgeFragV2 : edgeFragV1;

        this._edgeProgram = this._createProgram(edgeVert, edgeFrag, "edge");

        const nodeFrag = withDerivatives
          ? `${derivativePrefix}precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec2 v_unit;\nvarying float v_radius;\nvarying float v_lineWidth;\nvarying vec4 v_stroke;\nvarying vec4 v_fill;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvoid main(){\n  float dist = length(v_unit);\n  float inner = 1.0 - (v_lineWidth / max(v_radius, 0.0001));\n  inner = clamp(inner, 0.0, 1.0);\n  float aa = max(fwidth(dist), 0.01);\n  float outerMask = 1.0 - smoothstep(1.0 - aa, 1.0 + aa, dist);\n  float innerMask = 1.0 - smoothstep(inner - aa, inner + aa, dist);\n  float strokeMask = clamp(outerMask - innerMask, 0.0, 1.0);\n  float fillMask = innerMask;\n  if(v_dashType > 0.5){\n    float angle = atan(v_unit.y, v_unit.x) + 3.14159265;\n    float pos = mod(angle * max(v_radius, 0.0001), max(v_dashSize + v_dashGap, 0.0001));\n    strokeMask *= step(pos, v_dashSize);\n  }\n  vec3 rgbBase = v_fill.rgb * fillMask + v_stroke.rgb * strokeMask;\n  float alpha = v_fill.a * fillMask + v_stroke.a * strokeMask;\n  if(alpha <= 0.01) discard;\n  float contrast = max(u_contrast, 0.0);\n  vec3 rgb = clamp(u_clearColor + (rgbBase - u_clearColor) * contrast, 0.0, 1.0);\n  gl_FragColor = vec4(rgb, alpha);\n}\n`
          : `precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec2 v_unit;\nvarying float v_radius;\nvarying float v_lineWidth;\nvarying vec4 v_stroke;\nvarying vec4 v_fill;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvoid main(){\n  float dist = length(v_unit);\n  float inner = 1.0 - (v_lineWidth / max(v_radius, 0.0001));\n  inner = clamp(inner, 0.0, 1.0);\n  float outerMask = 1.0 - smoothstep(0.97, 1.03, dist);\n  float innerMask = 1.0 - smoothstep(inner - 0.03, inner + 0.03, dist);\n  float strokeMask = clamp(outerMask - innerMask, 0.0, 1.0);\n  float fillMask = innerMask;\n  if(v_dashType > 0.5){\n    float angle = atan(v_unit.y, v_unit.x) + 3.14159265;\n    float pos = mod(angle * max(v_radius, 0.0001), max(v_dashSize + v_dashGap, 0.0001));\n    strokeMask *= step(pos, v_dashSize);\n  }\n  vec3 rgbBase = v_fill.rgb * fillMask + v_stroke.rgb * strokeMask;\n  float alpha = v_fill.a * fillMask + v_stroke.a * strokeMask;\n  if(alpha <= 0.01) discard;\n  float contrast = max(u_contrast, 0.0);\n  vec3 rgb = clamp(u_clearColor + (rgbBase - u_clearColor) * contrast, 0.0, 1.0);\n  gl_FragColor = vec4(rgb, alpha);\n}\n`;

        this._nodeProgram = this._createProgram(
        `attribute vec2 a_unit;\nattribute vec2 a_center;\nattribute float a_radius;\nattribute float a_lineWidth;\nattribute vec4 a_stroke;\nattribute vec4 a_fill;\nattribute float a_dashType;\nattribute float a_dashSize;\nattribute float a_dashGap;\nuniform vec2 u_resolution;\nuniform float u_scale;\nuniform vec2 u_translate;\nvarying vec2 v_unit;\nvarying float v_radius;\nvarying float v_lineWidth;\nvarying vec4 v_stroke;\nvarying vec4 v_fill;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvoid main(){\n  vec2 world = a_center + a_unit * a_radius;\n  vec2 screen = world * u_scale + u_translate;\n  vec2 clip = screen / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clip.x, -clip.y, 0.0, 1.0);\n  v_unit = a_unit;\n  v_radius = a_radius;\n  v_lineWidth = a_lineWidth;\n  v_stroke = a_stroke;\n  v_fill = a_fill;\n  v_dashType = a_dashType;\n  v_dashSize = a_dashSize;\n  v_dashGap = a_dashGap;\n}\n`,
        nodeFrag,
        "node"
      );

        const arrowFrag = withDerivatives
          ? `${derivativePrefix}precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec4 v_color;\nvarying vec2 v_unit;\nvoid main(){\n  if(v_color.a <= 0.0) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, v_color.a);\n}\n`
          : `precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec4 v_color;\nvarying vec2 v_unit;\nvoid main(){\n  if(v_color.a <= 0.0) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, v_color.a);\n}\n`;

        this._arrowProgram = this._createProgram(
        `attribute vec2 a_unit;\nattribute vec2 a_pos;\nattribute vec2 a_dir;\nattribute float a_size;\nattribute vec4 a_color;\nuniform vec2 u_resolution;\nuniform float u_scale;\nuniform vec2 u_translate;\nvarying vec4 v_color;\nvarying vec2 v_unit;\nvoid main(){\n  vec2 perp = vec2(-a_dir.y, a_dir.x);\n  vec2 world = a_pos + a_dir * (a_unit.x * a_size) + perp * (a_unit.y * a_size);\n  vec2 screen = world * u_scale + u_translate;\n  vec2 clip = screen / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clip.x, -clip.y, 0.0, 1.0);\n  v_color = a_color;\n  v_unit = a_unit;\n}\n`,
        arrowFrag,
        "arrow"
      );

        const textFrag = withDerivatives
          ? `${derivativePrefix}precision ${fragPrecision} float;\nuniform sampler2D u_tex;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nuniform float u_alphaScale;\nvarying vec2 v_uv;\nvarying vec4 v_color;\nvoid main(){\n  float a = texture2D(u_tex, v_uv).a;\n  float outAlpha = v_color.a * a * clamp(u_alphaScale, 0.0, 1.0);\n  if(outAlpha <= 0.01) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, outAlpha);\n}\n`
          : `precision ${fragPrecision} float;\nuniform sampler2D u_tex;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nuniform float u_alphaScale;\nvarying vec2 v_uv;\nvarying vec4 v_color;\nvoid main(){\n  float a = texture2D(u_tex, v_uv).a;\n  float outAlpha = v_color.a * a * clamp(u_alphaScale, 0.0, 1.0);\n  if(outAlpha <= 0.01) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, outAlpha);\n}\n`;
        this._textProgram = this._createProgram(
        `attribute vec2 a_unit;\nattribute vec2 a_pos;\nattribute vec2 a_size;\nattribute vec4 a_uv;\nattribute vec4 a_color;\nuniform vec2 u_resolution;\nuniform float u_scale;\nuniform vec2 u_translate;\nvarying vec2 v_uv;\nvarying vec4 v_color;\nvoid main(){\n  vec2 world = a_pos + a_unit * a_size;\n  vec2 screen = world * u_scale + u_translate;\n  vec2 clip = screen / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clip.x, -clip.y, 0.0, 1.0);\n  vec2 unit = a_unit + vec2(0.5, 0.5);\n  v_uv = mix(a_uv.xy, a_uv.zw, unit);\n  v_color = a_color;\n}\n`,
        textFrag,
        "text"
      );

        this._pickNodeProgram = this._createProgram(
        `attribute vec2 a_unit;\nattribute vec2 a_center;\nattribute float a_radius;\nattribute vec4 a_pick;\nuniform vec2 u_resolution;\nuniform float u_scale;\nuniform vec2 u_translate;\nvarying vec2 v_unit;\nvarying vec4 v_pick;\nvoid main(){\n  vec2 world = a_center + a_unit * a_radius;\n  vec2 screen = world * u_scale + u_translate;\n  vec2 clip = screen / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clip.x, -clip.y, 0.0, 1.0);\n  v_unit = a_unit;\n  v_pick = a_pick;\n}\n`,
        `precision mediump float;\nvarying vec2 v_unit;\nvarying vec4 v_pick;\nvoid main(){\n  float dist = length(v_unit);\n  if(dist > 1.0) discard;\n  gl_FragColor = v_pick;\n}\n`,
        "pickNode"
      );

        this._pickEdgeProgram = this._createProgram(
        `attribute vec2 a_unit;\nattribute vec2 a_start;\nattribute vec2 a_end;\nattribute float a_width;\nattribute vec4 a_pick;\nuniform vec2 u_resolution;\nuniform float u_scale;\nuniform vec2 u_translate;\nvarying vec4 v_pick;\nvoid main(){\n  vec2 dir = a_end - a_start;\n  float len = length(dir);\n  if(len < 0.0001){ dir = vec2(1.0,0.0); len = 1.0; }\n  vec2 dirN = dir / len;\n  vec2 perp = vec2(-dirN.y, dirN.x);\n  vec2 world = a_start + dirN * (a_unit.x * len) + perp * (a_unit.y * 0.5 * a_width);\n  vec2 screen = world * u_scale + u_translate;\n  vec2 clip = screen / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clip.x, -clip.y, 0.0, 1.0);\n  v_pick = a_pick;\n}\n`,
        `precision mediump float;\nvarying vec4 v_pick;\nvoid main(){\n  gl_FragColor = v_pick;\n}\n`,
        "pickEdge"
      );
      };

      buildPrograms(useDerivatives);
      if (useDerivatives) {
        const failed = Object.values(this._shaderInfo || {}).some(
          (info) => !info.compiled && /fwidth|deriv/i.test(info.log || "")
        );
        if (failed) {
          this._emitWebglWarn("derivatives-fallback", "WebGL derivatives unavailable, fallback to basic shaders");
          useDerivatives = false;
          this._releaseGlPrograms();
          buildPrograms(false);
        }
      }
      this._hasDerivatives = useDerivatives;
      if (this._programInfo?.text && this._programInfo.text.linked === false) {
        this._useGpuText = false;
        this._textMode = "gpu";
      }

      this._initBuffers();
    }

    _createProgram(vsSource, fsSource, label = "") {
      return createGraphProgram(this, vsSource, fsSource, label);
    }

    _compileShader(type, source, label = "") {
      return compileGraphShader(this, type, source, label);
    }

    _emitWebglWarn(key, ..._args) {
      if (!this._webglWarn) return;
      const dedupeKey = String(key || "");
      if (dedupeKey) {
        if (this._webglWarnOnce.has(dedupeKey)) return;
        this._webglWarnOnce.add(dedupeKey);
      }
      try {
        console.warn("[Viz][WebGL] event=webgl_warning");
      } catch (e) {}
    }

    _initBuffers() {
      const gl = this.gl;
      if (!gl) return;
      this._edgeBuffer = gl.createBuffer();
      this._edgeUnitBuffer = gl.createBuffer();
      this._nodeBuffer = gl.createBuffer();
      this._nodeUnitBuffer = gl.createBuffer();
      this._shadowBuffer = gl.createBuffer();
      this._arrowBuffer = gl.createBuffer();
      this._arrowUnitBuffer = gl.createBuffer();
      this._textUnitBuffer = gl.createBuffer();
      this._pickNodeBuffer = gl.createBuffer();
      this._pickEdgeBuffer = gl.createBuffer();

      // Edge quad units
      gl.bindBuffer(gl.ARRAY_BUFFER, this._edgeUnitBuffer);
      gl.bufferData(
        gl.ARRAY_BUFFER,
        new Float32Array([0, -1, 1, -1, 0, 1, 1, 1]),
        gl.STATIC_DRAW
      );

      // Node quad units
      gl.bindBuffer(gl.ARRAY_BUFFER, this._nodeUnitBuffer);
      gl.bufferData(
        gl.ARRAY_BUFFER,
        new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]),
        gl.STATIC_DRAW
      );

      // Arrow triangle units
      gl.bindBuffer(gl.ARRAY_BUFFER, this._arrowUnitBuffer);
      gl.bufferData(
        gl.ARRAY_BUFFER,
        new Float32Array([0, 0, -1, 0.6, -1, -0.6]),
        gl.STATIC_DRAW
      );

      // Text quad units
      gl.bindBuffer(gl.ARRAY_BUFFER, this._textUnitBuffer);
      gl.bufferData(
        gl.ARRAY_BUFFER,
        new Float32Array([-0.5, -0.5, 0.5, -0.5, -0.5, 0.5, 0.5, 0.5]),
        gl.STATIC_DRAW
      );
    }

    _resize() {
      if (!this.canvas) return;
      const clearColor = this._resolveCanvasClearColor();
      this._clearColor = clearColor;
      this.canvas.style.backgroundColor = `rgb(${Math.round(clearColor[0] * 255)}, ${Math.round(clearColor[1] * 255)}, ${Math.round(
        clearColor[2] * 255
      )})`;
      const dpr = this._getEffectivePixelRatio();
      this._pixelRatio = dpr;
      const w = this.width;
      const h = this.height;
      this.canvas.width = Math.max(1, Math.round(w * dpr));
      this.canvas.height = Math.max(1, Math.round(h * dpr));
      if (this.gl) {
        this.gl.viewport(0, 0, this.canvas.width, this.canvas.height);
        this.gl.clearColor(clearColor[0], clearColor[1], clearColor[2], 1);
        this.gl.clear(this.gl.COLOR_BUFFER_BIT);
      }
      this._pickRenderDirty = true;
    }

    _getEffectivePixelRatio() {
      const dpr = window.devicePixelRatio || 1;
      const zoom = Math.max(Math.abs(this._scale || 1), 0.0001);
      if (this._highDpi) {
        return Math.max(1, Math.min(dpr, this._highDpiMax));
      }
      // Keep zoomed-in nodes sharp even on dense graphs; users are inspecting detail here.
      if (zoom >= 1.75) {
        return Math.min(dpr, 2);
      }
      const nodeCount = this._nodes?.length || 0;
      const edgeCount = this._edges?.length || 0;
      if (nodeCount > 3000 || edgeCount > 6000) return Math.min(dpr, 1);
      if (nodeCount > 1500 || edgeCount > 3000) return Math.min(dpr, 1.25);
      if (nodeCount > 800 || edgeCount > 1600) return Math.min(dpr, 1.5);
      return Math.min(dpr, 2);
    }

    _syncPixelRatio() {
      const desired = this._getEffectivePixelRatio();
      if (Math.abs(desired - this._pixelRatio) < 0.01) return;
      this._pixelRatio = desired;
      this._resize();
      this._labelsDirty = true;
      this._pickRenderDirty = true;
    }

    _getWorldViewBounds(marginPx = 80) {
      const scale = Math.max(this._scale || 1, 0.0001);
      const minX = (-this._translate.x - marginPx) / scale;
      const maxX = (this.width - this._translate.x + marginPx) / scale;
      const minY = (-this._translate.y - marginPx) / scale;
      const maxY = (this.height - this._translate.y + marginPx) / scale;
      return { minX, maxX, minY, maxY };
    }

    _shouldDeferEdgeUpdate() {
      if (!this._deferEdgeUpdate) return false;
      if (!this._interactionActive) return false;
      const edgeCount = this._edges?.length || 0;
      return edgeCount > this._deferEdgeUpdateThreshold;
    }

    _updateEdgeCullActive() {
      const edgeCount = this._edges?.length || 0;
      this._edgeCullActive = !!this._edgeCullEnabled && edgeCount > this._edgeCullThreshold;
      return this._edgeCullActive;
    }

    _markEdgeViewDirty() {
      this._edgeViewDirty = true;
    }

    _applyModes(modes) {
      const list = modes?.default || [];
      const has = (name) =>
        list.some((m) => {
          if (typeof m === "string") return m === name;
          return m?.type === name;
        });
      this._enableDragCanvas = has("drag-canvas");
      this._enableDragNode = has("drag-node");
      this._enableZoomCanvas = has("zoom-canvas");
    }

    _reindexRuntimeItems() {
      const nodeCount = this._nodes.length;
      this._nodes.forEach((node, idx) => {
        if (!node) return;
        node._index = idx;
        node._pickId = idx + 1;
        const model = node.getModel?.();
        if (model) model.__nodeIndex = idx;
      });
      this._edges.forEach((edge, idx) => {
        if (!edge) return;
        edge._index = idx;
        edge._pickId = nodeCount + idx + 1;
        const model = edge.getModel?.();
        if (model) model.__edgeIndex = idx;
      });
    }

    _buildItems() {
      this._nodes = [];
      this._edges = [];
      this._nodeMap.clear();
      this._edgeMap.clear();
      this._haloNodes.clear();
      this._highlightEdges.clear();
      this._highlightDirty = true;
      this._edgeViewDirty = true;
      const nodeList = Array.isArray(this._data?.nodes) ? this._data.nodes : [];
      const edgeList = Array.isArray(this._data?.edges) ? this._data.edges : [];
      nodeList.forEach((m, idx) => {
        if (!m || m.id == null) return;
        if (!Number.isFinite(m.x)) m.x = 0;
        if (!Number.isFinite(m.y)) m.y = 0;
        const item = new NodeItem(m);
        item._index = idx;
        m.__nodeIndex = idx;
        this._nodes.push(item);
        this._nodeMap.set(m.id, item);
      });
      this._shadowNodes.clear();
      this._nodes.forEach((node) => {
        const m = node.getModel();
        if (m?.nodeShadow) this._shadowNodes.add(node);
      });
      this._shadowNodesDirty = false;
      edgeList.forEach((m, idx) => {
        if (!m || m.id == null) return;
        const source = this._nodeMap.get(m.source);
        const target = this._nodeMap.get(m.target);
        if (!source || !target) return;
        const item = new EdgeItem(m, source, target);
        item._index = idx;
        m.__edgeIndex = idx;
        this._edges.push(item);
        this._edgeMap.set(m.id, item);
        source.addEdge(item);
        target.addEdge(item);
      });
      this._reindexRuntimeItems();
      this._rebuildPickGrid();
      this._pickGridDirty = false;
      this._edgeGridDirty = true;
      this._pickDataDirty = true;
      this._pickRenderDirty = true;
      this._labelCountEstimate = this._estimateLabelCount();
    }

    _rebuildPickGrid() {
      this._pickGrid.clear();
      const size = this._pickGridSize;
      this._nodes.forEach((node) => {
        const m = node.getModel();
        const x = Number(m.x) || 0;
        const y = Number(m.y) || 0;
        const gx = Math.floor(x / size);
        const gy = Math.floor(y / size);
        const key = `${gx},${gy}`;
        if (!this._pickGrid.has(key)) this._pickGrid.set(key, []);
        this._pickGrid.get(key).push(node);
        node._gridKey = key;
      });
    }

    _updatePickGridForNode(node) {
      if (!node) return;
      const size = this._pickGridSize;
      const m = node.getModel();
      const x = Number(m.x) || 0;
      const y = Number(m.y) || 0;
      const gx = Math.floor(x / size);
      const gy = Math.floor(y / size);
      const key = `${gx},${gy}`;
      if (node._gridKey === key) return;
      const prevKey = node._gridKey;
      if (prevKey && this._pickGrid.has(prevKey)) {
        const bucket = this._pickGrid.get(prevKey);
        const idx = bucket.indexOf(node);
        if (idx >= 0) bucket.splice(idx, 1);
        if (bucket.length === 0) this._pickGrid.delete(prevKey);
      }
      if (!this._pickGrid.has(key)) this._pickGrid.set(key, []);
      this._pickGrid.get(key).push(node);
      node._gridKey = key;
    }

    _clearEdgeGrid() {
      this._edgeGrid.clear();
      this._edges.forEach((edge) => {
        edge._edgeGridKeys = [];
      });
    }

    _edgeBounds(edge) {
      const m = edge.getModel();
      const s = edge.getSource().getModel();
      const t = edge.getTarget().getModel();
      const sx = Number(s.x) || 0;
      const sy = Number(s.y) || 0;
      const tx = Number(t.x) || 0;
      const ty = Number(t.y) || 0;
      const lineWidth = Number(m.lineWidth) || 2;
      const arrowSize = Number(m.arrowSize) || Math.max(6, lineWidth * 3);
      const arrowPad = Math.max(6, arrowSize * 1.1);
      if (m.source === m.target) {
        const r = Number(s.r) || 18;
        const outPad = Number(m.outPad ?? 1.2);
        const loopScale = Number(m.loopScale ?? 2);
        const fontSize = clamp(Number(m.fontSize ?? 12), 8, 36);
        const label = String(m.label || "");
        const lw = Math.max(24, estimateTextWidth(label, fontSize));
        const stroke = Number(s.lineWidth ?? 2) || 2;
        const sPad = Math.max(0.9, stroke / 2 + 0.4);
        const baseR = r + outPad + sPad;
        const peak = Math.max(baseR * 0.75 * loopScale, fontSize * 1.2);
        const rx = Math.max(baseR * 2.8 * loopScale, baseR + lw * 1.2);
        const ctrlY = sy - peak * 1.15;
        const pad = Math.max(8, lineWidth * 2) + arrowPad;
        return {
          minX: sx - rx - pad,
          maxX: sx + rx + pad,
          minY: ctrlY - pad,
          maxY: sy + pad,
        };
      }
      const isDouble = String(m.mode || "").toLowerCase() === "double";
      const offset = isDouble ? resolveDoubleGap(m, 1) / 2 : 0;
      const pad = Math.max(6, lineWidth * 2) + offset + arrowPad;
      return {
        minX: Math.min(sx, tx) - pad,
        maxX: Math.max(sx, tx) + pad,
        minY: Math.min(sy, ty) - pad,
        maxY: Math.max(sy, ty) + pad,
      };
    }

    _edgeGridAdd(edge, bounds) {
      const size = this._edgeGridSize;
      const minX = bounds.minX;
      const maxX = bounds.maxX;
      const minY = bounds.minY;
      const maxY = bounds.maxY;
      const gx0 = Math.floor(minX / size);
      const gx1 = Math.floor(maxX / size);
      const gy0 = Math.floor(minY / size);
      const gy1 = Math.floor(maxY / size);
      edge._edgeGridKeys = [];
      for (let gx = gx0; gx <= gx1; gx += 1) {
        for (let gy = gy0; gy <= gy1; gy += 1) {
          const key = `${gx},${gy}`;
          if (!this._edgeGrid.has(key)) this._edgeGrid.set(key, []);
          this._edgeGrid.get(key).push(edge);
          edge._edgeGridKeys.push(key);
        }
      }
    }

    _edgeGridRemove(edge) {
      const keys = edge._edgeGridKeys || [];
      keys.forEach((key) => {
        const bucket = this._edgeGrid.get(key);
        if (!bucket) return;
        const idx = bucket.indexOf(edge);
        if (idx >= 0) bucket.splice(idx, 1);
        if (bucket.length === 0) this._edgeGrid.delete(key);
      });
      edge._edgeGridKeys = [];
    }

    _updateEdgeGridForEdge(edge) {
      if (!edge) return;
      this._edgeGridRemove(edge);
      this._edgeGridAdd(edge, this._edgeBounds(edge));
    }

    _rebuildEdgeGrid() {
      this._clearEdgeGrid();
      this._edges.forEach((edge) => {
        this._edgeGridAdd(edge, this._edgeBounds(edge));
      });
      this._edgeGridDirty = false;
    }

    _ensureEdgeWorker() {
      if (!this._edgeWorkerSupported || this._edgeWorker) return;
      try {
        this._edgeWorker = createEdgeWorker(
          (ev) => this._applyEdgeWorkerResult(ev?.data || {}),
          () => {
            this._edgeWorkerSupported = false;
            this._edgeWorkerBusy = false;
            this._edgeWorker = null;
          }
        );
      } catch (e) {
        this._edgeWorkerSupported = false;
        this._edgeWorker = null;
      }
    }

    _scheduleEdgeWorker() {
      if (!this._edgeWorkerSupported) return false;
      if (this._edgeWorkerBusy) {
        this._edgeWorkerPending = true;
        return false;
      }
      this._ensureEdgeWorker();
      if (!this._edgeWorker) return false;
      const edges = this._edges || [];
      const sizeScale = this._getSizeScale();
      const edgePayload = edges.map((edge) => {
        const m = edge.getModel();
        const s = edge.getSource().getModel();
        const t = edge.getTarget().getModel();
        const baseStroke = parseColor(m.stroke || "rgba(2,6,23,.75)");
        let stroke = baseStroke;
        let lineWidth = (Number(m.lineWidth) || 2) * sizeScale;
        if (edge.hasState("selected") || edge.hasState("active")) {
          stroke = mixColor(baseStroke, parseColor(HIGHLIGHT_COLOR), 0.6);
          lineWidth += 2 * sizeScale;
        } else if (edge.hasState("hover")) {
          stroke = mixColor(baseStroke, parseColor(HIGHLIGHT_COLOR), 0.35);
          lineWidth += 1 * sizeScale;
        }
        const abstractAlpha = this._computeEdgeAbstractionAlpha(edge, m);
        if (abstractAlpha < 0.999) {
          stroke = (Array.isArray(stroke) ? stroke : parseColor(stroke)).slice();
          stroke[3] = clamp((Number(stroke[3]) || 1) * abstractAlpha, 0.015, 1);
        }
        const dash = resolveDash(m.edgeDash || m.edgeDashStyle, lineWidth);
        const sPad = (() => {
          const stroke = Number(s.lineWidth ?? 2) || 2;
          const base = Math.max(0.9, stroke / 2 + 0.4);
          const selected = edge.getSource()?.hasState?.("selected") || edge.getSource()?.hasState?.("active");
          const haloPad = selected ? 1.8 : 0;
          return (base + haloPad) * sizeScale;
        })();
        const tPad = (() => {
          const stroke = Number(t.lineWidth ?? 2) || 2;
          const base = Math.max(0.9, stroke / 2 + 0.4);
          const selected = edge.getTarget()?.hasState?.("selected") || edge.getTarget()?.hasState?.("active");
          const haloPad = selected ? 1.8 : 0;
          return (base + haloPad) * sizeScale;
        })();
        const outPad = (Number(m.outPad ?? (m.source === m.target ? 1.2 : 0.8)) || 0) * sizeScale;
        const holePad = (Number(m.holePad ?? 2) || 0) * sizeScale;
        const loopScale = Number(m.loopScale ?? 2) || 2;
        const arrowSize = Math.max((Number(m.arrowSize) || 10) * sizeScale, lineWidth * 3);
        const baseFont = clamp(Number(m.fontSize ?? 13), 8, 36);
        const presentationLabels = resolveEdgeLabelRows(m);
        return {
          id: m.id,
          source: m.source,
          target: m.target,
          sx: Number(s.x) || 0,
          sy: Number(s.y) || 0,
          tx: Number(t.x) || 0,
          ty: Number(t.y) || 0,
          sr: (Number(s.r) || 18) * sizeScale,
          tr: (Number(t.r) || 18) * sizeScale,
          mode: m.mode,
          edgeArrow: m.edgeArrow,
          showArrow: m.showArrow,
          lineWidth,
          stroke,
          dash,
          edgeOffset: (Number(m.edgeOffset) || 0) * sizeScale,
          outPad,
          holePad,
          loopScale,
          arrowSize,
          sPad,
          tPad,
          gap: resolveDoubleGap(m, sizeScale),
          fontSize: baseFont,
          label: presentationLabels.label,
          labelTop: presentationLabels.labelTop,
          labelBottom: presentationLabels.labelBottom,
          labelOffsetY: resolveEdgeLabelYOffset(m, sizeScale),
          sizeScale,
        };
      });
      this._edgeWorkerBusy = true;
      this._edgeWorkerSeq += 1;
      const seq = this._edgeWorkerSeq;
      try {
        this._edgeWorker.postMessage({ seq, edges: edgePayload, lite: this._edgeLiteMode });
        return true;
      } catch (e) {
        this._edgeWorkerBusy = false;
        this._edgeWorkerSupported = false;
        return false;
      }
    }

    _applyEdgeWorkerResult(result) {
      if (!result) return;
      const seq = result.seq;
      if (!seq || seq !== this._edgeWorkerSeq) {
        this._edgeWorkerBusy = false;
        if (this._edgeWorkerPending) {
          this._edgeWorkerPending = false;
          this._scheduleEdgeWorker();
        }
        return;
      }
      const gl = this.gl;
      if (!gl) return;
      this._edgeWorkerBusy = false;
      const edgeData = result.edgeData instanceof Float32Array ? result.edgeData : null;
      const arrowData = result.arrowData instanceof Float32Array ? result.arrowData : null;
      if (!edgeData) return;
      this._edgeData = edgeData;
      this._arrowData = arrowData || new Float32Array(0);
      const segStarts = result.segStarts instanceof Int32Array ? result.segStarts : null;
      const segCounts = result.segCounts instanceof Int32Array ? result.segCounts : null;
      const arrowStarts = result.arrowStarts instanceof Int32Array ? result.arrowStarts : null;
      const arrowCounts = result.arrowCounts instanceof Int32Array ? result.arrowCounts : null;
      if (segStarts && segCounts) {
        this._edges.forEach((edge, idx) => {
          const m = edge.getModel();
          m.__segStart = segStarts[idx];
          m.__segCount = segCounts[idx];
          if (arrowStarts && arrowCounts) {
            m.__arrowStart = arrowStarts[idx];
            m.__arrowCount = arrowCounts[idx];
          }
        });
      }
      this._edgeSegmentsCount = Math.floor(edgeData.length / 12);
      this._arrowCount = Math.floor((this._arrowData?.length || 0) / 9);
      gl.bindBuffer(gl.ARRAY_BUFFER, this._edgeBuffer);
      gl.bufferData(gl.ARRAY_BUFFER, this._edgeData, gl.DYNAMIC_DRAW);
      gl.bindBuffer(gl.ARRAY_BUFFER, this._arrowBuffer);
      gl.bufferData(gl.ARRAY_BUFFER, this._arrowData, gl.DYNAMIC_DRAW);
      this._edgeGeomDirty = false;
      this._markEdgeViewDirty();
      this._highlightDirty = true;
      if (this._useGpuPick) {
        this._pickDataDirty = true;
        this._rebuildPickData();
      }
      this._scheduleRender();
      if (this._edgeWorkerPending) {
        this._edgeWorkerPending = false;
        this._scheduleEdgeWorker();
      }
    }

    _rebuildVisibleEdges() {
      if (!this._edgeCullActive || !this.gl) return;
      if (!this._edgeData || this._edgeSegmentsCount <= 0) return;
      if (this._edgeGridDirty) this._rebuildEdgeGrid();
      const view = this._getWorldViewBounds(120);
      const size = this._edgeGridSize;
      const gx0 = Math.floor(view.minX / size);
      const gx1 = Math.floor(view.maxX / size);
      const gy0 = Math.floor(view.minY / size);
      const gy1 = Math.floor(view.maxY / size);
      const candidates = new Set();
      for (let gx = gx0; gx <= gx1; gx += 1) {
        for (let gy = gy0; gy <= gy1; gy += 1) {
          const key = `${gx},${gy}`;
          const bucket = this._edgeGrid.get(key);
          if (bucket) bucket.forEach((edge) => candidates.add(edge));
        }
      }
      const edgeStride = 12;
      const arrowStride = 9;
      const visibleEdges = [];
      let segTotal = 0;
      let arrowTotal = 0;
      const viewMinX = view.minX;
      const viewMaxX = view.maxX;
      const viewMinY = view.minY;
      const viewMaxY = view.maxY;
      const list = candidates.size ? Array.from(candidates) : this._edges;
      list.forEach((edge) => {
        const m = edge.getModel();
        const bounds = this._edgeBounds(edge);
        if (
          bounds.maxX < viewMinX ||
          bounds.minX > viewMaxX ||
          bounds.maxY < viewMinY ||
          bounds.minY > viewMaxY
        ) {
          return;
        }
        const segCount = m.__segCount || 0;
        const arrowCount = m.__arrowCount || 0;
        if (segCount <= 0) return;
        visibleEdges.push(edge);
        segTotal += segCount;
        arrowTotal += arrowCount;
      });
      if (!this._edgeVisibleData || this._edgeVisibleData.length < segTotal * edgeStride) {
        this._edgeVisibleData = new Float32Array(segTotal * edgeStride);
      }
      if (!this._arrowVisibleData || this._arrowVisibleData.length < arrowTotal * arrowStride) {
        this._arrowVisibleData = new Float32Array(arrowTotal * arrowStride);
      }
      let segCursor = 0;
      let arrowCursor = 0;
      visibleEdges.forEach((edge) => {
        const m = edge.getModel();
        const segStart = m.__segStart || 0;
        const segCount = m.__segCount || 0;
        const arrowStart = m.__arrowStart || 0;
        const arrowCount = m.__arrowCount || 0;
        if (segCount > 0) {
          const src = this._edgeData.subarray(segStart * edgeStride, (segStart + segCount) * edgeStride);
          this._edgeVisibleData.set(src, segCursor * edgeStride);
          segCursor += segCount;
        }
        if (arrowCount > 0 && this._arrowData) {
          const src = this._arrowData.subarray(arrowStart * arrowStride, (arrowStart + arrowCount) * arrowStride);
          this._arrowVisibleData.set(src, arrowCursor * arrowStride);
          arrowCursor += arrowCount;
        }
      });
      this._edgeVisibleCount = segCursor;
      this._arrowVisibleCount = arrowCursor;
      if (!this._edgeVisibleBuffer) this._edgeVisibleBuffer = this.gl.createBuffer();
      this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this._edgeVisibleBuffer);
      this.gl.bufferData(
        this.gl.ARRAY_BUFFER,
        this._edgeVisibleData.subarray(0, this._edgeVisibleCount * edgeStride),
        this.gl.DYNAMIC_DRAW
      );
      if (!this._arrowVisibleBuffer) this._arrowVisibleBuffer = this.gl.createBuffer();
      this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this._arrowVisibleBuffer);
      this.gl.bufferData(
        this.gl.ARRAY_BUFFER,
        this._arrowVisibleData.subarray(0, this._arrowVisibleCount * arrowStride),
        this.gl.DYNAMIC_DRAW
      );
      this._edgeViewDirty = false;
    }

    _rebuildHighlightBuffers() {
      if (!this.gl) return;
      if (!this._highlightEdges || this._highlightEdges.size === 0) {
        this._highlightEdgeCount = 0;
        this._highlightDirty = false;
        return;
      }
      if (!this._edgeData || this._edgeSegmentsCount <= 0) {
        this._highlightDirty = true;
        return;
      }
      const edgeStride = 12;
      const highlight = parseColor(HIGHLIGHT_COLOR);
      const sizeScale = this._getSizeScale();
      let total = 0;
      const list = Array.from(this._highlightEdges);
      list.forEach((edge) => {
        const m = edge.getModel();
        const segCount = m.__segCount || 0;
        if (!segCount) return;
        const states = edge.getStates();
        const selected = states.includes("selected") || states.includes("active");
        const hover = !selected && states.includes("hover");
        if (!selected && !hover) return;
        const isDouble = String(m.mode || "").toLowerCase() === "double";
        if (selected && isDouble) return;
        total += segCount;
      });
      if (!this._highlightEdgeData || this._highlightEdgeData.length < total * edgeStride) {
        this._highlightEdgeData = new Float32Array(total * edgeStride);
      }
      let cursor = 0;
      list.forEach((edge) => {
        const m = edge.getModel();
        const segStart = m.__segStart || 0;
        const segCount = m.__segCount || 0;
        if (!segCount) return;
        const states = edge.getStates();
        const selected = states.includes("selected") || states.includes("active");
        const hover = !selected && states.includes("hover");
        if (!selected && !hover) return;
        const isDouble = String(m.mode || "").toLowerCase() === "double";
        if (selected && isDouble) return;
        const glowOpacity = selected ? 0.9 : 0.45;
        const glowWidthBoost = (selected ? 3 : 2) * sizeScale;
        for (let i = 0; i < segCount; i += 1) {
          const srcIdx = (segStart + i) * edgeStride;
          const dstIdx = cursor * edgeStride;
          this._highlightEdgeData[dstIdx] = this._edgeData[srcIdx];
          this._highlightEdgeData[dstIdx + 1] = this._edgeData[srcIdx + 1];
          this._highlightEdgeData[dstIdx + 2] = this._edgeData[srcIdx + 2];
          this._highlightEdgeData[dstIdx + 3] = this._edgeData[srcIdx + 3];
          this._highlightEdgeData[dstIdx + 4] = this._edgeData[srcIdx + 4] + glowWidthBoost;
          this._highlightEdgeData[dstIdx + 5] = highlight[0];
          this._highlightEdgeData[dstIdx + 6] = highlight[1];
          this._highlightEdgeData[dstIdx + 7] = highlight[2];
          this._highlightEdgeData[dstIdx + 8] = highlight[3] * glowOpacity;
          this._highlightEdgeData[dstIdx + 9] = this._edgeData[srcIdx + 9];
          this._highlightEdgeData[dstIdx + 10] = this._edgeData[srcIdx + 10];
          this._highlightEdgeData[dstIdx + 11] = this._edgeData[srcIdx + 11];
          cursor += 1;
        }
      });
      this._highlightEdgeCount = cursor;
      if (!this._highlightEdgeBuffer) this._highlightEdgeBuffer = this.gl.createBuffer();
      this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this._highlightEdgeBuffer);
      this.gl.bufferData(
        this.gl.ARRAY_BUFFER,
        this._highlightEdgeData.subarray(0, this._highlightEdgeCount * edgeStride),
        this.gl.DYNAMIC_DRAW
      );
      this._highlightDirty = false;
    }

    _updateHighlightEdge(edge) {
      if (!edge) return;
      const states = edge.getStates();
      const on = states.includes("selected") || states.includes("active") || states.includes("hover");
      if (on) this._highlightEdges.add(edge);
      else this._highlightEdges.delete(edge);
      this._highlightDirty = true;
    }

    _disposeTextAtlasSet(key, set) {
      if (!set) return;
      const gl = this.gl;
      (Array.isArray(set.atlases) ? set.atlases : []).forEach((atlas) => {
        const buffer = this._textBufferMap.get(atlas);
        if (gl && buffer) {
          try {
            gl.deleteBuffer(buffer);
          } catch (e) {}
        }
        this._textBufferMap.delete(atlas);
      });
      try {
        set.destroy?.();
      } catch (e) {}
      this._textAtlases.delete(key);
    }

    _pruneTextAtlasCache(activeAtlases = null) {
      const entries = Array.from(this._textAtlases.entries());
      if (!entries.length) return;
      const totalTextureBytes = entries.reduce(
        (sum, [, set]) => sum + (set?.getStats?.().approxTextureBytes || 0),
        0
      );
      if (entries.length <= TEXT_ATLAS_SET_CACHE_LIMIT && totalTextureBytes <= TEXT_ATLAS_CACHE_LIMIT_BYTES) {
        return;
      }
      const now = typeof performance !== "undefined" && typeof performance.now === "function" ? performance.now() : Date.now();
      const inactiveEntries = entries.filter(([, set]) => !set?.hasActiveAtlas?.(activeAtlases));
      if (!inactiveEntries.length) return;
      let remainingSetCount = entries.length;
      let remainingTextureBytes = totalTextureBytes;
      const byLastUsed = inactiveEntries.slice().sort((left, right) => {
        const leftAt = Number(left[1]?.lastUsedAt) || 0;
        const rightAt = Number(right[1]?.lastUsedAt) || 0;
        return leftAt - rightAt;
      });
      const releaseUntilWithinLimit = (predicate) => {
        byLastUsed.forEach(([key, set]) => {
          if (remainingSetCount <= TEXT_ATLAS_SET_CACHE_LIMIT && remainingTextureBytes <= TEXT_ATLAS_CACHE_LIMIT_BYTES) {
            return;
          }
          if (!predicate(set)) {
            return;
          }
          const approxTextureBytes = set?.getStats?.().approxTextureBytes || 0;
          this._disposeTextAtlasSet(key, set);
          remainingSetCount -= 1;
          remainingTextureBytes -= approxTextureBytes;
        });
      };
      releaseUntilWithinLimit((set) => set?.isIdle?.(now, TEXT_ATLAS_IDLE_EVICT_MS));
      releaseUntilWithinLimit(() => true);
    }

    getTextAtlasStats() {
      const byKey = Array.from(this._textAtlases.entries()).map(([key, set]) => ({
        key,
        ...(set?.getStats?.() || {
          atlasCount: 0,
          glyphCount: 0,
          approxTextureBytes: 0,
          lastUsedAt: 0,
        }),
      }));
      return {
        setCount: byKey.length,
        atlasCount: byKey.reduce((sum, item) => sum + (item.atlasCount || 0), 0),
        glyphCount: byKey.reduce((sum, item) => sum + (item.glyphCount || 0), 0),
        approxTextureBytes: byKey.reduce((sum, item) => sum + (item.approxTextureBytes || 0), 0),
        batchCount: Array.isArray(this._textBatches) ? this._textBatches.length : 0,
        byKey,
      };
    }

    _getTextAtlasSet(fontFamily, fontWeight, fontStyle, fontSize) {
      const family = fontFamily || DEFAULT_FONT_FAMILY;
      const weight = fontWeight || 500;
      const style = fontStyle || "normal";
      const sizeHint = Number(fontSize) || this._textBaseSize;
      let baseSize = this._textBaseSize;
      if (sizeHint >= 18) baseSize = Math.max(baseSize, 64);
      else if (sizeHint >= 14) baseSize = Math.max(baseSize, 48);
      else if (sizeHint >= 12) baseSize = Math.max(baseSize, 40);
      const key = `${style}|${weight}|${family}|${baseSize}`;
      if (this._textAtlases.has(key)) {
        const existing = this._textAtlases.get(key);
        existing?._touch?.();
        return existing;
      }
      const maxTex = this.gl ? this.gl.getParameter(this.gl.MAX_TEXTURE_SIZE) : TEXT_ATLAS_SIZE;
      const size = Math.min(TEXT_ATLAS_SIZE, maxTex || TEXT_ATLAS_SIZE);
      const set = new TextAtlasSet(this.gl, {
        fontFamily: family,
        fontWeight: weight,
        fontStyle: style,
        baseSize,
        scale: this._textAtlasScale,
        size,
        maxSize: size,
        padding: TEXT_ATLAS_PADDING,
      });
      this._textAtlases.set(key, set);
      set._touch?.();
      return set;
    }

    _measureText(set, text) {
      const glyphs = [];
      let width = 0;
      const raw = String(text || "");
      for (const ch of raw) {
        const glyph = set.getGlyph(ch);
        if (!glyph) continue;
        glyphs.push(glyph);
        width += glyph.advance || glyph.width || 0;
      }
      const spacing = 0;
      return { width, glyphs, spacing };
    }

    _appendTextRun(batchMap, set, text, x, y, opts = {}) {
      const raw = String(text || "");
      if (!raw) return;
      const snapText = opts.snapToPixel !== false && this._snapTextToPixel;
      const snapGlyph = opts.snapGlyphToPixel !== false && this._snapGlyphToPixel;
      if (snapText) {
        const snapped = this._snapWorldToPixel(x, y);
        x = snapped.x;
        y = snapped.y;
      }
      const fontSize = Number(opts.fontSize) || 12;
      const sizeScale = this._getSizeScale();
      const scaleFactor = (fontSize * sizeScale) / this._textBaseSize;
      const measure = this._measureText(set, raw);
      const widthWorld = measure.width * scaleFactor;
      const fallbackAscent = (set.atlases[0]?.ascent || this._textBaseSize * 0.8) * scaleFactor;
      const fallbackDescent = (set.atlases[0]?.descent || this._textBaseSize * 0.2) * scaleFactor;
      let runTopRel = Infinity;
      let runBottomRel = -Infinity;
      measure.glyphs.forEach((glyph) => {
        const w = (glyph.width || glyph.w || 0) * scaleFactor;
        const h = (glyph.height || glyph.h || 0) * scaleFactor;
        if (w <= 0 || h <= 0) return;
        const topRel = -(glyph.bearingY || 0) * scaleFactor;
        const bottomRel = topRel + h;
        if (topRel < runTopRel) runTopRel = topRel;
        if (bottomRel > runBottomRel) runBottomRel = bottomRel;
      });
      const centerRel =
        Number.isFinite(runTopRel) && Number.isFinite(runBottomRel)
          ? (runTopRel + runBottomRel) * 0.5
          : (fallbackDescent - fallbackAscent) * 0.5;
      const baseline = y - centerRel;
      const startX = x - widthWorld / 2;
      const color = opts.color || [0.1, 0.1, 0.1, 1];
      let penX = 0;
      measure.glyphs.forEach((glyph) => {
        const atlas = glyph.atlas;
        if (!atlas) return;
        if (!batchMap.has(atlas)) batchMap.set(atlas, []);
        const list = batchMap.get(atlas);
        const w = (glyph.width || glyph.w || 0) * scaleFactor;
        const h = (glyph.height || glyph.h || 0) * scaleFactor;
        if (w <= 0 || h <= 0) {
          penX += (glyph.advance || glyph.width || 0) + (measure.spacing || 0);
          return;
        }
        const gx = startX + (penX - (glyph.bearingX || 0)) * scaleFactor;
        const gy = baseline - (glyph.bearingY || 0) * scaleFactor;
        let cx = gx + w * 0.5;
        let cy = gy + h * 0.5;
        if (snapGlyph) {
          const snappedGlyph = this._snapWorldToPixel(cx, cy);
          cx = snappedGlyph.x;
          cy = snappedGlyph.y;
        }
        list.push(
          cx,
          cy,
          w,
          h,
          glyph.u0,
          glyph.v0,
          glyph.u1,
          glyph.v1,
          color[0],
          color[1],
          color[2],
          color[3]
        );
        penX += (glyph.advance || glyph.width || 0) + (measure.spacing || 0);
      });

      if (opts.underline) {
        const white = set.getWhiteGlyph();
        if (white) {
          const atlas = white.atlas;
          if (atlas) {
            if (!batchMap.has(atlas)) batchMap.set(atlas, []);
            const list = batchMap.get(atlas);
            const underlineHeight = Math.max(1, fontSize * 0.08) * sizeScale;
            const underlineOffset = fontSize * 0.5 * sizeScale;
            list.push(
              x,
              y + underlineOffset,
              widthWorld,
              underlineHeight,
              white.u0,
              white.v0,
              white.u1,
              white.v1,
              color[0],
              color[1],
              color[2],
              color[3]
            );
          }
        }
      }
    }

    _appendGlyph(batchMap, glyph, cx, cy, size, color, opts = {}) {
      if (!glyph) return;
      const atlas = glyph.atlas;
      if (!atlas) return;
      if (this._snapTextToPixel) {
        const snapped = this._snapWorldToPixel(cx, cy);
        cx = snapped.x;
        cy = snapped.y;
      }
      if (!batchMap.has(atlas)) batchMap.set(atlas, []);
      const list = batchMap.get(atlas);
      const baseW = Number(glyph.width || glyph.w || 0);
      const baseH = Number(glyph.height || glyph.h || 0);
      if (!baseW || !baseH) return;
      const targetSize = Number(size) || baseH;
      const scale = targetSize / Math.max(1e-6, baseH);
      const w = baseW * scale;
      const h = baseH * scale;
      let visualOffsetX = (Number(glyph.offsetX) || 0) * scale;
      let visualOffsetY = (Number(glyph.offsetY) || 0) * scale;
      if (opts?.applyIconOffset) {
        visualOffsetX += (Number(glyph.iconOffsetX) || 0) * scale;
        visualOffsetY += (Number(glyph.iconOffsetY) || 0) * scale;
      }
      list.push(
        cx + visualOffsetX,
        cy + visualOffsetY,
        w,
        h,
        glyph.u0,
        glyph.v0,
        glyph.u1,
        glyph.v1,
        color[0],
        color[1],
        color[2],
        color[3]
      );
    }

    _buildTextBatches() {
      if (!this._shouldUseGpuText() || !this.gl) return;
      const batchMap = new Map();
      const sizeScale = this._getSizeScale();
      const view = this._getWorldViewBounds(GPU_TEXT_PREFETCH_MARGIN_PX);
      const inView = (x, y) =>
        x >= view.minX && x <= view.maxX && y >= view.minY && y <= view.maxY;

      // Edge labels
      this._edges.forEach((edge) => {
        const m = edge.getModel();
        const { label, labelTop, labelBottom } = resolveEdgeLabelRows(m);
        if (!label && !labelTop && !labelBottom) return;
        const s = edge.getSource().getModel();
        const t = edge.getTarget().getModel();
        const sx = Number(s.x) || 0;
        const sy = Number(s.y) || 0;
        const tx = Number(t.x) || 0;
        const ty = Number(t.y) || 0;
        const textColor = parseColor(m.textColor || "rgba(2,6,23,.82)");
        const fontFamily = m.fontFamily || DEFAULT_FONT_FAMILY;
        const baseSize = clamp(Number(m.fontSize ?? 13), 8, 36);
        const labelSize = Math.max(8, baseSize - 1);
        const fontStyle = asBool(m.fontItalic, false) ? "italic" : "normal";
        const fontWeight = asBool(m.fontBold, false) ? 700 : 500;
        const atlasSet = this._getTextAtlasSet(fontFamily, fontWeight, fontStyle, labelSize);
        const loopSet = this._getTextAtlasSet(fontFamily, fontWeight, fontStyle, baseSize);
        const shadow = this._gpuTextEnableShadow && asBool(m.fontShadow, false);
        const shadowOffset = shadow ? 1 * sizeScale : 0;
        const labelOffset = resolveEdgeLabelYOffset(m, sizeScale);
        const edgeLabelMaxWorldWidth = (size = labelSize) => {
          const distanceWorld = Math.hypot(tx0 - sx0, ty0 - sy0);
          if (!Number.isFinite(distanceWorld) || distanceWorld <= EPS) return Infinity;
          const sourceRadiusScreen = Math.max(8, Number(s.r) || 18);
          const targetRadiusScreen = Math.max(8, Number(t.r) || 18);
          const labelPaddingScreen = Math.max(16, Number(size) * 1.25);
          const availableScreen =
            distanceWorld / Math.max(sizeScale, EPS) -
            sourceRadiusScreen -
            targetRadiusScreen -
            labelPaddingScreen;
          return Math.max(0, availableScreen) * sizeScale;
        };
        const displayEdgeLabel = (text, size = labelSize) => {
          const maxWorldWidth = edgeLabelMaxWorldWidth(size);
          return truncateTextByWorldWidth(text, size, maxWorldWidth, sizeScale);
        };
        const offsetSingleLabelPoint = (point) => point;
        const drawLabel = (text, x, y, size = labelSize, set = atlasSet) => {
          const displayText = displayEdgeLabel(text, size);
          if (!displayText) return;
          if (shadow) {
            this._appendTextRun(batchMap, set, displayText, x, y + shadowOffset, {
              fontSize: size,
              color: [0.1, 0.1, 0.1, 0.35],
              snapToPixel: false,
              snapGlyphToPixel: false,
            });
          }
          this._appendTextRun(batchMap, set, displayText, x, y, {
            fontSize: size,
            color: textColor,
            underline: asBool(m.fontUnderline, false),
            snapToPixel: false,
            snapGlyphToPixel: false,
          });
        };

        const isDouble = String(m.mode || "").toLowerCase() === "double";
        const edgeOffset = (Number(m.edgeOffset) || 0) * sizeScale;
        const baseDir = normalizeVector(tx - sx, ty - sy);
        const basePerp = { x: -baseDir.y, y: baseDir.x };
        const sx0 = sx + basePerp.x * edgeOffset;
        const sy0 = sy + basePerp.y * edgeOffset;
        const tx0 = tx + basePerp.x * edgeOffset;
        const ty0 = ty + basePerp.y * edgeOffset;

        if (m.source === m.target) {
          const r = (Number(s.r) || 18) * sizeScale;
          const stroke = Number(s.lineWidth ?? 2) || 2;
          const selected = edge.getSource()?.hasState?.("selected") || edge.getSource()?.hasState?.("active");
          const sPad = (Math.max(0.9, stroke / 2 + 0.4) + (selected ? 1.8 : 0)) * sizeScale;
          const outPadSelf = (Number(m.outPad ?? 1.2) || 0) * sizeScale;
          const loopScale = Number(m.loopScale ?? 2) || 2;
          const baseR = r + outPadSelf + sPad;
          const peak = Math.max(baseR * 0.75 * loopScale, baseSize * 1.2 * sizeScale);
          const mid = { x: sx0, y: sy0 - peak };
          if (!inView(mid.x, mid.y)) return;
          if (label) drawLabel(label, mid.x, mid.y + labelOffset, baseSize, loopSet);
          return;
        }

        const singleAnchor = !isDouble ? computeSingleEdgeLabelAnchor(edge, sizeScale) : null;
        const mid =
          singleAnchor && Number.isFinite(singleAnchor.x) && Number.isFinite(singleAnchor.y)
            ? singleAnchor
            : { x: (sx0 + tx0) / 2, y: (sy0 + ty0) / 2 };
        if (!inView(mid.x, mid.y)) return;

        if (isDouble) {
          const gap = resolveDoubleGap(m, sizeScale);
          const half = gap / 2;
          const topMid = { x: mid.x + basePerp.x * half, y: mid.y + basePerp.y * half };
          const botMid = { x: mid.x - basePerp.x * half, y: mid.y - basePerp.y * half };
          const showCenter = !!m.detailLabel && !!label;
          if (showCenter) drawLabel(label, mid.x, mid.y + labelOffset);
          if (labelTop) drawLabel(labelTop, topMid.x, topMid.y + labelOffset);
          if (labelBottom) drawLabel(labelBottom, botMid.x, botMid.y + labelOffset);
        } else {
          if (label) {
            const labelPoint = offsetSingleLabelPoint(mid);
            drawLabel(label, labelPoint.x, labelPoint.y + labelOffset);
          }
        }
      });

      // Node labels
      let nodeLabelMode = "full";
      // User requirement: show all node labels; disable zoom-based node-label LOD trimming.
      let visibleNodeLabels = null;
      void visibleNodeLabels;

      this._nodes.forEach((node) => {
        const m = node.getModel();
        if (nodeLabelMode !== "full") {
          if (asBool(m.__labelForceShow, false) || asBool(m.labelForceShow, false)) {
            // explicit opt-in path for critical labels
          } else {
            const id = String(m.id || "").trim();
            if (!visibleNodeLabels || !visibleNodeLabels.has(id)) return;
          }
        }
        const nx = Number(m.x) || 0;
        const ny = Number(m.y) || 0;
        if (!inView(nx, ny)) return;
        const labelLayout = resolveNodeLabelRows(m, sizeScale);
        const textColor = parseColor(m.textColor || "rgba(2,6,23,.88)");
        const subTextColor = parseColor(m.subTextColor || "rgba(2,6,23,.72)");
        const fontFamily = m.fontFamily || DEFAULT_FONT_FAMILY;
        const baseSize = labelLayout.baseSize;
        const subSize = labelLayout.subSize;
        const fontStyle = asBool(m.fontItalic, false) ? "italic" : "normal";
        const weightStrong = asBool(m.fontBold, false) ? 800 : 600;
        const weightSub = asBool(m.fontBold, false) ? 600 : 400;
        const atlasStrong = this._getTextAtlasSet(fontFamily, weightStrong, fontStyle, baseSize);
        const atlasSub = this._getTextAtlasSet(fontFamily, weightSub, fontStyle, subSize);
        const shadow = this._gpuTextEnableShadow && asBool(m.fontShadow, false);
        const shadowOffset = shadow ? 1 * sizeScale : 0;
        if (m.iconSymbol) {
          const symbolId = String(m.iconSymbol || "").trim();
          if (symbolId) {
            const iconSize = Math.max(8, Number(m.iconSize ?? 16));
            const iconSet = this._getTextAtlasSet(fontFamily, weightStrong, fontStyle, iconSize);
            const url = buildIconDataUrl(symbolId, "#ffffff");
            const rec = getIconImageRecord(url, this);
            if (rec && rec.ready && rec.img && !rec.error) {
              const glyph = iconSet.getImageGlyph(symbolId, rec.img, iconSize, iconSize);
              const iconOffsetX = (Number(m.iconOffsetX) || 0) * sizeScale;
              const iconOffsetY = (Number(m.iconOffsetY) || 0) * sizeScale;
              const iconWorld = iconSize * sizeScale;
              const offsetX = (rec.offsetX || 0) * iconWorld;
              const offsetY = (rec.offsetY || 0) * iconWorld;
              if (shadow) {
                this._appendGlyph(
                  batchMap,
                  glyph,
                  nx + offsetX + iconOffsetX,
                  ny + iconOffsetY + offsetY + shadowOffset,
                  iconWorld,
                  [0.1, 0.1, 0.1, 0.35]
                );
              }
              this._appendGlyph(
                batchMap,
                glyph,
                nx + offsetX + iconOffsetX,
                ny + iconOffsetY + offsetY,
                iconWorld,
                textColor
              );
            } else if (rec && !rec.error) {
              this._labelsDirty = true;
              this._textDirty = true;
            }
          }
        } else if (m.icon) {
          const iconFont = Math.max(8, Number(m.iconSize ?? 16));
          const iconSet = this._getTextAtlasSet(fontFamily, weightStrong, fontStyle, iconFont);
          const iconColor = textColor;
          const iconOffsetX = (Number(m.iconOffsetX) || 0) * sizeScale;
          const iconOffsetY = (Number(m.iconOffsetY) || 0) * sizeScale;
          const iconText = String(m.icon || "");
          if (iconText.length === 1) {
            const glyph = iconSet.getGlyph(iconText);
            const iconWorld = iconFont * sizeScale;
            if (glyph) {
              if (shadow) {
                this._appendGlyph(
                  batchMap,
                  glyph,
                  nx + iconOffsetX,
                  ny + iconOffsetY + shadowOffset,
                  iconWorld,
                  [0.1, 0.1, 0.1, 0.35],
                  { applyIconOffset: true }
                );
              }
              this._appendGlyph(
                batchMap,
                glyph,
                nx + iconOffsetX,
                ny + iconOffsetY,
                iconWorld,
                iconColor,
                { applyIconOffset: true }
              );
            } else {
              if (shadow) {
                this._appendTextRun(batchMap, iconSet, iconText, nx + iconOffsetX, ny + iconOffsetY + shadowOffset, {
                  fontSize: iconFont,
                  color: [0.1, 0.1, 0.1, 0.35],
                });
              }
              this._appendTextRun(batchMap, iconSet, iconText, nx + iconOffsetX, ny + iconOffsetY, {
                fontSize: iconFont,
                color: iconColor,
              });
            }
          } else {
            if (shadow) {
              this._appendTextRun(batchMap, iconSet, iconText, nx + iconOffsetX, ny + iconOffsetY + shadowOffset, {
                fontSize: iconFont,
                color: [0.1, 0.1, 0.1, 0.35],
              });
            }
            this._appendTextRun(batchMap, iconSet, iconText, nx + iconOffsetX, ny + iconOffsetY, {
              fontSize: iconFont,
              color: iconColor,
            });
          }
        }
        labelLayout.rows.forEach((row) => {
          if (!row?.text) return;
          const isSub = row.tier === "sub";
          const atlas = isSub ? atlasSub : atlasStrong;
          const fontSize = isSub ? subSize : baseSize;
          const color = isSub ? subTextColor : textColor;
          const shadowColor = isSub ? [0.1, 0.1, 0.1, 0.28] : [0.1, 0.1, 0.1, 0.35];
          const y = ny + (Number(row.offsetY) || 0);
          if (shadow) {
            this._appendTextRun(batchMap, atlas, row.text, nx, y + shadowOffset, {
              fontSize,
              color: shadowColor,
            });
          }
          this._appendTextRun(batchMap, atlas, row.text, nx, y, {
            fontSize,
            color,
            underline: !isSub && asBool(m.fontUnderline, false),
          });
        });
      });

      // Upload batches
      this._textBatches = [];
      batchMap.forEach((list, atlas) => {
        const data = new Float32Array(list);
        let buffer = this._textBufferMap.get(atlas);
        if (!buffer && this.gl) {
          buffer = this.gl.createBuffer();
          this._textBufferMap.set(atlas, buffer);
        }
        if (buffer && this.gl) {
          this.gl.bindBuffer(this.gl.ARRAY_BUFFER, buffer);
          this.gl.bufferData(this.gl.ARRAY_BUFFER, data, this.gl.DYNAMIC_DRAW);
        }
        this._textBatches.push({ atlas, buffer, data, count: data.length / this._textStride });
      });
      this._pruneTextAtlasCache(new Set(batchMap.keys()));
      this._textDirty = false;
    }

    _bindTextAttributes(scale, translate, resolution, batch, contrast = 1, clearColor = null, alphaScale = 1) {
      return bindTextAttributes(this, scale, translate, resolution, batch, contrast, clearColor, alphaScale);
    }

    _drawTextBatches() {
      return drawGraphTextBatches(this);
    }

    _ensurePickFbo() {
      const gl = this.gl;
      if (!gl) return;
      const w = this.canvas.width;
      const h = this.canvas.height;
      const needInit = !this._pickFbo || !this._pickTex || !this._pickDepth;
      if (needInit) {
        this._pickFbo = gl.createFramebuffer();
        this._pickTex = gl.createTexture();
        this._pickDepth = gl.createRenderbuffer();
      }
      gl.bindTexture(gl.TEXTURE_2D, this._pickTex);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, w, h, 0, gl.RGBA, gl.UNSIGNED_BYTE, null);
      gl.bindRenderbuffer(gl.RENDERBUFFER, this._pickDepth);
      gl.renderbufferStorage(gl.RENDERBUFFER, gl.DEPTH_COMPONENT16, w, h);
      gl.bindFramebuffer(gl.FRAMEBUFFER, this._pickFbo);
      gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, this._pickTex, 0);
      gl.framebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, this._pickDepth);
      gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    }

    _rebuildPickData() {
      if (!this._useGpuPick || !this.gl) return;
      const gl = this.gl;
      const nodes = this._nodes;
      const edges = this._edges;
      const nodeStride = 7;
      const edgeStride = 9;
      const scale = this._scale || 1;
      const nodeCount = nodes.length;
      this._pickNodeCount = nodeCount;
      this._nodePickData = new Float32Array(nodeCount * nodeStride);
      nodes.forEach((node, i) => {
        const m = node.getModel();
        const idx = i * nodeStride;
        const r = Number(m.r) || 18;
        const pick = encodePickColor(node._pickId || i + 1);
        this._nodePickData[idx] = Number(m.x) || 0;
        this._nodePickData[idx + 1] = Number(m.y) || 0;
        this._nodePickData[idx + 2] = r + 6 / Math.max(scale, 0.001);
        this._nodePickData[idx + 3] = pick[0];
        this._nodePickData[idx + 4] = pick[1];
        this._nodePickData[idx + 5] = pick[2];
        this._nodePickData[idx + 6] = pick[3];
      });
      gl.bindBuffer(gl.ARRAY_BUFFER, this._pickNodeBuffer);
      gl.bufferData(gl.ARRAY_BUFFER, this._nodePickData, gl.DYNAMIC_DRAW);

      const segCount = this._edgeSegmentsCount || 0;
      this._pickEdgeCount = segCount;
      this._edgePickData = new Float32Array(segCount * edgeStride);
      edges.forEach((edge) => {
        const m = edge.getModel();
        const segStart = m.__segStart || 0;
        const segCountEdge = m.__segCount || 0;
        const pick = encodePickColor(edge._pickId || 0);
        for (let i = 0; i < segCountEdge; i += 1) {
          const srcIdx = (segStart + i) * 12;
          const dstIdx = (segStart + i) * edgeStride;
          const sx = this._edgeData[srcIdx];
          const sy = this._edgeData[srcIdx + 1];
          const tx = this._edgeData[srcIdx + 2];
          const ty = this._edgeData[srcIdx + 3];
          const lineWidth = this._edgeData[srcIdx + 4];
          const pickWidth = lineWidth + 6 / Math.max(scale, 0.001);
          this._edgePickData[dstIdx] = sx;
          this._edgePickData[dstIdx + 1] = sy;
          this._edgePickData[dstIdx + 2] = tx;
          this._edgePickData[dstIdx + 3] = ty;
          this._edgePickData[dstIdx + 4] = pickWidth;
          this._edgePickData[dstIdx + 5] = pick[0];
          this._edgePickData[dstIdx + 6] = pick[1];
          this._edgePickData[dstIdx + 7] = pick[2];
          this._edgePickData[dstIdx + 8] = pick[3];
        }
      });
      gl.bindBuffer(gl.ARRAY_BUFFER, this._pickEdgeBuffer);
      gl.bufferData(gl.ARRAY_BUFFER, this._edgePickData, gl.DYNAMIC_DRAW);
      this._pickDataDirty = false;
      this._pickRenderDirty = true;
    }

    _renderPickBuffer() {
      if (!this._useGpuPick || !this.gl) return;
      if (this._pickDataDirty) this._rebuildPickData();
      this._ensurePickFbo();
      const gl = this.gl;
      gl.bindFramebuffer(gl.FRAMEBUFFER, this._pickFbo);
      gl.viewport(0, 0, this.canvas.width, this.canvas.height);
      gl.disable(gl.BLEND);
      gl.clearColor(0, 0, 0, 0);
      gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);
      const resolution = [this.canvas.width, this.canvas.height];
      const scale = this._scale * this._pixelRatio;
      const translate = { x: this._translate.x * this._pixelRatio, y: this._translate.y * this._pixelRatio };

      if (this._pickEdgeCount > 0) {
        gl.useProgram(this._pickEdgeProgram);
        this._bindPickEdgeAttributes(scale, translate, resolution);
        this._drawInstanced(gl.TRIANGLE_STRIP, 4, this._pickEdgeCount);
      }
      if (this._pickNodeCount > 0) {
        gl.useProgram(this._pickNodeProgram);
        this._bindPickNodeAttributes(scale, translate, resolution);
        this._drawInstanced(gl.TRIANGLE_STRIP, 4, this._pickNodeCount);
      }
      gl.enable(gl.BLEND);
      gl.bindFramebuffer(gl.FRAMEBUFFER, null);
      gl.viewport(0, 0, this.canvas.width, this.canvas.height);
      this._pickRenderDirty = false;
    }

    _bindPickNodeAttributes(scale, translate, resolution) {
      return bindPickNodeAttributes(this, scale, translate, resolution);
    }

    _bindPickEdgeAttributes(scale, translate, resolution) {
      return bindPickEdgeAttributes(this, scale, translate, resolution);
    }

    _pickByGpu(clientX, clientY) {
      if (!this._useGpuPick || !this.gl) return null;
      if (this._disablePickDuringInteraction && (this._interactionActive || this._wheelActive)) return null;
      if (this._pickRenderDirty) this._renderPickBuffer();
      const gl = this.gl;
      const rect = this.canvas.getBoundingClientRect();
      const dpr = this._pixelRatio || 1;
      const x = Math.round((clientX - rect.left) * dpr);
      const y = Math.round((clientY - rect.top) * dpr);
      if (x < 0 || y < 0 || x >= this.canvas.width || y >= this.canvas.height) return null;
      const readY = this.canvas.height - y - 1;
      const pixel = new Uint8Array(4);
      gl.bindFramebuffer(gl.FRAMEBUFFER, this._pickFbo);
      gl.readPixels(x, readY, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, pixel);
      gl.bindFramebuffer(gl.FRAMEBUFFER, null);
      const id = pixel[0] + pixel[1] * 256 + pixel[2] * 65536 + pixel[3] * 16777216;
      if (!id) return null;
      const nodeCount = this._nodes.length;
      if (id <= nodeCount) return { type: "node", item: this._nodes[id - 1] };
      const edgeIndex = id - nodeCount - 1;
      if (edgeIndex >= 0 && edgeIndex < this._edges.length) {
        return { type: "edge", item: this._edges[edgeIndex] };
      }
      return null;
    }

    _scheduleRender() {
      if (this._destroyed) return;
      if (this._raf) return;
      this._raf = requestAnimationFrame(() => {
        this._raf = 0;
        if (this._destroyed) return;
        this._renderNow();
      });
    }

    _getSizeScale() {
      if (this._scaleWithView) return 1;
      return 1 / Math.max(this._scale || 1, 0.001);
    }

    _computeZoomVisualStyle() {
      const zoom = Math.max(Number(this._scale) || 1, 0.0001);
      const span = Math.max(0.0001, ZOOM_VISUAL_ADAPT_START - ZOOM_VISUAL_ADAPT_END);
      const tRaw = clamp((zoom - ZOOM_VISUAL_ADAPT_END) / span, 0, 1);
      const t = tRaw * tRaw * (3 - 2 * tRaw);
      const lowZoomWeight = 1 - t;
      const edgeContrast = clamp(
        EDGE_BASE_CONTRAST - (EDGE_BASE_CONTRAST - EDGE_LOW_ZOOM_CONTRAST_MIN) * lowZoomWeight,
        0.05,
        1
      );
      const nodeContrast = clamp(
        NODE_BASE_CONTRAST + (NODE_LOW_ZOOM_CONTRAST_MAX - NODE_BASE_CONTRAST) * lowZoomWeight,
        1,
        1.5
      );
      return {
        edgeContrast,
        nodeContrast,
        labelContrast: LABEL_BASE_CONTRAST,
        labelAlpha: LABEL_BASE_ALPHA,
      };
    }

    _isEdgeInFocus(edge) {
      if (!edge) return false;
      if (edge.hasState("selected") || edge.hasState("active") || edge.hasState("hover")) return true;
      const source = edge.getSource?.();
      const target = edge.getTarget?.();
      const nodeHot = (node) =>
        !!(
          node &&
          (node.hasState?.("selected") || node.hasState?.("active") || node.hasState?.("hover"))
        );
      return nodeHot(source) || nodeHot(target);
    }

    _computeEdgeImportance(model = {}) {
      const forward = Math.abs(Number(model.forward_amount) || 0);
      const reverse = Math.abs(Number(model.reverse_amount) || 0);
      const directionalAmount = forward + reverse;
      const ioAmount = Math.abs(Number(model.in_amount) || 0) + Math.abs(Number(model.out_amount) || 0);
      const amount = Math.abs(Number(model.amount) || 0);
      const count = Math.abs(Number(model.count) || 0);
      const amountValue = directionalAmount || ioAmount || amount;
      const amountScore = amountValue > 0 ? clamp(Math.log1p(amountValue) / 13.5, 0, 1) : 0;
      const countScore = count > 0 ? clamp(Math.log1p(count) / 5.2, 0, 1) : 0;
      const hint = Number(model.edgeImportance);
      if (Number.isFinite(hint)) return clamp(hint, 0, 1);
      const score = Math.max(amountScore, countScore);
      // Keep unlabeled / no-metric links visible enough instead of disappearing abruptly.
      return score > 0 ? score : 0.5;
    }

    _computeEdgeAbstractionAlpha(edge, model = null) {
      if (!edge) return 1;
      if (this._isEdgeInFocus(edge)) return 1;
      const zoom = Math.max(Number(this._scale) || 1, 0.0001);
      const span = Math.max(0.0001, EDGE_ABSTRACT_FADE_START - EDGE_ABSTRACT_FADE_END);
      const tRaw = clamp((zoom - EDGE_ABSTRACT_FADE_END) / span, 0, 1);
      const t = tRaw * tRaw * (3 - 2 * tRaw);
      const fade = 1 - t;
      if (fade <= EPS) return 1;
      const m = model || edge.getModel?.() || {};
      if (asBool(m.__keepVisible, false) || asBool(m.keepVisible, false)) {
        return 1 - fade * 0.24;
      }
      const importance = this._computeEdgeImportance(m);
      const sDeg = edge.getSource?.()?.getEdges?.()?.length || 0;
      const tDeg = edge.getTarget?.()?.getEdges?.()?.length || 0;
      const hubDegree = Math.max(1, sDeg, tDeg);
      const hubPenalty = 1 / Math.pow(hubDegree, 0.28);
      const targetAlphaFactor = clamp(
        (0.24 + 0.76 * importance) * (0.52 + 0.48 * hubPenalty),
        EDGE_ABSTRACT_ALPHA_MIN,
        1
      );
      return 1 - fade * (1 - targetAlphaFactor);
    }

    _snapWorldToPixel(x, y) {
      const scale = Math.max(this._scale || 1, 0.0001);
      const dpr = this._pixelRatio || 1;
      const tx = this._translate?.x || 0;
      const ty = this._translate?.y || 0;
      const sx = (x * scale + tx) * dpr;
      const sy = (y * scale + ty) * dpr;
      const rx = Math.round(sx);
      const ry = Math.round(sy);
      return {
        x: (rx / dpr - tx) / scale,
        y: (ry / dpr - ty) / scale,
      };
    }

    _canUseGpuText() {
      if (!this._useGpuText || !this.gl) return false;
      const info = this._programInfo?.text;
      if (info && info.linked === false) return false;
      return true;
    }

    _shouldUseGpuText() {
      return this._canUseGpuText();
    }

    _estimateLabelCount() {
      let count = 0;
      const nodes =
        this._nodes && this._nodes.length
          ? this._nodes.map((n) => n.getModel())
          : Array.isArray(this._data?.nodes)
            ? this._data.nodes
            : [];
      const edges =
        this._edges && this._edges.length
          ? this._edges.map((e) => e.getModel())
          : Array.isArray(this._data?.edges)
            ? this._data.edges
            : [];
      edges.forEach((m) => {
        if (!m) return;
        if (m.label) count += 1;
        if (m.labelTop) count += 1;
        if (m.labelBottom) count += 1;
      });
      nodes.forEach((m) => {
        if (!m) return;
        const hideNodeLabel = asBool(m.nodeLabelHidden, false);
        const name = hideNodeLabel ? "" : String(m.name || "").trim();
        const title = hideNodeLabel ? "" : String(m.title || m.label || m.id || "").trim();
        const displayId = hideNodeLabel ? "" : String(m.displayId || m.display_id || "").trim();
        if (name || title) count += 1;
        if (displayId) count += 1;
        const iconSymbol = String(m.iconSymbol || "").trim();
        if (iconSymbol) count += 1;
        else if (m.icon) count += 1;
      });
      this._labelCountEstimate = count;
      return count;
    }

    _applyTextModePolicy() {
      if (!this._canUseGpuText()) {
        this._setTextMode();
        return;
      }
      this._setTextMode();
    }

    _setTextMode() {
      if (this._textMode === "gpu") return;
      this._textMode = "gpu";
      this._labelsDirty = true;
      this._textDirty = true;
      this._scheduleRender();
    }

    _beginInteraction() {
      this._interactionActive = true;
    }

    _endInteraction() {
      this._interactionActive = false;
    }

    _pulseWave(now) {
      const elapsed = Math.max(0, (now - this._pulseStart) / 1000);
      const speed = 0.95;
      const phase = (elapsed * speed) % 1;
      // Ping-pong pulse: expand then contract in one cycle.
      const t = phase < 0.5 ? phase * 2 : (1 - phase) * 2;
      return t * t * (3 - 2 * t);
    }

    _selectedDensity(selectedCount) {
      return clamp((Number(selectedCount) - 1) / Math.max(1, SELECTED_PULSE_LIMIT - 1), 0, 1);
    }

    _buildHaloEntries(m, states, highlight, sizeScale, selectedCount, enablePulse, pulseWave) {
      const r = (Number(m?.r) || 18) * sizeScale;
      if (!states || !Array.isArray(states)) return [];
      const isSelected = states.includes("selected") || states.includes("active");
      if (isSelected) {
        const density = this._selectedDensity(selectedCount);
        const baseRadius = r + (3.05 + density * 0.55) * sizeScale;
        const baseWidth = (2.8 - density * 0.45) * sizeScale;
        const baseAlpha = 0.82 - density * 0.2;
        if (!enablePulse) {
          return [
            {
              x: m.x,
              y: m.y,
              r: baseRadius,
              stroke: highlight,
              lineWidth: baseWidth,
              fill: [0, 0, 0, 0],
              alpha: baseAlpha,
            },
          ];
        }
        // <=30: single-ring smooth breathing (no multi-layer moving rings).
        const wave = clamp(Number(pulseWave) || 0, 0, 1);
        // Single-direction pulse: expand only in-cycle, no in-cycle shrink.
        const ringRadius = baseRadius + wave * 2.4 * sizeScale;
        const ringWidth = Math.max(0.8 * sizeScale, baseWidth);
        const ringAlpha = clamp(baseAlpha, 0.16, 0.98);
        return [
          {
            x: m.x,
            y: m.y,
            r: ringRadius,
            stroke: highlight,
            lineWidth: ringWidth,
            fill: [0, 0, 0, 0],
            alpha: ringAlpha,
          },
        ];
      }
      if (states.includes("hover")) {
        return [
          {
            x: m.x,
            y: m.y,
            r: r + 2.8 * sizeScale,
            stroke: highlight,
            lineWidth: 1.8 * sizeScale,
            fill: [0, 0, 0, 0],
            alpha: 0.3,
          },
        ];
      }
      return [];
    }

    _renderNow() {
      if (this._destroyed) return;
      if (!this.gl) return;
      if (this._dirty) {
        this._buildItems();
        this._dirty = false;
        this._edgeGeomDirty = true;
        this._labelsDirty = true;
        this._textDirty = true;
        if (!this._interactionActive) {
          this._applyTextModePolicy("data");
        }
      }
      const sizeScale = this._getSizeScale();
      if (Math.abs(sizeScale - this._lastSizeScale) > 0.0005) {
        this._lastSizeScale = sizeScale;
        this._positionsDirty = true;
        this._edgeGeomDirty = true;
        this._haloDirty = true;
        this._labelsDirty = true;
        this._pickDataDirty = true;
        this._pickGridDirty = true;
        this._edgeGridDirty = true;
      }
      if (!this._interactionActive) {
        this._syncPixelRatio();
      }
      if (this._positionsDirty) {
        const allowPartial =
          !this._forceFullBufferRefresh &&
          (this._partialNodes || this._partialEdges) &&
          (!this._edgeGeomDirty || (this._edgeWorkerBusy && this._interactionActive));
        if (allowPartial) {
          this._updateBuffersPartial(this._partialNodes, this._partialEdges);
        } else {
          const edgesReady = this._updateBuffers();
          if (edgesReady) this._edgeGeomDirty = false;
          this._forceFullBufferRefresh = false;
        }
        this._positionsDirty = false;
        this._partialNodes = null;
        this._partialEdges = null;
      } else if (this._haloDirty && this._shadowBuffer) {
        this._rebuildShadowData();
        const gl = this.gl;
        gl.bindBuffer(gl.ARRAY_BUFFER, this._shadowBuffer);
        gl.bufferData(
          gl.ARRAY_BUFFER,
          this._shadowData.subarray(0, this._shadowCount * 12),
          gl.DYNAMIC_DRAW
        );
        this._haloDirty = false;
      }
      this._drawScene();
      const drawLabels = true;
      const useGpuText = this._shouldUseGpuText();
      if (useGpuText) {
        if (drawLabels) {
          if (this._labelsDirty) {
            this._textDirty = true;
            this._buildTextBatches();
            this._labelsDirty = false;
          }
          this._drawTextBatches();
        }
      } else if (drawLabels && this._labelsDirty) {
        this._labelsDirty = false;
      }
      if (this._pulseActive) {
        this._haloDirty = true;
        this._scheduleRender();
      }
    }

    _computeEdgeGeometry(edge) {
      const m = edge.getModel();
      const sm = edge.getSource().getModel();
      const tm = edge.getTarget().getModel();
      const sx = Number(sm.x) || 0;
      const sy = Number(sm.y) || 0;
      const tx = Number(tm.x) || 0;
      const ty = Number(tm.y) || 0;
      const sizeScale = this._getSizeScale();
      const routeMode = String(m.edgeRoute || m.route || "").toLowerCase();
      const isDoubleMode = String(m.mode || "").toLowerCase() === "double";
      const useOrthogonalRoute = !isDoubleMode && routeMode === "orthogonal";
      const sr = (Number(sm.r) || 18) * sizeScale;
      const tr = (Number(tm.r) || 18) * sizeScale;
      const sPad = (() => {
        const stroke = Number(sm.lineWidth ?? 2) || 2;
        const base = Math.max(0.9, stroke / 2 + 0.4);
        const selected = edge.getSource()?.hasState?.("selected") || edge.getSource()?.hasState?.("active");
        const haloPad = useOrthogonalRoute ? 0 : selected ? 1.8 : 0;
        return (base + haloPad) * sizeScale;
      })();
      const tPad = (() => {
        const stroke = Number(tm.lineWidth ?? 2) || 2;
        const base = Math.max(0.9, stroke / 2 + 0.4);
        const selected = edge.getTarget()?.hasState?.("selected") || edge.getTarget()?.hasState?.("active");
        const haloPad = useOrthogonalRoute ? 0 : selected ? 1.8 : 0;
        return (base + haloPad) * sizeScale;
      })();
      const baseStroke = parseColor(m.stroke || "rgba(2,6,23,.75)");
      const baseLineWidth = (Number(m.lineWidth) || 2) * sizeScale;
      const states = edge.getStates();
      const isSelected = states.includes("selected") || states.includes("active");
      const isHover = !isSelected && states.includes("hover");
      let edgeStroke = baseStroke;
      let edgeLineWidth = baseLineWidth;
      if (isSelected) {
        edgeStroke = mixColor(baseStroke, parseColor(HIGHLIGHT_COLOR), 0.6);
        edgeLineWidth = baseLineWidth + 2 * sizeScale;
      } else if (isHover) {
        edgeStroke = mixColor(baseStroke, parseColor(HIGHLIGHT_COLOR), 0.35);
        edgeLineWidth = baseLineWidth + 1 * sizeScale;
      }
      const backflow = !!(m.layoutBackflow || m.__layoutBackflow);
      if (backflow) {
        edgeStroke = mixColor(edgeStroke, parseColor("#f59e0b"), isSelected ? 0.24 : 0.4);
      }
      const abstractAlpha = this._computeEdgeAbstractionAlpha(edge, m);
      if (abstractAlpha < 0.999) {
        edgeStroke = (Array.isArray(edgeStroke) ? edgeStroke : parseColor(edgeStroke)).slice();
        edgeStroke[3] = clamp((Number(edgeStroke[3]) || 1) * abstractAlpha, 0.015, 1);
      }
      let edgeDash = resolveDash(m.edgeDash || m.edgeDashStyle, edgeLineWidth);
      if (backflow) {
        edgeDash = {
          type: 1,
          size: Math.max(4, edgeLineWidth * 2.7),
          gap: Math.max(3, edgeLineWidth * 1.9),
        };
      }
      const segments = [];
      const arrows = [];
      const edgeOffset = (Number(m.edgeOffset) || 0) * sizeScale;
      const arrowMode = m.edgeArrow || (m.showArrow === false ? "none" : "end");
      const showArrow = m.showArrow !== false && arrowMode !== "none";
      const arrowSize = Math.max((Number(m.arrowSize) || 10) * sizeScale, edgeLineWidth * 3);
      const arrowTrim = computeArrowLineTrim(arrowSize, edgeLineWidth, sizeScale);
      const baseFont = clamp(Number(m.fontSize ?? 13), 8, 36);
      const labelSize = Math.max(8, baseFont - 1);
      const labelPad = clamp(Math.round(labelSize * 0.42), 3, 10) * sizeScale;
      const labelOffsetY = resolveEdgeLabelYOffset(m, sizeScale);
      const measureLabel = (text, size = labelSize) => {
        if (!text) return { w: 0, h: 0 };
        const w = Math.max(24, estimateTextWidth(text, size)) * sizeScale;
        const h = Math.max(12, size + 6) * sizeScale;
        return { w, h };
      };
      const shiftLabelPointY = (point) => {
        if (!point || !labelOffsetY) return point;
        return { x: Number(point.x) || 0, y: (Number(point.y) || 0) + labelOffsetY };
      };
      const baseDir = normalizeVector(tx - sx, ty - sy);
      const basePerp = { x: -baseDir.y, y: baseDir.x };
      const sx0 = sx + basePerp.x * edgeOffset;
      const sy0 = sy + basePerp.y * edgeOffset;
      const tx0 = tx + basePerp.x * edgeOffset;
      const ty0 = ty + basePerp.y * edgeOffset;
      const edgeLabelMaxWorldWidth = (size = labelSize) => {
        const distanceWorld = Math.hypot(tx0 - sx0, ty0 - sy0);
        if (!Number.isFinite(distanceWorld) || distanceWorld <= EPS) return Infinity;
        const sourceRadiusScreen = Math.max(8, Number(sm.r) || 18);
        const targetRadiusScreen = Math.max(8, Number(tm.r) || 18);
        const labelPaddingScreen = Math.max(16, Number(size) * 1.25);
        const availableScreen =
          distanceWorld / Math.max(sizeScale, EPS) -
          sourceRadiusScreen -
          targetRadiusScreen -
          labelPaddingScreen;
        return Math.max(0, availableScreen) * sizeScale;
      };
      const displayEdgeLabel = (text, size = labelSize) => {
        const maxWorldWidth = edgeLabelMaxWorldWidth(size);
        return truncateTextByWorldWidth(text, size, maxWorldWidth, sizeScale);
      };
      const measureDisplayLabel = (text, size = labelSize) => measureLabel(displayEdgeLabel(text, size), size);
      const outPad = (Number(m.outPad ?? 0.8) || 0) * sizeScale;
      const holePad = (Number(m.holePad ?? 2) || 0) * sizeScale;
      if (m.source === m.target) {
        const r = (Number(sm.r) || 18) * sizeScale;
        const loopScale = Number(m.loopScale ?? 2) || 2;
        const label = String(m.label || "");
        const outPadSelf = (Number(m.outPad ?? 1.2) || 0) * sizeScale;
        const labelWidth = measureLabel(label, baseFont).w || 0;
        const baseR = r + outPadSelf + sPad;
        const endR = baseR + 1 * sizeScale;
        const peak = Math.max(baseR * 0.75 * loopScale, baseFont * 1.2 * sizeScale);
        const rx = Math.max(baseR * 2.8 * loopScale, baseR + labelWidth * 1.2);
        const cx = sx0;
        const cy = sy0;
        const pStart = { x: cx + endR, y: cy };
        const pEnd = { x: cx - endR, y: cy };
        const ctrlY = cy - peak * 1.15;
        const ctrl1 = { x: cx + rx, y: ctrlY };
        const ctrl2 = { x: cx - rx, y: ctrlY };
        const endDir = normalizeVector(pEnd.x - ctrl2.x, pEnd.y - ctrl2.y);
        const startDir = normalizeVector(ctrl1.x - pStart.x, ctrl1.y - pStart.y);
        const trimEnd = showArrow && (arrowMode === "end" || arrowMode === "both") ? arrowTrim : 0;
        const trimStart = showArrow && (arrowMode === "start" || arrowMode === "both") ? arrowTrim : 0;
        const pStartLine = trimStart ? { x: pStart.x + startDir.x * trimStart, y: pStart.y + startDir.y * trimStart } : pStart;
        const pEndLine = trimEnd ? { x: pEnd.x - endDir.x * trimEnd, y: pEnd.y - endDir.y * trimEnd } : pEnd;
        const steps = this._edgeLiteMode ? 14 : 22;
        const cubicAt = (p0, p1, p2, p3, t) => {
          const it = 1 - t;
          const b0 = it * it * it;
          const b1 = 3 * it * it * t;
          const b2 = 3 * it * t * t;
          const b3 = t * t * t;
          return {
            x: p0.x * b0 + p1.x * b1 + p2.x * b2 + p3.x * b3,
            y: p0.y * b0 + p1.y * b1 + p2.y * b2 + p3.y * b3,
          };
        };
        let prev = pStartLine;
        for (let i = 1; i <= steps; i += 1) {
          const t = i / steps;
          let pt = cubicAt(pStartLine, ctrl1, ctrl2, pEndLine, t);
          if (i === steps) pt = pEndLine;
          segments.push({
            sx: prev.x,
            sy: prev.y,
            tx: pt.x,
            ty: pt.y,
            stroke: edgeStroke,
            lineWidth: edgeLineWidth,
            dash: edgeDash,
          });
          prev = pt;
        }
        if (showArrow) {
          if (arrowMode === "end" || arrowMode === "both") {
            arrows.push({ x: pEnd.x, y: pEnd.y, dir: endDir, size: arrowSize, color: edgeStroke });
          }
          if (arrowMode === "start" || arrowMode === "both") {
            arrows.push({ x: pStart.x, y: pStart.y, dir: startDir, size: arrowSize, color: edgeStroke });
          }
        }
      } else {
        const isDouble = isDoubleMode;
        const route = routeMode;
        const useOrthogonal = useOrthogonalRoute;
        const orthAxis = String(m.orthAxis || "").toLowerCase() === "vertical" ? "vertical" : "horizontal";
        const labelAnchorMode = String(m.edgeLabelAnchorMode || m.edgeLabelAnchor || m.labelAnchor || "")
          .trim()
          .toLowerCase();
        const straightGapAnchor = useOrthogonal && labelAnchorMode === "center-snap";
        const routingLineWidth = useOrthogonal ? baseLineWidth : edgeLineWidth;
        const extraS = outPad + holePad + sPad;
        const extraT = outPad + holePad + tPad;
        const endpoints = useOrthogonal
          ? computeOrthogonalEndpoints(sx0, sy0, tx0, ty0, sr, tr, routingLineWidth, extraS, extraT, orthAxis)
          : computeEdgeEndpoints(sx0, sy0, tx0, ty0, sr, tr, edgeLineWidth, extraS, extraT);
        const sxTip = endpoints.sx;
        const syTip = endpoints.sy;
        const txTip = endpoints.tx;
        const tyTip = endpoints.ty;
        const dir = normalizeVector(txTip - sxTip, tyTip - syTip);
        const perp = { x: -dir.y, y: dir.x };
        const absUx = Math.abs(dir.x);
        const absUy = Math.abs(dir.y);
        if (isDouble) {
          const lanePref =
            String(m.__selectedLane || m.__hitLane || "top").toLowerCase() === "bottom" ? "bottom" : "top";
          const topStroke = isSelected && lanePref !== "top" ? baseStroke : edgeStroke;
          const botStroke = isSelected && lanePref !== "bottom" ? baseStroke : edgeStroke;
          const topLineWidth = isSelected && lanePref !== "top" ? baseLineWidth : edgeLineWidth;
          const botLineWidth = isSelected && lanePref !== "bottom" ? baseLineWidth : edgeLineWidth;
          const topDash = resolveDash(m.edgeDash || m.edgeDashStyle, topLineWidth);
          const botDash = resolveDash(m.edgeDash || m.edgeDashStyle, botLineWidth);
          const gap = resolveDoubleGap(m, sizeScale);
          const half = gap / 2;
          const sTop = { x: sx0 + perp.x * half, y: sy0 + perp.y * half };
          const tTop = { x: tx0 + perp.x * half, y: ty0 + perp.y * half };
          const sBot = { x: sx0 - perp.x * half, y: sy0 - perp.y * half };
          const tBot = { x: tx0 - perp.x * half, y: ty0 - perp.y * half };
          const sOutTop = { x: sxTip + perp.x * half, y: syTip + perp.y * half };
          const tOutTop = { x: txTip + perp.x * half, y: tyTip + perp.y * half };
          const sOutBot = { x: sxTip - perp.x * half, y: syTip - perp.y * half };
          const tOutBot = { x: txTip - perp.x * half, y: tyTip - perp.y * half };
          const showEnd = arrowMode === "end" || arrowMode === "both";
          const showStart = arrowMode === "start" || arrowMode === "both";
          const topEndTrim = showArrow && showEnd ? arrowTrim : 0;
          const botEndTrim = showArrow && showStart ? arrowTrim : 0;
          const topStart = { x: sOutTop.x, y: sOutTop.y };
          const topEnd = { x: tOutTop.x - dir.x * topEndTrim, y: tOutTop.y - dir.y * topEndTrim };
          const botStart = { x: tOutBot.x, y: tOutBot.y };
          const botEnd = { x: sOutBot.x + dir.x * botEndTrim, y: sOutBot.y + dir.y * botEndTrim };
          const labelTop = String(m.labelTop || "");
          const labelBot = String(m.labelBottom || "");
          const topMid = { x: (sTop.x + tTop.x) / 2, y: (sTop.y + tTop.y) / 2 };
          const botMid = { x: (sBot.x + tBot.x) / 2, y: (sBot.y + tBot.y) / 2 };
          const topGapCenter = shiftLabelPointY(topMid);
          const botGapCenter = shiftLabelPointY(botMid);
          const topDim = measureDisplayLabel(labelTop);
          const botDim = measureDisplayLabel(labelBot);
          const topGap = topDim.w > 0 ? computeLabelSpan(absUx, absUy, topDim.w, topDim.h) + labelPad * 2 : 0;
          const botGap = botDim.w > 0 ? computeLabelSpan(absUx, absUy, botDim.w, botDim.h) + labelPad * 2 : 0;
          buildGapSegments(topStart, topEnd, topGap, topGapCenter).forEach((seg) => {
            segments.push({
              sx: seg.sx,
              sy: seg.sy,
              tx: seg.tx,
              ty: seg.ty,
              stroke: topStroke,
              lineWidth: topLineWidth,
              dash: topDash,
            });
          });
          buildGapSegments(botStart, botEnd, botGap, botGapCenter).forEach((seg) => {
            segments.push({
              sx: seg.sx,
              sy: seg.sy,
              tx: seg.tx,
              ty: seg.ty,
              stroke: botStroke,
              lineWidth: botLineWidth,
              dash: botDash,
            });
          });
          if (showArrow) {
            if (showEnd) arrows.push({ x: tOutTop.x, y: tOutTop.y, dir, size: arrowSize, color: topStroke });
            if (showStart)
              arrows.push({
                x: sOutBot.x,
                y: sOutBot.y,
                dir: { x: -dir.x, y: -dir.y },
                size: arrowSize,
                color: botStroke,
              });
          }
        } else {
          const showEnd = arrowMode === "end" || arrowMode === "both";
          const showStart = arrowMode === "start" || arrowMode === "both";
          const label = String(m.label || "");
          const labelDim = measureDisplayLabel(label);
          const baseGapLen = labelDim.w > 0 ? computeLabelSpan(absUx, absUy, labelDim.w, labelDim.h) + labelPad * 2 : 0;
          const midAnchor = computeSingleEdgeLabelAnchor(edge, sizeScale);
          const mid =
            midAnchor && Number.isFinite(midAnchor.x) && Number.isFinite(midAnchor.y)
              ? midAnchor
              : { x: (sx0 + tx0) / 2, y: (sy0 + ty0) / 2 };
          const gapCenter = shiftLabelPointY(mid);
          if (straightGapAnchor) {
            const gapLen = baseGapLen;
            const startTrim = showArrow && showStart ? arrowTrim : 0;
            const endTrim = showArrow && showEnd ? arrowTrim : 0;
            const sLine = { x: sxTip + dir.x * startTrim, y: syTip + dir.y * startTrim };
            const tLine = { x: txTip - dir.x * endTrim, y: tyTip - dir.y * endTrim };
            buildGapSegments(sLine, tLine, gapLen, gapCenter).forEach((seg) => {
              segments.push({
                sx: seg.sx,
                sy: seg.sy,
                tx: seg.tx,
                ty: seg.ty,
                stroke: edgeStroke,
                lineWidth: edgeLineWidth,
                dash: edgeDash,
              });
            });
            if (showArrow) {
              if (showEnd) arrows.push({ x: txTip, y: tyTip, dir, size: arrowSize, color: edgeStroke });
              if (showStart)
                arrows.push({
                  x: sxTip,
                  y: syTip,
                  dir: { x: -dir.x, y: -dir.y },
                  size: arrowSize,
                  color: edgeStroke,
                });
            }
          } else if (useOrthogonal) {
            const axis = orthAxis;
            const bias = Number(m.orthBias);
            const startTrim = showArrow && showStart ? arrowTrim : 0;
            const endTrim = showArrow && showEnd ? arrowTrim : 0;
            const routeData = buildOrthogonalRouteData(
              { x: sxTip, y: syTip },
              { x: txTip, y: tyTip },
              axis,
              bias,
              startTrim,
              endTrim,
              axis,
              Number.isFinite(Number(m.orthSharedCoord)) ? Number(m.orthSharedCoord) : null
            );
            let gapLen = baseGapLen;
            const segIdx = resolveOrthogonalLabelSegmentIndex(edge, routeData, axis);
            if (labelDim.w > 0 && segIdx != null) {
              const labelSeg = routeData.segments?.[segIdx];
              if (labelSeg) {
                const segDir = normalizeVector(labelSeg.tx - labelSeg.sx, labelSeg.ty - labelSeg.sy);
                const segUx = Math.abs(segDir.x);
                const segUy = Math.abs(segDir.y);
                gapLen = computeLabelSpan(segUx, segUy, labelDim.w, labelDim.h) + labelPad * 2;
              }
            }
            const picked = resolveOrthogonalLabelPoint(edge, routeData, axis, {
              ...resolveOrthogonalLabelAnchorOpts(m, axis, sizeScale),
            });
            const sharedAnchor = computeSingleEdgeLabelAnchor(edge, sizeScale);
            const rawLabelPoint = picked?.point || routeData.labelPoint;
            const labelSide = resolveLabelSide(edge, axis);
            const fallbackLabelPoint = alignLabelColumnPoint(
              rawLabelPoint,
              edge,
              labelSide,
              labelDim.w,
              axis === "horizontal",
              axis === "horizontal" ? 0 : 0.5
            );
            const labelPoint =
              sharedAnchor && Number.isFinite(sharedAnchor.x) && Number.isFinite(sharedAnchor.y)
                ? sharedAnchor
                : fallbackLabelPoint;
            const routeGapCenter = shiftLabelPointY(labelPoint);
            const drawSegsRaw = applyGapToSegments(routeData.segments, gapLen, routeGapCenter, {
              pickIndex: picked?.index ?? segIdx,
              preferAxis: axis,
            });
            const drawSegs = sealPolylineJoints(drawSegsRaw, edgeLineWidth);
            drawSegs.forEach((seg) => {
              segments.push({
                sx: seg.sx,
                sy: seg.sy,
                tx: seg.tx,
                ty: seg.ty,
                stroke: edgeStroke,
                lineWidth: edgeLineWidth,
                dash: edgeDash,
              });
            });
            const startDir = routeData.startDir;
            const endDir = routeData.endDir;
            if (showArrow) {
              if (showEnd) arrows.push({ x: txTip, y: tyTip, dir: endDir, size: arrowSize, color: edgeStroke });
              if (showStart)
                arrows.push({
                  x: sxTip,
                  y: syTip,
                  dir: { x: -startDir.x, y: -startDir.y },
                  size: arrowSize,
                  color: edgeStroke,
                });
            }
          } else {
            const gapLen = baseGapLen;
            const startTrim = showArrow && showStart ? arrowTrim : 0;
            const endTrim = showArrow && showEnd ? arrowTrim : 0;
            const sLine = { x: sxTip + dir.x * startTrim, y: syTip + dir.y * startTrim };
            const tLine = { x: txTip - dir.x * endTrim, y: tyTip - dir.y * endTrim };
            buildGapSegments(sLine, tLine, gapLen, gapCenter).forEach((seg) => {
              segments.push({
                sx: seg.sx,
                sy: seg.sy,
                tx: seg.tx,
                ty: seg.ty,
                stroke: edgeStroke,
                lineWidth: edgeLineWidth,
                dash: edgeDash,
              });
            });
            if (showArrow) {
              if (showEnd) arrows.push({ x: txTip, y: tyTip, dir, size: arrowSize, color: edgeStroke });
              if (showStart)
                arrows.push({
                  x: sxTip,
                  y: syTip,
                  dir: { x: -dir.x, y: -dir.y },
                  size: arrowSize,
                  color: edgeStroke,
                });
            }
          }
        }
      }
      if (segments.length && m.source !== m.target) {
        const hasAxisAligned = segments.some((seg) => {
          const dx = Math.abs((Number(seg?.tx) || 0) - (Number(seg?.sx) || 0));
          const dy = Math.abs((Number(seg?.ty) || 0) - (Number(seg?.sy) || 0));
          return dx <= EPS || dy <= EPS;
        });
        if (hasAxisAligned) {
          const labelAvoidRects = [...buildNodeLabelAvoidRects(sm, sizeScale), ...buildNodeLabelAvoidRects(tm, sizeScale)];
          if (labelAvoidRects.length) {
            const clipped = applyLabelAvoidRectsToSegments(segments, labelAvoidRects);
            if (clipped.length) {
              segments.length = 0;
              clipped.forEach((seg) => segments.push(seg));
            }
          }
        }
      }
      return { segments, arrows };
    }

    _writeEdgeGeometry(edge, segStart, segCount, arrowStart, arrowCount) {
      const geom = this._computeEdgeGeometry(edge);
      if (segCount !== geom.segments.length || arrowCount !== geom.arrows.length) return false;
      geom.segments.forEach((seg, i) => {
        const idx = (segStart + i) * 12;
        this._edgeData[idx] = seg.sx;
        this._edgeData[idx + 1] = seg.sy;
        this._edgeData[idx + 2] = seg.tx;
        this._edgeData[idx + 3] = seg.ty;
        this._edgeData[idx + 4] = seg.lineWidth;
        this._edgeData[idx + 5] = seg.stroke[0];
        this._edgeData[idx + 6] = seg.stroke[1];
        this._edgeData[idx + 7] = seg.stroke[2];
        this._edgeData[idx + 8] = seg.stroke[3];
        this._edgeData[idx + 9] = seg.dash.type;
        this._edgeData[idx + 10] = seg.dash.size;
        this._edgeData[idx + 11] = seg.dash.gap;
      });
      geom.arrows.forEach((arrow, i) => {
        const idx = (arrowStart + i) * 9;
        this._arrowData[idx] = arrow.x;
        this._arrowData[idx + 1] = arrow.y;
        this._arrowData[idx + 2] = arrow.dir.x;
        this._arrowData[idx + 3] = arrow.dir.y;
        this._arrowData[idx + 4] = arrow.size;
        this._arrowData[idx + 5] = arrow.color[0];
        this._arrowData[idx + 6] = arrow.color[1];
        this._arrowData[idx + 7] = arrow.color[2];
        this._arrowData[idx + 8] = arrow.color[3];
      });
      return true;
    }

    _updateBuffers() {
      const nodes = this._nodes;
      const edges = this._edges;
      const gl = this.gl;
      if (!gl) return false;
      if (this._pickGridDirty) {
        this._rebuildPickGrid();
        this._pickGridDirty = false;
      }
      if (this._shadowNodesDirty) {
        this._shadowNodes.clear();
        nodes.forEach((node) => {
          const m = node.getModel();
          if (m?.nodeShadow) this._shadowNodes.add(node);
        });
        this._shadowNodesDirty = false;
      }
      if (this._edgeGridDirty) {
        this._rebuildEdgeGrid();
      }

      // Nodes
      const nodeStride = 15;
      const shadowStride = 15;
      const highlight = parseColor(HIGHLIGHT_COLOR);
      let nodeCount = nodes.length;
      const now = performance.now();
      const sizeScale = this._getSizeScale();
      let pulseActive = false;
      let pulseWave = 0;
      let selectedCount = 0;
      for (let i = 0; i < nodeCount; i += 1) {
        const st = nodes[i]?.getStates?.() || [];
        if (st.includes("selected") || st.includes("active")) selectedCount += 1;
      }
      const enablePulse = selectedCount > 0 && selectedCount <= SELECTED_PULSE_LIMIT;
      const ensurePulse = () => {
        if (!enablePulse) return;
        if (!pulseActive) {
          pulseActive = true;
          if (!this._pulseActive) this._pulseStart = now;
          pulseWave = this._pulseWave(now);
        }
      };
      if (!this._nodeData || this._nodeData.length < nodeCount * nodeStride) {
        this._nodeData = new Float32Array(nodeCount * nodeStride);
      }
      let shadowEntries = [];
      let haloEntries = [];
      for (let i = 0; i < nodeCount; i += 1) {
        const node = nodes[i];
        const m = node.getModel();
        const states = node.getStates();
        const nameText = String(m.name || "").trim();
        const baseStroke = parseColor(m.stroke || "rgba(2,6,23,.82)");
        const baseFill = parseColor(m.fill || m.nodeFill || "rgba(255,255,255,0.05)");
        let stroke = baseStroke;
        let lineWidth = (Number(m.lineWidth) || 2) * sizeScale;
        const r = (Number(m.r) || 18) * sizeScale;
        const nodeDash = resolveDash(m.nodeDash || m.nodeDashStyle, lineWidth);
        const isSelected = states.includes("selected") || states.includes("active");
        if (isSelected && enablePulse) ensurePulse();
        const halo = this._buildHaloEntries(
          m,
          states,
          highlight,
          sizeScale,
          selectedCount,
          enablePulse,
          pulseWave
        );
        if (halo.length) haloEntries.push(...halo);
        const idx = i * nodeStride;
        this._nodeData[idx] = Number(m.x) || 0;
        this._nodeData[idx + 1] = Number(m.y) || 0;
        this._nodeData[idx + 2] = r;
        this._nodeData[idx + 3] = lineWidth;
        this._nodeData[idx + 4] = stroke[0];
        this._nodeData[idx + 5] = stroke[1];
        this._nodeData[idx + 6] = stroke[2];
        this._nodeData[idx + 7] = stroke[3];
        this._nodeData[idx + 8] = baseFill[0];
        this._nodeData[idx + 9] = baseFill[1];
        this._nodeData[idx + 10] = baseFill[2];
        this._nodeData[idx + 11] = baseFill[3];
        this._nodeData[idx + 12] = nodeDash.type || 0;
        this._nodeData[idx + 13] = nodeDash.size || 0;
        this._nodeData[idx + 14] = nodeDash.gap || 0;

        if (m.nodeShadow) {
          shadowEntries.push({
            x: m.x,
            y: m.y,
            r: r + 4 * sizeScale,
            stroke: [0, 0, 0, 0],
            lineWidth: 0,
            fill: [0.1, 0.15, 0.2, 0.18],
          });
        }
        if (m.drillable) {
          const dotFill = parseColor("rgba(34,197,94,.95)");
          const dotStroke = parseColor("rgba(21,128,61,.85)");
          shadowEntries.push({
            x: m.x,
            y: (Number(m.y) || 0) + r + (nameText ? 8 : 12) * sizeScale,
            r: 3 * sizeScale,
            stroke: dotStroke,
            lineWidth: 1 * sizeScale,
            fill: dotFill,
          });
        }
      }

      // Shadows + halos
      const shadowItems = [...shadowEntries, ...haloEntries];
      if (!this._shadowData || this._shadowData.length < shadowItems.length * shadowStride) {
        this._shadowData = new Float32Array(shadowItems.length * shadowStride);
      }
      shadowItems.forEach((entry, i) => {
        const idx = i * shadowStride;
        this._shadowData[idx] = Number(entry.x) || 0;
        this._shadowData[idx + 1] = Number(entry.y) || 0;
        this._shadowData[idx + 2] = Number(entry.r) || 0;
        this._shadowData[idx + 3] = Number(entry.lineWidth) || 0;
        this._shadowData[idx + 4] = entry.stroke[0] || 0;
        this._shadowData[idx + 5] = entry.stroke[1] || 0;
        this._shadowData[idx + 6] = entry.stroke[2] || 0;
        this._shadowData[idx + 7] = entry.stroke[3] != null ? entry.stroke[3] * (entry.alpha || 1) : 0;
        this._shadowData[idx + 8] = entry.fill[0] || 0;
        this._shadowData[idx + 9] = entry.fill[1] || 0;
        this._shadowData[idx + 10] = entry.fill[2] || 0;
        this._shadowData[idx + 11] = entry.fill[3] != null ? entry.fill[3] * (entry.alpha || 1) : 0;
        this._shadowData[idx + 12] = 0;
        this._shadowData[idx + 13] = 0;
        this._shadowData[idx + 14] = 0;
      });

      // Upload buffers
      gl.bindBuffer(gl.ARRAY_BUFFER, this._nodeBuffer);
      gl.bufferData(gl.ARRAY_BUFFER, this._nodeData.subarray(0, nodeCount * nodeStride), gl.DYNAMIC_DRAW);

      gl.bindBuffer(gl.ARRAY_BUFFER, this._shadowBuffer);
      gl.bufferData(
        gl.ARRAY_BUFFER,
        this._shadowData.subarray(0, shadowItems.length * shadowStride),
        gl.DYNAMIC_DRAW
      );

      const canUseWorker =
        this._edgeWorkerSupported && edges.length > this._edgeWorkerThreshold && !this._interactionActive;
      const hasComplexRoutes =
        canUseWorker &&
        edges.some((edge) => {
          const m = edge?.getModel?.() || {};
          const route = String(m.edgeRoute || m.route || "").toLowerCase();
          return route === "orthogonal" || !!(m.layoutBackflow || m.__layoutBackflow);
        });
      const useWorker = canUseWorker && !hasComplexRoutes;
      let edgesReady = true;
      if (useWorker) {
        const scheduled = this._scheduleEdgeWorker();
        if (!scheduled || this._edgeSegmentsCount === 0) {
          edgesReady = this._buildEdgeBuffersSync();
        } else {
          edgesReady = false;
        }
      } else {
        edgesReady = this._buildEdgeBuffersSync();
      }

      this._shadowCount = shadowItems.length;
      this._nodeCount = nodeCount;
      this._haloDirty = false;
      this._pulseActive = pulseActive;
      if (this._useGpuPick) {
        this._pickDataDirty = true;
        if (edgesReady) this._rebuildPickData();
      }
      return edgesReady;
    }

    _rebuildShadowData() {
      if (this._shadowNodesDirty) {
        this._shadowNodes.clear();
        this._nodes.forEach((node) => {
          const m = node.getModel();
          if (m?.nodeShadow) this._shadowNodes.add(node);
        });
        this._shadowNodesDirty = false;
      }
      const shadowStride = 12;
      const highlight = parseColor(HIGHLIGHT_COLOR);
      const shadowEntries = [];
      const haloEntries = [];
      const now = performance.now();
      const sizeScale = this._getSizeScale();
      let pulseActive = false;
      let pulseWave = 0;
      let selectedCount = 0;
      this._haloNodes.forEach((node) => {
        const st = node?.getStates?.() || [];
        if (st.includes("selected") || st.includes("active")) selectedCount += 1;
      });
      const enablePulse = selectedCount > 0 && selectedCount <= SELECTED_PULSE_LIMIT;
      const ensurePulse = () => {
        if (!enablePulse) return;
        if (!pulseActive) {
          pulseActive = true;
          if (!this._pulseActive) this._pulseStart = now;
          pulseWave = this._pulseWave(now);
        }
      };
      this._shadowNodes.forEach((node) => {
        const m = node.getModel();
        const r = (Number(m.r) || 18) * sizeScale;
        shadowEntries.push({
          x: m.x,
          y: m.y,
          r: r + 4 * sizeScale,
          stroke: [0, 0, 0, 0],
          lineWidth: 0,
          fill: [0.1, 0.15, 0.2, 0.18],
        });
      });
      this._nodes.forEach((node) => {
        const m = node.getModel();
        if (!m?.drillable) return;
        const nameText = String(m.name || "").trim();
        const r = (Number(m.r) || 18) * sizeScale;
        const dotFill = parseColor("rgba(34,197,94,.95)");
        const dotStroke = parseColor("rgba(21,128,61,.85)");
        shadowEntries.push({
          x: m.x,
          y: (Number(m.y) || 0) + r + (nameText ? 8 : 12) * sizeScale,
          r: 3 * sizeScale,
          stroke: dotStroke,
          lineWidth: 1 * sizeScale,
          fill: dotFill,
        });
      });
      this._haloNodes.forEach((node) => {
        const m = node.getModel();
        const states = node.getStates();
        const isSelected = states.includes("selected") || states.includes("active");
        if (isSelected && enablePulse) ensurePulse();
        const halo = this._buildHaloEntries(
          m,
          states,
          highlight,
          sizeScale,
          selectedCount,
          enablePulse,
          pulseWave
        );
        if (halo.length) haloEntries.push(...halo);
      });
      const shadowItems = [...shadowEntries, ...haloEntries];
      if (!this._shadowData || this._shadowData.length < shadowItems.length * shadowStride) {
        this._shadowData = new Float32Array(shadowItems.length * shadowStride);
      }
      shadowItems.forEach((entry, i) => {
        const idx = i * shadowStride;
        this._shadowData[idx] = Number(entry.x) || 0;
        this._shadowData[idx + 1] = Number(entry.y) || 0;
        this._shadowData[idx + 2] = Number(entry.r) || 0;
        this._shadowData[idx + 3] = Number(entry.lineWidth) || 0;
        this._shadowData[idx + 4] = entry.stroke[0] || 0;
        this._shadowData[idx + 5] = entry.stroke[1] || 0;
        this._shadowData[idx + 6] = entry.stroke[2] || 0;
        this._shadowData[idx + 7] = entry.stroke[3] != null ? entry.stroke[3] * (entry.alpha || 1) : 0;
        this._shadowData[idx + 8] = entry.fill[0] || 0;
        this._shadowData[idx + 9] = entry.fill[1] || 0;
        this._shadowData[idx + 10] = entry.fill[2] || 0;
        this._shadowData[idx + 11] = entry.fill[3] != null ? entry.fill[3] * (entry.alpha || 1) : 0;
      });
      this._shadowCount = shadowItems.length;
      this._pulseActive = pulseActive;
    }

    _buildEdgeBuffersSync() {
      const gl = this.gl;
      if (!gl) return false;
      const edges = this._edges || [];
      const edgeStride = 12;
      const arrowStride = 9;
      let edgeSegments = [];
      let arrowInstances = [];
      edges.forEach((edge) => {
        const geom = this._computeEdgeGeometry(edge);
        const segStart = edgeSegments.length;
        geom.segments.forEach((seg) => edgeSegments.push(seg));
        const segCount = geom.segments.length;
        const arrowStart = arrowInstances.length;
        geom.arrows.forEach((arrow) => arrowInstances.push(arrow));
        const arrowCount = geom.arrows.length;
        const model = edge.getModel();
        model.__segStart = segStart;
        model.__segCount = segCount;
        model.__arrowStart = arrowStart;
        model.__arrowCount = arrowCount;
      });

      if (!this._edgeData || this._edgeData.length < edgeSegments.length * edgeStride) {
        this._edgeData = new Float32Array(edgeSegments.length * edgeStride);
      }
      edgeSegments.forEach((seg, i) => {
        const idx = i * edgeStride;
        this._edgeData[idx] = seg.sx;
        this._edgeData[idx + 1] = seg.sy;
        this._edgeData[idx + 2] = seg.tx;
        this._edgeData[idx + 3] = seg.ty;
        this._edgeData[idx + 4] = seg.lineWidth;
        this._edgeData[idx + 5] = seg.stroke[0];
        this._edgeData[idx + 6] = seg.stroke[1];
        this._edgeData[idx + 7] = seg.stroke[2];
        this._edgeData[idx + 8] = seg.stroke[3];
        this._edgeData[idx + 9] = seg.dash.type;
        this._edgeData[idx + 10] = seg.dash.size;
        this._edgeData[idx + 11] = seg.dash.gap;
      });

      if (!this._arrowData || this._arrowData.length < arrowInstances.length * arrowStride) {
        this._arrowData = new Float32Array(arrowInstances.length * arrowStride);
      }
      arrowInstances.forEach((arrow, i) => {
        const idx = i * arrowStride;
        this._arrowData[idx] = arrow.x;
        this._arrowData[idx + 1] = arrow.y;
        this._arrowData[idx + 2] = arrow.dir.x;
        this._arrowData[idx + 3] = arrow.dir.y;
        this._arrowData[idx + 4] = arrow.size;
        this._arrowData[idx + 5] = arrow.color[0];
        this._arrowData[idx + 6] = arrow.color[1];
        this._arrowData[idx + 7] = arrow.color[2];
        this._arrowData[idx + 8] = arrow.color[3];
      });

      gl.bindBuffer(gl.ARRAY_BUFFER, this._edgeBuffer);
      gl.bufferData(gl.ARRAY_BUFFER, this._edgeData.subarray(0, edgeSegments.length * edgeStride), gl.DYNAMIC_DRAW);

      gl.bindBuffer(gl.ARRAY_BUFFER, this._arrowBuffer);
      gl.bufferData(
        gl.ARRAY_BUFFER,
        this._arrowData.subarray(0, arrowInstances.length * arrowStride),
        gl.DYNAMIC_DRAW
      );

      this._edgeSegmentsCount = edgeSegments.length;
      this._arrowCount = arrowInstances.length;
      this._markEdgeViewDirty();
      this._highlightDirty = true;
      return true;
    }

    _updateBuffersPartial(nodes, edges) {
      const gl = this.gl;
      if (!gl) return;
      if (!this._nodeData || !this._edgeData || !this._arrowData) {
        const edgesReady = this._updateBuffers();
        if (edgesReady) this._edgeGeomDirty = false;
        return;
      }
      const usePick = this._useGpuPick;
      if (usePick && (!this._nodePickData || !this._edgePickData)) {
        this._pickDataDirty = true;
      }
      const nodeStride = 15;
      const edgeStride = 12;
      const arrowStride = 9;
      const highlight = parseColor(HIGHLIGHT_COLOR);
      const sizeScale = this._getSizeScale();
      const touchedEdges = new Set(edges ? Array.from(edges) : []);
      const allowEdgeUpdate = !this._shouldDeferEdgeUpdate();
      const nodeList = Array.from(nodes || []);
      nodeList.forEach((node) => {
        const idx = node._index != null ? node._index : node.getModel()?.__nodeIndex;
        if (idx == null) return;
        const m = node.getModel();
        const baseStroke = parseColor(m.stroke || "rgba(2,6,23,.82)");
        const baseFill = parseColor(m.fill || m.nodeFill || "rgba(255,255,255,0.05)");
        const states = node.getStates();
        let stroke = baseStroke;
        let lineWidth = (Number(m.lineWidth) || 2) * sizeScale;
        const r = (Number(m.r) || 18) * sizeScale;
        const nodeDash = resolveDash(m.nodeDash || m.nodeDashStyle, lineWidth);
        if (states.includes("selected") || states.includes("active")) {
          // keep base stroke; pulse halo handled in shadow layer
        } else if (states.includes("hover")) {
          // keep base stroke; hover halo handled in shadow layer
        }
        const base = idx * nodeStride;
        this._nodeData[base] = Number(m.x) || 0;
        this._nodeData[base + 1] = Number(m.y) || 0;
        this._nodeData[base + 2] = r;
        this._nodeData[base + 3] = lineWidth;
        this._nodeData[base + 4] = stroke[0];
        this._nodeData[base + 5] = stroke[1];
        this._nodeData[base + 6] = stroke[2];
        this._nodeData[base + 7] = stroke[3];
        this._nodeData[base + 8] = baseFill[0];
        this._nodeData[base + 9] = baseFill[1];
        this._nodeData[base + 10] = baseFill[2];
        this._nodeData[base + 11] = baseFill[3];
        this._nodeData[base + 12] = nodeDash.type || 0;
        this._nodeData[base + 13] = nodeDash.size || 0;
        this._nodeData[base + 14] = nodeDash.gap || 0;
        this._updatePickGridForNode(node);
        if (allowEdgeUpdate) {
          const edges = node.getEdges ? node.getEdges() : [];
          edges.forEach((edge) => touchedEdges.add(edge));
        }
      });

      gl.bindBuffer(gl.ARRAY_BUFFER, this._nodeBuffer);
      nodeList.forEach((node) => {
        const idx = node._index != null ? node._index : node.getModel()?.__nodeIndex;
        if (idx == null) return;
        const start = idx * nodeStride;
        const sub = this._nodeData.subarray(start, start + nodeStride);
        gl.bufferSubData(gl.ARRAY_BUFFER, start * 4, sub);
      });

      if (
        usePick &&
        this._nodePickData &&
        this._pickNodeBuffer &&
        !(this._disablePickDuringInteraction && this._interactionActive)
      ) {
        const pickStride = 7;
        const pickPad = 6 * sizeScale;
        gl.bindBuffer(gl.ARRAY_BUFFER, this._pickNodeBuffer);
        nodeList.forEach((node) => {
          const idx = node._index != null ? node._index : node.getModel()?.__nodeIndex;
          if (idx == null) return;
          const m = node.getModel();
          const base = idx * pickStride;
          const pick = encodePickColor(node._pickId || idx + 1);
          this._nodePickData[base] = Number(m.x) || 0;
          this._nodePickData[base + 1] = Number(m.y) || 0;
          this._nodePickData[base + 2] = (Number(m.r) || 18) * sizeScale + pickPad;
          this._nodePickData[base + 3] = pick[0];
          this._nodePickData[base + 4] = pick[1];
          this._nodePickData[base + 5] = pick[2];
          this._nodePickData[base + 6] = pick[3];
          const sub = this._nodePickData.subarray(base, base + pickStride);
          gl.bufferSubData(gl.ARRAY_BUFFER, base * 4, sub);
        });
        this._pickRenderDirty = true;
      }

      if (this._haloDirty || this._shadowNodesDirty) {
        this._rebuildShadowData();
        gl.bindBuffer(gl.ARRAY_BUFFER, this._shadowBuffer);
        gl.bufferData(
          gl.ARRAY_BUFFER,
          this._shadowData.subarray(0, this._shadowCount * nodeStride),
          gl.DYNAMIC_DRAW
        );
        this._haloDirty = false;
      }

      let forceFull = false;
      const edgeRanges = [];
      const arrowRanges = [];
      touchedEdges.forEach((edge) => {
        const m = edge.getModel();
        const segStart = m.__segStart;
        const segCount = m.__segCount;
        const arrowStart = m.__arrowStart;
        const arrowCount = m.__arrowCount;
        if (
          segStart == null ||
          segCount == null ||
          arrowStart == null ||
          arrowCount == null ||
          segCount < 0 ||
          arrowCount < 0
        ) {
          forceFull = true;
          return;
        }
        if (!this._writeEdgeGeometry(edge, segStart, segCount, arrowStart, arrowCount)) {
          forceFull = true;
          return;
        }
        if (segCount > 0) edgeRanges.push({ start: segStart, count: segCount, edge });
        if (arrowCount > 0) arrowRanges.push({ start: arrowStart, count: arrowCount });
      });

      if (forceFull) {
        this._edgeGeomDirty = true;
        const edgesReady = this._updateBuffers();
        if (edgesReady) this._edgeGeomDirty = false;
        return;
      }

      if (!this._edgeGridDirty) {
        touchedEdges.forEach((edge) => {
          this._updateEdgeGridForEdge(edge);
        });
      }
      if (edgeRanges.length > 0) {
        this._markEdgeViewDirty();
      }

      gl.bindBuffer(gl.ARRAY_BUFFER, this._edgeBuffer);
      edgeRanges.forEach((range) => {
        const start = range.start * edgeStride;
        const end = start + range.count * edgeStride;
        const sub = this._edgeData.subarray(start, end);
        gl.bufferSubData(gl.ARRAY_BUFFER, range.start * edgeStride * 4, sub);
      });

      if (this._arrowCount > 0) {
        gl.bindBuffer(gl.ARRAY_BUFFER, this._arrowBuffer);
        arrowRanges.forEach((range) => {
          const start = range.start * arrowStride;
          const end = start + range.count * arrowStride;
          const sub = this._arrowData.subarray(start, end);
          gl.bufferSubData(gl.ARRAY_BUFFER, range.start * arrowStride * 4, sub);
        });
      }

      if (
        usePick &&
        this._edgePickData &&
        this._pickEdgeBuffer &&
        !(this._disablePickDuringInteraction && this._interactionActive)
      ) {
        const pickStride = 9;
        const scale = this._scale || 1;
        gl.bindBuffer(gl.ARRAY_BUFFER, this._pickEdgeBuffer);
        edgeRanges.forEach((range) => {
          const start = range.start;
          const count = range.count;
          const pick = encodePickColor(range.edge?._pickId || 0);
          for (let i = 0; i < count; i += 1) {
            const segIndex = start + i;
            const srcIdx = segIndex * edgeStride;
            const dstIdx = segIndex * pickStride;
            const lineWidth = this._edgeData[srcIdx + 4];
            const pickWidth = lineWidth + 6 / Math.max(scale, 0.001);
            this._edgePickData[dstIdx] = this._edgeData[srcIdx];
            this._edgePickData[dstIdx + 1] = this._edgeData[srcIdx + 1];
            this._edgePickData[dstIdx + 2] = this._edgeData[srcIdx + 2];
            this._edgePickData[dstIdx + 3] = this._edgeData[srcIdx + 3];
            this._edgePickData[dstIdx + 4] = pickWidth;
            this._edgePickData[dstIdx + 5] = pick[0];
            this._edgePickData[dstIdx + 6] = pick[1];
            this._edgePickData[dstIdx + 7] = pick[2];
            this._edgePickData[dstIdx + 8] = pick[3];
          }
          const base = start * pickStride;
          const sub = this._edgePickData.subarray(base, base + count * pickStride);
          gl.bufferSubData(gl.ARRAY_BUFFER, base * 4, sub);
        });
        this._pickRenderDirty = true;
      }
      if (this._highlightEdges.size > 0) {
        let touchedHighlight = false;
        touchedEdges.forEach((edge) => {
          if (this._highlightEdges.has(edge)) touchedHighlight = true;
        });
        if (touchedHighlight) this._highlightDirty = true;
      }
    }

    _drawScene() {
      return drawGraphScene(this, { clearFallback: WEBGL_CLEAR_FALLBACK });
    }

    _drawInstanced(mode, vertexCount, instanceCount) {
      return drawGraphInstanced(this, mode, vertexCount, instanceCount);
    }

    _bindEdgeAttributes(scale, translate, resolution, bufferOverride, contrast = 1, clearColor = null) {
      return bindEdgeAttributes(this, scale, translate, resolution, bufferOverride, contrast, clearColor);
    }

    _bindNodeAttributes(scale, translate, resolution, useShadow, contrast = 1, clearColor = null) {
      return bindNodeAttributes(this, scale, translate, resolution, useShadow, contrast, clearColor);
    }

    _bindArrowAttributes(scale, translate, resolution, bufferOverride, contrast = 1, clearColor = null) {
      return bindArrowAttributes(this, scale, translate, resolution, bufferOverride, contrast, clearColor);
    }


    _bindEvents() {
      if (!this.canvas) return;
      this._unbindEvents();
      const canvas = this.canvas;
      const bind = (type, handler, options) => {
        canvas.addEventListener(type, handler, options);
        this._boundCanvasEvents.push({ type, handler, options });
      };
      bind("mousedown", (ev) => this._onPointerDown(ev));
      bind("mousemove", (ev) => this._onPointerMove(ev));
      bind("mouseup", (ev) => this._onPointerUp(ev));
      bind("mouseleave", (ev) => this._onPointerLeave(ev));
      bind("wheel", (ev) => this._onWheel(ev), { passive: false });
      bind("dblclick", (ev) => this._onDoubleClick(ev));
      bind("contextmenu", (ev) => this._onContextMenu(ev));
    }

    _unbindEvents() {
      const canvas = this.canvas;
      if (!canvas || !Array.isArray(this._boundCanvasEvents) || !this._boundCanvasEvents.length) return;
      this._boundCanvasEvents.forEach((row) => {
        try {
          canvas.removeEventListener(row.type, row.handler, row.options);
        } catch (e) {}
      });
      this._boundCanvasEvents = [];
    }

    _eventPayload(ev, item) {
      const pt = this.getPointByClient(ev.clientX, ev.clientY);
      return {
        item,
        x: pt.x,
        y: pt.y,
        canvasX: pt.x,
        canvasY: pt.y,
        originalEvent: ev,
      };
    }

    _onPointerDown(ev) {
      if (ev.button !== 0) return;
      const node = this._pickNode(ev.clientX, ev.clientY);
      this._downPos = { x: ev.clientX, y: ev.clientY, t: Date.now(), node };
      this._dragMoved = false;
      if (this._hoverRaf) {
        cancelAnimationFrame(this._hoverRaf);
        this._hoverRaf = 0;
      }
      this._hoverEvent = null;
      if (node && this._enableDragNode) {
        this._draggingNode = node;
        this._beginInteraction("node:dragstart");
        this._deferEdgeUpdate = false;
        if (this._edges.length > this._edgeLiteThreshold) this._edgeLiteMode = true;
        this.emit("node:dragstart", this._eventPayload(ev, node));
      } else if (this._enableDragCanvas) {
        this._draggingCanvas = true;
        this._beginInteraction("canvas:dragstart");
        if (this._edges.length > this._edgeLiteThreshold) this._edgeLiteMode = true;
        this.emit("canvas:dragstart", this._eventPayload(ev, null));
      }
    }

    _onPointerMove(ev) {
      if (!this._draggingNode && !this._draggingCanvas) {
        this._scheduleHover(ev);
      }

      if (this._draggingNode && this._downPos) {
        const pt = this.getPointByClient(ev.clientX, ev.clientY);
        const model = this._draggingNode.getModel();
        if (Math.hypot(ev.clientX - this._downPos.x, ev.clientY - this._downPos.y) > 2) {
          this._dragMoved = true;
        }
        model.x = pt.x;
        model.y = pt.y;
        this._positionsDirty = true;
        this._labelsDirty = true;
        if (!this._partialNodes) this._partialNodes = new Set();
        this._partialNodes.add(this._draggingNode);
        if (!this._disablePickDuringInteraction) this._pickRenderDirty = true;
        this._scheduleRender();
        this.emit("node:drag", this._eventPayload(ev, this._draggingNode));
        this.emit("mousemove", this._eventPayload(ev, this._draggingNode));
        return;
      }

      if (this._draggingCanvas && this._downPos) {
        const dx = ev.clientX - this._downPos.x;
        const dy = ev.clientY - this._downPos.y;
        if (Math.hypot(dx, dy) > 2) this._dragMoved = true;
        this._translate.x += dx;
        this._translate.y += dy;
        this._markEdgeViewDirty();
        this._downPos.x = ev.clientX;
        this._downPos.y = ev.clientY;
        if (!this._shouldUseGpuText()) this._labelsDirty = true;
        if (!this._disablePickDuringInteraction) this._pickRenderDirty = true;
        this._scheduleRender();
        this.emit("canvas:drag", this._eventPayload(ev, null));
        this.emit("viewportchange", { type: "pan" });
        this.emit("mousemove", this._eventPayload(ev, null));
      }
    }

    _scheduleHover(ev) {
      this._hoverEvent = { x: ev.clientX, y: ev.clientY, raw: ev };
      if (this._hoverRaf) return;
      this._hoverRaf = requestAnimationFrame(() => {
        this._hoverRaf = 0;
        const info = this._hoverEvent;
        if (!info) return;
        const node = this._pickNode(info.x, info.y, { useGpu: false });
        const raw = info.raw;
        if (node !== this._lastHoverNode) {
          if (this._lastHoverNode) this.emit("node:mouseleave", this._eventPayload(raw, this._lastHoverNode));
          if (node) this.emit("node:mouseenter", this._eventPayload(raw, node));
          this._lastHoverNode = node;
        }
        if (!node && this._edges.length <= 2000) {
          const edge = this._pickEdge(info.x, info.y, { useGpu: false });
          if (edge !== this._lastHoverEdge) {
            if (this._lastHoverEdge) this.emit("edge:mouseleave", this._eventPayload(raw, this._lastHoverEdge));
            if (edge) this.emit("edge:mouseenter", this._eventPayload(raw, edge));
            this._lastHoverEdge = edge;
          }
        } else if (node && this._lastHoverEdge) {
          this.emit("edge:mouseleave", this._eventPayload(raw, this._lastHoverEdge));
          this._lastHoverEdge = null;
        }
        this.emit("mousemove", this._eventPayload(raw, node));
      });
    }

    _onPointerUp(ev) {
      const wasDragNode = !!this._draggingNode;
      const wasDragCanvas = !!this._draggingCanvas;
      const down = this._downPos;
      this._draggingNode = null;
      this._draggingCanvas = false;
      this._deferEdgeUpdate = false;
      this._edgeLiteMode = false;
      if (wasDragNode || wasDragCanvas) {
        this._endInteraction("dragend");
      } else {
        this._interactionActive = false;
      }
      if (wasDragNode) {
        this._edgeGeomDirty = true;
        this._positionsDirty = true;
        this._pickDataDirty = true;
        this._pickRenderDirty = true;
      }
      if (wasDragNode) {
        this.emit("node:dragend", this._eventPayload(ev, down?.node || null));
      }
      if (wasDragCanvas) {
        this.emit("canvas:dragend", this._eventPayload(ev, null));
      }
      if (wasDragNode || wasDragCanvas) {
        this._labelsDirty = true;
        this._renderNow();
      }
      if (!down) return;
      if (this._dragMoved) return;
      const dist = Math.hypot(ev.clientX - down.x, ev.clientY - down.y);
      if (dist > 4) return;
      const node = this._pickNode(ev.clientX, ev.clientY);
      if (node) {
        this.emit("node:click", this._eventPayload(ev, node));
      } else {
        const edge = this._pickEdge(ev.clientX, ev.clientY);
        if (edge) this.emit("edge:click", this._eventPayload(ev, edge));
        else this.emit("canvas:click", this._eventPayload(ev, null));
      }
    }

    _onPointerLeave(ev) {
      if (this._hoverRaf) {
        cancelAnimationFrame(this._hoverRaf);
        this._hoverRaf = 0;
      }
      this._hoverEvent = null;
      if (this._lastHoverNode) {
        this.emit("node:mouseleave", this._eventPayload(ev, this._lastHoverNode));
        this._lastHoverNode = null;
      }
      if (this._lastHoverEdge) {
        this.emit("edge:mouseleave", this._eventPayload(ev, this._lastHoverEdge));
        this._lastHoverEdge = null;
      }
      this.emit("canvas:mouseleave", this._eventPayload(ev, null));
    }

    _normalizeWheelDelta(ev) {
      let dx = Number(ev?.deltaX) || 0;
      let dy = Number(ev?.deltaY) || 0;
      const mode = Number(ev?.deltaMode) || 0;
      if (mode === 1) {
        dx *= 16;
        dy *= 16;
      } else if (mode === 2) {
        dx *= this.width || window.innerWidth || 1;
        dy *= this.height || window.innerHeight || 1;
      }
      return { dx, dy };
    }

    _wheelScrollContainer(container, ev, { fallbackToHorizontal = true } = {}) {
      if (!container) return false;
      const { dx, dy } = this._normalizeWheelDelta(ev);
      if (!dx && !dy) return false;

      const maxTop = Math.max(0, (container.scrollHeight || 0) - (container.clientHeight || 0));
      const maxLeft = Math.max(0, (container.scrollWidth || 0) - (container.clientWidth || 0));
      const hasY = maxTop > 0;
      const hasX = maxLeft > 0;
      if (!hasY && !hasX) return false;

      const prevTop = container.scrollTop || 0;
      const prevLeft = container.scrollLeft || 0;
      let nextTop = prevTop;
      let nextLeft = prevLeft;

      if (Math.abs(dx) > 0.01 && hasX) nextLeft += dx;
      if (Math.abs(dy) > 0.01) {
        if (ev.shiftKey && hasX) nextLeft += dy;
        else if (hasY) nextTop += dy;
        else if (fallbackToHorizontal && hasX) nextLeft += dy;
      }

      nextTop = clamp(nextTop, 0, maxTop);
      nextLeft = clamp(nextLeft, 0, maxLeft);

      const moved = nextTop !== prevTop || nextLeft !== prevLeft;
      if (moved) {
        container.scrollTop = nextTop;
        container.scrollLeft = nextLeft;
      }
      return moved;
    }

    _pointInRect(x, y, rect) {
      if (!rect) return false;
      return x >= rect.left && x <= rect.right && y >= rect.top && y <= rect.bottom;
    }

    _isWheelRouteDebugEnabled() {
      if (this._wheelRouteDebug) return true;
      try {
        return !!(window.__ANALYTIX_DEBUG_TRACE || window.__ANALYTIX_DEBUG_WHEEL_ROUTE);
      } catch (e) {
        return false;
      }
    }

    _describeWheelTarget(target) {
      const el = target && target.nodeType === 1 ? target : null;
      if (!el) return "";
      const parts = [];
      parts.push(String(el.tagName || "").toLowerCase());
      if (el.id) parts.push(`#${el.id}`);
      if (el.classList && el.classList.length) {
        const cls = Array.from(el.classList)
          .slice(0, 3)
          .join(".");
        if (cls) parts.push(`.${cls}`);
      }
      return parts.join("");
    }

    _wheelRouteLog(_kind, _payload = {}) {
      if (!this._isWheelRouteDebugEnabled()) return;
      const now = Date.now();
      const key = "wheel-route";
      if (this._wheelRouteLogKey === key && now - this._wheelRouteLogAt < 40) return;
      this._wheelRouteLogKey = key;
      this._wheelRouteLogAt = now;
      try {
        console.log("[Viz][WheelRoute] event=wheel_route");
      } catch (e) {}
    }

    _routeOverlayWheel(ev) {
      const doc = typeof document !== "undefined" ? document : null;
      if (!doc) return false;
      const targetDesc = this._describeWheelTarget(ev?.target);
      this._wheelRouteLog("check", {
        target: targetDesc,
        x: Number(ev?.clientX) || 0,
        y: Number(ev?.clientY) || 0,
        deltaX: Number(ev?.deltaX) || 0,
        deltaY: Number(ev?.deltaY) || 0,
        deltaMode: Number(ev?.deltaMode) || 0,
      });

      const edgePop = doc.getElementById("edgeInfoPopover");
      const edgeOpen = !!(edgePop && edgePop.getAttribute("aria-hidden") === "false");
      if (edgeOpen) {
        const rect = edgePop.getBoundingClientRect?.();
        if (this._pointInRect(ev.clientX, ev.clientY, rect)) {
          const list = edgePop.querySelector(".edgeTxnList");
          const moved = this._wheelScrollContainer(list, ev, { fallbackToHorizontal: true });
          this._wheelRouteLog("route", {
            route: "edgePopover",
            moved,
            target: targetDesc,
            list: !!list,
            scrollTop: Number(list?.scrollTop || 0),
            scrollLeft: Number(list?.scrollLeft || 0),
            maxTop: Math.max(0, Number(list?.scrollHeight || 0) - Number(list?.clientHeight || 0)),
            maxLeft: Math.max(0, Number(list?.scrollWidth || 0) - Number(list?.clientWidth || 0)),
          });
          ev.preventDefault();
          return true;
        }
        this._wheelRouteLog("miss", { route: "edgePopover", reason: "outsideRect", target: targetDesc });
      }

      const nodePop = doc.getElementById("nodeInfoPopover");
      const nodeOpen = !!(nodePop && nodePop.getAttribute("aria-hidden") === "false");
      if (nodeOpen) {
        const rect = nodePop.getBoundingClientRect?.();
        if (this._pointInRect(ev.clientX, ev.clientY, rect)) {
          const list = nodePop.querySelector(".nodeInfoList");
          const moved = this._wheelScrollContainer(list, ev, { fallbackToHorizontal: true });
          this._wheelRouteLog("route", {
            route: "nodePopover",
            moved,
            target: targetDesc,
            list: !!list,
            scrollTop: Number(list?.scrollTop || 0),
            scrollLeft: Number(list?.scrollLeft || 0),
            maxTop: Math.max(0, Number(list?.scrollHeight || 0) - Number(list?.clientHeight || 0)),
            maxLeft: Math.max(0, Number(list?.scrollWidth || 0) - Number(list?.clientWidth || 0)),
          });
          ev.preventDefault();
          return true;
        }
        this._wheelRouteLog("miss", { route: "nodePopover", reason: "outsideRect", target: targetDesc });
      }

      this._wheelRouteLog("miss", { route: "overlay", reason: "noOverlayHit", target: targetDesc });
      return false;
    }

    _onWheel(ev) {
      if (this._routeOverlayWheel(ev)) return;
      if (!this._enableZoomCanvas) return;
      this._wheelRouteLog("route", {
        route: "canvasZoom",
        target: this._describeWheelTarget(ev?.target),
        scale: this._scale,
      });
      ev.preventDefault();
      this._wheelActive = true;
      if (this._edges.length > this._edgeLiteThreshold) this._edgeLiteMode = true;
      const delta = ev.deltaY;
      const factor = delta < 0 ? 1.08 : 0.92;
      const next = clamp(this._scale * factor, this.minZoom, this.maxZoom);
      const rect = this.canvas.getBoundingClientRect();
      const cx = ev.clientX - rect.left;
      const cy = ev.clientY - rect.top;
      const world = this.getPointByCanvas(cx, cy);
      this._scale = next;
      this._translate.x = cx - world.x * next;
      this._translate.y = cy - world.y * next;
      this._markEdgeViewDirty();
      if (!this._shouldUseGpuText()) this._labelsDirty = true;
      if (!this._disablePickDuringInteraction) {
        this._pickRenderDirty = true;
      } else {
        if (this._wheelPickTimer) clearTimeout(this._wheelPickTimer);
        this._wheelPickTimer = 0;
        this._wheelPickTimer = setTimeout(() => {
          this._wheelActive = false;
          this._edgeLiteMode = false;
          this._wheelPickTimer = 0;
          this._pickRenderDirty = true;
          this._renderNow();
        }, 80);
      }
      const wantsGpuInteraction = this._canUseGpuText();
      if (wantsGpuInteraction) {
        this._beginInteraction("wheel");
        if (this._wheelTimer) clearTimeout(this._wheelTimer);
        this._wheelTimer = setTimeout(() => {
          this._endInteraction("wheel");
          // Node/edge labels depend on zoom + view window; force a rebuild when wheel stops.
          this._labelsDirty = true;
          if (!this._wheelPickTimer) this._wheelActive = false;
          if (!this._wheelPickTimer) this._edgeLiteMode = false;
          this._scheduleRender();
        }, 120);
      } else {
        if (this._wheelTimer) clearTimeout(this._wheelTimer);
        this._wheelTimer = setTimeout(() => {
          this._wheelActive = false;
          this._edgeLiteMode = false;
          // Non-interaction wheel path also needs a final label refresh.
          this._labelsDirty = true;
          this._scheduleRender();
        }, 120);
      }
      this._scheduleRender();
      this.emit("viewportchange", { type: "zoom" });
    }

    _onDoubleClick(ev) {
      const node = this._pickNode(ev.clientX, ev.clientY);
      if (node) {
        this.emit("node:dblclick", this._eventPayload(ev, node));
      } else {
        const edge = this._pickEdge(ev.clientX, ev.clientY);
        if (edge) this.emit("edge:dblclick", this._eventPayload(ev, edge));
      }
    }

    _onContextMenu(ev) {
      const node = this._pickNode(ev.clientX, ev.clientY);
      if (node) {
        this.emit("node:contextmenu", this._eventPayload(ev, node));
        ev.preventDefault();
      }
    }

    _pickNode(clientX, clientY, opts = {}) {
      const useGpu = opts.useGpu !== false;
      if (useGpu && this._useGpuPick && this._pickNodeProgram) {
        const cached = this._lastPickHit;
        if (!cached || cached.x !== clientX || cached.y !== clientY) {
          const hit = this._pickByGpu(clientX, clientY);
          this._lastPickHit = { x: clientX, y: clientY, hit };
        }
        const hit = this._lastPickHit?.hit;
        if (hit && hit.type === "node") return hit.item;
      }
      if (this._pickGridDirty) {
        this._rebuildPickGrid();
        this._pickGridDirty = false;
      }
      const pt = this.getPointByClient(clientX, clientY);
      const size = this._pickGridSize;
      const gx = Math.floor(pt.x / size);
      const gy = Math.floor(pt.y / size);
      const candidates = [];
      for (let dx = -1; dx <= 1; dx += 1) {
        for (let dy = -1; dy <= 1; dy += 1) {
          const key = `${gx + dx},${gy + dy}`;
          const bucket = this._pickGrid.get(key);
          if (bucket) candidates.push(...bucket);
        }
      }
      let picked = null;
      let minDist = Infinity;
      candidates.forEach((node) => {
        const m = node.getModel();
        const r = Number(m.r) || 18;
        const dx = pt.x - (Number(m.x) || 0);
        const dy = pt.y - (Number(m.y) || 0);
        const d = Math.hypot(dx, dy);
        if (d <= r && d < minDist) {
          picked = node;
          minDist = d;
        }
      });
      return picked;
    }

    _resolveEdgeLane(edge, pt) {
      if (!edge || !pt) return "";
      const m = edge.getModel?.();
      if (!m || m.source === m.target || String(m.mode || "").toLowerCase() !== "double") return "";
      const s = edge.getSource?.()?.getModel?.();
      const t = edge.getTarget?.()?.getModel?.();
      if (!s || !t) return "";
      const sx = Number(s.x) || 0;
      const sy = Number(s.y) || 0;
      const tx = Number(t.x) || 0;
      const ty = Number(t.y) || 0;
      const dir = normalizeVector(tx - sx, ty - sy);
      const perp = { x: -dir.y, y: dir.x };
      const half = resolveDoubleGap(m, 1) / 2;
      const ox = perp.x * half;
      const oy = perp.y * half;
      const dTop = pointToSegmentDistance(pt.x, pt.y, sx + ox, sy + oy, tx + ox, ty + oy);
      const dBottom = pointToSegmentDistance(pt.x, pt.y, sx - ox, sy - oy, tx - ox, ty - oy);
      return dTop <= dBottom ? "top" : "bottom";
    }

    _markEdgeLaneHit(edge, lane) {
      if (!edge || !lane) return;
      const m = edge.getModel?.();
      if (!m || String(m.mode || "").toLowerCase() !== "double") return;
      const normalized = lane === "bottom" ? "bottom" : "top";
      m.__hitLane = normalized;
      if (edge.hasState?.("selected") || edge.hasState?.("active")) {
        m.__selectedLane = normalized;
      }
      if (!this._partialEdges) this._partialEdges = new Set();
      this._partialEdges.add(edge);
      this._positionsDirty = true;
    }

    _pickEdge(clientX, clientY, opts = {}) {
      const useGpu = opts.useGpu !== false;
      if (useGpu && this._useGpuPick && this._pickEdgeProgram) {
        const cached = this._lastPickHit;
        if (!cached || cached.x !== clientX || cached.y !== clientY) {
          const hit = this._pickByGpu(clientX, clientY);
          this._lastPickHit = { x: clientX, y: clientY, hit };
        }
        const hit = this._lastPickHit?.hit;
        if (hit && hit.type === "edge") {
          const lane = this._resolveEdgeLane(hit.item, this.getPointByClient(clientX, clientY));
          if (lane) this._markEdgeLaneHit(hit.item, lane);
          return hit.item;
        }
      }
      const pt = this.getPointByClient(clientX, clientY);
      const tol = 6 / this._scale;
      let picked = null;
      let pickedLane = "";
      let minDist = Infinity;
      let candidates = null;
      if (this._edgeGridDirty) {
        this._rebuildEdgeGrid();
      }
      if (this._edgeGrid.size > 0) {
        const size = this._edgeGridSize;
        const gx = Math.floor(pt.x / size);
        const gy = Math.floor(pt.y / size);
        const set = new Set();
        for (let dx = -1; dx <= 1; dx += 1) {
          for (let dy = -1; dy <= 1; dy += 1) {
            const key = `${gx + dx},${gy + dy}`;
            const bucket = this._edgeGrid.get(key);
            if (bucket) bucket.forEach((edge) => set.add(edge));
          }
        }
        candidates = Array.from(set);
      }
      const list = candidates || this._edges;
      const edgeStride = 12;
      const edgeData = this._edgeData;
      for (let i = 0; i < list.length; i += 1) {
        const edge = list[i];
        const m = edge.getModel();
        let candidateLane = "";
        const segStart = m.__segStart;
        const segCount = m.__segCount;
        if (edgeData && segStart != null && segCount > 0) {
          for (let j = 0; j < segCount; j += 1) {
            const idx = (segStart + j) * edgeStride;
            const sx = edgeData[idx];
            const sy = edgeData[idx + 1];
            const tx = edgeData[idx + 2];
            const ty = edgeData[idx + 3];
            const lineWidth = edgeData[idx + 4] || 1;
            const dist = pointToSegmentDistance(pt.x, pt.y, sx, sy, tx, ty);
            const hitTol = Math.max(tol, lineWidth * 0.75);
            if (dist < hitTol && dist < minDist) {
              minDist = dist;
              picked = edge;
              candidateLane = this._resolveEdgeLane(edge, pt) || "";
              pickedLane = candidateLane;
            }
          }
          continue;
        }
        const s = edge.getSource().getModel();
        const t = edge.getTarget().getModel();
        const sx = Number(s.x) || 0;
        const sy = Number(s.y) || 0;
        const tx = Number(t.x) || 0;
        const ty = Number(t.y) || 0;
        if (m.source === m.target) {
          const r = Number(s.r) || 18;
          const loopR = r * 1.8;
          const offset = r * 1.2;
          const cx = sx + loopR;
          const cy = sy - loopR - offset * 0.2;
          const dist = Math.abs(Math.hypot(pt.x - cx, pt.y - cy) - loopR);
          if (dist < tol && dist < minDist) {
            minDist = dist;
            picked = edge;
          }
          continue;
        }
        let dist = pointToSegmentDistance(pt.x, pt.y, sx, sy, tx, ty);
        if (String(m.mode || "").toLowerCase() === "double") {
          const dir = normalizeVector(tx - sx, ty - sy);
          const perp = { x: -dir.y, y: dir.x };
          const gap = resolveDoubleGap(m, 1);
          const half = gap / 2;
          const ox = perp.x * half;
          const oy = perp.y * half;
          const d1 = pointToSegmentDistance(pt.x, pt.y, sx + ox, sy + oy, tx + ox, ty + oy);
          const d2 = pointToSegmentDistance(pt.x, pt.y, sx - ox, sy - oy, tx - ox, ty - oy);
          candidateLane = d1 <= d2 ? "top" : "bottom";
          dist = Math.min(dist, d1, d2);
        }
        if (dist < tol && dist < minDist) {
          minDist = dist;
          picked = edge;
          pickedLane = candidateLane || this._resolveEdgeLane(edge, pt) || "";
        }
      }
      if (picked && pickedLane) this._markEdgeLaneHit(picked, pickedLane);
      return picked;
    }

    _computeBounds() {
      const nodes = this._nodes;
      if (!nodes.length) return null;
      let minX = Infinity;
      let minY = Infinity;
      let maxX = -Infinity;
      let maxY = -Infinity;
      nodes.forEach((node) => {
        const m = node.getModel();
        const r = Number(m.r) || 0;
        const x = Number(m.x) || 0;
        const y = Number(m.y) || 0;
        minX = Math.min(minX, x - r);
        maxX = Math.max(maxX, x + r);
        minY = Math.min(minY, y - r);
        maxY = Math.max(maxY, y + r);
      });
      if (!Number.isFinite(minX)) return null;
      return { minX, maxX, minY, maxY };
    }
  }

  function pointToSegmentDistance(px, py, x1, y1, x2, y2) {
    const dx = x2 - x1;
    const dy = y2 - y1;
    if (Math.abs(dx) < EPS && Math.abs(dy) < EPS) return Math.hypot(px - x1, py - y1);
    const t = ((px - x1) * dx + (py - y1) * dy) / (dx * dx + dy * dy);
    const clamped = clamp(t, 0, 1);
    const nx = x1 + clamped * dx;
    const ny = y1 + clamped * dy;
    return Math.hypot(px - nx, py - ny);
  }

  const engine = (window.AnalytixGraphEngine = window.AnalytixGraphEngine || {});
  engine.Graph = GraphAdapter;
  engine.Minimap = Minimap;
  engine.version = "AnalytixWebGL/0.1";
  engine.registerNode = () => {};
  engine.registerEdge = () => {};
})();

/* Shared layout core runtime for analysis flow */
(() => {
const root =
  typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : typeof window !== "undefined" ? window : null;
if (!root) return;

const layoutWorkerContract = root.__ANALYTIX_LAYOUT_WORKER_CONTRACT__;
if (
  !layoutWorkerContract ||
  !Array.isArray(layoutWorkerContract.LAYOUT_META_KEYS) ||
  typeof layoutWorkerContract.normalizeLayoutPreset !== "function" ||
  typeof layoutWorkerContract.normalizeLayoutDirection !== "function" ||
  typeof layoutWorkerContract.normalizeLayoutWorkerHints !== "function" ||
  typeof layoutWorkerContract.buildLayoutWorkerNodesPayload !== "function" ||
  typeof layoutWorkerContract.buildLayoutWorkerEdgesPayload !== "function" ||
  typeof layoutWorkerContract.projectLayoutNode !== "function" ||
  typeof layoutWorkerContract.projectLayoutNodes !== "function" ||
  typeof layoutWorkerContract.summarizeProjectedLayout !== "function" ||
  typeof layoutWorkerContract.buildLayoutWorkerRequest !== "function" ||
  typeof layoutWorkerContract.clonePrewarmLayoutInput !== "function" ||
  typeof layoutWorkerContract.buildLayoutWorkerResult !== "function"
) {
  throw new Error("layout-worker-contract-missing");
}
const layoutMetaModel = root.__ANALYTIX_FLOW_LAYOUT_META_MODEL__;
if (
  !layoutMetaModel ||
  !Array.isArray(layoutMetaModel.LAYOUT_META_KEYS) ||
  typeof layoutMetaModel.clearLayoutMeta !== "function"
) {
  throw new Error("layout-meta-model-missing");
}

const LAYOUT_META_KEYS = [...layoutMetaModel.LAYOUT_META_KEYS];
const LAYOUT_CONTRACT_VERSION = String(layoutWorkerContract.LAYOUT_CONTRACT_VERSION || "layout-contract-v1");
const LAYOUT_ALGO_VERSION =
  "compact-v20260216-semanticcorridor-coreengine-seedcore-radialguard-corridorbalance-leafring-corridorscatter-rigidradius-midspread-leafamountannulus-leafscatter-workeredgeamount-quantilemix-leafscatterboost-petalgap-scatterplus-leafgapbalance-leafgapuniform-leafhardmingap-leafmaxgap-coredistsafe-multicorehub-corepack-corridorshield-unknowncpmulticore-bridgechildpromotev2-territoryresync-swapcardringpack-pairgapv2";
const LAYOUT_MODE_RUNNER_KEY = "__ANALYTIX_LAYOUT_MODE_RUNNERS";
const normalizeLayoutPreset = layoutWorkerContract.normalizeLayoutPreset;
const normalizeLayoutDirection = layoutWorkerContract.normalizeLayoutDirection;
const normalizeLayoutWorkerHints = layoutWorkerContract.normalizeLayoutWorkerHints;
const buildLayoutWorkerNodesPayload = layoutWorkerContract.buildLayoutWorkerNodesPayload;
const buildLayoutWorkerEdgesPayload = layoutWorkerContract.buildLayoutWorkerEdgesPayload;
const projectLayoutNode = layoutWorkerContract.projectLayoutNode;
const projectLayoutNodes = layoutWorkerContract.projectLayoutNodes;
const summarizeProjectedLayout = layoutWorkerContract.summarizeProjectedLayout;
const buildLayoutWorkerRequest = layoutWorkerContract.buildLayoutWorkerRequest;
const clonePrewarmLayoutInput = layoutWorkerContract.clonePrewarmLayoutInput;
const clearLayoutMeta = layoutMetaModel.clearLayoutMeta;
const buildLayoutWorkerResult = (payload = {}) => {
  const src = payload && typeof payload === "object" ? payload : {};
  return layoutWorkerContract.buildLayoutWorkerResult({
    ...src,
    algoVersion: src?.algoVersion || LAYOUT_ALGO_VERSION,
  });
};

function getLayoutModeRegistry() {
  const current = root[LAYOUT_MODE_RUNNER_KEY];
  if (current && typeof current === "object") return current;
  const created = Object.create(null);
  root[LAYOUT_MODE_RUNNER_KEY] = created;
  return created;
}

function registerLayoutModeRunner(mode, runner) {
  const key = String(mode || "").trim().toLowerCase();
  if (!key || typeof runner !== "function") return false;
  const registry = getLayoutModeRegistry();
  if (!registry) return false;
  registry[key] = runner;
  return true;
}

function getLayoutModeRunner(mode) {
  const key = String(mode || "").trim().toLowerCase();
  if (!key) return null;
  const registry = getLayoutModeRegistry();
  const runner = registry?.[key];
  return typeof runner === "function" ? runner : null;
}

function runLayoutModeRunner(mode, context) {
  const runner = getLayoutModeRunner(mode);
  if (typeof runner !== "function") return false;
  try {
    return runner(context) !== false;
  } catch (e) {
    return false;
  }
}

function buildDegreeMap(edges) {
  const deg = new Map();
  for (const e of edges || []) {
    const s = e.source;
    const t = e.target;
    if (!s || !t) continue;
    deg.set(s, (deg.get(s) || 0) + 1);
    deg.set(t, (deg.get(t) || 0) + 1);
  }
  return deg;
}

function layoutScale(count) {
  if (count <= 3) return 1.85;
  if (count <= 6) return 1.58;
  if (count <= 10) return 1.36;
  if (count <= 18) return 1.18;
  if (count <= 40) return 1.06;
  return 1.0;
}

function stableHashUnit(value) {
  const s = String(value ?? "");
  let h = 2166136261 >>> 0;
  for (let i = 0; i < s.length; i += 1) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return (h >>> 0) / 4294967295;
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

function isCoreHardPinned(layoutOpts) {
  return normalizeCorePin(layoutOpts?.corePin) === "hard";
}

function computeLayoutCenter(nodes) {
  let sumX = 0;
  let sumY = 0;
  let count = 0;
  (nodes || []).forEach((n) => {
    const x = Number(n?.x);
    const y = Number(n?.y);
    if (!Number.isFinite(x) || !Number.isFinite(y)) return;
    sumX += x;
    sumY += y;
    count += 1;
  });
  if (!count) return null;
  return { x: sumX / count, y: sumY / count };
}

function computeLayoutBoundsCenter(nodes) {
  let minX = Infinity;
  let maxX = -Infinity;
  let minY = Infinity;
  let maxY = -Infinity;
  let count = 0;
  (nodes || []).forEach((n) => {
    const x = Number(n?.x);
    const y = Number(n?.y);
    if (!Number.isFinite(x) || !Number.isFinite(y)) return;
    minX = Math.min(minX, x);
    maxX = Math.max(maxX, x);
    minY = Math.min(minY, y);
    maxY = Math.max(maxY, y);
    count += 1;
  });
  if (!count) return null;
  return { x: (minX + maxX) / 2, y: (minY + maxY) / 2 };
}

function recenterLayout(nodes, targetCenter) {
  if (!targetCenter) return;
  const now = computeLayoutCenter(nodes);
  if (!now) return;
  const dx = targetCenter.x - now.x;
  const dy = targetCenter.y - now.y;
  if (!Number.isFinite(dx) || !Number.isFinite(dy)) return;
  if (Math.abs(dx) < 1e-6 && Math.abs(dy) < 1e-6) return;
  (nodes || []).forEach((n) => {
    if (!Number.isFinite(n?.x) || !Number.isFinite(n?.y)) return;
    n.x += dx;
    n.y += dy;
  });
}

function recenterLayoutByBounds(nodes, targetCenter) {
  if (!targetCenter) return;
  const now = computeLayoutBoundsCenter(nodes);
  if (!now) return;
  const dx = targetCenter.x - now.x;
  const dy = targetCenter.y - now.y;
  if (!Number.isFinite(dx) || !Number.isFinite(dy)) return;
  if (Math.abs(dx) < 1e-6 && Math.abs(dy) < 1e-6) return;
  (nodes || []).forEach((n) => {
    if (!Number.isFinite(n?.x) || !Number.isFinite(n?.y)) return;
    n.x += dx;
    n.y += dy;
  });
}

function stretchLayoutAroundCenter(nodes, factor, center) {
  if (!Number.isFinite(factor) || factor <= 1) return;
  const c = center || computeLayoutCenter(nodes);
  if (!c) return;
  (nodes || []).forEach((n) => {
    if (!Number.isFinite(n?.x) || !Number.isFinite(n?.y)) return;
    n.x = c.x + (n.x - c.x) * factor;
    n.y = c.y + (n.y - c.y) * factor;
  });
}

function computeAdaptiveLayoutStretch(nodeCount, edgeCount, mode) {
  const n = Math.max(1, Number(nodeCount) || 0);
  const e = Math.max(0, Number(edgeCount) || 0);
  const density = e / n;
  const sizeBoost = n <= 60 ? 1 : 1 + Math.min(0.72, Math.log(n / 60 + 1) * 0.26);
  const denseBoost = 1 + Math.min(0.52, Math.max(0, density - 2.2) * 0.06);
  const modeBoost = mode === "network" ? 1.08 : mode === "compact" ? 1.14 : 0.9;
  const modeCap = mode === "network" ? 2.2 : mode === "compact" ? 2.25 : 1.68;
  return Math.max(1, Math.min(modeCap, sizeBoost * denseBoost * modeBoost));
}

function relaxLayoutEdgeLength(nodes, edges, minLen, passes = 2, fixedIds = new Set()) {
  if (!Array.isArray(nodes) || nodes.length < 2 || !Array.isArray(edges) || !edges.length) return;
  const map = new Map(nodes.map((n) => [n.id, n]));
  const target = Math.max(6, Number(minLen) || 0);
  if (!target) return;
  for (let pass = 0; pass < passes; pass += 1) {
    (edges || []).forEach((edge) => {
      const s = map.get(edge?.source);
      const t = map.get(edge?.target);
      if (!s || !t || s === t) return;
      if (!Number.isFinite(s.x) || !Number.isFinite(s.y) || !Number.isFinite(t.x) || !Number.isFinite(t.y)) return;
      let dx = t.x - s.x;
      let dy = t.y - s.y;
      let len = Math.hypot(dx, dy);
      if (!Number.isFinite(len)) return;
      if (len < 1e-3) {
        const a = stableHashUnit(`${s.id}|${t.id}|${pass}`) * Math.PI * 2;
        dx = Math.cos(a);
        dy = Math.sin(a);
        len = 1;
      }
      if (len >= target) return;
      const ux = dx / len;
      const uy = dy / len;
      const diff = target - len;
      const movableS = !fixedIds.has(s.id);
      const movableT = !fixedIds.has(t.id);
      if (!movableS && !movableT) return;
      const capped = Math.min(target * 0.55, diff);
      if (movableS && movableT) {
        const shift = capped / 2;
        s.x -= ux * shift;
        s.y -= uy * shift;
        t.x += ux * shift;
        t.y += uy * shift;
      } else if (movableS) {
        s.x -= ux * capped;
        s.y -= uy * capped;
      } else if (movableT) {
        t.x += ux * capped;
        t.y += uy * capped;
      }
    });
  }
}

function relaxLayoutNodeDistance(nodes, minDist, passes = 2, fixedIds = new Set()) {
  if (!Array.isArray(nodes) || nodes.length < 2) return;
  const target = Math.max(6, Number(minDist) || 0);
  if (!target) return;
  const cell = target;
  for (let pass = 0; pass < passes; pass += 1) {
    const buckets = new Map();
    nodes.forEach((n, idx) => {
      if (!Number.isFinite(n?.x) || !Number.isFinite(n?.y)) return;
      const gx = Math.floor(n.x / cell);
      const gy = Math.floor(n.y / cell);
      const key = `${gx},${gy}`;
      if (!buckets.has(key)) buckets.set(key, []);
      buckets.get(key).push(idx);
    });
    nodes.forEach((a, i) => {
      if (!Number.isFinite(a?.x) || !Number.isFinite(a?.y)) return;
      const gx = Math.floor(a.x / cell);
      const gy = Math.floor(a.y / cell);
      for (let ox = -1; ox <= 1; ox += 1) {
        for (let oy = -1; oy <= 1; oy += 1) {
          const list = buckets.get(`${gx + ox},${gy + oy}`);
          if (!list || !list.length) continue;
          for (const j of list) {
            if (j <= i) continue;
            const b = nodes[j];
            if (!b || !Number.isFinite(b.x) || !Number.isFinite(b.y)) continue;
            let dx = b.x - a.x;
            let dy = b.y - a.y;
            let dist = Math.hypot(dx, dy);
            if (dist >= target) continue;
            if (dist < 1e-3) {
              const ang = stableHashUnit(`${a.id}|${b.id}|node|${pass}`) * Math.PI * 2;
              dx = Math.cos(ang);
              dy = Math.sin(ang);
              dist = 1;
            }
            const ux = dx / dist;
            const uy = dy / dist;
            const delta = Math.min(target * 0.42, target - dist);
            const movableA = !fixedIds.has(a.id);
            const movableB = !fixedIds.has(b.id);
            if (!movableA && !movableB) continue;
            if (movableA && movableB) {
              const half = delta / 2;
              a.x -= ux * half;
              a.y -= uy * half;
              b.x += ux * half;
              b.y += uy * half;
            } else if (movableA) {
              a.x -= ux * delta;
              a.y -= uy * delta;
            } else if (movableB) {
              b.x += ux * delta;
              b.y += uy * delta;
            }
          }
        }
      }
    });
  }
}

function estimateNodeGapStats(nodes) {
  const out = { sampled: 0, closePairs: 0, min: null };
  if (!Array.isArray(nodes) || !nodes.length) return out;
  const sampledNodes = [];
  const maxSample = 2200;
  const step = Math.max(1, Math.ceil(nodes.length / maxSample));
  for (let i = 0; i < nodes.length; i += step) {
    const node = nodes[i];
    if (!node || !Number.isFinite(node.x) || !Number.isFinite(node.y)) continue;
    sampledNodes.push(node);
  }
  out.sampled = sampledNodes.length;
  if (!sampledNodes.length) return out;
  const bucketSize = 96;
  const buckets = new Map();
  sampledNodes.forEach((node, idx) => {
    const gx = Math.floor(node.x / bucketSize);
    const gy = Math.floor(node.y / bucketSize);
    const key = `${gx},${gy}`;
    if (!buckets.has(key)) buckets.set(key, []);
    buckets.get(key).push(idx);
  });
  let minGap = Infinity;
  let closePairs = 0;
  sampledNodes.forEach((node, i) => {
    const gx = Math.floor(node.x / bucketSize);
    const gy = Math.floor(node.y / bucketSize);
    for (let ox = -1; ox <= 1; ox += 1) {
      for (let oy = -1; oy <= 1; oy += 1) {
        const list = buckets.get(`${gx + ox},${gy + oy}`);
        if (!list || !list.length) continue;
        for (const j of list) {
          if (j <= i) continue;
          const other = sampledNodes[j];
          const dist = Math.hypot((Number(other.x) || 0) - (Number(node.x) || 0), (Number(other.y) || 0) - (Number(node.y) || 0));
          if (!Number.isFinite(dist)) continue;
          minGap = Math.min(minGap, dist);
          if (dist < 72) closePairs += 1;
        }
      }
    }
  });
  if (minGap === Infinity && sampledNodes.length > 1 && sampledNodes.length <= 160) {
    for (let i = 0; i < sampledNodes.length; i += 1) {
      const a = sampledNodes[i];
      for (let j = i + 1; j < sampledNodes.length; j += 1) {
        const b = sampledNodes[j];
        const dist = Math.hypot((Number(b.x) || 0) - (Number(a.x) || 0), (Number(b.y) || 0) - (Number(a.y) || 0));
        if (!Number.isFinite(dist)) continue;
        minGap = Math.min(minGap, dist);
        if (dist < 72) closePairs += 1;
      }
    }
  }
  out.closePairs = closePairs;
  out.min = minGap === Infinity ? null : Number(minGap.toFixed(2));
  return out;
}

function buildLayoutAdjacency(nodeIds, edges) {
  const adjacency = new Map();
  nodeIds.forEach((id) => adjacency.set(id, new Set()));
  const edgeList = [];
  (edges || []).forEach((edge, index) => {
    const s = edge?.source;
    const t = edge?.target;
    if (!s || !t || !nodeIds.has(s) || !nodeIds.has(t)) return;
    adjacency.get(s).add(t);
    adjacency.get(t).add(s);
    edgeList.push({ index, source: s, target: t, edge });
  });
  return { adjacency, edgeList };
}

function collectLayoutComponents(nodeIds, adjacency) {
  const components = [];
  const visited = new Set();
  const sorted = Array.from(nodeIds).sort((a, b) => String(a).localeCompare(String(b), "zh-CN"));
  sorted.forEach((id) => {
    if (visited.has(id)) return;
    const queue = [id];
    const comp = new Set();
    visited.add(id);
    while (queue.length) {
      const cur = queue.shift();
      comp.add(cur);
      const next = adjacency.get(cur);
      if (!next) continue;
      next.forEach((nid) => {
        if (visited.has(nid)) return;
        visited.add(nid);
        queue.push(nid);
      });
    }
    components.push(comp);
  });
  return components;
}

function resolveDirectedEdge(edge) {
  const s = edge?.source;
  const t = edge?.target;
  if (!s || !t) return { directed: false, from: s, to: t, arrow: "none" };
  const rawArrow =
    edge?.edgeArrow ||
    edge?.arrow ||
    (edge?.mode === "double" ? "both" : edge?.showArrow ? "end" : "none");
  const arrow = ["none", "start", "end", "both"].includes(rawArrow) ? rawArrow : "none";
  if (arrow === "end") return { directed: true, from: s, to: t, arrow };
  if (arrow === "start") return { directed: true, from: t, to: s, arrow };
  return { directed: false, from: s, to: t, arrow };
}

function bfsDistanceFrom(seedId, adjacency) {
  const dist = new Map();
  if (!seedId || !(adjacency instanceof Map) || !adjacency.has(seedId)) return dist;
  const queue = [seedId];
  dist.set(seedId, 0);
  while (queue.length) {
    const cur = queue.shift();
    const next = adjacency.get(cur);
    if (!next) continue;
    const curDist = dist.get(cur) || 0;
    next.forEach((nid) => {
      if (dist.has(nid)) return;
      dist.set(nid, curDist + 1);
      queue.push(nid);
    });
  }
  return dist;
}

function pickCoreNodeIds(nodes, deg, focusId, maxCount = 1) {
  const withId = (nodes || []).filter((n) => n?.id);
  if (!withId.length) return [];
  if (withId.length <= Math.max(1, maxCount)) return withId.map((n) => n.id);
  const ordered = withId
    .slice()
    .sort((a, b) => (deg.get(b.id) || 0) - (deg.get(a.id) || 0) || String(a.id).localeCompare(String(b.id), "zh-CN"));
  const picked = [];
  const seen = new Set();
  if (focusId && withId.some((n) => n.id === focusId)) {
    picked.push(focusId);
    seen.add(focusId);
  }
  for (const node of ordered) {
    if (picked.length >= Math.max(1, maxCount)) break;
    if (seen.has(node.id)) continue;
    picked.push(node.id);
    seen.add(node.id);
  }
  return picked.slice(0, Math.max(1, maxCount));
}

function clampLayout(value, min, max) {
  return Math.max(min, Math.min(max, value));
}

function normalizeAngle(angle) {
  let v = Number(angle);
  if (!Number.isFinite(v)) return 0;
  while (v <= -Math.PI) v += Math.PI * 2;
  while (v > Math.PI) v -= Math.PI * 2;
  return v;
}

function positiveAngle(angle) {
  let v = Number(angle);
  if (!Number.isFinite(v)) return 0;
  while (v < 0) v += Math.PI * 2;
  while (v >= Math.PI * 2) v -= Math.PI * 2;
  return v;
}

function forwardAngleDistance(fromAngle, toAngle) {
  const from = positiveAngle(fromAngle);
  const to = positiveAngle(toAngle);
  const diff = to - from;
  return diff >= 0 ? diff : diff + Math.PI * 2;
}

function readableCorePairKey(a, b) {
  if (!a || !b || a === b) return "";
  return String(a) < String(b) ? `${a}::${b}` : `${b}::${a}`;
}

function computeReadableEdgeLength() {
  return 120;
}

function isAngleBlocked(angle, blocked, guard = 0) {
  if (!Array.isArray(blocked) || !blocked.length) return false;
  const extraGuard = Math.max(0, Number(guard) || 0);
  for (const row of blocked) {
    const base = Number(row?.angle);
    if (!Number.isFinite(base)) continue;
    const half = Math.max(0, Number(row?.half) || 0) + extraGuard;
    if (Math.abs(normalizeAngle(Number(angle) - base)) <= half) return true;
  }
  return false;
}

function projectToSegment(ax, ay, bx, by, px, py) {
  const vx = Number(bx) - Number(ax);
  const vy = Number(by) - Number(ay);
  const vv = vx * vx + vy * vy;
  const startX = Number(ax) || 0;
  const startY = Number(ay) || 0;
  const pointX = Number(px) || 0;
  const pointY = Number(py) || 0;
  if (vv <= 1e-12) {
    const dist = Math.hypot(pointX - startX, pointY - startY);
    return { t: 0, x: startX, y: startY, dist };
  }
  const tRaw = ((pointX - startX) * vx + (pointY - startY) * vy) / vv;
  const t = clampLayout(tRaw, 0, 1);
  const x = startX + vx * t;
  const y = startY + vy * t;
  const dist = Math.hypot(pointX - x, pointY - y);
  return { t, x, y, dist };
}

function addReadableCorePairScore(pairMap, a, b, score) {
  const key = readableCorePairKey(a, b);
  if (!key) return;
  pairMap.set(key, (pairMap.get(key) || 0) + (Number(score) || 0));
}

function registerLayoutExports(methods) {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : typeof window !== "undefined" ? window : null;
  if (!root) return false;
  const engine = root.AnalytixLayoutEngine;
  if (!engine || typeof engine !== "object" || !methods || typeof methods !== "object") return false;
  Object.entries(methods).forEach(([key, fn]) => {
    if (!key || typeof fn !== "function") return;
    engine[key] = fn;
  });
  return true;
}

function layoutFallbackGrid(nodes) {
  if (!Array.isArray(nodes) || !nodes.length) return nodes;
  const count = nodes.length;
  const scale = layoutScale(count);
  const cols = Math.ceil(Math.sqrt(count));
  const spacing = 120 * scale;
  const rows = Math.ceil(count / cols);
  const halfW = ((cols - 1) * spacing) / 2;
  const halfH = ((rows - 1) * spacing) / 2;
  nodes.forEach((node, i) => {
    const cx = i % cols;
    const cy = Math.floor(i / cols);
    node.x = cx * spacing - halfW;
    node.y = cy * spacing - halfH;
  });
  return nodes;
}

function runLayoutFallback(mode, nodes, edges, focusId, dir, hints) {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : typeof window !== "undefined" ? window : null;
  if (!root) return false;
  const engine = root.AnalytixLayoutEngine || {};

  if (mode === "network" && typeof engine.layoutNetwork === "function") {
    engine.layoutNetwork(nodes, edges, focusId);
    if (typeof engine.applyAdaptiveLayoutSpacing === "function") {
      engine.applyAdaptiveLayoutSpacing(nodes, edges, mode, focusId, hints || {});
    }
    return true;
  }

  if (mode === "hierarchy") {
    const d = dir?.hierarchy === "up" ? "vertical-reverse" : "vertical";
    if (typeof engine.layoutHierarchy === "function") {
      engine.layoutHierarchy(nodes, edges, focusId, { direction: d, hints });
      if (typeof engine.applyAdaptiveLayoutSpacing === "function") {
        engine.applyAdaptiveLayoutSpacing(nodes, edges, mode, focusId, hints || {});
      }
      return true;
    }
    if (typeof engine.layoutFlow === "function") {
      engine.layoutFlow(nodes, edges, focusId, { direction: d });
      if (typeof engine.applyAdaptiveLayoutSpacing === "function") {
        engine.applyAdaptiveLayoutSpacing(nodes, edges, mode, focusId, hints || {});
      }
      return true;
    }
  }

  if (mode === "flow" && typeof engine.layoutFlow === "function") {
    const d = dir?.flow === "left" ? "horizontal-reverse" : "horizontal";
    engine.layoutFlow(nodes, edges, focusId, { direction: d });
    if (typeof engine.applyAdaptiveLayoutSpacing === "function") {
      engine.applyAdaptiveLayoutSpacing(nodes, edges, mode, focusId, hints || {});
    }
    return true;
  }

  if (typeof engine.layoutRelation === "function") {
    engine.layoutRelation(nodes, edges, focusId, {
      spread: 1,
      preferredCoreIds: Array.isArray(hints?.preferredCoreIds) ? hints.preferredCoreIds : [],
      corePlacement: hints?.corePlacement,
      corePin: hints?.corePin,
      coreSource: hints?.coreSource,
      coreFallbackMinChildren: Number(hints?.coreFallbackMinChildren) || 2,
    });
    if (typeof engine.applyAdaptiveLayoutSpacing === "function") {
      engine.applyAdaptiveLayoutSpacing(nodes, edges, mode, focusId, hints || {});
    }
    return true;
  }

  return false;
}

function applyLayout(nodes, edges, preset, focusId, dir, hints = {}) {
  const mode = normalizeLayoutPreset(preset);
  const corePlacement = normalizeCorePlacement(hints?.corePlacement);
  const corePin = normalizeCorePin(hints?.corePin);
  const coreSource = normalizeCoreSource(hints?.coreSource);
  const normalizedHints = {
    ...hints,
    preferredCoreIds: Array.isArray(hints?.preferredCoreIds) ? hints.preferredCoreIds : [],
    corePlacement,
    corePin,
    coreSource,
  };
  clearLayoutMeta(nodes);

  const handled = runLayoutModeRunner(mode, {
    mode,
    nodes,
    edges,
    focusId,
    dir: dir || {},
    hints: normalizedHints,
    engine:
      (typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : window)
        ?.AnalytixLayoutEngine || null,
  });

  if (!handled) {
    const fallbackHandled = runLayoutFallback(mode, nodes, edges, focusId, dir || {}, normalizedHints);
    if (!fallbackHandled) layoutFallbackGrid(nodes);
  }

  return nodes;
}

const AnalytixLayoutEngine =
  root.AnalytixLayoutEngine && typeof root.AnalytixLayoutEngine === "object"
    ? root.AnalytixLayoutEngine
    : {};

Object.assign(AnalytixLayoutEngine, {
  LAYOUT_META_KEYS: [...LAYOUT_META_KEYS],
  LAYOUT_CONTRACT_VERSION,
  LAYOUT_ALGO_VERSION,
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
  registerLayoutExports,
  buildDegreeMap,
  layoutScale,
  stableHashUnit,
  normalizeCorePlacement,
  normalizeCorePin,
  normalizeCoreSource,
  isCoreHardPinned,
  computeLayoutCenter,
  computeLayoutBoundsCenter,
  recenterLayout,
  recenterLayoutByBounds,
  stretchLayoutAroundCenter,
  computeAdaptiveLayoutStretch,
  relaxLayoutEdgeLength,
  relaxLayoutNodeDistance,
  estimateNodeGapStats,
  clearLayoutMeta,
  buildLayoutAdjacency,
  collectLayoutComponents,
  resolveDirectedEdge,
  bfsDistanceFrom,
  pickCoreNodeIds,
  clampLayout,
  normalizeAngle,
  positiveAngle,
  forwardAngleDistance,
  readableCorePairKey,
  computeReadableEdgeLength,
  isAngleBlocked,
  projectToSegment,
  addReadableCorePairScore,
  registerLayoutModeRunner,
  getLayoutModeRunner,
  applyLayout,
});

root.AnalytixLayoutEngine = AnalytixLayoutEngine;
})();
